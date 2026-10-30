// Command planning runs the planning service: trip planning, route optimization, execution and telemetry.
package main

import (
	"context"
	"embed"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
	"github.com/handarudwiki/pharmacy-tms/pkg/grpcx"
)

//go:embed migrations/*.sql
var migrations embed.FS

const schema = "planning"

type server struct {
	tmsv1.UnimplementedPlanningServiceServer
	db        *pgxpool.Pool
	fleet     tmsv1.FleetServiceClient
	shipments tmsv1.ShipmentServiceClient
}

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx, env.DatabaseURL(), schema)
	grpcx.Must(err, "connect database")
	sub, _ := fs.Sub(migrations, "migrations")
	grpcx.Must(db.Migrate(ctx, pool, schema, sub), "migrate")

	fl, err := grpcx.Dial(env.Get("FLEET_TARGET", "localhost:9102"))
	grpcx.Must(err, "dial fleet")
	sh, err := grpcx.Dial(env.Get("SHIPMENT_TARGET", "localhost:9103"))
	grpcx.Must(err, "dial shipment")

	srv := grpcx.NewServer("planning")
	tmsv1.RegisterPlanningServiceServer(srv, &server{db: pool, fleet: tmsv1.NewFleetServiceClient(fl), shipments: tmsv1.NewShipmentServiceClient(sh)})
	grpcx.Serve(srv, env.Get("PLANNING_ADDR", ":9104"))
}
