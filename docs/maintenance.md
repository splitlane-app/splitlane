# Keeping SplitLane up to date

Everything SplitLane depends on is pinned to an exact version, most of it with a SHA-256, so nothing changes until a release of SplitLane changes it. A new sing-box upstream changes nothing for users: the core ships inside the installer, and a VPS keeps the version the kit installed. This page says how we learn that something is out and what to do about it.

## How we learn

- **Dependabot** (`.github/dependabot.yml`) opens pull requests every Monday for the Go modules of the service (`helper/`), the npm packages of the window (`app/`), and monthly for the actions of CI. Minor and patch updates come as one grouped PR per ecosystem, a major alone. Security updates come at once, without waiting for Monday. CI (`build.yml`) checks every PR.
- **upstream** (`.github/workflows/upstream.yml`, `scripts/upstream-check.sh`) runs every Monday and opens one issue per finding: a newer sing-box release, a newer Go patch release of the pinned minor or that minor out of support, the pinned Electron major out of support. It never opens the same title twice. Run it by hand under Actions → upstream, or locally: `bash scripts/upstream-check.sh --dry-run`. GitHub turns a scheduled workflow off after 60 days without activity in the repository; turn it back on under Actions → upstream.

## sing-box

The core is pinned in these places; after a change, `git grep -n <old version>` must find only history (release notes, plans, specs):

| What | Where |
|---|---|
| Version and SHA-256 of `sing-box.exe` | `helper/internal/singbox/driver.go` (`CoreVersion`, `CoreSHA256`) |
| SHA-256 of `libcronet.dll` | `helper/internal/install/install.go` (`CronetSHA256`) |
| The archive, the exe and the dll for the build | `scripts/core.sha256`, `scripts/fetch-core.ps1`, `scripts/build-helper.ps1`, `.github/workflows/build.yml` (`SPLITLANE_CORE`) |
| The server kit (Linux amd64 and arm64) | `scripts/server/install-singbox-server.sh` (`SINGBOX_VERSION`, `SHA256_AMD64`, `SHA256_ARM64`) and its copy `helper/internal/serversetup/kit/` (a test fails if the two differ) |
| Texts | `THIRD-PARTY.md`, `README.md`, `README.ru.md`, `scripts/server/README*.md`, the demo and test fixtures |

**A patch release** of the pinned minor (1.14.x): read the [changelog](https://sing-box.sagernet.org/changelog/). Take it when it fixes something SplitLane uses (security, Tailscale, a protocol, Windows, TUN) or every few months anyway, so the embedded Tailscale client doesn't grow old. Download the official archives (`windows-amd64.zip`, `linux-amd64.tar.gz`, `linux-arm64.tar.gz`), compute the SHA-256 of the archives and of the files the table names, change every place, then:

1. `go test ./...` in `helper` with `SPLITLANE_CORE` set to the new `sing-box.exe` (the real core reads the generated configs), `pnpm --dir app test`;
2. the kit on real systems in containers: `scripts/server/e2e/run-e2e.ps1` (see `scripts/server/README.md`);
3. a live check on a Windows PC with each way to connect (Tailscale exit node, a share link, a WireGuard file, a VPS) and both lanes;
4. a release with notes (`docs/release-notes/`). Users get it with **Check for updates**. A VPS gets the new core when its owner runs **Your own VPS** again on the same server: the kit replaces sing-box and keeps the keys.

**A new minor** (1.15 and later) may change the config format: fields are first deprecated, then removed. Read the [deprecated list](https://sing-box.sagernet.org/deprecated/) and the [migration guide](https://sing-box.sagernet.org/migration/), then check what the service relies on: the config builder `helper/internal/coreconfig` (its header lists the facts about the core it depends on: tags, log lines, the Clash API) and `helper/internal/singbox`. That is a plan of its own, with the full live check; there is no hurry while the pinned minor still gets fixes.

## Go

CI builds the service with the Go of `go-version` in `.github/workflows/build.yml`; `README.md` (Development) names it too. A patch release carries security fixes of the standard library: change both places and let CI build. When the pinned minor goes out of support, move to a supported minor the same way and check `go vet` and the tests; raise the `go` line of `helper/go.mod` only when a module needs it.

## Electron and the npm packages

Dependabot's grouped minor/patch PR is merged when CI is green. A major update of Electron comes alone: read its breaking changes, build the window and try it (the tray, the update banner, drag and drop, the Connection screen). When the upstream check reports the pinned Electron major out of support, that update is due: an unsupported Electron gets no Chromium security fixes.

## Security alerts

A Dependabot alert or a security release of a pinned project goes before other work: a fixed release of SplitLane, with a line about it in the release notes.
