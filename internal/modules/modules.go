package modules

import (
	_ "github.com/coffeinium/chaff/internal/modules/analyzer"
	_ "github.com/coffeinium/chaff/internal/modules/apply"
	_ "github.com/coffeinium/chaff/internal/modules/bridge"
	_ "github.com/coffeinium/chaff/internal/modules/dnssnoop"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/csv"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/hosts"
	_ "github.com/coffeinium/chaff/internal/modules/feeds/text"
	_ "github.com/coffeinium/chaff/internal/modules/feedsync"
	_ "github.com/coffeinium/chaff/internal/modules/ipblock"
	_ "github.com/coffeinium/chaff/internal/modules/sniblock"
	_ "github.com/coffeinium/chaff/internal/modules/webui"
)
