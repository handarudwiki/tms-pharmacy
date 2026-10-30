package main

import (
	"context"
	"fmt"

	"time"

	"github.com/jackc/pgx/v5"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

// Cold-chain limits for refrigerated pharmaceuticals (FR-10).
const (
	coldMinC = 2.0
	coldMaxC = 8.0
)

func shipmentIDs(t *tmsv1.Trip, statuses ...string) []string {
	var ids []string
	for _, st := range t.Stops {
		for _, s := range statuses {
			if st.Status == s {
				ids = append(ids, st.ShipmentId)
			}
		}
	}
	return ids
}

// driverGuard lets drivers act only on their own trips; office roles may act on any trip.
func driverGuard(ctx context.Context, t *tmsv1.Trip) error {
	a := actor.From(ctx)
	if a.HasRole(actor.RoleAdmin, actor.RoleTransportMgr, actor.RoleDispatcher) {
		return nil
	}
	if a.HasRole(actor.RoleDriver) && a.Email != "" && a.Email == t.DriverEmail {
		return nil
	}
	return errs.Denied("this trip is assigned to another driver")
}

func (s *server) setStatus(ctx context.Context, id, from, to, extra string) error {
	tag, err := s.db.Exec(ctx, `UPDATE trips SET status=$4`+extra+` WHERE id=$1 AND org_id=$2 AND status=$3`, id, actor.From(ctx).OrgID, from, to)
	if err != nil {
		return errs.DB(err, "trip")
	}
	if tag.RowsAffected() == 0 {
		return errs.Precondition("trip is not %s", from)
	}
	return nil
}

// ReleaseTrip hands a planned trip to the driver; its shipments become DISPATCHED.
func (s *server) ReleaseTrip(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Trip, error) {
	t, err := s.GetTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if t.Status != "PLANNED" {
		return nil, errs.Precondition("trip %s is %s", t.TripNumber, t.Status)
	}
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE trips SET status='RELEASED' WHERE id=$1 AND status='PLANNED'`, t.Id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.Precondition("trip was changed concurrently")
		}
		_, err = s.shipments.MarkDispatched(ctx, &tmsv1.MarkDispatchedRequest{Ids: shipmentIDs(t, "PENDING"), TripId: t.Id,
			VehiclePlate: t.VehiclePlate, DriverName: t.DriverName})
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "trip release")
	}
	return s.GetTrip(ctx, req)
}

func (s *server) StartTrip(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Trip, error) {
	t, err := s.GetTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := driverGuard(ctx, t); err != nil {
		return nil, err
	}
	if err := s.setStatus(ctx, t.Id, "RELEASED", "IN_PROGRESS", ", started_at=now()"); err != nil {
		return nil, err
	}
	return s.GetTrip(ctx, req)
}

// stopFor loads the trip and checks that the stop can still be worked.
func (s *server) stopFor(ctx context.Context, tripID, stopID string) (*tmsv1.Trip, *tmsv1.Stop, error) {
	t, err := s.GetTrip(ctx, &tmsv1.IdRequest{Id: tripID})
	if err != nil {
		return nil, nil, err
	}
	if err := driverGuard(ctx, t); err != nil {
		return nil, nil, err
	}
	if t.Status != "IN_PROGRESS" {
		return nil, nil, errs.Precondition("start the trip before working its stops")
	}
	for _, st := range t.Stops {
		if st.Id == stopID {
			if st.Status == "DELIVERED" || st.Status == "FAILED" {
				return nil, nil, errs.Precondition("stop %d is already %s", st.Sequence, st.Status)
			}
			return t, st, nil
		}
	}
	return nil, nil, errs.NotFound("stop not found on this trip")
}

func (s *server) ArriveStop(ctx context.Context, req *tmsv1.StopRequest) (*tmsv1.Trip, error) {
	t, st, err := s.stopFor(ctx, req.TripId, req.StopId)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE stops SET status='ARRIVED', arrived_at=now() WHERE id=$1 AND status='PENDING'`, st.Id); err != nil {
		return nil, errs.DB(err, "stop")
	}
	if _, err := s.db.Exec(ctx, `UPDATE trips SET last_lat=$2, last_lng=$3, last_ping_at=now() WHERE id=$1`, t.Id, st.Lat, st.Lng); err != nil {
		return nil, errs.DB(err, "trip")
	}
	return s.GetTrip(ctx, &tmsv1.IdRequest{Id: t.Id})
}

// CompleteStop records proof of delivery. The shipment call is idempotent per stop, so a retry
// after a partial failure cannot deliver twice.
func (s *server) CompleteStop(ctx context.Context, req *tmsv1.CompleteStopRequest) (*tmsv1.Trip, error) {
	t, st, err := s.stopFor(ctx, req.TripId, req.StopId)
	if err != nil {
		return nil, err
	}
	lat, lng := req.Lat, req.Lng
	if lat == 0 && lng == 0 {
		lat, lng = st.Lat, st.Lng
	}
	if _, err := s.shipments.RecordDelivery(ctx, &tmsv1.RecordDeliveryRequest{Id: st.ShipmentId, IdempotencyKey: "stop:" + st.Id,
		Pod: &tmsv1.ProofOfDelivery{ReceiverName: req.ReceiverName, SignatureCaptured: req.SignatureCaptured, Note: req.Note, Lat: lat, Lng: lng}}); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE stops SET status='DELIVERED', completed_at=now(), arrived_at=COALESCE(arrived_at, now()) WHERE id=$1`, st.Id); err != nil {
		return nil, errs.DB(err, "stop")
	}
	if _, err := s.db.Exec(ctx, `UPDATE trips SET last_lat=$2, last_lng=$3, last_ping_at=now() WHERE id=$1`, t.Id, lat, lng); err != nil {
		return nil, errs.DB(err, "trip")
	}
	return s.GetTrip(ctx, &tmsv1.IdRequest{Id: t.Id})
}

func (s *server) FailStop(ctx context.Context, req *tmsv1.FailStopRequest) (*tmsv1.Trip, error) {
	t, st, err := s.stopFor(ctx, req.TripId, req.StopId)
	if err != nil {
		return nil, err
	}
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE stops SET status='FAILED', failure_reason=$2, completed_at=now(), arrived_at=COALESCE(arrived_at, now())
			WHERE id=$1 AND status IN ('PENDING','ARRIVED')`, st.Id, req.Reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.Precondition("stop was completed concurrently")
		}
		_, err = s.shipments.RecordFailedDelivery(ctx, &tmsv1.RecordFailedDeliveryRequest{Id: st.ShipmentId, Reason: req.Reason,
			Note: req.Note, Reschedule: req.Reschedule})
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "failed stop")
	}
	return s.GetTrip(ctx, &tmsv1.IdRequest{Id: t.Id})
}

// CompleteTrip closes a trip once every stop is delivered or failed (FR-07).
func (s *server) CompleteTrip(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Trip, error) {
	t, err := s.GetTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := driverGuard(ctx, t); err != nil {
		return nil, err
	}
	if open := len(shipmentIDs(t, "PENDING", "ARRIVED")); open > 0 {
		return nil, errs.Precondition("%d stop(s) are still open — deliver or fail them first", open)
	}
	if err := s.setStatus(ctx, t.Id, "IN_PROGRESS", "COMPLETED", ", completed_at=now(), last_lat=depot_lat, last_lng=depot_lng"); err != nil {
		return nil, err
	}
	return s.GetTrip(ctx, req)
}

// CancelTrip returns the shipments of a trip that has not started to the planning pool (BR-07).
func (s *server) CancelTrip(ctx context.Context, req *tmsv1.CancelTripRequest) (*tmsv1.Trip, error) {
	if req.Reason == "" {
		return nil, errs.Invalid("a cancellation reason is required")
	}
	t, err := s.GetTrip(ctx, &tmsv1.IdRequest{Id: req.Id})
	if err != nil {
		return nil, err
	}
	if t.Status != "PLANNED" && t.Status != "RELEASED" {
		return nil, errs.Precondition("trip %s is %s; started trips must be completed stop by stop", t.TripNumber, t.Status)
	}
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE trips SET status='CANCELLED', cancel_reason=$3 WHERE id=$1 AND status=$2`, t.Id, t.Status, req.Reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.Precondition("trip was changed concurrently")
		}
		_, err = s.shipments.Unplan(ctx, &tmsv1.IdsRequest{Ids: shipmentIDs(t, "PENDING")})
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "trip cancellation")
	}
	return s.GetTrip(ctx, &tmsv1.IdRequest{Id: t.Id})
}

// RecordTelemetry stores a position/temperature ping. On a cold trip the first reading outside
// 2–8 °C after an in-range one raises a high-severity excursion for every shipment on board.
func (s *server) RecordTelemetry(ctx context.Context, req *tmsv1.Telemetry) (*tmsv1.Telemetry, error) {
	t, err := s.GetTrip(ctx, &tmsv1.IdRequest{Id: req.TripId})
	if err != nil {
		return nil, err
	}
	if err := driverGuard(ctx, t); err != nil {
		return nil, err
	}
	if t.Status != "IN_PROGRESS" {
		return nil, errs.Precondition("telemetry is only accepted for trips in progress")
	}
	excursion := t.TemperatureClass == "COLD" && req.HasTemperature && (req.TemperatureC < coldMinC || req.TemperatureC > coldMaxC)
	var temp any
	if req.HasTemperature {
		temp = req.TemperatureC
	}
	var wasExcursion bool
	_ = s.db.QueryRow(ctx, `SELECT excursion FROM telemetry WHERE trip_id=$1 ORDER BY recorded_at DESC LIMIT 1`, t.Id).Scan(&wasExcursion)
	err = s.db.QueryRow(ctx, `INSERT INTO telemetry(trip_id, lat, lng, temperature_c, speed_kmh, excursion) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`, t.Id, req.Lat, req.Lng, temp, req.SpeedKmh, excursion).Scan(&req.Id)
	if err != nil {
		return nil, errs.DB(err, "telemetry")
	}
	if _, err := s.db.Exec(ctx, `UPDATE trips SET last_lat=$2, last_lng=$3, last_temperature_c=COALESCE($4, last_temperature_c), last_ping_at=now()
		WHERE id=$1`, t.Id, req.Lat, req.Lng, temp); err != nil {
		return nil, errs.DB(err, "trip")
	}
	if excursion && !wasExcursion {
		for _, st := range t.Stops {
			if st.Status == "PENDING" || st.Status == "ARRIVED" {
				if _, err := s.shipments.RaiseException(ctx, &tmsv1.TransportException{DepotId: t.DepotId, ExceptionType: "TEMPERATURE_EXCURSION",
					Severity: "HIGH", ShipmentId: st.ShipmentId, ShipmentNumber: st.ShipmentNumber, TripId: t.Id,
					Details: fmt.Sprintf("%s on %s recorded %.1f °C (allowed %.0f–%.0f °C)", t.VehiclePlate, t.TripNumber, req.TemperatureC, coldMinC, coldMaxC)}); err != nil {
					return nil, err
				}
			}
		}
	}
	req.Excursion = excursion
	req.RecordedAt = ts.Time(time.Now())
	return req, nil
}

func (s *server) ListTelemetry(ctx context.Context, req *tmsv1.ListTelemetryRequest) (*tmsv1.ListTelemetryResponse, error) {
	if _, err := s.GetTrip(ctx, &tmsv1.IdRequest{Id: req.TripId}); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `SELECT id, trip_id, lat, lng, temperature_c, speed_kmh, excursion, recorded_at FROM (
		SELECT * FROM telemetry WHERE trip_id=$1 ORDER BY recorded_at DESC LIMIT $2) x ORDER BY recorded_at`, req.TripId, limit)
	if err != nil {
		return nil, errs.DB(err, "telemetry")
	}
	defer rows.Close()
	out := &tmsv1.ListTelemetryResponse{}
	for rows.Next() {
		var r tmsv1.Telemetry
		var temp *float64
		var at time.Time
		if err := rows.Scan(&r.Id, &r.TripId, &r.Lat, &r.Lng, &temp, &r.SpeedKmh, &r.Excursion, &at); err != nil {
			return nil, errs.DB(err, "telemetry")
		}
		if temp != nil {
			r.TemperatureC, r.HasTemperature = *temp, true
		}
		r.RecordedAt = ts.Time(at)
		out.Readings = append(out.Readings, &r)
	}
	return out, rows.Err()
}
