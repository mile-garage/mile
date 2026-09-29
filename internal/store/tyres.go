// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"slices"
)

// Tyres: a vehicle has sets of tyres (summer, winter, all-season). Events
// record when a set is fitted (the one fitted before goes to storage) and
// when its front and rear tyres are swapped; the km driven on each set are
// computed from the odometer of the events.

var (
	tyreSeasons = map[string]bool{"summer": true, "winter": true, "all_season": true}
	tyreEvents  = map[string]bool{"mount": true, "rotate": true}
)

type TyreSetInput struct {
	Season  string `json:"season"`
	Brand   string `json:"brand"`
	Model   string `json:"model"`
	Size    string `json:"size"`
	DOT     string `json:"dot"`
	Storage string `json:"storage"`
	Notes   string `json:"notes"`
	Retired bool   `json:"retired"`
}

type TyreSet struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	TyreSetInput
	Mounted         bool    `json:"mounted"`
	MountedSince    *string `json:"mounted_since"` // date of the last fitting
	LastRotation    *string `json:"last_rotation"`
	Km              int64   `json:"km"`                // driven on this set
	KmSinceRotation int64   `json:"km_since_rotation"` // since front and rear were last swapped
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type TyreEventInput struct {
	SetID    int64  `json:"set_id"`
	Kind     string `json:"kind"`
	Date     string `json:"date"`
	Odometer int64  `json:"odometer"`
	Notes    string `json:"notes"`
}

type TyreEvent struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	TyreEventInput
	CreatedAt string `json:"created_at"`
}

type Tyres struct {
	Sets   []TyreSet   `json:"sets"`   // fitted first, retired last
	Events []TyreEvent `json:"events"` // newest first
}

func (in *TyreSetInput) validate() error {
	in.Brand = Clean(in.Brand)
	in.Model = Clean(in.Model)
	in.Size = Clean(in.Size)
	in.DOT = Clean(in.DOT)
	in.Storage = Clean(in.Storage)
	in.Notes = CleanMultiline(in.Notes)
	if !tyreSeasons[in.Season] {
		return invalid("tyre_season_invalid", "Invalid tyre type")
	}
	return nil
}

// Tyres returns the vehicle's sets with the km driven on each, and the history.
func (s *Store) Tyres(v *Vehicle) (*Tyres, error) {
	out := &Tyres{Sets: []TyreSet{}, Events: []TyreEvent{}}
	rows, err := s.DB.Query(`SELECT id, vehicle_id, season, brand, model, size, dot, storage, notes, retired, created_at, updated_at
		FROM tyre_sets WHERE vehicle_id = ? ORDER BY id`, v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t TyreSet
		if err := rows.Scan(&t.ID, &t.VehicleID, &t.Season, &t.Brand, &t.Model, &t.Size, &t.DOT, &t.Storage, &t.Notes, &t.Retired,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out.Sets = append(out.Sets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	rows, err = s.DB.Query(`SELECT id, vehicle_id, set_id, kind, date, odometer, notes, created_at FROM tyre_events
		WHERE vehicle_id = ? ORDER BY date, odometer, id`, v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []TyreEvent
	for rows.Next() {
		var e TyreEvent
		if err := rows.Scan(&e.ID, &e.VehicleID, &e.SetID, &e.Kind, &e.Date, &e.Odometer, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sets := map[int64]*TyreSet{}
	for i := range out.Sets {
		sets[out.Sets[i].ID] = &out.Sets[i]
	}
	// Walk the history: each fitting closes the stretch of the set fitted before.
	var cur *TyreSet
	var from int64
	drive := func(to int64) {
		if cur != nil && to > from {
			cur.Km += to - from
			cur.KmSinceRotation += to - from
		}
		from = max(from, to)
	}
	for _, e := range events {
		t := sets[e.SetID]
		switch e.Kind {
		case "mount":
			drive(e.Odometer)
			if cur != nil {
				cur.Mounted = false
			}
			cur, from = t, e.Odometer
			cur.Mounted, cur.MountedSince = true, &e.Date
		case "rotate":
			if t == cur {
				drive(e.Odometer)
			}
			t.KmSinceRotation, t.LastRotation = 0, &e.Date
		}
	}
	if v.CurrentKm != nil {
		drive(*v.CurrentKm)
	}

	for i := len(events) - 1; i >= 0; i-- {
		out.Events = append(out.Events, events[i])
	}
	rank := func(t TyreSet) int {
		switch {
		case t.Mounted:
			return 0
		case t.Retired:
			return 2
		}
		return 1
	}
	slices.SortStableFunc(out.Sets, func(a, b TyreSet) int { return rank(a) - rank(b) })
	return out, nil
}

// MountedTyres returns the set fitted on the vehicle, or nil.
func (s *Store) MountedTyres(v *Vehicle) (*TyreSet, error) {
	ts, err := s.Tyres(v)
	if err != nil {
		return nil, err
	}
	if len(ts.Sets) > 0 && ts.Sets[0].Mounted {
		return &ts.Sets[0], nil
	}
	return nil, nil
}

func (s *Store) CreateTyreSet(vehicleID int64, in TyreSetInput) (int64, error) {
	if err := in.validate(); err != nil {
		return 0, err
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO tyre_sets (vehicle_id, season, brand, model, size, dot, storage, notes, retired, created_at, updated_at)
		VALUES (`+placeholders(11)+`)`, vehicleID, in.Season, in.Brand, in.Model, in.Size, in.DOT, in.Storage, in.Notes, boolInt(in.Retired), t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateTyreSet saves the set; the set fitted on the vehicle cannot be retired.
func (s *Store) UpdateTyreSet(id int64, in TyreSetInput) error {
	if err := in.validate(); err != nil {
		return err
	}
	if in.Retired {
		mounted, err := s.lastMounted(id)
		if err != nil {
			return err
		}
		if mounted {
			return invalid("tyres_mounted", "Fit another set before retiring this one")
		}
	}
	res, err := s.DB.Exec(`UPDATE tyre_sets SET season = ?, brand = ?, model = ?, size = ?, dot = ?, storage = ?, notes = ?, retired = ?,
		updated_at = ? WHERE id = ?`, in.Season, in.Brand, in.Model, in.Size, in.DOT, in.Storage, in.Notes, boolInt(in.Retired), now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// lastMounted reports whether the set is the last one fitted on its vehicle.
func (s *Store) lastMounted(setID int64) (bool, error) {
	var last int64
	err := s.DB.QueryRow(`SELECT e.set_id FROM tyre_events e JOIN tyre_sets t ON t.vehicle_id = e.vehicle_id
		WHERE t.id = ? AND e.kind = 'mount' ORDER BY e.date DESC, e.odometer DESC, e.id DESC LIMIT 1`, setID).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return last == setID, err
}

func (s *Store) DeleteTyreSet(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM tyre_sets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateTyreEvent(vehicleID int64, in TyreEventInput) (*TyreEvent, error) {
	in.Notes = Clean(in.Notes)
	if !tyreEvents[in.Kind] {
		return nil, invalid("tyre_event_invalid", "Invalid tyre operation")
	}
	if err := checkDate(in.Date, "date"); err != nil {
		return nil, err
	}
	if in.Odometer < 0 {
		return nil, invalid("km_invalid", "Invalid odometer value")
	}
	var retired bool
	err := s.DB.QueryRow(`SELECT retired FROM tyre_sets WHERE id = ? AND vehicle_id = ?`, in.SetID, vehicleID).Scan(&retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if retired {
		return nil, invalid("tyres_retired", "This set of tyres is retired")
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO tyre_events (vehicle_id, set_id, kind, date, odometer, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		vehicleID, in.SetID, in.Kind, in.Date, in.Odometer, in.Notes, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &TyreEvent{ID: id, VehicleID: vehicleID, TyreEventInput: in, CreatedAt: t}, nil
}

func (s *Store) DeleteTyreEvent(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM tyre_events WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
