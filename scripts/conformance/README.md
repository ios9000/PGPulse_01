# PostgreSQL conformance harness

Executes every collector's SQL — in the variant the collector's own version gating selects —
against real PostgreSQL 14, 15, 16, 17 and 18 and writes the collector × version matrix to
[`docs/conformance/README.md`](../../docs/conformance/README.md) (raw data: `results.json`).

```bash
scripts/conformance/run.sh
```

## How it works

| Piece | What it does |
|---|---|
| `run.sh` | Provisions one server per major, waits for it, runs the Go program, tears servers down. |
| `initdb/01-conformance.sql` | Fixture: `pgpulse_monitor` role (`pg_monitor` **only**), `pg_stat_statements`, a physical and a logical replication slot, and a seeded `conformance` database (tables, dead tuples, unlogged table, sequence, function, large object). |
| `harness.go` | Builds every collector with the same constructors as `orchestrator.buildCollectors`, passes it the `version.PGVersion` from `version.Detect`, and calls `Collect` / `CollectDB` on a connection with a pgx `QueryTracer`. The tracer records each statement's SQL, arguments, row count and error. Nothing re-implements gate selection: what runs is whatever the collector chose. |
| `report.go` | Renders the matrix, the grouped failure list and per-statement detail. |
| `harness_test.go` | Drift guard: fails if `internal/collector` gains a `New*Collector` that the harness does not run (or explicitly exclude). |

Server flags: `shared_preload_libraries=pg_stat_statements`, `track_io_timing=on`,
`track_functions=all`, `wal_level=logical`.

Not run (no SQL against the monitored connection): `NewOSCollector` (pgpulse-agent over HTTP),
`NewClusterCollector` (Patroni / etcd). `replication_status` is also run with `IsRecovery=true`
so its replica-only query is executed. `statements_snapshot` runs
`statements.SnapshotCapturer.CaptureInstance` (the `pgssGate` user) with a no-op store.

The harness reports; it never fixes. Failures are listed with the exact server error.

## Routes

| Route | When | How |
|---|---|---|
| `docker` (default) | Docker daemon available (GitHub runners, this repo's cloud sandbox) | Official `postgres:14` … `postgres:18` images on `127.0.0.1:54314` … `54318`. Image digests are recorded in the report. |
| `external` | No Docker | Install PGDG packages (`apt.postgresql.org`), one cluster per major (`pg_createcluster <major> conformance -p <port> -- ` then set the server flags above in `postgresql.conf`), apply `initdb/01-conformance.sql` with `psql` as `postgres`, then run with `CONFORMANCE_ROUTE=external CONFORMANCE_DSN_14=postgres://pgpulse_monitor:conformance@127.0.0.1:<port>/postgres?sslmode=disable …`. |

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `CONFORMANCE_VERSIONS` | `14 15 16 17 18` | Majors to check |
| `CONFORMANCE_ROUTE` | `docker` | `docker` or `external` |
| `CONFORMANCE_OUT` | `docs/conformance` | Output directory |
| `CONFORMANCE_KEEP` | `0` | `1` leaves containers running |
| `CONFORMANCE_STRICT` | `0` | `1` exits non-zero if any collector is not ok |

Exit codes of the Go program: `0` report written, `1` strict mode and something not ok,
`2` usage, `3` at least one server could not be reached (its column is reported as not executed —
never faked).

## CI

`.github/workflows/conformance.yml` runs weekly and on manual dispatch (not on push). It
publishes the matrix to the job summary, uploads it as an artifact, runs `TestDetectLive`
against the live servers, and warns when the committed `results.json` is stale.
