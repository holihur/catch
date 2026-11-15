# syntax=docker/dockerfile:1

# ---- build frontend -------------------------------------------------------
FROM node:24-alpine AS web
RUN npm install -g pnpm@12.15.0
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
ARG VITE_INGEST_KEY=""
ENV VITE_API_URL="" \
    VITE_INGEST_KEY=${VITE_INGEST_KEY} \
    VITE_ENV=production \
    VITE_RELEASE=web@docker
RUN pnpm build

# ---- build Go binary ------------------------------------------------------
FROM golang:1.27.3-alpine AS api
RUN apk add --no-cache git
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/catch ./cmd/server

# ---- runtime --------------------------------------------------------------
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 catch
WORKDIR /app
COPY --from=api /out/catch /app/catch
COPY --from=web /src/frontend/dist /app/web
USER catch
ENV STATIC_DIR=/app/web \
    PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/catch"]
