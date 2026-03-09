# M8_08 Developer Checklist

---

## Step 1: Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M8_08_03092026_logical-repl

cp /path/to/M8_08_requirements.md docs/iterations/M8_08_03092026_logical-repl/M8_08_requirements.md
cp /path/to/M8_08_design.md docs/iterations/M8_08_03092026_logical-repl/M8_08_design.md
cp /path/to/M8_08_team-prompt.md docs/iterations/M8_08_03092026_logical-repl/M8_08_team-prompt.md
cp /path/to/M8_08_checklist.md docs/iterations/M8_08_03092026_logical-repl/M8_08_checklist.md
```

## Step 2: Update CLAUDE.md current iteration

```
## Current Iteration
M8_08 — Logical Replication Monitoring (sub-collector + API + frontend)
See: docs/iterations/M8_08_03092026_logical-repl/
```

## Step 3: Commit docs

```bash
git add docs/iterations/M8_08_03092026_logical-repl/ .claude/CLAUDE.md
git commit -m "docs: add M8_08 requirements, design, team-prompt, checklist"
git push
```

## Step 4: Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `M8_08_team-prompt.md` into Claude Code.

**Team size: 3 specialists** (Collector Agent + Frontend Agent + QA Agent).

## Step 5: Watch-list of expected files

| Action | File | Agent |
|--------|------|-------|
| MODIFY | `internal/collector/database.go` — add logical repl sub-collector | Collector |
| CREATE | `internal/api/logical_replication.go` — handler + response structs | Collector |
| MODIFY | `internal/api/server.go` — register route | Collector |
| MODIFY | `internal/alert/rules.go` — seed new alert rule | Collector |
| CREATE | `web/src/hooks/useLogicalReplication.ts` | Frontend |
| CREATE | `web/src/components/LogicalReplicationSection.tsx` | Frontend |
| MODIFY | `web/src/pages/ServerDetail.tsx` — add section | Frontend |

## Step 6: Build verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

Expected:
- All checks pass
- 0 lint errors (both Go and TypeScript)

## Step 7: Commit clean build

```bash
git add .
git commit -m "feat(collector/api/web): logical replication monitoring"
git push
```

## Step 8: Wrap-up

1. Produce `M8_08_session-log.md`
2. Update `docs/roadmap.md` — add M8_08 sub-iteration, note Q41 ported
3. Update `docs/CHANGELOG.md`
4. Update query porting progress: Q41 now ported, update total from ~69 to ~70
5. Update save point (LATEST.md) with M8_07 + M8_08 additions
6. Create handoff for M9 (if ready to start next milestone)

```bash
git add docs/
git commit -m "docs: M8_07 + M8_08 session-logs, update roadmap, changelog, save point"
git push
```
