// Command shipment runs the shipment service: shipments, delivery events, proof of delivery and exceptions.
package main

import (
	"context"
	"embed"
	"io/fs"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/db"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
	"github.com/handarudwiki/pharmacy-tms/pkg/grpcx"
)

//go:embed migrations/*.sql
var migrations embed.FS

const schema = "shipment"

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx, env.DatabaseURL(), schema)
	grpcx.Must(err, "connect database")
	sub, _ := fs.Sub(migrations, "migrations")
	grpcx.Must(db.Migrate(ctx, pool, schema, sub), "migrate")

	srv := grpcx.NewServer("shipment")
	tmsv1.RegisterShipmentServiceServer(srv, &server{db: pool})
	grpcx.Serve(srv, env.Get("SHIPMENT_ADDR", ":9103"))
}
