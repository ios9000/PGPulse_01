# M8_10 Hotfix Checklist

## Step 1: Copy and spawn

```bash
cd C:\Users\Archer\Projects\PGPulse_01
mkdir -p docs/iterations/M8_10_03102026_hotfix2
cp /path/to/M8_10_hotfix.md docs/iterations/M8_10_03102026_hotfix2/M8_10_hotfix.md
git add . && git commit -m "docs: M8_10 hotfix" && git push
claude --model claude-opus-4-6
```

Paste M8_10_hotfix.md contents.

## Step 2: Build verification

```bash
cd web && rm -rf dist && npm run build && npm run typecheck && npm run lint && cd ..
go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

## Step 3: Commit + redeploy

```bash
git add . && git commit -m "fix: explain handler, breadcrumb, replication/lock/progress scan errors" && git push
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/pgpulse-server ./cmd/pgpulse-server
scp build/pgpulse-server ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
```

On VM:
```bash
sudo systemctl stop pgpulse
sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server /opt/pgpulse/bin/pgpulse-server
sudo chmod +x /opt/pgpulse/bin/pgpulse-server
sudo systemctl start pgpulse
```

## Step 4: Verify each fix

```bash
# Check no scan errors
sudo journalctl -u pgpulse --no-pager -n 30 | grep -i "error"

# Test explain endpoint
TOKEN=$(curl -s http://localhost:8989/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"pgpulse_admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
curl -s http://localhost:8989/api/v1/instances/production-primary/explain -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"database":"demo_app","query":"SELECT 1","analyze":false,"buffers":false}' | python3 -m json.tool | head -20
```

Then in browser: test Explain Query page, Replication section, Lock Tree, Progress, breadcrumb.
