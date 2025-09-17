#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}/../control-panel/cmd/main"

pushd "$APP_DIR" >/dev/null
go run main.go sync-parts-to-xero --dev
popd >/dev/null