// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package importer reads the CSV exports of other apps (Fuelio, LubeLogger)
// into MILE records. Values are converted to km, litres and cents.
package importer

import (
	"encoding/csv"
	"io"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/mile-garage/mile/internal/store"
)

// Warning is something the user should know about the import, e.g. rows
// that MILE cannot store; the frontend translates the code.
type Warning struct {
	Code string `json:"code"`
	N    int    `json:"n,omitempty"`
}

// Error is a file that cannot be imported at all.
type Error struct{ Code, Msg string }

func (e *Error) Error() string { return e.Msg }

func fail(code, msg string) error { return &Error{Code: code, Msg: msg} }

// Units of the source file.
const (
	kmPerMile    = 1.609344
	litresUSGal  = 3.785411784
	litresImpGal = 4.54609
)

type units struct {
	km     float64 // km per distance unit
	litres float64 // litres per volume unit
}

var metric = units{1, 1}

func (u units) converted() bool { return u != metric }

func (u units) odometer(v float64) int64 { return int64(math.Round(v * u.km)) }

func (u units) volume(v float64) float64 { return math.Round(v*u.litres*1000) / 1000 }

func cents(v float64) int64 { return int64(math.Round(v * 100)) }

// cell cleans a CSV value: spaces, byte order marks and non-breaking spaces.
func cell(s string) string {
	s = strings.ReplaceAll(s, string(rune(0xFEFF)), "")
	return strings.TrimFunc(s, unicode.IsSpace)
}

// header maps normalised column names to their index.
type header map[string]int

func newHeader(rec []string) header {
	h := header{}
	for i, name := range rec {
		h[strings.ToLower(cell(name))] = i
	}
	return h
}

// get returns the value of the column, or "" if the row or the file lacks it.
func (h header) get(rec []string, name string) string {
	if i, ok := h[name]; ok && i < len(rec) {
		return cell(rec[i])
	}
	return ""
}

// prefix finds the column whose name starts with p, e.g. "odo (" for "Odo (km)".
func (h header) prefix(p string) (string, bool) {
	for name := range h {
		if strings.HasPrefix(name, p) {
			return name, true
		}
	}
	return "", false
}

func (h header) has(names ...string) bool {
	for _, n := range names {
		if _, ok := h[n]; !ok {
			return false
		}
	}
	return true
}

// row is a CSV record and the line it starts on.
type row struct {
	line int
	rec  []string
}

func readCSV(r io.Reader) ([]row, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	var out []row
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fail("import_bad_csv", "The file is not a valid CSV: "+err.Error())
		}
		line, _ := cr.FieldPos(0)
		if len(rec) == 1 && cell(rec[0]) == "" {
			continue
		}
		out = append(out, row{line, rec})
	}
}

// joinNotes puts the non-empty parts on separate lines.
func joinNotes(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n")
}

// category guesses the MILE expense category from a name in English or
// Italian. Short words must match a whole word ("mot", not "motore").
func category(name string) (string, bool) {
	n := strings.ToLower(name)
	words := strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) })
	for _, c := range categoryWords {
		for _, w := range c.words {
			if len(w) > 4 && strings.Contains(n, w) || slices.Contains(words, w) {
				return c.category, true
			}
		}
	}
	return "", false
}

// Checked in order: "oil" before "service", so "Oil service" is an oil change.
var categoryWords = []struct {
	category string
	words    []string
}{
	{"oil_change", []string{"oil", "olio"}},
	{"tyres", []string{"tyre", "tyres", "tire", "tires", "gomme", "pneumatic"}},
	{"inspection", []string{"mot", "inspection", "revisione", "collaudo"}},
	{"road_tax", []string{"tax", "bollo", "registration", "immatricolazione"}},
	{"service", []string{"service", "tagliando"}},
	{"repair", []string{"repair", "riparazione", "carrozzeria", "collision"}},
	{"maintenance", []string{"maintenance", "manutenzione"}},
	{"parking", []string{"parking", "parcheggio", "sosta"}},
	{"tolls", []string{"toll", "tolls", "pedaggio", "pedaggi", "autostrada", "telepass"}},
	{"fine", []string{"fine", "fines", "ticket", "multa", "multe", "contravvenzione"}},
	{"wash", []string{"wash", "lavaggio"}},
	{"accessories", []string{"accessories", "accessori", "tuning", "upgrade"}},
}

// Result is what a file contains, ready for store.Import.
type Result struct {
	Batch    store.ImportBatch
	Invalid  []store.InvalidLine // rows with unreadable values
	Warnings []Warning
}

func (r *Result) warn(code string, n int) {
	if n > 0 {
		r.Warnings = append(r.Warnings, Warning{Code: code, N: n})
	}
}

func (r *Result) invalid(line int, what string) {
	r.Invalid = append(r.Invalid, store.InvalidLine{Line: line, Code: "import_value", Message: "Unreadable value: " + what})
}
