package notifier

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/config"
)

func newEmailTestEvent() alert.AlertEvent {
	return alert.AlertEvent{
		RuleID:     "rule-conn-count",
		RuleName:   "High Connection Count",
		InstanceID: "prod-db-01",
		Severity:   alert.SeverityWarning,
		Value:      95.50,
		Threshold:  80.00,
		Operator:   alert.OpGreater,
		Metric:     "pg_connections_active",
		FiredAt:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestEmailNotifier_Name(t *testing.T) {
	n := NewEmailNotifier(config.EmailConfig{}, "", slog.Default())
	if got := n.Name(); got != "email" {
		t.Errorf("Name() = %q, want %q", got, "email")
	}
}

// smtpWrite writes an SMTP response line to conn.
func smtpWrite(conn net.Conn, s string) {
	_, _ = conn.Write([]byte(s + "\r\n"))
}

func parseHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}

// startMockSMTP starts a minimal SMTP server on a random port.
// It captures the DATA content into the returned buffer.
func startMockSMTP(t *testing.T) (addr string, received *bytes.Buffer) {
	t.Helper()
	received = &bytes.Buffer{}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr = ln.Addr().String()
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, cErr := ln.Accept()
		if cErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

		reader := bufio.NewReader(conn)
		smtpWrite(conn, "220 localhost SMTP ready")

		for {
			line, rErr := reader.ReadString('\n')
			if rErr != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))

			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				smtpWrite(conn, "250-localhost Hello")
				smtpWrite(conn, "250 OK")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				smtpWrite(conn, "250 OK")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				smtpWrite(conn, "250 OK")
			case cmd == "DATA":
				smtpWrite(conn, "354 Start mail input")
				for {
					dataLine, dErr := reader.ReadString('\n')
					if dErr != nil {
						return
					}
					if strings.TrimSpace(dataLine) == "." {
						break
					}
					received.WriteString(dataLine)
				}
				smtpWrite(conn, "250 OK")
			case cmd == "QUIT":
				smtpWrite(conn, "221 Bye")
				return
			default:
				smtpWrite(conn, "250 OK")
			}
		}
	}()

	return addr, received
}

func TestEmailNotifier_Send_Success(t *testing.T) {
	addr, received := startMockSMTP(t)
	host, port := parseHostPort(t, addr)

	cfg := config.EmailConfig{
		Host:               host,
		Port:               port,
		From:               "alerts@pgpulse.local",
		Recipients:         []string{"admin@example.com"},
		SendTimeoutSeconds: 5,
	}
	n := NewEmailNotifier(cfg, "https://dashboard.example.com", slog.Default())

	ev := newEmailTestEvent()
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msg := received.String()
	checks := []struct {
		label string
		want  string
	}{
		{"from header", "From: alerts@pgpulse.local"},
		{"to header", "To: admin@example.com"},
		{"subject", "[WARNING] High Connection Count"},
		{"MIME version", "MIME-Version: 1.0"},
		{"multipart", "multipart/alternative"},
		{"text part", "text/plain"},
		{"html part", "text/html"},
		{"rule name in body", "High Connection Count"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.want) {
			t.Errorf("SMTP message missing %s: want substring %q", c.label, c.want)
		}
	}
}

func TestEmailNotifier_Send_Timeout(t *testing.T) {
	// Start a server that sends the greeting, reads EHLO, then closes the
	// connection abruptly. This causes an I/O error on the client side,
	// verifying that Send() propagates errors correctly.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, cErr := ln.Accept()
		if cErr != nil {
			return
		}
		// Send greeting so smtp.NewClient returns quickly.
		smtpWrite(conn, "220 localhost SMTP ready")
		// Read the EHLO command then close connection immediately.
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		_ = conn.Close()
	}()

	host, port := parseHostPort(t, ln.Addr().String())

	cfg := config.EmailConfig{
		Host:               host,
		Port:               port,
		From:               "alerts@pgpulse.local",
		Recipients:         []string{"admin@example.com"},
		SendTimeoutSeconds: 5,
	}
	n := NewEmailNotifier(cfg, "", slog.Default())

	ev := newEmailTestEvent()
	if err := n.Send(context.Background(), ev); err == nil {
		t.Error("Send() expected error (connection closed), got nil")
	}
}

func TestBuildMIMEMessage(t *testing.T) {
	msg := buildMIMEMessage(
		"sender@example.com",
		[]string{"rcpt1@example.com", "rcpt2@example.com"},
		"Test Subject",
		"Hello plain text",
		"<p>Hello HTML</p>",
	)

	s := string(msg)
	checks := []struct {
		label string
		want  string
	}{
		{"from header", "From: sender@example.com"},
		{"to header", "To: rcpt1@example.com, rcpt2@example.com"},
		{"subject header", "Subject: Test Subject"},
		{"MIME version", "MIME-Version: 1.0"},
		{"content type multipart", "Content-Type: multipart/alternative"},
		{"text part type", "text/plain"},
		{"html part type", "text/html"},
		{"text body", "Hello plain text"},
		{"html body", "<p>Hello HTML</p>"},
	}

	for _, c := range checks {
		if !strings.Contains(s, c.want) {
			t.Errorf("MIME message missing %s: want substring %q", c.label, c.want)
		}
	}
}
