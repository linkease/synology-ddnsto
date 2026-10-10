#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$#" -ne 2 ]; then
    echo "Usage: $0 ddnsto_arm|ddnsto_x86 dsm6|dsm7" >&2
    exit 2
fi
exec python3 "$ROOT/tools/package.py" --package "$1" --dsm "$2"
