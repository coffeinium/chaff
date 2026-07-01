package cli

import (
	"fmt"
	"os"

	"github.com/coffeinium/chaff/internal/version"
)

const usageText = `chaff — модульный IOC-файрволл (в разрыв, bump-in-the-wire)

демон:
  chaff serve                         запустить демон (обычно через systemd)
  chaff doctor                        preflight-проверки (демон не нужен)

врезка:
  chaff net up --in IF --out IF       поднять мост (data-plane)
  chaff net down | net status

модули:
  chaff module ls
  chaff module enable NAME | disable NAME

анализатор соединений (по умолчанию выключен):
  chaff module enable analyzer
  chaff flows [N]                      живые соединения: src(mac/ip) → dst(ip/sni/домен)

веб-панель:
  chaff web token create [--name N] [--ttl 168h]   выпустить токен доступа
  chaff web token ls | token rm ИМЯ|ID
  chaff web cert                       отпечаток TLS-сертификата

фиды:
  chaff source add --name N --adapter csv|text|hosts --uri U [--map indicator:0,type:1]
  chaff source ls | source sync
  chaff source enable NAME | source disable NAME

индикаторы:
  chaff list ip|cidr|domain|url|sha256|md5
  chaff allow add VALUE [--note ПРИЧИНА] | allow rm VALUE | allow ls
  chaff block add VALUE [--note ПРИЧИНА] | block rm VALUE | block ls
  chaff apply
  chaff test VALUE
  chaff status                         код возврата: 0 — мост поднят, 1 — иначе
  chaff hits [N]                       последние срабатывания блокировок

  --json к любой команде — вывод в JSON (для скриптов)
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
	case "-v", "--version", "version":
		fmt.Printf("chaff %s — %s\n", version.Version, version.Author)
		return 0
	case "serve":
		return cmdServe(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	default:
		return cmdClient(args)
	}
}

func errln(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "ошибка: "+format+"\n", a...)
	return 1
}
