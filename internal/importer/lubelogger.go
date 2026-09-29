// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mile-garage/mile/internal/store"
)

// LubeLogger exports one CSV per tab of a vehicle ("Export to CSV"). The
// files carry no units, and dates and amounts are written in the locale of
// the server: "3/7/2024" or "07/03/2024", "$1,234.50" or "1.234,50 €".
// So the user says what the file contains and which units it uses; the date
// order and the decimal separator are detected, or asked when ambiguous.

// Kinds of LubeLogger export.
const (
	LubeGas      = "gas"
	LubeService  = "service"
	LubeRepair   = "repair"
	LubeUpgrade  = "upgrade"
	LubeTax      = "tax"
	LubeOdometer = "odometer"
)

type LubeOptions struct {
	Kind      string
	Units     string // "km" (km and litres, default), "mi_us" (miles, US gallons), "mi_imp" (miles, imperial gallons)
	DateOrder string // "" to detect it, "dmy" or "mdy"
}

// columns each kind of export must have, and must not have (to tell them apart).
var lubeColumns = map[string]struct{ need, not []string }{
	LubeGas:      {[]string{"date", "odometer", "fuelconsumed", "cost"}, nil},
	LubeService:  {[]string{"date", "description", "cost", "odometer"}, []string{"fuelconsumed", "initialodometer", "partnumber"}},
	LubeRepair:   {[]string{"date", "description", "cost", "odometer"}, []string{"fuelconsumed", "initialodometer", "partnumber"}},
	LubeUpgrade:  {[]string{"date", "description", "cost", "odometer"}, []string{"fuelconsumed", "initialodometer", "partnumber"}},
	LubeTax:      {[]string{"date", "description", "cost"}, []string{"odometer", "partnumber"}},
	LubeOdometer: {[]string{"date", "initialodometer", "odometer"}, nil},
}

var lubeCategories = map[string]string{LubeService: "service", LubeRepair: "repair", LubeUpgrade: "accessories", LubeTax: "road_tax"}

func LubeLogger(data []byte, o LubeOptions) (*Result, error) {
	cols, ok := lubeColumns[o.Kind]
	if !ok {
		return nil, fail("import_kind", "Choose what the file contains")
	}
	rows, err := readCSV(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fail("import_empty", "The file is empty")
	}
	h := newHeader(rows[0].rec)
	names := rows[0].rec
	rows = rows[1:]
	if !h.has(cols.need...) {
		return nil, fail("import_wrong_kind", "This file is not a LubeLogger export of the chosen records")
	}
	for _, c := range cols.not {
		if h.has(c) {
			return nil, fail("import_wrong_kind", "This file is not a LubeLogger export of the chosen records")
		}
	}

	u := metric
	switch o.Units {
	case "mi_us":
		u = units{kmPerMile, litresUSGal}
	case "mi_imp":
		u = units{kmPerMile, litresImpGal}
	}
	res := &Result{}
	if u.converted() {
		res.Warnings = append(res.Warnings, Warning{Code: "units_converted"})
	}

	var dates, amounts []string
	for _, r := range rows {
		dates = append(dates, h.get(r.rec, "date"))
		amounts = append(amounts, h.get(r.rec, "cost"), h.get(r.rec, "fuelconsumed"))
	}
	order, err := dateOrder(dates, o.DateOrder)
	if err != nil {
		return nil, err
	}
	dec := decimalSeparator(amounts)

	noOdometer := 0
	for _, r := range rows {
		get := func(col string) string { return h.get(r.rec, col) }
		date, ok := isoDate(get("date"), order)
		if !ok {
			res.invalid(r.line, get("date"))
			continue
		}
		cost, err1 := amount(get("cost"), dec)
		odo, err2 := amount(get("odometer"), dec)
		if err1 != nil || err2 != nil {
			res.invalid(r.line, strings.Join(r.rec, ","))
			continue
		}
		notes := joinNotes(append([]string{get("notes"), tags(get("tags"))}, extraFields(names, r.rec)...)...)
		switch o.Kind {
		case LubeGas:
			qty, err := amount(get("fuelconsumed"), dec)
			if err != nil {
				res.invalid(r.line, get("fuelconsumed"))
				continue
			}
			if odo <= 0 {
				noOdometer++
				continue
			}
			res.Batch.Refuels = append(res.Batch.Refuels, store.Line[store.RefuelInput]{Line: r.line, Rec: store.RefuelInput{
				Date:           date,
				Odometer:       u.odometer(odo),
				Quantity:       u.volume(qty),
				TotalCents:     cents(cost),
				FullTank:       strings.EqualFold(get("isfilltofull"), "true"),
				MissedPrevious: strings.EqualFold(get("missedfuelup"), "true"),
				Notes:          notes,
			}})
		case LubeOdometer:
			if odo <= 0 {
				noOdometer++
				continue
			}
			res.Batch.Odometer = append(res.Batch.Odometer, store.Line[store.OdometerInput]{Line: r.line, Rec: store.OdometerInput{
				Date: date, Km: u.odometer(odo), Notes: notes,
			}})
		default:
			var km *int64
			if odo > 0 {
				km = new(u.odometer(odo))
			}
			res.Batch.Expenses = append(res.Batch.Expenses, store.Line[store.ExpenseInput]{Line: r.line, Rec: store.ExpenseInput{
				Date:        date,
				Category:    lubeCategories[o.Kind],
				Description: get("description"),
				AmountCents: cents(cost),
				Odometer:    km,
				Notes:       notes,
			}})
		}
	}
	res.warn("no_odometer_skipped", noOdometer)
	return res, nil
}

func tags(s string) string {
	if s == "" {
		return ""
	}
	return "Tag: " + s
}

// extraFields turns the extrafield_<Name> columns into "Name: value" lines.
func extraFields(names, rec []string) []string {
	var out []string
	for i, n := range names {
		n = cell(n)
		if !strings.HasPrefix(strings.ToLower(n), "extrafield_") || i >= len(rec) {
			continue
		}
		if v := cell(rec[i]); v != "" {
			out = append(out, n[len("extrafield_"):]+": "+v)
		}
	}
	return out
}

var dateRe = regexp.MustCompile(`^(\d{1,4})\D(\d{1,2})\D(\d{2,4})`)

func dateParts(s string) (a, b, c int, ok bool) {
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	a, _ = strconv.Atoi(m[1])
	b, _ = strconv.Atoi(m[2])
	c, _ = strconv.Atoi(m[3])
	return a, b, c, true
}

// dateOrder tells "dmy" from "mdy": a first part above 12 can only be a day,
// a second part above 12 only a day too. Dates that start with the year
// ("2024-03-07") are read as such whatever the order.
func dateOrder(dates []string, chosen string) (string, error) {
	if chosen == "dmy" || chosen == "mdy" {
		return chosen, nil
	}
	dmy, mdy, ambiguous := false, false, false
	for _, d := range dates {
		a, b, _, ok := dateParts(d)
		switch {
		case !ok || a > 31:
		case a > 12:
			dmy = true
		case b > 12:
			mdy = true
		case a != b:
			ambiguous = true
		}
	}
	switch {
	case dmy && mdy:
		return "", fail("import_date_mixed", "The dates of the file are written in different formats")
	case dmy:
		return "dmy", nil
	case mdy:
		return "mdy", nil
	case ambiguous:
		return "", fail("import_date_ambiguous", "Choose the format of the dates: day/month or month/day")
	}
	return "dmy", nil
}

func isoDate(s, order string) (string, bool) {
	a, b, c, ok := dateParts(s)
	if !ok {
		return "", false
	}
	y, m, d := c, a, b
	switch {
	case a > 31: // year first
		y, m, d = a, b, c
	case order == "dmy":
		m, d = b, a
	}
	if y < 100 {
		y += 2000
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true
}

// cleanNumber keeps digits, separators and sign: "$1,234.50" -> "1,234.50".
func cleanNumber(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == ',' || r == '-' {
			return r
		}
		return -1
	}, s)
}

// decimalSeparator detects the separator of a file from its values: the last
// one when both appear ("1.234,50"), or one not followed by exactly three
// digits ("41,3", "7,9423"). Otherwise it is '.'.
func decimalSeparator(values []string) byte {
	for _, v := range values {
		c := cleanNumber(v)
		i := strings.LastIndexAny(c, ".,")
		if i < 0 {
			continue
		}
		if strings.ContainsAny(c[:i], ".,") && c[:i][strings.LastIndexAny(c[:i], ".,")] != c[i] {
			return c[i]
		}
		if strings.Count(c, string(c[i])) == 1 && len(c)-i-1 != 3 {
			return c[i]
		}
	}
	return '.'
}

// amount reads a number written with the given decimal separator; empty is 0.
func amount(s string, dec byte) (float64, error) {
	c := cleanNumber(s)
	if c == "" {
		if strings.TrimSpace(s) == "" {
			return 0, nil
		}
		return 0, fmt.Errorf("not a number: %q", s)
	}
	thousands := ","
	if dec == ',' {
		thousands = "."
	}
	c = strings.ReplaceAll(c, thousands, "")
	c = strings.Replace(c, ",", ".", 1)
	return strconv.ParseFloat(c, 64)
}
