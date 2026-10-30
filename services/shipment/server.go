package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/idem"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

type server struct {
	tmsv1.UnimplementedShipmentServiceServer
	db *pgxpool.Pool
}

var (
	hhmm           = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	failureReasons = []string{"CUSTOMER_ABSENT", "REFUSED", "ADDRESS_NOT_FOUND", "DAMAGED", "CLOSED", "OTHER"}
)

// maxAttempts is the number of failed attempts after which a shipment returns to the depot.
const maxAttempts = 3

const shipmentColumns = `id, shipment_number, external_ref, source, depot_id, consignee_name, consignee_phone, address, city, lat, lng,
	window_start, window_end, required_date, weight_kg, volume_l, packages, temperature_class, controlled, priority, status, trip_id,
	carrier_id, carrier_name, freight_cost, attempts, pod_receiver_name, pod_signature, pod_note, pod_lat, pod_lng, pod_recorded_by,
	delivered_at, notes, created_at, updated_at, location_approximate, tender_id`

func scanShipment(row pgx.Row) (*tmsv1.Shipment, error) {
	var s tmsv1.Shipment
	var req pgtype.Date
	var pod tmsv1.ProofOfDelivery
	var delivered *time.Time
	var created, updated time.Time
	err := row.Scan(&s.Id, &s.ShipmentNumber, &s.ExternalRef, &s.Source, &s.DepotId, &s.ConsigneeName, &s.ConsigneePhone, &s.Address,
		&s.City, &s.Lat, &s.Lng, &s.WindowStart, &s.WindowEnd, &req, &s.WeightKg, &s.VolumeL, &s.Packages, &s.TemperatureClass,
		&s.Controlled, &s.Priority, &s.Status, &s.TripId, &s.CarrierId, &s.CarrierName, &s.FreightCost, &s.Attempts,
		&pod.ReceiverName, &pod.SignatureCaptured, &pod.Note, &pod.Lat, &pod.Lng, &pod.RecordedBy, &delivered, &s.Notes,
		&created, &updated, &s.LocationApproximate, &s.TenderId)
	s.RequiredDate, s.CreatedAt, s.UpdatedAt, s.DeliveredAt = ts.Date(req), ts.Time(created), ts.Time(updated), ts.PtrTime(delivered)
	if delivered != nil {
		pod.RecordedAt = s.DeliveredAt
		s.Pod = &pod
		if req.Valid {
			s.DeliveredOnTime = delivered.In(time.Local).Format(ts.DateLayout) <= ts.Date(req)
		}
	}
	return &s, err
}

func event(ctx context.Context, tx pgx.Tx, shipmentID, kind, desc string) error {
	_, err := tx.Exec(ctx, `INSERT INTO shipment_events(shipment_id, event_type, description, actor) VALUES ($1,$2,$3,$4)`,
		shipmentID, kind, desc, actor.From(ctx).Name())
	return err
}

func raise(ctx context.Context, tx pgx.Tx, e *tmsv1.TransportException) error {
	if e.Severity == "" {
		e.Severity = "MEDIUM"
	}
	_, err := tx.Exec(ctx, `INSERT INTO exceptions(org_id, depot_id, exception_type, severity, shipment_id, shipment_number, trip_id, details, raised_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, actor.From(ctx).OrgID, e.DepotId, e.ExceptionType, e.Severity, e.ShipmentId,
		e.ShipmentNumber, e.TripId, e.Details, actor.From(ctx).Name())
	return err
}

func (s *server) get(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (*tmsv1.Shipment, error) {
	sql := `SELECT ` + shipmentColumns + ` FROM shipments WHERE id=$1 AND org_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	sh, err := scanShipment(q.QueryRow(ctx, sql, id, actor.From(ctx).OrgID))
	return sh, errs.DB(err, "shipment")
}

// CreateShipment registers a shipment. A shipment whose external reference already exists is
// returned unchanged, which makes WMS imports idempotent (BR-06).
func (s *server) CreateShipment(ctx context.Context, req *tmsv1.CreateShipmentRequest) (*tmsv1.Shipment, error) {
	a := actor.From(ctx)
	if req.ExternalRef != "" {
		var id string
		if err := s.db.QueryRow(ctx, `SELECT id FROM shipments WHERE org_id=$1 AND external_ref=$2`, a.OrgID, req.ExternalRef).Scan(&id); err == nil {
			return s.get(ctx, s.db, id, false)
		}
	}
	if req.Source == "" {
		req.Source = "MANUAL"
	}
	if req.TemperatureClass == "" {
		req.TemperatureClass = "AMBIENT"
	}
	if req.WindowStart == "" {
		req.WindowStart = "08:00"
	}
	if req.WindowEnd == "" {
		req.WindowEnd = "17:00"
	}
	if req.Priority == 0 {
		req.Priority = 3
	}
	if req.Packages == 0 {
		req.Packages = 1
	}
	if req.VolumeL == 0 {
		req.VolumeL = req.WeightKg * 2.5
	}
	switch {
	case req.DepotId == "" || strings.TrimSpace(req.ConsigneeName) == "" || strings.TrimSpace(req.Address) == "":
		return nil, errs.Invalid("depot, consignee and address are required")
	case req.Lat == 0 && req.Lng == 0:
		return nil, errs.Invalid("delivery coordinates are required")
	case req.WeightKg <= 0:
		return nil, errs.Invalid("weight must be positive")
	case !hhmm.MatchString(req.WindowStart) || !hhmm.MatchString(req.WindowEnd) || req.WindowStart >= req.WindowEnd:
		return nil, errs.Invalid("delivery window must be HH:MM with start before end")
	case req.TemperatureClass != "AMBIENT" && req.TemperatureClass != "COLD":
		return nil, errs.Invalid("temperature class must be AMBIENT or COLD")
	case req.Priority < 1 || req.Priority > 5:
		return nil, errs.Invalid("priority must be between 1 and 5")
	}
	if req.RequiredDate == "" {
		req.RequiredDate = time.Now().AddDate(0, 0, 1).Format(ts.DateLayout)
	}
	required, err := ts.ParseDate(req.RequiredDate)
	if err != nil {
		return nil, errs.Invalid("required date must be YYYY-MM-DD")
	}
	var id string
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT nextval('shipment_seq')`).Scan(&seq); err != nil {
			return err
		}
		number := fmt.Sprintf("SHP-%s-%04d", time.Now().Format("060102"), seq)
		if err := tx.QueryRow(ctx, `INSERT INTO shipments(org_id, shipment_number, external_ref, source, depot_id, consignee_name, consignee_phone,
			address, city, lat, lng, location_approximate, window_start, window_end, required_date, weight_kg, volume_l, packages,
			temperature_class, controlled, priority, notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) RETURNING id`,
			a.OrgID, number, req.ExternalRef, req.Source, req.DepotId, req.ConsigneeName, req.ConsigneePhone, req.Address, req.City,
			req.Lat, req.Lng, req.LocationApproximate, req.WindowStart, req.WindowEnd, required, req.WeightKg, req.VolumeL, req.Packages,
			req.TemperatureClass, req.Controlled, req.Priority, req.Notes).Scan(&id); err != nil {
			return err
		}
		desc := "Shipment created"
		if req.Source == "WMS" {
			desc = "Imported from WMS order " + req.ExternalRef
		}
		return event(ctx, tx, id, "CREATED", desc)
	})
	if err != nil {
		return nil, errs.DB(err, "shipment")
	}
	return s.get(ctx, s.db, id, false)
}

func (s *server) ListShipments(ctx context.Context, req *tmsv1.ListShipmentsRequest) (*tmsv1.ListShipmentsResponse, error) {
	statuses, ids := req.Statuses, req.Ids
	if statuses == nil {
		statuses = []string{}
	}
	if ids == nil {
		ids = []string{}
	}
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2)
		AND ($3='' OR status=$3) AND (cardinality($4::text[])=0 OR status = ANY($4)) AND (cardinality($5::text[])=0 OR id::text = ANY($5))
		AND ($6='' OR trip_id=$6) AND ($7='' OR shipment_number ILIKE '%'||$7||'%' OR consignee_name ILIKE '%'||$7||'%' OR external_ref ILIKE '%'||$7||'%')
		AND ($8='' OR required_date <= $8::date) ORDER BY priority, required_date, created_at DESC LIMIT $9`,
		actor.From(ctx).OrgID, req.DepotId, req.Status, statuses, ids, req.TripId, req.Query, req.RequiredBy, limit)
	if err != nil {
		return nil, errs.DB(err, "shipments")
	}
	defer rows.Close()
	out := &tmsv1.ListShipmentsResponse{}
	for rows.Next() {
		sh, err := scanShipment(rows)
		if err != nil {
			return nil, errs.DB(err, "shipments")
		}
		out.Shipments = append(out.Shipments, sh)
	}
	return out, rows.Err()
}

func (s *server) GetShipment(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Shipment, error) {
	return s.get(ctx, s.db, req.Id, false)
}

// transition moves a set of shipments between statuses, all or nothing.
func (s *server) transition(ctx context.Context, ids []string, from []string, apply func(tx pgx.Tx, sh *tmsv1.Shipment) error) (*tmsv1.ListShipmentsResponse, error) {
	if len(ids) == 0 {
		return nil, errs.Invalid("no shipments given")
	}
	err := db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		for _, id := range ids {
			sh, err := s.get(ctx, tx, id, true)
			if err != nil {
				return err
			}
			if !slices.Contains(from, sh.Status) {
				return errs.Precondition("shipment %s is %s", sh.ShipmentNumber, sh.Status)
			}
			if err := apply(tx, sh); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, errs.DB(err, "shipments")
	}
	return s.ListShipments(ctx, &tmsv1.ListShipmentsRequest{Ids: ids})
}

func (s *server) MarkPlanned(ctx context.Context, req *tmsv1.MarkPlannedRequest) (*tmsv1.ListShipmentsResponse, error) {
	return s.transition(ctx, req.Ids, []string{"PENDING"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='PLANNED', trip_id=$2, updated_at=now() WHERE id=$1`, sh.Id, req.TripId); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "PLANNED", "Planned on trip")
	})
}

func (s *server) MarkTendered(ctx context.Context, req *tmsv1.MarkTenderedRequest) (*tmsv1.ListShipmentsResponse, error) {
	return s.transition(ctx, req.Ids, []string{"PENDING"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='TENDERED', tender_id=$2, updated_at=now() WHERE id=$1`, sh.Id, req.TenderId); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "TENDERED", "Offered to carriers for outsourcing")
	})
}

func (s *server) Unplan(ctx context.Context, req *tmsv1.IdsRequest) (*tmsv1.ListShipmentsResponse, error) {
	// DISPATCHED is allowed for in-house trips cancelled before they start (planning enforces that).
	return s.transition(ctx, req.Ids, []string{"PLANNED", "TENDERED", "DISPATCHED"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if sh.Status == "DISPATCHED" && sh.CarrierId != "" {
			return errs.Precondition("shipment %s is already with carrier %s", sh.ShipmentNumber, sh.CarrierName)
		}
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='PENDING', trip_id='', tender_id='', updated_at=now() WHERE id=$1`, sh.Id); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "UNPLANNED", "Returned to the planning pool")
	})
}

func (s *server) MarkDispatched(ctx context.Context, req *tmsv1.MarkDispatchedRequest) (*tmsv1.ListShipmentsResponse, error) {
	return s.transition(ctx, req.Ids, []string{"PLANNED"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if sh.TripId != req.TripId {
			return errs.Precondition("shipment %s is planned on another trip", sh.ShipmentNumber)
		}
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='DISPATCHED', updated_at=now() WHERE id=$1`, sh.Id); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "DISPATCHED", fmt.Sprintf("Out for delivery with %s (%s)", req.VehiclePlate, req.DriverName))
	})
}

// RecordDelivery closes a shipment with proof of delivery (BR-05). Repeats are idempotent.
func (s *server) RecordDelivery(ctx context.Context, req *tmsv1.RecordDeliveryRequest) (*tmsv1.Shipment, error) {
	pod := req.Pod
	if pod == nil || strings.TrimSpace(pod.ReceiverName) == "" || !pod.SignatureCaptured {
		return nil, errs.Invalid("proof of delivery needs the receiver's name and signature")
	}
	out := &tmsv1.Shipment{}
	scope := actor.From(ctx).OrgID + ":deliver"
	err := db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		found, err := idem.Lookup(ctx, tx, scope, req.IdempotencyKey, out)
		if err != nil || found {
			return err
		}
		sh, err := s.get(ctx, tx, req.Id, true)
		if err != nil {
			return err
		}
		if sh.Status == "DELIVERED" {
			out = sh
			return nil
		}
		if sh.Status != "DISPATCHED" {
			return errs.Precondition("shipment %s is %s, not out for delivery", sh.ShipmentNumber, sh.Status)
		}
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='DELIVERED', pod_receiver_name=$2, pod_signature=true, pod_note=$3,
			pod_lat=$4, pod_lng=$5, pod_recorded_by=$6, delivered_at=now(), attempts=attempts+1, updated_at=now() WHERE id=$1`,
			sh.Id, pod.ReceiverName, pod.Note, pod.Lat, pod.Lng, actor.From(ctx).Name()); err != nil {
			return err
		}
		if err := event(ctx, tx, sh.Id, "DELIVERED", "Delivered to "+pod.ReceiverName); err != nil {
			return err
		}
		if out, err = s.get(ctx, tx, sh.Id, false); err != nil {
			return err
		}
		if !out.DeliveredOnTime {
			if err := raise(ctx, tx, &tmsv1.TransportException{DepotId: sh.DepotId, ExceptionType: "LATE_DELIVERY", Severity: "LOW",
				ShipmentId: sh.Id, ShipmentNumber: sh.ShipmentNumber, TripId: sh.TripId,
				Details: fmt.Sprintf("Delivered after the required date %s", sh.RequiredDate)}); err != nil {
				return err
			}
		}
		return idem.Save(ctx, tx, scope, req.IdempotencyKey, out)
	})
	if err != nil {
		return nil, errs.DB(err, "delivery")
	}
	return out, nil
}

// RecordFailedDelivery logs a failed attempt. Rescheduled shipments return to the planning pool;
// otherwise, or after the maximum number of attempts, they are returned to the depot (FR-09).
func (s *server) RecordFailedDelivery(ctx context.Context, req *tmsv1.RecordFailedDeliveryRequest) (*tmsv1.Shipment, error) {
	if !slices.Contains(failureReasons, req.Reason) {
		return nil, errs.Invalid("reason must be one of %s", strings.Join(failureReasons, ", "))
	}
	err := db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		sh, err := s.get(ctx, tx, req.Id, true)
		if err != nil {
			return err
		}
		if sh.Status != "DISPATCHED" {
			return errs.Precondition("shipment %s is %s, not out for delivery", sh.ShipmentNumber, sh.Status)
		}
		attempts := sh.Attempts + 1
		next := "PENDING"
		if !req.Reschedule || req.Reason == "REFUSED" || attempts >= maxAttempts {
			next = "RETURNED"
		}
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status=$2, attempts=$3, trip_id=CASE WHEN $2='PENDING' THEN '' ELSE trip_id END,
			updated_at=now() WHERE id=$1`, sh.Id, next, attempts); err != nil {
			return err
		}
		desc := fmt.Sprintf("Attempt %d failed: %s. %s", attempts, strings.ReplaceAll(strings.ToLower(req.Reason), "_", " "), req.Note)
		if err := event(ctx, tx, sh.Id, "DELIVERY_FAILED", desc); err != nil {
			return err
		}
		kind, sev := "FAILED_DELIVERY", "MEDIUM"
		if next == "RETURNED" {
			kind, sev = "RETURNED_TO_DEPOT", "HIGH"
			if err := event(ctx, tx, sh.Id, "RETURNED", "Returned to depot"); err != nil {
				return err
			}
		} else if err := event(ctx, tx, sh.Id, "RESCHEDULED", "Back in the planning pool for another attempt"); err != nil {
			return err
		}
		return raise(ctx, tx, &tmsv1.TransportException{DepotId: sh.DepotId, ExceptionType: kind, Severity: sev, ShipmentId: sh.Id,
			ShipmentNumber: sh.ShipmentNumber, TripId: sh.TripId, Details: desc})
	})
	if err != nil {
		return nil, errs.DB(err, "failed delivery")
	}
	return s.get(ctx, s.db, req.Id, false)
}

// AssignCarrier hands a shipment to a third-party carrier at the awarded freight cost.
func (s *server) AssignCarrier(ctx context.Context, req *tmsv1.AssignCarrierRequest) (*tmsv1.Shipment, error) {
	if req.CarrierId == "" || req.FreightCost < 0 {
		return nil, errs.Invalid("carrier and a non-negative freight cost are required")
	}
	_, err := s.transition(ctx, []string{req.Id}, []string{"PENDING", "TENDERED"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='DISPATCHED', carrier_id=$2, carrier_name=$3, freight_cost=$4, trip_id='',
			updated_at=now() WHERE id=$1`, sh.Id, req.CarrierId, req.CarrierName, req.FreightCost); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "CARRIER_ASSIGNED", fmt.Sprintf("Outsourced to %s for IDR %.0f", req.CarrierName, req.FreightCost))
	})
	if err != nil {
		return nil, err
	}
	return s.get(ctx, s.db, req.Id, false)
}

func (s *server) CancelShipment(ctx context.Context, req *tmsv1.CancelShipmentRequest) (*tmsv1.Shipment, error) {
	if req.Reason == "" {
		return nil, errs.Invalid("a cancellation reason is required")
	}
	_, err := s.transition(ctx, []string{req.Id}, []string{"PENDING"}, func(tx pgx.Tx, sh *tmsv1.Shipment) error {
		if _, err := tx.Exec(ctx, `UPDATE shipments SET status='CANCELLED', notes=$2, updated_at=now() WHERE id=$1`, sh.Id, req.Reason); err != nil {
			return err
		}
		return event(ctx, tx, sh.Id, "CANCELLED", req.Reason)
	})
	if err != nil {
		return nil, err
	}
	return s.get(ctx, s.db, req.Id, false)
}

func (s *server) ListEvents(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.ListEventsResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT e.id, e.shipment_id, e.event_type, e.description, e.actor, e.created_at FROM shipment_events e
		JOIN shipments s ON s.id = e.shipment_id WHERE e.shipment_id::text=$1 AND s.org_id=$2 ORDER BY e.created_at`, req.Id, actor.From(ctx).OrgID)
	if err != nil {
		return nil, errs.DB(err, "events")
	}
	defer rows.Close()
	out := &tmsv1.ListEventsResponse{}
	for rows.Next() {
		var e tmsv1.ShipmentEvent
		var at time.Time
		if err := rows.Scan(&e.Id, &e.ShipmentId, &e.EventType, &e.Description, &e.Actor, &at); err != nil {
			return nil, errs.DB(err, "events")
		}
		e.CreatedAt = ts.Time(at)
		out.Events = append(out.Events, &e)
	}
	return out, rows.Err()
}

// --- exceptions ---

const exceptionColumns = `id, depot_id, exception_type, severity, shipment_id, shipment_number, trip_id, details, status, raised_by,
	resolved_by, resolution, created_at, resolved_at`

func scanException(row pgx.Row) (*tmsv1.TransportException, error) {
	var e tmsv1.TransportException
	var created time.Time
	var resolved *time.Time
	err := row.Scan(&e.Id, &e.DepotId, &e.ExceptionType, &e.Severity, &e.ShipmentId, &e.ShipmentNumber, &e.TripId, &e.Details,
		&e.Status, &e.RaisedBy, &e.ResolvedBy, &e.Resolution, &created, &resolved)
	e.CreatedAt, e.ResolvedAt = ts.Time(created), ts.PtrTime(resolved)
	return &e, err
}

func (s *server) RaiseException(ctx context.Context, req *tmsv1.TransportException) (*tmsv1.TransportException, error) {
	if req.ExceptionType == "" || req.Details == "" {
		return nil, errs.Invalid("exception type and details are required")
	}
	if req.Severity == "" {
		req.Severity = "MEDIUM"
	}
	a := actor.From(ctx)
	e, err := scanException(s.db.QueryRow(ctx, `INSERT INTO exceptions(org_id, depot_id, exception_type, severity, shipment_id, shipment_number,
		trip_id, details, raised_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+exceptionColumns,
		a.OrgID, req.DepotId, req.ExceptionType, req.Severity, req.ShipmentId, req.ShipmentNumber, req.TripId, req.Details, a.Name()))
	return e, errs.DB(err, "exception")
}

func (s *server) ListExceptions(ctx context.Context, req *tmsv1.ListExceptionsRequest) (*tmsv1.ListExceptionsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `SELECT `+exceptionColumns+` FROM exceptions WHERE org_id=$1 AND ($2='' OR depot_id=$2)
		AND ($3='' OR status=$3) ORDER BY created_at DESC LIMIT $4`, actor.From(ctx).OrgID, req.DepotId, req.Status, limit)
	if err != nil {
		return nil, errs.DB(err, "exceptions")
	}
	defer rows.Close()
	out := &tmsv1.ListExceptionsResponse{}
	for rows.Next() {
		e, err := scanException(rows)
		if err != nil {
			return nil, errs.DB(err, "exceptions")
		}
		out.Exceptions = append(out.Exceptions, e)
	}
	return out, rows.Err()
}

func (s *server) ResolveException(ctx context.Context, req *tmsv1.ResolveExceptionRequest) (*tmsv1.TransportException, error) {
	if req.Resolution == "" {
		return nil, errs.Invalid("a resolution note is required")
	}
	a := actor.From(ctx)
	e, err := scanException(s.db.QueryRow(ctx, `UPDATE exceptions SET status='RESOLVED', resolution=$3, resolved_by=$4, resolved_at=now()
		WHERE id=$1 AND org_id=$2 AND status='OPEN' RETURNING `+exceptionColumns, req.Id, a.OrgID, req.Resolution, a.Name()))
	return e, errs.DB(err, "open exception")
}

func (s *server) GetStats(ctx context.Context, req *tmsv1.ShipmentStatsRequest) (*tmsv1.ShipmentStats, error) {
	org, depot := actor.From(ctx).OrgID, req.DepotId
	out := &tmsv1.ShipmentStats{}
	rows, err := s.db.Query(ctx, `SELECT status, count(*) FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) GROUP BY status`, org, depot)
	if err != nil {
		return nil, errs.DB(err, "stats")
	}
	for rows.Next() {
		var sc tmsv1.StatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			rows.Close()
			return nil, errs.DB(err, "stats")
		}
		out.StatusCounts = append(out.StatusCounts, &sc)
	}
	rows.Close()
	err = s.db.QueryRow(ctx, `WITH d AS (
			SELECT * FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND delivered_at >= now() - interval '30 days'),
		closed AS (
			SELECT * FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status IN ('DELIVERED','RETURNED')
			AND updated_at >= now() - interval '30 days')
		SELECT
		(SELECT count(*) FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND created_at >= current_date),
		(SELECT count(*) FROM d WHERE delivered_at >= current_date),
		(SELECT COALESCE(100.0 * count(*) FILTER (WHERE delivered_at::date <= required_date) / NULLIF(count(*),0), 100) FROM d)::float8,
		(SELECT COALESCE(100.0 * count(*) FILTER (WHERE status='DELIVERED' AND delivered_at::date <= required_date AND attempts = 1)
			/ NULLIF(count(*),0), 100) FROM closed)::float8,
		(SELECT COALESCE(100.0 * count(*) FILTER (WHERE status='RETURNED') / NULLIF(count(*),0), 0) FROM closed)::float8,
		(SELECT count(*) FROM exceptions WHERE org_id=$1 AND ($2='' OR depot_id=$2) AND status='OPEN'),
		(SELECT count(*) FROM exceptions WHERE org_id=$1 AND ($2='' OR depot_id=$2) AND exception_type='TEMPERATURE_EXCURSION'
			AND created_at >= now() - interval '30 days'),
		(SELECT COALESCE(avg(extract(epoch FROM delivered_at - created_at)) / 3600, 0) FROM d)::float8,
		(SELECT COALESCE(100.0 * count(*) FILTER (WHERE carrier_id <> '') / NULLIF(count(*),0), 0) FROM d)::float8,
		(SELECT COALESCE(sum(freight_cost),0) FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND carrier_id <> ''
			AND updated_at >= now() - interval '30 days')::float8`,
		org, depot).Scan(&out.CreatedToday, &out.DeliveredToday, &out.OnTimePct, &out.OtifPct, &out.FailedRatePct, &out.OpenExceptions,
		&out.ColdChainExcursions_30D, &out.AvgLeadTimeHours, &out.OutsourcedPct, &out.CarrierSpend_30D)
	if err != nil {
		return nil, errs.DB(err, "stats")
	}
	rows, err = s.db.Query(ctx, `SELECT d::date::text,
		(SELECT count(*) FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND created_at::date = d::date),
		(SELECT count(*) FROM shipments WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND delivered_at::date = d::date)
		FROM generate_series(current_date - 13, current_date, interval '1 day') d ORDER BY d`, org, depot)
	if err != nil {
		return nil, errs.DB(err, "trend")
	}
	defer rows.Close()
	for rows.Next() {
		var d tmsv1.DailyCount
		if err := rows.Scan(&d.Date, &d.Created, &d.Delivered); err != nil {
			return nil, errs.DB(err, "trend")
		}
		out.Trend = append(out.Trend, &d)
	}
	return out, rows.Err()
}
