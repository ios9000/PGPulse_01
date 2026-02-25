# PostgreSQL-Specific Rules

## Version Handling
- Detect PG version ONCE on first connection, cache in memory
- Use internal/version/ package for version gating
- Each collector registers SQL variants per version range
- Minimum supported version: PostgreSQL 14
- Target compatibility: PG 14, 15, 16, 17, 18

## SQL Conventions
- All queries: parameterized args ($1, $2 or named args)
- statement_timeout per category:
  - Live dashboard queries: 5s (matches PGAM)
  - Per-DB analysis queries: 60s (reduced from PGAM's 600s)
  - Background collection: 30s
- application_name = 'pgpulse_<collector>' on every connection
- Single persistent connection per monitored instance

## Version-Specific Differences

### pg_stat_statements
- PG ≤ 12: total_time column
- PG ≥ 13: total_exec_time + total_plan_time columns
- PG ≥ 14: pg_stat_statements_info view for reset tracking

### WAL Functions (critical for replication collector)
- PG < 10: pg_xlog_location_diff(), pg_current_xlog_insert_location()
- PG ≥ 10: pg_wal_lsn_diff(), pg_current_wal_insert_lsn()

### Removed Functions
- PG ≥ 15: pg_is_in_backup() removed — do not use

### New System Catalogs
- PG ≥ 15: pg_stat_activity.query_id added natively
- PG ≥ 16: pg_stat_io view added (new collector opportunity)

### Replication Views
- PG < 10: pg_stat_replication uses location columns
- PG ≥ 10: pg_stat_replication uses lsn columns
- PG ≥ 14: pg_stat_replication_slots added

## PGAM Legacy Query Reference
When porting queries from PGAM, always check:
1. Does the query use deprecated functions? → Add version gate
2. Does the query use COPY TO PROGRAM? → Replace with agent-based collection
3. Does the query use string concatenation for params? → Rewrite with pgx args
4. Does the query hardcode VTB-specific paths? → Remove or make configurable

## Testing
- Integration tests use testcontainers-go with real PostgreSQL
- Test matrix: PG 14, 15, 16, 17
- Each collector must have tests verifying SQL executes without error
- Mock tests for version-gating logic
- Integration tests tagged with //go:build integration
