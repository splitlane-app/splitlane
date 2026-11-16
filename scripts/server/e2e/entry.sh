#!/bin/sh
# The entry of the e2e test server (Dockerfile next to this file): sets the passwords of root and kit
# and the key of kit from the environment, then becomes systemd. The variables are dropped first, so
# no service of the server sees them.
set -eu
: "${E2E_ROOT_PASSWORD:?}" "${E2E_KIT_PASSWORD:?}"
printf 'root:%s\n' "$E2E_ROOT_PASSWORD" | chpasswd
printf 'kit:%s\n' "$E2E_KIT_PASSWORD" | chpasswd
if [ -n "${E2E_KIT_KEY:-}" ]; then
	install -d -m 700 -o kit -g kit /home/kit/.ssh
	printf '%s\n' "$E2E_KIT_KEY" >/home/kit/.ssh/authorized_keys
	chown kit:kit /home/kit/.ssh/authorized_keys
	chmod 600 /home/kit/.ssh/authorized_keys
fi
unset E2E_ROOT_PASSWORD E2E_KIT_PASSWORD E2E_KIT_KEY
exec /sbin/init
