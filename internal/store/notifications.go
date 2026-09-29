// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"net/mail"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NotificationSettings are the user's channels. The ntfy token is a secret:
// it is never sent back to the browser (see HasNtfyToken).
type NotificationSettings struct {
	Email        string `json:"email"`
	EmailEnabled bool   `json:"email_enabled"`
	NtfyURL      string `json:"ntfy_url"`
	NtfyTopic    string `json:"ntfy_topic"`
	NtfyToken    string `json:"-"`
	HasNtfyToken bool   `json:"has_ntfy_token"`
	NtfyEnabled  bool   `json:"ntfy_enabled"`
	Days         []int  `json:"days"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// NotificationInput: a nil NtfyToken keeps the stored one, "" removes it.
type NotificationInput struct {
	Email        string  `json:"email"`
	EmailEnabled bool    `json:"email_enabled"`
	NtfyURL      string  `json:"ntfy_url"`
	NtfyTopic    string  `json:"ntfy_topic"`
	NtfyToken    *string `json:"ntfy_token"`
	NtfyEnabled  bool    `json:"ntfy_enabled"`
	Days         []int   `json:"days"`
}

const DefaultNtfyURL = "https://ntfy.sh"

var defaultDays = []int{30, 7, 1}

func defaultSettings() NotificationSettings {
	return NotificationSettings{NtfyURL: DefaultNtfyURL, Days: append([]int(nil), defaultDays...)}
}

func parseDays(s string) []int {
	out := []int{}
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n >= 0 && n <= 365 {
			out = append(out, n)
		}
	}
	return normDays(out)
}

// normDays sorts the days from the furthest, without duplicates.
func normDays(d []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, n := range d {
		if n >= 0 && n <= 365 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func joinDays(d []int) string {
	s := make([]string, len(d))
	for i, n := range d {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

func (s *Store) GetNotificationSettings(userID int64) (*NotificationSettings, error) {
	st := defaultSettings()
	var days string
	err := s.DB.QueryRow(`SELECT email, email_enabled, ntfy_url, ntfy_topic, ntfy_token, ntfy_enabled, days, updated_at
		FROM notification_settings WHERE user_id = ?`, userID).
		Scan(&st.Email, &st.EmailEnabled, &st.NtfyURL, &st.NtfyTopic, &st.NtfyToken, &st.NtfyEnabled, &days, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &st, nil
	}
	if err != nil {
		return nil, err
	}
	st.Days = parseDays(days)
	st.HasNtfyToken = st.NtfyToken != ""
	return &st, nil
}

// ValidNtfyURL accepts only http(s) server addresses without credentials or query.
func ValidNtfyURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return strings.TrimRight(u.String(), "/"), true
}

func validTopic(t string) bool {
	if len(t) < 1 || len(t) > 64 {
		return false
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (s *Store) SaveNotificationSettings(userID int64, in NotificationInput) (*NotificationSettings, error) {
	cur, err := s.GetNotificationSettings(userID)
	if err != nil {
		return nil, err
	}
	in.Email = Clean(in.Email)
	in.NtfyTopic = Clean(in.NtfyTopic)
	if in.Email != "" {
		a, err := mail.ParseAddress(in.Email)
		if err != nil || a.Name != "" {
			return nil, invalid("email_invalid", "Invalid email address")
		}
		in.Email = a.Address
	}
	if in.EmailEnabled && in.Email == "" {
		return nil, invalid("email_invalid", "Invalid email address")
	}
	if in.NtfyURL == "" {
		in.NtfyURL = DefaultNtfyURL
	}
	u, ok := ValidNtfyURL(in.NtfyURL)
	if !ok {
		return nil, invalid("ntfy_url_invalid", "Invalid ntfy server address")
	}
	in.NtfyURL = u
	if in.NtfyTopic != "" && !validTopic(in.NtfyTopic) {
		return nil, invalid("ntfy_topic_invalid", "Topic: 1-64 letters, digits, - and _")
	}
	if in.NtfyEnabled && in.NtfyTopic == "" {
		return nil, invalid("ntfy_topic_invalid", "Topic: 1-64 letters, digits, - and _")
	}
	token := cur.NtfyToken
	if in.NtfyToken != nil {
		token = strings.TrimSpace(*in.NtfyToken)
	}
	days := normDays(in.Days)
	if len(days) == 0 {
		days = append([]int(nil), defaultDays...)
	}
	_, err = s.DB.Exec(`INSERT INTO notification_settings (user_id, email, email_enabled, ntfy_url, ntfy_topic, ntfy_token, ntfy_enabled, days, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET email = excluded.email, email_enabled = excluded.email_enabled,
			ntfy_url = excluded.ntfy_url, ntfy_topic = excluded.ntfy_topic, ntfy_token = excluded.ntfy_token,
			ntfy_enabled = excluded.ntfy_enabled, days = excluded.days, updated_at = excluded.updated_at`,
		userID, in.Email, boolInt(in.EmailEnabled), in.NtfyURL, in.NtfyTopic, token, boolInt(in.NtfyEnabled), joinDays(days), now())
	if err != nil {
		return nil, err
	}
	return s.GetNotificationSettings(userID)
}

type NotifyTarget struct {
	User     User
	Settings NotificationSettings
}

// NotifyTargets returns the users with at least one channel enabled.
func (s *Store) NotifyTargets() ([]NotifyTarget, error) {
	rows, err := s.DB.Query(`SELECT user_id FROM notification_settings WHERE email_enabled = 1 OR ntfy_enabled = 1`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []NotifyTarget
	for _, id := range ids {
		u, err := s.GetUser(id)
		if err != nil {
			return nil, err
		}
		st, err := s.GetNotificationSettings(id)
		if err != nil {
			return nil, err
		}
		out = append(out, NotifyTarget{User: *u, Settings: *st})
	}
	return out, nil
}

func (s *Store) NotificationSent(userID int64, key, stage string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM notification_log WHERE user_id = ? AND deadline_key = ? AND stage = ?`,
		userID, key, stage).Scan(&n)
	return n > 0, err
}

func (s *Store) MarkNotificationSent(userID int64, key, stage string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO notification_log (user_id, deadline_key, stage, sent_at) VALUES (?, ?, ?, ?)`,
		userID, key, stage, now())
	return err
}

// PruneNotificationLog forgets notifications older than about a year and a half.
func (s *Store) PruneNotificationLog() error {
	_, err := s.DB.Exec(`DELETE FROM notification_log WHERE sent_at < ?`,
		time.Now().UTC().AddDate(0, -18, 0).Format(time.DateTime))
	return err
}
