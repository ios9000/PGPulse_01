# PGPulse Competitive Research — Synthesis & Strategic Positioning

**Date:** 2026-03-12
**Scope:** M6/M7 coverage — informs metric naming standardization, agent architecture, feature gaps
**Competitors analyzed (deep):** pgwatch v5, Percona PMM v3, pganalyze, Datadog DBM
**Competitors analyzed (focused):** pg_profile, pgpro_pwr

---

## 1. Market Archetypes

The PostgreSQL monitoring landscape divides into five distinct archetypes. PGPulse must understand which space it occupies and which it borrows from.

| Archetype | Representative | Core Philosophy |
|-----------|---------------|-----------------|
| Go-native SQL collector | pgwatch v5 | Metrics are SQL queries; storage is pluggable; visualization is Grafana |
| Open-source full-stack platform | Percona PMM v3 | Multi-DB observability with dedicated QAN pipeline; heavy infra |
| SaaS query intelligence | pganalyze | Opinionated cloud product; deep query analysis chain; deterministic advisors |
| Enterprise SaaS platform | Datadog DBM | DB monitoring as a layer within a unified APM/infra/logs ecosystem |
| In-DB workload reporter | pg_profile / pgpro_pwr | Pure PL/pgSQL; snapshot-and-diff; static HTML reports; no real-time |

**PGPulse's archetype: Hybrid — PostgreSQL-native collector + built-in RCA + optional OS agent + embedded UI + ML forecasting.** This is a new position that doesn't map cleanly to any existing archetype. The closest structural match is pgwatch (SQL-first collection, Go binary) combined with pganalyze's ambition (intelligent analysis) and pg_profile's philosophy (snapshot-based workload history).

---

## 2. Six-Dimension Competitive Matrix

### 2.1 Privilege / Security Model (Blast Radius)

| Competitor | Core Monitoring | Advanced Features | Shell/OS Execution | Blast Radius on Compromise |
|-----------|----------------|-------------------|-------------------|---------------------------|
| **pgwatch v5** | pg_monitor + helper functions (SECURITY DEFINER) | PL/Python helpers for OS stats | Yes (PL/Python psutil) | Medium — helpers can escalate |
| **PMM v3** | pg_monitor to SUPERUSER recommended | QAN needs pg_stat_statements or pg_stat_monitor | node_exporter on host | High — SUPERUSER recommended, host agent |
| **pganalyze** | Non-SUPERUSER with REVOKE ALL; SECURITY DEFINER wrappers | Separate pganalyze_explain role for EXPLAIN ANALYZE; explicitly not recommended for sensitive data | None | Low (core) / Medium (tuning workflow) |
| **Datadog DBM** | pg_monitor + datadog schema | auto_explain via shared_preload_libraries; log-based plans | None directly; Datadog Agent on host | Medium — host agent, data leaves infra |
| **pg_profile** | pg_monitor + dblink | N/A | None | Low — stats-only read access |
| **pgpro_pwr** | Dedicated monitoring role; least-privilege documented | pgpro_stats + pg_wait_sampling (Postgres Pro only) | None | Low — stats-only read access |
| **PGPulse** | pgpulse_monitor with pg_monitor + pg_read_server_files | pg_read_file('/proc/*') for OS metrics | SQL-based file read, no shell | Medium — pg_read_file is broader than stats-only |

**Strategic takeaway:** pganalyze sets the bar for least-privilege design. PGPulse's `pg_read_file('/proc/*')` approach is broader than pure stats access but narrower than PMM's SUPERUSER recommendation. For enterprise positioning, PGPulse should document the blast radius of each `os_metrics_method` mode explicitly, and frame the OS agent (M6) as the least-privilege alternative to SQL-based file reading.

---

### 2.2 Deployment Complexity / Operational Footprint

| Competitor | Components to Run | Persistent Stores | External Dependencies | Cost Model |
|-----------|-------------------|-------------------|----------------------|------------|
| **pgwatch v5** | 1 binary (gatherer) + Grafana | PG/TimescaleDB or Prometheus | pg_stat_statements | Free/OSS |
| **PMM v3** | PMM Server (Docker/VM) + PMM Client per host (pmm-agent, node_exporter, postgres_exporter, vmagent) | VictoriaMetrics + ClickHouse (QAN) + Grafana | pg_stat_statements or pg_stat_monitor | Free/OSS |
| **pganalyze** | 1 collector process | None (SaaS) or Enterprise Server | pg_stat_statements; auto_explain recommended | SaaS subscription per server |
| **Datadog DBM** | 1 Datadog Agent per host (or per ~30 instances) | None (SaaS) | pg_stat_statements; auto_explain recommended | Per-host SaaS pricing |
| **pg_profile** | None (PL/pgSQL extension inside DB) + cron | Repository tables inside DB | pg_stat_statements, dblink | Free/OSS |
| **pgpro_pwr** | None (PL/pgSQL extension inside DB) + cron | Repository tables inside DB | pgpro_stats (Postgres Pro), dblink | Requires Postgres Pro license |
| **PGPulse** | 1 binary (pgpulse-server) | TimescaleDB (persistent) or in-memory (live mode) | pg_stat_statements | Free/OSS |

**Strategic takeaway:** PGPulse's single-binary model with embedded UI is the lightest full-featured deployment. Only pg_profile/pgpro_pwr are lighter, but they lack real-time monitoring, alerting, and UI. The `--target=DSN` live mode is a unique competitive advantage — no other tool offers zero-config monitoring with a single command.

---

### 2.3 Query Analysis Workflow Maturity

| Capability | pgwatch v5 | PMM v3 | pganalyze | Datadog DBM | pg_profile | pgpro_pwr | PGPulse |
|-----------|-----------|--------|-----------|-------------|-----------|-----------|---------|
| Query fingerprinting | Via pg_stat_statements | QAN | Yes | Yes | Yes | Yes | Yes |
| Per-query history/trends | Grafana dashboards | QAN timeseries | Yes (deep) | Yes | Snapshot diffs | Snapshot diffs | Partial |
| EXPLAIN plan collection | No | Via QAN agent | Via auto_explain + collector workflow | Via auto_explain (log-based) | No | No | No |
| On-demand EXPLAIN from UI | No | Yes (QAN) | Yes (Workbooks) | No | No | No | No |
| Plan comparison / diff | No | No | Yes (Workbooks) | No | No | Comparison reports | No |
| Per-plan statistics | No | No | No | No | No | Yes (pgpro_stats) | No |
| Plan instability detection | No | No | Via Query Advisor (deterministic) | No | No | Yes (pgpro_stats) | No |
| Index Advisor | No | No | Yes (Indexing Engine) | No | No | No | No |
| Query rewrite suggestions | No | No | Yes (Query Advisor) | No | No | No | No |
| VACUUM Advisor | No | No | Yes (with Simulator) | No | No | Extended vacuum stats | No |

**Strategic takeaway:** pganalyze dominates query analysis depth with its full chain (fingerprint → plan → diff → advisor → workbook → validate). PMM is strong on the investigation UX (QAN + EXPLAIN). pgpro_pwr has unique per-plan statistics that no one else matches, but requires Postgres Pro. PGPulse should NOT try to replicate the full pganalyze chain immediately. Instead, build toward: (1) pg_stat_statements snapshot diffs (pg_profile paradigm), (2) plan collection via auto_explain integration, (3) ML-based anomaly detection on query metrics (unique differentiator).

---

### 2.4 Stateful History / Diff Capabilities

| Capability | pgwatch v5 | PMM v3 | pganalyze | Datadog DBM | pg_profile | pgpro_pwr | PGPulse |
|-----------|-----------|--------|-----------|-------------|-----------|-----------|---------|
| Workload snapshot diffs | change_events (limited in Prom mode) | No | Schema Statistics | No | Core feature | Core feature (richer) | No (planned) |
| pg_settings history | No | No | Partial (provider APIs) | Config viewer (current only) | Included in samples | Included in samples | Planned |
| Configuration drift detection | No | No | No | No (compare manually) | Comparison reports | Comparison reports | Planned |
| Plan shape diffs | No | No | Yes (Workbooks) | No | No | Comparison reports | No |
| RCA timeline (DB→OS→App) | No | No | Partial (OTel integration) | Yes (APM↔DBM correlation) | No | No | Planned (ML RCA) |
| DDL change tracking | change_events | No | Schema change tracking | No | No | No | No |

**Strategic takeaway:** The snapshot-and-diff paradigm (pg_profile/pgpro_pwr) is the right model for PGPulse's `pg_settings` history and workload comparison features. Datadog's APM↔DBM correlation is the gold standard for cross-layer RCA, but requires a full SaaS ecosystem. PGPulse's ML-based RCA can approach this from a different angle: correlate DB metric anomalies with OS metric anomalies on a shared timeline, without requiring application instrumentation.

---

### 2.5 Managed-Service / Cloud-Friendliness

| Competitor | RDS/Aurora | Azure DB | Google Cloud SQL | Patroni/HA | PgBouncer | Agentless Option |
|-----------|-----------|---------|-----------------|------------|-----------|-----------------|
| **pgwatch v5** | Via presets | Via presets | Limited | Continuous discovery | Built-in type | SQL-only (no host agent) |
| **PMM v3** | Dedicated setup | Dedicated setup | Limited | No auto-discovery | No | No — requires pmm-client |
| **pganalyze** | Best-in-class | Dedicated setup | Dedicated setup | No | No | Collector can run remotely |
| **Datadog DBM** | Quick Install (CloudFormation) | Dedicated setup | Dedicated setup | No | Explicit warning to avoid | Agent per host or remote |
| **pg_profile** | Partial (SQL script install, no extension system) | Same | Same | Multi-server via dblink | N/A | Inherently agentless (in-DB) |
| **pgpro_pwr** | No (Postgres Pro only) | No | No | Multi-server via dblink | N/A | Inherently agentless (in-DB) |
| **PGPulse** | Works (SQL collection) | Works (SQL collection) | Works (SQL collection) | No auto-discovery (planned) | No | SQL-first by default |

**Strategic takeaway:** pganalyze is the benchmark for cloud-native integration. PGPulse's SQL-first collection model works on managed services out of the box (no extensions to install beyond pg_stat_statements), which is a strength. However, OS metrics via `pg_read_file('/proc/*')` won't work on managed services — this is where the M6 OS Agent and cloud API integration become important for the enterprise story.

---

### 2.6 Extensibility Model

| Competitor | Primary Model | Custom SQL Metrics | Custom Dashboards | API | Plugin Ecosystem |
|-----------|--------------|-------------------|-------------------|-----|-----------------|
| **pgwatch v5** | SQL-first | Yes — any SQL query as a metric | Grafana (full) | Internal REST | Metric YAML files |
| **PMM v3** | Exporter-first | Via custom queries in exporter config | Grafana (full) | Limited | Prometheus exporters |
| **pganalyze** | SaaS-first | Not positioned as extensible | Built-in (not customizable) | GraphQL API | No |
| **Datadog DBM** | Platform-first | Yes — custom_queries in Agent config | Full dashboard builder | Extensive REST APIs | 1000+ integrations |
| **pg_profile/pgpro_pwr** | In-DB-first | No | No | No | No |
| **PGPulse** | SQL-first | Yes — any SQL collector | Built-in (React/ECharts) | REST API | Collector registry |

**Strategic takeaway:** PGPulse and pgwatch share the SQL-first extensibility model, which is the most natural for PostgreSQL DBA teams. This is a deliberate competitive advantage over pganalyze (not extensible) and PMM (exporter-based, more complex to extend). PGPulse's collector registry should be documented and promoted as a first-class feature.

---

## 3. Metric Naming Recommendation

### 3.1 What We Learned

Each competitor uses a different naming convention:

| Competitor | Convention | Example | Separator |
|-----------|-----------|---------|-----------|
| pgwatch v5 | Flat snake_case, PG view names | `db_stats`, `table_stats`, `backends` | Underscore (metric name = table name) |
| PMM / Prometheus | `pg_stat_{view}_{column}` | `pg_stat_database_xact_commit` | Underscore, `pg_` prefix |
| pganalyze | Proprietary internal | Not user-facing | N/A |
| Datadog | Dot-delimited hierarchical | `postgresql.connections`, `system.cpu.user` | Dot, integration prefix |
| PGPulse (current) | Dot-delimited hierarchical | `pg.connections.active`, `os.cpu.user` | Dot, layer prefix |

### 3.2 Recommendation: Keep Dot-Notation, Add Prometheus Mapping Layer

**PGPulse's current dot-notation is validated by Datadog's approach** — the largest enterprise monitoring platform uses the same pattern. It is human-readable in PGPulse's built-in UI and naturally hierarchical for grouping and filtering.

**However**, when PGPulse adds the Prometheus exporter (NEW-3), metric names must conform to Prometheus conventions (underscores, `_total` suffix for counters, `_bytes`/`_seconds` suffixes for units). Rather than changing internal names, implement a **mapping layer**:

**Internal (PGPulse storage + UI + API):**
```
pg.connections.active          → gauge
pg.connections.idle            → gauge
pg.database.size_bytes         → gauge
pg.transactions.committed      → counter
pg.bgwriter.checkpoints_timed  → counter
os.cpu.user_percent            → gauge
os.memory.available_bytes      → gauge
os.disk.read_bytes_per_sec     → gauge
```

**Prometheus export (when scraped):**
```
pgpulse_pg_connections{state="active"}
pgpulse_pg_connections{state="idle"}
pgpulse_pg_database_size_bytes{datname="mydb"}
pgpulse_pg_transactions_committed_total
pgpulse_pg_bgwriter_checkpoints_timed_total
pgpulse_os_cpu_user_percent
pgpulse_os_memory_available_bytes
pgpulse_os_disk_read_bytes_per_second
```

### 3.3 Naming Rules (Proposed Standard)

1. **Top-level prefix = layer:** `pg.` for PostgreSQL metrics, `os.` for OS metrics, `pgpulse.` for internal/meta metrics
2. **Second level = category:** `connections`, `database`, `transactions`, `replication`, `bgwriter`, `statements`, `locks`, `tables`, `indexes`, `vacuum`, `wal`; for OS: `cpu`, `memory`, `disk`, `network`, `load`
3. **Third level = specific metric:** descriptive, snake_case within level
4. **Units in name when ambiguous:** `_bytes`, `_percent`, `_per_sec`, `_seconds`, `_count`
5. **Labels/tags for dimensions:** `{instance="...", database="...", table="...", state="..."}`
6. **Counter vs. gauge:** document explicitly per metric (required for Prometheus mapping)

### 3.4 Existing Key Renames Needed

Based on the current codebase (from CODEBASE_DIGEST), these known mismatches should be fixed in the naming standardization iteration:

| Current Key | Proposed Key | Reason |
|------------|-------------|--------|
| `os.diskstat.read_kb` | `os.disk.read_bytes_per_sec` | Align with hierarchical naming + standard units |
| `os.diskstat.write_kb` | `os.disk.write_bytes_per_sec` | Same |
| `os.disk.read_bytes_per_sec` (old) | Remove | Was renamed but references may linger |
| Cache hit ratio (mismatched) | `pg.cache.hit_ratio` | Pre-existing mismatch flagged in known issues |

**Note:** The full rename inventory should be generated from the codebase digest's "Metric Keys" section during the dedicated naming standardization iteration.

---

## 4. Feature Gap Analysis — PGPulse Roadmap Implications

### 4.1 Features PGPulse Should Build (High Priority)

| Feature | Inspired By | Why It Matters | Suggested Milestone |
|---------|------------|----------------|-------------------|
| **Workload snapshot reports** (pg_stat_statements diffs between time windows) | pg_profile / pgpro_pwr | DBAs coming from Oracle expect AWR-like post-mortem reports; fills the "complementary diagnostic" gap | Post M6 |
| **pg_settings history + drift detection** | pg_profile (comparison reports), Datadog (config viewer) | Configuration drift is a common RCA signal; no competitor does this well as real-time | M8/M9 series |
| **Prometheus exporter endpoint** | PMM, pgwatch, Datadog | Interop with existing Grafana/Prometheus infrastructure; NEW-3 on roadmap | After naming standardization |
| **auto_explain integration** (plan collection) | pganalyze, Datadog | Foundation for plan-level analysis; passive collection, no EXPLAIN execution | M8/M9 series |
| **Recommendations / best-practice checks** | pgwatch (reco_*), PMM (Advisors), pganalyze (Alerts & Check-Up) | Rule-based checks for common issues (unused indexes, bloat, wraparound risk) | Post M6 |

### 4.2 Features PGPulse Should Study But Defer

| Feature | Best Reference | Why Defer |
|---------|---------------|-----------|
| Index Advisor | pganalyze (Indexing Engine) | Requires constraint programming model; high complexity, low ROI until query analysis chain is mature |
| VACUUM Advisor with Simulator | pganalyze | Requires deep autovacuum modeling; valuable but complex |
| Query Tuning Workbooks | pganalyze | Requires EXPLAIN execution pipeline + plan diff UI; prerequisite features not built yet |
| APM / OTel trace correlation | Datadog, pganalyze | Requires application-side instrumentation; valuable but outside PGPulse's core DB-native philosophy |
| Patroni / PgBouncer auto-discovery | pgwatch, PMM | Useful for enterprise but not a differentiator; implement when fleet management matures |

### 4.3 Features That Are PGPulse's Unique Differentiators (Protect & Invest)

| Feature | Competitor Coverage | PGPulse Advantage |
|---------|-------------------|-------------------|
| **ML anomaly detection + STL forecasting** | None (PMM: none; pganalyze: deterministic advisors; Datadog: platform-level anomaly monitors only) | PGPulse is the only PostgreSQL monitoring tool with built-in ML forecasting and confidence bands |
| **Single binary + embedded UI + zero-config live mode** | None | `pgpulse-server.exe --target=DSN` is unmatched in simplicity |
| **Agentless OS metrics via pg_read_file** | None (pgwatch uses PL/Python; PMM uses node_exporter; pganalyze uses cloud APIs) | Works without any host-side installation on self-hosted PG |
| **Three-mode OS collection** (SQL/agent/disabled per instance) | None | Enterprise-grade flexibility; lets ops team choose per-instance risk profile |
| **Forecast-threshold alerting with sustained-crossing logic** | None | Goes beyond simple threshold alerts without requiring full anomaly detection platform |

---

## 5. PGPulse Positioning Statement (Draft)

**For DBA teams managing PostgreSQL fleets** who need real-time monitoring with deep diagnostic capabilities,

**PGPulse** is a PostgreSQL-native monitoring platform

**that** combines SQL-first metric collection, ML-based anomaly detection, and built-in workload analysis

**unlike** pgwatch (no alerting, no ML, no built-in UI), PMM (heavy deployment, no ML, SUPERUSER recommended), pganalyze (SaaS-only, expensive per-server, not self-hosted-first), and Datadog (data leaves your infrastructure, per-host SaaS cost),

**PGPulse** deploys as a single binary, runs on vanilla PostgreSQL, and provides real-time dashboards, intelligent alerting, and ML forecasting without requiring external infrastructure, cloud subscriptions, or elevated database privileges.

---

## 6. Updated Roadmap Priorities (Post-Research)

Confirmed locked order from handoff, with research-informed annotations:

1. ~~Windows executable~~ — **DONE (MW_01)**
2. **ML/DL remediation** (rule-based approach first) — informed by pganalyze's deterministic advisor model and pgwatch's reco_* pattern
3. **Prometheus exporter** — requires metric naming standardization first; naming recommendation in §3 above
4. **Workload snapshot reports** (NEW, research-driven) — pg_profile paradigm applied to PGPulse's time-series data; generates static HTML post-mortem reports from stored metrics + pg_stat_statements snapshots
5. **pg_settings history + configuration drift** — informed by pg_profile's comparison reports and Datadog's config viewer

**Metric naming standardization** is now unblocked. Recommendation: dot-notation with Prometheus mapping layer (§3.2). Execute in a dedicated iteration before the Prometheus exporter work.

---

## Appendix: Competitor Quick Reference

### pgwatch v5
- **Repo:** github.com/cybertec-postgresql/pgwatch (pgwatch2 archived)
- **Language:** Go
- **Metric naming:** Flat snake_case, metric name = storage table name
- **Key strength:** SQL-first extensibility, presets, Prometheus/PG/TimescaleDB storage options
- **Key weakness:** No built-in alerting, no ML, no built-in UI (Grafana-dependent)

### Percona PMM v3
- **Docs:** docs.percona.com/percona-monitoring-and-management/3/
- **Architecture:** PMM Server + PMM Client (pmm-agent, exporters, vmagent)
- **Metric naming:** Prometheus standard (`pg_stat_database_xact_commit`)
- **Key strength:** QAN (query analytics with EXPLAIN), multi-DB support
- **Key weakness:** Heavy deployment, SUPERUSER recommended, no ML

### pganalyze
- **Docs:** pganalyze.com/docs
- **Architecture:** Open-source Go collector + proprietary SaaS/Enterprise Server backend
- **Key strength:** Query analysis chain (Advisor + Workbooks + Index Engine), least-privilege design
- **Key weakness:** SaaS cost, not SQL-extensible, no ML (deterministic planner-aware analysis)

### Datadog DBM
- **Docs:** docs.datadoghq.com/database_monitoring/
- **Architecture:** Datadog Agent (open-source Go) + Datadog SaaS
- **Metric naming:** Dot-notation (`postgresql.connections`, `system.cpu.user`)
- **Key strength:** APM↔DBM correlation, unified platform, 1000+ integrations
- **Key weakness:** Data leaves infrastructure, per-host SaaS cost, no Index/VACUUM advisor

### pg_profile
- **Repo:** github.com/zubkov-andrei/pg_profile
- **Architecture:** Pure PL/pgSQL extension inside PostgreSQL
- **Key strength:** AWR-style snapshot-and-diff reports, zero external dependencies
- **Key weakness:** Batch reports only, no real-time, no UI, no alerting, no OS metrics

### pgpro_pwr
- **Docs:** postgrespro.com/docs/postgrespro/current/pgpro-pwr
- **Architecture:** Pure PL/pgSQL extension inside Postgres Pro
- **Key strength:** Per-plan statistics, plan instability detection, load distribution charts, session state sampling — analytically the deepest in the field
- **Key weakness:** Requires Postgres Pro (not vanilla PG), batch reports only, no real-time, no UI, no alerting
- **Relationship to pg_profile:** Same author (Andrey Zubkov); pg_profile is the vanilla PG version, pgpro_pwr is the Postgres Pro fork with exclusive features powered by pgpro_stats
