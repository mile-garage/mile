package store

import (
	"database/sql"
	"errors"

	"github.com/mile-garage/mile/internal/deadlines"
)

// Reminders are custom deadlines: tied to a vehicle (visible to everyone who
// shares it) or personal (vehicle_id NULL, e.g. the driving licence).

type ReminderInput struct {
	VehicleID    *int64 `json:"vehicle_id"`
	Title        string `json:"title"`
	DueDate      string `json:"due_date"`
	RepeatMonths *int   `json:"repeat_months"`
	Notes        string `json:"notes"`
}

type Reminder struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`
	ReminderInput
	DoneAt    *string `json:"done_at"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func (in *ReminderInput) validate() error {
	in.Title = Clean(in.Title)
	in.Notes = CleanMultiline(in.Notes)
	if in.Title == "" {
		return invalid("title_required", "Enter a title")
	}
	if err := checkDate(in.DueDate, "due_date"); err != nil {
		return err
	}
	if in.RepeatMonths != nil && (*in.RepeatMonths <= 0 || *in.RepeatMonths > 240) {
		in.RepeatMonths = nil
	}
	return nil
}

const reminderCols = `r.id, r.user_id, r.vehicle_id, r.title, r.due_date, r.repeat_months, r.notes, r.done_at, r.created_at, r.updated_at`

// reminderVisible: personal reminders of the user, or reminders of vehicles the user can see.
const reminderVisible = `(r.vehicle_id IS NULL AND r.user_id = ?1 OR r.vehicle_id IN (SELECT vehicle_id FROM vehicle_users WHERE user_id = ?1))`

func scanReminder(row interface{ Scan(...any) error }) (*Reminder, error) {
	var r Reminder
	err := row.Scan(&r.ID, &r.UserID, &r.VehicleID, &r.Title, &r.DueDate, &r.RepeatMonths, &r.Notes, &r.DoneAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

// ListReminders returns the user's reminders, open ones first; vehicleID
// limits them to one vehicle.
func (s *Store) ListReminders(userID int64, vehicleID *int64, includeDone bool) ([]Reminder, error) {
	q := `SELECT ` + reminderCols + ` FROM reminders r WHERE ` + reminderVisible
	args := []any{userID}
	if vehicleID != nil {
		q += ` AND r.vehicle_id = ?2`
		args = append(args, *vehicleID)
	}
	if !includeDone {
		q += ` AND r.done_at IS NULL`
	}
	rows, err := s.DB.Query(q+` ORDER BY r.done_at IS NOT NULL, r.due_date`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// GetReminder returns the reminder if the user can see it, and whether they can edit it.
func (s *Store) GetReminder(userID, id int64) (*Reminder, bool, error) {
	r, err := scanReminder(s.DB.QueryRow(`SELECT `+reminderCols+` FROM reminders r WHERE r.id = ?2 AND `+reminderVisible, userID, id))
	if err != nil {
		return nil, false, err
	}
	if r.VehicleID == nil {
		return r, true, nil
	}
	role, err := s.VehicleRole(userID, *r.VehicleID)
	return r, err == nil && role.Allows(RoleEditor), err
}

func (s *Store) checkReminderVehicle(userID int64, in ReminderInput) error {
	if in.VehicleID == nil {
		return nil
	}
	role, err := s.VehicleRole(userID, *in.VehicleID)
	if err != nil {
		return err
	}
	if !role.Allows(RoleEditor) {
		return ErrForbidden
	}
	return nil
}

func (s *Store) CreateReminder(userID int64, in ReminderInput) (*Reminder, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.checkReminderVehicle(userID, in); err != nil {
		return nil, err
	}
	t := now()
	res, err := s.DB.Exec(`INSERT INTO reminders (user_id, vehicle_id, title, due_date, repeat_months, notes, created_at, updated_at)
		VALUES (`+placeholders(8)+`)`, userID, in.VehicleID, in.Title, in.DueDate, in.RepeatMonths, in.Notes, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	r, _, err := s.GetReminder(userID, id)
	return r, err
}

func (s *Store) UpdateReminder(userID, id int64, in ReminderInput) (*Reminder, error) {
	if err := s.canEditReminder(userID, id); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.checkReminderVehicle(userID, in); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`UPDATE reminders SET vehicle_id = ?, title = ?, due_date = ?, repeat_months = ?, notes = ?, updated_at = ?
		WHERE id = ?`, in.VehicleID, in.Title, in.DueDate, in.RepeatMonths, in.Notes, now(), id); err != nil {
		return nil, err
	}
	r, _, err := s.GetReminder(userID, id)
	return r, err
}

// CompleteReminder marks a reminder as done; a repeating one moves to its next date instead.
func (s *Store) CompleteReminder(userID, id int64) (*Reminder, error) {
	if err := s.canEditReminder(userID, id); err != nil {
		return nil, err
	}
	r, _, err := s.GetReminder(userID, id)
	if err != nil {
		return nil, err
	}
	if r.RepeatMonths != nil {
		due, _ := deadlines.ParseDate(r.DueDate)
		next := deadlines.Format(deadlines.AddMonths(due, *r.RepeatMonths))
		_, err = s.DB.Exec(`UPDATE reminders SET due_date = ?, updated_at = ? WHERE id = ?`, next, now(), id)
	} else {
		_, err = s.DB.Exec(`UPDATE reminders SET done_at = ?, updated_at = ? WHERE id = ?`, now(), now(), id)
	}
	if err != nil {
		return nil, err
	}
	r, _, err = s.GetReminder(userID, id)
	return r, err
}

func (s *Store) DeleteReminder(userID, id int64) error {
	if err := s.canEditReminder(userID, id); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM reminders WHERE id = ?`, id)
	return err
}

func (s *Store) canEditReminder(userID, id int64) error {
	_, canEdit, err := s.GetReminder(userID, id)
	if err != nil {
		return err
	}
	if !canEdit {
		return ErrForbidden
	}
	return nil
}
