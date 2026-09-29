// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"math"
)

// Records owned by a vehicle: expenses, refuels, odometer readings.
// Update and delete look up the vehicle first, so the caller can check the
// user's role on it (see RecordVehicle).

var expenseCategories = map[string]bool{
	"road_tax": true, "inspection": true, "service": true, "oil_change": true, "transmission_oil": true, "maintenance": true, "repair": true, "tyres": true,
	"parking": true, "tolls": true, "fine": true, "wash": true, "accessories": true, "other": true,
}

// RecordVehicle returns the vehicle of a record in one of the vehicle tables.
func (s *Store) RecordVehicle(table string, id int64) (int64, error) {
	switch table {
	case "expenses", "refuels", "odometer_readings", "policies", "attachments", "tyre_sets", "tyre_events":
	default:
		panic("RecordVehicle: unknown table " + table)
	}
	var v int64
	err := s.DB.QueryRow(`SELECT vehicle_id FROM `+table+` WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return v, err
}

func checkKm(km *int64) error {
	if km != nil && *km < 0 {
		return invalid("km_invalid", "Invalid odometer value")
	}
	return nil
}

// ---- expenses ----

type ExpenseInput struct {
	Date        string  `json:"date"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	AmountCents int64   `json:"amount_cents"`
	Odometer    *int64  `json:"odometer"`
	Vendor      string  `json:"vendor"`
	ValidUntil  *string `json:"valid_until"` // YYYY-MM, road tax only
	Notes       string  `json:"notes"`
}

type Expense struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	ExpenseInput
	Attachments []Attachment `json:"attachments"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

func (in *ExpenseInput) validate() error {
	in.Description = Clean(in.Description)
	in.Vendor = Clean(in.Vendor)
	in.Notes = CleanMultiline(in.Notes)
	if err := checkDate(in.Date, "date"); err != nil {
		return err
	}
	if !expenseCategories[in.Category] {
		return invalid("category_invalid", "Invalid category")
	}
	if in.AmountCents < 0 {
		return invalid("amount_invalid", "Invalid amount")
	}
	if err := checkKm(in.Odometer); err != nil {
		return err
	}
	in.ValidUntil = cleanPtr(in.ValidUntil)
	if in.Category != "road_tax" {
		in.ValidUntil = nil
	}
	if in.ValidUntil != nil && !validMonth(*in.ValidUntil) {
		return invalid("valid_until_invalid", "Invalid month")
	}
	return nil
}

const expenseCols = `id, vehicle_id, date, category, description, amount_cents, odometer, vendor, valid_until, notes, created_at, updated_at`

func scanExpense(row interface{ Scan(...any) error }) (*Expense, error) {
	var e Expense
	err := row.Scan(&e.ID, &e.VehicleID, &e.Date, &e.Category, &e.Description, &e.AmountCents, &e.Odometer, &e.Vendor,
		&e.ValidUntil, &e.Notes, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	e.Attachments = []Attachment{}
	return &e, err
}

func (s *Store) ListExpenses(vehicleID int64) ([]Expense, error) {
	rows, err := s.DB.Query(`SELECT `+expenseCols+` FROM expenses WHERE vehicle_id = ? ORDER BY date DESC, id DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	att, err := s.attachmentsBy(vehicleID, "expense_id")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if a := att[out[i].ID]; a != nil {
			out[i].Attachments = a
		}
	}
	return out, nil
}

func (s *Store) GetExpense(id int64) (*Expense, error) {
	e, err := scanExpense(s.DB.QueryRow(`SELECT `+expenseCols+` FROM expenses WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	att, err := s.attachmentsBy(e.VehicleID, "expense_id")
	if a := att[e.ID]; a != nil {
		e.Attachments = a
	}
	return e, err
}

func (s *Store) CreateExpense(vehicleID int64, in ExpenseInput) (*Expense, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	id, err := insertExpense(s.DB, vehicleID, in)
	if err != nil {
		return nil, err
	}
	return s.GetExpense(id)
}

// insertExpense stores a validated expense.
func insertExpense(q execer, vehicleID int64, in ExpenseInput) (int64, error) {
	t := now()
	res, err := q.Exec(`INSERT INTO expenses (vehicle_id, date, category, description, amount_cents, odometer, vendor,
		valid_until, notes, created_at, updated_at) VALUES (`+placeholders(11)+`)`,
		vehicleID, in.Date, in.Category, in.Description, in.AmountCents, in.Odometer, in.Vendor, in.ValidUntil, in.Notes, t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateExpense(id int64, in ExpenseInput) (*Expense, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`UPDATE expenses SET date = ?, category = ?, description = ?, amount_cents = ?, odometer = ?,
		vendor = ?, valid_until = ?, notes = ?, updated_at = ? WHERE id = ?`,
		in.Date, in.Category, in.Description, in.AmountCents, in.Odometer, in.Vendor, in.ValidUntil, in.Notes, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetExpense(id)
}

func (s *Store) DeleteExpense(id int64) error {
	return s.deleteWithFiles(`expenses`, `expense_id`, id)
}

// deleteWithFiles deletes a record and the files of its attachments.
func (s *Store) deleteWithFiles(table, fk string, id int64) error {
	keys, err := s.storageKeys(`SELECT storage_key FROM attachments WHERE `+fk+` = ?`, id)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.removeFiles(keys)
	return nil
}

// ---- refuels ----

type RefuelInput struct {
	Date           string  `json:"date"`
	Odometer       int64   `json:"odometer"`
	Quantity       float64 `json:"quantity"`
	TotalCents     int64   `json:"total_cents"`
	FullTank       bool    `json:"full_tank"`
	MissedPrevious bool    `json:"missed_previous"`
	Station        string  `json:"station"`
	Notes          string  `json:"notes"`
}

type Refuel struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	RefuelInput
	UnitPrice   *float64     `json:"unit_price"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

func (in *RefuelInput) validate() error {
	in.Station = Clean(in.Station)
	in.Notes = CleanMultiline(in.Notes)
	if err := checkDate(in.Date, "date"); err != nil {
		return err
	}
	if in.Odometer < 0 {
		return invalid("km_invalid", "Invalid odometer value")
	}
	if in.Quantity <= 0 || math.IsNaN(in.Quantity) || in.Quantity > 10000 {
		return invalid("quantity_invalid", "Invalid quantity")
	}
	if in.TotalCents < 0 {
		return invalid("amount_invalid", "Invalid amount")
	}
	return nil
}

const refuelCols = `id, vehicle_id, date, odometer, quantity, total_cents, full_tank, missed_previous, station, notes, created_at, updated_at`

func scanRefuel(row interface{ Scan(...any) error }) (*Refuel, error) {
	var r Refuel
	err := row.Scan(&r.ID, &r.VehicleID, &r.Date, &r.Odometer, &r.Quantity, &r.TotalCents, &r.FullTank, &r.MissedPrevious,
		&r.Station, &r.Notes, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if r.Quantity > 0 {
		p := float64(r.TotalCents) / 100 / r.Quantity
		r.UnitPrice = &p
	}
	r.Attachments = []Attachment{}
	return &r, err
}

func (s *Store) ListRefuels(vehicleID int64) ([]Refuel, error) {
	rows, err := s.DB.Query(`SELECT `+refuelCols+` FROM refuels WHERE vehicle_id = ? ORDER BY date DESC, odometer DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Refuel{}
	for rows.Next() {
		r, err := scanRefuel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	att, err := s.attachmentsBy(vehicleID, "refuel_id")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if a := att[out[i].ID]; a != nil {
			out[i].Attachments = a
		}
	}
	return out, nil
}

func (s *Store) GetRefuel(id int64) (*Refuel, error) {
	return scanRefuel(s.DB.QueryRow(`SELECT `+refuelCols+` FROM refuels WHERE id = ?`, id))
}

func (s *Store) CreateRefuel(vehicleID int64, in RefuelInput) (*Refuel, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	id, err := insertRefuel(s.DB, vehicleID, in)
	if err != nil {
		return nil, err
	}
	return s.GetRefuel(id)
}

// insertRefuel stores a validated refuel.
func insertRefuel(q execer, vehicleID int64, in RefuelInput) (int64, error) {
	t := now()
	res, err := q.Exec(`INSERT INTO refuels (vehicle_id, date, odometer, quantity, total_cents, full_tank, missed_previous,
		station, notes, created_at, updated_at) VALUES (`+placeholders(11)+`)`,
		vehicleID, in.Date, in.Odometer, in.Quantity, in.TotalCents, boolInt(in.FullTank), boolInt(in.MissedPrevious),
		in.Station, in.Notes, t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRefuel(id int64, in RefuelInput) (*Refuel, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`UPDATE refuels SET date = ?, odometer = ?, quantity = ?, total_cents = ?, full_tank = ?,
		missed_previous = ?, station = ?, notes = ?, updated_at = ? WHERE id = ?`,
		in.Date, in.Odometer, in.Quantity, in.TotalCents, boolInt(in.FullTank), boolInt(in.MissedPrevious),
		in.Station, in.Notes, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetRefuel(id)
}

func (s *Store) DeleteRefuel(id int64) error {
	return s.deleteWithFiles(`refuels`, `refuel_id`, id)
}

// ---- odometer readings ----

type OdometerInput struct {
	Date  string `json:"date"`
	Km    int64  `json:"km"`
	Notes string `json:"notes"`
}

type OdometerReading struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	OdometerInput
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListOdometer(vehicleID int64) ([]OdometerReading, error) {
	rows, err := s.DB.Query(`SELECT id, vehicle_id, date, km, notes, created_at FROM odometer_readings
		WHERE vehicle_id = ? ORDER BY date DESC, km DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OdometerReading{}
	for rows.Next() {
		var o OdometerReading
		if err := rows.Scan(&o.ID, &o.VehicleID, &o.Date, &o.Km, &o.Notes, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (in *OdometerInput) validate() error {
	in.Notes = Clean(in.Notes)
	if err := checkDate(in.Date, "date"); err != nil {
		return err
	}
	if in.Km < 0 {
		return invalid("km_invalid", "Invalid odometer value")
	}
	return nil
}

func (s *Store) CreateOdometer(vehicleID int64, in OdometerInput) (*OdometerReading, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO odometer_readings (vehicle_id, date, km, notes, created_at) VALUES (?, ?, ?, ?, ?)`,
		vehicleID, in.Date, in.Km, in.Notes, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &OdometerReading{ID: id, VehicleID: vehicleID, OdometerInput: in, CreatedAt: t}, nil
}

func (s *Store) DeleteOdometer(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM odometer_readings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
