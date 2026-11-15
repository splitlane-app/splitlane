# Security policy

## Supported versions

Only the latest release gets security fixes. Update with the in-app updater or from [Releases](https://github.com/splitlane-app/splitlane/releases).

## Reporting a vulnerability

Please don't report vulnerabilities in public issues. Use GitHub's private vulnerability reporting: open the **Security** tab of the repository and click **Report a vulnerability**, or go to <https://github.com/splitlane-app/splitlane/security/advisories/new>. Only the maintainers see the report.

Please include:

- the SplitLane version (the **Connection** screen shows the versions of SplitLane, the service and the core), the Windows version and the Tailscale version;
- what the problem is and what an attacker can do with it;
- what the attacker needs: for example, a program running as the same Windows user, another standard user on the same PC, or a position on the network;
- steps to reproduce, and a proof of concept if you have one.

Don't include node keys, login links, the names or addresses of your tailnet devices, or other personal data. Mask them in logs and screenshots.

What to expect: a first reply within 7 days, then updates while the problem is examined and fixed. Once a fix is released, the advisory is published, with credit to you if you want it.

## Security model in short

- Only the service (`splitlane-helper.exe`) and the network core run with administrator rights. Both run from `%ProgramFiles%\SplitLane\helper`, where only administrators can write; this is why the installation folder can't be changed.
- The pipe of the service is open only to the Windows user who set SplitLane up, administrators and SYSTEM. The service accepts commands only from the installed window `%ProgramFiles%\SplitLane\SplitLane.exe` or from elevated processes. The set of commands is closed, and none of them takes a file to run, a core configuration or a path to write to.
- The installed window can't be started as Node.js (Electron fuses), loads no remote content, and quits if it is started with debugging switches.
- The node key, the core configuration and the secrets of the core's local APIs are in `%ProgramData%\SplitLane`, which standard users can't open. The local APIs listen only on `127.0.0.1`, with a random port and secret on each start.
- Updates are checked by SHA-256 against the `.sha256` file of the same release. This proves integrity, not authenticity: the installer isn't signed yet.

What SplitLane doesn't defend against: malware that already runs as your Windows user. Such a program can inject code into the window, which runs without elevation as the same user, or replace a downloaded update installer in your temporary folder before it runs (Windows still shows the UAC prompt). Someone who gains access to the repository's releases could also publish an installer with a matching checksum.

Details (in Russian): [installer specification, section 5](docs/superpowers/specs/2026-11-15-splitlane-stage2-installer-design.md#5-проверка-вызывающей-программы), [stage 1 specification, section 11](docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md#11-безопасность) and [updates specification, section 9](docs/superpowers/specs/2026-11-15-splitlane-stage2-updates-design.md#9-известные-ограничения-части-2).
