# SplitLane landing page and launch texts — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A public one-page site at `https://splitlane-app.github.io/` with real screenshots from a demo mode of the window, a README hero image, and drafts of the launch posts.

**Architecture:** A demo entry of the window's renderer (its own HTML and Vite build, a fake bridge with fixed English data, never part of the product bundle) is captured to PNGs by a small Electron script. A separate public repository `splitlane-app/splitlane-app.github.io` holds a static `index.html` + `styles.css` + the screenshots, served by GitHub Pages from `main`. The README links to the site.

**Tech Stack:** React 19 / TypeScript 6 / Vite (electron-vite's renderer setup reused), Electron 45 (capture only), Vitest 5, plain HTML and CSS for the site, PowerShell 5.1 for icon extraction, `gh` for the repository and Pages.

**Spec:** `docs/superpowers/specs/2026-11-16-splitlane-landing-design.md`

## Global Constraints

- No personal data anywhere: no real addresses (only `203.0.113.10`, `203-0-113-10.sslip.io`, `vps.example.com`), no device, tailnet, user or PC names, no real subscription tokens; every PNG is looked at before it is committed.
- Program names real; icons real only inside screenshots of the interface, extracted at capture time into git-ignored files and never committed; a program not installed on this PC gets a neutral monogram tile; no vendor logo anywhere else.
- Footer line verbatim: "Steam, Epic Games, Fortnite, Counter-Strike, Discord, Firefox and Microsoft Edge are trademarks of their respective owners. SplitLane is not affiliated with them."
- Scenes: `board`, `connection`, `vps-progress`, `vps-done`, `phone`; themes `light`, `dark`; files `docs/screens/<scene>-<theme>.png`, captured at 1200×760 CSS px, zoom 2.
- Demo data: Via VPN (main lane) — Steam, Epic Games Launcher, Discord, Firefox; Direct — Counter-Strike 2 and Fortnite (as detected Steam/Epic games) and Microsoft Edge; server `203.0.113.10 VLESS` / `203.0.113.10 Hysteria2` from subscription `203-0-113-10.sslip.io`; phone QR of `https://203-0-113-10.sslip.io:8443/<random hex>/sub`.
- The demo never starts the helper, the pipe or the network, and never enters `app/out` or the installer; the product build (`pnpm build`, `scripts\build-installer.ps1`) is unchanged.
- Site: static, no framework, no build, no tracking, no third-party requests (system font stack), light and dark by `prefers-color-scheme`, works down to 360 px, contrast ≥ 4.5:1, visible focus, page weight < 1.5 MB, Open Graph + Twitter card with the board screenshot.
- Download button → `https://github.com/splitlane-app/splitlane/releases/latest`.
- Never run `pnpm dev`, `electron-vite dev`, `SplitLane.exe`, the installer or `scripts\test-installer.ps1`. Running Electron with the capture script only is allowed.
- Commits in English ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; the launch drafts are not committed.

## Review Focus

1. Personal data leaking into a screenshot (an icon extracted with a user path, a window title, the PC name in a tooltip, a real address in a field) — every scene shows only the demo data. Task 1: a test scans the demo data module for anything outside the allowed addresses and names; the controller looks at each PNG.
2. The demo code or its icon file shipped with the product — Task 1: a test asserts `electron.vite.config.ts` inputs are unchanged and `pnpm build`'s `out/` has no demo files.
3. The site at 360 px and in dark mode (overflowing screenshots, unreadable contrast, buttons off-screen) — Task 2: captures at 375 px and 1440 px in both themes, looked at by the controller.
4. Broken links (releases/latest, PRIVACY.md, CODE_SIGNING_POLICY.md, README anchors) — Task 2: a link check of every `href`.
5. A site claim the product does not support (protocol list, Windows versions, "no telemetry", the VPS flow) — Task 2: every claim mapped to a README/PRIVACY line in the report.

---

### Task 1: Demo mode and screenshots

**Files:**
- Create: `app/src/renderer/demo/demo.html`, `app/src/renderer/demo/main.tsx` (mounts `App` with the demo services, sets English, applies `?screen=` and `?theme=`), `app/src/renderer/demo/scenes.ts` (fixed snapshot, activity, proxy state, VPS run states, reveal answer), `app/src/renderer/demo/fakeBridge.ts` (a `SplitLaneBridge` answering from `scenes.ts`; `icon(exe)` from the icon map, else a monogram data URL), `app/src/renderer/demo/monogram.ts`, `app/vite.demo.config.ts` (renderer-only Vite build of `src/renderer/demo/demo.html` to `app/out-demo/`), `app/scripts/demo-icons.ps1` (finds the installed executables of the demo programs — Steam, Epic Games Launcher, Discord, Firefox, Microsoft Edge, Counter-Strike 2, Fortnite — via App Paths/uninstall keys and common folders, extracts 64×64 PNG icons with `System.Drawing`, writes `app/src/renderer/demo/icons.local.json` `{exeName: dataUrl}`), `app/scripts/capture-screens.mjs` (Electron: offscreen `BrowserWindow` 1200×760, `setZoomFactor(2)`, loads `out-demo/demo.html?screen=…&theme=…`, waits for a `data-demo-ready` attribute, `capturePage()` → `docs/screens/<scene>-<theme>.png`), tests `app/src/renderer/demo/scenes.test.ts`.
- Modify: `app/package.json` (scripts `demo:build`, `demo:capture`), `.gitignore` (`app/out-demo/`, `app/src/renderer/demo/icons.local.json`), `tsconfig.web.json` only if the demo needs inclusion for typecheck.

**Interfaces:**
- Produces: `docs/screens/{board,connection,vps-progress,vps-done,phone}-{light,dark}.png` (Task 2 copies them; Task 3 uses `board-light.png` in the README).
- The scene switch: `?screen=board` (the board with live-looking speeds), `connection` (Connection screen, Key or subscription tile, the two servers selected/listed and the subscription), `vps-progress` (Your own VPS tile mid-install: connect/system done, install running), `vps-done` (done, "Done. Servers added: 2", Connect a phone button), `phone` (the QR dialog open).

**Behaviour:** the demo reuses the real components and store (no forked UI): it fills `useAppStore` with the scene's snapshot and run state and drives navigation through the same store/route the app uses; anything that would call the main process answers from `scenes.ts`. Speeds: plausible values (e.g. Steam 2.4 MB/s down, games a few KB/s). Theme: the app's own theme mechanism (set `data-theme` or the setting the app reads), not a separate stylesheet.

- [ ] Steps: test that `scenes.ts` holds only allowed addresses (`203.0.113.10`, `203-0-113-10.sslip.io`, `vps.example.com`, `127.0.0.1` if any) and the listed program names, and that `electron.vite.config.ts` inputs do not include the demo → fail → implement → `pnpm typecheck; pnpm test; pnpm build` (out/ has no demo) → `powershell -File app\scripts\demo-icons.ps1` → `pnpm demo:build; pnpm demo:capture` → list the PNGs with sizes → commit `feat(app): a demo mode of the window for screenshots` (code) and `docs: screenshots of the window` (PNGs) — the icon JSON stays uncommitted.

### Task 2: The site

**Files (new repository `splitlane-app/splitlane-app.github.io`, public, cloned to `C:\code\splitlane-site`):** `index.html`, `styles.css`, `assets/screens/*.png` (copied from `docs/screens/`, optimized), `assets/favicon.png` and `assets/favicon.ico` (from `app/build/icon.ico`), `assets/og.png` (1200×630 from the board screenshot), `README.md` (what the repo is), `.nojekyll`.

**Behaviour:** sections in the order of the spec §3 (Hero, How it works, Works with your VPN, Your own VPS in one click, Private by design, FAQ, Footer) with the texts the spec lists; `<picture>` with light/dark screenshots by `prefers-color-scheme`; images with `width`/`height` and `loading="lazy"` below the fold; the FAQ as `<details>`; the trademark line in the footer. GitHub Pages enabled on `main` / root (`gh api -X POST repos/splitlane-app/splitlane-app.github.io/pages -f "source[branch]=main" -f "source[path]=/"`).

- [ ] Steps: write the page → local check by capturing it with the Task 1 capture approach at 1440 and 375 px in both themes (a copy of the script pointed at `index.html`) → link check of every `href` (HTTP status) → page weight → create the repository (`gh repo create splitlane-app/splitlane-app.github.io --public`), push, enable Pages, wait for `https://splitlane-app.github.io/` to answer 200 with the title → report with the claim-to-source map (Review Focus 5) and the captures' paths.

### Task 3: README and launch drafts

**Files:** `README.md` (the board screenshot under the title, a line linking the site), `README.ru.md` (the site link), README Development section (how to regenerate the screenshots: `demo-icons.ps1`, `demo:build`, `demo:capture`); drafts (not committed) in `C:\code\router\.superpowers\launch\reddit-selfhosted.md` and `show-hn.md`.

**Behaviour:** the drafts follow spec §5: "I built …" tone; the launcher/game ping and two-browser examples; the one-click VPS with a phone QR; any protocol through sing-box; honest limits (Windows only, not code-signed yet, applying for SignPath); links to the site and the repository; r/selfhosted flair/format; Show HN title ≤ 80 characters.

- [ ] Steps: README edits → `pnpm`-free check that the image path renders on GitHub (relative path) → commit `docs: README links the site and shows the window` → write the two drafts → report.
