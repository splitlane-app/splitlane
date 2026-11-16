# A test server for the end-to-end check of "Your own VPS" (run-e2e.ps1 next to this file): Debian
# or Ubuntu with systemd and sshd, like a freshly rented VPS. Only for local runs on Docker Desktop.
#
#   docker build --build-arg BASE=debian:12 -t splitlane-e2e:debian-12 scripts/server/e2e
#   docker run -d --privileged --cgroupns=host -p 127.0.0.1:2222:22 \
#     -e E2E_ROOT_PASSWORD=... -e E2E_KIT_PASSWORD=... [-e E2E_KIT_KEY="ssh-ed25519 ..."] \
#     splitlane-e2e:debian-12
#
# root logs in with its password; the user kit logs in with its password or its key and runs sudo
# with its password. entry.sh sets them from the environment when the container starts.
ARG BASE=debian:12
FROM ${BASE}

# What a minimal VPS image has: systemd, sshd, sudo, ss (iproute2), python3 and CA certificates.
# On Ubuntu, sudo is what ubuntu-minimal pulls in: sudo-rs too from 25.10 on, and then sudo-rs is
# the sudo of the system. netcat takes ports for the check. No curl: the kit installs it.
RUN set -eu; \
	export DEBIAN_FRONTEND=noninteractive; \
	apt-get update; \
	sudo_pkgs=sudo; \
	if apt-cache depends ubuntu-minimal 2>/dev/null | grep -qx '  Depends: sudo-rs'; then sudo_pkgs='sudo sudo-rs'; fi; \
	apt-get install -y --no-install-recommends systemd systemd-sysv dbus openssh-server $sudo_pkgs \
		iproute2 netcat-openbsd python3 procps ca-certificates; \
	rm -rf /var/lib/apt/lists/*

# Password logins for root as many providers hand a VPS over; kit is an administrator through sudo.
RUN set -eu; \
	useradd --create-home --shell /bin/bash --groups sudo kit; \
	printf '%s\n' 'PermitRootLogin yes' 'PasswordAuthentication yes' >/etc/ssh/sshd_config.d/splitlane-e2e.conf

COPY entry.sh /usr/local/sbin/splitlane-e2e-entry
RUN chmod 755 /usr/local/sbin/splitlane-e2e-entry

STOPSIGNAL SIGRTMIN+3
ENTRYPOINT ["/usr/local/sbin/splitlane-e2e-entry"]
