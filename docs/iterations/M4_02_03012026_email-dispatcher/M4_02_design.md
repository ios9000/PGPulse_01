# M4_02 Design — Email Notifier & Dispatcher

**Iteration:** M4_02
**Date:** 2026-03-01
**Input:** M4_02_requirements.md, M4_01 codebase (alert.go, evaluator.go, store.go)

---

## 1. Package Layout

```
internal/alert/
├── alert.go              ← (M4_01, unchanged)
├── evaluator.go          ← (M4_01, unchanged)
├── rules.go              ← (M4_01, unchanged)
├── store.go              ← (M4_01, unchanged)
├── pgstore.go            ← (M4_01, unchanged)
├── seed.go               ← (M4_01, unchanged)
├── notifier.go           ← NEW: Notifier interface + NotifierRegistry
├── template.go           ← NEW: HTML + text email templates
├── dispatcher.go         ← NEW: async dispatcher with cooldown, routing, retry
├── notifier/
│   ├── email.go          ← NEW: SMTP notifier implementation
│   └── email_test.go     ← NEW: SMTP mock tests
├── notifier_test.go      ← NEW: interface contract tests
├── template_test.go      ← NEW: template rendering tests
└── dispatcher_test.go    ← NEW: routing, cooldown, retry, shutdown tests
```

Config changes:

```
internal/config/config.go   ← Add EmailConfig, DashboardURL, DefaultChannels
internal/config/load.go     ← Add email validation
configs/pgpulse.example.yml ← Add email + dashboard_url sections
```

---

## 2. Notifier Interface (`internal/alert/notifier.go`)

```go
package alert

import "context"

// Notifier delivers alert notifications via a specific channel.
type Notifier interface {
    // Name returns the channel identifier (e.g. "email", "telegram").
    Name() string

    // Send delivers a notification for the given alert event.
    Send(ctx context.Context, event AlertEvent) error
}

// NotifierRegistry maps channel names to notifier implementations.
type NotifierRegistry struct {
    notifiers map[string]Notifier
}

// NewNotifierRegistry creates an empty registry.
func NewNotifierRegistry() *NotifierRegistry {
    return &NotifierRegistry{
        notifiers: make(map[string]Notifier),
    }
}

// Register adds a notifier. Overwrites if name already registered.
func (r *NotifierRegistry) Register(n Notifier) {
    r.notifiers[n.Name()] = n
}

// Get returns a notifier by channel name, or nil if not found.
func (r *NotifierRegistry) Get(name string) Notifier {
    return r.notifiers[name]
}

// Names returns all registered channel names.
func (r *NotifierRegistry) Names() []string {
    names := make([]string, 0, len(r.notifiers))
    for name := range r.notifiers {
        names = append(names, name)
    }
    return names
}
```

---

## 3. Email Notifier (`internal/alert/notifier/email.go`)

```go
package notifier

import (
    "bytes"
    "context"
    "crypto/tls"
    "fmt"
    "log/slog"
    "mime/multipart"
    "net"
    "net/smtp"
    "net/textproto"
    "time"

    "github.com/ios9000/PGPulse_01/internal/alert"
    "github.com/ios9000/PGPulse_01/internal/config"
)

// EmailNotifier sends alert notifications via SMTP.
type EmailNotifier struct {
    cfg          config.EmailConfig
    dashboardURL string
    logger       *slog.Logger
}

// NewEmailNotifier creates an SMTP-based notifier.
func NewEmailNotifier(cfg config.EmailConfig, dashboardURL string, logger *slog.Logger) *EmailNotifier {
    return &EmailNotifier{
        cfg:          cfg,
        dashboardURL: dashboardURL,
        logger:       logger,
    }
}

// Name returns "email".
func (n *EmailNotifier) Name() string { return "email" }

// Send delivers an alert event via SMTP email.
func (n *EmailNotifier) Send(ctx context.Context, event alert.AlertEvent) error {
    // Build email content
    subject := alert.FormatSubject(event)
    htmlBody, err := alert.RenderHTMLTemplate(event, n.dashboardURL)
    if err != nil {
        return fmt.Errorf("render html template: %w", err)
    }
    textBody, err := alert.RenderTextTemplate(event, n.dashboardURL)
    if err != nil {
        return fmt.Errorf("render text template: %w", err)
    }

    // Build multipart/alternative MIME message
    msg, boundary := buildMIMEMessage(n.cfg.From, n.cfg.Recipients, subject, textBody, htmlBody)

    // Send with timeout
    sendCtx, cancel := context.WithTimeout(ctx, time.Duration(n.cfg.SendTimeoutSeconds)*time.Second)
    defer cancel()

    return n.sendSMTP(sendCtx, msg, boundary)
}
```

### SMTP Send Implementation

```go
// sendSMTP connects to the SMTP server and delivers the message.
func (n *EmailNotifier) sendSMTP(ctx context.Context, msg []byte, boundary string) error {
    addr := fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port)

    // Dial with context timeout
    dialer := &net.Dialer{}
    conn, err := dialer.DialContext(ctx, "tcp", addr)
    if err != nil {
        return fmt.Errorf("smtp dial %s: %w", addr, err)
    }

    client, err := smtp.NewClient(conn, n.cfg.Host)
    if err != nil {
        conn.Close()
        return fmt.Errorf("smtp client: %w", err)
    }
    defer client.Close()

    // STARTTLS if supported
    if ok, _ := client.Extension("STARTTLS"); ok {
        tlsConfig := &tls.Config{
            ServerName:         n.cfg.Host,
            InsecureSkipVerify: n.cfg.TLSSkipVerify,
        }
        if err := client.StartTLS(tlsConfig); err != nil {
            return fmt.Errorf("smtp starttls: %w", err)
        }
    }

    // Auth if credentials provided
    if n.cfg.Username != "" {
        auth := smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
        if err := client.Auth(auth); err != nil {
            return fmt.Errorf("smtp auth: %w", err)
        }
    }

    // Send
    if err := client.Mail(n.cfg.From); err != nil {
        return fmt.Errorf("smtp mail from: %w", err)
    }
    for _, rcpt := range n.cfg.Recipients {
        if err := client.Rcpt(rcpt); err != nil {
            return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
        }
    }

    w, err := client.Data()
    if err != nil {
        return fmt.Errorf("smtp data: %w", err)
    }
    if _, err := w.Write(msg); err != nil {
        return fmt.Errorf("smtp write: %w", err)
    }
    if err := w.Close(); err != nil {
        return fmt.Errorf("smtp close data: %w", err)
    }

    return client.Quit()
}
```

### MIME Message Builder

```go
// buildMIMEMessage constructs a multipart/alternative email with text + HTML parts.
func buildMIMEMessage(from string, to []string, subject, textBody, htmlBody string) ([]byte, string) {
    var buf bytes.Buffer

    // Headers
    buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
    for _, rcpt := range to {
        buf.WriteString(fmt.Sprintf("To: %s\r\n", rcpt))
    }
    buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
    buf.WriteString("MIME-Version: 1.0\r\n")

    // Multipart boundary
    writer := multipart.NewWriter(&buf)
    boundary := writer.Boundary()
    buf.Reset() // we'll rebuild with the known boundary

    // Rebuild with explicit boundary in Content-Type header
    var msg bytes.Buffer
    msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
    for _, rcpt := range to {
        msg.WriteString(fmt.Sprintf("To: %s\r\n", rcpt))
    }
    msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
    msg.WriteString("MIME-Version: 1.0\r\n")
    msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
    msg.WriteString("\r\n")

    // Text part
    msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
    msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
    msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
    msg.WriteString(textBody)
    msg.WriteString("\r\n")

    // HTML part
    msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
    msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
    msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
    msg.WriteString(htmlBody)
    msg.WriteString("\r\n")

    // Close
    msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

    return msg.Bytes(), boundary
}
```

### Design Note on Package Structure

The email notifier lives in `internal/alert/notifier/email.go` (sub-package) to keep the
`internal/alert/` package focused on domain types. The sub-package imports from `internal/alert`
(for AlertEvent, template functions) and `internal/config` (for EmailConfig). This matches
the Strategy doc's module ownership: notifiers are in API & Security Agent territory, but
for M4_02 they're a natural extension of the alert package.

When M7+ adds Telegram/Slack, they go in:
- `internal/alert/notifier/telegram.go`
- `internal/alert/notifier/slack.go`
- `internal/alert/notifier/webhook.go`

All implement the same `alert.Notifier` interface.

### Testability

For testing, the email notifier's SMTP interaction is tested via:
1. **Mock SMTP server** — a `net.Listener` that accepts connections and records received messages
2. **Interface approach** — extract an `smtpSender` interface if needed for finer-grained mocking

Option 1 (mock server) is preferred — it tests the actual SMTP protocol flow end-to-end without
needing a real mail server.

---

## 4. Email Templates (`internal/alert/template.go`)

Uses Go `text/template` and `html/template` from stdlib.

```go
package alert

import (
    "bytes"
    "fmt"
    "html/template"
    "strings"
    texttemplate "text/template"
    "time"
)

// templateData holds all fields available to email templates.
type templateData struct {
    Event        AlertEvent
    DashboardURL string
    SeverityColor string
    SeverityEmoji string
    Duration      string   // for resolutions: how long the alert was firing
    Timestamp     string   // formatted fired_at
    ResolvedTime  string   // formatted resolved_at (empty for fires)
    LabelPairs    []labelPair
}

type labelPair struct {
    Key   string
    Value string
}

// FormatSubject returns the email subject line for an alert event.
func FormatSubject(event AlertEvent) string {
    tag := strings.ToUpper(string(event.Severity))
    if event.IsResolution {
        tag = "RESOLVED"
    }
    return fmt.Sprintf("[%s] %s — %s", tag, event.RuleName, event.InstanceID)
}
```

### HTML Template

```go
var fireHTMLTemplate = template.Must(template.New("fire_html").Parse(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5;">
  <div style="max-width: 600px; margin: 0 auto; background-color: #ffffff;">
    <!-- Severity Header -->
    <div style="background-color: {{.SeverityColor}}; padding: 16px 24px; color: #ffffff;">
      <h2 style="margin: 0; font-size: 18px;">{{.SeverityEmoji}} Alert: {{.Event.RuleName}}</h2>
    </div>

    <!-- Details Table -->
    <div style="padding: 24px;">
      <table style="width: 100%; border-collapse: collapse; font-size: 14px;">
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600; width: 140px;">Instance</td>
          <td style="padding: 8px 12px;">{{.Event.InstanceID}}</td>
        </tr>
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">Metric</td>
          <td style="padding: 8px 12px;"><code>{{.Event.Metric}}</code></td>
        </tr>
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">Current Value</td>
          <td style="padding: 8px 12px; font-weight: 700; color: {{.SeverityColor}};">{{printf "%.2f" .Event.Value}}</td>
        </tr>
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">Threshold</td>
          <td style="padding: 8px 12px;">{{.Event.Operator}} {{printf "%.2f" .Event.Threshold}}</td>
        </tr>
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">Severity</td>
          <td style="padding: 8px 12px;">
            <span style="background-color: {{.SeverityColor}}; color: #fff; padding: 2px 8px; border-radius: 4px; font-size: 12px;">
              {{.Event.Severity}}
            </span>
          </td>
        </tr>
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">Fired At</td>
          <td style="padding: 8px 12px;">{{.Timestamp}}</td>
        </tr>
        {{range .LabelPairs}}
        <tr style="border-bottom: 1px solid #e0e0e0;">
          <td style="padding: 8px 12px; color: #666; font-weight: 600;">{{.Key}}</td>
          <td style="padding: 8px 12px;">{{.Value}}</td>
        </tr>
        {{end}}
      </table>

      {{if .DashboardURL}}
      <div style="margin-top: 20px;">
        <a href="{{.DashboardURL}}/instances/{{.Event.InstanceID}}"
           style="display: inline-block; padding: 10px 20px; background-color: #2563eb; color: #ffffff; text-decoration: none; border-radius: 6px; font-size: 14px;">
          View in PGPulse Dashboard
        </a>
      </div>
      {{end}}
    </div>

    <!-- Footer -->
    <div style="padding: 16px 24px; background-color: #f9fafb; border-top: 1px solid #e0e0e0; font-size: 12px; color: #999;">
      Sent by PGPulse Alert Engine
    </div>
  </div>
</body>
</html>`))
```

Resolution template follows same structure with green header, "RESOLVED" title, and duration field.

### Severity Colors and Emoji

```go
func severityColor(s Severity) string {
    switch s {
    case SeverityCritical:
        return "#dc2626" // red-600
    case SeverityWarning:
        return "#d97706" // amber-600
    case SeverityInfo:
        return "#2563eb" // blue-600
    default:
        return "#6b7280" // gray-500
    }
}

func severityEmoji(s Severity) string {
    switch s {
    case SeverityCritical:
        return "🔴"
    case SeverityWarning:
        return "🟠"
    case SeverityInfo:
        return "🔵"
    default:
        return "⚪"
    }
}

// For resolution:
// Color: "#16a34a" (green-600)
// Emoji: "✅"
```

### Render Functions

```go
// RenderHTMLTemplate renders the HTML email body for an alert event.
func RenderHTMLTemplate(event AlertEvent, dashboardURL string) (string, error) {
    data := buildTemplateData(event, dashboardURL)
    tmpl := fireHTMLTemplate
    if event.IsResolution {
        tmpl = resolveHTMLTemplate
    }
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", fmt.Errorf("execute html template: %w", err)
    }
    return buf.String(), nil
}

// RenderTextTemplate renders the plain text email body for an alert event.
func RenderTextTemplate(event AlertEvent, dashboardURL string) (string, error) {
    data := buildTemplateData(event, dashboardURL)
    tmpl := fireTextTemplate
    if event.IsResolution {
        tmpl = resolveTextTemplate
    }
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", fmt.Errorf("execute text template: %w", err)
    }
    return buf.String(), nil
}

// buildTemplateData prepares the template context from an AlertEvent.
func buildTemplateData(event AlertEvent, dashboardURL string) templateData {
    data := templateData{
        Event:         event,
        DashboardURL:  dashboardURL,
        SeverityColor: severityColor(event.Severity),
        SeverityEmoji: severityEmoji(event.Severity),
        Timestamp:     event.FiredAt.Format(time.RFC3339),
    }
    if event.IsResolution {
        data.SeverityColor = "#16a34a" // green
        data.SeverityEmoji = "✅"
        if event.ResolvedAt != nil {
            data.ResolvedTime = event.ResolvedAt.Format(time.RFC3339)
            data.Duration = event.ResolvedAt.Sub(event.FiredAt).Round(time.Second).String()
        }
    }
    for k, v := range event.Labels {
        data.LabelPairs = append(data.LabelPairs, labelPair{Key: k, Value: v})
    }
    // Sort label pairs for deterministic output
    sort.Slice(data.LabelPairs, func(i, j int) bool {
        return data.LabelPairs[i].Key < data.LabelPairs[j].Key
    })
    return data
}
```

---

## 5. Dispatcher (`internal/alert/dispatcher.go`)

```go
package alert

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"
)

const (
    defaultBufferSize  = 100
    defaultDrainTimeout = 5 * time.Second
    maxRetries         = 3
)

// retryDelays defines exponential backoff intervals.
var retryDelays = [maxRetries]time.Duration{
    1 * time.Second,
    2 * time.Second,
    4 * time.Second,
}

// Dispatcher receives alert events and delivers them to registered notifiers.
// It runs as an async goroutine, enforces cooldown, and retries failed sends.
type Dispatcher struct {
    registry        *NotifierRegistry
    defaultChannels []string
    cooldownMinutes int
    logger          *slog.Logger

    events chan AlertEvent
    done   chan struct{}
    wg     sync.WaitGroup

    mu       sync.Mutex
    cooldowns map[string]time.Time // key: "ruleID:instanceID:severity" → last notified
}

// NewDispatcher creates a dispatcher with the given notifier registry.
func NewDispatcher(
    registry *NotifierRegistry,
    defaultChannels []string,
    defaultCooldownMinutes int,
    logger *slog.Logger,
) *Dispatcher {
    return &Dispatcher{
        registry:        registry,
        defaultChannels: defaultChannels,
        cooldownMinutes: defaultCooldownMinutes,
        logger:          logger,
        events:          make(chan AlertEvent, defaultBufferSize),
        done:            make(chan struct{}),
        cooldowns:       make(map[string]time.Time),
    }
}

// Start begins the dispatcher goroutine. Call Stop() to shut down.
func (d *Dispatcher) Start() {
    d.wg.Add(1)
    go d.run()
    d.logger.Info("alert dispatcher started", "buffer_size", defaultBufferSize)
}

// Dispatch enqueues an alert event for delivery. Non-blocking.
// Returns false if the buffer is full and the event was dropped.
func (d *Dispatcher) Dispatch(event AlertEvent) bool {
    select {
    case d.events <- event:
        return true
    default:
        d.logger.Warn("alert dispatcher buffer full, dropping event",
            "rule", event.RuleID,
            "instance", event.InstanceID,
            "severity", event.Severity,
        )
        return false
    }
}

// Stop signals the dispatcher to drain remaining events and shut down.
func (d *Dispatcher) Stop() {
    close(d.events)
    d.wg.Wait()
    d.logger.Info("alert dispatcher stopped")
}

// run is the main dispatcher loop.
func (d *Dispatcher) run() {
    defer d.wg.Done()

    for event := range d.events {
        d.processEvent(event)
    }
}

// processEvent handles cooldown check, channel resolution, and delivery.
func (d *Dispatcher) processEvent(event AlertEvent) {
    // Cooldown check (resolutions always pass)
    if !event.IsResolution && d.isCoolingDown(event) {
        d.logger.Debug("alert notification suppressed by cooldown",
            "rule", event.RuleID,
            "instance", event.InstanceID,
        )
        return
    }

    // Resolve channels
    channels := d.resolveChannels(event)
    if len(channels) == 0 {
        d.logger.Warn("no notification channels resolved for alert",
            "rule", event.RuleID,
            "channels_requested", event.Channels,
            "default_channels", d.defaultChannels,
        )
        return
    }

    // Fan out to notifiers
    for _, chName := range channels {
        notifier := d.registry.Get(chName)
        if notifier == nil {
            d.logger.Warn("notifier not registered",
                "channel", chName,
                "rule", event.RuleID,
            )
            continue
        }
        d.sendWithRetry(notifier, event)
    }

    // Record cooldown timestamp
    if !event.IsResolution {
        d.recordCooldown(event)
    }
}

// isCoolingDown checks if a notification was recently sent for this rule+instance+severity.
func (d *Dispatcher) isCoolingDown(event AlertEvent) bool {
    key := cooldownKey(event.RuleID, event.InstanceID, string(event.Severity))
    cooldownDuration := time.Duration(d.cooldownMinutes) * time.Minute

    d.mu.Lock()
    lastNotified, exists := d.cooldowns[key]
    d.mu.Unlock()

    if !exists {
        return false
    }
    return time.Since(lastNotified) < cooldownDuration
}

// recordCooldown stores the notification timestamp for cooldown tracking.
func (d *Dispatcher) recordCooldown(event AlertEvent) {
    key := cooldownKey(event.RuleID, event.InstanceID, string(event.Severity))
    d.mu.Lock()
    d.cooldowns[key] = time.Now()
    d.mu.Unlock()
}

// resolveChannels determines which notifiers should handle this event.
func (d *Dispatcher) resolveChannels(event AlertEvent) []string {
    if len(event.Channels) > 0 {
        return event.Channels
    }
    return d.defaultChannels
}

// sendWithRetry attempts to send via a notifier with exponential backoff.
func (d *Dispatcher) sendWithRetry(n Notifier, event AlertEvent) {
    for attempt := 0; attempt < maxRetries; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        err := n.Send(ctx, event)
        cancel()

        if err == nil {
            d.logger.Info("alert notification sent",
                "channel", n.Name(),
                "rule", event.RuleID,
                "instance", event.InstanceID,
                "severity", event.Severity,
                "attempt", attempt+1,
            )
            return
        }

        d.logger.Warn("alert notification failed",
            "channel", n.Name(),
            "rule", event.RuleID,
            "instance", event.InstanceID,
            "attempt", attempt+1,
            "max_attempts", maxRetries,
            "error", err,
        )

        if attempt < maxRetries-1 {
            time.Sleep(retryDelays[attempt])
        }
    }

    d.logger.Error("alert notification exhausted retries",
        "channel", n.Name(),
        "rule", event.RuleID,
        "instance", event.InstanceID,
        "severity", event.Severity,
    )
}

// cooldownKey builds the map key for cooldown tracking.
func cooldownKey(ruleID, instanceID, severity string) string {
    return ruleID + ":" + instanceID + ":" + severity
}
```

### Design Notes — Dispatcher

1. **Non-blocking Dispatch()**: Uses `select` with `default` — if the channel buffer (100) is full, the event is dropped and a warning is logged. The evaluator never blocks.

2. **Cooldown in dispatcher, not evaluator**: The evaluator emits fire events only on state transition (PENDING→FIRING). The dispatcher enforces cooldown to prevent duplicate notifications if the evaluator is somehow called twice for the same transition or if a future "reminder" feature re-emits events. Resolution events always bypass cooldown.

3. **Per-rule cooldown override**: Currently uses global `cooldownMinutes`. To use per-rule cooldown, the AlertEvent would need to carry the cooldown value from the rule. This can be added to AlertEvent in a later iteration if needed — for now, the global default covers all cases.

4. **Retry context**: Each send attempt gets a 15-second context (send_timeout from config + overhead). If the notifier's own timeout fires first, the retry context doesn't interfere.

5. **Channel draining**: When `Stop()` is called, `close(d.events)` causes `range d.events` to drain remaining items, then the goroutine exits. The `wg.Wait()` in Stop() blocks until draining completes. No separate drain timeout needed because the channel is finite (buffer size 100) and each send is bounded by retry timeout.

6. **Cooldown map cleanup**: The cooldown map can grow unbounded over time. For MVP this is acceptable (monitoring tools have limited rule×instance combinations). A periodic cleanup goroutine can be added in M7 if needed.

---

## 6. Config Additions

### EmailConfig

```go
// Add to internal/config/config.go:

type EmailConfig struct {
    Host               string   `koanf:"host"`
    Port               int      `koanf:"port"`
    Username           string   `koanf:"username"`
    Password           string   `koanf:"password"`
    From               string   `koanf:"from"`
    Recipients         []string `koanf:"recipients"`
    TLSSkipVerify      bool     `koanf:"tls_skip_verify"`
    SendTimeoutSeconds int      `koanf:"send_timeout_seconds"`
}

// Update AlertingConfig:
type AlertingConfig struct {
    Enabled                bool        `koanf:"enabled"`
    DefaultConsecutiveCount int        `koanf:"default_consecutive_count"`
    DefaultCooldownMinutes  int        `koanf:"default_cooldown_minutes"`
    DefaultChannels         []string   `koanf:"default_channels"`
    EvaluationTimeoutSec    int        `koanf:"evaluation_timeout_seconds"`
    HistoryRetentionDays    int        `koanf:"history_retention_days"`
    DashboardURL            string     `koanf:"dashboard_url"`
    Email                   *EmailConfig `koanf:"email"`
}
```

### Defaults

```go
func alertingDefaults(c *AlertingConfig) {
    // ... existing defaults from M4_01 ...
    if c.Email != nil {
        if c.Email.Port == 0 {
            c.Email.Port = 587
        }
        if c.Email.SendTimeoutSeconds == 0 {
            c.Email.SendTimeoutSeconds = 10
        }
    }
}
```

### Validation

```go
func validateAlerting(c *AlertingConfig) error {
    // ... existing validation from M4_01 ...

    // If "email" is in default_channels, email config must be present
    for _, ch := range c.DefaultChannels {
        if ch == "email" {
            if c.Email == nil {
                return fmt.Errorf("alerting.email config required when 'email' is in default_channels")
            }
            if c.Email.Host == "" {
                return fmt.Errorf("alerting.email.host is required")
            }
            if c.Email.From == "" {
                return fmt.Errorf("alerting.email.from is required")
            }
            if len(c.Email.Recipients) == 0 {
                return fmt.Errorf("alerting.email.recipients must not be empty")
            }
        }
    }
    return nil
}
```

### Example YAML

```yaml
alerting:
  enabled: true
  default_channels: ["email"]
  default_consecutive_count: 3
  default_cooldown_minutes: 15
  evaluation_timeout_seconds: 5
  history_retention_days: 30
  dashboard_url: "https://pgpulse.corp.example.com"

  email:
    host: "smtp.corp.example.com"
    port: 587
    username: "pgpulse@corp.example.com"
    password: "secret"
    from: "PGPulse Alerts <pgpulse@corp.example.com>"
    recipients:
      - "dba-team@corp.example.com"
      - "oncall@corp.example.com"
    tls_skip_verify: false
    send_timeout_seconds: 10
```

---

## 7. Dependency Graph

```
internal/alert/notifier.go     ← imports alert.go (AlertEvent)
internal/alert/template.go     ← imports alert.go (AlertEvent, Severity)
internal/alert/dispatcher.go   ← imports alert.go (AlertEvent), notifier.go (NotifierRegistry, Notifier)

internal/alert/notifier/
  email.go                     ← imports alert (AlertEvent, templates), config (EmailConfig), net/smtp
  email_test.go                ← imports alert, net (mock server)

internal/config/config.go      ← adds EmailConfig (no new external imports)
```

The alert package does NOT gain any new external dependencies. The email notifier uses only
Go stdlib (`net/smtp`, `crypto/tls`, `mime/multipart`).

---

## 8. Test Plan

### Template Tests (`internal/alert/template_test.go`)

| Test | What it validates |
|------|-------------------|
| TestFormatSubject_Fire | Subject contains severity tag, rule name, instance ID |
| TestFormatSubject_Resolve | Subject contains "[RESOLVED]" |
| TestRenderHTMLTemplate_Fire | HTML contains severity color, metric value, threshold, instance ID |
| TestRenderHTMLTemplate_Resolve | HTML contains green color, "RESOLVED", duration |
| TestRenderHTMLTemplate_WithLabels | Label key-value pairs appear in table |
| TestRenderHTMLTemplate_NoDashboardURL | Dashboard link section omitted when URL empty |
| TestRenderTextTemplate_Fire | Plain text is readable, contains all fields |
| TestRenderTextTemplate_Resolve | Plain text contains resolved timestamp and duration |

### Dispatcher Tests (`internal/alert/dispatcher_test.go`)

| Test | What it validates |
|------|-------------------|
| TestDispatcher_RoutesToCorrectNotifier | Event with channel="email" → email notifier's Send called |
| TestDispatcher_UsesDefaultChannels | Event with empty channels → uses default_channels |
| TestDispatcher_PerRuleChannelOverride | Event with channels=["email"] overrides defaults |
| TestDispatcher_CooldownSuppresses | Second fire within cooldown → suppressed, not sent |
| TestDispatcher_CooldownAllowsResolution | Resolution within cooldown → sent anyway |
| TestDispatcher_BufferOverflow | Fill buffer, dispatch returns false, no block |
| TestDispatcher_GracefulShutdown | Stop() drains remaining events, notifier receives all |
| TestDispatcher_RetryOnFailure | Notifier fails twice, succeeds third → 3 Send calls |
| TestDispatcher_RetryExhausted | Notifier fails all 3 → error logged, no panic |
| TestDispatcher_MultipleNotifiers | Two notifiers registered, both receive event |
| TestDispatcher_UnknownChannel | Event requests unregistered channel → warning logged |

Test helpers:
```go
// mockNotifier records Send calls for assertions.
type mockNotifier struct {
    name     string
    calls    []AlertEvent
    failN    int  // fail first N calls
    mu       sync.Mutex
}
```

### Email Notifier Tests (`internal/alert/notifier/email_test.go`)

| Test | What it validates |
|------|-------------------|
| TestEmailNotifier_Send | Mock SMTP server receives message with correct headers |
| TestEmailNotifier_MultipartContent | Message contains both text/plain and text/html parts |
| TestEmailNotifier_STARTTLSAttempt | Client attempts STARTTLS when server supports it |
| TestEmailNotifier_AuthWhenConfigured | PLAIN auth sent when username/password set |
| TestEmailNotifier_NoAuthWhenEmpty | No auth attempt when credentials empty |
| TestEmailNotifier_MultipleRecipients | All recipients receive RCPT TO |

Mock SMTP server pattern:
```go
func startMockSMTP(t *testing.T) (addr string, received chan []byte) {
    // net.Listen on :0, accept one connection
    // Speak minimal SMTP (220 greeting, EHLO, MAIL, RCPT, DATA, QUIT)
    // Capture DATA content into received channel
}
```

### Config Tests (additions to `internal/config/config_test.go`)

| Test | What it validates |
|------|-------------------|
| TestAlertingConfig_EmailRequired | email in default_channels + no email config → error |
| TestAlertingConfig_EmailDefaults | port defaults to 587, timeout to 10 |
| TestAlertingConfig_EmailValid | full email config passes validation |

---

## 9. Files to Create/Modify Summary

### New Files

| File | Lines (est.) | Description |
|------|-------------|-------------|
| `internal/alert/notifier.go` | ~50 | Notifier interface + NotifierRegistry |
| `internal/alert/template.go` | ~200 | HTML + text templates, render functions, helpers |
| `internal/alert/dispatcher.go` | ~200 | Async dispatcher with cooldown, routing, retry |
| `internal/alert/notifier/email.go` | ~180 | SMTP notifier + MIME builder |
| `internal/alert/template_test.go` | ~150 | Template rendering tests |
| `internal/alert/dispatcher_test.go` | ~250 | Dispatcher routing/cooldown/retry/shutdown tests |
| `internal/alert/notifier/email_test.go` | ~200 | Mock SMTP server tests |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add EmailConfig, update AlertingConfig (DefaultChannels, DashboardURL, Email) |
| `internal/config/load.go` | Add email defaults and validation |
| `internal/config/config_test.go` | +3 tests for email config |
| `configs/pgpulse.example.yml` | Add email + dashboard_url + default_channels |

### NOT Modified

| File | Why not |
|------|---------|
| `internal/alert/evaluator.go` | Evaluator unchanged — dispatcher is independent |
| `internal/alert/alert.go` | Data model unchanged |
| `cmd/pgpulse-server/main.go` | Wiring in M4_03 |
| `internal/orchestrator/*` | Hook in M4_03 |
| `internal/api/*` | Alert API in M4_03 |
