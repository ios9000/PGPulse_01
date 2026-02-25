# M0_01: Project Setup — Agent Team Prompt

> **Usage:** Paste everything below the line into Claude Code to start the Agent Team session.
> **Pre-requisites:** Repository must be initialized with `git init` and `.claude/` directory present.

---

Initialize the PGPulse project. Read .claude/CLAUDE.md for full context.
This is a PostgreSQL monitoring tool being rewritten from legacy PHP (PGAM) to Go.

Create a team of 2 specialists:

SPECIALIST 1 — SCAFFOLD:
Your job is project structure, build tooling, and deployment config.

1. Initialize Go module: github.com/ios9000/PGPulse_01
2. Create the full directory structure:
   - cmd/pgpulse-server/main.go (placeholder with version print)
   - cmd/pgpulse-agent/main.go (placeholder with version print)
   - internal/storage/.gitkeep
   - internal/api/.gitkeep
   - internal/auth/.gitkeep
   - internal/alert/notifier/.gitkeep
   - internal/config/.gitkeep
   - internal/ml/.gitkeep
   - internal/rca/.gitkeep
   - web/.gitkeep
   - migrations/.gitkeep
   - deploy/helm/.gitkeep
   - deploy/systemd/.gitkeep
3. Create Makefile with targets:
   - build: go build ./cmd/pgpulse-server/ and ./cmd/pgpulse-agent/
   - test: go test -race ./...
   - lint: golangci-lint run
   - docker-build: docker build -f deploy/docker/Dockerfile -t pgpulse:dev .
   - docker-up: docker-compose -f deploy/docker/docker-compose.yml up -d
   - docker-down: docker-compose -f deploy/docker/docker-compose.yml down
   - clean: remove build artifacts
4. Create deploy/docker/Dockerfile:
   - Builder stage: golang:1.23-alpine, copy source, go build
   - Runtime stage: alpine:3.19, copy binary, expose 8080, ENTRYPOINT
5. Create deploy/docker/docker-compose.yml:
   - pgpulse service: build from Dockerfile, port 8080, depends on postgres
   - postgres service: timescale/timescaledb:latest-pg16, port 5432,
     environment: POSTGRES_DB=pgpulse, POSTGRES_USER=pgpulse, POSTGRES_PASSWORD=pgpulse
   - Shared network, volume for postgres data
6. Create .github/workflows/ci.yml:
   - Trigger on push to main and pull requests
   - Steps: checkout, setup-go 1.23, cache modules, golangci-lint, test -race, build
7. Create .golangci.yml:
   - Enable: errcheck, govet, staticcheck, gosimple, ineffassign, unused
   - Timeout: 5m
8. Create .gitignore:
   - Go binaries, vendor/, .env, *.exe, pgpulse-server, pgpulse-agent
   - IDE files: .idea/, .vscode/, *.swp
   - OS files: .DS_Store, Thumbs.db
9. Create README.md with:
   - Project description (PostgreSQL Health & Activity Monitor)
   - Quick start (go build, docker-compose up)
   - Architecture overview (single binary, version-adaptive SQL)
   - Development section (make build, make test, make lint)
   - Link to docs/roadmap.md and docs/architecture.md

Commit: "chore: initialize project structure with build tooling and CI"

SPECIALIST 2 — INTERFACES & CONFIG:
Your job is shared Go interfaces, version detection, and configuration.

1. Create internal/collector/collector.go with these exact interfaces:
   - MetricPoint struct: InstanceID, Metric, Value (float64), Labels (map[string]string), Timestamp
   - Collector interface: Name() string, Collect(ctx, conn) ([]MetricPoint, error), Interval() time.Duration
   - MetricQuery struct: InstanceID, Metric, Labels, From, To, Limit
   - MetricStore interface: Write(ctx, []MetricPoint) error, Query(ctx, MetricQuery) ([]MetricPoint, error), Close() error
   - AlertEvaluator interface: Evaluate(ctx, metric, value, labels) error
   - All types and methods must have doc comments

2. Create internal/version/version.go:
   - PGVersion struct: Major, Minor, Num (int), Full (string)
   - String() method returning "Major.Minor"
   - AtLeast(major, minor int) bool method
   - Detect(ctx, conn) (PGVersion, error) function using "SHOW server_version_num"
   - Parse server_version_num: Major = num/10000, Minor = num%10000/100

3. Create internal/version/gate.go:
   - VersionRange struct: MinMajor, MinMinor, MaxMajor, MaxMinor
   - Contains(PGVersion) bool method
   - SQLVariant struct: Range + SQL string
   - Gate struct: Name + []SQLVariant
   - Select(PGVersion) (string, bool) method — first matching range wins

4. Create configs/pgpulse.example.yml:
   - Server config: host, port, timeouts
   - Storage config: PG connection for PGPulse metadata
   - Instances list: name, host, port, user, password (env var refs), sslmode, databases
   - Collection intervals: high (10s), medium (60s), low (5m)
   - Alert channels: telegram, slack, email, webhook (all with enabled flag)
   - Logging: level, format

5. Verify everything compiles:
   - Run: go mod tidy
   - Run: go build ./...
   - Run: go vet ./...
   - Fix any compilation errors

Commit: "feat: add shared interfaces, version detection, and sample config"

COORDINATION RULES:
- Specialist 1 creates the directory structure FIRST (Specialist 2 needs directories to exist)
- Specialist 2 starts with collector.go and version/ while Specialist 1 works on Docker/CI
- After both are done, Team Lead runs: go build ./... && go vet ./...
- Merge only when compilation succeeds with zero errors
- Final commit by Team Lead: "docs: add project documentation"
