package api

import (
	"context"
	"fmt"
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

	h["list"] = func(req ipc.Request) ipc.Response {
		inds, err := st.ListByKind(model.Kind(req.Arg("kind")))
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
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
		return ipc.OK(fl.Flows(limit))
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

	return h
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
	layer := "нет"
	switch kind {
	case model.KindIP, model.KindCIDR:
		layer = "L3/L4 (ipblock)"
	case model.KindDomain, model.KindURL:
		layer = "L7 инлайн (sniblock)"
	case model.KindSHA256, model.KindMD5:
		layer = "нет, хеш файла не дело сетевого файрволла"
	}
	verdict := "нет совпадения"
	matches, _ := k.Store.Lookup(value)
	for _, m := range matches {
		verdict = string(m.Action)
		break
	}
	if kind == model.KindSHA256 || kind == model.KindMD5 {
		if verdict != "нет совпадения" {
			verdict = "хранится (на сетевом уровне не enforce'ится)"
		}
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
