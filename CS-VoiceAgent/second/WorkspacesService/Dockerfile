FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/workspaces ./cmd/workspaces

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/workspaces /workspaces
EXPOSE 8080
ENTRYPOINT ["/workspaces"]
