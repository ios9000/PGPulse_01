# PGPulse — Iteration Handoff: M10_01 → M11

**Date:** 2026-03-16
**From:** M10_01 (Advisor Auto-Population)
**To:** M11 (Competitive Enrichment)

---

## DO NOT RE-DISCUSS

All items from REM_01 and M9_01 handoffs remain in force, plus:

### Background Evaluator — IMPLEMENTED (M10_01)

- **`internal/remediation/background.go`** — `BackgroundEvaluator` runs `Engine.Diagnose()` on ticker
- **Config section:** `remediation.enabled` (bool), `remediation.background_interval` (duration, default 5m), `remediation.retention_days` (int, default 30)
- **Wiring guard:** `cfg.Remediation.Enabled && persistentStore != nil` — never runs in live/MemoryStore mode
- **Instance listing:** uses `ml.NewDBInstanceLister(pgPool)`
- **Status lifecycle:** `active` → `resolved` (next cycle doesn't find it) or `acknowledged` (DBA dismisses)
- **Upsert logic:** existing active recommendation updated with new `evaluated_at`; resolved recommendations reactivated if rule fires again
- **`ResolveStale()`** marks active recommendations as resolved when not in current eval cycle
- **Migration 014:** `status`, `evaluated_at`, `resolved_at` columns + indexes on `remediation_recommendations`
- **NullStore:** all new methods are no-op (live mode safe)

### Frontend — Advisor Enhancements

- **Auto-refresh:** 30s refetch interval via React Query
- **"Last evaluated: X ago"** display with relative timestamp
- **Status filter:** Active / Resolved / Acknowledged / All (default: Active)
- **"Create Alert Rule" button** per recommendation row — opens `RuleFormModal` pre-filled with metric key, threshold, severity mapping; permission-gated to dba+
- **Sidebar badge:** red pill on "Advisor" nav item showing active recommendation count; hidden when 0

### API Enhancements

- `?status=active|resolved|acknowledged` query param on `GET /recommendations` and `GET /instances/{id}/recommendations`
- Response includes `status`, `evaluated_at`, `resolved_at`, `metric_key`, `metric_value`

### Config Path (Demo VM)

- `/opt/pgpulse/configs/pgpulse.yml` — NOT `/opt/pgpulse/etc/`
- `remediation:` section added with `enabled: true`, `background_interval: 5m`, `retention_days: 30`

---

## What Was Just Completed

### M9_01 — Alert & Advisor Polish (2 sessions + 1 fix)
- 12 alert rule metric key fixes in `rules.go`
- Alerts tab bar (Active | History | Rules) + sidebar expandable Alerts group
- DSN port parser fix (keyword/value format)
- Diagnose panel metric value population (MetricKey/MetricValue on RuleResult)
- SessionKillButtons restored on LongTransactionsTable with "Show actions" override

### M10_01 — Advisor Auto-Population (1 session)
- 3 new files, ~15 modified
- BackgroundEvaluator worker with configurable interval + retention
- Migration 014 (status columns)
- PGStore upsert + ResolveStale + status filter
- Config section (remediation.enabled/background_interval/retention_days)
- Frontend: auto-refresh, "Create Alert Rule" button, sidebar badge, status filters

---

## Demo Environment

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989     (persistent mode)
Login:          admin / pgpulse_admin
Config:         /opt/pgpulse/configs/pgpulse.yml

PostgreSQL 16.13:
  Primary:      localhost:5432
  Replica:      localhost:5433
  Chaos:        localhost:5434

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
Storage DB:     pgpulse_storage on port 5432

Chaos scripts:  /opt/pgpulse/chaos/*.sh (PGPASSWORD embedded)
```

---

## Known Issues (Post M10_01)

| Issue | Severity | Notes |
|-------|----------|-------|
| `wastedibytes` float64→int64 bloat scan | Pre-existing | DatabaseCollector bloat query returns float, scanner expects int |
| `c.command_desc` SQL bug in cluster progress | Pre-existing | PG16 compatibility |
| `pg.server.multixact_pct` — no collector | Minor | Alert rules exist but no collector emits this metric; add to ServerInfoCollector |
| `pg_stat_statements` not in shared_preload_libraries (some instances) | Expected | WARN logged, graceful degradation |
| `pg_largeobject` permission denied | Expected | Monitor user lacks access; sub-collector skips |
| `srsubstate` char scan error in logical replication | Minor | Binary format scan issue for char(1) column |
| Debug log in main.go (`"remediation config"`) | Cosmetic | Added during troubleshooting; remove in next iteration |

---

## Key Interfaces (Current)

```go
// internal/remediation/background.go
type BackgroundEvaluator struct { ... }
func NewBackgroundEvaluator(engine, store, metricSource, lister, interval, retentionDays, logger) *BackgroundEvaluator
func (b *BackgroundEvaluator) Start(ctx context.Context)
func (b *BackgroundEvaluator) Stop()

// internal/remediation/store.go — updated interface
type RecommendationStore interface {
    Write(ctx, recs) ([]Recommendation, error)
    ListByInstance(ctx, instanceID, opts) ([]Recommendation, int, error)
    ListAll(ctx, opts) ([]Recommendation, int, error)
    ListByAlertEvent(ctx, alertEventID) ([]Recommendation, error)
    Acknowledge(ctx, id, username) error
    CleanOld(ctx, olderThan) error
    ResolveStale(ctx, instanceID, currentRuleIDs) error  // NEW
}

// internal/config/config.go
type RemediationConfig struct {
    Enabled            bool          `koanf:"enabled"`
    BackgroundInterval time.Duration `koanf:"background_interval"`
    RetentionDays      int           `koanf:"retention_days"`
}
```

---

## API Endpoints (62 total, 5 with new behavior)

| Method | Path | Change in M10_01 |
|--------|------|-----------------|
| GET | /recommendations | Added `?status=` filter |
| GET | /instances/{id}/recommendations | Added `?status=` filter |
| PUT | /recommendations/{id}/acknowledge | No change (existing) |
| GET | /recommendations/rules | No change (existing) |
| POST | /instances/{id}/diagnose | No change (existing) |

No new endpoints added — all changes are query parameter enhancements to existing endpoints.

---

## Roadmap: Updated Priorities

### Queue (locked order)

1. ~~Alert & Advisor Polish~~ ✅ **M9 DONE**
2. ~~Advisor Auto-Population~~ ✅ **M10 DONE**
3. **Competitive Enrichment** (M11) ← NEXT
4. **Desktop App (Wails)** (M12)
5. **Prometheus Exporter** (M13)

### M11 — Competitive Enrichment: Scoping Notes

The competitive research synthesis (`PGPulse_Competitive_Research_Synthesis.md`) identified these high-priority gaps:

**Immediate candidates (informed by research):**
- **pg_stat_statements snapshot diffs** — pg_profile paradigm: periodic snapshots of statement stats, diff between timepoints to show "what changed" (top movers in total_time, calls, rows)
- **Query insights page** — pganalyze-style: per-query trend graphs, normalized fingerprints, call/time/rows history
- **Workload snapshot reports** — static HTML post-mortem reports from stored metrics + statement snapshots (pg_profile paradigm)

**Deferred (research confirmed as high-complexity):**
- Index Advisor (pganalyze Indexing Engine — requires constraint programming)
- VACUUM Advisor with Simulator (pganalyze — requires deep autovacuum modeling)
- Query Tuning Workbooks (pganalyze — requires EXPLAIN pipeline + plan diff)
- APM/OTel trace correlation (Datadog — outside PGPulse's DB-native philosophy)

**Key competitive positioning:**
- PGPulse's ML forecasting + single binary + zero-config live mode are unique differentiators — protect and invest
- pg_stat_statements snapshot diffs are the most tractable enrichment with highest DBA impact
- Plan collection via auto_explain integration is a natural next step after query insights

### Milestone Status

| Milestone | Scope | Status |
|-----------|-------|--------|
| ~~MW_01~~ | Windows executable + live mode | ✅ Done |
| ~~MW_01b~~ | Bugfixes (5 bugs) | ✅ Done |
| ~~MN_01~~ | Metric naming standardization | ✅ Done |
| ~~REM_01~~ | Rule-based remediation (3 sub-iterations) | ✅ Done |
| ~~M9~~ | Alert & Advisor Polish | ✅ Done |
| ~~M10~~ | Advisor Auto-Population | ✅ Done |
| M11 | Competitive Enrichment (query insights, pganalyze-style) | 🔲 Next |
| M12 | Desktop App (Wails packaging) | 🔲 |
| M13 | Prometheus Exporter | 🔲 |

---

## Build & Deploy

```bash
# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Cross-compile (MINGW64 — use export, not set)
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy to demo VM
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

---

## Project Knowledge Status

| Document | Status |
|----------|--------|
| PGPulse_Development_Strategy_v2.md | ✅ Current |
| PGAM_FEATURE_AUDIT.md | ✅ Current |
| Chat_Transition_Process.md | ✅ Current |
| Save_Point_System.md | ✅ Current |
| PGPulse_Competitive_Research_Synthesis.md | ✅ Current |
| CODEBASE_DIGEST.md | ⚠️ Re-upload after M10_01 digest regeneration |
