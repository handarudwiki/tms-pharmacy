// Package grpcx wires the cross-cutting concerns shared by every service: internal
// service authentication, actor and correlation-ID propagation, logging and recovery.
package grpcx

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
)

const (
	mdToken       = "x-internal-token"
	mdOrg         = "x-org-id"
	mdUser        = "x-user-id"
	mdEmail       = "x-user-email"
	mdRoles       = "x-user-roles"
	mdDepots      = "x-depot-ids"
	mdCorrelation = "x-correlation-id"
)

// NewServer builds a gRPC server that rejects calls without the internal service token
// and exposes the calling actor through actor.From(ctx).
func NewServer(service string, publicMethods ...string) *grpc.Server {
	token := env.InternalToken()
	log := slog.With("service", service)
	return grpc.NewServer(grpc.ChainUnaryInterceptor(
		func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			start := time.Now()
			md, _ := metadata.FromIncomingContext(ctx)
			a := actor.Actor{
				OrgID:         first(md, mdOrg),
				UserID:        first(md, mdUser),
				Email:         first(md, mdEmail),
				Roles:         split(first(md, mdRoles)),
				DepotIDs:      split(first(md, mdDepots)),
				CorrelationID: first(md, mdCorrelation),
			}
			if a.CorrelationID == "" {
				a.CorrelationID = uuid.NewString()
			}
			defer func() {
				if r := recover(); r != nil {
					log.Error("panic", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
					err = status.Error(codes.Internal, "internal error")
				}
				lvl := slog.LevelInfo
				if err != nil && status.Code(err) == codes.Internal {
					lvl = slog.LevelError
				}
				log.Log(ctx, lvl, "rpc", "method", info.FullMethod, "code", status.Code(err).String(),
					"duration_ms", time.Since(start).Milliseconds(), "correlation_id", a.CorrelationID,
					"org_id", a.OrgID, "actor", a.Name(), "error", errString(err))
			}()
			if subtle.ConstantTimeCompare([]byte(first(md, mdToken)), []byte(token)) != 1 {
				return nil, status.Error(codes.Unauthenticated, "invalid service token")
			}
			if a.OrgID == "" && !contains(publicMethods, info.FullMethod) {
				return nil, status.Error(codes.Unauthenticated, "missing organization context")
			}
			return handler(actor.With(ctx, a), req)
		},
	))
}

// Dial connects to another service, forwarding the actor of the current request.
func Dial(addr string) (*grpc.ClientConn, error) {
	token := env.InternalToken()
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(OutgoingContext(ctx, token), method, req, reply, cc, opts...)
		}),
	)
}

// OutgoingContext attaches the actor from ctx and the service token as gRPC metadata.
func OutgoingContext(ctx context.Context, token string) context.Context {
	a := actor.From(ctx)
	return metadata.AppendToOutgoingContext(ctx,
		mdToken, token,
		mdOrg, a.OrgID,
		mdUser, a.UserID,
		mdEmail, a.Email,
		mdRoles, strings.Join(a.Roles, ","),
		mdDepots, strings.Join(a.DepotIDs, ","),
		mdCorrelation, a.CorrelationID,
	)
}

// Serve listens on addr and stops gracefully on SIGINT/SIGTERM.
func Serve(srv *grpc.Server, addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "addr", addr, "error", err)
		os.Exit(1)
	}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		srv.GracefulStop()
	}()
	slog.Info("grpc server listening", "addr", addr)
	if err := srv.Serve(lis); err != nil {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}

// Must aborts startup on unrecoverable configuration errors.
func Must(err error, msg string) {
	if err != nil {
		slog.Error(msg, "error", err)
		os.Exit(1)
	}
}

func first(md metadata.MD, key string) string {
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}
