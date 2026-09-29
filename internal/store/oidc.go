// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"errors"
	"regexp"
	"strings"
)

// OIDCIdentity is a user at an OpenID Connect provider: the issuer and
// subject of their ID tokens, which never change (unlike the username).
type OIDCIdentity struct {
	Issuer  string
	Subject string
}

func (s *Store) UserByOIDC(id OIDCIdentity) (*User, error) {
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE oidc_issuer = ? AND oidc_subject = ?`, id.Issuer, id.Subject))
}

var usernameBad = regexp.MustCompile(`[^A-Za-z0-9._@-]+`)

// OIDCUsername turns the first usable candidate (preferred_username, email…)
// into a valid username, or returns "".
func OIDCUsername(candidates ...string) string {
	for _, c := range candidates {
		c = strings.Trim(usernameBad.ReplaceAllString(Clean(c), "-"), "-")
		if len(c) > 64 {
			c = c[:64]
		}
		if usernameRe.MatchString(c) {
			return c
		}
	}
	return ""
}

// CreateOIDCUser creates a user without a password, linked to the identity.
// The first user of a new installation becomes an administrator, as with setup.
func (s *Store) CreateOIDCUser(id OIDCIdentity, username, displayName string, admin bool) (*User, error) {
	if !usernameRe.MatchString(username) {
		return nil, invalid("username_invalid", "Username: 2-64 characters, letters, digits and . _ @ -")
	}
	n, err := s.CountUsers()
	if err != nil {
		return nil, err
	}
	in := UserInput{Username: username, DisplayName: Clean(displayName), IsAdmin: admin || n == 0}
	u, err := s.insertUser(in, "", &id)
	if ve := (*ValidationError)(nil); errors.As(err, &ve) && ve.Code == "username_taken" {
		return nil, invalid("sso_username_taken", "A user named "+username+" already exists: log in with the password and link the account in Settings")
	}
	return u, err
}

// LinkOIDC links the identity to an existing user, replacing any previous one.
func (s *Store) LinkOIDC(userID int64, id OIDCIdentity) error {
	_, err := s.DB.Exec(`UPDATE users SET oidc_issuer = ?, oidc_subject = ?, updated_at = ? WHERE id = ?`, id.Issuer, id.Subject, now(), userID)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return invalid("sso_linked_other", "This account is already linked to another MILE user")
	}
	return err
}

// UnlinkOIDC refuses to leave a user without a way to log in.
func (s *Store) UnlinkOIDC(userID int64) error {
	u, err := s.GetUser(userID)
	if err != nil {
		return err
	}
	if !u.HasPassword {
		return invalid("sso_no_password", "Set a password before unlinking the account")
	}
	_, err = s.DB.Exec(`UPDATE users SET oidc_issuer = NULL, oidc_subject = NULL, updated_at = ? WHERE id = ?`, now(), userID)
	return err
}

// SetAdmin grants or revokes the administrator role; the last administrator
// is never demoted.
func (s *Store) SetAdmin(userID int64, admin bool) error {
	q := `UPDATE users SET is_admin = 1, updated_at = ? WHERE id = ? AND is_admin = 0`
	if !admin {
		q = `UPDATE users SET is_admin = 0, updated_at = ? WHERE id = ? AND is_admin = 1
			AND (SELECT COUNT(*) FROM users WHERE is_admin = 1) > 1`
	}
	_, err := s.DB.Exec(q, now(), userID)
	return err
}
