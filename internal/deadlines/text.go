// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package deadlines

import "fmt"

// Texts used outside the web interface (calendar feed, notifications).

// Locale returns a supported locale: Italian is the default, since the rules are Italian.
func Locale(l string) string {
	if l == "en" {
		return "en"
	}
	return "it"
}

var titles = map[string]map[Kind]string{
	"it": {
		Inspection:             "Revisione",
		RoadTax:                "Bollo",
		Insurance:              "Scadenza assicurazione",
		Service:                "Tagliando",
		OilChange:              "Cambio olio",
		TyreRotation:           "Inversione gomme",
		TyreChange + "_winter": "Montare le gomme invernali",
		TyreChange + "_summer": "Montare le gomme estive",
	},
	"en": {
		Inspection:             "Inspection",
		RoadTax:                "Road tax",
		Insurance:              "Insurance expiry",
		Service:                "Service",
		OilChange:              "Oil change",
		TyreRotation:           "Tyre rotation",
		TyreChange + "_winter": "Fit winter tyres",
		TyreChange + "_summer": "Fit summer tyres",
	},
}

// TitleIn is what the deadline is: "Revisione", or the reminder's title.
func (d Deadline) TitleIn(locale string) string {
	if d.Title != "" {
		return d.Title
	}
	k := d.Kind
	if k == TyreChange {
		k += Kind("_" + d.Season)
	}
	return titles[Locale(locale)][k]
}

// FormatDateIn formats a YYYY-MM-DD date for people: 31/10/2026 or 31 Oct 2026.
func FormatDateIn(s, locale string) string {
	t, err := ParseDate(s)
	if err != nil {
		return s
	}
	if Locale(locale) == "en" {
		return t.Format("2 Jan 2006")
	}
	return t.Format("02/01/2006")
}

// FormatKm formats a distance with thousands separators: 45.000 or 45,000.
func FormatKm(km int64, locale string) string {
	sep := "."
	if Locale(locale) == "en" {
		sep = ","
	}
	s := fmt.Sprint(km)
	neg := km < 0
	if neg {
		s = s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + sep + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}
