## Install migrate with these commands.
# sudo apt-get install curl -y && \
# curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz && \
# sudo mv migrate /usr/local/bin/migrate

#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}/../control-panel/cmd/main"

pushd "$APP_DIR" >/dev/null
go run main.go run-migrations-up --dev
popd >/dev/null