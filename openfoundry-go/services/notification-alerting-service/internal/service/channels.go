// Package service hosts notification creation + per-channel dispatch.
package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
)

// DeliveryResult captures the per-channel outcome of dispatching a single
// notification. Mirrors the Rust DeliveryResult shape.
type DeliveryResult struct {
	Status   string // "sent" | "skipped" | "failed"
	Response string
}

func sent(msg string) DeliveryResult    { return DeliveryResult{Status: "sent", Response: msg} }
func skipped(msg string) DeliveryResult { return DeliveryResult{Status: "skipped", Response: msg} }
func failed(msg string) DeliveryResult  { return DeliveryResult{Status: "failed", Response: msg} }

// SMTPSender owns the small surface notification dispatch needs.
//
// Hand-rolled over net/smtp + crypto/tls to keep the dependency
// footprint small. Uses STARTTLS when the server announces it; falls
// back to plain SMTP otherwise (compatible with mailhog / mailpit /
// dev relays).
type SMTPSender struct {
	Host        string
	Port        uint16
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

// SendEmail sends a text/plain email with `subject` and `body`.
//
// Returns DeliveryResult so callers don't need to distinguish errors
// from "not configured" — the result type carries that information.
func (s *SMTPSender) SendEmail(ctx context.Context, to, subject, body string) DeliveryResult {
	if s.Host == "" {
		return skipped("SMTP adapter not configured")
	}
	if s.FromAddress == "" {
		return skipped("SMTP from address not configured")
	}

	addr := net.JoinHostPort(s.Host, strconv.Itoa(int(s.Port)))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return failed(fmt.Sprintf("dial smtp: %s", err))
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return failed(fmt.Sprintf("smtp client: %s", err))
	}
	defer c.Quit()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return failed(fmt.Sprintf("starttls: %s", err))
		}
	}

	if s.Username != "" && s.Password != "" {
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := c.Auth(auth); err != nil {
			return failed(fmt.Sprintf("smtp auth: %s", err))
		}
	}

	from := s.FromAddress
	if err := c.Mail(from); err != nil {
		return failed(fmt.Sprintf("smtp mail: %s", err))
	}
	if err := c.Rcpt(to); err != nil {
		return failed(fmt.Sprintf("smtp rcpt: %s", err))
	}

	wc, err := c.Data()
	if err != nil {
		return failed(fmt.Sprintf("smtp data: %s", err))
	}

	displayFrom := from
	if s.FromName != "" {
		displayFrom = fmt.Sprintf("%s <%s>", s.FromName, from)
	}
	msg := buildEmailMessage(displayFrom, to, subject, body)
	if _, err := wc.Write([]byte(msg)); err != nil {
		_ = wc.Close()
		return failed(fmt.Sprintf("smtp write: %s", err))
	}
	if err := wc.Close(); err != nil {
		return failed(fmt.Sprintf("smtp close: %s", err))
	}

	return sent(fmt.Sprintf("email delivered to %s", to))
}

func buildEmailMessage(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// PostWebhook does the slack/teams POST.
//
// Returns DeliveryResult — non-success codes are treated as failures
// (matches the Rust behaviour).
func PostWebhook(ctx context.Context, client *http.Client, url string, payload any) DeliveryResult {
	body, err := json.Marshal(payload)
	if err != nil {
		return failed(fmt.Sprintf("encode webhook payload: %s", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return failed(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return failed(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return sent(fmt.Sprintf("webhook delivered with status %d", resp.StatusCode))
	}
	return failed(fmt.Sprintf("webhook returned status %d", resp.StatusCode))
}

// dispatch picks the right adapter for `channel`. Mirrors the Rust
// dispatch_channel match block verbatim.
func (n *Notifier) dispatch(
	ctx context.Context,
	notification *models.NotificationRecord,
	preference *models.NotificationPreference,
	channel string,
) DeliveryResult {
	switch channel {
	case "in_app":
		return sent("delivered to in-app center")
	case "email":
		if preference == nil || !preference.EmailEnabled {
			return skipped("email channel disabled")
		}
		if preference.EmailAddress == nil || *preference.EmailAddress == "" {
			return skipped("email address not configured")
		}
		if n.SMTP == nil {
			return skipped("SMTP adapter not configured")
		}
		return n.SMTP.SendEmail(ctx, *preference.EmailAddress, notification.Title, notification.Body)
	case "slack":
		if preference == nil || preference.SlackWebhookURL == nil || *preference.SlackWebhookURL == "" {
			return skipped("slack webhook not configured")
		}
		return PostWebhook(ctx, n.HTTP, *preference.SlackWebhookURL,
			map[string]string{"text": notification.Title + "\n" + notification.Body})
	case "teams":
		if preference == nil || preference.TeamsWebhookURL == nil || *preference.TeamsWebhookURL == "" {
			return skipped("teams webhook not configured")
		}
		return PostWebhook(ctx, n.HTTP, *preference.TeamsWebhookURL,
			map[string]string{"text": notification.Title + "\n" + notification.Body})
	default:
		return skipped(fmt.Sprintf("unknown channel '%s'", channel))
	}
}

// errAdapter unused — placeholder for future typed errors per channel.
var errAdapter = errors.New("adapter error")
var _ = errAdapter
