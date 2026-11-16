#!/usr/bin/env bash
# The SplitLane server kit: a sing-box server on a Linux VPS for SplitLane's "Key or subscription"
# and "Configuration file" connections.
#
# Installs sing-box 1.14.4 (the version of SplitLane's own core) from the official release after
# checking the archive against the SHA-256 pinned below, generates the keys and sets up a server
# with up to four ways in (all four unless --protocols chooses):
#   VLESS + Reality + xtls-rprx-vision   443/tcp
#   Hysteria2 (self-signed certificate)  443/udp
#   Shadowsocks 2022                     8388/tcp+udp
#   WireGuard                            51820/udp
# (the default ports; --vless-port, --hy2-port, --ss-port and --wg-port change them). It prints
# the share links, a WireGuard .conf and a subscription file and keeps them in /root/splitlane-kit/.
# With --subscription, Caddy also serves the links as a subscription over HTTPS; with --harden,
# fail2ban guards SSH and security updates install themselves. For Debian 12+ and Ubuntu 22.04+
# on x86_64 or aarch64, run as root. A re-run keeps the keys and the links. --json is the output
# for SplitLane's "Your own VPS", which runs this kit over SSH. --help lists the options.
#
# https://github.com/splitlane-app/splitlane/tree/main/scripts/server
set -euo pipefail
umask 077

# ---- Settings --------------------------------------------------------------------------------

SINGBOX_VERSION=1.14.4
# SHA-256 of the official archives sing-box-1.14.4-linux-{amd64,arm64}.tar.gz of
# https://github.com/SagerNet/sing-box/releases/tag/v1.14.4; GitHub shows the same values as the
# digests of these release assets.
SHA256_AMD64=699a1b510aa4396b2176d32d1181920399ebf1e267835da6f3b598d0f7c24968
SHA256_ARM64=436441f76ee2f7fe21b5b2d426d3cb261a480455a704865789ad533e4273ec26

# The default ports: VLESS + Reality (tcp), Hysteria2 (udp), Shadowsocks 2022 (tcp and udp),
# WireGuard (udp) and the subscription (Caddy, HTTPS, tcp). The option of each is its name in
# lower case with "--" before and "-port" after (VLESS_PORT: --vless-port); choose_ports sets them.
PORT_KEYS=(VLESS_PORT HY2_PORT SS_PORT WG_PORT SUB_PORT)
declare -A PORT_DEFAULT=([VLESS_PORT]=443 [HY2_PORT]=443 [SS_PORT]=8388 [WG_PORT]=51820 [SUB_PORT]=8443)
VLESS_PORT="" HY2_PORT="" SS_PORT="" WG_PORT="" SUB_PORT=""
ACME_PORT=80 # Caddy gets the certificate of the subscription over HTTP here, tcp
# The kinds of each port, and the ports --auto-ports tries in this order when the option gives
# none and none is remembered. The subscription takes its port on udp too: Caddy serves HTTP/3
# there and does not start if the udp port is taken (by Hysteria2, say). The ports of VLESS and of
# the subscription never overlap.
declare -A PORT_KINDS=([VLESS_PORT]=tcp [HY2_PORT]=udp [SS_PORT]="tcp udp" [WG_PORT]=udp [SUB_PORT]="tcp udp")
declare -A AUTO_PORT_LIST=([VLESS_PORT]="443 2053 2083 2087 2096" [HY2_PORT]="443 4443 8443 10443"
	[SS_PORT]="8388 8389 9388" [WG_PORT]="51820 51821 51822" [SUB_PORT]="8443 9443 7443")

# The ways in, in the order of the links and of PROTOCOLS in the state. The port option of each is
# --<name>-port (vless: --vless-port).
ALL_PROTOCOLS=(vless hy2 ss wg)

# A common site that speaks TLS 1.3: Reality answers anyone who is not a client with its handshake.
REALITY_SNI_DEFAULT=www.microsoft.com
SS_METHOD=2022-blake3-aes-128-gcm
WG_SERVER_ADDRESSES=(10.66.0.1/24 fd66::1/64)
WG_CLIENT_ADDRESSES=(10.66.0.2/32 fd66::2/128)
WG_CLIENT_DNS=1.1.1.1
WG_KEEPALIVE=25
WG_MTU=1408
LOG_LEVEL=warn # info logs every connection: journalctl -u splitlane-kit
IP_SERVICE=https://api.ipify.org
DRY_RUN_HOST=192.0.2.10

SERVICE=splitlane-kit
SERVICE_USER=splitlane-kit

# Where things go. A dry run puts them under its own directory (ROOT). The binary is not on PATH,
# so a sing-box of the system or of another server stays the one that the command sing-box runs.
P_BIN=/usr/local/lib/splitlane-kit/sing-box
P_STATE=/etc/splitlane-kit
P_OUT=/root/splitlane-kit
P_WWW_ROOT=/var/lib/splitlane-kit
P_WWW=$P_WWW_ROOT/www
P_UNIT=/etc/systemd/system/$SERVICE.service
P_CADDYFILE=/etc/caddy/Caddyfile
P_CADDY_SITE=/etc/caddy/$SERVICE.caddy
# One run of the kit at a time (take_lock); /run is emptied at each boot.
P_LOCK=/run/$SERVICE.lock
LOCK_WAIT=900 # seconds
CADDY_MARK="# Added by the SplitLane server kit; install-singbox-server.sh --uninstall removes it."
CADDY_IMPORT="import $P_CADDY_SITE"
# --harden: the kit's fail2ban jail and the switch of automatic updates (written only when the
# system has none). Both stay after --uninstall, as fail2ban and Caddy do.
P_JAIL=/etc/fail2ban/jail.d/$SERVICE.local
P_AUTO_UPGRADES=/etc/apt/apt.conf.d/20auto-upgrades
HARDEN_PACKAGES=(fail2ban python3-systemd unattended-upgrades)

# The state file keeps these values between runs. --reset forgets the generated ones; the
# settings (from --host, --domain, --reality-sni, --protocols, the port options, --subscription)
# and the record of the installed binary stay.
GENERATED_KEYS=(UUID REALITY_PRIVATE_KEY REALITY_PUBLIC_KEY SHORT_ID HY2_PASSWORD HY2_CERT_NAME SS_PASSWORD
	WG_SERVER_PRIVATE_KEY WG_SERVER_PUBLIC_KEY WG_CLIENT_PRIVATE_KEY WG_CLIENT_PUBLIC_KEY WG_PRESHARED_KEY SUB_TOKEN)
SETTING_KEYS=(HOST DOMAIN REALITY_SNI PROTOCOLS "${PORT_KEYS[@]}" SUBSCRIPTION BINARY)
declare -A ST=()

# ---- Helpers ---------------------------------------------------------------------------------

# --json: the stdout of the caller is fd 3 and gets only JSON lines (steps, then one result or
# error line); everything else goes to stderr, except the output of apt and dpkg (see pkg_run).
# FINAL is set once the result or error line is out. A --json run goes on to its end when its client
# has gone (a cancelled or cut SSH session closes the pipes of stdout and stderr), so that it never
# leaves the server half set up: SIGPIPE is ignored (see main) and a write that fails is no reason to
# stop. In text mode a write that fails stops the run, as before.
JSON=0 FINAL=0 TMP=""
WARNINGS=() LAST_WARNING=""

say() { printf '%s\n' "$*" || ((JSON)); }
warn() { printf 'warning: %s\n' "$*" >&2 || ((JSON)); }
# A warning that SplitLane shows: its code goes into the result of --json.
warn_code() {
	WARNINGS+=("$1")
	shift
	LAST_WARNING=$*
	warn "$@"
}
# Stops the kit. The code tells SplitLane what went wrong: os_unsupported, download_failed,
# ports_busy or kit_failed.
die_code() {
	local code=$1
	shift
	printf 'error: %s\n' "$*" >&2 || true
	if ((JSON && !FINAL)); then
		FINAL=1
		printf '{"error":{"code":"%s","message":%s}}\n' "$code" "$(json_str "$*")" >&3 || true
	fi
	exit 1
}
die() { die_code kit_failed "$@"; }

# $1 as a JSON string: backslashes, quotes and control characters escaped.
json_str() {
	local s=$1 bs=\\ c hex LC_ALL=C re=$'[\x01-\x1f]'
	s=${s//"$bs"/"$bs$bs"}
	s=${s//\"/"$bs\""}
	s=${s//$'\n'/"${bs}n"}
	s=${s//$'\t'/"${bs}t"}
	s=${s//$'\r'/"${bs}r"}
	while [[ $s =~ $re ]]; do
		c=${BASH_REMATCH[0]}
		printf -v hex '\\u%04x' "'$c"
		s=${s//"$c"/"$hex"}
	done
	printf '"%s"' "$s"
}

# A step line of --json: step (system, singbox, config, service, harden, subscription), state
# (start, done, skip, warn) and a detail that never holds a secret. Nothing in text mode.
step() {
	if ((JSON)); then
		printf '{"step":"%s","state":"%s","detail":%s}\n' "$1" "$2" "$(json_str "$3")" >&3 || true
	fi
}

usage() {
	cat <<'EOF'
Usage: sudo bash install-singbox-server.sh [options]

Sets up a sing-box 1.14.4 server for SplitLane on this Debian 12+ or Ubuntu 22.04+ VPS:
VLESS + Reality (443/tcp), Hysteria2 (443/udp), Shadowsocks 2022 (8388/tcp+udp) and
WireGuard (51820/udp), on these ports by default. Prints the share links and keeps them, a
WireGuard .conf and a subscription file in /root/splitlane-kit/. A re-run keeps the keys and
the links.

Options:
  --protocols <list>    the ways in, comma-separated from vless, hy2, ss, wg (default: all four)
  --default-protocols <list>
                        the ways in of a new installation; a server this kit set up before keeps
                        its own (all four, if they were never chosen)
  --subscription        also serve the links as a subscription over HTTPS: Caddy on port 8443,
                        port 80 for its certificate (Caddy must be in the distribution's packages)
  --harden              also install fail2ban for SSH and automatic security updates
  --domain <name>       the name of the subscription address and of the Hysteria2 certificate
                        (default: the address as an sslip.io name, e.g. 192-0-2-10.sslip.io)
  --host <ip-or-name>   the server address in the links (default: the public IPv4 address,
                        from api.ipify.org)
  --reality-sni <host>  the TLS 1.3 site whose handshake Reality borrows (default: www.microsoft.com)
  --vless-port <port>   the port of VLESS + Reality, tcp (default 443)
  --hy2-port <port>     the port of Hysteria2, udp (default 443)
  --ss-port <port>      the port of Shadowsocks 2022, tcp and udp (default 8388)
  --wg-port <port>      the port of WireGuard, udp (default 51820)
  --sub-port <port>     the port of the subscription, tcp, also taken on udp by Caddy's HTTP/3
                        (default 8443; Caddy needs port 80 too)
  --auto-ports          for a port neither given nor remembered: the first free one of a short
                        list (VLESS 443, 2053, 2083, 2087, 2096; Hysteria2 443, 4443, 8443, 10443;
                        Shadowsocks 8388, 8389, 9388; WireGuard 51820, 51821, 51822; the
                        subscription 8443, 9443, 7443) instead of the default
  --reset               new keys, links and subscription address
  --uninstall           remove the service, sing-box, the settings and the subscription site (the
                        fail2ban and the updates of --harden stay)
  --json                the output for programs (SplitLane's "Your own VPS" runs the kit with
                        --json --default-protocols vless,hy2 --auto-ports --harden --subscription,
                        and --host when the address typed there is a public IPv4 address): one JSON
                        object per line on stdout, the text on stderr; the run goes on to its end
                        when the program that started it has gone
  --dry-run <dir>       a test: write everything under <dir> and check the config; no systemd,
                        firewall, Caddy or apt (address 192.0.2.10 unless --host is given)
  -h, --help            this help

--host, --domain, --reality-sni, the protocols (of --protocols, or of --default-protocols on a
new installation), the ports (those of --auto-ports too) and --subscription are remembered for
later runs, --reset included, until --uninstall. After --reset, add the new links or subscription
address in SplitLane and remove the old ones there. Runs go one at a time: a run waits up to 15
minutes for another one that is still working.
EOF
}

need_value() {
	if (($# < 2)) || [[ -z $2 || $2 == --* ]]; then
		die "$1 needs a value (see --help)"
	fi
}

is_ipv4() {
	local IFS=. o
	local -a parts
	[[ $1 =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || return 1
	read -r -a parts <<<"$1"
	for o in "${parts[@]}"; do
		((10#$o <= 255)) || return 1
	done
}

is_ipv6() { [[ $1 == *:* && $1 =~ ^[0-9A-Fa-f:.]+$ ]]; }

# The full form of an IPv6 address (eight groups of four digits) joined by $2. Fails on anything
# else, the IPv4-mapped form included. Joined by '-' it is an sslip.io name whose labels never
# start or end with '-', as the short form with "::" would.
ipv6_full() {
	local a=${1,,} sep=$2 head tail g i missing=0 out=""
	local -a h=() t=() all=()
	[[ $a =~ ^[0-9a-f:]+$ ]] || return 1
	if [[ $a == *::* ]]; then
		head=${a%%::*} tail=${a#*::}
		[[ $tail != *::* ]] || return 1
	else
		head=$a tail=""
	fi
	if [[ -n $head ]]; then
		IFS=: read -r -a h <<<"$head"
	fi
	if [[ -n $tail ]]; then
		IFS=: read -r -a t <<<"$tail"
	fi
	if [[ $a == *::* ]]; then
		missing=$((8 - ${#h[@]} - ${#t[@]}))
		((missing >= 1)) || return 1
	elif ((${#h[@]} != 8)); then
		return 1
	fi
	all=("${h[@]}")
	for ((i = 0; i < missing; i++)); do
		all+=(0)
	done
	all+=("${t[@]}")
	for g in "${all[@]}"; do
		[[ $g =~ ^[0-9a-f]{1,4}$ ]] || return 1
		out+="${out:+$sep}$(printf '%04x' "$((16#$g))")"
	done
	printf '%s' "$out"
}

is_name() {
	((${#1} <= 253)) && [[ $1 =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]]
}

sha256() {
	local s
	s=$(sha256sum "$1")
	printf '%s' "${s%% *}"
}

# apt-get or dpkg ($@) with the usual umask: packages and their scripts must not get the 077 of this
# script. In --json their output goes to a log in the temporary folder, not to the pipes of the
# session: apt, dpkg or a package script that cannot write there when the client has gone would
# fail and leave dpkg half done. The last lines of the log go to stderr when they fail. They run
# with the usual SIGPIPE and without the kit's lock (fd 9), which a daemon started by a package
# script must not keep.
pkg_run() {
	local rc=0
	if ((JSON)); then
		(umask 022 && trap - PIPE && DEBIAN_FRONTEND=noninteractive "$@") >"$TMP/apt.log" 2>&1 9>&- || rc=$?
		if ((rc)); then
			tail -n 20 -- "$TMP/apt.log" >&2 || true
		fi
		return "$rc"
	fi
	(umask 022 && DEBIAN_FRONTEND=noninteractive "$@") 9>&-
}

# apt-get, waiting up to 3 minutes for the lock of dpkg, which a fresh VPS often holds for its own
# first updates.
apt_get() { pkg_run apt-get -o DPkg::Lock::Timeout=180 "$@"; }

# apt-get update, tried 3 times 20 s apart: DPkg::Lock::Timeout does not cover the lock of the
# package lists, which apt-daily or cloud-init may hold on a fresh VPS.
apt_update() {
	local i
	for i in 1 2 3; do
		if apt_get update -qq; then
			return 0
		fi
		if ((i < 3)); then
			say "apt-get update failed; trying again in 20 s"
			sleep 20
		fi
	done
	return 1
}

# apt-get install of the packages $@, its stdout dropped. Before the first one, dpkg finishes what a
# run cut earlier (of the kit or of the system's own updates) left half done: apt refuses to install
# after an interrupted dpkg, and dpkg --audit lists what such a run left.
DPKG_AUDITED=0
apt_install() {
	if ((!DPKG_AUDITED)); then
		DPKG_AUDITED=1
		if [[ -n $(dpkg --audit 2>/dev/null || true) ]]; then
			say "dpkg was interrupted before; finishing its work (dpkg --configure -a)"
			pkg_run dpkg --configure -a || warn "dpkg --configure -a failed"
		fi
	fi
	apt_get install -y -qq "$@" >/dev/null
}

# The value after "<name>: " in the output of sing-box generate.
field() { sed -n "s/^$1: //p" <<<"$2"; }

# A list as text: a, b.
join_list() {
	local out="" v
	for v in "$@"; do
		out+="${out:+, }$v"
	done
	printf '%s' "$out"
}

# A list as JSON strings: "a", "b".
json_list() {
	local out="" v
	for v in "$@"; do
		out+="${out:+, }\"$v\""
	done
	printf '%s' "$out"
}

# Appends the text on stdin (a JSON object) to the list in the variable named $1, each item on new
# lines after a comma.
add_block() {
	local block
	block=$(cat)
	printf -v "$1" '%s%s\n%s' "${!1}" "${!1:+,}" "$block"
}

# The lines of a PEM file as JSON strings.
pem_json() {
	local out="" line
	while IFS= read -r line || [[ -n $line ]]; do
		if [[ -n $line ]]; then
			out+="${out:+, }\"$line\""
		fi
	done <"$1"
	printf '%s' "$out"
}

# Percent-encoding of a base64 key for the user part of an ss:// link (SIP002 asks for it with
# Shadowsocks 2022).
pct() {
	local s=$1
	s=${s//+/%2B}
	s=${s//\//%2F}
	s=${s//=/%3D}
	printf '%s' "$s"
}

# The protocol list $2 of $1 (an option, or the state) in the order of ALL_PROTOCOLS, into the
# variable named $3. Each of vless, hy2, ss, wg may be there once.
protocol_list() {
	local p out=""
	local -a items=()
	local -A seen=()
	[[ $2 =~ ^[a-z0-9]+(,[a-z0-9]+)*$ ]] || die "$1 needs a comma-separated list of vless, hy2, ss, wg, not '$2'"
	IFS=, read -r -a items <<<"$2"
	for p in "${items[@]}"; do
		case $p in
		vless | hy2 | ss | wg) ;;
		*) die "$1: '$p' is not one of vless, hy2, ss, wg" ;;
		esac
		[[ -z ${seen[$p]:-} ]] || die "$1: $p is in the list twice"
		seen[$p]=1
	done
	for p in "${ALL_PROTOCOLS[@]}"; do
		if [[ -n ${seen[$p]:-} ]]; then
			out+="${out:+,}$p"
		fi
	done
	printf -v "$3" '%s' "$out"
}

uses_protocol() { [[ ",$PROTOCOLS," == *",$1,"* ]]; }

# ---- Arguments -------------------------------------------------------------------------------

SUBSCRIPTION_FLAG="" DOMAIN_FLAG="" HOST_FLAG="" SNI_FLAG="" RESET=0 UNINSTALL=0 DRY_RUN=0 ROOT=""
PROTOCOLS_FLAG="" DEFAULT_PROTOCOLS_FLAG="" AUTO_PORTS=0 HARDEN=0
declare -A PORT_FLAG=()

parse_args() {
	local k
	while (($#)); do
		case $1 in
		--json) JSON=1 ;;
		--protocols)
			need_value "$@"
			protocol_list "$1" "$2" PROTOCOLS_FLAG
			shift
			;;
		--default-protocols)
			need_value "$@"
			protocol_list "$1" "$2" DEFAULT_PROTOCOLS_FLAG
			shift
			;;
		--auto-ports) AUTO_PORTS=1 ;;
		--harden) HARDEN=1 ;;
		--subscription) SUBSCRIPTION_FLAG=1 ;;
		--vless-port | --hy2-port | --ss-port | --wg-port | --sub-port)
			need_value "$@"
			k=${1#--}
			k=${k%-port}
			PORT_FLAG[${k^^}_PORT]=$2
			shift
			;;
		--domain)
			need_value "$@"
			DOMAIN_FLAG=$2
			shift
			;;
		--host)
			need_value "$@"
			HOST_FLAG=$2
			shift
			;;
		--reality-sni)
			need_value "$@"
			SNI_FLAG=$2
			shift
			;;
		--reset) RESET=1 ;;
		--uninstall) UNINSTALL=1 ;;
		--dry-run)
			need_value "$@"
			DRY_RUN=1
			ROOT=$2
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*) die "unknown option: $1 (see --help)" ;;
		esac
		shift
	done
	if ((UNINSTALL)) && [[ -n $SUBSCRIPTION_FLAG$DOMAIN_FLAG$HOST_FLAG$SNI_FLAG$PROTOCOLS_FLAG$DEFAULT_PROTOCOLS_FLAG ||
		$RESET$AUTO_PORTS$HARDEN != 000 || ${#PORT_FLAG[@]} != 0 ]]; then
		die "--uninstall takes no other options (except --dry-run and --json)"
	fi
	if ((DRY_RUN)); then
		mkdir -p -- "$ROOT"
		ROOT=$(cd -- "$ROOT" && pwd)
		[[ $ROOT != / ]] || die "--dry-run needs a directory other than /"
	fi
}

# ---- Checks ----------------------------------------------------------------------------------

os_field() { sed -n "s/^$1=//p" /etc/os-release | tr -d '"'; }

preflight() {
	case $(uname -m) in
	x86_64 | amd64) ARCH=amd64 SHA256=$SHA256_AMD64 ;;
	aarch64 | arm64) ARCH=arm64 SHA256=$SHA256_ARM64 ;;
	*) die_code os_unsupported "unsupported CPU architecture $(uname -m): the kit supports x86_64 and aarch64" ;;
	esac
	SYSTEM_TEXT="dry run, $ARCH"
	if ((!DRY_RUN)); then
		((EUID == 0)) || die "run it as root: sudo bash $0"
		local id version major need
		id=$(os_field ID 2>/dev/null || true)
		version=$(os_field VERSION_ID 2>/dev/null || true)
		major=${version%%.*}
		case $id in
		debian) need=12 ;;
		ubuntu) need=22 ;;
		*) die_code os_unsupported "this system (${id:-unknown}) is not supported: Debian 12+ or Ubuntu 22.04+ is needed" ;;
		esac
		if ! [[ $major =~ ^[0-9]+$ ]] || ((major < need)); then
			die_code os_unsupported "$id ${version:-without a version} is not supported: Debian 12+ or Ubuntu 22.04+ is needed"
		fi
		[[ -d /run/systemd/system ]] || die_code os_unsupported "systemd is not running here; the kit needs it"
		SYSTEM_TEXT="$id $version, $ARCH"
		if ! command -v curl >/dev/null 2>&1; then
			say "Installing curl"
			if ! apt_update || ! apt_install curl ca-certificates; then
				die_code download_failed "could not install curl with apt-get"
			fi
		fi
	fi
	local c
	for c in curl tar sha256sum base64 install sed; do
		command -v "$c" >/dev/null 2>&1 || die "$c is missing"
	done
}

# The main process of the kit's own server on a re-run: its sockets never make a port busy. A dry
# run asks no systemd.
KIT_PID=""
find_kit_pid() {
	if ((!DRY_RUN)) && systemctl is-active --quiet "$SERVICE"; then
		KIT_PID=$(systemctl show -p MainPID --value "$SERVICE" 2>/dev/null || true)
	fi
}

# Whether a program other than the kit's own server listens on port $1, kind $2 (tcp or udp).
# With $3 = caddy, Caddy does not count either: the kit's subscription site joins it there.
port_busy() {
	command -v ss >/dev/null 2>&1 || return 1
	local opt=-lntp lines
	if [[ $2 == udp ]]; then
		opt=-lnup
	fi
	lines=$(ss -H "$opt" "sport = :$1" 2>/dev/null || true)
	if [[ $KIT_PID =~ ^[1-9][0-9]*$ ]]; then
		lines=$(grep -vF -- "pid=$KIT_PID," <<<"$lines" || true)
	fi
	if [[ ${3:-} == caddy ]]; then
		lines=$(grep -vF -- '(("caddy",' <<<"$lines" || true)
	fi
	[[ -n $lines ]]
}

# Ports of the ways in that another program already listens on.
check_ports() {
	local p busy=()
	for p in "${SERVER_PORTS[@]}"; do
		if port_busy "${p%/*}" "${p#*/}"; then
			busy+=("$p")
		fi
	done
	if ((${#busy[@]})); then
		die_code ports_busy "another program already uses ${busy[*]} (see: ss -lntup); free them or choose other ports with --vless-port, --hy2-port, --ss-port, --wg-port"
	fi
}

# ---- State -----------------------------------------------------------------------------------

# Runs of the kit one at a time, before one reads the state: a --json run whose client has gone
# keeps working, and a run started meanwhile (SplitLane's "Try again") waits for it and then finds
# the server as that run left it. The lock is on fd 9 and goes with the process, however it ends. A
# dry run takes it under its directory.
take_lock() {
	local file=$ROOT$P_LOCK
	command -v flock >/dev/null 2>&1 || die "flock is missing"
	mkdir -p -- "$(dirname -- "$file")"
	exec 9>>"$file" || die "could not open $file"
	if ! flock -n 9; then
		say "Another run of the kit is working here; waiting for it to end (up to $((LOCK_WAIT / 60)) minutes)"
		flock -w "$LOCK_WAIT" 9 || die "another run of the kit is still working"
	fi
}

# STATE_FOUND: the kit has been here before (the protocols of such a server stay; see
# choose_protocols).
STATE_FOUND=0
load_state() {
	local file=$ROOT$P_STATE/state line k v
	if [[ -f $file ]]; then
		STATE_FOUND=1
		while IFS= read -r line || [[ -n $line ]]; do
			k=${line%%=*} v=${line#*=}
			if [[ " ${GENERATED_KEYS[*]} ${SETTING_KEYS[*]} " == *" $k "* ]]; then
				ST[$k]=$v
			fi
		done <"$file"
	fi
	if ((RESET)); then
		for k in "${GENERATED_KEYS[@]}"; do
			unset "ST[$k]"
		done
	fi
}

save_state() {
	local dir=$ROOT$P_STATE k
	mkdir -p -- "$dir"
	chmod 700 "$dir"
	for k in "${GENERATED_KEYS[@]}" "${SETTING_KEYS[@]}"; do
		if [[ -n ${ST[$k]:-} ]]; then
			printf '%s=%s\n' "$k" "${ST[$k]}"
		fi
	done >"$dir/state.new"
	chmod 600 "$dir/state.new"
	mv -f -- "$dir/state.new" "$dir/state"
}

# The form of each generated value; the settings are checked where they are used.
WG_KEY_FORM='^[A-Za-z0-9+/]{43}=$'
declare -A FORM=(
	[UUID]='^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
	[REALITY_PRIVATE_KEY]='^[A-Za-z0-9_-]{43}$'
	[REALITY_PUBLIC_KEY]='^[A-Za-z0-9_-]{43}$'
	[SHORT_ID]='^[0-9a-f]{16}$'
	[HY2_PASSWORD]='^[0-9a-f]{32}$'
	[HY2_CERT_NAME]='^[a-z0-9.-]{1,253}$'
	[SS_PASSWORD]='^[A-Za-z0-9+/]{22}==$'
	[WG_SERVER_PRIVATE_KEY]=$WG_KEY_FORM
	[WG_SERVER_PUBLIC_KEY]=$WG_KEY_FORM
	[WG_CLIENT_PRIVATE_KEY]=$WG_KEY_FORM
	[WG_CLIENT_PUBLIC_KEY]=$WG_KEY_FORM
	[WG_PRESHARED_KEY]=$WG_KEY_FORM
	[SUB_TOKEN]='^[0-9a-f]{24}$'
	[SUBSCRIPTION]='^[01]$'
	[BINARY]='^[0-9.]+:[0-9a-f]{64}$'
)

# check_state all: every generated value is there and has its form; check_state loaded: those
# that are there have it.
check_state() {
	local k
	for k in "${!FORM[@]}"; do
		if [[ -n ${ST[$k]:-} ]] || { [[ $1 == all ]] && [[ " ${GENERATED_KEYS[*]} " == *" $k "* ]]; }; then
			[[ ${ST[$k]:-} =~ ${FORM[$k]} ]] || die "$ROOT$P_STATE/state: $k is damaged; run again with --reset for new keys"
		fi
	done
}

# ---- Settings of this run --------------------------------------------------------------------

choose_settings() {
	if [[ -n $HOST_FLAG ]]; then
		HOST=$HOST_FLAG
	elif [[ -n ${ST[HOST]:-} ]]; then
		HOST=${ST[HOST]}
	elif ((DRY_RUN)); then
		HOST=$DRY_RUN_HOST
	else
		HOST=$(curl -fsS4 --max-time 15 "$IP_SERVICE" 2>/dev/null || true)
		is_ipv4 "$HOST" || die "could not learn the public IPv4 address from $IP_SERVICE; give it with --host <ip-or-name>"
	fi
	HOST=${HOST#[}
	HOST=${HOST%]}
	if is_ipv6 "$HOST"; then
		HOST=${HOST,,}
		ipv6_full "$HOST" : >/dev/null || die "--host: '$HOST' is not an IPv6 address the kit can use"
		HOST_URI="[$HOST]"
	elif is_ipv4 "$HOST" || is_name "$HOST"; then
		HOST_URI=$HOST
	else
		die "--host must be an IP address or a host name, not '$HOST'"
	fi
	if [[ -n $HOST_FLAG ]]; then
		ST[HOST]=$HOST
	fi

	if [[ -n $DOMAIN_FLAG ]]; then
		DOMAIN=$DOMAIN_FLAG
	elif [[ -n ${ST[DOMAIN]:-} ]]; then
		DOMAIN=${ST[DOMAIN]}
	elif is_ipv4 "$HOST"; then
		DOMAIN=${HOST//./-}.sslip.io
	elif is_ipv6 "$HOST"; then
		DOMAIN=$(ipv6_full "$HOST" -).sslip.io
	else
		DOMAIN=$HOST
	fi
	DOMAIN=${DOMAIN,,}
	if ! is_name "$DOMAIN" || is_ipv4 "$DOMAIN"; then
		die "--domain must be a host name, not '$DOMAIN'"
	fi
	if [[ -n $DOMAIN_FLAG ]]; then
		ST[DOMAIN]=$DOMAIN
	fi

	if [[ -n $SNI_FLAG ]]; then
		REALITY_SNI=$SNI_FLAG
	else
		REALITY_SNI=${ST[REALITY_SNI]:-$REALITY_SNI_DEFAULT}
	fi
	REALITY_SNI=${REALITY_SNI,,}
	if ! is_name "$REALITY_SNI" || is_ipv4 "$REALITY_SNI"; then
		die "--reality-sni must be a host name, not '$REALITY_SNI'"
	fi
	if [[ -n $SNI_FLAG ]]; then
		ST[REALITY_SNI]=$REALITY_SNI
		if ((!DRY_RUN)) && ! curl -sS --tlsv1.3 --max-time 10 -o /dev/null "https://$REALITY_SNI/" 2>/dev/null; then
			warn "https://$REALITY_SNI/ did not answer over TLS 1.3 from here; Reality needs a site that does"
		fi
	fi

	SUBSCRIPTION=${SUBSCRIPTION_FLAG:-${ST[SUBSCRIPTION]:-0}}
	choose_protocols
	choose_ports
}

# The ways in of this run: from --protocols, else the remembered ones, else all four on a server
# the kit set up before protocols could be chosen (SplitLane's --default-protocols must not take
# away the ones in use there), else --default-protocols, else all four. The ones from an option
# are remembered.
choose_protocols() {
	if [[ -n $PROTOCOLS_FLAG ]]; then
		PROTOCOLS=$PROTOCOLS_FLAG
		ST[PROTOCOLS]=$PROTOCOLS
	elif [[ -n ${ST[PROTOCOLS]:-} ]]; then
		protocol_list "$ROOT$P_STATE/state: PROTOCOLS" "${ST[PROTOCOLS]}" PROTOCOLS
	elif ((!STATE_FOUND)) && [[ -n $DEFAULT_PROTOCOLS_FLAG ]]; then
		PROTOCOLS=$DEFAULT_PROTOCOLS_FLAG
		ST[PROTOCOLS]=$PROTOCOLS
	else
		local IFS=,
		PROTOCOLS="${ALL_PROTOCOLS[*]}"
	fi
}

# The option of port key $1 (VLESS_PORT: --vless-port), into the variable named $2.
port_option() {
	local name=${1%_PORT}
	printf -v "$2" '%s' "--${name,,}-port"
}

# Whether this run listens on the port of key $1: the port of a chosen way in, or of the
# subscription.
port_used() {
	local name=${1%_PORT}
	name=${name,,}
	if [[ $name == sub ]]; then
		[[ $SUBSCRIPTION == 1 ]]
	else
		uses_protocol "$name"
	fi
}

# The ports of this run: from the option, else the remembered one, else (with --auto-ports) the
# first free one of its list, which is then remembered, else the default. Two ways in of this run
# never share a port of the same kind (tcp or udp); with the subscription, Caddy's two tcp ports
# count too. The ports that are given or remembered are claimed first, so that --auto-ports goes
# around them.
choose_ports() {
	local k v from kind
	local -a auto=()
	local -A user=()
	for k in "${PORT_KEYS[@]}"; do
		port_option "$k" from
		if [[ -n ${PORT_FLAG[$k]:-} ]]; then
			v=${PORT_FLAG[$k]}
		elif [[ -n ${ST[$k]:-} ]]; then
			v=${ST[$k]} from="$ROOT$P_STATE/state: $k"
		elif ((AUTO_PORTS)) && port_used "$k"; then
			auto+=("$k")
			continue
		else
			v=${PORT_DEFAULT[$k]}
		fi
		if ! [[ $v =~ ^[0-9]{1,5}$ ]] || ((10#$v < 1 || 10#$v > 65535)); then
			die "$from must be a port number from 1 to 65535, not '$v'"
		fi
		printf -v "$k" '%d' "$((10#$v))"
		if [[ -n ${PORT_FLAG[$k]:-} ]]; then
			ST[$k]=${!k}
		fi
		if port_used "$k"; then
			for kind in ${PORT_KINDS[$k]}; do
				claim_port "${!k}/$kind" "$from"
			done
		fi
	done
	if [[ $SUBSCRIPTION == 1 ]]; then
		claim_port "$ACME_PORT/tcp" "the certificate of the subscription"
	fi
	for k in "${auto[@]}"; do
		auto_port "$k"
	done
	# The ports as the firewall and the checks see them.
	SERVER_PORTS=()
	for v in "${ALL_PROTOCOLS[@]}"; do
		if uses_protocol "$v"; then
			k=${v^^}_PORT
			for kind in ${PORT_KINDS[$k]}; do
				SERVER_PORTS+=("${!k}/$kind")
			done
		fi
	done
	SUB_PORTS=("$ACME_PORT/tcp" "$SUB_PORT/tcp")
}

# Marks port/kind $1 as taken by $2 in this run (user, a local of choose_ports).
claim_port() {
	if [[ -n ${user[$1]:-} ]]; then
		die "${user[$1]} and $2 both need $1; give one of them another port"
	fi
	user[$1]=$2
}

# --auto-ports for key $1: the first port of its list that is free on each kind it needs (no other
# program listens there, see port_busy) and not taken in this run.
auto_port() {
	local k=$1 p kind free opt also=""
	port_option "$k" opt
	if [[ $k == SUB_PORT ]]; then
		also=caddy
	fi
	for p in ${AUTO_PORT_LIST[$k]}; do
		free=1
		for kind in ${PORT_KINDS[$k]}; do
			if [[ -n ${user[$p/$kind]:-} ]] || port_busy "$p" "$kind" "$also"; then
				free=0
				break
			fi
		done
		if ((free)); then
			printf -v "$k" '%d' "$p"
			ST[$k]=$p
			for kind in ${PORT_KINDS[$k]}; do
				claim_port "$p/$kind" "$opt"
			done
			return 0
		fi
	done
	die_code ports_busy "--auto-ports: every port it tries for $opt is taken (${AUTO_PORT_LIST[$k]// /, }/${PORT_KINDS[$k]// / and }; see: ss -lntup); free one or give a port with $opt"
}

# ---- sing-box --------------------------------------------------------------------------------

install_singbox() {
	local bin=$ROOT$P_BIN name archive version sum
	if [[ -n ${ST[BINARY]:-} ]] && [[ -x $bin ]] && [[ ${ST[BINARY]} == "$SINGBOX_VERSION:$(sha256 "$bin")" ]]; then
		say "sing-box $SINGBOX_VERSION is already installed"
		step singbox skip "sing-box $SINGBOX_VERSION is already installed"
		return
	fi
	if [[ -e $bin && -z ${ST[BINARY]:-} ]]; then
		die "$bin exists and was not installed by this kit; remove it (and whatever uses it) first"
	fi
	name=sing-box-$SINGBOX_VERSION-linux-$ARCH
	archive=$TMP/$name.tar.gz
	say "Downloading sing-box $SINGBOX_VERSION ($ARCH)"
	step singbox start "downloading sing-box $SINGBOX_VERSION ($ARCH)"
	curl -fsSL --proto '=https' --retry 3 --connect-timeout 20 --max-time 300 --speed-limit 1024 --speed-time 60 -o "$archive" \
		"https://github.com/SagerNet/sing-box/releases/download/v$SINGBOX_VERSION/$name.tar.gz" ||
		die_code download_failed "could not download sing-box; nothing was installed"
	local got
	got=$(sha256 "$archive")
	[[ $got == "$SHA256" ]] || die "the SHA-256 of $name.tar.gz is $got, not the expected $SHA256; nothing was installed"
	# The archive also holds libcronet.so, which only the naive outbound loads; a server needs the
	# binary alone.
	tar -xzf "$archive" --no-same-owner -C "$TMP" "$name/sing-box" || die "could not unpack $name.tar.gz"
	# The service user runs the binary, so its folder must be open to all, whatever the umask (077
	# here) made of it.
	(umask 022 && mkdir -p -- "$(dirname -- "$bin")")
	chmod 755 -- "$(dirname -- "$bin")"
	install -m 0755 "$TMP/$name/sing-box" "$bin.new"
	sum=$(sha256 "$bin.new")
	mv -f -- "$bin.new" "$bin"
	# Remembered at once, before any output: a run cut from here on (its client gone, say) leaves a
	# binary that the next run knows as the kit's own.
	ST[BINARY]=$SINGBOX_VERSION:$sum
	save_state
	version=$("$bin" version) || die "the installed sing-box does not run"
	say "Installed ${version%%$'\n'*}"
	step singbox "done" "installed sing-box $SINGBOX_VERSION"
}

make_secrets() {
	local sb=$ROOT$P_BIN out dir=$ROOT$P_STATE
	if [[ -z ${ST[UUID]:-} ]]; then
		ST[UUID]=$("$sb" generate uuid)
	fi
	if [[ -z ${ST[REALITY_PRIVATE_KEY]:-} || -z ${ST[REALITY_PUBLIC_KEY]:-} ]]; then
		out=$("$sb" generate reality-keypair)
		ST[REALITY_PRIVATE_KEY]=$(field PrivateKey "$out")
		ST[REALITY_PUBLIC_KEY]=$(field PublicKey "$out")
	fi
	if [[ -z ${ST[SHORT_ID]:-} ]]; then
		ST[SHORT_ID]=$("$sb" generate rand 8 --hex)
	fi
	if [[ -z ${ST[HY2_PASSWORD]:-} ]]; then
		ST[HY2_PASSWORD]=$("$sb" generate rand 16 --hex)
	fi
	if [[ -z ${ST[SS_PASSWORD]:-} ]]; then
		ST[SS_PASSWORD]=$("$sb" generate rand 16 --base64)
	fi
	if [[ -z ${ST[WG_SERVER_PRIVATE_KEY]:-} || -z ${ST[WG_SERVER_PUBLIC_KEY]:-} ]]; then
		out=$("$sb" generate wg-keypair)
		ST[WG_SERVER_PRIVATE_KEY]=$(field PrivateKey "$out")
		ST[WG_SERVER_PUBLIC_KEY]=$(field PublicKey "$out")
	fi
	if [[ -z ${ST[WG_CLIENT_PRIVATE_KEY]:-} || -z ${ST[WG_CLIENT_PUBLIC_KEY]:-} ]]; then
		out=$("$sb" generate wg-keypair)
		ST[WG_CLIENT_PRIVATE_KEY]=$(field PrivateKey "$out")
		ST[WG_CLIENT_PUBLIC_KEY]=$(field PublicKey "$out")
	fi
	if [[ -z ${ST[WG_PRESHARED_KEY]:-} ]]; then
		ST[WG_PRESHARED_KEY]=$("$sb" generate rand 32 --base64)
	fi
	if [[ -z ${ST[SUB_TOKEN]:-} ]]; then
		ST[SUB_TOKEN]=$("$sb" generate rand 12 --hex)
	fi
	# The Hysteria2 certificate: self-signed for the name of the subscription address (the links
	# skip its check), made again when that name changes.
	if [[ ${ST[HY2_CERT_NAME]:-} != "$DOMAIN" || ! -s $dir/hysteria2.crt || ! -s $dir/hysteria2.key ]]; then
		mkdir -p -- "$dir"
		chmod 700 "$dir"
		"$sb" generate tls-keypair "$DOMAIN" -m 120 >"$TMP/tls.pem"
		sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' "$TMP/tls.pem" >"$dir/hysteria2.crt"
		sed -n '/-----BEGIN PRIVATE KEY-----/,/-----END PRIVATE KEY-----/p' "$TMP/tls.pem" >"$dir/hysteria2.key"
		chmod 600 "$dir/hysteria2.crt" "$dir/hysteria2.key"
		[[ -s $dir/hysteria2.crt && -s $dir/hysteria2.key ]] || die "sing-box did not make the Hysteria2 certificate"
		ST[HY2_CERT_NAME]=$DOMAIN
	fi
	check_state all
}

# The config of the core with the ways in of this run only: a way in taken out of --protocols
# leaves the config on the next run.
write_config() {
	local dir=$ROOT$P_STATE sb=$ROOT$P_BIN listen="::" out inbounds="" endpoints=""
	# Without IPv6 in the kernel the server listens on IPv4 only.
	if [[ ! -e /proc/net/if_inet6 ]]; then
		listen=0.0.0.0
	fi
	if uses_protocol vless; then
		add_block inbounds <<EOF
    {
      "type": "vless", "tag": "vless-reality", "listen": "$listen", "listen_port": $VLESS_PORT,
      "users": [{"name": "splitlane", "uuid": "${ST[UUID]}", "flow": "xtls-rprx-vision"}],
      "tls": {
        "enabled": true, "server_name": "$REALITY_SNI",
        "reality": {
          "enabled": true,
          "handshake": {"server": "$REALITY_SNI", "server_port": 443},
          "private_key": "${ST[REALITY_PRIVATE_KEY]}",
          "short_id": ["${ST[SHORT_ID]}"]
        }
      }
    }
EOF
	fi
	if uses_protocol hy2; then
		add_block inbounds <<EOF
    {
      "type": "hysteria2", "tag": "hysteria2", "listen": "$listen", "listen_port": $HY2_PORT,
      "users": [{"name": "splitlane", "password": "${ST[HY2_PASSWORD]}"}],
      "tls": {
        "enabled": true, "server_name": "${ST[HY2_CERT_NAME]}", "alpn": ["h3"],
        "certificate": [$(pem_json "$dir/hysteria2.crt")],
        "key": [$(pem_json "$dir/hysteria2.key")]
      }
    }
EOF
	fi
	if uses_protocol ss; then
		add_block inbounds <<EOF
    {
      "type": "shadowsocks", "tag": "shadowsocks-2022", "listen": "$listen", "listen_port": $SS_PORT,
      "method": "$SS_METHOD", "password": "${ST[SS_PASSWORD]}"
    }
EOF
	fi
	if uses_protocol wg; then
		add_block endpoints <<EOF
    {
      "type": "wireguard", "tag": "wireguard", "mtu": $WG_MTU, "listen_port": $WG_PORT,
      "address": [$(json_list "${WG_SERVER_ADDRESSES[@]}")],
      "private_key": "${ST[WG_SERVER_PRIVATE_KEY]}",
      "peers": [
        {
          "public_key": "${ST[WG_CLIENT_PUBLIC_KEY]}",
          "pre_shared_key": "${ST[WG_PRESHARED_KEY]}",
          "allowed_ips": [$(json_list "${WG_CLIENT_ADDRESSES[@]}")]
        }
      ]
    }
EOF
	fi
	cat >"$dir/config.json.new" <<EOF
{
  "log": {"level": "$LOG_LEVEL"},
  "inbounds": [$inbounds
  ],
  "endpoints": [$endpoints
  ],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {
    "rules": [
      {"action": "resolve", "strategy": "prefer_ipv4"},
      {"ip_is_private": true, "action": "reject"}
    ],
    "final": "direct"
  }
}
EOF
	chmod 600 "$dir/config.json.new"
	if ! out=$("$sb" check -c "$dir/config.json.new" 2>&1); then
		printf '%s\n' "$out" >&2
		die "sing-box check refused the new config (above); the old one, if any, is unchanged"
	fi
	mv -f -- "$dir/config.json.new" "$dir/config.json"
	say "sing-box check: the config is valid"
}

# ---- What the user gets ----------------------------------------------------------------------

write_client_files() {
	local out=$ROOT$P_OUT
	VLESS_LINK="vless://${ST[UUID]}@$HOST_URI:$VLESS_PORT?encryption=none&flow=xtls-rprx-vision&security=reality"
	VLESS_LINK+="&sni=$REALITY_SNI&fp=chrome&pbk=${ST[REALITY_PUBLIC_KEY]}&sid=${ST[SHORT_ID]}&type=tcp&headerType=none"
	VLESS_LINK+="#SplitLane-VLESS-Reality"
	HY2_LINK="hysteria2://${ST[HY2_PASSWORD]}@$HOST_URI:$HY2_PORT?sni=${ST[HY2_CERT_NAME]}&insecure=1#SplitLane-Hysteria2"
	SS_LINK="ss://$SS_METHOD:$(pct "${ST[SS_PASSWORD]}")@$HOST_URI:$SS_PORT#SplitLane-SS2022"
	WG_CONF="[Interface]
PrivateKey = ${ST[WG_CLIENT_PRIVATE_KEY]}
Address = $(join_list "${WG_CLIENT_ADDRESSES[@]}")
DNS = $WG_CLIENT_DNS

[Peer]
PublicKey = ${ST[WG_SERVER_PUBLIC_KEY]}
PresharedKey = ${ST[WG_PRESHARED_KEY]}
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = $HOST_URI:$WG_PORT
PersistentKeepalive = $WG_KEEPALIVE"

	# The links of this run; the files of a way in taken out of --protocols go.
	LINKS=()
	if uses_protocol vless; then
		LINKS+=("$VLESS_LINK")
	fi
	if uses_protocol hy2; then
		LINKS+=("$HY2_LINK")
	fi
	if uses_protocol ss; then
		LINKS+=("$SS_LINK")
	fi
	mkdir -p -- "$out"
	chmod 700 "$out"
	if ((${#LINKS[@]})); then
		printf '%s\n' "${LINKS[@]}" >"$out/links.txt"
		local sub
		sub=$(base64 -w0 "$out/links.txt")
		printf '%s\n' "$sub" >"$out/subscription.txt"
		chmod 600 "$out/links.txt" "$out/subscription.txt"
	else
		rm -f -- "$out/links.txt" "$out/subscription.txt"
	fi
	if uses_protocol wg; then
		printf '%s\n' "$WG_CONF" >"$out/splitlane-wg.conf"
		chmod 600 "$out/splitlane-wg.conf"
	else
		rm -f -- "$out/splitlane-wg.conf"
	fi
}

# ---- systemd, firewall -----------------------------------------------------------------------

write_unit() {
	mkdir -p -- "$(dirname -- "$ROOT$P_UNIT")"
	cat >"$ROOT$P_UNIT" <<EOF
# The SplitLane server kit (install-singbox-server.sh); --uninstall removes it.
[Unit]
Description=sing-box server of the SplitLane server kit
Documentation=https://github.com/splitlane-app/splitlane/tree/main/scripts/server
After=network-online.target
Wants=network-online.target

[Service]
# A dedicated system user, not root: WireGuard here is the core's userspace endpoint (no kernel
# interface), so the server needs only CAP_NET_BIND_SERVICE (ports below 1024) and CAP_NET_ADMIN
# (socket options of the core).
User=$SERVICE_USER
Group=$SERVICE_USER
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
# The config holds the keys and stays readable by root only. Before each start, a step with full
# privileges ("+") copies it into the service's own folder in RAM, removed when the service stops.
RuntimeDirectory=$SERVICE
RuntimeDirectoryMode=0700
ExecStartPre=+/usr/bin/install -m 0600 -o $SERVICE_USER -g $SERVICE_USER $P_STATE/config.json /run/$SERVICE/config.json
ExecStart=$P_BIN run --disable-color -c /run/$SERVICE/config.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
	chmod 644 "$ROOT$P_UNIT"
}

start_service() {
	if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
		useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "$SERVICE_USER"
	fi
	write_unit
	systemctl daemon-reload
	systemctl enable --quiet "$SERVICE"
	systemctl restart "$SERVICE"
	sleep 2
	if ! systemctl is-active --quiet "$SERVICE"; then
		journalctl -u "$SERVICE" -n 20 --no-pager >&2 || true
		die "the $SERVICE service did not start (its log is above; systemctl status $SERVICE)"
	fi
	say "The $SERVICE service is running"
}

# The ports this run needs (PORTS), for the messages.
needed_ports() {
	PORTS=("${SERVER_PORTS[@]}")
	if [[ $SUBSCRIPTION == 1 ]]; then
		PORTS+=("${SUB_PORTS[@]}")
	fi
}

# Allows the given ports in ufw when it is active, before the server or Caddy that needs them
# starts (Let's Encrypt checks port 80 right away). The firewall of the provider is not the
# script's to change: its ports are listed at the end.
UFW_ACTIVE=0
open_ports() {
	local status="" p
	if command -v ufw >/dev/null 2>&1; then
		status=$(ufw status 2>/dev/null || true)
	fi
	if [[ $status != "Status: active"* ]]; then
		return 0
	fi
	UFW_ACTIVE=1
	for p in "$@"; do
		ufw allow "$p" comment "$SERVICE" >/dev/null
	done
	say "ufw: allowed $*"
}

# ---- The subscription ------------------------------------------------------------------------

# The served copy of subscription.txt and Caddy's site for it. $1 is the group of Caddy, empty in
# a dry run.
write_subscription_site() {
	local group=$1 www=$ROOT$P_WWW token=${ST[SUB_TOKEN]}
	rm -rf -- "$www"
	install -d -m 0750 ${group:+-g "$group"} "$ROOT$P_WWW_ROOT" "$www" "$www/$token"
	install -m 0640 ${group:+-g "$group"} "$ROOT$P_OUT/subscription.txt" "$www/$token/sub"
	mkdir -p -- "$(dirname -- "$ROOT$P_CADDY_SITE")"
	# The site names the token too, so it is as private as the served file.
	cat >"$TMP/site.caddy" <<EOF
# The SplitLane subscription, added by install-singbox-server.sh (the SplitLane server kit);
# --uninstall removes it. Only /<token>/sub is served: the address is the secret.
$DOMAIN:$SUB_PORT {
	tls {
		issuer acme {
			# Port 443 is the VLESS + Reality server by default (or another program), so the
			# certificate comes over HTTP (port 80).
			disable_tlsalpn_challenge
		}
	}
	handle /$token/sub {
		root * $P_WWW
		header Content-Type "text/plain; charset=utf-8"
		header Cache-Control "no-store"
		file_server
	}
	handle {
		respond 404
	}
}
EOF
	install -m 0640 ${group:+-g "$group"} "$TMP/site.caddy" "$ROOT$P_CADDY_SITE"
	SUB_URL=https://$DOMAIN:$SUB_PORT/$token/sub
}

# Adds the kit's two lines at the end of the Caddyfile (after a line break if the file lacks its
# last one), so that remove_caddy_import leaves the file as it was.
add_caddy_import() {
	local file=$P_CADDYFILE
	if grep -qxF "$CADDY_IMPORT" "$file"; then
		return 0
	fi
	if [[ -s $file && -n $(tail -c 1 -- "$file") ]]; then
		printf '\n' >>"$file"
	fi
	printf '%s\n%s\n' "$CADDY_MARK" "$CADDY_IMPORT" >>"$file"
}

# Takes the kit's two lines out of the Caddyfile. When grep cannot read the file (status 2), it
# fails and leaves the file alone; status 1 only means that no other line is left.
remove_caddy_import() {
	local file=$ROOT$P_CADDYFILE rc=0
	[[ -f $file ]] || return 0
	grep -vxF -e "$CADDY_MARK" -e "$CADDY_IMPORT" "$file" >"$TMP/Caddyfile" || rc=$?
	if ((rc > 1)); then
		warn "could not read $P_CADDYFILE; take the line '$CADDY_IMPORT' out of it by hand"
		return 1
	fi
	cat "$TMP/Caddyfile" >"$file"
}

# The subscription step: Caddy with the kit's site (in a dry run, only the files). A skipped
# subscription is a warning (sub_port80_busy or sub_skipped), not an error: the links work
# without it.
subscription_step() {
	SUB_URL=""
	if [[ $SUBSCRIPTION != 1 ]]; then
		step subscription skip "no --subscription"
		return 0
	fi
	step subscription start "the subscription over HTTPS (Caddy)"
	if ((${#LINKS[@]} == 0)); then
		warn_code sub_skipped "subscription skipped: it serves share links, and WireGuard has none"
	elif port_busy "$ACME_PORT" tcp caddy; then
		# Caddy gets the certificate on port 80 only; it does not move like the other ports.
		warn_code sub_port80_busy "subscription skipped: another program listens on port $ACME_PORT, which Caddy needs for the certificate (see: ss -lntp)"
	elif ((DRY_RUN)); then
		write_subscription_site ""
		ST[SUBSCRIPTION]=1
	else
		setup_subscription
	fi
	if [[ -n $SUB_URL ]]; then
		step subscription "done" "Caddy serves it on port $SUB_PORT"
	else
		SUBSCRIPTION=0
		step subscription warn "$LAST_WARNING"
	fi
}

setup_subscription() {
	if ! command -v caddy >/dev/null 2>&1; then
		say "Installing Caddy for the subscription"
		if ! apt_update; then
			warn_code sub_skipped "subscription skipped: apt-get update failed"
			return 0
		fi
		if ! apt-cache show caddy >/dev/null 2>&1; then
			warn_code sub_skipped "subscription skipped: this distribution has no caddy package (Debian 12 and Ubuntu 24.04 have one)"
			return 0
		fi
		if ! apt_install caddy; then
			warn_code sub_skipped "subscription skipped: apt-get install caddy failed"
			return 0
		fi
	fi
	if ! getent group caddy >/dev/null || [[ ! -f $P_CADDYFILE ]]; then
		warn_code sub_skipped "subscription skipped: Caddy is not set up as the caddy package does it (group caddy, $P_CADDYFILE)"
		return 0
	fi
	write_subscription_site caddy
	add_caddy_import
	if ! caddy validate --config "$P_CADDYFILE" --adapter caddyfile >"$TMP/caddy.log" 2>&1; then
		tail -n 5 "$TMP/caddy.log" >&2 || true
		drop_subscription_site
		warn_code sub_skipped "subscription skipped: Caddy refused the site (above)"
		return 0
	fi
	open_ports "${SUB_PORTS[@]}"
	if ! systemctl enable --quiet caddy || ! systemctl reload-or-restart caddy; then
		drop_subscription_site
		systemctl reload-or-restart caddy >/dev/null 2>&1 || true
		warn_code sub_skipped "subscription skipped: Caddy did not start with the site; is port $ACME_PORT or $SUB_PORT taken? (journalctl -u caddy)"
		return 0
	fi
	ST[SUBSCRIPTION]=1
}

# Undoes write_subscription_site and the import line after a failure.
drop_subscription_site() {
	if remove_caddy_import; then
		rm -f -- "$P_CADDY_SITE"
	fi
	rm -rf -- "$P_WWW_ROOT"
	SUB_URL=""
	unset 'ST[SUBSCRIPTION]'
}

# ---- Light hardening (--harden) --------------------------------------------------------------

# fail2ban for SSH and the distribution's automatic updates; nothing in the settings of SSH
# changes. A failure is the warning harden_failed: the VPN server works without them. A re-run
# writes the jail again (with the SSH sessions of its time) and leaves an existing 20auto-upgrades
# as it is. A dry run writes the files under its directory, without apt and systemctl.
harden() {
	local why=""
	step harden start "fail2ban for SSH, automatic security updates"
	if ((!DRY_RUN)) && ! harden_packages; then
		why="apt-get could not install ${HARDEN_PACKAGES[*]}"
	elif ! write_jail; then
		why="could not write $P_JAIL"
	elif ! write_auto_upgrades; then
		why="could not write $P_AUTO_UPGRADES"
	elif ((!DRY_RUN)) && ! start_fail2ban; then
		why="fail2ban did not start with the sshd jail (see: systemctl status fail2ban)"
	fi
	if [[ -n $why ]]; then
		warn_code harden_failed "hardening failed: $why; the VPN server works without it"
		step harden warn "$why"
		return 0
	fi
	say "fail2ban guards SSH (the jail sshd, $P_JAIL); $UPDATES_TEXT"
	step harden "done" "fail2ban guards SSH; $UPDATES_TEXT"
}

# Installs the packages unless they are there already (a re-run needs no apt).
harden_packages() {
	local installed
	# shellcheck disable=SC2016 # ${Status} is dpkg-query's field, not a shell variable
	installed=$(dpkg-query -W -f '${Status}\n' "${HARDEN_PACKAGES[@]}" 2>/dev/null | grep -cx 'install ok installed' || true)
	if ((installed == ${#HARDEN_PACKAGES[@]})); then
		return 0
	fi
	say "Installing ${HARDEN_PACKAGES[*]}"
	apt_update && apt_install "${HARDEN_PACKAGES[@]}"
}

# The jail of the kit: sshd on the journal of systemd, since Debian 12 has no auth.log. A ban closes
# the ports sshd listens on (port 22, ssh, when ss finds none): SplitLane's form takes another SSH
# port too. The bans are fail2ban's defaults: 5 failed logins within 10 minutes ban the address for
# 10 minutes. On its start fail2ban reads the journal back those 10 minutes, so the mistyped
# passwords just before this run would ban the session that runs it: the addresses connected over
# SSH now are exempt. A re-run writes the ports and the addresses of its own time.
write_jail() {
	local ports peers
	ports=$(sshd_ports)
	peers=$(ssh_peers "$ports")
	cat >"$TMP/jail" <<EOF || return 1
# Added by the SplitLane server kit (install-singbox-server.sh --harden); --uninstall leaves it.
[sshd]
enabled = true
backend = systemd
# The ports sshd listened on during the kit's last run; a ban closes them to the address.
port = ${ports:-ssh}
# This machine and the SSH sessions open during the kit's last run are never banned.
ignoreip = 127.0.0.1/8 ::1${peers:+ $peers}
EOF
	(umask 022 && mkdir -p -- "$(dirname -- "$ROOT$P_JAIL")") || return 1
	install -m 0644 "$TMP/jail" "$ROOT$P_JAIL"
}

# The tcp ports sshd listens on now, each once, in ascending order, joined by commas (22,2222):
# the lines of ss whose process list has sshd, which also covers Ubuntu's ssh.socket, where sshd
# inherits the listener of systemd.
sshd_ports() {
	command -v ss >/dev/null 2>&1 || return 0
	local state local_addr users p
	local -a sorted=()
	local -A seen=()
	while read -r state _ _ local_addr _ users; do
		p=${local_addr##*:}
		if [[ $state == LISTEN && $users == *'"sshd",'* && $p =~ ^[0-9]{1,5}$ ]]; then
			seen[$p]=1
		fi
	done < <(ss -H -ltnp 2>/dev/null || true)
	if ((${#seen[@]})); then
		mapfile -t sorted < <(printf '%s\n' "${!seen[@]}" | sort -n)
	fi
	local IFS=,
	printf '%s' "${sorted[*]}"
}

# The addresses of the SSH connections established now to one of the ports $1 (sshd_ports), each
# once, separated by spaces: IPv4, or IPv6 without brackets and scope; an IPv4-mapped IPv6 address
# becomes IPv4. ss tells, since sudo drops $SSH_CLIENT.
ssh_peers() {
	command -v ss >/dev/null 2>&1 || return 0
	local state local_addr peer p out=""
	local -a list=()
	local -A ports=() seen=()
	IFS=, read -r -a list <<<"$1"
	for p in "${list[@]}"; do
		ports[$p]=1
	done
	while read -r state _ _ local_addr peer _; do
		if [[ $state != ESTAB || -z ${ports[${local_addr##*:}]:-} ]]; then
			continue
		fi
		peer=${peer%:*}
		peer=${peer#[}
		peer=${peer%]}
		peer=${peer%%%*}
		if [[ ${peer,,} == ::ffff:* ]] && is_ipv4 "${peer:7}"; then
			peer=${peer:7}
		fi
		if { is_ipv4 "$peer" || is_ipv6 "$peer"; } && [[ -z ${seen[$peer]:-} ]]; then
			seen[$peer]=1
			out+="${out:+ }$peer"
		fi
	done < <(ss -H -tn 2>/dev/null || true)
	printf '%s' "$out"
}

# Turns on the daily package lists and unattended-upgrades (which installs the security updates of
# the distribution by default), unless the system already has this file (the package may write
# it): then it stays as it is, and UPDATES_TEXT says what apt makes of it.
write_auto_upgrades() {
	local on
	UPDATES_TEXT="automatic security updates are on"
	if [[ -e $ROOT$P_AUTO_UPGRADES ]]; then
		if ((DRY_RUN)); then
			UPDATES_TEXT="automatic updates as the existing $P_AUTO_UPGRADES sets them"
		else
			on=$(apt-config shell on APT::Periodic::Unattended-Upgrade 2>/dev/null || true)
			if [[ -z $on || $on == "on='0'" ]]; then
				UPDATES_TEXT="automatic updates stay off, as $P_AUTO_UPGRADES sets them"
			fi
		fi
		return 0
	fi
	printf '%s\n' 'APT::Periodic::Update-Package-Lists "1";' 'APT::Periodic::Unattended-Upgrade "1";' >"$TMP/auto-upgrades" || return 1
	(umask 022 && mkdir -p -- "$(dirname -- "$ROOT$P_AUTO_UPGRADES")") || return 1
	install -m 0644 "$TMP/auto-upgrades" "$ROOT$P_AUTO_UPGRADES"
}

# fail2ban with the kit's jail: a running fail2ban that answers reloads its configuration; one that
# does not (just started by the package, or failed without a log for sshd) is restarted. Then
# waits for the sshd jail.
start_fail2ban() {
	local i
	systemctl enable --quiet fail2ban || return 1
	if fail2ban-client ping >/dev/null 2>&1; then
		systemctl reload fail2ban || return 1
	else
		systemctl restart fail2ban || return 1
	fi
	for ((i = 0; i < 20; i++)); do
		if fail2ban-client status sshd >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	return 1
}

# ---- Removal ---------------------------------------------------------------------------------

uninstall() {
	if ((!DRY_RUN)); then
		((EUID == 0)) || die "run it as root: sudo bash $0 --uninstall"
	fi
	take_lock
	load_state
	local bin=$ROOT$P_BIN
	if [[ -f $ROOT$P_UNIT ]]; then
		if ((!DRY_RUN)); then
			systemctl disable --now --quiet "$SERVICE" || true
		fi
		rm -f -- "$ROOT$P_UNIT"
		if ((!DRY_RUN)); then
			systemctl daemon-reload
		fi
		say "Removed the $SERVICE service"
	fi
	if [[ -f $bin ]]; then
		if [[ ${ST[BINARY]:-} == "$SINGBOX_VERSION:$(sha256 "$bin")" ]]; then
			rm -f -- "$bin"
			rmdir -- "$(dirname -- "$bin")" 2>/dev/null || true
			say "Removed $P_BIN"
		else
			say "Left $P_BIN: this kit did not install it"
		fi
	fi
	if [[ -f $ROOT$P_CADDY_SITE ]]; then
		if remove_caddy_import; then
			rm -f -- "$ROOT$P_CADDY_SITE"
			if ((!DRY_RUN)) && systemctl is-active --quiet caddy; then
				systemctl reload caddy || warn "Caddy did not reload; check $P_CADDYFILE"
			fi
			say "Removed the subscription site from Caddy (Caddy itself stays)"
		else
			warn "left $P_CADDY_SITE: the Caddyfile still imports it"
		fi
	fi
	# The protection of --harden stays: without the kit's jail, the sshd jail of Debian 12's own
	# fail2ban looks for an auth.log the system lacks, and fail2ban would not start again.
	if [[ -f $ROOT$P_JAIL ]]; then
		say "fail2ban with the kit's jail for SSH ($P_JAIL) and the automatic updates stay"
	fi
	rm -rf -- "$ROOT$P_WWW_ROOT"
	if [[ -d $ROOT$P_STATE ]]; then
		rm -rf -- "$ROOT$P_STATE"
		say "Removed $P_STATE"
	fi
	if ((!DRY_RUN)) && id -u "$SERVICE_USER" >/dev/null 2>&1; then
		userdel "$SERVICE_USER" || true
	fi
	if command -v ufw >/dev/null 2>&1 && [[ $(ufw status 2>/dev/null || true) == "Status: active"* ]]; then
		say "ufw rules marked '$SERVICE' stay; see them with: ufw status numbered"
	fi
	if [[ -d $ROOT$P_OUT ]]; then
		say "$P_OUT was left for you to delete (it holds the keys): rm -rf $P_OUT"
	fi
}

# ---- Main ------------------------------------------------------------------------------------

# The links, the .conf and the subscription address for people (not in --json: there they are
# only in the result line).
print_result() {
	local out=$ROOT$P_OUT n=${#LINKS[@]} the_links
	local -a files=()
	case $n in
	1) the_links="the link" ;;
	2) the_links="both links" ;;
	*) the_links="the three links" ;;
	esac
	if ((n)); then
		say ""
		case $n in
		1) say "Share link (in SplitLane: Key or subscription):" ;;
		2) say "Share links (in SplitLane: Key or subscription; paste one or both):" ;;
		*) say "Share links (in SplitLane: Key or subscription; paste one or all three):" ;;
		esac
		printf '%s\n' "${LINKS[@]}"
		files+=(links.txt)
	fi
	if uses_protocol wg; then
		say ""
		say "WireGuard (in SplitLane: Configuration file, or paste the text): $out/splitlane-wg.conf"
		say "$WG_CONF"
		files+=(splitlane-wg.conf)
	fi
	if ((n)); then
		files+=("subscription.txt (base64 of links.txt)")
	fi
	say ""
	if [[ -n ${SUB_URL:-} ]]; then
		say "Subscription address (in SplitLane: Key or subscription):"
		say "$SUB_URL"
		say "The address is the secret: whoever has it gets $the_links. --reset changes it and every key."
		if ((!DRY_RUN)); then
			say "Caddy is getting a certificate for $DOMAIN; the address works once it has one (usually within a minute)."
		fi
		say ""
	fi
	say "Saved in $out: $(join_list "${files[@]}")"
	say "Keep them private: anyone with a link or the .conf can use this server."
	if ((RESET)); then
		say "After --reset the old links, .conf and subscription address no longer work: add the new ones"
		say "in SplitLane and remove the old servers and subscription there."
	fi
}

# The result line of --json: the links, the .conf, the subscription address, the ports of the ways
# in of this run and of the subscription, and the codes of the warnings.
print_json_result() {
	local links="" ports="" warnings="" wg=null sub=null p k
	for p in "${LINKS[@]}"; do
		links+="${links:+,}$(json_str "$p")"
	done
	for p in "${ALL_PROTOCOLS[@]}"; do
		if uses_protocol "$p"; then
			k=${p^^}_PORT
			ports+="${ports:+,}\"$p\":${!k}"
		fi
	done
	if uses_protocol wg; then
		wg=$(json_str "$WG_CONF")
	fi
	if [[ -n ${SUB_URL:-} ]]; then
		sub=$(json_str "$SUB_URL")
		ports+=",\"sub\":$SUB_PORT"
	fi
	for p in "${WARNINGS[@]}"; do
		warnings+="${warnings:+,}\"$p\""
	done
	FINAL=1
	printf '{"result":{"links":[%s],"wireguard":%s,"subscription":%s,"ports":{%s},"warnings":[%s]}}\n' \
		"$links" "$wg" "$sub" "$ports" "$warnings" >&3 || true
}

# Removes the temporary folder. A --json run that stops without its result or error line (a
# command failed under set -e) still ends with an error line.
on_exit() {
	local rc=$?
	if [[ -n $TMP ]]; then
		rm -rf -- "$TMP"
	fi
	if ((JSON && rc != 0 && !FINAL)); then
		FINAL=1
		printf '{"error":{"code":"kit_failed","message":"the kit stopped unexpectedly (exit code %d); its stderr says where"}}\n' "$rc" >&3 || true
		exit 1
	fi
}

main() {
	local a
	# --json is known before anything can fail, even in the arguments.
	for a in "$@"; do
		if [[ $a == --json ]]; then
			JSON=1
		fi
	done
	if ((JSON)); then
		exec 3>&1 1>&2
		trap '' PIPE
	fi
	trap on_exit EXIT
	parse_args "$@"
	TMP=$(mktemp -d)
	if ((UNINSTALL)); then
		uninstall
		return
	fi
	step system start "checking the system"
	preflight
	take_lock
	load_state
	check_state loaded
	find_kit_pid
	choose_settings
	if ((!DRY_RUN)); then
		check_ports
	fi
	needed_ports
	step system "done" "$SYSTEM_TEXT; ports ${PORTS[*]}"
	install_singbox
	save_state
	step config start "keys, the server config, the links"
	make_secrets
	save_state
	write_config
	write_client_files
	step config "done" "ways in: ${PROTOCOLS//,/, }"
	if ((DRY_RUN)); then
		write_unit
		step service skip "dry run: the unit file is written, nothing is started"
	else
		step service start "the $SERVICE service"
		open_ports "${SERVER_PORTS[@]}"
		start_service
		step service "done" "the $SERVICE service is running"
	fi
	if ((HARDEN)); then
		harden
	else
		step harden skip "no --harden"
	fi
	subscription_step
	save_state
	if ((!DRY_RUN && !UFW_ACTIVE)); then
		needed_ports
		say "Open these ports in the firewall of your VPS provider, if it has one: ${PORTS[*]}"
	fi
	if ((JSON)); then
		print_json_result
	else
		print_result
	fi
	if ((DRY_RUN)); then
		needed_ports
		say ""
		say "Dry run: nothing was installed or started; the files are under $ROOT (ports: ${PORTS[*]})"
	fi
}

main "$@"
