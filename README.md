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
