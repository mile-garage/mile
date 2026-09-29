// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
	Locale      string `json:"locale"`
	CreatedAt   string `json:"created_at"`
	HasPassword bool   `json:"has_password"`
	SSO         bool   `json:"sso"` // linked to an OpenID Connect identity
}

const (
	minPassword     = 8
	sessionDuration = 365 * 24 * time.Hour
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{2,64}$`)

var locales = map[string]bool{"": true, "it": true, "en": true}

const userCols = `id, username, display_name, is_admin, locale, created_at, password_hash != '', oidc_subject IS NOT NULL`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Locale, &u.CreatedAt, &u.HasPassword, &u.SSO)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func hashPassword(p string) (string, error) {
	if len(p) < minPassword {
		return "", invalid("password_short", "The password must be at least 8 characters long")
	}
	if len(p) > 72 {
		return "", invalid("password_long", "The password must be at most 72 characters long")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(h), err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Locale      string `json:"locale"`
	IsAdmin     bool   `json:"is_admin"`
}

func (s *Store) CreateUser(in UserInput) (*User, error) {
	in.Username = Clean(in.Username)
	in.DisplayName = Clean(in.DisplayName)
	if !usernameRe.MatchString(in.Username) {
		return nil, invalid("username_invalid", "Username: 2-64 characters, letters, digits and . _ @ -")
	}
	if !locales[in.Locale] {
		in.Locale = ""
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	return s.insertUser(in, hash, nil)
}

// insertUser stores a validated user; id links it to an OpenID Connect identity.
func (s *Store) insertUser(in UserInput, hash string, oidc *OIDCIdentity) (*User, error) {
	var issuer, subject any
	if oidc != nil {
		issuer, subject = oidc.Issuer, oidc.Subject
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO users (username, display_name, password_hash, is_admin, locale, ical_token, oidc_issuer, oidc_subject, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, in.Username, in.DisplayName, hash, boolInt(in.IsAdmin), in.Locale, randomToken(24), issuer, subject, t, t)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, invalid("username_taken", "This username is already taken")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(id)
}

// CreateFirstUser creates the administrator, only while there are no users.
func (s *Store) CreateFirstUser(in UserInput) (*User, error) {
	n, err := s.CountUsers()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrForbidden
	}
	in.IsAdmin = true
	return s.CreateUser(in)
}

func (s *Store) GetUser(id int64) (*User, error) {
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByName(username string) (*User, error) {
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ?`, Clean(username)))
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.DB.Query(`SELECT ` + userCols + ` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// dummyHash keeps the login time constant when the user does not exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("mile-dummy-password"), bcrypt.DefaultCost)

// Authenticate returns ErrNotFound for a wrong username or password.
func (s *Store) Authenticate(username, password string) (*User, error) {
	var id int64
	var hash string
	err := s.DB.QueryRow(`SELECT id, password_hash FROM users WHERE username = ?`, Clean(username)).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) || hash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrNotFound
	}
	return s.GetUser(id)
}

func (s *Store) CheckPassword(userID int64, password string) bool {
	var hash string
	if s.DB.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash) != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// SetPassword changes the password and closes the user's other sessions.
func (s *Store) SetPassword(userID int64, password, keepToken string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, now(), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, tokenHash(keepToken))
	return err
}

func (s *Store) UpdateProfile(userID int64, displayName, locale string) (*User, error) {
	if !locales[locale] {
		return nil, invalid("locale_invalid", "Unsupported language")
	}
	if _, err := s.DB.Exec(`UPDATE users SET display_name = ?, locale = ?, updated_at = ? WHERE id = ?`,
		Clean(displayName), locale, now(), userID); err != nil {
		return nil, err
	}
	return s.GetUser(userID)
}

// DeleteUser refuses to remove the last administrator or the only owner of a vehicle.
func (s *Store) DeleteUser(id int64) error {
	u, err := s.GetUser(id)
	if err != nil {
		return err
	}
	if u.IsAdmin {
		var admins int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return invalid("last_admin", "The last administrator cannot be deleted")
		}
	}
	var sole int
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM vehicle_users vu WHERE vu.user_id = ? AND vu.role = 'owner'
		AND NOT EXISTS (SELECT 1 FROM vehicle_users o WHERE o.vehicle_id = vu.vehicle_id AND o.role = 'owner' AND o.user_id != vu.user_id)`, id).Scan(&sole)
	if err != nil {
		return err
	}
	if sole > 0 {
		return invalid("user_owns_vehicles", "This user is the only owner of some vehicles: transfer or delete them first")
	}
	_, err = s.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// ---- calendar token ----

func (s *Store) ICalToken(userID int64) (string, error) {
	var t string
	err := s.DB.QueryRow(`SELECT ical_token FROM users WHERE id = ?`, userID).Scan(&t)
	return t, err
}

func (s *Store) RegenerateICalToken(userID int64) (string, error) {
	t := randomToken(24)
	_, err := s.DB.Exec(`UPDATE users SET ical_token = ?, updated_at = ? WHERE id = ?`, t, now(), userID)
	return t, err
}

func (s *Store) UserByICalToken(token string) (*User, error) {
	if len(token) < 20 {
		return nil, ErrNotFound
	}
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE ical_token = ?`, token))
}

// ---- sessions ----

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Store) CreateSession(userID int64) (string, error) {
	token := randomToken(32)
	n := time.Now().UTC()
	_, err := s.DB.Exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash(token), userID, n.Format(time.DateTime), n.Add(sessionDuration).Format(time.DateTime))
	return token, err
}

// SessionUser returns the user of a valid session, extending sessions older
// than a day so that active users never have to log in again.
func (s *Store) SessionUser(token string) (*User, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	h := tokenHash(token)
	var userID int64
	var expires string
	err := s.DB.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, h).Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.DateTime, expires)
	n := time.Now().UTC()
	if err != nil || n.After(exp) {
		s.DB.Exec(`DELETE FROM sessions WHERE token_hash = ?`, h)
		return nil, ErrNotFound
	}
	if exp.Sub(n) < sessionDuration-24*time.Hour {
		s.DB.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, n.Add(sessionDuration).Format(time.DateTime), h)
	}
	return s.GetUser(userID)
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash(token))
	return err
}

func (s *Store) DeleteExpiredSessions() error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now())
	return err
}
