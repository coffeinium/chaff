// Пакет modules делает blank-import всех пакетов-модулей, чтобы отработал их
// init() и они зарегистрировались в ядре. Импортируй его (ради side-effect) из
// main — ядро потом поднимет только включённые.
package modules

import (
	_ "github.com/coffeinium/chaff/internal/modules/apply"
	_ "github.com/coffeinium/chaff/internal/modules/bridge"
	_ "github.com/coffeinium/chaff/internal/modules/dnssnoop"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/csv"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/fstec"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/hosts"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/text"
	_ "github.com/coffeinium/chaff/internal/modules/feedsync"
	_ "github.com/coffeinium/chaff/internal/modules/ipblock"
	_ "github.com/coffeinium/chaff/internal/modules/sniblock"
)
