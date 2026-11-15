# SplitLane, план 2: окно — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Окно SplitLane для пользователя без прав администратора: первая настройка служебного процесса из окна (UAC), вход в Tailscale и выбор exit node, доска с двумя дорожками и перетаскиванием программ, живая активность, плашки состояний, трей и автозапуск — так, чтобы этап 1 работал без консоли и ручной правки файлов.

**Architecture:** Electron в папке `app/`. Основной процесс держит канал к служебному процессу (`\\.\pipe\SplitLane.Helper`, JSON по строке), управляет его установкой и перезапуском, треем, настройками окна, списком программ и значками; всё, что зависит от системы, — за маленькими интерфейсами, логика — в чистых модулях без `electron`. Предзагрузочный скрипт даёт странице узкий мост `window.splitlane`. Страница (React 19, zustand) строит доску из состояния служебного процесса и сводки активности чистыми функциями модели и отправляет намерения (`lanes.set`, `routing.enable` …) через мост, показывая изменение сразу и откатывая его при ошибке.

**Tech Stack:** Electron 45.2.1, electron-vite 6.0.0 (Vite 8.3.5), React 19.3.1, TypeScript 6.0.3, zustand 5.0.16, @dnd-kit/react 0.5.3, Vitest 5.0.5 (jsdom 30.1.3, @testing-library/react 16.3.3), electron-builder 26.18.0, pnpm 12.9.1, Node.js 24.

**Spec:** `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` — разделы 1, 4.1–4.3, 5.1, 5.2, 7, 8.1–8.4, 9, 10, 11, 12, 13. Контракт канала — по коду служебного процесса (`helper/internal/protocol`, `helper/internal/pipe`), выжимка — `.superpowers/sdd/2026-11-14-splitlane-plan-2-window/helper-protocol.md`; версии пакетов и API — `.superpowers/sdd/2026-11-14-splitlane-plan-2-window/stack.md`. План 1B (`docs/superpowers/plans/2026-11-14-splitlane-plan-1b-helper.md`, раздел «Что остаётся плану 2») — объём.

## Global Constraints

- Пакеты и версии — ровно эти, закреплены точно (без `^` и `~`), других пакетов нет: `electron` 45.2.1, `electron-vite` 6.0.0, `vite` 8.3.5, `@vitejs/plugin-react` 6.1.2, `react` и `react-dom` 19.3.1, `@types/react` и `@types/react-dom` 19.3.0, `typescript` 6.0.3, `zustand` 5.0.16, `@dnd-kit/react`, `@dnd-kit/dom`, `@dnd-kit/abstract`, `@dnd-kit/helpers` 0.5.3, `vitest` 5.0.5, `jsdom` 30.1.3, `@testing-library/react` 16.3.3, `@testing-library/dom` 10.4.2, `electron-builder` 26.18.0, `@types/node` 24.19.2. В `app/pnpm-workspace.yaml`: `allowBuilds: { electron: true, electron-winstaller: false }` (второе — сборочный скрипт установщика Squirrel из зависимостей electron-builder, не нужен), `minimumReleaseAge: 20160`.
- `app/package.json` без `"type": "module"`; `"main": "out/main/index.js"`. Предзагрузочный скрипт — один файл CJS, импортирует только `electron`.
- Окно: `contextIsolation: true`, `sandbox: true`, `nodeIntegration: false`; удалённое содержимое не загружается; CSP страницы `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`; навигация и новые окна запрещены. Окно не обращается к сети, кроме канала служебного процесса; `shell.openExternal` — только для ссылки входа `https://login.tailscale.com/...` (хост `login.tailscale.com`).
- Канал — по `helper-protocol.md`: первое сообщение `hello` с `protocol: 1`, после него `state.get`; окно читает канал непрерывно (очередь клиента у служебного процесса — 64 сообщения); строки любой длины; параметры строго по полям протокола (лишнее поле — `bad_request`). Окно не отправляет пути для запуска, конфиги и пути для записи (раздел 11 спецификации); `exe` в правилах — только путь, выбранный пользователем.
- Правила от окна: новый `id` — UUID v4 (`crypto.randomUUID`), `matcher.value` пустой, `name` — имя ярлыка или пустая строка; вид `package`, если путь проходит проверку `packageValue` служебного процесса (сегмент `WindowsApps` без учёта регистра, за ним папка из ровно пяти полей через `_` с непустыми первым и пятым, за ней ещё хотя бы один сегмент), иначе `folder`; плитка «Службы Windows» — вид `system`, `exe` и `value` пустые. Существующие правила уходят без изменений, кроме изменяемого поля. Пределы (байты UTF-8): правил не больше 500, `name` ≤ 256, `exe` и `value` ≤ 1024.
- Тексты: все строки интерфейса — в словаре `app/src/renderer/i18n/ru.ts`; в компонентах строк нет. Подписи: «Через VPN», «Напрямую», «Пустить через VPN», «Пустить напрямую», «Открепить», «основная», «сделать основной», «тихо». Тексты причин (18 кодов), кодов ошибок (9) и причин отказа правил (`too_broad`, `protected`, `duplicate`) — в словаре по коду.
- Оформление (9.7): фон `#0e1014`, панели `#14171d`, карточки `#181c24`, границы `#232733`, текст `#e6e8ee`, приглушённый `#8b93a7`, VPN `#8b7cff`, напрямую `#3ddc97`, предупреждение `#ffb454`, ошибка `#ff6b6b`; шрифт `"Segoe UI Variable", "Segoe UI", sans-serif`; числа — `"Cascadia Mono", Consolas, monospace`, `font-variant-numeric: tabular-nums`. Стили — CSS-модули и переменные; готовых наборов компонентов нет. Минимальный размер окна 860×560.
- Модульные тесты не трогают систему: нет настоящего канала продукта, `schtasks`, UAC, PowerShell, реестра, `%ProgramData%`. Логика основного процесса — в модулях без импорта `electron`; системное — за интерфейсами, в тестах подмены. Разрешено: временные папки, тестовый канал `\\.\pipe\SplitLane.WindowTest.<число>` с сервером на Node в том же тесте.
- Живой шаг — любой шаг, после которого системный exit node может оказаться выключен (включение деления из окна, выход из окна при включённом делении). Он идёт только под сторожем `slguard.exe` (`spike\plan1\p1lib.ps1`: `Start-P1Guard -NoHelper`, `Assert-P1Armed`, `Set-P1Heartbeat`, `Send-P1Guard`, `Test-P1SystemRestored`); между вызовами не больше 400 секунд. Запрос UAC принимает человек; отменённый запрос без согласия пользователя не повторяется. В живых шагах основная дорожка — VPN; в прямую дорожку попадают только копии `curl-*.exe` и `node-*.exe` из `spike\plan1\bin`.
- Личные данные: в плане, тестах, отчётах и коммитах нет имён устройств и сети Tailscale, имени ПК, настоящих адресов, ссылок для входа. Тестовые адреса — из `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`; адреса Tailscale — `100.64.0.x`; имена вымышленные.
- Файлы PowerShell содержат только ASCII. TypeScript — `strict`, без `any` в экспортируемых типах.
- Работа идёт в ветке `plan-2-window`. Каждая задача с кодом заканчивается коммитом; сообщение — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Текст плана — по-русски; код, команды, идентификаторы — по-английски.
- Команды `pnpm` выполняются из `C:\code\router\app`. Полная проверка задачи: `pnpm typecheck; pnpm test; pnpm build` — все три без ошибок.

**Ручное восстановление** — тот же текст, что в Global Constraints плана 1A; исполнитель показывает его пользователю до первого запроса UAC задачи 15.

## Review Focus

Условия, которые спецификация подразумевает, но прямо не описывает; от более вероятного к менее вероятному. Каждое закреплено тестом в задаче, которая владеет кодом.

1. Служебный процесс перезапускается (канал пропал), пока `lanes.set` в пути, а изменение уже показано на экране. Ожидание: вызов завершается ошибкой «канал закрыт», доска возвращается к раскладке из `state.get` после переподключения, правило не задваивается и не теряется. Задача 9: `rolls back a pending layout when the link drops`; задача 3: `fails pending calls when the pipe closes`.
2. Пользователь переносит несколько программ подряд быстрее, чем служебный процесс отвечает (`lanes.set` с сверкой занимает до 3 секунд). Ожидание: изменения уходят по одному в порядке действий, каждое поверх предыдущего; ошибка одного откатывает только его и следующие за ним, итог на экране совпадает с последним `state`. Задача 9: `queues rapid moves in order`, `a rejected move rolls back itself and the moves queued after it`.
3. Служебный процесс нормализует правило (вид `folder` → `file` для общей папки, пустое имя → описание файла), и в `state.apps` оно приходит не таким, каким его отправило окно. Ожидание: карточка не задваивается и не мигает — окно сопоставляет правило по `id` и берёт версию служебного процесса. Задача 9: `takes the helper's normalized rule by id`.
4. В сводке активности большое событие (подробности на 500 соединений), приходящее кусками, и многобайтовые символы (кириллица в путях), разрезанные границей куска. Ожидание: строка собирается целиком, символы не портятся. Задача 2: `joins a long line split into many chunks`, `keeps a multibyte character split across chunks`.
5. Из Проводника бросают не только `.exe`: ярлык на документ или папку, папку, `.bat`, сетевой путь. Ожидание: подходящие программы добавляются, остальные пропускаются с сообщением «Не программа: <имя>», в канал ничего лишнего не уходит. Задача 6: `resolves dropped files to programs and skips the rest`.

---

## Как читать этот план

**Что уже есть.** Служебный процесс (планы 1A и 1B) полностью работает; его протокол — в `helper-protocol.md`. `dist\helper\` (шесть файлов) собирает `scripts\build-helper.ps1`; окно берёт их оттуда в разработке и из `resources\helper\` в собранном приложении. Команда `install --user-sid <SID>` ставит продукт; задачу планировщика окно запускает само (`schtasks /run /tn "SplitLane Helper"`).

**Код в плане.** Задачи 1–14 — код с тестами, система не трогается. Для каждой задачи план задаёт обязательное: файлы, интерфейсы (точные имена и типы — по ним соседние задачи стыкуются), тесты (имена и проверяемое поведение) и команды проверки. Код каждой задачи пишется и проверяется в ветке `plan-2-window` в порядке задач (рецензия после каждой задачи, итоговая рецензия ветки); проверенный код — коммиты из таблицы «Где код задач 1–14» ниже, запись о нём — история git (в текст плана код не копируется). Задача 15 — живая проверка окна на ПК с пользователем.

**Порядок задач.** 1 — каркас; 2–7 — основной процесс и мост (канал, связь, установка, настройки и трей, источники программ, сборка основного процесса); 8–9 — модель страницы (словарь, плашки, доска, хранилище); 10–13 — интерфейс; 14 — сборка приложения; 15 — живая проверка.

### Карта файлов

| Файл | Ответственность | Задача |
|---|---|---|
| `app/package.json`, `pnpm-workspace.yaml`, `pnpm-lock.yaml`, `electron.vite.config.ts`, `tsconfig*.json`, `vitest.config.ts` | каркас, сборка, тесты | 1 |
| `app/src/shared/protocol.ts` | типы протокола, константы, пределы, `packageValue` | 2 |
| `app/src/shared/wire.ts` | строки канала: сборка запроса, разбор входящей строки, `LineDecoder` | 2 |
| `app/src/main/rpc.ts`, `app/src/main/pipe.ts` | клиент RPC поверх соединения; соединение с каналом и разбор ошибок подключения | 2 |
| `app/src/shared/status.ts`, `app/src/main/link.ts`, `app/src/main/task.ts` | типы `LinkStatus` и `InstallOutcome`; состояние связи, переподключение, запуск задачи планировщика | 3 |
| `app/src/main/setup.ts` | SID пользователя, установка служебного процесса с UAC, разбор вывода | 4 |
| `app/src/shared/settings.ts`, `app/src/main/settingsFile.ts`, `app/src/main/autoEnable.ts`, `app/src/main/trayModel.ts` | настройки окна, «Включать деление при запуске», состояние значка трея | 5 |
| `app/src/shared/programs.ts`, `app/src/main/programs.ts`, `app/src/main/storeApps.ts`, `app/src/main/icons.ts` | типы `Program` и `Dropped`; ярлыки меню «Пуск», приложения Магазина, брошенные файлы, значки с кэшем | 6 |
| `app/src/shared/ipc.ts`, `app/src/main/ipc.ts`, `app/src/main/index.ts`, `app/src/main/tray.ts`, `app/src/preload/index.ts` | мост, проверка данных от страницы, жизненный цикл окна, трей, автозапуск | 7 |
| `app/src/renderer/i18n/ru.ts`, `app/src/renderer/i18n/t.ts`, `app/src/renderer/model/format.ts`, `app/src/renderer/model/banners.ts` | словарь, числа и адреса, плашки состояний 9.5 | 8 |
| `app/src/renderer/model/board.ts`, `app/src/renderer/model/layout.ts`, `app/src/renderer/store/*.ts` | модель доски, операции раскладки, хранилище с откатом | 9 |
| `app/src/renderer/App.tsx`, `components/Header*`, `components/Banner*`, `components/Lane*`, `components/AppCard*`, `components/ActivityBars*`, `styles/theme.css` | главное окно без перетаскивания | 10 |
| `app/src/renderer/dnd/*`, `components/ContextMenu*` | перетаскивание, контекстное меню, клавиатура, файлы из Проводника | 11 |
| `app/src/renderer/components/AddPanel*`, `components/ProgramCard*` | панель «Добавить», карточка программы | 12 |
| `app/src/renderer/screens/*` | «Настройка», «Подключите VPN» / «Подключение», «Другой пользователь», «Обновите приложение», заставка | 13 |
| `app/electron-builder.yml`, `scripts/build-app.ps1`, `README.md` | сборка приложения с `resources\helper` | 14 |

### Разделы спецификации и задачи

| Раздел | Задачи |
|---|---|
| 4.3 Первая настройка и запуск | 3, 4, 13, 15 |
| 5.1 Канал, 5.2 Состояние | 2, 3, 9 |
| 7 Коннектор Tailscale (вход, exit node, истечение входа) | 8, 13 |
| 8.1–8.3 Правила и раскладка | 2, 9, 11, 12 |
| 8.4 Живая активность | 9, 10, 12 |
| 9.1 Главное окно | 10 |
| 9.2 Действия с программами | 9, 11 |
| 9.3 Добавление | 6, 12 |
| 9.4 Карточка программы | 12 |
| 9.5 Первый запуск и состояния | 8, 13 |
| 9.6 Трей и автозапуск | 5, 7 |
| 9.7 Оформление и тексты | 8, 10 |
| 9.8 Технологии окна | 1, 7, 11 |
| 10 Окно упало / служебный процесс упал | 3, 15 |
| 11 Безопасность окна | 7 |
| 12 Модульные тесты окна, ручной список | 2, 9, 15 |

### Где код задач 1–14

Проверенный код каждой задачи — коммиты ветки `plan-2-window`:

| Задача | Коммиты |
|---|---|
| 1 | `113d181`, `abd8ad7` |
| 2 | `e8d98ad` |
| 3 | `3f6170d` |
| 4 | `83a7e84`, `e065881` |
| 5 | `797a9e2` |
| 6 | `44fc5b7` |
| 7 | `75fff65` |
| 8 | `56b13c9` |
| 9 | `7d8d67c` |
| 10 | `0fe8771` |
| 11 | `0b87edf` |
| 12 | `2275bc7`, `fb4a81b` |
| 13 | `aa6b58b` |
| 14 | `198b8bb` |
| итоговая рецензия ветки | `7d1dfb4`, `728972d` |
| замечания живой проверки (задача 15) | `6b93c02` |
| 16 (добавлена после живой проверки) | `1247305`, `256264f` |

**Задача 16 (добавлена после живой проверки): игры Steam и Epic.** Пользователь не смог добавить игру Steam: её ярлык — интернет-ярлык `.url` (`steam://rungameid/<номер>`), а в «Установленных» игр не было (Steam и Epic кладут в меню «Пуск» ярлыки `.url`). Сделано: разбор `.url`; игры Steam по библиотекам (`libraryfolders.vdf`, `appmanifest_<номер>.acf`, только полностью установленные), главный исполняемый файл — ближайший к корню папки игры, не вспомогательный, с предпочтением файла, названного по игре; игры Epic по манифестам (`LaunchExecutable`); путь к Steam читается в UTF-8; неразрешимая игровая ссылка — подсказка «запустите игру, и она появится в «Сейчас в сети»». Там же — тексты экрана «Первая настройка» и тёмная рамка окна по просьбе пользователя.

---

### Task 1: Каркас приложения `app/`

**Files:**
- Create: `app/package.json`, `app/pnpm-workspace.yaml`, `app/pnpm-lock.yaml` (результат `pnpm install`), `app/electron.vite.config.ts`, `app/tsconfig.json`, `app/tsconfig.node.json`, `app/tsconfig.web.json`, `app/vitest.config.ts`
- Create: `app/src/main/index.ts` (окно 860×560, минимум 860×560, без меню), `app/src/preload/index.ts` (пустой мост `splitlane: {}`), `app/src/renderer/index.html` (CSP из Global Constraints, `lang="ru"`), `app/src/renderer/main.tsx`, `app/src/renderer/App.tsx` (заголовок «SplitLane»), `app/src/renderer/styles/theme.css` (переменные цветов и шрифтов 9.7), `app/src/renderer/env.d.ts`
- Create: `app/src/shared/smoke.test.ts`, `app/src/renderer/App.test.tsx`
- Modify: `.gitignore` (`app/node_modules/`, `app/out/`, `app/release/`)

**Interfaces:**
- Produces: скрипты `package.json`: `dev` (`electron-vite dev`), `build` (`electron-vite build`), `typecheck` (`tsc -p tsconfig.node.json --noEmit && tsc -p tsconfig.web.json --noEmit`), `test` (`vitest run`), `pack` (задача 14). Vitest — два проекта: `node` (`src/main/**/*.test.ts`, `src/shared/**/*.test.ts`, окружение `node`) и `renderer` (`src/renderer/**/*.test.{ts,tsx}`, окружение `jsdom`). Переменные CSS: `--bg`, `--panel`, `--card`, `--border`, `--text`, `--muted`, `--vpn`, `--direct`, `--warn`, `--error`, `--font-ui`, `--font-num`.

- [ ] **Step 1:** `pnpm install` по `package.json` с закреплёнными версиями; ожидается lock-файл и отсутствие ошибок сборочных скриптов (Electron разрешён в `allowBuilds`).
- [ ] **Step 2:** тесты: `smoke.test.ts` — `1 + 1 === 2` в проекте `node`; `App.test.tsx` — рендер `App` показывает текст «SplitLane» (проект `renderer`).
- [ ] **Step 3:** `pnpm typecheck; pnpm test; pnpm build` — без ошибок; `out/main/index.js`, `out/preload/index.js`, `out/renderer/index.html` существуют.
- [ ] **Step 4:** `pnpm dev` открывает тёмное окно с заголовком «SplitLane» (проверка глазами, закрыть).
- [ ] **Step 5:** коммит `feat(app): electron-vite scaffold of the window`.

### Task 2: Протокол, строки канала, клиент RPC

**Files:**
- Create: `app/src/shared/protocol.ts`, `app/src/shared/wire.ts`, `app/src/main/rpc.ts`, `app/src/main/pipe.ts`
- Test: `app/src/shared/protocol.test.ts`, `app/src/shared/wire.test.ts`, `app/src/main/rpc.test.ts`, `app/src/main/pipe.test.ts`

**Interfaces:**
- Produces (`shared/protocol.ts`):
  - `export const PROTOCOL_VERSION = 1; export const PIPE_NAME = '\\\\.\\pipe\\SplitLane.Helper'; export const TASK_NAME = 'SplitLane Helper';`
  - `export type Lane = 'vpn' | 'direct'; export type Phase = 'off' | 'starting' | 'on' | 'degraded' | 'blocked' | 'stopping' | 'error';`
  - `export const REASON_CODES` (18 строк раздела 5.2) и `export type ReasonCode = (typeof REASON_CODES)[number]; export interface Reason { code: ReasonCode; params: Record<string, string> }`
  - `export type MatcherKind = 'folder' | 'file' | 'package' | 'system'; export interface AppRule { id: string; name: string; exe: string; matcher: { kind: MatcherKind; value: string }; lane: Lane }`
  - `export type Backend = 'stopped' | 'needs_login' | 'starting' | 'running' | 'needs_approval' | 'expired'; export interface ExitNode { id: string; name: string; online: boolean }`
  - `export interface HelperState { phase: Phase; reason: Reason | null; allDirect: boolean; defaultLane: Lane; apps: AppRule[]; addresses: { vpn: string | null; direct: string | null }; tailscale: { backend: Backend; authUrl: string | null; deviceName: string; exitNodes: ExitNode[]; selectedExitNode: string | null; path: 'direct' | 'relay' | null }; systemTailscale: { installed: boolean; exitNodeOn: boolean; restoreOnDisable: boolean } }`
  - `export const ERROR_CODES` (9) и `export type ErrorCode`; `export type RejectReason = 'too_broad' | 'protected' | 'duplicate'; export interface RuleRejection { id: string; reason: RejectReason }; export interface HelperError { code: ErrorCode; message: string; protocol?: number; rules?: RuleRejection[] }`
  - `export interface HelloResult { protocol: number; helper: string; core: string }`
  - `export interface Traffic { down: number; up: number }; export interface ActivityApp { key: string; kind: 'app' | 'system' | 'unattributed'; name: string; exe: string; ruleId: string | null; lane: Lane; down: number; up: number; connections: number }; export interface ActivityDetail { key: string; exes: string[]; connections: { host: string; lane: Lane; down: number; up: number }[] }; export interface Activity { lanes: { vpn: Traffic; direct: Traffic }; apps: ActivityApp[]; detail?: ActivityDetail }`
  - `export const LIMITS = { maxApps: 500, maxNameBytes: 256, maxPathBytes: 1024 } as const; export function utf8Bytes(s: string): number; export function packageValue(exe: string): string | null` (тот же алгоритм, что `helper/internal/rules/matcher.go: packageValue`); `export function parseState(x: unknown): HelperState` (бросает `Error` на неверную форму: фаза, дорожка, массивы); `export function parseActivity(x: unknown): Activity`.
- Produces (`shared/wire.ts`): `export function encodeRequest(id: number, method: string, params?: object): string` (JSON + `\n`, без поля `params`, если оно не передано); `export type Incoming = { kind: 'result'; id: number; result: unknown } | { kind: 'error'; id: number | null; error: HelperError } | { kind: 'event'; event: string; data: unknown } | { kind: 'invalid'; line: string }; export function parseLine(line: string): Incoming; export class LineDecoder { push(chunk: Buffer | string): string[]; }` (байты собираются через `TextDecoder` с `stream: true`, строки делятся по `\n`, `\r` в конце отбрасывается, пустые строки пропускаются).
- Produces (`main/rpc.ts`): `export interface Conn { write(text: string): void; end(): void; onData(cb: (chunk: Buffer) => void): void; onClose(cb: (err?: Error) => void): void }`; `export class HelperCallError extends Error { readonly error: HelperError }`; `export class ConnectionClosedError extends Error`; `export class CallTimeoutError extends Error`; `export class RpcClient { constructor(conn: Conn, opts?: { timeoutMs?: number }); call<T>(method: string, params?: object, timeoutMs?: number): Promise<T>; onEvent(cb: (event: string, data: unknown) => void): () => void; onClose(cb: () => void): () => void; close(): void; readonly closed: boolean }` (время ожидания по умолчанию 30 секунд; `diagnostics.export` — 120 секунд).
- Produces (`main/pipe.ts`): `export type DialFailure = 'absent' | 'denied' | 'other'; export class DialError extends Error { readonly failure: DialFailure; readonly code?: string }; export function classifyDialError(code: string | undefined): DialFailure` (`ENOENT` → `absent`, `EACCES` и `EPERM` → `denied`, остальное → `other`); `export function dialPipe(name: string, timeoutMs?: number): Promise<Conn>` (через `net.connect`).

Тесты (имена обязательны):
- `wire.test.ts`: `encodes a request without params`, `encodes a request with params`, `parses result, error with rules and protocol, event, invalid`, `joins a long line split into many chunks` (строка 2 МиБ кусками по 7 байт), `keeps a multibyte character split across chunks` («Программы» в UTF-8, разрез посередине символа), `skips empty lines and strips CR`.
- `protocol.test.ts`: `packageValue matches the helper` (примеры из `helper/internal/rules` тестов: путь в `WindowsApps` с пятью полями, с пустым `ResourceId`, с четырьмя полями — `null`, без файла после папки — `null`, регистр), `utf8Bytes counts bytes`, `parseState accepts the helper's empty state` (строка из `helper/internal/protocol/protocol_test.go`: `reason: null`, `apps: []`, адреса `null`), `parseState rejects an unknown phase`, `parseActivity accepts a summary with detail and without it`.
- `rpc.test.ts` (подменное соединение): `matches results to calls by id`, `rejects with HelperCallError on an error response`, `delivers events in order between results`, `fails pending calls when the pipe closes`, `times out a call and ignores its late result`, `refuses calls after close`.
- `pipe.test.ts` (сервер `net.createServer` на `\\.\pipe\SplitLane.WindowTest.<pid>`): `dials a pipe and exchanges a line`, `reports absent for a missing pipe`, `classifyDialError maps codes`.

- [ ] Шаги: тесты → провал (нет модулей) → реализация → тесты проходят → полная проверка → коммит `feat(app): helper protocol types, line framing and RPC client`.

### Task 3: Связь со служебным процессом

Состояния связи и переподключение (4.3, 5.1, раздел 10: «Окно при потере канала сразу запускает задачу снова (`schtasks /run`, до 5 попыток с паузой 2 секунды) и показывает плашку «Служебный процесс перезапускается»; если не удалось — «Служебный процесс не отвечает» с действием «Повторить»»).

**Files:**
- Create: `app/src/shared/status.ts`, `app/src/main/link.ts`, `app/src/main/task.ts`
- Test: `app/src/main/link.test.ts`, `app/src/main/task.test.ts`

**Interfaces:**
- Consumes: `Conn`, `RpcClient`, `DialError` (задача 2); `HelloResult`, `HelperState`, `Activity`, `PROTOCOL_VERSION`, `parseState`, `parseActivity`.
- Produces (`shared/status.ts`): тип `LinkStatus` (ниже) и `export type InstallOutcome = { kind: 'ok' } | { kind: 'cancelled' } | { kind: 'refused'; reason: string } | { kind: 'failed'; detail: string }` (его заполняет задача 4).
- Produces (`main/task.ts`): `export interface Exec { (file: string, args: string[]): Promise<{ code: number; stdout: string; stderr: string }> }`; `export function taskRegistered(exec: Exec): Promise<boolean>` (`schtasks /query /tn "SplitLane Helper"`, код 0 — есть); `export function runTask(exec: Exec): Promise<void>` (`schtasks /run /tn "SplitLane Helper"`, ненулевой код — `Error` с выводом).
- Produces (`main/link.ts`; тип `LinkStatus` живёт в `shared/status.ts`):
  - `export type LinkStatus = { kind: 'connecting' } | { kind: 'ready'; hello: HelloResult } | { kind: 'setup-needed' } | { kind: 'other-user' } | { kind: 'helper-older'; helperProtocol: number } | { kind: 'helper-newer'; helperProtocol: number } | { kind: 'restarting'; attempt: number } | { kind: 'unresponsive' }`
  - `export interface LinkDeps { dial(): Promise<Conn>; taskRegistered(): Promise<boolean>; runTask(): Promise<void>; sleep(ms: number): Promise<void>; log(message: string): void }`
  - `export class HelperLink { constructor(deps: LinkDeps); start(): void; retry(): void; stop(): void; status(): LinkStatus; state(): HelperState | null; call<T>(method: string, params?: object, timeoutMs?: number): Promise<T>; setActivityDetail(key: string | null): Promise<void>; on(event: 'status', cb: (s: LinkStatus) => void): () => void; on(event: 'state', cb: (s: HelperState) => void): () => void; on(event: 'activity', cb: (a: Activity) => void): () => void }`
  - Порядок подключения: `dial` → `hello` → `state.get` → `activity.subscribe` (с запомненным `detailFor`) → `ready`. `protocol_mismatch`: `helperProtocol < 1` → `helper-older`, больше → `helper-newer`. `dial` `absent`: задачи нет → `setup-needed`; есть → цикл перезапуска. `denied` → `other-user`. Цикл перезапуска: попытки 1…5 — `runTask`, затем ожидание канала до 2 секунд (`dial` каждые 200 мс), статус `restarting` с номером попытки; после пятой — `unresponsive`. Канал пропал в `ready` → тот же цикл. `retry()` начинает подключение заново из любого статуса. `call` без связи — `ConnectionClosedError`.

Тесты: `connects: hello, state.get, subscribe, ready`, `setup-needed when there is no pipe and no task`, `starts the task when the pipe is absent but the task exists`, `other-user when the pipe denies access`, `helper-older and helper-newer on protocol_mismatch`, `restarts the task up to five times when the pipe is lost, then unresponsive`, `retry starts over`, `re-subscribes the activity detail after a reconnect`, `fails pending calls when the pipe closes` (вызов в пути при обрыве), `forwards state and activity events`; `task.test.ts`: `taskRegistered reads the exit code`, `runTask throws with the output on failure`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): helper link with restart and status`.

### Task 4: Установка служебного процесса из окна

4.3, пункты 1–3; 5.1 (`protocol_mismatch` → «Обновить служебный процесс» — та же команда `install`).

**Files:**
- Create: `app/src/main/setup.ts`
- Test: `app/src/main/setup.test.ts`

**Interfaces:**
- Consumes: `Exec` (задача 3); `InstallOutcome` (`shared/status.ts`, задача 3).
- Produces:
  - `export function helperSourceDir(o: { isPackaged: boolean; resourcesPath: string; appPath: string }): string` — собранное приложение: `<resourcesPath>\helper`; разработка: `<appPath>\..\dist\helper`.
  - `export const HELPER_FILES = ['splitlane-helper.exe', 'sing-box.exe', 'libcronet.dll', 'sing-box-LICENSE', 'LICENSE', 'THIRD-PARTY.md'] as const`
  - `export function parseSid(whoamiCsv: string): string` (вывод `whoami /user /fo csv /nh`; SID по `^S-1-\d+(-\d+)+$`, иначе `Error`)
  - `export function installScript(exe: string, sid: string, outFile: string): string` — текст PowerShell (ASCII): `Start-Process -FilePath cmd.exe -ArgumentList '/c', '""<exe>" install --user-sid <sid> > "<outFile>" 2>&1"' -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop`; отмена UAC (исключение) → `exit 1223`; иначе `exit 0`. Пути проверяются: без `"` и `'`, абсолютные.
  - `export function encodePowerShell(script: string): string[]` — аргументы `['-NoProfile', '-NonInteractive', '-EncodedCommand', <base64 UTF-16LE>]`
  - `export function parseInstallOutput(text: string): InstallOutcome` (последняя непустая строка `INSTALL-OK` → `ok`; `INSTALL-REFUSED: <r>` → `refused`; иначе `failed` с хвостом вывода)
  - `export async function installHelper(d: { exec: Exec; sourceDir: string; tempDir: string; readText(path: string): Promise<string>; exists(path: string): Promise<boolean> }): Promise<InstallOutcome>` — проверяет шесть файлов, берёт SID, запускает PowerShell, при коде 1223 — `cancelled`, иначе разбирает файл вывода.

Тесты: `helperSourceDir for packaged and dev`, `parseSid takes the SID from whoami csv`, `parseSid rejects garbage`, `installScript quotes paths and maps a cancelled UAC to 1223` (проверка текста), `installScript refuses quotes in paths`, `encodePowerShell is UTF-16LE base64`, `parseInstallOutput ok, refused, failed`, `installHelper reports missing files`, `installHelper maps exit 1223 to cancelled`, `installHelper reads the output file`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): install the helper with UAC from the window`.

### Task 5: Настройки окна, автовключение, модель трея

9.6.

**Files:**
- Create: `app/src/shared/settings.ts`, `app/src/main/settingsFile.ts`, `app/src/main/autoEnable.ts`, `app/src/main/trayModel.ts`
- Test: `app/src/shared/settings.test.ts`, `app/src/main/settingsFile.test.ts`, `app/src/main/autoEnable.test.ts`, `app/src/main/trayModel.test.ts`

**Interfaces:**
- Consumes: `HelperState`, `LinkStatus`.
- Produces (`shared/settings.ts`): `export interface WindowSettings { openAtLogin: boolean; enableOnStart: boolean; connectedOnce: boolean }`; `export const DEFAULT_SETTINGS: WindowSettings` (все `false`); `export function parseSettings(text: string): WindowSettings` (неизвестные и неверные поля — значения по умолчанию). (`main/settingsFile.ts`): `export class SettingsFile { constructor(path: string); load(): Promise<WindowSettings>; save(s: WindowSettings): Promise<void> }` (запись во временный файл и переименование; файл `%AppData%\SplitLane\settings.json`). `connectedOnce` ставится окном после первого перехода в фазу `on` (экран «Подключите VPN» больше не открывается сам).
- Produces (`autoEnable.ts`): `export interface AutoEnableDeps { enable(): Promise<void>; setTimer(fn: () => void, ms: number): () => void; now(): number }`; `export class AutoEnable { constructor(d: AutoEnableDeps); onReady(state: HelperState, setting: boolean): void; onState(state: HelperState): void; stop(): void }` — один раз за запуск окна: при `setting` и фазе `off` — `routing.enable`; если итог — фаза `error` с причиной `no_default_route`, повтор каждые 10 секунд не дольше 2 минут от первой попытки; любое действие пользователя с делением (`stop()`) прекращает повторы.
- Produces (`trayModel.ts`): `export type TrayIcon = 'working' | 'off' | 'action' | 'vpn-down'`; `export function trayIconFor(link: LinkStatus, state: HelperState | null, previous: TrayIcon): TrayIcon` — по таблице 9.6 (`on` → `working`; `off` → `off`; `degraded` + `vpn_unreachable` → `vpn-down`; `blocked`, `error`, `degraded` + `login_expired`, `restore_pending`, `allDirect`, связь не `ready` и не `connecting`/`restarting` → `action`; `starting`/`stopping` без причины → `previous`); `export function trayMenuModel(link: LinkStatus, state: HelperState | null): { toggle: 'enable' | 'disable' | null }`; `export function circleBitmap(size: number, rgb: [number, number, number]): Buffer` (BGRA, круг с мягким краем, прозрачный фон).

Тесты: `parseSettings fills defaults and ignores junk`, `SettingsFile saves atomically and loads back`, `enables once when the setting is on and the phase is off`, `does not enable when the setting is off or the phase is not off`, `retries no_default_route every 10 s for 2 minutes`, `stop cancels the retries`, `trayIconFor follows the table of 9.6` (по строке на каждую клетку таблицы), `keeps the previous icon in starting and stopping`, `trayMenuModel offers enable in off and error, disable in live phases`, `circleBitmap has the size and a transparent corner`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): window settings, enable on start and the tray model`.

### Task 6: Программы: меню «Пуск», Магазин, Проводник, значки

9.3, 9.2 (перетаскивание из Проводника), 9.8 (значки).

**Files:**
- Create: `app/src/shared/programs.ts`, `app/src/main/programs.ts`, `app/src/main/storeApps.ts`, `app/src/main/icons.ts`
- Test: `app/src/main/programs.test.ts`, `app/src/main/storeApps.test.ts`, `app/src/main/icons.test.ts`

**Interfaces:**
- Consumes: `packageValue`, `Exec`.
- Produces (`shared/programs.ts`): `export interface Program { exe: string; name: string }`; `export type Dropped = { kind: 'program'; program: Program } | { kind: 'skipped'; name: string }`.
- Produces (`main/programs.ts`): `export function startMenuDirs(env: Record<string, string | undefined>): string[]` (`%ProgramData%\Microsoft\Windows\Start Menu\Programs`, `%AppData%\Microsoft\Windows\Start Menu\Programs`); `export interface ProgramsFs { listFiles(dir: string): Promise<string[]> /* рекурсивно */; readShortcut(lnk: string): { target: string } | null }`; `export async function installedPrograms(fs: ProgramsFs, dirs: string[]): Promise<Program[]>` (только цели `.exe`, повторы по пути без учёта регистра убраны, имя — имя ярлыка без `.lnk`, сортировка по имени `localeCompare('ru')`); `export function resolveDropped(paths: string[], readShortcut: ProgramsFs['readShortcut']): Dropped[]` (`.exe` по абсолютному пути на букве диска — программа с пустым именем; `.lnk` с целью `.exe` — программа с именем ярлыка; остальное — `skipped`).
- Produces (`storeApps.ts`): `export const STORE_APPS_SCRIPT: string` (ASCII; `Get-StartApps` и `Get-AppxPackage -PackageTypeFilter Main`, путь `InstallLocation` + `Executable` из манифеста для каждого `AppId`, вывод `ConvertTo-Json -Compress` массивом `{name, exe}`); `export function parseStoreApps(json: string): Program[]` (только пути, для которых `packageValue` не `null`; повторы убраны); `export class StoreAppsCache { constructor(load: () => Promise<string>); get(): Promise<Program[]> }` (один вызов PowerShell за запуск окна, ошибка → пустой список).
- Produces (`icons.ts`): `export interface IconSource { (exe: string): Promise<{ png: Buffer } | null> }`; `export class IconCache { constructor(dir: string, source: IconSource, fs: { stat(p: string): Promise<{ mtimeMs: number } | null>; read(p: string): Promise<Buffer | null>; write(p: string, b: Buffer): Promise<void> }); get(exe: string): Promise<string | null> }` — ключ `sha1(путь в нижнем регистре + '|' + mtimeMs)`, файл `<dir>\<ключ>.png`, результат — `data:image/png;base64,...`; одновременные запросы одного файла — один вызов источника.

Тесты: `installedPrograms keeps exe targets, dedupes and sorts`, `startMenuDirs builds both folders`, `resolves dropped files to programs and skips the rest` (`.exe`, `.lnk` на `.exe`, `.lnk` на документ, папка, `.bat`, `\\server\share\a.exe` — пропуск), `parseStoreApps keeps package executables only`, `StoreAppsCache calls PowerShell once and survives a failure`, `IconCache reads from disk after the first call`, `IconCache shares one source call for concurrent requests`, `IconCache returns null without an icon`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): installed programs, Explorer drops and icon cache`.

### Task 7: Основной процесс, мост и трей

9.6, 9.8, 11 (окно), 4.3.

**Files:**
- Create: `app/src/shared/ipc.ts`, `app/src/main/ipc.ts`, `app/src/main/tray.ts`; Modify: `app/src/main/index.ts`, `app/src/preload/index.ts`
- Test: `app/src/main/ipc.test.ts`

**Interfaces:**
- Consumes: всё из задач 2–6.
- Produces (`shared/ipc.ts`):
  - `export type InstallState = { kind: 'idle' } | { kind: 'running' } | { kind: 'done'; outcome: InstallOutcome }`
  - `export interface Snapshot { link: LinkStatus; state: HelperState | null; settings: WindowSettings; install: InstallState }`
  - `export type Command = { method: 'routing.enable' } | { method: 'routing.disable'; skipRestore?: boolean } | { method: 'routing.allDirect'; on: boolean } | { method: 'lanes.set'; apps: AppRule[]; defaultLane: Lane } | { method: 'tailscale.login' } | { method: 'tailscale.logout' } | { method: 'tailscale.setExitNode'; id: string } | { method: 'systemTailscale.clearExitNode' }`
  - `export type CommandResult = { ok: true } | { ok: false; error: HelperError }` (обрыв канала — `error.code = 'internal'`, `message = 'link closed'`)
  - `export interface SplitLaneBridge { snapshot(): Promise<Snapshot>; onSnapshot(cb: (s: Snapshot) => void): () => void; onActivity(cb: (a: Activity) => void): () => void; command(c: Command): Promise<CommandResult>; setActivityDetail(key: string | null): Promise<void>; installHelper(): Promise<void>; retryLink(): Promise<void>; setSettings(patch: Partial<WindowSettings>): Promise<WindowSettings>; installedPrograms(): Promise<Program[]>; pickProgram(): Promise<Program | null>; resolveDropped(files: File[]): Promise<Dropped[]>; icon(exe: string): Promise<string | null>; openLogin(url: string): Promise<boolean>; saveDiagnostics(): Promise<{ saved: boolean; error?: HelperError }>; quit(): Promise<void> }`
  - `export const IPC = { ... } as const` — имена каналов.
- Produces (`main/ipc.ts`): `export function validateCommand(x: unknown): Command` (форма, пределы `LIMITS`, UUID, дорожки, виды; иначе `Error`); `export function validateLoginUrl(x: unknown): string | null` (`https:`, хост `login.tailscale.com`); `export function registerIpc(d: { link: HelperLink; ... }): void`.
- `main/index.ts`: один экземпляр (`requestSingleInstanceLock`, второй — показать окно); `--hidden` в аргументах — не показывать окно при старте; закрытие окна прячет его в трей; «Выход» в трее и `quit()` — `helper.shutdown` (до 30 секунд ожидания ответа), затем `app.quit()`; автозапуск — `app.setLoginItemSettings({ openAtLogin, path: process.execPath, args: ['--hidden'] })` только в собранном приложении; `app.setAppUserModelId('SplitLane')`; новые окна и навигация запрещены (`setWindowOpenHandler` → `deny`, `will-navigate` → `preventDefault`); после установки (`install` → `ok`) — `runTask` и `link.retry()`.
- `preload/index.ts`: `contextBridge.exposeInMainWorld('splitlane', bridge)`; `resolveDropped` переводит `File` в пути через `webUtils.getPathForFile` и отправляет только строки.

Тесты (`ipc.test.ts`): `validateCommand accepts every method of the union`, `validateCommand rejects unknown methods and extra fields`, `validateCommand enforces LIMITS, UUIDs and lanes`, `validateLoginUrl accepts only https login.tailscale.com`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка; `pnpm dev`: окно открывается, в журнале основного процесса — статус связи (`setup-needed` или `ready`, смотря по ПК) → коммит `feat(app): main process, bridge and tray`.

### Task 8: Словарь, форматы, плашки состояний

9.5, 9.7, 5.2 (коды причин), 5.1 (коды ошибок), 8.3 (причины отказа).

**Files:**
- Create: `app/src/renderer/i18n/ru.ts`, `app/src/renderer/i18n/t.ts`, `app/src/renderer/model/format.ts`, `app/src/renderer/model/banners.ts`
- Test: `app/src/renderer/i18n/t.test.ts`, `app/src/renderer/model/format.test.ts`, `app/src/renderer/model/banners.test.ts`

**Interfaces:**
- Produces: `export const ru = { ... } as const; export type Key = keyof typeof ru; export function t(key: Key, params?: Record<string, string | number>): string` (подстановка `{name}`; отсутствующий параметр — `Error` в тестах); ключи причин `reason.<code>`, ошибок `error.<code>`, отказов `reject.<reason>`.
- `format.ts`: `export function formatSpeed(bytesPerSecond: number): string` (`0` → `''`; `< 1024` → `"512 Б/с"`; КБ/с и МБ/с с одной цифрой после запятой, запятая — десятичный разделитель); `export function maskAddress(ip: string | null): string` (`203.0.113.7` → `203.0.•.•`; `null` → `''`).
- `banners.ts`: `export type BannerAction = 'enable' | 'cancel' | 'retry' | 'allDirectOn' | 'allDirectOff' | 'loginAgain' | 'clearSystemExitNode' | 'disable' | 'skipRestore' | 'saveLog' | 'retryLink'`; `export interface Banner { tone: 'info' | 'warn' | 'error'; title: string; detail?: string; command?: string; actions: BannerAction[] }`; `export function bannerFor(link: LinkStatus, state: HelperState | null): Banner | null` — строки таблицы 9.5, режим «всё напрямую», связь `restarting` («Служебный процесс перезапускается») и `unresponsive` («Служебный процесс не отвечает», `retryLink`); `restore_pending` — `detail = params.error`, `command = params.command` (пустая команда — без поля); `error` — заголовок «Не удалось включить: <текст причины>», действия `retry`, `saveLog`.

Тесты: `every reason, error and rejection code has a text` (перебор `REASON_CODES`, `ERROR_CODES`, трёх отказов), `t substitutes params`, `formatSpeed covers bytes, KB and MB with a comma`, `maskAddress hides the last two numbers`, `bannerFor follows the table of 9.5` (по строке на каждую строку таблицы), `bannerFor shows the restore command only when there is one`, `bannerFor covers allDirect, restarting and unresponsive`, `no banner in on without allDirect`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): dictionary, number formats and state banners`.

### Task 9: Модель доски и хранилище

8.1–8.4, 9.1, 9.2 (результаты действий), 12 («хранилище и операции доски»).

**Files:**
- Create: `app/src/renderer/model/board.ts`, `app/src/renderer/model/layout.ts`, `app/src/renderer/store/appStore.ts`, `app/src/renderer/store/layoutQueue.ts`
- Test: `app/src/renderer/model/board.test.ts`, `app/src/renderer/model/layout.test.ts`, `app/src/renderer/store/appStore.test.ts`

**Interfaces:**
- Consumes: протокол (задача 2), `Snapshot`, `Command`, `CommandResult`, `SplitLaneBridge` (задача 7), `t` (задача 8).
- Produces (`board.ts`): `export interface CardVM { key: string; ruleId: string | null; kind: 'app' | 'system' | 'unattributed'; name: string; exe: string; lane: Lane; pinned: boolean; down: number; up: number; connections: number; movable: boolean }`; `export interface LaneVM { lane: Lane; isDefault: boolean; pinned: CardVM[]; unpinned: CardVM[]; down: number; up: number; address: string | null }`; `export function boardView(apps: AppRule[], defaultLane: Lane, activity: Activity | null, addresses: HelperState['addresses'], search: string): { vpn: LaneVM; direct: LaneVM }` — закреплённые из правил (скорость — из записи активности с тем же `ruleId`; правило вида `system` — карточка «Службы Windows»), незакреплённые — записи активности без `ruleId` вида `app` в основной дорожке, особые плитки `system` (если нет правила `system`) и `unattributed` (не перемещается) — в основной; поиск — по имени и пути без учёта регистра; скорость дорожки — из `activity.lanes`.
- Produces (`layout.ts`): `export interface Layout { apps: AppRule[]; defaultLane: Lane }`; `export function moveTo(l: Layout, card: CardVM, lane: Lane, newId: () => string): Layout`; `export function pin(l: Layout, card: CardVM, newId: () => string): Layout`; `export function unpin(l: Layout, ruleId: string): Layout`; `export function makeDefault(l: Layout, lane: Lane): Layout`; `export function setWholeFolder(l: Layout, ruleId: string, whole: boolean): Layout` (`folder` ↔ `file`, `value` пустой); `export function addPrograms(l: Layout, programs: Program[], lane: Lane, newId: () => string): Layout` (программа с тем же `exe` уже закреплена — меняется её дорожка); `export function newRule(p: Program, lane: Lane, id: string): AppRule` (вид по `packageValue`); `export function layoutOf(state: HelperState): Layout`.
- Produces (`store/appStore.ts`): хранилище zustand `useAppStore` со срезами `snapshot`, `activity`, `history: Record<string, number[]>` (последние 30 значений `down + up` по ключу), `search`, `ui: { addOpen: boolean; cardKey: string | null; screen: 'board' | 'connection' }`, `notice: string | null` и действиями `setSnapshot`, `setActivity`, `setSearch`, `openAdd`, `openCard`, `showConnection`, `dismissNotice`, `change(op: (l: Layout) => Layout): Promise<void>`; селектор `effectiveLayout(s)` — раскладка из последнего `state` с применёнными изменениями в пути.
- Produces (`store/layoutQueue.ts`): `export class LayoutQueue { constructor(send: (l: Layout) => Promise<CommandResult>); push(op: (l: Layout) => Layout, base: Layout): Promise<CommandResult>; pending(base: Layout): Layout }` — изменения уходят по одному; ошибка отменяет своё изменение и все, что поставлены после него, и возвращает текст для `notice` (для `rule_rejected` — по правилам: «<имя>: <текст причины>»).

Тесты: `board.test.ts`: `pinned cards come from rules with speeds by ruleId`, `unpinned programs and special tiles sit in the default lane`, `the system tile disappears while a system rule is pinned`, `unattributed is not movable`, `search filters by name and path`; `layout.test.ts`: `moveTo changes the lane of a pinned rule and keeps its value`, `moveTo pins an unpinned program as a new folder rule`, `moveTo pins a Store program as a package rule`, `moveTo pins the system tile as a system rule`, `pin uses the default lane`, `unpin removes the rule`, `makeDefault keeps pinned rules`, `setWholeFolder switches folder and file with an empty value`, `addPrograms moves an already pinned program instead of duplicating`; `appStore.test.ts` (подменный мост): `applies a change at once and sends lanes.set`, `queues rapid moves in order`, `a rejected move rolls back itself and the moves queued after it`, `rule_rejected names the programs in the notice`, `takes the helper's normalized rule by id`, `rolls back a pending layout when the link drops`, `keeps 30 history points per key`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): board model, layout operations and the store`.

### Task 10: Главное окно

9.1, 9.5 (плашки над дорожками), 9.7.

**Files:**
- Modify: `app/src/renderer/App.tsx`, `app/src/renderer/main.tsx`, `app/src/renderer/styles/theme.css`
- Create: `app/src/renderer/components/Header.tsx` (+ `.module.css`), `Banner.tsx`, `Lane.tsx`, `AppCard.tsx`, `ActivityBars.tsx`, `Toggle.tsx`, `ConnectionPill.tsx`, `app/src/renderer/icons/builtin.tsx` (значки плиток «Службы Windows», «WSL и виртуальные машины», программы без значка — SVG), `app/src/renderer/bridge.ts` (связывает `window.splitlane` с хранилищем: снимки, активность, значки)
- Test: `app/src/renderer/components/Lane.test.tsx`, `Banner.test.tsx`, `Header.test.tsx`

**Interfaces:**
- Consumes: `useAppStore`, `boardView`, `bannerFor`, `t`, `formatSpeed`, `maskAddress`, `SplitLaneBridge`.
- Produces: компоненты `Header`, `Banner`, `Lane({ vm, onMakeDefault })`, `AppCard({ card, onOpen })`, `ActivityBars({ points })`; `App` выбирает экран по `snapshot.link` и состоянию: заставка (`connecting`), экраны задачи 13 (пока — заглушки с заголовком), доска.
- Поведение (9.1): шапка — название, выключатель с подписью (`routing.enable`/`routing.disable`), плашка подключения «Tailscale · <имя exit node> · прямое соединение | через ретранслятор» (нажатие — экран «Подключение»), поиск, «+ Добавить»; дорожки рядом — «Через VPN» слева, «Напрямую» справа; строка-пояснение под заголовком: «Сайты видят адрес сервера <имя exit node> · <адрес VPN>» и «Сайты видят ваш настоящий адрес · <маска>» (полный адрес — по нажатию); суммарная скорость в заголовке; плашка «основная» или ссылка «сделать основной»; карточки — значок, имя, скорость или «тихо», столбики активности, булавка; черта «остальные программы» и пунктирные незакреплённые ниже неё в основной дорожке; пока фаза не `on`/`degraded`/`blocked` — вместо них пояснение; в неосновной — зона «Перетащите сюда программу или ярлык».

Тесты: `Lane shows pinned cards, the line and unpinned cards in the default lane`, `Lane shows the drop hint in the other lane`, `AppCard shows «тихо» without traffic`, `Banner renders actions and calls the bridge`, `Header toggle sends routing.enable in off and routing.disable in on`, `the direct address is masked until clicked`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка; `pnpm dev` против работающего служебного процесса (если он установлен) или против заглушки — окно показывает доску → коммит `feat(app): main window board`.

### Task 11: Перетаскивание, контекстное меню, клавиатура, файлы из Проводника

9.2, 9.8.

**Files:**
- Create: `app/src/renderer/dnd/BoardDnd.tsx` (`DragDropProvider`), `app/src/renderer/dnd/dropTarget.ts` (чистая функция), `app/src/renderer/components/ContextMenu.tsx` (+ `.module.css`), `app/src/renderer/dnd/explorerDrop.ts`
- Modify: `Lane.tsx`, `AppCard.tsx`
- Test: `app/src/renderer/dnd/dropTarget.test.ts`, `app/src/renderer/components/ContextMenu.test.tsx`, `app/src/renderer/dnd/explorerDrop.test.ts`

**Interfaces:**
- Consumes: `moveTo`, `pin`, `unpin`, `addPrograms`, `useAppStore.change`, `SplitLaneBridge.resolveDropped`.
- Produces: `export type DropZone = { lane: Lane; area: 'pinned' | 'unpinned' }`; `export type DropOp = { op: 'move'; lane: Lane } | { op: 'pin' } | { op: 'unpin' } | null`; `export function dropOp(card: CardVM, zone: DropZone, defaultLane: Lane): DropOp` (таблица 9.2: в другую дорожку — закрепить там; незакреплённую выше черты — закрепить в основной; закреплённую ниже черты — открепить; на место, где она уже есть, — `null`; `unattributed` — всегда `null`); `export async function explorerDrop(files: File[], lane: Lane, bridge: Pick<SplitLaneBridge, 'resolveDropped'>): Promise<{ programs: Program[]; skipped: string[] }>`.
- Карточки — `useDraggable` с `id = card.key` и `data = card`; зоны — `useDroppable` с `id = '<lane>:<area>'`; клавиатура — встроенный `KeyboardSensor` (Пробел/Ввод, стрелки, Escape); контекстное меню по правой кнопке и по `Shift+F10`/клавише меню на карточке в фокусе: «Пустить через VPN», «Пустить напрямую», «Закрепить»/«Открепить»; Ввод на карточке — открыть карточку программы. Файлы из Проводника — обычные `onDragOver`/`onDrop` на дорожке (без `draggable="true"` на карточках); пропущенные — `notice` «Не программа: <имена>».

Тесты: `dropOp follows the table of 9.2` (все сочетания), `ContextMenu offers the other lane and pin or unpin`, `ContextMenu opens with Shift+F10 and closes with Escape`, `explorerDrop splits programs and skipped names`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка; `pnpm dev`: перенос карточки мышью и с клавиатуры, контекстное меню (проверка глазами против служебного процесса в фазе `off`: `lanes.set` в `off` сохраняет раскладку без ядра) → коммит `feat(app): drag and drop, context menu, keyboard and Explorer drops`.

### Task 12: Панель «Добавить» и карточка программы

9.3, 9.4, 8.4 (`detailFor`).

**Files:**
- Create: `app/src/renderer/components/AddPanel.tsx` (+ `.module.css`), `app/src/renderer/components/ProgramCard.tsx` (+ `.module.css`), `app/src/renderer/model/addRows.ts`
- Test: `app/src/renderer/model/addRows.test.ts`, `app/src/renderer/components/ProgramCard.test.tsx`

**Interfaces:**
- Consumes: `boardView`, `addPrograms`, `pin`, `moveTo`, `setWholeFolder`, `unpin`, `SplitLaneBridge.installedPrograms/pickProgram/setActivityDetail`.
- Produces: `export interface AddRow { key: string; name: string; exe: string; currentLane: Lane | null; pinned: boolean; actions: ('pin' | 'vpn' | 'direct')[] }`; `export function onlineRows(board: ReturnType<typeof boardView>): AddRow[]` (незакреплённые из активности с меткой дорожки; у идущих по основной — «Закрепить» и кнопка другой дорожки); `export function installedRows(programs: Program[], layout: Layout): AddRow[]` (уже закреплённые — с меткой их дорожки, кнопки «Через VPN» и «Напрямую»).
- Панель: вкладки «Сейчас в сети» (пусто вне фаз `on`/`degraded`/`blocked` — пояснение), «Установленные» (загрузка один раз за открытие), кнопка «Выбрать файл…»; строку можно перетащить на дорожку (тот же `useDraggable`). Карточка программы: имя, значок, состояние («закреплена · через VPN» / «не закреплена · по основной дорожке»), кнопки смены дорожки и открепления, «Вся папка программы» (`setWholeFolder`, доступно для закреплённых видов `folder`/`file`), путь и перечень замеченных файлов (`detail.exes`), «Соединения сейчас» (узел, дорожка, скорость); при открытии — `setActivityDetail(key)`, при закрытии — `setActivityDetail(null)`.

Тесты: `onlineRows offers pin and the other lane for programs on the default lane`, `installedRows marks pinned programs`, `ProgramCard subscribes to detail on open and unsubscribes on close`, `ProgramCard toggles the whole folder`, `ProgramCard lists connections with lanes and speeds`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): add panel and program card`.

### Task 13: Экраны настройки, подключения и состояния связи

4.3, 5.1 (`protocol_mismatch`), 7, 9.5, 9.6 (настройки на экране «Подключение»).

**Files:**
- Create: `app/src/renderer/screens/SetupScreen.tsx`, `ConnectScreen.tsx`, `OtherUserScreen.tsx`, `UpdateAppScreen.tsx`, `Splash.tsx` (+ `.module.css`), `app/src/renderer/model/connect.ts`
- Modify: `App.tsx` (выбор экрана)
- Test: `app/src/renderer/model/connect.test.ts`, `app/src/renderer/screens/SetupScreen.test.tsx`, `app/src/renderer/screens/ConnectScreen.test.tsx`

**Interfaces:**
- Consumes: `Snapshot`, `SplitLaneBridge`, `t`.
- Produces (`connect.ts`): `export type ConnectStep = 'login' | 'waitLogin' | 'approval' | 'exitNode' | 'enable'`; `export function connectStep(state: HelperState): ConnectStep` (вход не выполнен — `login`; есть `authUrl` — `waitLogin`; `needs_approval` — `approval`; узел в `starting`/`running` без выбранного exit node — `exitNode`; иначе `enable`); `export function screenFor(s: Snapshot, uiScreen: 'board' | 'connection'): 'splash' | 'setup' | 'update-helper' | 'update-app' | 'other-user' | 'connect' | 'connection' | 'board'` (`setup-needed` → `setup`; `helper-older` → `update-helper`; `helper-newer` → `update-app`; `other-user`; `ready` и (`!settings.connectedOnce`, или нет выбранного exit node, или фаза `off`/`error` при `authUrl` не `null` либо `backend` из `needs_login`, `needs_approval`, `expired` — после выхода из сети и после «Выключить деление и войти») → `connect`; `uiScreen === 'connection'` → `connection`; иначе `board`; `restarting`/`unresponsive` при известном состоянии — экран остаётся прежним, плашка из `bannerFor`).
- Экран «Настройка»: одна кнопка («Настроить» / «Обновить служебный процесс»), ход («Подтвердите запрос Windows…»), итог: отмена UAC — «Настройка отменена» и кнопка снова; отказ — текст отказа служебного процесса как есть. «Подключите VPN»: плитки «Tailscale» (доступна), «Ключ или подписка», «Файл настроек» (будущие, неактивны); шаги: войти (`tailscale.login`, затем ссылка открывается в браузере `openLogin`; кнопка «Открыть ссылку снова»), ожидание одобрения, выбор exit node (список, в сети — сверху), включение с предупреждением, если в системном Tailscale включён exit node; после перехода в `on` — `setSettings({ connectedOnce: true })`. «Подключение»: имя устройства, выбранный exit node и смена (`tailscale.setExitNode`), «Выйти из Tailscale» (только в `off`/`error`), две настройки 9.6. «Другой пользователь», «Обновите приложение», заставка «Подключение к служебному процессу…».

Тесты: `connectStep walks login, approval, exit node, enable`, `screenFor maps every link status`, `screenFor returns to connect after logout and an expired login`, `SetupScreen shows cancelled and refused outcomes`, `ConnectScreen opens the login link once per link`, `ConnectScreen warns about the system exit node before enabling`, `connection screen disables logout while routing is on`.

- [ ] Шаги: тесты → провал → реализация → проходят → полная проверка → коммит `feat(app): setup, connect and link screens`.

### Task 14: Сборка приложения

4.1, 13 («сборка не подписана; установщика нет»).

**Files:**
- Create: `app/electron-builder.yml` (`appId: app.splitlane.window`, `productName: SplitLane`, `directories.output: release`, `files: [out/**]`, `extraResources: [{ from: ../dist/helper, to: helper }]`, `win.target: dir`, без подписи), `scripts/build-app.ps1` (ASCII: `scripts\build-helper.ps1`, затем `pnpm install --frozen-lockfile`, `pnpm typecheck`, `pnpm test`, `pnpm build`, `pnpm pack`; проверяет шесть файлов в `app\release\win-unpacked\resources\helper`; последняя строка `BUILD-OK app\release\win-unpacked`)
- Modify: `app/package.json` (`pack`: `electron-builder --win --dir`), `README.md` (сборка и запуск окна)

- [ ] **Step 1:** `powershell -NoProfile -ExecutionPolicy Bypass -File C:\code\router\scripts\build-app.ps1` — последняя строка `BUILD-OK app\release\win-unpacked`.
- [ ] **Step 2:** запуск `app\release\win-unpacked\SplitLane.exe` без прав администратора: окно открывается; без установленного продукта — экран «Настройка» (если продукт установлен — доска); закрытие прячет в трей; второй запуск показывает первое окно; «Выход» в трее завершает приложение.
- [ ] **Step 3:** коммит `build(app): unpacked Windows build with the helper as resources`.

### Task 15: Живая проверка окна (нужен пользователь: UAC, мышь, Проводник, трей)

Основной сеанс, под сторожем `Start-P1Guard -NoHelper` (служебный процесс ставит и запускает само окно). Приложение — `C:\code\router\app\release\win-unpacked\SplitLane.exe` (задача 14). Продукт к началу не установлен (план 1B, задача 17, удаление), а `C:\ProgramData\SplitLane` с состоянием узла (вход выполнен, exit node выбран, раскладка из двух правил для `curl-vpn.exe` и `curl-direct.exe`) остался. Каждый шаг, после которого системный exit node может оказаться выключен, — один вызов PowerShell с тайм-аутом 600000 мс, начинается с `. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat`; между вызовами не больше 400 секунд. Сеанс исполнителя работает только через дорожку VPN: пока основная дорожка «Напрямую», исполнитель не делает запросов — смена основной дорожки проверяется одним вызовом, который сам дожидается и смены, и возврата. Режим «всё напрямую» (доступен только из плашки `degraded`) и фаза `blocked` в этой проверке не вызываются: их проверил сценарий плана 1B на уровне служебного процесса (`scripts/scenario.mjs`, шаги 5 и 8, и тесты автомата). После проверки продукт остаётся установленным, деление выключено.

**Если что-то пошло не так** — блок восстановления задачи 23 плана 1A; при `SYSTEM-WRONG`, `NO-TERMINAL-STATE`, `state=restore_blocked` или `no status` — показать пользователю ручное восстановление.

**Files:**
- Modify: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` (приложение A)

- [ ] **Step 1: Готовность (только чтение)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Test-P1Ready; Test-P1SystemRestored; schtasks /query /tn "SplitLane Helper" 2>$null | Out-Null; "task exists: $($LASTEXITCODE -eq 0)"; "program dir: $(Test-Path 'C:\Program Files\SplitLane'); data dir: $(Test-Path 'C:\ProgramData\SplitLane'); window running: $([bool](Get-Process SplitLane -ErrorAction SilentlyContinue)); settings file: $(Test-Path "$env:APPDATA\SplitLane\settings.json")"; $b = (Get-Item C:\code\router\app\release\win-unpacked\SplitLane.exe).LastWriteTime; $c = [DateTimeOffset]::FromUnixTimeSeconds([int64](git -C C:\code\router log -1 --format=%ct -- app helper)).LocalDateTime; "build newer than the code: $($b -gt $c)"
```

Ожидается `P1-READY`, `SYSTEM-RESTORED`, `task exists: False`, `program dir: False; data dir: True; window running: False; settings file: False`, `build newer than the code: True`. Иначе: `False` у сборки — сначала `scripts\build-app.ps1` (`BUILD-OK`); `settings file: True` — переименовать файл в `settings.json.bak` (первый запуск должен быть первым).

- [ ] **Step 2: Согласие пользователя**

Передайте пользователю текст ручного восстановления (Global Constraints плана 1A) и затем:

```text
Сейчас проверка самого окна SplitLane, около 30 минут, под сторожем. Понадобятся ваши руки: принять UAC от slguard.exe; в окне нажать «Настроить» и принять UAC установки; нажать «Включить деление»; перетащить карточку мышью туда и обратно; перетащить файл из Проводника; сменить основную дорожку и сразу вернуть; закрыть окно крестиком и открыть из трея; включить и выключить автозапуск; в конце выйти через значок в трее. Связь у программ будет пропадать на 5–25 секунд при включении, выключении и перезапуске. В значке Tailscale ничего не переключайте. Я буду говорить, что делать, перед каждым шагом.
```

- [ ] **Step 3: Сторож (нужен пользователь: UAC)**

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; Start-P1Guard -NoHelper
```

Ожидается `P1-READY` и `P1-RESULT: state=armed ...`. `UAC-CANCELLED` — без согласия пользователя не повторять.

- [ ] **Step 4: Первая настройка из окна (нужен пользователь: «Настроить», UAC)**

Скажите пользователю: «Сейчас откроется окно SplitLane с экраном «Настройка». Нажмите «Настроить» и подтвердите запрос Windows». Сразу после этого:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; Start-Process C:\code\router\app\release\win-unpacked\SplitLane.exe; $t0 = Get-Date; $ok = $false; while (((Get-Date) - $t0).TotalSeconds -lt 300) { Start-Sleep -Seconds 5; Set-P1Heartbeat; if (Get-ChildItem \\.\pipe\ | Where-Object Name -eq 'SplitLane.Helper') { $ok = $true; break } }; "pipe after setup: $ok seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; schtasks /query /tn "SplitLane Helper" 2>$null | Out-Null; "task exists: $($LASTEXITCODE -eq 0)"; Start-Sleep -Seconds 10; & "$P1Bin\slctl.exe" -timeout 30s state | Select-Object -Last 1
```

Ожидается `pipe after setup: True`, `task exists: True`, `SLCTL: phase=off ... backend=starting` или `running` `... exitNodes=<n>` с `n` больше нуля (окно подключилось, вход сохранён — служебный процесс запустил режим настройки). Пользователь видит экран «Подключите VPN» на шаге «Включить» с предупреждением про exit node в приложении Tailscale.

- [ ] **Step 5: Включение из окна (нужен пользователь: «Включить деление»)**

Скажите пользователю: «Нажмите «Включить деление»». Сразу после этого:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; & "$P1Bin\slctl.exe" -timeout 300s wait on,error | Select-Object -Last 1; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'; Start-Sleep -Seconds 3; "connectedOnce: $((Get-Content "$env:APPDATA\SplitLane\settings.json" -Raw | ConvertFrom-Json).connectedOnce)"
```

Ожидается `SLCTL: phase=on ...`, два `LANE-OK`, `connectedOnce: True`. Пользователь видит доску: в «Через VPN» — карточка `curl-vpn`, в «Напрямую» — `curl-direct`, строки «Сайты видят …» с адресами, плашка подключения «Tailscale · <exit node> · …»; значок в трее — цвет VPN.

- [ ] **Step 6: Перенос мышью (нужен пользователь)**

Скажите пользователю: «Перетащите карточку curl-direct из «Напрямую» в «Через VPN»; через несколько секунд перетащите её обратно». Сразу после этого:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $d = 'C:\code\router\spike\plan1\bin\direct\curl-direct.exe'; $t0 = Get-Date; $there = $false; while (((Get-Date) - $t0).TotalSeconds -lt 240 -and -not $there) { Start-Sleep -Seconds 3; Set-P1Heartbeat; $there = [bool](& "$P1Bin\slctl.exe" expect-lanes "$d=vpn" | Select-String -Pattern '^LANE-OK') }; "moved to vpn: $there"; $back = $false; while (((Get-Date) - $t0).TotalSeconds -lt 360 -and -not $back) { Start-Sleep -Seconds 3; Set-P1Heartbeat; $back = [bool](& "$P1Bin\slctl.exe" expect-lanes "$d=direct" | Select-String -Pattern '^LANE-OK') }; "moved back to direct: $back"
```

Ожидается `moved to vpn: True`, `moved back to direct: True`.

- [ ] **Step 7: Файл из Проводника и «Открепить» (нужен пользователь)**

Положите рядом текстовый файл и откройте пользователю папку: `Set-Content -Path C:\code\router\spike\plan1\bin\free\note.txt -Value 'not a program'; Start-Process explorer.exe C:\code\router\spike\plan1\bin\free`. Скажите: «Выделите в Проводнике curl-free.exe и note.txt и перетащите оба на дорожку «Напрямую»: окно добавит программу и скажет «Не программа: note.txt». Потом правой кнопкой на карточке curl-free выберите «Открепить»». Сразу после этого:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $f = 'C:\code\router\spike\plan1\bin\free\curl-free.exe'; $t0 = Get-Date; $in = $false; while (((Get-Date) - $t0).TotalSeconds -lt 240 -and -not $in) { Start-Sleep -Seconds 3; Set-P1Heartbeat; $in = [bool](& "$P1Bin\slctl.exe" expect-lanes "$f=direct" | Select-String -Pattern '^LANE-OK') }; "dropped onto direct: $in"; $out = $false; while (((Get-Date) - $t0).TotalSeconds -lt 360 -and -not $out) { Start-Sleep -Seconds 3; Set-P1Heartbeat; $out = [bool](& "$P1Bin\slctl.exe" expect-lanes "$f=vpn" | Select-String -Pattern '^LANE-OK') }; "unpinned, by the main lane: $out"; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

Ожидается `dropped onto direct: True`, `unpinned, by the main lane: True`, в состоянии `apps=2`. Пользователь подтверждает сообщение «Не программа: note.txt»; после шага удалите `note.txt`.

- [ ] **Step 8: Смена основной дорожки и возврат (нужен пользователь)**

Скажите пользователю: «Нажмите «сделать основной» у дорожки «Напрямую», дождитесь, пока незакреплённые программы переедут туда, и сразу нажмите «сделать основной» у «Через VPN». Пока основная — «Напрямую», я не смогу отвечать». Сразу после этого:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $f = 'C:\code\router\spike\plan1\bin\free\curl-free.exe'; $t0 = Get-Date; $s = ''; while (((Get-Date) - $t0).TotalSeconds -lt 240 -and $s -notmatch ' defaultLane=direct ') { Start-Sleep -Seconds 2; Set-P1Heartbeat; $s = & "$P1Bin\slctl.exe" state | Select-Object -Last 1 }; "main lane direct: $($s -match ' defaultLane=direct ')"; "free copy goes direct: $([bool](& "$P1Bin\slctl.exe" expect-lanes "$f=direct" | Select-String -Pattern '^LANE-OK'))"; while (((Get-Date) - $t0).TotalSeconds -lt 480 -and $s -notmatch ' defaultLane=vpn ') { Start-Sleep -Seconds 2; Set-P1Heartbeat; $s = & "$P1Bin\slctl.exe" state | Select-Object -Last 1 }; "main lane back to vpn: $($s -match ' defaultLane=vpn ')"; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn "$f=vpn" | Select-String -Pattern '^LANE'
```

Ожидается `main lane direct: True`, `free copy goes direct: True`, `main lane back to vpn: True`, два `LANE-OK`.

- [ ] **Step 9: Сбой служебного процесса: окно запускает его снова**

Скажите пользователю: «Сейчас я аварийно завершу служебный процесс; в окне появится плашка «Служебный процесс перезапускается», через полминуты-минуту деление включится снова». Затем:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $p0 = @(Get-Process splitlane-helper -ErrorAction SilentlyContinue | ForEach-Object { $_.Id }); Send-P1Guard -Action kill-helper; $t0 = Get-Date; $p1 = @(); while (((Get-Date) - $t0).TotalSeconds -lt 120) { Start-Sleep -Seconds 2; Set-P1Heartbeat; $p1 = @(Get-Process splitlane-helper -ErrorAction SilentlyContinue | ForEach-Object { $_.Id }); if ($p1.Count -gt 0 -and -not ($p1 | Where-Object { $p0 -contains $_ })) { break } }; "helper restarted by the window: $($p1.Count -gt 0 -and -not ($p1 | Where-Object { $p0 -contains $_ })) seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; & "$P1Bin\slctl.exe" -timeout 240s wait on,error | Select-Object -Last 1; Set-P1Heartbeat; & "$P1Bin\slctl.exe" expect-lanes C:\code\router\spike\plan1\bin\vpn\curl-vpn.exe=vpn C:\code\router\spike\plan1\bin\direct\curl-direct.exe=direct | Select-String -Pattern '^LANE'
```

Ожидается `P1-RESULT: ... result=...` сторожа, `helper restarted by the window: True`, `SLCTL: phase=on ...` (по журналу возврата: `stopping`, затем «было включено» — `starting`, `on`), два `LANE-OK`.

- [ ] **Step 10: Закрытие в трей, второй запуск, автозапуск (нужен пользователь)**

Скажите пользователю: «Закройте окно крестиком; потом откройте его щелчком по значку SplitLane в трее; на экране «Подключение» (нажмите плашку Tailscale в шапке) включите «Запускать при входе в Windows»». Затем:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $main = { @(Get-CimInstance Win32_Process -Filter "Name='SplitLane.exe'" | Where-Object { $_.CommandLine -notmatch '--type=' }).Count }; "main processes before: $(& $main)"; Start-Process C:\code\router\app\release\win-unpacked\SplitLane.exe; Start-Sleep -Seconds 5; "main processes after a second start: $(& $main)"; $t0 = Get-Date; $run = $null; while (((Get-Date) - $t0).TotalSeconds -lt 300 -and -not $run) { Start-Sleep -Seconds 3; Set-P1Heartbeat; $run = (Get-ItemProperty HKCU:\Software\Microsoft\Windows\CurrentVersion\Run -ErrorAction SilentlyContinue).PSObject.Properties | Where-Object { "$($_.Value)" -match 'SplitLane\.exe' } | Select-Object -First 1 }; "autostart entry: $([bool]$run) hidden flag: $("$($run.Value)" -match '--hidden')"; & "$P1Bin\slctl.exe" state | Select-Object -Last 1
```

Ожидается `main processes before: 1`, `main processes after a second start: 1` (второй запуск показал первое окно), `autostart entry: True hidden flag: True`, фаза `on`. Скажите пользователю выключить «Запускать при входе в Windows» и проверьте: `(Get-ItemProperty HKCU:\Software\Microsoft\Windows\CurrentVersion\Run).PSObject.Properties | Where-Object { "$($_.Value)" -match 'SplitLane\.exe' }` — пусто.

- [ ] **Step 10a: Проверка глазами (нужен пользователь, без команд)**

Спросите пользователя и запишите ответы: значок SplitLane в трее — круг цвета VPN при включённом делении; окно, суженное до минимума (860×560), с открытой панелью «+ Добавить» — обе дорожки читаются, ничего не налезает; на карточке Пробел начинает перетаскивание с клавиатуры, стрелки двигают, Escape отменяет его и не закрывает открытую панель «+ Добавить»; карточка программы (щелчок по карточке curl-vpn при трафике) показывает соединения и скорость.

- [ ] **Step 11: «Выход» из трея при включённом делении (нужен пользователь)**

Скажите пользователю: «Правой кнопкой на значке SplitLane в трее выберите «Выход»». Затем:

```powershell
. C:\code\router\spike\plan1\p1lib.ps1; if (-not (Assert-P1Armed)) { return }; Set-P1Heartbeat; $t0 = Get-Date; while (((Get-Date) - $t0).TotalSeconds -lt 240 -and (Get-Process SplitLane, splitlane-helper -ErrorAction SilentlyContinue)) { Start-Sleep -Seconds 2; Set-P1Heartbeat }; "left: [$(@(Get-Process SplitLane, splitlane-helper, sing-box -ErrorAction SilentlyContinue | ForEach-Object { $_.Name } | Sort-Object -Unique) -join ',')] seconds=$([int]((Get-Date) - $t0).TotalSeconds)"; Test-P1SystemRestored; Send-P1Guard -Action finish; Test-P1SystemRestored
```

Ожидается `left: []`, `SYSTEM-RESTORED`, `P1-RESULT: state=finished ... system_ok=True`, `SYSTEM-RESTORED`.

- [ ] **Step 12: Записать итог в приложение A и закоммитить**

В абзац «Проверки плана 1B» приложения A (после него — новый абзац «Проверки плана 2 (окно)») добавьте по строке на шаги 4–11 с итогом «подтверждено» или «НЕ подтверждено: …» и числами (секунды установки, включения, перезапуска служебного процесса). Затем:

```powershell
git -C C:\code\router diff -- docs | Select-String -Pattern '\b\d{1,3}(\.\d{1,3}){3}\b' | Where-Object { $_.Line -notmatch '100\.64\.0\.0/10|160\.79\.104\.0/23|1\.1\.1\.1|127\.0\.0\.1' }
git -C C:\code\router diff -- docs | Select-String -Pattern 'login\.tailscale\.com|fd7a:[0-9a-f]|\.ts\.net' | Where-Object { $_.Line -notmatch '\*\.ts\.net' }
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md
git -C C:\code\router commit -m "docs: record the live checks of plan 2 in Appendix A" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Первые две команды должны вывести пустоту.

---

## Что остаётся после плана 2

- Установщик, служба Windows, подпись, обновления, английский интерфейс — этап 2 (раздел 14 спецификации).
- Сутки непрерывной работы и трафик WSL из ручного списка раздела 12 — отдельным наблюдением после плана 2.
