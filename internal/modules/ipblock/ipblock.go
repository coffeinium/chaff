package ipblock

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"go4.org/netipx"

	"github.com/coffeinium/chaff/internal/dataplane"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("ipblock", func() kernel.Module { return &Module{} })
}

type Module struct {
	k       *kernel.Kernel
	mu      sync.Mutex
	count   int
	set     *netipx.IPSet
	lastErr error
}

func (m *Module) Name() string    { return "ipblock" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Блокировка по IP" }
func (m *Module) About() string {
	return "обрывает соединения к адресам из чёрного списка"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

func (m *Module) Handles() []model.Kind {
	return []model.Kind{model.KindIP, model.KindCIDR}
}

func (m *Module) Enforce(snap model.Ruleset) error {
	snoop, _ := m.k.Store.SnoopedIPsForBlockedDomains()
	return m.applySet(desiredPrefixes(snap.IPv4, snoop, snap.Allow.IPs))
}

func (m *Module) applySet(want []netip.Prefix) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := nftables.New()
	if err != nil {
		m.lastErr = err
		return err
	}
	tbl := &nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table}
	set, err := c.GetSetByName(tbl, dataplane.SetBadV4)
	if err != nil {
		m.lastErr = fmt.Errorf("set %s не найден (поднят ли bridge?): %w", dataplane.SetBadV4, err)
		return m.lastErr
	}

	var all []nftables.SetElement
	for _, p := range want {
		all = append(all, elems(p)...)
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
	var b netipx.IPSetBuilder
	for _, p := range want {
		b.AddPrefix(p)
	}
	m.set, _ = b.IPSet()
	m.count = len(want)
	m.lastErr = nil
	m.k.Log.Debug("ipblock: применено", "prefixes", len(want))
	return nil
}

func (m *Module) Blocked(ip netip.Addr) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set != nil && m.set.Contains(ip.Unmap())
}

func (m *Module) Health() kernel.Health {
	drops, hasDrops := dropCount()
	m.mu.Lock()
	defer m.mu.Unlock()
	met := map[string]any{"адресов": m.count}
	if hasDrops {
		met["дропов"] = drops
	}
	if m.lastErr != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + m.lastErr.Error(), Metrics: met}
	}
	return kernel.Health{OK: true, Detail: "включена", Metrics: met}
}

func dropCount() (uint64, bool) {
	c, err := nftables.New()
	if err != nil {
		return 0, false
	}
	tbl := &nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table}
	rules, err := c.GetRules(tbl, &nftables.Chain{Table: tbl, Name: dataplane.ChainForward})
	if err != nil {
		return 0, false
	}
	for _, r := range rules {
		bad, cnt := false, uint64(0)
		var have bool
		for _, e := range r.Exprs {
			if lk, ok := e.(*expr.Lookup); ok && lk.SetName == dataplane.SetBadV4 {
				bad = true
			}
			if x, ok := e.(*expr.Counter); ok {
				cnt, have = x.Packets, true
			}
		}
		if bad && have {
			return cnt, true
		}
	}
	return 0, false
}

func desiredPrefixes(blockV4 []netip.Prefix, snoop []string, allow []netip.Prefix) []netip.Prefix {
	var b netipx.IPSetBuilder
	for _, p := range blockV4 {
		if p.Addr().Is4() {
			b.AddPrefix(p)
		}
	}
	for _, s := range snoop {
		if a, err := netip.ParseAddr(s); err == nil && a.Is4() {
			b.AddPrefix(netip.PrefixFrom(a, 32))
		}
	}
	for _, p := range allow {
		b.RemovePrefix(p)
	}
	s, err := b.IPSet()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, p := range s.Prefixes() {
		if p.Addr().Is4() {
			out = append(out, p)
		}
	}
	return out
}

func elems(p netip.Prefix) []nftables.SetElement {
	r := netipx.RangeOfPrefix(p)
	start := r.From().As4()
	end := r.To().Next().As4()
	return []nftables.SetElement{
		{Key: start[:]},
		{Key: end[:], IntervalEnd: true},
	}
}
