#!/usr/bin/env bash
# The SplitLane server kit: a sing-box server on a Linux VPS for SplitLane's "Key or subscription"
# and "Configuration file" connections.
#
# Installs sing-box 1.14.4 (the version of SplitLane's own core) from the official release after
# checking the archive against the SHA-256 pinned below, generates the keys and sets up a server
# with four ways in:
#   VLESS + Reality + xtls-rprx-vision   443/tcp
#   Hysteria2 (self-signed certificate)  443/udp
#   Shadowsocks 2022                     8388/tcp+udp
#   WireGuard                            51820/udp
# (the default ports; --vless-port, --hy2-port, --ss-port and --wg-port change them). It prints
# the share links, a WireGuard .conf and a subscription file and keeps them in /root/splitlane-kit/.
# With --subscription, Caddy also serves the links as a subscription over HTTPS. For Debian 12+
# and Ubuntu 22.04+ on x86_64 or aarch64, run as root. A re-run keeps the keys and the links.
# --help lists the options.
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
CADDY_MARK="# Added by the SplitLane server kit; install-singbox-server.sh --uninstall removes it."
CADDY_IMPORT="import $P_CADDY_SITE"

# The state file keeps these values between runs. --reset forgets the generated ones; the
# settings (from --host, --domain, --reality-sni, the port options, --subscription) and the record
# of the installed binary stay.
GENERATED_KEYS=(UUID REALITY_PRIVATE_KEY REALITY_PUBLIC_KEY SHORT_ID HY2_PASSWORD HY2_CERT_NAME SS_PASSWORD
	WG_SERVER_PRIVATE_KEY WG_SERVER_PUBLIC_KEY WG_CLIENT_PRIVATE_KEY WG_CLIENT_PUBLIC_KEY WG_PRESHARED_KEY SUB_TOKEN)
SETTING_KEYS=(HOST DOMAIN REALITY_SNI "${PORT_KEYS[@]}" SUBSCRIPTION BINARY)
declare -A ST=()

# ---- Helpers ---------------------------------------------------------------------------------

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
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
  --subscription        also serve the links as a subscription over HTTPS: Caddy on port 8443,
                        port 80 for its certificate (Caddy must be in the distribution's packages)
  --domain <name>       the name of the subscription address and of the Hysteria2 certificate
                        (default: the address as an sslip.io name, e.g. 192-0-2-10.sslip.io)
  --host <ip-or-name>   the server address in the links (default: the public IPv4 address,
                        from api.ipify.org)
  --reality-sni <host>  the TLS 1.3 site whose handshake Reality borrows (default: www.microsoft.com)
  --vless-port <port>   the port of VLESS + Reality, tcp (default 443)
  --hy2-port <port>     the port of Hysteria2, udp (default 443)
  --ss-port <port>      the port of Shadowsocks 2022, tcp and udp (default 8388)
  --wg-port <port>      the port of WireGuard, udp (default 51820)
  --sub-port <port>     the port of the subscription, tcp (default 8443; Caddy needs port 80 too)
  --reset               new keys, links and subscription address
  --uninstall           remove the service, sing-box, the settings and the subscription site
  --dry-run <dir>       a test: write everything under <dir> and check the config; no systemd,
                        firewall, Caddy or apt (address 192.0.2.10 unless --host is given)
  -h, --help            this help

--host, --domain, --reality-sni, the ports and --subscription are remembered for later runs,
--reset included, until --uninstall. After --reset, add the new links or subscription address in
SplitLane and remove the old ones there.
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

# apt-get with the usual umask: packages and their scripts must not get the 077 of this script.
apt_get() { (umask 022 && DEBIAN_FRONTEND=noninteractive apt-get "$@"); }

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

# ---- Arguments -------------------------------------------------------------------------------

SUBSCRIPTION_FLAG="" DOMAIN_FLAG="" HOST_FLAG="" SNI_FLAG="" RESET=0 UNINSTALL=0 DRY_RUN=0 ROOT=""
declare -A PORT_FLAG=()

parse_args() {
	local k
	while (($#)); do
		case $1 in
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
	if ((UNINSTALL)) && [[ -n $SUBSCRIPTION_FLAG$DOMAIN_FLAG$HOST_FLAG$SNI_FLAG || $RESET == 1 || ${#PORT_FLAG[@]} != 0 ]]; then
		die "--uninstall takes no other options (except --dry-run)"
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
	*) die "unsupported CPU architecture $(uname -m): the kit supports x86_64 and aarch64" ;;
	esac
	if ((!DRY_RUN)); then
		((EUID == 0)) || die "run it as root: sudo bash $0"
		local id version major need
		id=$(os_field ID 2>/dev/null || true)
		version=$(os_field VERSION_ID 2>/dev/null || true)
		major=${version%%.*}
		case $id in
		debian) need=12 ;;
		ubuntu) need=22 ;;
		*) die "this system (${id:-unknown}) is not supported: Debian 12+ or Ubuntu 22.04+ is needed" ;;
		esac
		if ! [[ $major =~ ^[0-9]+$ ]] || ((major < need)); then
			die "$id ${version:-without a version} is not supported: Debian 12+ or Ubuntu 22.04+ is needed"
		fi
		[[ -d /run/systemd/system ]] || die "systemd is not running here; the kit needs it"
		if ! command -v curl >/dev/null 2>&1; then
			say "Installing curl"
			apt_get update -qq
			apt_get install -y -qq curl ca-certificates >/dev/null
		fi
	fi
	local c
	for c in curl tar sha256sum base64 install sed; do
		command -v "$c" >/dev/null 2>&1 || die "$c is missing"
	done
}

# Ports another program already listens on. The kit's own server (on a re-run) does not count:
# the sockets of its process are left out.
check_ports() {
	command -v ss >/dev/null 2>&1 || return 0
	local p opt pid="" lines busy=()
	if systemctl is-active --quiet "$SERVICE"; then
		pid=$(systemctl show -p MainPID --value "$SERVICE" 2>/dev/null || true)
	fi
	for p in "${SERVER_PORTS[@]}"; do
		opt=-lntp
		if [[ $p == */udp ]]; then
			opt=-lnup
		fi
		lines=$(ss -H "$opt" "sport = :${p%/*}" 2>/dev/null || true)
		if [[ $pid =~ ^[1-9][0-9]*$ ]]; then
			lines=$(grep -vF -- "pid=$pid," <<<"$lines" || true)
		fi
		if [[ -n $lines ]]; then
			busy+=("$p")
		fi
	done
	if ((${#busy[@]})); then
		die "another program already uses ${busy[*]} (see: ss -lntup); free them or choose other ports with --vless-port, --hy2-port, --ss-port, --wg-port"
	fi
}

# ---- State -----------------------------------------------------------------------------------

load_state() {
	local file=$ROOT$P_STATE/state line k v
	if [[ -f $file ]]; then
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
	choose_ports
}

# The ports of this run: from the option, else the remembered one, else the default. Two ways in
# never share a port of the same kind (tcp or udp); with the subscription, Caddy's two tcp ports
# count too.
choose_ports() {
	local k v from p who
	for k in "${PORT_KEYS[@]}"; do
		from=${k%_PORT}
		from=--${from,,}-port
		if [[ -n ${PORT_FLAG[$k]:-} ]]; then
			v=${PORT_FLAG[$k]}
		elif [[ -n ${ST[$k]:-} ]]; then
			v=${ST[$k]} from="$ROOT$P_STATE/state: $k"
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
	done
	local -a uses=("$VLESS_PORT/tcp=--vless-port" "$SS_PORT/tcp=--ss-port" "$HY2_PORT/udp=--hy2-port"
		"$SS_PORT/udp=--ss-port" "$WG_PORT/udp=--wg-port")
	if [[ $SUBSCRIPTION == 1 ]]; then
		uses+=("$SUB_PORT/tcp=--sub-port" "$ACME_PORT/tcp=the certificate of the subscription")
	fi
	local -A user=()
	for p in "${uses[@]}"; do
		who=${p#*=} p=${p%%=*}
		if [[ -n ${user[$p]:-} ]]; then
			die "${user[$p]} and $who both need $p; give one of them another port"
		fi
		user[$p]=$who
	done
	# The ports as the firewall and the checks see them.
	SERVER_PORTS=("$VLESS_PORT/tcp" "$HY2_PORT/udp" "$SS_PORT/tcp" "$SS_PORT/udp" "$WG_PORT/udp")
	SUB_PORTS=("$ACME_PORT/tcp" "$SUB_PORT/tcp")
}

# ---- sing-box --------------------------------------------------------------------------------

install_singbox() {
	local bin=$ROOT$P_BIN name archive version
	if [[ -n ${ST[BINARY]:-} ]] && [[ -x $bin ]] && [[ ${ST[BINARY]} == "$SINGBOX_VERSION:$(sha256 "$bin")" ]]; then
		say "sing-box $SINGBOX_VERSION is already installed"
		return
	fi
	if [[ -e $bin && -z ${ST[BINARY]:-} ]]; then
		die "$bin exists and was not installed by this kit; remove it (and whatever uses it) first"
	fi
	name=sing-box-$SINGBOX_VERSION-linux-$ARCH
	archive=$TMP/$name.tar.gz
	say "Downloading sing-box $SINGBOX_VERSION ($ARCH)"
	curl -fsSL --proto '=https' --retry 3 --connect-timeout 20 --max-time 300 --speed-limit 1024 --speed-time 60 -o "$archive" \
		"https://github.com/SagerNet/sing-box/releases/download/v$SINGBOX_VERSION/$name.tar.gz" ||
		die "could not download sing-box; nothing was installed"
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
	mv -f -- "$bin.new" "$bin"
	version=$("$bin" version) || die "the installed sing-box does not run"
	say "Installed ${version%%$'\n'*}"
	ST[BINARY]=$SINGBOX_VERSION:$(sha256 "$bin")
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

write_config() {
	local dir=$ROOT$P_STATE sb=$ROOT$P_BIN listen="::" out
	# Without IPv6 in the kernel the server listens on IPv4 only.
	if [[ ! -e /proc/net/if_inet6 ]]; then
		listen=0.0.0.0
	fi
	cat >"$dir/config.json.new" <<EOF
{
  "log": {"level": "$LOG_LEVEL"},
  "inbounds": [
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
    },
    {
      "type": "hysteria2", "tag": "hysteria2", "listen": "$listen", "listen_port": $HY2_PORT,
      "users": [{"name": "splitlane", "password": "${ST[HY2_PASSWORD]}"}],
      "tls": {
        "enabled": true, "server_name": "${ST[HY2_CERT_NAME]}", "alpn": ["h3"],
        "certificate": [$(pem_json "$dir/hysteria2.crt")],
        "key": [$(pem_json "$dir/hysteria2.key")]
      }
    },
    {
      "type": "shadowsocks", "tag": "shadowsocks-2022", "listen": "$listen", "listen_port": $SS_PORT,
      "method": "$SS_METHOD", "password": "${ST[SS_PASSWORD]}"
    }
  ],
  "endpoints": [
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

	mkdir -p -- "$out"
	chmod 700 "$out"
	printf '%s\n' "$VLESS_LINK" "$HY2_LINK" "$SS_LINK" >"$out/links.txt"
	printf '%s\n' "$WG_CONF" >"$out/splitlane-wg.conf"
	local sub
	sub=$(base64 -w0 "$out/links.txt")
	printf '%s\n' "$sub" >"$out/subscription.txt"
	chmod 600 "$out/links.txt" "$out/splitlane-wg.conf" "$out/subscription.txt"
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

setup_subscription() {
	SUB_URL=""
	if ! command -v caddy >/dev/null 2>&1; then
		say "Installing Caddy for the subscription"
		if ! apt_get update -qq; then
			warn "subscription skipped: apt-get update failed"
			return 0
		fi
		if ! apt-cache show caddy >/dev/null 2>&1; then
			warn "subscription skipped: this distribution has no caddy package (Debian 12 and Ubuntu 24.04 have one)"
			return 0
		fi
		if ! apt_get install -y -qq caddy >/dev/null; then
			warn "subscription skipped: apt-get install caddy failed"
			return 0
		fi
	fi
	if ! getent group caddy >/dev/null || [[ ! -f $P_CADDYFILE ]]; then
		warn "subscription skipped: Caddy is not set up as the caddy package does it (group caddy, $P_CADDYFILE)"
		return 0
	fi
	write_subscription_site caddy
	add_caddy_import
	if ! caddy validate --config "$P_CADDYFILE" --adapter caddyfile >"$TMP/caddy.log" 2>&1; then
		tail -n 5 "$TMP/caddy.log" >&2
		drop_subscription_site
		warn "subscription skipped: Caddy refused the site (above)"
		return 0
	fi
	open_ports "${SUB_PORTS[@]}"
	if ! systemctl enable --quiet caddy || ! systemctl reload-or-restart caddy; then
		drop_subscription_site
		systemctl reload-or-restart caddy >/dev/null 2>&1 || true
		warn "subscription skipped: Caddy did not start with the site; is port $ACME_PORT or $SUB_PORT taken? (journalctl -u caddy)"
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

# ---- Removal ---------------------------------------------------------------------------------

uninstall() {
	if ((!DRY_RUN)); then
		((EUID == 0)) || die "run it as root: sudo bash $0 --uninstall"
	fi
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

print_result() {
	local out=$ROOT$P_OUT
	say ""
	say "Share links (in SplitLane: Key or subscription; paste one or all three):"
	say "$VLESS_LINK"
	say "$HY2_LINK"
	say "$SS_LINK"
	say ""
	say "WireGuard (in SplitLane: Configuration file, or paste the text): $out/splitlane-wg.conf"
	say "$WG_CONF"
	say ""
	if [[ -n ${SUB_URL:-} ]]; then
		say "Subscription address (in SplitLane: Key or subscription):"
		say "$SUB_URL"
		say "The address is the secret: whoever has it gets the three links. --reset changes it and every key."
		if ((!DRY_RUN)); then
			say "Caddy is getting a certificate for $DOMAIN; the address works once it has one (usually within a minute)."
		fi
		say ""
	fi
	say "Saved in $out: links.txt, splitlane-wg.conf, subscription.txt (base64 of links.txt)"
	say "Keep them private: anyone with a link or the .conf can use this server."
	if ((RESET)); then
		say "After --reset the old links, .conf and subscription address no longer work: add the new ones"
		say "in SplitLane and remove the old servers and subscription there."
	fi
}

main() {
	parse_args "$@"
	TMP=$(mktemp -d)
	trap 'rm -rf -- "$TMP"' EXIT
	if ((UNINSTALL)); then
		uninstall
		return
	fi
	preflight
	load_state
	check_state loaded
	choose_settings
	if ((!DRY_RUN)); then
		check_ports
	fi
	install_singbox
	save_state
	make_secrets
	save_state
	write_config
	write_client_files
	if ((DRY_RUN)); then
		write_unit
		SUB_URL=""
		if [[ $SUBSCRIPTION == 1 ]]; then
			write_subscription_site ""
			ST[SUBSCRIPTION]=1
		fi
		save_state
		print_result
		needed_ports
		say ""
		say "Dry run: nothing was installed or started; the files are under $ROOT (ports: ${PORTS[*]})"
		return
	fi
	open_ports "${SERVER_PORTS[@]}"
	start_service
	if [[ $SUBSCRIPTION == 1 ]]; then
		setup_subscription
		if [[ -z $SUB_URL ]]; then
			SUBSCRIPTION=0
		fi
	fi
	save_state
	if ((!UFW_ACTIVE)); then
		needed_ports
		say "Open these ports in the firewall of your VPS provider, if it has one: ${PORTS[*]}"
	fi
	print_result
}

main "$@"
