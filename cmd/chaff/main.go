package main

import (
	"os"

	"github.com/coffeinium/chaff/internal/cli"

	_ "github.com/coffeinium/chaff/internal/modules"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
