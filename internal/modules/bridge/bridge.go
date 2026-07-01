package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"

	"github.com/coffeinium/chaff/internal/dataplane"
	"github.com/coffeinium/chaff/internal/kernel"
)

const nftRejectTCPRST = 1

const nfprotoIPv4 = 2

func init() {
	kernel.Register("bridge", func() kernel.Module { return &Module{} })
}

type conf struct {
	In   string `json:"in"`
	Out  string `json:"out"`
	Name string `json:"name"`
}

type Module struct {
	k *kernel.Kernel

	mu      sync.Mutex
	conf    conf
	built   bool
	lastErr error
}

func (m *Module) Name() string    { return "bridge" }
func (m *Module) Needs() []string { return nil }
func (m *Module) Title() string   { return "Врезка в сеть" }
func (m *Module) About() string {
	return "прозрачный мост между локальной сетью и роутером"
}

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	if blob, err := k.Store.GetModuleConfig("bridge"); err == nil {
		_ = json.Unmarshal([]byte(blob), &m.conf)
	}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.conf.In == "" || m.conf.Out == "" {
		m.k.Log.Info("bridge: не настроен, задай `chaff net up --in IF --out IF`")
		return nil
	}
	m.apply()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {

	return nil
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	met := map[string]any{"настроен": m.conf.In != "", "поднят": m.built}
	switch {
	case m.conf.In == "":
		return kernel.Health{OK: true, Detail: "не настроен (chaff net up)", Metrics: met}
	case m.lastErr != nil:
		return kernel.Health{OK: false, Detail: "ошибка: " + m.lastErr.Error(), Metrics: met}
	case m.built:
		return kernel.Health{OK: true, Detail: fmt.Sprintf("up %s (%s<->%s)", m.name(), m.conf.In, m.conf.Out), Metrics: met}
	default:
		return kernel.Health{OK: false, Detail: "настроен, не поднят", Metrics: met}
	}
}

func (m *Module) Configure(in, out, name string) error {
	m.mu.Lock()
	m.conf.In, m.conf.Out = in, out
	if name != "" {
		m.conf.Name = name
	}
	c := m.conf
	m.mu.Unlock()

	blob, _ := json.Marshal(c)
	if err := m.k.Store.SetModuleConfig("bridge", string(blob)); err != nil {
		return err
	}
	m.apply()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func (m *Module) Teardown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, err := nftables.New(); err == nil {
		c.DelTable(&nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table})
		_ = c.Flush()
	}
	if l, err := netlink.LinkByName(m.name()); err == nil {
		_ = netlink.LinkDel(l)
	}
	m.built = false
	return nil
}

func (m *Module) Status() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.conf.In == "":
		return "не настроен (chaff net up --in IF --out IF)"
	case m.built:
		return fmt.Sprintf("up: %s, %s<->%s, nft inet/%s, NFQUEUE %d",
			m.name(), m.conf.In, m.conf.Out, dataplane.Table, m.k.Config.NFQueueNum)
	case m.lastErr != nil:
		return "ошибка: " + m.lastErr.Error()
	default:
		return "настроен, не поднят"
	}
}

func (m *Module) name() string {
	if m.conf.Name != "" {
		return m.conf.Name
	}
	return "br0"
}

func (m *Module) apply() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.built = false
	if err := enableBrNetfilter(); err != nil {
		m.fail("br_netfilter", err)
		return
	}
	if err := m.setupBridge(); err != nil {
		m.fail("bridge", err)
		return
	}
	if err := m.buildRuleset(uint16(m.k.Config.NFQueueNum)); err != nil {
		m.fail("nftables", err)
		return
	}
	m.lastErr = nil
	m.built = true
	m.k.Log.Info("bridge: поднят", "name", m.name(), "in", m.conf.In, "out", m.conf.Out)
}

func (m *Module) fail(stage string, err error) {
	m.lastErr = fmt.Errorf("%s: %w", stage, err)
	m.k.Log.Error("bridge: настройка не удалась", "stage", stage, "err", err)
}

func enableBrNetfilter() error {
	_ = exec.Command("modprobe", "br_netfilter").Run()
	_ = exec.Command("modprobe", "nf_conntrack").Run()
	if err := os.WriteFile("/proc/sys/net/bridge/bridge-nf-call-iptables", []byte("1"), 0o644); err != nil {
		return err
	}

	_ = os.WriteFile("/proc/sys/net/bridge/bridge-nf-call-ip6tables", []byte("0"), 0o644)
	return nil
}

func (m *Module) setupBridge() error {
	name := m.name()
	var br *netlink.Bridge
	if l, err := netlink.LinkByName(name); err == nil {
		if b, ok := l.(*netlink.Bridge); ok {
			br = b
		} else {
			return fmt.Errorf("%s существует, но это не мост", name)
		}
	}
	if br == nil {
		la := netlink.NewLinkAttrs()
		la.Name = name
		br = &netlink.Bridge{LinkAttrs: la}
		if err := netlink.LinkAdd(br); err != nil {
			return fmt.Errorf("создать %s: %w", name, err)
		}
	}
	for _, ifn := range []string{m.conf.In, m.conf.Out} {
		port, err := netlink.LinkByName(ifn)
		if err != nil {
			return fmt.Errorf("интерфейс %s: %w", ifn, err)
		}

		if addrs, err := netlink.AddrList(port, netlink.FAMILY_ALL); err == nil {
			for i := range addrs {
				_ = netlink.AddrDel(port, &addrs[i])
			}
		}
		_ = netlink.LinkSetNoMaster(port)
		if err := netlink.LinkSetMaster(port, br); err != nil {
			return fmt.Errorf("enslave %s: %w", ifn, err)
		}
		if err := netlink.LinkSetUp(port); err != nil {
			return fmt.Errorf("up %s: %w", ifn, err)
		}
	}
	return netlink.LinkSetUp(br)
}

func ref[T any](v T) *T { return &v }

func (m *Module) buildRuleset(queueNum uint16) error {
	c, err := nftables.New()
	if err != nil {
		return err
	}
	if tbls, err := c.ListTablesOfFamily(nftables.TableFamilyINet); err == nil {
		for _, t := range tbls {
			if t.Name == dataplane.Table {
				c.DelTable(t)
				_ = c.Flush()
				break
			}
		}
	}

	tbl := c.AddTable(&nftables.Table{Family: nftables.TableFamilyINet, Name: dataplane.Table})
	setV4 := &nftables.Set{
		Table:    tbl,
		Name:     dataplane.SetBadV4,
		KeyType:  nftables.TypeIPAddr,
		Interval: true,
	}
	if err := c.AddSet(setV4, nil); err != nil {
		return err
	}
	ch := c.AddChain(&nftables.Chain{
		Name:     dataplane.ChainForward,
		Table:    tbl,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityFilter,
		Policy:   ref(nftables.ChainPolicyAccept),
	})

	c.AddRule(&nftables.Rule{Table: tbl, Chain: ch, Exprs: append(ctMarkMatch(dataplane.CtMarkAllow),
		&expr.Verdict{Kind: expr.VerdictAccept})})

	c.AddRule(&nftables.Rule{Table: tbl, Chain: ch, Exprs: append(ctMarkMatch(dataplane.CtMarkDeny),
		&expr.Reject{Type: nftRejectTCPRST, Code: 0})})

	c.AddRule(&nftables.Rule{Table: tbl, Chain: ch, Exprs: []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{nfprotoIPv4}},
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 16, Len: 4},
		&expr.Lookup{SourceRegister: 1, SetName: setV4.Name, SetID: setV4.ID},
		&expr.Counter{},
		&expr.Verdict{Kind: expr.VerdictDrop},
	}})

	for _, port := range []uint16{80, 443} {
		c.AddRule(&nftables.Rule{Table: tbl, Chain: ch, Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{nfprotoIPv4}},
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{6}},
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(port)},
			&expr.Ct{Register: 1, Key: expr.CtKeyMARK},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.NativeEndian.PutUint32(0)},
			&expr.Queue{Num: queueNum, Flag: expr.QueueFlagBypass},
		}})
	}
	return c.Flush()
}

func ctMarkMatch(mark uint32) []expr.Any {
	return []expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeyMARK},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.NativeEndian.PutUint32(mark)},
	}
}
