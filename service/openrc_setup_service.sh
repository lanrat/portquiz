#!/bin/sh
# Installs the portquiz server as an OpenRC service, e.g. on Alpine Linux.
#
# Run as root from an extracted release archive, or from a source checkout
# after running make. Running it again upgrades the installed binaries and
# service script; an existing /etc/conf.d/portquiz is kept.
set -eu
if [ "${TRACE-0}" = "1" ]; then set -x; fi

SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)
SRC_DIR=$(dirname -- "$SCRIPT_DIR")
BIN_DIR=/usr/local/bin

fail() {
	echo "error: $*" >&2
	exit 1
}

# replace installs $1 as $2 with mode $3, by renaming a new copy over the old
# one, so a running binary or a symlink left by an earlier install is replaced
# rather than written through
replace() {
	install -m "$3" "$1" "$2.new"
	mv -f "$2.new" "$2"
}

[ "$(id -u)" -eq 0 ] || fail "must be run as root"
command -v openrc-run >/dev/null 2>&1 || fail "OpenRC is not installed"
for bin in portquiz-server portquiz; do
	[ -f "$SRC_DIR/$bin" ] || fail "$SRC_DIR/$bin not found; run make first when using a source checkout"
done

if ! command -v iptables >/dev/null 2>&1 || ! command -v ip6tables >/dev/null 2>&1; then
	if command -v apk >/dev/null 2>&1; then
		echo "> installing iptables"
		apk add iptables ip6tables
	else
		fail "iptables and ip6tables are required"
	fi
fi

echo "> installing binaries to $BIN_DIR"
for bin in portquiz-server portquiz; do
	replace "$SRC_DIR/$bin" "$BIN_DIR/$bin" 0755
done

echo "> installing OpenRC service"
replace "$SCRIPT_DIR/openrc_portquiz" /etc/init.d/portquiz 0755
if [ -e /etc/conf.d/portquiz ]; then
	echo "> keeping existing /etc/conf.d/portquiz"
else
	install -m 0644 "$SCRIPT_DIR/openrc_portquiz.confd" /etc/conf.d/portquiz
fi

# remove the wrapper script linked by earlier versions of this installer
if [ -L "$BIN_DIR/portquiz-server-service.sh" ]; then
	rm -f "$BIN_DIR/portquiz-server-service.sh"
fi

rc-update add portquiz default

PORTQUIZ_LISTEN=""
# shellcheck source=/dev/null
. /etc/conf.d/portquiz
if [ -z "$PORTQUIZ_LISTEN" ]; then
	echo
	echo "Set PORTQUIZ_LISTEN in /etc/conf.d/portquiz to the IP addresses to"
	echo "listen on, then start the service with: rc-service portquiz start"
else
	echo "> starting service"
	rc-service portquiz restart
fi
