#!/bin/sh
# Runs before the portquiz-server apk package is removed: stop the service
# while its binary still exists, so the server removes its iptables rules,
# and take it out of every runlevel so no link to the removed script is left.
if rc-service --exists portquiz 2>/dev/null; then
	if rc-service portquiz status >/dev/null 2>&1; then
		rc-service portquiz stop
	fi
	rc-update --all delete portquiz >/dev/null 2>&1
fi
exit 0
