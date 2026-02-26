# Session: 2026-02-26 — M1_04 pg_stat_statements Collectors

## Goal
Port PGAM queries Q48–Q51 covering pg_stat_statements monitoring: settings/fill%/reset age and top-N query ranking by IO and CPU time.

## Agent Configuration
- **Mode:** Single Claude Code session (Sonnet) — not Agent Teams
- **Rationale:** 2 collectors + 2 test files + 1 helper. No version gates. Agent Teams overhead not justified.

## PGAM Queries Ported

| Query # | Description | Target Function | Notes |
|---------|-------------|-----------------|-------|
| Q48 | pgss settings + fill% | statements_config.go | 3 settings + count + derived fill_pct |
| Q49 | Stats reset age | statements_config.go | pg_stat_statements_info (always available PG 14+) |
| Q50 | Top queries by IO time | statements_top.go | Combined with Q51 into single query |
| Q51 | Top queries by CPU time | statements_top.go | Combined with Q50 into single query |

**Q52 (normalized text report):** Deliberately deferred to M2/API layer. PG 14+ queryid provides native normalization — PGAM's PHP regex approach is unnecessary. The per-queryid metrics from StatementsTopCollector already capture the data; the formatted report is a presentation concern.

## Files Created/Modified

| File | Action | Lines (approx) |
|------|--------|----------------|
| `internal/collector/base.go` | Modified | +15 (pgssAvailable helper) |
| `internal/collector/statements_config.go` | New | StatementsConfigCollector |
| `internal/collector/statements_top.go` | New | StatementsTopCollector |
| `internal/collector/statements_config_test.go` | New | 5 tests |
| `internal/collector/statements_top_test.go` | New | 6 tests |

## Architecture Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | pgssAvailable() as package-level helper in base.go | Loose coupling — each statements collector checks independently rather than threading pgss state through InstanceContext |
| D2 | Q50+Q51 combined into single SQL query | Single pgss scan computes both IO and CPU columns; halves work vs PGAM's two-query approach |
| D3 | Fixed LIMIT 20 instead of PGAM's 0.5% threshold | Predictable cardinality for metric labels; threshold was a rendering optimization |
| D4 | "Other" bucket via Go subtraction from CROSS JOIN totals | Single round-trip; no second query needed |
| D5 | queryid as label, not query text | Avoids cardinality bomb; API layer resolves text at read time |
| D6 | Q52 deferred to M2/API | queryid handles normalization natively in PG 14+; text report is presentation |
| D7 | Single Sonnet session, not Agent Teams | Right-sized for 2-file scope |

## PGAM Bugs Fixed

| Bug | Fix |
|-----|-----|
| Q48: No pgss availability check — crashes if extension missing | pgssAvailable() returns nil, nil |
| Q50/Q51: Two separate queries scanning pgss twice | Single query with both IO and CPU columns |
| Q50/Q51: 0.5% threshold → variable/unpredictable result set | Fixed LIMIT 20 |
| Q50/Q51: No guard against division by zero (calls=0) | WHERE calls > 0 filter |
| Q52: PHP regex normalization of query text | Unnecessary — PG 14+ queryid handles this |

## Build & Test Results

| Check | Result |
|-------|--------|
| go build ./... | ✅ Pass |
| go vet ./... | ✅ Pass |
| golangci-lint run | ✅ 0 issues |
| Statements unit tests | ✅ 9 pass, 2 skip (integration) |
| Full collector suite | ✅ All pass |

## Query Porting Running Total
- Before M1_04: 24/76 PGAM + 2 new
- After M1_04: **28/76 PGAM + 2 new** (+4 queries: Q48, Q49, Q50, Q51)

## Not Done / Next Iteration
- [ ] M1_05: Locks & wait events (Q53–Q58)
- [ ] Q52 text report → deferred to M2/API
