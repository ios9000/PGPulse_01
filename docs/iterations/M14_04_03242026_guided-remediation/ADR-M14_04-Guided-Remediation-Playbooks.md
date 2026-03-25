This is the final, comprehensive Architecture Decision Record (ADR) translated into English, integrating your foundational design, the Playbook Resolver, the Two-Loop Feedback model, and my architectural critique, risk mitigations, and future growth vectors.

You can save this directly as **`ADR-M14_04-Guided-Remediation-Playbooks.md`**.

***

# ADR: Guided Remediation Playbooks in PGPulse

**Status:** Proposed / Locked
**Identifier:** ADR-M14_04-Guided-Remediation-Playbooks
**Subsystem:** PGPulse Operations, RCA & Remediation Layer

## 1. Context and Problem Statement

PGPulse currently includes three powerful layers for incident management:
* **Alerting:** Detects the symptom and reports an anomaly.
* **RCA:** Builds a probable causal chain and explains *why* it happened.
* **Adviser:** Formulates general recommendations and directions for action.

While this combination is sufficient for an experienced DBA, it falls short for on-call shifts, L1/L2 operators, and application engineers. These users lack practical DBA expertise and should not be forced to manually interpret PostgreSQL system views, copy-paste SQL queries into `psql`, or make risky decisions without context.

A typical WAL archiving failure incident highlights this gap:
* [cite_start]*Alerting:* WAL archiving is failing[cite: 5].
* [cite_start]*RCA:* Probable cause is a problem in the backup/storage path or archive target[cite: 5].
* [cite_start]*Adviser:* Check the archive storage[cite: 5].

The operator still does not know:
* [cite_start]Which query to execute first[cite: 5].
* [cite_start]What is considered "normal"[cite: 5].
* [cite_start]When to urgently escalate to a DBA[cite: 5].
* [cite_start]Which actions can be safely automated[cite: 5].

This creates two poor outcomes: immediate escalation to DBAs (reducing the value of the monitoring stack) or manual copy-pasting (creating high risks of executing the wrong query on the wrong instance).

## 2. Solution: Guided Remediation Playbooks

We are introducing **Guided Remediation Playbooks** as a fourth operational layer. [cite_start]This is a system of executable, database-stored, UI-editable playbooks consisting of step-by-step diagnostic and remediation scenarios[cite: 5]. 

PGPulse will act as an operational assistant that:
* Connects to the correct instance using its own connection pool.
* [cite_start]Executes approved steps safely[cite: 5].
* Renders the results inline in the UI.
* [cite_start]Interprets the output (Green/Yellow/Red)[cite: 5].
* [cite_start]Guides the operator to the next step, branch, or escalation[cite: 5].

**Short Formula:** *Alerting detects. RCA explains. Playbook Resolver selects. Adviser presents. Guided Remediation executes safely. Feedback teaches the system what worked.*

---

## 3. Core Architectural Decisions

### 3.1. Storage and Versioning
Playbooks will be stored in the database, editable via the UI, and inherently version-aware.
* *Rationale:* No recompilation needed for new scenarios; allows shipping a built-in "seed pack" while permitting organizational customization. Playbooks become an operational asset, not just a static wiki page.
* *Requirements:* Each playbook must support an ID, version, status (draft, stable, deprecated), author, approval state, and an immutable audit trail.

### 3.2. Entry Points and The Playbook Resolver
[cite_start]Playbooks must be accessible via Alerting, RCA, Adviser, and a Manual Catalog[cite: 5]. However, to prevent competing recommendations, we are introducing the **Playbook Resolver**.

The Playbook Resolver is a centralized logical component that selects the most appropriate playbook based on the context provided by these subsystems. It ranks candidates using the following strict priority rule:
1.  [cite_start]**Explicit RCA Hook match** (Strongest signal) [cite: 5]
2.  [cite_start]**Root Cause key match** [cite: 5]
3.  [cite_start]**Alert key / alert class match** (Symptom-driven fallback) [cite: 5]
4.  [cite_start]**Adviser remediation class fallback** [cite: 5]
5.  [cite_start]**Manual catalog fallback** [cite: 5]

*Architectural Strength:* This separation of concerns prevents "alert storms" from overwhelming the operator with multiple conflicting playbooks.

### 3.3. The Role of Adviser (UX Layer)
While the Playbook Resolver handles the selection logic, the **Adviser** serves as the primary presentation layer. Psychologically, this is where the user expects to see "what to do next." The Adviser will display the recommended playbook, the reason for selection, the confidence level, and an "Open Guided Remediation" button.

### 3.4. Execution Safety Model
[cite_start]A strict, multi-tier safety model is enforced[cite: 5]:
* **Tier 1 (Diagnostic):** Safe, vetted read-only operations. Auto-run permitted. [cite_start]Inline rendering[cite: 5].
* **Tier 2 (Confirmed Action):** Controlled mutations with limited risk. [cite_start]Requires explicit UI confirmation showing the exact text and target instance[cite: 5].
* **Tier 3 (Approval Required):** Dangerous/high-impact actions. [cite_start]Requires strict RBAC approval (DBA escalation)[cite: 5].
* **Tier 4 (External/Manual Escalation):** Actions outside PGPulse's scope (e.g., OS-level disk expansion). [cite_start]Shows instructions only[cite: 5].

### 3.5. Branching and Result Interpretation
* [cite_start]**Bounded Conditional Branching:** Playbooks function as guarded decision trees (e.g., *if red -> go to emergency branch*), not arbitrary workflow engines[cite: 5]. [cite_start]Loops and nested workflows are explicitly forbidden in the first release[cite: 5].
* [cite_start]**Static Declarative Rules:** Result interpretation relies on typed, declarative checks (numeric thresholds, row counts, null checks) defined in the playbook JSON[cite: 5]. [cite_start]Go-side evaluator functions or embedded expression languages are rejected to maintain security, debuggability, and ease of auditing[cite: 5].

### 3.6. Seed Pack
[cite_start]The initial release will ship with a **Core 10** seed pack, covering high-impact operational stories: WAL archive failure, Replication lag, Connection saturation, Lock contention, Long transactions, Checkpoint storms, Disk full, Autovacuum failing, Wraparound risk, and Heavy query diagnostics[cite: 5].

---

## 4. Two-Loop Feedback Model

To evolve PGPulse into a learning expert system, we are implementing a strictly separated Two-Loop Feedback Model.

* **Loop 1: RCA Feedback (Diagnosis Evaluation):** Answers "Was the root cause identified correctly?" (Correct / Incorrect / Partial).
* **Loop 2: Guided Remediation Feedback (Treatment Evaluation):** Answers "Was the selected playbook useful, and did it resolve the issue?" (Relevant / Irrelevant, Resolved / Escalated).

**Phased Maturity (Guardrail):** Feedback will initially be used exclusively for observability, quality analytics, and supervised calibration. It is **strictly forbidden** for feedback to autonomously alter Safety Tiers, dangerous action policies, or execution weights without human supervision.

---

## 5. Risk Register and Mitigation Strategies

### 5.1. Security Risks
* **Risk:** *Data Exfiltration / Tampering via Tier 1.* Since playbooks are UI-editable, an attacker could modify a Tier 1 (Auto-run) step to extract sensitive data or drop tables.
* **Mitigation:** * Editing the SQL payload instantly transitions a playbook to `draft` status. Promoting to `stable` requires `SuperAdmin` RBAC approval.
    * The execution engine must forcefully wrap all Tier 1 queries in a `SET TRANSACTION READ ONLY;` block, regardless of their syntax.

### 5.2. Performance Risks
* **Risk:** *Connection Pool Starvation & OOM.* A poorly written diagnostic query scanning `pg_stat_activity` during an outage could return millions of rows, crashing the Go backend or exhausting the monitored instance's connection pool.
* **Mitigation:**
    * The execution engine must inject strict, non-negotiable `statement_timeout` and `lock_timeout` (e.g., 5-10 seconds) at the session level.
    * The backend must append a strict `LIMIT 100` (or similar cap) to any raw result sets before returning them to the UI.

### 5.3. Functionality Risks
* **Risk:** *Version Drift / State Loss.* An operator starts a playbook, but a DBA edits and saves a new version of that playbook while the operator is on Step 2. Alternatively, the operator closes their browser tab while waiting for a Tier 3 approval.
* **Mitigation:** * Introduce a `PlaybookRun` database entity. The run state must be strictly bound to the `playbook_version` at the time of initiation.
    * `PlaybookRun` saves the execution progress asynchronously, allowing the operator to safely close the tab and resume later.

* **Risk:** *Sparse Feedback.* Operators under stress will not manually fill out feedback forms.
* **Mitigation:** Collect **Implicit Behavioral Signals**. If a playbook is executed and the associated alert auto-resolves within 5 minutes, record an implicit "Success." If the "Call DBA" button is clicked, record an implicit "Escalation."

---

## 6. Future Growth Vectors (Deferred by Design)

[cite_start]To maintain a focused initial release, the following powerful Enterprise features are deferred, but the current architecture fully supports their future implementation[cite: 5]:

1.  **Parameterized Inputs (Interactive Playbooks):** Allowing operators to input specific values (e.g., a blocking `PID`) discovered in a Tier 1 diagnostic step into a Tier 2 remediation step (e.g., `SELECT pg_terminate_backend($1)`).
2.  **Dry-Run / Pre-flight Validation:** Before executing a Tier 2/3 mutating action, the engine automatically runs an `EXPLAIN` or syntax validation check inside a rolled-back transaction to ensure the query won't fail during a crisis.
3.  **Playbooks as Code (GitOps):** Supporting YAML/JSON declarative exports/imports, allowing enterprise clients to manage their custom playbooks in Git repositories and sync them to PGPulse via CI/CD pipelines. 
4.  **A/B Testing of Playbooks:** The Playbook Resolver can eventually route different scenarios for the same Root Cause (50/50 split) to objectively measure which remediation strategy yields the fastest MTTR (Mean Time to Resolution).

---
