package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	vloggoconfig "github.com/vinialx/vloggo-go/v2/config"
	"github.com/vinialx/vloggo-go/v2/types"
)

type EmailService struct {
	cfg        types.VLoggoConfig
	mu         sync.Mutex
	lastSent   time.Time
	retryAfter time.Time
	inFlight   bool
	closed     bool
}

func NewEmailService(cfg types.VLoggoConfig) *EmailService {
	return &EmailService{cfg: cloneEmailConfig(cfg)}
}

func cloneEmailConfig(cfg types.VLoggoConfig) types.VLoggoConfig {
	cfg.SMTP.To = append([]string(nil), cfg.SMTP.To...)
	cfg.Redact = append([]string(nil), cfg.Redact...)
	return cfg
}

// UpdateConfig replaces delivery settings without resetting throttle or in-flight state.
func (es *EmailService) UpdateConfig(cfg types.VLoggoConfig) error {
	es.mu.Lock()
	defer es.mu.Unlock()
	if es.closed {
		return errors.New("notification service is closed")
	}
	es.cfg = cloneEmailConfig(cfg)
	return nil
}

func (es *EmailService) ConfigurationIssue() error {
	es.mu.Lock()
	cfg := cloneEmailConfig(es.cfg)
	es.mu.Unlock()
	if !cfg.Notify {
		return nil
	}
	return vloggoconfig.ValidateSMTP(cfg.SMTP)
}

func (es *EmailService) NotifyFatal(entry types.LogEntry) error {
	return es.NotifyFatalContext(context.Background(), entry)
}

func (es *EmailService) NotifyFatalContext(ctx context.Context, entry types.LogEntry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	es.mu.Lock()
	if es.closed {
		es.mu.Unlock()
		return errors.New("notification service is closed")
	}
	cfg := cloneEmailConfig(es.cfg)
	if !cfg.Notify || vloggoconfig.ValidateSMTP(cfg.SMTP) != nil {
		es.mu.Unlock()
		return nil
	}
	if es.inFlight || now.Before(es.retryAfter) || (!es.lastSent.IsZero() && now.Sub(es.lastSent) < time.Duration(cfg.Throttle)*time.Millisecond) {
		es.mu.Unlock()
		return nil
	}
	es.inFlight = true
	es.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.NotificationTimeout)*time.Millisecond)
	defer cancel()
	err := es.send(ctx, cfg, entry)
	es.mu.Lock()
	es.inFlight = false
	if err == nil {
		es.lastSent = time.Now()
		es.retryAfter = time.Time{}
	} else if cfg.RetryDelay > 0 {
		es.retryAfter = time.Now().Add(time.Duration(cfg.RetryDelay) * time.Millisecond)
	}
	es.mu.Unlock()
	return err
}

func (es *EmailService) Flush(context.Context) error { return nil }

func (es *EmailService) Close() error {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.closed = true
	return nil
}

func (es *EmailService) send(ctx context.Context, cfg types.VLoggoConfig, entry types.LogEntry) error {
	smtpCfg := cfg.SMTP
	from, err := mail.ParseAddress(smtpCfg.From)
	if err != nil {
		return fmt.Errorf("invalid SMTP sender: %w", err)
	}
	recipients := make([]*mail.Address, 0, len(smtpCfg.To))
	for _, value := range smtpCfg.To {
		address, parseErr := mail.ParseAddress(value)
		if parseErr != nil {
			return fmt.Errorf("invalid SMTP recipient: %w", parseErr)
		}
		recipients = append(recipients, address)
	}
	conn, client, err := es.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer client.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()

	if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
		return err
	}
	if smtpCfg.Username != "" || smtpCfg.Password != "" {
		auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
		return err
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP sender command failed: %w", err)
	}
	for _, recipient := range recipients {
		if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
			return err
		}
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("SMTP recipient command failed: %w", err)
		}
	}
	if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data command failed: %w", err)
	}
	if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
		_ = writer.Close()
		return err
	}
	format := NewFormatService(cfg.Client, cfg.Format.Timezone)
	if _, err := writer.Write([]byte(es.message(format, from, recipients, entry))); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP message write failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP data close failed: %w", err)
	}
	if err := es.setSocketDeadline(conn, ctx, smtpCfg.SocketTimeout); err != nil {
		return err
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP quit failed: %w", err)
	}
	return nil
}

func (es *EmailService) connect(ctx context.Context, cfg types.VLoggoConfig) (net.Conn, *smtp.Client, error) {
	smtpCfg := cfg.SMTP
	address := net.JoinHostPort(smtpCfg.Host, strconv.Itoa(smtpCfg.Port))
	dialer := net.Dialer{Timeout: time.Duration(smtpCfg.ConnectionTimeout) * time.Millisecond}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("SMTP connection failed: %w", err)
	}
	if smtpCfg.Secure {
		secured := tls.Client(connection, &tls.Config{ServerName: smtpCfg.Host, MinVersion: tls.VersionTLS12})
		if deadlineErr := es.setSocketDeadline(connection, ctx, smtpCfg.GreetingTimeout); deadlineErr != nil {
			_ = connection.Close()
			return nil, nil, deadlineErr
		}
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = connection.Close()
			return nil, nil, fmt.Errorf("SMTP TLS handshake failed: %w", err)
		}
		connection = secured
	}
	if err := es.setSocketDeadline(connection, ctx, smtpCfg.GreetingTimeout); err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	client, err := smtp.NewClient(connection, smtpCfg.Host)
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("SMTP greeting failed: %w", err)
	}
	if !smtpCfg.Secure {
		if err := es.setSocketDeadline(connection, ctx, smtpCfg.GreetingTimeout); err != nil {
			_ = client.Close()
			_ = connection.Close()
			return nil, nil, err
		}
		ok, _ := client.Extension("STARTTLS")
		if !ok && smtpCfg.RequireTLS {
			_ = client.Close()
			_ = connection.Close()
			return nil, nil, errors.New("SMTP server does not support required STARTTLS")
		}
		if ok {
			if err := es.setSocketDeadline(connection, ctx, smtpCfg.GreetingTimeout); err != nil {
				_ = client.Close()
				_ = connection.Close()
				return nil, nil, err
			}
			if err := client.StartTLS(&tls.Config{ServerName: smtpCfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				_ = connection.Close()
				return nil, nil, fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		}
	}
	if smtpCfg.Debug && cfg.Outputs.Console {
		fmt.Printf("[VLoggo] [%s] [SMTP] connected to %s\n", cfg.Client, address)
	}
	return connection, client, nil
}

func (es *EmailService) setSocketDeadline(connection net.Conn, ctx context.Context, timeoutMS int) error {
	deadline := time.Time{}
	if timeoutMS > 0 {
		deadline = time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	}
	if contextDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || contextDeadline.Before(deadline)) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (es *EmailService) message(format *FormatService, from *mail.Address, recipients []*mail.Address, entry types.LogEntry) string {
	boundary := "vloggo-" + strings.ReplaceAll(entry.EventID, "-", "")
	if len(boundary) < 12 {
		boundary = "vloggo-message"
	}
	to := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, recipient.String())
	}
	subject := mime.QEncoding.Encode("utf-8", fmt.Sprintf("[%s] %s - %s", entry.Level, entry.Client, entry.Code))
	textBody, htmlBody := bodies(format, entry)
	headers := []string{
		"From: " + from.String(),
		"To: " + strings.Join(to, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + strconv.Quote(boundary),
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + textBody + "\r\n" +
		"--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + htmlBody + "\r\n" +
		"--" + boundary + "--\r\n"
}

func bodies(format *FormatService, entry types.LogEntry) (string, string) {
	timestamp := entry.Timestamp
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		timestamp = format.Date(parsed) + " " + format.Timezone
	}
	fields := [][2]string{
		{"Event ID", entry.EventID},
		{"Timestamp", timestamp},
		{"Application", entry.Client},
		{"Level", string(entry.Level)},
		{"Code", entry.Code},
		{"Caller", entry.Caller},
		{"Message", entry.Message},
	}
	var text strings.Builder
	var htmlBody strings.Builder
	fmt.Fprintf(&htmlBody, "<h2>%s: %s</h2><p>%s</p><dl>", htmlEscape(string(entry.Level)), htmlEscape(entry.Client), htmlEscape(entry.Message))
	for _, field := range fields {
		fmt.Fprintf(&text, "%s: %s\n", field[0], field[1])
		if field[0] == "Message" {
			continue
		}
		fmt.Fprintf(&htmlBody, "<dt><strong>%s</strong></dt><dd>%s</dd>", htmlEscape(field[0]), htmlEscape(field[1]))
	}
	htmlBody.WriteString("</dl>")
	if entry.Context != nil || entry.Data != nil || entry.Error != nil {
		details := map[string]any{}
		if len(entry.Context) > 0 {
			details["context"] = entry.Context
		}
		if len(entry.Data) > 0 {
			details["data"] = entry.Data
		}
		if len(entry.Error) > 0 {
			details["error"] = entry.Error
		}
		encoded, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			encoded = []byte("[details unavailable]")
		}
		text.WriteString("\nDetails\n")
		text.Write(encoded)
		fmt.Fprintf(&htmlBody, "<h3>Details</h3><pre>%s</pre>", htmlEscape(string(encoded)))
	}
	return text.String(), "<!doctype html><html><body>" + htmlBody.String() + "</body></html>"
}

func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;").Replace(value)
}
