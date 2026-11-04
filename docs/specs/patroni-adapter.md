# Spec — Patroni Adapter: Contract for an External Patroni Monitor

**Milestone:** M17 — Cluster as the first-class entity
**Status:** Draft for review
**Date:** 2026-11-04
**Depends on:** `docs/specs/tier-adapter.md` (envelope, tokens, `ext_events`, RCA evidence source)

---

## 1. Goal

Define the contract an external, Patroni-focused monitoring tool follows to feed
PGPulse as the **first tier adapter**. PGPulse already polls Patroni and etcd at a
coarse interval. The external tool runs next to the cluster, watches it
continuously, and pushes what polling misses: every per-member state transition,
DCS latency, and a phase-by-phase failover timeline. PGPulse places those on the
cluster timeline and uses them as RCA evidence.

The contract is written so an author outside this repository can build and
verify the tool without reading PGPulse source.

## 2. Non-goals

- PGPulse does not control Patroni (no switchover, restart, pause or reinit via
  the adapter, in either direction).
- The adapter does not replace the built-in `ClusterCollector`. Both run; PGPulse
  keeps both and reconciles (§6).
- No Consul or ZooKeeper specifics in M17. The payload is DCS-neutral
  (`tier: etcd` is used for etcd; other DCS types are an open question).
- No Patroni log shipping. The tool may *derive* events from logs, but sends
  events, not log lines.
- No Kubernetes-operator-specific objects (Patroni on Kubernetes with the
  Kubernetes DCS is an open question).

## 3. What the built-in collector already captures

`internal/collector/cluster.go` (`ClusterCollector`, registered in the medium
group, 60 s by default) with providers in `internal/cluster/patroni` and
`internal/cluster/etcd`:

| Metric key | Labels | Source | Notes |
|---|---|---|---|
| `cluster.patroni.member_state` | `member`, `role` | `GET /cluster` or `patronictl list` | `1` only when state is exactly `running`, else `0`. |
| `cluster.patroni.member_lag_bytes` | `member`, `role` | same | |
| `cluster.patroni.member_count` | — | same | |
| `cluster.patroni.leader_count` | — | same | Counts roles `leader`/`Leader`. |
| `cluster.patroni.replica_count` | — | same | Everything that is not leader. |
| `cluster.etcd.member_count` | — | etcd member list | |
| `cluster.etcd.leader_count` | — | same | |
| `cluster.etcd.member_healthy` | `member` (endpoint) | etcd endpoint health | |

Limits of the built-in collection that motivate the adapter:

1. **Resolution.** One snapshot per 60 s. A failover typically completes in
   10–40 s, so the collector sees "before" and "after", never the sequence.
2. **No history stored.** `PatroniProvider.GetHistory` exists but the collector
   never calls it. The REST parser for `/history` also maps fields incorrectly:
   Patroni returns `[timeline, lsn, reason, timestamp, new_leader]`; the code
   assigns the *timestamp* (index 3) to `FromNode` when 5 elements are present,
   and reads index 4 as `ToNode` only when there are 6. This should be fixed
   separately; this spec does not depend on it.
3. **State mapping.** Patroni 3.x reports streaming replicas with state
   `streaming`, not `running`, so healthy replicas show `member_state = 0`.
   Roles such as `standby_leader` and `sync_standby` are counted as replicas
   without distinction.
4. **Lag type.** Some Patroni versions report `lag` as the string `unknown` for a
   member whose lag cannot be computed; the REST provider unmarshals `lag` as
   `int64`, which would fail the whole `/cluster` response. To verify against
   Patroni 3.x and 4.x.
5. **Per-instance duplication.** Patroni and etcd settings are on each
   `InstanceConfig`, so a 3-member cluster configured on 3 instances is polled 3
   times and stored under 3 `instance_id`s.
6. **No DCS latency, no HA-loop timing, no pause/maintenance state, no
   scheduled switchover, no dynamic-config change, no `pending_restart`, no tags
   (`nofailover`, `noloadbalance`).**
7. **No RCA use.** No node in `internal/rca/chains.go` references any
   `cluster.patroni.*` or `cluster.etcd.*` key.

Items 2–4 are bugs or gaps in collected data and are tracked separately. The
adapter covers items 1 and 6; RCA integration (item 7) is defined in §7 for both
sources.

## 4. What the external tool adds

| Capability | Why polling cannot do it |
|---|---|
| Per-member state and role transitions with sub-second timestamps | Needs continuous watch of each member's `GET /patroni` (or DCS watch on member keys). |
| DCS latency per member and operation | Measured from where Patroni runs; PGPulse polls from elsewhere. |
| Leader-key TTL margin | Requires watching the leader key's `modifyIndex`/update time against `ttl`. |
| Failover / switchover timeline, phase by phase | Needs the sequence: leader lost → candidates → promote → new timeline → replicas reattached. |
| Cluster pause / maintenance mode, scheduled switchover | Present in `/cluster` and `/patroni`, not collected. |
| Dynamic configuration changes | DCS `config` key changes (e.g. `ttl`, `loop_wait`, `maximum_lag_on_failover`, `synchronous_mode`, PostgreSQL parameters). |
| Patroni HA-loop errors | e.g. "Error communicating with DCS", demotion because the leader key could not be renewed; usually only visible in Patroni logs. |

## 5. API contract

The tool uses the tier-adapter native envelope (`schema: pgpulse.ingest/v1`) over
`POST /api/v1/ingest`, or the file drop. Nothing Patroni-specific is added to the
server API.

### 5.1 Registration and token scope

Created by a user with `instance_management` (`super_admin` or `dba`):

```json
{
  "id": "patroni-watch",
  "description": "External Patroni monitor, dc1",
  "scope": {
    "clusters": ["orders-prod"],
    "tiers": ["patroni", "etcd", "time"],
    "item_types": ["event", "sample", "heartbeat"],
    "kind_prefixes": ["patroni.", "etcd.", "time."]
  }
}
```

`time` is included so the tool can report NTP offsets of the Patroni hosts it
already runs on; it is optional.

### 5.2 Identity rules

- `cluster_id` is the PGPulse cluster id. Every item also carries
  `attributes.patroni_scope` (the Patroni `scope`). If PGPulse has collected a
  `ClusterState.ClusterName` for that cluster and it differs, the item is
  rejected with `scope_mismatch`. This prevents a misconfigured tool from writing
  one cluster's failovers into another's timeline.
- `member` is the **Patroni member name** (`name` in `patroni.yml`), not the
  hostname. PGPulse resolves it to an instance through the cluster's member map
  (cluster-entity spec); unresolved members are kept as labels.
- `source` is `patroni` for facts read from Patroni, `etcd` for facts read from
  etcd, `patroni-watch` for facts the tool derived (failover phases).

### 5.3 Events

All events: `tier: patroni` unless stated. `severity` given is the required
default; the tool may raise but not lower it.

| Kind | Type | Severity | Required attributes | Optional attributes |
|---|---|---|---|---|
| `patroni.member.state_changed` | point | info; warning if `to_state` ∉ {`running`, `streaming`} | `patroni_scope`, `from_state`, `to_state`, `role` | `timeline`, `lsn`, `patroni_version` |
| `patroni.member.role_changed` | point | warning | `patroni_scope`, `from_role`, `to_role` | `timeline` |
| `patroni.member.unreachable` | interval | warning | `patroni_scope`, `api_url`, `error` | `consecutive_failures` |
| `patroni.member.pending_restart` | interval | info | `patroni_scope` | `parameters` (comma-separated) |
| `patroni.member.tags_changed` | point | info | `patroni_scope`, `tags_before`, `tags_after` (JSON-encoded strings) | |
| `patroni.leader.lost` | point | critical | `patroni_scope`, `previous_leader`, `cause` (`ttl_expired` \| `demoted` \| `deleted` \| `unknown`) | `ttl_s`, `last_renewed_at` |
| `patroni.leader.acquired` | point | warning | `patroni_scope`, `new_leader`, `timeline` | |
| `patroni.failover` | interval | critical | `patroni_scope`, `kind` (`failover` \| `switchover` \| `manual_failover`), `old_leader`, `new_leader`, `timeline_from`, `timeline_to` | `reason`, `candidates`, `lag_on_promote_bytes`, `phases_ms` (JSON-encoded) |
| `patroni.failover.phase` | point | info | `patroni_scope`, `phase` (§5.5), `timeline_to` | `member`, `detail` |
| `patroni.switchover.scheduled` | point | info | `patroni_scope`, `scheduled_at`, `from`, `to` | |
| `patroni.cluster.paused` / `patroni.cluster.resumed` | point | warning / info | `patroni_scope` | `by` |
| `patroni.config.changed` | point | warning | `patroni_scope`, `changed_keys` (comma-separated) | `before`, `after` (JSON-encoded, ≤ 1024 chars each) |
| `patroni.ha_loop.error` | point | warning | `patroni_scope`, `error_class` (`dcs_unreachable` \| `leader_key_update_failed` \| `postgres_unreachable` \| `other`) | `message` (≤ 512) |
| `patroni.sync_standby.changed` | point | warning | `patroni_scope`, `from`, `to` | `synchronous_mode` |
| `etcd.leader.changed` (tier `etcd`) | point | warning | `patroni_scope`, `from_member`, `to_member`, `raft_term` | |
| `etcd.member.unhealthy` (tier `etcd`) | interval | critical | `patroni_scope`, `endpoint`, `error` | |
| `etcd.quorum.lost` (tier `etcd`) | interval | critical | `patroni_scope`, `healthy`, `total` | |

**Event `id` construction** (required, deterministic, so restarts of the tool do
not create duplicates):

| Kind | `id` |
|---|---|
| state/role/tags changes | `pw:<scope>:<member>:<kind-suffix>:<observed_at as unix ms>` |
| `patroni.failover` | `pw:<scope>:failover:tl<timeline_to>` |
| `patroni.failover.phase` | `pw:<scope>:failover:tl<timeline_to>:<phase>` |
| `patroni.leader.lost` | `pw:<scope>:leader-lost:tl<timeline at loss>` |
| `patroni.leader.acquired` | `pw:<scope>:leader-acquired:tl<timeline>` |
| interval events | `pw:<scope>:<member or endpoint>:<kind-suffix>:<started_at unix ms>` |
| `patroni.config.changed` | `pw:<scope>:config:<DCS modify index>` |

The Patroni timeline number is the natural unique key for a promotion; using it
makes failover events idempotent even if two tool replicas observe the same
failover.

### 5.4 Samples

Stored as `ext.<tier>.<name>` (the server adds the prefix). Recommended cadence
in parentheses.

| Name (tier) | Labels | Unit | Meaning |
|---|---|---|---|
| `dcs_latency_ms` (patroni) | `member`, `op` (`get_cluster` \| `update_leader` \| `touch_member` \| `watch`) | ms | Round trip from the member host to the DCS for the operation (5–10 s; p50/p99 over the interval as two samples with label `stat`). |
| `leader_ttl_margin_seconds` (patroni) | — | s | `ttl - (now - last leader key renewal)`. Approaching 0 means the leader is about to lose the key (5 s). |
| `ha_loop_duration_ms` (patroni) | `member` | ms | Duration of the last HA loop if the tool can observe it (optional). |
| `member_up` (patroni) | `member`, `role` | 0/1 | Patroni API reachable and member `running` or `streaming` (10 s). |
| `member_lag_bytes` (patroni) | `member` | bytes | Same meaning as the collected key, finer resolution (10 s). |
| `timeline` (patroni) | `member` | — | Current timeline per member (on change and every 60 s). |
| `request_latency_ms` (etcd) | `endpoint`, `stat` | ms | Client-observed etcd request latency (5–10 s). |
| `wal_fsync_p99_ms` (etcd) | `endpoint` | ms | From etcd's own `/metrics` (`etcd_disk_wal_fsync_duration_seconds`), if scraped (15 s). |
| `db_size_bytes` (etcd) | `endpoint` | bytes | (60 s). |
| `ntp.offset_seconds` (time) | `member` | s | Optional; NTP offset of each Patroni host (60 s). |

### 5.5 Failover timeline phases

One `patroni.failover` interval event per promotion, plus one
`patroni.failover.phase` point event per phase, all with
`correlation_id = pw:<scope>:failover:tl<timeline_to>`:

| Phase | Observed when |
|---|---|
| `leader_unhealthy` | First sign: leader's API unreachable, PostgreSQL down on leader, or leader TTL margin < `loop_wait`. Optional. |
| `leader_key_lost` | Leader key expires or is deleted (or switchover initiated). `patroni.failover.started_at` = this time (or `leader_unhealthy` if present). |
| `election` | First candidate attempts to acquire the leader key. |
| `promote_started` | Winning member's role becomes `leader`/`promoted` or its Patroni state shows promotion. |
| `new_timeline` | Winning member reports `timeline_to`. |
| `accepting_writes` | New leader reports `running` with role `leader` and is not in recovery. |
| `replicas_attached` | Every remaining non-`nofailover`-excluded member reports `streaming` on `timeline_to`. `patroni.failover.ended_at` = this time. |
| `old_leader_rejoined` | Old leader is back as a replica (may be much later or never). Does not close the interval. |

`phases_ms` on the summary event is a JSON-encoded object of each phase's offset
from `started_at`, e.g. `{"leader_key_lost":0,"election":2140,"promote_started":2380,"new_timeline":3910,"accepting_writes":4420,"replicas_attached":17800}`.

If the tool starts or reconnects mid-failover, it sends the phases it saw and
sets `attributes.partial = true` on the summary event.

### 5.6 Heartbeat

Every 60 s: `{"type":"heartbeat","cluster_id":"orders-prod","tier":"patroni","source":"patroni-watch","ts":"…","tiers":["patroni","etcd"],"version":"patroni-watch 1.4.0"}`.
Without heartbeats PGPulse treats missing Patroni events as *no coverage* rather
than *no failover* (tier-adapter §9.3).

### 5.7 Example: a failover as sent

```json
{
  "schema": "pgpulse.ingest/v1",
  "adapter": "patroni-watch",
  "sent_at": "2026-11-04T10:41:33Z",
  "items": [
    {
      "type": "event", "id": "pw:orders:leader-lost:tl11",
      "cluster_id": "orders-prod", "tier": "patroni", "source": "patroni-watch",
      "member": "orders-db-01", "kind": "patroni.leader.lost", "severity": "critical",
      "observed_at": "2026-11-04T10:41:12.204Z",
      "summary": "Leader key for orders expired; orders-db-01 did not renew within TTL 30s",
      "attributes": { "patroni_scope": "orders", "previous_leader": "orders-db-01", "cause": "ttl_expired", "ttl_s": 30, "last_renewed_at": "2026-11-04T10:40:42.190Z" }
    },
    {
      "type": "event", "id": "pw:orders:failover:tl12",
      "cluster_id": "orders-prod", "tier": "patroni", "source": "patroni-watch",
      "kind": "patroni.failover", "severity": "critical",
      "started_at": "2026-11-04T10:41:12.204Z", "ended_at": "2026-11-04T10:41:30.004Z",
      "correlation_id": "pw:orders:failover:tl12",
      "summary": "Failover orders-db-01 → orders-db-02, timeline 11 → 12, writes resumed after 4.4s",
      "attributes": {
        "patroni_scope": "orders", "kind": "failover",
        "old_leader": "orders-db-01", "new_leader": "orders-db-02",
        "timeline_from": 11, "timeline_to": 12, "candidates": 2,
        "lag_on_promote_bytes": 0,
        "phases_ms": "{\"leader_key_lost\":0,\"election\":2140,\"promote_started\":2380,\"new_timeline\":3910,\"accepting_writes\":4420,\"replicas_attached\":17800}"
      }
    },
    {
      "type": "event", "id": "pw:orders:failover:tl12:accepting_writes",
      "cluster_id": "orders-prod", "tier": "patroni", "source": "patroni-watch",
      "member": "orders-db-02", "kind": "patroni.failover.phase", "severity": "info",
      "observed_at": "2026-11-04T10:41:16.624Z",
      "correlation_id": "pw:orders:failover:tl12",
      "summary": "orders-db-02 accepting writes on timeline 12",
      "attributes": { "patroni_scope": "orders", "phase": "accepting_writes", "timeline_to": 12 }
    },
    {
      "type": "sample", "cluster_id": "orders-prod", "tier": "patroni", "source": "patroni-watch",
      "member": "orders-db-01", "name": "dcs_latency_ms", "value": 2950, "unit": "ms",
      "ts": "2026-11-04T10:40:50Z", "labels": { "op": "update_leader", "stat": "p99" }
    }
  ]
}
```

### 5.8 Mapping from Patroni and etcd sources

For authors: where each payload field comes from.

| Payload | Patroni / etcd source |
|---|---|
| `member` | `GET /cluster` → `members[].name`; `GET /patroni` → `patroni.name` |
| `attributes.patroni_scope` | `GET /patroni` → `patroni.scope`; `/cluster` → `scope` |
| `from_state` / `to_state` | successive `GET /patroni` → `state` (`running`, `streaming`, `starting`, `stopped`, `crashed`, `restarting`, `creating replica`, …) |
| `from_role` / `to_role` | `GET /patroni` → `role` (`primary`/`master`, `replica`, `standby_leader`); `/cluster` → `members[].role` (`leader`, `replica`, `sync_standby`, `standby_leader`, …) — send the `/cluster` spelling |
| `timeline` | `GET /patroni` → `timeline` |
| `lsn` | `GET /patroni` → `xlog.location` (primary) or `xlog.replayed_location` (replica) |
| `pending_restart` | `GET /patroni` → `pending_restart` |
| paused | `GET /patroni` → `pause`; `/cluster` → `pause` |
| `switchover.scheduled` | `/cluster` → `scheduled_switchover` |
| tags | `/cluster` → `members[].tags` |
| `timeline_from` / `reason` for failover | `GET /history` → `[timeline, lsn, reason, timestamp, new_leader]` (last entry after promotion) |
| leader key, TTL, renewal | DCS: `/<namespace>/<scope>/leader` key (etcd v3: watch with revision; `modify_revision` change = renewal), `config` key → `ttl`, `loop_wait` |
| `config.changed` | DCS `/<namespace>/<scope>/config` key change; `changed_keys` = diff of top-level and `postgresql.parameters` keys |
| `dcs_latency_ms` | Timed client calls from each member host, or Patroni's own metrics endpoint if the deployment exposes one |
| etcd leader, health | etcd `/v3/maintenance/status` (`leader`, `raftTerm`, `dbSize`), `/health` |
| `wal_fsync_p99_ms` | etcd `/metrics` → `etcd_disk_wal_fsync_duration_seconds` histogram |

## 6. Reconciliation with the built-in collector

PGPulse keeps collected and ingested data side by side. Rules:

1. **Storage never merges.** Collected keys stay `cluster.patroni.*`, ingested
   keys are `ext.patroni.*` and `ext_events`. The adapter can never write a
   collected key (tier-adapter §5.2).
2. **Timeline dedupe.** The cluster timeline derives collected transitions from
   changes in `cluster.patroni.member_state`/`role` between snapshots. A collected
   transition and an ingested `patroni.member.*_changed` for the same member and
   target state within one collection interval are shown as **one** entry, timed
   by the ingested event (more precise), with `confirmed_by: collected`.
3. **Disagreement.** If at the same snapshot time the collector reports leader A
   and the adapter's most recent `leader.acquired` names B (and no later event
   explains it), the timeline shows both with a `source_disagreement` marker and
   RCA adds `"patroni sources disagree on leader at <t>"` to
   `Quality.ScopeLimitations`. PGPulse does not pick a winner.
4. **Coverage.** When the adapter's heartbeat stops, the timeline shows a gap
   band for the `patroni` lane; collected data continues to render.
5. **Precedence in RCA.** For a node with both sources (`Provenance = mixed`),
   both keys are in `MetricKeys`; `findStrongestAnomaly` takes the strongest. An
   ingested event at `critical` severity has strength 1.0, so a precisely timed
   adapter event naturally wins over a 60 s-granular collected change.

## 7. RCA nodes and chains

New nodes (layer `ha` for Patroni, `dcs` for etcd):

| Node id | Provenance | Evidence keys |
|---|---|---|
| `dcs_latency` | ingested | `ext.patroni.dcs_latency_ms`, `ext.etcd.request_latency_ms`, events `patroni.ha_loop.error` with `error_class = dcs_unreachable` |
| `etcd_disk_slow` | ingested | `ext.etcd.wal_fsync_p99_ms` |
| `etcd_unhealthy` | mixed | `cluster.etcd.member_healthy`, `cluster.etcd.leader_count`, events `etcd.member.unhealthy`, `etcd.quorum.lost`, `etcd.leader.changed` |
| `leader_ttl_pressure` | ingested | `ext.patroni.leader_ttl_margin_seconds` (low values) |
| `leader_lost` | mixed | `cluster.patroni.leader_count` (drop to 0), events `patroni.leader.lost` |
| `ha_failover` | mixed | `cluster.patroni.member_state`, `ext.patroni.timeline`, events `patroni.failover`, `patroni.leader.acquired` (synthetic key `ext.event.patroni.failover`) — symptom key `symptom.ha_failover` |
| `ha_config_change` | ingested | events `patroni.config.changed`, `patroni.cluster.paused` |

New chains (Tier B):

| Chain id | Path | Edge semantics |
|---|---|---|
| `chain.etcd_disk_dcs_failover` | etcd_disk_slow → dcs_latency → leader_ttl_pressure → leader_lost → ha_failover | BoundedLag; each 0–`ttl` (default 30 s); first two Supporting, rest Required |
| `chain.network_dcs_failover` | network_issue → dcs_latency → leader_lost → ha_failover | Reuses the network node from `tier-adapter.md` |
| `chain.etcd_quorum_failover` | etcd_unhealthy → leader_lost → ha_failover | BoundedLag 0–2×`ttl` |
| `chain.failover_reconnect_storm` | ha_failover → connection_spike → connection_pileup | BoundedLag 0–2 min (client reconnects to new primary) |
| `chain.failover_replica_lag` | ha_failover → wal_receiver_disconnect → replication_lag | BoundedLag 0–5 min |
| `chain.ha_config_change_failover` | ha_config_change → ha_failover | WhileEffective; e.g. `ttl`/`loop_wait` lowered, `maximum_lag_on_failover` changed |

**Clock skew on HA chains.** Failover phases are timestamped by the tool on one
host, PostgreSQL metrics by PGPulse, DCS latency on each member. When
`ext.time.ntp.offset_seconds` for any involved member exceeds 0.5 s, the engine
widens these chains' lag bounds by the offset and records the skew in
`Quality.ScopeLimitations` (tier-adapter §9.2). Phases from a single tool on one
clock keep their relative order regardless.

HA chains are selected by trigger metric (as every chain is, via
`ChainsForTrigger`): either a PGPulse alert on `cluster.patroni.leader_count`, or
the synthetic key `ext.event.patroni.failover` raised automatically when a
`patroni.failover` event arrives (auto-trigger subject to the existing 15-minute
per-metric cooldown in `internal/rca/trigger.go`).

## 8. Data model deltas and migration

No new tables. Everything fits `ext_events`, `metrics`, `ingest_coverage` from
`021_external_ingest.sql` (tier-adapter §7.2).

Additions:

- A seed row in the knowledge base: the nodes and chains in §7, bumping
  `KnowledgeVersion`.
- Registered kind catalogue for `patroni.` and `etcd.` (§5.3), enforced as an
  allow-list for adapters whose scope includes `patroni` or `etcd` (unknown kinds
  under these prefixes are rejected with `unknown_kind`, so typos surface during
  conformance rather than silently never matching an RCA node).
- An optional partial index if timeline queries on failovers are slow:
  `CREATE INDEX … ON ext_events (cluster_id, starts_at DESC) WHERE kind LIKE 'patroni.failover%'`
  (in a later migration, only if measured).

## 9. Conformance checklist

The external author runs these against a PGPulse instance with a test cluster
registered and a token for the adapter. Each check states its pass criterion.
Steps C-01 to C-08 use `?dry_run=true`, so they are safe against production.

| # | Check | How | Pass |
|---|---|---|---|
| C-01 | Token and scope | `GET /api/v1/ingest/whoami` | `adapter = patroni-watch`; tiers include `patroni`, `etcd`; `item_types` include `event`, `sample`, `heartbeat`. |
| C-02 | Envelope | Post an empty `items` array, then one valid heartbeat | First → `400`; second → `accepted: 1`. |
| C-03 | Every kind validates | Post one example of each kind in §5.3 with required attributes | `accepted` = number of kinds; no `rejected`. |
| C-04 | Required attributes enforced | For each kind, drop one required attribute | Each item rejected with `missing_attribute`. |
| C-05 | Scope guard | Send `patroni_scope` that differs from the cluster's collected scope | Rejected with `scope_mismatch`. |
| C-06 | Timestamps | Send a timestamp without offset, and one 10 min in the future | Both rejected (`bad_timestamp`, `future_timestamp`). |
| C-07 | Reserved keys | Send a sample named `cluster.patroni.member_state` | Rejected (`reserved_metric_prefix`). |
| C-08 | Rate behaviour | Burst above `whoami` limits | `429` with `Retry-After`; tool backs off and resends with the same `Idempotency-Key`. |
| C-09 | Idempotency | Send the failover example in §5.7 twice (real, not dry run) | First: `accepted: 4`; second: `duplicate: 3`, plus `accepted: 1` for the sample unless `Idempotency-Key` was reused (then identical replayed response). |
| C-10 | Interval close | Send `patroni.member.unreachable` with only `started_at`, then the same `id` with `ended_at` | Second → `updated: 1`; `GET /clusters/{id}/events` shows one closed interval. |
| C-11 | Failover ids | Run a real switchover on the test cluster | Exactly one `patroni.failover` with `id = pw:<scope>:failover:tl<new>`, phases ordered `leader_key_lost` ≤ `election` ≤ `promote_started` ≤ `new_timeline` ≤ `accepting_writes` ≤ `replicas_attached`; `ended_at` = `replicas_attached` time. |
| C-12 | Tool restart mid-failover | Kill the tool during a switchover, restart within 10 s | No duplicate events; summary has `partial: true` only if phases were missed. |
| C-13 | Member resolution | Check `GET /clusters/{id}/events` | Every event's `member` resolves to an `instance_id` for registered members. |
| C-14 | Timeline reconciliation | Compare cluster timeline after C-11 | One entry per member transition, `confirmed_by: collected` where the collector also saw it; no `source_disagreement`. |
| C-15 | Heartbeat coverage | Stop the tool for 6 min | Timeline shows a `patroni` coverage gap; `GET /adapters` shows the tier as not covered. |
| C-16 | RCA uses evidence | After C-11, `POST /instances/{new-leader}/rca/analyze` with body `{"metric": "ext.event.patroni.failover"}` | Incident timeline includes the ingested failover with `provenance: ingested` and a `ref`; `Quality.UnavailableDeps` does not list `tier:patroni`. |
| C-17 | Clock | Tool host NTP offset < 100 ms during the run | `received_at - sent_at` median < 1 s in `GET /adapters`. |

PGPulse will ship the §5.7 payload and one payload per kind as golden fixtures
for this checklist under `testdata/adapters/patroni/` when the adapter endpoint
is implemented, so C-03 and C-09 can be run by posting files.

## 10. Test plan (PGPulse side)

**Unit:**
- Kind catalogue: each kind's required attributes and severity floor; unknown
  kind under `patroni.`/`etcd.` rejected.
- `scope_mismatch` against collected `ClusterState.ClusterName`, and acceptance
  when nothing has been collected yet.
- Reconciliation: dedupe of collected vs ingested transitions within one
  interval; `source_disagreement` detection; coverage gap rendering.
- RCA: each chain in §7 on recorded fixtures (etcd fsync spike → DCS latency →
  TTL pressure → leader lost → failover; failover → reconnect storm); mixed
  provenance picks the ingested event; clock-skew widening applied.

**Integration (`//go:build integration`):** a Patroni + etcd test cluster
(testcontainers, 3 PostgreSQL members, PG 16 and 17), a reference implementation
of the tool's failover detector built from §5.8, a forced switchover and a forced
failover (stop the leader container); assert the conformance results C-09 to C-16
programmatically. Because Docker is unavailable on the main dev workstation,
this runs in CI only.

**Auth:** inherits the tier-adapter matrix; additionally a token without the
`etcd` tier cannot send `etcd.*` kinds.

## 11. Open questions

1. **Fix the built-in collector first?** The `streaming` state mapping, the
   `lag: "unknown"` risk and the `/history` index bug (§3) make collected Patroni
   data less trustworthy than the adapter's. Fix in M17 before reconciliation is
   built, or as part of it?
2. **Should the built-in collector consume `/history`** and emit
   `patroni.failover` summary events itself (coarse, but available without the
   external tool)? It would make HA chains usable for customers without the
   adapter.
3. **Other DCS.** Consul and ZooKeeper (and the Kubernetes DCS) have different
   latency and health semantics. Separate tiers (`consul`, `zookeeper`) or one
   `dcs` tier with a `dcs_type` attribute? This spec uses `etcd` because that is
   what PGPulse collects today.
4. **Member map ownership.** The Patroni member name → instance mapping needs a
   home in the cluster entity (cluster-entity spec). Until it exists, resolution
   falls back to host matching, which fails when Patroni members are named
   differently from hostnames.
5. **Auto-trigger on ingested events.** Should an incoming `patroni.failover`
   start an RCA run on its own (proposed), or only alerts on collected metrics, as
   today?
6. **Multiple tool replicas.** Running the tool on every member gives better DCS
   latency coverage but duplicate transition observations. Ids in §5.3 make
   failovers idempotent; member transitions keyed by `observed_at` ms are not.
   Require a single active instance (leader-elected via the DCS), or key
   transitions by DCS revision instead of time?
