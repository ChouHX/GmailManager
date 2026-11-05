# ---------- stage 1: build the SPA ----------
FROM node:22-alpine AS web

WORKDIR /app/web

# Allow mirrors for restricted networks (override with --build-arg NPM_REGISTRY=...)
ARG NPM_REGISTRY=https://registry.npmjs.org

COPY web/package.json web/package-lock.json* ./
RUN npm config set registry "$NPM_REGISTRY" \
 && (npm ci --no-audit --no-fund || npm install --no-audit --no-fund)

# Copy sources only, so a host-built dist or node_modules never leaks in
COPY web/index.html web/tsconfig.json web/vite.config.ts ./
COPY web/src/ ./src/
RUN npm run build


# ---------- stage 2: build the Go binary ----------
FROM golang:1.24-bookworm AS backend

WORKDIR /src

# Allow mirrors for restricted networks (override with --build-arg GOPROXY=...)
ARG GOPROXY=https://proxy.golang.org,direct

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN GOPROXY="$GOPROXY" GOSUMDB=off go mod download

COPY main.go ./
COPY internal/ ./internal/
# Embed the built frontend
COPY --from=web /app/web/dist ./web/dist

RUN GOPROXY="$GOPROXY" GOSUMDB=off go build -trimpath -ldflags="-s -w" -o /out/gmailmanager .


# ---------- stage 3: runtime ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app \
 && mkdir -p /data && chown -R app:app /data

COPY --from=backend /out/gmailmanager /usr/local/bin/gmailmanager

ENV TZ=Asia/Shanghai \
    GM_ADDR=:8080 \
    GM_DATA_DIR=/data \
    GM_DB_PATH=/data/gmailmanager.db

WORKDIR /app
USER app

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/gmailmanager"]
