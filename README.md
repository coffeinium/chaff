# chaff

[![Language](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev/) [![Status](https://img.shields.io/badge/status-alpha-orange)]() [![Mode](https://img.shields.io/badge/deploy-systemd%20VM-blue)]() [![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**chaff** встаёт в разрыв сети прозрачным мостом и режет исходящий трафик по
чёрным спискам: IP, подсети, домены. Ставится на Linux-VM, управляется из
терминала или через веб-панель (доступ по токену, выпущенному из CLI).

```
[ локальная сеть ] ──[ chaff ]──[ роутер ] ── интернет
```

Роутер не трогаем: он остаётся шлюзом, DHCP и DNS. chaff прозрачно просматривает
проходящий трафик и обрывает соединения к плохим адресам и сайтам.

---

## Установка

Нужно: Linux, `nftables`, права root (`CAP_NET_ADMIN`).

Одной командой (качает бинарь из релиза, ставит сервис, модули ядра и запускает
первоначальную настройку):

```bash
curl -fsSL https://raw.githubusercontent.com/coffeinium/chaff/main/deploy/install.sh | sudo bash
```

Установщик поднимет systemd-сервис, выпишет токен для веб-панели и предложит
врезать мост интерактивным меню (`chaff setup`). Повторный запуск обновляет бинарь,
конфиг и БД остаются.

Если запускать в терминале, установщик и апдейтер покажут меню выбора версии:
свежие релизы сверху (старые скрыты), возле каждой — тег канала, где
`experimental` (prerelease) подсвечен красным. Enter ставит последнюю стабильную.
Без терминала берётся `latest`. Конкретную версию можно задать напрямую —
`CHAFF_VERSION=vX.Y.Z`, а список посмотреть без установки:

```bash
curl -fsSL https://raw.githubusercontent.com/coffeinium/chaff/main/deploy/install.sh | bash -s -- versions
```

Обновление до последнего релиза (меняет только бинарь и перезапускает сервис):

```bash
curl -fsSL https://raw.githubusercontent.com/coffeinium/chaff/main/deploy/install.sh | sudo bash -s -- update
```

Полное удаление (снимает врезку, сервис и все файлы):

```bash
curl -fsSL https://raw.githubusercontent.com/coffeinium/chaff/main/deploy/install.sh | sudo bash -s -- uninstall
```

Из исходников:

```bash
git clone git@github.com:coffeinium/chaff.git
cd chaff
go build -o chaff ./cmd/chaff
sudo ./chaff doctor
sudo ./chaff setup
```

---

## Как пользоваться

Врезка в сеть (между интерфейсом локалки и интерфейсом роутера):

```
chaff net up --in eth0 --out eth1
chaff net down
chaff net status
```

Списки блокировок (источник: файл или ссылка):

```
chaff source add --name badips --adapter text  --uri https://example.com/badips.txt
chaff source add --name ti     --adapter csv    --uri https://host/list.csv --map indicator:0,type:1
chaff source ls
chaff source sync
chaff source indicators ti       # что добавляет источник
chaff source disable badips      # выключить источник и снять его блокировки
chaff source rm badips           # удалить источник вместе с индикаторами
```

Что и как блокируется:

```
chaff list                       # все блокировки, с колонкой вида
chaff list ip|cidr|domain|url|mac
chaff allow add example.com --note "ложное срабатывание"
chaff block add evil.example.com --note "фишинг"
chaff block add aa:bb:cc:dd:ee:ff --note "заражённая машина"
chaff test 203.0.113.10          # сработает ли и где
chaff status
chaff hits                       # последние срабатывания
```

Ручные `allow`/`block` применяются сразу, поверх списков. Вывод таблицей,
`--json` к любой команде даёт JSON для скриптов. `chaff status` возвращает код
`0`, если мост поднят, иначе `1` (удобно для мониторинга).

Функции включаются и выключаются на лету, без перезапуска демона:

```
chaff module ls
chaff module disable dnssnoop
```

### Веб-панель

Модуль `webui`: панель в браузере (Статус, Срабатывания, Соединения, Функции,
Списки, Блокировки), те же действия, что и в CLI.

Панель не заводит учётки сама, токен выпускается только из CLI. Логин обменивает
токен на сессию-куку (httpOnly + SameSite=Strict), сам токен в браузере не оседает.

```
chaff web token create --name laptop --ttl 168h   # выпустить токен, печатается один раз
chaff web token ls
chaff web token rm laptop
chaff web cert                                     # отпечаток TLS-сертификата
chaff module disable webui                         # выключить панель целиком
```

По умолчанию слушает `0.0.0.0:8787`. Раз порт открыт в сеть, TLS обязателен: если
сертификат не задан, демон генерит самоподписанный (кладёт в `/var/lib/chaff/web/`,
отпечаток через `chaff web cert`). Настройки через env: `CHAFF_WEB_ADDR`,
`CHAFF_WEB_TLS_CERT`/`_KEY`, `CHAFF_WEB_INSECURE=1` для plain-HTTP на loopback.

### Анализатор соединений

Модуль `analyzer` (по умолчанию выключен): живой список соединений, кто
(hostname/MAC/IP) куда (IP/SNI/домен) со счётчиками пакетов и байт. Имена машин
пассивно выучивает модуль `namesnoop` из DHCP и mDNS.

```
chaff module enable analyzer
chaff flows
chaff hosts                      # выученные имена машин
```

В веб-панели то же на вкладке «Соединения».

### Групповые политики (ОПАСНО, эксперимент)

Модуль `grouppolicy` (по умолчанию **выключен**). При включении открывается раздел
«Группы» в веб-панели и команды `chaff group ...` в CLI. Группа — это набор машин
(по MAC или имени хоста) со своими правилами (ip/cidr/домен/url) — тот же редактор,
что у глобальных правил, но действуют они только на участников группы, когда группа
включена.

**Глобальные правила всегда приоритетнее групповых**: глобальный allow перекрывает
групповой block, глобальный block действует независимо от групповых исключений.
Одна машина не может состоять больше чем в одной группе. MAC в правилах группы не
принимается — отрезание машины целиком остаётся за глобальным `chaff block add MAC`.

```
chaff module enable grouppolicy
chaff group add дети --note "фильтр для детских устройств"
chaff group add-member дети планшет-маши            # по имени хоста
chaff group add-member дети aa:bb:cc:dd:ee:ff
chaff group block дети youtube.com                  # только для машин группы
chaff group block дети 203.0.113.0/24
chaff group allow дети kids.youtube.com             # исключение внутри группы
chaff group enable дети                             # правила применяются
chaff group scan                                    # кандидаты из сети (имя|mac)
chaff group ls
chaff group disable дети
```

Доменные правила группы применяет sniblock по MAC источника, IP-правила — сам
модуль отдельной цепочкой в сетевом фильтре. Участники по имени хоста «ждут», пока
namesnoop не выучит их MAC из DHCP/mDNS; до этого правила к ним не применяются.
Функционал экспериментальный — интерфейс это подписывает и в CLI, и в панели.

---

## Что внутри

Всё устроено как набор функций, любую можно выключить.

| функция | что делает |
|---|---|
| Врезка в сеть | прозрачный мост между локальной сетью и роутером |
| Блокировка по IP | обрывает соединения к адресам из чёрного списка |
| Блокировка по MAC | отрезает машину от сети по её MAC-адресу |
| Блокировка по сайтам | обрывает по имени сайта (SNI / HTTP Host) |
| Анализ DNS | вычисляет адреса вредоносных доменов из ответов DNS |
| Анализатор соединений | живой список потоков (hostname/MAC/IP к домену/IP) |
| Имена машин | пассивно узнаёт hostname клиентов из DHCP и mDNS |
| Групповые политики | ОПАСНО/эксперимент: блок/разблок машин группами по MAC/имени |
| Обновление списков | периодически тянет источники |
| Источники: CSV / список / hosts | загружают списки из файлов и ссылок |
| Веб-панель | управление через браузер, вход по токену из CLI |

---

## Статус

alpha. Сетевой фильтр (мост, блок по IP и по SNI) проверен под root в
изолированной песочнице на network namespaces и на живом трафике через мост.
На постоянной эксплуатации пока не стоял.

---

Лицензия: [MIT](LICENSE).
