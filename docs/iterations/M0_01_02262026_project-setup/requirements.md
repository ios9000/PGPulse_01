# M0_01: Project Setup — Requirements

**Milestone:** M0 — Project Setup  
**Iteration:** M0_01_02262026_project-setup  
**Date:** 2026-02-26  
**Goal:** Initialize the PGPulse repository with complete project structure, build tooling, CI/CD pipeline, Docker environment, and shared interfaces.

---

## Acceptance Criteria

### Repository Structure
- [ ] Go module initialized: `github.com/ios9000/PGPulse_01`
- [ ] Full directory tree created per CLAUDE.md specification
- [ ] All placeholder `main.go` files compile
- [ ] `go build ./...` succeeds with zero errors

### Dependencies
- [ ] `go.mod` includes: jackc/pgx v5, go-chi/chi v5, koanf, golang.org/x/crypto (bcrypt)
- [ ] `go.sum` generated via `go mod tidy`
- [ ] No unnecessary dependencies

### Build & Tooling
- [ ] `Makefile` with targets: build, test, lint, docker-build, docker-up, docker-down, clean
- [ ] `.golangci.yml` configured with: errcheck, govet, staticcheck, gosimple, ineffassign, unused
- [ ] `golangci-lint run` passes on initial codebase

### Docker
- [ ] `deploy/docker/Dockerfile` — multi-stage Go build (builder + runtime)
- [ ] `deploy/docker/docker-compose.yml` — pgpulse-server + postgres:16 + timescaledb
- [ ] Docker build succeeds: `docker build -f deploy/docker/Dockerfile -t pgpulse:dev .`

### CI/CD
- [ ] `.github/workflows/ci.yml` — triggered on push to main and PRs
- [ ] CI steps: checkout → setup-go → lint → test → build
- [ ] CI config is syntactically valid

### Shared Interfaces & Version Detection
- [ ] `internal/collector/collector.go` — MetricPoint struct, Collector interface, MetricStore interface, AlertEvaluator interface
- [ ] `internal/version/version.go` — PG version detection via `SHOW server_version_num`, version comparison helpers, version range type
- [ ] `internal/version/gate.go` — SQL template registry keyed by version range
- [ ] Interfaces compile and are importable by other packages

### Configuration
- [ ] `configs/pgpulse.example.yml` — full sample config covering: server address, port, database connection, monitoring targets, collection intervals, alert thresholds, notification channels, logging level

### Documentation
- [ ] `docs/` directory contains all scaffolding docs (already created in planning)
- [ ] `README.md` at project root with: description, quick start, build instructions, architecture overview

---

## Out of Scope for M0
- No actual metric collection (that's M1)
- No API endpoints (that's M2)
- No authentication (that's M3)
- No frontend (that's M5)
- Interfaces are defined but implementations are stubs/placeholders
