#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

fail() {
  printf 'ModelPort compose contract test failed: %s\n' "$1" >&2
  exit 1
}

override=custom/server-compose.override.yml
service=custom/update/modelport-update.service
updater=custom/update/modelport-docker-updater

grep -Fqx '  sub2api:' "$override" || fail 'override must target the upstream sub2api service'
grep -Fqx '    image: ${MODELPORT_IMAGE:?Set MODELPORT_IMAGE to a pinned ModelPort image reference}' "$override" || \
  fail 'override must require an operator-provided image reference'
grep -Fqx '      - ${MODELPORT_DATA_DIR:-./data}:/app/data' "$override" || \
  fail 'container data must be visible at the host deployment data directory'
grep -Fqx '      - MODELPORT_UPDATE_MODE=${MODELPORT_UPDATE_MODE:-manual}' "$override" || \
  fail 'online updates must default to manual mode'
grep -Fqx '      - MODELPORT_UPDATE_REQUEST_FILE=${MODELPORT_UPDATE_REQUEST_FILE:-/app/data/update-request}' "$override" || \
  fail 'container request path must match the bind-mounted data directory'

if grep -Eq '(^|[[:space:]])image:.*:latest([[:space:]]|$)' "$override"; then
  fail 'ModelPort override must not use latest'
fi

grep -Fqx 'Environment=MODELPORT_COMPOSE_OVERRIDE_FILE=/opt/modelport/modelport-compose.override.yml' "$service" || \
  fail 'systemd service must load the released ModelPort compose override'
grep -Fqx 'Wants=network-online.target' "$service" || \
  fail 'systemd must actively wait for network-online before consuming a request'
grep -Fqx 'UMask=0077' "$service" || fail 'systemd updater must create secret files with a strict umask'
grep -Fqx 'TimeoutStartSec=20min' "$service" || fail 'systemd must allow the verified update health window'
grep -Fqx 'TimeoutStopSec=10min' "$service" || fail 'systemd must allow rollback to finish after termination'
grep -Fqx 'Environment=HOME=/var/cache/modelport-update' "$service" || \
  fail 'systemd must give cosign a writable home outside protected user homes'
grep -Fqx 'Environment=TUF_ROOT=/var/cache/modelport-update/sigstore-tuf' "$service" || \
  fail 'systemd must give Sigstore TUF a writable cache'
grep -Fqx 'CacheDirectory=modelport-update' "$service" || \
  fail 'systemd must provision the private Sigstore cache directory'
grep -Fqx 'CacheDirectoryMode=0700' "$service" || \
  fail 'the Sigstore cache directory must be private'
grep -Fqx 'ReadWritePaths=/opt/modelport /var/cache/modelport-update' "$service" || \
  fail 'the systemd write allowlist must include deployment and Sigstore cache paths'
grep -Fqx 'ProtectHome=true' "$service" || fail 'systemd must keep user homes protected'
grep -Fqx 'ProtectSystem=strict' "$service" || fail 'systemd must keep the host filesystem protected'
grep -Fq 'compose_override_file="${MODELPORT_COMPOSE_OVERRIDE_FILE:-}"' "$updater" || \
  fail 'updater must accept the compose override configured by systemd'
grep -Fqx 'umask 077' "$updater" || fail 'updater must set a strict umask before writing secrets'
grep -Fq 'MODELPORT_SETTINGS_URL' "$updater" || fail 'updater must verify the public ModelPort version'
grep -Fq "docker image inspect --format '{{.Id}}'" "$updater" || \
  fail 'updater must verify the running container image ID'

printf 'ModelPort compose contract test passed\n'
