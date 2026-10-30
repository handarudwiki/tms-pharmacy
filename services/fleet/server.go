package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	tmsv1 "github.com/handarudwiki/pharmacy-tms/gen/tms/v1"
	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/errs"
	"github.com/handarudwiki/pharmacy-tms/pkg/ts"
)

type server struct {
	tmsv1.UnimplementedFleetServiceServer
	db *pgxpool.Pool
}

var (
	vehicleTypes    = []string{"MOTORBIKE", "VAN", "BOX_TRUCK", "REEFER_VAN", "REEFER_TRUCK"}
	vehicleStatuses = []string{"AVAILABLE", "MAINTENANCE", "INACTIVE"}
	driverStatuses  = []string{"ACTIVE", "ON_LEAVE", "INACTIVE"}
)

func ids(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- depots ---

const depotColumns = `id, code, name, address, lat, lng, created_at`

func scanDepot(row pgx.Row) (*tmsv1.Depot, error) {
	var d tmsv1.Depot
	var created time.Time
	err := row.Scan(&d.Id, &d.Code, &d.Name, &d.Address, &d.Lat, &d.Lng, &created)
	d.CreatedAt = ts.Time(created)
	return &d, err
}

func (s *server) CreateDepot(ctx context.Context, req *tmsv1.Depot) (*tmsv1.Depot, error) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" || req.Name == "" || req.Lat < -90 || req.Lat > 90 || req.Lng < -180 || req.Lng > 180 || (req.Lat == 0 && req.Lng == 0) {
		return nil, errs.Invalid("depot code, name and valid coordinates are required")
	}
	d, err := scanDepot(s.db.QueryRow(ctx, `INSERT INTO depots(org_id, code, name, address, lat, lng) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+depotColumns, actor.From(ctx).OrgID, req.Code, req.Name, req.Address, req.Lat, req.Lng))
	return d, errs.DB(err, "depot")
}

func (s *server) ListDepots(ctx context.Context, _ *tmsv1.Empty) (*tmsv1.ListDepotsResponse, error) {
	a := actor.From(ctx)
	rows, err := s.db.Query(ctx, `SELECT `+depotColumns+` FROM depots WHERE org_id=$1 ORDER BY code`, a.OrgID)
	if err != nil {
		return nil, errs.DB(err, "depots")
	}
	list, err := collect(rows, scanDepot)
	out := &tmsv1.ListDepotsResponse{}
	for _, d := range list {
		if a.CanAccessDepot(d.Id) {
			out.Depots = append(out.Depots, d)
		}
	}
	return out, errs.DB(err, "depots")
}

func (s *server) GetDepot(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Depot, error) {
	d, err := scanDepot(s.db.QueryRow(ctx, `SELECT `+depotColumns+` FROM depots WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	return d, errs.DB(err, "depot")
}

// --- vehicles ---

const vehicleColumns = `id, depot_id, plate, vehicle_type, capacity_kg, capacity_volume_l, temperature_class, controlled_certified,
	status, odometer_km, fuel_km_per_liter, next_service_date, status_reason, created_at`

func scanVehicle(row pgx.Row) (*tmsv1.Vehicle, error) {
	var v tmsv1.Vehicle
	var next pgtype.Date
	var created time.Time
	err := row.Scan(&v.Id, &v.DepotId, &v.Plate, &v.VehicleType, &v.CapacityKg, &v.CapacityVolumeL, &v.TemperatureClass,
		&v.ControlledCertified, &v.Status, &v.OdometerKm, &v.FuelKmPerLiter, &next, &v.StatusReason, &created)
	v.NextServiceDate, v.CreatedAt = ts.Date(next), ts.Time(created)
	return &v, err
}

// UpsertVehicle registers a vehicle, or updates it when an id is given. Refrigerated vehicle
// types always carry the COLD temperature class.
func (s *server) UpsertVehicle(ctx context.Context, req *tmsv1.Vehicle) (*tmsv1.Vehicle, error) {
	req.Plate = strings.ToUpper(strings.TrimSpace(req.Plate))
	if req.FuelKmPerLiter == 0 {
		req.FuelKmPerLiter = 8
	}
	switch {
	case req.Plate == "" || req.DepotId == "":
		return nil, errs.Invalid("plate and depot are required")
	case !slices.Contains(vehicleTypes, req.VehicleType):
		return nil, errs.Invalid("unknown vehicle type %q", req.VehicleType)
	case req.CapacityKg <= 0 || req.CapacityVolumeL <= 0 || req.FuelKmPerLiter <= 0:
		return nil, errs.Invalid("capacities and fuel efficiency must be positive")
	}
	req.TemperatureClass = "AMBIENT"
	if strings.HasPrefix(req.VehicleType, "REEFER") {
		req.TemperatureClass = "COLD"
	}
	next, err := ts.ParseDate(req.NextServiceDate)
	if err != nil {
		return nil, errs.Invalid("next service date must be YYYY-MM-DD")
	}
	org := actor.From(ctx).OrgID
	var v *tmsv1.Vehicle
	if req.Id == "" {
		v, err = scanVehicle(s.db.QueryRow(ctx, `INSERT INTO vehicles(org_id, depot_id, plate, vehicle_type, capacity_kg, capacity_volume_l,
			temperature_class, controlled_certified, odometer_km, fuel_km_per_liter, next_service_date)
			SELECT $1, d.id, $3,$4,$5,$6,$7,$8,$9,$10,$11 FROM depots d WHERE d.id=$2 AND d.org_id=$1 RETURNING `+vehicleColumns,
			org, req.DepotId, req.Plate, req.VehicleType, req.CapacityKg, req.CapacityVolumeL, req.TemperatureClass,
			req.ControlledCertified, req.OdometerKm, req.FuelKmPerLiter, next))
	} else {
		v, err = scanVehicle(s.db.QueryRow(ctx, `UPDATE vehicles SET depot_id=$3, plate=$4, vehicle_type=$5, capacity_kg=$6, capacity_volume_l=$7,
			temperature_class=$8, controlled_certified=$9, odometer_km=$10, fuel_km_per_liter=$11, next_service_date=$12
			WHERE id=$1 AND org_id=$2 RETURNING `+vehicleColumns,
			req.Id, org, req.DepotId, req.Plate, req.VehicleType, req.CapacityKg, req.CapacityVolumeL, req.TemperatureClass,
			req.ControlledCertified, req.OdometerKm, req.FuelKmPerLiter, next))
	}
	return v, errs.DB(err, "vehicle (plate or depot)")
}

func (s *server) ListVehicles(ctx context.Context, req *tmsv1.ListVehiclesRequest) (*tmsv1.ListVehiclesResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2)
		AND ($3='' OR status=$3) AND (cardinality($4::text[])=0 OR id::text = ANY($4)) ORDER BY temperature_class, capacity_kg DESC, plate`,
		actor.From(ctx).OrgID, req.DepotId, req.Status, ids(req.Ids))
	if err != nil {
		return nil, errs.DB(err, "vehicles")
	}
	list, err := collect(rows, scanVehicle)
	return &tmsv1.ListVehiclesResponse{Vehicles: list}, errs.DB(err, "vehicles")
}

func (s *server) GetVehicle(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Vehicle, error) {
	v, err := scanVehicle(s.db.QueryRow(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	return v, errs.DB(err, "vehicle")
}

func (s *server) SetVehicleStatus(ctx context.Context, req *tmsv1.SetStatusRequest) (*tmsv1.Vehicle, error) {
	if !slices.Contains(vehicleStatuses, req.Status) {
		return nil, errs.Invalid("status must be one of %s", strings.Join(vehicleStatuses, ", "))
	}
	if req.Status != "AVAILABLE" && req.Reason == "" {
		return nil, errs.Invalid("a reason is required")
	}
	v, err := scanVehicle(s.db.QueryRow(ctx, `UPDATE vehicles SET status=$3, status_reason=$4 WHERE id=$1 AND org_id=$2 RETURNING `+vehicleColumns,
		req.Id, actor.From(ctx).OrgID, req.Status, req.Reason))
	return v, errs.DB(err, "vehicle")
}

// --- drivers ---

const driverColumns = `id, depot_id, name, phone, license_number, license_class, license_expiry, cold_chain_certified,
	controlled_certified, status, user_email, created_at`

func scanDriver(row pgx.Row) (*tmsv1.Driver, error) {
	var d tmsv1.Driver
	var exp pgtype.Date
	var created time.Time
	err := row.Scan(&d.Id, &d.DepotId, &d.Name, &d.Phone, &d.LicenseNumber, &d.LicenseClass, &exp, &d.ColdChainCertified,
		&d.ControlledCertified, &d.Status, &d.UserEmail, &created)
	d.LicenseExpiry, d.CreatedAt = ts.Date(exp), ts.Time(created)
	return &d, err
}

func (s *server) UpsertDriver(ctx context.Context, req *tmsv1.Driver) (*tmsv1.Driver, error) {
	if req.Name == "" || req.DepotId == "" || req.LicenseNumber == "" || req.LicenseExpiry == "" {
		return nil, errs.Invalid("name, depot, licence number and licence expiry are required")
	}
	if req.LicenseClass == "" {
		req.LicenseClass = "B1"
	}
	exp, err := ts.ParseDate(req.LicenseExpiry)
	if err != nil {
		return nil, errs.Invalid("licence expiry must be YYYY-MM-DD")
	}
	org := actor.From(ctx).OrgID
	email := strings.ToLower(strings.TrimSpace(req.UserEmail))
	var d *tmsv1.Driver
	if req.Id == "" {
		d, err = scanDriver(s.db.QueryRow(ctx, `INSERT INTO drivers(org_id, depot_id, name, phone, license_number, license_class, license_expiry,
			cold_chain_certified, controlled_certified, user_email)
			SELECT $1, d.id, $3,$4,$5,$6,$7,$8,$9,$10 FROM depots d WHERE d.id=$2 AND d.org_id=$1 RETURNING `+driverColumns,
			org, req.DepotId, req.Name, req.Phone, req.LicenseNumber, req.LicenseClass, exp, req.ColdChainCertified, req.ControlledCertified, email))
	} else {
		d, err = scanDriver(s.db.QueryRow(ctx, `UPDATE drivers SET depot_id=$3, name=$4, phone=$5, license_number=$6, license_class=$7,
			license_expiry=$8, cold_chain_certified=$9, controlled_certified=$10, user_email=$11 WHERE id=$1 AND org_id=$2 RETURNING `+driverColumns,
			req.Id, org, req.DepotId, req.Name, req.Phone, req.LicenseNumber, req.LicenseClass, exp, req.ColdChainCertified, req.ControlledCertified, email))
	}
	return d, errs.DB(err, "driver (licence number or depot)")
}

func (s *server) ListDrivers(ctx context.Context, req *tmsv1.ListDriversRequest) (*tmsv1.ListDriversResponse, error) {
	rows, err := s.db.Query(ctx, `SELECT `+driverColumns+` FROM drivers WHERE org_id=$1 AND ($2='' OR depot_id::text=$2)
		AND ($3='' OR status=$3) AND (cardinality($4::text[])=0 OR id::text = ANY($4)) AND ($5='' OR user_email=lower($5)) ORDER BY name`,
		actor.From(ctx).OrgID, req.DepotId, req.Status, ids(req.Ids), req.UserEmail)
	if err != nil {
		return nil, errs.DB(err, "drivers")
	}
	list, err := collect(rows, scanDriver)
	return &tmsv1.ListDriversResponse{Drivers: list}, errs.DB(err, "drivers")
}

func (s *server) GetDriver(ctx context.Context, req *tmsv1.IdRequest) (*tmsv1.Driver, error) {
	d, err := scanDriver(s.db.QueryRow(ctx, `SELECT `+driverColumns+` FROM drivers WHERE id=$1 AND org_id=$2`, req.Id, actor.From(ctx).OrgID))
	return d, errs.DB(err, "driver")
}

func (s *server) SetDriverStatus(ctx context.Context, req *tmsv1.SetStatusRequest) (*tmsv1.Driver, error) {
	if !slices.Contains(driverStatuses, req.Status) {
		return nil, errs.Invalid("status must be one of %s", strings.Join(driverStatuses, ", "))
	}
	d, err := scanDriver(s.db.QueryRow(ctx, `UPDATE drivers SET status=$3 WHERE id=$1 AND org_id=$2 RETURNING `+driverColumns,
		req.Id, actor.From(ctx).OrgID, req.Status))
	return d, errs.DB(err, "driver")
}

// --- maintenance ---

const maintenanceColumns = `id, vehicle_id, maintenance_type, description, performed_on, cost, odometer_km, next_due_date, recorded_by, created_at`

func scanMaintenance(row pgx.Row) (*tmsv1.Maintenance, error) {
	var m tmsv1.Maintenance
	var on, next pgtype.Date
	var created time.Time
	err := row.Scan(&m.Id, &m.VehicleId, &m.MaintenanceType, &m.Description, &on, &m.Cost, &m.OdometerKm, &next, &m.RecordedBy, &created)
	m.PerformedOn, m.NextDueDate, m.CreatedAt = ts.Date(on), ts.Date(next), ts.Time(created)
	return &m, err
}

// RecordMaintenance logs work on a vehicle and moves its next service date and odometer forward.
func (s *server) RecordMaintenance(ctx context.Context, req *tmsv1.Maintenance) (*tmsv1.Maintenance, error) {
	if req.VehicleId == "" || req.MaintenanceType == "" {
		return nil, errs.Invalid("vehicle and maintenance type are required")
	}
	if req.PerformedOn == "" {
		req.PerformedOn = time.Now().Format(ts.DateLayout)
	}
	on, err := ts.ParseDate(req.PerformedOn)
	if err != nil {
		return nil, errs.Invalid("performed date must be YYYY-MM-DD")
	}
	next, err := ts.ParseDate(req.NextDueDate)
	if err != nil {
		return nil, errs.Invalid("next due date must be YYYY-MM-DD")
	}
	a := actor.From(ctx)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	m, err := scanMaintenance(tx.QueryRow(ctx, `INSERT INTO maintenance(org_id, vehicle_id, maintenance_type, description, performed_on, cost,
		odometer_km, next_due_date, recorded_by) SELECT $1, v.id, $3,$4,$5,$6,$7,$8,$9 FROM vehicles v WHERE v.id=$2 AND v.org_id=$1
		RETURNING `+maintenanceColumns, a.OrgID, req.VehicleId, req.MaintenanceType, req.Description, on, req.Cost, req.OdometerKm, next, a.Name()))
	if err != nil {
		return nil, errs.DB(err, "maintenance (vehicle)")
	}
	if _, err := tx.Exec(ctx, `UPDATE vehicles SET odometer_km = GREATEST(odometer_km, $2), next_service_date = COALESCE($3, next_service_date)
		WHERE id=$1`, req.VehicleId, req.OdometerKm, next); err != nil {
		return nil, errs.DB(err, "vehicle")
	}
	return m, errs.DB(tx.Commit(ctx), "maintenance")
}

func (s *server) ListMaintenance(ctx context.Context, req *tmsv1.ListMaintenanceRequest) (*tmsv1.ListMaintenanceResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `SELECT `+maintenanceColumns+` FROM maintenance WHERE org_id=$1 AND ($2='' OR vehicle_id::text=$2)
		ORDER BY performed_on DESC, created_at DESC LIMIT $3`, actor.From(ctx).OrgID, req.VehicleId, limit)
	if err != nil {
		return nil, errs.DB(err, "maintenance")
	}
	list, err := collect(rows, scanMaintenance)
	return &tmsv1.ListMaintenanceResponse{Records: list}, errs.DB(err, "maintenance")
}

func (s *server) GetStats(ctx context.Context, req *tmsv1.FleetStatsRequest) (*tmsv1.FleetStats, error) {
	var out tmsv1.FleetStats
	err := s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'INACTIVE'),
		(SELECT count(*) FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status='AVAILABLE'),
		(SELECT count(*) FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status='MAINTENANCE'),
		(SELECT count(*) FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND temperature_class='COLD' AND status <> 'INACTIVE'),
		(SELECT count(*) FROM drivers WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status='ACTIVE'),
		(SELECT count(*) FROM drivers WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'INACTIVE'
			AND license_expiry >= current_date AND license_expiry < current_date + 30),
		(SELECT count(*) FROM drivers WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'INACTIVE' AND license_expiry < current_date),
		(SELECT count(*) FROM vehicles WHERE org_id=$1 AND ($2='' OR depot_id::text=$2) AND status <> 'INACTIVE' AND next_service_date < current_date + 14),
		(SELECT COALESCE(sum(m.cost),0)::float8 FROM maintenance m JOIN vehicles v ON v.id=m.vehicle_id WHERE m.org_id=$1
			AND ($2='' OR v.depot_id::text=$2) AND m.performed_on >= current_date - 30)`,
		actor.From(ctx).OrgID, req.DepotId).Scan(&out.Vehicles, &out.VehiclesAvailable, &out.VehiclesMaintenance, &out.ReeferVehicles,
		&out.DriversActive, &out.LicensesExpiring_30D, &out.LicensesExpired, &out.ServicesDue_14D, &out.MaintenanceCost_30D)
	return &out, errs.DB(err, "fleet stats")
}
