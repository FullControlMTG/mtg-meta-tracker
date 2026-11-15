# The cache mounts below need BuildKit; Jenkins forces it (DOCKER_BUILDKIT=1).
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
# Modules go into the image layer, not a cache mount: the test image runs
# `go test` again on an --internal network with no internet, so it must carry them.
RUN go mod download
COPY . .
# Go's build cache lives in a BuildKit cache mount, not the layer. Without it every
# source change recompiles pgx, chi and x/crypto from scratch, since COPY . . above
# invalidates the layer that would otherwise hold the cache.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# CI's checks, built with `docker build --target test`. The default build (the
# last stage, runtime) never needs this one, and BuildKit skips it; the legacy
# builder would run every stage in order, these tests included, on each deploy.
# DB tests skip here without TEST_DATABASE_URL; Jenkins runs the image again with
# it set, and with its own Go build cache volume, since the mount above is not
# part of the image.
FROM build AS test
RUN --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./...

FROM alpine:3.20 AS runtime
# postgresql16-client for pg_restore, which `server restore` and the admin restore
# API shell out to. Its major version must be >= the server's (postgres:16).
RUN apk add --no-cache ca-certificates postgresql16-client && adduser -D -u 10001 app
COPY --from=build /server /server
# Default IMAGE_CACHE_DIR mount point. This chown is what makes the cache
# writable in production: compose mounts an empty named volume here, and Docker
# seeds a fresh named volume from the image path's contents and ownership. Drop
# it and the volume lands root-owned, leaving the app user unable to write.
RUN mkdir -p /data/images && chown -R app:app /data/images
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8080/api/health || exit 1
ENTRYPOINT ["/server"]
