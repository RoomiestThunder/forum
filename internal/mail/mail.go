// Package mail sends transactional email (password reset, verification)
// over SMTP. When no host is configured, messages are logged instead of
// sent - the same graceful-degradation pattern the Redis and MinIO clients
// use, so local development and CI don't need a real mail server.
package mail

import (
	"fmt"
	"log/slog"
	"net/smtp"

	"forum/internal/config"
)

type Mailer struct {
	cfg    config.SMTPConfig
	logger *slog.Logger
}

func New(cfg config.SMTPConfig, logger *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, logger: logger}
}

// Send delivers a plain-text email, or logs it if SMTP isn't configured.
func (m *Mailer) Send(to, subject, body string) error {
	if m.cfg.Host == "" {
		m.logger.Info("email not sent (SMTP not configured) - logging instead",
			slog.String("to", to), slog.String("subject", subject), slog.String("body", body))
		return nil
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.cfg.From, to, subject, body)

	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(msg)); err != nil {
		m.logger.Error("failed to send email", slog.String("to", to), slog.Any("err", err))
		return err
	}
	return nil
}
