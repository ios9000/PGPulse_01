# Session: 2026-02-25 — M0_01 Project Setup

## Goal
Initialize the PGPulse repository with complete project structure, build tooling,
CI/CD pipeline, Docker environment, shared Go interfaces, and version detection.

## Agent Team Configuration
- Team Lead: Opus 4.6 (Claude Code 2.1.53)
- Specialists: Scaffold Agent, Interfaces Agent (planned as 2-agent team)
- Platform: Windows 10 / Git Bash (MSYS2) / In-process mode
- Duration: ~45 minutes (including troubleshooting)
- Outcome: **Partial agent success** — files created, bash execution failed

## What Happened

### Attempt 1: Full Agent Team
- Pasted full team-prompt.md with 2 specialists
- **TMPDIR issue:** Claude Code's temp path mangled `PGPulse_01` → `PGPulse-01`,
  causing EINVAL errors on all bash tool calls
- Scaffold Agent successfully created all files via file creation tool (no bash needed)
- Team Lead could not run `go build` verification — bash broken
- **Aborted** after persistent EINVAL errors

### Attempt 2: Trimmed prompt (interfaces only)
- Fixed TMPDIR in ~/.bashrc (`$HOME/tmp/claude`)
- Relaunched Claude Code with trimmed prompt (skip scaffold, do interfaces only)
- Agent found existing scaffold files, created all 4 interface/version/config files
- **Bash still broken** — TMPDIR fix in shell didn't propagate to Claude Code's internal task runner
- Agent provided manual commands as fallback

### Manual Completion
- Ran `go mod tidy`, `go build ./...`, `go vet ./...` manually — all clean
- Go auto-upgraded to 1.25.7 (pgx v5.8.0 requires Go ≥ 1.24)
- Two commits created and pushed to GitHub successfully
- GitHub token required `workflow` scope for CI file push — fixed and pushed

## Critical Issue: Claude Code Bash on Windows

**Root cause:** Claude Code's task runner writes output files to:
```
C:\Users\Archer\AppData\Local\Temp\claude\C--Users-Archer-Projects-PGPulse-01\tasks\*.output
```
The path `C--Users-Archer-Projects-PGPulse-01` is derived from the project path
and contains characters that cause EINVAL on Windows NTFS. This is NOT fixable
via shell TMPDIR — it's internal to Claude Code's task execution engine.

**Impact:** All bash commands fail. File creation/reading works fine. Agent Teams
coordination works fine. Only bash execution is broken.

**Workaround options for M1:**
1. Agents create files, developer runs bash commands manually
2. Investigate Claude Code Windows temp path configuration
3. Report as bug to Anthropic (Claude Code GitHub issues)
4. Use shorter project path without underscores (e.g., C:\pgpulse)

## Files Created

### By Scaffold Agent (Attempt 1)
| File | Lines | Description |
|---|---|---|
| cmd/pgpulse-server/main.go | 16 | Server entrypoint placeholder |
| cmd/pgpulse-agent/main.go | 16 | Agent entrypoint placeholder |
| Makefile | ~40 | build, test, lint, docker targets |
| deploy/docker/Dockerfile | ~25 | Multi-stage Go build |
| deploy/docker/docker-compose.yml | ~30 | pgpulse + timescaledb |
| .github/workflows/ci.yml | ~35 | Lint + test + build on push/PR |
| .golangci.yml | ~15 | Linter configuration |
| .gitignore | ~20 | Go + IDE + OS ignores |
| README.md | ~50 | Project description + quick start |
| go.mod | 5 | Module: github.com/ios9000/PGPulse_01 |
| 10x .gitkeep | — | Placeholder dirs |

### By Interfaces Agent (Attempt 2)
| File | Lines | Description |
|---|---|---|
| internal/collector/collector.go | ~75 | MetricPoint, Collector, MetricStore, AlertEvaluator |
| internal/version/version.go | ~55 | PGVersion struct, Detect(), AtLeast() |
| internal/version/gate.go | ~60 | VersionRange, Gate, SQLVariant, Select() |
| configs/pgpulse.example.yml | ~70 | Full sample config |

## Commits
| Hash | Message | Files |
|---|---|---|
| 78eb2c5 | chore: initialize project structure with build tooling and CI | 33 files |
| 182173d | feat: add shared interfaces, version detection, and sample config | 5 files |

## Verification Results
- `go mod tidy` ✅ (auto-upgraded to Go 1.25.7 for pgx v5.8.0)
- `go build ./...` ✅ zero errors
- `go vet ./...` ✅ zero warnings
- `git push origin master` ✅ pushed to github.com/ios9000/PGPulse_01

## Requirements Checklist
- [x] Go module initialized: github.com/ios9000/PGPulse_01
- [x] Full directory tree created
- [x] Both main.go placeholders compile
- [x] go.mod with pgx v5 dependency
- [x] Makefile with build/test/lint/docker targets
- [x] .golangci.yml configured
- [x] Dockerfile (multi-stage)
- [x] docker-compose.yml (pgpulse + timescaledb)
- [x] CI pipeline (.github/workflows/ci.yml)
- [x] Shared interfaces (collector.go)
- [x] Version detection (version.go + gate.go)
- [x] Sample config (pgpulse.example.yml)
- [x] README.md
- [ ] golangci-lint run (not executed — bash broken, defer to M1)
- [ ] Docker build test (not executed — defer to M1)

## Lessons Learned
1. **Claude Code bash is broken on Windows with Agent Teams** — temp path issue
2. **TMPDIR shell fix doesn't help** — Claude Code uses internal task runner paths
3. **File creation works fine** — agents can create Go files without bash
4. **Agent Teams coordination works** — task list, dependencies, messaging all functional
5. **Fallback strategy works** — agents create files, developer runs commands
6. **Go auto-upgrade** — pgx v5.8.0 pulled Go to 1.25.7, not a problem
7. **GitHub workflow scope** — PAT needs `workflow` permission for CI files

## Decisions for M1
- Investigate shorter project path to avoid Windows temp path issue
- OR: report bug and use hybrid mode (agents create files, manual bash)
- Fix LF/CRLF warnings: add `.gitattributes` with `* text=auto eol=lf`
- Run golangci-lint manually before M1 starts

## Not Done / Next Iteration
- [ ] golangci-lint run (manual)
- [ ] Docker build verification
- [ ] .gitattributes for line endings
- [ ] Resolve Claude Code Windows bash issue before M1
- [ ] Begin M1_01: Instance metrics collector (PGAM queries 1–19)
