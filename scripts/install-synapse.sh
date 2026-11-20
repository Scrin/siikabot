#!/usr/bin/env bash
# Installs the Synapse the end-to-end tests run against, into a venv of its own, and prints the venv's path. It
# does nothing when that venv is already there. The version follows production's; bump it with production.
#
# The tests look for the venv where this puts it, or in SIIKABOT_TEST_SYNAPSE. They never install it themselves.
set -euo pipefail

version=1.165.0
venv="${SIIKABOT_TEST_SYNAPSE:-${XDG_CACHE_HOME:-$HOME/.cache}/siikabot/synapse-$version}"

if [ -x "$venv/bin/python" ] && "$venv/bin/python" -c "import synapse, sys; sys.exit(synapse.__version__ != '$version')" 2>/dev/null; then
  echo "$venv"
  exit 0
fi

echo "Installing Synapse $version into $venv" >&2
rm -rf "$venv"
mkdir -p "$(dirname "$venv")"
python3 -m venv "$venv"
"$venv/bin/pip" install --quiet --disable-pip-version-check "matrix-synapse==$version" >&2
echo "$venv"
