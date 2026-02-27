# Session: 2026-02-26 — M2_01 Config & Orchestrator

## Goal

Make PGPulse a running system — YAML config loading, orchestrator lifecycle, interval-group scheduling, LogStore placeholder. After M2_01, `go run cmd/pgpulse-server/main.go -config configs/pgpulse.yml` starts collecting metrics on schedule.

## Agent Team Configuration

- **Team Lead:** Opus 4.6 (Claude Code Agent Teams)
- **Specialists:** Implementation Agent + QA Agent (2 agents)
- **Mode:** Agent Teams, hybrid workflow (agents create files, dev runs bash)

## What Was Built

### New Packages

| Package | Files | Purpose |
|---------|-------|---------|
| `internal/config/` | config.go, load.go | YAML config via koanf, validation, defaults |
| `internal/orchestrator/` | orchestrator.go, runner.go, group.go, logstore.go | Instance connection, version detection, interval-group scheduling |

### Modified Files

| File | Change |
|------|--------|
| `internal/collector/collector.go` | Added MetricQuery struct |
| `cmd/pgpulse-server/main.go` | Replaced placeholder with real main: config → orchestrator → signal handling |

### New Files

| File | Purpose |
|------|---------|
| `configs/pgpulse.example.yml` | Sample configuration |

### Test Files

| File | Tests |
|------|-------|
| `internal/config/config_test.go` | 7 tests (valid, defaults, missing file, invalid YAML, no instances, empty DSN, enabled=false) |
| `internal/orchestrator/group_test.go` | 4 tests (all success, partial failure, all fail, nil points) |
| `internal/orchestrator/logstore_test.go` | 4 tests (write, write empty, query, close) |
| `internal/orchestrator/orchestrator_test.go` | 1 test (construction) |

## Architecture Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Goroutine per interval group (not per collector) | Fewer goroutines, natural batching, collectors share connection |
| D2 | Three interval groups: high=10s, medium=60s, low=300s | Matches strategy doc frequency groups |
| D3 | InstanceContext refreshed per group cycle | Each group queries pg_is_in_recovery() at its own interval; simpler than shared lock |
| D4 | LogStore placeholder for MetricStore | Lets full pipeline run before real PG storage exists |
| D5 | YAML config only (no DB inventory) | Sufficient for MVP; DB-backed inventory adds auth questions (M3) |
| D6 | Skip cycle on connection error (no auto-reconnect) | Simplified resilience for M2_01; reconnect logic in M2_02+ |
| D7 | Explicit collector assignment in buildCollectors() | Matches D9 (no init() magic); build fails if constructor missing |
| D8 | koanf with env var overrides (PGPULSE_ prefix) | Standard pattern for containerized deployment |
| D9 | Enabled field as *bool | Distinguishes "not set" (default true) from "explicitly false" |

## Dependencies Added

- `github.com/knadh/koanf/v2`
- `github.com/knadh/koanf/parsers/yaml`
- `github.com/knadh/koanf/providers/file`
- `github.com/knadh/koanf/providers/env`

## Build Verification

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| Config tests (7) | ✅ All pass |
| Orchestrator tests (9) | ✅ All pass |
| Full suite (16) | ✅ All pass |

## Not Done / Next Iteration

- [ ] PG-backed MetricStore (TimescaleDB-ready) → M2_02
- [ ] REST API endpoints → M2_03
- [ ] Auto-reconnect on connection loss → M2_02+
- [ ] Dynamic instance add/remove → M3+
