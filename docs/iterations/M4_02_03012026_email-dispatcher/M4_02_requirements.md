# M4_02 Requirements — Email Notifier & Dispatcher

**Iteration:** M4_02
**Milestone:** M4 — Alerting
**Date:** 2026-03-01
**Depends on:** M4_01 (evaluator, rules, stores)

---

## Goal

Build the notification delivery pipeline: a generic Notifier interface, an SMTP email
implementation with rich HTML templates, and an async dispatcher with channel-based
buffering, cooldown enforcement, retry with exponential backoff, and global/per-rule
channel routing. After M4_02, the alert system can evaluate metrics AND send email
notifications — but is not yet wired to the orchestrator (M4_03).

## Scope

### In Scope

1. **Notifier interface** — generic contract for all notification channels (email now, Telegram/Slack/messenger later)
2. **Email notifier** — SMTP sender using Go `net/smtp` with STARTTLS support
3. **Email templates** — rich HTML for fire and resolve events: color-coded severity, table layout, metric details, remediation hints, dashboard link placeholder
4. **Plain text fallback** — for email clients that don't render HTML (multipart/alternative)
5. **Dispatcher** — async goroutine consuming from buffered channel:
   - Receives AlertEvents from evaluator
   - Enforces cooldown (suppress repeat notifications per rule+instance+severity)
   - Resolves notification channels (global defaults + per-rule overrides)
   - Fans out to matching notifiers
   - Retry with exponential backoff: 3 attempts (1s, 2s, 4s), then log and drop
6. **SMTP config** — `alerting.email:` section in pgpulse.yml (host, port, from, username, password, TLS settings)
7. **Channel config** — `alerting.default_channels: ["email"]` and per-rule channel overrides
8. **Unit tests** — template rendering, dispatcher routing, cooldown, retry logic, notifier mock
9. **Graceful shutdown** — dispatcher drains buffered events on stop (with timeout)

### Out of Scope

- Orchestrator wiring (evaluator → dispatcher hook) → M4_03
- REST API for alerts → M4_03
- main.go integration → M4_03
- Telegram, Slack, webhook notifiers → M7+
- Corporate messenger → M7+
- Email attachment support
- Email delivery confirmation / read receipts

## Functional Requirements

### FR-1: Notifier Interface

```go
type Notifier interface {
    Name() string                                          // channel identifier: "email", "telegram", etc.
    Send(ctx context.Context, event AlertEvent) error      // deliver one notification
}
```

All future notification channels implement this interface. The dispatcher works exclusively
through Notifier — it never knows implementation details.

### FR-2: Email Notifier

SMTP-based email sender:

- Connects to configured SMTP server (host:port)
- Supports PLAIN auth (username/password) and STARTTLS
- Sends multipart/alternative emails (text/plain + text/html)
- From address from config
- To addresses from config (`alerting.email.recipients: []`)
- Subject line includes: severity emoji indicator, rule name, instance ID
- Implements Notifier interface

Connection behavior:
- New connection per send (simple for MVP; connection pooling deferred)
- Timeout: 10 seconds per send attempt
- TLS: STARTTLS when server supports it, configurable skip_verify for internal CAs

### FR-3: Email Templates

Two templates: **fire** and **resolve**. Both produce HTML + plain text.

#### Fire Email

Subject: `[CRITICAL] Transaction ID Wraparound Critical — instance prod-db-01`
(severity in brackets, rule name, instance)

HTML body:
- Header bar color-coded by severity (red=critical, orange=warning, blue=info)
- Alert details table:
  - Rule: name + description
  - Instance: instance ID
  - Metric: metric name
  - Current Value: the breaching value
  - Threshold: operator + threshold (e.g. "> 50")
  - Severity: with color badge
  - Fired At: timestamp (RFC3339)
  - Labels: if present, key=value pairs
- Remediation hint: from rule description field
- Footer: "Sent by PGPulse Alert Engine" + dashboard link placeholder
  (`{{.DashboardURL}}/instances/{{.InstanceID}}`)

Plain text body:
- Same information in readable text format without HTML

#### Resolve Email

Subject: `[RESOLVED] Transaction ID Wraparound Critical — instance prod-db-01`

HTML body:
- Green header bar
- Alert resolved details table (same fields + resolved_at timestamp + duration)
- Footer same as fire

### FR-4: Dispatcher

Async goroutine-based dispatcher:

```
Evaluator ──AlertEvent──► [buffered channel (size 100)] ──► Dispatcher goroutine
                                                                 │
                                                          ┌──────┴───────┐
                                                          │ Cooldown     │
                                                          │ check        │
                                                          └──────┬───────┘
                                                                 │ (pass)
                                                          ┌──────┴───────┐
                                                          │ Resolve      │
                                                          │ channels     │
                                                          └──────┬───────┘
                                                                 │
                                                          ┌──────┴───────┐
                                                          │ Fan out to   │
                                                          │ notifiers    │
                                                          │ (retry each) │
                                                          └──────────────┘
```

- **Buffer**: channel of size 100. If full, log warning and drop event (non-blocking for evaluator).
- **Cooldown enforcement**: per (rule_id, instance_id, severity) — track last notification time.
  Skip if within cooldown window. Resolution events always pass (ignore cooldown).
- **Channel resolution**: event.Channels (per-rule override) or global default_channels.
  Look up registered notifiers by name.
- **Fan out**: send to each resolved notifier independently. One failure doesn't block others.
- **Retry**: per notifier, 3 attempts with exponential backoff (1s, 2s, 4s). Log each retry.
  After 3 failures, log error with full event context and move on.
- **Graceful shutdown**: Stop() signals the goroutine to drain remaining buffered events
  (with 5-second drain timeout), then returns.

### FR-5: SMTP Config

```yaml
alerting:
  enabled: true
  default_channels: ["email"]
  default_consecutive_count: 3
  default_cooldown_minutes: 15
  evaluation_timeout_seconds: 5
  history_retention_days: 30

  email:
    host: "smtp.corp.example.com"
    port: 587
    username: "pgpulse@corp.example.com"
    password: "secret"              # future: vault reference
    from: "PGPulse Alerts <pgpulse@corp.example.com>"
    recipients:
      - "dba-team@corp.example.com"
      - "oncall@corp.example.com"
    tls_skip_verify: false          # true for internal CAs without public trust
    send_timeout_seconds: 10

  dashboard_url: "https://pgpulse.corp.example.com"  # for email links
```

Validation:
- `alerting.email` required when `"email"` is in `default_channels`
- `email.host` and `email.from` and `email.recipients` (non-empty) required
- `email.port` defaults to 587

### FR-6: Graceful Shutdown

```
Dispatcher.Stop()
  → close events channel
  → drain loop: process remaining events (max 5s)
  → cancel retry contexts
  → return
```

## Non-Functional Requirements

- Dispatcher.Dispatch(event) must return in < 1ms (just channel send)
- Email send timeout: 10 seconds (configurable)
- Buffer overflow: log warning, do not block evaluator
- Retry backoff: 1s, 2s, 4s (total worst case: 7s per notifier per event)
- HTML email size: < 50KB per message
- All SMTP credentials from config, never hardcoded
- No external email library dependencies — use Go stdlib net/smtp

## Test Requirements

### Template Tests
- Render fire template: verify HTML contains severity color, metric value, threshold, instance ID
- Render resolve template: verify contains "RESOLVED", duration, green color
- Render plain text: verify readable without HTML tags
- Render with labels: verify label key=value pairs appear
- Render with empty optional fields: no panic, graceful fallback

### Dispatcher Tests
- Dispatch routes event to correct notifier by channel name
- Dispatch with per-rule channels overrides global defaults
- Cooldown suppresses repeat notification within window
- Cooldown allows resolution events regardless of window
- Cooldown allows severity escalation (warning → critical)
- Buffer overflow: Dispatch returns immediately, event dropped, warning logged
- Graceful shutdown drains remaining events
- Multiple notifiers: one failure doesn't prevent others

### Email Notifier Tests
- Send with mock SMTP server (use net.Listener or interface mock)
- Verify email headers: From, To, Subject, Content-Type (multipart/alternative)
- Verify STARTTLS negotiation attempt
- Verify send timeout is respected

### Retry Tests
- Success on first attempt: no retry
- Success on second attempt: 1 retry logged
- Failure on all 3 attempts: error logged, event dropped
- Backoff timing: verify delays approximate 1s, 2s, 4s

## Deliverables

| File | Description |
|------|-------------|
| `internal/alert/notifier.go` | Notifier interface + registry type |
| `internal/alert/notifier/email.go` | SMTP email notifier |
| `internal/alert/template.go` | HTML + plain text email templates (fire + resolve) |
| `internal/alert/dispatcher.go` | Async dispatcher with cooldown, routing, retry |
| `internal/config/config.go` | Add EmailConfig, update AlertingConfig |
| `configs/pgpulse.example.yml` | Add email + dashboard_url sections |
| `internal/alert/notifier_test.go` | Notifier interface contract tests |
| `internal/alert/notifier/email_test.go` | SMTP mock tests |
| `internal/alert/template_test.go` | Template rendering tests |
| `internal/alert/dispatcher_test.go` | Dispatcher routing, cooldown, retry, shutdown tests |
| `internal/config/config_test.go` | +2-3 tests for email config validation |
