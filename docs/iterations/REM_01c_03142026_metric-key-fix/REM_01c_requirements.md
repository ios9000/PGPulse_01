# REM_01c — Remediation Rule Metric Key Fix

**Iteration:** REM_01c (bugfix)
**Date:** 2026-03-14
**Scope:** Fix 13 broken remediation rules + add wraparound metric
**Follows:** REM_01b (commits fcf45b4, 6a3bc32)

---

## Problem

13 of 25 remediation rules reference metric keys that don't match the actual
collector output. Diagnose returns "No issues found" even when problems exist
because the rules look up non-existent keys in the MetricSnapshot.

## Root Cause

Rules were written against assumed metric key names during REM_01a design.
The actual keys emitted by collectors use different naming conventions.

## Decisions (Locked)

| ID | Decision | Choice |
|----|----------|--------|
| D315 | Wraparound rules | Patch + add wraparound metric to ServerInfoCollector |
| D316 | Iteration process | REM_01c as bugfix sub-iteration (full process) |

## Scope

### Fix 1: PG Rule Key Corrections (rules_pg.go)
- 9 rules with wrong metric key names → correct to match CODEBASE_DIGEST Section 3

### Fix 2: OS Rule Dual-Prefix Support (rules_os.go)
- 8 OS rules must check both `os.*` (agent) and `pg.os.*` (OSSQLCollector) prefixes
- Add `getOS()` helper function

### Fix 3: Add Wraparound Metric (server_info.go)
- Add `pg.server.wraparound_pct` metric to ServerInfoCollector
- Query: `SELECT max(age(datfrozenxid))::float / 2147483647 * 100 FROM pg_database`
- Enables rules 12 and 13 to fire correctly

### Fix 4: Update Tests (rules_test.go)
- Update all affected test cases to use correct metric keys in snapshots
