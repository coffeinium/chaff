# chaff ☕

[![Language](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev/) [![Status](https://img.shields.io/badge/status-alpha-orange)]() [![Mode](https://img.shields.io/badge/deploy-systemd%20VM-blue)]() [![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**chaff** встаёт в разрыв сети прозрачным мостом и режет исходящий трафик по
чёрным спискам — IP, подсети, домены. Ставится на Linux-VM, управляется из
терминала или через веб-панель (доступ по токену, выпущенному из CLI).

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
chaff source disable badips      # выключить источник (и снять его блокировки)
```

Что и как блокируется:

```
chaff list ip|cidr|domain|url
chaff allow add example.com --note "ложное срабатывание"   # исключение с причиной
chaff block add evil.example.com --note "фишинг"           # заблокировать вручную
chaff test 203.0.113.10          # сработает ли и где
chaff status
chaff hits                       # последние срабатывания блокировок
```

Ручные `allow`/`block` применяются сразу, поверх списков. Вывод — таблицей;
`--json` к любой команде даёт JSON для скриптов. `chaff status` возвращает код
`0`, если мост поднят, иначе `1` — удобно для мониторинга.

Функции можно включать и выключать на лету, без перезапуска демона:

```
chaff module ls
chaff module disable dnssnoop
```

### Веб-панель

Отдельный модуль `webui` — панель в браузере: Статус · Срабатывания · Функции ·
Списки · Блокировки, те же действия, что и в CLI.

Доступ бутстрапится из терминала: панель не умеет заводить учётки сама, токен
выпускается только из CLI. Логин обменивает токен на сессию-куку
(httpOnly + SameSite=Strict), сам токен в браузере не оседает.

```
chaff web token create --name laptop --ttl 168h   # выпустить токен (печатается один раз)
chaff web token ls
chaff web token rm laptop
chaff web cert                                     # отпечаток TLS-сертификата
chaff module disable webui                         # выключить панель целиком
```

По умолчанию слушает `0.0.0.0:8787`. Раз порт открыт в сеть — TLS обязателен:
если сертификат не задан, демон генерит самоподписанный (кладёт в
`/var/lib/chaff/web/`, отпечаток — `chaff web cert`). Настройки — через env
(`CHAFF_WEB_ADDR`, `CHAFF_WEB_TLS_CERT`/`_KEY`, `CHAFF_WEB_INSECURE=1` для
plain-HTTP на loopback).

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
| Веб-панель | управление через браузер, вход по токену из CLI |

Хеши файлов (sha256/md5) принимаются в списки, но сетью не блокируются — это не
её уровень.

---

## Статус

alpha. Сетевой фильтр (мост, блок по IP) проверен под root в изолированной
песочнице (network namespaces). Блок по сайтам и анализ DNS реализованы, боевая
проверка — на целевой VM.

---

Лицензия — [MIT](LICENSE).
