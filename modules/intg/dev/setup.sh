#!/usr/bin/env bash
# Builds the shell and the intg module from this checkout, installs the module
# into a state root of its own, and records the WSO2 Cloud dev context.
#
# Usage, from anywhere in the checkout:
#   modules/intg/dev/setup.sh
#
# The state root defaults to ~/.wso2-intg-test so your own ~/.wso2 is never
# touched, and it survives between runs so a login is kept. Override it with
# WSO2_HOME. Needs Go 1.26.9 or later on PATH.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repo_root"
export WSO2_HOME=${WSO2_HOME:-$HOME/.wso2-intg-test}
mkdir -p "$WSO2_HOME" bin

echo "== Building bin/ws"
go build -ldflags "-X github.com/wso2/wso2-cli/internal/version.shellVersion=1.0.0-dev -X github.com/wso2/wso2-cli/internal/output.commandName=ws" \
	-o bin/ws ./cmd/wso2

echo "== Installing the intg module into $WSO2_HOME"
# A rebuilt module is reinstalled from scratch so the shell's recorded digest
# matches the binary it launches.
./bin/ws product remove intg >/dev/null 2>&1 || true
go run ./cmd/wso2-module-dev -namespace intg -shell-version 1.0.0-dev \
	-shell-protocols checkout -shell-path ./bin/ws >/dev/null
./bin/ws version

# An earlier kit also recorded a workaround context; it is no longer needed.
if ./bin/ws context list 2>/dev/null | grep -q 'wso2cloud-dev-direct'; then
	echo "== Removing the earlier kit's wso2cloud-dev-direct context"
	./bin/ws context delete wso2cloud-dev-direct
fi

# The kit file's digest is kept beside the state, so a changed kit file is
# applied again while an unchanged one leaves the record, and any login it
# holds, alone.
kit=modules/intg/dev/wso2cloud-dev.yaml
stamp="$WSO2_HOME/intg-kit.sha256"
digest=$( (command -v sha256sum >/dev/null && sha256sum "$kit" || shasum -a 256 "$kit") | cut -d' ' -f1)
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$digest" ] &&
	./bin/ws context list 2>/dev/null | grep -q 'wso2cloud-dev'; then
	echo "== The WSO2 Cloud dev context is up to date"
	./bin/ws context use wso2cloud-dev
else
	echo "== Recording the WSO2 Cloud dev context"
	./bin/ws context apply -f "$kit" --no-install --use wso2cloud-dev
	echo "$digest" >"$stamp"
fi

cat <<NEXT

Ready. In this terminal, or any other, first run:
  export WSO2_HOME=$WSO2_HOME
  cd $repo_root

Then:
  ./bin/ws intg status
  ./bin/ws login                          # opens the browser; sign in and pick your organization
  ./bin/ws intg project list --verbose
  ./bin/ws intg project list -o json
NEXT
