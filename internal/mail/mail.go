// Package mail sends transactional email (password reset, verification)
// over SMTP. When no host is configured, messages are logged instead of
// sent - the same graceful-degradation pattern the Redis and MinIO clients
// use, so local development and CI don't need a real mail server.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	stdmail "net/mail"
	"net/smtp"
	"strings"
	"time"

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
//
// The message is built, and so validated, before anything else happens, which
// means a malformed recipient or a header injection attempt is rejected the
// same way in development (where mail is only logged) as in production.
func (m *Mailer) Send(to, subject, body string) error {
	msg, err := buildMessage(m.cfg.From, to, subject, body, time.Now())
	if err != nil {
		m.logger.Error("refusing to send email", slog.String("to", to), slog.Any("err", err))
		return err
	}

	if m.cfg.Host == "" {
		m.logger.Info("email not sent (SMTP not configured) - logging instead",
			slog.String("to", to), slog.String("subject", subject), slog.String("body", body))
		return nil
	}

	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	if err := smtp.SendMail(addr, auth, msg.from, []string{msg.to}, msg.data); err != nil {
		m.logger.Error("failed to send email", slog.String("to", to), slog.Any("err", err))
		return err
	}
	return nil
}

var errLineBreak = errors.New("mail: header value contains a line break")

type message struct {
	from, to string // bare envelope addresses
	data     []byte // headers + body, ready for the DATA command
}

// buildMessage assembles an RFC 5322 message. Header values come from callers
// (the recipient is a user-supplied email address), so a line break in any of
// them is refused outright: it would let the value end its header and start
// another - adding a Bcc, say - or end the headers and write its own body.
func buildMessage(from, to, subject, body string, now time.Time) (*message, error) {
	for _, value := range []string{from, to, subject} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, errLineBreak
		}
	}
	fromAddr, err := stdmail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid sender %q: %w", from, err)
	}
	toAddr, err := stdmail.ParseAddress(to)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid recipient %q: %w", to, err)
	}
	id, err := messageID(fromAddr.Address)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	header := func(name, value string) { buf.WriteString(name + ": " + value + "\r\n") }
	header("From", fromAddr.String())
	header("To", toAddr.String())
	// A non-ASCII subject (a localised email, a username in it) is not legal
	// in a header as is; RFC 2047 encoding leaves plain ASCII untouched.
	header("Subject", mime.QEncoding.Encode("UTF-8", subject))
	header("Date", now.Format(time.RFC1123Z))
	// Mail without Date or Message-ID looks machine-made to spam filters.
	header("Message-ID", id)
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	// Quoted-printable keeps the body 7-bit safe, so UTF-8 text survives a hop
	// through a server that doesn't advertise 8BITMIME, and wraps long lines.
	header("Content-Transfer-Encoding", "quoted-printable")
	buf.WriteString("\r\n")

	w := quotedprintable.NewWriter(&buf)
	if _, err := w.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return &message{from: fromAddr.Address, to: toAddr.Address, data: buf.Bytes()}, nil
}

// messageID returns a unique Message-ID in the sender's domain.
func messageID(sender string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("mail: cannot generate a message id: %w", err)
	}
	domain := "localhost"
	if at := strings.LastIndex(sender, "@"); at >= 0 && at < len(sender)-1 {
		domain = sender[at+1:]
	}
	return "<" + hex.EncodeToString(random) + "@" + domain + ">", nil
}
