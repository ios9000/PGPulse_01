# M8_07 Developer Checklist

---

## Step 1: Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M8_07_03092026_deferred-ui

cp /path/to/M8_07_requirements.md docs/iterations/M8_07_03092026_deferred-ui/M8_07_requirements.md
cp /path/to/M8_07_design.md docs/iterations/M8_07_03092026_deferred-ui/M8_07_design.md
cp /path/to/M8_07_team-prompt.md docs/iterations/M8_07_03092026_deferred-ui/M8_07_team-prompt.md
cp /path/to/M8_07_checklist.md docs/iterations/M8_07_03092026_deferred-ui/M8_07_checklist.md
```

## Step 2: Update CLAUDE.md current iteration

```
## Current Iteration
M8_07 — Deferred UI + Small Fixes (plan history, settings timeline, app_name, lint fix)
See: docs/iterations/M8_07_03092026_deferred-ui/
```

## Step 3: Update Project Knowledge (if needed)

No Project Knowledge changes needed.

## Step 4: Commit docs

```bash
git add docs/iterations/M8_07_03092026_deferred-ui/ .claude/CLAUDE.md
git commit -m "docs: add M8_07 requirements, design, team-prompt, checklist"
git push
```

## Step 5: Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `M8_07_team-prompt.md` into Claude Code.

**Team size: 2 specialists** (Frontend Agent + QA Agent).

## Step 6: Watch-list of expected files

| Action | File | Agent |
|--------|------|-------|
| VERIFY/MODIFY | `internal/api/server.go` — ensure plan + settings routes registered | Frontend |
| MODIFY | Long transactions struct/query — add application_name | Frontend |
| MODIFY | `web/src/pages/Administration.tsx` — lint fix | Frontend |
| CREATE | `web/src/hooks/usePlanHistory.ts` | Frontend |
| CREATE | `web/src/components/PlanHistory.tsx` | Frontend |
| CREATE | `web/src/hooks/useSettingsTimeline.ts` | Frontend |
| CREATE | `web/src/components/SettingsTimeline.tsx` | Frontend |
| MODIFY | `web/src/pages/ServerDetail.tsx` — add two new tabs | Frontend |
| MODIFY | `web/src/components/LongTransactionsTable.tsx` — pass application_name | Frontend |

## Step 7: Build verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

Expected:
- `npm run lint` — **0 errors** (Administration.tsx fix eliminates the last one)
- `npm run typecheck` — 0 errors
- `npm run build` — success
- `go build` — success
- `go test` — all pass

## Step 8: Commit clean build

```bash
git add .
git commit -m "feat(web): plan history UI, settings timeline UI, app_name enrichment, lint fix"
git push
```

## Step 9: Wrap-up

1. Produce `M8_07_session-log.md`
2. Update `docs/roadmap.md` — add M8_07 sub-iteration
3. Update `docs/CHANGELOG.md`
4. Proceed directly to M8_08 (logical replication) if time permits, or wrap and handoff
