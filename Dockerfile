# Multi-stage Dockerfile for FaultLine Chaos Reverse Proxy

# Stage 1: Build Frontend React Dashboard
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go Backend
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /faultline ./cmd/faultline

# Stage 3: Minimal Production Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /faultline /app/faultline
COPY --from=frontend-builder /app/web/dist /app/web/dist

EXPOSE 8080

ENTRYPOINT ["/app/faultline"]
CMD ["-port", "8080", "-static-dir", "/app/web/dist"]
