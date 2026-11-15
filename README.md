# SplitLane

[Русская версия](README.ru.md)

SplitLane is a Windows app for split routing by program. It puts programs on two lanes, **Via VPN** and **Direct**: programs on the VPN lane go through your Tailscale exit node, everything else goes directly through your normal connection. It is for people who use a Tailscale exit node as their VPN and want only some programs to use it, for example because some websites are slow or don't work through the VPN. You arrange programs by dragging them from one lane to the other.

This is a beta. It works with Tailscale only.

## How it works

- The SplitLane window runs without administrator rights. It sends commands to the SplitLane service, a background process with administrator rights that Task Scheduler starts when you sign in to Windows.
- The service runs a bundled network core (sing-box 1.14.4, unmodified). The core captures traffic with a virtual adapter and sends each connection to a lane by the path of the program that opened it.
- The VPN lane goes through an embedded Tailscale node inside the core. It is a separate device in your tailnet, named `<PC name>-splitlane`, and it uses the exit node you choose. The Direct lane uses your normal connection.
- One lane is the main lane. Programs you haven't pinned follow it; pinned programs stay on their lane.
- If the Tailscale app for Windows is installed and uses an exit node, SplitLane turns that exit node off while split routing is on and turns it back on when you turn split routing off. After a crash or a restart of Windows, it turns it back on the next time the service starts.

## Requirements

- Windows 10 or 11, x64.
- A Tailscale account (tailnet) with an exit node you can use; the exit node must reach the internet from a different address than this PC.
- The Tailscale app for Windows is optional: if it is installed and uses an exit node, SplitLane turns that exit node off while split routing is on and turns it back on afterwards.
- A Windows account with administrator rights. You install and use SplitLane under the same account: if you confirm the Windows prompt (UAC) with a different account, the installation stops.

## Install

1. Download `SplitLane-Setup-<version>.exe` and `SplitLane-Setup-<version>.exe.sha256` from [Releases](https://github.com/splitlane-app/splitlane/releases).
2. Verify the download. In PowerShell, in the folder with both files (replace `<version>`):

   ```powershell
   $file = 'SplitLane-Setup-<version>.exe'
   $expected = (-split (Get-Content "$file.sha256" -Raw))[0]
   (Get-FileHash $file -Algorithm SHA256).Hash -eq $expected
   ```

   `True` means the file matches the checksum of the release.
3. Run the installer. It isn't signed yet, so SmartScreen shows "Windows protected your PC": click **More info**, then **Run anyway**.
4. The installer asks for administrator rights once, installs SplitLane for all users in `%ProgramFiles%\SplitLane` (the folder can't be changed) and sets up the service for the Windows user who is signed in. The last page has a **Run SplitLane** checkbox.

The installer is in English or Russian, following the language of Windows.

Installing over an older version works the same way: the installer first stops the service of the old version and turns the exit node of the Tailscale app back on, if it used one. Your settings and the Tailscale login are kept. Silent install: `SplitLane-Setup-<version>.exe /S` (the window doesn't start; a non-zero exit code means the installation failed).

winget: coming soon.

## First start

1. If the window shows **First-time setup**, click **Install the service** and confirm the Windows prompt. The installer normally does this for you, so this screen appears only when the service needs to be repaired.
2. On **Connect a VPN**, click **Log in to Tailscale**. SplitLane's window opens the login page in your browser: log in to the tailnet that has your exit node. This logs in SplitLane's embedded node, a new device `<PC name>-splitlane`; the Tailscale app isn't involved. If your tailnet requires device approval, approve the device in the Tailscale admin console; the window continues by itself.
3. Choose an exit node, then click **Turn on split routing**.
4. Drag programs onto the lanes. You can move a card from one lane to the other, drop an `.exe` file or a shortcut from Explorer onto a lane, or click **+ Add** to pick a program that is online now, an installed program or a file. The same actions are in the context menu of a card (**Send via VPN**, **Send direct**, **Unpin**) and work from the keyboard.

A pinned program stays on its lane; other programs follow the main lane (**make main** on the other lane switches it). By default a program means its whole folder, so its updaters and helper processes go the same way; you can change this in the program card. **Settings and connection** in the header (or **Settings…** in the tray menu) opens the **Connection** screen: the exit node, logging out of Tailscale, the language and the settings **Start when I sign in to Windows**, **Turn on split routing at startup** and **Check for updates automatically**.

Closing the window keeps SplitLane in the tray. **Quit** in the tray menu turns split routing off and stops the service.

## Updates

SplitLane checks the latest release on [GitHub Releases](https://github.com/splitlane-app/splitlane/releases) 30 seconds after the window starts and then every 24 hours while it runs, when **Check for updates automatically** is on (it is on by default). You can check at any time with **Check for updates** in the tray menu or on the **Connection** screen.

When a newer version is out, a banner offers **Update**. SplitLane downloads the installer, checks its SHA-256 against the `.sha256` file of the same release and runs the installer silently; Windows asks you to confirm. The installer stops the service, turns the exit node of the Tailscale app back on if it used one, installs the new version and starts the window again. Split routing stays on or off, as it was. The window then shows "SplitLane was updated to version X", or the reason the update wasn't installed.

The SHA-256 check confirms that the download is intact, not who made it: the installer isn't signed yet.

## Uninstall

Open **Settings → Apps**, find SplitLane and click **Uninstall**, or run `%ProgramFiles%\SplitLane\Uninstall SplitLane.exe`. The uninstaller turns split routing off and, if the Tailscale app used an exit node, turns it back on before it removes anything.

The checkbox **Remove the settings and the Tailscale login** (off by default) also deletes `%ProgramData%\SplitLane` and your `%AppData%\SplitLane`. Without it, they are kept for the next installation.

Either way, the device `<PC name>-splitlane` stays in your tailnet. Remove it in the [Tailscale admin console](https://login.tailscale.com/admin/machines).

Silent uninstall: `"%ProgramFiles%\SplitLane\Uninstall SplitLane.exe" /S` keeps the data, `/S /PURGE` deletes it. The NSIS uninstaller copies itself to a temporary folder and returns at once; it finishes a few seconds later, when the `%ProgramFiles%\SplitLane` folder is gone.

## Privacy

SplitLane has no telemetry and no accounts of its own. In short:

- The window checks GitHub for updates. You can turn the automatic check off.
- The embedded Tailscale node talks to Tailscale's servers like any Tailscale device.
- While split routing is on, the service checks both lanes with `api.ipify.org`, and website names are resolved through encrypted DNS at `1.1.1.1` over the VPN lane.
- Settings and logs stay on your PC. A diagnostics archive leaves it only if you share it.

Details, and how to delete everything: [PRIVACY.md](PRIVACY.md).

## Known limitations of this beta

- The installer and the programs aren't signed. SmartScreen warns about the downloaded installer, and the update check verifies integrity, not authenticity.
- One Windows user per PC. Split routing applies to the whole PC, so programs in other Windows sessions follow the same lanes, but only the user who installed SplitLane can control it.
- Tailscale only. "Key or subscription" and "Configuration file" are shown in the app as coming soon.
- Installation only in `%ProgramFiles%\SplitLane`.
- The embedded node is a separate device in your tailnet with its own login expiry. When it expires, SplitLane asks you to log in again.
- DNS is shared by both lanes. While the VPN lane works, names are resolved through it, so content delivery networks may give programs on the Direct lane servers near the exit node. When the VPN server is unreachable or **All direct** is on, all names go to your ISP's DNS.
- If the Tailscale app manages DNS (its default), MagicDNS names don't resolve while split routing is on, and if your tailnet overrides DNS for all names, split routing doesn't turn on. It has been tested only with DNS management turned off in the Tailscale app.
- WSL and virtual machines follow the main lane. So do connections whose program the core can't identify (for example `ping`).
- Programs that go online through shared system processes (for example, the Edge web component) can't be separated from each other.
- Traffic to private, local and Tailscale addresses always goes directly.
- IPv6 to the internet doesn't work while split routing is on.
- There is no kill switch. When the core crashes, traffic goes directly until SplitLane turns the exit node of the Tailscale app back on (about a second), then through that exit node until the core restarts. When SplitLane restarts the core itself (after a network change, when you change the exit node), traffic goes through the exit node of the Tailscale app meanwhile. If the Tailscale app isn't installed or had no exit node selected, traffic goes directly in both cases.
- An exit node that reaches the internet from the same address as this PC can't be used.
- If the Tailscale app used automatic exit node selection, a specific exit node is selected there after split routing is turned off.
- If the Tailscale app is installed, SplitLane reads one of its settings through a debug command whose format can change. After you update Tailscale, turn split routing on and off once.
- Messages of the service (its log, the reasons the installer shows when it stops) are in English in both languages.

## Building from source

You need Windows 10/11 x64 with Windows PowerShell 5.1, Go 1.27.4 (from `%LOCALAPPDATA%\Programs\go\bin`, otherwise from `PATH`), Node.js 24 and pnpm 12.9.1.

1. Get the network core. sing-box 1.14.4 isn't stored in git: the script downloads the official archive and checks its SHA-256 against `scripts/core.sha256`. Result: `.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe`.

   ```powershell
   powershell -ExecutionPolicy Bypass -File scripts\fetch-core.ps1
   ```

2. Build the installer:

   ```powershell
   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1
   ```

   The steps: `scripts\build-helper.ps1` (the service, into `dist\helper`), then in `app`: `pnpm install --frozen-lockfile`, the Electron binary, type checking, tests, the window build and `electron-builder --win nsis`. Result: `app\release\SplitLane-Setup-<version>.exe` and next to it `SplitLane-Setup-<version>.exe.sha256` (one line: `<sha256>  SplitLane-Setup-<version>.exe`). The last line of the output is `BUILD-OK <path of the installer>`. The version is `version` in `app/package.json`; the service gets the same version. The installer isn't signed.

To build with another version (three numbers `x.y.z`), for example to test updates:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1 -Version 0.3.90
```

The service (`scripts\build-helper.ps1 -Version <x.y.z>`, which you can also run on its own), the window, `SplitLane.exe` and the installer with its name and `.sha256` get this version; `app/package.json` doesn't change.

## License

SplitLane is licensed under GPL-3.0-or-later (see [LICENSE](LICENSE)). It ships the unmodified sing-box core and other third-party components under their own licenses, listed with links to their source code in [THIRD-PARTY.md](THIRD-PARTY.md) (in Russian). SplitLane is not affiliated with sing-box or Tailscale.

Security issues: see [SECURITY.md](SECURITY.md). Other problems and questions: [GitHub issues](https://github.com/splitlane-app/splitlane/issues).

## Development

- **Layout.** `helper/`: the service `splitlane-helper.exe` (Go), the only part with administrator rights. `helper/cmd/slctl`: a test client for the service's pipe. `helper/cmd/slguard`: a guard for live checks on a PC during development, not shipped. `app/`: the window (Electron, React, TypeScript); `app/build/installer.nsh`: additions to the installer. `scripts/`: the core download, the builds and the installer check. `docs/`: specifications and plans, in Russian. `.github/workflows/build.yml`: the CI build.
- **The service alone** (Go 1.27.4, no CGO):

  ```powershell
  cd helper
  $env:CGO_ENABLED = '0'
  go test ./...
  go build -ldflags "-H=windowsgui" -o bin\splitlane-helper.exe .\cmd\splitlane-helper
  go build -o bin\slctl.exe .\cmd\slctl
  go build -o bin\slguard.exe .\cmd\slguard
  ```

  To check the generated core configs with the real core (it only reads them), set `SPLITLANE_CORE` to the full path of `sing-box.exe` before `go test ./...`.
- **Unpacked window.** `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-app.ps1` builds `app\release\win-unpacked\SplitLane.exe` with the six service files in `helper\` next to it, as the installer lays them out (last line `BUILD-OK app\release\win-unpacked`). It is for development only: the service works only from `%ProgramFiles%\SplitLane\helper`, and a window outside `%ProgramFiles%\SplitLane` says on **First-time setup** that SplitLane has to be installed with the installer.
- **Dev mode.** A service built from source runs outside the installed location only with `--dev`, from an elevated process, with absolute paths:

  ```text
  splitlane-helper.exe --dev --data-dir <repo>\.dev\data --core <repo>\.cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe
  ```

  `.dev\` isn't committed. In dev mode the service doesn't check which program connects to its pipe, so `slctl` and the scenario check `node scripts/scenario.mjs` work against it.
- **Update feed for tests.** `SPLITLANE_UPDATE_FEED=http://127.0.0.1:<port>` makes the window take releases from a local server instead of GitHub.
- **CI.** `.github/workflows/build.yml` runs on every push to any branch, on pull requests, on `v*` tags and by hand (Actions → build → Run workflow), on `windows-latest`: the core with its SHA-256 checked, `go vet` and `go test` of the service, `scripts\build-installer.ps1`, then `scripts\test-installer.ps1` and the artifact `SplitLane-Setup-<version>` (the installer and its `.sha256`, kept 2 days, not made for pull requests). After a failure there is a small `test-installer-logs` artifact. The run summary shows the SHA-256 of the installer and the result of the check. On a tag `v<version>`, which must match `version` in `app/package.json`, the `release` job creates a draft release "SplitLane <version>" with the installer and its `.sha256`. To download an artifact: `gh run download <run id> -n SplitLane-Setup-<version>`.
- **Installer check.** `scripts\test-installer.ps1 -Installer <path>` installs, reinstalls and uninstalls SplitLane silently and checks files, the task, the access list of the data folder, the Electron fuses and the result file `%ProgramData%\SplitLane\export\last-install.txt`. The last line is `TEST-OK`, `TEST-FAIL <step>: <what>` or `TEST-REFUSED: <reason>`. It changes the system and is meant for a CI runner: on a PC that already has SplitLane or Tailscale, it refuses without changing anything.
