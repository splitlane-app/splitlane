# SplitLane, этап 3, часть 1: ключи, подписки и WireGuard — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дорожка VPN SplitLane может идти не только через Tailscale, но и через сервер из ссылки (VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC), подписки или файла WireGuard.

**Architecture:** Новый пакет `helper/internal/connector` — модель сервера без знаний о ядре и строгие разборщики ссылок, `.conf` и тела подписки. `coreconfig` получает вариант дорожки `Proxy` и чистые функции, которые строят выход (или конечную точку WireGuard) sing-box из модели. Серверы и подписки хранятся в `servers.json` (только администраторы), в `state.json` — способ и id выбранного. Протокол канала — версия 2 (`connector`, блок `proxy`, методы `proxy.*`, причина `no_server`). Окно делает рабочими плитки «Ключ или подписка» и «Файл настроек», показывает список серверов и раздел «Подключения» по способу.

**Tech Stack:** Go 1.27.4 (стандартная библиотека; новых модулей нет), sing-box 1.14.4 (поля — по `sing-box schema` этой версии), Electron 45, React 19, TypeScript 6, Vitest 5.

**Spec:** `docs/superpowers/specs/2026-11-16-splitlane-stage3-proxy-design.md`.

## Global Constraints

- Ветка `stage3-proxy`. Коммиты — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Новых модулей Go и пакетов npm нет.
- Поля sing-box 1.14.4 (проверены `sing-box schema`/`check`): WireGuard — только конечная точка `endpoints[{type:"wireguard", tag:"vpn", address, private_key, mtu, peers:[{address, port, public_key, pre_shared_key, allowed_ips, persistent_keepalive_interval, reserved}]}]`; VLESS `uuid, flow, packet_encoding`; VMess `uuid, security, alter_id`; Trojan `password`; Shadowsocks `method, password` (методы, включая `2022-blake3-*`); Hysteria2 `password, server_ports, hop_interval, obfs{type:"salamander", password}, up_mbps, down_mbps`; TUIC `uuid, password, congestion_control, udp_relay_mode, zero_rtt_handshake`; TLS `enabled, server_name, insecure, alpn, utls{enabled, fingerprint}, reality{enabled, public_key, short_id}`; транспорты `ws{path, headers, max_early_data, early_data_header_name}`, `grpc{service_name}`, `httpupgrade{host, path, headers}`, `http{host, path}`, `quic{}`. `xhttp`/`splithttp`/`kcp` ядро не знает — отказ.
- Тег выхода дорожки VPN — `vpn`, как сейчас; для способа `proxy` конечной точки Tailscale и службы API Tailscale в конфиге нет.
- Пределы: текст `proxy.add` ≤ 64 КБ; серверов ≤ 500; подписок ≤ 20; ответ подписки ≤ 1 МБ, ≤ 20 с, ≤ 5 переадресаций; файл `.conf` в окне ≤ 64 КБ.
- Секреты (UUID, пароли, ключи, адреса подписок) — только в `servers.json` (атомарно, в папке данных с её списком доступа); не в `state.json`, не в архиве диагностики, не в журналах (маскировка), не в состоянии канала (только id, имя, протокол, источник).
- Канал не принимает путей и сырого JSON ядра.
- Протокол канала — версия 2; окно и служебный процесс одной версии.
- Тексты — `ru.ts`/`en.ts`/`mainText.ts`; глоссарий части 3 плюс: «сервер» — "server", «ключ» — "key", «подписка» — "subscription", «Ключ или подписка» — "Key or subscription", «Файл настроек» — "Configuration file", «Обновить подписку» — "Refresh", «VPN-сервер» — "VPN server".
- Модульные тесты не трогают систему и сеть вне `127.0.0.1`; `pnpm dev`, установщик, `SplitLane.exe` не запускаются; `scripts\test-installer.ps1` на этом ПК — только до отказа.
- Личные данные — нигде. Образцы ссылок в тестах — вымышленные адреса (`example.com`, `192.0.2.x`, `198.51.100.x`) и выдуманные ключи.
- Полная проверка: Go — `go vet ./...`, `go test ./...` (и с `SPLITLANE_CORE=<путь к .cache\core\…\sing-box.exe>` — проверка конфигов настоящим ядром); окно — `pnpm typecheck; pnpm test; pnpm build`; установщик — `BUILD-OK`; CI зелёный.

## Review Focus

1. Ссылки из разных клиентов пишут одно по-разному (vmess:// — base64 JSON v2rayN; ss:// — userinfo в base64 или в процентах для 2022; `pbk`/`sid`/`fp`/`sni`/`type`/`serviceName`; IPv6 в квадратных скобках; имя после `#` в процентах). Ожидание: каждая разбирается в ту же модель; неподдерживаемое — отказ с причиной. Задача 1: таблица образцов.
2. Подписка указывает на внутренний адрес (`http://127.0.0.1`, `https://192.168.1.1`, переадресация на `169.254.169.254`). Ожидание: отказ без запроса к цели. Задача 4.
3. Подписка обновилась и выбранного сервера в ней больше нет, пока деление включено. Ожидание: деление продолжает работать на старом конфиге до перезапуска; выбор снимается при выключенном делении; при включённом — подписка не обновляется, пока выбранный сервер из неё (отказ с причиной). Задача 5.
4. Неверный ключ или пароль. Ожидание: включение заканчивается `tunnel_not_proven` с нейтральным текстом, exit node системного Tailscale возвращается. Задача 5 (на подменах) и живая проверка.
5. Секрет попадает в журнал через ошибку `sing-box check` или в архив диагностики. Ожидание: маскировка. Задача 3.

---

## Как читать этот план

**Что уже есть** — см. спецификацию 1 и карту в отчёте исследования (сохранён в `.superpowers/sdd/<план>/coupling-map.md`): двигатель служебного процесса (`helper/internal/engine`) завязан на Tailscale через `CoreOptions.Hostname/ExitNode`, `NodeAPI`, режим настройки и `NeedLogin`; проверки дорожек, DoH, правила, надзор, согласование с системным Tailscale — общие; прогон без узла уже безопасно пропускает опрос узла (`nodeCall` → `errRunEnded`). Окно: `connect.ts` (`connectStep`, `needsConnect` — сейчас только Tailscale), плитки-заглушки в `ConnectScreen.tsx`.

**Порядок задач.** 1 — модель и разборщики; 2 — конфиг ядра; 3 — хранение и маскировка; 4 — загрузка подписки; 5 — протокол v2 и двигатель; 6 — окно: протокол, команды, модель; 7 — окно: экраны; 8 — набор для сервера на VPS, `slctl`, документация; 9 — живая проверка.

### Где код задач

| Задача | Коммиты |
|---|---|
| 1–8 | заполняется при исполнении |

---

### Task 1: Модель сервера и разборщики

**Files:** `helper/internal/connector/` (новый пакет): `model.go`, `link.go` (vless, vmess, trojan, ss, hysteria2/hy2, tuic), `wireguard.go`, `subscription.go` (тело: base64 или строки), `mask.go`; тесты и fuzz-тесты.

**Interfaces:**
```go
type Server struct { ID, Name, Protocol, Host string; Port int
  TLS *TLS; Transport *Transport
  VLESS *VLESS; VMess *VMess; Trojan *Trojan; Shadowsocks *Shadowsocks; Hysteria2 *Hysteria2; TUIC *TUIC; WireGuard *WireGuard }
func ParseLink(line string) (Server, error)                 // ошибки — *Rejection{Code, Detail}
func ParseWireGuardConf(text []byte, name string) (Server, error)
func ParseText(text string) (servers []Server, rejected []Rejection, subscriptionURL string) // ссылки по строке, .conf, или один https-адрес
func ParseSubscription(body []byte) ([]Server, []Rejection) // base64 (std/url, с паддингом и без) или строки
func ServerID(text string) string                           // стабильный id по нормализованному тексту (sha256, 16 hex)
func Mask(s string) string                                  // UUID, пароли, ключи, токены в адресах → "•••"
```
Коды отказов: `unsupported_protocol`, `unsupported_transport`, `bad_link`, `missing_field`, `bad_wireguard`, `amneziawg`, `plugin`, `too_long`.

**Поведение:** строгая проверка полей; порт 1–65535; имя — из `#…` (процентное декодирование) или `Interface`-имени файла, иначе `host:port`; `.conf`: `[Interface] PrivateKey, Address (через запятую), DNS (игнорировать), MTU`, `[Peer] PublicKey, PresharedKey, Endpoint, AllowedIPs, PersistentKeepalive`; параметры AmneziaWG (`Jc, Jmin, Jmax, S1, S2, H1–H4`) → `amneziawg`.

**Тесты:** таблица образцов реальных форматов (по 3–6 на протокол, в том числе Reality, ws с early data, grpc, httpupgrade, IPv6, имена с кириллицей), отказы по каждому коду, `ParseText` для смеси строк/подписки/`.conf`, `ParseSubscription` для base64 (std, url, без паддинга) и простого списка, `Mask`, `ServerID` стабильный; fuzz `FuzzParseLink`, `FuzzParseWireGuardConf`, `FuzzParseSubscription` (без паники; ограниченное время в CI — по умолчанию fuzz не гоняется, только seed-корпус).

- [ ] Шаги: тесты → провал → реализация → `go vet`, `go test` → коммит `feat(helper): server model and strict parsers for links, WireGuard and subscriptions`.

### Task 2: Конфиг ядра для серверов

**Files:** `helper/internal/coreconfig/config.go`, новый `proxy.go`, `testdata/proxy-*.golden.json`, тесты; `helper/internal/engine/ports.go` (`CoreOptions.Lane`), `helper/internal/singbox/driver.go` (передача).

**Interfaces:** `type Lane struct { Tailscale *TailscaleLane /*Hostname, ExitNode*/; Proxy *connector.Server }`; `CoreOptions.Lane` вместо `Hostname`/`ExitNode`; `func vpnLane(l Lane) (outbounds, endpoints []obj, nodeRule bool, api bool)`.

**Поведение:** конфиг способа Tailscale — байт в байт как сейчас (эталон `vpn.golden.json` не меняется); способ `proxy` — выход (или конечная точка WireGuard) с тегом `vpn`, без конечной точки Tailscale и её службы API; правило 0 — для Tailscale (для WireGuard — после проверки на ПК, задача 9; пока выключено, флаг в одном месте); всё остальное общее.

**Тесты:** эталон на каждый протокол и транспорт (≥ 12 эталонов), `TestCoreAcceptsConfigs` для всех эталонов с настоящим ядром при `SPLITLANE_CORE`, отказ от неизвестного, неизменность эталона Tailscale.

- [ ] Шаги → коммит `feat(helper): core configs for proxy servers and WireGuard`.

### Task 3: Хранение серверов и маскировка

**Files:** `helper/internal/store/store.go` (+ тест): `State.Connector` (`""` = tailscale), `State.SelectedServer`; новый `servers.json` (`LoadServers`, `SaveServers`; формат `{version:1, servers:[{id, name, source, text}], subscriptions:[{id, name, url, updatedAt, count, error}]}`); `helper/internal/engine/diagnostics.go` и `helper/internal/diag` (архив без `servers.json`; маскировка `connector.Mask` в журнале и в `config_invalid`); `helper/internal/singbox/driver.go` (маскировка вывода `sing-box check`).

**Тесты:** чтение старого `state.json` (без полей) → способ tailscale; круг записи `servers.json`; повреждённый `servers.json` — откладывается в `.bad`, как другие файлы; архив без `servers.json`; маскировка в `config_invalid` и в журнале.

- [ ] Шаги → коммит `feat(helper): servers.json, connector in state, secrets masked`.

### Task 4: Загрузка подписки

**Files:** `helper/internal/connector/fetch.go` (+ тест): `type Fetcher interface { Get(ctx, url string) ([]byte, error) }`, настоящий — на `net/http` с собственным `DialContext`, который проверяет каждый адрес назначения после разрешения имени (запрет loopback, link-local, частных, CGNAT `100.64.0.0/10`, multicast, unspecified) и проверкой каждой переадресации (только https, ≤ 5), пределы 1 МБ и 20 с, `User-Agent`.

**Тесты:** локальный TLS-сервер на `127.0.0.1` в тесте (с разрешением loopback только через параметр теста), запрет внутренних адресов (`127.0.0.1`, `10.0.0.1`, `169.254.169.254`, `100.100.100.100`, `[::1]`), переадресация на запрещённый адрес, http → отказ, превышение размера и времени.

- [ ] Шаги → коммит `feat(helper): safe subscription download`.

### Task 5: Протокол v2 и двигатель

**Files:** `helper/internal/protocol/protocol.go` (+ тест): `Version = 2`, поля `Connector`, `Proxy{Servers, Selected, Subscriptions}`, методы `proxy.add/refresh/remove/select`, причина `no_server`, проверка параметров; `helper/internal/engine/*` (команды, предпроверка `no_server`, выбор `Lane` по способу, пропуск всего Tailscale-специфичного для `proxy`, общий `reconnectLocked()` для смены exit node и сервера, `publish`/`syncSaved`, `Deps.Fetch`); `cmd/splitlane-helper/main.go` (настоящий Fetcher); `cmd/slctl` (команды `proxy add|refresh|remove|select|list`).

**Поведение:** раздел 4 спецификации; `proxy.select` при включённом делении — переподключение; удаление выбранного при включённом делении — `wrong_phase`; `proxy.refresh` подписки, из которой выбран сервер, при включённом делении — `wrong_phase`; добавление первого сервера выбирает его; `tailscale.setExitNode` переключает способ на tailscale.

**Тесты:** на подменах (fakeCore, fakeNode, подменный Fetcher): каждый метод, переходы способа, предпроверка `no_server`, включение способом proxy без узла (нет опроса узла, нет режима настройки), переподключение при смене сервера, отказ неверного сервера → `tunnel_not_proven` и возврат системного exit node, публикация состояния без секретов, Review Focus 3.

- [ ] Шаги → коммит(ы) `feat(helper): protocol v2 and the proxy connector in the engine`.

### Task 6: Окно — протокол, команды, модель

**Files:** `app/src/shared/protocol.ts` (v2: `connector`, `proxy`, причина `no_server`, коды отказов разбора), `app/src/shared/ipc.ts` (команды `proxy.*`), `app/src/main/ipc.ts` (проверка: текст ≤ 64 КБ, id), основной процесс: диалог выбора `.conf` (фильтр, ≤ 64 КБ, чтение UTF-8, отправка текста через `proxy.add`), `app/src/renderer/model/connect.ts` (`needsConnect`/`connectStep` по способу), тесты.

- [ ] Шаги → коммит `feat(app): protocol v2, proxy commands, connect model by connector`.

### Task 7: Окно — экраны

**Files:** `ConnectScreen.tsx` (плитки, поле вставки, итог добавления с отклонёнными строками, список серверов — группа переключателей с задержкой применения, как у exit node), `ConnectionScreen` (раздел по способу: серверы, подписки с «Обновить» и временем, «Удалить», «Добавить»), `ConnectionPill.tsx` («<сервер> · <протокол>»), нейтральные тексты причин и плашек, `Lane.tsx` (адрес сервера), уведомления и меню значка, `ru.ts`/`en.ts`/`mainText.ts`, тесты.

- [ ] Шаги → коммит `feat(app): keys, subscriptions and WireGuard files in the window`.

### Task 8: Набор для сервера, документация

**Files:** `scripts/server/` (для владельца VPS: `README.md` на двух языках, `install-singbox-server.sh` — ставит sing-box на Linux VPS из официального выпуска со сверкой SHA-256, генерирует ключи и конфиг сервера VLESS+Reality, Hysteria2 (самоподписанный сертификат с `insecure` или домен), Shadowsocks 2022 и WireGuard, открывает порты, печатает ссылки, файл `.conf` и файл подписки); `README.md`/`README.ru.md` (раздел «Ключи, подписки и WireGuard»), `PRIVACY.md` (подписки: служебный процесс обращается к адресу подписки), спецификация 1 раздел 13/14 (ссылка на этап 3).

- [ ] Шаги → `shellcheck` при наличии (иначе — внимательный разбор), коммит `docs: proxy connectors; a server kit for the live check`.

### Task 9: Живая проверка (нужен владелец и его VPS)

Владелец запускает `install-singbox-server.sh` на своём VPS и передаёт ссылки/файл (вставкой в окно). Под сторожем, для каждого протокола: добавление, включение, проверка дорожек (`curl` через обе дорожки — разные адреса), смена сервера на лету, выключение с возвратом exit node; подписка по адресу; неверный пароль → `tunnel_not_proven` и возврат; правило 0 с WireGuard (проверка, открывает ли конечная точка входящие соединения). Итоги — «Приложение A» спецификации.
