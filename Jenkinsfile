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
        POSTGRES_PASSWORD        = credentials('mtg-postgres-password')
        REVALIDATE_SECRET        = credentials('mtg-revalidate-secret')
        BOOTSTRAP_ADMIN_PASSWORD = credentials('mtg-bootstrap-admin-password')
        GOOGLE_CLIENT_ID         = credentials('mtg-google-client-id')
        GOOGLE_CLIENT_SECRET     = credentials('mtg-google-client-secret')

        // Non-secret configuration.
        POSTGRES_USER            = 'mtg'
        POSTGRES_DB              = 'mtg_meta'
        HTTP_ADDR                = ':8080'
        SESSION_COOKIE_NAME      = 'mtg_session'
        SESSION_TTL_HOURS        = '720'
        SCRYFALL_USER_AGENT      = 'mtg-meta-tracker/0.1 (contact: runyanjake@gmail.com)'
        SCRYFALL_MIN_INTERVAL_MS = '100'
        // Public origin of the deployment.
        APP_BASE_URL             = 'https://cube.whitney.rip'
        GOOGLE_REDIRECT_URL      = 'https://cube.whitney.rip/api/auth/google/callback'
        // Internal backend->frontend revalidation call, via the compose service DNS name.
        REVALIDATE_URL           = 'http://frontend:3000/api/revalidate'
        BOOTSTRAP_ADMIN_USERNAME = 'admin'
        BOOTSTRAP_ADMIN_EMAIL    = 'runyanjake@gmail.com'

        COMPOSE_PROJECT_NAME     = 'mtg-meta-tracker'
        BACKEND_CONTAINER        = 'mtg-backend'
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
                sh '''
                    set -eu
                    docker run --rm -v "$WORKSPACE/backend:/src" -w /src golang:1.22-alpine \
                        sh -c 'go vet ./... && go build ./...' \
                        || { echo "backend lint/compile failed" >&2; exit 1; }
                '''
                sh '''
                    set -eu
                    docker run --rm -v "$WORKSPACE/frontend:/app" -w /app node:20-alpine \
                        sh -c 'npm install --no-audit --no-fund && npx --yes tsc --noEmit' \
                        || { echo "frontend type-check failed" >&2; exit 1; }
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
APP_BASE_URL=${APP_BASE_URL}
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

        stage('Teardown') {
            steps {
                // Preserve the pgdata volume — no -v.
                sh 'docker compose down --remove-orphans || { echo "teardown of previous deployment failed" >&2; exit 1; }'
            }
        }

        stage('Build & Deploy') {
            steps {
                // .env was generated in the Prepare Environment stage above.
                sh '''
                    set -eu
                    docker compose up -d --build --remove-orphans \
                        || { echo "build & deploy failed" >&2; exit 1; }
                '''
            }
        }

        stage('Health Check') {
            steps {
                sh '''
                    set -eu
                    deadline=$(( $(date +%s) + 120 ))
                    while true; do
                        status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$BACKEND_CONTAINER" 2>/dev/null || echo missing)
                        case "$status" in
                            healthy)
                                echo "backend is healthy"; break ;;
                            exited|dead|missing)
                                echo "backend failed to start (status: $status)" >&2
                                docker logs --tail=50 "$BACKEND_CONTAINER" 2>&1 || true
                                exit 1 ;;
                        esac
                        [ "$(date +%s)" -lt "$deadline" ] || { echo "backend did not become healthy within timeout" >&2; exit 1; }
                        sleep 3
                    done
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
