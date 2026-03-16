# M11_01 Pre-Flight Corrections

**Purpose:** Identify structural mismatches and gotchas before spawning agents.
**Run these grep/verification commands in Claude Code BEFORE pasting the team-prompt.**

---

## 1. Verify No Name Collisions

```bash
# Check "SnapshotStore" isn't already used (settings package has PGSnapshotStore)
grep -r "SnapshotStore" internal/ --include="*.go" -l
# Expected: internal/settings/store.go — this is settings.PGSnapshotStore, different package = OK
# If statements.SnapshotStore clashes at import site, use alias

# Check migration numbering
ls internal/storage/migrations/
# Expected: latest is 014_*. Confirm 015 doesn't exist.
```

## 2. Find Exact Bug Locations

```bash
# wastedibytes — find the scan variable
grep -n -i "wastedibytes\|wasted_bytes\|wastedIBytes" internal/collector/database.go
# Note the exact variable name and line number for Agent 3

# srsubstate — find the scan
grep -rn "srsubstate" internal/ --include="*.go"
# Note which file and line — could be database.go or logical_replication.go

# Debug log in main.go
grep -n "remediation config" cmd/pgpulse-server/main.go
# Note exact line for Agent 3

# multixact_pct — verify alert rules reference it
grep -n "multixact_pct" internal/alert/rules.go
# Verify server_info.go does NOT emit it
grep -n "multixact_pct" internal/collector/server_info.go
# Expected: no results (confirms the bug)
```

## 3. Verify Patterns to Mirror

```bash
# BackgroundEvaluator pattern (for SnapshotCapturer)
head -60 internal/remediation/background.go
# Note: constructor signature, Start/Stop pattern, ticker loop structure

# Settings snapshot pattern
head -40 internal/settings/snapshot.go
# Note: how it gets pools, iterates instances

# Version gate usage
grep -n "version.Gate\|SQLVariant" internal/collector/ --include="*.go" | head -10
# Note: how existing collectors use Gate for version-specific SQL

# ConnFor signature
grep -n "ConnFor" internal/orchestrator/orchestrator.go | head -5
# Note: exact signature — agents need this for capture.go

# DBInstanceLister
grep -n "DBInstanceLister\|InstanceLister" internal/ml/ --include="*.go" | head -5
# Note: interface and constructor for listing active instances
```

## 4. Verify Import Paths

```bash
# Module name
head -1 go.mod
# Expected: module github.com/ios9000/PGPulse_01 (or similar)
# Agents must use this exact module path for internal imports
```

## 5. Verify pgx CopyFrom Usage

```bash
# Check if CopyFrom is used anywhere already (for reference)
grep -rn "CopyFrom\|CopyFromRows\|CopyFromSlice" internal/ --include="*.go"
# If no existing usage, agents should reference pgx docs for correct API
```

---

## Corrections to Include in Agent Instructions

Based on grep results, update the team-prompt with:

1. **Exact variable names** for wastedibytes and srsubstate bugs
2. **Exact line numbers** for debug log removal
3. **Module path** for import statements
4. **ConnFor signature** — exact function type for capture.go dependency
5. **InstanceLister interface** — exact location and method signature
6. **Any CopyFrom examples** to reference in pgstore.go
