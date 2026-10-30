#!/usr/bin/env bash
# Runs every TMS backend service locally (logs in ./logs). Stop with Ctrl+C.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p logs bin
export DATABASE_URL="${DATABASE_URL:-postgres://localhost:5432/pharmacy_tms?sslmode=disable}"

for s in identity fleet shipment planning freight gateway; do
  go build -o "bin/$s" "./services/$s"
done

pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT INT TERM
for s in identity fleet shipment planning freight; do
  "./bin/$s" >"logs/$s.log" 2>&1 &
  pids+=($!)
done
# Wait until every gRPC port accepts connections before starting the gateway.
for port in 9101 9102 9103 9104 9105; do
  for _ in $(seq 1 50); do nc -z localhost "$port" 2>/dev/null && break; sleep 0.2; done
done
./bin/gateway >"logs/gateway.log" 2>&1 &
pids+=($!)
echo "TMS backend running — gateway on http://localhost:8090 (logs in ./logs)"
wait
