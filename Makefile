SERVICES := identity fleet shipment planning freight gateway
DATABASE_URL ?= postgres://localhost:5432/pharmacy_tms?sslmode=disable

.PHONY: proto build db dev seed web

proto:
	protoc -I proto \
		--go_out=gen --go_opt=paths=source_relative \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		proto/tms/v1/*.proto

build:
	@mkdir -p bin
	@for s in $(SERVICES); do echo "building $$s"; go build -o bin/$$s ./services/$$s || exit 1; done

db:
	createdb pharmacy_tms 2>/dev/null || true

dev: db
	./scripts/dev.sh

seed:
	go run ./scripts/seed

web:
	cd web && npm install && npm run dev
