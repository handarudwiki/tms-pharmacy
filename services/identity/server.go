package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/token"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

type server struct {
	tmsv1.UnimplementedIdentityServiceServer
	db *pgxpool.Pool
}

const userColumns = `id, org_id, email, name, roles, depot_ids, active, created_at`

// bootstrap creates the first organization and its administrator on an empty database.
func (s *server) bootstrap(ctx context.Context) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&n); err != nil || n > 0 {
		return err
	}
	email := env.Get("TMS_ADMIN_EMAIL", "admin@tms.local")
	hash, err := bcrypt.GenerateFromPassword([]byte(env.Get("TMS_ADMIN_PASSWORD", "admin123")), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var orgID string
	if err := s.db.QueryRow(ctx, `INSERT INTO organizations(name) VALUES ($1) RETURNING id`,
		env.Get("TMS_ORG_NAME", "Sehat Pharma Distribution")).Scan(&orgID); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO users(org_id, email, name, password_hash, roles) VALUES ($1,$2,$3,$4,$5)`,
		orgID, email, "Organization Admin", string(hash), []string{actor.RoleAdmin})
	slog.Info("bootstrapped organization", "admin", email)
	return err
}

func scanUser(row pgx.Row) (*tmsv1.User, error) {
	var u tmsv1.User
	var created time.Time
	err := row.Scan(&u.Id, &u.OrgId, &u.Email, &u.Name, &u.Roles, &u.DepotIds, &u.Active, &created)
	u.CreatedAt = ts.Time(created)
	return &u, err
}

func (s *server) Login(ctx context.Context, req *tmsv1.LoginRequest) (*tmsv1.LoginResponse, error) {
	var hash, orgName string
	row := s.db.QueryRow(ctx, `SELECT u.password_hash, o.name, `+prefixed("u", userColumns)+`
		FROM users u JOIN organizations o ON o.id = u.org_id WHERE lower(u.email) = lower($1)`, strings.TrimSpace(req.Email))
	var u tmsv1.User
	var created time.Time
	err := row.Scan(&hash, &orgName, &u.Id, &u.OrgId, &u.Email, &u.Name, &u.Roles, &u.DepotIds, &u.Active, &created)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid email or password")
	}
	if !u.Active {
		return nil, status.Error(codes.PermissionDenied, "user is deactivated")
	}
	u.CreatedAt = ts.Time(created)
	tok, exp, err := token.Issue(actor.Actor{OrgID: u.OrgId, UserID: u.Id, Email: u.Email, Roles: u.Roles, DepotIDs: u.DepotIds})
	if err != nil {
		return nil, err
	}
	return &tmsv1.LoginResponse{Token: tok, ExpiresAt: ts.Time(exp), User: &u, OrgName: orgName}, nil
}

func (s *server) GetUser(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	return u, errs.DB(err, "user")
}

func (s *server) ListUsers(ctx context.Context, _ *tmsv1.Empty) (*tmsv1.ListUsersResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE org_id=$1 ORDER BY name`, actor.From(ctx).OrgID)
	if err != nil {
		return nil, errs.DB(err, "users")
	}
	defer rows.Close()
	out := &tmsv1.ListUsersResponse{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, errs.DB(err, "users")
		}
		out.Users = append(out.Users, u)
	}
	return out, rows.Err()
}

func validRoles(roles []string) error {
	if len(roles) == 0 {
		return errs.Invalid("at least one role is required")
	}
	for _, r := range roles {
		if !slices.Contains(actor.AllRoles, r) {
			return errs.Invalid("unknown role %q", r)
		}
	}
	return nil
}

func (s *server) CreateUser(ctx context.Context, req *tmsv1.CreateUserRequest) (*tmsv1.User, error) {
	if req.Email == "" || req.Name == "" || len(req.Password) < 6 {
		return nil, errs.Invalid("email, name and a password of at least 6 characters are required")
	}
	if err := validRoles(req.Roles); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	whs := req.DepotIds
	if whs == nil {
		whs = []string{}
	}
	u, err := scanUser(s.db.QueryRow(ctx, `INSERT INTO users(org_id, email, name, password_hash, roles, depot_ids)
		VALUES ($1, lower($2), $3, $4, $5, $6) RETURNING `+userColumns,
		actor.From(ctx).OrgID, strings.TrimSpace(req.Email), req.Name, string(hash), req.Roles, whs))
	return u, errs.DB(err, "user")
}

func (s *server) UpdateUser(ctx context.Context, req *tmsv1.UpdateUserRequest) (*tmsv1.User, error) {
	if err := validRoles(req.Roles); err != nil {
		return nil, err
	}
	a := actor.From(ctx)
	if req.Id == a.UserID && (!req.Active || !slices.Contains(req.Roles, actor.RoleAdmin)) && a.HasRole(actor.RoleAdmin) {
		return nil, errs.Precondition("you cannot deactivate yourself or remove your own administrator role")
	}
	whs := req.DepotIds
	if whs == nil {
		whs = []string{}
	}
	var hash any
	if req.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hash = string(h)
	}
	u, err := scanUser(s.db.QueryRow(ctx, `UPDATE users SET name=COALESCE(NULLIF($3,''), name), roles=$4, depot_ids=$5,
		active=$6, password_hash=COALESCE($7, password_hash) WHERE id=$1 AND org_id=$2 RETURNING `+userColumns,
		req.Id, a.OrgID, req.Name, req.Roles, whs, req.Active, hash))
	return u, errs.DB(err, "user")
}

func (s *server) RecordAudit(ctx context.Context, ev *tmsv1.AuditEvent) (*tmsv1.Empty, error) {
	a := actor.From(ctx)
	details := ev.Details
	if details == "" || !json.Valid([]byte(details)) {
		b, _ := json.Marshal(map[string]string{"message": details})
		details = string(b)
	}
	corr := ev.CorrelationId
	if corr == "" {
		corr = a.CorrelationID
	}
	_, err := s.db.Exec(ctx, `INSERT INTO audit_events(org_id, actor_id, actor_email, action, entity_type, entity_id, correlation_id, details)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, a.OrgID, a.UserID, a.Email, ev.Action, ev.EntityType, ev.EntityId, corr, details)
	return &tmsv1.Empty{}, errs.DB(err, "audit event")
}

func (s *server) ListAudit(ctx context.Context, req *tmsv1.ListAuditRequest) (*tmsv1.ListAuditResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id, actor_id, actor_email, action, entity_type, entity_id, correlation_id, details::text, created_at
		FROM audit_events WHERE org_id=$1
		AND ($2 = '' OR entity_type=$2) AND ($3 = '' OR entity_id=$3) AND ($4 = '' OR actor_id=$4)
		ORDER BY created_at DESC LIMIT $5`, actor.From(ctx).OrgID, req.EntityType, req.EntityId, req.ActorId, limit)
	if err != nil {
		return nil, errs.DB(err, "audit events")
	}
	defer rows.Close()
	out := &tmsv1.ListAuditResponse{}
	for rows.Next() {
		var e tmsv1.AuditEvent
		var created time.Time
		if err := rows.Scan(&e.Id, &e.ActorId, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityId, &e.CorrelationId, &e.Details, &created); err != nil {
			return nil, errs.DB(err, "audit events")
		}
		e.CreatedAt = ts.Time(created)
		out.Events = append(out.Events, &e)
	}
	return out, rows.Err()
}

func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		parts[i] = alias + "." + p
	}
	return strings.Join(parts, ", ")
}
