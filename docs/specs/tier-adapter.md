# Spec — Tier Adapter: Ingesting External Evidence

**Milestone:** M17 — Cluster as the first-class entity
**Status:** Draft for review
**Date:** 2026-11-04
**Related specs:** `docs/specs/app-attribution.md`, `docs/specs/patroni-adapter.md`

---

## 1. Goal

PGPulse **owns** the PostgreSQL-adjacent tiers (PostgreSQL, sessions, replication,
Patroni, etcd, PgBouncer, HAProxy) and collects them itself. It **ingests** every
other tier (network and NTP, SAN and storage arrays, power and UPS, backup jobs,
application traces, deploys) as dated evidence from tools the customer already runs.

The tier adapter is the single contract for that ingestion. It is used to:

1. Push **events** (discrete, dated facts: "backup job 4411 failed at 02:14:07Z")
   and **samples** (numeric time series: "NTP offset on db-02 was +0.84 s") into
   PGPulse, by REST or by dropping files into a watched directory.
2. Place those items on the **cluster timeline**, next to what PGPulse collected.
3. Let the **RCA engine** use them as evidence for causal-graph nodes that
   PGPulse cannot observe on its own (today `network_issue` and `settings_change`
   in `internal/rca/chains.go` have empty `MetricKeys` and can never carry evidence).

## 2. Non-goals

- PGPulse does not become a general log or metrics backend. No free-text log
  search, no arbitrary dashboards over ingested series, no PromQL.
- No pull-based scraping of third-party systems (SNMP polling, CloudWatch API,
  vCenter). A small script or the customer's existing tool pushes to PGPulse.
- No bidirectional integration (PGPulse does not acknowledge Alertmanager alerts,
  does not retry backup jobs).
- No replacement for the built-in collectors. An adapter may *supplement* an
  owned tier (see `patroni-adapter.md`) but never overwrites collected data.
- No OTLP metrics or logs. OTLP traces are covered in `app-attribution.md`.
- No multi-tenant isolation beyond the cluster scope on adapter tokens.

## 3. Current state (what the code has today)

| Area | Fact | Location |
|---|---|---|
| Cluster identity | None. Patroni and etcd settings are per instance (`patroni_url`, `etcd_endpoints` on `InstanceConfig`). There is no `clusters` table and no `cluster_id` anywhere. | `internal/config/config.go`, `internal/orchestrator/runner.go` |
| Metric storage | Single `metrics` table `(time, instance_id, metric, value, labels JSONB)`, hypertable when TimescaleDB is enabled. | `migrations/001_metrics.sql`, `002_timescaledb.sql` |
| Conditional migrations | Only `002_timescaledb.sql`, matched by exact filename in `isConditional`. | `internal/storage/migrate.go` |
| Auth | Human users only: JWT (HS256) + 4 roles. No API tokens, no service principals. | `internal/auth/` |
| Rate limiting | Only the in-memory login limiter (10 failures / 15 min / IP). | `internal/auth/ratelimit.go` |
| RCA evidence | `AnomalySource.GetAnomalies` over metric keys, plus a synthetic source (`StatementDiffSource`) merged into the anomaly map in step 4b of `Engine.Analyze`. | `internal/rca/engine.go`, `statement_source.go` |
| RCA scope | Every incident records `ScopeLimitations: ["single-instance only"]`. | `internal/rca/incident.go` |
| Incident evidence | Timeline events are serialized into `rca_incidents.timeline_json`, so an incident keeps its evidence after the source rows are purged. | `migrations/016_rca_incidents.sql` |

**Prerequisite.** This spec assumes the M17 cluster-entity work lands first and
provides at minimum a `clusters` table with a stable text `id` and a nullable
`instances.cluster_id`. It is sketched in §7.1 only so the migration order is
clear; its full design belongs to the cluster-entity spec.

## 4. Concepts

| Term | Meaning |
|---|---|
| **Adapter** | A registered external producer (a script, Alertmanager, a backup tool hook, the Patroni monitor). Has an id, an owner, scopes and one or more tokens. |
| **Tier** | The layer an item describes. Owned tiers: `postgres`, `session`, `replication`, `patroni`, `etcd`, `pooler`, `proxy`. Ingested tiers: `network`, `time`, `storage`, `power`, `backup`, `host`, `app`, `alert` (unclassified alerts from an alert router). The list is closed and versioned; unknown tiers are rejected. |
| **Event** | A dated fact. Point event (`observed_at` only) or interval event (`started_at` + optional `ended_at`). Has a `kind` (dotted, e.g. `backup.job.failed`) and a `severity`. |
| **Sample** | A numeric observation of a named series at a timestamp. Stored in `metrics` under the `ext.` prefix. |
| **Heartbeat** | An item saying "adapter X is alive and covers tiers T for cluster C". Needed so RCA can tell *no evidence* from *no coverage*. |

## 5. API contract

All endpoints live under `/api/v1/ingest` and are authenticated **only** by
adapter tokens (§5.4). A user JWT is rejected with `401` on these routes, and an
adapter token is rejected on every other route. This keeps scripts from holding
human credentials and keeps a leaked adapter token from reading anything.

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/ingest` | Batch of events, samples and heartbeats in the native envelope (§5.1). |
| POST | `/api/v1/ingest/alertmanager` | Native Prometheus Alertmanager webhook body (§5.6). Alertmanager cannot reshape its payload, so PGPulse adapts it. |
| GET | `/api/v1/ingest/whoami` | Returns the adapter id, scopes, rate limits and server schema version. Used by conformance checks. |

Both POST endpoints accept `?dry_run=true`: the request is authenticated,
scope-checked, validated and rate-counted exactly as normal, and the response is
identical, but nothing is stored. Adapter authors use it for conformance checks.

Adapter administration uses normal user JWTs:

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/api/v1/adapters` | `view_all` | List adapters, scopes, last-seen, per-tier coverage. |
| POST | `/api/v1/adapters` | `instance_management` | Register an adapter. |
| PUT | `/api/v1/adapters/{id}` | `instance_management` | Change scopes, enable or disable. |
| POST | `/api/v1/adapters/{id}/tokens` | `instance_management` | Issue a token. Plaintext returned exactly once. |
| DELETE | `/api/v1/adapters/{id}/tokens/{tokenId}` | `instance_management` | Revoke a token. |
| GET | `/api/v1/clusters/{clusterId}/events` | `view_all` | Query ingested events (filters: `from`, `to`, `tier`, `kind`, `severity`, `adapter`). |

`instance_management` is held by `super_admin` and `dba`. Adapters are
infrastructure integrations, so they sit with instance management rather than
with `user_management` (`roles_admin`) or `alert_management` (`app_admin`).

### 5.1 Native envelope

```json
{
  "schema": "pgpulse.ingest/v1",
  "adapter": "chrony-push",
  "sent_at": "2026-11-04T09:12:00Z",
  "items": [ { "...": "item" } ]
}
```

- `schema` — required, exact string. A future `v2` gets its own value; the
  server accepts the versions listed by `whoami`.
- `adapter` — required, must equal the adapter id bound to the token. A mismatch
  is `403`, which catches copy-pasted tokens.
- `sent_at` — optional, producer clock. Used only to estimate producer clock
  skew (`received_at - sent_at`), which is reported by `GET /adapters`.
- `items` — 1 to 500 items. Body limit 1 MiB (uncompressed; `Content-Encoding: gzip` accepted, 8 MiB decompressed cap).

### 5.2 Item schema

Fields common to every item:

| Field | Type | Required | Notes |
|---|---|---|---|
| `type` | `event` \| `sample` \| `heartbeat` | yes | `span` is added by `app-attribution.md`. |
| `id` | string, 1–128 chars, `[A-Za-z0-9._:-]` | events: yes; samples: no | Producer-stable idempotency id (§5.3). |
| `cluster_id` | string | yes | Must exist and be within the token's cluster scope. |
| `tier` | enum (§4) | yes | Must be within the token's tier scope. |
| `source` | string, ≤ 64 | yes | The originating system (`chrony`, `pgbackrest`, `nut`, `alertmanager`). Free text, shown in the UI. |
| `member` | string, ≤ 128 | no | Host, Patroni member name or instance id this item is about. Resolved to an `instance_id` when it matches an instance in the cluster (by id, then name, then host); otherwise kept as a label only. |
| `attributes` | object | no | Flat map; values are string, number or boolean. ≤ 32 keys, key ≤ 64 chars, string value ≤ 1024 chars. Nested objects rejected. |

**Event** fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `kind` | dotted lowercase string, ≤ 96, first segment must match a registered prefix for the tier (§5.5) | yes | e.g. `backup.job.failed`. |
| `observed_at` | RFC 3339 with offset | yes, unless `started_at` given | Point events. |
| `started_at` / `ended_at` | RFC 3339 with offset | interval events | `ended_at` may arrive later in a second item with the same `id` (upsert; only `ended_at`, `severity` and `attributes` may change). |
| `severity` | `info` \| `warning` \| `critical` | yes | Drives RCA evidence strength (§9). |
| `summary` | string, ≤ 256 | yes | One line for the timeline. |
| `correlation_id` | string, ≤ 128 | no | Groups related events (job start / job end, alert firing / resolved). |
| `link` | absolute `https` URL, ≤ 512 | no | Back-link to the source system. Rendered as a link, never fetched. |

**Sample** fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | `[a-z0-9_.]`, ≤ 96 | yes | Stored as metric key `ext.<tier>.<name>`. The server adds the prefix; a name that already starts with `ext.`, `pg.`, `os.` or `cluster.` is rejected, so an adapter can never write into a collected metric key. |
| `value` | finite number | yes | NaN and ±Inf rejected. |
| `ts` | RFC 3339 with offset | yes | |
| `labels` | flat string map, ≤ 8 keys | no | Merged with `member`, `source` and `adapter` labels. High-cardinality guard: ≤ 1000 distinct label sets per (adapter, name) per day; excess rejected per item. |
| `unit` | string, ≤ 16 | no | Informational (`s`, `ms`, `bytes`, `pct`). |

**Heartbeat** fields: `ts`, `tiers` (array, within scope), optional `version`
(producer version string). Recommended every 60 s. A tier without a heartbeat for
5 minutes is reported as *not covered* (§9.3).

**Timestamp rules** (all items): must have an explicit offset; must not be more
than 5 minutes in the future (server clock) — rejected; must not be older than the
retention window for its type — rejected. Items between 7 days old and the
retention limit are accepted but flagged `late=true` and never trigger RCA.

### 5.3 Idempotency

- **Events** are unique on `(adapter_id, id)`. A repeat with identical content is
  counted as `duplicate`. A repeat with changed mutable fields (`ended_at`,
  `severity`, `attributes`) is an update; a change to any other field is
  rejected per item as `conflict`.
- **Samples** are not individually keyed (no unique index on the metrics
  hypertable). Retries are made safe by a batch-level `Idempotency-Key` header:
  the server stores `(adapter_id, key, request_sha256, response)` for 24 h and
  replays the stored response for the same key and body; the same key with a
  different body is `422`. Producers that retry without the header can create
  duplicate sample points. This is documented, not hidden.
- Partial success is normal. The response always lists per-item outcomes.

Response (`200` if the envelope was valid, even when some items were rejected):

```json
{
  "accepted": 41,
  "duplicate": 2,
  "updated": 1,
  "rejected": [
    { "index": 7, "code": "tier_out_of_scope", "message": "token is not scoped for tier 'power'" }
  ],
  "server_time": "2026-11-04T09:12:00.412Z"
}
```

Envelope errors: `400` malformed JSON or schema, `401` missing or invalid token,
`403` adapter mismatch or disabled adapter, `413` body too large, `422` idempotency
key reuse, `429` rate limited (with `Retry-After`), `503` storage unavailable
(retry with backoff).

### 5.4 Authentication and token scoping

- Token format: `pgpa_<adapter-id-prefix>_<32 random bytes base62>`. Sent as
  `Authorization: Bearer <token>`.
- Stored as SHA-256 of the token (high-entropy secrets do not need bcrypt, and
  ingest must verify on every request). Plaintext is shown once at issue.
- A token belongs to exactly one adapter. Scopes live on the adapter and can be
  narrowed per token:
  - `clusters`: list of cluster ids, or `["*"]`.
  - `tiers`: list of tiers.
  - `item_types`: subset of `event`, `sample`, `heartbeat`, `span`.
  - `kind_prefixes`: optional, e.g. `["backup."]`.
  - `expires_at`: optional; tokens without expiry are flagged in the UI.
- An adapter may hold up to 2 active tokens, so rotation is issue-new, deploy,
  revoke-old with no gap.
- Every token use updates `last_used_at` (debounced to once per minute) and every
  admin action on adapters goes to the existing audit pattern (`session_audit_log`
  is session-specific, so adapter admin actions get their own audit rows in
  `ingest_adapters_audit`, §7.2).
- This adds a non-human principal to an RBAC model that today has only users.
  It is the one architectural change in this spec and is listed in §12 for
  agreement.

### 5.5 Rate limits

In-memory token bucket per token, same process model as the login limiter (no
shared state across replicas; PGPulse is single-binary today).

| Limit | Default | Config key |
|---|---|---|
| Requests | 20 / s, burst 40 | `ingest.rate.requests_per_sec` |
| Items | 5 000 / min | `ingest.rate.items_per_min` |
| Events | 600 / min | `ingest.rate.events_per_min` |
| Distinct sample series | 2 000 per adapter | `ingest.rate.max_series` |

Exceeding a limit returns `429` for the whole request with `Retry-After`. Limits
are visible in `whoami`. Event volume is capped lower than samples on purpose:
an adapter that sends thousands of events per minute is shipping logs, which is a
non-goal.

**Registered kind prefixes per tier** (first segment of `kind`):

| Tier | Prefixes |
|---|---|
| network | `network.` |
| time | `time.` |
| storage | `storage.` |
| power | `power.` |
| backup | `backup.` |
| host | `host.` |
| app | `app.`, `deploy.` |
| alert | `alert.` |
| patroni | `patroni.` |
| etcd | `etcd.` |
| pooler | `pooler.` |
| proxy | `proxy.` |
| postgres, session, replication | `pg.` (supplementary only; see §9.4) |

### 5.6 Alertmanager webhook

Alertmanager posts its standard body (`version: "4"`) to
`/api/v1/ingest/alertmanager?cluster_id=<id>`, with the token configured through
`http_config.authorization` (`type: Bearer`). Mapping:

| PGPulse field | From |
|---|---|
| `cluster_id` | Label `pgpulse_cluster` if present, else the `cluster_id` query parameter. Neither → item rejected. |
| `tier` | Label `pgpulse_tier` if present and valid, else adapter-level label rules (e.g. `job=~"node.*"` → `host`), else `alert`. |
| `kind` | `alert.<alertname lowercased, non [a-z0-9_] → _>` |
| `id` | `<alert.fingerprint>:<startsAt unix seconds>` — stable across re-notifications of the same firing. |
| `started_at` / `ended_at` | `startsAt` / `endsAt` (`endsAt` is used only when `status = resolved`; Alertmanager sends a future `endsAt` on firing alerts). |
| `severity` | Label `severity` mapped: `critical`/`page` → critical, `warning`/`warn` → warning, anything else → info. |
| `summary` | `annotations.summary`, else `annotations.description` truncated, else `alertname`. |
| `member` | Label `instance` with port stripped, else `host`, else `nodename`. |
| `attributes` | All labels prefixed `label.`, annotation keys prefixed `annotation.`, subject to the 32-key cap (labels first). |
| `link` | `generatorURL` if `https`. |
| `source` | `alertmanager` |

Re-notifications (`repeat_interval`) collapse onto the same event id. Resolution
updates `ended_at`. Alerts that PGPulse itself generated (label
`pgpulse_origin=true`) are dropped to avoid loops.

### 5.7 File drop

For hosts that cannot reach the API (air-gapped backup servers, UPS relays on a
management VLAN with only a file share):

- Config: `ingest.file_drop.dir` (disabled when empty), `ingest.file_drop.poll_interval` (default 10 s).
- Layout: `<dir>/incoming/<adapter-id>/*.ndjson`. The adapter id comes from the
  directory, not the file content; `<adapter-id>` must be a registered, enabled
  adapter with `file_drop` enabled. Its scopes apply exactly as for a token.
- Each line is one item in the §5.2 schema. No envelope; `schema` is implied as
  `pgpulse.ingest/v1`. Optional first line `{"schema":"pgpulse.ingest/v1"}` to pin it.
- Producers must write `name.ndjson.tmp` and rename to `name.ndjson` (atomic on
  the same filesystem). Files ending in `.tmp` are ignored.
- After processing the file moves to `<dir>/processed/<adapter-id>/` (kept 7 days)
  or, if any line was rejected, to `<dir>/failed/<adapter-id>/` with a
  `name.ndjson.result.json` sidecar containing the same per-item response as the
  REST endpoint (line number as `index`).
- Trust model: the filesystem. The directory must be owned by the PGPulse service
  user with mode `0750` or stricter, and each `incoming/<adapter-id>` subdirectory
  is writable only by that producer's account. PGPulse refuses to start the file
  drop if `incoming/` is world-writable. Symlinks are not followed.
- Limits: file ≤ 16 MiB, ≤ 50 000 lines; the same rate limits as the token apply
  (the file is processed in chunks and paused, not rejected, when limited).
- Idempotency: same rules as REST. For samples, the file's SHA-256 acts as the
  batch idempotency key, so re-dropping an identical file is a no-op.

## 6. Example payloads

### 6.1 NTP offset sample (chrony, pushed by cron every minute)

```json
{
  "schema": "pgpulse.ingest/v1",
  "adapter": "chrony-push",
  "sent_at": "2026-11-04T09:12:00Z",
  "items": [
    {
      "type": "sample",
      "cluster_id": "orders-prod",
      "tier": "time",
      "source": "chrony",
      "member": "db-02.dc1.example.internal",
      "name": "ntp.offset_seconds",
      "value": 0.842,
      "unit": "s",
      "ts": "2026-11-04T09:11:58Z",
      "labels": { "ntp_source": "10.0.0.5", "stratum": "3" }
    },
    {
      "type": "event",
      "id": "chrony-db-02-unsync-20261104T0911",
      "cluster_id": "orders-prod",
      "tier": "time",
      "source": "chrony",
      "member": "db-02.dc1.example.internal",
      "kind": "time.sync.lost",
      "severity": "warning",
      "started_at": "2026-11-04T09:11:00Z",
      "summary": "chronyd reports no selectable source; offset +0.84 s and growing",
      "attributes": { "leap_status": "Not synchronised", "max_error_s": 1.2 }
    },
    {
      "type": "heartbeat",
      "cluster_id": "orders-prod",
      "tier": "time",
      "source": "chrony",
      "ts": "2026-11-04T09:12:00Z",
      "tiers": ["time"],
      "version": "chrony-push 0.3"
    }
  ]
}
```

Stored as metric `ext.time.ntp.offset_seconds` with labels
`{member, ntp_source, stratum, source, adapter}` and `instance_id` resolved from
`db-02.dc1.example.internal` if that host is a registered member.

### 6.2 Alertmanager webhook (native body, unchanged)

`POST /api/v1/ingest/alertmanager?cluster_id=orders-prod`

```json
{
  "version": "4",
  "groupKey": "{}:{alertname=\"SANVolumeLatencyHigh\"}",
  "truncatedAlerts": 0,
  "status": "firing",
  "receiver": "pgpulse",
  "groupLabels": { "alertname": "SANVolumeLatencyHigh" },
  "commonLabels": { "alertname": "SANVolumeLatencyHigh", "severity": "warning" },
  "commonAnnotations": {},
  "externalURL": "https://alertmanager.example.internal",
  "alerts": [
    {
      "status": "firing",
      "labels": {
        "alertname": "SANVolumeLatencyHigh",
        "severity": "warning",
        "pgpulse_tier": "storage",
        "array": "pure-01",
        "volume": "pg-orders-data",
        "instance": "db-01.dc1.example.internal:9100"
      },
      "annotations": {
        "summary": "Volume pg-orders-data write latency 18 ms (p99) for 5m"
      },
      "startsAt": "2026-11-04T08:57:30Z",
      "endsAt": "2026-11-04T09:17:30Z",
      "generatorURL": "https://prometheus.example.internal/graph?g0.expr=...",
      "fingerprint": "7c1d0a4b9e2f3a10"
    }
  ]
}
```

Becomes one event: `id = 7c1d0a4b9e2f3a10:1793782650`, `tier = storage`,
`kind = alert.sanvolumelatencyhigh`, `severity = warning`,
`member = db-01.dc1.example.internal`, `ended_at` unset (still firing).

### 6.3 Backup job failure (pgBackRest post-job hook)

```json
{
  "schema": "pgpulse.ingest/v1",
  "adapter": "pgbackrest-hook",
  "items": [
    {
      "type": "event",
      "id": "pgbackrest-orders-full-20261104-020000F",
      "cluster_id": "orders-prod",
      "tier": "backup",
      "source": "pgbackrest",
      "member": "db-03.dc1.example.internal",
      "kind": "backup.job.failed",
      "severity": "critical",
      "started_at": "2026-11-04T02:00:03Z",
      "ended_at": "2026-11-04T02:14:07Z",
      "correlation_id": "orders/20261104-020000F",
      "summary": "Full backup of stanza 'orders' failed after 14m04s: archive-push timeout",
      "attributes": {
        "stanza": "orders",
        "backup_type": "full",
        "repo": "repo1",
        "bytes_copied": 412316860416,
        "exit_code": 82,
        "error": "ERROR: [082]: WAL segment 0000000A00001F2B000000C4 was not archived before the 60000ms timeout"
      },
      "link": "https://backup-ui.example.internal/jobs/20261104-020000F"
    }
  ]
}
```

The interval `started_at..ended_at` matters for RCA: a backup running from 02:00
to 02:14 is evidence for disk I/O saturation and replication lag in that window
(§9.2), independent of whether it finally failed.

### 6.4 UPS on-battery SNMP trap relayed by a script

`snmptrapd` with `traphandle 1.3.6.1.2.1.33.2.1 /usr/local/bin/ups-to-pgpulse`
(RFC 1628 `upsTrapOnBattery`). The script reads the trap varbinds and posts:

```json
{
  "schema": "pgpulse.ingest/v1",
  "adapter": "ups-trap-relay",
  "items": [
    {
      "type": "event",
      "id": "ups-dc1-rack7-onbattery-1793781321",
      "cluster_id": "orders-prod",
      "tier": "power",
      "source": "snmptrapd",
      "member": "ups-dc1-rack7",
      "kind": "power.ups.on_battery",
      "severity": "critical",
      "started_at": "2026-11-04T08:35:21Z",
      "correlation_id": "ups-dc1-rack7",
      "summary": "UPS dc1-rack7 on battery; 11 min runtime remaining",
      "attributes": {
        "trap_oid": "1.3.6.1.2.1.33.2.1",
        "ups_seconds_on_battery": 4,
        "ups_estimated_minutes_remaining": 11,
        "ups_alarms_present": 1,
        "agent_addr": "10.20.7.10",
        "feeds_hosts": "db-01,db-02"
      }
    }
  ]
}
```

The matching `upsTrapOff`/power-restored trap sends the same `id` with
`ended_at`, closing the interval. The same item as a file-drop line (one line in
`incoming/ups-trap-relay/1793781321.ndjson`):

```json
{"type":"event","id":"ups-dc1-rack7-onbattery-1793781321","cluster_id":"orders-prod","tier":"power","source":"snmptrapd","member":"ups-dc1-rack7","kind":"power.ups.on_battery","severity":"critical","started_at":"2026-11-04T08:35:21Z","summary":"UPS dc1-rack7 on battery; 11 min runtime remaining","attributes":{"trap_oid":"1.3.6.1.2.1.33.2.1","ups_estimated_minutes_remaining":11}}
```

`member` here is the UPS, not a database host; `attributes.feeds_hosts` is
informational. Mapping a UPS to the hosts it feeds is an open question (§12).

## 7. Data model deltas and migration sketch

Migrations are applied in filename order and recorded by filename in
`schema_migrations`. The last migration on `master` is `019_maintenance_forecasting.sql`.
Numbers below are provisional and are re-checked at merge time.

### 7.1 Prerequisite (owned by the cluster-entity spec, shown for ordering)

`020_clusters.sql`: `clusters(id TEXT PRIMARY KEY, name, kind, created_at, updated_at)`
and `ALTER TABLE instances ADD COLUMN cluster_id TEXT REFERENCES clusters(id)`.

### 7.2 `021_external_ingest.sql`

```sql
-- Adapters (non-human producers) and their tokens
CREATE TABLE IF NOT EXISTS ingest_adapters (
    id            TEXT PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    description   TEXT NOT NULL DEFAULT '',
    enabled       BOOLEAN NOT NULL DEFAULT true,
    file_drop     BOOLEAN NOT NULL DEFAULT false,
    scope         JSONB NOT NULL,           -- {clusters, tiers, item_types, kind_prefixes}
    label_rules   JSONB NOT NULL DEFAULT '[]', -- Alertmanager tier mapping rules
    created_by    BIGINT REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS ingest_tokens (
    id            BIGSERIAL PRIMARY KEY,
    adapter_id    TEXT NOT NULL REFERENCES ingest_adapters(id) ON DELETE CASCADE,
    token_sha256  BYTEA NOT NULL UNIQUE,
    token_prefix  TEXT NOT NULL,            -- first 12 chars, for display
    scope         JSONB,                    -- optional narrowing of adapter scope
    expires_at    TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,
    created_by    BIGINT REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS ingest_adapters_audit (
    id          BIGSERIAL PRIMARY KEY,
    adapter_id  TEXT NOT NULL,
    action      TEXT NOT NULL,              -- create, update, token_issue, token_revoke, disable
    actor_id    BIGINT REFERENCES users(id),
    detail      JSONB NOT NULL DEFAULT '{}',
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ingested events
CREATE TABLE IF NOT EXISTS ext_events (
    id              BIGSERIAL,
    adapter_id      TEXT NOT NULL,
    event_key       TEXT NOT NULL,          -- producer "id"
    cluster_id      TEXT NOT NULL,
    instance_id     TEXT,                   -- resolved from member, nullable
    member          TEXT,
    tier            TEXT NOT NULL,
    source          TEXT NOT NULL,
    kind            TEXT NOT NULL,
    severity        TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    summary         TEXT NOT NULL,
    starts_at       TIMESTAMPTZ NOT NULL,   -- observed_at for point events
    ends_at         TIMESTAMPTZ,
    is_interval     BOOLEAN NOT NULL,
    correlation_id  TEXT,
    link            TEXT,
    attributes      JSONB NOT NULL DEFAULT '{}',
    late            BOOLEAN NOT NULL DEFAULT false,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, starts_at),
    UNIQUE (adapter_id, event_key, starts_at)
);
CREATE INDEX IF NOT EXISTS idx_ext_events_cluster_time ON ext_events (cluster_id, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_ext_events_cluster_tier_kind ON ext_events (cluster_id, tier, kind, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_ext_events_open ON ext_events (cluster_id) WHERE is_interval AND ends_at IS NULL;

-- Coverage, from heartbeats
CREATE TABLE IF NOT EXISTS ingest_coverage (
    adapter_id   TEXT NOT NULL,
    cluster_id   TEXT NOT NULL,
    tier         TEXT NOT NULL,
    first_seen   TIMESTAMPTZ NOT NULL,
    last_seen    TIMESTAMPTZ NOT NULL,
    version      TEXT,
    PRIMARY KEY (adapter_id, cluster_id, tier)
);

-- Batch idempotency (24 h)
CREATE TABLE IF NOT EXISTS ingest_batches (
    adapter_id      TEXT NOT NULL,
    idem_key        TEXT NOT NULL,
    request_sha256  BYTEA NOT NULL,
    response        JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (adapter_id, idem_key)
);
```

Notes:

- `starts_at` is part of the primary and unique keys so the table can become a
  hypertable (TimescaleDB requires the partitioning column in unique indexes).
  The event idempotency check is therefore `(adapter_id, event_key, starts_at)`;
  an update that would change `starts_at` is rejected as `conflict`, which §5.3
  already requires.
- Samples need **no new table**: they go into `metrics` with
  `metric = 'ext.<tier>.<name>'`. The `instance_id` is the resolved member
  instance or, for cluster-scoped series, the pseudo-instance `cluster:<cluster_id>`.
  This keeps `ThresholdAnomalySource`, `metrics/history` and retention unchanged.
- `ext_events.cluster_id` has no foreign key on purpose: retention runs per table,
  and deleting a cluster should not cascade-delete evidence still referenced by
  incident review. Orphaned rows age out.

### 7.3 `021_external_ingest_timescaledb.sql` (conditional)

```sql
SELECT create_hypertable('ext_events', 'starts_at', if_not_exists => TRUE, migrate_data => TRUE);
```

`isConditional` in `internal/storage/migrate.go` matches only the exact filename
`002_timescaledb.sql` today. It must change to "filename ends in
`_timescaledb.sql`" (002 still matches). With plain sorting, `021_external_ingest.sql`
sorts before `021_external_ingest_timescaledb.sql` (`.` < `_`), so the table exists
before it is converted.

## 8. Cluster timeline

The cluster timeline (`GET /api/v1/clusters/{clusterId}/timeline`, defined by the
cluster-entity spec) merges, per cluster and window: alert history of member
instances, RCA incidents, settings changes, collected Patroni transitions and
**ingested events**. This spec defines how ingested events appear:

- One timeline entry per `ext_events` row, in a lane per `tier`.
- Ordering key is `starts_at` (producer time). The UI also shows `received_at`
  when they differ by more than 60 s, and shows a clock-skew badge when the
  adapter's estimated skew (`received_at - sent_at`, rolling median) exceeds 2 s,
  or when an `ext.time.ntp.offset_seconds` sample for that member exceeds 0.5 s
  in the window. A timeline is only as good as its clocks; PGPulse says so
  instead of silently ordering events wrongly.
- Interval events render as bars; open intervals extend to "now" and are marked open.
- Entries carry `source`, `adapter_id`, `severity`, `summary`, `link`, and a
  `provenance: "ingested"` flag. Collected entries carry `provenance: "collected"`.
- Samples do not appear as timeline entries. They appear as series in the
  cluster's metric panels and as RCA evidence.
- Timeline entry shape (ingested):

```json
{
  "at": "2026-11-04T02:00:03Z",
  "until": "2026-11-04T02:14:07Z",
  "lane": "backup",
  "provenance": "ingested",
  "adapter_id": "pgbackrest-hook",
  "source": "pgbackrest",
  "kind": "backup.job.failed",
  "severity": "critical",
  "member": "db-03.dc1.example.internal",
  "instance_id": "orders-db-03",
  "summary": "Full backup of stanza 'orders' failed after 14m04s: archive-push timeout",
  "link": "https://backup-ui.example.internal/jobs/20261104-020000F",
  "ref": "ext_event:918273"
}
```

## 9. RCA integration

### 9.1 Node model

`CausalNode` gains two fields (appended, existing fields unchanged):

| Field | Type | Meaning |
|---|---|---|
| `Provenance` | `collected` (default) \| `ingested` \| `mixed` | Where evidence for this node comes from. |
| `EventMatch` | list of `{tier, kind_prefix, min_severity}` | For ingested nodes: which events count as evidence. |

Ingested evidence enters the engine the same way statement diffs do today: a new
`ExternalEvidenceSource` is called in step 4b of `Engine.Analyze` and merges
synthetic `AnomalyEvent`s into the anomaly map. No change to traversal, pruning
or scoring code is needed.

- Each event-backed node gets one synthetic metric key per `EventMatch` entry,
  `ext.event.<kind_prefix without trailing dot>` (e.g. `ext.event.backup.job`,
  `ext.event.patroni.failover`), so it fits `MetricKeys`, `ChainsForTrigger` and
  `findStrongestAnomaly` unchanged. The same key can be used as an RCA trigger
  metric (`POST /api/v1/instances/{id}/rca/analyze` with `{"metric": "<key>"}`).
- Synthetic `AnomalyEvent`: `Timestamp = starts_at`, `Value = 1`, `Strength` from
  severity (`critical` 1.0, `warning` 0.7, `info` 0.4), `Source = "ingested"`.
  For interval events with `PersistentState` edges, the event counts when the
  interval overlaps the analysis window.
- Sample-backed ingested nodes (e.g. NTP offset) need nothing new: their keys are
  ordinary `ext.*` metrics and `ThresholdAnomalySource` scores them.
- Instance scoping: RCA runs per instance today. For ingested evidence the source
  queries events where `instance_id = req.InstanceID` **or** (`instance_id IS NULL`
  and `cluster_id` = the instance's cluster). Cluster-wide RCA is out of scope for
  this spec.
- `TimelineEvent.Evidence` for ingested events is
  `"ingested: <source> via <adapter_id>, event <event_key>"` and
  `TimelineEvent.Layer` takes the tier name. A new optional `Ref` field
  (`ext_event:<id>`) links back.

### 9.2 New nodes and chains (all Tier B, `experimental`, until validated on real incidents)

| Node id | Name | Layer | Provenance | Evidence |
|---|---|---|---|---|
| `network_issue` (existing, now with evidence) | Network Issue | network | ingested | events `network.*` ≥ warning; samples `ext.network.packet_loss_pct`, `ext.network.rtt_ms` |
| `clock_skew` | Clock Skew | time | ingested | sample `ext.time.ntp.offset_seconds` (abs); events `time.sync.lost` |
| `storage_latency` | Storage Array Latency | storage | ingested | events `storage.*`, `alert.*` with tier storage; samples `ext.storage.write_latency_ms` |
| `backup_running` | Backup Job Running | backup | ingested | interval events `backup.job.*` (any outcome) |
| `backup_failed` | Backup Job Failed | backup | ingested | events `backup.job.failed` |
| `power_event` | Power Event | power | ingested | events `power.*` ≥ warning |
| `host_event` | Host Event | host | ingested | events `host.*` (reboot, kernel panic, hypervisor migration) |

| Chain id | Path | Edge semantics |
|---|---|---|
| `chain.network_walrecv_repllag` (existing #3) | network_issue → wal_receiver_disconnect → replication_lag | unchanged; now has evidence |
| `chain.storage_latency_checkpoint_io` | storage_latency → disk_io → query_latency | BoundedLag 0–60 s, Required; then existing semantics |
| `chain.backup_io_repllag` | backup_running → disk_io → replication_lag | PersistentState (overlap), Supporting |
| `chain.backup_archive_wal_disk` | backup_failed → wal_retention → disk_fill | BoundedLag 0–6 h, Supporting; remediation `remediation.wal_archive` |
| `chain.power_host_crash` | power_event → host_event → pg_crash | BoundedLag 0–30 min, Required |

`KnowledgeVersion` bumps to `1.1.0` when these chains ship.

`clock_skew` is deliberately not a cause in any chain. Skewed clocks rarely cause
incidents directly; they make the evidence for every other chain unreliable. It
acts as a quality modifier instead: when `|ext.time.ntp.offset_seconds|` for a
member involved in a chain exceeds 0.5 s in the window, the engine widens that
edge's lag bounds by the offset and adds `"clock skew on <member>: ±<n>s"` to
`Quality.ScopeLimitations`. HA-specific use is in `patroni-adapter.md` §7.

### 9.3 Missing coverage is not missing evidence

Today a `EvidenceRequired` edge with no anomaly kills the branch. For ingested
nodes that is wrong when nobody is sending that tier: silence means "unknown",
not "fine". Rule:

- If `ingest_coverage` shows no heartbeat for the node's tier and cluster within
  the analysis window (± 5 min), the edge is evaluated as `EvidenceSupporting` for
  this run, its score contribution is 0, and the incident's
  `Quality.UnavailableDeps` gets `"tier:<tier> (no adapter coverage)"`.
- If coverage exists and no matching event or anomaly is found, the normal rule
  applies (Required → branch killed).
- `Quality.ScopeLimitations` lists covered tiers, so a reviewer sees exactly what
  the analysis could see.

### 9.4 Ingested data for owned tiers

Adapters may send `patroni`, `etcd`, `pooler`, `proxy` and even `postgres` tier
items (e.g. a Patroni monitor with finer-grained transitions). They land in
`ext.*` keys and `ext_events`, never in collected keys. Where a node has both,
`Provenance = mixed`, and `findStrongestAnomaly` sees both keys. Conflicts between
collected and ingested data for the same fact are resolved per tier in the
adapter's own spec (see `patroni-adapter.md` §6).

## 10. Retention

| Data | Default | Config key | Mechanism |
|---|---|---|---|
| Ingested samples | same as `storage.retention_days` | — | Existing metrics retention. |
| `ext_events` | 180 days | `ingest.retention.events_days` | Daily delete by `starts_at` (or `drop_chunks` with TimescaleDB). Longer than metrics because incident review and postmortems reach back months. |
| `ingest_batches` | 24 h | — | Hourly delete. |
| `ingest_coverage` | rows not seen for 30 days | — | Daily delete. |
| `ingest_adapters_audit` | 365 days | `ingest.retention.audit_days` | Daily delete. |
| File drop `processed/` | 7 days | `ingest.file_drop.keep_processed_days` | Sweep on poll. `failed/` is never auto-deleted. |

RCA incidents already copy evidence into `timeline_json`, so purging an event does
not break an old incident; its `Ref` simply stops resolving and the UI shows
"source event expired".

## 11. Test plan

**Unit (`internal/ingest/…_test.go`, table-driven):**
- Envelope and item validation: every field rule in §5.2, including offset-less
  timestamps, future timestamps, NaN, reserved metric prefixes, nested attributes,
  32-key cap, unknown tier, kind prefix not registered for tier.
- Scope enforcement: cluster, tier, item type, kind prefix; adapter/token mismatch.
- Idempotency: duplicate event, legal update (`ended_at`), illegal update
  (`kind` change → conflict), batch key replay, batch key with different body → 422.
- Alertmanager mapping: tier by label, by rule, fallback; severity mapping;
  firing → resolved transition; re-notification collapse; `pgpulse_origin` drop.
- Member resolution: by instance id, name, host; unmatched member stays a label.
- Rate limiter: bucket refill, per-token isolation, `Retry-After`.
- `ExternalEvidenceSource`: severity → strength, interval overlap, cluster-level
  events applied to member instances, coverage gap downgrades Required to Supporting.

**Auth (per `.claude/rules/security.md`):** for each ingest route — no token (401),
user JWT (401), revoked token (401), expired token (401), valid token wrong scope
(403 / per-item reject), valid token (200). For each admin route — unauthenticated
(401), `app_admin` and `roles_admin` (403), `dba` (200), expired JWT (401).

**Integration (`//go:build integration`, testcontainers, PG 14–17, with and
without TimescaleDB):** migrations 020–021 apply on a fresh DB and on a DB at 019;
conditional hypertable migration applies only with TimescaleDB; insert/upsert
path; retention job; file drop end to end (tmp-rename, processed, failed with
sidecar, world-writable refusal, symlink ignored).

**RCA fixtures:** recorded scenario per new chain (e.g. backup interval + disk I/O
anomaly + replication lag trigger) asserting the chain is selected, the timeline
contains the ingested event with `Ref`, and the coverage-gap case reports
`UnavailableDeps` instead of killing the branch.

**Security checks:** grep-based QA scan that ingest code uses only parameterized
queries; attributes and summaries are rendered escaped in the UI; `link` must be
`https` and is never fetched server-side.

**Load:** 20 adapters × 5 000 items/min sustained for 30 min on the reference
dev box; p99 ingest latency < 200 ms; no growth in goroutines.

## 12. Open questions

1. **Non-human principal.** Adapter tokens add a principal type outside the
   4-role RBAC model. Needs agreement before implementation (project rule: no
   architecture change without discussion).
2. **Cluster pseudo-instance.** Is `cluster:<id>` in `metrics.instance_id`
   acceptable, or should cluster-scoped samples get a `cluster_id` column?
   The pseudo-instance keeps all existing queries working; the column is cleaner.
3. **Topology for non-database members.** How does PGPulse know UPS `rack7` feeds
   `db-01` and `db-02`, or SAN volume `pg-orders-data` backs `db-01`? Options:
   attribute convention (`feeds_hosts`), a small topology table edited in the UI,
   or out of scope for M17.
4. **OTLP protobuf.** If spans arrive via OTLP (see `app-attribution.md`), do we
   accept the protobuf dependency or JSON only?
5. **Multiple PGPulse replicas.** Rate limits and the file drop assume one server
   process. If HA PGPulse appears, limits move to the storage DB and the file drop
   needs a lease.
6. **Late data and RCA.** Should an incident be re-analyzed automatically when
   relevant evidence arrives after it was created (e.g. backup tool posts
   15 minutes late)? Proposal: no auto re-run in M17; show "new evidence since
   analysis" on the incident page with a manual re-run button.
7. **Tier vocabulary.** The pending collector documentation work uses
   `patroni-etcd` and `pooler` as collector tiers. This spec splits `patroni` and
   `etcd` (they fail independently) and adds `proxy`. Confirm one vocabulary.
