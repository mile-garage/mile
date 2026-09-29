// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"os"
	"strings"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

func (r Role) rank() int {
	switch r {
	case RoleOwner:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

// Allows reports whether the role grants at least min.
func (r Role) Allows(min Role) bool { return r.rank() >= min.rank() }

var (
	vehicleKinds = map[string]bool{"car": true, "motorcycle": true, "moped": true, "van": true, "truck": true, "other": true}
	fuelTypes    = map[string]bool{"petrol": true, "diesel": true, "lpg": true, "cng": true, "hybrid": true, "electric": true, "other": true}
	inspRules    = map[string]bool{"standard": true, "annual": true, "none": true}
)

type VehicleInput struct {
	Name                  string   `json:"name"`
	Kind                  string   `json:"kind"`
	Make                  string   `json:"make"`
	Model                 string   `json:"model"`
	Plate                 string   `json:"plate"`
	VIN                   string   `json:"vin"`
	FuelType              string   `json:"fuel_type"`
	RegistrationDate      *string  `json:"registration_date"`
	PurchaseDate          *string  `json:"purchase_date"`
	InitialOdometer       *int64   `json:"initial_odometer"`
	TankCapacity          *float64 `json:"tank_capacity"`
	InspectionRule        string   `json:"inspection_rule"`
	TaxMonth              *int     `json:"tax_month"`
	TaxExemptUntil        *string  `json:"tax_exempt_until"`
	ServiceIntervalKm     *int64   `json:"service_interval_km"`
	ServiceIntervalMonths *int     `json:"service_interval_months"`
	OilIntervalKm         *int64   `json:"oil_interval_km"`
	OilIntervalMonths     *int     `json:"oil_interval_months"`
	TyreRotationKm        *int64   `json:"tyre_rotation_km"`
	Notes                 string   `json:"notes"`
}

type Vehicle struct {
	ID int64 `json:"id"`
	VehicleInput
	CoverID   *int64 `json:"cover_id"`
	Archived  bool   `json:"archived"`
	Role      Role   `json:"role"`
	CurrentKm *int64 `json:"current_km"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (in *VehicleInput) validate() error {
	in.Name = Clean(in.Name)
	in.Make = Clean(in.Make)
	in.Model = Clean(in.Model)
	in.Plate = strings.ToUpper(strings.ReplaceAll(Clean(in.Plate), " ", ""))
	in.VIN = strings.ToUpper(Clean(in.VIN))
	in.Notes = CleanMultiline(in.Notes)
	if in.Name == "" {
		in.Name = Clean(in.Make + " " + in.Model)
	}
	if in.Name == "" {
		return invalid("vehicle_name_required", "Enter a name, or make and model")
	}
	if !vehicleKinds[in.Kind] {
		return invalid("vehicle_kind_invalid", "Invalid vehicle type")
	}
	if !fuelTypes[in.FuelType] {
		return invalid("fuel_type_invalid", "Invalid fuel type")
	}
	if in.InspectionRule == "" {
		in.InspectionRule = "standard"
	}
	if !inspRules[in.InspectionRule] {
		return invalid("inspection_rule_invalid", "Invalid inspection rule")
	}
	if err := checkOptDate(&in.RegistrationDate, "registration_date"); err != nil {
		return err
	}
	if err := checkOptDate(&in.PurchaseDate, "purchase_date"); err != nil {
		return err
	}
	if err := checkOptDate(&in.TaxExemptUntil, "tax_exempt_until"); err != nil {
		return err
	}
	if in.TaxMonth != nil && (*in.TaxMonth < 1 || *in.TaxMonth > 12) {
		return invalid("tax_month_invalid", "Invalid road tax month")
	}
	if in.InitialOdometer != nil && *in.InitialOdometer < 0 {
		return invalid("km_invalid", "Invalid odometer value")
	}
	if in.TankCapacity != nil && *in.TankCapacity <= 0 {
		in.TankCapacity = nil
	}
	if in.ServiceIntervalKm != nil && *in.ServiceIntervalKm <= 0 {
		in.ServiceIntervalKm = nil
	}
	if in.ServiceIntervalMonths != nil && *in.ServiceIntervalMonths <= 0 {
		in.ServiceIntervalMonths = nil
	}
	if in.OilIntervalKm != nil && *in.OilIntervalKm <= 0 {
		in.OilIntervalKm = nil
	}
	if in.TyreRotationKm != nil && *in.TyreRotationKm <= 0 {
		in.TyreRotationKm = nil
	}
	if in.OilIntervalMonths != nil && *in.OilIntervalMonths <= 0 {
		in.OilIntervalMonths = nil
	}
	return nil
}

// currentKm is the highest odometer value known for the vehicle.
const currentKmExpr = `(SELECT MAX(km) FROM (
	SELECT v.initial_odometer AS km
	UNION ALL SELECT MAX(odometer) FROM refuels WHERE vehicle_id = v.id
	UNION ALL SELECT MAX(odometer) FROM expenses WHERE vehicle_id = v.id
	UNION ALL SELECT MAX(km) FROM odometer_readings WHERE vehicle_id = v.id
	UNION ALL SELECT MAX(odometer) FROM tyre_events WHERE vehicle_id = v.id))`

const vehicleCols = `v.id, v.name, v.kind, v.make, v.model, v.plate, v.vin, v.fuel_type, v.registration_date, v.purchase_date,
	v.initial_odometer, v.tank_capacity, v.inspection_rule, v.tax_month, v.tax_exempt_until, v.service_interval_km,
	v.service_interval_months, v.oil_interval_km, v.oil_interval_months, v.tyre_rotation_km, v.notes, v.cover_id, v.archived_at IS NOT NULL, vu.role, ` + currentKmExpr + `, v.created_at, v.updated_at`

func scanVehicle(row interface{ Scan(...any) error }) (*Vehicle, error) {
	var v Vehicle
	err := row.Scan(&v.ID, &v.Name, &v.Kind, &v.Make, &v.Model, &v.Plate, &v.VIN, &v.FuelType, &v.RegistrationDate, &v.PurchaseDate,
		&v.InitialOdometer, &v.TankCapacity, &v.InspectionRule, &v.TaxMonth, &v.TaxExemptUntil, &v.ServiceIntervalKm,
		&v.ServiceIntervalMonths, &v.OilIntervalKm, &v.OilIntervalMonths, &v.TyreRotationKm, &v.Notes, &v.CoverID, &v.Archived, &v.Role, &v.CurrentKm, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

// VehicleRole returns the user's role on the vehicle, or ErrNotFound if the
// user cannot see it (so that the vehicle's existence is not revealed).
func (s *Store) VehicleRole(userID, vehicleID int64) (Role, error) {
	var r Role
	err := s.DB.QueryRow(`SELECT role FROM vehicle_users WHERE vehicle_id = ? AND user_id = ?`, vehicleID, userID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return r, err
}

func (s *Store) ListVehicles(userID int64) ([]Vehicle, error) {
	rows, err := s.DB.Query(`SELECT `+vehicleCols+` FROM vehicles v JOIN vehicle_users vu ON vu.vehicle_id = v.id
		WHERE vu.user_id = ? ORDER BY v.archived_at IS NOT NULL, v.name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vehicle{}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (s *Store) GetVehicle(userID, id int64) (*Vehicle, error) {
	return scanVehicle(s.DB.QueryRow(`SELECT `+vehicleCols+` FROM vehicles v JOIN vehicle_users vu ON vu.vehicle_id = v.id
		WHERE vu.user_id = ? AND v.id = ?`, userID, id))
}

func (s *Store) CreateVehicle(userID int64, in VehicleInput) (*Vehicle, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.Exec(`INSERT INTO vehicles (name, kind, make, model, plate, vin, fuel_type, registration_date, purchase_date,
		initial_odometer, tank_capacity, inspection_rule, tax_month, tax_exempt_until, service_interval_km, service_interval_months,
		oil_interval_km, oil_interval_months, tyre_rotation_km, notes, created_at, updated_at) VALUES (`+placeholders(22)+`)`,
		in.Name, in.Kind, in.Make, in.Model, in.Plate, in.VIN, in.FuelType, in.RegistrationDate, in.PurchaseDate,
		in.InitialOdometer, in.TankCapacity, in.InspectionRule, in.TaxMonth, in.TaxExemptUntil, in.ServiceIntervalKm,
		in.ServiceIntervalMonths, in.OilIntervalKm, in.OilIntervalMonths, in.TyreRotationKm, in.Notes, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO vehicle_users (vehicle_id, user_id, role) VALUES (?, ?, 'owner')`, id, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetVehicle(userID, id)
}

func (s *Store) UpdateVehicle(userID, id int64, in VehicleInput) (*Vehicle, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`UPDATE vehicles SET name = ?, kind = ?, make = ?, model = ?, plate = ?, vin = ?, fuel_type = ?,
		registration_date = ?, purchase_date = ?, initial_odometer = ?, tank_capacity = ?, inspection_rule = ?, tax_month = ?,
		tax_exempt_until = ?, service_interval_km = ?, service_interval_months = ?, oil_interval_km = ?, oil_interval_months = ?, tyre_rotation_km = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		in.Name, in.Kind, in.Make, in.Model, in.Plate, in.VIN, in.FuelType, in.RegistrationDate, in.PurchaseDate,
		in.InitialOdometer, in.TankCapacity, in.InspectionRule, in.TaxMonth, in.TaxExemptUntil, in.ServiceIntervalKm,
		in.ServiceIntervalMonths, in.OilIntervalKm, in.OilIntervalMonths, in.TyreRotationKm, in.Notes, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetVehicle(userID, id)
}

func (s *Store) SetArchived(id int64, archived bool) error {
	var v any
	if archived {
		v = now()
	}
	_, err := s.DB.Exec(`UPDATE vehicles SET archived_at = ?, updated_at = ? WHERE id = ?`, v, now(), id)
	return err
}

// SetCover sets the vehicle's cover photo; nil removes it.
func (s *Store) SetCover(vehicleID int64, attachmentID *int64) error {
	if attachmentID != nil {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM attachments WHERE id = ? AND vehicle_id = ? AND kind = 'photo'`,
			*attachmentID, vehicleID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
	}
	_, err := s.DB.Exec(`UPDATE vehicles SET cover_id = ?, updated_at = ? WHERE id = ?`, attachmentID, now(), vehicleID)
	return err
}

// DeleteVehicle removes the vehicle, all its records and its files.
func (s *Store) DeleteVehicle(id int64) error {
	keys, err := s.storageKeys(`SELECT storage_key FROM attachments WHERE vehicle_id = ?`, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM vehicles WHERE id = ?`, id); err != nil {
		return err
	}
	s.removeFiles(keys)
	return nil
}

// ---- sharing ----

type Member struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
}

func (s *Store) ListMembers(vehicleID int64) ([]Member, error) {
	rows, err := s.DB.Query(`SELECT u.id, u.username, u.display_name, vu.role FROM vehicle_users vu
		JOIN users u ON u.id = vu.user_id WHERE vu.vehicle_id = ? ORDER BY u.username`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMember shares the vehicle with a user or changes their role. A vehicle
// always keeps at least one owner.
func (s *Store) SetMember(vehicleID int64, username string, role Role) error {
	if role.rank() == 0 {
		return invalid("role_invalid", "Invalid role")
	}
	u, err := s.GetUserByName(username)
	if errors.Is(err, ErrNotFound) {
		return invalid("user_not_found", "User not found")
	}
	if err != nil {
		return err
	}
	if role != RoleOwner {
		if err := s.checkNotLastOwner(vehicleID, u.ID); err != nil {
			return err
		}
	}
	_, err = s.DB.Exec(`INSERT INTO vehicle_users (vehicle_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT (vehicle_id, user_id) DO UPDATE SET role = excluded.role`, vehicleID, u.ID, role)
	return err
}

func (s *Store) RemoveMember(vehicleID, userID int64) error {
	if err := s.checkNotLastOwner(vehicleID, userID); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM vehicle_users WHERE vehicle_id = ? AND user_id = ?`, vehicleID, userID)
	return err
}

func (s *Store) checkNotLastOwner(vehicleID, userID int64) error {
	var others int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM vehicle_users WHERE vehicle_id = ? AND role = 'owner' AND user_id != ?`,
		vehicleID, userID).Scan(&others)
	if err != nil {
		return err
	}
	var isOwner int
	s.DB.QueryRow(`SELECT COUNT(*) FROM vehicle_users WHERE vehicle_id = ? AND user_id = ? AND role = 'owner'`, vehicleID, userID).Scan(&isOwner)
	if isOwner > 0 && others == 0 {
		return invalid("last_owner", "The vehicle must keep at least one owner")
	}
	return nil
}

// ---- files ----

func (s *Store) storageKeys(query string, args ...any) ([]string, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) removeFiles(keys []string) {
	for _, k := range keys {
		os.Remove(s.filePath(k))
	}
}
