# ── Frontend build ───────────────────────────────────
FROM node:24-alpine AS frontend

WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm install --no-audit --no-fund

COPY web/ ./
RUN npm run build

# ── Go build ─────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# CGO needed for sqlite3; build-base provides gcc
RUN apk add --no-cache build-base sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/web/dist ./web/dist

ENV CGO_ENABLED=1
RUN go build -o forum -ldflags="-s -w" -a -installsuffix cgo ./cmd/forum

# ── Runtime image ────────────────────────────────────
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates sqlite-libs tzdata && \
    addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

COPY --from=builder /app/forum .

RUN chown -R app:app /app
USER app

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

CMD ["./forum"]
