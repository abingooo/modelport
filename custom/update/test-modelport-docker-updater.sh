#!/bin/sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT

deploy_dir="${temporary_dir}/deploy"
bin_dir="${temporary_dir}/bin"
mkdir -p "${deploy_dir}/data" "${bin_dir}"
printf '%s\n' 'MODELPORT_IMAGE=ghcr.io/abingooo/modelport:old' 'SECRET=preserved' > "${deploy_dir}/.env"
printf '%s\n' 'services:' '  modelport:' '    image: ${MODELPORT_IMAGE}' > "${deploy_dir}/test-server-compose.yml"

printf '%s\n' '#!/bin/sh' 'printf '\''%s\n'\'' "$*" >> "${MODELPORT_TEST_DOCKER_LOG}"' > "${bin_dir}/docker"
printf '%s\n' '#!/bin/sh' 'exit 0' > "${bin_dir}/curl"
chmod 0755 "${bin_dir}/docker" "${bin_dir}/curl"

run_update() {
  MODELPORT_DEPLOY_DIR="${deploy_dir}" \
  MODELPORT_TEST_DOCKER_LOG="${temporary_dir}/docker.log" \
  PATH="${bin_dir}:${PATH}" \
    "${script_dir}/modelport-docker-updater"
}

printf '%s\n' '0.1.164.4-dev.14' > "${deploy_dir}/data/update-request"
run_update
grep -qx 'MODELPORT_IMAGE=ghcr.io/abingooo/modelport:0.1.164.4-dev.14' "${deploy_dir}/.env"
grep -qx 'SECRET=preserved' "${deploy_dir}/.env"
grep -q 'pull modelport' "${temporary_dir}/docker.log"
test ! -e "${deploy_dir}/data/update-request"

printf '%s\n' '0.1.164.5' > "${deploy_dir}/data/update-request"
run_update
grep -qx 'MODELPORT_IMAGE=ghcr.io/abingooo/modelport:custom-v0.1.164.5' "${deploy_dir}/.env"

printf '%s\n' 'v0.1.999' > "${deploy_dir}/data/update-request"
if run_update 2>/dev/null; then
  echo 'Official-style version was not rejected' >&2
  exit 1
fi
grep -qx 'MODELPORT_IMAGE=ghcr.io/abingooo/modelport:custom-v0.1.164.5' "${deploy_dir}/.env"
test -f "${deploy_dir}/data/update-request.failed"

echo 'modelport docker updater tests passed'
