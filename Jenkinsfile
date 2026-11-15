// Formats the build's commits for the Discord embed; @NonCPS so the changeSet
// iterator is not persisted across pipeline steps.
@NonCPS
def formatCommits(changeSets) {
    def lines = []
    for (set in changeSets) {
        for (entry in set.items) {
            lines.add("> ${entry.msg} (by *${entry.author.fullName}*)")
        }
    }
    return lines.isEmpty() ? 'No recent changes detected.' : lines.join('\n')
}

// True when this build is of main. A multibranch job sets BRANCH_NAME; a plain
// pipeline-from-SCM job only sets GIT_BRANCH, as "origin/main". A bare
// `when { branch 'main' }` reads only BRANCH_NAME, so in a plain job it would
// silently skip every deploy.
def isMain() {
    def branch = env.BRANCH_NAME ?: env.GIT_BRANCH ?: ''
    return branch == 'main' || branch == 'origin/main' || branch == 'refs/heads/main'
}

pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        timeout(time: 30, unit: 'MINUTES')
    }

    environment {
        // Secrets — pulled from Jenkins credentials, never hardcoded.
        DISCORD_WEBHOOK          = credentials('discord-pws-builds-channel-webhook')
        POSTGRES_PASSWORD        = credentials('mtg-meta-tracker-postgres-password')
        REVALIDATE_SECRET        = credentials('mtg-meta-tracker-revalidate-secret')
        BOOTSTRAP_ADMIN_PASSWORD = credentials('mtg-meta-tracker-bootstrap-admin-password')
        GOOGLE_CLIENT_ID         = credentials('mtg-meta-tracker-google-client-id')
        GOOGLE_CLIENT_SECRET     = credentials('mtg-meta-tracker-google-client-secret')

        // Non-secret configuration.
        POSTGRES_USER            = 'mtg'
        POSTGRES_DB              = 'mtg_meta'
        HTTP_ADDR                = ':8080'
        SESSION_COOKIE_NAME      = 'mtg_session'
        SESSION_TTL_HOURS        = '720'
        SCRYFALL_USER_AGENT      = 'mtg-meta-tracker/0.1 (contact: admin@fullcontrolmtg.com)'
        SCRYFALL_MIN_INTERVAL_MS = '100'
        GOOGLE_REDIRECT_URL      = 'https://cube.fullcontrolmtg.com/api/auth/google/callback'
        // Internal backend->frontend revalidation call, via the compose service DNS name.
        REVALIDATE_URL           = 'http://frontend:3000/api/revalidate'
        BOOTSTRAP_ADMIN_USERNAME = 'admin'
        BOOTSTRAP_ADMIN_EMAIL    = 'admin@fullcontrolmtg.com'

        COMPOSE_PROJECT_NAME     = 'mtg-meta-tracker'
        BACKEND_CONTAINER        = 'mtg-cube-backend'
        FRONTEND_CONTAINER       = 'mtg-cube-frontend'

        // Pre-deploy pg_dump retention: the newest N dumps in the dump bind
        // mount (docker-compose.yml, service db-dump) are kept.
        DUMP_KEEP                = '30'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Preflight') {
            steps {
                sh '''
                    set -eu
                    fail() { echo "Preflight failed: $1" >&2; exit 1; }

                    [ -n "${DISCORD_WEBHOOK:-}" ]          || fail "credential DISCORD_WEBHOOK is empty"
                    [ -n "${POSTGRES_PASSWORD:-}" ]        || fail "credential POSTGRES_PASSWORD is empty"
                    [ -n "${REVALIDATE_SECRET:-}" ]        || fail "credential REVALIDATE_SECRET is empty"
                    [ -n "${BOOTSTRAP_ADMIN_PASSWORD:-}" ] || fail "credential BOOTSTRAP_ADMIN_PASSWORD is empty"
                    [ -n "${GOOGLE_CLIENT_ID:-}" ]         || fail "credential GOOGLE_CLIENT_ID is empty"
                    [ -n "${GOOGLE_CLIENT_SECRET:-}" ]     || fail "credential GOOGLE_CLIENT_SECRET is empty"

                    docker version >/dev/null 2>&1         || fail "docker is not available on the agent"
                    docker compose version >/dev/null 2>&1 || fail "docker compose plugin is not available"
                    docker network inspect traefik >/dev/null 2>&1 || fail "external docker network 'traefik' does not exist"
                '''
            }
        }

        stage('Lint & Type-check') {
            steps {
                // Ship the source as a build context instead of bind-mounting
                // $WORKSPACE. The job name contains spaces and the agent may talk
                // to a socket-shared daemon (DinD), either of which makes a host
                // bind mount land empty; `docker build` transfers context to the
                // daemon the same way the compose stages do, so it is immune.
                // Images are throwaway — the check passes/fails on the RUN exit code.
                sh '''
                    set -eu
                    docker build --rm -f - backend <<'DOCKERFILE'
FROM golang:1.22-alpine
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go vet ./... && go build ./... && go test ./...
DOCKERFILE
                '''
                sh '''
                    set -eu
                    docker build --rm -f - frontend <<'DOCKERFILE'
FROM node:20-alpine
WORKDIR /app
COPY package.json ./
RUN npm install --no-audit --no-fund
COPY . .
RUN npx --yes tsc --noEmit
DOCKERFILE
                '''
            }
        }

        stage('Prepare Environment') {
            steps {
                // Generate .env before ANY compose command. The backend service
                // declares `env_file: .env`, so `docker compose` (including the
                // Teardown `down` and the config validation below) hard-fails when
                // .env is absent — which is the case on a clean workspace, since
                // nothing else creates it. Writing it here makes every downstream
                // compose invocation see a consistent, fully-populated file.
                sh '''
                    set -eu
                    umask 077
                    cat > .env <<EOF
DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB}?sslmode=disable
HTTP_ADDR=${HTTP_ADDR}
SESSION_COOKIE_NAME=${SESSION_COOKIE_NAME}
SESSION_TTL_HOURS=${SESSION_TTL_HOURS}
GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}
GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET}
GOOGLE_REDIRECT_URL=${GOOGLE_REDIRECT_URL}
SCRYFALL_USER_AGENT=${SCRYFALL_USER_AGENT}
SCRYFALL_MIN_INTERVAL_MS=${SCRYFALL_MIN_INTERVAL_MS}
REVALIDATE_URL=${REVALIDATE_URL}
REVALIDATE_SECRET=${REVALIDATE_SECRET}
BOOTSTRAP_ADMIN_USERNAME=${BOOTSTRAP_ADMIN_USERNAME}
BOOTSTRAP_ADMIN_EMAIL=${BOOTSTRAP_ADMIN_EMAIL}
BOOTSTRAP_ADMIN_PASSWORD=${BOOTSTRAP_ADMIN_PASSWORD}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=${POSTGRES_DB}
EOF
                    docker compose config -q \
                        || { echo "docker-compose.yml is not well-formed" >&2; exit 1; }
                '''
            }
        }

        // Everything from here touches prod, so only main gets this far.
        stage('Deploy') {
            when { expression { isMain() } }
            stages {
                stage('Build Images') {
                    steps {
                        // Build while the old deployment is still serving, so a
                        // failed build leaves prod up instead of torn down.
                        sh 'docker compose build || { echo "image build failed" >&2; exit 1; }'
                    }
                }

                stage('Backup Database') {
                    steps {
                        // Dump before Teardown: the new backend applies its schema
                        // on boot, and this is the last copy of the data from
                        // before that. Any failure here fails the build with the
                        // old deployment still running.
                        //
                        // The dump directory lives on the Docker host, which the
                        // agent may not see (see Lint & Type-check), so writing,
                        // checking and pruning all happen inside the db-dump
                        // container. Names sort by time, so pruning sorts by name
                        // rather than trusting mtimes on the NAS.
                        sh '''
                            set -eu
                            case "$DUMP_KEEP" in
                                ''|*[!0-9]*|0) echo "DUMP_KEEP must be a positive integer, got '$DUMP_KEEP'" >&2; exit 1 ;;
                            esac

                            if [ -z "$(docker compose ps --status running --quiet db)" ]; then
                                echo "No running db container (first deploy?); skipping the pre-deploy dump."
                                exit 0
                            fi

                            DUMP_FILE="mtg_meta_$(date -u +%Y%m%dT%H%M%SZ)_$(git rev-parse --short HEAD).dump"
                            echo "Dumping ${POSTGRES_DB} to ${DUMP_FILE}"
                            docker compose run --rm -T \
                                -e DUMP_FILE="$DUMP_FILE" -e DUMP_KEEP="$DUMP_KEEP" \
                                db-dump <<'SH' || { echo "pre-deploy database dump failed; not deploying" >&2; exit 1; }
umask 077
tmp="/dump/.${DUMP_FILE}.partial"
trap 'rm -f "$tmp"' EXIT
pg_dump -Fc -f "$tmp"
pg_restore --list "$tmp" >/dev/null
mv "$tmp" "/dump/${DUMP_FILE}"
echo "wrote /dump/${DUMP_FILE} ($(wc -c < "/dump/${DUMP_FILE}") bytes)"
ls -1 /dump | grep -E '^mtg_meta_[0-9]{8}T[0-9]{6}Z_[0-9a-f]+[.]dump$' | sort -r \
    | tail -n +$((DUMP_KEEP + 1)) | while read -r old; do
        echo "pruning /dump/$old"
        rm -f "/dump/$old"
    done
SH
                        '''
                    }
                }

                stage('Teardown') {
                    steps {
                        // Preserve the pgdata volume — no -v.
                        sh 'docker compose down --remove-orphans || { echo "teardown of previous deployment failed" >&2; exit 1; }'
                    }
                }

                stage('Start') {
                    steps {
                        // Images were built in Build Images; --no-build makes sure
                        // nothing is rebuilt now that prod is down.
                        sh '''
                            set -eu
                            docker compose up -d --no-build --remove-orphans \
                                || { echo "deploy failed" >&2; exit 1; }
                        '''
                    }
                }

                stage('Health Check') {
                    steps {
                        // Gate on both containers: Traefik drops an unhealthy frontend
                        // from its load balancer, which surfaces as a 404 in the browser.
                        sh '''
                            set -eu
                            wait_healthy() {
                                name="$1"
                                deadline=$(( $(date +%s) + 120 ))
                                while true; do
                                    status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$name" 2>/dev/null || echo missing)
                                    case "$status" in
                                        healthy)
                                            echo "$name is healthy"; return 0 ;;
                                        exited|dead|missing)
                                            echo "$name failed to start (status: $status)" >&2
                                            docker logs --tail=50 "$name" 2>&1 || true
                                            return 1 ;;
                                    esac
                                    [ "$(date +%s)" -lt "$deadline" ] || { echo "$name did not become healthy within timeout (last status: $status)" >&2; docker logs --tail=50 "$name" 2>&1 || true; return 1; }
                                    sleep 3
                                done
                            }
                            wait_healthy "$BACKEND_CONTAINER"  || exit 1
                            wait_healthy "$FRONTEND_CONTAINER" || exit 1
                        '''
                    }
                }

                stage('Smoke Test') {
                    steps {
                        sh '''
                            set -eu
                            body=$(docker compose exec -T backend wget -qO- http://localhost:8080/api/health) \
                                || { echo "smoke test failed: could not reach /api/health" >&2; exit 1; }
                            echo "health response: $body"
                            echo "$body" | grep -q '"ok":true' \
                                || { echo "smoke test failed: unexpected response body" >&2; exit 1; }
                        '''
                    }
                }
            }
        }
    }

    post {
        always {
            script {
                def result = currentBuild.currentResult
                def emoji = result == 'SUCCESS' ? ':green_circle:' : (result == 'FAILURE' ? ':red_circle:' : ':yellow_circle:')
                def branch = env.GIT_BRANCH ?: env.BRANCH_NAME ?: 'Main/Manual'
                def duration = currentBuild.durationString.replace(' and no weeks', '').replace(' and counting', '')
                def commits = formatCommits(currentBuild.changeSets)
                def description = """**Status:** ${emoji} ${result}
**Branch:** `${branch}`
**Duration:** :stopwatch: ${duration}

**Commits:**
${commits}"""

                discordSend(
                    webhookURL: env.DISCORD_WEBHOOK,
                    title: "📦 Build Alert: ${env.JOB_NAME} [Build #${env.BUILD_NUMBER}]",
                    link: "${env.BUILD_URL}",
                    result: "${currentBuild.currentResult}",
                    description: description
                )
            }
        }
        failure {
            sh '''
                echo "===== docker compose ps ====="
                docker compose ps || true
                echo "===== recent container logs ====="
                docker compose logs --tail=200 || true
                echo "===== backend health ====="
                docker inspect --format '{{json .State.Health}}' "$BACKEND_CONTAINER" 2>/dev/null || true
            '''
        }
    }
}
