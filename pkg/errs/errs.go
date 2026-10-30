// Package errs maps domain and database failures onto gRPC status codes.
package errs

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Invalid(format string, a ...any) error {
	return status.Errorf(codes.InvalidArgument, format, a...)
}

func NotFound(format string, a ...any) error {
	return status.Errorf(codes.NotFound, format, a...)
}

func Precondition(format string, a ...any) error {
	return status.Errorf(codes.FailedPrecondition, format, a...)
}

func Denied(format string, a ...any) error {
	return status.Errorf(codes.PermissionDenied, format, a...)
}

func Conflict(format string, a ...any) error {
	return status.Errorf(codes.AlreadyExists, format, a...)
}

// DB converts a database error into a gRPC error, describing what was being accessed.
func DB(err error, what string) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok && status.Code(err) != codes.Unknown {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("%s not found", what)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return Conflict("%s already exists", what)
		case "23514":
			return Precondition("%s violates an inventory invariant (%s)", what, pgErr.ConstraintName)
		case "23503":
			return Invalid("%s references an unknown record", what)
		case "22P02", "22007", "22008":
			return Invalid("invalid value for %s", what)
		}
	}
	return status.Error(codes.Internal, fmt.Sprintf("%s: %v", what, err))
}
