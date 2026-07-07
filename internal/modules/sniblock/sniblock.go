package sniblock

import (
	"context"
	"net"
	"strings"
	"sync"

	nfqueue "github.com/florianl/go-nfqueue/v2"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"

	"github.com/coffeinium/chaff/internal/dataplane"
	"github.com/coffeinium/chaff/internal/dpi"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
	"github.com/coffeinium/chaff/internal/store"
)

func init() {
	kernel.Register("sniblock", func() kernel.Module { return &Module{} })
}

type decision int

const (
	decNone decision = iota
	decAllow
	decBlock
	decMonitor
)

type Module struct {
	k      *kernel.Kernel
	nf     *nfqueue.Nfqueue
	cancel context.CancelFunc

	mu      sync.Mutex
	block   map[string]bool
	monitor map[string]bool
	allow   map[string]bool
	groups  []groupRules
	urls    int
	hits    int
	lastErr error
}

// groupRules — доменные правила одной группы, действуют только на её MAC.
type groupRules struct {
	name  string
	macs  map[string]bool
	block map[string]bool
	allow map[string]bool
}

func (m *Module) Name() string    { return "sniblock" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Блокировка по сайтам" }
func (m *Module) About() string {
	return "обрывает соединения к запрещённым доменам по имени сайта"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	m.block, m.monitor, m.allow = map[string]bool{}, map[string]bool{}, map[string]bool{}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	cfg := nfqueue.Config{
		NfQueue:      uint16(m.k.Config.NFQueueNum),
		MaxPacketLen: 0xFFFF,
		MaxQueueLen:  0xFFFF,
		AfFamily:     unix.AF_INET,
		Copymode:     nfqueue.NfQnlCopyPacket,
		Flags:        nfqueue.NfQaCfgFlagFailOpen,
	}
	nf, err := nfqueue.Open(&cfg)
	if err != nil {
		m.lastErr = err
		m.k.Log.Error("sniblock: NFQUEUE не открылся", "queue", m.k.Config.NFQueueNum, "err", err)
		return nil
	}
	_ = nf.SetOption(netlink.NoENOBUFS, true)
	m.nf = nf

	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	if err := nf.RegisterWithErrorFunc(loopCtx, m.hook, func(e error) int { return 0 }); err != nil {
		m.lastErr = err
		m.k.Log.Error("sniblock: register NFQUEUE", "err", err)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.nf != nil {
		_ = m.nf.Close()
	}
	return nil
}

func (m *Module) Handles() []model.Kind {
	return []model.Kind{model.KindDomain, model.KindURL}
}

func (m *Module) Enforce(snap model.Ruleset) error {
	block := make(map[string]bool)
	monitor := make(map[string]bool)
	allow := make(map[string]bool)
	for _, d := range snap.Domains {
		switch d.Action {
		case model.ActionBlock:
			block[dpi.NormalizeHost(d.Domain)] = true
		case model.ActionMonitor:
			monitor[dpi.NormalizeHost(d.Domain)] = true
		}
	}
	for d := range snap.Allow.Domains {
		allow[dpi.NormalizeHost(d)] = true
	}
	var groups []groupRules
	for _, gp := range snap.Groups {
		g := groupRules{name: gp.Name, macs: map[string]bool{}, block: map[string]bool{}, allow: map[string]bool{}}
		for _, mc := range gp.MACs {
			g.macs[mc] = true
		}
		for _, d := range gp.Domains {
			if d.Action == model.ActionBlock {
				g.block[dpi.NormalizeHost(d.Domain)] = true
			}
		}
		for d := range gp.Allow.Domains {
			g.allow[dpi.NormalizeHost(d)] = true
		}
		if len(g.macs) > 0 && (len(g.block) > 0 || len(g.allow) > 0) {
			groups = append(groups, g)
		}
	}
	m.mu.Lock()
	m.block, m.monitor, m.allow, m.groups, m.urls = block, monitor, allow, groups, len(snap.URLs)
	m.mu.Unlock()
	return nil
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastErr != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + m.lastErr.Error()}
	}
	met := map[string]any{
		"запрещено": len(m.block), "наблюдение": len(m.monitor), "исключения": len(m.allow), "срабатываний": m.hits,
	}
	if len(m.groups) > 0 {
		met["групп"] = len(m.groups)
	}
	return kernel.Health{OK: true, Detail: "включена", Metrics: met}
}

func (m *Module) hook(a nfqueue.Attribute) int {
	if a.PacketID == nil {
		return 0
	}
	id := *a.PacketID
	if a.Payload == nil {
		m.pass(id)
		return 0
	}
	p, ok := dpi.Parse(*a.Payload)
	if !ok || len(p.Payload) == 0 {
		m.pass(id)
		return 0
	}

	host, layer := m.extractHost(p)
	if host == "" {
		m.allowMark(id)
		return 0
	}
	switch m.decide(host, srcMAC(a)) {
	case decBlock:
		m.logHit(layer, host, p.SrcAddr.String(), "block")
		m.denyMark(id)
	case decMonitor:
		m.logHit(layer, host, p.SrcAddr.String(), "monitor")
		m.allowMark(id)
	default:
		m.allowMark(id)
	}
	return 0
}

// srcMAC достаёт MAC источника из метаданных NFQUEUE (для групповых правил).
func srcMAC(a nfqueue.Attribute) string {
	if a.HwAddr != nil && len(*a.HwAddr) >= 6 {
		return net.HardwareAddr((*a.HwAddr)[:6]).String()
	}
	return ""
}

func (m *Module) extractHost(p dpi.Packet) (host, layer string) {
	switch p.DstPort {
	case 443:
		if ch, ok := dpi.ParseClientHello(p.Payload); ok {
			return dpi.NormalizeHost(ch.SNI), "sni"
		}
	case 80:
		if h, ok := dpi.HTTPHost(p.Payload); ok {
			return dpi.NormalizeHost(h), "http"
		}
	}
	return "", ""
}

func (m *Module) Verdict(host, mac string) string {
	switch m.decide(dpi.NormalizeHost(host), model.NormalizeMAC(mac)) {
	case decBlock:
		return "block"
	case decMonitor:
		return "monitor"
	case decAllow:
		return "allow"
	}
	return ""
}

// decide: глобальные правила всегда приоритетнее групповых, поэтому сначала
// глобальный вердикт, и только при его отсутствии — правила группы машины.
func (m *Module) decide(host, mac string) decision {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inDomainSet(host, m.allow) {
		return decAllow
	}
	if inDomainSet(host, m.block) {
		return decBlock
	}
	if inDomainSet(host, m.monitor) {
		return decMonitor
	}
	if mac != "" {
		for i := range m.groups {
			g := &m.groups[i]
			if !g.macs[mac] {
				continue
			}
			if inDomainSet(host, g.allow) {
				return decAllow
			}
			if inDomainSet(host, g.block) {
				return decBlock
			}
			break // машина состоит максимум в одной группе
		}
	}
	return decNone
}

func inDomainSet(host string, set map[string]bool) bool {
	for host != "" {
		if set[host] {
			return true
		}
		i := strings.IndexByte(host, '.')
		if i < 0 {
			return false
		}
		host = host[i+1:]
	}
	return false
}

func (m *Module) pass(id uint32) { _ = m.nf.SetVerdict(id, nfqueue.NfAccept) }
func (m *Module) allowMark(id uint32) {
	_ = m.nf.SetVerdictWithConnMark(id, nfqueue.NfAccept, dataplane.CtMarkAllow)
}
func (m *Module) denyMark(id uint32) {
	_ = m.nf.SetVerdictWithConnMark(id, nfqueue.NfDrop, dataplane.CtMarkDeny)
}

func (m *Module) logHit(layer, host, src, action string) {
	m.mu.Lock()
	m.hits++
	m.mu.Unlock()
	_ = m.k.Store.AddHit(store.Hit{Layer: layer, Indicator: host, SrcIP: src, Detail: action})
}
