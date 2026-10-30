package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/token"
)

type gateway struct {
	c   *clients
	mux *http.ServeMux
}

var (
	marshal   = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	unmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

type ctxKey struct{}

// middleware assigns a correlation ID, authenticates the bearer token and logs the request.
func (g *gateway) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		corr := r.Header.Get("X-Request-ID")
		if corr == "" {
			corr = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", corr)
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		a := actor.Actor{CorrelationID: corr}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if verified, err := token.Verify(strings.TrimPrefix(h, "Bearer ")); err == nil {
				verified.CorrelationID = corr
				a = verified
			}
		}
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r.WithContext(actor.With(r.Context(), a)))
		if strings.HasPrefix(r.URL.Path, "/api/") {
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(), "correlation_id", corr, "actor", a.Name())
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// static serves the built web client with an SPA fallback when it is present.
func (g *gateway) static(dir string) {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return
	}
	fs := http.FileServer(http.Dir(dir))
	g.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, filepath.Clean(r.URL.Path))); err != nil {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

type handler func(ctx context.Context, r *http.Request) (any, error)

// route registers a handler guarded by authentication and (optionally) a set of roles.
// Org admins pass every role check. Successful mutations are written to the audit trail.
func (g *gateway) route(pattern, action string, roles []string, h handler) {
	method := strings.SplitN(pattern, " ", 2)[0]
	g.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		a := actor.From(ctx)
		if action != "auth.login" {
			if a.OrgID == "" {
				writeError(w, status.Error(codes.Unauthenticated, "authentication required"))
				return
			}
			if len(roles) > 0 && !a.HasRole(append(roles, actor.RoleAdmin)...) {
				writeError(w, status.Errorf(codes.PermissionDenied, "this action requires one of the roles: %s", strings.Join(roles, ", ")))
				return
			}
		}
		res, err := h(ctx, r)
		if err != nil {
			writeError(w, err)
			return
		}
		if method != http.MethodGet && action != "auth.login" {
			g.audit(ctx, action, r, res)
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (g *gateway) audit(ctx context.Context, action string, r *http.Request, res any) {
	entityType := strings.SplitN(action, ".", 2)[0]
	entityID := r.PathValue("id")
	for _, k := range []string{"trip_id", "tender_id", "stop_id"} {
		if entityID == "" {
			entityID = r.PathValue(k)
		}
	}
	if m, ok := res.(proto.Message); ok && entityID == "" {
		if f := m.ProtoReflect().Descriptor().Fields().ByName("id"); f != nil {
			entityID = m.ProtoReflect().Get(f).String()
		}
	}
	details, _ := json.Marshal(map[string]string{"method": r.Method, "path": r.URL.Path})
	if _, err := g.c.identity.RecordAudit(ctx, &tmsv1.AuditEvent{Action: action, EntityType: entityType, EntityId: entityID,
		Details: string(details)}); err != nil {
		slog.Warn("audit write failed", "action", action, "error", err)
	}
}

// rpc adapts a unary gRPC method to HTTP: the JSON body, path wildcards and query parameters
// are decoded into the request message (by proto field name). The Idempotency-Key header fills
// an idempotency_key field. Depot-scoped requests are checked against the user's access.
func rpc[Req any, Res proto.Message, P interface {
	*Req
	proto.Message
}](call func(context.Context, P, ...grpc.CallOption) (Res, error)) handler {
	return func(ctx context.Context, r *http.Request) (any, error) {
		req := P(new(Req))
		if err := decode(r, req); err != nil {
			return nil, err
		}
		return call(ctx, req)
	}
}

func decode(r *http.Request, msg proto.Message) error {
	if r.Body != nil && r.Method != http.MethodGet {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			return status.Error(codes.InvalidArgument, "cannot read request body")
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			if err := unmarshal.Unmarshal(body, msg); err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid JSON body: %v", err)
			}
		}
	}
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		name := string(f.Name())
		val := r.PathValue(name)
		if val == "" {
			val = r.URL.Query().Get(name)
		}
		if val == "" && name == "idempotency_key" {
			val = r.Header.Get("Idempotency-Key")
		}
		if val == "" || (m.Has(f) && r.PathValue(name) == "") {
			continue
		}
		if err := setField(m, f, val); err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid %s: %v", name, err)
		}
	}
	if f := fields.ByName("depot_id"); f != nil {
		if wh := m.Get(f).String(); !actor.From(r.Context()).CanAccessDepot(wh) {
			return status.Error(codes.PermissionDenied, "you do not have access to this depot")
		}
	}
	return nil
}

func setField(m protoreflect.Message, f protoreflect.FieldDescriptor, val string) error {
	if f.IsList() {
		if f.Kind() != protoreflect.StringKind {
			return nil
		}
		list := m.Mutable(f).List()
		for _, v := range strings.Split(val, ",") {
			if v = strings.TrimSpace(v); v != "" {
				list.Append(protoreflect.ValueOfString(v))
			}
		}
		return nil
	}
	switch f.Kind() {
	case protoreflect.StringKind:
		m.Set(f, protoreflect.ValueOfString(val))
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		m.Set(f, protoreflect.ValueOfBool(b))
	case protoreflect.Int32Kind:
		n, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return err
		}
		m.Set(f, protoreflect.ValueOfInt32(int32(n)))
	case protoreflect.Int64Kind:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		m.Set(f, protoreflect.ValueOfInt64(n))
	case protoreflect.DoubleKind:
		n, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		m.Set(f, protoreflect.ValueOfFloat64(n))
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if m, ok := v.(proto.Message); ok {
		b, err := marshal.Marshal(m)
		if err != nil {
			slog.Error("marshal response", "error", err)
		}
		_, _ = w.Write(b)
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

var httpStatus = map[codes.Code]int{
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.NotFound:           http.StatusNotFound,
	codes.AlreadyExists:      http.StatusConflict,
	codes.FailedPrecondition: http.StatusUnprocessableEntity,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.Unauthenticated:    http.StatusUnauthorized,
	codes.Unavailable:        http.StatusServiceUnavailable,
	codes.DeadlineExceeded:   http.StatusGatewayTimeout,
}

func writeError(w http.ResponseWriter, err error) {
	st := status.Convert(err)
	code, ok := httpStatus[st.Code()]
	if !ok {
		code = http.StatusInternalServerError
	}
	msg := st.Message()
	if code == http.StatusInternalServerError {
		slog.Error("internal error", "error", msg)
		msg = "internal server error"
	}
	if st.Code() == codes.Unavailable {
		msg = "a backend service is unavailable, please retry"
	}
	writeJSON(w, code, map[string]string{"error": msg, "code": st.Code().String()})
}
