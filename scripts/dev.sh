#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -f .env ]]; then set -a; source .env; set +a; fi
mkdir -p .local
if [[ ! -d frontend/node_modules/vite ]]; then (cd frontend && ../scripts/with-node.sh npm install --no-audit --no-fund); fi
docker compose up -d --wait db
(cd backend && go build -o ../.local/api .)
(cd backend && exec ../.local/api) &
api_pid=$!
cleanup() { kill "$api_pid" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
ready=false
for ((attempt=0; attempt<60; attempt++)); do
  if curl -fsS http://127.0.0.1:8080/api/health >/dev/null 2>&1; then ready=true; break; fi
  if ! kill -0 "$api_pid" 2>/dev/null; then echo "API başlatılamadı." >&2; exit 1; fi
  sleep 1
done
if [[ "$ready" != true ]]; then echo "API hazır olmadı. PostgreSQL bağlantısını kontrol edin." >&2; exit 1; fi
echo "Uygulama: http://127.0.0.1:5173"
(cd frontend && ../scripts/with-node.sh npm run dev)
