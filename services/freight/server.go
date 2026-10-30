package main

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

const carrierColumns = `id, code, name, service_level, supports_cold, supports_controlled, base_rate, rate_per_kg, rate_per_km,
	cold_surcharge_pct, on_time_pct, contact, active, transit_days`

func scanCarrier(row pgx.Row) (*tmsv1.Carrier, error) {
	var c tmsv1.Carrier
	return &c, row.Scan(&c.Id, &c.Code, &c.Name, &c.ServiceLevel, &c.SupportsCold, &c.SupportsControlled, &c.BaseRate, &c.RatePerKg,
		&c.RatePerKm, &c.ColdSurchargePct, &c.OnTimePct, &c.Contact, &c.Active, &c.TransitDays)
}

func (s *server) UpsertCarrier(ctx context.Context, req *tmsv1.Carrier) (*tmsv1.Carrier, error) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.ServiceLevel == "" {
		req.ServiceLevel = "NEXT_DAY"
	}
	if req.TransitDays == 0 {
		req.TransitDays = 1
	}
	switch {
	case req.Code == "" || req.Name == "":
		return nil, errs.Invalid("carrier code and name are required")
	case !slices.Contains([]string{"SAME_DAY", "NEXT_DAY", "REGULAR"}, req.ServiceLevel):
		return nil, errs.Invalid("unknown service level")
	case req.BaseRate < 0 || req.RatePerKg < 0 || req.RatePerKm < 0 || req.ColdSurchargePct < 0:
		return nil, errs.Invalid("rates cannot be negative")
	case req.BaseRate+req.RatePerKg+req.RatePerKm == 0:
		return nil, errs.Invalid("the rate card needs at least one rate")
	}
	org := actor.From(ctx).OrgID
	var c *tmsv1.Carrier
	var err error
	if req.Id == "" {
		c, err = scanCarrier(s.db.QueryRow(ctx, `INSERT INTO carriers(org_id, code, name, service_level, supports_cold, supports_controlled,
			base_rate, rate_per_kg, rate_per_km, cold_surcharge_pct, on_time_pct, contact, transit_days)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+carrierColumns, org, req.Code, req.Name, req.ServiceLevel,
			req.SupportsCold, req.SupportsControlled, req.BaseRate, req.RatePerKg, req.RatePerKm, req.ColdSurchargePct,
			max(req.OnTimePct, 0), req.Contact, req.TransitDays))
	} else {
		c, err = scanCarrier(s.db.QueryRow(ctx, `UPDATE carriers SET code=$3, name=$4, service_level=$5, supports_cold=$6, supports_controlled=$7,
			base_rate=$8, rate_per_kg=$9, rate_per_km=$10, cold_surcharge_pct=$11, on_time_pct=$12, contact=$13, transit_days=$14, active=$15
			WHERE id=$1 AND org_id=$2 RETURNING `+carrierColumns, req.Id, org, req.Code, req.Name, req.ServiceLevel, req.SupportsCold,
			req.SupportsControlled, req.BaseRate, req.RatePerKg, req.RatePerKm, req.ColdSurchargePct, req.OnTimePct, req.Contact,
			req.TransitDays, req.Active))
	}
	return c, errs.DB(err, "carrier")
}

func (s *server) ListCarriers(ctx context.Context, req *tmsv1.ListCarriersRequest) (*tmsv1.ListCarriersResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT `+carrierColumns+` FROM carriers WHERE org_id=$1 AND ($2 OR active) ORDER BY name`,
		actor.From(ctx).OrgID, req.IncludeInactive)
	if err != nil {
		return nil, errs.DB(err, "carriers")
	}
	defer rows.Close()
	out := &tmsv1.ListCarriersResponse{}
	for rows.Next() {
		c, err := scanCarrier(rows)
		if err != nil {
			return nil, errs.DB(err, "carriers")
		}
		out.Carriers = append(out.Carriers, c)
	}
	return out, rows.Err()
}

func (s *server) GetCarrier(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Carrier, error) {
	c, err := scanCarrier(s.db.QueryRow(ctx, `SELECT `+carrierColumns+` FROM carriers WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	return c, errs.DB(err, "carrier")
}

// compliant checks a carrier's capabilities against the tender requirements (FR-11).
func compliant(c *tmsv1.Carrier, t *tmsv1.Tender) (bool, string) {
	switch {
	case !c.Active:
		return false, "carrier is inactive"
	case t.RequiresCold && !c.SupportsCold:
		return false, "no cold-chain capability"
	case t.RequiresControlled && !c.SupportsControlled:
		return false, "not licensed for controlled drugs"
	}
	return true, ""
}

// quote prices a tender from a carrier's rate card.
func quote(c *tmsv1.Carrier, t *tmsv1.Tender) float64 {
	total := 0.0
	for _, sh := range t.Shipments {
		amount := c.BaseRate + c.RatePerKg*sh.WeightKg + c.RatePerKm*sh.DistanceKm
		if sh.TemperatureClass == "COLD" {
			amount *= 1 + c.ColdSurchargePct/100
		}
		total += amount
	}
	return math.Round(total/100) * 100
}

func roadKm(aLat, aLng, bLat, bLng float64) float64 {
	const r = 6371.0
	dLat := (bLat - aLat) * math.Pi / 180
	dLng := (bLng - aLng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(aLat*math.Pi/180)*math.Cos(bLat*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(h)) * 1.35
}

// CreateTender offers pending shipments to carriers. Every active carrier is quoted from its
// rate card straight away; carriers may still submit manual bids until the tender is awarded.
func (s *server) CreateTender(ctx context.Context, req *tmsv1.CreateTenderRequest) (*tmsv1.Tender, error) {
	if req.DepotId == "" || len(req.ShipmentIds) == 0 {
		return nil, errs.Invalid("depot and at least one shipment are required")
	}
	depot, err := s.fleet.GetDepot(ctx, &tmsv1.IdRequest{Id: req.DepotId})
	if err != nil {
		return nil, err
	}
	res, err := s.shipments.ListShipments(ctx, &tmsv1.ListShipmentsRequest{Ids: req.ShipmentIds})
	if err != nil {
		return nil, err
	}
	if len(res.Shipments) != len(req.ShipmentIds) {
		return nil, errs.NotFound("some shipments were not found")
	}
	deadline := time.Now().Add(4 * time.Hour)
	if req.Deadline != "" {
		if deadline, err = time.Parse(time.RFC3339, req.Deadline); err != nil {
			return nil, errs.Invalid("deadline must be an RFC 3339 timestamp")
		}
	}
	t := &tmsv1.Tender{DepotId: depot.Id}
	for _, sh := range res.Shipments {
		if sh.Status != "PENDING" || sh.DepotId != depot.Id {
			return nil, errs.Precondition("shipment %s is %s or ships from another depot", sh.ShipmentNumber, sh.Status)
		}
		ts := &tmsv1.TenderShipment{ShipmentId: sh.Id, ShipmentNumber: sh.ShipmentNumber, ConsigneeName: sh.ConsigneeName,
			WeightKg: sh.WeightKg, DistanceKm: math.Round(roadKm(depot.Lat, depot.Lng, sh.Lat, sh.Lng)*10) / 10,
			TemperatureClass: sh.TemperatureClass, Controlled: sh.Controlled}
		t.Shipments = append(t.Shipments, ts)
		t.TotalWeightKg += ts.WeightKg
		t.TotalDistanceKm += ts.DistanceKm
		t.RequiresCold = t.RequiresCold || sh.TemperatureClass == "COLD"
		t.RequiresControlled = t.RequiresControlled || sh.Controlled
	}
	carriers, err := s.ListCarriers(ctx, &tmsv1.ListCarriersRequest{})
	if err != nil {
		return nil, err
	}
	a := actor.From(ctx)
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT nextval('tender_seq')`).Scan(&seq); err != nil {
			return err
		}
		number := fmt.Sprintf("TND-%s-%03d", time.Now().Format("060102"), seq)
		if err := tx.QueryRow(ctx, `INSERT INTO tenders(org_id, tender_number, depot_id, total_weight_kg, total_distance_km, requires_cold,
			requires_controlled, deadline, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, a.OrgID, number, depot.Id,
			t.TotalWeightKg, t.TotalDistanceKm, t.RequiresCold, t.RequiresControlled, deadline, a.Name()).Scan(&t.Id); err != nil {
			return err
		}
		for _, sh := range t.Shipments {
			if _, err := tx.Exec(ctx, `INSERT INTO tender_shipments(tender_id, shipment_id, shipment_number, consignee_name, weight_kg,
				distance_km, temperature_class, controlled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, t.Id, sh.ShipmentId, sh.ShipmentNumber,
				sh.ConsigneeName, sh.WeightKg, sh.DistanceKm, sh.TemperatureClass, sh.Controlled); err != nil {
				return err
			}
		}
		for _, c := range carriers.Carriers {
			ok, why := compliant(c, t)
			if _, err := tx.Exec(ctx, `INSERT INTO bids(tender_id, carrier_id, amount, transit_days, compliant, non_compliance_reason, source, note)
				VALUES ($1,$2,$3,$4,$5,$6,'RATE_CARD','Automatic quote from contracted rate card')`, t.Id, c.Id, quote(c, t),
				c.TransitDays, ok, why); err != nil {
				return err
			}
		}
		_, err := s.shipments.MarkTendered(ctx, &tmsv1.MarkTenderedRequest{Ids: req.ShipmentIds, TenderId: t.Id})
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "tender")
	}
	return s.GetTender(ctx, &tmsv1.IdRequest{Id: t.Id})
}

const tenderColumns = `id, tender_number, depot_id, status, COALESCE(awarded_bid_id::text,''), total_weight_kg, total_distance_km,
	requires_cold, requires_controlled, deadline, created_by, created_at, awarded_at, cancel_reason`

func scanTender(row pgx.Row) (*tmsv1.Tender, error) {
	var t tmsv1.Tender
	var deadline, created time.Time
	var awarded *time.Time
	err := row.Scan(&t.Id, &t.TenderNumber, &t.DepotId, &t.Status, &t.AwardedBidId, &t.TotalWeightKg, &t.TotalDistanceKm, &t.RequiresCold,
		&t.RequiresControlled, &deadline, &t.CreatedBy, &created, &awarded, &t.CancelReason)
	t.Deadline, t.CreatedAt, t.AwardedAt = ts.Time(deadline), ts.Time(created), ts.PtrTime(awarded)
	return &t, err
}

func (s *server) details(ctx context.Context, t *tmsv1.Tender) error {
	rows, err := s.db.Query(ctx, `SELECT shipment_id, shipment_number, consignee_name, weight_kg, distance_km, temperature_class, controlled
		FROM tender_shipments WHERE tender_id=$1 ORDER BY shipment_number`, t.Id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sh tmsv1.TenderShipment
		if err := rows.Scan(&sh.ShipmentId, &sh.ShipmentNumber, &sh.ConsigneeName, &sh.WeightKg, &sh.DistanceKm, &sh.TemperatureClass, &sh.Controlled); err != nil {
			rows.Close()
			return err
		}
		t.Shipments = append(t.Shipments, &sh)
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT b.id, b.carrier_id, c.name, b.amount, b.transit_days, b.compliant, b.non_compliance_reason, b.source,
		b.note, b.submitted_at FROM bids b JOIN carriers c ON c.id=b.carrier_id WHERE b.tender_id=$1 ORDER BY b.compliant DESC, b.amount`, t.Id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var b tmsv1.Bid
		var at time.Time
		if err := rows.Scan(&b.Id, &b.CarrierId, &b.CarrierName, &b.Amount, &b.TransitDays, &b.Compliant, &b.NonComplianceReason, &b.Source,
			&b.Note, &at); err != nil {
			return err
		}
		b.SubmittedAt = ts.Time(at)
		t.Bids = append(t.Bids, &b)
	}
	return rows.Err()
}

func (s *server) GetTender(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Tender, error) {
	t, err := scanTender(s.db.QueryRow(ctx, `SELECT `+tenderColumns+` FROM tenders WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	if err != nil {
		return nil, errs.DB(err, "tender")
	}
	return t, errs.DB(s.details(ctx, t), "tender details")
}

func (s *server) ListTenders(ctx context.Context, req *tmsv1.ListTendersRequest) (*tmsv1.ListTendersResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT `+tenderColumns+` FROM tenders WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND ($3='' OR status=$3)
		ORDER BY created_at DESC LIMIT 200`, actor.From(ctx).OrgID, req.DepotId, req.Status)
	if err != nil {
		return nil, errs.DB(err, "tenders")
	}
	var list []*tmsv1.Tender
	for rows.Next() {
		t, err := scanTender(rows)
		if err != nil {
			rows.Close()
			return nil, errs.DB(err, "tenders")
		}
		list = append(list, t)
	}
	rows.Close()
	for _, t := range list {
		if err := s.details(ctx, t); err != nil {
			return nil, errs.DB(err, "tender details")
		}
	}
	return &tmsv1.ListTendersResponse{Tenders: list}, nil
}

func (s *server) SubmitBid(ctx context.Context, req *tmsv1.SubmitBidRequest) (*tmsv1.Tender, error) {
	if req.Amount <= 0 {
		return nil, errs.Invalid("bid amount must be positive")
	}
	t, err := s.GetTender(ctx, &tmsv1.IdRequest{Id: req.TenderId})
	if err != nil {
		return nil, err
	}
	if t.Status != "OPEN" {
		return nil, errs.Precondition("tender is %s", t.Status)
	}
	c, err := s.GetCarrier(ctx, &tmsv1.IdRequest{Id: req.CarrierId})
	if err != nil {
		return nil, err
	}
	if req.TransitDays == 0 {
		req.TransitDays = c.TransitDays
	}
	ok, why := compliant(c, t)
	_, err = s.db.Exec(ctx, `INSERT INTO bids(tender_id, carrier_id, amount, transit_days, compliant, non_compliance_reason, source, note)
		VALUES ($1,$2,$3,$4,$5,$6,'MANUAL',$7) ON CONFLICT (tender_id, carrier_id, source) DO UPDATE SET amount=EXCLUDED.amount,
		transit_days=EXCLUDED.transit_days, note=EXCLUDED.note, submitted_at=now()`, t.Id, c.Id, req.Amount, req.TransitDays, ok, why, req.Note)
	if err != nil {
		return nil, errs.DB(err, "bid")
	}
	return s.GetTender(ctx, &tmsv1.IdRequest{Id: t.Id})
}

// AwardTender assigns the tender's shipments to the winning carrier, splitting the bid amount
// across shipments by weight. Non-compliant bids cannot win.
func (s *server) AwardTender(ctx context.Context, req *tmsv1.AwardTenderRequest) (*tmsv1.Tender, error) {
	t, err := s.GetTender(ctx, &tmsv1.IdRequest{Id: req.TenderId})
	if err != nil {
		return nil, err
	}
	if t.Status != "OPEN" {
		return nil, errs.Precondition("tender is %s", t.Status)
	}
	var bid *tmsv1.Bid
	for _, b := range t.Bids {
		if b.Id == req.BidId {
			bid = b
		}
	}
	if bid == nil {
		return nil, errs.NotFound("bid not found on this tender")
	}
	if !bid.Compliant {
		return nil, errs.Precondition("%s cannot be awarded: %s", bid.CarrierName, bid.NonComplianceReason)
	}
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tenders SET status='AWARDED', awarded_bid_id=$2, awarded_at=now() WHERE id=$1 AND status='OPEN'`, t.Id, bid.Id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.Precondition("tender was changed concurrently")
		}
		for _, sh := range t.Shipments {
			share := math.Round(bid.Amount * sh.WeightKg / t.TotalWeightKg)
			if _, err := s.shipments.AssignCarrier(ctx, &tmsv1.AssignCarrierRequest{Id: sh.ShipmentId, CarrierId: bid.CarrierId,
				CarrierName: bid.CarrierName, FreightCost: share, TenderId: t.Id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, errs.DB(err, "award")
	}
	return s.GetTender(ctx, &tmsv1.IdRequest{Id: t.Id})
}

func (s *server) CancelTender(ctx context.Context, req *tmsv1.CancelTenderRequest) (*tmsv1.Tender, error) {
	if req.Reason == "" {
		return nil, errs.Invalid("a cancellation reason is required")
	}
	t, err := s.GetTender(ctx, &tmsv1.IdRequest{Id: req.Id})
	if err != nil {
		return nil, err
	}
	err = db.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tenders SET status='CANCELLED', cancel_reason=$2 WHERE id=$1 AND status='OPEN'`, t.Id, req.Reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.Precondition("only open tenders can be cancelled")
		}
		ids := make([]string, len(t.Shipments))
		for i, sh := range t.Shipments {
			ids[i] = sh.ShipmentId
		}
		_, err = s.shipments.Unplan(ctx, &tmsv1.IdsRequest{Ids: ids})
		return err
	})
	if err != nil {
		return nil, errs.DB(err, "tender cancellation")
	}
	return s.GetTender(ctx, &tmsv1.IdRequest{Id: t.Id})
}

func (s *server) GetStats(ctx context.Context, req *tmsv1.FreightStatsRequest) (*tmsv1.FreightStats, error) {
	org, depot := actor.From(ctx).OrgID, req.DepotId
	out := &tmsv1.FreightStats{}
	err := s.db.QueryRow(ctx, `WITH awarded AS (
			SELECT t.id, b.amount, (SELECT avg(amount) FROM bids WHERE tender_id=t.id AND compliant) AS avg_bid
			FROM tenders t JOIN bids b ON b.id=t.awarded_bid_id
			WHERE t.org_id=$1 AND ($2='' OR t.depot_id::text=$2) AND t.awarded_at >= now() - interval '30 days')
		SELECT
		(SELECT count(*) FROM tenders WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status='OPEN'),
		(SELECT count(*) FROM awarded),
		(SELECT COALESCE(sum(amount),0) FROM awarded)::float8,
		(SELECT COALESCE(avg(100 * (avg_bid - amount) / NULLIF(avg_bid,0)),0) FROM awarded)::float8`, org, depot).Scan(
		&out.OpenTenders, &out.Awarded_30D, &out.Spend_30D, &out.AvgSavingsPct)
	if err != nil {
		return nil, errs.DB(err, "freight stats")
	}
	rows, err := s.db.Query(ctx, `SELECT c.id, c.name, COALESCE(sum(b.amount),0)::float8, COALESCE(sum((SELECT count(*) FROM tender_shipments
		WHERE tender_id=t.id)),0)::bigint, COALESCE(sum(t.total_weight_kg),0)::float8
		FROM carriers c LEFT JOIN tenders t ON t.org_id=c.org_id AND t.status='AWARDED' AND ($2='' OR t.depot_id::text=$2)
			AND t.awarded_bid_id IN (SELECT id FROM bids WHERE carrier_id=c.id)
		LEFT JOIN bids b ON b.id=t.awarded_bid_id
		WHERE c.org_id=$1 GROUP BY c.id, c.name ORDER BY 3 DESC`, org, depot)
	if err != nil {
		return nil, errs.DB(err, "carrier spend")
	}
	defer rows.Close()
	for rows.Next() {
		var c tmsv1.CarrierSpend
		if err := rows.Scan(&c.CarrierId, &c.CarrierName, &c.Spend, &c.Shipments, &c.WeightKg); err != nil {
			return nil, errs.DB(err, "carrier spend")
		}
		out.ByCarrier = append(out.ByCarrier, &c)
	}
	return out, rows.Err()
}
