package main

import (
	"math"
	"sort"
	"time"
)

// Planning assumptions for urban pharmaceutical distribution (Jakarta metro).
const (
	roadFactor     = 1.35  // road distance vs straight line
	avgSpeedKmh    = 28.0  // average urban speed
	serviceMinutes = 12.0  // unloading + handover per stop
	dayStartHour   = 8     // trips leave the depot at 08:00
	fuelPricePerL  = 10000 // IDR
	driverPerHour  = 35000 // IDR
	tripFixedCost  = 150000
	maxStops       = 12
	maxRouteHours  = 9.0
)

type point struct{ lat, lng float64 }

func haversineKm(a, b point) float64 {
	const r = 6371.0
	dLat := (b.lat - a.lat) * math.Pi / 180
	dLng := (b.lng - a.lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.lat*math.Pi/180)*math.Cos(b.lat*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

func roadKm(a, b point) float64 { return haversineKm(a, b) * roadFactor }

func tourLength(depot point, pts []point, order []int) float64 {
	if len(order) == 0 {
		return 0
	}
	d := roadKm(depot, pts[order[0]])
	for i := 1; i < len(order); i++ {
		d += roadKm(pts[order[i-1]], pts[order[i]])
	}
	return d + roadKm(pts[order[len(order)-1]], depot)
}

// optimizeOrder returns a visiting order for pts as a closed tour from the depot:
// nearest-neighbour construction improved with 2-opt until no swap shortens the tour.
func optimizeOrder(depot point, pts []point) []int {
	n := len(pts)
	order := make([]int, 0, n)
	used := make([]bool, n)
	cur := depot
	for len(order) < n {
		best, bestD := -1, math.MaxFloat64
		for i, p := range pts {
			if !used[i] {
				if d := haversineKm(cur, p); d < bestD {
					best, bestD = i, d
				}
			}
		}
		used[best] = true
		order = append(order, best)
		cur = pts[best]
	}
	for improved := true; improved; {
		improved = false
		for i := 0; i < n-1; i++ {
			for k := i + 1; k < n; k++ {
				cand := append([]int{}, order[:i]...)
				for j := k; j >= i; j-- {
					cand = append(cand, order[j])
				}
				cand = append(cand, order[k+1:]...)
				if tourLength(depot, pts, cand) < tourLength(depot, pts, order)-1e-9 {
					order, improved = cand, true
				}
			}
		}
	}
	return order
}

type leg struct {
	distanceKm float64
	eta        time.Time
	late       bool
}

// schedule walks the ordered stops from the planned start, waiting for windows that have not
// opened yet, and reports per-stop distances/ETAs plus the total distance and duration.
func schedule(depot point, start time.Time, stops []routeStop) ([]leg, float64, int) {
	legs := make([]leg, len(stops))
	t, prev, total := start, depot, 0.0
	for i, s := range stops {
		d := roadKm(prev, s.point)
		t = t.Add(time.Duration(d / avgSpeedKmh * float64(time.Hour)))
		if open := clock(start, s.windowStart); t.Before(open) {
			t = open
		}
		legs[i] = leg{distanceKm: d, eta: t, late: t.After(clock(start, s.windowEnd))}
		t = t.Add(serviceMinutes * time.Minute)
		total += d
		prev = s.point
	}
	if len(stops) > 0 {
		back := roadKm(prev, depot)
		total += back
		t = t.Add(time.Duration(back / avgSpeedKmh * float64(time.Hour)))
	}
	return legs, total, int(t.Sub(start).Minutes())
}

// clock returns the HH:MM time on the same day as base.
func clock(base time.Time, hhmm string) time.Time {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return base
	}
	return time.Date(base.Year(), base.Month(), base.Day(), t.Hour(), t.Minute(), 0, 0, base.Location())
}

func tripCost(distanceKm float64, durationMin int, kmPerLiter float64) float64 {
	if kmPerLiter <= 0 {
		kmPerLiter = 8
	}
	return math.Round(distanceKm/kmPerLiter*fuelPricePerL + float64(durationMin)/60*driverPerHour + tripFixedCost)
}

// sweepOrder sorts points by polar angle around the depot so that consecutive shipments are
// geographically close — the basis of the sweep clustering used by auto-planning.
func sweepOrder(depot point, pts []point) []int {
	idx := make([]int, len(pts))
	for i := range idx {
		idx[i] = i
	}
	angle := func(p point) float64 { return math.Atan2(p.lat-depot.lat, p.lng-depot.lng) }
	sort.SliceStable(idx, func(a, b int) bool { return angle(pts[idx[a]]) < angle(pts[idx[b]]) })
	return idx
}
