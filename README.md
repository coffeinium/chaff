# chaff ☕

[![Language](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev/) [![Status](https://img.shields.io/badge/status-skeleton-orange)]() [![Mode](https://img.shields.io/badge/deploy-systemd%20VM-blue)]() [![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**chaff** — модульный IOC-файрволл: встаёт **в разрыв** (bump-in-the-wire)
прозрачным L2-мостом на Linux-VM и фильтрует исходящий трафик по индикаторам из
threat-intel фидов — IP, подсети (CIDR), домены и URL. Управление и врезка —
**только CLI**.

> [ LAN ] ──[ chaff · L2-мост ]──[ роутер / uplink ]── Internet

Вышестоящий роутер (DHCP/шлюз/DNS/NAT) не трогаем — врезка прозрачная, режем не
резолв, а сами соединения (SNI/Host + set по IP). Хеши файлов (sha256/md5)
принимаются, но **файрволлом не блокируются** — это не сетевой уровень (экспорт
в EDR/AV).

---

## Принцип: всё — модуль

Микроядро поднимает только включённые модули; любую функцию можно выключить, не
трогая остальные (`chaff module disable <name>`).

| модуль | что делает |
|---|---|
| `bridge` | прозрачный L2-мост в разрыв (фундамент data-plane) |
| `ipblock` | блок по IP/CIDR через nftables set |
| `sniblock` | инлайн-L7: блок по SNI / HTTP Host |
| `dnssnoop` | пассивный DNS-снуп → авто-IP в блоклист |
| `sync` | планировщик подтягивания фидов |
| `apply` | reconcile БД → data-plane |
| `feed-csv` `feed-fstec` `feed-text` `feed-hosts` | адаптеры источников |

---

## Установка

Нужно: Linux, `nftables`, модули ядра `nf_tables` / `nfnetlink_queue` /
`br_netfilter` / `nf_conntrack_bridge`, права `CAP_NET_ADMIN`.

**Из исходников:**

```bash
git clone git@github.com:coffeinium/chaff.git
cd chaff
go build -o chaff ./cmd/chaff
./chaff doctor                      # preflight: права, модули ядра, nft, интерфейсы
```

или напрямую:

```bash
go install github.com/coffeinium/chaff/cmd/chaff@latest
```

**Как сервис (VM, не docker — нужен kernel netfilter/bridge/NFQUEUE):**

```bash
sudo install -m755 chaff /usr/local/bin/chaff
sudo install -d /etc/chaff
sudo install -m644 deploy/chaff.env.example /etc/chaff/chaff.env
sudo install -m644 deploy/chaff.service /etc/systemd/system/chaff.service
sudo systemctl enable --now chaff
```

Настройки демона — `/etc/chaff/chaff.env` (путь к БД, сокету, уровень лога,
номер NFQUEUE). Всё остальное (модули, фиды, индикаторы) — в БД, правится из CLI.

---

## Команды

Демон и диагностика:

```bash
chaff serve                         # демон (обычно из-под systemd)
chaff doctor                        # проверки без демона
chaff tui                           # интерактивный дашборд
```

Врезка в сеть:

```bash
chaff net up --in eth0 --out eth1   # поднять мост между интерфейсами
chaff net down
chaff net status
```

Модули:

```bash
chaff module ls
chaff module enable  NAME
chaff module disable NAME
```

Фиды (адаптеры: `fstec`, `csv`, `text`, `hosts`):

```bash
chaff source add --name fstec --adapter fstec --uri file:///opt/iocs.csv
chaff source add --name ti --adapter csv --uri https://host/list.csv --map indicator:0,type:1,threat:2
chaff source ls
chaff source sync [NAME]            # немедленный pull (иначе по таймеру)
```

Индикаторы:

```bash
chaff list ip|cidr|domain|url|sha256|md5
chaff allow add VALUE               # исключение, allow важнее block
chaff allow rm VALUE
chaff allow ls
chaff apply                         # reconcile БД → data-plane
chaff test VALUE                    # сработает ли и на каком слое
chaff status                        # модули, здоровье, счётчики
```

Пример:

```bash
chaff source add --name fstec --adapter fstec --uri file:///opt/iocs.csv
chaff source sync
chaff test 144.31.191.142     # → ip, L3/L4 (ipblock), block
chaff test github.com         # → domain, L7 (sniblock), allow
chaff status
```

---

## Структура

```
cmd/chaff/            точка входа (serve | клиентские команды)
internal/kernel/      микроядро: реестр, lifecycle, общие сервисы
internal/store/       SQLite — source of truth
internal/model/       Indicator, Kind, Action, Ruleset
internal/ipc/         control-socket (клиент ↔ демон)
internal/feed/        парсинг-помощники адаптеров
internal/modules/     bridge · ipblock · sniblock · dnssnoop · sync · apply · feeds/*
internal/cli/         команды
internal/tui/         интерактивный дашборд (Bubble Tea)
migrations/           нумерованные SQL (embed)
deploy/               systemd-unit, env-пример
```

---

Лицензия — [MIT](LICENSE).
