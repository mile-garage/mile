// Package fuel computes consumption with the full-to-full method: the fuel
// put in after a full tank, up to and including the next full tank, divided by
// the km driven in between.
package fuel

import "sort"

type Refuel struct {
	ID             int64
	Date           string
	Odometer       int64
	Quantity       float64
	TotalCents     int64
	Full           bool
	MissedPrevious bool
}

// Segment is a stretch between two full tanks.
type Segment struct {
	FromDate string  `json:"from_date"`
	ToDate   string  `json:"to_date"`
	Km       int64   `json:"km"`
	Quantity float64 `json:"quantity"`
	Per100Km float64 `json:"per_100km"`
	Cents    int64   `json:"cents"`
}

type Stats struct {
	Refuels       int       `json:"refuels"`
	TotalQuantity float64   `json:"total_quantity"`
	TotalCents    int64     `json:"total_cents"`
	Km            int64     `json:"km"` // between the first and the last refuel
	AvgPer100Km   *float64  `json:"avg_per_100km,omitempty"`
	LastPer100Km  *float64  `json:"last_per_100km,omitempty"`
	CentsPerKm    *float64  `json:"cents_per_km,omitempty"` // fuel only
	AvgUnitPrice  *float64  `json:"avg_unit_price,omitempty"`
	Segments      []Segment `json:"segments"`
}

func Compute(refuels []Refuel) Stats {
	st := Stats{Segments: []Segment{}}
	if len(refuels) == 0 {
		return st
	}
	r := append([]Refuel(nil), refuels...)
	sort.SliceStable(r, func(i, j int) bool {
		if r[i].Odometer == r[j].Odometer {
			return r[i].Date < r[j].Date
		}
		return r[i].Odometer < r[j].Odometer
	})

	for _, x := range r {
		st.Refuels++
		st.TotalQuantity += x.Quantity
		st.TotalCents += x.TotalCents
	}
	st.Km = r[len(r)-1].Odometer - r[0].Odometer
	if st.TotalQuantity > 0 {
		p := float64(st.TotalCents) / 100 / st.TotalQuantity
		st.AvgUnitPrice = &p
	}

	last := -1 // index of the last full tank
	var qty float64
	var cents int64
	var segKm int64
	var segQty float64
	var segCents int64
	for i, x := range r {
		if x.MissedPrevious {
			last = -1 // the stretch before this refuel is incomplete
		}
		if last >= 0 {
			qty += x.Quantity
			cents += x.TotalCents
		}
		if !x.Full {
			continue
		}
		if last >= 0 {
			if km := x.Odometer - r[last].Odometer; km > 0 && qty > 0 {
				st.Segments = append(st.Segments, Segment{
					FromDate: r[last].Date, ToDate: x.Date, Km: km, Quantity: qty,
					Per100Km: qty / float64(km) * 100, Cents: cents,
				})
				segKm += km
				segQty += qty
				segCents += cents
			}
		}
		last, qty, cents = i, 0, 0
	}
	if segKm > 0 {
		avg := segQty / float64(segKm) * 100
		cpk := float64(segCents) / float64(segKm)
		lastSeg := st.Segments[len(st.Segments)-1].Per100Km
		st.AvgPer100Km, st.CentsPerKm, st.LastPer100Km = &avg, &cpk, &lastSeg
	}
	return st
}
