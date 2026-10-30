package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
)

var loc = func() *time.Location {
	l, err := time.LoadLocation(env.Get("TMS_TZ", "Asia/Jakarta"))
	if err != nil {
		return time.Local
	}
	return l
}()

func depotParam(r *http.Request) (string, error) {
	id := r.URL.Query().Get("depot_id")
	if id == "" {
		return "", status.Error(codes.InvalidArgument, "depot_id is required")
	}
	if !actor.From(r.Context()).CanAccessDepot(id) {
		return "", status.Error(codes.PermissionDenied, "you do not have access to this depot")
	}
	return id, nil
}

// parallel runs independent backend calls concurrently and returns the first error.
func parallel(fns ...func() error) error {
	var wg sync.WaitGroup
	errs := make([]error, len(fns))
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *gateway) dashboard(ctx context.Context, r *http.Request) (any, error) {
	depot, err := depotParam(r)
	if err != nil {
		return nil, err
	}
	today := time.Now().In(loc).Format("2006-01-02")
	var (
		ship    *tmsv1.ShipmentStats
		plan    *tmsv1.PlanningStats
		fleet   *tmsv1.FleetStats
		freight *tmsv1.FreightStats
		trips   *tmsv1.ListTripsResponse
		active  *tmsv1.ListTripsResponse
		exc     *tmsv1.ListExceptionsResponse
		pending *tmsv1.ListShipmentsResponse
	)
	err = parallel(
		func() (err error) {
			ship, err = g.c.shipments.GetStats(ctx, &tmsv1.ShipmentStatsRequest{DepotId: depot})
			return
		},
		func() (err error) {
			plan, err = g.c.planning.GetStats(ctx, &tmsv1.PlanningStatsRequest{DepotId: depot})
			return
		},
		func() (err error) {
			fleet, err = g.c.fleet.GetStats(ctx, &tmsv1.FleetStatsRequest{DepotId: depot})
			return
		},
		func() (err error) {
			freight, err = g.c.freight.GetStats(ctx, &tmsv1.FreightStatsRequest{DepotId: depot})
			return
		},
		func() (err error) {
			trips, err = g.c.planning.ListTrips(ctx, &tmsv1.ListTripsRequest{DepotId: depot, PlanDate: today})
			return
		},
		func() (err error) {
			active, err = g.c.planning.ListTrips(ctx, &tmsv1.ListTripsRequest{DepotId: depot, Statuses: []string{"RELEASED", "IN_PROGRESS"}})
			return
		},
		func() (err error) {
			exc, err = g.c.shipments.ListExceptions(ctx, &tmsv1.ListExceptionsRequest{DepotId: depot, Status: "OPEN", Limit: 6})
			return
		},
		func() (err error) {
			pending, err = g.c.shipments.ListShipments(ctx, &tmsv1.ListShipmentsRequest{DepotId: depot, Status: "PENDING"})
			return
		},
	)
	if err != nil {
		return nil, err
	}
	// Today's trips plus any still running from earlier days, without duplicates.
	seen := map[string]bool{}
	var mapTrips []*tmsv1.Trip
	for _, t := range append(trips.Trips, active.Trips...) {
		if !seen[t.Id] && t.Status != "CANCELLED" {
			seen[t.Id] = true
			mapTrips = append(mapTrips, t)
		}
	}
	var pendingKg float64
	for _, s := range pending.Shipments {
		pendingKg += s.WeightKg
	}
	return map[string]any{
		"shipments":         pj(ship),
		"planning":          pj(plan),
		"fleet":             pj(fleet),
		"freight":           pj(freight),
		"trips":             pjList(mapTrips),
		"exceptions":        pj(exc),
		"pending_shipments": len(pending.Shipments),
		"pending_weight_kg": pendingKg,
	}, nil
}

// driverTrips lists the signed-in driver's current and today's trips (the driver application).
func (g *gateway) driverTrips(ctx context.Context, _ *http.Request) (any, error) {
	a := actor.From(ctx)
	res, err := g.c.planning.ListTrips(ctx, &tmsv1.ListTripsRequest{DriverEmail: a.Email,
		Statuses: []string{"RELEASED", "IN_PROGRESS", "COMPLETED"}, Limit: 20})
	if err != nil {
		return nil, err
	}
	today := time.Now().In(loc).Format("2006-01-02")
	var out []*tmsv1.Trip
	for _, t := range res.Trips {
		if t.Status != "COMPLETED" || t.PlanDate == today {
			out = append(out, t)
		}
	}
	return map[string]any{"trips": pjList(out)}, nil
}
