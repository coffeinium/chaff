package api

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
	"github.com/coffeinium/chaff/internal/modules/analyzer"
	"github.com/coffeinium/chaff/internal/modules/feedsync"
	"github.com/coffeinium/chaff/internal/store"
)

func Handlers(k *kernel.Kernel) map[string]ipc.Handler {
	st := k.Store
	h := map[string]ipc.Handler{}

	h["status"] = func(_ ipc.Request) ipc.Response {
		counts, _ := st.CountByKind()
		return ipc.OK(map[string]any{
			"modules":    moduleList(k),
			"indicators": counts,
			"bridge":     bridgeState(k),
		})
	}

	h["hits"] = func(req ipc.Request) ipc.Response {
		limit := 100
		if v := req.Arg("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		hits, err := st.RecentHits(limit)
		if err != nil {
			return ipc.Err(err.Error())
		}
		enrichHits(st, hits)
		return ipc.OK(hits)
	}

	h["module.ls"] = func(_ ipc.Request) ipc.Response {
		return ipc.OK(moduleList(k))
	}
	h["module.enable"] = func(req ipc.Request) ipc.Response {
		return setModule(k, req.Arg("name"), true)
	}
	h["module.disable"] = func(req ipc.Request) ipc.Response {
		return setModule(k, req.Arg("name"), false)
	}

	h["source.add"] = func(req ipc.Request) ipc.Response {
		spec := model.SourceSpec{
			Name:      req.Arg("name"),
			Adapter:   req.Arg("adapter"),
			URI:       req.Arg("uri"),
			ColumnMap: parseColumnMap(req.Arg("map")),
		}
		if spec.Name == "" || spec.Adapter == "" {
			return ipc.Err("нужны --name и --adapter")
		}
		id, err := st.AddSource(spec)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(fmt.Sprintf("фид %q добавлен (id=%d)", spec.Name, id))
	}
	h["source.ls"] = func(_ ipc.Request) ipc.Response {
		specs, err := st.ListSources()
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(specs)
	}
	h["source.sync"] = func(_ ipc.Request) ipc.Response {
		if !running(k)["sync"] {
			return ipc.Err("модуль sync выключен")
		}
		n, err := feedsync.Run(context.Background(), k)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(fmt.Sprintf("синхронизировано индикаторов: %d", n))
	}
	h["source.enable"] = func(req ipc.Request) ipc.Response {
		return setSource(k, req, true)
	}
	h["source.disable"] = func(req ipc.Request) ipc.Response {
		return setSource(k, req, false)
	}
	h["source.rm"] = func(req ipc.Request) ipc.Response {
		spec, err := resolveSource(st, req)
		if err != nil {
			return ipc.Err(err.Error())
		}
		if err := st.RemoveSource(spec.ID); err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "source.rm"})
		return ipc.OK(fmt.Sprintf("источник %q удалён вместе с индикаторами", spec.Name))
	}
	h["source.indicators"] = func(req ipc.Request) ipc.Response {
		spec, err := resolveSource(st, req)
		if err != nil {
			return ipc.Err(err.Error())
		}
		limit := 500
		if v := req.Arg("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		inds, total, err := st.ListBySource(spec.ID, limit)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(map[string]any{"name": spec.Name, "total": total, "items": inds})
	}

	h["list"] = func(req ipc.Request) ipc.Response {
		kind := req.Arg("kind")
		var (
			inds []model.Indicator
			err  error
		)
		if kind == "" || kind == "all" {
			inds, err = st.ListAll()
		} else {
			inds, err = st.ListByKind(model.Kind(kind))
		}
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
	}

	h["ind.note"] = func(req ipc.Request) ipc.Response {
		id, err := strconv.ParseInt(req.Arg("id"), 10, 64)
		if err != nil || id <= 0 {
			return ipc.Err("нужен id индикатора")
		}
		n, err := st.UpdateNote(id, req.Arg("note"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		if n == 0 {
			return ipc.Err(fmt.Sprintf("индикатор id=%d не найден", id))
		}
		return ipc.OK("причина обновлена")
	}

	h["hosts"] = func(_ ipc.Request) ipc.Response {
		entries, err := st.ListHostnames()
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(entries)
	}

	h["apply"] = func(_ ipc.Request) ipc.Response {
		if err := k.Reconcile(); err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK("reconcile выполнен")
	}

	h["allow.add"] = func(req ipc.Request) ipc.Response {
		v := req.Arg("value")
		if v == "" {
			return ipc.Err("использование: chaff allow add VALUE")
		}
		if model.Classify(v) == model.KindUnknown {
			return ipc.Err("не распознан вид значения (ip/cidr/домен/url/mac)")
		}
		if err := st.AddManual(v, model.KindUnknown, model.ActionAllow, req.Arg("note")); err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "allow.add"})
		return ipc.OK(fmt.Sprintf("allow %q добавлен", v))
	}
	h["allow.rm"] = func(req ipc.Request) ipc.Response {
		v := req.Arg("value")
		if v == "" {
			return ipc.Err("использование: chaff allow rm VALUE")
		}
		n, err := st.RemoveManual(v)
		if err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "allow.rm"})
		return ipc.OK(fmt.Sprintf("удалено строк: %d", n))
	}
	h["allow.ls"] = func(_ ipc.Request) ipc.Response {
		inds, err := st.ListManual(model.ActionAllow)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
	}

	h["block.add"] = func(req ipc.Request) ipc.Response {
		v := req.Arg("value")
		if v == "" {
			return ipc.Err("использование: chaff block add VALUE")
		}
		if model.Classify(v) == model.KindUnknown {
			return ipc.Err("не распознан вид значения (ip/cidr/домен/url/mac)")
		}
		if err := st.AddManual(v, model.KindUnknown, model.ActionBlock, req.Arg("note")); err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "block.add"})
		return ipc.OK(fmt.Sprintf("block %q добавлен", v))
	}
	h["block.rm"] = func(req ipc.Request) ipc.Response {
		v := req.Arg("value")
		if v == "" {
			return ipc.Err("использование: chaff block rm VALUE")
		}
		n, err := st.RemoveManual(v)
		if err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "block.rm"})
		return ipc.OK(fmt.Sprintf("удалено строк: %d", n))
	}
	h["block.ls"] = func(_ ipc.Request) ipc.Response {
		inds, err := st.ListManual(model.ActionBlock)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
	}

	h["test"] = func(req ipc.Request) ipc.Response {
		return testValue(k, req.Arg("value"))
	}

	h["analyzer.flows"] = func(req ipc.Request) ipc.Response {
		fl, ok := runningModule(k, "analyzer").(interface{ Flows(int) []analyzer.Flow })
		if !ok {
			return ipc.Err("модуль analyzer выключен (chaff module enable analyzer)")
		}
		limit := 200
		if v := req.Arg("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		flows := fl.Flows(limit)
		markVerdicts(k, flows)
		enrichFlows(st, flows)
		return ipc.OK(flows)
	}

	h["net.up"] = func(req ipc.Request) ipc.Response {
		bc, ok := netCtl(k)
		if !ok {
			return ipc.Err("модуль bridge выключен")
		}
		in, out := req.Arg("in"), req.Arg("out")
		if in == "" || out == "" {
			return ipc.Err("нужны --in IF и --out IF")
		}
		if err := bc.Configure(in, out, req.Arg("name")); err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "net.up"})
		return ipc.OK(bc.Status())
	}
	h["net.down"] = func(_ ipc.Request) ipc.Response {
		bc, ok := netCtl(k)
		if !ok {
			return ipc.Err("модуль bridge выключен")
		}
		if err := bc.Teardown(); err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK("мост снят")
	}
	h["net.status"] = func(_ ipc.Request) ipc.Response {
		bc, ok := netCtl(k)
		if !ok {
			return ipc.Err("модуль bridge выключен")
		}
		return ipc.OK(bc.Status())
	}

	registerGroupHandlers(k, h)

	return h
}

// registerGroupHandlers — ОПАСНЫЙ ЭКСПЕРИМЕНТ: групповые политики.
func registerGroupHandlers(k *kernel.Kernel, h map[string]ipc.Handler) {
	st := k.Store
	gate := func() error {
		if !running(k)["grouppolicy"] {
			return fmt.Errorf("модуль grouppolicy выключен (chaff module enable grouppolicy)")
		}
		return nil
	}
	reload := func(reason string) {
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: reason})
	}

	h["group.ls"] = func(_ ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		groups, err := st.ListGroups()
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(groups)
	}
	h["group.scan"] = func(_ ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		entries, err := st.ListHostnames()
		if err != nil {
			return ipc.Err(err.Error())
		}
		claimed, _ := st.MemberMachineIndex()
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			grp := ""
			if e.Kind == "mac" {
				grp = claimed[model.NormalizeMAC(e.Key)]
			}
			out = append(out, map[string]any{
				"hostname": e.Hostname, "kind": e.Kind, "key": e.Key,
				"via": e.Via, "seen_at": e.SeenAt, "group": grp,
			})
		}
		return ipc.OK(out)
	}
	h["group.add"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.CreateGroup(req.Arg("name"), req.Arg("action"), req.Arg("note"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(fmt.Sprintf("группа %q создана (действие: %s, выключена)", g.Name, g.Action))
	}
	h["group.rm"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		name, err := st.DeleteGroup(req.Arg("ref"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		reload("group.rm")
		return ipc.OK(fmt.Sprintf("группа %q удалена", name))
	}
	h["group.enable"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.SetGroupEnabled(req.Arg("ref"), true)
		if err != nil {
			return ipc.Err(err.Error())
		}
		reload("group.enable")
		return ipc.OK(fmt.Sprintf("группа %q включена, политика применяется", g.Name))
	}
	h["group.disable"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.SetGroupEnabled(req.Arg("ref"), false)
		if err != nil {
			return ipc.Err(err.Error())
		}
		reload("group.disable")
		return ipc.OK(fmt.Sprintf("группа %q выключена", g.Name))
	}
	h["group.action"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.SetGroupAction(req.Arg("ref"), req.Arg("action"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		reload("group.action")
		return ipc.OK(fmt.Sprintf("действие группы %q: %s", g.Name, g.Action))
	}
	h["group.note"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.SetGroupNote(req.Arg("ref"), req.Arg("note"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(fmt.Sprintf("заметка группы %q обновлена", g.Name))
	}
	h["group.member.add"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, okMsg, err := st.AddGroupMember(req.Arg("ref"), req.Arg("value"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		if g.Enabled {
			reload("group.member.add")
		}
		return ipc.OK(okMsg)
	}
	h["group.member.rm"] = func(req ipc.Request) ipc.Response {
		if err := gate(); err != nil {
			return ipc.Err(err.Error())
		}
		g, err := st.RemoveGroupMember(req.Arg("ref"), req.Arg("value"))
		if err != nil {
			return ipc.Err(err.Error())
		}
		if g.Enabled {
			reload("group.member.rm")
		}
		return ipc.OK(fmt.Sprintf("участник удалён из группы %q", g.Name))
	}
}

func markVerdicts(k *kernel.Kernel, flows []analyzer.Flow) {
	ipb, _ := runningModule(k, "ipblock").(interface{ Blocked(netip.Addr) bool })
	mac, _ := runningModule(k, "macblock").(interface{ Blocked(string) bool })
	sni, _ := runningModule(k, "sniblock").(interface{ Verdict(string) string })
	for i := range flows {
		f := &flows[i]
		if mac != nil && f.SrcMAC != "" && mac.Blocked(f.SrcMAC) {
			f.SrcBlocked, f.Verdict, f.Blocked = true, "block", true
			continue
		}
		if ipb != nil {
			if a, err := netip.ParseAddr(f.DstIP); err == nil && ipb.Blocked(a) {
				f.Verdict, f.Blocked = "block", true
				continue
			}
		}
		if sni != nil && f.Kind != "ip" {
			if v := sni.Verdict(f.Dst); v != "" {
				f.Verdict, f.Blocked = v, v == "block"
			}
		}
	}
}

func enrichFlows(st *store.Store, flows []analyzer.Flow) {
	byMAC, byIP, err := st.Hostnames()
	if err != nil {
		return
	}
	for i := range flows {
		f := &flows[i]
		if h := byMAC[f.SrcMAC]; h != "" {
			f.SrcHost = h
			continue
		}
		f.SrcHost = byIP[f.SrcIP]
	}
}

func enrichHits(st *store.Store, hits []store.Hit) {
	_, byIP, _ := st.Hostnames()
	actions := map[string]string{}
	for i := range hits {
		h := &hits[i]
		if h.SrcIP != "" {
			h.SrcHost = byIP[h.SrcIP]
		}
		act, ok := actions[h.Indicator]
		if !ok {
			act = currentAction(st, h.Indicator)
			if act == "" && (h.Detail == "block" || h.Detail == "monitor") {
				act = h.Detail
			}
			actions[h.Indicator] = act
		}
		h.Action = act
	}
}

func currentAction(st *store.Store, value string) string {
	matches, _ := st.Lookup(value)
	act := ""
	for _, m := range matches {
		if m.SourceID == store.ManualSourceID {
			return string(m.Action)
		}
		if act == "" {
			act = string(m.Action)
		}
	}
	return act
}

func setModule(k *kernel.Kernel, name string, enabled bool) ipc.Response {
	if name == "" {
		return ipc.Err("нужно имя модуля")
	}
	known := false
	for _, n := range kernel.Registered() {
		if n == name {
			known = true
			break
		}
	}
	if !known {
		return ipc.Err("неизвестный модуль: " + name)
	}
	if err := k.SetModule(name, enabled); err != nil {
		return ipc.Err(err.Error())
	}
	if enabled {
		return ipc.OK(name + " включён")
	}
	return ipc.OK(name + " выключен")
}

func testValue(k *kernel.Kernel, value string) ipc.Response {
	if value == "" {
		return ipc.Err("использование: chaff test VALUE")
	}
	kind := model.Classify(value)
	if kind == model.KindMAC {
		value = model.NormalizeMAC(value)
	}
	layer := "нет"
	switch kind {
	case model.KindIP, model.KindCIDR:
		layer = "L3/L4 (ipblock)"
	case model.KindMAC:
		layer = "L2 (macblock)"
	case model.KindDomain, model.KindURL:
		layer = "L7 инлайн (sniblock)"
	}
	verdict := "нет совпадения"
	matches, _ := k.Store.Lookup(value)
	for _, m := range matches {
		verdict = string(m.Action)
		break
	}
	return ipc.OK(map[string]any{
		"value":   value,
		"kind":    kind,
		"layer":   layer,
		"verdict": verdict,
	})
}

func moduleList(k *kernel.Kernel) []map[string]any {
	run := running(k)
	var out []map[string]any
	for _, name := range kernel.Registered() {
		title, about := kernel.Describe(name)
		entry := map[string]any{
			"name":    name,
			"title":   title,
			"about":   about,
			"enabled": k.Store.IsModuleEnabled(name),
			"running": run[name],
		}
		if m := runningModule(k, name); m != nil {
			entry["health"] = m.Health()
		}
		out = append(out, entry)
	}
	return out
}

type netController interface {
	Configure(in, out, name string) error
	Teardown() error
	Status() string
}

func setSource(k *kernel.Kernel, req ipc.Request, enabled bool) ipc.Response {
	spec, err := resolveSource(k.Store, req)
	if err != nil {
		return ipc.Err(err.Error())
	}
	if err := k.Store.SetSourceEnabled(spec.ID, enabled); err != nil {
		return ipc.Err(err.Error())
	}
	k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "source.toggle"})
	if enabled {
		return ipc.OK(fmt.Sprintf("источник %q включён", spec.Name))
	}
	return ipc.OK(fmt.Sprintf("источник %q выключен", spec.Name))
}

func resolveSource(st *store.Store, req ipc.Request) (model.SourceSpec, error) {
	specs, err := st.ListSources()
	if err != nil {
		return model.SourceSpec{}, err
	}
	if idStr := req.Arg("id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return model.SourceSpec{}, fmt.Errorf("плохой id: %s", idStr)
		}
		for _, s := range specs {
			if s.ID == id {
				return s, nil
			}
		}
		return model.SourceSpec{}, fmt.Errorf("источник id=%d не найден", id)
	}
	name := req.Arg("name")
	if name == "" {
		return model.SourceSpec{}, fmt.Errorf("нужно имя источника")
	}
	for _, s := range specs {
		if s.Name == name {
			return s, nil
		}
	}
	return model.SourceSpec{}, fmt.Errorf("источник %q не найден", name)
}

func bridgeState(k *kernel.Kernel) map[string]any {
	out := map[string]any{"running": false, "configured": false, "up": false, "ok": false}
	m := runningModule(k, "bridge")
	if m == nil {
		return out
	}
	hh := m.Health()
	out["running"] = true
	out["ok"] = hh.OK
	out["detail"] = hh.Detail
	if v, ok := hh.Metrics["настроен"].(bool); ok {
		out["configured"] = v
	}
	if v, ok := hh.Metrics["поднят"].(bool); ok {
		out["up"] = v
	}
	return out
}

func netCtl(k *kernel.Kernel) (netController, bool) {
	if m := runningModule(k, "bridge"); m != nil {
		if c, ok := m.(netController); ok {
			return c, true
		}
	}
	return nil, false
}

func running(k *kernel.Kernel) map[string]bool {
	set := map[string]bool{}
	for _, m := range k.Running() {
		set[m.Name()] = true
	}
	return set
}

func runningModule(k *kernel.Kernel, name string) kernel.Module {
	for _, m := range k.Running() {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

func parseColumnMap(s string) map[string]int {
	out := map[string]int{}
	if s == "" {
		return out
	}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) != 2 {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(kv[1])); err == nil {
			out[strings.TrimSpace(kv[0])] = n
		}
	}
	return out
}
