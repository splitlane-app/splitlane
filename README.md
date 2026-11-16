# SplitLane

[Русская версия](README.ru.md)

SplitLane is a Windows app for split routing by program. It puts programs on two lanes, **Via VPN** and **Direct**: programs on the VPN lane go through your VPN, everything else goes directly through your normal connection. The VPN is a Tailscale exit node, a server from a key (a share link), a subscription or a WireGuard file, or a VPS you rent, which SplitLane sets up itself. It is for people who want only some programs to use the VPN, for example because some websites are slow or don't work through it. You arrange programs by dragging them from one lane to the other.

This is a beta.

## How it works

- The SplitLane window runs without administrator rights. It sends commands to the SplitLane service, a background process with administrator rights that Task Scheduler starts when you sign in to Windows.
- The service runs a bundled network core (sing-box 1.14.4, unmodified). The core captures traffic with a virtual adapter and sends each connection to a lane by the path of the program that opened it.
- With Tailscale, the VPN lane goes through an embedded Tailscale node inside the core. It is a separate device in your tailnet, named `<PC name>-splitlane`, and it uses the exit node you choose. With a key, a subscription or a WireGuard file, the core connects to the server you chose itself, without a Tailscale node. The Direct lane uses your normal connection.
- One lane is the main lane. Programs you haven't pinned follow it; pinned programs stay on their lane.
- If the Tailscale app for Windows is installed and uses an exit node, SplitLane turns that exit node off while split routing is on and turns it back on when you turn split routing off. After a crash or a restart of Windows, it turns it back on the next time the service starts.

## Requirements

- Windows 10 or 11, x64.
- A VPN: a Tailscale account (tailnet) with an exit node you can use, a server of a protocol SplitLane supports (see [Keys, subscriptions and WireGuard](#keys-subscriptions-and-wireguard)), or a VPS with Debian 12+ or Ubuntu 22.04+ for SplitLane to set up (see [Your own VPS](#your-own-vps)). The VPN must reach the internet from a different address than this PC.
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

Which files will be signed, by whom and how: [Code signing policy](CODE_SIGNING_POLICY.md).

Installing over an older version works the same way: the installer first stops the service of the old version and turns the exit node of the Tailscale app back on, if it used one. Your settings and the Tailscale login are kept. Silent install: `SplitLane-Setup-<version>.exe /S` (the window doesn't start; a non-zero exit code means the installation failed).

winget: coming soon.

## First start

1. If the window shows **First-time setup**, click **Install the service** and confirm the Windows prompt. The installer normally does this for you, so this screen appears only when the service needs to be repaired.
2. On **Connect a VPN**, choose how you connect: **Tailscale**, **Key or subscription**, **Configuration file** or **Your own VPS**. For a key, a subscription or a WireGuard file, see [Keys, subscriptions and WireGuard](#keys-subscriptions-and-wireguard); for a VPS you rent, see [Your own VPS](#your-own-vps). For Tailscale, click **Log in to Tailscale**. SplitLane's window opens the login page in your browser: log in to the tailnet that has your exit node. This logs in SplitLane's embedded node, a new device `<PC name>-splitlane`; the Tailscale app isn't involved. If your tailnet requires device approval, approve the device in the Tailscale admin console; the window continues by itself.
3. Choose an exit node or a server, then click **Turn on split routing**.
4. Drag programs onto the lanes. You can move a card from one lane to the other, drop an `.exe` file or a shortcut from Explorer onto a lane, or click **+ Add** to pick a program that is online now, an installed program or a file. The same actions are in the context menu of a card (**Send via VPN**, **Send direct**, **Unpin**) and work from the keyboard.

A pinned program stays on its lane; other programs follow the main lane (**make main** on the other lane switches it). By default a program means its whole folder, so its updaters and helper processes go the same way; you can change this in the program card. **Settings and connection** in the header (or **Settings…** in the tray menu) opens the **Connection** screen: the exit node, logging out of Tailscale, the servers and subscriptions of keys and WireGuard files, the language and the settings **Start when I sign in to Windows**, **Turn on split routing at startup**, **Check for updates automatically** and **Show notifications**.

Closing the window keeps SplitLane in the tray; while it is there, SplitLane shows a Windows notification when split routing stops with an error, the VPN server can't be reached, the Tailscale app turns its exit node back on, the Tailscale login expires or a new version is available. **Quit** in the tray menu turns split routing off and stops the service.

## Keys, subscriptions and WireGuard

Instead of a Tailscale exit node, the VPN lane can go through a VPN server of your own or of a VPN service. SplitLane's core speaks these protocols itself:

- **Protocols:** VLESS (including Reality and `xtls-rprx-vision`), VMess, Trojan, Shadowsocks (including Shadowsocks 2022 and Outline keys), Hysteria2 and TUIC v5 from share links (`vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://` or `hy2://`, `tuic://`); WireGuard from a `.conf` file.
- **Transports:** plain TCP, WebSocket, gRPC, HTTPUpgrade, HTTP/2 and QUIC; TLS with SNI, ALPN and a uTLS fingerprint, or Reality.
- **Subscriptions:** an `https://` address that returns a list of share links, as plain text or in base64.

**Adding servers.** On **Connect a VPN** (or later on **Connection**), choose **Key or subscription**, paste one or more links (one per line) or one subscription address, and click **Add**. For WireGuard, choose **Configuration file** and pick the `.conf` file; you can also paste its text under **Key or subscription**, alone or together with links. SplitLane then shows how many servers it added and which lines it rejected and why, by line number (the line itself isn't shown: it holds a key). A link with something SplitLane doesn't support is rejected with the reason rather than half understood.

**Choosing a server.** The servers are listed with their name, protocol and source (added by hand, a file or the subscription's name). The server you choose leads the VPN lane, and the header shows it as `<server> · <protocol>`. Changing the server while split routing is on restarts split routing briefly, like changing the exit node. Choosing an exit node switches the VPN lane back to Tailscale.

**Subscriptions.** **Connection** lists each subscription with its number of servers, the time of its last update and the error of the last refresh, if it failed. **Refresh** downloads it again and replaces its servers; the selected server stays selected if the subscription still has it. **Remove** deletes a server, or a subscription with all its servers. While split routing is on, the selected server can't be removed, and its subscription can't be refreshed or removed.

Links, WireGuard files and subscription addresses are secrets. SplitLane keeps them in `%ProgramData%\SplitLane\servers.json` (administrators only); they aren't written to the logs or put in diagnostics archives. The keys of the selected server are also in the core configuration file in the same folder, `core-config.json` (administrators only, never in diagnostics archives). SplitLane writes that file again each time split routing turns on, so after you remove a server its keys may stay there until split routing is next turned on.

**Your own server.** **Your own VPS** sets up a rented server from the window (see [Your own VPS](#your-own-vps)). [scripts/server](scripts/server/README.md) has the script it uses, which you can also run by hand: it sets up VLESS + Reality, Hysteria2, Shadowsocks 2022 and WireGuard on a Debian or Ubuntu VPS and prints the links, the `.conf` file and, if you like, a subscription address.

**Limits.**

- Only the protocols and transports listed above. AmneziaWG and OpenVPN aren't supported yet. The `xhttp`, `splithttp` and `kcp` transports, the `http` header over TCP, Shadowsocks plugins and Clash or sing-box subscriptions are rejected with a reason.
- A WireGuard file must send all public addresses through the server: one whose `AllowedIPs` don't cover all public IPv4 is rejected. `0.0.0.0/0` does, and so does the list the WireGuard apps write with "Exclude private IPs" (SplitLane sends local and private addresses directly anyway).
- Subscriptions are refreshed only when you click **Refresh**, not on a schedule.
- A server without UDP can't carry the UDP traffic of programs on the VPN lane (games, calls).
- There is no latency test and no automatic choice of server; one server leads the VPN lane at a time.
- Some internet providers recognize Shadowsocks and WireGuard by DPI and cut them (Russia, Iran, China). To SplitLane this looks like a server that doesn't answer: it says it couldn't make sure the tunnel works, turns split routing off and suggests VLESS + Reality or Hysteria2, which usually get through.

## Your own VPS

If you rent a VPS (a virtual server) with Debian 12+ or Ubuntu 22.04+, SplitLane can turn it into your VPN server by itself. You need only what your hosting provider emails you after you rent it: the server address, the username and the password (or an SSH key file).

**Setting it up.**

1. Turn split routing off if it is on. It must stay off during the setup: setting up the server the tunnel goes through would cut the connection in the middle of the installation.
2. On **Connect a VPN** (or later on **Connection**), choose **Your own VPS**.
3. Enter the server address (an IP address or a name; add `:port` if SSH isn't on port 22, such as `203.0.113.10:2222`), the username (usually `root`) and the password. To sign in with a key, choose **Key file**, pick an OpenSSH (or PEM) private key and enter its passphrase if it has one. Export a PuTTY key (`.ppk`) first: in PuTTYgen, **Conversions** → **Export OpenSSH key**.
4. Click **Set up**. The window shows the steps (connecting to the server, checking the system, installing the VPN server, securing the server, the subscription for a phone, adding to SplitLane) and the time elapsed. It usually takes a few minutes; **Cancel** stops it on this PC (an installation already started on the server finishes there, and running the setup again gets its servers).
5. When it's done, the new servers are in the list and the first one (VLESS + Reality) is selected. Click **Turn on split routing**.

**What it installs.** SplitLane signs in over SSH, checks the system (Debian 12+ or Ubuntu 22.04+, x86_64 or ARM64, systemd) and the rights (root, or a user with sudo), then runs the [server kit](scripts/server/README.md) built into SplitLane (always the version that came with it):

- sing-box with VLESS + Reality and Hysteria2 on free ports (443, or the next free one of a short list);
- a subscription for a phone: an `https://<address>.sslip.io:<port>/…` address served by Caddy with a Let's Encrypt certificate (SplitLane's list of subscriptions shows it by that sslip.io name);
- light protection: fail2ban for SSH and automatic security updates.

The settings of SSH (password logins, the port) don't change. On a server where the kit already runs, a new setup keeps its keys and ports: SplitLane adds the same servers and selects the first one.

**The password and the key** are used once, for this setup, and never saved. They are only in the memory of the window while its screen is open, and they reach the setup process through its standard input, not its command line; the window drops them when you leave the screen or close (hide) the window, and a setup already running goes on. To change something on the server later, run the setup again.

**The server's host key isn't verified.** On the first connection to a freshly rented VPS there is nothing to compare its SSH host key with, so SplitLane doesn't check it. The known risk: someone who intercepts the connection at the very moment of the setup could learn the password. If that matters to you, sign in with a key, or change the server's password after the setup.

**If something goes wrong,** the window says what happened and offers **Try again** with the same details. If the connection breaks in the middle of the installation, run the setup again: the installation on the server finishes by itself meanwhile, and the new setup waits for it and adds the same servers.

**fail2ban.** After the setup, fail2ban bans an address for 10 minutes after 5 failed SSH logins from it within 10 minutes. If that happens to you (say, after mistyping the password a few times), wait 10 minutes, or sign in from another address or through your hosting provider's web console and run `sudo fail2ban-client unban <your address>`; `sudo fail2ban-client status sshd` lists the banned addresses. The addresses connected over SSH during the setup (this PC's among them) are exempt from the ban, as the kit writes them to `ignoreip`, so a ban mostly concerns a new address.

**Limits.**

- The server needs a public IPv4 address: an IPv6-only VPS won't do.
- A new setup installs only VLESS + Reality and Hysteria2 (Shadowsocks and WireGuard are often cut by DPI); the [script](scripts/server/README.md) run by hand still installs any of them.
- A phone needs port 80 and the subscription port open to the internet. If your hosting provider blocks them, or another program uses port 80, the phone can't connect, but this PC works. Ubuntu 22.04 has no Caddy package, so there is no subscription there.
- SplitLane doesn't keep access to the server: to change something there, you run the setup again.
- PuTTY keys (`.ppk`) aren't accepted, only OpenSSH and PEM keys.

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
- With Tailscale, the embedded Tailscale node talks to Tailscale's servers like any Tailscale device. With a key or a WireGuard file, the core connects to that server; the service downloads a subscription from the address you add, when you add or refresh it.
- While split routing is on, the service checks both lanes with `api.ipify.org`, and website names are resolved through encrypted DNS at `1.1.1.1` over the VPN lane.
- With **Your own VPS**, SplitLane connects to your server over SSH for the setup; the password or the key is used once and isn't saved, and the server's host key isn't verified at the first connection.
- Settings and logs stay on your PC. A diagnostics archive leaves it only if you share it.

Details, and how to delete everything: [PRIVACY.md](PRIVACY.md).

## Known limitations of this beta

- The installer and the programs aren't signed. SmartScreen warns about the downloaded installer, and the update check verifies integrity, not authenticity.
- One Windows user per PC. Split routing applies to the whole PC, so programs in other Windows sessions follow the same lanes, but only the user who installed SplitLane can control it.
- Keys, subscriptions and WireGuard files have limits of their own: see [Keys, subscriptions and WireGuard](#keys-subscriptions-and-wireguard). So does **Your own VPS**: see [Your own VPS](#your-own-vps).
- Installation only in `%ProgramFiles%\SplitLane`.
- The embedded node is a separate device in your tailnet with its own login expiry. When it expires, SplitLane asks you to log in again.
- DNS is shared by both lanes. While the VPN lane works, names are resolved through it, so content delivery networks may give programs on the Direct lane servers near the VPN server. When the VPN server is unreachable or **All direct** is on, all names go to your ISP's DNS.
- If the Tailscale app manages DNS (its default), MagicDNS names don't resolve while split routing is on, and if your tailnet overrides DNS for all names, split routing doesn't turn on. It has been tested only with DNS management turned off in the Tailscale app.
- WSL and virtual machines follow the main lane. So do connections whose program the core can't identify (for example `ping`).
- Programs that go online through shared system processes (for example, the Edge web component) can't be separated from each other.
- Traffic to private, local and Tailscale addresses always goes directly.
- IPv6 to the internet doesn't work while split routing is on.
- There is no kill switch. When the core crashes, traffic goes directly until SplitLane turns the exit node of the Tailscale app back on (about a second), then through that exit node until the core restarts. When SplitLane restarts the core itself (after a network change, when you change the exit node or the server), traffic goes through the exit node of the Tailscale app meanwhile. If the Tailscale app isn't installed or had no exit node selected, traffic goes directly in both cases.
- An exit node or a server that reaches the internet from the same address as this PC can't be used.
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

Code signing: see the [Code signing policy](CODE_SIGNING_POLICY.md). Security issues: see [SECURITY.md](SECURITY.md). Other problems and questions: [GitHub issues](https://github.com/splitlane-app/splitlane/issues).

## Development

- **Layout.** `helper/`: the service `splitlane-helper.exe` (Go), the only part with administrator rights. `helper/cmd/slctl`: a test client for the service's pipe. `helper/cmd/slguard`: a guard for live checks on a PC during development, not shipped. `app/`: the window (Electron, React, TypeScript); `app/build/installer.nsh`: additions to the installer. `scripts/`: the core download, the builds and the installer check; `scripts/server`: the server kit for a VPS (a bash script, not shipped). `docs/`: specifications and plans, in Russian. `.github/workflows/build.yml`: the CI build.
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
