package mail

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"forum/internal/config"
)

// fakeSMTP is a minimal SMTP server that records what a client sends, so the
// tests exercise the real net/smtp client and the bytes it puts on the wire.
type fakeSMTP struct {
	host string
	port int

	mu         sync.Mutex
	from       []string
	rcpt       []string
	messages   [][]byte
	authLines  []string
	offerAuth  bool
	rejectRCPT bool
}

func startFakeSMTP(t *testing.T, configure func(*fakeSMTP)) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	s := &fakeSMTP{host: "127.0.0.1", port: ln.Addr().(*net.TCPAddr).Port}
	if configure != nil {
		configure(s)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)
	reply := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }

	reply("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		s.mu.Lock()
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			if s.offerAuth {
				reply("250-fake\r\n250-AUTH PLAIN\r\n250 OK")
			} else {
				reply("250 fake")
			}
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			s.authLines = append(s.authLines, line)
			reply("235 authenticated")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.from = append(s.from, line)
			reply("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.rcpt = append(s.rcpt, line)
			if s.rejectRCPT {
				reply("550 no such user")
			} else {
				reply("250 OK")
			}
		case upper == "DATA":
			reply("354 go ahead")
			var data bytes.Buffer
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					s.mu.Unlock()
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(strings.TrimPrefix(l, ".")) // undo dot-stuffing
			}
			s.messages = append(s.messages, data.Bytes())
			reply("250 queued")
		case upper == "QUIT":
			reply("221 bye")
			s.mu.Unlock()
			return
		default:
			reply("250 OK")
		}
		s.mu.Unlock()
	}
}

func (s *fakeSMTP) sent() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.messages...)
}

func newMailer(s *fakeSMTP, logs io.Writer) *Mailer {
	if logs == nil {
		logs = io.Discard
	}
	return New(config.SMTPConfig{Host: s.host, Port: s.port, From: "Forum <noreply@forum.example>"},
		slog.New(slog.NewTextHandler(logs, nil)))
}

func parse(t *testing.T, raw []byte) (*stdmail.Message, string) {
	t.Helper()
	msg, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the message isn't valid RFC 5322: %v\n%s", err, raw)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	return msg, string(body)
}

func TestSendDeliversAWellFormedMessage(t *testing.T) {
	server := startFakeSMTP(t, nil)
	body := "Click to verify:\nhttps://forum.example/verify?token=abc\n"

	if err := newMailer(server, nil).Send("alice@example.com", "Verify your email", body); err != nil {
		t.Fatal(err)
	}

	sent := server.sent()
	if len(sent) != 1 {
		t.Fatalf("want 1 message, got %d", len(sent))
	}
	msg, gotBody := parse(t, sent[0])

	if got := msg.Header.Get("Subject"); got != "Verify your email" {
		t.Errorf("Subject = %q", got)
	}
	if got := msg.Header.Get("To"); !strings.Contains(got, "alice@example.com") {
		t.Errorf("To = %q", got)
	}
	if got := msg.Header.Get("From"); !strings.Contains(got, "noreply@forum.example") {
		t.Errorf("From = %q", got)
	}
	if got := msg.Header.Get("MIME-Version"); got != "1.0" {
		t.Errorf("MIME-Version = %q", got)
	}
	if got := msg.Header.Get("Content-Type"); got != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if _, err := msg.Header.Date(); err != nil {
		t.Errorf("a valid Date header is required: %v", err)
	}
	if id := msg.Header.Get("Message-ID"); !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@forum.example>") {
		t.Errorf("Message-ID = %q, want one in the sender's domain", id)
	}
	if strings.ReplaceAll(gotBody, "\r\n", "\n") != body {
		t.Errorf("body = %q, want %q", gotBody, body)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.from) != 1 || !strings.Contains(server.from[0], "<noreply@forum.example>") {
		t.Errorf("envelope sender = %v", server.from)
	}
	if len(server.rcpt) != 1 || !strings.Contains(server.rcpt[0], "<alice@example.com>") {
		t.Errorf("envelope recipient = %v", server.rcpt)
	}
}

func TestNonASCIISubjectAndBodySurvive(t *testing.T) {
	server := startFakeSMTP(t, nil)
	subject, body := "Сброс пароля", "Здравствуйте! Перейдите по ссылке.\n"

	if err := newMailer(server, nil).Send("alice@example.com", subject, body); err != nil {
		t.Fatal(err)
	}

	raw := server.sent()[0]
	msg, gotBody := parse(t, raw)
	decoded, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || decoded != subject {
		t.Errorf("subject decodes to %q (%v), want %q", decoded, err, subject)
	}
	if strings.ReplaceAll(gotBody, "\r\n", "\n") != body {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
	for i, b := range raw {
		if b > 127 {
			t.Fatalf("byte %d of the message is non-ASCII (%#x); a server without 8BITMIME would mangle it", i, b)
		}
	}
}

func TestLongBodyLinesAreWrapped(t *testing.T) {
	server := startFakeSMTP(t, nil)
	if err := newMailer(server, nil).Send("alice@example.com", "Long", strings.Repeat("word ", 200)); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(server.sent()[0]), "\r\n") {
		if len(line) > 998 { // RFC 5322 hard limit; quoted-printable keeps well under it
			t.Fatalf("line of %d characters exceeds the RFC limit", len(line))
		}
	}
}

// Header values include a user-supplied email address and must not be able to
// add headers (a Bcc to an attacker) or end the header block early.
func TestLineBreaksInHeadersAreRefused(t *testing.T) {
	server := startFakeSMTP(t, nil)
	mailer := newMailer(server, nil)

	cases := map[string][2]string{
		"subject":   {"alice@example.com", "Hello\r\nBcc: attacker@example.com"},
		"recipient": {"alice@example.com\r\nBcc: attacker@example.com", "Hello"},
		"bare LF":   {"alice@example.com", "Hello\nBcc: attacker@example.com"},
	}
	for name, c := range cases {
		if err := mailer.Send(c[0], c[1], "body"); err == nil {
			t.Errorf("%s: a line break in a header must be refused", name)
		}
	}
	if n := len(server.sent()); n != 0 {
		t.Fatalf("nothing may reach the server for a refused message, but %d did", n)
	}
}

func TestInvalidRecipientsAreRefused(t *testing.T) {
	server := startFakeSMTP(t, nil)
	for _, to := range []string{"", "not-an-address", "@example.com", "alice@"} {
		if err := newMailer(server, nil).Send(to, "Hello", "body"); err == nil {
			t.Errorf("recipient %q should be refused", to)
		}
	}
	if n := len(server.sent()); n != 0 {
		t.Fatalf("%d messages reached the server", n)
	}
}

func TestEachMessageGetsAUniqueID(t *testing.T) {
	a, err := buildMessage("noreply@forum.example", "alice@example.com", "s", "b", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := buildMessage("noreply@forum.example", "alice@example.com", "s", "b", time.Now())

	idOf := func(m *message) string {
		parsed, _ := stdmail.ReadMessage(bytes.NewReader(m.data))
		return parsed.Header.Get("Message-ID")
	}
	if idOf(a) == idOf(b) {
		t.Fatal("Message-IDs must be unique")
	}
}

func TestSendLogsInsteadOfSendingWhenSMTPIsNotConfigured(t *testing.T) {
	var logs bytes.Buffer
	mailer := New(config.SMTPConfig{From: "noreply@forum.example"}, slog.New(slog.NewTextHandler(&logs, nil)))

	if err := mailer.Send("alice@example.com", "Verify", "the link"); err != nil {
		t.Fatalf("with no SMTP host, sending is a no-op that must not fail: %v", err)
	}
	for _, want := range []string{"alice@example.com", "Verify", "the link"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("the log should contain %q, got:\n%s", want, logs.String())
		}
	}
}

// Even when mail is only logged, bad input is refused, so a header-injection
// attempt can't pass in development and surface only in production.
func TestUnconfiguredMailerStillValidates(t *testing.T) {
	mailer := New(config.SMTPConfig{From: "noreply@forum.example"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := mailer.Send("alice@example.com", "Hi\r\nBcc: x@y.z", "body"); err == nil {
		t.Fatal("a header injection attempt must be refused even when mail is only logged")
	}
}

func TestCredentialsAreUsedWhenConfigured(t *testing.T) {
	server := startFakeSMTP(t, func(s *fakeSMTP) { s.offerAuth = true })
	mailer := New(config.SMTPConfig{
		Host: server.host, Port: server.port, From: "noreply@forum.example", User: "mailer", Pass: "s3cret",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := mailer.Send("alice@example.com", "Hello", "body"); err != nil {
		t.Fatal(err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.authLines) != 1 {
		t.Fatalf("expected one AUTH attempt, got %v", server.authLines)
	}
	creds, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(server.authLines[0], "AUTH PLAIN "))
	if err != nil || !strings.Contains(string(creds), "mailer") || !strings.Contains(string(creds), "s3cret") {
		t.Errorf("AUTH PLAIN credentials = %q (%v)", creds, err)
	}
}

func TestServerRefusalIsReturnedToTheCaller(t *testing.T) {
	server := startFakeSMTP(t, func(s *fakeSMTP) { s.rejectRCPT = true })
	var logs bytes.Buffer

	err := newMailer(server, &logs).Send("nobody@example.com", "Hello", "body")

	if err == nil {
		t.Fatal("a rejected recipient must surface as an error")
	}
	if !strings.Contains(logs.String(), "failed to send email") {
		t.Errorf("the failure should be logged: %s", logs.String())
	}
	if n := len(server.sent()); n != 0 {
		t.Errorf("no message should have been accepted, got %d", n)
	}
}

func TestUnreachableServerIsAnError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // nothing listens on it now

	mailer := New(config.SMTPConfig{Host: "127.0.0.1", Port: port, From: "noreply@forum.example"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := mailer.Send("alice@example.com", "Hello", "body"); err == nil {
		t.Fatal("an unreachable SMTP server must be an error, not silently dropped mail (port " + strconv.Itoa(port) + ")")
	}
}
