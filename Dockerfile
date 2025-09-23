# Build stage
FROM golang:1.23 AS builder
WORKDIR /app

# Cache modules
COPY src/go.mod src/go.sum ./
RUN go mod download

# Copy backend sources
COPY src/ .

# Build a static binary (assumes main is in ./cmd/web)
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o /app/web ./cmd/web

# Final runtime image
FROM debian:bullseye-slim
WORKDIR /app

# minimal runtime deps for HTTPS
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy binary and static assets (output.css is now in source control under src/static)
COPY --from=builder /app/web .
COPY --from=builder /app/src/static /app/static

EXPOSE 8080

CMD ["./web"]