#!/bin/sh
# Installs Hooklane on one Docker host: PostgreSQL, migrations, the API behind
# an nginx proxy, and a worker. Safe to re-run: an existing .env is never
# touched, and an existing database volume is reused.
#
#   ./scripts/install.sh                      # from a checkout
#   curl -fsSL https://raw.githubusercontent.com/0furkancolak/hooklane/vX.Y.Z/scripts/install.sh \
#     | sh -s -- --version vX.Y.Z             # without a checkout
#
# POSIX sh; works with the bash 3.2 and BSD tools that ship with macOS.
set -eu

REPO="0furkancolak/hooklane"
VERSION=""
VERSION_SET=""
PORT_SET=""
DIR="./hooklane"
PORT="8090"
PROJECT=""
START=1
ORG_NAME="My Organization"
ORG_SLUG="default"
API_REPLICAS=""
WORKER_REPLICAS=""

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Installs Hooklane (PostgreSQL, migrations, API behind a proxy, worker) with
Docker Compose, generates secrets, starts it and creates the first API key.

Options:
  --version TAG        Image tag to run, e.g. v0.2.0 (default: the latest
                       release, or "latest" when run from a checkout)
  --dir PATH           Deploy directory (default: ./hooklane)
  --port PORT          Host port for the proxy, bound to 127.0.0.1 (default: 8090)
  --project NAME       Docker Compose project name (default: the directory name)
  --api-replicas N     API replicas to start (default: 1)
  --worker-replicas N  Worker replicas to start (default: 1)
  --org-name NAME      Name of the first organization (default: "My Organization")
  --org-slug SLUG      Slug of the first organization (default: default)
  --no-start           Write the files only; do not start anything
  -h, --help           Show this help

Environment:
  HOOKLANE_IMAGE       Image repository to run (default: ghcr.io/0furkancolak/hooklane)
  HOOKLANE_RAW_BASE    Base URL to download deploy files from (default: GitHub raw,
                       at the tag given by --version). Ignored in a checkout.

Secrets go to <dir>/.env (mode 600). Back up ENDPOINT_SECRET_KEY from that file.
EOF
}

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_value() { [ "$#" -ge 2 ] || die "$1 needs a value"; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) need_value "$@"; VERSION="$2"; VERSION_SET=1; shift 2 ;;
    --dir) need_value "$@"; DIR="$2"; shift 2 ;;
    --port) need_value "$@"; PORT="$2"; PORT_SET=1; shift 2 ;;
    --project) need_value "$@"; PROJECT="$2"; shift 2 ;;
    --api-replicas) need_value "$@"; API_REPLICAS="$2"; shift 2 ;;
    --worker-replicas) need_value "$@"; WORKER_REPLICAS="$2"; shift 2 ;;
    --org-name) need_value "$@"; ORG_NAME="$2"; shift 2 ;;
    --org-slug) need_value "$@"; ORG_SLUG="$2"; shift 2 ;;
    --no-start) START=0; shift ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; die "unknown option: $1" ;;
  esac
done

case "$PORT" in '' | *[!0-9]*) die "--port must be a number" ;; esac
[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || die "--port must be between 1 and 65535"
case "$ORG_SLUG" in '' | *[!a-z0-9-]*) die "--org-slug may contain lowercase letters, digits and dashes only" ;; esac
for n in "$API_REPLICAS" "$WORKER_REPLICAS"; do
  case "$n" in *[!0-9]*) die "replica counts must be numbers" ;; esac
done

# --- prerequisites -----------------------------------------------------------

command -v docker >/dev/null 2>&1 || die "docker is not installed (https://docs.docker.com/get-docker/)"
docker info >/dev/null 2>&1 || die "cannot talk to the Docker daemon; is it running, and may you use it?"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required ('docker compose version' failed)"
if command -v curl >/dev/null 2>&1; then
  HTTP=curl
elif command -v wget >/dev/null 2>&1; then
  HTTP=wget
else
  die "curl or wget is required"
fi

# GET URL: prints the body, fails on HTTP errors.
http_get() {
  if [ "$HTTP" = curl ]; then curl -fsSL --max-time 30 "$1"; else wget -q -T 30 -O - "$1"; fi
}

# --- where do the deploy files come from? -------------------------------------

SRC=""
case "$0" in
  */*) script_dir="$(cd "$(dirname "$0")" 2>/dev/null && pwd)" || script_dir="" ;;
  *) script_dir="" ;;
esac
if [ -n "$script_dir" ] && [ -f "$script_dir/../deploy/compose.production.yaml" ]; then
  SRC="$(cd "$script_dir/../deploy" && pwd)"
fi

if [ -z "$VERSION" ]; then
  if [ -n "$SRC" ]; then
    VERSION="latest"
  else
    log "looking up the latest release"
    VERSION="$(http_get "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
      sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)" || VERSION=""
    [ -n "$VERSION" ] || die "could not determine the latest release; pass --version vX.Y.Z"
  fi
fi
case "$VERSION" in '' | *[!A-Za-z0-9._-]*) die "invalid --version: $VERSION" ;; esac

TMP="$(mktemp -d "${TMPDIR:-/tmp}/hooklane-install.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT INT TERM

fetch_deploy_file() { # NAME
  if [ -n "$SRC" ]; then
    cp "$SRC/$1" "$TMP/$1"
  else
    base="${HOOKLANE_RAW_BASE:-https://raw.githubusercontent.com/$REPO/$VERSION/deploy}"
    http_get "$base/$1" >"$TMP/$1" || die "could not download $base/$1 (is $VERSION a release tag?)"
  fi
  [ -s "$TMP/$1" ] || die "downloaded $1 is empty"
}

fetch_deploy_file compose.production.yaml
fetch_deploy_file nginx.conf.template
if [ -n "$SRC" ]; then
  log "using deploy files from $SRC"
else
  log "downloaded deploy files for $VERSION"
fi

# --- deploy directory --------------------------------------------------------

mkdir -p "$DIR"
DIR="$(cd "$DIR" && pwd)"

# install_file SRC DEST: copies, keeping a backup when DEST exists and differs.
install_file() {
  if [ -f "$2" ]; then
    if cmp -s "$1" "$2"; then return 0; fi
    backup="$2.bak.$(date +%Y%m%d%H%M%S)"
    cp "$2" "$backup"
    warn "$(basename "$2") changed; previous copy saved as $(basename "$backup")"
  fi
  cp "$1" "$2"
}
install_file "$TMP/compose.production.yaml" "$DIR/compose.yaml"
install_file "$TMP/nginx.conf.template" "$DIR/nginx.conf.template"

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$1"
  else
    head -c "$1" /dev/urandom | od -An -vtx1 | tr -d ' \n'
  fi
}
random_b64_32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32
  else
    head -c 32 /dev/urandom | base64
  fi
}

# set_env KEY VALUE: replaces KEY=... in .env or appends it. Used only for
# settings given on the command line; generated secrets are never rewritten.
set_env() {
  if grep -q "^$1=" "$DIR/.env"; then
    sed "s|^$1=.*|$1=$2|" "$DIR/.env" >"$DIR/.env.tmp" && cat "$DIR/.env.tmp" >"$DIR/.env" && rm -f "$DIR/.env.tmp"
  else
    printf '%s=%s\n' "$1" "$2" >>"$DIR/.env"
  fi
}

if [ -f "$DIR/.env" ]; then
  log "keeping existing $DIR/.env"
  grep -Eq '^ENDPOINT_SECRET_KEY=.+' "$DIR/.env" || die "$DIR/.env has no ENDPOINT_SECRET_KEY; refusing to generate one over an existing installation"
  # Only settings passed explicitly on this run are updated.
  [ -z "$VERSION_SET" ] || set_env HOOKLANE_VERSION "$VERSION"
  [ -z "$PORT_SET" ] || set_env HOOKLANE_PORT "$PORT"
  [ -z "$API_REPLICAS" ] || set_env HOOKLANE_API_REPLICAS "$API_REPLICAS"
  [ -z "$WORKER_REPLICAS" ] || set_env HOOKLANE_WORKER_REPLICAS "$WORKER_REPLICAS"
else
  log "creating $DIR/.env with generated secrets"
  (
    umask 077
    {
      printf '# Generated by scripts/install.sh. Contains secrets; keep it private.\n'
      printf '# Back up ENDPOINT_SECRET_KEY: without it stored endpoint secrets cannot be decrypted.\n'
      printf 'HOOKLANE_VERSION=%s\n' "$VERSION"
      printf 'POSTGRES_PASSWORD=%s\n' "$(random_hex 24)"
      printf 'ENDPOINT_SECRET_KEY=%s\n' "$(random_b64_32)"
      printf 'HOOKLANE_BIND=127.0.0.1\n'
      printf 'HOOKLANE_PORT=%s\n' "$PORT"
      printf 'HOOKLANE_API_REPLICAS=%s\n' "${API_REPLICAS:-1}"
      printf 'HOOKLANE_WORKER_REPLICAS=%s\n' "${WORKER_REPLICAS:-1}"
      [ -z "$PROJECT" ] || printf 'COMPOSE_PROJECT_NAME=%s\n' "$PROJECT"
      [ -z "${HOOKLANE_IMAGE:-}" ] || printf 'HOOKLANE_IMAGE=%s\n' "$HOOKLANE_IMAGE"
      [ -z "${HOOKLANE_APP_ENV:-}" ] || printf 'HOOKLANE_APP_ENV=%s\n' "$HOOKLANE_APP_ENV"
      [ -z "${HOOKLANE_PULL_POLICY:-}" ] || printf 'HOOKLANE_PULL_POLICY=%s\n' "$HOOKLANE_PULL_POLICY"
    } >"$DIR/.env"
  )
  chmod 600 "$DIR/.env"
fi

EFFECTIVE_PORT="$(sed -n 's/^HOOKLANE_PORT=//p' "$DIR/.env" | tail -n 1)"
EFFECTIVE_PORT="${EFFECTIVE_PORT:-$PORT}"

cat <<EOF

IMPORTANT: back up the ENDPOINT_SECRET_KEY line of
    $DIR/.env
somewhere safe (a password manager). Without it, stored endpoint secrets
cannot be decrypted and deliveries to those endpoints will fail.

EOF

if [ "$START" -eq 0 ]; then
  log "files written to $DIR; start with: cd $DIR && docker compose up -d"
  exit 0
fi

# --- start -------------------------------------------------------------------

compose() { (cd "$DIR" && docker compose "$@"); }

log "starting the stack (this pulls images on the first run)"
compose up -d --remove-orphans || die "docker compose up failed; see 'cd $DIR && docker compose logs'"

log "waiting for the API to become ready on 127.0.0.1:$EFFECTIVE_PORT"
ready=0
i=0
while [ "$i" -lt 90 ]; do
  if http_get "http://127.0.0.1:$EFFECTIVE_PORT/readyz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 2
done
if [ "$ready" -ne 1 ]; then
  compose ps >&2 || true
  die "the API did not become ready within 3 minutes; check 'cd $DIR && docker compose logs migrate api'"
fi
log "ready"

# --- first organization and API key --------------------------------------------

orgs="$(compose exec -T postgres psql -U hooklane -d hooklane -tAc 'SELECT count(*) FROM organizations' 2>/dev/null | tr -d '[:space:]')" || orgs=""
API_KEY=""
if [ "$orgs" = "0" ]; then
  log "creating the first organization and API key"
  # The key is read from the command's output, never passed as an argument,
  # so it does not appear in shell history or the process list.
  out="$(compose run --rm --no-deps -T migrate hooklane-admin bootstrap \
    -org-name "$ORG_NAME" -org-slug "$ORG_SLUG" -project-name default -key-name install)" ||
    die "bootstrap failed; run it again later with: cd $DIR && docker compose run --rm --no-deps migrate hooklane-admin bootstrap -org-name ... -org-slug ..."
  API_KEY="$(printf '%s\n' "$out" | sed -n 's/^api_key=//p')"
  PROJECT_ID="$(printf '%s\n' "$out" | sed -n 's/^project_id=//p')"
  [ -n "$API_KEY" ] || die "bootstrap printed no API key"
elif [ -n "$orgs" ]; then
  log "an organization already exists; not creating another"
else
  warn "could not check for existing organizations; skipping bootstrap"
fi

cat <<EOF

Hooklane is running.

  Dashboard / API   http://127.0.0.1:$EFFECTIVE_PORT
  Deploy directory  $DIR
  Logs              cd $DIR && docker compose logs -f
  Scale             cd $DIR && docker compose up -d --scale api=3 --scale worker=3
  Stop              cd $DIR && docker compose down   (data volume is kept)

The proxy listens on 127.0.0.1 only. Put a TLS terminator in front of it
before exposing Hooklane (see docs/deployment.md).
EOF

if [ -n "$API_KEY" ]; then
  cat <<EOF

================ FIRST API KEY (shown once, store it now) ================
  $API_KEY
  project id: ${PROJECT_ID:-unknown}
===========================================================================
Sign in to the dashboard with this key. More keys can be created there.
EOF
else
  printf '\nNo new API key was created. Create one in the dashboard or via the API.\n'
fi
