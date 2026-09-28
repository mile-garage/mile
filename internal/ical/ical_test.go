package ical

import (
	"strings"
	"testing"
	"time"

	"github.com/mile-garage/mile/internal/deadlines"
)

func TestFeed(t *testing.T) {
	km := int64(45000)
	out := string(Feed([]deadlines.Deadline{
		{Kind: deadlines.Inspection, VehicleID: 3, VehicleName: "Panda, grigia", Due: "2026-11-30", Status: deadlines.OK},
		{Kind: deadlines.Service, VehicleID: 3, DueKm: &km, Status: deadlines.OK}, // km only: skipped
	}, "it", time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)))

	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"UID:inspection-3-0@mile.garage\r\n",
		"DTSTART;VALUE=DATE:20261130\r\n",
		"DTEND;VALUE=DATE:20261201\r\n",
		`SUMMARY:Revisione – Panda\, grigia`,
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "BEGIN:VEVENT") != 1 {
		t.Error("km-only deadline should be skipped")
	}
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line longer than 75 octets: %q", l)
		}
	}
}

func TestFold(t *testing.T) {
	s := fold(strings.Repeat("è", 60))
	for _, l := range strings.Split(strings.TrimSuffix(s, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line too long: %d", len(l))
		}
	}
}
