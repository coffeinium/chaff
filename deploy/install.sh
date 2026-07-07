#!/usr/bin/env bash
set -euo pipefail

REPO="coffeinium/chaff"
BIN="/usr/local/bin/chaff"
ETC="/etc/chaff"
STATE="/var/lib/chaff"
UNIT="/etc/systemd/system/chaff.service"
MODLOAD="/etc/modules-load.d/chaff.conf"
VER="${CHAFF_VERSION:-latest}"
API="https://api.github.com/repos/$REPO"

# Неудачные образцы помечаются суффиксом тега "-failed" (например v0.4.0-failed):
# показываются в списках, но установка запрещена.
is_failed() { case "$1" in *-failed) return 0 ;; esac; return 1; }

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	c_red=$'\033[31m'; c_grn=$'\033[32m'; c_dim=$'\033[2m'; c_bold=$'\033[1m'; c_rst=$'\033[0m'
else
	c_red=; c_grn=; c_dim=; c_bold=; c_rst=
fi

die() { echo "ошибка: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null || die "нужен $1"; }

[ "$(id -u)" = 0 ] || die "нужен root, запускай через sudo"
[ "$(uname -s)" = Linux ] || die "chaff работает только под Linux"

arch() {
	case "$(uname -m)" in
		x86_64 | amd64) echo amd64 ;;
		aarch64 | arm64) echo arm64 ;;
		*) die "неподдерживаемая архитектура: $(uname -m)" ;;
	esac
}

resolve_version() {
	if [ "$VER" = latest ]; then
		VER=$(curl -fsSL "$API/releases/latest" | grep -oP '"tag_name"\s*:\s*"\K[^"]+') \
			|| die "не узнать последнюю версию"
	fi
	[ -n "$VER" ] || die "пустая версия"
}

# list_releases печатает строки "ТЕГ<TAB>КАНАЛ" (канал: stable|experimental|failed),
# свежие сверху. Экспериментальными считаются prerelease-релизы, неудачными —
# теги с суффиксом -failed.
list_releases() {
	local json
	json=$(curl -fsSL "$API/releases?per_page=40") || return 1
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$json" | jq -r '
			.[] | select(.draft | not)
			| [.tag_name, (if (.tag_name | endswith("-failed")) then "failed"
				elif .prerelease then "experimental" else "stable" end)] | @tsv'
		return 0
	fi
	# без jq: вытаскиваем tag_name и prerelease по порядку и склеиваем парами
	printf '%s' "$json" \
		| grep -oP '"(tag_name|prerelease)":\s*"?\K(true|false|[^",}]+)' \
		| paste - - \
		| awk -F'\t' 'NF==2 {print $1"\t"($1 ~ /-failed$/ ? "failed" : ($2=="true"?"experimental":"stable"))}'
}

channel_label() {
	case "$1" in
		experimental) printf '%s' "${c_red}${c_bold}experimental${c_rst}" ;;
		failed)       printf '%s' "${c_red}${c_bold}failed${c_rst}${c_dim} · неудачный образец, не ставится${c_rst}" ;;
		stable)       printf '%s' "${c_grn}stable${c_rst}" ;;
		*)            printf '%s' "$1" ;;
	esac
}

# pick_version выбирает версию: явно заданная (CHAFF_VERSION) — как есть; при живом
# терминале показывает меню релизов с тегом канала; иначе — последняя стабильная.
pick_version() {
	if [ -n "${CHAFF_VERSION:-}" ] && [ "$CHAFF_VERSION" != latest ]; then
		VER="$CHAFF_VERSION"
		return
	fi
	if [ "${CHAFF_VERSION:-}" = latest ] || [ ! -t 1 ] || [ ! -r /dev/tty ]; then
		resolve_version
		return
	fi

	need curl
	local rows=()
	mapfile -t rows < <(list_releases) || true
	if [ "${#rows[@]}" -eq 0 ]; then
		echo "список релизов недоступен, беру latest" >&2
		resolve_version
		return
	fi

	# показываем только самые свежие, старые прячем
	local total=${#rows[@]} max=${CHAFF_MENU_MAX:-8} hidden=0
	if [ "$total" -gt "$max" ]; then
		hidden=$((total - max))
		rows=("${rows[@]:0:max}")
	fi

	echo "доступные версии chaff (новые сверху):"
	local i=1 r tag ch extra latest_marked=""
	for r in "${rows[@]}"; do
		tag=${r%%$'\t'*}
		ch=${r#*$'\t'}
		extra=""
		if [ "$ch" = stable ] && [ -z "$latest_marked" ]; then
			extra="  ${c_dim}(latest)${c_rst}"
			latest_marked=1
		fi
		printf '  %2d) %-16s %s%s\n' "$i" "$tag" "$(channel_label "$ch")" "$extra"
		i=$((i + 1))
	done
	if [ "$hidden" -gt 0 ]; then
		printf '  %s\n' "${c_dim}…ещё $hidden старее скрыто (CHAFF_VERSION=ТЕГ чтобы поставить любую)${c_rst}"
	fi

	local choice=""
	printf 'выбери версию [1-%d, Enter = 1]: ' "${#rows[@]}"
	read -r choice < /dev/tty || choice=""
	[ -z "$choice" ] && choice=1
	case "$choice" in
		*[!0-9]*) die "нужен номер из списка" ;;
	esac
	[ "$choice" -ge 1 ] && [ "$choice" -le "${#rows[@]}" ] || die "номер вне диапазона 1-${#rows[@]}"

	local sel=${rows[choice - 1]}
	VER=${sel%%$'\t'*}
	ch=${sel#*$'\t'}
	if [ "$ch" = failed ]; then
		die "$VER — неудачный образец (failed), установка запрещена"
	fi
	if [ "$ch" = experimental ]; then
		echo "${c_red}${c_bold}внимание:${c_rst} ${c_red}$VER — экспериментальный релиз (prerelease)${c_rst}"
	fi
}

show_versions() {
	need curl
	local rows=() r tag ch extra latest_marked=""
	mapfile -t rows < <(list_releases) || true
	[ "${#rows[@]}" -gt 0 ] || die "список релизов недоступен"
	echo "релизы chaff ($REPO):"
	for r in "${rows[@]}"; do
		tag=${r%%$'\t'*}
		ch=${r#*$'\t'}
		extra=""
		if [ "$ch" = stable ] && [ -z "$latest_marked" ]; then
			extra="  ${c_dim}(latest)${c_rst}"
			latest_marked=1
		fi
		printf '  %-16s %s%s\n' "$tag" "$(channel_label "$ch")" "$extra"
	done
	echo "поставить конкретную: CHAFF_VERSION=ТЕГ ... | sudo bash"
}

fetch_bin() {
	local a url
	is_failed "$VER" && die "$VER — неудачный образец (failed), установка запрещена"
	a=$(arch)
	url="https://github.com/$REPO/releases/download/$VER/chaff-linux-$a"
	echo "качаю $url"
	curl -fSL -o "$1" "$url" || die "не скачать бинарь (есть ли asset chaff-linux-$a в релизе $VER?)"
	chmod 0755 "$1"
}

installed_version() {
	"$BIN" version 2>/dev/null | grep -oP 'chaff \K[0-9][0-9.]*' || echo "?"
}

install_chaff() {
	need curl
	need systemctl
	pick_version
	echo "== chaff $VER ($(arch)) =="

	local TMP
	TMP=$(mktemp)
	fetch_bin "$TMP"
	install -m0755 "$TMP" "$BIN"
	rm -f "$TMP"
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
обновление: curl -fsSL https://raw.githubusercontent.com/$REPO/main/deploy/install.sh | sudo bash -s -- update
удаление:   curl -fsSL https://raw.githubusercontent.com/$REPO/main/deploy/install.sh | sudo bash -s -- uninstall
DONE
}

update_chaff() {
	need curl
	[ -x "$BIN" ] || die "chaff не установлен, сначала install"
	pick_version
	local cur new
	cur=$(installed_version)
	new=${VER#v}
	if [ "$cur" = "$new" ] && [ -z "${CHAFF_FORCE:-}" ]; then
		echo "уже последняя ($cur), CHAFF_FORCE=1 чтобы переустановить"
		return
	fi
	echo "== обновление $cur -> $new ($(arch)) =="
	local TMP
	TMP=$(mktemp)
	fetch_bin "$TMP"
	install -m0755 "$TMP" "$BIN"
	rm -f "$TMP"
	if command -v systemctl >/dev/null; then
		systemctl restart chaff 2>/dev/null || true
	fi
	echo "обновлено: $("$BIN" version)"
}

uninstall_chaff() {
	echo "== удаление chaff (все версии) =="

	local bins=() b
	command -v chaff >/dev/null && bins+=("$(command -v chaff)")
	for b in /usr/local/bin/chaff /usr/bin/chaff /bin/chaff /opt/chaff/chaff "${HOME:-/root}/go/bin/chaff"; do
		[ -e "$b" ] && bins+=("$b")
	done

	local runner=""
	for b in "${bins[@]}"; do [ -x "$b" ] && { runner="$b"; break; }; done
	[ -n "$runner" ] && { "$runner" net down 2>/dev/null && echo "мост снят" || true; }

	if command -v systemctl >/dev/null; then
		local u
		for u in $(systemctl list-unit-files --no-legend 'chaff*.service' 2>/dev/null | awk '{print $1}') chaff.service; do
			systemctl disable --now "$u" 2>/dev/null || true
		done
		rm -f /etc/systemd/system/chaff*.service /run/systemd/system/chaff*.service /usr/lib/systemd/system/chaff*.service
		systemctl daemon-reload 2>/dev/null || true
	fi

	pkill -x chaff 2>/dev/null || true

	command -v nft >/dev/null && nft delete table inet chaff 2>/dev/null || true
	for b in br0 chaff0; do ip link del "$b" 2>/dev/null || true; done

	if [ -f "$ETC/chaff.env" ]; then
		set -a
		. "$ETC/chaff.env"
		set +a
	fi
	rm -f "${CHAFF_SOCKET:-/run/chaff.sock}"
	[ -n "${CHAFF_DB:-}" ] && rm -f "$CHAFF_DB" "$CHAFF_DB-wal" "$CHAFF_DB-shm" || true

	for b in "${bins[@]}"; do rm -f "$b" && echo "снят бинарь: $b" || true; done
	rm -rf "$ETC" "$STATE"
	rm -f "$MODLOAD"

	echo "удалено: бинари, $ETC, $STATE, юниты chaff*, автозагрузка модулей, сокет, таблица nft inet chaff"
	echo "модули br_netfilter/nf_conntrack_bridge оставлены (общие)"
	echo "нестандартное имя моста снять вручную: ip link del ИМЯ"
}

case "${1:-install}" in
	install) install_chaff ;;
	update | upgrade) update_chaff ;;
	uninstall | remove) uninstall_chaff ;;
	versions | list) show_versions ;;
	*) die "использование: install.sh [install|update|uninstall|versions]" ;;
esac
