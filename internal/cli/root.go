// Пакет cli — поверхность команд. serve поднимает демон; doctor работает сам по
// себе; всё остальное — тонкий клиент, который ходит к демону через сокет.
package cli

import (
	"fmt"
	"os"
)

const usageText = `chaff — модульный IOC-файрволл (в разрыв, bump-in-the-wire)

демон:
  chaff serve                         запустить демон (обычно через systemd)
  chaff doctor                        preflight-проверки (демон не нужен)
  chaff tui                           интерактивный дашборд

врезка:
  chaff net up --in IF --out IF       поднять мост (data-plane)
  chaff net down | net status

модули:
  chaff module ls
  chaff module enable NAME | disable NAME

фиды:
  chaff source add --name N --adapter csv|fstec|text|hosts --uri U [--map indicator:0,type:1]
  chaff source ls | source sync [NAME]

индикаторы:
  chaff list ip|cidr|domain|url|sha256|md5
  chaff allow add VALUE | allow rm VALUE | allow ls
  chaff apply
  chaff test VALUE
  chaff status
`

func Main(args []string) int {
	if len(args) == 0 {
		fmt.Print(usageText)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usageText)
		return 0
	case "serve":
		return cmdServe(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	case "tui":
		return cmdTUI(args[1:])
	default:
		return cmdClient(args)
	}
}

func errln(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "ошибка: "+format+"\n", a...)
	return 1
}
