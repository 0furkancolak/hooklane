#!/bin/sh
# Stops a Hooklane stack created by install.sh. Containers and networks are
# removed; the PostgreSQL data volume and the deploy directory (with .env and
# ENDPOINT_SECRET_KEY) are kept unless --purge-data is given and confirmed.
set -eu

DIR="./hooklane"
PURGE=0

usage() {
  cat <<'EOF'
Usage: uninstall.sh [--dir PATH] [--purge-data]

  --dir PATH      Deploy directory (default: ./hooklane)
  --purge-data    Also delete the database volume. You must type the name of
                  the deploy directory to confirm. This cannot be undone;
                  take a backup first (docs/deployment.md#backups).
  -h, --help      Show this help

Without --purge-data nothing is deleted except containers and networks, and
running install.sh again brings the same data back.
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir) [ "$#" -ge 2 ] || die "--dir needs a value"; DIR="$2"; shift 2 ;;
    --purge-data) PURGE=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; die "unknown option: $1" ;;
  esac
done

command -v docker >/dev/null 2>&1 || die "docker is not installed"
[ -f "$DIR/compose.yaml" ] || die "$DIR/compose.yaml not found; pass --dir"
DIR="$(cd "$DIR" && pwd)"
NAME="$(basename "$DIR")"

if [ "$PURGE" -eq 1 ]; then
  [ -t 0 ] || die "--purge-data needs an interactive terminal to confirm"
  printf 'This deletes the PostgreSQL volume of the stack in %s: all organizations,\n' "$DIR"
  printf 'projects, events and deliveries. Type the directory name (%s) to confirm: ' "$NAME"
  read -r answer
  [ "$answer" = "$NAME" ] || die "confirmation did not match; nothing was deleted"
  (cd "$DIR" && docker compose down --remove-orphans --volumes)
  printf 'Stack stopped and its volumes were deleted. %s/.env still holds the old secrets.\n' "$DIR"
else
  (cd "$DIR" && docker compose down --remove-orphans)
  printf 'Stack stopped. Data volume and %s were kept.\n' "$DIR"
fi
