# M10_01 — Advisor Auto-Population

## Requirements

**Iteration:** M10_01
**Milestone:** M10 — Advisor Auto-Population
**Date:** 2026-03-16
**Scope:** Background remediation evaluation, Advisor auto-population, "Create Alert Rule" promotion

---

## Problem Statement

The remediation engine's 25 rules only run when a DBA manually clicks "Diagnose" on a specific instance. The Advisor page remains empty until someone actively checks. DBAs need issues surfaced proactively without manual polling of each server.

## Solution

Run `Engine.Diagnose()` periodically in the background for every monitored instance. Store results in the existing `remediation_recommendations` table with timestamps. Surface them automatically on the Advisor page. Allow DBAs to promote any recommendation into a persistent alert rule.

---

## Requirements

### R1: Background Evaluation Worker

- New goroutine-based worker that runs on a configurable timer
- Calls `Engine.Diagnose(ctx, instanceID, snapshot)` for every active instance
- Stores results via `RecommendationStore.Write()`
- Only runs when persistent storage is available (NOT in live/MemoryStore mode)
- Skips instances that are currently unreachable (no crash on connection error)
- Logs each evaluation cycle: instance count, recommendations found, duration

### R2: Configuration

New config section in YAML:

```yaml
remediation:
  enabled: true
  background_interval: 5m
  retention_days: 30
```

- `remediation.enabled` (bool, default false) — master switch for background evaluation
- `remediation.background_interval` (duration, default 5m) — how often to evaluate each instance
- `remediation.retention_days` (int, default 30) — how long to keep recommendation history

### R3: Recommendation History

- Each evaluation cycle produces new recommendation records with timestamps
- Old recommendations cleaned up based on `retention_days`
- Advisor page shows both current (latest cycle) and historical recommendations
- Status model: `active` (latest cycle found it), `resolved` (latest cycle didn't find it), `acknowledged` (DBA dismissed it)

### R4: "Create Alert Rule" Promotion

- Each recommendation row on the Advisor page gets a "Create Alert Rule" button
- Clicking opens the existing `RuleFormModal` pre-filled with:
  - Metric key from the recommendation
  - Suggested threshold derived from the rule
  - Severity mapped from recommendation priority (action_required → critical, suggestion → warning, info → info)
- DBA can edit any field before saving
- Uses existing `POST /api/v1/alerts/rules` endpoint — no new backend endpoint needed
- Button only visible to users with `dba+` role

### R5: Live Mode Behavior

- Background evaluation does NOT run in live mode (MemoryStore)
- Diagnose-on-demand continues to work as before (current behavior)
- Advisor page shows on-demand Diagnose results only in live mode
- No change to live mode UX

### R6: Advisor Page Enhancements

- Auto-refresh on configurable interval (matches background_interval or 30s default)
- Show "Last evaluated: {timestamp}" per instance
- Filter by status: Active / Resolved / Acknowledged / All
- Sort by: priority, timestamp, instance
- Badge count on sidebar "Advisor" item showing active recommendation count

---

## Acceptance Criteria

- [ ] Background evaluation runs every 5min (default) for all instances
- [ ] Advisor page populates automatically without manual Diagnose clicks
- [ ] Recommendations have timestamps and accumulate as history
- [ ] "Create Alert Rule" button opens pre-filled RuleFormModal
- [ ] Old recommendations cleaned up per retention_days
- [ ] Live mode unchanged — Diagnose-on-demand only
- [ ] Config section works: enabled/disabled, interval, retention
- [ ] Build clean: Go + TypeScript + lint

---

## Out of Scope

- Auto-promote (automatic alert rule creation) — future iteration
- Notification on new recommendations (email/Slack) — future iteration
- Recommendation severity escalation over time — future iteration
- `pg.server.multixact_pct` collector addition — tracked separately
