#!/usr/bin/env bash
set -Eeuo pipefail

umask 077
export LC_ALL=C

readonly DEFAULT_REDIS_IMAGE='redis:8-alpine@sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576'
readonly REPORT_SCHEMA_VERSION=1
readonly AOF_CHECK_COPY_TARGET='/data/.modelport-legacy-aof-check-copy'
readonly RESTORE_RUN_LABEL='com.abingooo.modelport.restore-run'
SCRIPT_DIR="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -P -- "$SCRIPT_DIR/.." && pwd)"

SOURCE_KIND=''
INPUT_PATH=''
INPUT_FD_PATH=''
INPUT_IDENTITY=''
VERIFIED_INPUT_PATH=''
VERIFIED_INPUT_SHA256=''
REPORT_PATH=''
EXPECTED_SHA256=''
EXPECTED_REDIS_MAJOR=''
RPO_AT=''
REDIS_IMAGE="$DEFAULT_REDIS_IMAGE"
SENSITIVE_STORAGE_CONFIRMED=0
LEGACY_AOF_CONFIRMED=0

STARTED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
STARTED_EPOCH="$(date -u '+%s')"
WORK_DIR=''
REPORT_TEMP=''
INPUT_SHA256=''
INPUT_SIZE=''
INPUT_MTIME_EPOCH=''
INPUT_MTIME_UTC=''
RPO_EPOCH=''
REDIS_IMAGE_ID=''
COPY_TARGET=''
COPY_OWNER=''
CHECKER=''
REDIS_DATABASES=16

REPORT_BASENAME=''
REPORT_PARENT_PHYSICAL=''
REPORT_PARENT_ID=''
REPORT_PUBLISHED=0
REPORT_PUBLISHED_IDENTITY=''

NETWORK_NAME=''
DATA_VOLUME=''
DATA_MOUNT_RW=''
DATA_MOUNT_RO=''
INIT_CONTAINER=''
CHECK_CONTAINER=''
HASH_CONTAINER=''
REDIS_CONTAINER=''
NETWORK_CREATED=0
VOLUME_CREATED=0
INIT_CREATED=0
CHECK_MAY_EXIST=0
HASH_MAY_EXIST=0
REDIS_CREATED=0
RESTORE_RUN_ID=''

usage() {
  cat >&2 <<'EOF'
Usage:
  deploy/modelport-isolated-redis-restore.sh \
    --source-kind rdb|legacy-aof \
    --input /absolute/path/to/local-backup \
    --expected-sha256 64-lowercase-hex \
    --expected-redis-major positive-integer \
    --rpo-at YYYY-MM-DDTHH:MM:SSZ \
    --report /absolute/path/to/new-report.json \
    [--redis-image repository@sha256:64-lowercase-hex] \
    [--confirm-complete-legacy-aof] \
    --confirm-sensitive-storage

Only a local regular RDB or a complete old-style legacy single-file AOF is
accepted. --confirm-complete-legacy-aof is mandatory for legacy-aof and confirms
capture provenance; Redis multipart base/incremental members are unsupported.
Remote URLs, Redis endpoints, credentials, existing containers, and existing
volumes are intentionally not configurable.

--rpo-at is the independently recorded persistence boundary. For RDB it should
match the LASTSAVE observation associated with the completed source file. For
legacy AOF it should be the end of the operator's consistent capture window.

--confirm-sensitive-storage confirms that the input, report destination, and
Docker data root use approved encrypted storage with sufficient capacity. The
Redis image must already be present locally and pinned by digest; it is never
pulled by this tool.
EOF
}

validate_local_docker_endpoint() {
  local docker_context=''
  local docker_endpoint=''
  local socket_path=''

  [[ -z "${DOCKER_HOST:-}" ]] || return 1
  docker_context="$(docker context show 2>"$WORK_DIR/docker-context-show.log")" || return 1
  [[ -n "$docker_context" && "$docker_context" != *$'\n'* && "$docker_context" != *$'\r'* ]] || return 1
  docker_endpoint="$(docker context inspect --format '{{ .Endpoints.docker.Host }}' \
    "$docker_context" 2>"$WORK_DIR/docker-context-inspect.log")" || return 1
  [[ "$docker_endpoint" == unix:///* && "$docker_endpoint" != *$'\n'* && "$docker_endpoint" != *$'\r'* ]] || return 1
  socket_path="${docker_endpoint#unix://}"
  [[ "$socket_path" == /* && -S "$socket_path" ]]
}

say_error() {
  printf 'ModelPort isolated Redis restore failed [%s]: %s\n' "$1" "$2" >&2
}

fail() {
  say_error "$1" "$2"
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

path_is_within() {
  local candidate="$1"
  local root="$2"
  [[ "$candidate" == "$root" || "$candidate" == "$root/"* ]]
}

directory_identity() {
  local path="$1"
  local value=''
  if value="$(stat -c '%d:%i' -- "$path" 2>/dev/null)" && [[ "$value" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  value="$(stat -f '%d:%i' -- "$path" 2>/dev/null)" || return 1
  [[ "$value" =~ ^[0-9]+:[0-9]+$ ]] || return 1
  printf '%s' "$value"
}

report_parent_is_bound() {
  local current_id=''
  local named_id=''
  current_id="$(directory_identity .)" || return 1
  named_id="$(directory_identity "$REPORT_PARENT_PHYSICAL")" || return 1
  [[ "$current_id" == "$REPORT_PARENT_ID" && "$named_id" == "$REPORT_PARENT_ID" ]]
}

file_identity() {
  local path="$1"
  local value=''
  if value="$(stat -c '%d:%i' -- "$path" 2>/dev/null)" && [[ "$value" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  value="$(stat -f '%d:%i' -- "$path" 2>/dev/null)" || return 1
  [[ "$value" =~ ^[0-9]+:[0-9]+$ ]] || return 1
  printf '%s' "$value"
}

open_input_read() {
  local current_identity=''
  exec 8<"$INPUT_PATH" || return 1
  current_identity="$(file_identity /dev/fd/8)" || return 1
  [[ "$current_identity" == "$INPUT_IDENTITY" ]] || return 1
  INPUT_FD_PATH='/dev/fd/8'
}

sha256_file() {
  if command_exists sha256sum; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

file_mtime_epoch() {
  local path="$1"
  local value=''
  if value="$(stat -c '%Y' -- "$path" 2>/dev/null)" && [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  value="$(stat -f '%m' -- "$path" 2>/dev/null)" || return 1
  [[ "$value" =~ ^[0-9]+$ ]] || return 1
  printf '%s' "$value"
}

epoch_to_utc() {
  local epoch="$1"
  local value=''
  if value="$(date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null)"; then
    printf '%s' "$value"
    return 0
  fi
  date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ'
}

rfc3339_to_epoch() {
  local value="$1"
  local epoch=''
  local canonical=''
  [[ "$value" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || return 1
  if epoch="$(date -u -d "$value" '+%s' 2>/dev/null)"; then
    :
  elif epoch="$(date -j -u -f '%Y-%m-%dT%H:%M:%SZ' "$value" '+%s' 2>/dev/null)"; then
    :
  else
    return 1
  fi
  [[ "$epoch" =~ ^[0-9]+$ ]] || return 1
  canonical="$(epoch_to_utc "$epoch")" || return 1
  [[ "$canonical" == "$value" ]] || return 1
  printf '%s' "$epoch"
}

info_value() {
  local path="$1"
  local key="$2"
  awk -v key="$key" '
    index($0, key ":") == 1 {
      value = substr($0, length(key) + 2)
      sub(/\r$/, "", value)
      print value
      exit
    }
  ' "$path"
}

docker_resource_is_absent() {
  local resource_type="$1"
  local resource_name="$2"
  local names=''
  case "$resource_type" in
    container)
      names="$(docker container ls --all --format '{{.Names}}' 2>/dev/null)" || return 1
      ;;
    network)
      names="$(docker network ls --format '{{.Name}}' 2>/dev/null)" || return 1
      ;;
    volume)
      names="$(docker volume ls --format '{{.Name}}' 2>/dev/null)" || return 1
      ;;
    *) return 1 ;;
  esac
  awk -v expected="$resource_name" '
    $0 == expected { found = 1 }
    END { exit found ? 1 : 0 }
  ' <<<"$names"
}

docker_resource_is_owned() {
  local resource_type="$1"
  local resource_name="$2"
  local owner=''
  [[ -n "$RESTORE_RUN_ID" ]] || return 1
  case "$resource_type" in
    container)
      owner="$(docker container inspect --format \
        '{{ index .Config.Labels "com.abingooo.modelport.restore-run" }}' \
        "$resource_name" 2>/dev/null)" || return 1
      ;;
    network)
      owner="$(docker network inspect --format \
        '{{ index .Labels "com.abingooo.modelport.restore-run" }}' \
        "$resource_name" 2>/dev/null)" || return 1
      ;;
    volume)
      owner="$(docker volume inspect --format \
        '{{ index .Labels "com.abingooo.modelport.restore-run" }}' \
        "$resource_name" 2>/dev/null)" || return 1
      ;;
    *) return 1 ;;
  esac
  [[ "$owner" == "$RESTORE_RUN_ID" ]]
}

remove_container_exact() {
  local name="$1"
  docker_resource_is_absent container "$name" && return 0
  docker_resource_is_owned container "$name" || return 1
  docker rm --force "$name" >/dev/null 2>&1 || docker_resource_is_absent container "$name"
}

remove_network_exact() {
  local name="$1"
  docker_resource_is_absent network "$name" && return 0
  docker_resource_is_owned network "$name" || return 1
  docker network rm "$name" >/dev/null 2>&1 || docker_resource_is_absent network "$name"
}

remove_volume_exact() {
  local name="$1"
  docker_resource_is_absent volume "$name" && return 0
  docker_resource_is_owned volume "$name" || return 1
  docker volume rm "$name" >/dev/null 2>&1 || docker_resource_is_absent volume "$name"
}

cleanup_docker() {
  local failed=0

  if [[ "$CHECK_MAY_EXIST" -eq 1 ]]; then
    if remove_container_exact "$CHECK_CONTAINER"; then
      CHECK_MAY_EXIST=0
    else
      failed=1
    fi
  fi
  if [[ "$HASH_MAY_EXIST" -eq 1 ]]; then
    if remove_container_exact "$HASH_CONTAINER"; then
      HASH_MAY_EXIST=0
    else
      failed=1
    fi
  fi
  if [[ "$REDIS_CREATED" -eq 1 ]]; then
    if remove_container_exact "$REDIS_CONTAINER"; then
      REDIS_CREATED=0
    else
      failed=1
    fi
  fi
  if [[ "$INIT_CREATED" -eq 1 ]]; then
    if remove_container_exact "$INIT_CONTAINER"; then
      INIT_CREATED=0
    else
      failed=1
    fi
  fi
  if [[ "$NETWORK_CREATED" -eq 1 ]]; then
    if remove_network_exact "$NETWORK_NAME"; then
      NETWORK_CREATED=0
    else
      failed=1
    fi
  fi
  if [[ "$VOLUME_CREATED" -eq 1 ]]; then
    if remove_volume_exact "$DATA_VOLUME"; then
      VOLUME_CREATED=0
    else
      failed=1
    fi
  fi
  return "$failed"
}

cleanup_work_dir() {
  if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
    rm -rf -- "$WORK_DIR" || return 1
    WORK_DIR=''
  fi
}

cleanup_local() {
  local failed=0
  if [[ -n "$REPORT_TEMP" && -e "$REPORT_TEMP" ]]; then
    if rm -f -- "$REPORT_TEMP"; then
      REPORT_TEMP=''
    else
      failed=1
    fi
  fi
  if ! cleanup_work_dir; then
    failed=1
  fi
  return "$failed"
}

remove_published_report() {
  local current_identity=''
  [[ "$REPORT_PUBLISHED" -eq 1 ]] || return 0
  if [[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]]; then
    REPORT_PUBLISHED=0
    REPORT_PUBLISHED_IDENTITY=''
    return 0
  fi
  [[ -n "$REPORT_PUBLISHED_IDENTITY" && ! -L "./$REPORT_BASENAME" ]] || return 1
  current_identity="$(file_identity "./$REPORT_BASENAME")" || return 1
  [[ "$current_identity" == "$REPORT_PUBLISHED_IDENTITY" ]] || return 1
  rm -f -- "./$REPORT_BASENAME" || return 1
  REPORT_PUBLISHED=0
  REPORT_PUBLISHED_IDENTITY=''
}

on_exit() {
  local status=$?
  trap - EXIT
  set +e
  if ! cleanup_docker; then
    say_error cleanup_failed 'one or more exact-name Docker resources could not be removed'
    status=1
  fi
  if ! cleanup_local; then
    say_error cleanup_failed 'one or more local staging artifacts could not be removed'
    status=1
  fi
  if [[ "$status" -ne 0 && "$REPORT_PUBLISHED" -eq 1 ]]; then
    if ! remove_published_report; then
      say_error cleanup_failed 'the published report could not be removed after a failed run'
      status=1
    fi
  fi
  exit "$status"
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source-kind)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      SOURCE_KIND="$2"
      shift 2
      ;;
    --input)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      INPUT_PATH="$2"
      shift 2
      ;;
    --expected-sha256)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      EXPECTED_SHA256="$2"
      shift 2
      ;;
    --expected-redis-major)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      EXPECTED_REDIS_MAJOR="$2"
      shift 2
      ;;
    --rpo-at)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      RPO_AT="$2"
      shift 2
      ;;
    --report)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      REPORT_PATH="$2"
      shift 2
      ;;
    --redis-image)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      REDIS_IMAGE="$2"
      shift 2
      ;;
    --confirm-sensitive-storage)
      SENSITIVE_STORAGE_CONFIRMED=1
      shift
      ;;
    --confirm-complete-legacy-aof)
      LEGACY_AOF_CONFIRMED=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      usage
      printf 'Unknown option: %s\n' "$1" >&2
      exit 2
      ;;
  esac
done

case "$SOURCE_KIND" in
  rdb)
    CHECKER='redis-check-rdb'
    COPY_TARGET='/data/dump.rdb'
    ;;
  legacy-aof)
    CHECKER='redis-check-aof'
    ;;
  '') usage; printf '%s\n' '--source-kind is required' >&2; exit 2 ;;
  *) usage; printf '%s\n' '--source-kind must be rdb or legacy-aof' >&2; exit 2 ;;
esac

[[ "$SENSITIVE_STORAGE_CONFIRMED" -eq 1 ]] || {
  printf '%s\n' '--confirm-sensitive-storage is required' >&2
  exit 2
}
if [[ "$SOURCE_KIND" == 'legacy-aof' && "$LEGACY_AOF_CONFIRMED" -ne 1 ]]; then
  printf '%s\n' '--confirm-complete-legacy-aof is required for --source-kind legacy-aof' >&2
  exit 2
fi
if [[ "$SOURCE_KIND" == 'rdb' && "$LEGACY_AOF_CONFIRMED" -eq 1 ]]; then
  printf '%s\n' '--confirm-complete-legacy-aof is only valid with --source-kind legacy-aof' >&2
  exit 2
fi
[[ "$INPUT_PATH" == /* ]] || { printf '%s\n' '--input must be an absolute path' >&2; exit 2; }
[[ -f "$INPUT_PATH" && ! -L "$INPUT_PATH" && -r "$INPUT_PATH" && -s "$INPUT_PATH" ]] || {
  printf '%s\n' '--input must be a readable, non-empty, regular, non-symbolic-link file' >&2
  exit 2
}
[[ "$EXPECTED_SHA256" =~ ^[0-9a-f]{64}$ ]] || {
  printf '%s\n' '--expected-sha256 must contain exactly 64 lowercase hexadecimal characters' >&2
  exit 2
}
[[ "$EXPECTED_REDIS_MAJOR" =~ ^[0-9]+$ && "$EXPECTED_REDIS_MAJOR" -gt 0 ]] || {
  printf '%s\n' '--expected-redis-major must be a positive integer' >&2
  exit 2
}
[[ "$REPORT_PATH" == /* ]] || { printf '%s\n' '--report must be an absolute path' >&2; exit 2; }
[[ ! -e "$REPORT_PATH" && ! -L "$REPORT_PATH" ]] || {
  printf '%s\n' '--report must not already exist or be a symbolic link' >&2
  exit 2
}
[[ -d "$(dirname -- "$REPORT_PATH")" ]] || {
  printf '%s\n' '--report parent directory does not exist' >&2
  exit 2
}
case "$INPUT_PATH$REPORT_PATH" in
  *$'\n'*|*$'\r'*) printf '%s\n' 'input and report paths must not contain control characters' >&2; exit 2 ;;
esac
if [[ "$SOURCE_KIND" == 'legacy-aof' ]]; then
  input_basename="${INPUT_PATH##*/}"
  case "$input_basename" in
    appendonly.aof.manifest|*.base.aof|*.base.rdb|*.incr.aof)
      printf '%s\n' 'Redis multipart AOF manifest/base/incremental members are unsupported' >&2
      exit 2
      ;;
  esac
fi
[[ "$REDIS_IMAGE" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[0-9a-f]{64}$ ]] || {
  printf '%s\n' '--redis-image must be a digest-pinned local image reference' >&2
  exit 2
}
RPO_EPOCH="$(rfc3339_to_epoch "$RPO_AT")" || {
  printf '%s\n' '--rpo-at must be a real UTC timestamp formatted as YYYY-MM-DDTHH:MM:SSZ' >&2
  exit 2
}
if (( RPO_EPOCH > STARTED_EPOCH )); then
  printf '%s\n' '--rpo-at must not be in the future' >&2
  exit 2
fi

input_parent="$(cd -P -- "$(dirname -- "$INPUT_PATH")" && pwd)" || {
  printf '%s\n' 'could not resolve the physical input parent directory' >&2
  exit 2
}
report_parent="$(cd -P -- "$(dirname -- "$REPORT_PATH")" && pwd)" || {
  printf '%s\n' 'could not resolve the physical report parent directory' >&2
  exit 2
}
physical_input="$input_parent/${INPUT_PATH##*/}"
physical_report="$report_parent/${REPORT_PATH##*/}"
if path_is_within "$physical_input" "$REPO_ROOT" || path_is_within "$physical_report" "$REPO_ROOT"; then
  printf '%s\n' 'input and report paths must be outside the repository tree' >&2
  exit 2
fi
INPUT_PATH="$physical_input"

REPORT_BASENAME="${REPORT_PATH##*/}"
REPORT_PARENT_PHYSICAL="$report_parent"
REPORT_PATH="$physical_report"
REPORT_PARENT_ID="$(directory_identity "$REPORT_PARENT_PHYSICAL")" || {
  printf '%s\n' 'could not identify the physical report parent directory' >&2
  exit 2
}
cd -P -- "$REPORT_PARENT_PHYSICAL" || {
  printf '%s\n' 'could not bind the physical report parent directory' >&2
  exit 2
}
report_parent_is_bound || {
  printf '%s\n' 'the physical report parent directory changed during validation' >&2
  exit 2
}
[[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]] || {
  printf '%s\n' '--report must not already exist or be a symbolic link' >&2
  exit 2
}

for dependency in docker jq awk od wc tr sed date stat dirname basename mktemp chmod ln rm cp sleep id; do
  command_exists "$dependency" || {
    printf 'required command is unavailable: %s\n' "$dependency" >&2
    exit 1
  }
done
if ! command_exists sha256sum && ! command_exists shasum; then
  printf '%s\n' 'sha256sum or shasum is required' >&2
  exit 1
fi

WORK_DIR="$(mktemp -d './.modelport-isolated-redis-restore-work.XXXXXX')"
chmod 0700 "$WORK_DIR"
WORK_DIR="$REPORT_PARENT_PHYSICAL/${WORK_DIR#./}"

if ! exec 9<"$INPUT_PATH"; then
  fail input_unreadable 'could not open the local input'
fi
if [[ ! -f /dev/fd/9 || ! -r /dev/fd/9 || ! -s /dev/fd/9 ]]; then
  fail input_unreadable 'the opened local input is not a readable, non-empty regular file'
fi
INPUT_IDENTITY="$(file_identity /dev/fd/9)" || fail input_unreadable 'could not identify the opened local input'
[[ -n "$INPUT_IDENTITY" ]] || fail input_unreadable 'the opened local input identity was empty'

open_input_read || fail input_changed 'the local input identity changed before hashing'
source_sha256_before="$(sha256_file "$INPUT_FD_PATH" 2>"$WORK_DIR/input-hash.log")" || \
  fail input_unreadable 'could not hash the local input'
[[ "$source_sha256_before" =~ ^[0-9a-f]{64}$ ]] || fail input_unreadable 'the local input hash was invalid'
open_input_read || fail input_changed 'the local input identity changed before sizing'
source_size_before="$(wc -c < "$INPUT_FD_PATH" | tr -d '[:space:]')"
[[ "$source_size_before" =~ ^[0-9]+$ && "$source_size_before" -gt 0 ]] || \
  fail input_unreadable 'the local input size was invalid'
open_input_read || fail input_changed 'the local input identity changed before reading its modification time'
INPUT_MTIME_EPOCH="$(file_mtime_epoch "$INPUT_FD_PATH")" || fail input_unreadable 'could not read the local input modification time'
INPUT_MTIME_UTC="$(epoch_to_utc "$INPUT_MTIME_EPOCH")" || fail input_unreadable 'could not format the local input modification time'

VERIFIED_INPUT_PATH="$WORK_DIR/verified-input.backup"
open_input_read || fail input_changed 'the local input identity changed before creating the stable copy'
cp -- "$INPUT_FD_PATH" "$VERIFIED_INPUT_PATH" || fail input_unreadable 'could not create the stable input copy'
chmod 0400 "$VERIFIED_INPUT_PATH" || fail input_unreadable 'could not protect the stable input copy'
VERIFIED_INPUT_SHA256="$(sha256_file "$VERIFIED_INPUT_PATH")" || fail input_unreadable 'could not hash the stable input copy'
INPUT_SIZE="$(wc -c < "$VERIFIED_INPUT_PATH" | tr -d '[:space:]')"
[[ "$VERIFIED_INPUT_SHA256" =~ ^[0-9a-f]{64}$ && "$INPUT_SIZE" =~ ^[0-9]+$ && "$INPUT_SIZE" -gt 0 ]] || \
  fail input_unreadable 'the stable input copy metadata was invalid'
if [[ "$VERIFIED_INPUT_SHA256" != "$source_sha256_before" || "$INPUT_SIZE" != "$source_size_before" ]]; then
  fail input_changed 'the local input changed while the stable copy was created'
fi
INPUT_SHA256="$VERIFIED_INPUT_SHA256"
[[ "$INPUT_SHA256" == "$EXPECTED_SHA256" ]] || fail input_checksum_mismatch 'the stable input copy does not match --expected-sha256'

magic="$(od -An -tx1 -N5 "$VERIFIED_INPUT_PATH" | tr -d '[:space:]')"
if [[ "$SOURCE_KIND" == 'rdb' && "$magic" != '5245444953' ]]; then
  fail format_mismatch 'the RDB input does not start with the Redis RDB signature'
fi
if [[ "$SOURCE_KIND" == 'legacy-aof' ]]; then
  if [[ "$magic" == '5245444953' ]]; then
    aof_base_name='appendonly.aof.1.base.rdb'
  else
    aof_base_name='appendonly.aof.1.base.aof'
  fi
  COPY_TARGET="/data/appendonlydir/$aof_base_name"
fi

validate_local_docker_endpoint || \
  fail docker_endpoint_invalid 'Docker must use an existing local Unix-socket context and DOCKER_HOST must be unset'

REDIS_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$REDIS_IMAGE" 2>"$WORK_DIR/image-inspect.log")" || \
  fail image_unavailable 'the digest-pinned Redis image is not preloaded locally'
[[ "$REDIS_IMAGE_ID" =~ ^sha256:[0-9a-f]{64}$ ]] || fail image_unavailable 'the Redis image resolved to an invalid image ID'

suffix="${WORK_DIR##*.}"
[[ "$suffix" =~ ^[A-Za-z0-9]+$ ]] || fail isolation_setup_failed 'could not derive safe random resource names'
RESTORE_RUN_ID="$suffix"
NETWORK_NAME="modelport-redis-restore-net-$suffix"
DATA_VOLUME="modelport-redis-restore-data-$suffix"
DATA_MOUNT_RW="type=volume,source=$DATA_VOLUME,target=/data,volume-nocopy"
DATA_MOUNT_RO="$DATA_MOUNT_RW,readonly"
INIT_CONTAINER="modelport-redis-restore-init-$suffix"
CHECK_CONTAINER="modelport-redis-restore-check-$suffix"
HASH_CONTAINER="modelport-redis-restore-hash-$suffix"
REDIS_CONTAINER="modelport-redis-restore-server-$suffix"

NETWORK_CREATED=1
if ! docker network create \
  --internal \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  "$NETWORK_NAME" >"$WORK_DIR/network-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the internal Docker network'
fi
VOLUME_CREATED=1
if ! docker volume create \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  "$DATA_VOLUME" >"$WORK_DIR/volume-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the disposable Redis data volume'
fi

host_uid="$(id -u)"
host_gid="$(id -g)"
[[ "$host_uid" =~ ^[0-9]+$ && "$host_gid" =~ ^[0-9]+$ ]] || fail isolation_setup_failed 'could not resolve the local numeric user identity'

init_script='set -eu; chmod 0777 /data; : > /tmp/.modelport-init-ready; exec tail -f /dev/null'
if [[ "$SOURCE_KIND" == 'rdb' ]]; then
  finalize_script='chmod 0444 /data/dump.rdb'
  docker_copy_path='/data/dump.rdb'
else
  finalize_script="set -eu; mkdir -p /data/appendonlydir; mv /data/input.aof '$COPY_TARGET'; cp '$COPY_TARGET' '$AOF_CHECK_COPY_TARGET'; chmod 0777 /data/appendonlydir; chmod 0444 '$COPY_TARGET'; chmod 0600 '$AOF_CHECK_COPY_TARGET'; printf 'file %s seq 1 type b\\n' '$aof_base_name' > /data/appendonlydir/appendonly.aof.manifest; chmod 0666 /data/appendonlydir/appendonly.aof.manifest"
  docker_copy_path='/data/input.aof'
fi
docker_copy_target="$INIT_CONTAINER:$docker_copy_path"

INIT_CREATED=1
if ! docker create \
  --name "$INIT_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network none \
  --pull=never \
  --read-only \
  --user '0:0' \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 64 \
  --mount "$DATA_MOUNT_RW" \
  --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=16m,mode=0700' \
  --entrypoint sh \
  "$REDIS_IMAGE_ID" -c "$init_script" \
  >"$WORK_DIR/init-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the disposable input initializer'
fi
if ! docker start "$INIT_CONTAINER" >"$WORK_DIR/init-start.log" 2>&1; then
  fail isolation_setup_failed 'could not start the disposable input initializer'
fi
init_ready=0
init_attempt=0
while (( init_attempt < 30 )); do
  if docker exec "$INIT_CONTAINER" test -f /tmp/.modelport-init-ready >/dev/null 2>&1; then
    init_ready=1
    break
  fi
  sleep 1
  init_attempt=$((init_attempt + 1))
done
[[ "$init_ready" -eq 1 ]] || fail isolation_setup_failed 'the disposable input initializer did not become ready'
if ! docker cp "$VERIFIED_INPUT_PATH" "$docker_copy_target" >"$WORK_DIR/input-copy.log" 2>&1; then
  fail isolation_setup_failed 'could not copy the input into the disposable volume'
fi
COPY_OWNER="$(docker exec "$INIT_CONTAINER" stat -c '%u:%g' "$docker_copy_path" 2>"$WORK_DIR/input-owner.log")" || \
  fail isolation_setup_failed 'could not identify the copied input owner'
[[ "$COPY_OWNER" =~ ^[0-9]+:[0-9]+$ ]] || fail isolation_setup_failed 'the copied input owner was invalid'
if ! docker exec --user "$COPY_OWNER" "$INIT_CONTAINER" sh -c "$finalize_script" \
  >"$WORK_DIR/input-finalize.log" 2>&1; then
  fail isolation_setup_failed 'could not prepare the copied input for isolated restore'
fi
if ! docker rm --force "$INIT_CONTAINER" >"$WORK_DIR/init-remove.log" 2>&1; then
  fail cleanup_failed 'could not remove the disposable input initializer'
fi
INIT_CREATED=0

HASH_MAY_EXIST=1
copied_sha256="$(docker run --rm \
  --name "$HASH_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network none \
  --pull=never \
  --read-only \
  --user "$host_uid:$host_gid" \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 32 \
  --mount "$DATA_MOUNT_RO" \
  --entrypoint sha256sum \
  "$REDIS_IMAGE_ID" "$COPY_TARGET" \
  2>"$WORK_DIR/copied-hash.log" | awk '{print $1}')" || \
  fail copied_checksum_failed 'could not hash the copied input inside the disposable volume'
HASH_MAY_EXIST=0
[[ "$copied_sha256" == "$EXPECTED_SHA256" ]] || fail copied_checksum_failed 'the disposable input copy does not match --expected-sha256'

CHECK_MAY_EXIST=1
checker_log="$WORK_DIR/integrity-check.log"
checker_user="$host_uid:$host_gid"
checker_mount="$DATA_MOUNT_RO"
checker_target="$COPY_TARGET"
if [[ "$SOURCE_KIND" == 'legacy-aof' ]]; then
  checker_user="$COPY_OWNER"
  checker_mount="$DATA_MOUNT_RW"
  checker_target="$AOF_CHECK_COPY_TARGET"
fi
if ! docker run --rm \
  --name "$CHECK_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network none \
  --pull=never \
  --read-only \
  --user "$checker_user" \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 64 \
  --mount "$checker_mount" \
  --entrypoint "$CHECKER" \
  "$REDIS_IMAGE_ID" "$checker_target" \
  >"$checker_log" 2>&1; then
  fail integrity_check_failed 'the non-destructive Redis integrity checker rejected the copied input'
fi
CHECK_MAY_EXIST=0

if [[ "$SOURCE_KIND" == 'rdb' ]]; then
  required_databases="$(awk '
    /^\[offset [0-9]+\] Selecting DB ID [0-9]+$/ {
      value = $NF + 0
      if (!found || value > maximum) maximum = value
      found = 1
    }
    END {
      if (found) printf "%.0f\n", maximum + 1
      else print "1"
    }
  ' "$checker_log")" || fail probe_failed 'could not derive the RDB database range'
  [[ "$required_databases" =~ ^[0-9]+$ && "$required_databases" -gt 0 ]] || \
    fail probe_failed 'the RDB checker returned an invalid database range'
  if (( required_databases > REDIS_DATABASES )); then
    REDIS_DATABASES="$required_databases"
  fi
fi

if [[ "$SOURCE_KIND" == 'legacy-aof' ]]; then
  HASH_MAY_EXIST=1
  copied_sha256_after_checker="$(docker run --rm \
    --name "$HASH_CONTAINER" \
    --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
    --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
    --network none \
    --pull=never \
    --read-only \
    --user "$host_uid:$host_gid" \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    --pids-limit 32 \
    --mount "$DATA_MOUNT_RO" \
    --entrypoint sha256sum \
    "$REDIS_IMAGE_ID" "$COPY_TARGET" \
    2>"$WORK_DIR/copied-hash-after-checker.log" | awk '{print $1}')" || \
    fail copied_checksum_failed 'could not re-hash the legacy AOF restore input after checking'
  HASH_MAY_EXIST=0
  [[ "$copied_sha256_after_checker" == "$EXPECTED_SHA256" ]] || \
    fail copied_checksum_failed 'the legacy AOF checker changed the immutable restore input'
fi

redis_args=(
  --bind 127.0.0.1
  --protected-mode yes
  --port 6379
  --dir /data
  --daemonize no
  --save ''
  --loglevel notice
  --databases "$REDIS_DATABASES"
)
if [[ "$SOURCE_KIND" == 'rdb' ]]; then
  redis_args+=(--dbfilename dump.rdb --appendonly no)
else
  redis_args+=(
    --dbfilename disabled.rdb
    --appendonly yes
    --appendfilename appendonly.aof
    --appenddirname appendonlydir
    --appendfsync no
    --auto-aof-rewrite-percentage 0
  )
fi

REDIS_CREATED=1
if ! docker create \
  --name "$REDIS_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network "$NETWORK_NAME" \
  --pull=never \
  --read-only \
  --user "$host_uid:$host_gid" \
  --env HOME=/tmp \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 256 \
  --mount "$DATA_MOUNT_RW" \
  --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=64m,mode=0700' \
  --entrypoint redis-server \
  "$REDIS_IMAGE_ID" "${redis_args[@]}" \
  >"$WORK_DIR/redis-create.log" 2>&1; then
  fail redis_start_failed 'could not create the isolated Redis container'
fi
if ! docker start "$REDIS_CONTAINER" >"$WORK_DIR/redis-start.log" 2>&1; then
  fail redis_start_failed 'could not start the isolated Redis container'
fi

ready=0
attempt=0
while (( attempt < 60 )); do
  container_status="$(docker inspect --format '{{.State.Status}}' "$REDIS_CONTAINER" 2>/dev/null || true)"
  case "$container_status" in
    exited|dead|removing) break ;;
  esac
  if ping_output="$(docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
    redis-cli -h 127.0.0.1 -p 6379 --raw PING 2>/dev/null)" && [[ "$ping_output" == 'PONG' ]]; then
    ready=1
    break
  fi
  sleep 1
  attempt=$((attempt + 1))
done
[[ "$ready" -eq 1 ]] || fail redis_start_failed 'the isolated Redis instance did not become ready'

docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
  redis-cli -h 127.0.0.1 -p 6379 --raw INFO server \
  >"$WORK_DIR/info-server.txt" 2>"$WORK_DIR/info-server.log" || \
  fail probe_failed 'INFO server failed in the isolated Redis instance'
docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
  redis-cli -h 127.0.0.1 -p 6379 --raw INFO persistence \
  >"$WORK_DIR/info-persistence.txt" 2>"$WORK_DIR/info-persistence.log" || \
  fail probe_failed 'INFO persistence failed in the isolated Redis instance'
docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
  redis-cli -h 127.0.0.1 -p 6379 --raw INFO keyspace \
  >"$WORK_DIR/info-keyspace.txt" 2>"$WORK_DIR/info-keyspace.log" || \
  fail probe_failed 'INFO keyspace failed in the isolated Redis instance'
dbsize="$(docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
  redis-cli -h 127.0.0.1 -p 6379 --raw DBSIZE 2>"$WORK_DIR/dbsize.log")" || \
  fail probe_failed 'DBSIZE failed in the isolated Redis instance'
redis_cli_version="$(docker exec --user "$host_uid:$host_gid" "$REDIS_CONTAINER" \
  redis-cli --version 2>"$WORK_DIR/redis-cli-version.log")" || \
  fail probe_failed 'could not read the isolated redis-cli version'

[[ "$dbsize" =~ ^[0-9]+$ ]] || fail probe_failed 'DBSIZE returned an invalid value'
[[ "$redis_cli_version" =~ ^redis-cli[[:space:]][0-9]+\.[0-9]+ ]] || fail probe_failed 'redis-cli returned an invalid version'
redis_version="$(info_value "$WORK_DIR/info-server.txt" redis_version)"
loading="$(info_value "$WORK_DIR/info-persistence.txt" loading)"
rdb_last_save_time="$(info_value "$WORK_DIR/info-persistence.txt" rdb_last_save_time)"
rdb_last_bgsave_status="$(info_value "$WORK_DIR/info-persistence.txt" rdb_last_bgsave_status)"
aof_enabled="$(info_value "$WORK_DIR/info-persistence.txt" aof_enabled)"
aof_last_write_status="$(info_value "$WORK_DIR/info-persistence.txt" aof_last_write_status)"
aof_current_size="$(info_value "$WORK_DIR/info-persistence.txt" aof_current_size)"
aof_base_size="$(info_value "$WORK_DIR/info-persistence.txt" aof_base_size)"

[[ "$redis_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)? ]] || fail probe_failed 'INFO server returned an invalid Redis version'
redis_server_major="${redis_version%%.*}"
[[ "$redis_server_major" == "$EXPECTED_REDIS_MAJOR" ]] || \
  fail redis_major_mismatch 'the isolated Redis server major does not match --expected-redis-major'
[[ "$loading" == '0' ]] || fail probe_failed 'the isolated Redis instance is still loading'
[[ "$rdb_last_save_time" =~ ^[0-9]+$ ]] || fail probe_failed 'INFO persistence returned an invalid RDB save time'
[[ "$rdb_last_bgsave_status" == 'ok' ]] || fail probe_failed 'INFO persistence reported an RDB failure state'
[[ "$aof_enabled" == '0' || "$aof_enabled" == '1' ]] || fail probe_failed 'INFO persistence returned an invalid AOF state'
if [[ "$SOURCE_KIND" == 'rdb' && "$aof_enabled" != '0' ]]; then
  fail probe_failed 'the isolated RDB restore unexpectedly enabled AOF'
fi
if [[ "$SOURCE_KIND" == 'legacy-aof' ]]; then
  [[ "$aof_enabled" == '1' ]] || fail probe_failed 'the isolated legacy AOF restore did not enable AOF'
  [[ "$aof_last_write_status" == 'ok' ]] || fail probe_failed 'INFO persistence reported an AOF failure state'
  [[ "$aof_current_size" =~ ^[0-9]+$ && "$aof_base_size" =~ ^[0-9]+$ ]] || \
    fail probe_failed 'INFO persistence returned invalid AOF sizes'
fi
aof_current_size_json='null'
aof_base_size_json='null'
if [[ "$aof_current_size" =~ ^[0-9]+$ ]]; then
  aof_current_size_json="$aof_current_size"
elif [[ -n "$aof_current_size" ]]; then
  fail probe_failed 'INFO persistence returned an invalid current AOF size'
fi
if [[ "$aof_base_size" =~ ^[0-9]+$ ]]; then
  aof_base_size_json="$aof_base_size"
elif [[ -n "$aof_base_size" ]]; then
  fail probe_failed 'INFO persistence returned an invalid base AOF size'
fi
rdb_last_save_utc="$(epoch_to_utc "$rdb_last_save_time")" || fail probe_failed 'could not format the isolated RDB save time'

: > "$WORK_DIR/keyspace.ndjson"
while IFS= read -r keyspace_line || [[ -n "$keyspace_line" ]]; do
  keyspace_line="${keyspace_line%$'\r'}"
  case "$keyspace_line" in
    ''|'# '*) continue ;;
  esac
  if [[ "$keyspace_line" =~ ^(db[0-9]+):keys=([0-9]+),expires=([0-9]+),avg_ttl=([0-9]+)(,subexpiry=([0-9]+))?$ ]]; then
    keyspace_db="${BASH_REMATCH[1]}"
    keyspace_keys="${BASH_REMATCH[2]}"
    keyspace_expires="${BASH_REMATCH[3]}"
    keyspace_avg_ttl="${BASH_REMATCH[4]}"
    keyspace_subexpiry="${BASH_REMATCH[6]:-0}"
    jq -n \
      --arg database "$keyspace_db" \
      --argjson keys "$keyspace_keys" \
      --argjson expires "$keyspace_expires" \
      --argjson avg_ttl_ms "$keyspace_avg_ttl" \
      --argjson subexpiry "$keyspace_subexpiry" \
      '{database: $database, keys: $keys, expires: $expires, avg_ttl_ms: $avg_ttl_ms, subexpiry: $subexpiry}' \
      >> "$WORK_DIR/keyspace.ndjson" || fail report_build_failed 'could not normalize INFO keyspace'
  else
    fail probe_failed 'INFO keyspace returned an unexpected non-summary line'
  fi
done < "$WORK_DIR/info-keyspace.txt"
jq -s '.' "$WORK_DIR/keyspace.ndjson" > "$WORK_DIR/keyspace.json" || fail report_build_failed 'could not build the keyspace summary'

HASH_MAY_EXIST=1
copied_sha256_after="$(docker run --rm \
  --name "$HASH_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-redis-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network none \
  --pull=never \
  --read-only \
  --user "$host_uid:$host_gid" \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 32 \
  --mount "$DATA_MOUNT_RO" \
  --entrypoint sha256sum \
  "$REDIS_IMAGE_ID" "$COPY_TARGET" \
  2>"$WORK_DIR/copied-hash-after.log" | awk '{print $1}')" || \
  fail copied_checksum_failed 'could not re-hash the restored input copy'
HASH_MAY_EXIST=0
[[ "$copied_sha256_after" == "$EXPECTED_SHA256" ]] || fail copied_checksum_failed 'the isolated restore changed the copied input'

open_input_read || fail input_changed 'the local input identity changed during verification'
final_input_sha256="$(sha256_file "$INPUT_FD_PATH" 2>"$WORK_DIR/final-input-hash.log")" || \
  fail input_changed 'could not re-hash the local input after verification'
open_input_read || fail input_changed 'the local input identity changed during verification'
final_input_size="$(wc -c < "$INPUT_FD_PATH" | tr -d '[:space:]')"
if [[ "$final_input_sha256" != "$INPUT_SHA256" || "$final_input_size" != "$INPUT_SIZE" ]]; then
  fail input_changed 'the local input changed during verification'
fi
final_verified_sha256="$(sha256_file "$VERIFIED_INPUT_PATH")" || \
  fail input_changed 'could not re-hash the stable input copy after verification'
final_verified_size="$(wc -c < "$VERIFIED_INPUT_PATH" | tr -d '[:space:]')"
if [[ "$final_verified_sha256" != "$VERIFIED_INPUT_SHA256" || "$final_verified_size" != "$INPUT_SIZE" ]]; then
  fail input_changed 'the stable input copy changed during verification'
fi

if ! cleanup_docker; then
  fail cleanup_failed 'one or more exact-name Docker resources could not be removed before reporting success'
fi

FINISHED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
FINISHED_EPOCH="$(date -u '+%s')"
rpo_age_seconds=$((FINISHED_EPOCH - RPO_EPOCH))
report_parent_is_bound || fail report_write_failed 'the physical report parent directory changed before report creation'
[[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]] || \
  fail report_write_failed 'the no-clobber report destination appeared during verification'
REPORT_TEMP="$(mktemp './.modelport-isolated-redis-report.XXXXXX')" || fail report_write_failed 'could not create the report staging file'
chmod 0600 "$REPORT_TEMP" || fail report_write_failed 'could not secure the report staging file'

jq -n \
  --argjson schema_version "$REPORT_SCHEMA_VERSION" \
  --arg started_at "$STARTED_AT" \
  --arg finished_at "$FINISHED_AT" \
  --arg source_kind "$SOURCE_KIND" \
  --argjson complete_legacy_aof_confirmed "$LEGACY_AOF_CONFIRMED" \
  --arg input_sha256 "$INPUT_SHA256" \
  --arg verified_input_sha256 "$VERIFIED_INPUT_SHA256" \
  --arg expected_sha256 "$EXPECTED_SHA256" \
  --arg copied_sha256 "$copied_sha256" \
  --argjson input_size "$INPUT_SIZE" \
  --arg input_mtime_utc "$INPUT_MTIME_UTC" \
  --arg rpo_at "$RPO_AT" \
  --argjson rpo_age_seconds "$rpo_age_seconds" \
  --arg redis_image "$REDIS_IMAGE" \
  --arg redis_image_id "$REDIS_IMAGE_ID" \
  --arg redis_version "$redis_version" \
  --arg redis_server_major "$redis_server_major" \
  --arg expected_redis_major "$EXPECTED_REDIS_MAJOR" \
  --arg redis_cli_version "$redis_cli_version" \
  --arg checker "$CHECKER" \
  --argjson configured_databases "$REDIS_DATABASES" \
  --argjson dbsize "$dbsize" \
  --argjson loading "$loading" \
  --argjson rdb_last_save_time "$rdb_last_save_time" \
  --arg rdb_last_save_utc "$rdb_last_save_utc" \
  --arg rdb_last_bgsave_status "$rdb_last_bgsave_status" \
  --argjson aof_enabled "$aof_enabled" \
  --arg aof_last_write_status "$aof_last_write_status" \
  --argjson aof_current_size "$aof_current_size_json" \
  --argjson aof_base_size "$aof_base_size_json" \
  --slurpfile keyspace "$WORK_DIR/keyspace.json" \
  '{
    schema_version: $schema_version,
    result: "passed",
    started_at: $started_at,
    finished_at: $finished_at,
    input: {
      kind: $source_kind,
      sha256: $input_sha256,
      verified_copy_sha256: $verified_input_sha256,
      expected_sha256: $expected_sha256,
      disposable_copy_sha256: $copied_sha256,
      size_bytes: $input_size,
      source_mtime_utc: $input_mtime_utc,
      path_recorded: false,
      complete_legacy_aof_confirmed: ($complete_legacy_aof_confirmed == 1)
    },
    rpo: {
      boundary_utc: $rpo_at,
      basis: "operator_supplied_persistence_boundary",
      age_seconds_at_verification: $rpo_age_seconds
    },
    runtime: {
      redis_image: {reference: $redis_image, id: $redis_image_id},
      redis_server_version: $redis_version,
      redis_server_major: ($redis_server_major | tonumber),
      expected_redis_major: ($expected_redis_major | tonumber),
      redis_cli_version: $redis_cli_version
    },
    verification: {
      integrity_checker: $checker,
      checker_non_destructive: true,
      stable_verified_input_copy_used: true,
      checker_writable_temporary_copy: ($source_kind == "legacy-aof"),
      restore_input_read_only_during_check: true,
      ping: "PONG",
      configured_databases: $configured_databases,
      dbsize_default_database: $dbsize,
      keyspace: $keyspace[0],
      persistence: {
        loading: $loading,
        rdb_last_save_time: $rdb_last_save_time,
        rdb_last_save_utc: $rdb_last_save_utc,
        rdb_last_bgsave_status: $rdb_last_bgsave_status,
        aof_enabled: ($aof_enabled == 1),
        aof_last_write_status: (if $aof_last_write_status == "" then null else $aof_last_write_status end),
        aof_current_size: $aof_current_size,
        aof_base_size: $aof_base_size
      }
    },
    isolation: {
      preloaded_digest_pinned_image: true,
      docker_internal_network: true,
      published_ports: false,
      read_only_rootfs: true,
      capabilities_dropped: true,
      no_new_privileges: true,
      disposable_random_volume: true,
      resources_removed_before_success_report: true,
      sensitive_storage_confirmed: true
    },
    content_disclosure: {
      key_names_included: false,
      values_included: false
    }
  }' > "$REPORT_TEMP" || fail report_write_failed 'could not build the structured report'

jq -e '
  .schema_version == 1 and .result == "passed" and
  .input.sha256 == .input.expected_sha256 and
  .input.sha256 == .input.verified_copy_sha256 and
  .input.sha256 == .input.disposable_copy_sha256 and
  ((.input.kind == "rdb" and .input.complete_legacy_aof_confirmed == false) or
   (.input.kind == "legacy-aof" and .input.complete_legacy_aof_confirmed == true)) and
  .verification.ping == "PONG" and
  .verification.configured_databases >= 16 and
  .isolation.resources_removed_before_success_report == true and
  .content_disclosure.key_names_included == false and
  .content_disclosure.values_included == false
' "$REPORT_TEMP" >/dev/null || fail report_write_failed 'the structured report failed its schema invariants'

if ! cleanup_work_dir; then
  fail cleanup_failed 'could not remove the local verification work directory before publishing success'
fi
report_parent_is_bound || fail report_write_failed 'the physical report parent directory changed before publication'

# A hard link atomically publishes without replacing an existing report.
report_temp_identity="$(file_identity "$REPORT_TEMP")" || \
  fail report_write_failed 'could not identify the report staging file'
if ! ln "$REPORT_TEMP" "./$REPORT_BASENAME"; then
  fail report_write_failed 'could not atomically publish the no-clobber report'
fi
REPORT_PUBLISHED=1
REPORT_PUBLISHED_IDENTITY="$(file_identity "./$REPORT_BASENAME")" || \
  fail report_write_failed 'could not identify the published report'
[[ "$REPORT_PUBLISHED_IDENTITY" == "$report_temp_identity" ]] || \
  fail report_write_failed 'the published report identity did not match the staging file'
report_parent_is_bound || fail report_write_failed 'the physical report parent directory changed during publication'
if ! rm -f -- "$REPORT_TEMP"; then
  fail cleanup_failed 'could not remove the report staging link after publication'
fi
REPORT_TEMP=''

printf 'ModelPort isolated Redis restore passed; report: %s\n' "$REPORT_PATH"
