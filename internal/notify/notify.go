// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package notify sends a daily digest of the deadlines that need attention,
// by email and ntfy. Each deadline is notified once per stage (N days before,
// due soon by km, overdue), so a user never gets the same reminder twice.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mile-garage/mile/internal/deadlines"
	"github.com/mile-garage/mile/internal/store"
)

// Message is one digest for one user.
type Message struct {
	Title  string   // short, for the email subject and the ntfy title
	Lines  []string // one per deadline
	URL    string   // link to open MILE, empty if the base URL is not configured
	Locale string
	Urgent bool // something is overdue: higher priority on ntfy
}

// Channel delivers a message to a user.
type Channel interface {
	Name() string
	// Enabled reports whether the user wants this channel and the server can use it.
	Enabled(s store.NotificationSettings) bool
	Send(ctx context.Context, s store.NotificationSettings, m Message) error
}

type Runner struct {
	Store    *store.Store
	Channels []Channel
	Hour     int    // digests are sent from this local hour on
	BaseURL  string // e.g. https://mile.example.com, for links in the messages
	Now      func() time.Time
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Run checks every 10 minutes; the digest goes out once the configured hour
// has passed, and deduplication keeps it to once a day per deadline stage.
func (r *Runner) Run(ctx context.Context) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for {
		if r.now().Hour() >= r.Hour {
			if err := r.RunOnce(ctx); err != nil {
				slog.Error("notifications", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Stage returns which reminder a deadline is at, or "" if none is due yet.
// days are the user's lead times, sorted from the furthest.
func Stage(d deadlines.Deadline, days []int) string {
	switch d.Status {
	case deadlines.Suspended:
		return "" // a suspended policy is a choice, not something to remind
	case deadlines.Overdue, deadlines.Grace:
		return "overdue"
	}
	if d.DaysLeft != nil {
		stage := ""
		for _, n := range days { // the closest threshold already reached wins
			if *d.DaysLeft <= n {
				stage = "d" + strconv.Itoa(n)
			}
		}
		if stage != "" {
			return stage
		}
	}
	if d.Status == deadlines.DueSoon && d.KmLeft != nil {
		return "km"
	}
	return ""
}

// Key identifies a deadline occurrence: it changes when the deadline moves.
func Key(d deadlines.Deadline) string {
	km := int64(0)
	if d.DueKm != nil {
		km = *d.DueKm
	}
	return fmt.Sprintf("%s-%d-%d-%s-%d", d.Kind, d.VehicleID, d.RefID, d.Due, km)
}

// RunOnce sends the pending notifications to every user who enabled them.
func (r *Runner) RunOnce(ctx context.Context) error {
	targets, err := r.Store.NotifyTargets()
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err := r.notifyUser(ctx, t); err != nil {
			slog.Error("notifications", "user", t.User.Username, "err", err)
		}
	}
	return r.Store.PruneNotificationLog()
}

type pending struct {
	d     deadlines.Deadline
	stage string
}

func (r *Runner) notifyUser(ctx context.Context, t store.NotifyTarget) error {
	channels := r.enabled(t.Settings)
	if len(channels) == 0 {
		return nil
	}
	ds, err := r.Store.Deadlines(t.User.ID, nil)
	if err != nil {
		return err
	}
	var todo []pending
	for _, d := range ds {
		stage := Stage(d, t.Settings.Days)
		if stage == "" {
			continue
		}
		sent, err := r.Store.NotificationSent(t.User.ID, Key(d), stage)
		if err != nil {
			return err
		}
		if !sent {
			todo = append(todo, pending{d, stage})
		}
	}
	if len(todo) == 0 {
		return nil
	}
	list := make([]deadlines.Deadline, len(todo))
	for i, p := range todo {
		list[i] = p.d
	}
	msg := Digest(list, t.User.Locale, r.BaseURL)
	if err := r.send(ctx, channels, t.Settings, msg); err != nil {
		return err // nothing is marked: it will be retried at the next check
	}
	for _, p := range todo {
		if err := r.Store.MarkNotificationSent(t.User.ID, Key(p.d), p.stage); err != nil {
			return err
		}
	}
	slog.Info("notifications sent", "user", t.User.Username, "deadlines", len(todo))
	return nil
}

func (r *Runner) enabled(s store.NotificationSettings) []Channel {
	var out []Channel
	for _, c := range r.Channels {
		if c.Enabled(s) {
			out = append(out, c)
		}
	}
	return out
}

// send succeeds if at least one channel delivered the message.
func (r *Runner) send(ctx context.Context, channels []Channel, s store.NotificationSettings, m Message) error {
	var errs []error
	for _, c := range channels {
		if err := c.Send(ctx, s, m); err != nil {
			slog.Error("notification channel", "channel", c.Name(), "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", c.Name(), err))
		}
	}
	if len(errs) == len(channels) {
		return errors.Join(errs...)
	}
	return nil
}

// Test sends a test message on every channel the user enabled and reports
// the outcome of each one ("ok" or the error).
func (r *Runner) Test(ctx context.Context, u store.User, s store.NotificationSettings) map[string]string {
	tx := texts[deadlines.Locale(u.Locale)]
	msg := Message{Title: tx["testTitle"], Lines: []string{tx["testBody"]}, URL: r.BaseURL, Locale: deadlines.Locale(u.Locale)}
	out := map[string]string{}
	for _, c := range r.enabled(s) {
		if err := c.Send(ctx, s, msg); err != nil {
			out[c.Name()] = err.Error()
		} else {
			out[c.Name()] = "ok"
		}
	}
	return out
}

// ---- texts ----

var texts = map[string]map[string]string{
	"it": {
		"one":       "MILE: 1 scadenza da controllare",
		"many":      "MILE: %d scadenze da controllare",
		"due":       "entro il %s",
		"expires":   "scade il %s",
		"byKm":      "o a %s km",
		"atKm":      "a %s km",
		"kmLeft":    "mancano %s km",
		"kmOver":    "superato di %s km",
		"today":     "oggi",
		"tomorrow":  "domani",
		"inDays":    "tra %d giorni",
		"overdue":   "SCADUTA",
		"grace":     "scaduta, tolleranza fino al %s",
		"open":      "Apri MILE",
		"footer":    "Ricevi questo messaggio perché hai attivato le notifiche in MILE (Impostazioni › Notifiche).",
		"testTitle": "MILE: notifica di prova",
		"testBody":  "Le notifiche funzionano. Riceverai qui gli avvisi delle scadenze dei tuoi veicoli.",
	},
	"en": {
		"one":       "MILE: 1 deadline to check",
		"many":      "MILE: %d deadlines to check",
		"due":       "by %s",
		"expires":   "expires on %s",
		"byKm":      "or at %s km",
		"atKm":      "at %s km",
		"kmLeft":    "%s km left",
		"kmOver":    "%s km over",
		"today":     "today",
		"tomorrow":  "tomorrow",
		"inDays":    "in %d days",
		"overdue":   "OVERDUE",
		"grace":     "expired, grace period until %s",
		"open":      "Open MILE",
		"footer":    "You receive this message because you enabled notifications in MILE (Settings › Notifications).",
		"testTitle": "MILE: test notification",
		"testBody":  "Notifications work. You will get your vehicle deadlines here.",
	},
}

// Digest builds the message for a list of deadlines, most urgent first.
func Digest(ds []deadlines.Deadline, locale, baseURL string) Message {
	locale = deadlines.Locale(locale)
	tx := texts[locale]
	m := Message{URL: baseURL, Locale: locale}
	if len(ds) == 1 {
		m.Title = tx["one"]
	} else {
		m.Title = fmt.Sprintf(tx["many"], len(ds))
	}
	for _, d := range ds {
		m.Lines = append(m.Lines, Line(d, locale))
		if d.Status == deadlines.Overdue || d.Status == deadlines.Grace {
			m.Urgent = true
		}
	}
	return m
}

// Line describes one deadline: "Revisione – Panda: entro il 31/10/2026 (tra 33 giorni)".
func Line(d deadlines.Deadline, locale string) string {
	tx := texts[deadlines.Locale(locale)]
	s := d.TitleIn(locale)
	if d.VehicleName != "" {
		s += " – " + d.VehicleName
	}
	var parts []string
	if d.Due != "" {
		key := "due"
		if d.Kind == deadlines.Insurance {
			key = "expires"
		}
		p := fmt.Sprintf(tx[key], deadlines.FormatDateIn(d.Due, locale))
		if d.DueKm != nil {
			p += " " + fmt.Sprintf(tx["byKm"], deadlines.FormatKm(*d.DueKm, locale))
		}
		parts = append(parts, p)
	} else if d.DueKm != nil {
		parts = append(parts, fmt.Sprintf(tx["atKm"], deadlines.FormatKm(*d.DueKm, locale)))
	}
	if d.KmLeft != nil {
		if *d.KmLeft >= 0 {
			parts = append(parts, fmt.Sprintf(tx["kmLeft"], deadlines.FormatKm(*d.KmLeft, locale)))
		} else {
			parts = append(parts, fmt.Sprintf(tx["kmOver"], deadlines.FormatKm(-*d.KmLeft, locale)))
		}
	}
	switch {
	case d.Status == deadlines.Grace:
		parts = append(parts, fmt.Sprintf(tx["grace"], deadlines.FormatDateIn(d.GraceUntil, locale)))
	case d.Status == deadlines.Overdue:
		parts = append(parts, tx["overdue"])
	case d.DaysLeft != nil && *d.DaysLeft == 0:
		parts = append(parts, tx["today"])
	case d.DaysLeft != nil && *d.DaysLeft == 1:
		parts = append(parts, tx["tomorrow"])
	case d.DaysLeft != nil && *d.DaysLeft > 1:
		parts = append(parts, fmt.Sprintf(tx["inDays"], *d.DaysLeft))
	}
	if len(parts) > 0 {
		s += ": " + strings.Join(parts, " · ")
	}
	return s
}
