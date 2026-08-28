#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

fail() {
  printf 'ModelPort release contract test failed: %s\n' "$1" >&2
  exit 1
}

workflow=.github/workflows/custom-image.yml
upstream_workflow=.github/workflows/release.yml
github_slot_check=custom/release/assert-github-slots-absent
image_slot_check=custom/release/assert-image-tags-absent
version=$(tr -d '[:space:]' < custom/VERSION)

for required_release_dependency in "$github_slot_check" "$image_slot_check"; do
  [ -x "$required_release_dependency" ] || \
    fail "release dependency is missing or not executable: $required_release_dependency"
  git ls-files --error-unmatch "$required_release_dependency" >/dev/null 2>&1 || \
    fail "release dependency is not tracked by Git: $required_release_dependency"
  if git check-ignore --no-index -q "$required_release_dependency"; then
    fail "release dependency is ignored: $required_release_dependency"
  fi
done

[ "$version" = '0.1.183.1' ] || fail 'custom/VERSION is not 0.1.183.1'
[ "$(tr -d '[:space:]' < backend/cmd/server/VERSION)" = "$version" ] || \
  fail 'backend and custom versions differ'
[ "$(tr -d '[:space:]' < backend/cmd/server/UPSTREAM_VERSION)" = '0.1.183' ] || \
  fail 'upstream version is not 0.1.183'
[ -s "docs/releases/${version}.md" ] || fail 'release notes are missing'

grep -Fqx '  RELEASE_IMAGE: ghcr.io/abingooo/modelport' "$workflow" || \
  fail 'release image repository changed'
grep -Fqx '  EXPECTED_REPOSITORY: abingooo/modelport' "$workflow" || \
  fail 'release repository identity gate is missing'
grep -Fqx '  group: modelport-release' "$workflow" || \
  fail 'release workflow must serialize all publication attempts'
grep -Fqx '  LEGACY_MODELPORT_TAG: custom-v0.1.176.2' "$workflow" || \
  fail 'legacy compatibility tag changed'
grep -Fqx '          make_latest: false' "$workflow" || \
  fail 'GitHub Release must not become the mutable latest release'
# shellcheck disable=SC2016 # The workflow expression is matched literally.
grep -Fq 'repos/${GITHUB_REPOSITORY}/immutable-releases' "$workflow" || \
  fail 'workflow must preflight GitHub Immutable Releases'
grep -Fq "'.immutable' <<< \"\${release_json}\"" "$workflow" || \
  fail 'public verification must confirm release immutability'

base_image_count=$(grep -Ec '^  (NODE|GOLANG|ALPINE|POSTGRES|REDIS)_IMAGE: [^[:space:]]+@sha256:[0-9a-f]{64}$' "$workflow" || true)
[ "$base_image_count" -eq 5 ] || fail 'all five release base images must be pinned by digest'

grep -Fqx '          scan-type: config' "$workflow" || \
  fail 'release quality gates must scan Dockerfile configuration'
grep -Fq "docker image inspect --format '{{.Config.User}}'" "$workflow" || \
  fail 'release image inspection must require the non-root image user'
grep -Fqx '          docker volume create modelport-smoke-data' "$workflow" || \
  fail 'release smoke test must exercise the application named volume'
grep -Fq 'docker exec modelport-smoke id -u' "$workflow" || \
  fail 'release smoke test must verify the running application UID'

if grep -Eq '(^|[[:space:]])[^#]*:latest([[:space:]]|$)' "$workflow"; then
  fail 'ModelPort workflow must not publish or consume latest image tags'
fi

if awk '
  /uses:/ {
    ref = $0
    sub(/^.*uses:[[:space:]]*/, "", ref)
    sub(/[[:space:]]*#.*/, "", ref)
    if (ref !~ /@[0-9a-f]{40}$/) exit 1
  }
' "$workflow"; then
  :
else
  fail 'every action dependency must be pinned to a 40-character commit'
fi

gate_count=$(grep -Fc "github.repository == 'Wei-Shaw/sub2api'" "$upstream_workflow" || true)
[ "$gate_count" -eq 4 ] || fail 'all four upstream release jobs must be disabled in ModelPort'

github_slot_check_count=$(grep -Fc '/bin/sh custom/release/assert-github-slots-absent' "$workflow" || true)
[ "$github_slot_check_count" -eq 2 ] || fail 'GitHub slots must be checked before build and before release'
grep -Fq '/bin/sh custom/release/assert-image-tags-absent' "$workflow" || \
  fail 'GHCR tags must use a fail-closed existence check'
# shellcheck disable=SC2016 # The workflow command is matched literally.
grep -Fq 'created="$(git show -s --format=%cI "${GITHUB_SHA}")"' "$workflow" || \
  fail 'image creation time must come from the exact release commit'

/bin/sh -n "$github_slot_check"
/bin/sh -n "$image_slot_check"

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
mock_bin="$temporary_dir/bin"
mkdir -p "$mock_bin"

cat > "$mock_bin/git" <<'EOF'
#!/bin/sh
exit "${MODELPORT_TEST_GIT_STATUS:-2}"
EOF
cat > "$mock_bin/curl" <<'EOF'
#!/bin/sh
cat >/dev/null
case "${MODELPORT_TEST_CURL_MODE:-404}" in
  network) exit 7 ;;
  *) printf '%s' "${MODELPORT_TEST_CURL_MODE:-404}" ;;
esac
EOF
cat > "$mock_bin/docker" <<'EOF'
#!/bin/sh
case "${MODELPORT_TEST_DOCKER_MODE:-absent}" in
  exists) exit 0 ;;
  absent) echo 'unexpected status from HEAD request: 404 Not Found' >&2; exit 1 ;;
  manifest_unknown) echo 'manifest unknown' >&2; exit 1 ;;
  unauthorized) echo 'unexpected status from HEAD request: 401 Unauthorized' >&2; exit 1 ;;
  network) echo 'dial tcp: network is unreachable' >&2; exit 1 ;;
  token_404) echo 'unexpected status from token request: 404 Not Found' >&2; exit 1 ;;
  mixed_404_unauthorized)
    echo 'unexpected status from HEAD request: 404 Not Found' >&2
    echo 'token request failed: 401 Unauthorized' >&2
    exit 1
    ;;
  mixed_manifest_network)
    echo 'manifest unknown' >&2
    echo 'dial tcp: network is unreachable' >&2
    exit 1
    ;;
  version_404_unauthorized) echo 'ghcr.io/abingooo/modelport:custom-v0.1.404.1: 401 Unauthorized' >&2; exit 1 ;;
  sha_404_unauthorized) echo 'ghcr.io/abingooo/modelport:sha-a404b123456789abcdef0123456789abcdef0123: 401 Unauthorized' >&2; exit 1 ;;
esac
EOF
chmod 0755 "$mock_bin/git" "$mock_bin/curl" "$mock_bin/docker"

run_github_slot_check() {
  PATH="$mock_bin:$PATH" \
  GH_TOKEN=modelport-test-token \
  MODELPORT_TEST_GIT_STATUS="${MODELPORT_TEST_GIT_STATUS:-2}" \
  MODELPORT_TEST_CURL_MODE="${MODELPORT_TEST_CURL_MODE:-404}" \
    /bin/sh "$github_slot_check" origin custom-v0.1.183.1 abingooo/modelport
}

run_image_slot_check() {
  PATH="$mock_bin:$PATH" \
  MODELPORT_TEST_DOCKER_MODE="${MODELPORT_TEST_DOCKER_MODE:-absent}" \
    /bin/sh "$image_slot_check" \
      ghcr.io/abingooo/modelport:custom-v0.1.183.1 \
      ghcr.io/abingooo/modelport:sha-0123456789abcdef0123456789abcdef01234567
}

MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE=404 run_github_slot_check >/dev/null || \
  fail 'confirmed absent GitHub slots were rejected'
if MODELPORT_TEST_GIT_STATUS=0 run_github_slot_check >/dev/null 2>&1; then
  fail 'an existing Git tag was treated as absent'
fi
if MODELPORT_TEST_GIT_STATUS=128 run_github_slot_check >/dev/null 2>&1; then
  fail 'a Git transport failure was treated as absence'
fi
if MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE=200 run_github_slot_check >/dev/null 2>&1; then
  fail 'an existing GitHub Release was treated as absent'
fi
for curl_mode in 403 500 network; do
  if MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE="$curl_mode" run_github_slot_check >/dev/null 2>&1; then
    fail "GitHub API failure $curl_mode was treated as absence"
  fi
done

MODELPORT_TEST_DOCKER_MODE=absent run_image_slot_check >/dev/null || \
  fail 'a confirmed 404 GHCR response was rejected'
MODELPORT_TEST_DOCKER_MODE=manifest_unknown run_image_slot_check >/dev/null || \
  fail 'a confirmed manifest-unknown GHCR response was rejected'
for docker_mode in \
  exists unauthorized network token_404 mixed_404_unauthorized mixed_manifest_network \
  version_404_unauthorized sha_404_unauthorized; do
  if MODELPORT_TEST_DOCKER_MODE="$docker_mode" run_image_slot_check >/dev/null 2>&1; then
    fail "GHCR state $docker_mode was treated as absence"
  fi
done

printf 'ModelPort release contract test passed\n'
