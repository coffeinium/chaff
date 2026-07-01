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
	type check struct{ status, name, detail, fix string }
	var checks []check
	add := func(status, name, detail, fix string) {
		checks = append(checks, check{status, name, detail, fix})
	}

	fmt.Printf("chaff %s, проверка окружения\n\n", version.Version)

	root := os.Geteuid() == 0
	if root {
		add("OK", "права", "root", "")
	} else {
		add("WARN", "права", "не root; data-plane требует CAP_NET_ADMIN/CAP_NET_RAW", "запускай под root: sudo chaff ...")
	}

	nftOK := false
	if p, err := exec.LookPath("nft"); err == nil {
		nftOK = true
		add("OK", "nft", p, "")
	} else {
		add("ERR", "nft", "не найден в PATH", "поставь пакет nftables")
	}

	kmods := []struct{ name, note, fix string }{
		{"nf_tables", "ядро nftables", "sudo modprobe nf_tables"},
		{"br_netfilter", "фильтрация на мосту", "загрузится при chaff net up"},
		{"nf_conntrack_bridge", "conntrack на мосту", "загрузится при chaff net up"},
		{"nfnetlink_queue", "блок по SNI", "sudo modprobe nfnetlink_queue"},
	}
	for _, km := range kmods {
		if _, err := os.Stat("/sys/module/" + km.name); err == nil {
			add("OK", "kmod "+km.name, "загружен", "")
		} else {
			add("WARN", "kmod "+km.name, "не загружен ("+km.note+")", km.fix)
		}
	}

	if root && nftOK {
		if ok, msg := nftProbe(); ok {
			add("OK", "nft запись", "таблица создаётся и удаляется", "")
		} else {
			add("ERR", "nft запись", "таблицу не создать: "+msg, "нужен CAP_NET_ADMIN и рабочий nf_tables")
		}
	}

	if ok, rel := kernelGE(5, 3); ok {
		add("OK", "ядро", rel, "")
	} else {
		add("WARN", "ядро", rel+" < 5.3; nf_conntrack_bridge может отсутствовать", "обнови ядро")
	}

	if _, err := os.Stat("/proc/sys/net/bridge/bridge-nf-call-iptables"); err == nil {
		add("OK", "bridge-nf", "доступен (br_netfilter активен)", "")
	} else {
		add("WARN", "bridge-nf", "sysctl нет (br_netfilter не загружен)", "sudo modprobe br_netfilter")
	}

	dbDir := filepath.Dir(cfg.DBPath)
	if writableDir(dbDir) {
		add("OK", "каталог БД", dbDir, "")
	} else {
		status := "WARN"
		if root {
			status = "ERR"
		}
		add(status, "каталог БД", dbDir+": нет записи", "sudo install -d -m0755 "+dbDir)
	}
	if !writableDir(filepath.Dir(cfg.SocketPath)) {
		add("WARN", "каталог сокета", filepath.Dir(cfg.SocketPath)+": нет записи", "")
	}

	daemonUp := false
	if _, err := ipc.Call(cfg.SocketPath, ipc.Request{Cmd: "status"}); err == nil {
		daemonUp = true
		add("OK", "демон", "уже запущен на "+cfg.SocketPath, "")
	} else {
		add("OK", "демон", "не запущен (chaff serve)", "")
	}

	add("OK", "веб-панель", webURL(cfg), "")
	if daemonUp {
		add("OK", "веб-порт", cfg.WebAddr+": занят демоном chaff", "")
	} else if portFree(cfg.WebAddr) {
		add("OK", "веб-порт", cfg.WebAddr+": свободен", "")
	} else {
		add("WARN", "веб-порт", cfg.WebAddr+": занят другим процессом", "смени CHAFF_WEB_ADDR или освободи порт")
	}

	if root && nftOK && nftTableChaff() {
		add("OK", "data-plane", "таблица inet chaff уже есть (мост поднят)", "")
	}

	if s, ok := dbSummary(cfg.DBPath); ok {
		add("OK", "БД", s, "")
	}

	if ifaces, err := net.Interfaces(); err == nil {
		def := defaultRouteIface()
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
		add("OK", "интерфейсы", strings.Join(names, ", "), "")
		if def != "" {
			add("OK", "аплинк", def+" (маршрут по умолчанию, не бриджуй как --in)", "")
		}
	}

	hasErr := false
	for _, c := range checks {
		if c.status == "ERR" {
			hasErr = true
		}
		fmt.Printf("[%-4s] %s %s\n", c.status, padName(c.name, 24), c.detail)
	}

	var fixes []string
	for _, c := range checks {
		if c.status != "OK" && c.fix != "" {
			fixes = append(fixes, c.fix)
		}
	}
	if len(fixes) > 0 {
		fmt.Println("\nисправить:")
		for _, f := range fixes {
			fmt.Printf("  · %s\n", f)
		}
	}

	if hasErr {
		return 1
	}
	fmt.Print(`
дальше:
  chaff setup                          токен веб-панели + врезка моста
  chaff net up --in IF --out IF        врезать мост вручную
  chaff status
`)
	return 0
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
	return fmt.Sprintf("индикаторов %d, источников %d, токенов %d", total, len(srcs), len(toks)), true
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
