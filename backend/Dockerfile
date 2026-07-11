FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
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
