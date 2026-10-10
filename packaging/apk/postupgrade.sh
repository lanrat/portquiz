#!/bin/sh
# Runs after the portquiz-server apk package is upgraded: restart the service
# if it is running, so the new version takes over.
if rc-service --exists portquiz 2>/dev/null && rc-service portquiz status >/dev/null 2>&1; then
	rc-service portquiz restart
fi
exit 0
