CREATE TABLE depots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    address text NOT NULL DEFAULT '',
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, code)
);

CREATE TABLE vehicles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    depot_id uuid NOT NULL REFERENCES depots(id),
    plate text NOT NULL,
    vehicle_type text NOT NULL CHECK (vehicle_type IN ('MOTORBIKE','VAN','BOX_TRUCK','REEFER_VAN','REEFER_TRUCK')),
    capacity_kg double precision NOT NULL CHECK (capacity_kg > 0),
    capacity_volume_l double precision NOT NULL CHECK (capacity_volume_l > 0),
    temperature_class text NOT NULL DEFAULT 'AMBIENT' CHECK (temperature_class IN ('AMBIENT','COLD')),
    controlled_certified boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN ('AVAILABLE','MAINTENANCE','INACTIVE')),
    status_reason text NOT NULL DEFAULT '',
    odometer_km double precision NOT NULL DEFAULT 0,
    fuel_km_per_liter double precision NOT NULL DEFAULT 8,
    next_service_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, plate)
);

CREATE TABLE drivers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    depot_id uuid NOT NULL REFERENCES depots(id),
    name text NOT NULL,
    phone text NOT NULL DEFAULT '',
    license_number text NOT NULL,
    license_class text NOT NULL DEFAULT 'B1',
    license_expiry date NOT NULL,
    cold_chain_certified boolean NOT NULL DEFAULT false,
    controlled_certified boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ON_LEAVE','INACTIVE')),
    user_email text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, license_number)
);

CREATE TABLE maintenance (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    vehicle_id uuid NOT NULL REFERENCES vehicles(id),
    maintenance_type text NOT NULL,
    description text NOT NULL DEFAULT '',
    performed_on date NOT NULL,
    cost double precision NOT NULL DEFAULT 0,
    odometer_km double precision NOT NULL DEFAULT 0,
    next_due_date date,
    recorded_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
