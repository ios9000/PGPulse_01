# Session: 2026-03-01 — M4_02 Email Notifier & Dispatcher

## Goal

Build the alert notification delivery pipeline: Notifier interface, SMTP email notifier
with rich HTML templates, and async dispatcher with cooldown enforcement, channel routing,
and retry with exponential backoff.

## Agent Team Configuration

- Team Lead: Claude Code (Opus 4.6)
- Specialists: 2 (Notifier & Dispatcher + Tests)
- Bash: Working (v2.1.63)

## Decisions Applied

| # | Decision | Choice |
|---|----------|--------|
| D59 | Email format | Rich HTML: color-coded severity header, table layout, dashboard link, plain text fallback |
| D60 | Dispatcher model | Async goroutine with buffered channel (100), non-blocking for evaluator |
| D61 | Retry policy | 3 attempts, exponential backoff (1s, 2s, 4s), then log and drop |
| D62 | SMTP approach | Go stdlib net/smtp, STARTTLS, PLAIN auth, no external libraries |

## Files Created

### Production Code

| File | Description |
|------|-------------|
| internal/alert/notifier.go | Notifier interface + NotifierRegistry |
| internal/alert/template.go | HTML/text email templates (fire + resolution), FormatSubject(), severity helpers |
| internal/alert/dispatcher.go | Async dispatcher: buffered channel (100), cooldown, retry (1s/2s/4s) |
| internal/alert/notifier/email.go | SMTP EmailNotifier: STARTTLS, PLAIN auth, MIME multipart/alternative |

### Config Changes

| File | Change |
|------|--------|
| internal/config/config.go | Added EmailConfig struct, extended AlertingConfig (DefaultChannels, DashboardURL, Email) |
| internal/config/load.go | Email defaults (port 587, timeout 10s) + validation (email required when in channels) |
| configs/pgpulse.example.yml | Updated with full email alerting config section |

### Tests

| File | Tests | Description |
|------|-------|-------------|
| internal/alert/template_test.go | 9 | Subject formatting, HTML/text rendering, labels sorting, dashboard URL |
| internal/alert/dispatcher_test.go | 8 | Routing, default channels, cooldown, resolution bypass, retry, shutdown, buffer full, unknown channel |
| internal/alert/notifier/email_test.go | 4 | Name, mock SMTP success, connection error, MIME structure |
| internal/config/config_test.go | +3 | Email channel missing config/host, valid email config with defaults |

## Build & Validation

| Check | Result |
|-------|--------|
| go build ./... | ✅ Pass |
| go vet ./... | ✅ Pass |
| go test ./... | ✅ All pass, zero regressions |
| golangci-lint run | ✅ 0 issues (fixed 2 errcheck violations in email.go) |

## Commit

- `eae52e1` — feat(alert): add email notifier, dispatcher, and templates (M4_02)

## Implementation Notes

- 2 errcheck lint violations found and fixed in email.go during validation
- Mock SMTP server pattern used for email tests (net.Listener on :0)
- Retry delays configurable on Dispatcher for test speedup (as suggested in design)

## Not Done / Next Iteration

- [ ] M4_03: Alert API endpoints (CRUD rules, list active alerts, test notification)
- [ ] M4_03: Orchestrator post-collect wiring (evaluator hook)
- [ ] M4_03: main.go integration (evaluator + dispatcher + notifier startup/shutdown)
- [ ] Per-rule cooldown override (currently uses global default only)
- [ ] Cooldown map periodic cleanup (grows unbounded, acceptable for MVP)
