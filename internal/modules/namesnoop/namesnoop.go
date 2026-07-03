package namesnoop

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket/afpacket"
	"github.com/miekg/dns"
	"golang.org/x/net/bpf"

	"github.com/coffeinium/chaff/internal/dpi"
	"github.com/coffeinium/chaff/internal/kernel"
)

const rewriteAfter = 10 * time.Minute

func init() {
	kernel.Register("namesnoop", func() kernel.Module { return &Module{} })
}

type seen struct {
	host string
	at   time.Time
}

type Module struct {
	k     *kernel.Kernel
	iface string

	cancel context.CancelFunc
	done   chan struct{}
	tp     *afpacket.TPacket

	mu      sync.Mutex
	cache   map[string]seen
	byMAC   map[string]string
	lastErr error
}

func (m *Module) Name() string    { return "namesnoop" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Имена машин" }
func (m *Module) About() string {
	return "узнаёт hostname клиентов из DHCP и mDNS"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	m.cache = map[string]seen{}
	m.byMAC = map[string]string{}
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
		m.k.Log.Info("namesnoop: нет интерфейса, настрой `chaff net up`")
		return nil
	}
	tp, err := afpacket.NewTPacket(
		afpacket.OptInterface(m.iface),
		afpacket.OptTPacketVersion(afpacket.TPacketVersion3),
		afpacket.OptFrameSize(2048),
		afpacket.OptBlockSize(1<<20),
		afpacket.OptNumBlocks(16),
	)
	if err != nil {
		m.lastErr = err
		m.k.Log.Error("namesnoop: afpacket не открылся", "iface", m.iface, "err", err)
		return nil
	}
	if raw, err := bpf.Assemble(nameFilter()); err == nil {
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
	n := len(m.cache)
	err := m.lastErr
	m.mu.Unlock()
	if m.iface == "" {
		return kernel.Health{OK: true, Detail: "простаивает (сеть не настроена)"}
	}
	if err != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + err.Error()}
	}
	return kernel.Health{OK: true, Detail: "слушает на " + m.iface, Metrics: map[string]any{"имён": n}}
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)
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
	if !ok || p.Proto != 17 {
		return
	}
	switch {
	case p.SrcPort == 5353:
		m.handleMDNS(p.Payload)
	case p.SrcPort == 67 || p.DstPort == 67:
		m.handleDHCP(p.Payload)
	}
}

func (m *Module) handleMDNS(payload []byte) {
	var msg dns.Msg
	if err := msg.Unpack(payload); err != nil || !msg.Response {
		return
	}
	for _, rr := range append(msg.Answer, msg.Extra...) {
		var ip string
		switch a := rr.(type) {
		case *dns.A:
			ip = a.A.String()
		case *dns.AAAA:
			ip = a.AAAA.String()
		default:
			continue
		}
		host := shortHost(rr.Header().Name)
		if host == "" {
			continue
		}
		m.put("ip", ip, host, "mdns")
	}
}

func (m *Module) handleDHCP(b []byte) {
	msg, ok := parseDHCP(b)
	if !ok {
		return
	}
	switch msg.op {
	case 1:
		if msg.hostname == "" {
			return
		}
		if msg.mac != "" {
			m.mu.Lock()
			m.byMAC[msg.mac] = msg.hostname
			m.mu.Unlock()
			m.put("mac", msg.mac, msg.hostname, "dhcp")
		}
		if msg.clientIP != "" {
			m.put("ip", msg.clientIP, msg.hostname, "dhcp")
		}
	case 2:
		if msg.mac == "" || msg.yourIP == "" {
			return
		}
		m.mu.Lock()
		host := m.byMAC[msg.mac]
		m.mu.Unlock()
		if host != "" {
			m.put("ip", msg.yourIP, host, "dhcp")
		}
	}
}

func (m *Module) put(kind, key, host, via string) {
	ck := kind + "|" + key
	now := time.Now()
	m.mu.Lock()
	if prev, ok := m.cache[ck]; ok && prev.host == host && now.Sub(prev.at) < rewriteAfter {
		m.mu.Unlock()
		return
	}
	m.cache[ck] = seen{host: host, at: now}
	m.mu.Unlock()
	if err := m.k.Store.PutHostname(kind, key, host, via); err != nil {
		m.mu.Lock()
		m.lastErr = err
		m.mu.Unlock()
	}
}

type dhcpMsg struct {
	op       byte
	mac      string
	hostname string
	clientIP string
	yourIP   string
}

func parseDHCP(b []byte) (dhcpMsg, bool) {
	if len(b) < 241 || (b[0] != 1 && b[0] != 2) {
		return dhcpMsg{}, false
	}
	if b[236] != 99 || b[237] != 130 || b[238] != 83 || b[239] != 99 {
		return dhcpMsg{}, false
	}
	msg := dhcpMsg{op: b[0]}
	if b[1] == 1 && b[2] == 6 {
		msg.mac = net.HardwareAddr(b[28:34]).String()
	}
	if ip, ok := netip.AddrFromSlice(b[12:16]); ok && !ip.IsUnspecified() {
		msg.clientIP = ip.String()
	}
	if ip, ok := netip.AddrFromSlice(b[16:20]); ok && !ip.IsUnspecified() {
		msg.yourIP = ip.String()
	}
	for i := 240; i+1 < len(b); {
		code := b[i]
		if code == 0 {
			i++
			continue
		}
		if code == 255 {
			break
		}
		ln := int(b[i+1])
		if i+2+ln > len(b) {
			break
		}
		val := b[i+2 : i+2+ln]
		switch code {
		case 12:
			msg.hostname = shortHost(string(val))
		case 50:
			if msg.clientIP == "" && ln == 4 {
				if ip, ok := netip.AddrFromSlice(val); ok && !ip.IsUnspecified() {
					msg.clientIP = ip.String()
				}
			}
		}
		i += 2 + ln
	}
	return msg, true
}

func shortHost(name string) string {
	h := dpi.NormalizeHost(name)
	h = strings.TrimSuffix(h, ".local")
	if h == "" || strings.ContainsAny(h, " \t") {
		return ""
	}
	return h
}

func nameFilter() []bpf.Instruction {
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x0800, SkipFalse: 12},
		bpf.LoadAbsolute{Off: 23, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipFalse: 10},
		bpf.LoadAbsolute{Off: 20, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 8},
		bpf.LoadMemShift{Off: 14},
		bpf.LoadIndirect{Off: 14, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 67, SkipTrue: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 5353, SkipTrue: 3},
		bpf.LoadIndirect{Off: 16, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 67, SkipTrue: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 5353, SkipFalse: 1},
		bpf.RetConstant{Val: 262144},
		bpf.RetConstant{Val: 0},
	}
}
