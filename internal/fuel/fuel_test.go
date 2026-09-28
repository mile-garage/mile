package fuel

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestCompute(t *testing.T) {
	st := Compute([]Refuel{
		{Date: "2026-01-01", Odometer: 1000, Quantity: 40, TotalCents: 7000, Full: true},
		{Date: "2026-01-10", Odometer: 1300, Quantity: 10, TotalCents: 1800, Full: false},
		{Date: "2026-01-20", Odometer: 1600, Quantity: 26, TotalCents: 4700, Full: true}, // 36 l / 600 km = 6.0
		{Date: "2026-02-01", Odometer: 2100, Quantity: 25, TotalCents: 4500, Full: true}, // 25 l / 500 km = 5.0
	})
	if len(st.Segments) != 2 {
		t.Fatalf("segments: %+v", st.Segments)
	}
	if !near(st.Segments[0].Per100Km, 6) || !near(st.Segments[1].Per100Km, 5) {
		t.Errorf("per 100 km: %+v", st.Segments)
	}
	if st.AvgPer100Km == nil || !near(*st.AvgPer100Km, 61.0/1100*100) {
		t.Errorf("average: %v", st.AvgPer100Km)
	}
	if !near(*st.LastPer100Km, 5) || st.Km != 1100 || st.TotalCents != 18000 {
		t.Errorf("last=%v km=%d total=%d", *st.LastPer100Km, st.Km, st.TotalCents)
	}
}

func TestMissedRefuelBreaksSegment(t *testing.T) {
	st := Compute([]Refuel{
		{Date: "2026-01-01", Odometer: 1000, Quantity: 40, Full: true},
		{Date: "2026-01-20", Odometer: 1600, Quantity: 30, Full: true, MissedPrevious: true},
		{Date: "2026-02-01", Odometer: 2100, Quantity: 25, Full: true},
	})
	if len(st.Segments) != 1 || st.Segments[0].FromDate != "2026-01-20" {
		t.Errorf("segments: %+v", st.Segments)
	}
}

func TestNoFullTanks(t *testing.T) {
	st := Compute([]Refuel{{Odometer: 100, Quantity: 10}, {Odometer: 300, Quantity: 12}})
	if st.AvgPer100Km != nil || len(st.Segments) != 0 || st.Refuels != 2 {
		t.Errorf("unexpected: %+v", st)
	}
}
