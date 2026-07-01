package dnssnoop

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gopacket/gopacket/afpacket"
	"github.com/miekg/dns"
	"golang.org/x/net/bpf"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/dpi"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("dnssnoop", func() kernel.Module { return &Module{} })
}

type Module struct {
	k     *kernel.Kernel
	iface string

	cancel context.CancelFunc
	done   chan struct{}
	tp     *afpacket.TPacket

	mu      sync.Mutex
	blocked map[string]bool
	count   int
	lastPub time.Time
	lastErr error
}

func (m *Module) Name() string    { return "dnssnoop" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Анализ DNS" }
func (m *Module) About() string {
	return "вычисляет адреса вредоносных доменов из ответов DNS"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	m.blocked = map[string]bool{}

	if blob, err := k.Store.GetModuleConfig("bridge"); err == nil {
		var bc struct {
			Out string `json:"out"`
		}
		_ = json.Unmarshal([]byte(blob), &bc)
		m.iface = bc.Out
	}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.iface == "" {
		m.k.Log.Info("dnssnoop: нет uplink-интерфейса, настрой `chaff net up`")
		return nil
	}
	m.refresh()

	tp, err := afpacket.NewTPacket(
		afpacket.OptInterface(m.iface),
		afpacket.OptTPacketVersion(afpacket.TPacketVersion3),
		afpacket.OptFrameSize(2048),
		afpacket.OptBlockSize(1<<20),
		afpacket.OptNumBlocks(64),
	)
	if err != nil {
		m.lastErr = err
		m.k.Log.Error("dnssnoop: afpacket не открылся", "iface", m.iface, "err", err)
		return nil
	}
	if raw, err := bpf.Assemble(dnsFilter()); err == nil {
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
	defer m.mu.Unlock()
	if m.iface == "" {
		return kernel.Health{OK: true, Detail: "простаивает (сеть не настроена)"}
	}
	if m.lastErr != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + m.lastErr.Error()}
	}
	return kernel.Health{OK: true, Detail: "анализирует на " + m.iface, Metrics: map[string]any{"доменов": m.count}}
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)

	reload := m.k.Bus.Subscribe(bus.TopicReload)
	expire := time.NewTicker(time.Minute)
	defer expire.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reload:
				m.refresh()
			case <-expire.C:
				_, _ = m.k.Store.ExpireSnoop()
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
	if !ok || p.Proto != 17 || p.SrcPort != 53 {
		return
	}
	var msg dns.Msg
	if err := msg.Unpack(p.Payload); err != nil || !msg.Response || len(msg.Question) == 0 {
		return
	}
	qname := dpi.NormalizeHost(msg.Question[0].Name)
	if !m.isBlocked(qname) {
		return
	}
	wrote := false
	for _, rr := range msg.Answer {
		a, ok := rr.(*dns.A)
		if !ok {
			continue
		}
		ttl := clampTTL(rr.Header().Ttl)
		if err := m.k.Store.PutSnoop(qname, a.A.String(), ttl); err == nil {
			wrote = true
		}
	}
	if wrote {
		m.maybeReload()
	}
}

func (m *Module) isBlocked(domain string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocked[domain]
}

func (m *Module) refresh() {
	inds, err := m.k.Store.ListByKind(model.KindDomain)
	if err != nil {
		return
	}
	set := make(map[string]bool, len(inds))
	for _, in := range inds {
		if in.Action == model.ActionBlock {
			set[dpi.NormalizeHost(in.Value)] = true
		}
	}
	m.mu.Lock()
	m.blocked = set
	m.count = len(set)
	m.mu.Unlock()
}

func (m *Module) maybeReload() {
	m.mu.Lock()
	if time.Since(m.lastPub) < 2*time.Second {
		m.mu.Unlock()
		return
	}
	m.lastPub = time.Now()
	m.mu.Unlock()
	m.k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "dnssnoop"})
}

func clampTTL(ttl uint32) time.Duration {
	d := time.Duration(ttl) * time.Second
	if d < 10*time.Second {
		return 10 * time.Second
	}
	if d > 300*time.Second {
		return 300 * time.Second
	}
	return d
}

func dnsFilter() []bpf.Instruction {
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x0800, SkipFalse: 8},
		bpf.LoadAbsolute{Off: 23, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipFalse: 6},
		bpf.LoadAbsolute{Off: 20, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 4},
		bpf.LoadMemShift{Off: 14},
		bpf.LoadIndirect{Off: 14, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 53, SkipFalse: 1},
		bpf.RetConstant{Val: 262144},
		bpf.RetConstant{Val: 0},
	}
}
