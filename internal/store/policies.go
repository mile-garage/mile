package store

import (
	"database/sql"
	"errors"

	"github.com/mile-garage/mile/internal/deadlines"
)

type PolicyInput struct {
	Insurer      string `json:"insurer"`
	PolicyNumber string `json:"policy_number"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	PremiumCents *int64 `json:"premium_cents"`
	Notes        string `json:"notes"`
}

type Suspension struct {
	ID        int64   `json:"id"`
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

type Policy struct {
	ID        int64 `json:"id"`
	VehicleID int64 `json:"vehicle_id"`
	PolicyInput
	Suspensions   []Suspension `json:"suspensions"`
	Attachments   []Attachment `json:"attachments"`
	EffectiveEnd  string       `json:"effective_end"` // end date moved by the suspended days
	ExtensionDays int          `json:"extension_days"`
	Suspended     bool         `json:"suspended"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
}

func (in *PolicyInput) validate() error {
	in.Insurer = Clean(in.Insurer)
	in.PolicyNumber = Clean(in.PolicyNumber)
	in.Notes = CleanMultiline(in.Notes)
	if err := checkDate(in.StartDate, "start_date"); err != nil {
		return err
	}
	if err := checkDate(in.EndDate, "end_date"); err != nil {
		return err
	}
	if in.EndDate <= in.StartDate {
		return invalid("policy_dates", "The end date must be after the start date")
	}
	if in.PremiumCents != nil && *in.PremiumCents < 0 {
		return invalid("amount_invalid", "Invalid amount")
	}
	return nil
}

const policyCols = `id, vehicle_id, insurer, policy_number, start_date, end_date, premium_cents, notes, created_at, updated_at`

func scanPolicy(row interface{ Scan(...any) error }) (*Policy, error) {
	var p Policy
	err := row.Scan(&p.ID, &p.VehicleID, &p.Insurer, &p.PolicyNumber, &p.StartDate, &p.EndDate, &p.PremiumCents, &p.Notes,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	p.Suspensions, p.Attachments = []Suspension{}, []Attachment{}
	return &p, err
}

func (s *Store) ListPolicies(vehicleID int64) ([]Policy, error) {
	rows, err := s.DB.Query(`SELECT `+policyCols+` FROM policies WHERE vehicle_id = ? ORDER BY end_date DESC, id DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	att, err := s.attachmentsBy(vehicleID, "policy_id")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillPolicy(&out[i]); err != nil {
			return nil, err
		}
		if a := att[out[i].ID]; a != nil {
			out[i].Attachments = a
		}
	}
	if out == nil {
		out = []Policy{}
	}
	return out, nil
}

func (s *Store) GetPolicy(id int64) (*Policy, error) {
	p, err := scanPolicy(s.DB.QueryRow(`SELECT `+policyCols+` FROM policies WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return p, s.fillPolicy(p)
}

// fillPolicy loads the suspensions and computes the effective end date.
func (s *Store) fillPolicy(p *Policy) error {
	rows, err := s.DB.Query(`SELECT id, start_date, end_date FROM policy_suspensions WHERE policy_id = ? ORDER BY start_date`, p.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var susp []deadlines.Suspension
	for rows.Next() {
		var x Suspension
		if err := rows.Scan(&x.ID, &x.StartDate, &x.EndDate); err != nil {
			return err
		}
		p.Suspensions = append(p.Suspensions, x)
		start, _ := deadlines.ParseDate(x.StartDate)
		ds := deadlines.Suspension{Start: start}
		if x.EndDate != nil {
			e, _ := deadlines.ParseDate(*x.EndDate)
			ds.End = &e
		}
		susp = append(susp, ds)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	end, _ := deadlines.ParseDate(p.EndDate)
	st := deadlines.InsuranceEnd(end, susp, deadlines.Today(s.Loc))
	p.EffectiveEnd = deadlines.Format(st.EffectiveEnd)
	p.ExtensionDays = st.ExtensionDays
	p.Suspended = st.SuspendedSince != nil
	return nil
}

func (s *Store) CreatePolicy(vehicleID int64, in PolicyInput) (*Policy, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO policies (vehicle_id, insurer, policy_number, start_date, end_date, premium_cents, notes,
		created_at, updated_at) VALUES (`+placeholders(9)+`)`,
		vehicleID, in.Insurer, in.PolicyNumber, in.StartDate, in.EndDate, in.PremiumCents, in.Notes, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetPolicy(id)
}

func (s *Store) UpdatePolicy(id int64, in PolicyInput) (*Policy, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`UPDATE policies SET insurer = ?, policy_number = ?, start_date = ?, end_date = ?, premium_cents = ?,
		notes = ?, updated_at = ? WHERE id = ?`,
		in.Insurer, in.PolicyNumber, in.StartDate, in.EndDate, in.PremiumCents, in.Notes, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetPolicy(id)
}

func (s *Store) DeletePolicy(id int64) error {
	return s.deleteWithFiles(`policies`, `policy_id`, id)
}

// Suspend starts a suspension on the given date.
func (s *Store) Suspend(policyID int64, date string) (*Policy, error) {
	p, err := s.GetPolicy(policyID)
	if err != nil {
		return nil, err
	}
	if err := checkDate(date, "date"); err != nil {
		return nil, err
	}
	if p.Suspended {
		return nil, invalid("already_suspended", "The policy is already suspended")
	}
	if date < p.StartDate || date > p.EffectiveEnd {
		return nil, invalid("suspension_outside", "The date must fall within the policy period")
	}
	for _, x := range p.Suspensions {
		if x.EndDate != nil && date < *x.EndDate {
			return nil, invalid("suspension_overlap", "The date overlaps a previous suspension")
		}
	}
	if _, err := s.DB.Exec(`INSERT INTO policy_suspensions (policy_id, start_date) VALUES (?, ?)`, policyID, date); err != nil {
		return nil, err
	}
	return s.GetPolicy(policyID)
}

// Resume ends the suspension in progress on the given date.
func (s *Store) Resume(policyID int64, date string) (*Policy, error) {
	if err := checkDate(date, "date"); err != nil {
		return nil, err
	}
	var id int64
	var start string
	err := s.DB.QueryRow(`SELECT id, start_date FROM policy_suspensions WHERE policy_id = ? AND end_date IS NULL`, policyID).Scan(&id, &start)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, invalid("not_suspended", "The policy is not suspended")
	}
	if err != nil {
		return nil, err
	}
	if date < start {
		return nil, invalid("resume_before_start", "The reactivation date is before the suspension start")
	}
	if _, err := s.DB.Exec(`UPDATE policy_suspensions SET end_date = ? WHERE id = ?`, date, id); err != nil {
		return nil, err
	}
	return s.GetPolicy(policyID)
}

// DeleteSuspension removes a suspension recorded by mistake.
func (s *Store) DeleteSuspension(policyID, suspensionID int64) (*Policy, error) {
	res, err := s.DB.Exec(`DELETE FROM policy_suspensions WHERE id = ? AND policy_id = ?`, suspensionID, policyID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetPolicy(policyID)
}
