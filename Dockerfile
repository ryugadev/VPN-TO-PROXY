# =========================================================
# Stage 1: Build the React Frontend
# =========================================================
FROM node:22-alpine AS frontend-builder
WORKDIR /app
COPY webapp/package*.json ./
RUN npm install
COPY webapp/ ./
RUN npm run build

# =========================================================
# Stage 2: Build the Go Backend
# =========================================================
FROM golang:1.26-alpine AS backend-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=1 is required for the go-sqlite3 driver
ENV CGO_ENABLED=1
RUN go build -ldflags="-w -s" -o server ./cmd/server

# =========================================================
# Stage 3: Final Production Runner
# =========================================================
FROM alpine:latest
# Install runtime dependencies for WireGuard and Network namespaces
RUN apk add --no-cache wireguard-tools iptables iproute2 openvpn ca-certificates

WORKDIR /app
COPY --from=backend-builder /app/server .
COPY --from=frontend-builder /app/dist ./webapp/dist

EXPOSE 8080

ENTRYPOINT ["./server"]
