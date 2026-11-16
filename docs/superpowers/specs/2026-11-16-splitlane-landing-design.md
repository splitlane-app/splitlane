# SplitLane: landing page and launch texts

Date: 2026-11-16. Status: agreed with the product owner (international audience first; site at `splitlane-app.github.io`; screenshots from a demo mode with the programs' real icons and a trademark notice; the main examples — launchers through the VPN and games direct with a lower ping; two browsers in different lanes).

## 1. Goal

A public page that explains SplitLane in a minute and gets a Windows user to download it, plus screenshots for the README and a launch post. Audience: international, English. Success: a visitor understands "choose which apps use your VPN", sees the real interface, trusts it enough to run an unsigned installer, and finds the download in one click.

Out of scope: a custom domain (can be added later), analytics or any tracking on the site, a Russian version of the site (the app and the READMEs are already bilingual), a blog, a GIF/video (static screenshots first).

## 2. Screenshots — demo mode (main repository)

- A demo build of the window's renderer with a fake bridge and fixed English data; it is never part of the product bundle (its own entry and build command, output outside `out/`), and it starts nothing: no helper, no pipe, no network.
- One URL per scene: `?screen=board|connection|vps-progress|vps-done|phone&theme=light|dark`.
- Data (no personal data anywhere):
  - **Via VPN** (main lane): Steam, Epic Games Launcher, Discord, Firefox — with live-looking speeds and activity graphs.
  - **Direct**: two games (Counter-Strike 2, Fortnite) shown as detected Steam/Epic games, and Microsoft Edge — two browsers in different lanes (Firefox through the VPN, Edge direct for local banking and streaming) is a scenario everyone recognizes.
  - Program names are the real names. Icons are the programs' real icons, as SplitLane would show them on any PC: extracted at capture time from programs installed on this PC into a git-ignored folder and never committed — only the screenshots are; a program that is not installed gets a neutral monogram tile. No vendor logo appears anywhere else on the site (no "works with" logo wall, no banners).
  - Header: on, server `203.0.113.10 VLESS` (documentation address, the kit's naming); Connection: servers `203.0.113.10 VLESS` / `203.0.113.10 Hysteria2` from subscription `203-0-113-10.sslip.io`, plus a Tailscale tile; VPS: progress mid-install and the done state; phone: the QR dialog for an invented address `https://203-0-113-10.sslip.io:8443/<random hex>/sub`.
- Capture: a script run with the Electron already in the dev dependencies (`electron app/scripts/capture-screens.mjs`), which loads each scene in an offscreen window at 1200×760 CSS px, zoom 2, and writes PNGs to `docs/screens/<scene>-<theme>.png`, then optimizes their size (no new dependency if Electron's `nativeImage` resize/PNG suffices). The script does not load the app's main process.
- Every PNG is checked by eye for personal data before it is committed.

## 3. The site (`splitlane-app/splitlane-app.github.io`)

- A new public repository in the organization; GitHub Pages serves its `main` branch root. Static `index.html` + `styles.css` + `assets/` (screenshots copied from the main repo, favicon from the app icon); no framework, no build, no tracking, no third-party requests except fonts if any (prefer the system font stack — then none).
- Light and dark by `prefers-color-scheme`; responsive down to 360 px; accessible (landmarks, alt texts, contrast ≥ 4.5:1, visible focus).
- Meta: title "SplitLane — choose which apps use your VPN", description, Open Graph and Twitter card with the board screenshot.
- Sections, in order:
  1. **Hero** — headline "Choose which apps use your VPN"; subline: launchers through the VPN, games direct with a lower ping; one browser through the VPN, another direct; the main button "Download for Windows" → `https://github.com/splitlane-app/splitlane/releases/latest`; "Free and open source · GPL-3.0 · Windows 10/11 x64"; the board screenshot.
  2. **How it works** — two lanes and the main lane; drag programs or pick installed ones; Steam and Epic games found automatically; a program means its whole folder, so updaters follow.
  3. **Works with your VPN** — Tailscale exit node; keys and subscriptions (VLESS + Reality, VMess, Trojan, Shadowsocks, Hysteria2, TUIC); WireGuard files.
  4. **Your own VPS in one click** — address, login, password or key → Set up → a ready server, selected, and a QR for the phone; the two VPS screenshots and the phone QR.
  5. **Private by design** — no telemetry, no accounts of its own; credentials used once, never stored; open source; links to the privacy policy and the code signing policy.
  6. **FAQ** — "Windows SmartScreen warns me" (the installer is not code-signed yet; signing is being applied for; More info → Run anyway; how to check the SHA-256 published with each release); "Is SplitLane a VPN?" (no — it sends the programs you choose through your VPN and the rest directly); "Which VPN do I need?"; "How do I uninstall it?"; "Is my data sent anywhere?".
  7. **Footer** — GitHub, Releases, Privacy policy, Code signing policy, License; the line "Steam, Epic Games, Fortnite, Counter-Strike, Discord, Firefox and Microsoft Edge are trademarks of their respective owners. SplitLane is not affiliated with them."
- Texts: plain English, short sentences, no superlatives or claims the code does not support (every claim checked against README/PRIVACY).

## 4. Main repository changes

- `README.md`: the board screenshot at the top and a link to the site; `README.ru.md`: the same link (screenshot in English is fine).
- The demo mode and the capture script are documented in the Development section of the README (how to regenerate the screenshots).

## 5. Launch texts (not committed)

- Drafts for r/selfhosted (the "Your own VPS" kit + split routing) and for "Show HN", written to a local file outside git and handed to the owner, who posts them from their own accounts. Each draft: what it is, why (the launcher/game ping example), what is novel (one-click VPS with a phone QR, any protocol via sing-box), honest limits (Windows only, unsigned for now), links to the site and the repo. Reddit-friendly tone: "I built …", no marketing language.

## 6. Verification

- Demo build and capture run without the product's main process; PNGs reviewed for personal data.
- The site checked in a browser at 1440 px and 375 px, light and dark; every link resolves; page weight under 1.5 MB; no console errors; HTML valid.
- After publishing: the site answers at `https://splitlane-app.github.io/` with the right title and Open Graph preview.
