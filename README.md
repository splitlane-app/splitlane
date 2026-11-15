# SplitLane

SplitLane — приложение для Windows 10/11, в котором программы раскладываются по двум дорожкам: «Через VPN» и «Напрямую». Этап 1 работает с Tailscale и exit node. Спецификация: `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md`.

Лицензия: GPL-3.0-or-later (файл `LICENSE`). Сторонние компоненты перечислены в `THIRD-PARTY.md`.

## Установка

1. Скачайте `SplitLane-Setup-<версия>.exe` и `SplitLane-Setup-<версия>.exe.sha256` со страницы выпусков (Releases) репозитория или из артефакта запуска GitHub Actions (раздел «Проверка на GitHub Actions»). Сверка: `(Get-FileHash SplitLane-Setup-<версия>.exe).Hash` совпадает с первым словом файла `.sha256` (без учёта регистра).
2. Запустите установщик. Он пока не подписан, поэтому SmartScreen показывает «Система Windows защитила ваш компьютер»: нажмите «Подробнее» → «Выполнить в любом случае».
3. Установщик один раз просит права администратора (UAC), ставит SplitLane для всех пользователей в `%ProgramFiles%\SplitLane` (каталог не выбирается) и настраивает служебный процесс для пользователя, который вошёл в Windows. На последней странице галочка «Запустить SplitLane».

Установка поверх прежней версии (в том числе поверх установки этапа 1) идёт так же: установщик сначала останавливает прежний служебный процесс с возвратом exit node, настройки и вход в Tailscale остаются. Тихая установка: `SplitLane-Setup-<версия>.exe /S` (окно не запускается; отказ — ненулевой код выхода).

## Удаление

«Параметры» → «Приложения» → SplitLane → «Удалить», или `%ProgramFiles%\SplitLane\Uninstall SplitLane.exe`. Галочка «Удалить настройки и вход в Tailscale» (по умолчанию выключена) удаляет ещё `%ProgramData%\SplitLane` и `%AppData%\SplitLane`; устройство `<имя ПК>-splitlane` при этом остаётся в сети Tailscale — его удаляют в консоли администратора Tailscale. Без галочки настройки остаются для следующей установки.

Тихое удаление: `"%ProgramFiles%\SplitLane\Uninstall SplitLane.exe" /S` — данные остаются, с `/S /PURGE` — удаляются. Деинсталлятор NSIS копирует себя во временную папку и сразу возвращает управление; удаление идёт ещё несколько секунд, пока не исчезнет папка `%ProgramFiles%\SplitLane`.

## Обновления

Окно проверяет последний выпуск на GitHub Releases репозитория `splitlane-app/splitlane`: через 30 секунд после запуска и затем раз в сутки (настройка «Проверять обновления»), а также сразу по пункту «Проверить обновления» в меню значка и кнопке на экране «Подключение». Если версия выпуска новее, плашка предлагает «Обновить»: окно скачивает `SplitLane-Setup-<версия>.exe`, сверяет его SHA-256 с файлом `.sha256` того же выпуска и запускает установщик тихо (`/S --force-run`) с подтверждением Windows (UAC). Установщик останавливает служебный процесс с возвратом exit node, ставит новую версию и запускает окно, деление остаётся, как было. Итог служебный процесс новой версии (его подготовка и настройка, которые запускает установщик) записывает в `%ProgramData%\SplitLane\export\last-install.txt`, только если папка данных создана администратором; окно после перезапуска показывает «SplitLane обновлён до <версия>» или причину отказа. Сверка хеша проверяет целостность, не подлинность: установщик пока не подписан.

Пока репозиторий закрыт, обновления не находятся: GitHub без входа не отдаёт его выпуски.

## Состав

- `helper/` — служебный процесс `splitlane-helper.exe` (Go), единственная часть с правами администратора.
- `helper/cmd/slctl` — тестовый клиент канала служебного процесса.
- `helper/cmd/slguard` — сторож для проверок на ПК во время разработки; в поставку не входит.
- `scripts/` — получение ядра с проверкой контрольной суммы, сборка и проверка установщика.
- `docs/` — спецификации и планы.
- `app/` — окно (Electron), см. раздел «Окно»; `app/build/installer.nsh` — дополнения установщика.
- `.github/workflows/build.yml` — сборка и проверка на GitHub Actions.

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

## Окно

Сборка окна (Electron) вместе со служебным процессом, одной командой (нужны Go, ядро из `scripts\fetch-core.ps1`, Node и pnpm):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-app.ps1
```

Скрипт собирает `dist\helper`, ставит зависимости, запускает проверку типов и тесты и пакует окно. Итог: `app\release\win-unpacked\SplitLane.exe`, шесть файлов служебного процесса лежат в `helper\` рядом с ним, как их раскладывает установщик. Последняя строка вывода: `BUILD-OK app\release\win-unpacked`.

Папка `win-unpacked` — только для разработки: служебный процесс работает только из `%ProgramFiles%\SplitLane\helper`, и окно не из `%ProgramFiles%\SplitLane` на экране «Первая настройка» говорит, что SplitLane нужно установить установщиком (разделы «Установка» и «Сборка»).

## Сборка

Установщик собирается одной командой. Нужны Go 1.27.4 (в `%LOCALAPPDATA%\Programs\go\bin`, иначе из PATH), Node.js 24, pnpm 12.9.1 и ядро:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\fetch-core.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1
```

Шаги: `scripts\build-helper.ps1` (`dist\helper`), затем в `app`: `pnpm install --frozen-lockfile`, двоичный файл Electron, проверка типов, тесты, сборка окна и `electron-builder --win nsis`. Итог: `app\release\SplitLane-Setup-<версия>.exe` и рядом `SplitLane-Setup-<версия>.exe.sha256` (строка `<sha256>  SplitLane-Setup-<версия>.exe`); последняя строка вывода `BUILD-OK <путь установщика>`. Версия — `version` в `app/package.json`, её же получает служебный процесс. Установщик не подписан.

Сборка для проверки с другой версией (три числа `x.y.z`), например для проверки обновлений:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1 -Version 0.2.90
```

Эту версию получают служебный процесс (`scripts\build-helper.ps1 -Version <x.y.z>`, его можно запустить и отдельно), окно, `SplitLane.exe` и установщик с его именем и `.sha256`; `app/package.json` не меняется.

## Проверка на GitHub Actions

Рабочий процесс `.github/workflows/build.yml` запускается на каждую отправку в любую ветку, на pull request, на метку `v*` и вручную (Actions → build → Run workflow). Задача `build` на бегуне `windows-latest`: ядро со сверкой SHA-256, `go vet` и `go test` служебного процесса, `scripts\build-installer.ps1`, затем `scripts\test-installer.ps1` и артефакт `SplitLane-Setup-<версия>` (установщик и `.sha256`). Место для артефактов закрытого репозитория — 500 МБ, установщик — около 160 МБ, поэтому артефакт хранится 2 дня и на pull request не создаётся (его создаёт запуск по отправке ветки). При сбое — небольшой артефакт `test-installer-logs` (вывод диагностических запусков служебного процесса и журналы `%ProgramData%\SplitLane\logs`, тоже 2 дня). В сводке запуска — SHA-256 установщика и итог проверки.

`scripts\test-installer.ps1 -Installer <путь>` на бегуне (администратор, без Tailscale и без SplitLane): тихая установка поверх раскладки этапа 1, затем файлы, задача «SplitLane Helper» и её команда, `setup.json`, список доступа `%ProgramData%\SplitLane`, предохранители Electron и сведения о версии `SplitLane.exe`; повторная установка поверх (настройки остаются); тихое удаление без `/PURGE` (данные остаются) и с `/PURGE` (данные удалены); после каждой успешной установки — файл итога `%ProgramData%\SplitLane\export\last-install.txt` с версией установщика и `ok` (отказы на бегуне не проверяются). Служебный процесс не запускается, сеть не трогается. Последняя строка: `TEST-OK`, `TEST-FAIL <шаг>: <что>` или `TEST-REFUSED: <причина>`. Скрипт меняет систему, поэтому на ПК, где уже есть SplitLane или Tailscale, он отказывает, ничего не меняя; на своём ПК его не запускают.

На метке `v<версия>` (должна совпадать с `version` в `app/package.json`, иначе сборка падает) задача `release` создаёт черновик выпуска «SplitLane <версия>» с установщиком и `.sha256`.

Скачать артефакт запуска: `gh run download <id запуска> -n SplitLane-Setup-<версия>`.

## Режим разработки

Служебный процесс этой сборки запускается только с `--dev`, из процесса с правами администратора:

```text
splitlane-helper.exe --dev --data-dir C:\code\router\.dev\data --core C:\code\router\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe
```

Каталог `.dev\` в git не попадает.
