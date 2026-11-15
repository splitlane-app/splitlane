# SplitLane, этап 2, часть 3: английский интерфейс — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Окно, меню значка, установщик и деинсталлятор SplitLane говорят по-английски на нерусской Windows; язык можно выбрать в окне.

**Architecture:** Основной процесс разрешает язык (настройка `language` + языки Windows) чистой функцией и кладёт его в `Snapshot.language`; страница берёт строки из словаря этого языка (`ru.ts`, `en.ts` с одинаковыми ключами); основной процесс — из своего двуязычного словаря. Установщик — два языка NSIS с автоматическим выбором и `LangString` для своих текстов.

**Tech Stack:** Electron 45.2.1 (`app.getPreferredSystemLanguages`), React 19, TypeScript 6, Vitest 5, NSIS (electron-builder 26.18.0).

**Spec:** `docs/superpowers/specs/2026-11-15-splitlane-stage2-english-design.md` (далее «спецификация»).

## Global Constraints

- Работа идёт в ветке `stage2-english`. Каждая задача с кодом заканчивается коммитом; сообщение — по-английски, последняя строка `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Новых пакетов нет. Языки — только `ru` и `en`; тип `Lang = 'ru' | 'en'`; настройка `language: 'system' | 'ru' | 'en'` (по умолчанию `system`).
- Разрешение языка: настройка `ru`/`en` — она; `system` — первый элемент `app.getPreferredSystemLanguages()`: начинается с `ru` (без учёта регистра) → `ru`, иначе `en`; пустой список → `en`.
- Строки страницы — только в `app/src/renderer/i18n/ru.ts` и `app/src/renderer/i18n/en.ts` (одинаковые ключи; одинаковые параметры `{…}`); строки основного процесса — в одном словаре основного процесса с двумя языками; в компонентах строк нет.
- Глоссарий английских терминов (единообразно везде): «Через VPN» — "Via VPN"; «Напрямую» — "Direct"; «основная» (метка дорожки) — "main"; «сделать основной» — "make main"; «Пустить через VPN» — "Send via VPN"; «Пустить напрямую» — "Send direct"; «Открепить» — "Unpin"; «тихо» — "quiet"; «Включено»/«Выключено» — "On"/"Off"; «Подключение» (экран) — "Connection"; «служба», «служебный процесс» — "service"; «деление» — "split routing"; «exit node» — "exit node"; «Первая настройка» — "First-time setup"; «Установить службу» — "Install the service"; «Обновить службу» — "Update the service"; «Проверить обновления» — "Check for updates"; «Доступна версия {v}» — "Version {v} is available"; «Обновить» (обновление) — "Update"; «Что нового» — "What's new"; «Позже» — "Later"; «Закрыть» — "Close"; «Повторить» — "Retry"; «Отменить» — "Cancel"; «Язык» — "Language"; «Как в Windows» — "Same as Windows". Тон — короткий, нейтральный, как в системных приложениях Windows; без восклицательных знаков.
- Названия языков в выборе — всегда на своём языке: «Русский», "English"; пункт «Как в Windows» / "Same as Windows" — на текущем языке.
- Модульные тесты не трогают систему. `pnpm dev` / `electron-vite dev` не запускаются; установщик и `SplitLane.exe` не запускаются; `scripts\test-installer.ps1` на этом ПК — только до отказа.
- Личные данные: нигде нет имён устройств и сети, адресов, SID, учётной записи владельца.
- PowerShell — только ASCII; `app/build/installer.nsh` — UTF-8 с BOM.
- Полная проверка (из `C:\code\router\app`): `pnpm typecheck; pnpm test; pnpm build`; установщик — `scripts\build-installer.ps1` → `BUILD-OK`; CI — зелёный запуск.

## Review Focus

1. В английском словаре пропущен параметр (`{v}`) или лишний — строка покажется с `{v}` или `t()` упадёт. Ожидание: тест сравнивает параметры каждой пары строк. Задача 2: `en and ru have the same placeholders`.
2. Язык Windows — `ru-RU`, `ru`, `RU`, `uk-UA`, `be-BY`, пустой список. Ожидание: русский только для `ru*`, иначе английский. Задача 1: `resolveLanguage` таблица.
3. Пользователь меняет язык, когда открыт экран «Подключение», плашка обновления или контекстное меню. Ожидание: всё на новом языке без перезапуска. Задача 2: `switching the language re-renders open screens`.
4. Числа скорости в английском — с точкой (`1.5 MB/s`), в русском — с запятой. Задача 2: `formatSpeed per language`.
5. Причина обновления, которой нет в словаре (новый текст из механизма). Ожидание: показана как есть, без падения. Задача 2: `unknown update reasons pass through`.

---

## Как читать этот план

**Что уже есть.** Страница: `t(key, params)` в `app/src/renderer/i18n/t.ts` берёт строки только из `ru.ts` (≈220 ключей), `Key` — тип ключей; `formatSpeed` в `app/src/renderer/model/format.ts` ставит запятую; `<html lang="ru">` в `app/src/renderer/index.html`. Основной процесс: `MAIN_TEXT` в `app/src/main/tray.ts` (меню значка, фильтры диалогов); сортировка `localeCompare(..., 'ru')` в `app/src/main/ipc.ts`, `app/src/main/programs.ts`, `app/src/renderer/model/board.ts`. Настройки — `app/src/shared/settings.ts`, патч страницы проверяет `validateSettingsPatch` в `app/src/main/ipc.ts`. Причины обновлений — английские строки из `app/src/main/updater.ts`. Установщик — `app/build/installer.nsh`, `installerLanguages: [ru_RU]`, `language: 1049` в `app/electron-builder.yml`.

**Порядок задач.** 1 — язык в основном процессе (настройка, разрешение, снимок, словарь основного процесса, меню значка); 2 — страница (английский словарь, `t`, форматирование, выбор языка, причины обновлений); 3 — установщик; 4 — живая проверка.

### Где код задач

| Задача | Коммиты |
|---|---|
| 1 | 4b8b190 |
| 2 | c659db8, a1f8abc |
| 3 | a7f1cbc |
| итоговая рецензия ветки | e6375a0 |

---

### Task 1: Язык в основном процессе

**Files:** `app/src/shared/i18n.ts` (новый: `Lang`, `LanguageSetting`, `resolveLanguage`) + тест; `app/src/shared/settings.ts` (+ тест); `app/src/shared/ipc.ts` (`Snapshot.language: Lang`); `app/src/main/ipc.ts` (+ тест: патч `language`); `app/src/main/tray.ts` (словарь основного процесса); `app/src/main/index.ts`; `app/src/main/programs.ts`, `app/src/main/ipc.ts` (сортировка по языку).

**Interfaces (produces):**
```ts
export type Lang = 'ru' | 'en'
export type LanguageSetting = 'system' | Lang
export function resolveLanguage(setting: LanguageSetting, system: readonly string[]): Lang
export function mainText(lang: Lang): MainText   // в tray.ts: MAIN_TEXT становится { ru: {...}, en: {...} }
```
`WindowSettings.language: LanguageSetting` (по умолчанию `system`; неизвестное значение → `system`); `validateSettingsPatch` принимает `language`.

**Поведение:** `index.ts` разрешает язык при старте и при каждой смене настройки (`resolveLanguage(settings.language, app.getPreferredSystemLanguages())`), кладёт в снимок, пересобирает меню значка и подсказку; диалоги берут фильтры из `mainText(lang)`; сортировка программ в основном процессе — `localeCompare(a, b, lang)`. Английские тексты меню: "Open", "Settings…", "Turn on", "Turn off", "Check for updates", "Quit", фильтры "Programs", "ZIP archive".

**Тесты:** `resolveLanguage` (таблица: `ru-RU`, `ru`, `RU-ru`, `en-US`, `uk-UA`, `be-BY`, `de-DE`, пусто; настройка `ru`/`en` поверх системы); `parseSettings reads language with a default`; `validateSettingsPatch accepts language and refuses other values`; `tray menu texts per language` (модель меню или словарь); снимок содержит язык.

- [ ] Шаги: тесты → провал → реализация → полная проверка → коммит `feat(app): window language setting resolved in the main process`.

### Task 2: Страница на двух языках

**Files:** `app/src/renderer/i18n/en.ts` (новый), `ru.ts` (если нужны новые ключи), `t.ts` (+ тест), `app/src/renderer/model/format.ts` (+ тест), `app/src/renderer/model/board.ts` (сортировка), `app/src/renderer/model/update.ts` (причины), `app/src/renderer/screens/ConnectScreen.tsx` (выбор языка, + тест), хранилище страницы (язык из снимка), `app/src/renderer/index.html` / корень приложения (`document.documentElement.lang`).

**Interfaces:** `export const en: Record<Key, string>`; `t(key, params)` — строка текущего языка (язык задаётся из снимка; функция установки языка — например `setLang(lang: Lang)` в `t.ts`, вызывается хранилищем при новом снимке); `formatSpeed(bps)` — с разделителем текущего языка; `translateUpdateReason(reason: string): string` — известные английские причины из `updater.ts` (`the release is not available`, `the checksum file is larger than 1 KB`, `the file is larger than the release says`, `the file is larger than 400 MB`, `the file does not match its checksum`, `the release answer is larger than 1 MB`, `HTTP 403 (GitHub request limit?)`, `HTTP <n>`, `the installer did not start the update`, `redirect to a host that is not allowed`, `too many redirects`, `the address is not allowed` и прочие, найденные в `updater.ts` и `netHop.ts`) → ключи `update.reason.*` словаря; остальное — как есть.

**Поведение:** выбор «Язык» на «Подключении»: «Как в Windows» / "Same as Windows", «Русский», "English" → `setSettings({ language })`; страница переходит на новый язык сразу после снимка; `lang` документа = текущий язык. Все строки `ru.ts` переведены на английский по глоссарию; тексты причин деления (18 кодов), ошибок (9 кодов), отказов правил — тоже.

**Тесты:** `en has every key of ru` (тип + тест), `en and ru have the same placeholders`, `t uses the current language`, `formatSpeed per language`, `switching the language re-renders open screens` (например, «Подключение» и плашка обновления), `the language selector saves the setting`, `known update reasons are translated`, `unknown update reasons pass through`, `document lang follows the language`.

- [ ] Шаги: тесты → провал → реализация → полная проверка → коммит `feat(app): English window texts and the language selector`.

### Task 3: Установщик на двух языках

**Files:** `app/electron-builder.yml`, `app/build/installer.nsh`.

**Поведение:** `nsis.installerLanguages: [ru_RU, en_US]`, `multiLanguageInstaller: true`, `displayLanguageSelector: false` (язык — по языку Windows; проверить, что electron-builder 26.18 так и выбирает, и записать в отчёт, по какому языку — интерфейса или региональному); `language` не задан или английский по умолчанию — выбрать так, чтобы на нерусской Windows был английский. Все свои тексты `installer.nsh` — через `LangString <name> ${LANG_RUSSIAN} "…"` и `${LANG_ENGLISH} "…"` (английские — по глоссарию плана); сообщения используют `$(name)`. Сборка с `-WX` чистая в обеих сборках; BOM сохранён.

**Проверка задачи:** `scripts\build-installer.ps1` → `BUILD-OK`; в собранном установщике есть оба языка (makensis-лог или распаковка); отправка ветки, CI зелёный (тихий режим не меняется).

- [ ] Шаги: реализация → сборка → CI → коммит `feat(app): the installer and uninstaller in Russian and English`.

### Task 4: Живая проверка (нужен пользователь, без сторожа)

Исполняет основная сессия. Сеть не меняется (деление не трогаем), сторож не нужен.

1. Сборка `scripts\build-installer.ps1 -Version 0.2.95`; пользователь ставит её поверх 0.2.93 (UAC; на русской Windows мастер русский).
2. Пользователь переключает «Язык» на "English": доска, «Подключение» (Connection), плашки, меню значка, контекстное меню карточки — по-английски; снимки экрана.
3. Обратно «Русский» и «Как в Windows» — всё по-русски.
4. Итоги — «Приложение A» спецификации; коммит `docs: record the live check of stage 2 part 3`.

## Что остаётся после части 3

Часть 4 — открытый бета-выпуск (английский README и страница выпуска, проверка английского установщика на чистой виртуальной машине), часть 5 — подпись.
