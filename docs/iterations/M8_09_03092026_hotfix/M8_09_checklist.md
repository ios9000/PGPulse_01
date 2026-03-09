# M8_09 Hotfix Checklist

## Step 1: Copy to iteration folder and spawn

```bash
cd C:\Users\Archer\Projects\PGPulse_01
mkdir -p docs/iterations/M8_09_03092026_hotfix
cp /path/to/M8_09_hotfix.md docs/iterations/M8_09_03092026_hotfix/M8_09_hotfix.md
```

Update CLAUDE.md:
```
## Current Iteration
M8_09 — HOTFIX: production crash + collector bugs
See: docs/iterations/M8_09_03092026_hotfix/
```

```bash
git add . && git commit -m "docs: M8_09 hotfix" && git push
claude --model claude-opus-4-6
```

Paste contents of M8_09_hotfix.md.

## Step 2: Build verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

## Step 3: Commit + redeploy

```bash
git add . && git commit -m "fix: production crash, collector PG16 compat, CSP, port config" && git push

# Rebuild for Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/pgpulse-server ./cmd/pgpulse-server

# Upload to VM
scp build/pgpulse-server ml4dbs@185.159.111.139:/tmp/

# SSH to VM and deploy
ssh ml4dbs@185.159.111.139
sudo cp /tmp/pgpulse-server /opt/pgpulse/bin/pgpulse-server
sudo chmod +x /opt/pgpulse/bin/pgpulse-server
sudo systemctl restart pgpulse
sudo journalctl -u pgpulse --no-pager -n 20
```

## Step 4: Verify in browser

- Fleet Overview loads with 3 instance cards
- Server Detail loads for all 3 instances
- No CSP errors in console
- No 404s on statements endpoint
- Collector warnings gone from journalctl
