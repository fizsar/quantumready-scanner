package mail

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

var testCfg = SMTPConfig{Host: "127.0.0.1", Port: 25, Security: "none",
	From: "QuantumReady <no-reply@quantumready.fizsar.com>", ReplyTo: "soporte@fizsar.com"}

func TestRenderEveryTemplate(t *testing.T) {
	d := Data{Link: "https://q.example/verify?token=abc", ValidFor: "24 horas", SiteURL: "https://q.example", Support: "soporte@fizsar.com"}
	for name := range catalog {
		for _, lang := range []string{"es", "en"} {
			e, err := Render(name, lang, "ana@example.com", d)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, lang, err)
			}
			if e.To != "ana@example.com" || e.Template != name || e.Lang != lang || e.Subject == "" || strings.Contains(e.Body, "{{") || strings.Contains(e.Body, "<no value>") {
				t.Errorf("%s/%s: %+v", name, lang, e)
			}
			if (name == VerifyEmail || name == ResetPassword) && !strings.Contains(e.Body, d.Link) {
				t.Errorf("%s/%s: link missing", name, lang)
			}
		}
	}
	if e, _ := Render(VerifyEmail, "fr", "a@example.com", d); e.Lang != "es" {
		t.Errorf("unknown language → %s, want es", e.Lang)
	}
	if _, err := Render("nope", "es", "a@example.com", d); err == nil {
		t.Error("unknown template rendered")
	}
}

func TestMessage(t *testing.T) {
	msg, err := Message(testCfg, "ana@example.com", "Confirma tu email — ñ", "Hola:\nenlace: https://q.example/x?t=abc=def\n", time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(msg)))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	body, _ := io.ReadAll(quotedprintable.NewReader(m.Body))
	if subject != "Confirma tu email — ñ" || m.Header.Get("To") != "<ana@example.com>" ||
		m.Header.Get("Reply-To") != "<soporte@fizsar.com>" || !strings.HasSuffix(m.Header.Get("Message-ID"), "@quantumready.fizsar.com>") ||
		m.Header.Get("Content-Type") != "text/plain; charset=utf-8" || m.Header.Get("Auto-Submitted") != "auto-generated" {
		t.Errorf("headers: %v (subject %q)", m.Header, subject)
	}
	if string(body) != "Hola:\r\nenlace: https://q.example/x?t=abc=def\r\n" {
		t.Errorf("body = %q", body)
	}
	// Nothing from outside can add headers.
	for _, to := range []string{"ana@example.com\r\nBcc: x@evil.example", "not an address", ""} {
		if _, err := Message(testCfg, to, "s", "b", time.Now()); !errors.Is(err, ErrBadRecipient) {
			t.Errorf("Message(to=%q) = %v", to, err)
		}
	}
	if _, err := Message(testCfg, "ana@example.com", "s\r\nBcc: x@evil.example", "b", time.Now()); err == nil {
		t.Error("line break in the subject accepted")
	}
}

func TestSMTPConfigValidate(t *testing.T) {
	ok := []SMTPConfig{testCfg, {Host: "smtp.tem.scaleway.com", Port: 587, Security: "starttls", Username: "u", Password: "p", From: "a@b.example"}}
	for _, c := range ok {
		if err := c.Validate(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
	bad := []SMTPConfig{
		{Host: "x", Port: 25, Security: "none", Username: "u", Password: "p", From: "a@b.example"}, // credentials in clear
		{Host: "x", Port: 25, Security: "ssl", From: "a@b.example"},
		{Host: "", Port: 25, Security: "tls", From: "a@b.example"},
		{Host: "x", Port: 25, Security: "tls", From: "nope"},
	}
	for _, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
}

// fakeSMTP is a minimal SMTP server that records what it receives. reject,
// if set, is the reply to RCPT TO.
type fakeSMTP struct {
	addr   string
	mu     sync.Mutex
	got    []string // DATA of each message
	rcpt   []string
	reject string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f := &fakeSMTP{addr: l.Addr().String()}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	tp := textproto.NewConn(c)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch cmd {
		case "EHLO", "HELO":
			_ = tp.PrintfLine("250 fake")
		case "MAIL":
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			f.mu.Lock()
			f.rcpt = append(f.rcpt, line)
			reject := f.reject
			f.mu.Unlock()
			if reject != "" {
				_ = tp.PrintfLine("%s", reject)
				continue
			}
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go")
			data, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.got = append(f.got, string(data))
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 no")
		}
	}
}

func (f *fakeSMTP) cfg() SMTPConfig {
	c := testCfg
	host, port, _ := net.SplitHostPort(f.addr)
	c.Host = host
	c.Port, _ = strconv.Atoi(port)
	return c
}

func TestSendToFakeServer(t *testing.T) {
	f := startFakeSMTP(t)
	cfg := f.cfg()
	msg, _ := Message(cfg, "ana@example.com", "Hola", "cuerpo", time.Now())
	if err := Send(context.Background(), cfg, "ana@example.com", msg); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 || !strings.Contains(f.got[0], "Subject: Hola") || !strings.Contains(f.rcpt[0], "<ana@example.com>") {
		t.Errorf("server got %q, rcpt %v", f.got, f.rcpt)
	}
	f.reject = "550 no such user"
	err := Send(context.Background(), cfg, "nobody@example.com", msg)
	if err == nil || !Permanent(err) {
		t.Errorf("550: %v, permanent=%v", err, Permanent(err))
	}
	f.reject = "451 try later"
	if err := Send(context.Background(), cfg, "later@example.com", msg); err == nil || Permanent(err) {
		t.Errorf("451: %v, permanent=%v", err, Permanent(err))
	}
	// STARTTLS required but not offered: refuse rather than send in clear.
	cfg.Security = "starttls"
	if err := Send(context.Background(), cfg, "ana@example.com", msg); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("starttls against a plain server: %v", err)
	}
}

// The mailer against the real outbox, with its production role: it reads
// and updates the outbox only.
func TestMailerWithOutbox(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	owner := store.NewStore(pool)
	for _, to := range []string{"ok@example.com", "bad@example.com", "later@example.com"} {
		e, _ := Render(VerifyEmail, "es", to, Data{Link: "https://q.example/v?t=1", ValidFor: "24 horas", SiteURL: "https://q.example", Support: "s@example.com"})
		if err := owner.EnqueueEmail(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	ms := store.NewStore(testdb.AsRole(t, pool, store.RoleMailer))
	var sent []string
	m := &Mailer{Outbox: ms.Queries, SMTP: testCfg, Logger: quiet, Send: func(_ context.Context, _ SMTPConfig, to string, _ []byte) error {
		switch to {
		case "bad@example.com":
			return &textproto.Error{Code: 550, Msg: "no such user"}
		case "later@example.com":
			return &textproto.Error{Code: 451, Msg: "try later"}
		}
		sent = append(sent, to)
		return nil
	}}
	if !m.Once(ctx) {
		t.Fatal("first batch failed")
	}
	status := func() map[string]string {
		out := map[string]string{}
		rows, err := pool.Query(ctx, "SELECT to_email, status || '/' || attempts FROM email_outbox")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a, b string
			_ = rows.Scan(&a, &b)
			out[a] = b
		}
		return out
	}
	got := status()
	if got["ok@example.com"] != "sent/1" || got["bad@example.com"] != "failed/1" || got["later@example.com"] != "queued/1" || len(sent) != 1 {
		t.Errorf("after one batch: %v (sent %v)", got, sent)
	}
	// The retry is not due yet: a second batch does nothing.
	m.Once(ctx)
	if got := status(); got["later@example.com"] != "queued/1" {
		t.Errorf("retried before its delay: %v", got)
	}
	// Old sent and failed emails are purged (they carry tokens).
	if _, err := pool.Exec(ctx, "UPDATE email_outbox SET created_at = now() - interval '8 days'"); err != nil {
		t.Fatal(err)
	}
	if n, err := ms.PurgeEmails(ctx, (7 * 24 * time.Hour).Seconds()); err != nil || n != 2 {
		t.Errorf("purged %d (%v), want the sent and the failed one", n, err)
	}
}
