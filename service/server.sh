#!/usr/bin/env bash
set -eu
set -o pipefail
if [[ "${TRACE-0}" == "1" ]]; then set -o xtrace; fi
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
PID_FILE="/run/portquiz.pid"

echo $$ > "$PID_FILE"

# either address family may be unavailable, so a failed lookup is not fatal
IP4="$(curl -fsS4 --max-time 10 https://ip.toor.sh || true)"
IP6="$(curl -fsS6 --max-time 10 https://ip.toor.sh || true)"

echo "WAN IPs: ${IP4:-none} ${IP6:-none}"

if [[ -z "$IP4" && -z "$IP6" ]]; then
    echo "unable to determine any WAN IP" >&2
    exit 1
fi

exec "portquiz-server" -tcp -udp -listen "$IP4,$IP6"

