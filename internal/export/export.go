// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package export writes the data of some vehicles as a ZIP archive with one
// CSV file per kind of record, and optionally the attachments.
package export

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mile-garage/mile/internal/store"
)

type Options struct {
	// Spreadsheet is for Excel and LibreOffice set to Italian: ';' between
	// columns, decimal comma, a byte order mark so that accents show, and
	// names and values in Locale. Otherwise: ',' and '.', keys in English.
	Spreadsheet bool
	Locale      string
	Files       bool // add the attachments
}

// Kinds of value that are formatted differently.
type (
	money int64  // cents
	enum  string // translated in spreadsheets
)

// file names of the tables, translated in Italian spreadsheets.
var fileNames = map[string]string{
	"vehicles": "veicoli", "refuels": "rifornimenti", "expenses": "spese", "odometer": "chilometri",
	"policies": "polizze", "reminders": "promemoria", "tyre_sets": "gomme", "tyre_events": "cambi_gomme",
	"attachments": "allegati",
}

type table struct {
	name string
	cols []string
	rows [][]any
}

func (t *table) add(vals ...any) { t.rows = append(t.rows, vals) }

// Write writes the archive; personal adds the reminders not tied to a vehicle.
func Write(w io.Writer, st *store.Store, userID int64, vehicles []store.Vehicle, personal bool, o Options) error {
	if o.Locale != "en" {
		o.Locale = "it"
	}
	vt := &table{name: "vehicles", cols: []string{"id", "name", "kind", "make", "model", "plate", "vin", "fuel_type",
		"registration_date", "purchase_date", "initial_odometer", "tank_capacity", "inspection_rule", "tax_month",
		"tax_exempt_until", "service_interval_km", "service_interval_months", "oil_interval_km", "oil_interval_months",
		"transmission_oil_interval_km", "transmission_oil_interval_months", "brake_pads_interval_km", "brake_discs_interval_km",
		"tyre_rotation_km", "archived", "notes"}}
	rt := &table{name: "refuels", cols: []string{"vehicle_id", "vehicle", "date", "odometer", "quantity", "total", "unit_price",
		"full_tank", "missed_previous", "station", "notes"}}
	et := &table{name: "expenses", cols: []string{"vehicle_id", "vehicle", "date", "category", "description", "amount",
		"odometer", "vendor", "valid_until", "notes"}}
	ot := &table{name: "odometer", cols: []string{"vehicle_id", "vehicle", "date", "odometer", "notes"}}
	pt := &table{name: "policies", cols: []string{"vehicle_id", "vehicle", "insurer", "policy_number", "start_date", "end_date",
		"effective_end", "premium", "suspensions", "notes"}}
	mt := &table{name: "reminders", cols: []string{"vehicle_id", "vehicle", "title", "due_date", "repeat_months", "done_at", "notes"}}
	st1 := &table{name: "tyre_sets", cols: []string{"vehicle_id", "vehicle", "id", "season", "brand", "model", "size", "dot",
		"storage", "mounted", "km_driven", "retired", "notes"}}
	st2 := &table{name: "tyre_events", cols: []string{"vehicle_id", "vehicle", "date", "event", "tyre_set", "odometer", "notes"}}
	at := &table{name: "attachments", cols: []string{"vehicle_id", "vehicle", "record", "record_date", "file_name", "size_bytes", "file"}}

	type file struct {
		a    store.Attachment
		name string
	}
	var files []file
	attach := func(v *store.Vehicle, record, date string, list []store.Attachment) {
		if !o.Files {
			return
		}
		for _, a := range list {
			name := path.Join("files", Slug(v.Name)+"-"+strconv.FormatInt(v.ID, 10), strconv.FormatInt(a.ID, 10)+"-"+safeName(a.FileName))
			at.add(v.ID, v.Name, enum(record), date, a.FileName, a.Size, name)
			files = append(files, file{a, name})
		}
	}

	names := map[int64]string{}
	for i := range vehicles {
		v := &vehicles[i]
		names[v.ID] = v.Name
		vt.add(v.ID, v.Name, enum(v.Kind), v.Make, v.Model, v.Plate, v.VIN, enum(v.FuelType), v.RegistrationDate, v.PurchaseDate,
			v.InitialOdometer, v.TankCapacity, enum(v.InspectionRule), v.TaxMonth, v.TaxExemptUntil, v.ServiceIntervalKm,
			v.ServiceIntervalMonths, v.OilIntervalKm, v.OilIntervalMonths, v.TransmissionOilKm, v.TransmissionOilMonths, v.BrakePadsKm, v.BrakeDiscsKm, v.TyreRotationKm, v.Archived, v.Notes)

		refuels, err := st.ListRefuels(v.ID)
		if err != nil {
			return err
		}
		for _, r := range refuels {
			var unit any
			if r.UnitPrice != nil {
				unit = math.Round(*r.UnitPrice*1000) / 1000
			}
			rt.add(v.ID, v.Name, r.Date, r.Odometer, r.Quantity, money(r.TotalCents), unit, r.FullTank, r.MissedPrevious, r.Station, r.Notes)
			attach(v, "refuel", r.Date, r.Attachments)
		}
		expenses, err := st.ListExpenses(v.ID)
		if err != nil {
			return err
		}
		for _, e := range expenses {
			et.add(v.ID, v.Name, e.Date, enum(e.Category), e.Description, money(e.AmountCents), e.Odometer, e.Vendor, e.ValidUntil, e.Notes)
			attach(v, "expense", e.Date, e.Attachments)
		}
		readings, err := st.ListOdometer(v.ID)
		if err != nil {
			return err
		}
		for _, r := range readings {
			ot.add(v.ID, v.Name, r.Date, r.Km, r.Notes)
		}
		policies, err := st.ListPolicies(v.ID)
		if err != nil {
			return err
		}
		for _, p := range policies {
			var premium any
			if p.PremiumCents != nil {
				premium = money(*p.PremiumCents)
			}
			pt.add(v.ID, v.Name, p.Insurer, p.PolicyNumber, p.StartDate, p.EndDate, p.EffectiveEnd, premium, suspensions(p.Suspensions), p.Notes)
			attach(v, "policy", p.StartDate, p.Attachments)
		}
		tyres, err := st.Tyres(v)
		if err != nil {
			return err
		}
		sets := map[int64]store.TyreSet{}
		for _, s := range tyres.Sets {
			sets[s.ID] = s
			st1.add(v.ID, v.Name, s.ID, enum(s.Season), s.Brand, s.Model, s.Size, s.DOT, s.Storage, s.Mounted, s.Km, s.Retired, s.Notes)
		}
		for _, e := range tyres.Events {
			s := sets[e.SetID]
			label := strings.TrimSpace(strings.Join([]string{s.Brand, s.Model, s.Size}, " "))
			if label == "" {
				label = "#" + strconv.FormatInt(s.ID, 10)
			}
			st2.add(v.ID, v.Name, e.Date, enum(e.Kind), label, e.Odometer, e.Notes)
		}
		if o.Files {
			photos, err := st.ListPhotos(v.ID)
			if err != nil {
				return err
			}
			for _, p := range photos {
				attach(v, "photo", p.CreatedAt[:10], []store.Attachment{p})
			}
		}
	}

	// Reminders: those of the vehicles, plus the personal ones if asked.
	var vid *int64
	if len(vehicles) == 1 && !personal {
		vid = &vehicles[0].ID
	}
	reminders, err := st.ListReminders(userID, vid, true)
	if err != nil {
		return err
	}
	for _, r := range reminders {
		if r.VehicleID == nil && !personal {
			continue
		}
		if r.VehicleID != nil && names[*r.VehicleID] == "" {
			continue
		}
		var doneAt any
		if r.DoneAt != nil {
			doneAt = (*r.DoneAt)[:10]
		}
		var name string
		if r.VehicleID != nil {
			name = names[*r.VehicleID]
		}
		mt.add(r.VehicleID, name, r.Title, r.DueDate, r.RepeatMonths, doneAt, r.Notes)
	}

	zw := zip.NewWriter(w)
	tables := []*table{vt, rt, et, ot, pt, mt, st1, st2}
	if o.Files {
		tables = append(tables, at)
	}
	for _, t := range tables {
		if err := o.writeTable(zw, t); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := copyFile(zw, st, f.a, f.name); err != nil {
			return err
		}
	}
	return zw.Close()
}

func (o Options) writeTable(zw *zip.Writer, t *table) error {
	name := t.name
	if o.Spreadsheet && o.Locale == "it" {
		name = fileNames[name]
	}
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name + ".csv", Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	cw := csv.NewWriter(f)
	cw.UseCRLF = true
	head := t.cols
	if o.Spreadsheet {
		cw.Comma = ';'
		f.Write([]byte{0xEF, 0xBB, 0xBF}) // byte order mark
		head = make([]string, len(t.cols))
		for i, c := range t.cols {
			head[i] = headers[o.Locale][c]
		}
	}
	cw.Write(head)
	rec := make([]string, len(t.cols))
	for _, r := range t.rows {
		for i, v := range r {
			rec[i] = o.format(v)
		}
		cw.Write(rec)
	}
	cw.Flush()
	return cw.Error()
}

func (o Options) format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return o.text(x)
	case *string:
		if x == nil {
			return ""
		}
		return o.text(*x)
	case int:
		return strconv.Itoa(x)
	case *int:
		if x == nil {
			return ""
		}
		return strconv.Itoa(*x)
	case int64:
		return strconv.FormatInt(x, 10)
	case *int64:
		if x == nil {
			return ""
		}
		return strconv.FormatInt(*x, 10)
	case float64:
		return o.decimal(strconv.FormatFloat(x, 'f', -1, 64))
	case *float64:
		if x == nil {
			return ""
		}
		return o.decimal(strconv.FormatFloat(*x, 'f', -1, 64))
	case money:
		return o.decimal(fmt.Sprintf("%.2f", float64(x)/100))
	case bool:
		return o.label(strconv.FormatBool(x))
	case enum:
		return o.label(string(x))
	}
	panic(fmt.Sprintf("export: cannot format %T", v))
}

func (o Options) decimal(s string) string {
	if o.Spreadsheet {
		return strings.Replace(s, ".", ",", 1)
	}
	return s
}

func (o Options) label(key string) string {
	if o.Spreadsheet {
		if l, ok := values[o.Locale][key]; ok {
			return l
		}
	}
	return key
}

// text keeps spreadsheets from running a value as a formula ("=HYPERLINK(...)").
func (o Options) text(s string) string {
	if o.Spreadsheet && s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// suspensions lists the periods: "2026-02-01 → 2026-03-03; 2026-06-01 →".
func suspensions(list []store.Suspension) string {
	var out []string
	for _, s := range list {
		p := s.StartDate + " →"
		if s.EndDate != nil {
			p += " " + *s.EndDate
		}
		out = append(out, p)
	}
	return strings.Join(out, "; ")
}

func copyFile(zw *zip.Writer, st *store.Store, a store.Attachment, name string) error {
	src, err := st.OpenAttachment(&a)
	if err != nil {
		return fmt.Errorf("attachment %d: %w", a.ID, err)
	}
	defer src.Close()
	// Photos and PDFs are compressed already.
	dst, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

// Slug makes a vehicle name usable as a file name: "Fiat Panda" -> "fiat-panda".
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "vehicle"
	}
	return out
}

// safeName removes path separators and control characters from a file name.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, s)
	if s == "" || s == "." || s == ".." {
		return "file"
	}
	return s
}
