# PGPulse — Save Point System

> **Purpose:** Capture complete project state at key milestones so the entire
> project can be restored in a new Claude.ai Project, a different AI tool,
> or with a different developer — with minimal loss.
>
> Think of it as a "Mass Effect save" — you reload and continue from exactly
> where you left off, with all decisions, context, and history intact.

---

## When to Create a Save Point

| Trigger | Example |
|---|---|
| **Milestone completed** | M0 done, M1 done, MVP done |
| **Major architecture decision** | Changed from Svelte to React, switched storage backend |
| **Before risky changes** | Before ML module, before major refactor |
| **Project migration** | Moving to new Claude.ai Project, new AI tool, new developer |
| **Monthly checkpoint** | Even if mid-milestone, save state monthly |

---

## Save Point Structure

```
docs/save-points/
├── SAVEPOINT_M0_20260225.md      ← after M0
├── SAVEPOINT_M1_20260310.md      ← after M1
├── SAVEPOINT_MVP_20260420.md     ← after M5 (MVP release)
└── LATEST.md                     ← symlink/copy of most recent
```

Each save point is a **single self-contained Markdown file** that includes
everything needed to restore the project. No external references — if the
repo disappeared tomorrow, this file alone would let you rebuild.

---

## Save Point Template

````markdown
# PGPulse — Save Point

**Save Point:** M{X} — {Milestone Name}
**Date:** YYYY-MM-DD
**Commit:** {git hash}
**Developer:** {name}
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code 2.1.53 (Agent Teams)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor
**Repo:** https://github.com/ios9000/PGPulse_01
**Legacy repo:** https://github.com/ios9000/pgam-legacy
**Go module:** github.com/ios9000/PGPulse_01
**License:** [TBD]

### What PGPulse Does
[2-3 paragraph description of the product — what it monitors, who it's for,
how it differs from competitors. Written for someone who has never seen the project.]

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank.
PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero auth,
SQL injection vulnerabilities, and relied on COPY TO PROGRAM for OS metrics (superuser required).
PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while
fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.25.7 | Performance, single binary, goroutines for collectors |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries |
| HTTP Router | go-chi/chi v5 | — | Lightweight, middleware-friendly |
| Storage | PostgreSQL + TimescaleDB | — | Time-series hypertables for metrics |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed |
| Config | koanf | — | YAML + env vars |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | — | Real PG instances in tests |
| ML (Phase 1) | gonum | — | Pure Go statistics |
| CI | GitHub Actions | — | Lint + test + build |

### Architecture Diagram
```
┌─────────────────────────────────────────┐
│         PGPulse Server (Go binary)      │
│                                         │
│  ┌─────────┐  ┌──────┐  ┌──────────┐   │
│  │Collectors│→ │Storage│← │ REST API │   │
│  │(pgx)    │  │(TSDB) │  │(chi+JWT) │   │
│  └────┬────┘  └───────┘  └────┬─────┘   │
│       │                       │          │
│  ┌────▼────┐            ┌────▼─────┐    │
│  │ Version │            │  Auth    │    │
│  │  Gate   │            │ (RBAC)   │    │
│  └─────────┘            └──────────┘    │
│                                         │
│  ┌─────────┐  ┌──────────────────┐      │
│  │  Alert  │  │  Web UI (embed)  │      │
│  │ Engine  │  │  Svelte+Tailwind │      │
│  └─────────┘  └──────────────────┘      │
└─────────────────────────────────────────┘
         │
    ┌────▼─────┐
    │ PGPulse  │  (optional, separate binary)
    │  Agent   │  OS metrics via procfs
    └──────────┘
```

### Key Design Decisions

| # | Decision | Rationale | Date |
|---|----------|-----------|------|
| D1 | Single binary deployment | Simplicity, go:embed frontend | 2026-02-25 |
| D2 | pgx v5 (not database/sql) | Named args, COPY protocol, pgx-specific features | 2026-02-25 |
| D3 | Version-adaptive SQL via Gate pattern | Support PG 14-18 without code branches | 2026-02-25 |
| D4 | No COPY TO PROGRAM ever | PGAM's worst security flaw — eliminated entirely | 2026-02-25 |
| D5 | pg_monitor role only | Least privilege, never superuser | 2026-02-25 |
| D6 | One Collector per module | Testable, enable/disable, independent intervals | 2026-02-25 |
| D7 | Hybrid agent workflow | Claude Code bash broken on Windows; agents create files, dev runs bash | 2026-02-25 |
| [Add more as project evolves] | | |

---

## 3. CODEBASE STATE

### File Tree (at save point)
[Paste output of: find . -not -path './.git/*' -type f | sort]

### Key Interfaces

```go
// internal/collector/collector.go

type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
    Interval() time.Duration
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

```go
// internal/version/version.go

type PGVersion struct {
    Major int
    Minor int
    Num   int
    Full  string
}

func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error)
func (v PGVersion) AtLeast(major, minor int) bool
```

```go
// internal/version/gate.go

type Gate struct {
    Name     string
    Variants []SQLVariant
}

func (g Gate) Select(v PGVersion) (string, bool)
```

### Dependencies (go.mod)
[Paste contents of go.mod at save point]

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php #1–19 | Instance metrics | collector/instance group | [STATUS] |
| analiz2.php #20–41 | Replication | collector/replication.go | [STATUS] |
| analiz2.php #42–47 | Progress | collector/progress.go | [STATUS] |
| analiz2.php #48–52 | Statements | collector/statements.go | [STATUS] |
| analiz2.php #53–58 | Locks | collector/locks.go | [STATUS] |
| analiz_db.php #1–18 | Per-DB analysis | collector/database.go | [STATUS] |
| **Total: 76** | | | **X/76 ported** |

### PGAM Bugs Fixed During Port
[List every PGAM bug discovered and fixed, with query number]

1. Q11: Connection count includes own connection → added WHERE pid != pg_backend_pid()
2. Q14: Cache hit ratio division by zero → added NULLIF guard
3. [Add more as discovered]

### Version Gates Implemented
[List every PG version difference handled]

1. Q10: pg_is_in_backup() → removed in PG 15, return false
2. [Add more as discovered]

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | [STATUS] | [DATE] |
| M2 | Storage & API | [STATUS] | [DATE] |
| M3 | Auth & Security | [STATUS] | [DATE] |
| M4 | Alerting | [STATUS] | [DATE] |
| M5 | Web UI (MVP) | [STATUS] | [DATE] |
| M6 | Agent Mode | [STATUS] | [DATE] |
| M7 | P1 Features | [STATUS] | [DATE] |
| M8 | ML Phase 1 | [STATUS] | [DATE] |
| M9 | Reports & Export | [STATUS] | [DATE] |
| M10 | Polish & Release | [STATUS] | [DATE] |

### What Was Just Completed
[Detailed description of the milestone that triggered this save point]

### What's Next
[Specific next task with enough detail to start immediately]

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) + PowerShell |
| Go | 1.25.7 |
| Node.js | 22.14.0 |
| Claude Code | 2.1.53 |
| Git | 2.52.0 |
| golangci-lint | v1.64.8 |
| IDE | [TBD] |

### Development Method
- **Two-contour model:** Claude.ai (Brain) + Claude Code (Hands)
- **Agent Teams:** Enabled but bash broken on Windows (EINVAL temp path bug)
- **Hybrid workflow:** Agents create files, developer runs go build/test/commit manually
- **One chat per iteration** in Claude.ai
- **Project Knowledge** contains: strategy, PGAM audit, architecture doc
- **Iteration Handoff** documents bridge between chats

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL on Windows | Unresolved (v2.1.53) | Agents create files, dev runs bash manually |
| LF/CRLF warnings | Needs .gitattributes | Add `* text=auto eol=lf` |
| WSL2 unavailable | BIOS virtualization disabled | Using native Windows |
| Go auto-upgraded to 1.25.7 | Accepted | pgx v5.8.0 requires Go ≥ 1.24 |

---

## 7. KEY LEARNINGS & DECISIONS LOG

### Architecture Decisions
[Chronological log of every significant decision with rationale]

| Date | Decision | Alternatives Considered | Why This Choice |
|---|---|---|---|
| 2026-02-25 | Go over Rust | Rust has steeper learning curve, Go goroutines natural for collectors | Faster development, good enough performance |
| 2026-02-25 | pgx over database/sql | database/sql lacks PG-specific features | Named args, COPY, notifications |
| 2026-02-25 | TimescaleDB over InfluxDB | InfluxDB requires separate service | PG-native, one less dependency |
| 2026-02-25 | Agent Teams (4 agents) | 7 agents (enterprise template) | Right-sized for 1-dev project |
| 2026-02-25 | Hybrid mode (agents + manual bash) | Pure Agent Teams | Windows bash bug forced this |
| [Add more] | | | |

### Issues & Resolutions
[Every significant problem encountered and how it was resolved]

| Date | Issue | Resolution |
|---|---|---|
| 2026-02-25 | TMPDIR path mangling in Claude Code | Not resolved — working around with hybrid mode |
| 2026-02-25 | GitHub PAT missing workflow scope | Added workflow scope to PAT |
| 2026-02-25 | WSL2 install failed (BIOS virtualization) | Abandoned WSL, using native Git Bash |
| 2026-02-25 | New chat lost context | Created handoff document system |
| [Add more] | | |

### Competitive Intelligence Summary
[Key takeaways from research, not the full report]

- pgwatch v3: Go-based, SQL metrics, 4 storage backends — closest architectural cousin
- Percona PMM: heavyweight but comprehensive, QAN is the gold standard for query analytics
- pganalyze: SaaS, $149/mo, HypoPG index advisor — sets bar for query-level analytics
- Tantor Platform: most feature-rich Russian solution, microservice arch, Kubernetes
- SolarWinds DPA: ML anomaly detection with seasonal baselines — our ML target

---

## 8. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. Continue from [milestone]."

### Option B: New Claude.ai Project from Scratch
1. Create new Claude.ai Project named "PGPulse"
2. Upload to Project Knowledge:
   - This save point file
   - PGAM_FEATURE_AUDIT.md
   - PGPulse_Development_Strategy_v2.md (from repo docs/)
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse project from save point. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone repo: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this save point file — it contains complete project context
3. Read `.claude/CLAUDE.md` for module ownership and interfaces
4. Read `docs/roadmap.md` for current milestone status
5. Continue development from the "What's Next" section above

### Option D: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces and key code snippets
2. The architecture and decisions are documented above
3. PGAM SQL queries are in section 4 (or in the audit doc)
4. Rebuild from the interfaces → implement collectors → add storage → add API
````

---

## Creating a Save Point — Checklist

Run this at the end of each milestone:

```bash
cd ~/Projects/PGPulse_01

# 1. Capture file tree
echo "### File Tree" > /tmp/filetree.txt
find . -not -path './.git/*' -type f | sort >> /tmp/filetree.txt

# 2. Capture go.mod
echo "### go.mod" > /tmp/gomod.txt
cat go.mod >> /tmp/gomod.txt

# 3. Capture git log
echo "### Recent Commits" > /tmp/gitlog.txt
git log --oneline -20 >> /tmp/gitlog.txt

# 4. Capture test results
echo "### Test Results" > /tmp/tests.txt
go test ./... 2>&1 >> /tmp/tests.txt

# 5. Open the save point template and fill in:
#    - Paste file tree, go.mod, git log, test results
#    - Update milestone status
#    - Update query porting status
#    - Add any new decisions/issues
#    - Save as docs/save-points/SAVEPOINT_M{X}_{date}.md

# 6. Commit
git add docs/save-points/
git commit -m "docs: create save point after M{X}"
git push
```

---

## Save Point vs. Other Documents

```
SAVE POINT (complete snapshot)
├── Contains: architecture + interfaces + decisions + status + env + history
├── Self-contained: works even if repo is lost
├── Created: per milestone (or monthly)
└── Used for: project migration, disaster recovery, new developer onboarding

HANDOFF (transition bridge)
├── Contains: what changed + what's next + key interfaces
├── Focused: only what the next chat needs
├── Created: per chat transition
└── Used for: chat-to-chat continuity within same project

SESSION-LOG (iteration record)
├── Contains: what happened in one iteration
├── Detailed: agent activity, commits, test results
├── Created: per iteration
└── Used for: audit trail, debugging, learning
```
