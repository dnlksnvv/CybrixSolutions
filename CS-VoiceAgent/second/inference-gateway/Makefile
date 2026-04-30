.PHONY: help build run dev test fmt vet tidy

help:
	@echo "make build  - compile the gateway binary into ./bin/inference-gateway"
	@echo "make run    - go run ./cmd/server"
	@echo "make dev    - air-style hot reload (requires 'air' installed)"
	@echo "make test   - go test ./..."
	@echo "make fmt    - gofmt -w ."
	@echo "make vet    - go vet ./..."
	@echo "make tidy   - go mod tidy"

build:
	go build -o bin/inference-gateway ./cmd/server

run:
	go run ./cmd/server

dev:
	air -c .air.toml

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy
