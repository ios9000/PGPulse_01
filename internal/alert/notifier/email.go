package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// EmailNotifier delivers alert notifications via SMTP email.
type EmailNotifier struct {
	cfg          config.EmailConfig
	dashboardURL string
	logger       *slog.Logger
}

// NewEmailNotifier creates an email notifier with the given SMTP configuration.
func NewEmailNotifier(cfg config.EmailConfig, dashboardURL string, logger *slog.Logger) *EmailNotifier {
	return &EmailNotifier{cfg: cfg, dashboardURL: dashboardURL, logger: logger}
}

// Name returns the channel name for this notifier.
func (n *EmailNotifier) Name() string { return "email" }

// Send renders and delivers an alert event as a MIME email.
func (n *EmailNotifier) Send(ctx context.Context, event alert.AlertEvent) error {
	subject := alert.FormatSubject(event)
	htmlBody, err := alert.RenderHTMLTemplate(event, n.dashboardURL, event.Recommendations)
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	textBody, err := alert.RenderTextTemplate(event, n.dashboardURL, event.Recommendations)
	if err != nil {
		return fmt.Errorf("render text: %w", err)
	}

	msg := buildMIMEMessage(n.cfg.From, n.cfg.Recipients, subject, textBody, htmlBody)

	sendCtx, cancel := context.WithTimeout(ctx, time.Duration(n.cfg.SendTimeoutSeconds)*time.Second)
	defer cancel()

	return n.sendSMTP(sendCtx, msg)
}

func (n *EmailNotifier) sendSMTP(ctx context.Context, msg []byte) error {
	host, port, err := net.SplitHostPort(fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port))
	if err != nil {
		return fmt.Errorf("invalid smtp address: %w", err)
	}
	addr := net.JoinHostPort(host, port)

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// STARTTLS if the server supports it.
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: n.cfg.TLSSkipVerify, //nolint:gosec // configurable per deployment
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	// Authenticate if credentials are provided.
	if n.cfg.Username != "" {
		auth := smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(n.cfg.From); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range n.cfg.Recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %q: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}

	return client.Quit()
}

// buildMIMEMessage constructs a multipart/alternative MIME email with text and HTML parts.
func buildMIMEMessage(from string, to []string, subject, textBody, htmlBody string) []byte {
	var buf strings.Builder

	// Create a multipart writer to generate the boundary.
	// We write the parts manually into the buffer using the generated boundary.
	mpBuf := &strings.Builder{}
	mp := multipart.NewWriter(mpBuf)
	boundary := mp.Boundary()

	// Headers.
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	buf.WriteString("\r\n")

	// Text part.
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(textBody)
	buf.WriteString("\r\n")

	// HTML part.
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)
	buf.WriteString("\r\n")

	// Closing boundary.
	buf.WriteString("--" + boundary + "--\r\n")

	return []byte(buf.String())
}
