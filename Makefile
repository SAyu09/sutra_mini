.PHONY: all migrate seed api worker mock web test lint clean

all: lint test

migrate:
	go run ./cmd/migrate up

seed:
	go run ./cmd/migrate seed

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

mock:
	go run ./cmd/mock

web:
	cd web && npm run dev

test:
	go test -v ./...

lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt found unformatted files:" && gofmt -l . && exit 1)
	go vet ./...

clean:
	rm -rf bin/
