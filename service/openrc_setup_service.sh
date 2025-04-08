#!/usr/bin/env bash
set -eu
set -o pipefail
if [[ "${TRACE-0}" == "1" ]]; then set -o xtrace; fi
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# install portquiz
echo "> adding links to binaries"
ln -sf "$(realpath "$SCRIPT_DIR/../portquiz-client")" /usr/local/bin/portquiz-client
ln -sf "$(realpath "$SCRIPT_DIR/../portquiz-server")" /usr/local/bin/portquiz-server

# install service file
echo "> adding links to openrc service files"
ln -sf "$SCRIPT_DIR/server.sh" /usr/local/bin/portquiz-server-service.sh
ln -sf "$SCRIPT_DIR/openrc_portquiz" /etc/init.d/portquiz

# enable service
echo "> adding and starting service"
openrc default
rc-update add portquiz default
service portquiz start


