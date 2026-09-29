# PGPulse documentation

This folder records how PGPulse was planned and built, iteration by iteration. The Claude Code harness reads these paths, so files are never moved.

## How the process works

1. **Plan** (Claude.ai). Each iteration starts with requirements, a design and a team prompt.
2. **Build** (Claude Code). The team prompt drives an agent team, or a single agent, that implements, tests and commits the change.
3. **Record.** A session log captures what was done, the decisions made and the verification results.
4. **Hand off.** A handoff carries the context into the next planning chat.
5. **Save.** At milestones, a save point captures the whole project state.

## Index

| Path | What it holds |
|---|---|
| [iterations/](iterations/) | One folder per iteration, named `<ID>_<MMDDYYYY>_<slug>` (53 folders). Typical files are `<ID>_requirements.md`, `<ID>_design.md`, `<ID>_team-prompt.md`, `<ID>_session-log.md`, and from M8 on `<ID>_checklist.md`. |
| [iterations/](iterations/) `HANDOFF_<from>_to_<to>.md` | Handoffs between planning chats. Each one opens with a "DO NOT RE-DISCUSS" list of settled decisions. Most sit at the top of `iterations/`, a few inside their iteration folder, and one in this folder. |
| [save-points/](save-points/) | `SAVEPOINT_<milestone>_<YYYYMMDD>.md` are self-contained project snapshots: architecture, interfaces, decision log, status and environment. [LATEST.md](save-points/LATEST.md) is the newest one. |
| ADRs | [M14_03 RCA–adviser bridge](iterations/M14_03_03222026_expansion-calibration/M14_03_ADR_rca_adviser_bridge.txt) and [M14_04 guided remediation playbooks](iterations/M14_04_03242026_guided-remediation/ADR-M14_04-Guided-Remediation-Playbooks.md). Most other decisions live in the save-point decision tables and the session logs. |
| [CODEBASE_DIGEST.md](CODEBASE_DIGEST.md) | Generated reference: file inventory, interfaces, metric keys, API routes, collectors and config keys. |
| [roadmap.md](roadmap.md), [CHANGELOG.md](CHANGELOG.md) | Milestone status and what each iteration delivered. |
| [PGPulse_Development_Strategy_v2.md](PGPulse_Development_Strategy_v2.md) | The development method: planning in Claude.ai, implementation in Claude Code. |
| [RESTORE_CONTEXT.md](RESTORE_CONTEXT.md) | Emergency context recovery if a handoff is lost. |
| [desktop/](desktop/) | User and setup guides for the Windows desktop build. |
| [legacy/](legacy/) | Competitive research notes. |
| [../.claude/CLAUDE.md](../.claude/CLAUDE.md), [../.claude/rules/](../.claude/rules/) | Instructions and rules the agents follow: module ownership, code style, security, PostgreSQL conventions, chat transitions, save points. |
