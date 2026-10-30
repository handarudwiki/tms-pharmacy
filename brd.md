# Transportation Management System (TMS)
**Business Requirements Document — Version 1.0**

## 1. Document Overview

| Item | Description |
|---|---|
| Product | Transportation Management System (TMS) |
| Document type | Business Requirements Document |
| Version | 1.0 |
| Status | Initial Draft |
| Backend | Go |
| Inter-service communication | gRPC / Protocol Buffers |
| Frontend | Vue.js |
| State management | Pinia |
| Database | PostgreSQL |
| Architecture | Microservices |
| Initial target market | Medium-sized pharmaceutical, cosmetics and FMCG distributors |
| Companion system | Pharmacy WMS (warehouse execution, shipment readiness) |

## 2. Executive Summary

The Transportation Management System (TMS) plans, executes and controls the outbound delivery of goods that the Warehouse Management System (WMS) has packed and staged. It owns the responsibilities the WMS explicitly excludes: route optimization, fleet management, driver execution and freight procurement.

The platform aims to deliver on time and in full, keep cold-chain and controlled-drug shipments compliant, maximize vehicle utilization and reduce freight cost, while providing proof of delivery and end-to-end traceability.

The system must handle real-world exceptions, including:

- Vehicles or drivers that are unavailable, uncertified or over capacity.
- Shipments that cannot be planned with the available fleet.
- Failed delivery attempts, refusals and returns.
- Temperature excursions on cold-chain trips.
- Late deliveries against customer time windows.
- Outsourcing overflow volume to third-party carriers.

## 3. Business Objectives

### BO-01: Improve on-time, in-full delivery
Deliver shipments within the customer's required date and delivery window.

### BO-02: Reduce transportation cost
Minimize distance, empty capacity and freight spend through route optimization and carrier tendering.

### BO-03: Ensure regulatory compliance
Guarantee that cold-chain and controlled-drug shipments travel only on certified vehicles with certified drivers, and that temperature is monitored.

### BO-04: Provide delivery visibility
Track trips, vehicle positions, stop status and proof of delivery in real time.

### BO-05: Improve fleet productivity
Track vehicle availability, maintenance and driver licensing to maximize productive fleet time.

### BO-06: Enable system integration
Receive shipment readiness from the WMS and expose delivery status to ERP, OMS and customer-facing systems.

## 4. Scope

### 4.1 In Scope

1. Depot (dispatch origin) management.
2. Fleet management: vehicles, capacity, temperature capability, maintenance.
3. Driver management: licences, certifications, availability.
4. Shipment management (from WMS integration or manual entry).
5. Trip planning: manual and automatic, capacity- and compliance-aware.
6. Route optimization and stop sequencing with ETAs.
7. Trip execution: release, start, stop arrival, completion.
8. Driver application for delivery execution and proof of delivery.
9. Failed delivery handling, rescheduling and returns.
10. Vehicle telemetry: position and temperature monitoring.
11. Carrier management, rate cards and freight tendering.
12. Freight cost estimation and spend reporting.
13. Exception management.
14. Reporting, audit trails and role-based access.
15. WMS integration.

### 4.2 Out of Scope for the Initial MVP

1. Warehouse operations (owned by the WMS).
2. Customer invoicing and accounts receivable.
3. Turn-by-turn navigation (drivers use their preferred navigation app).
4. Live traffic-aware routing.
5. Cross-border customs documentation.
6. Hardware telematics device management.

## 5. Stakeholders and User Roles

| Role | Responsibilities |
|---|---|
| Organization Administrator | Manages organization configuration and access |
| Transport Manager | Oversees delivery operations, approves plans and exceptions |
| Dispatcher / Planner | Plans trips, assigns vehicles and drivers, releases trips |
| Fleet Manager | Maintains vehicles, maintenance and driver records |
| Driver | Executes trips, records arrivals, deliveries and failures |
| Carrier Manager | Manages carriers, rate cards and freight tenders |
| Customer Service | Tracks shipments and answers delivery queries |
| Transport Analyst | Reviews KPIs, cost and performance |
| Integration Administrator | Manages WMS / ERP connections |

A single employee may hold several roles; sensitive actions must still follow authorization rules.

## 6. Functional Requirements

### FR-01: Depot Management
The system shall support multiple depots per organization, each with an address and geographic coordinates used as the origin of trips.

### FR-02: Fleet Management
The system shall support:
- Vehicle registration with plate, type, weight and volume capacity.
- Temperature capability (ambient or refrigerated 2–8 °C).
- Controlled-drug certification of vehicles.
- Vehicle status (available, maintenance, inactive).
- Maintenance records and next-service dates.
- Fuel efficiency used for cost estimation.

### FR-03: Driver Management
The system shall support:
- Driver records with licence number, class and expiry date.
- Cold-chain and controlled-drug certifications.
- Driver status (active, on leave, inactive).
- Linking a driver to a user account for the driver application.
- Alerts for licences expiring within 30 days.

### FR-04: Shipment Management
The system shall:
- Accept shipments from the WMS (shipment readiness) and from authorized users.
- De-duplicate shipments by external reference.
- Record consignee, address, coordinates, delivery window and required date.
- Record weight, volume, package count, temperature requirement and controlled-drug flag.
- Maintain a status lifecycle and an auditable event timeline.
- Support cancellation before dispatch.

### FR-05: Trip Planning
The system shall:
- Allow dispatchers to build trips manually.
- Automatically plan pending shipments into trips for a date and depot.
- Respect vehicle weight and volume capacity.
- Respect temperature compatibility (cold shipments only on refrigerated vehicles).
- Respect controlled-drug certification of both vehicle and driver.
- Prevent double-booking of vehicles and drivers on the same date.
- Reject drivers with expired licences.
- Report shipments that could not be planned and why.

### FR-06: Route Optimization
The system shall:
- Sequence stops to minimize travel distance from and back to the depot.
- Calculate distance, estimated duration and ETA per stop.
- Flag stops whose ETA falls outside the customer delivery window.
- Allow re-optimization of a planned trip.

### FR-07: Trip Execution
The system shall:
- Release a planned trip for execution, marking shipments as dispatched.
- Record trip start, stop arrival, stop completion and trip completion.
- Prevent completion of a trip with unresolved stops.
- Support cancellation of trips that have not started, returning shipments to planning.

### FR-08: Driver Application
The system shall provide drivers with:
- Their assigned trip and ordered stop list.
- Arrival confirmation per stop.
- Proof of delivery capture: receiver name, signature confirmation, notes and location.
- Failed delivery reporting with a reason.
- Position updates.

### FR-09: Failed Delivery and Returns
The system shall:
- Record failed delivery attempts and reasons.
- Allow rescheduling, returning the shipment to the planning pool.
- Mark shipments as returned to depot after repeated failure or refusal.
- Raise exceptions for failed deliveries.

### FR-10: Telemetry and Cold-Chain Monitoring
The system shall:
- Record vehicle position and temperature readings for trips.
- Detect temperature excursions outside 2–8 °C on cold trips.
- Raise high-severity exceptions for excursions affecting shipments.

### FR-11: Carrier Management and Tendering
The system shall:
- Maintain carriers with service level, capabilities and rate cards.
- Create freight tenders for shipments that will be outsourced.
- Collect bids (automatic quotes from rate cards and manual bids).
- Exclude non-compliant bids (e.g. a carrier without cold-chain capability for cold shipments).
- Award a tender, assigning the shipments and freight cost to the carrier.

### FR-12: Freight Cost
The system shall:
- Estimate in-house trip cost from distance, fuel efficiency and driver time.
- Record awarded carrier freight cost per shipment.
- Report cost per shipment, per kilogram and per kilometre.

### FR-13: Reporting and Analytics
The system shall provide reports for:
- On-time and OTIF delivery.
- Shipment status and backlog.
- Trips, distance and vehicle utilization.
- Failed deliveries and exceptions.
- Freight spend by carrier.
- Fleet availability, maintenance and licence compliance.

### FR-14: Security and Auditability
The system shall support authentication, role-based access, organization-level data isolation, audit trails for sensitive operations, request correlation across services and secure service-to-service communication.

## 7. Business Rules

### BR-01: Temperature Compatibility
Cold-chain shipments may only be assigned to refrigerated vehicles driven by cold-chain certified drivers.

### BR-02: Controlled Drugs
Controlled-drug shipments require a certified vehicle and a certified driver.

### BR-03: Capacity
A trip's total weight and volume must not exceed the vehicle's capacity.

### BR-04: Resource Availability
A vehicle or driver may be assigned to at most one active trip per date; vehicles in maintenance and drivers with expired licences cannot be assigned.

### BR-05: Proof of Delivery
A shipment can only be marked delivered with proof of delivery recorded.

### BR-06: Idempotent Processing
Repeated processing of the same logical request (including WMS shipment imports) must not create duplicate shipments, trips or deliveries.

### BR-07: Cancellation
Shipments already delivered cannot be cancelled. Trips that have started cannot be cancelled; their stops must be completed or failed.

## 8. Non-Functional Requirements

| Category | Requirement |
|---|---|
| Consistency | Shipment status changes and trip changes must remain consistent |
| Availability | Driver execution should remain available during delivery hours |
| Security | Enforce authentication, authorization and protected service communication |
| Auditability | Sensitive actions must be attributable to an authorized actor |
| Observability | Structured logs and correlation IDs across services |
| Integration | Versioned gRPC contracts and documented external interfaces |
| Data isolation | Prevent cross-organization access |

## 9. Success Metrics

1. On-time delivery rate.
2. OTIF (on time in full) rate.
3. Failed delivery rate.
4. Vehicle capacity utilization.
5. Cost per shipment and per kilometre.
6. Cold-chain excursion count.
7. Average stops per trip.
8. Fleet availability.

## 10. Implementation Phases

### Phase 1: Operational MVP
- Depots, fleet and drivers.
- Shipments and WMS integration.
- Manual and automatic trip planning with route optimization.
- Trip execution and driver application with proof of delivery.

### Phase 2: Exceptions and Compliance
- Failed delivery, rescheduling and returns.
- Telemetry and cold-chain excursion detection.
- Licence and maintenance alerts.

### Phase 3: Freight and Optimization
- Carriers, rate cards and tendering.
- Cost analytics and utilization reporting.
