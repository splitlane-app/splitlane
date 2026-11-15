# SplitLane, этап 2, часть 1: установщик — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SplitLane ставится и удаляется установщиком, как обычная программа Windows: окно и служебный процесс в `%ProgramFiles%\SplitLane\`, служебный процесс перезапускается после сбоя, канал принимает только программы SplitLane, установщик собирается и проверяется на GitHub Actions.

**Architecture:** Служебный процесс переезжает в `%ProgramFiles%\SplitLane\helper\` и получает надзирателя (тот же exe: процесс задачи запускает рабочую копию `--worker` и перезапускает её после сбоя), подкоманду `prepare` (остановка прежней версии до замены файлов), `install` на месте вместо копирования и проверку вызывающей программы в канале. Установщик — NSIS из electron-builder с включаемым скриптом `app/build/installer.nsh`, который вызывает `prepare`, `install` и `uninstall` служебного процесса. GitHub Actions собирает всё с нуля, ставит и удаляет установщик на бегуне и хранит установщик как артефакт.

**Tech Stack:** Go 1.27.4 (`golang.org/x/sys/windows`, `github.com/Microsoft/go-winio` — уже в `go.mod`), Electron 45.2.1, electron-builder 26.18.0 (NSIS, `electronFuses`), pnpm 12.9.1, Node.js 24.21.0, GitHub Actions (`windows-latest`).

**Spec:** `docs/superpowers/specs/2026-11-15-splitlane-stage2-installer-design.md` (далее «спецификация»); основа — `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` («спецификация 1»).

## Global Constraints

- Работа идёт в ветке `stage2-installer`. Каждая задача с кодом заканчивается коммитом; сообщение — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Текст плана — по-русски; код, команды, идентификаторы — по-английски.
- Пути продукта: `place.ProductDir()` = `<Program Files из реестра>\SplitLane` (окно, деинсталлятор); `place.ProgramDir()` = `<ProductDir>\helper` (служебный процесс и ядро). Program Files — значение реестра, как сейчас в `place`, не переменная окружения.
- Шесть файлов служебного процесса (`install.Files`): `splitlane-helper.exe`, `sing-box.exe`, `libcronet.dll`, `sing-box-LICENSE`, `LICENSE`, `THIRD-PARTY.md`. Те же шесть имён в корне `ProductDir` — старая раскладка этапа 1, установщик их удаляет.
- Коды выхода служебного процесса не меняются: 0 OK, 2 usage, 3 pipe taken, 4 bad core, 5 internal, 6 not installed, 7 refused. Надзиратель: коды рабочей копии 0, 3, 4, 6, 7 — окончательные (выход с тем же кодом), любой другой — сбой; пауза перед перезапуском 5 секунд; не больше 3 перезапусков за 10 минут.
- Строки подкоманд: `PREPARE-OK` / `PREPARE-REFUSED: <причина>`, `INSTALL-OK` / `INSTALL-REFUSED: <причина>`, `UNINSTALL-OK` / `UNINSTALL-REFUSED: <причина>`, `UNINSTALL-NOTE: ...`; отказ — код 7.
- Проверка вызывающей программы (только режим продукта): клиент допускается, если его маркер повышен (`TokenElevation`), или путь его образа лежит внутри `ProductDir` (без учёта регистра, по границе папки). Ошибка определения — отказ. В журнал — только имя файла образа и PID.
- Предохранители Electron: `runAsNode: false`, `enableNodeOptionsEnvironmentVariable: false`, `enableNodeCliInspectArguments: false`, `onlyLoadAppFromAsar: true`.
- Версия продукта — `version` в `app/package.json`, `0.2.0`; служебный процесс получает её `-X main.version=<version>`. Установщик — `SplitLane-Setup-<version>.exe` в `app\release\`, рядом `SplitLane-Setup-<version>.exe.sha256` (строка `<sha256>  SplitLane-Setup-<version>.exe`).
- Пакеты: новых пакетов npm и модулей Go нет. GitHub Actions: `windows-latest`, Go 1.27.4, Node.js 24.21.0, pnpm 12.9.1; каждое действие закреплено полным SHA коммита с комментарием версии.
- Модульные тесты не трогают систему: нет настоящего канала продукта, `schtasks`, UAC, записи в реестр, `%ProgramFiles%`, `%ProgramData%`. Разрешено: временные папки (в том числе соединение каталогов `mklink /J` внутри временной папки), тестовые каналы `\\.\pipe\SplitLane.Test.<число>`, запуск собранного тестового exe из временной папки.
- Живой шаг — любой шаг, после которого системный exit node может оказаться выключен (установка поверх работающего продукта, включение деления, сбой рабочей копии). Он идёт только под сторожем `slguard.exe` (`spike\plan1\p1lib.ps1`: `Start-P1Guard -NoHelper`, `Assert-P1Armed`, `Set-P1Heartbeat`, `Send-P1Guard`, `Test-P1SystemRestored`); между вызовами не больше 400 секунд. Запрос UAC принимает человек; отменённый запрос без согласия пользователя не повторяется. `tailscale set --exit-node=...` вручную не выполняется. Режим «всё напрямую» и фаза `blocked` в живых шагах не вызываются; основная дорожка — VPN. `electron-vite dev` не запускается никогда (он запускает задачу продукта).
- Личные данные: в плане, тестах, отчётах и коммитах нет имён устройств и сети Tailscale, имени ПК, настоящих адресов, SID и имени учётной записи владельца, ссылок для входа. Тестовые SID — вымышленные (`S-1-5-21-1000-2000-3000-1001`).
- Файлы PowerShell содержат только ASCII. `app/build/installer.nsh` — UTF-8 с BOM (русские строки мастера).
- Полная проверка служебного процесса (из `C:\code\router\helper`): `go vet ./...` и `go test ./...` без ошибок. Окна (из `C:\code\router\app`): `pnpm typecheck; pnpm test; pnpm build` без ошибок. Go — `%LOCALAPPDATA%\Programs\go\bin\go.exe`.

## Review Focus

Условия, которые спецификация подразумевает, но прямо не описывает; от более вероятного к менее вероятному. Каждое закреплено тестом в задаче, которая владеет кодом.

1. Обновление поверх этапа 1, когда деление включено: служебный процесс этапа 1 работает, системный exit node снят. Ожидание: `prepare` останавливает его через `helper.shutdown` с подтверждённым возвратом и ждёт, пока процесс и ядро действительно завершатся (файлы иначе заняты); оставшееся ядро этапа 1 ищется и по старому пути в корне. Задача 3: `TestPrepareStopsTheRunningHelperAndWaits`, `TestPrepareFindsTheLegacyCore`, `TestPrepareRefusesWhileProcessesStay`.
2. Рабочая копия падает сразу после каждого старта. Ожидание: надзиратель делает не больше 3 перезапусков за 10 минут и выходит с кодом последнего сбоя, а не крутится бесконечно; сбои старше 10 минут не считаются. Задача 2: `TestRunStopsAfterThreeRestartsInTenMinutes`, `TestDecideForgetsOldRestarts`.
3. UAC пройден другим администратором (пользователь сеанса — не администратор). Ожидание: `prepare --session-user` отказывает до любых изменений; у сеанса без пользователя (сеанс 0, бегун GitHub Actions) берётся пользователь процесса. Задача 3: `TestSessionUserMismatchRefusesBeforeChanges`, `TestSessionWithoutUserTakesTheProcessUser`.
4. Клиент исчезает сразу после подключения (процесс не открыть) или его путь похож на папку продукта только началом (`C:\Program Files\SplitLaneX\...`). Ожидание: отказ, служебный процесс продолжает принимать других клиентов. Задача 4: `TestAllowedFolderBoundary`, `TestCheckErrorRejects`, `TestRejectedClientDoesNotStopTheServer`.
5. `%AppData%\SplitLane` (папка пользователя) оказывается соединением каталогов на чужую папку. Ожидание: `uninstall --purge` с правами администратора удаляет только само соединение, не содержимое цели. Задача 3: `TestPurgeDoesNotFollowAJunction`.

---

## Как читать этот план

**Что уже есть.** Служебный процесс (планы 1A, 1B) и окно (план 2) работают; продукт этапа 1 ставит `install --user-sid` из окна, копируя шесть файлов в `%ProgramFiles%\SplitLane\`. Сборка: `scripts\fetch-core.ps1` (ядро в `.cache\core`), `scripts\build-helper.ps1` (`dist\helper\`), `scripts\build-app.ps1` (`app\release\win-unpacked\`). Сторож живых проверок — `helper\cmd\slguard` и `spike\plan1\p1lib.ps1` (папка `spike\` не в git).

**Код в плане.** Задачи 1–7 — код с тестами; система не трогается, кроме сборки. Для каждой задачи план задаёт обязательное: файлы, интерфейсы (точные имена и типы), поведение, тесты (имена и что проверяют) и команды проверки. Подробности реализации, которых нет в плане, исполнитель берёт из кода рядом и из спецификации. Задача 8 — живая проверка на ПК владельца.

**Порядок задач.** 1 — место файлов и версия; 2 — надзиратель; 3 — `install` на месте, `prepare`, `uninstall`, пользователь сеанса; 4 — проверка вызывающей программы; 5 — окно; 6 — установщик; 7 — проверка установщика и GitHub Actions; 8 — живая проверка.

### Карта файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `helper/internal/place/place.go` | `ProductDir`, `ProgramDir` | 1 |
| `helper/internal/coreconfig/config_test.go`, `testdata/*.golden.json` | путь служебного процесса в эталонах | 1 |
| `scripts/build-helper.ps1`, `app/package.json` | версия в сборке, Go из `PATH` | 1 |
| `helper/internal/supervise/supervise.go` | решение и цикл надзирателя | 2 |
| `helper/cmd/splitlane-helper/main.go` | надзиратель или рабочая копия, подкоманды, проверка канала | 2, 3, 4 |
| `helper/cmd/slguard/guard.go`, `system.go` | действие сторожа `kill-worker` | 2 |
| `helper/internal/install/install.go`, `system.go` | `install` на месте, `prepare`, `uninstall`, пользователь сеанса, процессы продукта | 3 |
| `helper/internal/pipe/caller.go`, `server.go` | проверка вызывающей программы | 4 |
| `app/src/main/setup.ts`, `app/src/main/index.ts`, `app/src/shared/*`, `app/src/preload/index.ts`, экраны | папка служебного процесса, неустановленное окно, версии | 5 |
| `app/electron-builder.yml`, `app/build/installer.nsh`, `app/build/icon.ico`, `app/scripts/make-icon.mjs` | установщик, значок, предохранители | 6 |
| `scripts/build-installer.ps1`, `scripts/build-app.ps1`, `THIRD-PARTY.md` | сборка установщика | 6 |
| `scripts/test-installer.ps1`, `.github/workflows/build.yml`, `README.md` | проверка установщика, CI | 7 |

### Где код задач

| Задача | Коммиты |
|---|---|
| 1 | 753f25e |
| 2 | adf88fc, 10a1715 |
| 3 | 81cb6c8, 6b7d710 |
| 4 | 380d446, 5eb5af6, 73bb993, f1c2ffa |
| 5 | 821e990, ef6b760 |
| 6 | 7da472d, 595d2be |
| 7 | fc453ba, db88565, e7b215d, c845582 (зелёный запуск 43706518032) |
| итоговая рецензия ветки | 89678fd, b7e5ef2, d9c0c3b, 5b3b49e (зелёный запуск 43711189737) |

После итоговой рецензии изменились тексты и правила, которые цитируют задачи 2 и 4: строка надзирателя при отказе — `...; 3 restarts within 10m already; giving up`; без повышения канал принимает только `<ProductDir>\SplitLane.exe`, а не любую программу папки (спецификация, раздел 5). Источник истины — спецификация и код.

---

### Task 1: Место файлов и версия

Спецификация, разделы 2, 6 (версия).

**Files:**
- Modify: `helper/internal/place/place.go`, `helper/internal/place/place_test.go`
- Modify: `helper/cmd/splitlane-helper/main.go` (комментарии о месте; код `productPlace` не меняется — он уже берёт `programDir()`)
- Modify: `helper/internal/install/install.go` (комментарии `Options.ProgramDir`)
- Modify: `helper/internal/coreconfig/config_test.go` и эталоны в `helper/internal/coreconfig/testdata/`
- Modify: `scripts/build-helper.ps1`, `app/package.json`

**Interfaces:**
- Produces:
  - `func ProductDir() (string, error)` — `<Program Files>\SplitLane` (то, что сейчас возвращает `ProgramDir`).
  - `func ProgramDir() (string, error)` — `<ProductDir>\helper`.

**Поведение:**
- `ProgramDir` строится из `ProductDir`; ошибки — как сейчас.
- `config_test.go`: `HelperPath` эталонных конфигов — `C:\Program Files\SplitLane\helper\splitlane-helper.exe`; эталоны пересобираются штатным способом пакета (флаг обновления эталонов, если он есть; иначе правкой строки правила). Ни одно другое поле эталонов не меняется.
- `build-helper.ps1`: версия читается из `app\package.json` (`version`, проверка `^\d+\.\d+\.\d+$`, иначе `BUILD-FAILED`), сборка с `-ldflags "-H=windowsgui -X main.version=<version>"`; Go — `%LOCALAPPDATA%\Programs\go\bin\go.exe`, если он есть, иначе `go` из `PATH` (на GitHub Actions); в последней строке `BUILD-OK dist\helper <version>`. Комментарий в начале скрипта — о новой раскладке (`helper\`, установщик).
- `app/package.json`: `"version": "0.2.0"`.

Тесты: `TestProductDirIsUnderProgramFiles`, `TestProgramDirIsTheHelperFolder` (на подменной функции чтения реестра, как существующие тесты `place`); эталонные тесты `coreconfig` проходят с новым путём.

- [ ] Шаги: тесты → провал → реализация → `go vet ./...`, `go test ./...` → сборка `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-helper.ps1` (последняя строка `BUILD-OK dist\helper 0.2.0`) → коммит `feat(helper): the helper folder is ProgramFiles\SplitLane\helper; version from package.json`.

### Task 2: Надзиратель

Спецификация, раздел 4.

**Files:**
- Create: `helper/internal/supervise/supervise.go`, `helper/internal/supervise/supervise_test.go`
- Modify: `helper/cmd/splitlane-helper/main.go`, `helper/cmd/splitlane-helper/main_test.go`
- Modify: `helper/cmd/slguard/guard.go`, `helper/cmd/slguard/guard_test.go` (и `system.go`, если нужен список PID по образу)

**Interfaces:**
- Produces (`package supervise`):
  - `const Pause = 5 * time.Second`, `const Window = 10 * time.Minute`, `const MaxRestarts = 3`
  - `func Final(code int) bool` — `true` для 0, 3, 4, 6, 7.
  - `func Decide(code int, restarts []time.Time, now time.Time) bool` — перезапускать ли: `!Final(code)` и перезапусков в `(now-Window, now]` меньше `MaxRestarts`.
  - `type Worker interface { Wait() int }` — код выхода рабочей копии (аварийный код NTSTATUS — как есть, `int(uint32)`).
  - `type Clock interface { Now() time.Time; After(time.Duration) <-chan time.Time }`
  - `func Run(start func() (Worker, error), c Clock, logf func(format string, args ...any)) int` — запускает, ждёт, решает; ошибка запуска — строка журнала и код 5; возвращает код выхода надзирателя.
- Флаг `--worker` у `splitlane-helper.exe` (только режим продукта, вместе с `--dev` — ошибка использования, код 2).

**Поведение:**
- Режим продукта без `--worker`: проверка места (тот же `productPlace` exe, что сейчас; при отказе — его код, рабочая копия не запускается), затем `supervise.Run` с запуском `<свой exe> --worker` (без окна консоли: exe собран `-H=windowsgui`; стандартные потоки — `NUL`). Объекта задания нет (спецификация, раздел 4).
- Режим продукта с `--worker`: всё, что режим продукта делает сейчас.
- `--dev`: как сейчас, надзирателя нет.
- Журнал надзирателя — строки в `<DataDir>\logs\helper.log` в формате журнала служебного процесса, файл открывается на дозапись на одну строку и сразу закрывается: `supervisor: the helper ended with code <n>; starting it again in 5s (restart <k> of 3 in 10m)`, `supervisor: the helper ended with code <n>; not starting it again`, `supervisor: the helper ended with code <n> 3 times within 10m; giving up`. Нет папки журнала — строки пропускаются.
- `slguard`: новое действие `kill-worker` — завершает процессы `splitlane-helper.exe`, чей родитель — тоже `splitlane-helper.exe` (рабочие копии), надзирателя не трогает; результат `worker processes ended: <n>`. Список действий в комментарии `command` обновляется.

Тесты (`supervise_test.go`, подменные `Worker` и `Clock`): `TestFinalCodes`, `TestDecideRestartsAFailure`, `TestDecideForgetsOldRestarts` (3 перезапуска 11 минут назад не мешают), `TestRunExitsWithAFinalCode` (0 → 0 без перезапуска; 3 → 3), `TestRunRestartsAfterAFailureAndPause` (код 2, затем 0: две рабочие копии, пауза 5 с через `Clock.After`, итог 0), `TestRunStopsAfterThreeRestartsInTenMinutes` (четыре сбоя подряд: 3 перезапуска, итог — код последнего сбоя), `TestRunLogsEveryDecision`, `TestRunStartFailureIsInternal`. `main_test.go`: `TestWorkerFlagNeedsProductMode` (`--dev --worker` → 2), `TestProductModeSupervisesItself` (подменная функция запуска рабочей копии получает свой exe и `--worker`). `guard_test.go`: `TestKillWorkerCommand` (на подменной системе: из надзирателя и рабочей копии завершается только рабочая копия).

- [ ] Шаги: тесты → провал → реализация → полная проверка служебного процесса → коммит `feat(helper): a supervisor restarts the worker after a failure`.

### Task 3: `install` на месте, `prepare`, `uninstall`, пользователь сеанса

Спецификация, раздел 3.

**Files:**
- Modify: `helper/internal/install/install.go`, `helper/internal/install/system.go`, `helper/internal/install/install_test.go`, `helper/internal/install/system_test.go`
- Modify: `helper/cmd/splitlane-helper/main.go`, `helper/cmd/splitlane-helper/main_test.go` (подкоманда `prepare`, флаг `--session-user`, справка в комментарии пакета)

**Interfaces:**
- `Options` получает: `ProductDir string` (`place.ProductDir`), `SessionUser bool` (флаг `--session-user`; с `--user-sid` вместе — ошибка использования), `UserDataDir string` (`<RoamingAppData пользователя процесса>\SplitLane`, `windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, 0)`; используется только `--purge`).
- `Sys` получает:
  - `SessionUserSID() (string, error)` — SID пользователя сеанса этого процесса (`ProcessIdToSessionId`, `WTSQuerySessionInformationW` с `WTSUserName` и `WTSDomainName`, затем `LookupAccountName`); у сеанса без пользователя — `""` без ошибки.
  - `ProductProcesses(productDir string) ([]Proc, error)`, `type Proc struct { PID uint32; Image string }` — процессы с образом `splitlane-helper.exe` или `sing-box.exe` внутри `productDir` (на любой глубине), кроме самого процесса.
- Produces: `func Prepare(o Options, s Sys) error` (строки `PREPARE-*`, как `Install`).
- Подкоманды: `prepare --session-user`, `install --session-user | --user-sid <SID>`, `uninstall [--purge]`.

**Поведение:**
- Пользователь установки (`resolveUser`): `--user-sid` — как сейчас; `--session-user` — `SessionUserSID()`, а при `""` — пользователь процесса (`Token`). Дальше — существующая `checkUser` (повышение, пользователь процесса = пользователь установки, имя учётной записи).
- `prepare`: `resolveUser`+`checkUser` → `CheckDir(DataDir)` → `stopOld` → ожидание `waitGone`: `ProductProcesses(ProductDir)` раз в 500 мс до 10 секунд; остались — отказ `<имена образов> still running (process <pid>, ...); the files cannot be replaced`. Ничего не пишет и не удаляет. Работает из любой папки (обычно временная папка установщика).
- `stopOld`: оставшееся ядро ищется по образу `<ProgramDir>\sing-box.exe`, а если не найдено — `<ProductDir>\sing-box.exe` (этап 1).
- `install`: только из `ProgramDir` — иначе отказ `install runs from <ProgramDir>, where the installer puts the files`; файлы не копируются; `readSource(ProgramDir)` со сверкой SHA-256 ядра — как сейчас; затем `stopOld` и шаги 5–7 как сейчас; задача — с командой `<ProgramDir>\splitlane-helper.exe`.
- `uninstall`: отказа «из папки программы» больше нет; после `stopOld` — `waitGone`; затем задача; папку программы не удаляет; `--purge` — `DataDir`, затем `UserDataDir` (нет папки — не ошибка), затем `UNINSTALL-NOTE` как сейчас. Удаление не проходит по соединениям каталогов и символическим ссылкам (удаляется сама ссылка).
- Строки хода (`progress`) — без путей пользователя, кроме `ProgramDir`/`DataDir`.

Тесты (`install_test.go`, подменная `Sys`): `TestInstallRunsOnlyFromTheProgramFolder`, `TestInstallDoesNotCopy` (нет `WriteFile` в папку программы; задача с командой `ProgramDir\splitlane-helper.exe`), `TestInstallStillChecksTheCore`, `TestSessionUserTakesTheSessionUser`, `TestSessionWithoutUserTakesTheProcessUser`, `TestSessionUserMismatchRefusesBeforeChanges` (для `prepare` и `install`: ни одного вызова изменения `Sys`), `TestPrepareStopsTheRunningHelperAndWaits`, `TestPrepareRefusesWhileProcessesStay` (подменные часы: отказ через 10 с с именем образа), `TestPrepareFindsTheLegacyCore`, `TestPrepareRefusesAnUnconfirmedRestore`, `TestUninstallKeepsTheProgramFiles`, `TestUninstallWaitsForProcesses`, `TestPurgeRemovesTheUserFolder`. `system_test.go` (временные папки): `TestPurgeDoesNotFollowAJunction` (соединение `mklink /J` во временной папке на другую временную папку с файлом: после удаления файл цели на месте), `TestProductProcessesSkipsItself` (собственный процесс теста под временной «папкой продукта» не попадает в список). `main_test.go`: `TestPrepareFlags`, `TestSessionUserAndUserSidExclude`.

- [ ] Шаги: тесты → провал → реализация → полная проверка служебного процесса → коммит `feat(helper): install in place, prepare before an upgrade, the session user`.

### Task 4: Проверка вызывающей программы

Спецификация, раздел 5.

**Files:**
- Create: `helper/internal/pipe/caller.go`, `helper/internal/pipe/caller_test.go`
- Modify: `helper/internal/pipe/server.go`, `helper/internal/pipe/server_test.go`
- Modify: `helper/cmd/splitlane-helper/main.go` (режим продукта передаёт проверку, `--dev` — нет)

**Interfaces:**
- Produces (`package pipe`):
  - `type Caller struct { PID uint32; Image string; Elevated bool }`
  - `func Allowed(c Caller, productDir string) bool` — `c.Elevated`, или `c.Image` внутри `productDir` (сравнение без учёта регистра, `productDir` — граница папки: `C:\Program Files\SplitLaneX\a.exe` не внутри `C:\Program Files\SplitLane`).
  - `func ClientOf(conn net.Conn) (Caller, error)` — дескриптор соединения go-winio (`Fd()`), `GetNamedPipeClientProcessId`, `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`, `QueryFullProcessImageName`, `OpenProcessToken(TOKEN_QUERY)` и `Token.IsElevated()`.
  - `type Check func(conn net.Conn) error` — `nil`: клиент допущен.
  - `func ProductCheck(productDir string) Check` — `ClientOf` + `Allowed`; ошибка: `not part of SplitLane: <base name of image> (process <pid>)` или `the client process: <err>`.
  - `Listen(name, userSID string, h Handler, log func(string, ...any), check Check) (*Server, error)` — новый последний параметр; `nil` — без проверки (режим разработки, тесты).

**Поведение:**
- Проверка — в горутине клиента до чтения первой строки (цикл `Accept` не ждёт); отказ — строка журнала `pipe: refused a client: <ошибка Check>`, соединение закрывается без ответа, клиент не попадает в `clients` и не меняет `SetClients`.
- Остальное поведение сервера без изменений.

Тесты: `TestAllowedFolderBoundary` (внутри, глубже, другой регистр, `SplitLaneX`, относительный путь, пустой путь), `TestAllowedElevated`, `TestClientOfThisProcess` (тестовый канал в том же процессе: `Image` = `os.Executable()` без учёта регистра, `PID` = свой), `TestCheckErrorRejects` (проверка возвращает ошибку → клиент получает EOF до ответа на `hello`), `TestRejectedClientDoesNotStopTheServer` (после отказа второй клиент с допускающей проверкой получает ответ на `hello`), `TestNilCheckAcceptsAll`.

- [ ] Шаги: тесты → провал → реализация → полная проверка служебного процесса → коммит `feat(helper): the pipe accepts only SplitLane programs and elevated callers`.

### Task 5: Окно: установленная раскладка и версии

Спецификация, разделы 3 (ремонт, автозапуск), 6 (версия).

**Files:**
- Modify: `app/src/main/setup.ts`, `app/src/main/setup.test.ts`, `app/src/main/index.ts`
- Modify: `app/src/shared/ipc.ts` (или файл, где описан мост), `app/src/preload/index.ts`
- Modify: `app/src/renderer/screens/SetupScreen.tsx`, `SetupScreen.test.tsx`, `ConnectScreen.tsx`, `ConnectScreen.test.tsx`, `app/src/renderer/i18n/ru.ts`

**Interfaces:**
- `helperSourceDir(o: { isPackaged: boolean; execPath: string; appPath: string }): string` — собранное приложение: `<dirname(execPath)>\helper`; разработка — как сейчас.
- `export function installedAt(execPath: string, programFiles: string | undefined): boolean` — `execPath` внутри `<programFiles>\SplitLane\` (без учёта регистра, по границе папки); `programFiles` пустой — `false`.
- Мост: `window.splitlane.appInfo(): Promise<{ version: string; installed: boolean }>` (основной процесс: `app.getVersion()`; `installed` — `app.isPackaged && installedAt(process.execPath, process.env.ProgramFiles)`; в разработке — `true`, чтобы экран настройки работал с `dist\helper`).

**Поведение:**
- Экран «Первая настройка» в неустановленном собранном окне: вместо кнопки — текст `setup.notInstalled`: «Эта копия SplitLane не установлена. Установите SplitLane установщиком: он поставит и фоновую службу.»
- Экран «Подключение»: строка `connect.versions`: «SplitLane {app} · служба {helper} · ядро {core}» (`{helper}`, `{core}` — из `hello`; без связи — только «SplitLane {app}» по ключу `connect.versionApp`).
- Остальные тексты не меняются.

Тесты: `helperSourceDir for packaged and dev` (обновлённый: `C:\Program Files\SplitLane\SplitLane.exe` → `C:\Program Files\SplitLane\helper`), `installedAt inside, outside, prefix, case, empty`, `SetupScreen shows the install hint when not installed`, `ConnectScreen shows the versions`, `ConnectScreen shows the app version without a link`.

- [ ] Шаги: тесты → провал → реализация → `pnpm typecheck; pnpm test; pnpm build` → коммит `feat(app): the installed layout, the repair path and the versions`.

### Task 6: Установщик

Спецификация, разделы 3, 5 (предохранители), 6.

**Files:**
- Modify: `app/electron-builder.yml`, `app/package.json` (скрипт `dist`), `scripts/build-app.ps1`, `THIRD-PARTY.md`
- Create: `app/build/installer.nsh` (UTF-8 с BOM), `app/build/icon.ico`, `app/scripts/make-icon.mjs`, `scripts/build-installer.ps1`

**Поведение:**
- `electron-builder.yml`: `directories.buildResources: build`; `extraFiles: [{ from: ../dist/helper, to: helper }]` вместо `extraResources`; `win.target: nsis`, `win.icon: build/icon.ico`; строка `signAndEditExecutable: false` убирается (значок и сведения о версии в exe; подписи нет — сертификат не задан); `nsis`: `oneClick: false`, `perMachine: true`, `allowToChangeInstallationDirectory: false`, `createDesktopShortcut: true`, `createStartMenuShortcut: true`, `runAfterFinish: true`, `deleteAppDataOnUninstall: false`, `installerLanguages: [ru_RU]`, `language: 1049`, `include: build/installer.nsh`, `artifactName: SplitLane-Setup-${version}.${ext}`, `uninstallDisplayName: SplitLane`; `electronFuses` — четыре значения из Global Constraints.
- `scripts/build-app.ps1` собирает по-прежнему папку (`electron-builder --dir`) и проверяет шесть файлов в `win-unpacked\helper\`.
- `installer.nsh`:
  - `customCheckAppRunning` (только в установщике, `!ifndef BUILD_UNINSTALLER`; в деинсталляторе — штатная проверка): штатная `_CHECK_APP_RUNNING`; затем новый `splitlane-helper.exe` из `dist\helper` во время сборки извлекается в `$PLUGINSDIR` и запускается `prepare --session-user` через `nsExec::ExecToStack`; код не 0 — сообщение с последней строкой вывода (не в тихом режиме), `SetErrorLevel 2`, `Quit`; затем удаляются шесть файлов этапа 1 из корня `$INSTDIR`.
  - `customInstall`: `"$INSTDIR\helper\splitlane-helper.exe" install --session-user`; код не 0 — сообщение, `SetErrorLevel 2`, `Abort`.
  - `customUnWelcomePage`: страница с пояснением и галочкой «Удалить настройки и вход в Tailscale» (выключена); в тихом режиме вместо неё — параметр `/PURGE` (`GetParameters`/`GetOptions`).
  - `customUnInstall`: при `${isUpdated}` ничего; иначе `"$INSTDIR\helper\splitlane-helper.exe" uninstall` (с `--purge` по галочке или `/PURGE`); код не 0 — сообщение, `SetErrorLevel 2`, `Abort` (файлы остаются); при успехе с удалением данных — сообщение со строкой `UNINSTALL-NOTE` (не в тихом режиме).
  - Тексты мастера: «Подготовка: служебный процесс прежней версии останавливается…», «SplitLane не установлен: <причина>», «SplitLane не удалён: <причина>», «Удалить настройки и вход в Tailscale», пояснение страницы удаления: «Без галочки настройки окна и вход встроенного узла в Tailscale остаются для следующей установки.»
- `app/scripts/make-icon.mjs`: рисует знак дорожек (геометрия и цвета — как у `laneGlyphBitmap` в `app/src/main/trayModel.ts`, на тёмном фоне `#0e1014` со скруглением) в размерах 16, 24, 32, 48, 64, 128, 256 и пишет ICO с PNG внутри (`node:zlib`, без пакетов); `app/build/icon.ico` — его результат, в git.
- `scripts/build-installer.ps1`: `build-helper.ps1` → в `app`: `pnpm install --frozen-lockfile`, двоичный файл Electron, `pnpm typecheck`, `pnpm test`, `pnpm build`, `electron-builder --win nsis` → проверка, что есть `app\release\SplitLane-Setup-<version>.exe` → файл `.sha256` → последняя строка `BUILD-OK <путь установщика>`, иначе `BUILD-FAILED <причина>`. Если electron-builder не может распаковать свой набор `winCodeSign` без права на символические ссылки (известная беда на Windows без режима разработчика), скрипт заранее распаковывает его в кэш electron-builder без папки `darwin` — исполнитель проверяет на этом ПК и записывает в отчёт, понадобилось ли.
- `THIRD-PARTY.md`: строка об NSIS (zlib/libpng) и шаблонах electron-builder (MIT).

Проверка задачи (модульных тестов NSIS нет): `scripts\build-installer.ps1` → `BUILD-OK`; распаковка установщика 7-Zip-ом (если есть) или проверка состава `app\release\win-unpacked\` после `--dir`: `helper\` с шестью файлами; значок и `FileDescription` `SplitLane.exe` (`(Get-Item ...).VersionInfo`); байты предохранителей (как в задаче 7). Установщик в этой задаче не запускается.

- [ ] Шаги: реализация → `pnpm typecheck; pnpm test; pnpm build` → `scripts\build-installer.ps1` → проверка состава → коммит `feat(app): NSIS installer with prepare, install and uninstall of the helper`.

### Task 7: Проверка установщика и GitHub Actions

Спецификация, разделы 6, 7.

**Files:**
- Create: `scripts/test-installer.ps1`, `.github/workflows/build.yml`
- Modify: `README.md` (установка, сборка, CI)

**Поведение:**
- `scripts/test-installer.ps1 -Installer <путь>` — только для бегуна CI (администратор, без Tailscale, без продукта): при запуске проверяет, что задачи «SplitLane Helper» и `%ProgramFiles%\SplitLane` нет, иначе `TEST-REFUSED` (на ПК владельца не запускается). Шаги, каждый с понятной строкой `TEST-FAIL <шаг>: <что>`:
  1. Старая раскладка: создать `%ProgramFiles%\SplitLane\` с шестью файлами этапа 1 (копии из `dist\helper`).
  2. Тихая установка (`/S`, ожидание процесса) → код 0; в корне нет шести файлов этапа 1; есть `SplitLane.exe`, `Uninstall SplitLane.exe`, `helper\` с шестью файлами; задача есть, её команда — `%ProgramFiles%\SplitLane\helper\splitlane-helper.exe`; `%ProgramData%\SplitLane\setup.json` с SID пользователя бегуна; список доступа `%ProgramData%\SplitLane` — `install.DataSDDL`; байты предохранителей `SplitLane.exe` (строка-метка `dL7pKGdnNz796PbbjQWNKmHXBZaB9tsX`, за ней версия и состояния; порядок — по `@electron/fuses` из `node_modules`) соответствуют Global Constraints; `FileDescription` — `SplitLane`.
  3. Повторная установка поверх (метка-файл в `%ProgramData%\SplitLane` и `%AppData%\SplitLane`) → код 0; метки на месте, задача есть.
  4. Тихое удаление без `/PURGE` (деинсталлятор NSIS копирует себя во временную папку: ждать исчезновения `%ProgramFiles%\SplitLane` до 120 с) → задачи нет, папки программы нет, `%ProgramData%\SplitLane` и метки на месте.
  5. Установка и тихое удаление с `/PURGE` → нет `%ProgramData%\SplitLane` и `%AppData%\SplitLane`.
  6. Последняя строка `TEST-OK`.
- `.github/workflows/build.yml`: на `push` в любую ветку, `pull_request`, метку `v*` и вручную; одна задача `windows-latest`, `permissions: contents: read` (задаче выпуска — `contents: write`); шаги: checkout → setup-go 1.27.4 → setup-node 24.21.0 → pnpm 12.9.1 → `scripts\fetch-core.ps1` → `go vet ./...`, `go test ./...` (в `helper`) → `scripts\build-installer.ps1` → `scripts\test-installer.ps1` → загрузка артефакта (установщик и `.sha256`). На метке: проверка `v<version>` = метка (иначе ошибка), черновик выпуска `gh release create <метка> --draft --title "SplitLane <version>"` с двумя файлами. Действия закреплены SHA (исполнитель берёт SHA через `gh api repos/<owner>/<repo>/git/ref/tags/<tag>` и пишет версию в комментарии).
- README: раздел «Установка» (скачать установщик из выпусков или артефакта, SmartScreen до подписи — «Подробнее → Выполнить в любом случае»), «Удаление», «Сборка» (`scripts\build-installer.ps1`), «Проверка на GitHub Actions».
- Метка выпуска в этой части не создаётся: задача выпуска проверяется в части 4 (первый открытый выпуск).

Проверка задачи: ветка отправляется на GitHub (разрешено владельцем), запуск рабочего процесса зелёный (`gh run watch`), артефакт есть. Красный запуск — исправить и отправить снова; правки только в сборке и проверке, а если упала сама установка — в коде продукта с тестом.

- [ ] Шаги: реализация → `gh workflow`/`gh run` до зелёного → коммит(ы) `ci: build, install and uninstall the installer on GitHub Actions`.

### Task 8: Живая проверка на ПК владельца (нужен пользователь: UAC, окно)

Спецификация, раздел 7 (живая проверка). Исполняет основная сессия, не подчинённый агент.

Исходное состояние: продукт этапа 1 установлен (`%ProgramFiles%\SplitLane\splitlane-helper.exe`, задача «SplitLane Helper»), окно из `app\release\win-unpacked` (сборка плана 2) с автозапуском.

1. Сборка: `scripts\build-installer.ps1` → `BUILD-OK`. Сторож и набор `spike\plan1\bin` пересобираются из ветки (новое действие `kill-worker`); в `Send-P1Guard` (`spike\plan1\p1lib.ps1`, не в git) — `kill-worker` в списке действий.
2. `Start-P1Guard -NoHelper` при выключенном делении (UAC — пользователь), `Assert-P1Armed`; затем пользователь включает деление в окне этапа 1 (основная дорожка — VPN).
3. Пользователь запускает установщик (UAC — пользователь), мастер закрывает окно, «Установить», «Готово» с запуском окна. Проверка (сессия, одним вызовом с ожиданием): системный exit node вернулся на время установки (журнал `helper.log` старого и нового процессов), раскладка `%ProgramFiles%\SplitLane\` (корень без шести файлов этапа 1, `helper\`), команда задачи, новое окно подключилось, деление снова включено (`was on`), запись автозапуска указывает на установленный `SplitLane.exe`, `Test-P1SystemRestored` не нужен — деление включено намеренно.
4. Экран «Подключение»: версии `0.2.0`. Сессия запускает установленный `SplitLane.exe --remote-debugging-port=9222`: процесс выходит с кодом 1, окна нет, порт 9222 никто не слушает.
5. Пользователь закрывает окно из меню значка («Выход»), деление остаётся включённым. `Send-P1Guard -Action kill-worker`; в том же вызове ожидание до 90 секунд: в `helper.log` — `supervisor: ... starting it again`, новая рабочая копия, деление снова `on`.
6. Пользователь запускает окно из меню «Пуск», выключает деление; `Test-P1SystemRestored`.
7. Пользователь удаляет SplitLane из «Приложений» без галочки: задача и файлы удалены, `%ProgramData%\SplitLane` на месте, exit node на месте. Затем снова ставит установщиком: вход в Tailscale сохранился (окно сразу на доске).
8. `Send-P1Guard -Action finish` (служебный процесс продукта к этому моменту может работать: если сторож требует его отсутствия, пользователь сначала выходит из окна и служебный процесс завершается сам через 60 секунд).
Состояние служебного процесса сессия видит без канала (канал её не пускает): процессы `splitlane-helper.exe` и `sing-box.exe` с временем запуска, адаптер `SplitLane`, системный exit node (`Get-P1SystemExitNode`), журнал — из архива диагностики, который пользователь сохраняет из окна в `%ProgramData%\SplitLane\export\`.

Необязательный шаг (по согласию пользователя): выход из Windows или перезагрузка при включённом делении — после входа exit node на месте, надзиратель и рабочая копия запущены задачей, деление снова включено.

9. Удаление с галочкой на ПК владельца не проверяется (удалило бы вход встроенного узла, устройство осталось бы в сети Tailscale) — его проверяет задача 7.
10. Итоги — новое «Приложение A: проверки» в спецификации; коммит `docs: record the live check of stage 2 part 1`.

## Что остаётся после части 1

Части 2–5 этапа 2 (спецификация, раздел 1): обновления через GitHub Releases, английский интерфейс, открытый бета-выпуск (с первой меткой `v*` и проверкой задачи выпуска), заявка в SignPath Foundation.

Отложенные мелочи из рецензий (не мешают работе, решить до открытого выпуска или по ходу):
- деинсталлятор удаляет копию установщика в `%LocalAppData%\splitlane-app-updater` через путь, который пользователь может сделать соединением каталогов: пропускать папку-ссылку (часть 2 всё равно решает судьбу этой копии);
- тихая проверка «служебный процесс не запущен» при удалении без работающего служебного процесса считает «не запущен», если не стартовал сам `cmd.exe`;
- нет теста, что строка `UNINSTALL-REFUSED` при ошибке удаления папки окна не содержит путь пользователя; нет теста запуска рабочей копии настоящим процессом;
- английский текст причины отказа под русской подсказкой в окне (только на устаревшей странице) — вместе с частью 3;
- в истории git все коммиты подписаны настоящим именем и личной почтой владельца — решить до открытия репозитория (часть 4).
