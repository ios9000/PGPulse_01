# Spec — Application Attribution and Trace Context

**Milestone:** M17 — Cluster as the first-class entity
**Status:** Draft for review
**Date:** 2026-11-04
**Depends on:** `docs/specs/tier-adapter.md` (ingest envelope, adapter tokens, `ext_events`)

---

## 1. Goal

Answer "which service did this?" for the workload PGPulse already sees, and let
RCA name a service, a deploy or a trace instead of stopping at "a long
transaction" or "a connection spike". Three sources, in increasing precision:

1. **`application_name`** — set by the client, visible in `pg_stat_activity`.
   Coarse, works everywhere, no application changes.
2. **SQL comments** — sqlcommenter-style key/value comments
   (`/*service='checkout',route='%2Fapi%2Forders',traceparent='00-…'*/`) parsed
   from `pg_stat_activity.query` and, with caveats, from `pg_stat_statements.query`.
3. **OpenTelemetry spans** pushed through the tier adapter, joined to PGPulse's
   statement and session data by trace context (exact) or by statement
   fingerprint and time window (approximate).

## 2. Non-goals

- PGPulse is not a tracing backend. No trace waterfall UI, no span search, no
  storage of non-database spans. Only database client spans and deploy/restart
  events are accepted.
- No instrumentation libraries or agents shipped by PGPulse. Applications use
  the sqlcommenter and OTel SDKs they already have.
- No log parsing (`log_line_prefix`, `auto_explain` output).
- No per-execution attribution from `pg_stat_statements`: it aggregates by
  `queryid` and cannot attribute individual executions. That is a PostgreSQL
  property, not a gap to engineer around.
- No storage of raw query text with literals by default.

## 3. Current state (what the code has today)

The premise "application_name is already enriched in collectors" is **only partly
true** and the spec is written against what exists:

| Where | `application_name` used? | Notes |
|---|---|---|
| `GET /instances/{id}/activity/*` (live) | Yes, returned per backend | Live query, not stored, not available to RCA history. `internal/api/activity.go` |
| Replication collectors | Yes, as the standby's name | Labels standbys, not workload. `replication_lag.go`, `replication_status.go` |
| `progress_operations.go` | Yes, as a label on progress points | Only for running maintenance operations. |
| Playbook seed SQL | Yes, ad hoc grouping | `internal/playbook/seed_connections.go` |
| `pg.connections.*` | **No** | Labels: `state` only. |
| `pg.long_transactions.*` | **No** | Labels: `type` (`active`/`waiting`); only `state = 'active'` backends — **idle-in-transaction sessions are not counted**. |
| `pg.wait_events.*`, `pg.locks.*` | **No** | |
| `pg.statements.top.*` | N/A | Labels: `queryid`; `pg_stat_statements` has no `application_name`. |

PGPulse sets its own `application_name` (`pgpulse_collector`, `pgpulse_api`,
`pgpulse_storage`, `pgpulse_explain`, `pgpulse_forecast`, `pgpulse_logical_repl`)
— those are excluded from all attribution.

No collector reads `pg_stat_activity.query` or `query_id`. `pgss_snapshot_entries`
(migration 015) stores `queryid` and `query` text per snapshot, which is what the
fingerprint join builds on.

## 4. Design

### 4.1 Per-application session metrics (new collector)

`SessionAttributionCollector` in the high-frequency group (10 s), one query over
`pg_stat_activity` (client backends only, PGPulse's own sessions excluded):

| Metric key | Labels | Meaning |
|---|---|---|
| `pg.app.connections` | `app`, `state` | Backends per application and state. |
| `pg.app.new_connections` | `app` | Backends with `backend_start` inside the last collection interval. Basis for connection-storm detection. |
| `pg.app.idle_in_tx` | `app` | Backends in `idle in transaction` / `idle in transaction (aborted)`. |
| `pg.app.oldest_xact_seconds` | `app` | `max(now() - xact_start)` across all states (fills the idle-in-transaction gap in `pg.long_transactions.*`). |
| `pg.app.waiting` | `app` | Backends with `wait_event_type IN ('Lock', 'LWLock', 'IPC', 'BufferPin')`. |

- `app` = `application_name`, trimmed, ≤ 64 chars, empty → `(none)`.
- **Cardinality cap:** top 20 applications by connection count per instance per
  cycle, everything else as `app = (other)`. Ephemeral names (`psql`, PIDs or
  UUIDs embedded in names) are normalized by configurable regexes
  (`attribution.app_name_rewrites`, default strips trailing `-<hex{8,}>` and
  `-<digits>`), so Kubernetes pod-suffixed names collapse to the deployment.
- Version gate: none needed (all columns exist in PG 14+). Uses the existing 5 s
  statement timeout.

### 4.2 Activity sampler (new collector) — trace context from live sessions

`ActivitySampleCollector`, high-frequency group, captures **non-idle client
backends** each cycle (cap 200 rows per instance per cycle, longest-running first):

```sql
SELECT pid, backend_start, xact_start, query_start, state_change, state,
       wait_event_type, wait_event, usename, datname,
       application_name, client_addr, query_id, query
FROM pg_stat_activity
WHERE backend_type = 'client backend'
  AND pid <> pg_backend_pid()
  AND state <> 'idle'
  AND application_name NOT LIKE 'pgpulse\_%'
ORDER BY coalesce(xact_start, query_start) ASC NULLS LAST
LIMIT $1
```

- `query_id` exists in PG 14+ and is non-null only when `compute_query_id` is
  `on`, or `auto` with `pg_stat_statements` loaded. If null, the sample still
  stores a PGPulse fingerprint (§4.4).
- The raw `query` text is parsed in the collector and **discarded**; only the
  parsed comment tags, trace context and fingerprint are stored (see §4.3 and
  `attribution.store_query_text`, default `false`).
- Samples of the same `(pid, backend_start, query_start)` across cycles are
  stored once, with `last_seen_at` updated.

### 4.3 Comment parsing

Parser accepts the sqlcommenter format, leading or trailing:

```
SELECT ... FROM orders WHERE id = $1 /*action='create',controller='orders',db_driver='pgx',framework='gin',route='%2Fapi%2Forders',traceparent='00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'*/
```

Rules:

- Only a comment at the **very start** or **very end** of the statement (after
  trimming whitespace and a trailing `;`) is considered. Comments in the middle
  are ignored; this avoids treating string literals or SQL hints as tags.
- Body is `key='value'` pairs separated by `,`; keys are sorted by spec but order
  is not required; values are URL-decoded after unquoting; `\'` is an escaped quote.
- **Allow-listed keys** only: `traceparent`, `tracestate`, `service`,
  `application`, `route`, `controller`, `action`, `framework`, `db_driver`,
  `job`, `team`, plus `attribution.extra_comment_keys`. Others are dropped.
- Limits: comment ≤ 2 KiB, value ≤ 256 chars after decoding. Oversize → ignored.
- `traceparent` must match W3C Trace Context v00
  (`00-<32 hex, not all 0>-<16 hex, not all 0>-<2 hex>`); invalid → dropped.
- Truncation: `pg_stat_activity.query` is cut at `track_activity_query_size`
  (default 1024 bytes). A trailing comment on a long statement is lost; the
  parser tolerates an unterminated comment by ignoring it. Recommend 4096 (a
  restart-only setting) or a leading comment (Rails `query_log_tags` with
  `prepend_comment`, Django/Flask sqlcommenter options).
- Comment content is untrusted input: it is never interpolated into SQL, is
  length-limited, and is rendered escaped in the UI.

**From `pg_stat_statements`:** `queryid` is computed from the parse tree, so
comments do not change it; the stored `query` text is the one from the first
execution that created the entry. Consequences:

- A `traceparent` in `pg_stat_statements.query` names **one arbitrary past
  execution**. PGPulse never uses it.
- Static tags (`service`, `route`, `controller`, `action`) are useful *if* one
  call site owns the statement. PGPulse records them on the statement as
  `first_seen_tags` and labels them as such everywhere they are shown. If the
  activity sampler later sees the same `query_id` with different static tags, the
  statement is marked `shared_call_sites = true` and first-seen tags are no longer
  shown as attribution.

**Prepared statements:** with the extended protocol, a driver that prepares a
statement once and reuses it keeps the comment baked into the prepared text, so
`pg_stat_activity.query` shows a **stale** `traceparent`. Detection: the same
`(pid, traceparent)` observed across samples with different `query_start` →
`trace_context_stale = true` for that pid, and exact span joins are disabled for
it. The spec documents that per-execution trace context requires unprepared or
per-execution comments (which in turn hurt plan caching); that trade-off is the
application's, not PGPulse's.

### 4.4 Fingerprint

PGPulse cannot compute `queryid` outside PostgreSQL. It computes its own
fingerprint so statement text from spans can be matched to `pg_stat_statements`
entries:

1. Remove all comments (`/* … */` nesting-aware, `-- …` to end of line).
2. Replace string literals, numeric literals, `$n` parameters and `?` with `?`.
3. Collapse `IN (?, ?, …)` and `VALUES (?, …), (?, …)` lists to a single element.
4. Lowercase everything outside double-quoted identifiers; collapse whitespace;
   strip trailing `;`.
5. `fingerprint = hex(sha256(normalized))[0:16]`.

Implemented as a hand-written tokenizer in Go (no cgo, no `libpg_query`). Known
limits: queries that differ only in literal *types* collide; queries reformatted
by the ORM between versions do not match. The fingerprint → `queryid` map is
built per instance from `pgss_snapshot_entries.query` and refreshed on each pgss
snapshot. A fingerprint mapping to more than one `queryid` (e.g. different
`search_path`) is kept as one-to-many and the join reports `ambiguous`.

### 4.5 OTel spans through the adapter

Two ingest paths, both authenticated with adapter tokens (`item_types` must
include `span`, tier `app`):

**(a) Native envelope** — new item type `span` in `pgpulse.ingest/v1`:

| Field | Required | From OTel |
|---|---|---|
| `type` = `span` | yes | — |
| `cluster_id` | yes | resource attribute `pgpulse.cluster_id` or adapter default |
| `trace_id` (32 hex), `span_id` (16 hex), `parent_span_id` | yes / yes / no | span |
| `service` | yes | `service.name` |
| `service_version` | no | `service.version` |
| `environment` | no | `deployment.environment.name` (or legacy `deployment.environment`) |
| `name` | yes | span name |
| `start`, `end` | yes | RFC 3339 nanos |
| `status` | no | `ok` \| `error` \| `unset` |
| `db_system` | yes, must be `postgresql` | `db.system.name` (or legacy `db.system`) |
| `db_namespace` | no | `db.namespace` (database name) |
| `server_address`, `server_port` | no | `server.address`, `server.port` — used to resolve the instance |
| `query_text` | no | `db.query.text` (or legacy `db.statement`) — fingerprinted on arrival, then discarded unless `store_query_text` |
| `query_summary` | no | `db.query.summary` |
| `operation` | no | `db.operation.name` |

**(b) OTLP/HTTP JSON** at `POST /api/v1/ingest/otlp/v1/traces`, so an existing
OpenTelemetry Collector can export with its `otlphttp` exporter
(`encoding: json`, `headers: {Authorization: "Bearer pgpa_…"}`). The server keeps
only spans with `kind = SPAN_KIND_CLIENT` and a PostgreSQL `db.system(.name)`;
everything else is counted as `dropped_non_db` in the response. Customers should
filter and sample in the Collector (filter processor on `db.system.name`, tail
sampling on latency/errors) — PGPulse documents a reference Collector config.

Deploy and restart events use the normal event item: tier `app`, kinds
`deploy.started`, `deploy.finished`, `deploy.rolled_back`, `app.restart`, with
attributes `service`, `version`, `previous_version`, `replicas`. Sources: CI/CD
webhook relay, Argo CD notifications, a `kubectl rollout` wrapper.

Span limits: 2 000 spans per request, 20 000 spans per minute per token. Spans
are a sample, not a census; attribution reports coverage (§4.6).

### 4.6 Joining spans to PGPulse data

| Join | Keys | Precision | Used for |
|---|---|---|---|
| **Exact** | `activity_samples.span_id` = span's `span_id` (the `traceparent` parent-id written by the client instrumentation is the DB client span) and same `trace_id` | Exact per execution | Naming the service/trace behind a specific long transaction, blocker or waiting session. |
| **Fingerprint + window** | span `fingerprint` → `queryid` (via §4.4 map) on the instance resolved from `server_address`/`server_port`/`db_namespace`; span `start` within the pgss snapshot interval | Statistical | Splitting a statement's call and time deltas across services (`share = spans of service S / all spans` for that fingerprint in the interval). |
| **application_name** | `app` label = `service` (via `attribution.service_aliases` map) | Coarse | Fallback when no spans or comments exist. |

- **Time window tolerance:** span clocks are application-host clocks. Exact
  joins need no tolerance. Fingerprint joins use the snapshot interval
  ± `max(2 s, |ext.time.ntp.offset_seconds| of the app host if known)`.
- **Instance resolution:** `server_address` matched to instance host (and port),
  then to any HAProxy/PgBouncer address registered in the cluster (the span sees
  the proxy, not the backend); a proxy address resolves to the **cluster**, and
  then to the primary for writes, unknown for reads.
- Every attributed fact carries `attribution_method` (`exact`, `fingerprint`,
  `comment`, `comment_first_seen`, `application_name`) and, for fingerprint
  joins, `coverage` = spans seen / `pg_stat_statements` calls in the interval.

## 5. RCA: new nodes and chains

All new chains start as Tier B (`experimental`).

| Node id | Name | Layer | Evidence |
|---|---|---|---|
| `service_deploy` | Service Deploy | app | events `deploy.*` (ingested); root cause `root_cause.service_deploy` |
| `service_restart` | Service Restart | app | events `app.restart` (ingested) |
| `connection_storm` | Connection Storm | session | `pg.app.new_connections` (any `app`), mechanism `mechanism.connection_storm` |
| `long_tx_by_service` | Long Transaction by Service | session | `pg.app.oldest_xact_seconds`, `pg.app.idle_in_tx`; attribution from activity samples |

| Chain id | Path | Notes |
|---|---|---|
| `chain.deploy_new_query_shift` | service_deploy → new_query → resource_shift | Gives existing chain #13 a root cause with evidence. BoundedLag 0–15 min. |
| `chain.deploy_query_regression` | service_deploy → query_regression → latency_spike | Gives existing chain #12 a root cause. BoundedLag 0–30 min. |
| `chain.restart_conn_storm_exhaustion` | service_restart → connection_storm → connection_pileup | BoundedLag 0–2 min; remediation `remediation.connection_pooling`. |
| `chain.conn_storm_memory_oom` | connection_storm → memory_pressure → oom_risk | Parallels chain #10 with a per-service cause. |
| `chain.service_longtx_bloat` | long_tx_by_service → mvcc_bloat → conn_holding | Parallels chain #11; root names the service. |
| `chain.service_longtx_block_vacuum` | long_tx_by_service → vacuum_blocked → dead_tuple_growth | Parallels chain #18. |

Matching a deploy to a statement shift: `service_deploy` evidence is stronger
(`Strength` × 1.25, capped at 1.0) when the new or regressed `queryid` is
attributed (any method other than `application_name`) to the deployed service.

Node-level attribution: the `CausalNode` contract does not change. Attribution
travels on the `TimelineEvent` (§6).

## 6. Data model deltas

### 6.1 Go types (appended fields; nothing removed)

`rca.TimelineEvent` gains:

```
Attribution *Attribution `json:"attribution,omitempty"`
Ref         string       `json:"ref,omitempty"`   // shared with tier-adapter.md
```

`Attribution`: `Service`, `ApplicationName`, `Route`, `Controller`, `Action`,
`TraceID`, `SpanID`, `PID`, `UserName`, `Database`, `QueryID`, `Fingerprint`,
`DeployVersion`, `Method`, `Coverage`.

### 6.2 Evidence export fields

A new `GET /api/v1/rca/incidents/{incidentId}/evidence` (permission `view_all`,
`format=json|ndjson`) returns one record per timeline event for attaching to
tickets and postmortems:

| Field | Description |
|---|---|
| `incident_id`, `instance_id`, `cluster_id` | Scope |
| `at`, `until` | Event time and interval end |
| `node_id`, `role`, `layer`, `provenance` | Position in the chain |
| `metric_key`, `value`, `baseline`, `z_score`, `strength` | Numeric evidence |
| `service`, `service_version`, `application_name`, `route`, `controller`, `action` | Who |
| `trace_id`, `span_id`, `trace_link` | Trace context; `trace_link` built from `attribution.trace_url_template` (e.g. `https://tempo.example.internal/trace/{trace_id}`) |
| `queryid`, `fingerprint`, `query_summary` | What (never literal SQL) |
| `pid`, `usename`, `datname`, `backend_start`, `xact_start` | Session identity (from activity samples) |
| `attribution_method`, `coverage` | How sure |
| `source`, `adapter_id`, `ref` | Provenance for ingested evidence |

### 6.3 Migration sketch — `022_app_attribution.sql`

(Numbering follows `tier-adapter.md`: 020 clusters, 021 external ingest.)

```sql
CREATE TABLE IF NOT EXISTS activity_samples (
    instance_id        TEXT NOT NULL,
    first_seen_at      TIMESTAMPTZ NOT NULL,
    last_seen_at       TIMESTAMPTZ NOT NULL,
    pid                INTEGER NOT NULL,
    backend_start      TIMESTAMPTZ NOT NULL,
    xact_start         TIMESTAMPTZ,
    query_start        TIMESTAMPTZ,
    state              TEXT NOT NULL,
    wait_event_type    TEXT,
    wait_event         TEXT,
    usename            TEXT,
    datname            TEXT,
    application_name   TEXT,
    query_id           BIGINT,
    fingerprint        TEXT,
    tags               JSONB NOT NULL DEFAULT '{}',  -- allow-listed comment tags
    trace_id           TEXT,
    span_id            TEXT,
    trace_context_stale BOOLEAN NOT NULL DEFAULT false,
    query_text         TEXT,                          -- only when store_query_text = true
    PRIMARY KEY (instance_id, pid, backend_start, query_start, first_seen_at)
);
CREATE INDEX IF NOT EXISTS idx_activity_samples_time ON activity_samples (instance_id, first_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_samples_trace ON activity_samples (trace_id, span_id) WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_activity_samples_query ON activity_samples (instance_id, query_id, first_seen_at DESC);

CREATE TABLE IF NOT EXISTS ext_spans (
    cluster_id       TEXT NOT NULL,
    instance_id      TEXT,                 -- resolved, nullable
    adapter_id       TEXT NOT NULL,
    trace_id         TEXT NOT NULL,
    span_id          TEXT NOT NULL,
    parent_span_id   TEXT,
    service          TEXT NOT NULL,
    service_version  TEXT,
    environment      TEXT,
    name             TEXT NOT NULL,
    start_at         TIMESTAMPTZ NOT NULL,
    end_at           TIMESTAMPTZ NOT NULL,
    status           TEXT,
    db_namespace     TEXT,
    server_address   TEXT,
    server_port      INTEGER,
    fingerprint      TEXT,
    query_summary    TEXT,
    operation        TEXT,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (trace_id, span_id, start_at)
);
CREATE INDEX IF NOT EXISTS idx_ext_spans_fp ON ext_spans (instance_id, fingerprint, start_at DESC);
CREATE INDEX IF NOT EXISTS idx_ext_spans_service ON ext_spans (cluster_id, service, start_at DESC);

CREATE TABLE IF NOT EXISTS statement_fingerprints (
    instance_id        TEXT NOT NULL,
    queryid            BIGINT NOT NULL,
    fingerprint        TEXT NOT NULL,
    first_seen_tags    JSONB NOT NULL DEFAULT '{}',
    shared_call_sites  BOOLEAN NOT NULL DEFAULT false,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (instance_id, queryid)
);
CREATE INDEX IF NOT EXISTS idx_stmt_fp ON statement_fingerprints (instance_id, fingerprint);
```

`022_app_attribution_timescaledb.sql` (conditional, same mechanism as
`tier-adapter.md` §7.3) converts `activity_samples` (on `first_seen_at`) and
`ext_spans` (on `start_at`) to hypertables. Span idempotency is the primary key:
re-sent spans are `duplicate`.

### 6.4 Retention

| Data | Default | Config key |
|---|---|---|
| `activity_samples` | 14 days | `attribution.retention.activity_days` |
| `ext_spans` | 7 days | `attribution.retention.spans_days` |
| `statement_fingerprints` | rows not updated for 30 days | — |
| `pg.app.*` metrics | `storage.retention_days` | — |

Incidents copy attribution into `timeline_json`, so evidence survives purges.

### 6.5 Configuration

```yaml
attribution:
  enabled: true
  store_query_text: false
  max_activity_rows: 200
  top_apps: 20
  app_name_rewrites: ["-[0-9a-f]{8,}$", "-[0-9]+$"]
  service_aliases: { "checkout-api": "checkout" }
  extra_comment_keys: []
  trace_url_template: ""
  retention: { activity_days: 14, spans_days: 7 }
```

## 7. Managed PostgreSQL (Aurora PostgreSQL, RDS for PostgreSQL)

No OS access, no superuser, `rds_superuser` can grant `pg_monitor`.

| Capability | RDS | Aurora | Notes |
|---|---|---|---|
| `application_name`, `pg.app.*` metrics | Works | Works | Plain `pg_stat_activity` with `pg_monitor`. |
| Activity sampler, comment parsing | Works | Works | `pg_monitor` grants `pg_read_all_stats`, so other users' `query` text is visible. |
| `query_id` in `pg_stat_activity` | Works on 14+ | Works on 14+ | `compute_query_id` via parameter group (dynamic). |
| `track_activity_query_size` increase | Works, reboot | Works, reboot | Static parameter in the parameter group. |
| `pg_stat_statements` and fingerprint map | Works | Works | Must be in `shared_preload_libraries` (RDS default includes it on recent versions) and created as an extension. |
| OTel span joins | Works | Works | `server_address` is the RDS endpoint or Aurora cluster/reader endpoint; Aurora writer endpoint resolves to the cluster's writer, reader endpoint to "a reader, unknown which". RDS Proxy addresses resolve to the cluster only. |
| Deploy/restart events | Works | Works | Ingested; independent of the database. |
| RCA chains with `os.*` evidence (`memory_pressure`, `oom_risk` in `chain.conn_storm_memory_oom`) | **Does not work** from collectors | **Does not work** | No agent, no `pg_read_file` on `/proc`. Can work if CloudWatch / Enhanced Monitoring metrics are pushed as `ext.host.*` samples through the tier adapter and mapped; out of scope for M17. |
| `pg_stat_activity` for Aurora storage-layer waits | Partial | Partial | Aurora-specific wait events (e.g. `IO:XactSync`) appear and are labelled as-is; no special handling. |
| Activity on Aurora readers | Works per instance | Works per instance | Each reader is a separate instance in PGPulse; attribution is per instance. |
| Comments stripped by proxies | n/a | n/a | RDS Proxy passes comments through; some poolers in statement mode with server-side prepared statements may make trace context stale (§4.3). |

## 8. Test plan

**Unit:**
- Comment parser: leading/trailing/middle comments, escaped quotes, URL
  decoding, unknown keys dropped, oversize, unterminated (truncated) comment,
  invalid and all-zero `traceparent`, nested `/* /* */ */`.
- Fingerprint: table of statement pairs that must match (literals, `$n` vs `?`,
  `IN` list lengths, whitespace, case, comments) and must not match (different
  tables, columns, operators); golden file of 200 statements from the pgss
  fixtures in `internal/collector` tests.
- Application name rewrites and top-N/`(other)` cardinality cap.
- Stale trace context detection across samples.
- Span validation: non-DB spans dropped, wrong `db.system`, bad IDs, OTLP JSON
  decoding of resource/scope/span attributes, legacy vs current semantic
  convention attribute names.
- Join logic: exact join; fingerprint one-to-many → `ambiguous`; share
  calculation; proxy address → cluster; clock-skew tolerance widening.
- RCA: each new chain on recorded fixtures; deploy-strength boost only when the
  regressed `queryid` is attributed to the deployed service.

**Auth:** span endpoints follow the tier-adapter auth matrix; evidence export:
unauthenticated 401, valid `app_admin` 200 (`view_all`), expired 401.

**Integration (`//go:build integration`, PG 14–17):** a test client opens
sessions with `application_name` and sqlcommenter comments (simple and extended
protocol), holds an idle-in-transaction session, and opens 50 connections in a
burst; assert `pg.app.*` metrics, activity samples, parsed tags, stale detection
under prepared statements, and `query_id` null vs set with `compute_query_id`
off/on. Fingerprint map built from a real `pg_stat_statements`.

**Privilege test:** the sampler runs as a role with only `pg_monitor` and sees
other roles' query text; without `pg_monitor` it degrades to
`<insufficient privilege>` rows, which are counted, not parsed, and reported in
`Quality.UnavailableDeps`.

**Load:** 200 active backends, 10 s cycle: collector query < 50 ms p99 on PG 16;
20 000 spans/min ingest sustained.

## 9. Open questions

1. **Query text privacy.** Is `store_query_text: false` (fingerprint only) the
   right default for all deployments, or should `query_summary`-style text with
   literals stripped be stored by default for readability?
2. **Fingerprint vs `libpg_query`.** The hand-written tokenizer avoids cgo but
   is less exact than libpg_query's fingerprint. Acceptable for M17?
3. **OTLP protobuf.** The OTel Collector's `otlphttp` exporter defaults to
   protobuf. JSON-only means a config change on the customer side; protobuf
   adds the `go.opentelemetry.io/proto/otlp` dependency.
4. **Service identity.** When `application_name`, the comment `service` tag and
   the span `service.name` disagree, which wins? Proposal: span > comment >
   `application_name`, with all three shown.
5. **Per-application alert rules.** `pg.app.*` makes per-service thresholds
   possible ("checkout > 300 connections"). In scope for M17 or later?
6. **Extend `pg.long_transactions.*`** to include idle-in-transaction, or keep it
   unchanged and rely on `pg.app.oldest_xact_seconds`? Changing it alters the
   meaning of an existing metric and its alert rules.
