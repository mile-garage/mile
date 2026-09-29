// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mile-garage/mile/internal/deadlines"
)

// Deadlines returns the user's upcoming deadlines, most urgent first:
// those of the active vehicles they can see (vehicleID limits them to one)
// plus their reminders.
func (s *Store) Deadlines(userID int64, vehicleID *int64) ([]deadlines.Deadline, error) {
	today := deadlines.Today(s.Loc)
	vehicles, err := s.ListVehicles(userID)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	out := []deadlines.Deadline{}
	for _, v := range vehicles {
		names[v.ID] = v.Name
		if v.Archived || (vehicleID != nil && v.ID != *vehicleID) {
			continue
		}
		ds, err := s.vehicleDeadlines(&v, today)
		if err != nil {
			return nil, err
		}
		for i := range ds {
			ds[i].VehicleID, ds[i].VehicleName = v.ID, v.Name
		}
		out = append(out, ds...)
	}

	reminders, err := s.ListReminders(userID, vehicleID, false)
	if err != nil {
		return nil, err
	}
	for _, r := range reminders {
		due, err := deadlines.ParseDate(r.DueDate)
		if err != nil {
			continue
		}
		d := deadlines.ReminderDeadline(r.ID, r.Title, due, today)
		if r.VehicleID != nil {
			d.VehicleID, d.VehicleName = *r.VehicleID, names[*r.VehicleID]
		}
		out = append(out, d)
	}
	deadlines.Sort(out)
	return out, nil
}

func parseOpt(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := deadlines.ParseDate(*s)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Store) vehicleDeadlines(v *Vehicle, today time.Time) ([]deadlines.Deadline, error) {
	var out []deadlines.Deadline

	// inspection: last one recorded as an expense
	var lastInsp *string
	if err := s.DB.QueryRow(`SELECT MAX(date) FROM expenses WHERE vehicle_id = ? AND category = 'inspection'`, v.ID).Scan(&lastInsp); err != nil {
		return nil, err
	}
	if due, first, ok := deadlines.NextInspection(deadlines.InspectionRule(v.InspectionRule), parseOpt(v.RegistrationDate), parseOpt(lastInsp)); ok {
		out = append(out, deadlines.InspectionDeadline(due, first, today))
	}

	// road tax: last payment's validity, unless exempt
	exempt := parseOpt(v.TaxExemptUntil)
	if exempt == nil || exempt.Before(today) {
		var validUntil *string
		if err := s.DB.QueryRow(`SELECT MAX(valid_until) FROM expenses WHERE vehicle_id = ? AND category = 'road_tax'`, v.ID).Scan(&validUntil); err != nil {
			return nil, err
		}
		var last *time.Time
		if validUntil != nil {
			last = parseOpt(ptr(*validUntil + "-01"))
		}
		month := 0
		if v.TaxMonth != nil {
			month = *v.TaxMonth
		}
		if due, ok := deadlines.NextRoadTax(month, last, today); ok {
			out = append(out, deadlines.RoadTaxDeadline(due, today))
		}
	}

	// insurance: the policy that ends last
	policies, err := s.ListPolicies(v.ID)
	if err != nil {
		return nil, err
	}
	if len(policies) > 0 {
		p := policies[0]
		for _, q := range policies[1:] {
			if q.EffectiveEnd > p.EffectiveEnd {
				p = q
			}
		}
		end, _ := deadlines.ParseDate(p.EndDate)
		var susp []deadlines.Suspension
		for _, x := range p.Suspensions {
			start, _ := deadlines.ParseDate(x.StartDate)
			susp = append(susp, deadlines.Suspension{Start: start, End: parseOpt(x.EndDate)})
		}
		out = append(out, deadlines.InsuranceDeadline(p.ID, deadlines.InsuranceEnd(end, susp, today), today))
	}

	// service, oil change and automatic transmission oil: a service includes the
	// oil change, so it resets that count too; the transmission oil is separate
	for _, x := range []struct {
		kind       deadlines.Kind
		km         *int64
		months     *int
		categories string
	}{
		{deadlines.Service, v.ServiceIntervalKm, v.ServiceIntervalMonths, `'service'`},
		{deadlines.OilChange, v.OilIntervalKm, v.OilIntervalMonths, `'oil_change', 'service'`},
		{deadlines.TransmissionOil, v.TransmissionOilKm, v.TransmissionOilMonths, `'transmission_oil'`},
	} {
		if x.km == nil && x.months == nil {
			continue
		}
		d, ok, err := s.intervalDeadline(v, x.kind, x.km, x.months, x.categories, today)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, d)
		}
	}

	// tyres: the seasonal change, and swapping front and rear of the set fitted
	t, err := s.MountedTyres(v)
	if err != nil || t == nil {
		return out, err
	}
	if since := parseOpt(t.MountedSince); since != nil {
		if due, fit, ok := deadlines.NextTyreChange(deadlines.TyreSeason(t.Season), *since, today); ok {
			out = append(out, deadlines.TyreChangeDeadline(t.ID, fit, due, today))
		}
	}
	if v.TyreRotationKm != nil && v.CurrentKm != nil {
		since := t.MountedSince
		if t.LastRotation != nil && *t.LastRotation > *since {
			since = t.LastRotation
		}
		points, err := s.kmPoints(v)
		if err != nil {
			return nil, err
		}
		in := deadlines.ServiceInput{IntervalKm: *v.TyreRotationKm, LastDate: parseOpt(since), LastKm: ptr(*v.CurrentKm - t.KmSinceRotation),
			CurrentKm: v.CurrentKm, KmPerDay: deadlines.KmPerDay(points, today)}
		if sd, ok := deadlines.NextService(in, today); ok {
			d := deadlines.ServiceDeadline(deadlines.TyreRotation, sd, in.IntervalKm, today)
			d.RefID = t.ID
			out = append(out, d)
		}
	}
	return out, nil
}

// intervalDeadline computes a "every N km or M months" deadline, counted from
// the last expense in the given categories, or from purchase/registration.
func (s *Store) intervalDeadline(v *Vehicle, kind deadlines.Kind, km *int64, months *int, categories string, today time.Time) (deadlines.Deadline, bool, error) {
	in := deadlines.ServiceInput{CurrentKm: v.CurrentKm}
	if km != nil {
		in.IntervalKm = *km
	}
	if months != nil {
		in.IntervalMonths = *months
	}
	var lastDate *string
	var lastKm *int64
	err := s.DB.QueryRow(`SELECT date, odometer FROM expenses WHERE vehicle_id = ? AND category IN (`+categories+`)
		ORDER BY date DESC, id DESC LIMIT 1`, v.ID).Scan(&lastDate, &lastKm)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return deadlines.Deadline{}, false, err
	}
	if lastDate == nil {
		lastDate = v.PurchaseDate
		if lastDate == nil {
			lastDate = v.RegistrationDate
		}
		lastKm = v.InitialOdometer
		if lastKm == nil && v.PurchaseDate == nil {
			lastKm = ptr(int64(0)) // new vehicle: counted from registration
		}
	}
	in.LastDate, in.LastKm = parseOpt(lastDate), lastKm
	points, err := s.kmPoints(v)
	if err != nil {
		return deadlines.Deadline{}, false, err
	}
	in.KmPerDay = deadlines.KmPerDay(points, today)
	sd, ok := deadlines.NextService(in, today)
	if !ok {
		return deadlines.Deadline{}, false, nil
	}
	return deadlines.ServiceDeadline(kind, sd, in.IntervalKm, today), true, nil
}

// kmPoints collects every dated odometer value of the vehicle.
func (s *Store) kmPoints(v *Vehicle) ([]deadlines.KmPoint, error) {
	rows, err := s.DB.Query(`
		SELECT date, odometer FROM refuels WHERE vehicle_id = ?1
		UNION ALL SELECT date, odometer FROM expenses WHERE vehicle_id = ?1 AND odometer IS NOT NULL
		UNION ALL SELECT date, km FROM odometer_readings WHERE vehicle_id = ?1
		UNION ALL SELECT date, odometer FROM tyre_events WHERE vehicle_id = ?1`, v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deadlines.KmPoint
	for rows.Next() {
		var d string
		var km int64
		if err := rows.Scan(&d, &km); err != nil {
			return nil, err
		}
		if t, err := deadlines.ParseDate(d); err == nil {
			out = append(out, deadlines.KmPoint{Date: t, Km: km})
		}
	}
	if v.InitialOdometer != nil {
		base := v.PurchaseDate
		if base == nil {
			base = v.RegistrationDate
		}
		if t := parseOpt(base); t != nil {
			out = append(out, deadlines.KmPoint{Date: *t, Km: *v.InitialOdometer})
		}
	}
	return out, rows.Err()
}

func ptr[T any](v T) *T { return &v }
