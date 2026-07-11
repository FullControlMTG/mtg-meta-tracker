FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /server /server
# Default IMAGE_CACHE_DIR mount point. Owned by app so the cache works even with
# no volume mounted; a bind mount over it must carry the same ownership (compose
# has an images-init service for that).
RUN mkdir -p /data/images && chown -R app:app /data/images
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8080/api/health || exit 1
ENTRYPOINT ["/server"]
