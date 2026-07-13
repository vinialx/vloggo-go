package services

import (
	"crypto/tls"
	"fmt"
	"html"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	config "github.com/vinialx/vloggo-go/config"
	types "github.com/vinialx/vloggo-go/types"
)

type EmailService struct {
	cfg      types.VLoggoConfig
	format   *FormatService
	lastSent time.Time
	mu       sync.Mutex
}

func NewEmailService(cfg types.VLoggoConfig) *EmailService {
	return &EmailService{
		cfg:    cfg,
		format: NewFormatService(cfg.Client),
	}
}

func (es *EmailService) NotifyFatal(entry types.LogEntry) error {
	if !es.cfg.Notify {
		return nil
	}
	if err := config.ValidateSMTP(es.cfg.SMTP); err != nil {
		return err
	}
	if !es.canSend() {
		return nil
	}

	subject := fmt.Sprintf("[%s] Fatal error: %s", es.cfg.Client, entry.Code)
	body := es.fatalHTML(entry)
	if err := es.send(subject, body); err != nil {
		return err
	}

	es.markSent()
	return nil
}

func (es *EmailService) canSend() bool {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.lastSent.IsZero() {
		return true
	}

	throttle := time.Duration(es.cfg.Throttle) * time.Millisecond
	if throttle <= 0 {
		return true
	}

	return time.Since(es.lastSent) >= throttle
}

func (es *EmailService) markSent() {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.lastSent = time.Now()
}

func (es *EmailService) fatalHTML(entry types.LogEntry) string {
	return fmt.Sprintf(`<!doctype html>
<html>
<body>
  <h2>VLoggo fatal notification</h2>
  <p><strong>Client:</strong> %s</p>
  <p><strong>Timestamp:</strong> %s</p>
  <p><strong>Level:</strong> %s</p>
  <p><strong>Code:</strong> %s</p>
  <p><strong>Caller:</strong> %s</p>
  <p><strong>Message:</strong></p>
  <pre>%s</pre>
</body>
</html>`,
		html.EscapeString(es.cfg.Client),
		html.EscapeString(es.format.IsoDate()),
		html.EscapeString(string(entry.Level)),
		html.EscapeString(entry.Code),
		html.EscapeString(entry.Caller),
		html.EscapeString(entry.Message),
	)
}

func (es *EmailService) send(subject, htmlBody string) error {
	smtpCfg := es.cfg.SMTP
	addr := net.JoinHostPort(smtpCfg.Host, strconv.Itoa(smtpCfg.Port))

	client, err := es.smtpClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if smtpCfg.Username != "" || smtpCfg.Password != "" {
		auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed > %w", err)
		}
	}

	if err := client.Mail(smtpCfg.From); err != nil {
		return fmt.Errorf("smtp sender failed > %w", err)
	}
	for _, recipient := range smtpCfg.To {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp recipient failed > %w", err)
		}
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed > %w", err)
	}

	message := es.message(subject, htmlBody)
	if _, err := wc.Write([]byte(message)); err != nil {
		wc.Close()
		return fmt.Errorf("smtp write failed > %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close failed > %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit failed > %w", err)
	}

	return nil
}

func (es *EmailService) smtpClient(addr string) (*smtp.Client, error) {
	smtpCfg := es.cfg.SMTP
	if smtpCfg.Secure {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: smtpCfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial failed > %w", err)
		}

		client, err := smtp.NewClient(conn, smtpCfg.Host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp client failed > %w", err)
		}

		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial failed > %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: smtpCfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			client.Close()
			return nil, fmt.Errorf("smtp starttls failed > %w", err)
		}
	}

	return client, nil
}

func (es *EmailService) message(subject, htmlBody string) string {
	smtpCfg := es.cfg.SMTP
	headers := []string{
		"From: " + smtpCfg.From,
		"To: " + strings.Join(smtpCfg.To, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	return strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody
}
