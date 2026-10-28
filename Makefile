.PHONY: install db dev api build test stop
install:
	cd backend && go mod download
	cd frontend && ../scripts/with-node.sh npm ci --no-audit --no-fund
db:
	docker compose up -d --wait db
api:
	cd backend && (set -a; if [ -f ../.env ]; then . ../.env; fi; set +a; go run .)
dev:
	./scripts/dev.sh
build:
	cd frontend && ../scripts/with-node.sh npm run build
	cd backend && go build -o aimovieai .
test:
	cd backend && go test -race ./... && go vet ./...
stop:
	docker compose stop
