package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

func cmdDoctor(_ []string) int {
	type check struct{ status, name, detail string }
	var checks []check
	add := func(status, name, detail string) {
		checks = append(checks, check{status, name, detail})
	}

	if os.Geteuid() == 0 {
		add("OK", "права", "запущен под root")
	} else {
		add("WARN", "права", "не root; для data-plane нужен CAP_NET_ADMIN")
	}

	for _, km := range []string{"nf_tables", "nfnetlink_queue", "br_netfilter"} {
		if _, err := os.Stat("/sys/module/" + km); err == nil {
			add("OK", "kmod "+km, "загружен")
		} else {
			add("WARN", "kmod "+km, "не загружен (modprobe "+km+")")
		}
	}

	if p, err := exec.LookPath("nft"); err == nil {
		add("OK", "nft", p)
	} else {
		add("WARN", "nft", "не найден в PATH (поставь nftables)")
	}

	if ifaces, err := net.Interfaces(); err == nil {
		var names []string
		for _, i := range ifaces {
			names = append(names, i.Name)
		}
		add("OK", "интерфейсы", strings.Join(names, ", "))
	} else {
		add("WARN", "интерфейсы", err.Error())
	}

	warn := false
	for _, c := range checks {
		if c.status != "OK" {
			warn = true
		}
		fmt.Printf("[%-4s] %-16s %s\n", c.status, c.name, c.detail)
	}
	if warn {
		return 1
	}
	return 0
}
