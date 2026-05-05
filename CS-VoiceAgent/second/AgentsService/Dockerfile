# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23

# --- Base: deps + исходники
FROM golang:${GO_VERSION}-bookworm AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# --- Сборка бинарника (production)
FROM base AS build
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/agent-service ./cmd/agent-service

# --- Одноразовый контейнер: unit + API tests (без E2E-тега)
FROM base AS test
ENV CGO_ENABLED=0
CMD ["go", "test", "./...", "-count=1"]

# --- Минимальный production-образ (без shell / curl)
FROM gcr.io/distroless/static-debian12:nonroot AS production
COPY --from=build /out/agent-service /agent-service
USER nonroot:nonroot
ENTRYPOINT ["/agent-service"]

# --- Образ для docker-compose.test: healthcheck через curl
FROM alpine:3.20 AS runtime-test
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/agent-service /agent-service
# Порт задаётся через HTTP_ADDR (в compose: :8080)
EXPOSE 8080
ENTRYPOINT ["/agent-service"]
