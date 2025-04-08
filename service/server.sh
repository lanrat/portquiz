#!/usr/bin/env bash
set -eu
set -o pipefail
if [[ "${TRACE-0}" == "1" ]]; then set -o xtrace; fi
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
PID_FILE="/run/portquiz.pid"

echo $$ > "$PID_FILE"

IP4="$(curl -s4 http://ip.toor.sh)"
IP6="$(curl -s6 http://ip.toor.sh)"

echo "WAN IPs: $IP4 $IP6"

exec "portquiz-server" -tcp -udp -listen "$IP4,$IP6"


