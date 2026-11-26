package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"os"
	"runtime"
	"strings"
	"time"
)

// Mailer sends one plain-text email. Copied from tools/custodian/mailer.go,
// which sends sign-in codes; here it tells somebody that money was sent to
// their email address (transfers.go).
type Mailer interface {
	Send(to, subject, body string) error
}

// smtpMailer submits mail over SMTP with STARTTLS and a login, the way any
// mail client does. Configured by a file, never by flags, because it holds a
// password:
//
//	host=mail.yamaletech.com
//	port=587
//	username=no-reply@yamaletech.com
//	password=...
//	from=Yamale <no-reply@yamaletech.com>
//	# optional: connect here instead of host, still verifying host's certificate
//	dial=127.0.0.1:587
type smtpMailer struct {
	host, port, username, password, from, dial string
}

func loadSMTPConfig(path string) (*smtpMailer, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by others (%v); it holds a password, so chmod 600 it", path, info.Mode().Perm())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := &smtpMailer{port: "587"}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s: %q is not key=value", path, line)
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "host":
			m.host = v
		case "port":
			m.port = v
		case "username":
			m.username = v
		case "password":
			m.password = v
		case "from":
			m.from = v
		case "dial":
			m.dial = v
		default:
			return nil, fmt.Errorf("%s: unknown key %q", path, k)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if m.host == "" || m.username == "" || m.password == "" || m.from == "" {
		return nil, fmt.Errorf("%s needs host, username, password and from", path)
	}
	return m, nil
}

func (m *smtpMailer) Send(to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") {
		return fmt.Errorf("refusing an address with a line break")
	}
	addr := m.dial
	if addr == "" {
		addr = net.JoinHostPort(m.host, m.port)
	}
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	// Never in the clear: the login is a password.
	if err := c.StartTLS(&tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	if err := c.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	envelopeFrom := m.username
	if err := c.Mail(envelopeFrom); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(message(m.from, to, subject, body, m.host, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// message is an RFC 5322 plain-text message in UTF-8.
func message(from, to, subject, body, host string, at time.Time) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + at.UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + hex.EncodeToString(id) + "@" + host + ">\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}
