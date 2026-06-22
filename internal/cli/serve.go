package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/log"
	"github.com/coffeinium/chaff/internal/model"
	"github.com/coffeinium/chaff/internal/modules/feedsync"
	"github.com/coffeinium/chaff/internal/store"
)

func cmdServe(_ []string) int {
	cfg := config.Load()
	logger := log.New(cfg.LogLevel)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("открыть стор", "err", err)
		return 1
	}
	defer st.Close()

	k := kernel.New(cfg, logger, st, bus.New())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := k.Boot(ctx); err != nil {
		logger.Error("boot не удался", "err", err)
		return 1
	}

	srv := ipc.NewServer(cfg.SocketPath, logger)
	registerHandlers(srv, k)
	if err := srv.Listen(); err != nil {
		logger.Error("listen не удался", "socket", cfg.SocketPath, "err", err)
		shutdown(k)
		return 1
	}
	go srv.Serve(ctx)
	logger.Info("chaff готов", "socket", cfg.SocketPath, "db", cfg.DBPath)

	<-ctx.Done()
	logger.Info("останавливаюсь")
	srv.Close()
	shutdown(k)
	return 0
}

func shutdown(k *kernel.Kernel) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	k.Shutdown(ctx)
}

func registerHandlers(srv *ipc.Server, k *kernel.Kernel) {
	st := k.Store

	srv.Handle("status", func(_ ipc.Request) ipc.Response {
		counts, _ := st.CountByKind()
		return ipc.OK(map[string]any{
			"modules":    moduleList(k),
			"indicators": counts,
		})
	})

	srv.Handle("module.ls", func(_ ipc.Request) ipc.Response {
		return ipc.OK(moduleList(k))
	})
	srv.Handle("module.enable", func(req ipc.Request) ipc.Response {
		return setModule(st, req.Arg("name"), true)
	})
	srv.Handle("module.disable", func(req ipc.Request) ipc.Response {
		return setModule(st, req.Arg("name"), false)
	})

	srv.Handle("source.add", func(req ipc.Request) ipc.Response {
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
	})
	srv.Handle("source.ls", func(_ ipc.Request) ipc.Response {
		specs, err := st.ListSources()
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(specs)
	})
	srv.Handle("source.sync", func(_ ipc.Request) ipc.Response {
		if !running(k)["sync"] {
			return ipc.Err("модуль sync выключен")
		}
		n, err := feedsync.Run(context.Background(), k)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(fmt.Sprintf("синхронизировано индикаторов: %d", n))
	})

	srv.Handle("list", func(req ipc.Request) ipc.Response {
		inds, err := st.ListByKind(model.Kind(req.Arg("kind")))
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
	})

	srv.Handle("apply", func(_ ipc.Request) ipc.Response {
		if err := k.Reconcile(); err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK("reconcile выполнен")
	})

	srv.Handle("allow.add", func(req ipc.Request) ipc.Response {
		v := req.Arg("value")
		if v == "" {
			return ipc.Err("использование: chaff allow add VALUE")
		}
		if err := st.AddManual(v, model.KindUnknown, model.ActionAllow); err != nil {
			return ipc.Err(err.Error())
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "allow.add"})
		return ipc.OK(fmt.Sprintf("allow %q добавлен", v))
	})
	srv.Handle("allow.rm", func(req ipc.Request) ipc.Response {
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
	})
	srv.Handle("allow.ls", func(_ ipc.Request) ipc.Response {
		inds, err := st.ListManual(model.ActionAllow)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(inds)
	})

	srv.Handle("test", func(req ipc.Request) ipc.Response {
		return testValue(k, req.Arg("value"))
	})

	srv.Handle("net.up", func(req ipc.Request) ipc.Response {
		return ipc.OK(fmt.Sprintf("заглушка: поднял бы мост %q <-> %q (data-plane ждёт verify)",
			req.Arg("in"), req.Arg("out")))
	})
	srv.Handle("net.down", func(_ ipc.Request) ipc.Response {
		return ipc.OK("заглушка: убрал бы мост")
	})
	srv.Handle("net.status", func(_ ipc.Request) ipc.Response {
		return ipc.OK("заглушка: мост (data-plane) ждёт verify")
	})
}

func setModule(st *store.Store, name string, enabled bool) ipc.Response {
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
	if err := st.SetModuleEnabled(name, enabled); err != nil {
		return ipc.Err(err.Error())
	}
	state := "выключен"
	if enabled {
		state = "включён"
	}
	return ipc.OK(fmt.Sprintf("%s %s (применится после рестарта демона)", name, state))
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
		layer = "нет — хеш файла, не дело сетевого файрволла"
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

// moduleList — все зарегистрированные модули с состоянием вкл/выкл, запущен ли и
// здоровьем.
func moduleList(k *kernel.Kernel) []map[string]any {
	run := running(k)
	var out []map[string]any
	for _, name := range kernel.Registered() {
		entry := map[string]any{
			"name":    name,
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
