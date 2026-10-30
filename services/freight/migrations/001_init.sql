CREATE TABLE carriers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    service_level text NOT NULL DEFAULT 'NEXT_DAY' CHECK (service_level IN ('SAME_DAY','NEXT_DAY','REGULAR')),
    supports_cold boolean NOT NULL DEFAULT false,
    supports_controlled boolean NOT NULL DEFAULT false,
    base_rate double precision NOT NULL DEFAULT 0 CHECK (base_rate >= 0),
    rate_per_kg double precision NOT NULL DEFAULT 0 CHECK (rate_per_kg >= 0),
    rate_per_km double precision NOT NULL DEFAULT 0 CHECK (rate_per_km >= 0),
    cold_surcharge_pct double precision NOT NULL DEFAULT 0 CHECK (cold_surcharge_pct >= 0),
    on_time_pct double precision NOT NULL DEFAULT 95,
    transit_days int NOT NULL DEFAULT 1,
    contact text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    UNIQUE (org_id, code)
);

CREATE SEQUENCE tender_seq;
CREATE TABLE tenders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    tender_number text NOT NULL,
    depot_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','AWARDED','CANCELLED')),
    total_weight_kg double precision NOT NULL,
    total_distance_km double precision NOT NULL,
    requires_cold boolean NOT NULL,
    requires_controlled boolean NOT NULL,
    deadline timestamptz NOT NULL,
    awarded_bid_id uuid,
    awarded_at timestamptz,
    cancel_reason text NOT NULL DEFAULT '',
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, tender_number)
);

CREATE TABLE tender_shipments (
    tender_id uuid NOT NULL REFERENCES tenders(id),
    shipment_id uuid NOT NULL,
    shipment_number text NOT NULL,
    consignee_name text NOT NULL,
    weight_kg double precision NOT NULL,
    distance_km double precision NOT NULL,
    temperature_class text NOT NULL,
    controlled boolean NOT NULL,
    PRIMARY KEY (tender_id, shipment_id)
);

CREATE TABLE bids (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tender_id uuid NOT NULL REFERENCES tenders(id),
    carrier_id uuid NOT NULL REFERENCES carriers(id),
    amount double precision NOT NULL CHECK (amount > 0),
    transit_days int NOT NULL DEFAULT 1,
    compliant boolean NOT NULL,
    non_compliance_reason text NOT NULL DEFAULT '',
    source text NOT NULL CHECK (source IN ('RATE_CARD','MANUAL')),
    note text NOT NULL DEFAULT '',
    submitted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tender_id, carrier_id, source)
);
