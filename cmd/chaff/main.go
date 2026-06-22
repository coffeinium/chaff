// Команда chaff — модульный IOC-файрволл, который встаёт в разрыв
// (bump-in-the-wire) на VM в Proxmox и фильтрует egress по индикаторам
// (IP/CIDR, домены, URL) из threat-intel фидов. Управление — только CLI.
package main

import (
	"os"

	"github.com/coffeinium/chaff/internal/cli"

	// Side-effect: каждый модуль регистрирует себя в ядре.
	_ "github.com/coffeinium/chaff/internal/modules"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
