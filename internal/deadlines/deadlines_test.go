package deadlines

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func dp(s string) *time.Time { t := d(s); return &t }

func i64(v int64) *int64 { return &v }

func TestAddMonths(t *testing.T) {
	cases := []struct {
		in, want string
		n        int
	}{
		{"2026-01-31", "2026-02-28", 1},
		{"2024-01-31", "2024-02-29", 1},
		{"2026-03-15", "2027-03-15", 12},
		{"2026-11-30", "2027-05-30", 6},
	}
	for _, c := range cases {
		if got := Format(AddMonths(d(c.in), c.n)); got != c.want {
			t.Errorf("AddMonths(%s, %d) = %s, want %s", c.in, c.n, got, c.want)
		}
	}
}

func TestNextInspection(t *testing.T) {
	cases := []struct {
		name      string
		rule      InspectionRule
		reg, last *time.Time
		want      string
		wantFirst bool
		wantOK    bool
	}{
		{"first after 4 years, end of month", InspectionStandard, dp("2022-05-10"), nil, "2026-05-31", true, true},
		{"then every 2 years from the last one", InspectionStandard, dp("2018-05-10"), dp("2024-11-03"), "2026-11-30", false, true},
		{"annual", InspectionAnnual, dp("2025-02-14"), nil, "2026-02-28", true, true},
		{"annual after last", InspectionAnnual, nil, dp("2025-12-01"), "2026-12-31", false, true},
		{"none", InspectionNone, dp("2020-01-01"), nil, "", false, false},
		{"no data", InspectionStandard, nil, nil, "", false, false},
	}
	for _, c := range cases {
		due, first, ok := NextInspection(c.rule, c.reg, c.last)
		if ok != c.wantOK || first != c.wantFirst || (ok && Format(due) != c.want) {
			t.Errorf("%s: got %s first=%v ok=%v, want %s first=%v ok=%v", c.name, Format(due), first, ok, c.want, c.wantFirst, c.wantOK)
		}
	}
}

func TestNextRoadTax(t *testing.T) {
	cases := []struct {
		name  string
		month int
		last  *time.Time
		today string
		want  string
	}{
		{"from last payment: expires December, pay by end of January", 12, dp("2026-12-01"), "2026-10-01", "2027-01-31"},
		{"from last payment, overdue", 4, dp("2026-04-01"), "2026-09-28", "2026-05-31"},
		{"month only: window still open", 12, nil, "2027-01-15", "2027-01-31"},
		{"month only: next window", 4, nil, "2026-06-01", "2027-05-31"},
		{"month only: this year", 8, nil, "2026-03-01", "2026-09-30"},
	}
	for _, c := range cases {
		got, ok := NextRoadTax(c.month, c.last, d(c.today))
		if !ok || Format(got) != c.want {
			t.Errorf("%s: got %s ok=%v, want %s", c.name, Format(got), ok, c.want)
		}
	}
	if _, ok := NextRoadTax(0, nil, d("2026-01-01")); ok {
		t.Error("no month and no payment should give no deadline")
	}
}

func TestInsuranceEnd(t *testing.T) {
	end := d("2026-06-30")
	st := InsuranceEnd(end, []Suspension{
		{Start: d("2025-11-01"), End: dp("2026-03-01")}, // 120 days
	}, d("2026-04-01"))
	if Format(st.EffectiveEnd) != "2026-10-28" || st.ExtensionDays != 120 || st.SuspendedSince != nil {
		t.Errorf("closed suspension: got %s (+%d)", Format(st.EffectiveEnd), st.ExtensionDays)
	}
	if Format(st.GraceUntil) != "2026-11-12" {
		t.Errorf("grace: got %s", Format(st.GraceUntil))
	}

	st = InsuranceEnd(end, []Suspension{{Start: d("2026-05-01")}}, d("2026-05-11"))
	if st.SuspendedSince == nil || st.ExtensionDays != 10 || Format(st.EffectiveEnd) != "2026-07-10" {
		t.Errorf("open suspension: got %s (+%d)", Format(st.EffectiveEnd), st.ExtensionDays)
	}
}

func TestInsuranceDeadlineStatus(t *testing.T) {
	st := InsuranceEnd(d("2026-09-20"), nil, d("2026-09-28"))
	if got := InsuranceDeadline(1, st, d("2026-09-28")).Status; got != Grace {
		t.Errorf("8 days after expiry: got %s, want grace", got)
	}
	if got := InsuranceDeadline(1, st, d("2026-10-06")).Status; got != Overdue {
		t.Errorf("16 days after expiry: got %s, want overdue", got)
	}
	st = InsuranceEnd(d("2026-09-20"), []Suspension{{Start: d("2026-09-01")}}, d("2026-09-28"))
	if got := InsuranceDeadline(1, st, d("2026-09-28")).Status; got != Suspended {
		t.Errorf("suspended: got %s", got)
	}
}

func TestNextService(t *testing.T) {
	today := d("2026-09-28")
	// 15000 km or 12 months, last at 30000 km on 2026-03-01, now at 40000, 50 km/day:
	// 5000 km left = 100 days = 2027-01-06; the 12 months are 2027-03-01.
	s, ok := NextService(ServiceInput{
		IntervalKm: 15000, IntervalMonths: 12,
		LastDate: dp("2026-03-01"), LastKm: i64(30000),
		CurrentKm: i64(40000), KmPerDay: 50,
	}, today)
	if !ok || Format(*s.Due) != "2027-01-06" || !s.Estimated || *s.DueKm != 45000 || *s.KmLeft != 5000 {
		t.Fatalf("km first: got %+v", s)
	}

	// Time limit first.
	s, _ = NextService(ServiceInput{
		IntervalKm: 15000, IntervalMonths: 12,
		LastDate: dp("2025-10-15"), LastKm: i64(30000),
		CurrentKm: i64(33000), KmPerDay: 10,
	}, today)
	if Format(*s.Due) != "2026-10-15" || s.Estimated {
		t.Fatalf("time first: got %+v", s)
	}
	if st, _ := ServiceStatus(s, 15000, today); st != DueSoon {
		t.Errorf("time first status: %s", st)
	}

	// Km exceeded.
	s, _ = NextService(ServiceInput{
		IntervalKm: 10000, LastDate: dp("2026-01-01"), LastKm: i64(0), CurrentKm: i64(10500),
	}, today)
	if st, _ := ServiceStatus(s, 10000, today); st != Overdue || s.Due != nil {
		t.Errorf("km exceeded: got %s %+v", st, s)
	}

	if _, ok := NextService(ServiceInput{IntervalKm: 10000}, today); ok {
		t.Error("no base date should give no deadline")
	}
}

func TestKmPerDay(t *testing.T) {
	today := d("2026-09-28")
	pts := []KmPoint{
		{d("2020-01-01"), 0},
		{d("2025-09-28"), 50000},
		{d("2026-09-28"), 60950},
	}
	if got := KmPerDay(pts, today); got < 29.9 || got > 30.1 {
		t.Errorf("last year: got %.2f, want 30", got)
	}
	if got := KmPerDay([]KmPoint{{d("2026-09-20"), 100}, {d("2026-09-28"), 900}}, today); got != 0 {
		t.Errorf("under two weeks: got %.2f, want 0", got)
	}
}

func TestSort(t *testing.T) {
	ds := []Deadline{
		{Kind: Service, Status: OK, Due: "2027-01-01"},
		{Kind: Service, Status: DueSoon},
		{Kind: RoadTax, Status: DueSoon, Due: "2026-10-31"},
		{Kind: Insurance, Status: Overdue, Due: "2026-01-01"},
	}
	Sort(ds)
	if ds[0].Kind != Insurance || ds[1].Kind != RoadTax || ds[2].Due != "" || ds[3].Status != OK {
		t.Errorf("unexpected order: %+v", ds)
	}
}
