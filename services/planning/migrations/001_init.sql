CREATE SEQUENCE trip_seq;
CREATE TABLE trips (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    trip_number text NOT NULL,
    depot_id uuid NOT NULL,
    depot_lat double precision NOT NULL,
    depot_lng double precision NOT NULL,
    vehicle_id uuid NOT NULL,
    vehicle_plate text NOT NULL,
    driver_id uuid NOT NULL,
    driver_name text NOT NULL,
    driver_email text NOT NULL DEFAULT '',
    plan_date date NOT NULL,
    status text NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED','RELEASED','IN_PROGRESS','COMPLETED','CANCELLED')),
    distance_km double precision NOT NULL DEFAULT 0,
    duration_min int NOT NULL DEFAULT 0,
    weight_kg double precision NOT NULL DEFAULT 0,
    volume_l double precision NOT NULL DEFAULT 0,
    capacity_util_pct double precision NOT NULL DEFAULT 0,
    estimated_cost double precision NOT NULL DEFAULT 0,
    temperature_class text NOT NULL DEFAULT 'AMBIENT',
    planned_start timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    cancel_reason text NOT NULL DEFAULT '',
    last_lat double precision NOT NULL DEFAULT 0,
    last_lng double precision NOT NULL DEFAULT 0,
    last_temperature_c double precision,
    last_ping_at timestamptz,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, trip_number)
);
-- BR-04: one active trip per vehicle and per driver per day.
CREATE UNIQUE INDEX trips_vehicle_day_idx ON trips (org_id, vehicle_id, plan_date) WHERE status <> 'CANCELLED';
CREATE UNIQUE INDEX trips_driver_day_idx ON trips (org_id, driver_id, plan_date) WHERE status <> 'CANCELLED';

CREATE TABLE stops (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    sequence int NOT NULL,
    shipment_id uuid NOT NULL,
    shipment_number text NOT NULL,
    consignee_name text NOT NULL,
    address text NOT NULL,
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    eta timestamptz,
    window_start text NOT NULL,
    window_end text NOT NULL,
    distance_from_prev_km double precision NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ARRIVED','DELIVERED','FAILED')),
    arrived_at timestamptz,
    completed_at timestamptz,
    weight_kg double precision NOT NULL,
    volume_l double precision NOT NULL,
    packages int NOT NULL,
    temperature_class text NOT NULL,
    controlled boolean NOT NULL,
    failure_reason text NOT NULL DEFAULT ''
);
CREATE INDEX stops_trip_idx ON stops (trip_id, sequence);

CREATE TABLE telemetry (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    temperature_c double precision,
    speed_kmh double precision NOT NULL DEFAULT 0,
    excursion boolean NOT NULL DEFAULT false,
    recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX telemetry_trip_idx ON telemetry (trip_id, recorded_at);
