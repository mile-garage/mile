package store

import (
	"github.com/mile-garage/mile/internal/deadlines"
	"github.com/mile-garage/mile/internal/fuel"
)

type MonthCost struct {
	Month     string `json:"month"` // YYYY-MM
	Fuel      int64  `json:"fuel"`
	Expenses  int64  `json:"expenses"`
	Insurance int64  `json:"insurance"`
}

type Stats struct {
	Fuel         fuel.Stats       `json:"fuel"`
	ByCategory   map[string]int64 `json:"by_category"` // all time, including "fuel" and "insurance"
	Total        int64            `json:"total"`
	Last12Months []MonthCost      `json:"last_12_months"`
	Km           *int64           `json:"km"`           // driven since the first known odometer value
	CentsPerKm   *float64         `json:"cents_per_km"` // all costs
}

// VehicleStats summarises the costs of a vehicle. Insurance premiums count
// in the month the policy starts.
func (s *Store) VehicleStats(v *Vehicle) (*Stats, error) {
	st := &Stats{ByCategory: map[string]int64{}}

	refuels, err := s.ListRefuels(v.ID)
	if err != nil {
		return nil, err
	}
	fr := make([]fuel.Refuel, len(refuels))
	for i, r := range refuels {
		fr[i] = fuel.Refuel{ID: r.ID, Date: r.Date, Odometer: r.Odometer, Quantity: r.Quantity, TotalCents: r.TotalCents,
			Full: r.FullTank, MissedPrevious: r.MissedPrevious}
	}
	st.Fuel = fuel.Compute(fr)
	if st.Fuel.TotalCents > 0 {
		st.ByCategory["fuel"] = st.Fuel.TotalCents
	}

	rows, err := s.DB.Query(`SELECT category, SUM(amount_cents) FROM expenses WHERE vehicle_id = ? GROUP BY category`, v.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c string
		var n int64
		if err := rows.Scan(&c, &n); err != nil {
			rows.Close()
			return nil, err
		}
		st.ByCategory[c] = n
	}
	rows.Close()
	var ins *int64
	if err := s.DB.QueryRow(`SELECT SUM(premium_cents) FROM policies WHERE vehicle_id = ?`, v.ID).Scan(&ins); err != nil {
		return nil, err
	}
	if ins != nil && *ins > 0 {
		st.ByCategory["insurance"] = *ins
	}
	for _, n := range st.ByCategory {
		st.Total += n
	}

	// last 12 months, oldest first
	today := deadlines.Today(s.Loc)
	months := map[string]*MonthCost{}
	for i := 11; i >= 0; i-- {
		m := deadlines.AddMonths(today, -i).Format("2006-01")
		st.Last12Months = append(st.Last12Months, MonthCost{Month: m})
	}
	for i := range st.Last12Months {
		months[st.Last12Months[i].Month] = &st.Last12Months[i]
	}
	from := st.Last12Months[0].Month + "-01"
	rows, err = s.DB.Query(`
		SELECT 'fuel', substr(date, 1, 7), SUM(total_cents) FROM refuels WHERE vehicle_id = ?1 AND date >= ?2 GROUP BY 2
		UNION ALL SELECT 'expenses', substr(date, 1, 7), SUM(amount_cents) FROM expenses WHERE vehicle_id = ?1 AND date >= ?2 GROUP BY 2
		UNION ALL SELECT 'insurance', substr(start_date, 1, 7), SUM(premium_cents) FROM policies
			WHERE vehicle_id = ?1 AND start_date >= ?2 AND premium_cents IS NOT NULL GROUP BY 2`, v.ID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, month string
		var n int64
		if err := rows.Scan(&kind, &month, &n); err != nil {
			return nil, err
		}
		m := months[month]
		if m == nil {
			continue
		}
		switch kind {
		case "fuel":
			m.Fuel += n
		case "expenses":
			m.Expenses += n
		case "insurance":
			m.Insurance += n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points, err := s.kmPoints(v)
	if err != nil {
		return nil, err
	}
	if len(points) >= 2 {
		lo, hi := points[0].Km, points[0].Km
		for _, p := range points {
			lo, hi = min(lo, p.Km), max(hi, p.Km)
		}
		if km := hi - lo; km > 0 {
			st.Km = &km
			cpk := float64(st.Total) / float64(km)
			st.CentsPerKm = &cpk
		}
	}
	return st, nil
}
