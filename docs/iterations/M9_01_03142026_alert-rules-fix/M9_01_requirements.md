# M9_01 — Alert & Advisor Polish (Metric Keys + UI Nav + Cosmetic Fixes)

## Requirements

**Iteration:** M9_01
**Milestone:** M9 — Alert & Advisor Polish
**Date:** 2026-03-14
**Scope:** Alert rules metric key audit, alert navigation UX, cosmetic fix batch

---

## Work Streams

### A. Alert Rules Metric Key Audit (Backend)

**Problem:** The 23 default alert rules in `internal/alert/rules.go` were authored in M4 before the MN_01 metric naming standardization. They likely reference pre-standardization metric key names. This is the same class of issue fixed in REM_01c for the 25 remediation rules.

**Requirement:** Audit every alert rule's `Metric` field against the canonical metric keys in CODEBASE_DIGEST Section 3. Fix any mismatches. Verify all 23 rules (19 standard + 3 forecast + 1 logical replication) reference keys that collectors actually emit.

**Acceptance Criteria:**
- Every rule's `Metric` field matches a key from CODEBASE_DIGEST Section 3 exactly
- Alert tests updated to verify correct metric keys
- No alert rule references a metric key that no collector emits
- Seed logic (if any) applies fixed keys on fresh installs

### B. Alert Rules Page Navigation (Frontend)

**Problem:** Two navigation gaps:
1. No way to navigate from the Alerts page (`/alerts`) to the Alert Rules page (`/alerts/rules`)
2. No sidebar entry for Alert Rules — users can only reach it via direct URL

**Requirement:** Add both a tab bar and a sidebar sub-item.

**B1 — Tab Bar on Alerts Pages:**
- Shared tab bar component at the top of both `AlertsDashboard` and `AlertRules` pages
- Three tabs: **Active** | **History** | **Rules**
- "Active" and "History" route to `/alerts` with appropriate filter/view state
- "Rules" routes to `/alerts/rules`
- Active tab visually highlighted based on current route/view

**B2 — Sidebar Sub-Items:**
- "Alerts" sidebar entry becomes expandable/collapsible group
- Sub-items: "Dashboard" (→ `/alerts`), "Rules" (→ `/alerts/rules`)
- Current: Sidebar has flat "Alerts" item with Bell icon
- After: "Alerts" group with chevron, expands to show Dashboard + Rules

**Acceptance Criteria:**
- Tab bar renders on both `/alerts` and `/alerts/rules` pages
- Active tab matches current page
- Sidebar shows expandable Alerts group with two sub-items
- Both navigation paths reach Alert Rules page
- Advisor sidebar item remains between Alerts group and Admin
- No broken navigation on page refresh
- TypeScript compilation clean, no lint warnings

### C. Cosmetic Fix Batch

**C1 — Port Display Parser Fix (Backend + Frontend):**

**Problem:** Fleet Overview, Administration, and ServerDetail header all show `:5432` for every instance. The DSN parser defaults to port 5432 when parsing keyword/value format DSNs (e.g., `host=localhost port=5433 dbname=postgres`).

**Requirement:** Fix the DSN parser to correctly extract the port from both URI format (`postgres://host:port/db`) and keyword/value format (`host=x port=y dbname=z`). Instances without explicit port should still default to 5432.

**Acceptance Criteria:**
- Instance with `port=5433` in keyword/value DSN shows `:5433` in UI
- Instance with URI format DSN shows correct port
- Instance with no port specified shows `:5432`
- All three affected views (FleetOverview, Administration, ServerDetail) display correct port

**C2 — Diagnose Panel Metric Value Fix (Backend + Frontend):**

**Problem:** DiagnosePanel shows `= 0.00` instead of the actual metric value. In Diagnose mode, `metric_value` isn't populated (it's only set in alert-triggered mode).

**Requirement:** Populate `metric_value` from the snapshot data in Diagnose mode. Each rule in `Engine.Diagnose()` evaluates against the snapshot — the metric value that triggered the rule should be included in the recommendation result.

**Acceptance Criteria:**
- Diagnose results include actual metric values (not 0.00)
- DiagnosePanel displays the metric value when present
- Alert-triggered recommendations still work as before (no regression)
- Engine tests verify metric_value is populated in Diagnose mode

**C3 — Chaos Script Fix (Demo Environment):**

**Problem:** `sudo bash /opt/pgpulse/chaos/*.sh` fails because `sudo` strips `PGPASSWORD` environment variable.

**Requirement:** Fix all chaos scripts to embed the password inline (hardcoded `PGPASSWORD=pgpulse_monitor_demo` before each `psql` invocation inside the script body).

**Acceptance Criteria:**
- All chaos scripts work with `sudo bash /opt/pgpulse/chaos/<script>.sh`
- Password is embedded in each script, not relying on caller's environment
- Existing chaos behavior unchanged (just the auth mechanism)

---

## Out of Scope

- Adding new alert rules (only fixing existing rule metric keys)
- Changing alert thresholds or severities
- Adding new remediation rules
- Prometheus exporter (next iteration)
- `wastedibytes` float64→int64 bug (pre-existing, tracked separately)
- `c.command_desc` PG16 SQL bug (pre-existing, tracked separately)
