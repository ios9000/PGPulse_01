# M1_02a — Claude Code Prompt: InstanceContext Refactor

> **Mode:** Single Claude Code session (no Agent Teams)
> **Model:** claude-opus-4-6
> **Estimated tokens:** ~100K input, ~50K output
> **Estimated time:** ~15 minutes

---

## Prompt (paste into Claude Code)

```
Read CLAUDE.md for project context, then read docs/iterations/M1_02a/design.md
for the exact changes to make.

This is a focused interface refactor. No new features, no new collectors.
The goal is to add an InstanceContext struct to the Collector interface so
that per-scrape-cycle state (starting with IsRecovery) can be passed from
the orchestrator to all collectors.

Here is the complete change list:

### 1. internal/collector/collector.go
- Add InstanceContext struct with IsRecovery bool field (with doc comment)
- Update Collector interface: Collect() now takes InstanceContext as third arg

### 2. Six mechanical collector updates (signature only)
Update the Collect() method signature in each file — add `_ InstanceContext`
as the third parameter. No other changes to the method body.
Files:
- internal/collector/connections.go
- internal/collector/cache.go
- internal/collector/transactions.go
- internal/collector/database_sizes.go
- internal/collector/settings.go
- internal/collector/extensions.go

### 3. internal/collector/server_info.go (logic change)
- Update Collect() signature: add `ic InstanceContext` (NOT underscore — we use it)
- REMOVE the pg_is_in_recovery() query from the method body
- INSTEAD, read ic.IsRecovery to emit the pgpulse.server.is_in_recovery metric:
  ```go
  recoveryVal := 0.0
  if ic.IsRecovery {
      recoveryVal = 1.0
  }
  points = append(points, b.point("server.is_in_recovery", recoveryVal, nil))
  ```
- Keep pg_is_in_backup() query unchanged (it stays in this collector)

### 4. internal/collector/registry.go
- Update CollectAll() signature: add InstanceContext as third arg (after conn, before collectors)
- Pass ic to each collector's Collect() call inside the loop

### 5. Test files — update all Collect() calls
In every test file, update calls to Collect() to pass InstanceContext{}:

- internal/collector/testutil_test.go — update any helpers that call Collect()
- internal/collector/connections_test.go — Collect(ctx, conn, InstanceContext{})
- internal/collector/cache_test.go — same
- internal/collector/transactions_test.go — same
- internal/collector/database_sizes_test.go — same
- internal/collector/settings_test.go — same
- internal/collector/extensions_test.go — same

### 6. internal/collector/server_info_test.go (logic change)
- Remove any mock expectation for pg_is_in_recovery() query
- Add two test cases:
  - Primary mode: Collect(ctx, conn, InstanceContext{IsRecovery: false})
    → assert pgpulse.server.is_in_recovery == 0.0
  - Replica mode: Collect(ctx, conn, InstanceContext{IsRecovery: true})
    → assert pgpulse.server.is_in_recovery == 1.0

### 7. internal/collector/registry_test.go
- Update mockCollector.Collect() signature to match new interface
- Update CollectAll() calls to pass InstanceContext{}

## IMPORTANT CONSTRAINTS
- Do NOT create any new files
- Do NOT add any new dependencies to go.mod
- Do NOT change internal/version/ at all
- Do NOT modify the MetricPoint struct or MetricStore/AlertEvaluator interfaces
- Do NOT add any fields to InstanceContext beyond IsRecovery
- Do NOT rename Base struct fields or methods
- The ONLY logic change is in server_info.go (remove query, read from ic)
- Everything else is a mechanical signature update

## CANNOT RUN BASH
You cannot run shell commands on this platform (Windows bash bug).
Create/edit files only. After you finish, list all modified files so the
developer can run the validation commands manually:

go build ./...
go vet ./...
golangci-lint run
go test ./internal/collector/...
go test ./internal/version/...
```

---

## Developer Post-Session Checklist

After Claude Code finishes editing files:

```bash
cd ~/Projects/PGPulse_01

# 1. Build
go build ./...

# 2. Vet
go vet ./...

# 3. Lint
golangci-lint run

# 4. Test
go test ./internal/collector/...
go test ./internal/version/...

# 5. If all pass — commit
git add internal/collector/
git commit -m "refactor(collector): add InstanceContext to Collector interface

- Add InstanceContext struct with IsRecovery field
- Update Collector.Collect() signature across all 8 collectors
- ServerInfoCollector reads ic.IsRecovery instead of querying pg_is_in_recovery()
- Update registry.CollectAll() to pass InstanceContext
- Update all tests for new signature
- Prepares interface for M1_02b replication collectors"

git push origin main
```

## Success Criteria

| Check | Expected |
|-------|----------|
| `go build ./...` | ✅ No errors |
| `go vet ./...` | ✅ No warnings |
| `golangci-lint run` | ✅ 0 issues |
| `go test ./internal/collector/...` | ✅ All pass |
| `go test ./internal/version/...` | ✅ All pass (unchanged) |
| No new files created | ✅ Only edits to existing files |
| No new go.mod dependencies | ✅ Unchanged |
| `git diff --stat` | ~18 files changed, ~50 insertions, ~30 deletions |
