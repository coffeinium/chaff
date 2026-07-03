package macblock

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/google/nftables"

	"github.com/coffeinium/chaff/internal/dataplane"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("macblock", func() kernel.Module { return &Module{} })
}

type Module struct {
	k       *kernel.Kernel
	mu      sync.Mutex
	blocked map[string]bool
	lastErr error
}

func (m *Module) Name() string    { return "macblock" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Блокировка по MAC" }
func (m *Module) About() string {
	return "отрезает машину от сети по её MAC-адресу"
}

func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

func (m *Module) Handles() []model.Kind {
	return []model.Kind{model.KindMAC}
}

func (m *Module) Enforce(snap model.Ruleset) error {
	want := make(map[string]bool, len(snap.MACs))
	var all []nftables.SetElement
	for _, v := range snap.MACs {
		hw, err := net.ParseMAC(v)
		if err != nil || len(hw) != 6 {
			continue
		}
		key := hw.String()
		if want[key] {
			continue
		}
		want[key] = true
		all = append(all, nftables.SetElement{Key: []byte(hw)})
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := nftables.New()
	if err != nil {
		m.lastErr = err
		return err
	}
	tbl := &nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table}
	set, err := c.GetSetByName(tbl, dataplane.SetBadMAC)
	if err != nil {
		m.lastErr = fmt.Errorf("set %s не найден (поднят ли bridge?): %w", dataplane.SetBadMAC, err)
		return m.lastErr
	}

	c.FlushSet(set)
	if len(all) > 0 {
		if err := c.SetAddElements(set, all); err != nil {
			m.lastErr = err
			return err
		}
	}
	if err := c.Flush(); err != nil {
		m.lastErr = err
		return err
	}
	m.blocked = want
	m.lastErr = nil
	m.k.Log.Debug("macblock: применено", "macs", len(want))
	return nil
}

func (m *Module) Blocked(mac string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocked[model.NormalizeMAC(mac)]
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	met := map[string]any{"адресов": len(m.blocked)}
	if m.lastErr != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + m.lastErr.Error(), Metrics: met}
	}
	return kernel.Health{OK: true, Detail: "включена", Metrics: met}
}
