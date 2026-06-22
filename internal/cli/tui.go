package cli

import (
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/tui"
)

func cmdTUI(_ []string) int {
	socket := config.Load().SocketPath
	// Заранее проверим демона, чтобы не открывать TUI впустую.
	if _, err := ipc.Call(socket, ipc.Request{Cmd: "status"}); err != nil {
		return errln("%v", err)
	}
	if err := tui.Run(socket); err != nil {
		return errln("%v", err)
	}
	return 0
}
