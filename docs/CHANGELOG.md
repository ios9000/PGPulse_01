# Changelog

All notable changes to PGPulse will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] — 2026-02-25 — M0: Project Setup

### Added
- Go module `github.com/ios9000/PGPulse_01` with pgx v5.8.0 dependency
- Project directory structure: cmd/, internal/, web/, migrations/, deploy/, docs/
- `cmd/pgpulse-server/main.go` — server entrypoint placeholder
- `cmd/pgpulse-agent/main.go` — agent entrypoint placeholder
- `internal/collector/collector.go` — shared interfaces: MetricPoint, Collector, MetricStore, AlertEvaluator
- `internal/version/version.go` — PG version detection via `SHOW server_version_num`
- `internal/version/gate.go` — version-gated SQL template registry
- `configs/pgpulse.example.yml` — full sample configuration
- `deploy/docker/Dockerfile` — multi-stage Go build
- `deploy/docker/docker-compose.yml` — pgpulse + TimescaleDB
- `.github/workflows/ci.yml` — CI pipeline (lint + test + build)
- `.golangci.yml` — linter configuration
- `Makefile` — build, test, lint, docker targets
- `README.md` — project description and quick start
- `.claude/CLAUDE.md` — Claude Code context file with Agent Teams config
- `.claude/rules/` — code-style, architecture, security, PostgreSQL rules
- `docs/` — development strategy, roadmap, RESTORE_CONTEXT, CHANGELOG, iteration docs

### Notes
- Go auto-upgraded to 1.25.7 (pgx v5.8.0 requires Go ≥ 1.24)
- Agent Teams file creation worked; bash execution broken on Windows (EINVAL temp path)
- Build commands run manually: `go build ./...` and `go vet ./...` both clean

---

*Entries below will be added as milestones are completed:*

## [0.2.0] — M1: Core Collector
### Added
- (pending)

## [0.3.0] — M2: Storage & API
### Added
- (pending)

## [0.4.0] — M3: Auth & Security
### Added
- (pending)

## [0.5.0] — M4: Alerting
### Added
- (pending)

## [0.6.0-mvp] — M5: Web UI (MVP Release)
### Added
- (pending)
