# Code signing policy · Политика подписи

[English](#english) · [Русский](#русский)

## English

Status: SplitLane has applied to the SignPath Foundation for free code signing. Until the application is approved, releases are not signed.

Free code signing provided by SignPath.io, certificate by SignPath Foundation

### What is signed

When signing is on, these files are signed:

- `SplitLane.exe`, the window;
- `splitlane-helper.exe`, the service;
- the installer `SplitLane-Setup-<version>.exe` and its uninstaller.

They are built from the source code in this repository by GitHub Actions (`.github/workflows/build.yml`), and nothing else is signed.

The release also contains files from other projects, which are included unsigned and unchanged: `sing-box.exe` and `libcronet.dll` from the official release of SagerNet. The build checks both against the SHA-256 pinned in `scripts/core.sha256` (see [THIRD-PARTY.md](THIRD-PARTY.md), in Russian).

### Roles

- **Committers and reviewers:** the members of the GitHub organization [splitlane-app](https://github.com/orgs/splitlane-app/people).
- **Approvers:** the owners of that organization.

Every signing request for a release is approved manually by an approver.

### Privacy

SplitLane sends nothing over the network except what the [privacy policy](PRIVACY.md) lists.

## Русский

Состояние: SplitLane подал заявку в SignPath Foundation на бесплатную подпись кода. Пока заявка не одобрена, выпуски не подписаны.

Free code signing provided by SignPath.io, certificate by SignPath Foundation

### Что подписывается

Когда подпись включена, подписываются эти файлы:

- `SplitLane.exe` — окно;
- `splitlane-helper.exe` — служба;
- установщик `SplitLane-Setup-<версия>.exe` и его деинсталлятор.

Их собирает из исходного кода этого репозитория GitHub Actions (`.github/workflows/build.yml`); ничего другого не подписывается.

В выпуск входят и файлы других проектов, они включены без подписи и без изменений: `sing-box.exe` и `libcronet.dll` из официального выпуска SagerNet. Сборка сверяет оба файла с SHA-256, закреплённым в `scripts/core.sha256` (см. [THIRD-PARTY.md](THIRD-PARTY.md)).

### Роли

- **Авторы изменений и рецензенты (committers and reviewers):** участники организации GitHub [splitlane-app](https://github.com/orgs/splitlane-app/people).
- **Утверждающие (approvers):** владельцы этой организации.

Каждый запрос на подпись выпуска утверждает вручную утверждающий.

### Конфиденциальность

SplitLane ничего не отправляет по сети, кроме того, что перечислено в [политике конфиденциальности](PRIVACY.md).
