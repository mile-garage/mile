// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package deadlines computes when each vehicle obligation falls due, following
// the Italian rules for inspection (revisione), road tax (bollo), insurance
// with suspensions, scheduled service (tagliando) and seasonal tyres.
//
// Every function is pure: dates are civil dates at midnight UTC, "today" is
// passed in by the caller.
package deadlines

import (
	"math"
	"sort"
	"time"
)

const DateLayout = "2006-01-02"

// SoonDays: a deadline within this many days is "due soon".
const SoonDays = 30

// InsuranceGraceDays: after expiry an Italian RCA policy still covers the
// vehicle for 15 days (periodo di tolleranza).
const InsuranceGraceDays = 15

type Kind string

const (
	Inspection   Kind = "inspection"
	RoadTax      Kind = "road_tax"
	Insurance    Kind = "insurance"
	Service      Kind = "service"
	OilChange    Kind = "oil_change"
	TyreChange   Kind = "tyre_change"
	TyreRotation Kind = "tyre_rotation"
	Reminder     Kind = "reminder"
)

type Status string

const (
	Overdue   Status = "overdue"
	Grace     Status = "grace" // insurance expired but within the tolerance period
	DueSoon   Status = "due_soon"
	OK        Status = "ok"
	Suspended Status = "suspended" // insurance suspended: the vehicle must not be driven
)

// Severity orders statuses from the most urgent.
func (s Status) Severity() int {
	switch s {
	case Overdue:
		return 0
	case Grace:
		return 1
	case DueSoon:
		return 2
	case Suspended:
		return 3
	}
	return 4
}

// Deadline is what the UI and the calendar feed show. Text is built by the
// frontend from these fields, so it can be translated.
type Deadline struct {
	Kind        Kind   `json:"kind"`
	VehicleID   int64  `json:"vehicle_id,omitempty"`
	VehicleName string `json:"vehicle_name,omitempty"`
	RefID       int64  `json:"ref_id,omitempty"` // policy, reminder or tyre set
	Title       string `json:"title,omitempty"`  // reminders only
	Season      string `json:"season,omitempty"` // tyre change: the tyres to fit
	Due         string `json:"due,omitempty"`    // empty when only a km limit is known
	DaysLeft    *int   `json:"days_left,omitempty"`
	DueKm       *int64 `json:"due_km,omitempty"`
	KmLeft      *int64 `json:"km_left,omitempty"`
	Estimated   bool   `json:"estimated,omitempty"` // due date estimated from the average km per day
	First       bool   `json:"first,omitempty"`     // inspection: first one, counted from registration
	GraceUntil  string `json:"grace_until,omitempty"`
	Since       string `json:"since,omitempty"` // insurance: suspended since
	Status      Status `json:"status"`
}

func ParseDate(s string) (time.Time, error) { return time.Parse(DateLayout, s) }

func Format(t time.Time) string { return t.Format(DateLayout) }

// Today returns the current civil date in the given location, as midnight UTC.
func Today(loc *time.Location) time.Time {
	n := time.Now().In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// EndOfMonth returns the last day of the month; months outside 1-12 roll over.
func EndOfMonth(y int, m time.Month) time.Time {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
}

// AddMonths adds n months keeping the day, clamped to the last day of the
// target month (31 Jan + 1 month = 28/29 Feb).
func AddMonths(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	d := min(t.Day(), EndOfMonth(first.Year(), first.Month()).Day())
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
}

func days(from, to time.Time) int { return int(math.Round(to.Sub(from).Hours() / 24)) }

func statusFor(due, today time.Time) (Status, int) {
	d := days(today, due)
	switch {
	case d < 0:
		return Overdue, d
	case d <= SoonDays:
		return DueSoon, d
	}
	return OK, d
}

// ---- inspection (revisione) ----

type InspectionRule string

const (
	InspectionStandard InspectionRule = "standard" // cars, motorcycles, light vans: 4 years, then every 2
	InspectionAnnual   InspectionRule = "annual"   // taxis, heavy vehicles, ambulances
	InspectionNone     InspectionRule = "none"
)

// NextInspection: the first inspection is due by the end of the month of
// registration, 4 years later; the following ones by the end of the month of
// the last inspection, 2 years later (1 year for the annual rule).
func NextInspection(rule InspectionRule, registration, last *time.Time) (due time.Time, first, ok bool) {
	if rule == InspectionNone {
		return time.Time{}, false, false
	}
	base, months := last, 24
	if last == nil {
		base, months, first = registration, 48, true
	}
	if base == nil {
		return time.Time{}, false, false
	}
	if rule == InspectionAnnual {
		months = 12
	}
	return EndOfMonth(base.Year(), base.Month()+time.Month(months)), first, true
}

// ---- road tax (bollo) ----

// NextRoadTax: the bollo is paid by the last day of the month following the
// one in which the previous payment expires. lastValidUntil is any day in the
// last month covered by the most recent payment; without it, the next
// payment window for taxMonth (1-12) on or after today is used.
func NextRoadTax(taxMonth int, lastValidUntil *time.Time, today time.Time) (time.Time, bool) {
	if lastValidUntil != nil {
		return EndOfMonth(lastValidUntil.Year(), lastValidUntil.Month()+1), true
	}
	if taxMonth < 1 || taxMonth > 12 {
		return time.Time{}, false
	}
	for y := today.Year() - 1; ; y++ {
		if d := EndOfMonth(y, time.Month(taxMonth)+1); !d.Before(today) {
			return d, true
		}
	}
}

// ---- insurance ----

type Suspension struct {
	Start time.Time
	End   *time.Time // nil: in progress
}

type InsuranceState struct {
	EffectiveEnd   time.Time  // end date moved forward by the suspended days
	GraceUntil     time.Time  // end of the 15-day tolerance period
	SuspendedSince *time.Time // non-nil if a suspension is in progress
	ExtensionDays  int
}

// InsuranceEnd: a suspension pauses the policy, so its expiry moves forward by
// the number of days the vehicle was suspended (reactivation day excluded).
// While a suspension is in progress the expiry keeps moving with today.
func InsuranceEnd(end time.Time, susp []Suspension, today time.Time) InsuranceState {
	var st InsuranceState
	for _, s := range susp {
		if s.End == nil {
			start := s.Start
			st.SuspendedSince = &start
			if today.After(s.Start) {
				st.ExtensionDays += days(s.Start, today)
			}
			continue
		}
		if s.End.After(s.Start) {
			st.ExtensionDays += days(s.Start, *s.End)
		}
	}
	st.EffectiveEnd = end.AddDate(0, 0, st.ExtensionDays)
	st.GraceUntil = st.EffectiveEnd.AddDate(0, 0, InsuranceGraceDays)
	return st
}

// ---- service (tagliando) ----

type ServiceInput struct {
	IntervalKm     int64
	IntervalMonths int
	LastDate       *time.Time // last service, or registration/purchase date
	LastKm         *int64     // odometer at the last service, or initial odometer
	CurrentKm      *int64
	KmPerDay       float64
}

type ServiceDue struct {
	Due       *time.Time // nil if only the km limit is known
	DueKm     *int64
	KmLeft    *int64
	Estimated bool // Due comes from the km limit and the average km per day
}

// NextService: the service is due at whichever comes first between the time
// interval and the km interval; the km limit is turned into a date using the
// average km driven per day.
func NextService(in ServiceInput, today time.Time) (ServiceDue, bool) {
	var out ServiceDue
	if in.LastDate == nil || (in.IntervalKm <= 0 && in.IntervalMonths <= 0) {
		return out, false
	}
	if in.IntervalMonths > 0 {
		d := AddMonths(*in.LastDate, in.IntervalMonths)
		out.Due = &d
	}
	if in.IntervalKm > 0 && in.LastKm != nil {
		dueKm := *in.LastKm + in.IntervalKm
		out.DueKm = &dueKm
		if in.CurrentKm != nil {
			left := dueKm - *in.CurrentKm
			out.KmLeft = &left
			if in.KmPerDay > 0 {
				d := today
				if left > 0 {
					d = today.AddDate(0, 0, int(math.Ceil(float64(left)/in.KmPerDay)))
				}
				if out.Due == nil || d.Before(*out.Due) {
					out.Due, out.Estimated = &d, true
				}
			}
		}
	}
	if out.Due == nil && out.DueKm == nil {
		return out, false
	}
	return out, true
}

// ServiceStatus combines the date and km limits.
func ServiceStatus(s ServiceDue, intervalKm int64, today time.Time) (Status, *int) {
	st, dl := OK, (*int)(nil)
	if s.Due != nil {
		var d int
		st, d = statusFor(*s.Due, today)
		dl = &d
	}
	if s.KmLeft != nil {
		soonKm := max(int64(1000), intervalKm/10)
		switch {
		case *s.KmLeft <= 0:
			st = Overdue
		case *s.KmLeft <= soonKm && st == OK:
			st = DueSoon
		}
	}
	return st, dl
}

// ---- seasonal tyres ----

type TyreSeason string

const (
	Summer    TyreSeason = "summer"
	Winter    TyreSeason = "winter"
	AllSeason TyreSeason = "all_season"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// NextTyreChange: in Italy winter tyres (or chains on board) are required from
// 15 November to 15 April, and may be fitted from 15 October; winter tyres
// with a speed index lower than the one in the registration document must be
// removed by 15 May. All-season tyres need no change.
//
// The change is due by the first deadline after the tyres were fitted (since);
// a missed one stays overdue until the following season starts (16 April for
// summer tyres, 15 October for winter ones), then the next deadline applies.
func NextTyreChange(fitted TyreSeason, since, today time.Time) (due time.Time, fit TyreSeason, ok bool) {
	y := today.Year()
	switch fitted {
	case Summer:
		due, fit = date(y, time.November, 15), Winter
		if today.Before(date(y, time.April, 16)) {
			due = date(y-1, time.November, 15)
		}
	case Winter:
		due, fit = date(y, time.May, 15), Summer
		if !today.Before(date(y, time.October, 15)) {
			due = date(y+1, time.May, 15)
		}
	default:
		return time.Time{}, "", false
	}
	if since.After(due) {
		due = due.AddDate(1, 0, 0)
	}
	return due, fit, true
}

// ---- average km per day ----

type KmPoint struct {
	Date time.Time
	Km   int64
}

// KmPerDay estimates the daily distance from odometer readings, preferring
// the last year; it returns 0 when the data spans less than two weeks.
func KmPerDay(points []KmPoint, today time.Time) float64 {
	if len(points) < 2 {
		return 0
	}
	p := append([]KmPoint(nil), points...)
	sort.Slice(p, func(i, j int) bool {
		if p[i].Date.Equal(p[j].Date) {
			return p[i].Km < p[j].Km
		}
		return p[i].Date.Before(p[j].Date)
	})
	rate := func(p []KmPoint) float64 {
		first, last := p[0], p[len(p)-1]
		span := days(first.Date, last.Date)
		if span < 14 || last.Km <= first.Km {
			return 0
		}
		return float64(last.Km-first.Km) / float64(span)
	}
	yearAgo := today.AddDate(-1, 0, 0)
	i := sort.Search(len(p), func(i int) bool { return !p[i].Date.Before(yearAgo) })
	if len(p)-i >= 2 {
		if r := rate(p[i:]); r > 0 {
			return r
		}
	}
	return rate(p)
}

// ---- building the list ----

func dateDeadline(k Kind, due, today time.Time) Deadline {
	st, d := statusFor(due, today)
	return Deadline{Kind: k, Due: Format(due), DaysLeft: &d, Status: st}
}

func InspectionDeadline(due time.Time, first bool, today time.Time) Deadline {
	d := dateDeadline(Inspection, due, today)
	d.First = first
	return d
}

func RoadTaxDeadline(due, today time.Time) Deadline { return dateDeadline(RoadTax, due, today) }

func InsuranceDeadline(policyID int64, st InsuranceState, today time.Time) Deadline {
	d := dateDeadline(Insurance, st.EffectiveEnd, today)
	d.RefID = policyID
	d.GraceUntil = Format(st.GraceUntil)
	switch {
	case st.SuspendedSince != nil:
		d.Status, d.Since = Suspended, Format(*st.SuspendedSince)
	case d.Status == Overdue && !today.After(st.GraceUntil):
		d.Status = Grace
	}
	return d
}

// ServiceDeadline is used for the service and the oil change, which follow the same rules.
func ServiceDeadline(kind Kind, s ServiceDue, intervalKm int64, today time.Time) Deadline {
	st, dl := ServiceStatus(s, intervalKm, today)
	d := Deadline{Kind: kind, DaysLeft: dl, DueKm: s.DueKm, KmLeft: s.KmLeft, Estimated: s.Estimated, Status: st}
	if s.Due != nil {
		d.Due = Format(*s.Due)
	}
	return d
}

func TyreChangeDeadline(setID int64, fit TyreSeason, due, today time.Time) Deadline {
	d := dateDeadline(TyreChange, due, today)
	d.RefID, d.Season = setID, string(fit)
	return d
}

func ReminderDeadline(id int64, title string, due, today time.Time) Deadline {
	d := dateDeadline(Reminder, due, today)
	d.RefID, d.Title = id, title
	return d
}

// Sort orders deadlines by urgency, then by date (km-only ones last).
func Sort(ds []Deadline) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if sa, sb := a.Status.Severity(), b.Status.Severity(); sa != sb {
			return sa < sb
		}
		if (a.Due == "") != (b.Due == "") {
			return a.Due != ""
		}
		return a.Due < b.Due
	})
}
