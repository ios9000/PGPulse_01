# M9_01 — Design Document

**Iteration:** M9_01
**Milestone:** M9 — Alert & Advisor Polish
**Date:** 2026-03-14
**Module:** alert-rules-fix

---

## A. Alert Rules Metric Key Audit

### Approach

Same methodology as REM_01c:
1. Read `internal/alert/rules.go` (169 lines, 23 rules)
2. Extract the `Metric` field from every `AlertRule` struct literal
3. Compare each against the canonical metric key in CODEBASE_DIGEST Section 3
4. Fix mismatches in-place
5. Update tests in `rules_test.go` if they assert on metric keys

### Expected Canonical Key Mapping

The table below lists every expected alert rule and its correct canonical metric key. Agents MUST verify each rule's actual `Metric` field against this table. If a rule's current key differs from the canonical key, fix it.

#### Standard Threshold Rules (19)

| Rule Name (expected) | Canonical Metric Key | Operator | Threshold | Severity | Notes |
|---|---|---|---|---|---|
| Wraparound WARNING | `pg.server.wraparound_pct` | > | 20 | warning | Added to ServerInfoCollector in REM_01c |
| Wraparound CRITICAL | `pg.server.wraparound_pct` | > | 50 | critical | |
| Connections WARNING | `pg.connections.utilization_pct` | > | 80 | warning | Pre-computed percentage, no division needed |
| Connections CRITICAL | `pg.connections.utilization_pct` | >= | 99 | critical | |
| Cache Hit WARNING | `pg.cache.hit_ratio` | < | 90 | warning | CacheCollector |
| Commit Ratio WARNING | `pg.transactions.commit_ratio_pct` | < | 90 | warning | Labeled: database |
| Inactive Repl Slots WARNING | `pg.replication.slot.active` | == | 0 | warning | Labeled: slot_name, slot_type, plugin |
| Long Tx WARNING | `pg.long_transactions.oldest_seconds` | > | 60 | warning | Labeled: state |
| Long Tx CRITICAL | `pg.long_transactions.oldest_seconds` | > | 300 | critical | |
| Blocking Locks WARNING | `pg.locks.blocker_count` | > | 0 | warning | LockTreeCollector |
| Bloat WARNING | `pg.db.bloat.table_ratio` | > | 2 | warning | Per-DB metric, labeled |
| Bloat CRITICAL | `pg.db.bloat.table_ratio` | > | 50 | critical | |
| PGSS Fill WARNING | `pg.statements.fill_pct` | >= | 95 | warning | StatementsConfigCollector |
| Track IO Timing INFO | `pg.settings.track_io_timing` | == | 0 | info | SettingsCollector |
| Replication Lag WARNING | `pg.replication.lag.total_bytes` | > | 1048576 | warning | 1 MB, labeled: replica |
| Replication Lag CRITICAL | `pg.replication.lag.total_bytes` | > | 104857600 | critical | 100 MB |
| Connection Saturation WARNING | `pg.connections.utilization_pct` | > | 90 | warning | May share metric with Connections rules |
| Schema DDL Changes INFO | (verify in code) | | | info | May not have a standard metric key |
| (19th rule — verify in code) | (verify in code) | | | | CODEBASE_DIGEST says 19 standard — verify count |

**AGENT INSTRUCTION:** The exact 19th rule and schema DDL rule metric keys must be verified by reading the actual `rules.go` file. The table above covers 18 known rules — identify the remaining rule(s) and verify their keys.

#### Forecast Rules (3)

| Rule Name (expected) | Related Metric Key | Notes |
|---|---|---|
| WAL Spike WARNING | `pg.checkpoint.buffers_written` or similar | Forecast-based, verify in evaluator_forecast logic |
| Query Regression WARNING | `pg.statements.top.total_time_ms` or similar | Forecast-based |
| Disk Forecast CRITICAL | `os.disk.used_bytes` or `pg.os.disk.*` | Forecast-based, may need dual-prefix check |

**AGENT INSTRUCTION:** Forecast rules may reference metric keys differently than standard rules (they go through the ForecastProvider interface). Verify the metric key used in forecast alert evaluation, not just the rule definition.

#### Logical Replication Rule (1)

| Rule Name (expected) | Canonical Metric Key | Notes |
|---|---|---|
| Logical Repl Pending Sync | `pg.db.logical_replication.pending_sync_tables` | Per-DB metric |

### Risk: Dual-prefix OS metrics

If any alert rule references OS metrics, it must handle the dual-prefix pattern (`os.*` from agent, `pg.os.*` from SQL collector). The remediation rules solved this with `getOS()`/`isOSMetric()` helpers. Alert rules may need similar treatment — but only if they reference OS metrics (likely only the disk forecast rule).

### Files Modified

| File | Change |
|---|---|
| `internal/alert/rules.go` | Fix metric key strings in AlertRule definitions |
| `internal/alert/rules_test.go` | Update test assertions for metric keys |
| `internal/alert/seed.go` | Verify seed creates rules with correct keys (if applicable) |

---

## B. Alert Rules Page Navigation

### B1 — Tab Bar Component

**New file:** `web/src/components/alerts/AlertsTabBar.tsx`

```typescript
// Shared tab bar used on both AlertsDashboard and AlertRules pages
// Props: activeTab: 'active' | 'history' | 'rules'

interface AlertsTabBarProps {
  activeTab: 'active' | 'history' | 'rules';
}

// Three tabs:
//   Active  → /alerts (with filter set to active)
//   History → /alerts?view=history (or filter state)
//   Rules   → /alerts/rules
//
// Use react-router-dom Link for navigation
// Active tab gets highlighted border-bottom + bold text
// Match existing Tailwind patterns from the project
```

**Modified files:**

| File | Change |
|---|---|
| `web/src/pages/AlertsDashboard.tsx` | Import + render AlertsTabBar at top, determine active tab from view state |
| `web/src/pages/AlertRules.tsx` | Import + render AlertsTabBar with activeTab="rules" |

**Design note:** If AlertsDashboard currently doesn't distinguish Active vs History views (both use AlertFilters), add a `view` query param or state to control which tab is active. The AlertFilters component may already handle this — verify in code.

### B2 — Sidebar Expandable Group

**Modified file:** `web/src/components/layout/Sidebar.tsx` (109 lines)

Current structure (inferred):
```
Fleet Overview    (LayoutGrid icon)  → /
Alerts            (Bell icon)        → /alerts
Advisor           (Lightbulb icon)   → /advisor
Admin             (Settings icon)    → /admin
```

New structure:
```
Fleet Overview    (LayoutGrid icon)  → /
Alerts            (Bell icon)        → expandable group
  Dashboard       (no icon or dot)   → /alerts
  Rules           (no icon or dot)   → /alerts/rules
Advisor           (Lightbulb icon)   → /advisor
Admin             (Settings icon)    → /admin
```

**Implementation:**
- Add expandable/collapsible behavior to the Alerts sidebar item
- Chevron icon (ChevronDown/ChevronRight from lucide-react) indicates expandable state
- Sub-items indented, smaller font
- Group auto-expands when current route matches `/alerts*`
- Collapse state stored in layoutStore (Zustand) — persists across navigation

---

## C1. Port Display Parser Fix

### Root Cause

When instances use keyword/value DSN format (`host=localhost port=5433 dbname=postgres user=monitor`), the display parser doesn't extract the port. It falls back to `:5432`.

### Fix Location

Search for where host:port display string is constructed. Likely candidates:
- `internal/storage/instances.go` — may have a helper that parses DSN for display
- `internal/api/instances.go` — may construct display data in list/detail handlers
- Frontend components — InstanceCard, InstancesTab, HeaderCard may parse DSN client-side

### Implementation

Add or fix a DSN port extraction function that handles both formats:

```go
// parseDSNPort extracts port from DSN string.
// Handles both URI format (postgres://host:port/db) and
// keyword/value format (host=x port=y dbname=z).
// Returns "5432" if no port specified.
func parseDSNPort(dsn string) string {
    // 1. Check if URI format — look for "://"
    //    Parse with url.Parse, extract port from Host
    // 2. Otherwise keyword/value — regex or string split for "port=NNNN"
    // 3. Default to "5432"
}
```

If the frontend is doing the parsing, apply equivalent logic in TypeScript.

### Files Modified

| File | Change |
|---|---|
| (Determine in code) | Add/fix DSN port parser |
| Affected frontend components if needed | Use correct port display |

---

## C2. Diagnose Panel Metric Value Fix

### Root Cause

`Engine.Diagnose()` iterates all rules against a full `MetricSnapshot`. When a rule fires, it creates a `Recommendation` but doesn't set the `MetricValue` field — that field is only populated in alert-triggered mode (`EvaluateMetric`) where the alert event carries the value.

### Fix

In `internal/remediation/engine.go`, the `Diagnose()` method needs to populate `MetricValue` from the snapshot when building recommendations.

**Option A (preferred):** Each rule's `Evaluate` function returns the metric key + value that triggered it in the `RuleResult`. The engine copies these into the `Recommendation`.

Add to `RuleResult` struct (in `rule.go`):
```go
type RuleResult struct {
    Title       string
    Description string
    DocURL      string
    MetricKey   string   // NEW: which metric key triggered this rule
    MetricValue float64  // NEW: the actual value that triggered this rule
}
```

Each rule's `Evaluate` function already knows which metric it checked and what value it found — it just needs to set these two fields on the result.

**Option B (simpler, if RuleResult already has these fields):** Just verify they're being set in Diagnose mode. The issue may be that `EvalContext.Value` is 0 in Diagnose mode and rules are copying from there instead of from the snapshot lookup.

### Frontend Change

In `web/src/components/server/DiagnosePanel.tsx` (85 lines):
- If `metric_value` is 0 and `metric_key` is empty, hide the value display
- If `metric_value` > 0 (or `metric_key` is non-empty), display `metric_key = metric_value`
- Format the value appropriately (percentage, bytes, seconds, etc.)

### Files Modified

| File | Change |
|---|---|
| `internal/remediation/rule.go` | Add MetricKey + MetricValue to RuleResult (if missing) |
| `internal/remediation/rules_pg.go` | Set MetricKey + MetricValue in each rule's Evaluate func |
| `internal/remediation/rules_os.go` | Same for OS rules |
| `internal/remediation/engine.go` | Map RuleResult.MetricKey/Value into Recommendation |
| `internal/remediation/engine_test.go` | Verify MetricValue populated in Diagnose mode |
| `web/src/components/server/DiagnosePanel.tsx` | Display actual metric value |

### Risk: Cascading changes to Recommendation struct

If `Recommendation` doesn't have `MetricKey`/`MetricValue` fields yet, adding them means:
- Update `Recommendation` struct in `engine.go`
- Update PGStore serialization (if Recommendation is stored to DB)
- Update NullStore (no-op, should be fine)
- Update API JSON response (should auto-serialize with `json` tags)
- Update TypeScript model in `web/src/types/models.ts`

If the fields already exist but are just not populated in Diagnose mode, the change is smaller.

**AGENT INSTRUCTION:** Read the actual `Recommendation` struct first. If it already has metric value fields, just fix the population logic. If not, add them with the cascading updates listed above.

---

## C3. Chaos Script Fix

### Fix Pattern

For each script in `/opt/pgpulse/chaos/`:

Before:
```bash
#!/bin/bash
# Relies on PGPASSWORD being set in caller's environment
psql -h localhost -p 5432 -U pgpulse_monitor -d postgres -c "..."
```

After:
```bash
#!/bin/bash
# Password embedded — works with sudo bash
PGPASSWORD=pgpulse_monitor_demo psql -h localhost -p 5432 -U pgpulse_monitor -d postgres -c "..."
```

**IMPORTANT:** This is a demo environment only fix. Not committed to the main repo — applied directly on the VM via SCP or SSH.

### Files

All scripts in `/opt/pgpulse/chaos/*.sh` on the demo VM (185.159.111.139).

---

## Agent Team Sizing

**2 agents:** Backend + Frontend

| Agent | Owns | Work |
|---|---|---|
| **Backend Agent** | `internal/alert/`, `internal/remediation/`, `internal/storage/`, `internal/api/` | A (metric key audit), C1 backend (DSN parser), C2 backend (metric value population) |
| **Frontend Agent** | `web/src/` | B1 (tab bar), B2 (sidebar), C1 frontend (if needed), C2 frontend (DiagnosePanel) |

Both agents write their own tests. No dedicated QA agent — scope is contained.

**Chaos script fix (C3)** is a manual step documented in the checklist (SSH to VM). Not agent work.
