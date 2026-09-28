// Package store contains data access and validation.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/mile-garage/mile/internal/db"
)

type Store struct {
	DB       *sql.DB
	FilesDir string
	Loc      *time.Location // time zone used to decide what "today" is
}

func New(d *sql.DB, filesDir string, loc *time.Location) *Store {
	return &Store{DB: d, FilesDir: filesDir, Loc: loc}
}

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
)

// ValidationError is shown to the user: Code is translated by the frontend,
// Msg is the English fallback.
type ValidationError struct {
	Code string
	Msg  string
}

func (e *ValidationError) Error() string { return e.Msg }

func invalid(code, msg string) error { return &ValidationError{Code: code, Msg: msg} }

// Clean normalises a single-line text value.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// CleanMultiline is like Clean but keeps line breaks (for notes).
func CleanMultiline(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = Clean(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func cleanPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := Clean(*s)
	if v == "" {
		return nil
	}
	return &v
}

var (
	dateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
)

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func validMonth(s string) bool {
	if !monthRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01", s)
	return err == nil
}

// checkDate validates a required date.
func checkDate(s, field string) error {
	if !validDate(s) {
		return invalid("invalid_date", "Invalid date: "+field)
	}
	return nil
}

// checkOptDate validates an optional date, turning "" into nil.
func checkOptDate(s **string, field string) error {
	*s = cleanPtr(*s)
	if *s != nil && !validDate(**s) {
		return invalid("invalid_date", "Invalid date: "+field)
	}
	return nil
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func now() string { return db.Now() }

// placeholders returns "?, ?, ?" for n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
