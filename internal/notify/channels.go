// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/mile-garage/mile/internal/deadlines"
	"github.com/mile-garage/mile/internal/store"
)

// ---- email ----

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "MILE <mile@example.com>" or just the address
	TLS      string // starttls (default), tls (implicit, usually port 465), none
}

func (c SMTPConfig) Configured() bool { return c.Host != "" && c.From != "" }

type Email struct {
	Config  SMTPConfig
	Timeout time.Duration
}

func (e *Email) Name() string { return "email" }

func (e *Email) Enabled(s store.NotificationSettings) bool {
	return e.Config.Configured() && s.EmailEnabled && s.Email != ""
}

func (e *Email) Send(ctx context.Context, s store.NotificationSettings, m Message) error {
	from, err := mail.ParseAddress(e.Config.From)
	if err != nil {
		return fmt.Errorf("MILE_SMTP_FROM: %w", err)
	}
	body, err := e.render(from, s.Email, m)
	if err != nil {
		return err
	}
	return e.deliver(ctx, from.Address, s.Email, body)
}

// render builds a plain-text UTF-8 message.
func (e *Email) render(from *mail.Address, to string, m Message) ([]byte, error) {
	var text strings.Builder
	for _, l := range m.Lines {
		text.WriteString("• " + l + "\r\n")
	}
	if m.URL != "" {
		text.WriteString("\r\n" + openText(m) + ": " + m.URL + "\r\n")
	}
	text.WriteString("\r\n-- \r\n" + footerText(m) + "\r\n")

	id := make([]byte, 12)
	rand.Read(id)
	domain := from.Address[strings.LastIndexByte(from.Address, '@')+1:]
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Title))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id), domain)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(text.String()))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes(), nil
}

func (e *Email) deliver(ctx context.Context, from, to string, msg []byte) error {
	c := e.Config
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	port := c.Port
	if port == 0 {
		port = 587
		if c.TLS == "tls" {
			port = 465
		}
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: timeout}
	tlsConf := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if c.TLS == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConf}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(timeout))
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()
	if c.TLS != "tls" && c.TLS != "none" {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("the SMTP server does not support STARTTLS (set MILE_SMTP_TLS=tls or none)")
		}
		if err := cl.StartTLS(tlsConf); err != nil {
			return err
		}
	}
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	}
	if err := cl.Mail(from); err != nil {
		return err
	}
	if err := cl.Rcpt(to); err != nil {
		return err
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

// ---- ntfy ----

type Ntfy struct {
	Client *http.Client
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Enabled(s store.NotificationSettings) bool {
	return s.NtfyEnabled && s.NtfyTopic != "" && s.NtfyURL != ""
}

// Send publishes with the JSON API, which handles UTF-8 titles safely.
func (n *Ntfy) Send(ctx context.Context, s store.NotificationSettings, m Message) error {
	base, ok := store.ValidNtfyURL(s.NtfyURL)
	if !ok {
		return errors.New("invalid ntfy server address")
	}
	payload := map[string]any{
		"topic":   s.NtfyTopic,
		"title":   m.Title,
		"message": "• " + strings.Join(m.Lines, "\n• "),
		"tags":    []string{"car"},
	}
	if m.Urgent {
		payload["priority"] = 4
		payload["tags"] = []string{"warning", "car"}
	}
	if m.URL != "" {
		payload["click"] = m.URL
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.NtfyToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.NtfyToken)
	}
	cl := n.Client
	if cl == nil {
		cl = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("ntfy: status %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// ---- shared texts ----

func openText(m Message) string   { return texts[deadlines.Locale(m.Locale)]["open"] }
func footerText(m Message) string { return texts[deadlines.Locale(m.Locale)]["footer"] }
