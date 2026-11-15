# SplitLane, этап 2, часть 2: обновления — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Окно SplitLane само узнаёт о новой версии на GitHub Releases, по одному нажатию скачивает установщик, сверяет SHA-256 и ставит его с UAC; после перезапуска окно сообщает итог.

**Architecture:** Чистые функции (разбор выпуска, версии, файлы `.sha256` и `last-install.txt`, итог после перезапуска, допуск адресов) — в `app/src/shared/update.ts`. Механизм в основном процессе (`app/src/main/updater.ts`) — без импорта `electron`: сеть, файлы, таймеры и запуск установщика передаются ему зависимостями; `index.ts` подключает `net` Electron. Состояние обновления едет в `Snapshot`; страница показывает плашку и строку на экране «Подключение». Установщик пишет файл итога в `%ProgramData%\SplitLane\export\`.

**Tech Stack:** Electron 45.2.1 (`net`), Node.js 24 (`node:crypto`, `node:fs`, `node:http` — в тестах), React 19, TypeScript 6, Vitest 5, NSIS (electron-builder 26.18.0), PowerShell 5.1.

**Spec:** `docs/superpowers/specs/2026-11-15-splitlane-stage2-updates-design.md` (далее «спецификация»); рядом — спецификация установщика `docs/superpowers/specs/2026-11-15-splitlane-stage2-installer-design.md`.

## Global Constraints

- Работа идёт в ветке `stage2-updates`. Каждая задача с кодом заканчивается коммитом; сообщение — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Текст плана — по-русски; код, команды, идентификаторы — по-английски.
- Новых пакетов npm и модулей Go нет.
- Репозиторий выпусков — `splitlane-app/splitlane`; API — `https://api.github.com/repos/splitlane-app/splitlane/releases/latest`; файлы — `SplitLane-Setup-<версия>.exe` и `SplitLane-Setup-<версия>.exe.sha256` по адресу `https://github.com/splitlane-app/splitlane/releases/download/v<версия>/<имя>`; страница «Что нового» — `https://github.com/splitlane-app/splitlane/releases/tag/v<версия>`. Версия — три числа `x.y.z`, метка — `v<версия>`.
- Узлы, разрешённые для запросов и переадресаций: `api.github.com`, `github.com`, `objects.githubusercontent.com`, `release-assets.githubusercontent.com`; только `https`, порт по умолчанию. Источник для проверок `SPLITLANE_UPDATE_FEED` — только `http://127.0.0.1:<порт>` (без пути, без пользователя); тогда разрешён только этот источник.
- Пределы: ответ API ≤ 1 МБ, файл `.sha256` ≤ 1 КБ, установщик ≤ 400 МБ; 60 секунд без данных — обрыв; заголовок `User-Agent: SplitLane/<версия окна>`.
- Расписание: первая автоматическая проверка через 30 секунд после старта, затем каждые 24 часа; одновременно одна проверка или одна загрузка.
- Папка загрузки — `<TEMP>\SplitLane-update\`; перед загрузкой очищается; файл пишется как `<имя>.part` и переименовывается после сверки.
- Запуск установщика: `Start-Process -FilePath <exe> -ArgumentList '/S','--force-run' -Verb RunAs -ErrorAction Stop` без ожидания, через `powershell.exe` с `-EncodedCommand` (`encodePowerShell` из `app/src/main/setup.ts`); отмена UAC — код 1223.
- Файл итога установщика — `%ProgramData%\SplitLane\export\last-install.txt`: строка 1 — версия, строка 2 — `ok` или `refused`, строка 3 — причина (пустая для `ok`); CRLF; UTF-8 без BOM. Пишет его служебный процесс — `prepare` и `install`, которые запускает установщик (спецификация, раздел 6); сам установщик его не пишет.
- Тексты страницы — только в `app/src/renderer/i18n/ru.ts`, тексты основного процесса (меню значка) — в `MAIN_TEXT` (`app/src/main/tray.ts`); точные тексты — в задачах.
- Модульные тесты не трогают систему: нет настоящего канала, `schtasks`, UAC, PowerShell, реестра, `%ProgramData%`, сети вне `127.0.0.1`. Разрешено: временные папки, HTTP-сервер на `127.0.0.1` в том же тесте.
- `pnpm dev` / `electron-vite dev` не запускаются никогда. Установщик, деинсталлятор и `SplitLane.exe` в задачах 1–4 не запускаются; `scripts\test-installer.ps1` на этом ПК — только до отказа.
- Личные данные: в плане, тестах, отчётах и коммитах нет имён устройств и сети Tailscale, имени ПК, настоящих адресов, SID и имени учётной записи владельца.
- Файлы PowerShell — только ASCII; `app/build/installer.nsh` — UTF-8 с BOM.
- Полная проверка окна (из `C:\code\router\app`): `pnpm typecheck; pnpm test; pnpm build` без ошибок; установщика — `scripts\build-installer.ps1` → `BUILD-OK`; CI — зелёный запуск.

## Review Focus

Условия, которые спецификация подразумевает, но прямо не описывает; от более вероятного к менее вероятному. Каждое закреплено тестом в задаче, которая владеет кодом.

1. GitHub отвечает 403 (предел запросов) или сети нет во время автоматической проверки. Ожидание: плашки нет, строка в журнале окна, следующая проверка — по расписанию; ручная проверка показывает «Не удалось проверить обновления: <причина>». Задача 2: `automatic check failure shows nothing`, `manual check failure shows the reason`.
2. Переадресация на чужой узел или на `http` во время загрузки. Ожидание: загрузка прерывается до запроса к чужому узлу, файл удалён, «Не удалось скачать версию X: …». Задача 2: `refuses a redirect to another host`, `refuses a redirect to http`.
3. После перезапуска окна в `last-install.txt` лежит итог другой (старой) установки. Ожидание: окно не выдаёт чужую причину — итог сопоставляется по версии. Задача 1: `outcomeAfterRestart ignores a result of another version`.
4. Пользователь жмёт «Обновить» дважды или «Проверить обновления» во время загрузки. Ожидание: идёт одна операция, вторая игнорируется. Задача 2: `a second update while downloading is ignored`, `a check during a download is ignored`.
5. Ошибка записи во временную папку (диск полон, файл занят антивирусом). Ожидание: «Не удалось скачать версию X: <причина>», установщик не запускается, частичный файл удалён, если возможно. Задача 2: `a write error fails the download`.

---

## Как читать этот план

**Что уже есть.** Установщик части 1 (`app/build/installer.nsh`, `scripts\build-installer.ps1`) с тихим режимом и `--force-run` (шаблон electron-builder запускает окно после тихой установки помощника, если передан `--force-run`); `prepare` сохраняет признак «было включено»; `canInstall` в `app/src/main/setup.ts` говорит, установлено ли окно; `encodePowerShell` там же. Окно: `Snapshot` (`app/src/shared/ipc.ts`) рассылается странице при каждом изменении; `registerIpc` (`app/src/main/ipc.ts`) проверяет каждый аргумент страницы; настройки окна — `app/src/shared/settings.ts` + `app/src/main/settingsFile.ts`; меню значка — `app/src/main/tray.ts`; плашки состояния — `app/src/renderer/model/banners.ts` и `components/Banner.tsx`; экран «Подключение» — `ConnectionScreen` в `app/src/renderer/screens/ConnectScreen.tsx`.

**Код в плане.** Для каждой задачи план задаёт обязательное: файлы, интерфейсы (точные имена и типы), поведение, тесты (имена и что проверяют) и команды проверки. Подробности, которых нет в плане, исполнитель берёт из кода рядом и из спецификации. Задача 5 — живая проверка на ПК владельца.

**Порядок задач.** 1 — чистые функции; 2 — механизм в основном процессе; 3 — окно (мост, плашка, экран, меню, настройка); 4 — установщик, сборка, CI; 5 — живая проверка.

**Поправка после задачи 4.** `last-install.txt` пишет служебный процесс (`prepare` и `install`, которые запускает установщик), а не макрос установщика; где ниже сказано, что файл итога пишет установщик, это первоначальный замысел.

### Карта файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `app/src/shared/update.ts` | типы состояния, версии, разбор выпуска и файлов, допуск адресов, итог после перезапуска | 1 |
| `app/src/main/updater.ts` | проверка, загрузка, сверка, запуск установщика, расписание | 2 |
| `app/src/shared/settings.ts`, `app/src/main/settingsFile.ts`, `app/src/main/ipc.ts` | настройки `checkUpdates`, `pendingUpdate` | 2, 3 |
| `app/src/main/index.ts`, `app/src/shared/ipc.ts`, `app/src/preload/index.ts`, `app/src/main/tray.ts` | подключение механизма, мост, меню значка | 3 |
| `app/src/renderer/model/update.ts`, `app/src/renderer/components/UpdateBanner.tsx`, `ConnectScreen.tsx`, `i18n/ru.ts` | плашка и строка обновлений | 3 |
| `app/build/installer.nsh` | файл итога | 4 |
| `scripts/build-installer.ps1`, `scripts/build-helper.ps1` | параметр `-Version` | 4 |
| `scripts/test-installer.ps1` | проверка файла итога | 4 |

### Где код задач

| Задача | Коммиты |
|---|---|
| 1 | f98b140, 2564af6 |
| 2 | 305e727, 0df1fa9 |
| 3 | 8e661dd, 21cabc3 |
| 4 | d858b29, 77e9107, 7420b46 |
| итоговая рецензия ветки | ed51143, ee1366f, 296e012 |
| живая проверка: отмена UAC в PowerShell 5.1 | 82d080a |

---

### Task 1: Чистые функции обновлений

Спецификация, разделы 2, 4, 5, 6.

**Files:**
- Create: `app/src/shared/update.ts`, `app/src/shared/update.test.ts`

**Interfaces (produces):**

```ts
export type Version = readonly [number, number, number]
export function parseVersion(s: string): Version | null          // ровно x.y.z, числа без знака и без ведущих нулей, кроме "0"
export function compareVersions(a: Version, b: Version): number   // <0, 0, >0

export interface Feed {
  api: string            // полный адрес releases/latest
  assetPrefix: string    // ожидаемое начало адресов файлов: <origin>/splitlane-app/splitlane/releases/download/
  origin: 'github' | 'test'
  testOrigin?: string    // http://127.0.0.1:<порт>, только для 'test'
}
export function feedFrom(env: string | undefined): { feed: Feed; refused?: string } // refused — текст для журнала, если env задан неверно

export type ReleasePick =
  | { kind: 'newer'; version: string; installerUrl: string; shaUrl: string; notesUrl: string; name: string }
  | { kind: 'notNewer'; version: string }
  | { kind: 'invalid'; reason: string }
export function pickRelease(json: unknown, current: string, feed: Feed): ReleasePick

export function allowedHop(url: string, feed: Feed): boolean       // github: https + разрешённые узлы + порт по умолчанию; test: только testOrigin
export function parseSha256File(text: string, fileName: string): string | null // 64 hex (строчные на выходе) или null
export interface LastInstall { version: string; result: 'ok' | 'refused'; reason: string }
export function parseLastInstall(text: string): LastInstall | null

export type UpdateState =
  | { kind: 'idle' }
  | { kind: 'checking'; manual: boolean }
  | { kind: 'upToDate' }                                    // только после ручной проверки
  | { kind: 'checkFailed'; reason: string }                 // только после ручной проверки
  | { kind: 'available'; version: string; notesUrl: string; hidden: boolean }
  | { kind: 'downloading'; version: string; notesUrl: string; percent: number }
  | { kind: 'verifying'; version: string; notesUrl: string }
  | { kind: 'launching'; version: string; notesUrl: string }
  | { kind: 'cancelled'; version: string; notesUrl: string }
  | { kind: 'downloadFailed'; version: string; notesUrl: string; reason: string }
  | { kind: 'installed'; version: string }
  | { kind: 'installRefused'; version: string; notesUrl: string; reason: string } // reason может быть ''

export function notesUrlFor(version: string): string // https://github.com/splitlane-app/splitlane/releases/tag/v<версия>
export function outcomeAfterRestart(pending: string | null, appVersion: string, last: LastInstall | null): UpdateState | null
```

**Поведение:**
- `feedFrom(undefined или '')` → GitHub. `feedFrom('http://127.0.0.1:8123')` (и с завершающей `/`) → test: `api` = `http://127.0.0.1:8123/repos/splitlane-app/splitlane/releases/latest`, `assetPrefix` = `http://127.0.0.1:8123/splitlane-app/splitlane/releases/download/`. Любое другое значение (`https://…`, `http://localhost:…`, другой адрес, путь, пользователь, без порта) → GitHub и `refused`.
- `pickRelease`: `json` — объект с `tag_name`, `draft`, `prerelease`, `assets[] { name, browser_download_url, size }`. Черновик или предварительный → `invalid`. Метка не `v<x.y.z>` → `invalid`. Версия ≤ текущей → `notNewer`. Нет ровно одного файла с каждым из двух имён, адрес файла не равен `assetPrefix + 'v<версия>/' + имя`, или `size` установщика > 400 МБ → `invalid` с причиной. Лишние файлы выпуска допускаются. `current` не разбирается → `invalid`.
- `allowedHop`: разбор через `URL`; github: `protocol === 'https:'`, `hostname` из списка (без учёта регистра — `URL` уже приводит), `port === ''`, нет `username`/`password`; test: `origin === testOrigin`.
- `parseSha256File`: первая непустая строка — `<64 hex><один или два пробела или табуляция><имя>`, имя совпадает точно; иначе `null`.
- `parseLastInstall`: CRLF или LF; строка 1 — версия `x.y.z`, строка 2 — `ok` или `refused`, строка 3 — причина (может отсутствовать — тогда `''`); иначе `null`.
- `outcomeAfterRestart`: `pending === null` → `null`. `pending === appVersion` → `installed`. Иначе `last` с `version === pending` и `refused` → `installRefused` с причиной; иначе `installRefused` с `reason: ''`.

Тесты (`update.test.ts`): `parseVersion accepts x.y.z only`, `compareVersions orders`, `feedFrom github by default`, `feedFrom accepts only http 127.0.0.1 with a port`, `pickRelease newer`, `pickRelease not newer`, `pickRelease refuses a draft and a prerelease`, `pickRelease refuses a bad tag`, `pickRelease refuses missing or duplicate assets`, `pickRelease refuses a foreign asset url`, `pickRelease refuses an oversized installer`, `allowedHop github hosts`, `allowedHop refuses http, other hosts, ports, credentials`, `allowedHop test origin only`, `parseSha256File`, `parseLastInstall`, `outcomeAfterRestart installed`, `outcomeAfterRestart refused with reason`, `outcomeAfterRestart ignores a result of another version`, `outcomeAfterRestart without pending is null`.

- [ ] Шаги: тесты → провал → реализация → проходят → `pnpm typecheck; pnpm test; pnpm build` → коммит `feat(app): update model: versions, releases, checksums, install results`.

### Task 2: Механизм обновлений в основном процессе

Спецификация, разделы 2, 3, 5, 6.

**Files:**
- Create: `app/src/main/updater.ts`, `app/src/main/updater.test.ts`
- Modify: `app/src/shared/settings.ts` (+ тест), `app/src/main/settingsFile.ts` (если нужно для новых полей)

**Interfaces:**
- Consumes (задача 1): всё из `app/src/shared/update.ts`; `encodePowerShell` из `app/src/main/setup.ts`.
- `WindowSettings` получает `checkUpdates: boolean` (по умолчанию `true`) и `pendingUpdate: string | null` (по умолчанию `null`; только версия `x.y.z` или `null`, иначе `null` при разборе).
- Produces:

```ts
export interface HopResponse { status: number; location: string | null; body: AsyncIterable<Uint8Array> | null; contentLength: number | null }
export interface UpdaterDeps {
  appVersion: string
  feed: Feed
  canUpdate: boolean                         // canInstall части 1, или test-источник в разработке
  hop(url: string, signal: AbortSignal): Promise<HopResponse>  // один запрос БЕЗ следования переадресациям
  tempDir: string                             // <TEMP>\SplitLane-update
  fs: { rmDir(p: string): Promise<void>; mkdir(p: string): Promise<void>; open(p: string): Promise<{ write(b: Uint8Array): Promise<void>; close(): Promise<void> }>; rename(a: string, b: string): Promise<void>; unlink(p: string): Promise<void>; readText(p: string): Promise<string | null> }
  runInstaller(exe: string): Promise<'started' | 'cancelled' | { error: string }>
  settings: { get(): WindowSettings; set(patch: Partial<WindowSettings>): Promise<void> }
  lastInstallFile: string                     // %ProgramData%\SplitLane\export\last-install.txt
  clock: { now(): number; setTimeout(fn: () => void, ms: number): unknown; clearTimeout(h: unknown): void }
  log(line: string): void
  changed(state: UpdateState): void
}
export class Updater {
  constructor(d: UpdaterDeps)
  start(): Promise<void>        // итог после перезапуска (outcomeAfterRestart; pendingUpdate снимается), затем расписание: 30 s, 24 h
  stop(): void
  state(): UpdateState
  check(manual: boolean): Promise<void>
  update(): Promise<void>       // из available/cancelled/downloadFailed/installRefused
  cancel(): void                // во время downloading
  later(): void                 // available → hidden: true
  dismiss(): void               // installed / upToDate / checkFailed → idle
}
export function runInstallerScript(exe: string): string // текст PowerShell: Start-Process … -Verb RunAs, отмена → exit 1223, успех → exit 0
```

**Поведение:**
- Запрос (проверка и загрузки): `hop` по шагам; на 301/302/303/307/308 — следующий адрес из `location` (относительный — от текущего), каждый шаг проверяется `allowedHop` ДО запроса; не больше 5 шагов; иначе ошибка `redirect to a host that is not allowed` / `too many redirects`. Код ≠ 200 — ошибка `HTTP <код>` (для 403 — `HTTP 403 (GitHub request limit?)`). Пределы размера — по Global Constraints, по факту прочитанных байтов (и по `contentLength`, если он больше). Обрыв по 60 секундам без данных — через `AbortSignal` и `clock`.
- `check(manual)`: если идёт проверка или загрузка — ничего. `canUpdate === false` — ничего (ручная: `checkFailed` «обновления доступны только у установленного SplitLane»). Ответ API → JSON → `pickRelease`. `newer` → `available` (hidden: false). `notNewer` → ручная: `upToDate`, автоматическая: прежнее состояние, если оно не `checking`, иначе `idle`. `invalid` или ошибка → ручная: `checkFailed` с причиной; автоматическая: строка журнала, состояние `idle` (или прежнее `available`).
- `update()`: только из состояний с версией (см. интерфейс); иначе ничего. Очистка `tempDir`, `mkdir`; загрузка `.sha256` (≤ 1 КБ) → `parseSha256File`; загрузка установщика в `<tempDir>\<имя>.part` с подсчётом SHA-256 (`node:crypto`) и процентами (по `size` выпуска; `changed` не чаще раза в 250 мс и при каждом новом целом проценте); `verifying`; несовпадение хеша → `downloadFailed` «файл не совпал с контрольной суммой»; `rename` в `<имя>`; `settings.set({ pendingUpdate: version })`; `launching`; `runInstaller`: `cancelled` → `settings.set({ pendingUpdate: null })` и `cancelled`; `{error}` → то же и `downloadFailed` с текстом; `started` → состояние `launching` остаётся (установщик закроет окно). Любая ошибка записи/сети → `downloadFailed`, `.part` удаляется (ошибка удаления — только журнал).
- `cancel()`: прерывает загрузку → `available` (hidden: false), файл удаляется.
- Расписание: только при `settings.get().checkUpdates`; ручная проверка — всегда. `stop()` снимает таймеры и прерывает операцию.
- `start()`: `outcomeAfterRestart(settings.pendingUpdate, appVersion, parseLastInstall(readText(lastInstallFile)))`; не `null` → это состояние и `pendingUpdate: null`.

Тесты (`updater.test.ts`; HTTP-сервер `node:http` на `127.0.0.1` с test-источником, `hop` — через `fetch` Node с `redirect: 'manual'`; подменные часы и `runInstaller`): `finds a newer release`, `manual check without a newer release says up to date`, `automatic check failure shows nothing`, `manual check failure shows the reason`, `downloads, verifies and starts the installer`, `a checksum mismatch fails and removes the file`, `an oversized download fails`, `a stalled download times out`, `refuses a redirect to another host`, `refuses a redirect to http` (github-источник с подменным `hop`), `follows an allowed redirect`, `cancel stops the download`, `a second update while downloading is ignored`, `a check during a download is ignored`, `a write error fails the download`, `cancelled UAC clears pendingUpdate`, `start reports installed and clears pendingUpdate`, `start reports a refusal with its reason`, `schedule: first check after 30 s, then every 24 h, none when checkUpdates is off`, `runInstallerScript quotes the path and maps a cancelled UAC to 1223`. `settings.test.ts` (или рядом): `parseSettings reads checkUpdates and pendingUpdate with defaults`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка окна → коммит `feat(app): the updater checks GitHub Releases, downloads, verifies and starts the installer`.

### Task 3: Окно: мост, плашка, экран «Подключение», меню значка

Спецификация, разделы 3, 4.

**Files:**
- Modify: `app/src/shared/ipc.ts`, `app/src/main/ipc.ts` (+ тест), `app/src/preload/index.ts`, `app/src/main/index.ts`, `app/src/main/tray.ts`, `app/src/renderer/i18n/ru.ts`, `app/src/renderer/screens/ConnectScreen.tsx` (+ тест), экран доски (где показываются плашки состояния)
- Create: `app/src/renderer/model/update.ts` (+ тест), `app/src/renderer/components/UpdateBanner.tsx` (+ тест, + `.module.css`)

**Interfaces:**
- Consumes: `UpdateState`, `Feed`, `feedFrom` (задача 1); `Updater`, `UpdaterDeps` (задача 2); `canInstall` (часть 1).
- `Snapshot` получает `update: UpdateState`.
- Мост `window.splitlane`: `checkUpdates(): Promise<void>`, `installUpdate(): Promise<void>`, `cancelUpdate(): Promise<void>`, `updateLater(): Promise<void>`, `dismissUpdate(): Promise<void>`, `openUpdateNotes(): Promise<boolean>` (открывает `notesUrl` текущего состояния; адрес проверяется ещё раз: ровно `https://github.com/splitlane-app/splitlane/releases/tag/v<x.y.z>`). Каналы `IPC.checkUpdates` и т. д. Аргументов от страницы нет.
- `validateSettingsPatch` принимает `checkUpdates` (boolean); `pendingUpdate` от страницы не принимается.
- `export function updateBannerModel(s: UpdateState): { text: string; actions: UpdateAction[]; tone: 'info' | 'warn' } | null` (`renderer/model/update.ts`), `type UpdateAction = 'install' | 'notes' | 'later' | 'cancel' | 'retry' | 'dismiss'`; `export function updateStatusLine(s: UpdateState): string | null` — для экрана «Подключение».

**Поведение:**
- `index.ts`: `Updater` с `hop` на `net.request` Electron (`redirect: 'manual'`; событие `redirect` даёт `statusCode` и `redirectUrl` — шаг завершается без следования; ответ — поток данных; `User-Agent`), `feed = feedFrom(process.env.SPLITLANE_UPDATE_FEED)` (`refused` — в журнал окна), `canUpdate = canInstall(...) || (!app.isPackaged && feed.origin === 'test')`, `tempDir = join(app.getPath('temp'), 'SplitLane-update')`, `lastInstallFile = join(process.env.ProgramData ?? 'C:\\ProgramData', 'SplitLane', 'export', 'last-install.txt')`, `runInstaller` — `powershell.exe` с `encodePowerShell(runInstallerScript(exe))`, код 1223 → `cancelled`, 0 → `started`, иное — `{ error }`. `updater.start()` после готовности окна; `changed` → новый снимок.
- Меню значка: пункт `MAIN_TEXT.checkUpdates` = «Проверить обновления» (перед «Выход»): ручная проверка и открыть окно на экране «Подключение».
- Плашка (`UpdateBanner`) — над доской, под плашкой состояния деления; тексты (ключи `update.*` в `ru.ts`):
  - available (не hidden): «Доступна версия {v}» — «Обновить», «Что нового», «Позже»;
  - downloading: «Загрузка версии {v}: {p}%» — «Отменить»;
  - verifying: «Проверка версии {v}…»;
  - launching: «Установка версии {v}: Windows попросит подтверждение»;
  - cancelled: «Обновление отменено» — «Обновить»;
  - downloadFailed: «Не удалось скачать версию {v}: {reason}» — «Повторить»;
  - installed: «SplitLane обновлён до {v}» — «Закрыть»;
  - installRefused: «Обновление до {v} не установлено: {reason}» (пустая причина — «Обновление до {v} не установлено») — «Повторить», «Что нового»;
  - idle, checking, upToDate, checkFailed, available hidden — плашки нет.
- Экран «Подключение»: под строкой версий — строка `updateStatusLine` («Проверка обновлений…», «Установлена последняя версия», «Не удалось проверить обновления: {reason}», «Доступна версия {v}», для прочих — как плашка без кнопок) и кнопка «Проверить обновления» (недоступна во время проверки и загрузки); переключатель «Проверять обновления» рядом с «Запускать при входе в Windows».
- Причины ошибок, которые приходят из механизма (английский текст ошибки), показываются как есть (английский интерфейс — часть 3).

Тесты: `updateBannerModel for every state` (таблица), `updateStatusLine for every state`, `UpdateBanner buttons call the bridge`, `ConnectionScreen shows the update line and the check button`, `the check button is disabled while checking or downloading`, `the checkUpdates toggle saves the setting`, `validateSettingsPatch accepts checkUpdates and refuses pendingUpdate`, `openUpdateNotes opens only the release page of the current state`.

- [ ] Шаги: тесты → провал → реализация → полная проверка окна → коммит `feat(app): update banner, connection screen line, tray item and setting`.

### Task 4: Установщик пишет итог; версия сборки; CI

Спецификация, разделы 6, 7.

**Files:**
- Modify: `app/build/installer.nsh`, `scripts/build-installer.ps1`, `scripts/build-helper.ps1`, `scripts/test-installer.ps1`, `README.md` (раздел «Сборка»: параметр `-Version`; раздел про обновления: как окно обновляется и что пока репозиторий закрыт, обновления не находятся)

**Поведение:**
- `installer.nsh`: макрос `splitlaneWriteResult RESULT REASON` (только в сборке установщика): если есть папка `<ProgramData>\SplitLane\export` (`SetShellVarContext all` + `$APPDATA`, или переменная окружения `ProgramData` — выбрать то, что даёт настоящую `C:\ProgramData` в установщике с повышением; папку не создавать), записать `last-install.txt` целиком (строки: `${VERSION}`, RESULT, REASON; CRLF; без BOM): после успешного `install` в `customInstall` — `ok` и пустая причина; при отказе `prepare` и при отказе `install` — `refused` и причина без префикса (`splitlaneReason`). Ошибка записи не меняет исход установки.
- `build-helper.ps1 [-Version <x.y.z>]` и `build-installer.ps1 [-Version <x.y.z>]`: без параметра — версия из `app\package.json`; с параметром — та же проверка `^\d+\.\d+\.\d+$`, служебный процесс собирается с ней, electron-builder получает `-c.extraMetadata.version=<v>`; имя установщика и `.sha256` — с этой версией; `app\package.json` не меняется. Последняя строка — как сейчас.
- `test-installer.ps1`: после каждой успешной установки — `last-install.txt` существует, строка 1 — версия установщика, строка 2 — `ok`, строка 3 пустая.

Проверка задачи: `scripts\build-installer.ps1` и `scripts\build-installer.ps1 -Version 0.2.90` → `BUILD-OK` (во втором случае — `SplitLane-Setup-0.2.90.exe`, у `SplitLane.exe` — `ProductVersion` 0.2.90; `app\package.json` не изменён); отправка ветки и зелёный запуск CI.

- [ ] Шаги: реализация → обе сборки → отправка → CI зелёный → коммит(ы) `feat(app): the installer records its result; build version parameter`.

### Task 5: Живая проверка на ПК владельца (нужен пользователь: UAC)

Спецификация, раздел 8. Исполняет основная сессия, не подчинённый агент.

Исходное состояние: установлен SplitLane 0.2.0 (часть 1), деление выключено, окно закрыто.

1. Сборки: `scripts\build-installer.ps1 -Version 0.2.90` (A) и `-Version 0.2.91` (B); файлы B и его `.sha256` — в отдельную папку для локального источника.
2. Локальный источник: маленький HTTP-сервер на `127.0.0.1:<порт>` (скрипт Node в `spike\` или во временной папке, вне git), отдаёт `GET /repos/splitlane-app/splitlane/releases/latest` (JSON выпуска `v0.2.91` с двумя файлами) и `GET /splitlane-app/splitlane/releases/download/v0.2.91/<имя>`.
3. `Start-P1Guard -NoHelper` (UAC — пользователь), `Assert-P1Armed`.
4. Пользователь ставит A установщиком (UAC) поверх 0.2.0; окно A открывается. Сессия закрывает окно и запускает установленное окно A с `SPLITLANE_UPDATE_FEED=http://127.0.0.1:<порт>` (в окружении процесса). Пользователь включает деление.
5. Пользователь открывает «Подключение» → «Проверить обновления» → плашка «Доступна версия 0.2.91» → «Обновить» → загрузка → UAC (пользователь) → окно закрывается, ставится B, окно B запускается; проверка сессии одним вызовом с ожиданием: exit node вернулся на время установки, деление снова включено, `last-install.txt` = `0.2.91`/`ok`, у окна версия 0.2.91 и плашка «SplitLane обновлён до 0.2.91» (пользователь подтверждает).
6. Отмена UAC: собрать C (`-Version 0.2.92`; переименовать B нельзя — у выпуска свои версия и хеш), источник отдаёт `v0.2.92`; пользователь жмёт «Обновить» и отменяет UAC → «Обновление отменено», версия 0.2.91 на месте.
7. `Send-P1Guard -Action finish` (после выхода из окна и остановки служебного процесса), итоги — в «Приложение A: проверки» спецификации; коммит `docs: record the live check of stage 2 part 2`.

## Что остаётся после части 2

Часть 3 — английский интерфейс; часть 4 — открытый бета-выпуск (обновления начнут находиться, когда репозиторий станет открытым и выйдет первый опубликованный выпуск с версией больше 0.2.91); часть 5 — подпись и проверка подписи установщика перед запуском.
