#!/usr/bin/env bash
#
# Robustly install the Claude Code CLI globally, verifying it actually runs.
#
# Why this exists instead of a plain `npm install -g @anthropic-ai/claude-code`:
# the CLI's ~270 MB native binary ships as an npm *optionalDependency*. If that
# large download is truncated/interrupted, npm marks the optional dep "skipped"
# and STILL EXITS 0, leaving bin/claude.exe as a non-working stub — so a plain
# install can "succeed" while `claude` is actually broken. We therefore retry
# based on a functional `claude --version` check (not npm's exit code), and
# clear leftover npm staging dirs (`.claude-code-<hash>`) that otherwise cause
# ENOTEMPTY on a reinstall.
#
# Runs from devcontainer.json's postCreateCommand. Also safe to run by hand to
# repair a broken install: it will not delete a staging dir that a running
# Claude session is currently executing from.

set -uo pipefail

PKG="@anthropic-ai/claude-code"
ATTEMPTS="${CLAUDE_INSTALL_ATTEMPTS:-3}"

verify() { command -v claude >/dev/null 2>&1 && claude --version >/dev/null 2>&1; }

# Remove leftover npm install-staging dirs (cause of ENOTEMPTY), but never one
# that a live claude process is running from.
clean_staging() {
  local groot d p inuse
  groot="$(npm root -g 2>/dev/null || true)"
  [ -n "$groot" ] || return 0
  for d in "$groot"/@anthropic-ai/.claude-code-*; do
    [ -e "$d" ] || continue
    inuse=""
    for p in $(pgrep -x claude 2>/dev/null || true); do
      case "$(readlink -f "/proc/$p/exe" 2>/dev/null || true)" in
        "$d"/*) inuse=1 ;;
      esac
    done
    if [ -n "$inuse" ]; then
      echo ">> keeping in-use staging dir: $d" >&2
    else
      rm -rf "$d" 2>/dev/null || true
    fi
  done
}

for i in $(seq 1 "$ATTEMPTS"); do
  echo ">> Installing $PKG (attempt $i/$ATTEMPTS)…"
  clean_staging
  npm install -g "$PKG" || true
  if verify; then
    echo ">> OK: $(claude --version)"
    exit 0
  fi
  echo ">> claude did not run after attempt $i (likely a truncated native-binary download); retrying…" >&2
done

echo "!! claude still not working after $ATTEMPTS attempts." >&2
echo "!! Likely a repeatedly-truncated native-binary download — see the manual repair runbook." >&2
exit 1
