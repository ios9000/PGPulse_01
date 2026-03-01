package alert

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	texttemplate "text/template"
	"time"
)

type templateData struct {
	Event         AlertEvent
	DashboardURL  string
	SeverityColor string
	SeverityEmoji string
	Duration      string
	Timestamp     string
	ResolvedTime  string
	LabelPairs    []labelPair
}

type labelPair struct {
	Key   string
	Value string
}

// FormatSubject returns a formatted email subject line for the alert event.
func FormatSubject(event AlertEvent) string {
	tag := strings.ToUpper(string(event.Severity))
	if event.IsResolution {
		tag = "RESOLVED"
	}
	return fmt.Sprintf("[%s] %s — %s", tag, event.RuleName, event.InstanceID)
}

func severityColor(s Severity) string {
	switch s {
	case SeverityCritical:
		return "#dc2626"
	case SeverityWarning:
		return "#d97706"
	case SeverityInfo:
		return "#2563eb"
	default:
		return "#6b7280"
	}
}

func severityEmoji(s Severity) string {
	switch s {
	case SeverityCritical:
		return "\xf0\x9f\x94\xb4" // red circle
	case SeverityWarning:
		return "\xf0\x9f\x9f\xa0" // orange circle
	case SeverityInfo:
		return "\xf0\x9f\x94\xb5" // blue circle
	default:
		return "\xe2\x9a\xaa" // white circle
	}
}

// RenderHTMLTemplate renders the appropriate HTML email body for the event.
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

// RenderTextTemplate renders the appropriate plain-text email body for the event.
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

func buildTemplateData(event AlertEvent, dashboardURL string) templateData {
	data := templateData{
		Event:         event,
		DashboardURL:  dashboardURL,
		SeverityColor: severityColor(event.Severity),
		SeverityEmoji: severityEmoji(event.Severity),
		Timestamp:     event.FiredAt.Format(time.RFC3339),
	}
	if event.IsResolution {
		data.SeverityColor = "#16a34a"
		data.SeverityEmoji = "\xe2\x9c\x85" // checkmark
		if event.ResolvedAt != nil {
			data.ResolvedTime = event.ResolvedAt.Format(time.RFC3339)
			data.Duration = event.ResolvedAt.Sub(event.FiredAt).Round(time.Second).String()
		}
	}
	for k, v := range event.Labels {
		data.LabelPairs = append(data.LabelPairs, labelPair{Key: k, Value: v})
	}
	sort.Slice(data.LabelPairs, func(i, j int) bool {
		return data.LabelPairs[i].Key < data.LabelPairs[j].Key
	})
	return data
}

var fireHTMLTemplate = template.Must(template.New("fire_html").Funcs(template.FuncMap{
	"printf": fmt.Sprintf,
}).Parse(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;font-family:Arial,Helvetica,sans-serif;background:#f9fafb;">
<table width="100%" cellpadding="0" cellspacing="0" style="max-width:600px;margin:0 auto;">
  <tr>
    <td style="background:{{.SeverityColor}};color:#fff;padding:16px 24px;font-size:18px;font-weight:bold;">
      {{.SeverityEmoji}} Alert: {{.Event.RuleName}}
    </td>
  </tr>
  <tr>
    <td style="background:#fff;padding:24px;">
      <table width="100%" cellpadding="8" cellspacing="0" style="border-collapse:collapse;">
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;width:140px;">Instance</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Event.InstanceID}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Metric</td>
          <td style="border-bottom:1px solid #e5e7eb;"><code style="background:#f3f4f6;padding:2px 6px;border-radius:3px;">{{.Event.Metric}}</code></td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Current Value</td>
          <td style="border-bottom:1px solid #e5e7eb;color:{{.SeverityColor}};font-weight:bold;">{{printf "%.2f" .Event.Value}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Threshold</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Event.Operator}} {{printf "%.2f" .Event.Threshold}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Severity</td>
          <td style="border-bottom:1px solid #e5e7eb;"><span style="background:{{.SeverityColor}};color:#fff;padding:2px 8px;border-radius:4px;font-size:13px;">{{.Event.Severity}}</span></td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Fired At</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Timestamp}}</td>
        </tr>
{{- range .LabelPairs}}
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">{{.Key}}</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Value}}</td>
        </tr>
{{- end}}
      </table>
{{- if .DashboardURL}}
      <div style="margin-top:24px;text-align:center;">
        <a href="{{.DashboardURL}}/instances/{{.Event.InstanceID}}" style="display:inline-block;background:{{.SeverityColor}};color:#fff;padding:10px 24px;text-decoration:none;border-radius:6px;font-weight:bold;">View in Dashboard</a>
      </div>
{{- end}}
    </td>
  </tr>
  <tr>
    <td style="background:#f3f4f6;color:#6b7280;padding:12px 24px;font-size:12px;text-align:center;">
      Sent by PGPulse Alert Engine
    </td>
  </tr>
</table>
</body>
</html>`))

var resolveHTMLTemplate = template.Must(template.New("resolve_html").Funcs(template.FuncMap{
	"printf": fmt.Sprintf,
}).Parse(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;font-family:Arial,Helvetica,sans-serif;background:#f9fafb;">
<table width="100%" cellpadding="0" cellspacing="0" style="max-width:600px;margin:0 auto;">
  <tr>
    <td style="background:#16a34a;color:#fff;padding:16px 24px;font-size:18px;font-weight:bold;">
      {{.SeverityEmoji}} Resolved: {{.Event.RuleName}}
    </td>
  </tr>
  <tr>
    <td style="background:#fff;padding:24px;">
      <table width="100%" cellpadding="8" cellspacing="0" style="border-collapse:collapse;">
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;width:140px;">Instance</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Event.InstanceID}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Metric</td>
          <td style="border-bottom:1px solid #e5e7eb;"><code style="background:#f3f4f6;padding:2px 6px;border-radius:3px;">{{.Event.Metric}}</code></td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Current Value</td>
          <td style="border-bottom:1px solid #e5e7eb;color:#16a34a;font-weight:bold;">{{printf "%.2f" .Event.Value}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Threshold</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Event.Operator}} {{printf "%.2f" .Event.Threshold}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Severity</td>
          <td style="border-bottom:1px solid #e5e7eb;"><span style="background:#16a34a;color:#fff;padding:2px 8px;border-radius:4px;font-size:13px;">resolved</span></td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Fired At</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Timestamp}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Resolved At</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.ResolvedTime}}</td>
        </tr>
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">Duration</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Duration}}</td>
        </tr>
{{- range .LabelPairs}}
        <tr>
          <td style="border-bottom:1px solid #e5e7eb;font-weight:bold;">{{.Key}}</td>
          <td style="border-bottom:1px solid #e5e7eb;">{{.Value}}</td>
        </tr>
{{- end}}
      </table>
{{- if .DashboardURL}}
      <div style="margin-top:24px;text-align:center;">
        <a href="{{.DashboardURL}}/instances/{{.Event.InstanceID}}" style="display:inline-block;background:#16a34a;color:#fff;padding:10px 24px;text-decoration:none;border-radius:6px;font-weight:bold;">View in Dashboard</a>
      </div>
{{- end}}
    </td>
  </tr>
  <tr>
    <td style="background:#f3f4f6;color:#6b7280;padding:12px 24px;font-size:12px;text-align:center;">
      Sent by PGPulse Alert Engine
    </td>
  </tr>
</table>
</body>
</html>`))

var textFuncMap = texttemplate.FuncMap{
	"printf": fmt.Sprintf,
	"upper":  strings.ToUpper,
}

var fireTextTemplate = texttemplate.Must(texttemplate.New("fire_text").Funcs(textFuncMap).Parse(`[{{printf "%s" .Event.Severity | upper}}] Alert: {{.Event.RuleName}}

Instance:      {{.Event.InstanceID}}
Metric:        {{.Event.Metric}}
Current Value: {{printf "%.2f" .Event.Value}}
Threshold:     {{.Event.Operator}} {{printf "%.2f" .Event.Threshold}}
Severity:      {{.Event.Severity}}
Fired At:      {{.Timestamp}}
{{- if .LabelPairs}}
Labels:        {{range $i, $lp := .LabelPairs}}{{if $i}}, {{end}}{{$lp.Key}}={{$lp.Value}}{{end}}
{{- end}}
{{- if .DashboardURL}}

Dashboard: {{.DashboardURL}}/instances/{{.Event.InstanceID}}
{{- end}}

--
Sent by PGPulse Alert Engine
`))

var resolveTextTemplate = texttemplate.Must(texttemplate.New("resolve_text").Funcs(textFuncMap).Parse(`[RESOLVED] Resolved: {{.Event.RuleName}}

Instance:      {{.Event.InstanceID}}
Metric:        {{.Event.Metric}}
Current Value: {{printf "%.2f" .Event.Value}}
Threshold:     {{.Event.Operator}} {{printf "%.2f" .Event.Threshold}}
Severity:      {{.Event.Severity}}
Fired At:      {{.Timestamp}}
Resolved At:   {{.ResolvedTime}}
Duration:      {{.Duration}}
{{- if .LabelPairs}}
Labels:        {{range $i, $lp := .LabelPairs}}{{if $i}}, {{end}}{{$lp.Key}}={{$lp.Value}}{{end}}
{{- end}}
{{- if .DashboardURL}}

Dashboard: {{.DashboardURL}}/instances/{{.Event.InstanceID}}
{{- end}}

--
Sent by PGPulse Alert Engine
`))
