#!/usr/bin/env bash
# Starts the whole development stack with one command (`make dev`):
# PostgreSQL in Docker, migrations, the API, the worker and the dashboard dev
# server. Ctrl+C stops everything except the database container.
#
# Works with the bash 3.2 that ships with macOS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE="$ROOT/.dev"
BIN="$STATE/bin"
cd "$ROOT"

log() { printf '\033[1m[dev]\033[0m %s\n' "$*"; }
die() { printf '\033[31m[dev]\033[0m %s\n' "$*" >&2; exit 1; }

for tool in docker go npm; do
  command -v "$tool" >/dev/null || die "$tool is required but not installed"
done

# A GOROOT exported for a different Go install makes `go` mix toolchains
# ("compile: version ... does not match go tool version"). Let the go on PATH
# use its own root instead.
if [ -n "${GOROOT:-}" ] && [ "$(GOROOT='' go env GOROOT)" != "$GOROOT" ]; then
  log "ignoring GOROOT=$GOROOT; it does not belong to $(command -v go)"
  unset GOROOT
fi

# --- configuration ---------------------------------------------------------

random_key() {
  if command -v openssl >/dev/null; then
    openssl rand -base64 32
  else
    head -c 32 /dev/urandom | base64
  fi
}

if [ ! -f .env ]; then
  log "creating .env from .env.example"
  cp .env.example .env
fi
if ! grep -Eq '^ENDPOINT_SECRET_KEY=.+' .env; then
  log "generating ENDPOINT_SECRET_KEY in .env"
  key="$(random_key)"
  if grep -q '^ENDPOINT_SECRET_KEY=' .env; then
    # A temporary file keeps this portable between GNU and BSD sed.
    sed "s|^ENDPOINT_SECRET_KEY=.*|ENDPOINT_SECRET_KEY=$key|" .env >.env.tmp && mv .env.tmp .env
  else
    printf 'ENDPOINT_SECRET_KEY=%s\n' "$key" >>.env
  fi
fi

# Load .env without overriding variables already set in the environment, so
# `PORT=9000 make dev` works.
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in '' | \#*) continue ;; esac
  name="${line%%=*}"
  case "$name" in *[!A-Za-z0-9_]* | '') continue ;; esac
  [ -n "${!name+x}" ] || export "$name=${line#*=}"
done <.env

# The local stack always runs in development mode so http:// and localhost
# receivers work, whatever APP_ENV says for other uses of .env.
export APP_ENV=development
POSTGRES_PORT="${POSTGRES_PORT:-5433}"
export DATABASE_URL="${DATABASE_URL:-postgres://hooklane:${POSTGRES_PASSWORD:-local_dev_only_change_me}@localhost:${POSTGRES_PORT}/hooklane?sslmode=disable}"

port_in_use() { nc -z 127.0.0.1 "$1" >/dev/null 2>&1; }

# Use PORT when it is free, otherwise the next free port, so another service
# on 8080 does not stop the stack from starting.
requested_port="${PORT:-8080}"
api_port="$requested_port"
while port_in_use "$api_port"; do
  api_port=$((api_port + 1))
  [ "$api_port" -le $((requested_port + 50)) ] || die "no free port near $requested_port for the API; set PORT in .env"
done
[ "$api_port" = "$requested_port" ] || log "port $requested_port is busy, the API uses $api_port"
export PORT="$api_port"

# --- database --------------------------------------------------------------

log "starting PostgreSQL on 127.0.0.1:$POSTGRES_PORT"
docker compose up -d --wait postgres >/dev/null

# --- build -----------------------------------------------------------------

log "building the API, worker and admin CLI"
mkdir -p "$BIN"
(cd api && go build -o "$BIN/" ./cmd/api ./cmd/worker ./cmd/admin)

if [ ! -d web/node_modules ]; then
  log "installing dashboard dependencies"
  (cd web && npm ci)
fi

log "applying migrations"
"$BIN/admin" migrate up | sed 's/^/      /'

# First run on an empty database: create an organization, a project and an
# API key so the dashboard can be used right away. The key goes to a file
# readable only by you instead of the terminal.
orgs="$(docker compose exec -T postgres psql -U hooklane -d hooklane -tAc 'SELECT count(*) FROM organizations')"
if [ "$orgs" = "0" ]; then
  log "empty database: bootstrapping a development organization"
  umask 077
  "$BIN/admin" bootstrap -org-name "Development" -org-slug dev -project-name default -key-name dev \
    | sed -n 's/^api_key=//p' >"$STATE/api-key"
  umask 022
  log "API key saved to .dev/api-key"
fi

# --- run -------------------------------------------------------------------

pids=()

# run NAME COMMAND...: starts a process with its output prefixed by NAME.
run() {
  local name="$1"
  shift
  "$@" > >(awk -v p="$(printf '%-7s' "$name")" '{ print p "| " $0; fflush() }') 2>&1 &
  pids+=("$!")
}

stopping=0
stop() {
  [ "$stopping" = 1 ] && return
  stopping=1
  echo
  log "stopping (PostgreSQL keeps running; make db-down stops it)"
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  wait 2>/dev/null || true
}
trap stop EXIT
trap 'stop; exit 0' INT TERM

run api "$BIN/api"
# The worker gets its own metrics listener only when asked for, so it never
# competes with the API for METRICS_ADDR.
run worker env METRICS_ADDR="${WORKER_METRICS_ADDR:-}" "$BIN/worker"
run web env HOOKLANE_API_URL="http://127.0.0.1:$PORT" npm --prefix web run dev -- --host 127.0.0.1 --strictPort

log "API        http://127.0.0.1:$PORT"
log "Dashboard  http://127.0.0.1:5173"
if [ -s "$STATE/api-key" ]; then
  log "Sign in with the key in .dev/api-key"
else
  log "Sign in with an API key (make bootstrap creates one)"
fi
log "Press Ctrl+C to stop"

# Stop everything as soon as one process exits, so a crash is not hidden
# behind the other processes' output.
while :; do
  for pid in "${pids[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      log "a process exited; shutting down"
      exit 1
    fi
  done
  sleep 1
done
