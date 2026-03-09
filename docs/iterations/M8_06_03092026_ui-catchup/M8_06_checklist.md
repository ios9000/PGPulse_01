# M8_06 Developer Checklist

---

## Step 1: Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M8_06_03092026_ui-catchup

cp /path/to/M8_06_requirements.md docs/iterations/M8_06_03092026_ui-catchup/M8_06_requirements.md
cp /path/to/M8_06_design.md docs/iterations/M8_06_03092026_ui-catchup/M8_06_design.md
cp /path/to/M8_06_team-prompt.md docs/iterations/M8_06_03092026_ui-catchup/M8_06_team-prompt.md
cp /path/to/M8_06_checklist.md docs/iterations/M8_06_03092026_ui-catchup/M8_06_checklist.md
```

## Step 2: Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` — set:

```
## Current Iteration
M8_06 — UI Catch-Up + Forecast Extension
See: docs/iterations/M8_06_03092026_ui-catchup/
```

## Step 3: Update Project Knowledge (if needed)

No Project Knowledge changes needed for M8_06 — purely frontend, no process changes.

## Step 4: Commit docs

```bash
git add docs/iterations/M8_06_03092026_ui-catchup/ .claude/CLAUDE.md
git commit -m "docs: add M8_06 requirements, design, team-prompt, checklist"
git push
```

## Step 5: Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `M8_06_team-prompt.md` into Claude Code.

**Team size: 2 specialists** (Frontend Agent + QA Agent) — all work is frontend-only,
no need for Collector Agent or API Agent.

## Step 6: Watch-list of expected files

| Action | File | Agent |
|--------|------|-------|
| CREATE | `web/src/components/ConfirmModal.tsx` | Frontend |
| CREATE | `web/src/components/SessionActions.tsx` | Frontend |
| CREATE | `web/src/components/SettingsDiff.tsx` | Frontend |
| CREATE | `web/src/components/QueryPlanViewer.tsx` | Frontend |
| CREATE | `web/src/components/PlanNode.tsx` | Frontend |
| CREATE | `web/src/hooks/useForecastChart.ts` | Frontend |
| MODIFY | `web/src/pages/ServerDetail.tsx` | Frontend |

## Step 7: Build verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

Expected:
- `npm run build` — success
- `npm run typecheck` — zero errors
- `npm run lint` — zero new errors (pre-existing `Administration.tsx` error is allowed)
- `go build` — success (confirms go:embed picks up new frontend build)
- `go test` — all existing tests pass (no Go changes in this iteration)

If errors: paste them back into Claude Code for fixes, then re-run.

## Step 8: Commit clean build

```bash
git add .
git commit -m "feat(web): add session kill UI, settings diff, query plan viewer, extend forecast overlay"
git push
```

## Step 9: Wrap-up

1. Return to Claude.ai
2. Produce `M8_06_session-log.md` with:
   - Goal, agent team config, agent activity summary
   - Files created/modified
   - Build verification results
   - Any decisions made during implementation
3. Update `docs/roadmap.md` — mark M8 milestone as complete
4. Update `docs/CHANGELOG.md` — add M8_06 features
5. Create `HANDOFF_M8_to_M9.md` (or next target milestone)
6. Create save point `SAVEPOINT_M8_03092026.md` (M8 milestone complete)
7. Final commit:

```bash
git add docs/
git commit -m "docs: add M8_06 session-log, update roadmap and changelog, M8 save point"
git push
```
