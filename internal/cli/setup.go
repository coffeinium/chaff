package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coffeinium/chaff/internal/auth"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/store"
	"github.com/coffeinium/chaff/internal/version"
)

func cmdSetup(_ []string) int {
	cfg := config.Load()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return errln("открыть стор: %v", err)
	}
	defer st.Close()

	fmt.Println(rHdr.Render(fmt.Sprintf("chaff %s: первоначальная настройка", version.Version)))
	fmt.Printf("  %s %s\n", rDim.Render("БД:   "), cfg.DBPath)
	fmt.Printf("  %s %s\n", rDim.Render("сокет:"), cfg.SocketPath)

	if err := setupToken(st); err != nil {
		return errln("%v", err)
	}
	setupBridge(cfg, st)

	fmt.Println("\n" + rHdr.Render("веб-панель: ") + rOK.Render(webURL(cfg)))
	fmt.Println("\n" + rHdr.Render("дальше:"))
	fmt.Println("  " + rOK.Render("chaff status"))
	fmt.Println("  " + rOK.Render("chaff module ls"))
	return 0
}

func setupToken(st *store.Store) error {
	toks, err := st.ListTokens()
	if err != nil {
		return fmt.Errorf("токены: %w", err)
	}
	if len(toks) > 0 {
		fmt.Println("\n" + rDim.Render(fmt.Sprintf("токены уже есть (%d), новый не создаю (chaff web token create)", len(toks))))
		return nil
	}
	plain, hash, err := auth.GenerateToken()
	if err != nil {
		return fmt.Errorf("генерация токена: %w", err)
	}
	if _, err := st.AddToken("setup", hash, time.Now().Unix(), 0); err != nil {
		return fmt.Errorf("сохранить токен: %w", err)
	}
	fmt.Println("\n" + rHdr.Render("токен веб-панели (сохрани, больше не покажется):"))
	fmt.Println("  " + rOK.Render(plain))
	return nil
}

func setupBridge(cfg *config.Config, st *store.Store) {
	ifs := ifaceRows()
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Println("\n" + rHdr.Render("интерфейсы для врезки (--in / --out):"))
		for _, x := range ifs {
			fmt.Printf("  %s\t%s\n", x.name, rDim.Render(x.ips))
		}
		fmt.Println("\nподнять мост: " + rOK.Render("chaff net up --in IF --out IF"))
		return
	}
	defer tty.Close()
	r := bufio.NewReader(tty)

	fmt.Fprintln(tty, "\n"+rHdr.Render("врезка моста в сеть (Enter чтобы пропустить):"))
	fmt.Fprintln(tty, "  "+rWarn.Render("ВНИМАНИЕ: не выбирай интерфейс, через который подключён (SSH/mgmt): мост его заберёт."))
	for i, x := range ifs {
		fmt.Fprintf(tty, "  %s) %-12s %s\n", rOK.Render(strconv.Itoa(i+1)), x.name, rDim.Render(x.ips))
	}
	in := pick(tty, r, ifs, "интерфейс ЛОКАЛКИ (--in): ")
	if in == "" {
		fmt.Fprintln(tty, rDim.Render("пропущено. позже: chaff net up --in IF --out IF"))
		return
	}
	out := pick(tty, r, ifs, "интерфейс РОУТЕРА (--out): ")
	if out == "" || out == in {
		fmt.Fprintln(tty, rDim.Render("отменено (out пуст или совпадает с in)"))
		return
	}
	fmt.Fprintf(tty, "поднять мост %s <-> %s? [y/N]: ", in, out)
	ans, _ := r.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(ans)) != "y" {
		fmt.Fprintln(tty, rDim.Render("отменено"))
		return
	}
	live, err := applyBridge(cfg, st, in, out)
	switch {
	case err != nil:
		fmt.Fprintln(tty, rOff.Render("ошибка: "+err.Error()))
	case live:
		fmt.Fprintln(tty, rOK.Render("мост поднят"))
	default:
		fmt.Fprintln(tty, rDim.Render("сохранено, мост поднимется при старте демона"))
	}
}

func pick(tty *os.File, r *bufio.Reader, ifs []ifrow, prompt string) string {
	fmt.Fprint(tty, prompt)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(ifs) {
		return ifs[n-1].name
	}
	for _, x := range ifs {
		if x.name == line {
			return line
		}
	}
	fmt.Fprintln(tty, rDim.Render("нет такого интерфейса, пропускаю"))
	return ""
}

func applyBridge(cfg *config.Config, st *store.Store, in, out string) (bool, error) {
	resp, err := ipc.Call(cfg.SocketPath, ipc.Request{Cmd: "net.up", Args: map[string]string{"in": in, "out": out}})
	if err == nil {
		if resp.OK {
			return true, nil
		}
		return false, fmt.Errorf("%s", resp.Error)
	}
	blob, _ := json.Marshal(map[string]string{"in": in, "out": out})
	if e := st.SetModuleConfig("bridge", string(blob)); e != nil {
		return false, e
	}
	return false, nil
}

type ifrow struct {
	name string
	ips  string
}

func ifaceRows() []ifrow {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []ifrow
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 {
			continue
		}
		var ips []string
		if addrs, _ := i.Addrs(); addrs != nil {
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok {
					ips = append(ips, ipn.IP.String())
				}
			}
		}
		out = append(out, ifrow{name: i.Name, ips: strings.Join(ips, ", ")})
	}
	return out
}

func webURL(cfg *config.Config) string {
	host, port, err := net.SplitHostPort(cfg.WebAddr)
	if err != nil {
		host, port = cfg.WebAddr, ""
	}
	ip := net.ParseIP(host)
	loop := host == "localhost" || (ip != nil && ip.IsLoopback())

	scheme := "https"
	if cfg.WebInsecure || (loop && cfg.WebTLSCert == "") {
		scheme = "http"
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		if g := firstGlobalIP(); g != "" {
			host = g
		} else {
			host = "<адрес-хоста>"
		}
	}
	if port != "" {
		return scheme + "://" + net.JoinHostPort(host, port)
	}
	return scheme + "://" + host
}

func firstGlobalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if v4 := ipn.IP.To4(); v4 != nil && ipn.IP.IsGlobalUnicast() && !ipn.IP.IsLoopback() {
				return v4.String()
			}
		}
	}
	return ""
}
