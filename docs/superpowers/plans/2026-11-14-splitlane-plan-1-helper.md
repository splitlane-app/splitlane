# SplitLane, план 1A: служебный процесс — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть три проверки на ПК, от которых зависит работа (большие пакеты UDP по дорожке VPN, запуск через планировщик (е), завершение сеанса Windows (ж)), создать репозиторий и служебный процесс `splitlane-helper.exe` на Go — канал, автомат включения и отката, сборку конфига ядра, файлы правил, журнал возврата — с модульными тестами на подменных реализациях, и один раз включить и выключить деление на ПК через тестовый клиент под независимым сторожем.

**Architecture:** Модуль Go `splitlane/helper` в каталоге `helper/`. Чистые пакеты: `protocol` (сообщения канала 5.1, состояние 5.2, проверка входных данных 11), `rules` (8.1–8.2: вид правила, регулярные выражения, отказы, вложенные правила с `invert`), `coreconfig` (раздел 6), `store` (`state.json`, `restore.json`, файлы правил, конфиг). Пакет `engine` — автомат фаз 5.1–5.7 и 8.3; всё, что трогает систему, стоит за интерфейсами `engine/ports.go`, поэтому каждая ветка 5.3–5.7 и каждая строка раздела 10 проверяется на подменах с часами, которые не ждут. Ядро скрыто за интерфейсом драйвера `CoreDriver` из раздела 4: собрать конфиг, записать дорожки (файлы правил и режим), получить и закрыть соединения, состояние коннектора по журналу ядра. `engine` не знает ни конфига sing-box, ни формата файлов правил, ни имён режимов, ни строк журнала, поэтому второе ядро этапа 3 — второй драйвер, а не переделка автомата. Реализации для Windows: `singbox` (единственный драйвер этапа 1: конфиг из `coreconfig`, файлы дорожек из `rules`, процесс ядра, `CTRL_BREAK_EVENT` через скрытую консоль ядра, Clash API), `systs` (`tailscale.exe` по полному пути), `winhost` (маршруты, пересылка, NRPT, адаптер, кэш DNS), `probe` (запрос внешнего адреса), `pipe` (канал с явным описателем безопасности), `session` (скрытое окно для `WM_ENDSESSION`). Программы: `cmd/splitlane-helper` (без окна консоли), `cmd/slctl` (тестовый клиент канала), `cmd/slguard` (сторож для живых шагов: ничего не берёт из `internal/`). Проверки (е) и (ж) — одноразовый прототип в `spike\plan1\echeck`.

**Tech Stack:** Go 1.27.4 (директива `go 1.26.0`, `CGO_ENABLED=0`), `golang.org/x/sys` v0.49.0, `github.com/Microsoft/go-winio` v0.6.4, sing-box 1.14.4, Windows PowerShell 5.1, Python 3.14 (только оснастка плана 0 в задаче 1).

**Spec:** `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` — разделы 4.1–4.3, 5.1–5.7, 6, 7 (выбор exit node), 8.1–8.3, 10, 11, 12 и приложение A. План 0 (`docs/superpowers/plans/2026-11-13-splitlane-plan-0-core-checks.md`) — источник фактов о ядре 1.14.4 и оснастки watchdog; файл больше 256 КБ, читается частями.

**Почему 1A, а не весь план 1.** Весь служебный процесс (ещё коннектор Tailscale с режимом настройки и службой API, живая активность 8.4, сон и смена сети, команда `install` с ACL и задачей, `diagnostics.export`, сценарная проверка) с полным кодом и тестами не помещается в один план из примерно 25 задач. План 1A доводит до работающего включения и выключения ядро продукта: автомат 5.3–5.7, раскладку 8.3, канал, сборку конфига, файлы правил, журнал возврата — и проверяет это один раз на ПК. Что входит в план 1B, перечислено в разделе «Что остаётся плану 1B» в конце.

## Global Constraints

- Ядро: только sing-box 1.14.4 `windows-amd64` без изменений. SHA-256 архива `sing-box-1.14.4-windows-amd64.zip`: `99185d75d314a07dbba4a130c8ad986cbeb629a07927975636b341ca61f991df`; SHA-256 `sing-box.exe` из него: `8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9`. Конфиг собирается только под эту версию.
- Go: тулчейн 1.27.4 в `%LOCALAPPDATA%\Programs\go\bin`. В инструменте PowerShell исполнителя `go` и `gofmt` по `PATH` не находятся (папка добавлена в `PATH` пользователя после запуска сеанса), поэтому каждый блок команд Go начинается с `$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path";`. Директива модуля `go 1.26.0`. Каждая команда `go` выполняется с `CGO_ENABLED=0` (в PowerShell: `$env:CGO_ENABLED='0'` в том же вызове). Ошибка `The term 'go' is not recognized` — в блоке пропущено начало; проверка формата (задача 22) выполняется так, что не найденный `gofmt.exe` даёт провал, а не пустой список.
- Зависимости ровно две, версии закреплены: `golang.org/x/sys v0.49.0` и `github.com/Microsoft/go-winio v0.6.4`. Правило выбора: самая новая версия, выпущенная не позже чем за 14 дней до даты плана (2026-11-14); `x/sys v0.50.0` от 2026-11-06 слишком свежая, `x/sys v0.49.0` от 2026-10-06 — та, которую требует `go-winio v0.6.4` (2026-10-13). Других модулей нет; `go.sum` получается командой `go mod tidy`.
- Путь модуля `splitlane/helper`, каталог `helper/`. Служебный процесс собирается с `-ldflags "-H=windowsgui"` (без окна консоли), ядро запускается с `CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW`.
- Канал `\\.\pipe\SplitLane.Helper`; описатель безопасности `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<SID пользователя>)`; протокол 1; сообщения — JSON по одному в строке, строка не длиннее 1 МиБ.
- Файлы, которые читает ядро (конфиг, файлы правил), и `state.json`, `restore.json` пишутся в UTF-8 без BOM, атомарно: временный файл в той же папке, сброс на диск, `MoveFileEx` с `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`, повтор при отказе (`internal/atomicfile`). В PowerShell — только `[IO.File]::WriteAllText` с `New-Object Text.UTF8Encoding $false`.
- Модульные тесты не трогают систему. `tailscale.exe`, запуск ядра командой `run`, маршруты, `%ProgramData%`, задачи планировщика — только за интерфейсами `engine`, в тестах подмены. Разрешено тестам: временные папки, именованные каналы с уникальным именем `SplitLane.Test.<число>`, скрытое окно в своём процессе, дочерний процесс — сам тестовый бинарник. С настоящим ядром — только `sing-box check` (переменная `SPLITLANE_CORE`). Тест консольного сигнала (`SPLITLANE_CONSOLE_TEST=1`) и чтение настоящей системы (`SPLITLANE_LIVE_READONLY=1`) включаются явно, в задаче 22.
- Живой шаг — любой шаг, после которого системный exit node может оказаться выключен. Он идёт только под сторожем: в задаче 1 — watchdog плана 0, в задаче 23 — `slguard.exe`, запущенный с правами администратора до служебного процесса. Каждый живой шаг — один вызов инструмента PowerShell с тайм-аутом 600000 мс; в задаче 23 он начинается с проверки, что сторож этого запуска жив и взведён (`Assert-P1Armed`; иначе шаг ничего не делает), затем обновляется heartbeat (`Set-P0Heartbeat` или `Set-P1Heartbeat`); итог опрашивается внутри того же вызова; между вызовами не больше 400 секунд.
- Запрос UAC принимает человек. Такие шаги помечены «нужен пользователь: UAC». Перед запросом исполнитель предупреждает пользователя. Отменённый запрос ничего не меняет в системе, повторять его без согласия пользователя нельзя. При безошибочном ходе запросов три: задачи 1, 2 и 23. Ещё один возможен только при провале проверки (е): уборка задачи и оставшихся процессов прототипа в задаче 2.
- В правила прямой дорожки попадают только переименованные копии: `curl-*.exe` (и `node-*.exe` оснастки плана 0 в задаче 1). `claude.exe`, `bun.exe`, `node.exe`, `python.exe`, `svchost.exe` и браузеры в прямое правило не попадают никогда.
- Закрепление программ исполнителя за дорожкой VPN (флаг `--dev-pin`: правило по именам `claude.exe`, `bun.exe`, `node.exe`, `python.exe`, `pythonw.exe` и правило по адресам `160.79.104.0/23`, `anthropic.com`, `claude.ai`, `claude.com`, оба перед правилами режимов, как правила 8 и 9 плана 0) существует только в сборке разработки и только вместе с `--dev`. В конфиге продукта его нет (тест `TestDevPinOnlyWhenAsked`, задача 8). В живых шагах плана 1A основная дорожка всегда VPN.
- Личные данные. В плане, спецификации и коммитах нет имён устройств и сети Tailscale, имени ПК, настоящих адресов, секретов, ссылок для входа. Настоящие значения берутся во время работы из локальных игнорируемых файлов (`spike\run\phase-b-v2.json`, `spike\plan1\guard\guard-state.json`); `slctl` и сторож закрывают адреса в выводе (`<ip>`); вывод с `-show-addresses` в отчёты не переносится.
- Файлы PowerShell содержат только ASCII: Windows PowerShell 5.1 читает скрипт без BOM в кодировке ANSI.
- Работа идёт в ветке `feature/plan-1a-helper` (создаётся в задаче 1). Каждая задача заканчивается коммитом; сообщение коммита — по-английски и заканчивается строкой `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Текст плана — по-русски; код, команды, идентификаторы и вывод программ — по-английски.

**Ручное восстановление.** Этот текст пользователь получает до первого шага с UAC (задача 1, шаг 2). Исполнитель показывает его снова при итоге `state=restore_unverified` (watchdog), `P1-RESULT: NO-TERMINAL-STATE`, `state=restore_blocked`, `P1-RESULT: no status`, `SYSTEM-WRONG`, `NO-GUARD-HELPER-LEFT` и всякий раз, когда сторож не дошёл до `tripped` или `finished`.

```text
Если интернет в программах пропал и не вернулся за 10 минут:
1. Нажмите значок Tailscale в трее, откройте список Exit Nodes и выберите прежний exit node.
2. Откройте Диспетчер задач (Ctrl+Shift+Esc), вкладка «Подробности», и снимите задачи
   splitlane-helper.exe, затем sing-box.exe (если они есть). Диспетчер попросит подтверждение
   администратора: это нормально, процессы запущены с повышенными правами.
3. Последним там же снимите сторож проверок, если он остался: slguard.exe, а также python.exe,
   запущенный от имени администратора (сторож плана 0). Пока exit node не выбран, сторож сам
   повторяет его выбор, поэтому снимайте его только после шага 1.
Порядок важен: сначала exit node, потом процессы.

Если компьютер во время проверок перезагрузился, выключился или завис и был перезапущен:
после входа в Windows VPN может оказаться выключенным, и тогда программы идут в интернет
напрямую без всякого предупреждения. Сразу нажмите значок Tailscale в трее и, если exit node
не выбран, выберите прежний.
```

## Review Focus

Условия, которые спецификация подразумевает, но прямо не описывает; от более вероятного к менее вероятному. Для каждого в задачу, которая владеет кодом, добавлен тест.

1. Пользователь снова выбрал exit node в системном Tailscale (фаза `blocked`), и в это время падает ядро. Ожидание: выбор пользователя не перезаписывается значением из журнала возврата и не выключается шагом 6 перезапуска. Перезапуск по 5.5 выключил бы exit node, который пользователь только что выбрал сам, поэтому в `blocked` действует 5.7 («выбор пользователя не меняется»): перезапуска нет, выполняется 5.4 без возврата (журнал удаляется), фаза `error` с причиной `core_crashed`; включить деление снова — решение пользователя. Закреплено в задаче 11: `TestCrashInBlockedKeepsTheUsersExitNode`; решение вынесено владельцу продукта в «Открытые вопросы».
2. `lanes.set` приходит, когда ядро ещё не перечитало новый файл (около секунды): закрытое сверкой соединение открывается заново по старым правилам. Ожидание: сверка закрывает его снова, и `lanes.set` успешен, если к концу 3 секунд расхождений нет. Закреплено в задаче 14: `TestLanesSetConvergesWhileTheCoreReloads`.
3. Окно зависло и перестало читать канал. Ожидание: служебный процесс не блокируется, зависший клиент отключается, остальные получают события. Закреплено в задаче 18: `TestStalledClientIsDropped`.
4. Сервис определения адреса отвечает ошибкой HTTP (например, 503). Ожидание: это не провал связи, дорожка VPN не уходит в `degraded`. Закреплено в задаче 13: `TestHTTPErrorIsNotAFailure`.
5. Windows PowerShell 5.1 теряет двойные кавычки в аргументах внешних программ (`slctl call routing.allDirect '{"on":true}'` доходит как `{on:true}`). Ожидание: клиент сразу сообщает об ошибке, а не ждёт ответа на испорченный запрос. Закреплено в задаче 20: `TestErrorsAndRejections`.

---

## Как читать этот план

**Порядок задач.** Задачи 1–3 — проверки на ПК, которые спецификация требует до основного кода; их провал меняет работу (таблица ниже). Задача 4 создаёт репозиторий и модуль. Задачи 5–21 — код с тестами, система не трогается. Задача 22 — сборка и проверки на ПК без переключений. Задача 23 — включение и выключение деления под сторожем.

| Проверка | Провал |
|---|---|
| Задача 1, большие пакеты UDP по дорожке VPN | вопрос владельцу продукта (приложение A); задачи 2 и 3 можно выполнять, задачу 4 — только после ответа владельца |
| Задача 2, (е) запуск через планировщик | по разделу 12 работа останавливается и возвращается владельцу продукта; задача 4 не начинается |
| Задача 3, (ж) завершение сеанса | работу плана 1A не останавливает; в отчёте и в приложении A прямо сказано, что путь завершения сеанса из 5.4 в план 1A входит непроверенным. Запуск задачи при входе пользователя (`J4`, последняя часть (е)) проверяется только здесь: `J4 FAIL` — провал (е), работа останавливается; пропуск — запуск при входе «не проверялось», это блокирует команду `install` плана 1B |

**Кто выполняет.** Задачи 1, 2, 3 и 23 целиком и шаг 2 задачи 1 выполняет исполнитель, который сам ведёт разговор с пользователем (основной сеанс, superpowers:executing-plans). Подчинённому агенту их передавать нельзя: у него нет канала к пользователю, он не может показать ручное восстановление, предупредить о запросе UAC и дождаться ответа, а его фоновые процессы завершаются вместе с ним. Задачи 4–22 можно отдавать подчинённым агентам.

**Код в плане.** Файл плана больше 256 КБ и не помещается в один вызов инструмента чтения: читайте его частями (параметры `offset` и `limit`, по 500 строк). Блок после строки «Файл `путь`, целиком:» переносится в файл полностью, от строки после открывающих обратных апострофов до строки перед закрывающими. Пути вида `helper/...` — от корня репозитория `C:\code\router`; пути `C:\code\router\spike\plan1\...` — в игнорируемой папке. После записи файла сверяйте его последнюю строку с последней строкой блока. Файлы Go пишутся в UTF-8 без BOM (`Write` инструмента или `[IO.File]::WriteAllText`).

**Команды.** Блоки `powershell` рассчитаны на инструмент PowerShell (Windows PowerShell 5.1). Команды `go` выполняются из каталога `C:\code\router\helper`, если не сказано иное. Если в выводе теста `go test` нет строки, которую ждёт шаг, а есть `Traceback`, `panic` или ошибка сборки — исправьте файл по плану и повторите шаг; живые шаги в этом случае не начинаются.

### Карта файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `C:\code\router\spike\plan1\udp_cf.py` | замер UDP на сервере Cloudflare через оснастку плана 0 | 1 |
| `C:\code\router\spike\plan1\echeck\` (`go.mod`, `proto\main.go`, `checkctl\main.go`, `checkctl\main_test.go`, `echecks.ps1`, `jcheck.ps1`) | одноразовый прототип для (е) и (ж) | 2, 3 |
| `.gitignore`, `README.md`, `THIRD-PARTY.md`, `LICENSE` | корень репозитория (4.1) | 4 |
| `scripts/fetch-core.ps1`, `scripts/core.sha256` | получение ядра с проверкой SHA-256 | 4 |
| `helper/go.mod`, `helper/go.sum` | модуль `splitlane/helper` | 4, 18 |
| `helper/internal/atomicfile/` | атомарная запись без BOM | 4 |
| `helper/internal/protocol/` | сообщения канала, состояние, проверка входных данных | 5 |
| `helper/internal/rules/matcher.go` | вид правила и `matcher.value`, регулярные выражения | 6 |
| `helper/internal/rules/layout.go` | отказы, вложенные правила, файлы правил, дорожка процесса | 7 |
| `helper/internal/coreconfig/` | конфиг sing-box 1.14.4, режимы | 8 |
| `helper/internal/store/` | `state.json`, `restore.json`, файлы правил, конфиг | 9 |
| `helper/internal/engine/ports.go`, `engine.go`, `teardown.go` | интерфейсы (в том числе драйвер ядра `CoreDriver`), состояние, выключение и откат 5.4 | 10 |
| `helper/internal/engine/enable.go` | включение 5.3, поток надзора и перезапуск 5.5 | 11 |
| `helper/internal/engine/startup.go` | старт по журналу 5.7, завершение сеанса 5.4 | 12 |
| `helper/internal/engine/supervise.go` | проверки 5.5, системный Tailscale 5.6 | 13 |
| `helper/internal/engine/commands.go` | таблица фаз 5.1, `lanes.set` 8.3, «всё напрямую» | 14 |
| `helper/internal/singbox/` | драйвер ядра sing-box: конфиг, файлы дорожек, режимы, процесс, консольный сигнал, журнал, Clash API | 15 |
| `helper/internal/systs/` | `tailscale.exe` | 16 |
| `helper/internal/winhost/`, `helper/internal/probe/` | вопросы к Windows, запрос внешнего адреса | 17 |
| `helper/internal/pipe/` | именованный канал | 18 |
| `helper/internal/session/`, `helper/cmd/splitlane-helper/` | окно завершения сеанса, программа служебного процесса | 19 |
| `helper/cmd/slctl/` | тестовый клиент канала | 20 |
| `helper/cmd/slguard/`, `C:\code\router\spike\plan1\p1lib.ps1` | сторож живых шагов и его команды PowerShell | 21 |

### Раздел 10 спецификации и тесты плана 1A

| Строка раздела 10 | Тест (задача) |
|---|---|
| Ядро не запустилось или конфиг не прошёл проверку | `TestCoreDoesNotStart`, `TestEarlyExitIsRetriedOnce` (11) |
| Туннель не доказан на шаге 5 | `TestTunnelNotProven` (11) |
| Проверка после переключения не прошла | `TestSwitchCheckFails`, `TestLeakAtSwitch` (11) |
| Ядро упало во время работы | `TestCrashRestoresAtOnceAndRestarts`, `TestThreeFailedRestarts`, `TestCrashInBlockedKeepsTheUsersExitNode` (11); сторож живых шагов — `TestNoVPNPathTrips` (21) |
| Сервер VPN недоступен | `TestDegradedAndBack`, `TestNoNetworkChangesNothing`, `TestHTTPErrorIsNotAFailure` (13) |
| Путь через адаптер два раза подряд отвечает с адреса прямой дорожки | `TestLeakWhileOn`, `TestLeakAfterDirectAddressChange`, `TestAddressesFollowTheChecks` (13) |
| Путь через адаптер не отвечает при работающем `probe-vpn` | `TestBrokenCoreIsRestarted`, `TestAdapterFailuresNeedAWorkingVPN` (13) |
| Пользователь включил exit node в системном Tailscale | `TestBlockedPhase`, `TestClearSystemExitNodeJournalsUsersChoice` (13), `TestClearSystemExitNodeByPhase`, столбец `blocked` в `TestPhaseTable` (14) |
| Команда возврата системного exit node не срабатывает | `TestUnconfirmedRestoreKeepsTheCoreUntilSkip` (10), `TestStartWithUnconfirmedRestore` (12), `TestDisableInRestorePending` (14) |
| Окно упало или закрыто | `TestBroadcastToEveryClient`, `TestStalledClientIsDropped` (18): клиент уходит, канал продолжает работать; `TestLastWindowGoneKeepsRouting` (13): после ухода последнего окна деление и проверки 5.5 и 5.6 продолжаются, ядро не тронуто |
| Служебный процесс упал | `TestStartAfterHelperDiedBetweenSteps6And9`, `TestStartAfterCrashWhileOn`, `TestLeftoverCoreWithoutJournal`, `TestForeignCoreIsNotTouched` (12); на ПК — задача 23, шаг 7 |
| Файл правил не применился | `TestLanesSetFailuresRestorePrevious` (14) |
| Сон и пробуждение, смена сети | план 1B (уведомления о питании и маршрутах); в 1A — только `TestNoNetworkChangesNothing` (13) |
| Завершение сеанса Windows при включённом делении | `TestSessionEndWhileOn`, `TestSessionEndInBlockedKeepsTheUsersExitNode`, `TestSessionEndWhileOff` (12), `TestEndSessionCallsTheHandler` (19); на ПК — задача 3 |
| Перезагрузка, отключение питания или сбой Windows | `TestStartAfterCrashWhileOn` (12): журнал и «было включено» при следующем старте |

### Факты, на которые опирается план

Факты о ядре 1.14.4 собраны в плане 0, раздел «Факты о ядре 1.14.4, на которые опирается план», и здесь не повторяются; план 1A опирается на них в пакетах `coreconfig`, `singbox` и `engine`. Дополнительно, по исходному коду метки `v1.14.4`:

- Поле `rule` в `GET /connections` — текст правила и действие: `rule_set=lane-direct => route(direct)`, `clash_mode=vpn => route(vpn)`, либо `final` (`experimental/clashapi/connections.go`, `route/rule/rule_abstract.go`, `route/rule/rule_item_rule_set.go`, `route/rule/rule_item_clash_mode.go`). По нему служебный процесс отличает соединения, чью дорожку решают правила 8–11 из 6.2.
- `chains[0]` — тег выхода соединения; в Windows `metadata.processPath` — путь из `QueryFullProcessImageName` без добавок (`common/trafficcontrol/tracker.go`, `common/process/searcher_windows.go`).
- По `os.Interrupt` (так Go видит `CTRL_BREAK_EVENT`) ядро закрывается и выходит с кодом 0; если закрытие не уложилось в предел ядра, оно пишет `FATAL ... sing-box did not close!` и выходит с кодом 1 (`cmd/sing-box/cmd_run.go`). Строки об успешном закрытии нет.
- Журнал, направленный в файл, пишется без цветовых кодов; поля `log.disable_color` в 1.14.4 нет (`sing-box check` его отвергает).
- Проверено при подготовке плана: `CTRL_BREAK_EVENT` через `AttachConsole` к скрытой консоли дочернего процесса, запущенного с `CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW`, останавливает его штатно (тест `TestInterruptStopsTheCore` на подменном ядре); из процесса без консоли (служебный процесс) это проверяет задача 2.

### Что уже проверено при подготовке плана

Без запуска ядра командой `run`, без watchdog, без UAC и без изменения настроек:

- Весь код Go этого плана перенесён из текста плана скриптом в `spike\plan1\verify\` и там собран (`CGO_ENABLED=0`): `go vet ./...` без замечаний, `gofmt -l` пусто, `go test ./...` проходит, в том числе с `SPLITLANE_CORE` (каждый конфиг из `TestCoreAcceptsConfigs` — пять режимов, с закреплением и без — прошёл `sing-box check` настоящего ядра 1.14.4), с `SPLITLANE_CONSOLE_TEST=1` и с `SPLITLANE_LIVE_READONLY=1`. Тесты движка прогнаны 15 раз подряд без сбоев.
- Для каждой задачи с кодом проверено, что файлы предыдущих задач плюс только тесты этой задачи дают ровно ту ошибку, которую называет её шаг «Запустить тест, убедиться в провале», а вместе с реализацией — проходят.
- Прототип задачи 2 собран, тест XML задачи проходит; `echecks.ps1`, `jcheck.ps1`, `p1lib.ps1`, `fetch-core.ps1` разобраны парсером PowerShell без ошибок и состоят из ASCII.
- `udp_cf.py selftest` проходит. `fetch-core.ps1 -Zip` с настоящим архивом печатает `FETCH-OK`, с подменным — `FETCH-FAILED` и удаляет архив.
- Служебный процесс собран и запущен без прав администратора с временной папкой данных и тестовым каналом: `slctl state` сразу после запуска, без паузы (клиент дождался канала) — фаза `off`, `slctl shutdown` — процесс завершился с кодом 0. Сторож `slguard.exe` запущен без прав администратора и без служебного процесса: `armed`, по команде `finish` — `finished system_ok=True`; так же через `Start-P1Guard -DryRun`, при этом `Assert-P1Armed` дал `True` у взведённого сторожа и `P1-NO-GUARD` после `finish`. `Test-P1Ready` на папке данных с `wasOn: true` и `restore.json` ответил `P1-NOT-READY: development data left by a tripped run`, после `slctl reset-dev` — `P1-READY`. `Wait-P1Pipe` без служебного процесса вернул `False`.
- Каждый новый тест второй редакции проверен на прежнем коде и падает без своего исправления: `TestLeakAfterDirectAddressChange`, `TestAddressesFollowTheChecks`, `TestSessionEndInBlockedKeepsTheUsersExitNode`, `TestWaitsForThePipe`, `TestExitNodeLostAfterCoreKillIsSetAgain`; `TestPhaseTable` ловит подмену ответа `busy` на `wrong_phase` в клетке `routing.allDirect` / `starting`.
- Третья редакция (интерфейс `CoreDriver`, сброс кэша DNS при «всё напрямую», сбой ядра в `blocked`, `skipRestore` по 5.1, ответ на `helper.shutdown`, сторож без пути VPN): каждый её тест падает без своего исправления. `TestCrashInBlockedKeepsTheUsersExitNode` — без ветки `blocked` в потоке надзора (перезапуск доходит до `on`, выключив exit node пользователя); `TestAllDirect` — без сброса кэша при переключении, со сбросом до переключения, без сброса после возврата прежнего режима и со сбросом в `degraded`; `TestPhaseTable` — клетки `routing.disable` с `skipRestore` в `on` и `degraded`, если признак не передан выключению; `TestShutdownReplyIsNotLost` — на прежнем `Server.Close` ответ теряется в первом же круге; `TestNoVPNPathTrips` и `TestExitNodeLostBeforeCoreKillIsSetFirst` — на прежнем сторожe; `TestNoVPNPathTrips` ещё и без выдержки 10 секунд и с выдержкой короче. Проверки на ПК из пунктов выше (служебный процесс в фазе `off`, пробный запуск сторожа) выполнялись на второй редакции; задача 22 повторяет их на этой.
- Все блоки `powershell` плана разобраны парсером PowerShell без ошибок. Передача команды уборки задачи 2 через `-EncodedCommand` проверена безвредной командой без повышения прав: имя задачи в кавычках дошло до дочернего `powershell.exe` целым.

Сами живые утверждения (переключение, (е), (ж), UDP) этим не проверены: их проверяют задачи 1–3, 22, 23.

---

### Task 1: Большие пакеты UDP по дорожке VPN на сервере Cloudflare

Вопрос владельцу продукта из приложения A (запись `K3`): проходят ли по дорожке VPN пакеты UDP больше примерно 1250 байт. Все прогоны плана 0 шли на сервер Google, который не отвечает на такие пакеты и через системный exit node; сервер Cloudflare через системный exit node ответил на все размеры. Задача повторяет замер на сервере Cloudflare в одном сеансе watchdog плана 0 с одним запросом UAC. Контроль — тот же сервер и те же размеры с копии прямой дорожки.

**Files:**
- Create: `C:\code\router\spike\plan1\udp_cf.py`
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Consumes (план 0, не меняются): `C:\code\router\spike\plan0\lib.ps1` — `Set-P0Heartbeat`, `Start-P0Watchdog -Mode engage -Config <файл>`, `Send-P0Command -Action stop`; `C:\code\router\spike\plan0\p0lib.py` — `require_state`, `get_mode`, `set_mode`, `lanes`, `heartbeat`, `record_live`, `die`, `NODE_VPN`, `NODE_DIRECT`; `C:\code\router\spike\plan0\check_revmap.py` — `stun_lanes(node, server, port, pads, vpn, direct)`, `PADS` (размеры 20, 624, 1124, 1224, 1324, 1424 байта), `ATTEMPTS = 3`, `payload(pad)`; `C:\code\router\spike\plan0\prepare.py`; конфиг `C:\code\router\spike\run\p0-main-vpn.json` (его заново собирает `prepare.py`).
- Produces: записи `K6-udp-vpn-cloudflare-sizes` и `K7-udp-direct-cloudflare-sizes` в `C:\code\router\spike\plan0\out\results.json`; строки в приложении A.

- [ ] **Step 1: Создать ветку**

```powershell
git -C C:\code\router switch -c feature/plan-1a-helper; git -C C:\code\router status --short
```

Ожидается: `Switched to a new branch 'feature/plan-1a-helper'` и в `status` ровно одна строка — сам файл плана, ещё не добавленный в git (его добавляет шаг 10):

```text
?? docs/superpowers/plans/2026-11-14-splitlane-plan-1-helper.md
```

Если в `status` есть любая другая строка — остановиться и выяснить у пользователя, чьи это изменения.

- [ ] **Step 2: Показать пользователю ручное восстановление (нужен пользователь: прочитать)**

Передайте пользователю целиком текст из блока «Ручное восстановление» раздела Global Constraints. Продолжайте после ответа, что текст прочитан.

- [ ] **Step 3: Записать `udp_cf.py`**

Файл `C:\code\router\spike\plan1\udp_cf.py`, целиком:

```python
"""Plan 1, task 1: large UDP datagrams through the VPN lane, measured on a STUN server that answers
large datagrams through the system exit node (stun.cloudflare.com:3478). Owner question of
appendix A (record K3): do UDP datagrams larger than about 1250 bytes pass the VPN lane?

usage: udp_cf.py            the measurement; records K7 (control, direct lane) and K6 (VPN lane)
       udp_cf.py selftest   the verdict rules on scripted answers; needs no core and no session
Run in watchdog state "ready" with p0-main-vpn.json, core in mode vpn, base rule files.
"""
import os
import sys

sys.path.insert(0, r"C:\code\router\spike\plan0")
from p0lib import *  # noqa: E402,F401  (Plan 0 helpers: session, lanes, records)
import check_revmap as cr  # noqa: E402  (stun_lanes, k3_verdict, PADS, payload)

SERVER, PORT = "stun.cloudflare.com", 3478


def lane_verdict(seen, want):
    """(ok, lanes, late, lost) for one copy; ok is None when no size was answered at all.

    PASS: every size answered from the wanted lane, even if not at the first attempt.
    FAIL: an answer from another lane, or some sizes silent three times while others answered.
    """
    lanes_seen = {str(size): tries[-1] for size, tries in seen.items()}
    lost = [size for size, tries in seen.items() if tries[-1] == "NONE"]
    late = [size for size, tries in seen.items() if len(tries) > 1 and tries[-1] != "NONE"]
    ok = None if len(lost) == len(seen) else all(lane == want for lane in lanes_seen.values())
    return ok, lanes_seen, late, lost


def note_for(name, lanes_seen, late, lost):
    note = "%s, payload size -> lane: %s" % (name, lanes_seen)
    if late:
        note += "; answered only when sent again: %s" % late
    if lost:
        note += "; no answer in %d attempts: %s" % (cr.ATTEMPTS, lost)
    return note


def selftest():
    vpn, direct, t = "203.0.113.10", "192.0.2.1", "TIMEOUT"

    def play(script, want):
        def scripted(node, server, port, pads):
            answers = {pad: script.get(pad, [vpn if want == "VPN" else direct]).pop(0) for pad in pads}
            return {cr.payload(pad): text for pad, text in answers.items() if text is not None}

        cr.stun, cr.heartbeat = scripted, (lambda: None)
        return lane_verdict(cr.stun_lanes("node", "server", 0, cr.PADS, vpn, direct), want)

    ok, lanes_seen, late, lost = play({}, "VPN")
    assert ok is True and not late and not lost, "all sizes at once through the VPN lane"
    ok, lanes_seen, late, lost = play({1300: [t, t, t], 1400: [t, t, t]}, "VPN")
    assert ok is False and lost == [1324, 1424], "large sizes lost three times: FAIL"
    ok, lanes_seen, late, lost = play({1400: [t, direct]}, "DIRECT")
    assert ok is True and late == [1424], "the control copy, one late answer: PASS"
    ok, lanes_seen, late, lost = play({600: [vpn]}, "DIRECT")
    assert ok is False and lanes_seen["624"] == "VPN", "the control copy answered from the VPN lane: FAIL"
    ok, lanes_seen, late, lost = play({pad: [t, t, t] for pad in cr.PADS}, "VPN")
    assert ok is None, "silence everywhere: no verdict"
    print("ok   udp_cf: verdicts for the VPN lane copy and the direct lane control")


def main():
    if sys.argv[1:] == ["selftest"]:
        selftest()
        return
    status = require_state("ready")
    running = os.path.basename(str(status.get("config")))
    if running != "p0-main-vpn.json":
        die("SESSION-NOT-READY: udp_cf.py needs the core started with p0-main-vpn.json, status.json says %s" % running)
    mode, _ = get_mode()
    if mode != "vpn":
        set_mode("vpn")
        die("this check starts in mode vpn, the core was in %s; switched back to vpn, run the step again" % mode)
    vpn, direct = lanes()
    control = lane_verdict(cr.stun_lanes(NODE_DIRECT, SERVER, PORT, cr.PADS, vpn, direct), "DIRECT")
    heartbeat()
    measured = lane_verdict(cr.stun_lanes(NODE_VPN, SERVER, PORT, cr.PADS, vpn, direct), "VPN")
    heartbeat()
    if control[0] is None:
        print("INFO K7-udp-direct-cloudflare-sizes the server answered no size from the direct lane copy")
    else:
        record_live("K7-udp-direct-cloudflare-sizes", control[0],
                    note_for("STUN %s:%d from the direct lane copy (control)" % (SERVER, PORT), *control[1:]),
                    lanes=control[1])
    if measured[0] is None:
        die("MEASUREMENT-FAILED: the STUN server answered none of the %d sizes through the VPN lane "
            "(the direct lane copy got answers: %s); K6 is not recorded" % (len(cr.PADS), control[0] is not None))
    record_live("K6-udp-vpn-cloudflare-sizes", measured[0],
                note_for("STUN %s:%d through the VPN lane" % (SERVER, PORT), *measured[1:]), lanes=measured[1])


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Самопроверка правил оценки (без ядра и без сеанса)**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan1\udp_cf.py selftest
```

Ожидается одна строка:

```text
ok   udp_cf: verdicts for the VPN lane copy and the direct lane control
```

- [ ] **Step 5: Состояние ПК**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается последней строкой `PREPARE-OK`, перед ней строки `STATE ... OK` (системный exit node включён, `sing-box.exe` не запущен, сеанса watchdog нет) и `CHECK-OK p0-main-vpn.json`. Скрипт ничего не меняет в системе. При `PREPARE-FAILED` — устранить причину из строк `WRONG`; при строке `STATE no live watchdog session WRONG` — по таблице «Состояния watchdog и что делать» плана 0.

- [ ] **Step 6: Получить согласие, затем один сеанс watchdog (нужен пользователь: согласие, UAC)**

Передайте пользователю и дождитесь ответа:

```text
Сейчас я один раз включу деление проверочным ядром (около 2–4 минут) и выключу его. При включении и выключении связь у всех программ компьютера пропадёт на 5–25 секунд; открытые соединения (звонки, игры, загрузки) оборвутся. Основная дорожка всё время — VPN. В значке Tailscale в это время ничего не переключайте. Сейчас появится запрос UAC от Python: его нужно принять в течение 120 секунд; отмена ничего не меняет.
```

После ответа — инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Start-P0Watchdog -Mode engage -Config p0-main-vpn.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan1\udp_cf.py; Set-P0Heartbeat; Send-P0Command -Action stop
```

Ожидается (идентификаторы будут другими; включение занимало около 8 секунд):

```text
P0-LAUNCH: run=1a2b3c4d mode=engage config=p0-main-vpn.json - the UAC prompt must be accepted within 120 s
P0-STATUS: state=starting run=1a2b3c4d seq=1 cmd_id= config=p0-main-vpn.json
P0-STATUS: state=engaging run=1a2b3c4d seq=2 cmd_id= config=p0-main-vpn.json
P0-STATUS: state=ready run=1a2b3c4d seq=3 cmd_id= config=p0-main-vpn.json
P0-RESULT: state=ready run=1a2b3c4d seq=3 cmd_id= config=p0-main-vpn.json
PASS K7-udp-direct-cloudflare-sizes STUN stun.cloudflare.com:3478 from the direct lane copy (control), payload size -> lane: {'20': 'DIRECT', '624': 'DIRECT', '1124': 'DIRECT', '1224': 'DIRECT', '1324': 'DIRECT', '1424': 'DIRECT'}
PASS K6-udp-vpn-cloudflare-sizes STUN stun.cloudflare.com:3478 through the VPN lane, payload size -> lane: {'20': 'VPN', '624': 'VPN', '1124': 'VPN', '1224': 'VPN', '1324': 'VPN', '1424': 'VPN'}
P0-COMMAND: stop  id=c1a2b3c4
P0-STATUS: state=rolling_back run=1a2b3c4d seq=4 cmd_id=c1a2b3c4 reason=stop command
P0-STATUS: state=rolled_back run=1a2b3c4d seq=5 cmd_id=c1a2b3c4 reason=stop command system_vpn_ok=True
P0-RESULT: state=rolled_back run=1a2b3c4d seq=5 cmd_id=c1a2b3c4 reason=stop command system_vpn_ok=True
```

Другой итог `P0-RESULT` — по таблице «Состояния watchdog и что делать» плана 0 (Global Constraints плана 0). `P0-RESULT: UAC-CANCELLED`: система не изменена, без согласия пользователя не повторять. Строка `ABORT SESSION-NOT-READY` или `ABORT SESSION-LOST` — записей нет; если `Send-P0Command` напечатал `NO-LIVE-SESSION`, сеанс уже закончился сам. Строка `ABORT MEASUREMENT-FAILED` — сервер не ответил ни на один размер через дорожку VPN; запись `K6` не сделана, `K7` (контроль) — сделана; повторить шаг один раз с согласия пользователя (новый запрос UAC). `Traceback` — ошибка в скрипте: сеанс закрывает `Send-P0Command -Action stop` в той же строке; исправить файл по плану и повторить шаг.

- [ ] **Step 7: Проверить систему после сеанса**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается `PREPARE-OK`. При `WRONG` в строках `STATE system exit node on` или `STATE no sing-box.exe running`, а также после `state=restore_unverified` — показать пользователю ручное восстановление и повторить команду.

- [ ] **Step 8: Оценить результат**

| `K7` (контроль, прямая дорожка) | `K6` (дорожка VPN) | Вывод | Что дальше |
|---|---|---|---|
| `PASS` | `PASS` | большие пакеты UDP по дорожке VPN проходят; провал плана 0 объяснялся сервером Google | вопрос владельцу закрыт, работа продолжается |
| `PASS` | `FAIL`, без ответа только большие размеры | сервер отвечает на большие пакеты, а дорожка VPN их теряет | вопрос владельцу продукта; задачи 2 и 3 выполнять можно, задачу 4 — после ответа |
| `PASS` | `FAIL`, ответ с другой дорожки | копия дорожки VPN вышла не через VPN | расхождение с приложением A, вопрос владельцу продукта, как в предыдущей строке |
| `FAIL`, в `lane:` есть ответ `VPN` | любой | правило прямой дорожки оснастки плана 0 не сработало: копия прямой дорожки вышла через VPN — расхождение с планом 0, а не свойство сервера | `K6` не оценивать; разобрать оснастку плана 0 до ответа владельцу; задачи 2 и 3 выполнять можно |
| `FAIL` только из-за размеров без ответа (`NONE`) или `INFO` (сервер не ответил прямой копии) | любой | замер не состоялся: сервер недоступен или фильтруется | вопрос остаётся открытым; сообщить владельцу; задачи 2 и 3 выполнять можно |

- [ ] **Step 9: Записать итог в приложение A**

В спецификации после пункта «Решения для плана 1 по итогам плана 0» (перед абзацем «Вопросы владельцу продукта…») вставьте абзац (дата — дата выполнения):

```text
Проверки плана 1 (2026-11-14), та же система; адреса и имена устройств не приводятся:

- UDP по дорожке VPN на сервере Cloudflare (stun.cloudflare.com:3478), пакеты от 20 до 1424 байт, без разбора протоколов: <подтверждено | НЕ подтверждено (без ответа: <размеры>) | не проверялось (<причина>)>. Контроль — те же размеры с копии прямой дорожки: <все ответили | без ответа: <размеры>>.
```

Абзац «Вопросы владельцу продукта, для которых в спецификации нет запасного способа…» замените по итогу шага 8: при `K6 PASS` — словами «Вопросов владельцу продукта без запасного способа нет: замер UDP на сервере Cloudflare (проверки плана 1) показал, что пакеты до 1424 байт проходят по дорожке VPN.»; иначе — оставьте вопрос и добавьте к нему одну фразу с итогом замера на сервере Cloudflare. В предложении «Не проверялось: …» в конце приложения уберите слова «большие пакеты UDP по дорожке VPN на сервере, который отвечает на них через системный exit node», если `K6` записан.

- [ ] **Step 10: Проверить, что в коммит не попадают личные данные, и закоммитить**

```powershell
git -C C:\code\router diff -- docs | Select-String -Pattern '\b\d{1,3}(\.\d{1,3}){3}\b' | Where-Object { $_.Line -notmatch '100\.64\.0\.0/10|160\.79\.104\.0/23|1\.1\.1\.1|127\.0\.0\.1' }
```

Ожидается пустой вывод. Затем (файл плана добавляется, если его ещё нет в git):

```powershell
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md docs/superpowers/plans/2026-11-14-splitlane-plan-1-helper.md
git -C C:\code\router commit -m "docs: record the Cloudflare UDP measurement on the VPN lane in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Сообщите владельцу продукта итог шага 8 одним абзацем.

---

### Task 2: (е) Запуск через планировщик задач — прототип

Проверка (е) раздела 12: задача из XML (от имени пользователя настройки, наивысшие права, запуск по требованию, триггер при входе, без ограничения времени и условий питания, `IgnoreNew`) стартует без UAC из процесса без прав администратора; окна консоли нет; канал с явным описателем безопасности доступен такому процессу; ядро останавливается штатным сигналом; после принудительного завершения служебного процесса ядро продолжает работать, а задача запускается снова. Прототип `proto.exe` повторяет то, что потом сделает служебный процесс: скрытое окно, канал, ядро в своей группе процессов без окна, `CTRL_BREAK_EVENT` через `AttachConsole`, опознание оставшегося ядра по идентификатору процесса, времени создания и пути. Ядро прототипа ничего не перехватывает: один локальный вход и прямой выход, без адаптера и без Tailscale; системный VPN не меняется. Один запрос UAC — регистрация задачи; удаляет задачу сам прототип (он работает с правами администратора) в задаче 3. Проверка «при входе пользователя» выполняется в задаче 3 (выход из Windows и вход).

Если любая из проверок `E2`, `E3`, `E4`, `E6`, `E7`, `E8` даёт `FAIL` и повтор шага 6 это подтверждает — по спецификации работа останавливается и возвращается владельцу продукта: задача 4 не начинается. Перед остановкой удалите задачу (шаг 9).

**Files:**
- Create: `C:\code\router\spike\plan1\echeck\go.mod`, `C:\code\router\spike\plan1\echeck\checkctl\main_test.go`, `C:\code\router\spike\plan1\echeck\checkctl\main.go`, `C:\code\router\spike\plan1\echeck\proto\main.go`, `C:\code\router\spike\plan1\echeck\echecks.ps1`, `C:\code\router\spike\plan1\echeck\jcheck.ps1`
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Produces: `bin\proto.exe -dir <папка> -core <sing-box.exe>` (без окна консоли) — канал `\\.\pipe\SplitLane.Check`, команды-строки JSON `{"cmd": "status" | "stop-core" | "start-core" | "crash" | "quit" | "unregister"}`, журнал `run\events.log`; `bin\checkctl.exe xml -exe -dir -core -out`, `checkctl wait-pipe <секунды>` (`PIPE-READY` / `PIPE-TIMEOUT` / `PIPE-ACCESS-DENIED`), `checkctl send <команда>` (`ANSWER {...}`); `TaskXML(account, sid, command, arguments string) string` — образец XML задачи для команды `install` плана 1B.
- Consumes: ядро `C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64\sing-box.exe` (его SHA-256 проверяет задача 4; здесь ядро только проксирует локальный вход).

- [ ] **Step 1: Записать `go.mod` и тест XML задачи**

Файл `C:\code\router\spike\plan1\echeck\go.mod`, целиком:

```text
module echeck

go 1.26.0

require (
	github.com/Microsoft/go-winio v0.6.4
	golang.org/x/sys v0.49.0
)
```

Файл `C:\code\router\spike\plan1\echeck\checkctl\main_test.go`, целиком:

```go
package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTaskXML(t *testing.T) {
	text := TaskXML(`PC\Пользователь & Co`, "S-1-5-21-1-2-3-1001", `C:\code\router\spike\plan1\echeck\bin\proto.exe`, `-dir "C:\x" -core "C:\y\sing-box.exe"`)
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
	case task.Registration.URI != `\SplitLane Check`:
		t.Fatal(task.Registration.URI)
	case task.Registration.SD != "D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;S-1-5-21-1-2-3-1001)":
		t.Fatal(task.Registration.SD)
	case !task.Trigger.Enabled || task.Trigger.User != `PC\Пользователь & Co` || task.Principal.User != task.Trigger.User:
		t.Fatalf("%+v %+v", task.Trigger, task.Principal)
	case task.Principal.Logon != "InteractiveToken" || task.Principal.RunLevel != "HighestAvailable":
		t.Fatalf("%+v", task.Principal)
	case task.Settings.Multiple != "IgnoreNew" || task.Settings.NoBattery || task.Settings.StopBattery || !task.Settings.OnDemand || task.Settings.TimeLimit != "PT0S":
		t.Fatalf("%+v", task.Settings)
	case !strings.HasSuffix(task.Exec.Command, `proto.exe`) || task.Exec.Arguments != `-dir "C:\x" -core "C:\y\sing-box.exe"`:
		t.Fatalf("%+v", task.Exec)
	}
}

func TestUTF16File(t *testing.T) {
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
cd C:\code\router\spike\plan1\echeck; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./checkctl/
```

Ожидается: `FAIL echeck/checkctl [build failed]` и `undefined: TaskXML`.

- [ ] **Step 3: Записать `checkctl` и прототип**

Файл `C:\code\router\spike\plan1\echeck\checkctl\main.go`, целиком:

```go
// Command checkctl is the unelevated side of checks (е) and (ж): it writes the task XML for the
// current user and talks to the prototype's pipe.
//
//	checkctl xml -exe PROTO.EXE -dir DIR -core SING-BOX.EXE -out FILE
//	checkctl wait-pipe SECONDS
//	checkctl send status|stop-core|start-core|crash|quit|unregister
package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const pipeName = `\\.\pipe\SplitLane.Check`

// TaskName is the name of the throwaway task.
const TaskName = "SplitLane Check"

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// TaskXML is the task of spec 4.3: the setup user, highest privileges, logon trigger, start on demand,
// no time limit, no power conditions, IgnoreNew; the user may read and run it (FRFX), not change it.
func TaskXML(account, sid, command, arguments string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <URI>\` + esc(TaskName) + `</URI>
    <SecurityDescriptor>D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;` + esc(sid) + `)</SecurityDescriptor>
    <Description>SplitLane Plan 1 check (е), throwaway</Description>
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
      <Arguments>` + esc(arguments) + `</Arguments>
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

func send(command string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, pipeName)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			fmt.Println("PIPE-ACCESS-DENIED")
		} else {
			fmt.Println("PIPE-UNREACHABLE", err)
		}
		return 1
	}
	defer conn.Close()
	fmt.Fprintf(conn, "{\"cmd\":%q}\n", command)
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		fmt.Println("NO-ANSWER", err)
		return 1
	}
	fmt.Print("ANSWER ", line)
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: checkctl xml|wait-pipe|send ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "xml":
		fs := flag.NewFlagSet("xml", flag.ExitOnError)
		exe := fs.String("exe", "", "proto.exe")
		dir := fs.String("dir", "", "folder of the prototype")
		core := fs.String("core", "", "sing-box.exe")
		out := fs.String("out", "", "XML file to write")
		fs.Parse(os.Args[2:])
		u, err := user.Current()
		if err != nil || *exe == "" || *out == "" {
			fmt.Println("XML-FAILED", err)
			os.Exit(1)
		}
		args := fmt.Sprintf(`-dir "%s" -core "%s"`, *dir, *core)
		if err := os.WriteFile(*out, utf16File(TaskXML(u.Username, u.Uid, *exe, args)), 0o644); err != nil {
			fmt.Println("XML-FAILED", err)
			os.Exit(1)
		}
		fmt.Println("XML-OK")
	case "wait-pipe":
		seconds := 20
		if len(os.Args) > 2 {
			fmt.Sscan(os.Args[2], &seconds)
		}
		deadline := time.Now().Add(time.Duration(seconds) * time.Second)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			conn, err := winio.DialPipeContext(ctx, pipeName)
			cancel()
			if err == nil {
				conn.Close()
				fmt.Println("PIPE-READY")
				return
			}
			if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				fmt.Println("PIPE-ACCESS-DENIED")
				os.Exit(1)
			}
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Println("PIPE-TIMEOUT")
		os.Exit(1)
	case "send":
		if len(os.Args) != 3 {
			os.Exit(2)
		}
		os.Exit(send(os.Args[2]))
	default:
		os.Exit(2)
	}
}
```

Файл `C:\code\router\spike\plan1\echeck\proto\main.go`, целиком:

```go
// Command proto is the throwaway prototype of the helper for checks (е) and (ж) of spec 12:
// started by Task Scheduler, elevated, without a console; it serves a pipe with an explicit
// security descriptor, runs a harmless core (no adapter, no Tailscale) in its own process group,
// stops it with CTRL_BREAK, stops a core left by a killed predecessor, and logs the end of the
// Windows session. Everything it sees goes to events.log in its folder.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const pipeName = `\\.\pipe\SplitLane.Check`

var (
	dir, corePath string
	logMu         sync.Mutex
	childMu       sync.Mutex
	child         *exec.Cmd
	childDone     chan struct{}
)

// logf appends one line to events.log and flushes it to disk at once: the session may end any moment.
func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(filepath.Join(dir, "events.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s pid=%d %s\n", time.Now().Format("2006-01-02 15:04:05.000"), os.Getpid(), fmt.Sprintf(format, args...))
	f.Sync()
	f.Close()
}

// ---- console signal (the same technique as internal/singbox/console.go) ----

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole    = kernel32.NewProc("AttachConsole")
	procFreeConsole      = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	consoleMu            sync.Mutex
)

func attach(pid uint32) error {
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return err
	}
	return nil
}

func ctrlBreak(pid uint32) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	procFreeConsole.Call()
	if err := attach(pid); err != nil {
		return fmt.Errorf("attach console: %w", err)
	}
	defer procFreeConsole.Call()
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)
}

// consoleVisible reports whether the console of pid has a visible window.
func consoleVisible(pid uint32) (bool, error) {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	procFreeConsole.Call()
	if err := attach(pid); err != nil {
		return false, err
	}
	defer procFreeConsole.Call()
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return false, nil
	}
	v, _, _ := procIsWindowVisible.Call(hwnd)
	return v != 0, nil
}

func ownConsoleWindow() bool {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd != 0
}

// ---- the child core ----

type identity struct {
	PID     int    `json:"pid"`
	Created int64  `json:"created"`
	Path    string `json:"path"`
	WasOn   bool   `json:"wasOn"`
}

func processInfo(h windows.Handle) (int64, string, error) {
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return 0, "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return 0, "", err
	}
	return int64(c.HighDateTime)<<32 | int64(c.LowDateTime), windows.UTF16ToString(buf[:n]), nil
}

func saveState(id identity) {
	b, _ := json.Marshal(id)
	tmp := filepath.Join(dir, "proto-state.json.tmp")
	os.WriteFile(tmp, b, 0o644)
	os.Rename(tmp, filepath.Join(dir, "proto-state.json"))
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 17899
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startCore runs the core with a config that captures nothing: one local proxy inbound, a direct outbound.
func startCore() error {
	childMu.Lock()
	defer childMu.Unlock()
	cfg := map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true, "output": filepath.ToSlash(filepath.Join(dir, "core.log"))},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "in", "listen": "127.0.0.1", "listen_port": freePort()}},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	// the core reads this file: temporary file in the same folder, then rename (UTF-8 without BOM)
	cfgPath := filepath.Join(dir, "core-check.json")
	if err := os.WriteFile(cfgPath+".tmp", b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(cfgPath+".tmp", cfgPath); err != nil {
		return err
	}
	out, _ := os.OpenFile(filepath.Join(dir, "core-stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	cmd := exec.Command(corePath, "run", "-c", cfgPath, "-D", dir)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); out.Close(); close(done) }()
	child, childDone = cmd, done
	id := identity{PID: cmd.Process.Pid, WasOn: true}
	if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid)); err == nil {
		id.Created, id.Path, _ = processInfo(h)
		windows.CloseHandle(h)
	}
	saveState(id)
	logf("core started child_pid=%d", cmd.Process.Pid)
	return nil
}

func childAlive() (int, bool) {
	childMu.Lock()
	defer childMu.Unlock()
	if child == nil {
		return 0, false
	}
	select {
	case <-childDone:
		return child.Process.Pid, false
	default:
		return child.Process.Pid, true
	}
}

// stopCore: CTRL_BREAK, at most 5 s, then the forced end (spec 5.4 step 2).
func stopCore() map[string]any {
	pid, alive := childAlive()
	if !alive {
		return map[string]any{"error": "no core running"}
	}
	childMu.Lock()
	cmd, done := child, childDone
	childMu.Unlock()
	start := time.Now()
	res := map[string]any{"child_pid": pid}
	if err := ctrlBreak(uint32(pid)); err != nil {
		res["signal_error"] = err.Error()
		cmd.Process.Kill()
		<-done
		res["graceful"] = false
	} else {
		select {
		case <-done:
			res["graceful"] = true
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
			<-done
			res["graceful"] = false
		}
	}
	res["ms"] = time.Since(start).Milliseconds()
	res["exit_code"] = cmd.ProcessState.ExitCode()
	logf("stop core: %v", res)
	return res
}

// leftover stops the core a killed predecessor left behind, found by pid, creation time and path (5.7).
func leftover() {
	b, err := os.ReadFile(filepath.Join(dir, "proto-state.json"))
	if err != nil {
		logf("leftover: no proto-state.json")
		return
	}
	var id identity
	json.Unmarshal(b, &id)
	logf("leftover: previous state wasOn=%v child_pid=%d", id.WasOn, id.PID)
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		logf("leftover: none running")
		return
	}
	defer windows.CloseHandle(h)
	created, path, err := processInfo(h)
	if err != nil || created != id.Created || !strings.EqualFold(path, id.Path) {
		logf("leftover: pid %d is another process, not touched", id.PID)
		return
	}
	start := time.Now()
	graceful := false
	if err := ctrlBreak(uint32(id.PID)); err != nil {
		logf("leftover: signal failed: %v", err)
	} else if ev, _ := windows.WaitForSingleObject(h, 5000); ev == windows.WAIT_OBJECT_0 {
		graceful = true
	}
	if !graceful {
		windows.TerminateProcess(h, 1)
		windows.WaitForSingleObject(h, 5000)
	}
	var code uint32
	windows.GetExitCodeProcess(h, &code)
	logf("leftover: core stopped graceful=%v in %dms exit_code=%d", graceful, time.Since(start).Milliseconds(), code)
}

// ---- the end of the session (check (ж)) ----

const (
	wmDestroy         = 0x0002
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)

var (
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background windows.Handle
	MenuName, ClassName                *uint16
	IconSm                             windows.Handle
}

type msg struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             [2]int32
	LPrivate       uint32
}

// programFiles is the 64-bit Program Files folder named by HKLM, never %ProgramFiles%: the
// prototype runs elevated with the user's environment (spec 5.6, 11).
func programFiles() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("ProgramFilesDir"); err == nil && filepath.IsAbs(v) && !strings.Contains(v, "%") {
			return v
		}
	}
	return `C:\Program Files`
}

// endSession is the shortened order of 5.4: "was on" cleared, one restore command (here a
// read-only `tailscale status --json` of the same cost), the core ended without waiting.
func endSession() {
	start := time.Now()
	logf("endsession begin")
	saveState(identity{WasOn: false})
	logf("endsession wasOn cleared after %dms", time.Since(start).Milliseconds())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t0 := time.Now()
	ts := filepath.Join(programFiles(), "Tailscale", "tailscale.exe")
	cmd := exec.CommandContext(ctx, ts, "status", "--json")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	err := cmd.Run()
	cancel()
	logf("endsession stand-in restore command took %dms err=%v", time.Since(t0).Milliseconds(), err)
	pid, alive := childAlive()
	logf("endsession core alive before kill: %v (child_pid=%d)", alive, pid)
	if alive {
		childMu.Lock()
		child.Process.Kill()
		childMu.Unlock()
	}
	logf("endsession done elapsed=%dms", time.Since(start).Milliseconds())
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmQueryEndSession:
		logf("queryendsession lParam=%#x", lParam)
		return 1
	case wmEndSession:
		logf("endsession message ending=%v lParam=%#x", wParam != 0, lParam)
		if wParam != 0 {
			endSession()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

func sessionWindow() {
	runtime.LockOSThread()
	var instance windows.Handle
	windows.GetModuleHandleEx(0, nil, &instance)
	name := windows.StringToUTF16Ptr("SplitLaneCheckSession")
	wc := wndClassEx{WndProc: syscall.NewCallback(wndProc), Instance: instance, ClassName: name}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logf("session window: register class: %v", err)
		return
	}
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		logf("session window: create: %v", err)
		return
	}
	logf("session window ready")
	var m msg
	for {
		if r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// ---- the pipe ----

func status() map[string]any {
	tok := windows.GetCurrentProcessToken()
	var sid uint32
	windows.ProcessIdToSessionId(uint32(os.Getpid()), &sid)
	pid, alive := childAlive()
	res := map[string]any{"pid": os.Getpid(), "elevated": tok.IsElevated(), "session": sid,
		"own_console_window": ownConsoleWindow(), "child_pid": pid, "child_alive": alive}
	if alive {
		visible, err := consoleVisible(uint32(pid))
		res["child_console_visible"] = visible
		if err != nil {
			res["child_console_error"] = err.Error()
		}
	}
	return res
}

func serve(conn net.Conn, quit chan<- int) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var req struct {
			Cmd string `json:"cmd"`
		}
		json.Unmarshal(sc.Bytes(), &req)
		logf("pipe command %q", req.Cmd)
		var res any
		switch req.Cmd {
		case "status":
			res = status()
		case "stop-core":
			res = stopCore()
		case "start-core":
			if err := startCore(); err != nil {
				res = map[string]any{"error": err.Error()}
			} else {
				res = status()
			}
		case "crash":
			conn.Write([]byte(`{"ok":"terminating without cleanup"}` + "\n"))
			logf("crash: TerminateProcess(self)")
			windows.TerminateProcess(windows.CurrentProcess(), 9)
		case "quit":
			res = stopCore()
			defer func() { quit <- 0 }()
		case "unregister":
			cmd := exec.Command("schtasks.exe", "/delete", "/tn", "SplitLane Check", "/f")
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
			err := cmd.Run()
			res = map[string]any{"unregistered": err == nil, "error": fmt.Sprint(err), "core": stopCore()}
			defer func() { quit <- 0 }()
		default:
			res = map[string]any{"error": "unknown command"}
		}
		b, _ := json.Marshal(res)
		conn.Write(append(b, '\n'))
	}
}

func main() {
	flag.StringVar(&dir, "dir", "", "folder for events.log and the core files")
	flag.StringVar(&corePath, "core", "", "sing-box.exe")
	flag.Parse()
	if dir == "" || corePath == "" {
		os.Exit(2)
	}
	os.MkdirAll(dir, 0o755)
	tok := windows.GetCurrentProcessToken()
	user, _ := tok.GetTokenUser()
	var session uint32
	windows.ProcessIdToSessionId(uint32(os.Getpid()), &session)
	logf("start elevated=%v session=%d own_console_window=%v parent=%d", tok.IsElevated(), session, ownConsoleWindow(), os.Getppid())
	interrupts := make(chan os.Signal, 1)
	go func() {
		for range interrupts {
			logf("console interrupt reached the prototype (must not happen)")
		}
	}()
	signal.Notify(interrupts, os.Interrupt)
	l, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + user.User.Sid.String() + ")",
	})
	if err != nil {
		logf("pipe not created: %v", err)
		os.Exit(3)
	}
	leftover()
	if err := startCore(); err != nil {
		logf("core not started: %v", err)
	}
	go sessionWindow()
	quit := make(chan int, 1)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					logf("accept: %v", err)
				}
				return
			}
			go serve(conn, quit)
		}
	}()
	code := <-quit
	l.Close()
	logf("exit %d", code)
	os.Exit(code)
}
```

- [ ] **Step 4: Собрать и проверить**

```powershell
cd C:\code\router\spike\plan1\echeck; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go mod tidy; go vet ./...; go test ./...; go build -ldflags "-H=windowsgui" -o bin\proto.exe .\proto; go build -o bin\checkctl.exe .\checkctl; Get-ChildItem bin | Select-Object Name
```

Ожидается: `ok  echeck/checkctl`, `?  echeck/proto [no test files]`, два файла `checkctl.exe` и `proto.exe`; `go vet` молчит.

- [ ] **Step 5: Записать скрипты проверок и XML задачи**

Файл `C:\code\router\spike\plan1\echeck\echecks.ps1`, целиком:

```powershell
# SplitLane Plan 1, check (e): the task of spec 4.3 started from an unelevated process.
# ASCII only. Runs unelevated, after the task "SplitLane Check" was registered (one UAC, task 2).
# Prints one line per check: E<n> PASS|FAIL|INFO <text>; the last line is ECHECKS-DONE.
$ErrorActionPreference = 'Continue'
$E    = 'C:\code\router\spike\plan1\echeck'
$Ctl  = "$E\bin\checkctl.exe"
$Log  = "$E\run\events.log"
$Task = 'SplitLane Check'

function Say([string]$id, [bool]$ok, [string]$text) {
    $word = if ($ok) { 'PASS' } else { 'FAIL' }
    Write-Host "$id $word $text"
}

function Answer([string]$cmd) {
    # checkctl prints "ANSWER {json}"; returns the object or $null
    $line = & $Ctl send $cmd | Select-Object -Last 1
    if ($line -notlike 'ANSWER *') { Write-Host "   checkctl: $line"; return $null }
    $line.Substring(7) | ConvertFrom-Json
}

function Count-Proto { @(Get-Process -Name 'proto' -ErrorAction SilentlyContinue).Count }

# E1: the registered task has the settings of spec 4.3
$xml = $null
try { $xml = [xml]((schtasks /query /tn $Task /xml) -join "`n") } catch { $xml = $null }
if ($xml) {
    $s = $xml.Task.Settings; $p = $xml.Task.Principals.Principal
    # An element left out of the read-back has its schema default (IgnoreNew and on-demand start are defaults).
    function Or([string]$value, [string]$default) { if ($value) { $value } else { $default } }
    $instances = Or $s.MultipleInstancesPolicy 'IgnoreNew'
    $onDemand = Or $s.AllowStartOnDemand 'true'
    $logonType = Or $p.LogonType 'InteractiveToken'
    $ok = ($p.RunLevel -eq 'HighestAvailable') -and ($logonType -eq 'InteractiveToken') -and
          ($instances -eq 'IgnoreNew') -and ($s.ExecutionTimeLimit -eq 'PT0S') -and
          ($s.DisallowStartIfOnBatteries -eq 'false') -and ($s.StopIfGoingOnBatteries -eq 'false') -and
          ($onDemand -eq 'true') -and ($null -ne $xml.Task.Triggers.LogonTrigger)
    Say 'E1' $ok "task read back: RunLevel=$($p.RunLevel) LogonType=$logonType Instances=$instances OnDemand=$onDemand TimeLimit=$($s.ExecutionTimeLimit) Batteries=$($s.DisallowStartIfOnBatteries)/$($s.StopIfGoingOnBatteries) LogonTrigger=$($null -ne $xml.Task.Triggers.LogonTrigger)"
} else {
    Say 'E1' $false 'schtasks /query did not return the task XML'
}

# E2, E4: schtasks /run from this unelevated process, then the pipe answers
if ((Count-Proto) -gt 0) { Write-Host 'ECHECKS-REFUSED a proto.exe is already running'; return }
schtasks /run /tn $Task | Out-Null
$runCode = $LASTEXITCODE
$wait = & $Ctl wait-pipe 30 | Select-Object -Last 1
$st = Answer 'status'
Say 'E2' (($runCode -eq 0) -and $st -and $st.elevated) "schtasks /run exit=$runCode, prototype elevated=$($st.elevated), no UAC prompt was needed"
Say 'E4' (($wait -eq 'PIPE-READY') -and $null -ne $st) "pipe with an explicit security descriptor from an unelevated process: $wait"

# E3: no console window, neither the prototype's nor the core's. A second after the start: right
# after it the core's console may not exist yet, and the check of it fails instead of looking.
Start-Sleep -Seconds 1
$st = Answer 'status'
$procs = @(Get-Process -Name 'proto', 'sing-box' -ErrorAction SilentlyContinue)
$windows = @($procs | Where-Object { $_.MainWindowHandle -ne [IntPtr]::Zero }).Count
Say 'E3' ($st -and -not $st.own_console_window -and $st.child_alive -and -not $st.child_console_visible -and -not $st.child_console_error -and $windows -eq 0) "own_console_window=$($st.own_console_window) child_alive=$($st.child_alive) child_console_visible=$($st.child_console_visible) child_console_error=$($st.child_console_error) visible windows=$windows"

# E5: a second start while it runs is ignored (IgnoreNew)
schtasks /run /tn $Task | Out-Null
Start-Sleep -Seconds 3
Say 'E5' ((Count-Proto) -eq 1) "proto.exe processes after a second schtasks /run: $(Count-Proto)"

# E6: the core stops on the graceful signal (CTRL_BREAK through its hidden console)
$stop = Answer 'stop-core'
Say 'E6' ($stop -and $stop.graceful -and $stop.ms -lt 5000 -and $stop.exit_code -eq 0) "graceful=$($stop.graceful) ms=$($stop.ms) exit_code=$($stop.exit_code) signal_error=$($stop.signal_error)"
$null = Answer 'start-core'

# E7: after a forced end of the prototype the core keeps running
$corePid = (Answer 'status').child_pid
$null = & $Ctl send crash
Start-Sleep -Seconds 2
$coreAlive = [bool](Get-Process -Id $corePid -ErrorAction SilentlyContinue)
Say 'E7' (((Count-Proto) -eq 0) -and $coreAlive) "prototype gone=$((Count-Proto) -eq 0), core still running=$coreAlive"

# E8: the task starts again; the new prototype stops the core left behind
$before = @(Get-Content -LiteralPath $Log).Count
schtasks /run /tn $Task | Out-Null
$wait = & $Ctl wait-pipe 30 | Select-Object -Last 1
Start-Sleep -Seconds 6
$new = @(Get-Content -LiteralPath $Log | Select-Object -Skip $before)
$left = @($new | Where-Object { $_ -like '*leftover: core stopped*' }) | Select-Object -First 1
$oldGone = -not (Get-Process -Id $corePid -ErrorAction SilentlyContinue)
Say 'E8' (($wait -eq 'PIPE-READY') -and $oldGone -and ($left -like '*graceful=true*')) "restart: $wait; old core gone=$oldGone; log: $left"
Write-Host 'ECHECKS-DONE (the prototype keeps running for task 3)'
```

Файл `C:\code\router\spike\plan1\echeck\jcheck.ps1`, целиком:

```powershell
# SplitLane Plan 1, check (zh): what the prototype logged around the logoff, read after the next logon.
# ASCII only. Usage: jcheck.ps1 -Skip <number of events.log lines before the logoff>
param([int]$Skip = 0)
$Log = 'C:\code\router\spike\plan1\echeck\run\events.log'
$lines = @(Get-Content -LiteralPath $Log | Select-Object -Skip $Skip)

function Say([string]$id, $ok, [string]$text) {
    $word = if ($null -eq $ok) { 'INFO' } elseif ($ok) { 'PASS' } else { 'FAIL' }
    Write-Host "$id $word $text"
}

function First([string]$pattern) { @($lines | Where-Object { $_ -like $pattern }) | Select-Object -First 1 }

$query = First '*queryendsession*'
$begin = First '*endsession begin*'
$cmd   = First '*stand-in restore command took*'
$alive = First '*core alive before kill*'
$done  = First '*endsession done elapsed=*'
$ms = -1
if ($done -match 'elapsed=(\d+)ms') { $ms = [int]$Matches[1] }
Say 'J1' ($null -ne $query) "WM_QUERYENDSESSION reached the hidden window: $query"
Say 'J2' (($null -ne $begin) -and $ms -ge 0 -and $ms -lt 5000) "shortened order of 5.4 finished in ${ms} ms; $cmd"
Say 'J3' $null "did the core end on its own before the kill: $alive"
$starts = @($lines | Where-Object { $_ -like '*start elevated=*' })
$logon = $starts | Select-Object -Last 1
Say 'J4' (($starts.Count -ge 1) -and ($logon -like '*elevated=true*')) "start after the logon (logon trigger, check (e)): $logon"
$left = @($lines | Where-Object { $_ -like '*leftover:*' }) | Select-Object -Last 1
Say 'J5' $null "the instance after the logon about the old core: $left"
Write-Host 'JCHECK-DONE'
```

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; schtasks /query /tn "SplitLane Check" 2>$null | Out-Null; "task exists before: $($LASTEXITCODE -eq 0)"; & "$E\bin\checkctl.exe" xml -exe "$E\bin\proto.exe" -dir "$E\run" -core 'C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64\sing-box.exe' -out "$E\task.xml"
```

Ожидается: `task exists before: False` и `XML-OK`. Если задача уже есть — это след прошлого запуска: удалить её можно только с правами администратора (шаг 9 работает, пока запущен прототип; иначе спросить пользователя). Файл `task.xml` содержит имя учётной записи и SID и остаётся в игнорируемой папке.

- [ ] **Step 6: Зарегистрировать задачу (нужен пользователь: UAC)**

Предупредите пользователя: сейчас появится запрос UAC от «Планировщик заданий» (schtasks.exe); отмена ничего не меняет. Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
try { $p = Start-Process -FilePath schtasks.exe -ArgumentList '/create', '/tn', '"SplitLane Check"', '/xml', '"C:\code\router\spike\plan1\echeck\task.xml"' -Verb RunAs -Wait -PassThru -WindowStyle Hidden -ErrorAction Stop; "REGISTER exit=$($p.ExitCode)" } catch { 'UAC-CANCELLED - nothing was registered. Do not retry without the user.' }
```

Ожидается `REGISTER exit=0`. Ненулевой код — XML отвергнут; текст ошибки виден, если выполнить ту же команду `schtasks /create ...` в окне PowerShell администратора (попросить пользователя). Тогда (е) не проверена; решение — у владельца продукта.

- [ ] **Step 7: Проверки E1–E8 (без UAC)**

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\spike\plan1\echeck\echecks.ps1
```

Ожидается (числа будут другими):

```text
E1 PASS task read back: RunLevel=HighestAvailable LogonType=InteractiveToken Instances=IgnoreNew OnDemand=true TimeLimit=PT0S Batteries=false/false LogonTrigger=True
E2 PASS schtasks /run exit=0, prototype elevated=True, no UAC prompt was needed
E4 PASS pipe with an explicit security descriptor from an unelevated process: PIPE-READY
E3 PASS own_console_window=False child_alive=True child_console_visible=False child_console_error= visible windows=0
E5 PASS proto.exe processes after a second schtasks /run: 1
E6 PASS graceful=True ms=420 exit_code=0 signal_error=
E7 PASS prototype gone=True, core still running=True
E8 PASS restart: PIPE-READY; old core gone=True; log: 2026-11-14 12:00:00.000 pid=4321 leftover: core stopped graceful=true in 380ms exit_code=0
ECHECKS-DONE (the prototype keeps running for task 3)
```

| Запись | Что подтверждает |
|---|---|
| `E1` | задача зарегистрирована с настройками 4.3 (элемент, которого нет в выгрузке, имеет значение по умолчанию схемы) |
| `E2` | `schtasks /run` из процесса без прав администратора запускает прототип с повышенными правами, без UAC |
| `E3` | нет окна консоли ни у прототипа, ни у ядра (через секунду после запуска; ошибка проверки консоли ядра в `child_console_error` — тоже `FAIL`: значит, прототип её не увидел) |
| `E4` | канал с явным описателем безопасности открывается процессом без прав администратора |
| `E5` | повторный запуск игнорируется (`IgnoreNew`) |
| `E6` | ядро останавливается штатным сигналом из процесса без консоли, код выхода 0, быстрее 5 секунд |
| `E7` | после принудительного завершения прототипа ядро продолжает работать |
| `E8` | задача запускается снова, новый прототип останавливает оставшееся ядро штатно |

Если какая-то запись `FAIL` — один раз повторите шаг после уборки. Сначала без прав администратора: работающий прототип останавливает своё ядро и выходит.

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; if (Get-Process -Name proto -ErrorAction SilentlyContinue) { & "$E\bin\checkctl.exe" send quit; Start-Sleep -Seconds 3 }; "left: [$(@(Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается `left: []`. Остался `sing-box.exe` (например, ядро, оставленное `E7`, при провале `E8`: новый прототип уже переписал `proto-state.json` и это ядро больше не найдёт) или зависший `proto.exe` — оба работают с правами администратора, и процесс исполнителя их не завершит. Тогда один вызов с уборкой (нужен пользователь: UAC; предупредить пользователя; задача остаётся для повтора). `taskkill /im sing-box.exe` завершает любое ядро на ПК, поэтому команда сначала убеждается, что сеанса watchdog плана 0 и сторожа плана 1 нет (их `status.json` в конечном состоянии), иначе печатает `CLEANUP-REFUSED` и ничего не делает:

```powershell
. C:\code\router\spike\plan0\lib.ps1; $s0 = Read-P0Status; $g = Read-P0Json -Path C:\code\router\spike\plan1\guard\status.json; $live = @(); if ($s0 -and ($P0Terminal -notcontains $s0.state)) { $live += "plan 0 watchdog state=$($s0.state)" }; if ($g -and (@('refused', 'finished', 'tripped') -notcontains $g.state)) { $live += "plan 1 guard state=$($g.state)" }; if (Get-Process -Name splitlane-helper, slguard -ErrorAction SilentlyContinue) { $live += 'a helper or guard process runs' }; if ($live.Count -gt 0) { "CLEANUP-REFUSED: $($live -join '; ')"; return }; $c = 'taskkill /f /im proto.exe; taskkill /f /im sing-box.exe'; $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($c)); try { Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-EncodedCommand', $enc -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop } catch { 'UAC-CANCELLED - nothing was changed. Do not retry without the user.'; return }; Start-Sleep -Seconds 2; "left: [$(@(Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается `left: []`; затем повтор шага 7. Команда передаётся через `-EncodedCommand`: так кавычки внутри неё доходят до повышенного процесса без изменений. `CLEANUP-REFUSED` — живой сеанс плана 0 или плана 1 закончить по его правилам, затем повторить уборку.

`E5 FAIL` — `IgnoreNew` не действует: второй запуск задачи стартует второй экземпляр. Служебный процесс от этого защищён: канал создаёт первый экземпляр, второй выходит с кодом 3 (задача 19). Записать в приложение A, сообщить владельцу продукта; работу это не останавливает. `FAIL E6` с `signal_error` — `CTRL_BREAK_EVENT` из процесса без консоли не работает: способ остановки из 5.4 не подтверждён, вопрос владельцу продукта (запасной способ в спецификации не описан). `E1 FAIL`, а остальные `PASS` — расхождение в выгрузке XML, записывается в отчёт и работу не останавливает. `E2 FAIL` с `schtasks /run exit=1` (отказ в доступе) — описатель безопасности из `RegistrationInfo` не применился: процесс без прав администратора не может запустить задачу. Это провал (е) в том виде, как задача регистрируется сейчас; сообщите владельцу продукта и предложите проверить регистрацию через COM-интерфейс планировщика (`RegisterTaskDefinition` с явным SDDL) отдельным шагом.

- [ ] **Step 8: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1» приложения A добавьте строки (слово «подтверждено» или «НЕ подтверждено» — по записям):

```text
- (е) Задача планировщика из XML (пользователь настройки, наивысшие права, запуск по требованию, триггер при входе, без ограничения времени и условий питания, IgnoreNew): регистрация, настройки при чтении назад: <подтверждено>.
- (е) Запуск задачи из процесса без прав администратора без UAC, процесс с повышенными правами: <подтверждено>.
- (е) Нет окна консоли у служебного процесса и у ядра: <подтверждено>.
- (е) Канал с явным описателем безопасности доступен процессу без прав администратора: <подтверждено>.
- (е) Ядро останавливается штатным сигналом CTRL_BREAK_EVENT через его скрытую консоль из процесса без консоли: <подтверждено (мс: N)>.
- (е) После принудительного завершения служебного процесса ядро работает, задача запускается снова, новый экземпляр останавливает оставшееся ядро: <подтверждено>.
- (е) Повторный запуск задачи, пока она работает, игнорируется (IgnoreNew): <подтверждено | НЕ подтверждено — второй экземпляр защищён каналом первого>.
```

Проверка выполнена, и её итог (подтверждено или НЕ подтверждено) записан строками выше, поэтому в предложении «Не проверялось: …» в конце приложения A уберите слова «запуск через планировщик задач» и допишите туда же «(е) запуск задачи при входе пользователя (проверяется в задаче 3, запись `J4`)»: одна и та же проверка не может быть и подтверждена, и не проверена.

```powershell
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record check (e), Task Scheduler launch, in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 9: Только при остановке работы: удалить задачу и процессы прототипа**

Если по итогам шага 7 работа останавливается, задачу удаляют сейчас: задача 3 при остановке не выполняется, а задача с триггером при входе запускала бы прототип и его ядро с правами администратора при каждом входе. Сначала без UAC: задачу удаляет сам прототип (он работает с правами администратора); если его нет, его запускают задачей.

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; if (-not (Get-Process -Name proto -ErrorAction SilentlyContinue)) { schtasks /run /tn "SplitLane Check" 2>$null | Out-Null; & "$E\bin\checkctl.exe" wait-pipe 30 }; & "$E\bin\checkctl.exe" send unregister; Start-Sleep -Seconds 2; schtasks /query /tn "SplitLane Check" 2>$null | Out-Null; "task exists after: $($LASTEXITCODE -eq 0)"; "left: [$(@(Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается `ANSWER {"core":{...},"error":"\u003cnil\u003e","unregistered":true}`, `task exists after: False` и `left: []` (перед ними `PIPE-READY`, если прототип пришлось запустить). Если `task exists after: True` (канал недоступен — например, провал `E4` — или прототип не запускается) или в `left:` остался процесс — один вызов с уборкой (нужен пользователь: UAC; предупредить пользователя). Как и в шаге 7, команда сначала убеждается, что сеанса watchdog плана 0 и сторожа плана 1 нет:

```powershell
. C:\code\router\spike\plan0\lib.ps1; $s0 = Read-P0Status; $g = Read-P0Json -Path C:\code\router\spike\plan1\guard\status.json; $live = @(); if ($s0 -and ($P0Terminal -notcontains $s0.state)) { $live += "plan 0 watchdog state=$($s0.state)" }; if ($g -and (@('refused', 'finished', 'tripped') -notcontains $g.state)) { $live += "plan 1 guard state=$($g.state)" }; if (Get-Process -Name splitlane-helper, slguard -ErrorAction SilentlyContinue) { $live += 'a helper or guard process runs' }; if ($live.Count -gt 0) { "CLEANUP-REFUSED: $($live -join '; ')"; return }; $c = 'schtasks /delete /tn "SplitLane Check" /f; taskkill /f /im proto.exe; taskkill /f /im sing-box.exe'; $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($c)); try { Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-EncodedCommand', $enc -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop } catch { 'UAC-CANCELLED - nothing was changed. Do not retry without the user.'; return }; Start-Sleep -Seconds 2; schtasks /query /tn "SplitLane Check" 2>$null | Out-Null; "task exists after: $($LASTEXITCODE -eq 0)"; "left: [$(@(Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается `task exists after: False` и `left: []`. Иначе сообщить пользователю: задачу `SplitLane Check` удаляют в «Планировщике заданий» (Библиотека планировщика), процессы `proto.exe` и `sing-box.exe` — в Диспетчере задач (с подтверждением администратора). Затем работа возвращается владельцу продукта.

---

### Task 3: (ж) Завершение сеанса Windows — нужен пользователь

Проверка (ж): уведомление о завершении сеанса в процессе Go со скрытым окном (`WM_QUERYENDSESSION`, `WM_ENDSESSION`) и сокращённый порядок 5.4 (снять «было включено», один раз отдать команду возврата, завершить ядро без ожидания). Вместо команды `tailscale set` прототип выполняет `tailscale status --json` той же стоимости: система не меняется (путь `tailscale.exe` прототип берёт из реестра, `ProgramFilesDir` в `HKLM`, а не из `%ProgramFiles%`: он работает с правами администратора и окружением пользователя). Выход из Windows завершает и сеанс исполнителя, поэтому итог записывается в `events.log` и читается после нового входа. Попутно проверяется последняя часть (е): запуск задачи по триггеру при входе пользователя.

Провал (ж) или пропуск задачи работу плана 1A не останавливает. Тогда в отчёте и в приложении A прямо сказано: путь завершения сеанса из 5.4 (метод `SessionEnding` задачи 12 и окно задачи 19) в план 1A входит непроверенным на ПК, а запуск задачи при входе не проверялся. Непроверенный запуск при входе — непроверенная часть (е), провал которой по разделу 12 останавливает работу: он блокирует команду `install` плана 1B, и план 1B начинает её с повтора этой проверки (раздел «Что остаётся плану 1B», пункт 4). `J4 FAIL` — провал (е): работа останавливается сразу.

**Files:**
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Consumes: задача `SplitLane Check`, `proto.exe`, `checkctl.exe`, `jcheck.ps1` из задачи 2.

- [ ] **Step 1: Подготовить прототип и отметку в журнале**

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; if (-not (Get-Process -Name proto -ErrorAction SilentlyContinue)) { schtasks /run /tn "SplitLane Check" | Out-Null }; & "$E\bin\checkctl.exe" wait-pipe 30; $n = @(Get-Content "$E\run\events.log").Count; [IO.File]::WriteAllText("$E\before-logoff.txt", [string]$n, (New-Object Text.ASCIIEncoding)); "LINES-BEFORE $n"
```

Ожидается `PIPE-READY` и `LINES-BEFORE <число>`.

- [ ] **Step 2: Выход из Windows и вход (нужен пользователь)**

Передайте пользователю и дождитесь его возвращения:

```text
Проверка завершения сеанса. Сохраните открытые документы. Затем: Пуск → значок пользователя → «Выйти». Мой сеанс при этом закончится — это ожидаемо. После выхода снова войдите в Windows, подождите минуту, откройте терминал в C:\code\router, запустите `claude --continue` и напишите «продолжай задачу 3, шаг 3». Если не хотите выходить сейчас, напишите «пропусти (ж)».
```

Если пользователь отказался — перейдите к шагу 4 и в шаге 5 запишите «не проверялось», а для запуска при входе — «(е) запуск при входе: не проверялось — блокирует команду install плана 1B». Сообщите пользователю, что эту часть (е) план 1B выполнит первой, до команды `install`.

- [ ] **Step 3: Прочитать итог после входа**

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; $n = [int](Get-Content "$E\before-logoff.txt"); powershell -NoProfile -ExecutionPolicy Bypass -File "$E\jcheck.ps1" -Skip $n; Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | Select-Object Name, Id
```

Ожидается (числа другие):

```text
J1 PASS WM_QUERYENDSESSION reached the hidden window: 2026-11-14 12:10:00.100 pid=4321 queryendsession lParam=0x80000000
J2 PASS shortened order of 5.4 finished in 260 ms; 2026-11-14 12:10:00.350 pid=4321 endsession stand-in restore command took 230ms err=<nil>
J3 INFO did the core end on its own before the kill: 2026-11-14 12:10:00.352 pid=4321 endsession core alive before kill: true (child_pid=5555)
J4 PASS start after the logon (logon trigger, check (e)): 2026-11-14 12:11:05.000 pid=6789 start elevated=true session=1 own_console_window=false parent=1234
J5 INFO the instance after the logon about the old core: 2026-11-14 12:11:05.010 pid=6789 leftover: none running
JCHECK-DONE
```

`J1` и `J2` — проверка (ж); `J4` — запуск задачи при входе пользователя (последняя часть (е)); `J3` и `J5` — наблюдения: завершилось ли ядро само раньше (спецификация допускает это) и что увидел новый экземпляр. `J2 FAIL` с `elapsed` больше 5000 мс — сокращённый порядок не укладывается в отведённое время; `J1 FAIL` — окно не получило уведомление: способ из 5.4 не работает, вопрос владельцу продукта (журнал возврата 5.7 при следующем старте остаётся единственной защитой). `J4 FAIL` — задача не стартовала при входе: провал (е), по спецификации работа останавливается и возвращается владельцу.

- [ ] **Step 4: Удалить задачу**

```powershell
$E = 'C:\code\router\spike\plan1\echeck'; if (-not (Get-Process -Name proto -ErrorAction SilentlyContinue)) { schtasks /run /tn "SplitLane Check" | Out-Null; & "$E\bin\checkctl.exe" wait-pipe 30 }; & "$E\bin\checkctl.exe" send unregister; Start-Sleep -Seconds 2; schtasks /query /tn "SplitLane Check" 2>$null | Out-Null; "task exists after: $($LASTEXITCODE -eq 0)"; Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | Select-Object Name, Id
```

Ожидается `ANSWER {... "unregistered":true}`, `task exists after: False` и ни одного процесса `proto` или `sing-box`. Если `task exists after: True` или остался процесс `proto` или `sing-box` — задача с триггером при входе запускала бы прототип и его ядро с правами администратора при каждом входе: один вызов с уборкой (нужен пользователь: UAC; предупредить пользователя), та же команда, что в шаге 9 задачи 2, — она сначала убеждается, что сеанса watchdog плана 0 и сторожа плана 1 нет:

```powershell
. C:\code\router\spike\plan0\lib.ps1; $s0 = Read-P0Status; $g = Read-P0Json -Path C:\code\router\spike\plan1\guard\status.json; $live = @(); if ($s0 -and ($P0Terminal -notcontains $s0.state)) { $live += "plan 0 watchdog state=$($s0.state)" }; if ($g -and (@('refused', 'finished', 'tripped') -notcontains $g.state)) { $live += "plan 1 guard state=$($g.state)" }; if (Get-Process -Name splitlane-helper, slguard -ErrorAction SilentlyContinue) { $live += 'a helper or guard process runs' }; if ($live.Count -gt 0) { "CLEANUP-REFUSED: $($live -join '; ')"; return }; $c = 'schtasks /delete /tn "SplitLane Check" /f; taskkill /f /im proto.exe; taskkill /f /im sing-box.exe'; $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($c)); try { Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-EncodedCommand', $enc -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop } catch { 'UAC-CANCELLED - nothing was changed. Do not retry without the user.'; return }; Start-Sleep -Seconds 2; schtasks /query /tn "SplitLane Check" 2>$null | Out-Null; "task exists after: $($LASTEXITCODE -eq 0)"; "left: [$(@(Get-Process -Name proto, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')]"
```

Ожидается `task exists after: False` и `left: []`. Иначе сообщить пользователю: задачу `SplitLane Check` удаляют в «Планировщике заданий» (Библиотека планировщика), процессы `proto.exe` и `sing-box.exe` — в Диспетчере задач (с подтверждением администратора); задача 4 не начинается, пока задачи нет.

- [ ] **Step 5: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1» добавьте:

```text
- (ж) Уведомление о завершении сеанса (выход из Windows) в процессе Go со скрытым окном, WM_QUERYENDSESSION и WM_ENDSESSION: <подтверждено | НЕ подтверждено | не проверялось>.
- (ж) Сокращённый порядок 5.4 при выходе из Windows (признак, одна команда того же веса, что `tailscale set`, завершение ядра) уложился в отведённое время: <подтверждено (мс: N) | НЕ подтверждено | не проверялось>; ядро к этому моменту <ещё работало | уже завершилось само>.
- (е) Запуск задачи планировщика при входе пользователя: <подтверждено | НЕ подтверждено | не проверялось — блокирует команду install плана 1B>.
```

Выход и вход выполнены, итог записан строками выше, поэтому из предложения «Не проверялось: …» в конце приложения A уберите слова «завершение сеанса Windows» и слова «(е) запуск задачи при входе пользователя (проверяется в задаче 3, запись `J4`)», дописанные шагом 8 задачи 2.

При пропуске задачи вместо этих строк слова «завершение сеанса Windows» в предложении «Не проверялось» остаются (не дописывайте их второй раз); к ним добавьте только «(путь 5.4 в служебном процессе плана 1A на ПК не проверен)», а к словам о запуске при входе — «— блокирует команду install плана 1B».

```powershell
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record check (zh), Windows session end, in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Репозиторий: лицензия, описание, получение ядра, модуль Go и атомарная запись

Задача создаёт то, что раздел 4.1 требует от корня репозитория, скрипт получения ядра с проверкой SHA-256 и модуль `splitlane/helper` с первым пакетом — атомарной записью файлов. Репозиторий git уже существует (план 0); ветка создана в задаче 1. Перед началом: задачи 1–3 выполнены, и ни одна из них не остановила работу (таблица в разделе «Как читать этот план»).

**Files:**
- Create: `LICENSE` (скачивается), `README.md`, `THIRD-PARTY.md`, `scripts/fetch-core.ps1`, `scripts/core.sha256`, `helper/go.mod`, `helper/go.sum` (создаёт `go mod tidy`), `helper/internal/atomicfile/atomicfile.go`, `helper/internal/atomicfile/atomicfile_test.go`
- Modify: `.gitignore` (новый текст целиком)

**Interfaces:**
- Produces: `atomicfile.Write(path string, data []byte) error` — UTF-8 без BOM (данные с BOM отвергаются), временный файл в той же папке, `Sync`, `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)` с повтором при `ERROR_ACCESS_DENIED`, `ERROR_SHARING_VIOLATION`, `ERROR_LOCK_VIOLATION`; переменные `atomicfile.Attempts = 40`, `atomicfile.Pause = 50 * time.Millisecond`. Скрипт `scripts\fetch-core.ps1 [-Zip <файл>]` → `.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe`, последняя строка `FETCH-OK <путь>` или `FETCH-FAILED <причина>`.

- [ ] **Step 1: Заменить `.gitignore`**

Файл `.gitignore`, целиком:

```text
# Одноразовая проверка осуществимости: ключ узла Tailscale, секрет API, настоящие адреса.
spike/

# Макеты из обсуждения дизайна.
.superpowers/

node_modules/

# Скачанное ядро (scripts/fetch-core.ps1).
/.cache/

# Данные режима разработки: состояние встроенного узла, журналы, конфиг ядра.
/.dev/

# Каталоги сборки.
/helper/bin/
/app/out/
/app/dist/
```

```powershell
foreach ($p in '.cache/x', '.dev/x', 'helper/bin/x', 'spike/x', '.superpowers/x') { git -C C:\code\router check-ignore -q $p; "$p ignored=$($LASTEXITCODE -eq 0)" }
```

Ожидается `ignored=True` во всех пяти строках.

- [ ] **Step 2: Скачать текст GPL-3.0 и сверить его**

```powershell
$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest -Uri https://www.gnu.org/licenses/gpl-3.0.txt -OutFile C:\code\router\LICENSE -UseBasicParsing; (Get-FileHash C:\code\router\LICENSE -Algorithm SHA256).Hash.ToLower(); (Get-Item C:\code\router\LICENSE).Length
```

Ожидается `3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986` и `35149` (так было при подготовке плана). Если сумма другая — открыть файл: первая строка должна быть `GNU GENERAL PUBLIC LICENSE`, вторая `Version 3, 29 June 2007`; при другом тексте остановиться.

- [ ] **Step 3: Записать `README.md` и `THIRD-PARTY.md`**

Файл `README.md`, целиком:

````markdown
# SplitLane

SplitLane — приложение для Windows 10/11, в котором программы раскладываются по двум дорожкам: «Через VPN» и «Напрямую». Этап 1 работает с Tailscale и exit node. Спецификация: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`.

Лицензия: GPL-3.0-or-later (файл `LICENSE`). Сторонние компоненты перечислены в `THIRD-PARTY.md`.

## Состав

- `helper/` — служебный процесс `splitlane-helper.exe` (Go), единственная часть с правами администратора.
- `helper/cmd/slctl` — тестовый клиент канала служебного процесса.
- `helper/cmd/slguard` — сторож для проверок на ПК во время разработки; в поставку не входит.
- `scripts/` — получение ядра с проверкой контрольной суммы.
- `docs/` — спецификации и планы.
- `app/` — окно (Electron), появится в плане 2.

## Ядро

Ядро sing-box 1.14.4 в git не хранится. Скрипт скачивает официальный архив и сверяет SHA-256 со значениями из `scripts/core.sha256`:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\fetch-core.ps1
```

Итог: `.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe`.

## Сборка служебного процесса

Нужен Go 1.26 или новее; CGO не нужен.

```powershell
cd helper
$env:CGO_ENABLED = '0'
go test ./...
go build -ldflags "-H=windowsgui" -o bin\splitlane-helper.exe .\cmd\splitlane-helper
go build -o bin\slctl.exe .\cmd\slctl
go build -o bin\slguard.exe .\cmd\slguard
```

Проверка конфигов настоящим ядром (ядро ничего не запускает, только читает конфиг): переменная `SPLITLANE_CORE` с полным путём `sing-box.exe` перед `go test ./...`.

## Режим разработки

Служебный процесс этой сборки запускается только с `--dev`, из процесса с правами администратора:

```text
splitlane-helper.exe --dev --data-dir C:\code\router\.dev\data --core C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe
```

Каталог `.dev\` в git не попадает. Команда `install` и задача планировщика появятся в следующем плане.
````

Файл `THIRD-PARTY.md`, целиком:

```markdown
# Сторонние компоненты

## sing-box

- Версия: 1.14.4, официальная сборка `windows-amd64`, без изменений. Файл ядра сохраняет имя `sing-box.exe`.
- Лицензия: GNU GPL версии 3 или более поздней, с дополнительным условием автора: производная работа не может использовать название sing-box или намекать на связь с ним без согласия автора. Поэтому слово «sing-box» не входит в название SplitLane, задачи планировщика, канала и собственных исполняемых файлов. Текст лицензии ядра — файл `LICENSE` в архиве ядра.
- Исходный код этой версии: https://github.com/SagerNet/sing-box/tree/v1.14.4
- Архив: https://github.com/SagerNet/sing-box/releases/download/v1.14.4/sing-box-1.14.4-windows-amd64.zip
- SHA-256 архива и `sing-box.exe`: `scripts/core.sha256`. Ядро в git не хранится, его скачивает `scripts/fetch-core.ps1`.

## Модули Go служебного процесса

- `golang.org/x/sys` v0.49.0 — лицензия BSD-3-Clause, https://cs.opensource.google/go/x/sys
- `github.com/Microsoft/go-winio` v0.6.4 — лицензия MIT, https://github.com/microsoft/go-winio
```

- [ ] **Step 4: Записать скрипт получения ядра и закреплённые суммы**

Файл `scripts/core.sha256`, целиком:

```text
99185d75d314a07dbba4a130c8ad986cbeb629a07927975636b341ca61f991df  sing-box-1.14.4-windows-amd64.zip
8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9  sing-box-1.14.4-windows-amd64/sing-box.exe
```

Файл `scripts/fetch-core.ps1`, целиком:

```powershell
# SplitLane: get sing-box 1.14.4 windows-amd64 and check it against the SHA-256 pinned in core.sha256.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -ExecutionPolicy Bypass -File scripts\fetch-core.ps1 [-Zip <local copy of the archive>]
# Result: .cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe; last line FETCH-OK <path> or FETCH-FAILED <reason>.
param([string]$Zip = '')
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$version = '1.14.4'
$name = "sing-box-$version-windows-amd64"
$url = "https://github.com/SagerNet/sing-box/releases/download/v$version/$name.zip"
$root = Split-Path -Parent $PSScriptRoot
$dir = Join-Path $root '.cache\core'

function Get-Pin([string]$file) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $PSScriptRoot 'core.sha256')) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $file) { return $parts[0].ToLower() }
    }
    throw "no pin for $file in core.sha256"
}

function Get-Sha256([string]$path) { (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLower() }

try {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $archive = Join-Path $dir "$name.zip"
    if ($Zip) {
        Copy-Item -LiteralPath $Zip -Destination $archive -Force
    } elseif (-not (Test-Path -LiteralPath $archive)) {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $url -OutFile "$archive.part" -UseBasicParsing
        Move-Item -LiteralPath "$archive.part" -Destination $archive -Force
    }
    $want = Get-Pin "$name.zip"
    $got = Get-Sha256 $archive
    if ($got -ne $want) {
        Remove-Item -LiteralPath $archive -Force
        Write-Host "FETCH-FAILED $name.zip has SHA-256 $got, the pin is $want; the archive was deleted"
        exit 1
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $dir -Force
    $exe = Join-Path $dir "$name\sing-box.exe"
    $wantExe = Get-Pin "$name/sing-box.exe"
    $gotExe = Get-Sha256 $exe
    if ($gotExe -ne $wantExe) {
        Write-Host "FETCH-FAILED sing-box.exe has SHA-256 $gotExe, the pin is $wantExe"
        exit 1
    }
    Write-Host "FETCH-OK $exe"
} catch {
    Write-Host "FETCH-FAILED $($_.Exception.Message)"
    exit 1
}
```

- [ ] **Step 5: Проверить скрипт на испорченном архиве, затем скачать ядро**

```powershell
[IO.File]::WriteAllText('C:\code\router\spike\plan1\bad.zip', 'x'); powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\scripts\fetch-core.ps1 -Zip C:\code\router\spike\plan1\bad.zip; "exit=$LASTEXITCODE"; powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\scripts\fetch-core.ps1; "exit=$LASTEXITCODE"
```

Ожидается сначала `FETCH-FAILED sing-box-1.14.4-windows-amd64.zip has SHA-256 2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881, the pin is 99185d75…; the archive was deleted` и `exit=1`, затем `FETCH-OK C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe` и `exit=0`. Если скачивание с GitHub не проходит, то же делает `-Zip C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64.zip` (архив плана 0, проверяется теми же суммами).

- [ ] **Step 6: Создать модуль и написать тест атомарной записи**

Файл `helper/go.mod`, целиком:

```text
module splitlane/helper

go 1.26.0

require golang.org/x/sys v0.49.0
```

Файл `helper/internal/atomicfile/atomicfile_test.go`, целиком:

```go
package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCreatesAndReplacesWithoutBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lane-vpn.json")
	if err := Write(path, []byte(`{"a":"Кириллица"}`)); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte(`{"b":2}`)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"b":2}` {
		t.Fatalf("content %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestWriteRefusesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	err := Write(path, append([]byte{0xEF, 0xBB, 0xBF}, '{', '}'))
	if err == nil || !strings.Contains(err.Error(), "byte order mark") {
		t.Fatalf("want a BOM error, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target must not exist, stat: %v", err)
	}
}

func TestWriteRetriesWhileReaderHoldsTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lane-direct.json")
	if err := Write(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	// os.Open shares read and write but not delete, like a reader that blocks the rename.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		f.Close()
	}()
	if err := Write(path, []byte("new")); err != nil {
		t.Fatalf("write while held: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content %q", got)
	}
}

func TestWriteGivesUpAfterAttempts(t *testing.T) {
	oldAttempts, oldPause := Attempts, Pause
	Attempts, Pause = 3, 10*time.Millisecond
	defer func() { Attempts, Pause = oldAttempts, oldPause }()
	dir := t.TempDir()
	path := filepath.Join(dir, "held.json")
	if err := Write(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Write(path, []byte("new")); err == nil {
		t.Fatal("want an error while the target stays held")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
```

- [ ] **Step 7: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/atomicfile/
```

Ожидается: `FAIL splitlane/helper/internal/atomicfile [build failed]` и `internal\atomicfile\atomicfile_test.go:14:12: undefined: Write`.

- [ ] **Step 8: Реализовать атомарную запись**

Файл `helper/internal/atomicfile/atomicfile.go`, целиком:

```go
// Package atomicfile writes the files that the core and the helper read back:
// UTF-8 without a byte order mark, a temp file in the same folder, flushed to
// disk, then renamed over the target (spec 6.5, 5.7).
package atomicfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Attempts and Pause bound the retries of the final rename: the core may hold
// the target open for reading at that moment (spec 6.5).
var (
	Attempts = 40
	Pause    = 50 * time.Millisecond
)

var bom = []byte{0xEF, 0xBB, 0xBF}

// Write replaces path with data. data must not start with a UTF-8 byte order mark.
func Write(path string, data []byte) error {
	if bytes.HasPrefix(data, bom) {
		return errors.New("atomicfile: data starts with a byte order mark")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
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
	for attempt := 1; ; attempt++ {
		err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if attempt >= Attempts || !retryable(err) {
			return fmt.Errorf("atomicfile: replace %s: %w", path, err)
		}
		time.Sleep(Pause)
	}
}

func retryable(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
```

- [ ] **Step 9: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go mod tidy; go vet ./...; go test ./internal/atomicfile/; Get-Content go.sum
```

Ожидается `ok  splitlane/helper/internal/atomicfile` и в `go.sum` ровно две строки:

```text
golang.org/x/sys v0.49.0 h1:XbzkgYJHdqh/8m2Uu0W/dQv8nktxx4BFHp1M0gROTXA=
golang.org/x/sys v0.49.0/go.mod h1:hNLxWAXmnKAxqDtdwIYC4bM9oQPEecfsnNMuSxOs3og=
```

- [ ] **Step 10: Проверить отсутствие игнорируемых путей и закоммитить**

```powershell
git -C C:\code\router status --short
```

Ожидаются только `.gitignore`, `LICENSE`, `README.md`, `THIRD-PARTY.md`, `scripts/`, `helper/`; нет `spike/`, `.cache/`, `.dev/`, `.superpowers/`.

```powershell
git -C C:\code\router add .gitignore LICENSE README.md THIRD-PARTY.md scripts helper
git -C C:\code\router commit -m "chore: bootstrap repository, core fetch script and atomic file writes" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Сообщения канала, состояние и проверка входных данных

Пакет `protocol` — словарь канала из 5.1: методы, коды ошибок, фазы, причины, форма состояния 5.2 и правила программы 8.1, строгий разбор параметров (неизвестное поле — `bad_request`) и проверки раздела 11: две дорожки, четыре вида правил, ограничения длины, UUID, адрес exit node из списка или из `100.64.0.0/10`.

**Files:**
- Create: `helper/internal/protocol/protocol.go`, `helper/internal/protocol/protocol_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: `const Version = 1`; константы `Method*` (13 методов) и `Methods []string`; `Code*` (9 кодов); `Phase*` (7 фаз); `Reason*` (18 причин); ключи `Reason.Params` — `ParamCommand = "command"`, `ParamError = "error"`; `LaneVPN`, `LaneDirect`, `KindFolder`, `KindFile`, `KindPackage`, `KindSystem`; `RejectTooBroad`, `RejectProtected`, `RejectDuplicate`; `MaxLine`, `MaxApps`, `MaxNameLen`, `MaxPathLen`; типы `Request{ID json.RawMessage; Method string; Params json.RawMessage}`, `Error{Code, Message string; Protocol int; Rules []RuleRejection}` (метод `Error() string`), `RuleRejection{ID, Reason}`, `Event`, `Matcher{Kind, Value}`, `AppRule{ID, Name, Exe string; Matcher Matcher; Lane string}`, `ExitNode{ID, Name string; Online bool}`, `Reason{Code string; Params map[string]string}`, `Addresses{VPN, Direct *string}`, `Tailscale`, `SystemTailscale`, `State` (метод `Clone() State`), `HelloParams`, `HelloResult{Protocol int; Helper, Core string}`, `DisableParams{SkipRestore bool}`, `AllDirectParams{On *bool}`, `LanesSetParams{Apps []AppRule; DefaultLane string}`, `SetExitNodeParams{ID string}`; функции `Errorf(code, format string, args ...any) *Error`, `DecodeParams(raw json.RawMessage, v any) *Error`, `ValidateLanes(LanesSetParams) *Error`, `ValidateExitNodeID(id string, known []ExitNode) *Error`, `NewScanner(io.Reader) *bufio.Scanner`, `EncodeResult(id json.RawMessage, result any) []byte`, `EncodeError(id json.RawMessage, e *Error) []byte`, `EncodeEvent(name string, data any) []byte`, `MaskAddresses(string) string`.

Формы JSON на канале (5.1, 5.2, 8.4 — последняя в плане 1B):

```text
запрос:   {"id": 1, "method": "lanes.set", "params": {"apps": [...], "defaultLane": "vpn"}}
ответ:    {"id": 1, "result": {}}
ошибка:   {"id": 1, "error": {"code": "rule_rejected", "message": "...", "rules": [{"id": "<uuid>", "reason": "protected"}]}}
          {"id": 1, "error": {"code": "protocol_mismatch", "message": "...", "protocol": 1}}
событие:  {"event": "state", "data": {<состояние 5.2>}}
hello:    {"id": 1, "method": "hello", "params": {"protocol": 1}} -> {"id": 1, "result": {"protocol": 1, "helper": "0.1.0-dev", "core": "1.14.4"}}
состояние: {"phase": "off", "reason": null, "allDirect": false, "defaultLane": "vpn", "apps": [],
            "addresses": {"vpn": null, "direct": null},
            "tailscale": {"backend": "stopped", "authUrl": null, "deviceName": "...", "exitNodes": [], "selectedExitNode": null, "path": null},
            "systemTailscale": {"installed": true, "exitNodeOn": true, "restoreOnDisable": false}}
причины:  "reason": {"code": "restore_pending", "params": {"command": "\"<каталог клиента>\\tailscale.exe\" set --exit-node=<адрес> --exit-node-allow-lan-access=<true|false>", "error": "<текст ошибки tailscale.exe>"}}
          "reason": {"code": "switch_check_failed", "params": {"error": "<текст>"}}   (так же config_invalid и core_start_timeout; ключ error может отсутствовать)
          "reason": {"code": "vpn_unreachable", "params": {}}                         (остальные причины — с пустым params)
activity (8.4, отправляет план 1B):
          {"id": 1, "method": "activity.subscribe", "params": {"detailFor": "<key>"}}   (detailFor необязателен)
          {"event": "activity", "data": {"lanes": {"vpn": {"down": 0, "up": 0}, "direct": {"down": 0, "up": 0}},
           "apps": [{"key": "", "kind": "app", "name": "", "exe": "", "ruleId": null, "lane": "vpn", "down": 0, "up": 0, "connections": 0}],
           "detail": {"key": "", "exes": [], "connections": [{"host": "", "lane": "vpn", "down": 0, "up": 0}]}}}
```

Ключи `params` причин — договор с окном (план 2) и сценарной проверкой: они заданы константами `ParamCommand` и `ParamError` и в коде служебного процесса строками не пишутся.

- [ ] **Step 1: Написать тест**

Файл `helper/internal/protocol/protocol_test.go`, целиком:

```go
package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

const goodID = "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"

func app(id, lane, kind string) AppRule {
	return AppRule{ID: id, Exe: `C:\Tools\curl-a.exe`, Matcher: Matcher{Kind: kind}, Lane: lane}
}

func TestEncodeResultAndError(t *testing.T) {
	got := string(EncodeResult(json.RawMessage(`7`), nil))
	if got != "{\"id\":7,\"result\":{}}\n" {
		t.Fatalf("result line %q", got)
	}
	e := &Error{Code: CodeProtocolMismatch, Message: "helper speaks 1", Protocol: 1}
	got = string(EncodeError(json.RawMessage(`"a"`), e))
	if got != "{\"id\":\"a\",\"error\":{\"code\":\"protocol_mismatch\",\"message\":\"helper speaks 1\",\"protocol\":1}}\n" {
		t.Fatalf("error line %q", got)
	}
	e = &Error{Code: CodeRuleRejected, Message: "1 rule", Rules: []RuleRejection{{ID: goodID, Reason: RejectProtected}}}
	if !strings.Contains(string(EncodeError(nil, e)), `"rules":[{"id":"`+goodID+`","reason":"protected"}]`) {
		t.Fatal("rules field missing")
	}
	if got := string(EncodeEvent("state", map[string]int{"x": 1})); got != "{\"event\":\"state\",\"data\":{\"x\":1}}\n" {
		t.Fatalf("event line %q", got)
	}
}

func TestDecodeParamsIsStrict(t *testing.T) {
	var p DisableParams
	if err := DecodeParams(nil, &p); err != nil || p.SkipRestore {
		t.Fatalf("missing params: %v %+v", err, p)
	}
	if err := DecodeParams(json.RawMessage(`{"skipRestore":true}`), &p); err != nil || !p.SkipRestore {
		t.Fatalf("skipRestore: %v %+v", err, p)
	}
	if err := DecodeParams(json.RawMessage(`{"skip":true}`), &p); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("unknown field must be bad_request, got %v", err)
	}
	if err := DecodeParams(json.RawMessage(`{"skipRestore":"yes"}`), &p); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("wrong type must be bad_request, got %v", err)
	}
}

func TestValidateLanes(t *testing.T) {
	ok := LanesSetParams{DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneDirect, KindFile)}}
	if err := ValidateLanes(ok); err != nil {
		t.Fatal(err)
	}
	long := app(goodID, LaneVPN, KindFile)
	long.Name = strings.Repeat("я", MaxNameLen)
	cases := map[string]LanesSetParams{
		"default lane": {DefaultLane: "both"},
		"not a uuid":   {DefaultLane: LaneVPN, Apps: []AppRule{app("1", LaneVPN, KindFile)}},
		"repeated id":  {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneVPN, KindFile), app(goodID, LaneDirect, KindFile)}},
		"lane":         {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, "tor", KindFile)}},
		"kind":         {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneVPN, "glob")}},
		"name length":  {DefaultLane: LaneVPN, Apps: []AppRule{long}},
	}
	for name, p := range cases {
		if err := ValidateLanes(p); err == nil || err.Code != CodeBadRequest {
			t.Errorf("%s: want bad_request, got %v", name, err)
		}
	}
}

func TestValidateExitNodeID(t *testing.T) {
	known := []ExitNode{{ID: "100.64.0.7", Name: "a", Online: true}}
	if err := ValidateExitNodeID("100.64.0.7", known); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExitNodeID("100.64.0.8", known); err == nil || err.Code != CodeUnknownExitNode {
		t.Fatalf("not in list: %v", err)
	}
	if err := ValidateExitNodeID("100.100.1.1", nil); err != nil {
		t.Fatalf("in range without a list: %v", err)
	}
	if err := ValidateExitNodeID("192.0.2.1", nil); err == nil || err.Code != CodeUnknownExitNode {
		t.Fatalf("outside range: %v", err)
	}
	if err := ValidateExitNodeID("node-name", nil); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("not an address: %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	ip := "192.0.2.1"
	s := State{Apps: []AppRule{app(goodID, LaneVPN, KindFile)}, Reason: &Reason{Code: ReasonRestorePending, Params: map[string]string{"command": "x"}}}
	s.Addresses.VPN = &ip
	c := s.Clone()
	c.Apps[0].Lane = LaneDirect
	c.Reason.Params["command"] = "y"
	*c.Addresses.VPN = "198.51.100.1"
	if s.Apps[0].Lane != LaneVPN || s.Reason.Params["command"] != "x" || *s.Addresses.VPN != ip {
		t.Fatal("clone shares memory with the original")
	}
}

func TestStateJSONHasEmptyListsAndNulls(t *testing.T) {
	s := State{Phase: PhaseOff, DefaultLane: LaneVPN, Apps: []AppRule{}, Tailscale: Tailscale{Backend: "stopped", ExitNodes: []ExitNode{}}}
	b, _ := json.Marshal(s)
	for _, want := range []string{`"reason":null`, `"apps":[]`, `"addresses":{"vpn":null,"direct":null}`, `"exitNodes":[]`, `"selectedExitNode":null`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state JSON lacks %s: %s", want, b)
		}
	}
}

func TestMaskAddresses(t *testing.T) {
	if got := MaskAddresses(`{"vpn":"203.0.113.9","core":"1.14.4"}`); got != `{"vpn":"<ip>","core":"1.14.4"}` {
		t.Fatalf("masked %q", got)
	}
}

func TestScannerTakesLongLines(t *testing.T) {
	long := `{"id":1,"method":"lanes.set","params":{"x":"` + strings.Repeat("a", 200000) + `"}}`
	s := NewScanner(strings.NewReader(long + "\n" + `{"id":2}` + "\n"))
	n := 0
	for s.Scan() {
		n++
	}
	if s.Err() != nil || n != 2 {
		t.Fatalf("lines %d err %v", n, s.Err())
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/protocol/
```

Ожидается: `FAIL splitlane/helper/internal/protocol [build failed]` и `undefined: AppRule`.

- [ ] **Step 3: Реализовать пакет**

Файл `helper/internal/protocol/protocol.go`, целиком:

```go
// Package protocol holds the pipe messages of spec 5.1, the state of 5.2 and
// the program rule of 8.1, with the input checks of spec 11.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"regexp"
)

// Version is the pipe protocol version of stage 1 (spec 5.1).
const Version = 1

// Methods (spec 5.1). There are no others.
const (
	MethodHello               = "hello"
	MethodStateGet            = "state.get"
	MethodEnable              = "routing.enable"
	MethodDisable             = "routing.disable"
	MethodAllDirect           = "routing.allDirect"
	MethodLanesSet            = "lanes.set"
	MethodLogin               = "tailscale.login"
	MethodLogout              = "tailscale.logout"
	MethodSetExitNode         = "tailscale.setExitNode"
	MethodClearSystemExitNode = "systemTailscale.clearExitNode"
	MethodActivitySubscribe   = "activity.subscribe"
	MethodDiagnosticsExport   = "diagnostics.export"
	MethodShutdown            = "helper.shutdown"
)

// Methods lists every method of spec 5.1.
var Methods = []string{
	MethodHello, MethodStateGet, MethodEnable, MethodDisable, MethodAllDirect, MethodLanesSet,
	MethodLogin, MethodLogout, MethodSetExitNode, MethodClearSystemExitNode,
	MethodActivitySubscribe, MethodDiagnosticsExport, MethodShutdown,
}

// Error codes: the closed list of spec 5.1.
const (
	CodeBadRequest       = "bad_request"
	CodeUnknownMethod    = "unknown_method"
	CodeProtocolMismatch = "protocol_mismatch"
	CodeWrongPhase       = "wrong_phase"
	CodeBusy             = "busy"
	CodeRuleRejected     = "rule_rejected"
	CodeApplyFailed      = "apply_failed"
	CodeUnknownExitNode  = "unknown_exit_node"
	CodeInternal         = "internal"
)

// Phases (spec 5.2).
const (
	PhaseOff      = "off"
	PhaseStarting = "starting"
	PhaseOn       = "on"
	PhaseDegraded = "degraded"
	PhaseBlocked  = "blocked"
	PhaseStopping = "stopping"
	PhaseError    = "error"
)

// Reason codes (spec 5.2).
const (
	ReasonNoDefaultRoute    = "no_default_route"
	ReasonIPForwarding      = "ip_forwarding"
	ReasonOtherCore         = "other_core"
	ReasonNotLoggedIn       = "not_logged_in"
	ReasonNoExitNode        = "no_exit_node"
	ReasonSystemDNSOverride = "system_dns_override"
	ReasonConfigInvalid     = "config_invalid"
	ReasonCoreStartTimeout  = "core_start_timeout"
	ReasonTunnelNotProven   = "tunnel_not_proven"
	ReasonSameAddress       = "same_address"
	ReasonSwitchCheckFailed = "switch_check_failed"
	ReasonLeakDetected      = "leak_detected"
	ReasonCoreCrashed       = "core_crashed"
	ReasonCoreUnresponsive  = "core_unresponsive"
	ReasonVPNUnreachable    = "vpn_unreachable"
	ReasonLoginExpired      = "login_expired"
	ReasonRestorePending    = "restore_pending"
	ReasonAdapterLeft       = "adapter_left"
)

// Keys of Reason.Params (spec 5.2): restore_pending carries the manual restore command and the
// error text of tailscale.exe; switch_check_failed, config_invalid and core_start_timeout may
// carry the error text. Every other reason has empty params.
const (
	ParamCommand = "command"
	ParamError   = "error"
)

// Lanes and rule kinds (spec 3, 8.1).
const (
	LaneVPN    = "vpn"
	LaneDirect = "direct"

	KindFolder  = "folder"
	KindFile    = "file"
	KindPackage = "package"
	KindSystem  = "system"
)

// Rejection reasons of lanes.set (spec 8.3).
const (
	RejectTooBroad  = "too_broad"
	RejectProtected = "protected"
	RejectDuplicate = "duplicate"
)

// Limits of spec 11: the window's data is checked before it is used.
const (
	MaxLine    = 1 << 20
	MaxApps    = 500
	MaxNameLen = 256
	MaxPathLen = 1024
)

// Request is one line from a client.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Error is the error object of a response.
type Error struct {
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Protocol int             `json:"protocol,omitempty"`
	Rules    []RuleRejection `json:"rules,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf builds an *Error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// RuleRejection names one rejected rule of lanes.set (spec 8.3).
type RuleRejection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type okResponse struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result"`
}

type errResponse struct {
	ID    json.RawMessage `json:"id"`
	Error *Error          `json:"error"`
}

// Event is a message from the helper without an id (spec 5.1).
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Matcher says which executables belong to a program (spec 8.1).
type Matcher struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// AppRule is a pinned program (spec 8.1).
type AppRule struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Exe     string  `json:"exe"`
	Matcher Matcher `json:"matcher"`
	Lane    string  `json:"lane"`
}

// ExitNode is one entry of tailscale.exitNodes; ID is the node's IPv4 address in the tailnet (spec 7).
type ExitNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

// Reason explains the phase; Params carries extra text, for example the manual command of restore_pending.
type Reason struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params"`
}

// Addresses are the external addresses of the two lanes.
type Addresses struct {
	VPN    *string `json:"vpn"`
	Direct *string `json:"direct"`
}

// Tailscale describes the embedded node.
type Tailscale struct {
	Backend          string     `json:"backend"`
	AuthURL          *string    `json:"authUrl"`
	DeviceName       string     `json:"deviceName"`
	ExitNodes        []ExitNode `json:"exitNodes"`
	SelectedExitNode *string    `json:"selectedExitNode"`
	Path             *string    `json:"path"`
}

// SystemTailscale describes the official client (spec 5.6).
type SystemTailscale struct {
	Installed        bool `json:"installed"`
	ExitNodeOn       bool `json:"exitNodeOn"`
	RestoreOnDisable bool `json:"restoreOnDisable"`
}

// State is the full state of spec 5.2.
type State struct {
	Phase           string          `json:"phase"`
	Reason          *Reason         `json:"reason"`
	AllDirect       bool            `json:"allDirect"`
	DefaultLane     string          `json:"defaultLane"`
	Apps            []AppRule       `json:"apps"`
	Addresses       Addresses       `json:"addresses"`
	Tailscale       Tailscale       `json:"tailscale"`
	SystemTailscale SystemTailscale `json:"systemTailscale"`
}

// Clone returns a deep copy, safe to hand to another goroutine.
func (s State) Clone() State {
	c := s
	c.Apps = append([]AppRule{}, s.Apps...)
	c.Tailscale.ExitNodes = append([]ExitNode{}, s.Tailscale.ExitNodes...)
	if s.Reason != nil {
		r := Reason{Code: s.Reason.Code, Params: map[string]string{}}
		for k, v := range s.Reason.Params {
			r.Params[k] = v
		}
		c.Reason = &r
	}
	c.Addresses.VPN = copyString(s.Addresses.VPN)
	c.Addresses.Direct = copyString(s.Addresses.Direct)
	c.Tailscale.AuthURL = copyString(s.Tailscale.AuthURL)
	c.Tailscale.SelectedExitNode = copyString(s.Tailscale.SelectedExitNode)
	c.Tailscale.Path = copyString(s.Tailscale.Path)
	return c
}

func copyString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Parameters and results.
type HelloParams struct {
	Protocol int `json:"protocol"`
}

type HelloResult struct {
	Protocol int    `json:"protocol"`
	Helper   string `json:"helper"`
	Core     string `json:"core"`
}

type DisableParams struct {
	SkipRestore bool `json:"skipRestore"`
}

type AllDirectParams struct {
	On *bool `json:"on"`
}

type LanesSetParams struct {
	Apps        []AppRule `json:"apps"`
	DefaultLane string    `json:"defaultLane"`
}

type SetExitNodeParams struct {
	ID string `json:"id"`
}

// DecodeParams decodes params strictly: unknown fields and trailing data are bad_request.
// Missing params decode as the zero value.
func DecodeParams(raw json.RawMessage, v any) *Error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Errorf(CodeBadRequest, "params: %v", err)
	}
	if dec.More() {
		return Errorf(CodeBadRequest, "params: trailing data")
	}
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateLanes checks the shape of lanes.set (spec 11): lanes, kinds, lengths, unique UUIDs.
// The meaning of each rule (8.2) is checked by package rules.
func ValidateLanes(p LanesSetParams) *Error {
	if p.DefaultLane != LaneVPN && p.DefaultLane != LaneDirect {
		return Errorf(CodeBadRequest, "defaultLane must be vpn or direct")
	}
	if len(p.Apps) > MaxApps {
		return Errorf(CodeBadRequest, "too many apps: %d > %d", len(p.Apps), MaxApps)
	}
	seen := map[string]bool{}
	for i, a := range p.Apps {
		switch {
		case !uuidRe.MatchString(a.ID):
			return Errorf(CodeBadRequest, "apps[%d].id is not a UUID", i)
		case seen[a.ID]:
			return Errorf(CodeBadRequest, "apps[%d].id repeats", i)
		case a.Lane != LaneVPN && a.Lane != LaneDirect:
			return Errorf(CodeBadRequest, "apps[%d].lane must be vpn or direct", i)
		case len(a.Name) > MaxNameLen:
			return Errorf(CodeBadRequest, "apps[%d].name is longer than %d bytes", i, MaxNameLen)
		case len(a.Exe) > MaxPathLen || len(a.Matcher.Value) > MaxPathLen:
			return Errorf(CodeBadRequest, "apps[%d] path is longer than %d bytes", i, MaxPathLen)
		}
		switch a.Matcher.Kind {
		case KindFolder, KindFile, KindPackage, KindSystem:
		default:
			return Errorf(CodeBadRequest, "apps[%d].matcher.kind is unknown", i)
		}
		seen[a.ID] = true
	}
	return nil
}

var tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")

// ValidateExitNodeID checks tailscale.setExitNode (spec 11): an IPv4 address from the known
// list, or, when there is no list, an address in 100.64.0.0/10.
func ValidateExitNodeID(id string, known []ExitNode) *Error {
	addr, err := netip.ParseAddr(id)
	if err != nil || !addr.Is4() {
		return Errorf(CodeBadRequest, "id must be an IPv4 address")
	}
	if len(known) == 0 {
		if !tailnetV4.Contains(addr) {
			return Errorf(CodeUnknownExitNode, "id is outside 100.64.0.0/10")
		}
		return nil
	}
	for _, n := range known {
		if n.ID == id {
			return nil
		}
	}
	return Errorf(CodeUnknownExitNode, "id is not in the list of exit nodes")
}

// NewScanner reads one message per line, a line at most MaxLine bytes long.
func NewScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), MaxLine)
	return s
}

// EncodeResult is the line of a successful response.
func EncodeResult(id json.RawMessage, result any) []byte {
	if result == nil {
		result = struct{}{}
	}
	return line(okResponse{ID: rawID(id), Result: result})
}

// EncodeError is the line of a failed response.
func EncodeError(id json.RawMessage, e *Error) []byte {
	return line(errResponse{ID: rawID(id), Error: e})
}

// EncodeEvent is the line of an event.
func EncodeEvent(name string, data any) []byte {
	return line(Event{Event: name, Data: data})
}

func rawID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func line(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(errResponse{ID: json.RawMessage("null"), Error: Errorf(CodeInternal, "encode: %v", err)})
	}
	return append(b, '\n')
}

var ipv4Re = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// MaskAddresses replaces IPv4 addresses with <ip>: logs and console output never carry them.
func MaskAddresses(s string) string { return ipv4Re.ReplaceAllString(s, "<ip>") }
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/protocol/
```

Ожидается `ok  splitlane/helper/internal/protocol`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/protocol
git -C C:\code\router commit -m "feat(helper): pipe protocol types, state shape and input checks" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Правило программы: вид, `matcher.value`, регулярные выражения

8.1 и первая половина 8.2: служебный процесс сам вычисляет `matcher.value` из `exe` и вида; папка с именем-версией (`app-1.0.9`, `124.0.6367.91`) заменяется папкой уровнем выше; общая папка (корень диска, `C:\Windows` и всё внутри, корни Program Files, «Рабочий стол», «Загрузки», «Документы», временная папка) превращает вид `folder` в `file`; пакет Магазина — `<имя пакета>_<издатель>/<файл>`; `system` — `svchost.exe` и `:System`. Выражения без учёта регистра, с экранированием (`regexp.QuoteMeta`, тот же синтаксис, что у ядра). Пустое имя заменяется описанием файла или именем без расширения.

**Files:**
- Create: `helper/internal/rules/matcher.go`, `helper/internal/rules/matcher_test.go`

**Interfaces:**
- Consumes: `protocol.AppRule`, `protocol.Matcher`, `protocol.Kind*`, `protocol.MaxNameLen`.
- Produces: `type Env struct { WindowsDir, ProgramFiles, ProgramFilesX86 string; UserDirs []string; Protected []string; Describe func(exe string) string }`; `Normalize(a protocol.AppRule, env Env) (protocol.AppRule, error)` (ошибка — некорректный ввод, `bad_request`); `Expressions(a protocol.AppRule, env Env) []string`; `BuiltinExpression(path string) string` (правила 5 и 6 из 6.2).

- [ ] **Step 1: Написать тест**

Файл `helper/internal/rules/matcher_test.go`, целиком:

```go
package rules

import (
	"regexp"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

var testEnv = Env{
	WindowsDir:      `C:\Windows`,
	ProgramFiles:    `C:\Program Files`,
	ProgramFilesX86: `C:\Program Files (x86)`,
	UserDirs: []string{`C:\Users\u\Desktop`, `C:\Users\u\Downloads`, `C:\Users\u\Documents`,
		`C:\Users\u\AppData\Local\Temp`},
	Protected: []string{`C:\Program Files\SplitLane\splitlane-helper.exe`, `C:\Program Files\SplitLane\sing-box.exe`,
		`C:\Program Files\Tailscale\tailscaled.exe`, `C:\Program Files\Tailscale\tailscale-ipn.exe`},
	Describe: func(exe string) string {
		if strings.EqualFold(exe, `C:\Games\Steam\steam.exe`) {
			return "Steam Client"
		}
		return ""
	},
}

func rule(id, lane, kind, exe string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: kind}, Lane: lane}
}

func normalized(t *testing.T, kind, exe string) protocol.AppRule {
	t.Helper()
	n, err := Normalize(rule("r", protocol.LaneVPN, kind, exe), testEnv)
	if err != nil {
		t.Fatalf("%s %s: %v", kind, exe, err)
	}
	return n
}

func matches(t *testing.T, a protocol.AppRule, path string) bool {
	t.Helper()
	for _, e := range Expressions(a, testEnv) {
		if regexp.MustCompile(e).MatchString(path) {
			return true
		}
	}
	return false
}

func TestFolderValue(t *testing.T) {
	cases := []struct{ exe, kind, value string }{
		{`C:\Games\Steam\steam.exe`, protocol.KindFolder, `C:\Games\Steam\`},
		{`C:\Users\u\AppData\Local\Discord\app-1.0.9\Discord.exe`, protocol.KindFolder, `C:\Users\u\AppData\Local\Discord\`},
		{`C:\Apps\Tool\124.0.6367.91\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\`},
		{`C:\Apps\Tool\v2\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\v2\`},
		{`C:\Apps\Tool\1.2.3.4.5\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\1.2.3.4.5\`},
		// shared folders turn the rule into a file rule
		{`D:\tool.exe`, protocol.KindFile, `D:\tool.exe`},
		{`C:\Windows\System32\curl.exe`, protocol.KindFile, `C:\Windows\System32\curl.exe`},
		{`C:\Program Files\app.exe`, protocol.KindFile, `C:\Program Files\app.exe`},
		{`C:\Program Files (x86)\app.exe`, protocol.KindFile, `C:\Program Files (x86)\app.exe`},
		{`C:\Program Files\1.2\app.exe`, protocol.KindFile, `C:\Program Files\1.2\app.exe`},
		{`C:\Users\u\Downloads\setup.exe`, protocol.KindFile, `C:\Users\u\Downloads\setup.exe`},
		{`C:\Users\u\Desktop\tool.exe`, protocol.KindFile, `C:\Users\u\Desktop\tool.exe`},
		{`C:\Users\u\Documents\tool.exe`, protocol.KindFile, `C:\Users\u\Documents\tool.exe`},
		{`C:\Users\u\AppData\Local\Temp\x.exe`, protocol.KindFile, `C:\Users\u\AppData\Local\Temp\x.exe`},
		// a subfolder of a shared folder is a program folder of its own
		{`C:\Users\u\Downloads\tool\tool.exe`, protocol.KindFolder, `C:\Users\u\Downloads\tool\`},
	}
	for _, c := range cases {
		n := normalized(t, protocol.KindFolder, c.exe)
		if n.Matcher.Kind != c.kind || n.Matcher.Value != c.value {
			t.Errorf("%s: got %s %q, want %s %q", c.exe, n.Matcher.Kind, n.Matcher.Value, c.kind, c.value)
		}
	}
}

func TestFolderRegexMatchesWholeFolderOnly(t *testing.T) {
	n := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`)
	if !matches(t, n, `c:\games\steam\bin\steamwebhelper.exe`) {
		t.Error("a file in a subfolder must match, in another case too")
	}
	if matches(t, n, `C:\Games\SteamTools\x.exe`) {
		t.Error("a sibling folder with the same prefix must not match")
	}
}

func TestFileRegexWithSpacesBracketsCyrillic(t *testing.T) {
	paths := []string{
		`C:\Tools\Dir With Spaces\curl-space.exe`,
		`C:\Tools\Brackets (x86) [v1]\curl-brackets.exe`,
		`C:\Tools\Кириллица Папка\curl-кириллица.exe`,
	}
	for _, p := range paths {
		n := normalized(t, protocol.KindFile, p)
		if !matches(t, n, p) || !matches(t, n, strings.ToUpper(p)) {
			t.Errorf("%s: the expression %v must match the path in any case", p, Expressions(n, testEnv))
		}
		if matches(t, n, p+".bak") || matches(t, n, `X`+p[1:]) {
			t.Errorf("%s: the expression must match the whole path only", p)
		}
	}
}

func TestPackageRule(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`
	n := normalized(t, protocol.KindPackage, exe)
	if n.Matcher.Value != `Microsoft.WindowsTerminal_8wekyb3d8bbwe/WindowsTerminal.exe` {
		t.Fatalf("value %q", n.Matcher.Value)
	}
	if !matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`) {
		t.Error("another version of the same package must match")
	}
	if !matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__8wekyb3d8bbwe\sub\WindowsTerminal.exe`) {
		t.Error("the same file in a subfolder of the package must match")
	}
	if matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__aaaaaaaaaaaaa\WindowsTerminal.exe`) {
		t.Error("another publisher must not match")
	}
	if _, err := Normalize(rule("p", protocol.LaneVPN, protocol.KindPackage, `C:\Tools\a.exe`), testEnv); err == nil {
		t.Error("a package rule outside WindowsApps must be an error")
	}
}

func TestSystemRule(t *testing.T) {
	n, err := Normalize(protocol.AppRule{ID: "s", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneDirect}, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	if n.Matcher.Value != "" || n.Name != "svchost" {
		t.Fatalf("system rule %+v", n)
	}
	if !matches(t, n, `C:\WINDOWS\system32\svchost.exe`) || !matches(t, n, `:System`) {
		t.Error("svchost.exe and the pseudo path :System must match")
	}
	if matches(t, n, `C:\Windows\explorer.exe`) || matches(t, n, `:System Idle Process`) {
		t.Error("nothing else may match")
	}
}

func TestNormalizeRejectsMalformedInput(t *testing.T) {
	bad := []protocol.AppRule{
		rule("a", protocol.LaneVPN, protocol.KindFile, `tools\curl.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFile, `\\server\share\curl.exe`),
		rule("c", protocol.LaneVPN, protocol.KindFile, `C:\Tools\script.cmd`),
		rule("d", protocol.LaneVPN, "glob", `C:\Tools\curl.exe`),
		{ID: "e", Exe: `C:\Tools\curl.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFile, Value: `C:\Other\curl.exe`}, Lane: protocol.LaneVPN},
		{ID: "f", Matcher: protocol.Matcher{Kind: protocol.KindSystem, Value: "x"}, Lane: protocol.LaneVPN},
	}
	for _, a := range bad {
		if _, err := Normalize(a, testEnv); err == nil {
			t.Errorf("rule %s must be rejected as malformed", a.ID)
		}
	}
}

func TestNormalizeKeepsAnUnchangedRule(t *testing.T) {
	first := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`)
	again, err := Normalize(first, testEnv)
	if err != nil || again != first {
		t.Fatalf("an unchanged rule must normalize to itself: %v %+v", err, again)
	}
}

func TestNameComesFromDescriptionOrFileName(t *testing.T) {
	if n := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`); n.Name != "Steam Client" {
		t.Errorf("name %q", n.Name)
	}
	if n := normalized(t, protocol.KindFile, `C:\Tools\curl-a.exe`); n.Name != "curl-a" {
		t.Errorf("name %q", n.Name)
	}
	r := rule("n", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`)
	r.Name = "Мой curl"
	if n, _ := Normalize(r, testEnv); n.Name != "Мой curl" {
		t.Errorf("a name from the window must stay, got %q", n.Name)
	}
}

func TestBuiltinExpression(t *testing.T) {
	re := regexp.MustCompile(BuiltinExpression(`C:\code\router\helper\bin\splitlane-helper.exe`))
	if !re.MatchString(`C:\CODE\ROUTER\HELPER\BIN\SPLITLANE-HELPER.EXE`) || re.MatchString(`C:\code\router\helper\bin\splitlane-helper.exe.old`) {
		t.Fatal("built-in expression must match the exact path in any case")
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/rules/
```

Ожидается: `FAIL splitlane/helper/internal/rules [build failed]` и `undefined: Env`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/rules/matcher.go`, целиком:

```go
// Package rules turns pinned programs (spec 8.1) into matchers, regular
// expressions and the two rule-set files of the core (spec 6.5, 8.2).
package rules

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"splitlane/helper/internal/protocol"
)

// Env is what the rules need to know about this computer.
type Env struct {
	WindowsDir      string   // C:\Windows
	ProgramFiles    string   // C:\Program Files
	ProgramFilesX86 string   // C:\Program Files (x86)
	UserDirs        []string // Desktop, Downloads, Documents and the temp folder of the setup user
	Protected       []string // full paths: the helper, the core, tailscaled.exe, tailscale-ipn.exe
	// Describe returns the file description from the version resources of exe, or "".
	Describe func(exe string) string
}

// versionDir is a folder named like a version: optional "app-" and two to four numbers (spec 8.2).
var versionDir = regexp.MustCompile(`(?i)^(app-)?\d+(\.\d+){1,3}$`)

// Normalize computes matcher.value (spec 8.1, 8.2) and fills an empty name.
// A folder that is shared by many programs turns the rule into kind file.
// An error means the input itself is malformed (bad_request).
func Normalize(a protocol.AppRule, env Env) (protocol.AppRule, error) {
	n := a
	if a.Matcher.Kind == protocol.KindSystem {
		if a.Matcher.Value != "" {
			return n, fmt.Errorf("rule %s: a system rule has an empty value", a.ID)
		}
		if n.Name == "" {
			n.Name = displayName(svchost(env), env)
		}
		return n, nil
	}
	exe, err := cleanExe(a.Exe)
	if err != nil {
		return n, fmt.Errorf("rule %s: %v", a.ID, err)
	}
	n.Exe = exe
	kind, value := a.Matcher.Kind, ""
	switch kind {
	case protocol.KindFile:
		value = exe
	case protocol.KindFolder:
		dir := filepath.Dir(exe)
		if versionDir.MatchString(filepath.Base(dir)) && !isRoot(dir) {
			dir = filepath.Dir(dir)
		}
		if shared(dir, env) {
			kind, value = protocol.KindFile, exe
		} else {
			value = strings.TrimRight(dir, `\`) + `\`
		}
	case protocol.KindPackage:
		v, ok := packageValue(exe)
		if !ok {
			return n, fmt.Errorf("rule %s: %s is not inside a WindowsApps package folder", a.ID, exe)
		}
		value = v
	default:
		return n, fmt.Errorf("rule %s: unknown kind %q", a.ID, kind)
	}
	if a.Matcher.Value != "" && a.Matcher.Value != value {
		return n, fmt.Errorf("rule %s: matcher.value differs from the one the helper computes", a.ID)
	}
	n.Matcher = protocol.Matcher{Kind: kind, Value: value}
	if n.Name == "" {
		n.Name = displayName(exe, env)
	}
	return n, nil
}

func cleanExe(exe string) (string, error) {
	if exe == "" || !filepath.IsAbs(exe) || filepath.VolumeName(exe) == "" || strings.HasPrefix(exe, `\\`) {
		return "", fmt.Errorf("exe must be an absolute local path")
	}
	exe = filepath.Clean(exe)
	if !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return "", fmt.Errorf("exe must end with .exe")
	}
	return exe, nil
}

func isRoot(dir string) bool {
	return strings.TrimRight(dir, `\`) == strings.TrimRight(filepath.VolumeName(dir), `\`)
}

// shared reports the folders that many programs share: there a folder rule becomes a file rule (spec 8.2).
func shared(dir string, env Env) bool {
	d := strings.TrimRight(dir, `\`)
	if isRoot(dir) || underOrEqual(d, env.WindowsDir) {
		return true
	}
	for _, s := range append([]string{env.ProgramFiles, env.ProgramFilesX86}, env.UserDirs...) {
		if s != "" && strings.EqualFold(d, strings.TrimRight(s, `\`)) {
			return true
		}
	}
	return false
}

func underOrEqual(path, root string) bool {
	if root == "" {
		return false
	}
	root = strings.TrimRight(root, `\`)
	return strings.EqualFold(path, root) || hasPrefixFold(path, root+`\`)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// packageValue is "<package name>_<publisher id>/<file name>" for an exe inside
// ...\WindowsApps\<Name>_<Version>_<Arch>_<ResourceId>_<PublisherId>\...
func packageValue(exe string) (string, bool) {
	parts := strings.Split(exe, `\`)
	for i := 0; i+2 < len(parts); i++ {
		if !strings.EqualFold(parts[i], "WindowsApps") {
			continue
		}
		fields := strings.Split(parts[i+1], "_")
		if len(fields) != 5 || fields[0] == "" || fields[4] == "" {
			return "", false
		}
		return fields[0] + "_" + fields[4] + "/" + parts[len(parts)-1], true
	}
	return "", false
}

func splitPackage(value string) (name, publisher, file string) {
	slash := strings.LastIndex(value, "/")
	id := value[:slash]
	under := strings.LastIndex(id, "_")
	return id[:under], id[under+1:], value[slash+1:]
}

func svchost(env Env) string {
	return strings.TrimRight(env.WindowsDir, `\`) + `\System32\svchost.exe`
}

func displayName(exe string, env Env) string {
	if env.Describe != nil {
		if d := strings.TrimSpace(env.Describe(exe)); d != "" {
			return truncate(d)
		}
	}
	base := filepath.Base(exe)
	return truncate(strings.TrimSuffix(base, filepath.Ext(base)))
}

func truncate(s string) string {
	for len(s) > protocol.MaxNameLen {
		r := []rune(s)
		s = string(r[:len(r)-1])
	}
	return s
}

// Expressions are the case-insensitive process_path_regex expressions of a normalized rule (spec 8.1).
func Expressions(a protocol.AppRule, env Env) []string {
	v := a.Matcher.Value
	switch a.Matcher.Kind {
	case protocol.KindFolder:
		return []string{`(?i)^` + regexp.QuoteMeta(v)}
	case protocol.KindFile:
		return []string{`(?i)^` + regexp.QuoteMeta(v) + `$`}
	case protocol.KindPackage:
		name, publisher, file := splitPackage(v)
		return []string{`(?i)^.*\\WindowsApps\\` + regexp.QuoteMeta(name) + `_[^\\]*_` +
			regexp.QuoteMeta(publisher) + `\\(?:[^\\]*\\)*` + regexp.QuoteMeta(file) + `$`}
	case protocol.KindSystem:
		return []string{`(?i)^` + regexp.QuoteMeta(svchost(env)) + `$`, `(?i)^:System$`}
	}
	return nil
}

// BuiltinExpression is the expression of the built-in rules 5 and 6 of spec 6.2: one exact path.
func BuiltinExpression(path string) string {
	return `(?i)^` + regexp.QuoteMeta(filepath.Clean(path)) + `$`
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/rules/
```

Ожидается `ok  splitlane/helper/internal/rules`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/rules
git -C C:\code\router commit -m "feat(helper): program rule kinds, matcher values and case-insensitive expressions" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Раскладка: отказы, вложенные правила, файлы правил, дорожка процесса

Вторая половина 8.2 и шаг 1 из 8.3: `lanes.set` проверяется целиком (одно отклонённое правило — ничего не сохраняется); отказы `too_broad`, `protected` (правило поймало бы служебный процесс, ядро или системный Tailscale), `duplicate` (тот же охват в разных дорожках, в том числе `system` и `svchost.exe`). Вложенные правила разных дорожек: более широкое правило-папка записывается как логическое `and` с инвертированными вложенными правилами другой дорожки (решение 3 приложения A). Файлы правил — формат 3, первая запись — заглушка, одно поле в объекте. `Set.LaneOf` определяет дорожку процесса по тому же порядку: `file`, `package`, `system` важнее `folder`, вложенная папка важнее внешней.

**Files:**
- Create: `helper/internal/rules/layout.go`, `helper/internal/rules/layout_test.go`

**Interfaces:**
- Consumes: `Env`, `Normalize`, `Expressions` (задача 6); `protocol.RuleRejection`, `protocol.Reject*`.
- Produces: `const Stub = "^:splitlane-stub$"`; `Check(apps []protocol.AppRule, env Env) (normalized []protocol.AppRule, rejected []protocol.RuleRejection, err error)`; `type Headless struct{ Type, Mode string; Rules []Headless; ProcessPathRegex []string; Invert bool }` (JSON `type`, `mode`, `rules`, `process_path_regex`, `invert`); `type RuleSet struct{ Version int; Rules []Headless }`; `Documents(apps, env) (direct, vpn RuleSet, err error)`; `Files(apps, env) (direct, vpn []byte, err error)`; `Compile(apps, env) (*Set, error)`; `(*Set).LaneOf(path string) (lane string, ok bool)`.

Пример файла прямой дорожки для раскладки «папка Steam — напрямую, игра внутри — через VPN»:

```json
{
  "version": 3,
  "rules": [
    { "process_path_regex": ["^:splitlane-stub$"] },
    { "type": "logical", "mode": "and", "rules": [
      { "process_path_regex": ["(?i)^C:\\\\Games\\\\Steam\\\\"] },
      { "process_path_regex": ["(?i)^C:\\\\Games\\\\Steam\\\\steamapps\\\\common\\\\Game\\\\game\\.exe$"], "invert": true }
    ] }
  ]
}
```

- [ ] **Step 1: Написать тест**

Файл `helper/internal/rules/layout_test.go`, целиком:

```go
package rules

import (
	"reflect"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

func checked(t *testing.T, apps ...protocol.AppRule) []protocol.AppRule {
	t.Helper()
	out, rejected, err := Check(apps, testEnv)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	return out
}

func TestCheckRejectsProtectedRules(t *testing.T) {
	_, rejected, err := Check([]protocol.AppRule{
		rule("helper", protocol.LaneDirect, protocol.KindFolder, `C:\Program Files\SplitLane\splitlane-helper.exe`),
		rule("ts", protocol.LaneVPN, protocol.KindFile, `C:\Program Files\Tailscale\tailscaled.exe`),
		rule("ok", protocol.LaneDirect, protocol.KindFile, `C:\Tools\curl-a.exe`),
	}, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.RuleRejection{{ID: "helper", Reason: protocol.RejectProtected}, {ID: "ts", Reason: protocol.RejectProtected}}
	if !reflect.DeepEqual(rejected, want) {
		t.Fatalf("rejected %v", rejected)
	}
}

func TestCheckRejectsSameScopeInBothLanes(t *testing.T) {
	_, rejected, _ := Check([]protocol.AppRule{
		rule("a", protocol.LaneDirect, protocol.KindFile, `C:\Tools\curl-a.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFile, `c:\tools\CURL-A.exe`),
	}, testEnv)
	want := []protocol.RuleRejection{{ID: "a", Reason: protocol.RejectDuplicate}, {ID: "b", Reason: protocol.RejectDuplicate}}
	if !reflect.DeepEqual(rejected, want) {
		t.Fatalf("rejected %v", rejected)
	}
	sys := protocol.AppRule{ID: "s1", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneDirect}
	svc := rule("s2", protocol.LaneVPN, protocol.KindFile, `C:\Windows\System32\svchost.exe`)
	if _, rejected, _ := Check([]protocol.AppRule{sys, svc}, testEnv); len(rejected) != 2 {
		t.Fatalf("system rule and a svchost file rule in other lanes: %v", rejected)
	}
	same := checked(t, rule("c", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`),
		rule("d", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`))
	if len(same) != 2 {
		t.Fatal("the same scope in one lane is allowed")
	}
}

func TestTooBroad(t *testing.T) {
	for _, v := range []string{`C:\`, `D:\`, `C:\Windows\`, `C:\Program Files\`, `C:\Program Files (x86)\`} {
		if !tooBroad(protocol.AppRule{Matcher: protocol.Matcher{Kind: protocol.KindFolder, Value: v}}, testEnv) {
			t.Errorf("%s must be too broad", v)
		}
	}
	for _, v := range []string{`C:\Games\`, `C:\Program Files\Vendor\`} {
		if tooBroad(protocol.AppRule{Matcher: protocol.Matcher{Kind: protocol.KindFolder, Value: v}}, testEnv) {
			t.Errorf("%s must be allowed", v)
		}
	}
}

func TestCheckReportsMalformedInputAsError(t *testing.T) {
	if _, _, err := Check([]protocol.AppRule{rule("a", protocol.LaneVPN, protocol.KindFile, "curl.exe")}, testEnv); err == nil {
		t.Fatal("want an error")
	}
}

func TestDocumentsAlwaysHaveTheStub(t *testing.T) {
	d, v, err := Files(nil, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 3,\n  \"rules\": [\n    {\n      \"process_path_regex\": [\n        \"^:splitlane-stub$\"\n      ]\n    }\n  ]\n}\n"
	if string(d) != want || string(v) != want {
		t.Fatalf("empty layout:\n%s\n%s", d, v)
	}
}

func TestNestedFileInsideDirectFolder(t *testing.T) {
	apps := checked(t,
		rule("steam", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		rule("game", protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`))
	d, v, err := Documents(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	wantDirect := Headless{Type: "logical", Mode: "and", Rules: []Headless{
		{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\`}},
		{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\steamapps\\common\\Game\\game\.exe$`}, Invert: true},
	}}
	if !reflect.DeepEqual(d.Rules[1], wantDirect) {
		t.Fatalf("direct rule %+v", d.Rules[1])
	}
	if !reflect.DeepEqual(v.Rules[1], Headless{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\steamapps\\common\\Game\\game\.exe$`}}) {
		t.Fatalf("vpn rule %+v", v.Rules[1])
	}
	raw, _, _ := Files(apps, testEnv)
	if !strings.Contains(string(raw), `"invert": true`) || !strings.Contains(string(raw), `"type": "logical"`) {
		t.Fatalf("file text:\n%s", raw)
	}
}

func TestNestedDirectFileInsideVPNFolder(t *testing.T) {
	apps := checked(t,
		rule("tools", protocol.LaneVPN, protocol.KindFolder, `C:\Tools\bin\tool.exe`),
		rule("curl", protocol.LaneDirect, protocol.KindFile, `C:\Tools\bin\curl-d.exe`))
	d, v, _ := Documents(apps, testEnv)
	if v.Rules[1].Type != "logical" || len(v.Rules[1].Rules) != 2 || !v.Rules[1].Rules[1].Invert {
		t.Fatalf("vpn rule %+v", v.Rules[1])
	}
	if d.Rules[1].Type != "" {
		t.Fatalf("direct rule %+v", d.Rules[1])
	}
}

func TestNestingChainAndLaneOf(t *testing.T) {
	apps := checked(t,
		rule("a", protocol.LaneDirect, protocol.KindFolder, `C:\Nest\outer\a.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFolder, `C:\Nest\outer\inner\b.exe`),
		rule("c", protocol.LaneDirect, protocol.KindFile, `C:\Nest\outer\inner\deep\c.exe`))
	d, v, _ := Documents(apps, testEnv)
	// a excludes b only (c is in a's own lane), b excludes c
	if got := len(d.Rules[1].Rules); got != 2 || !strings.Contains(d.Rules[1].Rules[1].ProcessPathRegex[0], `inner\\`) {
		t.Fatalf("a: %+v", d.Rules[1])
	}
	if got := len(v.Rules[1].Rules); got != 2 || !strings.Contains(v.Rules[1].Rules[1].ProcessPathRegex[0], `c\.exe`) {
		t.Fatalf("b: %+v", v.Rules[1])
	}
	set, err := Compile(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`C:\Nest\outer\x.exe`:            protocol.LaneDirect,
		`C:\Nest\outer\inner\y.exe`:      protocol.LaneVPN,
		`C:\NEST\OUTER\INNER\DEEP\C.EXE`: protocol.LaneDirect,
		`C:\Nest\outer\inner\deep\z.exe`: protocol.LaneVPN,
	}
	for path, want := range cases {
		if got, ok := set.LaneOf(path); !ok || got != want {
			t.Errorf("%s: got %q %v, want %s", path, got, ok, want)
		}
	}
	if _, ok := set.LaneOf(`C:\Elsewhere\x.exe`); ok {
		t.Error("a path outside every rule has no lane")
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/rules/
```

Ожидается: `FAIL splitlane/helper/internal/rules [build failed]` и `undefined: Check`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/rules/layout.go`, целиком:

```go
package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"splitlane/helper/internal/protocol"
)

// Stub keeps a rule file from ever being empty (spec 6.5).
const Stub = `^:splitlane-stub$`

// Check validates a whole layout (spec 8.2, 8.3 step 1). It returns the normalized
// rules, or the rejected ones, or an error for malformed input (bad_request).
// Nothing is returned as normalized when at least one rule is rejected.
func Check(apps []protocol.AppRule, env Env) ([]protocol.AppRule, []protocol.RuleRejection, error) {
	out := make([]protocol.AppRule, 0, len(apps))
	for _, a := range apps {
		n, err := Normalize(a, env)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, n)
	}
	var rejected []protocol.RuleRejection
	reject := map[string]string{}
	for _, a := range out {
		switch {
		case tooBroad(a, env):
			reject[a.ID] = protocol.RejectTooBroad
		case protects(a, env):
			reject[a.ID] = protocol.RejectProtected
		}
	}
	for i, a := range out {
		for _, b := range out[i+1:] {
			if a.Lane != b.Lane && sameScope(a, b, env) {
				for _, id := range []string{a.ID, b.ID} {
					if reject[id] == "" {
						reject[id] = protocol.RejectDuplicate
					}
				}
			}
		}
	}
	for _, a := range out {
		if r := reject[a.ID]; r != "" {
			rejected = append(rejected, protocol.RuleRejection{ID: a.ID, Reason: r})
		}
	}
	if len(rejected) > 0 {
		return nil, rejected, nil
	}
	return out, nil, nil
}

// tooBroad: a rule may not cover a drive root, the Windows folder or a Program Files folder entirely (spec 8.2).
func tooBroad(a protocol.AppRule, env Env) bool {
	if a.Matcher.Kind != protocol.KindFolder {
		return false
	}
	d := strings.TrimRight(a.Matcher.Value, `\`)
	if isRoot(a.Matcher.Value) {
		return true
	}
	for _, s := range []string{env.WindowsDir, env.ProgramFiles, env.ProgramFilesX86} {
		if s != "" && underOrEqual(strings.TrimRight(s, `\`), d) {
			return true
		}
	}
	return false
}

// protects: the rule would catch the helper, the core or the system Tailscale processes (spec 8.2).
func protects(a protocol.AppRule, env Env) bool {
	for _, e := range Expressions(a, env) {
		re, err := regexp.Compile(e)
		if err != nil {
			return true
		}
		for _, p := range env.Protected {
			if p != "" && re.MatchString(p) {
				return true
			}
		}
	}
	return false
}

// scopes are the keys by which two rules have the same coverage.
func scopes(a protocol.AppRule, env Env) []string {
	v := strings.ToLower(a.Matcher.Value)
	switch a.Matcher.Kind {
	case protocol.KindSystem:
		return []string{"system", "file:" + strings.ToLower(svchost(env))}
	default:
		return []string{a.Matcher.Kind + ":" + v}
	}
}

func sameScope(a, b protocol.AppRule, env Env) bool {
	for _, x := range scopes(a, env) {
		for _, y := range scopes(b, env) {
			if x == y {
				return true
			}
		}
	}
	return false
}

// contains: outer is a folder rule and inner lies inside it (spec 8.2).
func contains(outer, inner protocol.AppRule, env Env) bool {
	if outer.Matcher.Kind != protocol.KindFolder {
		return false
	}
	o := outer.Matcher.Value
	switch inner.Matcher.Kind {
	case protocol.KindFolder:
		return hasPrefixFold(inner.Matcher.Value, o) && !strings.EqualFold(inner.Matcher.Value, o)
	case protocol.KindFile:
		return hasPrefixFold(inner.Matcher.Value, o)
	case protocol.KindPackage:
		return hasPrefixFold(inner.Exe, o)
	case protocol.KindSystem:
		return hasPrefixFold(svchost(env), o)
	}
	return false
}

// Headless is one rule object of a source rule-set file of sing-box (format version 3).
type Headless struct {
	Type             string     `json:"type,omitempty"`
	Mode             string     `json:"mode,omitempty"`
	Rules            []Headless `json:"rules,omitempty"`
	ProcessPathRegex []string   `json:"process_path_regex,omitempty"`
	Invert           bool       `json:"invert,omitempty"`
}

// RuleSet is a source rule-set file.
type RuleSet struct {
	Version int        `json:"version"`
	Rules   []Headless `json:"rules"`
}

// Documents builds the rule sets of the direct and the VPN lane from a checked layout.
// A broader rule gets the nested rules of the other lane as inverted parts of a
// logical "and" rule (spec 8.2). Every expression is compiled here first (spec 6.5).
func Documents(apps []protocol.AppRule, env Env) (direct, vpn RuleSet, err error) {
	direct = RuleSet{Version: 3, Rules: []Headless{{ProcessPathRegex: []string{Stub}}}}
	vpn = RuleSet{Version: 3, Rules: []Headless{{ProcessPathRegex: []string{Stub}}}}
	for _, a := range apps {
		own := Expressions(a, env)
		if err := compileAll(own); err != nil {
			return direct, vpn, fmt.Errorf("rule %s: %v", a.ID, err)
		}
		entry := Headless{ProcessPathRegex: own}
		var excluded []Headless
		for _, b := range apps {
			if b.Lane != a.Lane && contains(a, b, env) {
				excluded = append(excluded, Headless{ProcessPathRegex: Expressions(b, env), Invert: true})
			}
		}
		if len(excluded) > 0 {
			entry = Headless{Type: "logical", Mode: "and", Rules: append([]Headless{{ProcessPathRegex: own}}, excluded...)}
		}
		if a.Lane == protocol.LaneDirect {
			direct.Rules = append(direct.Rules, entry)
		} else {
			vpn.Rules = append(vpn.Rules, entry)
		}
	}
	return direct, vpn, nil
}

// Files is Documents encoded as the bytes written to disk: UTF-8 without BOM, one trailing newline.
func Files(apps []protocol.AppRule, env Env) (direct, vpn []byte, err error) {
	d, v, err := Documents(apps, env)
	if err != nil {
		return nil, nil, err
	}
	if direct, err = json.MarshalIndent(d, "", "  "); err != nil {
		return nil, nil, err
	}
	if vpn, err = json.MarshalIndent(v, "", "  "); err != nil {
		return nil, nil, err
	}
	return append(direct, '\n'), append(vpn, '\n'), nil
}

func compileAll(expressions []string) error {
	for _, e := range expressions {
		if _, err := regexp.Compile(e); err != nil {
			return err
		}
	}
	return nil
}

// Set answers "which lane does this process belong to" by the precedence of spec 8.2:
// file, package and system rules beat folder rules, a deeper folder beats an outer one.
type Set struct {
	entries []entry
}

type entry struct {
	lane  string
	rank  int
	depth int
	res   []*regexp.Regexp
}

// Compile prepares a checked layout for LaneOf.
func Compile(apps []protocol.AppRule, env Env) (*Set, error) {
	s := &Set{}
	for _, a := range apps {
		e := entry{lane: a.Lane, rank: 2, depth: len(a.Matcher.Value)}
		if a.Matcher.Kind == protocol.KindFolder {
			e.rank = 1
		}
		for _, x := range Expressions(a, env) {
			re, err := regexp.Compile(x)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %v", a.ID, err)
			}
			e.res = append(e.res, re)
		}
		s.entries = append(s.entries, e)
	}
	return s, nil
}

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

func matchAny(res []*regexp.Regexp, path string) bool {
	for _, re := range res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/rules/
```

Ожидается `ok  splitlane/helper/internal/rules`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/rules
git -C C:\code\router commit -m "feat(helper): layout checks, nested rules with invert and rule-set files" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Конфиг ядра 1.14.4

Раздел 6: адаптер `SplitLane` (IPv4 и IPv6, `auto_route`, `strict_route`, `gvisor`, исключены диапазоны Tailscale), входы `probe-vpn` и `probe-direct` на `127.0.0.1`, встроенный узел `vpn`, выход `direct`, порядок правил 6.2 (с правилами 5 и 6 по полному пути и пятью режимами), DNS 6.4 с `reverse_mapping`, Clash API и служба API на своих портах со своими секретами, ограничение источников Clash API (`access_control_allow_origin`), журнал `info` в файл. Режим запуска `default_mode` — тот, что нужен сейчас (6.3). Закрепление программ исполнителя — только при `DevPin`. Эталонный конфиг лежит в `testdata`; проверка настоящим ядром — `sing-box check` при заданной `SPLITLANE_CORE` (пять режимов, с закреплением и без, с файлами правил с кириллицей, скобками и логическим правилом).

**Files:**
- Create: `helper/internal/coreconfig/config.go`, `helper/internal/coreconfig/config_test.go`, `helper/internal/coreconfig/testdata/vpn.golden.json`

**Interfaces:**
- Consumes: `rules.BuiltinExpression`, `rules.Check`, `rules.Files`, `rules.Env` (тест), `protocol` (тест).
- Produces: константы `TagProbeVPN`, `TagProbeDirect`, `TagTun`, `TagVPN = "vpn"`, `TagDirect = "direct"`, `RuleSetDirect = "lane-direct"`, `RuleSetVPN = "lane-vpn"`, `AdapterName = "SplitLane"`, `ModeVPN`, `ModeVPNLocalDNS`, `ModeDirect`, `ModeDirectLocalDNS`, `ModeAllDirect`, `StartLine = "sing-box started"`, `LoginLine = "Waiting for authentication"`, `ReloadErrorLine = "reload rule-set lane-"`; `Modes []string`; `ModeFor(defaultLane string, allDirect, degraded bool) string`; `MainLane(mode string) string`; `DevPinProcesses`, `DevPinDomains`, `DevPinCIDR`; `type Ports struct{ ProbeVPN, ProbeDirect, Clash, API int }`; `type Options struct{ Mode, DataDir, HelperPath, TailscaleDir, Hostname, ExitNode string; Ports Ports; ClashSecret, APISecret string; DevPin bool }`; `type Paths struct{ RulesDir, RulesDirect, RulesVPN, LogsDir, CoreLog, CoreStdout, StateDir, Config string }`; `PathsFor(dataDir string) Paths`; `Build(o Options) ([]byte, error)`.

- [ ] **Step 1: Написать тест и эталонный конфиг**

Файл `helper/internal/coreconfig/config_test.go`, целиком:

```go
package coreconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

func testOptions(dataDir string) Options {
	return Options{
		Mode:         ModeVPN,
		DataDir:      dataDir,
		HelperPath:   `C:\Program Files\SplitLane\splitlane-helper.exe`,
		TailscaleDir: `C:\Program Files\Tailscale`,
		Hostname:     "PC-splitlane",
		ExitNode:     "100.64.0.1",
		Ports:        Ports{ProbeVPN: 30001, ProbeDirect: 30002, Clash: 30003, API: 30004},
		ClashSecret:  "0123456789abcdef0123456789abcdef",
		APISecret:    "fedcba9876543210fedcba9876543210",
	}
}

func TestModeFor(t *testing.T) {
	cases := []struct {
		lane              string
		allDirect, degrad bool
		want              string
	}{
		{"vpn", false, false, ModeVPN},
		{"vpn", false, true, ModeVPNLocalDNS},
		{"direct", false, false, ModeDirect},
		{"direct", false, true, ModeDirectLocalDNS},
		{"vpn", true, false, ModeAllDirect},
		{"direct", true, true, ModeAllDirect},
	}
	for _, c := range cases {
		if got := ModeFor(c.lane, c.allDirect, c.degrad); got != c.want {
			t.Errorf("ModeFor(%s,%v,%v)=%s want %s", c.lane, c.allDirect, c.degrad, got, c.want)
		}
	}
	for mode, lane := range map[string]string{ModeVPN: "vpn", ModeVPNLocalDNS: "vpn", ModeDirect: "direct", ModeDirectLocalDNS: "direct", ModeAllDirect: "direct"} {
		if MainLane(mode) != lane {
			t.Errorf("MainLane(%s)", mode)
		}
	}
}

func TestBuildMatchesGolden(t *testing.T) {
	got, err := Build(testOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "vpn.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("config differs from testdata/vpn.golden.json:\n%s", got)
	}
}

// summary turns route.rules into short words, to check the order of spec 6.2.
func summary(t *testing.T, config []byte) []string {
	t.Helper()
	var c struct {
		Route struct {
			Rules []map[string]any `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range c.Route.Rules {
		var key string
		for _, k := range []string{"ip_version", "inbound", "port", "domain_suffix", "ip_cidr", "process_path_regex", "ip_is_private", "process_name", "clash_mode", "rule_set"} {
			if v, ok := r[k]; ok {
				key = k
				if k == "clash_mode" {
					key += "=" + v.(string)
				}
				if k == "inbound" || k == "rule_set" {
					key += "=" + v.([]any)[0].(string)
				}
				break
			}
		}
		out = append(out, key+">"+anyString(r["outbound"])+anyString(r["action"]))
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestRuleOrderOfSpec62(t *testing.T) {
	b, _ := Build(testOptions(`C:\ProgramData\SplitLane`))
	want := []string{
		"ip_version>reject",
		"inbound=probe-vpn>vpnroute", "inbound=probe-direct>directroute",
		"port>hijack-dns",
		"ip_cidr>reject",
		"process_path_regex>directroute", "process_path_regex>vpnroute",
		"ip_is_private>directroute",
		"clash_mode=alldirect>directroute",
		"rule_set=lane-direct>directroute", "rule_set=lane-vpn>vpnroute",
		"clash_mode=direct>directroute", "clash_mode=direct-localdns>directroute",
		"clash_mode=vpn>vpnroute", "clash_mode=vpn-localdns>vpnroute",
	}
	if got := summary(t, b); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("rule order:\n%v\nwant\n%v", got, want)
	}
}

func TestDevPinOnlyWhenAsked(t *testing.T) {
	plain, _ := Build(testOptions(`C:\ProgramData\SplitLane`))
	if bytes.Contains(plain, []byte("claude.exe")) || bytes.Contains(plain, []byte("160.79.104.0/23")) {
		t.Fatal("the product config must not carry the development pin")
	}
	o := testOptions(`C:\ProgramData\SplitLane`)
	o.DevPin = true
	pinned, _ := Build(o)
	got := summary(t, pinned)
	pin, firstMode := -1, -1
	for i, s := range got {
		if strings.HasPrefix(s, "process_name>vpn") && pin < 0 {
			pin = i
		}
		if strings.HasPrefix(s, "clash_mode=") && firstMode < 0 {
			firstMode = i
		}
	}
	if pin < 0 || got[pin+1] != "domain_suffix>vpnroute" || pin > firstMode {
		t.Fatalf("the pin must come before every mode rule: %v", got)
	}
}

func TestBuildRejectsBadOptions(t *testing.T) {
	bad := []func(*Options){
		func(o *Options) { o.Mode = "global" },
		func(o *Options) { o.ExitNode = "exit-node-name" },
		func(o *Options) { o.Ports.API = o.Ports.Clash },
		func(o *Options) { o.Hostname = "" },
		func(o *Options) { o.ClashSecret = "short" },
		func(o *Options) { o.HelperPath = "splitlane-helper.exe" },
	}
	for i, change := range bad {
		o := testOptions(`C:\ProgramData\SplitLane`)
		change(&o)
		if _, err := Build(o); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
}

// TestCoreAcceptsConfigs runs `sing-box check` when SPLITLANE_CORE names sing-box.exe.
// The check reads the config and the rule files only; it starts nothing.
func TestCoreAcceptsConfigs(t *testing.T) {
	core := os.Getenv("SPLITLANE_CORE")
	if core == "" {
		t.Skip("SPLITLANE_CORE is not set")
	}
	dir := t.TempDir()
	paths := PathsFor(dir)
	for _, d := range []string{paths.RulesDir, paths.LogsDir, paths.StateDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := rules.Env{WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`, ProgramFilesX86: `C:\Program Files (x86)`}
	apps, rejected, err := rules.Check([]protocol.AppRule{
		{ID: "a", Exe: `C:\Games\Steam\steam.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFolder}, Lane: protocol.LaneDirect},
		{ID: "b", Exe: `C:\Games\Steam\Кириллица (x86) [1]\game.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFile}, Lane: protocol.LaneVPN},
		{ID: "c", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneVPN},
	}, env)
	if err != nil || rejected != nil {
		t.Fatal(err, rejected)
	}
	direct, vpn, err := rules.Files(apps, env)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(paths.RulesDirect, direct, 0o644)
	os.WriteFile(paths.RulesVPN, vpn, 0o644)
	for _, mode := range Modes {
		for _, pin := range []bool{false, true} {
			o := testOptions(dir)
			o.Mode, o.DevPin = mode, pin
			b, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.Config, b, 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(core, "check", "-c", paths.Config, "-D", dir).CombinedOutput()
			if err != nil {
				t.Errorf("mode %s pin %v: sing-box check: %v\n%s", mode, pin, err, out)
			}
		}
	}
}
```

Файл `helper/internal/coreconfig/testdata/vpn.golden.json`, целиком:

```json
{
  "dns": {
    "final": "dns-vpn",
    "reverse_mapping": true,
    "rules": [
      {
        "action": "route",
        "clash_mode": "vpn-localdns",
        "server": "dns-local"
      },
      {
        "action": "route",
        "clash_mode": "direct-localdns",
        "server": "dns-local"
      },
      {
        "action": "route",
        "clash_mode": "alldirect",
        "server": "dns-local"
      }
    ],
    "servers": [
      {
        "tag": "dns-local",
        "type": "local"
      },
      {
        "detour": "vpn",
        "server": "1.1.1.1",
        "tag": "dns-vpn",
        "type": "https"
      }
    ],
    "strategy": "ipv4_only"
  },
  "endpoints": [
    {
      "exit_node": "100.64.0.1",
      "hostname": "PC-splitlane",
      "state_directory": "C:/ProgramData/SplitLane/tailscale",
      "tag": "vpn",
      "type": "tailscale"
    }
  ],
  "experimental": {
    "clash_api": {
      "access_control_allow_origin": [
        "http://127.0.0.1"
      ],
      "access_control_allow_private_network": false,
      "default_mode": "vpn",
      "external_controller": "127.0.0.1:30003",
      "secret": "0123456789abcdef0123456789abcdef"
    }
  },
  "inbounds": [
    {
      "listen": "127.0.0.1",
      "listen_port": 30001,
      "tag": "probe-vpn",
      "type": "mixed"
    },
    {
      "listen": "127.0.0.1",
      "listen_port": 30002,
      "tag": "probe-direct",
      "type": "mixed"
    },
    {
      "address": [
        "172.19.0.1/30",
        "fdfe:dcba:9876::1/126"
      ],
      "auto_route": true,
      "interface_name": "SplitLane",
      "route_exclude_address": [
        "100.64.0.0/10",
        "fd7a:115c:a1e0::/48"
      ],
      "stack": "gvisor",
      "strict_route": true,
      "tag": "tun-in",
      "type": "tun"
    }
  ],
  "log": {
    "level": "info",
    "output": "C:/ProgramData/SplitLane/logs/core.log",
    "timestamp": true
  },
  "outbounds": [
    {
      "domain_resolver": "dns-local",
      "tag": "direct",
      "type": "direct"
    }
  ],
  "route": {
    "auto_detect_interface": true,
    "default_domain_resolver": "dns-local",
    "final": "vpn",
    "find_process": true,
    "rule_set": [
      {
        "format": "source",
        "path": "C:/ProgramData/SplitLane/rules/lane-direct.json",
        "tag": "lane-direct",
        "type": "local"
      },
      {
        "format": "source",
        "path": "C:/ProgramData/SplitLane/rules/lane-vpn.json",
        "tag": "lane-vpn",
        "type": "local"
      }
    ],
    "rules": [
      {
        "action": "reject",
        "ip_version": 6
      },
      {
        "action": "route",
        "inbound": [
          "probe-vpn"
        ],
        "outbound": "vpn"
      },
      {
        "action": "route",
        "inbound": [
          "probe-direct"
        ],
        "outbound": "direct"
      },
      {
        "action": "hijack-dns",
        "port": 53
      },
      {
        "action": "reject",
        "ip_cidr": [
          "172.19.0.0/30"
        ]
      },
      {
        "action": "route",
        "outbound": "direct",
        "process_path_regex": [
          "(?i)^C:\\\\Program Files\\\\Tailscale\\\\tailscaled\\.exe$",
          "(?i)^C:\\\\Program Files\\\\Tailscale\\\\tailscale-ipn\\.exe$"
        ]
      },
      {
        "action": "route",
        "outbound": "vpn",
        "process_path_regex": [
          "(?i)^C:\\\\Program Files\\\\SplitLane\\\\splitlane-helper\\.exe$"
        ]
      },
      {
        "action": "route",
        "ip_is_private": true,
        "outbound": "direct"
      },
      {
        "action": "route",
        "clash_mode": "alldirect",
        "outbound": "direct"
      },
      {
        "action": "route",
        "outbound": "direct",
        "rule_set": [
          "lane-direct"
        ]
      },
      {
        "action": "route",
        "outbound": "vpn",
        "rule_set": [
          "lane-vpn"
        ]
      },
      {
        "action": "route",
        "clash_mode": "direct",
        "outbound": "direct"
      },
      {
        "action": "route",
        "clash_mode": "direct-localdns",
        "outbound": "direct"
      },
      {
        "action": "route",
        "clash_mode": "vpn",
        "outbound": "vpn"
      },
      {
        "action": "route",
        "clash_mode": "vpn-localdns",
        "outbound": "vpn"
      }
    ]
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

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/coreconfig/
```

Ожидается: `FAIL splitlane/helper/internal/coreconfig [build failed]` и `undefined: Options`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/coreconfig/config.go`, целиком:

```go
// Package coreconfig builds the config of sing-box 1.14.4 (spec 6). The helper
// alone builds it, from checked data; nothing in it comes from the window as is.
package coreconfig

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"

	"splitlane/helper/internal/rules"
)

// Tags, names and the core's log lines the helper relies on (Plan 0, "Факты о ядре 1.14.4").
const (
	TagProbeVPN    = "probe-vpn"
	TagProbeDirect = "probe-direct"
	TagTun         = "tun-in"
	TagVPN         = "vpn"
	TagDirect      = "direct"
	RuleSetDirect  = "lane-direct"
	RuleSetVPN     = "lane-vpn"
	AdapterName    = "SplitLane"

	ModeVPN            = "vpn"
	ModeVPNLocalDNS    = "vpn-localdns"
	ModeDirect         = "direct"
	ModeDirectLocalDNS = "direct-localdns"
	ModeAllDirect      = "alldirect"

	// StartLine is written by the core when it has started (spec 5.3 step 4).
	StartLine = "sing-box started"
	// LoginLine means the embedded node needs a login (spec 5.3 step 4, 7).
	LoginLine = "Waiting for authentication"
	// ReloadErrorLine is written when a rule file could not be read again (spec 6.5, 8.3).
	ReloadErrorLine = "reload rule-set lane-"
)

// Modes are the five clash modes of spec 6.3.
var Modes = []string{ModeVPN, ModeVPNLocalDNS, ModeDirect, ModeDirectLocalDNS, ModeAllDirect}

// ModeFor is the clash mode for the main lane, the "all direct" switch and the degraded phase (spec 6.3).
func ModeFor(defaultLane string, allDirect, degraded bool) string {
	switch {
	case allDirect:
		return ModeAllDirect
	case defaultLane == TagDirect && degraded:
		return ModeDirectLocalDNS
	case defaultLane == TagDirect:
		return ModeDirect
	case degraded:
		return ModeVPNLocalDNS
	default:
		return ModeVPN
	}
}

// MainLane is the outbound that rule 11 of spec 6.2 gives to a program without a rule in this mode.
func MainLane(mode string) string {
	if mode == ModeVPN || mode == ModeVPNLocalDNS {
		return TagVPN
	}
	return TagDirect
}

// Development-only pin (Plan 1, safety rule 4): while the developer's AI assistant reaches its
// service only through the VPN, its programs and the service addresses stay on the VPN lane.
// The product never sets Options.DevPin.
var (
	DevPinProcesses = []string{"claude.exe", "bun.exe", "node.exe", "python.exe", "pythonw.exe"}
	DevPinDomains   = []string{"anthropic.com", "claude.ai", "claude.com"}
	DevPinCIDR      = []string{"160.79.104.0/23"}
)

// Ports of one core run: two probe inbounds, the Clash API and the API service (spec 6.1).
type Ports struct {
	ProbeVPN    int
	ProbeDirect int
	Clash       int
	API         int
}

// Options are the inputs of one config.
type Options struct {
	Mode         string // clash_api.default_mode: the mode needed at start (spec 6.3)
	DataDir      string // %ProgramData%\SplitLane or the development data folder
	HelperPath   string // full path of splitlane-helper.exe (rule 6)
	TailscaleDir string // install folder of the system Tailscale client (rule 5)
	Hostname     string // <PC>-splitlane
	ExitNode     string // IPv4 address of the exit node in the tailnet (spec 7)
	Ports        Ports
	ClashSecret  string
	APISecret    string
	DevPin       bool
}

// Paths are the files of the data folder that the config names.
type Paths struct {
	RulesDir    string
	RulesDirect string
	RulesVPN    string
	LogsDir     string
	CoreLog     string
	CoreStdout  string
	StateDir    string
	Config      string
}

// PathsFor lists the files under a data folder (spec 4.2).
func PathsFor(dataDir string) Paths {
	rulesDir := filepath.Join(dataDir, "rules")
	logs := filepath.Join(dataDir, "logs")
	return Paths{
		RulesDir:    rulesDir,
		RulesDirect: filepath.Join(rulesDir, "lane-direct.json"),
		RulesVPN:    filepath.Join(rulesDir, "lane-vpn.json"),
		LogsDir:     logs,
		CoreLog:     filepath.Join(logs, "core.log"),
		CoreStdout:  filepath.Join(logs, "core-stdout.log"),
		StateDir:    filepath.Join(dataDir, "tailscale"),
		Config:      filepath.Join(dataDir, "core-config.json"),
	}
}

type obj = map[string]any

func route(outbound string, match obj) obj {
	match["action"] = "route"
	match["outbound"] = outbound
	return match
}

// Build returns the config as indented JSON (UTF-8 without BOM, keys sorted).
func Build(o Options) ([]byte, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	tsDir := o.TailscaleDir
	if tsDir == "" {
		tsDir = `C:\Program Files\Tailscale`
	}
	routeRules := []any{
		obj{"ip_version": 6, "action": "reject"},
		route(TagVPN, obj{"inbound": []string{TagProbeVPN}}),
		route(TagDirect, obj{"inbound": []string{TagProbeDirect}}),
		obj{"port": 53, "action": "hijack-dns"},
		obj{"ip_cidr": []string{"172.19.0.0/30"}, "action": "reject"},
		route(TagDirect, obj{"process_path_regex": []string{
			rules.BuiltinExpression(filepath.Join(tsDir, "tailscaled.exe")),
			rules.BuiltinExpression(filepath.Join(tsDir, "tailscale-ipn.exe")),
		}}),
		route(TagVPN, obj{"process_path_regex": []string{rules.BuiltinExpression(o.HelperPath)}}),
		route(TagDirect, obj{"ip_is_private": true}),
	}
	if o.DevPin {
		routeRules = append(routeRules,
			route(TagVPN, obj{"process_name": DevPinProcesses}),
			route(TagVPN, obj{"domain_suffix": DevPinDomains, "ip_cidr": DevPinCIDR}))
	}
	routeRules = append(routeRules,
		route(TagDirect, obj{"clash_mode": ModeAllDirect}),
		route(TagDirect, obj{"rule_set": []string{RuleSetDirect}}),
		route(TagVPN, obj{"rule_set": []string{RuleSetVPN}}),
		route(TagDirect, obj{"clash_mode": ModeDirect}),
		route(TagDirect, obj{"clash_mode": ModeDirectLocalDNS}),
		route(TagVPN, obj{"clash_mode": ModeVPN}),
		route(TagVPN, obj{"clash_mode": ModeVPNLocalDNS}),
	)
	localDNS := func(mode string) obj {
		return obj{"clash_mode": mode, "action": "route", "server": "dns-local"}
	}
	config := obj{
		// a log written to a file carries no color codes (spec 6.1)
		"log": obj{"level": "info", "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{
			"servers": []any{
				obj{"type": "local", "tag": "dns-local"},
				obj{"type": "https", "tag": "dns-vpn", "server": "1.1.1.1", "detour": TagVPN},
			},
			"rules":           []any{localDNS(ModeVPNLocalDNS), localDNS(ModeDirectLocalDNS), localDNS(ModeAllDirect)},
			"final":           "dns-vpn",
			"strategy":        "ipv4_only",
			"reverse_mapping": true,
		},
		"inbounds": []any{
			obj{"type": "mixed", "tag": TagProbeVPN, "listen": "127.0.0.1", "listen_port": o.Ports.ProbeVPN},
			obj{"type": "mixed", "tag": TagProbeDirect, "listen": "127.0.0.1", "listen_port": o.Ports.ProbeDirect},
			obj{
				"type":                  "tun",
				"tag":                   TagTun,
				"interface_name":        AdapterName,
				"address":               []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route":            true,
				"strict_route":          true,
				"route_exclude_address": []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"},
				"stack":                 "gvisor",
			},
		},
		"endpoints": []any{obj{
			"type":            "tailscale",
			"tag":             TagVPN,
			"state_directory": filepath.ToSlash(p.StateDir),
			"hostname":        o.Hostname,
			"exit_node":       o.ExitNode,
		}},
		"outbounds": []any{obj{"type": "direct", "tag": TagDirect, "domain_resolver": "dns-local"}},
		"route": obj{
			"rules": routeRules,
			"rule_set": []any{
				obj{"type": "local", "tag": RuleSetDirect, "format": "source", "path": filepath.ToSlash(p.RulesDirect)},
				obj{"type": "local", "tag": RuleSetVPN, "format": "source", "path": filepath.ToSlash(p.RulesVPN)},
			},
			"final":                   TagVPN,
			"auto_detect_interface":   true,
			"find_process":            true,
			"default_domain_resolver": "dns-local",
		},
		"services": []any{obj{
			"type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": o.Ports.API, "secret": o.APISecret,
		}},
		"experimental": obj{"clash_api": obj{
			"external_controller":                  fmt.Sprintf("127.0.0.1:%d", o.Ports.Clash),
			"secret":                               o.ClashSecret,
			"default_mode":                         o.Mode,
			"access_control_allow_origin":          []string{"http://127.0.0.1"},
			"access_control_allow_private_network": false,
		}},
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (o Options) validate() error {
	known := false
	for _, m := range Modes {
		known = known || m == o.Mode
	}
	if !known {
		return fmt.Errorf("coreconfig: unknown mode %q", o.Mode)
	}
	if addr, err := netip.ParseAddr(o.ExitNode); err != nil || !addr.Is4() {
		return fmt.Errorf("coreconfig: exit node must be an IPv4 address")
	}
	seen := map[int]bool{}
	for _, port := range []int{o.Ports.ProbeVPN, o.Ports.ProbeDirect, o.Ports.Clash, o.Ports.API} {
		if port <= 0 || port > 65535 || seen[port] {
			return fmt.Errorf("coreconfig: ports must be four different numbers 1..65535")
		}
		seen[port] = true
	}
	switch {
	case o.DataDir == "" || !filepath.IsAbs(o.DataDir):
		return fmt.Errorf("coreconfig: data folder must be an absolute path")
	case o.HelperPath == "" || !filepath.IsAbs(o.HelperPath):
		return fmt.Errorf("coreconfig: helper path must be an absolute path")
	case o.Hostname == "":
		return fmt.Errorf("coreconfig: hostname is empty")
	case len(o.ClashSecret) < 16 || len(o.APISecret) < 16:
		return fmt.Errorf("coreconfig: secrets must be at least 16 characters")
	}
	return nil
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе, в том числе с настоящим ядром**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/coreconfig/; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 -v -run TestCoreAcceptsConfigs ./internal/coreconfig/
```

Ожидается `ok  splitlane/helper/internal/coreconfig`, затем `--- PASS: TestCoreAcceptsConfigs` (не `SKIP`). Ядро здесь только читает конфиги командой `check`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/coreconfig
git -C C:\code\router commit -m "feat(helper): sing-box 1.14.4 config with five clash modes and rule order of spec 6.2" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Хранилище: `state.json`, журнал возврата, файлы правил, конфиг

5.2 и 5.7: `state.json` (раскладка, основная дорожка, выбранный exit node, признак «вход выполнен», список exit node, «было включено», идентичность ядра, адрес сервиса проверки) и `restore.json` (адрес exit node системного Tailscale и признак доступа к локальной сети). Всё пишется через `atomicfile`, журнал — с записью на диск до вызова `tailscale set`. Нечитаемый журнал откладывается в `restore.json.bad` и сообщается ошибкой. Каталог `rules\` создаётся один раз и не пересоздаётся (6.5).

**Files:**
- Create: `helper/internal/store/store.go`, `helper/internal/store/store_test.go`

**Interfaces:**
- Consumes: `atomicfile.Write`, `coreconfig.PathsFor`, `protocol.AppRule`, `protocol.ExitNode`.
- Produces: `const DefaultProbeURL = "https://api.ipify.org"`; `type CoreIdentity struct{ PID int; Created int64; Path string }` (JSON `pid`, `created`, `path`); `type State struct{ Version int; Apps []protocol.AppRule; DefaultLane, SelectedExitNode string; LoggedIn bool; ExitNodes []protocol.ExitNode; DeviceName string; WasOn bool; Core *CoreIdentity; ProbeURL string }`; `type Journal struct{ ExitNode string; AllowLAN bool }` (JSON `exitNode`, `allowLan`); `Defaults() State`; `type Dir struct{ Path string }` с методами `LoadState() (State, error)`, `SaveState(State) error`, `LoadJournal() (*Journal, error)`, `SaveJournal(Journal) error`, `DeleteJournal() error`, `WriteRules(direct, vpn []byte) error`, `WriteConfig(config []byte) (string, error)`.

- [ ] **Step 1: Написать тест**

Файл `helper/internal/store/store_test.go`, целиком:

```go
package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"splitlane/helper/internal/protocol"
)

func TestStateRoundTripAndDefaults(t *testing.T) {
	d := Dir{Path: filepath.Join(t.TempDir(), "data")}
	s, err := d.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultLane != protocol.LaneVPN || s.ProbeURL != DefaultProbeURL || s.Apps == nil || s.WasOn {
		t.Fatalf("defaults %+v", s)
	}
	s.LoggedIn, s.WasOn, s.SelectedExitNode = true, true, "100.64.0.1"
	s.Core = &CoreIdentity{PID: 42, Created: 133000000000000000, Path: `C:\Program Files\SplitLane\sing-box.exe`}
	s.Apps = append(s.Apps, protocol.AppRule{ID: "a", Name: "Кириллица", Exe: `C:\x.exe`, Matcher: protocol.Matcher{Kind: "file", Value: `C:\x.exe`}, Lane: "direct"})
	if err := d.SaveState(s); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(d.Path, "state.json"))
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("state.json starts with a BOM")
	}
	got, err := d.LoadState()
	if err != nil || !got.WasOn || got.Core.PID != 42 || got.Apps[0].Name != "Кириллица" {
		t.Fatalf("round trip: %v %+v", err, got)
	}
}

func TestCorruptStateGivesDefaultsAndError(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "state.json"), []byte("{broken"), 0o644)
	s, err := d.LoadState()
	if err == nil || s.DefaultLane != protocol.LaneVPN {
		t.Fatalf("want defaults and an error, got %+v %v", s, err)
	}
}

func TestJournalLifecycle(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if j, err := d.LoadJournal(); j != nil || err != nil {
		t.Fatalf("no journal: %v %v", j, err)
	}
	if err := d.SaveJournal(Journal{ExitNode: "100.64.0.9", AllowLAN: true}); err != nil {
		t.Fatal(err)
	}
	j, err := d.LoadJournal()
	if err != nil || j.ExitNode != "100.64.0.9" || !j.AllowLAN {
		t.Fatalf("journal %+v %v", j, err)
	}
	if err := d.DeleteJournal(); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteJournal(); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
}

func TestUnreadableJournalIsSetAside(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{}"), 0o644)
	if j, err := d.LoadJournal(); j != nil || err == nil {
		t.Fatalf("want an error, got %v %v", j, err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json.bad")); err != nil {
		t.Fatal("restore.json.bad missing")
	}
}

func TestRulesAndConfigFiles(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if err := d.WriteRules([]byte("D\n"), []byte("V\n")); err != nil {
		t.Fatal(err)
	}
	path, err := d.WriteConfig([]byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		filepath.Join(d.Path, "rules", "lane-direct.json"): "D\n",
		filepath.Join(d.Path, "rules", "lane-vpn.json"):    "V\n",
		path: "{}\n",
	} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v", file, got, err)
		}
	}
	for _, sub := range []string{"logs", "tailscale"} {
		if _, err := os.Stat(filepath.Join(d.Path, sub)); err != nil {
			t.Errorf("folder %s missing", sub)
		}
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/store/
```

Ожидается: `FAIL splitlane/helper/internal/store [build failed]` и `undefined: Dir`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/store/store.go`, целиком:

```go
// Package store keeps the helper's files in the data folder: state.json (spec 5.2),
// the restore journal restore.json (spec 5.7), the rule files and the core config.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
)

// DefaultProbeURL answers with the external address (spec 11); it is kept in state.json, not sent over the pipe.
const DefaultProbeURL = "https://api.ipify.org"

// CoreIdentity tells the helper's own core from any other sing-box.exe (spec 5.7).
type CoreIdentity struct {
	PID     int    `json:"pid"`
	Created int64  `json:"created"` // process creation time, FILETIME in 100 ns units
	Path    string `json:"path"`
}

// State is state.json (spec 5.2).
type State struct {
	Version          int                 `json:"version"`
	Apps             []protocol.AppRule  `json:"apps"`
	DefaultLane      string              `json:"defaultLane"`
	SelectedExitNode string              `json:"selectedExitNode"`
	LoggedIn         bool                `json:"loggedIn"`
	ExitNodes        []protocol.ExitNode `json:"exitNodes"`
	DeviceName       string              `json:"deviceName,omitempty"`
	WasOn            bool                `json:"wasOn"`
	Core             *CoreIdentity       `json:"core,omitempty"`
	ProbeURL         string              `json:"probeUrl"`
}

// Journal is restore.json: what to give back to the system Tailscale (spec 5.7).
type Journal struct {
	ExitNode string `json:"exitNode"`
	AllowLAN bool   `json:"allowLan"`
}

// Defaults is the state of a fresh install.
func Defaults() State {
	return State{Version: 1, Apps: []protocol.AppRule{}, DefaultLane: protocol.LaneVPN,
		ExitNodes: []protocol.ExitNode{}, ProbeURL: DefaultProbeURL}
}

// Dir is the data folder: %ProgramData%\SplitLane, or the development data folder.
type Dir struct {
	Path string
}

func (d Dir) state() string   { return filepath.Join(d.Path, "state.json") }
func (d Dir) journal() string { return filepath.Join(d.Path, "restore.json") }

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
	if s.Apps == nil {
		s.Apps = []protocol.AppRule{}
	}
	if s.ExitNodes == nil {
		s.ExitNodes = []protocol.ExitNode{}
	}
	if s.DefaultLane != protocol.LaneDirect {
		s.DefaultLane = protocol.LaneVPN
	}
	if s.ProbeURL == "" {
		s.ProbeURL = DefaultProbeURL
	}
	return s, nil
}

// SaveState writes state.json atomically.
func (d Dir) SaveState(s State) error {
	return d.writeJSON(d.state(), s)
}

// LoadJournal reads restore.json; nil without error when there is none.
// An unreadable journal is renamed to restore.json.bad and reported as an error.
func (d Dir) LoadJournal() (*Journal, error) {
	b, err := os.ReadFile(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(b, &j); err != nil || j.ExitNode == "" {
		_ = os.Rename(d.journal(), d.journal()+".bad")
		return nil, fmt.Errorf("restore.json is unreadable and was set aside as restore.json.bad")
	}
	return &j, nil
}

// SaveJournal writes restore.json atomically and flushed to disk, before `tailscale set` runs (spec 5.7).
func (d Dir) SaveJournal(j Journal) error {
	return d.writeJSON(d.journal(), j)
}

// DeleteJournal removes restore.json; a missing file is not an error.
func (d Dir) DeleteJournal() error {
	err := os.Remove(d.journal())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// WriteRules writes both rule files (spec 6.5). The rules folder is created once and never recreated.
func (d Dir) WriteRules(direct, vpn []byte) error {
	p := coreconfig.PathsFor(d.Path)
	if err := os.MkdirAll(p.RulesDir, 0o755); err != nil {
		return err
	}
	if err := atomicfile.Write(p.RulesDirect, direct); err != nil {
		return err
	}
	return atomicfile.Write(p.RulesVPN, vpn)
}

// WriteConfig writes the core config and returns its path.
func (d Dir) WriteConfig(config []byte) (string, error) {
	p := coreconfig.PathsFor(d.Path)
	if err := os.MkdirAll(p.LogsDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		return "", err
	}
	return p.Config, atomicfile.Write(p.Config, config)
}

func (d Dir) writeJSON(path string, v any) error {
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(b, '\n'))
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/store/
```

Ожидается `ok  splitlane/helper/internal/store`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/store
git -C C:\code\router commit -m "feat(helper): state.json, restore journal, rule files and core config storage" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Автомат: интерфейсы, состояние, выключение и откат 5.4

Пакет `engine` — сердце служебного процесса. В этой задаче: интерфейсы ко всему, что трогает систему (`ports.go`), в том числе драйвер ядра `CoreDriver` из раздела 4 (конфиг, файлы дорожек, имена режимов, процессы, состояние коннектора и ошибка перечитывания по журналу, Clash API): автомат не импортирует `coreconfig` и не разбирает тексты sing-box, единственная реализация — пакет `singbox` (задача 15); структура автомата с публикацией состояния и фоновыми операциями (`engine.go`) и выключение 5.4 (`teardown.go`): возврат системного exit node по журналу с подтверждением, повтор каждые 5 секунд, причина `restore_pending` через 30 секунд с командой ручного восстановления, «Выключить без возврата», остановка ядра штатным сигналом (5 секунд, затем принудительно; сразу принудительно, если сигнал не отправился), ожидание исчезновения адаптера (10 секунд, иначе `adapter_left`). Подмены всех интерфейсов (`fakes_test.go`) пишутся здесь один раз и служат тестам задач 10–14: часы не ждут, а сдвигают время; каждое действие подмен записывается в один упорядоченный список, по нему тесты проверяют порядок шагов. Подменный драйвер говорит как драйвер sing-box (режимы и строки журнала из `coreconfig`, файлы из `rules`), но вместо конфига отдаёт параметры запуска в JSON: тесты видят, что попросил автомат.

Правила автомата, общие для задач 10–14:

- Одна фоновая операция за раз (`op`): включение, выключение, перезапуск, старт. Команды, меняющие состояние, проходят через очередь (`queue`) по одной; переходы `starting` и `stopping` очередь не занимают — в них отвечает таблица 5.1.
- `e.mu` охраняет поля; вызовы систем (`tailscale.exe`, HTTP, процессы) — вне `e.mu`, кроме короткой записи файлов в `save`.
- «Было включено» ставится в `finishOn` (шаг 9), снимается в `finish` (любой переход в `off` и `error`) и в начале выключения по `routing.disable` и `helper.shutdown`.

**Files:**
- Create: `helper/internal/engine/ports.go`, `helper/internal/engine/engine.go`, `helper/internal/engine/teardown.go`, `helper/internal/engine/fakes_test.go`, `helper/internal/engine/teardown_test.go`

**Interfaces:**
- Consumes: `protocol` (задача 5), `rules.Env` (задача 6), `store.State`, `store.Journal`, `store.CoreIdentity`, `store.Defaults` (задача 9); в тестах — `coreconfig.ModeFor`, `coreconfig.StartLine`, `LoginLine`, `ReloadErrorLine` (задача 8) и `rules.Files` (задача 7) для подменного драйвера.
- Produces (`ports.go`): `Clock{Now() time.Time; After(d time.Duration) <-chan time.Time}`; `Store` (методы `store.Dir`); `CoreProc{Identity() store.CoreIdentity; Done() <-chan struct{}; Interrupt() error; Kill() error}`; `Ports{ProbeVPN, ProbeDirect, Clash, API int}`; `CoreOptions{Mode, HelperPath, TailscaleDir, Hostname, ExitNode string; Ports Ports; ClashSecret, APISecret string; DevPin bool}`; `CoreEvents{Started, NeedLogin, ReloadFailed bool}`; `CoreDriver{BuildConfig(CoreOptions) ([]byte, error); LaneFiles(apps []protocol.AppRule, env rules.Env) (direct, vpn []byte, err error); Mode(defaultLane string, allDirect, degraded bool) string; Check(ctx, configPath string) error; Start(configPath string) (CoreProc, error); Find(id store.CoreIdentity) (CoreProc, bool); LogSize() int64; Events(offset int64) CoreEvents; Clash(port int, secret string) ClashAPI}`; `Conn{ID, ProcessPath, Lane string; ByLanes bool}` (`ByLanes` — дорожку соединения выбрали правила 8–11 из 6.2); `ClashAPI{SetMode(ctx, mode string) error; Connections(ctx) ([]Conn, error); CloseConn(ctx, id string) error}`; `SysStatus{Installed, ExitNodeOn bool; ExitNodeIP string}`; `SystemTailscale{Status(ctx) (SysStatus, error); AllowLAN(ctx) (bool, error); SetExitNode(ctx, ip string, allowLAN bool) error; ClearExitNode(ctx) error; RestoreCommand(ip string, allowLAN bool) string; Dir() string}`; `ProbeResult{Reached bool; IP string}`; `Prober{ViaProxy(ctx, url string, port int) ProbeResult; Direct(ctx, url string) ProbeResult}`; `Host{HasDefaultRoute() bool; IPForwarding() bool; DNSOverride() bool; CoreRunning() bool; AdapterPresent() bool; FlushDNS() error; FreePorts(n int) ([]int, error); Secret() string}`; `Settings{HelperPath, Hostname string; DevPin bool; HelperVersion, CoreVersion string}`; `Deps{Clock; Store; Core CoreDriver; Sys; Probe; Host; Rules rules.Env; Settings; Log func(string, ...any); OnState func(protocol.State); OnExit func()}`.
- Produces (`engine.go`, `teardown.go`): `New(d Deps) *Engine`; `(*Engine).State() protocol.State`, `Hello() protocol.HelloResult`, `SetClients(n int)`, `Wait()`; внутренние `load()`, `startOp(kind string, fn func(ctx)) bool`, `setPhase`, `finish`, `finishOn`, `dropJournal`, `publish`, `reason(code string, kv ...string) *protocol.Reason`, `sleep(ctx, d) bool`, `exited(CoreProc) bool`, `localDNS(allDirect, degraded bool) bool` (имена через DNS физического адаптера, 6.4), `flushDNS()`, `restoreSystem()`, `retryRestore(skip bool)`, `stopCore()`, `stopProc(CoreProc)`, `waitAdapterGone() bool`, `teardown() bool`, `beginDisableLocked() bool`, `rollback(why *protocol.Reason, cancelled bool)`; значение `errCancelled`; поля `coreRun{proc, clash, ports, mode, direct, vpn, stopping}` (`mode` — имя режима от драйвера); в `fakes_test.go` — `countSince(h, mark, prefix)` и `systemExitNode(h)` для тестов задач 11–14.

Тесты задачи и что они закрепляют:

| Тест | Ветка |
|---|---|
| `TestDisableRestoresBeforeStoppingTheCore` | 5.4: шаг 1 (возврат, подтверждение, журнал удалён) до шага 2; штатная остановка без принудительной |
| `TestUnconfirmedRestoreKeepsTheCoreUntilSkip` | 5.4 и раздел 10, строка «команда возврата не срабатывает»: ядро не останавливается, попытки каждые 5 с, через 30 с `restore_pending` с командой и текстом ошибки; `skipRestore` — журнал удалён, ядро остановлено, `off` |
| `TestRestoreConfirmedOnALaterAttempt` | 5.4: подтверждение не с первой попытки |
| `TestUsersOwnExitNodeIsKept` | 5.7: выбор пользователя не меняется, журнал удаляется |
| `TestStubbornCoreIsKilled` | 5.4 шаг 2: 5 с, затем принудительно; сигнал не отправился — сразу принудительно |
| `TestAdapterLeft` | 5.4 шаг 3: `error` с причиной `adapter_left` |
| `TestNoJournalNoTailscaleCalls` | без журнала системный Tailscale не вызывается |
| `TestRollbackFinalPhase` | откат после сбоя — `error` с причиной; после отмены — `off`; внутренняя причина отмены не публикуется |

- [ ] **Step 1: Записать интерфейсы**

Интерфейсы — договор между автоматом и реализациями задач 15–17; без них подмены не собираются.

Файл `helper/internal/engine/ports.go`, целиком:

```go
// Package engine is the phase machine of the helper: enabling (spec 5.3),
// disabling and rollback (5.4), supervision (5.5), the system Tailscale (5.6),
// the restore journal (5.7) and the commands of 5.1. Everything that touches the
// system sits behind the interfaces of this file, so the tests run on fakes; the core
// sits behind CoreDriver (spec 4), so the engine does not depend on sing-box.
package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// Clock is time; tests use a clock that does not wait.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Store is the data folder (package store).
type Store interface {
	LoadState() (store.State, error)
	SaveState(store.State) error
	LoadJournal() (*store.Journal, error)
	SaveJournal(store.Journal) error
	DeleteJournal() error
	WriteRules(direct, vpn []byte) error
	WriteConfig(config []byte) (string, error)
}

// CoreProc is one running core process.
type CoreProc interface {
	Identity() store.CoreIdentity
	Done() <-chan struct{}
	Interrupt() error // the graceful signal: CTRL_BREAK_EVENT to its process group (spec 5.4)
	Kill() error
}

// Ports are the random local ports of one core run: two probe inbounds, the core's local API
// and its API service (spec 6.1).
type Ports struct {
	ProbeVPN    int
	ProbeDirect int
	Clash       int
	API         int
}

// CoreOptions are the values of one run that the engine decides (spec 6); the driver adds its
// own paths (data folder, rule files, log).
type CoreOptions struct {
	Mode         string // the core's mode at start, from CoreDriver.Mode (spec 6.3)
	HelperPath   string // full path of splitlane-helper.exe (rule 6 of 6.2)
	TailscaleDir string // install folder of the system Tailscale client (rule 5 of 6.2)
	Hostname     string // <PC>-splitlane
	ExitNode     string // IPv4 address of the exit node in the tailnet (spec 7)
	Ports        Ports
	ClashSecret  string
	APISecret    string
	DevPin       bool // development only (Plan 1, safety rule 4)
}

// CoreEvents is what the core's log says after a byte offset: the connector state (spec 5.3
// step 4, 7) and the reload of a lane file (8.3).
type CoreEvents struct {
	Started      bool // this run of the core wrote its start line
	NeedLogin    bool // the connector asks for a login
	ReloadFailed bool // the core could not read a lane file again
}

// CoreDriver hides the core (spec 4): build the config, apply the lanes (the rule files and the
// mode), get and close connections (ClashAPI), the connector state (Events). Stage 1 has one
// driver, for sing-box 1.14.4 (package singbox). The engine knows nothing of the core's config,
// rule-file format, mode names or log lines: a second core (stage 3) is a second driver.
type CoreDriver interface {
	// BuildConfig returns the config of one run (spec 6).
	BuildConfig(o CoreOptions) ([]byte, error)
	// LaneFiles renders the layout as the two rule files the core re-reads (6.5, 8.2).
	LaneFiles(apps []protocol.AppRule, env rules.Env) (direct, vpn []byte, err error)
	// Mode names the core's mode for the main lane, "all direct" and degraded (6.3).
	Mode(defaultLane string, allDirect, degraded bool) string
	Check(ctx context.Context, configPath string) error
	// Start sets the previous log aside and starts the core; the log then holds only this run.
	Start(configPath string) (CoreProc, error)
	// Find returns the helper's own core left from an earlier run, if it still runs (spec 5.7).
	Find(id store.CoreIdentity) (CoreProc, bool)
	// LogSize is the size of the current core log; Events reads the log after a byte offset.
	LogSize() int64
	Events(offset int64) CoreEvents
	Clash(port int, secret string) ClashAPI
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
type ClashAPI interface {
	// SetMode switches the clash mode and reads it back; an error when the core did not take it (spec 6.3).
	SetMode(ctx context.Context, mode string) error
	Connections(ctx context.Context) ([]Conn, error)
	CloseConn(ctx context.Context, id string) error
}

// SysStatus is what the system Tailscale client says (spec 5.6).
type SysStatus struct {
	Installed  bool
	ExitNodeOn bool
	ExitNodeIP string // IPv4 address of the selected exit node
}

// SystemTailscale is the official client, called by its full path (package systs).
type SystemTailscale interface {
	Status(ctx context.Context) (SysStatus, error)
	AllowLAN(ctx context.Context) (bool, error)
	SetExitNode(ctx context.Context, ip string, allowLAN bool) error
	ClearExitNode(ctx context.Context) error
	RestoreCommand(ip string, allowLAN bool) string
	Dir() string
}

// ProbeResult: Reached is false on a timeout or a connection error; IP is empty when the
// service answered without an address (an HTTP error is not a failure of the link, spec 5.5).
type ProbeResult struct {
	Reached bool
	IP      string
}

// Prober asks the address service which address a request comes from (spec 5.3, 5.5).
type Prober interface {
	ViaProxy(ctx context.Context, url string, port int) ProbeResult // through a probe inbound of the core
	Direct(ctx context.Context, url string) ProbeResult             // the helper's own request: the path through the adapter
}

// Host is the rest of Windows (package winhost).
type Host interface {
	HasDefaultRoute() bool
	IPForwarding() bool
	DNSOverride() bool
	CoreRunning() bool
	AdapterPresent() bool
	FlushDNS() error
	FreePorts(n int) ([]int, error)
	Secret() string
}

// Settings are fixed for the life of the process.
type Settings struct {
	HelperPath    string
	Hostname      string // <PC>-splitlane; state.json deviceName overrides it
	DevPin        bool   // development only, see coreconfig.DevPinProcesses
	HelperVersion string
	CoreVersion   string
}

// Deps are everything the engine uses.
type Deps struct {
	Clock    Clock
	Store    Store
	Core     CoreDriver
	Sys      SystemTailscale
	Probe    Prober
	Host     Host
	Rules    rules.Env
	Settings Settings
	Log      func(format string, args ...any)
	OnState  func(protocol.State) // every change of the state (event "state")
	OnExit   func()               // after helper.shutdown has finished
}
```

- [ ] **Step 2: Записать подмены и тест**

Файл `helper/internal/engine/fakes_test.go`, целиком:

```go
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// Addresses of the fakes (documentation ranges). The exit node lives in the tailnet range.
const (
	exitNode   = "100.64.0.1"
	vpnAddr    = "203.0.113.10"
	directAddr = "198.51.100.20"
	portVPN    = 40001
	portDirect = 40002
)

// recorder keeps one ordered list of everything the fakes were asked to do.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf(format, args...))
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.events...)
}

// index of the first event that starts with prefix, or -1.
func (r *recorder) index(prefix string) int {
	for i, ev := range r.list() {
		if strings.HasPrefix(ev, prefix) {
			return i
		}
	}
	return -1
}

func (r *recorder) count(prefix string) int {
	n := 0
	for _, ev := range r.list() {
		if strings.HasPrefix(ev, prefix) {
			n++
		}
	}
	return n
}

// fakeClock never waits: After moves the time on and fires at once (after a real millisecond,
// so that a loop that waits for the user does not spin).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.now = c.now.Add(d)
	t := c.now
	c.mu.Unlock()
	time.Sleep(time.Millisecond)
	ch := make(chan time.Time, 1)
	ch <- t
	return ch
}

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
		return nil, nil
	}
	j := *s.journal
	return &j, nil
}

func (s *fakeStore) SaveJournal(j store.Journal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journalErr != nil {
		return s.journalErr
	}
	s.rec.add("journal:save %s %v", j.ExitNode, j.AllowLAN)
	s.journal = &j
	return nil
}

func (s *fakeStore) DeleteJournal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.add("journal:delete")
	s.journal = nil
	return nil
}

func (s *fakeStore) hasJournal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.journal != nil
}

func (s *fakeStore) WriteRules(direct, vpn []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rulesErr != nil {
		return s.rulesErr
	}
	s.rec.add("rules:write")
	s.direct, s.vpn = direct, vpn
	if s.onRules != nil {
		s.onRules()
	}
	return nil
}

func (s *fakeStore) WriteConfig(config []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	return `C:\data\core-config.json`, nil
}

func (s *fakeStore) saved() store.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

type fakeProc struct {
	rec             *recorder
	id              store.CoreIdentity
	done            chan struct{}
	once            sync.Once
	ignoreInterrupt bool
	interruptErr    error
}

func (p *fakeProc) Identity() store.CoreIdentity { return p.id }
func (p *fakeProc) Done() <-chan struct{}        { return p.done }
func (p *fakeProc) exit()                        { p.once.Do(func() { close(p.done) }) }
func (p *fakeProc) alive() bool                  { return !exited(p) }

func (p *fakeProc) Interrupt() error {
	p.rec.add("core:interrupt %d", p.id.PID)
	if p.interruptErr != nil {
		return p.interruptErr
	}
	if !p.ignoreInterrupt {
		p.exit()
	}
	return nil
}

func (p *fakeProc) Kill() error {
	p.rec.add("core:kill %d", p.id.PID)
	p.exit()
	return nil
}

// crash ends the process as if it failed on its own.
func (p *fakeProc) crash() {
	p.rec.add("core:crash %d", p.id.PID)
	p.exit()
}

// fakeCore is the core driver of the tests. It speaks like the sing-box driver (modes, log lines
// and rule files from coreconfig and rules); its processes are fakeProc and its config is the
// options as JSON, so the tests see what the engine asked for.
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
	opts      CoreOptions // of the last BuildConfig
}

func (c *fakeCore) BuildConfig(o CoreOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.opts = o
	return json.Marshal(o)
}

func (c *fakeCore) options() CoreOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opts
}

func (c *fakeCore) LaneFiles(apps []protocol.AppRule, env rules.Env) ([]byte, []byte, error) {
	return rules.Files(apps, env)
}

func (c *fakeCore) Mode(defaultLane string, allDirect, degraded bool) string {
	return coreconfig.ModeFor(defaultLane, allDirect, degraded)
}

func (c *fakeCore) Events(offset int64) CoreEvents {
	text := c.logFrom(offset)
	return CoreEvents{
		Started:      strings.Contains(text, coreconfig.StartLine),
		NeedLogin:    strings.Contains(text, coreconfig.LoginLine),
		ReloadFailed: strings.Contains(text, coreconfig.ReloadErrorLine),
	}
}

func (c *fakeCore) Check(ctx context.Context, path string) error {
	c.rec.add("core:check")
	return c.checkErr
}

func (c *fakeCore) Start(path string) (CoreProc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.startErr != nil {
		return nil, c.startErr
	}
	p := &fakeProc{rec: c.rec, id: store.CoreIdentity{PID: 1000 + len(c.procs), Created: int64(len(c.procs) + 1), Path: `C:\core\sing-box.exe`}, done: make(chan struct{})}
	c.procs = append(c.procs, p)
	c.rec.add("core:start %d", p.id.PID)
	c.log = ""
	switch {
	case c.dieEarly > 0:
		c.dieEarly--
		p.exit()
	case c.silent > 0:
		c.silent--
	default:
		c.log = "INFO sing-box started (0.42s)\n"
		if c.needLogin {
			c.log += "INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x\n"
		}
	}
	if c.onStart != nil {
		c.onStart(p)
	}
	return p, nil
}

func (c *fakeCore) Find(id store.CoreIdentity) (CoreProc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.leftover != nil && c.leftover.id == id && c.leftover.alive() {
		return c.leftover, true
	}
	return nil, false
}

func (c *fakeCore) LogSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.log))
}

func (c *fakeCore) logFrom(offset int64) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if offset > int64(len(c.log)) {
		return ""
	}
	return c.log[offset:]
}

func (c *fakeCore) appendLog(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log += line + "\n"
}

func (c *fakeCore) Clash(port int, secret string) ClashAPI { return c.clash }

func (c *fakeCore) last() *fakeProc {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.procs) == 0 {
		return nil
	}
	return c.procs[len(c.procs)-1]
}

func (c *fakeCore) anyAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range append(c.procs, c.leftover) {
		if p != nil && p.alive() {
			return true
		}
	}
	return false
}

type fakeClash struct {
	mu        sync.Mutex
	rec       *recorder
	mode      string
	refuse    map[string]bool // modes the core does not take
	conns     []Conn
	connsErr  error
	reconnect func(c Conn) *Conn // a closed connection comes back like this (nil: it does not)
}

func (k *fakeClash) SetMode(ctx context.Context, mode string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec.add("clash:mode %s", mode)
	if k.refuse[mode] {
		return fmt.Errorf("mode %s not taken", mode)
	}
	k.mode = mode
	return nil
}

func (k *fakeClash) Connections(ctx context.Context) ([]Conn, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]Conn{}, k.conns...), k.connsErr
}

func (k *fakeClash) CloseConn(ctx context.Context, id string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec.add("clash:close %s", id)
	for i, c := range k.conns {
		if c.ID == id {
			k.conns = append(k.conns[:i], k.conns[i+1:]...)
			if k.reconnect != nil {
				if n := k.reconnect(c); n != nil {
					k.conns = append(k.conns, *n)
				}
			}
			break
		}
	}
	return nil
}

func (k *fakeClash) currentMode() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.mode
}

type fakeSys struct {
	mu        sync.Mutex
	rec       *recorder
	store     *fakeStore
	installed bool
	on        bool
	ip        string
	allowLAN  bool
	statusErr error
	setErr    error
	ignoreSet bool // `tailscale set` reports success but nothing changes
	clearErr  error
	allowErr  error
}

func (s *fakeSys) Status(ctx context.Context) (SysStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statusErr != nil {
		return SysStatus{Installed: s.installed}, s.statusErr
	}
	st := SysStatus{Installed: s.installed, ExitNodeOn: s.on}
	if s.on {
		st.ExitNodeIP = s.ip
	}
	return st, nil
}

func (s *fakeSys) AllowLAN(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowLAN, s.allowErr
}

func (s *fakeSys) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.add("sys:set %s %v", ip, allowLAN)
	if s.setErr != nil {
		return s.setErr
	}
	if !s.ignoreSet {
		s.on, s.ip, s.allowLAN = true, ip, allowLAN
	}
	return nil
}

func (s *fakeSys) ClearExitNode(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.store.hasJournal() {
		s.rec.add("sys:clear WITHOUT JOURNAL")
	} else {
		s.rec.add("sys:clear")
	}
	if s.clearErr != nil {
		return s.clearErr
	}
	s.on = false
	return nil
}

func (s *fakeSys) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf("tailscale set --exit-node=%s --exit-node-allow-lan-access=%v", ip, allowLAN)
}

func (s *fakeSys) Dir() string { return `C:\Program Files\Tailscale` }

func (s *fakeSys) set(f func(s *fakeSys)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// fakeProbe answers like the address service: through probe-vpn, probe-direct, and the helper's own request.
type fakeProbe struct {
	mu     sync.Mutex
	vpn    []ProbeResult // consumed one by one, the last one repeats
	direct []ProbeResult
	self   []ProbeResult
	hang   bool // every request hangs until its context ends
}

func next(list *[]ProbeResult) ProbeResult {
	r := (*list)[0]
	if len(*list) > 1 {
		*list = (*list)[1:]
	}
	return r
}

func (p *fakeProbe) ViaProxy(ctx context.Context, url string, port int) ProbeResult {
	p.mu.Lock()
	hang := p.hang
	p.mu.Unlock()
	if hang {
		<-ctx.Done()
		return noAnswer
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if port == portVPN {
		return next(&p.vpn)
	}
	return next(&p.direct)
}

func (p *fakeProbe) Direct(ctx context.Context, url string) ProbeResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return next(&p.self)
}

func (p *fakeProbe) script(vpn, direct, self []ProbeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if vpn != nil {
		p.vpn = vpn
	}
	if direct != nil {
		p.direct = direct
	}
	if self != nil {
		p.self = self
	}
}

var (
	atVPN    = ProbeResult{Reached: true, IP: vpnAddr}
	atDirect = ProbeResult{Reached: true, IP: directAddr}
	noAnswer = ProbeResult{}
)

type fakeHost struct {
	mu           sync.Mutex
	rec          *recorder
	core         *fakeCore
	noRoute      int // so many next checks find no default route; -1: never a route
	forwarding   bool
	dnsOverride  bool
	otherCore    bool
	adapterStuck bool
}

func (h *fakeHost) HasDefaultRoute() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.noRoute < 0 {
		return false
	}
	if h.noRoute > 0 {
		h.noRoute--
		return false
	}
	return true
}

func (h *fakeHost) IPForwarding() bool   { h.mu.Lock(); defer h.mu.Unlock(); return h.forwarding }
func (h *fakeHost) DNSOverride() bool    { h.mu.Lock(); defer h.mu.Unlock(); return h.dnsOverride }
func (h *fakeHost) CoreRunning() bool    { h.mu.Lock(); defer h.mu.Unlock(); return h.otherCore }
func (h *fakeHost) AdapterPresent() bool { return h.adapterStuck || h.core.anyAlive() }

func (h *fakeHost) FlushDNS() error {
	h.rec.add("dns:flush")
	return nil
}

func (h *fakeHost) FreePorts(n int) ([]int, error) {
	return []int{portVPN, portDirect, 40003, 40004}[:n], nil
}

func (h *fakeHost) Secret() string { return "0123456789abcdef0123456789abcdef" }

// harness wires an engine to the fakes. By default: logged in, exit node selected,
// the system client on with the same exit node and LAN access allowed.
type harness struct {
	t      *testing.T
	e      *Engine
	rec    *recorder
	clock  *fakeClock
	store  *fakeStore
	core   *fakeCore
	clash  *fakeClash
	sys    *fakeSys
	probe  *fakeProbe
	host   *fakeHost
	mu     sync.Mutex
	states []protocol.State
	exited bool
}

var testRules = rules.Env{WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`, ProgramFilesX86: `C:\Program Files (x86)`}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, rec: &recorder{}, clock: &fakeClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}}
	h.store = &fakeStore{rec: h.rec, state: store.Defaults()}
	h.store.state.LoggedIn = true
	h.store.state.SelectedExitNode = exitNode
	h.clash = &fakeClash{rec: h.rec, mode: "vpn", refuse: map[string]bool{}}
	h.core = &fakeCore{rec: h.rec, clash: h.clash}
	h.sys = &fakeSys{rec: h.rec, store: h.store, installed: true, on: true, ip: exitNode, allowLAN: true}
	h.probe = &fakeProbe{vpn: []ProbeResult{atVPN}, direct: []ProbeResult{atDirect}, self: []ProbeResult{atVPN}}
	h.host = &fakeHost{rec: h.rec, core: h.core}
	h.e = New(Deps{
		Clock: h.clock, Store: h.store, Core: h.core, Sys: h.sys, Probe: h.probe, Host: h.host,
		Rules: testRules,
		Settings: Settings{HelperPath: `C:\code\router\helper\bin\splitlane-helper.exe`,
			Hostname: "PC-splitlane", HelperVersion: "0.1.0", CoreVersion: "1.14.4"},
		Log: t.Logf,
		OnState: func(s protocol.State) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.states = append(h.states, s)
		},
		OnExit: func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.exited = true
		},
	})
	return h
}

// load puts the fake store into the engine, like the start of the helper does.
func (h *harness) load() { h.e.load() }

// phases is the sequence of published phases without repeats.
func (h *harness) phases() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, s := range h.states {
		if len(out) == 0 || out[len(out)-1] != s.Phase {
			out = append(out, s.Phase)
		}
	}
	return out
}

// sawReason reports whether a published state carried this reason code.
func (h *harness) sawReason(code string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.states {
		if s.Reason != nil && s.Reason.Code == code {
			return true
		}
	}
	return false
}

// waitFor polls cond for up to 5 s of real time.
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; phases %v; events %v", what, h.phases(), h.rec.list())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *harness) waitPhase(phase string) {
	h.t.Helper()
	h.waitFor("phase "+phase, func() bool { return h.e.State().Phase == phase && h.idle() })
}

func (h *harness) idle() bool {
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return h.e.op == nil
}

// running puts the engine into phase on with a live core and a journal, as after step 9 of 5.3.
// The watcher of 5.5 is not started here; the tests of the watcher start it themselves.
func (h *harness) running() *fakeProc {
	h.t.Helper()
	p, _ := h.core.Start(`C:\data\core-config.json`)
	proc := p.(*fakeProc)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.load()
	h.e.mu.Lock()
	r := &coreRun{proc: proc, clash: h.clash, ports: portsForTest(), mode: "vpn"}
	h.e.run = r
	h.e.st.Addresses = protocol.Addresses{VPN: strPtr(vpnAddr), Direct: strPtr(directAddr)}
	h.e.finishOn()
	h.e.mu.Unlock()
	return proc
}

func portsForTest() Ports {
	return Ports{ProbeVPN: portVPN, ProbeDirect: portDirect, Clash: 40003, API: 40004}
}

// countSince counts the events from mark on that start with prefix.
func countSince(h *harness, mark int, prefix string) int {
	n := 0
	for _, ev := range h.rec.list()[mark:] {
		if strings.HasPrefix(ev, prefix) {
			n++
		}
	}
	return n
}

// systemExitNode is the exit node of the fake system client now.
func systemExitNode(h *harness) (bool, string) {
	h.sys.mu.Lock()
	defer h.sys.mu.Unlock()
	return h.sys.on, h.sys.ip
}

func strPtr(s string) *string { return &s }

var errBoom = errors.New("boom")
```

Файл `helper/internal/engine/teardown_test.go`, целиком:

```go
package engine

import (
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

func disable(h *harness) {
	h.e.mu.Lock()
	h.e.beginDisableLocked()
	h.e.mu.Unlock()
}

// 5.4 step 1 before step 2: the exit node is back and confirmed before the core gets its signal.
func TestDisableRestoresBeforeStoppingTheCore(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	set, del, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("journal:delete"), h.rec.index("core:interrupt")
	if set < 0 || del < set || stop < del {
		t.Fatalf("order set=%d delete=%d interrupt=%d: %v", set, del, stop, h.rec.list())
	}
	if h.rec.index("core:kill") >= 0 || p.alive() {
		t.Fatal("a core that obeys the signal must not be killed")
	}
	if h.store.hasJournal() || h.store.saved().WasOn || h.e.State().SystemTailscale.RestoreOnDisable {
		t.Fatal("journal and the was-on flag must be gone")
	}
	if !h.e.State().SystemTailscale.ExitNodeOn {
		t.Fatal("the confirmed restore shows in systemTailscale.exitNodeOn")
	}
	if got := h.phases(); strings.Join(got, ",") != "on,stopping,off" {
		t.Fatalf("phases %v", got)
	}
}

// 5.4 step 1: without a confirmation the core keeps running; after 30 s the reason is restore_pending
// with the manual command; only "disable without restore" (skipRestore) lets the core stop.
func TestUnconfirmedRestoreKeepsTheCoreUntilSkip(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
	disable(h)
	h.waitFor("restore_pending", func() bool {
		s := h.e.State()
		return s.Phase == protocol.PhaseStopping && s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending
	})
	s := h.e.State()
	if !strings.Contains(s.Reason.Params[protocol.ParamCommand], "--exit-node=100.64.0.1 --exit-node-allow-lan-access=true") || s.Reason.Params[protocol.ParamError] == "" {
		t.Fatalf("params %v", s.Reason.Params)
	}
	if !p.alive() || h.rec.index("core:interrupt") >= 0 {
		t.Fatal("the core must not be stopped before the restore is confirmed")
	}
	if h.rec.count("sys:set") < 6 {
		t.Fatalf("attempts every 5 s for 30 s: %d", h.rec.count("sys:set"))
	}
	h.e.retryRestore(true)
	h.waitPhase(protocol.PhaseOff)
	if p.alive() || h.store.hasJournal() {
		t.Fatal("after skip the core stops and the journal is deleted")
	}
}

// A late confirmation needs no user decision.
func TestRestoreConfirmedOnALaterAttempt(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.setErr = errBoom })
	disable(h)
	h.waitFor("two failed attempts", func() bool { return h.rec.count("sys:set") >= 2 })
	h.sys.set(func(s *fakeSys) { s.setErr = nil })
	h.waitPhase(protocol.PhaseOff)
	if h.store.hasJournal() {
		t.Fatal("journal must be deleted after the confirmation")
	}
}

// 5.7: an exit node the user selected himself (phase blocked) stays as it is; the journal goes.
func TestUsersOwnExitNodeIsKept(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("sys:set") >= 0 {
		t.Fatalf("the user's exit node must not be changed: %v", h.rec.list())
	}
	if h.store.hasJournal() {
		t.Fatal("journal must be deleted")
	}
}

// 5.4 step 2: a core that ignores the signal is ended after 5 s; a failed signal ends it at once.
func TestStubbornCoreIsKilled(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	p.ignoreInterrupt = true
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("core:interrupt") < 0 || h.rec.index("core:kill") < h.rec.index("core:interrupt") {
		t.Fatalf("signal first, then the forced end: %v", h.rec.list())
	}

	h = newHarness(t)
	p = h.running()
	p.interruptErr = errBoom
	before := h.clock.Now()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("core:kill") < 0 {
		t.Fatal("a failed signal must end the core at once")
	}
	if waited := h.clock.Now().Sub(before); waited >= stopWait {
		t.Fatalf("no 5 s wait after a failed signal, waited %s", waited)
	}
}

// 5.4 step 3: an adapter that stays for 10 s gives error with reason adapter_left.
func TestAdapterLeft(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.host.adapterStuck = true
	disable(h)
	h.waitPhase(protocol.PhaseError)
	if r := h.e.State().Reason; r == nil || r.Code != protocol.ReasonAdapterLeft {
		t.Fatalf("reason %+v", r)
	}
}

// Without a journal the system Tailscale is not touched at all.
func TestNoJournalNoTailscaleCalls(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.store.journal = nil
	h.e.mu.Lock()
	h.e.journal = nil
	h.e.mu.Unlock()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("sys:") >= 0 {
		t.Fatalf("no tailscale call expected: %v", h.rec.list())
	}
}

// rollback after a failure ends in error with the reason; after a cancel it ends in off.
func TestRollbackFinalPhase(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.e.rollback(reason(protocol.ReasonTunnelNotProven), false)
	if s := h.e.State(); s.Phase != protocol.PhaseError || s.Reason.Code != protocol.ReasonTunnelNotProven {
		t.Fatalf("state %+v", s)
	}
	h = newHarness(t)
	h.running()
	h.e.rollback(errCancelled, true)
	if s := h.e.State(); s.Phase != protocol.PhaseOff || s.Reason != nil {
		t.Fatalf("state %+v", s)
	}
	if h.sawReason("cancelled") {
		t.Fatal("the internal cancel reason must never be published")
	}
}
```

- [ ] **Step 3: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/
```

Ожидается: `FAIL splitlane/helper/internal/engine [build failed]` и строки `undefined: exited`, `undefined: Engine`, `undefined: New`.

- [ ] **Step 4: Реализовать автомат и выключение**

Файл `helper/internal/engine/engine.go`, целиком:

```go
package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// Times of spec 5.3–5.7 and 8.3.
const (
	startWait      = 30 * time.Second       // 5.3 step 4
	proveWait      = 45 * time.Second       // 5.3 step 5
	switchWait     = 45 * time.Second       // 5.3 step 8
	routeWait      = 60 * time.Second       // 5.3 step 1, unattended enable
	restoreEvery   = 5 * time.Second        // 5.4 step 1, 5.7
	restorePending = 30 * time.Second       // 5.4 step 1
	stopWait       = 5 * time.Second        // 5.4 step 2
	adapterWait    = 10 * time.Second       // 5.4 step 3
	applyWindow    = 3 * time.Second        // 8.3
	applyEvery     = 250 * time.Millisecond // 8.3
	healthEvery    = 10 * time.Second       // 5.5
	systemEvery    = 3 * time.Second        // 5.6
	callTimeout    = 20 * time.Second       // one call of tailscale.exe
)

// restartPauses: at most three restarts with a growing pause (5.5).
var restartPauses = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// errCancelled ends an operation that routing.disable or helper.shutdown cancelled; it is never published.
var errCancelled = &protocol.Reason{Code: "cancelled", Params: map[string]string{}}

// Engine is the helper's phase machine.
type Engine struct {
	d Deps

	// queue keeps state-changing commands one at a time, in arrival order (5.1).
	queue sync.Mutex

	mu           sync.Mutex
	st           protocol.State
	saved        store.State
	journal      *store.Journal
	run          *coreRun
	op           *op
	degraded     bool
	sysExitOn    bool
	vpnFails     int
	adapterFails int
	leakHits     int
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
	ending       bool         // the Windows session ends (5.4)
	exiting      bool         // helper.shutdown was accepted
	skip         bool         // "disable without restore" (5.4)
	wake         chan struct{}
}

// coreRun is one running core with its ports and the rule files it reads now.
type coreRun struct {
	proc     CoreProc
	clash    ClashAPI
	ports    Ports
	mode     string // the driver's name of the core's mode now
	direct   []byte
	vpn      []byte
	stopping bool // guarded by Engine.mu: the helper stops this core on purpose
}

// op is the one background operation: enabling, disabling, a restart or the start-up.
type op struct {
	kind   string
	cancel context.CancelFunc
	done   chan struct{}
}

// New returns an engine in phase off; Start (start-up of 5.7) is separate.
func New(d Deps) *Engine {
	if d.Log == nil {
		d.Log = func(string, ...any) {}
	}
	if d.OnState == nil {
		d.OnState = func(protocol.State) {}
	}
	if d.OnExit == nil {
		d.OnExit = func() {}
	}
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults()}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
		Apps:        []protocol.AppRule{},
		Tailscale:   protocol.Tailscale{Backend: "stopped", ExitNodes: []protocol.ExitNode{}},
	}
	e.syncSaved()
	return e
}

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

// State is a copy of the current state (5.2).
func (e *Engine) State() protocol.State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st.Clone()
}

// Hello is the answer to hello (5.1).
func (e *Engine) Hello() protocol.HelloResult {
	return protocol.HelloResult{Protocol: protocol.Version, Helper: e.d.Settings.HelperVersion, Core: e.d.Settings.CoreVersion}
}

// SetClients tells how many windows are connected (5.6: the system client is polled for them).
func (e *Engine) SetClients(n int) { e.clients.Store(int32(n)) }

// Wait blocks until no background operation runs.
func (e *Engine) Wait() {
	for {
		e.mu.Lock()
		o := e.op
		e.mu.Unlock()
		if o == nil {
			return
		}
		<-o.done
	}
}

// startOp runs fn in the background; e.mu must be held. It refuses while another operation runs.
func (e *Engine) startOp(kind string, fn func(ctx context.Context)) bool {
	if e.op != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	o := &op{kind: kind, cancel: cancel, done: make(chan struct{})}
	e.op = o
	e.skip = false
	go func() {
		defer func() {
			e.mu.Lock()
			if e.op == o {
				e.op = nil
			}
			e.mu.Unlock()
			cancel()
			close(o.done)
		}()
		fn(ctx)
	}()
	return true
}

// syncSaved copies the saved settings into the published state; e.mu must be held.
func (e *Engine) syncSaved() {
	e.st.DefaultLane = e.saved.DefaultLane
	e.st.Apps = append([]protocol.AppRule{}, e.saved.Apps...)
	e.st.Tailscale.ExitNodes = append([]protocol.ExitNode{}, e.saved.ExitNodes...)
	e.st.Tailscale.SelectedExitNode = nil
	if e.saved.SelectedExitNode != "" {
		v := e.saved.SelectedExitNode
		e.st.Tailscale.SelectedExitNode = &v
	}
	e.st.Tailscale.DeviceName = e.hostname()
	e.st.SystemTailscale.RestoreOnDisable = e.journal != nil
}

func (e *Engine) hostname() string {
	if e.saved.DeviceName != "" {
		return e.saved.DeviceName
	}
	return e.d.Settings.Hostname
}

// save writes state.json; e.mu must be held.
func (e *Engine) save() {
	if err := e.d.Store.SaveState(e.saved); err != nil {
		e.d.Log("state.json not saved: %v", err)
	}
}

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

// setPhase changes phase and reason and publishes; e.mu must be held.
func (e *Engine) setPhase(phase string, why *protocol.Reason) {
	e.st.Phase, e.st.Reason = phase, why
	e.publish()
}

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

// dropJournal removes restore.json; e.mu must be held.
func (e *Engine) dropJournal() {
	if err := e.d.Store.DeleteJournal(); err != nil {
		e.d.Log("restore journal not deleted: %v", err)
	}
	e.journal = nil
	if e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending {
		e.st.Reason = nil
	}
	e.publish()
}

func live(phase string) bool {
	return phase == protocol.PhaseOn || phase == protocol.PhaseDegraded || phase == protocol.PhaseBlocked
}

// localDNS: names resolve through the DNS of the physical adapter (spec 6.4), in degraded and in
// "all direct"; otherwise through the encrypted DNS of the VPN lane.
func localDNS(allDirect, degraded bool) bool { return allDirect || degraded }

// flushDNS empties the Windows DNS cache: at step 7 of 5.3 and at every switch of the DNS, both
// ways (6.4); the core clears only its own cache when the mode changes.
func (e *Engine) flushDNS() {
	if err := e.d.Host.FlushDNS(); err != nil {
		e.d.Log("flush DNS: %v", err)
	}
}

// reason builds a Reason from a code and key/value pairs.
func reason(code string, kv ...string) *protocol.Reason {
	r := &protocol.Reason{Code: code, Params: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		r.Params[kv[i]] = kv[i+1]
	}
	return r
}

// sleep waits d or until ctx ends; false when ctx ended.
func (e *Engine) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-e.d.Clock.After(d):
		return ctx.Err() == nil
	}
}

func exited(p CoreProc) bool {
	select {
	case <-p.Done():
		return true
	default:
		return false
	}
}
```

Файл `helper/internal/engine/teardown.go`, целиком:

```go
package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
)

// restoreSystem is step 1 of 5.4: the system exit node back from the journal, confirmed by
// `tailscale status --json`, tried every 5 s. After 30 s without a confirmation the phase is
// stopping with reason restore_pending. It returns only after the confirmation or after the
// user's explicit "disable without restore"; until then the core is not stopped.
// An exit node the user selected himself in the meantime stays, and the journal goes (5.7).
func (e *Engine) restoreSystem() {
	e.mu.Lock()
	j := e.journal
	e.mu.Unlock()
	if j == nil {
		return
	}
	started := e.d.Clock.Now()
	for {
		e.mu.Lock()
		if e.skip {
			e.skip = false
			e.dropJournal()
			e.mu.Unlock()
			e.d.Log("restore skipped by the user: journal deleted, system exit node left as is")
			return
		}
		e.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		st, statusErr := e.d.Sys.Status(ctx)
		var setErr error
		if statusErr != nil || !st.ExitNodeOn {
			setErr = e.d.Sys.SetExitNode(ctx, j.ExitNode, j.AllowLAN)
			st, statusErr = e.d.Sys.Status(ctx)
		}
		cancel()
		if statusErr == nil && st.ExitNodeOn {
			e.mu.Lock()
			e.st.SystemTailscale.Installed, e.st.SystemTailscale.ExitNodeOn = true, true
			e.dropJournal()
			e.mu.Unlock()
			if st.ExitNodeIP != j.ExitNode {
				e.d.Log("system exit node is on with another node than the journal's; the user's choice stays")
			}
			return
		}
		if e.d.Clock.Now().Sub(started) >= restorePending {
			text := "exit node not confirmed"
			if setErr != nil {
				text = setErr.Error()
			} else if statusErr != nil {
				text = statusErr.Error()
			}
			e.mu.Lock()
			e.setPhase(protocol.PhaseStopping, reason(protocol.ReasonRestorePending,
				protocol.ParamCommand, e.d.Sys.RestoreCommand(j.ExitNode, j.AllowLAN), protocol.ParamError, text))
			e.mu.Unlock()
		}
		select {
		case <-e.wake:
		case <-e.d.Clock.After(restoreEvery):
		}
	}
}

// retryRestore wakes the restore loop for an attempt now ("Повторить"); skip asks it to give up.
func (e *Engine) retryRestore(skip bool) {
	e.mu.Lock()
	if skip {
		e.skip = true
	}
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

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
	e.mu.Lock()
	if e.run == r {
		e.run = nil
	}
	e.mu.Unlock()
}

// stopProc: the graceful signal, at most 5 s, then the forced end; at once when the signal fails.
func (e *Engine) stopProc(p CoreProc) {
	if exited(p) {
		return
	}
	if err := p.Interrupt(); err != nil {
		e.d.Log("graceful stop signal failed: %v; ending the core", err)
		_ = p.Kill()
	} else if !exited(p) {
		select {
		case <-p.Done():
		case <-e.d.Clock.After(stopWait):
			e.d.Log("core did not stop within %s; ending it", stopWait)
			_ = p.Kill()
		}
	}
	if exited(p) {
		return
	}
	select {
	case <-p.Done():
	case <-e.d.Clock.After(stopWait):
		e.d.Log("core still runs after it was ended")
	}
}

// waitAdapterGone is step 3 of 5.4: false when the adapter is still there after 10 s.
func (e *Engine) waitAdapterGone() bool {
	deadline := e.d.Clock.Now().Add(adapterWait)
	for {
		if !e.d.Host.AdapterPresent() {
			return true
		}
		if !e.d.Clock.Now().Before(deadline) {
			return false
		}
		<-e.d.Clock.After(500 * time.Millisecond)
	}
}

// teardown is 5.4 in its order: restore and confirm, stop the core, the adapter gone.
func (e *Engine) teardown() bool {
	e.restoreSystem()
	e.stopCore()
	return e.waitAdapterGone()
}

// beginDisableLocked starts the disabling of 5.4 for routing.disable and helper.shutdown;
// e.mu must be held. "Was on" is cleared at its very beginning (5.2).
func (e *Engine) beginDisableLocked() bool {
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

// rollback is 5.4 after a failure or a cancel: the final phase is error with the reason,
// off after a cancel, error with adapter_left when the adapter stayed.
func (e *Engine) rollback(why *protocol.Reason, cancelled bool) {
	e.mu.Lock()
	e.setPhase(protocol.PhaseStopping, nil)
	e.mu.Unlock()
	ok := e.teardown()
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case !ok:
		e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
	case cancelled || why == errCancelled:
		e.finish(protocol.PhaseOff, nil)
	default:
		e.finish(protocol.PhaseError, why)
	}
}
```

- [ ] **Step 5: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=3 ./internal/engine/
```

Ожидается `ok  splitlane/helper/internal/engine`.

- [ ] **Step 6: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine
git -C C:\code\router commit -m "feat(helper): engine ports, state publishing and the disable/rollback order of 5.4" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Автомат: включение 5.3, поток надзора и перезапуск 5.5

Шаги 1–9 из 5.3 в проверенном порядке: предварительные проверки (при включении без участия пользователя — ожидание маршрута до 60 с); чтение системного Tailscale (`ExitNodeStatus`, признак доступа к локальной сети из `debug prefs`); файлы правил, конфиг, `sing-box check`; запуск ядра и строка запуска именно этого процесса (до 30 с; ядро, вышедшее до строки запуска, — ещё одна попытка с новыми портами); «нужен вход» — откат сразу, признак «вход выполнен» снимается; доказательство туннеля без касания системного VPN (до 45 с; `tunnel_not_proven`, `same_address`); журнал до `tailscale set --exit-node=`; сброс кэша DNS; проверка пути через адаптер (два удачных прохода подряд, до 45 с; два ответа подряд с адреса прямой дорожки — `leak_detected` сразу). Поток надзора (первый пункт 5.5): при неожиданном завершении ядра сразу отдаётся команда возврата, затем перезапуск — не больше трёх раз с паузами 2, 5, 10 с в фазе `starting`; «было включено» не снимается; после трёх неудач — `error` с причиной `core_crashed`. В фазе `blocked` пользователь сам снова выбрал exit node в системном клиенте: нет ни немедленного возврата, ни перезапуска (шаг 6 перезапуска выключил бы этот exit node), а выполняется 5.4 без возврата — журнал удаляется, выбор пользователя остаётся, фаза `error` с причиной `core_crashed` (5.7 важнее 5.5; Review Focus 1). Всё, что зависит от ядра (конфиг, файлы дорожек, режим, строка запуска, просьба войти), автомат получает от `CoreDriver`.

**Files:**
- Create: `helper/internal/engine/enable.go`, `helper/internal/engine/enable_test.go`

**Interfaces:**
- Consumes: всё из задачи 10 (`CoreDriver.LaneFiles`, `Mode`, `BuildConfig` с `CoreOptions`, `Events` — `Started`, `NeedLogin`; `flushDNS`); `store.Journal`; `protocol.ParamError`.
- Produces: `beginEnableLocked(unattended bool) bool`, `runEnable(ctx, unattended bool)`, `enable(ctx, unattended bool) *protocol.Reason`, `preflight`, `startCore(ctx) (*coreRun, *protocol.Reason, bool)`, `attach(*coreRun)`, `loginLost()`, `proveTunnel(ctx, *coreRun) (string, *protocol.Reason)`, `checkSwitch(ctx, *coreRun, directIP string) *protocol.Reason`, `watch(*coreRun)`, `runRestart(ctx, why string)`.

Тесты задачи:

| Тест | Ветка 5.3 / 5.5 / строка раздела 10 |
|---|---|
| `TestEnableInOrder` | 5.3 шаги 3–9 по порядку; журнал до `sys:clear`; адреса, «было включено», идентичность ядра в `state.json`; режим и exit node в конфиге |
| `TestEnableWithoutSystemExitNode` | шаг 2: exit node не выбран, клиент не установлен, служба остановлена — нет журнала и переключения |
| `TestPreflightRefusals` | шаг 1: `not_logged_in`, `no_exit_node`, `no_default_route`, `ip_forwarding`, `other_core`, `system_dns_override`; раздел 10 строка 1: система не тронута |
| `TestUnattendedEnableWaitsForRoute` | шаг 1 без участия пользователя: ожидание маршрута до 60 с |
| `TestCoreDoesNotStart` | раздел 10 строка 1: `config_invalid` (ядро не запускается), `core_start_timeout` (ядро остановлено, система не тронута) |
| `TestEarlyExitIsRetriedOnce` | шаг 4: ядро вышло до строки запуска — одна повторная попытка с новыми портами |
| `TestLoginNeededAfterStart` | шаг 4: «нужен вход» — откат сразу, `login_expired`, признак «вход выполнен» снят |
| `TestTunnelNotProven` | шаг 5 и раздел 10 строка 2: `tunnel_not_proven` (молчит `probe-vpn` или `probe-direct`), `same_address`; системный exit node не тронут |
| `TestSwitchCheckFails` | шаг 8 и раздел 10 строка 3: откат 5.4, возврат до остановки, журнал удалён |
| `TestLeakAtSwitch` | шаг 8: два ответа подряд с прямого адреса — откат сразу; один — не утечка |
| `TestStep6Failures` | шаг 6: журнал не записался — переключения нет; переключение не удалось или признак LAN неизвестен — `switch_check_failed` |
| `TestExistingJournalIsNotOverwritten` | 5.7: существующий журнал не перезаписывается |
| `TestCancelDuringStarting` | `routing.disable` в `starting`: откат 5.4, `off` без ошибки |
| `TestCrashRestoresAtOnceAndRestarts` | 5.5 и раздел 10 строка 4: немедленный возврат, перезапуск в `starting`, «было включено» не снят, журнал хранит значение до первого выключения |
| `TestCrashInBlockedKeepsTheUsersExitNode` | Review Focus 1: сбой в `blocked` — ни `sys:set`, ни `sys:clear`, ни нового ядра; exit node пользователя включён, журнал удалён, `error` с причиной `core_crashed` |
| `TestThreeFailedRestarts` | 5.5: три неудачных перезапуска — `error`, `core_crashed`, exit node возвращён |

- [ ] **Step 1: Написать тест**

Файл `helper/internal/engine/enable_test.go`, целиком:

```go
package engine

import (
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func startEnable(h *harness) {
	h.e.mu.Lock()
	h.e.beginEnableLocked(false)
	h.e.mu.Unlock()
}

func enabled(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	return h
}

func wantError(t *testing.T, h *harness, code string) {
	t.Helper()
	h.waitPhase(protocol.PhaseError)
	if r := h.e.State().Reason; r == nil || r.Code != code {
		t.Fatalf("reason %+v, want %s; events %v", r, code, h.rec.list())
	}
}

// 5.3 in order: rules and config, check, start, tunnel proven, journal before the switch, DNS flush, path check.
func TestEnableInOrder(t *testing.T) {
	h := enabled(t)
	order := []string{"rules:write", "core:check", "core:start", "journal:save 100.64.0.1 true", "sys:clear", "dns:flush"}
	last := -1
	for _, ev := range order {
		i := h.rec.index(ev)
		if i <= last {
			t.Fatalf("%s out of order: %v", ev, h.rec.list())
		}
		last = i
	}
	if h.rec.index("sys:clear WITHOUT JOURNAL") >= 0 {
		t.Fatal("the switch ran before the journal was written")
	}
	s := h.e.State()
	if *s.Addresses.VPN != vpnAddr || *s.Addresses.Direct != directAddr || !s.SystemTailscale.RestoreOnDisable || s.SystemTailscale.ExitNodeOn {
		t.Fatalf("state %+v", s)
	}
	saved := h.store.saved()
	if !saved.WasOn || saved.Core == nil || saved.Core.PID != h.core.last().id.PID {
		t.Fatalf("state.json %+v", saved)
	}
	if o := h.core.options(); o.Mode != "vpn" || o.ExitNode != exitNode || o.Ports != portsForTest() || o.DevPin {
		t.Fatalf("config options %+v", o)
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Without an exit node in the system client (or without the client) nothing is switched or journaled.
func TestEnableWithoutSystemExitNode(t *testing.T) {
	for name, change := range map[string]func(s *fakeSys){
		"exit node off":   func(s *fakeSys) { s.on = false },
		"not installed":   func(s *fakeSys) { s.installed, s.on = false, false },
		"service stopped": func(s *fakeSys) { s.statusErr = errBoom },
	} {
		h := newHarness(t)
		h.sys.set(change)
		h.load()
		startEnable(h)
		h.waitPhase(protocol.PhaseOn)
		if h.rec.index("sys:") >= 0 || h.rec.index("journal:") >= 0 {
			t.Errorf("%s: no journal and no switch expected: %v", name, h.rec.list())
		}
	}
}

// 5.3 step 1 and section 10 row 1: a refusal leaves the system untouched.
func TestPreflightRefusals(t *testing.T) {
	cases := map[string]func(h *harness){
		protocol.ReasonNotLoggedIn:       func(h *harness) { h.store.state.LoggedIn = false },
		protocol.ReasonNoExitNode:        func(h *harness) { h.store.state.SelectedExitNode = "" },
		protocol.ReasonNoDefaultRoute:    func(h *harness) { h.host.noRoute = 1 },
		protocol.ReasonIPForwarding:      func(h *harness) { h.host.forwarding = true },
		protocol.ReasonOtherCore:         func(h *harness) { h.host.otherCore = true },
		protocol.ReasonSystemDNSOverride: func(h *harness) { h.host.dnsOverride = true },
	}
	for code, change := range cases {
		h := newHarness(t)
		change(h)
		h.load()
		startEnable(h)
		wantError(t, h, code)
		if h.rec.index("core:start") >= 0 || h.rec.index("sys:") >= 0 {
			t.Errorf("%s: the system must stay untouched: %v", code, h.rec.list())
		}
	}
}

// Unattended enable waits up to 60 s for a default route.
func TestUnattendedEnableWaitsForRoute(t *testing.T) {
	h := newHarness(t)
	h.host.noRoute = 3
	h.load()
	h.e.mu.Lock()
	h.e.beginEnableLocked(true)
	h.e.mu.Unlock()
	h.waitPhase(protocol.PhaseOn)

	h = newHarness(t)
	h.host.noRoute = -1
	h.load()
	before := h.clock.Now()
	h.e.mu.Lock()
	h.e.beginEnableLocked(true)
	h.e.mu.Unlock()
	wantError(t, h, protocol.ReasonNoDefaultRoute)
	if waited := h.clock.Now().Sub(before); waited < routeWait {
		t.Fatalf("gave up after %s", waited)
	}
}

// Section 10 row 1: the core did not start or its config failed the check.
func TestCoreDoesNotStart(t *testing.T) {
	h := newHarness(t)
	h.core.checkErr = errBoom
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonConfigInvalid)
	if h.rec.index("core:start") >= 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("nothing may start: %v", h.rec.list())
	}

	h = newHarness(t)
	h.core.silent = 1
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonCoreStartTimeout)
	if h.core.last().alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("the silent core must be stopped and the system untouched: %v", h.rec.list())
	}
}

// A core that exits before its start line gets one more start; two such exits are a failure.
func TestEarlyExitIsRetriedOnce(t *testing.T) {
	h := newHarness(t)
	h.core.dieEarly = 1
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if n := h.rec.count("core:start"); n != 2 {
		t.Fatalf("starts %d", n)
	}
	h = newHarness(t)
	h.core.dieEarly = 2
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonCoreStartTimeout)
}

// 5.3 step 4: the node asks for a login: rollback at once, the "logged in" flag goes.
func TestLoginNeededAfterStart(t *testing.T) {
	h := newHarness(t)
	h.core.needLogin = true
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonLoginExpired)
	if h.store.saved().LoggedIn || h.rec.index("sys:clear") >= 0 {
		t.Fatalf("logged-in flag must be cleared, the system untouched: %v", h.rec.list())
	}
}

// Section 10 row 2: the tunnel is not proven at step 5: core stopped, system exit node never touched.
func TestTunnelNotProven(t *testing.T) {
	for name, script := range map[string][2][]ProbeResult{
		"vpn silent":    {{noAnswer}, {atDirect}},
		"direct silent": {{atVPN}, {noAnswer}},
	} {
		h := newHarness(t)
		h.probe.script(script[0], script[1], nil)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonTunnelNotProven)
		if h.rec.index("sys:") >= 0 || h.core.last().alive() {
			t.Errorf("%s: %v", name, h.rec.list())
		}
	}
	h := newHarness(t)
	h.probe.script([]ProbeResult{atDirect}, []ProbeResult{atDirect}, nil)
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
}

// Section 10 row 3: the check after the switch fails: rollback 5.4 restores the exit node first.
func TestSwitchCheckFails(t *testing.T) {
	h := newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{noAnswer})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt")
	if set < 0 || stop < set || h.store.hasJournal() {
		t.Fatalf("restore before stop, journal deleted: %v", h.rec.list())
	}
}

// 5.3: two answers in a row from the direct address are a leak, rolled back at once; one is not.
func TestLeakAtSwitch(t *testing.T) {
	h := newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{atDirect, atDirect, atVPN})
	h.load()
	before := h.clock.Now()
	startEnable(h)
	wantError(t, h, protocol.ReasonLeakDetected)
	if h.clock.Now().Sub(before) >= switchWait {
		t.Fatal("a leak must not wait for the end of the 45 s")
	}
	h = newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{atDirect, atVPN, atVPN})
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
}

// Failures around step 6 never leave the exit node off without a journal.
func TestStep6Failures(t *testing.T) {
	h := newHarness(t)
	h.store.journalErr = errBoom
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.rec.index("sys:clear") >= 0 {
		t.Fatalf("no switch without a journal: %v", h.rec.list())
	}
	h = newHarness(t)
	h.sys.set(func(s *fakeSys) { s.clearErr = errBoom })
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.store.hasJournal() {
		t.Fatal("the journal must be deleted: the exit node is still on")
	}
	h = newHarness(t)
	h.sys.set(func(s *fakeSys) { s.allowErr = errBoom })
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.rec.index("core:start") >= 0 {
		t.Fatal("without the LAN flag nothing may start")
	}
}

// 5.7: while a journal exists, enabling does not overwrite it.
func TestExistingJournalIsNotOverwritten(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: "100.64.0.50", AllowLAN: false}
	h.load()
	h.e.mu.Lock()
	h.e.st.Phase = protocol.PhaseOff
	h.e.mu.Unlock()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	j, _ := h.store.LoadJournal()
	if j == nil || j.ExitNode != "100.64.0.50" || j.AllowLAN || h.rec.index("journal:save") >= 0 {
		t.Fatalf("journal %+v, events %v", j, h.rec.list())
	}
}

// routing.disable during starting cancels the enable: rollback 5.4, phase off, no error.
func TestCancelDuringStarting(t *testing.T) {
	h := newHarness(t)
	h.probe.hang = true
	h.load()
	startEnable(h)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	h.e.mu.Lock()
	h.e.op.cancel()
	h.e.mu.Unlock()
	h.waitPhase(protocol.PhaseOff)
	if h.e.State().Reason != nil || h.core.last().alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("cancel: %+v %v", h.e.State().Reason, h.rec.list())
	}
}

// Section 10 row 4: the core fails while on: the exit node comes back at once, then a restart
// in phase starting; "was on" stays set; the journal keeps the value read before the first switch.
func TestCrashRestoresAtOnceAndRestarts(t *testing.T) {
	h := enabled(t)
	first := h.core.last()
	h.e.mu.Lock()
	r := h.e.run
	h.e.mu.Unlock()
	if r.proc != first {
		t.Fatal("run is not the started core")
	}
	first.crash()
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	crash, set, start := h.rec.index("core:crash"), h.rec.index("sys:set 100.64.0.1 true"), len(h.rec.list())-1
	for i, ev := range h.rec.list() {
		if strings.HasPrefix(ev, "core:start") {
			start = i
		}
	}
	if !(crash < set && set < start) {
		t.Fatalf("restore must come right after the crash, before the new start: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
	if !h.store.saved().WasOn {
		t.Fatal("was-on must stay set through a restart")
	}
	j, _ := h.store.LoadJournal()
	if j == nil || j.ExitNode != exitNode || !j.AllowLAN {
		t.Fatalf("journal %+v", j)
	}
}

// Review Focus 1: a crash in phase blocked. The exit node the user selected himself is neither
// overwritten from the journal nor switched off at step 6 of a restart (5.7): no restart, 5.4
// without a restore (the journal goes), error core_crashed.
func TestCrashInBlockedKeepsTheUsersExitNode(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.mu.Lock()
	h.e.sysExitOn = true
	h.e.setPhase(protocol.PhaseBlocked, nil)
	h.e.mu.Unlock()
	mark := len(h.rec.list())
	h.core.last().crash()
	wantError(t, h, protocol.ReasonCoreCrashed)
	on, ip := systemExitNode(h)
	if countSince(h, mark, "sys:") != 0 || countSince(h, mark, "core:start") != 0 || !on || ip != "100.64.0.99" {
		t.Fatalf("the user's exit node must stay, no restart: %v", h.rec.list())
	}
	if h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("journal deleted, was-on cleared: %v", h.rec.list())
	}
}

// Three failed restarts end in error with reason core_crashed.
func TestThreeFailedRestarts(t *testing.T) {
	h := enabled(t)
	checks := h.rec.count("core:check")
	h.core.mu.Lock()
	h.core.startErr = errBoom
	h.core.mu.Unlock()
	h.core.last().crash()
	wantError(t, h, protocol.ReasonCoreCrashed)
	if n := h.rec.count("core:check") - checks; n != 3 {
		t.Fatalf("restart attempts %d", n)
	}
	if h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatal("after the last failure: exit node restored, was-on cleared")
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/
```

Ожидается: `FAIL splitlane/helper/internal/engine [build failed]` и `h.e.beginEnableLocked undefined (type *Engine has no field or method beginEnableLocked)`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/engine/enable.go`, целиком:

```go
package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// beginEnableLocked starts 5.3 in the background; e.mu must be held.
// unattended: the enable after a failure or at the start of the helper (no route yet is not a refusal).
func (e *Engine) beginEnableLocked(unattended bool) bool {
	e.setPhase(protocol.PhaseStarting, nil)
	return e.startOp("enable", func(ctx context.Context) { e.runEnable(ctx, unattended) })
}

// runEnable is 5.3 with its ending: phase on, or the rollback of 5.4.
func (e *Engine) runEnable(ctx context.Context, unattended bool) {
	why := e.enable(ctx, unattended)
	if why == nil && ctx.Err() == nil {
		e.mu.Lock()
		e.finishOn()
		e.mu.Unlock()
		return
	}
	e.rollback(why, ctx.Err() != nil)
}

// enable is steps 1–8 of 5.3; the order was proven on the PC (appendix A) and does not change.
func (e *Engine) enable(ctx context.Context, unattended bool) *protocol.Reason {
	if why := e.preflight(ctx, unattended); why != nil {
		return why
	}
	// Step 2: is an exit node selected in the system client, and with LAN access?
	sctx, cancel := context.WithTimeout(ctx, callTimeout)
	sys, err := e.d.Sys.Status(sctx)
	if err != nil {
		e.d.Log("system Tailscale status: %v; counted as no exit node (5.6)", err)
		sys = SysStatus{Installed: sys.Installed}
	}
	allowLAN := false
	if sys.ExitNodeOn {
		if sys.ExitNodeIP == "" {
			cancel()
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, "the system exit node has no IPv4 address")
		}
		if allowLAN, err = e.d.Sys.AllowLAN(sctx); err != nil {
			cancel()
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, err.Error())
		}
	}
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
	stop := e.ending || e.exiting
	e.mu.Unlock()
	if stop || ctx.Err() != nil {
		return errCancelled
	}
	// Step 6: the journal first, flushed to disk, then the system exit node off (5.7).
	if sys.ExitNodeOn {
		e.mu.Lock()
		have := e.journal != nil
		e.mu.Unlock()
		if !have {
			j := store.Journal{ExitNode: sys.ExitNodeIP, AllowLAN: allowLAN}
			if err := e.d.Store.SaveJournal(j); err != nil {
				return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, "restore journal: "+err.Error())
			}
			e.mu.Lock()
			e.journal = &j
			e.publish()
			e.mu.Unlock()
		}
		cctx, cancel := context.WithTimeout(ctx, callTimeout)
		err := e.d.Sys.ClearExitNode(cctx)
		cancel()
		if err != nil {
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, err.Error())
		}
		e.mu.Lock()
		e.st.SystemTailscale.Installed, e.st.SystemTailscale.ExitNodeOn = true, false
		e.mu.Unlock()
	}
	// Step 7.
	e.flushDNS()
	// Step 8.
	return e.checkSwitch(ctx, r, directIP)
}

// preflight is step 1 of 5.3.
func (e *Engine) preflight(ctx context.Context, unattended bool) *protocol.Reason {
	e.mu.Lock()
	s := e.saved
	e.mu.Unlock()
	switch {
	case !s.LoggedIn:
		return reason(protocol.ReasonNotLoggedIn)
	case s.SelectedExitNode == "":
		return reason(protocol.ReasonNoExitNode)
	}
	if !e.d.Host.HasDefaultRoute() {
		if !unattended {
			return reason(protocol.ReasonNoDefaultRoute)
		}
		deadline := e.d.Clock.Now().Add(routeWait)
		for !e.d.Host.HasDefaultRoute() {
			if !e.d.Clock.Now().Before(deadline) {
				return reason(protocol.ReasonNoDefaultRoute)
			}
			if !e.sleep(ctx, 2*time.Second) {
				return errCancelled
			}
		}
	}
	switch {
	case e.d.Host.IPForwarding():
		return reason(protocol.ReasonIPForwarding)
	case e.d.Host.CoreRunning():
		return reason(protocol.ReasonOtherCore)
	case e.d.Host.DNSOverride():
		return reason(protocol.ReasonSystemDNSOverride)
	}
	return nil
}

// startCore is steps 3 and 4 of 5.3. retry is true when the core exited before its start line.
func (e *Engine) startCore(ctx context.Context) (*coreRun, *protocol.Reason, bool) {
	e.mu.Lock()
	s := e.saved
	e.mu.Unlock()
	invalid := func(err error) (*coreRun, *protocol.Reason, bool) {
		return nil, reason(protocol.ReasonConfigInvalid, protocol.ParamError, err.Error()), false
	}
	direct, vpn, err := e.d.Core.LaneFiles(s.Apps, e.d.Rules)
	if err != nil {
		return invalid(err)
	}
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		return invalid(err)
	}
	p, err := e.d.Host.FreePorts(4)
	if err != nil {
		return invalid(err)
	}
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
		DevPin: e.d.Settings.DevPin,
	})
	if err != nil {
		return invalid(err)
	}
	path, err := e.d.Store.WriteConfig(config)
	if err != nil {
		return invalid(err)
	}
	if err := e.d.Core.Check(ctx, path); err != nil {
		return invalid(err)
	}
	proc, err := e.d.Core.Start(path)
	if err != nil {
		return nil, reason(protocol.ReasonCoreStartTimeout, protocol.ParamError, err.Error()), false
	}
	r.proc, r.clash = proc, e.d.Core.Clash(r.ports.Clash, clashSecret)
	e.attach(r)
	deadline := e.d.Clock.Now().Add(startWait)
	for {
		ev := e.d.Core.Events(0)
		if ev.NeedLogin {
			e.loginLost()
			return r, reason(protocol.ReasonLoginExpired), false
		}
		if ev.Started {
			return r, nil, false
		}
		if exited(proc) {
			return r, reason(protocol.ReasonCoreStartTimeout, protocol.ParamError, "the core exited before its start line"), true
		}
		if !e.d.Clock.Now().Before(deadline) {
			return r, reason(protocol.ReasonCoreStartTimeout), false
		}
		if !e.sleep(ctx, 250*time.Millisecond) {
			return r, errCancelled, false
		}
	}
}

// attach makes r the current core, records its identity in state.json (5.7) and starts its watcher (5.5).
func (e *Engine) attach(r *coreRun) {
	e.mu.Lock()
	if e.run != nil {
		e.run.stopping = true
	}
	e.run = r
	id := r.proc.Identity()
	e.saved.Core = &id
	e.save()
	e.st.AllDirect = false
	e.mu.Unlock()
	go e.watch(r)
}

// loginLost: the node needs a login; the "logged in" flag goes (5.3 step 4, 7).
func (e *Engine) loginLost() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved.LoggedIn = false
	e.save()
}

// proveTunnel is step 5 of 5.3: probe-vpn answers and its address differs from probe-direct's.
// It returns the address of the direct lane.
func (e *Engine) proveTunnel(ctx context.Context, r *coreRun) (string, *protocol.Reason) {
	e.mu.Lock()
	url := e.saved.ProbeURL
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(proveWait)
	for {
		if e.d.Core.Events(0).NeedLogin {
			e.loginLost()
			return "", reason(protocol.ReasonLoginExpired)
		}
		if exited(r.proc) {
			return "", reason(protocol.ReasonCoreCrashed)
		}
		v := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN)
		d := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect)
		if v.IP != "" && d.IP != "" {
			if v.IP == d.IP {
				return "", reason(protocol.ReasonSameAddress)
			}
			e.mu.Lock()
			e.st.Addresses = protocol.Addresses{VPN: &v.IP, Direct: &d.IP}
			e.publish()
			e.mu.Unlock()
			return d.IP, nil
		}
		if !e.d.Clock.Now().Before(deadline) {
			return "", reason(protocol.ReasonTunnelNotProven)
		}
		if !e.sleep(ctx, time.Second) {
			return "", errCancelled
		}
	}
}

// checkSwitch is step 8 of 5.3: two good answers in a row through the adapter. Two answers in a
// row from the direct lane's address are a leak: the rollback starts at once. One such answer is
// not a leak: the core now and then misses the owner of a connection.
func (e *Engine) checkSwitch(ctx context.Context, r *coreRun, directIP string) *protocol.Reason {
	e.mu.Lock()
	url := e.saved.ProbeURL
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(switchWait)
	good, leaks := 0, 0
	for {
		if exited(r.proc) {
			return reason(protocol.ReasonCoreCrashed)
		}
		res := e.d.Probe.Direct(ctx, url)
		switch {
		case res.IP != "" && res.IP == directIP:
			good, leaks = 0, leaks+1
			if leaks >= 2 {
				return reason(protocol.ReasonLeakDetected)
			}
		case res.IP != "":
			good, leaks = good+1, 0
			if good >= 2 {
				return nil
			}
		default:
			good = 0
		}
		if !e.d.Clock.Now().Before(deadline) {
			return reason(protocol.ReasonSwitchCheckFailed)
		}
		if !e.sleep(ctx, time.Second) {
			return errCancelled
		}
	}
}

// watch is the separate thread of 5.5: on an unexpected exit of the core it gives the system
// exit node back at once (journal present), without waiting for anything, then restarts the core.
// In phase blocked the user has selected an exit node in the system client himself: it is neither
// overwritten from the journal nor switched off at step 6 of a restart (5.7 wins over 5.5 here).
// Then 5.4 runs without a restart: the journal goes, the user's choice stays, error core_crashed.
func (e *Engine) watch(r *coreRun) {
	<-r.proc.Done()
	e.mu.Lock()
	if r.stopping || e.run != r || e.ending || e.exiting {
		e.mu.Unlock()
		return
	}
	j := e.journal
	busy := e.op != nil
	blocked := e.sysExitOn // the user selected an exit node himself: it stays (5.7)
	e.mu.Unlock()
	e.d.Log("core exited unexpectedly")
	if j != nil && !blocked {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		if err := e.d.Sys.SetExitNode(ctx, j.ExitNode, j.AllowLAN); err != nil {
			e.d.Log("immediate restore failed: %v", err)
		}
		cancel()
	}
	if busy {
		return // the running operation sees the exit itself
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != r || e.op != nil || e.exiting || e.ending {
		return
	}
	if blocked {
		e.d.Log("core exited in phase blocked: no restart, the exit node the user selected stays")
		e.setPhase(protocol.PhaseStopping, nil)
		e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonCoreCrashed), false) })
		return
	}
	e.setPhase(protocol.PhaseStarting, nil)
	e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreCrashed) })
}

// runRestart is the restart of 5.5: 5.4 (restore confirmed, core stopped), a pause, then 5.3;
// at most three times with a growing pause, in phase starting; "was on" stays set.
func (e *Engine) runRestart(ctx context.Context, why string) {
	for _, pause := range restartPauses {
		if !e.teardown() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		if !e.sleep(ctx, pause) {
			e.rollback(errCancelled, true)
			return
		}
		if got := e.enable(ctx, true); got == nil && ctx.Err() == nil {
			e.mu.Lock()
			e.finishOn()
			e.mu.Unlock()
			return
		} else if ctx.Err() != nil {
			e.rollback(errCancelled, true)
			return
		} else {
			e.d.Log("restart attempt failed: %s", got.Code)
		}
	}
	e.rollback(reason(why), false)
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=3 ./internal/engine/
```

Ожидается `ok  splitlane/helper/internal/engine`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine
git -C C:\code\router commit -m "feat(helper): enable order of 5.3, core watcher and restarts of 5.5" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: Автомат: старт по журналу 5.7 и завершение сеанса 5.4

`Start` загружает `state.json` и журнал и начинает старт 5.7 в фоне (канал в это время уже работает). Журнал есть — фаза `stopping`, шаг 1 из 5.4 независимо от того, осталось ли ядро; затем своё оставшееся ядро (идентификатор процесса, время создания, путь) останавливается; чужой `sing-box.exe` не трогается. «Было включено» установлено — деление включается заново без участия пользователя. `SessionEnding` — сокращённый порядок 5.4 для завершения сеанса: признак снят, одна команда возврата (не дольше 3 секунд), ядро завершается без ожидания, журнал остаётся до следующего старта; перезапуска после этого нет. В фазе `blocked` команды возврата нет: пользователь снова выбрал exit node в системном клиенте, и по 5.7 этот выбор при возврате не меняется (так же поступают поток надзора задачи 11 и `restoreSystem` задачи 10); журнал остаётся, следующий старт найдёт exit node включённым и удалит журнал.

**Files:**
- Create: `helper/internal/engine/startup.go`, `helper/internal/engine/startup_test.go`

**Interfaces:**
- Consumes: задачи 10 и 11 (`load`, `restoreSystem`, `stopProc`, `waitAdapterGone`, `finish`, `runEnable`).
- Produces: `(*Engine).Start()`, `(*Engine).SessionEnding()`, внутренний `startup(ctx)`.

Тесты задачи:

| Тест | Ветка |
|---|---|
| `TestCleanStartIsOff` | без журнала и без ядра — `off`, никаких действий |
| `TestStartAfterHelperDiedBetweenSteps6And9` | раздел 12: служебный процесс завершён между шагами 6 и 9 — новый экземпляр возвращает exit node по журналу, затем останавливает оставшееся ядро |
| `TestStartAfterCrashWhileOn` | раздел 10 строки «служебный процесс упал» и «отключение питания»: возврат, остановка старого ядра, затем включение заново |
| `TestLeftoverCoreWithoutJournal` | 5.7: ядро без журнала — остановка в `stopping`, Tailscale не вызывается |
| `TestForeignCoreIsNotTouched` | 5.7: чужой `sing-box.exe` не трогается |
| `TestStartWithUnconfirmedRestore` | 5.7: оставшееся ядро работает, пока возврат не подтверждён; `restore_pending` |
| `TestSessionEndWhileOn` | 5.4 и раздел 10 строка «завершение сеанса»: признак снят, одна команда возврата, ядро завершено без ожидания, журнал остаётся, перезапуска нет |
| `TestSessionEndInBlockedKeepsTheUsersExitNode` | 5.4 и 5.7, завершение сеанса в `blocked`: команды возврата нет (выбор пользователя не перезаписывается), ядро завершено, журнал остаётся, признак снят |
| `TestSessionEndWhileOff` | при выключенном делении ничего не делается |

- [ ] **Step 1: Написать тест**

Файл `helper/internal/engine/startup_test.go`, целиком:

```go
package engine

import (
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// leftover puts a still running core of an earlier helper into the fakes and state.json.
func leftover(h *harness) *fakeProc {
	p := &fakeProc{rec: h.rec, id: store.CoreIdentity{PID: 777, Created: 99, Path: `C:\core\sing-box.exe`}, done: make(chan struct{})}
	h.core.leftover = p
	id := p.id
	h.store.state.Core = &id
	return p
}

func TestCleanStartIsOff(t *testing.T) {
	h := newHarness(t)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if len(h.rec.list()) != 0 {
		t.Fatalf("nothing to do at a clean start: %v", h.rec.list())
	}
}

// Section 12: the helper was ended between steps 6 and 9 of 5.3: the new one gives the exit node
// back by the journal first, then stops the core left behind; "was on" was not set yet.
func TestStartAfterHelperDiedBetweenSteps6And9(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() {
		t.Fatalf("restore first, then the old core: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" {
		t.Fatalf("phases %s", got)
	}
	if h.store.saved().Core != nil {
		t.Fatal("the old core identity must be cleared")
	}
}

// The previous helper failed while on ("was on" set): restore, stop the old core, enable again.
func TestStartAfterCrashWhileOn(t *testing.T) {
	h := newHarness(t)
	leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.waitPhase(protocol.PhaseOn)
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" {
		t.Fatalf("phases %s", got)
	}
	if h.rec.index("core:interrupt 777") > h.rec.index("core:start") {
		t.Fatal("the old core must be stopped before the new one starts")
	}
}

// A core left without a journal is stopped in phase stopping; Tailscale is not called.
func TestLeftoverCoreWithoutJournal(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if old.alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("events %v", h.rec.list())
	}
}

// A sing-box.exe that is not ours (other pid or creation time) is never touched.
func TestForeignCoreIsNotTouched(t *testing.T) {
	h := newHarness(t)
	foreign := leftover(h)
	h.store.state.Core = &store.CoreIdentity{PID: 777, Created: 1, Path: `C:\core\sing-box.exe`}
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if !foreign.alive() || h.rec.index("core:") >= 0 {
		t.Fatalf("a foreign core was touched: %v", h.rec.list())
	}
}

// At start, too, the old core keeps running until the restore is confirmed.
func TestStartWithUnconfirmedRestore(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on, s.ignoreSet = false, true })
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if !old.alive() {
		t.Fatal("the old core must keep running")
	}
	h.e.retryRestore(true)
	h.waitPhase(protocol.PhaseOff)
	if old.alive() || h.store.hasJournal() {
		t.Fatal("after skip: journal deleted, core stopped")
	}
}

// 5.4, end of the session: was-on cleared, one restore command, the core ended without a wait,
// the journal kept for the next start, no restart after the core's exit.
func TestSessionEndWhileOn(t *testing.T) {
	h := enabled(t)
	h.e.SessionEnding()
	if h.store.saved().WasOn {
		t.Fatal("was-on must be cleared")
	}
	if h.rec.count("sys:set") != 1 || h.rec.index("core:kill") < h.rec.index("sys:set") || h.rec.index("core:interrupt") >= 0 {
		t.Fatalf("one restore command, then the forced end: %v", h.rec.list())
	}
	if !h.store.hasJournal() {
		t.Fatal("the journal stays until a confirmed restore")
	}
	h.e.Wait()
	if n := h.rec.count("core:start"); n != 1 {
		t.Fatalf("no restart at the end of the session, starts %d", n)
	}
}

// 5.7 at the end of the session in blocked: the exit node the user selected again is not
// overwritten by the journal; the journal stays and the next start deletes it (exit node on).
func TestSessionEndInBlockedKeepsTheUsersExitNode(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.mu.Lock()
	h.e.sysExitOn = true
	h.e.setPhase(protocol.PhaseBlocked, nil)
	h.e.mu.Unlock()
	h.e.SessionEnding()
	if h.rec.count("sys:set") != 0 || h.rec.index("core:kill") < 0 {
		t.Fatalf("no restore command over the user's choice, the core ended: %v", h.rec.list())
	}
	if !h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatal("the journal stays for the next start, was-on is cleared")
	}
	h.e.Wait()
}

func TestSessionEndWhileOff(t *testing.T) {
	h := newHarness(t)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	h.e.SessionEnding()
	if len(h.rec.list()) != 0 {
		t.Fatalf("nothing to do: %v", h.rec.list())
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/
```

Ожидается: `FAIL splitlane/helper/internal/engine [build failed]` и `h.e.Start undefined (type *Engine has no field or method Start)`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/engine/startup.go`, целиком:

```go
package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
)

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
	j, s := e.journal, e.saved
	e.mu.Unlock()
	var left CoreProc
	if s.Core != nil {
		if p, ok := e.d.Core.Find(*s.Core); ok {
			left = p
		}
	}
	if j != nil || left != nil {
		e.mu.Lock()
		e.setPhase(protocol.PhaseStopping, nil)
		e.mu.Unlock()
		e.restoreSystem()
		if left != nil {
			e.d.Log("stopping the core left from the previous run")
			e.stopProc(left)
			if !e.waitAdapterGone() {
				e.mu.Lock()
				e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
				e.mu.Unlock()
				return
			}
		}
	}
	e.mu.Lock()
	e.saved.Core = nil
	wasOn := e.saved.WasOn
	if !wasOn {
		e.finish(protocol.PhaseOff, nil)
		e.mu.Unlock()
		return
	}
	e.setPhase(protocol.PhaseStarting, nil)
	e.mu.Unlock()
	e.runEnable(ctx, true)
}

// sessionEndCall bounds the one restore command at the end of the session: Windows gives about 5 s.
const sessionEndCall = 3 * time.Second

// SessionEnding is the shortened order of 5.4 for the end of the Windows session (logoff,
// restart, shutdown): "was on" cleared, the restore command given once, the core ended without
// waiting for a confirmation. The journal stays and is handled at the next start (5.7).
// In blocked the user has selected an exit node in the system client again: that choice is not
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
	switch {
	case j != nil && blocked:
		e.d.Log("session end: the exit node the user selected stays; the journal waits for the next start")
	case j != nil:
		ctx, cancel := context.WithTimeout(context.Background(), sessionEndCall)
		if err := e.d.Sys.SetExitNode(ctx, j.ExitNode, j.AllowLAN); err != nil {
			e.d.Log("session end: restore command failed: %v", err)
		}
		cancel()
	}
	if r != nil {
		_ = r.proc.Kill()
	}
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=3 ./internal/engine/
```

Ожидается `ok  splitlane/helper/internal/engine`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine
git -C C:\code\router commit -m "feat(helper): start-up by the restore journal and the session-end order" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: Автомат: проверки 5.5 и системный Tailscale 5.6

Раз в 10 секунд — две проверки 5.5. Первая: `probe-vpn`; три неудачи подряд при работающем `probe-direct` — `degraded` с причиной `vpn_unreachable`, режим ядра с запасным DNS и сброс кэша DNS Windows; молчат обе — нет сети, ничего не меняется; ответ с ошибкой HTTP — не неудача; одна удачная проверка возвращает `on` (и режим, и сброс кэша). Вторая (не в `blocked`): путь через адаптер; два ответа подряд с адреса прямой дорожки — откат 5.4 и `error` (`leak_detected`); три неудачи при работающем `probe-vpn` — перезапуск ядра (откат 5.4, затем 5.3; при неудаче — `core_unresponsive`). Запросы одного круга (`probe-vpn`, `probe-direct`, путь через адаптер) идут одновременно, у каждого свой предел 5 секунд. `probe-direct` опрашивается каждый круг, а не только при молчании `probe-vpn`: по 5.5 адреса обновляются при каждой удачной проверке, и проверка утечки сравнивает ответ адаптера с адресом прямой дорожки этого круга (если `probe-direct` не ответил — с последним известным). Иначе после смены адреса у провайдера (продление DHCP, CGNAT, другая сеть) утечка отвечала бы с нового адреса и не была бы замечена. Ответ `probe-direct`, совпавший с ответом `probe-vpn`, — не прямая дорожка (системный exit node снова включён, 5.6): сохранённый адрес остаётся. Пробы идут без очереди, действия — с очередью, чтобы `lanes.set` не ждал сетевых таймаутов. Окно не нужно ни проверкам, ни делению: уход последнего окна меняет только опрос 5.6 при выключенном делении. Раз в 3 секунды — `tailscale status --json` (5.6): пока деление включено и пока подключено окно; выбранный в системном клиенте exit node даёт `blocked` (при обоих условиях — `blocked`). `systemTailscale.clearExitNode` в `blocked` записывает в журнал exit node, выбранный пользователем сейчас, и выключает его.

**Files:**
- Create: `helper/internal/engine/supervise.go`, `helper/internal/engine/supervise_test.go`

**Interfaces:**
- Consumes: задачи 10–12; `CoreDriver.Mode`, `flushDNS`.
- Produces: `(*Engine).Run(ctx)` (две петли: `pollSystem` каждые 3 с, `checkHealth` каждые 10 с, настоящее время), внутренние `checkHealth(ctx)`, `refreshLive()`, `pollSystem(ctx)`, `clearSystemExitNode(ctx)` (вызывается при удержанной очереди).

Тесты задачи:

| Тест | Ветка 5.5 / 5.6 / раздел 10 |
|---|---|
| `TestDegradedAndBack` | строка 5: `degraded` после трёх неудач, режим `vpn-localdns`, сброс кэша; ядро работает; возврат в `on` после одной удачи |
| `TestNoNetworkChangesNothing` | молчат обе пробы — фаза и режим не меняются |
| `TestHTTPErrorIsNotAFailure` | Review Focus 4 |
| `TestLeakWhileOn` | строка 6: два ответа подряд с прямого адреса — откат, `error`; не подряд — не утечка |
| `TestLeakAfterDirectAddressChange` | строка 6 после смены адреса прямой дорожки: адаптер дважды отвечает с нового адреса, который сообщил `probe-direct`, — `leak_detected` |
| `TestAddressesFollowTheChecks` | 5.5: оба адреса обновляются при удачных проверках, прямой — и при работающей дорожке VPN; ответ `probe-direct`, равный ответу `probe-vpn`, адрес не меняет |
| `TestBrokenCoreIsRestarted` | строка 7: три неудачи пути через адаптер при работающем `probe-vpn` — откат 5.4 и включение 5.3 |
| `TestAdapterFailuresNeedAWorkingVPN` | неудачи пути через адаптер при молчащем `probe-vpn` не считаются |
| `TestBlockedPhase` | строка 8 и 5.6: `blocked`, `blocked` важнее `degraded`, в `blocked` путь через адаптер не проверяется, возврат в `degraded` |
| `TestClearSystemExitNodeJournalsUsersChoice` | 5.7: журнал — выбор пользователя в этот момент, затем выключение и сброс кэша |
| `TestPollWhileOffNeedsAWindow` | 5.6: при выключенном делении опрос только для подключённого окна |
| `TestLastWindowGoneKeepsRouting` | раздел 10, строка «окно упало или закрыто»: после ухода последнего окна фаза `on`, ядро работает, ни одного действия с ядром и системным Tailscale |

- [ ] **Step 1: Написать тест**

Файл `helper/internal/engine/supervise_test.go`, целиком:

```go
package engine

import (
	"context"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func rounds(h *harness, n int) {
	for i := 0; i < n; i++ {
		h.e.checkHealth(context.Background())
	}
}

// Section 10 row 5: the VPN server is unreachable: degraded after three failures while probe-direct
// works; DNS through the provider (mode vpn-localdns) and the Windows cache flushed; the core stays.
func TestDegradedAndBack(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	rounds(h, 2)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatal("two failures are not enough")
	}
	rounds(h, 1)
	s := h.e.State()
	if s.Phase != protocol.PhaseDegraded || s.Reason.Code != protocol.ReasonVPNUnreachable {
		t.Fatalf("state %+v", s)
	}
	if h.clash.currentMode() != "vpn-localdns" || h.rec.index("dns:flush") < h.rec.index("clash:mode vpn-localdns") {
		t.Fatalf("mode %s, events %v", h.clash.currentMode(), h.rec.list())
	}
	if !p.alive() {
		t.Fatal("in degraded the core is not stopped")
	}
	h.probe.script([]ProbeResult{atVPN}, nil, []ProbeResult{atVPN})
	rounds(h, 1)
	if h.e.State().Phase != protocol.PhaseOn || h.clash.currentMode() != "vpn" || h.rec.count("dns:flush") != 2 {
		t.Fatalf("back to on: %+v %v", h.e.State(), h.rec.list())
	}
}

// No network at all (both probes silent) is not a VPN failure: nothing changes.
func TestNoNetworkChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	rounds(h, 5)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.index("clash:") >= 0 || h.rec.index("core:start") > 0 {
		t.Fatalf("state %+v events %v", h.e.State(), h.rec.list())
	}
}

// An HTTP error from the address service is not a failure of the link (5.5).
func TestHTTPErrorIsNotAFailure(t *testing.T) {
	h := newHarness(t)
	h.running()
	served := ProbeResult{Reached: true}
	h.probe.script([]ProbeResult{served}, []ProbeResult{atDirect}, []ProbeResult{served})
	rounds(h, 4)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
}

// Section 10 row 6: two answers in a row from the direct address through the adapter: rollback, error.
func TestLeakWhileOn(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script(nil, nil, []ProbeResult{atDirect, atVPN, atDirect, atDirect})
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatal("answers from the direct address that are not in a row are no leak")
	}
	rounds(h, 1)
	wantError(t, h, protocol.ReasonLeakDetected)
	if h.rec.index("sys:set") > h.rec.index("core:interrupt") {
		t.Fatalf("restore before stop: %v", h.rec.list())
	}
}

// Section 10 row 6 after the provider gave a new address (DHCP, CGNAT, another network): probe-direct
// runs every round, so a leak that answers from the new direct address is still caught.
func TestLeakAfterDirectAddressChange(t *testing.T) {
	h := newHarness(t)
	h.running()
	moved := ProbeResult{Reached: true, IP: "198.51.100.77"}
	h.probe.script(nil, []ProbeResult{moved}, []ProbeResult{moved, moved})
	rounds(h, 2)
	wantError(t, h, protocol.ReasonLeakDetected)
}

// 5.5: addresses follow every successful check, the direct one too while the VPN lane works.
// A probe-direct answer equal to probe-vpn's is not the direct lane: the stored one stays.
func TestAddressesFollowTheChecks(t *testing.T) {
	h := newHarness(t)
	h.running()
	vpn, direct := ProbeResult{Reached: true, IP: "203.0.113.77"}, ProbeResult{Reached: true, IP: "198.51.100.77"}
	h.probe.script([]ProbeResult{vpn}, []ProbeResult{direct}, []ProbeResult{vpn})
	rounds(h, 1)
	a := h.e.State().Addresses
	if a.VPN == nil || *a.VPN != vpn.IP || a.Direct == nil || *a.Direct != direct.IP {
		t.Fatalf("addresses %+v", a)
	}
	h.probe.script(nil, []ProbeResult{vpn}, nil)
	rounds(h, 1)
	if a := h.e.State().Addresses; a.Direct == nil || *a.Direct != direct.IP || h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("direct address taken from a VPN answer: %+v", a)
	}
}

// Section 10 row 7: the adapter path fails three times while probe-vpn works: 5.4 then 5.3.
func TestBrokenCoreIsRestarted(t *testing.T) {
	h := newHarness(t)
	old := h.running()
	h.probe.script(nil, nil, []ProbeResult{noAnswer, noAnswer, noAnswer, atVPN})
	rounds(h, 3)
	h.waitFor("restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if old.alive() || h.rec.index("sys:set") > h.rec.index("core:interrupt") {
		t.Fatalf("old core stopped after the restore: %v", h.rec.list())
	}
}

// Failures of the adapter path while probe-vpn fails too are not counted.
func TestAdapterFailuresNeedAWorkingVPN(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	rounds(h, 4)
	if h.rec.count("core:start") != 1 {
		t.Fatalf("no restart expected: %v", h.rec.list())
	}
}

// Section 10 row 8 and 5.6: the user selects an exit node again: blocked; cleared: back.
// With both blocked and degraded the phase is blocked. In blocked the adapter path is not checked.
func TestBlockedPhase(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.pollSystem(context.Background())
	if h.e.State().Phase != protocol.PhaseBlocked {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{atDirect})
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseBlocked {
		t.Fatalf("blocked wins over degraded, and no leak check in blocked: %s", h.e.State().Phase)
	}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.pollSystem(context.Background())
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("after the user cleared it: %s", h.e.State().Phase)
	}
}

// systemTailscale.clearExitNode: the exit node the user selected now goes into the journal.
func TestClearSystemExitNodeJournalsUsersChoice(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip, s.allowLAN = true, "100.64.0.99", false })
	h.e.pollSystem(context.Background())
	h.e.queue.Lock()
	h.e.clearSystemExitNode(context.Background())
	h.e.queue.Unlock()
	j, _ := h.store.LoadJournal()
	if j == nil || *j != (store.Journal{ExitNode: "100.64.0.99", AllowLAN: false}) {
		t.Fatalf("journal %+v", j)
	}
	if h.rec.index("journal:save 100.64.0.99 false") > h.rec.index("sys:clear") || h.rec.index("dns:flush") < 0 {
		t.Fatalf("events %v", h.rec.list())
	}
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
}

// Section 10, "the window crashed or was closed": the last window going away changes nothing in
// the routing; the core keeps running and the checks of 5.5 and 5.6 go on.
func TestLastWindowGoneKeepsRouting(t *testing.T) {
	h := enabled(t)
	core := h.core.last()
	mark := len(h.rec.list())
	h.e.SetClients(1)
	h.e.SetClients(0)
	h.e.pollSystem(context.Background())
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseOn || !core.alive() || countSince(h, mark, "core:") != 0 || countSince(h, mark, "sys:") != 0 {
		t.Fatalf("phase %s, events %v", h.e.State().Phase, h.rec.list())
	}
}

// While routing is off the system client is read only for a connected window (5.6).
func TestPollWhileOffNeedsAWindow(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.e.pollSystem(context.Background())
	if h.e.State().SystemTailscale.Installed {
		t.Fatal("no window: no poll")
	}
	h.e.SetClients(1)
	h.e.pollSystem(context.Background())
	s := h.e.State().SystemTailscale
	if !s.Installed || !s.ExitNodeOn || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("system %+v", s)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/
```

Ожидается: `FAIL splitlane/helper/internal/engine [build failed]` и `h.e.checkHealth undefined (type *Engine has no field or method checkHealth)`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/engine/supervise.go`, целиком:

```go
package engine

import (
	"context"
	"sync"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s.
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	e.every(ctx, healthEvery, e.checkHealth)
}

func (e *Engine) every(ctx context.Context, d time.Duration, fn func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

// checkHealth is one round of 5.5. First: the VPN lane through probe-vpn; three failures in a row
// while probe-direct works give degraded; both silent means no network, nothing changes. Second
// (not in blocked): the path through the adapter; two answers in a row from the direct address are
// a leak (rollback, error); three failures while probe-vpn works mean a broken core (restart).
// The requests of a round run side by side. probe-direct runs every round: the addresses follow
// every successful check (5.5), and the leak check compares with this round's direct address, so
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

	// Probes ran without the queue; acting on them takes it, so that lanes.set is not delayed by them.
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	if e.run != r || e.op != nil || !live(e.st.Phase) {
		e.mu.Unlock()
		return
	}
	before, dirty := e.degraded, false
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
		if e.leakHits >= 2 {
			e.d.Log("traffic leaves through the direct address: rollback")
			e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonLeakDetected), false) })
			e.mu.Unlock()
			return
		}
		if e.adapterFails >= 3 {
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.setPhase(protocol.PhaseStarting, nil)
			e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreUnresponsive) })
			e.mu.Unlock()
			return
		}
	}
	now := e.degraded
	mode, current := e.d.Core.Mode(e.saved.DefaultLane, e.st.AllDirect, now), r.mode
	if dirty {
		e.refreshLive()
	}
	e.mu.Unlock()
	if before != now && mode != current {
		// degraded: names through the provider's DNS (6.3, 6.4); the Windows DNS cache is flushed both ways
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.d.Log("mode %s not set: %v", mode, err)
		} else {
			e.mu.Lock()
			r.mode = mode
			e.mu.Unlock()
		}
		e.flushDNS()
	}
}

// refreshLive sets on, degraded or blocked from the flags and publishes; e.mu must be held.
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

// pollSystem is 5.6: while routing is on, and while a window is connected, the system client is
// read every 3 s; an exit node selected there again gives phase blocked.
func (e *Engine) pollSystem(ctx context.Context) {
	e.mu.Lock()
	watch := live(e.st.Phase) || e.clients.Load() > 0
	e.mu.Unlock()
	if !watch {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	st, err := e.d.Sys.Status(cctx)
	cancel()
	if err != nil {
		st.ExitNodeOn = false // a stopped service or no login counts as no exit node (5.6)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	sys := &e.st.SystemTailscale
	changed := sys.Installed != st.Installed || sys.ExitNodeOn != st.ExitNodeOn
	sys.Installed, sys.ExitNodeOn = st.Installed, st.ExitNodeOn
	if live(e.st.Phase) && e.op == nil && e.run != nil && e.sysExitOn != st.ExitNodeOn {
		e.sysExitOn = st.ExitNodeOn
		e.refreshLive()
		return
	}
	if changed {
		e.publish()
	}
}

// clearSystemExitNode is systemTailscale.clearExitNode in phase blocked: the exit node the user
// selected now goes into the journal (5.7), then it is switched off; the queue must be held.
func (e *Engine) clearSystemExitNode(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	st, err := e.d.Sys.Status(cctx)
	if err != nil || !st.ExitNodeOn || st.ExitNodeIP == "" {
		e.d.Log("clear system exit node: nothing selected (%v)", err)
		return
	}
	allowLAN, err := e.d.Sys.AllowLAN(cctx)
	if err != nil {
		e.d.Log("clear system exit node: LAN flag unknown: %v", err)
		return
	}
	j := store.Journal{ExitNode: st.ExitNodeIP, AllowLAN: allowLAN}
	if err := e.d.Store.SaveJournal(j); err != nil {
		e.d.Log("clear system exit node: journal not written: %v", err)
		return
	}
	e.mu.Lock()
	e.journal = &j
	e.mu.Unlock()
	if err := e.d.Sys.ClearExitNode(cctx); err != nil {
		e.d.Log("clear system exit node: %v", err)
		return
	}
	e.flushDNS()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.SystemTailscale.ExitNodeOn = false
	if live(e.st.Phase) && e.run != nil {
		e.sysExitOn = false
		e.refreshLive()
	}
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=3 ./internal/engine/
```

Ожидается `ok  splitlane/helper/internal/engine`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine
git -C C:\code\router commit -m "feat(helper): health checks of 5.5 and the system Tailscale poll of 5.6" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 14: Автомат: команды по таблице фаз 5.1, раскладка 8.3, «всё напрямую»

`Handle` — единственная точка входа для команд канала. Неизвестный метод — `unknown_method`; `hello`, `state.get` — в любой фазе; `tailscale.login`, `tailscale.logout`, `activity.subscribe`, `diagnostics.export` в этой сборке отвечают `internal` с текстом «not available in this build (Plan 1B)». Остальные идут через очередь и таблицу фаз 5.1 (`wrong_phase`, `busy`, «без действия»). `lanes.set` (8.3): проверка всей раскладки (`bad_request`, `rule_rejected` с полем `rules`); при работающем ядре — запись файлов и режима, затем 3 секунды сверки каждые 250 мс: соединение, чью дорожку решают правила 8–11 (признак `ByLanes` от драйвера) и которое идёт не по своей дорожке, закрывается через API; ошибка перечитывания файла дорожки (`Events.ReloadFailed` после отметки в журнале ядра) или расхождения в конце — прежние файлы и режим возвращаются, `apply_failed`, ничего не сохраняется. `routing.allDirect` — та же сверка; при неудаче прежний режим, признак не меняется; ответ сразу, итог в событии `state`, очередь держится до конца сверки. Включение и выключение «всё напрямую» переключает DNS (6.4: в «всё напрямую» имена идут через DNS физического адаптера), поэтому после каждого такого переключения, и после возврата прежнего режима при неудаче, сбрасывается кэш DNS Windows; в `degraded` DNS уже локальный, переключения DNS и сброса нет. `tailscale.setExitNode` — сохранить; при включённом делении — выключение и повторное включение. `helper.shutdown` — выключение (или откат включения), затем `OnExit`.

**Files:**
- Create: `helper/internal/engine/commands.go`, `helper/internal/engine/commands_test.go`

**Interfaces:**
- Consumes: задачи 10–13; `rules.Check`, `rules.Compile`, `rules.Set`; `protocol.DecodeParams`, `ValidateLanes`, `ValidateExitNodeID`; `CoreDriver.LaneFiles`, `Mode`, `LogSize`, `Events` (`ReloadFailed`); `Conn.Lane`, `Conn.ByLanes`; `localDNS`, `flushDNS`.
- Produces: `(*Engine).Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error)`; внутренние `cmdEnable`, `cmdDisable`, `cmdAllDirect`, `cmdLanesSet`, `applyLanes`, `revert`, `reconcile(ctx, *coreRun, want func(path string) string, mark int64) bool`, `expectedLane(set *rules.Set, mainLane string, allDirect bool, path string) string`, `applyAllDirect(ctx, on bool)`, `cmdSetExitNode`, `cmdClearSystemExitNode`, `cmdShutdown`; в тесте — `applied(h)` (дождаться конца применения, держащего очередь).

Тесты задачи:

| Тест | Строки таблицы 5.1 и 8.3 |
|---|---|
| `TestUnknownAndLaterMethods` | `unknown_method`; методы плана 1B — `internal`; `state.get`; лишний параметр — `bad_request` |
| `TestEnableByPhase` | `routing.enable`: `off`, `error` — включение; `on` — без действия; `degraded` — внеочередная проверка; `stopping` — `busy` |
| `TestDisableByPhase` | `routing.disable`: `off` — `off`; `error` — `off`; `on` — выключение; `starting` — отмена включения |
| `TestDisableInRestorePending` | `stopping` с `restore_pending`: «Повторить» не удаляет журнал; `skipRestore` — выключение без возврата |
| `TestAllDirect` | `routing.allDirect`: `off` — `wrong_phase`; без `on` — `bad_request`; `on` — режим `alldirect` и один сброс кэша DNS после переключения (6.4); при возврате соединение, открытое в «всё напрямую», закрыто (8.3), и ещё один сброс после переключения; `blocked` — `wrong_phase`; режим принят, но сверка не удалась — прежний режим, признак не меняется, два сброса: после переключения и после возврата; в `degraded` включение и выключение — без сброса. Число и порядок событий проверяются только среди событий после отметки перед командой |
| `TestAllDirectNotTakenKeepsTheFlag` | режим не переключился — признак не меняется, прежний режим |
| `TestLanesSetWhileOff` | `off`: проверить и сохранить; `matcher.value` и имя заполнены |
| `TestLanesSetRejections` | неделимость: одно отклонённое правило — ничего не сохранено, поле `rules`; `bad_request`; `starting` — `busy` |
| `TestLanesSetWhileOn` | `on`: файлы, режим, 3 секунды сверки; соединения встроенных правил не трогаются; смена основной дорожки |
| `TestLanesSetConvergesWhileTheCoreReloads` | Review Focus 2 |
| `TestLanesSetFailuresRestorePrevious` | раздел 10 строка «файл правил не применился»: ошибка перечитывания и оставшиеся расхождения — прежние файлы и режим, `apply_failed` |
| `TestCommandsRunOneAtATime` | 5.1: следующая команда ждёт конца сверки |
| `TestSetExitNode` | `off` — сохранить; проверка по списку; `on` — выключение и включение с новым exit node |
| `TestClearSystemExitNodeByPhase` | `off` — `wrong_phase`; `on` — без действия |
| `TestShutdown` | `helper.shutdown` в `off`, `on` (выключение 5.4) и `starting` (откат), затем выход; после — `busy` |
| `TestPhaseTable` | вся таблица 5.1 по клеткам: 8 строк (`routing.enable`, `routing.disable` без `skipRestore` и с ним, `routing.allDirect`, `lanes.set`, `tailscale.setExitNode`, `systemTailscale.clearExitNode`, `helper.shutdown`) × 7 фаз, 56 подтестов; ожидаемые значения — одна таблица, переписанная из 5.1 |

`TestPhaseTable` доводит каждую фазу до настоящей, а не только меняет поле: `starting` — включение ждёт на шаге 5 (проба висит), `stopping` без `restore_pending` — выключение ждёт на шаге 1 из 5.4 (подменный `tailscale.exe` не отвечает на `status`, пока тест его не отпустит), `degraded` — три круга проверок без `probe-vpn`, `blocked` — опрос системного клиента. Для каждой клетки проверяется одно из четырёх: ответ без ошибки и действие таблицы произошло (по событиям подмен и итоговой фазе); ответ без ошибки и ничего не изменилось (список событий и фаза те же); `wrong_phase`; `busy`. `tailscale.login` и `tailscale.logout` в этой сборке отвечают `internal` в любой фазе (`TestUnknownAndLaterMethods`); их строки таблицы — в плане 1B. `routing.disable` с `skipRestore: true` — по строке метода в 5.1 «выключить без возврата системного exit node» в каждой фазе, где он выключает: в `on` и `degraded` журнал удаляется без `tailscale set`, exit node остаётся выключенным; в `blocked` выбор пользователя остаётся и так; в `starting` отменённое включение откатывается без возврата; в `stopping` — «Выключить без возврата» из 5.4. Это явное решение пользователя; в живых шагах плана 1A `-skip-restore` не используется.

- [ ] **Step 1: Написать тест**

Файл `helper/internal/engine/commands_test.go`, целиком:

```go
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"splitlane/helper/internal/protocol"
)

const (
	idA = "11111111-1111-4111-8111-111111111111"
	idB = "22222222-2222-4222-8222-222222222222"
)

var (
	curlA = `C:\Tools\a\curl-a.exe`
	curlB = `C:\Tools\b\curl-b.exe`
)

func call(h *harness, method string, params any) *protocol.Error {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	_, err := h.e.Handle(context.Background(), method, raw)
	return err
}

func wantCode(t *testing.T, err *protocol.Error, code string) {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func fileRule(id, exe, lane string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: protocol.KindFile}, Lane: lane}
}

func setPhase(h *harness, phase string) {
	h.e.mu.Lock()
	h.e.setPhase(phase, nil)
	h.e.mu.Unlock()
}

func TestUnknownAndLaterMethods(t *testing.T) {
	h := newHarness(t)
	wantCode(t, call(h, "routing.toggle", nil), protocol.CodeUnknownMethod)
	for _, m := range []string{protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport} {
		wantCode(t, call(h, m, nil), protocol.CodeInternal)
	}
	res, err := h.e.Handle(context.Background(), protocol.MethodStateGet, nil)
	if err != nil || res.(protocol.State).Phase != protocol.PhaseOff {
		t.Fatalf("state.get: %v %v", res, err)
	}
	wantCode(t, call(h, protocol.MethodEnable, map[string]any{"now": true}), protocol.CodeBadRequest)
}

// routing.enable by phase (5.1).
func TestEnableByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	if err := call(h, protocol.MethodEnable, nil); err != nil || h.rec.count("core:start") != 1 {
		t.Fatalf("in on: no action, got %v", err)
	}
	setPhase(h, protocol.PhaseDegraded)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatalf("in degraded (retry): %v", err)
	}
	setPhase(h, protocol.PhaseStopping)
	wantCode(t, call(h, protocol.MethodEnable, nil), protocol.CodeBusy)

	h = newHarness(t)
	h.load()
	setPhase(h, protocol.PhaseError)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
}

// routing.disable by phase (5.1).
func TestDisableByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodDisable, nil); err != nil || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("off: %v", err)
	}
	h.e.mu.Lock()
	h.e.finish(protocol.PhaseError, reason(protocol.ReasonTunnelNotProven))
	h.e.mu.Unlock()
	if err := call(h, protocol.MethodDisable, nil); err != nil || h.e.State().Phase != protocol.PhaseOff || h.e.State().Reason != nil {
		t.Fatalf("error -> off: %v %+v", err, h.e.State())
	}

	h = enabled(t)
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)

	h = newHarness(t)
	h.probe.hang = true
	h.load()
	call(h, protocol.MethodEnable, nil)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
}

// In stopping with restore_pending: "Повторить" (plain disable) keeps trying, skipRestore gives up.
func TestDisableInRestorePending(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
	call(h, protocol.MethodDisable, nil)
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if err := call(h, protocol.MethodDisable, nil); err != nil || !h.store.hasJournal() {
		t.Fatalf("retry must keep the journal: %v", err)
	}
	if err := call(h, protocol.MethodDisable, protocol.DisableParams{SkipRestore: true}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	if h.store.hasJournal() || h.core.last().alive() {
		t.Fatal("skip: journal deleted, core stopped")
	}
}

// applied waits until a routing.allDirect apply that holds the queue is done.
func applied(h *harness) {
	h.e.queue.Lock()
	h.e.queue.Unlock()
}

// inOrder reports whether the events from mark on hold these prefixes in this order (other events
// may come between them).
func inOrder(h *harness, mark int, prefixes ...string) bool {
	for _, ev := range h.rec.list()[mark:] {
		if len(prefixes) > 0 && strings.HasPrefix(ev, prefixes[0]) {
			prefixes = prefixes[1:]
		}
	}
	return len(prefixes) == 0
}

// routing.allDirect by phase, its reconciliation (8.3) and the DNS cache (6.4).
func TestAllDirect(t *testing.T) {
	h := newHarness(t)
	h.load()
	wantCode(t, call(h, protocol.MethodAllDirect, map[string]bool{"on": true}), protocol.CodeWrongPhase)
	h = enabled(t)
	wantCode(t, call(h, protocol.MethodAllDirect, nil), protocol.CodeBadRequest)
	mark := len(h.rec.list())
	if err := call(h, protocol.MethodAllDirect, map[string]bool{"on": true}); err != nil {
		t.Fatal(err)
	}
	applied(h)
	if !h.e.State().AllDirect || h.clash.currentMode() != "alldirect" {
		t.Fatalf("flag %v mode %s", h.e.State().AllDirect, h.clash.currentMode())
	}
	if countSince(h, mark, "dns:flush") != 1 || !inOrder(h, mark, "clash:mode alldirect", "dns:flush") {
		t.Fatalf("6.4: the switch to the adapter's DNS flushes the Windows cache after it: %v", h.rec.list()[mark:])
	}
	// a connection opened in "all direct" by an unpinned program stays wrong after the return: closed
	h.clash.conns = []Conn{{ID: "c1", ProcessPath: curlA, Lane: "direct", ByLanes: true}}
	mark = len(h.rec.list())
	if err := call(h, protocol.MethodAllDirect, map[string]bool{"on": false}); err != nil {
		t.Fatal(err)
	}
	applied(h)
	if h.e.State().AllDirect || countSince(h, mark, "clash:close c1") != 1 || countSince(h, mark, "dns:flush") != 1 ||
		!inOrder(h, mark, "clash:mode vpn", "dns:flush") {
		t.Fatalf("off: closed c1 and flushed the cache again after the switch: %v", h.rec.list()[mark:])
	}
	setPhase(h, protocol.PhaseBlocked)
	wantCode(t, call(h, protocol.MethodAllDirect, map[string]bool{"on": true}), protocol.CodeWrongPhase)

	// The switch is taken, the reconciliation fails: the previous mode comes back and with it the
	// VPN's DNS, so the cache is flushed after the switch and again after the return.
	h = enabled(t)
	h.clash.conns = []Conn{{ID: "stuck", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	h.clash.reconnect = func(c Conn) *Conn { return &c } // the core keeps it on the VPN lane
	mark = len(h.rec.list())
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	if h.e.State().AllDirect || h.clash.currentMode() != "vpn" || countSince(h, mark, "dns:flush") != 2 ||
		!inOrder(h, mark, "clash:mode alldirect", "dns:flush", "clash:mode vpn", "dns:flush") {
		t.Fatalf("failed apply: the previous mode and two flushes: %v", h.rec.list()[mark:])
	}

	// In degraded the names go through the adapter's DNS already: "all direct" moves no DNS.
	h = enabled(t)
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("setup: phase %s", h.e.State().Phase)
	}
	flushes := h.rec.count("dns:flush")
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	call(h, protocol.MethodAllDirect, map[string]bool{"on": false})
	applied(h)
	if h.rec.count("clash:mode alldirect") != 1 || h.clash.currentMode() != "vpn-localdns" || h.rec.count("dns:flush") != flushes {
		t.Fatalf("degraded: no flush expected: %v", h.rec.list())
	}
}

func TestAllDirectNotTakenKeepsTheFlag(t *testing.T) {
	h := enabled(t)
	h.clash.refuse["alldirect"] = true
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	if h.e.State().AllDirect || h.clash.currentMode() != "vpn" {
		t.Fatalf("flag %v mode %s", h.e.State().AllDirect, h.clash.currentMode())
	}
}

// lanes.set while off: checked, normalized and saved; nothing is applied.
func TestLanesSetWhileOff(t *testing.T) {
	h := newHarness(t)
	h.load()
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{{ID: idA, Exe: curlA, Matcher: protocol.Matcher{Kind: protocol.KindFolder}, Lane: "vpn"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := h.e.State()
	if s.DefaultLane != "direct" || len(s.Apps) != 1 || s.Apps[0].Matcher.Value != `C:\Tools\a\` || s.Apps[0].Name != "curl-a" {
		t.Fatalf("state %+v", s)
	}
	if h.store.saved().DefaultLane != "direct" || h.rec.index("rules:write") >= 0 {
		t.Fatalf("saved %+v, events %v", h.store.saved(), h.rec.list())
	}
}

// lanes.set is atomic: one rejected rule, nothing saved; malformed input is bad_request.
func TestLanesSetRejections(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.e.d.Rules.Protected = []string{`C:\Tools\b\curl-b.exe`}
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct"), fileRule(idB, curlB, "direct")}})
	wantCode(t, err, protocol.CodeRuleRejected)
	if len(err.Rules) != 1 || err.Rules[0] != (protocol.RuleRejection{ID: idB, Reason: protocol.RejectProtected}) {
		t.Fatalf("rules %+v", err.Rules)
	}
	if len(h.store.saved().Apps) != 0 {
		t.Fatal("nothing may be saved")
	}
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule("1", curlA, "direct")}}), protocol.CodeBadRequest)
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, "curl-a.exe", "direct")}}), protocol.CodeBadRequest)
	setPhase(h, protocol.PhaseStarting)
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn"}), protocol.CodeBusy)
}

// lanes.set while on: files and mode, then 3 s of reconciliation: a connection on the wrong lane
// is closed (it comes back on the right one); connections of built-in rules are left alone.
func TestLanesSetWhileOn(t *testing.T) {
	h := enabled(t)
	h.clash.conns = []Conn{
		{ID: "moved", ProcessPath: curlA, Lane: "vpn", ByLanes: true},
		{ID: "probe", ProcessPath: "", Lane: "vpn", ByLanes: false}, // a built-in rule (probe-vpn) chose it
		{ID: "free", ProcessPath: `C:\Other\x.exe`, Lane: "vpn", ByLanes: true},
	}
	h.clash.reconnect = func(c Conn) *Conn {
		return &Conn{ID: c.ID + "-again", ProcessPath: c.ProcessPath, Lane: "direct", ByLanes: true}
	}
	before := h.clock.Now()
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	if err != nil {
		t.Fatal(err)
	}
	if h.rec.index("clash:close moved") < 0 || h.rec.index("clash:close probe") >= 0 || h.rec.index("clash:close free") >= 0 {
		t.Fatalf("events %v", h.rec.list())
	}
	if !strings.Contains(string(h.store.direct), `curl-a\\.exe$`) {
		t.Fatalf("direct file:\n%s", h.store.direct)
	}
	if h.clock.Now().Sub(before) < applyWindow {
		t.Fatal("the reconciliation lasts 3 s")
	}
	if h.store.saved().Apps[0].ID != idA {
		t.Fatal("layout not saved")
	}
	// the main lane: direct makes the mode direct; an unpinned program moves
	h.clash.conns = []Conn{{ID: "free2", ProcessPath: `C:\Other\x.exe`, Lane: "vpn", ByLanes: true}}
	if err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}}); err != nil {
		t.Fatal(err)
	}
	if h.clash.currentMode() != "direct" || h.rec.index("clash:close free2") < 0 {
		t.Fatalf("mode %s events %v", h.clash.currentMode(), h.rec.list())
	}
}

// Review Focus 2: the core reads the new file about a second later; until then a closed connection
// comes back on the old lane. The check closes it again and succeeds once the core has the file.
func TestLanesSetConvergesWhileTheCoreReloads(t *testing.T) {
	h := enabled(t)
	h.clash.conns = []Conn{{ID: "app", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	closes := 0
	h.clash.reconnect = func(c Conn) *Conn {
		closes++
		n := c
		n.ID = fmt.Sprintf("app-%d", closes)
		if closes >= 3 {
			n.Lane = "direct"
		}
		return &n
	}
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	if err != nil || closes != 3 {
		t.Fatalf("err %v, closes %d", err, closes)
	}
}

// Section 10 row "rule file not applied": the previous files and mode come back, apply_failed.
func TestLanesSetFailuresRestorePrevious(t *testing.T) {
	h := enabled(t)
	oldDirect := append([]byte{}, h.store.direct...)
	h.store.onRules = func() { h.core.appendLog("ERROR router: reload rule-set lane-direct: parse error") }
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	wantCode(t, err, protocol.CodeApplyFailed)
	if string(h.store.direct) != string(oldDirect) || h.clash.currentMode() != "vpn" || len(h.store.saved().Apps) != 0 {
		t.Fatalf("not restored: mode %s, saved %+v", h.clash.currentMode(), h.store.saved().Apps)
	}

	h = enabled(t)
	h.clash.conns = []Conn{{ID: "stuck", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	h.clash.reconnect = func(c Conn) *Conn { return &c } // the core keeps the old rules
	err = call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	wantCode(t, err, protocol.CodeApplyFailed)
	if len(h.store.saved().Apps) != 0 {
		t.Fatal("nothing saved after a failed apply")
	}
}

// Commands that change state run one at a time: a second one waits for lanes.set (5.1).
func TestCommandsRunOneAtATime(t *testing.T) {
	h := enabled(t)
	done := make(chan struct{})
	go func() {
		call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn", Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
		close(done)
	}()
	h.waitFor("lanes.set running", func() bool { return h.rec.count("rules:write") == 2 })
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	select {
	case <-done:
	default:
		t.Fatal("routing.allDirect was answered while lanes.set still ran")
	}
}

// tailscale.setExitNode: saved while off; checked against the list; while on: off and on again.
func TestSetExitNode(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.2"}); err != nil {
		t.Fatal(err)
	}
	if h.store.saved().SelectedExitNode != "100.64.0.2" || *h.e.State().Tailscale.SelectedExitNode != "100.64.0.2" {
		t.Fatal("not saved")
	}
	h.e.mu.Lock()
	h.e.saved.ExitNodes = []protocol.ExitNode{{ID: "100.64.0.2", Name: "a", Online: true}}
	h.e.mu.Unlock()
	wantCode(t, call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.3"}), protocol.CodeUnknownExitNode)

	h = enabled(t)
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if h.core.options().ExitNode != "100.64.0.5" {
		t.Fatal("the new core must use the new exit node")
	}
	if h.rec.index("sys:set 100.64.0.1 true") < 0 {
		t.Fatal("the system exit node is restored between the two runs")
	}
}

func TestClearSystemExitNodeByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	wantCode(t, call(h, protocol.MethodClearSystemExitNode, nil), protocol.CodeWrongPhase)
	h = enabled(t)
	if err := call(h, protocol.MethodClearSystemExitNode, nil); err != nil || h.rec.count("sys:clear") != 1 {
		t.Fatalf("in on: no action, got %v", err)
	}
}

// Outcomes of a cell of the phase table: the action of the table happened, or nothing changed;
// any other value is the error code of the answer.
const (
	cellAct  = "act"
	cellNone = "none"
)

// tablePhases are the columns of the phase table of 5.1, one phase each.
var tablePhases = []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
	protocol.PhaseDegraded, protocol.PhaseBlocked, protocol.PhaseStopping}

// slowSys is a tailscale.exe whose `status` answers only after hold is closed.
type slowSys struct {
	*fakeSys
	hold chan struct{}
}

func (s slowSys) Status(ctx context.Context) (SysStatus, error) {
	<-s.hold
	return s.fakeSys.Status(ctx)
}

// inPhase puts a new harness into the phase for real: starting waits in step 5 of 5.3 (a probe
// that hangs), stopping waits in step 1 of 5.4 (a tailscale.exe that does not answer yet; no
// restore_pending). release lets a waiting operation go on and waits until it is done.
func inPhase(t *testing.T, phase string) (*harness, func()) {
	t.Helper()
	var h *harness
	wait := func() {}
	switch phase {
	case protocol.PhaseOff:
		h = newHarness(t)
		h.load()
	case protocol.PhaseError:
		h = newHarness(t)
		h.load()
		h.e.mu.Lock()
		h.e.finish(protocol.PhaseError, reason(protocol.ReasonTunnelNotProven))
		h.e.mu.Unlock()
	case protocol.PhaseStarting:
		h = newHarness(t)
		h.probe.hang = true
		h.load()
		call(h, protocol.MethodEnable, nil)
		h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
		wait = func() { call(h, protocol.MethodDisable, nil) }
	case protocol.PhaseOn:
		h = enabled(t)
	case protocol.PhaseDegraded:
		h = enabled(t)
		h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
		rounds(h, 3)
		h.probe.script([]ProbeResult{atVPN}, nil, nil)
	case protocol.PhaseBlocked:
		h = enabled(t)
		h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
		h.e.pollSystem(context.Background())
	case protocol.PhaseStopping:
		h = enabled(t)
		hold := make(chan struct{})
		h.e.d.Sys = slowSys{h.sys, hold}
		call(h, protocol.MethodDisable, nil)
		wait = func() { close(hold) }
	}
	if got := h.e.State().Phase; got != phase {
		t.Fatalf("setup: phase %s, want %s; events %v", got, phase, h.rec.list())
	}
	var once sync.Once
	return h, func() {
		once.Do(wait)
		h.e.Wait()
	}
}

func exitedNow(h *harness) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exited
}

// disabledAct checks a routing.disable that acted: off, no core, no journal, and the system
// exit node as 5.4 and 5.7 want it in that phase; skip is skipRestore: true (no restore, 5.1).
func disabledAct(skip bool) func(t *testing.T, h *harness, phase string, mark int, release func()) {
	return func(t *testing.T, h *harness, phase string, mark int, release func()) {
		if phase == protocol.PhaseStopping { // only with skipRestore: no restore, the core is stopped
			h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
			release()
		}
		h.waitPhase(protocol.PhaseOff)
		if h.e.State().Reason != nil || h.core.anyAlive() || h.store.hasJournal() || h.store.saved().WasOn {
			t.Fatalf("off, no core, no journal, was-on cleared: %+v %v", h.e.State(), h.rec.list())
		}
		on, ip := systemExitNode(h)
		switch phase {
		case protocol.PhaseStarting: // step 6 was not reached: the system exit node was never touched
			if countSince(h, mark, "sys:") != 0 || !on {
				t.Fatalf("events %v", h.rec.list())
			}
		case protocol.PhaseBlocked: // the exit node the user selected stays (5.7)
			if countSince(h, mark, "sys:set") != 0 || !on || ip != "100.64.0.99" {
				t.Fatalf("the user's exit node must stay: %v", h.rec.list())
			}
		case protocol.PhaseOn, protocol.PhaseDegraded:
			if skip { // the journal goes without a restore, the system exit node stays off
				if countSince(h, mark, "sys:set") != 0 || on {
					t.Fatalf("no restore with skipRestore: %v", h.rec.list())
				}
			} else if countSince(h, mark, "sys:set "+exitNode) == 0 || !on || ip != exitNode { // 5.4 restores
				t.Fatalf("restore expected: %v", h.rec.list())
			}
		case protocol.PhaseStopping: // skipRestore: an attempt already under way ends, no new one follows
			if on {
				t.Fatalf("no restore with skipRestore: %v", h.rec.list())
			}
		}
	}
}

// TestPhaseTable walks the phase table of 5.1 cell by cell for the methods of this build
// (tailscale.login and tailscale.logout answer internal in every phase: TestUnknownAndLaterMethods).
// routing.disable with skipRestore disables without a restore wherever it disables (5.1).
func TestPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	lanes := protocol.LanesSetParams{DefaultLane: "vpn", Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}}
	rows := []struct {
		name   string
		method string
		params any
		want   []string // off, error, starting, on, degraded, blocked, stopping
		act    func(t *testing.T, h *harness, phase string, mark int, release func())
	}{
		{"enable", protocol.MethodEnable, nil,
			[]string{cellAct, cellAct, cellNone, cellNone, cellAct, cellNone, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if phase == protocol.PhaseDegraded { // "Повторить": an extra check; the VPN lane answers again
					h.waitFor("on after the extra check", func() bool {
						return h.e.State().Phase == protocol.PhaseOn && h.clash.currentMode() == "vpn" && countSince(h, mark, "dns:flush") == 1
					})
					return
				}
				h.waitPhase(protocol.PhaseOn)
				if countSince(h, mark, "core:start") != 1 {
					t.Fatalf("one enable: %v", h.rec.list())
				}
			}},
		{"disable", protocol.MethodDisable, nil,
			[]string{cellNone, cellAct, cellAct, cellAct, cellAct, cellAct, cellNone}, disabledAct(false)},
		{"disable skipRestore", protocol.MethodDisable, protocol.DisableParams{SkipRestore: true},
			[]string{cellNone, cellAct, cellAct, cellAct, cellAct, cellAct, cellAct}, disabledAct(true)},
		{"allDirect", protocol.MethodAllDirect, map[string]bool{"on": true},
			[]string{wrong, wrong, busy, cellAct, cellAct, wrong, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if !h.e.State().AllDirect || countSince(h, mark, "clash:mode alldirect") != 1 {
					t.Fatalf("all direct applied: %v", h.rec.list())
				}
			}},
		{"lanes.set", protocol.MethodLanesSet, lanes,
			[]string{cellAct, cellAct, busy, cellAct, cellAct, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				applied := countSince(h, mark, "rules:write") > 0
				if applied != live(phase) || len(h.store.saved().Apps) != 1 {
					t.Fatalf("applied %v (only while the core runs), saved %+v", applied, h.store.saved().Apps)
				}
			}},
		{"setExitNode", protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"},
			[]string{cellAct, cellAct, busy, cellAct, cellAct, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if h.store.saved().SelectedExitNode != "100.64.0.5" {
					t.Fatal("not saved")
				}
				if !live(phase) {
					if countSince(h, mark, "core:") != 0 {
						t.Fatalf("saved only: %v", h.rec.list())
					}
					return
				}
				h.waitFor("off and on again", func() bool {
					return countSince(h, mark, "core:start") == 1 && h.e.State().Phase == protocol.PhaseOn && h.idle()
				})
			}},
		{"clearExitNode", protocol.MethodClearSystemExitNode, nil,
			[]string{wrong, wrong, busy, cellNone, cellNone, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if countSince(h, mark, "sys:clear") != 1 || h.e.State().Phase != protocol.PhaseOn {
					t.Fatalf("cleared in blocked: %s %v", h.e.State().Phase, h.rec.list())
				}
			}},
		{"shutdown", protocol.MethodShutdown, nil,
			[]string{cellAct, cellAct, cellAct, cellAct, cellAct, cellAct, cellAct},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if phase == protocol.PhaseStopping {
					if exitedNow(h) {
						t.Fatal("in stopping the helper waits for the end of the disabling")
					}
					release()
				}
				h.waitFor("exit", func() bool { return exitedNow(h) })
				want := protocol.PhaseOff
				if phase == protocol.PhaseError {
					want = protocol.PhaseError
				}
				if h.e.State().Phase != want || h.core.anyAlive() || h.store.saved().WasOn {
					t.Fatalf("phase %s, events %v", h.e.State().Phase, h.rec.list())
				}
			}},
	}
	for _, row := range rows {
		for i, phase := range tablePhases {
			t.Run(row.name+"/"+phase, func(t *testing.T) {
				h, release := inPhase(t, phase)
				defer release()
				before, phaseBefore := h.rec.list(), h.e.State().Phase
				err := call(h, row.method, row.params)
				h.e.queue.Lock() // an apply that took over the queue is done
				h.e.queue.Unlock()
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h, phase, len(before), release)
				case cellNone:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phaseBefore {
						t.Fatalf("no action expected: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				default:
					wantCode(t, err, want)
				}
			})
		}
	}
}

// helper.shutdown: disable (or cancel the enable), then the process ends.
func TestShutdown(t *testing.T) {
	h := newHarness(t)
	h.load()
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })

	h = enabled(t)
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })
	if h.e.State().Phase != protocol.PhaseOff || h.core.last().alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("shutdown must disable first: %+v", h.e.State())
	}
	wantCode(t, call(h, protocol.MethodEnable, nil), protocol.CodeBusy)

	h = newHarness(t)
	h.probe.hang = true
	h.load()
	call(h, protocol.MethodEnable, nil)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })
	if h.e.State().Phase != protocol.PhaseOff || h.core.last().alive() {
		t.Fatalf("shutdown in starting rolls back: %+v", h.e.State())
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/engine/
```

Ожидается: `FAIL splitlane/helper/internal/engine [build failed]` и `h.e.Handle undefined (type *Engine has no field or method Handle)`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/engine/commands.go`, целиком:

```go
package engine

import (
	"context"
	"encoding/json"
	"slices"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// Handle answers one request of 5.1. The pipe server answers hello itself and gates the
// protocol version. State-changing commands run one at a time in arrival order (the queue);
// the transitions starting and stopping do not hold the queue: the phase table answers there.
func (e *Engine) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	if !slices.Contains(protocol.Methods, method) {
		return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown method %q", method)
	}
	switch method {
	case protocol.MethodHello:
		return e.Hello(), nil
	case protocol.MethodStateGet:
		if err := noParams(params); err != nil {
			return nil, err
		}
		return e.State(), nil
	case protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport:
		return nil, protocol.Errorf(protocol.CodeInternal, "%s is not available in this build (Plan 1B)", method)
	}
	e.queue.Lock()
	keep := false // true when a background job took over the queue and unlocks it when done
	defer func() {
		if !keep {
			e.queue.Unlock()
		}
	}()
	var err *protocol.Error
	switch method {
	case protocol.MethodEnable:
		err = e.cmdEnable(params)
	case protocol.MethodDisable:
		err = e.cmdDisable(params)
	case protocol.MethodAllDirect:
		keep, err = e.cmdAllDirect(params)
	case protocol.MethodLanesSet:
		err = e.cmdLanesSet(ctx, params)
	case protocol.MethodSetExitNode:
		err = e.cmdSetExitNode(params)
	case protocol.MethodClearSystemExitNode:
		keep, err = e.cmdClearSystemExitNode(params)
	case protocol.MethodShutdown:
		err = e.cmdShutdown(params)
	}
	if err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func noParams(params json.RawMessage) *protocol.Error {
	var none struct{}
	return protocol.DecodeParams(params, &none)
}

func busy() *protocol.Error { return protocol.Errorf(protocol.CodeBusy, "the helper is busy") }

func wrongPhase(phase string) *protocol.Error {
	return protocol.Errorf(protocol.CodeWrongPhase, "not possible in phase %s", phase)
}

// cmdEnable: off and error start 5.3; degraded runs an extra check ("Повторить"); stopping is busy.
func (e *Engine) cmdEnable(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exiting {
		return busy()
	}
	switch e.st.Phase {
	case protocol.PhaseOff, protocol.PhaseError:
		if !e.beginEnableLocked(false) {
			return busy()
		}
	case protocol.PhaseDegraded:
		go e.checkHealth(context.Background())
	case protocol.PhaseStopping:
		return busy()
	}
	return nil
}

// cmdDisable: 5.4 with the phase table of 5.1; "was on" is cleared at the beginning. With
// skipRestore the journal goes without a restore in every phase that disables (5.1, 5.4).
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
	var p protocol.DisableParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.st.Phase {
	case protocol.PhaseOff:
	case protocol.PhaseError:
		e.finish(protocol.PhaseOff, nil)
	case protocol.PhaseStarting:
		e.saved.WasOn = false
		e.save()
		e.skip = p.SkipRestore
		if e.op != nil {
			e.op.cancel()
		}
	case protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked:
		if !e.beginDisableLocked() {
			return busy()
		}
		e.skip = p.SkipRestore // read by the operation under e.mu, so it is set before its first look
	case protocol.PhaseStopping:
		pending := e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending
		if p.SkipRestore {
			e.skip = true
			if e.op != nil {
				e.op.cancel()
			}
		}
		if p.SkipRestore || pending {
			select {
			case e.wake <- struct{}{}:
			default:
			}
		}
	}
	return nil
}

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

func (e *Engine) revert(ctx context.Context, r *coreRun, direct, vpn []byte, mode string, modeChanged bool) {
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		e.d.Log("previous rule files not restored: %v", err)
	}
	if modeChanged {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.d.Log("previous mode %s not restored: %v", mode, err)
		}
	}
}

// reconcile is the check of 8.3: for 3 s, every 250 ms, each connection whose lane rules 8–11 of
// 6.2 decide (Conn.ByLanes) and that runs on another lane than want names is closed through the
// API. False: the core could not read a lane file again, or connections on the wrong lane were
// still there at the end.
func (e *Engine) reconcile(ctx context.Context, r *coreRun, want func(path string) string, mark int64) bool {
	end := e.d.Clock.Now().Add(applyWindow)
	for {
		conns, err := r.clash.Connections(ctx)
		wrong := 0
		for _, c := range conns {
			if !c.ByLanes {
				continue
			}
			if c.Lane != want(c.ProcessPath) {
				wrong++
				if err := r.clash.CloseConn(ctx, c.ID); err != nil {
					e.d.Log("close connection: %v", err)
				}
			}
		}
		if e.d.Core.Events(mark).ReloadFailed {
			e.d.Log("the core could not read a rule file again")
			return false
		}
		if !e.d.Clock.Now().Before(end) {
			return err == nil && wrong == 0
		}
		if !e.sleep(ctx, applyEvery) {
			return false
		}
	}
}

// expectedLane is where rules 8–11 of 6.2 send a process: everything direct in "all direct",
// else the lane of its rule, else the main lane (in degraded too: only the DNS changes there).
func expectedLane(set *rules.Set, mainLane string, allDirect bool, path string) string {
	if allDirect {
		return protocol.LaneDirect
	}
	if path != "" {
		if lane, ok := set.LaneOf(path); ok {
			return lane
		}
	}
	return mainLane
}

// applyAllDirect switches "all direct" on or off and reconciles (8.3); on failure the previous
// mode comes back and allDirect stays as it was. The window learns the result from the state event.
// The DNS moves with the mode (6.4): to the adapter's DNS and back, unless degraded has it there
// already; each such switch, the one back after a failure too, flushes the Windows DNS cache.
func (e *Engine) applyAllDirect(ctx context.Context, on bool) {
	e.mu.Lock()
	r, lane, degraded, apps, was := e.run, e.saved.DefaultLane, e.degraded, e.saved.Apps, e.st.AllDirect
	old := ""
	if r != nil {
		old = r.mode
	}
	e.mu.Unlock()
	if r == nil {
		return
	}
	mode := e.d.Core.Mode(lane, on, degraded)
	set, err := rules.Compile(apps, e.d.Rules)
	if err != nil {
		e.d.Log("all direct: %v", err)
		return
	}
	dnsMoves := localDNS(was, degraded) != localDNS(on, degraded)
	mark := e.d.Core.LogSize()
	switched := r.clash.SetMode(ctx, mode) == nil
	if switched && dnsMoves {
		e.flushDNS()
	}
	ok := switched && e.reconcile(ctx, r, func(path string) string { return expectedLane(set, lane, on, path) }, mark)
	if !ok {
		if err := r.clash.SetMode(ctx, old); err != nil {
			e.d.Log("all direct: previous mode %s not restored: %v", old, err)
		} else if switched && dnsMoves {
			e.flushDNS()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ok {
		r.mode = mode
		e.st.AllDirect = on
	}
	e.publish()
}

// cmdSetExitNode saves the choice; while routing is on it disables and enables again (7).
func (e *Engine) cmdSetExitNode(params json.RawMessage) *protocol.Error {
	var p protocol.SetExitNodeParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	phase := e.st.Phase
	if e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping {
		return busy()
	}
	if err := protocol.ValidateExitNodeID(p.ID, e.saved.ExitNodes); err != nil {
		return err
	}
	e.saved.SelectedExitNode = p.ID
	e.save()
	if !live(phase) {
		e.publish()
		return nil
	}
	e.setPhase(protocol.PhaseStopping, nil)
	e.startOp("reconnect", func(ctx context.Context) {
		if !e.teardown() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		e.runEnable(ctx, false)
	})
	return nil
}

// cmdClearSystemExitNode: in blocked the system exit node is switched off (5.6, 5.7).
func (e *Engine) cmdClearSystemExitNode(params json.RawMessage) (bool, *protocol.Error) {
	if err := noParams(params); err != nil {
		return false, err
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	switch {
	case exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return false, busy()
	case phase == protocol.PhaseOff || phase == protocol.PhaseError:
		return false, wrongPhase(phase)
	case phase != protocol.PhaseBlocked:
		return false, nil
	}
	go func() {
		defer e.queue.Unlock()
		e.clearSystemExitNode(context.Background())
	}()
	return true, nil
}

// cmdShutdown: disable (or roll back) as the phase requires, then the process ends (OnExit).
func (e *Engine) cmdShutdown(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exiting {
		return nil
	}
	e.exiting = true
	e.saved.WasOn = false
	e.save()
	switch e.st.Phase {
	case protocol.PhaseStarting:
		if e.op != nil {
			e.op.cancel()
		}
	case protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked:
		e.beginDisableLocked()
	}
	go func() {
		e.Wait()
		e.d.OnExit()
	}()
	return nil
}
```

- [ ] **Step 4: Запустить тест, убедиться в успехе, затем прогнать движок 15 раз**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=15 ./internal/engine/
```

Ожидается `ok  splitlane/helper/internal/engine` (около 20 секунд). Нестабильный тест здесь — ошибка в коде или в тесте, а не повод повторять прогон.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/engine
git -C C:\code\router commit -m "feat(helper): command phase table, lanes.set reconciliation and all-direct mode" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 15: Ядро: процесс, консольный сигнал, Clash API

Драйвер ядра раздела 4: единственная реализация `engine.CoreDriver` — для неизменённого `sing-box.exe` 1.14.4. `BuildConfig` — `coreconfig.Build` с папкой данных драйвера; `LaneFiles` — `rules.Files`; `Mode` — `coreconfig.ModeFor`; `Events` — строки `StartLine`, `LoginLine` и `ReloadErrorLine` в журнале после смещения (состояние коннектора и ошибка перечитывания файла дорожки). `Start` откладывает прежний журнал (`core.log.prev`): строка запуска должна принадлежать этому процессу (5.3 шаг 4); ядро запускается в своей группе процессов со скрытой консолью (`CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW`), его вывод — в `core-stdout.log`. Идентичность — идентификатор процесса, время создания (`GetProcessTimes`) и путь образа (`QueryFullProcessImageName`); по ним `Find` находит своё ядро от прошлого запуска (5.7). `Interrupt` — `CTRL_BREAK_EVENT` группе ядра: служебный процесс собран без консоли, поэтому на момент сигнала он присоединяется к скрытой консоли ядра (`AttachConsole`) и сразу её отпускает (способ проверен задачей 2). `LogSize`/`LogFrom` читают журнал с нужного места, не перечитывая его целиком. `Clash` — клиент Clash API без прокси: `PATCH /configs` и обязательная проверка `GET /configs` (ядро отвечает 204 и на неизвестный режим), `GET /connections` (дорожка соединения — первый элемент `chains`: выходы в конфиге названы по дорожкам; признак `ByLanes` — по полю `rule`: `clash_mode=…`, `rule_set=lane-…` или `final`), `DELETE /connections/<id>` (без идентификатора — никогда). `VerifyCore` сверяет SHA-256 ядра с закреплённым значением.

Тесты не запускают настоящее ядро: роль ядра играет сам тестовый бинарник (`TestMain` с `SPLITLANE_FAKE_CORE=1`), Clash API — `httptest`. Тест настоящего консольного сигнала включается переменной `SPLITLANE_CONSOLE_TEST=1` (задача 22): на время сигнала тестовый процесс отпускает свою консоль.

**Files:**
- Create: `helper/internal/singbox/console.go`, `helper/internal/singbox/driver.go`, `helper/internal/singbox/clash.go`, `helper/internal/singbox/clash_test.go`, `helper/internal/singbox/driver_test.go`

**Interfaces:**
- Consumes: `engine.CoreDriver`, `engine.CoreOptions`, `engine.CoreEvents`, `engine.Ports`, `engine.CoreProc`, `engine.ClashAPI`, `engine.Conn` (задача 10); `coreconfig.Build`, `Options`, `Ports`, `ModeFor`, `PathsFor`, `StartLine`, `LoginLine`, `ReloadErrorLine` (задача 8); `rules.Files`, `rules.Env` (задача 7); `store.CoreIdentity`.
- Produces: `const CoreVersion = "1.14.4"`; `const CoreSHA256 = "8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9"`; `VerifyCore(exe string) error`; `type Driver struct{ Exe, DataDir string }` (реализует `engine.CoreDriver`; ещё `(*Driver).LogFrom(offset int64) string`); `CtrlBreak(pid uint32) error`; `NewClash(base, secret string) *Clash` (реализует `engine.ClashAPI`), внутренний `byLanes(rule string) bool`. Тест `TestDriverSpeaksSingBox` сверяет режимы, строки журнала до и после отметки, конфиг в папке драйвера и файлы дорожек.

- [ ] **Step 1: Написать тесты**

Файл `helper/internal/singbox/clash_test.go`, целиком:

```go
package singbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI behaves like the Clash API of sing-box 1.14.4: 204 for any PATCH, unknown modes ignored.
type fakeAPI struct {
	mu      sync.Mutex
	mode    string
	deleted []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer s3cret" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPatch && r.URL.Path == "/configs":
		var body struct {
			Mode string `json:"mode"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		for _, m := range []string{"vpn", "vpn-localdns", "direct", "direct-localdns", "alldirect"} {
			if strings.EqualFold(m, body.Mode) {
				f.mode = m
			}
		}
		w.WriteHeader(http.StatusNoContent)
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
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestClashModeIsReadBack(t *testing.T) {
	api := &fakeAPI{mode: "vpn"}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClash(srv.URL, "s3cret")
	if err := c.SetMode(context.Background(), "direct-localdns"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMode(context.Background(), "global"); err == nil || !strings.Contains(err.Error(), "did not take") {
		t.Fatalf("an unknown mode must fail on the read back, got %v", err)
	}
	if err := NewClash(srv.URL, "wrong").SetMode(context.Background(), "vpn"); err == nil {
		t.Fatal("a wrong secret must fail")
	}
}

func TestClashConnectionsAndClose(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClash(srv.URL, "s3cret")
	conns, err := c.Connections(context.Background())
	if err != nil || len(conns) != 4 {
		t.Fatalf("%v %v", conns, err)
	}
	if conns[0].ProcessPath != `C:\Tools\curl-a.exe` || conns[0].Lane != "direct" || !conns[0].ByLanes {
		t.Fatalf("first %+v", conns[0])
	}
	// rules 8–11 of 6.2 (a lane file, the final route, a mode) count; a built-in rule does not
	if !conns[1].ByLanes || conns[2].ByLanes || !conns[3].ByLanes || conns[2].Lane != "vpn" {
		t.Fatalf("%+v", conns)
	}
	if err := c.CloseConn(context.Background(), "a1"); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseConn(context.Background(), ""); err == nil {
		t.Fatal("an empty id must never become DELETE /connections")
	}
	if len(api.deleted) != 1 || api.deleted[0] != "a1" {
		t.Fatalf("deleted %v", api.deleted)
	}
}
```

Файл `helper/internal/singbox/driver_test.go`, целиком:

```go
package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

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
	if args[0] == "check" {
		if bytes.Contains(config, []byte("bad")) {
			os.Stdout.WriteString("FATAL decode config: bad field\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	var c struct {
		Log struct {
			Output string `json:"output"`
		} `json:"log"`
	}
	json.Unmarshal(config, &c)
	appendLine := func(s string) {
		f, _ := os.OpenFile(c.Log.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString(s + "\n")
		f.Close()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	appendLine("INFO sing-box started (0.01s)")
	<-sig
	appendLine("INFO interrupted")
	os.Exit(0)
}

func fakeDriver(t *testing.T) (*Driver, string) {
	t.Helper()
	t.Setenv("SPLITLANE_FAKE_CORE", "1")
	dir := t.TempDir()
	p := coreconfig.PathsFor(dir)
	config := filepath.Join(dir, "config.json")
	b, _ := json.Marshal(map[string]any{"log": map[string]string{"output": filepath.ToSlash(p.CoreLog)}})
	os.WriteFile(config, b, 0o644)
	exe, _ := os.Executable()
	return &Driver{Exe: exe, DataDir: dir}, config
}

func waitLog(t *testing.T, d *Driver, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(d.LogFrom(0), text) {
		if time.Now().After(deadline) {
			t.Fatalf("log never showed %q: %q", text, d.LogFrom(0))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartRotatesTheLogAndIdentifiesTheProcess(t *testing.T) {
	d, config := fakeDriver(t)
	p := coreconfig.PathsFor(d.DataDir)
	os.MkdirAll(p.LogsDir, 0o755)
	os.WriteFile(p.CoreLog, []byte("INFO sing-box started (old run)\n"), 0o644)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Kill(); <-proc.Done() })
	if old, _ := os.ReadFile(p.CoreLog + ".prev"); !strings.Contains(string(old), "old run") {
		t.Fatal("the old log must be set aside")
	}
	waitLog(t, d, "sing-box started (0.01s)")
	if strings.Contains(d.LogFrom(0), "old run") {
		t.Fatal("the start line of an earlier run must not count")
	}
	id := proc.Identity()
	exe, _ := os.Executable()
	if id.PID <= 0 || id.Created == 0 || !strings.EqualFold(id.Path, exe) {
		t.Fatalf("identity %+v", id)
	}
	size := d.LogSize()
	if size == 0 || d.LogFrom(size) != "" {
		t.Fatalf("size %d, text after the end %q", size, d.LogFrom(size))
	}
}

func TestFindOnlyTheSameProcess(t *testing.T) {
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, "started")
	id := proc.Identity()
	other := store.CoreIdentity{PID: id.PID, Created: id.Created + 1, Path: id.Path}
	if _, ok := d.Find(other); ok {
		t.Fatal("another creation time is another process")
	}
	found, ok := d.Find(id)
	if !ok {
		t.Fatal("own core not found")
	}
	if err := found.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-found.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("found process did not end")
	}
	<-proc.Done()
	if _, ok := d.Find(id); ok {
		t.Fatal("an ended process must not be found")
	}
}

func TestCheckReportsTheCoreMessage(t *testing.T) {
	d, config := fakeDriver(t)
	if err := d.Check(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(config, []byte(`{"bad":true}`), 0o644)
	if err := d.Check(context.Background(), config); err == nil || !strings.Contains(err.Error(), "bad field") {
		t.Fatalf("got %v", err)
	}
}

// The engine's operations in sing-box terms (spec 4): the modes of 6.3, the log lines of 5.3 and
// 8.3, the config of spec 6 in the driver's data folder, the rule files of 6.5.
func TestDriverSpeaksSingBox(t *testing.T) {
	d := &Driver{Exe: `C:\core\sing-box.exe`, DataDir: t.TempDir()}
	for _, c := range []struct {
		lane          string
		all, degraded bool
		want          string
	}{
		{"vpn", false, false, "vpn"}, {"vpn", false, true, "vpn-localdns"}, {"direct", false, false, "direct"},
		{"direct", false, true, "direct-localdns"}, {"direct", true, true, "alldirect"},
	} {
		if got := d.Mode(c.lane, c.all, c.degraded); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	p := coreconfig.PathsFor(d.DataDir)
	os.MkdirAll(p.LogsDir, 0o755)
	os.WriteFile(p.CoreLog, []byte("INFO sing-box started (0.42s)\n"), 0o644)
	mark := d.LogSize()
	if ev := d.Events(0); !ev.Started || ev.NeedLogin || ev.ReloadFailed {
		t.Fatalf("start: %+v", ev)
	}
	f, _ := os.OpenFile(p.CoreLog, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x\n" +
		"ERROR router: reload rule-set lane-direct: parse error\n")
	f.Close()
	if ev := d.Events(mark); ev.Started || !ev.NeedLogin || !ev.ReloadFailed {
		t.Fatalf("after the mark: %+v", ev)
	}
	config, err := d.BuildConfig(engine.CoreOptions{Mode: "vpn-localdns", HelperPath: `C:\SplitLane\splitlane-helper.exe`,
		TailscaleDir: `C:\Program Files\Tailscale`, Hostname: "pc-splitlane", ExitNode: "100.64.0.1",
		Ports:       engine.Ports{ProbeVPN: 40001, ProbeDirect: 40002, Clash: 40003, API: 40004},
		ClashSecret: "0123456789abcdef", APISecret: "fedcba9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_mode": "vpn-localdns"`, `"exit_node": "100.64.0.1"`, `"listen_port": 40002`,
		`"output": "` + filepath.ToSlash(p.CoreLog) + `"`, `"path": "` + filepath.ToSlash(p.RulesDirect) + `"`} {
		if !strings.Contains(string(config), want) {
			t.Errorf("config lacks %s", want)
		}
	}
	direct, vpn, err := d.LaneFiles(nil, rules.Env{})
	if err != nil || !strings.Contains(string(direct), rules.Stub) || !strings.Contains(string(vpn), rules.Stub) {
		t.Fatalf("lane files: %v\n%s\n%s", err, direct, vpn)
	}
}

func TestVerifyCore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box.exe")
	os.WriteFile(path, []byte("not the core"), 0o644)
	if err := VerifyCore(path); err == nil {
		t.Fatal("a wrong file must fail")
	}
	if real := os.Getenv("SPLITLANE_CORE"); real != "" {
		if err := VerifyCore(real); err != nil {
			t.Fatal(err)
		}
	}
}

// TestInterruptStopsTheCore sends the real CTRL_BREAK through the core's hidden console. The test
// process lets its own console go for a moment, so it runs only with SPLITLANE_CONSOLE_TEST=1.
func TestInterruptStopsTheCore(t *testing.T) {
	if os.Getenv("SPLITLANE_CONSOLE_TEST") != "1" {
		t.Skip("SPLITLANE_CONSOLE_TEST is not set")
	}
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, "started")
	if err := proc.Interrupt(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		proc.Kill()
		t.Fatal("the core did not stop within 5 s after CTRL_BREAK")
	}
	waitLog(t, d, "interrupted")
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/singbox/
```

Ожидается: `FAIL splitlane/helper/internal/singbox [build failed]` и `undefined: Driver`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/singbox/console.go`, целиком:

```go
package singbox

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procFreeConsole   = kernel32.NewProc("FreeConsole")
)

// attachParentProcess is ATTACH_PARENT_PROCESS, (DWORD)-1.
const attachParentProcess = ^uint32(0)

var consoleMu sync.Mutex

func attachConsole(pid uint32) error {
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return err
	}
	return nil
}

func freeConsole() { procFreeConsole.Call() }

// CtrlBreak sends CTRL_BREAK_EVENT to the process group whose id is pid: the core is started in
// a group of its own, with a hidden console (spec 5.4). The helper is built without a console;
// for the moment of the signal it attaches to the core's console and then lets it go. A caller
// that has a console of its own (go test, for example) gets it back afterwards.
// The caller is not in the core's group, so the event does not reach it.
func CtrlBreak(pid uint32) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	hadConsole := false
	if err := attachConsole(pid); err != nil {
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return fmt.Errorf("attach to the console of %d: %w", pid, err)
		}
		hadConsole = true // already attached to a console: let it go first
		freeConsole()
		if err := attachConsole(pid); err != nil {
			attachConsole(attachParentProcess)
			return fmt.Errorf("attach to the console of %d: %w", pid, err)
		}
	}
	defer func() {
		freeConsole()
		if hadConsole {
			attachConsole(attachParentProcess)
		}
	}()
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)
}
```

Файл `helper/internal/singbox/clash.go`, целиком:

```go
package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"splitlane/helper/internal/engine"
)

// Clash is the client of the core's Clash API (Plan 0, "Факты о ядре 1.14.4").
type Clash struct {
	base   string
	secret string
	http   *http.Client
}

var _ engine.ClashAPI = (*Clash)(nil)

// NewClash talks to base ("http://127.0.0.1:<port>") with the run's secret, never through a proxy.
func NewClash(base, secret string) *Clash {
	return &Clash{base: base, secret: secret, http: &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}}
}

func (c *Clash) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("clash api %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// SetMode is PATCH /configs, then GET /configs: the core answers 204 even for a mode it does not
// know, so the switch counts only when the mode reads back (spec 6.3).
func (c *Clash) SetMode(ctx context.Context, mode string) error {
	if err := c.do(ctx, http.MethodPatch, "/configs", map[string]string{"mode": mode}, nil); err != nil {
		return err
	}
	var cfg struct {
		Mode string `json:"mode"`
	}
	if err := c.do(ctx, http.MethodGet, "/configs", nil, &cfg); err != nil {
		return err
	}
	if !strings.EqualFold(cfg.Mode, mode) {
		return fmt.Errorf("the core did not take mode %s (it is in %s)", mode, cfg.Mode)
	}
	return nil
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
		return nil, err
	}
	out := make([]engine.Conn, 0, len(data.Connections))
	for _, x := range data.Connections {
		conn := engine.Conn{ID: x.ID, ProcessPath: x.Metadata.ProcessPath, ByLanes: byLanes(x.Rule)}
		if len(x.Chains) > 0 {
			conn.Lane = x.Chains[0]
		}
		out = append(out, conn)
	}
	return out, nil
}

// byLanes reads the "rule" field: the text of the rule and its action. Rules 8–11 of 6.2 are a
// mode rule, a lane file or the final route: "clash_mode=vpn => route(vpn)",
// "rule_set=lane-direct => route(direct)", "final" (Plan 1A, "Факты, на которые опирается план").
func byLanes(rule string) bool {
	return strings.HasPrefix(rule, "clash_mode=") || strings.HasPrefix(rule, "rule_set=lane-") || rule == "final"
}

// CloseConn is DELETE /connections/<id>. DELETE /connections without an id is never used:
// it resets the whole network of the core (spec 6.3).
func (c *Clash) CloseConn(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("empty connection id")
	}
	return c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}
```

Файл `helper/internal/singbox/driver.go`, целиком:

```go
// Package singbox is the one core driver of stage 1 (spec 4): the unmodified sing-box 1.14.4
// behind engine.CoreDriver. It builds the config (package coreconfig) and the lane files (package
// rules), names the modes, runs the processes, reads the log and talks to the Clash API (5.3–5.5).
package singbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// CoreVersion is the only core version the configs are built for (spec 6).
const CoreVersion = "1.14.4"

// CoreSHA256 is the SHA-256 of sing-box.exe from sing-box-1.14.4-windows-amd64.zip (spec 4.1, 4.3).
const CoreSHA256 = "8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9"

// maxLogRead bounds one read of the core log.
const maxLogRead = 1 << 20

// VerifyCore compares the SHA-256 of exe with CoreSHA256.
func VerifyCore(exe string) error {
	f, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != CoreSHA256 {
		return fmt.Errorf("%s has SHA-256 %s, want %s", exe, got, CoreSHA256)
	}
	return nil
}

// Driver implements engine.CoreDriver for one sing-box.exe and one data folder.
type Driver struct {
	Exe     string
	DataDir string
}

var _ engine.CoreDriver = (*Driver)(nil)

// BuildConfig is the config of spec 6 for the options, with the files of the driver's data folder.
func (d *Driver) BuildConfig(o engine.CoreOptions) ([]byte, error) {
	return coreconfig.Build(coreconfig.Options{
		Mode: o.Mode, DataDir: d.DataDir, HelperPath: o.HelperPath, TailscaleDir: o.TailscaleDir,
		Hostname: o.Hostname, ExitNode: o.ExitNode, Ports: coreconfig.Ports(o.Ports),
		ClashSecret: o.ClashSecret, APISecret: o.APISecret, DevPin: o.DevPin,
	})
}

// LaneFiles are the rule-set files (format 3) of both lanes (6.5, 8.2).
func (d *Driver) LaneFiles(apps []protocol.AppRule, env rules.Env) ([]byte, []byte, error) {
	return rules.Files(apps, env)
}

// Mode is the clash mode of spec 6.3.
func (d *Driver) Mode(defaultLane string, allDirect, degraded bool) string {
	return coreconfig.ModeFor(defaultLane, allDirect, degraded)
}

// Events reads the log after offset: the start line of this run, the login request of the
// embedded node and a lane file that could not be read again.
func (d *Driver) Events(offset int64) engine.CoreEvents {
	text := d.LogFrom(offset)
	return engine.CoreEvents{
		Started:      strings.Contains(text, coreconfig.StartLine),
		NeedLogin:    strings.Contains(text, coreconfig.LoginLine),
		ReloadFailed: strings.Contains(text, coreconfig.ReloadErrorLine),
	}
}

func hidden(flags uint32) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

// Check runs `sing-box check`; the core reads the rule files already here (spec 12).
func (d *Driver) Check(ctx context.Context, configPath string) error {
	cmd := exec.CommandContext(ctx, d.Exe, "check", "-c", configPath, "-D", d.DataDir)
	cmd.SysProcAttr = hidden(windows.CREATE_NO_WINDOW)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 400 {
			text = text[:400]
		}
		return fmt.Errorf("sing-box check: %v: %s", err, text)
	}
	return nil
}

// Start sets the old log aside, then starts the core in a process group of its own, with a
// hidden console and its output in core-stdout.log (spec 5.3 step 4, 5.4 step 2).
func (d *Driver) Start(configPath string) (engine.CoreProc, error) {
	p := coreconfig.PathsFor(d.DataDir)
	if err := os.MkdirAll(p.LogsDir, 0o755); err != nil {
		return nil, err
	}
	if err := setAside(p.CoreLog); err != nil {
		return nil, err
	}
	out, err := os.OpenFile(p.CoreStdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(d.Exe, "run", "-c", configPath, "-D", d.DataDir)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = hidden(windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW)
	if err := cmd.Start(); err != nil {
		out.Close()
		return nil, err
	}
	pid := uint32(cmd.Process.Pid)
	id, err := identify(pid)
	if err != nil {
		id = store.CoreIdentity{PID: int(pid), Path: d.Exe}
	}
	proc := &process{id: id, done: make(chan struct{}), kill: cmd.Process.Kill}
	go func() {
		_ = cmd.Wait()
		out.Close()
		close(proc.done)
	}()
	return proc, nil
}

// setAside renames the previous log to .prev: the start line must come from this run (5.3 step 4).
func setAside(log string) error {
	if _, err := os.Stat(log); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_ = os.Remove(log + ".prev")
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(log, log+".prev"); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if rmErr := os.Remove(log); rmErr == nil {
		return nil
	}
	return fmt.Errorf("the previous core log cannot be set aside: %w", err)
}

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
		windows.CloseHandle(h)
		return nil, false
	}
	proc := &process{id: id, done: make(chan struct{})}
	var mu sync.Mutex
	closed := false
	proc.kill = func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		return windows.TerminateProcess(h, 1)
	}
	go func() {
		_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
		mu.Lock()
		closed = true
		windows.CloseHandle(h)
		mu.Unlock()
		close(proc.done)
	}()
	return proc, true
}

// LogSize is the size of the current core log.
func (d *Driver) LogSize() int64 {
	fi, err := os.Stat(coreconfig.PathsFor(d.DataDir).CoreLog)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// LogFrom is the text of the current core log after offset, at most 1 MiB.
func (d *Driver) LogFrom(offset int64) string {
	f, err := os.Open(coreconfig.PathsFor(d.DataDir).CoreLog)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(f, maxLogRead))
	return string(b)
}

// Clash returns the Clash API client of the run that listens on port with secret.
func (d *Driver) Clash(port int, secret string) engine.ClashAPI {
	return NewClash(fmt.Sprintf("http://127.0.0.1:%d", port), secret)
}

type process struct {
	id   store.CoreIdentity
	done chan struct{}
	kill func() error
}

func (p *process) Identity() store.CoreIdentity { return p.id }
func (p *process) Done() <-chan struct{}        { return p.done }
func (p *process) Interrupt() error             { return CtrlBreak(uint32(p.id.PID)) }
func (p *process) Kill() error                  { return p.kill() }

func identify(pid uint32) (store.CoreIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return store.CoreIdentity{}, err
	}
	defer windows.CloseHandle(h)
	created, path, err := processInfo(h)
	if err != nil {
		return store.CoreIdentity{}, err
	}
	return store.CoreIdentity{PID: int(pid), Created: created, Path: path}, nil
}

// processInfo returns the creation time (FILETIME, 100 ns units) and the image path of a process.
func processInfo(h windows.Handle) (int64, string, error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
		return 0, "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return 0, "", err
	}
	return int64(created.HighDateTime)<<32 | int64(created.LowDateTime), windows.UTF16ToString(buf[:size]), nil
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 -v ./internal/singbox/ | Select-String -Pattern '^(--- |ok|FAIL|PASS)'
```

Ожидается `--- PASS` у семи тестов, `--- SKIP: TestInterruptStopsTheCore` и `ok  splitlane/helper/internal/singbox`. `TestVerifyCore` с заданной `SPLITLANE_CORE` сверяет и настоящее ядро.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/singbox
git -C C:\code\router commit -m "feat(helper): sing-box process driver, console break signal and Clash API client" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 16: Системный Tailscale

Реализация `engine.SystemTailscale`: `tailscale.exe` только по полному пути из каталога установки (5.6, 11), без окна. Каталог установки — папка `tailscaled.exe` из `ImagePath` службы `Tailscale` (`HKLM\SYSTEM\CurrentControlSet\Services\Tailscale`; этот раздел пишут только администраторы; путь в кавычках берётся до закрывающей кавычки, без кавычек — до `tailscaled.exe`): так находится и клиент, установленный не в Program Files. Если службы нет или путь не годится (другая программа, относительный путь, `%` в пути) — папка `Tailscale` в каталоге Program Files из реестра (`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion`, значение `ProgramFilesDir`, 64-битное представление). Ни то, ни другое не берётся из переменной `%ProgramFiles%`: служебный процесс работает с правами администратора, но с окружением пользователя (в плане 1B его запускает задача с наивысшими правами), а `HKCU\Environment` может задать `ProgramFiles` с папкой, куда пишет сам пользователь. Тогда служебный процесс запустил бы с правами администратора чужой `tailscale.exe` — обход UAC сверх остаточного риска раздела 11; тот же путь даёт правило 5 из 6.2 (`Dir`) и список защищённых программ. Значение с `%`, относительное или нечитаемое — запасной `C:\Program Files`. `status --json`: не установлен — `Installed: false`; ошибка или `BackendState` не `Running` — exit node не выбран; адрес — первый IPv4 из `ExitNodeStatus.TailscaleIPs` без `/32`. Признак доступа к локальной сети — из `debug prefs` (`ExitNodeAllowLANAccess`; его нет в `status`); отсутствие поля — ошибка, а не `false`. Команды `set --exit-node=<адрес> --exit-node-allow-lan-access=<признак>` и `set --exit-node=`. Тесты подменяют запуск программы: настоящий `tailscale.exe` не вызывается.

**Files:**
- Create: `helper/internal/systs/systs.go`, `helper/internal/systs/systs_test.go`

**Interfaces:**
- Consumes: `engine.SystemTailscale`, `engine.SysStatus`.
- Produces: `DefaultExe() string` (`tailscale.exe` рядом с `tailscaled.exe` службы `Tailscale`, иначе `<ProgramFilesDir из HKLM>\Tailscale\tailscale.exe`), внутренние `serviceImagePath() (string, error)`, `serviceDir(imagePath string) (string, bool)`, `exeFrom(service, programFiles func() (string, error)) string`, `programFilesDir() (string, error)` и `exeIn(read func() (string, error)) string`; `type Runner func(ctx, exe string, args ...string) ([]byte, error)`; `type Client struct{ Exe string; Run Runner }`; `New() *Client`; `ParseStatus(out []byte) (engine.SysStatus, error)`; методы интерфейса. Тест `TestExeIgnoresTheEnvironment` задаёт `ProgramFiles` временной папкой и проверяет на заданных значениях выбор между путём службы и каталогом Program Files, а на настоящем реестре — что путь от `%ProgramFiles%` не зависит; он читает два значения реестра и ничего не меняет.

- [ ] **Step 1: Написать тест**

Файл `helper/internal/systs/systs_test.go`, целиком:

```go
package systs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shapes of the real output of Tailscale 1.104 (values replaced by documentation ones).
const (
	statusOn = `{"Version":"1.104.2","BackendState":"Running","ExitNodeStatus":{"ID":"nX","Online":true,
 "TailscaleIPs":["100.64.0.1/32","fd7a:115c:a1e0::1/128"]},"Self":{"HostName":"pc"},"Peer":{}}`
	statusOff     = `{"BackendState":"Running","ExitNodeStatus":null}`
	statusStopped = `{"BackendState":"Stopped","ExitNodeStatus":{"TailscaleIPs":["100.64.0.1/32"]}}`
	prefs         = `{"ControlURL":"https://controlplane.tailscale.com","ExitNodeID":"nX","ExitNodeIP":"","ExitNodeAllowLANAccess":true,"CorpDNS":false}`
)

type call struct{ args []string }

func fake(t *testing.T, outputs map[string]string, fail error) (*Client, *[]call) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "tailscale.exe")
	os.WriteFile(exe, []byte("x"), 0o644)
	var calls []call
	c := &Client{Exe: exe, Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
		if path != exe {
			t.Fatalf("tailscale.exe must be called by its full path, got %s", path)
		}
		calls = append(calls, call{args})
		if fail != nil {
			return nil, fail
		}
		return []byte(outputs[strings.Join(args, " ")]), nil
	}}
	return c, &calls
}

// Spec 5.6, 11: the path never follows the environment the elevated helper inherits.
func TestExeIgnoresTheEnvironment(t *testing.T) {
	user := t.TempDir()
	t.Setenv("ProgramFiles", user)
	cases := map[string]struct {
		dir  string
		err  error
		want string
	}{
		"registry":   {`D:\Apps`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"unreadable": {"", errors.New("no value"), `C:\Program Files\Tailscale\tailscale.exe`},
		"relative":   {`Program Files`, nil, `C:\Program Files\Tailscale\tailscale.exe`},
		"unexpanded": {`%SystemDrive%\Program Files`, nil, `C:\Program Files\Tailscale\tailscale.exe`},
	}
	for name, c := range cases {
		if got := exeIn(func() (string, error) { return c.dir, c.err }); got != c.want {
			t.Errorf("%s: %s", name, got)
		}
	}
	// The folder of the service's tailscaled.exe comes first (a client installed elsewhere).
	programFiles := func() (string, error) { return `D:\Apps`, nil }
	services := map[string]struct {
		image string
		err   error
		want  string
	}{
		"quoted":       {`"E:\Net\Tailscale\tailscaled.exe"`, nil, `E:\Net\Tailscale\tailscale.exe`},
		"with args":    {`E:\Net\Tailscale\tailscaled.exe -port 41641`, nil, `E:\Net\Tailscale\tailscale.exe`},
		"no service":   {"", errors.New("not found"), `D:\Apps\Tailscale\tailscale.exe`},
		"unexpanded":   {`"%ProgramFiles%\Tailscale\tailscaled.exe"`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"other binary": {`"E:\Net\other.exe"`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"relative":     {`Tailscale\tailscaled.exe`, nil, `D:\Apps\Tailscale\tailscale.exe`},
	}
	for name, c := range services {
		if got := exeFrom(func() (string, error) { return c.image, c.err }, programFiles); got != c.want {
			t.Errorf("service %s: %s", name, got)
		}
	}
	// The real registry values (read only).
	if exe := New().Exe; !filepath.IsAbs(exe) || strings.HasPrefix(strings.ToLower(exe), strings.ToLower(user)) {
		t.Fatalf("the path follows %%ProgramFiles%%: %s", exe)
	}
}

func TestStatus(t *testing.T) {
	cases := map[string]struct {
		out string
		on  bool
		ip  string
	}{
		"on":      {statusOn, true, "100.64.0.1"},
		"off":     {statusOff, false, ""},
		"stopped": {statusStopped, false, ""},
	}
	for name, c := range cases {
		client, _ := fake(t, map[string]string{"status --json": c.out}, nil)
		st, err := client.Status(context.Background())
		if err != nil || !st.Installed || st.ExitNodeOn != c.on || st.ExitNodeIP != c.ip {
			t.Errorf("%s: %+v %v", name, st, err)
		}
	}
}

func TestStatusNotInstalledOrFailing(t *testing.T) {
	c := &Client{Exe: filepath.Join(t.TempDir(), "missing", "tailscale.exe"), Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("must not run a missing client")
		return nil, nil
	}}
	if st, err := c.Status(context.Background()); err != nil || st.Installed {
		t.Fatalf("%+v %v", st, err)
	}
	failing, _ := fake(t, nil, errors.New("service stopped"))
	st, err := failing.Status(context.Background())
	if err == nil || !st.Installed || st.ExitNodeOn {
		t.Fatalf("a failing client counts as no exit node: %+v %v", st, err)
	}
}

func TestAllowLANAndCommands(t *testing.T) {
	c, calls := fake(t, map[string]string{"debug prefs": prefs}, nil)
	if lan, err := c.AllowLAN(context.Background()); err != nil || !lan {
		t.Fatalf("%v %v", lan, err)
	}
	c.SetExitNode(context.Background(), "100.64.0.1", true)
	c.ClearExitNode(context.Background())
	got := []string{}
	for _, x := range *calls {
		got = append(got, strings.Join(x.args, " "))
	}
	want := "debug prefs|set --exit-node=100.64.0.1 --exit-node-allow-lan-access=true|set --exit-node="
	if strings.Join(got, "|") != want {
		t.Fatalf("calls %v", got)
	}
	if !strings.Contains(c.RestoreCommand("100.64.0.1", false), `set --exit-node=100.64.0.1 --exit-node-allow-lan-access=false`) {
		t.Fatal(c.RestoreCommand("100.64.0.1", false))
	}
	broken, _ := fake(t, map[string]string{"debug prefs": `{"CorpDNS":false}`}, nil)
	if _, err := broken.AllowLAN(context.Background()); err == nil {
		t.Fatal("a missing flag must be an error, not false")
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/systs/
```

Ожидается: `FAIL splitlane/helper/internal/systs [build failed]` и `undefined: Client`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/systs/systs.go`, целиком:

```go
// Package systs talks to the official Tailscale client through tailscale.exe, always by its
// full path from the install folder, never through PATH (spec 5.6, 11).
package systs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/engine"
)

// fallbackProgramFiles is used when the registry value cannot be read or is not a plain path.
const fallbackProgramFiles = `C:\Program Files`

// DefaultExe is the CLI in the install folder of the official client (spec 5.6): the folder of
// tailscaled.exe that the Tailscale service names (its ImagePath in HKLM\SYSTEM), else the
// Program Files folder that the registry names (HKLM, 64-bit view). Never %ProgramFiles%: the
// helper runs elevated with the user's environment, and HKCU\Environment can set ProgramFiles to
// a folder the user writes. Only binaries that administrators write may run elevated (spec 11).
func DefaultExe() string { return exeFrom(serviceImagePath, programFilesDir) }

// serviceImagePath reads ImagePath of HKLM\SYSTEM\CurrentControlSet\Services\Tailscale.
func serviceImagePath() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tailscale`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ImagePath")
	return v, err
}

// serviceDir is the folder of tailscaled.exe in an ImagePath: a quoted path up to its closing
// quote, an unquoted one up to tailscaled.exe. Another program, a relative path or a value that
// would need the environment to expand is not used.
func serviceDir(imagePath string) (string, bool) {
	v := strings.TrimSpace(imagePath)
	if strings.HasPrefix(v, `"`) {
		end := strings.Index(v[1:], `"`)
		if end < 0 {
			return "", false
		}
		v = v[1 : 1+end]
	} else if i := strings.Index(strings.ToLower(v), "tailscaled.exe"); i >= 0 {
		v = v[:i+len("tailscaled.exe")]
	}
	if !filepath.IsAbs(v) || strings.Contains(v, "%") || !strings.EqualFold(filepath.Base(v), "tailscaled.exe") {
		return "", false
	}
	return filepath.Dir(v), true
}

// exeFrom takes the CLI next to the service's tailscaled.exe, else from the Program Files folder.
func exeFrom(service, programFiles func() (string, error)) string {
	if v, err := service(); err == nil {
		if dir, ok := serviceDir(v); ok {
			return filepath.Join(dir, "tailscale.exe")
		}
	}
	return exeIn(programFiles)
}

// programFilesDir reads ProgramFilesDir from HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion.
func programFilesDir() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProgramFilesDir")
	return v, err
}

// exeIn builds the path from the folder read; an error, a relative path or a value that would need
// the environment to expand gives the fallback.
func exeIn(read func() (string, error)) string {
	dir, err := read()
	if err != nil || !filepath.IsAbs(dir) || strings.Contains(dir, "%") {
		dir = fallbackProgramFiles
	}
	return filepath.Join(dir, "Tailscale", "tailscale.exe")
}

// Runner runs exe with args and returns its standard output.
type Runner func(ctx context.Context, exe string, args ...string) ([]byte, error)

// Client implements engine.SystemTailscale.
type Client struct {
	Exe string
	Run Runner
}

var _ engine.SystemTailscale = (*Client)(nil)

// New uses the default install path and runs tailscale.exe without a window.
func New() *Client { return &Client{Exe: DefaultExe(), Run: run} }

func run(ctx context.Context, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %v: %s", filepath.Base(exe), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (c *Client) installed() bool {
	_, err := os.Stat(c.Exe)
	return err == nil
}

// Status reads `tailscale status --json`. Not installed: Installed false. A stopped service,
// a missing login or any other error: no exit node (5.6), with the error for the log.
func (c *Client) Status(ctx context.Context) (engine.SysStatus, error) {
	if !c.installed() {
		return engine.SysStatus{}, nil
	}
	out, err := c.Run(ctx, c.Exe, "status", "--json")
	if err != nil {
		return engine.SysStatus{Installed: true}, err
	}
	return ParseStatus(out)
}

// ParseStatus takes BackendState and ExitNodeStatus.TailscaleIPs (first IPv4, without "/32").
func ParseStatus(out []byte) (engine.SysStatus, error) {
	var s struct {
		BackendState   string
		ExitNodeStatus *struct {
			TailscaleIPs []string
		}
	}
	if err := json.Unmarshal(out, &s); err != nil {
		return engine.SysStatus{Installed: true}, fmt.Errorf("tailscale status --json: %w", err)
	}
	st := engine.SysStatus{Installed: true}
	if s.BackendState != "Running" || s.ExitNodeStatus == nil {
		return st, nil
	}
	st.ExitNodeOn = true
	for _, p := range s.ExitNodeStatus.TailscaleIPs {
		ip := strings.SplitN(p, "/", 2)[0]
		if !strings.Contains(ip, ":") {
			st.ExitNodeIP = ip
			break
		}
	}
	return st, nil
}

// AllowLAN reads ExitNodeAllowLANAccess from `tailscale debug prefs` (it is not in status, 5.3 step 2).
func (c *Client) AllowLAN(ctx context.Context) (bool, error) {
	out, err := c.Run(ctx, c.Exe, "debug", "prefs")
	if err != nil {
		return false, err
	}
	var p struct {
		ExitNodeAllowLANAccess *bool
	}
	if err := json.Unmarshal(out, &p); err != nil || p.ExitNodeAllowLANAccess == nil {
		return false, fmt.Errorf("tailscale debug prefs: no ExitNodeAllowLANAccess (%v)", err)
	}
	return *p.ExitNodeAllowLANAccess, nil
}

// SetExitNode is the restore command of 5.4 step 1.
func (c *Client) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	_, err := c.Run(ctx, c.Exe, "set", "--exit-node="+ip, "--exit-node-allow-lan-access="+strconv.FormatBool(allowLAN))
	return err
}

// ClearExitNode is step 6 of 5.3.
func (c *Client) ClearExitNode(ctx context.Context) error {
	_, err := c.Run(ctx, c.Exe, "set", "--exit-node=")
	return err
}

// RestoreCommand is the manual command shown with restore_pending (5.2).
func (c *Client) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf(`"%s" set --exit-node=%s --exit-node-allow-lan-access=%v`, c.Exe, ip, allowLAN)
}

// Dir is the install folder (rule 5 of 6.2).
func (c *Client) Dir() string { return filepath.Dir(c.Exe) }
```

- [ ] **Step 4: Запустить тест, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/systs/
```

Ожидается `ok  splitlane/helper/internal/systs`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/systs
git -C C:\code\router commit -m "feat(helper): system Tailscale client by full path" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 17: Windows и запрос внешнего адреса

`winhost` отвечает на вопросы предварительных проверок 5.3 и выключения 5.4. Решения — чистые функции над простыми данными, с тестами: маршрут по умолчанию через адаптер, который не наш и не Tailscale (`DefaultRouteOK`); пересылка IPv4 на включённом физическом адаптере (`PhysicalForwarding`; виртуальные адаптеры WSL и Hyper-V пересылают всегда и не считаются); правило NRPT системного Tailscale для всех имён (`TailscaleCatchAll`: имя `.` и сервер `100.100.100.100`). Чтение системы (`Interfaces` через `GetAdaptersAddresses`, `GetIfEntry2Ex`, `GetIpInterfaceEntry`; `DefaultRoutes` через `GetIpForwardTable2`; `ReadNRPT` из двух разделов реестра; `CountProcesses` через снимок процессов) только читает. Неизвестное состояние адаптера считается «адаптер есть»: откат не должен ошибочно доложить о чистой системе. Сброс кэша DNS — `DnsFlushResolverCache`. `RulesEnv` описывает этот компьютер для пакета `rules` (известные папки пользователя, `FileDescription` из ресурсов версии). `probe` — запрос `api.ipify.org` с тайм-аутом 5 секунд, каждый раз новым соединением: через вход ядра (`ViaProxy`) или обычным путём (`Direct`); ошибка HTTP — связь есть, адрес неизвестен.

**Files:**
- Create: `helper/internal/winhost/host.go`, `helper/internal/winhost/env.go`, `helper/internal/winhost/host_test.go`, `helper/internal/probe/probe.go`, `helper/internal/probe/probe_test.go`

**Interfaces:**
- Consumes: `engine.Host`, `engine.Prober`, `engine.ProbeResult`; `coreconfig.AdapterName`; `rules.Env`.
- Produces: `winhost.Iface{LUID uint64; Alias string; Type uint32; Up, Hardware, Forwarding bool}`, `winhost.Route{LUID uint64; PrefixLen uint8}`, `winhost.NRPTRule{Names []string; Servers string}`, `DefaultRouteOK([]Route, []Iface) bool`, `PhysicalForwarding([]Iface) bool`, `TailscaleCatchAll([]NRPTRule) bool`, `type Host struct{}` (реализует `engine.Host`), `Interfaces() ([]Iface, error)`, `DefaultRoutes() ([]Route, error)`, `ReadNRPT() []NRPTRule`, `CountProcesses(image string) (int, error)`, `RulesEnv(protected []string) rules.Env`, `FileDescription(path string) string`; `probe.Prober{Timeout time.Duration}`, `probe.New() *Prober` (реализует `engine.Prober`).

- [ ] **Step 1: Написать тесты**

Файл `helper/internal/winhost/host_test.go`, целиком:

```go
package winhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

var (
	ethernet = Iface{LUID: 1, Alias: "Ethernet", Type: 6, Up: true, Hardware: true}
	ours     = Iface{LUID: 2, Alias: "SplitLane", Type: 53, Up: true}
	ts       = Iface{LUID: 3, Alias: "Tailscale", Type: 53, Up: true}
	wsl      = Iface{LUID: 4, Alias: "vEthernet (WSL)", Type: 6, Up: true, Forwarding: true}
)

func TestDefaultRouteOK(t *testing.T) {
	ifaces := []Iface{ethernet, ours, ts}
	if !DefaultRouteOK([]Route{{LUID: 1}}, ifaces) {
		t.Fatal("a default route through Ethernet counts")
	}
	if DefaultRouteOK([]Route{{LUID: 2}, {LUID: 3}}, ifaces) {
		t.Fatal("default routes of SplitLane and Tailscale do not count")
	}
	if DefaultRouteOK([]Route{{LUID: 1, PrefixLen: 1}}, ifaces) || DefaultRouteOK(nil, ifaces) {
		t.Fatal("no default route")
	}
}

func TestPhysicalForwarding(t *testing.T) {
	if PhysicalForwarding([]Iface{ethernet, wsl}) {
		t.Fatal("forwarding on a virtual adapter (WSL, Hyper-V) does not count")
	}
	shared := ethernet
	shared.Forwarding = true
	if !PhysicalForwarding([]Iface{shared}) {
		t.Fatal("forwarding on a physical adapter counts")
	}
	shared.Up = false
	if PhysicalForwarding([]Iface{shared}) {
		t.Fatal("an adapter that is down does not count")
	}
}

func TestTailscaleCatchAll(t *testing.T) {
	if !TailscaleCatchAll([]NRPTRule{{Names: []string{".ts.net"}, Servers: "100.100.100.100"}, {Names: []string{"."}, Servers: "100.100.100.100"}}) {
		t.Fatal("the rule for all names counts")
	}
	if TailscaleCatchAll([]NRPTRule{{Names: []string{".ts.net"}, Servers: "100.100.100.100"}, {Names: []string{"."}, Servers: "10.0.0.1"}}) {
		t.Fatal("only Tailscale's rule for all names counts")
	}
}

func TestFreePortsAndSecret(t *testing.T) {
	ports, err := Host{}.FreePorts(4)
	if err != nil || len(ports) != 4 {
		t.Fatal(ports, err)
	}
	seen := map[int]bool{}
	for _, p := range ports {
		if seen[p] || p <= 0 {
			t.Fatalf("ports %v", ports)
		}
		seen[p] = true
	}
	a, b := Host{}.Secret(), Host{}.Secret()
	if len(a) != 32 || a == b {
		t.Fatalf("secrets %q %q", a, b)
	}
}

func TestFileDescription(t *testing.T) {
	win, _ := windows.GetWindowsDirectory()
	if d := FileDescription(filepath.Join(win, "System32", "kernel32.dll")); d == "" {
		t.Fatal("kernel32.dll has a description")
	}
	exe, _ := os.Executable()
	if d := FileDescription(exe); d != "" {
		t.Fatalf("the test binary has no version resources, got %q", d)
	}
}

// TestLiveReadOnly reads the real system and changes nothing. It runs with SPLITLANE_LIVE_READONLY=1
// and expects a PC with a working network, no sing-box.exe and no SplitLane adapter.
func TestLiveReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	h := Host{}
	ifaces, err := Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Fatalf("interfaces: %v", err)
	}
	t.Logf("LIVE default route %v, physical forwarding %v, NRPT catch-all %v, sing-box running %v, adapter %v",
		h.HasDefaultRoute(), h.IPForwarding(), h.DNSOverride(), h.CoreRunning(), h.AdapterPresent())
	if !h.HasDefaultRoute() || h.CoreRunning() || h.AdapterPresent() {
		t.Fatal("expected: a default route, no sing-box.exe, no SplitLane adapter")
	}
	env := RulesEnv(nil)
	if env.WindowsDir == "" || env.ProgramFiles == "" || len(env.UserDirs) != 4 || strings.Contains(strings.Join(env.UserDirs, "|"), "||") {
		t.Fatalf("env %+v", env)
	}
}
```

Файл `helper/internal/probe/probe_test.go`, целиком:

```go
package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"splitlane/helper/internal/engine"
)

func service(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestDirect(t *testing.T) {
	p := New()
	cases := []struct {
		status int
		body   string
		want   engine.ProbeResult
	}{
		{200, "203.0.113.7\n", engine.ProbeResult{Reached: true, IP: "203.0.113.7"}},
		{503, "busy", engine.ProbeResult{Reached: true}},
		{200, "<html>", engine.ProbeResult{Reached: true}},
		{200, "2001:db8::1", engine.ProbeResult{Reached: true}},
	}
	for _, c := range cases {
		srv := service(c.status, c.body)
		if got := p.Direct(context.Background(), srv.URL); got != c.want {
			t.Errorf("%d %q: got %+v", c.status, c.body, got)
		}
		srv.Close()
	}
}

func TestFailureIsNotReached(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if got := New().Direct(context.Background(), "http://"+addr+"/"); got.Reached {
		t.Fatal("a refused connection is a failure")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(time.Second) }))
	defer slow.Close()
	p := &Prober{Timeout: 100 * time.Millisecond}
	if got := p.Direct(context.Background(), slow.URL); got.Reached {
		t.Fatal("a timeout is a failure")
	}
}

// ViaProxy goes through the given local port, as through the probe inbounds of the core.
func TestViaProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "address.example" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("198.51.100.4"))
	}))
	defer proxy.Close()
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	got := New().ViaProxy(context.Background(), "http://address.example/", port)
	if got != (engine.ProbeResult{Reached: true, IP: "198.51.100.4"}) {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/winhost/ ./internal/probe/
```

Ожидается: `FAIL splitlane/helper/internal/winhost [build failed]`, `FAIL splitlane/helper/internal/probe [build failed]`, `undefined: Iface` и `undefined: New`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/winhost/host.go`, целиком:

```go
// Package winhost answers the engine's questions about Windows: routes, IP forwarding, the NRPT
// rule of the system Tailscale, a running sing-box, the adapter, the DNS cache (spec 5.3, 5.4).
// The decisions are pure functions over plain data; the readers only read the system.
package winhost

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
)

// Iface is one network interface as the decisions see it.
type Iface struct {
	LUID       uint64
	Alias      string
	Type       uint32
	Up         bool
	Hardware   bool // a physical adapter
	Forwarding bool // IPv4 forwarding enabled
}

// Route is one IPv4 route.
type Route struct {
	LUID      uint64
	PrefixLen uint8
}

// NRPTRule is one rule of the Name Resolution Policy Table.
type NRPTRule struct {
	Names   []string
	Servers string
}

// ignored adapters: the core's own and the system Tailscale's.
func ignored(i Iface) bool {
	return strings.EqualFold(i.Alias, coreconfig.AdapterName) || strings.EqualFold(i.Alias, "Tailscale") ||
		i.Type == windows.IF_TYPE_SOFTWARE_LOOPBACK
}

// DefaultRouteOK: an IPv4 default route through an adapter that is neither ours nor Tailscale's.
func DefaultRouteOK(routes []Route, ifaces []Iface) bool {
	byLUID := map[uint64]Iface{}
	for _, i := range ifaces {
		byLUID[i.LUID] = i
	}
	for _, r := range routes {
		i, ok := byLUID[r.LUID]
		if r.PrefixLen == 0 && ok && !ignored(i) {
			return true
		}
	}
	return false
}

// PhysicalForwarding: a physical adapter that is up forwards IPv4 (Internet sharing, mobile hotspot).
func PhysicalForwarding(ifaces []Iface) bool {
	for _, i := range ifaces {
		if i.Hardware && i.Up && i.Forwarding && !ignored(i) {
			return true
		}
	}
	return false
}

// TailscaleCatchAll: the system Tailscale's NRPT rule for all names ("."), served by 100.100.100.100.
func TailscaleCatchAll(rules []NRPTRule) bool {
	for _, r := range rules {
		for _, n := range r.Names {
			if n == "." && strings.Contains(r.Servers, "100.100.100.100") {
				return true
			}
		}
	}
	return false
}

// Host implements engine.Host on the real system.
type Host struct{}

var _ engine.Host = Host{}

func (Host) HasDefaultRoute() bool {
	ifaces, err := Interfaces()
	if err != nil {
		return false
	}
	routes, err := DefaultRoutes()
	if err != nil {
		return false
	}
	return DefaultRouteOK(routes, ifaces)
}

func (Host) IPForwarding() bool {
	ifaces, err := Interfaces()
	return err == nil && PhysicalForwarding(ifaces)
}

func (Host) DNSOverride() bool { return TailscaleCatchAll(ReadNRPT()) }

func (Host) CoreRunning() bool {
	n, _ := CountProcesses("sing-box.exe")
	return n > 0
}

func (Host) AdapterPresent() bool {
	ifaces, err := Interfaces()
	if err != nil {
		return true // unknown counts as present: the rollback must not report a clean system by mistake
	}
	for _, i := range ifaces {
		if strings.EqualFold(i.Alias, coreconfig.AdapterName) {
			return true
		}
	}
	return false
}

var (
	dnsapi                    = windows.NewLazySystemDLL("dnsapi.dll")
	procDnsFlushResolverCache = dnsapi.NewProc("DnsFlushResolverCache")
)

// FlushDNS empties the Windows DNS cache (spec 5.3 step 7, 6.4).
func (Host) FlushDNS() error {
	if r, _, err := procDnsFlushResolverCache.Call(); r == 0 {
		return err
	}
	return nil
}

// FreePorts takes n free ports on 127.0.0.1 at once and lets them go.
func (Host) FreePorts(n int) ([]int, error) {
	var ports []int
	var ls []net.Listener
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// Secret is a fresh random secret for the Clash API or the API service (spec 6.1).
func (Host) Secret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Interfaces reads every interface with its IPv4 forwarding flag.
func Interfaces() ([]Iface, error) {
	size := uint32(16 * 1024)
	var buf []byte
	for {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) || size <= uint32(len(buf)) {
			return nil, err
		}
	}
	var out []Iface
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		i := Iface{LUID: a.Luid, Alias: windows.UTF16PtrToString(a.FriendlyName), Type: a.IfType, Up: a.OperStatus == windows.IfOperStatusUp}
		row := windows.MibIfRow2{InterfaceLuid: a.Luid}
		if windows.GetIfEntry2Ex(windows.MibIfTableNormalWithoutStatistics, &row) == nil {
			i.Hardware = row.InterfaceAndOperStatusFlags&1 != 0
		}
		ip := windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: a.Luid}
		if windows.GetIpInterfaceEntry(&ip) == nil {
			i.Forwarding = ip.ForwardingEnabled != 0
		}
		out = append(out, i)
	}
	return out, nil
}

// DefaultRoutes reads the IPv4 routes with prefix length 0.
func DefaultRoutes() ([]Route, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	var out []Route
	for _, r := range table.Rows() {
		if r.DestinationPrefix.PrefixLength == 0 {
			out = append(out, Route{LUID: r.InterfaceLuid, PrefixLen: 0})
		}
	}
	return out, nil
}

// ReadNRPT reads the NRPT rules of both places Windows keeps them.
func ReadNRPT() []NRPTRule {
	var out []NRPTRule
	for _, path := range []string{
		`SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`,
		`SOFTWARE\Policies\Microsoft\Windows NT\DNSClient\DnsPolicyConfig`,
	} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sk, err := registry.OpenKey(k, n, registry.READ)
			if err != nil {
				continue
			}
			var r NRPTRule
			r.Names, _, _ = sk.GetStringsValue("Name")
			r.Servers, _, _ = sk.GetStringValue("GenericDNSServers")
			sk.Close()
			out = append(out, r)
		}
		k.Close()
	}
	return out
}

// CountProcesses counts running processes with this image name (case-insensitive).
func CountProcesses(image string) (int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), image) {
			n++
		}
	}
	return n, nil
}
```

Файл `helper/internal/winhost/env.go`, целиком:

```go
package winhost

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/rules"
)

// RulesEnv describes this computer for package rules; protected are the full paths of the
// helper, the core and the system Tailscale processes (spec 8.2).
func RulesEnv(protected []string) rules.Env {
	win, _ := windows.GetWindowsDirectory()
	known := func(id *windows.KNOWNFOLDERID) string {
		p, err := windows.KnownFolderPath(id, 0)
		if err != nil {
			return ""
		}
		return p
	}
	return rules.Env{
		WindowsDir:      win,
		ProgramFiles:    known(windows.FOLDERID_ProgramFiles),
		ProgramFilesX86: known(windows.FOLDERID_ProgramFilesX86),
		UserDirs: []string{known(windows.FOLDERID_Desktop), known(windows.FOLDERID_Downloads),
			known(windows.FOLDERID_Documents), strings.TrimRight(os.TempDir(), `\`)},
		Protected: protected,
		Describe:  FileDescription,
	}
}

// FileDescription is the FileDescription of the version resources of path, or "".
func FileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	lang := "040904b0"
	var trans *[2]uint16
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&trans), &n) == nil && n >= 4 {
		lang = fmt.Sprintf("%04x%04x", trans[0], trans[1])
	}
	var text *uint16
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\`+lang+`\FileDescription`, unsafe.Pointer(&text), &n) != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString(text))
}
```

Файл `helper/internal/probe/probe.go`, целиком:

```go
// Package probe asks the address service from which address a request arrives (spec 5.3 steps
// 5 and 8, 5.5). Every check is a new connection with a 5 s timeout.
package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"splitlane/helper/internal/engine"
)

// Prober implements engine.Prober.
type Prober struct {
	Timeout time.Duration
}

var _ engine.Prober = (*Prober)(nil)

// New returns a prober with the 5 s timeout of 5.5.
func New() *Prober { return &Prober{Timeout: 5 * time.Second} }

// ViaProxy sends the request through a probe inbound of the core on 127.0.0.1:port.
func (p *Prober) ViaProxy(ctx context.Context, target string, port int) engine.ProbeResult {
	proxy := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	return p.get(ctx, target, &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true})
}

// Direct is the helper's own request in the ordinary way, through the adapter when it is up.
func (p *Prober) Direct(ctx context.Context, target string) engine.ProbeResult {
	return p.get(ctx, target, &http.Transport{Proxy: nil, DisableKeepAlives: true})
}

// get: a timeout or a connection error is a failure; an HTTP error or a body without an IPv4
// address is a link that works with an unknown address (5.5).
func (p *Prober) get(ctx context.Context, target string, tr *http.Transport) engine.ProbeResult {
	defer tr.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return engine.ProbeResult{}
	}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return engine.ProbeResult{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return engine.ProbeResult{Reached: true}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	text := strings.TrimSpace(string(body))
	if addr, err := netip.ParseAddr(text); err != nil || !addr.Is4() {
		return engine.ProbeResult{Reached: true}
	}
	return engine.ProbeResult{Reached: true, IP: text}
}
```

- [ ] **Step 4: Запустить тесты, убедиться в успехе**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/winhost/ ./internal/probe/
```

Ожидается `ok` для обоих пакетов (`TestLiveReadOnly` пропущен: переменная не задана).

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/winhost helper/internal/probe
git -C C:\code\router commit -m "feat(helper): Windows host checks and the external address probe" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 18: Именованный канал

Канал 5.1 на `go-winio`: описатель безопасности задаётся явно (SYSTEM, администраторы, пользователь настройки; `P` — без наследуемых записей), удалённые клиенты отвергаются (`FILE_PIPE_REJECT_REMOTE_CLIENTS` в `go-winio`), второй экземпляр с тем же именем не создаётся (одна копия служебного процесса). Первое сообщение — `hello`; несовпадение версии — `protocol_mismatch` с полем `protocol`, после него принимается только `helper.shutdown`. Несколько клиентов одновременно; событие `state` — всем, кто сказал `hello`. Клиент, который не читает, отключается, а не блокирует служебный процесс (очередь 64 сообщения). Счётчик клиентов передаётся автомату (`SetClients` атомарен и безопасен под замком канала: так нет взаимной блокировки «автомат публикует состояние ↔ канал считает клиентов»). `Close` не теряет ответов: служебный процесс закрывает канал сразу после `helper.shutdown`, когда ответ на него может быть ещё в пути. Поэтому `Close` перестаёт принимать запросы, ждёт, пока ответы на уже принятые встанут в очереди, дописывает очереди и только потом закрывает соединения; каждое ожидание ограничено (1 и 2 секунды), клиент, который не читает, закрывается всё равно.

**Files:**
- Create: `helper/internal/pipe/server.go`, `helper/internal/pipe/server_test.go`
- Modify: `helper/go.mod`, `helper/go.sum` (командами `go get` и `go mod tidy`)

**Interfaces:**
- Consumes: `protocol.*`; `engine.(*Engine).Handle`, `Hello`, `SetClients` (через интерфейс `Handler`).
- Produces: константа `DefaultName` (значение `\\.\pipe\SplitLane.Helper`); `type Handler interface{ Handle(ctx, method string, params json.RawMessage) (any, *protocol.Error); Hello() protocol.HelloResult; SetClients(n int) }`; `SDDL(userSID string) string`; `Listen(name, userSID string, h Handler, log func(string, ...any)) (*Server, error)`; `(*Server).Serve() error`, `Close()` (ответы в пути и очереди дописываются, затем соединения закрываются), `Broadcast(protocol.State)`. Тест `TestShutdownReplyIsNotLost`: 200 кругов, обработчик закрывает сервер прямо из `helper.shutdown`, а клиент всё равно читает ответ и затем конец канала.

- [ ] **Step 1: Добавить зависимость**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go get github.com/Microsoft/go-winio@v0.6.4
```

Ожидается `go: added github.com/Microsoft/go-winio v0.6.4`.

- [ ] **Step 2: Написать тест**

Файл `helper/internal/pipe/server_test.go`, целиком:

```go
package pipe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/protocol"
)

type fakeHandler struct {
	mu      sync.Mutex
	methods []string
	clients int
}

func (f *fakeHandler) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods = append(f.methods, method)
	if method == "nope" {
		return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
	}
	return map[string]string{"ok": method}, nil
}

func (f *fakeHandler) Hello() protocol.HelloResult {
	return protocol.HelloResult{Protocol: 1, Helper: "0.1.0", Core: "1.14.4"}
}

func (f *fakeHandler) SetClients(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clients = n
}

func (f *fakeHandler) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clients
}

func currentSID(t *testing.T) string {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return u.User.Sid.String()
}

// serve starts a server on a pipe name of its own; the pipe disappears with the test.
func serve(t *testing.T) (*Server, *fakeHandler, string) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	h := &fakeHandler{}
	s, err := Listen(name, currentSID(t), h, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(s.Close)
	return s, h, name
}

type testClient struct {
	conn net.Conn
	r    *bufio.Reader
}

func dial(t *testing.T, name string) *testClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &testClient{conn: conn, r: bufio.NewReader(conn)}
}

func (c *testClient) send(t *testing.T, line string) map[string]any {
	t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	return c.next(t)
}

func (c *testClient) next(t *testing.T) map[string]any {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	text, err := c.r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	return m
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestSDDL(t *testing.T) {
	if got := SDDL("S-1-5-21-1-2-3-1001"); got != "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;S-1-5-21-1-2-3-1001)" {
		t.Fatal(got)
	}
}

func TestHelloFirst(t *testing.T) {
	_, h, name := serve(t)
	c := dial(t, name)
	if code := errCode(c.send(t, `{"id":1,"method":"state.get"}`)); code != protocol.CodeBadRequest {
		t.Fatalf("before hello: %s", code)
	}
	m := c.send(t, `{"id":2,"method":"hello","params":{"protocol":1}}`)
	res, _ := m["result"].(map[string]any)
	if m["id"].(float64) != 2 || res["core"] != "1.14.4" || res["protocol"].(float64) != 1 {
		t.Fatalf("hello: %v", m)
	}
	if code := errCode(c.send(t, `{"id":3,"method":"nope"}`)); code != protocol.CodeUnknownMethod {
		t.Fatalf("unknown method: %s", code)
	}
	if code := errCode(c.send(t, `not json`)); code != protocol.CodeBadRequest {
		t.Fatalf("garbage: %s", code)
	}
	if h.count() != 1 {
		t.Fatalf("clients %d", h.count())
	}
}

// After a protocol mismatch only helper.shutdown is accepted; the error carries the helper's version.
func TestProtocolMismatch(t *testing.T) {
	_, h, name := serve(t)
	c := dial(t, name)
	m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":2}}`)
	e, _ := m["error"].(map[string]any)
	if errCode(m) != protocol.CodeProtocolMismatch || e["protocol"].(float64) != 1 {
		t.Fatalf("mismatch: %v", m)
	}
	if code := errCode(c.send(t, `{"id":2,"method":"state.get"}`)); code != protocol.CodeProtocolMismatch {
		t.Fatalf("after mismatch: %s", code)
	}
	if m := c.send(t, `{"id":3,"method":"helper.shutdown"}`); m["result"] == nil {
		t.Fatalf("shutdown after mismatch: %v", m)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.Join(h.methods, ",") != "helper.shutdown" {
		t.Fatalf("handler saw %v", h.methods)
	}
}

// Events go to every client that said hello; several clients at once (spec 5.1).
func TestBroadcastToEveryClient(t *testing.T) {
	s, h, name := serve(t)
	a, b, silent := dial(t, name), dial(t, name), dial(t, name)
	a.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`)
	b.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`)
	_ = silent
	deadline := time.Now().Add(5 * time.Second)
	for h.count() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.Broadcast(protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}})
	for _, c := range []*testClient{a, b} {
		m := c.next(t)
		data, _ := m["data"].(map[string]any)
		if m["event"] != "state" || data["phase"] != "on" {
			t.Fatalf("event %v", m)
		}
	}
	a.conn.Close()
	for h.count() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.count() != 1 {
		t.Fatalf("clients after a close: %d", h.count())
	}
}

// Review Focus 4: a window that hangs and stops reading is dropped; the helper does not block
// and the other clients keep getting events.
func TestStalledClientIsDropped(t *testing.T) {
	s, h, name := serve(t)
	stalled, live := dial(t, name), dial(t, name)
	stalled.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":1}}` + "\n")) // never reads again
	live.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`)
	big := protocol.State{Phase: protocol.PhaseOn}
	for i := 0; i < 40; i++ {
		big.Apps = append(big.Apps, protocol.AppRule{ID: fmt.Sprint(i), Name: strings.Repeat("x", 250), Exe: strings.Repeat("y", 250)})
	}
	got := make(chan struct{}, 1)
	go func() {
		for {
			live.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, err := live.r.ReadString('\n'); err != nil {
				close(got)
				return
			}
			got <- struct{}{}
		}
	}()
	// ~20 KB per event: the stalled client's pipe buffer (64 KB) and queue (64) fill up long before 150.
	// Events go out one at a time, as the helper sends them, so the reading client keeps up.
	finished := make(chan string, 1)
	go func() {
		for i := 0; i < 150; i++ {
			s.Broadcast(big)
			select {
			case _, ok := <-got:
				if !ok {
					finished <- fmt.Sprintf("the reading client lost the pipe after %d events", i)
					return
				}
			case <-time.After(5 * time.Second):
				finished <- fmt.Sprintf("the reading client got no event %d", i)
				return
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
		t.Fatal("Broadcast blocked on a client that does not read")
	}
	if h.count() != 1 {
		t.Fatalf("clients %d: the stalled one must be dropped", h.count())
	}
}

// exitOnShutdown is the helper's exit: on helper.shutdown it closes the server at once, while the
// answer is still on its way (cmd/splitlane-helper: OnExit, then Close).
type exitOnShutdown struct {
	fakeHandler
	srv *Server
}

func (h *exitOnShutdown) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	if method == protocol.MethodShutdown {
		go h.srv.Close()
	}
	return h.fakeHandler.Handle(ctx, method, params)
}

// The answer to helper.shutdown reaches the client although the helper closes the pipe right
// away: Close writes the queues out before it closes the connections (slctl shutdown, Task 22, 23).
func TestShutdownReplyIsNotLost(t *testing.T) {
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
		h := &exitOnShutdown{}
		s, err := Listen(name, currentSID(t), h, nil)
		if err != nil {
			t.Fatal(err)
		}
		h.srv = s
		go s.Serve()
		c := dial(t, name)
		c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`)
		c.conn.Write([]byte(`{"id":2,"method":"helper.shutdown"}` + "\n"))
		c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		text, err := c.r.ReadString('\n')
		if err != nil || !strings.Contains(text, `"id":2`) || !strings.Contains(text, `"result"`) {
			t.Fatalf("round %d: the answer was lost: %q %v", i, text, err)
		}
		if _, err := c.r.ReadString('\n'); err == nil {
			t.Fatalf("round %d: the pipe must close after the answer", i)
		}
		c.conn.Close()
		s.Close()
	}
}

// A second helper cannot take the same pipe: one instance at a time.
func TestSecondListenFails(t *testing.T) {
	_, _, name := serve(t)
	if _, err := Listen(name, currentSID(t), &fakeHandler{}, nil); err == nil {
		t.Fatal("the pipe name is taken")
	}
}
```

- [ ] **Step 3: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/pipe/
```

Ожидается: `FAIL splitlane/helper/internal/pipe [build failed]` и `undefined: Server`.

- [ ] **Step 4: Реализовать**

Файл `helper/internal/pipe/server.go`, целиком:

```go
// Package pipe is the named pipe of spec 5.1: JSON lines, hello first, events to every client.
package pipe

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"

	"splitlane/helper/internal/protocol"
)

// DefaultName is the pipe of the product.
const DefaultName = `\\.\pipe\SplitLane.Helper`

// Handler is what the server serves (the engine).
type Handler interface {
	Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error)
	Hello() protocol.HelloResult
	SetClients(n int)
}

// SDDL gives read and write access to SYSTEM, the administrators and the setup user only (spec 5.1, 11).
// "P" protects the list from inherited entries.
func SDDL(userSID string) string {
	return "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + userSID + ")"
}

// Server serves one pipe name.
type Server struct {
	h         Handler
	l         net.Listener
	log       func(format string, args ...any)
	mu        sync.Mutex
	clients   map[*client]bool
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
func Listen(name, userSID string, h Handler, log func(string, ...any)) (*Server, error) {
	l, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: SDDL(userSID),
		InputBufferSize:    64 * 1024,
		OutputBufferSize:   64 * 1024,
	})
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
		c := &client{conn: conn, out: make(chan []byte, 64)}
		s.wg.Add(2)
		go s.write(c)
		go s.read(c)
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
	s.mu.Lock()
	var conns []net.Conn
	for c := range s.clients {
		conns = append(conns, c.conn)
		c.conn.SetWriteDeadline(time.Now().Add(closeWait))
		s.end(c, false)
	}
	s.mu.Unlock()
	if !waitFor(&s.wg, 2*closeWait) {
		for _, conn := range conns {
			conn.Close()
		}
	}
}

// waitFor waits for wg at most d; false when the time ran out.
func waitFor(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// Broadcast sends the state event to every client that said hello (spec 5.1).
func (s *Server) Broadcast(st protocol.State) {
	line := protocol.EncodeEvent("state", st)
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if c.hello {
			s.send(c, line)
		}
	}
}

// send queues a line; a client that does not read is dropped instead of blocking the helper.
// s.mu must be held.
func (s *Server) send(c *client, line []byte) {
	if c.ended {
		return
	}
	select {
	case c.out <- line:
	default:
		s.log("pipe: a client does not read its messages; dropping it")
		s.drop(c)
	}
}

// drop closes a client at once; s.mu must be held.
func (s *Server) drop(c *client) { s.end(c, true) }

// end closes the queue of a client once: at once with the connection (now), or after the writer
// has written what is queued (Close). s.mu must be held.
func (s *Server) end(c *client, now bool) {
	if !c.ended {
		c.ended = true
		close(c.out)
	}
	if now {
		c.conn.Close()
	}
	if s.clients[c] {
		delete(s.clients, c)
		s.h.SetClients(s.countHello())
	}
}

func (s *Server) countHello() int {
	n := 0
	for c := range s.clients {
		if c.hello {
			n++
		}
	}
	return n
}

func (s *Server) write(c *client) {
	defer s.wg.Done()
	defer c.conn.Close() // the queue was ended: everything in it is written
	for line := range c.out {
		if _, err := c.conn.Write(line); err != nil {
			s.mu.Lock()
			s.drop(c)
			s.mu.Unlock()
			return
		}
	}
}

func (s *Server) reply(c *client, line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[c] {
		s.send(c, line)
	}
}

func (s *Server) read(c *client) {
	defer s.wg.Done()
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.end(c, !s.closing) // while closing, the writer writes the queue out first
		s.mu.Unlock()
	}()
	sc := protocol.NewScanner(c.conn)
	for sc.Scan() {
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			return
		}
		s.answering.Add(1)
		s.mu.Unlock()
		var req protocol.Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil || req.Method == "" {
			s.reply(c, protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "not a request")))
		} else {
			s.reply(c, s.answer(c, req))
		}
		s.answering.Done()
	}
}

// answer applies the hello rules of 5.1 and passes everything else to the handler.
func (s *Server) answer(c *client, req protocol.Request) []byte {
	s.mu.Lock()
	hello, mismatch := c.hello, c.mismatch
	s.mu.Unlock()
	if req.Method == protocol.MethodHello {
		var p protocol.HelloParams
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return protocol.EncodeError(req.ID, err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if p.Protocol != protocol.Version {
			c.mismatch, c.hello = true, false
			s.h.SetClients(s.countHello())
			return protocol.EncodeError(req.ID, &protocol.Error{Code: protocol.CodeProtocolMismatch,
				Message: "the helper speaks another protocol version", Protocol: protocol.Version})
		}
		c.mismatch, c.hello = false, true
		s.h.SetClients(s.countHello())
		return protocol.EncodeResult(req.ID, s.h.Hello())
	}
	switch {
	case mismatch && req.Method != protocol.MethodShutdown:
		return protocol.EncodeError(req.ID, &protocol.Error{Code: protocol.CodeProtocolMismatch,
			Message: "only helper.shutdown is accepted after a protocol mismatch", Protocol: protocol.Version})
	case !hello && !mismatch:
		return protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "the first message must be hello"))
	}
	res, err := s.h.Handle(context.Background(), req.Method, req.Params)
	if err != nil {
		return protocol.EncodeError(req.ID, err)
	}
	return protocol.EncodeResult(req.ID, res)
}
```

- [ ] **Step 5: Привести модуль в порядок, запустить тесты**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go mod tidy; Get-Content go.mod; go vet ./...; go test ./internal/pipe/ ./internal/engine/
```

Ожидается `go.mod`:

```text
module splitlane/helper

go 1.26.0

require golang.org/x/sys v0.49.0

require github.com/Microsoft/go-winio v0.6.4
```

и `ok` для обоих пакетов. В `go.sum` добавились две строки `github.com/Microsoft/go-winio v0.6.4 h1:kJF2pamtYCWXf0xAT8sAkwKC0pMl2HYBrRPHAqyUc+g=` и `github.com/Microsoft/go-winio v0.6.4/go.mod h1:1xl5JXG4ze/m74GH6Dc4Mwxfc3VL5siRVQjIAlHH07U=`.

- [ ] **Step 6: Закоммитить**

```powershell
git -C C:\code\router add helper/go.mod helper/go.sum helper/internal/pipe
git -C C:\code\router commit -m "feat(helper): named pipe with an explicit security descriptor, hello gating and events" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 19: Окно завершения сеанса и программа служебного процесса

`session.Watch` создаёт скрытое окно верхнего уровня (не окно только для сообщений: оно не получает рассылку `WM_QUERYENDSESSION`) на своём потоке и вызывает обработчик из `WM_ENDSESSION` с `wParam = TRUE` до возврата из сообщения — тот же способ, что проверен прототипом задачи 3. `cmd/splitlane-helper` связывает всё: разбор флагов (в этой сборке только `--dev` с абсолютными `--data-dir` и `--core`; `--dev-pin` — закрепление программ исполнителя; `--pipe` — имя канала), журнал `logs\helper.log` (прежний — в `.prev`), проверка SHA-256 ядра (`VerifyCore`) до любых действий, канал с SID пользователя процесса, обработка консольного прерывания (служебный процесс не должен завершиться от случайного события), окно завершения сеанса, `Start`, `Run`, `Serve`; выход — после `helper.shutdown` (канал перед выходом дописывает ответ на него, задача 18). Коды выхода: 0 — штатно, 2 — флаги, 3 — канал занят (работает другой служебный процесс), 4 — не то ядро, 5 — внутренняя ошибка.

**Files:**
- Create: `helper/internal/session/session.go`, `helper/internal/session/session_test.go`, `helper/cmd/splitlane-helper/main.go`, `helper/cmd/splitlane-helper/main_test.go`

**Interfaces:**
- Consumes: все пакеты `internal`; `engine.New`, `engine.Deps`, `engine.Settings`, `(*Engine).Start`, `Run`, `SessionEnding`; `pipe.Listen`, `(*Server).Serve`, `Broadcast`, `Close`; `singbox.VerifyCore`, `singbox.Driver`, `singbox.CoreVersion`; `systs.New`; `winhost.Host`, `winhost.RulesEnv`; `probe.New`; `store.Dir`.
- Produces: `session.Watch(onEnd func(), log func(string, ...any)) (*Watcher, error)`, `(*Watcher).Stop()`; программа `splitlane-helper.exe --dev --data-dir <папка> --core <sing-box.exe> [--dev-pin] [--pipe <имя>]`.

- [ ] **Step 1: Написать тесты**

Файл `helper/internal/session/session_test.go`, целиком:

```go
package session

import (
	"sync/atomic"
	"testing"
	"time"
)

// The hidden window answers WM_QUERYENDSESSION with "yes" and calls the handler on WM_ENDSESSION
// with wParam TRUE only, before the message returns.
func TestEndSessionCallsTheHandler(t *testing.T) {
	var calls atomic.Int32
	w, err := Watch(func() { calls.Add(1) }, t.Logf)
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

func TestTwoWatchersAndStop(t *testing.T) {
	a, err := Watch(func() {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Watch(func() {}, nil)
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

Файл `helper/cmd/splitlane-helper/main_test.go`, целиком:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	if _, err := parse([]string{"--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}); err == nil {
		t.Fatal("this build needs --dev")
	}
	if _, err := parse([]string{"--dev", "--data-dir", `data`, "--core", `C:\c\sing-box.exe`}); err == nil {
		t.Fatal("relative paths are refused")
	}
	o, err := parse([]string{"--dev", "--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`, "--pipe", `\\.\pipe\x`})
	if err != nil || !o.devPin || o.pipe != `\\.\pipe\x` {
		t.Fatalf("%+v %v", o, err)
	}
	if o, _ := parse([]string{"--dev", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}); o.pipe != `\\.\pipe\SplitLane.Helper` || o.devPin {
		t.Fatalf("defaults %+v", o)
	}
}

// A file that is not the pinned core is refused before anything starts (spec 4.3).
func TestRunRefusesAWrongCore(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "sing-box.exe")
	os.WriteFile(core, []byte("not the core"), 0o644)
	if code := run([]string{"--dev", "--data-dir", filepath.Join(dir, "data"), "--core", core}); code != exitBadCore {
		t.Fatalf("exit code %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "data", "logs", "helper.log"))
	if !strings.Contains(string(b), "core refused") {
		t.Fatalf("log:\n%s", b)
	}
	if code := run(nil); code != exitUsage {
		t.Fatalf("no arguments: %d", code)
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./internal/session/ ./cmd/splitlane-helper/
```

Ожидается: `FAIL ... [build failed]` для обоих пакетов и `undefined: Watch`.

- [ ] **Step 3: Реализовать**

Файл `helper/internal/session/session.go`, целиком:

```go
// Package session tells the helper that the Windows session ends (logoff, restart, shutdown).
// A process that loaded user32.dll gets no console event for it; it gets WM_QUERYENDSESSION and
// WM_ENDSESSION in a top-level window (spec 5.4). A message-only window does not get them.
package session

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSendMessageW     = user32.NewProc("SendMessageW")
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       [2]int32
	LPrivate uint32
}

// Watcher owns the hidden window.
type Watcher struct {
	hwnd uintptr
}

var (
	handlers sync.Map // hwnd -> *handler
	wndProc  = syscall.NewCallback(proc)
	class    sync.Once
	classErr error
)

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
		case wmDestroy:
			handlers.Delete(hwnd)
			procPostQuitMessage.Call(0)
			return 0
		}
	}
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
	ready := make(chan error, 1)
	w := &Watcher{}
	go func() {
		runtime.LockOSThread()
		var instance windows.Handle
		if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
			ready <- err
			return
		}
		class.Do(func() {
			wc := wndClassEx{WndProc: wndProc, Instance: instance, ClassName: className}
			wc.Size = uint32(unsafe.Sizeof(wc))
			if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
				classErr = err
			}
		})
		if classErr != nil {
			ready <- classErr
			return
		}
		// WS_OVERLAPPED, never shown: a top-level window without a taskbar entry
		hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("SplitLane"))), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
		if hwnd == 0 {
			ready <- err
			return
		}
		handlers.Store(hwnd, &handler{onEnd: onEnd, log: log})
		w.hwnd = hwnd
		ready <- nil
		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	if err := <-ready; err != nil {
		return nil, errors.Join(errors.New("session watch window"), err)
	}
	return w, nil
}

// Stop closes the window and ends its thread.
func (w *Watcher) Stop() {
	if w != nil && w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, wmClose, 0, 0)
	}
}

// send delivers a message to the window and waits for its answer (tests).
func (w *Watcher) send(message, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(w.hwnd, message, wParam, lParam)
	return r
}
```

Файл `helper/cmd/splitlane-helper/main.go`, целиком:

```go
// Command splitlane-helper is the elevated helper of SplitLane (spec 5). It is built without a
// console window (-H=windowsgui) and starts the core without a window.
//
// This build (Plan 1A) runs only in development mode, from an elevated process, with paths in the
// repository: --dev --data-dir <folder> --core <sing-box.exe>. The install command and the
// scheduled task of spec 4.3 come with Plan 1B.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/probe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/session"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/systs"
	"splitlane/helper/internal/winhost"
)

var version = "0.1.0-dev"

// Exit codes; the process has no console, so they and helper.log are its only output.
const (
	exitOK        = 0
	exitUsage     = 2
	exitPipeTaken = 3
	exitBadCore   = 4
	exitInternal  = 5
)

const pipeTakenLabel = "the pipe already exists: another helper runs"

type options struct {
	dev     bool
	dataDir string
	core    string
	pipe    string
	devPin  bool
}

func parse(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("splitlane-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.dev, "dev", false, "development mode (required in this build)")
	fs.StringVar(&o.dataDir, "data-dir", "", "data folder (absolute)")
	fs.StringVar(&o.core, "core", "", "full path of sing-box.exe 1.14.4")
	fs.StringVar(&o.pipe, "pipe", pipe.DefaultName, "pipe name")
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	switch {
	case !o.dev:
		return o, fmt.Errorf("this build runs only with --dev")
	case !filepath.IsAbs(o.dataDir) || !filepath.IsAbs(o.core):
		return o, fmt.Errorf("--data-dir and --core must be absolute paths")
	}
	return o, nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func run(args []string) int {
	o, err := parse(args)
	if err != nil {
		return exitUsage
	}
	paths := coreconfig.PathsFor(o.dataDir)
	if err := os.MkdirAll(paths.LogsDir, 0o755); err != nil {
		return exitUsage
	}
	logPath := filepath.Join(paths.LogsDir, "helper.log")
	_ = os.Rename(logPath, logPath+".prev")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return exitUsage
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
	logf := func(format string, a ...any) { logger.Printf(format, a...) }
	logf("splitlane-helper %s starting, dev=%v dev-pin=%v", version, o.dev, o.devPin)

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
	sys := systs.New()
	protected := []string{helperPath, o.core, filepath.Join(sys.Dir(), "tailscaled.exe"), filepath.Join(sys.Dir(), "tailscale-ipn.exe")}
	host, _ := os.Hostname()
	exit := make(chan struct{})
	var srv *pipe.Server
	eng := engine.New(engine.Deps{
		Clock: realClock{},
		Store: store.Dir{Path: o.dataDir},
		Core:  &singbox.Driver{Exe: o.core, DataDir: o.dataDir},
		Sys:   sys,
		Probe: probe.New(),
		Host:  winhost.Host{},
		Rules: winhost.RulesEnv(protected),
		Settings: engine.Settings{HelperPath: helperPath, Hostname: host + "-splitlane",
			DevPin: o.devPin, HelperVersion: version, CoreVersion: singbox.CoreVersion},
		Log: logf,
		OnState: func(s protocol.State) {
			code := "-"
			if s.Reason != nil {
				code = s.Reason.Code
			}
			logf("state: phase=%s reason=%s allDirect=%v", s.Phase, code, s.AllDirect)
			srv.Broadcast(s)
		},
		OnExit: func() { close(exit) },
	})
	srv, err = pipe.Listen(o.pipe, user.User.Sid.String(), eng, logf)
	if err != nil {
		logf("%s (%v)", pipeTakenLabel, err)
		return exitPipeTaken
	}
	// A stray console event must never end the helper: it is handled here, not by the default handler.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for range interrupts {
			logf("console interrupt ignored")
		}
	}()
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
}
```

- [ ] **Step 4: Запустить тесты, собрать программу**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./internal/session/ ./cmd/splitlane-helper/; go build -ldflags "-H=windowsgui" -o bin\splitlane-helper.exe .\cmd\splitlane-helper; "built: $(Test-Path bin\splitlane-helper.exe)"
```

Ожидается `ok` для обоих пакетов и `built: True`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/internal/session helper/cmd/splitlane-helper
git -C C:\code\router commit -m "feat(helper): session-end window and the helper program in development mode" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 20: Тестовый клиент канала `slctl`

Клиент, через который каждое поведение служебного процесса проверяется без окна: все методы 5.1 (`state`, `enable`, `disable [-skip-restore]`, `alldirect on|off`, `lanes FILE`, `exitnode IP`, `clear-system-exit-node`, `shutdown`, `call METHOD [PARAMS|@FILE]`), ожидание фазы (`wait on,error`), печать событий (`watch`), проверка дорожек переименованными копиями `curl.exe` (`expect-lanes EXE=LANE ...`: адрес из ответа сервиса сравнивается с `state.addresses`; правило оценки плана 0 — первый ответ верный или два из трёх) и запись `state.json` для узла, который уже вошёл в сеть (`seed`, только папка разработки: входа в плане 1A ещё нет). `reset-dev DATA-DIR` (только папка разработки с существующим `state.json`) снимает «было включено», стирает идентичность ядра и удаляет `restore.json` — след сеанса, который сторож прервал при включённом делении (задача 23, шаг 1). Канала, которого ещё нет, `slctl` ждёт: только что запущенный служебный процесс открывает канал через долю секунды (журнал, SHA-256 ядра, автомат), а `go-winio` повторяет попытку только для занятого канала (`ERROR_PIPE_BUSY`), отсутствующий (`ERROR_FILE_NOT_FOUND`) он сразу возвращает ошибкой. Поэтому `slctl` повторяет подключение каждые 200 мс, пока не выйдет `-timeout`, но не дольше 30 секунд; неизвестная команда — ошибка использования сразу, без канала. Адреса в выводе закрыты, пока нет `-show-addresses`. Папку продукта `seed` и `reset-dev` узнают по известной папке `ProgramData` (`KnownFolderPath`), а не по переменной окружения; тесты подставляют вместо неё временную папку, поэтому ошибка в `devFolder` не может ничего записать в настоящий `%ProgramData%`. Неверный JSON параметров — сразу ошибка использования (Review Focus 5). Коды выхода: 0 — успех, 1 — ошибка ответа или итоговая фаза не та, 2 — использование или нет канала, 3 — время ожидания вышло. Тестовый клиент плана 1 написан на Go; сценарная проверка `scripts/scenario.mjs` из раздела 12 — в плане 1B.

**Files:**
- Create: `helper/cmd/slctl/main.go`, `helper/cmd/slctl/main_test.go`

**Interfaces:**
- Consumes: `pipe.DefaultName`, `pipe.Listen` (тест), `protocol.*`, `store.Dir`, `store.DefaultProbeURL`.
- Produces: программа `slctl.exe [-pipe NAME] [-timeout 150s] [-show-addresses] [-url URL] COMMAND [ARGS]`; последняя строка ожидающих команд — `SLCTL: phase=... reason=... allDirect=... defaultLane=... apps=N vpn=<ip> direct=<ip> systemExitNode=... restoreOnDisable=...`; строки `LANE-OK <exe> <lane>` / `LANE-WRONG <exe> want=<lane> seen=[...]`; `SEED-OK state.json written`; `RESET-OK wasOn=false core=none restore.json absent`; внутренние `dial(name string, wait time.Duration) (net.Conn, error)`, `programData` (переменная-функция: известная папка `ProgramData`), `devFolder(dir string) bool`, `resetDev(args []string, out io.Writer) int`, `egress` (переменная-функция: запуск копии `curl.exe`). Тесты: `TestWaitsForThePipe` (канал создаётся через 500 мс после начала `slctl state`), `TestSeed` и `TestResetDev` (папка продукта — во временной папке вместо `ProgramData`), `TestExpectLanes` (ответы копий по сценарию вместо `curl.exe`: `LANE-OK` по первому верному ответу, `LANE-WRONG` при двух неверных из трёх, отказ при равных адресах дорожек, ошибка использования без `=` или с неизвестной дорожкой), в `TestErrorsAndRejections` — отсутствующий канал ждётся до `-timeout 1s`, неизвестная команда отвергается сразу.

- [ ] **Step 1: Написать тест**

Файл `helper/cmd/slctl/main_test.go`, целиком:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func TestVerdict(t *testing.T) {
	cases := []struct {
		seen     []string
		ok, done bool
	}{
		{[]string{"vpn"}, true, true},
		{[]string{"none", "vpn"}, true, true},
		{[]string{"direct"}, false, false},
		{[]string{"direct", "vpn", "vpn"}, true, true},
		{[]string{"direct", "vpn", "direct"}, false, true},
		{[]string{"none", "none", "none", "none", "none", "none"}, false, true},
	}
	for _, c := range cases {
		if ok, done := verdict(c.seen, "vpn"); ok != c.ok || done != c.done {
			t.Errorf("%v: %v %v", c.seen, ok, done)
		}
	}
}

func TestLaneName(t *testing.T) {
	v, d := "203.0.113.1", "198.51.100.1"
	s := protocol.State{Addresses: protocol.Addresses{VPN: &v, Direct: &d}}
	for ip, want := range map[string]string{v: "vpn", d: "direct", "192.0.2.9": "other", "": "none"} {
		if got := laneName(ip, s); got != want {
			t.Errorf("%q: %s", ip, got)
		}
	}
}

// fakeProgramData points the product root at a temporary folder for the test and returns the
// product's data folder under it: a regression then writes only into a temporary folder.
func fakeProgramData(t *testing.T) string {
	t.Helper()
	if real := programData(); !filepath.IsAbs(real) {
		t.Fatalf("the known folder ProgramData: %q", real)
	}
	root := t.TempDir()
	old := programData
	programData = func() string { return root }
	t.Cleanup(func() { programData = old })
	return filepath.Join(root, "SplitLane")
}

func TestSeed(t *testing.T) {
	product := fakeProgramData(t)
	var out strings.Builder
	if code := run([]string{"seed", product, "100.64.0.1"}, &out); code != exitUsage {
		t.Fatalf("the product folder must be refused: %d %s", code, out.String())
	}
	if _, err := os.Stat(product); err == nil {
		t.Fatal("nothing may be written into the product folder")
	}
	dir := t.TempDir()
	if code := run([]string{"seed", dir, "192.0.2.1"}, &out); code != exitUsage {
		t.Fatal("an address outside the tailnet must be refused")
	}
	if code := run([]string{"seed", dir, "100.64.0.1", "pc-splitlane-dev"}, &out); code != exitOK {
		t.Fatalf("%d %s", code, out.String())
	}
	s, _ := store.Dir{Path: dir}.LoadState()
	if !s.LoggedIn || s.SelectedExitNode != "100.64.0.1" || s.DeviceName != "pc-splitlane-dev" {
		t.Fatalf("%+v", s)
	}
}

// fakeHelper answers like the engine: enable goes starting -> on through state events.
type fakeHelper struct {
	mu  sync.Mutex
	srv *pipe.Server
	st  protocol.State
}

func (f *fakeHelper) set(phase string) {
	f.mu.Lock()
	f.st.Phase = phase
	s := f.st
	f.mu.Unlock()
	f.srv.Broadcast(s)
}

func (f *fakeHelper) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	switch method {
	case protocol.MethodStateGet:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.st, nil
	case protocol.MethodEnable:
		f.set(protocol.PhaseStarting)
		go func() {
			time.Sleep(50 * time.Millisecond)
			f.set(protocol.PhaseOn)
		}()
		return struct{}{}, nil
	case protocol.MethodLanesSet:
		return nil, &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected",
			Rules: []protocol.RuleRejection{{ID: "x", Reason: protocol.RejectProtected}}}
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
}

func (f *fakeHelper) Hello() protocol.HelloResult { return protocol.HelloResult{Protocol: 1} }
func (f *fakeHelper) SetClients(int)              {}

func testPipeName() string { return fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63()) }

// serveFake opens the pipe name with the fake helper behind it.
func serveFake(name string) (*pipe.Server, error) {
	vpn, direct := "203.0.113.1", "198.51.100.1"
	return serveState(name, protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{},
		Addresses: protocol.Addresses{VPN: &vpn, Direct: &direct}})
}

// serveState opens the pipe name with a fake helper in state st.
func serveState(name string, st protocol.State) (*pipe.Server, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	f := &fakeHelper{st: st}
	srv, err := pipe.Listen(name, u.User.Sid.String(), f, nil)
	if err != nil {
		return nil, err
	}
	f.srv = srv
	go srv.Serve()
	return srv, nil
}

func fakePipe(t *testing.T) string {
	t.Helper()
	name := testPipeName()
	srv, err := serveFake(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return name
}

// A helper that was just launched opens its pipe a moment later (Task 23, step 7): slctl waits.
func TestWaitsForThePipe(t *testing.T) {
	name := testPipeName()
	opened := make(chan *pipe.Server, 1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		srv, _ := serveFake(name)
		opened <- srv
	}()
	var out strings.Builder
	code := run([]string{"-pipe", name, "-timeout", "10s", "state"}, &out)
	srv := <-opened
	if srv == nil {
		t.Fatal("the test pipe was not created")
	}
	defer srv.Close()
	if code != exitOK || !strings.Contains(out.String(), "SLCTL: phase=off") {
		t.Fatalf("%d\n%s", code, out.String())
	}
}

// reset-dev clears "was on", the core identity and the journal of a development folder only.
func TestResetDev(t *testing.T) {
	product := fakeProgramData(t)
	var out strings.Builder
	if code := run([]string{"reset-dev", product}, &out); code != exitUsage {
		t.Fatalf("the product folder must be refused: %d %s", code, out.String())
	}
	dir := t.TempDir()
	if code := run([]string{"reset-dev", dir}, &out); code != exitUsage {
		t.Fatal("a folder without state.json must be refused")
	}
	d := store.Dir{Path: dir}
	s := store.Defaults()
	s.LoggedIn, s.SelectedExitNode, s.WasOn = true, "100.64.0.1", true
	s.Core = &store.CoreIdentity{PID: 42, Created: 7, Path: `C:\core\sing-box.exe`}
	d.SaveState(s)
	d.SaveJournal(store.Journal{ExitNode: "100.64.0.1", AllowLAN: true})
	out.Reset()
	if code := run([]string{"reset-dev", dir}, &out); code != exitOK || !strings.Contains(out.String(), "RESET-OK") {
		t.Fatalf("%d %s", code, out.String())
	}
	got, _ := d.LoadState()
	j, _ := d.LoadJournal()
	if got.WasOn || got.Core != nil || j != nil || !got.LoggedIn || got.SelectedExitNode != "100.64.0.1" {
		t.Fatalf("state %+v journal %+v", got, j)
	}
}

// expect-lanes (Task 23, steps 5, 6, 8): the address of each copy against state.addresses by the
// Plan 0 rule; the copies of curl.exe are played by a scripted egress.
func TestExpectLanes(t *testing.T) {
	name := fakePipe(t)
	var mu sync.Mutex
	answers := map[string][]string{
		`C:\x\curl-vpn.exe`:    {"203.0.113.1"},                                // right at once
		`C:\x\curl-direct.exe`: {"203.0.113.1", "198.51.100.1", "203.0.113.1"}, // two of three wrong
	}
	old := egress
	egress = func(exe, url string) string {
		mu.Lock()
		defer mu.Unlock()
		next := answers[exe]
		if len(next) == 0 {
			return ""
		}
		answers[exe] = next[1:]
		return next[0]
	}
	t.Cleanup(func() { egress = old })
	var out strings.Builder
	code := run([]string{"-pipe", name, "expect-lanes", `C:\x\curl-vpn.exe=vpn`, `C:\x\curl-direct.exe=direct`}, &out)
	text := out.String()
	if code != exitFailed || !strings.Contains(text, "LANE-OK curl-vpn.exe vpn") ||
		!strings.Contains(text, "LANE-WRONG curl-direct.exe want=direct seen=[vpn direct vpn]") {
		t.Fatalf("%d\n%s", code, text)
	}
	for _, bad := range []string{`C:\x\curl-vpn.exe`, `C:\x\curl-vpn.exe=tunnel`} {
		if code := run([]string{"-pipe", name, "expect-lanes", bad}, &out); code != exitUsage {
			t.Errorf("%s: %d", bad, code)
		}
	}
	same := "203.0.113.1"
	equal := testPipeName()
	srv, err := serveState(equal, protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{},
		Addresses: protocol.Addresses{VPN: &same, Direct: &same}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	out.Reset()
	if code := run([]string{"-pipe", equal, "expect-lanes", `C:\x\curl-vpn.exe=vpn`}, &out); code != exitFailed ||
		!strings.Contains(out.String(), "lane addresses are unknown or equal") {
		t.Fatalf("equal addresses: %d\n%s", code, out.String())
	}
}

func TestStateIsMasked(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "state"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	if strings.Contains(out.String(), "203.0.113.1") || !strings.Contains(out.String(), "SLCTL: phase=off") || !strings.Contains(out.String(), "vpn=<ip>") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	run([]string{"-pipe", name, "-show-addresses", "state"}, &out)
	if !strings.Contains(out.String(), "vpn=203.0.113.1") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestEnableWaitsForOn(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "-timeout", "10s", "enable"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "EVENT state phase=starting") || !strings.Contains(text, "SLCTL: phase=on") {
		t.Fatalf("output:\n%s", text)
	}
}

func TestWaitForAPhase(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "-timeout", "10s", "wait", "off,error"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	if code := run([]string{"-pipe", name, "-timeout", "2s", "wait", "on"}, &out); code != exitTimeout {
		t.Fatalf("a phase that never comes is a timeout: %d\n%s", code, out.String())
	}
}

func TestErrorsAndRejections(t *testing.T) {
	name := fakePipe(t)
	file := filepath.Join(t.TempDir(), "lanes.json")
	os.WriteFile(file, []byte(`{"apps":[],"defaultLane":"vpn"}`), 0o644)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "lanes", file}, &out); code != exitFailed || !strings.Contains(out.String(), "ERROR rule_rejected rules rejected rules: x=protected") {
		t.Fatalf("%d\n%s", code, out.String())
	}
	out.Reset()
	// PowerShell 5.1 loses the quotes: {on:true} is not JSON and must not hang the client
	if code := run([]string{"-pipe", name, "-timeout", "5s", "call", "routing.allDirect", "{on:true}"}, &out); code != exitUsage {
		t.Fatalf("invalid params: %d\n%s", code, out.String())
	}
	params := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(params, []byte(`{"on":true}`), 0o644)
	if code := run([]string{"-pipe", name, "call", "routing.allDirect", "@" + params}, &out); code != exitFailed || !strings.Contains(out.String(), "ERROR unknown_method") {
		t.Fatalf("params from a file: %d\n%s", code, out.String())
	}
	start := time.Now()
	if code := run([]string{"-pipe", `\\.\pipe\SplitLane.Missing.0`, "-timeout", "1s", "state"}, &out); code != exitUsage {
		t.Fatalf("no pipe: %d", code)
	}
	if waited := time.Since(start); waited < 500*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("a missing pipe is waited for up to -timeout, waited %s", waited)
	}
	start = time.Now()
	if code := run([]string{"frobnicate"}, &out); code != exitUsage || time.Since(start) > time.Second {
		t.Fatalf("an unknown command is a usage error at once, without a pipe: %d", code)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slctl/
```

Ожидается: `FAIL splitlane/helper/cmd/slctl [build failed]` и `undefined: verdict`.

- [ ] **Step 3: Реализовать**

Файл `helper/cmd/slctl/main.go`, целиком:

```go
// Command slctl is the test client of the helper's pipe (Plan 1): every method of spec 5.1, waiting
// for phases, a lane check with renamed copies of curl.exe, and a development seed of state.json.
// Addresses are masked in the output unless -show-addresses is given.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

const usage = `usage: slctl [-pipe NAME] [-timeout 150s] [-show-addresses] [-url URL] COMMAND [ARGS]
  state                          print the state
  enable                         routing.enable, wait for on or error
  disable [-skip-restore]        routing.disable, wait for off or error
  alldirect on|off               routing.allDirect, wait for the flag
  lanes FILE                     lanes.set with the params in FILE (JSON)
  exitnode IP                    tailscale.setExitNode
  clear-system-exit-node         systemTailscale.clearExitNode
  shutdown                       helper.shutdown, wait until the pipe closes
  call METHOD [PARAMS|@FILE]     any method, params as JSON or from a file
  wait PHASE[,PHASE...]          wait for one of the phases; success when it is the first one
  watch [SECONDS]                print events
  expect-lanes EXE=LANE ...      run copies of curl.exe, compare their address with state.addresses
  seed DATA-DIR EXIT-NODE [HOSTNAME]   development only: state.json of a logged-in node
  reset-dev DATA-DIR             development only: clear "was on", the core identity and restore.json
A pipe that does not exist yet is waited for, up to -timeout and at most 30 s.`

// Exit codes.
const (
	exitOK      = 0
	exitFailed  = 1 // an error response, a final phase error, a lane on the wrong address
	exitUsage   = 2
	exitTimeout = 3
)

// pipeCommands are the commands that talk to the helper.
var pipeCommands = []string{"state", "enable", "disable", "alldirect", "lanes", "exitnode", "clear-system-exit-node",
	"shutdown", "call", "wait", "watch", "expect-lanes"}

// A helper that was just started opens its pipe a moment later (its log, the SHA-256 of the core,
// the engine). go-winio retries only a busy pipe, so slctl tries a missing one again itself.
const (
	dialWaitMax = 30 * time.Second
	dialEvery   = 200 * time.Millisecond
)

// dial connects to the pipe; while the pipe does not exist it tries again every 200 ms for wait.
func dial(name string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		conn, err := winio.DialPipeContext(ctx, name)
		cancel()
		if err == nil || !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || !time.Now().Add(dialEvery).Before(deadline) {
			return conn, err
		}
		time.Sleep(dialEvery)
	}
}

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
	text, err := c.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return nil, err
	}
	if ev, ok := m["event"]; ok {
		var name string
		json.Unmarshal(ev, &name)
		if name == "state" {
			var s protocol.State
			json.Unmarshal(m["data"], &s)
			c.state = &s
			c.print("EVENT state %s", summary(s))
		} else {
			c.print("EVENT %s", name)
		}
	}
	return m, nil
}

// call sends a request and waits for its response.
func (c *client) call(method string, params any) (json.RawMessage, *protocol.Error, error) {
	c.nextID++
	req := map[string]any{"id": c.nextID, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	if _, err := c.conn.Write(append(b, '\n')); err != nil {
		return nil, nil, err
	}
	for {
		m, err := c.read()
		if err != nil {
			return nil, nil, err
		}
		var id int
		if raw, ok := m["id"]; !ok || json.Unmarshal(raw, &id) != nil || id != c.nextID {
			continue
		}
		if raw, ok := m["error"]; ok {
			var e protocol.Error
			json.Unmarshal(raw, &e)
			c.print("ERROR %s %s%s", e.Code, e.Message, rulesText(e.Rules))
			return nil, &e, nil
		}
		c.print("RESULT %s", m["result"])
		return m["result"], nil, nil
	}
}

func rulesText(rules []protocol.RuleRejection) string {
	var parts []string
	for _, r := range rules {
		parts = append(parts, r.ID+"="+r.Reason)
	}
	if len(parts) == 0 {
		return ""
	}
	return " rules: " + strings.Join(parts, ",")
}

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

func str(p *string) string {
	if p == nil {
		return "null"
	}
	return *p
}

// fetchState asks for the state now.
func (c *client) fetchState() (protocol.State, error) {
	raw, perr, err := c.call(protocol.MethodStateGet, nil)
	if err != nil {
		return protocol.State{}, err
	}
	if perr != nil {
		return protocol.State{}, perr
	}
	var s protocol.State
	json.Unmarshal(raw, &s)
	c.state = &s
	return s, nil
}

// waitFor reads events until cond holds for the state, after asking for it once.
func (c *client) waitFor(cond func(protocol.State) bool) (protocol.State, error) {
	s, err := c.fetchState()
	if err != nil {
		return s, err
	}
	for !cond(s) {
		if _, err := c.read(); err != nil {
			return s, err
		}
		s = *c.state
	}
	return s, nil
}

func phaseIn(phases ...string) func(protocol.State) bool {
	return func(s protocol.State) bool {
		for _, p := range phases {
			if s.Phase == p {
				return true
			}
		}
		return false
	}
}

func run(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("slctl", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("pipe", pipe.DefaultName, "pipe name")
	timeout := fs.Duration("timeout", 150*time.Second, "how long to wait for an answer or a phase")
	show := fs.Bool("show-addresses", false, "print addresses as they are")
	url := fs.String("url", store.DefaultProbeURL, "address service for expect-lanes")
	fs.Usage = func() { fmt.Fprintln(out, usage) }
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch {
	case cmd == "seed":
		return seed(rest, out)
	case cmd == "reset-dev":
		return resetDev(rest, out)
	case !slices.Contains(pipeCommands, cmd):
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	conn, err := dial(*name, min(*timeout, dialWaitMax))
	if err != nil {
		fmt.Fprintf(out, "SLCTL: cannot connect to %s: %v\n", *name, err)
		return exitUsage
	}
	defer conn.Close()
	c := &client{conn: conn, r: bufio.NewReader(conn), out: out, show: *show, timeout: time.Now().Add(*timeout)}
	if _, perr, err := c.call(protocol.MethodHello, protocol.HelloParams{Protocol: protocol.Version}); err != nil || perr != nil {
		fmt.Fprintf(out, "SLCTL: hello failed: %v %v\n", err, perr)
		return exitFailed
	}
	code := command(c, cmd, rest, *url)
	if errors.Is(c.lastErr, os.ErrDeadlineExceeded) {
		code = exitTimeout
	}
	return code
}

// finish prints the final line; a read error (a timeout) gives exit code 3.
func (c *client) finish(s protocol.State, err error, ok func(protocol.State) bool) int {
	if err != nil {
		c.lastErr = err
		c.print("SLCTL: TIMEOUT or broken pipe (%v), last state %s", err, summary(s))
		return exitTimeout
	}
	c.print("SLCTL: %s", summary(s))
	if !ok(s) {
		return exitFailed
	}
	return exitOK
}

func command(c *client, cmd string, args []string, url string) int {
	simple := func(method string, params any) int {
		if _, perr, err := c.call(method, params); err != nil || perr != nil {
			return exitFailed
		}
		return exitOK
	}
	switch cmd {
	case "state":
		s, err := c.fetchState()
		return c.finish(s, err, func(protocol.State) bool { return true })
	case "enable":
		if code := simple(protocol.MethodEnable, nil); code != exitOK {
			return code
		}
		s, err := c.waitFor(phaseIn(protocol.PhaseOn, protocol.PhaseError, protocol.PhaseDegraded, protocol.PhaseBlocked))
		return c.finish(s, err, phaseIn(protocol.PhaseOn))
	case "disable":
		fs := flag.NewFlagSet("disable", flag.ContinueOnError)
		skip := fs.Bool("skip-restore", false, "disable without restoring the system exit node")
		if fs.Parse(args) != nil {
			return exitUsage
		}
		if code := simple(protocol.MethodDisable, protocol.DisableParams{SkipRestore: *skip}); code != exitOK {
			return code
		}
		s, err := c.waitFor(func(s protocol.State) bool {
			return phaseIn(protocol.PhaseOff, protocol.PhaseError)(s) ||
				(s.Phase == protocol.PhaseStopping && s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending)
		})
		return c.finish(s, err, phaseIn(protocol.PhaseOff))
	case "alldirect":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			return exitUsage
		}
		on := args[0] == "on"
		before, err := c.fetchState()
		if err != nil {
			return c.finish(before, err, nil)
		}
		if code := simple(protocol.MethodAllDirect, map[string]bool{"on": on}); code != exitOK {
			return code
		}
		// the apply takes about 3 s; the next state event carries its result
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			if s, err := c.fetchState(); err != nil || s.AllDirect == on {
				return c.finish(s, err, func(s protocol.State) bool { return s.AllDirect == on })
			}
		}
		s, _ := c.fetchState()
		return c.finish(s, nil, func(s protocol.State) bool { return s.AllDirect == on })
	case "lanes":
		if len(args) != 1 {
			return exitUsage
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			c.print("SLCTL: %v", err)
			return exitUsage
		}
		return simple(protocol.MethodLanesSet, json.RawMessage(b))
	case "exitnode":
		if len(args) != 1 {
			return exitUsage
		}
		return simple(protocol.MethodSetExitNode, protocol.SetExitNodeParams{ID: args[0]})
	case "clear-system-exit-node":
		return simple(protocol.MethodClearSystemExitNode, nil)
	case "shutdown":
		if code := simple(protocol.MethodShutdown, nil); code != exitOK {
			return code
		}
		for {
			if _, err := c.read(); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					c.lastErr = err
					c.print("SLCTL: TIMEOUT the helper did not exit")
					return exitTimeout
				}
				c.print("SLCTL: the helper closed the pipe")
				return exitOK
			}
		}
	case "call":
		if len(args) < 1 || len(args) > 2 {
			return exitUsage
		}
		var params any
		if len(args) == 2 {
			raw := []byte(args[1])
			if strings.HasPrefix(args[1], "@") {
				b, err := os.ReadFile(args[1][1:])
				if err != nil {
					c.print("SLCTL: %v", err)
					return exitUsage
				}
				raw = b
			}
			if !json.Valid(raw) {
				// Windows PowerShell 5.1 drops the double quotes of a native argument: use @FILE there
				c.print("SLCTL: params are not valid JSON; write them to a file and pass @FILE")
				return exitUsage
			}
			params = json.RawMessage(raw)
		}
		return simple(args[0], params)
	case "wait":
		if len(args) != 1 {
			return exitUsage
		}
		phases := strings.Split(args[0], ",")
		s, err := c.waitFor(phaseIn(phases...))
		return c.finish(s, err, phaseIn(phases[0]))
	case "watch":
		seconds := 60
		if len(args) == 1 {
			fmt.Sscan(args[0], &seconds)
		}
		c.timeout = time.Now().Add(time.Duration(seconds) * time.Second)
		for {
			if _, err := c.read(); err != nil {
				return exitOK
			}
		}
	case "expect-lanes":
		return expectLanes(c, args, url)
	}
	fmt.Fprintln(c.out, usage)
	return exitUsage
}

// laneName says which lane an address belongs to.
func laneName(ip string, s protocol.State) string {
	switch {
	case ip == "":
		return "none"
	case s.Addresses.VPN != nil && ip == *s.Addresses.VPN:
		return protocol.LaneVPN
	case s.Addresses.Direct != nil && ip == *s.Addresses.Direct:
		return protocol.LaneDirect
	}
	return "other"
}

// verdict judges answers only (Plan 0 rule): the first answer is right, or two of three are.
// More requests are needed while it returns done=false.
func verdict(seen []string, want string) (ok, done bool) {
	var answers []string
	for _, s := range seen {
		if s != "none" {
			answers = append(answers, s)
		}
	}
	switch {
	case len(answers) > 0 && answers[0] == want:
		return true, true
	case len(answers) >= 3:
		n := 0
		for _, a := range answers[:3] {
			if a == want {
				n++
			}
		}
		return n >= 2, true
	case len(seen) >= 6:
		return false, true
	}
	return false, false
}

// egress runs a copy of curl.exe against the address service, past any proxy.
var egress = func(exe, url string) string {
	cmd := exec.Command(exe, "-4", "-s", "--noproxy", "*", "--max-time", "10", url)
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func expectLanes(c *client, args []string, url string) int {
	s, err := c.fetchState()
	if err != nil {
		return c.finish(s, err, nil)
	}
	if s.Addresses.VPN == nil || s.Addresses.Direct == nil || *s.Addresses.VPN == *s.Addresses.Direct {
		c.print("SLCTL: lane addresses are unknown or equal; nothing to compare")
		return exitFailed
	}
	code := exitOK
	for _, a := range args {
		exe, want, ok := strings.Cut(a, "=")
		if !ok || (want != protocol.LaneVPN && want != protocol.LaneDirect) {
			return exitUsage
		}
		var seen []string
		good, done := false, false
		for !done {
			seen = append(seen, laneName(egress(exe, url), s))
			good, done = verdict(seen, want)
		}
		if good {
			c.print("LANE-OK %s %s", filepath.Base(exe), want)
		} else {
			c.print("LANE-WRONG %s want=%s seen=%v", filepath.Base(exe), want, seen)
			code = exitFailed
		}
	}
	return code
}

// programData is the root of the product's data folder: the known folder ProgramData, never the
// environment variable. The tests point it at a temporary folder.
var programData = func() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0); err == nil && p != "" {
		return p
	}
	return `C:\ProgramData`
}

// devFolder: an absolute folder outside %ProgramData% (the product's data folder is never touched).
func devFolder(dir string) bool {
	return filepath.IsAbs(dir) && !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(programData()))
}

// seed writes state.json for a node that is already logged in (development only, Plan 1A has no
// login yet). It refuses the product data folder.
func seed(args []string, out io.Writer) int {
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	dir := args[0]
	if !devFolder(dir) {
		fmt.Fprintln(out, "SLCTL: seed takes an absolute development folder, never %ProgramData%")
		return exitUsage
	}
	if err := protocol.ValidateExitNodeID(args[1], nil); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitUsage
	}
	d := store.Dir{Path: dir}
	s, err := d.LoadState()
	if err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	s.LoggedIn, s.SelectedExitNode = true, args[1]
	if len(args) == 3 {
		s.DeviceName = args[2]
	}
	if err := d.SaveState(s); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	fmt.Fprintln(out, "SEED-OK state.json written")
	return exitOK
}

// resetDev clears what a run that the guard ended while on leaves in a development data folder:
// "was on", the core identity and the restore journal. Without it the next helper would restore
// and enable again on its own at start (5.7). Only after the system was found restored (Task 23).
func resetDev(args []string, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	dir := args[0]
	if !devFolder(dir) {
		fmt.Fprintln(out, "SLCTL: reset-dev takes an absolute development folder, never %ProgramData%")
		return exitUsage
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		fmt.Fprintf(out, "SLCTL: no state.json in %s\n", dir)
		return exitUsage
	}
	d := store.Dir{Path: dir}
	s, err := d.LoadState()
	if err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	s.WasOn, s.Core = false, nil
	if err := d.SaveState(s); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	if err := d.DeleteJournal(); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	fmt.Fprintln(out, "RESET-OK wasOn=false core=none restore.json absent")
	return exitOK
}
```

- [ ] **Step 4: Запустить тест, собрать**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test ./cmd/slctl/; go build -o bin\slctl.exe .\cmd\slctl; "built: $(Test-Path bin\slctl.exe)"
```

Ожидается `ok  splitlane/helper/cmd/slctl` и `built: True`.

- [ ] **Step 5: Закоммитить**

```powershell
git -C C:\code\router add helper/cmd/slctl
git -C C:\code\router commit -m "feat(helper): slctl test client for the helper pipe" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 21: Сторож живых шагов `slguard` и его команды PowerShell

Независимый сторож (правило безопасности 2). Запускается с правами администратора раньше служебного процесса и сам его запускает (один запрос UAC на всё). Ничего не берёт из `internal/`: свой код для `tailscale.exe`, процессов и адаптера, чтобы ошибка служебного процесса не была и его ошибкой. При старте проверяет исходную точку (свежий heartbeat, нет `sing-box.exe` и `splitlane-helper.exe`, системный exit node включён) и запоминает адрес exit node и признак LAN в `guard-state.json`. Срабатывает, когда `deadline.txt` не обновлён вовремя (дедлайн не дальше 900 секунд от записи файла), появился `stop.flag`, прошёл предел 60 минут или пути VPN нет совсем: системный exit node выключен и ни один `sing-box.exe` не работает дольше 10 секунд (чтение раз в два такта, около 4 секунд; неудачное чтение ничего не меняет). Ни один шаг задачи 23 через это состояние не проходит: 5.3 запускает ядро до выключения exit node, 5.4 возвращает exit node до остановки ядра, после `kill-helper` ядро продолжает работать. Без этой проверки все программы ПК шли бы напрямую до срабатывания heartbeat (до 400 секунд), а если API исполнителя доступен напрямую — без конца. При срабатывании сторож завершает служебный процесс (он единственный, кто выключает exit node) и запущенные им `tailscale.exe` (команда `tailscale set --exit-node=`, начатая служебным процессом на шаге 6 включения или в `clearSystemExitNode`, не входит в задание Windows и переживает родителя; завершаются только процессы `tailscale.exe`, чей родитель — завершённый служебный процесс; служба `tailscaled.exe` и значок `tailscale-ipn.exe` не трогаются), возвращает системный exit node и ждёт подтверждения (повтор каждые 5 секунд без конца). Команда, отправленная до завершения `tailscale.exe`, всё равно может дойти до службы позже, поэтому сторож читает exit node каждые 2 секунды (до 60 секунд), пока он не прочитается включённым три раза подряд, — ещё при работающем ядре: опоздавшая команда тогда даёт заблокированный трафик (exit node включён, адаптер ядра есть), а не утечку; выключен — включает снова (с тем же подтверждением). Только потом сторож завершает `sing-box.exe`, ждёт исчезновения адаптера и повторяет те же три чтения; `tripped system_ok=true` пишется только после них, без ядра и без адаптера. Каждый `tailscale set`, отданный сторожем, записывается в `guard.log` строкой `set the exit node back`. Каждый вызов `tailscale.exe` ограничен 30 секундами: зависший CLI не держит сторож в `tripping`. Путь `tailscale.exe` — из каталога Program Files, который называет реестр (`HKLM`, `ProgramFilesDir`), а не `%ProgramFiles%` из окружения пользователя (сторож работает с правами администратора; так же в задаче 16). Команды через `cmd.json`: `kill-helper` (имитация сбоя служебного процесса), `launch-helper` (тот же запуск снова), `finish` (конец: система должна быть восстановлена, иначе срабатывание). Самопроверка — модульные тесты на подменной системе с часами, которые не ждут. `p1lib.ps1` — команды PowerShell для живых шагов по образцу `lib.ps1` плана 0.

**Files:**
- Create: `helper/cmd/slguard/guard.go`, `helper/cmd/slguard/system.go`, `helper/cmd/slguard/main.go`, `helper/cmd/slguard/guard_test.go`, `C:\code\router\spike\plan1\p1lib.ps1`

**Interfaces:**
- Consumes: только стандартная библиотека, `golang.org/x/sys/windows` и `golang.org/x/sys/windows/registry`.
- Produces: программа `slguard.exe run -dir DIR [-cap 60m] [-- HELPER COMMAND LINE]`, коды выхода 0 (`finished`), 3 (`refused`), 1 (`tripped`); интерфейс `System` (`ExitNode`, `AllowLAN`, `SetExitNode`, `Kill(image string) []uint32` — идентификаторы завершённых процессов, `KillChildren(parents []uint32, image string) int`, `Count`, `AdapterPresent`, `Launch`), константы `HelperImage`, `CoreImage`, `TailscaleCLI`, внутренние `restoreExitNode(reason string)`, `settle(reason, when string) bool`, `noVPNPath(now time.Time) bool` (константы `noPathGrace = 10 * time.Second`, `pathEvery = 2`), `withImage`, `childrenOf`, `programFiles() string`; файлы в `DIR`: `deadline.txt` (секунды Unix, пишет исполнитель), `stop.flag`, `cmd.json` (`{"action": "kill-helper" | "launch-helper" | "finish", "id": "..."}`), `status.json` (`{"run", "seq", "state": "starting" | "armed" | "refused" | "tripping" | "restore_blocked" | "tripped" | "finished", "time", "reason", "cmd_id", "result", "system_ok", "attempt", "manual"}`), `guard-state.json`, `run.txt` (идентификатор запуска, пишет `Start-P1Guard`), `guard.log` (адреса закрыты). PowerShell (после `. C:\code\router\spike\plan1\p1lib.ps1`): `Set-P1Heartbeat`, `Test-P1DevDataLeft`, `Test-P1Ready`, `Wait-P1Pipe [-Seconds 60]` (`$true`, когда служебный процесс отвечает в канале), `Assert-P1Armed` (`$true`, только если процесс `slguard` жив и `status.json` — этого запуска и в состоянии `armed`; иначе печатает `P1-NO-GUARD: ...`), `Start-P1Guard [-DryRun]`, `Send-P1Guard -Action kill-helper|launch-helper|finish`, `Stop-P1Guard` (без процесса `slguard` сразу печатает `P1-RESULT: NO-LIVE-GUARD`), `Get-P1Guard`, `Get-P1SystemExitNode`, `Test-P1SystemRestored`; итоговые строки `P1-RESULT: ...`, `P1-READY` / `P1-NOT-READY: ...`, `P1-NO-GUARD: ...`, `SYSTEM-RESTORED` / `SYSTEM-WRONG: ...`.

| Строка `P1-RESULT:` | Что значит | Действие |
|---|---|---|
| `state=armed` | сторож работает, служебный процесс запущен | продолжать |
| `UAC-CANCELLED` | ничего не запущено | спросить пользователя; без согласия не повторять |
| `REFUSED - starting point not ready` или `state=refused reason=...` | сторож отказался до любых изменений | устранить причину; новый запуск — новый запрос UAC |
| `HELPER-PIPE-MISSING` | служебный процесс не открыл канал | `Stop-P1Guard`, затем читать `C:\code\router\.dev\data\logs\helper.log` |
| `state=tripped ... system_ok=True` | сторож сработал, система возвращена | причина в `reason`; работа продолжается только с новым запуском |
| `state=tripped ... system_ok=False` или `NO-TERMINAL-STATE` | система могла остаться изменённой | показать пользователю ручное восстановление |
| `state=restore_blocked ...` | сторож не может вернуть exit node и повторяет попытку каждые 5 секунд (`error=` — текст ошибки `tailscale.exe`) | показать пользователю ручное восстановление; новых команд сторожу не отправлять, `Get-P1Guard` раз в минуту до `tripped` |
| `no status` | сторож не ответил на команду за 120 секунд или его `status.json` не читается | `Get-P1Guard`; показать пользователю ручное восстановление |
| `state=finished system_ok=True` | конец сеанса, система в исходном состоянии | продолжать |
| `NO-LIVE-GUARD` | сторож уже закончил (`Send-P1Guard`) или его процесса нет (`Stop-P1Guard` отвечает так сразу, без ожидания) | `Get-P1Guard`, прочитать причину; служебный процесс без сторожа выключают и завершают через `slctl` (блок «Если что-то пошло не так» задачи 23) |
| `P1-NO-GUARD: ...` (от `Assert-P1Armed`) | сторож этого запуска не работает или не в `armed` | шаг не выполнять ни в какой части; блок «Если что-то пошло не так» задачи 23 |

- [ ] **Step 1: Написать тест сторожа**

Файл `helper/cmd/slguard/guard_test.go`, целиком:

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSystem: the exit node is off while "routing is on"; the processes are counters.
type fakeSystem struct {
	mu             sync.Mutex
	events         []string
	exitOn         bool
	exitIP         string
	ignoreSet      int // so many SetExitNode calls change nothing
	helpers        int
	cores          int
	adapter        bool
	launchErr      error
	loseOnCoreKill bool             // a late `set --exit-node=` of the helper lands just when the core is ended
	clock          func() time.Time // the guard's virtual clock, for the time of the trip
	helperKilledAt time.Time        // the first `kill splitlane-helper.exe` on that clock
}

func (f *fakeSystem) add(s string) { f.events = append(f.events, s) }

func (f *fakeSystem) ExitNode() (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exitOn {
		return true, f.exitIP, nil
	}
	return false, "", nil
}

func (f *fakeSystem) AllowLAN() (bool, error) { return true, nil }

func (f *fakeSystem) SetExitNode(ip string, lan bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(fmt.Sprintf("set %s %v", ip, lan))
	if f.ignoreSet > 0 {
		f.ignoreSet--
		return errors.New("tailscaled not ready")
	}
	f.exitOn, f.exitIP = true, ip
	return nil
}

func ids(n int) []uint32 {
	out := []uint32{}
	for i := 0; i < n; i++ {
		out = append(out, uint32(100+i))
	}
	return out
}

func (f *fakeSystem) Kill(image string) []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add("kill " + image)
	switch image {
	case HelperImage:
		if f.clock != nil && f.helperKilledAt.IsZero() {
			f.helperKilledAt = f.clock()
		}
		n := f.helpers
		f.helpers = 0
		return ids(n)
	case CoreImage:
		n := f.cores
		f.cores, f.adapter = 0, false
		if f.loseOnCoreKill {
			f.exitOn, f.loseOnCoreKill = false, false
		}
		return ids(n)
	}
	return nil
}

func (f *fakeSystem) KillChildren(parents []uint32, image string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(fmt.Sprintf("kill %s children of %d", image, len(parents)))
	return 0
}

func (f *fakeSystem) Count(image string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if image == HelperImage {
		return f.helpers
	}
	return f.cores
}

func (f *fakeSystem) AdapterPresent() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adapter
}

func (f *fakeSystem) Launch(cmd []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add("launch " + strings.Join(cmd, " "))
	if f.launchErr != nil {
		return f.launchErr
	}
	f.helpers = 1
	return nil
}

func (f *fakeSystem) list() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.events, "|")
}

// newGuard: virtual time that moves only in Sleep; script runs at every tick (it plays the world).
func newGuard(t *testing.T, sys *fakeSystem, script func(g *Guard, tick int)) *Guard {
	t.Helper()
	dir := t.TempDir()
	now := time.Now()
	g := &Guard{Dir: dir, Sys: sys, HardCap: time.Hour, MaxLead: 900 * time.Second, Tick: 2 * time.Second, RunID: "test"}
	g.Now = func() time.Time { return now }
	tick := 0
	g.Sleep = func(d time.Duration) {
		now = now.Add(d)
		tick++
		if script != nil {
			script(g, tick)
		}
		if tick > 100000 {
			t.Fatal("the guard never ended")
		}
	}
	heartbeat(g, 400)
	return g
}

// heartbeat writes deadline.txt as the assistant does; its modification time follows the virtual clock.
func heartbeat(g *Guard, seconds int) {
	p := filepath.Join(g.Dir, "deadline.txt")
	os.WriteFile(p, []byte(fmt.Sprint(g.Now().Unix()+int64(seconds))), 0o644)
	os.Chtimes(p, g.Now(), g.Now())
}

func command(g *Guard, action string) {
	os.WriteFile(filepath.Join(g.Dir, "cmd.json"), []byte(`{"action":"`+action+`","id":"c1"}`), 0o644)
}

func lastStatus(t *testing.T, g *Guard) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(g.Dir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	json.Unmarshal(b, &s)
	return s
}

// engaged: what the helper does after the launch: a core, the adapter, the exit node off.
func engaged(sys *fakeSystem) func(g *Guard, tick int) {
	return func(g *Guard, tick int) {
		if tick == 1 {
			sys.mu.Lock()
			sys.cores, sys.adapter, sys.exitOn = 1, true, false
			sys.mu.Unlock()
		}
	}
}

func TestArmRefusals(t *testing.T) {
	cases := map[string]func(s *fakeSystem, g *Guard){
		"no heartbeat":   func(s *fakeSystem, g *Guard) { os.Remove(filepath.Join(g.Dir, "deadline.txt")) },
		"old heartbeat":  func(s *fakeSystem, g *Guard) { heartbeat(g, -1) },
		"core running":   func(s *fakeSystem, g *Guard) { s.cores = 1 },
		"helper running": func(s *fakeSystem, g *Guard) { s.helpers = 1 },
		"exit node off":  func(s *fakeSystem, g *Guard) { s.exitOn = false },
	}
	for name, change := range cases {
		sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
		g := newGuard(t, sys, nil)
		g.Launch = []string{"helper.exe"}
		change(sys, g)
		if got := g.Run(); got != "refused" {
			t.Errorf("%s: %s", name, got)
		}
		if strings.Contains(sys.list(), "kill") || strings.Contains(sys.list(), "set") || strings.Contains(sys.list(), "launch") {
			t.Errorf("%s: a refusal changes nothing: %s", name, sys.list())
		}
	}
}

// A stale heartbeat trips: helper first, then the exit node (confirmed), then the core.
func TestStaleHeartbeatTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, engaged(sys))
	g.Launch = []string{"helper.exe", "--dev"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe --dev|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	s := lastStatus(t, g)
	if s["state"] != "tripped" || s["reason"] != "heartbeat deadline expired" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// A `set --exit-node=` the helper started lands after the core was ended: the exit node goes off
// with no core. The guard sees it, sets the exit node again and only then reports the system ok.
func TestExitNodeLostAfterCoreKillIsSetAgain(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", loseOnCoreKill: true}
	g := newGuard(t, sys, engaged(sys))
	g.Launch = []string{"helper.exe"}
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe|set 100.64.0.1 true"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	if s := lastStatus(t, g); s["state"] != "tripped" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// A late `set --exit-node=` lands while the guard checks the exit node and the core still runs:
// the exit node is set again before the core is ended (blocked traffic, not a leak).
func TestExitNodeLostBeforeCoreKillIsSetFirst(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		engaged(sys)(g, tick)
		switch tick {
		case 1:
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		case 2: // the first read of the trip after the restore: the core still runs
			sys.mu.Lock()
			sys.exitOn = false
			sys.mu.Unlock()
		}
	})
	g.Launch = []string{"helper.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	if s := lastStatus(t, g); s["state"] != "tripped" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// No VPN path at all (the exit node off and no core) trips once it lasted 10 s, not before; the
// exit node off while the core runs (a killed helper, Task 23 step 7) does not.
func TestNoVPNPathTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	var goneAt time.Time
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick == 1 {
			sys.mu.Lock()
			sys.exitOn = false // the core is gone, the exit node stayed off
			sys.mu.Unlock()
			goneAt = g.Now()
		}
	})
	sys.clock = g.Now
	g.Launch = []string{"helper.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	s := lastStatus(t, g)
	if !strings.HasPrefix(fmt.Sprint(s["reason"]), "no VPN path") || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
	// the trip starts with the helper's end: not before the path was gone for 10 s (the literal,
	// so that a shorter noPathGrace fails too)
	if after := sys.helperKilledAt.Sub(goneAt); goneAt.IsZero() || after < 10*time.Second || after > time.Minute {
		t.Fatalf("tripped %s after the VPN path was gone", after)
	}
	if !strings.HasPrefix(sys.list(), "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true") {
		t.Fatalf("events %s", sys.list())
	}

	sys = &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g = newGuard(t, sys, func(g *Guard, tick int) {
		engaged(sys)(g, tick) // a core runs, the exit node is off
		if tick == 60 {
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		}
	})
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	if s := lastStatus(t, g); s["reason"] != "stop requested" {
		t.Fatalf("the exit node off with a core running is no trip: %v", s)
	}
}

// The process filters of the real system: by image name, and the children of ended helpers.
func TestProcessFilters(t *testing.T) {
	ps := []proc{
		{pid: 10, parent: 1, image: "splitlane-helper.exe"},
		{pid: 11, parent: 10, image: "tailscale.exe"},
		{pid: 12, parent: 77, image: "tailscale.exe"},
		{pid: 13, parent: 10, image: "sing-box.exe"},
		{pid: 14, parent: 1, image: "TAILSCALE.EXE"},
	}
	if got := fmt.Sprint(withImage(ps, "tailscale.exe")); got != "[11 12 14]" {
		t.Fatalf("by image: %s", got)
	}
	if got := fmt.Sprint(childrenOf(ps, []uint32{10}, "tailscale.exe")); got != "[11]" {
		t.Fatalf("children: %s", got)
	}
}

// %ProgramFiles% from the user's environment never decides which tailscale.exe runs elevated.
func TestTailscalePathIgnoresTheEnvironment(t *testing.T) {
	user := t.TempDir()
	t.Setenv("ProgramFiles", user)
	if exe := newSystem().tailscale; !filepath.IsAbs(exe) || strings.HasPrefix(strings.ToLower(exe), strings.ToLower(user)) {
		t.Fatalf("the path follows %%ProgramFiles%%: %s", exe)
	}
}

// The core is never ended before the exit node is confirmed; attempts go on every 5 s.
func TestRestoreRetriesBeforeTheCoreGoes(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", ignoreSet: 3}
	g := newGuard(t, sys, engaged(sys))
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "kill splitlane-helper.exe|set 100.64.0.1 true|set 100.64.0.1 true|set 100.64.0.1 true|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
}

// A heartbeat that is refreshed keeps the guard armed; finish ends it when the system is back.
func TestHeartbeatAndFinish(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick%100 == 0 {
			heartbeat(g, 400)
		}
		if tick == 1000 {
			command(g, "finish")
		}
	})
	if got := g.Run(); got != "finished" {
		t.Fatalf("%s, events %s", got, sys.list())
	}
	if strings.Contains(sys.list(), "kill") || strings.Contains(sys.list(), "set") {
		t.Fatalf("nothing to do: %s", sys.list())
	}
}

// finish while the core still runs is a trip, not a quiet end.
func TestFinishWithCoreRunningTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick == 1 {
			sys.mu.Lock()
			sys.cores, sys.adapter = 1, true
			sys.mu.Unlock()
			command(g, "finish")
		}
	})
	if got := g.Run(); got != "tripped" || !strings.HasSuffix(sys.list(), "kill sing-box.exe") {
		t.Fatalf("%s %s", got, sys.list())
	}
}

// kill-helper simulates a failure of the helper; launch-helper starts it again; the core stays.
func TestKillAndLaunchCommands(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		switch tick {
		case 1:
			sys.mu.Lock()
			sys.cores, sys.adapter, sys.exitOn = 1, true, false
			sys.mu.Unlock()
			command(g, "kill-helper")
		case 2:
			command(g, "launch-helper")
		case 3:
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		}
	})
	g.Launch = []string{"helper.exe"}
	g.Run()
	want := "launch helper.exe|kill splitlane-helper.exe|launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
}

// A deadline far in the future counts only up to 900 s after the file was written.
func TestDeadlineIsCapped(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, nil)
	heartbeat(g, 100000)
	start := g.Now()
	g.Run()
	if waited := g.Now().Sub(start); waited < 890*time.Second || waited > 930*time.Second { // the trip reads for 12 s
		t.Fatalf("tripped after %s", waited)
	}
}

func TestHardCap(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick%100 == 0 {
			heartbeat(g, 400)
		}
	})
	g.HardCap = 10 * time.Minute
	g.Run()
	if s := lastStatus(t, g); s["reason"] != "hard time cap reached" {
		t.Fatalf("status %v", s)
	}
}

func TestLaunchFailureTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", launchErr: errors.New("no file")}
	g := newGuard(t, sys, nil)
	g.Launch = []string{"missing.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
}

func TestLogMasksAddresses(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", ignoreSet: 1}
	g := newGuard(t, sys, engaged(sys))
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	g.Run()
	b, _ := os.ReadFile(filepath.Join(g.Dir, "guard.log"))
	if strings.Contains(string(b), "100.64.0.1") || !strings.Contains(string(b), "<ip>") {
		t.Fatalf("log:\n%s", b)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться в провале**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go test ./cmd/slguard/
```

Ожидается: `FAIL splitlane/helper/cmd/slguard [build failed]` и `undefined: HelperImage`.

- [ ] **Step 3: Реализовать сторож**

Файл `helper/cmd/slguard/guard.go`, целиком:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Image names the guard ends. TailscaleCLI is only the command line program: the service is
// tailscaled.exe and the tray app tailscale-ipn.exe, the guard never touches them.
const (
	HelperImage  = "splitlane-helper.exe"
	CoreImage    = "sing-box.exe"
	TailscaleCLI = "tailscale.exe"
)

// Times of the restore: the exit node must read on settleReads times in a row, settleEvery
// apart, within settleFor; before the core is ended and again after.
const (
	settleFor   = 60 * time.Second
	settleEvery = 2 * time.Second
	settleReads = 3
)

// No VPN path at all: the system exit node off and no sing-box.exe. No step of a live run
// passes through this state (5.3 starts the core before the exit node goes off, 5.4 restores
// before it stops the core, a killed helper leaves its core running), so it trips the guard
// once it lasts noPathGrace; it is read every pathEvery ticks.
const (
	noPathGrace = 10 * time.Second
	pathEvery   = 2
)

// System is everything the guard does to Windows; the tests replace it.
type System interface {
	ExitNode() (on bool, ip string, err error) // tailscale status --json
	AllowLAN() (bool, error)                   // tailscale debug prefs
	SetExitNode(ip string, allowLAN bool) error
	Kill(image string) []uint32                      // ends every process with this image name, returns their ids
	KillChildren(parents []uint32, image string) int // ends the processes of image that one of parents started
	Count(image string) int                          // running processes with this image name
	AdapterPresent() bool                            // the SplitLane adapter
	Launch(command []string) error
}

// Guard is the dead-man switch. Files in Dir: deadline.txt (unix seconds, written by the
// developer's assistant), stop.flag, cmd.json, status.json, guard-state.json, guard.log.
type Guard struct {
	Dir     string
	Sys     System
	Now     func() time.Time
	Sleep   func(time.Duration)
	Launch  []string      // the helper's command line; empty: the guard starts nothing
	HardCap time.Duration // the guard trips after this long whatever happens
	MaxLead time.Duration // a deadline never reaches further than this past its own write
	Tick    time.Duration
	RunID   string

	exitIP   string
	allowLAN bool
	started  time.Time
	deadline time.Time
	seq      int
	ticks    int
	noPathAt time.Time // first read of the current run of reads without a VPN path
}

func (g *Guard) path(name string) string { return filepath.Join(g.Dir, name) }

func (g *Guard) logf(format string, args ...any) {
	f, err := os.OpenFile(g.path("guard.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", g.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// writeAtomic: temp file in the same folder, then rename (the reader never sees half a file).
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (g *Guard) status(state string, fields map[string]any) {
	g.seq++
	s := map[string]any{"run": g.RunID, "seq": g.seq, "state": state, "time": g.Now().Unix()}
	for k, v := range fields {
		s[k] = v
	}
	b, _ := json.Marshal(s)
	if err := writeAtomic(g.path("status.json"), b); err != nil {
		g.logf("status not written: %v", err)
	}
	g.logf("status %s", maskIP(string(b)))
}

// readDeadline: the last good value stays when the file is half written; never later than mtime+MaxLead.
func (g *Guard) readDeadline() (time.Time, bool) {
	p := g.path("deadline.txt")
	b, err := os.ReadFile(p)
	fi, statErr := os.Stat(p)
	if err == nil && statErr == nil {
		if n, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); perr == nil {
			d := time.Unix(n, 0)
			if limit := fi.ModTime().Add(g.MaxLead); d.After(limit) {
				d = limit
			}
			g.deadline = d
		}
	}
	return g.deadline, !g.deadline.IsZero()
}

// arm checks the starting point: a fresh heartbeat, no core and no helper, the system exit node on.
// It returns the reason of a refusal, or "".
func (g *Guard) arm() string {
	if d, ok := g.readDeadline(); !ok || !d.After(g.Now()) {
		return "heartbeat missing or stale: refresh deadline.txt first"
	}
	if g.Sys.Count(CoreImage) > 0 {
		return "a sing-box.exe is running"
	}
	if g.Sys.Count(HelperImage) > 0 {
		return "a splitlane-helper.exe is running"
	}
	on, ip, err := g.Sys.ExitNode()
	if err != nil || !on || ip == "" {
		return fmt.Sprintf("the system exit node is not on (%v)", err)
	}
	lan, err := g.Sys.AllowLAN()
	if err != nil {
		return fmt.Sprintf("the LAN access flag of the system client is unknown (%v)", err)
	}
	g.exitIP, g.allowLAN = ip, lan
	b, _ := json.Marshal(map[string]any{"exitNode": ip, "allowLan": lan})
	if err := writeAtomic(g.path("guard-state.json"), b); err != nil {
		return fmt.Sprintf("guard-state.json not written: %v", err)
	}
	return ""
}

// Run arms the guard and watches until it trips or finishes; it returns the final state.
func (g *Guard) Run() string {
	g.started = g.Now()
	os.Remove(g.path("stop.flag"))
	os.Remove(g.path("cmd.json"))
	g.status("starting", nil)
	if why := g.arm(); why != "" {
		g.status("refused", map[string]any{"reason": why})
		return "refused"
	}
	g.status("armed", nil)
	if len(g.Launch) > 0 {
		if err := g.Sys.Launch(g.Launch); err != nil {
			return g.trip(fmt.Sprintf("helper launch failed: %v", err))
		}
		g.logf("helper launched")
	}
	for {
		g.Sleep(g.Tick)
		now := g.Now()
		if _, err := os.Stat(g.path("stop.flag")); err == nil {
			return g.trip("stop requested")
		}
		if d, _ := g.readDeadline(); !now.Before(d) {
			return g.trip("heartbeat deadline expired")
		}
		if now.Sub(g.started) > g.HardCap {
			return g.trip("hard time cap reached")
		}
		if final := g.command(); final != "" {
			return final
		}
		if g.ticks++; g.ticks%pathEvery == 0 && g.noVPNPath(now) {
			return g.trip("no VPN path: the system exit node is off and no sing-box.exe runs")
		}
	}
}

// noVPNPath reports a lasting state without any VPN path: the exit node off and no core for at
// least noPathGrace. A read that fails changes nothing.
func (g *Guard) noVPNPath(now time.Time) bool {
	on, _, err := g.Sys.ExitNode()
	if err != nil {
		return false
	}
	if on || g.Sys.Count(CoreImage) > 0 {
		g.noPathAt = time.Time{}
		return false
	}
	if g.noPathAt.IsZero() {
		g.noPathAt = now
		g.logf("no VPN path: the system exit node is off and no sing-box.exe runs")
	}
	return now.Sub(g.noPathAt) >= noPathGrace
}

// command runs one command from cmd.json: kill-helper, launch-helper, finish.
func (g *Guard) command() string {
	b, err := os.ReadFile(g.path("cmd.json"))
	if err != nil {
		return ""
	}
	var cmd struct {
		Action string `json:"action"`
		ID     string `json:"id"`
	}
	if json.Unmarshal(b, &cmd) != nil {
		return "" // half written: read it again on the next tick
	}
	if os.Remove(g.path("cmd.json")) != nil {
		return "" // never run a command twice
	}
	g.logf("command %s id=%s", cmd.Action, cmd.ID)
	switch cmd.Action {
	case "kill-helper":
		n := len(g.Sys.Kill(HelperImage))
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("helper processes ended: %d", n)})
	case "launch-helper":
		result := "helper launched"
		if len(g.Launch) == 0 {
			result = "no helper command line"
		} else if err := g.Sys.Launch(g.Launch); err != nil {
			result = "launch failed: " + err.Error()
		}
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": result})
	case "finish":
		if g.Sys.Count(HelperImage) == 0 && g.systemOK() {
			g.status("finished", map[string]any{"cmd_id": cmd.ID, "system_ok": true})
			return "finished"
		}
		return g.trip("finish found the system not restored or the helper still running")
	default:
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "cmd_error": "unknown action"})
	}
	return ""
}

func (g *Guard) systemOK() bool {
	on, _, err := g.Sys.ExitNode()
	return err == nil && on && g.Sys.Count(CoreImage) == 0 && !g.Sys.AdapterPresent()
}

// trip puts the system back. The helper goes first: it is the one that switches the exit node off.
// A `tailscale.exe set --exit-node=` it had just started is not in a job and outlives it: it goes
// too. Then the exit node, confirmed, tried every 5 s without end. A command that landed late is
// caught while the core still runs (the exit node on with the adapter up blocks traffic, it does
// not leak): the exit node must read on three times in a row before the core is ended, and again
// after; system_ok needs those reads, no core and no adapter.
func (g *Guard) trip(reason string) string {
	g.status("tripping", map[string]any{"reason": reason})
	helpers := g.Sys.Kill(HelperImage)
	g.logf("helper processes ended: %d", len(helpers))
	if len(helpers) > 0 {
		g.logf("tailscale.exe started by the helper ended: %d", g.Sys.KillChildren(helpers, TailscaleCLI))
	}
	g.restoreExitNode(reason)
	g.settle(reason, "before the core was ended")
	g.logf("core processes ended: %d", len(g.Sys.Kill(CoreImage)))
	for i := 0; i < 30 && g.Sys.AdapterPresent(); i++ {
		g.Sleep(500 * time.Millisecond)
	}
	ok := g.settle(reason, "after the core was ended") && g.systemOK()
	g.status("tripped", map[string]any{"reason": reason, "system_ok": ok})
	return "tripped"
}

// settle reads the exit node every 2 s until it reads on three times in a row (at most 60 s);
// an off reading restores it again. False when the reads did not settle.
func (g *Guard) settle(reason, when string) bool {
	good := 0
	for end := g.Now().Add(settleFor); good < settleReads && g.Now().Before(end); {
		g.Sleep(settleEvery)
		if on, _, err := g.Sys.ExitNode(); err == nil && on {
			good++
			continue
		}
		good = 0
		g.logf("the exit node went off %s: restoring it again", when)
		g.restoreExitNode(reason)
	}
	return good >= settleReads
}

// restoreExitNode sets the exit node back and returns only after `tailscale status` confirms it;
// attempts every 5 s without end (status restore_blocked with the manual command meanwhile).
func (g *Guard) restoreExitNode(reason string) {
	manual := fmt.Sprintf("tailscale set --exit-node=%s --exit-node-allow-lan-access=%v", g.exitIP, g.allowLAN)
	for attempt := 1; ; attempt++ {
		if on, _, err := g.Sys.ExitNode(); err == nil && on {
			return
		}
		g.logf("set the exit node back (attempt %d)", attempt)
		err := g.Sys.SetExitNode(g.exitIP, g.allowLAN)
		if on, _, serr := g.Sys.ExitNode(); serr == nil && on {
			return
		}
		g.status("restore_blocked", map[string]any{"reason": reason, "attempt": attempt, "error": fmt.Sprint(err), "manual": manual})
		g.Sleep(5 * time.Second)
	}
}

var ipv4 = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// maskIP keeps addresses out of the log.
func maskIP(s string) string { return ipv4.ReplaceAllString(s, "<ip>") }
```

Файл `helper/cmd/slguard/system.go`, целиком:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// tsTimeout bounds one call of tailscale.exe: a hung CLI must not stall the guard in tripping.
const tsTimeout = 30 * time.Second

// winSystem is the real System. It shares no code with the helper on purpose.
type winSystem struct {
	tailscale string
}

func newSystem() *winSystem {
	return &winSystem{tailscale: filepath.Join(programFiles(), "Tailscale", "tailscale.exe")}
}

// programFiles is the 64-bit Program Files folder named by HKLM. The guard runs elevated with the
// user's environment, where %ProgramFiles% is the user's to change: it is never read.
func programFiles() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("ProgramFilesDir"); err == nil && filepath.IsAbs(v) && !strings.Contains(v, "%") {
			return v
		}
	}
	return `C:\Program Files`
}

func (s *winSystem) ts(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tsTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.tailscale, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.Output()
}

func (s *winSystem) ExitNode() (bool, string, error) {
	out, err := s.ts("status", "--json")
	if err != nil {
		return false, "", err
	}
	var st struct {
		BackendState   string
		ExitNodeStatus *struct{ TailscaleIPs []string }
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return false, "", err
	}
	if st.BackendState != "Running" || st.ExitNodeStatus == nil {
		return false, "", nil
	}
	for _, p := range st.ExitNodeStatus.TailscaleIPs {
		if ip := strings.SplitN(p, "/", 2)[0]; !strings.Contains(ip, ":") {
			return true, ip, nil
		}
	}
	return true, "", nil
}

func (s *winSystem) AllowLAN() (bool, error) {
	out, err := s.ts("debug", "prefs")
	if err != nil {
		return false, err
	}
	var p struct{ ExitNodeAllowLANAccess *bool }
	if err := json.Unmarshal(out, &p); err != nil || p.ExitNodeAllowLANAccess == nil {
		return false, fmt.Errorf("no ExitNodeAllowLANAccess in debug prefs (%v)", err)
	}
	return *p.ExitNodeAllowLANAccess, nil
}

func (s *winSystem) SetExitNode(ip string, allowLAN bool) error {
	_, err := s.ts("set", "--exit-node="+ip, "--exit-node-allow-lan-access="+strconv.FormatBool(allowLAN))
	return err
}

// proc is one entry of the process snapshot.
type proc struct {
	pid, parent uint32
	image       string
}

func snapshot() []proc {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, proc{pid: e.ProcessID, parent: e.ParentProcessID, image: windows.UTF16ToString(e.ExeFile[:])})
	}
	return out
}

// withImage: the ids of the processes with this image name.
func withImage(ps []proc, image string) []uint32 {
	var out []uint32
	for _, p := range ps {
		if strings.EqualFold(p.image, image) {
			out = append(out, p.pid)
		}
	}
	return out
}

// childrenOf: the ids of the processes with this image name that one of parents started. A child
// keeps its parent's id after the parent has ended.
func childrenOf(ps []proc, parents []uint32, image string) []uint32 {
	var out []uint32
	for _, p := range ps {
		if strings.EqualFold(p.image, image) && slices.Contains(parents, p.parent) {
			out = append(out, p.pid)
		}
	}
	return out
}

// terminate ends the processes and returns the ids of those that ended.
func terminate(ids []uint32) []uint32 {
	var ended []uint32
	for _, pid := range ids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
		if err != nil {
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			windows.WaitForSingleObject(h, 5000)
			ended = append(ended, pid)
		}
		windows.CloseHandle(h)
	}
	return ended
}

func (s *winSystem) Count(image string) int { return len(withImage(snapshot(), image)) }

func (s *winSystem) Kill(image string) []uint32 { return terminate(withImage(snapshot(), image)) }

func (s *winSystem) KillChildren(parents []uint32, image string) int {
	return len(terminate(childrenOf(snapshot(), parents, image)))
}

func (s *winSystem) AdapterPresent() bool {
	_, err := net.InterfaceByName("SplitLane")
	return err == nil
}

func (s *winSystem) Launch(command []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
```

Файл `helper/cmd/slguard/main.go`, целиком:

```go
// Command slguard is the development dead-man switch of Plan 1 (safety rule 2). It runs elevated,
// is started before the helper and does not depend on the helper being right: when deadline.txt
// is not refreshed in time, when stop.flag appears or after the time cap, it ends the helper, gives
// the system Tailscale its exit node back and waits for the confirmation, ends sing-box.exe and
// checks that the adapter is gone. It is a development tool and never ships.
//
//	slguard run -dir DIR [-cap 60m] [-- HELPER COMMAND LINE]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: slguard run -dir DIR [-cap 60m] [-- HELPER COMMAND LINE]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := fs.String("dir", "", "folder of deadline.txt, stop.flag, cmd.json, status.json")
	hardCap := fs.Duration("cap", 60*time.Minute, "trip after this long whatever happens")
	fs.Parse(os.Args[2:])
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	id := make([]byte, 4)
	rand.Read(id)
	g := &Guard{
		Dir: *dir, Sys: newSystem(), Now: time.Now, Sleep: time.Sleep, Launch: fs.Args(),
		HardCap: *hardCap, MaxLead: 900 * time.Second, Tick: 2 * time.Second, RunID: hex.EncodeToString(id),
	}
	switch g.Run() {
	case "finished":
		os.Exit(0)
	case "refused":
		os.Exit(3)
	default:
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Запустить тест, собрать**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; go vet ./...; go test -count=3 ./cmd/slguard/; go build -o bin\slguard.exe .\cmd\slguard; "built: $(Test-Path bin\slguard.exe)"
```

Ожидается `ok  splitlane/helper/cmd/slguard` и `built: True`. Тесты закрепляют: отказ без свежего heartbeat, при работающем ядре или служебном процессе, при выключенном exit node — без каких-либо действий (`TestArmRefusals`); порядок срабатывания «служебный процесс → его `tailscale.exe` → exit node с подтверждением → ядро» (`TestStaleHeartbeatTrips`); ядро не завершается, пока exit node не подтверждён (`TestRestoreRetriesBeforeTheCoreGoes`); exit node, выключенный запоздавшей командой при работающем ядре, включается снова до завершения ядра (`TestExitNodeLostBeforeCoreKillIsSetFirst`), а выключенный уже после ядра — тоже, и только потом `system_ok=true` (`TestExitNodeLostAfterCoreKillIsSetAgain`); нет пути VPN (exit node выключен, ядра нет) — срабатывание не раньше чем через 10 секунд (тест сравнивает время первого завершения служебного процесса по часам сторожа со временем пропажи пути), а exit node, выключенный при работающем ядре, — не срабатывание (`TestNoVPNPathTrips`); отбор процессов по имени и по родителю (`TestProcessFilters`); путь `tailscale.exe` не зависит от `%ProgramFiles%` (`TestTailscalePathIgnoresTheEnvironment`, читает одно значение реестра); `finish` (`TestHeartbeatAndFinish`, `TestFinishWithCoreRunningTrips`); команды `kill-helper` и `launch-helper` (`TestKillAndLaunchCommands`); дедлайн не дальше 900 секунд (`TestDeadlineIsCapped`, вместе с чтениями срабатывания); предел времени (`TestHardCap`); сбой запуска служебного процесса (`TestLaunchFailureTrips`); адреса в журнале закрыты (`TestLogMasksAddresses`).

- [ ] **Step 5: Записать `p1lib.ps1`**

Файл `C:\code\router\spike\plan1\p1lib.ps1`, целиком:

```powershell
# SplitLane Plan 1: heartbeat and the dead-man guard (slguard.exe) for the live steps.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: . C:\code\router\spike\plan1\p1lib.ps1

$P1Guard    = 'C:\code\router\spike\plan1\guard'
$P1Bin      = 'C:\code\router\helper\bin'
$P1Data     = 'C:\code\router\.dev\data'
$P1Core     = 'C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'
$P1Ts       = 'C:\Program Files\Tailscale\tailscale.exe'
$P1Terminal = @('refused', 'finished', 'tripped')

function Get-P1Now {
    # Never Get-Date -UFormat %s: on this machine it gives local time with a decimal comma.
    [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
}

function Write-P1Atomic {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Text)
    $tmp = "$Path.pstmp"
    [IO.File]::WriteAllText($tmp, $Text, (New-Object Text.UTF8Encoding $false))
    foreach ($attempt in 1..20) {
        try { Move-Item -LiteralPath $tmp -Destination $Path -Force -ErrorAction Stop; return }
        catch { Start-Sleep -Milliseconds 100 }
    }
    throw "Write-P1Atomic: could not replace $Path"
}

function Set-P1Heartbeat {
    # Every live command starts with this: the guard trips when now passes this deadline (now + 400 s).
    New-Item -ItemType Directory -Force -Path $P1Guard | Out-Null
    Write-P1Atomic -Path (Join-Path $P1Guard 'deadline.txt') -Text ([string]((Get-P1Now) + 400))
}

function Read-P1Status {
    try { Get-Content -LiteralPath (Join-Path $P1Guard 'status.json') -Raw -ErrorAction Stop | ConvertFrom-Json } catch { $null }
}

function Format-P1Status {
    param($Status)
    if (-not $Status) { return 'no status' }
    $text = "state=$($Status.state) run=$($Status.run) seq=$($Status.seq)"
    foreach ($name in 'cmd_id', 'reason', 'result', 'cmd_error', 'system_ok', 'attempt', 'error') {
        if ($null -ne $Status.$name) { $text += " $name=$($Status.$name)" }
    }
    $text -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>'
}

function Get-P1SystemExitNode {
    # Read-only: on/off, address and LAN flag of the system Tailscale client. Never printed as is.
    try {
        $s = & $P1Ts status --json | Out-String | ConvertFrom-Json
        $p = & $P1Ts debug prefs | Out-String | ConvertFrom-Json
    } catch { return $null }
    $ip = $null
    if ($s.BackendState -eq 'Running' -and $s.ExitNodeStatus) {
        $ip = @($s.ExitNodeStatus.TailscaleIPs | ForEach-Object { ($_ -split '/')[0] } | Where-Object { $_ -notmatch ':' })[0]
    }
    [pscustomobject]@{ On = [bool]$ip; IP = $ip; AllowLAN = [bool]$p.ExitNodeAllowLANAccess }
}

function Test-P1DevDataLeft {
    # A run the guard ended while routing was on leaves "was on" in state.json and restore.json:
    # the next helper would restore and enable again on its own at start (5.7), in a step that
    # does not poll it. A core identity alone is harmless: the helper keeps it after a normal
    # disable, and a core is only stopped when pid, creation time and path all match.
    $st = $null
    try { $st = Get-Content -LiteralPath (Join-Path $P1Data 'state.json') -Raw -ErrorAction Stop | ConvertFrom-Json } catch { $st = $null }
    [bool](($st -and $st.wasOn) -or (Test-Path -LiteralPath (Join-Path $P1Data 'restore.json')))
}

function Test-P1Ready {
    # Read-only starting point of a live step: no core, no helper, no guard, system exit node on,
    # no development data left by a tripped run.
    $problems = @()
    if (Get-Process -Name 'sing-box' -ErrorAction SilentlyContinue) { $problems += 'a sing-box.exe is running' }
    if (Get-Process -Name 'splitlane-helper' -ErrorAction SilentlyContinue) { $problems += 'a splitlane-helper.exe is running' }
    if (Get-Process -Name 'slguard' -ErrorAction SilentlyContinue) { $problems += 'a slguard.exe is running' }
    $sys = Get-P1SystemExitNode
    if (-not $sys -or -not $sys.On) { $problems += 'the system exit node is not on' }
    if (Get-NetAdapter -Name 'SplitLane' -ErrorAction SilentlyContinue) { $problems += 'the SplitLane adapter exists' }
    if (Test-P1DevDataLeft) { $problems += 'development data left by a tripped run' }
    foreach ($f in "$P1Bin\slguard.exe", "$P1Bin\slctl.exe", "$P1Bin\splitlane-helper.exe", $P1Core) {
        if (-not (Test-Path -LiteralPath $f)) { $problems += "missing $f" }
    }
    if ($problems.Count -eq 0) { Write-Host 'P1-READY'; return $true }
    Write-Host "P1-NOT-READY: $($problems -join '; ')"
    $false
}

function Wait-P1Status {
    # Polls status.json of the guard for one of the states (or a terminal one); $After drops older runs.
    param([string[]]$Target, [string]$CmdId = '', [int64]$After = 0, [int]$TimeoutSeconds = 150)
    $limit = (Get-Date).AddSeconds($TimeoutSeconds)
    $last = ''
    while ((Get-Date) -lt $limit) {
        $s = Read-P1Status
        if ($s -and $s.time -ge $After) {
            $line = Format-P1Status $s
            if ($line -ne $last) { Write-Host "P1-STATUS: $line"; $last = $line }
            $mine = (-not $CmdId) -or ($s.cmd_id -eq $CmdId)
            if ($mine -and ($Target -contains $s.state)) { return $s }
            if ($P1Terminal -contains $s.state) { return $s }
        }
        Start-Sleep -Milliseconds 500
    }
    $null
}

function Wait-P1Pipe {
    # True when the helper answers on its pipe. A helper that was just launched opens the pipe
    # after its log, the SHA-256 of the core and the engine; slctl itself waits up to 10 s here.
    param([int]$Seconds = 60)
    $limit = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $limit) {
        & "$P1Bin\slctl.exe" -timeout 10s state | Out-Null
        if ($LASTEXITCODE -eq 0) { return $true }
        Start-Sleep -Seconds 1
    }
    $false
}

function Assert-P1Armed {
    # Every live step starts with this: the guard of THIS run (guard\run.txt) is running and armed.
    $run = ''
    try { $run = (Get-Content -LiteralPath (Join-Path $P1Guard 'run.txt') -Raw -ErrorAction Stop).Trim() } catch { $run = '' }
    $s = Read-P1Status
    $alive = [bool](Get-Process -Name 'slguard' -ErrorAction SilentlyContinue)
    if ($alive -and $run -and $s -and ($s.run -eq $run) -and ($s.state -eq 'armed')) { return $true }
    Write-Host "P1-NO-GUARD: process_alive=$alive expected_run=$run status=$(Format-P1Status $s) - switch nothing; run the recovery block"
    $false
}

function Start-P1Guard {
    # One UAC prompt: slguard.exe starts elevated, checks the starting point, launches the helper.
    # -DryRun: unelevated, without a helper (self-test of this file; changes nothing).
    param([switch]$DryRun)
    if (-not (Test-P1Ready)) { Write-Host 'P1-RESULT: REFUSED - starting point not ready'; return }
    New-Item -ItemType Directory -Force -Path $P1Guard | Out-Null
    foreach ($f in 'stop.flag', 'cmd.json', 'run.txt') { Remove-Item -LiteralPath (Join-Path $P1Guard $f) -ErrorAction SilentlyContinue }
    Set-P1Heartbeat
    $launched = Get-P1Now
    $argList = @('run', '-dir', $P1Guard)
    if (-not $DryRun) {
        $argList += @('--', "$P1Bin\splitlane-helper.exe", '--dev', '--dev-pin', '--data-dir', $P1Data, '--core', $P1Core)
    }
    try {
        if ($DryRun) {
            Start-Process -FilePath "$P1Bin\slguard.exe" -ArgumentList $argList -WindowStyle Hidden -ErrorAction Stop | Out-Null
        } else {
            Write-Host 'P1-LAUNCH: the UAC prompt of slguard.exe must be accepted'
            Start-Process -FilePath "$P1Bin\slguard.exe" -ArgumentList $argList -Verb RunAs -WindowStyle Hidden -ErrorAction Stop | Out-Null
        }
    } catch {
        Write-Host 'P1-RESULT: UAC-CANCELLED - nothing was started, the system is unchanged. Do not retry without the user.'
        return
    }
    $s = Wait-P1Status -Target @('armed') -After $launched -TimeoutSeconds 150
    if (-not $s -or $s.state -ne 'armed') { Write-Host "P1-RESULT: $(Format-P1Status $s)"; return }
    [IO.File]::WriteAllText((Join-Path $P1Guard 'run.txt'), [string]$s.run, (New-Object Text.ASCIIEncoding))
    Set-P1Heartbeat
    if (-not $DryRun) {
        if (-not (Wait-P1Pipe)) { Write-Host 'P1-RESULT: HELPER-PIPE-MISSING - the helper did not open its pipe; run Stop-P1Guard'; return }
        & "$P1Bin\slctl.exe" state | Select-Object -Last 1
    }
    Write-Host "P1-RESULT: $(Format-P1Status $s)"
}

function Send-P1Guard {
    # kill-helper (a failure of the helper), launch-helper (the same command line again), finish (the end).
    param([Parameter(Mandatory)][ValidateSet('kill-helper', 'launch-helper', 'finish')][string]$Action)
    $s = Read-P1Status
    if (-not $s -or ($P1Terminal -contains $s.state)) { Write-Host "P1-RESULT: NO-LIVE-GUARD - $(Format-P1Status $s)"; return }
    Set-P1Heartbeat
    $id = 'c' + [guid]::NewGuid().ToString('N').Substring(0, 7)
    Write-P1Atomic -Path (Join-Path $P1Guard 'cmd.json') -Text ("{`"action`":`"$Action`",`"id`":`"$id`"}")
    $target = if ($Action -eq 'finish') { @('finished') } else { @('armed') }
    $r = Wait-P1Status -Target $target -CmdId $id -TimeoutSeconds 120
    Write-Host "P1-RESULT: $(Format-P1Status $r)"
}

function Stop-P1Guard {
    # Trips the guard now: helper ended, exit node back and confirmed, core ended.
    # No slguard process: nothing would read stop.flag; say so at once instead of waiting 300 s.
    if (-not (Get-Process -Name 'slguard' -ErrorAction SilentlyContinue)) {
        Write-Host "P1-RESULT: NO-LIVE-GUARD - no slguard.exe runs; $(Format-P1Status (Read-P1Status))"
        return
    }
    [IO.File]::WriteAllText((Join-Path $P1Guard 'stop.flag'), 'stop', (New-Object Text.ASCIIEncoding))
    $r = Wait-P1Status -Target @('tripped') -TimeoutSeconds 300
    if (-not $r -or $P1Terminal -notcontains $r.state) {
        Write-Host 'P1-RESULT: NO-TERMINAL-STATE - show the user the manual recovery text'
        return
    }
    Write-Host "P1-RESULT: $(Format-P1Status $r)"
}

function Get-P1Guard {
    $alive = [bool](Get-Process -Name 'slguard' -ErrorAction SilentlyContinue)
    Write-Host "P1-GUARD: $(Format-P1Status (Read-P1Status)) process_alive=$alive"
}

function Test-P1SystemRestored {
    # The system exit node and its LAN flag equal the values the guard captured before the run.
    try { $before = Get-Content -LiteralPath (Join-Path $P1Guard 'guard-state.json') -Raw | ConvertFrom-Json } catch { $before = $null }
    $now = Get-P1SystemExitNode
    $problems = @()
    if (-not $before) { $problems += 'guard-state.json missing' }
    elseif (-not $now -or -not $now.On) { $problems += 'system exit node off' }
    else {
        if ($now.IP -ne $before.exitNode) { $problems += 'another exit node' }
        if ($now.AllowLAN -ne [bool]$before.allowLan) { $problems += 'another LAN access flag' }
    }
    if (Get-Process -Name 'sing-box' -ErrorAction SilentlyContinue) { $problems += 'sing-box.exe still running' }
    if (Get-NetAdapter -Name 'SplitLane' -ErrorAction SilentlyContinue) { $problems += 'SplitLane adapter still there' }
    if ($problems.Count -eq 0) { Write-Host 'SYSTEM-RESTORED' } else { Write-Host "SYSTEM-WRONG: $($problems -join '; ')" }
}
```

- [ ] **Step 6: Закоммитить**

```powershell
git -C C:\code\router add helper/cmd/slguard
git -C C:\code\router commit -m "feat(helper): slguard dead-man switch for live development runs" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

`p1lib.ps1` лежит в игнорируемой папке и в коммит не входит.

---

### Task 22: Сборка и проверки на ПК без переключений, данные режима разработки

Всё, что можно проверить на настоящей системе, не выключая системный exit node: полный прогон тестов, проверки конфигов настоящим ядром, настоящий консольный сигнал на подменном ядре, чтение маршрутов, адаптеров и NRPT, запуск служебного процесса без прав администратора в фазе `off`, пробные запуски сторожа без служебного процесса (срабатывание и `finish`). Затем — папка данных режима разработки: копия состояния встроенного узла и `state.json` для узла, который уже вошёл в сеть (вход появится в плане 1B), копии `curl.exe` и раскладки для задачи 23. UAC не нужен; системный VPN не меняется.

**Files:**
- Create (игнорируются git): `C:\code\router\.dev\data\` (копия `spike\run\ts-state` и `state.json`), `C:\code\router\spike\plan1\bin\{vpn,direct,free}\curl-*.exe`, `C:\code\router\spike\plan1\lanes-1.json`, `C:\code\router\spike\plan1\lanes-2.json`

**Interfaces:**
- Consumes: программы задач 19–21; `scripts\fetch-core.ps1` (задача 4); `p1lib.ps1`.
- Produces: `helper\bin\splitlane-helper.exe`, `helper\bin\slctl.exe`, `helper\bin\slguard.exe`; папка данных разработки с `state.json` (`loggedIn: true`, выбранный exit node, имя устройства); раскладки `lanes-1.json` (копия `curl-vpn.exe` — через VPN, `curl-direct.exe` — напрямую) и `lanes-2.json` (то же плюс `curl-free.exe` — напрямую).

- [ ] **Step 1: Полный прогон тестов и сборка**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $global:LASTEXITCODE = -1; $g = & "$env:LOCALAPPDATA\Programs\go\bin\gofmt.exe" -l .; "gofmt exit=$LASTEXITCODE files=[$g]"; go vet ./...; $env:SPLITLANE_CORE='C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe'; go test -count=1 ./...; go build -ldflags "-H=windowsgui" -o bin\splitlane-helper.exe .\cmd\splitlane-helper; go build -o bin\slctl.exe .\cmd\slctl; go build -o bin\slguard.exe .\cmd\slguard; Get-ChildItem bin | Select-Object Name
```

Ожидается `gofmt exit=0 files=[]`, `ok` у всех пакетов (тесты есть у каждого) и три программы в `bin`. Строка `gofmt exit=-1` или ошибка `CommandNotFoundException` — `gofmt.exe` не запустился, проверка формата не выполнена: шаг не пройден.

- [ ] **Step 2: Настоящий консольный сигнал и чтение системы**

```powershell
cd C:\code\router\helper; $env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"; $env:CGO_ENABLED='0'; $env:SPLITLANE_CONSOLE_TEST='1'; go test -count=1 -v -run TestInterruptStopsTheCore ./internal/singbox/ | Select-String -Pattern '^(--- |ok|FAIL)'; $env:SPLITLANE_CONSOLE_TEST=''; $env:SPLITLANE_LIVE_READONLY='1'; go test -count=1 -v -run TestLiveReadOnly ./internal/winhost/ | Select-String -CaseSensitive -Pattern 'LIVE |^(--- |ok|FAIL)'
```

Ожидается:

```text
--- PASS: TestInterruptStopsTheCore (0.06s)
ok      splitlane/helper/internal/singbox
    host_test.go:96: LIVE default route true, physical forwarding false, NRPT catch-all false, sing-box running false, adapter false
--- PASS: TestLiveReadOnly (0.02s)
ok      splitlane/helper/internal/winhost
```

`physical forwarding true` или `NRPT catch-all true` — включение в задаче 23 откажет с `ip_forwarding` или `system_dns_override`: сообщите пользователю (общий доступ к интернету или мобильная точка доступа; `accept-dns` в системном Tailscale) и не начинайте задачу 23, пока строка не станет как выше.

- [ ] **Step 3: Служебный процесс без прав администратора в фазе `off`**

Без `routing.enable` служебный процесс ничего не меняет в системе; с подключённым клиентом он раз в 3 секунды читает `tailscale status --json`. Паузы после запуска не нужно: `slctl` сам ждёт, пока появится канал (задача 20).

```powershell
$B = 'C:\code\router\helper\bin'; $D = 'C:\code\router\spike\plan1\smoke'; Remove-Item -Recurse -Force $D -ErrorAction SilentlyContinue; Start-Process -FilePath "$B\splitlane-helper.exe" -ArgumentList '--dev', '--data-dir', $D, '--core', 'C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe', '--pipe', '\\.\pipe\SplitLane.Smoke'; & "$B\slctl.exe" -pipe '\\.\pipe\SplitLane.Smoke' -timeout 30s state | Select-Object -Last 1; & "$B\slctl.exe" -pipe '\\.\pipe\SplitLane.Smoke' -timeout 20s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 1; "helper running: $([bool](Get-Process splitlane-helper -ErrorAction SilentlyContinue))"; Get-Content "$D\logs\helper.log"
```

Ожидается:

```text
SLCTL: phase=off reason=- allDirect=false defaultLane=vpn apps=0 vpn=null direct=null systemExitNode=false restoreOnDisable=false
SLCTL: the helper closed the pipe
helper running: False
<время> splitlane-helper 0.1.0-dev starting, dev=true dev-pin=false
<время> state: phase=off reason=- allDirect=false
<время> helper.shutdown done; exiting
```

(Между строками журнала может быть ещё одна `state: phase=off`, когда опрос 5.6 узнал о системном клиенте.)

- [ ] **Step 4: Пробные запуски сторожа без служебного процесса**

Два запуска подряд: первый срабатывает по `Stop-P1Guard`, второй заканчивается командой `finish`.

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; $log = Join-Path $P1Guard 'guard.log'; $n0 = @(Get-Content -LiteralPath $log -ErrorAction SilentlyContinue).Count; Start-P1Guard -DryRun; Stop-P1Guard; Test-P1SystemRestored; "set commands of the guard: $(@(Get-Content -LiteralPath $log | Select-Object -Skip $n0 | Select-String -SimpleMatch 'set the exit node back').Count)"; Start-Sleep -Seconds 3; Start-P1Guard -DryRun; "armed check: $(Assert-P1Armed)"; Send-P1Guard -Action finish; "armed check after finish: $(Assert-P1Armed)"; Test-P1SystemRestored
```

Ожидается: `P1-READY`, `P1-RESULT: state=armed ...`, затем строки `P1-STATUS` и `P1-RESULT: state=tripped ... reason=stop requested system_ok=True` (около 15 секунд: чтения exit node до и после ядра), `SYSTEM-RESTORED`, `set commands of the guard: 0`; затем `P1-READY`, `P1-RESULT: state=armed ...`, `armed check: True`, `P1-RESULT: state=finished ... system_ok=True`, строка `P1-NO-GUARD: process_alive=... status=state=finished ...`, `armed check after finish: False`, `SYSTEM-RESTORED`. Первый запуск проходит настоящий путь срабатывания (завершение процессов по имени — их нет, чтения exit node, проверка адаптера, запись состояний) раньше, чем он понадобится в задаче 23; exit node всё время включён, поэтому сторож ничего не переключает: `set commands of the guard: 0`. Второй проверяет и сам `Assert-P1Armed`: он пропускает живые шаги только при взведённом стороже этого запуска. Сторож при этом только читал систему. `system_ok=False`, `set commands` больше нуля или другой итог — задачу 23 не начинать, причину искать в `C:\code\router\spike\plan1\guard\guard.log`.

- [ ] **Step 5: Папка данных режима разработки**

Состояние встроенного узла копируется из проверок плана 0: узел уже вошёл в сеть. Одновременно два ядра с одним состоянием узла не работают: ядро плана 0 не запущено (шаг 2: `sing-box running false`). Адрес exit node и имя устройства берутся из локального файла и не печатаются.

```powershell
$D = 'C:\code\router\.dev\data'; New-Item -ItemType Directory -Force "$D\tailscale" | Out-Null; robocopy C:\code\router\spike\run\ts-state "$D\tailscale" /E /NFL /NDL /NJH /NJS /NP | Out-Null; "robocopy exit=$LASTEXITCODE"; $ep = (Get-Content C:\code\router\spike\run\phase-b-v2.json -Raw | ConvertFrom-Json).endpoints[0]; & C:\code\router\helper\bin\slctl.exe seed $D $ep.exit_node $ep.hostname; git -C C:\code\router check-ignore -q .dev/data/state.json; "ignored=$($LASTEXITCODE -eq 0)"
```

Ожидается `robocopy exit=1` (файлы скопированы; код меньше 8 — успех), `SEED-OK state.json written`, `ignored=True`. Для продукта состояние узла будет в `%ProgramData%\SplitLane\tailscale\` (план 1B, команда `install`); копия в папку разработки — только для этого плана.

- [ ] **Step 6: Копии `curl.exe` и раскладки**

```powershell
$Bn = 'C:\code\router\spike\plan1\bin'; foreach ($n in 'vpn', 'direct', 'free') { New-Item -ItemType Directory -Force "$Bn\$n" | Out-Null; Copy-Item C:\Windows\System32\curl.exe "$Bn\$n\curl-$n.exe" -Force }
$vpn = '{"id":"6f1c2a10-0000-4000-8000-000000000001","name":"","exe":"C:\\code\\router\\spike\\plan1\\bin\\vpn\\curl-vpn.exe","matcher":{"kind":"file","value":""},"lane":"vpn"}'
$direct = '{"id":"6f1c2a10-0000-4000-8000-000000000002","name":"","exe":"C:\\code\\router\\spike\\plan1\\bin\\direct\\curl-direct.exe","matcher":{"kind":"file","value":""},"lane":"direct"}'
$free = '{"id":"6f1c2a10-0000-4000-8000-000000000003","name":"","exe":"C:\\code\\router\\spike\\plan1\\bin\\free\\curl-free.exe","matcher":{"kind":"file","value":""},"lane":"direct"}'
$utf8 = New-Object Text.UTF8Encoding $false
[IO.File]::WriteAllText('C:\code\router\spike\plan1\lanes-1.json', "{`"defaultLane`":`"vpn`",`"apps`":[$vpn,$direct]}", $utf8)
[IO.File]::WriteAllText('C:\code\router\spike\plan1\lanes-2.json', "{`"defaultLane`":`"vpn`",`"apps`":[$vpn,$direct,$free]}", $utf8)
foreach ($f in 'lanes-1', 'lanes-2') { "$f apps: $(@((Get-Content "C:\code\router\spike\plan1\$f.json" -Raw | ConvertFrom-Json).apps).Count)" }
```

Ожидается `lanes-1 apps: 2` и `lanes-2 apps: 3`. В правила прямой дорожки попадают только эти копии.

- [ ] **Step 7: Убедиться, что git ничего не видит**

```powershell
git -C C:\code\router status --short
```

Ожидается пустой вывод (`helper/bin/`, `.cache/`, `.dev/`, `spike/` игнорируются). Коммита в этой задаче нет.

---

### Task 23: Включение и выключение деления под сторожем

Один сеанс сторожа с одним запросом UAC: служебный процесс включает деление (5.3) с раскладкой из двух копий, переносит третью копию в прямую дорожку (8.3), переживает принудительное завершение служебного процесса (5.7: новый экземпляр возвращает exit node по журналу, останавливает оставшееся ядро и включает деление заново), выключает деление (5.4) и возвращает систему. Основная дорожка всё время — VPN; закрепление программ исполнителя включено (`--dev-pin`). Каждая команда — один вызов PowerShell с тайм-аутом 600000 мс. Каждый живой шаг (4–9) начинается с `if (-not (Assert-P1Armed)) { return }`: если сторож этого запуска не работает или уже не в `armed` (упал, его сняли в Диспетчере задач, он сработал), шаг не выполняет ничего, кроме печати `P1-NO-GUARD: ...`; затем `Set-P1Heartbeat`. Между вызовами не больше 400 секунд, сеанс укладывается в 60 минут предела сторожа. Задачу целиком выполняет основной сеанс. Строки `P1-STATUS: ...` в выводе промежуточные: `Send-P1Guard` и `Start-P1Guard` печатают каждое состояние, которое застали при опросе, поэтому их число может отличаться от показанного (например, `state=starting` пропадает, если сторож взвёлся до первого опроса, а перед ответом на команду появляется прежнее `state=armed`). С ожидаемым сверяются все остальные строки, `P1-RESULT` — по состоянию и полям `result`, `system_ok`; идентификаторы, номера `seq`, секунды и номера строк могут быть другими.

**Если что-то пошло не так** (любой итог, не совпадающий с ожидаемым, в том числе `P1-NO-GUARD`), в одном вызове:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Set-P1Heartbeat; $guardAlive = [bool](Get-Process -Name slguard -ErrorAction SilentlyContinue); "guard alive: $guardAlive"; $s = & "$P1Bin\slctl.exe" -timeout 15s state | Select-Object -Last 1; "helper: $s"; if ((-not $guardAlive) -and ($s -match '^SLCTL: phase=(starting|stopping) ') -and ($s -notmatch 'reason=restore_pending')) { Set-P1Heartbeat; $s = & "$P1Bin\slctl.exe" -timeout 240s wait on,degraded,blocked,error,off | Select-Object -Last 1; "helper: $s" }; if (($s -match '^SLCTL: phase=(on|degraded|blocked|error) ') -or ($s -match '^SLCTL: phase=stopping reason=restore_pending')) { Set-P1Heartbeat; $s = & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; "helper: $s" }; Set-P1Heartbeat; if ($guardAlive) { Stop-P1Guard } elseif ($s -match '^SLCTL: phase=off ') { & "$P1Bin\slctl.exe" -timeout 30s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 2 } else { 'NO-GUARD-HELPER-LEFT: no guard, and the helper did not reach off' }; Test-P1SystemRestored
```

Со сторожем (`guard alive: True`) `slctl disable` отправляется, только если служебный процесс отвечает и его фаза — `on`, `degraded`, `blocked`, `error` или `stopping` с причиной `restore_pending` («Повторить»): тогда он выключает деление штатно. В `starting` и в `stopping` без `restore_pending` (например, старт 5.7 нового экземпляра в шаге 7: после возврата по журналу он сам включит деление снова) блок, как и при молчащем канале, сразу переходит к `Stop-P1Guard`: сторож завершает служебный процесс, возвращает exit node с подтверждением и только потом завершает ядро, независимо от служебного процесса. Без сторожа (`guard alive: False`: он упал или его сняли в Диспетчере задач) служебный процесс с повышенными правами некому завершить, кроме него самого: в `starting` и в `stopping` без `restore_pending` блок сначала ждёт до 240 секунд, пока он сам дойдёт до `on`, `degraded`, `blocked`, `error` или `off`, затем выключает деление (`slctl disable`) и, если выключение дошло до `off`, завершает служебный процесс (`slctl shutdown`, ответ `SLCTL: the helper closed the pipe`). Вызов укладывается в 600 секунд (15 + 240 + 200 + 30). При `SYSTEM-WRONG`, `NO-TERMINAL-STATE`, `NO-GUARD-HELPER-LEFT`, `state=restore_blocked` или `no status` — показать пользователю ручное восстановление. Причину искать в `C:\code\router\.dev\data\logs\helper.log`, `C:\code\router\.dev\data\logs\core.log` и `C:\code\router\spike\plan1\guard\guard.log`; повторный сеанс — только после исправления и с согласия пользователя (новый запрос UAC), начиная с шага 1.

**Files:**
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

**Interfaces:**
- Consumes: всё из задач 19–22; `slctl reset-dev` (задача 20); `Assert-P1Armed`, `Wait-P1Pipe`, `Test-P1DevDataLeft` (задача 21).

- [ ] **Step 1: Готовность (только чтение)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Test-P1Ready; Get-P1Guard; Test-P1SystemRestored; (Get-Content "$P1Data\state.json" -Raw | ConvertFrom-Json) | Select-Object loggedIn, wasOn, defaultLane
```

Ожидается `P1-READY`, `P1-GUARD: state=finished ...` (от шага 4 задачи 22), `SYSTEM-RESTORED` и `loggedIn True`, `wasOn False`, `defaultLane vpn`.

Повторный сеанс после срабатывания сторожа: `P1-NOT-READY` называет `development data left by a tripped run`. Сторож прервал прошлый сеанс при включённом делении, и в `state.json` осталось `wasOn: true` и (или) остался `restore.json`; служебный процесс, которого запустит шаг 3, сам вернул бы exit node по журналу и сразу включил бы деление заново (5.7) — полное переключение внутри шага 3, который за ним не следит. Только если в том же выводе `SYSTEM-RESTORED`, а других причин в `P1-NOT-READY` нет, сбросить данные разработки одним вызовом:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (Get-Process -Name splitlane-helper, sing-box, slguard -ErrorAction SilentlyContinue) { 'RESET-REFUSED: a helper, core or guard is running' } else { & "$P1Bin\slctl.exe" reset-dev $P1Data; Test-P1Ready }
```

Ожидается `RESET-OK wasOn=false core=none restore.json absent` и `P1-READY`. При `SYSTEM-WRONG` сброс не выполнять: показать пользователю ручное восстановление, данные разработки не трогать.

- [ ] **Step 2: Согласие пользователя (нужен пользователь)**

Передайте пользователю целиком текст из блока «Ручное восстановление» раздела Global Constraints (задача 1 могла быть давно и в другом сеансе) и затем:

```text
Сейчас я трижды переключу сеть служебным процессом SplitLane: включу деление, проверю восстановление после сбоя служебного процесса и выключу деление. Каждый раз связь у всех программ компьютера пропадает на 5–25 секунд, открытые соединения обрываются. Основная дорожка всё время — VPN; напрямую ходят только тестовые копии curl. Всё время работы следит отдельный сторож: если моя связь пропадёт больше чем на 7 минут, он сам вернёт exit node в Tailscale. В значке Tailscale в это время ничего не переключайте. Сейчас появится запрос UAC от slguard.exe — его нужно принять.
```

- [ ] **Step 3: Запустить сторож и служебный процесс (нужен пользователь: UAC)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Start-P1Guard
```

Ожидается:

```text
P1-READY
P1-LAUNCH: the UAC prompt of slguard.exe must be accepted
P1-STATUS: state=starting run=1a2b3c4d seq=1
P1-STATUS: state=armed run=1a2b3c4d seq=2
SLCTL: phase=off reason=- allDirect=false defaultLane=vpn apps=0 vpn=null direct=null systemExitNode=false restoreOnDisable=false
P1-RESULT: state=armed run=1a2b3c4d seq=2
```

`systemExitNode` в первом ответе может быть `false`: служебный процесс узнаёт о системном клиенте при опросе раз в 3 секунды. `Start-P1Guard` записывает идентификатор запуска в `guard\run.txt` (по нему `Assert-P1Armed` узнаёт свой сторож) и ждёт канал служебного процесса около минуты (`Wait-P1Pipe`). Другой итог — по таблице `P1-RESULT` задачи 21. `UAC-CANCELLED` — ничего не запущено; без согласия пользователя не повторять. `P1-RESULT: REFUSED` с `development data left by a tripped run` — вернуться к шагу 1.

- [ ] **Step 4: Раскладка и включение**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; if (-not (Select-String -Path "$P1Data\logs\helper.log" -SimpleMatch 'dev-pin=true' -Quiet)) { 'NO-DEV-PIN: the helper runs without --dev-pin; nothing was sent'; return }; & "$P1Bin\slctl.exe" lanes C:\code\router\spike\plan1\lanes-1.json | Select-Object -Last 1; Set-P1Heartbeat; $t0 = Get-Date; & "$P1Bin\slctl.exe" -timeout 240s enable | Select-Object -Last 1; "SLCTL-EXIT $LASTEXITCODE seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; Set-P1Heartbeat; "dev pin in config: $(Select-String -Path "$P1Data\core-config.json" -Pattern '"claude.exe"' -Quiet)"
```

Ожидается:

```text
RESULT {}
SLCTL: phase=on reason=- allDirect=false defaultLane=vpn apps=2 vpn=<ip> direct=<ip> systemExitNode=false restoreOnDisable=true
SLCTL-EXIT 0 seconds=12
dev pin in config: True
```

`phase=error` — включение не удалось, служебный процесс уже откатился (5.4); причина в `reason` (коды 5.2). Выполнить шаг 9, затем разбор. `NO-DEV-PIN` — служебный процесс запущен без закрепления программ исполнителя (первая строка `helper.log`): ничего не отправлено, блок «Если что-то пошло не так». `dev pin in config: False` — выключить немедленно (тот же блок).

- [ ] **Step 5: Дорожки копий**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct C:\code\router\spike\plan1\bin\free\curl-free.exe=vpn | Select-String -Pattern '^LANE|^SLCTL'
```

Ожидается три строки `LANE-OK` (`curl-vpn.exe vpn`, `curl-direct.exe direct`, `curl-free.exe vpn`). `LANE-WRONG` для `curl-direct.exe` — правило файла прямой дорожки не сработало; для `curl-vpn.exe` или `curl-free.exe` — трафик дорожки VPN ушёл напрямую: немедленно блок «Если что-то пошло не так».

- [ ] **Step 6: Перенос копии в прямую дорожку без перезапуска ядра**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $pid0 = (Get-Process sing-box).Id; & "$P1Bin\slctl.exe" lanes C:\code\router\spike\plan1\lanes-2.json | Select-Object -Last 1; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\free\curl-free.exe=direct C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'; "core restarted: $((Get-Process sing-box).Id -ne $pid0)"
```

Ожидается `RESULT {}` (ответ приходит через 3 секунды сверки), три `LANE-OK` и `core restarted: False`.

- [ ] **Step 7: Сбой служебного процесса и восстановление по журналу**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; Send-P1Guard -Action kill-helper; Start-Sleep -Seconds 3; "core running: $([bool](Get-Process sing-box -ErrorAction SilentlyContinue))"; "system exit node on: $((Get-P1SystemExitNode).On)"; Set-P1Heartbeat; Send-P1Guard -Action launch-helper; $t0 = Get-Date; $p = Wait-P1Pipe; "pipe: $p"; if (-not $p) { 'STEP7-NO-PIPE: the new helper did not open its pipe within 60 s'; return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 240s wait on,error | Select-Object -Last 1; "seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; Set-P1Heartbeat; $l = @(Get-Content "$P1Data\logs\helper.log"); $i = @('starting, dev=', 'state: phase=stopping', 'stopping the core left', 'state: phase=starting', 'state: phase=on') | ForEach-Object { $m = $_; ($l | Select-String -SimpleMatch $m | Select-Object -First 1).LineNumber }; "order: $($i -join ',')"
```

Ожидается (числа будут другими):

```text
P1-RESULT: state=armed run=1a2b3c4d seq=3 cmd_id=c0a1b2c3 result=helper processes ended: 1
core running: True
system exit node on: False
P1-RESULT: state=armed run=1a2b3c4d seq=4 cmd_id=c4d5e6f7 result=helper launched
pipe: True
SLCTL: phase=on reason=- allDirect=false defaultLane=vpn apps=3 vpn=<ip> direct=<ip> systemExitNode=false restoreOnDisable=true
seconds=25
order: 1,2,4,6,9
```

Пока служебного процесса нет, ядро работает без надзора, а программы идут через него (раздел 10). Новый экземпляр сначала возвращает exit node по журналу и ждёт подтверждения, потом останавливает старое ядро, потом включает деление заново: признак «было включено» остался установленным. Сторож пишет `helper launched` сразу после запуска процесса, а канал появляется позже (журнал, SHA-256 ядра, автомат), поэтому перед ожиданием фазы шаг ждёт канал (`Wait-P1Pipe`, около минуты; `slctl` и сам ждёт отсутствующий канал). `order:` — номера первых строк `helper.log` нового экземпляра с метками `starting, dev=`, `state: phase=stopping`, `stopping the core left`, `state: phase=starting`, `state: phase=on`. Строки состояния повторяются (удаление журнала публикует ещё одну `stopping`, доказательство туннеля и журнал шага 6 — ещё `starting`), поэтому сверяется только порядок: пять возрастающих чисел. Пустое место между запятыми — метки нет, это несовпадение. `STEP7-NO-PIPE` — блок «Если что-то пошло не так» (он сразу перейдёт к `Stop-P1Guard`).

- [ ] **Step 8: Дорожки после восстановления**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\free\curl-free.exe=direct C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'
```

Ожидается три `LANE-OK`: раскладка сохранилась в `state.json` и применена новым ядром.

- [ ] **Step 9: Выключение и конец сеанса**

`shutdown` и `finish` отправляются, только если выключение дошло до `off`: иначе служебный процесс ещё возвращает exit node, `helper.shutdown` ждал бы этого возврата, а `finish` застал бы служебный процесс живым и сработал бы сторож.

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $t0 = Get-Date; $d = & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; $d; "seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; if ($d -notmatch '^SLCTL: phase=off ') { 'STEP9-STOPPED: disable did not reach off; shutdown and finish not sent'; return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 30s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 2; Set-P1Heartbeat; Send-P1Guard -Action finish; Test-P1SystemRestored
```

Ожидается:

```text
SLCTL: phase=off reason=- allDirect=false defaultLane=vpn apps=3 vpn=null direct=null systemExitNode=true restoreOnDisable=false
seconds=4
SLCTL: the helper closed the pipe
P1-STATUS: state=finished run=1a2b3c4d seq=5 cmd_id=c8d9e0f1 system_ok=True
P1-RESULT: state=finished run=1a2b3c4d seq=5 cmd_id=c8d9e0f1 system_ok=True
SYSTEM-RESTORED
```

`SLCTL: phase=stopping reason=restore_pending ...` и `STEP9-STOPPED` — возврат exit node не подтверждается 30 секунд; служебный процесс продолжает попытки, сторож работает. Прочитать текст ошибки (`error=`) и повторить («Повторить» из 5.4) через минуту одним вызовом:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; Start-Sleep -Seconds 60; Set-P1Heartbeat; $d = & "$P1Bin\slctl.exe" -timeout 200s disable | Select-Object -Last 1; $d; if ($d -notmatch '^SLCTL: phase=off ') { 'STEP9-STOPPED: disable did not reach off; shutdown and finish not sent'; return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 30s shutdown | Select-Object -Last 1; Start-Sleep -Seconds 2; Set-P1Heartbeat; Send-P1Guard -Action finish; Test-P1SystemRestored
```

Ожидается то же, что выше, без строки `seconds=`. Снова `STEP9-STOPPED`, а также `STEP9-STOPPED` с `phase=error` (`adapter_left`) — блок «Если что-то пошло не так». `P1-RESULT: state=tripped` на `finish` — сторож нашёл систему не восстановленной и восстановил её сам; это провал шага. `state=restore_blocked` или `no status` — показать пользователю ручное восстановление (таблица `P1-RESULT` задачи 21).

- [ ] **Step 10: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1» приложения A добавьте (секунды — из вывода шагов 4, 7, 9):

```text
- План 1A, служебный процесс под сторожем, через тестовый клиент: включение по 5.3 (секунд до фазы on: N) и выключение по 5.4 (секунд: N): <подтверждено>.
- План 1A: правила по пути процесса в файлах обеих дорожек и перенос копии между дорожками по lanes.set без перезапуска ядра: <подтверждено>.
- План 1A: служебный процесс завершён принудительно при включённом делении; ядро продолжило работать; новый экземпляр вернул exit node по журналу, остановил оставшееся ядро и включил деление заново (5.7; секунд: N): <подтверждено>.
- План 1A: после выключения exit node и признак доступа к локальной сети системного Tailscale равны исходным, ядра и адаптера нет: <подтверждено>.
```

```powershell
git -C C:\code\router diff -- docs | Select-String -Pattern '\b\d{1,3}(\.\d{1,3}){3}\b' | Where-Object { $_.Line -notmatch '100\.64\.0\.0/10|160\.79\.104\.0/23|1\.1\.1\.1|127\.0\.0\.1' }
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record the first live enable/disable of the helper in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Первая команда должна вывести пустоту.

---

## Что остаётся плану 1B

План 1B начинается с ветки, в которую влит план 1A, и опирается на интерфейсы `engine/ports.go`. В нём:

1. **Коннектор Tailscale (раздел 7).** Конфиг режима настройки (встроенный узел, локальный DNS, `default_domain_resolver`, служба API; без адаптера и без `auto_detect_interface`); `tailscale.login` и `tailscale.logout`; данные узла из службы API (`sing-box api tailscale status`, `exit-node list`, `ping` → `tailscale.path`), список exit node и признак «вход выполнен» в `state.json` вместо `slctl seed`; `backend`, `authUrl`; истечение входа во время работы (`degraded` с `backend = expired`, `tailscale.login` в этом состоянии — выключение 5.4, затем вход); запуск режима настройки при подключении окна и его остановка через 5 минут после отключения последнего окна, перед шагом 3 включения и при `helper.shutdown`; исключение собственного ядра режима настройки из проверки `other_core`. Конфиг режима настройки и чтение данных узла из службы API — новые методы `CoreDriver` (задача 10) в драйвере `singbox`; автомат по-прежнему не знает sing-box.
2. **Живая активность (8.4).** `activity.subscribe` и `detailFor`; сводка раз в секунду; группировка по правилам и по папке программы, особые плитки `system` и `unattributed`, пять минут после последнего соединения; имена узлов из обратного сопоставления.
3. **Сон и смена сети (раздел 10).** Сначала — проверка (и) раздела 12 на ПК (пробуждение после сна и смена сети Wi-Fi ↔ кабель при включённом делении: возвращается ли прямое соединение встроенного узла без перезапуска ядра, известная проблема ядра № 4686), под сторожем задачи 21 и с записью итога в приложение A; от неё зависит, нужен ли перезапуск ядра после пробуждения или смены сети. Затем код: уведомления о питании и об изменении маршрутов (`NotifyRouteChange2`, уведомления о приостановке и возобновлении); ожидание маршрута по умолчанию без отката; 60 секунд на успешные проверки 5.5, затем перезапуск.
4. **Команды `install` и `uninstall` (4.3, 4.2).** Первый шаг, если запуск задачи при входе пользователя (запись `J4` задачи 3, часть проверки (е)) в приложении A не подтверждён: заново зарегистрировать задачу `SplitLane Check` (шаги 5–6 задачи 2, один запрос UAC) и выполнить шаги 1–4 задачи 3 (выход из Windows и вход; решает только `J4`). `J4 FAIL` по разделу 12 останавливает работу и возвращает вопрос владельцу продукта: команда `install` и выход по триггеру через 60 секунд не пишутся. Затем: сверка SHA-256 ядра, копирование служебного процесса, ядра и текстов лицензий (`LICENSE` и `THIRD-PARTY.md` репозитория, `LICENSE` из архива ядра; 4.2 и раздел 14) в `%ProgramFiles%\SplitLane\`, `%ProgramData%\SplitLane\` с явным списком доступа без наследования и подкаталогом `export\`, запись SID, задача `SplitLane Helper` из XML по образцу `TaskXML` задачи 2 (по итогам (е)), отказ при несовпадении пользователя UAC и `--user-sid`, повторная установка через `helper.shutdown`, удаление с возвратом по журналу; режим без `--dev` (пути `%ProgramFiles%` и `%ProgramData%`, SID из данных настройки); при старте по триггеру — выход через 60 секунд без окна и без «было включено»; опознание своего ядра по пути в `%ProgramFiles%\SplitLane\`; флаг `--dev-pin` — по-прежнему только вместе с `--dev`: `parse()` отвергает его без `--dev` (тест `TestDevPinNeedsDev`), чтобы закрепление программ исполнителя не попало в продукт.
5. **`diagnostics.export`** — архив журналов служебного процесса и ядра и снимок состояния в `export\`, без конфига ядра, каталога узла и секретов; ограничение размера `core.log` (на уровне `info` ядро пишет строку на каждое соединение).
6. **Сценарная проверка раздела 12** — `scripts/scenario.mjs` как второй клиент канала (по две копии `curl` и `node`, восемь шагов: включение, TCP и UDP по дорожкам, перенос не дольше 3 секунд при неизменном ядре, смена основной дорожки и «всё напрямую» с возвратом, вложенные правила, контроль утечки, выключение), затем ручной список раздела 12. Живая проверка прямой основной дорожки и «всё напрямую» (в плане 1A основная дорожка всегда VPN) — там же, с согласием пользователя и закреплением программ исполнителя.
7. Методы `tailscale.login`, `tailscale.logout`, `activity.subscribe`, `diagnostics.export` в сборке 1A отвечают `internal`; план 1B заменяет эти ответы и добавляет строки таблицы фаз 5.1 для них в `commands.go` с тестами.

## Открытые вопросы владельцу продукта и исполнителю

- Если задача 1 закончилась `K6 FAIL` при `K7 PASS`, вопрос о больших пакетах UDP (приложение A) решает владелец продукта до задачи 4.
- Проверка пересылки IPv4 считает только физические адаптеры (`Hardware` в `GetIfEntry2Ex`): виртуальные адаптеры WSL и Hyper-V пересылают всегда, и иначе деление не включилось бы у пользователей WSL. Мобильная точка доступа Windows создаёт виртуальный адаптер; если её нужно ловить отдельно, это решение владельца (5.3, шаг 1).
- Правило NRPT системного Tailscale распознаётся по имени `.` и серверу `100.100.100.100`; на этом ПК `accept-dns` выключен, проверено только на подменных данных.
- В режиме разработки ядро запускается из `.cache\core\` (папка пользователя) процессом с правами администратора; в продукте ядро лежит в `%ProgramFiles%\SplitLane\` (план 1B).
- При пропуске задачи 3 путь завершения сеанса 5.4 и запуск задачи при входе пользователя в план 1A входят непроверенными на ПК; это записано в приложении A. Непроверенный запуск при входе блокирует команду `install` плана 1B (пункт 4 выше).
- Сбой ядра в фазе `blocked`: 5.5 велит перезапустить ядро по 5.3, но шаг 6 перезапуска выключил бы exit node, который пользователь только что выбрал в системном клиенте сам, а 5.7 говорит, что выбор пользователя не меняется. План 1A выбирает 5.7: в `blocked` перезапуска нет, 5.4 без возврата (журнал удаляется), фаза `error` с причиной `core_crashed` (Review Focus 1, задача 11). Если владелец продукта решит иначе, меняются ветка `blocked` в `watch` и тест `TestCrashInBlockedKeepsTheUsersExitNode`; решение стоит записать в 5.5 спецификации.
- Проверка (и) раздела 12 (пробуждение и смена сети Wi-Fi ↔ кабель, проблема ядра № 4686) по-прежнему открыта: в приложении A она отмечена «не проверялось», план 1A её не выполняет. Она идёт первым шагом пункта 3 плана 1B, до кода уведомлений о питании и сети.
