package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
)

// Known delivery points of the demo customers. A production deployment would call a geocoding
// service; unknown addresses fall back to an approximate point that dispatchers can correct.
var knownLocations = map[string][2]float64{
	"apotek k-24 kemang":       {-6.2607, 106.8136},
	"rs siloam lippo village":  {-6.2263, 106.6000},
	"guardian grand indonesia": {-6.1951, 106.8199},
	"klinik pratama sehat":     {-6.2108, 106.8497},
	"apotek century senayan":   {-6.2272, 106.7990},
	"rsud tangerang":           {-6.1745, 106.6337},
	"kimia farma apotek 201":   {-6.1862, 106.8340},
	"watsons pondok indah":     {-6.2655, 106.7838},
	"rs mitra keluarga bekasi": {-6.2471, 107.0003},
	"puskesmas cilandak":       {-6.2915, 106.7997},
	"apotek roxy cempaka":      {-6.1667, 106.8710},
	"rs hermina depok":         {-6.3910, 106.8250},
	"boots pacific place":      {-6.2243, 106.8096},
	"klinik medika plaza":      {-6.2260, 106.8300},
	"rscm kencana":             {-6.1966, 106.8473},
	"apotek bunda":             {-6.2400, 106.8650},
	"apotek sehat sentosa":     {-6.2800, 106.7960},
}

func geocode(name, address string) (lat, lng float64, approximate bool) {
	if p, ok := knownLocations[strings.ToLower(strings.TrimSpace(name))]; ok {
		return p[0], p[1], false
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name + "|" + address)))
	v := h.Sum32()
	return -6.21 + (float64(v%1600)/10000 - 0.08), 106.83 + (float64((v/1600)%1600)/10000 - 0.08), true
}

type wmsEvent struct {
	ID          string `json:"id"`
	EventType   string `json:"event_type"`
	AggregateID string `json:"aggregate_id"`
	Payload     string `json:"payload"`
}

type readyPayload struct {
	OrderNumber      string  `json:"order_number"`
	Customer         string  `json:"customer"`
	ShipTo           string  `json:"ship_to"`
	Packages         int32   `json:"packages"`
	RequiredDate     string  `json:"required_date"`
	Priority         int32   `json:"priority"`
	TotalWeightKg    float64 `json:"total_weight_kg"`
	TotalVolumeL     float64 `json:"total_volume_l"`
	TemperatureClass string  `json:"temperature_class"`
	Controlled       bool    `json:"controlled"`
}

func wmsCall(ctx context.Context, method, url, token string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return status.Errorf(codes.Unavailable, "WMS is unreachable at %s", url)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return status.Errorf(codes.FailedPrecondition, "WMS answered %d for %s", res.StatusCode, url)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// syncWMS pulls `shipment.ready` events from the WMS integration outbox and turns each into a
// TMS shipment. Imports are keyed by the WMS order number, so syncing twice never duplicates
// a shipment (BR-06).
func (g *gateway) syncWMS(ctx context.Context, r *http.Request) (any, error) {
	var body struct {
		DepotID string `json:"depot_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	depots, err := g.c.fleet.ListDepots(ctx, &tmsv1.Empty{})
	if err != nil {
		return nil, err
	}
	var depot *tmsv1.Depot
	for _, d := range depots.Depots {
		if d.Id == body.DepotID || (body.DepotID == "" && d.Code == env.Get("WMS_DEPOT_CODE", "CKR")) {
			depot = d
		}
	}
	if depot == nil {
		return nil, status.Error(codes.FailedPrecondition, "no depot is configured to receive WMS shipments")
	}
	base := strings.TrimRight(env.Get("WMS_API_URL", "http://localhost:8080"), "/")
	var login struct {
		Token string `json:"token"`
	}
	if err := wmsCall(ctx, "POST", base+"/api/auth/login", "", map[string]string{
		"email": env.Get("WMS_SYNC_EMAIL", "integration@wms.local"), "password": env.Get("WMS_SYNC_PASSWORD", "password123"),
	}, &login); err != nil {
		return nil, err
	}
	var events struct {
		Events []wmsEvent `json:"events"`
	}
	if err := wmsCall(ctx, "GET", base+"/api/integration/events?limit=500", login.Token, nil, &events); err != nil {
		return nil, err
	}
	existing, err := g.c.shipments.ListShipments(ctx, &tmsv1.ListShipmentsRequest{Limit: 1000})
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, s := range existing.Shipments {
		known[s.ExternalRef] = true
	}
	withdrawn := map[string]bool{}
	for _, e := range events.Events {
		if e.EventType == "shipment.withdrawn" || e.EventType == "shipment.dispatched" {
			withdrawn[e.AggregateID] = true
		}
	}
	result := map[string]any{"events_seen": len(events.Events), "imported": 0, "already_imported": 0, "skipped": 0, "depot": depot.Code}
	var imported []string
	for _, e := range events.Events {
		if e.EventType != "shipment.ready" {
			continue
		}
		var p readyPayload
		if err := json.Unmarshal([]byte(e.Payload), &p); err != nil || p.OrderNumber == "" {
			result["skipped"] = result["skipped"].(int) + 1
			continue
		}
		ref := "WMS:" + p.OrderNumber
		switch {
		case known[ref]:
			result["already_imported"] = result["already_imported"].(int) + 1
			continue
		case withdrawn[e.AggregateID]:
			// Already handed over by the WMS (or packing reopened) — nothing left to plan.
			result["skipped"] = result["skipped"].(int) + 1
			continue
		}
		lat, lng, approx := geocode(p.Customer, p.ShipTo)
		weight := p.TotalWeightKg
		if weight <= 0 {
			weight = float64(max(p.Packages, 1)) * 5
		}
		required := p.RequiredDate
		if required == "" || required < time.Now().In(loc).Format("2006-01-02") {
			required = time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
		}
		s, err := g.c.shipments.CreateShipment(ctx, &tmsv1.CreateShipmentRequest{ExternalRef: ref, Source: "WMS", DepotId: depot.Id,
			ConsigneeName: p.Customer, Address: firstNonEmpty(p.ShipTo, p.Customer), City: "Jakarta", Lat: lat, Lng: lng,
			LocationApproximate: approx, RequiredDate: required, WeightKg: weight, VolumeL: p.TotalVolumeL, Packages: max(p.Packages, 1),
			TemperatureClass: firstNonEmpty(p.TemperatureClass, "AMBIENT"), Controlled: p.Controlled, Priority: clamp(p.Priority),
			Notes: fmt.Sprintf("WMS order %s", p.OrderNumber)})
		if err != nil {
			return nil, err
		}
		known[ref] = true
		imported = append(imported, s.ShipmentNumber)
		result["imported"] = result["imported"].(int) + 1
	}
	result["shipments"] = imported
	return result, nil
}

func clamp(p int32) int32 {
	if p < 1 || p > 5 {
		return 3
	}
	return p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
