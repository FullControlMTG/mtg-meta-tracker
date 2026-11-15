# The cache mounts below need BuildKit; Jenkins forces it (DOCKER_BUILDKIT=1).
FROM node:20-alpine AS deps
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
# The npm cache mount makes a lockfile change re-link from cache instead of
# re-downloading every package.
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./

# CI's type-check, built with `docker build --target typecheck .`. It shares the
# deps layer with the prod build, so `npm ci` runs once per lockfile, not twice.
FROM deps AS typecheck
RUN npx tsc --noEmit

FROM deps AS build
# Next bakes the /api/* rewrite destination at build time, so BACKEND_ORIGIN
# must be set before `npm run build` — a runtime env var is too late.
ARG BACKEND_ORIGIN=http://backend:8080
ENV BACKEND_ORIGIN=${BACKEND_ORIGIN}
# .next/cache is Next's incremental build cache. Kept in a mount, it survives the
# COPY above being invalidated by every frontend change.
RUN --mount=type=cache,target=/app/.next/cache npm run build

FROM node:20-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    PORT=3000 \
    HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
# `output: standalone` does NOT bundle public/ — it must be copied in by hand, or the
# standalone server 404s every static asset (e.g. /logo.png in the nav).
COPY --from=build /app/public ./public
# Next.js writes ISR / on-demand revalidation data to .next/cache at runtime.
# The COPYed .next is root-owned, so pre-create the cache dir owned by the
# non-root runtime user; otherwise revalidateTag/Path fails with EACCES mkdir.
RUN mkdir -p .next/cache && chown -R node:node .next
USER node
EXPOSE 3000
# Probe via node (always present) rather than BusyBox wget.
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD node -e "require('http').get('http://127.0.0.1:3000/',r=>process.exit(r.statusCode<400?0:1)).on('error',()=>process.exit(1))"
CMD ["node", "server.js"]
