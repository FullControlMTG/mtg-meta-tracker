FROM node:20-alpine AS build
# Next bakes the /api/* rewrite destination at build time, so BACKEND_ORIGIN
# must be set before `npm run build` — a runtime env var is too late.
ARG BACKEND_ORIGIN=http://backend:8080
WORKDIR /app
# No package-lock.json in the repo, so install rather than ci.
COPY frontend/package.json ./
RUN npm install
COPY frontend/ ./
ENV BACKEND_ORIGIN=${BACKEND_ORIGIN}
RUN npm run build

FROM node:20-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    PORT=3000 \
    HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
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
