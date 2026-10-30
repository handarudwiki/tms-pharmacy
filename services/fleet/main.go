// Command fleet runs the fleet service: depots, vehicles, drivers and maintenance.
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

const schema = "fleet"

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx, env.DatabaseURL(), schema)
	grpcx.Must(err, "connect database")
	sub, _ := fs.Sub(migrations, "migrations")
	grpcx.Must(db.Migrate(ctx, pool, schema, sub), "migrate")

	srv := grpcx.NewServer("fleet")
	tmsv1.RegisterFleetServiceServer(srv, &server{db: pool})
	grpcx.Serve(srv, env.Get("FLEET_ADDR", ":9102"))
}
