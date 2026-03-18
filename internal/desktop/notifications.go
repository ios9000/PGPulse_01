//go:build desktop

package desktop

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

const (
	// defaultNotifCooldown prevents toast spam for the same rule+instance.
	defaultNotifCooldown = 5 * time.Minute
	// defaultMinSeverity filters out low-severity alerts from OS notifications.
	defaultMinSeverity = alert.SeverityWarning
)

// AlertNotifier bridges alert events to OS toast notifications.
type AlertNotifier struct {
	svc         *notifications.NotificationService
	window      *application.WebviewWindow
	mu          sync.Mutex
	lastFired   map[string]time.Time
	cooldown    time.Duration
	minSeverity alert.Severity
	seqID       int64
}

// NewAlertNotifier creates an AlertNotifier that sends toasts via the given
// notification service and focuses the window when the user clicks a toast.
func NewAlertNotifier(svc *notifications.NotificationService, window *application.WebviewWindow) *AlertNotifier {
	an := &AlertNotifier{
		svc:         svc,
		window:      window,
		lastFired:   make(map[string]time.Time),
		cooldown:    defaultNotifCooldown,
		minSeverity: defaultMinSeverity,
	}

	// When the user clicks on a notification, show and focus the main window.
	svc.OnNotificationResponse(func(_ notifications.NotificationResult) {
		if window != nil {
			window.Show().Focus()
		}
	})

	return an
}

// HandleAlert processes an alert event and sends an OS toast if the event
// passes severity and rate-limit checks.
func (an *AlertNotifier) HandleAlert(event alert.AlertEvent) {
	if !an.shouldNotify(event) {
		return
	}

	an.mu.Lock()
	an.seqID++
	id := fmt.Sprintf("pgpulse-alert-%d", an.seqID)
	key := notifCooldownKey(event)
	an.lastFired[key] = time.Now()
	an.mu.Unlock()

	title := fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), event.RuleName)
	body := fmt.Sprintf("Instance: %s\nMetric: %s\nValue: %.2f (threshold: %.2f)",
		event.InstanceID, event.Metric, event.Value, event.Threshold)

	_ = an.svc.SendNotification(notifications.NotificationOptions{
		ID:    id,
		Title: title,
		Body:  body,
	})
}

// shouldNotify checks severity filter and per-rule cooldown.
func (an *AlertNotifier) shouldNotify(event alert.AlertEvent) bool {
	// Skip resolution events — only fire on new alerts.
	if event.IsResolution {
		return false
	}

	// Severity filter.
	if severityRank(event.Severity) < severityRank(an.minSeverity) {
		return false
	}

	// Cooldown check.
	key := notifCooldownKey(event)
	an.mu.Lock()
	last, exists := an.lastFired[key]
	an.mu.Unlock()
	if exists && time.Since(last) < an.cooldown {
		return false
	}

	return true
}

func notifCooldownKey(event alert.AlertEvent) string {
	return event.RuleID + ":" + event.InstanceID
}

// severityRank returns a numeric rank for severity comparison.
func severityRank(s alert.Severity) int {
	switch s {
	case alert.SeverityInfo:
		return 0
	case alert.SeverityWarning:
		return 1
	case alert.SeverityCritical:
		return 2
	default:
		return -1
	}
}
