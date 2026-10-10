#!/bin/sh
# Builds the Go scanning library as an Android archive (.aar) for the app.
# gomobile and gobind are built from the versions pinned in go.mod.
#
# Usage: bind.sh /absolute/path/to/output.aar
# Needs Go, ANDROID_HOME, and ANDROID_NDK_HOME pointing at an installed NDK.
set -eu
if [ $# -ne 1 ]; then
	echo "usage: $0 /absolute/path/to/output.aar" >&2
	exit 1
fi
out=$1
cd "$(dirname "$0")"

bin="$(pwd)/.bin"
go build -o "$bin/" golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
PATH="$bin:$PATH" gomobile bind \
	-target=android/arm64 \
	-androidapi 29 \
	-javapkg=com.vorsk.portquiz \
	-trimpath \
	-ldflags="-s -w" \
	-o "$out" \
	.
