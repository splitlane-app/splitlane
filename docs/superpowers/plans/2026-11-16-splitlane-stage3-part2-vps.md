# SplitLane, этап 3, часть 2: настройка своего VPS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Человек вводит адрес VPS, логин и пароль (или файл ключа), нажимает «Настроить» — SplitLane сам ставит на VPS сервер VLESS + Reality и Hysteria2 с подпиской, лёгкой защитой и добавляет его; телефон подключается по QR.

**Architecture:** Набор `scripts/server/install-singbox-server.sh` получает машинный режим (`--json`) и флаги выбора протоколов, портов и защиты. Новый пакет `helper/internal/serversetup` — SSH-клиент (`golang.org/x/crypto/ssh`), который проверяет систему, загружает вшитый набор, запускает его и ждёт подписку; его вызывает подкоманда `splitlane-helper.exe server-setup`, которую окно запускает без повышения и кормит учётными данными через stdin. Окно добавляет итог существующими `proxy.add`/`proxy.select` (протокол канала 3 даёт id серверов в ответе и метод `proxy.reveal` для QR).

**Tech Stack:** Go 1.27.4 + `golang.org/x/crypto` v0.58.0 (единственный новый модуль Go), bash (набор), Electron 45, React 19, TypeScript 6, Vitest 5, `qrcode-generator` 2.0.4 (единственный новый пакет npm), Docker (только локальная сквозная проверка).

**Spec:** `docs/superpowers/specs/2026-11-16-splitlane-server-setup-design.md` («спецификация»); основа — `docs/superpowers/specs/2026-11-16-splitlane-stage3-proxy-design.md` («спецификация 3.1»).

## Global Constraints

- Ветка `stage3-vps` от `main`. Коммиты — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Новые зависимости — только `golang.org/x/crypto v0.58.0` (helper) и `qrcode-generator 2.0.4` (окно, `devDependencies`, как остальные пакеты окна).
- Учётные данные (пароль, текст ключа, пароль ключа): только память формы окна, основного процесса окна и подкоманды; окно → подкоманда только через stdin; никогда — в аргументах процесса, на диске, в журналах, архивах, служебном процессе SYSTEM, текстах ошибок. Подкоманда после использования затирает свои байтовые копии.
- Файл ключа в окне ≤ 16 КБ (как `.conf` — чтение из дескриптора, путь в интерфейс не уходит).
- Сроки: подключение SSH — 15 с; вся настройка — 10 мин (`timeout`); ожидание подписки — 90 с, опрос раз в 5 с; после закрытия stdin окно ждёт выхода подкоманды 5 с, затем завершает её.
- Команда набора от SplitLane: `--json --default-protocols vless,hy2 --auto-ports --harden --subscription`.
- Списки портов `--auto-ports` (дословно из спецификации, раздел 4): VLESS (tcp) 443, 2053, 2083, 2087, 2096; Hysteria2 (udp) 443, 4443, 8443, 10443; подписка (tcp) 8443, 9443, 7443; Shadowsocks (tcp и udp) 8388, 8389, 9388; WireGuard (udp) 51820, 51821, 51822. Порт 80 не переносится.
- Протокол канала — версия 3 (`PROTOCOL_VERSION`/`protocol.Version`); окно и служебный процесс одной версии.
- «Настроить» доступна только в фазах `off` и `error`.
- Коды ошибок подкоманды (спецификация, раздел 8): `bad_input`, `ssh_unreachable`, `ssh_auth`, `ssh_key`, `password_expired`, `no_admin`, `os_unsupported`, `download_failed`, `ports_busy`, `kit_failed`, `timeout`, `cancelled`, `connection_lost`; предупреждения: `harden_failed`, `sub_unreachable`, `sub_port80_busy`.
- Тексты — `ru.ts`/`en.ts`; глоссарий части 3 плюс: «Свой VPS» — "Your own VPS", «Настроить» — "Set up", «Подключить телефон» — "Connect a phone", «Отменить» — "Cancel".
- Модульные тесты не трогают систему и сеть вне `127.0.0.1`; Docker — только в задаче 4 и не во время сетевых проверок; `pnpm dev`, установщик, `SplitLane.exe` не запускаются; `scripts\test-installer.ps1` на этом ПК не запускать.
- Личные данные — нигде. В тестах — `192.0.2.x`, `198.51.100.x`, `example.com`, выдуманные ключи и пароли.
- Полная проверка: Go — `gofmt -l .` пусто, `go vet ./...`, `go test ./...`; набор — shellcheck (Docker `koalaman/shellcheck:stable`, по умолчанию и `-o all -e SC2250,SC2310,SC2312`, оба с кодом 0); окно — `pnpm typecheck; pnpm test; pnpm build`; установщик — `scripts\build-installer.ps1` → `BUILD-OK`; CI зелёный.

## Review Focus

1. Пароль со спецсимволами (`'`, `"`, `$`, `\`, пробел, `!`, кириллица) — для SSH и для sudo передаётся байт в байт и нигде не проходит через интерпретацию оболочки. Задача 2: тест с паролем `p'a"s$ s\w!рд` и для входа, и для `sudo -S`.
2. sudo без пароля (`NOPASSWD`), с «лекцией» при первом вызове, с `requiretty`. Ожидание: при `NOPASSWD` пароль не отправляется вовсе (`sudo -n`), лекция и подсказки не ломают разбор, `requiretty` — понятный `no_admin`. Задача 2: тесты на поддельном сервере для всех трёх.
3. Адрес в разных формах: `198.51.100.7`, `198.51.100.7:2222`, `[2001:db8::7]:22`, `2001:db8::7`, `vps.example.com`, с пробелами по краям; мусор (`http://…`, `a b`, порт 0 или 70000) — `bad_input`. Задача 2 (`ParseRequest`) и задача 7 (проверка формы).
4. Посторонние строки в выводе набора (шум apt, предупреждения dpkg, пустые строки, обрезанный JSON). Ожидание: подкоманда пропускает не-JSON строки и строки неизвестного вида; итогом считает только строку `result` или `error`; если набор завершился без итога — `kit_failed`. Задача 2.
5. Обрыв SSH посреди установки и повторный запуск. Ожидание: `connection_lost` с подсказкой; повторная настройка на том же сервере доходит до конца с теми же ключами, окно выбирает первый сервер, хотя все уже были в списке. Задача 2 (обрыв на поддельном сервере), задача 4 (повторный запуск на контейнере), задача 6 (выбор по полю `servers`).

---

## Как читать этот план

**Порядок задач.** 1 — набор: `--json`, протоколы, порты, защита; 2 — пакет `serversetup` (SSH и сценарий настройки); 3 — подкоманда `server-setup`; 4 — сквозная проверка в Docker; 5 — протокол канала 3 (`servers`/`subscription` в ответе `proxy.add`, `proxy.reveal`); 6 — основной процесс окна: запуск настройки; 7 — экран «Свой VPS» и документация; 8 — «Подключить телефон» (QR); 9 — живая проверка.

### Где код задач

| Задача | Коммиты |
|---|---|
| 1–8 | заполняется при исполнении |

---

### Task 1: Набор — `--json`, выбор протоколов, автоматические порты, защита

**Files:** `scripts/server/install-singbox-server.sh`, `scripts/server/README.md`, `scripts/server/README.ru.md`.

**Interfaces (что видят задачи 2 и 4):**
- Флаги: `--json`, `--protocols <список>`, `--default-protocols <список>` (список — из `vless`, `hy2`, `ss`, `wg` через запятую, без повторов), `--auto-ports`, `--harden`; прежние флаги без изменений. `--uninstall` принимает из новых только `--json`.
- Строки stdout в `--json` (каждая — один объект JSON, без переносов внутри):
  - шаг: `{"step":"system|singbox|config|service|harden|subscription","state":"start|done|skip|warn","detail":"<текст без секретов>"}`;
  - итог: `{"result":{"links":["<ссылка>",…],"wireguard":"<текст .conf>"|null,"subscription":"https://…"|null,"ports":{"vless":N,"hy2":N,"ss":N,"wg":N,"sub":N},"warnings":["harden_failed"|"sub_port80_busy"|"sub_skipped",…]}}` — в `ports` только установленные протоколы и подписка;
  - ошибка: `{"error":{"code":"os_unsupported|download_failed|ports_busy|kit_failed","message":"<текст>"}}`, код выхода 1.
  - Всё человекочитаемое в `--json` — на stderr.
- Состояние: новые ключи `PROTOCOLS` (канонический порядок `vless,hy2,ss,wg`) и запомненные порты (уже есть из 8a3a1c1).

**Поведение:**
- Протоколы: `--protocols` > запомненный `PROTOCOLS` > «все четыре», если файл состояния уже есть (установка до этой части) > `--default-protocols` > «все четыре». Конфиг ядра, ссылки, `.conf`, порты брандмауэра, проверка портов, вывод — только для выбранных. `--reset` протоколы не трогает. Убранный протокол исчезает из конфига при следующем запуске.
- `--auto-ports`: для каждого выбранного протокола и для подписки, у которых порт не задан флагом и не запомнен, — первый порт списка (Global Constraints), свободный по `ss` (не слушает никто, кроме процесса сервера набора — как `check_ports`), не совпадающий с уже выбранными портами того же вида (tcp/udp; у Shadowsocks — и tcp, и udp). Выбранный запоминается. Весь список занят — ошибка `ports_busy`. Без `--auto-ports` — прежнее поведение (порт по умолчанию, занят — ошибка).
- Порт подписки занимается и проверяется и по tcp, и по udp: Caddy слушает его ещё и для HTTP/3 (Ruling 5); брандмауэр — только tcp. Порт 80 при `--subscription`: слушает процесс не `caddy` — подписка пропускается, предупреждение `sub_port80_busy` (в `--json`) или `warning:` (текст). Неудача Caddy, как сейчас, — подписка пропущена, предупреждение `sub_skipped`.
- `--harden`: `apt-get install -y fail2ban python3-systemd unattended-upgrades` (с `apt_get`, umask 022); файл `/etc/fail2ban/jail.d/splitlane-kit.local` (`[sshd]`, `enabled = true`, `backend = systemd`, режим 644); `systemctl enable --now fail2ban` и перезагрузка конфигурации; `/etc/apt/apt.conf.d/20auto-upgrades` с `APT::Periodic::Update-Package-Lists "1";` и `APT::Periodic::Unattended-Upgrade "1";` (если файла нет; существующий не трогается). В файле тюрьмы — `ignoreip = 127.0.0.1/8 ::1 <адреса>`: источники SSH-соединений, установленных к портам sshd во время запуска (по `ss`), — чтобы fail2ban, перечитав журнал за `findtime`, не забанил сам сеанс настройки (Ruling 6); повторный запуск переписывает список. Неудача любого шага — предупреждение `harden_failed`, работа продолжается. Повторный запуск ничего не дублирует. `--dry-run`: файлы под `ROOT`, без apt и systemctl. `--uninstall` оставляет файл тюрьмы, fail2ban и обновления (Ruling 4: без файла в Debian 12 fail2ban перестаёт запускаться) и пишет об этом. `apt_get` ждёт блокировку dpkg до 180 с (Ruling 7).
- Коды ошибок: `die_code <code> <текст>`; неподдерживаемая система, архитектура, нет systemd — `os_unsupported`; не скачался sing-box или пакет — `download_failed`; занятые порты (`check_ports`, `--auto-ports`) — `ports_busy`; всё прочее (`die`) — `kit_failed`. В текстовом режиме вывод прежний.
- JSON-строки собираются функцией экранирования (`\`, `"`, управляющие символы как `\n`, `\t`, `\u00XX`); ссылки, `.conf` и адрес подписки появляются только в строке `result`.
- Справка и оба README: новые флаги; в README — что команда SplitLane «Свой VPS» запускает набор с ними, что fail2ban по умолчанию блокирует адрес после 5 неверных попыток за 10 минут и как снять блокировку (`sudo fail2ban-client unban <адрес>`).

**Тесты (Docker `debian:12`, сценарий в рабочей папке, не в git):** shellcheck (оба режима); `--dry-run`: (а) `--default-protocols vless,hy2` без состояния — в конфиге два входа, в итоге две ссылки, `wireguard: null`; (б) состояние без `PROTOCOLS` + `--default-protocols vless,hy2` — все четыре; (в) `--protocols vless` поверх — один, запомнен; (г) `--auto-ports` с занятыми `nc -l 443` (tcp) и `nc -lu 443` (udp) — VLESS 2053, Hysteria2 4443, запомнены, повторный запуск их сохраняет; (д) все порты VLESS заняты — `{"error":{"code":"ports_busy",…}}` и код 1; (е) `--json`: каждая строка stdout разбирается `python3 -c 'import json,sys;[json.loads(l) for l in sys.stdin]'`, ровно одна строка `result`; (ж) пароль Shadowsocks с `+/=` и имя с `#` в ссылке — JSON корректен. Контейнер `ubuntu:24.04` с systemd: `--harden` (fail2ban активен, `fail2ban-client status sshd` отвечает, повторный запуск — один файл тюрьмы, `--uninstall` удаляет только его).

- [ ] Шаги: тесты-сценарии → провал на текущем наборе → реализация → shellcheck и сценарии зелёные → коммит `feat(server-kit): JSON output, protocol choice, automatic ports, light hardening`.

### Task 2: Пакет `serversetup` — SSH и сценарий настройки

**Files:** `helper/internal/serversetup/` (новый): `request.go` (вход и проверка), `run.go` (сценарий), `ssh.go` (подключение, команды, загрузка файла), `kitlines.go` (разбор строк набора), `kit.go` (`//go:embed kit/install-singbox-server.sh`), `kit/install-singbox-server.sh` (копия набора); тесты `request_test.go`, `run_test.go`, `kitlines_test.go`, `kit_test.go`, `fakessh_test.go` (поддельный SSH-сервер). `helper/go.mod`, `helper/go.sum` (`golang.org/x/crypto v0.58.0`).

**Interfaces:**
```go
package serversetup

type Request struct {
	Host string; Port int; User string
	Password   []byte // или
	Key        []byte // текст ключа OpenSSH/PEM
	Passphrase []byte
}
// ParseRequest читает одну строку JSON stdin (поля host, port, user, password | key, passphrase):
// host — IPv4, IPv6 (в [] или без) или имя (RFC 1123), с ":порт" или без; port 1..65535 (по умолчанию 22);
// user — 1..32 символа [a-z_][a-z0-9_-]*; ровно один из password/key. Иначе Failure{Code:"bad_input"}.
func ParseRequest(line []byte) (Request, *Failure)
func (r *Request) Wipe() // затирает Password, Key, Passphrase

type Event struct{ Step, State, Detail string } // json: step, state, detail; Step: connect|system|install|harden|subscription
type Done struct {
	Subscription *string `json:"subscription"`
	Links        string  `json:"links"` // ссылки по строке
	Warnings     []string `json:"warnings"`
}
type Failure struct{ Code, Detail string } // json: code, detail; Detail уже замаскирован connector.Mask

type Deps struct {
	Dial           func(ctx context.Context, network, addr string) (net.Conn, error)
	Fetch          connector.Fetcher
	ConnectTimeout time.Duration // 15 s
	Total          time.Duration // 10 min
	SubWait        time.Duration // 90 s
	SubEvery       time.Duration // 5 s
}
func DefaultDeps(version string) Deps
func Run(ctx context.Context, req Request, d Deps, emit func(Event)) (Done, *Failure)
var Kit []byte // вшитый набор
```

**Поведение `Run`:**
1. `connect`: TCP за `ConnectTimeout` (`ssh_unreachable` при отказе, тайм-ауте, ошибке имени); SSH-клиент с `HostKeyCallback: ssh.InsecureIgnoreHostKey()` (спецификация, раздел 3 — комментарий в коде об этом решении); методы: пароль — `ssh.Password` и `ssh.KeyboardInteractive` (на каждый вопрос с `echo=false` — пароль; вопрос с `echo=true` или больше трёх раундов — `ssh_auth`); ключ — `ssh.ParsePrivateKey` / `ssh.ParsePrivateKeyWithPassphrase` (`.ppk` по заголовку `PuTTY-User-Key-File` — `ssh_key` с `detail:"ppk"`; неверный пароль ключа или неизвестный формат — `ssh_key`). Отказ сервера в аутентификации — `ssh_auth`.
2. `system`: команды (каждая — отдельная сессия, вывод ≤ 64 КБ): `cat /etc/os-release` (ID `debian` ≥ 12 или `ubuntu` ≥ 22.04), `uname -m` (`x86_64`/`aarch64`), `test -d /run/systemd/system` — иначе `os_unsupported` с `detail` (что не так); `id -u` — `0` → root; иначе `sudo -n true` (успех → sudo без пароля), иначе `sudo -S -p '' -v` с паролем на stdin (только при входе по паролю; при входе по ключу и без `NOPASSWD` — `no_admin`); отказ, `sudo: a terminal is required`/`requiretty`, нет `sudo` — `no_admin`.
3. `install`: `mktemp /tmp/splitlane-kit.XXXXXX` → `cat > <файл>` с `Kit` на stdin → `chmod 600`; запуск: root — `bash <файл> <флаги>`; sudo без пароля — `sudo -n bash <файл> <флаги>`; sudo с паролем — `sudo -S -p '' bash <файл> <флаги>`, пароль и `\n` первыми байтами stdin, затем stdin закрывается. Флаги — Global Constraints. stdout читается построчно (`kitlines.go`): шаги `singbox|config|service` → `install`, `harden` → `harden`, `subscription` → `subscription` (state/detail как есть, `detail` через `connector.Mask`); строка `result` → итог; `error` → `Failure{Code: тот же (os_unsupported|download_failed|ports_busy, иначе kit_failed), Detail: Mask(message)}`; не-JSON и неизвестные строки пропускаются; конец без итога — `kit_failed` (detail — последние 2 строки stderr через `Mask`). В конце (и при ошибке) — `rm -f <файл>` отдельной сессией (ошибку удаления игнорировать).
4. `subscription`: если итог дал адрес — опрос `Fetch.Get` раз в `SubEvery` до `SubWait`; успех — тело даёт хотя бы один сервер по `connector.ParseSubscription`; не дождались — `Subscription=nil`, предупреждение `sub_unreachable`. Адреса нет — шаг `skip`.
5. Весь `Run` — под `context.WithTimeout(Total)`: истёк — `timeout`; отмена `ctx` — `cancelled`; обрыв SSH-соединения после `connect` — `connection_lost`. `Links` — `strings.Join(result.links, "\n")`. Секреты (пароль, ключ, ссылки, адрес) не попадают ни в `Event.Detail`, ни в `Failure.Detail`.

**Тесты:** `request_test.go` — таблица адресов из Review Focus 3, пользователей, обоих видов учётных данных, ошибок `bad_input`; `Wipe`. `fakessh_test.go` — сервер `x/crypto/ssh` на `127.0.0.1:0` с записанным сценарием: ответы на команды по префиксу, проверка, что пароль пришёл в stdin `sudo -S` байт в байт и никогда — в строке команды; `run_test.go` — вход по паролю (с паролем из Review Focus 1), keyboard-interactive, ключ ed25519 с паролем и без, `.ppk`, неверный пароль → `ssh_auth`, недоступный порт → `ssh_unreachable`, Debian 11 / CentOS / `armv7l` / без systemd → `os_unsupported`, root / sudo с паролем / `NOPASSWD` (пароль не отправлен) / `requiretty` / неверный пароль sudo → `no_admin`, итог набора с подпиской и без, ошибка набора каждого кода, шум и обрезанный JSON (Review Focus 4), обрыв посреди вывода → `connection_lost` (Review Focus 5), отмена `ctx` → `cancelled`, `Total` → `timeout`, подписка ответила со 2-й попытки / не ответила → `sub_unreachable`, временный файл удаляется и при ошибке; во всех случаях пароль не встречается в событиях и `Failure`. `kit_test.go` — `Kit` побайтно равен `../../../scripts/server/install-singbox-server.sh` (дрейф копии ловит `go test`).

- [ ] Шаги: тесты → провал → реализация → `gofmt -l .`, `go vet ./...`, `go test ./...` → коммит `feat(helper): server setup over SSH with the embedded server kit`.

### Task 3: Подкоманда `server-setup`

**Files:** `helper/cmd/splitlane-helper/main.go` (разбор подкоманды до `parse`, не в `adminCommands`), новый `helper/cmd/splitlane-helper/serversetup.go`, `helper/cmd/splitlane-helper/main_test.go`; `scripts/build-helper.ps1` не меняется (копия набора уже в пакете).

**Interfaces:**
```go
// serverSetup runs `server-setup`: one JSON line from stdin (serversetup.ParseRequest), events and the
// final line to stdout, exit code 0 (done) or 1 (failed). Closing stdin cancels.
func serverSetup(stdin io.Reader, stdout io.Writer, run func(context.Context, serversetup.Request, func(serversetup.Event)) (serversetup.Done, *serversetup.Failure)) int
```
Строки stdout (по одной на событие, `json.Encoder`): `{"step":…,"state":…,"detail":…}`; в конце `{"done":{…Done…}}` или `{"failed":{"code":…,"detail":…}}`.

**Поведение:** `run(["server-setup"])` — до любых флагов, без папки данных, журнала и канала; работает без прав администратора; читает одну строку stdin (≤ 64 КБ; длиннее или пусто — `failed: bad_input`); затем горутина читает stdin до EOF и отменяет контекст (`cancelled`); `request.Wipe()` после `Run`; ничего не пишет на stderr, кроме паники рантайма. Версия для `DefaultDeps` — `main.version`.

**Тесты:** подкоманда с подменённым `run`: строка запроса → события и `done` в stdout, код 0; `failed` → код 1; пустой stdin и мусор → `bad_input`; закрытие stdin посреди работы → `cancelled`; в выводе нет пароля; `server-setup` не требует `--dev` и не трогает папку данных (временный `ProgramData` в тесте пуст после запуска).

- [ ] Шаги: тесты → провал → реализация → `go vet`, `go test` → коммит `feat(helper): the server-setup subcommand for the window`.

### Task 4: Сквозная проверка в Docker

**Files:** `scripts/server/e2e/Dockerfile` (образ с systemd и sshd: аргумент `BASE` — `debian:12`, `ubuntu:22.04`, `ubuntu:24.04`; пользователь `kit` с `sudo` по паролю; `root` с паролем; вход по ключу для `kit`), `scripts/server/e2e/run-e2e.ps1` (собирает `splitlane-helper.exe` во временную папку, поднимает контейнеры с `--privileged --cgroupns=host -p 127.0.0.1:<порт>:22`, запускает `server-setup` с запросом через stdin, проверяет итог, останавливает и удаляет контейнеры и образы), `helper/internal/serversetup/e2e_test.go` (`//go:build e2e`; цели из переменной `SPLITLANE_E2E` — JSON-список `{host,port,user,password|keyfile}`); раздел «Сквозная проверка» в `scripts/server/README.md`/`README.ru.md` (как запустить, только локально).

**Поведение и проверки (каждый образ):** от `root` по паролю; от `kit` через `sudo` с паролем; от `kit` по ключу (ожидание: `no_admin`, sudo требует пароль); с занятыми в контейнере 443/tcp и 443/udp (`nc`) — порты 2053 и 4443 в итоге; повторный запуск — те же ссылки; контейнер с набором, поставленным заранее без новых флагов (все четыре протокола) — после `server-setup` в `/etc/splitlane-kit/state` `PROTOCOLS` не появился, в итоге четыре вида ссылок; `--harden`: `fail2ban-client status sshd` в контейнере; ожидание подписки — `sub_unreachable` (контейнер без внешнего 80-го порта), ссылки есть. Сертификат Let's Encrypt здесь не проверяется (живая проверка).

- [ ] Шаги: Dockerfile и сценарий → прогон на трёх образах (Docker Desktop; не во время сетевых проверок) → итоги в отчёт задачи → коммит `test(server-kit): end-to-end server setup on Debian and Ubuntu containers`.

### Task 5: Протокол канала 3 — `servers`/`subscription` в ответе, `proxy.reveal`

**Files:** `helper/internal/protocol/protocol.go` (`Version = 3`; `ProxyAddResult.Servers []string`, `ProxyAddResult.Subscription *string`; `MethodProxyReveal = "proxy.reveal"`, `ProxyRevealParams{ID}`, `ProxyRevealResult{Address}`), `helper/internal/engine/proxy.go` (заполнение полей в `proxy.add`/`proxy.refresh`, `cmdProxyReveal`), `helper/internal/engine/*_test.go`, `helper/cmd/slctl` (команда `proxy-reveal <id>`, вывод адреса — только для разработки); окно: `app/src/shared/protocol.ts` (`PROTOCOL_VERSION = 3`, разбор новых полей и ответа `proxy.reveal`), `app/src/shared/ipc.ts` (`proxyReveal(id): Promise<{address: string} | {error: HelperError}>`, канал IPC), `app/src/main/ipc.ts`, `app/src/preload/*`, тесты.

**Поведение:** `servers` — id всех серверов текста или подписки в их порядке, и новых, и уже бывших (`existing`), не больше 500; `subscription` — id подписки (`proxy.add` с адресом или `proxy.refresh`), иначе `null`. `proxy.reveal {id}`: id подписки (`^sub-[0-9a-f]{16}$`) — `{address}`; id сервера — `bad_request`; нет такой — `unknown_server`; любая фаза; в журнал — `proxy.reveal <id>` без адреса. Остальные методы без изменений.

**Тесты:** Go — поля `servers` и `subscription` при новых, частично бывших и полностью бывших серверах, у ссылок (без подписки) и у подписки, при `proxy.refresh`; `proxy.reveal` — подписка, сервер, неизвестный id, адреса нет в журнале и в архиве диагностики (расширить существующий тест «нет секретов»); окно — `parseProxyAddResult` требует `servers` и `subscription` (окно и служба всегда одной версии; ответ без них — ошибка разбора, как у других полей), разбор ответа `proxy.reveal`, `proxyReveal` в main и preload.

- [ ] Шаги: тесты → провал → реализация → Go и окно (полная проверка) → коммит `feat: protocol 3 — server ids in add results and proxy.reveal`.

### Task 6: Основной процесс окна — запуск настройки

**Files:** новые `app/src/shared/vps.ts` (+ `vps.test.ts`), `app/src/main/vpsSetup.ts` (+ `vpsSetup.test.ts`); `app/src/main/index.ts` (обработчики IPC, отмена при «Выход»), `app/src/shared/ipc.ts`, `app/src/preload/*`.

**Interfaces:**
```ts
// shared/vps.ts — общие правила адреса и логина для формы (задача 7) и основного процесса; те же, что ParseRequest (задача 2)
export function parseVpsAddress(s: string): { host: string; port: number } | null // IPv4, IPv6 ([..]:порт или без), имя; порт 1..65535, по умолчанию 22; пробелы по краям срезаются
export function validVpsUser(s: string): boolean // 1..32, [a-z_][a-z0-9_-]*
// shared/ipc.ts
export type VpsStep = 'connect' | 'system' | 'install' | 'harden' | 'subscription' | 'add'
export type VpsStepState = 'start' | 'done' | 'skip' | 'warn' | 'failed'
export interface VpsEvent { step: VpsStep; state: VpsStepState; detail?: string }
export interface VpsRequest { address: string; user: string; password?: string; useKey: boolean; keyPassphrase?: string } // address — как ввёл человек; main проверяет его parseVpsAddress
export type VpsOutcome =
  | { kind: 'done'; servers: number; subscriptionId: string | null; warnings: string[] }
  | { kind: 'failed'; code: string; detail?: string }
  | { kind: 'cancelled' }
// bridge:
vpsPickKey(): Promise<{ name: string } | { error: 'too_large' | 'not_text' } | null> // ключ остаётся в main
vpsForgetKey(): Promise<void>
vpsSetup(req: VpsRequest): Promise<VpsOutcome>
vpsCancel(): Promise<void>
onVpsEvent(cb: (e: VpsEvent) => void): () => void // push IPC.vpsEvent
```
**Поведение:** `vpsSetup` — одна за раз (вторая — `{kind:'failed', code:'busy'}`); проверяет фазу (`off`/`error`, иначе `failed: routing_on`); `spawn(<helperSourceDir>\splitlane-helper.exe, ['server-setup'], {stdio: 'pipe', windowsHide: true})`; проверяет адрес и логин (`parseVpsAddress`, `validVpsUser`; иначе `failed: bad_input` без запуска процесса), пишет в stdin одну строку JSON (`host`, `port`, `user`, `password` или `key` из памяти main и `passphrase`), stdin остаётся открытым; строки stdout → `IPC.vpsEvent`; `done` → `proxy.add` с адресом подписки или ссылками (через `runProxyAdd` части 1) → `proxy.select` первого из `servers` → событие `add` `done` → `{kind:'done', servers: servers.length, …}`; неудача `proxy.add` — `failed: add_failed` (с кодом канала в `detail`). `vpsCancel`/«Выход»: закрыть stdin, через 5 с — `kill()`. Ключ: `vpsPickKey` — диалог (фильтр «Ключ SSH», все файлы), чтение ≤ 16 КБ + 1 из дескриптора (`readUpTo` из `confFile.ts`), не UTF-8 — `not_text`, больше — `too_large`; хранится в памяти main до `vpsForgetKey`, конца `vpsSetup` или закрытия окна. Запрос и ключ не пишутся в журналы окна; объект запроса и буфер ключа обнуляются после записи в stdin.

**Тесты (подмены `spawn`, диалога, файла, `LinkCalls`):** `vps.test.ts` — таблица адресов из Review Focus 3 и логинов; порядок событий; `done` → `proxy.add` с адресом подписки, затем `proxy.select(servers[0])`; без подписки — со ссылками; все серверы уже были (`added: []`, `servers: [a,b]`) — выбирается `a`; `failed` из подкоманды; процесс завершился без итоговой строки — `failed: kit_failed`; `vpsCancel` — stdin закрыт, через 5 с `kill`; вторая настройка — `busy`; фаза `on` — `routing_on` без запуска процесса; ключ 16 КБ + 1 — `too_large`, не UTF-8 — `not_text`; в аргументах `spawn` нет пароля; после `vpsSetup` ключ забыт.

- [ ] Шаги: тесты → провал → реализация → полная проверка окна → коммит `feat(app): run the server setup from the window`.

### Task 7: Экран «Свой VPS» и документация

**Files:** `app/src/renderer/model/connect.ts` (`ConnectMethod` + `'vps'`), новые `app/src/renderer/model/vps.ts` (проверка формы через `shared/vps.ts` задачи 6, редуктор шагов), `app/src/renderer/components/VpsPanel.tsx`, `VpsProgress.tsx` (+ `.module.css`), `app/src/renderer/screens/ConnectScreen.tsx` (плитка в обоих экранах), `app/src/renderer/i18n/ru.ts`, `en.ts`; тесты `model/vps.test.ts`, `components/VpsPanel.test.tsx`, `screens/ConnectScreen*.test.tsx`; `README.md`, `README.ru.md` (раздел «Свой VPS»), `PRIVACY.md` (что уходит на сервер, что не сохраняется, риск непроверяемого ключа сервера), `docs/superpowers/specs/2026-11-16-splitlane-server-setup-design.md` (только если в ходе задачи уточнён текст).

**Поведение:** плитка «Свой VPS» четвёртой; форма по спецификации, раздел 6 (подсказка дословно); проверка формы — `parseVpsAddress`/`validVpsUser` (Review Focus 3), ошибки под полем; «Настроить» недоступна вне `off`/`error` с подсказкой; прогресс — шесть шагов (`connect`, `system`, `install`, `harden`, `subscription`, `add`) с состояниями «ждёт/идёт/готово/пропущено/ошибка» и временем; «Отменить»; итог: «Готово: добавлено N серверов», в мастере — «Включить деление», «Подключить телефон» (открывает диалог задачи 8 по `subscriptionId`), предупреждения `sub_unreachable`, `sub_port80_busy`, `sub_skipped` («Телефон подключить не получилось: …»), `harden_failed`; ошибка — текст по коду (таблица спецификации, раздел 8, плюс `routing_on`, `busy`, `add_failed`) и «Попробовать снова»; уход с экрана — `vpsForgetKey` и очистка пароля в состоянии формы. Тексты на двух языках.

**Тесты:** проверка формы (таблица адресов и логинов), переходы шагов, недоступность при `on`, итог с подпиской и без, каждый код ошибки показывает свой текст на ru и en, «Попробовать снова» сохраняет поля, уход с экрана вызывает `vpsForgetKey`, пароль не попадает в снимок состояния окна (`settings`), в мастере после успеха — шаг «Включить».

- [ ] Шаги: тесты → провал → реализация → полная проверка окна → коммит `feat(app): your own VPS tile — form, progress and result; docs`.

### Task 8: «Подключить телефон» — QR подписки

**Files:** `app/package.json` (`qrcode-generator` `2.0.4`, точная версия), новый `app/src/renderer/components/PhoneQr.tsx` (+ `.module.css`, + тест), `app/src/renderer/components/Subscriptions.tsx` (кнопка у каждой подписки), экран итога задачи 7, `ru.ts`/`en.ts`.

**Поведение:** кнопка «Подключить телефон» у каждой подписки и на экране итога; по нажатию — `proxyReveal(id)`; ошибка — короткий текст по коду; успех — модальный диалог (фокус внутри, Esc и «Закрыть» закрывают, фокус возвращается на кнопку) с QR (`qrcode-generator`, тип 0 — авто, уровень коррекции `M`, SVG, белое поле, без сети) и подписью из спецификации, раздел 6; адрес в состоянии только пока диалог открыт; текстом адрес не показывается.

**Тесты:** нажатие → `proxyReveal` с id подписки; диалог показывает SVG и подпись; Esc закрывает, фокус возвращается; после закрытия адреса нет в DOM; ошибка `unknown_server` — текст; QR кодирует ровно адрес (декодировать не нужно: проверить, что `qrcode-generator` получил адрес — подмена модуля).

- [ ] Шаги: тесты → провал → реализация → полная проверка окна, `scripts\build-installer.ps1` → `BUILD-OK` → коммит `feat(app): connect a phone by the subscription QR`.

### Task 9: Живая проверка (нужен владелец и свежий VPS)

Владелец даёт свежий VPS (Debian 12+ или Ubuntu 22.04+, только что арендованный или с переустановленной системой) и вводит адрес, логин и пароль **в окно** (не в чат). Проверочная сборка ставится установщиком поверх 0.4.0 (UAC — владелец). Шаги: (1) неверный пароль — `ssh_auth`, понятный текст; (2) настройка из мастера по паролю — все шаги, итог, время; (3) «Включить деление» под сторожем (`Start-P1Guard -NoHelper`) — дорожки (VPN — адрес VPS, «Напрямую» — свой), выключение с возвратом exit node; (4) «Подключить телефон» — Hiddify на телефоне владельца сканирует QR и подключается; (5) повторная настройка того же сервера — те же серверы, «уже были в списке», выбран первый; (6) на сервере (через ту же SSH-сессию владельца или журнал набора) — fail2ban активен, порты как в итоге. Итоги — «Приложение A» спецификации; коммит `docs: the live check of stage 3 part 2`.
