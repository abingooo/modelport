#!/bin/bash

set -euo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${TEST_DIR}/.." && pwd)"
SCRIPT="${DEPLOY_DIR}/apple-container.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-apple-test.XXXXXX")"
STATE_DIR="${TEST_ROOT}/state"
ENV_FILE="${TEST_ROOT}/sub2api.env"

cleanup() {
    rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

assert_exists() {
    [[ -e "$1" ]] || fail "Expected path to exist: $1"
}

assert_missing() {
    [[ ! -e "$1" ]] || fail "Expected path to be absent: $1"
}

assert_contains() {
    grep -Fq "$2" "$1" || fail "Expected $1 to contain: $2"
}

set_env_value() {
    local key=$1
    local value=$2
    local temp_file="${ENV_FILE}.tmp"

    awk -v wanted="${key}" -v replacement="${value}" '
        BEGIN { replaced = 0 }
        {
            separator = index($0, "=")
            key = separator == 0 ? "" : substr($0, 1, separator - 1)
            if (key == wanted) {
                if (!replaced) { print wanted "=" replacement }
                replaced = 1
                next
            }
            print
        }
        END {
            if (!replaced) { print wanted "=" replacement }
        }
    ' "${ENV_FILE}" >"${temp_file}"
    chmod 600 "${temp_file}"
    mv "${temp_file}" "${ENV_FILE}"
}

export FAKE_CONTAINER_STATE="${STATE_DIR}"
export PATH="${TEST_DIR}/fixtures/bin:${PATH}"
export SUB2API_ENV_FILE="${ENV_FILE}"

mkdir -p "${STATE_DIR}"

"${SCRIPT}" init
[[ "$(stat -f '%Lp' "${ENV_FILE}")" == "600" ]] || fail "init did not create a mode-600 env file"
grep -q '^POSTGRES_PASSWORD=change_this_secure_password$' "${ENV_FILE}" && fail "init retained the placeholder password"

chmod 644 "${ENV_FILE}"
if "${SCRIPT}" up >/dev/null 2>&1; then
    fail "up accepted an insecure env file"
fi
chmod 600 "${ENV_FILE}"

if "${SCRIPT}" up >"${TEST_ROOT}/default-image.stdout" 2>"${TEST_ROOT}/default-image.stderr"; then
    fail "up accepted the formal linux/amd64 image for Apple linux/arm64"
fi
assert_contains "${TEST_ROOT}/default-image.stderr" "ModelPort 0.1.183.1 publishes only linux/amd64"
assert_missing "${STATE_DIR}/system-running"

set_env_value APPLE_CONTAINER_SUB2API_IMAGE "weishaw/sub2api:arm64"
if "${SCRIPT}" up >"${TEST_ROOT}/upstream-image.stdout" 2>"${TEST_ROOT}/upstream-image.stderr"; then
    fail "up accepted an upstream Sub2API ARM64 image"
fi
assert_contains "${TEST_ROOT}/upstream-image.stderr" "Upstream Sub2API images are not valid ModelPort artifacts"
assert_missing "${STATE_DIR}/system-running"

set_env_value APPLE_CONTAINER_SUB2API_IMAGE "ghcr.io/abingooo/modelport:reviewed-arm64-test"
"${SCRIPT}" up
assert_exists "${STATE_DIR}/containers/sub2api-apple"
assert_exists "${STATE_DIR}/containers/sub2api-apple-postgres"
assert_exists "${STATE_DIR}/containers/sub2api-apple-redis"
assert_exists "${STATE_DIR}/running/sub2api-apple"
"${SCRIPT}" status >/dev/null

"${SCRIPT}" up --recreate
assert_exists "${STATE_DIR}/running/sub2api-apple"
"${SCRIPT}" down
assert_missing "${STATE_DIR}/running/sub2api-apple"
assert_missing "${STATE_DIR}/running/sub2api-apple-postgres"
assert_missing "${STATE_DIR}/running/sub2api-apple-redis"

"${SCRIPT}" destroy --yes
assert_missing "${STATE_DIR}/containers/sub2api-apple"
assert_missing "${STATE_DIR}/networks/sub2api-apple"
assert_exists "${STATE_DIR}/volumes/sub2api-apple-data"

"${SCRIPT}" up
"${SCRIPT}" destroy --volumes --yes
assert_missing "${STATE_DIR}/volumes/sub2api-apple-data"
assert_missing "${STATE_DIR}/volumes/sub2api-apple-postgres-data"
assert_missing "${STATE_DIR}/volumes/sub2api-apple-redis-data"

touch "${STATE_DIR}/system-running"
touch "${STATE_DIR}/containers/sub2api-apple"
touch "${STATE_DIR}/unowned/container/sub2api-apple"
if "${SCRIPT}" status >/dev/null 2>&1; then
    fail "status accepted an unowned same-name container"
fi

printf 'Apple container lifecycle tests passed.\n'
