package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSMTPConfig(t *testing.T) {
	env := map[string]string{"SMTP_HOST": "smtp.tem.scaleway.com", "SMTP_PORT": "587", "SMTP_SECURITY": "starttls",
		"SMTP_USERNAME": "u", "SMTP_PASSWORD": "p", "MAIL_FROM": "QuantumReady <no-reply@quantumready.fizsar.com>", "MAIL_REPLY_TO": "soporte@fizsar.com"}
	get := func(k string) string { return env[k] }
	if c, err := smtpConfig(get); err != nil || c.Port != 587 {
		t.Fatalf("%+v %v", c, err)
	}
	env["SMTP_SECURITY"] = "none" // credentials in clear: refused
	if _, err := smtpConfig(get); err == nil {
		t.Error("SMTP_SECURITY=none with credentials accepted")
	}
	env["SMTP_PORT"] = "x"
	if _, err := smtpConfig(get); err == nil {
		t.Error("bad port accepted")
	}
}

func TestHeartbeat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hb")
	now := time.Now()
	if err := checkHeartbeat(p, now, time.Minute); err == nil {
		t.Error("missing heartbeat accepted")
	}
	if err := writeHeartbeat(p, now.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := checkHeartbeat(p, now, time.Minute); err != nil {
		t.Errorf("fresh heartbeat: %v", err)
	}
	if err := checkHeartbeat(p, now.Add(2*time.Minute), time.Minute); err == nil {
		t.Error("stale heartbeat accepted")
	}
}
