package analyzer

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/gopacket/gopacket/afpacket"
	"golang.org/x/net/bpf"

	"github.com/coffeinium/chaff/internal/dpi"
	"github.com/coffeinium/chaff/internal/kernel"
)

const (
	maxFlows = 8192
	flowTTL  = 5 * time.Minute
)

func init() {
	kernel.Register("analyzer", func() kernel.Module { return &Module{} })
}

type Flow struct {
	SrcMAC  string `json:"src_mac"`
	SrcIP   string `json:"src_ip"`
	Dst     string `json:"dst"`
	DstIP   string `json:"dst_ip"`
	Port    uint16 `json:"port"`
	Proto   string `json:"proto"`
	Kind    string `json:"kind"`
	Packets uint64 `json:"packets"`
	Bytes   uint64 `json:"bytes"`
	First   int64  `json:"first"`
	Last    int64  `json:"last"`
}

type flow struct {
	clientMAC string
	clientIP  netip.Addr
	serverIP  netip.Addr
	port      uint16
	proto     uint8
	kind      string
	label     string
	pkts      uint64
	bytes     uint64
	first     int64
	last      int64
}

type Module struct {
	k     *kernel.Kernel
	iface string

	cancel context.CancelFunc
	done   chan struct{}
	tp     *afpacket.TPacket

	mu      sync.Mutex
	flows   map[string]*flow
	lastErr error
}

func (m *Module) Name() string    { return "analyzer" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Анализатор соединений" }
func (m *Module) About() string {
	return "живой список соединений: кто (mac/ip) куда (ip/sni/домен) и сколько"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	m.flows = map[string]*flow{}
	if blob, err := k.Store.GetModuleConfig("bridge"); err == nil {
		var bc struct {
			In string `json:"in"`
		}
		_ = json.Unmarshal([]byte(blob), &bc)
		m.iface = bc.In
	}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.iface == "" {
		m.k.Log.Info("analyzer: нет интерфейса, настрой `chaff net up`")
		return nil
	}
	tp, err := afpacket.NewTPacket(
		afpacket.OptInterface(m.iface),
		afpacket.OptTPacketVersion(afpacket.TPacketVersion3),
		afpacket.OptFrameSize(2048),
		afpacket.OptBlockSize(1<<20),
		afpacket.OptNumBlocks(64),
	)
	if err != nil {
		m.lastErr = err
		m.k.Log.Error("analyzer: afpacket не открылся", "iface", m.iface, "err", err)
		return nil
	}
	if raw, err := bpf.Assemble(ipFilter()); err == nil {
		_ = tp.SetBPF(raw)
	}
	m.tp = tp

	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.loop(loopCtx)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.tp != nil {
		m.tp.Close()
	}
	if m.done != nil {
		select {
		case <-m.done:
		case <-ctx.Done():
		}
	}
	return nil
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	n := len(m.flows)
	err := m.lastErr
	m.mu.Unlock()
	if m.iface == "" {
		return kernel.Health{OK: true, Detail: "простаивает (сеть не настроена)"}
	}
	if err != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + err.Error()}
	}
	return kernel.Health{OK: true, Detail: "следит на " + m.iface, Metrics: map[string]any{"потоков": n}}
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)
	prune := time.NewTicker(30 * time.Second)
	defer prune.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-prune.C:
				m.prune()
			}
		}
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		data, _, err := m.tp.ZeroCopyReadPacketData()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		m.handle(data)
	}
}

func (m *Module) handle(frame []byte) {
	p, ok := dpi.Parse(frame)
	if !ok {
		return
	}
	src := srcMAC(frame)
	now := time.Now().Unix()

	a := netip.AddrPortFrom(p.SrcAddr, p.SrcPort)
	b := netip.AddrPortFrom(p.DstAddr, p.DstPort)
	clientIsSrc := orientClient(p)
	key := flowKey(a, b, p.Proto)

	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.flows[key]
	if f == nil {
		if len(m.flows) >= maxFlows {
			m.evictOldest()
		}
		f = &flow{proto: p.Proto, first: now}
		if clientIsSrc {
			f.clientMAC, f.clientIP, f.serverIP, f.port = src, p.SrcAddr, p.DstAddr, p.DstPort
		} else {
			f.clientIP, f.serverIP, f.port = p.DstAddr, p.SrcAddr, p.SrcPort
		}
		m.flows[key] = f
	}
	if clientIsSrc && f.clientMAC == "" {
		f.clientMAC = src
	}
	f.pkts++
	f.bytes += uint64(len(frame))
	f.last = now

	if clientIsSrc && len(p.Payload) > 0 && f.kind != "sni" {
		if f.port == 443 {
			if hlo, ok := dpi.ParseClientHello(p.Payload); ok && hlo.SNI != "" {
				f.kind, f.label = "sni", dpi.NormalizeHost(hlo.SNI)
			}
		} else if f.port == 80 && f.kind == "" {
			if host, ok := dpi.HTTPHost(p.Payload); ok {
				f.kind, f.label = "host", dpi.NormalizeHost(host)
			}
		}
	}
}

func (m *Module) Flows(limit int) []Flow {
	m.mu.Lock()
	fls := make([]flow, 0, len(m.flows))
	for _, f := range m.flows {
		fls = append(fls, *f)
	}
	m.mu.Unlock()

	sort.Slice(fls, func(i, j int) bool { return fls[i].last > fls[j].last })
	if limit > 0 && len(fls) > limit {
		fls = fls[:limit]
	}
	out := make([]Flow, 0, len(fls))
	for i := range fls {
		f := &fls[i]
		kind, label := f.kind, f.label
		if kind == "" {
			if d, ok, _ := m.k.Store.DomainForIP(f.serverIP.String()); ok && d != "" {
				kind, label = "domain", d
			} else {
				kind, label = "ip", f.serverIP.String()
			}
		}
		out = append(out, Flow{
			SrcMAC:  f.clientMAC,
			SrcIP:   f.clientIP.String(),
			Dst:     label,
			DstIP:   f.serverIP.String(),
			Port:    f.port,
			Proto:   protoName(f.proto),
			Kind:    kind,
			Packets: f.pkts,
			Bytes:   f.bytes,
			First:   f.first,
			Last:    f.last,
		})
	}
	return out
}

func (m *Module) prune() {
	cut := time.Now().Add(-flowTTL).Unix()
	m.mu.Lock()
	for k, f := range m.flows {
		if f.last < cut {
			delete(m.flows, k)
		}
	}
	m.mu.Unlock()
}

func (m *Module) evictOldest() {
	var oldestKey string
	var oldest int64 = 1<<63 - 1
	for k, f := range m.flows {
		if f.last < oldest {
			oldest, oldestKey = f.last, k
		}
	}
	if oldestKey != "" {
		delete(m.flows, oldestKey)
	}
}

func orientClient(p dpi.Packet) bool {
	if p.Proto == 6 && p.Flags&0x12 == 0x02 {
		return true
	}
	return p.SrcPort >= p.DstPort
}

func flowKey(a, b netip.AddrPort, proto uint8) string {
	x, y := a.String(), b.String()
	if x > y {
		x, y = y, x
	}
	return string(rune(proto)) + x + "|" + y
}

func srcMAC(frame []byte) string {
	if len(frame) >= 14 && frame[0]>>4 != 4 && frame[0]>>4 != 6 {
		return net.HardwareAddr(frame[6:12]).String()
	}
	return ""
}

func protoName(p uint8) string {
	switch p {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	}
	return "ip"
}

func ipFilter() []bpf.Instruction {
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x0800, SkipTrue: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x86DD, SkipTrue: 1},
		bpf.RetConstant{Val: 0},
		bpf.RetConstant{Val: 262144},
	}
}
