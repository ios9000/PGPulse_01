# Archive — PGAM lineage (historical)

Everything in this folder is historical and is kept only for the record. PGPulse started in February 2026 as a Go rewrite of PGAM, a PHP PostgreSQL activity monitor from 2019, and its first milestones (M1–M7) were planned against PGAM's feature inventory. That framing ended with M16 ("Cut the anchor"): PGPulse is now a fleet incident-intelligence system with cross-tier root cause analysis, and the product form is a DBA agent system in which PGPulse is the evidence layer and DBPortal is the execution layer with human approval. Nothing here describes the current product, drives any milestone, or should be cited as a requirement. The PGAM feature audit itself (the 76-query inventory, `PGAM_FEATURE_AUDIT.md`) was never committed to this repository because it records internal infrastructure details; it stays in the Claude.ai project knowledge as a reference and is not needed for development. For the current direction see [`../../roadmap.md`](../../roadmap.md) (M16–M19) and the top-level [`README.md`](../../../README.md).

| File | What it is |
|---|---|
| [PGPulse_Competitive_Research_Synthesis.md](PGPulse_Competitive_Research_Synthesis.md) | March 2026 competitive research (pgwatch, PMM, pganalyze, Datadog DBM, pg_profile) that informed metric naming and the OS agent. Its naming recommendations are being applied in M16. |
