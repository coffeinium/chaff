// Пакет feedsync (имя модуля "sync") периодически тянет каждый включённый фид
// через его адаптер, апсертит индикаторы и шлёт reload, чтобы apply пересобрал
// data-plane.
package feedsync

import (
	"context"
	"time"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/kernel"
)

func init() {
	kernel.Register("sync", func() kernel.Module { return &Module{} })
}

type Module struct {
	k      *kernel.Kernel
	cancel context.CancelFunc
	done   chan struct{}
}

func (m *Module) Name() string    { return "sync" }
func (m *Module) Needs() []string { return nil }
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
	return kernel.Health{OK: true, Detail: "планировщик фидов работает"}
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := Run(ctx, m.k); err != nil {
				m.k.Log.Error("плановый синк не удался", "err", err)
			}
		}
	}
}

// Run тянет все включённые фиды один раз. Вынесена наружу, чтобы команда
// source.sync могла дёрнуть немедленный синк через работающий модуль.
func Run(ctx context.Context, k *kernel.Kernel) (int, error) {
	specs, err := k.Store.EnabledSources()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, spec := range specs {
		src, ok := k.SourceFor(spec.Adapter)
		if !ok {
			_ = k.Store.UpdateSourceStatus(spec.ID, "нет адаптера: "+spec.Adapter, 0, "")
			k.Log.Warn("синк: нет модуля-адаптера", "source", spec.Name, "adapter", spec.Adapter)
			continue
		}
		inds, err := src.Fetch(ctx, spec)
		if err != nil {
			_ = k.Store.UpdateSourceStatus(spec.ID, "ошибка: "+err.Error(), 0, "")
			k.Log.Error("синк: fetch не удался", "source", spec.Name, "err", err)
			continue
		}
		n, err := k.Store.UpsertIndicators(spec.ID, inds)
		if err != nil {
			_ = k.Store.UpdateSourceStatus(spec.ID, "ошибка стора: "+err.Error(), 0, "")
			k.Log.Error("синк: upsert не удался", "source", spec.Name, "err", err)
			continue
		}
		_ = k.Store.UpdateSourceStatus(spec.ID, "ok", n, "")
		total += n
		k.Log.Info("синк: фид обновлён", "source", spec.Name, "indicators", n)
	}
	k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "sync"})
	return total, nil
}
