# PGPulse — Chat Transition Process

> **Purpose:** How to move between Claude.ai chats without losing context.
> This is a PROCESS document (rules), not a log.

---

## The Problem

Claude.ai chats are isolated. A new chat cannot:
- Read files from your GitHub repo
- See previous chat history
- Access uploaded files from other chats

Everything the new chat needs must be **explicitly provided** in that chat.

## Document Taxonomy

| Document | Type | Lives In | Changes? | Upload to New Chat? |
|---|---|---|---|---|
| **PGPulse_Development_Strategy** | Rules/Process | `.claude/rules/` OR Project Knowledge | Rarely (process changes only) | Add to **Project Knowledge** once |
| **PGAM_FEATURE_AUDIT.md** | Reference | Project Knowledge | Never | Add to **Project Knowledge** once |
| **Iteration Handoff** | Transition | `docs/iterations/` (latest only matters) | Every chat transition | **YES — upload at chat start** |
| **RESTORE_CONTEXT.md** | Emergency recovery | `docs/` | Per milestone | Only if handoff is lost |
| **Session-log** | History | `docs/iterations/M*_*/` | End of each chat | No (stays in repo) |
| **roadmap.md** | Status tracker | `docs/` | End of each chat | No (referenced in handoff) |
| **CHANGELOG.md** | History | `docs/` | End of each chat | No (stays in repo) |

## What Goes Where

### Project Knowledge (upload once, available to ALL chats in the project)

These are stable documents that every chat needs:

1. **PGPulse_Development_Strategy_v2.md** — process rules
2. **PGAM_FEATURE_AUDIT.md** — legacy SQL reference (76 queries)
3. **pgpulse_architecture.docx** — system architecture

> **How:** In Claude.ai → Projects → PGPulse → Project Knowledge → Upload files
> These persist across all chats in the project. Upload once, never again.

### Iteration Handoff (upload per chat, contains EVERYTHING)

Created at the end of each chat. Uploaded at the start of the next chat.

**Naming:** `HANDOFF_M{from}_to_M{to}.md`

**Must contain (self-contained, no external references):**

```markdown
# PGPulse — Iteration Handoff: M{X} → M{Y}_01

## DO NOT RE-DISCUSS
[List of final decisions — prevents the new chat from revisiting]

## What Exists Now
[Actual code snippets of key interfaces — not just file paths]
[List of files in the repo with one-line descriptions]

## What Was Just Completed
[Summary of the previous iteration's output]

## Known Issues
[Bugs, workarounds, environment quirks]

## Next Task: M{Y}_01
[Full specification including:]
- Goal
- Input data (actual SQL queries, not "see file X")
- Architecture decisions already made
- Files to create
- Version gates required
- Testing approach

## Workflow Reminder
[Claude Code agents run go build/test/lint/commit directly. No manual steps needed.]
```

### Session-Log (stays in repo, not uploaded)

Created at the end of each chat. Committed to git. NOT uploaded to next chat
(the handoff summarizes what matters).

---

## Step-by-Step: Transitioning to a New Chat

### End of Current Chat

1. **Create session-log.md** — goals, agent activity, decisions, commits
2. **Create handoff document** — self-contained, includes actual content
3. **Update roadmap.md** — mark iteration done, update dates
4. **Update CHANGELOG.md** — add completed features
5. **Commit and push:**
   ```bash
   git add docs/
   git commit -m "docs: add M{X} session-log and handoff to M{Y}"
   git push
   ```

### Start of New Chat

1. **Open new chat in the PGPulse Project** (so Project Knowledge is available)
2. **Upload the handoff document** as your first message:
   > "Here's the iteration handoff. Let's begin M{Y}_01."
3. **Do NOT upload:** strategy doc (it's in Project Knowledge), session-logs, roadmap
4. **If Claude asks for PGAM queries:** they should be in the handoff already.
   If not, the PGAM_FEATURE_AUDIT.md should be in Project Knowledge.

### What the New Chat Receives

```
┌─ Project Knowledge (auto-loaded) ──────────┐
│ • PGPulse_Development_Strategy_v2.md       │
│ • PGAM_FEATURE_AUDIT.md                    │
│ • pgpulse_architecture.docx                │
└────────────────────────────────────────────┘
         +
┌─ Uploaded by Developer ────────────────────┐
│ • HANDOFF_M{X}_to_M{Y}.md                 │
│   (self-contained: interfaces, decisions,  │
│    SQL queries, next task spec)             │
└────────────────────────────────────────────┘
         =
   Everything needed to start immediately
```

---

## Anti-Patterns

| Wrong | Why | Right |
|---|---|---|
| "Read docs/legacy/PGAM_FEATURE_AUDIT.md" | New chat can't read repo files | Include actual queries in handoff, or add to Project Knowledge |
| Upload strategy doc every chat | Wastes context window | Add to Project Knowledge once |
| Skip the handoff document | New chat re-discovers everything | Always create handoff |
| Put session history in strategy doc | Strategy becomes bloated and unstable | Session history → session-log.md (repo only) |
| Start new chat without uploading anything | Claude has zero context | Always upload handoff as first message |
| Let new chat re-discuss decided architecture | Wastes time and may contradict decisions | "DO NOT RE-DISCUSS" section in handoff |
