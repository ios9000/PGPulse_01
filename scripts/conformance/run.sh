#!/usr/bin/env bash
# PGPulse PostgreSQL version conformance harness.
#
# Provisions PostgreSQL 14–18, runs every collector's version-gated SQL against
# each one as a pg_monitor-only role, and writes:
#   docs/conformance/README.md     collector × version matrix + failures
#   docs/conformance/results.json  raw per-statement results
#
# Routes:
#   docker (default)  official postgres:<major> images, one container per major.
#   external          servers you provisioned yourself (e.g. PGDG apt packages +
#                     pg_createcluster). Apply scripts/conformance/initdb/01-conformance.sql
#                     to each as a superuser, then export CONFORMANCE_DSN_<major>
#                     (pgpulse_monitor role, database "postgres") and run with
#                     CONFORMANCE_ROUTE=external.
#
# Env:
#   CONFORMANCE_VERSIONS  space-separated majors (default "14 15 16 17 18")
#   CONFORMANCE_ROUTE     docker | external (default docker)
#   CONFORMANCE_OUT       output dir (default docs/conformance)
#   CONFORMANCE_KEEP=1    leave containers running afterwards
#   CONFORMANCE_STRICT=1  exit 1 if any collector is not ok
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

VERSIONS="${CONFORMANCE_VERSIONS:-14 15 16 17 18}"
ROUTE="${CONFORMANCE_ROUTE:-docker}"
OUT="${CONFORMANCE_OUT:-docs/conformance}"
BASE_PORT=54300
PREFIX=pgpulse-conformance

SERVER_FLAGS=(
  -c shared_preload_libraries=pg_stat_statements
  -c pg_stat_statements.track=all
  -c track_io_timing=on
  -c track_functions=all
  -c wal_level=logical
  -c max_replication_slots=10
)

cleanup() {
  if [[ "$ROUTE" == docker && "${CONFORMANCE_KEEP:-0}" != 1 ]]; then
    for v in $VERSIONS; do docker rm -f "$PREFIX-$v" >/dev/null 2>&1 || true; done
  fi
}
trap cleanup EXIT

wait_ready() { # container
  local c=$1
  # The image's entrypoint starts a temporary server for init scripts and then
  # restarts; wait for the final server (it logs "ready" twice).
  for _ in $(seq 1 120); do
    if [[ "$(docker logs "$c" 2>&1 | grep -c 'database system is ready to accept connections')" -ge 2 ]] \
       && docker exec "$c" pg_isready -q -h 127.0.0.1 -U postgres; then
      return 0
    fi
    if [[ "$(docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null)" != true ]]; then
      echo "container $c exited:" >&2; docker logs "$c" >&2 || true; return 1
    fi
    sleep 1
  done
  echo "timed out waiting for $c" >&2; docker logs "$c" >&2 || true
  return 1
}

TARGETS=()
case "$ROUTE" in
  docker)
    command -v docker >/dev/null || { echo "docker not found; use CONFORMANCE_ROUTE=external" >&2; exit 2; }
    for v in $VERSIONS; do
      port=$((BASE_PORT + v))
      docker rm -f "$PREFIX-$v" >/dev/null 2>&1 || true
      echo "starting postgres:$v on 127.0.0.1:$port" >&2
      docker run -d --name "$PREFIX-$v" \
        -e POSTGRES_PASSWORD=conformance-admin \
        -p "127.0.0.1:$port:5432" \
        -v "$ROOT/scripts/conformance/initdb:/docker-entrypoint-initdb.d:ro" \
        "postgres:$v" "${SERVER_FLAGS[@]}" >/dev/null
    done
    ROUTE_DESC="docker: official postgres:<major> images"
    for v in $VERSIONS; do
      wait_ready "$PREFIX-$v"
      digest="$(docker image inspect -f '{{index .RepoDigests 0}}' "postgres:$v" 2>/dev/null || echo "postgres:$v")"
      ROUTE_DESC+=", \`${digest}\`"
      TARGETS+=(-target "pg$v=postgres://pgpulse_monitor:conformance@127.0.0.1:$((BASE_PORT + v))/postgres?sslmode=disable")
    done
    ;;
  external)
    ROUTE_DESC="external: pre-provisioned clusters (e.g. PGDG apt + pg_createcluster)"
    for v in $VERSIONS; do
      var="CONFORMANCE_DSN_$v"
      if [[ -z "${!var:-}" ]]; then echo "missing $var" >&2; exit 2; fi
      TARGETS+=(-target "pg$v=${!var}")
    done
    ;;
  *) echo "unknown CONFORMANCE_ROUTE=$ROUTE" >&2; exit 2 ;;
esac

mkdir -p "$OUT"
EXTRA=()
[[ "${CONFORMANCE_STRICT:-0}" == 1 ]] && EXTRA+=(-strict)

go run ./scripts/conformance \
  "${TARGETS[@]}" \
  -route "$ROUTE_DESC" \
  -commit "$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
  -json "$OUT/results.json" \
  -md "$OUT/README.md" \
  "${EXTRA[@]}"
