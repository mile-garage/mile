// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// execer is a *sql.DB or a *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Line is a record read from line Line of an imported file.
type Line[T any] struct {
	Line int
	Rec  T
}

// ImportBatch holds the records read from a file of another app.
type ImportBatch struct {
	Refuels  []Line[RefuelInput]
	Expenses []Line[ExpenseInput]
	Odometer []Line[OdometerInput]
}

// InvalidLine is a record left out because it does not pass validation.
type InvalidLine struct {
	Line    int    `json:"line"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImportResult struct {
	Refuels    int           `json:"refuels"`
	Expenses   int           `json:"expenses"`
	Odometer   int           `json:"odometer"`
	Duplicates int           `json:"duplicates"` // already in MILE, or twice in the file
	Invalid    []InvalidLine `json:"invalid"`
}

// Import adds the records to the vehicle in one transaction, skipping those
// already present, so that importing the same file twice changes nothing.
// With dryRun nothing is saved, but the result is the same.
func (s *Store) Import(vehicleID int64, b ImportBatch, dryRun bool) (*ImportResult, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res := &ImportResult{Invalid: []InvalidLine{}}

	// Each record is validated, then looked up by its natural key.
	add := func(line int, validate func() error, exists string, args []any, insert func() error) (bool, error) {
		if err := validate(); err != nil {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				return false, err
			}
			res.Invalid = append(res.Invalid, InvalidLine{Line: line, Code: ve.Code, Message: ve.Msg})
			return false, nil
		}
		var found int
		err := tx.QueryRow(`SELECT COUNT(*) FROM `+exists, append([]any{vehicleID}, args...)...).Scan(&found)
		if err != nil {
			return false, err
		}
		if found > 0 {
			res.Duplicates++
			return false, nil
		}
		if err := insert(); err != nil {
			return false, fmt.Errorf("line %d: %w", line, err)
		}
		return true, nil
	}

	for _, l := range b.Refuels {
		in := l.Rec
		ok, err := add(l.Line, in.validate, `refuels WHERE vehicle_id = ? AND date = ? AND odometer = ?`, []any{&in.Date, &in.Odometer},
			func() error { _, err := insertRefuel(tx, vehicleID, in); return err })
		if err != nil {
			return nil, err
		}
		if ok {
			res.Refuels++
		}
	}
	for _, l := range b.Expenses {
		in := l.Rec
		ok, err := add(l.Line, in.validate, `expenses WHERE vehicle_id = ? AND date = ? AND category = ? AND amount_cents = ? AND description = ?`,
			[]any{&in.Date, &in.Category, &in.AmountCents, &in.Description},
			func() error { _, err := insertExpense(tx, vehicleID, in); return err })
		if err != nil {
			return nil, err
		}
		if ok {
			res.Expenses++
		}
	}
	for _, l := range b.Odometer {
		in := l.Rec
		ok, err := add(l.Line, in.validate, `odometer_readings WHERE vehicle_id = ? AND date = ? AND km = ?`, []any{&in.Date, &in.Km},
			func() error {
				_, err := tx.Exec(`INSERT INTO odometer_readings (vehicle_id, date, km, notes, created_at) VALUES (?, ?, ?, ?, ?)`,
					vehicleID, in.Date, in.Km, in.Notes, now())
				return err
			})
		if err != nil {
			return nil, err
		}
		if ok {
			res.Odometer++
		}
	}
	if dryRun {
		return res, nil
	}
	return res, tx.Commit()
}
