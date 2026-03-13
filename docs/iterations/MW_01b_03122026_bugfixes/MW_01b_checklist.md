# MW_01b — Developer Checklist

## 1. Copy docs to iteration folder
```bash
cd C:\Users\Archer\Projects\PGPulse_01
mkdir -p docs/iterations/MW_01b_03122026_bugfixes
cp MW_01b_team-prompt.md docs/iterations/MW_01b_03122026_bugfixes/MW_01b_team-prompt.md
cp MW_01b_checklist.md docs/iterations/MW_01b_03122026_bugfixes/MW_01b_checklist.md
```

## 2. Update CLAUDE.md current iteration
Set: `Iteration: MW_01b`, `Next planned work: Metric naming standardization`

## 3. Update Project Knowledge if needed
Upload `PGPulse_Competitive_Research_Synthesis.md` to Project Knowledge (produced this session).

## 4. Commit docs
```bash
git add docs/iterations/MW_01b_03122026_bugfixes/
git commit -m "docs(MW_01b): bugfix team prompt and checklist"
```

## 5. Spawn agent
```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```
Paste the MW_01b_team-prompt.md content.

## 6. Watch-list of expected file changes
- `cmd/pgpulse-server/main.go` — default port fix
- `scripts/build-release.sh` — PowerShell fallback
- `internal/collector/cache.go` OR `web/src/...` — cache hit metric key alignment
- `web/src/...` — replication lag unit formatting fix
- Optional: `web/src/...` — instance display name cosmetic

## 7. Build verification
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
```

## 8. Commit clean build
```bash
git add -A
git status  # review changes
# Individual commits per bug (see team-prompt for commit messages)
git push origin main
```

## 9. Deploy to demo VM
```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

## 10. Verify on demo VM
Open http://185.159.111.139:8989 and confirm:
- [ ] Cache Hit Ratio stat card shows a percentage value
- [ ] Cache Hit Ratio chart shows time-series data
- [ ] Replication Lag chart Y-axis shows proper unit labels (B, KB, MB)
- [ ] All other charts still work

## 11. Wrap-up
- [ ] Write `MW_01b_session-log.md` in the iteration folder
- [ ] Update handoff: remove fixed items from Known Issues
- [ ] Update `docs/CODEBASE_DIGEST.md` (re-run generation if metric keys changed)
- [ ] Upload updated CODEBASE_DIGEST.md to Project Knowledge
- [ ] Update roadmap: MW_01b done, next = metric naming standardization
