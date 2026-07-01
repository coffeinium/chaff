package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/store"
	"github.com/coffeinium/chaff/internal/version"
)

func cmdDoctor(_ []string) int {
	cfg := config.Load()
	type check struct{ group, status, name, detail, fix string }
	var checks []check
	add := func(group, status, name, detail, fix string) {
		checks = append(checks, check{group, status, name, detail, fix})
	}

	const (
		gAccess = "доступ и инструменты"
		gKernel = "ядро и модули"
		gData   = "data-plane (nftables)"
		gStore  = "хранилище"
		gDaemon = "демон и веб-панель"
		gNet    = "сеть"
	)

	fmt.Println(rHdr.Render(fmt.Sprintf("chaff %s · проверка окружения", version.Version)))

	root := os.Geteuid() == 0
	if root {
		add(gAccess, "OK", "права", "root, есть CAP_NET_ADMIN/CAP_NET_RAW", "")
	} else {
		add(gAccess, "WARN", "права", "не root; data-plane требует CAP_NET_ADMIN/CAP_NET_RAW", "запускай под root: sudo chaff ...")
	}
	nftOK := false
	if p, err := exec.LookPath("nft"); err == nil {
		nftOK = true
		add(gAccess, "OK", "nft", p+" · утилита nftables", "")
	} else {
		add(gAccess, "ERR", "nft", "не найден в PATH", "поставь пакет nftables")
	}

	kmods := []struct{ name, note, fix string }{
		{"nf_tables", "движок nftables", "sudo modprobe nf_tables"},
		{"br_netfilter", "фильтрация трафика на мосту", "загрузится при chaff net up"},
		{"nf_conntrack_bridge", "conntrack на мосту (гейт SNI)", "загрузится при chaff net up"},
		{"nfnetlink_queue", "блок по SNI через NFQUEUE", "sudo modprobe nfnetlink_queue"},
	}
	for _, km := range kmods {
		if _, err := os.Stat("/sys/module/" + km.name); err == nil {
			add(gKernel, "OK", km.name, "загружен · "+km.note, "")
		} else {
			add(gKernel, "WARN", km.name, "не загружен · "+km.note, km.fix)
		}
	}
	if ok, rel := kernelGE(5, 3); ok {
		add(gKernel, "OK", "ядро", rel+" · nf_conntrack_bridge поддерживается", "")
	} else {
		add(gKernel, "WARN", "ядро", rel+" · нужно >= 5.3 для nf_conntrack_bridge", "обнови ядро")
	}
	if _, err := os.Stat("/proc/sys/net/bridge/bridge-nf-call-iptables"); err == nil {
		add(gKernel, "OK", "bridge-nf", "sysctl доступен · br_netfilter активен", "")
	} else {
		add(gKernel, "WARN", "bridge-nf", "sysctl нет · br_netfilter не загружен", "sudo modprobe br_netfilter")
	}

	if root && nftOK {
		if ok, msg := nftProbe(); ok {
			add(gData, "OK", "запись в nft", "таблица создаётся и удаляется", "")
		} else {
			add(gData, "ERR", "запись в nft", "таблицу не создать: "+msg, "нужен CAP_NET_ADMIN и рабочий nf_tables")
		}
		if nftTableChaff() {
			add(gData, "OK", "таблица inet chaff", "поднята · мост активен", "")
		} else {
			add(gData, "OK", "таблица inet chaff", "не создана · мост не поднят (chaff net up)", "")
		}
	} else {
		add(gData, "WARN", "запись в nft", "пропущено · нужен root", "sudo chaff doctor")
	}

	dbDir := filepath.Dir(cfg.DBPath)
	if writableDir(dbDir) {
		add(gStore, "OK", "каталог БД", dbDir+" · запись есть", "")
	} else {
		status := "WARN"
		if root {
			status = "ERR"
		}
		add(gStore, status, "каталог БД", dbDir+" · нет записи", "sudo install -d -m0755 "+dbDir)
	}
	if s, ok := dbSummary(cfg.DBPath); ok {
		add(gStore, "OK", "БД", s, "")
	} else {
		add(gStore, "OK", "БД", "ещё не создана (создастся при первом запуске)", "")
	}
	if !writableDir(filepath.Dir(cfg.SocketPath)) {
		add(gStore, "WARN", "каталог сокета", filepath.Dir(cfg.SocketPath)+" · нет записи", "")
	}

	daemonUp := false
	if _, err := ipc.Call(cfg.SocketPath, ipc.Request{Cmd: "status"}); err == nil {
		daemonUp = true
		add(gDaemon, "OK", "демон", "запущен на "+cfg.SocketPath, "")
	} else {
		add(gDaemon, "OK", "демон", "не запущен · запусти chaff serve (или systemctl start chaff)", "")
	}
	add(gDaemon, "OK", "веб-панель", webURL(cfg), "")
	if daemonUp {
		add(gDaemon, "OK", "веб-порт", cfg.WebAddr+" · занят демоном chaff", "")
	} else if portFree(cfg.WebAddr) {
		add(gDaemon, "OK", "веб-порт", cfg.WebAddr+" · свободен", "")
	} else {
		add(gDaemon, "WARN", "веб-порт", cfg.WebAddr+" · занят другим процессом", "смени CHAFF_WEB_ADDR или освободи порт")
	}

	def := defaultRouteIface()
	if ifaces, err := net.Interfaces(); err == nil {
		var names []string
		for _, i := range ifaces {
			if i.Flags&net.FlagLoopback != 0 {
				continue
			}
			n := i.Name
			if n == def {
				n += "*"
			}
			names = append(names, n)
		}
		add(gNet, "OK", "интерфейсы", strings.Join(names, ", "), "")
	}
	if def != "" {
		add(gNet, "OK", "аплинк", def+" · маршрут по умолчанию, не бриджуй как --in", "")
	}

	groups := []string{gAccess, gKernel, gData, gStore, gDaemon, gNet}
	for _, g := range groups {
		var idx []int
		for i, c := range checks {
			if c.group == g {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		fmt.Println("\n" + rHdr.Render(g))
		for j, i := range idx {
			conn := "├─"
			if j == len(idx)-1 {
				conn = "└─"
			}
			c := checks[i]
			fmt.Printf("%s %s %s %s\n", rDim.Render(conn), glyph(c.status), padName(c.name, 20), c.detail)
		}
	}
	fmt.Println("   " + rDim.Render("* маршрут по умолчанию; для врезки нужны два ethernet-порта (wifi/tun/wireguard не бриджуются)"))

	ok, warn, errc := 0, 0, 0
	var fixes []string
	for _, c := range checks {
		switch c.status {
		case "OK":
			ok++
		case "WARN":
			warn++
		case "ERR":
			errc++
		}
		if c.status != "OK" && c.fix != "" {
			fixes = append(fixes, c.fix)
		}
	}

	fmt.Println("\n" + rDim.Render(strings.Repeat("─", 52)))
	warnB := rDim.Render(fmt.Sprintf("%d !", warn))
	if warn > 0 {
		warnB = rWarn.Render(fmt.Sprintf("%d !", warn))
	}
	errB := rDim.Render(fmt.Sprintf("%d ✗", errc))
	if errc > 0 {
		errB = rOff.Render(fmt.Sprintf("%d ✗", errc))
	}
	fmt.Printf("итог   %s   %s   %s\n", rOK.Render(fmt.Sprintf("%d ✓", ok)), warnB, errB)

	if len(fixes) > 0 {
		fmt.Println("\n" + rWarn.Render("исправить"))
		for _, f := range fixes {
			fmt.Println(rDim.Render("  → ") + f)
		}
	}

	if errc > 0 {
		return 1
	}
	fmt.Println("\n" + rHdr.Render("дальше"))
	fmt.Println(rDim.Render("  → ") + rOK.Render("chaff setup") + "  токен веб-панели + врезка моста")
	fmt.Println(rDim.Render("  → ") + rOK.Render("chaff net up --in IF --out IF") + "  врезать мост вручную")
	fmt.Println(rDim.Render("  → ") + rOK.Render("chaff status"))
	return 0
}

func glyph(s string) string {
	switch s {
	case "ERR":
		return rOff.Render("✗")
	case "WARN":
		return rWarn.Render("!")
	default:
		return rOK.Render("✓")
	}
}

func nftProbe() (bool, string) {
	_ = exec.Command("nft", "delete", "table", "inet", "chaff_doctor").Run()
	if out, err := exec.Command("nft", "add", "table", "inet", "chaff_doctor").CombinedOutput(); err != nil {
		return false, strings.TrimSpace(string(out))
	}
	_ = exec.Command("nft", "delete", "table", "inet", "chaff_doctor").Run()
	return true, ""
}

func nftTableChaff() bool {
	return exec.Command("nft", "list", "table", "inet", "chaff").Run() == nil
}

func kernelGE(major, minor int) (bool, string) {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return true, ""
	}
	rel := strings.TrimSpace(string(b))
	var ma, mi int
	fmt.Sscanf(rel, "%d.%d", &ma, &mi)
	if ma > major || (ma == major && mi >= minor) {
		return true, rel
	}
	return false, rel
}

func portFree(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func defaultRouteIface() string {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	for i, x := range f {
		if x == "dev" && i+1 < len(f) {
			return f[i+1]
		}
	}
	return ""
}

func dbSummary(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	st, err := store.Open(path)
	if err != nil {
		return "не открыть: " + err.Error(), true
	}
	defer st.Close()
	counts, _ := st.CountByKind()
	total := 0
	for _, n := range counts {
		total += n
	}
	srcs, _ := st.ListSources()
	toks, _ := st.ListTokens()
	return fmt.Sprintf("индикаторов %d · источников %d · токенов %d", total, len(srcs), len(toks)), true
}

func padName(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func writableDir(dir string) bool {
	if dir == "" || dir == "." {
		dir = "."
	}
	fi, err := os.Stat(dir)
	if err != nil {
		if parent := filepath.Dir(dir); parent != dir {
			return writableDir(parent)
		}
		return false
	}
	if !fi.IsDir() {
		return false
	}
	tmp := filepath.Join(dir, ".chaff-doctor")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(tmp)
	return true
}
