# PGPulse Demo Deployment

## Quick Start

From your Windows dev machine:

```bash
# 1. Build and package
make demo-package

# 2. Deploy (one command)
make deploy-demo VM=ubuntu@your-vm-ip

# Or manually:
scp build/pgpulse-demo.tar.gz ubuntu@vm:/tmp/
ssh ubuntu@vm
cd /tmp && tar xzf pgpulse-demo.tar.gz
sudo bash pgpulse-demo/provision.sh pgpulse-demo/pgpulse-server
```

## What Gets Created

```
Ubuntu 24 VM
├── PostgreSQL 16
│   ├── :5432  "Production Primary"     ← healthy, streaming to replica
│   ├── :5433  "Production Replica"     ← streaming replica, WAL replay
│   └── :5434  "Staging (Chaos)"        ← standalone, chaos target
│
├── Replication
│   ├── Physical: :5432 → :5433 (streaming)
│   └── Logical:  :5432.demo_orders → :5434.demo_orders
│
├── PGPulse Server
│   ├── Web UI:  http://vm-ip:8989
│   ├── Login:   admin / pgpulse_admin
│   ├── Config:  /opt/pgpulse/configs/pgpulse.yml
│   └── Service: systemctl {start|stop|restart} pgpulse
│
├── Demo Data
│   ├── pgbench tables (scale 10) on chaos instance
│   ├── demo_large (500k rows) for cache thrash
│   ├── demo_queries (200k rows) with droppable index
│   └── demo_orders (5k rows) replicated primary → chaos
│
└── Chaos Scripts: /opt/pgpulse/chaos/
```

## Chaos Scripts

Each script simulates one problem. Run it, watch PGPulse react, then stop it.

| Script | What It Does | PGPulse Shows |
|--------|-------------|---------------|
| `long-transaction.sh` | Holds a transaction open 10min | Long Transactions table (amber/red) |
| `lock-contention.sh` | 1 blocker + 2 waiters | Lock Tree with blocking hierarchy |
| `connection-flood.sh` | Opens 90 idle connections | Connection gauge amber→red, alert |
| `bloat-generator.sh` | Updates all rows, disables vacuum | Bloat ratio, vacuum need, red warnings |
| `replication-lag.sh` | Pauses WAL replay on replica | Replication lag chart spikes |
| `cache-thrash.sh` | Sequential scans evict cache | Cache hit ratio drops below 90% |
| `settings-drift.sh` | ALTER SYSTEM on 5 settings | Settings Timeline diff, cross-instance diff |
| `query-regression.sh` | Drops index → plan changes | Plan History regressions tab |
| `logical-repl-pause.sh` | Disables subscription | Logical Replication pending tables |
| `cleanup-all.sh` | Reverses everything at once | Metrics return to normal |

### Usage Pattern

```bash
# Start a scenario
sudo /opt/pgpulse/chaos/long-transaction.sh

# Watch PGPulse detect it (10-60 seconds)
# Open http://vm-ip:8989 → Server Detail → Staging (Chaos)

# Stop the scenario
sudo /opt/pgpulse/chaos/long-transaction.sh stop

# Or clean up everything at once
sudo /opt/pgpulse/chaos/cleanup-all.sh
```

### Demo Flow (Suggested Order)

1. **Start clean** — show fleet overview with 3 healthy instances
2. **Connection flood** — watch gauge go red, alert fires
3. **Lock contention** — show the lock tree visualization
4. **Long transaction** — amber→red transition in real time
5. **Replication lag** — pause replay, watch lag chart climb
6. **Query regression** — drop index, show Plan History regressions tab
7. **Settings drift** — change settings, compare in Settings Timeline
8. **Bloat** — show per-DB analysis with high bloat ratios
9. **Cleanup** — run cleanup-all.sh, watch everything recover
10. **Forecast** — if ML has had 30+ min, show confidence bands on charts

## Managing the Demo

```bash
# PGPulse service
sudo systemctl status pgpulse
sudo systemctl restart pgpulse
sudo journalctl -u pgpulse -f        # live logs

# PostgreSQL clusters
pg_lsclusters                         # list all clusters
sudo pg_ctlcluster 16 main restart    # restart primary
sudo pg_ctlcluster 16 replica restart # restart replica
sudo pg_ctlcluster 16 chaos restart   # restart chaos

# Connect to instances
sudo -u postgres psql -p 5432         # primary
sudo -u postgres psql -p 5433         # replica (read-only)
sudo -u postgres psql -p 5434         # chaos
```

## Firewall

If the VM has a firewall, open port 8989 for PGPulse:

```bash
sudo ufw allow 8989/tcp
# Or for cloud providers, add an inbound rule for TCP 8989
```

## Security Notes

This is a **demo environment** with simple passwords. For production:
- Change all passwords in provision.sh before running
- Use TLS for PGPulse (configure in pgpulse.yml)
- Restrict PostgreSQL listen_addresses
- Use certificate-based auth for replication
- Don't expose port 8989 to the public internet without a reverse proxy
