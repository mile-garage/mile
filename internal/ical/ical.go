// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package ical renders deadlines as an iCalendar (RFC 5545) feed that
// calendar apps can subscribe to.
package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/mile-garage/mile/internal/deadlines"
)

var titles = map[string]map[deadlines.Kind]string{
	"it": {
		deadlines.Inspection: "Revisione",
		deadlines.RoadTax:    "Bollo",
		deadlines.Insurance:  "Scadenza assicurazione",
		deadlines.Service:    "Tagliando",
		deadlines.OilChange:  "Cambio olio",
	},
	"en": {
		deadlines.Inspection: "Inspection",
		deadlines.RoadTax:    "Road tax",
		deadlines.Insurance:  "Insurance expiry",
		deadlines.Service:    "Service",
		deadlines.OilChange:  "Oil change",
	},
}

var texts = map[string]map[string]string{
	"it": {"calendar": "Scadenze veicoli", "estimated": "Data stimata dai km percorsi", "km": "Entro %d km", "grace": "Tolleranza fino al %s", "suspended": "Polizza sospesa dal %s"},
	"en": {"calendar": "Vehicle deadlines", "estimated": "Date estimated from the distance driven", "km": "By %d km", "grace": "Grace period until %s", "suspended": "Policy suspended since %s"},
}

// Feed builds the calendar. Deadlines known only by km are left out.
func Feed(ds []deadlines.Deadline, locale string, now time.Time) []byte {
	if locale != "en" {
		locale = "it" // Italian rules: Italian is the default
	}
	tx := texts[locale]
	var b bytes.Buffer
	line := func(s string) { b.WriteString(fold(s)) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//MILE//Vehicle deadlines//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:MILE – " + escape(tx["calendar"]))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT6H")
	line("X-PUBLISHED-TTL:PT6H")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, d := range ds {
		if d.Due == "" {
			continue
		}
		due, err := deadlines.ParseDate(d.Due)
		if err != nil {
			continue
		}
		title := d.Title
		if title == "" {
			title = titles[locale][d.Kind]
		}
		if d.VehicleName != "" {
			title += " – " + d.VehicleName
		}
		var desc []string
		if d.Estimated {
			desc = append(desc, tx["estimated"])
		}
		if d.DueKm != nil {
			desc = append(desc, fmt.Sprintf(tx["km"], *d.DueKm))
		}
		if d.Kind == deadlines.Insurance && d.GraceUntil != "" {
			desc = append(desc, fmt.Sprintf(tx["grace"], d.GraceUntil))
		}
		if d.Since != "" {
			desc = append(desc, fmt.Sprintf(tx["suspended"], d.Since))
		}

		line("BEGIN:VEVENT")
		line(fmt.Sprintf("UID:%s-%d-%d@mile.garage", d.Kind, d.VehicleID, d.RefID))
		line("DTSTAMP:" + stamp)
		line("DTSTART;VALUE=DATE:" + due.Format("20060102"))
		line("DTEND;VALUE=DATE:" + due.AddDate(0, 0, 1).Format("20060102"))
		line("SUMMARY:" + escape(title))
		if len(desc) > 0 {
			line("DESCRIPTION:" + escape(strings.Join(desc, "\n")))
		}
		line("TRANSP:TRANSPARENT")
		for _, before := range []string{"-P7D", "-P1D"} {
			line("BEGIN:VALARM")
			line("ACTION:DISPLAY")
			line("DESCRIPTION:" + escape(title))
			line("TRIGGER:" + before)
			line("END:VALARM")
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.Bytes()
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

// fold splits lines longer than 75 octets, without breaking UTF-8 sequences,
// and terminates them with CRLF.
func fold(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		l := len(string(r))
		if n+l > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += l
	}
	b.WriteString("\r\n")
	return b.String()
}
