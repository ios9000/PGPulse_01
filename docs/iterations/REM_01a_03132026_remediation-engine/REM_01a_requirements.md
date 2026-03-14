# REM_01a — Rule-Based Remediation Engine (Backend)

**Iteration:** REM_01a
**Date:** 2026-03-13
**Scope:** Backend engine, database schema, API endpoints
**Follows:** MN_01 (commit 2f96bed)

---

## Goal

Add a rule-based remediation engine that maps detected anomalies and threshold
breaches to actionable recommendations. Inspired by pganalyze's deterministic
advisor model — no ML in this phase, pure expert-knowledge rules compiled into
the binary.

## Decisions (Locked)

| ID | Decision | Choice |
|----|----------|--------|
| D300 | Package location | New `internal/remediation/` (clean separation from alert/) |
| D301 | Recommendation attachment | Both: embedded field on alert events + separate DB table |
| D302 | Rule evaluation trigger | On alert fire + on-demand Diagnose API |
| D303 | Rule storage | Go code only (compiled-in, like builtin alert rules) |
| D304 | Priority model | Own priority: info / suggestion / action_required (decoupled from alert severity) |
| D305 | Frontend display | Dedicated Advisor page + alert detail panel (REM_01b) |
| D306 | Iteration split | REM_01a (backend + API) → REM_01b (frontend + Advisor page) |
| D307 | Live mode behavior | Partial: on-demand Diagnose works, auto-attachment disabled |
| D308 | Initial rule count | ~25 rules including OS metric scenarios |

## Functional Requirements

### FR-1: Remediation Rule Engine
- Compiled-in rules (Go code, no DB table for rule definitions)
- Each rule matches a metric key pattern and evaluates a condition
- Rules can reference multiple metric values (composite conditions for OS rules)
- Engine returns zero or more recommendations per evaluation
- Two evaluation modes:
  - **Alert-triggered**: metric key + value + severity → matching recommendations
  - **On-demand Diagnose**: full metric snapshot → all matching recommendations

### FR-2: Recommendation Persistence
- Recommendations stored in `remediation_recommendations` table
- Linked to alert events via optional `alert_event_id` FK
- Supports acknowledgment (user marks as reviewed)
- Retention cleanup (matches alert history retention)

### FR-3: API Endpoints
- `GET /instances/{id}/recommendations` — list recommendations for instance
- `POST /instances/{id}/diagnose` — on-demand rule evaluation
- `GET /recommendations` — fleet-wide listing (for Advisor page in REM_01b)
- `PUT /recommendations/{id}/acknowledge` — mark as acknowledged

### FR-4: Alert Integration
- When an alert fires, remediation engine evaluates and attaches recommendations
- Recommendations included in alert detail API response (embedded field)
- Recommendations included in alert notification emails

### FR-5: Live Mode Support
- `NullRecommendationStore` for live mode (no persistence)
- Diagnose endpoint works (queries MemoryStore for current metrics)
- Auto-attachment to alerts disabled (no history table in live mode)
- Fleet-wide listing returns empty in live mode

### FR-6: Seed Rules (~25)
- ~17 PostgreSQL metric rules covering all 20 builtin alert scenarios
- ~8 OS metric rules (CPU, memory, load, disk I/O)
- Each rule has: ID, priority, category, title, description, optional doc URL

## Non-Functional Requirements

- No new Go dependencies
- No import cycles: `internal/remediation` must NOT import `internal/alert`
- Interface-based integration: `internal/alert` defines a thin interface, `internal/remediation` implements it
- All rules must be unit-testable in isolation
- Migration must be idempotent (IF NOT EXISTS)
