#!/usr/bin/env bash
# Builds the shell and the intg module from this checkout, installs the module
# into a state root of its own, and records the WSO2 Cloud dev contexts.
#
# Usage, from anywhere in the checkout:
#   modules/intg/dev/setup.sh [context]
# context is wso2cloud-dev-direct (default) or wso2cloud-dev.
#
# The state root defaults to ~/.wso2-intg-test so your own ~/.wso2 is never
# touched, and it survives between runs so a login is kept. Override it with
# WSO2_HOME. Needs Go 1.26.9 or later on PATH.
set -euo pipefail

context=${1:-wso2cloud-dev-direct}
case $context in
wso2cloud-dev | wso2cloud-dev-direct) ;;
*)
	echo "unknown context $context: use wso2cloud-dev-direct or wso2cloud-dev" >&2
	exit 64
	;;
esac

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

# The kit file's digest is kept beside the state, so a changed kit file is
# applied again while an unchanged one leaves the records, and any login they
# hold, alone.
kit=modules/intg/dev/wso2cloud-dev.yaml
stamp="$WSO2_HOME/intg-kit.sha256"
digest=$( (command -v sha256sum >/dev/null && sha256sum "$kit" || shasum -a 256 "$kit") | cut -d' ' -f1)
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$digest" ] &&
	./bin/ws context list 2>/dev/null | grep -q 'wso2cloud-dev-direct'; then
	echo "== The WSO2 Cloud dev contexts are up to date; selecting $context"
	./bin/ws context use "$context"
else
	echo "== Recording the WSO2 Cloud dev contexts, selecting $context"
	./bin/ws context apply -f "$kit" --no-install --use "$context"

	# An input file cannot state that a product has no grant: apply fills the
	# module's exchange grant in. The workaround context is made direct here
	# instead, by removing that grant and naming intg as the login product,
	# which a direct product requires. The shell validates the document on
	# its next read, which the context list below is.
	document="$WSO2_HOME/cli/contexts.yaml"
	awk '
		/^  - name: / { current = $3 }
		current == "wso2cloud-dev-direct" && /^      provider: thunder$/ { print; print "      product: intg"; next }
		current == "wso2cloud-dev-direct" && /^        grant:$/ { skip = 1; next }
		skip && /^          / { next }
		{ skip = 0; print }
	' "$document" >"$document.tmp" && mv "$document.tmp" "$document"
	./bin/ws context list
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

Switch contexts with ./bin/ws context use wso2cloud-dev or wso2cloud-dev-direct.
NEXT
