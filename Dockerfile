# syntax=docker/dockerfile:1

# Stage 1: build the frontend (mirrors scripts/build.sh).
FROM node:24-alpine AS web
WORKDIR /build/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
# pnpm 11 keeps overrides and patchedDependencies in pnpm-workspace.yaml; the
# frozen lockfile check compares against them, so the file and its patch files
# must exist before install.
COPY web/pnpm-workspace.yaml ./
COPY web/patches/ ./patches/
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# Stage 2: build the backend with the embedded frontend.
FROM golang:1.26-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
COPY agent/ ./agent/
RUN go mod download
COPY . .
COPY --from=web /build/web/dist ./internal/webfs/dist
ARG DENOVA_VERSION=""
RUN VERSION="${DENOVA_VERSION}"; \
    if [ -z "${VERSION}" ]; then VERSION="$(sed -n 's/^  "version": "\(.*\)",\?$/\1/p' web/package.json | head -n 1)"; fi; \
    CGO_ENABLED=0 go build -tags embedweb \
      -ldflags "-s -w -X denova/internal/buildinfo.Version=${VERSION:-dev}" \
      -o /out/denova ./cmd/denova/

# Stage 3: runtime. bash and ripgrep back the Agent shell and search tools.
FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata bash ripgrep
WORKDIR /app
COPY --from=backend /out/denova ./denova
COPY skills/ ./skills/
COPY config.toml ./config.toml
ENV DENOVA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/auth/status || exit 1
CMD ["./denova", "--no-open"]
