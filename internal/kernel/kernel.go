package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/store"
)

type Kernel struct {
	Config *config.Config
	Log    *slog.Logger
	Store  *store.Store
	Bus    *bus.Bus

	mu      sync.Mutex
	running []Module
}

func New(cfg *config.Config, log *slog.Logger, st *store.Store, b *bus.Bus) *Kernel {
	return &Kernel{Config: cfg, Log: log, Store: st, Bus: b}
}

func (k *Kernel) Boot(ctx context.Context) error {
	want := make(map[string]bool)
	for _, name := range Registered() {
		if k.Store.IsModuleEnabled(name) {
			want[name] = true
		}
	}

	inst := make(map[string]Module)
	var add func(name string) error
	add = func(name string) error {
		if _, ok := inst[name]; ok {
			return nil
		}
		m, err := instantiate(name)
		if err != nil {
			return err
		}
		inst[name] = m
		for _, dep := range m.Needs() {
			if !want[dep] {
				k.Log.Warn("включаю зависимость автоматически", "module", name, "dep", dep)
			}
			if err := add(dep); err != nil {
				return err
			}
		}
		return nil
	}
	for name := range want {
		if err := add(name); err != nil {
			return err
		}
	}

	order, err := topoSort(inst)
	if err != nil {
		return err
	}

	for _, name := range order {
		m := inst[name]
		if err := m.Init(k); err != nil {
			return fmt.Errorf("init %s: %w", name, err)
		}
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
		k.mu.Lock()
		k.running = append(k.running, m)
		k.mu.Unlock()
		k.Log.Info("модуль запущен", "module", name)
	}
	return nil
}

func (k *Kernel) Shutdown(ctx context.Context) {
	k.mu.Lock()
	mods := append([]Module(nil), k.running...)
	k.running = nil
	k.mu.Unlock()

	for i := len(mods) - 1; i >= 0; i-- {
		m := mods[i]
		if err := m.Stop(ctx); err != nil {
			k.Log.Error("модуль не остановился чисто", "module", m.Name(), "err", err)
		} else {
			k.Log.Info("модуль остановлен", "module", m.Name())
		}
	}
}

func (k *Kernel) Running() []Module {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]Module(nil), k.running...)
}

func (k *Kernel) Enforcers() []Enforcer {
	var es []Enforcer
	for _, m := range k.Running() {
		if e, ok := m.(Enforcer); ok {
			es = append(es, e)
		}
	}
	return es
}

func (k *Kernel) SourceFor(adapter string) (Source, bool) {
	for _, m := range k.Running() {
		if s, ok := m.(Source); ok && s.Adapter() == adapter {
			return s, true
		}
	}
	return nil, false
}

func (k *Kernel) Reconcile() error {
	snap, err := k.Store.BuildRuleset()
	if err != nil {
		return err
	}
	for _, e := range k.Enforcers() {
		if err := e.Enforce(snap); err != nil {
			k.Log.Error("enforce не удался", "module", e.Name(), "err", err)
		}
	}
	return nil
}
