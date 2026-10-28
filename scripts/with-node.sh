#!/usr/bin/env bash
set -euo pipefail
# Prefer the shell's runtime; fall back to an already-installed modern Node.
supported_node() {
  "$1" -e 'const [major,minor]=process.versions.node.split(".").map(Number);process.exit((major===20&&minor>=19)||(major===22&&minor>=12)||major>22?0:1)' 2>/dev/null
}
node_bin="$(command -v node || true)"
if [[ -z "$node_bin" ]] || ! supported_node "$node_bin"; then
  node_bin=""
  for candidate in /opt/homebrew/bin/node /usr/local/bin/node; do
    if [[ -x "$candidate" ]] && supported_node "$candidate"; then node_bin="$candidate"; break; fi
  done
fi
if [[ -z "$node_bin" ]]; then echo "Node.js 20.19+ veya 22.12+ gerekli. Güncel Node ile yeniden deneyin." >&2; exit 1; fi
export PATH="$(dirname "$node_bin"):$PATH"
exec "$@"
