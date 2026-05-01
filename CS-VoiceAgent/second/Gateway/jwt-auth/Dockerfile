FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/jwt-auth ./cmd/jwt-auth

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/jwt-auth /jwt-auth
EXPOSE 8000
ENTRYPOINT ["/jwt-auth"]
