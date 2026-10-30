# Pharmacy TMS

Transportation Management System for pharmaceutical, cosmetics and FMCG distribution, built from
[`brd.md`](./brd.md). It is the companion of Pharmacy WMS: the WMS packs and stages orders, the TMS plans,
executes and proves their delivery.

**Stack:** Go microservices · gRPC / Protocol Buffers · PostgreSQL · Vue 3 + Pinia + Tailwind · Leaflet (OpenStreetMap).

## Screenshots

Captured from the seeded demo data (`docs/screenshots/`).

| Page | |
|---|---|
| Login | [![Login](docs/screenshots/00-login.png)](docs/screenshots/00-login.png) |
| Dashboard | [![Dashboard](docs/screenshots/01-dashboard.png)](docs/screenshots/01-dashboard.png) |
| Live Tracking | [![Live Tracking](docs/screenshots/02-live-tracking.png)](docs/screenshots/02-live-tracking.png) |
| Shipments | [![Shipments](docs/screenshots/03-shipments.png)](docs/screenshots/03-shipments.png) |
| Planning | [![Planning](docs/screenshots/04-planning.png)](docs/screenshots/04-planning.png) |
| Trips | [![Trips](docs/screenshots/05-trips.png)](docs/screenshots/05-trips.png) |
| Fleet Drivers | [![Fleet Drivers](docs/screenshots/06-fleet-drivers.png)](docs/screenshots/06-fleet-drivers.png) |
| Carriers Tenders | [![Carriers Tenders](docs/screenshots/07-carriers-tenders.png)](docs/screenshots/07-carriers-tenders.png) |
| Exceptions | [![Exceptions](docs/screenshots/08-exceptions.png)](docs/screenshots/08-exceptions.png) |
| Analytics | [![Analytics](docs/screenshots/09-analytics.png)](docs/screenshots/09-analytics.png) |
| Integrations | [![Integrations](docs/screenshots/10-integrations.png)](docs/screenshots/10-integrations.png) |
| Users Roles | [![Users Roles](docs/screenshots/11-users-roles.png)](docs/screenshots/11-users-roles.png) |
| Audit Trail | [![Audit Trail](docs/screenshots/12-audit-trail.png)](docs/screenshots/12-audit-trail.png) |
| Trip Detail | [![Trip Detail](docs/screenshots/13-trip-detail.png)](docs/screenshots/13-trip-detail.png) |
| Driver App | [![Driver App](docs/screenshots/14-driver-app.png)](docs/screenshots/14-driver-app.png) |
| Dashboard (dark mode) | [![Dashboard (dark mode)](docs/screenshots/15-dashboard-dark.png)](docs/screenshots/15-dashboard-dark.png) |

## Architecture

```
          Vue 3 + Pinia web app (web/) — office screens + driver app
                       │  REST / JSON
                ┌──────▼───────┐  JWT auth, RBAC, depot access, audit, correlation IDs,
                │   gateway    │  Idempotency-Key passthrough, dashboard, WMS import
                └──────┬───────┘
         gRPC (tms.v1, internal service token, actor + correlation metadata)
   ┌──────────┬────────┴──────┬──────────────┬────────────┐
identity    fleet         shipment        planning      freight
users,      depots,       shipments,      auto/manual   carriers,
roles,      vehicles,     timeline,       planning,     rate cards,
audit       drivers,      proof of        route optim., tenders,
            maintenance   delivery,       execution,    bids, awards
                          exceptions      telemetry
```

Each service owns its own PostgreSQL schema and runs its migrations on start-up. Contracts live in
[`proto/tms/v1`](./proto/tms/v1).

### How the BRD's rules are enforced

| Rule | Where |
|---|---|
| BR-01 temperature | Cold shipments only on refrigerated vehicles with cold-chain certified drivers (planning compliance check). |
| BR-02 controlled drugs | Vehicle *and* driver must be certified. |
| BR-03 capacity | Weight and volume checked on every manual and automatic trip. |
| BR-04 availability | One active trip per vehicle/driver per day (unique DB index); maintenance vehicles and expired licences are excluded. |
| BR-05 proof of delivery | A DB check constraint makes `DELIVERED` impossible without receiver name, signature and timestamp. |
| BR-06 idempotency | Delivery confirmations are keyed per stop; WMS imports are keyed by WMS order number. |
| BR-07 cancellation | Delivered shipments and started trips cannot be cancelled. |

**Planning:** shipments are swept by polar angle around the depot; each round picks the smallest compatible
free vehicle that fits the expected load and the most qualified free driver, packs up to 12 stops / 9 hours,
then orders stops with nearest-neighbour + 2-opt and schedules ETAs against delivery windows (waiting for
windows to open, flagging late stops). Leftovers are reported with a reason and can be tendered to carriers.

**Execution:** releasing a trip dispatches its shipments; the driver app records arrival, proof of delivery
or failure (rescheduled back to the pool, or returned to the depot after three attempts / a refusal) and position
pings. A cold trip reading outside 2–8 °C raises a high-severity excursion for every shipment still on board.

**Freight:** a tender quotes every active carrier from its rate card (base + per kg + per km + cold surcharge),
marks bids from carriers lacking cold-chain or controlled-drug capability as non-compliant, and awarding splits
the amount across shipments by weight.

## WMS integration

`POST /api/integrations/wms/sync` (Integrations screen) logs into the WMS as its integration user, reads the
outbox, and creates a shipment for every `shipment.ready` event (weight, volume, temperature class and
controlled flag come from WMS packing). Already-dispatched or withdrawn orders are skipped. Configure with
`WMS_API_URL`, `WMS_SYNC_EMAIL`, `WMS_SYNC_PASSWORD` and `WMS_DEPOT_CODE` (default depot `CKR`).

## Running locally

Requirements: Go 1.25+, Node 20+, PostgreSQL 15+. Ports: gRPC 9101–9105, gateway 8090, web 5174
(so it runs side by side with the WMS on 9001–9005 / 8080 / 5173).

```bash
createdb pharmacy_tms
./scripts/dev.sh                    # builds and starts all services; gateway on :8090
go run ./scripts/seed               # demo data (empty database); imports from the WMS if it is running
cd web && npm install && npm run dev   # http://localhost:5174
```

Or in containers: `docker compose up --build` → <http://localhost:8090>.
`./scripts/reset-db.sh` drops every service schema (restart the services afterwards).

### Demo accounts (after seeding)

| Email | Password | Role |
|---|---|---|
| admin@tms.local | admin123 | Organization admin |
| transport@tms.local | password123 | Transport manager |
| dispatcher@tms.local | password123 | Dispatcher / planner |
| fleet@tms.local | password123 | Fleet manager |
| carrier@tms.local | password123 | Carrier manager |
| cs@tms.local | password123 | Customer service |
| budi@tms.local, slamet@tms.local, andi@tms.local, … | password123 | Drivers (driver app) |

## Known limitations

- Distances use straight-line distance × 1.35 and an average urban speed; there is no road network or live traffic.
- Customers that are not in the built-in address book are placed approximately and flagged for the dispatcher.
- Cross-service steps rely on idempotent retries and compensation rather than distributed transactions.
- Delivery status is not yet pushed back to the WMS/ERP; it is available through the shipment API.
- Map tiles come from the public OpenStreetMap tile server; use a commercial tile provider in production.
