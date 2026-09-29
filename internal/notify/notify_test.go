// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mile-garage/mile/internal/db"
	"github.com/mile-garage/mile/internal/deadlines"
	"github.com/mile-garage/mile/internal/store"
)

func ip(n int) *int      { return &n }
func i64(n int64) *int64 { return &n }
func days() []int        { return []int{30, 7, 1} }

func TestStage(t *testing.T) {
	cases := []struct {
		d    deadlines.Deadline
		want string
	}{
		{deadlines.Deadline{Status: deadlines.OK, DaysLeft: ip(45)}, ""},
		{deadlines.Deadline{Status: deadlines.OK, DaysLeft: ip(30)}, "d30"},
		{deadlines.Deadline{Status: deadlines.DueSoon, DaysLeft: ip(12)}, "d30"},
		{deadlines.Deadline{Status: deadlines.DueSoon, DaysLeft: ip(5)}, "d7"}, // first run close to the date: only the nearest stage
		{deadlines.Deadline{Status: deadlines.DueSoon, DaysLeft: ip(0)}, "d1"},
		{deadlines.Deadline{Status: deadlines.Overdue, DaysLeft: ip(-3)}, "overdue"},
		{deadlines.Deadline{Status: deadlines.Grace, DaysLeft: ip(-3)}, "overdue"},
		{deadlines.Deadline{Status: deadlines.Suspended, DaysLeft: ip(-3)}, ""},
		{deadlines.Deadline{Status: deadlines.DueSoon, KmLeft: i64(800)}, "km"}, // km only
		{deadlines.Deadline{Status: deadlines.Overdue, KmLeft: i64(-10)}, "overdue"},
	}
	for i, c := range cases {
		if got := Stage(c.d, days()); got != c.want {
			t.Errorf("%d: got %q, want %q", i, got, c.want)
		}
	}
}

func TestLine(t *testing.T) {
	d := deadlines.Deadline{Kind: deadlines.Service, VehicleName: "Panda", Due: "2027-01-06", DueKm: i64(45000),
		KmLeft: i64(5000), DaysLeft: ip(100), Status: deadlines.OK}
	if got := Line(d, "it"); got != "Tagliando – Panda: entro il 06/01/2027 o a 45.000 km · mancano 5.000 km · tra 100 giorni" {
		t.Errorf("it: %s", got)
	}
	if got := Line(d, "en"); got != "Service – Panda: by 6 Jan 2027 or at 45,000 km · 5,000 km left · in 100 days" {
		t.Errorf("en: %s", got)
	}
	g := deadlines.Deadline{Kind: deadlines.Insurance, VehicleName: "Vespa", Due: "2026-09-20", GraceUntil: "2026-10-05",
		DaysLeft: ip(-8), Status: deadlines.Grace}
	if got := Line(g, "it"); got != "Scadenza assicurazione – Vespa: scade il 20/09/2026 · scaduta, tolleranza fino al 05/10/2026" {
		t.Errorf("grace: %s", got)
	}
	m := Digest([]deadlines.Deadline{d, g}, "it", "")
	if m.Title != "MILE: 2 scadenze da controllare" || !m.Urgent || len(m.Lines) != 2 {
		t.Errorf("digest: %+v", m)
	}
}

func TestNtfy(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	s := store.NotificationSettings{NtfyEnabled: true, NtfyURL: srv.URL, NtfyTopic: "mile-test", NtfyToken: "tk_abc"}
	n := &Ntfy{}
	if !n.Enabled(s) {
		t.Fatal("should be enabled")
	}
	err := n.Send(context.Background(), s, Message{Title: "MILE: 1 scadenza", Lines: []string{"Revisione – Panda"}, URL: "https://m.example", Urgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if got["topic"] != "mile-test" || got["title"] != "MILE: 1 scadenza" || got["click"] != "https://m.example" || got["priority"] != float64(4) {
		t.Errorf("payload: %v", got)
	}
	if auth != "Bearer tk_abc" {
		t.Errorf("auth: %q", auth)
	}
}

// fakeSMTP accepts one message without TLS and returns what it received.
func fakeSMTP(t *testing.T) (addr string, received chan string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received = make(chan string, 1)
	go func() {
		defer l.Close()
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { c.Write([]byte(s + "\r\n")) }
		say("220 fake")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					received <- data.String()
					say("250 ok")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250 fake")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	return l.Addr().String(), received
}

func TestEmail(t *testing.T) {
	addr, received := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	var p int
	json.Unmarshal([]byte(port), &p)
	e := &Email{Config: SMTPConfig{Host: host, Port: p, From: "MILE <mile@example.com>", TLS: "none"}, Timeout: 5 * time.Second}
	s := store.NotificationSettings{Email: "me@example.com", EmailEnabled: true}
	if !e.Enabled(s) {
		t.Fatal("should be enabled")
	}
	m := Message{Title: "MILE: 1 scadenza da controllare", Lines: []string{"Revisione – Panda: entro il 31/10/2026"}, URL: "https://m.example", Locale: "it"}
	if err := e.Send(context.Background(), s, m); err != nil {
		t.Fatal(err)
	}
	msg := <-received
	if !strings.Contains(msg, "Subject: MILE: 1 scadenza da controllare\r\n") || !strings.Contains(msg, "To: me@example.com") {
		t.Errorf("headers:\n%s", msg)
	}
	body := msg[strings.Index(msg, "\r\n\r\n")+4:]
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil || !strings.Contains(string(dec), "• Revisione – Panda: entro il 31/10/2026") || !strings.Contains(string(dec), "Apri MILE: https://m.example") {
		t.Errorf("body: %v %s", err, dec)
	}
	if (&Email{}).Enabled(s) {
		t.Error("email without SMTP configuration must be disabled")
	}
}

type fakeChannel struct {
	sent []Message
	err  error
}

func (f *fakeChannel) Name() string                              { return "fake" }
func (f *fakeChannel) Enabled(s store.NotificationSettings) bool { return s.NtfyEnabled }
func (f *fakeChannel) Send(_ context.Context, _ store.NotificationSettings, m Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func TestRunnerSendsOnce(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	st := store.New(d, t.TempDir(), time.UTC)
	u, err := st.CreateFirstUser(store.UserInput{Username: "gabri", Password: "password123", Locale: "it"})
	if err != nil {
		t.Fatal(err)
	}
	due := deadlines.Format(deadlines.Today(time.UTC).AddDate(0, 0, 5))
	if _, err := st.CreateReminder(u.ID, store.ReminderInput{Title: "Rinnovo patente", DueDate: due}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveNotificationSettings(u.ID, store.NotificationInput{NtfyEnabled: true, NtfyTopic: "t", Days: []int{7, 1}}); err != nil {
		t.Fatal(err)
	}

	ch := &fakeChannel{err: errors.New("down")}
	r := &Runner{Store: st, Channels: []Channel{ch}}
	r.RunOnce(context.Background())
	if len(ch.sent) != 0 {
		t.Fatal("nothing should be sent while the channel is down")
	}
	ch.err = nil
	r.RunOnce(context.Background()) // retried
	r.RunOnce(context.Background()) // already sent: nothing new
	if len(ch.sent) != 1 || !strings.Contains(ch.sent[0].Lines[0], "Rinnovo patente") || !strings.Contains(ch.sent[0].Lines[0], "tra 5 giorni") {
		t.Fatalf("sent: %+v", ch.sent)
	}
}
