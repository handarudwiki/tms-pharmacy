CREATE TABLE idempotency_keys (
    key text PRIMARY KEY,
    response bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE SEQUENCE shipment_seq;
CREATE TABLE shipments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    shipment_number text NOT NULL,
    external_ref text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'MANUAL' CHECK (source IN ('WMS','MANUAL')),
    depot_id uuid NOT NULL,
    consignee_name text NOT NULL,
    consignee_phone text NOT NULL DEFAULT '',
    address text NOT NULL,
    city text NOT NULL DEFAULT '',
    lat double precision NOT NULL,
    lng double precision NOT NULL,
    location_approximate boolean NOT NULL DEFAULT false,
    window_start text NOT NULL DEFAULT '08:00',
    window_end text NOT NULL DEFAULT '17:00',
    required_date date NOT NULL,
    weight_kg double precision NOT NULL CHECK (weight_kg > 0),
    volume_l double precision NOT NULL DEFAULT 0 CHECK (volume_l >= 0),
    packages int NOT NULL DEFAULT 1 CHECK (packages > 0),
    temperature_class text NOT NULL DEFAULT 'AMBIENT' CHECK (temperature_class IN ('AMBIENT','COLD')),
    controlled boolean NOT NULL DEFAULT false,
    priority int NOT NULL DEFAULT 3 CHECK (priority BETWEEN 1 AND 5),
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','PLANNED','TENDERED','DISPATCHED','DELIVERED','RETURNED','CANCELLED')),
    trip_id text NOT NULL DEFAULT '',
    tender_id text NOT NULL DEFAULT '',
    carrier_id text NOT NULL DEFAULT '',
    carrier_name text NOT NULL DEFAULT '',
    freight_cost double precision NOT NULL DEFAULT 0,
    attempts int NOT NULL DEFAULT 0,
    pod_receiver_name text NOT NULL DEFAULT '',
    pod_signature boolean NOT NULL DEFAULT false,
    pod_note text NOT NULL DEFAULT '',
    pod_lat double precision NOT NULL DEFAULT 0,
    pod_lng double precision NOT NULL DEFAULT 0,
    pod_recorded_by text NOT NULL DEFAULT '',
    delivered_at timestamptz,
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, shipment_number),
    -- BR-05: a delivered shipment always carries proof of delivery.
    CONSTRAINT delivered_has_pod CHECK (status <> 'DELIVERED' OR (pod_receiver_name <> '' AND pod_signature AND delivered_at IS NOT NULL))
);
-- WMS imports are de-duplicated by the WMS order reference (BR-06).
CREATE UNIQUE INDEX shipments_external_ref_idx ON shipments (org_id, external_ref) WHERE external_ref <> '';
CREATE INDEX shipments_status_idx ON shipments (org_id, depot_id, status);

CREATE TABLE shipment_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id uuid NOT NULL REFERENCES shipments(id),
    event_type text NOT NULL,
    description text NOT NULL DEFAULT '',
    actor text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shipment_events_idx ON shipment_events (shipment_id, created_at);

CREATE TABLE exceptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    depot_id text NOT NULL DEFAULT '',
    exception_type text NOT NULL,
    severity text NOT NULL DEFAULT 'MEDIUM',
    shipment_id text NOT NULL DEFAULT '',
    shipment_number text NOT NULL DEFAULT '',
    trip_id text NOT NULL DEFAULT '',
    details text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'OPEN',
    raised_by text NOT NULL DEFAULT '',
    resolved_by text NOT NULL DEFAULT '',
    resolution text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);
