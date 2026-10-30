// Command identity runs the identity service: users, roles, login and audit trail.
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

const schema = "identity"

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx, env.DatabaseURL(), schema)
	grpcx.Must(err, "connect database")
	sub, _ := fs.Sub(migrations, "migrations")
	grpcx.Must(db.Migrate(ctx, pool, schema, sub), "migrate")

	s := &server{db: pool}
	grpcx.Must(s.bootstrap(ctx), "bootstrap organization")

	srv := grpcx.NewServer("identity", tmsv1.IdentityService_Login_FullMethodName)
	tmsv1.RegisterIdentityServiceServer(srv, s)
	grpcx.Serve(srv, env.Get("IDENTITY_ADDR", ":9101"))
}
