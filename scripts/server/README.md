# SplitLane server kit

[Русская версия](README.ru.md)

`install-singbox-server.sh` turns a Linux VPS into a VPN server for SplitLane's **Key or subscription** and **Configuration file** connections. It is a helper for testing SplitLane and for running your own server; it isn't part of the Windows app, and SplitLane works with any server of the supported protocols.

## What it sets up

- **sing-box 1.14.4**, the same version as SplitLane's core, from the [official release](https://github.com/SagerNet/sing-box/releases/tag/v1.14.4). The script checks the SHA-256 of the archive against the values written in it and stops, installing nothing, if they differ.
- **One server with up to four ways in** (all four unless `--protocols` chooses):

  | Protocol | Port | Name in SplitLane |
  |---|---|---|
  | VLESS + Reality, `xtls-rprx-vision` | 443/tcp | `SplitLane-VLESS-Reality` |
  | Hysteria2, self-signed certificate | 443/udp | `SplitLane-Hysteria2` |
  | Shadowsocks 2022, `2022-blake3-aes-128-gcm` | 8388/tcp and udp | `SplitLane-SS2022` |
  | WireGuard | 51820/udp | the file `splitlane-wg.conf` |

  Where providers filter traffic by DPI (Russia, Iran, China), VLESS + Reality and Hysteria2 are the ones that usually get through; see [If something doesn't work](#if-something-doesnt-work).

  These are the default ports. If another program already uses one (for example a VPN server of your own on 443), choose another with `--vless-port`, `--hy2-port`, `--ss-port` or `--wg-port`, or let `--auto-ports` take the first free one of a short list (see the options).

  Connections from clients to loopback and private-network addresses (the VPS's own `127.0.0.1`, its provider's internal network) are refused. Services on the VPS's public address stay reachable through the tunnel, as from the internet.
- A systemd service `splitlane-kit` that runs as its own system user, not as root, and restarts after a failure. The kit's sing-box lives in `/usr/local/lib/splitlane-kit/`, off `PATH`, so a sing-box you already have, and the server it runs, stay as they are.
- With `--subscription`: [Caddy](https://caddyserver.com) serves the links as a subscription at `https://<name>:8443/<random token>/sub`, with a certificate from Let's Encrypt.
- With `--harden`: fail2ban for SSH and automatic security updates (see [Light hardening](#light-hardening---harden)).

## Requirements

- A VPS with Debian 12 or later, or Ubuntu 22.04 or later, on x86_64 or arm64 (aarch64), with systemd; you log in as root or as a user with sudo.
- A public IPv4 address. The script asks `api.ipify.org` for it; with `--host` you give the address or a name yourself.
- Free ports 443/tcp, 443/udp, 8388/tcp, 8388/udp and 51820/udp (only those of the chosen ways in), or the ones you choose instead. The script checks them first and stops, installing nothing, if one is taken. If your provider has a firewall in its control panel, open them there.
- For `--subscription`: Caddy in the distribution's packages (Debian 12 and later, Ubuntu 24.04 and later; Ubuntu 22.04 has none, so the step is skipped with a message), ports 80/tcp and 8443/tcp (or the one of `--sub-port`) reachable from the internet (Let's Encrypt checks port 80; if a program other than Caddy listens on it, the subscription is skipped with a warning) and a name that points to the VPS. By default the name is `<a-b-c-d>.sslip.io` for the address a.b.c.d: [sslip.io](https://sslip.io) resolves such names to the address in them, so you need no domain of your own.

## Install

On the VPS:

```sh
curl -fsSLO https://raw.githubusercontent.com/splitlane-app/splitlane/main/scripts/server/install-singbox-server.sh
less install-singbox-server.sh    # read what you are about to run as root
sudo bash install-singbox-server.sh --subscription    # or without --subscription
```

If the address doesn't open (for example, before the kit is on the `main` branch), copy the file from your copy of the repository to the home folder of your user on the VPS and run it there (Windows 10 and 11 have `scp` built in):

```sh
scp scripts/server/install-singbox-server.sh <user>@<VPS address>:    # on your PC
sudo bash ~/install-singbox-server.sh --subscription                  # on the VPS
```

| Option | What it does |
|---|---|
| `--protocols <list>` | The ways in, comma-separated from `vless`, `hy2`, `ss`, `wg`, such as `vless,hy2`. Default: all four. A way in left out of the list leaves the server, the links and the `.conf` on that run. |
| `--default-protocols <list>` | The ways in of a new installation. A server this kit set up before keeps its own: the remembered ones, or all four if they were never chosen. |
| `--subscription` | Also serve the links as a subscription over HTTPS (Caddy, port 8443; port 80 for the certificate). |
| `--harden` | Also install fail2ban for SSH and automatic security updates (see [Light hardening](#light-hardening---harden)). |
| `--domain <name>` | The name of the subscription address and of the Hysteria2 certificate. Default: the address as an sslip.io name, such as `192-0-2-10.sslip.io`. |
| `--host <address or name>` | The server address in the links and in the `.conf`. Default: the public IPv4 address. |
| `--reality-sni <name>` | The site whose TLS 1.3 handshake Reality borrows. Default: `www.microsoft.com`. |
| `--vless-port <port>` | The port of VLESS + Reality, tcp. Default: 443. |
| `--hy2-port <port>` | The port of Hysteria2, udp. Default: 443. |
| `--ss-port <port>` | The port of Shadowsocks 2022, tcp and udp. Default: 8388. |
| `--wg-port <port>` | The port of WireGuard, udp. Default: 51820. |
| `--sub-port <port>` | The port of the subscription, tcp. Default: 8443. Port 80 is needed for the certificate either way. |
| `--auto-ports` | For each port that no option gives and no earlier run remembers: the first one of a short list that no other program listens on, instead of the default. The lists: VLESS 443, 2053, 2083, 2087, 2096 (tcp); Hysteria2 443, 4443, 8443, 10443 (udp); Shadowsocks 8388, 8389, 9388 (tcp and udp); WireGuard 51820, 51821, 51822 (udp); the subscription 8443, 9443, 7443 (tcp). If every port of a list is taken, the script stops. Port 80 of the certificate never moves. |
| `--reset` | New keys, links and subscription address; the old ones stop working. |
| `--uninstall` | Remove the kit (see below). |
| `--json` | Output for programs: one JSON object per line on stdout (the steps, then the result with the links, or an error with its code), the text for people on stderr. |
| `--dry-run <folder>` | A test: write everything under the folder and check the config with `sing-box check`; no systemd, firewall, Caddy or apt. The address is `192.0.2.10` unless `--host` is given. |
| `-h`, `--help` | The list of options. |

`--host`, `--domain`, `--reality-sni`, the ways in (of `--protocols`, or of `--default-protocols` on a new installation), the ports (those that `--auto-ports` chose too) and `--subscription` are remembered for later runs, `--reset` included, until `--uninstall`; giving one again with another value replaces it. The log level is a variable at the top of the script.

**SplitLane's "Your own VPS"** runs this script over SSH with `--json --default-protocols vless,hy2 --auto-ports --harden --subscription`: a new server gets VLESS + Reality and Hysteria2 on free ports, fail2ban, automatic security updates and the subscription for a phone. On a server where the kit already runs, its ways in and ports stay as they are.

## What it prints

The share links, the text of the WireGuard file and, with `--subscription`, the subscription address. For example, with all four ways in (the secrets are cut here):

```text
vless://…@192.0.2.10:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com&fp=chrome&pbk=…&sid=…&type=tcp&headerType=none#SplitLane-VLESS-Reality
hysteria2://…@192.0.2.10:443?sni=192-0-2-10.sslip.io&insecure=1#SplitLane-Hysteria2
ss://2022-blake3-aes-128-gcm:…@192.0.2.10:8388#SplitLane-SS2022
https://192-0-2-10.sslip.io:8443/…/sub
```

The same is saved in `/root/splitlane-kit/`, which only root can open: `links.txt` (the links), `splitlane-wg.conf` (WireGuard) and `subscription.txt` (the links in base64, the format of subscriptions).

If ufw is active, the script allows the ports in it before the server and Caddy start; otherwise it lists the ports to open in your provider's firewall.

## Add the server to SplitLane

- **Links:** on **Connect a VPN** (or later on **Connection**), choose **Key or subscription**, paste one link or all of them, one per line, and click **Add**.
- **WireGuard:** paste the text the script printed (or `sudo cat /root/splitlane-kit/splitlane-wg.conf`), from `[Interface]` to the last line, under **Key or subscription**, alone or together with the links. To pick the file under **Configuration file** instead, copy it to your home folder on the VPS with `sudo install -m 600 -o "$USER" /root/splitlane-kit/splitlane-wg.conf ~/` and then to your PC with `scp <user>@<VPS address>:splitlane-wg.conf .`; delete both copies once SplitLane has it.
- **Subscription:** paste the subscription address under **Key or subscription**. **Refresh** next to the subscription on **Connection** downloads it again after a run that keeps the address, for example with a new `--reality-sni` (a new `--host` keeps it only together with `--domain`: the default name comes from the address). After `--reset` the address changes: see below.

Then choose a server in the list and turn split routing on.

## Run again, start over, remove

- **A second run** keeps the keys and the links (they are stored in `/etc/splitlane-kit/`). Run it again to print them, to change an option, or to update the kit. A new port changes the links (and the `.conf` for `--wg-port`): add the new ones in SplitLane and remove the old ones, or click **Refresh** next to the subscription. A new `--protocols` list adds or takes away ways in the same way.
- **`--reset`** generates new keys, links and subscription address. The old ones stop working, and the old subscription address answers "not found", so **Refresh** can't pick up the change. In SplitLane, in this order: turn split routing off (the selected server and its subscription can't be removed while it is on), remove the old subscription and the old servers on **Connection**, then add the new subscription address (or the new links and `.conf`). That way two subscriptions of the same name with look-alike servers are never in the list at once.
- **`--uninstall`** stops and removes the `splitlane-kit` service and its system user, the kit's sing-box (`/usr/local/lib/splitlane-kit/sing-box`), `/etc/splitlane-kit`, the subscription site in Caddy (Caddy itself stays installed) and the kit's fail2ban jail (fail2ban and the automatic updates stay). It leaves `/root/splitlane-kit` for you to delete (`rm -rf /root/splitlane-kit`) and keeps the ufw rules it added (marked `splitlane-kit`; see them with `ufw status numbered`).

## Light hardening (`--harden`)

- **fail2ban guards SSH.** The kit installs fail2ban and adds the jail `sshd` in `/etc/fail2ban/jail.d/splitlane-kit.local`, which reads the systemd journal (Debian 12 has no `auth.log`). With fail2ban's defaults, 5 failed logins from one address within 10 minutes ban that address for 10 minutes. If that happens to you, wait, or run `sudo fail2ban-client unban <address>` from another address; `sudo fail2ban-client status sshd` lists the banned ones.
- **Security updates install themselves.** The kit installs `unattended-upgrades` and turns on its daily run in `/etc/apt/apt.conf.d/20auto-upgrades`, unless that file is already there: then it stays as it is.
- Nothing in the settings of SSH changes (password logins, the port). A failure here is a warning, not an error: the VPN server works without it. A re-run doubles nothing. `--uninstall` removes only the kit's jail. On Debian 12, fail2ban's own `sshd` jail then looks for `/var/log/auth.log`, which the system doesn't have, so fail2ban won't start again (after a reboot, for example): remove it with `sudo apt-get purge fail2ban`, or keep a jail with `backend = systemd` of your own.

## Security notes

- **Keys** are generated on the VPS by sing-box. They stay in `/etc/splitlane-kit` and `/root/splitlane-kit` (root only) and, once you add them, in SplitLane's `%ProgramData%\SplitLane\servers.json` on your PC (administrators only). Anyone who has a link or the `.conf` can use your server: treat them like passwords.
- **The subscription address is a secret.** The random token in its path is all that protects it: whoever has the address gets all the links. `--reset` changes it. On the VPS the token is in files that only root and Caddy can read (`/etc/caddy/splitlane-kit.caddy` and the served copy, `640 root:caddy`), but also in Caddy's admin API on `localhost:2019`, which any local account can ask. So the secrecy of the subscription assumes the VPS has no local accounts you don't trust.
- **Reality borrows the handshake of a public site** (`www.microsoft.com` by default): to anyone without the key, port 443/tcp of your VPS answers like that site, and the server connects to the site for these handshakes. With `--reality-sni` you can choose another site that speaks TLS 1.3.
- **Hysteria2 uses a self-signed certificate, and its link skips the certificate check** (`insecure=1`). The traffic is encrypted, but someone who can intercept the connection and answer in place of your server could pose as it and learn the Hysteria2 password. Where that matters, use VLESS + Reality or WireGuard.
- **The service runs as the user `splitlane-kit`** with only two capabilities: binding ports below 1024 (such as 443) and network administration for the core's socket options. The config stays readable by root only; systemd copies it to `/run/splitlane-kit` (in memory) when the service starts.
- **Logs:** `journalctl -u splitlane-kit`. The level is `warn`, so connections aren't logged; `LOG_LEVEL=info` at the top of the script logs every connection.

## If something doesn't work

- `systemctl status splitlane-kit` and `journalctl -u splitlane-kit -n 50` show why the server stopped.
- The subscription address doesn't open: Caddy may still be getting its certificate, or port 80 isn't reachable from the internet; see `journalctl -u caddy -n 50`.
- No connection at all: check that the ports are open in the firewall of your provider.
- Only Shadowsocks or WireGuard fails (SplitLane says it couldn't make sure the tunnel works and turns split routing off), while VLESS + Reality or Hysteria2 to the same server works: your internet provider probably blocks that protocol. Some providers (in Russia, Iran, China) recognize Shadowsocks and WireGuard by DPI and cut them: in SplitLane's own check in Russia, both were cut after the first packets, while VLESS + Reality and Hysteria2 got through. Use those two there.
