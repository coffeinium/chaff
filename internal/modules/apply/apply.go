package apply

import (
	"context"
	"time"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/kernel"
)

func init() {
	kernel.Register("apply", func() kernel.Module { return &Module{} })
}

type Module struct {
	k      *kernel.Kernel
	cancel context.CancelFunc
	done   chan struct{}
}

func (m *Module) Name() string    { return "apply" }
func (m *Module) Needs() []string { return nil }
func (m *Module) Title() string   { return "Применение правил" }
func (m *Module) About() string {
	return "переносит списки блокировки в сетевой фильтр"
}
func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}

func (m *Module) Start(ctx context.Context) error {
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
	if m.done != nil {
		select {
		case <-m.done:
		case <-ctx.Done():
		}
	}
	return nil
}

func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "работает"}
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)
	reload := m.k.Bus.Subscribe(bus.TopicReload)
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()

	m.reconcile("boot")

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-reload:
			reason, _ := ev.Data.(string)
			m.reconcile(reason)
		case <-tick.C:
			m.reconcile("periodic")
		}
	}
}

func (m *Module) reconcile(reason string) {
	if err := m.k.Reconcile(); err != nil {
		m.k.Log.Error("reconcile не удался", "reason", reason, "err", err)
		return
	}
	m.k.Log.Debug("reconcile", "reason", reason)
}
