// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"strings"
	"sync"
	"time"

	"github.com/mile-garage/mile/internal/store"
)

// Login throttling. Failed attempts are counted per username, not per IP:
// behind a reverse proxy every request has the proxy's address. After
// maxLoginFailures wrong passwords in loginWindow that username is refused
// until the window has passed. A semaphore also caps the bcrypt checks running
// at once, so a flood of logins cannot take all the CPU of a small server.
const (
	maxLoginFailures = 5
	loginWindow      = time.Minute
	maxLoginChecks   = 4
)

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	checks   chan struct{}
	now      func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string][]time.Time{}, checks: make(chan struct{}, maxLoginChecks), now: time.Now}
}

// recent drops the failures older than the window; the caller holds mu.
func (l *loginLimiter) recent(key string) []time.Time {
	since := l.now().Add(-loginWindow)
	ts := l.failures[key]
	i := 0
	for i < len(ts) && ts[i].Before(since) {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.failures, key)
	} else {
		l.failures[key] = ts
	}
	return ts
}

func (l *loginLimiter) allow(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(strings.ToLower(username))) < maxLoginFailures
}

func (l *loginLimiter) failed(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := strings.ToLower(username)
	l.failures[key] = append(l.recent(key), l.now())
	// keep the map small when many usernames are tried
	if len(l.failures) > 10000 {
		for k := range l.failures {
			l.recent(k)
		}
	}
}

func (l *loginLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, strings.ToLower(username))
}

// check runs a password check, at most maxLoginChecks at a time.
func (l *loginLimiter) check(f func() (*store.User, error)) (*store.User, error) {
	l.checks <- struct{}{}
	defer func() { <-l.checks }()
	return f()
}
