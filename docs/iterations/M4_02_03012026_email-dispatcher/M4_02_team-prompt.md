# M4_02 Team Prompt — Email Notifier & Dispatcher

**Paste this into Claude Code after placing M4_02_requirements.md and M4_02_design.md
in `docs/iterations/M4_02_03012026_email-dispatcher/`.**

---

Build the alert notification delivery pipeline for PGPulse.
Read CLAUDE.md for project context.
Read docs/iterations/M4_02_03012026_email-dispatcher/M4_02_design.md for the full design.

This iteration builds: Notifier interface, SMTP email notifier with rich HTML templates,
async dispatcher with cooldown enforcement and retry. Builds on M4_01 (evaluator, rules, stores).
No orchestrator wiring or API endpoints — those are M4_03.

Create a team of 2 specialists:

---

SPECIALIST 1 — NOTIFIER & DISPATCHER (production code):

Create the following files:

1. `internal/alert/notifier.go` — Notifier interface and registry:
   - Notifier interface: Name() string, Send(ctx, AlertEvent) error
   - NotifierRegistry struct with map[string]Notifier
   - NewNotifierRegistry(), Register(Notifier), Get(name) Notifier, Names() []string

2. `internal/alert/template.go` — Email templates and rendering:
   - templateData struct: Event AlertEvent, DashboardURL, SeverityColor, SeverityEmoji, Duration, Timestamp, ResolvedTime, LabelPairs []labelPair
   - labelPair struct: Key, Value string
   - FormatSubject(event) string — e.g. "[CRITICAL] Transaction ID Wraparound Critical — prod-db-01"
     * For resolutions: "[RESOLVED] Rule Name — instance"
   - severityColor(Severity) string — critical=#dc2626 (red), warning=#d97706 (amber), info=#2563eb (blue)
   - severityEmoji(Severity) string — critical=🔴, warning=🟠, info=🔵
   - RenderHTMLTemplate(event, dashboardURL) (string, error) — rich HTML email:
     * Color-coded header bar by severity (green #16a34a for resolutions)
     * Details table: Instance, Metric (code formatted), Current Value (colored), Threshold (operator + value), Severity (badge), Fired At, Labels (if present)
     * Dashboard link button when dashboardURL non-empty: {{dashboardURL}}/instances/{{instanceID}}
     * Footer: "Sent by PGPulse Alert Engine"
     * Resolution variant: green header, "✅ Resolved" title, includes Duration and Resolved At
   - RenderTextTemplate(event, dashboardURL) (string, error) — plain text version of same content
   - buildTemplateData(event, dashboardURL) templateData — helper preparing template context
   - Use html/template for HTML, text/template for plain text
   - Sort label pairs by key for deterministic output (import "sort")

3. `internal/alert/dispatcher.go` — Async dispatcher:
   - Constants: defaultBufferSize=100, maxRetries=3, retryDelays=[1s,2s,4s]
   - Dispatcher struct with: registry *NotifierRegistry, defaultChannels []string, cooldownMinutes int, logger, events chan AlertEvent, done chan struct{}, wg sync.WaitGroup, mu sync.Mutex, cooldowns map[string]time.Time
   - NewDispatcher(registry, defaultChannels, defaultCooldownMinutes, logger) *Dispatcher
   - Start() — launch goroutine consuming from events channel
   - Dispatch(event AlertEvent) bool — non-blocking send to channel; returns false if buffer full (log warning, drop)
   - Stop() — close events channel, wg.Wait() for drain
   - run() — range over events channel, call processEvent for each
   - processEvent(event):
     * Skip if cooldown active (resolutions always pass)
     * Resolve channels: event.Channels if non-empty, else defaultChannels
     * Fan out to each notifier via sendWithRetry
     * Record cooldown timestamp after successful dispatch
   - isCoolingDown(event) bool — check cooldowns map
   - recordCooldown(event) — store time.Now() in cooldowns map
   - resolveChannels(event) []string
   - sendWithRetry(notifier, event) — 3 attempts with exponential backoff (1s, 2s, 4s)
     * Each attempt: context.WithTimeout(15s), call notifier.Send()
     * Log success/failure/exhaustion with structured fields
   - cooldownKey(ruleID, instanceID, severity) string — "ruleID:instanceID:severity"

4. `internal/alert/notifier/email.go` — SMTP email notifier:
   - Package: `notifier` (internal/alert/notifier/)
   - EmailNotifier struct: cfg config.EmailConfig, dashboardURL string, logger *slog.Logger
   - NewEmailNotifier(cfg, dashboardURL, logger) *EmailNotifier
   - Name() returns "email"
   - Send(ctx, event) error:
     * Call alert.FormatSubject(event) for subject
     * Call alert.RenderHTMLTemplate(event, dashboardURL) for HTML body
     * Call alert.RenderTextTemplate(event, dashboardURL) for text body
     * Build multipart/alternative MIME message via buildMIMEMessage()
     * Send via sendSMTP()
   - sendSMTP(ctx, msg) error:
     * Dial with context timeout (net.Dialer.DialContext)
     * smtp.NewClient
     * STARTTLS if server supports it (crypto/tls, respect TLSSkipVerify)
     * PLAIN auth if username non-empty
     * MAIL FROM, RCPT TO (all recipients), DATA, write message, QUIT
   - buildMIMEMessage(from, to, subject, textBody, htmlBody) ([]byte, string):
     * Build proper MIME headers: From, To, Subject, MIME-Version, Content-Type multipart/alternative
     * Text part: text/plain; charset=utf-8
     * HTML part: text/html; charset=utf-8
     * Use mime/multipart.Writer for boundary generation

5. Config changes:
   - Add EmailConfig struct to internal/config/config.go:
     * Host, Port (int), Username, Password, From, Recipients ([]string), TLSSkipVerify (bool), SendTimeoutSeconds (int)
   - Update AlertingConfig — add fields:
     * DefaultChannels []string `koanf:"default_channels"`
     * DashboardURL string `koanf:"dashboard_url"`
     * Email *EmailConfig `koanf:"email"` (pointer — nil when not configured)
   - Update alertingDefaults() in load.go:
     * Email.Port defaults to 587
     * Email.SendTimeoutSeconds defaults to 10
   - Update validateAlerting() in load.go:
     * If "email" is in DefaultChannels, Email config must be non-nil
     * Email.Host, Email.From, Email.Recipients (non-empty) required
   - Update configs/pgpulse.example.yml — add complete alerting section with email example

IMPORTANT RULES:
- Import alert types from "github.com/ios9000/PGPulse_01/internal/alert"
- Import config from "github.com/ios9000/PGPulse_01/internal/config"
- Email notifier uses ONLY Go stdlib: net/smtp, crypto/tls, mime/multipart, net, net/textproto
- NO external email libraries
- Template must use html/template (not text/template) for HTML rendering to auto-escape
- Dispatcher.Dispatch() must be non-blocking (select with default)
- All slog fields use structured logging: slog.String(), slog.Int(), etc.
- Follow existing code style patterns from internal/alert/evaluator.go and internal/auth/

---

SPECIALIST 2 — TESTS:

Create comprehensive tests for all notification delivery code:

1. `internal/alert/template_test.go`:
   - TestFormatSubject_Fire: severity tag in brackets, rule name, instance ID
   - TestFormatSubject_Resolve: contains "[RESOLVED]"
   - TestFormatSubject_AllSeverities: info, warning, critical all produce correct tags
   - TestRenderHTMLTemplate_Fire: HTML contains severity color hex, metric value, threshold, instance ID
   - TestRenderHTMLTemplate_Resolve: contains green color (#16a34a), "RESOLVED" or "Resolved", duration
   - TestRenderHTMLTemplate_WithLabels: label key-value pairs present in output
   - TestRenderHTMLTemplate_NoDashboardURL: no dashboard link when URL empty
   - TestRenderHTMLTemplate_WithDashboardURL: contains link with instance ID
   - TestRenderTextTemplate_Fire: readable plain text, all fields present
   - TestRenderTextTemplate_Resolve: contains resolved timestamp

2. `internal/alert/dispatcher_test.go`:
   - Create mockNotifier at top of file:
     ```go
     type mockNotifier struct {
         name  string
         mu    sync.Mutex
         calls []AlertEvent
         failN int  // fail first N Send() calls
     }
     ```
     Implements Notifier: Name() returns name, Send() records call, returns error if failN > 0 (decrement)
   - TestDispatcher_RoutesToCorrectNotifier: register "email" mock, dispatch event, verify mock.calls has 1 event
   - TestDispatcher_UsesDefaultChannels: event with empty Channels, default_channels=["email"], verify email mock called
   - TestDispatcher_PerRuleChannelOverride: event.Channels=["email"], default_channels=["other"], verify email called, not other
   - TestDispatcher_CooldownSuppresses: dispatch same event twice within cooldown, verify only 1 Send call
   - TestDispatcher_CooldownAllowsResolution: dispatch fire, then resolution within cooldown, verify 2 calls
   - TestDispatcher_BufferOverflow: create dispatcher with small buffer override (or fill buffer with blocked mock), verify Dispatch returns false
   - TestDispatcher_GracefulShutdown: dispatch events, call Stop(), verify all events processed
   - TestDispatcher_RetryOnFailure: mockNotifier.failN=2 (fails twice, succeeds third), verify 3 Send calls total
   - TestDispatcher_RetryExhausted: mockNotifier.failN=5 (exceeds max), verify exactly 3 Send calls (maxRetries)
   - TestDispatcher_MultipleNotifiers: register 2 mocks, default_channels=["a","b"], verify both receive event
   - TestDispatcher_UnknownChannel: event.Channels=["nonexistent"], verify no panic, warning logged

   NOTE ON TIMING: Dispatcher is async — tests must wait for goroutine to process events.
   Use patterns like: dispatch → time.Sleep(50ms) → check mock.calls. Or use a done channel/WaitGroup in mock.
   For retry tests, be aware of retry delays (1s+2s+4s=7s total). Consider making retryDelays configurable
   via a package-level variable or dispatcher option for test speedup, OR accept longer test runtime.
   Recommended: make retry delays a Dispatcher field set in constructor, with a test helper to override.

3. `internal/alert/notifier/email_test.go`:
   - Create startMockSMTP helper:
     ```go
     func startMockSMTP(t *testing.T) (addr string, messages chan string, cleanup func())
     ```
     Uses net.Listen on ":0", goroutine accepts connection, speaks minimal SMTP protocol
     (220 greeting, responds to EHLO, MAIL FROM, RCPT TO, DATA, QUIT), captures DATA content.
   - TestEmailNotifier_Send: start mock SMTP, create notifier with mock addr, send event, verify message received
   - TestEmailNotifier_MultipartContent: verify received message contains "multipart/alternative", "text/plain", "text/html"
   - TestEmailNotifier_Headers: verify From, To, Subject headers correct
   - TestEmailNotifier_MultipleRecipients: 2 recipients, verify both get RCPT TO
   - TestEmailNotifier_Name: returns "email"

4. `internal/config/config_test.go` — ADD tests (don't replace existing):
   - TestAlertingConfig_EmailRequiredWhenInChannels: email in default_channels + no email config → error
   - TestAlertingConfig_EmailDefaults: port=0→587, timeout=0→10
   - TestAlertingConfig_EmailValid: full email config passes validation

TESTING RULES:
- Dispatcher tests need awareness of async processing — add small sleeps or synchronization
- For retry timing, consider adding a RetryDelays field to Dispatcher for test override
- Mock SMTP server: keep minimal (just enough to not hang the client)
- All tests must pass with `go test -race ./internal/alert/... ./internal/config/...`
- Use t.Run() for subtests, table-driven where applicable
- Run `golangci-lint run` on all new code — fix any issues

---

COORDINATION:

- Both specialists can start simultaneously:
  * Specialist 1 creates production code
  * Specialist 2 creates test structure and mocks immediately, fills assertions once production code exists
- After both are done:
  1. Run `go build ./...` — must compile cleanly
  2. Run `go vet ./...` — must pass
  3. Run `go test -race ./internal/alert/... ./internal/config/...` — all tests must pass
  4. Run `golangci-lint run` — 0 issues
  5. Run `go test -race ./...` — full regression, all prior tests must still pass
- Fix any issues before declaring done
- Commit: `git add . && git commit -m "feat(alert): add email notifier, dispatcher, and templates (M4_02)"`
