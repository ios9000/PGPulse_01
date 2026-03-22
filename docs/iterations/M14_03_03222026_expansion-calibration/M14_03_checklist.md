# M14_03 — Developer Checklist

**Iteration:** M14_03
**Date:** 2026-03-22

---

## Step 1: Copy Docs to Iteration Folder

```bash
cd ~/Projects/PGPulse_01

# Create iteration folder
mkdir -p docs/iterations/M14_03_03222026_expansion-calibration

# Copy all planning docs
cp /path/to/M14_03_requirements.md docs/iterations/M14_03_03222026_expansion-calibration/M14_03_requirements.md
cp /path/to/M14_03_design.md docs/iterations/M14_03_03222026_expansion-calibration/M14_03_design.md
cp /path/to/M14_03_team-prompt.md docs/iterations/M14_03_03222026_expansion-calibration/M14_03_team-prompt.md
cp /path/to/M14_03_checklist.md docs/iterations/M14_03_03222026_expansion-calibration/M14_03_checklist.md
cp /path/to/D506_recommendation_schema_evolution.md docs/iterations/M14_03_03222026_expansion-calibration/D506_recommendation_schema_evolution.md
cp /path/to/Architecture_Decision_Record_RCA_-_Adviser_Bridge__M14_03_.txt docs/iterations/M14_03_03222026_expansion-calibration/M14_03_ADR_rca_adviser_bridge.txt
```

## Step 2: Update CLAUDE.md

Set current iteration to M14_03:

```bash
# Edit CLAUDE.md — update the "Current State" section:
# - Milestone: M14
# - Iteration: M14_03
# - Last completed feature: M14_02 (RCA UI)
# - Next planned work: M14_03 (Expansion, Calibration, Knowledge Integration)
```

## Step 3: Update Project Knowledge

Upload to Claude.ai Project Knowledge:
- [ ] `M14_03_requirements.md`
- [ ] `M14_03_design.md`
- [ ] `D506_recommendation_schema_evolution.md`
- [ ] Updated `CODEBASE_DIGEST.md` (if regenerated after M14_02)

## Step 4: Commit Planning Docs

```bash
cd ~/Projects/PGPulse_01
git add docs/iterations/M14_03_03222026_expansion-calibration/
git add CLAUDE.md
git commit -m "docs: M14_03 planning — expansion, calibration, knowledge integration

- Requirements (W1-W10), design, team-prompt, checklist
- D506 recommendation schema evolution spec
- ADR: RCA→Adviser bridge architecture
- Decisions D500-D508, Q1-Q3 locked"
```

## Step 5: Pre-Flight Greps (MANDATORY before spawning agents)

Run these greps to verify structural assumptions. Record findings in corrections doc.

```bash
cd ~/Projects/PGPulse_01

# 5.1 — Chain definitions: MetricKeys for all 20 chains
grep -n "MetricKeys" internal/rca/chains.go

# 5.2 — Tier B filter location
grep -rn "TierB\|Tier B\|tierB\|tier_b" internal/rca/

# 5.3 — WhileEffective stub location
grep -n "WhileEffective" internal/rca/graph.go

# 5.4 — Statement diff DiffEntry shape
grep -n "type DiffEntry\|type Diff " internal/statements/diff.go
grep -n "MeanExecTime\|Regression\|NewQuery" internal/statements/diff.go

# 5.5 — Existing Recommendation struct
grep -A 20 "type Recommendation struct" internal/remediation/rule.go

# 5.6 — Existing RecommendationStore interface
grep -A 15 "type RecommendationStore interface" internal/remediation/store.go

# 5.7 — Existing Write() method signature and column list
grep -A 30 "func.*PGRecommendationStore.*Write" internal/remediation/pgstore.go

# 5.8 — Existing ListAll/ListByInstance column scanning
grep -n "Scan\|scan" internal/remediation/pgstore.go | head -20

# 5.9 — RCA Engine constructor
grep -n "func New\|func.*Engine.*{" internal/rca/engine.go | head -10

# 5.10 — RCA Engine Analyze method — find where to inject statement source + EvaluateHook
grep -n "func.*Engine.*Analyze" internal/rca/engine.go
grep -n "anomal\|detect\|step 4\|step.*4" internal/rca/engine.go

# 5.11 — RemediationHook usage in chains
grep -n "RemediationHook" internal/rca/chains.go | head -20

# 5.12 — Hook constants in ontology
grep -n "Hook\|hook" internal/rca/ontology.go

# 5.13 — Remediation rule IDs (to map hooks to rules)
grep -n "RuleID\|rule_id\|ruleID" internal/remediation/rules_pg.go | head -20
grep -n "RuleID\|rule_id\|ruleID" internal/remediation/rules_os.go | head -10

# 5.14 — Migration 016 schema — confirm review_status exists
grep -n "review_status\|review_notes" migrations/016_*.sql

# 5.15 — Frontend Recommendation type location
grep -rn "interface Recommendation" web/src/types/

# 5.16 — Frontend RCA type field names (current PascalCase)
grep -n "ID\|Name\|MetricKeys\|FromNode\|ToNode" web/src/types/rca.ts

# 5.17 — CausalGraphView field access patterns
grep -n "\.\(ID\|Name\|MetricKeys\|FromNode\|ToNode\|Nodes\|Edges\)" web/src/components/rca/CausalGraphView.tsx

# 5.18 — SettingsSnapshot store interface
grep -A 10 "type.*SnapshotStore\|type.*Store interface" internal/settings/store.go

# 5.19 — Existing config structs for RCA and Remediation
grep -A 15 "type RCAConfig struct" internal/rca/config.go
grep -A 10 "type RemediationConfig\|Remediation.*struct" internal/config/config.go

# 5.20 — Current ML config in demo YAML (if local copy exists)
grep -A 30 "^ml:" pgpulse.yml 2>/dev/null || echo "No local pgpulse.yml"

# 5.21 — Incident struct (check for ReviewStatus field)
grep -A 30 "type Incident struct" internal/rca/incident.go

# 5.22 — RCA API handler routes
grep -n "rca\|RCA" internal/api/server.go

# 5.23 — Adviser page current sort order
grep -n "sort\|order\|Order\|Sort" web/src/pages/Advisor.tsx
grep -n "sort\|order\|Order\|Sort" web/src/hooks/useRecommendations.ts

# 5.24 — InstanceConnProvider usage in RCA
grep -rn "ConnFor\|connProv\|InstanceConnProvider" internal/rca/
```

## Step 6: Write Corrections Doc

Based on grep findings, create `M14_03_corrections.md`:

```bash
# Record all mismatches, surprises, and adjustments needed
# Template:
# | Grep # | Finding | Impact | Action |
# |--------|---------|--------|--------|
# | 5.1    | Chain 3 MetricKeys = [...] | Need to verify collector exists | Check catalog |
# | ...    | ...     | ...    | ...    |
```

Save to iteration folder:
```bash
cp M14_03_corrections.md docs/iterations/M14_03_03222026_expansion-calibration/
git add docs/iterations/M14_03_03222026_expansion-calibration/M14_03_corrections.md
git commit -m "docs: M14_03 pre-flight corrections"
```

## Step 7: Spawn Agents

```bash
cd ~/Projects/PGPulse_01

# Close all other CLI terminals to avoid OOM
# Then spawn:
claude --model claude-opus-4-6
```

Paste the team-prompt content. Monitor for:
- [ ] Agent reads CODEBASE_DIGEST.md, design.md, requirements.md, corrections.md
- [ ] Agent 1 starts Phase 1 tasks (W2, W3, W4, W9-Go) in parallel
- [ ] Agent 2 starts Phase 1 tasks (W9-TS, W8 types) in parallel
- [ ] No agent relitigates locked decisions

## Step 8: Watch List — Expected Files

New files that MUST appear:
- [ ] `internal/rca/settings.go`
- [ ] `internal/rca/settings_adapter.go`
- [ ] `internal/rca/settings_adapter_test.go`
- [ ] `internal/rca/statement_source.go`
- [ ] `internal/rca/statement_source_test.go`
- [ ] `internal/remediation/hooks.go`
- [ ] `internal/remediation/urgency.go`
- [ ] `migrations/017_recommendation_rca_bridge.sql`
- [ ] `web/src/components/advisor/RCABadge.tsx`
- [ ] `web/src/components/rca/ReviewWidget.tsx`
- [ ] `web/src/hooks/useRecommendationsByIncident.ts`

## Step 9: Build Verification

```bash
cd ~/Projects/PGPulse_01
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

All must pass. If failures:
- TypeScript type errors → likely missed a PascalCase→snake_case rename in frontend
- Go compile errors → likely missing interface method on NullStore
- Go test failures → check pgx array scanning for `incident_ids`
- Lint warnings → check json tag formatting

## Step 10: Commit Clean Build

```bash
git add -A
git commit -m "feat(M14_03): expansion, calibration, knowledge integration

RCA Engine:
- Comprehensive ML metric key alignment (D501)
- Threshold fallback: 4h baseline + 15min calm period (D503)
- WhileEffective temporal semantics for chain 19 (W3)
- Statement diff integration for chains 12, 13 (W4)
- All 20 chains active — Tier B filter removed (W5)
- Improved summaries with metric values + timestamps (W6)
- Refined confidence model with temporal weighting (W7)

RCA→Adviser Bridge (D505/D506):
- Migration 017: source, urgency_score, incident_ids columns
- Unified Upsert with soft cap at 10.0 (Q1)
- EvaluateHook on remediation.Engine (D507)
- hookToRuleID registry + urgency scoring
- Write() initializes urgency_score (Q2)

Frontend:
- RCABadge on incident-linked recommendations
- Inline recommendations on Incident Detail page
- Adviser feed sorted by urgency_score
- Review widget (confirmed/false_positive/inconclusive)
- JSON tag cleanup: snake_case CausalNode/CausalEdge (D504)
- MinLag/MaxLag serialized as seconds, not nanoseconds"
```

## Step 11: Deploy to Demo VM

```bash
# Cross-compile
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy binary
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/

# Update config + restart (Agent 2 should have already updated YAML)
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Verify ML models loaded
ssh ml4dbs@185.159.111.139 'sleep 5 && curl -s http://localhost:8989/api/v1/ml/models | python3 -m json.tool | wc -l'

# Check for incidents after chaos test
ssh ml4dbs@185.159.111.139 'curl -s http://localhost:8989/api/v1/rca/incidents | python3 -m json.tool'
```

## Step 12: Acceptance Criteria Verification

- [ ] AC1: ML models endpoint returns 30+ entries
- [ ] AC2: Chaos script produces RCA incident with primary chain within 5 minutes
- [ ] AC3: Timeline events contain specific metric values
- [ ] AC4: Summary includes metric values, timestamps, chain name
- [ ] AC5: Fired chain with hook → recommendation on Incident Detail AND Adviser
- [ ] AC6: Adviser sorts by urgency; RCA items have badge
- [ ] AC7: Second incident for same root cause bumps urgency (no duplicate rec)
- [ ] AC8: Causal graph API returns lowercase JSON
- [ ] AC9: Chain 19 fires on settings change (if testable)
- [ ] AC10: Chains 12/13 fire on statement regression/new query (if snapshots enabled)
- [ ] AC11: Threshold fallback shows "baseline unreliable" when data volatile
- [ ] AC12: Review buttons update review_status
- [ ] AC13: Full build verification passes

## Step 13: Wrap-Up

### 13.1 Session Log

Create `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_session-log.md`:
- Decisions confirmed
- Issues encountered
- Files created/modified (final list)
- Test results
- Demo verification results

### 13.2 Regenerate CODEBASE_DIGEST

```bash
# In Claude Code:
# "Regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md"
```

Upload updated digest to Project Knowledge.

### 13.3 Update Roadmap + Changelog

```bash
# Update docs/roadmap.md — mark M14_03 complete
# Update CHANGELOG.md — add M14_03 entry
```

### 13.4 Handoff Document

Create `docs/iterations/M14_03_03222026_expansion-calibration/HANDOFF_M14_03_to_M15.md`:
- What M14_03 built
- Current state of RCA (all 20 chains active, bridge working)
- Known issues
- What M15 needs to build on

### 13.5 Final Commit

```bash
git add docs/
git add CHANGELOG.md
git commit -m "docs: M14_03 wrap-up — session log, digest, handoff to M15"
git push origin master
```

---

## Quick Reference: File Locations

| Document | Path |
|----------|------|
| Requirements | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_requirements.md` |
| Design | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_design.md` |
| Team Prompt | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_team-prompt.md` |
| Checklist | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_checklist.md` |
| Corrections | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_corrections.md` |
| D506 Schema | `docs/iterations/M14_03_03222026_expansion-calibration/D506_recommendation_schema_evolution.md` |
| ADR | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_ADR_rca_adviser_bridge.txt` |
| Session Log | `docs/iterations/M14_03_03222026_expansion-calibration/M14_03_session-log.md` |
| Handoff | `docs/iterations/M14_03_03222026_expansion-calibration/HANDOFF_M14_03_to_M15.md` |
