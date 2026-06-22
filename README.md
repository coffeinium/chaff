# chaff ☕

[![Language](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev/) [![Status](https://img.shields.io/badge/status-alpha-orange)]() [![Mode](https://img.shields.io/badge/deploy-systemd%20VM-blue)]() [![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**chaff** встаёт в разрыв сети прозрачным мостом и режет исходящий трафик по
чёрным спискам — IP, подсети, домены. Ставится на Linux-VM, управляется только
из терминала.

```
[ локальная сеть ] ──[ chaff ]──[ роутер ] ── интернет
```

Роутер не трогаем — он остаётся шлюзом, DHCP и DNS. chaff просто прозрачно
просматривает проходящий трафик и обрывает соединения к плохим адресам и сайтам.

---

## Установка

Нужно: Linux, `nftables`, права root (`CAP_NET_ADMIN`).

```bash
git clone git@github.com:coffeinium/chaff.git
cd chaff
go build -o chaff ./cmd/chaff
sudo ./chaff doctor
```

Как сервис:

```bash
sudo install -m755 chaff /usr/local/bin/chaff
sudo install -d /etc/chaff
sudo install -m644 deploy/chaff.env.example /etc/chaff/chaff.env
sudo install -m644 deploy/chaff.service /etc/systemd/system/chaff.service
sudo systemctl enable --now chaff
```

---

## Как пользоваться

Врезка в сеть (между интерфейсом локалки и интерфейсом роутера):

```
chaff net up --in eth0 --out eth1
chaff net down
chaff net status
```

Списки блокировок (источник — файл или ссылка):

```
chaff source add --name badips --adapter text  --uri https://example.com/badips.txt
chaff source add --name ti     --adapter csv    --uri https://host/list.csv --map indicator:0,type:1
chaff source ls
chaff source sync
```

Что и как блокируется:

```
chaff list ip|cidr|domain|url
chaff allow add example.com      # исключение
chaff test 203.0.113.10          # сработает ли и где
chaff status
```

Функции можно включать и выключать:

```
chaff module ls
chaff module disable dnssnoop
```

Наглядная панель в терминале:

```
chaff tui
```

---

## Что внутри

Всё устроено как набор функций — любую можно выключить.

| функция | что делает |
|---|---|
| Врезка в сеть | прозрачный мост между локальной сетью и роутером |
| Блокировка по IP | обрывает соединения к адресам из чёрного списка |
| Блокировка по сайтам | обрывает по имени сайта (SNI / HTTP Host) |
| Анализ DNS | вычисляет адреса вредоносных доменов из ответов DNS |
| Обновление списков | периодически тянет источники |
| Источники: CSV / список / hosts | загружают списки из файлов и ссылок |

Хеши файлов (sha256/md5) принимаются в списки, но сетью не блокируются — это не
её уровень.

---

## Статус

alpha. Сетевой фильтр (мост, блок по IP) проверен под root в изолированной
песочнице (network namespaces). Блок по сайтам и анализ DNS реализованы, боевая
проверка — на целевой VM.

---

Лицензия — [MIT](LICENSE).
