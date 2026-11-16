# SplitLane server kit

[Русская версия](README.ru.md)

`install-singbox-server.sh` turns a Linux VPS into a VPN server for SplitLane's **Key or subscription** and **Configuration file** connections. It is a helper for testing SplitLane and for running your own server; it isn't part of the Windows app, and SplitLane works with any server of the supported protocols.

## What it sets up

- **sing-box 1.14.4**, the same version as SplitLane's core, from the [official release](https://github.com/SagerNet/sing-box/releases/tag/v1.14.4). The script checks the SHA-256 of the archive against the values written in it and stops, installing nothing, if they differ.
- **One server with four ways in:**

  | Protocol | Port | Name in SplitLane |
  |---|---|---|
  | VLESS + Reality, `xtls-rprx-vision` | 443/tcp | `SplitLane-VLESS-Reality` |
  | Hysteria2, self-signed certificate | 443/udp | `SplitLane-Hysteria2` |
  | Shadowsocks 2022, `2022-blake3-aes-128-gcm` | 8388/tcp and udp | `SplitLane-SS2022` |
  | WireGuard | 51820/udp | the file `splitlane-wg.conf` |

  Connections from clients to loopback and private-network addresses (the VPS's own `127.0.0.1`, its provider's internal network) are refused. Services on the VPS's public address stay reachable through the tunnel, as from the internet.
- A systemd service `splitlane-kit` that runs as its own system user, not as root, and restarts after a failure.
- With `--subscription`: [Caddy](https://caddyserver.com) serves the three links as a subscription at `https://<name>:8443/<random token>/sub`, with a certificate from Let's Encrypt.

## Requirements

- A VPS with Debian 12 or later, or Ubuntu 22.04 or later, on x86_64 or arm64 (aarch64), with systemd; you log in as root or as a user with sudo.
- A public IPv4 address. The script asks `api.ipify.org` for it; with `--host` you give the address or a name yourself.
- Free ports 443/tcp, 443/udp, 8388/tcp, 8388/udp and 51820/udp. If your provider has a firewall in its control panel, open them there.
- For `--subscription`: Caddy in the distribution's packages (Debian 12 and later, Ubuntu 24.04 and later; Ubuntu 22.04 has none, so the step is skipped with a message), ports 80/tcp and 8443/tcp reachable from the internet (Let's Encrypt checks port 80) and a name that points to the VPS. By default the name is `<a-b-c-d>.sslip.io` for the address a.b.c.d: [sslip.io](https://sslip.io) resolves such names to the address in them, so you need no domain of your own.

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
| `--subscription` | Also serve the links as a subscription over HTTPS (Caddy, port 8443; port 80 for the certificate). |
| `--domain <name>` | The name of the subscription address and of the Hysteria2 certificate. Default: the address as an sslip.io name, such as `192-0-2-10.sslip.io`. |
| `--host <address or name>` | The server address in the links and in the `.conf`. Default: the public IPv4 address. |
| `--reality-sni <name>` | The site whose TLS 1.3 handshake Reality borrows. Default: `www.microsoft.com`. |
| `--reset` | New keys, links and subscription address; the old ones stop working. |
| `--uninstall` | Remove the kit (see below). |
| `--dry-run <folder>` | A test: write everything under the folder and check the config with `sing-box check`; no systemd, firewall, Caddy or apt. The address is `192.0.2.10` unless `--host` is given. |
| `-h`, `--help` | The list of options. |

`--host`, `--domain`, `--reality-sni` and `--subscription` are remembered for later runs, `--reset` included, until `--uninstall`; giving one again with another value replaces it. The port numbers, the Reality site and the log level are variables at the top of the script.

## What it prints

The three share links, the text of the WireGuard file and, with `--subscription`, the subscription address. For example (the secrets are cut here):

```text
vless://…@192.0.2.10:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com&fp=chrome&pbk=…&sid=…&type=tcp&headerType=none#SplitLane-VLESS-Reality
hysteria2://…@192.0.2.10:443?sni=192-0-2-10.sslip.io&insecure=1#SplitLane-Hysteria2
ss://2022-blake3-aes-128-gcm:…@192.0.2.10:8388#SplitLane-SS2022
https://192-0-2-10.sslip.io:8443/…/sub
```

The same is saved in `/root/splitlane-kit/`, which only root can open: `links.txt` (the three links), `splitlane-wg.conf` (WireGuard) and `subscription.txt` (the three links in base64, the format of subscriptions).

If ufw is active, the script allows the ports in it before the server and Caddy start; otherwise it lists the ports to open in your provider's firewall.

## Add the server to SplitLane

- **Links:** on **Connect a VPN** (or later on **Connection**), choose **Key or subscription**, paste one link or all three, one per line, and click **Add**.
- **WireGuard:** paste the text the script printed (or `sudo cat /root/splitlane-kit/splitlane-wg.conf`) under **Key or subscription**. To pick the file under **Configuration file** instead, copy it to your home folder on the VPS with `sudo install -m 600 -o "$USER" /root/splitlane-kit/splitlane-wg.conf ~/` and then to your PC with `scp <user>@<VPS address>:splitlane-wg.conf .`; delete both copies once SplitLane has it.
- **Subscription:** paste the subscription address under **Key or subscription**. **Refresh** next to the subscription on **Connection** downloads it again after a run that keeps the address, for example with a new `--reality-sni` (a new `--host` keeps it only together with `--domain`: the default name comes from the address). After `--reset` the address changes: see below.

Then choose a server in the list and turn split routing on.

## Run again, start over, remove

- **A second run** keeps the keys and the links (they are stored in `/etc/splitlane-kit/`). Run it again to print them, to change an option, or to update the kit.
- **`--reset`** generates new keys, links and subscription address. The old ones stop working, and the old subscription address answers "not found", so **Refresh** can't pick up the change. In SplitLane, in this order: turn split routing off (the selected server and its subscription can't be removed while it is on), remove the old subscription and the old servers on **Connection**, then add the new subscription address (or the new links and `.conf`). That way two subscriptions of the same name with look-alike servers are never in the list at once.
- **`--uninstall`** stops and removes the `splitlane-kit` service and its system user, `/usr/local/bin/sing-box`, `/etc/splitlane-kit` and the subscription site in Caddy (Caddy itself stays installed). It leaves `/root/splitlane-kit` for you to delete (`rm -rf /root/splitlane-kit`) and keeps the ufw rules it added (marked `splitlane-kit`; see them with `ufw status numbered`).

## Security notes

- **Keys** are generated on the VPS by sing-box. They stay in `/etc/splitlane-kit` and `/root/splitlane-kit` (root only) and, once you add them, in SplitLane's `%ProgramData%\SplitLane\servers.json` on your PC (administrators only). Anyone who has a link or the `.conf` can use your server: treat them like passwords.
- **The subscription address is a secret.** The random token in its path is all that protects it: whoever has the address gets all three links. `--reset` changes it. On the VPS the token is in files that only root and Caddy can read (`/etc/caddy/splitlane-kit.caddy` and the served copy, `640 root:caddy`), but also in Caddy's admin API on `localhost:2019`, which any local account can ask. So the secrecy of the subscription assumes the VPS has no local accounts you don't trust.
- **Reality borrows the handshake of a public site** (`www.microsoft.com` by default): to anyone without the key, port 443/tcp of your VPS answers like that site, and the server connects to the site for these handshakes. With `--reality-sni` you can choose another site that speaks TLS 1.3.
- **Hysteria2 uses a self-signed certificate, and its link skips the certificate check** (`insecure=1`). The traffic is encrypted, but someone who can intercept the connection and answer in place of your server could pose as it and learn the Hysteria2 password. Where that matters, use VLESS + Reality or WireGuard.
- **The service runs as the user `splitlane-kit`** with only two capabilities: binding port 443 and network administration for the core's socket options. The config stays readable by root only; systemd copies it to `/run/splitlane-kit` (in memory) when the service starts.
- **Logs:** `journalctl -u splitlane-kit`. The level is `warn`, so connections aren't logged; `LOG_LEVEL=info` at the top of the script logs every connection.

## If something doesn't work

- `systemctl status splitlane-kit` and `journalctl -u splitlane-kit -n 50` show why the server stopped.
- The subscription address doesn't open: Caddy may still be getting its certificate, or port 80 isn't reachable from the internet; see `journalctl -u caddy -n 50`.
- No connection at all: check that the ports are open in the firewall of your provider.
