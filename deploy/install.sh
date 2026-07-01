#!/usr/bin/env bash
set -euo pipefail

REPO="coffeinium/chaff"
BIN="/usr/local/bin/chaff"
ETC="/etc/chaff"
STATE="/var/lib/chaff"
UNIT="/etc/systemd/system/chaff.service"
MODLOAD="/etc/modules-load.d/chaff.conf"
VER="${CHAFF_VERSION:-latest}"

die() { echo "ошибка: $*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "нужен root, запускай через sudo"
[ "$(uname -s)" = Linux ] || die "chaff работает только под Linux"

install_chaff() {
	command -v curl >/dev/null || die "нужен curl"
	command -v systemctl >/dev/null || die "нужен systemd"

	local ARCH
	case "$(uname -m)" in
		x86_64 | amd64) ARCH=amd64 ;;
		aarch64 | arm64) ARCH=arm64 ;;
		*) die "неподдерживаемая архитектура: $(uname -m)" ;;
	esac

	if [ "$VER" = latest ]; then
		VER=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep -oP '"tag_name"\s*:\s*"\K[^"]+') \
			|| die "не удалось узнать последнюю версию"
	fi
	[ -n "$VER" ] || die "пустая версия"
	echo "== chaff $VER ($ARCH) =="

	local URL TMP
	URL="https://github.com/$REPO/releases/download/$VER/chaff-linux-$ARCH"
	TMP=$(mktemp)
	trap 'rm -f "$TMP"' EXIT
	echo "качаю $URL"
	curl -fSL -o "$TMP" "$URL" || die "не удалось скачать бинарь (есть ли asset chaff-linux-$ARCH в релизе $VER?)"
	install -m0755 "$TMP" "$BIN"
	echo "бинарь: $BIN ($("$BIN" version))"

	install -d -m0755 "$ETC"
	if [ ! -f "$ETC/chaff.env" ]; then
		cat >"$ETC/chaff.env" <<'ENV'
CHAFF_DB=/var/lib/chaff/chaff.db
CHAFF_SOCKET=/run/chaff.sock
CHAFF_LOG_LEVEL=info
CHAFF_NFQUEUE_NUM=100
CHAFF_WEB_ADDR=0.0.0.0:8787
ENV
		echo "конфиг: $ETC/chaff.env"
	fi

	cat >"$MODLOAD" <<'MOD'
br_netfilter
nf_conntrack_bridge
MOD
	modprobe br_netfilter 2>/dev/null || true
	modprobe nf_conntrack_bridge 2>/dev/null || true

	cat >"$UNIT" <<'UNITEOF'
[Unit]
Description=chaff modular IOC firewall (bump-in-the-wire)
Documentation=https://github.com/coffeinium/chaff
After=network-pre.target
Wants=network-pre.target

[Service]
Type=simple
ExecStart=/usr/local/bin/chaff serve
EnvironmentFile=-/etc/chaff/chaff.env
Restart=on-failure
RestartSec=2
ExecStopPost=-/usr/sbin/nft delete table inet chaff
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
StateDirectory=chaff

[Install]
WantedBy=multi-user.target
UNITEOF

	systemctl daemon-reload
	systemctl enable --now chaff
	echo "сервис запущен"

	sleep 1
	set -a
	. "$ETC/chaff.env"
	set +a
	"$BIN" setup || true

	cat <<DONE

готово. дальше:
  chaff net up --in IF --out IF
  chaff status
удаление: curl -fsSL https://raw.githubusercontent.com/$REPO/main/deploy/install.sh | sudo bash -s -- uninstall
DONE
}

uninstall_chaff() {
	echo "== удаление chaff =="
	if [ -x "$BIN" ]; then
		"$BIN" net down 2>/dev/null && echo "мост снят" || true
	fi
	if command -v systemctl >/dev/null; then
		systemctl disable --now chaff 2>/dev/null || true
		rm -f "$UNIT"
		systemctl daemon-reload 2>/dev/null || true
	fi
	command -v nft >/dev/null && nft delete table inet chaff 2>/dev/null || true
	for br in br0 chaff0; do ip link del "$br" 2>/dev/null || true; done
	rm -f /run/chaff.sock
	rm -f "$BIN"
	rm -rf "$ETC" "$STATE"
	rm -f "$MODLOAD"
	echo "удалено: бинарь, $ETC, $STATE, юнит, автозагрузка модулей, сокет, таблица nft inet chaff"
	echo "модули br_netfilter/nf_conntrack_bridge оставлены загруженными (общие)"
	echo "нестандартное имя моста снять вручную: ip link del ИМЯ"
}

case "${1:-install}" in
	install) install_chaff ;;
	uninstall | remove) uninstall_chaff ;;
	*) die "использование: install.sh [install|uninstall]" ;;
esac
