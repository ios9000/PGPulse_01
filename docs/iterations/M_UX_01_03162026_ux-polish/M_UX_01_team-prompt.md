# M_UX_01 Team Prompt — Alert Detail Panel + Duration Fix + UX Polish

**Iteration:** M_UX_01
**Date:** 2026-03-16
**Agents:** 2 (Frontend Specialist, Backend/Data Specialist)

---

## CONTEXT

PGPulse is a PostgreSQL monitoring platform. This is a UX polish iteration before a stakeholder demo. Three issues:

1. **Alert click → redirects to server page instead of showing detail.** Should open an alert detail panel with metric description, threshold, remediation advice, and linked advisor recommendations.
2. **Duration strings show raw Go output** like "4h24m36.79747s". Should be "4h 25m".
3. **Several pages need CSS consistency polish** — alerts, advisor, settings diff, query insights.

**Existing infrastructure (already built, just not used by frontend):**
- `AlertEvent` struct has: `ID, RuleID, RuleName, InstanceID, Severity, Value, Threshold, Operator, Metric, Labels, FiredAt, ResolvedAt, Recommendations`
- `GET /api/v1/alerts/rules` returns rules with `Description` field
- `GET /api/v1/instances/{id}/recommendations` returns recommendations with `title, description, doc_url, priority, category, metric_key, metric_value, status`
- Remediation rules in `internal/remediation/rules_pg.go` and `rules_os.go` have rich descriptions and doc URLs

**Key pattern references:**
- `web/src/components/alerts/AlertRow.tsx` (105 lines) — current alert row component
- `web/src/pages/AlertsDashboard.tsx` (113 lines) — current alerts page
- `web/src/components/advisor/AdvisorRow.tsx` (131 lines) — recommendation display pattern
- `web/src/lib/formatters.ts` (72 lines) — existing formatters
- `web/src/hooks/useAlerts.ts` (85 lines) — existing alert hooks
- `web/src/hooks/useAlertRules.ts` (82 lines) — existing rule hooks

---

## DO NOT RE-DISCUSS

- No backend schema changes needed — all data already flows through the API.
- The metric description registry is a frontend-only static map. No API endpoint.
- Alert detail is a slide-out panel or expandable row, NOT a new page.
- Duration formatting fix goes in the shared `formatters.ts`, NOT per-component.
- Dark mode must work for all changes.

---

## TEAM STRUCTURE

### Agent 1 — Frontend Specialist
**Owns:** Alert detail panel, metric descriptions, page links, CSS polish

**Work order:**

#### Part A: Metric Description Registry

1. **`web/src/lib/metricDescriptions.ts`** — NEW file (~200 lines).
   A static `Record<string, MetricDescription>` map:
   ```typescript
   export interface MetricDescription {
     name: string;           // Human-readable name
     description: string;    // What this metric means
     significance: string;   // Why a DBA should care
     pageLink?: string;      // Relative page link pattern (e.g., "/servers/{id}/query-insights")
   }
   
   export const METRIC_DESCRIPTIONS: Record<string, MetricDescription> = {
     'pg.cache.hit_ratio': {
       name: 'Buffer Cache Hit Ratio',
       description: 'Percentage of page reads served from shared_buffers vs disk.',
       significance: 'Below 90% indicates insufficient shared_buffers or working set larger than RAM.',
     },
     'pg.connections.utilization_pct': {
       name: 'Connection Utilization',
       description: 'Current connections as a percentage of max_connections.',
       significance: 'Above 80% risks connection exhaustion. Consider pgBouncer or raising max_connections.',
     },
     'pg.extensions.pgss_fill_pct': {
       name: 'pg_stat_statements Fill Percentage',
       description: 'How full the pg_stat_statements hash table is (entries used / pg_stat_statements.max).',
       significance: 'Above 95% means new queries are evicting old entries. Increase pg_stat_statements.max or call pg_stat_statements_reset().',
       pageLink: '/servers/{id}/query-insights',
     },
     'pg.server.txid_wraparound_pct': {
       name: 'Transaction ID Wraparound',
       description: 'Percentage of available 32-bit transaction IDs consumed since last aggressive VACUUM.',
       significance: 'Above 50% is dangerous. PostgreSQL will shut down at 100% to prevent data corruption. Run VACUUM FREEZE on large tables.',
     },
     'pg.server.multixact_pct': {
       name: 'MultiXact ID Wraparound',
       description: 'Percentage of available multixact IDs consumed.',
       significance: 'Similar to txid wraparound. Requires VACUUM to prevent shutdown.',
     },
     'pg.replication.lag_bytes': {
       name: 'Replication Lag (bytes)',
       description: 'WAL bytes the replica is behind the primary.',
       significance: 'Large lag means replicas are falling behind. Check replica hardware, network, or long queries blocking replay.',
     },
     'pg.long_transactions.active_count': {
       name: 'Long Active Transactions',
       description: 'Number of transactions running longer than 1 minute.',
       significance: 'Long transactions hold locks, bloat tables, and prevent VACUUM from reclaiming space.',
     },
     'pg.long_transactions.oldest_seconds': {
       name: 'Oldest Transaction Age',
       description: 'Duration of the longest-running transaction in seconds.',
       significance: 'Transactions running for hours cause massive bloat and replication lag.',
     },
     'pg.locks.blocked_count': {
       name: 'Blocked Queries',
       description: 'Number of queries waiting to acquire a lock.',
       significance: 'Blocked queries indicate lock contention. Check the lock tree for the blocker.',
     },
     'os.cpu.user_pct': {
       name: 'CPU User Time',
       description: 'Percentage of CPU time spent in user-space processes.',
       significance: 'Sustained high CPU indicates query optimization needed or insufficient compute resources.',
     },
     'os.memory.available_kb': {
       name: 'Available Memory',
       description: 'Memory available for new allocations without swapping.',
       significance: 'Low available memory leads to OOM kills and swap thrashing.',
     },
     'os.disk.util_pct': {
       name: 'Disk Utilization',
       description: 'Percentage of time the disk was busy processing I/O.',
       significance: 'Sustained 90%+ means I/O bottleneck. Consider faster storage or reducing write amplification.',
     },
     'os.load.1m': {
       name: 'System Load (1m)',
       description: '1-minute average number of processes in runnable or uninterruptible state.',
       significance: 'Load above CPU count indicates CPU or I/O saturation.',
     },
     // Add entries for ALL metrics referenced in internal/alert/rules.go
     // Use grep to find all metric keys: grep -oP "'[a-z]+\.[a-z._]+'" internal/alert/rules.go
   };
   
   export function getMetricDescription(metricKey: string): MetricDescription | null {
     return METRIC_DESCRIPTIONS[metricKey] || null;
   }
   
   // Build page link with instance ID substitution
   export function getMetricPageLink(metricKey: string, instanceId: string): string | null {
     const desc = METRIC_DESCRIPTIONS[metricKey];
     if (!desc?.pageLink) return null;
     return desc.pageLink.replace('{id}', instanceId);
   }
   ```

   **IMPORTANT:** Cover ALL metric keys used in alert rules. Check `internal/alert/rules.go` for the complete list. If there are metric keys in rules not listed above, add them. Common patterns:
   - `pg.connections.*` → Connection management advice
   - `pg.cache.*` → shared_buffers tuning
   - `pg.replication.*` → Replication health
   - `pg.server.*` → Wraparound, uptime
   - `pg.statements.*` → PGSS health, link to Query Insights page
   - `pg.locks.*` → Lock contention
   - `pg.long_transactions.*` → Long transaction cleanup
   - `os.*` → OS resource management

#### Part B: Alert Detail Panel

2. **`web/src/components/alerts/AlertDetailPanel.tsx`** — NEW file (~220 lines).
   Props: `alert: AlertEvent`, `onClose: () => void`, `rules: AlertRule[]`
   
   **Layout:**
   ```
   ┌─────────────────────────────────────────────────┐
   │  ✕ Close                                         │
   ├─────────────────────────────────────────────────┤
   │  🔴 CRITICAL: Buffer Cache Hit Ratio             │
   │  Rule: cache_hit_ratio_low                       │
   │  Instance: production-primary                    │
   │  Fired: 5 minutes ago                            │
   ├─────────────────────────────────────────────────┤
   │  📊 Metric Details                               │
   │  Key: pg.cache.hit_ratio                         │
   │  Current Value: 85.2%                            │
   │  Threshold: < 90%                                │
   │                                                   │
   │  ℹ What is this?                                 │
   │  Percentage of page reads served from            │
   │  shared_buffers vs disk.                          │
   │                                                   │
   │  ⚠ Why it matters                               │
   │  Below 90% indicates insufficient shared_buffers │
   │  or working set larger than RAM.                 │
   ├─────────────────────────────────────────────────┤
   │  📋 Recommendation                               │
   │  (from linked advisor recommendation, if any)    │
   │  Title: Increase shared_buffers                  │
   │  Description: Current hit ratio is below 90%...  │
   │  [View in Advisor →]                             │
   ├─────────────────────────────────────────────────┤
   │  🔗 Quick Links                                  │
   │  [View Server Dashboard →]                       │
   │  [View Query Insights →]  (if metric has link)   │
   │  [View Alert Rules →]                            │
   └─────────────────────────────────────────────────┘
   ```
   
   **Data sources:**
   - `alert` prop provides: metric key, value, threshold, severity, instance_id, rule_id, recommendations
   - Match `alert.rule_id` against `rules` array to get Rule.description (the rule's own description)
   - Use `getMetricDescription(alert.metric)` for human-readable metric info
   - Use `getMetricPageLink(alert.metric, alert.instance_id)` for relevant page link
   - Alert.recommendations (already on the AlertEvent) shows linked advisor recommendations
   
   **Styling:**
   - Slide-in panel from right side (like a drawer), or expandable section below the alert row
   - Severity color accent: red for critical, amber for warning, blue for info
   - Dark mode support
   - Sections with subtle dividers
   - Close button (X) and click-outside-to-close

3. **Modify `web/src/components/alerts/AlertRow.tsx`** — Change click behavior:
   - REMOVE: navigation to server page on click
   - ADD: `onClick` prop that calls parent's `setSelectedAlert(alert)`
   - Make the row visually clickable (cursor-pointer, hover effect)
   - Keep severity badge, metric key, value, time ago display

4. **Modify `web/src/pages/AlertsDashboard.tsx`** — Wire up the detail panel:
   - Add state: `selectedAlert: AlertEvent | null`
   - Load rules via `useAlertRules()` (already available)
   - When `selectedAlert` is set, render `AlertDetailPanel` as a side panel or overlay
   - Clear selection when panel closes

#### Part C: Duration Formatting Fix

5. **Modify `web/src/lib/formatters.ts`** — Add or fix duration formatting:
   ```typescript
   // Format Go duration string "4h24m36.79747s" → "4h 25m"
   // Also handle: "30m0s" → "30m", "5m30s" → "5m 30s", "45.123s" → "45s"
   export function formatDurationHuman(goStr: string): string {
     // Parse hours, minutes, seconds from Go duration string
     // Regex: /(?:(\d+)h)?(?:(\d+)m)?(?:([\d.]+)s)?/
     // Rules:
     //   - If hours present: show "Xh Ym" (round minutes)
     //   - If only minutes: show "Xm" or "Xm Ys"
     //   - If only seconds: show "Xs" (integer)
     //   - Drop sub-second precision always
   }
   ```
   
   Also check all components that display durations and ensure they use this formatter:
   - WorkloadReport summary card (the "Period" field)
   - DiffResult.duration in Query Insights
   - Alert fired/resolved timestamps (these likely use relative time, which is fine)
   - Snapshot captured_at (these use date formatting, which is fine)
   
   Search for: `duration` in all .tsx files to find all display points.

#### Part D: CSS Polish (targeted pages)

6. **`web/src/pages/AlertsDashboard.tsx`** — Polish:
   - Add consistent max-w-7xl mx-auto container
   - Improve filter bar spacing
   - Add subtle card/border around alert list
   - Better empty state when no alerts

7. **`web/src/components/alerts/AlertRow.tsx`** — Polish:
   - Better padding (py-3 px-4)
   - Severity indicator as left border color (4px solid red/amber/blue)
   - Right-align timestamp
   - Show rule name alongside metric key
   - Show "Rule: {rule_name}" instead of empty Rule field

8. **`web/src/pages/Advisor.tsx`** — Polish:
   - Add max-w-7xl mx-auto
   - Consistent spacing with other pages
   - Better filter bar alignment

9. **`web/src/components/advisor/AdvisorRow.tsx`** — Polish:
   - Consistent row styling with AlertRow
   - Left border color by priority
   - Better padding

10. **`web/src/pages/QueryInsights.tsx`** — Minor polish:
    - Ensure empty state looks good
    - Consistent section spacing

11. **`web/src/components/snapshots/DiffTable.tsx`** — Check if M11_02b fixes are applied and look good. If not, apply:
    - Alternating rows
    - Sticky header
    - Right-aligned numbers
    - Query text wrapping

### Agent 2 — Backend/Data Specialist
**Owns:** Ensure API returns all needed data, verify alert→recommendation linkage

1. **Verify `GET /api/v1/alerts` and `GET /api/v1/alerts/history` responses include full data:**
   - Check that `AlertEvent` JSON includes: `rule_id`, `rule_name`, `metric`, `value`, `threshold`, `operator`, `severity`, `instance_id`, `labels`, `fired_at`, `recommendations`
   - If `recommendations` is null/empty on alert events that should have linked recommendations, investigate the linkage in the alert evaluator or dispatcher

2. **Verify `GET /api/v1/alerts/rules` returns `description` field:**
   - Check that the built-in rules in `internal/alert/rules.go` have meaningful descriptions
   - If descriptions are empty or generic, update them with DBA-actionable text (similar to remediation rule descriptions)
   - Example: "Buffer cache hit ratio below threshold" → "Buffer cache hit ratio dropped below {threshold}%. This indicates the working set exceeds shared_buffers. Consider increasing shared_buffers or investigating queries with high shared_blks_read."

3. **Update alert rule descriptions in `internal/alert/rules.go`:**
   Check each rule's `Description` field. If empty or generic, update to include:
   - What the metric measures
   - Why the threshold matters
   - A brief remediation hint
   
   Pattern from the existing good descriptions in `remediation/rules_pg.go` — mirror that quality level for alert rules.

4. **Check duration formatting on API responses:**
   - Search for `time.Duration.String()` usage in API responses
   - If any handler returns raw Go duration strings, consider formatting them server-side too
   - Check `DiffResult.Duration` — if it's a Go `time.Duration` serialized as string, verify the format

5. **Build verification:**
   ```bash
   cd web && npm run build && npm run lint && npm run typecheck
   cd .. && go build ./cmd/pgpulse-server
   go test ./cmd/... ./internal/... -count=1
   golangci-lint run ./cmd/... ./internal/...
   ```

---

## BUILD VERIFICATION COMMAND

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

---

## EXPECTED OUTPUT FILES

### New Files
- `web/src/lib/metricDescriptions.ts` (~200 lines)
- `web/src/components/alerts/AlertDetailPanel.tsx` (~220 lines)

### Modified Files (Agent 1)
- `web/src/components/alerts/AlertRow.tsx` — click behavior + styling
- `web/src/pages/AlertsDashboard.tsx` — wire detail panel + layout
- `web/src/lib/formatters.ts` — duration formatting
- `web/src/pages/Advisor.tsx` — CSS polish
- `web/src/components/advisor/AdvisorRow.tsx` — CSS polish
- `web/src/pages/QueryInsights.tsx` — CSS polish

### Modified Files (Agent 2)
- `internal/alert/rules.go` — update rule descriptions
- Possibly `internal/api/alerts.go` — ensure full data in responses

---

## COORDINATION NOTES

- **Agent 1 and Agent 2 are independent.** No file conflicts.
- **Agent 2 should start by verifying API responses** — if descriptions are already rich, minimal changes needed.
- **Commit order:** Agent 2 (backend description updates) → Agent 1 (frontend) → build verification.
