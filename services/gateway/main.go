// Command gateway exposes the TMS services as a JSON/HTTP API for the web client.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
	"github.com/handarudwiki/pharmacy-tms/pkg/grpcx"
)

type clients struct {
	identity  tmsv1.IdentityServiceClient
	fleet     tmsv1.FleetServiceClient
	shipments tmsv1.ShipmentServiceClient
	planning  tmsv1.PlanningServiceClient
	freight   tmsv1.FreightServiceClient
}

func main() {
	dial := func(key, def string) *grpc.ClientConn {
		conn, err := grpcx.Dial(env.Get(key, def))
		grpcx.Must(err, "dial "+key)
		return conn
	}
	c := &clients{
		identity:  tmsv1.NewIdentityServiceClient(dial("IDENTITY_TARGET", "localhost:9101")),
		fleet:     tmsv1.NewFleetServiceClient(dial("FLEET_TARGET", "localhost:9102")),
		shipments: tmsv1.NewShipmentServiceClient(dial("SHIPMENT_TARGET", "localhost:9103")),
		planning:  tmsv1.NewPlanningServiceClient(dial("PLANNING_TARGET", "localhost:9104")),
		freight:   tmsv1.NewFreightServiceClient(dial("FREIGHT_TARGET", "localhost:9105")),
	}
	g := &gateway{c: c, mux: http.NewServeMux()}
	g.routes()
	g.static(env.Get("WEB_DIR", "web/dist"))

	srv := &http.Server{Addr: env.Get("GATEWAY_ADDR", ":8090"), Handler: g.middleware(g.mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	slog.Info("gateway listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		grpcx.Must(err, "serve http")
	}
}
