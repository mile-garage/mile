// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mile-garage/mile/internal/store"
)

func TestLoginLimit(t *testing.T) {
	ts := newServer(t)
	admin := newClient(t, ts)
	admin.must("POST", "/api/setup", map[string]string{"username": "gabri", "password": "password123"}, nil)

	c := newClient(t, ts)
	for i := 0; i < maxLoginFailures; i++ {
		if code := c.do("POST", "/api/login", map[string]string{"username": "Gabri", "password": "wrong-one"}, nil); code != 401 {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	// the right password is refused too until the window has passed
	if code := c.do("POST", "/api/login", map[string]string{"username": "gabri", "password": "password123"}, nil); code != 429 {
		t.Fatalf("after %d failures: %d", maxLoginFailures, code)
	}
	// another username is not affected
	if code := c.do("POST", "/api/login", map[string]string{"username": "lisa", "password": "whatever1"}, nil); code != 401 {
		t.Fatalf("other username: %d", code)
	}
}

func TestLoginLimiterWindow(t *testing.T) {
	l := newLoginLimiter()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	for i := 0; i < maxLoginFailures; i++ {
		l.failed("gabri")
	}
	if l.allow("GABRI") {
		t.Fatal("should be blocked, whatever the case")
	}
	now = now.Add(loginWindow + time.Second)
	if !l.allow("gabri") {
		t.Fatal("should be allowed after the window")
	}
	l.failed("gabri")
	l.reset("gabri")
	if len(l.failures) != 0 {
		t.Fatalf("reset: %v", l.failures)
	}
}

func TestSetupOnce(t *testing.T) {
	ts := newServer(t)
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newClient(t, ts)
			codes <- c.do("POST", "/api/setup", map[string]string{"username": "admin" + itoa(int64(i)), "password": "password123"}, nil)
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		if c == 200 {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d administrators created at the same time", ok)
	}
}

func TestReminderMadePersonal(t *testing.T) {
	ts := newServer(t)
	admin := newClient(t, ts)
	admin.must("POST", "/api/setup", map[string]string{"username": "gabri", "password": "password123"}, nil)
	admin.must("POST", "/api/users", map[string]string{"username": "lisa", "password": "password456"}, nil)
	var v store.Vehicle
	admin.must("POST", "/api/vehicles", map[string]any{"make": "Fiat", "model": "Panda", "kind": "car", "fuel_type": "petrol"}, &v)
	admin.must("PUT", "/api/vehicles/"+itoa(v.ID)+"/members", map[string]string{"username": "lisa", "role": "editor"}, nil)
	var r store.Reminder
	admin.must("POST", "/api/reminders", map[string]any{"vehicle_id": v.ID, "title": "Lavaggio", "due_date": "2027-01-01"}, &r)

	// lisa, an editor of the vehicle, makes the reminder personal: it becomes hers
	lisa := newClient(t, ts)
	lisa.must("POST", "/api/login", map[string]string{"username": "lisa", "password": "password456"}, nil)
	lisa.must("PUT", "/api/reminders/"+itoa(r.ID), map[string]any{"vehicle_id": nil, "title": "Lavaggio", "due_date": "2027-01-01"}, nil)
	var mine, hers []store.Reminder
	admin.must("GET", "/api/reminders", nil, &mine)
	lisa.must("GET", "/api/reminders", nil, &hers)
	if len(mine) != 0 || len(hers) != 1 {
		t.Fatalf("admin sees %d, lisa sees %d", len(mine), len(hers))
	}
}

func TestPageCSP(t *testing.T) {
	script := "\n      var t = 'dark'\n    "
	csp := pageCSP([]byte("<head><script>" + script + "</script><script type=\"module\" src=\"/assets/x.js\"></script></head>"))
	sum := sha256.Sum256([]byte(script))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(csp, "script-src 'self' "+want+";") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("csp: %s", csp)
	}
}

func TestSecureCookieFromBaseURL(t *testing.T) {
	app := newApp(t)
	app.SetBaseURL("https://mile.example.com")
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	c := newClient(t, ts)
	req, _ := http.NewRequest("POST", ts.URL+"/api/setup", strings.NewReader(`{"username":"gabri","password":"password123"}`))
	req.Header.Set("X-Requested-With", "fetch")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if sc := res.Header.Get("Set-Cookie"); !strings.Contains(sc, "Secure") {
		t.Fatalf("cookie without Secure behind an https base URL: %s", sc)
	}
}

func TestSSOSessionNotExtended(t *testing.T) {
	app := newApp(t)
	u, err := app.store.CreateFirstUser(store.UserInput{Username: "gabri", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	expiry := func(token string) time.Time {
		var s string
		app.store.DB.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, hashOf(token)).Scan(&s)
		e, _ := time.Parse(time.DateTime, s)
		return e
	}
	sso, _ := app.store.CreateSession(u.ID, true)
	pw, _ := app.store.CreateSession(u.ID, false)
	// pretend both were opened two days ago
	app.store.DB.Exec(`UPDATE sessions SET expires_at = datetime(expires_at, '-2 days')`)
	before := expiry(sso)
	if _, err := app.store.SessionUser(sso); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SessionUser(pw); err != nil {
		t.Fatal(err)
	}
	if !expiry(sso).Equal(before) {
		t.Fatal("an SSO session was extended")
	}
	if d := time.Until(before); d > 29*24*time.Hour || d < 27*24*time.Hour {
		t.Fatalf("SSO session lasts %v", d)
	}
	if time.Until(expiry(pw)) < 364*24*time.Hour {
		t.Fatal("a password session was not extended")
	}
}

func hashOf(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
