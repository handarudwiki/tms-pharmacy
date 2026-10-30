package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

var loc = func() *time.Location {
	l, err := time.LoadLocation(env.Get("TMS_TZ", "Asia/Jakarta"))
	if err != nil {
		return time.Local
	}
	return l
}()

type routeStop struct {
	point
	shipment               *tmsv1.Shipment
	windowStart, windowEnd string
}

func toRouteStops(list []*tmsv1.Shipment) []routeStop {
	out := make([]routeStop, len(list))
	for i, s := range list {
		out[i] = routeStop{point: point{s.Lat, s.Lng}, shipment: s, windowStart: s.WindowStart, windowEnd: s.WindowEnd}
	}
	return out
}

func plannedStart(date string) (time.Time, error) {
	d, err := time.ParseInLocation(ts.DateLayout, date, loc)
	if err != nil {
		return time.Time{}, errs.Invalid("plan date must be YYYY-MM-DD")
	}
	return d.Add(dayStartHour * time.Hour), nil
}

// compliance checks a vehicle/driver pair against the shipments it would carry (BR-01..BR-04).
func compliance(v *tmsv1.Vehicle, d *tmsv1.Driver, date string, list []*tmsv1.Shipment) string {
	switch {
	case v.Status != "AVAILABLE":
		return fmt.Sprintf("vehicle %s is %s", v.Plate, strings.ToLower(v.Status))
	case d.Status != "ACTIVE":
		return fmt.Sprintf("driver %s is %s", d.Name, strings.ToLower(strings.ReplaceAll(d.Status, "_", " ")))
	case d.LicenseExpiry < date:
		return fmt.Sprintf("driver %s's licence expired on %s", d.Name, d.LicenseExpiry)
	}
	var kg, vol float64
	for _, s := range list {
		kg += s.WeightKg
		vol += s.VolumeL
		switch {
		case s.TemperatureClass != v.TemperatureClass && s.TemperatureClass == "COLD":
			return fmt.Sprintf("%s needs a refrigerated vehicle", s.ShipmentNumber)
		case s.TemperatureClass != v.TemperatureClass:
			return fmt.Sprintf("%s is ambient; refrigerated vehicles run dedicated cold routes", s.ShipmentNumber)
		case s.TemperatureClass == "COLD" && !d.ColdChainCertified:
			return fmt.Sprintf("driver %s is not cold-chain certified", d.Name)
		case s.Controlled && !v.ControlledCertified:
			return fmt.Sprintf("vehicle %s is not certified for controlled drugs (%s)", v.Plate, s.ShipmentNumber)
		case s.Controlled && !d.ControlledCertified:
			return fmt.Sprintf("driver %s is not certified for controlled drugs (%s)", d.Name, s.ShipmentNumber)
		}
	}
	if kg > v.CapacityKg {
		return fmt.Sprintf("load %.0f kg exceeds %s capacity %.0f kg", kg, v.Plate, v.CapacityKg)
	}
	if vol > v.CapacityVolumeL {
		return fmt.Sprintf("load %.0f L exceeds %s volume %.0f L", vol, v.Plate, v.CapacityVolumeL)
	}
	return ""
}

// persistTrip optimizes the stop order, stores the trip and marks its shipments PLANNED. If the
// shipment service refuses (e.g. a shipment was planned concurrently) nothing is stored.
func (s *server) persistTrip(ctx context.Context, depot *tmsv1.Depot, v *tmsv1.Vehicle, d *tmsv1.Driver, date string, list []*tmsv1.Shipment) (string, error) {
	start, err := plannedStart(date)
	if err != nil {
		return "", err
	}
	dp := point{depot.Lat, depot.Lng}
	stops := toRouteStops(list)
	pts := make([]point, len(stops))
	for i, st := range stops {
		pts[i] = st.point
	}
	order := optimizeOrder(dp, pts)
	ordered := make([]routeStop, len(order))
	for i, o := range order {
		ordered[i] = stops[o]
	}
	legs, dist, dur := schedule(dp, start, ordered)
	var kg, vol float64
	ids := make([]string, len(list))
	for i, sh := range list {
		kg += sh.WeightKg
		vol += sh.VolumeL
		ids[i] = sh.Id
	}
	util := max(kg/v.CapacityKg, vol/v.CapacityVolumeL) * 100
	a := actor.From(ctx)
	var tripID string
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT nextval('trip_seq')`).Scan(&seq); err != nil {
			return err
		}
		number := fmt.Sprintf("TRP-%s-%03d", strings.ReplaceAll(date[2:], "-", ""), seq)
		if err := tx.QueryRow(ctx, `INSERT INTO trips(org_id, trip_number, depot_id, depot_lat, depot_lng, vehicle_id, vehicle_plate, driver_id,
			driver_name, driver_email, plan_date, distance_km, duration_min, weight_kg, volume_l, capacity_util_pct, estimated_cost,
			temperature_class, planned_start, last_lat, last_lng, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$4,$5,$20) RETURNING id`,
			a.OrgID, number, depot.Id, depot.Lat, depot.Lng, v.Id, v.Plate, d.Id, d.Name, d.UserEmail, date, dist, dur, kg, vol, util,
			tripCost(dist, dur, v.FuelKmPerLiter), v.TemperatureClass, start, a.Name()).Scan(&tripID); err != nil {
			return err
		}
		for i, st := range ordered {
			sh := st.shipment
			if _, err := tx.Exec(ctx, `INSERT INTO stops(trip_id, sequence, shipment_id, shipment_number, consignee_name, address, lat, lng, eta,
				window_start, window_end, distance_from_prev_km, weight_kg, volume_l, packages, temperature_class, controlled)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, tripID, i+1, sh.Id, sh.ShipmentNumber, sh.ConsigneeName,
				sh.Address, sh.Lat, sh.Lng, legs[i].eta, sh.WindowStart, sh.WindowEnd, legs[i].distanceKm, sh.WeightKg, sh.VolumeL,
				sh.Packages, sh.TemperatureClass, sh.Controlled); err != nil {
				return err
			}
		}
		_, err := s.shipments.MarkPlanned(ctx, &tmsv1.MarkPlannedRequest{Ids: ids, TripId: tripID})
		return err
	})
	if err != nil && tripID != "" {
		// The shipments may have been marked PLANNED before the commit failed; release them.
		if _, uerr := s.shipments.Unplan(ctx, &tmsv1.IdsRequest{Ids: ids}); uerr != nil {
			slog.Warn("compensating unplan failed", "trip", tripID, "error", uerr)
		}
	}
	return tripID, errs.DB(err, "trip (vehicle or driver already booked that day)")
}

func (s *server) resources(ctx context.Context, depotID string) (*tmsv1.Depot, []*tmsv1.Vehicle, []*tmsv1.Driver, error) {
	depot, err := s.fleet.GetDepot(ctx, &tmsv1.IdRequest{Id: depotID})
	if err != nil {
		return nil, nil, nil, err
	}
	vs, err := s.fleet.ListVehicles(ctx, &tmsv1.ListVehiclesRequest{DepotId: depotID})
	if err != nil {
		return nil, nil, nil, err
	}
	ds, err := s.fleet.ListDrivers(ctx, &tmsv1.ListDriversRequest{DepotId: depotID})
	if err != nil {
		return nil, nil, nil, err
	}
	return depot, vs.Vehicles, ds.Drivers, nil
}

func (s *server) bookedOn(ctx context.Context, date string) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT vehicle_id::text, driver_id::text FROM trips WHERE org_id=$1 AND plan_date=$2 AND status <> 'CANCELLED'`,
		actor.From(ctx).OrgID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	booked := map[string]bool{}
	for rows.Next() {
		var v, d string
		if err := rows.Scan(&v, &d); err != nil {
			return nil, err
		}
		booked[v], booked[d] = true, true
	}
	return booked, rows.Err()
}

func (s *server) CreateTrip(ctx context.Context, req *tmsv1.CreateTripRequest) (*tmsv1.Trip, error) {
	if req.DepotId == "" || req.VehicleId == "" || req.DriverId == "" || len(req.ShipmentIds) == 0 {
		return nil, errs.Invalid("depot, vehicle, driver and at least one shipment are required")
	}
	if req.PlanDate == "" {
		req.PlanDate = time.Now().In(loc).Format(ts.DateLayout)
	}
	if len(req.ShipmentIds) > maxStops {
		return nil, errs.Invalid("a trip can have at most %d stops", maxStops)
	}
	depot, err := s.fleet.GetDepot(ctx, &tmsv1.IdRequest{Id: req.DepotId})
	if err != nil {
		return nil, err
	}
	v, err := s.fleet.GetVehicle(ctx, &tmsv1.IdRequest{Id: req.VehicleId})
	if err != nil {
		return nil, err
	}
	d, err := s.fleet.GetDriver(ctx, &tmsv1.IdRequest{Id: req.DriverId})
	if err != nil {
		return nil, err
	}
	if v.DepotId != depot.Id || d.DepotId != depot.Id {
		return nil, errs.Invalid("vehicle and driver must belong to the trip's depot")
	}
	booked, err := s.bookedOn(ctx, req.PlanDate)
	if err != nil {
		return nil, errs.DB(err, "trips")
	}
	if booked[v.Id] || booked[d.Id] {
		return nil, errs.Precondition("the vehicle or driver already has a trip on %s", req.PlanDate)
	}
	res, err := s.shipments.ListShipments(ctx, &tmsv1.ListShipmentsRequest{Ids: req.ShipmentIds})
	if err != nil {
		return nil, err
	}
	if len(res.Shipments) != len(req.ShipmentIds) {
		return nil, errs.NotFound("some shipments were not found")
	}
	for _, sh := range res.Shipments {
		if sh.Status != "PENDING" {
			return nil, errs.Precondition("shipment %s is %s", sh.ShipmentNumber, sh.Status)
		}
		if sh.DepotId != depot.Id {
			return nil, errs.Invalid("shipment %s ships from another depot", sh.ShipmentNumber)
		}
	}
	if msg := compliance(v, d, req.PlanDate, res.Shipments); msg != "" {
		return nil, errs.Precondition("%s", msg)
	}
	id, err := s.persistTrip(ctx, depot, v, d, req.PlanDate, res.Shipments)
	if err != nil {
		return nil, err
	}
	return s.GetTrip(ctx, &tmsv1.IdRequest{Id: id})
}

// AutoPlan builds trips for a depot and date from the pending pool (FR-05). Shipments are swept
// by polar angle around the depot so each vehicle gets a compact area, vehicles are filled
// largest-first, and every assignment passes the same compliance checks as manual planning.
// Shipments that cannot be placed are reported with the reason.
func (s *server) AutoPlan(ctx context.Context, req *tmsv1.AutoPlanRequest) (*tmsv1.AutoPlanResponse, error) {
	if req.DepotId == "" {
		return nil, errs.Invalid("depot is required")
	}
	if req.PlanDate == "" {
		req.PlanDate = time.Now().In(loc).Format(ts.DateLayout)
	}
	day, err := time.Parse(ts.DateLayout, req.PlanDate)
	if err != nil {
		return nil, errs.Invalid("plan date must be YYYY-MM-DD")
	}
	depot, vehicles, drivers, err := s.resources(ctx, req.DepotId)
	if err != nil {
		return nil, err
	}
	pool, err := s.shipments.ListShipments(ctx, &tmsv1.ListShipmentsRequest{DepotId: req.DepotId, Status: "PENDING",
		RequiredBy: day.AddDate(0, 0, 1).Format(ts.DateLayout)})
	if err != nil {
		return nil, err
	}
	booked, err := s.bookedOn(ctx, req.PlanDate)
	if err != nil {
		return nil, errs.DB(err, "trips")
	}
	out := &tmsv1.AutoPlanResponse{}
	if len(pool.Shipments) == 0 {
		return out, nil
	}
	dp := point{depot.Lat, depot.Lng}
	pts := make([]point, len(pool.Shipments))
	for i, sh := range pool.Shipments {
		pts[i] = point{sh.Lat, sh.Lng}
	}
	var remaining []*tmsv1.Shipment
	for _, i := range sweepOrder(dp, pts) {
		remaining = append(remaining, pool.Shipments[i])
	}
	// Urgent shipments are placed first within the sweep.
	sort.SliceStable(remaining, func(a, b int) bool { return remaining[a].Priority == 1 && remaining[b].Priority != 1 })

	sort.SliceStable(vehicles, func(a, b int) bool { return vehicles[a].CapacityKg < vehicles[b].CapacityKg })
	tried := map[string]bool{}
	for len(remaining) > 0 {
		// Right-size: take the smallest free vehicle that can carry the next route's expected
		// load (the first maxStops compatible shipments in sweep order), else the largest one.
		var v *tmsv1.Vehicle
		for _, cand := range vehicles {
			if cand.Status != "AVAILABLE" || booked[cand.Id] || tried[cand.Id] {
				continue
			}
			target, n := 0.0, 0
			for _, sh := range remaining {
				if sh.TemperatureClass == cand.TemperatureClass && n < maxStops {
					target += sh.WeightKg
					n++
				}
			}
			if n == 0 {
				continue
			}
			if v == nil || (cand.CapacityKg >= target && (v.CapacityKg < target || cand.CapacityKg < v.CapacityKg)) ||
				(v.CapacityKg < target && cand.CapacityKg > v.CapacityKg) {
				v = cand
			}
		}
		if v == nil {
			break
		}
		tried[v.Id] = true
		// Prefer the most qualified free driver so controlled/cold loads stay possible.
		var driver *tmsv1.Driver
		for _, d := range drivers {
			if booked[d.Id] || d.Status != "ACTIVE" || d.LicenseExpiry < req.PlanDate {
				continue
			}
			if v.TemperatureClass == "COLD" && !d.ColdChainCertified {
				continue
			}
			if driver == nil || (d.ControlledCertified && v.ControlledCertified && !driver.ControlledCertified) {
				driver = d
			}
		}
		if driver == nil {
			continue
		}
		var load, rest []*tmsv1.Shipment
		for _, sh := range remaining {
			if len(load) < maxStops && compliance(v, driver, req.PlanDate, append(append([]*tmsv1.Shipment{}, load...), sh)) == "" {
				if _, _, dur := schedule(dp, time.Now(), toRouteStops(append(append([]*tmsv1.Shipment{}, load...), sh))); float64(dur)/60 <= maxRouteHours {
					load = append(load, sh)
					continue
				}
			}
			rest = append(rest, sh)
		}
		if len(load) == 0 {
			continue
		}
		id, err := s.persistTrip(ctx, depot, v, driver, req.PlanDate, load)
		if err != nil {
			return nil, err
		}
		booked[v.Id], booked[driver.Id] = true, true
		t, err := s.GetTrip(ctx, &tmsv1.IdRequest{Id: id})
		if err != nil {
			return nil, err
		}
		out.Trips = append(out.Trips, t)
		remaining = rest
	}
	for _, sh := range remaining {
		out.Unplanned = append(out.Unplanned, &tmsv1.Unplanned{ShipmentId: sh.Id, ShipmentNumber: sh.ShipmentNumber, Reason: unplannedReason(sh, vehicles, drivers, booked, req.PlanDate)})
	}
	return out, nil
}

func unplannedReason(sh *tmsv1.Shipment, vehicles []*tmsv1.Vehicle, drivers []*tmsv1.Driver, booked map[string]bool, date string) string {
	free := func(v *tmsv1.Vehicle) bool { return v.Status == "AVAILABLE" && !booked[v.Id] }
	hasVehicle, hasCertified := false, false
	for _, v := range vehicles {
		if v.TemperatureClass == sh.TemperatureClass && v.CapacityKg >= sh.WeightKg {
			hasVehicle = true
			if !sh.Controlled || v.ControlledCertified {
				hasCertified = true
			}
		}
	}
	freeAny := false
	for _, v := range vehicles {
		if free(v) && v.TemperatureClass == sh.TemperatureClass {
			freeAny = true
		}
	}
	switch {
	case !hasVehicle && sh.TemperatureClass == "COLD":
		return "no refrigerated vehicle large enough"
	case !hasVehicle:
		return "no vehicle large enough for this shipment"
	case !hasCertified:
		return "no vehicle certified for controlled drugs"
	case !freeAny:
		return "all compatible vehicles are fully booked — consider tendering to a carrier"
	}
	return "remaining capacity, route duration or certified drivers exhausted — consider tendering to a carrier"
}

// OptimizeTrip re-sequences the stops of a planned trip.
func (s *server) OptimizeTrip(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Trip, error) {
	t, err := s.GetTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if t.Status != "PLANNED" {
		return nil, errs.Precondition("only planned trips can be re-optimized")
	}
	v, err := s.fleet.GetVehicle(ctx, &tmsv1.IdRequest{Id: t.VehicleId})
	if err != nil {
		return nil, err
	}
	dp := point{t.DepotLat, t.DepotLng}
	stops := make([]routeStop, len(t.Stops))
	pts := make([]point, len(t.Stops))
	for i, st := range t.Stops {
		// The stop id rides in the shipment slot so the new order can be written back per stop.
		stops[i] = routeStop{point: point{st.Lat, st.Lng}, shipment: &tmsv1.Shipment{Id: st.Id}, windowStart: st.WindowStart, windowEnd: st.WindowEnd}
		pts[i] = stops[i].point
	}
	order := optimizeOrder(dp, pts)
	ordered := make([]routeStop, len(order))
	for i, o := range order {
		ordered[i] = stops[o]
	}
	start, _ := plannedStart(t.PlanDate)
	legs, dist, dur := schedule(dp, start, ordered)
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		for i, st := range ordered {
			if _, err := tx.Exec(ctx, `UPDATE stops SET sequence=$2, eta=$3, distance_from_prev_km=$4 WHERE id=$1`,
				st.shipment.Id, i+1, legs[i].eta, legs[i].distanceKm); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE trips SET distance_km=$2, duration_min=$3, estimated_cost=$4 WHERE id=$1`,
			t.Id, dist, dur, tripCost(dist, dur, v.FuelKmPerLiter))
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "trip")
	}
	return s.GetTrip(ctx, req)
}

// --- reads ---

const tripColumns = `id, trip_number, depot_id, vehicle_id, vehicle_plate, driver_id, driver_name, driver_email, plan_date, status,
	distance_km, duration_min, weight_kg, volume_l, capacity_util_pct, estimated_cost, temperature_class, planned_start, started_at,
	completed_at, created_by, created_at, last_lat, last_lng, last_temperature_c, last_ping_at, depot_lat, depot_lng`

func scanTrip(row pgx.Row) (*tmsv1.Trip, error) {
	var t tmsv1.Trip
	var date time.Time
	var planned, created time.Time
	var started, completed, ping *time.Time
	var temp *float64
	err := row.Scan(&t.Id, &t.TripNumber, &t.DepotId, &t.VehicleId, &t.VehiclePlate, &t.DriverId, &t.DriverName, &t.DriverEmail, &date,
		&t.Status, &t.DistanceKm, &t.DurationMin, &t.WeightKg, &t.VolumeL, &t.CapacityUtilPct, &t.EstimatedCost, &t.TemperatureClass,
		&planned, &started, &completed, &t.CreatedBy, &created, &t.LastLat, &t.LastLng, &temp, &ping, &t.DepotLat, &t.DepotLng)
	t.PlanDate, t.PlannedStart, t.StartedAt, t.CompletedAt = date.Format(ts.DateLayout), ts.Time(planned), ts.PtrTime(started), ts.PtrTime(completed)
	t.CreatedAt, t.LastPingAt = ts.Time(created), ts.PtrTime(ping)
	if temp != nil {
		t.LastTemperatureC = *temp
	}
	return &t, err
}

func (s *server) loadStops(ctx context.Context, trips []*tmsv1.Trip) error {
	if len(trips) == 0 {
		return nil
	}
	byID := map[string]*tmsv1.Trip{}
	ids := make([]string, len(trips))
	for i, t := range trips {
		byID[t.Id], ids[i] = t, t.Id
	}
	rows, err := s.db.Query(ctx, `SELECT trip_id, id, sequence, shipment_id, shipment_number, consignee_name, address, lat, lng, eta,
		window_start, window_end, distance_from_prev_km, status, arrived_at, completed_at, weight_kg, packages, temperature_class,
		controlled, failure_reason FROM stops WHERE trip_id::text = ANY($1) ORDER BY sequence`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var st tmsv1.Stop
		var eta, arrived, completed *time.Time
		if err := rows.Scan(&tid, &st.Id, &st.Sequence, &st.ShipmentId, &st.ShipmentNumber, &st.ConsigneeName, &st.Address, &st.Lat, &st.Lng,
			&eta, &st.WindowStart, &st.WindowEnd, &st.DistanceFromPrevKm, &st.Status, &arrived, &completed, &st.WeightKg, &st.Packages,
			&st.TemperatureClass, &st.Controlled, &st.FailureReason); err != nil {
			return err
		}
		st.Eta, st.ArrivedAt, st.CompletedAt = ts.PtrTime(eta), ts.PtrTime(arrived), ts.PtrTime(completed)
		if eta != nil {
			st.EtaOutsideWindow = eta.In(loc).Format("15:04") > st.WindowEnd
		}
		t := byID[tid]
		t.Stops = append(t.Stops, &st)
		switch st.Status {
		case "DELIVERED":
			t.DeliveredStops++
		case "FAILED":
			t.FailedStops++
		}
	}
	return rows.Err()
}

func (s *server) GetTrip(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Trip, error) {
	t, err := scanTrip(s.db.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	if err != nil {
		return nil, errs.DB(err, "trip")
	}
	return t, errs.DB(s.loadStops(ctx, []*tmsv1.Trip{t}), "stops")
}

func (s *server) ListTrips(ctx context.Context, req *tmsv1.ListTripsRequest) (*tmsv1.ListTripsResponse, error) {
	statuses := req.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `SELECT `+tripColumns+` FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2)
		AND ($3='' OR plan_date=$3::date) AND ($4='' OR status=$4) AND (cardinality($5::text[])=0 OR status = ANY($5))
		AND ($6='' OR driver_email=lower($6)) ORDER BY plan_date DESC, trip_number LIMIT $7`,
		actor.From(ctx).OrgID, req.DepotId, req.PlanDate, req.Status, statuses, req.DriverEmail, limit)
	if err != nil {
		return nil, errs.DB(err, "trips")
	}
	var list []*tmsv1.Trip
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			rows.Close()
			return nil, errs.DB(err, "trips")
		}
		list = append(list, t)
	}
	rows.Close()
	return &tmsv1.ListTripsResponse{Trips: list}, errs.DB(s.loadStops(ctx, list), "stops")
}

func (s *server) GetStats(ctx context.Context, req *tmsv1.PlanningStatsRequest) (*tmsv1.PlanningStats, error) {
	org, depot := actor.From(ctx).OrgID, req.DepotId
	today := time.Now().In(loc).Format(ts.DateLayout)
	out := &tmsv1.PlanningStats{}
	err := s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND plan_date=$3::date AND status <> 'CANCELLED'),
		(SELECT count(*) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status IN ('RELEASED','IN_PROGRESS')),
		(SELECT count(*) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status='COMPLETED' AND completed_at >= $3::date),
		(SELECT COALESCE(avg(capacity_util_pct),0) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'CANCELLED'
			AND plan_date >= $3::date - 7)::float8,
		(SELECT COALESCE(avg(n),0) FROM (SELECT count(s.id) n FROM trips t JOIN stops s ON s.trip_id=t.id WHERE t.org_id=$1
			AND ($2='' OR t.depot_id::text=$2) AND t.status <> 'CANCELLED' AND t.plan_date >= $3::date - 7 GROUP BY t.id) x)::float8,
		(SELECT COALESCE(sum(distance_km),0) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'CANCELLED'
			AND plan_date >= $3::date - 7)::float8,
		(SELECT COALESCE(sum(estimated_cost),0) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'CANCELLED'
			AND plan_date >= $3::date - 7)::float8,
		(SELECT count(*) FROM stops s JOIN trips t ON t.id=s.trip_id WHERE t.org_id=$1 AND ($2='' OR t.depot_id::text=$2)
			AND t.status IN ('PLANNED','RELEASED','IN_PROGRESS') AND s.status IN ('PENDING','ARRIVED')
			AND to_char(s.eta AT TIME ZONE $4, 'HH24:MI') > s.window_end)`,
		org, depot, today, loc.String()).Scan(&out.TripsToday, &out.ActiveTrips, &out.CompletedToday, &out.AvgUtilizationPct,
		&out.AvgStopsPerTrip, &out.DistanceKm_7D, &out.Cost_7D, &out.LateStopsPlanned)
	if err != nil {
		return nil, errs.DB(err, "planning stats")
	}
	if out.DistanceKm_7D > 0 {
		out.CostPerKm_7D = out.Cost_7D / out.DistanceKm_7D
	}
	rows, err := s.db.Query(ctx, `SELECT status, count(*) FROM trips WHERE org_id=$1 AND ($2='' OR depot_id::text=$2)
		AND plan_date >= $3::date - 7 GROUP BY status`, org, depot, today)
	if err != nil {
		return nil, errs.DB(err, "planning stats")
	}
	defer rows.Close()
	for rows.Next() {
		var sc tmsv1.StatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, errs.DB(err, "planning stats")
		}
		out.StatusCounts = append(out.StatusCounts, &sc)
	}
	return out, rows.Err()
}
