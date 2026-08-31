#!/bin/sh
set -eu

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT

deploy_dir="${temporary_dir}/deploy"
bin_dir="${temporary_dir}/bin"
mkdir -p "${deploy_dir}/data" "${bin_dir}"
previous_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
previous_image_ref='ghcr.io/abingooo/modelport:custom-v0.1.176.2'
printf '%s\n' "MODELPORT_IMAGE=${previous_image_ref}" 'SECRET=preserved' > "${deploy_dir}/.env"
chmod 0600 "${deploy_dir}/.env"
printf '%s\n' 'services:' '  sub2api:' "    image: \${MODELPORT_IMAGE}" > "${deploy_dir}/docker-compose.yml"
printf '%s\n' 'services:' '  sub2api:' '    volumes:' '      - ./data:/app/data' > "${deploy_dir}/modelport-compose.override.yml"

digest='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
revision='0123456789abcdef0123456789abcdef01234567'
tag_object_sha='89abcdef89abcdef89abcdef89abcdef89abcdef'
image_id='sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee'
previous_image_id='sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff'
previous_version='0.1.176.2'
real_awk="$(command -v awk)"

cat > "${bin_dir}/docker" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "${MODELPORT_TEST_DOCKER_LOG}"
record_mode() {
  label="$1"
  path="$2"
  [ -e "${path}" ] || return 0
  set -- $(LC_ALL=C ls -ld "${path}")
  printf '%s %s\n' "${label}" "$1" >> "${MODELPORT_TEST_MODE_LOG}"
}
using_previous_image() {
  grep -Eq "(${MODELPORT_TEST_PREVIOUS_IMAGE_REF}|ghcr.io/abingooo/modelport@${MODELPORT_TEST_PREVIOUS_DIGEST})" \
    "${MODELPORT_TEST_DEPLOY_DIR}/.env"
}
case "$*" in
  'compose version') exit 0 ;;
  *'org.opencontainers.image.version'*) printf '%s\n' "${MODELPORT_TEST_LABEL_VERSION:-custom-v${MODELPORT_TEST_VERSION}}" ;;
  *'org.opencontainers.image.revision'*) printf '%s\n' "${MODELPORT_TEST_REVISION}" ;;
  *'org.opencontainers.image.source'*) printf '%s\n' "${MODELPORT_TEST_SOURCE:-https://github.com/abingooo/modelport}" ;;
  'image inspect --format {{.Id}} '* )
    case "$*" in
      *"${MODELPORT_TEST_PREVIOUS_DIGEST}"*|*"${MODELPORT_TEST_PREVIOUS_IMAGE_REF}"*)
        printf '%s\n' "${MODELPORT_TEST_PREVIOUS_IMAGE_ID}"
        ;;
      *) printf '%s\n' "${MODELPORT_TEST_IMAGE_ID}" ;;
    esac
    ;;
  'image inspect --format {{range .RepoDigests}}{{println .}}{{end}} '* )
    case "$*" in
      *"${MODELPORT_TEST_PREVIOUS_IMAGE_ID}"*|*"${MODELPORT_TEST_PREVIOUS_IMAGE_REF}"*)
        printf '%s@%s\n' 'ghcr.io/abingooo/modelport' "${MODELPORT_TEST_PREVIOUS_DIGEST}"
        ;;
      *) printf '%s@%s\n' 'ghcr.io/abingooo/modelport' "${MODELPORT_TEST_DIGEST}" ;;
    esac
    ;;
  *' ps -q sub2api')
    if [ "${MODELPORT_TEST_CONTAINER_MODE}" = missing_after_update ] && \
       grep -q "${MODELPORT_TEST_DIGEST}" "${MODELPORT_TEST_DEPLOY_DIR}/.env"; then
      :
    else
      printf '%s\n' "${MODELPORT_TEST_CONTAINER_ID}"
    fi
    ;;
  'inspect --format {{.State.Running}} '*) printf '%s\n' "${MODELPORT_TEST_CONTAINER_RUNNING}" ;;
  'inspect --format {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} '*)
    printf '%s\n' "${MODELPORT_TEST_CONTAINER_HEALTH}"
    ;;
  'inspect --format {{.Image}} '*)
    if using_previous_image; then
      printf '%s\n' "${MODELPORT_TEST_PREVIOUS_IMAGE_ID}"
    elif [ -n "${MODELPORT_TEST_RUNNING_IMAGE_ID_AFTER}" ]; then
      printf '%s\n' "${MODELPORT_TEST_RUNNING_IMAGE_ID_AFTER}"
    else
      printf '%s\n' "${MODELPORT_TEST_IMAGE_ID}"
    fi
    ;;
  *' up -d --no-deps sub2api')
    record_mode env "${MODELPORT_TEST_DEPLOY_DIR}/.env"
    for backup in "${MODELPORT_TEST_DEPLOY_DIR}"/run/env.before-update.*; do
      [ -e "${backup}" ] || continue
      record_mode backup "${backup}"
    done
    ;;
esac
EOF

cat > "${bin_dir}/curl" <<'EOF'
#!/bin/sh
set -eu
using_previous_image() {
  grep -Eq "(${MODELPORT_TEST_PREVIOUS_IMAGE_REF}|ghcr.io/abingooo/modelport@${MODELPORT_TEST_PREVIOUS_DIGEST})" \
    "${MODELPORT_TEST_DEPLOY_DIR}/.env"
}
case "$*" in
  *manifest-digest.txt*) printf '%s\n' "${MODELPORT_TEST_REMOTE_DIGEST:-${MODELPORT_TEST_DIGEST}}" ;;
  *release-revision.txt*) printf '%s\n' "${MODELPORT_TEST_REMOTE_REVISION:-${MODELPORT_TEST_REVISION}}" ;;
  *api.github.com/repos/abingooo/modelport/releases/tags/*)
    printf '{"tag_name":"custom-v%s","target_commitish":"%s","draft":false,"immutable":%s}\n' \
      "${MODELPORT_TEST_VERSION}" "${MODELPORT_TEST_REVISION}" "${MODELPORT_TEST_RELEASE_IMMUTABLE}"
    ;;
  *api.github.com/repos/abingooo/modelport/git/ref/tags/*)
    tag_ref_count=1
    [ "${MODELPORT_TEST_TAG_REF_DUPLICATE}" = true ] && tag_ref_count=2
    while [ "${tag_ref_count}" -gt 0 ]; do
      printf '{"ref":"%s","object":{"type":"%s","sha":"%s"}}\n' \
        "${MODELPORT_TEST_TAG_REF_NAME}" "${MODELPORT_TEST_TAG_REF_TYPE}" \
        "${MODELPORT_TEST_TAG_OBJECT_SHA}"
      tag_ref_count=$((tag_ref_count - 1))
    done
    ;;
  *api.github.com/repos/abingooo/modelport/git/tags/*)
    tag_object_count=1
    [ "${MODELPORT_TEST_TAG_OBJECT_DUPLICATE}" = true ] && tag_object_count=2
    while [ "${tag_object_count}" -gt 0 ]; do
      printf '{"sha":"%s","tag":"%s","object":{"type":"%s","sha":"%s"}}\n' \
        "${MODELPORT_TEST_TAG_OBJECT_SELF_SHA}" "${MODELPORT_TEST_TAG_NAME}" \
        "${MODELPORT_TEST_TAG_TARGET_TYPE}" "${MODELPORT_TEST_TAG_REVISION}"
      tag_object_count=$((tag_object_count - 1))
    done
    ;;
  *api/v1/settings/public*)
    if using_previous_image; then
      printf '{"data":{"version":"%s"}}\n' "${MODELPORT_TEST_PREVIOUS_VERSION}"
    elif [ -n "${MODELPORT_TEST_PUBLIC_VERSION_AFTER}" ]; then
      printf '{"data":{"version":"%s"}}\n' "${MODELPORT_TEST_PUBLIC_VERSION_AFTER}"
    else
      printf '{"data":{"version":"%s"}}\n' "${MODELPORT_TEST_VERSION}"
    fi
    ;;
  *) exit 0 ;;
esac
EOF
cat > "${bin_dir}/flock" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "${bin_dir}/cosign" <<'EOF'
#!/bin/sh
[ "${MODELPORT_TEST_COSIGN_MODE:-success}" = success ]
EOF
cat > "${bin_dir}/chmod" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "${bin_dir}/awk" <<'EOF'
#!/bin/sh
set -eu
for candidate in "${MODELPORT_TEST_DEPLOY_DIR}"/.env.tmp.*; do
  [ -e "${candidate}" ] || continue
  mode="$(LC_ALL=C ls -ld "${candidate}" | cut -c 1-10)"
  printf 'temporary %s\n' "${mode}" >> "${MODELPORT_TEST_MODE_LOG}"
done
exec "${MODELPORT_TEST_REAL_AWK}" "$@"
EOF
chmod 0755 "${bin_dir}/docker" "${bin_dir}/curl" "${bin_dir}/flock" "${bin_dir}/cosign" "${bin_dir}/chmod" "${bin_dir}/awk"

run_update() {
  env -i \
    MODELPORT_DEPLOY_DIR="${deploy_dir}" \
    MODELPORT_COMPOSE_OVERRIDE_FILE="${deploy_dir}/modelport-compose.override.yml" \
    MODELPORT_TEST_DOCKER_LOG="${temporary_dir}/docker.log" \
    MODELPORT_TEST_VERSION='0.1.183.1' \
    MODELPORT_TEST_DIGEST="${digest}" \
    MODELPORT_TEST_REVISION="${revision}" \
    MODELPORT_TEST_REMOTE_DIGEST='' \
    MODELPORT_TEST_REMOTE_REVISION='' \
    MODELPORT_TEST_LABEL_VERSION='' \
    MODELPORT_TEST_SOURCE='' \
    MODELPORT_TEST_RELEASE_IMMUTABLE=true \
    MODELPORT_TEST_TAG_REF_NAME=refs/tags/custom-v0.1.183.1 \
    MODELPORT_TEST_TAG_REF_TYPE=tag \
    MODELPORT_TEST_TAG_OBJECT_SHA="${tag_object_sha}" \
    MODELPORT_TEST_TAG_REF_DUPLICATE=false \
    MODELPORT_TEST_TAG_OBJECT_SELF_SHA="${tag_object_sha}" \
    MODELPORT_TEST_TAG_NAME=custom-v0.1.183.1 \
    MODELPORT_TEST_TAG_TARGET_TYPE=commit \
    MODELPORT_TEST_TAG_REVISION="${revision}" \
    MODELPORT_TEST_TAG_OBJECT_DUPLICATE=false \
    MODELPORT_TEST_COSIGN_MODE=success \
    MODELPORT_TEST_IMAGE_ID="${image_id}" \
    MODELPORT_TEST_PREVIOUS_DIGEST="${previous_digest}" \
    MODELPORT_TEST_PREVIOUS_IMAGE_REF="${previous_image_ref}" \
    MODELPORT_TEST_PREVIOUS_IMAGE_ID="${previous_image_id}" \
    MODELPORT_TEST_PREVIOUS_VERSION="${previous_version}" \
    MODELPORT_TEST_RUNNING_IMAGE_ID_AFTER='' \
    MODELPORT_TEST_CONTAINER_ID=modelport-test-container \
    MODELPORT_TEST_CONTAINER_MODE=normal \
    MODELPORT_TEST_CONTAINER_RUNNING=true \
    MODELPORT_TEST_CONTAINER_HEALTH=healthy \
    MODELPORT_TEST_PUBLIC_VERSION_AFTER='' \
    MODELPORT_TEST_DEPLOY_DIR="${deploy_dir}" \
    MODELPORT_TEST_MODE_LOG="${temporary_dir}/modes.log" \
    MODELPORT_TEST_REAL_AWK="${real_awk}" \
    PATH="${bin_dir}:${PATH}" \
    "$@" \
    "${script_dir}/modelport-docker-updater"
}

expect_update_failure() {
  description="$1"
  expected_message="$2"
  shift 2
  stderr_file="${temporary_dir}/update.stderr"
  if run_update "$@" 2>"${stderr_file}"; then
    echo "${description} was not rejected" >&2
    exit 1
  fi
  if ! grep -Fqx "${expected_message}" "${stderr_file}"; then
    echo "${description} failed for the wrong reason; expected: ${expected_message}" >&2
    sed 's/^/  /' "${stderr_file}" >&2
    exit 1
  fi
}

printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
run_update
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
grep -qx 'SECRET=preserved' "${deploy_dir}/.env"
grep -q '^pull ghcr.io/abingooo/modelport:custom-v0.1.183.1$' "${temporary_dir}/docker.log"
grep -q 'compose --env-file .* -f .*docker-compose.yml -f .*modelport-compose.override.yml pull sub2api' "${temporary_dir}/docker.log"
grep -qx 'temporary -rw-------' "${temporary_dir}/modes.log"
grep -qx 'env -rw-------' "${temporary_dir}/modes.log"
grep -qx 'backup -rw-------' "${temporary_dir}/modes.log"
# shellcheck disable=SC2046 # Split the stable ls fields to inspect the mode.
set -- $(LC_ALL=C ls -ld "${deploy_dir}/.env")
[ "$1" = '-rw-------' ] || { echo 'Updated env permissions are not private' >&2; exit 1; }
test ! -e "${deploy_dir}/data/update-request"
test ! -e "${deploy_dir}/data/update-request.processing"

printf '%s\n' 'v0.1.999' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Official-style version' \
  'Rejected ModelPort version: v0.1.999'
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mutable GitHub Release' \
  'Release custom-v0.1.183.1 is not immutable or does not match its revision' \
  MODELPORT_TEST_RELEASE_IMMUTABLE=false
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mismatched annotated tag revision' \
  "Annotated Git tag custom-v0.1.183.1 does not resolve uniquely to ${revision}" \
  MODELPORT_TEST_TAG_REVISION=89abcdef0123456789abcdef0123456789abcdef
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Wrong Git tag ref' \
  'Git tag ref refs/tags/custom-v0.1.183.1 is not the expected annotated tag' \
  MODELPORT_TEST_TAG_REF_NAME=refs/tags/custom-v0.1.183.2
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Lightweight Git tag ref' \
  'Git tag ref refs/tags/custom-v0.1.183.1 is not the expected annotated tag' \
  MODELPORT_TEST_TAG_REF_TYPE=commit
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Ambiguous Git tag ref response' \
  'Git tag ref refs/tags/custom-v0.1.183.1 is not the expected annotated tag' \
  MODELPORT_TEST_TAG_REF_DUPLICATE=true
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mismatched annotated tag identity' \
  "Annotated Git tag custom-v0.1.183.1 does not resolve uniquely to ${revision}" \
  MODELPORT_TEST_TAG_OBJECT_SELF_SHA=abcdef0123456789abcdef0123456789abcdef01
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Wrong annotated tag name' \
  "Annotated Git tag custom-v0.1.183.1 does not resolve uniquely to ${revision}" \
  MODELPORT_TEST_TAG_NAME=custom-v0.1.183.2
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Annotated tag with non-commit target' \
  "Annotated Git tag custom-v0.1.183.1 does not resolve uniquely to ${revision}" \
  MODELPORT_TEST_TAG_TARGET_TYPE=tree
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Ambiguous annotated tag response' \
  "Annotated Git tag custom-v0.1.183.1 does not resolve uniquely to ${revision}" \
  MODELPORT_TEST_TAG_OBJECT_DUPLICATE=true
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Invalid image signature' \
  'Image signature verification failed for custom-v0.1.183.1' \
  MODELPORT_TEST_COSIGN_MODE=failure
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mismatched release revision' \
  'Release custom-v0.1.183.1 is not immutable or does not match its revision' \
  MODELPORT_TEST_REMOTE_REVISION=89abcdef0123456789abcdef0123456789abcdef
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mismatched release digest' \
  'Image digest mismatch for custom-v0.1.183.1' \
  MODELPORT_TEST_REMOTE_DIGEST=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Mismatched image source label' \
  'Image source label mismatch' \
  MODELPORT_TEST_SOURCE=https://github.com/example/untrusted
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${digest}" "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' "MODELPORT_IMAGE=${previous_image_ref}" 'SECRET=preserved' > "${deploy_dir}/.env"
chmod 0600 "${deploy_dir}/.env"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Missing target Compose container' \
  'Expected exactly one Compose container for sub2api, found 0' \
  MODELPORT_TEST_CONTAINER_MODE=missing_after_update
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${previous_digest}" "${deploy_dir}/.env"
grep -qx 'SECRET=preserved' "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Wrong running image' \
  "Running container image does not match ${digest}" \
  MODELPORT_TEST_RUNNING_IMAGE_ID_AFTER=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${previous_digest}" "${deploy_dir}/.env"
grep -qx 'SECRET=preserved' "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

rm -f "${deploy_dir}/data/update-request.failed"
printf '%s\n' '0.1.183.1' > "${deploy_dir}/data/update-request"
expect_update_failure \
  'Stale public version response' \
  'Running ModelPort version 0.1.182.9 does not match 0.1.183.1' \
  MODELPORT_TEST_PUBLIC_VERSION_AFTER=0.1.182.9
grep -qx "MODELPORT_IMAGE=ghcr.io/abingooo/modelport@${previous_digest}" "${deploy_dir}/.env"
grep -qx 'SECRET=preserved' "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

echo 'modelport docker updater tests passed'
