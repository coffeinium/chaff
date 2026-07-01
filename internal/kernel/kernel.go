package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/store"
)

type Kernel struct {
	Config *config.Config
	Log    *slog.Logger
	Store  *store.Store
	Bus    *bus.Bus

	ctx  context.Context
	opMu sync.Mutex

	mu      sync.Mutex
	running []Module
}

func New(cfg *config.Config, log *slog.Logger, st *store.Store, b *bus.Bus) *Kernel {
	return &Kernel{Config: cfg, Log: log, Store: st, Bus: b}
}

func (k *Kernel) Boot(ctx context.Context) error {
	k.ctx = ctx
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

func (k *Kernel) runningByName(name string) Module {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, m := range k.running {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

func (k *Kernel) SetModule(name string, enabled bool) error {
	k.opMu.Lock()
	defer k.opMu.Unlock()
	if k.ctx == nil {
		return fmt.Errorf("ядро не готово")
	}
	if enabled {
		if err := k.Store.SetModuleEnabled(name, true); err != nil {
			return err
		}
		if err := k.startRec(name); err != nil {
			_ = k.Store.SetModuleEnabled(name, false)
			return err
		}
	} else {
		if err := k.Store.SetModuleEnabled(name, false); err != nil {
			return err
		}
		if err := k.stopModule(name); err != nil {
			_ = k.Store.SetModuleEnabled(name, true)
			return err
		}
	}
	return k.Reconcile()
}

func (k *Kernel) startRec(name string) error {
	if k.runningByName(name) != nil {
		return nil
	}
	m, err := instantiate(name)
	if err != nil {
		return err
	}
	for _, dep := range m.Needs() {
		if err := k.startRec(dep); err != nil {
			return err
		}
	}
	if err := m.Init(k); err != nil {
		return fmt.Errorf("init %s: %w", name, err)
	}
	if err := m.Start(k.ctx); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	k.mu.Lock()
	k.running = append(k.running, m)
	k.mu.Unlock()
	k.Log.Info("модуль запущен", "module", name)
	return nil
}

func (k *Kernel) stopModule(name string) error {
	target := k.runningByName(name)
	if target == nil {
		return nil
	}
	for _, m := range k.Running() {
		if m.Name() == name {
			continue
		}
		for _, dep := range m.Needs() {
			if dep == name {
				return fmt.Errorf("сначала выключи %s, он зависит от %s", m.Name(), name)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := target.Stop(ctx); err != nil {
		return err
	}
	k.mu.Lock()
	out := k.running[:0]
	for _, m := range k.running {
		if m.Name() != name {
			out = append(out, m)
		}
	}
	k.running = out
	k.mu.Unlock()
	k.Log.Info("модуль остановлен", "module", name)
	return nil
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
