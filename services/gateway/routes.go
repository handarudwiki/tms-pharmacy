package main

import (
	"context"
	"net/http"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
)

// Role groups for authorization. Org admins are always allowed.
var (
	anyone      []string
	admins      = []string{actor.RoleAdmin}
	managers    = []string{actor.RoleTransportMgr}
	fleetMgrs   = []string{actor.RoleFleetMgr, actor.RoleTransportMgr}
	planners    = []string{actor.RoleDispatcher, actor.RoleTransportMgr}
	shipEntry   = []string{actor.RoleDispatcher, actor.RoleTransportMgr, actor.RoleCustomerService, actor.RoleIntegration}
	drivers     = []string{actor.RoleDriver, actor.RoleDispatcher, actor.RoleTransportMgr}
	carrierMgrs = []string{actor.RoleCarrierMgr, actor.RoleTransportMgr}
	resolvers   = []string{actor.RoleTransportMgr, actor.RoleDispatcher, actor.RoleCustomerService}
	deliverers  = []string{actor.RoleDispatcher, actor.RoleTransportMgr, actor.RoleCustomerService}
	auditors    = []string{actor.RoleTransportMgr, actor.RoleAnalyst}
	integrators = []string{actor.RoleIntegration, actor.RoleTransportMgr}
)

func (g *gateway) routes() {
	c := g.c
	g.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Identity & audit
	g.route("POST /api/auth/login", "auth.login", anyone, rpc(c.identity.Login))
	g.route("GET /api/me", "", anyone, func(ctx context.Context, _ *http.Request) (any, error) {
		return c.identity.GetUser(ctx, &tmsv1.IdRequest{Id: actor.From(ctx).UserID})
	})
	g.route("GET /api/roles", "", anyone, func(context.Context, *http.Request) (any, error) {
		return map[string]any{"roles": actor.AllRoles}, nil
	})
	g.route("GET /api/users", "", admins, rpc(c.identity.ListUsers))
	g.route("POST /api/users", "user.create", admins, rpc(c.identity.CreateUser))
	g.route("PUT /api/users/{id}", "user.update", admins, rpc(c.identity.UpdateUser))
	g.route("GET /api/audit", "", auditors, rpc(c.identity.ListAudit))

	// Fleet
	g.route("GET /api/depots", "", anyone, rpc(c.fleet.ListDepots))
	g.route("POST /api/depots", "depot.create", admins, rpc(c.fleet.CreateDepot))
	g.route("GET /api/vehicles", "", anyone, rpc(c.fleet.ListVehicles))
	g.route("POST /api/vehicles", "vehicle.create", fleetMgrs, rpc(c.fleet.UpsertVehicle))
	g.route("PUT /api/vehicles/{id}", "vehicle.update", fleetMgrs, rpc(c.fleet.UpsertVehicle))
	g.route("POST /api/vehicles/{id}/status", "vehicle.set_status", fleetMgrs, rpc(c.fleet.SetVehicleStatus))
	g.route("GET /api/drivers", "", anyone, rpc(c.fleet.ListDrivers))
	g.route("POST /api/drivers", "driver.create", fleetMgrs, rpc(c.fleet.UpsertDriver))
	g.route("PUT /api/drivers/{id}", "driver.update", fleetMgrs, rpc(c.fleet.UpsertDriver))
	g.route("POST /api/drivers/{id}/status", "driver.set_status", fleetMgrs, rpc(c.fleet.SetDriverStatus))
	g.route("GET /api/maintenance", "", anyone, rpc(c.fleet.ListMaintenance))
	g.route("POST /api/maintenance", "maintenance.record", fleetMgrs, rpc(c.fleet.RecordMaintenance))

	// Shipments
	g.route("GET /api/shipments", "", anyone, rpc(c.shipments.ListShipments))
	g.route("POST /api/shipments", "shipment.create", shipEntry, rpc(c.shipments.CreateShipment))
	g.route("GET /api/shipments/{id}", "", anyone, rpc(c.shipments.GetShipment))
	g.route("GET /api/shipments/{id}/events", "", anyone, rpc(c.shipments.ListEvents))
	g.route("POST /api/shipments/{id}/cancel", "shipment.cancel", planners, rpc(c.shipments.CancelShipment))
	g.route("POST /api/shipments/{id}/deliver", "shipment.deliver", deliverers, rpc(c.shipments.RecordDelivery))
	g.route("GET /api/exceptions", "", anyone, rpc(c.shipments.ListExceptions))
	g.route("POST /api/exceptions/{id}/resolve", "exception.resolve", resolvers, rpc(c.shipments.ResolveException))

	// Planning & execution
	g.route("POST /api/planning/auto", "trip.auto_plan", planners, rpc(c.planning.AutoPlan))
	g.route("GET /api/trips", "", anyone, rpc(c.planning.ListTrips))
	g.route("POST /api/trips", "trip.create", planners, rpc(c.planning.CreateTrip))
	g.route("GET /api/trips/{id}", "", anyone, rpc(c.planning.GetTrip))
	g.route("POST /api/trips/{id}/optimize", "trip.optimize", planners, rpc(c.planning.OptimizeTrip))
	g.route("POST /api/trips/{id}/release", "trip.release", planners, rpc(c.planning.ReleaseTrip))
	g.route("POST /api/trips/{id}/cancel", "trip.cancel", planners, rpc(c.planning.CancelTrip))
	g.route("POST /api/trips/{id}/start", "trip.start", drivers, rpc(c.planning.StartTrip))
	g.route("POST /api/trips/{id}/complete", "trip.complete", drivers, rpc(c.planning.CompleteTrip))
	g.route("POST /api/trips/{trip_id}/stops/{stop_id}/arrive", "stop.arrive", drivers, rpc(c.planning.ArriveStop))
	g.route("POST /api/trips/{trip_id}/stops/{stop_id}/deliver", "stop.deliver", drivers, rpc(c.planning.CompleteStop))
	g.route("POST /api/trips/{trip_id}/stops/{stop_id}/fail", "stop.fail", drivers, rpc(c.planning.FailStop))
	g.route("POST /api/trips/{trip_id}/telemetry", "", drivers, rpc(c.planning.RecordTelemetry))
	g.route("GET /api/trips/{trip_id}/telemetry", "", anyone, rpc(c.planning.ListTelemetry))
	g.route("GET /api/driver/trips", "", drivers, g.driverTrips)

	// Freight
	g.route("GET /api/carriers", "", anyone, rpc(c.freight.ListCarriers))
	g.route("POST /api/carriers", "carrier.create", carrierMgrs, rpc(c.freight.UpsertCarrier))
	g.route("PUT /api/carriers/{id}", "carrier.update", carrierMgrs, rpc(c.freight.UpsertCarrier))
	g.route("GET /api/tenders", "", anyone, rpc(c.freight.ListTenders))
	g.route("POST /api/tenders", "tender.create", carrierMgrs, rpc(c.freight.CreateTender))
	g.route("GET /api/tenders/{id}", "", anyone, rpc(c.freight.GetTender))
	g.route("POST /api/tenders/{tender_id}/bids", "tender.bid", carrierMgrs, rpc(c.freight.SubmitBid))
	g.route("POST /api/tenders/{tender_id}/award", "tender.award", carrierMgrs, rpc(c.freight.AwardTender))
	g.route("POST /api/tenders/{id}/cancel", "tender.cancel", carrierMgrs, rpc(c.freight.CancelTender))

	// Cross-service views and integrations
	g.route("GET /api/dashboard", "", anyone, g.dashboard)
	g.route("POST /api/integrations/wms/sync", "integration.wms_sync", integrators, g.syncWMS)
}
