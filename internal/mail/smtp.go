package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// SMTPConfig is how the mailer reaches the provider (Scaleway TEM in
// production, Mailpit in development).
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// Security: "starttls" (port 587), "tls" (implicit TLS, port 465) or
	// "none" (only for a local catcher such as Mailpit: no credentials).
	Security string
	From     string // "QuantumReady <no-reply@quantumready.fizsar.com>"
	ReplyTo  string // "soporte@fizsar.com"
	Timeout  time.Duration
}

// Validate checks the configuration before the first send.
func (c SMTPConfig) Validate() error {
	if c.Host == "" || c.Port <= 0 || c.Port > 65535 {
		return errors.New("mail: SMTP host and port are required")
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return fmt.Errorf("mail: From: %w", err)
	}
	if c.ReplyTo != "" {
		if _, err := mail.ParseAddress(c.ReplyTo); err != nil {
			return fmt.Errorf("mail: Reply-To: %w", err)
		}
	}
	switch c.Security {
	case "starttls", "tls":
	case "none":
		if c.Username != "" || c.Password != "" {
			return errors.New("mail: refusing to send credentials without TLS")
		}
	default:
		return fmt.Errorf("mail: SMTP security %q: want starttls, tls or none", c.Security)
	}
	return nil
}

// ErrBadRecipient is a permanent failure: the address cannot be used.
var ErrBadRecipient = errors.New("mail: invalid recipient")

// Message builds the RFC 5322 message: plain text, UTF-8, quoted-printable,
// RFC 2047 subject. CR and LF in the subject or the address are rejected, so
// nothing from a user can add headers.
func Message(cfg SMTPConfig, to, subject, body string, now time.Time) ([]byte, error) {
	addr, err := mail.ParseAddress(to)
	if err != nil || strings.ContainsAny(to, "\r\n") {
		return nil, ErrBadRecipient
	}
	if strings.ContainsAny(subject, "\r\n") {
		return nil, errors.New("mail: line break in subject")
	}
	from, _ := mail.ParseAddress(cfg.From)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]

	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("From", from.String())
	header("To", addr.String())
	if cfg.ReplyTo != "" {
		rt, _ := mail.ParseAddress(cfg.ReplyTo)
		header("Reply-To", rt.String())
	}
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Message-ID", fmt.Sprintf("<%s@%s>", hex.EncodeToString(id), domain))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated") // RFC 3834: no auto-replies
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Send delivers one message. Errors are returned as is; the mailer decides
// whether to retry (temporary 4xx, network) or give up.
func Send(ctx context.Context, cfg SMTPConfig, to string, msg []byte) error {
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return ErrBadRecipient
	}
	from, _ := mail.ParseAddress(cfg.From)
	hostPort := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	if cfg.Security == "tls" {
		conn, err = (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", hostPort)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", hostPort)
	}
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	// Cancelling ctx (shutdown) also interrupts a session in progress.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()
	if cfg.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not offer STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(addr.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Permanent reports an SMTP error that retrying will not fix (5xx) or a bad
// recipient.
func Permanent(err error) bool {
	if errors.Is(err, ErrBadRecipient) {
		return true
	}
	var tp *textproto.Error
	return errors.As(err, &tp) && tp.Code >= 500 && tp.Code < 600
}
