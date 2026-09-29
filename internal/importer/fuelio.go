// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"archive/zip"
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/mile-garage/mile/internal/store"
)

// Fuelio backups (vehicle-N-sync.csv, one per vehicle) are made of sections:
// a line "## Name", a header row, then the data rows. Columns are matched by
// name, since newer versions of the app add some. Numbers always use '.',
// dates are "2006-01-02 15:04" (older files: "2006-01-02"), and values are
// in the units of the vehicle.

type section struct {
	h    header
	rows []row
}

// Built-in cost categories; the names are translated, the ids are not.
// Ids from 32 up are created by the user and matched by name.
var fuelioCategories = map[string]string{
	"1": "service", "2": "maintenance", "4": "road_tax", "5": "parking", "6": "wash",
	"7": "tolls", "8": "fine", "9": "accessories", "31": "other",
}

// Fuelio reads a Fuelio CSV backup, or the .zip that holds it (Google Drive sync).
func Fuelio(data []byte) (*Result, error) {
	data, err := unzipOne(data)
	if err != nil {
		return nil, err
	}
	rows, err := readCSV(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	sections := map[string]*section{}
	var cur *section
	for _, r := range rows {
		if name, ok := sectionName(r.rec); ok {
			cur = &section{}
			sections[name] = cur
			continue
		}
		if cur == nil {
			return nil, notFuelio()
		}
		if cur.h == nil {
			cur.h = newHeader(r.rec)
			continue
		}
		cur.rows = append(cur.rows, r)
	}
	log := sections["log"]
	if log == nil || log.h == nil || !log.h.has("data") {
		return nil, notFuelio()
	}

	u := fuelioUnits(sections["vehicle"], log.h)
	res := &Result{}
	if u.converted() {
		res.Warnings = append(res.Warnings, Warning{Code: "units_converted"})
	}

	stations := map[string]string{}
	if s := sections["favstations"]; s != nil && s.h != nil {
		for _, r := range s.rows {
			stations[s.h.get(r.rec, "stationid")] = s.h.get(r.rec, "namebrand")
		}
	}
	odoCol, _ := log.h.prefix("odo")
	fuelCol, _ := log.h.prefix("fuel (")
	priceCol, _ := log.h.prefix("price")
	cityCol, _ := log.h.prefix("city")
	notesCol, _ := log.h.prefix("notes")
	stationCol, _ := log.h.prefix("stationid")
	tank2 := 0
	for _, r := range log.rows {
		get := func(col string) string { return log.h.get(r.rec, col) }
		if t := get("tanknumber"); t != "" && t != "0" && t != "1" {
			tank2++
			continue
		}
		odo, err1 := number(get(odoCol))
		qty, err2 := number(get(fuelCol))
		price, err3 := optNumber(get(priceCol))
		if err1 != nil || err2 != nil || err3 != nil {
			res.invalid(r.line, strings.Join(r.rec, ","))
			continue
		}
		station := get(cityCol)
		if station == "" {
			station = stations[get(stationCol)]
		}
		res.Batch.Refuels = append(res.Batch.Refuels, store.Line[store.RefuelInput]{Line: r.line, Rec: store.RefuelInput{
			Date:           day(get("data")),
			Odometer:       u.odometer(odo),
			Quantity:       u.volume(qty),
			TotalCents:     cents(price),
			FullTank:       get("full") == "1",
			MissedPrevious: get("missed") == "1",
			Station:        station,
			Notes:          get(notesCol),
		}})
	}
	res.warn("second_tank_skipped", tank2)

	names := map[string]string{}
	if s := sections["costcategories"]; s != nil && s.h != nil {
		for _, r := range s.rows {
			names[s.h.get(r.rec, "costtypeid")] = s.h.get(r.rec, "name")
		}
	}
	income := 0
	if s := sections["costs"]; s != nil && s.h != nil {
		for _, r := range s.rows {
			get := func(col string) string { return s.h.get(r.rec, col) }
			if get("istemplate") == "1" {
				continue // recurring cost template, not a real expense
			}
			if get("isincome") == "1" {
				income++
				continue
			}
			amount, err1 := number(get("cost"))
			odo, err2 := optNumber(get("odo"))
			if err1 != nil || err2 != nil {
				res.invalid(r.line, strings.Join(r.rec, ","))
				continue
			}
			var km *int64
			if odo > 0 {
				km = new(u.odometer(odo))
			}
			id := get("costtypeid")
			res.Batch.Expenses = append(res.Batch.Expenses, store.Line[store.ExpenseInput]{Line: r.line, Rec: store.ExpenseInput{
				Date:        day(get("date")),
				Category:    fuelioCategory(id, names[id]),
				Description: get("costtitle"),
				AmountCents: cents(amount),
				Odometer:    km,
				Notes:       get("notes"),
			}})
		}
	}
	res.warn("income_skipped", income)
	return res, nil
}

func notFuelio() error {
	return fail("import_not_fuelio", "This is not a Fuelio backup: export it from Fuelio › Backup › CSV")
}

// sectionName recognises the lines like "## Log".
func sectionName(rec []string) (string, bool) {
	first := cell(rec[0])
	if !strings.HasPrefix(first, "##") {
		return "", false
	}
	for _, v := range rec[1:] {
		if cell(v) != "" {
			return "", false
		}
	}
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(first, "##"))), true
}

// fuelioUnits reads the units from the vehicle (DistUnit 1 = miles; FuelUnit
// 1 = US gallons, 2 = imperial gallons), or else from the log header, e.g. "Odo (mi)".
func fuelioUnits(v *section, log header) units {
	u := metric
	if v != nil && v.h != nil && len(v.rows) > 0 {
		rec := v.rows[0].rec
		if v.h.get(rec, "distunit") == "1" {
			u.km = kmPerMile
		}
		switch v.h.get(rec, "fuelunit") {
		case "1":
			u.litres = litresUSGal
		case "2":
			u.litres = litresImpGal
		}
		return u
	}
	if _, ok := log.prefix("odo (mi"); ok {
		u.km = kmPerMile
	}
	if col, ok := log.prefix("fuel ("); ok && strings.Contains(col, "gal") {
		u.litres = litresImpGal
		if strings.Contains(col, "us") {
			u.litres = litresUSGal
		}
	}
	return u
}

func fuelioCategory(id, name string) string {
	if c, ok := fuelioCategories[id]; ok {
		return c
	}
	if c, ok := category(name); ok {
		return c
	}
	return "other"
}

// day keeps the date of a "2006-01-02 15:04" timestamp.
func day(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func number(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// optNumber reads an optional number: empty means 0.
func optNumber(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return number(s)
}

// maxUnzipped limits what is read from an archive.
const maxUnzipped = 64 << 20

// unzipOne returns the only CSV file of a .zip archive, or the data itself
// if it is not an archive.
func unzipOne(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return data, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fail("import_bad_zip", "The archive cannot be read")
	}
	var csvs []*zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			csvs = append(csvs, f)
		}
	}
	switch len(csvs) {
	case 0:
		return nil, notFuelio()
	case 1:
	default:
		return nil, fail("import_zip_many", "The archive holds more than one vehicle: extract it and import each vehicle-N-sync.csv into its vehicle")
	}
	rc, err := csvs[0].Open()
	if err != nil {
		return nil, fail("import_bad_zip", "The archive cannot be read")
	}
	defer rc.Close()
	out, err := io.ReadAll(io.LimitReader(rc, maxUnzipped+1))
	if err != nil {
		return nil, fail("import_bad_zip", "The archive cannot be read")
	}
	if len(out) > maxUnzipped {
		return nil, fail("import_too_large", "The file is too large")
	}
	return out, nil
}
