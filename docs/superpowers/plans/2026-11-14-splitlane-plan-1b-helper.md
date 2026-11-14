# SplitLane, план 1B: служебный процесс до рабочего состояния — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Довести служебный процесс `splitlane-helper.exe` до состояния, с которым окно (план 2) работает без ручной правки файлов: исправить шесть отложенных находок плана 1A, убрать сбои запуска ядра, найденные на ПК, добавить коннектор Tailscale (вход, выход, список exit node, истечение входа, вид соединения), сон и смену сети, живую активность, `diagnostics.export`, режим продукта с командами `install` и `uninstall` и сценарную проверку раздела 12.

**Architecture:** Тот же модуль Go `splitlane/helper`. Автомат (`internal/engine`) получает режим настройки ядра и коннектор за интерфейсом `NodeAPI` (служба API ядра через `sing-box api`), уведомления о сети и питании, источник активности; драйвер `internal/singbox` — конфиг режима настройки и клиента службы API; новые пакеты: `internal/activity` (подсчёт активности 8.4), `internal/diag` (архив журналов), `internal/place` (пути и данные настройки продукта), `internal/install` (команды `install` и `uninstall`). Сценарная проверка — `scripts/scenario.mjs` на Node.js без зависимостей, второй клиент канала.

**Tech Stack:** Go 1.27.4 (директива `go 1.26.0`, `CGO_ENABLED=0`), `golang.org/x/sys` v0.49.0, `github.com/Microsoft/go-winio` v0.6.4, sing-box 1.14.4, Windows PowerShell 5.1, Node.js 24 (только `scripts/scenario.mjs`, без пакетов npm).

**Spec:** `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` — разделы 4.2, 4.3, 5.1–5.7, 6.1, 6.4, 7, 8.4, 10, 11, 12 и приложение A (проверки планов 1A и 1B). План 1A (`docs/superpowers/plans/2026-11-14-splitlane-plan-1-helper.md`, раздел «Что остаётся плану 1B») — перечень объёма; его отчёт `docs/superpowers/reports/2026-11-14-splitlane-plan-1a-rulings.md` — шесть отложенных находок, которые этот план исправляет первыми.

## Global Constraints

- Ядро: только sing-box 1.14.4 `windows-amd64` без изменений. SHA-256 архива `sing-box-1.14.4-windows-amd64.zip`: `99185d75d314a07dbba4a130c8ad986cbeb629a07927975636b341ca61f991df`; SHA-256 `sing-box.exe` из него: `8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9`. Рядом с ним в архиве — `libcronet.dll` (SHA-256 `3217c6260fbca5f16072e0b79735742f40109a63bb0ff88fd6b96dd6b54a2928`) и `LICENSE`; продукт поставляет все три файла без изменений. Конфиг собирается только под эту версию. Ядро для разработки: `C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe` (его кладёт `scripts\fetch-core.ps1`).
- Go: тулчейн 1.27.4 в `%LOCALAPPDATA%\Programs\go\bin`. В инструменте PowerShell каждый блок команд Go начинается с `$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0';`. Директива модуля `go 1.26.0`. Ошибка `The term 'go' is not recognized` — в блоке пропущено начало.
- Зависимости Go ровно две, версии закреплены: `golang.org/x/sys v0.49.0` и `github.com/Microsoft/go-winio v0.6.4`. Новых модулей нет; всё новое — стандартная библиотека и `x/sys/windows` (в нём есть `SetupDi*`, `NotifyRouteChange2` вызывается через `NewLazySystemDLL`).
- Канал `\\.\pipe\SplitLane.Helper`; описатель безопасности `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<SID пользователя>)`; протокол 1; сообщения — JSON по одному в строке, строка не длиннее 1 МиБ. Методы и коды ошибок — только из 5.1; новых методов и кодов нет.
- Файлы, которые читает ядро, и `state.json`, `restore.json`, `setup.json` пишутся в UTF-8 без BOM атомарно (`internal/atomicfile`). В PowerShell — только `[IO.File]::WriteAllText` с `New-Object Text.UTF8Encoding $false`.
- Модульные тесты не трогают систему. `tailscale.exe`, запуск ядра командой `run` с адаптером, маршруты, `%ProgramData%`, `%ProgramFiles%`, задачи планировщика, удаление устройств — только за интерфейсами, в тестах подмены. Разрешено тестам: временные папки, именованные каналы `SplitLane.Test.<число>`, скрытое окно в своём процессе, дочерний процесс — сам тестовый бинарник, `sing-box check` настоящего ядра (`SPLITLANE_CORE`), чтение настоящей системы только с `SPLITLANE_LIVE_READONLY=1`. Ядро в режиме настройки (без адаптера) тест не запускает: это делает задача 15 вручную.
- Живой шаг — любой шаг, после которого системный exit node может оказаться выключен. Он идёт только под сторожем `slguard.exe` плана 1A (`spike\plan1\p1lib.ps1`: `Start-P1Guard`, `Assert-P1Armed`, `Set-P1Heartbeat`, `Send-P1Guard`, `Stop-P1Guard`, `Test-P1SystemRestored`); каждый живой шаг — один вызов PowerShell с тайм-аутом 600000 мс, начинается с `if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat`, между вызовами не больше 400 секунд. Запрос UAC принимает человек; отменённый запрос без согласия пользователя не повторяется.
- В правила прямой дорожки в живых шагах попадают только переименованные копии `curl-*.exe` и `node-*.exe`. `claude.exe`, `bun.exe`, `node.exe`, `python.exe`, `svchost.exe` и браузеры в прямое правило не попадают никогда; основная дорожка в живых шагах — VPN; служебный процесс в живых шагах разработки запускается с `--dev --dev-pin`.
- Личные данные. В плане, спецификации, отчётах и коммитах нет имён устройств и сети Tailscale, имени ПК, настоящих адресов, секретов, ссылок для входа. Настоящие значения берутся во время работы из локальных игнорируемых файлов; `slctl` и сторож закрывают адреса в выводе (`<ip>`); ссылку для входа `slctl login` печатает только в консоль пользователя, в отчёты она не переносится. Тестовые данные — адреса из диапазонов документации (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) и вымышленные имена.
- Файлы PowerShell содержат только ASCII. `scripts/scenario.mjs` — ASCII, модуль ECMAScript, без пакетов npm, Node.js 24.
- Работа идёт в ветке `plan-1b-helper`. Каждая задача с кодом заканчивается коммитом; сообщение коммита — по-английски и заканчивается строкой `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Текст плана — по-русски; код, команды, идентификаторы и вывод программ — по-английски.

**Ручное восстановление** — тот же текст, что в Global Constraints плана 1A (блок «Ручное восстановление»); исполнитель показывает его пользователю до первого запроса UAC задач 16 и 17 и при любом итоге сторожа, кроме `armed`, `finished` и `tripped ... system_ok=True`.

## Review Focus

Условия, которые спецификация подразумевает, но прямо не описывает; от более вероятного к менее вероятному. Для каждого в задачу, которая владеет кодом, добавлен тест.

1. Окно подключается и отключается несколько раз подряд (перезагрузка страницы, повторный запуск окна), пока ядро режима настройки ещё стартует. Ожидание: работает не больше одного ядра режима настройки, отсчёт 5 минут до остановки начинается от последнего отключения и сбрасывается новым подключением, процесс ядра не остаётся без хозяина. Закреплено в задаче 8: `TestSetupModeSurvivesReconnects`.
2. Сеть пропадает снова во время 60-секундного ожидания проверок после смены сети. Ожидание: пока нет маршрута по умолчанию, ядро не перезапускается и откат не начинается; отсчёт 60 секунд начинается заново после возврата маршрута. Закреплено в задаче 10: `TestFlappingNetworkWaitsAgain`.
3. Клиент подписался на активность и перестал читать канал. Ожидание: сводки этому клиенту не блокируют служебный процесс и других клиентов; зависший клиент отключается так же, как при событиях `state`. Закреплено в задаче 11: `TestStalledSubscriberIsDropped`.
4. `install` запущен поверх работающей установки, у которой деление включено, а возврат системного exit node не подтверждается. Ожидание: `install` не заменяет файлы и задачу, пока прежний служебный процесс не подтвердил возврат и не завершился; сообщает причину и выходит с ошибкой; компьютер не остаётся без exit node и без служебного процесса. Закреплено в задаче 14: `TestReinstallWaitsForRestore`.
5. `tailscale.login` приходит повторно, пока ядро режима настройки уже работает и ссылка для входа уже есть. Ожидание: второе ядро не запускается, ссылка в состоянии остаётся прежней, ответ — без ошибки. Закреплено в задаче 8: `TestLoginTwiceKeepsOneSetupCore`.

---

## Как читать этот план

**Проверки на ПК, выполненные до плана** (2026-11-14, приложение A, «Проверки плана 1B»): сон — прямое соединение встроенного узла возвращается само; отключение кабеля — узел остаётся на ретрансляторе, пока процесс ядра не заменён; неподключённое устройство Wintun `SplitLane`, оставшееся после принудительного завершения ядра, в двух запусках из четырёх даёт 15-секундный сбой создания адаптера; в двух запусках из девяти собственный DNS ядра первые 44–48+ секунд не получает ответов. Отсюда четыре решения плана, которые меняют спецификацию (каждое записывает в спецификацию задача, которая его реализует):

| Решение | Раздел спецификации | Задача |
|---|---|---|
| Перед каждым запуском ядра служебный процесс удаляет неподключённое устройство Wintun со своим именем адаптера | 5.3, шаг 4 | 2 |
| Если доказательство туннеля (5.3, шаг 5) не удалось с причиной `tunnel_not_proven`, служебный процесс один раз останавливает ядро и повторяет шаги 3–5 с новыми портами; система до шага 6 не тронута | 5.3, шаг 5 | 7 |
| Флаг разработки `--core-log-level debug` для сбора сведений о задержке DNS; в продукте уровень `info` | 6.1 | 7 |
| После смены сети, когда проверки 5.5 прошли, а вид соединения с exit node 60 секунд остаётся `relay`, хотя до события был `direct`, ядро перезапускается (5.4, затем 5.3) | 10 | 10 |

**Порядок задач.** Задачи 1–14 — код с тестами, система не трогается. Задача 15 — команда сторожа `kill-core`, сборка и проверки на ПК без переключений (в том числе ядро режима настройки без прав администратора). Задача 16 — живая проверка под сторожем. Задача 17 — установка в режиме продукта, перезагрузка и удаление.

**Кто выполняет.** Задачи 16 и 17 целиком выполняет исполнитель, который сам ведёт разговор с пользователем (основной сеанс). Задачи 1–15 можно отдавать подчинённым агентам; живые шаги задачи 15 не переключают сеть.

**Код в плане.** Код каждой задачи с 1 по 15 написан и проверен при подготовке плана в отдельной рабочей копии: тест задачи без её кода даёт ровно ту ошибку, которую называет шаг «Запустить тест, убедиться в провале», вместе с кодом — проходит; `go vet` и `gofmt -l` чисты. Блок после строки вида «Файл <путь в обратных апострофах>, целиком:» переносится в файл полностью. Изменение существующего файла записано парами «Найти» / «Заменить на»: текст «Найти» встречается в файле ровно один раз и заменяется целиком. Пути вида `helper/...` и `scripts/...` — от корня репозитория `C:\code\router`. Файлы Go пишутся в UTF-8 без BOM.

**Где лежит проверенный код.** Коммиты задач 1–15 в том виде, который воспроизводит текст плана, — ветка `plan-1b-draft` (от `51426da` до `f202069`, по одному коммиту на задачу, в порядке задач). Весь текст плана, применённый подряд к `51426da`, даёт ровно файлы `f202069` (проверка `.superpowers\sdd\2026-11-14-splitlane-plan-1b-helper\apply-check.py`; вне git остаются только файлы в `spike\`).

**Команды.** Блоки `powershell` рассчитаны на инструмент PowerShell (Windows PowerShell 5.1). Команды `go` выполняются из каталога `C:\code\router\helper`, если не сказано иное.

### Карта файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `helper/internal/store/` | чтение `state.json` и `restore.json` без потери данных при ошибке ввода-вывода | 1 |
| `helper/internal/pipe/server.go` | приём соединений не прекращается после разовой ошибки | 1 |
| `helper/internal/engine/engine.go` | загрузка и сохранение состояния (1); операции «всё напрямую» и раскладки (6) | 1, 6 |
| `helper/internal/engine/commands.go` | таблица фаз 5.1 для новых методов | 6, 8, 9, 11, 12 |
| `helper/internal/winhost/adapter.go` | GUID адаптера по имени, удаление неподключённого устройства Wintun | 2 |
| `helper/internal/engine/enable.go` | удаление устройства перед запуском (2), повтор при `tunnel_not_proven` (7), остановка режима настройки перед шагом 3 (8) | 2, 7, 8 |
| `helper/internal/coreconfig/` | уровень журнала (7), конфиг режима настройки (3) | 3, 7 |
| `helper/internal/singbox/node.go` | клиент службы API ядра (`sing-box api tailscale ...`) | 3 |
| `helper/internal/engine/ports.go` | `NodeAPI`, `SetupOptions`, новые методы `CoreDriver` (3), `Host.RemoveStaleAdapter` (2), поля `Conn` (11) | 2, 3, 11 |
| `helper/internal/engine/connector.go` | режим настройки, вход, выход, данные узла (8), истечение входа и вид соединения при работе (9) | 8, 9 |
| `helper/internal/winhost/netwatch.go`, `helper/internal/session/` | уведомления об изменении маршрутов и о питании | 10 |
| `helper/internal/engine/network.go` | ожидание сети и проверок после события, перезапуск по `relay` | 10 |
| `helper/internal/protocol/activity.go`, `helper/internal/rules/folder.go`, `helper/internal/activity/` | сводка активности 8.4: типы, группировка, скорости | 4 |
| `helper/internal/singbox/clash.go`, `helper/internal/engine/activity.go`, `helper/internal/pipe/` | счётчики соединений, подписка `activity.subscribe`, событие `activity` | 11 |
| `helper/internal/diag/` | архив журналов `diagnostics.export` | 12 |
| `helper/internal/place/` | пути продукта и `setup.json` | 13 |
| `helper/internal/singbox/driver.go` | своё ядро опознаётся и по пути образа (5.7) | 13 |
| `helper/cmd/splitlane-helper/` | уровень журнала ядра (7), уведомления о сети (10), активность (11), режим продукта (13), подкоманды `install` и `uninstall` (14) | 7, 10, 11, 13, 14 |
| `helper/internal/install/`, `scripts/build-helper.ps1` | установка и удаление, сборка `dist\helper\` | 14 |
| `helper/cmd/slctl/` | команды `login`, `logout`, `activity`, `export` | 8, 11, 12 |
| `scripts/scenario.mjs`, `scripts/scenario.test.mjs` | сценарная проверка раздела 12 | 5 |

### Раздел 10 спецификации и тесты плана 1B

Строки раздела 10, которых касается этот план (остальные закреплены в плане 1A):

| Строка раздела 10 | Тест (задача) |
|---|---|
| Ядро не запустилось | `TestStaleAdapterIsRemovedBeforeStart` (2), `TestTunnelNotProvenIsRetriedOnce` (7) |
| Сервер VPN недоступен, истечение входа | `TestExpiredLoginWhileOn`, `TestLoginWhileExpiredDisablesFirst` (9) |
| Служебный процесс упал | `TestJournalReadErrorIsNotNoJournal` (1), `TestCrashDuringAllDirectWaitsForTheOperation` (6) |
| Сон и пробуждение, смена сети | `TestNoRouteHoldsTheCore`, `TestChecksFailAfterNetworkChangeRestart`, `TestFlappingNetworkWaitsAgain`, `TestRelayAfterNetworkChangeRestarts` (10) |
| Перезагрузка, отключение питания или сбой Windows | `TestIdleExitWithoutWindow`, `TestNoIdleExitWhenWasOn` (13); на ПК — задача 17 |

---

### Task 1: Надёжность хранилища и канала

Четыре находки финальной проверки плана 1A (отчёт `docs/superpowers/reports/2026-11-14-splitlane-plan-1a-rulings.md`, раздел «Припаркованные находки»). Общее у них одно: редкая ошибка ввода-вывода теряет данные, от которых зависит безопасность. (1) `store.LoadJournal` отдаёт одинаковое `(nil, err)` и для испорченного `restore.json` (его откладывают в сторону), и для целого журнала, который просто не удалось прочитать (доступ запрещён, файл занят другим процессом). `engine.load()` в обоих случаях решает, что журнала нет. После сбоя служебного процесса и такой ошибки чтения компьютер может остаться без ядра и без системного exit node, хотя 5.7 требует: «При старте служебный процесс первым делом читает журнал. Если журнал есть, фаза — `stopping`: выполняется шаг 1 из 5.4». (2) `store.LoadState` принимает ошибку чтения за порчу файла, и первое же сохранение затирает `state.json`: пропадают раскладка, выбранный exit node и признак «вход выполнен». (3) `engine.save()` только пишет неудачу в журнал, поэтому `lanes.set` отвечает успехом, хотя раскладка не сохранена (8.3, шаг 3: «Сохраняет раскладку в `state.json` … и отвечает окну»). (4) `pipe.Server.Serve` выходит при любой ошибке `Accept`, а `main` этот выход не замечает: после одного сбоя `CreateNamedPipe` служебный процесс держит имя канала, но новых окон не принимает (go-winio оставляет слушатель рабочим после любой ошибки, кроме `net.ErrClosed`).

Задача вводит `store.ErrIO` — «файл есть, но прочитать его не удалось». Ошибку оборачивают через `%w` вместе с причиной, так что `errors.Is(err, store.ErrIO)` истинно только для ошибки чтения. Испорченное содержимое, как и раньше, откладывается в сторону. Теперь так же поступают и с `state.json`: он уходит в `state.json.bad` (или в первое свободное `state.json.bad.<n>`) через тот же помощник, что и журнал. Если журнал не прочитан, старт стоит в фазе `stopping` с причиной `restore_pending`: в `params.error` — текст ошибки чтения, `params.command` пуст, потому что адрес exit node ещё неизвестен. Журнал перечитывается каждые 5 секунд по часам автомата; «Повторить» перечитывает его сразу. «Выключить без возврата» удаляет журнал, насколько это возможно, и старт идёт дальше без него. После удачного чтения старт идёт ровно как раньше. Непрочитанный `state.json` перечитывается ещё три раза через 200 мс. Если не вышло, в памяти остаются значения по умолчанию, а каждое сохранение сначала снова читает файл и ничего не пишет поверх, пока чтение не удаётся. Когда файл прочитался, его настройки возвращаются; признак «было включено» и идентичность ядра берутся из текущего запуска. Изменение, на котором это случилось, уступает файлу (`lanes.set` — `apply_failed`). `save()` теперь возвращает ошибку. При неудачной записи `lanes.set` отвечает `apply_failed`, а прежние файлы правил и режим ядра возвращаются так же, как в остальных ветках `apply_failed`. Остальные вызовы `save()` по-прежнему только пишут в журнал. Каждое сохранение записывает всю копию состояния, поэтому то, что не записала неудачная попытка («было включено», идентичность ядра), уходит на диск со следующим сохранением. `Serve` при ошибке `Accept`, которая не означает закрытия слушателя, пишет её в журнал и через секунду принимает снова; возвращается он только после `Close`. Спецификация не меняется: задача возвращает поведение, которое в ней уже описано.

**Files:**
- Modify: `helper/internal/store/store.go`, `helper/internal/store/store_test.go`
- Modify: `helper/internal/pipe/server.go`, `helper/internal/pipe/server_test.go`
- Modify: `helper/internal/engine/engine.go`, `helper/internal/engine/startup.go`, `helper/internal/engine/commands.go`, `helper/internal/engine/fakes_test.go`
- Create: `helper/internal/engine/storage_test.go`

**Interfaces:**
- Consumes: `store.Dir`, `store.Defaults`, `atomicfile.Write`, интерфейс `engine.Store` (`ports.go`, без изменений), `Engine.restoreSystem`, `Engine.dropJournal`, `Engine.revert`, `Engine.wake`, `Engine.skip`, `Clock.After`, `protocol.ReasonRestorePending`, `protocol.ParamCommand`, `protocol.ParamError`, `protocol.CodeApplyFailed`, `net.ErrClosed`.
- Produces: `var ErrIO = errors.New("store: read failed")` в пакете `store`; `func (d Dir) LoadState() (State, error)` (ошибка чтения — `ErrIO` и `Defaults()`, файл остаётся на месте; испорченное содержимое — `state.json.bad[.<n>]` и `Defaults()`); `func (d Dir) LoadJournal() (*Journal, error)` (ошибка чтения — `ErrIO`, журнал остаётся на месте); `func (e *Engine) save() error`; `func (e *Engine) readJournal()`; `func (e *Engine) applyLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string) (func(), *protocol.Error)`; `func newServer(l net.Listener, h Handler, log func(string, ...any)) *Server` в пакете `pipe`; `const acceptPause = time.Second`; `Listen` не меняется.

- [ ] **Step 1: Написать тесты хранилища**

Изменить `helper/internal/store/store_test.go`:

Найти:

```go
func TestJournalLifecycle(t *testing.T) {
```

Заменить на:

```go
// errSharing is the text Windows gives for a file another process holds open without sharing.
var errSharing = errors.New("The process cannot access the file because it is being used by another process.")

// failRead makes readFile fail with cause for the file with this name, as Windows does for a file held
// open by another process; other files are read. The returned function, and the end of the test,
// undo it.
func failRead(t *testing.T, name string, cause error) func() {
	old := readFile
	readFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == name {
			return nil, &os.PathError{Op: "open", Path: path, Err: cause}
		}
		return old(path)
	}
	undo := func() { readFile = old }
	t.Cleanup(undo)
	return undo
}

// An intact state.json that cannot be read gives the defaults and ErrIO; the file stays as it was and
// nothing is set aside, so the caller can keep it from being written over.
func TestStateIOErrorKeepsTheFile(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	s := Defaults()
	s.WasOn, s.SelectedExitNode = true, "100.64.0.1"
	if err := d.SaveState(s); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(d.Path, "state.json"))
	undo := failRead(t, "state.json", errSharing)
	got, err := d.LoadState()
	if !errors.Is(err, ErrIO) || !errors.Is(err, errSharing) || got.WasOn || got.SelectedExitNode != "" || got.DefaultLane != protocol.LaneVPN {
		t.Fatalf("want the defaults and ErrIO with the cause, got %+v %v", got, err)
	}
	undo()
	if after, _ := os.ReadFile(filepath.Join(d.Path, "state.json")); string(after) != string(before) {
		t.Fatalf("state.json changed:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "state.json.bad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nothing may be set aside: %v", err)
	}
	if got, err := d.LoadState(); err != nil || !got.WasOn || got.SelectedExitNode != "100.64.0.1" {
		t.Fatalf("the file must read again as it was: %+v %v", got, err)
	}
}

// Unreadable content of state.json is set aside the way a journal is: state.json.bad, then the first
// free state.json.bad.<n>. The error gives the cause and the new name and is not ErrIO; defaults follow.
func TestCorruptStateIsSetAside(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	for _, content := range []string{"{first", "{second"} {
		os.WriteFile(filepath.Join(d.Path, "state.json"), []byte(content), 0o644)
		s, err := d.LoadState()
		if err == nil || errors.Is(err, ErrIO) || !strings.Contains(err.Error(), "invalid character") ||
			!strings.Contains(err.Error(), "was set aside as state.json.bad") {
			t.Fatalf("%s: want a set-aside error with the cause, got %v", content, err)
		}
		if s.DefaultLane != protocol.LaneVPN || len(s.Apps) != 0 || s.ProbeURL != DefaultProbeURL {
			t.Fatalf("%s: want the defaults, got %+v", content, s)
		}
	}
	for file, want := range map[string]string{"state.json.bad": "{first", "state.json.bad.1": "{second"} {
		got, err := os.ReadFile(filepath.Join(d.Path, file))
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v, want %q", file, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(d.Path, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state.json must be gone after it was set aside: %v", err)
	}
}

func TestJournalLifecycle(t *testing.T) {
```

Найти:

```go
	for n := 1; n < maxBadJournals; n++ {
```

Заменить на:

```go
	for n := 1; n < maxBadFiles; n++ {
```

Найти:

```go
func TestRulesAndConfigFiles(t *testing.T) {
```

Заменить на:

```go
// An intact restore.json that cannot be read (a sharing violation, access denied) is not corrupt: the
// error is ErrIO with the cause, the journal stays where it is and nothing is set aside, so the caller
// cannot take it for "no journal". Unreadable content is never ErrIO.
func TestJournalIOErrorIsNotCorrupt(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if err := d.SaveJournal(Journal{ExitNode: "100.64.0.9", AllowLAN: true}); err != nil {
		t.Fatal(err)
	}
	undo := failRead(t, "restore.json", errSharing)
	j, err := d.LoadJournal()
	if j != nil || !errors.Is(err, ErrIO) || !errors.Is(err, errSharing) || strings.Contains(err.Error(), "set aside") {
		t.Fatalf("want ErrIO with the cause, nothing set aside, got %v %v", j, err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json.bad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nothing may be set aside: %v", err)
	}
	undo()
	if j, err := d.LoadJournal(); err != nil || j == nil || j.ExitNode != "100.64.0.9" || !j.AllowLAN {
		t.Fatalf("the journal must read again as it was: %+v %v", j, err)
	}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{broken"), 0o644)
	if _, err := d.LoadJournal(); err == nil || errors.Is(err, ErrIO) {
		t.Fatalf("unreadable content is not ErrIO: %v", err)
	}
}

func TestRulesAndConfigFiles(t *testing.T) {
```

- [ ] **Step 2: Запустить тесты хранилища, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/store/ -count=1
```

Ожидается: `FAIL splitlane/helper/internal/store [build failed]` и строки `undefined: readFile`, `undefined: ErrIO`, `undefined: maxBadFiles`.

- [ ] **Step 3: Реализовать `ErrIO` и откладывание `state.json`**

Изменить `helper/internal/store/store.go`:

Найти:

```go
// LoadState reads state.json; a missing file gives Defaults.
func (d Dir) LoadState() (State, error) {
	s := Defaults()
	b, err := os.ReadFile(d.state())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Defaults(), fmt.Errorf("state.json: %w", err)
	}
```

Заменить на:

```go
// ErrIO marks a file that is there but could not be read (access denied, a sharing violation): its
// content is unknown, so it stays where it is and must not be taken for missing or written over.
// Unreadable content is another error: that file is set aside.
var ErrIO = errors.New("store: read failed")

// readFile is os.ReadFile; the tests replace it to simulate a read that fails.
var readFile = os.ReadFile

// LoadState reads state.json; a missing file gives Defaults. A file that cannot be read gives Defaults
// and an error that wraps ErrIO: the file stays, and the caller must not save over it. Unreadable
// content is set aside as state.json.bad (or the first free state.json.bad.<n>) and gives Defaults
// with an error that gives the cause; if it cannot be set aside, the error says so.
func (d Dir) LoadState() (State, error) {
	s := Defaults()
	b, err := readFile(d.state())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("%w: %w", ErrIO, err)
	}
	if cause := json.Unmarshal(b, &s); cause != nil {
		return Defaults(), setAside(d.state(), cause)
	}
```

Найти:

```go
// maxBadJournals bounds how many set-aside journals are kept side by side.
const maxBadJournals = 100

// LoadJournal reads restore.json; nil without error when there is none.
// An unreadable journal is renamed to restore.json.bad (or the first free restore.json.bad.<n>)
// and reported as an error that gives the cause. If it cannot be renamed, the error says so
// and the journal stays where it is.
func (d Dir) LoadJournal() (*Journal, error) {
	b, err := os.ReadFile(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	cause := json.Unmarshal(b, &j)
	if cause == nil && j.ExitNode == "" {
		cause = errors.New("empty exitNode")
	}
	if cause != nil {
		bad, err := d.setAsideJournal()
		if err != nil {
			return nil, fmt.Errorf("restore.json is unreadable (%v) and could not be set aside: %w", cause, err)
		}
		return nil, fmt.Errorf("restore.json is unreadable (%v) and was set aside as %s", cause, filepath.Base(bad))
	}
	return &j, nil
}

// setAsideJournal renames restore.json to the first free name among restore.json.bad,
// restore.json.bad.1, restore.json.bad.2 ... so that an earlier set-aside journal is never replaced.
func (d Dir) setAsideJournal() (string, error) {
	for n := 0; n < maxBadJournals; n++ {
		bad := d.journal() + ".bad"
		if n > 0 {
			bad += "." + strconv.Itoa(n)
		}
		_, err := os.Lstat(bad)
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return bad, renameFile(d.journal(), bad)
	}
	return "", fmt.Errorf("all %d names restore.json.bad ... are taken", maxBadJournals)
}
```

Заменить на:

```go
// maxBadFiles bounds how many set-aside copies of one file are kept side by side.
const maxBadFiles = 100

// LoadJournal reads restore.json; nil without error when there is none. A journal that is there
// but cannot be read gives an error that wraps ErrIO: it stays where it is, and the caller must not
// take it for "no journal". An unreadable journal is renamed to restore.json.bad (or the first free
// restore.json.bad.<n>) and reported as an error that gives the cause. If it cannot be renamed, the
// error says so and the journal stays where it is.
func (d Dir) LoadJournal() (*Journal, error) {
	b, err := readFile(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIO, err)
	}
	var j Journal
	cause := json.Unmarshal(b, &j)
	if cause == nil && j.ExitNode == "" {
		cause = errors.New("empty exitNode")
	}
	if cause != nil {
		return nil, setAside(d.journal(), cause)
	}
	return &j, nil
}

// setAside renames a file with unreadable content to the first free name among <name>.bad,
// <name>.bad.1, <name>.bad.2 ... so that an earlier set-aside copy is never replaced, and returns
// the error that reports it with its cause. If it cannot be renamed, the error says so and the file
// stays where it is.
func setAside(path string, cause error) error {
	name := filepath.Base(path)
	bad, err := badName(path)
	if err == nil {
		err = renameFile(path, bad)
	}
	if err != nil {
		return fmt.Errorf("%s is unreadable (%v) and could not be set aside: %w", name, cause, err)
	}
	return fmt.Errorf("%s is unreadable (%v) and was set aside as %s", name, cause, filepath.Base(bad))
}

// badName is the first name among <path>.bad, <path>.bad.1, <path>.bad.2 ... that is not taken.
func badName(path string) (string, error) {
	for n := 0; n < maxBadFiles; n++ {
		bad := path + ".bad"
		if n > 0 {
			bad += "." + strconv.Itoa(n)
		}
		_, err := os.Lstat(bad)
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return bad, nil
	}
	return "", fmt.Errorf("all %d names %s.bad ... are taken", maxBadFiles, filepath.Base(path))
}
```

- [ ] **Step 4: Запустить тесты хранилища, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/store/ -count=1 -v
```

Ожидается: `--- PASS: TestStateIOErrorKeepsTheFile`, `--- PASS: TestCorruptStateIsSetAside`, `--- PASS: TestJournalIOErrorIsNotCorrupt`, прежние тесты пакета тоже `PASS` (в том числе `TestJournalThatCannotBeSetAsideSaysSo` и `TestJournalSetAsideGivesUpWhenAllNamesAreTaken`), в конце `ok  	splitlane/helper/internal/store`.

- [ ] **Step 5: Написать тест канала**

Изменить `helper/internal/pipe/server_test.go`:

Найти:

```go
	"encoding/json"
	"fmt"
	"math/rand"
```

Заменить на:

```go
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
```

Найти:

```go
// A second helper cannot take the same pipe: one instance at a time.
```

Заменить на:

```go
// flakyListener fails its first Accept calls with errs, then hands out the connections sent on conns
// until it is closed, like go-winio's listener: an error other than net.ErrClosed leaves it usable.
type flakyListener struct {
	mu     sync.Mutex
	errs   []error
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newFlakyListener(errs ...error) *flakyListener {
	return &flakyListener{errs: errs, conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *flakyListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *flakyListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *flakyListener) Addr() net.Addr { return &net.UnixAddr{Name: "flaky", Net: "unix"} }

// An Accept error that is not the close of the listener (a failed CreateNamedPipe) does not end the
// server: it is logged, Accept is tried again after a pause, the next client is served. Serve returns
// only after Close.
func TestAcceptErrorDoesNotStopServing(t *testing.T) {
	l := newFlakyListener(errors.New("CreateNamedPipe: All pipe instances are busy."))
	s := newServer(l, &fakeHandler{}, t.Logf)
	served := make(chan error, 1)
	began := time.Now()
	go func() { served <- s.Serve() }()
	conn, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	select {
	case l.conns <- conn:
	case err := <-served:
		t.Fatalf("Serve returned after an accept error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept was not tried again after the error")
	}
	if waited := time.Since(began); waited < acceptPause*9/10 {
		t.Fatalf("Accept was tried again after %s, want a pause of %s", waited, acceptPause)
	}
	c := &testClient{conn: peer, r: bufio.NewReader(peer)}
	if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`); m["result"] == nil {
		t.Fatalf("the client after the error is not served: %v", m)
	}
	select {
	case err := <-served:
		t.Fatalf("Serve returned before Close: %v", err)
	default:
	}
	s.Close()
	select {
	case err := <-served:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Serve ended with %v, want the close of the listener", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

// A second helper cannot take the same pipe: one instance at a time.
```

- [ ] **Step 6: Запустить тест канала, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/pipe/ -run 'TestAcceptErrorDoesNotStopServing' -count=1
```

Ожидается: `FAIL splitlane/helper/internal/pipe [build failed]` и строки `undefined: newServer`, `undefined: acceptPause`. (Если добавить только `newServer` и `acceptPause`, а `Serve` оставить прежним, тест падает с `Serve returned after an accept error: CreateNamedPipe: All pipe instances are busy.`: так проверено при подготовке плана.)

- [ ] **Step 7: Реализовать повтор `Accept`**

Изменить `helper/internal/pipe/server.go`:

Найти:

```go
	"context"
	"encoding/json"
	"net"
```

Заменить на:

```go
	"context"
	"encoding/json"
	"errors"
	"net"
```

Найти:

```go
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Server{h: h, l: l, log: log, clients: map[*client]bool{}}, nil
}

// Serve accepts clients until Close.
func (s *Server) Serve() error {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			return err
		}
```

Заменить на:

```go
	if err != nil {
		return nil, err
	}
	return newServer(l, h, log), nil
}

// newServer serves the connections of l; the tests give it a listener of their own.
func newServer(l net.Listener, h Handler, log func(string, ...any)) *Server {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Server{h: h, l: l, log: log, clients: map[*client]bool{}}
}

// acceptPause is the wait before Accept is tried again after an error.
const acceptPause = time.Second

// Serve accepts clients until Close and then returns the listener's error (net.ErrClosed). Any
// other Accept error (go-winio keeps its listener usable after it, for example after a failed
// CreateNamedPipe) is logged and Accept is tried again after a pause: one rare failure must not
// leave the helper holding the pipe name without taking new windows.
func (s *Server) Serve() error {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || s.isClosing() {
				return err
			}
			s.log("pipe: accept failed: %v; trying again in %s", err, acceptPause)
			time.Sleep(acceptPause)
			continue
		}
```

Найти:

```go
// waitFor waits for wg at most d; false when the time ran out.
```

Заменить на:

```go
func (s *Server) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

// waitFor waits for wg at most d; false when the time ran out.
```

- [ ] **Step 8: Запустить тесты канала, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/pipe/ -count=1 -v
```

Ожидается: `--- PASS: TestAcceptErrorDoesNotStopServing (1.00s)` (секунда — пауза перед повтором), остальные тесты пакета `PASS`, в конце `ok  	splitlane/helper/internal/pipe`.

- [ ] **Step 9: Написать тесты автомата**

Подмена хранилища учится отказывать: `LoadState` — ошибкой чтения заданное число раз, `LoadJournal` — заданными ошибками по очереди, `SaveState` — заданной ошибкой; она считает вызовы.

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
type fakeStore struct {
	mu         sync.Mutex
	rec        *recorder
	state      store.State
	journal    *store.Journal
	direct     []byte
	vpn        []byte
	config     []byte
	rulesErr   error
	journalErr error
	onRules    func() // runs after each write of the rule files
}

func (s *fakeStore) LoadState() (store.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}

func (s *fakeStore) SaveState(st store.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
	return nil
}

func (s *fakeStore) LoadJournal() (*store.Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal == nil {
```

Заменить на:

```go
type fakeStore struct {
	mu              sync.Mutex
	rec             *recorder
	state           store.State
	journal         *store.Journal
	direct          []byte
	vpn             []byte
	config          []byte
	rulesErr        error
	journalErr      error
	onRules         func()  // runs after each write of the rule files
	stateReadFails  int     // so many next LoadState calls fail with a read error (store.ErrIO)
	journalReadErrs []error // the next LoadJournal calls fail with these, one by one
	saveErr         error   // SaveState fails with it; state.json keeps what it had
	stateReads      int     // calls of LoadState
	stateSaves      int     // calls of SaveState
	journalReads    int     // calls of LoadJournal
}

// readErr is how the store reports a file that is there but could not be read (store.ErrIO).
func readErr(file string) error {
	return fmt.Errorf("%w: open C:\\data\\%s: The process cannot access the file because it is being used by another process.", store.ErrIO, file)
}

// calls gives how often state.json was read and written and restore.json was read.
func (s *fakeStore) calls() (stateReads, stateSaves, journalReads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateReads, s.stateSaves, s.journalReads
}

func (s *fakeStore) LoadState() (store.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateReads++
	if s.stateReadFails > 0 {
		s.stateReadFails--
		return store.Defaults(), readErr("state.json")
	}
	return s.state, nil
}

func (s *fakeStore) SaveState(st store.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateSaves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = st
	return nil
}

func (s *fakeStore) LoadJournal() (*store.Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journalReads++
	if len(s.journalReadErrs) > 0 {
		err := s.journalReadErrs[0]
		s.journalReadErrs = s.journalReadErrs[1:]
		return nil, err
	}
	if s.journal == nil {
```

Файл `helper/internal/engine/storage_test.go`, целиком:

```go
package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// pendingState is the first published state with reason restore_pending, or nil.
func pendingState(h *harness) *protocol.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.states {
		if s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending {
			return &s
		}
	}
	return nil
}

// 5.7 and the row "the helper failed" of section 10: a restore.json that is there but cannot be read
// (a sharing violation, access denied) is not "no journal". The start-up stays in stopping with
// restore_pending (the read error, no command: the exit node is unknown until the journal is read)
// and reads it again every 5 s; once it reads, step 1 of 5.4 gives the exit node back. Unreadable
// content, which the store sets aside, is no journal, as before.
func TestJournalReadErrorIsNotNoJournal(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.journalReadErrs = []error{readErr("restore.json"), readErr("restore.json")}
	h.sys.set(func(s *fakeSys) { s.on = false })
	began := h.clock.Now()
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.store.hasJournal() {
		t.Fatalf("the exit node must come back from the journal read at last (on %v %s): %v", on, ip, h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" {
		t.Fatalf("phases %s", got)
	}
	p := pendingState(h)
	if p == nil || p.Phase != protocol.PhaseStopping || p.Reason.Params[protocol.ParamCommand] != "" ||
		!strings.Contains(p.Reason.Params[protocol.ParamError], "being used by another process") ||
		!p.SystemTailscale.RestoreOnDisable {
		t.Fatalf("want stopping with restore_pending, the read error and no command, got %+v", p)
	}
	if _, _, reads := h.store.calls(); reads != 3 || h.clock.Now().Sub(began) < 2*restoreEvery {
		t.Fatalf("read again every %s: %d reads in %s", restoreEvery, reads, h.clock.Now().Sub(began))
	}

	h = newHarness(t)
	h.store.journalReadErrs = []error{errors.New("restore.json is unreadable (invalid character 'b') and was set aside as restore.json.bad")}
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if got := strings.Join(h.phases(), ","); got != "off" || len(h.rec.list()) != 0 {
		t.Fatalf("a journal set aside is no journal: phases %s, events %v", got, h.rec.list())
	}
}

// "Disable without restore" (5.4) ends the wait for a journal that cannot be read, too: the journal is
// deleted as far as it can be, the system client is not called, the start-up goes on without it.
func TestUnreadJournalCanBeSkipped(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	for i := 0; i < 10000; i++ {
		h.store.journalReadErrs = append(h.store.journalReadErrs, readErr("restore.json"))
	}
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return pendingState(h) != nil })
	if err := call(h, protocol.MethodDisable, protocol.DisableParams{SkipRestore: true}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	if h.rec.count("journal:delete") != 1 || h.rec.index("sys:") >= 0 || h.e.State().SystemTailscale.RestoreOnDisable {
		t.Fatalf("skip: journal deleted, the system client left alone: %v", h.rec.list())
	}
}

// A state.json that is there but cannot be read is never written over. The start reads it three more
// times 200 ms apart, then goes on with the defaults in memory; every save reads it again first and
// writes nothing while that fails (lanes.set: apply_failed). Once it reads, its settings come back
// ("was on" and the core identity stay the current run's), the change that met it gives way
// (apply_failed), and from then on saving works as before.
func TestStateReadErrorIsNeverOverwritten(t *testing.T) {
	h := newHarness(t)
	h.store.state.Apps = []protocol.AppRule{fileRule(idA, curlA, protocol.LaneDirect)}
	h.store.stateReadFails = 1000
	began := h.clock.Now()
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	reads, saves, _ := h.store.calls()
	if saves != 0 || reads < 5 || h.clock.Now().Sub(began) < 600*time.Millisecond {
		t.Fatalf("want 4 reads 200 ms apart at start and a read instead of a write: %d reads, %d saves in %s",
			reads, saves, h.clock.Now().Sub(began))
	}
	if s := h.e.State(); len(s.Apps) != 0 || s.Tailscale.SelectedExitNode != nil {
		t.Fatalf("the defaults must be in memory: %+v", s)
	}
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneDirect, Apps: []protocol.AppRule{fileRule(idB, curlB, protocol.LaneDirect)}}
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	if _, saves, _ := h.store.calls(); saves != 0 || len(h.e.State().Apps) != 0 {
		t.Fatalf("still unreadable: nothing written, nothing kept: %d saves, %+v", saves, h.e.State().Apps)
	}

	h.store.mu.Lock()
	h.store.stateReadFails = 0
	h.store.mu.Unlock()
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	s := h.e.State()
	if len(s.Apps) != 1 || s.Apps[0].ID != idA || s.Tailscale.SelectedExitNode == nil || *s.Tailscale.SelectedExitNode != exitNode {
		t.Fatalf("the settings of state.json must come back: %+v", s)
	}
	if got := h.store.saved(); len(got.Apps) != 1 || got.Apps[0].ID != idA || !got.LoggedIn || got.WasOn {
		t.Fatalf("state.json must keep its settings: %+v", got)
	}
	if err := call(h, protocol.MethodLanesSet, lanes); err != nil {
		t.Fatal(err)
	}
	if got := h.store.saved().Apps; len(got) != 1 || got[0].ID != idB {
		t.Fatalf("saving works again: %+v", got)
	}
}

// 8.3 step 3: lanes.set answers after the layout is in state.json. A write that fails is apply_failed;
// as on the other apply_failed paths the previous rule files and mode come back, and the layout in
// memory stays the previous one. What a failed save did not write goes with the next save.
func TestLanesSetSaveFailureIsApplyFailed(t *testing.T) {
	h := enabled(t)
	oldDirect, oldVPN := string(h.store.direct), string(h.store.vpn)
	h.store.mu.Lock()
	h.store.saveErr = errors.New("There is not enough space on the disk.")
	h.store.mu.Unlock()
	mark := len(h.rec.list())
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneDirect, Apps: []protocol.AppRule{fileRule(idA, curlA, protocol.LaneDirect)}}
	err := call(h, protocol.MethodLanesSet, lanes)
	wantCode(t, err, protocol.CodeApplyFailed)
	if !strings.Contains(err.Message, "not enough space") {
		t.Fatalf("the message must give the write error: %q", err.Message)
	}
	if string(h.store.direct) != oldDirect || string(h.store.vpn) != oldVPN || h.clash.currentMode() != "vpn" ||
		!inOrder(h, mark, "rules:write", "clash:mode direct", "rules:write", "clash:mode vpn") {
		t.Fatalf("the previous rule files and mode must come back: mode %s, events %v", h.clash.currentMode(), h.rec.list()[mark:])
	}
	h.e.mu.Lock()
	runDirect, runMode := string(h.e.run.direct), h.e.run.mode
	h.e.mu.Unlock()
	if runDirect != oldDirect || runMode != "vpn" {
		t.Fatalf("the running core must be known to read the previous files in mode vpn, got mode %s", runMode)
	}
	if s := h.e.State(); s.DefaultLane != protocol.LaneVPN || len(s.Apps) != 0 {
		t.Fatalf("the layout must stay the previous one: %+v", s)
	}

	call(h, protocol.MethodDisable, nil) // "was on" goes, but the disk is full
	h.waitPhase(protocol.PhaseOff)
	if !h.store.saved().WasOn {
		t.Fatal("setup: the failed saves must have left state.json as it was")
	}
	h.store.mu.Lock()
	h.store.saveErr = nil
	h.store.mu.Unlock()
	if err := call(h, protocol.MethodLanesSet, lanes); err != nil {
		t.Fatal(err)
	}
	if got := h.store.saved(); got.WasOn || got.DefaultLane != protocol.LaneDirect || len(got.Apps) != 1 {
		t.Fatalf("the next save writes the whole state: %+v", got)
	}
}
```

- [ ] **Step 10: Запустить тесты автомата, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestJournalReadErrorIsNotNoJournal|TestUnreadJournalCanBeSkipped|TestStateReadErrorIsNeverOverwritten|TestLanesSetSaveFailureIsApplyFailed' -count=1
```

Ожидается: четыре провала, `TestUnreadJournalCanBeSkipped` — после 5 секунд ожидания:

```text
--- FAIL: TestJournalReadErrorIsNotNoJournal (0.00s)
    storage_test.go:39: the exit node must come back from the journal read at last (on false 100.64.0.1): []
--- FAIL: TestUnreadJournalCanBeSkipped (5.01s)
    storage_test.go:72: timed out waiting for restore_pending; phases [off]; events []
--- FAIL: TestStateReadErrorIsNeverOverwritten (0.00s)
    storage_test.go:96: want 4 reads 200 ms apart at start and a read instead of a write: 1 reads, 1 saves in 0s
--- FAIL: TestLanesSetSaveFailureIsApplyFailed (0.02s)
    storage_test.go:139: want apply_failed, got <nil>
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 11: Реализовать загрузку и сохранение без потери данных**

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
import (
	"context"
	"sync"
```

Заменить на:

```go
import (
	"context"
	"errors"
	"sync"
```

Найти:

```go
// restartPauses: at most three restarts with a growing pause (5.5).
var restartPauses = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// errCancelled ends an operation that routing.disable or helper.shutdown cancelled; it is never published.
var errCancelled = &protocol.Reason{Code: "cancelled", Params: map[string]string{}}
```

Заменить на:

```go
// A state.json that cannot be read at start (an antivirus or a backup holds it for a moment) is read
// again so many times, this far apart, before the helper goes on with the defaults.
const (
	stateRetries = 3
	stateEvery   = 200 * time.Millisecond
)

// restartPauses: at most three restarts with a growing pause (5.5).
var restartPauses = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// errCancelled ends an operation that routing.disable or helper.shutdown cancelled; it is never published.
var errCancelled = &protocol.Reason{Code: "cancelled", Params: map[string]string{}}

// errStateLate: a save found state.json readable only now; its settings replaced the change being saved.
var errStateLate = errors.New("state.json could be read only now: the settings it holds are back and this change is not kept")
```

Найти:

```go
	saved        store.State
	journal      *store.Journal
	run          *coreRun
```

Заменить на:

```go
	saved        store.State
	journal      *store.Journal
	stateErr     error // state.json could not be read (store.ErrIO): nothing is written over it until it reads
	journalErr   error // restore.json could not be read (store.ErrIO): the start-up reads it again first
	run          *coreRun
```

Найти:

```go
// load reads state.json and restore.json into the engine.
func (e *Engine) load() {
	s, err := e.d.Store.LoadState()
	if err != nil {
		e.d.Log("state.json: %v; starting from defaults", err)
	}
	j, err := e.d.Store.LoadJournal()
	if err != nil {
		e.d.Log("restore journal: %v", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved, e.journal = s, j
	e.syncSaved()
}
```

Заменить на:

```go
// load reads state.json and restore.json into the engine. A state.json that cannot be read
// (store.ErrIO) is read again three times 200 ms apart; if it still fails, the defaults stay in
// memory and save writes nothing over the file until it reads. A journal that cannot be read is
// not "no journal": the start-up reads it again before anything else (readJournal). A journal with
// unreadable content has been set aside by the store: there is none.
func (e *Engine) load() {
	s, err := e.d.Store.LoadState()
	for n := 0; n < stateRetries && errors.Is(err, store.ErrIO); n++ {
		<-e.d.Clock.After(stateEvery)
		s, err = e.d.Store.LoadState()
	}
	var stateErr error
	switch {
	case errors.Is(err, store.ErrIO):
		stateErr = err
		e.d.Log("state.json: %v; starting from defaults, the file is not written over until it can be read", err)
	case err != nil:
		e.d.Log("state.json: %v; starting from defaults", err)
	}
	j, err := e.d.Store.LoadJournal()
	var journalErr error
	switch {
	case errors.Is(err, store.ErrIO):
		journalErr = err
		e.d.Log("restore journal: %v; it is read again before anything else", err)
	case err != nil:
		e.d.Log("restore journal: %v", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved, e.journal = s, j
	e.stateErr, e.journalErr = stateErr, journalErr
	e.syncSaved()
}

// readJournal begins the start-up (5.7) when restore.json could not be read: the journal may be
// there, so the helper does not go on as if there were none. The phase is stopping with reason
// restore_pending (the read error; no command: the exit node is unknown until the journal reads),
// and the journal is read again every 5 s, or at once on "Повторить", until it reads. "Disable
// without restore" (5.4) gives it up: the journal is deleted as far as it can be and the start-up
// goes on without it.
func (e *Engine) readJournal() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.journalErr == nil {
		return
	}
	for e.journalErr != nil {
		if e.skip {
			e.skip, e.journalErr = false, nil
			e.dropJournal()
			e.d.Log("restore skipped by the user: the unread journal deleted, system exit node left as is")
			return
		}
		e.setPhase(protocol.PhaseStopping, reason(protocol.ReasonRestorePending,
			protocol.ParamCommand, "", protocol.ParamError, e.journalErr.Error()))
		e.mu.Unlock()
		select {
		case <-e.wake:
		case <-e.d.Clock.After(restoreEvery):
		}
		j, err := e.d.Store.LoadJournal()
		e.mu.Lock()
		switch {
		case errors.Is(err, store.ErrIO):
			e.journalErr = err
		case err != nil:
			e.d.Log("restore journal: %v", err)
			e.journal, e.journalErr = nil, nil
		default:
			e.journal, e.journalErr = j, nil
		}
	}
	e.d.Log("restore journal read")
	e.setPhase(protocol.PhaseStopping, nil)
}
```

Найти:

```go
	e.st.SystemTailscale.RestoreOnDisable = e.journal != nil
}
```

Заменить на:

```go
	e.st.SystemTailscale.RestoreOnDisable = e.journal != nil || e.journalErr != nil
}
```

Найти:

```go
// save writes state.json; e.mu must be held.
func (e *Engine) save() {
	if err := e.d.Store.SaveState(e.saved); err != nil {
		e.d.Log("state.json not saved: %v", err)
	}
}
```

Заменить на:

```go
// save writes state.json; e.mu must be held. A failed write leaves the engine's copy as it is, and
// the next save writes all of it again, "was on" and the core identity with it. While state.json
// could not be read (load), nothing is written over it: each save reads it again first. Once it
// reads, the settings it holds are taken over, "was on" and the core identity stay those of this
// run, and save returns errStateLate: the change being saved has given way to the file.
func (e *Engine) save() error {
	late := false
	if e.stateErr != nil {
		s, err := e.d.Store.LoadState()
		if errors.Is(err, store.ErrIO) {
			e.stateErr = err
			e.d.Log("state.json not saved: it still cannot be read and is not written over: %v", err)
			return err
		}
		if err != nil {
			e.d.Log("state.json: %v", err)
		}
		e.d.Log("state.json can be read now: its settings are back")
		s.WasOn, s.Core = e.saved.WasOn, e.saved.Core
		e.saved, e.stateErr, late = s, nil, true
	}
	err := e.d.Store.SaveState(e.saved)
	if err != nil {
		e.d.Log("state.json not saved: %v", err)
	}
	if late {
		return errStateLate
	}
	return err
}
```

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
// Start loads state.json and the journal and begins the start-up of 5.7. It returns at once;
// the pipe already works while the start-up runs and shows its phase and reason.
func (e *Engine) Start() {
	e.load()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startOp("startup", e.startup)
}

// startup is 5.7: the journal first (phase stopping, step 1 of 5.4, whether a core is left or
// not), then the helper's own core left from an earlier run is stopped; a foreign sing-box.exe
// is never touched. If "was on" is still set, the previous helper ended abnormally: enable again.
func (e *Engine) startup(ctx context.Context) {
	e.mu.Lock()
```

Заменить на:

```go
// Start loads state.json and the journal and begins the start-up of 5.7. It returns at once (after
// at most 600 ms when state.json cannot be read); the pipe already works while the start-up runs
// and shows its phase and reason.
func (e *Engine) Start() {
	e.load()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startOp("startup", e.startup)
}

// startup is 5.7: the journal first (phase stopping, step 1 of 5.4, whether a core is left or
// not; a journal that could not be read is read again until it reads), then the helper's own core
// left from an earlier run is stopped; a foreign sing-box.exe is never touched. If "was on" is
// still set, the previous helper ended abnormally: enable again.
func (e *Engine) startup(ctx context.Context) {
	e.readJournal()
	e.mu.Lock()
```

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
	"context"
	"encoding/json"
	"slices"
```

Заменить на:

```go
	"context"
	"encoding/json"
	"errors"
	"slices"
```

Найти:

```go
	if live(phase) {
		if err := e.applyLanes(ctx, apps, p.DefaultLane); err != nil {
			return err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved.Apps, e.saved.DefaultLane = apps, p.DefaultLane
	e.save()
	e.publish()
	return nil
}

// applyLanes writes the rule files and switches the mode, then reconciles for 3 s (8.3).
// On a reload error or connections left on the wrong lane the previous files and mode come back.
func (e *Engine) applyLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string) *protocol.Error {
	e.mu.Lock()
	r, allDirect, degraded := e.run, e.st.AllDirect, e.degraded
	var oldDirect, oldVPN []byte
	var oldMode string
	if r != nil {
		oldDirect, oldVPN, oldMode = r.direct, r.vpn, r.mode
	}
	e.mu.Unlock()
	if r == nil {
		return protocol.Errorf(protocol.CodeApplyFailed, "no core is running")
	}
	set, err := rules.Compile(apps, e.d.Rules)
	if err != nil {
		return protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	direct, vpn, err := e.d.Core.LaneFiles(apps, e.d.Rules)
	if err != nil {
		return protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	mode := e.d.Core.Mode(defaultLane, allDirect, degraded)
	mark := e.d.Core.LogSize()
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
		return protocol.Errorf(protocol.CodeApplyFailed, "rule files: %v", err)
	}
	if mode != oldMode {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
			return protocol.Errorf(protocol.CodeApplyFailed, "mode: %v", err)
		}
	}
	if !e.reconcile(ctx, r, func(path string) string { return expectedLane(set, defaultLane, allDirect, path) }, mark) {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, mode != oldMode)
		return protocol.Errorf(protocol.CodeApplyFailed, "the core did not apply the layout within %s", applyWindow)
	}
	e.mu.Lock()
	r.direct, r.vpn, r.mode = direct, vpn, mode
	e.mu.Unlock()
	return nil
}
```

Заменить на:

```go
	var undo func()
	if live(phase) {
		var aerr *protocol.Error
		if undo, aerr = e.applyLanes(ctx, apps, p.DefaultLane); aerr != nil {
			return aerr
		}
	}
	// Step 3: the answer comes only after the layout is in state.json. A write that fails is
	// apply_failed: the previous layout stays, and the core gets its previous files and mode back.
	e.mu.Lock()
	prevApps, prevLane := e.saved.Apps, e.saved.DefaultLane
	e.saved.Apps, e.saved.DefaultLane = apps, p.DefaultLane
	saveErr := e.save()
	switch {
	case saveErr == nil, errors.Is(saveErr, errStateLate):
		e.publish() // the new layout, or the one state.json holds, read only now
	default:
		e.saved.Apps, e.saved.DefaultLane = prevApps, prevLane
	}
	e.mu.Unlock()
	if saveErr != nil {
		if undo != nil {
			undo()
		}
		return protocol.Errorf(protocol.CodeApplyFailed, "layout not saved: %v", saveErr)
	}
	return nil
}

// applyLanes writes the rule files and switches the mode, then reconciles for 3 s (8.3).
// On a reload error or connections left on the wrong lane the previous files and mode come back.
// After a success it returns undo, which brings the previous files and mode back the same way.
func (e *Engine) applyLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string) (func(), *protocol.Error) {
	e.mu.Lock()
	r, allDirect, degraded := e.run, e.st.AllDirect, e.degraded
	var oldDirect, oldVPN []byte
	var oldMode string
	if r != nil {
		oldDirect, oldVPN, oldMode = r.direct, r.vpn, r.mode
	}
	e.mu.Unlock()
	if r == nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "no core is running")
	}
	set, err := rules.Compile(apps, e.d.Rules)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	direct, vpn, err := e.d.Core.LaneFiles(apps, e.d.Rules)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	mode := e.d.Core.Mode(defaultLane, allDirect, degraded)
	mark := e.d.Core.LogSize()
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "rule files: %v", err)
	}
	if mode != oldMode {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
			return nil, protocol.Errorf(protocol.CodeApplyFailed, "mode: %v", err)
		}
	}
	if !e.reconcile(ctx, r, func(path string) string { return expectedLane(set, defaultLane, allDirect, path) }, mark) {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, mode != oldMode)
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "the core did not apply the layout within %s", applyWindow)
	}
	e.mu.Lock()
	r.direct, r.vpn, r.mode = direct, vpn, mode
	e.mu.Unlock()
	undo := func() {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, mode != oldMode)
		e.mu.Lock()
		r.direct, r.vpn, r.mode = oldDirect, oldVPN, oldMode
		e.mu.Unlock()
	}
	return undo, nil
}
```

Остальные вызовы `e.save()` (в `enable.go`, `teardown.go`, `startup.go`, `commands.go`) не меняются: возвращаемую ошибку они не читают, а `save` сама пишет её в журнал.

- [ ] **Step 12: Запустить тесты автомата, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestJournalReadErrorIsNotNoJournal|TestUnreadJournalCanBeSkipped|TestStateReadErrorIsNeverOverwritten|TestLanesSetSaveFailureIsApplyFailed' -count=1 -v
```

Ожидается: `--- PASS: TestJournalReadErrorIsNotNoJournal`, `--- PASS: TestUnreadJournalCanBeSkipped`, `--- PASS: TestStateReadErrorIsNeverOverwritten`, `--- PASS: TestLanesSetSaveFailureIsApplyFailed`, затем `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 13: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/ ./internal/pipe/ ./internal/store/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё три `ok` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 14: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/store helper/internal/pipe helper/internal/engine
git -C C:\code\router commit -m "fix(helper): keep state.json and restore.json on read errors, keep accepting pipe clients" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 2: Оставшееся устройство Wintun

Приложение A, проверки плана 1B: любое принудительное завершение ядра (сработал сторож, служебный процесс завершил ядро через 5 секунд после штатного сигнала, пользователь завершил `sing-box.exe` в диспетчере задач) оставляет устройство Wintun `SWD\WINTUN\{GUID}` адаптера `SplitLane` в системе неподключённым. В двух из четырёх следующих запусков ядро при таком устройстве висело примерно 15 секунд и завершалось ошибкой `configure tun interface: (create adapter: Cannot create a file when that file already exists. | open existing adapter: Element not found.)`; пока оно висело, его собственные соединения получали `WSAEACCES`. Штатная остановка ядра устройство удаляет. Решение плана (5.3, шаг 4): перед каждым запуском ядра служебный процесс удаляет неподключённое устройство с идентификатором экземпляра своего адаптера и не трогает никакое другое.

GUID адаптера sing-tun выводит из имени (`tun_windows.go`, `generateGUIDByDeviceName`): MD5 от `"wintun"` и имени, 16 байт читаются как `windows.GUID` (поля Data1–Data3 в порядке little-endian); идентификатор экземпляра — `SWD\WINTUN\{GUID}`, класс устройств Net. Решение «какое устройство удалить» отделено от вызовов Windows интерфейсом `devices` и проверяется на подмене. Настоящий список — SetupAPI с перечислителем `SWD\Wintun`: неподключённое устройство есть в списке без `DIGCF_PRESENT` и отсутствует в списке с ним. Удаление — `DIF_REMOVE` во всех профилях оборудования, как «Удалить устройство» в диспетчере устройств; оно требует прав администратора, поэтому тест без прав только читает список (`SPLITLANE_LIVE_READONLY=1`). Движок вызывает удаление в `startCore` сразу после `sing-box check` и перед `Start`, то есть и перед повтором после выхода ядра до строки запуска. Ошибка удаления записывается в журнал и запуск не останавливает: ядро может подхватить устройство само, а повтор с новыми портами остаётся.

**Files:**
- Create: `helper/internal/winhost/adapter.go`, `helper/internal/winhost/adapter_test.go`
- Modify: `helper/internal/engine/ports.go`, `helper/internal/engine/enable.go`, `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`
- Test: `helper/internal/winhost/adapter_test.go`, `helper/internal/engine/enable_test.go`, `helper/internal/engine/fakes_test.go`

**Interfaces:**
- Consumes: `coreconfig.AdapterName`; из `golang.org/x/sys/windows` v0.49.0 — `SetupDiGetClassDevsEx`, `DevInfo.EnumDeviceInfo`, `DevInfo.DeviceInstanceID`, `DevInfo.SetClassInstallParams`, `DevInfo.CallClassInstaller`, `MakeClassInstallHeader`, `RemoveDeviceParams`, `DIF_REMOVE`, `DI_REMOVEDEVICE_GLOBAL`, `DIGCF_PRESENT`, `GUID.String`.
- Produces (пакет `winhost`): `func AdapterGUID(name string) windows.GUID`; `func AdapterInstanceID(name string) string`; `type WintunDevice struct{ InstanceID string; Present bool }`; `func (Host) RemoveStaleAdapter() (bool, error)` (приёмник по значению, как у остальных методов `Host`: `main.go` передаёт `winhost.Host{}`, а метод доступен и у `*Host`). Пакет `engine`: метод интерфейса `Host` — `RemoveStaleAdapter() (removed bool, err error)`; в тестах — метод `fakeHost.RemoveStaleAdapter` с событием `host:removestale`, поля `fakeHost.staleAdapter bool` и `fakeHost.removeErr error`, помощник `logEvents(h *harness)` (строки журнала движка — события `log:<строка>`).

- [ ] **Step 1: Написать тесты GUID и решения**

Файл `helper/internal/winhost/adapter_test.go`, целиком:

```go
package winhost

import (
	"os"
	"strings"
	"testing"

	"splitlane/helper/internal/coreconfig"
)

// Both vectors were read on the PC: the instance ids of the devices sing-tun created for these names.
func TestAdapterGUIDVectors(t *testing.T) {
	for name, want := range map[string]string{
		"SplitLane":   "{5C76B3EA-99F5-8F54-B2FD-465779C01F24}",
		"RouterSpike": "{5A9409FA-A8AE-9CFA-F352-D298E1BD5312}",
	} {
		if got := AdapterGUID(name).String(); got != want {
			t.Errorf("%s: GUID %s, want %s", name, got, want)
		}
		if got := AdapterInstanceID(name); got != `SWD\WINTUN\`+want {
			t.Errorf("%s: instance id %s", name, got)
		}
	}
}

// fakeDevices is the device list of the decision; removed records every removal it was asked for.
type fakeDevices struct {
	list      []WintunDevice
	listErr   error
	removeErr error
	removed   []string
}

func (f *fakeDevices) Wintun() ([]WintunDevice, error) { return f.list, f.listErr }

func (f *fakeDevices) Remove(instanceID string) error {
	f.removed = append(f.removed, instanceID)
	return f.removeErr
}

// Only a non-present device with the instance id of our adapter goes; nothing else is touched.
func TestStaleDecision(t *testing.T) {
	ours, other := AdapterInstanceID("SplitLane"), AdapterInstanceID("RouterSpike")
	for name, c := range map[string]struct {
		list []WintunDevice
		gone string // the device that must be removed, "" for none
	}{
		"present: a core runs":        {list: []WintunDevice{{InstanceID: ours, Present: true}}},
		"absent":                      {list: []WintunDevice{{InstanceID: other, Present: true}}},
		"non-present, ours":           {list: []WintunDevice{{InstanceID: other, Present: true}, {InstanceID: ours}}, gone: ours},
		"non-present, another's":      {list: []WintunDevice{{InstanceID: other}}},
		"no Wintun device at all":     {},
		"non-present, ours, any case": {list: []WintunDevice{{InstanceID: strings.ToLower(ours)}}, gone: strings.ToLower(ours)},
	} {
		d := &fakeDevices{list: c.list}
		removed, err := removeStale(d, "SplitLane")
		if err != nil || removed != (c.gone != "") || strings.Join(d.removed, ",") != c.gone {
			t.Errorf("%s: removed %v, err %v, removals %v", name, removed, err, d.removed)
		}
	}
	d := &fakeDevices{list: []WintunDevice{{InstanceID: ours}}, removeErr: os.ErrPermission}
	if removed, err := removeStale(d, "SplitLane"); removed || err == nil || !strings.Contains(err.Error(), ours) {
		t.Fatalf("a failed removal: removed %v, err %v", removed, err)
	}
	d = &fakeDevices{listErr: os.ErrPermission}
	if removed, err := removeStale(d, "SplitLane"); removed || err == nil || len(d.removed) != 0 {
		t.Fatalf("a failed listing: removed %v, err %v, removals %v", removed, err, d.removed)
	}
}

// TestLiveWintunListReadOnly lists the Wintun devices of the real system and removes nothing: the
// decision runs on the real list with the fake removal. It runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveWintunListReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	list, err := setupAPI{}.Wintun()
	if err != nil {
		t.Fatalf("wintun devices: %v", err)
	}
	for _, d := range list {
		t.Logf("LIVE wintun %s present=%v", d.InstanceID, d.Present)
	}
	removed, err := removeStale(&fakeDevices{list: list}, coreconfig.AdapterName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LIVE wintun %d devices, stale device of %s: %v", len(list), coreconfig.AdapterName, removed)
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/winhost/ -run 'TestAdapterGUIDVectors|TestStaleDecision|TestLiveWintunListReadOnly' -count=1
```

Ожидается: `FAIL	splitlane/helper/internal/winhost [build failed]` и `undefined: AdapterGUID`.

- [ ] **Step 3: Реализовать GUID, решение и SetupAPI**

Файл `helper/internal/winhost/adapter.go`, целиком:

```go
package winhost

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
)

// netClass is GUID_DEVCLASS_NET, the device class of network adapters.
var netClass = windows.GUID{Data1: 0x4d36e972, Data2: 0xe325, Data3: 0x11ce, Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18}}

// wintunEnumerator is the enumerator of the devices Wintun creates (software devices).
const wintunEnumerator = `SWD\Wintun`

// AdapterGUID is the GUID sing-tun asks Wintun for when it creates the adapter of this name: the
// MD5 of "wintun" and the name, its 16 bytes read as a GUID (sing-tun, generateGUIDByDeviceName).
func AdapterGUID(name string) windows.GUID {
	sum := md5.Sum([]byte("wintun" + name))
	return windows.GUID{
		Data1: binary.LittleEndian.Uint32(sum[0:4]),
		Data2: binary.LittleEndian.Uint16(sum[4:6]),
		Data3: binary.LittleEndian.Uint16(sum[6:8]),
		Data4: [8]byte(sum[8:16]),
	}
}

// AdapterInstanceID is the device instance id of that adapter: SWD\WINTUN\{GUID}.
func AdapterInstanceID(name string) string {
	return `SWD\WINTUN\` + AdapterGUID(name).String()
}

// WintunDevice is one network device that Wintun created. Present is false for a device Windows
// keeps although nothing runs it: a forced end of the core leaves its device so.
type WintunDevice struct {
	InstanceID string
	Present    bool
}

// devices is the device list of Windows as removeStale uses it; the tests use a fake.
type devices interface {
	Wintun() ([]WintunDevice, error)
	Remove(instanceID string) error
}

// removeStale removes the device of the adapter name when it is there and not present, and
// nothing else: a present device belongs to a running core, any other device to another program.
func removeStale(d devices, name string) (bool, error) {
	list, err := d.Wintun()
	if err != nil {
		return false, err
	}
	id := AdapterInstanceID(name)
	for _, dev := range list {
		if !strings.EqualFold(dev.InstanceID, id) {
			continue
		}
		if dev.Present {
			return false, nil
		}
		if err := d.Remove(dev.InstanceID); err != nil {
			return false, fmt.Errorf("remove %s: %w", dev.InstanceID, err)
		}
		return true, nil
	}
	return false, nil
}

// RemoveStaleAdapter removes the Wintun device of the core's adapter that a forced end of an
// earlier core left behind, non-present: with it, the next core may fail to create its adapter
// (spec 5.3 step 4; appendix A, checks of plan 1B). The removal needs administrator rights.
func (Host) RemoveStaleAdapter() (bool, error) {
	return removeStale(setupAPI{}, coreconfig.AdapterName)
}

// setupAPI is the device list of Windows (SetupAPI).
type setupAPI struct{}

// Wintun lists the Wintun devices, present or not; present are those a listing with
// DIGCF_PRESENT also has.
func (setupAPI) Wintun() ([]WintunDevice, error) {
	all, err := wintunIDs(0)
	if err != nil {
		return nil, err
	}
	present, err := wintunIDs(windows.DIGCF_PRESENT)
	if err != nil {
		return nil, err
	}
	out := make([]WintunDevice, 0, len(all))
	for _, id := range all {
		d := WintunDevice{InstanceID: id}
		for _, p := range present {
			d.Present = d.Present || strings.EqualFold(p, id)
		}
		out = append(out, d)
	}
	return out, nil
}

// Remove removes the Wintun device with this instance id, present or not, as Device Manager
// uninstalls a device: DIF_REMOVE in all hardware profiles.
func (setupAPI) Remove(instanceID string) error {
	set, err := windows.SetupDiGetClassDevsEx(&netClass, wintunEnumerator, 0, 0, 0, "")
	if err != nil {
		return err
	}
	defer set.Close()
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return fmt.Errorf("no device %s", instanceID)
		}
		if err != nil {
			return err
		}
		if id, err := set.DeviceInstanceID(data); err != nil || !strings.EqualFold(id, instanceID) {
			continue
		}
		params := windows.RemoveDeviceParams{
			ClassInstallHeader: *windows.MakeClassInstallHeader(windows.DIF_REMOVE),
			Scope:              windows.DI_REMOVEDEVICE_GLOBAL,
		}
		if err := set.SetClassInstallParams(data, &params.ClassInstallHeader, uint32(unsafe.Sizeof(params))); err != nil {
			return err
		}
		return set.CallClassInstaller(windows.DIF_REMOVE, data)
	}
}

// wintunIDs lists the instance ids of the Wintun devices of the Net class; with DIGCF_PRESENT
// only the present ones.
func wintunIDs(flags windows.DIGCF) ([]string, error) {
	set, err := windows.SetupDiGetClassDevsEx(&netClass, wintunEnumerator, 0, flags, 0, "")
	if err != nil {
		return nil, err
	}
	defer set.Close()
	var ids []string
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return ids, nil
		}
		if err != nil {
			return nil, err
		}
		id, err := set.DeviceInstanceID(data)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/winhost/; go test ./internal/winhost/ -run 'TestAdapterGUIDVectors|TestStaleDecision|TestLiveWintunListReadOnly' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'
```

Ожидается:

```text
--- PASS: TestAdapterGUIDVectors (0.00s)
--- PASS: TestStaleDecision (0.00s)
--- SKIP: TestLiveWintunListReadOnly (0.00s)
ok  	splitlane/helper/internal/winhost
```

- [ ] **Step 5: Прочитать устройства Wintun настоящей системы (только чтение)**

Тест только перечисляет устройства через SetupAPI и прогоняет решение на настоящем списке с подменой удаления; система не меняется, прав администратора не нужно.

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_LIVE_READONLY='1'; go test -count=1 -v -run TestLiveWintunListReadOnly ./internal/winhost/ | Select-String -CaseSensitive -Pattern 'LIVE |^(--- |ok|FAIL)'; $env:SPLITLANE_LIVE_READONLY=''
```

Ожидается по строке `LIVE wintun SWD\WINTUN\{<GUID>} present=<true|false>` на каждое устройство Wintun, затем строка с числом устройств и признаком оставшегося устройства `SplitLane`, `--- PASS` и `ok`. При подготовке плана на ПК было так (первое — адаптер системного Tailscale, его GUID задан в коде Tailscale; второе — неподключённое устройство адаптера `RouterSpike` проверок плана 0: имя другое, служебный процесс его не трогает):

```text
    adapter_test.go:82: LIVE wintun SWD\WINTUN\{37217669-42DA-4657-A55B-0D995D328250} present=true
    adapter_test.go:82: LIVE wintun SWD\WINTUN\{5A9409FA-A8AE-9CFA-F352-D298E1BD5312} present=false
    adapter_test.go:88: LIVE wintun 2 devices, stale device of SplitLane: false
--- PASS: TestLiveWintunListReadOnly (0.01s)
ok  	splitlane/helper/internal/winhost
```

`stale device of SplitLane: true` — после принудительного завершения ядра осталось устройство `SWD\WINTUN\{5C76B3EA-99F5-8F54-B2FD-465779C01F24}`; это не сбой шага: его удалит служебный процесс перед следующим запуском ядра (задача 16). `--- FAIL` с `wintun devices: ...` — SetupAPI не прочитан: шаг не пройден.

- [ ] **Step 6: Написать тест движка**

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
	otherCore    bool
	adapterStuck bool
}
```

Заменить на:

```go
	otherCore    bool
	adapterStuck bool
	staleAdapter bool  // a forced end of a core left its Wintun device behind; the next removal takes it
	removeErr    error // the removal of the stale Wintun device fails
}
```

Найти:

```go
func (h *fakeHost) Secret() string { return "0123456789abcdef0123456789abcdef" }
```

Заменить на:

```go
func (h *fakeHost) Secret() string { return "0123456789abcdef0123456789abcdef" }

func (h *fakeHost) RemoveStaleAdapter() (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rec.add("host:removestale")
	if h.removeErr != nil {
		return false, h.removeErr
	}
	removed := h.staleAdapter
	h.staleAdapter = false
	return removed, nil
}
```

Изменить `helper/internal/engine/enable_test.go`:

Найти:

```go
	if on, _ := systemExitNode(h); on {
		t.Fatalf("the second run must have switched the system exit node off: %v", h.rec.list())
	}
}
```

Заменить на:

```go
	if on, _ := systemExitNode(h); on {
		t.Fatalf("the second run must have switched the system exit node off: %v", h.rec.list())
	}
}

// logEvents makes each log line of the engine an event "log:<line>" of the recorder.
func logEvents(h *harness) {
	h.e.d.Log = func(format string, args ...any) { h.rec.add("log:"+format, args...) }
}

// 5.3 step 4 and appendix A (checks of plan 1B): the Wintun device that a forced end of the core
// left behind goes right before every start of the core, after its check. A failed removal is
// logged and the start goes on: the core may still take the device, an early exit is tried again.
func TestStaleAdapterIsRemovedBeforeStart(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.host.staleAdapter = true
	h.core.dieEarly = 1
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	var order []string
	for _, ev := range h.rec.list() {
		if ev == "core:check" || ev == "host:removestale" || strings.HasPrefix(ev, "core:start") {
			order = append(order, strings.Fields(ev)[0])
		}
	}
	if got := strings.Join(order, ","); got != "core:check,host:removestale,core:start,core:check,host:removestale,core:start" {
		t.Fatalf("order %s: %v", got, h.rec.list())
	}
	if n := h.rec.count("log:removed the stale Wintun device of the adapter"); n != 1 {
		t.Fatalf("removal logged %d times: %v", n, h.rec.list())
	}

	h = newHarness(t)
	logEvents(h)
	h.host.removeErr = errBoom
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	remove, start := h.rec.index("host:removestale"), h.rec.index("core:start")
	if remove < 0 || start < remove || h.rec.count("log:stale Wintun device of the adapter not removed: boom") != 1 {
		t.Fatalf("a failed removal is logged and the start goes on: %v", h.rec.list())
	}
}
```

- [ ] **Step 7: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestStaleAdapterIsRemovedBeforeStart' -count=1
```

Ожидается: `--- FAIL: TestStaleAdapterIsRemovedBeforeStart` со строкой `order core:check,core:start,core:check,core:start: [...]` (движок ещё не вызывает удаление; пакет собирается, потому что лишний метод подмены интерфейсу не мешает).

- [ ] **Step 8: Реализовать вызов перед запуском ядра**

Изменить `helper/internal/engine/ports.go`:

Найти:

```go
	FreePorts(n int) ([]int, error)
	Secret() string
}
```

Заменить на:

```go
	FreePorts(n int) ([]int, error)
	Secret() string
	// RemoveStaleAdapter removes the Wintun device of the core's adapter that a forced end of an
	// earlier core left behind, non-present (5.3 step 4); removed tells that there was one.
	RemoveStaleAdapter() (removed bool, err error)
}
```

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
	if err := e.d.Core.Check(ctx, path); err != nil {
		return invalid(err)
	}
	proc, err := e.d.Core.Start(path)
```

Заменить на:

```go
	if err := e.d.Core.Check(ctx, path); err != nil {
		return invalid(err)
	}
	// A forced end of an earlier core leaves its Wintun device behind, non-present, and the next core
	// may then fail to create its adapter (appendix A, checks of plan 1B). A failed removal does not
	// stop the start: the core may still take the device, and an early exit is tried again.
	if removed, err := e.d.Host.RemoveStaleAdapter(); err != nil {
		e.d.Log("stale Wintun device of the adapter not removed: %v", err)
	} else if removed {
		e.d.Log("removed the stale Wintun device of the adapter")
	}
	proc, err := e.d.Core.Start(path)
```

`winhost.Host` уже реализует новый метод (шаг 3), `main.go` не меняется: `var _ engine.Host = Host{}` в `host.go` проверяет это при сборке.

- [ ] **Step 9: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go build ./...; go test ./internal/engine/ -run 'TestStaleAdapterIsRemovedBeforeStart' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'
```

Ожидается:

```text
--- PASS: TestStaleAdapterIsRemovedBeforeStart (0.01s)
ok  	splitlane/helper/internal/engine
```

- [ ] **Step 10: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не печатают; `ok` у каждого пакета (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost` и пакеты, добавленные задачами до этой); последняя строка — `ok  	splitlane/helper/internal/engine` пятикратного прогона.

- [ ] **Step 11: Записать решение в спецификацию**

Изменить `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`:

Найти:

```markdown
без ожидания шага 5, с причиной `login_expired`; признак «вход выполнен» снимается.
```

Заменить на:

```markdown
без ожидания шага 5, с причиной `login_expired`; признак «вход выполнен» снимается. Перед запуском служебный процесс удаляет неподключённое устройство Wintun с именем своего адаптера, если оно осталось после принудительного завершения ядра (приложение A, проверки плана 1B).
```

- [ ] **Step 12: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/winhost/adapter.go helper/internal/winhost/adapter_test.go helper/internal/engine/ports.go helper/internal/engine/enable.go helper/internal/engine/enable_test.go helper/internal/engine/fakes_test.go docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "feat(helper): remove the stale Wintun device of the adapter before each core start" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 3: Служба API ядра и конфиг режима настройки

Раздел 7: основной источник данных о встроенном узле — служба API ядра 1.14 (решение 2 по итогам плана 0), а вход выполняется в режиме настройки — ядро без виртуального адаптера. В плане 0 (приложение A, (б), (д), (з)) службу опрашивал собственный клиент ядра: `sing-box api tailscale status`, `exit-node list`, `ping`, `logout`. Служба отвечает и в режиме настройки, и в обычном, отдаёт ссылку для входа, состояние узла, список exit node с признаком «в сети» и вид соединения, отклоняет неверный секрет; истечение входа видно как состояние `NeedsLogin`. Эта задача даёт драйверу две вещи. Первая — конфиг режима настройки `coreconfig.BuildSetup` в той форме, что прошла проверки плана 0 (`p0-setup.json`): журнал уровня `info` в файл журнала ядра, один сервер DNS `local`, узел `vpn` без `exit_node`, прямой выход `direct`, `route.default_domain_resolver`, служба `api` на `127.0.0.1`; ни входов, ни `tun`, ни `route.auto_detect_interface`, ни `clash_api`. Вторая — клиент службы API `singbox.Node`. Адрес и секрет службы клиент передаёт ядру в переменных окружения `BOX_API_URL` и `BOX_API_SECRET`, а не флагами `--url` и `--secret`: командную строку процесса видит любой пользователь компьютера, а секрет известен только служебному процессу (6.1). Автомат получает интерфейс `NodeAPI`, типы `NodeState` и `SetupOptions`, два метода `CoreDriver` и константы `protocol.Backend*`, `protocol.Path*`, но новыми методами пока не пользуется — это задачи 8 и 9; для их тестов здесь появляются подмены `fakeNode` и `fakeCore.Node`.

Разборщики вывода написаны по исходникам sing-box 1.14.4 (`cmd/sing-box/cmd_api_tailscale_status.go`, `cmd_api_tailscale_exit_node_list.go`, `cmd_api_tailscale_peer.go`, `cmd_api_tailscale_peer_list.go`, `cmd_api_tailscale_ping.go`, `cmd_api_output.go`). `status` печатает тег узла и блок строк `  <метка>:` с выровненными значениями (`  State:    Running`, `  Log in:  https://…`); пустое значение печатается как `-`. Адресов узла в выводе `status` нет, поэтому признак «узел получил адрес в сети Tailscale» (раздел 7) берётся из `peer list`: его первая строка — всегда сам узел (Tailscale заполняет `Self` в любом состоянии), второй столбец — первый из адресов узла или `-`. Таблицы `peer list` и `exit-node list`, когда вывод идёт не в терминал, — строки с ячейками через табуляцию, пустая ячейка — `-`. Строка exit node без адреса IPv4 пропускается (`exitNodes[].id` — адрес IPv4, раздел 7); имя — первая метка DNS-имени (`exit-a.example.ts.net` → `exit-a`). `ping` печатает до десяти строк `pong from <имя> (<адрес>) via <путь> in <время>` и останавливается на первом прямом ответе; вид соединения определяется по последней строке: `DERP(…)` и `peer relay …` — `relay`, `<адрес>:<порт>` — `direct`, ни одной строки — `""` (неизвестно). Состояния узла (`ipn.State` Tailscale): `NeedsLogin` → `needs_login`, `NeedsMachineAuth` → `needs_approval`, `Starting` → `starting`, `Running` → `running`, остальные (`Stopped`, `NoState`, `InUseOtherUser`, `-`) → `stopped`; значение `expired` выводит сам автомат (задача 9). Ненулевой код выхода ядра — ошибка с текстом его stderr без цветовых кодов, которыми ядро красит строку `FATAL`.

**Files:**
- Create: `helper/internal/protocol/tailscale.go`, `helper/internal/coreconfig/setup.go`, `helper/internal/coreconfig/testdata/setup.golden.json`, `helper/internal/singbox/node.go`
- Modify: `helper/internal/engine/ports.go`, `helper/internal/engine/fakes_test.go`, `helper/internal/singbox/driver.go`, `helper/internal/singbox/driver_test.go`
- Test: `helper/internal/coreconfig/setup_test.go`, `helper/internal/singbox/node_test.go`

**Interfaces:**
- Consumes: `coreconfig.PathsFor`, `coreconfig.TagVPN`, `coreconfig.TagDirect`, `protocol.ExitNode`, `singbox.hidden` (`CREATE_NO_WINDOW`), `engine.CoreDriver` (план 1A); в тестах пакета `singbox` — `fakeDriver` и `TestMain` (тестовый бинарник в роли ядра).
- Produces:
  - `protocol`: `const BackendStopped = "stopped"`, `BackendNeedsLogin = "needs_login"`, `BackendStarting = "starting"`, `BackendRunning = "running"`, `BackendNeedsApproval = "needs_approval"`, `BackendExpired = "expired"`, `PathDirect = "direct"`, `PathRelay = "relay"`.
  - `engine`: `type SetupOptions struct{ Hostname string; APIPort int; APISecret string }`; `type NodeState struct{ Backend string; AuthURL string; HasAddress bool }`; `type NodeAPI interface{ State(ctx context.Context) (NodeState, error); ExitNodes(ctx context.Context) ([]protocol.ExitNode, error); Path(ctx context.Context, exitNode string) (string, error); Logout(ctx context.Context) error }`; новые методы `CoreDriver` (после `Clash`): `BuildSetupConfig(o SetupOptions) ([]byte, error)`, `Node(port int, secret string) NodeAPI`.
  - `coreconfig`: `type SetupOptions struct{ DataDir, Hostname string; APIPort int; APISecret string }`; `func BuildSetup(o SetupOptions) ([]byte, error)` (абсолютный каталог данных, непустое имя, порт 1..65535, секрет не короче 16 символов).
  - `singbox`: `type Runner func(ctx context.Context, env []string, exe string, args ...string) ([]byte, error)`; `type Node struct{ Exe, URL, Secret string; Run Runner }` (реализует `engine.NodeAPI`); `func NewNode(exe string, port int, secret string) *Node`; `func (d *Driver) BuildSetupConfig(o engine.SetupOptions) ([]byte, error)`; `func (d *Driver) Node(port int, secret string) engine.NodeAPI`.
  - Подмены автомата (`fakes_test.go`): `type fakeNode struct{ mu sync.Mutex; rec *recorder; state NodeState; nodes []protocol.ExitNode; path string; err error; logouts int }`; `newFakeNode(rec *recorder) *fakeNode` — по умолчанию `running`, адрес есть, в списке `{exitNode, "exit-a", true}`, путь `direct`; `Logout` пишет `node:logout`, увеличивает `logouts` и при успехе оставляет `needs_login` без ссылки и адреса; `err` возвращают все четыре метода; `(*fakeNode).set(func(n *fakeNode))`. Поля `fakeCore.node` (`h.core.node`, один на все запуски) и `fakeCore.setupOpts`; `fakeCore.Node` возвращает `h.core.node`; `fakeCore.BuildSetupConfig` пишет `core:setupconfig` и запоминает параметры.

- [ ] **Step 1: Написать тесты конфига режима настройки**

Файл `helper/internal/coreconfig/setup_test.go`, целиком:

```go
package coreconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func testSetupOptions(dataDir string) SetupOptions {
	return SetupOptions{
		DataDir:   dataDir,
		Hostname:  "PC-splitlane",
		APIPort:   30004,
		APISecret: "fedcba9876543210fedcba9876543210",
	}
}

func TestSetupConfigMatchesGolden(t *testing.T) {
	got, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "setup.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("config differs from testdata/setup.golden.json:\n%s", got)
	}
}

// Spec 7: no adapter and no route.auto_detect_interface in the setup mode; the node has no exit
// node there, and only the API service listens (no probes, no Clash API).
func TestSetupConfigHasNoAdapter(t *testing.T) {
	b, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if _, ok := c["inbounds"]; ok {
		t.Fatal("the setup mode has no inbounds: no tun adapter, no probes")
	}
	if _, ok := c["experimental"]; ok {
		t.Fatal("the setup mode has no clash_api")
	}
	route := c["route"].(map[string]any)
	if _, ok := route["auto_detect_interface"]; ok {
		t.Fatal("route.auto_detect_interface must not be set in the setup mode")
	}
	if route["default_domain_resolver"] != "dns-local" {
		t.Fatalf("route %v", route)
	}
	node := c["endpoints"].([]any)[0].(map[string]any)
	if _, ok := node["exit_node"]; ok || node["type"] != "tailscale" || node["tag"] != TagVPN {
		t.Fatalf("node %v", node)
	}
	api := c["services"].([]any)[0].(map[string]any)
	if api["type"] != "api" || api["listen"] != "127.0.0.1" || api["listen_port"] != float64(30004) {
		t.Fatalf("api service %v", api)
	}
}

func TestSetupConfigRejectsBadOptions(t *testing.T) {
	bad := []func(*SetupOptions){
		func(o *SetupOptions) { o.DataDir = `SplitLane` },
		func(o *SetupOptions) { o.Hostname = "" },
		func(o *SetupOptions) { o.APIPort = 0 },
		func(o *SetupOptions) { o.APIPort = 65536 },
		func(o *SetupOptions) { o.APISecret = "short" },
	}
	for i, change := range bad {
		o := testSetupOptions(`C:\ProgramData\SplitLane`)
		change(&o)
		if _, err := BuildSetup(o); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
}

// TestSetupConfigPassesCheck runs `sing-box check` when SPLITLANE_CORE names sing-box.exe.
// The check reads the config only; it starts nothing.
func TestSetupConfigPassesCheck(t *testing.T) {
	core := os.Getenv("SPLITLANE_CORE")
	if core == "" {
		t.Skip("SPLITLANE_CORE is not set")
	}
	dir := t.TempDir()
	paths := PathsFor(dir)
	for _, d := range []string{paths.LogsDir, paths.StateDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := BuildSetup(testSetupOptions(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(core, "check", "-c", paths.Config, "-D", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("sing-box check: %v\n%s", err, out)
	}
}
```

Файл `helper/internal/coreconfig/testdata/setup.golden.json`, целиком:

```json
{
  "dns": {
    "servers": [
      {
        "tag": "dns-local",
        "type": "local"
      }
    ]
  },
  "endpoints": [
    {
      "hostname": "PC-splitlane",
      "state_directory": "C:/ProgramData/SplitLane/tailscale",
      "tag": "vpn",
      "type": "tailscale"
    }
  ],
  "log": {
    "level": "info",
    "output": "C:/ProgramData/SplitLane/logs/core.log",
    "timestamp": true
  },
  "outbounds": [
    {
      "tag": "direct",
      "type": "direct"
    }
  ],
  "route": {
    "default_domain_resolver": "dns-local"
  },
  "services": [
    {
      "listen": "127.0.0.1",
      "listen_port": 30004,
      "secret": "fedcba9876543210fedcba9876543210",
      "tag": "api",
      "type": "api"
    }
  ]
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/coreconfig/ -count=1
```

Ожидается: `FAIL	splitlane/helper/internal/coreconfig [build failed]` и строки `internal\coreconfig\setup_test.go:12:39: undefined: SetupOptions`, `internal\coreconfig\setup_test.go:22:14: undefined: BuildSetup`.

- [ ] **Step 3: Реализовать конфиг режима настройки**

Файл `helper/internal/coreconfig/setup.go`, целиком:

```go
package coreconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// SetupOptions are the inputs of the setup-mode config (spec 7).
type SetupOptions struct {
	DataDir   string // %ProgramData%\SplitLane or the development data folder
	Hostname  string // <PC>-splitlane
	APIPort   int    // the API service of this run (spec 6.1)
	APISecret string
}

// BuildSetup returns the config of the setup mode (spec 7) as indented JSON, in the shape that
// Plan 0 checked ((б), (д), (з)): the embedded node (no exit node), the local DNS server,
// route.default_domain_resolver and the API service. There is no adapter and no
// route.auto_detect_interface: without an adapter with strict_route the block of the system
// Tailscale would not let the core out through the physical adapter, so the node reaches the
// control server by the ordinary route of the system. No probes and no Clash API either.
func BuildSetup(o SetupOptions) ([]byte, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	config := obj{
		"log": obj{"level": "info", "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{"servers": []any{obj{"type": "local", "tag": "dns-local"}}},
		"endpoints": []any{obj{
			"type":            "tailscale",
			"tag":             TagVPN,
			"state_directory": filepath.ToSlash(p.StateDir),
			"hostname":        o.Hostname,
		}},
		"outbounds": []any{obj{"type": "direct", "tag": TagDirect}},
		"route":     obj{"default_domain_resolver": "dns-local"},
		"services": []any{obj{
			"type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": o.APIPort, "secret": o.APISecret,
		}},
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// validate checks what Build checks for the same fields, with the same words.
func (o SetupOptions) validate() error {
	switch {
	case o.DataDir == "" || !filepath.IsAbs(o.DataDir):
		return fmt.Errorf("coreconfig: data folder must be an absolute path")
	case o.Hostname == "":
		return fmt.Errorf("coreconfig: hostname is empty")
	case o.APIPort <= 0 || o.APIPort > 65535:
		return fmt.Errorf("coreconfig: the API port must be a number 1..65535")
	case len(o.APISecret) < 16:
		return fmt.Errorf("coreconfig: secrets must be at least 16 characters")
	}
	return nil
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test ./internal/coreconfig/ -run 'TestSetup' -count=1 -v
```

Ожидается:

```text
--- PASS: TestSetupConfigMatchesGolden (0.00s)
--- PASS: TestSetupConfigHasNoAdapter (0.00s)
--- PASS: TestSetupConfigRejectsBadOptions (0.00s)
--- PASS: TestSetupConfigPassesCheck (0.07s)
ok  	splitlane/helper/internal/coreconfig
```

`TestSetupConfigPassesCheck` запускает настоящее ядро только командой `sing-box check`; без `SPLITLANE_CORE` он пропускается (`SKIP`).

- [ ] **Step 5: Написать подмены службы API для тестов автомата**

Автомат новыми методами пока не пользуется; подмены нужны задачам 8 и 9. Изменения касаются только подмены ядра.

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
	onStart   func(p *fakeProc)
	opts      CoreOptions // of the last BuildConfig
}
```

Заменить на:

```go
	onStart   func(p *fakeProc)
	opts      CoreOptions  // of the last BuildConfig
	setupOpts SetupOptions // of the last BuildSetupConfig
	node      *fakeNode    // the API service of every run
}
```

Найти:

```go
func (c *fakeCore) Clash(port int, secret string) ClashAPI { return c.clash }
```

Заменить на:

```go
func (c *fakeCore) Clash(port int, secret string) ClashAPI { return c.clash }

func (c *fakeCore) BuildSetupConfig(o SetupOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rec.add("core:setupconfig")
	c.setupOpts = o
	return json.Marshal(o)
}

func (c *fakeCore) Node(port int, secret string) NodeAPI { return c.node }

// fakeNode is the API service of the core; all runs share it (h.core.node). By default the node
// is logged in and running, with its address, the harness's exit node in the list and a direct
// path. A logout that succeeds leaves it at needs_login without a link and without an address.
type fakeNode struct {
	mu      sync.Mutex
	rec     *recorder
	state   NodeState
	nodes   []protocol.ExitNode
	path    string
	err     error // every call fails with it
	logouts int
}

func newFakeNode(rec *recorder) *fakeNode {
	return &fakeNode{
		rec:   rec,
		state: NodeState{Backend: protocol.BackendRunning, HasAddress: true},
		nodes: []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}},
		path:  protocol.PathDirect,
	}
}

func (n *fakeNode) State(ctx context.Context) (NodeState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return NodeState{}, n.err
	}
	return n.state, nil
}

func (n *fakeNode) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return nil, n.err
	}
	return append([]protocol.ExitNode{}, n.nodes...), nil
}

func (n *fakeNode) Path(ctx context.Context, exitNode string) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return "", n.err
	}
	return n.path, nil
}

func (n *fakeNode) Logout(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rec.add("node:logout")
	n.logouts++
	if n.err != nil {
		return n.err
	}
	n.state = NodeState{Backend: protocol.BackendNeedsLogin}
	return nil
}

func (n *fakeNode) set(f func(n *fakeNode)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	f(n)
}
```

Найти:

```go
	h.core = &fakeCore{rec: h.rec, clash: h.clash}
```

Заменить на:

```go
	h.core = &fakeCore{rec: h.rec, clash: h.clash, node: newFakeNode(h.rec)}
```

- [ ] **Step 6: Запустить тесты автомата, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -count=1
```

Ожидается: `FAIL	splitlane/helper/internal/engine [build failed]` и строки вида `internal\engine\fakes_test.go:<строка>:<столбец>: undefined: SetupOptions`, `... undefined: NodeAPI`, `... undefined: NodeState`, `... undefined: protocol.BackendRunning`, `... undefined: protocol.PathDirect`.

- [ ] **Step 7: Добавить константы протокола и интерфейс службы API**

Файл `helper/internal/protocol/tailscale.go`, целиком:

```go
package protocol

// Values of tailscale.backend and tailscale.path (spec 5.2). BackendExpired is the engine's own:
// the node says needs_login while the main core runs (spec 7, «Истечение входа»).
const (
	BackendStopped       = "stopped"
	BackendNeedsLogin    = "needs_login"
	BackendStarting      = "starting"
	BackendRunning       = "running"
	BackendNeedsApproval = "needs_approval"
	BackendExpired       = "expired"

	PathDirect = "direct"
	PathRelay  = "relay"
)
```

Изменить `helper/internal/engine/ports.go`:

Найти:

```go
// CoreDriver hides the core (spec 4): build the config, apply the lanes (the rule files and the
// mode), get and close connections (ClashAPI), the connector state (Events). Stage 1 has one
// driver, for sing-box 1.14.4 (package singbox). The engine knows nothing of the core's config,
// rule-file format, mode names or log lines: a second core (stage 3) is a second driver.
```

Заменить на:

```go
// CoreDriver hides the core (spec 4): build the config, apply the lanes (the rule files and the
// mode), get and close connections (ClashAPI), the connector state (Events, NodeAPI), the config
// of the setup mode (spec 7). Stage 1 has one driver, for sing-box 1.14.4 (package singbox). The
// engine knows nothing of the core's config, rule-file format, mode names or log lines: a second
// core (stage 3) is a second driver.
```

Найти:

```go
	Clash(port int, secret string) ClashAPI
}
```

Заменить на:

```go
	Clash(port int, secret string) ClashAPI
	// BuildSetupConfig returns the config of the setup mode (spec 7): the node, local DNS, the API service; no adapter.
	BuildSetupConfig(o SetupOptions) ([]byte, error)
	// Node is the API service of a running core.
	Node(port int, secret string) NodeAPI
}
```

Найти:

```go
// SysStatus is what the system Tailscale client says (spec 5.6).
type SysStatus struct {
```

Заменить на:

```go
// SetupOptions are the values of one run of the setup mode (spec 7); the driver adds its paths.
type SetupOptions struct {
	Hostname  string
	APIPort   int
	APISecret string
}

// NodeState is what the API service says about the embedded node (spec 7, 5.2).
type NodeState struct {
	Backend    string // protocol.Backend*: stopped, needs_login, starting, running, needs_approval (never expired: the engine derives it)
	AuthURL    string // the login link, "" when none is offered
	HasAddress bool   // the node has its IPv4 address in the tailnet
}

// NodeAPI is the core's API service (spec 3, 7), one run of the core.
type NodeAPI interface {
	State(ctx context.Context) (NodeState, error)
	ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) // id = IPv4 address in the tailnet (spec 7)
	Path(ctx context.Context, exitNode string) (string, error)  // protocol.PathDirect, protocol.PathRelay or "" when unknown
	Logout(ctx context.Context) error
}

// SysStatus is what the system Tailscale client says (spec 5.6).
type SysStatus struct {
```

- [ ] **Step 8: Запустить тесты автомата, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ ./internal/protocol/ -count=1
```

Ожидается:

```text
ok  	splitlane/helper/internal/engine
ok  	splitlane/helper/internal/protocol
```

Пакет `internal/singbox` до шага 11 не собирается (`*Driver does not implement engine.CoreDriver`), поэтому `go vet ./...` здесь не запускается.

- [ ] **Step 9: Написать тесты клиента службы API**

Тесты разбора кормят клиента выводом ядра 1.14.4 в том виде, в каком его печатают исходники (значения вымышленные); `TestNodeErrorIsReturned` проверяет настоящий запуск процесса: тестовый бинарник в роли ядра получает адрес и секрет из окружения и при неверном секрете завершается с кодом 1 и строкой `FATAL` в stderr.

Файл `helper/internal/singbox/node_test.go`, целиком:

```go
package singbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/protocol"
)

// Output of `sing-box api tailscale ...` of 1.14.4 when stdout is not a terminal, as
// cmd/sing-box/cmd_api_tailscale_*.go and cmd_api_output.go print it. `status` is the endpoint tag
// and a block of "  <label>:" lines, the values padded to the longest label plus three. `peer list`
// and `exit-node list` are rows of tab-separated cells, "-" for an empty one; `peer list` starts
// with the node itself, its IP cell is the first of its tailnet addresses. Values are invented.
const (
	testAPISecret = "fedcba9876543210fedcba9876543210"

	statusRunning    = "vpn\n  State:    Running\n  Network:  example.org\n"
	peersWithAddress = "pc-splitlane.example.ts.net\t100.101.102.103\tyes\nexit-a.example.ts.net\t100.101.102.104\tyes\n"
	peersNoAddress   = "pc\t-\tno\n"
)

type apiCall struct {
	env  []string
	exe  string
	args []string
}

// scripted is a Node whose runner answers from outputs (by the arguments after "api") and keeps
// every call.
func scripted(t *testing.T, outputs map[string]string) (*Node, *[]apiCall) {
	t.Helper()
	n := NewNode(`C:\core\sing-box.exe`, 40004, testAPISecret)
	var calls []apiCall
	n.Run = func(ctx context.Context, env []string, exe string, args ...string) ([]byte, error) {
		calls = append(calls, apiCall{env: env, exe: exe, args: args})
		out, ok := outputs[strings.Join(args[1:], " ")]
		if !ok {
			return nil, fmt.Errorf("unexpected command %v", args)
		}
		return []byte(out), nil
	}
	return n, &calls
}

// fakeAPICommand plays `sing-box api` when the test binary runs as the core (TestMain): the URL
// and the secret come from the environment, as for the real CLI without --url and --secret, and a
// failure is the core's FATAL line on stderr, colored as the core colors it.
func fakeAPICommand(args []string) {
	if os.Getenv("BOX_API_URL") != "127.0.0.1:40004" || os.Getenv("BOX_API_SECRET") != testAPISecret {
		os.Stderr.WriteString("\x1b[31mFATAL\x1b[0m[0000] Unauthenticated: invalid secret\n")
		os.Exit(1)
	}
	switch strings.Join(args, " ") {
	case "tailscale status":
		os.Stdout.WriteString(statusRunning)
	case "tailscale peer list":
		os.Stdout.WriteString(peersWithAddress)
	default:
		os.Stderr.WriteString("FATAL[0000] unknown command\n")
		os.Exit(1)
	}
}

func TestNodeStateParse(t *testing.T) {
	cases := []struct {
		name, status, peers string
		want                engine.NodeState
	}{
		{"no state", "vpn\n  State:  NoState\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"stopped", "vpn\n  State:  Stopped\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"empty state", "vpn\n  State:  -\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"another user", "vpn\n  State:  InUseOtherUser\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"login link", "vpn\n  State:   NeedsLogin\n  Log in:  https://login.example/a1b2\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: "https://login.example/a1b2"}},
		{"expired, no link yet", "vpn\n  State:    NeedsLogin\n  Network:  example.org\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendNeedsLogin, HasAddress: true}},
		{"approval", "vpn\n  State:    NeedsMachineAuth\n  Network:  example.org\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendNeedsApproval}},
		{"starting without an address", "vpn\n  State:  Starting\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStarting}},
		{"starting", "vpn\n  State:    Starting\n  Network:  example.org\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendStarting, HasAddress: true}},
		{"running, every label", "vpn\n  State:    Running\n  Network:  example.org\n  Auth:     auth key\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendRunning, HasAddress: true}},
		{"no peers at all", statusRunning, "",
			engine.NodeState{Backend: protocol.BackendRunning}},
	}
	for _, c := range cases {
		n, _ := scripted(t, map[string]string{"tailscale status": c.status, "tailscale peer list": c.peers})
		got, err := n.State(context.Background())
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.name, got, err, c.want)
		}
	}
}

func TestExitNodesParse(t *testing.T) {
	n, _ := scripted(t, map[string]string{"tailscale exit-node list": "" +
		"exit-a.example.ts.net\t100.101.102.104\tyes\tselected\n" +
		"exit-b.example.ts.net\t100.101.102.105\tno\t-\n" +
		"exit-c.example.ts.net\t-\tyes\t-\n" +
		"exit-d.example.ts.net\tfd7a:115c:a1e0::5\tyes\t-\n"})
	got, err := n.ExitNodes(context.Background())
	want := []protocol.ExitNode{
		{ID: "100.101.102.104", Name: "exit-a", Online: true},
		{ID: "100.101.102.105", Name: "exit-b", Online: false},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	empty, _ := scripted(t, map[string]string{"tailscale exit-node list": ""})
	if got, err := empty.ExitNodes(context.Background()); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no exit nodes is an empty list: %#v %v", got, err)
	}
}

func TestPathParse(t *testing.T) {
	const pong = "pong from exit-a (100.101.102.104) via "
	cases := map[string]struct {
		out  string
		want string
	}{
		"derp region code": {strings.Repeat(pong+"DERP(fra) in 23ms\n", 10) + "direct connection not established\n", protocol.PathRelay},
		"derp region id":   {strings.Repeat(pong+"DERP(4) in 23ms\n", 10) + "direct connection not established\n", protocol.PathRelay},
		"peer relay": {strings.Repeat(pong+"peer relay 198.51.100.7:7777:vni:3 in 12ms\n", 10) +
			"direct connection not established, relayed by peer relay 198.51.100.7:7777:vni:3\n", protocol.PathRelay},
		"direct after derp": {pong + "DERP(fra) in 31ms\n" + pong + "198.51.100.7:41641 in 4ms\n", protocol.PathDirect},
		"nothing":           {"", ""},
	}
	for name, c := range cases {
		n, calls := scripted(t, map[string]string{"tailscale ping 100.101.102.104": c.out})
		got, err := n.Path(context.Background(), "100.101.102.104")
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", name, got, err, c.want)
		}
		if len(*calls) != 1 {
			t.Errorf("%s: calls %v", name, *calls)
		}
	}
}

// Plan 0 (б): the URL and the secret go through the environment, so the secret is never on the
// command line of a process that every user can list.
func TestNodeUsesEnvSecret(t *testing.T) {
	n, calls := scripted(t, map[string]string{
		"tailscale status": statusRunning, "tailscale peer list": peersWithAddress,
		"tailscale exit-node list": "", "tailscale ping 100.101.102.104": "", "tailscale logout": "",
	})
	ctx := context.Background()
	n.State(ctx)
	n.ExitNodes(ctx)
	n.Path(ctx, "100.101.102.104")
	if err := n.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, c := range *calls {
		commands = append(commands, strings.Join(c.args, " "))
		if c.exe != `C:\core\sing-box.exe` {
			t.Errorf("exe %s", c.exe)
		}
		if !reflect.DeepEqual(c.env, []string{"BOX_API_URL=127.0.0.1:40004", "BOX_API_SECRET=" + testAPISecret}) {
			t.Errorf("env %v", c.env)
		}
		for _, a := range c.args {
			if strings.Contains(a, testAPISecret) {
				t.Errorf("the secret is on the command line: %v", c.args)
			}
		}
	}
	want := "api tailscale status|api tailscale peer list|api tailscale exit-node list|" +
		"api tailscale ping 100.101.102.104|api tailscale logout"
	if strings.Join(commands, "|") != want {
		t.Fatalf("commands %v", commands)
	}
}

// The real runner, with the test binary as the core: the environment reaches the CLI, and a
// non-zero exit is an error with the core's message, without color codes and without the secret.
func TestNodeErrorIsReturned(t *testing.T) {
	d, _ := fakeDriver(t)
	st, err := d.Node(40004, testAPISecret).State(context.Background())
	if err != nil || st != (engine.NodeState{Backend: protocol.BackendRunning, HasAddress: true}) {
		t.Fatalf("with the run's secret: %+v %v", st, err)
	}
	const wrong = "0000000000000000wrong"
	_, err = d.Node(40004, wrong).State(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FATAL[0000] Unauthenticated: invalid secret") {
		t.Fatalf("a non-zero exit must be an error with the core's message, got %v", err)
	}
	if strings.Contains(err.Error(), "\x1b[") || strings.Contains(err.Error(), wrong) {
		t.Fatalf("color codes or the secret in the error: %q", err)
	}
	failing, _ := scripted(t, nil)
	if nodes, err := failing.ExitNodes(context.Background()); err == nil || nodes != nil {
		t.Fatalf("a failed command gives no list: %v %v", nodes, err)
	}
	if path, err := failing.Path(context.Background(), "100.101.102.104"); err == nil || path != "" {
		t.Fatalf("a failed ping gives no path: %q %v", path, err)
	}
	if err := failing.Logout(context.Background()); err == nil {
		t.Fatal("a failed logout must be an error")
	}
}

// The setup mode in the driver's data folder (spec 7): the same log and node state as the main
// config, the API service of this run.
func TestDriverSetupConfig(t *testing.T) {
	d := &Driver{Exe: `C:\core\sing-box.exe`, DataDir: t.TempDir()}
	b, err := d.BuildSetupConfig(engine.SetupOptions{Hostname: "pc-splitlane", APIPort: 40004, APISecret: testAPISecret})
	if err != nil {
		t.Fatal(err)
	}
	p := coreconfig.PathsFor(d.DataDir)
	for _, want := range []string{`"output": "` + filepath.ToSlash(p.CoreLog) + `"`,
		`"state_directory": "` + filepath.ToSlash(p.StateDir) + `"`, `"hostname": "pc-splitlane"`, `"listen_port": 40004`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("setup config lacks %s", want)
		}
	}
	if strings.Contains(string(b), `"tun"`) {
		t.Fatal("the setup mode has no adapter")
	}
}
```

Изменить `helper/internal/singbox/driver_test.go`:

Найти:

```go
// TestMain lets the test binary play the core: started with SPLITLANE_FAKE_CORE=1 it reads the
// config, writes the start line into the log the config names, and waits for CTRL_BREAK.
func TestMain(m *testing.M) {
	if os.Getenv("SPLITLANE_FAKE_CORE") == "1" {
		fakeCore(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeCore(args []string) {
	config, _ := os.ReadFile(args[2])
```

Заменить на:

```go
// TestMain lets the test binary play the core: started with SPLITLANE_FAKE_CORE=1 it reads the
// config, writes the start line into the log the config names, and waits for CTRL_BREAK; as
// `api ...` it plays the CLI of the API service (fakeAPICommand).
func TestMain(m *testing.M) {
	if os.Getenv("SPLITLANE_FAKE_CORE") == "1" {
		fakeCore(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeCore(args []string) {
	if args[0] == "api" {
		fakeAPICommand(args[1:])
		return
	}
	config, _ := os.ReadFile(args[2])
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -count=1
```

Ожидается: `FAIL	splitlane/helper/internal/singbox [build failed]` и строки `internal\singbox\driver.go:<строка>:<столбец>: cannot use (*Driver)(nil) (value of type *Driver) as engine.CoreDriver value in variable declaration: *Driver does not implement engine.CoreDriver (missing method BuildSetupConfig)`, `internal\singbox\node_test.go:38:58: undefined: Node`, `internal\singbox\node_test.go:40:7: undefined: NewNode`.

- [ ] **Step 11: Реализовать клиента службы API и методы драйвера**

Файл `helper/internal/singbox/node.go`, целиком:

```go
package singbox

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/protocol"
)

// Runner runs exe with args, with env added to the helper's own environment, and returns the
// standard output; a non-zero exit is an error that carries the standard error text.
type Runner func(ctx context.Context, env []string, exe string, args ...string) ([]byte, error)

// Node is the client of the core's API service (spec 7; Plan 0, check (б)) through the core's own
// CLI, `sing-box api tailscale ...`. The URL and the secret go through the environment
// (BOX_API_URL, BOX_API_SECRET): the secret is never on a command line. The caller's ctx bounds
// every call; a ping takes up to about ten seconds.
type Node struct {
	Exe    string
	URL    string // 127.0.0.1:<port> of the API service of this run
	Secret string
	Run    Runner
}

var _ engine.NodeAPI = (*Node)(nil)

// NewNode talks to the API service on 127.0.0.1:port with the run's secret, through exe without a window.
func NewNode(exe string, port int, secret string) *Node {
	return &Node{Exe: exe, URL: fmt.Sprintf("127.0.0.1:%d", port), Secret: secret, Run: runAPI}
}

// colorCode is an ANSI color sequence: the core colors its FATAL line on stderr.
var colorCode = regexp.MustCompile("\x1b\\[[0-9;]*m")

func runAPI(ctx context.Context, env []string, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = hidden(windows.CREATE_NO_WINDOW)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(colorCode.ReplaceAllString(stderr.String(), ""))
		if len(text) > 400 {
			text = text[:400]
		}
		return out, fmt.Errorf("%s %s: %v: %s", filepath.Base(exe), strings.Join(args, " "), err, text)
	}
	return out, nil
}

func (n *Node) tailscale(ctx context.Context, args ...string) ([]byte, error) {
	env := []string{"BOX_API_URL=" + n.URL, "BOX_API_SECRET=" + n.Secret}
	return n.Run(ctx, env, n.Exe, append([]string{"api", "tailscale"}, args...)...)
}

// State reads `status` (the State and Log in lines) and the first row of `peer list`, which is
// the node itself: `status` does not print the node's addresses.
func (n *Node) State(ctx context.Context) (engine.NodeState, error) {
	out, err := n.tailscale(ctx, "status")
	if err != nil {
		return engine.NodeState{}, err
	}
	st := parseStatus(out)
	peers, err := n.tailscale(ctx, "peer", "list")
	if err != nil {
		return engine.NodeState{}, err
	}
	st.HasAddress = selfHasAddress(peers)
	return st, nil
}

// parseStatus reads the block of `status`: "  State:    Running", "  Log in:   https://...".
// An empty value is printed as "-".
func parseStatus(out []byte) engine.NodeState {
	var state, authURL string
	for _, line := range lines(out) {
		label, value, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(label, "  ") {
			continue
		}
		if value = strings.TrimSpace(value); value == "-" {
			value = ""
		}
		switch strings.TrimSpace(label) {
		case "State":
			if state == "" {
				state = value
			}
		case "Log in":
			if authURL == "" {
				authURL = value
			}
		}
	}
	return engine.NodeState{Backend: backendOf(state), AuthURL: authURL}
}

// backendOf maps the state of the node (ipn.State of Tailscale) to tailscale.backend (spec 5.2).
func backendOf(state string) string {
	switch state {
	case "NeedsLogin":
		return protocol.BackendNeedsLogin
	case "NeedsMachineAuth":
		return protocol.BackendNeedsApproval
	case "Starting":
		return protocol.BackendStarting
	case "Running":
		return protocol.BackendRunning
	default: // Stopped, NoState, InUseOtherUser, nothing
		return protocol.BackendStopped
	}
}

// selfHasAddress reads the first row of `peer list`, the node itself: its IP cell is the first of
// its tailnet addresses, "-" while it has none.
func selfHasAddress(out []byte) bool {
	r := rows(out)
	return len(r) > 0 && len(r[0]) > 1 && isIPv4(r[0][1])
}

// ExitNodes reads `exit-node list`: the peers that offer to be an exit node, one row each (DNS
// name, first tailnet address, online yes/no, selected). The id is the IPv4 address (spec 7), so
// a row without one is left out; the name is the first label of the DNS name.
func (n *Node) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	out, err := n.tailscale(ctx, "exit-node", "list")
	if err != nil {
		return nil, err
	}
	nodes := []protocol.ExitNode{}
	for _, cells := range rows(out) {
		if len(cells) < 3 || !isIPv4(cells[1]) {
			continue
		}
		name, _, _ := strings.Cut(cells[0], ".")
		nodes = append(nodes, protocol.ExitNode{ID: cells[1], Name: name, Online: cells[2] == "yes"})
	}
	return nodes, nil
}

// Path pings the exit node through the embedded node (`ping <address>`: up to ten pongs, it stops
// at the first direct one). The last pong tells the path: "via DERP(...)" and "via peer relay ..."
// are relays, "via <ip:port>" is direct. No pong: "" (unknown).
func (n *Node) Path(ctx context.Context, exitNode string) (string, error) {
	out, err := n.tailscale(ctx, "ping", exitNode)
	if err != nil {
		return "", err
	}
	path := ""
	for _, line := range lines(out) {
		at := strings.LastIndex(line, " via ")
		if !strings.HasPrefix(line, "pong from ") || at < 0 {
			continue
		}
		via := line[at+len(" via "):]
		if strings.HasPrefix(via, "DERP(") || strings.HasPrefix(via, "peer relay ") {
			path = protocol.PathRelay
		} else {
			path = protocol.PathDirect
		}
	}
	return path, nil
}

// Logout logs the embedded node out of the tailnet (spec 7, tailscale.logout).
func (n *Node) Logout(ctx context.Context) error {
	_, err := n.tailscale(ctx, "logout")
	return err
}

func lines(out []byte) []string {
	return strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
}

// rows are the non-empty lines of a table, split at tabs.
func rows(out []byte) [][]string {
	var r [][]string
	for _, line := range lines(out) {
		if line != "" {
			r = append(r, strings.Split(line, "\t"))
		}
	}
	return r
}

func isIPv4(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is4()
}
```

Изменить `helper/internal/singbox/driver.go`:

Найти:

```go
// rules), names the modes, runs the processes, reads the log and talks to the Clash API (5.3–5.5).
```

Заменить на:

```go
// rules), names the modes, runs the processes, reads the log, talks to the Clash API (5.3–5.5) and
// to the API service (spec 7), and builds the config of the setup mode.
```

Найти:

```go
// Clash returns the Clash API client of the run that listens on port with secret.
func (d *Driver) Clash(port int, secret string) engine.ClashAPI {
	return NewClash(fmt.Sprintf("http://127.0.0.1:%d", port), secret)
}
```

Заменить на:

```go
// Clash returns the Clash API client of the run that listens on port with secret.
func (d *Driver) Clash(port int, secret string) engine.ClashAPI {
	return NewClash(fmt.Sprintf("http://127.0.0.1:%d", port), secret)
}

// BuildSetupConfig is the config of the setup mode (spec 7), with the files of the driver's data folder.
func (d *Driver) BuildSetupConfig(o engine.SetupOptions) ([]byte, error) {
	return coreconfig.BuildSetup(coreconfig.SetupOptions{
		DataDir: d.DataDir, Hostname: o.Hostname, APIPort: o.APIPort, APISecret: o.APISecret,
	})
}

// Node returns the client of the API service of the run that listens on port with secret.
func (d *Driver) Node(port int, secret string) engine.NodeAPI {
	return NewNode(d.Exe, port, secret)
}
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -run 'TestNode|TestExitNodes|TestPath|TestDriverSetup' -count=1 -v
```

Ожидается:

```text
--- PASS: TestNodeStateParse (0.00s)
--- PASS: TestExitNodesParse (0.00s)
--- PASS: TestPathParse (0.00s)
--- PASS: TestNodeUsesEnvSecret (0.00s)
--- PASS: TestNodeErrorIsReturned (0.45s)
--- PASS: TestDriverSetupConfig (0.00s)
ok  	splitlane/helper/internal/singbox
```

- [ ] **Step 13: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; каждый пакет — `ok`:

```text
ok  	splitlane/helper/cmd/slctl
ok  	splitlane/helper/cmd/slguard
ok  	splitlane/helper/cmd/splitlane-helper
ok  	splitlane/helper/internal/atomicfile
ok  	splitlane/helper/internal/coreconfig
ok  	splitlane/helper/internal/engine
ok  	splitlane/helper/internal/pipe
ok  	splitlane/helper/internal/probe
ok  	splitlane/helper/internal/protocol
ok  	splitlane/helper/internal/rules
ok  	splitlane/helper/internal/session
ok  	splitlane/helper/internal/singbox
ok  	splitlane/helper/internal/store
ok  	splitlane/helper/internal/systs
ok  	splitlane/helper/internal/winhost
ok  	splitlane/helper/internal/engine
```

Последняя строка — пятикратный прогон тестов автомата.

- [ ] **Step 14: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/protocol/tailscale.go helper/internal/coreconfig/setup.go helper/internal/coreconfig/setup_test.go helper/internal/coreconfig/testdata/setup.golden.json helper/internal/engine/ports.go helper/internal/engine/fakes_test.go helper/internal/singbox/node.go helper/internal/singbox/node_test.go helper/internal/singbox/driver.go helper/internal/singbox/driver_test.go
git -C C:\code\router commit -m "feat(helper): setup-mode config and the client of the core's API service" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 4: Живая активность: типы и подсчёт

Сводка живой активности раздела 8.4 — чистый код, без автомата, канала и ядра: опрос соединений ядра, подписку `activity.subscribe` и рассылку события `activity` раз в секунду подключает задача 11. Здесь три части. В `internal/protocol` — типы события в точности по примеру 8.4 и разбор параметров `activity.subscribe` с необязательным `detailFor` (5.1). В `internal/rules` — `ProgramFolder`, ключ группы незакреплённой программы по правилу «папка программы» из 8.2, посчитанный тем же `Normalize`, что и `matcher.value` вида `folder` (8.1: правило реализовано один раз, им же группируется активность), и `Set.RuleOf` — правило, которое решает дорожку пути, с тем же старшинством, что у `LaneOf` (8.2: группировка активности определяет дорожку по тому же порядку; `LaneOf` теперь идёт через `RuleOf`). Новый пакет `internal/activity` — `Tracker`: из снимков соединений с накопленными счётчиками он считает скорости за секунду, складывает их по дорожкам, группирует соединения по правилу, по папке программы и в особые плитки `system` и `unattributed`, держит группу 5 минут после её последнего соединения и отдаёт подробности одной группы для `detailFor`.

Решения сверх буквы 8.4, каждое закреплено тестом. Пути Windows сравниваются без учёта регистра: `C:\WINDOWS\system32\svchost.exe` — та же плитка `system`, папка в другом регистре — та же программа; ключ группы — написание из первого её соединения. Если хотя бы один счётчик соединения уменьшился, это новое соединение под прежним id, и с нуля считаются оба счётчика. `exe` группы — файл с наибольшим числом байт с её появления (а не за последнюю секунду), при равенстве — путь, который раньше при сортировке; имя незакреплённой программы следует за ним. Группа без соединений, которую держат 5 минут, каждую секунду сверяется с текущей раскладкой и основной дорожкой: если её программу закрепили или открепили, группа уходит из сводки сразу (иначе программа, перенесённая из «остальных» на дорожку, 5 минут видна дважды), иначе обновляются дорожка и имя правила. До первого снимка и после `Reset` особые плитки стоят на дорожке `vpn`, основной дорожке по умолчанию (5.2). Особая плитка `system` есть в каждой сводке, даже когда службы Windows закреплены правилом вида `system`: тогда их трафик идёт в группу правила, а плитка `system` показывает нули. `ProgramFolder` не читает ресурсы файла (она вызывается для каждого соединения), а описание файла `Tracker` берёт один раз на каждую смену `exe` группы; правила сверяются один раз на путь за снимок. Вид группы `system` — уже существующая константа `protocol.KindSystem` (то же слово, что вид правила плитки «Службы Windows»); новых констант вида две: `KindApp` и `KindUnattributed`.

**Files:**
- Create: `helper/internal/protocol/activity.go`, `helper/internal/protocol/activity_test.go`, `helper/internal/rules/folder.go`, `helper/internal/rules/folder_test.go`, `helper/internal/activity/activity.go`, `helper/internal/activity/activity_test.go`
- Modify: `helper/internal/rules/layout.go` (`Set` хранит правило, `RuleOf`, `LaneOf` через `RuleOf`)
- Test: `helper/internal/rules/layout_test.go` (`TestRuleOfPrecedence`)

**Interfaces:**
- Consumes: `protocol.DecodeParams`, `protocol.Errorf`, `protocol.MaxPathLen`, `protocol.CodeBadRequest`, `protocol.AppRule`, `protocol.Matcher`, `protocol.KindFolder`, `protocol.KindSystem`, `protocol.LaneVPN`, `protocol.LaneDirect`, `rules.Env`, `rules.Normalize`; в тестах `rules.Check`, `rules.Compile`.
- Produces:
  - `protocol`: `const EventActivity = "activity"`; `const KindApp = "app"`, `KindUnattributed = "unattributed"` (вид `system` — прежняя `KindSystem = "system"`); `const KeySystem = "system"`, `KeyUnattributed = "unattributed"`; `type Traffic struct{ Down, Up int64 }` (JSON `down`, `up`); `type ActivityLanes struct{ VPN, Direct Traffic }` (`vpn`, `direct`); `type ActivityApp struct{ Key, Kind, Name, Exe string; RuleID *string; Lane string; Down, Up int64; Connections int }` (`key`, `kind`, `name`, `exe`, `ruleId`, `lane`, `down`, `up`, `connections`); `type ActivityConn struct{ Host, Lane string; Down, Up int64 }` (`host`, `lane`, `down`, `up`); `type ActivityDetail struct{ Key string; Exes []string; Connections []ActivityConn }` (`key`, `exes`, `connections`); `type Activity struct{ Lanes ActivityLanes; Apps []ActivityApp; Detail *ActivityDetail }` (`lanes`, `apps`, `detail` с `omitempty`); `func (a Activity) MarshalJSON() ([]byte, error)` и `func (d ActivityDetail) MarshalJSON() ([]byte, error)` пишут пустые списки как `[]`, никогда `null`; `type SubscribeParams struct{ DetailFor string }` (`detailFor`); `func DecodeSubscribe(params json.RawMessage) (detailFor string, err *Error)`.
  - `rules`: `func ProgramFolder(exe string, env Env) string`; `func (s *Set) RuleOf(path string) (protocol.AppRule, bool)` (у `nil` — `false`).
  - `activity`: `type Conn struct{ ID, ProcessPath, Lane, Host string; Upload, Download int64 }`; `type Layout interface{ RuleOf(path string) (protocol.AppRule, bool) }`; `type Tracker struct{ /* unexported */ }`, безопасен для вызова из нескольких горутин; `func New(env rules.Env) *Tracker`; `func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string)`; `func (t *Tracker) Summary(detailFor string) protocol.Activity`; `func (t *Tracker) Reset()`.

- [ ] **Step 1: Написать тест типов активности**

Файл `helper/internal/protocol/activity_test.go`, целиком:

```go
package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActivityJSONShape(t *testing.T) {
	id := goodID
	a := Activity{
		Lanes: ActivityLanes{VPN: Traffic{Down: 3, Up: 4}, Direct: Traffic{Down: 1, Up: 2}},
		Apps: []ActivityApp{
			{Key: id, Kind: KindApp, Name: "Steam", Exe: `C:\Games\Steam\steam.exe`, RuleID: &id, Lane: LaneDirect, Down: 1, Up: 2, Connections: 3},
			{Key: KeySystem, Kind: KindSystem, Lane: LaneVPN},
		},
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"lanes":{"vpn":{"down":3,"up":4},"direct":{"down":1,"up":2}},"apps":[` +
		`{"key":"` + id + `","kind":"app","name":"Steam","exe":"C:\\Games\\Steam\\steam.exe","ruleId":"` + id + `","lane":"direct","down":1,"up":2,"connections":3},` +
		`{"key":"system","kind":"system","name":"","exe":"","ruleId":null,"lane":"vpn","down":0,"up":0,"connections":0}]}`
	if string(b) != want {
		t.Fatalf("without detailFor:\n got %s\nwant %s", b, want)
	}
	a.Detail = &ActivityDetail{Key: id, Exes: []string{`C:\Games\Steam\steam.exe`},
		Connections: []ActivityConn{{Host: "store.example", Lane: LaneDirect, Down: 5, Up: 6}}}
	b, _ = json.Marshal(a)
	if !strings.HasSuffix(string(b), `,"detail":{"key":"`+id+`","exes":["C:\\Games\\Steam\\steam.exe"],"connections":[{"host":"store.example","lane":"direct","down":5,"up":6}]}}`) {
		t.Fatalf("with detail: %s", b)
	}
	// nil lists are written as [], never null
	b, _ = json.Marshal(Activity{Detail: &ActivityDetail{Key: KeyUnattributed}})
	want = `{"lanes":{"vpn":{"down":0,"up":0},"direct":{"down":0,"up":0}},"apps":[],"detail":{"key":"unattributed","exes":[],"connections":[]}}`
	if string(b) != want {
		t.Fatalf("empty lists:\n got %s\nwant %s", b, want)
	}
	if got := string(EncodeEvent(EventActivity, Activity{})); !strings.HasPrefix(got, `{"event":"activity","data":{"lanes":`) || !strings.Contains(got, `"apps":[]`) {
		t.Fatalf("event line %q", got)
	}
}

func TestDecodeSubscribe(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", `{"detailFor":""}`} {
		if key, err := DecodeSubscribe(json.RawMessage(raw)); err != nil || key != "" {
			t.Errorf("%q: want no detail, got %q %v", raw, key, err)
		}
	}
	if key, err := DecodeSubscribe(json.RawMessage(`{"detailFor":"C:\\Apps\\Tool\\"}`)); err != nil || key != `C:\Apps\Tool\` {
		t.Fatalf("detailFor: %q %v", key, err)
	}
	longest := strings.Repeat("a", MaxPathLen)
	if key, err := DecodeSubscribe(json.RawMessage(`{"detailFor":"` + longest + `"}`)); err != nil || key != longest {
		t.Fatalf("a key of MaxPathLen bytes must pass: %v", err)
	}
	bad := map[string]string{
		"unknown field": `{"detailFor":"system","all":true}`,
		"too long key":  `{"detailFor":"` + longest + `a"}`,
		"wrong type":    `{"detailFor":5}`,
		"not an object": `["system"]`,
	}
	for name, raw := range bad {
		if key, err := DecodeSubscribe(json.RawMessage(raw)); err == nil || err.Code != CodeBadRequest || key != "" {
			t.Errorf("%s: want bad_request, got %q %v", name, key, err)
		}
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/protocol/ -run 'TestActivityJSONShape|TestDecodeSubscribe' -count=1
```

Ожидается: `FAIL splitlane/helper/internal/protocol [build failed]` и `undefined: Activity`.

- [ ] **Step 3: Реализовать типы и разбор `activity.subscribe`**

Файл `helper/internal/protocol/activity.go`, целиком:

```go
package protocol

import "encoding/json"

// EventActivity is the summary of the live activity, sent once a second (spec 5.1, 8.4).
const EventActivity = "activity"

// Kinds of an activity group (spec 8.4). The kind of the "Windows services" group is KindSystem,
// the same word as the rule kind of that tile.
const (
	KindApp          = "app"
	KindUnattributed = "unattributed"
)

// Keys of the special tiles of unpinned traffic (spec 8.4). Every other key is a rule id or a program folder.
const (
	KeySystem       = "system"
	KeyUnattributed = "unattributed"
)

// Traffic is a rate: bytes per second over the last second.
type Traffic struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

// ActivityLanes are the rates of the two lanes.
type ActivityLanes struct {
	VPN    Traffic `json:"vpn"`
	Direct Traffic `json:"direct"`
}

// ActivityApp is one group of the summary: a pinned program, an unpinned program folder or a special tile.
type ActivityApp struct {
	Key         string  `json:"key"`
	Kind        string  `json:"kind"` // KindApp, KindSystem, KindUnattributed
	Name        string  `json:"name"`
	Exe         string  `json:"exe"`
	RuleID      *string `json:"ruleId"`
	Lane        string  `json:"lane"`
	Down        int64   `json:"down"`
	Up          int64   `json:"up"`
	Connections int     `json:"connections"`
}

// ActivityConn is one connection of the group named by detailFor.
type ActivityConn struct {
	Host string `json:"host"`
	Lane string `json:"lane"`
	Down int64  `json:"down"`
	Up   int64  `json:"up"`
}

// ActivityDetail is the detail of one group: the executables seen in it and its connections now.
type ActivityDetail struct {
	Key         string         `json:"key"`
	Exes        []string       `json:"exes"`
	Connections []ActivityConn `json:"connections"`
}

// Activity is the data of the activity event (spec 8.4). Detail is there only for a subscription with detailFor.
type Activity struct {
	Lanes  ActivityLanes   `json:"lanes"`
	Apps   []ActivityApp   `json:"apps"`
	Detail *ActivityDetail `json:"detail,omitempty"`
}

// MarshalJSON writes a missing list of apps as [], never null.
func (a Activity) MarshalJSON() ([]byte, error) {
	type plain Activity
	p := plain(a)
	if p.Apps == nil {
		p.Apps = []ActivityApp{}
	}
	return json.Marshal(p)
}

// MarshalJSON writes missing lists as [], never null.
func (d ActivityDetail) MarshalJSON() ([]byte, error) {
	type plain ActivityDetail
	p := plain(d)
	if p.Exes == nil {
		p.Exes = []string{}
	}
	if p.Connections == nil {
		p.Connections = []ActivityConn{}
	}
	return json.Marshal(p)
}

// SubscribeParams are the params of activity.subscribe.
type SubscribeParams struct {
	DetailFor string `json:"detailFor"`
}

// DecodeSubscribe reads the params of activity.subscribe: {} or {"detailFor": "<key>"} (spec 5.1, 8.4).
// Unknown fields and a key longer than MaxPathLen are bad_request; "" asks for no detail.
func DecodeSubscribe(params json.RawMessage) (detailFor string, err *Error) {
	var p SubscribeParams
	if e := DecodeParams(params, &p); e != nil {
		return "", e
	}
	if len(p.DetailFor) > MaxPathLen {
		return "", Errorf(CodeBadRequest, "detailFor is longer than %d bytes", MaxPathLen)
	}
	return p.DetailFor, nil
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/protocol/ -run 'TestActivityJSONShape|TestDecodeSubscribe' -count=1 -v
```

Ожидается: `--- PASS: TestActivityJSONShape`, `--- PASS: TestDecodeSubscribe` и `ok  splitlane/helper/internal/protocol`.

- [ ] **Step 5: Написать тесты папки программы и `RuleOf`**

Файл `helper/internal/rules/folder_test.go`, целиком:

```go
package rules

import "testing"

func TestProgramFolder(t *testing.T) {
	terminal := `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\`
	cases := map[string]string{
		`C:\Games\Steam\steam.exe`:                               `C:\Games\Steam\`,
		`C:\Games\Steam\bin\cef\steamwebhelper.exe`:              `C:\Games\Steam\bin\cef\`,
		`C:\Users\u\AppData\Local\Discord\app-1.0.9\Discord.exe`: `C:\Users\u\AppData\Local\Discord\`,
		`C:\Users\u\AppData\Local\Discord\Update.exe`:            `C:\Users\u\AppData\Local\Discord\`,
		`C:\Apps\Browser\124.0.6367.91\browser.exe`:              `C:\Apps\Browser\`,
		terminal + `WindowsTerminal.exe`:                         terminal,
		// shared folders give the file itself
		`D:\tool.exe`:                         `D:\tool.exe`,
		`C:\Windows\explorer.exe`:             `C:\Windows\explorer.exe`,
		`C:\Windows\System32\curl.exe`:        `C:\Windows\System32\curl.exe`,
		`C:\Program Files\app.exe`:            `C:\Program Files\app.exe`,
		`C:\Program Files (x86)\app.exe`:      `C:\Program Files (x86)\app.exe`,
		`C:\Users\u\Downloads\setup.exe`:      `C:\Users\u\Downloads\setup.exe`,
		`C:\Users\u\AppData\Local\Temp\x.exe`: `C:\Users\u\AppData\Local\Temp\x.exe`,
		`C:\Users\u\Downloads\tool\tool.exe`:  `C:\Users\u\Downloads\tool\`,
		// paths that Normalize refuses are their own group
		`:System`:              `:System`,
		`Registry`:             `Registry`,
		`C:\Tools\script.cmd`:  `C:\Tools\script.cmd`,
		`\\server\share\a.exe`: `\\server\share\a.exe`,
	}
	for exe, want := range cases {
		if got := ProgramFolder(exe, testEnv); got != want {
			t.Errorf("%s: got %q, want %q", exe, got, want)
		}
	}
	calls := 0
	env := testEnv
	env.Describe = func(string) string { calls++; return "" }
	ProgramFolder(`C:\Games\Steam\steam.exe`, env)
	if calls != 0 {
		t.Error("ProgramFolder must not read the file's resources")
	}
}
```

Изменить `helper/internal/rules/layout_test.go`:

Найти:

```go
	if _, ok := set.LaneOf(`C:\Elsewhere\x.exe`); ok {
		t.Error("a path outside every rule has no lane")
	}
}
```

Заменить на:

```go
	if _, ok := set.LaneOf(`C:\Elsewhere\x.exe`); ok {
		t.Error("a path outside every rule has no lane")
	}
}

func TestRuleOfPrecedence(t *testing.T) {
	apps := checked(t,
		rule("steam", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		rule("game", protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`),
		rule("tools", protocol.LaneVPN, protocol.KindFolder, `C:\Games\Steam\tools\t.exe`))
	set, err := Compile(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`C:\Games\Steam\steam.exe`:                          "steam",
		`C:\Games\Steam\steamapps\common\Game\launcher.exe`: "steam",
		`C:\Games\Steam\steamapps\common\Game\game.exe`:     "game",
		`c:\games\steam\STEAMAPPS\common\game\GAME.EXE`:     "game",
		`C:\Games\Steam\tools\sub\helper.exe`:               "tools",
	}
	for path, want := range cases {
		r, ok := set.RuleOf(path)
		if !ok || r.ID != want {
			t.Errorf("%s: got %q %v, want %s", path, r.ID, ok, want)
			continue
		}
		if lane, _ := set.LaneOf(path); lane != r.Lane {
			t.Errorf("%s: RuleOf gives lane %s, LaneOf %s", path, r.Lane, lane)
		}
	}
	if r, _ := set.RuleOf(`C:\Games\Steam\steamapps\common\Game\game.exe`); r != apps[1] {
		t.Errorf("RuleOf must return the normalized rule, got %+v", r)
	}
	if r, ok := set.RuleOf(`C:\Elsewhere\x.exe`); ok || r.ID != "" {
		t.Errorf("a path outside every rule has no rule, got %+v", r)
	}
	var none *Set
	if _, ok := none.RuleOf(`C:\Games\Steam\steam.exe`); ok {
		t.Error("a nil set has no rules")
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/rules/ -run 'TestProgramFolder|TestRuleOfPrecedence' -count=1
```

Ожидается: `FAIL splitlane/helper/internal/rules [build failed]`, `undefined: ProgramFolder` и `set.RuleOf undefined (type *Set has no field or method RuleOf)`.

- [ ] **Step 7: Реализовать `ProgramFolder` и `RuleOf`**

Файл `helper/internal/rules/folder.go`, целиком:

```go
package rules

import "splitlane/helper/internal/protocol"

// ProgramFolder is the group key of an unpinned program (spec 8.2, 8.4): the program folder with a
// trailing backslash, or the file path itself when the folder is shared (the cases where Normalize
// turns folder into file). The same rule as a pinned folder, so a program keeps its folder when it
// is pinned. A path that Normalize refuses (a pseudo-path such as ":System", not an .exe, not on a
// local drive) is returned as it is. It runs for every connection of every sample, so it does not
// read the file's resources: the name is not needed here.
func ProgramFolder(exe string, env Env) string {
	env.Describe = nil
	n, err := Normalize(protocol.AppRule{Exe: exe, Matcher: protocol.Matcher{Kind: protocol.KindFolder}}, env)
	if err != nil {
		return exe
	}
	return n.Matcher.Value
}
```

Изменить `helper/internal/rules/layout.go`:

Найти:

```go
type entry struct {
	lane  string
	rank  int
	depth int
	res   []*regexp.Regexp
}

// Compile prepares a checked layout for LaneOf. A rule without a usable expression
// is an error: it would never match, and LaneOf would report no lane for it.
func Compile(apps []protocol.AppRule, env Env) (*Set, error) {
	s := &Set{}
	for _, a := range apps {
		e := entry{lane: a.Lane, rank: 2, depth: len(a.Matcher.Value)}
```

Заменить на:

```go
type entry struct {
	rule  protocol.AppRule
	rank  int
	depth int
	res   []*regexp.Regexp
}

// Compile prepares a checked layout for LaneOf. A rule without a usable expression
// is an error: it would never match, and LaneOf would report no lane for it.
func Compile(apps []protocol.AppRule, env Env) (*Set, error) {
	s := &Set{}
	for _, a := range apps {
		e := entry{rule: a, rank: 2, depth: len(a.Matcher.Value)}
```

Найти:

```go
// LaneOf returns the lane of the most specific matching rule, and false when no rule matches.
func (s *Set) LaneOf(path string) (string, bool) {
	best := -1
	for i, e := range s.entries {
		if !matchAny(e.res, path) {
			continue
		}
		if best < 0 || e.rank > s.entries[best].rank || (e.rank == s.entries[best].rank && e.depth > s.entries[best].depth) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return s.entries[best].lane, true
}
```

Заменить на:

```go
// LaneOf returns the lane of the most specific matching rule, and false when no rule matches.
func (s *Set) LaneOf(path string) (string, bool) {
	r, ok := s.RuleOf(path)
	return r.Lane, ok
}

// RuleOf is the rule that decides the lane of path (the same precedence as LaneOf), if any.
// The activity groups a pinned program by it (spec 8.4). A nil Set has no rules.
func (s *Set) RuleOf(path string) (protocol.AppRule, bool) {
	if s == nil {
		return protocol.AppRule{}, false
	}
	best := -1
	for i, e := range s.entries {
		if !matchAny(e.res, path) {
			continue
		}
		if best < 0 || e.rank > s.entries[best].rank || (e.rank == s.entries[best].rank && e.depth > s.entries[best].depth) {
			best = i
		}
	}
	if best < 0 {
		return protocol.AppRule{}, false
	}
	return s.entries[best].rule, true
}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/rules/ -run 'TestProgramFolder|TestRuleOfPrecedence' -count=1 -v; go test ./internal/rules/ -count=1
```

Ожидается: `--- PASS: TestProgramFolder`, `--- PASS: TestRuleOfPrecedence`, затем `ok  splitlane/helper/internal/rules` для всего пакета (прежние тесты `LaneOf` проходят через `RuleOf`).

- [ ] **Step 9: Написать тесты подсчёта активности**

Файл `helper/internal/activity/activity_test.go`, целиком:

```go
package activity

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

var testEnv = rules.Env{
	WindowsDir:      `C:\Windows`,
	ProgramFiles:    `C:\Program Files`,
	ProgramFilesX86: `C:\Program Files (x86)`,
	UserDirs:        []string{`C:\Users\u\Downloads`},
	Describe: func(exe string) string {
		switch strings.ToLower(exe) {
		case `c:\games\steam\steam.exe`:
			return "Steam Client"
		case `c:\apps\editor\editor.exe`:
			return "Text Editor"
		}
		return ""
	},
}

var start = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

const (
	steamID = "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"
	gameID  = "5d0f3c1a-2b7e-4f60-8a9d-1c2e3f4a5b6c"
	otherID = "7a6b5c4d-3e2f-4a1b-9c8d-7e6f5a4b3c2d"
	sysID   = "9e8d7c6b-5a49-4f38-8e27-160f5e4d3c2b"

	discord = `C:\Users\u\AppData\Local\Discord\`
)

// layout is a checked and compiled layout, the way the engine builds it.
func layout(t *testing.T, apps ...protocol.AppRule) *rules.Set {
	t.Helper()
	checked, rejected, err := rules.Check(apps, testEnv)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	set, err := rules.Compile(checked, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func pinned(id, lane, kind, exe string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: kind}, Lane: lane}
}

func conn(id, path, lane string, up, down int64) Conn {
	return Conn{ID: id, ProcessPath: path, Lane: lane, Host: "host.example", Upload: up, Download: down}
}

func find(t *testing.T, s protocol.Activity, key string) protocol.ActivityApp {
	t.Helper()
	for _, a := range s.Apps {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("no group %q in %+v", key, s.Apps)
	return protocol.ActivityApp{}
}

func keys(s protocol.Activity) []string {
	out := []string{}
	for _, a := range s.Apps {
		out = append(out, a.Key)
	}
	return out
}

func idle(lane string) []protocol.ActivityApp {
	return []protocol.ActivityApp{
		{Key: protocol.KeySystem, Kind: protocol.KindSystem, Lane: lane},
		{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: lane},
	}
}

func TestRatesFromCounters(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	a, b := `C:\Apps\A\a.exe`, `C:\Apps\B\b.exe`
	tr.Sample(start, []Conn{conn("1", a, protocol.LaneVPN, 100, 1000), conn("2", b, protocol.LaneDirect, 10, 20)}, none, protocol.LaneVPN)
	s := tr.Summary("")
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 100}, Direct: protocol.Traffic{Down: 20, Up: 10}}); s.Lanes != want {
		t.Fatalf("a connection seen for the first time counts its whole counters: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 1000 || g.Up != 100 || g.Connections != 1 {
		t.Fatalf("group a %+v", g)
	}
	// 1 goes on, 2 is gone, 3 is new
	tr.Sample(start.Add(time.Second), []Conn{conn("1", a, protocol.LaneVPN, 150, 1600), conn("3", b, protocol.LaneDirect, 5, 7)}, none, protocol.LaneVPN)
	s = tr.Summary("")
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 600, Up: 50}, Direct: protocol.Traffic{Down: 7, Up: 5}}); s.Lanes != want {
		t.Fatalf("second sample: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 600 || g.Up != 50 {
		t.Fatalf("group a %+v", g)
	}
	if g := find(t, s, `C:\Apps\B\`); g.Down != 7 || g.Up != 5 || g.Connections != 1 {
		t.Fatalf("group b %+v", g)
	}
	// id 1 now belongs to a new connection: its upload went down, so both counters count from 0;
	// 3 did not move
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 30, 2000), conn("3", b, protocol.LaneDirect, 5, 7)}, none, protocol.LaneVPN)
	s = tr.Summary("")
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 2000, Up: 30}}); s.Lanes != want {
		t.Fatalf("a reused id: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\B\`); g.Down != 0 || g.Up != 0 || g.Connections != 1 {
		t.Fatalf("an idle connection: %+v", g)
	}
}

func TestGroupingByRuleAndFolder(t *testing.T) {
	set := layout(t,
		pinned(steamID, protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		pinned(gameID, protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`))
	tr := New(testEnv)
	tr.Sample(start, []Conn{
		conn("1", `C:\Games\Steam\steam.exe`, protocol.LaneDirect, 10, 100),
		conn("2", `C:\Games\Steam\bin\cef\steamwebhelper.exe`, protocol.LaneDirect, 10, 300),
		conn("3", `C:\Games\Steam\steamapps\common\Game\game.exe`, protocol.LaneVPN, 50, 5000),
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 10),
		// Windows paths ignore case: the same folder written otherwise is the same program
		conn("6", `c:\users\u\appdata\local\discord\app-1.0.9\Squirrel.exe`, protocol.LaneVPN, 0, 1),
		conn("7", `C:\Users\u\Downloads\tool.exe`, protocol.LaneVPN, 1, 1),
		conn("8", `C:\Apps\Editor\editor.exe`, protocol.LaneVPN, 0, 2),
	}, set, protocol.LaneVPN)
	steam, game := steamID, gameID
	want := []protocol.ActivityApp{
		{Key: gameID, Kind: protocol.KindApp, Name: "game", Exe: `C:\Games\Steam\steamapps\common\Game\game.exe`,
			RuleID: &game, Lane: protocol.LaneVPN, Down: 5000, Up: 50, Connections: 1},
		{Key: discord, Kind: protocol.KindApp, Name: "Discord", Exe: discord + `app-1.0.9\Discord.exe`,
			Lane: protocol.LaneVPN, Down: 911, Up: 100, Connections: 3},
		// the name of a pinned program is the rule's, whichever file has the most traffic
		{Key: steamID, Kind: protocol.KindApp, Name: "Steam Client", Exe: `C:\Games\Steam\bin\cef\steamwebhelper.exe`,
			RuleID: &steam, Lane: protocol.LaneDirect, Down: 400, Up: 20, Connections: 2},
		{Key: `C:\Apps\Editor\`, Kind: protocol.KindApp, Name: "Text Editor", Exe: `C:\Apps\Editor\editor.exe`,
			Lane: protocol.LaneVPN, Down: 2, Connections: 1},
		// a shared folder: the file is the program
		{Key: `C:\Users\u\Downloads\tool.exe`, Kind: protocol.KindApp, Name: "tool", Exe: `C:\Users\u\Downloads\tool.exe`,
			Lane: protocol.LaneVPN, Down: 1, Up: 1, Connections: 1},
		{Key: protocol.KeySystem, Kind: protocol.KindSystem, Lane: protocol.LaneVPN},
		{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: protocol.LaneVPN},
	}
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, want) {
		t.Fatalf("summary:\n got %+v\nwant %+v", got, want)
	}
	// exe is the file with the most bytes since the group appeared (not in the last second),
	// a tie goes to the path that sorts first, and the name follows the file
	tr.Sample(start.Add(time.Second), []Conn{
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 1000),
	}, set, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), discord); g.Exe != discord+`app-1.0.9\Discord.exe` || g.Name != "Discord" {
		t.Fatalf("1000 bytes of Update.exe against 1000 of Discord.exe so far: %+v", g)
	}
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 1001),
	}, set, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), discord); g.Exe != discord+`Update.exe` || g.Name != "Update" || g.Connections != 2 {
		t.Fatalf("1001 bytes of Update.exe against 1000: %+v", g)
	}
}

func TestSystemAndUnattributedTiles(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneVPN)) {
		t.Fatalf("before the first sample: %+v", got)
	}
	tr.Sample(start, nil, none, protocol.LaneDirect)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneDirect)) {
		t.Fatalf("idle, the main lane is direct: %+v", got)
	}
	tr.Sample(start.Add(time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 1, 2),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 3, 4),
		conn("3", "", protocol.LaneVPN, 10, 20),
		conn("4", "", protocol.LaneDirect, 1, 1),
		conn("5", `C:\Windows\explorer.exe`, protocol.LaneVPN, 0, 1),
		conn("6", `C:\Tools\svchost.exe`, protocol.LaneVPN, 0, 1),
	}, none, protocol.LaneVPN)
	s := tr.Summary("")
	if g := find(t, s, protocol.KeySystem); g.Kind != protocol.KindSystem || g.RuleID != nil || g.Name != "" ||
		g.Lane != protocol.LaneVPN || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("system %+v", g)
	}
	if g := find(t, s, protocol.KeyUnattributed); g.Kind != protocol.KindUnattributed || g.RuleID != nil || g.Name != "" ||
		g.Exe != "" || g.Lane != protocol.LaneVPN || g.Down != 21 || g.Up != 11 || g.Connections != 2 {
		t.Errorf("unattributed %+v", g)
	}
	// another program of the Windows folder, and svchost.exe outside System32, are programs of their own
	if g := find(t, s, `C:\Windows\explorer.exe`); g.Kind != protocol.KindApp || g.Name != "explorer" {
		t.Errorf("explorer %+v", g)
	}
	if g := find(t, s, `C:\Tools\`); g.Kind != protocol.KindApp || g.Name != "svchost" {
		t.Errorf("svchost.exe elsewhere %+v", g)
	}
	// a pinned "Windows services" tile takes svchost.exe and :System; the special tile stays, idle
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 2, 4),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 6, 8),
	}, layout(t, pinned(sysID, protocol.LaneDirect, protocol.KindSystem, "")), protocol.LaneVPN)
	s = tr.Summary("")
	if g := find(t, s, sysID); g.Kind != protocol.KindSystem || g.RuleID == nil || *g.RuleID != sysID || g.Name != "svchost" ||
		g.Lane != protocol.LaneDirect || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("pinned system %+v", g)
	}
	if g := find(t, s, protocol.KeySystem); g.Down != 0 || g.Up != 0 || g.Connections != 0 {
		t.Errorf("the special tile without traffic %+v", g)
	}
	find(t, s, protocol.KeyUnattributed)
}

func TestUnpinnedKeptFiveMinutes(t *testing.T) {
	tool, other, gameExe := `C:\Apps\Tool\tool.exe`, `C:\Apps\Other\other.exe`, `C:\Games\Game\game.exe`
	game := pinned(gameID, protocol.LaneVPN, protocol.KindFile, gameExe)
	set := layout(t, game)
	tr := New(testEnv)
	tr.Sample(start, []Conn{conn("1", tool, protocol.LaneVPN, 5, 5), conn("2", gameExe, protocol.LaneVPN, 1, 1)}, set, protocol.LaneVPN)
	tr.Sample(start.Add(time.Second), []Conn{conn("3", other, protocol.LaneVPN, 1, 1)}, set, protocol.LaneVPN)
	s := tr.Summary("")
	if g := find(t, s, `C:\Apps\Tool\`); g.Down != 0 || g.Up != 0 || g.Connections != 0 || g.Exe != tool || g.Name != "tool" {
		t.Fatalf("a program without connections stays, with zero rates: %+v", g)
	}
	if g := find(t, s, gameID); g.Connections != 0 {
		t.Fatalf("a pinned program too: %+v", g)
	}
	find(t, s, `C:\Apps\Other\`)
	// a kept program that gets pinned leaves at once: its traffic is the rule's now
	set = layout(t, game, pinned(otherID, protocol.LaneDirect, protocol.KindFolder, other))
	tr.Sample(start.Add(2*time.Second), nil, set, protocol.LaneVPN)
	if got, want := keys(tr.Summary("")), []string{gameID, `C:\Apps\Tool\`, protocol.KeySystem, protocol.KeyUnattributed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after pinning other.exe: %v", got)
	}
	// 5 minutes after the last connection the program is still there, and follows the main lane
	tr.Sample(start.Add(5*time.Minute), nil, set, protocol.LaneDirect)
	s = tr.Summary("")
	if g := find(t, s, `C:\Apps\Tool\`); g.Lane != protocol.LaneDirect {
		t.Fatalf("the main lane moved: %+v", g)
	}
	if g := find(t, s, gameID); g.Lane != protocol.LaneVPN {
		t.Fatalf("a pinned program keeps the lane of its rule: %+v", g)
	}
	tr.Sample(start.Add(5*time.Minute+time.Second), nil, set, protocol.LaneDirect)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneDirect)) {
		t.Fatalf("later than 5 minutes after the last connection: %+v", got)
	}
}

func TestDetailForAGroup(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	tr.Sample(start, []Conn{
		{ID: "1", ProcessPath: discord + `app-1.0.9\Discord.exe`, Lane: protocol.LaneVPN, Host: "gateway.discord.example", Upload: 10, Download: 100},
		{ID: "2", ProcessPath: discord + `Update.exe`, Lane: protocol.LaneDirect, Host: "203.0.113.7", Upload: 1, Download: 1000},
		{ID: "3", ProcessPath: discord + `app-1.0.9\Discord.exe`, Lane: protocol.LaneVPN, Download: 5},
		{ID: "4", ProcessPath: `C:\Apps\Tool\tool.exe`, Lane: protocol.LaneVPN, Host: "tool.example", Upload: 1, Download: 1},
	}, none, protocol.LaneVPN)
	if d := tr.Summary("").Detail; d != nil {
		t.Fatalf("no detail without detailFor: %+v", d)
	}
	if d := tr.Summary(`C:\Apps\Nothing\`).Detail; d != nil {
		t.Fatalf("a key that is not in the summary gives no detail: %+v", d)
	}
	s := tr.Summary(discord)
	want := &protocol.ActivityDetail{Key: discord,
		Exes: []string{discord + `Update.exe`, discord + `app-1.0.9\Discord.exe`},
		Connections: []protocol.ActivityConn{
			{Host: "203.0.113.7", Lane: protocol.LaneDirect, Down: 1000, Up: 1},
			{Host: "gateway.discord.example", Lane: protocol.LaneVPN, Down: 100, Up: 10},
			{Host: "", Lane: protocol.LaneVPN, Down: 5},
		}}
	if !reflect.DeepEqual(s.Detail, want) {
		t.Fatalf("detail:\n got %+v\nwant %+v", s.Detail, want)
	}
	if !reflect.DeepEqual(s.Apps, tr.Summary("").Apps) {
		t.Fatal("the detail must not change the summary")
	}
	// kept without connections: the files stay, the connections are empty
	tr.Sample(start.Add(time.Second), nil, none, protocol.LaneVPN)
	if d := tr.Summary(discord).Detail; d == nil || len(d.Exes) != 2 || d.Connections == nil || len(d.Connections) != 0 {
		t.Fatalf("kept group: %+v", d)
	}
	if d := tr.Summary(protocol.KeyUnattributed).Detail; d == nil || d.Exes == nil || len(d.Exes) != 0 || d.Connections == nil {
		t.Fatalf("a special tile has a detail too: %+v", d)
	}
}

func TestResetForgets(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	tool := `C:\Apps\Tool\tool.exe`
	tr.Sample(start, []Conn{conn("1", tool, protocol.LaneVPN, 100, 200), conn("2", "", protocol.LaneDirect, 1, 1)}, none, protocol.LaneDirect)
	tr.Reset()
	s := tr.Summary(`C:\Apps\Tool\`)
	if s.Lanes != (protocol.ActivityLanes{}) || !reflect.DeepEqual(s.Apps, idle(protocol.LaneVPN)) || s.Detail != nil {
		t.Fatalf("after Reset: %+v", s)
	}
	// the same connection counts its whole counters again
	tr.Sample(start.Add(time.Second), []Conn{conn("1", tool, protocol.LaneVPN, 100, 200)}, none, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), `C:\Apps\Tool\`); g.Down != 200 || g.Up != 100 {
		t.Fatalf("after Reset: %+v", g)
	}
}
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/activity/ -count=1
```

Ожидается: `FAIL splitlane/helper/internal/activity [build failed]`, `undefined: Conn` и `undefined: New`.

- [ ] **Step 11: Реализовать `Tracker`**

Файл `helper/internal/activity/activity.go`, целиком:

```go
// Package activity counts the live activity of spec 8.4: the connections of the core, sampled once
// a second, become the rates of the two lanes and of every program, grouped by rule or by program
// folder, in the data of the activity event.
package activity

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// keep is how long a group stays in the summary after its last connection (spec 8.4).
const keep = 5 * time.Minute

// Conn is one connection of the core at one moment: cumulative byte counters.
type Conn struct {
	ID          string
	ProcessPath string // "" when the core could not tell the owner
	Lane        string // protocol.LaneVPN or protocol.LaneDirect
	Host        string // name from the reverse DNS mapping, or the address
	Upload      int64  // bytes so far
	Download    int64
}

// Layout tells the rule of a process path (rules.Set satisfies it).
type Layout interface {
	RuleOf(path string) (protocol.AppRule, bool)
}

// Tracker turns the samples of the core's connections into the summary. It is safe for concurrent use.
type Tracker struct {
	env     rules.Env
	svchost string

	mu     sync.Mutex
	prev   map[string]counters // the counters of the previous sample, by connection id
	groups map[string]*group   // by class.id
	lanes  protocol.ActivityLanes
}

type counters struct{ up, down int64 }

// group is one entry of the summary.
type group struct {
	key      string
	kind     string
	ruleID   string // "" for an unpinned group
	name     string // the rule's name; for an unpinned program, the name of exe
	named    string // the exe that name was taken from
	lane     string
	exe      string               // the executable with the most bytes since the group appeared
	exes     map[string]*exeBytes // every executable seen while the group is kept, by lower-case path
	lastSeen time.Time
	down, up int64
	conns    []protocol.ActivityConn // the connections of the last sample
}

type exeBytes struct {
	path  string
	bytes int64
}

// class is the group that a process path belongs to now.
type class struct {
	id     string // the index of the group: the key, and a lower-case folder for an unpinned program
	key    string
	kind   string
	ruleID string
	name   string
	lane   string
}

// New is a tracker with nothing seen yet; its summary has the two special tiles on the VPN lane,
// the default lane (spec 5.2), until the first sample names the main lane.
func New(env rules.Env) *Tracker {
	t := &Tracker{env: env, svchost: strings.TrimRight(env.WindowsDir, `\`) + `\System32\svchost.exe`}
	t.reset()
	return t
}

// Reset forgets everything (the core stopped): counters, groups and the files seen.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reset()
}

func (t *Tracker) reset() {
	t.prev = map[string]counters{}
	t.lanes = protocol.ActivityLanes{}
	t.groups = map[string]*group{}
	for _, key := range []string{protocol.KeySystem, protocol.KeyUnattributed} {
		c := t.special(key, protocol.LaneVPN)
		t.groups[c.id] = newGroup(c)
	}
}

// special is the class of the tile system or unattributed: unpinned, always on the main lane.
func (t *Tracker) special(key, mainLane string) class {
	kind := protocol.KindSystem
	if key == protocol.KeyUnattributed {
		kind = protocol.KindUnattributed
	}
	return class{id: key, key: key, kind: kind, lane: mainLane}
}

// Sample takes the connections of one second. mainLane is the lane of unpinned programs now (the default lane,
// or direct in "all direct").
func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, g := range t.groups {
		g.down, g.up, g.conns = 0, 0, nil
	}
	t.lanes = protocol.ActivityLanes{}
	// A program has many connections: the rules are matched once per path and sample.
	classes := map[string]class{}
	classOf := func(path string) class {
		cl, ok := classes[path]
		if !ok {
			cl = t.classify(path, layout, mainLane)
			classes[path] = cl
		}
		return cl
	}
	next := make(map[string]counters, len(conns))
	for _, c := range conns {
		cur := counters{up: c.Upload, down: c.Download}
		was, seen := t.prev[c.ID]
		if !seen || cur.up < was.up || cur.down < was.down {
			was = counters{} // a new connection, or a new one under a reused id
		}
		next[c.ID] = cur
		up, down := cur.up-was.up, cur.down-was.down
		switch c.Lane {
		case protocol.LaneVPN:
			t.lanes.VPN.Up += up
			t.lanes.VPN.Down += down
		case protocol.LaneDirect:
			t.lanes.Direct.Up += up
			t.lanes.Direct.Down += down
		}
		cl := classOf(c.ProcessPath)
		g := t.groups[cl.id]
		if g == nil {
			g = newGroup(cl)
			t.groups[cl.id] = g
		}
		g.take(cl)
		g.lastSeen = now
		g.up += up
		g.down += down
		g.conns = append(g.conns, protocol.ActivityConn{Host: c.Host, Lane: c.Lane, Down: down, Up: up})
		if c.ProcessPath != "" {
			low := strings.ToLower(c.ProcessPath)
			e := g.exes[low]
			if e == nil {
				e = &exeBytes{path: c.ProcessPath}
				g.exes[low] = e
			}
			e.bytes += up + down
		}
	}
	t.prev = next
	for id, g := range t.groups {
		switch {
		case id == protocol.KeySystem || id == protocol.KeyUnattributed:
			g.lane = mainLane
		case len(g.conns) > 0:
			// its connections decided it above
		case now.Sub(g.lastSeen) > keep:
			delete(t.groups, id)
			continue
		default:
			// A kept group follows the layout and the main lane. When its program now belongs
			// to another group (it was pinned or unpinned), the group leaves at once: the
			// program is in the other one.
			cl := classOf(g.exe)
			if cl.id != id {
				delete(t.groups, id)
				continue
			}
			g.take(cl)
		}
		t.settle(g)
	}
}

// classify finds the group of a process path (spec 8.4): no owner, a pinned rule, the system
// tile, or the program folder.
func (t *Tracker) classify(path string, layout Layout, mainLane string) class {
	if path == "" {
		return t.special(protocol.KeyUnattributed, mainLane)
	}
	if layout != nil {
		if r, ok := layout.RuleOf(path); ok {
			kind := protocol.KindApp
			if r.Matcher.Kind == protocol.KindSystem {
				kind = protocol.KindSystem
			}
			return class{id: "rule:" + r.ID, key: r.ID, kind: kind, ruleID: r.ID, name: r.Name, lane: r.Lane}
		}
	}
	if strings.EqualFold(path, ":System") || strings.EqualFold(path, t.svchost) {
		return t.special(protocol.KeySystem, mainLane)
	}
	key := rules.ProgramFolder(path, t.env)
	return class{id: "folder:" + strings.ToLower(key), key: key, kind: protocol.KindApp, lane: mainLane}
}

func newGroup(c class) *group {
	g := &group{key: c.key, exes: map[string]*exeBytes{}}
	g.take(c)
	return g
}

// take updates what the layout and the main lane decide: the kind, the rule, the lane, a rule's name.
func (g *group) take(c class) {
	g.kind, g.ruleID, g.lane = c.kind, c.ruleID, c.lane
	if c.ruleID != "" {
		g.name = c.name
	}
}

// settle picks the executable with the most bytes; an unpinned program is named after it.
func (t *Tracker) settle(g *group) {
	top := ""
	for low, e := range g.exes {
		if top == "" || e.bytes > g.exes[top].bytes || (e.bytes == g.exes[top].bytes && low < top) {
			top = low
		}
	}
	if top != "" {
		g.exe = g.exes[top].path
	}
	if g.ruleID == "" && g.kind == protocol.KindApp && g.named != g.exe {
		g.name, g.named = t.describe(g.exe), g.exe
	}
}

// describe is the file description from the resources of exe, or its name without the extension (spec 8.4).
func (t *Tracker) describe(exe string) string {
	if t.env.Describe != nil {
		if d := strings.TrimSpace(t.env.Describe(exe)); d != "" {
			return d
		}
	}
	base := filepath.Base(exe)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Summary is the event of the last sample; detailFor "" gives no detail. Apps are ordered by
// down+up, the busiest first, then by key.
func (t *Tracker) Summary(detailFor string) protocol.Activity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := protocol.Activity{Lanes: t.lanes, Apps: make([]protocol.ActivityApp, 0, len(t.groups))}
	for _, g := range t.groups {
		a := protocol.ActivityApp{Key: g.key, Kind: g.kind, Name: g.name, Exe: g.exe, Lane: g.lane,
			Down: g.down, Up: g.up, Connections: len(g.conns)}
		if g.ruleID != "" {
			id := g.ruleID
			a.RuleID = &id
		}
		out.Apps = append(out.Apps, a)
		if detailFor != "" && g.key == detailFor {
			out.Detail = g.detail()
		}
	}
	sort.Slice(out.Apps, func(i, j int) bool {
		a, b := out.Apps[i], out.Apps[j]
		if a.Down+a.Up != b.Down+b.Up {
			return a.Down+a.Up > b.Down+b.Up
		}
		return a.Key < b.Key
	})
	return out
}

// detail lists the files seen in the group, sorted, and its connections now, the busiest first.
func (g *group) detail() *protocol.ActivityDetail {
	d := &protocol.ActivityDetail{Key: g.key, Exes: make([]string, 0, len(g.exes)),
		Connections: append([]protocol.ActivityConn{}, g.conns...)}
	for _, e := range g.exes {
		d.Exes = append(d.Exes, e.path)
	}
	sort.Strings(d.Exes)
	sort.SliceStable(d.Connections, func(i, j int) bool {
		a, b := d.Connections[i], d.Connections[j]
		if a.Down+a.Up != b.Down+b.Up {
			return a.Down+a.Up > b.Down+b.Up
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Lane < b.Lane
	})
	return d
}
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/activity/ -count=1 -v
```

Ожидается: `--- PASS: TestRatesFromCounters`, `--- PASS: TestGroupingByRuleAndFolder`, `--- PASS: TestSystemAndUnattributedTiles`, `--- PASS: TestUnpinnedKeptFiveMinutes`, `--- PASS: TestDetailForAGroup`, `--- PASS: TestResetForgets` и `ok  splitlane/helper/internal/activity`.

- [ ] **Step 13: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; каждый пакет `ok`, среди них `ok  splitlane/helper/internal/activity`, `ok  splitlane/helper/internal/protocol`, `ok  splitlane/helper/internal/rules` и `ok  splitlane/helper/internal/engine` (автомат зовёт `LaneOf`, который теперь идёт через `RuleOf`); последней строкой — `ok  splitlane/helper/internal/engine` пяти прогонов подряд.

- [ ] **Step 14: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/protocol helper/internal/rules helper/internal/activity
git -C C:\code\router commit -m "feat(helper): live activity types, program folder, RuleOf and the activity tracker" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 5: Сценарная проверка scripts/scenario.mjs

Задача пишет сценарную проверку раздела 12 («Сценарная проверка на ПК»): скрипт `scripts/scenario.mjs` на Node.js 24 без пакетов npm подключается к каналу `\\.\pipe\SplitLane.Helper` вторым клиентом (5.1: `hello` с протоколом 1, затем `state.get`, запросы с номерами и события `state`), закрепляет переименованные копии `curl.exe` и `node.exe` правилами 8.1 и проходит восемь шагов раздела 12 с их условиями. Каждый шаг печатает строку `SCENARIO <n> PASS|FAIL <текст>`, в конце — `SCENARIO-RESTORE ...` и `SCENARIO-DONE pass=<k> fail=<m>`; код выхода 0 только тогда, когда пройдены все шаги и прежняя раскладка вернулась. Адреса не печатаются: адреса дорожек из `state.addresses` становятся `<vpn>` и `<direct>`, любой другой адрес — `<other>` (Global Constraints, личные данные). Против служебного процесса скрипт в этой задаче не запускается: это делает задача 16 под сторожем, командой `node C:\code\router\scripts\scenario.mjs --heartbeat C:\code\router\spike\plan1\guard\deadline.txt`.

Чистая логика — в `scripts/scenario-lib.mjs`: сообщения STUN (запрос Binding и разбор XOR-MAPPED-ADDRESS по RFC 5389), разбиение потока канала на строки, сопоставление ответов запросам по `id`, адреса дорожек и их маски, раскладка копий (UUID — `crypto.randomUUID`), решения шагов 2–8 по записанным наблюдениям, разбор вывода `tailscale.exe` и `tasklist`. Тесты запускаются командой `node --test scripts/scenario.test.mjs`; два из них запускают сам `scenario.mjs` против поддельного служебного процесса и поддельного сервиса адреса на каналах `SplitLane.Test.<число>`. Сеть и система в тестах не участвуют. Подробности, которых спецификация не задаёт:

- Подготовка. Копии лежат в `%TEMP%\splitlane-scenario\` (по длинному пути: ядро сравнивает настоящий путь процесса), по папке на дорожку: `vpn\` — `curl-vpn.exe` и `node-vpn.exe` (вид `file`, VPN), `direct\` — `curl-direct.exe` и `node-direct.exe` (`file`, напрямую), `free\` — `curl-free.exe` и `node-free.exe` (не закреплены); для шага 6 папка `nest\` закреплена напрямую видом `folder` (по `nest\curl-outer.exe`), а `nest\inner\curl-nest.exe` — через VPN видом `file`. Основная дорожка — VPN. Сценарий начинается только из фазы `off` (иначе `SCENARIO 0 FAIL` и ничего не меняется), отправляет раскладку `lanes.set` до шага 1 и в конце, если нет `--keep`, возвращает раскладку из ответа `state.get`.
- Копии `node` запускают этот же файл в режиме ребёнка (`--child stun`, `--child keep`), а не скрипт, записанный рядом с ними: ядро видит процесс копии, а код STUN проверен тестами. Ребёнок печатает `<unix ms> <адрес>` (или `-`), `keep` — ещё номер соединения: часы у родителя и ребёнка общие, а номер показывает, что соединение действительно держится.
- Шаги 2, 3, 5 и 6 оценивают ответы копии так же, как `slctl expect-lanes`: верен первый ответ или два из первых трёх; ответ копии дорожки VPN с адреса прямой дорожки — провал сразу.
- Шаг 4: копия `node-vpn` переносится напрямую и обратно. На каждом переносе до `lanes.set` есть два ответа старой дорожки по одному соединению, первый ответ новой приходит не позже 3 секунд после `lanes.set`, после него ответов старой нет; процесс ядра по `tasklist` тот же.
- Шаг 5: основная дорожка «Напрямую» — `curl-free` и `node-free` выходят напрямую, `curl-vpn` — через VPN; основная VPN — обе свободные копии снова через VPN; «всё напрямую» — `curl-free` напрямую, `node-vpn` открывает постоянное соединение; после возврата это соединение закрыто: первый ответ `node-vpn` с адреса VPN не позже 5 секунд (3 секунды сверки 8.3 и один запрос), после него ни одного ответа напрямую, а `curl-vpn` выходит через VPN.
- Шаг 6: `curl-nest` — через VPN, `curl-outer` — напрямую. Без второй копии правило папки не проверялось бы: при основной дорожке VPN копия из вложенной папки выходила бы через VPN и без правил.
- Шаг 7: копия `curl-vpn` раз в 500 мс спрашивает адрес от начала шага 1 до конца шага 8. Ответ учитывается, если ядро (`tasklist` раз в 500 мс) жило всё время запроса и запрос не пересёкся с окном «всё напрямую». Окно открывается запросом включения и закрывается событием `state` с `allDirect: false`, пришедшим после события с `true` или после запроса выключения: события, отправленные во время сверки 8.3, ещё несут прежнее `false`. Учтённый ответ с адреса прямой дорожки — провал; без учтённых ответов шаг тоже не пройден. Строки шагов 7 и 8 печатаются после выключения.
- Шаг 8: exit node и признак доступа к локальной сети системного Tailscale читаются (`status --json` и `debug prefs`, как `Get-P1SystemExitNode`) перед шагом 1 и после `routing.disable` и только сравниваются; `sing-box.exe` не работает; нет ни адаптера `SplitLane`, ни маршрута через него.
- `--heartbeat <файл>` пишет то же, что `Set-P1Heartbeat` (`spike\plan1\p1lib.ps1`): время unix в секундах плюс 400, без перевода строки, через временный файл и переименование, сразу и раз в 30 секунд. Сторож читает `spike\plan1\guard\deadline.txt`.

**Files:**
- Create: `scripts/scenario-lib.mjs`, `scripts/scenario.mjs`
- Test: `scripts/scenario.test.mjs`

**Interfaces:**
- Consumes: канал 5.1 (`hello`, `state.get`, `routing.enable`, `routing.disable`, `routing.allDirect`, `lanes.set`, событие `state`); поля состояния 5.2 `phase`, `reason.code`, `allDirect`, `defaultLane`, `apps`, `addresses`; правило 8.1 `{id, name, exe, matcher:{kind, value:""}, lane}`; формат `guard\deadline.txt` из `Set-P1Heartbeat`; `tailscale.exe status --json` (`BackendState`, `ExitNodeStatus.TailscaleIPs`) и `tailscale.exe debug prefs` (`ExitNodeAllowLANAccess`); `tasklist /fi "imagename eq sing-box.exe" /fo csv /nh`; `Get-NetAdapter`, `Get-NetRoute`.
- Produces: команда `node scripts/scenario.mjs [--pipe \\.\pipe\SplitLane.Helper] [--heartbeat FILE] [--keep]` (строки `SCENARIO <n> PASS|FAIL <text>`, `SCENARIO-PREP`, `SCENARIO-RESTORE PASS|FAIL`, `SCENARIO-ABORT`, `SCENARIO-DONE pass=<k> fail=<m>`; код выхода 0, 1, 2 — неверные аргументы); режимы ребёнка `--child stun HOST PORT` и `--child keep URL MS [PIPE]`. Модуль `scripts/scenario-lib.mjs`: `STUN_MAGIC`, `stunRequest(txId) -> Buffer`, `parseStunResponse(buf, txId) -> {ip, port} | null`; `MAX_LINE`, `createLineReader(onLine, max = MAX_LINE) -> {push(chunk)}`; `createRpc(send) -> {call(method, params, timeoutMs = 30000) -> Promise<{id, result} | {id, error}>, handle(msg) -> boolean, fail(err), pending() -> number}`; `isIPv4(s)`, `addressBook() -> {vpn: [], direct: []}`, `learn(book, addresses) -> book`, `laneOf(ip, book) -> 'vpn' | 'direct' | 'other' | 'none'`, `mask(ip, book)`, `maskText(text, book)`, `lanesKnown(addresses)`; `verdictLane(seen, want) -> {ok, done}`, `verdictSwitch(samples, tSet, oldIp, newIp, limitMs = 3000) -> {ok, firstMs, why}`, `createAllDirectTracker() -> {requested(on, t), observed(allDirect, t), windows()}`, `coreAliveThroughout(core, from, to)`, `verdictLeak(samples, core, windows, book) -> {ok, counted, ignored, leaks}`; `scenarioPaths(root)`, `buildLayout(paths) -> {apps, defaultLane}`, `withLane(layout, exe, lane)`, `withDefault(layout, lane)`; `parseSample(line) -> {t, ip, conn}`, `systemExitNode(statusText, prefsText) -> {installed, on, ip, allowLan} | null`, `sameSystem(a, b)`, `parseTasklist(text) -> number[]`, `heartbeatText(nowMs)`.

- [ ] **Step 1: Написать тесты чистых частей**

Файл `scripts/scenario.test.mjs`, целиком:

```javascript
// Tests of the scenario check of spec 12: node --test scripts/scenario.test.mjs
// The pure parts of scenario-lib.mjs. Test data uses documentation addresses only.
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addressBook, buildLayout, coreAliveThroughout, createAllDirectTracker, createLineReader, createRpc,
  laneOf, lanesKnown, learn, mask, maskText, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault,
  withLane,
} from './scenario-lib.mjs';

const VPN = '198.51.100.7';
const DIRECT = '203.0.113.9';

// stunSuccess is a Binding success response to tx: a SOFTWARE attribute of 5 bytes (padded to 8)
// before an XOR-MAPPED-ADDRESS, IPv4 for an ip, IPv6 when ip is null.
function stunSuccess(tx, ip, port) {
  const software = Buffer.alloc(12);
  software.writeUInt16BE(0x8022, 0);
  software.writeUInt16BE(5, 2);
  software.write('tests', 4, 'latin1');
  let mapped;
  if (ip) {
    mapped = Buffer.alloc(12);
    mapped.writeUInt16BE(0x0020, 0);
    mapped.writeUInt16BE(8, 2);
    mapped[5] = 0x01;
    mapped.writeUInt16BE(port ^ 0x2112, 6);
    const n = ip.split('.').reduce((a, octet) => a * 256 + Number(octet), 0);
    mapped.writeUInt32BE((n ^ 0x2112a442) >>> 0, 8);
  } else {
    mapped = Buffer.alloc(24);
    mapped.writeUInt16BE(0x0020, 0);
    mapped.writeUInt16BE(20, 2);
    mapped[5] = 0x02;
  }
  const head = Buffer.alloc(20);
  head.writeUInt16BE(0x0101, 0);
  head.writeUInt16BE(software.length + mapped.length, 2);
  head.writeUInt32BE(0x2112a442, 4);
  tx.copy(head, 8);
  return Buffer.concat([head, software, mapped]);
}

test('stun request and response', () => {
  const tx = Buffer.from('0123456789ab');
  const req = stunRequest(tx);
  assert.equal(req.length, 20);
  assert.equal(req.readUInt16BE(0), 0x0001, 'Binding request');
  assert.equal(req.readUInt16BE(2), 0, 'no attributes');
  assert.equal(req.readUInt32BE(4), 0x2112a442, 'magic cookie');
  assert.deepEqual(req.subarray(8), tx);
  assert.throws(() => stunRequest(Buffer.alloc(11)), /12 bytes/);

  const res = stunSuccess(tx, '203.0.113.7', 54321);
  assert.deepEqual(parseStunResponse(res, tx), { ip: '203.0.113.7', port: 54321 });
  assert.equal(parseStunResponse(res, Buffer.from('ba9876543210')), null, 'another transaction');
  assert.equal(parseStunResponse(res.subarray(0, 30), tx), null, 'cut short');
  const failure = Buffer.from(res);
  failure.writeUInt16BE(0x0111, 0);
  assert.equal(parseStunResponse(failure, tx), null, 'an error response');
  assert.equal(parseStunResponse(stunSuccess(tx, null, 0), tx), null, 'IPv6 only');
});

test('line reader splits and joins chunks', () => {
  const lines = [];
  const reader = createLineReader((line) => lines.push(line));
  reader.push(Buffer.from('{"a":1}\n{"b"'));
  reader.push(Buffer.from(':2}\r\n\n'));
  const zhe = Buffer.from('"\u0416"\n', 'utf8'); // a two-byte character split between two chunks
  reader.push(zhe.subarray(0, 2));
  reader.push(zhe.subarray(2));
  reader.push(Buffer.from('{"c":3}\n{"d":4}\n'));
  assert.deepEqual(lines, ['{"a":1}', '{"b":2}', '"\u0416"', '{"c":3}', '{"d":4}']);

  const small = createLineReader(() => {}, 8);
  small.push(Buffer.from('12345678\n'));
  assert.throws(() => small.push(Buffer.from('123456789')), /longer than 8/);
});

test('requests match replies by id', async () => {
  const sent = [];
  const rpc = createRpc((line) => sent.push(JSON.parse(line)));
  const hello = rpc.call('hello', { protocol: 1 });
  const state = rpc.call('state.get');
  assert.deepEqual(sent, [{ id: 1, method: 'hello', params: { protocol: 1 } }, { id: 2, method: 'state.get' }]);
  assert.equal(rpc.handle({ event: 'state', data: { phase: 'on' } }), false, 'an event');
  assert.equal(rpc.handle({ id: 9, result: {} }), false, 'nobody asked');
  assert.equal(rpc.handle({ id: 2, result: { phase: 'off' } }), true);
  assert.equal(rpc.handle({ id: 1, error: { code: 'protocol_mismatch', message: 'm', protocol: 2 } }), true);
  assert.equal(rpc.handle({ id: 1, result: {} }), false, 'answered once');
  assert.deepEqual(await state, { id: 2, result: { phase: 'off' } });
  assert.equal((await hello).error.code, 'protocol_mismatch');

  await assert.rejects(rpc.call('routing.enable', undefined, 20), /no reply to routing.enable within 20 ms/);
  const pending = rpc.call('lanes.set', {});
  rpc.fail(new Error('the pipe closed'));
  await assert.rejects(pending, /the pipe closed/);
  assert.equal(rpc.pending(), 0);
});

test('mask hides addresses', () => {
  const book = addressBook();
  learn(book, { vpn: VPN, direct: DIRECT });
  learn(book, { vpn: null, direct: null });
  learn(book, { vpn: '198.51.100.8', direct: DIRECT });
  assert.deepEqual(book, { vpn: [VPN, '198.51.100.8'], direct: [DIRECT] });
  assert.equal(laneOf('198.51.100.8', book), 'vpn');
  assert.equal(laneOf(DIRECT, book), 'direct');
  assert.equal(laneOf('192.0.2.1', book), 'other');
  assert.equal(laneOf(null, book), 'none');
  assert.equal(mask(DIRECT, book), '<direct>');
  assert.equal(
    maskText(`vpn ${VPN}, direct ${DIRECT}, other 192.0.2.1, core 1.14.4`, book),
    'vpn <vpn>, direct <direct>, other <other>, core 1.14.4');
  assert.equal(maskText(`before the state: ${DIRECT}`, addressBook()), 'before the state: <other>');
});

test('lane verdict wants the own address', () => {
  assert.deepEqual(verdictLane(['vpn'], 'vpn'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['direct'], 'vpn'), { ok: false, done: true });
  assert.deepEqual(verdictLane(['none', 'other'], 'vpn'), { ok: false, done: false });
  assert.deepEqual(verdictLane(['other', 'vpn', 'vpn'], 'vpn'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['other', 'vpn', 'direct'], 'vpn'), { ok: false, done: true });
  assert.deepEqual(verdictLane(['vpn', 'direct', 'direct'], 'direct'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['vpn', 'other', 'direct'], 'direct'), { ok: false, done: true });
  assert.deepEqual(verdictLane(Array(6).fill('none'), 'direct'), { ok: false, done: true });
  assert.equal(lanesKnown({ vpn: VPN, direct: DIRECT }), true);
  assert.equal(lanesKnown({ vpn: VPN, direct: VPN }), false);
  assert.equal(lanesKnown({ vpn: null, direct: DIRECT }), false);
  assert.equal(lanesKnown(undefined), false);
});

test('switch verdict passes and fails', () => {
  const at = (t, ip, conn = 1) => ({ t, ip, conn });
  const good = [at(100, VPN), at(350, VPN), at(600, VPN), at(1100, VPN), at(1400, null), at(1950, DIRECT, 2), at(1700, DIRECT, 2)];
  assert.deepEqual(verdictSwitch(good, 1000, VPN, DIRECT), { ok: true, firstMs: 700, why: '' });

  const late = [at(350, VPN), at(600, VPN), at(4100, DIRECT, 2)];
  assert.deepEqual(verdictSwitch(late, 1000, VPN, DIRECT),
    { ok: false, firstMs: 3100, why: 'the first reply from the new lane came later than 3000 ms' });
  assert.equal(verdictSwitch(late, 1000, VPN, DIRECT, 5000).ok, true, 'a longer limit');

  assert.deepEqual(verdictSwitch([at(350, VPN), at(600, VPN), at(1500, DIRECT, 2), at(1750, VPN, 3)], 1000, VPN, DIRECT),
    { ok: false, firstMs: 500, why: 'a reply from the old lane after the first one from the new lane' });
  assert.equal(verdictSwitch([at(1500, DIRECT)], 1000, VPN, DIRECT).why, 'no reply from the old lane before the switch');
  assert.equal(verdictSwitch([at(350, VPN, 1), at(600, VPN, 2), at(1500, DIRECT, 3)], 1000, VPN, DIRECT).why,
    'the copy did not keep its connection before the switch');
  assert.deepEqual(verdictSwitch([at(350, VPN), at(1000, VPN), at(1000, DIRECT, 2), at(1300, DIRECT, 2)], 1000, VPN, DIRECT),
    { ok: true, firstMs: 300, why: '' }, 'a reply stamped with the time of the switch came before it');
  assert.equal(verdictSwitch([at(350, VPN), at(600, VPN), at(1500, VPN)], 1000, VPN, DIRECT).why, 'no reply from the new lane');
  assert.equal(verdictSwitch([at(350, VPN), at(600, VPN), at(800, DIRECT, 2), at(1500, VPN, 3), at(1600, DIRECT, 4)], 1000,
    VPN, DIRECT).firstMs, 600, 'a reply from before the switch is not the first one');
});

test('leak verdict ignores all-direct', () => {
  const book = learn(addressBook(), { vpn: VPN, direct: DIRECT });
  const tracker = createAllDirectTracker();
  tracker.requested(true, 1000);
  tracker.observed(false, 1200); // a state sent while the switch is applied still says false
  tracker.observed(true, 2500);
  tracker.requested(false, 5000);
  tracker.observed(true, 5100); // the switch back is applied
  tracker.observed(false, 7000);
  tracker.observed(false, 7500);
  tracker.observed(true, 12000); // switched on by another client
  tracker.observed(false, 12500);
  const windows = tracker.windows();
  assert.deepEqual(windows, [{ from: 1000, to: 7000 }, { from: 12000, to: 12500 }]);
  const open = createAllDirectTracker();
  open.requested(true, 100);
  open.observed(false, 200);
  assert.deepEqual(open.windows(), [{ from: 100, to: Infinity }], 'not closed before it was on');

  const core = [{ t: 0, alive: false }];
  for (let t = 400; t <= 8900; t += 500) core.push({ t, alive: true });
  core.push({ t: 9400, alive: false }, { t: 9900, alive: false });
  const s = (sent, done, ip) => ({ sent, done, ip });
  const samples = [
    s(100, 300, DIRECT), // no core yet
    s(500, 700, VPN),
    s(900, 1100, DIRECT), // all direct begins during the request
    s(3000, 3200, DIRECT), // all direct
    s(6900, 7100, DIRECT), // all direct ends during the request
    s(7600, 7800, VPN),
    s(8000, 8200, null), // no reply: counted, not a leak
    s(8800, 9200, DIRECT), // the core may have ended during the request
    s(9500, 9700, DIRECT), // no core any more
  ];
  assert.deepEqual(verdictLeak(samples, core, windows, book), { ok: true, counted: 3, ignored: 6, leaks: 0 });
  assert.deepEqual(verdictLeak([...samples, s(8100, 8300, DIRECT)], core, windows, book),
    { ok: false, counted: 4, ignored: 6, leaks: 1 });
  assert.equal(verdictLeak([s(100, 300, VPN)], core, windows, book).ok, false, 'nothing counted proves nothing');

  assert.equal(coreAliveThroughout(core, 500, 700), true);
  assert.equal(coreAliveThroughout(core, 300, 500), false);
  assert.equal(coreAliveThroughout([{ t: 0, alive: true }], 100, 200), false, 'no look after the reply');
  assert.equal(coreAliveThroughout([{ t: 0, alive: true }, { t: 300, alive: null }], 100, 200), false, 'tasklist failed');
});

test('layout has unique uuids and file rules', () => {
  const p = scenarioPaths('C:\\Temp\\splitlane-scenario');
  assert.equal(p.curlNest, 'C:\\Temp\\splitlane-scenario\\nest\\inner\\curl-nest.exe');
  assert.equal(p.curlOuter, 'C:\\Temp\\splitlane-scenario\\nest\\curl-outer.exe');
  const exes = Object.values(p).filter((x) => x.endsWith('.exe'));
  assert.deepEqual(exes.map((x) => x.split('\\').pop()).sort(), ['curl-direct.exe', 'curl-free.exe', 'curl-nest.exe',
    'curl-outer.exe', 'curl-vpn.exe', 'node-direct.exe', 'node-free.exe', 'node-vpn.exe']);
  const folders = new Set([p.curlVpn, p.curlDirect, p.curlFree].map((x) => x.slice(0, x.lastIndexOf('\\'))));
  assert.equal(folders.size, 3, 'one folder per lane');

  const layout = buildLayout(p);
  assert.equal(layout.defaultLane, 'vpn');
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
  assert.ok(layout.apps.every((a) => uuid.test(a.id)), 'UUIDs');
  assert.equal(new Set(layout.apps.map((a) => a.id)).size, layout.apps.length, 'unique');
  assert.notEqual(buildLayout(p).apps[0].id, layout.apps[0].id, 'new ids on every run');
  assert.deepEqual(layout.apps.map((a) => [a.exe, a.matcher.kind, a.matcher.value, a.lane]), [
    [p.curlVpn, 'file', '', 'vpn'],
    [p.nodeVpn, 'file', '', 'vpn'],
    [p.curlDirect, 'file', '', 'direct'],
    [p.nodeDirect, 'file', '', 'direct'],
    [p.curlOuter, 'folder', '', 'direct'],
    [p.curlNest, 'file', '', 'vpn'],
  ]);
  assert.deepEqual(layout.apps.map((a) => a.name), ['scenario curl-vpn', 'scenario node-vpn', 'scenario curl-direct',
    'scenario node-direct', 'scenario nest', 'scenario curl-nest']);

  const moved = withLane(layout, p.nodeVpn, 'direct');
  assert.equal(moved.apps.find((a) => a.exe === p.nodeVpn).lane, 'direct');
  assert.equal(layout.apps.find((a) => a.exe === p.nodeVpn).lane, 'vpn', 'the original stays');
  assert.deepEqual(moved.apps.map((a) => a.id), layout.apps.map((a) => a.id), 'the same ids');
  assert.equal(withDefault(layout, 'direct').defaultLane, 'direct');
  assert.equal(layout.defaultLane, 'vpn');
});

test('system output is parsed and compared', () => {
  const status = JSON.stringify({ BackendState: 'Running',
    ExitNodeStatus: { ID: 'n1', Online: true, TailscaleIPs: ['fd7a:115c:a1e0::5/128', '100.64.0.5/32'] } });
  const on = systemExitNode(status, '{"ExitNodeAllowLANAccess":true}');
  assert.deepEqual(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: true });
  assert.deepEqual(systemExitNode('{"BackendState":"Running","ExitNodeStatus":null}', '{}'),
    { installed: true, on: false, ip: null, allowLan: false });
  assert.deepEqual(systemExitNode(JSON.stringify({ BackendState: 'Stopped', ExitNodeStatus: { TailscaleIPs: ['100.64.0.5/32'] } }),
    '{"ExitNodeAllowLANAccess":true}'), { installed: true, on: false, ip: null, allowLan: true });
  assert.equal(systemExitNode('', '{}'), null);

  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: true }), true);
  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: false }), false);
  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.6', allowLan: true }), false);
  assert.equal(sameSystem({ installed: false }, { installed: false }), true);
  assert.equal(sameSystem(null, null), false, 'unreadable is never equal');

  assert.deepEqual(parseTasklist('"sing-box.exe","1234","Services","0","25,000 K"\r\n"Sing-Box.exe","88","Console","1","1,024 K"\r\n'),
    [1234, 88]);
  assert.deepEqual(parseTasklist('INFO: No tasks are running which match the specified criteria.\r\n'), []);
});
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router; node --test --test-reporter=spec scripts\scenario.test.mjs
```

Ожидается: `Error [ERR_MODULE_NOT_FOUND]: Cannot find module 'C:\code\router\scripts\scenario-lib.mjs' imported from C:\code\router\scripts\scenario.test.mjs`, затем `✖ scripts\scenario.test.mjs` и `ℹ fail 1`.

- [ ] **Step 3: Реализовать `scenario-lib.mjs`**

Файл `scripts/scenario-lib.mjs`, целиком:

```javascript
// Pure parts of the scenario check of spec 12 (scripts/scenario.mjs): STUN messages (RFC 5389), the
// line framing of the pipe and the matching of replies to requests (spec 5.1), the lane addresses
// and their masks, the layout of the renamed copies (spec 8.1), the verdicts of the steps and the
// parsing of what tailscale.exe and tasklist.exe print. Nothing here does I/O.
import { randomUUID } from 'node:crypto';
import { win32 } from 'node:path';
import { StringDecoder } from 'node:string_decoder';

export const STUN_MAGIC = 0x2112a442;
const STUN_BINDING_REQUEST = 0x0001;
const STUN_BINDING_SUCCESS = 0x0101;
const STUN_XOR_MAPPED_ADDRESS = 0x0020;
const STUN_FAMILY_IPV4 = 0x01;

// stunRequest is a Binding request without attributes; txId is the 12-byte transaction id.
export function stunRequest(txId) {
  if (txId.length !== 12) throw new Error('a STUN transaction id is 12 bytes');
  const b = Buffer.alloc(20);
  b.writeUInt16BE(STUN_BINDING_REQUEST, 0);
  b.writeUInt16BE(0, 2);
  b.writeUInt32BE(STUN_MAGIC, 4);
  Buffer.from(txId).copy(b, 8);
  return b;
}

// parseStunResponse gives {ip, port} of the IPv4 XOR-MAPPED-ADDRESS in a Binding success response
// to txId, or null for anything else: another transaction, an error response, a cut message.
export function parseStunResponse(buf, txId) {
  if (buf.length < 20 || buf.readUInt16BE(0) !== STUN_BINDING_SUCCESS || buf.readUInt32BE(4) !== STUN_MAGIC) {
    return null;
  }
  if (!buf.subarray(8, 20).equals(Buffer.from(txId))) return null;
  const end = 20 + buf.readUInt16BE(2);
  if (end > buf.length) return null;
  for (let at = 20; at + 4 <= end;) {
    const type = buf.readUInt16BE(at);
    const len = buf.readUInt16BE(at + 2);
    const value = at + 4;
    if (value + len > end) return null;
    if (type === STUN_XOR_MAPPED_ADDRESS && len >= 8 && buf[value + 1] === STUN_FAMILY_IPV4) {
      const port = buf.readUInt16BE(value + 2) ^ (STUN_MAGIC >>> 16);
      const addr = (buf.readUInt32BE(value + 4) ^ STUN_MAGIC) >>> 0;
      return { ip: [addr >>> 24, (addr >>> 16) & 255, (addr >>> 8) & 255, addr & 255].join('.'), port };
    }
    at = value + len + ((4 - (len % 4)) % 4); // attributes are padded to 4 bytes
  }
  return null;
}

// MAX_LINE is the longest message line of the pipe (spec 5.1: 1 MiB).
export const MAX_LINE = 1 << 20;

// createLineReader frames a byte stream into lines: push takes the chunks as they come; a line may
// span chunks, a chunk may hold many lines, a character split between chunks is joined. A trailing
// \r is dropped and empty lines are skipped; a line longer than max characters throws.
export function createLineReader(onLine, max = MAX_LINE) {
  const decoder = new StringDecoder('utf8');
  let rest = '';
  const tooLong = () => new Error(`a line longer than ${max} characters`);
  return {
    push(chunk) {
      rest += typeof chunk === 'string' ? chunk : decoder.write(chunk);
      for (let nl = rest.indexOf('\n'); nl >= 0; nl = rest.indexOf('\n')) {
        const line = rest.slice(0, nl).replace(/\r$/, '');
        rest = rest.slice(nl + 1);
        if (line.length > max) throw tooLong();
        if (line !== '') onLine(line);
      }
      if (rest.length > max) throw tooLong();
    },
  };
}

// createRpc numbers the requests (spec 5.1: {id, method, params}) and matches each reply to its
// request by id. send writes one line. call resolves with the reply, {id, result} or {id, error};
// it rejects only when no reply comes within timeoutMs or the connection ends (fail).
export function createRpc(send) {
  let next = 1;
  const pending = new Map();
  return {
    call(method, params, timeoutMs = 30000) {
      const id = next++;
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          pending.delete(id);
          reject(new Error(`no reply to ${method} within ${timeoutMs} ms`));
        }, timeoutMs);
        pending.set(id, { resolve, reject, timer });
        send(JSON.stringify(params === undefined ? { id, method } : { id, method, params }) + '\n');
      });
    },
    // handle takes one parsed message and says whether it was the reply to a pending call.
    handle(msg) {
      if (msg === null || typeof msg !== 'object' || 'event' in msg) return false;
      const p = pending.get(msg.id);
      if (!p) return false;
      pending.delete(msg.id);
      clearTimeout(p.timer);
      p.resolve(msg);
      return true;
    },
    fail(err) {
      for (const p of pending.values()) {
        clearTimeout(p.timer);
        p.reject(err);
      }
      pending.clear();
    },
    pending: () => pending.size,
  };
}

export function isIPv4(s) {
  return typeof s === 'string' && /^\d{1,3}(\.\d{1,3}){3}$/.test(s) && s.split('.').every((n) => Number(n) <= 255);
}

// addressBook keeps every lane address the helper has reported in state.addresses (spec 5.2);
// learn adds the addresses of one state and returns the book.
export function addressBook() {
  return { vpn: [], direct: [] };
}

export function learn(book, addresses) {
  for (const lane of ['vpn', 'direct']) {
    const ip = addresses?.[lane];
    if (isIPv4(ip) && !book[lane].includes(ip)) book[lane].push(ip);
  }
  return book;
}

// laneOf names the lane of an address: vpn, direct, other, or none when there was no reply.
export function laneOf(ip, book) {
  if (!ip) return 'none';
  if (book.direct.includes(ip)) return 'direct';
  if (book.vpn.includes(ip)) return 'vpn';
  return 'other';
}

export function mask(ip, book) {
  return `<${laneOf(ip, book)}>`;
}

// maskText replaces every IPv4 address in a text by its lane: the output never carries an address.
export function maskText(text, book) {
  return String(text).replace(/\b\d{1,3}(?:\.\d{1,3}){3}\b/g, (ip) => mask(ip, book));
}

// lanesKnown: step 1 needs both lane addresses, and different ones (spec 12).
export function lanesKnown(addresses) {
  return isIPv4(addresses?.vpn) && isIPv4(addresses?.direct) && addresses.vpn !== addresses.direct;
}

// verdictLane judges the lanes of the replies of one copy (laneOf of each try), like slctl
// expect-lanes: a copy of the VPN lane fails at once on a reply from the direct address; otherwise
// the first reply is right, or two of the first three are (the exit node's external address may
// change). done is false while more tries are needed; six tries at most.
export function verdictLane(seen, want) {
  const answers = seen.filter((lane) => lane !== 'none');
  if (want === 'vpn' && answers.includes('direct')) return { ok: false, done: true };
  if (answers.length > 0 && answers[0] === want) return { ok: true, done: true };
  if (answers.length >= 3) return { ok: answers.slice(0, 3).filter((lane) => lane === want).length >= 2, done: true };
  if (seen.length >= 6) return { ok: false, done: true };
  return { ok: false, done: false };
}

// verdictSwitch is step 4 (and the return from "all direct" in step 5): samples are {t, ip, conn}
// replies of one keep-alive copy, conn the number of its connection; tSet is when the switch was
// sent, and a reply stamped tSet came before it. Before the switch the copy answered from oldIp
// at least twice over one connection (it keeps the connection); the first reply from newIp comes
// within limitMs after tSet, and no reply from oldIp follows it. firstMs is the time from tSet to
// that first reply.
export function verdictSwitch(samples, tSet, oldIp, newIp, limitMs = 3000) {
  const sorted = [...samples].sort((a, b) => a.t - b.t);
  const old = sorted.filter((s) => s.t <= tSet && s.ip === oldIp);
  if (old.length === 0) return { ok: false, firstMs: null, why: 'no reply from the old lane before the switch' };
  if (!old.some((s, i) => old.findIndex((o) => o.conn === s.conn) !== i)) {
    return { ok: false, firstMs: null, why: 'the copy did not keep its connection before the switch' };
  }
  const first = sorted.find((s) => s.t > tSet && s.ip === newIp);
  if (!first) return { ok: false, firstMs: null, why: 'no reply from the new lane' };
  const firstMs = first.t - tSet;
  if (firstMs > limitMs) {
    return { ok: false, firstMs, why: `the first reply from the new lane came later than ${limitMs} ms` };
  }
  if (sorted.some((s) => s.t > first.t && s.ip === oldIp)) {
    return { ok: false, firstMs, why: 'a reply from the old lane after the first one from the new lane' };
  }
  return { ok: true, firstMs, why: '' };
}

// createAllDirectTracker keeps the windows of "all direct" for step 7. The scenario tells when it
// asks for the switch (requested) and what each state event says (observed). A window opens at the
// request to switch on, or at a state with allDirect true. It closes at a state with allDirect
// false that comes after a state with true or after the request to switch off: the states sent
// while the switch is still being applied (up to 3 s, spec 8.3) carry the old false.
export function createAllDirectTracker() {
  const windows = [];
  let open = null;
  const begin = (t, seenOn) => {
    open = { from: t, to: Infinity, seenOn, offAsked: false };
    windows.push(open);
  };
  return {
    requested(on, t) {
      if (on && !open) begin(t, false);
      else if (!on && open) open.offAsked = true;
    },
    observed(allDirect, t) {
      if (allDirect) {
        if (open) open.seenOn = true;
        else begin(t, true);
      } else if (open && (open.seenOn || open.offAsked)) {
        open.to = t;
        open = null;
      }
    },
    windows: () => windows.map(({ from, to }) => ({ from, to })),
  };
}

// coreAliveThroughout: core is the list of looks at the core process, {t, alive} (alive null when
// the look failed). True when the last look at or before from and every look up to the first one
// at or after to saw the core.
export function coreAliveThroughout(core, from, to) {
  let start = -1;
  for (let i = 0; i < core.length; i++) {
    if (core[i].t <= from) start = i;
  }
  if (start < 0) return false;
  for (let i = start; i < core.length; i++) {
    if (core[i].alive !== true) return false;
    if (core[i].t >= to) return true;
  }
  return false;
}

// verdictLeak is step 7: samples are {sent, done, ip} replies of the VPN copy of curl during the
// whole scenario. A reply counts when the core lived through the request and the request did not
// overlap a window of "all direct"; a counted reply from a direct address is a leak. With no
// counted reply there is nothing to judge, and the step fails.
export function verdictLeak(samples, core, windows, book) {
  let counted = 0;
  let ignored = 0;
  let leaks = 0;
  for (const s of samples) {
    const allDirect = windows.some((w) => s.sent <= w.to && s.done >= w.from);
    if (allDirect || !coreAliveThroughout(core, s.sent, s.done)) {
      ignored++;
      continue;
    }
    counted++;
    if (laneOf(s.ip, book) === 'direct') leaks++;
  }
  return { ok: counted > 0 && leaks === 0, counted, ignored, leaks };
}

// scenarioPaths are the copies of spec 12 under root (%TEMP%\splitlane-scenario): a folder for each
// lane and one for the free copies; for step 6 the folder nest, pinned direct as a whole, with
// nest\curl-outer.exe and nest\inner\curl-nest.exe (pinned vpn).
export function scenarioPaths(root) {
  const at = (...parts) => win32.join(root, ...parts);
  return {
    root,
    curlVpn: at('vpn', 'curl-vpn.exe'),
    nodeVpn: at('vpn', 'node-vpn.exe'),
    curlDirect: at('direct', 'curl-direct.exe'),
    nodeDirect: at('direct', 'node-direct.exe'),
    curlFree: at('free', 'curl-free.exe'),
    nodeFree: at('free', 'node-free.exe'),
    curlOuter: at('nest', 'curl-outer.exe'),
    curlNest: at('nest', 'inner', 'curl-nest.exe'),
  };
}

// buildLayout is the lanes.set of the scenario (spec 8.1): new UUIDs, an empty matcher.value (the
// helper computes it), kind file for each pinned copy and kind folder for nest; the free copies
// are not pinned; the main lane is vpn.
export function buildLayout(p) {
  const rule = (exe, kind, lane) => ({
    id: randomUUID(),
    name: `scenario ${kind === 'folder' ? win32.basename(win32.dirname(exe)) : win32.basename(exe, '.exe')}`,
    exe,
    matcher: { kind, value: '' },
    lane,
  });
  return {
    apps: [
      rule(p.curlVpn, 'file', 'vpn'),
      rule(p.nodeVpn, 'file', 'vpn'),
      rule(p.curlDirect, 'file', 'direct'),
      rule(p.nodeDirect, 'file', 'direct'),
      rule(p.curlOuter, 'folder', 'direct'),
      rule(p.curlNest, 'file', 'vpn'),
    ],
    defaultLane: 'vpn',
  };
}

// withLane is a copy of the layout in which the rule of exe goes to lane, with the same ids.
export function withLane(layout, exe, lane) {
  return { ...layout, apps: layout.apps.map((a) => (a.exe === exe ? { ...a, matcher: { ...a.matcher }, lane } : a)) };
}

export function withDefault(layout, lane) {
  return { ...layout, defaultLane: lane };
}

// parseSample reads a line of a child: "<unix ms> <ip>", or "<unix ms> -" when there was no reply;
// the keep child adds the number of the connection: "<unix ms> <ip> <conn>".
export function parseSample(line) {
  const [t, ip, conn] = String(line).trim().split(/\s+/);
  return { t: Number(t), ip: isIPv4(ip) ? ip : null, conn: conn === undefined ? null : Number(conn) };
}

// systemExitNode reads the system Tailscale client as Get-P1SystemExitNode does (spike\plan1\p1lib.ps1):
// the exit node from `tailscale status --json` (ExitNodeStatus.TailscaleIPs, the IPv4 one without its
// prefix length, only while BackendState is Running), the LAN flag from `tailscale debug prefs`
// (ExitNodeAllowLANAccess). null when either output is not JSON.
export function systemExitNode(statusText, prefsText) {
  let status;
  let prefs;
  try {
    status = JSON.parse(statusText);
    prefs = JSON.parse(prefsText);
  } catch {
    return null;
  }
  let ip = null;
  if (status?.BackendState === 'Running' && status.ExitNodeStatus) {
    const ips = (status.ExitNodeStatus.TailscaleIPs ?? []).map((a) => String(a).split('/')[0]);
    ip = ips.find((a) => !a.includes(':')) ?? null;
  }
  return { installed: true, on: ip !== null, ip, allowLan: Boolean(prefs?.ExitNodeAllowLANAccess) };
}

// sameSystem compares two readings of the system client (step 8); an unreadable one (null) never
// equals anything.
export function sameSystem(a, b) {
  return a !== null && b !== null && ['installed', 'on', 'ip', 'allowLan'].every((k) => a[k] === b[k]);
}

// parseTasklist gives the process ids of sing-box.exe in `tasklist /fo csv /nh` output.
export function parseTasklist(text) {
  const pids = [];
  for (const line of String(text).split(/\r?\n/)) {
    const m = /^"([^"]+)","(\d+)"/.exec(line.trim());
    if (m && m[1].toLowerCase() === 'sing-box.exe') pids.push(Number(m[2]));
  }
  return pids;
}

// heartbeatText is what Set-P1Heartbeat (spike\plan1\p1lib.ps1) writes to guard\deadline.txt:
// the unix time in seconds 400 s from now, no newline.
export function heartbeatText(nowMs) {
  return String(Math.floor(nowMs / 1000) + 400);
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router; node --test --test-reporter=spec scripts\scenario.test.mjs
```

Ожидается девять строк `✔`: `✔ stun request and response`, `✔ line reader splits and joins chunks`, `✔ requests match replies by id`, `✔ mask hides addresses`, `✔ lane verdict wants the own address`, `✔ switch verdict passes and fails`, `✔ leak verdict ignores all-direct`, `✔ layout has unique uuids and file rules`, `✔ system output is parsed and compared`; затем `ℹ pass 9` и `ℹ fail 0`.

- [ ] **Step 5: Написать тесты самого `scenario.mjs`**

Отказ при фазе не `off` (только `hello` и `state.get`, файл сторожа записан) и ребёнок `keep` (одно соединение, после его закрытия — новое) на каналах `SplitLane.Test.<число>`.

Изменить `scripts/scenario.test.mjs`:

Найти:

```javascript
// Tests of the scenario check of spec 12: node --test scripts/scenario.test.mjs
// The pure parts of scenario-lib.mjs. Test data uses documentation addresses only.
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addressBook, buildLayout, coreAliveThroughout, createAllDirectTracker, createLineReader, createRpc,
  laneOf, lanesKnown, learn, mask, maskText, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault,
  withLane,
} from './scenario-lib.mjs';
```

Заменить на:

```javascript
// Tests of the scenario check of spec 12: node --test scripts/scenario.test.mjs
// The pure parts of scenario-lib.mjs, and scenario.mjs against a fake helper and a fake address
// service on named pipes SplitLane.Test.<number>. Test data uses documentation addresses only.
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import http from 'node:http';
import net from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  addressBook, buildLayout, coreAliveThroughout, createAllDirectTracker, createLineReader, createRpc,
  laneOf, lanesKnown, learn, mask, maskText, parseSample, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault,
  withLane,
} from './scenario-lib.mjs';

const SCENARIO = fileURLToPath(new URL('./scenario.mjs', import.meta.url));

// runNode runs this node with args and gives the exit code and the output.
function runNode(args) {
  return new Promise((resolve) => {
    execFile(process.execPath, args, { timeout: 20000, windowsHide: true }, (err, stdout, stderr) => {
      resolve({ code: err ? err.code : 0, stdout, stderr });
    });
  });
}

function listen(server, pipe) {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(pipe, resolve);
  });
}
```

Найти:

```javascript
  assert.deepEqual(parseTasklist('INFO: No tasks are running which match the specified criteria.\r\n'), []);
});
```

Заменить на:

```javascript
  assert.deepEqual(parseTasklist('INFO: No tasks are running which match the specified criteria.\r\n'), []);
});

test('scenario refuses a helper that is not off', async () => {
  const pipe = `\\\\.\\pipe\\SplitLane.Test.${process.pid}1`;
  const methods = [];
  const server = net.createServer((conn) => {
    conn.on('error', () => {});
    const reader = createLineReader((line) => {
      const req = JSON.parse(line);
      methods.push(req.method);
      const result = req.method === 'hello'
        ? { protocol: 1, helper: '0.0.0', core: '1.14.4' }
        : { phase: 'on', reason: null, allDirect: false, defaultLane: 'vpn', apps: [], addresses: { vpn: VPN, direct: DIRECT } };
      conn.write(JSON.stringify({ id: req.id, result }) + '\n');
    });
    conn.on('data', (chunk) => reader.push(chunk));
  });
  await listen(server, pipe);
  const dir = mkdtempSync(join(tmpdir(), 'scenario-test-'));
  const heartbeat = join(dir, 'guard', 'deadline.txt');
  try {
    const r = await runNode([SCENARIO, '--pipe', pipe, '--heartbeat', heartbeat]);
    assert.deepEqual(r.stdout.trim().split(/\r?\n/), [
      'SCENARIO 0 FAIL the helper is in phase on; the scenario starts from phase off',
      'SCENARIO-DONE pass=0 fail=1',
    ], r.stderr);
    assert.equal(r.code, 1);
    assert.deepEqual(methods, ['hello', 'state.get'], 'nothing is changed');
    const text = readFileSync(heartbeat, 'utf8');
    assert.match(text, /^\d+$/);
    const ahead = Number(text) - Math.floor(Date.now() / 1000);
    assert.ok(ahead > 380 && ahead <= 400, `the deadline is ${ahead} s ahead`);
  } finally {
    server.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test('keep child reuses one connection and reconnects', async () => {
  const pipe = `\\\\.\\pipe\\SplitLane.Test.${process.pid}2`;
  const conns = new Map();
  const server = http.createServer((req, res) => {
    const c = conns.get(req.socket);
    c.requests++;
    if (c.n === 1 && c.requests === 3) {
      req.socket.destroy(); // like the helper closing a connection on the wrong lane (spec 8.3)
      return;
    }
    res.end(c.n === 1 ? '192.0.2.1' : '192.0.2.2');
  });
  server.on('connection', (socket) => conns.set(socket, { n: conns.size + 1, requests: 0 }));
  await listen(server, pipe);
  try {
    const r = await runNode([SCENARIO, '--child', 'keep', 'http://scenario.test/', '1800', pipe]);
    assert.equal(r.code, 0, r.stderr);
    const samples = r.stdout.trim().split(/\r?\n/).map(parseSample);
    const ips = samples.map((s) => s.ip);
    assert.deepEqual(ips.slice(0, 4), ['192.0.2.1', '192.0.2.1', null, '192.0.2.2']);
    assert.ok(ips.slice(3).every((ip) => ip === '192.0.2.2'), ips.join(' '));
    const numbers = samples.map((s) => s.conn);
    assert.deepEqual(numbers.slice(0, 4), [1, 1, 1, 2], 'the number of the connection of each reply');
    assert.ok(numbers.slice(3).every((n) => n === 2), numbers.join(' '));
    assert.ok(samples.length >= 4 && samples.length <= 9, `${samples.length} replies in 1.8 s`);
    assert.ok(samples.every((s, i) => Number.isInteger(s.t) && (i === 0 || s.t >= samples[i - 1].t)));
    assert.equal(conns.size, 2, 'one connection until it was closed, then one more');
  } finally {
    server.close();
  }
});
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router; node --test --test-reporter=spec scripts\scenario.test.mjs
```

Ожидается: `✖ scenario refuses a helper that is not off` и `✖ keep child reuses one connection and reconnects`, у обоих `Error: Cannot find module 'C:\code\router\scripts\scenario.mjs'` (в первом ещё `+   ''` вместо строк `SCENARIO`, во втором `1 !== 0`); `ℹ pass 9`, `ℹ fail 2`.

- [ ] **Step 7: Реализовать `scenario.mjs`**

Файл `scripts/scenario.mjs`, целиком:

```javascript
// The scenario check of spec 12: a second client of the helper's pipe (spec 5.1). It pins renamed
// copies of curl.exe and node.exe to the two lanes, enables routing, checks both lanes over TCP and
// UDP, moves a copy between the lanes, changes the main lane and "all direct", checks nested rules,
// watches the VPN copy of curl for leaks the whole time and disables routing again (steps 1-8).
// It is run by hand next to a running helper (plan 1B, task 16); the tests run only its refusal
// and its child. Addresses are never printed: the lane addresses of state.addresses become <vpn>
// and <direct>, any other address <other>.
//
// usage: node scripts/scenario.mjs [--pipe \\.\pipe\SplitLane.Helper] [--heartbeat FILE] [--keep]
//   --heartbeat FILE  every 30 s write the deadline of the plan 1A guard to FILE as Set-P1Heartbeat
//                     does (spike\plan1\p1lib.ps1); the guard reads spike\plan1\guard\deadline.txt
//   --keep            leave the layout of the scenario in place instead of the previous one
// Output: SCENARIO <n> PASS|FAIL <text> for each step (n 0: the preparation failed and nothing was
// changed), SCENARIO-RESTORE, then SCENARIO-DONE pass=<k> fail=<m>. The exit code is 0 only when
// every step passed and the previous layout came back.
//
// The same file is the child that the node copies run, so that the core sees the copy:
//   --child stun HOST PORT      one STUN Binding request (three tries), the mapped IPv4 address
//   --child keep URL MS [PIPE]  URL over one keep-alive connection every 250 ms for MS milliseconds;
//                               with PIPE the requests go to that named pipe (the tests)
// Each line of a child is "<unix ms> <ip>", or "<unix ms> -" when there was no reply.
import { execFile, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import dgram from 'node:dgram';
import { once } from 'node:events';
import { copyFileSync, existsSync, mkdirSync, realpathSync, renameSync, statSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

import {
  addressBook, buildLayout, createAllDirectTracker, createLineReader, createRpc, heartbeatText, isIPv4,
  laneOf, lanesKnown, learn, maskText, parseSample, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault, withLane,
} from './scenario-lib.mjs';

const USAGE = 'usage: node scripts/scenario.mjs [--pipe \\\\.\\pipe\\SplitLane.Helper] [--heartbeat FILE] [--keep]';
const PIPE = '\\\\.\\pipe\\SplitLane.Helper';
const ADDRESS_URL = 'https://api.ipify.org'; // the address service of the helper (store.DefaultProbeURL)
const STUN_HOST = 'stun.cloudflare.com'; // answers on the VPN lane up to 1424 bytes (spec, appendix A)
const STUN_PORT = 3478;
const SYSTEM32 = path.join(process.env.SystemRoot || 'C:\\Windows', 'System32');
const CURL = path.join(SYSTEM32, 'curl.exe');
const TASKLIST = path.join(SYSTEM32, 'tasklist.exe');
const POWERSHELL = path.join(SYSTEM32, 'WindowsPowerShell', 'v1.0', 'powershell.exe');
const TAILSCALE = path.join(process.env.ProgramFiles || 'C:\\Program Files', 'Tailscale', 'tailscale.exe');
const SELF = fileURLToPath(import.meta.url);

const ENABLE_WAIT = 120000; // step 1
const DISABLE_WAIT = 30000; // step 8
const SWITCH_LIMIT = 3000; // step 4
const RETURN_LIMIT = 5000; // step 5: the 3 s check of spec 8.3 and one more request
const ALL_DIRECT_WAIT = 10000;
const LEAK_EVERY = 500; // step 7
const CORE_EVERY = 500;
const HEARTBEAT_EVERY = 30000;
const KEEP_MS = '60000'; // the longest life of a keep child; the scenario stops it earlier

const book = addressBook();

// say prints one line; every address in it becomes its lane.
function say(line) {
  process.stdout.write(maskText(line, book) + '\n');
}

const sec = (ms) => (ms / 1000).toFixed(1);

async function sleepUntil(t) {
  const ms = t - Date.now();
  if (ms > 0) await sleep(ms);
}

async function waitUntil(pred, timeoutMs) {
  const end = Date.now() + timeoutMs;
  while (!pred()) {
    if (Date.now() >= end) return false;
    await sleep(50);
  }
  return true;
}

// run starts a program without a window. It gives the exit code (-1 when the program could not
// start or was stopped by the time limit) and the output, and never throws.
function run(file, args, timeoutMs = 15000) {
  return new Promise((resolve) => {
    execFile(file, args, { timeout: timeoutMs, windowsHide: true, maxBuffer: 4 << 20 }, (err, stdout) => {
      resolve({ code: err ? (Number.isInteger(err.code) ? err.code : -1) : 0, stdout: String(stdout ?? '') });
    });
  });
}

// egress asks the address service with a copy of curl.exe, past any proxy (as slctl expect-lanes).
async function egress(exe) {
  const r = await run(exe, ['-4', '-s', '--noproxy', '*', '--max-time', '8', ADDRESS_URL], 12000);
  const ip = r.stdout.trim();
  return r.code === 0 && isIPv4(ip) ? ip : null;
}

// The children get no proxy and no NODE_OPTIONS: they talk to the services themselves.
function childEnv() {
  const env = { ...process.env };
  for (const k of Object.keys(env)) {
    if (/^(node_options|node_use_env_proxy|https?_proxy|all_proxy)$/i.test(k)) delete env[k];
  }
  return env;
}

// startChild runs this file as a child of a node copy; onSample gets each line. ended resolves when
// the child is gone; stop ends it.
function startChild(exe, args, onSample) {
  const child = spawn(exe, [SELF, '--child', ...args], { windowsHide: true, env: childEnv(), stdio: ['ignore', 'pipe', 'ignore'] });
  const reader = createLineReader((line) => onSample(parseSample(line)));
  child.stdout?.on('data', (chunk) => reader.push(chunk));
  const ended = new Promise((resolve) => {
    child.once('close', resolve);
    child.once('error', resolve);
  });
  return {
    ended,
    stop: async () => {
      child.kill();
      await ended;
    },
  };
}

// stunAddress asks the STUN server with a node copy (steps 3 and 5).
async function stunAddress(exe) {
  let ip = null;
  const c = startChild(exe, ['stun', STUN_HOST, String(STUN_PORT)], (s) => {
    ip = s.ip;
  });
  const timer = setTimeout(() => c.stop(), 20000);
  await c.ended;
  clearTimeout(timer);
  return ip;
}

// checkLane asks with one copy until verdictLane decides; the text names the copy and the lane of
// every reply.
async function checkLane(exe, want, ask = egress) {
  const seen = [];
  for (;;) {
    seen.push(laneOf(await ask(exe), book));
    const v = verdictLane(seen, want);
    if (v.done) return { ok: v.ok, text: `${path.basename(exe, '.exe')}=${seen.map((lane) => `<${lane}>`).join(',')}` };
  }
}

// errText names an error reply by its code (and the reasons of rule_rejected) only: the message
// may carry paths of this computer.
function errText(reply) {
  const e = reply.error ?? {};
  const rules = Array.isArray(e.rules) ? ` (${e.rules.map((r) => r.reason).join(', ')})` : '';
  return `${e.code ?? 'no result'}${rules}`;
}

// Helper is the connection to the pipe: replies go to their requests; state events update state and
// go to onState and to the waiters.
class Helper {
  static async connect(name) {
    const socket = net.connect(name);
    await once(socket, 'connect');
    return new Helper(socket);
  }

  constructor(socket) {
    this.socket = socket;
    this.state = null;
    this.onState = () => {};
    this.waiters = new Set();
    this.rpc = createRpc((line) => socket.write(line));
    const reader = createLineReader((line) => this.message(line));
    socket.on('data', (chunk) => {
      try {
        reader.push(chunk);
      } catch (err) {
        socket.destroy(err);
      }
    });
    socket.on('error', () => {});
    socket.on('close', () => this.rpc.fail(new Error('the pipe closed')));
  }

  message(line) {
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      return;
    }
    if (this.rpc.handle(msg) || msg?.event !== 'state') return;
    this.update(msg.data);
  }

  update(state) {
    this.state = state;
    this.onState(state, Date.now());
    for (const w of [...this.waiters]) w(state);
  }

  call(method, params, timeoutMs = 15000) {
    if (this.socket.destroyed) return Promise.reject(new Error('the pipe closed'));
    return this.rpc.call(method, params, timeoutMs);
  }

  // waitState resolves with the first state, the current one included, for which pred is true, or
  // with null after timeoutMs.
  waitState(pred, timeoutMs) {
    if (this.state && pred(this.state)) return Promise.resolve(this.state);
    return new Promise((resolve) => {
      const finish = (s) => {
        clearTimeout(timer);
        this.waiters.delete(check);
        resolve(s);
      };
      const check = (s) => {
        if (pred(s)) finish(s);
      };
      const timer = setTimeout(() => finish(null), timeoutMs);
      this.waiters.add(check);
    });
  }

  close() {
    this.socket.end();
  }
}

// writeAtomic writes a temporary file in the same folder and renames it, again while a reader holds
// the target (as Write-P1Atomic in p1lib.ps1 does, with another temporary name).
async function writeAtomic(file, text) {
  const tmp = `${file}.mjstmp`;
  let last;
  for (let attempt = 1; attempt <= 20; attempt++) {
    try {
      writeFileSync(tmp, text);
      renameSync(tmp, file);
      return;
    } catch (err) {
      last = err;
      await sleep(100);
    }
  }
  throw last;
}

// startHeartbeat keeps the guard of plan 1A armed for the whole run (Set-P1Heartbeat).
function startHeartbeat(file) {
  mkdirSync(path.dirname(file), { recursive: true });
  let writing = Promise.resolve();
  const beat = () => {
    writing = writing
      .then(() => writeAtomic(file, heartbeatText(Date.now())))
      .catch((err) => say(`SCENARIO-HEARTBEAT FAIL ${err.code ?? err.message}`));
  };
  beat();
  const timer = setInterval(beat, HEARTBEAT_EVERY);
  return async () => {
    clearInterval(timer);
    await writing;
  };
}

// corePids lists the processes of sing-box.exe; null when tasklist failed.
async function corePids() {
  const r = await run(TASKLIST, ['/fi', 'imagename eq sing-box.exe', '/fo', 'csv', '/nh']);
  return r.code === 0 ? parseTasklist(r.stdout) : null;
}

// watchCore looks at the core process every 500 ms for step 7; stop takes one more look.
function watchCore() {
  const looks = [];
  let stopped = false;
  const look = async () => {
    const t = Date.now();
    const pids = await corePids();
    looks.push({ t, alive: pids === null ? null : pids.length > 0 });
  };
  const loop = (async () => {
    while (!stopped) {
      await look();
      await sleep(CORE_EVERY);
    }
  })();
  let stopping = null;
  return {
    looks,
    stop: () => (stopping ??= (async () => {
      stopped = true;
      await loop;
      await look();
    })()),
  };
}

// watchLeaks asks with the VPN copy of curl every 500 ms for the whole scenario (step 7).
function watchLeaks(exe) {
  const samples = [];
  let stopped = false;
  const loop = (async () => {
    while (!stopped) {
      const sent = Date.now();
      const ip = await egress(exe);
      samples.push({ sent, done: Date.now(), ip });
      await sleepUntil(sent + LEAK_EVERY);
    }
  })();
  let stopping = null;
  return {
    samples,
    stop: () => (stopping ??= (async () => {
      stopped = true;
      await loop;
    })()),
  };
}

// systemTailscale reads the exit node and the LAN flag of the system client (step 8); the values
// are only compared, never printed. null: the output could not be read.
async function systemTailscale() {
  if (!existsSync(TAILSCALE)) return { installed: false };
  const status = await run(TAILSCALE, ['status', '--json']);
  const prefs = await run(TAILSCALE, ['debug', 'prefs']);
  return systemExitNode(status.stdout, prefs.stdout);
}

// adapterGone: neither the adapter SplitLane nor a route through it (step 8).
async function adapterGone() {
  const r = await run(POWERSHELL, ['-NoProfile', '-NonInteractive', '-Command',
    '$n = @(Get-NetAdapter -Name SplitLane -ErrorAction SilentlyContinue).Count + '
    + '@(Get-NetRoute -InterfaceAlias SplitLane -ErrorAction SilentlyContinue).Count; \'SPLITLANE-OBJECTS \' + $n'], 30000);
  return r.stdout.trim() === 'SPLITLANE-OBJECTS 0';
}

// prepareCopies puts the copies of spec 12 under %TEMP%\splitlane-scenario, by its long path: the
// core matches the real path of a process. A copy of the same size stays from an earlier run.
function prepareCopies() {
  const p = scenarioPaths(path.join(realpathSync.native(os.tmpdir()), 'splitlane-scenario'));
  const copy = (from, to) => {
    mkdirSync(path.dirname(to), { recursive: true });
    if (!existsSync(to) || statSync(to).size !== statSync(from).size) copyFileSync(from, to);
  };
  for (const k of ['curlVpn', 'curlDirect', 'curlFree', 'curlOuter', 'curlNest']) copy(CURL, p[k]);
  for (const k of ['nodeVpn', 'nodeDirect', 'nodeFree']) copy(process.execPath, p[k]);
  return p;
}

function switchText(v) {
  return v.ok ? `first reply from the new lane after ${v.firstMs} ms, none from the old lane after it` : v.why;
}

// Step 1: routing.enable, phase on within 120 s, both lane addresses known and different.
async function stepEnable(helper) {
  const t = Date.now();
  const r = await helper.call('routing.enable');
  if (r.error) return { ok: false, text: `routing.enable: ${errText(r)}` };
  const s = await helper.waitState((st) => st.phase === 'on' || st.phase === 'error', ENABLE_WAIT);
  if (!s) return { ok: false, text: `no phase on within ${ENABLE_WAIT / 1000} s (phase ${helper.state?.phase})` };
  if (s.phase !== 'on') return { ok: false, text: `phase ${s.phase}, reason ${s.reason?.code ?? 'none'}` };
  const ms = Date.now() - t;
  if (!lanesKnown(s.addresses)) return { ok: false, text: `phase on after ${sec(ms)} s, but the lane addresses are missing or equal` };
  return { ok: true, text: `phase on after ${sec(ms)} s; the lane addresses are known and different` };
}

// Step 2: TCP, each pinned copy of curl gets the address of its lane.
async function stepTcp({ p }) {
  const vpn = await checkLane(p.curlVpn, 'vpn');
  const direct = await checkLane(p.curlDirect, 'direct');
  return { ok: vpn.ok && direct.ok, text: `${vpn.text} ${direct.text}` };
}

// Step 3: UDP, each pinned copy of node gets the address of its lane in the STUN reply.
async function stepUdp({ p }) {
  const vpn = await checkLane(p.nodeVpn, 'vpn', stunAddress);
  const direct = await checkLane(p.nodeDirect, 'direct', stunAddress);
  return { ok: vpn.ok && direct.ok, text: `${vpn.text} ${direct.text}` };
}

// Step 4: the VPN copy of node keeps one connection and asks every 250 ms; lanes.set moves it to
// direct and back. Each time the first reply from the new lane comes within 3 s after lanes.set
// and no reply from the old lane follows it; the core process stays the same.
async function stepMove({ helper, p, layout }) {
  const { vpn, direct } = helper.state.addresses;
  const pidsBefore = await corePids();
  const samples = [];
  const keep = startChild(p.nodeVpn, ['keep', ADDRESS_URL, KEEP_MS], (s) => samples.push(s));
  let there;
  let back;
  let tThere;
  let tBack;
  try {
    await waitUntil(() => samples.filter((s) => s.ip === vpn).length >= 2, 15000);
    tThere = Date.now();
    there = await helper.call('lanes.set', withLane(layout, p.nodeVpn, 'direct'));
    await sleepUntil(tThere + SWITCH_LIMIT + 1000);
    tBack = Date.now();
    back = await helper.call('lanes.set', layout);
    await sleepUntil(tBack + SWITCH_LIMIT + 1000);
  } finally {
    await keep.stop();
  }
  const pidsAfter = await corePids();
  const toDirect = verdictSwitch(samples.filter((s) => s.t <= tBack), tThere, vpn, direct, SWITCH_LIMIT);
  const toVpn = verdictSwitch(samples, tBack, direct, vpn, SWITCH_LIMIT);
  const samePid = pidsBefore?.length === 1 && pidsAfter?.length === 1 && pidsBefore[0] === pidsAfter[0];
  return {
    ok: !there.error && !back.error && toDirect.ok && toVpn.ok && samePid,
    text: [
      `node-vpn to direct: ${there.error ? `lanes.set ${errText(there)}` : switchText(toDirect)}`,
      `back to vpn: ${back.error ? `lanes.set ${errText(back)}` : switchText(toVpn)}`,
      `core process ${samePid ? 'the same' : 'changed or not found'}`,
    ].join('; '),
  };
}

// Step 5: the main lane and "all direct". The free copies follow the main lane while the VPN copy
// of curl stays on VPN; in "all direct" the free copy goes direct and the VPN copy of node opens a
// connection; after the return that connection is closed (its replies come from VPN within 5 s and
// never from direct again) and the VPN copy of curl is on VPN.
async function stepMain({ helper, p, layout, tracker }) {
  const { vpn, direct } = helper.state.addresses;
  const parts = [];
  let ok = true;
  const fail = (text) => {
    ok = false;
    parts.push(text);
  };
  const check = async (label, checks) => {
    const texts = [];
    for (const [exe, want, ask] of checks) {
      const r = await checkLane(exe, want, ask);
      ok &&= r.ok;
      texts.push(r.text);
    }
    parts.push(`${label}: ${texts.join(' ')}`);
  };
  const lanes = async (label, params) => {
    const r = await helper.call('lanes.set', params);
    if (r.error) fail(`${label}: lanes.set ${errText(r)}`);
    return !r.error;
  };

  await check('main vpn', [[p.curlFree, 'vpn']]);
  if (await lanes('main direct', withDefault(layout, 'direct'))) {
    await check('main direct', [[p.curlFree, 'direct'], [p.nodeFree, 'direct', stunAddress], [p.curlVpn, 'vpn']]);
  }
  if (await lanes('main vpn again', layout)) {
    await check('main vpn again', [[p.curlFree, 'vpn'], [p.nodeFree, 'vpn', stunAddress]]);
  }

  tracker.requested(true, Date.now());
  const on = await helper.call('routing.allDirect', { on: true });
  if (on.error) {
    tracker.requested(false, Date.now());
    fail(`all direct: routing.allDirect ${errText(on)}`);
    return { ok, text: parts.join('; ') };
  }
  const isOn = await helper.waitState((s) => s.allDirect === true, ALL_DIRECT_WAIT);
  if (isOn) await check('all direct', [[p.curlFree, 'direct']]);
  else fail(`all direct: not on within ${ALL_DIRECT_WAIT / 1000} s`);
  const samples = [];
  const keep = isOn ? startChild(p.nodeVpn, ['keep', ADDRESS_URL, KEEP_MS], (s) => samples.push(s)) : null;
  let tOff;
  let off;
  let returned = null;
  try {
    if (keep) await waitUntil(() => samples.filter((s) => s.ip === direct).length >= 2, 15000);
    tOff = Date.now();
    tracker.requested(false, tOff);
    off = await helper.call('routing.allDirect', { on: false });
    if (!off.error) returned = await helper.waitState((s) => s.allDirect === false, ALL_DIRECT_WAIT);
    if (keep) await sleepUntil(tOff + RETURN_LIMIT + 1000);
  } finally {
    await keep?.stop();
  }
  if (off.error) fail(`back: routing.allDirect ${errText(off)}`);
  else if (!returned) fail(`back: all direct still on after ${ALL_DIRECT_WAIT / 1000} s`);
  if (keep) {
    const v = verdictSwitch(samples, tOff, direct, vpn, RETURN_LIMIT);
    ok &&= v.ok;
    parts.push(`after all direct: node-vpn ${switchText(v)}`);
  }
  await check('after all direct', [[p.curlVpn, 'vpn']]);
  return { ok, text: parts.join('; ') };
}

// Step 6: nested rules. The folder nest is pinned direct and nest\inner\curl-nest.exe vpn: the inner
// copy goes by VPN, the copy elsewhere in the folder directly.
async function stepNested({ p }) {
  const inner = await checkLane(p.curlNest, 'vpn');
  const outer = await checkLane(p.curlOuter, 'direct');
  return { ok: inner.ok && outer.ok, text: `${inner.text} ${outer.text}` };
}

// Step 8: routing.disable, phase off within 30 s; the system exit node and its LAN flag as before
// step 1, no core process, neither the adapter SplitLane nor a route through it.
async function stepDisable(helper, before) {
  const t = Date.now();
  const r = await helper.call('routing.disable', {});
  if (r.error) return { ok: false, text: `routing.disable: ${errText(r)}` };
  const off = await helper.waitState((s) => s.phase === 'off', DISABLE_WAIT);
  const ms = Date.now() - t;
  const system = sameSystem(before, await systemTailscale());
  const pids = await corePids();
  const gone = await adapterGone();
  let core = 'no sing-box.exe';
  if (pids === null) core = 'tasklist failed';
  else if (pids.length > 0) core = 'sing-box.exe still runs';
  return {
    ok: Boolean(off) && system && pids?.length === 0 && gone,
    text: [
      off ? `phase off after ${sec(ms)} s`
        : `no phase off within ${DISABLE_WAIT / 1000} s (phase ${helper.state?.phase}, reason ${helper.state?.reason?.code ?? 'none'})`,
      `system exit node and LAN flag ${system ? 'equal' : 'different or unreadable'}`,
      core,
      gone ? 'no SplitLane adapter or route' : 'SplitLane adapter or route still there',
    ].join('; '),
  };
}

async function disableQuietly(helper) {
  try {
    await helper.call('routing.disable', {});
    await helper.waitState((s) => s.phase === 'off', DISABLE_WAIT);
  } catch {
    // The pipe is gone: the guard of plan 1A and slctl are the way back.
  }
}

// restoreLayout gives the helper the layout it had before the scenario, unless --keep.
async function restoreLayout(helper, previous, keep) {
  if (keep) {
    say('SCENARIO-RESTORE PASS the layout of the scenario stays (--keep)');
    return true;
  }
  try {
    const r = await helper.call('lanes.set', previous);
    say(`SCENARIO-RESTORE ${r.error ? `FAIL ${errText(r)}` : 'PASS'} previous layout of ${previous.apps.length} rules`);
    return !r.error;
  } catch (err) {
    say(`SCENARIO-RESTORE FAIL ${err.message}`);
    return false;
  }
}

// withHelper is the scenario on a connected pipe; it gives whether the previous layout came back.
async function withHelper(helper, opts, report) {
  const hello = await helper.call('hello', { protocol: 1 });
  if (hello.result?.protocol !== 1) {
    report(0, false, `hello: ${errText(hello)}`);
    return true;
  }
  const first = await helper.call('state.get');
  if (!first.result) {
    report(0, false, `state.get: ${errText(first)}`);
    return true;
  }
  helper.update(first.result);
  if (first.result.phase !== 'off') {
    report(0, false, `the helper is in phase ${first.result.phase}; the scenario starts from phase off`);
    return true;
  }
  const previous = { apps: first.result.apps ?? [], defaultLane: first.result.defaultLane === 'direct' ? 'direct' : 'vpn' };
  const before = await systemTailscale();
  const p = prepareCopies();
  const layout = buildLayout(p);
  const set = await helper.call('lanes.set', layout);
  if (set.error) {
    report(0, false, `lanes.set of the scenario layout: ${errText(set)}`);
    return true;
  }
  say(`SCENARIO-PREP ${layout.apps.length} rules for the copies in %TEMP%\\splitlane-scenario; `
    + `the previous layout has ${previous.apps.length} rules`);

  const tracker = createAllDirectTracker();
  helper.onState = (s, t) => {
    learn(book, s.addresses);
    tracker.observed(s.allDirect === true, t);
  };
  const ctx = { helper, p, layout, tracker };
  const core = watchCore();
  const leaks = watchLeaks(p.curlVpn);
  let routing = true; // from routing.enable until the disable of step 8
  let restored = false;
  try {
    const one = await stepEnable(helper);
    report(1, one.ok, one.text);
    for (const [n, step] of [[2, stepTcp], [3, stepUdp], [4, stepMove], [5, stepMain], [6, stepNested]]) {
      if (!one.ok) {
        report(n, false, 'not run: routing is not on');
        continue;
      }
      const r = await step(ctx);
      report(n, r.ok, r.text);
    }
    const eight = await stepDisable(helper, before);
    routing = false;
    await leaks.stop();
    await core.stop();
    const v = verdictLeak(leaks.samples, core.looks, tracker.windows(), book);
    report(7, v.ok, `${v.counted} replies of curl-vpn counted, ${v.ignored} not counted (no core or all direct), `
      + `${v.leaks} of them from <direct>`);
    report(8, eight.ok, eight.text);
  } finally {
    if (routing) await disableQuietly(helper);
    await leaks.stop();
    await core.stop();
    restored = await restoreLayout(helper, previous, opts.keep);
  }
  return restored;
}

async function scenario(opts) {
  const stopHeartbeat = opts.heartbeat ? startHeartbeat(opts.heartbeat) : async () => {};
  const passed = [];
  const failed = [];
  const report = (n, ok, text) => {
    (ok ? passed : failed).push(n);
    say(`SCENARIO ${n} ${ok ? 'PASS' : 'FAIL'} ${text}`);
  };
  let restored = true;
  let helper = null;
  try {
    try {
      helper = await Helper.connect(opts.pipe);
    } catch (err) {
      report(0, false, `no pipe ${opts.pipe}: ${err.code ?? err.message}`);
    }
    if (helper) restored = await withHelper(helper, opts, report);
  } catch (err) {
    failed.push('abort');
    say(`SCENARIO-ABORT ${err.message}`);
  } finally {
    helper?.close();
    await stopHeartbeat();
  }
  say(`SCENARIO-DONE pass=${passed.length} fail=${failed.length}`);
  return failed.length === 0 && restored ? 0 : 1;
}

// childStun sends one Binding request to host:port, tried three times, 2 s each (steps 3 and 5).
async function childStun(host, port) {
  const socket = dgram.createSocket('udp4');
  socket.on('error', () => {});
  try {
    for (let attempt = 0; attempt < 3; attempt++) {
      const tx = randomBytes(12);
      const got = await new Promise((resolve) => {
        const finish = (v) => {
          clearTimeout(timer);
          socket.off('message', onMessage);
          resolve(v);
        };
        const onMessage = (msg) => {
          const a = parseStunResponse(msg, tx);
          if (a) finish(a);
        };
        const timer = setTimeout(() => finish(null), 2000);
        socket.on('message', onMessage);
        socket.send(stunRequest(tx), port, host, (err) => {
          if (err) finish(null);
        });
      });
      if (got) {
        process.stdout.write(`${Date.now()} ${got.ip}\n`);
        return;
      }
    }
    process.stdout.write(`${Date.now()} -\n`);
  } finally {
    socket.close();
  }
}

// ask is one request of the keep child: {ip, conn}, the address in the body (or null) and the
// number that numberOf gave its connection (0: it got none).
function ask(mod, target, agent, pipe, numberOf) {
  return new Promise((resolve) => {
    let conn = 0;
    const done = (ip) => resolve({ ip, conn });
    const req = mod.get(target, pipe ? { agent, socketPath: pipe } : { agent }, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (d) => {
        body += d;
      });
      res.on('end', () => done(isIPv4(body.trim()) ? body.trim() : null));
      res.on('error', () => done(null));
      res.on('close', () => done(null)); // after end this changes nothing
    });
    req.on('socket', (socket) => {
      conn = numberOf(socket);
    });
    req.setTimeout(5000, () => req.destroy(new Error('timeout')));
    req.on('error', () => done(null));
  });
}

// childKeep asks url over one keep-alive connection every 250 ms for ms milliseconds (steps 4, 5).
// A request on a connection that the helper closes fails ("-"); the next one opens a new connection.
// Each line carries the number of the connection, so that the parent sees the connection kept.
async function childKeep(url, ms, pipe) {
  const target = new URL(url);
  const mod = target.protocol === 'http:' ? http : https;
  const agent = new mod.Agent({ keepAlive: true, maxSockets: 1 });
  const numbers = new WeakMap();
  let opened = 0;
  const numberOf = (socket) => {
    if (!numbers.has(socket)) numbers.set(socket, ++opened);
    return numbers.get(socket);
  };
  const end = Date.now() + ms;
  while (Date.now() < end) {
    const started = Date.now();
    const { ip, conn } = await ask(mod, target, agent, pipe, numberOf);
    process.stdout.write(`${Date.now()} ${ip ?? '-'} ${conn}\n`);
    await sleepUntil(started + 250);
  }
  agent.destroy();
}

let args;
try {
  args = parseArgs({
    options: {
      pipe: { type: 'string', default: PIPE },
      heartbeat: { type: 'string' },
      keep: { type: 'boolean', default: false },
      child: { type: 'string' },
    },
    allowPositionals: true,
  });
} catch (err) {
  process.stderr.write(`${err.message}\n${USAGE}\n`);
  process.exit(2);
}
const { values, positionals } = args;
let code = 0;
if (values.child === 'stun') await childStun(positionals[0], Number(positionals[1]));
else if (values.child === 'keep') await childKeep(positionals[0], Number(positionals[1]), positionals[2]);
else if (values.child || positionals.length > 0) {
  process.stderr.write(`${USAGE}\n`);
  code = 2;
} else code = await scenario(values);
process.stdout.write('', () => process.exit(code));
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router; node --test --test-reporter=spec scripts\scenario.test.mjs
```

Ожидается одиннадцать строк `✔`: девять из шага 4, `✔ scenario refuses a helper that is not off` и `✔ keep child reuses one connection and reconnects`; затем `ℹ pass 11` и `ℹ fail 0`.

- [ ] **Step 9: Полная проверка**

Код Go задача не меняет. Проверяются синтаксис обоих модулей, только ASCII в трёх файлах и пять прогонов тестов подряд:

```powershell
cd C:\code\router; node --check scripts\scenario.mjs; node --check scripts\scenario-lib.mjs; $bad = Select-String -Path scripts\scenario.mjs, scripts\scenario-lib.mjs, scripts\scenario.test.mjs -Pattern '[^\x00-\x7F]'; if ($bad) { $bad } else { 'ASCII-OK' }; 1..5 | ForEach-Object { $out = node --test --test-reporter=spec scripts\scenario.test.mjs 2>&1 | Out-String; $code = $LASTEXITCODE; $pass = ([regex]::Match($out, 'pass (\d+)')).Groups[1].Value; $fail = ([regex]::Match($out, 'fail (\d+)')).Groups[1].Value; "run $_ exit=$code pass=$pass fail=$fail" }
```

Ожидается: `node --check` ничего не печатает, затем `ASCII-OK` и пять строк `run <N> exit=0 pass=11 fail=0`.

- [ ] **Step 10: Закоммитить**

```powershell
git -C C:\code\router add scripts/scenario.mjs scripts/scenario-lib.mjs scripts/scenario.test.mjs
git -C C:\code\router commit -m "feat(scripts): scenario check of spec 12 as a second pipe client" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 6: Операции автомата: «всё напрямую», раскладка и выключение во время перезапуска

Две отложенные находки финальной проверки плана 1A (отчёт `docs/superpowers/reports/2026-11-14-splitlane-plan-1a-rulings.md`, раздел «Припаркованные находки»). (1, помечена MUST) `routing.allDirect` и `lanes.set` при работающем ядре не регистрируются как операции (`e.op`), поэтому поток надзора (`watch` в `enable.go`) их не ждёт. Если ядро падает во время трёхсекундной сверки 8.3, перезапуск идёт рядом с применением. Переключение «всё напрямую» после этого ставит `allDirect` поверх нового ядра, запущенного в обычном режиме. `lanes.set` сохраняет раскладку, которую перезапущенное ядро не читало: оно взяло прежнюю из `state.json`, а новые файлы правил затёрло прежними. Тот же класс ошибки для `systemTailscale.clearExitNode` исправлен в коммите `9f783ee`, и задача делает то же самое. Обе команды выполняются как операции (виды `alldirect` и `lanes`). Поток надзора, как и раньше, сразу возвращает системный exit node (5.5), затем ждёт конца операции и только после этого перезапускает ядро. Перезапуск сбрасывает «всё напрямую» (5.2). Теперь сброс стоит в одном месте, в `beginRestartLocked`: он выполняется до первого состояния фазы `starting`, и это же место использует проверка связи 5.5. Ядро перезапускается с той раскладкой, которая к этому моменту записана в `state.json`. Очередь 5.1 не меняется: `routing.allDirect` отвечает сразу и держит очередь до конца операции, `lanes.set` отвечает по её завершении. Пока идёт другая операция, обе команды отвечают `busy`, как `tailscale.setExitNode`.

(2) `routing.disable` без `skipRestore` в фазе `stopping` во время выключающей половины одной из двух операций — старта 5.7 с признаком «было включено» или переподключения по `tailscale.setExitNode` — по букве таблицы 5.1 ничего не делал. Включение, которое следует за этой половиной, снова включало деление, и явное выключение пользователя терялось. Теперь такой `routing.disable` снимает признак «было включено» и сохраняет `state.json` (5.2: признак снимается в начале выключения). Обе операции читают признак прямо перед включением: старт уже был так устроен, переподключение получает ту же проверку. Итоговая фаза — `off`, признак снят, новый exit node остаётся выбранным. Отмена действует только без причины `restore_pending`. С этой причиной обычный `routing.disable` — действие «Повторить» окна (9.5): одна внеочередная попытка возврата, и больше ничего; после подтверждения возврата включение идёт, как шло бы без команды. С `skipRestore: true` поведение прежнее, во всех других случаях фазы `stopping` тоже (без действия). Сначала задача отменяет включение при любой причине (шаги 5–8), затем отдельный тест `TestRetryInRestorePendingKeepsTheReenable` (шаги 9–12) оставляет «Повторить» чистой попыткой. Правило задача записывает в 5.1, в строку `routing.disable`. Ячейки `TestPhaseTable` не меняются: столбец `stopping` там проверяет обычное выключение, за которым включения нет.

**Files:**
- Modify: `helper/internal/engine/commands.go`, `helper/internal/engine/enable.go`, `helper/internal/engine/supervise.go`, `helper/internal/engine/engine.go`, `helper/internal/engine/startup.go`
- Test: `helper/internal/engine/fakes_test.go`, `helper/internal/engine/supervise_test.go`, `helper/internal/engine/commands_test.go`, `helper/internal/engine/startup_test.go`
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (5.1, строка `routing.disable`)

**Interfaces:**
- Consumes: `func (e *Engine) startOp(kind string, fn func(ctx context.Context)) bool`, `func (e *Engine) watch(r *coreRun)`, `func (e *Engine) runRestart(ctx context.Context, why string)`, `func (e *Engine) applyAllDirect(ctx context.Context, on bool)`, `func (e *Engine) applyLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string) (func(), *protocol.Error)`, `func (e *Engine) teardown() bool`, `func (e *Engine) finish(phase string, why *protocol.Reason)`, `func (e *Engine) save() error`; в тестах — `slowSys`, `within`, `publishedCount`, `inOrder`, `countSince`, `leftover`, `enabled`, `call`, `fileRule`.
- Produces: ничего экспортируемого, экспортируемые имена не меняются. Внутри пакета: виды операций `"alldirect"` и `"lanes"`; `func (e *Engine) beginRestartLocked(why string)`; `func (e *Engine) reenablesLocked() bool`; `func (e *Engine) setLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string, apply bool) *protocol.Error`; `func (e *Engine) cmdLanesSet(params json.RawMessage) *protocol.Error` (параметра `ctx` больше нет: применение работает с контекстом своей операции). В тестах: поле `onConns func()` у `fakeClash`, `func gateConns(t *testing.T, h *harness) (entered chan struct{}, release func())`, `func crashInsideTheApply(t *testing.T, h *harness, entered chan struct{}) int`, `func holdStatus(t *testing.T, h *harness) (release func())`.

- [ ] **Step 1: Написать тесты сбоя ядра во время применения**

Подменный Clash API получает крючок `onConns`: он вызывается при каждом запросе списка соединений, до ответа и вне блокировки. Через него тест задерживает сверку 8.3 на середине.

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
	connsErr  error
	reconnect func(c Conn) *Conn // a closed connection comes back like this (nil: it does not)
}
```

Заменить на:

```go
	connsErr  error
	reconnect func(c Conn) *Conn // a closed connection comes back like this (nil: it does not)
	onConns   func()             // runs at each call of Connections, before the answer, outside the lock
}
```

Найти:

```go
func (k *fakeClash) Connections(ctx context.Context) ([]Conn, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
```

Заменить на:

```go
func (k *fakeClash) Connections(ctx context.Context) ([]Conn, error) {
	k.mu.Lock()
	hook := k.onConns
	k.mu.Unlock()
	if hook != nil {
		hook()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
```

Тесты идут в конец `supervise_test.go`, после `TestPollWhileOffNeedsAWindow`.

Изменить `helper/internal/engine/supervise_test.go`:

Найти:

```go
	if !s.Installed || !s.ExitNodeOn || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("system %+v", s)
	}
}
```

Заменить на:

```go
	if !s.Installed || !s.ExitNodeOn || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("system %+v", s)
	}
}

// gateConns makes the next call of Connections (the reconciliation of 8.3) wait until release;
// entered is closed when that call came. Every later call goes through at once.
func gateConns(t *testing.T, h *harness) (entered chan struct{}, release func()) {
	t.Helper()
	entered, gate := make(chan struct{}), make(chan struct{})
	var hit, open sync.Once
	release = func() { open.Do(func() { close(gate) }) }
	t.Cleanup(release)
	h.clash.mu.Lock()
	defer h.clash.mu.Unlock()
	h.clash.onConns = func() {
		first := false
		hit.Do(func() { first = true })
		if first {
			close(entered)
			<-gate
		}
	}
	return entered, release
}

// crashInsideTheApply waits until the apply is held in its reconciliation, ends the core there and
// waits for the watcher's immediate restore (5.5). It fails the test if the core is restarted while
// the apply is still held. It returns the number of events before the crash.
func crashInsideTheApply(t *testing.T, h *harness, entered chan struct{}) int {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("the reconciliation never came: %v", h.rec.list())
	}
	mark := len(h.rec.list())
	h.core.last().crash()
	h.waitFor("the immediate restore", func() bool { return countSince(h, mark, "sys:set "+exitNode) == 1 })
	if within(300*time.Millisecond, func() bool { return h.rec.count("core:start") == 2 }) {
		t.Fatalf("the core was restarted while the apply still ran: %v", h.rec.list()[mark:])
	}
	return mark
}

// Final review of plan 1A, the finding "routing.allDirect and lanes.set are not operations": the core
// fails in the middle of the reconciliation of an all-direct switch. Before, the watcher restarted the
// core beside the switch, which then set allDirect over the new core running in the normal mode. The
// switch is an operation now: the watcher gives the exit node back at once, waits for the switch to
// end, then restarts; the restart resets "all direct" (5.2), the published starting state too.
func TestCrashDuringAllDirectWaitsForTheOperation(t *testing.T) {
	h := enabled(t)
	entered, release := gateConns(t, h)
	if err := call(h, protocol.MethodAllDirect, map[string]bool{"on": true}); err != nil {
		t.Fatal(err)
	}
	crashInsideTheApply(t, h, entered)
	states := publishedCount(h)
	release()
	h.waitFor("restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if s := h.e.State(); s.AllDirect || h.core.options().Mode != "vpn" {
		t.Fatalf("the restarted core runs in the normal mode: allDirect %v, mode %s", s.AllDirect, h.core.options().Mode)
	}
	h.mu.Lock()
	after := append([]protocol.State{}, h.states[states:]...)
	h.mu.Unlock()
	for _, s := range after {
		if s.Phase == protocol.PhaseStarting && s.AllDirect {
			t.Fatalf("the restart published all direct: %+v", s)
		}
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// The same finding for lanes.set: the core fails in the middle of the reconciliation of a new layout.
// Before, the watcher restarted at once and the new core read the old layout from state.json, while
// lanes.set went on to save the new one: state.json and the running core disagreed. Now lanes.set is
// an operation the watcher waits for: it answers, then the restarted core reads the stored layout.
func TestCrashDuringLanesSetWaitsForTheOperation(t *testing.T) {
	h := enabled(t)
	entered, release := gateConns(t, h)
	answer := make(chan *protocol.Error, 1)
	go func() {
		answer <- call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: protocol.LaneVPN,
			Apps: []protocol.AppRule{fileRule(idA, curlA, protocol.LaneDirect)}})
	}()
	mark := crashInsideTheApply(t, h, entered)
	release()
	select {
	case err := <-answer:
		if err != nil {
			t.Fatalf("lanes.set: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("lanes.set never answered: %v", h.rec.list())
	}
	h.waitFor("restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if got := h.store.saved().Apps; len(got) != 1 || got[0].ID != idA {
		t.Fatalf("the layout must be saved: %+v", got)
	}
	if !strings.Contains(string(h.store.direct), `curl-a\\.exe$`) || !inOrder(h, mark, "core:crash", "rules:write", "core:start") {
		t.Fatalf("the restarted core must read the stored layout:\n%s\nevents %v", h.store.direct, h.rec.list()[mark:])
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestCrashDuring(AllDirect|LanesSet)WaitsForTheOperation' -count=1
```

Ожидается: два провала. Перезапуск начинается сразу, пока применение задержано на сверке (список событий после `core:start 1001` зависит от момента печати):

```text
--- FAIL: TestCrashDuringAllDirectWaitsForTheOperation (0.00s)
    supervise_test.go:556: the core was restarted while the apply still ran: [core:crash 1000 sys:set 100.64.0.1 true journal:delete rules:write core:check host:removestale core:start 1001 ...]
--- FAIL: TestCrashDuringLanesSetWaitsForTheOperation (0.00s)
    supervise_test.go:590: the core was restarted while the apply still ran: [core:crash 1000 sys:set 100.64.0.1 true journal:delete rules:write core:check host:removestale core:start 1001 ...]
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 3: Выполнять «всё напрямую» и раскладку как операции**

`routing.allDirect` запускает операцию `alldirect` в той же блокировке, в которой проверена фаза, и, как `systemTailscale.clearExitNode`, держит очередь до закрытия канала `done` операции. `lanes.set` после проверки раскладки снова читает фазу: пока правила проверялись, ядро могло упасть. При работающем ядре применение и сохранение выполняются одной операцией `lanes`, и ответ ждёт её конца. В `off` и `error` раскладка только сохраняется, как раньше. Сохранение (шаг 3 из 8.3) переносится без изменений в `setLanes`.

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
	case protocol.MethodLanesSet:
		err = e.cmdLanesSet(ctx, params)
```

Заменить на:

```go
	case protocol.MethodLanesSet:
		err = e.cmdLanesSet(params)
```

Найти:

```go
// cmdAllDirect: applied in on and degraded; the answer comes at once, the result in state events.
func (e *Engine) cmdAllDirect(params json.RawMessage) (bool, *protocol.Error) {
	var p protocol.AllDirectParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return false, err
	}
	if p.On == nil {
		return false, protocol.Errorf(protocol.CodeBadRequest, "params.on is required")
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	switch {
	case exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return false, busy()
	case phase != protocol.PhaseOn && phase != protocol.PhaseDegraded:
		return false, wrongPhase(phase)
	}
	go func() {
		defer e.queue.Unlock()
		e.applyAllDirect(context.Background(), *p.On)
	}()
	return true, nil
}

// cmdLanesSet is 8.3: check the whole layout, apply it while the core runs, save it.
func (e *Engine) cmdLanesSet(ctx context.Context, params json.RawMessage) *protocol.Error {
	var p protocol.LanesSetParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	if exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping {
		return busy()
	}
	if err := protocol.ValidateLanes(p); err != nil {
		return err
	}
	apps, rejected, err := rules.Check(p.Apps, e.d.Rules)
	if err != nil {
		return protocol.Errorf(protocol.CodeBadRequest, "%v", err)
	}
	if len(rejected) > 0 {
		return &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected", Rules: rejected}
	}
	var undo func()
	if live(phase) {
		var aerr *protocol.Error
		if undo, aerr = e.applyLanes(ctx, apps, p.DefaultLane); aerr != nil {
			return aerr
		}
	}
	// Step 3: the answer comes only after the layout is in state.json. A write that fails is
	// apply_failed: the previous layout stays, and the core gets its previous files and mode back.
	e.mu.Lock()
	prevApps, prevLane := e.saved.Apps, e.saved.DefaultLane
	e.saved.Apps, e.saved.DefaultLane = apps, p.DefaultLane
	saveErr := e.save()
	switch {
	case saveErr == nil, errors.Is(saveErr, errStateLate):
		e.publish() // the new layout, or the one state.json holds, read only now
	default:
		e.saved.Apps, e.saved.DefaultLane = prevApps, prevLane
	}
	e.mu.Unlock()
	if saveErr != nil {
		if undo != nil {
			undo()
		}
		return protocol.Errorf(protocol.CodeApplyFailed, "layout not saved: %v", saveErr)
	}
	return nil
}
```

Заменить на:

```go
// cmdAllDirect: applied in on and degraded; the answer comes at once, the result in state events.
// The switch with its reconciliation (8.3) is an operation, like systemTailscale.clearExitNode: a
// crash of the core during it is handled by the watcher after it, never beside it (the restart would
// otherwise meet a switch that still sets the mode and the flag of the core that is gone). Another
// operation still in place is busy. The queue stays taken until the operation is gone (5.1: the next
// command waits for the reconciliation).
func (e *Engine) cmdAllDirect(params json.RawMessage) (bool, *protocol.Error) {
	var p protocol.AllDirectParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return false, err
	}
	if p.On == nil {
		return false, protocol.Errorf(protocol.CodeBadRequest, "params.on is required")
	}
	on := *p.On
	e.mu.Lock()
	defer e.mu.Unlock()
	phase := e.st.Phase
	switch {
	case e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return false, busy()
	case phase != protocol.PhaseOn && phase != protocol.PhaseDegraded:
		return false, wrongPhase(phase)
	}
	if !e.startOp("alldirect", func(ctx context.Context) { e.applyAllDirect(ctx, on) }) {
		return false, busy()
	}
	o := e.op
	go func() {
		<-o.done
		e.queue.Unlock()
	}()
	return true, nil
}

// cmdLanesSet is 8.3: check the whole layout, apply it while the core runs, save it. While the core
// runs (on, degraded, blocked) the apply and the save are one operation, which the answer waits for:
// the watcher waits for it too, so the core restarted after a crash during the reconciliation reads
// the layout that state.json holds once the apply is over. Another operation still in place is busy.
func (e *Engine) cmdLanesSet(params json.RawMessage) *protocol.Error {
	var p protocol.LanesSetParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	if exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping {
		return busy()
	}
	if err := protocol.ValidateLanes(p); err != nil {
		return err
	}
	apps, rejected, err := rules.Check(p.Apps, e.d.Rules)
	if err != nil {
		return protocol.Errorf(protocol.CodeBadRequest, "%v", err)
	}
	if len(rejected) > 0 {
		return &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected", Rules: rejected}
	}
	// The phase is read again: the core may have failed while the layout was checked.
	e.mu.Lock()
	phase = e.st.Phase
	switch {
	case e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		e.mu.Unlock()
		return busy()
	case !live(phase):
		e.mu.Unlock()
		return e.setLanes(context.Background(), apps, p.DefaultLane, false)
	}
	var aerr *protocol.Error
	started := e.startOp("lanes", func(ctx context.Context) { aerr = e.setLanes(ctx, apps, p.DefaultLane, true) })
	o := e.op
	e.mu.Unlock()
	if !started {
		return busy()
	}
	<-o.done
	return aerr
}

// setLanes is steps 2 and 3 of 8.3 for a checked layout: with apply (the core runs) the files and the
// mode first, with the reconciliation; then the layout goes into state.json. The answer comes only
// after the layout is in state.json. A write that fails is apply_failed: the previous layout stays,
// and the core gets its previous files and mode back.
func (e *Engine) setLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string, apply bool) *protocol.Error {
	var undo func()
	if apply {
		var aerr *protocol.Error
		if undo, aerr = e.applyLanes(ctx, apps, defaultLane); aerr != nil {
			return aerr
		}
	}
	e.mu.Lock()
	prevApps, prevLane := e.saved.Apps, e.saved.DefaultLane
	e.saved.Apps, e.saved.DefaultLane = apps, defaultLane
	saveErr := e.save()
	switch {
	case saveErr == nil, errors.Is(saveErr, errStateLate):
		e.publish() // the new layout, or the one state.json holds, read only now
	default:
		e.saved.Apps, e.saved.DefaultLane = prevApps, prevLane
	}
	e.mu.Unlock()
	if saveErr != nil {
		if undo != nil {
			undo()
		}
		return protocol.Errorf(protocol.CodeApplyFailed, "layout not saved: %v", saveErr)
	}
	return nil
}
```

Поток надзора уже ждёт любую операцию. Меняются его описание и начало перезапуска: `beginRestartLocked` сбрасывает «всё напрямую» до того, как фаза `starting` уйдёт окну.

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
// systemTailscale.clearExitNode is such an operation: when it ends, blocked may be over and the
// user's exit node off for this run, in the clear's journal. So when the watcher restored nothing
// at first, it reads blocked again after the wait and gives the exit node back now, before the
// restart. After its own restore it does not read blocked again: the exit node is on because of it.
func (e *Engine) watch(r *coreRun) {
```

Заменить на:

```go
// systemTailscale.clearExitNode is such an operation: when it ends, blocked may be over and the
// user's exit node off for this run, in the clear's journal. So when the watcher restored nothing
// at first, it reads blocked again after the wait and gives the exit node back now, before the
// restart. After its own restore it does not read blocked again: the exit node is on because of it.
// routing.allDirect and lanes.set while the core runs are such operations too: the restart that
// follows them resets "all direct" (5.2) and starts the core with the layout state.json holds then.
func (e *Engine) watch(r *coreRun) {
```

Найти:

```go
		e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonCoreCrashed), false) })
		return
	}
	e.setPhase(protocol.PhaseStarting, nil)
	e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreCrashed) })
}
```

Заменить на:

```go
		e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonCoreCrashed), false) })
		return
	}
	e.beginRestartLocked(protocol.ReasonCoreCrashed)
}

// beginRestartLocked starts the restart of 5.5 in phase starting; e.mu must be held. "All direct"
// ends here (5.2): the next core starts in the normal mode, and no state of the restart shows the
// flag, also not one that an all-direct switch which ended just before set for the core that is gone.
func (e *Engine) beginRestartLocked(why string) {
	e.st.AllDirect = false
	e.setPhase(protocol.PhaseStarting, nil)
	e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, why) })
}
```

Перезапуск по проверке связи (5.5) начинается так же.

Изменить `helper/internal/engine/supervise.go`:

Найти:

```go
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.setPhase(protocol.PhaseStarting, nil)
			e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreUnresponsive) })
			e.mu.Unlock()
```

Заменить на:

```go
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.beginRestartLocked(protocol.ReasonCoreUnresponsive)
			e.mu.Unlock()
```

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
// op is the one background operation: enabling, disabling, a restart, the start-up or
// systemTailscale.clearExitNode.
type op struct {
```

Заменить на:

```go
// op is the one background operation: enabling, disabling, a restart, the start-up,
// systemTailscale.clearExitNode, or routing.allDirect and lanes.set while the core runs.
type op struct {
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestCrashDuring(AllDirect|LanesSet)WaitsForTheOperation' -count=1 -v; go test ./internal/engine/ -count=1
```

Ожидается: `--- PASS: TestCrashDuringAllDirectWaitsForTheOperation (0.32s)` и `--- PASS: TestCrashDuringLanesSetWaitsForTheOperation (0.32s)` (0,3 секунды — проверка, что перезапуск не начался, пока применение задержано), затем `ok  	splitlane/helper/internal/engine` для пакета целиком: `TestPhaseTable`, `TestAllDirect`, `TestCommandsRunOneAtATime` и тесты `lanes.set` проходят без изменений.

- [ ] **Step 5: Написать тесты выключения во время перезапуска**

`holdStatus` задерживает каждый вызов `tailscale status` до `release`, как `inPhase` для фазы `stopping`: операция стоит на шаге 1 из 5.4, в фазе `stopping` без `restore_pending`. Тест переподключения идёт в `commands_test.go` после `TestSetExitNode`.

Изменить `helper/internal/engine/commands_test.go`:

Найти:

```go
	if h.rec.index("sys:set 100.64.0.1 true") < 0 {
		t.Fatal("the system exit node is restored between the two runs")
	}
}
```

Заменить на:

```go
	if h.rec.index("sys:set 100.64.0.1 true") < 0 {
		t.Fatal("the system exit node is restored between the two runs")
	}
}

// holdStatus makes every `tailscale status` wait until release, like a tailscale.exe that does not
// answer yet: an operation in step 1 of 5.4 stays there, in phase stopping without restore_pending.
func holdStatus(t *testing.T, h *harness) (release func()) {
	t.Helper()
	hold := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(hold) }) }
	t.Cleanup(release)
	h.e.d.Sys = slowSys{h.sys, hold}
	return release
}

// Final review of plan 1A: a routing.disable that came while tailscale.setExitNode disabled before
// enabling again (phase stopping) did nothing, as the literal table of 5.1 said, and the enable
// after it switched routing back on: the user's disable was lost. Now the enable is given up: the
// disabling ends in off with "was on" cleared; the new exit node stays saved.
func TestDisableDuringReconnectCancelsTheEnable(t *testing.T) {
	h := enabled(t)
	release := holdStatus(t, h)
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"}); err != nil {
		t.Fatal(err)
	}
	if p := h.e.State().Phase; p != protocol.PhaseStopping {
		t.Fatalf("setup: phase %s", p)
	}
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	release()
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "starting,on,stopping,off" || h.rec.count("core:start") != 1 {
		t.Fatalf("the disable must cancel the enable: phases %s, events %v", got, h.rec.list())
	}
	if h.core.anyAlive() || h.store.hasJournal() || h.store.saved().WasOn || h.e.State().Reason != nil {
		t.Fatalf("off: no core, no journal, was-on cleared: %+v %v", h.e.State(), h.rec.list())
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode {
		t.Fatalf("the system exit node must be given back: %v", h.rec.list())
	}
	if got := h.store.saved().SelectedExitNode; got != "100.64.0.5" {
		t.Fatalf("the new exit node stays saved: %q", got)
	}
}
```

Тест старта идёт в `startup_test.go` после `TestStartAfterCrashWhileOn`.

Изменить `helper/internal/engine/startup_test.go`:

Найти:

```go
	if set < 0 || stop <= set || start <= stop || old.alive() {
		t.Fatalf("restore, then stop the old core, then start the new one (old alive %v): %v", old.alive(), h.rec.list())
	}
}
```

Заменить на:

```go
	if set < 0 || stop <= set || start <= stop || old.alive() {
		t.Fatalf("restore, then stop the old core, then start the new one (old alive %v): %v", old.alive(), h.rec.list())
	}
}

// Final review of plan 1A: the start-up of 5.7 with "was on" set gives the exit node back and stops
// the old core in phase stopping, then enables again. A routing.disable in that stopping half did
// nothing, and the enable after it switched routing on against the user's disable. Now it clears
// "was on": after the restore and the stop of the old core the start-up ends in off.
func TestDisableDuringStartupCancelsTheReenable(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	release()
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "stopping,off" || h.rec.count("core:start") != 0 {
		t.Fatalf("the disable must cancel the enable of the start-up: phases %s, events %v", got, h.rec.list())
	}
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("restore, then the old core stopped; journal gone, was-on cleared: %v", h.rec.list())
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestDisableDuring(Reconnect|Startup)Cancels' -count=1
```

Ожидается: два провала. В обоих после выключающей половины снова идёт включение (`starting,on`), выключение потеряно:

```text
--- FAIL: TestDisableDuringReconnectCancelsTheEnable (0.01s)
    commands_test.go:432: the disable must cancel the enable: phases starting,on,stopping,starting,on, events [rules:write core:check host:removestale core:start 1000 journal:save 100.64.0.1 true sys:clear dns:flush sys:set 100.64.0.1 true journal:delete core:interrupt 1000 rules:write core:check host:removestale core:start 1001 journal:save 100.64.0.1 true sys:clear dns:flush]
--- FAIL: TestDisableDuringStartupCancelsTheReenable (0.00s)
    startup_test.go:91: the disable must cancel the enable of the start-up: phases stopping,starting,on, events [sys:set 100.64.0.1 true journal:delete core:interrupt 777 rules:write core:check host:removestale core:start 1000 journal:save 100.64.0.1 true sys:clear dns:flush]
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 7: Снимать «было включено» по выключению во время перезапуска**

`routing.disable` без `skipRestore` в фазе `stopping` снимает признак «было включено», если текущая операция — переподключение или старт 5.7 и признак ещё стоит (`reenablesLocked`). Переподключение после 5.4 проверяет признак так же, как старт.

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
// cmdDisable: 5.4 with the phase table of 5.1; "was on" is cleared at the beginning. With
// skipRestore the journal goes without a restore in every phase that disables (5.1, 5.4).
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
```

Заменить на:

```go
// cmdDisable: 5.4 with the phase table of 5.1; "was on" is cleared at the beginning. With
// skipRestore the journal goes without a restore in every phase that disables (5.1, 5.4).
// In stopping a plain disable gives up the enable that the operation would start after its
// stopping half (reenablesLocked): "was on" is cleared, and the operation, which reads it before
// it enables, ends in off. With restore_pending it is also the extra attempt of "Повторить".
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
```

Найти:

```go
		pending := e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending
		if p.SkipRestore {
			e.skip = true
			if e.op != nil {
				e.op.cancel()
			}
		}
		if p.SkipRestore || pending {
```

Заменить на:

```go
		pending := e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending
		switch {
		case p.SkipRestore:
			e.skip = true
			if e.op != nil {
				e.op.cancel()
			}
		case e.reenablesLocked():
			e.saved.WasOn = false
			e.save()
		}
		if p.SkipRestore || pending {
```

Найти:

```go
	return nil
}

// cmdAllDirect: applied in on and degraded; the answer comes at once, the result in state events.
```

Заменить на:

```go
	return nil
}

// reenablesLocked: the current operation enables after its stopping half while "was on" is set:
// the reconnect of tailscale.setExitNode (7) or the start-up of 5.7; e.mu must be held. Both read
// "was on" right before they enable, so clearing it gives that enable up (5.1).
func (e *Engine) reenablesLocked() bool {
	return e.op != nil && e.saved.WasOn && (e.op.kind == "reconnect" || e.op.kind == "startup")
}

// cmdAllDirect: applied in on and degraded; the answer comes at once, the result in state events.
```

Найти:

```go
// cmdSetExitNode saves the choice; while routing is on it disables and enables again (7).
func (e *Engine) cmdSetExitNode(params json.RawMessage) *protocol.Error {
```

Заменить на:

```go
// cmdSetExitNode saves the choice; while routing is on it disables and enables again (7). The
// enable follows only while "was on" is still set: a routing.disable during the disabling (phase
// stopping) clears it, and the reconnect then ends in off (5.1).
func (e *Engine) cmdSetExitNode(params json.RawMessage) *protocol.Error {
```

Найти:

```go
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		e.runEnable(ctx, false)
	})
```

Заменить на:

```go
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		if !e.saved.WasOn {
			e.d.Log("\"was on\" was cleared during the reconnect: no enable")
			e.finish(protocol.PhaseOff, nil)
			e.mu.Unlock()
			return
		}
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		e.runEnable(ctx, false)
	})
```

Старт 5.7 уже читает признак перед включением; меняется только его описание.

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
// left from an earlier run is stopped; a foreign sing-box.exe is never touched. If "was on" is
// still set, the previous helper ended abnormally: enable again.
func (e *Engine) startup(ctx context.Context) {
```

Заменить на:

```go
// left from an earlier run is stopped; a foreign sing-box.exe is never touched. If "was on" is
// still set, the previous helper ended abnormally: enable again. A routing.disable in phase
// stopping clears it (cmdDisable, 5.1): the start-up then ends in off.
func (e *Engine) startup(ctx context.Context) {
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestDisableDuring(Reconnect|Startup)Cancels' -count=1 -v; go test ./internal/engine/ -count=1
```

Ожидается: `--- PASS: TestDisableDuringReconnectCancelsTheEnable`, `--- PASS: TestDisableDuringStartupCancelsTheReenable`, затем `ok  	splitlane/helper/internal/engine` для пакета целиком (`TestPhaseTable` без изменений: столбец `stopping` проверяет обычное выключение, за которым включения нет).

- [ ] **Step 9: Написать тест «Повторить» при `restore_pending`**

С причиной `restore_pending` обычный `routing.disable` — «Повторить»: попытка возврата сейчас, а включение после старта остаётся. Тест идёт в `startup_test.go` после `TestDisableDuringStartupCancelsTheReenable`.

Изменить `helper/internal/engine/startup_test.go`:

Найти:

```go
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("restore, then the old core stopped; journal gone, was-on cleared: %v", h.rec.list())
	}
}
```

Заменить на:

```go
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("restore, then the old core stopped; journal gone, was-on cleared: %v", h.rec.list())
	}
}

// In stopping with restore_pending a plain routing.disable is «Повторить» (5.1, 5.4): one more
// restore attempt now and nothing else. The start-up with "was on" set keeps its enable: once the
// restore confirms, the old core is stopped and routing comes back on.
func TestRetryInRestorePendingKeepsTheReenable(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on, s.ignoreSet = false, true })
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	if !h.store.hasJournal() || !h.store.saved().WasOn || !old.alive() {
		t.Fatalf("a retry keeps the journal, \"was on\" and the old core: was-on %v, events %v", h.store.saved().WasOn, h.rec.list())
	}
	h.sys.set(func(s *fakeSys) { s.ignoreSet = false })
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" || h.rec.count("core:start") != 1 {
		t.Fatalf("the retry must keep the enable of the start-up: phases %s, events %v", got, h.rec.list())
	}
	if old.alive() || !h.store.saved().WasOn {
		t.Fatalf("the old core stopped, \"was on\" set again: %v", h.rec.list())
	}
}
```

- [ ] **Step 10: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestRetryInRestorePendingKeepsTheReenable' -count=1
```

Ожидается: провал. Код шага 7 снимает «было включено» и при `restore_pending` (число строк `sys:set` в списке зависит от момента печати):

```text
--- FAIL: TestRetryInRestorePendingKeepsTheReenable (0.01s)
    startup_test.go:114: a retry keeps the journal, "was on" and the old core: was-on false, events [sys:set 100.64.0.1 true sys:set 100.64.0.1 true ...]
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 11: Оставить «Повторить» чистой попыткой**

Отмена включения — только без причины `restore_pending`; описания команды, переподключения и старта говорят то же.

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
// In stopping a plain disable gives up the enable that the operation would start after its
// stopping half (reenablesLocked): "was on" is cleared, and the operation, which reads it before
// it enables, ends in off. With restore_pending it is also the extra attempt of "Повторить".
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
```

Заменить на:

```go
// In stopping without restore_pending a plain disable gives up the enable that the operation would
// start after its stopping half (reenablesLocked): "was on" is cleared, and the operation, which
// reads it before it enables, ends in off. With restore_pending a plain disable is "Повторить":
// one more restore attempt now, and the enable that follows stays.
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
```

Найти:

```go
		case e.reenablesLocked():
			e.saved.WasOn = false
```

Заменить на:

```go
		case !pending && e.reenablesLocked():
			e.saved.WasOn = false
```

Найти:

```go
// enable follows only while "was on" is still set: a routing.disable during the disabling (phase
// stopping) clears it, and the reconnect then ends in off (5.1).
```

Заменить на:

```go
// enable follows only while "was on" is still set: a routing.disable during the disabling (phase
// stopping without restore_pending) clears it, and the reconnect then ends in off (5.1).
```

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
// still set, the previous helper ended abnormally: enable again. A routing.disable in phase
// stopping clears it (cmdDisable, 5.1): the start-up then ends in off.
```

Заменить на:

```go
// still set, the previous helper ended abnormally: enable again. A routing.disable in phase
// stopping without restore_pending clears it (cmdDisable, 5.1): the start-up then ends in off.
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestRetryInRestorePendingKeepsTheReenable|TestDisableDuring(Reconnect|Startup)Cancels|TestDisableInRestorePending' -count=1 -v; go test ./internal/engine/ -count=1
```

Ожидается: `--- PASS: TestDisableInRestorePending`, `--- PASS: TestDisableDuringReconnectCancelsTheEnable`, `--- PASS: TestDisableDuringStartupCancelsTheReenable`, `--- PASS: TestRetryInRestorePendingKeepsTheReenable`, затем `ok  	splitlane/helper/internal/engine` для пакета целиком.

- [ ] **Step 13: Записать правило в спецификацию**

В 5.1, в таблице поведения методов по фазам, строка `routing.disable`, столбец `stopping`: после «иначе без действия» вставляется правило об отмене включения без причины `restore_pending`, остальное в ячейке не меняется.

Изменить `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`:

Найти:

```markdown
| `routing.disable` | перейти в `off` | прервать включение, откат 5.4 | выключение 5.4 | при причине `restore_pending` — внеочередная попытка возврата (действие «Повторить»), иначе без действия; с `skipRestore: true` — выключение без возврата (5.4) |
```

Заменить на:

```markdown
| `routing.disable` | перейти в `off` | прервать включение, откат 5.4 | выключение 5.4 | при причине `restore_pending` — внеочередная попытка возврата (действие «Повторить»), иначе без действия; без причины restore_pending: если за этим выключением должно последовать включение (смена exit node, старт 5.7 с признаком «было включено»), включение отменяется и итоговая фаза — off; с `skipRestore: true` — выключение без возврата (5.4) |
```

- [ ] **Step 14: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё одно `ok  	splitlane/helper/internal/engine` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 15: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "fix(engine): all direct and lanes.set run as operations; a disable in stopping cancels the enable that follows" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 7: Повтор запуска ядра при tunnel_not_proven и уровень журнала ядра

Проверки плана 1B на ПК (приложение A, последний пункт): в двух запусках ядра из девяти собственный DNS ядра (сервер `local`, DNS физического адаптера) 44 и больше 48 секунд не получал ответов. Встроенный узел не мог разрешить имя сервера управления Tailscale («missing Tailscale IPv4 address»), прямая дорожка — ни одного имени. Один раз из-за этого истекли 45 секунд шага 5 из 5.3 (`tunnel_not_proven`), а следующее включение с новым процессом ядра сразу прошло. Причина не установлена. Задача делает две вещи. Первая — повтор: если шаг 5 кончился причиной `tunnel_not_proven`, служебный процесс останавливает это ядро так же, как на шаге 2 из 5.4 (штатный сигнал, не дольше 5 секунд, затем принудительное завершение), и один раз повторяет шаги 3–5 с новыми портами. Откат — только после второй такой неудачи. До шага 6 система не тронута, поэтому повтор безопасен. Причины `same_address` и `login_expired` не повторяются. Прежний повтор при выходе ядра до строки запуска (6.1) остаётся; каждый из двух повторов бывает не больше одного раза за включение, так что за одно включение стартует не больше трёх ядер. Шаги 3–5 собраны в новую функцию `startAndProve`; её вызывает `enable`, а через него — перезапуски 5.5 и включения без участия пользователя, поэтому повтор действует и там. Всё это время фаза — `starting`. Отмена (`routing.disable`) во время повтора, как и раньше, ведёт к откату и фазе `off`. Шаг 3 из 5.4 (ждать исчезновения адаптера) перед повтором не выполняется: устройство Wintun, оставшееся после принудительного завершения, удаляет шаг 4 следующего запуска (задача 2).

Вторая — флаг разработки `--core-log-level` для сбора сведений о задержке: значения `info` и `debug`, по умолчанию `info`, только вместе с `--dev`. Продукт всегда пишет журнал ядра на уровне `info` (6.1). Уровень идёт от флага в `engine.Settings.CoreLogLevel`, оттуда в `CoreOptions.LogLevel` каждого запуска ядра, а драйвер передаёт его в `coreconfig.Options.LogLevel`. Пустой уровень — `info`; любой уровень, кроме `info` и `debug`, — ошибка сборки конфига. Режим настройки следует тому же флагу: поле `LogLevel` есть и у `engine.SetupOptions`, и у `coreconfig.SetupOptions`, драйвер его передаёт. Заполняет его задача 8, которая собирает `SetupOptions` в автомате (`LogLevel: e.d.Settings.CoreLogLevel`).

**Files:**
- Modify: `helper/internal/coreconfig/config.go`, `helper/internal/coreconfig/setup.go`, `helper/internal/engine/enable.go`, `helper/internal/engine/ports.go`, `helper/internal/singbox/driver.go`, `helper/cmd/splitlane-helper/main.go`
- Test: `helper/internal/coreconfig/config_test.go`, `helper/internal/coreconfig/setup_test.go`, `helper/internal/engine/enable_test.go`, `helper/internal/singbox/driver_test.go`, `helper/internal/singbox/node_test.go`, `helper/cmd/splitlane-helper/main_test.go`
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (5.3, шаг 5; 6.1)

**Interfaces:**
- Consumes: `func (e *Engine) startCore(ctx context.Context) (*coreRun, *protocol.Reason, bool)`, `func (e *Engine) proveTunnel(ctx context.Context, r *coreRun) (string, *protocol.Reason)`, `func (e *Engine) stopCore()`, `func (e *Engine) runRestart(ctx context.Context, why string)`, `func (o Options) validate() error` и `func (o SetupOptions) validate() error` в `coreconfig`; в тестах — `fakeCore.onStart`, `fakeProbe.script`, `logEvents`, `onceOnState`, `inOrder`, `wantError`, `enabled`, `startEnable`, `systemExitNode`, `testOptions`, `testSetupOptions`, `parse`.
- Produces: поле `LogLevel string` у `engine.CoreOptions` и у `engine.SetupOptions`; поле `CoreLogLevel string` у `engine.Settings`; поле `LogLevel string` у `coreconfig.Options` и у `coreconfig.SetupOptions`; `const coreconfig.LogInfo = "info"`, `const coreconfig.LogDebug = "debug"`; флаг `--core-log-level` команды `splitlane-helper` (поле `coreLogLevel string` у `options`). Внутри пакетов: `func (e *Engine) startAndProve(ctx context.Context) (*coreRun, string, *protocol.Reason)`, `func logLevel(level string) (string, error)` в `coreconfig`; в тестах — `func probesPerCore(h *harness, answers map[int][2]ProbeResult)`.

- [ ] **Step 1: Написать тесты уровня журнала в конфиге**

Пустой уровень и `info` дают `"level": "info"`, `debug` — `"level": "debug"`; `trace`, `warn` и `INFO` (имена, которые ядро само приняло бы) отклоняются.

Изменить `helper/internal/coreconfig/config_test.go`:

Найти:

```go
func TestBuildRejectsBadOptions(t *testing.T) {
```

Заменить на:

```go
// Spec 6.1: the core log is written at level info; debug comes only from the development flag
// --core-log-level. Any other level is refused, also the names the core itself would take.
func TestLogLevel(t *testing.T) {
	for _, c := range []struct{ level, want string }{{"", `"level": "info"`}, {"info", `"level": "info"`}, {"debug", `"level": "debug"`}} {
		o := testOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = c.level
		b, err := Build(o)
		if err != nil || !bytes.Contains(b, []byte(c.want)) {
			t.Errorf("level %q: %v, want %s", c.level, err, c.want)
		}
	}
	for _, level := range []string{"trace", "warn", "INFO"} {
		o := testOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = level
		if _, err := Build(o); err == nil || !strings.Contains(err.Error(), "log level must be info or debug") {
			t.Errorf("level %q: want an error, got %v", level, err)
		}
	}
}

func TestBuildRejectsBadOptions(t *testing.T) {
```

Изменить `helper/internal/coreconfig/setup_test.go`:

Найти:

```go
	"path/filepath"
	"testing"
)
```

Заменить на:

```go
	"path/filepath"
	"strings"
	"testing"
)
```

Найти:

```go
func TestSetupConfigRejectsBadOptions(t *testing.T) {
```

Заменить на:

```go
// The setup mode follows --core-log-level as the main config does (spec 6.1).
func TestSetupLogLevel(t *testing.T) {
	for _, c := range []struct{ level, want string }{{"", `"level": "info"`}, {"info", `"level": "info"`}, {"debug", `"level": "debug"`}} {
		o := testSetupOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = c.level
		b, err := BuildSetup(o)
		if err != nil || !bytes.Contains(b, []byte(c.want)) {
			t.Errorf("level %q: %v, want %s", c.level, err, c.want)
		}
	}
	o := testSetupOptions(`C:\ProgramData\SplitLane`)
	o.LogLevel = "trace"
	if _, err := BuildSetup(o); err == nil || !strings.Contains(err.Error(), "log level must be info or debug") {
		t.Errorf("level trace: want an error, got %v", err)
	}
}

func TestSetupConfigRejectsBadOptions(t *testing.T) {
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/coreconfig/ -run 'Test(Setup)?LogLevel' -count=1
```

Ожидается:

```text
internal\coreconfig\config_test.go:155:5: o.LogLevel undefined (type Options has no field or method LogLevel)
internal\coreconfig\config_test.go:163:5: o.LogLevel undefined (type Options has no field or method LogLevel)
internal\coreconfig\setup_test.go:75:5: o.LogLevel undefined (type SetupOptions has no field or method LogLevel)
internal\coreconfig\setup_test.go:82:4: o.LogLevel undefined (type SetupOptions has no field or method LogLevel)
FAIL	splitlane/helper/internal/coreconfig [build failed]
```

- [ ] **Step 3: Уровень журнала в обоих конфигах**

Изменить `helper/internal/coreconfig/config.go`:

Найти:

```go
	ClashSecret  string
	APISecret    string
	DevPin       bool
}

// Paths are the files of the data folder that the config names.
```

Заменить на:

```go
	ClashSecret  string
	APISecret    string
	DevPin       bool
	LogLevel     string // "" or info (spec 6.1); debug only from the development flag --core-log-level
}

// Levels of the core log (spec 6.1): the product writes info; debug is for development only, to
// collect evidence of a failure (appendix A: the core's own DNS without answers at its start).
const (
	LogInfo  = "info"
	LogDebug = "debug"
)

// logLevel is the level a config writes: "" is info; any level but info and debug is an error.
func logLevel(level string) (string, error) {
	switch level {
	case "", LogInfo:
		return LogInfo, nil
	case LogDebug:
		return LogDebug, nil
	}
	return "", fmt.Errorf("coreconfig: log level must be info or debug, not %q", level)
}

// Paths are the files of the data folder that the config names.
```

Найти:

```go
	if err := o.validate(); err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	tsDir := o.TailscaleDir
```

Заменить на:

```go
	if err := o.validate(); err != nil {
		return nil, err
	}
	level, err := logLevel(o.LogLevel)
	if err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	tsDir := o.TailscaleDir
```

Найти:

```go
		"log": obj{"level": "info", "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{
			"servers": []any{
```

Заменить на:

```go
		"log": obj{"level": level, "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{
			"servers": []any{
```

Изменить `helper/internal/coreconfig/setup.go`:

Найти:

```go
	APIPort   int    // the API service of this run (spec 6.1)
	APISecret string
}
```

Заменить на:

```go
	APIPort   int    // the API service of this run (spec 6.1)
	APISecret string
	LogLevel  string // as Options.LogLevel: "" or info, debug only from --core-log-level
}
```

Найти:

```go
	if err := o.validate(); err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	config := obj{
		"log": obj{"level": "info", "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
```

Заменить на:

```go
	if err := o.validate(); err != nil {
		return nil, err
	}
	level, err := logLevel(o.LogLevel)
	if err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	config := obj{
		"log": obj{"level": level, "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test ./internal/coreconfig/ -run 'Test(Setup)?LogLevel' -count=1 -v; go test ./internal/coreconfig/ -count=1
```

Ожидается: `--- PASS: TestLogLevel`, `--- PASS: TestSetupLogLevel`, затем `ok  	splitlane/helper/internal/coreconfig` для пакета целиком. Эталоны `testdata/vpn.golden.json` и `testdata/setup.golden.json` не меняются: пустой уровень пишется как `info`. Проверка настоящим `sing-box check` (`TestCoreAcceptsConfigs`, `TestSetupConfigPassesCheck`) проходит.

- [ ] **Step 5: Написать тесты повтора**

`probesPerCore` задаёт ответы обоих входов проверки для каждого запускаемого ядра по его PID (подменное ядро нумерует процессы с 1000): ядро с PID из ключей получает свою пару (`probe-vpn`, `probe-direct`), остальные доказывают туннель. `TestTunnelNotProvenIsRetriedOnce` — первое ядро молчит все 45 секунд, второе отвечает: фаза `on`, два старта, первое ядро остановлено до второго, журнал возврата и `tailscale set` — только после доказательства на втором ядре (метка `test:proven` ставится при первой публикации адресов). `TestTunnelNotProvenTwiceRollsBack` — два молчащих ядра: откат, `tunnel_not_proven`, система не тронута; с выходом ядра до строки запуска перед ними стартуют три ядра, не больше. `TestSameAddressIsNotRetried` — `same_address` откатывается сразу, даже если второе ядро доказало бы туннель, и так же, когда его даёт ядро повтора. `TestRestartRetriesTheTunnelProof` — перезапуск 5.5 проходит тот же путь: повтор внутри одной попытки, а не неудачная попытка. Тесты идут после `TestTunnelNotProven`. Он сам не меняется: оба его случая теперь запускают по два ядра и по-прежнему кончаются причиной `tunnel_not_proven`.

Изменить `helper/internal/engine/enable_test.go`:

Найти:

```go
	h := newHarness(t)
	h.probe.script([]ProbeResult{atDirect}, []ProbeResult{atDirect}, nil)
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
}
```

Заменить на:

```go
	h := newHarness(t)
	h.probe.script([]ProbeResult{atDirect}, []ProbeResult{atDirect}, nil)
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
}

// probesPerCore scripts both probe inbounds for each core that starts: the core whose PID is a key
// of answers gets that pair (probe-vpn, probe-direct), every other core proves the tunnel.
func probesPerCore(h *harness, answers map[int][2]ProbeResult) {
	h.core.mu.Lock()
	defer h.core.mu.Unlock()
	h.core.onStart = func(p *fakeProc) {
		a, ok := answers[p.id.PID]
		if !ok {
			a = [2]ProbeResult{atVPN, atDirect}
		}
		h.probe.script([]ProbeResult{a[0]}, []ProbeResult{a[1]}, nil)
	}
}

// Appendix A (checks of plan 1B): the core's own DNS now and then gets no answers for the first
// tens of seconds. A tunnel that is not proven within the 45 s of 5.3 step 5 gets one more core with
// new ports, in phase starting; the system is not touched before that core has proven the tunnel.
func TestTunnelNotProvenIsRetriedOnce(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1000: {noAnswer, noAnswer}})
	onceOnState(h, func(s protocol.State) bool { return s.Addresses.VPN != nil }, func() { h.rec.add("test:proven") })
	h.load()
	before := h.clock.Now()
	startEnable(h)
	h.waitFor("the end of the enable", h.idle)
	if s := h.e.State(); s.Phase != protocol.PhaseOn {
		t.Fatalf("phase %s reason %+v, starts %d: %v", s.Phase, s.Reason, h.rec.count("core:start"), h.rec.list())
	}
	if n := h.rec.count("core:start"); n != 2 || h.rec.count("log:tunnel not proven; trying the core once more with new ports") != 1 {
		t.Fatalf("one retry expected, starts %d: %v", n, h.rec.list())
	}
	if !inOrder(h, 0, "core:start 1000", "log:tunnel not proven", "core:interrupt 1000", "core:start 1001", "test:proven", "journal:save", "sys:clear") {
		t.Fatalf("the first core stopped before the second, the system touched only after the proof: %v", h.rec.list())
	}
	if proven := h.rec.index("test:proven"); h.rec.index("sys:") < proven || h.rec.index("journal:") < proven || h.rec.index("sys:set") >= 0 {
		t.Fatalf("nothing switched or journaled before the second proof: %v", h.rec.list())
	}
	if waited := h.clock.Now().Sub(before); waited < proveWait {
		t.Fatalf("the first proof gave up after %s", waited)
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Two cores that do not prove the tunnel: the rollback of 5.4, error tunnel_not_proven, the system
// untouched. With a core that exits before its start line first, the enable starts three cores, not more.
func TestTunnelNotProvenTwiceRollsBack(t *testing.T) {
	for _, c := range []struct {
		name   string
		early  int
		starts int
	}{{"two cores", 0, 2}, {"early exit first", 1, 3}} {
		h := newHarness(t)
		h.core.dieEarly = c.early
		h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, nil)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonTunnelNotProven)
		if n := h.rec.count("core:start"); n != c.starts {
			t.Errorf("%s: starts %d, want %d: %v", c.name, n, c.starts, h.rec.list())
		}
		if h.rec.index("sys:") >= 0 || h.rec.index("journal:") >= 0 || h.core.anyAlive() {
			t.Errorf("%s: system untouched, no core left: %v", c.name, h.rec.list())
		}
		if on, ip := systemExitNode(h); !on || ip != exitNode {
			t.Errorf("%s: exit node on=%v ip=%s", c.name, on, ip)
		}
	}
}

// The retry is for tunnel_not_proven alone: same_address rolls back at once, even when a second
// core would prove the tunnel, and also when the core of the retry gives it (no third core).
func TestSameAddressIsNotRetried(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1000: {atDirect, atDirect}})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
	if n := h.rec.count("core:start"); n != 1 || h.rec.count("log:tunnel not proven") != 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("same_address: one core, no retry, system untouched; starts %d: %v", n, h.rec.list())
	}

	h = newHarness(t)
	probesPerCore(h, map[int][2]ProbeResult{1000: {noAnswer, noAnswer}, 1001: {atDirect, atDirect}})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
	if n := h.rec.count("core:start"); n != 2 || h.rec.index("sys:") >= 0 {
		t.Fatalf("same_address of the retried core: two cores, system untouched; starts %d: %v", n, h.rec.list())
	}
}

// The restarts of 5.5 run the same 5.3: a core of a restart that does not prove the tunnel gets one
// more core within the same attempt, not a failed attempt.
func TestRestartRetriesTheTunnelProof(t *testing.T) {
	h := enabled(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1001: {noAnswer, noAnswer}})
	h.core.last().crash()
	h.waitFor("third core on", func() bool {
		return h.rec.count("core:start") == 3 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if h.rec.count("log:restart attempt failed") != 0 || h.rec.count("log:tunnel not proven") != 1 {
		t.Fatalf("one restart attempt with a retried proof: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestTunnelNotProven(IsRetriedOnce|TwiceRollsBack)|TestSameAddressIsNotRetried|TestRestartRetriesTheTunnelProof' -count=1
```

Ожидается: четыре провала. Первое же `tunnel_not_proven` кончается откатом; в перезапуске оно — неудачная попытка (`restart attempt failed`), за ней идёт следующая (длинные списки событий сокращены):

```text
--- FAIL: TestTunnelNotProvenIsRetriedOnce (0.06s)
    enable_test.go:231: phase error reason &{Code:tunnel_not_proven Params:map[]}, starts 1: [rules:write core:check host:removestale core:start 1000 core:interrupt 1000]
--- FAIL: TestTunnelNotProvenTwiceRollsBack (0.11s)
    enable_test.go:265: two cores: starts 1, want 2: [rules:write core:check host:removestale core:start 1000 core:interrupt 1000]
    enable.go:85: core exited before its start line; trying again with new ports
    enable_test.go:265: early exit first: starts 2, want 3: [rules:write core:check host:removestale core:start 1000 rules:write core:check host:removestale core:start 1001 core:interrupt 1001]
--- FAIL: TestSameAddressIsNotRetried (0.06s)
    enable_test.go:293: reason &{Code:tunnel_not_proven Params:map[]}, want same_address; events [rules:write core:check host:removestale core:start 1000 core:interrupt 1000]
--- FAIL: TestRestartRetriesTheTunnelProof (0.06s)
    enable_test.go:310: one restart attempt with a retried proof: [... core:start 1001 log:restart attempt failed: tunnel_not_proven core:interrupt 1001 rules:write core:check host:removestale core:start 1002 journal:save 100.64.0.1 true sys:clear dns:flush]
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 7: Повторить шаги 3–5 один раз при `tunnel_not_proven`**

Шаги 3–5 переходят из `enable` в `startAndProve`. Цикл прежнего повтора при раннем выходе ядра становится частью этой функции, а у каждого из двух повторов — свой признак «уже был». Ядро, которое не доказало туннель, останавливает `stopCore` (шаг 2 из 5.4): ядро помечается `stopping`, поэтому его поток надзора ничего не делает; затем запуск начинается заново с шага 3, с новыми портами от `FreePorts`.

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
	cancel()
	// Steps 3 and 4; a core that exits before its start line gets one more try with new ports (6.1).
	var r *coreRun
	for attempt := 1; ; attempt++ {
		var why *protocol.Reason
		var retry bool
		r, why, retry = e.startCore(ctx)
		if why == nil {
			break
		}
		if !retry || attempt == 2 || ctx.Err() != nil {
			return why
		}
		e.d.Log("core exited before its start line; trying again with new ports")
	}
	// Step 5: the tunnel works, without touching the system VPN.
	directIP, why := e.proveTunnel(ctx, r)
	if why != nil {
		return why
	}
	e.mu.Lock()
```

Заменить на:

```go
	cancel()
	r, directIP, why := e.startAndProve(ctx)
	if why != nil {
		return why
	}
	e.mu.Lock()
```

Найти:

```go
// startCore is steps 3 and 4 of 5.3. retry is true when the core exited before its start line.
```

Заменить на:

```go
// startAndProve is steps 3–5 of 5.3 and returns the core that proved the tunnel with the address
// of the direct lane. A core that exits before its start line gets one more try with new ports
// (6.1). A tunnel that is not proven gets one more core with new ports too: the core's own DNS now
// and then gets no answers for the first tens of seconds (appendix A, checks of plan 1B), and the
// system is untouched before step 6. That core is stopped as in step 2 of 5.4 first. Each retry
// happens at most once per enable, so at most three cores start; same_address and login_expired
// are not retried. The restarts of 5.5 and the unattended enables run the same steps.
func (e *Engine) startAndProve(ctx context.Context) (*coreRun, string, *protocol.Reason) {
	exitRetried, proofRetried := false, false
	for {
		r, why, retry := e.startCore(ctx)
		if why != nil {
			if !retry || exitRetried || ctx.Err() != nil {
				return nil, "", why
			}
			exitRetried = true
			e.d.Log("core exited before its start line; trying again with new ports")
			continue
		}
		// Step 5: the tunnel works, without touching the system VPN.
		directIP, why := e.proveTunnel(ctx, r)
		if why == nil {
			return r, directIP, nil
		}
		if why.Code != protocol.ReasonTunnelNotProven || proofRetried || ctx.Err() != nil {
			return nil, "", why
		}
		proofRetried = true
		e.d.Log("tunnel not proven; trying the core once more with new ports")
		e.stopCore()
		if ctx.Err() != nil {
			return nil, "", errCancelled
		}
	}
}

// startCore is steps 3 and 4 of 5.3. retry is true when the core exited before its start line.
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestTunnelNotProven(IsRetriedOnce|TwiceRollsBack)|TestSameAddressIsNotRetried|TestRestartRetriesTheTunnelProof' -count=1 -v; go test ./internal/engine/ -count=1
```

Ожидается: `--- PASS: TestTunnelNotProvenIsRetriedOnce`, `--- PASS: TestTunnelNotProvenTwiceRollsBack`, `--- PASS: TestSameAddressIsNotRetried`, `--- PASS: TestRestartRetriesTheTunnelProof`, затем `ok  	splitlane/helper/internal/engine` для пакета целиком (`TestTunnelNotProven`, `TestEarlyExitIsRetriedOnce`, `TestStaleAdapterIsRemovedBeforeStart` и тесты перезапусков проходят без изменений).

- [ ] **Step 9: Написать тесты уровня журнала от флага до конфига**

Автомат передаёт `Settings.CoreLogLevel` в параметры каждого запуска ядра; драйвер — в оба конфига; команда принимает `--core-log-level` только с `--dev` (и `info`, указанный явно, без `--dev` — тоже ошибка разбора), только `info` или `debug`, по умолчанию `info`.

Изменить `helper/internal/engine/enable_test.go`:

Найти:

```go
// Without an exit node in the system client (or without the client) nothing is switched or journaled.
```

Заменить на:

```go
// The development flag --core-log-level reaches the config of the core (spec 6.1); without a level
// in Settings the options carry none, and the driver writes info.
func TestCoreLogLevelGoesToTheConfig(t *testing.T) {
	h := enabled(t)
	if o := h.core.options(); o.LogLevel != "" {
		t.Fatalf("without a level in Settings: %q", o.LogLevel)
	}
	h = newHarness(t)
	h.e.d.Settings.CoreLogLevel = "debug"
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if o := h.core.options(); o.LogLevel != "debug" {
		t.Fatalf("level %q", o.LogLevel)
	}
}

// Without an exit node in the system client (or without the client) nothing is switched or journaled.
```

Изменить `helper/internal/singbox/driver_test.go`:

Найти:

```go
		ClashSecret: "0123456789abcdef", APISecret: "fedcba9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_mode": "vpn-localdns"`, `"exit_node": "100.64.0.1"`, `"listen_port": 40002`,
```

Заменить на:

```go
		ClashSecret: "0123456789abcdef", APISecret: "fedcba9876543210", LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_mode": "vpn-localdns"`, `"exit_node": "100.64.0.1"`, `"listen_port": 40002`, `"level": "debug"`,
```

Изменить `helper/internal/singbox/node_test.go`:

Найти:

```go
	b, err := d.BuildSetupConfig(engine.SetupOptions{Hostname: "pc-splitlane", APIPort: 40004, APISecret: testAPISecret})
	if err != nil {
		t.Fatal(err)
	}
	p := coreconfig.PathsFor(d.DataDir)
	for _, want := range []string{`"output": "` + filepath.ToSlash(p.CoreLog) + `"`,
```

Заменить на:

```go
	b, err := d.BuildSetupConfig(engine.SetupOptions{Hostname: "pc-splitlane", APIPort: 40004, APISecret: testAPISecret, LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	p := coreconfig.PathsFor(d.DataDir)
	for _, want := range []string{`"output": "` + filepath.ToSlash(p.CoreLog) + `"`, `"level": "debug"`,
```

Изменить `helper/cmd/splitlane-helper/main_test.go`:

Найти:

```go
// A file that is not the pinned core is refused before anything starts (spec 4.3).
```

Заменить на:

```go
// --core-log-level is a development flag (spec 6.1): only with --dev, only info or debug; info by default.
func TestCoreLogLevelNeedsDev(t *testing.T) {
	paths := []string{"--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}
	for _, level := range []string{"debug", "info"} {
		if _, err := parse(append([]string{"--core-log-level", level}, paths...)); err == nil || !strings.Contains(err.Error(), "--core-log-level needs --dev") {
			t.Errorf("%s without --dev: %v", level, err)
		}
	}
	if _, err := parse(append([]string{"--dev", "--core-log-level", "trace"}, paths...)); err == nil || !strings.Contains(err.Error(), "info or debug") {
		t.Errorf("trace: %v", err)
	}
	o, err := parse(append([]string{"--dev", "--core-log-level", "debug"}, paths...))
	if err != nil || o.coreLogLevel != "debug" {
		t.Fatalf("debug: %+v %v", o, err)
	}
	if o, _ := parse(append([]string{"--dev"}, paths...)); o.coreLogLevel != "info" {
		t.Fatalf("default %q", o.coreLogLevel)
	}
}

// A file that is not the pinned core is refused before anything starts (spec 4.3).
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ ./internal/singbox/ ./cmd/splitlane-helper/ -run 'TestCoreLogLevelGoesToTheConfig|TestDriverSpeaksSingBox|TestDriverSetupConfig|TestCoreLogLevelNeedsDev' -count=1
```

Ожидается: три пакета не собираются (порядок пакетов в выводе может отличаться; у `engine` и у команды показаны не все строки ошибок: их пять и три):

```text
internal\engine\enable_test.go:73:30: o.LogLevel undefined (type CoreOptions has no field or method LogLevel)
internal\engine\enable_test.go:77:17: h.e.d.Settings.CoreLogLevel undefined (type Settings has no field or method CoreLogLevel)
FAIL	splitlane/helper/internal/engine [build failed]
internal\singbox\driver_test.go:188:67: unknown field LogLevel in struct literal of type engine.CoreOptions
internal\singbox\node_test.go:222:119: unknown field LogLevel in struct literal of type engine.SetupOptions
FAIL	splitlane/helper/internal/singbox [build failed]
cmd\splitlane-helper\main_test.go:38:21: o.coreLogLevel undefined (type options has no field or method coreLogLevel)
FAIL	splitlane/helper/cmd/splitlane-helper [build failed]
```

- [ ] **Step 11: Провести уровень от флага до конфига**

Изменить `helper/internal/engine/ports.go`:

Найти:

```go
	ClashSecret  string
	APISecret    string
	DevPin       bool // development only (Plan 1, safety rule 4)
}
```

Заменить на:

```go
	ClashSecret  string
	APISecret    string
	DevPin       bool   // development only (Plan 1, safety rule 4)
	LogLevel     string // the core's log level: "" is info (spec 6.1), debug only from Settings.CoreLogLevel
}
```

Найти:

```go
	Hostname  string
	APIPort   int
	APISecret string
}
```

Заменить на:

```go
	Hostname  string
	APIPort   int
	APISecret string
	LogLevel  string // as CoreOptions.LogLevel
}
```

Найти:

```go
	DevPin        bool   // development only, see coreconfig.DevPinProcesses
	HelperVersion string
	CoreVersion   string
}
```

Заменить на:

```go
	DevPin        bool   // development only, see coreconfig.DevPinProcesses
	HelperVersion string
	CoreVersion   string
	CoreLogLevel  string // development only: debug from --core-log-level; "" or info in the product (spec 6.1)
}
```

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
		DevPin: e.d.Settings.DevPin,
	})
```

Заменить на:

```go
		DevPin: e.d.Settings.DevPin, LogLevel: e.d.Settings.CoreLogLevel,
	})
```

Изменить `helper/internal/singbox/driver.go`:

Найти:

```go
		ClashSecret: o.ClashSecret, APISecret: o.APISecret, DevPin: o.DevPin,
	})
```

Заменить на:

```go
		ClashSecret: o.ClashSecret, APISecret: o.APISecret, DevPin: o.DevPin, LogLevel: o.LogLevel,
	})
```

Найти:

```go
		DataDir: d.DataDir, Hostname: o.Hostname, APIPort: o.APIPort, APISecret: o.APISecret,
	})
```

Заменить на:

```go
		DataDir: d.DataDir, Hostname: o.Hostname, APIPort: o.APIPort, APISecret: o.APISecret, LogLevel: o.LogLevel,
	})
```

Флаг проверяется до остальных условий: `fs.Visit` перечисляет только флаги, указанные в командной строке, поэтому без `--dev` отклоняется любой `--core-log-level`, в том числе `info`. Уровень пишется в первую строку `helper.log`.

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
type options struct {
	dev     bool
	dataDir string
	core    string
	pipe    string
	devPin  bool
}
```

Заменить на:

```go
type options struct {
	dev          bool
	dataDir      string
	core         string
	pipe         string
	devPin       bool
	coreLogLevel string
}
```

Найти:

```go
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	switch {
	case !o.dev:
```

Заменить на:

```go
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	fs.StringVar(&o.coreLogLevel, "core-log-level", coreconfig.LogInfo, "development only: the core's log level, info or debug (spec 6.1)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	switch {
	case given["core-log-level"] && !o.dev:
		return o, fmt.Errorf("--core-log-level needs --dev: the product always writes the core log at level info")
	case o.coreLogLevel != coreconfig.LogInfo && o.coreLogLevel != coreconfig.LogDebug:
		return o, fmt.Errorf("--core-log-level must be info or debug")
	case !o.dev:
```

Найти:

```go
	logf("splitlane-helper %s starting, dev=%v dev-pin=%v", version, o.dev, o.devPin)
```

Заменить на:

```go
	logf("splitlane-helper %s starting, dev=%v dev-pin=%v core-log-level=%s", version, o.dev, o.devPin, o.coreLogLevel)
```

Найти:

```go
			DevPin: o.devPin, HelperVersion: version, CoreVersion: singbox.CoreVersion},
```

Заменить на:

```go
			DevPin: o.devPin, HelperVersion: version, CoreVersion: singbox.CoreVersion, CoreLogLevel: o.coreLogLevel},
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ ./internal/singbox/ ./cmd/splitlane-helper/ -run 'TestCoreLogLevelGoesToTheConfig|TestDriverSpeaksSingBox|TestDriverSetupConfig|TestCoreLogLevelNeedsDev' -count=1 -v; go test ./internal/engine/ ./internal/singbox/ ./cmd/splitlane-helper/ -count=1
```

Ожидается: `--- PASS: TestCoreLogLevelGoesToTheConfig`, `--- PASS: TestDriverSpeaksSingBox`, `--- PASS: TestDriverSetupConfig`, `--- PASS: TestCoreLogLevelNeedsDev`, затем `ok` у `splitlane/helper/internal/engine`, `splitlane/helper/internal/singbox` и `splitlane/helper/cmd/splitlane-helper` (`TestParse` проходит без изменений).

- [ ] **Step 13: Записать решения в спецификацию**

Изменить `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`:

Найти:

```markdown
без адреса прямой дорожки сравнивать не с чем. Если адреса совпали — откат с причиной `same_address`.
```

Заменить на:

```markdown
без адреса прямой дорожки сравнивать не с чем. Если адреса совпали — откат с причиной `same_address`. Если доказать не удалось (`tunnel_not_proven`), служебный процесс один раз останавливает это ядро и повторяет шаги 3–5 с новыми портами: собственный DNS ядра иногда первые десятки секунд не получает ответов (приложение A, проверки плана 1B); откат — только после второй неудачи.
```

Найти:

```markdown
без цветовых кодов: строка запуска (5.3, шаг 4) и ссылка для входа пишутся на этом уровне.
```

Заменить на:

```markdown
без цветовых кодов: строка запуска (5.3, шаг 4) и ссылка для входа пишутся на этом уровне. В режиме разработки флаг `--core-log-level debug` включает подробный журнал для разбора сбоев.
```

- [ ] **Step 14: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё одно `ok  	splitlane/helper/internal/engine` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 15: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/coreconfig helper/internal/engine helper/internal/singbox helper/cmd/splitlane-helper docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "feat(helper): one more core when the tunnel is not proven; the development flag --core-log-level" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 8: Коннектор: режим настройки, вход и выход

Раздел 7 спецификации: вход во встроенный узел идёт через ядро в режиме настройки — без виртуального адаптера, с локальным DNS и службой API. Конфиг этого режима и клиента службы API сделала задача 3 (`CoreDriver.BuildSetupConfig`, `CoreDriver.Node`, `singbox.Node`), поле уровня журнала `SetupOptions.LogLevel` — задача 7. Эта задача добавляет в автомат сам режим настройки и методы `tailscale.login` и `tailscale.logout` (до неё они отвечали `internal`), а в `slctl` — команды `login` и `logout`.

Режим настройки (новый файл `internal/engine/connector.go`) — один прогон ядра: свой порт службы API из `Host.FreePorts`, секрет из `Host.Secret`, уровень журнала из `Settings.CoreLogLevel`, запись конфига, `sing-box check`, запуск, идентификатор процесса ядра в `state.json` (5.7), строка запуска — не дольше 30 секунд. Устройство Wintun перед запуском не удаляется: адаптера у режима настройки нет. Режим работает только в фазах `off` и `error` и фазу не меняет. С рабочим ядром одновременно он не работает никогда (у них общий каталог состояния узла): прогон сначала ждёт конца операции, которая ещё завершается, а первое действие включения — до предварительных проверок 5.3, шаг 1 — `stopSetup`: прогон отменяется, служебный процесс дожидается конца его ядра (штатный сигнал, 5.4, шаг 2). Ядро, которое ещё стартует, тоже останавливается: прогон владеет процессом с запуска до конца. Поэтому проверка «запущено другое ядро sing-box» шага 1 своё ядро режима настройки уже не видит (5.3, шаг 1). Запускают режим `tailscale.login` — повторный вход, пока режим работает, ничего нового не запускает, ссылка в состоянии остаётся прежней (Review Focus 5), — и подключение окна (`SetClients` от 0 к числу больше нуля), если сохранён признак «вход выполнен». Останавливают его включение, `helper.shutdown` (до `OnExit`), конец сеанса Windows (принудительно, без ожидания, как рабочее ядро) и 5 минут без окон. Отсчёт идёт от ухода последнего окна, новое подключение его сбрасывает (Review Focus 1); если за прогон ни одно окно не уходило, отсчёт идёт от начала опроса. `SetClients` канал вызывает под своей блокировкой, а опубликованное состояние берёт её после `e.mu`, поэтому `SetClients` блокировок автомата не берёт: запуск режима идёт отдельной горутиной, время ухода последнего окна хранится атомарно и читается циклом опроса.

Пока ядро режима настройки работает, служебный процесс раз в 2 секунды читает `NodeAPI.State`: из него `tailscale.backend` и `authUrl` (пустая ссылка — `null`). «Вход выполнен» (раздел 7) — ссылки нет, у узла есть адрес в сети Tailscale, `backend` равен `starting` или `running`. Тогда в `state.json` записываются признак и список `ExitNodes`; этот список показывает состояние, пока ядро не запущено (5.2). `needs_login` снимает признак, `needs_approval` его не трогает. Каждое изменение публикуется, `backend` и список — одним состоянием. Когда режим настройки не работает, `backend` — `stopped`: прежнее правило `publish()` («running или stopped по фазе») заменено полями, которые ставит коннектор; пока рабочее ядро работает в фазах `on`, `degraded`, `blocked`, остаётся `running`, как было (уточнение по службе API рабочего ядра — задача 9).

`tailscale.logout` в `off` и `error`: запустить режим настройки, если он не работает, дождаться ответа его службы API (не дольше 30 секунд), `NodeAPI.Logout`, снять «вход выполнен», список exit node оставить, опубликовать. Ядро режима настройки после этого продолжает работать: ядро после выхода само заводит новую ссылку для входа (`Endpoint.Logout` в sing-box 1.14.4). Ответ опроса, полученный до выхода, признак обратно не ставит: выход сдвигает счётчик прогона, и такой ответ отбрасывается. Оба метода отвечают сразу после проверки фазы (5.1): `starting`, `stopping` и время после принятого `helper.shutdown` — `busy`, `on`, `degraded`, `blocked` — `wrong_phase` (ячейка `degraded` с `backend = expired` — задача 9). Выход выполняется в фоне, по одному; включение и `helper.shutdown`, пришедшие во время выхода, ждут его конца, а начавшееся включение прерывает ожидание службы API. Ошибки — в журнал и событие `state`. Ошибки службы API несут аргументы командной строки ядра и его stderr, где бывают имена и адреса узлов сети, поэтому в журнал они попадают только через `protocol.MaskAddresses` (`<ip>`), одна и та же ошибка опроса — один раз подряд.

Ядро режима настройки, оставшееся от упавшего служебного процесса, при старте останавливается так же, как оставшееся рабочее ядро (5.7, фаза `stopping`): идентификатор в `state.json` тот же. Отдельного кода для этого нет, достаточно записи идентификатора при запуске; тест закрепляет поведение.

`slctl login [-timeout 300s]` отправляет `tailscale.login`, печатает `SLCTL: backend=<b>` при каждой смене и один раз — `SLCTL: open this link to log in: <url>` (ссылка идёт только в консоль пользователя) и выходит с кодом 0, когда `backend` равен `starting` или `running`, а `authUrl` — `null`. `slctl logout [-timeout 60s]` выходит с кодом 0 при `backend = needs_login`. Свой `-timeout` у команды заменяет общий только для ожидания узла; соединение и `hello` по-прежнему ограничивает общий. Состояния команда видит и в событиях, и в ответе `state.get`, поэтому ссылка не теряется, даже если пришла, пока клиент ждал ответа на свой запрос.

**Files:**
- Create: `helper/internal/engine/connector.go`, `helper/internal/engine/connector_test.go`
- Modify: `helper/internal/engine/engine.go`, `helper/internal/engine/commands.go`, `helper/internal/engine/enable.go`, `helper/internal/engine/startup.go`, `helper/cmd/slctl/main.go`
- Test: `helper/internal/engine/fakes_test.go`, `helper/internal/engine/commands_test.go`, `helper/internal/engine/supervise_test.go`, `helper/cmd/slctl/main_test.go`

**Interfaces:**
- Consumes: `NodeAPI` (`State`, `ExitNodes`, `Logout`), `NodeState`, `SetupOptions` (с `LogLevel`), `CoreDriver.BuildSetupConfig`, `CoreDriver.Node`, `CoreDriver.Check`, `CoreDriver.Start`, `CoreDriver.Events`, `Host.FreePorts`, `Host.Secret`, `Store.WriteConfig`, `Settings.CoreLogLevel`, `protocol.BackendStopped`, `protocol.BackendNeedsLogin`, `protocol.BackendStarting`, `protocol.BackendRunning`, `protocol.BackendNeedsApproval`, `protocol.MaskAddresses`; внутри пакета — `func (e *Engine) stopProc(p CoreProc)`, `func (e *Engine) sleep(ctx context.Context, d time.Duration) bool`, `func (e *Engine) save() error`, `func (e *Engine) publish()`, `func (e *Engine) hostname() string`, `busy`, `wrongPhase`, `noParams`, `exited`; в тестах — `fakeNode` (`h.core.node`, `set`), `fakeCore.setupOpts`, `inPhase`, `logEvents`, `inOrder`, `exitedNow`, `cellAct`, `call`, `wantCode`; в `slctl` — `serveHelper`, `testPipeName`, `fakeHelper`, `client.call`, `client.waitFor`, `client.finish`, `client.fail`.
- Produces: нового экспортируемого API в `engine` нет: методы идут через `Handle`, у `publish` прежняя сигнатура, новых полей у `Deps` и `Settings` нет; у `func (e *Engine) SetClients(n int)` прежняя сигнатура и новое поведение. Внутри пакета: `type setupRun struct{ cancel context.CancelFunc; done, ready chan struct{}; node NodeAPI; proc CoreProc; ending, again bool; gen int }`, `const nodeEvery = 2 * time.Second`, `const setupIdle = 5 * time.Minute`, `func (e *Engine) cmdLogin(params json.RawMessage) *protocol.Error`, `func (e *Engine) cmdLogout(params json.RawMessage) *protocol.Error`, `func (e *Engine) setupPhaseLocked() *protocol.Error`, `func (e *Engine) windowCame()`, `func (e *Engine) startSetupLocked()`, `func (e *Engine) stopSetup()`, `func (e *Engine) setupMayStart(ctx context.Context) bool`, `func (e *Engine) runSetup(ctx context.Context, s *setupRun)`, `func (e *Engine) windowless(since time.Time) bool`, `func (e *Engine) readNode(ctx context.Context, s *setupRun) error`, `func (e *Engine) logout()`, `func (e *Engine) logoutFailed(why string)`; поля `Engine`: `lastGone atomic.Int64`, `setup *setupRun`, `nodeBackend string`, `nodeAuthURL string`, `connector sync.Mutex`. В тестах: поле `fakeProc.setup`, поле `fakeCore.setupNext`, `func (c *fakeCore) setupAlive() bool`, событие `core:start <pid> setup`, `fakeHost.CoreRunning` видит работающее ядро режима настройки; `func shutdown(t *testing.T, h *harness)`, `type gateCore`, `func gateSetup(h *harness) *gateCore`, `func setupRunning(t *testing.T) (*harness, *fakeProc)`, `func backendIs(h *harness, backend string) bool`, `func waitClosed(t *testing.T, ch chan struct{}, what string)`, `const loginURL`. Команды `slctl login [-timeout 300s]` и `slctl logout [-timeout 60s]`; в `slctl`: поле `client.onState func(protocol.State)`, `var nodeTimeouts`, `func nodeCommand(c *client, cmd string, args []string) int`, `func loggedIn(s protocol.State) bool`; в тесте — поле `fakeHelper.login []protocol.Tailscale`.

- [ ] **Step 1: Написать тесты коннектора**

Подмены. `fakeCore` отмечает ядро, запущенное из конфига режима настройки (поле `setup` у `fakeProc`), и пишет для него событие `core:start <pid> setup`. `fakeHost.CoreRunning`, как настоящая проверка (любой `sing-box.exe`), видит работающее ядро режима настройки: если включение не остановит его первым, предварительная проверка ответит `other_core`. `tailscale.login` и `tailscale.logout` уходят из списка методов, которые отвечают `internal`. `TestPollWhileOffNeedsAWindow` подключает окно при сохранённом входе; теперь это запускает режим настройки, поэтому тест начинается без входа.

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
	ignoreInterrupt bool
	interruptErr    error
}
```

Заменить на:

```go
	ignoreInterrupt bool
	interruptErr    error
	setup           bool // a core of the setup mode (spec 7): started from the setup config
}
```

Найти:

```go
	node      *fakeNode    // the API service of every run
}

func (c *fakeCore) BuildConfig(o CoreOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.opts = o
	return json.Marshal(o)
}
```

Заменить на:

```go
	node      *fakeNode    // the API service of every run
	setupNext bool         // the last config built was the setup mode's: the next start is a setup core
}

func (c *fakeCore) BuildConfig(o CoreOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.opts = o
	c.setupNext = false
	return json.Marshal(o)
}
```

Найти:

```go
	p := &fakeProc{rec: c.rec, id: store.CoreIdentity{PID: 1000 + len(c.procs), Created: int64(len(c.procs) + 1), Path: `C:\core\sing-box.exe`}, done: make(chan struct{})}
	c.procs = append(c.procs, p)
	c.rec.add("core:start %d", p.id.PID)
```

Заменить на:

```go
	p := &fakeProc{rec: c.rec, id: store.CoreIdentity{PID: 1000 + len(c.procs), Created: int64(len(c.procs) + 1), Path: `C:\core\sing-box.exe`},
		done: make(chan struct{}), setup: c.setupNext}
	c.procs = append(c.procs, p)
	if p.setup {
		c.rec.add("core:start %d setup", p.id.PID)
	} else {
		c.rec.add("core:start %d", p.id.PID)
	}
```

Найти:

```go
	c.setupOpts = o
	return json.Marshal(o)
}
```

Заменить на:

```go
	c.setupOpts = o
	c.setupNext = true
	return json.Marshal(o)
}

// setupAlive tells whether a core of the setup mode runs.
func (c *fakeCore) setupAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.procs {
		if p.setup && p.alive() {
			return true
		}
	}
	return false
}
```

Найти:

```go
func (h *fakeHost) CoreRunning() bool    { h.mu.Lock(); defer h.mu.Unlock(); return h.otherCore }
func (h *fakeHost) AdapterPresent() bool { return h.adapterStuck || h.core.anyAlive() }
```

Заменить на:

```go
func (h *fakeHost) AdapterPresent() bool { return h.adapterStuck || h.core.anyAlive() }

// CoreRunning sees a foreign sing-box.exe (otherCore) and, as the real check does, a core of the
// setup mode that still runs: step 1 of 5.3 must never meet one (spec 7).
func (h *fakeHost) CoreRunning() bool {
	h.mu.Lock()
	other := h.otherCore
	h.mu.Unlock()
	return other || h.core.setupAlive()
}
```

Изменить `helper/internal/engine/commands_test.go`:

Найти:

```go
	for _, m := range []string{protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport} {
```

Заменить на:

```go
	for _, m := range []string{protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport} {
```

Найти:

```go
// (tailscale.login and tailscale.logout answer internal in every phase: TestUnknownAndLaterMethods).
```

Заменить на:

```go
// (tailscale.login and tailscale.logout: TestLoginPhaseTable).
```

Изменить `helper/internal/engine/supervise_test.go`:

Найти:

```go
func TestPollWhileOffNeedsAWindow(t *testing.T) {
	h := newHarness(t)
	h.load()
```

Заменить на:

```go
func TestPollWhileOffNeedsAWindow(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false // so the window starts no setup mode (spec 7): only 5.6 is looked at here
	h.load()
```

Тесты коннектора. Каждый кончается `helper.shutdown` (помощник `shutdown`), так что ни одно ядро режима настройки не переживает свой тест. `gateCore` держит проверку конфига режима настройки, пока тест её не отпустит: ядро «ещё стартует», и видно, что ответ метода пришёл до запуска ядра. Часы подмены не ждут: каждое ожидание цикла опроса сдвигает время на 2 секунды, так что «5 минут» проходят за доли секунды настоящего времени. Время в тестах на 5 минут читается так, чтобы граница всегда была в пользу проверки: перед уходом окна — для «не раньше 5 минут», после ухода — для «окно вернулось вовремя».

- `TestLoginStartsSetupMode` — окно без входа ничего не запускает; `tailscale.login` отвечает до запуска ядра; порядок `core:setupconfig`, `core:check`, `core:start 1000 setup`; без удаления устройства Wintun и без вызовов системного Tailscale; `SetupOptions{Hostname: "PC-splitlane", APIPort: 40001, APISecret, LogLevel: "debug"}`; идентификатор ядра в `state.json`; фаза всё время `off`, в состоянии `needs_login` и ссылка.
- `TestLoginTwiceKeepsOneSetupCore` — Review Focus 5.
- `TestSetupModeSurvivesReconnects` — Review Focus 1: переподключения, пока ядро стартует; 7 минут с окном после ухода и возврата — ядро работает; остановка — не раньше 5 минут после ухода последнего окна; одно ядро за весь тест.
- `TestSetupStopsFiveMinutesAfterLastWindow` — остановка через 5–6 минут после ухода окна, штатным сигналом; признак и список в `state.json` остаются.
- `TestEnableStopsSetupFirst` — ядро режима настройки завершено до запуска рабочего ядра, причины `other_core` нет, фазы `off,starting,on`; ядро, которое ещё стартует, тоже не доживает до рабочего.
- `TestLoggedInIsSaved` — ссылка есть — не вход; `starting` без адреса — не вход; `starting` с адресом — вход и список exit node в `state.json` и в состоянии; после перезапуска служебного процесса список и признак на месте, `backend` — `stopped`.
- `TestNeedsLoginClearsLoggedIn`, `TestLogoutClearsLoggedIn` — признак снимается, список остаётся; выход отвечает сразу, ядро режима настройки после выхода работает.
- `TestNodeErrorsAreMasked` — ошибка опроса и ошибка выхода попадают в журнал с `<ip>`, без адреса; неудачный выход ничего не меняет.
- `TestShutdownStopsSetup`, `TestSessionEndKillsSetup` — штатная остановка до `OnExit`; в конце сеанса — принудительная, без сигнала.
- `TestLeftoverSetupCoreIsStopped` — 5.7 для ядра режима настройки.
- `TestLoginPhaseTable` — ячейки 5.1 для обоих методов в `off`, `error`, `starting`, `on`, `blocked`, `stopping`.

Файл `helper/internal/engine/connector_test.go`, целиком:

```go
package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
)

// loginURL is the login link the fake node offers while it needs a login.
const loginURL = "https://login.example/a/1f2e3d"

// shutdown is helper.shutdown and its end: the setup mode is stopped before the process ends (spec 7).
// Each test of the connector ends with it, so no setup core outlives its test.
func shutdown(t *testing.T, h *harness) {
	t.Helper()
	if err := call(h, protocol.MethodShutdown, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("exit", func() bool { return exitedNow(h) })
}

// gateCore holds the check of the setup config until release is closed (or its context ends);
// entered is closed when the first such check came. The checks of the main core go through at once.
type gateCore struct {
	*fakeCore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *gateCore) Check(ctx context.Context, path string) error {
	c.fakeCore.mu.Lock()
	setup := c.fakeCore.setupNext
	c.fakeCore.mu.Unlock()
	if setup {
		c.once.Do(func() { close(c.entered) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.fakeCore.Check(ctx, path)
}

// gateSetup puts a gateCore in front of the fake core: the setup core stays "still starting".
func gateSetup(h *harness) *gateCore {
	g := &gateCore{fakeCore: h.core, entered: make(chan struct{}), release: make(chan struct{})}
	h.e.d.Core = g
	return g
}

func waitClosed(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// backendIs: the state shows this tailscale.backend now.
func backendIs(h *harness, backend string) bool { return h.e.State().Tailscale.Backend == backend }

// setupRunning is a logged-in helper in phase off to which a window has connected: the setup core
// runs and its node has been read (backend running).
func setupRunning(t *testing.T) (*harness, *fakeProc) {
	t.Helper()
	h := newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	return h, h.core.last()
}

// tailscale.login in off (spec 7, 5.1): the answer comes at once, before the core starts; the setup
// core starts without an adapter from its own config: the API port from FreePorts, the secret, the log
// level of Settings; the config is checked, the identity goes into state.json (5.7). The phase stays
// off; the node's state and its login link reach the state. A window alone starts nothing while the
// node is not logged in.
func TestLoginStartsSetupMode(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.e.d.Settings.CoreLogLevel = "debug"
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	time.Sleep(20 * time.Millisecond)
	if len(h.rec.list()) != 0 {
		t.Fatalf("a window starts no setup mode before a login: %v", h.rec.list())
	}
	g := gateSetup(h)
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, g.entered, "the check of the setup config")
	if h.rec.count("core:start") != 0 {
		t.Fatalf("the answer comes before the core starts: %v", h.rec.list())
	}
	close(g.release)
	h.waitFor("the login link", func() bool {
		u := h.e.State().Tailscale.AuthURL
		return u != nil && *u == loginURL
	})
	if s := h.e.State(); s.Phase != protocol.PhaseOff || s.Tailscale.Backend != protocol.BackendNeedsLogin || strings.Join(h.phases(), ",") != "off" {
		t.Fatalf("phase off throughout, backend needs_login: %+v, phases %v", s, h.phases())
	}
	if !inOrder(h, 0, "core:setupconfig", "core:check", "core:start 1000 setup") || h.rec.index("host:removestale") >= 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("setup config, check, start; no adapter, the system untouched: %v", h.rec.list())
	}
	h.core.mu.Lock()
	o := h.core.setupOpts
	h.core.mu.Unlock()
	if o != (SetupOptions{Hostname: "PC-splitlane", APIPort: portVPN, APISecret: h.host.Secret(), LogLevel: "debug"}) {
		t.Fatalf("setup options %+v", o)
	}
	if c := h.store.saved().Core; c == nil || *c != h.core.last().id {
		t.Fatalf("the identity of the setup core in state.json: %+v", c)
	}
}

// Review Focus 5: tailscale.login again while the setup core runs and its link is there: no second
// core, the same link, no error.
func TestLoginTwiceKeepsOneSetupCore(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the login link", func() bool { return h.e.State().Tailscale.AuthURL != nil })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatalf("a second login is no error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	u := h.e.State().Tailscale.AuthURL
	if h.rec.count("core:start") != 1 || h.rec.count("core:setupconfig") != 1 || u == nil || *u != loginURL || !h.core.last().alive() {
		t.Fatalf("one setup core, the same link: %v %v", u, h.rec.list())
	}
}

// Review Focus 1: a window connects and disconnects several times while the setup core is still
// starting: one setup core runs. The 5 minutes count from the last disconnect: a new connection
// resets them, and the core stops 5 minutes after the window that went last. The core never
// outlives its run.
func TestSetupModeSurvivesReconnects(t *testing.T) {
	h := newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	h.e.SetClients(1)
	waitClosed(t, g.entered, "the check of the setup config")
	for i := 0; i < 3; i++ {
		h.e.SetClients(0)
		h.e.SetClients(1)
	}
	close(g.release)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	setup := h.core.last()
	h.e.SetClients(0)
	first := h.clock.Now() // read after: the disconnect is not later than this
	h.e.SetClients(1)
	h.waitFor("7 minutes with a window", func() bool { return h.clock.Now().Sub(first) > 7*time.Minute })
	if !setup.alive() || h.rec.count("core:start") != 1 {
		t.Fatalf("the reconnect resets the 5 minutes; one setup core: %v", h.rec.list())
	}
	last := h.clock.Now() // read first: the disconnect is not earlier than this
	h.e.SetClients(0)
	h.waitFor("the setup core stopped", func() bool { return !setup.alive() && backendIs(h, protocol.BackendStopped) })
	if waited := h.clock.Now().Sub(last); waited < 5*time.Minute || h.rec.count("core:start") != 1 || h.core.anyAlive() {
		t.Fatalf("stopped %s after the last window went; events %v", waited, h.rec.list())
	}
}

// Spec 7: the setup core stops 5 minutes after the last window disconnected, gracefully (step 2 of
// 5.4); the saved login and exit nodes stay, the state shows backend stopped.
func TestSetupStopsFiveMinutesAfterLastWindow(t *testing.T) {
	h, setup := setupRunning(t)
	gone := h.clock.Now() // read first: the disconnect is not earlier than this
	h.e.SetClients(0)
	h.waitFor("the setup core stopped", func() bool { return !setup.alive() && backendIs(h, protocol.BackendStopped) })
	if waited := h.clock.Now().Sub(gone); waited < 5*time.Minute || waited > 6*time.Minute {
		t.Fatalf("stopped %s after the last window went", waited)
	}
	if h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)) < 0 || h.rec.index("core:kill") >= 0 {
		t.Fatalf("the graceful signal: %v", h.rec.list())
	}
	if s := h.store.saved(); !s.LoggedIn || len(s.ExitNodes) != 1 || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("state.json %+v, phase %s", s, h.e.State().Phase)
	}
}

// Spec 7 and 5.3 step 1: the first action of an enable stops the setup core and waits for its end;
// only then the preflight runs, so its check for another core does not see it (the fake host counts a
// setup core as another sing-box.exe), and the main core starts after it. A setup core that is still
// starting is stopped the same way and never runs beside the main core.
func TestEnableStopsSetupFirst(t *testing.T) {
	h, setup := setupRunning(t)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	stop, start := h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)), h.rec.index("core:start 1001")
	if setup.alive() || stop < 0 || start < stop || h.sawReason(protocol.ReasonOtherCore) {
		t.Fatalf("the setup core ends before the main core starts: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "off,starting,on" {
		t.Fatalf("phases %s", got)
	}

	h = newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	h.e.SetClients(1)
	waitClosed(t, g.entered, "the check of the setup config")
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	time.Sleep(20 * time.Millisecond)
	if h.rec.count("core:start") != 1 || h.rec.index("core:start 1000 setup") >= 0 || h.core.setupAlive() {
		t.Fatalf("the starting setup core is given up, only the main core runs: %v", h.rec.list())
	}
}

// Spec 7: "logged in" is no login link, an address in the tailnet and the backend starting or running
// (starting counts: with the system exit node on, the node of the setup mode stays there). Then the
// flag and the exit nodes go into state.json; the state shows them while no core runs, and they survive
// a restart of the helper.
func TestLoggedInIsSaved(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the login link", func() bool { return h.e.State().Tailscale.AuthURL != nil })
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendStarting} })
	h.waitFor("starting without a link", func() bool {
		s := h.e.State().Tailscale
		return s.Backend == protocol.BackendStarting && s.AuthURL == nil
	})
	if h.store.saved().LoggedIn {
		t.Fatal("no address in the tailnet yet: not logged in")
	}
	nodes := []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}, {ID: "100.64.0.2", Name: "exit-b", Online: false}}
	h.core.node.set(func(n *fakeNode) {
		n.state = NodeState{Backend: protocol.BackendStarting, HasAddress: true}
		n.nodes = nodes
	})
	h.waitFor("logged in and the exit nodes saved", func() bool {
		s := h.store.saved()
		return s.LoggedIn && slices.Equal(s.ExitNodes, nodes) && slices.Equal(h.e.State().Tailscale.ExitNodes, nodes)
	})
	shutdown(t, h)
	next := newHarness(t)
	next.store.state = h.store.saved()
	next.e.Start()
	next.waitPhase(protocol.PhaseOff)
	if s := next.e.State().Tailscale; !slices.Equal(s.ExitNodes, nodes) || s.Backend != protocol.BackendStopped || !next.store.saved().LoggedIn {
		t.Fatalf("after a restart: %+v, logged in %v", s, next.store.saved().LoggedIn)
	}
}

// Spec 7: the node of the setup core needs a login (the key expired, the device was removed in the
// admin console): "logged in" goes from state.json, the last list of exit nodes stays, the state shows
// needs_login with the new link.
func TestNeedsLoginClearsLoggedIn(t *testing.T) {
	h, _ := setupRunning(t)
	list := h.store.saved().ExitNodes
	if len(list) != 1 {
		t.Fatalf("the exit nodes of the logged-in node are saved: %+v", list)
	}
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.waitFor("logged in cleared", func() bool { return !h.store.saved().LoggedIn })
	h.waitFor("needs_login with the link", func() bool {
		s := h.e.State().Tailscale
		return s.Backend == protocol.BackendNeedsLogin && s.AuthURL != nil && *s.AuthURL == loginURL
	})
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) || !slices.Equal(h.e.State().Tailscale.ExitNodes, list) {
		t.Fatalf("the list stays: %+v", got)
	}
}

// tailscale.logout in off (spec 7): the answer comes at once; the setup mode starts because it does not
// run, its API service is waited for, the node logs out through the core and "logged in" goes; the list
// of exit nodes stays and the setup core keeps running (the new login link comes from it).
func TestLogoutClearsLoggedIn(t *testing.T) {
	h := newHarness(t)
	list := []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}}
	h.store.state.ExitNodes = list
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	if err := call(h, protocol.MethodLogout, nil); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, g.entered, "the check of the setup config")
	if h.rec.count("node:logout") != 0 || !h.store.saved().LoggedIn {
		t.Fatalf("the answer comes before the logout: %v", h.rec.list())
	}
	close(g.release)
	h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
	h.waitFor("needs_login", func() bool { return backendIs(h, protocol.BackendNeedsLogin) })
	if !inOrder(h, 0, "core:start 1000 setup", "node:logout") || !h.core.last().alive() || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("setup core first, it keeps running, phase off: %v", h.rec.list())
	}
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) {
		t.Fatalf("the list stays: %+v", got)
	}
	time.Sleep(20 * time.Millisecond)
	if h.store.saved().LoggedIn {
		t.Fatal("a look at the node from before the logout must not set it again")
	}
}

// The errors of the API service carry the core's command line and its standard error, which may name
// peers and addresses in the tailnet: the log gets them masked (<ip>), never as they are. A failed
// logout changes nothing.
func TestNodeErrorsAreMasked(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	h.core.node.set(func(n *fakeNode) {
		n.err = errors.New("sing-box.exe api tailscale status: exit status 1: dial tcp 100.64.0.7:41641: connection refused")
	})
	if err := call(h, protocol.MethodLogout, nil); err != nil {
		t.Fatal(err)
	}
	logged := func(prefix string) string {
		for _, ev := range h.rec.list() {
			if strings.HasPrefix(ev, prefix) {
				return ev
			}
		}
		return ""
	}
	h.waitFor("both errors logged", func() bool {
		return logged("log:setup mode: node: ") != "" && logged("log:tailscale.logout failed: ") != ""
	})
	for _, prefix := range []string{"log:setup mode: node: ", "log:tailscale.logout failed: "} {
		if ev := logged(prefix); !strings.Contains(ev, "dial tcp <ip>:41641") {
			t.Errorf("masked: %q", ev)
		}
	}
	for _, ev := range h.rec.list() {
		if strings.Contains(ev, "100.64.0.7") {
			t.Fatalf("an address in the log: %q", ev)
		}
	}
	if !h.store.saved().LoggedIn {
		t.Fatal("a failed logout changes nothing")
	}
}

// helper.shutdown in off (spec 7): the setup core is stopped as in step 2 of 5.4 (the graceful
// signal, its end waited for) before the process ends.
func TestShutdownStopsSetup(t *testing.T) {
	h, setup := setupRunning(t)
	orig := h.e.d.OnExit
	h.e.d.OnExit = func() {
		h.rec.add("helper:exit")
		orig()
	}
	shutdown(t, h)
	stop, exit := h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)), h.rec.index("helper:exit")
	if setup.alive() || stop < 0 || exit < stop {
		t.Fatalf("the setup core ends before the helper: %v", h.rec.list())
	}
}

// The end of the Windows session (5.4): the setup core is ended at once, without waiting.
func TestSessionEndKillsSetup(t *testing.T) {
	h, setup := setupRunning(t)
	h.e.SessionEnding()
	if setup.alive() || h.rec.index(fmt.Sprintf("core:kill %d", setup.id.PID)) < 0 || h.rec.index("core:interrupt") >= 0 {
		t.Fatalf("killed without the graceful signal: %v", h.rec.list())
	}
	h.waitFor("backend stopped", func() bool { return backendIs(h, protocol.BackendStopped) })
}

// 5.7: the identity of the setup core goes into state.json like the main core's. A helper that ended
// with the setup core running (a crash) leaves it behind; the next one stops it at its start-up, in
// phase stopping, and ends in off.
func TestLeftoverSetupCoreIsStopped(t *testing.T) {
	h, setup := setupRunning(t)
	saved := h.store.saved()
	if saved.Core == nil || *saved.Core != setup.id {
		t.Fatalf("the identity of the setup core in state.json: %+v", saved.Core)
	}
	next := newHarness(t)
	next.store.state = saved
	next.core.leftover = setup
	next.e.Start()
	next.waitPhase(protocol.PhaseOff)
	if setup.alive() || strings.Join(next.phases(), ",") != "stopping,off" || next.store.saved().Core != nil {
		t.Fatalf("the leftover setup core stopped in stopping: phases %v, events %v", next.phases(), h.rec.list())
	}
	if h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)) < 0 {
		t.Fatalf("stopped with the graceful signal: %v", h.rec.list())
	}
	h.waitFor("the first helper sees its setup core gone", func() bool { return backendIs(h, protocol.BackendStopped) })
}

// TestLoginPhaseTable walks the cells of 5.1 for tailscale.login and tailscale.logout in off, error,
// starting, on, blocked and stopping: in off and error the setup mode starts (login) or the node logs
// out (logout) and the phase stays; starting and stopping are busy; on and blocked are wrong_phase.
func TestLoginPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	phases := []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
		protocol.PhaseBlocked, protocol.PhaseStopping}
	rows := []struct {
		name   string
		method string
		want   []string // off, error, starting, on, blocked, stopping
		act    func(t *testing.T, h *harness)
	}{
		{"login", protocol.MethodLogin, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("the setup core's node read", func() bool {
					return h.rec.index("core:start 1000 setup") >= 0 && backendIs(h, protocol.BackendRunning)
				})
			}},
		{"logout", protocol.MethodLogout, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
			}},
	}
	for _, row := range rows {
		for i, phase := range phases {
			t.Run(row.name+"/"+phase, func(t *testing.T) {
				h, release := inPhase(t, phase)
				defer release()
				t.Cleanup(func() { shutdown(t, h) })
				before := h.rec.list()
				err := call(h, row.method, nil)
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h)
					if got := h.e.State().Phase; got != phase {
						t.Fatalf("the phase stays %s, got %s", phase, got)
					}
				default:
					wantCode(t, err, want)
					time.Sleep(20 * time.Millisecond)
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phase {
						t.Fatalf("nothing happens: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				}
			})
		}
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'Test(LoginStartsSetupMode|LoginTwiceKeepsOneSetupCore|SetupModeSurvivesReconnects|SetupStopsFiveMinutesAfterLastWindow|EnableStopsSetupFirst|LoggedInIsSaved|NeedsLoginClearsLoggedIn|LogoutClearsLoggedIn|NodeErrorsAreMasked|ShutdownStopsSetup|SessionEndKillsSetup|LeftoverSetupCoreIsStopped|LoginPhaseTable)$' -count=1
```

Ожидается: тринадцать провалов, около 40 секунд (восемь тестов ждут ядро режима настройки по 5 секунд). Методы отвечают `internal`, окно ничего не запускает. В `TestLoginPhaseTable` проваливаются все двенадцать ячеек (показаны первые три; строки `teardown.go:47` в ячейках `blocked` пишет выключение при завершении теста):

```text
--- FAIL: TestLoginStartsSetupMode (0.02s)
    connector_test.go:103: internal: tailscale.login is not available in this build (Plan 1B)
--- FAIL: TestLoginTwiceKeepsOneSetupCore (0.00s)
    connector_test.go:140: internal: tailscale.login is not available in this build (Plan 1B)
--- FAIL: TestSetupModeSurvivesReconnects (5.00s)
    connector_test.go:163: timed out waiting for the check of the setup config
--- FAIL: TestSetupStopsFiveMinutesAfterLastWindow (5.00s)
    connector_test.go:189: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestEnableStopsSetupFirst (5.00s)
    connector_test.go:209: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestLoggedInIsSaved (0.00s)
    connector_test.go:249: internal: tailscale.login is not available in this build (Plan 1B)
--- FAIL: TestNeedsLoginClearsLoggedIn (5.00s)
    connector_test.go:283: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestLogoutClearsLoggedIn (0.00s)
    connector_test.go:310: internal: tailscale.logout is not available in this build (Plan 1B)
--- FAIL: TestNodeErrorsAreMasked (5.00s)
    connector_test.go:340: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestShutdownStopsSetup (5.00s)
    connector_test.go:376: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestSessionEndKillsSetup (5.00s)
    connector_test.go:391: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestLeftoverSetupCoreIsStopped (5.00s)
    connector_test.go:403: timed out waiting for the setup core's node read; phases []; events []
--- FAIL: TestLoginPhaseTable (0.05s)
    --- FAIL: TestLoginPhaseTable/login/off (0.00s)
        connector_test.go:457: answer internal: tailscale.login is not available in this build (Plan 1B)
    --- FAIL: TestLoginPhaseTable/login/error (0.00s)
        connector_test.go:457: answer internal: tailscale.login is not available in this build (Plan 1B)
    --- FAIL: TestLoginPhaseTable/login/starting (0.01s)
        connector_test.go:464: want busy, got internal: tailscale.login is not available in this build (Plan 1B)
FAIL
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 3: Режим настройки, вход и выход в автомате**

Файл `helper/internal/engine/connector.go`, целиком:

```go
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"splitlane/helper/internal/protocol"
)

// Times of the connector (spec 7).
const (
	nodeEvery = 2 * time.Second // the node of the setup core is read so often
	setupIdle = 5 * time.Minute // the setup core runs so long after the last window went
)

// setupRun is one run of the setup mode (spec 7): the core with the embedded node, its local DNS and
// its API service, without an adapter. Its goroutine (runSetup) owns the core from its start to its end.
type setupRun struct {
	cancel context.CancelFunc
	done   chan struct{} // closed when the run is over and its core has ended
	ready  chan struct{} // closed when the API service has answered once
	node   NodeAPI       // the API service of this run; set before ready is closed
	proc   CoreProc      // guarded by Engine.mu; nil until the core has started
	ending bool          // guarded by Engine.mu: the run is stopping; a login or a window now asks for the next one
	again  bool          // guarded by Engine.mu: the next run starts as soon as this one is over
	gen    int           // guarded by Engine.mu: a logout moves it on, so a look at the node from before it is dropped
}

// setupPhaseLocked is the phase check of tailscale.login and tailscale.logout (5.1): nil in off and
// error; busy in starting and stopping and after helper.shutdown; wrong_phase in on, degraded and
// blocked. e.mu must be held.
func (e *Engine) setupPhaseLocked() *protocol.Error {
	switch phase := e.st.Phase; {
	case e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return busy()
	case phase != protocol.PhaseOff && phase != protocol.PhaseError:
		return wrongPhase(phase)
	}
	return nil
}

// cmdLogin is tailscale.login in off and error: the setup mode starts, the login link comes in the
// state (spec 7). While it runs nothing new starts: its link stays (Review Focus 5). The answer comes
// at once, the core starts in the background.
func (e *Engine) cmdLogin(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.setupPhaseLocked(); err != nil {
		return err
	}
	e.startSetupLocked()
	return nil
}

// cmdLogout is tailscale.logout in off and error (spec 7): the answer comes at once, the logout runs in
// the background (logout).
func (e *Engine) cmdLogout(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.setupPhaseLocked(); err != nil {
		return err
	}
	go e.logout()
	return nil
}

// windowCame: a window connected while none was (SetClients). With "logged in" saved the setup mode
// starts, to show the state of the node and its exit nodes (spec 7).
func (e *Engine) windowCame() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.clients.Load() > 0 && e.saved.LoggedIn {
		e.startSetupLocked()
	}
}

// startSetupLocked starts the setup mode unless it runs (spec 7): only in off and error, never after
// helper.shutdown or the end of the session; e.mu must be held. A run that is ending is followed by a
// new one once its core has ended.
func (e *Engine) startSetupLocked() {
	if e.exiting || e.ending || (e.st.Phase != protocol.PhaseOff && e.st.Phase != protocol.PhaseError) {
		return
	}
	if s := e.setup; s != nil {
		if s.ending {
			s.again = true
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &setupRun{cancel: cancel, done: make(chan struct{}), ready: make(chan struct{})}
	e.setup = s
	go e.runSetup(ctx, s)
}

// stopSetup ends the setup mode and returns once its core has ended (step 2 of 5.4): the first action
// of an enable (spec 7; 5.3 step 1 does not count the own setup core as another core, it is gone by
// then) and of helper.shutdown. A logout under way is finished first. A starting run is given up: its
// core, if it has started, is stopped like a running one.
func (e *Engine) stopSetup() {
	e.connector.Lock()
	defer e.connector.Unlock()
	for {
		e.mu.Lock()
		s := e.setup
		if s != nil {
			s.again = false
		}
		e.mu.Unlock()
		if s == nil {
			return
		}
		s.cancel()
		<-s.done
	}
}

// setupMayStart waits for an operation that is still finishing (the setup core never runs beside the
// main core: they share the node's state folder) and tells whether the setup core may start: phase off
// or error, no helper.shutdown, no end of the session, the run not cancelled. An operation that enables
// keeps the phase away from off and error, and its first action cancels this run (stopSetup).
func (e *Engine) setupMayStart(ctx context.Context) bool {
	for {
		e.mu.Lock()
		o := e.op
		ok := !e.exiting && !e.ending && (e.st.Phase == protocol.PhaseOff || e.st.Phase == protocol.PhaseError)
		e.mu.Unlock()
		if !ok || o == nil {
			return ok && ctx.Err() == nil
		}
		select {
		case <-o.done:
		case <-ctx.Done():
			return false
		}
	}
}

// runSetup is one run of the setup mode: the core as in steps 3 and 4 of 5.3 without the adapter (its
// own API port and secret, the config checked, the identity in state.json for 5.7, the start line
// within 30 s), then a look at the node every 2 s until the run is cancelled (stopSetup, the end of the
// session), its core exits, or no window has been connected for 5 minutes. The core is stopped as in
// step 2 of 5.4 before the run is over (killed at the end of the session). The phase never changes.
func (e *Engine) runSetup(ctx context.Context, s *setupRun) {
	var proc CoreProc
	defer func() {
		e.mu.Lock()
		s.ending = true
		kill := e.ending
		e.mu.Unlock()
		switch {
		case proc == nil:
		case kill:
			_ = proc.Kill() // the end of the session: no wait (5.4)
		default:
			e.stopProc(proc)
		}
		e.mu.Lock()
		if e.setup == s {
			e.setup = nil
		}
		if e.nodeBackend != protocol.BackendStopped || e.nodeAuthURL != "" {
			e.nodeBackend, e.nodeAuthURL = protocol.BackendStopped, ""
			e.publish()
		}
		if s.again {
			e.startSetupLocked()
		}
		e.mu.Unlock()
		s.cancel()
		close(s.done)
	}()
	if !e.setupMayStart(ctx) {
		return
	}
	p, err := e.d.Host.FreePorts(1)
	if err != nil {
		e.d.Log("setup mode: no free port: %v", err)
		return
	}
	secret := e.d.Host.Secret()
	e.mu.Lock()
	host := e.hostname()
	e.mu.Unlock()
	config, err := e.d.Core.BuildSetupConfig(SetupOptions{Hostname: host, APIPort: p[0], APISecret: secret, LogLevel: e.d.Settings.CoreLogLevel})
	if err != nil {
		e.d.Log("setup mode: config: %v", err)
		return
	}
	path, err := e.d.Store.WriteConfig(config)
	if err != nil {
		e.d.Log("setup mode: config not written: %v", err)
		return
	}
	if err := e.d.Core.Check(ctx, path); err != nil {
		if ctx.Err() == nil {
			e.d.Log("setup mode: config refused: %v", err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	proc, err = e.d.Core.Start(path)
	if err != nil {
		proc = nil
		e.d.Log("setup mode: the core did not start: %v", err)
		return
	}
	e.mu.Lock()
	s.proc = proc
	id := proc.Identity()
	e.saved.Core = &id
	e.save()
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(startWait)
	for !e.d.Core.Events(0).Started {
		switch {
		case exited(proc):
			e.d.Log("setup mode: the core exited before its start line")
			return
		case !e.d.Clock.Now().Before(deadline):
			e.d.Log("setup mode: no start line within %s", startWait)
			return
		}
		if !e.sleep(ctx, 250*time.Millisecond) {
			return
		}
	}
	e.d.Log("setup mode: core started")
	s.node = e.d.Core.Node(p[0], secret)
	since, ready, lastErr := e.d.Clock.Now(), false, ""
	for {
		if err := e.readNode(ctx, s); err != nil {
			// The errors carry the core's command line and its stderr: peers and tailnet addresses.
			if text := protocol.MaskAddresses(err.Error()); text != lastErr && ctx.Err() == nil {
				lastErr = text
				e.d.Log("setup mode: node: %s", text)
			}
		} else {
			lastErr = ""
			if !ready {
				ready = true
				close(s.ready)
			}
		}
		switch {
		case exited(proc):
			e.d.Log("setup mode: the core exited")
			return
		case e.windowless(since):
			e.d.Log("setup mode: no window for %s; stopping it", setupIdle)
			return
		}
		if !e.sleep(ctx, nodeEvery) {
			return
		}
	}
}

// windowless: no window is connected, and the last one went (or, when none went during this run, the
// run started) 5 minutes ago or more (spec 7).
func (e *Engine) windowless(since time.Time) bool {
	if e.clients.Load() > 0 {
		return false
	}
	if gone := time.Unix(0, e.lastGone.Load()); gone.After(since) {
		since = gone
	}
	return !e.d.Clock.Now().Before(since.Add(setupIdle))
}

// readNode is one look at the node of the setup core (spec 7, 5.2): tailscale.backend and authUrl from
// its state. "Logged in" is no login link, an address in the tailnet and the backend starting or
// running (not running only: with the system exit node on, the node of the setup mode stays in
// starting); then the flag and the exit nodes go into state.json, and the state shows that list while
// no core runs. needs_login clears the flag; needs_approval leaves it as it is. A change is published,
// the backend and the list in one state. A look that a logout overtook is dropped.
func (e *Engine) readNode(ctx context.Context, s *setupRun) error {
	e.mu.Lock()
	gen := s.gen
	e.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	st, err := s.node.State(cctx)
	if err != nil {
		return err
	}
	loggedIn := st.AuthURL == "" && st.HasAddress && (st.Backend == protocol.BackendStarting || st.Backend == protocol.BackendRunning)
	var nodes []protocol.ExitNode
	var nodesErr error
	if loggedIn {
		nodes, nodesErr = s.node.ExitNodes(cctx)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if s.gen != gen {
		return nil
	}
	dirty := e.nodeBackend != st.Backend || e.nodeAuthURL != st.AuthURL
	e.nodeBackend, e.nodeAuthURL = st.Backend, st.AuthURL
	save := false
	switch {
	case loggedIn && !e.saved.LoggedIn:
		e.saved.LoggedIn, save = true, true
	case st.Backend == protocol.BackendNeedsLogin && e.saved.LoggedIn:
		e.saved.LoggedIn, save = false, true
	}
	if loggedIn && nodesErr == nil && !slices.Equal(nodes, e.saved.ExitNodes) {
		e.saved.ExitNodes, save = nodes, true
	}
	if save {
		e.save()
		dirty = true
	}
	if dirty {
		e.publish()
	}
	return nodesErr
}

// logout is tailscale.logout after its answer (spec 7): the setup mode is started unless it runs, its
// API service is waited for (at most 30 s), the node logs out through the core and "logged in" is
// cleared; the list of exit nodes stays, and the setup core keeps running with the node's new login
// link. A failure is logged and the state published: the method has answered. Logouts run one at a
// time, and an enable or helper.shutdown that comes meanwhile waits for this one (stopSetup); an enable
// that began ends the wait at once.
func (e *Engine) logout() {
	e.connector.Lock()
	defer e.connector.Unlock()
	e.mu.Lock()
	e.startSetupLocked()
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(startWait)
	var s *setupRun
	for s == nil {
		e.mu.Lock()
		cur, phaseErr := e.setup, e.setupPhaseLocked()
		ending := cur != nil && cur.ending
		if ending {
			e.startSetupLocked() // its core is being stopped: the next run follows it
		}
		e.mu.Unlock()
		if cur == nil || phaseErr != nil {
			e.logoutFailed("the setup mode does not run")
			return
		}
		select {
		case <-cur.ready:
			if !ending {
				s = cur
				continue
			}
		default:
		}
		if !e.d.Clock.Now().Before(deadline) {
			e.logoutFailed(fmt.Sprintf("the API service of the setup core did not answer within %s", startWait))
			return
		}
		e.sleep(context.Background(), 250*time.Millisecond)
	}
	e.mu.Lock()
	s.gen++
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	err := s.node.Logout(ctx)
	cancel()
	e.mu.Lock()
	defer e.mu.Unlock()
	s.gen++
	if err != nil {
		e.d.Log("tailscale.logout failed: %s", protocol.MaskAddresses(err.Error()))
		e.publish()
		return
	}
	e.d.Log("tailscale.logout: the node logged out")
	e.saved.LoggedIn = false
	e.save()
	e.publish()
}

func (e *Engine) logoutFailed(why string) {
	e.d.Log("tailscale.logout failed: %s", why)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.publish()
}
```

Поля коннектора в автомате, `backend` и `authUrl` от коннектора в `publish`, `SetClients` без блокировок автомата.

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
	ending       bool         // the Windows session ends (5.4)
	exiting      bool         // helper.shutdown was accepted
	skip         bool         // "disable without restore" (5.4)
	wake         chan struct{}
}
```

Заменить на:

```go
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
	lastGone     atomic.Int64 // Clock time (UnixNano) at which the last window went (spec 7)
	ending       bool         // the Windows session ends (5.4)
	exiting      bool         // helper.shutdown was accepted
	skip         bool         // "disable without restore" (5.4)
	wake         chan struct{}

	// The connector (spec 7, connector.go).
	setup       *setupRun  // the setup mode now; nil when it does not run
	nodeBackend string     // tailscale.backend as the setup core's node says; stopped when it does not run
	nodeAuthURL string     // the login link of the setup core's node, "" when there is none
	connector   sync.Mutex // logouts one at a time; stopSetup waits for the one under way
}
```

Найти:

```go
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults()}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
		Apps:        []protocol.AppRule{},
		Tailscale:   protocol.Tailscale{Backend: "stopped", ExitNodes: []protocol.ExitNode{}},
	}
```

Заменить на:

```go
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults(), nodeBackend: protocol.BackendStopped}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
		Apps:        []protocol.AppRule{},
		Tailscale:   protocol.Tailscale{Backend: protocol.BackendStopped, ExitNodes: []protocol.ExitNode{}},
	}
```

Найти:

```go
// SetClients tells how many windows are connected (5.6: the system client is polled for them).
func (e *Engine) SetClients(n int) { e.clients.Store(int32(n)) }
```

Заменить на:

```go
// SetClients tells how many windows are connected (5.6: the system client is polled for them; spec 7:
// the first window that comes starts the setup mode when "logged in" is saved, and the setup mode
// stops 5 minutes after the last one went). The pipe calls it under its own lock, which a published
// state takes after e.mu, so it takes no lock of the engine: the start runs on its own goroutine.
func (e *Engine) SetClients(n int) {
	old := e.clients.Swap(int32(n))
	switch {
	case n == 0 && old > 0:
		e.lastGone.Store(e.d.Clock.Now().UnixNano())
	case n > 0 && old == 0:
		go e.windowCame()
	}
}
```

Найти:

```go
// publish sends the state to the clients; e.mu must be held.
func (e *Engine) publish() {
	e.syncSaved()
	if e.run != nil && live(e.st.Phase) {
		e.st.Tailscale.Backend = "running"
	} else {
		e.st.Tailscale.Backend = "stopped"
	}
	e.d.OnState(e.st.Clone())
}
```

Заменить на:

```go
// publish sends the state to the clients; e.mu must be held. tailscale.backend and authUrl come from
// the connector (spec 7): the node of the setup core, stopped when no core runs (5.2); while the main
// core runs in a live phase, running.
func (e *Engine) publish() {
	e.syncSaved()
	ts := &e.st.Tailscale
	ts.Backend, ts.AuthURL = e.nodeBackend, nil
	if e.run != nil && live(e.st.Phase) {
		ts.Backend = protocol.BackendRunning
	} else if e.nodeAuthURL != "" {
		u := e.nodeAuthURL
		ts.AuthURL = &u
	}
	e.d.OnState(e.st.Clone())
}
```

Методы через очередь команд, остановка режима перед `OnExit`.

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
	case protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport:
```

Заменить на:

```go
	case protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport:
```

Найти:

```go
		keep, err = e.cmdClearSystemExitNode(params)
	case protocol.MethodShutdown:
```

Заменить на:

```go
		keep, err = e.cmdClearSystemExitNode(params)
	case protocol.MethodLogin:
		err = e.cmdLogin(params)
	case protocol.MethodLogout:
		err = e.cmdLogout(params)
	case protocol.MethodShutdown:
```

Найти:

```go
// exitWhenDone waits for the operations to end, disables as long as a live phase is left, then calls
// OnExit. The loop closes the window in which an enable or restart has published a live phase but
// startOp's cleanup has not yet cleared e.op: beginDisableLocked refuses there, so shutdown would
// otherwise end the process with the core running and the system exit node off. exiting keeps every
// other command and the watcher out, so nothing but this loop starts an operation now.
```

Заменить на:

```go
// exitWhenDone waits for the operations to end, disables as long as a live phase is left, stops the
// setup mode (spec 7), then calls OnExit. The loop closes the window in which an enable or restart has
// published a live phase but startOp's cleanup has not yet cleared e.op: beginDisableLocked refuses
// there, so shutdown would otherwise end the process with the core running and the system exit node
// off. exiting keeps every other command, the watcher and a new setup mode out, so nothing but this
// loop starts an operation now.
```

Найти:

```go
		if !again {
			break
		}
	}
	e.d.OnExit()
}
```

Заменить на:

```go
		if !again {
			break
		}
	}
	e.stopSetup()
	e.d.OnExit()
}
```

Первое действие включения — остановка режима настройки. `enable` вызывают и включение по команде, и перезапуски 5.5, и включение при старте (5.7).

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
// enable is steps 1–8 of 5.3; the order was proven on the PC (appendix A) and does not change.
func (e *Engine) enable(ctx context.Context, unattended bool) *protocol.Reason {
	if why := e.preflight(ctx, unattended); why != nil {
```

Заменить на:

```go
// enable is steps 1–8 of 5.3; the order was proven on the PC (appendix A) and does not change.
// Its first action ends the setup mode (spec 7): the main core shares the node's state folder with
// it, and step 1 must not meet it as another core.
func (e *Engine) enable(ctx context.Context, unattended bool) *protocol.Reason {
	e.stopSetup()
	if why := e.preflight(ctx, unattended); why != nil {
```

Старт 5.7 (только комментарий: код тот же) и конец сеанса.

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
// left from an earlier run is stopped; a foreign sing-box.exe is never touched. If "was on" is
// still set, the previous helper ended abnormally: enable again. A routing.disable in phase
// stopping without restore_pending clears it (cmdDisable, 5.1): the start-up then ends in off.
```

Заменить на:

```go
// left from an earlier run is stopped, the main core or the setup mode's (spec 7: both record their
// identity in state.json); a foreign sing-box.exe is never touched. If "was on" is still set, the
// previous helper ended abnormally: enable again. A routing.disable in phase stopping without
// restore_pending clears it (cmdDisable, 5.1): the start-up then ends in off.
```

Найти:

```go
// overwritten (5.7); the next start finds the exit node on and deletes the journal.
// It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o := e.journal, e.run, e.op
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
	}
	e.mu.Unlock()
	if o != nil {
		o.cancel()
	}
```

Заменить на:

```go
// overwritten (5.7); the next start finds the exit node on and deletes the journal.
// The core of the setup mode (spec 7) is ended the same way, without waiting.
// It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o, s := e.journal, e.run, e.op, e.setup
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
	}
	var setupProc CoreProc
	if s != nil {
		s.again, setupProc = false, s.proc
	}
	e.mu.Unlock()
	if o != nil {
		o.cancel()
	}
	if s != nil {
		s.cancel()
	}
	if setupProc != nil {
		_ = setupProc.Kill()
	}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/engine/; go test ./internal/engine/ -run 'Test(LoginStartsSetupMode|LoginTwiceKeepsOneSetupCore|SetupModeSurvivesReconnects|SetupStopsFiveMinutesAfterLastWindow|EnableStopsSetupFirst|LoggedInIsSaved|NeedsLoginClearsLoggedIn|LogoutClearsLoggedIn|NodeErrorsAreMasked|ShutdownStopsSetup|SessionEndKillsSetup|LeftoverSetupCoreIsStopped|LoginPhaseTable)$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./internal/engine/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestLoginStartsSetupMode`, `--- PASS: TestLoginTwiceKeepsOneSetupCore`, `--- PASS: TestSetupModeSurvivesReconnects`, `--- PASS: TestSetupStopsFiveMinutesAfterLastWindow`, `--- PASS: TestEnableStopsSetupFirst`, `--- PASS: TestLoggedInIsSaved`, `--- PASS: TestNeedsLoginClearsLoggedIn`, `--- PASS: TestLogoutClearsLoggedIn`, `--- PASS: TestNodeErrorsAreMasked`, `--- PASS: TestShutdownStopsSetup`, `--- PASS: TestSessionEndKillsSetup`, `--- PASS: TestLeftoverSetupCoreIsStopped`, `--- PASS: TestLoginPhaseTable`, затем `ok  	splitlane/helper/internal/engine` для выбранных тестов и для пакета целиком (`TestPhaseTable`, `TestPreflightRefusals`, `TestUnknownAndLaterMethods`, `TestPollWhileOffNeedsAWindow`, `TestLastWindowGoneKeepsRouting`, тесты старта и конца сеанса проходят).

- [ ] **Step 5: Написать тест `slctl login`**

Подмена служебного процесса после `tailscale.login` шлёт состояния узла по одному: ссылка в двух состояниях подряд, затем `needs_approval` (не конец), затем `starting` без ссылки.

Изменить `helper/cmd/slctl/main_test.go`:

Найти:

```go
	silent   map[string]bool // methods that get no answer until release is closed
	release  chan struct{}   // lets the silent methods return
	stateErr *protocol.Error // state.get is rejected with it
}
```

Заменить на:

```go
	silent   map[string]bool      // methods that get no answer until release is closed
	release  chan struct{}        // lets the silent methods return
	stateErr *protocol.Error      // state.get is rejected with it
	login    []protocol.Tailscale // the node's states after tailscale.login, one state event each, 30 ms apart
}
```

Найти:

```go
	case protocol.MethodLanesSet:
		return nil, &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected",
			Rules: []protocol.RuleRejection{{ID: "x", Reason: protocol.RejectProtected}}}
	}
```

Заменить на:

```go
	case protocol.MethodLanesSet:
		return nil, &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected",
			Rules: []protocol.RuleRejection{{ID: "x", Reason: protocol.RejectProtected}}}
	case protocol.MethodLogin:
		go func() {
			for _, ts := range f.login {
				time.Sleep(30 * time.Millisecond)
				f.mu.Lock()
				f.st.Tailscale = ts
				s := f.st
				f.mu.Unlock()
				f.srv.Broadcast(s)
			}
		}()
		return struct{}{}, nil
	}
```

Найти:

```go
func TestWaitForAPhase(t *testing.T) {
```

Заменить на:

```go
// slctl login (spec 7): tailscale.login, then the state events. Each change of the backend is printed;
// the login link once, though several states carry it; needs_approval is not the end. The command ends
// with exit code 0 when the node is logged in: backend starting or running without a link.
func TestLoginPrintsTheLinkOnce(t *testing.T) {
	link := "https://login.example/a/1f2e3d"
	name := testPipeName()
	srv, err := serveHelper(name, &fakeHelper{
		st: protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{}, Tailscale: protocol.Tailscale{Backend: protocol.BackendStopped}},
		login: []protocol.Tailscale{
			{Backend: protocol.BackendNeedsLogin, AuthURL: &link},
			{Backend: protocol.BackendNeedsLogin, AuthURL: &link, DeviceName: "pc-splitlane"},
			{Backend: protocol.BackendNeedsApproval},
			{Backend: protocol.BackendStarting},
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	code := run([]string{"-pipe", name, "login", "-timeout", "10s"}, &out)
	text := out.String()
	if code != exitOK || strings.Count(text, link) != 1 || !strings.Contains(text, "SLCTL: open this link to log in: "+link) {
		t.Fatalf("exit %d, the link %d times:\n%s", code, strings.Count(text, link), text)
	}
	for _, b := range []string{"needs_login", "needs_approval", "starting"} {
		if n := strings.Count(text, "SLCTL: backend="+b+"\n"); n != 1 {
			t.Errorf("backend=%s printed %d times", b, n)
		}
	}
	if t.Failed() {
		t.Fatalf("output:\n%s", text)
	}
}

func TestWaitForAPhase(t *testing.T) {
```

- [ ] **Step 6: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestLoginPrintsTheLinkOnce' -count=1
```

Ожидается: команды `login` нет, `slctl` печатает справку и выходит с кодом 2 (справка сокращена):

```text
--- FAIL: TestLoginPrintsTheLinkOnce (0.00s)
    main_test.go:369: exit 2, the link 0 times:
        usage: slctl [-pipe NAME] [-timeout 150s] [-show-addresses] [-url URL] COMMAND [ARGS]
          state                          print the state
FAIL
FAIL	splitlane/helper/cmd/slctl
```

- [ ] **Step 7: Команды `login` и `logout`**

Изменить `helper/cmd/slctl/main.go`:

Найти:

```go
  clear-system-exit-node         systemTailscale.clearExitNode
  shutdown                       helper.shutdown, wait until the pipe closes
```

Заменить на:

```go
  clear-system-exit-node         systemTailscale.clearExitNode
  login [-timeout 300s]          tailscale.login, print the login link once, wait until logged in
  logout [-timeout 60s]          tailscale.logout, wait for backend needs_login
  shutdown                       helper.shutdown, wait until the pipe closes
```

Найти:

```go
	"shutdown", "call", "wait", "watch", "expect-lanes"}
```

Заменить на:

```go
	"login", "logout", "shutdown", "call", "wait", "watch", "expect-lanes"}
```

Найти:

```go
	state   *protocol.State
	timeout time.Time
	lastErr error
}
```

Заменить на:

```go
	state   *protocol.State
	timeout time.Time
	lastErr error
	onState func(protocol.State) // sees every state that comes, by event or by state.get (login, logout)
}
```

Найти:

```go
			c.print("EVENT state %s", summary(s))
		} else {
```

Заменить на:

```go
			c.print("EVENT state %s", summary(s))
			if c.onState != nil {
				c.onState(s)
			}
		} else {
```

Найти:

```go
	c.state = &s
	return s, nil
}
```

Заменить на:

```go
	c.state = &s
	if c.onState != nil {
		c.onState(s)
	}
	return s, nil
}
```

Найти:

```go
	case "clear-system-exit-node":
		return simple(protocol.MethodClearSystemExitNode, nil)
```

Заменить на:

```go
	case "clear-system-exit-node":
		return simple(protocol.MethodClearSystemExitNode, nil)
	case "login", "logout":
		return nodeCommand(c, cmd, args)
```

Найти:

```go
// laneName says which lane an address belongs to.
```

Заменить на:

```go
// nodeTimeouts: login waits for the user in the browser; logout for the setup core and the core's call.
var nodeTimeouts = map[string]time.Duration{"login": 300 * time.Second, "logout": 60 * time.Second}

// nodeCommand is login and logout (spec 7): the method, then the states until the node is logged in
// (login: backend starting or running without a login link) or out (logout: backend needs_login). Each
// change of the backend is printed; the login link once, to this console only: it never goes into a
// report or a log.
func nodeCommand(c *client, cmd string, args []string) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(c.out)
	wait := fs.Duration("timeout", nodeTimeouts[cmd], "how long to wait for the node")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return exitUsage
	}
	c.timeout = time.Now().Add(*wait)
	method, done := protocol.MethodLogin, loggedIn
	if cmd == "logout" {
		method = protocol.MethodLogout
		done = func(s protocol.State) bool { return s.Tailscale.Backend == protocol.BackendNeedsLogin }
	}
	backend, linked := "", false
	c.onState = func(s protocol.State) {
		if b := s.Tailscale.Backend; b != backend {
			backend = b
			c.print("SLCTL: backend=%s", b)
		}
		if cmd == "login" && !linked && s.Tailscale.AuthURL != nil {
			linked = true
			c.print("SLCTL: open this link to log in: %s", *s.Tailscale.AuthURL)
		}
	}
	_, perr, err := c.call(method, nil)
	switch {
	case err != nil:
		return c.fail(err)
	case perr != nil:
		return exitFailed // call printed its ERROR line
	}
	s, err := c.waitFor(done)
	return c.finish(s, err, done)
}

// loggedIn is the end of slctl login (spec 7): the backend starting or running, no login link.
func loggedIn(s protocol.State) bool {
	b := s.Tailscale.Backend
	return (b == protocol.BackendStarting || b == protocol.BackendRunning) && s.Tailscale.AuthURL == nil
}

// laneName says which lane an address belongs to.
```

- [ ] **Step 8: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./cmd/slctl/; go test ./cmd/slctl/ -run 'TestLoginPrintsTheLinkOnce' -count=1 -v; go test ./cmd/slctl/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestLoginPrintsTheLinkOnce`, затем дважды `ok  	splitlane/helper/cmd/slctl` (остальные тесты `slctl` проходят без изменений).

- [ ] **Step 9: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё одно `ok  	splitlane/helper/internal/engine` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 10: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine helper/cmd/slctl
git -C C:\code\router commit -m "feat(helper): the setup mode of the core, tailscale.login and tailscale.logout; slctl login and logout" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 9: Коннектор при работе: истечение входа, вид соединения, список exit node

Раздел 7 спецификации («Истечение входа», «Выбор exit node»), 5.2 (`tailscale.path`, `exitNodes` с признаком `online`), ячейка таблицы 5.1 «`tailscale.login` в `on`, `degraded`, `blocked`» и 5.5. Задача 8 сделала режим настройки и данные узла, пока рабочее ядро не запущено; пока оно работает, состояние показывало `backend = running`, `path = null` и сохранённый список. В конфиге рабочего ядра служба API уже есть (порт `Ports.API`, секрет `APISecret`, задача 3): теперь секрет запоминается для прогона, и при запуске ядра создаётся `Core.Node(port, secret)` — служба API этого прогона (`coreRun.node`).

Раз в 10 секунд (`checkNode`, свой таймер в `Run` рядом с проверками 5.5), пока рабочее ядро работает в `on`, `degraded` или `blocked` и операции нет, служебный процесс читает `NodeAPI.State`: `tailscale.backend` берётся из узла. `needs_login` — истечение входа (раздел 7): фаза `degraded` с причиной `login_expired`, `backend = expired`, ссылки для входа в состоянии нет (её даёт режим настройки), признак «вход выполнен» снимается в `state.json`. Ядро продолжает работать: программы дорожки VPN остаются без связи и напрямую не уходят (5.5), имена идут через DNS провайдера, как в любом `degraded` (`vpnDown` вместо `degraded` в проверке 5.5, в `lanes.set` и в `routing.allDirect`). К тому же состоянию ведёт строка журнала ядра о входе (`CoreEvents.NeedLogin`), появившаяся после прошлого взгляда; журнал читается от отметки с запасом 4 КиБ, чтобы не перечитывать его целиком каждые 10 секунд и не потерять строку, которую ядро дописывало в тот момент. На шагах 4–5 включения (5.3) эта строка по-прежнему даёт откат с причиной `login_expired`. Если в фазе `blocked` выполняются оба условия, фаза остаётся `blocked` (5.2), `backend` — `expired`. Пока идёт операция, взгляд пропускается, следующий найдёт то же самое.

Вид соединения: сразу после фазы `on` (в фоне из `finishOnChecked`: операция ping не ждёт) и затем раз в 60 секунд (`refreshNode`) — `NodeAPI.Path(selectedExitNode)`; пустой ответ и ошибка — вид неизвестен, `tailscale.path = null`. Последнее значение и время чтения хранятся в прогоне (`coreRun.path`, `coreRun.pathAt`); задача 10 читает их через `lastPath()` и читает вид заново через `readPath(ctx)`. В состоянии вид показывается, только пока рабочее ядро работает в фазе `on`, `degraded` или `blocked`; иначе `null`. Вместе с видом раз в 60 секунд читается список exit node с признаками «в сети»: при изменении он сохраняется в `state.json` и публикуется, выбранный exit node не меняется.

`tailscale.login` в `degraded` с `backend = expired` начинает выключение 5.4 (`disableThenLocked` — то же `beginDisableLocked` с продолжением: признак «было включено» снимается в его начале, как при `routing.disable`), а когда фаза стала `off`, запускает режим настройки и вход задачи 8; прогон режима настройки дожидается конца операции выключения сам (`setupMayStart`). Ответ приходит сразу. Остальные ячейки `on`, `degraded`, `blocked` — по-прежнему `wrong_phase`, `tailscale.logout` в них — `wrong_phase`.

Каждый вызов службы API — отдельный процесс `sing-box.exe api ...` этого прогона, а `winhost` считает каждый `sing-box.exe` ядром. Поэтому вызовы привязаны к прогону (`nodeCall`): после отметки «ядро останавливается» новый не начинается, `stopCore` (шаг 2 из 5.4, он же в перезапуске и в повторе запуска) отменяет идущие и дожидается их конца до сигнала ядру; конец сеанса Windows только отменяет их, не дожидаясь. Ошибки вызовов несут командную строку и stderr ядра, где бывают имена и адреса узлов: в журнал они попадают только через `protocol.MaskAddresses` (`<ip>`), одна и та же ошибка одного вида вызова — один раз подряд. В режиме настройки список exit node теперь читается при первом взгляде, который нашёл узел вошедшим, и затем не чаще раза в минуту, а не при каждом опросе раз в 2 секунды; тестов задачи 8 это не меняет (`TestLoggedInIsSaved` меняет список вместе со входом).

Строка состояния `slctl` (`summary`: команды `state`, `enable`, `wait`, `disable`, события) заканчивается полями ` backend=<tailscale.backend> path=<tailscale.path или null> exitNodes=<число>`; ссылку для входа она не печатает. Тестов, сравнивающих строку целиком, не было; новый тест закрепляет конец строки. Помощник тестов `enabled(t)` теперь ждёт и первого чтения вида соединения, чтобы фоновое чтение не попадало в подсчёт опубликованных состояний (`TestSetExitNodeBusyWhileAnOpFinishes`).

**Files:**
- Modify: `helper/internal/engine/connector.go`, `helper/internal/engine/engine.go`, `helper/internal/engine/enable.go`, `helper/internal/engine/teardown.go`, `helper/internal/engine/supervise.go`, `helper/internal/engine/commands.go`, `helper/internal/engine/startup.go`, `helper/cmd/slctl/main.go`
- Test: `helper/internal/engine/fakes_test.go`, `helper/internal/engine/connector_test.go`, `helper/internal/engine/enable_test.go`, `helper/cmd/slctl/main_test.go`

**Interfaces:**
- Consumes: `NodeAPI` (`State`, `ExitNodes`, `Path`), `NodeState`, `CoreDriver.Node`, `CoreDriver.LogSize`, `CoreDriver.Events` (`CoreEvents.NeedLogin`), `Ports.API`, `CoreOptions.APISecret`, `Host.Secret`, `protocol.Tailscale.Path`, `protocol.BackendExpired`, `protocol.BackendNeedsLogin`, `protocol.BackendRunning`, `protocol.PathDirect`, `protocol.PathRelay`, `protocol.ReasonLoginExpired`, `protocol.MaskAddresses`; внутри пакета — из задачи 8 `setupRun`, `readNode`, `startSetupLocked`, `setupPhaseLocked`, `nodeEvery`, из плана 1A `beginDisableLocked`, `stopCore`, `refreshLive`, `publish`, `save`, `live`, `every`, `healthEvery`, `callTimeout`; в тестах — `setupRunning`, `shutdown`, `waitClosed`, `backendIs`, `loginURL`, `inPhase`, `cellAct`, `enabled`, `startEnable`, `rounds`, `logEvents`, `inOrder`, `countSince`, `publishedCount`, `call`, `wantCode`; в `slctl` — `summary`, `str`, `serveState`, `testPipeName`, `run`.
- Produces: для задачи 10 — `func (e *Engine) lastPath() (path string, at time.Time)` (последний вид, прочитанный, пока рабочее ядро работает; `""`, если неизвестен или ядра нет) и `func (e *Engine) readPath(ctx context.Context) string` (прочитать сейчас, запомнить, опубликовать изменение). Внутри пакета: `func (e *Engine) checkNode(ctx context.Context)`, `func (e *Engine) refreshNode(ctx context.Context)`, `func (e *Engine) nodeCall(ctx context.Context, r *coreRun, kind string, call func(ctx context.Context, n NodeAPI) error) error`, `func (e *Engine) loginExpired() bool`, `func (e *Engine) vpnDown() bool`, `func (e *Engine) disableThenLocked(then func()) bool`, `const listEvery = time.Minute`, `const logOverlap = 4096`, `var errRunEnded`; поля `coreRun`: `node NodeAPI`, `calls sync.WaitGroup`, `callsEnd context.Context`, `endCalls context.CancelFunc`, `backend string`, `expired bool`, `path string`, `pathAt time.Time`, `logMark int64`, `errs map[string]string`; поле `setupRun.listAt time.Time`; `Run` запускает `checkNode` раз в 10 секунд и `refreshNode` раз в 60 секунд. В тестах: `func newFakeNode(rec *recorder, clock *fakeClock) *fakeNode`, поля `fakeNode.clock`, `pathErr`, `hold`, `looks`, `lists`, `func (n *fakeNode) calls() (looks int, lists []time.Time)`, событие `node:path cancelled`, поля `fakeCore.nodePort`, `fakeCore.nodeSecret`, `const colExpired`, `func expiredOn(t *testing.T) *harness`; `enabled(t)` ждёт первого чтения вида соединения. В `slctl`: строка `summary` заканчивается ` backend=%s path=%s exitNodes=%d`.

- [ ] **Step 1: Написать тест частоты чтения списка в режиме настройки**

Подмена службы API считает вызовы `State` и запоминает время каждого вызова `ExitNodes` по часам теста.

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
type fakeNode struct {
	mu      sync.Mutex
	rec     *recorder
	state   NodeState
	nodes   []protocol.ExitNode
	path    string
	err     error // every call fails with it
	logouts int
}

func newFakeNode(rec *recorder) *fakeNode {
	return &fakeNode{
		rec:   rec,
		state: NodeState{Backend: protocol.BackendRunning, HasAddress: true},
		nodes: []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}},
		path:  protocol.PathDirect,
	}
}

func (n *fakeNode) State(ctx context.Context) (NodeState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return NodeState{}, n.err
	}
	return n.state, nil
}

func (n *fakeNode) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
```

Заменить на:

```go
type fakeNode struct {
	mu      sync.Mutex
	rec     *recorder
	clock   *fakeClock
	state   NodeState
	nodes   []protocol.ExitNode
	path    string
	err     error // every call fails with it
	logouts int
	looks   int         // calls of State
	lists   []time.Time // the clock at each call of ExitNodes
}

func newFakeNode(rec *recorder, clock *fakeClock) *fakeNode {
	return &fakeNode{
		rec:   rec,
		clock: clock,
		state: NodeState{Backend: protocol.BackendRunning, HasAddress: true},
		nodes: []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}},
		path:  protocol.PathDirect,
	}
}

// calls gives how often State was called and when ExitNodes was.
func (n *fakeNode) calls() (looks int, lists []time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.looks, append([]time.Time{}, n.lists...)
}

func (n *fakeNode) State(ctx context.Context) (NodeState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.looks++
	if n.err != nil {
		return NodeState{}, n.err
	}
	return n.state, nil
}

func (n *fakeNode) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lists = append(n.lists, n.clock.Now())
	if n.err != nil {
```

Найти:

```go
	h.core = &fakeCore{rec: h.rec, clash: h.clash, node: newFakeNode(h.rec)}
```

Заменить на:

```go
	h.core = &fakeCore{rec: h.rec, clash: h.clash, node: newFakeNode(h.rec, h.clock)}
```

Изменить `helper/internal/engine/connector_test.go`:

Найти:

```go
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) || !slices.Equal(h.e.State().Tailscale.ExitNodes, list) {
		t.Fatalf("the list stays: %+v", got)
	}
}

// tailscale.logout in off (spec 7): the answer comes at once; the setup mode starts because it does not
```

Заменить на:

```go
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) || !slices.Equal(h.e.State().Tailscale.ExitNodes, list) {
		t.Fatalf("the list stays: %+v", got)
	}
}

// Spec 7: every call of the API service is one more sing-box.exe, and the check for another core (5.3 step
// 1) counts each. So the setup mode reads the exit nodes when the node is first seen logged in, then at most
// once a minute, not at each look at the node every 2 s.
func TestSetupReadsExitNodesOncePerMinute(t *testing.T) {
	h, _ := setupRunning(t)
	h.waitFor("three reads of the list", func() bool { _, lists := h.core.node.calls(); return len(lists) >= 3 })
	looks, lists := h.core.node.calls()
	for i := 1; i < len(lists); i++ {
		if gap := lists[i].Sub(lists[i-1]); gap < time.Minute || gap > time.Minute+nodeEvery {
			t.Fatalf("read %d came %s after the one before", i+1, gap)
		}
	}
	if looks < 60 {
		t.Fatalf("the node itself is still read every 2 s: %d looks", looks)
	}
}

// tailscale.logout in off (spec 7): the answer comes at once; the setup mode starts because it does not
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestSetupReadsExitNodesOncePerMinute$' -count=1
```

Ожидается: список читается при каждом опросе, второе чтение — через 2 секунды после первого:

```text
--- FAIL: TestSetupReadsExitNodesOncePerMinute (0.01s)
    connector.go:239: setup mode: core started
    connector_test.go:308: read 2 came 2s after the one before
FAIL
FAIL	splitlane/helper/internal/engine
```

- [ ] **Step 3: Список exit node в режиме настройки — не чаще раза в минуту**

Изменить `helper/internal/engine/connector.go`:

Найти:

```go
	setupIdle = 5 * time.Minute // the setup core runs so long after the last window went
)
```

Заменить на:

```go
	setupIdle = 5 * time.Minute // the setup core runs so long after the last window went
	listEvery = time.Minute     // the exit nodes are read at most so often: each call is one more sing-box.exe
)
```

Найти:

```go
	gen    int           // guarded by Engine.mu: a logout moves it on, so a look at the node from before it is dropped
}
```

Заменить на:

```go
	gen    int           // guarded by Engine.mu: a logout moves it on, so a look at the node from before it is dropped
	listAt time.Time     // owned by runSetup: the last read of the exit nodes; zero while the node is not logged in
}
```

Найти:

```go
// starting); then the flag and the exit nodes go into state.json, and the state shows that list while
// no core runs. needs_login clears the flag; needs_approval leaves it as it is. A change is published,
// the backend and the list in one state. A look that a logout overtook is dropped.
```

Заменить на:

```go
// starting); then the flag and the exit nodes go into state.json, and the state shows that list while
// no core runs. The exit nodes are read at the first look that finds the node logged in, then at most
// once a minute: each call of the API service is one more sing-box.exe, which the check for another
// core counts. needs_login clears the flag; needs_approval leaves it as it is. A change is published,
// the backend and the list in one state. A look that a logout overtook is dropped.
```

Найти:

```go
	var nodes []protocol.ExitNode
	var nodesErr error
	if loggedIn {
		nodes, nodesErr = s.node.ExitNodes(cctx)
	}
```

Заменить на:

```go
	var nodes []protocol.ExitNode
	var nodesErr error
	listed := false
	switch now := e.d.Clock.Now(); {
	case !loggedIn:
		s.listAt = time.Time{} // the next look that finds the node logged in reads the list at once
	case s.listAt.IsZero() || !now.Before(s.listAt.Add(listEvery)):
		s.listAt, listed = now, true
		nodes, nodesErr = s.node.ExitNodes(cctx)
	}
```

Найти:

```go
	if loggedIn && nodesErr == nil && !slices.Equal(nodes, e.saved.ExitNodes) {
```

Заменить на:

```go
	if listed && nodesErr == nil && !slices.Equal(nodes, e.saved.ExitNodes) {
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/engine/; go test ./internal/engine/ -run 'Test(SetupReadsExitNodesOncePerMinute|LoggedInIsSaved|NeedsLoginClearsLoggedIn)$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./internal/engine/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestLoggedInIsSaved`, `--- PASS: TestNeedsLoginClearsLoggedIn`, `--- PASS: TestSetupReadsExitNodesOncePerMinute`, затем `ok  	splitlane/helper/internal/engine` для выбранных тестов и для пакета целиком.

- [ ] **Step 5: Написать тесты коннектора при работе**

Подмена ядра запоминает порт и секрет последнего вызова `Node`; подмена службы API умеет отвечать ошибкой только на `Path` и задерживать один вызов `Path` до отмены его контекста (событие `node:path cancelled`). Таблица `TestLoginPhaseTable` получает столбцы `degraded` и «`degraded` с `backend = expired`» (`colExpired`, фазу готовит `expiredOn`).

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
type fakeCore struct {
	mu        sync.Mutex
	rec       *recorder
	checkErr  error
	startErr  error
	procs     []*fakeProc
	log       string
	silent    int  // so many next starts never write the start line
	dieEarly  int  // so many next starts exit before the start line
	needLogin bool // the node asks for a login after start
	leftover  *fakeProc
	clash     *fakeClash
	onStart   func(p *fakeProc)
	opts      CoreOptions  // of the last BuildConfig
	setupOpts SetupOptions // of the last BuildSetupConfig
	node      *fakeNode    // the API service of every run
	setupNext bool         // the last config built was the setup mode's: the next start is a setup core
}
```

Заменить на:

```go
type fakeCore struct {
	mu         sync.Mutex
	rec        *recorder
	checkErr   error
	startErr   error
	procs      []*fakeProc
	log        string
	silent     int  // so many next starts never write the start line
	dieEarly   int  // so many next starts exit before the start line
	needLogin  bool // the node asks for a login after start
	leftover   *fakeProc
	clash      *fakeClash
	onStart    func(p *fakeProc)
	opts       CoreOptions  // of the last BuildConfig
	setupOpts  SetupOptions // of the last BuildSetupConfig
	node       *fakeNode    // the API service of every run
	setupNext  bool         // the last config built was the setup mode's: the next start is a setup core
	nodePort   int          // the port of the last call of Node
	nodeSecret string       // the secret of the last call of Node
}
```

Найти:

```go
func (c *fakeCore) Node(port int, secret string) NodeAPI { return c.node }
```

Заменить на:

```go
func (c *fakeCore) Node(port int, secret string) NodeAPI {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodePort, c.nodeSecret = port, secret
	return c.node
}
```

Найти:

```go
	err     error // every call fails with it
	logouts int
```

Заменить на:

```go
	err     error         // every call fails with it
	pathErr error         // Path fails with it
	hold    chan struct{} // the next Path closes it and waits for the end of its context
	logouts int
```

Найти:

```go
func (n *fakeNode) Path(ctx context.Context, exitNode string) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return "", n.err
	}
	return n.path, nil
}
```

Заменить на:

```go
func (n *fakeNode) Path(ctx context.Context, exitNode string) (string, error) {
	n.mu.Lock()
	hold := n.hold
	n.hold = nil
	n.mu.Unlock()
	if hold != nil {
		close(hold)
		<-ctx.Done()
		n.rec.add("node:path cancelled")
		return "", ctx.Err()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	switch {
	case n.err != nil:
		return "", n.err
	case n.pathErr != nil:
		return "", n.pathErr
	}
	return n.path, nil
}
```

Изменить `helper/internal/engine/enable_test.go`:

Найти:

```go
func enabled(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	return h
}
```

Заменить на:

```go
// enabled is a helper in phase on after 5.3, with the look at the path to the exit node that follows
// phase on done (spec 7): nothing of the enable runs on behind the test.
func enabled(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	return h
}
```

Изменить `helper/internal/engine/connector_test.go`:

Найти:

```go
// TestLoginPhaseTable walks the cells of 5.1 for tailscale.login and tailscale.logout in off, error,
// starting, on, blocked and stopping: in off and error the setup mode starts (login) or the node logs
// out (logout) and the phase stays; starting and stopping are busy; on and blocked are wrong_phase.
func TestLoginPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	phases := []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
		protocol.PhaseBlocked, protocol.PhaseStopping}
	rows := []struct {
		name   string
		method string
		want   []string // off, error, starting, on, blocked, stopping
		act    func(t *testing.T, h *harness)
	}{
		{"login", protocol.MethodLogin, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("the setup core's node read", func() bool {
					return h.rec.index("core:start 1000 setup") >= 0 && backendIs(h, protocol.BackendRunning)
				})
			}},
		{"logout", protocol.MethodLogout, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
			}},
	}
	for _, row := range rows {
		for i, phase := range phases {
			t.Run(row.name+"/"+phase, func(t *testing.T) {
				h, release := inPhase(t, phase)
				defer release()
				t.Cleanup(func() { shutdown(t, h) })
				before := h.rec.list()
				err := call(h, row.method, nil)
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h)
					if got := h.e.State().Phase; got != phase {
						t.Fatalf("the phase stays %s, got %s", phase, got)
					}
				default:
					wantCode(t, err, want)
					time.Sleep(20 * time.Millisecond)
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phase {
						t.Fatalf("nothing happens: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				}
			})
		}
	}
}
```

Заменить на:

```go
// colExpired is the column of TestLoginPhaseTable for phase degraded with backend expired (5.1).
const colExpired = "degraded-expired"

// expiredOn is a helper in phase on whose look at the node of the main core every 10 s has found it
// needing a login again (spec 7): phase degraded, backend expired, the node offers a login link.
func expiredOn(t *testing.T) *harness {
	t.Helper()
	h := enabled(t)
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.e.checkNode(context.Background())
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || s.Tailscale.Backend != protocol.BackendExpired {
		t.Fatalf("setup: %+v", s)
	}
	return h
}

// TestLoginPhaseTable walks the cells of 5.1 for tailscale.login and tailscale.logout in off, error,
// starting, on, degraded, degraded with backend expired, blocked and stopping: in off and error the setup
// mode starts (login) or the node logs out (logout) and the phase stays; in degraded with backend expired
// login disables first (5.4), then the setup mode starts with the node's login link; starting and
// stopping are busy; every other live cell is wrong_phase.
func TestLoginPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	cols := []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
		protocol.PhaseDegraded, colExpired, protocol.PhaseBlocked, protocol.PhaseStopping}
	rows := []struct {
		name   string
		method string
		want   []string // off, error, starting, on, degraded, degraded with backend expired, blocked, stopping
		act    func(t *testing.T, h *harness, col string)
	}{
		{"login", protocol.MethodLogin, []string{cellAct, cellAct, busy, wrong, wrong, cellAct, wrong, busy},
			func(t *testing.T, h *harness, col string) {
				if col == colExpired {
					h.waitFor("off, the setup core and its login link", func() bool {
						s := h.e.State()
						return s.Phase == protocol.PhaseOff && h.core.setupAlive() && s.Tailscale.AuthURL != nil && *s.Tailscale.AuthURL == loginURL
					})
					return
				}
				h.waitFor("the setup core's node read", func() bool {
					return h.rec.index("core:start 1000 setup") >= 0 && backendIs(h, protocol.BackendRunning)
				})
			}},
		{"logout", protocol.MethodLogout, []string{cellAct, cellAct, busy, wrong, wrong, wrong, wrong, busy},
			func(t *testing.T, h *harness, col string) {
				h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
			}},
	}
	for _, row := range rows {
		for i, col := range cols {
			t.Run(row.name+"/"+col, func(t *testing.T) {
				var h *harness
				release := func() {}
				if col == colExpired {
					h = expiredOn(t)
				} else {
					h, release = inPhase(t, col)
				}
				defer release()
				t.Cleanup(func() { shutdown(t, h) })
				before, phase := h.rec.list(), h.e.State().Phase
				err := call(h, row.method, nil)
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h, col)
					if got := h.e.State().Phase; got != phase && col != colExpired {
						t.Fatalf("the phase stays %s, got %s", phase, got)
					}
				default:
					wantCode(t, err, want)
					time.Sleep(20 * time.Millisecond)
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phase {
						t.Fatalf("nothing happens: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				}
			})
		}
	}
}

// Spec 7, 5.5 and section 10, the row "Сервер VPN недоступен, истечение входа": while the main core
// runs, the look at its node every 10 s finds that it needs a login again: phase degraded with reason
// login_expired, backend expired, no login link (the setup mode gives it), "logged in" cleared in
// state.json. The core keeps running and the system is not touched; the next round of 5.5 sends names
// through the provider's DNS as in every degraded, and the VPN lane stays on the VPN: its programs have
// no link, they never go direct. The login line in the core's log leads to the same state. The node is
// the run's own API service: the port and the secret of its config.
func TestExpiredLoginWhileOn(t *testing.T) {
	h := enabled(t)
	o := h.core.options()
	h.core.mu.Lock()
	port, secret := h.core.nodePort, h.core.nodeSecret
	h.core.mu.Unlock()
	if port != o.Ports.API || secret != o.APISecret || secret == "" {
		t.Fatalf("the API service of the run: port %d, secret %q; config %d, %q", port, secret, o.Ports.API, o.APISecret)
	}
	core := h.core.last()
	mark := len(h.rec.list())
	h.e.checkNode(context.Background())
	if s := h.e.State(); s.Phase != protocol.PhaseOn || s.Tailscale.Backend != protocol.BackendRunning {
		t.Fatalf("a logged-in node changes nothing: %+v", s)
	}
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.e.checkNode(context.Background())
	s := h.e.State()
	if s.Phase != protocol.PhaseDegraded || s.Reason == nil || s.Reason.Code != protocol.ReasonLoginExpired ||
		s.Tailscale.Backend != protocol.BackendExpired || s.Tailscale.AuthURL != nil {
		t.Fatalf("degraded, login_expired, backend expired, no link: %+v", s)
	}
	if saved := h.store.saved(); saved.LoggedIn || !saved.WasOn || !core.alive() || countSince(h, mark, "core:") != 0 || countSince(h, mark, "sys:") != 0 {
		t.Fatalf("logged in cleared, the core runs on, the system untouched: %+v, events %v", saved, h.rec.list()[mark:])
	}
	rounds(h, 1)
	if s := h.e.State(); h.clash.currentMode() != "vpn-localdns" || s.Phase != protocol.PhaseDegraded || s.Reason.Code != protocol.ReasonLoginExpired {
		t.Fatalf("names through the provider's DNS, the VPN lane on the VPN: mode %s, %+v", h.clash.currentMode(), s)
	}

	h = enabled(t)
	h.core.appendLog("INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x")
	h.e.checkNode(context.Background())
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || s.Reason.Code != protocol.ReasonLoginExpired ||
		s.Tailscale.Backend != protocol.BackendExpired || h.store.saved().LoggedIn || !h.core.last().alive() {
		t.Fatalf("the login line of the core's log: %+v", s)
	}
}

// Spec 7 and 5.1: tailscale.login in degraded with backend expired disables first (5.4: the system exit
// node given back and confirmed, then the core stopped; "was on" cleared at the start) and only then
// starts the setup mode, whose node offers the login link: in degraded a browser on the VPN lane could
// not open it. The phase ends in off; after the login the user enables again.
func TestLoginWhileExpiredDisablesFirst(t *testing.T) {
	h := expiredOn(t)
	t.Cleanup(func() { shutdown(t, h) })
	main := h.core.last()
	mark := len(h.rec.list())
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	if h.store.saved().WasOn {
		t.Fatal("\"was on\" is cleared at the start of the disabling")
	}
	h.waitFor("the login link of the setup core", func() bool {
		u := h.e.State().Tailscale.AuthURL
		return u != nil && *u == loginURL
	})
	order := []string{"sys:set " + exitNode + " true", "journal:delete", fmt.Sprintf("core:interrupt %d", main.id.PID),
		"core:setupconfig", fmt.Sprintf("core:start %d setup", main.id.PID+1)}
	if !inOrder(h, mark, order...) || main.alive() {
		t.Fatalf("restore confirmed, core stopped, then the setup core: %v", h.rec.list()[mark:])
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,degraded,stopping,off" {
		t.Fatalf("phases %s", got)
	}
	if s := h.e.State(); s.Phase != protocol.PhaseOff || s.Tailscale.Backend != protocol.BackendNeedsLogin || h.store.hasJournal() || h.store.saved().LoggedIn {
		t.Fatalf("off, the setup core's node needs a login, no journal: %+v", s)
	}
}

// tailscale.path (5.2, spec 7): read through the run's API service right after phase on, then every
// 60 s while the core runs live; lastPath keeps it with the time it was read (the network watch of
// task 10), readPath reads it now. No pong is an unknown path: null. Null when the main core does not
// run: after a disable.
func TestPathIsReported(t *testing.T) {
	h := enabled(t)
	if p := h.e.State().Tailscale.Path; p == nil || *p != protocol.PathDirect {
		t.Fatalf("direct right after phase on: %v", p)
	}
	path, at := h.e.lastPath()
	if path != protocol.PathDirect || at.IsZero() {
		t.Fatalf("lastPath %q at %s", path, at)
	}
	h.core.node.set(func(n *fakeNode) { n.path = protocol.PathRelay })
	h.clock.After(listEvery)
	h.e.refreshNode(context.Background())
	if p := h.e.State().Tailscale.Path; p == nil || *p != protocol.PathRelay {
		t.Fatalf("relay a minute later: %v", p)
	}
	if path, later := h.e.lastPath(); path != protocol.PathRelay || !later.After(at) {
		t.Fatalf("lastPath %q at %s", path, later)
	}
	h.core.node.set(func(n *fakeNode) { n.path = "" })
	if got := h.e.readPath(context.Background()); got != "" || h.e.State().Tailscale.Path != nil {
		t.Fatalf("no pong: unknown, null: %q %v", got, h.e.State().Tailscale.Path)
	}
	h.core.node.set(func(n *fakeNode) { n.path = protocol.PathDirect })
	if got := h.e.readPath(context.Background()); got != protocol.PathDirect {
		t.Fatalf("readPath %q", got)
	}
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	if path, _ := h.e.lastPath(); h.e.State().Tailscale.Path != nil || path != "" || h.e.readPath(context.Background()) != "" {
		t.Fatalf("null after the disable: %v %q", h.e.State().Tailscale.Path, path)
	}
}

// Spec 7, 5.2: while the core runs live, the exit nodes with their online flags are read every 60 s
// with the path, saved in state.json and published; the selection stays. The look every 10 s does not
// read them.
func TestExitNodesRefreshWhileOn(t *testing.T) {
	h := enabled(t)
	_, before := h.core.node.calls()
	h.e.checkNode(context.Background())
	if _, lists := h.core.node.calls(); len(lists) != len(before) {
		t.Fatalf("the look every 10 s reads no list: %d reads", len(lists)-len(before))
	}
	nodes := []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: false}, {ID: "100.64.0.2", Name: "exit-b", Online: true}}
	h.core.node.set(func(n *fakeNode) { n.nodes = nodes })
	states := publishedCount(h)
	h.e.refreshNode(context.Background())
	if !slices.Equal(h.store.saved().ExitNodes, nodes) || !slices.Equal(h.e.State().Tailscale.ExitNodes, nodes) || publishedCount(h) == states {
		t.Fatalf("saved and published: %+v", h.store.saved().ExitNodes)
	}
	if s := h.e.State(); s.Phase != protocol.PhaseOn || s.Tailscale.SelectedExitNode == nil || *s.Tailscale.SelectedExitNode != exitNode {
		t.Fatalf("on, the selection stays: %+v", s)
	}
}

// Spec 7: the errors of the API service carry the core's command line and its standard error, which may
// name peers and addresses in the tailnet: while the main core runs they reach the log masked (<ip>),
// once while they repeat. A failed ping is an unknown path: tailscale.path null, lastPath "".
func TestNodeErrorsWhileOnAreMasked(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	h.core.node.set(func(n *fakeNode) {
		n.pathErr = errors.New("sing-box.exe api tailscale ping 100.64.0.1: exit status 1: no reply from 100.64.0.1 via 192.0.2.7:41641")
	})
	h.e.refreshNode(context.Background())
	h.e.refreshNode(context.Background())
	var logged []string
	for _, ev := range h.rec.list() {
		if strings.HasPrefix(ev, "log:node of the core: ") {
			logged = append(logged, ev)
		}
		if strings.Contains(ev, "192.0.2.7") {
			t.Fatalf("an address in the log: %q", ev)
		}
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "ping <ip>: exit status 1: no reply from <ip> via <ip>:41641") {
		t.Fatalf("logged masked, once: %q", logged)
	}
	if path, _ := h.e.lastPath(); h.e.State().Tailscale.Path != nil || path != "" {
		t.Fatalf("a failed ping is an unknown path: %v %q", h.e.State().Tailscale.Path, path)
	}
}

// Every call of the API service is a sing-box.exe of that run (task 3): none is in flight when its core
// ends. The stop of 5.4 (step 2) cancels the call and waits for its end before the core gets its signal,
// and the 60-second look reads nothing more.
func TestStopWaitsForNodeCalls(t *testing.T) {
	h := enabled(t)
	entered := make(chan struct{})
	h.core.node.set(func(n *fakeNode) { n.hold = entered })
	_, lists := h.core.node.calls()
	done := make(chan struct{})
	go func() {
		h.e.refreshNode(context.Background())
		close(done)
	}()
	waitClosed(t, entered, "the ping of the exit node")
	mark := len(h.rec.list())
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	waitClosed(t, done, "the end of the 60-second look")
	if !inOrder(h, mark, "sys:set", "node:path cancelled", fmt.Sprintf("core:interrupt %d", h.core.last().id.PID)) {
		t.Fatalf("the call ends before the core is stopped: %v", h.rec.list()[mark:])
	}
	if _, after := h.core.node.calls(); len(after) != len(lists) {
		t.Fatalf("no list read after the cancel: %d", len(after)-len(lists))
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'Test(ExpiredLoginWhileOn|LoginWhileExpiredDisablesFirst|PathIsReported|ExitNodesRefreshWhileOn|NodeErrorsWhileOnAreMasked|StopWaitsForNodeCalls|LoginPhaseTable)$' -count=1
```

Ожидается: тесты не собираются, методов узла рабочего ядра нет:

```text
# splitlane/helper/internal/engine [splitlane/helper/internal/engine.test]
internal\engine\connector_test.go:448:6: h.e.checkNode undefined (type *Engine has no field or method checkNode)
internal\engine\connector_test.go:541:6: h.e.checkNode undefined (type *Engine has no field or method checkNode)
internal\engine\connector_test.go:546:6: h.e.checkNode undefined (type *Engine has no field or method checkNode)
internal\engine\connector_test.go:562:6: h.e.checkNode undefined (type *Engine has no field or method checkNode)
internal\engine\connector_test.go:610:18: h.e.lastPath undefined (type *Engine has no field or method lastPath)
internal\engine\connector_test.go:616:6: h.e.refreshNode undefined (type *Engine has no field or method refreshNode)
internal\engine\connector_test.go:620:24: h.e.lastPath undefined (type *Engine has no field or method lastPath)
internal\engine\connector_test.go:624:16: h.e.readPath undefined (type *Engine has no field or method readPath)
internal\engine\connector_test.go:628:16: h.e.readPath undefined (type *Engine has no field or method readPath)
internal\engine\connector_test.go:635:20: h.e.lastPath undefined (type *Engine has no field or method lastPath)
internal\engine\connector_test.go:635:20: too many errors
FAIL	splitlane/helper/internal/engine [build failed]
FAIL
```

- [ ] **Step 7: Узел рабочего ядра в автомате**

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
// coreRun is one running core with its ports and the rule files it reads now.
type coreRun struct {
	proc     CoreProc
	clash    ClashAPI
	ports    Ports
	mode     string // the driver's name of the core's mode now
	direct   []byte
	vpn      []byte
	stopping bool // guarded by Engine.mu: the helper stops this core on purpose
	cleared  bool // guarded by Engine.mu: this run has switched the system exit node off (step 6 of 5.3)
}
```

Заменить на:

```go
// coreRun is one running core with its ports, the rule files it reads now and its API service.
type coreRun struct {
	proc     CoreProc
	clash    ClashAPI
	node     NodeAPI // the API service of this run (spec 7): the port and the secret of its config
	ports    Ports
	mode     string // the driver's name of the core's mode now
	direct   []byte
	vpn      []byte
	stopping bool // guarded by Engine.mu: the helper stops this core on purpose
	cleared  bool // guarded by Engine.mu: this run has switched the system exit node off (step 6 of 5.3)

	// The node of the core while it runs (spec 7, connector.go).
	calls    sync.WaitGroup     // the calls of node under way; none begins once stopping is set
	callsEnd context.Context    // ends with the run (stopCore, the end of the session), and the calls with it
	endCalls context.CancelFunc // ends callsEnd
	backend  string             // guarded by Engine.mu: tailscale.backend as the node said last (running: the tunnel was proven)
	expired  bool               // guarded by Engine.mu: the node asked for a login again while the core ran
	path     string             // guarded by Engine.mu: the path to the exit node read last, "" while unknown
	pathAt   time.Time          // guarded by Engine.mu: when the path was read
	logMark  int64              // guarded by Engine.mu: the core's log is read for a login request from here
	errs     map[string]string  // guarded by Engine.mu: the last error of each kind of call, masked
}
```

Найти:

```go
// publish sends the state to the clients; e.mu must be held. tailscale.backend and authUrl come from
// the connector (spec 7): the node of the setup core, stopped when no core runs (5.2); while the main
// core runs in a live phase, running.
func (e *Engine) publish() {
	e.syncSaved()
	ts := &e.st.Tailscale
	ts.Backend, ts.AuthURL = e.nodeBackend, nil
	if e.run != nil && live(e.st.Phase) {
		ts.Backend = protocol.BackendRunning
	} else if e.nodeAuthURL != "" {
```

Заменить на:

```go
// publish sends the state to the clients; e.mu must be held. tailscale.backend, authUrl and path come
// from the connector (spec 7, 5.2). While the main core runs in a live phase: its node (expired once it
// asked for a login again), no login link (the setup mode gives it), the path read last. Otherwise the
// node of the setup core with its link, stopped when no core runs, and path null.
func (e *Engine) publish() {
	e.syncSaved()
	ts := &e.st.Tailscale
	ts.Backend, ts.AuthURL, ts.Path = e.nodeBackend, nil, nil
	if r := e.run; r != nil && live(e.st.Phase) {
		ts.Backend = protocol.BackendRunning
		switch {
		case r.expired:
			ts.Backend = protocol.BackendExpired
		case r.backend != "":
			ts.Backend = r.backend
		}
		if r.path != "" {
			p := r.path
			ts.Path = &p
		}
	} else if e.nodeAuthURL != "" {
```

Изменить `helper/internal/engine/enable.go`:

Найти:

```go
// returned and the phase stays as it is.
func (e *Engine) finishOnChecked() *protocol.Reason {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r != nil && !r.stopping && exited(r.proc) {
		return reason(protocol.ReasonCoreCrashed)
	}
	e.finishOn()
	return nil
}
```

Заменить на:

```go
// returned and the phase stays as it is. Right after phase on the path to the exit node is read
// (spec 7, tailscale.path), in the background: the operation does not wait for the ping.
func (e *Engine) finishOnChecked() *protocol.Reason {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r != nil && !r.stopping && exited(r.proc) {
		return reason(protocol.ReasonCoreCrashed)
	}
	e.finishOn()
	go e.readPath(context.Background())
	return nil
}
```

Найти:

```go
	r := &coreRun{
		ports:  Ports{ProbeVPN: p[0], ProbeDirect: p[1], Clash: p[2], API: p[3]},
		mode:   e.d.Core.Mode(s.DefaultLane, false, false),
		direct: direct,
		vpn:    vpn,
	}
	clashSecret := e.d.Host.Secret()
	config, err := e.d.Core.BuildConfig(CoreOptions{
		Mode: r.mode, HelperPath: e.d.Settings.HelperPath, TailscaleDir: e.d.Sys.Dir(), Hostname: e.hostname(),
		ExitNode: s.SelectedExitNode, Ports: r.ports, ClashSecret: clashSecret, APISecret: e.d.Host.Secret(),
```

Заменить на:

```go
	r := &coreRun{
		ports:   Ports{ProbeVPN: p[0], ProbeDirect: p[1], Clash: p[2], API: p[3]},
		mode:    e.d.Core.Mode(s.DefaultLane, false, false),
		direct:  direct,
		vpn:     vpn,
		backend: protocol.BackendRunning,
	}
	clashSecret, apiSecret := e.d.Host.Secret(), e.d.Host.Secret()
	config, err := e.d.Core.BuildConfig(CoreOptions{
		Mode: r.mode, HelperPath: e.d.Settings.HelperPath, TailscaleDir: e.d.Sys.Dir(), Hostname: e.hostname(),
		ExitNode: s.SelectedExitNode, Ports: r.ports, ClashSecret: clashSecret, APISecret: apiSecret,
```

Найти:

```go
	r.proc, r.clash = proc, e.d.Core.Clash(r.ports.Clash, clashSecret)
	e.attach(r)
```

Заменить на:

```go
	r.proc, r.clash, r.node = proc, e.d.Core.Clash(r.ports.Clash, clashSecret), e.d.Core.Node(r.ports.API, apiSecret)
	r.callsEnd, r.endCalls = context.WithCancel(context.Background())
	e.attach(r)
```

Изменить `helper/internal/engine/teardown.go`:

Найти:

```go
// stopCore is step 2 of 5.4 for the current core.
func (e *Engine) stopCore() {
	e.mu.Lock()
	r := e.run
	if r != nil {
		r.stopping = true
	}
	e.mu.Unlock()
	if r == nil {
		return
	}
	e.stopProc(r.proc)
```

Заменить на:

```go
// stopCore is step 2 of 5.4 for the current core. The calls of its API service under way are cancelled
// and waited for first: each is a sing-box.exe of this run, and none may outlive it (spec 7); none
// begins once stopping is set.
func (e *Engine) stopCore() {
	e.mu.Lock()
	r := e.run
	if r != nil {
		r.stopping = true
	}
	e.mu.Unlock()
	if r == nil {
		return
	}
	if r.endCalls != nil {
		r.endCalls()
	}
	r.calls.Wait()
	e.stopProc(r.proc)
```

Найти:

```go
func (e *Engine) beginDisableLocked() bool {
	if e.op != nil {
		return false
	}
	e.saved.WasOn = false
	e.save()
	e.setPhase(protocol.PhaseStopping, nil)
	return e.startOp("disable", func(context.Context) {
		ok := e.teardown()
		e.mu.Lock()
		defer e.mu.Unlock()
		if ok {
			e.finish(protocol.PhaseOff, nil)
		} else {
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
		}
	})
}
```

Заменить на:

```go
func (e *Engine) beginDisableLocked() bool { return e.disableThenLocked(nil) }

// disableThenLocked is beginDisableLocked with then, which runs with e.mu held right after the final
// phase is set: tailscale.login with an expired login starts the setup mode there (spec 7).
func (e *Engine) disableThenLocked(then func()) bool {
	if e.op != nil {
		return false
	}
	e.saved.WasOn = false
	e.save()
	e.setPhase(protocol.PhaseStopping, nil)
	return e.startOp("disable", func(context.Context) {
		ok := e.teardown()
		e.mu.Lock()
		defer e.mu.Unlock()
		if ok {
			e.finish(protocol.PhaseOff, nil)
		} else {
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
		}
		if then != nil {
			then()
		}
	})
}
```

Изменить `helper/internal/engine/supervise.go`:

Найти:

```go
// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s.
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	e.every(ctx, healthEvery, e.checkHealth)
}
```

Заменить на:

```go
// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s;
// and the node of the main core (spec 7): its state every 10 s, its path and exit nodes every 60 s.
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	go e.every(ctx, healthEvery, e.checkNode)
	go e.every(ctx, listEvery, e.refreshNode)
	e.every(ctx, healthEvery, e.checkHealth)
}
```

Найти:

```go
	now := e.degraded
```

Заменить на:

```go
	now := e.vpnDown()
```

Найти:

```go
// With both conditions the phase is blocked (5.2).
func (e *Engine) refreshLive() {
	phase, why := protocol.PhaseOn, (*protocol.Reason)(nil)
	if e.degraded {
		phase, why = protocol.PhaseDegraded, reason(protocol.ReasonVPNUnreachable)
	}
	if e.sysExitOn {
		phase, why = protocol.PhaseBlocked, nil
	}
	e.setPhase(phase, why)
}
```

Заменить на:

```go
// With both conditions the phase is blocked (5.2). An expired login (spec 7) is degraded with reason
// login_expired, also while the VPN server does not answer: the window then offers "Выключить деление
// и войти".
func (e *Engine) refreshLive() {
	phase, why := protocol.PhaseOn, (*protocol.Reason)(nil)
	switch {
	case e.loginExpired():
		phase, why = protocol.PhaseDegraded, reason(protocol.ReasonLoginExpired)
	case e.degraded:
		phase, why = protocol.PhaseDegraded, reason(protocol.ReasonVPNUnreachable)
	}
	if e.sysExitOn {
		phase, why = protocol.PhaseBlocked, nil
	}
	e.setPhase(phase, why)
}

// vpnDown: the VPN lane has no link, because the VPN server does not answer (5.5) or the login expired
// (spec 7); names then go through the provider's DNS, as in every degraded (6.3, 6.4). e.mu must be held.
func (e *Engine) vpnDown() bool { return e.degraded || e.loginExpired() }
```

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
	r, allDirect, degraded := e.run, e.st.AllDirect, e.degraded
```

Заменить на:

```go
	r, allDirect, degraded := e.run, e.st.AllDirect, e.vpnDown()
```

Найти:

```go
	r, lane, degraded, apps, was := e.run, e.saved.DefaultLane, e.degraded, e.saved.Apps, e.st.AllDirect
```

Заменить на:

```go
	r, lane, degraded, apps, was := e.run, e.saved.DefaultLane, e.vpnDown(), e.saved.Apps, e.st.AllDirect
```

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
// The core of the setup mode (spec 7) is ended the same way, without waiting.
// It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o, s := e.journal, e.run, e.op, e.setup
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
	}
```

Заменить на:

```go
// The core of the setup mode (spec 7) is ended the same way, without waiting, and so are the calls of
// the main core's API service. It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o, s := e.journal, e.run, e.op, e.setup
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
		if r.endCalls != nil {
			r.endCalls()
		}
	}
```

Изменить `helper/internal/engine/connector.go`:

Найти:

```go
	"encoding/json"
	"fmt"
```

Заменить на:

```go
	"encoding/json"
	"errors"
	"fmt"
```

Найти:

```go
// error; busy in starting and stopping and after helper.shutdown; wrong_phase in on, degraded and
// blocked. e.mu must be held.
```

Заменить на:

```go
// error; busy in starting and stopping and after helper.shutdown; wrong_phase in on, degraded and
// blocked (tailscale.login takes degraded with backend expired before it). e.mu must be held.
```

Найти:

```go
// cmdLogin is tailscale.login in off and error: the setup mode starts, the login link comes in the
// state (spec 7). While it runs nothing new starts: its link stays (Review Focus 5). The answer comes
// at once, the core starts in the background.
func (e *Engine) cmdLogin(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.setupPhaseLocked(); err != nil {
```

Заменить на:

```go
// cmdLogin is tailscale.login (5.1). In off and error the setup mode starts, the login link comes in
// the state (spec 7); while it runs nothing new starts: its link stays (Review Focus 5). In degraded
// with backend expired the disabling of 5.4 comes first ("was on" cleared at its start, as for
// routing.disable), and the setup mode starts once the phase is off: in degraded a browser on the VPN
// lane could not open the login page. The answer comes at once, the rest runs in the background.
func (e *Engine) cmdLogin(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.exiting && e.st.Phase == protocol.PhaseDegraded && e.loginExpired() {
		if !e.disableThenLocked(e.startSetupLocked) {
			return busy()
		}
		e.d.Log("tailscale.login with the login expired: disabling first, then the setup mode")
		return nil
	}
	if err := e.setupPhaseLocked(); err != nil {
```

Найти:

```go
func (e *Engine) logoutFailed(why string) {
	e.d.Log("tailscale.logout failed: %s", why)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.publish()
}
```

Заменить на:

```go
func (e *Engine) logoutFailed(why string) {
	e.d.Log("tailscale.logout failed: %s", why)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.publish()
}

// The node of the main core (spec 7, 5.2): while the core runs in on, degraded or blocked, its API
// service is read like the setup core's, through calls bound to the run.

// logOverlap: the core's log is read again from this many bytes before the end it had at the last
// look, so that a line being written then is seen whole.
const logOverlap = 4096

// errRunEnded: a call of the API service of the main core did not run, or its run ended during it.
var errRunEnded = errors.New("the run of the core has ended")

// loginExpired: the node of the main core needs a login again (spec 7); e.mu must be held.
func (e *Engine) loginExpired() bool { return e.run != nil && e.run.expired }

// nodeCall is one call of the API service of the main core's run r, bounded by ctx and by callTimeout.
// Each call is one more sing-box.exe of that run, so none begins once the run is stopping, and the
// stop cancels the calls under way and waits for them before the core ends (stopCore). An error
// carries the core's command line and its standard error, which may name peers and tailnet addresses:
// it is logged masked, once while it repeats for the same kind of call. errRunEnded: no call was made,
// or the run ended during it.
func (e *Engine) nodeCall(ctx context.Context, r *coreRun, kind string, call func(ctx context.Context, n NodeAPI) error) error {
	e.mu.Lock()
	if r.node == nil || r.stopping {
		e.mu.Unlock()
		return errRunEnded
	}
	r.calls.Add(1)
	e.mu.Unlock()
	defer r.calls.Done()
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	stop := context.AfterFunc(r.callsEnd, cancel)
	defer stop()
	err := call(cctx, r.node)
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case r.stopping || ctx.Err() != nil:
		return errRunEnded
	case err != nil:
		if text := protocol.MaskAddresses(err.Error()); r.errs[kind] != text {
			if r.errs == nil {
				r.errs = map[string]string{}
			}
			r.errs[kind] = text
			e.d.Log("node of the core: %s: %s", kind, text)
		}
	default:
		delete(r.errs, kind)
	}
	return err
}

// checkNode is the look at the node of the main core every 10 s while it runs in on, degraded or
// blocked (spec 7, 5.5): tailscale.backend from its state. A node that needs a login again (the key
// expired, the device was removed in the admin console), needs_login from the API service or the login
// line in the core's log since the last look, is the expiry of spec 7: phase degraded with reason
// login_expired, backend expired, "logged in" cleared in state.json. The core keeps running: the
// programs of the VPN lane stay without a link and never go direct (5.5); names go through the
// provider's DNS as in every degraded (vpnDown). An operation under way is left alone: the next look
// finds the same, the log is read again from the same place.
func (e *Engine) checkNode(ctx context.Context) {
	e.mu.Lock()
	r := e.run
	ok := r != nil && live(e.st.Phase) && e.op == nil && !e.exiting && !e.ending
	var mark int64
	if ok {
		mark = r.logMark
	}
	e.mu.Unlock()
	if !ok {
		return
	}
	var st NodeState
	err := e.nodeCall(ctx, r, "state", func(ctx context.Context, n NodeAPI) (err error) {
		st, err = n.State(ctx)
		return err
	})
	size := e.d.Core.LogSize()
	ev := e.d.Core.Events(mark)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != r || r.stopping || e.op != nil || !live(e.st.Phase) {
		return
	}
	r.logMark = max(mark, size-logOverlap)
	changed := false
	if err == nil && st.Backend != r.backend {
		r.backend, changed = st.Backend, true
	}
	if !r.expired && ((err == nil && st.Backend == protocol.BackendNeedsLogin) || ev.NeedLogin) {
		r.expired = true
		e.d.Log("the node needs a login again: login expired; the core keeps running")
		e.saved.LoggedIn = false
		e.save()
		e.refreshLive()
		return
	}
	if changed {
		e.publish()
	}
}

// refreshNode is the look at the node of the main core every 60 s while it runs live (spec 7, 5.2): the
// path to the selected exit node (readPath), then the exit nodes with their online flags, saved in
// state.json and published when they changed.
func (e *Engine) refreshNode(ctx context.Context) {
	e.mu.Lock()
	r := e.run
	ok := r != nil && live(e.st.Phase)
	e.mu.Unlock()
	if !ok {
		return
	}
	e.readPath(ctx)
	var nodes []protocol.ExitNode
	err := e.nodeCall(ctx, r, "exit nodes", func(ctx context.Context, n NodeAPI) (err error) {
		nodes, err = n.ExitNodes(ctx)
		return err
	})
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil || e.run != r || !live(e.st.Phase) || slices.Equal(nodes, e.saved.ExitNodes) {
		return
	}
	e.saved.ExitNodes = nodes
	e.save()
	e.publish()
}

// readPath reads the path to the selected exit node now through the API service of the main core and
// keeps it with the time it was read (tailscale.path, 5.2); a change is published. "" is an unknown
// path (null): no pong, a failed call, or no main core running live.
func (e *Engine) readPath(ctx context.Context) string {
	e.mu.Lock()
	r, exitNode := e.run, e.saved.SelectedExitNode
	ok := r != nil && live(e.st.Phase)
	e.mu.Unlock()
	if !ok {
		return ""
	}
	var path string
	err := e.nodeCall(ctx, r, "path", func(ctx context.Context, n NodeAPI) (err error) {
		path, err = n.Path(ctx, exitNode)
		return err
	})
	switch {
	case errors.Is(err, errRunEnded):
		return ""
	case err != nil:
		path = "" // the ping failed: the path is unknown
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != r {
		return ""
	}
	changed := r.path != path
	r.path, r.pathAt = path, e.d.Clock.Now()
	if changed && live(e.st.Phase) {
		e.publish()
	}
	return path
}

// lastPath is the path to the exit node read last while the main core runs, with the time it was read;
// "" when it is unknown or no main core runs.
func (e *Engine) lastPath() (path string, at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r != nil {
		return r.path, r.pathAt
	}
	return "", time.Time{}
}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/engine/; go test ./internal/engine/ -run 'Test(ExpiredLoginWhileOn|LoginWhileExpiredDisablesFirst|PathIsReported|ExitNodesRefreshWhileOn|NodeErrorsWhileOnAreMasked|StopWaitsForNodeCalls|LoginPhaseTable)$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./internal/engine/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestLoginPhaseTable` (шестнадцать ячеек, в том числе `login/degraded-expired` и `logout/degraded-expired`; строки ячеек фильтр не показывает), `--- PASS: TestExpiredLoginWhileOn`, `--- PASS: TestLoginWhileExpiredDisablesFirst`, `--- PASS: TestPathIsReported`, `--- PASS: TestExitNodesRefreshWhileOn`, `--- PASS: TestNodeErrorsWhileOnAreMasked`, `--- PASS: TestStopWaitsForNodeCalls`, затем `ok  	splitlane/helper/internal/engine` для выбранных тестов и для пакета целиком (тесты 5.5, таблица фаз и тесты задачи 8 проходят без изменений).

- [ ] **Step 9: Написать тест строки состояния `slctl`**

Изменить `helper/cmd/slctl/main_test.go`:

Найти:

```go
func TestEnableWaitsForOn(t *testing.T) {
```

Заменить на:

```go
// The summary line of state, enable, wait and disable ends with the connector, which tasks 15-17 read:
// the backend, the path to the exit node or null, the number of exit nodes. The login link never goes
// there: slctl login prints it once, to the user's console.
func TestSummaryShowsTheConnector(t *testing.T) {
	link, path := "https://login.example/a/1f2e3d", protocol.PathRelay
	name := testPipeName()
	srv, err := serveState(name, protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{},
		Tailscale: protocol.Tailscale{Backend: protocol.BackendRunning, AuthURL: &link, Path: &path,
			ExitNodes: []protocol.ExitNode{{ID: "100.64.0.1", Name: "exit-a", Online: true}, {ID: "100.64.0.2", Name: "exit-b"}}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "state"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	_, line, _ := strings.Cut(out.String(), "SLCTL: ")
	if !strings.HasSuffix(line, " restoreOnDisable=false backend=running path=relay exitNodes=2\n") || strings.Contains(line, link) {
		t.Fatalf("output:\n%s", out.String())
	}
	if got := summary(protocol.State{Tailscale: protocol.Tailscale{Backend: protocol.BackendExpired}}); !strings.HasSuffix(got, " backend=expired path=null exitNodes=0") {
		t.Fatalf("summary %q", got)
	}
}

func TestEnableWaitsForOn(t *testing.T) {
```

- [ ] **Step 10: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestSummaryShowsTheConnector$' -count=1
```

Ожидается: строка `SLCTL:` кончается на `restoreOnDisable=false` (строки `RESULT` с ответами `hello` и `state.get` сокращены):

```text
--- FAIL: TestSummaryShowsTheConnector (0.01s)
    main_test.go:354: output:
        RESULT {"protocol":1,"helper":"","core":""}
        RESULT {"phase":"on", ...}
        SLCTL: phase=on reason=- allDirect=false defaultLane= apps=0 vpn=null direct=null systemExitNode=false restoreOnDisable=false
FAIL
FAIL	splitlane/helper/cmd/slctl
```

- [ ] **Step 11: Коннектор в строке состояния `slctl`**

Изменить `helper/cmd/slctl/main.go`:

Найти:

```go
func summary(s protocol.State) string {
	reason := "-"
	if s.Reason != nil {
		reason = s.Reason.Code
		for k, v := range s.Reason.Params {
			reason += fmt.Sprintf(" %s=%q", k, v)
		}
	}
	return fmt.Sprintf("phase=%s reason=%s allDirect=%v defaultLane=%s apps=%d vpn=%s direct=%s systemExitNode=%v restoreOnDisable=%v",
		s.Phase, reason, s.AllDirect, s.DefaultLane, len(s.Apps), str(s.Addresses.VPN), str(s.Addresses.Direct),
		s.SystemTailscale.ExitNodeOn, s.SystemTailscale.RestoreOnDisable)
}
```

Заменить на:

```go
// summary is the state in one line, the connector at its end (spec 7: the backend, the path to the exit
// node, the number of exit nodes). The login link is never part of it: slctl login prints it once.
func summary(s protocol.State) string {
	reason := "-"
	if s.Reason != nil {
		reason = s.Reason.Code
		for k, v := range s.Reason.Params {
			reason += fmt.Sprintf(" %s=%q", k, v)
		}
	}
	return fmt.Sprintf("phase=%s reason=%s allDirect=%v defaultLane=%s apps=%d vpn=%s direct=%s systemExitNode=%v restoreOnDisable=%v backend=%s path=%s exitNodes=%d",
		s.Phase, reason, s.AllDirect, s.DefaultLane, len(s.Apps), str(s.Addresses.VPN), str(s.Addresses.Direct),
		s.SystemTailscale.ExitNodeOn, s.SystemTailscale.RestoreOnDisable, s.Tailscale.Backend, str(s.Tailscale.Path), len(s.Tailscale.ExitNodes))
}
```

- [ ] **Step 12: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./cmd/slctl/; go test ./cmd/slctl/ -run 'TestSummaryShowsTheConnector$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./cmd/slctl/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestSummaryShowsTheConnector`, затем дважды `ok  	splitlane/helper/cmd/slctl` (остальные тесты `slctl` сравнивают только начало строки и проходят без изменений).

- [ ] **Step 13: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё одно `ok  	splitlane/helper/internal/engine` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 14: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine helper/cmd/slctl
git -C C:\code\router commit -m "feat(helper): the connector while the core runs: login expiry, the path to the exit node, the exit node list; slctl prints the connector" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 10: Сон и смена сети

Раздел 10 спецификации, строка «Сон и пробуждение, смена сети», и решение плана 1B из «Как читать»: после сна прямое соединение встроенного узла с exit node вернулось само, а после отключения кабеля на 29 секунд узел остался на ретрансляторе не меньше 7,5 минуты, тогда как новый процесс ядра сразу получил прямое соединение (приложение A, проверка (и) плана 1B). Служебный процесс получает уведомления Windows об изменении маршрутов и о питании. Маршруты: `winhost.WatchRoutes` — `NotifyRouteChange2` из `iphlpapi.dll` для IPv4 и IPv6 без начального уведомления, обратный вызов через `windows.NewCallback`, отмена `CancelMibChangeNotify2`; пачка уведомлений сводится в один вызов `onChange` через 2 секунды после последнего уведомления пачки (чистый тип `debounce`, времена ему передаются, тест идёт по часам теста). Питание: скрытое окно пакета `session`, которое уже получает `WM_ENDSESSION`, обрабатывает и `WM_POWERBROADCAST`: `PBT_APMSUSPEND` — событие `suspend`, `PBT_APMRESUMEAUTOMATIC` и `PBT_APMRESUMESUSPEND` — одно событие `resume` на одно пробуждение (Windows присылает при пробуждении первое всегда, второе — когда вернулся пользователь). `session.Watch` принимает теперь набор обработчиков `Handlers` (`OnEnd`, `OnPower`).

`Engine.NetworkChanged(why)` (`why` — `route` или `resume`; `suspend` пропускается, в счёт идёт пробуждение после него) действует, только пока рабочее ядро работает в `on`, `degraded` или `blocked` и нет операции, которая запускает или останавливает ядро (`routing.allDirect`, `lanes.set` и `systemTailscale.clearExitNode` ядро не трогают и не мешают). Оно запоминает вид соединения до события (`lastPath`, задача 9) и начинает ожидание `netWatch`; новое событие во время ожидания начинает его заново и сохраняет вид соединения до первого события. Своего таймера у ожидания нет: его ведут проверки 5.5 раз в 10 секунд (`checkHealth` вызывает `netRound`), так что тесты идут по часам теста без гонок. Пока после события проверки не прошли (`holds`), неудачи раунда ни к чему не ведут: нет фазы `degraded` и переключения DNS, нет перезапуска ядра при неработающем пути через адаптер; утечка (два ответа подряд с адреса прямой дорожки) по-прежнему даёт откат сразу. Порядок:

1. Пока нет маршрута по умолчанию (раунд смотрит `Host.HasDefaultRoute()` в начале), ядро не трогается сколько угодно долго.
2. Когда маршрут есть, начинается отсчёт 60 секунд (с события, если маршрут уже был, иначе с раунда, нашедшего его). Раунд, в котором прошли `probe-vpn` и путь через адаптер (в `blocked` — только `probe-vpn`), заканчивает ожидание. Через 60 секунд без такого раунда ядро перезапускается: откат 5.4 (возврат и подтверждение системного exit node, остановка ядра), затем включение 5.3 (`beginRestartLocked`, в журнале `network changed and the checks did not pass in 60 s; restarting the core`).
3. Если маршрут снова пропал, всё возвращается к шагу 1, и 60 секунд отсчитываются заново после его возврата (Review Focus 2).
4. Если проверки прошли, а вид соединения до события был `direct`, вид читается в каждом раунде (`readPath`, раз в 10 секунд) до 60 секунд: `direct` заканчивает ожидание, `relay` через 60 секунд — тот же перезапуск (`the path to the exit node stayed relay after the network change; restarting the core`).

Решения задачи. Раунд, в котором не ответил и `probe-direct`, по 5.5 означает отсутствие сети или отказ сервиса проверки: отсчёт 60 секунд начинается заново, перезапуска нет; иначе смена сети при недоступном сервисе адресов или в сети со страницей входа провайдера вела бы к перезапуску, включение не доказало бы туннель, и после трёх попыток деление ушло бы в `error`. В фазе `blocked` и при истёкшем входе перезапуска нет: шаг 6 включения выключил бы exit node, который пользователь выбрал сам (5.7), а без входа включение не проходит (раздел 7); проверки 5.5 идут дальше как без смены сети. Причина перезапуска — `core_unresponsive` (видна, только если все три попытки 5.5 не удались). Свои изменения маршрутов (адаптер ядра, шаг 6 включения) приходят через 2 секунды и могут начать короткое ожидание сразу после фазы `on`; его заканчивает первый удачный раунд. `golang.org/x/sys` v0.49.0 уже содержит `NotifyRouteChange2` и `CancelMibChangeNotify2` (ленивая загрузка `iphlpapi.dll` через `NewLazySystemDLL` внутри пакета), поэтому своего описания процедур нет. На процесс регистрируется одно уведомление: обратный вызов создаётся один раз и только будит всех наблюдателей, последний `stop` отменяет регистрацию.

`main` передаёт события питания в `NetworkChanged` в отдельной горутине (процедура окна должна ответить сразу), изменения маршрутов — напрямую из горутины наблюдателя, и останавливает наблюдатель при выходе. Задача дописывает в спецификацию (раздел 10) решение о перезапуске при `relay`.

**Files:**
- Create: `helper/internal/engine/network.go`, `helper/internal/winhost/netwatch.go`
- Modify: `helper/internal/engine/engine.go`, `helper/internal/engine/supervise.go`, `helper/internal/session/session.go`, `helper/cmd/splitlane-helper/main.go`, `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`
- Test: `helper/internal/engine/network_test.go`, `helper/internal/winhost/netwatch_test.go`, `helper/internal/session/session_test.go`

**Interfaces:**
- Consumes: `Host.HasDefaultRoute`, `protocol.PathDirect`, `protocol.PathRelay`, `protocol.PhaseBlocked`, `protocol.ReasonCoreUnresponsive`, `Engine.SessionEnding`, `windows.NotifyRouteChange2`, `windows.CancelMibChangeNotify2`, `windows.NewCallback`, `windows.AF_UNSPEC`; внутри пакета `engine` — из задачи 9 `lastPath`, `readPath`, `loginExpired`, из плана 1A `checkHealth`, `beginRestartLocked`, `finish`, `finishOn`, `live`, `healthEvery`; в тестах — `newHarness`, `logEvents`, `startEnable`, `inPhase`, `holdOp`, `rounds`, `inOrder`, `countSince`, `publishedCount`, `wantError`, `systemExitNode`, `fakeNode.set`, `fakeHost.noRoute`, `fakeCore.onStart`.
- Produces: `func winhost.WatchRoutes(onChange func(), log func(string, ...any)) (stop func(), err error)`; `type session.Handlers struct { OnEnd func(); OnPower func(event string) }`; `func session.Watch(h Handlers, log func(string, ...any)) (*Watcher, error)`; `func (e *Engine) NetworkChanged(why string)`. Внутри пакетов: `const netWait = 60 * time.Second`, `type netWatch struct { before string; routeAt, pathFrom time.Time }`, `func (w *netWatch) holds() bool`, `func (e *Engine) followsNetworkLocked() bool`, `func (e *Engine) netRound(w *netWatch, route, passed, online bool, path string) bool`, `func (e *Engine) netRestartLocked(cause string) bool`, поле `Engine.net *netWatch` (сбрасывается в `finish` и `finishOn`); в `winhost` — `const routeQuiet = 2 * time.Second`, `type debounce` с методами `note`, `wait`, `over`; в `session` — `wmPowerBroadcast`, `pbtAPMSuspend`, `pbtAPMResumeSuspend`, `pbtAPMResumeAutomatic`. В тестах: `netOn`, `ticks`, `routeUp`, `following`, `healthyCores`, `restartedAfter`, `logChecksFailed`, `logStayedRelay`.

- [ ] **Step 1: Написать тесты смены сети в автомате**

Помощники: `netOn` — фаза `on` после 5.3 с журналом в событиях; `ticks` — раунды 5.5 через 10 секунд по часам теста; `routeUp` — маршрут по умолчанию есть или нет; `healthyCores` — каждое новое ядро проходит все проверки; `restartedAfter` — перезапуск 5.4, затем 5.3 после строки журнала.

Файл `helper/internal/engine/network_test.go`, целиком:

```go
package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

// The log lines of the restarts after a network change (section 10).
const (
	logChecksFailed = "network changed and the checks did not pass in 60 s; restarting the core"
	logStayedRelay  = "the path to the exit node stayed relay after the network change; restarting the core"
)

// netOn is a helper in phase on after 5.3 whose log lines are events "log:<line>", with the path to
// the exit node read after phase on: direct.
func netOn(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	logEvents(h)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	return h
}

// ticks runs n rounds of 5.5 ten seconds apart on the clock of the test, the first one 10 s from now.
func ticks(h *harness, n int) {
	for i := 0; i < n; i++ {
		h.clock.After(healthEvery)
		h.e.checkHealth(context.Background())
	}
}

// routeUp gives the computer its default route back, or takes it away until the next call.
func routeUp(h *harness, up bool) {
	h.host.mu.Lock()
	defer h.host.mu.Unlock()
	h.host.noRoute = 0
	if !up {
		h.host.noRoute = -1
	}
}

// following tells whether the helper follows a network change now.
func following(h *harness) bool {
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return h.e.net != nil
}

// healthyCores makes every core that starts from now on pass all checks: probe-vpn, probe-direct and
// the path through the adapter.
func healthyCores(h *harness) {
	h.core.mu.Lock()
	defer h.core.mu.Unlock()
	h.core.onStart = func(*fakeProc) {
		h.probe.script([]ProbeResult{atVPN}, []ProbeResult{atDirect}, []ProbeResult{atVPN})
	}
}

// restartedAfter waits for the restart that a network change led to, logged as why: the rollback of
// 5.4 (the system exit node given back, the old core stopped), then the enable of 5.3 up to phase on.
// No other phase came between: neither degraded nor error.
func restartedAfter(t *testing.T, h *harness, old *fakeProc, mark int, why string) {
	t.Helper()
	h.waitFor("the restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	order := []string{"log:" + why, "sys:set " + exitNode + " true", fmt.Sprintf("core:interrupt %d", old.id.PID),
		fmt.Sprintf("core:start %d", old.id.PID+1)}
	if !inOrder(h, mark, order...) || h.rec.count("log:"+why) != 1 || old.alive() {
		t.Fatalf("restore, stop, start after %q: %v", why, h.rec.list()[mark:])
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Section 10, the row «Сон и пробуждение, смена сети»: after a network change (here the wake from
// sleep) the helper waits for the default route; while there is none it leaves the core alone, however
// long: the failures of the rounds of 5.5 lead neither to degraded with the provider's DNS (probe-vpn
// silent while probe-direct answers) nor to the restart of a core whose path through the adapter fails
// while probe-vpn works. The route back and a round that passes end the wait (the path to the exit
// node, direct before, is read once more: direct); then the rounds act again.
func TestNoRouteHoldsTheCore(t *testing.T) {
	h := netOn(t)
	core := h.core.last()
	mark := len(h.rec.list())
	routeUp(h, false)
	h.e.NetworkChanged("resume")
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	ticks(h, 9)
	h.probe.script([]ProbeResult{atVPN}, nil, nil)
	ticks(h, 9)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || !core.alive() || countSince(h, mark, "core:") != 0 ||
		countSince(h, mark, "sys:") != 0 || countSince(h, mark, "clash:") != 0 || countSince(h, mark, "dns:") != 0 {
		t.Fatalf("no route: the core is left alone: phase %s, events %v", s.Phase, h.rec.list()[mark:])
	}
	routeUp(h, true)
	h.probe.script(nil, nil, []ProbeResult{atVPN})
	ticks(h, 2)
	if following(h) || h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("the route back and a round that passes end the wait: %v", h.rec.list()[mark:])
	}
	h.probe.script([]ProbeResult{noAnswer}, nil, []ProbeResult{noAnswer})
	ticks(h, 3)
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || s.Reason == nil || s.Reason.Code != protocol.ReasonVPNUnreachable {
		t.Fatalf("after the wait the rounds act again: %+v", s)
	}
}

// Section 10: once the network is back the helper waits up to 60 s for a round of 5.5 in which
// probe-vpn and the path through the adapter pass; meanwhile the failures lead to nothing (no degraded,
// no switch of the DNS). A round in which probe-direct does not answer either finds no network (5.5):
// the 60 s start again. At 60 s with the network there the core is restarted: the rollback of 5.4 (the
// system exit node given back and confirmed, the core stopped), then the enable of 5.3.
func TestChecksFailAfterNetworkChangeRestart(t *testing.T) {
	h := netOn(t)
	healthyCores(h)
	old := h.core.last()
	mark := len(h.rec.list())
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	h.e.NetworkChanged("route")
	ticks(h, 9)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("a route but no answer through probe-direct either is no network: %v", h.rec.list()[mark:])
	}
	h.probe.script(nil, []ProbeResult{atDirect}, nil)
	ticks(h, 5)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 || countSince(h, mark, "clash:") != 0 {
		t.Fatalf("within 60 s of the network nothing happens: phase %s, events %v", s.Phase, h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logChecksFailed)
}

// Review Focus 2: the network goes again during the 60 s of the wait. While there is no default route
// nothing happens, however long (probe-direct answers here, so that only the missing route holds the
// core); the 60 s start again with the round that finds the route back.
func TestFlappingNetworkWaitsAgain(t *testing.T) {
	h := netOn(t)
	healthyCores(h)
	old := h.core.last()
	mark := len(h.rec.list())
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	h.e.NetworkChanged("route")
	ticks(h, 3)
	routeUp(h, false)
	ticks(h, 6)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("90 s after the route came back first, but no route now: %v", h.rec.list()[mark:])
	}
	routeUp(h, true)
	ticks(h, 6)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("50 s after the route came back again: %v", h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logChecksFailed)
}

// The decision of plan 1B (appendix A, checks of plan 1B, (и): after a cable change the embedded node
// stays on a relay, while a new core gets the direct path at once). Once the checks pass after a
// network change, a path to the exit node that was direct before it is read every 10 s: direct again
// within 60 s keeps the core, relay at 60 s restarts it (5.4, then 5.3). A second change during the
// wait starts it again and keeps the path from before the first one.
func TestRelayAfterNetworkChangeRestarts(t *testing.T) {
	h := netOn(t)
	old := h.core.last()
	mark := len(h.rec.list())
	path := func(p string) { h.core.node.set(func(n *fakeNode) { n.path = p }) }
	path(protocol.PathRelay)
	h.e.NetworkChanged("route")
	ticks(h, 4) // the checks pass, then relay for 30 s
	path(protocol.PathDirect)
	ticks(h, 1)
	if following(h) || h.rec.count("core:start") != 1 {
		t.Fatalf("direct again within 60 s: the core stays: %v", h.rec.list()[mark:])
	}

	h.e.NetworkChanged("route")
	path(protocol.PathRelay)
	h.e.refreshNode(context.Background()) // the look every 60 s reads relay meanwhile
	if p, _ := h.e.lastPath(); p != protocol.PathRelay {
		t.Fatalf("setup: lastPath %q", p)
	}
	h.e.NetworkChanged("route")
	ticks(h, 6) // the checks pass, then relay for 50 s
	if h.rec.count("core:start") != 1 {
		t.Fatalf("relay for 50 s: the core stays: %v", h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logStayedRelay)
}

// The restart for relay is only for a path that was direct before the network change: a path that was
// relay or unknown before it is not watched once the checks pass, and the core stays.
func TestRelayWithoutDirectBeforeKeepsTheCore(t *testing.T) {
	for _, before := range []string{protocol.PathRelay, ""} {
		t.Run("before="+before, func(t *testing.T) {
			h := netOn(t)
			h.core.node.set(func(n *fakeNode) { n.path = before })
			h.e.readPath(context.Background())
			h.core.node.set(func(n *fakeNode) { n.path = protocol.PathRelay })
			mark := len(h.rec.list())
			h.e.NetworkChanged("route")
			ticks(h, 1)
			if following(h) {
				t.Fatal("the checks passed and the path before was not direct: the network change is over")
			}
			ticks(h, 8)
			if s := h.e.State(); s.Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 || countSince(h, mark, "sys:") != 0 {
				t.Fatalf("the core stays: phase %s, events %v", s.Phase, h.rec.list()[mark:])
			}
		})
	}
}

// While the helper waits for the default route, a leak (two answers in a row from the address of the
// direct lane through the adapter, 5.5) still rolls back at once: 5.4, error leak_detected.
func TestLeakStillRollsBackDuringTheWait(t *testing.T) {
	h := netOn(t)
	routeUp(h, false)
	h.e.NetworkChanged("route")
	h.probe.script(nil, nil, []ProbeResult{atDirect})
	ticks(h, 1)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("one answer from the direct address is no leak: %v", h.rec.list())
	}
	ticks(h, 1)
	wantError(t, h, protocol.ReasonLeakDetected)
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.core.anyAlive() || following(h) {
		t.Fatalf("the system exit node back, no core: %v", h.rec.list())
	}
}

// A network change is followed only while the core runs in on, degraded or blocked and no operation
// starts or stops it; "suspend" is never followed (the wake after it is).
func TestNetworkChangeWhenOffDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.load()
	states := publishedCount(h)
	for _, why := range []string{"route", "resume", "suspend"} {
		h.e.NetworkChanged(why)
	}
	if following(h) || publishedCount(h) != states || len(h.rec.list()) != 0 {
		t.Fatalf("off: nothing happens: %v", h.rec.list())
	}

	h, release := inPhase(t, protocol.PhaseStarting)
	h.e.NetworkChanged("route")
	if following(h) {
		t.Fatal("an enable under way is not followed")
	}
	release()

	h = netOn(t)
	h.e.NetworkChanged("suspend")
	done := holdOp(h) // the enable that published phase on is still in place
	h.e.NetworkChanged("route")
	done()
	if following(h) {
		t.Fatal("suspend, and a change during an operation that starts the core, are not followed")
	}
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("nothing holds the rounds: %+v", h.e.State())
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'Test(NoRouteHoldsTheCore|ChecksFailAfterNetworkChangeRestart|FlappingNetworkWaitsAgain|RelayAfterNetworkChangeRestarts|RelayWithoutDirectBeforeKeepsTheCore|LeakStillRollsBackDuringTheWait|NetworkChangeWhenOffDoesNothing)$' -count=1
```

Ожидается: тесты не собираются, у автомата нет `NetworkChanged` и поля `net`:

```text
# splitlane/helper/internal/engine [splitlane/helper/internal/engine.test]
internal\engine\network_test.go:53:13: h.e.net undefined (type *Engine has no field or method net)
internal\engine\network_test.go:95:6: h.e.NetworkChanged undefined (type *Engine has no field or method NetworkChanged)
...
FAIL	splitlane/helper/internal/engine [build failed]
```

- [ ] **Step 3: Ожидание сети и проверок после события**

Файл `helper/internal/engine/network.go`, целиком:

```go
package engine

import (
	"time"

	"splitlane/helper/internal/protocol"
)

// netWait: after a network change the checks of 5.5 have so long to pass once the network is back, and
// then a path to the exit node that was direct before the change so long to be direct again (section 10).
const netWait = 60 * time.Second

// netWatch is the sequence after a network change (section 10), driven by the rounds of 5.5 (netRound);
// guarded by Engine.mu. Until the checks pass, the failures of a round count for nothing: no degraded,
// no restart; a leak still rolls back at once.
type netWatch struct {
	before   string    // the path to the exit node before the first change of the sequence (lastPath)
	routeAt  time.Time // the 60 s of the checks count from here; zero while there is no default route
	pathFrom time.Time // the checks passed here, and the path is read from here on; zero before
}

// holds: the checks have not passed since the change; the failures of a round count for nothing.
func (w *netWatch) holds() bool { return w.pathFrom.IsZero() }

// NetworkChanged is a change of the network that Windows reports (section 10): why is "route" for a burst
// of route changes (winhost.WatchRoutes) and "resume" for the wake from sleep (session); "suspend" is
// ignored, the wake after it counts. It is followed only while the main core runs in on, degraded or
// blocked and no operation starts or stops it: the core resets its connections itself. The path to the
// exit node read last is kept as the path before the change; a change while one is followed starts the
// wait again and keeps the path from before the first. It returns at once: the rounds of 5.5 follow the
// network from here (netRound).
func (e *Engine) NetworkChanged(why string) {
	e.mu.Lock()
	ok := e.followsNetworkLocked()
	e.mu.Unlock()
	if why == "suspend" || !ok {
		return
	}
	before, _ := e.lastPath()
	route := e.d.Host.HasDefaultRoute()
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.followsNetworkLocked() {
		return
	}
	w, again := &netWatch{before: before}, ""
	if e.net != nil {
		w.before, again = e.net.before, " again"
	}
	e.net = w
	if !route {
		e.d.Log("network changed%s (%s): no default route; the core is left alone until it is back", again, why)
		return
	}
	w.routeAt = e.d.Clock.Now()
	e.d.Log("network changed%s (%s): waiting up to %s for the checks of 5.5", again, why, netWait)
}

// followsNetworkLocked: a network change is followed now; e.mu must be held. Routing switches, lanes.set
// and systemTailscale.clearExitNode leave the core running; every other operation starts or stops it.
func (e *Engine) followsNetworkLocked() bool {
	r := e.run
	if r == nil || r.stopping || !live(e.st.Phase) || e.exiting || e.ending {
		return false
	}
	return e.op == nil || e.op.kind == "alldirect" || e.op.kind == "lanes" || e.op.kind == "clear"
}

// netRound is the part of a round of 5.5 that follows a network change (section 10); e.mu must be held.
// route: the default route was there when the round began; passed: probe-vpn and the path through the
// adapter passed (in blocked probe-vpn alone, 5.5); online: probe-direct answered; path: the path to the
// exit node read in this round once the checks have passed. True when a restart began.
//  1. Without a default route nothing counts, however long; the 60 s start when it is back, again after
//     each time it went.
//  2. A round that passes within the 60 s ends the wait. A round in which probe-direct does not answer
//     either finds no network (5.5): the 60 s start again. At 60 s the core is restarted: 5.4, then 5.3.
//  3. After the wait a path that was direct before the change is read every round (10 s): direct ends
//     the sequence, relay at 60 s restarts the core: the embedded node does not get the direct path back
//     by itself, a new core does (appendix A, checks of plan 1B).
func (e *Engine) netRound(w *netWatch, route, passed, online bool, path string) bool {
	now := e.d.Clock.Now()
	if !w.holds() {
		switch {
		case path == protocol.PathDirect:
			e.d.Log("the path to the exit node is direct again after the network change")
			e.net = nil
		case !now.Before(w.pathFrom.Add(netWait)):
			e.net = nil
			if path == protocol.PathRelay {
				return e.netRestartLocked("the path to the exit node stayed relay after the network change")
			}
			e.d.Log("the path to the exit node is unknown %s after the network change; the core stays", netWait)
		}
		return false
	}
	switch {
	case !route:
		if !w.routeAt.IsZero() {
			e.d.Log("the default route is gone again; the core is left alone until it is back")
		}
		w.routeAt = time.Time{}
		return false
	case w.routeAt.IsZero():
		e.d.Log("the default route is back: waiting up to %s for the checks of 5.5", netWait)
		w.routeAt = now
	}
	switch {
	case passed && w.before == protocol.PathDirect:
		e.d.Log("the checks passed after the network change; the path to the exit node was direct before it and is read for %s", netWait)
		w.pathFrom = now
	case passed:
		e.d.Log("the checks passed after the network change")
		e.net = nil
	case !online:
		w.routeAt = now // probe-direct is silent too: no network (5.5), the 60 s start again
	case !now.Before(w.routeAt.Add(netWait)):
		e.net = nil
		return e.netRestartLocked("network changed and the checks did not pass in 60 s")
	}
	return false
}

// netRestartLocked is the restart after a network change (5.5: 5.4, then 5.3), logged with its cause;
// e.mu must be held. There is none in blocked: step 6 of the enable would switch off the exit node the
// user selected (5.7); and none with the login expired: no enable passes before the login (spec 7). The
// rounds of 5.5 then go on as they do without a network change.
func (e *Engine) netRestartLocked(cause string) bool {
	switch {
	case e.st.Phase == protocol.PhaseBlocked:
		e.d.Log("%s; no restart in phase blocked: the exit node the user selected stays", cause)
		return false
	case e.loginExpired():
		e.d.Log("%s; no restart while the login is expired", cause)
		return false
	}
	e.d.Log("%s; restarting the core", cause)
	e.beginRestartLocked(protocol.ReasonCoreUnresponsive)
	return true
}
```

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
	adapterFails int
	leakHits     int
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
```

Заменить на:

```go
	adapterFails int
	leakHits     int
	net          *netWatch    // the sequence after a network change (section 10, network.go); nil when none runs
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
```

Найти:

```go
// finish ends in off or error: "was on" cleared, all direct reset (5.2); e.mu must be held.
func (e *Engine) finish(phase string, why *protocol.Reason) {
	e.saved.WasOn = false
	e.save()
	e.st.AllDirect = false
	e.st.Addresses = protocol.Addresses{}
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.setPhase(phase, why)
}

// finishOn is step 9 of 5.3: phase on, "was on" set; e.mu must be held.
func (e *Engine) finishOn() {
	e.saved.WasOn = true
	e.save()
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.setPhase(protocol.PhaseOn, nil)
}
```

Заменить на:

```go
// finish ends in off or error: "was on" cleared, all direct reset (5.2); e.mu must be held.
// A network change that was followed is over with the core.
func (e *Engine) finish(phase string, why *protocol.Reason) {
	e.saved.WasOn = false
	e.save()
	e.st.AllDirect = false
	e.st.Addresses = protocol.Addresses{}
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.net = nil
	e.setPhase(phase, why)
}

// finishOn is step 9 of 5.3: phase on, "was on" set; e.mu must be held. A network change that was
// followed is over: the core is a new one.
func (e *Engine) finishOn() {
	e.saved.WasOn = true
	e.save()
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.net = nil
	e.setPhase(protocol.PhaseOn, nil)
}
```

Изменить `helper/internal/engine/supervise.go`:

Найти:

```go
// a new address of the provider (DHCP, CGNAT, another network) does not hide a leak.
func (e *Engine) checkHealth(ctx context.Context) {
	e.mu.Lock()
	r, phase, url := e.run, e.st.Phase, e.saved.ProbeURL
	idle := e.op == nil && !e.exiting && !e.ending
	e.mu.Unlock()
	if r == nil || !idle || !live(phase) {
		return
	}
	adapter := phase != protocol.PhaseBlocked
	var v, d, a ProbeResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); v = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN) }()
	go func() { defer wg.Done(); d = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect) }()
	if adapter {
		wg.Add(1)
		go func() { defer wg.Done(); a = e.d.Probe.Direct(ctx, url) }()
	}
	wg.Wait()
```

Заменить на:

```go
// a new address of the provider (DHCP, CGNAT, another network) does not hide a leak.
// After a network change (section 10, netRound) the round also looks at the default route while the
// checks have not passed, and reads the path to the exit node once they have; until they pass, the
// failures count for nothing (no degraded, no restart), and a leak still rolls back.
func (e *Engine) checkHealth(ctx context.Context) {
	e.mu.Lock()
	r, phase, url, w := e.run, e.st.Phase, e.saved.ProbeURL, e.net
	idle := e.op == nil && !e.exiting && !e.ending
	waiting, watchPath := w != nil && w.holds(), w != nil && !w.holds()
	e.mu.Unlock()
	if r == nil || !idle || !live(phase) {
		return
	}
	adapter := phase != protocol.PhaseBlocked
	route := !waiting || e.d.Host.HasDefaultRoute()
	var v, d, a ProbeResult
	var path string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); v = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN) }()
	go func() { defer wg.Done(); d = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect) }()
	if adapter {
		wg.Add(1)
		go func() { defer wg.Done(); a = e.d.Probe.Direct(ctx, url) }()
	}
	if watchPath {
		wg.Add(1)
		go func() { defer wg.Done(); path = e.readPath(ctx) }()
	}
	wg.Wait()
```

Найти:

```go
	dirty := false
	// An answer of probe-direct equal to probe-vpn's is not the direct lane (the system exit node
	// is on again, 5.6): the stored direct address stays.
	if d.IP != "" && d.IP != v.IP && (e.st.Addresses.Direct == nil || *e.st.Addresses.Direct != d.IP) {
		ip := d.IP
		e.st.Addresses.Direct, dirty = &ip, true
	}
	switch {
	case v.Reached:
		e.vpnFails = 0
		if e.degraded {
			e.degraded, dirty = false, true // one good check of probe-vpn is enough to return to on
		}
		if v.IP != "" && (e.st.Addresses.VPN == nil || *e.st.Addresses.VPN != v.IP) {
			ip := v.IP
			e.st.Addresses.VPN, dirty = &ip, true
		}
	case d.Reached:
		e.vpnFails++
		if e.vpnFails >= 3 && !e.degraded {
			e.degraded, dirty = true, true
		}
	}
	if adapter {
		direct := e.st.Addresses.Direct // this round's when probe-direct answered, else the last one
		switch {
		case a.IP != "" && direct != nil && a.IP == *direct:
			e.leakHits, e.adapterFails = e.leakHits+1, 0
		case a.Reached:
			e.adapterFails = 0
			if a.IP != "" {
				e.leakHits = 0
			}
		case v.Reached:
			e.adapterFails++
		}
```

Заменить на:

```go
	dirty := false
	hold := e.net != nil && e.net.holds() // since a network change the checks have not passed
	// An answer of probe-direct equal to probe-vpn's is not the direct lane (the system exit node
	// is on again, 5.6): the stored direct address stays.
	if d.IP != "" && d.IP != v.IP && (e.st.Addresses.Direct == nil || *e.st.Addresses.Direct != d.IP) {
		ip := d.IP
		e.st.Addresses.Direct, dirty = &ip, true
	}
	switch {
	case v.Reached:
		e.vpnFails = 0
		if e.degraded {
			e.degraded, dirty = false, true // one good check of probe-vpn is enough to return to on
		}
		if v.IP != "" && (e.st.Addresses.VPN == nil || *e.st.Addresses.VPN != v.IP) {
			ip := v.IP
			e.st.Addresses.VPN, dirty = &ip, true
		}
	case d.Reached && !hold:
		e.vpnFails++
		if e.vpnFails >= 3 && !e.degraded {
			e.degraded, dirty = true, true
		}
	}
	passed := v.Reached && !adapter // in blocked probe-vpn alone (5.5)
	if adapter {
		direct := e.st.Addresses.Direct // this round's when probe-direct answered, else the last one
		switch {
		case a.IP != "" && direct != nil && a.IP == *direct:
			e.leakHits, e.adapterFails = e.leakHits+1, 0
		case a.Reached:
			e.adapterFails = 0
			if a.IP != "" {
				e.leakHits = 0
			}
			passed = v.Reached
		case v.Reached && !hold:
			e.adapterFails++
		}
```

Найти:

```go
		if e.adapterFails >= 3 {
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.beginRestartLocked(protocol.ReasonCoreUnresponsive)
			e.mu.Unlock()
			return
		}
	}
	now := e.vpnDown()
```

Заменить на:

```go
		if e.adapterFails >= 3 {
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.beginRestartLocked(protocol.ReasonCoreUnresponsive)
			e.mu.Unlock()
			return
		}
	}
	// A change that came during the round starts its wait from the next round on.
	if w != nil && e.net == w && e.netRound(w, route, passed, d.Reached, path) {
		e.mu.Unlock()
		return
	}
	now := e.vpnDown()
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/engine/; go test ./internal/engine/ -run 'Test(NoRouteHoldsTheCore|ChecksFailAfterNetworkChangeRestart|FlappingNetworkWaitsAgain|RelayAfterNetworkChangeRestarts|RelayWithoutDirectBeforeKeepsTheCore|LeakStillRollsBackDuringTheWait|NetworkChangeWhenOffDoesNothing)$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./internal/engine/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestNoRouteHoldsTheCore`, `--- PASS: TestChecksFailAfterNetworkChangeRestart`, `--- PASS: TestFlappingNetworkWaitsAgain`, `--- PASS: TestRelayAfterNetworkChangeRestarts`, `--- PASS: TestRelayWithoutDirectBeforeKeepsTheCore`, `--- PASS: TestLeakStillRollsBackDuringTheWait`, `--- PASS: TestNetworkChangeWhenOffDoesNothing`, затем `ok  	splitlane/helper/internal/engine` для выбранных тестов и для пакета целиком (тесты 5.5 без смены сети проходят без изменений: без события раунд ведёт себя как прежде).

- [ ] **Step 5: Написать тест сведения уведомлений о маршрутах**

`TestRouteDebounce` проверяет чистый тип по часам теста. `TestLiveRouteWatchReadOnly` регистрирует настоящее уведомление и отменяет его, дважды; систему он не меняет и без `SPLITLANE_LIVE_READONLY=1` пропускается.

Файл `helper/internal/winhost/netwatch_test.go`, целиком:

```go
package winhost

import (
	"os"
	"testing"
	"time"
)

// A burst of route notifications is reported once, 2 s after its last notification; the times are
// those of a clock of the test.
func TestRouteDebounce(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	d := debounce{quiet: routeQuiet}
	if _, ok := d.wait(t0); ok || d.over(t0) {
		t.Fatal("no notification yet: nothing to wait for, nothing to report")
	}
	d.note(at(0))
	d.note(at(500))
	d.note(at(1900))
	if w, ok := d.wait(at(2000)); !ok || w != 1900*time.Millisecond {
		t.Fatalf("2 s after the last notification: wait %s %v", w, ok)
	}
	if d.over(at(3899)) {
		t.Fatal("reported before 2 s passed since the last notification")
	}
	if !d.over(at(3900)) {
		t.Fatal("the burst is over 2 s after its last notification")
	}
	if _, ok := d.wait(at(5000)); ok || d.over(at(10000)) {
		t.Fatal("a burst is reported once")
	}
	d.note(at(20000))
	if w, ok := d.wait(at(25000)); !ok || w != 0 || !d.over(at(25000)) {
		t.Fatalf("a late look reports the next burst at once: wait %s %v", w, ok)
	}
}

// TestLiveRouteWatchReadOnly registers the route notification of Windows and cancels it, twice; it
// changes nothing. It runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveRouteWatchReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	for i := 0; i < 2; i++ {
		stop, err := WatchRoutes(func() { t.Log("LIVE routes changed") }, t.Logf)
		if err != nil {
			t.Fatalf("watch %d: %v", i, err)
		}
		routeMu.Lock()
		registered := routeHandle != 0 && routeCount == 1
		routeMu.Unlock()
		stop()
		stop()
		routeMu.Lock()
		cancelled := routeHandle == 0 && routeCount == 0
		routeMu.Unlock()
		if !registered || !cancelled {
			t.Fatalf("watch %d: registered %v, cancelled %v", i, registered, cancelled)
		}
	}
	t.Log("LIVE route notification registered and cancelled twice")
}
```

- [ ] **Step 6: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/winhost/ -run 'Test(RouteDebounce|LiveRouteWatchReadOnly)$' -count=1
```

Ожидается:

```text
# splitlane/helper/internal/winhost [splitlane/helper/internal/winhost.test]
internal\winhost\netwatch_test.go:14:7: undefined: debounce
internal\winhost\netwatch_test.go:14:23: undefined: routeQuiet
internal\winhost\netwatch_test.go:46:16: undefined: WatchRoutes
...
FAIL	splitlane/helper/internal/winhost [build failed]
```

- [ ] **Step 7: Уведомления об изменении маршрутов**

Файл `helper/internal/winhost/netwatch.go`, целиком:

```go
package winhost

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// routeQuiet: a burst of route notifications is over this long after its last notification.
const routeQuiet = 2 * time.Second

// debounce coalesces a burst of route notifications: it is over once quiet has passed since its last
// notification, and then it is reported once. It keeps no clock: the times come from the caller.
type debounce struct {
	quiet time.Duration
	last  time.Time // the last notification of the burst under way; zero when none is under way
}

// note records a notification at now.
func (d *debounce) note(now time.Time) { d.last = now }

// wait is how long after now the burst under way may be over; false when none is under way.
func (d *debounce) wait(now time.Time) (time.Duration, bool) {
	if d.last.IsZero() {
		return 0, false
	}
	return max(d.last.Add(d.quiet).Sub(now), 0), true
}

// over reports, once per burst, that the burst under way is over at now.
func (d *debounce) over(now time.Time) bool {
	if d.last.IsZero() || now.Before(d.last.Add(d.quiet)) {
		return false
	}
	d.last = time.Time{}
	return true
}

// One notification of route changes serves every watcher of the process: Windows calls routeChanged on
// a thread of its own, which only wakes the watchers. NewCallback is called once: a process has only so
// many callbacks.
var (
	routeCallback = windows.NewCallback(routeChanged)
	routeMu       sync.Mutex     // registers and cancels the notification; the callback never takes it
	routeHandle   windows.Handle // the notification of NotifyRouteChange2; 0 while no watcher runs
	routeCount    int            // the watchers that use routeHandle
	routeWatchers sync.Map       // *routeWatch -> struct{}: every watcher the callback wakes
)

// routeChanged is the callback of NotifyRouteChange2 (callerContext, the changed MIB_IPFORWARD_ROW2 and the
// kind of change are not used): any change of a route, IPv4 or IPv6, wakes every watcher.
func routeChanged(callerContext, row, notificationType uintptr) uintptr {
	routeWatchers.Range(func(w, _ any) bool {
		w.(*routeWatch).kick()
		return true
	})
	return 0
}

// routeWatch is one watcher: its goroutine (loop) turns the wakes into one call of onChange per burst.
type routeWatch struct {
	kicks chan struct{} // a notification came; one pending wake is enough
	done  chan struct{} // closed by stop
}

func (w *routeWatch) kick() {
	select {
	case w.kicks <- struct{}{}:
	default:
	}
}

func (w *routeWatch) loop(onChange func(), log func(string, ...any)) {
	d := debounce{quiet: routeQuiet}
	timer := time.NewTimer(routeQuiet)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-w.kicks:
			d.note(time.Now())
		case <-timer.C:
			if d.over(time.Now()) {
				log("routes changed")
				onChange()
			}
		}
		if wait, ok := d.wait(time.Now()); ok {
			timer.Reset(wait)
		}
	}
}

// WatchRoutes calls onChange when the routes of Windows change (section 10): once per burst, 2 s after
// its last notification, on a goroutine of its own. The notification is NotifyRouteChange2 of
// iphlpapi.dll for IPv4 and IPv6, without the initial notification; stop ends the watcher and, with the
// last watcher, cancels the notification (CancelMibChangeNotify2). stop may be called more than once.
// Only the notification is registered: nothing in the system changes.
func WatchRoutes(onChange func(), log func(string, ...any)) (stop func(), err error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	w := &routeWatch{kicks: make(chan struct{}, 1), done: make(chan struct{})}
	routeMu.Lock()
	defer routeMu.Unlock()
	if routeCount == 0 {
		var h windows.Handle
		if err := windows.NotifyRouteChange2(windows.AF_UNSPEC, routeCallback, nil, false, &h); err != nil {
			return nil, fmt.Errorf("NotifyRouteChange2: %w", err)
		}
		routeHandle = h
	}
	routeCount++
	routeWatchers.Store(w, struct{}{})
	go w.loop(onChange, log)
	var once sync.Once
	return func() {
		once.Do(func() {
			routeWatchers.Delete(w)
			close(w.done)
			routeMu.Lock()
			defer routeMu.Unlock()
			routeCount--
			if routeCount == 0 {
				if err := windows.CancelMibChangeNotify2(routeHandle); err != nil {
					log("route watch: CancelMibChangeNotify2: %v", err)
				}
				routeHandle = 0
			}
		})
	}, nil
}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

Второй прогон читает систему: регистрирует уведомление и отменяет его (разрешено Global Constraints с `SPLITLANE_LIVE_READONLY=1`; ничего не меняет).

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/winhost/; go test ./internal/winhost/ -run 'Test(RouteDebounce|LiveRouteWatchReadOnly)$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; $env:SPLITLANE_LIVE_READONLY='1'; go test ./internal/winhost/ -run 'TestLiveRouteWatchReadOnly$' -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL|\s+netwatch)'; Remove-Item Env:SPLITLANE_LIVE_READONLY; go test ./internal/winhost/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestRouteDebounce`, `--- SKIP: TestLiveRouteWatchReadOnly`, `ok  	splitlane/helper/internal/winhost`; затем `netwatch_test.go:62: LIVE route notification registered and cancelled twice`, `--- PASS: TestLiveRouteWatchReadOnly`, `ok  	splitlane/helper/internal/winhost`; затем ещё одно `ok  	splitlane/helper/internal/winhost`.

- [ ] **Step 9: Написать тест событий питания**

Существующие тесты пакета переходят на `Handlers`; новый тест отправляет `WM_POWERBROADCAST` скрытому окну своего процесса, как `TestEndSessionCallsTheHandler` — `WM_ENDSESSION`.

Файл `helper/internal/session/session_test.go`, целиком:

```go
package session

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The hidden window answers WM_QUERYENDSESSION with "yes" and calls the handler on WM_ENDSESSION
// with wParam TRUE only, before the message returns.
func TestEndSessionCallsTheHandler(t *testing.T) {
	var calls atomic.Int32
	w, err := Watch(Handlers{OnEnd: func() { calls.Add(1) }}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	if r := w.send(wmQueryEndSession, 0, 0); r != 1 {
		t.Fatalf("WM_QUERYENDSESSION answered %d", r)
	}
	w.send(wmEndSession, 0, 0) // the session does not end after all
	if calls.Load() != 0 {
		t.Fatal("WM_ENDSESSION with FALSE must not call the handler")
	}
	w.send(wmEndSession, 1, 0)
	if calls.Load() != 1 {
		t.Fatal("WM_ENDSESSION with TRUE must call the handler before it returns")
	}
}

// Section 10: the same hidden window gets WM_POWERBROADCAST. PBT_APMSUSPEND is "suspend"; the wake is
// "resume" once, although Windows sends PBT_APMRESUMEAUTOMATIC and, when the user is back,
// PBT_APMRESUMESUSPEND; other power events are not reported. Every one is answered TRUE.
func TestPowerBroadcastCallsTheHandler(t *testing.T) {
	var mu sync.Mutex
	var got []string
	w, err := Watch(Handlers{OnPower: func(event string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, event)
	}}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	const pbtPowerStatusChange = 0x000A
	for _, event := range []uintptr{pbtAPMSuspend, pbtAPMResumeAutomatic, pbtAPMResumeSuspend, pbtPowerStatusChange,
		pbtAPMSuspend, pbtAPMResumeSuspend} {
		if r := w.send(wmPowerBroadcast, event, 0); r != 1 {
			t.Fatalf("WM_POWERBROADCAST %#x answered %d", event, r)
		}
	}
	w.send(wmEndSession, 1, 0) // no OnEnd: nothing to call
	mu.Lock()
	defer mu.Unlock()
	if s := strings.Join(got, ","); s != "suspend,resume,suspend,resume" {
		t.Fatalf("events %s", s)
	}
}

func TestTwoWatchersAndStop(t *testing.T) {
	a, err := Watch(Handlers{OnEnd: func() {}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Watch(Handlers{OnEnd: func() {}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.Stop()
	b.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, okA := handlers.Load(a.hwnd)
		_, okB := handlers.Load(b.hwnd)
		if !okA && !okB {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("windows not destroyed after Stop")
}
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/session/ -count=1
```

Ожидается:

```text
# splitlane/helper/internal/session [splitlane/helper/internal/session.test]
internal\session\session_test.go:15:18: undefined: Handlers
internal\session\session_test.go:39:18: undefined: Handlers
internal\session\session_test.go:49:34: undefined: pbtAPMSuspend
...
FAIL	splitlane/helper/internal/session [build failed]
```

- [ ] **Step 11: Сон и пробуждение в скрытом окне, подключение в `main`**

Изменить `helper/internal/session/session.go`:

Найти:

```go
// Package session tells the helper that the Windows session ends (logoff, restart, shutdown).
// A process that loaded user32.dll gets no console event for it; it gets WM_QUERYENDSESSION and
// WM_ENDSESSION in a top-level window (spec 5.4). A message-only window does not get them.
package session
```

Заменить на:

```go
// Package session tells the helper that the Windows session ends (logoff, restart, shutdown) and
// that the computer goes to sleep and wakes (section 10). A process that loaded user32.dll gets no
// console event for the end of the session; it gets WM_QUERYENDSESSION and WM_ENDSESSION in a
// top-level window (spec 5.4), and WM_POWERBROADCAST there too. A message-only window gets none of them.
package session
```

Найти:

```go
const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)
```

Заменить на:

```go
const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmPowerBroadcast  = 0x0218
)

// The events of WM_POWERBROADCAST (wParam) that the helper uses.
const (
	pbtAPMSuspend         = 0x0004 // the computer goes to sleep
	pbtAPMResumeSuspend   = 0x0007 // it woke, and the user is there
	pbtAPMResumeAutomatic = 0x0012 // it woke; sent at every wake
)
```

Найти:

```go
type handler struct {
	onEnd func()
	log   func(string, ...any)
}

func proc(hwnd, message, wParam, lParam uintptr) uintptr {
	if v, ok := handlers.Load(hwnd); ok {
		h := v.(*handler)
		switch message {
		case wmQueryEndSession:
			h.log("session: WM_QUERYENDSESSION lParam=%#x", lParam)
			return 1 // the session may end
		case wmEndSession:
			h.log("session: WM_ENDSESSION ending=%v lParam=%#x", wParam != 0, lParam)
			if wParam != 0 {
				h.onEnd()
			}
			return 0
```

Заменить на:

```go
// Handlers are what the hidden window reports; a nil one is not called.
type Handlers struct {
	OnEnd   func()             // WM_ENDSESSION: the session ends; it must finish within a few seconds
	OnPower func(event string) // WM_POWERBROADCAST: "suspend" before sleep, "resume" once per wake; it must return at once
}

type handler struct {
	h       Handlers
	log     func(string, ...any)
	resumed bool // the wake since the last suspend was reported; only the window's thread uses it
}

func proc(hwnd, message, wParam, lParam uintptr) uintptr {
	if v, ok := handlers.Load(hwnd); ok {
		h := v.(*handler)
		switch message {
		case wmQueryEndSession:
			h.log("session: WM_QUERYENDSESSION lParam=%#x", lParam)
			return 1 // the session may end
		case wmEndSession:
			h.log("session: WM_ENDSESSION ending=%v lParam=%#x", wParam != 0, lParam)
			if wParam != 0 {
				h.h.OnEnd()
			}
			return 0
		case wmPowerBroadcast:
			h.power(wParam)
			return 1
```

Найти:

```go
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

var className = windows.StringToUTF16Ptr("SplitLaneSessionWatch")

// Watch creates the hidden window on a thread of its own and calls onEnd from WM_ENDSESSION,
// before Windows ends the process. onEnd must finish within a few seconds.
func Watch(onEnd func(), log func(string, ...any)) (*Watcher, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
```

Заменить на:

```go
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// power reports one event of WM_POWERBROADCAST. A wake comes as PBT_APMRESUMEAUTOMATIC and, once the
// user is there, PBT_APMRESUMESUSPEND as well: it is reported once, until the next suspend.
func (h *handler) power(event uintptr) {
	switch event {
	case pbtAPMSuspend:
		h.log("session: WM_POWERBROADCAST suspend")
		h.resumed = false
		h.h.OnPower("suspend")
	case pbtAPMResumeAutomatic, pbtAPMResumeSuspend:
		h.log("session: WM_POWERBROADCAST resume %#x", event)
		if !h.resumed {
			h.resumed = true
			h.h.OnPower("resume")
		}
	}
}

var className = windows.StringToUTF16Ptr("SplitLaneSessionWatch")

// Watch creates the hidden window on a thread of its own. It calls h.OnEnd from WM_ENDSESSION, before
// Windows ends the process, and h.OnPower from WM_POWERBROADCAST.
func Watch(h Handlers, log func(string, ...any)) (*Watcher, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	if h.OnEnd == nil {
		h.OnEnd = func() {}
	}
	if h.OnPower == nil {
		h.OnPower = func(string) {}
	}
```

Найти:

```go
		handlers.Store(hwnd, &handler{onEnd: onEnd, log: log})
```

Заменить на:

```go
		handlers.Store(hwnd, &handler{h: h, log: log})
```

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
	watcher, err := session.Watch(eng.SessionEnding, logf)
	if err != nil {
		logf("session end will not be noticed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start()
	go eng.Run(ctx)
	go srv.Serve()
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	return exitOK
```

Заменить на:

```go
	// Section 10: sleep and wake come to the hidden window, whose procedure must answer at once; route
	// changes come a burst at a time. The engine follows them only while routing is on.
	watcher, err := session.Watch(session.Handlers{
		OnEnd:   eng.SessionEnding,
		OnPower: func(event string) { go eng.NetworkChanged(event) },
	}, logf)
	if err != nil {
		logf("session end and sleep will not be noticed: %v", err)
	}
	stopRoutes, err := winhost.WatchRoutes(func() { eng.NetworkChanged("route") }, logf)
	if err != nil {
		logf("route changes will not be noticed: %v", err)
		stopRoutes = func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start()
	go eng.Run(ctx)
	go srv.Serve()
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	stopRoutes()
	return exitOK
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/session/ ./cmd/splitlane-helper/; go test ./internal/session/ -count=1 -v | Select-String -Pattern '^(--- |ok|FAIL)'; go test ./cmd/splitlane-helper/ -count=1
```

Ожидается: `go vet` молчит; `--- PASS: TestEndSessionCallsTheHandler`, `--- PASS: TestPowerBroadcastCallsTheHandler`, `--- PASS: TestTwoWatchersAndStop`, `ok  	splitlane/helper/internal/session`, затем `ok  	splitlane/helper/cmd/splitlane-helper`.

- [ ] **Step 13: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `gofmt exit=0 files=[]`, `go vet` молчит, `ok` у всех пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`), затем ещё одно `ok  	splitlane/helper/internal/engine` для пятикратного прогона. Строка `gofmt exit=-1` — `gofmt.exe` не запустился, шаг не пройден. Нестабильный тест — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 14: Записать решение в спецификацию**

Изменить `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`:

Найти:

```markdown
Если сеть есть, а проверки не проходят, ядро перезапускается: сначала откат 5.4 (возврат и подтверждение системного exit node), затем включение 5.3. |
```

Заменить на:

```markdown
Если сеть есть, а проверки не проходят, ядро перезапускается: сначала откат 5.4 (возврат и подтверждение системного exit node), затем включение 5.3. Если после смены сети проверки прошли, а вид соединения встроенного узла с exit node 60 секунд остаётся «через ретранслятор», хотя до события был прямым, ядро тоже перезапускается: узел сам прямое соединение не восстанавливает (приложение A, проверки плана 1B). |
```

- [ ] **Step 15: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine helper/internal/winhost helper/internal/session helper/cmd/splitlane-helper docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "feat(helper): sleep and network change: wait for the route and the checks, restart when they fail or the path stays relay" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 11: Живая активность: соединения ядра, подписка и событие

Раздел 8.4: раз в секунду, пока работает ядро с адаптером (фазы `on`, `degraded`, `blocked`), окно получает сводку активности. Драйвер читает в `GET /connections` ядра 1.14.4 ещё и накопленные счётчики `upload`, `download` и узел соединения: `metadata.host` — имя, которое ядро знает из обратного сопоставления ответов DNS (6.4), иначе `metadata.destinationIP` (формат — `experimental/clashapi/connections.go`). Движок раз в секунду, пока ядро работает в живой фазе и подключено хотя бы одно окно, передаёт соединения текущего запуска в `activity.Tracker` задачи 4 вместе с сохранённой раскладкой (компилируется заново, когда она меняется) и основной дорожкой (`direct` в режиме «всё напрямую», иначе дорожка по умолчанию). С каждым новым запуском ядра и при выходе фазы из `on`, `degraded`, `blocked` подсчёт начинается заново. Без окна соединения не читаются, а прежняя сводка больше не отдаётся: вернувшееся окно получило бы устаревшие скорости. `Engine.Activity` отдаёт сводку последнего замера.

5.1: на `activity.subscribe` отвечает сам канал (`{}`), он хранит подписку и её `detailFor` у клиента до закрытия соединения; повторная подписка заменяет прежнюю, запрос с неверными параметрами — `bad_request`, подписка остаётся прежней. `RunActivity` раз в секунду спрашивает источник (движок) один раз на каждый различный `detailFor` — вне блокировки канала, потому что движок публикует состояние под своей блокировкой, — и ставит событие `activity` в ту же очередь клиента, что и события `state`: клиент, который перестал читать, отключается, остальные продолжают получать события (Review Focus 3). Движок на `activity.subscribe`, если тот всё же до него дойдёт, отвечает `bad_request`, а не `internal`.

Два решения контроллера меняют подсчёт задачи 4: незакреплённая плитка `system` не входит в сводку (и не даёт подробностей), пока в раскладке есть закреплённое правило вида `system` — его трафик показывает группа правила, а вторая плитка «Службы Windows» всегда стояла бы без трафика; плитка `unattributed` есть в каждой сводке. Скорость — байты с прошлого замера на секунду реального времени между замерами, не меньше секунды: опоздавший такт не даёт всплеска, первый замер и такт сразу после опоздавшего считаются за секунду; исполняемый файл группы по-прежнему выбирается по байтам. `slctl activity` подписывается и печатает строку на каждое событие (её читают задачи 15 и 16). Дополнение: строки `RESULT` у `slctl` несут сырой JSON ответов, в том числе `tailscale.authUrl` — ссылку для входа; `slctl` закрывает её в каждой строке, которую печатает (`"authUrl":"<login link>"`), и с `-show-addresses` тоже; ссылку показывает только отдельная строка `slctl login`.

**Files:**
- Create: `helper/internal/engine/activity.go`, `helper/internal/engine/activity_test.go`, `helper/internal/pipe/activity.go`, `helper/internal/pipe/activity_test.go`
- Modify: `helper/internal/activity/activity.go`, `helper/internal/singbox/clash.go`, `helper/internal/engine/ports.go`, `helper/internal/engine/engine.go`, `helper/internal/engine/supervise.go`, `helper/internal/engine/commands.go`, `helper/internal/pipe/server.go`, `helper/cmd/splitlane-helper/main.go`, `helper/cmd/slctl/main.go`
- Test: `helper/internal/activity/activity_test.go`, `helper/internal/singbox/clash_test.go`, `helper/internal/engine/activity_test.go`, `helper/internal/engine/commands_test.go`, `helper/internal/pipe/activity_test.go`, `helper/cmd/slctl/main_test.go`

**Interfaces:**
- Consumes: `activity.New(env rules.Env) *activity.Tracker`, `(*activity.Tracker).Sample(now time.Time, conns []activity.Conn, layout activity.Layout, mainLane string)`, `(*activity.Tracker).Summary(detailFor string) protocol.Activity`, `(*activity.Tracker).Reset()`, `activity.Conn` (задача 4); `protocol.Activity`, `protocol.EventActivity`, `protocol.SubscribeParams`, `protocol.DecodeSubscribe(params json.RawMessage) (detailFor string, err *protocol.Error)` (задача 4); `rules.Compile(apps []protocol.AppRule, env rules.Env) (*rules.Set, error)`, `(*rules.Set).RuleOf(path string) (protocol.AppRule, bool)` (задача 4); `protocol.MaskAddresses(s string) string`, `protocol.EncodeEvent(name string, data any) []byte`.
- Produces: поля `engine.Conn`: `Host string`, `Upload int64`, `Download int64`; `func (e *Engine) Activity(detailFor string) (protocol.Activity, bool)` (пакет `engine`); `type ActivitySource interface { Activity(detailFor string) (protocol.Activity, bool) }` и `func (s *Server) RunActivity(src ActivitySource, every time.Duration)` (пакет `pipe`, возвращается после `Close`); команда `slctl activity [-seconds 10] [-detail KEY]`: на каждое событие строка `ACTIVITY vpn=<down>/<up> direct=<down>/<up> apps=<n> top=<name or key>:<lane>:<down+up>`, с подробностями ещё `DETAIL key=<k> exes=<n> connections=<n>`, в конце `SLCTL: <n> activity events in <s> s` и код 0 или `SLCTL: no activity event in <s> s: ...` и код 3.

- [ ] **Step 1: Написать тесты подсчёта: плитка `system` при закреплённом правиле и скорость по прошедшему времени**

Последняя часть `TestSystemAndUnattributedTiles` ждала, что особая плитка `system` остаётся рядом с закреплённым правилом; по решению контроллера её теперь нет.

Изменить `helper/internal/activity/activity_test.go`:

Найти:

```go
import (
	"reflect"
	"strings"
	"testing"
	"time"
```

Заменить на:

```go
import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
```

Найти:

```go
	if g := find(t, s, `C:\Tools\`); g.Kind != protocol.KindApp || g.Name != "svchost" {
		t.Errorf("svchost.exe elsewhere %+v", g)
	}
	// a pinned "Windows services" tile takes svchost.exe and :System; the special tile stays, idle
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 2, 4),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 6, 8),
```

Заменить на:

```go
	if g := find(t, s, `C:\Tools\`); g.Kind != protocol.KindApp || g.Name != "svchost" {
		t.Errorf("svchost.exe elsewhere %+v", g)
	}
	// a pinned "Windows services" tile takes svchost.exe and :System; the special tile leaves the
	// summary while the rule is there (TestNoSystemTileWhenSystemIsPinned)
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 2, 4),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 6, 8),
```

Найти:

```go
		g.Lane != protocol.LaneDirect || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("pinned system %+v", g)
	}
	if g := find(t, s, protocol.KeySystem); g.Down != 0 || g.Up != 0 || g.Connections != 0 {
		t.Errorf("the special tile without traffic %+v", g)
	}
	find(t, s, protocol.KeyUnattributed)
}

func TestUnpinnedKeptFiveMinutes(t *testing.T) {
```

Заменить на:

```go
		g.Lane != protocol.LaneDirect || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("pinned system %+v", g)
	}
	if got := keys(s); slices.Contains(got, protocol.KeySystem) || !slices.Contains(got, protocol.KeyUnattributed) {
		t.Errorf("the pinned rule instead of the special tile: %v", got)
	}
}

// Ruling of plan 1B: while the layout pins "Windows services" (a rule of kind system), its traffic is
// the rule's, and the unpinned tile system is left out of the summary, its detail too: two tiles of
// the same name would show one of them always idle. The tile unattributed stays in every summary.
// Once the rule goes, the special tile is back and the rule's group leaves at once.
func TestNoSystemTileWhenSystemIsPinned(t *testing.T) {
	tr := New(testEnv)
	sys := layout(t, pinned(sysID, protocol.LaneDirect, protocol.KindSystem, ""))
	tr.Sample(start, []Conn{conn("1", `:System`, protocol.LaneDirect, 1, 2), conn("2", "", protocol.LaneVPN, 3, 4)}, sys, protocol.LaneVPN)
	s := tr.Summary(protocol.KeySystem)
	if got, want := keys(s), []string{protocol.KeyUnattributed, sysID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned: %v, want %v", got, want)
	}
	if s.Detail != nil {
		t.Fatalf("no detail for a tile that is not in the summary: %+v", s.Detail)
	}
	tr.Sample(start.Add(time.Second), nil, sys, protocol.LaneVPN)
	if got, want := keys(tr.Summary("")), []string{sysID, protocol.KeyUnattributed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned, idle: %v, want %v", got, want)
	}
	tr.Sample(start.Add(2*time.Second), nil, layout(t), protocol.LaneVPN)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneVPN)) {
		t.Fatalf("the rule is gone: %+v", got)
	}
}

// Ruling of plan 1B: a rate is the bytes since the previous sample per second of the real time between
// the two samples, so a tick that came late does not show a burst; a gap shorter than a second (the tick
// after a late one) counts as one second. The first sample counts as one second too. The executable of
// a group is still chosen by its bytes.
func TestRatesScaleWithElapsedTime(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	a, b := `C:\Apps\A\a.exe`, `C:\Apps\A\b.exe`
	tr.Sample(start, []Conn{conn("1", a, protocol.LaneVPN, 100, 1000)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes.VPN != (protocol.Traffic{Down: 1000, Up: 100}) {
		t.Fatalf("first sample: %+v", s.Lanes)
	}
	// 2 s later: 2000 and 200 more bytes are 1000 and 100 a second; a new connection counts its
	// whole counters over the same 2 s
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 300, 3000), conn("2", b, protocol.LaneDirect, 40, 401)}, none, protocol.LaneVPN)
	s := tr.Summary(`C:\Apps\A\`)
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 100}, Direct: protocol.Traffic{Down: 200, Up: 20}}); s.Lanes != want {
		t.Fatalf("2 s: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 1200 || g.Up != 120 || g.Exe != a {
		t.Fatalf("2 s: group %+v", g)
	}
	want := []protocol.ActivityConn{{Host: "host.example", Lane: protocol.LaneVPN, Down: 1000, Up: 100},
		{Host: "host.example", Lane: protocol.LaneDirect, Down: 200, Up: 20}}
	if s.Detail == nil || !reflect.DeepEqual(s.Detail.Connections, want) {
		t.Fatalf("2 s: detail %+v", s.Detail)
	}
	// 500 ms later counts as a second
	tr.Sample(start.Add(2500*time.Millisecond), []Conn{conn("1", a, protocol.LaneVPN, 350, 3500), conn("2", b, protocol.LaneDirect, 40, 401)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 500, Up: 50}}) {
		t.Fatalf("500 ms: %+v", s.Lanes)
	}
}

func TestUnpinnedKeptFiveMinutes(t *testing.T) {
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/activity/ -run 'TestSystemAndUnattributedTiles|TestNoSystemTileWhenSystemIsPinned|TestRatesScaleWithElapsedTime' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/activity` и три провала:

```text
--- FAIL: TestSystemAndUnattributedTiles (0.00s)
    activity_test.go:225: the pinned rule instead of the special tile: [9e8d7c6b-5a49-4f38-8e27-160f5e4d3c2b C:\Tools\ C:\Windows\explorer.exe system unattributed]
--- FAIL: TestNoSystemTileWhenSystemIsPinned (0.00s)
    activity_test.go:239: pinned: [unattributed 9e8d7c6b-5a49-4f38-8e27-160f5e4d3c2b system], want [unattributed 9e8d7c6b-5a49-4f38-8e27-160f5e4d3c2b]
--- FAIL: TestRatesScaleWithElapsedTime (0.00s)
    activity_test.go:271: 2 s: {VPN:{Down:2000 Up:200} Direct:{Down:401 Up:40}}
```

- [ ] **Step 3: Реализовать в подсчёте оба решения**

Изменить `helper/internal/activity/activity.go`:

Найти:

```go
	env     rules.Env
	svchost string

	mu     sync.Mutex
	prev   map[string]counters // the counters of the previous sample, by connection id
	groups map[string]*group   // by class.id
	lanes  protocol.ActivityLanes
}

type counters struct{ up, down int64 }
```

Заменить на:

```go
	env     rules.Env
	svchost string

	mu        sync.Mutex
	prev      map[string]counters // the counters of the previous sample, by connection id
	last      time.Time           // the time of the previous sample; zero before the first
	groups    map[string]*group   // by class.id
	lanes     protocol.ActivityLanes
	sysPinned bool // the layout of the last sample pins "Windows services": the tile system is left out
}

type counters struct{ up, down int64 }
```

Найти:

```go
	return t
}

// Reset forgets everything (the core stopped): counters, groups and the files seen.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
```

Заменить на:

```go
	return t
}

// Reset forgets everything (the core stopped): counters, groups, the files seen, the last sample's time.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
```

Найти:

```go
func (t *Tracker) reset() {
	t.prev = map[string]counters{}
	t.lanes = protocol.ActivityLanes{}
	t.groups = map[string]*group{}
	for _, key := range []string{protocol.KeySystem, protocol.KeyUnattributed} {
```

Заменить на:

```go
func (t *Tracker) reset() {
	t.prev = map[string]counters{}
	t.last = time.Time{}
	t.sysPinned = false
	t.lanes = protocol.ActivityLanes{}
	t.groups = map[string]*group{}
	for _, key := range []string{protocol.KeySystem, protocol.KeyUnattributed} {
```

Найти:

```go
}

// Sample takes the connections of one second. mainLane is the lane of unpinned programs now (the default lane,
// or direct in "all direct").
func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string) {
	t.mu.Lock()
	defer t.mu.Unlock()
```

Заменить на:

```go
}

// Sample takes the connections of one second. mainLane is the lane of unpinned programs now (the default lane,
// or direct in "all direct"). The rates are the bytes since the previous sample per second of the time
// between the two samples (a tick may come late), and at least one second: the first sample, and a tick
// right after a late one, count as one second.
func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string) {
	t.mu.Lock()
	defer t.mu.Unlock()
```

Найти:

```go
		g.down, g.up, g.conns = 0, 0, nil
	}
	t.lanes = protocol.ActivityLanes{}
	// A program has many connections: the rules are matched once per path and sample.
	classes := map[string]class{}
	classOf := func(path string) class {
```

Заменить на:

```go
		g.down, g.up, g.conns = 0, 0, nil
	}
	t.lanes = protocol.ActivityLanes{}
	t.sysPinned = pinsSystem(layout)
	elapsed := time.Second
	if !t.last.IsZero() && now.Sub(t.last) > elapsed {
		elapsed = now.Sub(t.last)
	}
	t.last = now
	// A program has many connections: the rules are matched once per path and sample.
	classes := map[string]class{}
	classOf := func(path string) class {
```

Найти:

```go
			was = counters{} // a new connection, or a new one under a reused id
		}
		next[c.ID] = cur
		up, down := cur.up-was.up, cur.down-was.down
		switch c.Lane {
		case protocol.LaneVPN:
			t.lanes.VPN.Up += up
```

Заменить на:

```go
			was = counters{} // a new connection, or a new one under a reused id
		}
		next[c.ID] = cur
		bytes := cur.up - was.up + cur.down - was.down
		up, down := perSecond(cur.up-was.up, elapsed), perSecond(cur.down-was.down, elapsed)
		switch c.Lane {
		case protocol.LaneVPN:
			t.lanes.VPN.Up += up
```

Найти:

```go
				e = &exeBytes{path: c.ProcessPath}
				g.exes[low] = e
			}
			e.bytes += up + down
		}
	}
	t.prev = next
```

Заменить на:

```go
				e = &exeBytes{path: c.ProcessPath}
				g.exes[low] = e
			}
			e.bytes += bytes
		}
	}
	t.prev = next
```

Найти:

```go
	}
}

// classify finds the group of a process path (spec 8.4): no owner, a pinned rule, the system
// tile, or the program folder.
func (t *Tracker) classify(path string, layout Layout, mainLane string) class {
```

Заменить на:

```go
	}
}

// perSecond is n bytes over elapsed (at least a second) as bytes per second; float64, so that the
// whole counters of a long connection seen for the first time cannot overflow.
func perSecond(n int64, elapsed time.Duration) int64 {
	if elapsed <= time.Second {
		return n
	}
	return int64(float64(n) * float64(time.Second) / float64(elapsed))
}

// pinsSystem: the layout has a rule of kind system ("Windows services" pinned, spec 8.1). Only the
// expressions of such a rule match the pseudo-path :System.
func pinsSystem(layout Layout) bool {
	if layout == nil {
		return false
	}
	r, ok := layout.RuleOf(":System")
	return ok && r.Matcher.Kind == protocol.KindSystem
}

// classify finds the group of a process path (spec 8.4): no owner, a pinned rule, the system
// tile, or the program folder.
func (t *Tracker) classify(path string, layout Layout, mainLane string) class {
```

Найти:

```go
}

// Summary is the event of the last sample; detailFor "" gives no detail. Apps are ordered by
// down+up, the busiest first, then by key.
func (t *Tracker) Summary(detailFor string) protocol.Activity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := protocol.Activity{Lanes: t.lanes, Apps: make([]protocol.ActivityApp, 0, len(t.groups))}
	for _, g := range t.groups {
		a := protocol.ActivityApp{Key: g.key, Kind: g.kind, Name: g.name, Exe: g.exe, Lane: g.lane,
			Down: g.down, Up: g.up, Connections: len(g.conns)}
		if g.ruleID != "" {
```

Заменить на:

```go
}

// Summary is the event of the last sample; detailFor "" gives no detail. Apps are ordered by
// down+up, the busiest first, then by key. While the layout pins "Windows services", the special
// tile system is left out (its traffic is the rule's); the tile unattributed is always there.
func (t *Tracker) Summary(detailFor string) protocol.Activity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := protocol.Activity{Lanes: t.lanes, Apps: make([]protocol.ActivityApp, 0, len(t.groups))}
	for id, g := range t.groups {
		if id == protocol.KeySystem && t.sysPinned {
			continue
		}
		a := protocol.ActivityApp{Key: g.key, Kind: g.kind, Name: g.name, Exe: g.exe, Lane: g.lane,
			Down: g.down, Up: g.up, Connections: len(g.conns)}
		if g.ruleID != "" {
```

- [ ] **Step 4: Запустить тесты пакета, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/activity/ -count=1 -v
```

Ожидается среди прочего `--- PASS: TestSystemAndUnattributedTiles`, `--- PASS: TestNoSystemTileWhenSystemIsPinned`, `--- PASS: TestRatesScaleWithElapsedTime`, `--- PASS: TestResetForgets` и `ok  	splitlane/helper/internal/activity`.

- [ ] **Step 5: Написать тест счётчиков и узла в списке соединений**

Поддельный API отвечает в форме sing-box 1.14.4 (все поля `connectionObject.MarshalJSON`, ключи по алфавиту, как их пишет `json.Marshal` карты).

Изменить `helper/internal/singbox/clash_test.go`:

Найти:

```go
	case r.Method == http.MethodGet && r.URL.Path == "/configs":
		json.NewEncoder(w).Encode(map[string]any{"mode": f.mode, "mode-list": []string{"vpn"}})
	case r.Method == http.MethodGet && r.URL.Path == "/connections":
		w.Write([]byte(`{"downloadTotal":1,"uploadTotal":2,"connections":[
 {"id":"a1","metadata":{"processPath":"C:\\Tools\\curl-a.exe","host":"example.org"},"chains":["direct"],"rule":"rule_set=lane-direct => route(direct)"},
 {"id":"b2","metadata":{"processPath":""},"chains":["vpn"],"rule":"final"},
 {"id":"c3","metadata":{"processPath":""},"chains":["vpn"],"rule":"inbound=[probe-vpn] => route(vpn)"},
 {"id":"d4","metadata":{"processPath":"C:\\Tools\\x.exe"},"chains":["direct"],"rule":"clash_mode=alldirect => route(direct)"}],"memory":0}`))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/connections/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/connections/"))
		w.WriteHeader(http.StatusNoContent)
```

Заменить на:

```go
	case r.Method == http.MethodGet && r.URL.Path == "/configs":
		json.NewEncoder(w).Encode(map[string]any{"mode": f.mode, "mode-list": []string{"vpn"}})
	case r.Method == http.MethodGet && r.URL.Path == "/connections":
		// The shape of experimental/clashapi/connections.go in sing-box 1.14.4: host is the domain
		// (the reverse mapping of DNS answers gives one, 6.4), destinationIP is "" for a destination
		// without an address, the counters are cumulative byte counts.
		w.Write([]byte(`{"connections":[
 {"chains":["direct"],"download":5000000000,"id":"a1","metadata":{"destinationIP":"203.0.113.5","destinationPort":"443","dnsMode":"normal","host":"example.org","network":"tcp","processPath":"C:\\Tools\\curl-a.exe","sourceIP":"192.0.2.10","sourcePort":"50001","type":"tun/tun-in"},"rule":"rule_set=lane-direct => route(direct)","rulePayload":"","start":"2026-11-14T12:00:00.5+03:00","upload":1024},
 {"chains":["vpn"],"download":0,"id":"b2","metadata":{"destinationIP":"198.51.100.7","destinationPort":"443","dnsMode":"normal","host":"","network":"udp","processPath":"","sourceIP":"192.0.2.10","sourcePort":"50002","type":"tun/tun-in"},"rule":"final","rulePayload":"","start":"2026-11-14T12:00:01+03:00","upload":0},
 {"chains":["vpn"],"download":10,"id":"c3","metadata":{"destinationIP":"","destinationPort":"443","dnsMode":"normal","host":"api.ipify.org","network":"tcp","processPath":"","sourceIP":"127.0.0.1","sourcePort":"50003","type":"mixed/probe-vpn"},"rule":"inbound=[probe-vpn] => route(vpn)","rulePayload":"","start":"2026-11-14T12:00:02+03:00","upload":20},
 {"chains":["direct"],"download":7,"id":"d4","metadata":{"destinationIP":"","destinationPort":"0","dnsMode":"normal","host":"","network":"tcp","processPath":"C:\\Tools\\x.exe","sourceIP":"192.0.2.10","sourcePort":"50004","type":"tun/tun-in"},"rule":"clash_mode=alldirect => route(direct)","rulePayload":"","start":"2026-11-14T12:00:03+03:00","upload":3}],
 "downloadTotal":5000000017,"memory":0,"uploadTotal":1047}`))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/connections/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/connections/"))
		w.WriteHeader(http.StatusNoContent)
```

Найти:

```go
		t.Fatalf("deleted %v", api.deleted)
	}
}
```

Заменить на:

```go
		t.Fatalf("deleted %v", api.deleted)
	}
}

// Spec 8.4: the connections give the activity its counters and hosts. upload and download are the
// cumulative bytes (more than 4 GiB fit); the host is metadata.host, the name from the reverse mapping
// of DNS answers (6.4), else metadata.destinationIP, else "".
func TestConnectionsHaveCounters(t *testing.T) {
	srv := httptest.NewServer(&fakeAPI{})
	defer srv.Close()
	conns, err := NewClash(srv.URL, "s3cret").Connections(context.Background())
	if err != nil || len(conns) != 4 {
		t.Fatalf("%v %v", conns, err)
	}
	want := []struct {
		host             string
		upload, download int64
	}{
		{"example.org", 1024, 5000000000},
		{"198.51.100.7", 0, 0},
		{"api.ipify.org", 20, 10},
		{"", 3, 7},
	}
	for i, w := range want {
		if c := conns[i]; c.Host != w.host || c.Upload != w.upload || c.Download != w.download {
			t.Errorf("connection %d: %+v, want host %q up %d down %d", i, c, w.host, w.upload, w.download)
		}
	}
}
```

- [ ] **Step 6: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -run 'TestClash|TestConnectionsHaveCounters' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/singbox [build failed]` и `c.Host undefined (type engine.Conn has no field or method Host)` (с ним `c.Upload undefined` и `c.Download undefined`).

- [ ] **Step 7: Добавить поля `Conn` и прочитать их в драйвере**

Изменить `helper/internal/engine/ports.go`:

Найти:

```go
}

// Conn is one connection of the core. Lane is the lane it runs on (the driver names its outbounds
// after the lanes); ByLanes tells that rules 8–11 of 6.2 (the mode or a lane file) chose it.
type Conn struct {
	ID          string
	ProcessPath string
	Lane        string
	ByLanes     bool
}

// ClashAPI is the core's local API (spec 3).
```

Заменить на:

```go
}

// Conn is one connection of the core. Lane is the lane it runs on (the driver names its outbounds
// after the lanes); ByLanes tells that rules 8–11 of 6.2 (the mode or a lane file) chose it. Host,
// Upload and Download are for the live activity (8.4).
type Conn struct {
	ID          string
	ProcessPath string
	Lane        string
	ByLanes     bool
	Host        string // the name from the reverse mapping of DNS answers (6.4), else the destination address
	Upload      int64  // bytes sent so far
	Download    int64  // bytes received so far
}

// ClashAPI is the core's local API (spec 3).
```

Изменить `helper/internal/singbox/clash.go`:

Найти:

```go
}

// Connections is GET /connections: id, process path, the lane (the first of "chains" is the
// outbound, and the config names the outbounds after the lanes) and whether rules 8–11 chose it.
func (c *Clash) Connections(ctx context.Context) ([]engine.Conn, error) {
	var data struct {
		Connections []struct {
			ID       string `json:"id"`
			Metadata struct {
				ProcessPath string `json:"processPath"`
			} `json:"metadata"`
			Chains []string `json:"chains"`
			Rule   string   `json:"rule"`
		} `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/connections", nil, &data); err != nil {
```

Заменить на:

```go
}

// Connections is GET /connections: id, process path, the lane (the first of "chains" is the
// outbound, and the config names the outbounds after the lanes), whether rules 8–11 chose it, and for
// the activity (8.4) the cumulative counters and the host: metadata.host, the domain the core knows
// (the reverse mapping of DNS answers, 6.4), else metadata.destinationIP ("" without an address).
func (c *Clash) Connections(ctx context.Context) ([]engine.Conn, error) {
	var data struct {
		Connections []struct {
			ID       string `json:"id"`
			Metadata struct {
				ProcessPath   string `json:"processPath"`
				Host          string `json:"host"`
				DestinationIP string `json:"destinationIP"`
			} `json:"metadata"`
			Upload   int64    `json:"upload"`
			Download int64    `json:"download"`
			Chains   []string `json:"chains"`
			Rule     string   `json:"rule"`
		} `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/connections", nil, &data); err != nil {
```

Найти:

```go
	}
	out := make([]engine.Conn, 0, len(data.Connections))
	for _, x := range data.Connections {
		conn := engine.Conn{ID: x.ID, ProcessPath: x.Metadata.ProcessPath, ByLanes: byLanes(x.Rule)}
		if len(x.Chains) > 0 {
			conn.Lane = x.Chains[0]
		}
```

Заменить на:

```go
	}
	out := make([]engine.Conn, 0, len(data.Connections))
	for _, x := range data.Connections {
		conn := engine.Conn{ID: x.ID, ProcessPath: x.Metadata.ProcessPath, ByLanes: byLanes(x.Rule),
			Host: x.Metadata.Host, Upload: x.Upload, Download: x.Download}
		if conn.Host == "" {
			conn.Host = x.Metadata.DestinationIP
		}
		if len(x.Chains) > 0 {
			conn.Lane = x.Chains[0]
		}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -run 'TestClash|TestConnectionsHaveCounters' -count=1 -v
```

Ожидается `--- PASS: TestClashModeIsReadBack`, `--- PASS: TestClashConnectionsAndClose`, `--- PASS: TestConnectionsHaveCounters` и `ok  	splitlane/helper/internal/singbox`.

- [ ] **Step 9: Написать тесты активности в движке**

Тесты вызывают такт `sampleActivity` сами (как `checkNode` и `refreshNode` в задаче 9) и двигают поддельные часы; число окон задают напрямую, чтобы в `off` не запускался режим настройки (раздел 7). `TestUnknownAndLaterMethods` теперь ждёт `bad_request` на `activity.subscribe`.

Файл `helper/internal/engine/activity_test.go`, целиком:

```go
package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// checkedApps is a layout as lanes.set saves it in state.json: checked and normalized.
func checkedApps(t *testing.T, apps ...protocol.AppRule) []protocol.AppRule {
	t.Helper()
	checked, rejected, err := rules.Check(apps, testRules)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	return checked
}

// windows sets the number of connected windows that the pipe gives with SetClients; set directly, so
// that no setup mode starts in off (spec 7).
func windows(h *harness, n int) { h.e.clients.Store(int32(n)) }

// countReads counts the calls of Connections from now on.
func countReads(h *harness) func() int {
	var mu sync.Mutex
	n := 0
	h.clash.mu.Lock()
	h.clash.onConns = func() {
		mu.Lock()
		defer mu.Unlock()
		n++
	}
	h.clash.mu.Unlock()
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// setConns makes conns the connections of the core.
func setConns(h *harness, conns ...Conn) {
	h.clash.mu.Lock()
	defer h.clash.mu.Unlock()
	h.clash.conns = conns
}

// appIn is the group key of a summary; ok false when it is not there.
func appIn(s protocol.Activity, key string) (protocol.ActivityApp, bool) {
	for _, a := range s.Apps {
		if a.Key == key {
			return a, true
		}
	}
	return protocol.ActivityApp{}, false
}

func summaryNow(t *testing.T, h *harness, detailFor string) protocol.Activity {
	t.Helper()
	s, ok := h.e.Activity(detailFor)
	if !ok {
		t.Fatalf("no summary in phase %s", h.e.State().Phase)
	}
	return s
}

// Spec 8.4: while the core with the adapter runs and a window is connected, the connections of the
// run are read once a second; two samples give the rates of the lanes and of each group: a pinned
// program by its rule, an unpinned one by its folder on the main lane, a connection without an owner
// in the tile unattributed, the detail of the group asked for with the host of each connection. The
// layout is compiled again when it changes; the main lane is direct in "all direct". Without a window
// nothing is read and there is no summary.
func TestActivityWhileOn(t *testing.T) {
	h := newHarness(t)
	h.store.state.Apps = checkedApps(t, fileRule(idA, curlA, protocol.LaneDirect))
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	ctx := context.Background()
	reads := countReads(h)
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneDirect, Host: "a.example", Upload: 100, Download: 1000},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Host: "203.0.113.7", Upload: 10, Download: 20})
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != 0 {
		t.Fatalf("no window: nothing read (%d), no summary", reads())
	}
	windows(h, 1)
	h.e.sampleActivity(ctx)
	if s := summaryNow(t, h, ""); s.Lanes.Direct != (protocol.Traffic{Down: 1000, Up: 100}) {
		t.Fatalf("the first sample counts the whole counters: %+v", s.Lanes)
	}
	h.clock.After(time.Second)
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneDirect, Host: "a.example", Upload: 150, Download: 1600},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Host: "203.0.113.7", Upload: 12, Download: 25},
		Conn{ID: "3", Lane: protocol.LaneVPN, Host: "198.51.100.9", Upload: 7, Download: 9})
	h.e.sampleActivity(ctx)
	s := summaryNow(t, h, idA)
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 14, Up: 9}, Direct: protocol.Traffic{Down: 600, Up: 50}}); s.Lanes != want {
		t.Fatalf("lanes %+v, want %+v", s.Lanes, want)
	}
	if g, ok := appIn(s, idA); !ok || g.RuleID == nil || *g.RuleID != idA || g.Lane != protocol.LaneDirect ||
		g.Down != 600 || g.Up != 50 || g.Connections != 1 || g.Exe != curlA {
		t.Fatalf("the pinned program: %+v", g)
	}
	if g, ok := appIn(s, `C:\Tools\b\`); !ok || g.RuleID != nil || g.Kind != protocol.KindApp || g.Name != "curl-b" ||
		g.Lane != protocol.LaneVPN || g.Down != 5 || g.Up != 2 {
		t.Fatalf("the unpinned program on the main lane: %+v", g)
	}
	if g, ok := appIn(s, protocol.KeyUnattributed); !ok || g.Down != 9 || g.Up != 7 || g.Connections != 1 {
		t.Fatalf("no owner: %+v", g)
	}
	if _, ok := appIn(s, protocol.KeySystem); !ok {
		t.Fatalf("the special tiles are there: %+v", s.Apps)
	}
	if d := s.Detail; d == nil || d.Key != idA || len(d.Connections) != 1 || d.Connections[0] != (protocol.ActivityConn{Host: "a.example", Lane: protocol.LaneDirect, Down: 600, Up: 50}) {
		t.Fatalf("detail %+v", s.Detail)
	}
	// curl-b is pinned now and "all direct" is on: the new layout groups it, the main lane is direct
	h.e.mu.Lock()
	h.e.saved.Apps = checkedApps(t, fileRule(idA, curlA, protocol.LaneDirect), fileRule(idB, curlB, protocol.LaneDirect))
	h.e.st.AllDirect = true
	h.e.mu.Unlock()
	h.clock.After(time.Second)
	other := `C:\Tools\c\curl-c.exe`
	setConns(h,
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneDirect, Upload: 12, Download: 30},
		Conn{ID: "4", ProcessPath: other, Lane: protocol.LaneDirect, Upload: 1, Download: 1})
	h.e.sampleActivity(ctx)
	s = summaryNow(t, h, "")
	if g, ok := appIn(s, idB); !ok || g.Lane != protocol.LaneDirect || g.Down != 5 {
		t.Fatalf("curl-b under its new rule: %+v", s.Apps)
	}
	if g, ok := appIn(s, `C:\Tools\c\`); !ok || g.Lane != protocol.LaneDirect {
		t.Fatalf("an unpinned program in all direct: %+v", s.Apps)
	}
	if g, ok := appIn(s, protocol.KeyUnattributed); !ok || g.Lane != protocol.LaneDirect {
		t.Fatalf("the special tile on the main lane: %+v", g)
	}
	windows(h, 0)
	before := reads()
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != before {
		t.Fatal("the last window went: nothing is read, the old summary is not offered")
	}
}

// Spec 8.4: the event goes only while the core with the adapter runs: on, degraded, blocked. In off,
// in starting and stopping, and after a disable, the connections are not read and there is no
// summary, with a window connected. A phase that leaves on, degraded and blocked starts the tracker
// again: back in on, the connections count their whole counters.
func TestNoActivityWhenOff(t *testing.T) {
	h := newHarness(t)
	h.load()
	windows(h, 1)
	ctx := context.Background()
	reads := countReads(h)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: 1000})
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != 0 {
		t.Fatalf("off: read %d times", reads())
	}
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	want := int64(1000) // the first sample of the run counts the whole counters, the next ones what is new
	for _, phase := range []string{protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked} {
		setPhase(h, phase)
		h.clock.After(time.Second)
		h.e.sampleActivity(ctx)
		if s := summaryNow(t, h, ""); s.Lanes.VPN.Down != want {
			t.Fatalf("%s: %+v", phase, s.Lanes)
		}
		want = 0
	}
	read := reads()
	for _, phase := range []string{protocol.PhaseStarting, protocol.PhaseStopping} {
		setPhase(h, phase)
		h.e.sampleActivity(ctx)
		if _, ok := h.e.Activity(""); ok || reads() != read {
			t.Fatalf("%s: a summary, or read", phase)
		}
	}
	setPhase(h, protocol.PhaseOn)
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	if s := summaryNow(t, h, ""); s.Lanes.VPN.Down != 1000 {
		t.Fatalf("back in on the tracker starts again: %+v", s.Lanes)
	}
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	read = reads()
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != read {
		t.Fatal("after the disable: a summary, or read")
	}
}

// Spec 8.4: the tracker starts again with each run of the core. After a restart (5.5) the connections
// of the new core count their whole counters, and a program seen only by the core that is gone is not
// kept: its 5 minutes belonged to that run. The new run has no summary before its first sample.
func TestActivityResetsOnRestart(t *testing.T) {
	h := enabled(t)
	windows(h, 1)
	ctx := context.Background()
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: 1000},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Upload: 1, Download: 1})
	h.e.sampleActivity(ctx)
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 150, Download: 1600})
	h.e.sampleActivity(ctx)
	s := summaryNow(t, h, "")
	if g, ok := appIn(s, `C:\Tools\a\`); !ok || g.Down != 600 || g.Up != 50 {
		t.Fatalf("first run: %+v", s.Apps)
	}
	if _, ok := appIn(s, `C:\Tools\b\`); !ok {
		t.Fatalf("first run: curl-b is kept for 5 minutes: %+v", s.Apps)
	}
	h.core.last().crash()
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if _, ok := h.e.Activity(""); ok {
		t.Fatal("the new run has no summary before its first sample")
	}
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	s = summaryNow(t, h, "")
	if g, ok := appIn(s, `C:\Tools\a\`); !ok || g.Down != 1600 || g.Up != 150 {
		t.Fatalf("the new run counts the whole counters: %+v", s.Apps)
	}
	if _, ok := appIn(s, `C:\Tools\b\`); ok {
		t.Fatalf("a program of the run that is gone is not kept: %+v", s.Apps)
	}
}
```

Изменить `helper/internal/engine/commands_test.go`:

Найти:

```go
func TestUnknownAndLaterMethods(t *testing.T) {
	h := newHarness(t)
	wantCode(t, call(h, "routing.toggle", nil), protocol.CodeUnknownMethod)
	for _, m := range []string{protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport} {
		wantCode(t, call(h, m, nil), protocol.CodeInternal)
	}
	res, err := h.e.Handle(context.Background(), protocol.MethodStateGet, nil)
	if err != nil || res.(protocol.State).Phase != protocol.PhaseOff {
		t.Fatalf("state.get: %v %v", res, err)
```

Заменить на:

```go
func TestUnknownAndLaterMethods(t *testing.T) {
	h := newHarness(t)
	wantCode(t, call(h, "routing.toggle", nil), protocol.CodeUnknownMethod)
	// the pipe answers activity.subscribe itself (package pipe): one that reaches the engine is refused
	wantCode(t, call(h, protocol.MethodActivitySubscribe, nil), protocol.CodeBadRequest)
	wantCode(t, call(h, protocol.MethodDiagnosticsExport, nil), protocol.CodeInternal)
	res, err := h.e.Handle(context.Background(), protocol.MethodStateGet, nil)
	if err != nil || res.(protocol.State).Phase != protocol.PhaseOff {
		t.Fatalf("state.get: %v %v", res, err)
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestActivityWhileOn|TestNoActivityWhenOff|TestActivityResetsOnRestart|TestUnknownAndLaterMethods' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/engine [build failed]` и `h.e.Activity undefined (type *Engine has no field or method Activity)`, `h.e.sampleActivity undefined (type *Engine has no field or method sampleActivity)`.

- [ ] **Step 11: Реализовать активность в движке**

Такт раз в секунду идёт из `Run`, как остальные такты надзора; время замера — по часам движка. Поля `run` и `fresh` защищены `Engine.mu`, остальное принадлежит такту. `Activity` отвечает `false`, пока нет замера текущего запуска, сделанного при подключённом окне.

Файл `helper/internal/engine/activity.go`, целиком:

```go
package engine

import (
	"context"
	"slices"
	"time"

	"splitlane/helper/internal/activity"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// activityEvery: the connections of the core are read once a second (spec 8.4).
const activityEvery = time.Second

// liveActivity is the live activity of spec 8.4: the tracker of the current run of the core and the
// layout its samples are grouped by.
type liveActivity struct {
	tracker *activity.Tracker
	run     *coreRun // guarded by Engine.mu: the run the tracker's samples are of; nil after a reset
	fresh   bool     // guarded by Engine.mu: the last tick sampled run while a window was connected

	// Owned by sampleActivity.
	apps   []protocol.AppRule // the layout that set was compiled from
	set    *rules.Set         // nil until the first sample
	failed string             // the last error of the connection list, masked; "" while it works
}

// sampleActivity is one tick of spec 8.4, once a second from Run: while the core with the adapter runs
// (on, degraded, blocked) and a window is connected, the connections of the run go into the tracker,
// grouped by the saved layout (compiled again when it changes), the unpinned programs on the main lane:
// direct in "all direct", else the default lane. The tracker starts again from nothing for each run of
// the core and when the phase leaves on, degraded and blocked. Without a window nothing is read, and
// the summary of the last sample is no longer offered: it would be old when a window comes back. A
// failed read is logged masked, once while it repeats; the summary of that tick is not offered either.
func (e *Engine) sampleActivity(ctx context.Context) {
	a := &e.act
	e.mu.Lock()
	r := e.run
	if r == nil || r.stopping || !live(e.st.Phase) {
		if a.run != nil {
			a.tracker.Reset()
		}
		a.run, a.fresh = nil, false
		e.mu.Unlock()
		return
	}
	if a.run != r {
		a.tracker.Reset()
		a.run, a.fresh = r, false
	}
	if e.clients.Load() == 0 {
		a.fresh = false
		e.mu.Unlock()
		return
	}
	apps, mainLane := slices.Clone(e.saved.Apps), e.saved.DefaultLane
	if e.st.AllDirect {
		mainLane = protocol.LaneDirect
	}
	e.mu.Unlock()
	if a.set == nil || !slices.Equal(a.apps, apps) {
		set, err := rules.Compile(apps, e.d.Rules)
		if err != nil {
			e.d.Log("activity: the layout does not compile, every program is grouped by its folder: %v", err)
			set = &rules.Set{}
		}
		a.apps, a.set = apps, set
	}
	conns, err := r.clash.Connections(ctx)
	now := e.d.Clock.Now()
	if err != nil {
		e.mu.Lock()
		a.fresh = false
		gone := e.run != r || r.stopping
		e.mu.Unlock()
		if text := protocol.MaskAddresses(err.Error()); !gone && text != a.failed {
			a.failed = text
			e.d.Log("activity: the connections of the core: %s", text)
		}
		return
	}
	a.failed = ""
	sample := make([]activity.Conn, 0, len(conns))
	for _, c := range conns {
		sample = append(sample, activity.Conn{ID: c.ID, ProcessPath: c.ProcessPath, Lane: c.Lane, Host: c.Host,
			Upload: c.Upload, Download: c.Download})
	}
	a.tracker.Sample(now, sample, a.set, mainLane)
	e.mu.Lock()
	a.fresh = e.run == r
	e.mu.Unlock()
}

// Activity is the summary of the last sample (spec 8.4) with the detail of the group detailFor ("" for
// none): the data of the activity event. false while no core with the adapter runs (any phase but on,
// degraded and blocked), and until the run has been sampled with a window connected.
func (e *Engine) Activity(detailFor string) (protocol.Activity, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r == nil || r.stopping || r != e.act.run || !e.act.fresh || !live(e.st.Phase) {
		return protocol.Activity{}, false
	}
	return e.act.tracker.Summary(detailFor), true
}
```

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
	"sync/atomic"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)
```

Заменить на:

```go
	"sync/atomic"
	"time"

	"splitlane/helper/internal/activity"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)
```

Найти:

```go
	nodeBackend string     // tailscale.backend as the setup core's node says; stopped when it does not run
	nodeAuthURL string     // the login link of the setup core's node, "" when there is none
	connector   sync.Mutex // logouts one at a time; stopSetup waits for the one under way
}

// coreRun is one running core with its ports, the rule files it reads now and its API service.
```

Заменить на:

```go
	nodeBackend string     // tailscale.backend as the setup core's node says; stopped when it does not run
	nodeAuthURL string     // the login link of the setup core's node, "" when there is none
	connector   sync.Mutex // logouts one at a time; stopSetup waits for the one under way

	act liveActivity // the live activity (spec 8.4, activity.go)
}

// coreRun is one running core with its ports, the rule files it reads now and its API service.
```

Найти:

```go
	if d.OnExit == nil {
		d.OnExit = func() {}
	}
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults(), nodeBackend: protocol.BackendStopped}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
```

Заменить на:

```go
	if d.OnExit == nil {
		d.OnExit = func() {}
	}
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults(), nodeBackend: protocol.BackendStopped,
		act: liveActivity{tracker: activity.New(d.Rules)}}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
```

Изменить `helper/internal/engine/supervise.go`:

Найти:

```go
)

// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s;
// and the node of the main core (spec 7): its state every 10 s, its path and exit nodes every 60 s.
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	go e.every(ctx, healthEvery, e.checkNode)
	go e.every(ctx, listEvery, e.refreshNode)
	e.every(ctx, healthEvery, e.checkHealth)
}
```

Заменить на:

```go
)

// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s;
// the node of the main core (spec 7): its state every 10 s, its path and exit nodes every 60 s; and
// the connections of the core for the live activity every second (8.4).
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	go e.every(ctx, healthEvery, e.checkNode)
	go e.every(ctx, listEvery, e.refreshNode)
	go e.every(ctx, activityEvery, e.sampleActivity)
	e.every(ctx, healthEvery, e.checkHealth)
}
```

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
			return nil, err
		}
		return e.State(), nil
	case protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport:
		return nil, protocol.Errorf(protocol.CodeInternal, "%s is not available in this build (Plan 1B)", method)
	}
	e.queue.Lock()
```

Заменить на:

```go
			return nil, err
		}
		return e.State(), nil
	case protocol.MethodActivitySubscribe:
		// The pipe answers it and keeps the subscription of each client (package pipe); the events come
		// from Activity. It never reaches the engine.
		return nil, protocol.Errorf(protocol.CodeBadRequest, "activity.subscribe is answered by the pipe")
	case protocol.MethodDiagnosticsExport:
		return nil, protocol.Errorf(protocol.CodeInternal, "%s is not available in this build (Plan 1B)", method)
	}
	e.queue.Lock()
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestActivityWhileOn|TestNoActivityWhenOff|TestActivityResetsOnRestart|TestUnknownAndLaterMethods' -count=1 -v
```

Ожидается `--- PASS: TestActivityWhileOn`, `--- PASS: TestNoActivityWhenOff`, `--- PASS: TestActivityResetsOnRestart`, `--- PASS: TestUnknownAndLaterMethods` и `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 13: Написать тесты подписки и события в канале**

Источник активности в тестах — подделка; `RunActivity` тесты запускают сами, когда клиенты уже подписаны. В `TestStalledSubscriberIsDropped` каждое событие ждёт, пока читающий клиент получит предыдущее, как позволили бы такты раз в секунду: отстаёт только клиент, который не читает.

Файл `helper/internal/pipe/activity_test.go`, целиком:

```go
package pipe

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
)

// fakeSource plays the engine's activity: a summary, with the detail of detailFor when one is asked
// for, while ok; it records the detailFor of every call. With pace, each call first waits (at most
// 5 s) for a token that the reading client gives back for each event it got.
type fakeSource struct {
	mu    sync.Mutex
	ok    bool
	apps  []protocol.ActivityApp
	calls []string
	pace  chan struct{}
}

var _ ActivitySource = (*fakeSource)(nil)

func (f *fakeSource) Activity(detailFor string) (protocol.Activity, bool) {
	f.mu.Lock()
	f.calls = append(f.calls, detailFor)
	ok, apps, pace := f.ok, f.apps, f.pace
	f.mu.Unlock()
	if pace != nil {
		select {
		case <-pace:
		case <-time.After(5 * time.Second):
		}
	}
	a := protocol.Activity{Lanes: protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 100}}, Apps: apps}
	if detailFor != "" {
		a.Detail = &protocol.ActivityDetail{Key: detailFor}
	}
	return a, ok
}

func (f *fakeSource) set(ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ok = ok
}

func (f *fakeSource) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

// request sends a request and returns its answer; the events that come before it are skipped.
func (c *testClient) request(t *testing.T, line string) map[string]any {
	t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	for {
		if m := c.next(t); m["event"] == nil {
			return m
		}
	}
}

// activity is the next message, which must be an activity event; it returns its detail key, "" for none.
func (c *testClient) activity(t *testing.T) string {
	t.Helper()
	m := c.next(t)
	data, _ := m["data"].(map[string]any)
	if m["event"] != protocol.EventActivity || data["lanes"] == nil {
		t.Fatalf("not an activity event: %v", m)
	}
	d, _ := data["detail"].(map[string]any)
	key, _ := d["key"].(string)
	return key
}

func hello(t *testing.T, cs ...*testClient) {
	t.Helper()
	for _, c := range cs {
		if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`); m["result"] == nil {
			t.Fatalf("hello: %v", m)
		}
	}
}

// Spec 5.1, 8.4: the server answers activity.subscribe itself, with {} (after hello only), and sends the
// event to every client that subscribed; a client with detailFor gets the detail, the others do not, and
// a client that did not subscribe gets nothing. The source is asked once per detailFor and tick, not
// once per client. RunActivity returns at Close.
func TestActivitySubscribe(t *testing.T) {
	s, h, name := serve(t)
	early := dial(t, name)
	if code := errCode(early.send(t, `{"id":1,"method":"activity.subscribe"}`)); code != protocol.CodeBadRequest {
		t.Fatalf("before hello: %s", code)
	}
	a, a2, b, quiet := dial(t, name), dial(t, name), dial(t, name), dial(t, name)
	hello(t, a, a2, b, quiet)
	key := `C:\Apps\Tool\`
	for _, c := range []*testClient{a, a2} {
		if m := c.send(t, `{"id":2,"method":"activity.subscribe"}`); fmt.Sprint(m["result"]) != "map[]" {
			t.Fatalf("subscribe: %v", m)
		}
	}
	if m := b.send(t, `{"id":2,"method":"activity.subscribe","params":{"detailFor":"C:\\Apps\\Tool\\"}}`); fmt.Sprint(m["result"]) != "map[]" {
		t.Fatalf("subscribe with detailFor: %v", m)
	}
	h.mu.Lock()
	seen := strings.Join(h.methods, ",")
	h.mu.Unlock()
	if strings.Contains(seen, protocol.MethodActivitySubscribe) {
		t.Fatalf("the handler got activity.subscribe: %s", seen)
	}
	src := &fakeSource{ok: true}
	returned := make(chan struct{})
	go func() {
		s.RunActivity(src, 20*time.Millisecond)
		close(returned)
	}()
	for i := 0; i < 3; i++ {
		for _, c := range []*testClient{a, a2} {
			if got := c.activity(t); got != "" {
				t.Fatalf("a subscription without detailFor got the detail of %q", got)
			}
		}
		if got := b.activity(t); got != key {
			t.Fatalf("detail %q, want %q", got, key)
		}
	}
	// had it been sent an event, the event would come before the answer
	if m := quiet.send(t, `{"id":2,"method":"state.get"}`); m["event"] != nil || m["id"] == nil {
		t.Fatalf("a client that did not subscribe got %v", m)
	}
	calls := src.asked()
	for i := 0; i+1 < len(calls); i += 2 {
		if pair := []string{calls[i], calls[i+1]}; !(pair[0] == "" && pair[1] == key) && !(pair[0] == key && pair[1] == "") {
			t.Fatalf("calls %q: once per detailFor and tick", calls)
		}
	}
	s.Close()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("RunActivity did not return at Close")
	}
}

// Spec 5.1: a repeated activity.subscribe replaces the subscription; one with bad params is bad_request
// and leaves the subscription as it was.
func TestResubscribeReplaces(t *testing.T) {
	s, _, name := serve(t)
	c := dial(t, name)
	hello(t, c)
	c.send(t, `{"id":2,"method":"activity.subscribe","params":{"detailFor":"k"}}`)
	go s.RunActivity(&fakeSource{ok: true}, 20*time.Millisecond)
	if got := c.activity(t); got != "k" {
		t.Fatalf("detail %q", got)
	}
	if m := c.request(t, `{"id":3,"method":"activity.subscribe","params":{}}`); fmt.Sprint(m["result"]) != "map[]" {
		t.Fatalf("subscribe again: %v", m)
	}
	for i := 0; i < 3; i++ {
		if got := c.activity(t); got != "" {
			t.Fatalf("after the second subscribe: detail %q", got)
		}
	}
	if code := errCode(c.request(t, `{"id":4,"method":"activity.subscribe","params":{"detailFor":"k","more":1}}`)); code != protocol.CodeBadRequest {
		t.Fatalf("unknown field: %s", code)
	}
	for i := 0; i < 3; i++ {
		if got := c.activity(t); got != "" {
			t.Fatalf("after a refused subscribe: detail %q", got)
		}
	}
}

// Review Focus 3: a client that subscribed and stopped reading does not block the helper or the other
// clients: it is dropped as for state events, and the reading client keeps getting the events.
func TestStalledSubscriberIsDropped(t *testing.T) {
	s, h, name := serve(t)
	stalled, reading := dial(t, name), dial(t, name)
	stalled.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":1}}` + "\n" +
		`{"id":2,"method":"activity.subscribe"}` + "\n")) // never reads again
	hello(t, reading)
	reading.send(t, `{"id":2,"method":"activity.subscribe"}`)
	deadline := time.Now().Add(5 * time.Second)
	for h.count() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// ~20 KB per event: the stalled client's pipe buffer (64 KB) and queue (64) fill up long before 150.
	// Each event waits until the reading client has the one before (pace), as the 1 s ticks would allow.
	src := &fakeSource{ok: true, pace: make(chan struct{}, 1)}
	for i := 0; i < 40; i++ {
		src.apps = append(src.apps, protocol.ActivityApp{Key: fmt.Sprint(i), Kind: protocol.KindApp,
			Name: strings.Repeat("x", 250), Exe: strings.Repeat("y", 250), Lane: protocol.LaneVPN})
	}
	src.pace <- struct{}{}
	go s.RunActivity(src, time.Millisecond)
	finished := make(chan string, 1)
	go func() {
		for i := 0; i < 150; i++ {
			reading.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, err := reading.r.ReadString('\n'); err != nil {
				finished <- fmt.Sprintf("the reading client got no event %d: %v", i, err)
				return
			}
			select {
			case src.pace <- struct{}{}:
			default:
			}
		}
		finished <- ""
	}()
	select {
	case msg := <-finished:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("RunActivity blocked on a client that does not read")
	}
	if h.count() != 1 {
		t.Fatalf("clients %d: the stalled one must be dropped", h.count())
	}
}

// Spec 8.4: no event while the source has no summary (no core with the adapter runs); events come once
// it has one.
func TestNoActivityEventWhenSourceSaysNo(t *testing.T) {
	s, _, name := serve(t)
	c := dial(t, name)
	hello(t, c)
	c.send(t, `{"id":2,"method":"activity.subscribe"}`)
	src := &fakeSource{}
	go s.RunActivity(src, 10*time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for len(src.asked()) < 5 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(src.asked()); n < 5 {
		t.Fatalf("the source was asked %d times", n)
	}
	if m := c.send(t, `{"id":3,"method":"state.get"}`); m["event"] != nil || m["id"] == nil {
		t.Fatalf("an event without a summary: %v", m)
	}
	src.set(true)
	c.activity(t)
}
```

- [ ] **Step 14: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/pipe/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/pipe [build failed]` и `undefined: ActivitySource`, `s.RunActivity undefined (type *Server has no field or method RunActivity)`.

- [ ] **Step 15: Реализовать подписку и событие в канале, запустить рассылку в служебном процессе**

Файл `helper/internal/pipe/activity.go`, целиком:

```go
package pipe

import (
	"time"

	"splitlane/helper/internal/protocol"
)

// ActivitySource gives the data of the activity event (spec 8.4): the engine. false while there is
// nothing to send: no core with the adapter runs.
type ActivitySource interface {
	Activity(detailFor string) (protocol.Activity, bool)
}

// subscribe is activity.subscribe (5.1): the server keeps the subscription of each client itself, until
// the connection closes; a repeated one replaces it. Params that do not decode change nothing.
func (s *Server) subscribe(c *client, req protocol.Request) []byte {
	detailFor, err := protocol.DecodeSubscribe(req.Params)
	if err != nil {
		return protocol.EncodeError(req.ID, err)
	}
	s.mu.Lock()
	c.subscribed, c.detailFor = true, detailFor
	s.mu.Unlock()
	return protocol.EncodeResult(req.ID, struct{}{})
}

// RunActivity sends the activity event (5.1, 8.4) every period to each client that said hello and
// subscribed, until Close.
func (s *Server) RunActivity(src ActivitySource, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.sendActivity(src)
		}
	}
}

// sendActivity is one tick of RunActivity. src is asked once per distinct detailFor, outside the
// server's lock: the engine publishes its state under its own lock, and Broadcast takes this one.
// Nothing goes to a client whose source has no summary. The events go through the queue of the state
// events, so a client that does not read is dropped and the others go on (Review Focus 3).
func (s *Server) sendActivity(src ActivitySource) {
	s.mu.Lock()
	want := map[string]bool{}
	for c := range s.clients {
		if c.hello && c.subscribed {
			want[c.detailFor] = true
		}
	}
	s.mu.Unlock()
	lines := map[string][]byte{}
	for detailFor := range want {
		if a, ok := src.Activity(detailFor); ok {
			lines[detailFor] = protocol.EncodeEvent(protocol.EventActivity, a)
		}
	}
	if len(lines) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if line := lines[c.detailFor]; line != nil && c.hello && c.subscribed {
			s.send(c, line)
		}
	}
}
```

Изменить `helper/internal/pipe/server.go`:

Найти:

```go
	wg        sync.WaitGroup // the reader and writer of every client
	answering sync.WaitGroup // requests between their arrival and their queued answer
	closing   bool
}

// closeWait bounds each wait of Close: answers in flight, then the queues written out.
const closeWait = time.Second

type client struct {
	conn     net.Conn
	out      chan []byte
	ended    bool // out is closed: nothing more is queued (s.mu)
	hello    bool // a hello with the right protocol came
	mismatch bool // a hello with another protocol came: only helper.shutdown is accepted
}

// Listen creates the pipe. It fails when the pipe already exists: another helper runs.
```

Заменить на:

```go
	wg        sync.WaitGroup // the reader and writer of every client
	answering sync.WaitGroup // requests between their arrival and their queued answer
	closing   bool
	done      chan struct{} // closed by the first Close: RunActivity returns
}

// closeWait bounds each wait of Close: answers in flight, then the queues written out.
const closeWait = time.Second

type client struct {
	conn       net.Conn
	out        chan []byte
	ended      bool   // out is closed: nothing more is queued (s.mu)
	hello      bool   // a hello with the right protocol came
	mismatch   bool   // a hello with another protocol came: only helper.shutdown is accepted
	subscribed bool   // activity.subscribe came: the activity events go to this client (s.mu)
	detailFor  string // the detailFor of the subscription, "" for none (s.mu)
}

// Listen creates the pipe. It fails when the pipe already exists: another helper runs.
```

Найти:

```go
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Server{h: h, l: l, log: log, clients: map[*client]bool{}}
}

// acceptPause is the wait before Accept is tried again after an error.
```

Заменить на:

```go
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Server{h: h, l: l, log: log, clients: map[*client]bool{}, done: make(chan struct{})}
}

// acceptPause is the wait before Accept is tried again after an error.
```

Найти:

```go
	}
}

// Close stops accepting, lets the answers in flight reach their queues (the answer to
// helper.shutdown among them: the helper exits right after Close), writes every queue out and
// closes the clients. Each wait is bounded; a client that does not read is closed anyway.
func (s *Server) Close() {
	s.l.Close()
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	waitFor(&s.answering, closeWait)
```

Заменить на:

```go
	}
}

// Close stops accepting and the activity events, lets the answers in flight reach their queues (the
// answer to helper.shutdown among them: the helper exits right after Close), writes every queue out
// and closes the clients. Each wait is bounded; a client that does not read is closed anyway.
func (s *Server) Close() {
	s.l.Close()
	s.mu.Lock()
	if !s.closing {
		close(s.done)
	}
	s.closing = true
	s.mu.Unlock()
	waitFor(&s.answering, closeWait)
```

Найти:

```go
	}
}

// answer applies the hello rules of 5.1 and passes everything else to the handler.
func (s *Server) answer(c *client, req protocol.Request) []byte {
	s.mu.Lock()
	hello, mismatch := c.hello, c.mismatch
```

Заменить на:

```go
	}
}

// answer applies the hello rules of 5.1, answers activity.subscribe itself (activity.go) and passes
// everything else to the handler.
func (s *Server) answer(c *client, req protocol.Request) []byte {
	s.mu.Lock()
	hello, mismatch := c.hello, c.mismatch
```

Найти:

```go
			Message: "only helper.shutdown is accepted after a protocol mismatch", Protocol: protocol.Version})
	case !hello && !mismatch:
		return protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "the first message must be hello"))
	}
	res, err := s.h.Handle(context.Background(), req.Method, req.Params)
	if err != nil {
```

Заменить на:

```go
			Message: "only helper.shutdown is accepted after a protocol mismatch", Protocol: protocol.Version})
	case !hello && !mismatch:
		return protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "the first message must be hello"))
	case req.Method == protocol.MethodActivitySubscribe:
		return s.subscribe(c, req)
	}
	res, err := s.h.Handle(context.Background(), req.Method, req.Params)
	if err != nil {
```

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
	eng.Start()
	go eng.Run(ctx)
	go srv.Serve()
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
```

Заменить на:

```go
	eng.Start()
	go eng.Run(ctx)
	go srv.Serve()
	go srv.RunActivity(eng, time.Second) // the activity event once a second (spec 8.4)
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
```

- [ ] **Step 16: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/pipe/ -count=1 -v; go vet ./cmd/splitlane-helper/
```

Ожидается `--- PASS: TestActivitySubscribe`, `--- PASS: TestResubscribeReplaces`, `--- PASS: TestStalledSubscriberIsDropped`, `--- PASS: TestNoActivityEventWhenSourceSaysNo`, прежние тесты пакета тоже `PASS`, `ok  	splitlane/helper/internal/pipe`; `go vet` ничего не выводит.

- [ ] **Step 17: Написать тесты `slctl activity` и закрытой ссылки для входа**

Поддельный служебный процесс тестов `slctl` становится и источником активности для `RunActivity`.

Изменить `helper/cmd/slctl/main_test.go`:

Найти:

```go
	release  chan struct{}        // lets the silent methods return
	stateErr *protocol.Error      // state.get is rejected with it
	login    []protocol.Tailscale // the node's states after tailscale.login, one state event each, 30 ms apart
}

func (f *fakeHelper) set(phase string) {
```

Заменить на:

```go
	release  chan struct{}        // lets the silent methods return
	stateErr *protocol.Error      // state.get is rejected with it
	login    []protocol.Tailscale // the node's states after tailscale.login, one state event each, 30 ms apart

	activity *protocol.Activity                 // the live activity (spec 8.4); nil: no core with the adapter runs
	details  map[string]protocol.ActivityDetail // the detail of each group key
}

// Activity is the engine's summary for the pipe's activity events: the detail of detailFor when it has one.
func (f *fakeHelper) Activity(detailFor string) (protocol.Activity, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.activity == nil {
		return protocol.Activity{}, false
	}
	a := *f.activity
	if d, ok := f.details[detailFor]; ok {
		a.Detail = &d
	}
	return a, true
}

func (f *fakeHelper) set(phase string) {
```

Найти:

```go
	}
}

func TestEnableWaitsForOn(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
```

Заменить на:

```go
	}
}

// Addition of plan 1B: the RESULT lines carry the raw JSON of the answers, tailscale.authUrl of state.get
// among them. The login link is masked there as in every line slctl prints, with -show-addresses too;
// only slctl login prints it, once, in its own line (TestLoginPrintsTheLinkOnce).
func TestResultLinesHideTheLoginLink(t *testing.T) {
	link := "https://login.example/a/1f2e3d?next=%2F&x=1"
	name := testPipeName()
	srv, err := serveState(name, protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{},
		Tailscale: protocol.Tailscale{Backend: protocol.BackendNeedsLogin, AuthURL: &link}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	for _, args := range [][]string{{"state"}, {"-show-addresses", "state"}, {"call", "state.get"}} {
		var out strings.Builder
		if code := run(append([]string{"-pipe", name}, args...), &out); code != exitOK {
			t.Fatalf("%v: %d\n%s", args, code, out.String())
		}
		if text := out.String(); strings.Contains(text, "1f2e3d") || !strings.Contains(text, `"authUrl":"<login link>"`) {
			t.Errorf("%v:\n%s", args, text)
		}
	}
	for line, want := range map[string]string{
		`RESULT {"authUrl":null,"backend":"stopped"}`:  `RESULT {"authUrl":null,"backend":"stopped"}`,
		`{"authUrl":"https://x.example/a\"b\\","n":1}`: `{"authUrl":"<login link>","n":1}`,
		`{"authUrl" : "https://x.example/"}`:           `{"authUrl":"<login link>"}`,
	} {
		if got := hideLoginLink(line); got != want {
			t.Errorf("%s: %s", line, got)
		}
	}
}

// slctl activity (spec 8.4, task 16): activity.subscribe, then one ACTIVITY line per event for -seconds,
// the busiest group as top (its name, or its key when it has none); with -detail also the DETAIL line of
// that group. Exit code 0 when at least one event came, 3 when none did (no core with the adapter runs).
func TestActivityCommand(t *testing.T) {
	steam := "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}},
		activity: &protocol.Activity{
			Lanes: protocol.ActivityLanes{VPN: protocol.Traffic{Down: 5000, Up: 300}, Direct: protocol.Traffic{Down: 400, Up: 20}},
			Apps: []protocol.ActivityApp{
				{Key: `C:\Games\Game\`, Kind: protocol.KindApp, Name: "Game", Lane: protocol.LaneVPN, Down: 5000, Up: 300, Connections: 3},
				{Key: steam, Kind: protocol.KindApp, Name: "Steam Client", RuleID: &steam, Lane: protocol.LaneDirect, Down: 400, Up: 20, Connections: 2},
				{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: protocol.LaneVPN},
			}},
		details: map[string]protocol.ActivityDetail{steam: {Key: steam, Exes: []string{`C:\Games\Steam\steam.exe`, `C:\Games\Steam\bin\helper.exe`},
			Connections: []protocol.ActivityConn{{Host: "203.0.113.7", Lane: protocol.LaneDirect, Down: 400, Up: 20}}}},
	}
	name := testPipeName()
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	go srv.RunActivity(f, 50*time.Millisecond)
	t.Cleanup(srv.Close)
	var out strings.Builder
	code := run([]string{"-pipe", name, "activity", "-seconds", "1"}, &out)
	text := out.String()
	if code != exitOK || !strings.Contains(text, "ACTIVITY vpn=5000/300 direct=400/20 apps=3 top=Game:vpn:5300\n") || strings.Contains(text, "DETAIL") {
		t.Fatalf("exit %d:\n%s", code, text)
	}
	out.Reset()
	code = run([]string{"-pipe", name, "activity", "-seconds", "1", "-detail", steam}, &out)
	text = out.String()
	if code != exitOK || !strings.Contains(text, "ACTIVITY vpn=5000/300") || !strings.Contains(text, "DETAIL key="+steam+" exes=2 connections=1\n") {
		t.Fatalf("with -detail, exit %d:\n%s", code, text)
	}
	if got := activityLine(protocol.Activity{Apps: []protocol.ActivityApp{{Key: protocol.KeyUnattributed, Lane: protocol.LaneVPN, Down: 1, Up: 2}}}); got != "ACTIVITY vpn=0/0 direct=0/0 apps=1 top=unattributed:vpn:3" {
		t.Fatalf("a group without a name: %q", got)
	}
	f.mu.Lock()
	f.activity = nil
	f.mu.Unlock()
	out.Reset()
	start := time.Now()
	code = run([]string{"-pipe", name, "activity", "-seconds", "1"}, &out)
	if text = out.String(); code != exitTimeout || strings.Contains(text, "ACTIVITY") || !strings.Contains(text, "SLCTL: no activity event") {
		t.Fatalf("no core: exit %d:\n%s", code, text)
	}
	if waited := time.Since(start); waited < 900*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("waited %s for -seconds 1", waited)
	}
	if code := run([]string{"-pipe", name, "activity", "-seconds", "0"}, &out); code != exitUsage {
		t.Fatalf("-seconds 0: %d", code)
	}
}

func TestEnableWaitsForOn(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
```

- [ ] **Step 18: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestActivityCommand|TestResultLinesHideTheLoginLink' -count=1
```

Ожидается `FAIL	splitlane/helper/cmd/slctl [build failed]` и `undefined: hideLoginLink`, `undefined: activityLine`.

- [ ] **Step 19: Реализовать `slctl activity` и закрыть ссылку в выводе**

Событие `activity` печатает `read` (как событие `state`), поэтому строки `ACTIVITY` и `DETAIL` появляются и пока `call` ждёт ответа на `activity.subscribe`. Окончание `-seconds` — обычный конец команды, а не тайм-аут ответа.

Изменить `helper/cmd/slctl/main.go`:

Найти:

```go
// Command slctl is the test client of the helper's pipe (Plan 1): every method of spec 5.1, waiting
// for phases, a lane check with renamed copies of curl.exe, and a development seed of state.json.
// Addresses are masked in the output unless -show-addresses is given.
package main

import (
```

Заменить на:

```go
// Command slctl is the test client of the helper's pipe (Plan 1): every method of spec 5.1, waiting
// for phases, a lane check with renamed copies of curl.exe, and a development seed of state.json.
// Addresses are masked in the output unless -show-addresses is given; the login link always is, but
// in the one line of slctl login that shows it.
package main

import (
```

Найти:

```go
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
```

Заменить на:

```go
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
```

Найти:

```go
  clear-system-exit-node         systemTailscale.clearExitNode
  login [-timeout 300s]          tailscale.login, print the login link once, wait until logged in
  logout [-timeout 60s]          tailscale.logout, wait for backend needs_login
  shutdown                       helper.shutdown, wait until the pipe closes
  call METHOD [PARAMS|@FILE]     any method, params as JSON or from a file
  wait PHASE[,PHASE...]          wait for one of the phases; success when it is the first one
```

Заменить на:

```go
  clear-system-exit-node         systemTailscale.clearExitNode
  login [-timeout 300s]          tailscale.login, print the login link once, wait until logged in
  logout [-timeout 60s]          tailscale.logout, wait for backend needs_login
  activity [-seconds 10] [-detail KEY]   activity.subscribe, print each event; 3 when none came
  shutdown                       helper.shutdown, wait until the pipe closes
  call METHOD [PARAMS|@FILE]     any method, params as JSON or from a file
  wait PHASE[,PHASE...]          wait for one of the phases; success when it is the first one
```

Найти:

```go
// pipeCommands are the commands that talk to the helper.
var pipeCommands = []string{"state", "enable", "disable", "alldirect", "lanes", "exitnode", "clear-system-exit-node",
	"login", "logout", "shutdown", "call", "wait", "watch", "expect-lanes"}

// A helper that was just started opens its pipe a moment later (its log, the SHA-256 of the core,
// the engine). go-winio retries only a busy pipe, so slctl tries a missing one again itself.
```

Заменить на:

```go
// pipeCommands are the commands that talk to the helper.
var pipeCommands = []string{"state", "enable", "disable", "alldirect", "lanes", "exitnode", "clear-system-exit-node",
	"login", "logout", "activity", "shutdown", "call", "wait", "watch", "expect-lanes"}

// A helper that was just started opens its pipe a moment later (its log, the SHA-256 of the core,
// the engine). go-winio retries only a busy pipe, so slctl tries a missing one again itself.
```

Найти:

```go
func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

type client struct {
	conn    net.Conn
	r       *bufio.Reader
	out     io.Writer
	show    bool
	nextID  int
	state   *protocol.State
	timeout time.Time
	lastErr error
	onState func(protocol.State) // sees every state that comes, by event or by state.get (login, logout)
}

func (c *client) print(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if !c.show {
		line = protocol.MaskAddresses(line)
	}
	fmt.Fprintln(c.out, line)
}

// read returns the next message; events update the known state and are printed.
func (c *client) read() (map[string]json.RawMessage, error) {
	c.conn.SetReadDeadline(c.timeout)
```

Заменить на:

```go
func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

type client struct {
	conn       net.Conn
	r          *bufio.Reader
	out        io.Writer
	show       bool
	nextID     int
	state      *protocol.State
	timeout    time.Time
	lastErr    error
	onState    func(protocol.State)    // sees every state that comes, by event or by state.get (login, logout)
	onActivity func(protocol.Activity) // sees every activity event (activity)
}

// print writes one line: addresses masked unless -show-addresses is given, the login link always.
func (c *client) print(format string, args ...any) {
	line := hideLoginLink(fmt.Sprintf(format, args...))
	if !c.show {
		line = protocol.MaskAddresses(line)
	}
	fmt.Fprintln(c.out, line)
}

// authURLRe is tailscale.authUrl with a link in the JSON of a state (RESULT lines).
var authURLRe = regexp.MustCompile(`"authUrl"\s*:\s*"(?:[^"\\]|\\.)*"`)

// hideLoginLink masks the login link in the JSON of a state: it goes only to the user's console, in the
// one line of slctl login, never into a report or a log (Global Constraints of plan 1B).
func hideLoginLink(line string) string {
	return authURLRe.ReplaceAllString(line, `"authUrl":"<login link>"`)
}

// read returns the next message; events update the known state and are printed.
func (c *client) read() (map[string]json.RawMessage, error) {
	c.conn.SetReadDeadline(c.timeout)
```

Найти:

```go
	if ev, ok := m["event"]; ok {
		var name string
		json.Unmarshal(ev, &name)
		if name == "state" {
			var s protocol.State
			json.Unmarshal(m["data"], &s)
			c.state = &s
```

Заменить на:

```go
	if ev, ok := m["event"]; ok {
		var name string
		json.Unmarshal(ev, &name)
		switch name {
		case "state":
			var s protocol.State
			json.Unmarshal(m["data"], &s)
			c.state = &s
```

Найти:

```go
			if c.onState != nil {
				c.onState(s)
			}
		} else {
			c.print("EVENT %s", name)
		}
	}
```

Заменить на:

```go
			if c.onState != nil {
				c.onState(s)
			}
		case protocol.EventActivity:
			var a protocol.Activity
			json.Unmarshal(m["data"], &a)
			c.print("%s", activityLine(a))
			if d := a.Detail; d != nil {
				c.print("DETAIL key=%s exes=%d connections=%d", d.Key, len(d.Exes), len(d.Connections))
			}
			if c.onActivity != nil {
				c.onActivity(a)
			}
		default:
			c.print("EVENT %s", name)
		}
	}
```

Найти:

```go
		s.SystemTailscale.ExitNodeOn, s.SystemTailscale.RestoreOnDisable, s.Tailscale.Backend, str(s.Tailscale.Path), len(s.Tailscale.ExitNodes))
}

func str(p *string) string {
	if p == nil {
		return "null"
```

Заменить на:

```go
		s.SystemTailscale.ExitNodeOn, s.SystemTailscale.RestoreOnDisable, s.Tailscale.Backend, str(s.Tailscale.Path), len(s.Tailscale.ExitNodes))
}

// activityLine is an activity event in one line (spec 8.4): the rates of both lanes as down/up in bytes
// a second, the number of groups, and the busiest group (the first of the summary) with its name, or
// its key when it has none, its lane and its down+up.
func activityLine(a protocol.Activity) string {
	top := "-"
	if len(a.Apps) > 0 {
		g := a.Apps[0]
		name := g.Name
		if name == "" {
			name = g.Key
		}
		top = fmt.Sprintf("%s:%s:%d", name, g.Lane, g.Down+g.Up)
	}
	return fmt.Sprintf("ACTIVITY vpn=%d/%d direct=%d/%d apps=%d top=%s",
		a.Lanes.VPN.Down, a.Lanes.VPN.Up, a.Lanes.Direct.Down, a.Lanes.Direct.Up, len(a.Apps), top)
}

func str(p *string) string {
	if p == nil {
		return "null"
```

Найти:

```go
		return simple(protocol.MethodClearSystemExitNode, nil)
	case "login", "logout":
		return nodeCommand(c, cmd, args)
	case "shutdown":
		if code := simple(protocol.MethodShutdown, nil); code != exitOK {
			return code
```

Заменить на:

```go
		return simple(protocol.MethodClearSystemExitNode, nil)
	case "login", "logout":
		return nodeCommand(c, cmd, args)
	case "activity":
		return activityCommand(c, args)
	case "shutdown":
		if code := simple(protocol.MethodShutdown, nil); code != exitOK {
			return code
```

Найти:

```go
	return (b == protocol.BackendStarting || b == protocol.BackendRunning) && s.Tailscale.AuthURL == nil
}

// laneName says which lane an address belongs to.
func laneName(ip string, s protocol.State) string {
	switch {
```

Заменить на:

```go
	return (b == protocol.BackendStarting || b == protocol.BackendRunning) && s.Tailscale.AuthURL == nil
}

// activityCommand is activity.subscribe (spec 5.1, 8.4), with -detail KEY as detailFor, then the events
// for -seconds: read prints an ACTIVITY line for each, and the DETAIL line of the group when the event has
// it. Exit code 0 when at least one event came, 3 when none did: the event goes only while the core with
// the adapter runs (on, degraded, blocked).
func activityCommand(c *client, args []string) int {
	fs := flag.NewFlagSet("activity", flag.ContinueOnError)
	fs.SetOutput(c.out)
	seconds := fs.Int("seconds", 10, "how long to print the events")
	detail := fs.String("detail", "", "the key of a group (spec 8.4): print its detail too")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *seconds <= 0 {
		return exitUsage
	}
	events := 0
	c.onActivity = func(protocol.Activity) { events++ }
	var params any
	if *detail != "" {
		params = protocol.SubscribeParams{DetailFor: *detail}
	}
	_, perr, err := c.call(protocol.MethodActivitySubscribe, params)
	switch {
	case err != nil:
		return c.fail(err)
	case perr != nil:
		return exitFailed // call printed its ERROR line
	}
	c.timeout = time.Now().Add(time.Duration(*seconds) * time.Second)
	for {
		if _, err := c.read(); err != nil {
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				return c.fail(err)
			}
			break
		}
	}
	if events == 0 {
		c.print("SLCTL: no activity event in %d s: it comes only while the core with the adapter runs (on, degraded, blocked)", *seconds)
		return exitTimeout
	}
	c.print("SLCTL: %d activity events in %d s", events, *seconds)
	return exitOK
}

// laneName says which lane an address belongs to.
func laneName(ip string, s protocol.State) string {
	switch {
```

- [ ] **Step 20: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestActivityCommand|TestResultLinesHideTheLoginLink|TestLoginPrintsTheLinkOnce|TestSummaryShowsTheConnector' -count=1 -v
```

Ожидается `--- PASS: TestSummaryShowsTheConnector`, `--- PASS: TestResultLinesHideTheLoginLink`, `--- PASS: TestActivityCommand` (около 3 секунд: три запуска по `-seconds 1`), `--- PASS: TestLoginPrintsTheLinkOnce` и `ok  	splitlane/helper/cmd/slctl`.

- [ ] **Step 21: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/ ./internal/pipe/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; `ok` у всех шестнадцати пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`); пятикратный прогон — `ok  	splitlane/helper/internal/engine` и `ok  	splitlane/helper/internal/pipe`.

- [ ] **Step 22: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/activity helper/internal/singbox/clash.go helper/internal/singbox/clash_test.go helper/internal/engine helper/internal/pipe helper/cmd/splitlane-helper/main.go helper/cmd/slctl
git -C C:\code\router commit -m "feat(helper): live activity: the core's connection counters, activity.subscribe in the pipe, the activity event; slctl activity and the login link hidden in RESULT lines" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 12: diagnostics.export

5.1: `diagnostics.export` собирает журналы служебного процесса и ядра и снимок состояния в архив в `export\` каталога данных (4.2) и возвращает полный путь (`{"path": "..."}`). Метод работает в любой фазе и идёт мимо очереди команд, меняющих состояние: архив только читает; ответ приходит, когда архив записан. Параметров у метода нет: `{}` или их отсутствие, всё остальное — `bad_request`. Новый пакет `internal/diag` строит zip из того, что ему передали: `logs/<имя>` для каждого журнала из `store.Dir.Logs` (`helper.log`, `helper.log.prev`, `core.log`, `core.log.prev` в `logs\`), `state.json` — сохранённое состояние этого запуска (то, что движок держит для `state.json`), `meta.txt` — строки `ключ: значение` по алфавиту: версии служебного процесса и ядра, фаза, код причины (`-` без причины), время, `journal: present|absent` (нечитаемый журнал считается присутствующим, как для `restoreOnDisable`). Журнала, которого нет (у первого запуска нет `.prev`), в архиве нет, его имя стоит в строке `missing logs`. Конфиг ядра, файлы правил, каталог `tailscale\`, `restore.json` и секреты в архив не попадают (5.1, 11). Архив пишется во временный файл в той же папке и переименовывается (`atomicfile`): окно никогда не встретит половину архива. Имя — `splitlane-diagnostics-YYYYMMDD-HHMMSS.zip` по местному времени часов движка. В `export\` остаются пять архивов: только что записанный и четыре самых новых из прежних по имени (в имени время); он остаётся, даже если часы ушли назад и его имя старше всех. Прочие файлы папки не трогаются. `slctl export` печатает `SLCTL: export written: <путь>`.

Решение плана 1B: журнал ядра во время работы не обрезается; архив берёт не больше последних 8 МиБ каждого журнала, первой строкой тогда идёт `[splitlane: the first N bytes were left out]`. Берутся только целые строки: строка, которую разрезала граница, выпадает целиком (от половины ссылки для входа не осталось бы `https://`, её нельзя было бы найти и закрыть). Ссылка для входа встроенного узла в архив не попадает (Global Constraints): ядро печатает её в строке `Waiting for authentication` (`coreconfig.LoginLine`), Tailscale может напечатать её отдельной строкой. Любая ссылка сервера входа `https://login.tailscale.com/...`, любая `https://<узел>/a/<токен>` и любая ссылка в строке `Waiting for authentication` и в строке после неё заменяются на `<login link>`. Ошибки `NodeAPI` попадают в `helper.log` уже закрытыми (`protocol.MaskAddresses`; это закрепляют `TestNodeErrorsAreMasked` и `TestNodeErrorsWhileOnAreMasked` задач 8 и 9), задача не добавляет вызовов `NodeAPI` и новых записей его ошибок; адреса соединений в `core.log` остаются, это сведения для диагностики. Папку `export\` с правом чтения для пользователя создаёт `install` задачи 14; если её нет (разработка), `diag.Build` создаёт обычную папку.

**Files:**
- Create: `helper/internal/diag/diag.go`, `helper/internal/diag/diag_test.go`, `helper/internal/engine/diagnostics.go`, `helper/internal/engine/diagnostics_test.go`
- Modify: `helper/internal/store/store.go`, `helper/internal/protocol/protocol.go`, `helper/internal/engine/ports.go`, `helper/internal/engine/commands.go`, `helper/cmd/slctl/main.go`
- Test: `helper/internal/store/store_test.go`, `helper/internal/engine/fakes_test.go`, `helper/internal/engine/commands_test.go`, `helper/cmd/slctl/main_test.go`

**Interfaces:**
- Consumes: `atomicfile.Write(path string, data []byte) error`; `coreconfig.PathsFor(dataDir string) coreconfig.Paths` (`LogsDir`, `CoreLog`); `coreconfig.LoginLine`; `protocol.MethodDiagnosticsExport`, `protocol.DecodeParams`; `Clock.Now() time.Time` движка.
- Produces: `type diag.Input struct{ Logs []string; State []byte; Meta map[string]string }`; `func diag.Build(zipPath string, in diag.Input) error`; `func (d store.Dir) ExportDir() string` (`<data>\export`); `func (d store.Dir) Logs() []string` (`helper.log`, `helper.log.prev`, `core.log`, `core.log.prev` в `<data>\logs\`); методы `ExportDir() string` и `Logs() []string` интерфейса `engine.Store`; `type protocol.ExportResult struct{ Path string }` (JSON `path`); команда `slctl export`.

- [ ] **Step 1: Написать тест путей архива и журналов в хранилище**

Изменить `helper/internal/store/store_test.go`:

Найти:

```go
	"path/filepath"
	"strconv"
```

Заменить на:

```go
	"path/filepath"
	"slices"
	"strconv"
```

Найти:

```go
	for _, sub := range []string{"logs", "tailscale"} {
		if _, err := os.Stat(filepath.Join(d.Path, sub)); err != nil {
			t.Errorf("folder %s missing", sub)
		}
	}
}
```

Заменить на:

```go
	for _, sub := range []string{"logs", "tailscale"} {
		if _, err := os.Stat(filepath.Join(d.Path, sub)); err != nil {
			t.Errorf("folder %s missing", sub)
		}
	}
}

// diagnostics.export (spec 5.1, 4.2): the archives go into export\ of the data folder; the logs it takes are
// the helper's and the core's in logs\, each with the one of the run before. Nothing is created.
func TestExportPaths(t *testing.T) {
	d := Dir{Path: `C:\data`}
	if got := d.ExportDir(); got != `C:\data\export` {
		t.Fatalf("export folder %q", got)
	}
	want := []string{`C:\data\logs\helper.log`, `C:\data\logs\helper.log.prev`, `C:\data\logs\core.log`, `C:\data\logs\core.log.prev`}
	if got := d.Logs(); !slices.Equal(got, want) {
		t.Fatalf("logs %q", got)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/store/ -run 'TestExportPaths' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/store [build failed]` и `d.ExportDir undefined (type Dir has no field or method ExportDir)`, `d.Logs undefined (type Dir has no field or method Logs)`.

- [ ] **Step 3: Назвать папку архивов и журналы в хранилище**

Изменить `helper/internal/store/store.go`:

Найти:

```go
// Package store keeps the helper's files in the data folder: state.json (spec 5.2),
// the restore journal restore.json (spec 5.7), the rule files and the core config.
```

Заменить на:

```go
// Package store keeps the helper's files in the data folder: state.json (spec 5.2),
// the restore journal restore.json (spec 5.7), the rule files and the core config; it names
// the logs and the folder of the diagnostics archives (spec 5.1, 4.2).
```

Найти:

```go
	return p.Config, atomicfile.Write(p.Config, config)
}
```

Заменить на:

```go
	return p.Config, atomicfile.Write(p.Config, config)
}

// ExportDir is the folder of the diagnostics archives, export\ of the data folder (spec 4.2): the one
// folder that the user who set SplitLane up may read.
func (d Dir) ExportDir() string { return filepath.Join(d.Path, "export") }

// Logs are the logs that diagnostics.export takes (spec 5.1): the helper's and the core's in logs\,
// each with the one of the run before (.prev).
func (d Dir) Logs() []string {
	p := coreconfig.PathsFor(d.Path)
	helper := filepath.Join(p.LogsDir, "helper.log")
	return []string{helper, helper + ".prev", p.CoreLog, p.CoreLog + ".prev"}
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/store/ -run 'TestExportPaths' -count=1 -v
```

Ожидается `--- PASS: TestExportPaths` и `ok  	splitlane/helper/internal/store`.

- [ ] **Step 5: Написать тесты архива**

Четыре теста пакета `diag`: хвост длинного журнала по целым строкам (граница посреди строки и граница сразу после конца строки), закрытая ссылка для входа во всех её видах, пропущенный журнал в `meta.txt`, в архиве нет ничего, кроме переданного, и рядом не остаётся временного файла.

Файл `helper/internal/diag/diag_test.go`, целиком:

```go
package diag

import (
	"archive/zip"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write puts text into path, creating its folder.
func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// entries reads the archive: the content of each entry by its name.
func entries(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// The decision of plan 1B: the core's log is never cut while it is written; the archive takes at most the
// last 8 MiB of each log. A longer log goes in by the whole lines of its last 8 MiB, after a first line that
// says how many bytes were left out: a cut in the middle of a line leaves that line out too (half a login
// link would not be found and hidden), a cut right after a line end keeps the next line. A shorter log goes
// in whole, without that line.
func TestBuildTakesTheTail(t *testing.T) {
	dir := t.TempDir()
	var middle strings.Builder // lines of 100 bytes: the cut falls in the middle of the first line
	for n := 0; middle.Len() < maxLog+50; n++ {
		fmt.Fprintf(&middle, "%099d\n", n)
	}
	var edge strings.Builder // one line of 18 bytes, then exactly 8 MiB of lines of 64 bytes
	edge.WriteString("this line is left\n")
	for n := 0; n < maxLog/64; n++ {
		fmt.Fprintf(&edge, "%063d\n", n)
	}
	logs := []string{
		write(t, filepath.Join(dir, "logs", "core.log"), middle.String()),
		write(t, filepath.Join(dir, "logs", "core.log.prev"), edge.String()),
		write(t, filepath.Join(dir, "logs", "helper.log"), "short\n"),
	}
	zipPath := filepath.Join(dir, "export", "a.zip")
	if err := Build(zipPath, Input{Logs: logs, State: []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	if middle.Len() != maxLog+92 {
		t.Fatalf("core.log has %d bytes", middle.Len())
	}
	got := entries(t, zipPath)
	// the cut is 92 bytes into the first line of 100 bytes: that whole line is left out
	if want := "[splitlane: the first 100 bytes were left out]\n" + middle.String()[100:]; got["logs/core.log"] != want {
		t.Errorf("core.log: %d bytes, starts %q", len(got["logs/core.log"]), got["logs/core.log"][:min(80, len(got["logs/core.log"]))])
	}
	if want := "[splitlane: the first 18 bytes were left out]\n" + edge.String()[18:]; got["logs/core.log.prev"] != want {
		t.Errorf("core.log.prev: %d bytes, starts %q", len(got["logs/core.log.prev"]), got["logs/core.log.prev"][:min(80, len(got["logs/core.log.prev"]))])
	}
	if got["logs/helper.log"] != "short\n" {
		t.Errorf("helper.log: %q", got["logs/helper.log"])
	}
}

// The login link of the node never goes into the archive (Global Constraints of plan 1B): the core prints it
// in the line of "Waiting for authentication", Tailscale may print it in a line of its own. Every link of the
// login server and every https://<host>/a/<token> becomes <login link>, and so does every link in the line of
// "Waiting for authentication" and in the line after it. Other links and lines stay as they are.
func TestBuildRedactsLoginLinks(t *testing.T) {
	dir := t.TempDir()
	core := write(t, filepath.Join(dir, "logs", "core.log"), strings.Join([]string{
		"+0000 2026-11-14 12:00:00 INFO endpoint/tailscale[ts-ep]: Waiting for authentication: https://login.tailscale.com/a/1f2e3d4c5b",
		"+0000 2026-11-14 12:00:01 DEBUG endpoint/tailscale[ts-ep]: control: AuthURL is https://login.tailscale.com/a/6a7b8c9d0e",
		"+0000 2026-11-14 12:00:02 INFO endpoint/tailscale[ts-ep]: Waiting for authentication:",
		"https://control.example/register/nodekey:abcdef0123",
		"+0000 2026-11-14 12:00:03 INFO endpoint/tailscale[ts-ep]: visit https://control.example/a/77aa88bb",
		"+0000 2026-11-14 12:00:04 INFO outbound/direct[direct]: outbound connection to api.ipify.org:443",
		"",
	}, "\n"))
	helper := write(t, filepath.Join(dir, "logs", "helper.log"),
		"2026/11/14 12:00:05 probe https://api.ipify.org: ok\n2026/11/14 12:00:06 {\"authUrl\":\"https://login.tailscale.com/a/99ff\"}\n")
	zipPath := filepath.Join(dir, "export", "b.zip")
	if err := Build(zipPath, Input{Logs: []string{helper, core}, State: []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	all := got["logs/core.log"] + got["logs/helper.log"]
	for _, token := range []string{"1f2e3d4c5b", "6a7b8c9d0e", "nodekey:abcdef0123", "77aa88bb", "99ff", "login.tailscale.com"} {
		if strings.Contains(all, token) {
			t.Errorf("%q is in the archive", token)
		}
	}
	if n := strings.Count(all, "<login link>"); n != 5 {
		t.Errorf("%d login links hidden, want 5", n)
	}
	for _, kept := range []string{"Waiting for authentication: <login link>\n", "AuthURL is <login link>\n", "visit <login link>\n",
		"outbound connection to api.ipify.org:443\n", "probe https://api.ipify.org: ok\n", `{"authUrl":"<login link>"}`} {
		if !strings.Contains(all, kept) {
			t.Errorf("missing %q", kept)
		}
	}
	if t.Failed() {
		t.Fatalf("archive:\n%s", all)
	}
}

// A log that is not there (a first run has no .prev) is left out of the archive and named in meta.txt.
// meta.txt is one "key: value" line per key, sorted by key; state.json holds the bytes given.
func TestBuildSkipsMissingLogs(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	helper := write(t, filepath.Join(logs, "helper.log"), "helper line\n")
	zipPath := filepath.Join(dir, "export", "c.zip")
	state := "{\n  \"version\": 1\n}\n"
	err := Build(zipPath, Input{
		Logs:  []string{helper, filepath.Join(logs, "helper.log.prev"), filepath.Join(logs, "core.log"), filepath.Join(logs, "core.log.prev")},
		State: []byte(state),
		Meta:  map[string]string{"phase": "off", "helper": "0.1.0", "core": "1.14.4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/helper.log", "meta.txt", "state.json"}) {
		t.Fatalf("entries %v", names)
	}
	if got["state.json"] != state || got["logs/helper.log"] != "helper line\n" {
		t.Fatalf("state.json %q, helper.log %q", got["state.json"], got["logs/helper.log"])
	}
	if want := "core: 1.14.4\nhelper: 0.1.0\nmissing logs: helper.log.prev, core.log, core.log.prev\nphase: off\n"; got["meta.txt"] != want {
		t.Fatalf("meta.txt:\n%s", got["meta.txt"])
	}
}

// The archive holds the logs it is given, state.json and meta.txt, nothing else of the data folder (spec 5.1,
// 11): not the core config with the secrets of the run, not the node's state folder with its key, not the
// restore journal, not the rule files. It is written to a temp file in the same folder and renamed: no temp
// file is left beside it.
func TestBuildNeverAddsOtherFiles(t *testing.T) {
	data := t.TempDir()
	const secret = "s3cr3t-0123456789abcdef"
	write(t, filepath.Join(data, "core-config.json"), `{"experimental":{"clash_api":{"secret":"`+secret+`"}}}`)
	write(t, filepath.Join(data, "tailscale", "tailscaled.state"), `{"_machinekey":"privkey:`+secret+`"}`)
	write(t, filepath.Join(data, "restore.json"), `{"exitNode":"100.64.0.1","allowLan":true}`)
	write(t, filepath.Join(data, "rules", "lane-direct.json"), `{"version":3,"rules":[]}`)
	helper := write(t, filepath.Join(data, "logs", "helper.log"), "helper line\n")
	core := write(t, filepath.Join(data, "logs", "core.log"), "core line\n")
	export := filepath.Join(data, "export")
	zipPath := filepath.Join(export, "splitlane-diagnostics-20261114-120000.zip")
	if err := Build(zipPath, Input{Logs: []string{helper, core}, State: []byte("{}\n"), Meta: map[string]string{"phase": "on"}}); err != nil {
		t.Fatal(err)
	}
	got := entries(t, zipPath)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/core.log", "logs/helper.log", "meta.txt", "state.json"}) {
		t.Fatalf("entries %v", names)
	}
	for name, text := range got {
		if strings.Contains(text, secret) {
			t.Fatalf("%s holds the secret", name)
		}
	}
	left, err := os.ReadDir(export)
	if err != nil || len(left) != 1 || left[0].Name() != filepath.Base(zipPath) {
		t.Fatalf("the export folder holds %v (%v)", left, err)
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/diag/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/diag [build failed]` и `undefined: maxLog`, `undefined: Build`, `undefined: Input`.

- [ ] **Step 7: Реализовать архив**

Файл `helper/internal/diag/diag.go`, целиком:

```go
// Package diag builds the archive of diagnostics.export (spec 5.1): the logs of the helper and of the
// core, the saved state and a few facts, as one zip. It takes only what it is given, never the core
// config, the node's state folder or a secret (spec 11), and it hides the node's login link in the logs.
package diag

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/coreconfig"
)

// maxLog bounds what the archive takes of one log: its last 8 MiB. The logs themselves are never cut
// while they are written (plan 1B).
const maxLog = 8 << 20

// Input is what one archive holds.
type Input struct {
	Logs  []string          // full paths; each goes in as logs/<base name>, one that is not there is named in meta.txt
	State []byte            // the saved state, as state.json
	Meta  map[string]string // meta.txt: one "key: value" line each, sorted by key
}

// loginLink takes the place of a login link in the logs.
const loginLink = "<login link>"

var (
	// loginRe is a link of the Tailscale login server, or a link https://<host>/a/<token> of any control server.
	loginRe = regexp.MustCompile(`https?://(?:login\.tailscale\.com|[^\s"'<>]*/a/)[^\s"'<>]*`)
	// urlRe is any link: in the line of coreconfig.LoginLine and the line after it every link is the login link.
	urlRe = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// Build writes the archive to zipPath: logs/<base name> for each log that is there (its tail, the login
// links hidden), state.json and meta.txt; the logs that are not there are named in meta.txt under
// "missing logs". The folder is created when it is not there; the archive is written to a temp file in
// it and renamed (atomicfile), so a reader never meets half an archive.
func Build(zipPath string, in Input) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now()
	meta := maps.Clone(in.Meta)
	if meta == nil {
		meta = map[string]string{}
	}
	var missing []string
	for _, path := range in.Logs {
		name := filepath.Base(path)
		data, ok, err := tail(path)
		switch {
		case err != nil:
			return fmt.Errorf("diag: log %s: %w", name, err)
		case !ok:
			missing = append(missing, name)
			continue
		}
		if err := add(zw, "logs/"+name, redact(data), now); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		meta["missing logs"] = strings.Join(missing, ", ")
	}
	if err := add(zw, "state.json", in.State, now); err != nil {
		return err
	}
	var text strings.Builder
	for _, k := range slices.Sorted(maps.Keys(meta)) {
		fmt.Fprintf(&text, "%s: %s\n", k, meta[k])
	}
	if err := add(zw, "meta.txt", []byte(text.String()), now); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(zipPath, buf.Bytes())
}

func add(zw *zip.Writer, name string, data []byte, at time.Time) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: at})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// tail reads a log as far as it was written when it was opened: a log of up to maxLog bytes whole; of a
// longer one the whole lines in its last maxLog bytes, after a first line that says how many bytes were
// left out. ok is false when the file is not there.
func tail(path string) (data []byte, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := fi.Size()
	if size <= maxLog {
		data, err = io.ReadAll(io.LimitReader(f, size))
		return data, err == nil, err
	}
	// One byte more than maxLog: when the cut falls right after a line end, that end is in buf and the
	// line after it is kept whole.
	from := size - maxLog - 1
	buf := make([]byte, maxLog+1)
	if _, err := f.ReadAt(buf, from); err != nil {
		return nil, false, err
	}
	i := bytes.IndexByte(buf, '\n')
	if i < 0 {
		i = len(buf) - 1 // one line longer than maxLog: nothing of it is taken
	}
	head := fmt.Sprintf("[splitlane: the first %d bytes were left out]\n", from+int64(i)+1)
	return append([]byte(head), buf[i+1:]...), true, nil
}

// redact hides the login links of a log: every link of the login server or of the form
// https://<host>/a/<token>, and every link in the line where the core reports the login link
// (coreconfig.LoginLine) and in the line after it.
func redact(text []byte) []byte {
	lines := bytes.SplitAfter(text, []byte("\n"))
	after := false
	for i, line := range lines {
		wait := bytes.Contains(line, []byte(coreconfig.LoginLine))
		if wait || after {
			lines[i] = urlRe.ReplaceAll(line, []byte(loginLink))
		} else {
			lines[i] = loginRe.ReplaceAll(line, []byte(loginLink))
		}
		after = wait
	}
	return bytes.Join(lines, nil)
}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/diag/ -count=1 -v
```

Ожидается `--- PASS: TestBuildTakesTheTail`, `--- PASS: TestBuildRedactsLoginLinks`, `--- PASS: TestBuildSkipsMissingLogs`, `--- PASS: TestBuildNeverAddsOtherFiles` и `ok  	splitlane/helper/internal/diag`.

- [ ] **Step 9: Написать тесты метода в движке**

Подменное хранилище получает настоящую папку данных (`dataDir`, её задаёт `exportData` только в тестах этого метода): архив пишет настоящий `diag.Build`. `TestUnknownAndLaterMethods` больше не ждёт от `diagnostics.export` ошибки `internal`.

Файл `helper/internal/engine/diagnostics_test.go`, целиком:

```go
package engine

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// exportData gives the fake store a real data folder for diagnostics.export: logs\ with the helper's log and
// the core's, the core's with the login link of the node, and the core config with the secret of the run.
func exportData(h *harness) string {
	h.t.Helper()
	data := h.t.TempDir()
	h.store.dataDir = data
	for name, text := range map[string]string{
		"logs/helper.log":  "2026/11/14 12:00:00 splitlane-helper 0.1.0 starting\n",
		"logs/core.log":    "INFO sing-box started (0.42s)\nINFO endpoint/tailscale[ts-ep]: Waiting for authentication: https://login.tailscale.com/a/1f2e3d4c\n",
		"core-config.json": `{"experimental":{"clash_api":{"secret":"0123456789abcdef0123456789abcdef"}}}`,
	} {
		path := filepath.Join(data, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
	return data
}

// export calls diagnostics.export with params and reads the path from its answer as the pipe sends it.
func export(h *harness, params any) (string, *protocol.Error) {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	res, err := h.e.Handle(context.Background(), protocol.MethodDiagnosticsExport, raw)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(res)
	var out struct {
		Path string `json:"path"`
	}
	json.Unmarshal(b, &out)
	return out.Path, nil
}

// archiveEntries reads an archive: the content of each entry by its name.
func archiveEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// archiveName is the name of an archive written now: the local time of the engine's clock.
func archiveName(h *harness) string {
	return "splitlane-diagnostics-" + h.clock.Now().Local().Format("20060102-150405") + ".zip"
}

// diagnostics.export (5.1) works in every phase and outside the queue: it answers while a state-changing
// command holds the queue. The archive is export\splitlane-diagnostics-YYYYMMDD-HHMMSS.zip in the local time of
// the engine's clock, and holds the logs with the login link hidden, state.json as it is saved and meta.txt with
// the versions, the phase, its reason, the time and the journal; never the core config.
func TestDiagnosticsExportInAnyPhase(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	state, _ := json.MarshalIndent(h.store.saved(), "", "  ")
	h.e.queue.Lock() // a state-changing command under way
	defer h.e.queue.Unlock()
	journal := "absent"
	for _, c := range []struct {
		phase string
		why   *protocol.Reason
	}{
		{protocol.PhaseOff, nil}, {protocol.PhaseStarting, nil}, {protocol.PhaseOn, nil},
		{protocol.PhaseDegraded, reason(protocol.ReasonVPNUnreachable)}, {protocol.PhaseBlocked, nil},
		{protocol.PhaseStopping, reason(protocol.ReasonRestorePending)}, {protocol.PhaseError, reason(protocol.ReasonCoreCrashed)},
	} {
		h.e.mu.Lock()
		h.e.setPhase(c.phase, c.why)
		if c.phase == protocol.PhaseOn {
			h.e.journal, journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}, "present"
		}
		h.e.mu.Unlock()
		answered := make(chan struct{})
		var path string
		var err *protocol.Error
		go func() {
			defer close(answered)
			path, err = export(h, nil)
		}()
		select {
		case <-answered:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: diagnostics.export waits for the queue", c.phase)
		}
		if err != nil || path != filepath.Join(data, "export", archiveName(h)) {
			t.Fatalf("%s: %q %v", c.phase, path, err)
		}
		got := archiveEntries(t, path)
		if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"logs/core.log", "logs/helper.log", "meta.txt", "state.json"}) {
			t.Fatalf("%s: entries %v", c.phase, names)
		}
		code := "-"
		if c.why != nil {
			code = c.why.Code
		}
		meta := "core: 1.14.4\nhelper: 0.1.0\njournal: " + journal + "\nphase: " + c.phase + "\nreason: " + code +
			"\ntime: " + h.clock.Now().Local().Format(time.RFC3339) + "\n"
		if got["meta.txt"] != meta || got["state.json"] != string(state)+"\n" {
			t.Fatalf("%s: meta.txt\n%s\nstate.json\n%s", c.phase, got["meta.txt"], got["state.json"])
		}
		if log := got["logs/core.log"]; strings.Contains(log, "1f2e3d4c") || !strings.Contains(log, "Waiting for authentication: <login link>\n") {
			t.Fatalf("%s: core.log\n%s", c.phase, log)
		}
		<-h.clock.After(time.Second) // the next archive, the next second
	}
}

// diagnostics.export keeps 5 archives in export\: the new one and the 4 newest others by name (the name carries
// the time); older archives are deleted, other files stay. The new archive stays also when the clock has gone
// back and its name is the oldest.
func TestExportKeepsFiveArchives(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	dir := filepath.Join(data, "export")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var old []string
	for n := range 6 {
		old = append(old, fmt.Sprintf("splitlane-diagnostics-20261113-10000%d.zip", n))
	}
	for _, name := range append(slices.Clone(old), "notes.txt") {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := func() []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	first, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files(), append(append([]string{"notes.txt"}, old[2:]...), filepath.Base(first)); !slices.Equal(got, want) {
		t.Fatalf("after the first export: %v, want %v", got, want)
	}
	h.clock.mu.Lock()
	h.clock.now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) // the clock has gone back
	h.clock.mu.Unlock()
	second, err := export(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]string{"notes.txt", filepath.Base(second)}, old[3:]...), filepath.Base(first))
	if got := files(); !slices.Equal(got, want) {
		t.Fatalf("after the clock went back: %v", got)
	}
}

// diagnostics.export takes no params (5.1): {} and none are fine; anything else is bad_request and writes
// nothing. An archive that cannot be written is internal, with the cause in the helper's log.
func TestExportBadParams(t *testing.T) {
	h := newHarness(t)
	data := exportData(h)
	h.load()
	for _, params := range []any{map[string]any{"x": 1}, []int{1}, "text", 1} {
		if _, err := export(h, params); err == nil || err.Code != protocol.CodeBadRequest {
			t.Fatalf("%v: %v", params, err)
		}
	}
	dir := filepath.Join(data, "export")
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused export writes nothing: %v", err)
	}
	if path, err := export(h, map[string]any{}); err != nil || filepath.Dir(path) != dir {
		t.Fatalf("{}: %q %v", path, err)
	}
	logEvents(h)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o644); err != nil { // export\ is a file: no folder can be made there
		t.Fatal(err)
	}
	if _, err := export(h, nil); err == nil || err.Code != protocol.CodeInternal {
		t.Fatalf("an archive that cannot be written: %v", err)
	}
	if h.rec.index("log:diagnostics.export failed: ") < 0 {
		t.Fatalf("not logged: %v", h.rec.list())
	}
}
```

Изменить `helper/internal/engine/fakes_test.go`:

Найти:

```go
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
```

Заменить на:

```go
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
```

Найти:

```go
	stateReads      int     // calls of LoadState
	stateSaves      int     // calls of SaveState
	journalReads    int     // calls of LoadJournal
}
```

Заменить на:

```go
	stateReads      int     // calls of LoadState
	stateSaves      int     // calls of SaveState
	journalReads    int     // calls of LoadJournal
	dataDir         string  // a real folder for diagnostics.export (exportData); "" in the other tests
}
```

Найти:

```go
func (s *fakeStore) saved() store.State {
```

Заменить на:

```go
// ExportDir and Logs are export\ and two logs in logs\ of dataDir: diagnostics.export writes a real archive.
func (s *fakeStore) ExportDir() string { return filepath.Join(s.dataDir, "export") }

func (s *fakeStore) Logs() []string {
	return []string{filepath.Join(s.dataDir, "logs", "helper.log"), filepath.Join(s.dataDir, "logs", "core.log")}
}

func (s *fakeStore) saved() store.State {
```

Изменить `helper/internal/engine/commands_test.go`:

Найти:

```go
	wantCode(t, call(h, protocol.MethodActivitySubscribe, nil), protocol.CodeBadRequest)
	wantCode(t, call(h, protocol.MethodDiagnosticsExport, nil), protocol.CodeInternal)
```

Заменить на:

```go
	wantCode(t, call(h, protocol.MethodActivitySubscribe, nil), protocol.CodeBadRequest)
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestDiagnosticsExportInAnyPhase|TestExportKeepsFiveArchives|TestExportBadParams|TestUnknownAndLaterMethods' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/engine` и три провала:

```text
--- FAIL: TestDiagnosticsExportInAnyPhase (0.00s)
    diagnostics_test.go:130: off: "" internal: diagnostics.export is not available in this build (Plan 1B)
--- FAIL: TestExportKeepsFiveArchives (0.01s)
    diagnostics_test.go:185: internal: diagnostics.export is not available in this build (Plan 1B)
--- FAIL: TestExportBadParams (0.00s)
    diagnostics_test.go:211: map[x:1]: internal: diagnostics.export is not available in this build (Plan 1B)
```

- [ ] **Step 11: Реализовать метод в движке**

Ответ метода — тип в `protocol`; `Store` получает папку архивов и список журналов; `Handle` отвечает на метод до очереди.

Изменить `helper/internal/protocol/protocol.go`:

Найти:

```go
type HelloResult struct {
	Protocol int    `json:"protocol"`
	Helper   string `json:"helper"`
	Core     string `json:"core"`
}
```

Заменить на:

```go
type HelloResult struct {
	Protocol int    `json:"protocol"`
	Helper   string `json:"helper"`
	Core     string `json:"core"`
}

// ExportResult is the answer of diagnostics.export: the full path of the archive.
type ExportResult struct {
	Path string `json:"path"`
}
```

Изменить `helper/internal/engine/ports.go`:

Найти:

```go
	WriteRules(direct, vpn []byte) error
	WriteConfig(config []byte) (string, error)
}
```

Заменить на:

```go
	WriteRules(direct, vpn []byte) error
	WriteConfig(config []byte) (string, error)
	ExportDir() string // the folder of the diagnostics archives (spec 4.2)
	Logs() []string    // the logs that diagnostics.export takes, full paths (spec 5.1)
}
```

Изменить `helper/internal/engine/commands.go`:

Найти:

```go
	case protocol.MethodDiagnosticsExport:
		return nil, protocol.Errorf(protocol.CodeInternal, "%s is not available in this build (Plan 1B)", method)
```

Заменить на:

```go
	case protocol.MethodDiagnosticsExport:
		// Any phase, outside the queue: the archive only reads the state; the answer comes when it is written.
		return e.cmdExport(params)
```

Файл `helper/internal/engine/diagnostics.go`, целиком:

```go
package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"splitlane/helper/internal/diag"
	"splitlane/helper/internal/protocol"
)

// The archives of diagnostics.export (5.1, 4.2): export\splitlane-diagnostics-YYYYMMDD-HHMMSS.zip, the
// local time of the engine's clock; so many are kept, the new one among them.
const (
	archivePrefix = "splitlane-diagnostics-"
	archiveSuffix = ".zip"
	keepArchives  = 5
)

// cmdExport is diagnostics.export (5.1): in any phase, outside the queue of the state-changing commands;
// the answer comes when the archive is written. The archive holds the logs of the helper and of the core,
// the saved state (state.json as this run holds it; never the core config, the rule files, the node's
// state folder, the journal or a secret: spec 11) and meta.txt: the versions, the phase and its reason,
// the time, and whether a restore journal is there (one that could not be read counts as there, as for
// restoreOnDisable). The state is read under e.mu; the archive is built without it.
func (e *Engine) cmdExport(params json.RawMessage) (any, *protocol.Error) {
	if err := noParams(params); err != nil {
		return nil, err
	}
	now := e.d.Clock.Now().Local()
	e.mu.Lock()
	state, err := json.MarshalIndent(e.saved, "", "  ")
	why := "-"
	if e.st.Reason != nil {
		why = e.st.Reason.Code
	}
	journal := "absent"
	if e.journal != nil || e.journalErr != nil {
		journal = "present"
	}
	meta := map[string]string{"helper": e.d.Settings.HelperVersion, "core": e.d.Settings.CoreVersion,
		"phase": e.st.Phase, "reason": why, "time": now.Format(time.RFC3339), "journal": journal}
	e.mu.Unlock()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: state: %v", err)
	}
	dir := e.d.Store.ExportDir()
	name := archivePrefix + now.Format("20060102-150405") + archiveSuffix
	path := filepath.Join(dir, name)
	in := diag.Input{Logs: e.d.Store.Logs(), State: append(state, '\n'), Meta: meta}
	if err := diag.Build(path, in); err != nil {
		e.d.Log("diagnostics.export failed: %v", err)
		return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: %v", err)
	}
	e.d.Log("diagnostics.export: %s written", name)
	e.pruneArchives(dir, name)
	return protocol.ExportResult{Path: path}, nil
}

// pruneArchives deletes the older archives in dir: the one just written (keep) stays whatever its name,
// and so do the newest of the others by name (the name carries the time; os.ReadDir sorts by name),
// keepArchives in all. Other files stay. A failure is logged: the archive is written, the export is done.
func (e *Engine) pruneArchives(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.d.Log("diagnostics.export: the older archives not listed: %v", err)
		return
	}
	var others []string
	for _, en := range entries {
		n := en.Name()
		if n != keep && en.Type().IsRegular() && strings.HasPrefix(n, archivePrefix) && strings.HasSuffix(n, archiveSuffix) {
			others = append(others, n)
		}
	}
	for _, n := range others[:max(0, len(others)-(keepArchives-1))] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			e.d.Log("diagnostics.export: the older archive %s not deleted: %v", n, err)
		}
	}
}
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'TestDiagnosticsExportInAnyPhase|TestExportKeepsFiveArchives|TestExportBadParams|TestUnknownAndLaterMethods' -count=1 -v
```

Ожидается `--- PASS: TestUnknownAndLaterMethods`, `--- PASS: TestDiagnosticsExportInAnyPhase` (между строками — записи журнала `diagnostics.export: splitlane-diagnostics-...zip written`, семь архивов по секунде), `--- PASS: TestExportKeepsFiveArchives`, `--- PASS: TestExportBadParams` и `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 13: Написать тест `slctl export`**

Подменный служебный процесс отвечает на `diagnostics.export` путём из поля `archive`, а с пустым полем — ошибкой `internal`.

Изменить `helper/cmd/slctl/main_test.go`:

Найти:

```go
	details  map[string]protocol.ActivityDetail // the detail of each group key
}
```

Заменить на:

```go
	details  map[string]protocol.ActivityDetail // the detail of each group key
	archive  string                             // the path diagnostics.export answers with; "": it fails
}
```

Найти:

```go
		return struct{}{}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
```

Заменить на:

```go
		return struct{}{}, nil
	case protocol.MethodDiagnosticsExport:
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.archive == "" {
			return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: the disk is full")
		}
		return protocol.ExportResult{Path: f.archive}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
```

Найти:

```go
func TestEnableWaitsForOn(t *testing.T) {
```

Заменить на:

```go
// slctl export (spec 5.1): diagnostics.export, then the path of the archive in one line, exit code 0. An
// error response is exit code 1 with its ERROR line; an argument is a usage error.
func TestExportCommand(t *testing.T) {
	path := `C:\ProgramData\SplitLane\export\splitlane-diagnostics-20261114-120000.zip`
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}}, archive: path}
	name := testPipeName()
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "export"}, &out); code != exitOK || !strings.Contains(out.String(), "SLCTL: export written: "+path+"\n") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	f.mu.Lock()
	f.archive = ""
	f.mu.Unlock()
	out.Reset()
	if code := run([]string{"-pipe", name, "export"}, &out); code != exitFailed ||
		!strings.Contains(out.String(), "ERROR internal diagnostics.export: the disk is full") || strings.Contains(out.String(), "export written") {
		t.Fatalf("a failed export: exit %d:\n%s", code, out.String())
	}
	if code := run([]string{"-pipe", name, "export", "now"}, &out); code != exitUsage {
		t.Fatalf("an argument: exit %d", code)
	}
}

func TestEnableWaitsForOn(t *testing.T) {
```

- [ ] **Step 14: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestExportCommand' -count=1
```

Ожидается `FAIL	splitlane/helper/cmd/slctl`, `--- FAIL: TestExportCommand` и `main_test.go:486: exit 2:` с текстом usage: команды `export` ещё нет.

- [ ] **Step 15: Реализовать `slctl export`**

Изменить `helper/cmd/slctl/main.go`:

Найти:

```text
  shutdown                       helper.shutdown, wait until the pipe closes
```

Заменить на:

```text
  export                         diagnostics.export, print the path of the archive
  shutdown                       helper.shutdown, wait until the pipe closes
```

Найти:

```go
	"login", "logout", "activity", "shutdown", "call", "wait", "watch", "expect-lanes"}
```

Заменить на:

```go
	"login", "logout", "activity", "export", "shutdown", "call", "wait", "watch", "expect-lanes"}
```

Найти:

```go
	case "activity":
		return activityCommand(c, args)
```

Заменить на:

```go
	case "activity":
		return activityCommand(c, args)
	case "export":
		if len(args) != 0 {
			return exitUsage
		}
		raw, perr, err := c.call(protocol.MethodDiagnosticsExport, nil)
		switch {
		case err != nil:
			return c.fail(err)
		case perr != nil:
			return exitFailed // call printed its ERROR line
		}
		var res protocol.ExportResult
		if json.Unmarshal(raw, &res) != nil || res.Path == "" {
			c.print("SLCTL: the answer names no archive")
			return exitFailed
		}
		c.print("SLCTL: export written: %s", res.Path)
		return exitOK
```

- [ ] **Step 16: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/ -run 'TestExportCommand' -count=1 -v
```

Ожидается `--- PASS: TestExportCommand` и `ok  	splitlane/helper/cmd/slctl`.

- [ ] **Step 17: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; `ok` у всех семнадцати пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/diag`, `internal/engine`, `internal/pipe`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`); пятикратный прогон — `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 18: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/diag helper/internal/engine helper/internal/protocol/protocol.go helper/internal/store helper/cmd/slctl
git -C C:\code\router commit -m "feat(helper): diagnostics.export: the logs and the saved state in an archive in export\, the login link hidden, five archives kept; slctl export" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 13: Режим продукта: пути, SID, выход без окна, --dev-pin только с --dev

Режим продукта (4.2, 4.3, 11). Без `--dev` служебный процесс — продукт, и всё, что в разработке задают флаги, он берёт из системы. Каталог данных — `place.DataDir()`: известная папка ProgramData (`windows.KnownFolderPath(FOLDERID_ProgramData)`) плюс `\SplitLane`. Папка программы — `place.ProgramDir()`: значение `ProgramFilesDir` из `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion` (64-битное представление реестра) плюс `\SplitLane`, но не переменная `%ProgramFiles%`: служебный процесс работает с правами администратора в окружении пользователя, а это окружение пользователь может менять (`HKCU\Environment`); с правами администратора запускаются только файлы, которые пишут администраторы (11). Значение с `%` или относительный путь — ошибка. Ядро — `<ProgramDir>\sing-box.exe` (его SHA-256 сверяется, как в разработке). Запущенный файл должен быть `<ProgramDir>\splitlane-helper.exe` (сравнение без учёта регистра), иначе строка `refused: the product runs only as ...` в журнале и код выхода 2 (`exitUsage`). SID пользователя для описателя безопасности канала (5.1) берётся из `setup.json` каталога данных (его пишет `install` задачи 14 через `place.WriteSetup`). Принимается только форма `S-1-...`, которую `windows.StringToSid` читает и отдаёт обратно без изменений: SID встаёт в SDDL канала, и псевдоним вроде `WD` («все») или лишний текст открыли бы канал другим. Нет каталога данных или `setup.json` — код выхода 6 (`exitNotInstalled`, «не установлен»). Каталог данных служебный процесс сам никогда не создаёт: его создаёт `install` с явным списком доступа без наследования (4.2), а папка, созданная здесь, унаследовала бы чужой; поэтому без каталога журнал не пишется. `--data-dir`, `--core`, `--pipe` без `--dev` отвергаются. `--dev-pin` и `--core-log-level` без `--dev` — ошибка разбора, даже со значением `false` или `info` (план 1A, «Что остаётся плану 1B», пункт 4: закрепление программ исполнителя не должно попасть в продукт); эти две проверки идут раньше проверок режима продукта. Режим разработки не меняется. Тесты не читают реестр и известные папки и не трогают `%ProgramData%`: в `place` подменяются функции поиска, в `cmd/splitlane-helper` — `programDir`, `productData` и `executable`, а `init` тестового файла заранее ставит отказ на обе папки.

Своё ядро (5.7): `singbox.Driver.Find` теперь требует, чтобы путь образа процесса совпадал не только с записанным в `state.json`, но и с `sing-box.exe` самого драйвера; в продукте это `<ProgramDir>\sing-box.exe`. Процесс другого `sing-box.exe` с тем же номером и временем создания служебный процесс не трогает.

Выход без окна (4.3; раздел 10, «Перезагрузка»). `(*Engine).ExitIfIdle(after)` ждёт конца старта 5.7, то есть возврата по журналу: пока возврат не подтверждён, старт не кончается и отсчёт не идёт. Затем ждёт `after` по часам движка. Если с начала работы не подключилось ни одно окно (клиент канала, приславший `hello`; `slctl` тоже считается), старт не включал деление по «было включено» и фаза `off`, вызывается тот же путь, что и для `helper.shutdown` (`cmdShutdown` под очередью команд), и процесс завершается. Иначе ничего не происходит, в журнал пишется причина. Окно, которое подключилось и ушло, тоже оставляет процесс работать. Решение задачи: в фазе `error` (например, `adapter_left` после остановки оставшегося ядра) процесс остаётся, чтобы окно показало причину; раздел 4.3 этот случай отдельно не называет. `main` вызывает `ExitIfIdle` с 60 секундами только в режиме продукта, в отдельной горутине.

**Files:**
- Create: `helper/internal/place/place.go`, `helper/internal/place/place_test.go`
- Modify: `helper/internal/singbox/driver.go`, `helper/internal/engine/engine.go`, `helper/internal/engine/startup.go`, `helper/cmd/splitlane-helper/main.go`
- Test: `helper/internal/singbox/driver_test.go`, `helper/internal/engine/startup_test.go`, `helper/cmd/splitlane-helper/main_test.go`

**Interfaces:**
- Consumes: `atomicfile.Write(path string, data []byte) error`; `singbox.VerifyCore(exe string) error`; `pipe.Listen(name, userSID string, h pipe.Handler, log func(string, ...any)) (*pipe.Server, error)`, `pipe.DefaultName`; `coreconfig.PathsFor(dataDir string) coreconfig.Paths`; движок: `Start()`, `Wait()`, `SetClients(n int)`, `cmdShutdown`, очередь команд `queue`, `Clock.After`.
- Produces: `const place.Name = "SplitLane"`; `const place.SetupFile = "setup.json"`; `func place.ProgramDir() (string, error)`; `func place.DataDir() (string, error)`; ``type place.Setup struct{ UserSID string `json:"userSid"` }``; `func place.ReadSetup(dataDir string) (place.Setup, error)` (нет файла — ошибка, которая оборачивает `os.ErrNotExist`); `func place.WriteSetup(dataDir string, s place.Setup) error` (атомарно, UTF-8 без BOM, папка `dataDir` должна уже существовать); `func (e *engine.Engine) ExitIfIdle(after time.Duration)` (возвращается, когда решение принято); код выхода `splitlane-helper` 6 — не установлен; `singbox.Driver.Find` опознаёт только процесс образа `Driver.Exe`.

- [ ] **Step 1: Написать тесты путей продукта и `setup.json`**

Файл `helper/internal/place/place_test.go`, целиком:

```go
package place

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake makes the lookup *f answer value and err for the test.
func fake(t *testing.T, f *func() (string, error), value string, err error) {
	t.Helper()
	old := *f
	*f = func() (string, error) { return value, err }
	t.Cleanup(func() { *f = old })
}

// The program folder is ProgramFilesDir of HKLM as it is written there (spec 4.2, 11), never the
// environment of the elevated process: a value that would need the environment to expand, a relative
// one or a failed read gives an error.
func TestProgramDirFromRegistryValue(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &programFiles, `D:\Apps`, nil)
	if dir, err := ProgramDir(); err != nil || dir != `D:\Apps\SplitLane` {
		t.Fatalf("%q %v", dir, err)
	}
	for _, bad := range []string{`%ProgramFiles%`, `%SystemDrive%\Program Files`, `Program Files`, `\Program Files`, ``} {
		fake(t, &programFiles, bad, nil)
		if dir, err := ProgramDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programFiles, "", errors.New("The system cannot find the file specified."))
	if _, err := ProgramDir(); err == nil || !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("a failed read: %v", err)
	}
}

// The data folder is the known folder ProgramData (spec 4.2), with the same checks.
func TestDataDirFromKnownFolder(t *testing.T) {
	t.Setenv("ProgramData", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &programData, `C:\ProgramData`, nil)
	if dir, err := DataDir(); err != nil || dir != `C:\ProgramData\SplitLane` {
		t.Fatalf("%q %v", dir, err)
	}
	for _, bad := range []string{`%ProgramData%`, `ProgramData`, ``} {
		fake(t, &programData, bad, nil)
		if dir, err := DataDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programData, "", errors.New("Element not found."))
	if _, err := DataDir(); err == nil {
		t.Fatal("a failed lookup must be an error")
	}
}

const testSID = "S-1-5-21-1000-2000-3000-1001"

// setup.json is UTF-8 without a BOM and reads back; a missing file is os.ErrNotExist (not installed).
// WriteSetup never makes the data folder: install makes it with its access list (spec 4.2).
func TestSetupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadSetup(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no setup.json: %v", err)
	}
	if err := WriteSetup(dir, Setup{UserSID: testSID}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "setup.json"))
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) || !bytes.Contains(raw, []byte(`"userSid": "`+testSID+`"`)) {
		t.Fatalf("setup.json:\n%s", raw)
	}
	s, err := ReadSetup(dir)
	if err != nil || s.UserSID != testSID {
		t.Fatalf("%+v %v", s, err)
	}
	missing := filepath.Join(dir, "missing")
	if err := WriteSetup(missing, Setup{UserSID: testSID}); err == nil {
		t.Fatal("the data folder must already be there")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("WriteSetup made the data folder")
	}
}

// The SID goes into the access list of the pipe (5.1): only the plain form S-1-... that
// windows.StringToSid reads and gives back unchanged is taken. An alias such as WD (Everyone) or text
// that would add an entry is refused, on write and on read.
func TestSetupRejectsBadSID(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "not-a-sid", "WD", "BU", "s-1-5-21-1000-2000-3000-1001", testSID + ")(A;;GA;;;WD", " " + testSID} {
		if err := WriteSetup(dir, Setup{UserSID: bad}); err == nil {
			t.Errorf("WriteSetup took %q", bad)
		}
		if _, err := os.Stat(filepath.Join(dir, "setup.json")); err == nil {
			t.Fatalf("setup.json written for %q", bad)
		}
	}
	for _, text := range []string{`{"userSid":"WD"}`, `{"userSid":""}`, `{}`, `{broken`} {
		os.WriteFile(filepath.Join(dir, "setup.json"), []byte(text), 0o644)
		if s, err := ReadSetup(dir); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Errorf("ReadSetup took %s: %+v %v", text, s, err)
		}
	}
}

// TestLivePlacesReadOnly reads the real registry value and known folder and changes nothing. It runs
// with SPLITLANE_LIVE_READONLY=1.
func TestLivePlacesReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	prog, err := ProgramDir()
	if err != nil || !strings.EqualFold(filepath.Base(prog), "SplitLane") {
		t.Fatalf("program folder %q %v", prog, err)
	}
	data, err := DataDir()
	if err != nil || !strings.EqualFold(filepath.Base(data), "SplitLane") {
		t.Fatalf("data folder %q %v", data, err)
	}
	t.Logf("LIVE program folder %s, data folder %s", prog, data)
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/place/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/place [build failed]` и `internal\place\place_test.go:25:11: undefined: programFiles`, `internal\place\place_test.go:26:17: undefined: ProgramDir`, `internal\place\place_test.go:44:11: undefined: programData`.

- [ ] **Step 3: Реализовать пакет `place`**

Файл `helper/internal/place/place.go`, целиком:

```go
// Package place knows where the product lives (spec 4.2) and what its setup recorded (4.3): the
// program folder in Program Files, the data folder in ProgramData and setup.json with the SID of the
// user who ran the setup. The helper runs elevated with the user's environment, which the user may
// change (HKCU\Environment): no path here comes from an environment variable.
package place

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/atomicfile"
)

// Name is the product's folder in Program Files and in ProgramData.
const Name = "SplitLane"

// SetupFile is the name of the setup data in the data folder.
const SetupFile = "setup.json"

// The lookups of the system; the tests replace them.
var (
	// programFiles reads ProgramFilesDir of HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion, 64-bit view.
	programFiles = func() (string, error) {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
			registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return "", err
		}
		defer k.Close()
		v, _, err := k.GetStringValue("ProgramFilesDir")
		return v, err
	}
	// programData is the known folder ProgramData.
	programData = func() (string, error) { return windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0) }
)

// ProgramDir is %ProgramFiles%\SplitLane (spec 4.2): the helper, the core and the licence texts,
// written only by administrators. Program Files is the folder the registry names, never the
// environment variable: only binaries that administrators write may run elevated (spec 11).
func ProgramDir() (string, error) {
	dir, err := programFiles()
	if err != nil {
		return "", fmt.Errorf("the Program Files folder: %w", err)
	}
	return inside(dir, "the Program Files folder")
}

// DataDir is %ProgramData%\SplitLane (spec 4.2), from the known folder ProgramData.
func DataDir() (string, error) {
	dir, err := programData()
	if err != nil {
		return "", fmt.Errorf("the ProgramData folder: %w", err)
	}
	return inside(dir, "the ProgramData folder")
}

// inside is the product's folder in dir; a relative dir or one that would need the environment to
// expand is refused.
func inside(dir, what string) (string, error) {
	if strings.Contains(dir, "%") || !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%s is not an absolute path: %q", what, dir)
	}
	return filepath.Join(dir, Name), nil
}

// Setup is setup.json (spec 4.3): the user who ran the setup. The pipe gives this user read and
// write (5.1), the folder export\ gives him read (4.2).
type Setup struct {
	UserSID string `json:"userSid"`
}

// ReadSetup reads setup.json of dataDir. A missing file gives an error that wraps os.ErrNotExist: the
// product is not installed. A file that does not parse or holds no valid SID is an error too.
func ReadSetup(dataDir string) (Setup, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, SetupFile))
	if err != nil {
		return Setup{}, err
	}
	var s Setup
	if err := json.Unmarshal(b, &s); err != nil {
		return Setup{}, fmt.Errorf("%s: %w", SetupFile, err)
	}
	if err := checkSID(s.UserSID); err != nil {
		return Setup{}, fmt.Errorf("%s: %w", SetupFile, err)
	}
	return s, nil
}

// WriteSetup writes setup.json into dataDir atomically, UTF-8 without a BOM. The folder must be there:
// install makes it first, with its access list (spec 4.2); a folder made here would inherit another.
func WriteSetup(dataDir string, s Setup) error {
	if err := checkSID(s.UserSID); err != nil {
		return err
	}
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("the data folder %s is not there", dataDir)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dataDir, SetupFile), append(b, '\n'))
}

// checkSID takes a SID only in the plain form S-1-... that windows.StringToSid reads and gives back
// unchanged: the SID goes into the access list of the pipe (5.1), where an alias such as WD (Everyone)
// or extra text would give access to others.
func checkSID(sid string) error {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return fmt.Errorf("user SID %q: %w", sid, err)
	}
	if s.String() != sid {
		return fmt.Errorf("user SID %q is not in the form S-1-...", sid)
	}
	return nil
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/place/ -count=1 -v
```

Ожидается `--- PASS: TestProgramDirFromRegistryValue`, `--- PASS: TestDataDirFromKnownFolder`, `--- PASS: TestSetupRoundTrip`, `--- PASS: TestSetupRejectsBadSID`, `--- SKIP: TestLivePlacesReadOnly` и `ok  	splitlane/helper/internal/place`.

Затем то же чтение настоящей системы (только чтение реестра и известной папки, ничего не меняет):

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_LIVE_READONLY='1'; go test ./internal/place/ -run 'TestLivePlacesReadOnly' -count=1 -v; Remove-Item Env:SPLITLANE_LIVE_READONLY
```

Ожидается `LIVE program folder C:\Program Files\SplitLane, data folder C:\ProgramData\SplitLane` (на ПК со стандартными папками), `--- PASS: TestLivePlacesReadOnly`.

- [ ] **Step 5: Написать тест опознания своего ядра по образу драйвера**

Изменить `helper/internal/singbox/driver_test.go`:

Найти:

```go
func TestCheckReportsTheCoreMessage(t *testing.T) {
```

Заменить на:

```go
// 5.7: the own core is also the image of the driver's sing-box.exe, in the product the one in
// %ProgramFiles%\SplitLane\. The same process seen by a driver of another sing-box.exe is not its core;
// the driver's path written with forward slashes is the same file.
func TestFindOnlyTheDriversCore(t *testing.T) {
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Kill(); <-proc.Done() })
	waitLog(t, d, "started")
	other := &Driver{Exe: filepath.Join(t.TempDir(), "sing-box.exe"), DataDir: d.DataDir}
	if _, ok := other.Find(proc.Identity()); ok {
		t.Fatal("a process of another image is not the driver's core")
	}
	slashes := &Driver{Exe: filepath.ToSlash(d.Exe), DataDir: d.DataDir}
	if _, ok := slashes.Find(proc.Identity()); !ok {
		t.Fatal("the driver's own exe written with forward slashes")
	}
}

func TestCheckReportsTheCoreMessage(t *testing.T) {
```

- [ ] **Step 6: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -run 'TestFind' -count=1
```

Ожидается:

```text
--- FAIL: TestFindOnlyTheDriversCore (0.05s)
    driver_test.go:157: a process of another image is not the driver's core
FAIL
```

- [ ] **Step 7: Сверять образ с `sing-box.exe` драйвера**

Изменить `helper/internal/singbox/driver.go`:

Найти:

```go
	"os/exec"
	"strings"
```

Заменить на:

```go
	"os/exec"
	"path/filepath"
	"strings"
```

Найти:

```go
// Find opens the core of an earlier run by pid, creation time and image path (5.7).
func (d *Driver) Find(id store.CoreIdentity) (engine.CoreProc, bool) {
	if id.PID <= 0 {
		return nil, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		return nil, false
	}
	created, path, err := processInfo(h)
	if err != nil || created != id.Created || !strings.EqualFold(path, id.Path) {
```

Заменить на:

```go
// Find opens the core of an earlier run by pid, creation time and image path (5.7): the image must be
// the one state.json names and the driver's own sing-box.exe, in the product the one in
// %ProgramFiles%\SplitLane\.
func (d *Driver) Find(id store.CoreIdentity) (engine.CoreProc, bool) {
	if id.PID <= 0 {
		return nil, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		return nil, false
	}
	created, path, err := processInfo(h)
	if err != nil || created != id.Created || !strings.EqualFold(path, id.Path) || !strings.EqualFold(path, filepath.Clean(d.Exe)) {
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/ -run 'TestFind' -count=1 -v
```

Ожидается `--- PASS: TestFindOnlyTheSameProcess`, `--- PASS: TestFindOnlyTheDriversCore` и `ok  	splitlane/helper/internal/singbox`.

- [ ] **Step 9: Написать тесты выхода без окна**

Изменить `helper/internal/engine/startup_test.go`:

Найти:

```go
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
```

Заменить на:

```go
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
```

Найти:

```go
func TestSessionEndWhileOff(t *testing.T) {
```

Заменить на:

```go
// idleState tells whether ExitIfIdle has returned and whether helper.shutdown's path was taken.
func idleState(h *harness, decided chan struct{}) (returned, exiting bool) {
	select {
	case <-decided:
		returned = true
	default:
	}
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return returned, h.e.exiting
}

// Spec 4.3, section 10: started by the task at logon after a restart with the journal left, the helper
// gives the exit node back (5.7); routing stays off and no window comes, so 60 s of the engine's clock
// after the start-up it ends by the path of helper.shutdown. The 60 s begin only when the start-up is
// over: a restore that waits for tailscale.exe holds them.
func TestIdleExitWithoutWindow(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	decided := make(chan struct{})
	go func() {
		h.e.ExitIfIdle(time.Minute)
		close(decided)
	}()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	time.Sleep(20 * time.Millisecond)
	if returned, exiting := idleState(h, decided); returned || exiting {
		t.Fatalf("the wait begins after the start-up: returned %v, exiting %v", returned, exiting)
	}
	began := h.clock.Now()
	release()
	waitClosed(t, decided, "the idle decision")
	h.waitFor("exit", func() bool { return exitedNow(h) })
	if waited := h.clock.Now().Sub(began); waited < time.Minute {
		t.Fatalf("the helper ended %s of the engine's clock after the start-up, want 1m0s", waited)
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" || h.rec.index("sys:set 100.64.0.1 true") < 0 || h.store.hasJournal() {
		t.Fatalf("the restore by the journal comes first: phases %s, events %v", got, h.rec.list())
	}
}

// "Was on" made the start-up enable again (5.7): routing is on without a window, and the helper stays.
func TestNoIdleExitWhenWasOn(t *testing.T) {
	h := newHarness(t)
	leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.e.ExitIfIdle(time.Minute)
	time.Sleep(20 * time.Millisecond)
	if _, exiting := idleState(h, nil); exiting || exitedNow(h) || h.e.State().Phase != protocol.PhaseOn || !h.store.saved().WasOn {
		t.Fatalf("routing stays on: phase %s, exiting %v, was-on %v", h.e.State().Phase, exiting, h.store.saved().WasOn)
	}
}

// A window that connected once keeps the helper, even when it went again before the 60 s were over
// (spec 4.3: only a helper that no window reached ends); here it came during the start-up.
func TestNoIdleExitWhenAWindowCame(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	decided := make(chan struct{})
	go func() {
		h.e.ExitIfIdle(time.Minute)
		close(decided)
	}()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	h.e.SetClients(1)
	h.e.SetClients(0)
	release()
	waitClosed(t, decided, "the idle decision")
	time.Sleep(20 * time.Millisecond)
	if _, exiting := idleState(h, decided); exiting || exitedNow(h) || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("the helper stays after a window: phase %s, exiting %v", h.e.State().Phase, exiting)
	}
}

func TestSessionEndWhileOff(t *testing.T) {
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'IdleExit' -count=1
```

Ожидается `FAIL	splitlane/helper/internal/engine [build failed]` и трижды `h.e.ExitIfIdle undefined (type *Engine has no field or method ExitIfIdle)` (`startup_test.go:231:7`, `:259:6`, `:276:7`).

- [ ] **Step 11: Реализовать `ExitIfIdle`**

Изменить `helper/internal/engine/engine.go`:

Найти:

```go
	lastGone     atomic.Int64 // Clock time (UnixNano) at which the last window went (spec 7)
```

Заменить на:

```go
	lastGone     atomic.Int64 // Clock time (UnixNano) at which the last window went (spec 7)
	windowSeen   atomic.Bool  // a window has connected since the start: no exit without a window (4.3)
	reenabled    bool         // the start-up of 5.7 enabled again because "was on" was set (4.3)
```

Найти:

```go
// stops 5 minutes after the last one went). The pipe calls it under its own lock, which a published
// state takes after e.mu, so it takes no lock of the engine: the start runs on its own goroutine.
func (e *Engine) SetClients(n int) {
	old := e.clients.Swap(int32(n))
```

Заменить на:

```go
// stops 5 minutes after the last one went; spec 4.3: a helper that a window has reached does not end
// on its own). The pipe calls it under its own lock, which a published state takes after e.mu, so it
// takes no lock of the engine: the start runs on its own goroutine.
func (e *Engine) SetClients(n int) {
	if n > 0 {
		e.windowSeen.Store(true)
	}
	old := e.clients.Swap(int32(n))
```

Изменить `helper/internal/engine/startup.go`:

Найти:

```go
	e.setPhase(protocol.PhaseStarting, nil)
	e.mu.Unlock()
	e.runEnable(ctx, true)
}
```

Заменить на:

```go
	e.reenabled = true
	e.setPhase(protocol.PhaseStarting, nil)
	e.mu.Unlock()
	e.runEnable(ctx, true)
}

// ExitIfIdle ends a helper that nothing needs (spec 4.3): the scheduled task starts it at logon, and
// once the start-up of 5.7 is over (Start must have been called) it waits after on the engine's clock;
// if no window has connected since the start and the phase is off, it takes the path of
// helper.shutdown. It does nothing when "was on" made the start-up enable again, when a window has
// connected (even one that went again), or in another phase: an error stays for the window to show.
// It returns once it has decided.
func (e *Engine) ExitIfIdle(after time.Duration) {
	e.Wait()
	<-e.d.Clock.After(after)
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	why := ""
	switch {
	case e.windowSeen.Load():
		why = "a window has connected"
	case e.reenabled:
		why = "the start-up enabled routing again"
	case e.st.Phase != protocol.PhaseOff:
		why = "phase " + e.st.Phase
	}
	e.mu.Unlock()
	if why != "" {
		e.d.Log("the helper stays: %s", why)
		return
	}
	e.d.Log("no window within %s after the start-up and routing is off: exiting (spec 4.3)", after)
	e.cmdShutdown(nil)
}
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/ -run 'IdleExit' -count=1 -v
```

Ожидается `--- PASS: TestIdleExitWithoutWindow`, `--- PASS: TestNoIdleExitWhenWasOn`, `--- PASS: TestNoIdleExitWhenAWindowCame` и `ok  	splitlane/helper/internal/engine`; в журнале теста первого — `no window within 1m0s after the start-up and routing is off: exiting (spec 4.3)`, двух других — `the helper stays: ...`.

- [ ] **Step 13: Написать тесты разбора флагов продукта**

Изменить `helper/cmd/splitlane-helper/main_test.go`:

Найти:

```go
	"strings"
	"testing"
)
```

Заменить на:

```go
	"strings"
	"testing"

	"splitlane/helper/internal/pipe"
)
```

Найти:

```go
		t.Fatal("this build needs --dev")
```

Заменить на:

```go
		t.Fatal("the development paths need --dev")
```

Найти:

```go
	if o, _ := parse(append([]string{"--dev"}, paths...)); o.coreLogLevel != "info" {
		t.Fatalf("default %q", o.coreLogLevel)
	}
}
```

Заменить на:

```go
	if o, _ := parse(append([]string{"--dev"}, paths...)); o.coreLogLevel != "info" {
		t.Fatalf("default %q", o.coreLogLevel)
	}
}

// --dev-pin is development only (plan 1A, Global Constraints): without --dev it is a parse error, even
// as false, so the AI assistant's programs are never pinned to the VPN lane in the product.
func TestDevPinNeedsDev(t *testing.T) {
	for _, args := range [][]string{{"--dev-pin"}, {"--dev-pin=false"}, {"--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "--dev-pin needs --dev") {
			t.Errorf("%v: %v", args, err)
		}
	}
	o, err := parse([]string{"--dev", "--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`})
	if err != nil || !o.devPin {
		t.Fatalf("with --dev: %+v %v", o, err)
	}
}

// Without --dev the helper is the product (spec 4.2, 4.3): no flags at all; its folders and its pipe
// come from the system, never from the command line. The development flags are refused before
// anything is read or written.
func TestProductModeRejectsDevFlags(t *testing.T) {
	for _, args := range [][]string{{"--data-dir", `C:\d`}, {"--core", `C:\c\sing-box.exe`}, {"--pipe", `\\.\pipe\x`}, {"--pipe", pipe.DefaultName}} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "need --dev") {
			t.Errorf("%v: %v", args, err)
		}
		if code := run(args); code != exitUsage {
			t.Errorf("run %v: exit code %d", args, code)
		}
	}
	o, err := parse(nil)
	if err != nil || o.dev || o.devPin || o.dataDir != "" || o.core != "" || o.pipe != pipe.DefaultName || o.coreLogLevel != "info" {
		t.Fatalf("no flags is the product: %+v %v", o, err)
	}
}
```

Найти:

```go
	if code := run(nil); code != exitUsage {
		t.Fatalf("no arguments: %d", code)
	}
```

Заменить на:

```go
	if code := run([]string{"--dev", "--data-dir", `data`, "--core", core}); code != exitUsage {
		t.Fatalf("a relative data folder: %d", code)
	}
```

Запуск без аргументов теперь — режим продукта; его проверяет `TestProductPaths` (шаг 17) с подменёнными папками.

- [ ] **Step 14: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1
```

Ожидается:

```text
--- FAIL: TestDevPinNeedsDev (0.00s)
    main_test.go:53: [--dev-pin]: this build runs only with --dev
    main_test.go:53: [--dev-pin=false]: this build runs only with --dev
    main_test.go:53: [--dev-pin --data-dir C:\d --core C:\c\sing-box.exe]: this build runs only with --dev
--- FAIL: TestProductModeRejectsDevFlags (0.00s)
    main_test.go:68: [--data-dir C:\d]: this build runs only with --dev
    main_test.go:68: [--core C:\c\sing-box.exe]: this build runs only with --dev
    main_test.go:68: [--pipe \\.\pipe\x]: this build runs only with --dev
    main_test.go:68: [--pipe \\.\pipe\SplitLane.Helper]: this build runs only with --dev
    main_test.go:76: no flags is the product: {dev:false dataDir: core: pipe:\\.\pipe\SplitLane.Helper devPin:false coreLogLevel:info} this build runs only with --dev
FAIL
```

- [ ] **Step 15: Разобрать флаги продукта**

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
	fs.BoolVar(&o.dev, "dev", false, "development mode (required in this build)")
	fs.StringVar(&o.dataDir, "data-dir", "", "data folder (absolute)")
	fs.StringVar(&o.core, "core", "", "full path of sing-box.exe 1.14.4")
	fs.StringVar(&o.pipe, "pipe", pipe.DefaultName, "pipe name")
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	fs.StringVar(&o.coreLogLevel, "core-log-level", coreconfig.LogInfo, "development only: the core's log level, info or debug (spec 6.1)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	switch {
	case given["core-log-level"] && !o.dev:
		return o, fmt.Errorf("--core-log-level needs --dev: the product always writes the core log at level info")
	case o.coreLogLevel != coreconfig.LogInfo && o.coreLogLevel != coreconfig.LogDebug:
		return o, fmt.Errorf("--core-log-level must be info or debug")
	case !o.dev:
		return o, fmt.Errorf("this build runs only with --dev")
	case !filepath.IsAbs(o.dataDir) || !filepath.IsAbs(o.core):
```

Заменить на:

```go
	fs.BoolVar(&o.dev, "dev", false, "development mode: the paths below; without it the helper is the product")
	fs.StringVar(&o.dataDir, "data-dir", "", "development only: data folder (absolute)")
	fs.StringVar(&o.core, "core", "", "development only: full path of sing-box.exe 1.14.4")
	fs.StringVar(&o.pipe, "pipe", pipe.DefaultName, "development only: pipe name")
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	fs.StringVar(&o.coreLogLevel, "core-log-level", coreconfig.LogInfo, "development only: the core's log level, info or debug (spec 6.1)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	switch {
	case given["core-log-level"] && !o.dev:
		return o, fmt.Errorf("--core-log-level needs --dev: the product always writes the core log at level info")
	case given["dev-pin"] && !o.dev:
		return o, fmt.Errorf("--dev-pin needs --dev: the product never pins the AI assistant's programs to a lane")
	case o.coreLogLevel != coreconfig.LogInfo && o.coreLogLevel != coreconfig.LogDebug:
		return o, fmt.Errorf("--core-log-level must be info or debug")
	case !o.dev && (given["data-dir"] || given["core"] || given["pipe"]):
		return o, fmt.Errorf("--data-dir, --core and --pipe need --dev: the product takes its folders and its pipe from the system (spec 4.2)")
	case !o.dev:
		return o, nil // the product: run finds its folders (spec 4.2, 4.3)
	case !filepath.IsAbs(o.dataDir) || !filepath.IsAbs(o.core):
```

- [ ] **Step 16: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1 -v
```

Ожидается `--- PASS: TestParse`, `--- PASS: TestCoreLogLevelNeedsDev`, `--- PASS: TestDevPinNeedsDev`, `--- PASS: TestProductModeRejectsDevFlags`, `--- PASS: TestRunRefusesAWrongCore` и `ok  	splitlane/helper/cmd/splitlane-helper`. Запуск без флагов до шага 19 не делать: у `run` ещё нет ветки продукта.

- [ ] **Step 17: Написать тест путей продукта в `run`**

Изменить `helper/cmd/splitlane-helper/main_test.go`:

Найти:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/pipe"
)
```

Заменить на:

```go
import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
)
```

Найти:

```go
// A file that is not the pinned core is refused before anything starts (spec 4.3).
```

Заменить на:

```go
// No test reads the real places of the product: a test of the product mode fakes them (fakePlace), any
// other run without --dev stops at once.
func init() {
	programDir = func() (string, error) { return "", errors.New("the tests do not read the real program folder") }
	productData = func() (string, error) { return "", errors.New("the tests do not read the real data folder") }
}

// fakePlace puts the product into temporary folders: the program folder with the helper's exe in it
// (the running exe is said to be that one) and the data folder that install made. It returns both.
func fakePlace(t *testing.T) (prog, data string) {
	t.Helper()
	root := t.TempDir()
	prog, data = filepath.Join(root, "Program Files", "SplitLane"), filepath.Join(root, "ProgramData", "SplitLane")
	os.MkdirAll(prog, 0o755)
	os.MkdirAll(data, 0o755)
	oldProg, oldData, oldExe := programDir, productData, executable
	programDir = func() (string, error) { return prog, nil }
	productData = func() (string, error) { return data, nil }
	executable = func() (string, error) { return filepath.Join(prog, "splitlane-helper.exe"), nil }
	t.Cleanup(func() { programDir, productData, executable = oldProg, oldData, oldExe })
	return prog, data
}

func helperLog(t *testing.T, data string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(data, "logs", "helper.log"))
	return string(b)
}

// The product (spec 4.2, 4.3, 11): the data folder from place.DataDir, the core next to the helper in
// the program folder, the user of the pipe from setup.json. A helper that runs from another folder, a
// missing setup.json and a missing data folder are refused before the core is looked at; the data
// folder is never made here (install makes it with its access list).
func TestProductPaths(t *testing.T) {
	prog, data := fakePlace(t)
	const sid = "S-1-5-21-1000-2000-3000-1001"
	if err := place.WriteSetup(data, place.Setup{UserSID: sid}); err != nil {
		t.Fatal(err)
	}
	core, user, code := productPlace(filepath.Join(prog, "splitlane-helper.exe"), data, t.Logf)
	if code != exitOK || core != filepath.Join(prog, "sing-box.exe") || user != sid {
		t.Fatalf("core %q, user %q, code %d", core, user, code)
	}
	// The whole run, with a file in place of the core: the log in the data folder names the one refused.
	os.WriteFile(filepath.Join(prog, "sing-box.exe"), []byte("not the core"), 0o644)
	if code := run(nil); code != exitBadCore || !strings.Contains(helperLog(t, data), "core refused: "+filepath.Join(prog, "sing-box.exe")) {
		t.Fatalf("exit code %d, log:\n%s", code, helperLog(t, data))
	}

	executable = func() (string, error) { return `C:\Users\u\Downloads\splitlane-helper.exe`, nil }
	if code := run(nil); code != exitUsage || !strings.Contains(helperLog(t, data), `the product runs only as `+filepath.Join(prog, "splitlane-helper.exe")) {
		t.Fatalf("a helper outside the program folder: exit code %d, log:\n%s", code, helperLog(t, data))
	}
	executable = func() (string, error) { return strings.ToUpper(filepath.Join(prog, "splitlane-helper.exe")), nil }

	os.Remove(filepath.Join(data, "setup.json"))
	if code := run(nil); code != exitNotInstalled || !strings.Contains(helperLog(t, data), "not installed") {
		t.Fatalf("no setup.json: exit code %d, log:\n%s", code, helperLog(t, data))
	}

	os.RemoveAll(data)
	if code := run(nil); code != exitNotInstalled {
		t.Fatalf("no data folder: exit code %d", code)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the helper made the data folder")
	}
}

// A file that is not the pinned core is refused before anything starts (spec 4.3).
```

- [ ] **Step 18: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1
```

Ожидается `FAIL	splitlane/helper/cmd/splitlane-helper [build failed]` и `cmd\splitlane-helper\main_test.go:85:2: undefined: programDir`, `cmd\splitlane-helper\main_test.go:86:2: undefined: productData`, `cmd\splitlane-helper\main_test.go:97:55: undefined: executable`.

- [ ] **Step 19: Реализовать режим продукта в `run`**

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
// This build (Plan 1A) runs only in development mode, from an elevated process, with paths in the
// repository: --dev --data-dir <folder> --core <sing-box.exe>. The install command and the
// scheduled task of spec 4.3 come with Plan 1B.
package main
```

Заменить на:

```go
// Without flags it is the product (spec 4.2, 4.3): the scheduled task "SplitLane Helper" starts
// <ProgramDir>\splitlane-helper.exe; the data folder is %ProgramData%\SplitLane, the core is
// <ProgramDir>\sing-box.exe, the user of the pipe comes from setup.json, and a helper that no window
// reaches ends 60 s after the start-up. For development it runs from an elevated terminal with paths
// in the repository: --dev --data-dir <folder> --core <sing-box.exe>.
package main
```

Найти:

```go
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/probe"
```

Заменить на:

```go
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/probe"
```

Найти:

```go
	exitOK        = 0
	exitUsage     = 2
	exitPipeTaken = 3
	exitBadCore   = 4
	exitInternal  = 5
)

const pipeTakenLabel = "the pipe already exists: another helper runs"
```

Заменить на:

```go
	exitOK           = 0
	exitUsage        = 2
	exitPipeTaken    = 3
	exitBadCore      = 4
	exitInternal     = 5
	exitNotInstalled = 6 // the product without its data folder or setup.json: install has not run
)

const pipeTakenLabel = "the pipe already exists: another helper runs"

// idleExit: a product helper that no window reaches ends so long after the start-up (spec 4.3).
const idleExit = 60 * time.Second

// The places of the product (package place) and the running exe; the tests replace them, so that no
// test reads the registry or the known folders or touches %ProgramData%.
var (
	programDir  = place.ProgramDir
	productData = place.DataDir
	executable  = os.Executable
)
```

Найти:

```go
	o, err := parse(args)
	if err != nil {
		return exitUsage
	}
	paths := coreconfig.PathsFor(o.dataDir)
```

Заменить на:

```go
	o, err := parse(args)
	if err != nil {
		return exitUsage
	}
	if !o.dev {
		// The product: the data folder is the one install made with its access list (spec 4.2); the
		// helper never makes it, so without it there is nowhere to write even the log.
		if o.dataDir, err = productData(); err != nil {
			return exitInternal
		}
		if fi, err := os.Stat(o.dataDir); err != nil || !fi.IsDir() {
			return exitNotInstalled
		}
	}
	paths := coreconfig.PathsFor(o.dataDir)
```

Найти:

```go
	if err := singbox.VerifyCore(o.core); err != nil {
		logf("core refused: %v", err)
		return exitBadCore
	}
	helperPath, err := os.Executable()
	if err != nil {
		logf("own path: %v", err)
		return exitInternal
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		logf("own user: %v", err)
		return exitInternal
	}
```

Заменить на:

```go
	helperPath, err := executable()
	if err != nil {
		logf("own path: %v", err)
		return exitInternal
	}
	userSID := ""
	if !o.dev {
		var code int
		if o.core, userSID, code = productPlace(helperPath, o.dataDir, logf); code != exitOK {
			return code
		}
	}
	if err := singbox.VerifyCore(o.core); err != nil {
		logf("core refused: %v", err)
		return exitBadCore
	}
	if o.dev {
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			logf("own user: %v", err)
			return exitInternal
		}
		userSID = user.User.Sid.String()
	}
```

Найти:

```go
	srv, err = pipe.Listen(o.pipe, user.User.Sid.String(), eng, logf)
```

Заменить на:

```go
	srv, err = pipe.Listen(o.pipe, userSID, eng, logf)
```

Найти:

```go
	go srv.RunActivity(eng, time.Second) // the activity event once a second (spec 8.4)
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	stopRoutes()
	return exitOK
}
```

Заменить на:

```go
	go srv.RunActivity(eng, time.Second) // the activity event once a second (spec 8.4)
	if !o.dev {
		go eng.ExitIfIdle(idleExit) // spec 4.3: started at logon, the helper ends when no window comes
	}
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	stopRoutes()
	return exitOK
}

// productPlace checks that this helper is the product's own copy and finds the rest of the product
// (spec 4.2, 4.3, 11): the helper must run as <ProgramDir>\splitlane-helper.exe, where only
// administrators write, since it runs elevated; the core is <ProgramDir>\sing-box.exe; the user of
// the pipe is the one setup.json names. A refusal is logged and gives the exit code.
func productPlace(helperPath, dataDir string, logf func(string, ...any)) (core, userSID string, code int) {
	dir, err := programDir()
	if err != nil {
		logf("the program folder: %v", err)
		return "", "", exitInternal
	}
	want := filepath.Join(dir, "splitlane-helper.exe")
	if !strings.EqualFold(filepath.Clean(helperPath), want) {
		logf("refused: the product runs only as %s, this is %s (spec 11); a development run takes --dev", want, helperPath)
		return "", "", exitUsage
	}
	s, err := place.ReadSetup(dataDir)
	if err != nil {
		logf("not installed: %v; the window runs install first (spec 4.3)", err)
		return "", "", exitNotInstalled
	}
	return filepath.Join(dir, "sing-box.exe"), s.UserSID, exitOK
}
```

- [ ] **Step 20: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1 -v
```

Ожидается `--- PASS: TestParse`, `--- PASS: TestCoreLogLevelNeedsDev`, `--- PASS: TestDevPinNeedsDev`, `--- PASS: TestProductModeRejectsDevFlags`, `--- PASS: TestProductPaths`, `--- PASS: TestRunRefusesAWrongCore` и `ok  	splitlane/helper/cmd/splitlane-helper`.

- [ ] **Step 21: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; `ok` у всех восемнадцати пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/diag`, `internal/engine`, `internal/pipe`, `internal/place`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`); пятикратный прогон — `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 22: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/place helper/internal/singbox/driver.go helper/internal/singbox/driver_test.go helper/internal/engine/engine.go helper/internal/engine/startup.go helper/internal/engine/startup_test.go helper/cmd/splitlane-helper/main.go helper/cmd/splitlane-helper/main_test.go
git -C C:\code\router commit -m "feat(helper): product mode: the program and data folders from the system, the pipe's user from setup.json, the own core only from the program folder, exit 60 s after the start-up without a window; --dev-pin only with --dev" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 14: Команды install и uninstall, сборка dist\helper

Установка и удаление продукта (4.3, пункты 2 и 4 и абзац «Повторный install»; 4.2; 11; 13 — установщика нет, окно запускает команду с UAC; 14). Новый пакет `internal/install`; каждое действие с системой — за интерфейсом `Sys`, тесты логики идут на подмене. `splitlane-helper.exe install --user-sid <SID>` выполняет строго по порядку: 1) SID в форме `S-1-...` (он попадает в списки доступа; `place` других форм не принимает), процесс повышен, пользователь его маркера равен `--user-sid` — иначе UAC пройден под другой учётной записью, отказ; 2) источник — папка запущенного файла: читаются все шесть файлов раскладки, SHA-256 `sing-box.exe` и `libcronet.dll` сверяются с зашитыми (`singbox.CoreSHA256` и новый `install.CronetSHA256`), дальше копируются ровно проверенные байты; 3) если на `\\.\pipe\SplitLane.Helper` есть служебный процесс — `hello` и `helper.shutdown`, затем ожидание конца его процесса (сервер канала, `GetNamedPipeServerProcessId`) не дольше 120 секунд; не завершился (например, не подтверждён возврат 5.4) — отказ с последней фазой и причиной из событий `state`, ничего не изменено, прежний процесс продолжает попытки (Review Focus 4). Канала нет — команда сама делает работу старта 5.7: журнал возврата (5.4, шаг 1: `tailscale set` по полному пути, подтверждение по `tailscale status --json`, попытка раз в 5 секунд; через 30 секунд — отказ с командой ручного восстановления), затем своё оставшееся ядро — по идентичности из `state.json` и только с образом `<ProgramDir>\sing-box.exe` (`singbox.Driver.Find`, задача 13): штатный сигнал, через 5 секунд принудительное завершение; 4) шесть файлов в `<ProgramDir>` (временный файл в той же папке и переименование поверх; файл, который держит работающая программа, — отказ); 5) `<DataDir>` со списком `D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)` и `<DataDir>\export` с тем же списком и чтением (`0x1200a9`) для пользователя настройки, в том числе у папок, которые уже есть; 6) `setup.json` через `place.WriteSetup` (папку он не создаёт, поэтому после шага 5; без `setup.json` служебный процесс, запущенный задачей, выходит с кодом 6); 7) задача `SplitLane Helper` из XML проверки (е) плана 1A (задача 6 плана 1A, `TaskXML`) с командой `<ProgramDir>\splitlane-helper.exe` без аргументов и именем учётной записи по SID: `schtasks /create /tn "SplitLane Helper" /xml <файл> /f`, файл UTF-16 LE с BOM удаляется после. Строки хода — `INSTALL: ...`, последняя — `INSTALL-OK` либо `INSTALL-REFUSED: <причина>` с кодом выхода 7 (ошибка аргументов — код 2). `uninstall [--purge]` работает только из своей копии приложения (файл внутри `<ProgramDir>` — отказ до любых действий), делает ту же остановку (без подтверждения — отказ), затем удаляет задачу и `<ProgramDir>`, с `--purge` — ещё `<DataDir>` и печатает `UNINSTALL-NOTE: the device <PC>-splitlane stays in your Tailscale network; remove it in the Tailscale admin console` (имя устройства — из `state.json`, иначе `<имя ПК>-splitlane`); последняя строка — `UNINSTALL-OK` или `UNINSTALL-REFUSED: <причина>`. В `cmd/splitlane-helper` подкоманды разбираются до флагов режима работы; папки берутся из `place`, как в режиме продукта, и никогда из командной строки.

Решения задачи (спецификация их не называет). Файл XML задачи пишется не во временную папку пользователя, а в `<DataDir>` (туда пишут только администраторы и SYSTEM, к шагу 7 папка уже есть): процесс пользователя без прав не успеет подменить команду задачи между записью файла и `schtasks`. Папка, которая уже есть, получает список, только если это обычная папка (не ссылка и не точка соединения: список ушёл бы на её цель) и её владелец — администраторы или SYSTEM: `ProgramData` разрешает пользователям создавать папки, и папка, созданная пользователем до первой настройки, вместе со ссылками внутри отвергается; новая папка создаётся сразу со списком (`CreateDirectory` с описателем), без мгновения с унаследованным. Если журнал есть, а системный Tailscale не установлен, возвращать нечего (5.6: шаги с ним пропускаются) — журнал удаляется, команда продолжает. Признак «было включено» в `state.json` команда не трогает: следующий служебный процесс включит деление сам, как после любого аварийного завершения (5.2). `install`, запущенный из самой `<ProgramDir>`, отвергается. В XML задачи нет элемента `<Arguments>`. Значение SHA-256 `libcronet.dll` посчитано по файлу из проверенного архива (`3217c626...`) и записано в `scripts/core.sha256` (его читает `scripts/build-helper.ps1`, как `fetch-core.ps1`) и в `install.CronetSHA256`. Настоящий код проверяют два теста без касания системы: список доступа на временной папке с полным доступом текущего пользователя (чтение назад через `GetNamedSecurityInfo`) и клиент канала на тестовом канале `SplitLane.Test.<число>`, сервер которого — сам тест.

`scripts/build-helper.ps1` (ASCII) собирает `dist\helper\splitlane-helper.exe` с `-ldflags "-H=windowsgui"` и кладёт рядом из `.cache\core\sing-box-1.14.4-windows-amd64\` без изменений `sing-box.exe` и `libcronet.dll` (SHA-256 обоих сверяется с `scripts\core.sha256` до копирования) и `LICENSE` под именем `sing-box-LICENSE`, а также `LICENSE` и `THIRD-PARTY.md` репозитория; `slctl` и `slguard` в сборку не входят. Последняя строка — `BUILD-OK dist\helper` или `BUILD-FAILED <причина>`. `/dist/` добавлен в `.gitignore`; `THIRD-PARTY.md` называет `libcronet.dll` и имя `sing-box-LICENSE`.

**Files:**
- Create: `helper/internal/install/task.go`, `helper/internal/install/install.go`, `helper/internal/install/system.go`, `scripts/build-helper.ps1`
- Modify: `helper/cmd/splitlane-helper/main.go`, `scripts/core.sha256`, `.gitignore`, `THIRD-PARTY.md`
- Test: `helper/internal/install/task_test.go`, `helper/internal/install/install_test.go`, `helper/internal/install/system_test.go`, `helper/cmd/splitlane-helper/main_test.go`

**Interfaces:**
- Consumes: `singbox.CoreSHA256`, `singbox.CoreVersion`; `func (d *singbox.Driver) Find(id store.CoreIdentity) (engine.CoreProc, bool)` (образ процесса равен `Driver.Exe`, задача 13); `store.Dir` с `LoadJournal() (*store.Journal, error)`, `DeleteJournal() error`, `LoadState() (store.State, error)`, `store.ErrIO`, `store.CoreIdentity`, `store.State.DeviceName`; `place.ProgramDir()`, `place.DataDir()`, `func place.WriteSetup(dataDir string, s place.Setup) error`, `place.SetupFile`; `systs.New() *systs.Client` с `Status`, `SetExitNode`, `RestoreCommand`; `engine.SysStatus`, `engine.CoreProc`; `pipe.DefaultName`; `protocol.MethodHello`, `protocol.MethodShutdown`, `protocol.HelloParams`, `protocol.State`, `protocol.NewScanner`; `winio.DialPipeContext`; `windows.GetNamedPipeServerProcessId`, `SetNamedSecurityInfo`, `CreateDirectory`.
- Produces: `func install.Install(o install.Options, s install.Sys) error` и `func install.Uninstall(o install.Options, s install.Sys) error` (печатают строки в `Options.Out`, ошибка — отказ, строка `-REFUSED` уже напечатана); `type install.Options struct{ UserSID string; Purge bool; Exe, ProgramDir, DataDir, Hostname string; Out io.Writer }` (`UserSID` — `--user-sid`; `Purge` — `--purge`; `Exe` — запущенный файл, его папка — источник `install`; `ProgramDir`, `DataDir` — из `place`; `Hostname` — имя ПК для строки `UNINSTALL-NOTE`; `Out` — строки для окна); `type install.Sys interface{ Token() (sid string, elevated bool, err error); AccountName(sid string) (string, error); ReadFile(path string) ([]byte, error); ShutdownHelper(wait time.Duration) (found bool, err error); SystemStatus(ctx context.Context) (engine.SysStatus, error); SetExitNode(ctx context.Context, ip string, allowLAN bool) error; RestoreCommand(ip string, allowLAN bool) string; FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool); After(d time.Duration) <-chan time.Time; WriteFile(path string, data []byte) error; SecureDir(dir, sddl string) error; WriteSetup(dataDir string, s place.Setup) error; RegisterTask(xml, dir string) error; DeleteTask() error; RemoveAll(dir string) error }`; `type install.System struct{ Pipe string; Tailscale *systs.Client }` и `func install.NewSys() *install.System`; `const install.TaskName = "SplitLane Helper"`; `func install.TaskXML(account, sid, command string) string`; `var install.Files` (шесть имён раскладки); `const install.CronetSHA256`; `const install.DataSDDL`; `func install.ExportSDDL(userSID string) string`; подкоманды `splitlane-helper install --user-sid <SID>` и `splitlane-helper uninstall [--purge]` (код выхода 0 — успех, 2 — аргументы, 7 — отказ); `scripts/build-helper.ps1` и раскладка `dist\helper\`: `splitlane-helper.exe`, `sing-box.exe`, `libcronet.dll`, `sing-box-LICENSE`, `LICENSE`, `THIRD-PARTY.md`.

- [ ] **Step 1: Написать тест XML задачи**

Файл `helper/internal/install/task_test.go`, целиком:

```go
package install

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

// The task of spec 4.3 is the XML of check (е) (plan 1A, Task 6), for the product: the task
// SplitLane Helper runs <ProgramDir>\splitlane-helper.exe without arguments, as the setup user with
// the highest privileges, at his logon and on demand, without a time limit or power conditions, a
// second start ignored; he may read and run it, not change it. The file is UTF-16 LE with a BOM.
func TestTaskXMLProduct(t *testing.T) {
	const sid = "S-1-5-21-1000-2000-3000-1001"
	text := TaskXML(`TESTPC\Тестер & Co`, sid, `C:\Program Files\SplitLane\splitlane-helper.exe`)
	var task struct {
		Registration struct {
			URI string `xml:"URI"`
			SD  string `xml:"SecurityDescriptor"`
		} `xml:"RegistrationInfo"`
		Trigger struct {
			Enabled bool   `xml:"Enabled"`
			User    string `xml:"UserId"`
		} `xml:"Triggers>LogonTrigger"`
		Principal struct {
			User     string `xml:"UserId"`
			Logon    string `xml:"LogonType"`
			RunLevel string `xml:"RunLevel"`
		} `xml:"Principals>Principal"`
		Settings struct {
			Multiple    string `xml:"MultipleInstancesPolicy"`
			NoBattery   bool   `xml:"DisallowStartIfOnBatteries"`
			StopBattery bool   `xml:"StopIfGoingOnBatteries"`
			OnDemand    bool   `xml:"AllowStartOnDemand"`
			Enabled     bool   `xml:"Enabled"`
			TimeLimit   string `xml:"ExecutionTimeLimit"`
		} `xml:"Settings"`
		Exec struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	// encoding/xml refuses a declared UTF-16 encoding for a Go string; parse the body only.
	body := text[strings.Index(text, "<Task"):]
	if err := xml.Unmarshal([]byte(body), &task); err != nil {
		t.Fatal(err)
	}
	switch {
	case TaskName != "SplitLane Helper" || task.Registration.URI != `\SplitLane Helper`:
		t.Fatal(task.Registration.URI)
	case task.Registration.SD != "D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;"+sid+")":
		t.Fatal(task.Registration.SD)
	case !task.Trigger.Enabled || task.Trigger.User != `TESTPC\Тестер & Co` || task.Principal.User != task.Trigger.User:
		t.Fatalf("%+v %+v", task.Trigger, task.Principal)
	case task.Principal.Logon != "InteractiveToken" || task.Principal.RunLevel != "HighestAvailable":
		t.Fatalf("%+v", task.Principal)
	case task.Settings.Multiple != "IgnoreNew" || task.Settings.NoBattery || task.Settings.StopBattery ||
		!task.Settings.OnDemand || !task.Settings.Enabled || task.Settings.TimeLimit != "PT0S":
		t.Fatalf("%+v", task.Settings)
	case task.Exec.Command != `C:\Program Files\SplitLane\splitlane-helper.exe` || task.Exec.Arguments != "" || strings.Contains(text, "<Arguments>"):
		t.Fatalf("%+v", task.Exec)
	}

	b := utf16File("a\nя")
	if b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("no BOM")
	}
	units := make([]uint16, (len(b)-2)/2)
	for i := range units {
		units[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
	}
	if string(utf16.Decode(units)) != "a\r\nя" {
		t.Fatalf("%q", string(utf16.Decode(units)))
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/install/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/install [build failed]` и `internal\install\task_test.go:16:10: undefined: TaskXML`, `internal\install\task_test.go:50:7: undefined: TaskName`, `internal\install\task_test.go:65:7: undefined: utf16File`.

- [ ] **Step 3: Реализовать XML задачи**

Файл `helper/internal/install/task.go`, целиком:

```go
package install

import (
	"encoding/xml"
	"strings"
	"unicode/utf16"
)

// TaskName is the scheduled task of the product (spec 4.3); the window starts it with schtasks /run.
const TaskName = "SplitLane Helper"

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// TaskXML is the task of spec 4.3, the XML of check (е) of Appendix A (plan 1A, Task 6) for the
// product: command without arguments, as the setup user (account, sid) in his session with the
// highest privileges; a logon trigger of that user; start on demand; no time limit, no power
// conditions; a second start while it runs is ignored (IgnoreNew). The user may read and run the
// task (FRFX), not change it.
func TaskXML(account, sid, command string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <URI>\` + esc(TaskName) + `</URI>
    <SecurityDescriptor>D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;` + esc(sid) + `)</SecurityDescriptor>
    <Description>SplitLane: the elevated helper, started at logon of the setup user and by the window</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(account) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(account) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(command) + `</Command>
    </Exec>
  </Actions>
</Task>
`
}

// utf16File is what schtasks /create /xml reads: UTF-16 LE with a byte order mark, CRLF lines.
func utf16File(text string) []byte {
	text = strings.ReplaceAll(text, "\n", "\r\n")
	units := utf16.Encode([]rune(text))
	b := []byte{0xFF, 0xFE}
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/install/ -count=1 -v
```

Ожидается `--- PASS: TestTaskXMLProduct` и `ok  	splitlane/helper/internal/install`.

- [ ] **Step 5: Написать тесты порядка install и uninstall на подмене системы**

Подмена `fakeSys` записывает каждый вызов в один упорядоченный список; запись начинается с шага: `sid`, `hash`, `stop`, `copy`, `acl`, `setup`, `task`, `remove`. `After` срабатывает сразу. Значения SHA-256 в тестах — от поддельных файлов (`pinned` подменяется), настоящие сверяет `TestInstallRefusesWrongCoreHash`.

Файл `helper/internal/install/install_test.go`, целиком:

```go
package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
)

const (
	testSID  = "S-1-5-21-1000-2000-3000-1001"
	otherSID = "S-1-5-21-1000-2000-3000-1002"
	exitNode = "100.64.0.1" // the tailnet range, as in the engine's tests
	account  = `TESTPC\tester`
)

// fakeSys plays the system. It records every call in one ordered list; each entry starts with its
// step: sid, hash, stop, copy, acl, setup, task, remove.
type fakeSys struct {
	log      []string
	user     string            // the user of the token
	elevated bool              // the token is elevated
	files    map[string][]byte // what ReadFile finds, by full path
	helper   string            // "none": no helper has the pipe; "ends": it ends after helper.shutdown; "stays": it does not
	sys      engine.SysStatus  // the system Tailscale
	setTakes bool              // SetExitNode turns the exit node on
	core     *fakeCore         // the core of an earlier run that FindCore finds
	inUse    string            // the name of a file that WriteFile cannot replace
	written  map[string][]byte // what WriteFile wrote, by full path
	xml      string            // the XML of the task registered
}

func (f *fakeSys) add(format string, a ...any) { f.log = append(f.log, fmt.Sprintf(format, a...)) }

func (f *fakeSys) Token() (string, bool, error) {
	f.add("sid token")
	return f.user, f.elevated, nil
}

func (f *fakeSys) AccountName(sid string) (string, error) {
	f.add("sid account %s", sid)
	return account, nil
}

func (f *fakeSys) ReadFile(path string) ([]byte, error) {
	f.add("hash read %s", filepath.Base(path))
	b, ok := f.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return b, nil
}

func (f *fakeSys) ShutdownHelper(wait time.Duration) (bool, error) {
	f.add("stop shutdown %s", wait)
	switch f.helper {
	case "ends":
		return true, nil
	case "stays":
		return true, errors.New("the helper (process 4242) did not end within 2m0s; its last state: phase=stopping reason=restore_pending (exit node not confirmed)")
	}
	return false, nil
}

func (f *fakeSys) SystemStatus(context.Context) (engine.SysStatus, error) {
	f.add("stop status")
	return f.sys, nil
}

func (f *fakeSys) SetExitNode(_ context.Context, ip string, allowLAN bool) error {
	f.add("stop set %s %v", ip, allowLAN)
	if !f.setTakes {
		return errors.New("tailscale set: exit status 1: no such node")
	}
	f.sys.ExitNodeOn, f.sys.ExitNodeIP = true, ip
	return nil
}

func (f *fakeSys) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf(`"C:\Program Files\Tailscale\tailscale.exe" set --exit-node=%s --exit-node-allow-lan-access=%v`, ip, allowLAN)
}

func (f *fakeSys) FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool) {
	f.add("stop find %s %d", exe, id.PID)
	if f.core == nil || f.core.id != id || !strings.EqualFold(exe, id.Path) {
		return nil, false
	}
	return f.core, true
}

// After fires at once: the tests do not wait.
func (f *fakeSys) After(d time.Duration) <-chan time.Time {
	f.add("stop wait %s", d)
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func (f *fakeSys) WriteFile(path string, data []byte) error {
	f.add("copy %s", path)
	if filepath.Base(path) == f.inUse {
		return fmt.Errorf("%s cannot be replaced (a program that runs may hold it): Access is denied.", path)
	}
	f.written[path] = data
	return nil
}

func (f *fakeSys) SecureDir(dir, sddl string) error {
	f.add("acl %s %s", dir, sddl)
	return nil
}

func (f *fakeSys) WriteSetup(dataDir string, s place.Setup) error {
	f.add("setup %s %s", dataDir, s.UserSID)
	return nil
}

func (f *fakeSys) RegisterTask(xml, dir string) error {
	f.add("task register %s", dir)
	f.xml = xml
	return nil
}

func (f *fakeSys) DeleteTask() error {
	f.add("task delete")
	return nil
}

func (f *fakeSys) RemoveAll(dir string) error {
	f.add("remove %s", dir)
	return nil
}

// fakeCore is the core of an earlier run: it ends on the graceful signal, or only when it is killed.
type fakeCore struct {
	f        *fakeSys
	id       store.CoreIdentity
	done     chan struct{}
	graceful bool
	ended    bool
}

func (c *fakeCore) Identity() store.CoreIdentity { return c.id }
func (c *fakeCore) Done() <-chan struct{}        { return c.done }

func (c *fakeCore) Interrupt() error {
	c.f.add("stop interrupt %d", c.id.PID)
	if c.graceful {
		c.end()
	}
	return nil
}

func (c *fakeCore) Kill() error {
	c.f.add("stop kill %d", c.id.PID)
	c.end()
	return nil
}

func (c *fakeCore) end() {
	if !c.ended {
		c.ended = true
		close(c.done)
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// setup is an elevated setup user with a complete dist\helper, no helper on the pipe, the system
// Tailscale installed without an exit node, and the options of install: the program and data folders
// in a temporary folder. The pins are those of the fake core and library.
func setup(t *testing.T) (*fakeSys, Options, *strings.Builder) {
	t.Helper()
	root := t.TempDir()
	out := &strings.Builder{}
	o := Options{
		UserSID:    testSID,
		Exe:        filepath.Join(root, "dist", "helper", "splitlane-helper.exe"),
		ProgramDir: filepath.Join(root, "Program Files", "SplitLane"),
		DataDir:    filepath.Join(root, "ProgramData", "SplitLane"),
		Hostname:   "testpc",
		Out:        out,
	}
	f := &fakeSys{user: testSID, elevated: true, files: map[string][]byte{}, helper: "none",
		sys: engine.SysStatus{Installed: true}, written: map[string][]byte{}}
	for _, name := range Files {
		f.files[filepath.Join(filepath.Dir(o.Exe), name)] = []byte("content of " + name)
	}
	old := pinned
	pinned = map[string]string{"sing-box.exe": sum("content of sing-box.exe"), "libcronet.dll": sum("content of libcronet.dll")}
	t.Cleanup(func() { pinned = old })
	return f, o, out
}

// steps is the log as its steps, in order, a step repeated in a row once.
func steps(log []string) string {
	var s []string
	for _, e := range log {
		k, _, _ := strings.Cut(e, " ")
		if len(s) == 0 || s[len(s)-1] != k {
			s = append(s, k)
		}
	}
	return strings.Join(s, ",")
}

// inOrder: every entry of want is in the log, each after the one before.
func inOrder(log, want []string) error {
	i := 0
	for _, e := range log {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	if i < len(want) {
		return fmt.Errorf("%q not found in order; log:\n%s", want[i], strings.Join(log, "\n"))
	}
	return nil
}

func count(log []string, prefix string) int {
	n := 0
	for _, e := range log {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// Install goes in the order of spec 4.3: the user of the elevated process, the SHA-256 of the core and
// its library, the helper that runs stopped, the files copied, the access lists of the data folder and
// of export\, setup.json, the task. The bytes copied are the ones that were hashed.
func TestInstallOrder(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, want := steps(f.log), "sid,hash,stop,copy,acl,setup,task"; got != want {
		t.Fatalf("steps %s, want %s; log:\n%s", got, want, strings.Join(f.log, "\n"))
	}
	src := filepath.Dir(o.Exe)
	for _, name := range Files {
		if got := f.written[filepath.Join(o.ProgramDir, name)]; string(got) != "content of "+name {
			t.Errorf("%s: %q", name, got)
		}
		if !slices.Contains(f.log, "hash read "+name) || f.files[filepath.Join(src, name)] == nil {
			t.Errorf("%s was not read from %s", name, src)
		}
	}
	for _, w := range []string{
		"sid account " + testSID,
		"stop shutdown 2m0s",
		"acl " + o.DataDir + " D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
		"acl " + filepath.Join(o.DataDir, "export") + " D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;" + testSID + ")",
		"setup " + o.DataDir + " " + testSID,
		"task register " + o.DataDir,
	} {
		if !slices.Contains(f.log, w) {
			t.Errorf("no %q in the log:\n%s", w, strings.Join(f.log, "\n"))
		}
	}
	if !strings.Contains(f.xml, "<Command>"+esc(filepath.Join(o.ProgramDir, "splitlane-helper.exe"))+"</Command>") ||
		!strings.Contains(f.xml, "<UserId>"+esc(account)+"</UserId>") || !strings.Contains(f.xml, "(A;;FRFX;;;"+testSID+")") {
		t.Errorf("task XML:\n%s", f.xml)
	}
	if lastLine(out.String()) != "INSTALL-OK" {
		t.Fatalf("output:\n%s", out)
	}
}

// UAC passed under another account (spec 4.3): the user of the elevated process is not the one the
// window names. Install refuses before it reads anything else. A process that is not elevated and a
// SID that is not plain S-1-... (it goes into access lists) are refused as well.
func TestInstallRefusesOtherUser(t *testing.T) {
	f, o, out := setup(t)
	f.user = otherSID
	if err := Install(o, f); err == nil ||
		!strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: UAC was passed under another account: this process runs as "+otherSID+", the setup user is "+testSID) {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token" {
		t.Fatalf("log %s", got)
	}

	f, o, out = setup(t)
	f.elevated = false
	if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: install runs elevated") || len(f.log) != 1 {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}

	for _, bad := range []string{"", "WD", "s-1-5-21-1000-2000-3000-1001", testSID + ")(A;;GA;;;WD", " " + testSID} {
		f, o, out = setup(t)
		o.UserSID, f.user = bad, bad
		if err := Install(o, f); err == nil || len(f.log) != 0 || !strings.Contains(out.String(), "is not a SID in the form S-1-...") {
			t.Errorf("%q: %v\n%s\n%v", bad, err, out, f.log)
		}
	}
}

// Only the unchanged core is installed (spec 4.3, 11): sing-box.exe and libcronet.dll in the folder of
// the running exe must have the pinned SHA-256, and every file of the layout must be there; otherwise
// install refuses before it stops a helper or changes anything.
func TestInstallRefusesWrongCoreHash(t *testing.T) {
	if len(pinned) != 2 || pinned["sing-box.exe"] != singbox.CoreSHA256 || pinned["libcronet.dll"] != CronetSHA256 {
		t.Fatalf("pins %v", pinned)
	}
	for _, c := range []struct{ name, content, want string }{
		{"sing-box.exe", "another core", `\sing-box.exe has SHA-256 ` + sum("another core") + ", want " + sum("content of sing-box.exe")},
		{"libcronet.dll", "another library", `\libcronet.dll has SHA-256 ` + sum("another library")},
		{"THIRD-PARTY.md", "", "INSTALL-REFUSED: the files to install are not complete in "},
	} {
		f, o, out := setup(t)
		path := filepath.Join(filepath.Dir(o.Exe), c.name)
		if c.content == "" {
			delete(f.files, path)
		} else {
			f.files[path] = []byte(c.content)
		}
		if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: ") || !strings.Contains(out.String(), c.want) {
			t.Errorf("%s: %v\n%s", c.name, err, out)
		}
		if got := steps(f.log); got != "sid,hash" {
			t.Errorf("%s: steps %s", c.name, got)
		}
	}
}

// A file of the program folder that a program still holds cannot be replaced: install refuses, and the
// access lists, setup.json and the task are not touched.
func TestInstallRefusesAFileInUse(t *testing.T) {
	f, o, out := setup(t)
	f.helper, f.inUse = "ends", "sing-box.exe"
	if err := Install(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the program folder: ") ||
		!strings.Contains(out.String(), "may hold it") {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := steps(f.log); got != "sid,hash,stop,copy" {
		t.Fatalf("steps %s", got)
	}
}

// Review Focus 4: install over an installation that runs, with routing on and the return of the system
// exit node not confirmed. The disabling of the helper (5.4) keeps it and its core while the exit node
// is not back, so it does not end within 120 s: install refuses with the reason and replaces neither the
// files nor the task; the helper keeps running and keeps trying, the journal is its own.
func TestReinstallWaitsForRestore(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "stays"
	if err := Install(o, f); err == nil ||
		!strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the helper that runs did not end: ") ||
		!strings.Contains(out.String(), "reason=restore_pending") || !strings.Contains(out.String(), "nothing was changed") {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := steps(f.log); got != "sid,hash,stop" {
		t.Fatalf("steps %s; log:\n%s", got, strings.Join(f.log, "\n"))
	}
	if got := strings.Join(f.log[len(f.log)-1:], ""); got != "stop shutdown 2m0s" || len(f.written) != 0 {
		t.Fatalf("last %q, written %d", got, len(f.written))
	}
}

// No helper on the pipe, but a journal and a core left by one that ended abnormally (spec 4.3, the
// paragraph on a repeated install; 5.7): install gives the exit node back first, confirmed by the
// status (5.4 step 1), deletes the journal, then stops the core that state.json names, found only with
// the image <ProgramDir>\sing-box.exe, and copies only after that. An exit node not confirmed within
// 30 s refuses the install with the manual command; the journal and the core stay. Without the
// system client there is nothing to give back (5.6): the journal goes.
func TestInstallWithoutPipeRestoresByJournal(t *testing.T) {
	prepare := func() (*fakeSys, Options, *strings.Builder, store.Dir) {
		f, o, out := setup(t)
		d := store.Dir{Path: o.DataDir}
		id := store.CoreIdentity{PID: 4242, Created: 134000000000000000, Path: filepath.Join(o.ProgramDir, "sing-box.exe")}
		st := store.Defaults()
		st.Core = &id
		if err := d.SaveState(st); err != nil {
			t.Fatal(err)
		}
		if err := d.SaveJournal(store.Journal{ExitNode: exitNode, AllowLAN: true}); err != nil {
			t.Fatal(err)
		}
		f.core = &fakeCore{f: f, id: id, done: make(chan struct{}), graceful: true}
		return f, o, out, d
	}

	f, o, out, d := prepare()
	f.setTakes = true
	if err := Install(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	core := filepath.Join(o.ProgramDir, "sing-box.exe")
	if err := inOrder(f.log, []string{"stop shutdown 2m0s", "stop status", "stop set " + exitNode + " true", "stop status",
		"stop find " + core + " 4242", "stop interrupt 4242", "copy " + filepath.Join(o.ProgramDir, Files[0])}); err != nil {
		t.Fatal(err)
	}
	if j, err := d.LoadJournal(); j != nil || err != nil {
		t.Fatalf("the journal after the confirmed return: %v %v", j, err)
	}
	if count(f.log, "stop kill") != 0 || lastLine(out.String()) != "INSTALL-OK" {
		t.Fatalf("log:\n%s\noutput:\n%s", strings.Join(f.log, "\n"), out)
	}

	f, o, out, d = prepare()
	err := Install(o, f)
	if err == nil || !strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: the system exit node was not given back within 30s (tailscale set: exit status 1: no such node)") ||
		!strings.Contains(out.String(), `give it back by hand: "C:\Program Files\Tailscale\tailscale.exe" set --exit-node=100.64.0.1 --exit-node-allow-lan-access=true`) {
		t.Fatalf("%v\n%s", err, out)
	}
	if count(f.log, "stop set ") != 7 || count(f.log, "stop wait 5s") != 6 || count(f.log, "stop find") != 0 || steps(f.log) != "sid,hash,stop" {
		t.Fatalf("log:\n%s", strings.Join(f.log, "\n"))
	}
	if j, _ := d.LoadJournal(); j == nil {
		t.Fatal("the journal went without a confirmed return")
	}

	f, o, out, d = prepare()
	f.sys = engine.SysStatus{}
	if err := Install(o, f); err != nil || !strings.Contains(out.String(), "the system Tailscale is not installed") || count(f.log, "stop set ") != 0 {
		t.Fatalf("%v\n%s", err, out)
	}
	if j, _ := d.LoadJournal(); j != nil {
		t.Fatal("the journal stays without a system client")
	}
}

// uninstall (spec 4.3 point 4) stops first, as install does: a helper on the pipe gets helper.shutdown
// and must end; one that does not refuses the uninstall, and nothing is removed. Without a helper on the
// pipe the core left by one that ended abnormally is stopped, by force when the signal is not enough.
// Only then the task and the program folder go; the data folder stays. A process that is not elevated
// is refused before anything.
func TestUninstallStopsFirst(t *testing.T) {
	f, o, out := setup(t)
	f.helper = "ends"
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop shutdown 2m0s,task delete,remove "+o.ProgramDir {
		t.Fatalf("log %s", got)
	}
	if lastLine(out.String()) != "UNINSTALL-OK" {
		t.Fatalf("output:\n%s", out)
	}

	f, o, out = setup(t)
	f.helper = "stays"
	if err := Uninstall(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "UNINSTALL-REFUSED: the helper that runs did not end: ") {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop shutdown 2m0s" {
		t.Fatalf("log %s", got)
	}

	f, o, out = setup(t)
	id := store.CoreIdentity{PID: 4343, Created: 134000000000000000, Path: filepath.Join(o.ProgramDir, "sing-box.exe")}
	st := store.Defaults()
	st.Core = &id
	if err := (store.Dir{Path: o.DataDir}).SaveState(st); err != nil {
		t.Fatal(err)
	}
	f.core = &fakeCore{f: f, id: id, done: make(chan struct{})}
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := inOrder(f.log, []string{"stop shutdown 2m0s", "stop find " + id.Path + " 4343", "stop interrupt 4343", "stop wait 5s",
		"stop kill 4343", "task delete", "remove " + o.ProgramDir}); err != nil {
		t.Fatal(err)
	}

	f, o, out = setup(t)
	f.elevated = false
	if err := Uninstall(o, f); err == nil || !strings.HasPrefix(lastLine(out.String()), "UNINSTALL-REFUSED: uninstall runs elevated") || len(f.log) != 1 {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}

// uninstall --purge removes the data folder too, after the program folder, and says that the embedded
// node stays a device of the Tailscale network (spec 4.3 point 4): the name state.json keeps, else
// <PC>-splitlane. Without --purge the data folder stays and there is no note.
func TestUninstallPurge(t *testing.T) {
	note := "UNINSTALL-NOTE: the device %s stays in your Tailscale network; remove it in the Tailscale admin console\nUNINSTALL-OK\n"

	f, o, out := setup(t)
	o.Purge = true
	st := store.Defaults()
	st.DeviceName = "desk-7-splitlane"
	if err := (store.Dir{Path: o.DataDir}).SaveState(st); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(f.log, ","); got != "sid token,stop shutdown 2m0s,task delete,remove "+o.ProgramDir+",remove "+o.DataDir {
		t.Fatalf("log %s", got)
	}
	if !strings.HasSuffix(out.String(), fmt.Sprintf(note, "desk-7-splitlane")) {
		t.Fatalf("output:\n%s", out)
	}

	f, o, out = setup(t)
	o.Purge = true
	if err := Uninstall(o, f); err != nil || !strings.HasSuffix(out.String(), fmt.Sprintf(note, "testpc-splitlane")) {
		t.Fatalf("%v\n%s", err, out)
	}

	f, o, out = setup(t)
	if err := Uninstall(o, f); err != nil || slices.Contains(f.log, "remove "+o.DataDir) || strings.Contains(out.String(), "UNINSTALL-NOTE") {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}

// uninstall runs from the application's own copy (dist\helper of the window), never from the program
// folder it removes: an exe there or below it, in any case of its name, is refused before anything. A
// folder that only begins with the same name is another folder. install likewise copies from the
// application's own copy, never from the program folder onto itself.
func TestUninstallRefusesFromProgramDir(t *testing.T) {
	for _, exe := range []func(dir string) string{
		func(dir string) string { return filepath.Join(dir, "splitlane-helper.exe") },
		func(dir string) string { return strings.ToUpper(filepath.Join(dir, "splitlane-helper.exe")) },
		func(dir string) string { return filepath.Join(dir, "old", "splitlane-helper.exe") },
	} {
		f, o, out := setup(t)
		o.Exe = exe(o.ProgramDir)
		if err := Uninstall(o, f); err == nil || len(f.log) != 0 ||
			lastLine(out.String()) != "UNINSTALL-REFUSED: uninstall runs from the application's own copy, not from "+o.ProgramDir {
			t.Errorf("%s: %v\n%s\n%v", o.Exe, err, out, f.log)
		}
	}

	f, o, out := setup(t)
	o.Exe = filepath.Join(o.ProgramDir+"2", "splitlane-helper.exe")
	if err := Uninstall(o, f); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	f, o, out = setup(t)
	o.Exe = filepath.Join(o.ProgramDir, "splitlane-helper.exe")
	if err := Install(o, f); err == nil || steps(f.log) != "sid" ||
		!strings.HasPrefix(lastLine(out.String()), "INSTALL-REFUSED: install copies the application's own copy into "+o.ProgramDir) {
		t.Fatalf("%v\n%s\n%v", err, out, f.log)
	}
}
```

- [ ] **Step 6: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/install/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/install [build failed]` и, среди прочих, `internal\install\install_test.go:187:37: undefined: Options`, `internal\install\install_test.go:201:23: undefined: Files`, `internal\install\install_test.go:204:9: undefined: pinned`, `internal\install\install_test.go:257:12: undefined: Install`, `too many errors`.

- [ ] **Step 7: Реализовать install и uninstall**

Файл `helper/internal/install/install.go`, целиком:

```go
// Package install is the commands install and uninstall of splitlane-helper.exe (spec 4.3, points 2
// and 4); stage 1 has no installer (spec 13): the window runs them with UAC. install puts the helper,
// the unchanged core with its library and the licence texts into the program folder, makes the data
// folder with its access list (4.2), writes setup.json and registers the scheduled task; uninstall
// takes them away again. A helper that runs is stopped first, and nothing is replaced or removed
// until it has given the system exit node back and ended (Review Focus 4 of plan 1B). Every effect on
// the system sits behind Sys, so the tests run on a fake; System is the real one.
package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
)

// Files are what install copies from the folder of the running exe (dist\helper) into the program
// folder, in this order (spec 4.2, 14): the helper, the unchanged core and its library under their
// own names, the core's licence, the licence texts of SplitLane.
var Files = []string{"splitlane-helper.exe", "sing-box.exe", "libcronet.dll", "sing-box-LICENSE", "LICENSE", "THIRD-PARTY.md"}

// CronetSHA256 is the SHA-256 of libcronet.dll from sing-box-1.14.4-windows-amd64.zip: the core loads
// it from its own folder.
const CronetSHA256 = "3217c6260fbca5f16072e0b79735742f40109a63bb0ff88fd6b96dd6b54a2928"

// pinned are the files whose SHA-256 install checks before anything else (spec 4.3, 11); the tests
// replace it.
var pinned = map[string]string{"sing-box.exe": singbox.CoreSHA256, "libcronet.dll": CronetSHA256}

// DataSDDL is the access list of the data folder (spec 4.2): SYSTEM and the administrators only, not
// inherited from ProgramData (P), passed on to everything inside (OICI).
const DataSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// ExportSDDL is the access list of export\ (spec 4.2): that of the data folder and, for the setup
// user, read and execute (0x1200a9): he may read the diagnostics archives, not change them.
func ExportSDDL(userSID string) string { return DataSDDL + "(A;OICI;0x1200a9;;;" + userSID + ")" }

const (
	helperWait    = 120 * time.Second // helper.shutdown to the end of the helper's process (spec 4.3)
	restoreEvery  = 5 * time.Second   // 5.4 step 1: the return of the system exit node is tried this often
	restoreWithin = 30 * time.Second  // ... and is refused when not confirmed within this
	restoreTries  = int(restoreWithin / restoreEvery)
	stopWait      = 5 * time.Second  // 5.4 step 2: the graceful stop of the core, then the forced end
	callTimeout   = 10 * time.Second // one call of tailscale.exe
)

// Options are what the command line and the system give install and uninstall.
type Options struct {
	UserSID    string    // install: the setup user, plain S-1-... (--user-sid); the pipe and export\ give him access
	Purge      bool      // uninstall: remove the data folder too (--purge)
	Exe        string    // the running splitlane-helper.exe: install copies the files of its folder
	ProgramDir string    // %ProgramFiles%\SplitLane (place.ProgramDir)
	DataDir    string    // %ProgramData%\SplitLane (place.DataDir)
	Hostname   string    // the computer's name: the device <Hostname>-splitlane of the uninstall note when state.json names none
	Out        io.Writer // the lines for the window: "<INSTALL|UNINSTALL>: " steps, INSTALL-OK, INSTALL-REFUSED: <reason>, UNINSTALL-NOTE: ..., UNINSTALL-OK
}

// Sys is every effect of install and uninstall on the system; System is the real one.
type Sys interface {
	// Token is the user of this process's token, S-1-..., and whether the token is elevated.
	Token() (sid string, elevated bool, err error)
	// AccountName is the account of a SID, DOMAIN\name: the user of the task.
	AccountName(sid string) (string, error)
	// ReadFile reads a file whole: install checks and copies the same bytes.
	ReadFile(path string) ([]byte, error)
	// ShutdownHelper: when a helper has the pipe, it sends hello and helper.shutdown and waits at most
	// wait for the helper's process to end. found is false when no helper has the pipe; err tells that
	// one has it and did not end, with its last phase and reason.
	ShutdownHelper(wait time.Duration) (found bool, err error)
	// SystemStatus, SetExitNode and RestoreCommand are the system Tailscale client called by its full
	// path (5.4 step 1, 5.6; package systs).
	SystemStatus(ctx context.Context) (engine.SysStatus, error)
	SetExitNode(ctx context.Context, ip string, allowLAN bool) error
	RestoreCommand(ip string, allowLAN bool) string
	// FindCore opens the helper's core of an earlier run by its identity from state.json, only when
	// its image is exe (singbox.Driver.Find, spec 5.7).
	FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool)
	// After is time.After.
	After(d time.Duration) <-chan time.Time
	// WriteFile replaces path with data, making its folder when it is missing; a file in use is an error.
	WriteFile(path string, data []byte) error
	// SecureDir makes the folder dir with the access list sddl, or gives the list to the folder that is
	// there and to everything in it; a folder that is a link, or that was not made by an administrator,
	// is refused.
	SecureDir(dir, sddl string) error
	// WriteSetup is place.WriteSetup.
	WriteSetup(dataDir string, s place.Setup) error
	// RegisterTask registers TaskName from the XML (schtasks /create /xml /f); the XML file is written
	// in dir, where only administrators write, and removed after.
	RegisterTask(xml, dir string) error
	// DeleteTask removes TaskName; a task that is not there is no error.
	DeleteTask() error
	// RemoveAll removes a folder with everything in it; a folder that is not there is no error.
	RemoveAll(dir string) error
}

// sayFunc prints one line of a step.
type sayFunc func(format string, a ...any)

func output(o Options) io.Writer {
	if o.Out == nil {
		return io.Discard
	}
	return o.Out
}

// progress prints the lines "<label>: ..." of the steps.
func progress(o Options, label string) sayFunc {
	return func(format string, a ...any) { fmt.Fprintf(output(o), label+": "+format+"\n", a...) }
}

// finish prints the last line, <label>-OK or <label>-REFUSED: <reason>, and returns err.
func finish(o Options, label string, err error) error {
	if err != nil {
		fmt.Fprintf(output(o), "%s-REFUSED: %v\n", label, err)
		return err
	}
	fmt.Fprintf(output(o), "%s-OK\n", label)
	return nil
}

// Install is `install --user-sid <SID>` (spec 4.3 point 2 and the paragraph on a repeated install), in
// this order: 1. this process is elevated and runs as the setup user; 2. the files of the folder of the
// running exe are read, the core and its library checked by SHA-256; 3. the helper that runs is
// stopped, or without one what an abnormal end left is undone (stopOld); 4. the files go into the
// program folder; 5. the data folder and export\ get their access lists; 6. setup.json (a product
// helper without it exits with code 6); 7. the scheduled task. It prints INSTALL-OK, or
// INSTALL-REFUSED: <reason> and returns the error.
func Install(o Options, s Sys) error {
	return finish(o, "INSTALL", install(o, s, progress(o, "INSTALL")))
}

func install(o Options, s Sys, say sayFunc) error {
	account, err := checkUser(o, s)
	if err != nil {
		return err
	}
	src := filepath.Dir(o.Exe)
	if inside(src, o.ProgramDir) {
		return fmt.Errorf("install copies the application's own copy into %s; it does not run from there", o.ProgramDir)
	}
	files, err := readSource(s, src)
	if err != nil {
		return err
	}
	if err := stopOld(o, s, say); err != nil {
		return err
	}
	for _, name := range Files {
		if err := s.WriteFile(filepath.Join(o.ProgramDir, name), files[name]); err != nil {
			return fmt.Errorf("the program folder: %w", err)
		}
	}
	say("%d files copied to %s", len(Files), o.ProgramDir)
	if err := s.SecureDir(o.DataDir, DataSDDL); err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	if err := s.SecureDir(filepath.Join(o.DataDir, "export"), ExportSDDL(o.UserSID)); err != nil {
		return fmt.Errorf("the folder export: %w", err)
	}
	say("%s and its folder export have their access lists", o.DataDir)
	if err := s.WriteSetup(o.DataDir, place.Setup{UserSID: o.UserSID}); err != nil {
		return fmt.Errorf("%s: %w", place.SetupFile, err)
	}
	if err := s.RegisterTask(TaskXML(account, o.UserSID, filepath.Join(o.ProgramDir, "splitlane-helper.exe")), o.DataDir); err != nil {
		return fmt.Errorf("the task %s: %w", TaskName, err)
	}
	say("the task %s runs %s as the setup user", TaskName, filepath.Join(o.ProgramDir, "splitlane-helper.exe"))
	return nil
}

// checkUser is step 1 (spec 4.3): the SID is plain S-1-... (it goes into access lists; place takes no
// other form), this process is elevated and runs as that user. A UAC passed under another account
// would give the task an account without administrator rights, which stage 1 does not support. It
// returns the user's account name for the task.
func checkUser(o Options, s Sys) (string, error) {
	if sid, err := windows.StringToSid(o.UserSID); err != nil || sid.String() != o.UserSID {
		return "", fmt.Errorf("--user-sid %q is not a SID in the form S-1-...", o.UserSID)
	}
	user, elevated, err := s.Token()
	switch {
	case err != nil:
		return "", fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return "", errors.New("install runs elevated: the window starts it with UAC (spec 4.3)")
	case user != o.UserSID:
		return "", fmt.Errorf("UAC was passed under another account: this process runs as %s, the setup user is %s; stage 1 needs the setup user to be an administrator (spec 4.3, 13)", user, o.UserSID)
	}
	account, err := s.AccountName(o.UserSID)
	if err != nil {
		return "", fmt.Errorf("the account of %s: %w", o.UserSID, err)
	}
	return account, nil
}

// readSource is step 2: every file of Files from the folder src, the pinned ones checked.
func readSource(s Sys, src string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range Files {
		b, err := s.ReadFile(filepath.Join(src, name))
		if err != nil {
			return nil, fmt.Errorf("the files to install are not complete in %s: %w", src, err)
		}
		if want, ok := pinned[name]; ok {
			sum := sha256.Sum256(b)
			if got := hex.EncodeToString(sum[:]); got != want {
				return nil, fmt.Errorf("%s has SHA-256 %s, want %s: only the unchanged sing-box %s is installed",
					filepath.Join(src, name), got, want, singbox.CoreVersion)
			}
		}
		files[name] = b
	}
	return files, nil
}

// stopOld is step 3 of install and the first step of uninstall (spec 4.3). A helper that has the pipe
// gets helper.shutdown and must end within helperWait: its disabling gives the system exit node back
// first (5.4), so a helper that stays (restore_pending, for example) refuses the command, nothing is
// changed, and that helper keeps trying. Without a helper on the pipe the command does the start-up's
// work of 5.7 itself: the journal first (5.4 step 1), then the helper's own core left from an earlier
// run, found by the identity in state.json and the image <ProgramDir>\sing-box.exe. "Was on" stays in
// state.json: the next helper enables again after its start-up, as after any abnormal end (5.2).
func stopOld(o Options, s Sys, say sayFunc) error {
	if found, err := s.ShutdownHelper(helperWait); found {
		if err != nil {
			return fmt.Errorf("the helper that runs did not end: %v; nothing was changed: it ends once the system exit node is back (spec 5.4)", err)
		}
		say("the helper that ran has ended")
		return nil
	}
	d := store.Dir{Path: o.DataDir}
	j, err := d.LoadJournal()
	switch {
	case errors.Is(err, store.ErrIO):
		return fmt.Errorf("restore.json is there but cannot be read: %v; nothing was changed", err)
	case err != nil:
		say("restore.json: %v; there is no exit node to give back", err)
	case j != nil:
		if err := restore(s, d, *j, say); err != nil {
			return err
		}
	}
	st, err := d.LoadState()
	if err != nil {
		say("state.json: %v; the core of an earlier run is not looked for", err)
	}
	if st.Core == nil {
		return nil
	}
	return stopCore(s, filepath.Join(o.ProgramDir, "sing-box.exe"), *st.Core, say)
}

// restore is 5.4 step 1 for install and uninstall: the exit node of the journal back, confirmed by
// `tailscale status --json`, tried every 5 s for 30 s; then the journal goes. An exit node that the
// user selected in the meantime stays. Not confirmed: the command is refused with the manual command.
// Without the system client there is nothing to give back (5.6) and the journal goes too.
func restore(s Sys, d store.Dir, j store.Journal, say sayFunc) error {
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		st, err := s.SystemStatus(ctx)
		why := err
		if err == nil && !st.Installed {
			cancel()
			say("the system Tailscale is not installed: there is no exit node to give back (spec 5.6)")
			return dropJournal(d)
		}
		if err != nil || !st.ExitNodeOn {
			why = s.SetExitNode(ctx, j.ExitNode, j.AllowLAN)
			st, err = s.SystemStatus(ctx)
			if why == nil {
				why = err
			}
		}
		cancel()
		if err == nil && st.ExitNodeOn {
			if st.ExitNodeIP != j.ExitNode {
				say("the system exit node is on with another node than the journal's: the user's choice stays")
			} else {
				say("the system exit node is back")
			}
			return dropJournal(d)
		}
		if attempt == restoreTries {
			if why == nil {
				why = errors.New("the exit node is not on")
			}
			return fmt.Errorf("the system exit node was not given back within %s (%v); give it back by hand: %s",
				restoreWithin, why, s.RestoreCommand(j.ExitNode, j.AllowLAN))
		}
		<-s.After(restoreEvery)
	}
}

func dropJournal(d store.Dir) error {
	if err := d.DeleteJournal(); err != nil {
		return fmt.Errorf("restore.json: %w", err)
	}
	return nil
}

// stopCore is 5.4 step 2 for the core left from an earlier run: the graceful signal, at most 5 s, then
// the forced end; at once when the signal fails. A core that does not end refuses the command.
func stopCore(s Sys, exe string, id store.CoreIdentity, say sayFunc) error {
	p, ok := s.FindCore(exe, id)
	if !ok {
		return nil
	}
	say("stopping the core left from an earlier run (process %d)", id.PID)
	if err := p.Interrupt(); err != nil {
		say("the graceful stop signal failed: %v; ending the core", err)
		_ = p.Kill()
	} else if !ended(p) {
		select {
		case <-p.Done():
		case <-s.After(stopWait):
			say("the core did not stop within %s; ending it", stopWait)
			_ = p.Kill()
		}
	}
	if ended(p) {
		return nil
	}
	select {
	case <-p.Done():
		return nil
	case <-s.After(stopWait):
		return fmt.Errorf("the core left from an earlier run (process %d) does not end; nothing else was changed", id.PID)
	}
}

func ended(p engine.CoreProc) bool {
	select {
	case <-p.Done():
		return true
	default:
		return false
	}
}

// Uninstall is `uninstall [--purge]` (spec 4.3 point 4), in this order: the same stop as step 3 of
// install (refused while it is not confirmed); the task; the program folder; with Purge the data
// folder, and a note that the embedded node stays a device of the Tailscale network. It runs from the
// application's own copy: an exe inside the program folder is refused before anything, it would be
// removed under itself. It prints UNINSTALL-OK, or UNINSTALL-REFUSED: <reason> and returns the error.
func Uninstall(o Options, s Sys) error {
	return finish(o, "UNINSTALL", uninstall(o, s, progress(o, "UNINSTALL")))
}

func uninstall(o Options, s Sys, say sayFunc) error {
	if inside(o.Exe, o.ProgramDir) {
		return fmt.Errorf("uninstall runs from the application's own copy, not from %s", o.ProgramDir)
	}
	switch _, elevated, err := s.Token(); {
	case err != nil:
		return fmt.Errorf("the user of this process: %w", err)
	case !elevated:
		return errors.New("uninstall runs elevated: the window starts it with UAC (spec 4.3)")
	}
	if err := stopOld(o, s, say); err != nil {
		return err
	}
	device := ""
	if o.Purge {
		device = deviceName(o)
	}
	if err := s.DeleteTask(); err != nil {
		return fmt.Errorf("the task %s: %w", TaskName, err)
	}
	say("the task %s is removed", TaskName)
	if err := s.RemoveAll(o.ProgramDir); err != nil {
		return fmt.Errorf("the program folder: %w", err)
	}
	say("%s is removed", o.ProgramDir)
	if !o.Purge {
		return nil
	}
	if err := s.RemoveAll(o.DataDir); err != nil {
		return fmt.Errorf("the data folder: %w", err)
	}
	say("%s is removed", o.DataDir)
	fmt.Fprintf(output(o), "UNINSTALL-NOTE: the device %s stays in your Tailscale network; remove it in the Tailscale admin console\n", device)
	return nil
}

// deviceName is the embedded node's device in the Tailscale network: the name state.json keeps, else
// <Hostname>-splitlane, as the helper names it.
func deviceName(o Options) string {
	if st, err := (store.Dir{Path: o.DataDir}).LoadState(); err == nil && st.DeviceName != "" {
		return st.DeviceName
	}
	return o.Hostname + "-splitlane"
}

// inside tells that path is dir or below it; names compare without case, as on Windows (filepath.Rel).
func inside(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
```

- [ ] **Step 8: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/install/ -count=1 -v
```

Ожидается `--- PASS: TestInstallOrder`, `--- PASS: TestInstallRefusesOtherUser`, `--- PASS: TestInstallRefusesWrongCoreHash`, `--- PASS: TestInstallRefusesAFileInUse`, `--- PASS: TestReinstallWaitsForRestore`, `--- PASS: TestInstallWithoutPipeRestoresByJournal`, `--- PASS: TestUninstallStopsFirst`, `--- PASS: TestUninstallPurge`, `--- PASS: TestUninstallRefusesFromProgramDir`, `--- PASS: TestTaskXMLProduct` и `ok  	splitlane/helper/internal/install`.

- [ ] **Step 9: Написать тесты настоящей системы: список доступа и клиент канала**

Оба теста ничего не меняют вне временной папки и своего тестового канала: список доступа ставится на папку во временном каталоге теста с полным доступом текущего пользователя, точка соединения создаётся командой `mklink /J` там же; сервер канала `SplitLane.Test.<число>` — сам тест (его процесс не завершается, поэтому проверяется ветка «не завершился»).

Файл `helper/internal/install/system_test.go`, целиком:

```go
package install

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/protocol"
)

// readDACL reads the access list of path back: its SDDL and whether it is protected from inheritance.
func readDACL(t *testing.T, path string) (string, bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	return sd.String(), control&windows.SE_DACL_PROTECTED != 0
}

// The access lists of spec 4.2 with the real API, on a temporary folder and with an SDDL that keeps the
// current user's full access. A folder that is not there is made with the list; one that is there gets
// it, protected, with exactly its entries, and a file in it gets them as inherited entries. A junction
// in place of the folder is refused and its target keeps its list (the list would go to the target); so
// is a folder whose owner is not one of the owners given (one that a user made before the setup).
func TestApplyACLOnTempDir(t *testing.T) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	me := u.User.Sid.String()
	owners := []string{me, "S-1-5-32-544"} // the folders of this test belong to the user, or to the administrators when it runs elevated
	sddl := "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + me + ")"
	root := t.TempDir()

	made := filepath.Join(root, "made")
	if err := secureDir(made, sddl, owners); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, made); got != sddl || !protected {
		t.Fatalf("a new folder: %s protected=%v", got, protected)
	}

	there := filepath.Join(root, "there")
	if err := os.Mkdir(there, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(there, "state.json")
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, there); got == sddl || protected {
		t.Fatalf("before: %s protected=%v", got, protected)
	}
	if err := secureDir(there, sddl, owners); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, there); got != sddl || !protected {
		t.Fatalf("a folder that was there: %s protected=%v", got, protected)
	}
	if got, protected := readDACL(t, file); got != "D:AI(A;ID;FA;;;SY)(A;ID;FA;;;BA)(A;ID;FA;;;"+me+")" || protected {
		t.Fatalf("the file in it: %s protected=%v", got, protected)
	}

	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	before, _ := readDACL(t, target)
	link := filepath.Join(root, "link")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if err := secureDir(link, sddl, owners); err == nil || !strings.Contains(err.Error(), "is not a plain folder") {
		t.Fatalf("a junction: %v", err)
	}
	if after, _ := readDACL(t, target); after != before {
		t.Fatalf("the junction's target changed: %s -> %s", before, after)
	}

	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := secureDir(other, sddl, []string{"S-1-5-18"}); err == nil || !strings.Contains(err.Error(), "was not made by an administrator") {
		t.Fatalf("a folder of another owner: %v", err)
	}
	if got, _ := readDACL(t, other); got == sddl {
		t.Fatal("a folder of another owner got the list")
	}
}

// The pipe client of install on a test pipe whose server is this process (the helper's process, which
// never ends here). No pipe: no helper. A helper that answers helper.shutdown and stays is reported
// after the wait with its process and the phase and reason of its last state event (restore_pending,
// 5.4); an error answer to helper.shutdown is reported at once.
func TestShutdownHelperOnATestPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, time.Now().UnixNano())
	s := &System{Pipe: name}
	if found, err := s.ShutdownHelper(time.Second); found || err != nil {
		t.Fatalf("no pipe: %v %v", found, err)
	}
	l, err := winio.ListenPipe(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	refuse := make(chan bool, 2)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				sc := protocol.NewScanner(conn)
				for sc.Scan() {
					var req protocol.Request
					json.Unmarshal(sc.Bytes(), &req)
					switch req.Method {
					case protocol.MethodHello:
						conn.Write(protocol.EncodeResult(req.ID, protocol.HelloResult{Protocol: protocol.Version}))
					case protocol.MethodShutdown:
						if <-refuse {
							conn.Write(protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeInternal, "test")))
							continue
						}
						conn.Write(protocol.EncodeResult(req.ID, struct{}{}))
						conn.Write(protocol.EncodeEvent("state", protocol.State{Phase: protocol.PhaseStopping, Reason: &protocol.Reason{
							Code: protocol.ReasonRestorePending, Params: map[string]string{protocol.ParamError: "exit node not confirmed"}}}))
					}
				}
			}(conn)
		}
	}()

	refuse <- false
	start := time.Now()
	found, err := s.ShutdownHelper(time.Second)
	want := fmt.Sprintf("the helper (process %d) did not end within 1s; its last state: phase=stopping reason=restore_pending (exit node not confirmed)", os.Getpid())
	if !found || err == nil || err.Error() != want || time.Since(start) < time.Second {
		t.Fatalf("a helper that stays: %v %v", found, err)
	}

	refuse <- true
	found, err = s.ShutdownHelper(time.Minute)
	if !found || err == nil || !strings.Contains(err.Error(), "refused helper.shutdown: internal: test") {
		t.Fatalf("a refused shutdown: %v %v", found, err)
	}
}
```

- [ ] **Step 10: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/install/ -count=1
```

Ожидается `FAIL	splitlane/helper/internal/install [build failed]` и `internal\install\system_test.go:50:12: undefined: secureDir` (так же в строках 68, 87, 98), `internal\install\system_test.go:112:8: undefined: System`.

- [ ] **Step 11: Реализовать настоящую систему**

Файл `helper/internal/install/system.go`, целиком:

```go
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/systs"
)

// System is the real Sys: the token of this process, the files, the helper's pipe, the system
// Tailscale client by its full path, the core's process, the access lists and schtasks.exe of Windows.
type System struct {
	Pipe      string        // the helper's pipe
	Tailscale *systs.Client // the official client by its full path (spec 5.6)
}

var _ Sys = (*System)(nil)

// NewSys is the System of the product: the pipe \\.\pipe\SplitLane.Helper, tailscale.exe from the
// install folder of the official client.
func NewSys() *System { return &System{Pipe: pipe.DefaultName, Tailscale: systs.New()} }

// Token: the user of this process's token and whether UAC elevated it.
func (*System) Token() (string, bool, error) {
	t := windows.GetCurrentProcessToken()
	u, err := t.GetTokenUser()
	if err != nil {
		return "", false, err
	}
	return u.User.Sid.String(), t.IsElevated(), nil
}

// AccountName is DOMAIN\name of the SID, the user of the task.
func (*System) AccountName(sid string) (string, error) {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return "", err
	}
	account, domain, _, err := s.LookupAccount("")
	if err != nil {
		return "", err
	}
	return domain + `\` + account, nil
}

func (*System) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// dialTimeout bounds the connection to a pipe that is there and busy.
const dialTimeout = 5 * time.Second

// ShutdownHelper: see Sys. The helper's process is the server of the pipe (GetNamedPipeServerProcessId);
// while it is waited for, the answer to helper.shutdown and the state events are read: the last state
// says why a helper that does not end stays (restore_pending, 5.4).
func (s *System) ShutdownHelper(wait time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	conn, err := winio.DialPipeContext(ctx, s.Pipe)
	cancel()
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return true, fmt.Errorf("the pipe %s is there but does not answer: %w", s.Pipe, err)
	}
	defer conn.Close()
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return true, errors.New("the pipe gives no handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(f.Fd()), &pid); err != nil {
		return true, fmt.Errorf("the process of the pipe: %w", err)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return true, fmt.Errorf("the helper's process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	w := &watch{}
	go w.read(conn)
	msgs := append(request(1, protocol.MethodHello, protocol.HelloParams{Protocol: protocol.Version}), request(2, protocol.MethodShutdown, nil)...)
	if _, err := conn.Write(msgs); err != nil {
		if ev, _ := windows.WaitForSingleObject(h, 0); ev == windows.WAIT_OBJECT_0 {
			return true, nil
		}
		return true, fmt.Errorf("helper.shutdown could not be sent to the helper (process %d): %w", pid, err)
	}
	deadline := time.Now().Add(wait)
	for {
		ev, err := windows.WaitForSingleObject(h, 500)
		switch {
		case err != nil:
			return true, fmt.Errorf("waiting for the helper (process %d): %w", pid, err)
		case ev == windows.WAIT_OBJECT_0:
			return true, nil
		}
		if fail := w.failed(); fail != nil {
			return true, fmt.Errorf("the helper (process %d) refused helper.shutdown: %v", pid, fail)
		}
		if !time.Now().Before(deadline) {
			return true, fmt.Errorf("the helper (process %d) did not end within %s; its last state: %s", pid, wait, w.state())
		}
	}
}

// request is one request line of spec 5.1.
func request(id int, method string, params any) []byte {
	m := map[string]any{"id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	b, _ := json.Marshal(m)
	return append(b, '\n')
}

// watch keeps what the helper sends after helper.shutdown: an error answer to it (request 2) and the
// phase and reason of the last state event.
type watch struct {
	mu   sync.Mutex
	last string
	fail error
}

func (w *watch) read(r io.Reader) {
	sc := protocol.NewScanner(r)
	for sc.Scan() {
		var m struct {
			ID    int             `json:"id"`
			Error *protocol.Error `json:"error"`
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		w.mu.Lock()
		switch {
		case m.Event == "state":
			var st protocol.State
			if json.Unmarshal(m.Data, &st) == nil {
				w.last = "phase=" + st.Phase
				if st.Reason != nil {
					w.last += " reason=" + st.Reason.Code
					if text := st.Reason.Params[protocol.ParamError]; text != "" {
						w.last += " (" + text + ")"
					}
				}
			}
		case m.ID == 2 && m.Error != nil:
			w.fail = m.Error
		}
		w.mu.Unlock()
	}
}

func (w *watch) failed() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fail
}

func (w *watch) state() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.last == "" {
		return "no state event came"
	}
	return w.last
}

func (s *System) SystemStatus(ctx context.Context) (engine.SysStatus, error) {
	return s.Tailscale.Status(ctx)
}

func (s *System) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	return s.Tailscale.SetExitNode(ctx, ip, allowLAN)
}

func (s *System) RestoreCommand(ip string, allowLAN bool) string {
	return s.Tailscale.RestoreCommand(ip, allowLAN)
}

// FindCore is singbox.Driver.Find with the driver's core exe: the image of the process must be that file.
func (*System) FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool) {
	return (&singbox.Driver{Exe: exe}).Find(id)
}

func (*System) After(d time.Duration) <-chan time.Time { return time.After(d) }

// WriteFile: a temporary file in the same folder, flushed to disk, then renamed over path. Windows does
// not replace a program or a library that runs: that is the error of a file in use.
func (*System) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // nothing left behind on failure; a no-op after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("%s cannot be replaced (a program that runs may hold it): %w", path, err)
	}
	return nil
}

// adminOwners may own a folder that is there already: what install or the helper made (the objects of
// an elevated administrator belong to the administrators) or the system.
var adminOwners = []string{"S-1-5-32-544", "S-1-5-18"}

// SecureDir: see secureDir; a folder that is there must belong to the administrators or to SYSTEM.
func (*System) SecureDir(dir, sddl string) error { return secureDir(dir, sddl, adminOwners) }

// secureDir gives the folder dir the access list of sddl (spec 4.2). A folder that is not there is made
// with it at once, so it never has another list: ProgramData lets users make folders. A folder that is
// there must be a plain folder, not a link or a junction (the list would go to its target), and belong
// to one of owners (a user could have made it before the first install, with links inside); it then
// gets the list, protected, and Windows passes the inherited entries on to everything inside it.
func secureDir(dir, sddl string, owners []string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("the access list %q: %w", sddl, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("the access list %q: %w", sddl, err)
	}
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		p, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		if err := windows.CreateDirectory(p, sa); err != nil {
			return fmt.Errorf("make %s: %w", dir, err)
		}
	case err != nil:
		return err
	case fi.Mode().Type() != fs.ModeDir:
		return fmt.Errorf("%s is not a plain folder (a file, a link or a junction): it is not touched", dir)
	default:
		if err := checkOwner(dir, owners); err != nil {
			return err
		}
	}
	err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", dir, err)
	}
	return nil
}

// checkOwner: the owner of dir is one of owners.
func checkOwner(dir string, owners []string) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("the owner of %s: %w", dir, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("the owner of %s: %w", dir, err)
	}
	if !slices.Contains(owners, owner.String()) {
		return fmt.Errorf("%s is there and belongs to %s: it was not made by an administrator; remove it first", dir, owner)
	}
	return nil
}

func (*System) WriteSetup(dataDir string, s place.Setup) error { return place.WriteSetup(dataDir, s) }

// RegisterTask: the XML in UTF-16 LE with a BOM in dir\task.xml, schtasks /create /tn ... /xml ... /f
// (the task is replaced when it is there), the file removed after.
func (*System) RegisterTask(xml, dir string) error {
	file := filepath.Join(dir, "task.xml")
	if err := os.WriteFile(file, utf16File(xml), 0o600); err != nil {
		return err
	}
	defer os.Remove(file)
	return schtasks("/create", "/tn", TaskName, "/xml", file, "/f")
}

// DeleteTask: schtasks /delete when schtasks /query finds the task.
func (*System) DeleteTask() error {
	if schtasks("/query", "/tn", TaskName) != nil {
		return nil
	}
	return schtasks("/delete", "/tn", TaskName, "/f")
}

func (*System) RemoveAll(dir string) error { return os.RemoveAll(dir) }

// schtasks runs schtasks.exe of the system folder, never one found through PATH, without a window.
func schtasks(args ...string) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(dir, "schtasks.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
```

- [ ] **Step 12: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./internal/install/; go test ./internal/install/ -count=1 -v
```

Ожидается: `go vet` ничего не выводит; `--- PASS` у всех десяти тестов шага 8 и у `TestApplyACLOnTempDir`, `TestShutdownHelperOnATestPipe` (около 1,5 с: ожидание в 1 секунду с проверкой раз в 500 мс); `ok  	splitlane/helper/internal/install`.

- [ ] **Step 13: Написать тест подкоманд**

Изменить `helper/cmd/splitlane-helper/main_test.go`:

Найти:

```go
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
)
```

Заменить на:

```go
	"splitlane/helper/internal/install"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
)
```

Найти:

```go
// No test reads the real places of the product: a test of the product mode fakes them (fakePlace), any
// other run without --dev stops at once.
func init() {
	programDir = func() (string, error) { return "", errors.New("the tests do not read the real program folder") }
	productData = func() (string, error) { return "", errors.New("the tests do not read the real data folder") }
}
```

Заменить на:

```go
// No test reads the real places of the product: a test of the product mode fakes them (fakePlace), any
// other run without --dev stops at once. No test reaches the real system of install and uninstall.
func init() {
	programDir = func() (string, error) { return "", errors.New("the tests do not read the real program folder") }
	productData = func() (string, error) { return "", errors.New("the tests do not read the real data folder") }
	newSys = func() install.Sys { return nil }
}
```

Найти:

```go
	if code := run([]string{"--dev", "--data-dir", `data`, "--core", core}); code != exitUsage {
		t.Fatalf("a relative data folder: %d", code)
	}
}
```

Заменить на:

```go
	if code := run([]string{"--dev", "--data-dir", `data`, "--core", core}); code != exitUsage {
		t.Fatalf("a relative data folder: %d", code)
	}
}

// The subcommands install and uninstall (spec 4.3) come first, before any flag of the run mode:
// install takes --user-sid only and needs it, uninstall takes --purge only. The folders come from the
// system as in the product, never from the command line; the lines go to the output that the window
// reads, and a refusal exits with code 7 (a usage error with 2). The commands are replaced here.
func TestInstallSubcommandArgs(t *testing.T) {
	prog, data := fakePlace(t)
	const sid = "S-1-5-21-1000-2000-3000-1001"
	var got []install.Options
	var answer error
	old := adminCommands
	adminCommands = map[string]func(install.Options, install.Sys) error{}
	for name := range old {
		adminCommands[name] = func(o install.Options, s install.Sys) error {
			got = append(got, o)
			return answer
		}
	}
	t.Cleanup(func() { adminCommands = old })

	var out strings.Builder
	if code := admin("install", []string{"--user-sid", sid}, &out); code != exitOK || len(got) != 1 {
		t.Fatalf("exit code %d, %d calls, output:\n%s", code, len(got), out.String())
	}
	o := got[0]
	if o.UserSID != sid || o.Purge || o.ProgramDir != prog || o.DataDir != data || o.Exe != filepath.Join(prog, "splitlane-helper.exe") || o.Out != &out {
		t.Fatalf("options %+v", o)
	}
	if code := run([]string{"uninstall", "--purge"}); code != exitOK || len(got) != 2 || !got[1].Purge || got[1].UserSID != "" {
		t.Fatalf("run uninstall --purge: exit code %d, %+v", code, got)
	}
	if code := run([]string{"uninstall"}); code != exitOK || len(got) != 3 || got[2].Purge {
		t.Fatalf("run uninstall: exit code %d, %+v", code, got)
	}

	for _, c := range []struct {
		cmd  string
		args []string
		want string
	}{
		{"install", nil, "--user-sid is required"},
		{"install", []string{"--user-sid"}, "flag needs an argument: -user-sid"},
		{"install", []string{"--user-sid", sid, "extra"}, `unexpected "extra"`},
		{"install", []string{"--user-sid", sid, "--purge"}, "flag provided but not defined: -purge"},
		{"install", []string{"--dev", "--user-sid", sid}, "flag provided but not defined: -dev"},
		{"uninstall", []string{"--user-sid", sid}, "flag provided but not defined: -user-sid"},
		{"uninstall", []string{"--purge", "--data-dir", `C:\d`}, "flag provided but not defined: -data-dir"},
	} {
		out.Reset()
		label := strings.ToUpper(c.cmd) + "-REFUSED: "
		if code := admin(c.cmd, c.args, &out); code != exitUsage || !strings.HasPrefix(out.String(), label) || !strings.Contains(out.String(), c.want) {
			t.Errorf("%s %v: exit code %d, output %q", c.cmd, c.args, code, out.String())
		}
	}

	out.Reset()
	answer = errors.New("refused")
	if code := admin("uninstall", nil, &out); code != exitRefused {
		t.Fatalf("a refused uninstall: exit code %d", code)
	}
	programDir = func() (string, error) { return "", errors.New("no program folder") }
	out.Reset()
	calls := len(got)
	if code := admin("install", []string{"--user-sid", sid}, &out); code != exitRefused || len(got) != calls || out.String() != "INSTALL-REFUSED: no program folder\n" {
		t.Fatalf("without the program folder: exit code %d, output %q", code, out.String())
	}
}
```

- [ ] **Step 14: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1
```

Ожидается `FAIL	splitlane/helper/cmd/splitlane-helper [build failed]` и, среди прочих, `cmd\splitlane-helper\main_test.go:88:2: undefined: newSys`, `cmd\splitlane-helper\main_test.go:179:9: undefined: adminCommands`, `cmd\splitlane-helper\main_test.go:190:13: undefined: admin`, `cmd\splitlane-helper\main_test.go:226:52: undefined: exitRefused`, `too many errors`.

- [ ] **Step 15: Подкоманды install и uninstall**

Изменить `helper/cmd/splitlane-helper/main.go`:

Найти:

```go
// in the repository: --dev --data-dir <folder> --core <sing-box.exe>.
package main
```

Заменить на:

```go
// in the repository: --dev --data-dir <folder> --core <sing-box.exe>.
//
// The window runs the setup with UAC from its own copy, dist\helper (package install, spec 4.3):
//
//	splitlane-helper.exe install --user-sid <SID>
//	splitlane-helper.exe uninstall [--purge]
//
// Their lines go to the standard output (the window redirects it); a refusal exits with code 7.
package main
```

Найти:

```go
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
```

Заменить на:

```go
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/install"
	"splitlane/helper/internal/pipe"
```

Найти:

```go
	exitNotInstalled = 6 // the product without its data folder or setup.json: install has not run
)
```

Заменить на:

```go
	exitNotInstalled = 6 // the product without its data folder or setup.json: install has not run
	exitRefused      = 7 // install or uninstall refused; the reason is on its -REFUSED line
)
```

Найти:

```go
	executable  = os.Executable
)
```

Заменить на:

```go
	executable  = os.Executable
)

// adminCommands are the subcommands of the setup (spec 4.3) and newSys the system they change; the
// tests replace both.
var (
	adminCommands = map[string]func(install.Options, install.Sys) error{
		"install":   install.Install,
		"uninstall": install.Uninstall,
	}
	newSys = func() install.Sys { return install.NewSys() }
)
```

Найти:

```go
func run(args []string) int {
	o, err := parse(args)
```

Заменить на:

```go
func run(args []string) int {
	if len(args) > 0 && adminCommands[args[0]] != nil {
		return admin(args[0], args[1:], os.Stdout)
	}
	o, err := parse(args)
```

Найти:

```go
	return filepath.Join(dir, "sing-box.exe"), s.UserSID, exitOK
}
```

Заменить на:

```go
	return filepath.Join(dir, "sing-box.exe"), s.UserSID, exitOK
}

var adminUsage = map[string]string{
	"install":   "splitlane-helper install --user-sid <SID>",
	"uninstall": "splitlane-helper uninstall [--purge]",
}

// parseAdmin reads the flags of a subcommand: --user-sid of install, which it needs, or --purge of
// uninstall; nothing else, no flag of the run mode.
func parseAdmin(cmd string, args []string) (install.Options, error) {
	var o install.Options
	fs := flag.NewFlagSet("splitlane-helper "+cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if cmd == "install" {
		fs.StringVar(&o.UserSID, "user-sid", "", "the SID of the user who runs the setup, S-1-...")
	} else {
		fs.BoolVar(&o.Purge, "purge", false, "remove the data folder too")
	}
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("%v; usage: %s", err, adminUsage[cmd])
	}
	switch {
	case fs.NArg() != 0:
		return o, fmt.Errorf("unexpected %q; usage: %s", fs.Arg(0), adminUsage[cmd])
	case cmd == "install" && o.UserSID == "":
		return o, fmt.Errorf("--user-sid is required; usage: %s", adminUsage[cmd])
	}
	return o, nil
}

// admin runs install or uninstall with its lines to out. The program and data folders come from the
// system as in the product (package place); the source of install is the folder of the running exe.
func admin(cmd string, args []string, out io.Writer) int {
	refused := strings.ToUpper(cmd) + "-REFUSED"
	o, err := parseAdmin(cmd, args)
	if err != nil {
		fmt.Fprintf(out, "%s: %v\n", refused, err)
		return exitUsage
	}
	if o.ProgramDir, err = programDir(); err == nil {
		if o.DataDir, err = productData(); err == nil {
			o.Exe, err = executable()
		}
	}
	if err != nil {
		fmt.Fprintf(out, "%s: %v\n", refused, err)
		return exitRefused
	}
	o.Hostname, _ = os.Hostname()
	o.Out = out
	if adminCommands[cmd](o, newSys()) != nil {
		return exitRefused // the command printed its -REFUSED line
	}
	return exitOK
}
```

- [ ] **Step 16: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/splitlane-helper/ -count=1 -v
```

Ожидается `--- PASS: TestParse`, `--- PASS: TestCoreLogLevelNeedsDev`, `--- PASS: TestDevPinNeedsDev`, `--- PASS: TestProductModeRejectsDevFlags`, `--- PASS: TestProductPaths`, `--- PASS: TestRunRefusesAWrongCore`, `--- PASS: TestInstallSubcommandArgs` и `ok  	splitlane/helper/cmd/splitlane-helper`.

- [ ] **Step 17: Сборка dist\helper**

Изменить `scripts/core.sha256`:

Найти:

```text
8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9  sing-box-1.14.4-windows-amd64/sing-box.exe
```

Заменить на:

```text
8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9  sing-box-1.14.4-windows-amd64/sing-box.exe
3217c6260fbca5f16072e0b79735742f40109a63bb0ff88fd6b96dd6b54a2928  sing-box-1.14.4-windows-amd64/libcronet.dll
```

Изменить `.gitignore`:

Найти:

```text
/helper/bin/
/app/out/
```

Заменить на:

```text
/helper/bin/
/dist/
/app/out/
```

Изменить `THIRD-PARTY.md`:

Найти:

```markdown
- Лицензия: GNU GPL версии 3 или более поздней, с дополнительным условием автора: производная работа не может использовать название sing-box или намекать на связь с ним без согласия автора. Поэтому слово «sing-box» не входит в название SplitLane, задачи планировщика, канала и собственных исполняемых файлов. Текст лицензии ядра — файл `LICENSE` в архиве ядра.
- Исходный код этой версии: https://github.com/SagerNet/sing-box/tree/v1.14.4
- Архив: https://github.com/SagerNet/sing-box/releases/download/v1.14.4/sing-box-1.14.4-windows-amd64.zip
- SHA-256 архива и `sing-box.exe`: `scripts/core.sha256`. Ядро в git не хранится, его скачивает `scripts/fetch-core.ps1`.
```

Заменить на:

```markdown
- Лицензия: GNU GPL версии 3 или более поздней, с дополнительным условием автора: производная работа не может использовать название sing-box или намекать на связь с ним без согласия автора. Поэтому слово «sing-box» не входит в название SplitLane, задачи планировщика, канала и собственных исполняемых файлов. Текст лицензии ядра — файл `LICENSE` в архиве ядра; рядом со служебным процессом он лежит под именем `sing-box-LICENSE`.
- Исходный код этой версии: https://github.com/SagerNet/sing-box/tree/v1.14.4
- Архив: https://github.com/SagerNet/sing-box/releases/download/v1.14.4/sing-box-1.14.4-windows-amd64.zip
- Вместе с ядром из того же архива без изменений поставляется `libcronet.dll` (библиотека проекта cronet-go: https://github.com/sagernet/cronet-go); ядро загружает её из своей папки.
- SHA-256 архива, `sing-box.exe` и `libcronet.dll`: `scripts/core.sha256`. Ядро в git не хранится, его скачивает `scripts/fetch-core.ps1`.
```

Файл `scripts/build-helper.ps1`, целиком:

```powershell
# SplitLane: build dist\helper\, the files that install copies into %ProgramFiles%\SplitLane (spec 4.2, 4.3,
# 14): splitlane-helper.exe without a console window, the unchanged sing-box.exe and libcronet.dll of the
# core, the core's licence as sing-box-LICENSE, LICENSE and THIRD-PARTY.md of SplitLane. The window runs
# dist\helper\splitlane-helper.exe install --user-sid <SID> with UAC. The test tools slctl and slguard are
# not part of it. The SHA-256 of sing-box.exe and libcronet.dll are checked against scripts\core.sha256
# before they are copied.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-helper.ps1
# Needs: Go in %LOCALAPPDATA%\Programs\go\bin, the core from scripts\fetch-core.ps1.
# Result: last line BUILD-OK dist\helper or BUILD-FAILED <reason>.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$name = 'sing-box-1.14.4-windows-amd64'
$root = Split-Path -Parent $PSScriptRoot
$core = Join-Path $root ".cache\core\$name"
$out = Join-Path $root 'dist\helper'

function Get-Pin([string]$file) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $PSScriptRoot 'core.sha256')) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $file) { return $parts[0].ToLower() }
    }
    throw "no pin for $file in core.sha256"
}

function Get-Sha256([string]$path) { (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLower() }

try {
    $go = Join-Path $env:LOCALAPPDATA 'Programs\go\bin\go.exe'
    if (-not (Test-Path -LiteralPath $go)) {
        Write-Host "BUILD-FAILED no Go toolchain at $go"
        exit 1
    }
    foreach ($file in 'sing-box.exe', 'libcronet.dll') {
        $path = Join-Path $core $file
        if (-not (Test-Path -LiteralPath $path)) {
            Write-Host "BUILD-FAILED no $path; run scripts\fetch-core.ps1 first"
            exit 1
        }
        $want = Get-Pin "$name/$file"
        $got = Get-Sha256 $path
        if ($got -ne $want) {
            Write-Host "BUILD-FAILED $file has SHA-256 $got, the pin is $want"
            exit 1
        }
    }
    if (Test-Path -LiteralPath $out) { Remove-Item -LiteralPath $out -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $out | Out-Null
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    Push-Location (Join-Path $root 'helper')
    try {
        $ErrorActionPreference = 'Continue'
        & $go build -ldflags '-H=windowsgui' -o (Join-Path $out 'splitlane-helper.exe') ./cmd/splitlane-helper
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = 'Stop'
        Pop-Location
    }
    if ($code -ne 0) {
        Write-Host "BUILD-FAILED go build exit code $code"
        exit 1
    }
    Copy-Item -LiteralPath (Join-Path $core 'sing-box.exe') -Destination $out
    Copy-Item -LiteralPath (Join-Path $core 'libcronet.dll') -Destination $out
    Copy-Item -LiteralPath (Join-Path $core 'LICENSE') -Destination (Join-Path $out 'sing-box-LICENSE')
    Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination $out
    Copy-Item -LiteralPath (Join-Path $root 'THIRD-PARTY.md') -Destination $out
    Write-Host 'BUILD-OK dist\helper'
} catch {
    Write-Host "BUILD-FAILED $($_.Exception.Message)"
    exit 1
}
```

Скрипт не трогает систему: он собирает и копирует только внутри репозитория.

```powershell
$b = [IO.File]::ReadAllBytes('C:\code\router\scripts\build-helper.ps1'); "non-ascii bytes: $(@($b | Where-Object { $_ -gt 127 }).Count)"; $tok = $null; $err = $null; [void][System.Management.Automation.Language.Parser]::ParseFile('C:\code\router\scripts\build-helper.ps1', [ref]$tok, [ref]$err); "parse errors: $($err.Count)"; powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\scripts\build-helper.ps1 | Select-Object -Last 1; Get-ChildItem C:\code\router\dist\helper | Select-Object Name; git -C C:\code\router status --short --ignored dist
```

Ожидается `non-ascii bytes: 0`, `parse errors: 0`, `BUILD-OK dist\helper`, в `dist\helper` файлы `libcronet.dll`, `LICENSE`, `sing-box-LICENSE`, `sing-box.exe`, `splitlane-helper.exe`, `THIRD-PARTY.md` и строка `!! dist/` (папка игнорируется git). `BUILD-FAILED no ...\sing-box.exe; run scripts\fetch-core.ps1 first` — ядра нет: выполнить `scripts\fetch-core.ps1` и повторить.

- [ ] **Step 18: Полная проверка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/
```

Ожидается: `go vet` и `gofmt -l` ничего не выводят; `ok` у всех девятнадцати пакетов (`cmd/slctl`, `cmd/slguard`, `cmd/splitlane-helper`, `internal/activity`, `internal/atomicfile`, `internal/coreconfig`, `internal/diag`, `internal/engine`, `internal/install`, `internal/pipe`, `internal/place`, `internal/probe`, `internal/protocol`, `internal/rules`, `internal/session`, `internal/singbox`, `internal/store`, `internal/systs`, `internal/winhost`); пятикратный прогон — `ok  	splitlane/helper/internal/engine`.

- [ ] **Step 19: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/install helper/cmd/splitlane-helper/main.go helper/cmd/splitlane-helper/main_test.go scripts/build-helper.ps1 scripts/core.sha256 .gitignore THIRD-PARTY.md
git -C C:\code\router commit -m "feat(helper): install and uninstall: the setup user, the SHA-256 of the core and its library, the helper that runs stopped first, the program folder, the data folder with its access list, setup.json and the scheduled task; scripts/build-helper.ps1 builds dist\helper" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

---

### Task 15: Сторож: команда `kill-core`; сборка и проверки на ПК без переключений

Задача 16 проверяет на ПК пункт ручного списка раздела 12 «завершение ядра через диспетчер задач с измерением времени до возврата системного exit node» и удаление оставшегося устройства Wintun (задача 2): для этого сторожу плана 1A нужна команда `kill-core` — принудительно завершить ядро при работающем служебном процессе (так ведёт себя «Снять задачу» в диспетчере). Затем всё, что можно проверить на настоящей системе, не выключая системный exit node: полный прогон тестов, сборка, `dist\helper\`, тесты, читающие систему, служебный процесс без прав администратора в фазе `off`, архив диагностики и режим настройки ядра без прав администратора на данных разработки (узел уже вошёл в сеть). UAC не нужен; системный VPN не меняется.

**Files:**
- Modify: `helper/cmd/slguard/guard.go`, `helper/cmd/slguard/guard_test.go`
- Modify (игнорируется git): `C:\code\router\spike\plan1\p1lib.ps1`

**Interfaces:**
- Consumes: всё из задач 1–14; `scripts/build-helper.ps1` (задача 14); `p1lib.ps1` плана 1A.
- Produces: действие сторожа `kill-core` (ответ `result=core processes ended: <n>`), `Send-P1Guard -Action kill-core`; программы `helper\bin\{splitlane-helper,slctl,slguard}.exe`; `dist\helper\`.

- [ ] **Step 1: Написать тест команды `kill-core`**

Изменить `helper/cmd/slguard/guard_test.go`:

Найти:

```go
		strings.Contains(string(b), `"state":"tripped"`) {
		t.Fatalf("log:\n%s", b)
	}
}
```

Заменить на:

```go
		strings.Contains(string(b), `"state":"tripped"`) {
		t.Fatalf("log:\n%s", b)
	}
}

func TestKillCoreCommand(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	var result any
	g := newGuard(t, sys, func(g *Guard, tick int) {
		switch tick {
		case 1:
			sys.mu.Lock()
			sys.cores, sys.adapter = 1, true
			sys.mu.Unlock()
			command(g, "kill-core")
		case 3:
			result = lastStatus(t, g)["result"]
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		}
	})
	g.Run()
	if result != "core processes ended: 1" {
		t.Fatalf("result %v, events %s", result, sys.list())
	}
	if !strings.HasPrefix(sys.list(), "kill sing-box.exe|") {
		t.Fatalf("events %s", sys.list())
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slguard/ -run TestKillCoreCommand -count=1
```

Ожидается:

```text
--- FAIL: TestKillCoreCommand
    guard_test.go:548: result <nil>, events kill splitlane-helper.exe|kill sing-box.exe
FAIL
```

Сторож не знает действия и отвечает `cmd_error=unknown action`; ядро завершает только срабатывание по `stop.flag`.

- [ ] **Step 3: Добавить команду**

Изменить `helper/cmd/slguard/guard.go`:

Найти:

```go
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("helper processes ended: %d", n)})
	case "launch-helper":
```

Заменить на:

```go
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("helper processes ended: %d", n)})
	case "kill-core":
		// A crash of the core while the helper runs (spec 5.5, 12): the helper gives the exit node back and restarts.
		n := len(g.Sys.Kill(CoreImage))
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("core processes ended: %d", n)})
	case "launch-helper":
```

- [ ] **Step 4: Запустить тесты сторожа**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slguard/ -count=3
```

Ожидается `ok  	splitlane/helper/cmd/slguard`.

- [ ] **Step 5: Действие `kill-core` в `p1lib.ps1`**

Файл лежит в игнорируемой папке; правка — одной командой, затем разбор парсером PowerShell:

```powershell
$p = 'C:\code\router\spike\plan1\p1lib.ps1'; $t = [IO.File]::ReadAllText($p); $old = "[ValidateSet('kill-helper', 'launch-helper', 'finish')]"; $new = "[ValidateSet('kill-helper', 'kill-core', 'launch-helper', 'finish')]"; "found: $(([regex]::Matches($t, [regex]::Escape($old))).Count)"; [IO.File]::WriteAllText($p, $t.Replace($old, $new), (New-Object Text.UTF8Encoding $false)); $tok = $null; $err = $null; [void][System.Management.Automation.Language.Parser]::ParseFile($p, [ref]$tok, [ref]$err); "parse errors: $($err.Count)"; "kill-core present: $([bool](Select-String -Path $p -SimpleMatch "'kill-core'"))"
```

Ожидается `found: 1`, `parse errors: 0`, `kill-core present: True`.

- [ ] **Step 6: Полный прогон тестов, сборка, `dist\helper\`**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go test -count=5 ./internal/engine/; go build -ldflags "-H=windowsgui" -o bin\splitlane-helper.exe .\cmd\splitlane-helper; go build -o bin\slctl.exe .\cmd\slctl; go build -o bin\slguard.exe .\cmd\slguard; Get-ChildItem bin | Select-Object Name; powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\scripts\build-helper.ps1 | Select-Object -Last 1; Get-ChildItem C:\code\router\dist\helper | Select-Object Name
```

Ожидается `gofmt exit=0 files=[]`, `ok` у всех пакетов (в том числе `internal/activity`, `internal/diag`, `internal/install`, `internal/place`), три программы в `bin`, `BUILD-OK dist\helper` и в `dist\helper` файлы `LICENSE`, `libcronet.dll`, `sing-box-LICENSE`, `sing-box.exe`, `splitlane-helper.exe`, `THIRD-PARTY.md`.

- [ ] **Step 7: Тесты, читающие систему, и тесты сценария**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_CONSOLE_TEST='1'; go test -count=1 -v -run TestInterruptStopsTheCore ./internal/singbox/ | Select-String -Pattern '^(--- |ok|FAIL)'; $env:SPLITLANE_CONSOLE_TEST=''; $env:SPLITLANE_LIVE_READONLY='1'; go test -count=1 -v -run 'TestLive' ./internal/winhost/ | Select-String -CaseSensitive -Pattern 'LIVE |^(--- |ok|FAIL)'; $env:SPLITLANE_LIVE_READONLY=''; Set-Location C:\code\router; node --test scripts/scenario.test.mjs 2>&1 | Select-String -Pattern '^. (pass|fail) '
```

Ожидается: `--- PASS: TestInterruptStopsTheCore`; строка `LIVE default route true, physical forwarding false, NRPT catch-all false, sing-box running false, adapter false`, строки `LIVE wintun ...` (устройство системного Tailscale `present=true`; устройство `SplitLane` — только если оно осталось, тогда `present=false`), `--- PASS` у `TestLiveReadOnly`, `TestLiveWintunListReadOnly`, `TestLiveRouteWatchReadOnly`; `pass 11`, `fail 0`. `physical forwarding true` или `NRPT catch-all true` — задачу 16 не начинать (как в плане 1A, задача 22, шаг 2).

- [ ] **Step 8: Служебный процесс без прав администратора: фаза `off`, архив, активность**

Чистая временная папка данных, тестовый канал; ядро не запускается: вход не выполнен, режима настройки нет.

```powershell
$B = 'C:\code\router\helper\bin'; $D = 'C:\code\router\spike\plan1b\smoke'; $P = '\\.\pipe\SplitLane.Smoke'; Remove-Item -Recurse -Force $D -ErrorAction SilentlyContinue; Start-Process -FilePath "$B\splitlane-helper.exe" -ArgumentList '--dev', '--data-dir', $D, '--core', 'C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe', '--pipe', $P; & "$B\slctl.exe" -pipe $P -timeout 30s state | Select-Object -Last 1; $e = & "$B\slctl.exe" -pipe $P -timeout 30s export | Select-Object -Last 1; $e; $zip = ($e -replace '^SLCTL: export written: ', ''); Add-Type -AssemblyName System.IO.Compression.FileSystem; $z = [IO.Compression.ZipFile]::OpenRead($zip); "entries: $(($z.Entries | ForEach-Object { $_.FullName }) -join ',')"; $z.Dispose(); & "$B\slctl.exe" -pipe $P activity -seconds 3 | Select-Object -Last 1; "activity exit=$LASTEXITCODE"; & "$B\slctl.exe" -pipe $P -timeout 20s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 1; "helper running: $([bool](Get-Process splitlane-helper -ErrorAction SilentlyContinue)); core running: $([bool](Get-Process sing-box -ErrorAction SilentlyContinue))"
```

Ожидается:

```text
SLCTL: phase=off reason=- allDirect=false defaultLane=vpn apps=0 vpn=null direct=null systemExitNode=false restoreOnDisable=false backend=stopped path=null exitNodes=0
SLCTL: export written: C:\code\router\spike\plan1b\smoke\export\splitlane-diagnostics-<дата>-<время>.zip
entries: logs/helper.log,meta.txt,state.json
SLCTL: no activity event in 3 s: it comes only while the core with the adapter runs (on, degraded, blocked)
activity exit=3
SLCTL: the helper closed the pipe
helper running: False; core running: False
```

(`systemExitNode` в первом ответе может быть `false`: опрос 5.6 раз в 3 секунды; порядок записей архива может быть другим, но в нём нет `core-config.json`, `rules/`, `tailscale/`, `restore.json`. `activity exit=3` — событий нет: ядро с адаптером не работает.)

- [ ] **Step 9: Режим настройки без прав администратора на данных разработки**

Ядро режима настройки работает без адаптера и без прав администратора (раздел 7); узел в `C:\code\router\.dev\data` уже вошёл в сеть (`loggedIn: true` с плана 1A). Подключение `slctl` — это подключение окна: служебный процесс сам запускает режим настройки. Выход из сети (`slctl logout`) здесь НЕ выполняется: он требует нового входа в браузере (задача 16).

```powershell
$B = 'C:\code\router\helper\bin'; if (Get-Process -Name splitlane-helper, sing-box -ErrorAction SilentlyContinue) { 'REFUSED: a helper or core runs'; return }; Start-Process -FilePath "$B\splitlane-helper.exe" -ArgumentList '--dev', '--dev-pin', '--data-dir', 'C:\code\router\.dev\data', '--core', 'C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; & "$B\slctl.exe" -timeout 120s login | Select-Object -Last 2; "login exit=$LASTEXITCODE"; & "$B\slctl.exe" -timeout 10s state | Select-Object -Last 1; "setup core running: $([bool](Get-Process sing-box -ErrorAction SilentlyContinue))"; & "$B\slctl.exe" -timeout 30s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 2; "left: [$(@(Get-Process -Name splitlane-helper, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"; Get-Content C:\code\router\.dev\data\logs\helper.log | Select-Object -Last 12 | ForEach-Object { $_ -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>' }
```

Ожидается: `slctl login` печатает `SLCTL: backend=starting` или `backend=running` и выходит с `login exit=0` без ссылки для входа (узел уже вошёл); в `state` — `phase=off ... backend=starting` или `running`, `path=null`, `exitNodes=` больше нуля; `setup core running: True`; после `shutdown` — `left: []`. Системный exit node всё время включён; при включённом системном exit node узел режима настройки остаётся в `starting` (приложение A, (д)). `SLCTL: open this link to log in:` — узел потерял вход: задачу 16 начинать с шага входа (шаг 10 задачи 16).

- [ ] **Step 10: Убедиться, что git видит только изменения сторожа, и закоммитить**

```powershell
git -C C:\code\router status --short
```

Ожидается ровно две строки: ` M helper/cmd/slguard/guard.go` и ` M helper/cmd/slguard/guard_test.go` (`helper/bin/`, `dist/`, `.cache/`, `.dev/`, `spike/` игнорируются).

```powershell
git -C C:\code\router add helper/cmd/slguard/guard.go helper/cmd/slguard/guard_test.go
git -C C:\code\router commit -m "feat(slguard): kill-core command for a live crash of the core" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 16: Живая проверка под сторожем

Один сеанс сторожа плана 1A с одним запросом UAC, служебный процесс разработки (`--dev --dev-pin`) из `helper\bin` с данными `C:\code\router\.dev\data`. Проверяется то, что задачи 1–14 добавили и что модульные тесты проверить не могут: вид соединения и список exit node от службы API ядра (задачи 3, 9), живая активность (задачи 4, 11), архив диагностики (задача 12), падение ядра при работающем служебном процессе с измерением времени до возврата системного exit node и удаление оставшегося устройства Wintun перед перезапуском (задача 2, пункт ручного списка раздела 12), отключение кабеля с перезапуском ядра по `relay` (задача 10), сценарная проверка раздела 12 целиком (задача 5), выход из сети и повторный вход через браузер (задача 8, пункт ручного списка). Основная дорожка всё время — VPN; напрямую ходят только копии `curl-*.exe` и `node-*.exe`. Каждый живой шаг (4–11) — один вызов PowerShell с тайм-аутом 600000 мс, начинается с `. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat`; между вызовами не больше 400 секунд. Задачу целиком выполняет основной сеанс.

**Если что-то пошло не так** — блок восстановления задачи 23 плана 1A (раздел «Если что-то пошло не так», один вызов), затем разбор по `C:\code\router\.dev\data\logs\helper.log`, `core.log` и `C:\code\router\spike\plan1\guard\guard.log`. При `SYSTEM-WRONG`, `NO-TERMINAL-STATE`, `NO-GUARD-HELPER-LEFT`, `state=restore_blocked` или `no status` — показать пользователю ручное восстановление.

**Files:**
- Create (игнорируется git): `C:\code\router\spike\plan1b\net-observe.ps1`
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Consumes: программы задачи 15; `slctl` (`state` с полями `backend`, `path`, `exitNodes`; `login`, `logout`, `activity`, `export`); `Send-P1Guard -Action kill-core` (задача 15); `scripts/scenario.mjs` (задача 5); раскладка `C:\code\router\spike\plan1\lanes-1.json` и копии `C:\code\router\spike\plan1\bin\{vpn,direct,free}\curl-*.exe` плана 1A.

- [ ] **Step 1: Скрипт наблюдения**

Файл `C:\code\router\spike\plan1b\net-observe.ps1`, целиком:

```powershell
# SplitLane Plan 1B, check (i): one snapshot line every -Every seconds for -Seconds seconds (ASCII only).
# phase of the helper, pid of the core (a change means the core was restarted), the embedded node's
# path to the exit node (direct | relay | peer-relay | none) and the lanes of two curl copies.
# Never prints an address. Refreshes the heartbeat of the guard each round. Changes nothing.
param([int]$Seconds = 90, [int]$Every = 10, [string]$Label = 'obs')
. C:\code\router\spike\plan1\p1lib.ps1
$cfgPath = "$P1Data\core-config.json"
$bin = 'C:\code\router\spike\plan1\bin'

function Get-Path {
    try {
        $c = Get-Content -LiteralPath $cfgPath -Raw | ConvertFrom-Json
        $api = @($c.services | Where-Object { $_.type -eq 'api' })[0]
        $exit = @($c.endpoints | Where-Object { $_.type -eq 'tailscale' })[0].exit_node
        if (-not $api -or -not $exit) { return 'no-api' }
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $P1Core
        $psi.Arguments = "api tailscale ping $exit"
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.CreateNoWindow = $true
        $psi.EnvironmentVariables['BOX_API_URL'] = "127.0.0.1:$($api.listen_port)"
        $psi.EnvironmentVariables['BOX_API_SECRET'] = $api.secret
        $p = [System.Diagnostics.Process]::Start($psi)
        $read = $p.StandardOutput.ReadToEndAsync()
        if (-not $p.WaitForExit(25000)) { try { $p.Kill() } catch {} }
        $out = $read.Result
        $pong = @($out -split "`n" | Where-Object { $_ -like 'pong from*' -and $_ -like '* via *' }) | Select-Object -Last 1
        if (-not $pong) { return 'none' }
        $via = ($pong -split ' via ', 2)[1]
        if ($via -like 'DERP(*') { 'relay' } elseif ($via -like 'peer relay*') { 'peer-relay' } else { 'direct' }
    } catch { 'error' }
}

$t0 = Get-Date
do {
    Set-P1Heartbeat
    $s = & "$P1Bin\slctl.exe" -timeout 10s state | Select-Object -Last 1
    $phase = if ($s -match 'phase=(\S+) reason=(\S+)') { "$($Matches[1])/$($Matches[2])" } else { 'no-answer' }
    $core = @(Get-Process sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Id }) -join '+'
    if (-not $core) { $core = 'none' }
    $path = if ($core -ne 'none') { Get-Path } else { '-' }
    $lanes = (& "$P1Bin\slctl.exe" expect-lanes "$bin\vpn\curl-vpn.exe=vpn" "$bin\direct\curl-direct.exe=direct" | Where-Object { $_ -match '^LANE' } | ForEach-Object { ($_ -split ' ')[0] + ':' + (($_ -split ' ')[1] -replace '\.exe$', '') }) -join ' '
    '{0} t={1,3}s phase={2} core={3} path={4} {5}' -f $Label, [int]((Get-Date) - $t0).TotalSeconds, $phase, $core, $path, $lanes
    if (((Get-Date) - $t0).TotalSeconds -ge $Seconds) { break }
    Start-Sleep -Seconds $Every
} while ($true)
```

```powershell
$t = $null; $e = $null; [void][System.Management.Automation.Language.Parser]::ParseFile('C:\code\router\spike\plan1b\net-observe.ps1', [ref]$t, [ref]$e); "parse errors: $($e.Count)"; "non-ascii: $(@(Select-String -Path C:\code\router\spike\plan1b\net-observe.ps1 -Pattern '[^\x00-\x7F]').Count)"
```

Ожидается `parse errors: 0` и `non-ascii: 0`.

- [ ] **Step 2: Готовность (только чтение)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Test-P1Ready; Get-P1Guard; Test-P1SystemRestored; (Get-Content "$P1Data\state.json" -Raw | ConvertFrom-Json) | Select-Object loggedIn, wasOn, defaultLane
```

Ожидается `P1-READY`, `P1-GUARD: state=finished ...`, `SYSTEM-RESTORED`, `loggedIn True`, `wasOn False`, `defaultLane vpn`. `P1-NOT-READY` с `development data left by a tripped run` — сброс, как в задаче 23 плана 1A, шаг 1.

- [ ] **Step 3: Согласие пользователя и запуск сторожа (нужен пользователь: UAC)**

Передайте пользователю текст ручного восстановления (Global Constraints плана 1A) и затем:

```text
Сейчас один сеанс служебного процесса SplitLane под сторожем, около 30–40 минут. Связь у всех программ компьютера будет пропадать на 5–25 секунд при каждом включении, выключении и перезапуске ядра (их будет около десяти); открытые соединения оборвутся. Основная дорожка всё время — VPN. Понадобятся ваши руки трижды: принять запрос UAC от slguard.exe; по моему сигналу вынуть сетевой кабель на 20–30 секунд и вставить обратно; войти в Tailscale по ссылке, которую я покажу (выход и повторный вход встроенного узла). В значке Tailscale в это время ничего не переключайте.
```

После ответа:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Start-P1Guard
```

Ожидается `P1-READY`, `P1-RESULT: state=armed ...` и перед ним строка `SLCTL: phase=off ...` (с `backend=starting` или `running`: подключение `slctl` запустило режим настройки). `UAC-CANCELLED` — без согласия пользователя не повторять.

- [ ] **Step 4: Включение, вид соединения и список exit node**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" lanes C:\code\router\spike\plan1\lanes-1.json | Select-Object -Last 1; $t0 = Get-Date; & "$P1Bin\slctl.exe" -timeout 240s enable | Select-Object -Last 1; "SLCTL-EXIT $LASTEXITCODE seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; Set-P1Heartbeat; $s = ''; for ($i = 0; $i -lt 16 -and $s -notmatch ' path=direct '; $i++) { Start-Sleep -Seconds 5; $s = & "$P1Bin\slctl.exe" state | Select-Object -Last 1 }; $s; Set-P1Heartbeat; $min = (1..6 | ForEach-Object { Start-Sleep -Seconds 1; @(Get-Process sing-box -ErrorAction SilentlyContinue).Count } | Measure-Object -Minimum).Minimum; "setup core gone: $($min -eq 1)"
```

Ожидается `RESULT {}`, `SLCTL: phase=on ... backend=running path=... exitNodes=<n>` и `SLCTL-EXIT 0`, затем строка состояния с `path=direct` и `exitNodes` больше нуля (не позже чем через 80 секунд: путь читается сразу после `on` и раз в 60 секунд), `setup core gone: True` (работает одно ядро — рабочее; ядро режима настройки остановлено первым действием включения; каждый вызов службы API — короткий отдельный процесс `sing-box.exe api ...`, поэтому берётся наименьшее из шести чисел за 6 секунд). `phase=error` — шаг «Если что-то пошло не так» и разбор; `reason=tunnel_not_proven` после повтора (задача 7) — сохранить `core.log`, собранный с `--core-log-level debug` нельзя: этот запуск шёл на `info`.

- [ ] **Step 5: Живая активность**

Две копии `curl` открывают соединения раз в 300 мс; `slctl activity` подписан с подробностями о копии дорожки VPN (её правило — `6f1c2a10-0000-4000-8000-000000000001`).

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $bin = 'C:\code\router\spike\plan1\bin'; $jobs = foreach ($n in 'vpn', 'direct') { Start-Job -ArgumentList "$bin\$n\curl-$n.exe" -ScriptBlock { param($exe) $end = (Get-Date).AddSeconds(10); while ((Get-Date) -lt $end) { & $exe -4 -s -o NUL --noproxy * --max-time 5 https://api.ipify.org; Start-Sleep -Milliseconds 300 } } }; & "$P1Bin\slctl.exe" activity -seconds 8 -detail 6f1c2a10-0000-4000-8000-000000000001 | Select-Object -Last 4; "activity exit=$LASTEXITCODE"; $jobs | Wait-Job -Timeout 20 | Remove-Job -Force
```

Ожидается не меньше одной строки `ACTIVITY vpn=<down>/<up> direct=<down>/<up> apps=<n> top=...`, где у обеих дорожек есть ненулевые числа, строка `DETAIL key=6f1c2a10-0000-4000-8000-000000000001 exes=1 connections=<n>` и `activity exit=0`.

- [ ] **Step 6: Архив диагностики**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $e = & "$P1Bin\slctl.exe" export | Select-Object -Last 1; $e; $zip = ($e -replace '^SLCTL: export written: ', ''); Add-Type -AssemblyName System.IO.Compression.FileSystem; $z = [IO.Compression.ZipFile]::OpenRead($zip); "entries: $(($z.Entries | ForEach-Object { $_.FullName }) -join ',')"; $text = ($z.Entries | ForEach-Object { $r = New-Object IO.StreamReader($_.Open()); $r.ReadToEnd(); $r.Dispose() }) -join "`n"; $z.Dispose(); "login link inside: $($text -match 'login\.tailscale\.com/a/')"; "secret-looking keys inside: $($text -match '\"secret\"')"
```

Ожидается `SLCTL: export written: C:\code\router\.dev\data\export\splitlane-diagnostics-...zip`, в `entries` — `logs/helper.log`, `logs/core.log` (и `.prev`, если есть), `state.json`, `meta.txt` и ничего больше; `login link inside: False`, `secret-looking keys inside: False`.

- [ ] **Step 7: Падение ядра: возврат системного exit node и удаление оставшегося устройства**

Сторож принудительно завершает ядро (как «Снять задачу» в диспетчере): поток надзора служебного процесса сразу возвращает системный exit node (5.5), затем перезапуск по 5.3; принудительно завершённое ядро оставило неподключённое устройство Wintun, и перед запуском нового ядра служебный процесс его удаляет (задача 2).

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $n0 = @(Get-Content "$P1Data\logs\helper.log").Count; $t0 = Get-Date; Send-P1Guard -Action kill-core; $back = $null; for ($i = 0; $i -lt 100; $i++) { if ((Get-P1SystemExitNode).On) { $back = [int]((Get-Date) - $t0).TotalMilliseconds; break }; Start-Sleep -Milliseconds 200 }; "system exit node back after ms: $back (including the guard's command pickup)"; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 240s wait on,error | Select-Object -Last 1; "seconds to on: $([int]((Get-Date) - $t0).TotalSeconds)"; Set-P1Heartbeat; $new = @(Get-Content "$P1Data\logs\helper.log" | Select-Object -Skip $n0); "removed stale device: $(@($new | Select-String -SimpleMatch 'removed the stale Wintun device of the adapter').Count)"; "early exits: $(@($new | Select-String -SimpleMatch 'core exited before its start line').Count)"; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'
```

Ожидается `P1-RESULT: state=armed ... result=core processes ended: 1`, `system exit node back after ms:` — число до 3000 (в нём и до секунды на то, чтобы сторож прочёл команду), `SLCTL: phase=on ...`, `removed stale device: 1`, `early exits: 0`, два `LANE-OK`. `removed stale device: 0` и `early exits: 1` — удаление устройства не сработало (запись `stale Wintun device of the adapter not removed: ...` в журнале): записать в приложение A и разобрать после сеанса; деление при этом должно было включиться повтором.

- [ ] **Step 8: Кабель: перезапуск ядра по `relay` (нужен пользователь)**

Скажите пользователю: «Выньте сетевой кабель сейчас, через 20–30 секунд вставьте обратно и ждите: наблюдение идёт 5 минут, результат я покажу сам». Сразу после этого — один вызов:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; $n0 = @(Get-Content "$P1Data\logs\helper.log").Count; powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\spike\plan1b\net-observe.ps1 -Seconds 300 -Every 10 -Label cable; Set-P1Heartbeat; Get-Content "$P1Data\logs\helper.log" | Select-Object -Skip $n0 | Where-Object { $_ -notmatch 'state: phase' } | ForEach-Object { $_ -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>' }
```

Ожидается: пока кабеля нет — фаза `on` (отсутствие сети не сбой), номер процесса ядра не меняется; после возврата сети — `path=relay`; не позже чем через 2 минуты после возврата сети в журнале `the path to the exit node stayed relay after the network change; restarting the core`, номер процесса ядра один раз меняется, затем `path=direct` и два `LANE-OK`. Если узел сам вернулся на `direct` до 60 секунд — перезапуска нет, это тоже верно (записать). Без кабеля сеанс исполнителя тоже теряет связь: вызов продолжает работать локально, результат придёт после возврата сети.

- [ ] **Step 9: Сценарная проверка раздела 12**

Сценарий сам включает и выключает деление и временно заменяет раскладку; перед ним деление выключено.

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; Set-P1Heartbeat; node C:\code\router\scripts\scenario.mjs --heartbeat C:\code\router\spike\plan1\guard\deadline.txt; "scenario exit=$LASTEXITCODE"; Set-P1Heartbeat; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

Ожидается `SLCTL: phase=off ...`, затем `SCENARIO-PREP ...`, строки `SCENARIO 1 PASS` … `SCENARIO 8 PASS`, `SCENARIO-RESTORE ...`, `SCENARIO-DONE pass=8 fail=0`, `scenario exit=0` и итоговая фаза `off`. Сценарий длится несколько минут и сам обновляет отметку сторожа (`--heartbeat`). Любой `FAIL` — записать номер шага и текст в приложение A; `FAIL` шага 7 (контроль утечки) — блок «Если что-то пошло не так» немедленно.

- [ ] **Step 10: Выход из сети и повторный вход (нужен пользователь: браузер)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 60s logout | Select-Object -Last 1; "logout exit=$LASTEXITCODE"; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

Ожидается `SLCTL: backend=needs_login`, `logout exit=0`, в состоянии `phase=off ... backend=needs_login`. Затем скажите пользователю: «Сейчас я покажу ссылку для входа в Tailscale. Откройте её в браузере, войдите той же учётной записью и подтвердите устройство; в консоли Tailscale это то же устройство `<имя ПК>-splitlane`». Один вызов:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $job = Start-Job -ScriptBlock { for ($i = 0; $i -lt 9; $i++) { Start-Sleep -Seconds 30; . C:\code\router\spike\plan1\p1lib.ps1; Set-P1Heartbeat } }; & "$P1Bin\slctl.exe" -timeout 300s login; "login exit=$LASTEXITCODE"; $job | Stop-Job -PassThru | Remove-Job; Set-P1Heartbeat; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

`slctl login` печатает `SLCTL: open this link to log in: <ссылка>` — эту строку передайте пользователю в ответе (ссылка только в разговоре, не в отчётах и не в приложении A). Ожидается затем `SLCTL: backend=starting` или `running`, `login exit=0`, в состоянии `exitNodes` больше нуля. Если пользователь не успел за 300 секунд (`login exit=3`) — повторить этот вызов.

- [ ] **Step 11: Включение после входа, выключение и конец сеанса**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 240s enable | Select-Object -Last 1; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'; Set-P1Heartbeat; $d = & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; $d; if ($d -notmatch '^SLCTL: phase=off ') { 'STEP-STOPPED: disable did not reach off; shutdown and finish not sent'; return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 30s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 2; Set-P1Heartbeat; Send-P1Guard -Action finish; Test-P1SystemRestored
```

Ожидается `SLCTL: phase=on ...`, два `LANE-OK`, `SLCTL: phase=off ... systemExitNode=true ...`, `SLCTL: the helper closed the pipe`, `P1-RESULT: state=finished ... system_ok=True`, `SYSTEM-RESTORED`.

- [ ] **Step 12: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1B» приложения A добавьте (числа и итоги — из шагов 4–11; «подтверждено» или «НЕ подтверждено» по фактам):

```text
- План 1B, служебный процесс под сторожем: вид соединения с exit node и список exit node от службы API ядра в состоянии: <подтверждено>.
- План 1B: живая активность (событие activity, скорости обеих дорожек, подробности программы): <подтверждено>.
- План 1B: архив diagnostics.export без конфига ядра, состояния узла, ссылок для входа и секретов: <подтверждено>.
- План 1B: принудительное завершение ядра при работающем служебном процессе: системный exit node возвращён через <N> мс (с задержкой сторожа до секунды), деление включено заново за <N> секунд; оставшееся устройство Wintun удалено перед перезапуском: <подтверждено>.
- План 1B: отключение кабеля: перезапуск ядра по виду соединения «через ретранслятор» вернул прямое соединение: <подтверждено | не понадобился — узел вернулся сам>.
- План 1B: сценарная проверка раздела 12 (scripts/scenario.mjs), восемь шагов: <подтверждено | НЕ подтверждено: шаг N — текст>.
- План 1B: выход встроенного узла из сети и повторный вход по ссылке из состояния: <подтверждено>.
```

```powershell
git -C C:\code\router diff -- docs | Select-String -Pattern '\b\d{1,3}(\.\d{1,3}){3}\b' | Where-Object { $_.Line -notmatch '100\.64\.0\.0/10|160\.79\.104\.0/23|1\.1\.1\.1|127\.0\.0\.1' }
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record the live checks of plan 1B in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Первая команда должна вывести пустоту.

---

### Task 17: Установка в режиме продукта, перезагрузка и удаление

Проверка продукта целиком, без режима разработки: `install` из `dist\helper\` (задача 14) с запросом UAC, запуск задачи планировщика `SplitLane Helper` из процесса без прав администратора, включение и выключение под сторожем, перезагрузка при включённом делении (раздел 10: при следующем входе служебный процесс возвращает системный exit node по журналу и, раз окна нет, через 60 секунд завершается — задача 13), затем `uninstall`. Окна ещё нет (план 2): вместо него канал использует `slctl`. Чтобы не регистрировать в сети Tailscale второе устройство, установленный продукт получает копию состояния узла из данных разработки (одно и то же устройство `<имя ПК>-splitlane`; два ядра с ним одновременно не работают). Задачу целиком выполняет основной сеанс; перезагрузка завершает его, после неё пользователь запускает `claude --continue`.

**Files:**
- Modify (игнорируется git): `C:\code\router\spike\plan1\p1lib.ps1` (ключ `-NoHelper` у `Start-P1Guard`)
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Consumes: `dist\helper\` и `helper\bin\` задачи 15; подкоманды `install --user-sid` и `uninstall` (задача 14); режим продукта (задача 13); сторож плана 1A.

- [ ] **Step 1: Ключ `-NoHelper` у `Start-P1Guard`**

Сторож запускается с правами администратора, но служебный процесс не запускает: его запускает задача планировщика продукта. Правка в игнорируемом файле — одной командой:

```powershell
$p = 'C:\code\router\spike\plan1\p1lib.ps1'; $t = [IO.File]::ReadAllText($p); $pairs = @(
  @("    param([switch]`$DryRun)`n    if (-not (Test-P1Ready))", "    param([switch]`$DryRun, [switch]`$NoHelper)`n    if (-not (Test-P1Ready))"),
  @("    if (-not `$DryRun) {`n        `$argList += @('--'", "    if (-not `$DryRun -and -not `$NoHelper) {`n        `$argList += @('--'"),
  @("    if (-not `$DryRun) {`n        if (-not (Wait-P1Pipe))", "    if (-not `$DryRun -and -not `$NoHelper) {`n        if (-not (Wait-P1Pipe))")
); $t = $t.Replace("`r`n", "`n"); foreach ($pr in $pairs) { $c = ([regex]::Matches($t, [regex]::Escape($pr[0]))).Count; "found $c"; $t = $t.Replace($pr[0], $pr[1]) }; [IO.File]::WriteAllText($p, $t, (New-Object Text.UTF8Encoding $false)); $tok = $null; $err = $null; [void][System.Management.Automation.Language.Parser]::ParseFile($p, [ref]$tok, [ref]$err); "parse errors: $($err.Count)"
```

Ожидается три строки `found 1` и `parse errors: 0`.

- [ ] **Step 2: Согласие пользователя (нужен пользователь)**

Передайте пользователю текст ручного восстановления (Global Constraints плана 1A) и затем:

```text
Сейчас я установлю служебный процесс SplitLane как продукт (в C:\Program Files\SplitLane и C:\ProgramData\SplitLane, задача планировщика «SplitLane Helper»), проверю его, затем вы перезагрузите компьютер при включённом делении, а после входа я проверю, что Tailscale вернулся в прежнее состояние, и удалю установку. Запросов UAC будет четыре: установка, копирование состояния узла Tailscale в папку продукта, сторож и удаление. Перезагрузка завершит мой сеанс: после входа в Windows откройте терминал в C:\code\router, запустите `claude --continue` и напишите «продолжай задачу 17, шаг 9». Если после перезагрузки интернет в программах не работает — значок Tailscale в трее, Exit Nodes, выбрать прежний exit node.
```

- [ ] **Step 3: Установка (нужен пользователь: UAC)**

Вывод повышенного процесса попадает в файл: окна консоли у него нет. Вся команда для `cmd /c` взята в ещё одну пару кавычек: иначе `cmd` срезает первую и последнюю кавычку и перенаправление ломается.

```powershell
$sid = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value; $out = Join-Path $env:TEMP 'splitlane-install.txt'; Remove-Item $out -ErrorAction SilentlyContinue; $cmd = "`"`"C:\code\router\dist\helper\splitlane-helper.exe`" install --user-sid $sid > `"$out`" 2>&1`""; try { Start-Process -FilePath cmd.exe -ArgumentList '/c', $cmd -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop } catch { 'UAC-CANCELLED - nothing was installed. Do not retry without the user.'; return }; Get-Content $out
```

Ожидается последней строкой `INSTALL-OK`. `INSTALL-REFUSED: <причина>` — ничего не изменено (задача 14); причину устранить и повторить с согласия пользователя.

- [ ] **Step 4: Что установлено (без UAC)**

```powershell
$pf = 'C:\Program Files\SplitLane'; Get-ChildItem $pf | Select-Object Name; "core sha ok: $((Get-FileHash "$pf\sing-box.exe" -Algorithm SHA256).Hash -eq '8F30682EE198855440D2A7FE070D5D3653793B2D01BB8C85EEA88F93BBE7C8B9')"; $x = [xml]((schtasks /query /tn "SplitLane Helper" /xml) -join "`n"); "task: RunLevel=$($x.Task.Principals.Principal.RunLevel) LogonTrigger=$($null -ne $x.Task.Triggers.LogonTrigger) Command=$($x.Task.Actions.Exec.Command)"; try { Get-ChildItem C:\ProgramData\SplitLane -ErrorAction Stop | Out-Null; 'data folder readable by the user: True' } catch { 'data folder readable by the user: False' }; "export readable: $(Test-Path C:\ProgramData\SplitLane\export)"; "pipe before start: $([bool](Get-ChildItem \\.\pipe\ | Where-Object Name -eq 'SplitLane.Helper'))"
```

Ожидается: файлы `LICENSE`, `libcronet.dll`, `sing-box-LICENSE`, `sing-box.exe`, `splitlane-helper.exe`, `THIRD-PARTY.md`; `core sha ok: True`; `task: RunLevel=HighestAvailable LogonTrigger=True Command=C:\Program Files\SplitLane\splitlane-helper.exe`; `data folder readable by the user: False`; `export readable: True`; `pipe before start: False`.

- [ ] **Step 5: Состояние узла для продукта (нужен пользователь: UAC)**

Копируется каталог узла и `state.json` разработки (вход, список exit node, выбранный exit node, раскладка); служебного процесса продукта ещё нет, ядро не работает.

```powershell
if (Get-Process -Name splitlane-helper, sing-box -ErrorAction SilentlyContinue) { 'REFUSED: a helper or core runs'; return }; $c = "robocopy C:\code\router\.dev\data\tailscale C:\ProgramData\SplitLane\tailscale /E /NFL /NDL /NJH /NJS /NP; Copy-Item C:\code\router\.dev\data\state.json C:\ProgramData\SplitLane\state.json -Force"; $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($c)); try { Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-EncodedCommand', $enc -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop; 'COPY-DONE' } catch { 'UAC-CANCELLED - nothing was copied. Do not retry without the user.' }
```

Ожидается `COPY-DONE`.

- [ ] **Step 6: Сторож без служебного процесса и запуск задачи (нужен пользователь: UAC)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Start-P1Guard -NoHelper; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; schtasks /run /tn "SplitLane Helper" | Out-Null; "schtasks exit=$LASTEXITCODE"; & "$P1Bin\slctl.exe" -timeout 60s state | Select-Object -Last 1; $h = Get-Process splitlane-helper -ErrorAction SilentlyContinue | Select-Object -First 1; "helper path: $($h.Path)"
```

Ожидается `P1-RESULT: state=armed ...`, `schtasks exit=0`, `SLCTL: phase=off ... backend=starting` или `running` `... exitNodes=<n>` (окно — здесь `slctl` — подключилось, вход сохранён, служебный процесс запустил режим настройки), `helper path: C:\Program Files\SplitLane\splitlane-helper.exe`. Задача запущена из процесса без прав администратора без UAC (проверка (е)).

- [ ] **Step 7: Включение и выключение установленным продуктом**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" lanes C:\code\router\spike\plan1\lanes-1.json | Select-Object -Last 1; & "$P1Bin\slctl.exe" -timeout 240s enable | Select-Object -Last 1; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'; $core = Get-Process sing-box -ErrorAction SilentlyContinue | Select-Object -First 1; "core path: $($core.Path)"; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 240s enable | Select-Object -Last 1
```

Ожидается `RESULT {}`, `SLCTL: phase=on ...`, два `LANE-OK`, `core path: C:\Program Files\SplitLane\sing-box.exe`, `SLCTL: phase=off ... systemExitNode=true ...`, снова `SLCTL: phase=on ...`. Деление оставлено включённым для перезагрузки.

- [ ] **Step 8: Перезагрузка при включённом делении (нужен пользователь)**

Сторож остаётся взведённым и завершается вместе с сеансом Windows; служебный процесс сам получает уведомление о завершении сеанса (5.4, проверка (ж)). Если сторож успеет заметить при выключении «нет ни exit node, ни ядра», он только вернёт exit node — это не мешает проверке. Обновите отметку:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

Ожидается `SLCTL: phase=on ...`. Передайте пользователю: «Перезагрузите компьютер сейчас: Пуск → Выключение → Перезагрузка. После входа в Windows подождите две минуты, затем откройте терминал в C:\code\router, запустите `claude --continue` и напишите «продолжай задачу 17, шаг 9». Если после входа интернет не работает — значок Tailscale в трее, Exit Nodes, выбрать прежний exit node.»

Сторож при перезагрузке завершается вместе с сеансом; если пользователь задержится с перезагрузкой больше чем на 7 минут, сторож сработает сам (вернёт exit node, завершит процессы) — тогда шаг 9 покажет фазу `off` без перезагрузки, и перезагрузку нужно повторить по новому согласию с шага 6.

- [ ] **Step 9: После входа: возврат exit node и выход служебного процесса без окна**

Через две минуты после входа служебный процесс, запущенный триггером, уже вернул exit node по журналу и, не дождавшись окна за 60 секунд, завершился. Подключение к каналу сейчас запустило бы его снова, поэтому сначала — только чтение:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; "boot: $((Get-CimInstance Win32_OperatingSystem).LastBootUpTime)"; "helper running: $([bool](Get-Process splitlane-helper -ErrorAction SilentlyContinue)); core running: $([bool](Get-Process sing-box -ErrorAction SilentlyContinue)); guard running: $([bool](Get-Process slguard -ErrorAction SilentlyContinue))"; $x = Get-P1SystemExitNode; "system exit node on: $($x.On)"; Test-P1SystemRestored; "adapter SplitLane: $([bool](Get-NetAdapter -Name SplitLane -ErrorAction SilentlyContinue))"
```

Ожидается время загрузки после шага 8, `helper running: False; core running: False; guard running: False`, `system exit node on: True`, `SYSTEM-RESTORED`, `adapter SplitLane: False`. `system exit node on: False` — показать пользователю ручное восстановление (выбрать exit node в трее), затем разбор журнала. Журнал служебного процесса в каталоге продукта пользователю недоступен; его читает шаг 10 через `diagnostics.export`.

- [ ] **Step 10: Журнал после перезагрузки и удаление (нужен пользователь: UAC)**

```powershell
$B = 'C:\code\router\helper\bin'; schtasks /run /tn "SplitLane Helper" | Out-Null; & "$B\slctl.exe" -timeout 60s state | Select-Object -Last 1; $e = & "$B\slctl.exe" export | Select-Object -Last 1; $zip = ($e -replace '^SLCTL: export written: ', ''); Add-Type -AssemblyName System.IO.Compression.FileSystem; $z = [IO.Compression.ZipFile]::OpenRead($zip); $prev = $z.Entries | Where-Object FullName -eq 'logs/helper.log.prev'; if ($prev) { $r = New-Object IO.StreamReader($prev.Open()); $r.ReadToEnd() -split "`n" | Select-Object -Last 12 | ForEach-Object { $_ -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>' }; $r.Dispose() }; $z.Dispose(); $out = Join-Path $env:TEMP 'splitlane-uninstall.txt'; Remove-Item $out -ErrorAction SilentlyContinue; $cmd = "`"`"C:\code\router\dist\helper\splitlane-helper.exe`" uninstall > `"$out`" 2>&1`""; try { Start-Process -FilePath cmd.exe -ArgumentList '/c', $cmd -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop } catch { 'UAC-CANCELLED - nothing was removed. Do not retry without the user.'; return }; Get-Content $out; schtasks /query /tn "SplitLane Helper" 2>$null | Out-Null; "task exists: $($LASTEXITCODE -eq 0)"; "program dir exists: $(Test-Path 'C:\Program Files\SplitLane')"; "left: [$(@(Get-Process -Name splitlane-helper, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается: `SLCTL: phase=off ...`; в хвосте журнала прошлого запуска (`helper.log.prev` — служебный процесс, запущенный триггером при входе) строки старта, возврата по журналу (фаза `stopping`, затем `off`) и выхода через 60 секунд без окна; `UNINSTALL-OK`; `task exists: False`; `program dir exists: False`; `left: []`. Каталог данных `C:\ProgramData\SplitLane` остаётся (без `--purge`): в нём состояние узла; план 2 установит продукт заново и найдёт его.

- [ ] **Step 11: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1B» добавьте:

```text
- План 1B, режим продукта: install из dist\helper с UAC (файлы в %ProgramFiles%\SplitLane, каталог данных без доступа пользователя, export\ доступен на чтение, задача планировщика), запуск задачи без UAC, включение и выключение установленным служебным процессом и ядром из %ProgramFiles%\SplitLane: <подтверждено>.
- План 1B: перезагрузка при включённом делении: после входа системный exit node включён, адаптера нет, служебный процесс, запущенный триггером, без окна завершился через 60 секунд: <подтверждено>.
- План 1B: uninstall: задача и %ProgramFiles%\SplitLane удалены, процессов не осталось: <подтверждено>.
```

```powershell
git -C C:\code\router diff -- docs | Select-String -Pattern '\b\d{1,3}(\.\d{1,3}){3}\b' | Where-Object { $_.Line -notmatch '100\.64\.0\.0/10|160\.79\.104\.0/23|1\.1\.1\.1|127\.0\.0\.1' }
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record the product install, reboot and uninstall of plan 1B in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Первая команда должна вывести пустоту.

---

## Что остаётся плану 2

План 2 — окно (Electron, React, TypeScript, electron-vite, pnpm; раздел 9 и 4.1 спецификации): доска с двумя дорожками, добавление программ, карточка программы, экраны «Настройка», «Подключите VPN», состояния 9.5, трей и автозапуск, словарь текстов. Окно запускает `dist\helper\splitlane-helper.exe install --user-sid <SID>` с UAC (задача 14 этого плана), затем `schtasks /run /tn "SplitLane Helper"` и подключается к каналу. Модульные тесты окна и ручной список раздела 12 — в плане 2.

## Открытые вопросы владельцу продукта

- Причина задержки DNS на старте ядра (приложение A) не установлена; повтор запуска (задача 7) лечит следствие. Если задача 16 снова её поймает, собранный с `--core-log-level debug` журнал ядра нужно разобрать отдельно.
- Перезапуск ядра при `relay` после смены сети (задача 10) — решение исполнителя по итогам проверки (и); цена — разрыв связи на 5–25 секунд после каждой смены сети, при которой узел потерял прямое соединение.
