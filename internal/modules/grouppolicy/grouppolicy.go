// Package grouppolicy — ОПАСНЫЙ ЭКСПЕРИМЕНТАЛЬНЫЙ модуль групповых политик.
//
// Модуль по умолчанию выключен. При включении открывается раздел «Группы» в
// веб-панели и команды `chaff group ...` в CLI. Группа — это набор машин
// (по MAC или имени хоста) со своими правилами (ip/cidr/домен/url), которые
// действуют только на участников, когда группа включена.
//
// Глобальные правила (ручные и фиды) всегда приоритетнее групповых: глобальный
// allow перекрывает групповой block, глобальный block действует независимо от
// групповых allow. Одна машина не может состоять больше чем в одной группе.
//
// Доменные и URL-правила групп применяет sniblock (по MAC источника из NFQUEUE),
// IP-правила — этот модуль: per-group наборы (MAC участников + IPv4-префиксы)
// в цепочке groups таблицы chaff.
package grouppolicy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"go4.org/netipx"

	"github.com/coffeinium/chaff/internal/dataplane"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

const nfprotoIPv4 = 2

func init() {
	kernel.Register("grouppolicy", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel

	mu       sync.Mutex
	applied  int
	matchers []matcher
	lastErr  error
}

// matcher — применённые IP-правила одной группы, для подсветки соединений.
type matcher struct {
	macs map[string]bool
	set  *netipx.IPSet
}

func (m *Module) Name() string    { return "grouppolicy" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Title() string   { return "Групповые политики" }
func (m *Module) About() string {
	return "ОПАСНО · ЭКСПЕРИМЕНТ: правила, действующие только на машины группы"
}

func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

// Handles пуст: модуль применяет не глобальные индикаторы, а snap.Groups.
func (m *Module) Handles() []model.Kind { return nil }

// Enforce перестраивает per-group наборы и правила в цепочке groups:
// для каждой группы — drop пакетов от MAC участников к её IPv4-префиксам.
// Доменные правила групп применяет sniblock.
func (m *Module) Enforce(snap model.Ruleset) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := nftables.New()
	if err != nil {
		m.lastErr = err
		return err
	}
	tbl := &nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table}
	sets, err := c.GetSets(tbl)
	if err != nil {
		m.lastErr = fmt.Errorf("таблица %s недоступна (поднят ли bridge?): %w", dataplane.Table, err)
		return m.lastErr
	}

	ch := &nftables.Chain{Table: tbl, Name: dataplane.ChainGroups}
	c.FlushChain(ch)
	for _, s := range sets {
		if strings.HasPrefix(s.Name, dataplane.GroupSetPrefix) {
			c.DelSet(s)
		}
	}

	applied := 0
	var matchers []matcher
	for _, gp := range snap.Groups {
		prefixes := groupPrefixes(gp, snap.Allow.IPs)
		if len(gp.MACs) == 0 || len(prefixes) == 0 {
			continue
		}
		var macEls []nftables.SetElement
		for _, v := range gp.MACs {
			hw, err := net.ParseMAC(v)
			if err != nil || len(hw) != 6 {
				continue
			}
			macEls = append(macEls, nftables.SetElement{Key: []byte(hw)})
		}
		if len(macEls) == 0 {
			continue
		}
		var v4Els []nftables.SetElement
		for _, p := range prefixes {
			v4Els = append(v4Els, elems(p)...)
		}

		macSet := &nftables.Set{Table: tbl, Name: dataplane.GroupMACSet(gp.ID), KeyType: nftables.TypeEtherAddr}
		if err := c.AddSet(macSet, macEls); err != nil {
			m.lastErr = err
			return err
		}
		v4Set := &nftables.Set{Table: tbl, Name: dataplane.GroupV4Set(gp.ID), KeyType: nftables.TypeIPAddr, Interval: true}
		if err := c.AddSet(v4Set, v4Els); err != nil {
			m.lastErr = err
			return err
		}
		c.AddRule(&nftables.Rule{Table: tbl, Chain: ch, Exprs: []expr.Any{
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseLLHeader, Offset: 6, Len: 6},
			&expr.Lookup{SourceRegister: 1, SetName: macSet.Name, SetID: macSet.ID},
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{nfprotoIPv4}},
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 16, Len: 4},
			&expr.Lookup{SourceRegister: 1, SetName: v4Set.Name, SetID: v4Set.ID},
			&expr.Counter{},
			&expr.Verdict{Kind: expr.VerdictDrop},
		}})
		applied++

		var b netipx.IPSetBuilder
		for _, p := range prefixes {
			b.AddPrefix(p)
		}
		set, _ := b.IPSet()
		macs := make(map[string]bool, len(gp.MACs))
		for _, v := range gp.MACs {
			macs[v] = true
		}
		matchers = append(matchers, matcher{macs: macs, set: set})
	}

	if err := c.Flush(); err != nil {
		m.lastErr = err
		return err
	}
	m.applied = applied
	m.matchers = matchers
	m.lastErr = nil
	m.k.Log.Debug("grouppolicy: применено", "groups", applied)
	return nil
}

// Blocked: заблокировано ли соединение машины mac к адресу ip групповым IP-правилом.
func (m *Module) Blocked(mac string, ip netip.Addr) bool {
	mac = model.NormalizeMAC(mac)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.matchers {
		mt := &m.matchers[i]
		if mt.macs[mac] {
			return mt.set != nil && mt.set.Contains(ip.Unmap())
		}
	}
	return false
}

// groupPrefixes сводит IPv4-правила группы: block-префиксы минус allow группы
// минус глобальные allow (глобальные правила приоритетнее групповых).
func groupPrefixes(gp model.GroupPolicy, globalAllow []netip.Prefix) []netip.Prefix {
	var b netipx.IPSetBuilder
	for _, p := range gp.IPv4 {
		if p.Addr().Is4() {
			b.AddPrefix(p)
		}
	}
	for _, p := range gp.Allow.IPs {
		b.RemovePrefix(p)
	}
	for _, p := range globalAllow {
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

func (m *Module) Health() kernel.Health {
	groups, err := m.k.Store.ListGroups()
	if err != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + err.Error()}
	}
	on, machines, rules := 0, 0, 0
	for _, g := range groups {
		if g.Enabled {
			on++
		}
		machines += len(g.Members)
		rules += len(g.Rules)
	}
	met := map[string]any{"групп": len(groups), "включено": on, "участников": machines, "правил": rules}

	m.mu.Lock()
	lastErr := m.lastErr
	m.mu.Unlock()
	if lastErr != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + lastErr.Error(), Metrics: met}
	}
	return kernel.Health{
		OK:      true,
		Detail:  "ЭКСПЕРИМЕНТ · глобальные правила в приоритете",
		Metrics: met,
	}
}
