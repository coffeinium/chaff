package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

func cmdDoctor(_ []string) int {
	type check struct{ status, name, detail, fix string }
	var checks []check
	add := func(status, name, detail, fix string) {
		checks = append(checks, check{status, name, detail, fix})
	}

	if os.Geteuid() == 0 {
		add("OK", "права", "запущен под root", "")
	} else {
		add("WARN", "права", "не root; для data-plane нужен CAP_NET_ADMIN", "запусти под root: sudo chaff ...")
	}

	for _, km := range []string{"nf_tables", "nfnetlink_queue", "br_netfilter"} {
		if _, err := os.Stat("/sys/module/" + km); err == nil {
			add("OK", "kmod "+km, "загружен", "")
		} else {
			add("WARN", "kmod "+km, "не загружен", "sudo modprobe "+km)
		}
	}

	if p, err := exec.LookPath("nft"); err == nil {
		add("OK", "nft", p, "")
	} else {
		add("WARN", "nft", "не найден в PATH", "поставь пакет nftables")
	}

	if ifaces, err := net.Interfaces(); err == nil {
		var names []string
		for _, i := range ifaces {
			names = append(names, i.Name)
		}
		add("OK", "интерфейсы", strings.Join(names, ", "), "")
	} else {
		add("WARN", "интерфейсы", err.Error(), "")
	}

	warn := false
	for _, c := range checks {
		if c.status != "OK" {
			warn = true
		}
		fmt.Printf("[%-4s] %-16s %s\n", c.status, c.name, c.detail)
	}

	if warn {
		fmt.Println("\nисправить:")
		for _, c := range checks {
			if c.status != "OK" && c.fix != "" {
				fmt.Printf("  · %s\n", c.fix)
			}
		}
		return 1
	}

	fmt.Print(`
готово к работе. дальше:
  1. chaff net up --in IF --out IF      врезать мост между локалкой и роутером
  2. chaff source add --name N --adapter text --uri https://...   добавить список
  3. chaff source sync                  загрузить и применить
  4. chaff tui                          дашборд
`)
	return 0
}
