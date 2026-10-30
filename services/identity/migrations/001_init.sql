CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES organizations(id),
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    roles text[] NOT NULL DEFAULT '{}',
    depot_ids text[] NOT NULL DEFAULT '{}',
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    actor_id text NOT NULL DEFAULT '',
    actor_email text NOT NULL DEFAULT '',
    action text NOT NULL,
    entity_type text NOT NULL DEFAULT '',
    entity_id text NOT NULL DEFAULT '',
    correlation_id text NOT NULL DEFAULT '',
    details jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_org_created_idx ON audit_events (org_id, created_at DESC);
CREATE INDEX audit_events_entity_idx ON audit_events (org_id, entity_type, entity_id);
