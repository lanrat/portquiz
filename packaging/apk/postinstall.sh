#!/bin/sh
# Runs after the portquiz-server apk package is first installed.
cat <<'MSG'

portquiz-server installed. To run it as a service:
  1. set PORTQUIZ_LISTEN in /etc/conf.d/portquiz to IP addresses dedicated to
     portquiz; all traffic to them is redirected, including SSH
  2. rc-update add portquiz default
  3. rc-service portquiz start

MSG
