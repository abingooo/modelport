#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

readonly DEFAULT_POSTGRES_IMAGE='postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
readonly REPORT_SCHEMA_VERSION=1
readonly RESTORE_RUN_LABEL='com.abingooo.modelport.restore-run'
SCRIPT_DIR="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -P -- "$SCRIPT_DIR/.." && pwd)"

SOURCE_KIND=''
DUMP_PATH=''
DUMP_FD_PATH=''
DUMP_IDENTITY=''
VERIFIED_DUMP_PATH=''
VERIFIED_DUMP_SHA256=''
REPORT_PATH=''
EXPECTED_SHA256=''
EXPECTED_REVISION=''
EXPECTED_POSTGRES_MAJOR=''
POSTGRES_IMAGE="$DEFAULT_POSTGRES_IMAGE"
PRODUCTION_STORAGE_CONFIRMED=0
INPUT_SHA256=''
INPUT_SIZE=''
INPUT_FORMAT='none'
RESTORE_METHOD='none'
DETECTED_LINEAGE='empty'
STARTED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
FAILURE_CODE='internal_error'
REPORT_READY=0
REPORT_WRITTEN=0
REPORT_RESULT=''
REPORT_WRITTEN_IDENTITY=''
WORK_DIR=''
REPORT_TEMP=''
REPORT_BASENAME=''
REPORT_PARENT_PHYSICAL=''
REPORT_PARENT_ID=''
NETWORK_NAME=''
PG_CONTAINER=''
PG_VOLUME=''
SPACE_CONTAINER=''
NETWORK_CREATED=0
PG_CREATED=0
PG_VOLUME_CREATED=0
SPACE_CREATED=0
MIGRATION_RUNS=0
MIGRATION_BINARY_SHA256=''
SOURCE_ROOT="$REPO_ROOT"
MIGRATION_SOURCE='working-tree'
RESTORE_RUN_ID=''

usage() {
  cat >&2 <<'EOF'
Usage:
  deploy/modelport-isolated-restore.sh \
    --source-kind empty|upstream|legacy-modelport|production \
    --report /absolute/path/report.json \
    [--dump /absolute/path/backup.sql[.gz]] \
    [--expected-sha256 64-lowercase-hex] \
    [--expected-revision 40-lowercase-hex] \
    [--expected-postgres-major positive-integer] \
    [--postgres-image repository@sha256:64-lowercase-hex] \
    [--confirm-production-storage]

The empty source kind forbids --dump. Every other source kind requires it.
Database hosts, DSNs, credentials, image references, and resource names are
intentionally not configurable.

--postgres-image may select a different locally available PostgreSQL client
and server major, but the reference must be digest-pinned. No image is pulled.

--confirm-production-storage is mandatory for production snapshots. It is the
operator's confirmation that the isolated Docker host uses encrypted storage
and has enough capacity for the fully restored database.
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
  printf 'ModelPort isolated restore failed [%s]: %s\n' "$1" "$2" >&2
}

fail() {
  FAILURE_CODE="$1"
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

sha256_file() {
  if command_exists sha256sum; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

sha256_text() {
  if command_exists sha256sum; then
    printf '%s' "$1" | sha256sum | awk '{print $1}'
  else
    printf '%s' "$1" | shasum -a 256 | awk '{print $1}'
  fi
}

file_identity() {
  local path="$1"
  if stat -f '%d:%i' "$path" >/dev/null 2>&1; then
    stat -f '%d:%i' "$path"
  else
    stat -Lc '%d:%i' "$path"
  fi
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

open_dump_read() {
  local current_identity
  exec 8<"$DUMP_PATH" || return 1
  current_identity="$(file_identity /dev/fd/8)" || return 1
  [[ "$current_identity" == "$DUMP_IDENTITY" ]] || return 1
  DUMP_FD_PATH='/dev/fd/8'
}

build_required_bridge_contract() {
  local lottery_migration="$SOURCE_ROOT/backend/migrations/233_modelport_lottery_bridge.sql"
  local audit_migration="$SOURCE_ROOT/backend/migrations/234_modelport_instruction_audit_bridge.sql"
  local platform_migration="$SOURCE_ROOT/backend/migrations/236_modelport_legacy_platform_constraints.sql"

  [[ -f "$lottery_migration" && -f "$audit_migration" && -f "$platform_migration" ]] || return 1

  sed -nE \
    's/^[[:space:]]*CREATE TABLE IF NOT EXISTS ([A-Za-z0-9_]+).*/\1/p' \
    "$lottery_migration" "$audit_migration" |
    LC_ALL=C sort -u |
    jq -Rsc 'split("\n") | map(select(length > 0))' \
      > "$WORK_DIR/required-bridge-tables.json" || return 1

  sed -nE \
    's/.*CONSTRAINT[[:space:]]+(IF[[:space:]]+EXISTS[[:space:]]+)?([A-Za-z0-9_]+).*/\2/p' \
    "$lottery_migration" "$audit_migration" "$platform_migration" |
    LC_ALL=C sort -u |
    jq -Rsc 'split("\n") | map(select(length > 0))' \
      > "$WORK_DIR/required-bridge-constraints.json" || return 1

  sed -nE \
    's/^[[:space:]]*CREATE (UNIQUE )?INDEX IF NOT EXISTS ([A-Za-z0-9_]+).*/\2/p' \
    "$lottery_migration" "$audit_migration" |
    LC_ALL=C sort -u |
    jq -Rsc 'split("\n") | map(select(length > 0))' \
      > "$WORK_DIR/required-bridge-indexes.json" || return 1

  jq -e 'length == 44 and all(.[]; test("^[a-z0-9_]+$"))' \
    "$WORK_DIR/required-bridge-tables.json" >/dev/null || return 1
  jq -e 'length == 182 and all(.[]; test("^[a-z0-9_]+$"))' \
    "$WORK_DIR/required-bridge-constraints.json" >/dev/null || return 1
  jq -e 'length == 76 and all(.[]; test("^[a-z0-9_]+$"))' \
    "$WORK_DIR/required-bridge-indexes.json" >/dev/null || return 1
}

build_legacy_manifest() {
  local manifest="$SOURCE_ROOT/backend/migrations/modelport_legacy/v0.1.176.2/manifest.tsv"
  [[ -f "$manifest" ]] || return 1
  jq -Rn '
    [inputs |
      select(length > 0 and (startswith("#") | not)) |
      split("\t") |
      select(length == 3) |
      {filename: .[0], raw_sha256: .[1], checksum: .[2]}]
  ' < "$manifest" > "$WORK_DIR/legacy-manifest.json" || return 1
  jq -e '
    length == 38 and
    all(.[];
      (.filename | test("^[0-9]{3}_[a-z0-9_]+\\.sql$")) and
      (.raw_sha256 | test("^[0-9a-f]{64}$")) and
      (.checksum | test("^[0-9a-f]{64}$")))
  ' "$WORK_DIR/legacy-manifest.json" >/dev/null
}

validate_legacy_archive_and_ledger() {
  if ! (
    cd "$SOURCE_ROOT/backend"
    go test ./migrations \
      -run '^TestLegacyModelPortMigrationArchiveMatchesManifest$' \
      -count=1
  ) >"$WORK_DIR/legacy-archive-test.log" 2>&1; then
    return 1
  fi
  jq -e -n \
    --slurpfile manifest "$WORK_DIR/legacy-manifest.json" \
    --slurpfile snapshot "$WORK_DIR/before.json" '
      ($snapshot[0].schema_migrations |
        map({key: .filename, value: .checksum}) | from_entries) as $ledger |
      $manifest[0] | all(.[]; $ledger[.filename] == .checksum)
    ' >/dev/null
}

json_file_or_empty() {
  local path="$1"
  if [[ -s "$path" ]] && jq -e 'type == "object"' "$path" >/dev/null 2>&1; then
    printf '%s' "$path"
  else
    printf '%s' "$WORK_DIR/empty.json"
  fi
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
  docker rm --force --volumes "$name" >/dev/null 2>&1 || docker_resource_is_absent container "$name"
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
  if [[ "$PG_CREATED" -eq 1 ]]; then
    if remove_container_exact "$PG_CONTAINER"; then
      PG_CREATED=0
    else
      failed=1
    fi
  fi
  if [[ "$SPACE_CREATED" -eq 1 ]]; then
    if remove_container_exact "$SPACE_CONTAINER"; then
      SPACE_CREATED=0
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
  if [[ "$PG_VOLUME_CREATED" -eq 1 ]]; then
    if remove_volume_exact "$PG_VOLUME"; then
      PG_VOLUME_CREATED=0
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

stage_report() {
  local result="$1"
  local failure_code_json='null'
  local before_file after_first_file after_file runtime_file invariants_file finished_at

  [[ "$REPORT_READY" -eq 1 ]] || return 1
  report_parent_is_bound || return 1
  [[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]] || return 1
  if [[ -n "$REPORT_TEMP" ]]; then
    rm -f -- "$REPORT_TEMP" || return 1
    REPORT_TEMP=''
  fi
  before_file="$(json_file_or_empty "$WORK_DIR/before.json")"
  after_first_file="$(json_file_or_empty "$WORK_DIR/after-first.json")"
  after_file="$(json_file_or_empty "$WORK_DIR/after.json")"
  runtime_file="$(json_file_or_empty "$WORK_DIR/runtime.json")"
  invariants_file="$WORK_DIR/invariants.json"
  if [[ ! -s "$invariants_file" ]] || ! jq -e 'type == "array"' "$invariants_file" >/dev/null 2>&1; then
    invariants_file="$WORK_DIR/empty-array.json"
  fi
  if [[ "$result" != 'passed' ]]; then
    failure_code_json="$(jq -Rn --arg value "$FAILURE_CODE" '$value')"
  fi
  finished_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  REPORT_TEMP="$(mktemp './.modelport-isolated-restore-report.XXXXXX')" || return 1
  chmod 0600 "$REPORT_TEMP" || return 1

  jq -n \
    --argjson schema_version "$REPORT_SCHEMA_VERSION" \
    --arg result "$result" \
    --arg source_kind "$SOURCE_KIND" \
    --arg detected_lineage "$DETECTED_LINEAGE" \
    --arg started_at "$STARTED_AT" \
    --arg finished_at "$finished_at" \
    --arg input_sha256 "$INPUT_SHA256" \
    --arg verified_dump_sha256 "$VERIFIED_DUMP_SHA256" \
    --arg expected_sha256 "$EXPECTED_SHA256" \
    --arg input_size "$INPUT_SIZE" \
    --arg input_format "$INPUT_FORMAT" \
    --arg restore_method "$RESTORE_METHOD" \
    --argjson failure_code "$failure_code_json" \
    --slurpfile runtime "$runtime_file" \
    --slurpfile before "$before_file" \
    --slurpfile after_first "$after_first_file" \
    --slurpfile after "$after_file" \
    --slurpfile invariants "$invariants_file" \
    '
      def public_snapshot:
        if type == "object" then
          . + {settings_key_count: (.settings_value_summaries // {} | length)} |
          del(.settings_value_summaries)
        else . end;
      {
        schema_version: $schema_version,
        result: $result,
        failure_code: $failure_code,
        source_kind: $source_kind,
        detected_lineage: $detected_lineage,
        started_at: $started_at,
        finished_at: $finished_at,
        input: {
          sha256: (if $input_sha256 == "" then null else $input_sha256 end),
          verified_copy_sha256: (if $verified_dump_sha256 == "" then null else $verified_dump_sha256 end),
          expected_sha256: (if $expected_sha256 == "" then null else $expected_sha256 end),
          size_bytes: (if $input_size == "" then null else ($input_size | tonumber) end),
          format: $input_format
        },
        runtime: $runtime[0],
        restore: {
          method: $restore_method,
          verified_copy_used: ($restore_method != "none")
        },
        before: ($before[0] | public_snapshot),
        after_first_migration: ($after_first[0] | public_snapshot),
        after: ($after[0] | public_snapshot),
        invariants: $invariants[0]
      }
    ' > "$REPORT_TEMP" || return 1

  REPORT_RESULT="$result"
}

publish_staged_report() {
  local staged_identity=''
  local published_identity=''
  [[ -n "$REPORT_TEMP" && -f "$REPORT_TEMP" && ! -L "$REPORT_TEMP" ]] || return 1
  report_parent_is_bound || return 1
  [[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]] || return 1
  staged_identity="$(file_identity "$REPORT_TEMP")" || return 1

  # A hard link provides an atomic no-clobber publish on the bound filesystem.
  ln "$REPORT_TEMP" "./$REPORT_BASENAME" || return 1
  REPORT_WRITTEN=1
  published_identity="$(file_identity "./$REPORT_BASENAME")" || return 1
  [[ "$published_identity" == "$staged_identity" ]] || return 1
  REPORT_WRITTEN_IDENTITY="$published_identity"
  if ! report_parent_is_bound; then
    remove_written_report || true
    return 1
  fi
  if ! rm -f -- "$REPORT_TEMP"; then
    remove_written_report || true
    return 1
  fi
  REPORT_TEMP=''
}

remove_written_report() {
  local current_identity=''
  [[ "$REPORT_WRITTEN" -eq 1 ]] || return 0
  if [[ ! -e "./$REPORT_BASENAME" && ! -L "./$REPORT_BASENAME" ]]; then
    REPORT_WRITTEN=0
    REPORT_WRITTEN_IDENTITY=''
    return 0
  fi
  [[ -n "$REPORT_WRITTEN_IDENTITY" && ! -L "./$REPORT_BASENAME" ]] || return 1
  current_identity="$(file_identity "./$REPORT_BASENAME")" || return 1
  [[ "$current_identity" == "$REPORT_WRITTEN_IDENTITY" ]] || return 1
  rm -f -- "./$REPORT_BASENAME" || return 1
  REPORT_WRITTEN=0
  REPORT_WRITTEN_IDENTITY=''
}

write_report() {
  local result="$1"
  stage_report "$result" && publish_staged_report
}

on_exit() {
  local status=$?
  trap - EXIT
  set +e
  if ! cleanup_docker; then
    FAILURE_CODE='cleanup_failed'
    say_error cleanup_failed 'one or more exact-name Docker resources could not be removed'
    status=1
  fi
  if [[ "$status" -ne 0 && "$REPORT_READY" -eq 1 && "$REPORT_WRITTEN" -eq 0 ]]; then
    if ! write_report failed; then
      say_error report_write_failed 'could not publish the structured failure report'
    fi
  fi
  if ! cleanup_local; then
    FAILURE_CODE='cleanup_failed'
    say_error cleanup_failed 'one or more local staging artifacts could not be removed'
    status=1
  fi
  if [[ "$status" -ne 0 && "$REPORT_WRITTEN" -eq 1 && "$REPORT_RESULT" == passed ]]; then
    if ! remove_written_report; then
      say_error cleanup_failed 'the passed report could not be removed after cleanup failure'
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
    --dump)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      DUMP_PATH="$2"
      shift 2
      ;;
    --report)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      REPORT_PATH="$2"
      shift 2
      ;;
    --expected-sha256)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      EXPECTED_SHA256="$2"
      shift 2
      ;;
    --expected-revision)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      EXPECTED_REVISION="$2"
      shift 2
      ;;
    --expected-postgres-major)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      EXPECTED_POSTGRES_MAJOR="$2"
      shift 2
      ;;
    --postgres-image)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      POSTGRES_IMAGE="$2"
      shift 2
      ;;
    --confirm-production-storage)
      PRODUCTION_STORAGE_CONFIRMED=1
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
  empty|upstream|legacy-modelport|production) ;;
  '') usage; printf '%s\n' '--source-kind is required' >&2; exit 2 ;;
  *) usage; printf 'Invalid source kind: %s\n' "$SOURCE_KIND" >&2; exit 2 ;;
esac

if [[ -z "$REPORT_PATH" ]]; then
  usage
  printf '%s\n' '--report is required' >&2
  exit 2
fi

if [[ "$SOURCE_KIND" == 'production' && "$PRODUCTION_STORAGE_CONFIRMED" -ne 1 ]]; then
  printf '%s\n' '--confirm-production-storage is required for production snapshots' >&2
  exit 2
fi
if [[ ! "$POSTGRES_IMAGE" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[0-9a-f]{64}$ ]]; then
  printf '%s\n' '--postgres-image must be a digest-pinned local image reference' >&2
  exit 2
fi
if [[ -n "$EXPECTED_REVISION" && ! "$EXPECTED_REVISION" =~ ^[0-9a-f]{40}$ ]]; then
  printf '%s\n' '--expected-revision must contain exactly 40 lowercase hexadecimal characters' >&2
  exit 2
fi
if [[ -n "$EXPECTED_POSTGRES_MAJOR" && \
      ( ! "$EXPECTED_POSTGRES_MAJOR" =~ ^[0-9]+$ || "$EXPECTED_POSTGRES_MAJOR" -eq 0 ) ]]; then
  printf '%s\n' '--expected-postgres-major must be a positive integer' >&2
  exit 2
fi
if [[ "$SOURCE_KIND" == 'production' && -z "$EXPECTED_REVISION" ]]; then
  printf '%s\n' '--expected-revision is required for production snapshots' >&2
  exit 2
fi
if [[ "$SOURCE_KIND" == 'production' && -z "$EXPECTED_POSTGRES_MAJOR" ]]; then
  printf '%s\n' '--expected-postgres-major is required for production snapshots' >&2
  exit 2
fi
if [[ "$REPORT_PATH" != /* ]]; then
  printf '%s\n' '--report must be an absolute path' >&2
  exit 2
fi
if [[ -e "$REPORT_PATH" || -L "$REPORT_PATH" ]]; then
  printf '%s\n' '--report must not already exist or be a symbolic link' >&2
  exit 2
fi
if [[ ! -d "$(dirname -- "$REPORT_PATH")" ]]; then
  printf '%s\n' '--report parent directory does not exist' >&2
  exit 2
fi

if [[ "$SOURCE_KIND" == 'empty' ]]; then
  if [[ -n "$DUMP_PATH" ]]; then
    printf '%s\n' '--dump is forbidden for the empty source kind' >&2
    exit 2
  fi
  if [[ -n "$EXPECTED_SHA256" ]]; then
    printf '%s\n' '--expected-sha256 is forbidden for the empty source kind' >&2
    exit 2
  fi
else
  if [[ -z "$DUMP_PATH" ]]; then
    printf '%s\n' '--dump is required for non-empty source kinds' >&2
    exit 2
  fi
  if [[ "$DUMP_PATH" != /* ]]; then
    printf '%s\n' '--dump must be an absolute path' >&2
    exit 2
  fi
  if [[ ! -f "$DUMP_PATH" || -L "$DUMP_PATH" ]]; then
    printf '%s\n' '--dump must be a regular, non-symbolic-link file' >&2
    exit 2
  fi
  if [[ ! -r "$DUMP_PATH" || ! -s "$DUMP_PATH" ]]; then
    printf '%s\n' '--dump must be readable and non-empty' >&2
    exit 2
  fi
  if [[ -n "$EXPECTED_SHA256" && ! "$EXPECTED_SHA256" =~ ^[0-9a-f]{64}$ ]]; then
    printf '%s\n' '--expected-sha256 must contain exactly 64 lowercase hexadecimal characters' >&2
    exit 2
  fi
  if [[ "$SOURCE_KIND" == 'production' && -z "$EXPECTED_SHA256" ]]; then
    printf '%s\n' '--expected-sha256 is required for production snapshots' >&2
    exit 2
  fi
fi

case "$DUMP_PATH$REPORT_PATH" in
  *$'\n'*|*$'\r'*)
    printf '%s\n' 'dump and report paths must not contain control characters' >&2
    exit 2
    ;;
esac

if [[ "$SOURCE_KIND" != 'empty' ]]; then
  dump_physical_parent="$(cd -P -- "$(dirname -- "$DUMP_PATH")" && pwd)" || {
    printf '%s\n' 'could not resolve the physical dump parent directory' >&2
    exit 2
  }
  DUMP_PATH="$dump_physical_parent/${DUMP_PATH##*/}"
fi
report_physical_parent="$(cd -P -- "$(dirname -- "$REPORT_PATH")" && pwd)" || {
  printf '%s\n' 'could not resolve the physical report parent directory' >&2
  exit 2
}
REPORT_BASENAME="${REPORT_PATH##*/}"
REPORT_PARENT_PHYSICAL="$report_physical_parent"
REPORT_PATH="$REPORT_PARENT_PHYSICAL/$REPORT_BASENAME"

if [[ "$SOURCE_KIND" == 'production' ]]; then
  if path_is_within "$DUMP_PATH" "$REPO_ROOT" || \
     path_is_within "$REPORT_PATH" "$REPO_ROOT"; then
    printf '%s\n' 'production dump and report paths must be outside the repository tree' >&2
    exit 2
  fi
fi

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
if [[ -e "./$REPORT_BASENAME" || -L "./$REPORT_BASENAME" ]]; then
  printf '%s\n' '--report must not already exist or be a symbolic link' >&2
  exit 2
fi

for dependency in docker jq go git openssl gzip awk od wc tr sed sort stat head date dirname mktemp chmod ln rm mv cp tar sleep env; do
  command_exists "$dependency" || fail dependency_missing "required command is unavailable: $dependency"
done
if ! command_exists sha256sum && ! command_exists shasum; then
  fail dependency_missing 'sha256sum or shasum is required'
fi

WORK_DIR="$(mktemp -d './.modelport-isolated-restore-work.XXXXXX')"
chmod 0700 "$WORK_DIR"
WORK_DIR="$REPORT_PARENT_PHYSICAL/${WORK_DIR#./}"
printf '{}\n' > "$WORK_DIR/empty.json"
printf '[]\n' > "$WORK_DIR/empty-array.json"
printf '{}\n' > "$WORK_DIR/before.json"
printf '{}\n' > "$WORK_DIR/after-first.json"
printf '{}\n' > "$WORK_DIR/after.json"
printf '{}\n' > "$WORK_DIR/runtime.json"
printf '[]\n' > "$WORK_DIR/invariants.json"
REPORT_READY=1

if [[ "$SOURCE_KIND" != 'empty' ]]; then
  if ! exec 9<"$DUMP_PATH"; then
    fail invalid_input 'could not open the input backup'
  fi
  if [[ ! -f /dev/fd/9 || ! -r /dev/fd/9 || ! -s /dev/fd/9 ]]; then
    fail invalid_input 'the opened input backup is not a readable, non-empty regular file'
  fi
  DUMP_IDENTITY="$(file_identity /dev/fd/9)" || fail invalid_input 'could not identify the opened input backup'
  [[ -n "$DUMP_IDENTITY" ]] || fail invalid_input 'the opened input backup identity was empty'
fi

repository_commit_start="$(git -C "$REPO_ROOT" rev-parse HEAD 2>"$WORK_DIR/git-start.log")" || \
  fail repository_mismatch 'could not resolve the repository HEAD'
repository_tree_start="$(git -C "$REPO_ROOT" rev-parse 'HEAD^{tree}' 2>>"$WORK_DIR/git-start.log")" || \
  fail repository_mismatch 'could not resolve the repository tree'
[[ "$repository_commit_start" =~ ^[0-9a-f]{40}$ && "$repository_tree_start" =~ ^[0-9a-f]{40}$ ]] || \
  fail repository_mismatch 'the repository revision metadata was invalid'
repository_dirty_start=false
if [[ -n "$(git -C "$REPO_ROOT" status --porcelain=v1 --untracked-files=normal 2>>"$WORK_DIR/git-start.log")" ]]; then
  repository_dirty_start=true
fi
if [[ -n "$EXPECTED_REVISION" && "$repository_commit_start" != "$EXPECTED_REVISION" ]]; then
  fail repository_mismatch 'the repository HEAD does not match --expected-revision'
fi
if [[ "$SOURCE_KIND" == 'production' && "$repository_dirty_start" == true ]]; then
  fail repository_mismatch 'production restore evidence requires a clean repository checkout'
fi
jq -n \
  --arg commit "$repository_commit_start" \
  --arg tree_sha "$repository_tree_start" \
  --argjson dirty "$repository_dirty_start" \
  '{repository: {commit: $commit, tree_sha: $tree_sha, dirty_at_start: $dirty}}' \
  > "$WORK_DIR/runtime.json" || fail metadata_failed 'could not record repository evidence'

if [[ "$SOURCE_KIND" == 'production' ]]; then
  SOURCE_ROOT="$WORK_DIR/source-snapshot"
  mkdir -m 0700 "$SOURCE_ROOT" || fail repository_mismatch 'could not create the candidate source snapshot directory'
  if ! git -C "$REPO_ROOT" archive --format=tar "$EXPECTED_REVISION" 2>"$WORK_DIR/git-archive.log" | \
      tar -xf - -C "$SOURCE_ROOT" 2>"$WORK_DIR/git-archive-extract.log"; then
    fail repository_mismatch 'could not materialize the exact expected revision source snapshot'
  fi
  [[ -d "$SOURCE_ROOT/backend/cmd/modelport-restore-migrate" && \
     -d "$SOURCE_ROOT/backend/migrations" ]] || \
    fail repository_mismatch 'the expected revision source snapshot lacks the restore migration sources'
  MIGRATION_SOURCE='git-archive'
fi

build_required_bridge_contract || \
  fail repository_mismatch 'could not derive the required bridge object contract from migrations 233, 234, and 236'
build_legacy_manifest || \
  fail repository_mismatch 'the archived legacy ModelPort migration manifest is missing or invalid'

if [[ "$SOURCE_KIND" != 'empty' ]]; then
  open_dump_read || fail input_changed 'the input backup identity changed before hashing'
  source_sha256_before="$(sha256_file "$DUMP_FD_PATH")" || fail invalid_input 'could not hash the input backup'
  [[ "$source_sha256_before" =~ ^[0-9a-f]{64}$ ]] || fail invalid_input 'the input SHA-256 was invalid'
  open_dump_read || fail input_changed 'the input backup identity changed before sizing'
  source_size_before="$(wc -c < "$DUMP_FD_PATH" | tr -d '[:space:]')"
  [[ "$source_size_before" =~ ^[0-9]+$ ]] || fail invalid_input 'the input size was invalid'

  VERIFIED_DUMP_PATH="$WORK_DIR/verified-input.backup"
  open_dump_read || fail input_changed 'the input backup identity changed before creating the stable copy'
  cp -- "$DUMP_FD_PATH" "$VERIFIED_DUMP_PATH" || fail invalid_input 'could not create the stable input copy'
  chmod 0400 "$VERIFIED_DUMP_PATH" || fail invalid_input 'could not protect the stable input copy'
  VERIFIED_DUMP_SHA256="$(sha256_file "$VERIFIED_DUMP_PATH")" || fail invalid_input 'could not hash the stable input copy'
  INPUT_SIZE="$(wc -c < "$VERIFIED_DUMP_PATH" | tr -d '[:space:]')"
  [[ "$VERIFIED_DUMP_SHA256" =~ ^[0-9a-f]{64}$ && "$INPUT_SIZE" =~ ^[0-9]+$ ]] || \
    fail invalid_input 'the stable input copy metadata was invalid'
  if [[ "$VERIFIED_DUMP_SHA256" != "$source_sha256_before" || "$INPUT_SIZE" != "$source_size_before" ]]; then
    fail input_changed 'the input backup changed while the stable copy was created'
  fi
  INPUT_SHA256="$VERIFIED_DUMP_SHA256"
  if [[ -n "$EXPECTED_SHA256" && "$INPUT_SHA256" != "$EXPECTED_SHA256" ]]; then
    fail input_checksum_mismatch 'the stable input copy does not match --expected-sha256'
  fi

  magic="$(od -An -tx1 -N5 "$VERIFIED_DUMP_PATH" | tr -d '[:space:]')"
  if [[ "$magic" == 1f8b* ]]; then
    if ! gzip -t -- "$VERIFIED_DUMP_PATH" >"$WORK_DIR/gzip-check.log" 2>&1; then
      fail format_invalid 'the gzip backup is corrupt'
    fi
    set +o pipefail
    inner_magic="$(gzip -dc -- "$VERIFIED_DUMP_PATH" 2>/dev/null | head -c 5 | od -An -tx1 | tr -d '[:space:]')"
    set -o pipefail
    if [[ "$inner_magic" == '5047444d50' ]]; then
      INPUT_FORMAT='postgres-custom-gzip'
      RESTORE_METHOD='pg_restore-single-transaction'
    else
      INPUT_FORMAT='plain-sql-gzip'
      RESTORE_METHOD='psql-single-transaction'
    fi
  elif [[ "$magic" == '5047444d50' ]]; then
    INPUT_FORMAT='postgres-custom'
    RESTORE_METHOD='pg_restore-single-transaction'
  else
    INPUT_FORMAT='plain-sql'
    RESTORE_METHOD='psql-single-transaction'
  fi
fi

suffix="$(openssl rand -hex 16)" || fail isolation_setup_failed 'could not generate isolated resource names'
RESTORE_RUN_ID="$suffix"
admin_password="$(openssl rand -hex 24)" || fail isolation_setup_failed 'could not generate the temporary administrator password'
restore_password="$(openssl rand -hex 24)" || fail isolation_setup_failed 'could not generate the temporary restore password'
NETWORK_NAME="modelport-restore-$suffix"
PG_CONTAINER="modelport-restore-pg-$suffix"
PG_VOLUME="modelport-restore-pgdata-$suffix"
SPACE_CONTAINER="modelport-restore-space-$suffix"
admin_user="modelport_admin_$suffix"
restore_user="modelport_restore_user_$suffix"
database_name="modelport_restore_$suffix"
env_file="$WORK_DIR/postgres.env"
pgpass_line="127.0.0.1:5432:$database_name:$restore_user:$restore_password"
{
  printf 'POSTGRES_USER=%s\n' "$admin_user"
  printf 'POSTGRES_PASSWORD=%s\n' "$admin_password"
  printf 'POSTGRES_DB=postgres\n'
  printf 'PGDATA=/var/lib/postgresql/data\n'
} > "$env_file"
chmod 0600 "$env_file"

validate_local_docker_endpoint || \
  fail docker_endpoint_invalid 'Docker must use an existing local Unix-socket context and DOCKER_HOST must be unset'

postgres_image_metadata="$(docker image inspect --format '{{.Id}} {{.Os}} {{.Architecture}}' \
  "$POSTGRES_IMAGE" 2>"$WORK_DIR/image-inspect.log")" || \
  fail image_unavailable 'the pinned PostgreSQL image is not available locally'
read -r postgres_image_id postgres_image_os postgres_image_architecture <<<"$postgres_image_metadata"
[[ "$postgres_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || \
  fail image_unavailable 'the pinned PostgreSQL image resolved to an invalid image ID'
[[ "$postgres_image_os" == 'linux' ]] || \
  fail image_unavailable 'the pinned PostgreSQL image is not a Linux image'
case "$postgres_image_architecture" in
  amd64|arm64) ;;
  *) fail image_unavailable 'the pinned PostgreSQL image architecture is unsupported' ;;
esac

NETWORK_CREATED=1
if ! docker network create \
  --internal \
  --label 'com.abingooo.modelport.purpose=isolated-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  "$NETWORK_NAME" >"$WORK_DIR/network-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the internal Docker network'
fi

PG_VOLUME_CREATED=1
if ! docker volume create \
  --label 'com.abingooo.modelport.purpose=isolated-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  "$PG_VOLUME" >"$WORK_DIR/volume-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the isolated PostgreSQL volume'
fi

SPACE_CREATED=1
if ! docker create \
  --name "$SPACE_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network none \
  --pull=never \
  --read-only \
  --user '0:0' \
  --cap-drop ALL \
  --cap-add CHOWN \
  --security-opt no-new-privileges \
  --mount "type=volume,src=$PG_VOLUME,dst=/var/lib/postgresql" \
  --entrypoint sh \
  "$postgres_image_id" -c \
  'mkdir -p /var/lib/postgresql/data && chown 70:70 /var/lib/postgresql /var/lib/postgresql/data && exec df -Pk /var/lib/postgresql' \
  >"$WORK_DIR/space-create.log" 2>&1; then
  fail isolation_setup_failed 'could not create the Docker volume space-check container'
fi
if ! docker_space_output="$(docker start --attach "$SPACE_CONTAINER" 2>"$WORK_DIR/space-check.log")"; then
  fail isolation_setup_failed 'could not inspect available Docker volume space'
fi
if ! docker rm --volumes "$SPACE_CONTAINER" >"$WORK_DIR/space-remove.log" 2>&1; then
  fail isolation_setup_failed 'could not remove the Docker volume space-check container'
fi
SPACE_CREATED=0
docker_available_kib="$(awk 'NR > 1 {available = $4} END {print available}' <<<"$docker_space_output")"
[[ "$docker_available_kib" =~ ^[0-9]+$ ]] || \
  fail isolation_setup_failed 'Docker returned an invalid available-space measurement'
capacity_input_size="${INPUT_SIZE:-0}"
if ! awk -v available_kib="$docker_available_kib" -v input_bytes="$capacity_input_size" '
  BEGIN {
    minimum_kib = 1048576
    input_kib = int((input_bytes + 1023) / 1024)
    required_kib = minimum_kib + (input_kib * 2)
    exit !(available_kib >= required_kib)
  }
'; then
  fail isolation_setup_failed 'Docker storage has less than the minimum restore headroom'
fi

# The official image's initialization-only server listens on Unix sockets.
# Require loopback TCP so this becomes healthy only after the final server starts.
PG_CREATED=1
if ! docker create \
  --name "$PG_CONTAINER" \
  --label 'com.abingooo.modelport.purpose=isolated-restore' \
  --label "$RESTORE_RUN_LABEL=$RESTORE_RUN_ID" \
  --network "$NETWORK_NAME" \
  --pull=never \
  --read-only \
  --user '70:70' \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --pids-limit 512 \
  --memory 2g \
  --cpus 2 \
  --mount "type=volume,src=$PG_VOLUME,dst=/var/lib/postgresql" \
  --tmpfs '/var/run/postgresql:rw,nosuid,nodev,noexec,uid=70,gid=70,mode=0770' \
  --tmpfs '/tmp:rw,exec,nosuid,nodev,uid=70,gid=70,mode=0700' \
  --env-file "$env_file" \
  --health-cmd "pg_isready -h 127.0.0.1 -p 5432 -U $admin_user -d postgres" \
  --health-interval 1s \
  --health-timeout 3s \
  --health-retries 60 \
  "$postgres_image_id" >"$WORK_DIR/postgres-create.log" 2>&1; then
  fail postgres_start_failed 'could not create the disposable PostgreSQL container'
fi
if ! docker start "$PG_CONTAINER" >"$WORK_DIR/postgres-start.log" 2>&1; then
  fail postgres_start_failed 'could not start the disposable PostgreSQL container'
fi

healthy=0
health_attempt=0
while (( health_attempt < 75 )); do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$PG_CONTAINER" 2>/dev/null || true)"
  if [[ "$health" == 'healthy' ]]; then
    healthy=1
    break
  fi
  if [[ "$health" == 'exited' || "$health" == 'dead' || "$health" == 'unhealthy' ]]; then
    break
  fi
  sleep 1
  health_attempt=$((health_attempt + 1))
done
[[ "$healthy" -eq 1 ]] || fail postgres_start_failed 'disposable PostgreSQL did not become healthy'

if ! {
  printf 'CREATE ROLE "%s" LOGIN PASSWORD '\''%s'\'' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION;\n' \
    "$restore_user" "$restore_password"
  printf 'CREATE DATABASE "%s" OWNER "%s" TEMPLATE template0 ENCODING '\''UTF8'\'';\n' \
    "$database_name" "$restore_user"
  printf 'REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;\n'
  printf 'REVOKE CONNECT ON DATABASE template1 FROM PUBLIC;\n'
} | docker exec --user '70:70' -i "$PG_CONTAINER" \
  psql -X -q -v ON_ERROR_STOP=1 -U "$admin_user" -d postgres \
  >"$WORK_DIR/database-setup.log" 2>&1
then
  fail database_setup_failed 'could not create the temporary restore role and database'
fi

if ! printf '%s\n' "$pgpass_line" | docker exec --user '70:70' -i "$PG_CONTAINER" \
  sh -c 'umask 077; cat > /tmp/modelport-restore.pgpass' \
  >"$WORK_DIR/pgpass-setup.log" 2>&1; then
  fail database_setup_failed 'could not provision temporary database credentials'
fi

psql_exec=(docker exec --user '70:70' -i -e PGPASSFILE=/tmp/modelport-restore.pgpass "$PG_CONTAINER" \
  psql -X -qAt -v ON_ERROR_STOP=1 -h 127.0.0.1 -p 5432 -U "$restore_user" -d "$database_name")

if [[ "$SOURCE_KIND" != 'empty' ]]; then
  case "$INPUT_FORMAT" in
    postgres-custom)
      if ! docker exec --user '70:70' -i "$PG_CONTAINER" pg_restore --list \
        < "$VERIFIED_DUMP_PATH" >"$WORK_DIR/archive-list.log" 2>&1; then
        fail format_invalid 'the PostgreSQL custom-format archive failed pg_restore --list validation'
      fi
      ;;
    postgres-custom-gzip)
      if ! gzip -dc -- "$VERIFIED_DUMP_PATH" | \
        docker exec --user '70:70' -i "$PG_CONTAINER" pg_restore --list \
        >"$WORK_DIR/archive-list.log" 2>&1; then
        fail format_invalid 'the gzipped PostgreSQL custom-format archive failed pg_restore --list validation'
      fi
      ;;
  esac
  case "$INPUT_FORMAT" in
    plain-sql)
      if ! docker exec --user '70:70' -i -e PGPASSFILE=/tmp/modelport-restore.pgpass "$PG_CONTAINER" \
        psql -X -v ON_ERROR_STOP=1 --single-transaction \
        -h 127.0.0.1 -p 5432 -U "$restore_user" -d "$database_name" \
        < "$VERIFIED_DUMP_PATH" >"$WORK_DIR/restore.log" 2>&1; then
        fail restore_failed 'plain SQL restore failed'
      fi
      ;;
    plain-sql-gzip)
      if ! gzip -dc -- "$VERIFIED_DUMP_PATH" | docker exec --user '70:70' -i -e PGPASSFILE=/tmp/modelport-restore.pgpass "$PG_CONTAINER" \
        psql -X -v ON_ERROR_STOP=1 --single-transaction \
        -h 127.0.0.1 -p 5432 -U "$restore_user" -d "$database_name" \
        >"$WORK_DIR/restore.log" 2>&1; then
        fail restore_failed 'gzipped plain SQL restore failed'
      fi
      ;;
    postgres-custom)
      if ! docker exec --user '70:70' -i -e PGPASSFILE=/tmp/modelport-restore.pgpass "$PG_CONTAINER" \
        pg_restore --exit-on-error --single-transaction --clean --if-exists --no-owner --no-acl \
        -h 127.0.0.1 -p 5432 -U "$restore_user" -d "$database_name" \
        < "$VERIFIED_DUMP_PATH" >"$WORK_DIR/restore.log" 2>&1; then
        fail restore_failed 'PostgreSQL custom-format restore failed'
      fi
      ;;
    postgres-custom-gzip)
      if ! gzip -dc -- "$VERIFIED_DUMP_PATH" | docker exec --user '70:70' -i -e PGPASSFILE=/tmp/modelport-restore.pgpass "$PG_CONTAINER" \
        pg_restore --exit-on-error --single-transaction --clean --if-exists --no-owner --no-acl \
        -h 127.0.0.1 -p 5432 -U "$restore_user" -d "$database_name" \
        >"$WORK_DIR/restore.log" 2>&1; then
        fail restore_failed 'gzipped PostgreSQL custom-format restore failed'
      fi
      ;;
    *) fail format_invalid 'unsupported input format' ;;
  esac
fi

collect_snapshot() {
  local phase="$1"
  local destination="$2"
  if ! "${psql_exec[@]}" > "$destination" 2>"$WORK_DIR/$phase-metadata.log" <<'SQL'
-- MODELPORT_REPORT_SNAPSHOT_V1
CREATE TEMP TABLE report_migrations (
  filename text PRIMARY KEY,
  checksum text NOT NULL,
  applied_at text
);
DO $report$
BEGIN
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    INSERT INTO report_migrations (filename, checksum, applied_at)
    SELECT filename::text, checksum::text, applied_at::text
    FROM public.schema_migrations;
  END IF;
END
$report$;

CREATE TEMP TABLE report_key_counts (
  table_name text PRIMARY KEY,
  table_exists boolean NOT NULL,
  row_count bigint
);
DO $report$
DECLARE
  candidate text;
  counted bigint;
BEGIN
  FOREACH candidate IN ARRAY ARRAY[
    'users', 'groups', 'api_keys', 'accounts', 'channels', 'usage_logs',
    'payment_orders', 'user_subscriptions', 'lottery_campaigns',
    'lottery_prizes', 'lottery_entries', 'lottery_draw_runs', 'lottery_events',
    'instruction_audit_events', 'instruction_audit_v2_events',
    'batch_image_jobs'
  ] LOOP
    IF to_regclass(format('public.%I', candidate)) IS NULL THEN
      INSERT INTO report_key_counts VALUES (candidate, FALSE, NULL);
    ELSE
      EXECUTE format('SELECT count(*) FROM public.%I', candidate) INTO counted;
      INSERT INTO report_key_counts VALUES (candidate, TRUE, counted);
    END IF;
  END LOOP;
END
$report$;

CREATE TEMP TABLE report_all_table_counts (
  table_name text PRIMARY KEY,
  row_count bigint NOT NULL
);
DO $report$
DECLARE
  item record;
  counted bigint;
BEGIN
  FOR item IN
    SELECT cls.relname AS table_name
    FROM pg_class cls
    JOIN pg_namespace ns ON ns.oid = cls.relnamespace
    WHERE ns.nspname = 'public'
      AND cls.relkind IN ('r', 'p')
      AND cls.relname <> 'schema_migrations'
    ORDER BY cls.relname
  LOOP
    EXECUTE format('SELECT count(*) FROM public.%I', item.table_name) INTO counted;
    INSERT INTO report_all_table_counts VALUES (item.table_name, counted);
  END LOOP;
END
$report$;

-- Migration 234 creates a number of audit tables and intentionally seeds only
-- a small, fixed set of rows. Keep row-level evidence for those seed tables so
-- the restore invariant can distinguish those additions from arbitrary writes.
CREATE TEMP TABLE report_audit_seed_tables (
  seed_name text PRIMARY KEY,
  table_name text NOT NULL,
  table_exists boolean NOT NULL
);

CREATE TEMP TABLE report_audit_seed_rows (
  seed_name text NOT NULL,
  row_key text NOT NULL,
  fingerprint text NOT NULL,
  canonical boolean NOT NULL,
  PRIMARY KEY (seed_name, row_key)
);

CREATE OR REPLACE FUNCTION pg_temp.report_audit_row_fingerprint(
  value jsonb,
  excluded_keys text[] DEFAULT ARRAY[]::text[]
)
RETURNS text
LANGUAGE SQL
IMMUTABLE
AS $fingerprint$
  SELECT encode(
    sha256(convert_to((value - excluded_keys)::text, 'UTF8')),
    'hex'
  )
$fingerprint$;

DO $report$
BEGIN
  INSERT INTO report_audit_seed_tables VALUES
    ('instruction_audit_state', 'instruction_audit_state',
      to_regclass('public.instruction_audit_state') IS NOT NULL),
    ('instruction_audit_reason_policies', 'instruction_audit_reason_policies',
      to_regclass('public.instruction_audit_reason_policies') IS NOT NULL),
    ('instruction_audit_runtime_config', 'instruction_audit_runtime_config',
      to_regclass('public.instruction_audit_runtime_config') IS NOT NULL),
    ('instruction_audit_outcome_rollup_state', 'instruction_audit_outcome_rollup_state',
      to_regclass('public.instruction_audit_outcome_rollup_state') IS NOT NULL),
    ('instruction_audit_operational_counters', 'instruction_audit_operational_counters',
      to_regclass('public.instruction_audit_operational_counters') IS NOT NULL),
    ('instruction_audit_sensitive_access_grants', 'instruction_audit_sensitive_access_grants',
      to_regclass('public.instruction_audit_sensitive_access_grants') IS NOT NULL),
    ('instruction_audit_v2_config', 'instruction_audit_v2_config',
      to_regclass('public.instruction_audit_v2_config') IS NOT NULL),
    ('instruction_audit_v2_client_profiles', 'instruction_audit_v2_client_profiles',
      to_regclass('public.instruction_audit_v2_client_profiles') IS NOT NULL);

  IF to_regclass('public.instruction_audit_state') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_state',
        r.id::text,
        pg_temp.report_audit_row_fingerprint(to_jsonb(r), ARRAY['updated_at']),
        r.id = 1
      FROM public.instruction_audit_state AS r
    $sql$;
  END IF;

  IF to_regclass('public.instruction_audit_reason_policies') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_reason_policies',
        r.reason::text,
        pg_temp.report_audit_row_fingerprint(to_jsonb(r), ARRAY['updated_at']),
        r.reason IN (
          'hash_mismatch', 'fields_missing', 'field_invalid', 'invalid_json',
          'request_too_large', 'structure_too_complex', 'parse_timeout',
          'config_unavailable', 'group_not_allowed', 'client_not_allowed',
          'ai_rejected', 'ai_uncertain', 'ai_error'
        )
      FROM public.instruction_audit_reason_policies AS r
    $sql$;
  END IF;

  IF to_regclass('public.instruction_audit_runtime_config') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_runtime_config',
        r.id::text,
        pg_temp.report_audit_row_fingerprint(to_jsonb(r), ARRAY['updated_at']),
        r.id = 1
      FROM public.instruction_audit_runtime_config AS r
    $sql$;
  END IF;

  IF to_regclass('public.instruction_audit_outcome_rollup_state') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_outcome_rollup_state',
        r.id::text,
        pg_temp.report_audit_row_fingerprint(to_jsonb(r), ARRAY['updated_at']),
        r.id = 1
      FROM public.instruction_audit_outcome_rollup_state AS r
    $sql$;
  END IF;

  IF to_regclass('public.instruction_audit_operational_counters') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_operational_counters',
        r.id::text,
        pg_temp.report_audit_row_fingerprint(to_jsonb(r), ARRAY['updated_at']),
        r.id = 1
      FROM public.instruction_audit_operational_counters AS r
    $sql$;
  END IF;

  -- The migration bootstrap grant is conditional: it is inserted only when
  -- the table is empty and targets the earliest active, non-deleted admin.
  IF to_regclass('public.instruction_audit_sensitive_access_grants') IS NOT NULL THEN
    IF to_regclass('public.users') IS NOT NULL THEN
      EXECUTE $sql$
        INSERT INTO report_audit_seed_rows
          (seed_name, row_key, fingerprint, canonical)
        SELECT
          'instruction_audit_sensitive_access_grants',
          g.id::text,
          pg_temp.report_audit_row_fingerprint(to_jsonb(g), ARRAY[]::text[]),
          g.grant_source = 'migration_bootstrap'
          AND g.grant_reason = 'Automatic bootstrap for the earliest active administrator'
          AND g.granted_by IS NULL
          AND g.revoked_at IS NULL
          AND g.revoke_source IS NULL
          AND a.id IS NOT NULL
          AND g.subject_user_id = a.id
          AND g.subject_email_snapshot = a.email
        FROM public.instruction_audit_sensitive_access_grants AS g
        LEFT JOIN LATERAL (
          SELECT u.id, u.email
          FROM public.users AS u
          WHERE u.role = 'admin'
            AND u.status = 'active'
            AND u.deleted_at IS NULL
          ORDER BY u.created_at ASC, u.id ASC
          LIMIT 1
        ) AS a ON TRUE
      $sql$;
    ELSE
      EXECUTE $sql$
        INSERT INTO report_audit_seed_rows
          (seed_name, row_key, fingerprint, canonical)
        SELECT
          'instruction_audit_sensitive_access_grants',
          g.id::text,
          pg_temp.report_audit_row_fingerprint(to_jsonb(g), ARRAY[]::text[]),
          FALSE
        FROM public.instruction_audit_sensitive_access_grants AS g
      $sql$;
    END IF;
  END IF;

  IF to_regclass('public.instruction_audit_v2_config') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_v2_config',
        r.id::text,
        pg_temp.report_audit_row_fingerprint(
          to_jsonb(r),
          ARRAY['updated_at', 'allow_empty_fields', 'async_retry_schedule_seconds']
        ),
        r.id = 1
      FROM public.instruction_audit_v2_config AS r
    $sql$;
  END IF;

  IF to_regclass('public.instruction_audit_v2_client_profiles') IS NOT NULL THEN
    EXECUTE $sql$
      INSERT INTO report_audit_seed_rows
        (seed_name, row_key, fingerprint, canonical)
      SELECT
        'instruction_audit_v2_client_profiles',
        r.profile_key::text,
        pg_temp.report_audit_row_fingerprint(
          to_jsonb(r),
          ARRAY['updated_at', 'prompt_audit_enabled']
        ),
        r.profile_key IN (
          'codex_vscode', 'codex_cli', 'codex_desktop', 'opencode',
          'modelport_internal', 'other', 'unknown'
        )
      FROM public.instruction_audit_v2_client_profiles AS r
    $sql$;
  END IF;
END
$report$;

CREATE TEMP TABLE report_settings (
  setting_key text PRIMARY KEY,
  value_sha256 text NOT NULL
);
DO $report$
BEGIN
  IF to_regclass('public.settings') IS NOT NULL
     AND EXISTS (
       SELECT 1 FROM information_schema.columns
       WHERE table_schema = 'public' AND table_name = 'settings' AND column_name = 'key'
     )
     AND EXISTS (
       SELECT 1 FROM information_schema.columns
       WHERE table_schema = 'public' AND table_name = 'settings' AND column_name = 'value'
     ) THEN
    INSERT INTO report_settings (setting_key, value_sha256)
    SELECT key::text, encode(sha256(convert_to(value::text, 'UTF8')), 'hex')
    FROM public.settings;
  END IF;
END
$report$;

CREATE TEMP TABLE report_financial (
  aggregate_key text PRIMARY KEY,
  table_exists boolean NOT NULL,
  column_exists boolean NOT NULL,
  non_null_rows bigint,
  total text
);
DO $report$
DECLARE
  item record;
  rows_count bigint;
  total_value text;
  relation_exists boolean;
  attribute_exists boolean;
BEGIN
  FOR item IN
    SELECT * FROM (VALUES
      ('users', 'balance'),
      ('users', 'frozen_balance'),
      ('users', 'total_recharged'),
      ('payment_orders', 'amount'),
      ('payment_orders', 'pay_amount'),
      ('payment_orders', 'refund_amount'),
      ('usage_logs', 'total_cost'),
      ('usage_logs', 'actual_cost'),
      ('api_keys', 'quota'),
      ('api_keys', 'quota_used'),
      ('user_subscriptions', 'daily_usage_usd'),
      ('user_subscriptions', 'weekly_usage_usd'),
      ('user_subscriptions', 'monthly_usage_usd')
    ) AS candidates(table_name, column_name)
  LOOP
    relation_exists := to_regclass(format('public.%I', item.table_name)) IS NOT NULL;
    SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = item.table_name
        AND column_name = item.column_name
    ) INTO attribute_exists;
    IF relation_exists AND attribute_exists THEN
      EXECUTE format(
        'SELECT count(%1$I), COALESCE(sum(%1$I), 0)::text FROM public.%2$I',
        item.column_name, item.table_name
      ) INTO rows_count, total_value;
      INSERT INTO report_financial VALUES (
        item.table_name || '.' || item.column_name,
        TRUE, TRUE, rows_count, total_value
      );
    ELSE
      INSERT INTO report_financial VALUES (
        item.table_name || '.' || item.column_name,
        relation_exists, attribute_exists, NULL, NULL
      );
    END IF;
  END LOOP;
END
$report$;

CREATE TEMP TABLE report_sequences (
  sequence_schema text NOT NULL,
  sequence_name text NOT NULL,
  start_value text NOT NULL,
  increment_by text NOT NULL,
  min_value text NOT NULL,
  max_value text NOT NULL,
  cache_size text NOT NULL,
  cycle boolean NOT NULL,
  last_value text,
  is_called boolean,
  owned_table text,
  owned_column text,
  owned_table_min text,
  owned_table_max text,
  next_value text,
  position_valid boolean NOT NULL,
  PRIMARY KEY (sequence_schema, sequence_name)
);
DO $report$
DECLARE
  item record;
  current_last text;
  current_called boolean;
  table_min text;
  table_max text;
  candidate numeric;
  candidate_text text;
  valid_position boolean;
BEGIN
  FOR item IN
    SELECT
      ns.nspname AS sequence_schema,
      seq.relname AS sequence_name,
      cfg.seqstart,
      cfg.seqincrement,
      cfg.seqmin,
      cfg.seqmax,
      cfg.seqcache,
      cfg.seqcycle,
      owned.relname AS owned_table,
      attr.attname AS owned_column
    FROM pg_class seq
    JOIN pg_namespace ns ON ns.oid = seq.relnamespace
    JOIN pg_sequence cfg ON cfg.seqrelid = seq.oid
    LEFT JOIN pg_depend dep
      ON dep.classid = 'pg_class'::regclass
      AND dep.objid = seq.oid
      AND dep.objsubid = 0
      AND dep.refclassid = 'pg_class'::regclass
      AND dep.deptype IN ('a', 'i')
    LEFT JOIN pg_class owned ON owned.oid = dep.refobjid
    LEFT JOIN pg_attribute attr
      ON attr.attrelid = dep.refobjid AND attr.attnum = dep.refobjsubid
    WHERE ns.nspname = 'public'
    ORDER BY seq.relname
  LOOP
    EXECUTE format(
      'SELECT last_value::text, is_called FROM %I.%I',
      item.sequence_schema, item.sequence_name
    ) INTO current_last, current_called;
    table_min := NULL;
    table_max := NULL;
    candidate_text := NULL;
    valid_position := TRUE;
    IF item.owned_table IS NOT NULL AND item.owned_column IS NOT NULL THEN
      EXECUTE format(
        'SELECT min(%1$I)::text, max(%1$I)::text FROM public.%2$I',
        item.owned_column, item.owned_table
      ) INTO table_min, table_max;
      IF current_last IS NULL OR current_called IS NULL THEN
        valid_position := FALSE;
      ELSE
        candidate := current_last::numeric;
        IF current_called THEN
          candidate := candidate + item.seqincrement;
        END IF;
        IF candidate > item.seqmax THEN
          IF item.seqcycle THEN
            candidate := item.seqmin;
          ELSE
            valid_position := FALSE;
          END IF;
        ELSIF candidate < item.seqmin THEN
          IF item.seqcycle THEN
            candidate := item.seqmax;
          ELSE
            valid_position := FALSE;
          END IF;
        END IF;
        candidate_text := candidate::text;
        IF valid_position AND item.seqincrement > 0 AND table_max IS NOT NULL THEN
          valid_position := candidate > table_max::numeric;
        ELSIF valid_position AND item.seqincrement < 0 AND table_min IS NOT NULL THEN
          valid_position := candidate < table_min::numeric;
        END IF;
      END IF;
    END IF;
    INSERT INTO report_sequences VALUES (
      item.sequence_schema, item.sequence_name, item.seqstart::text,
      item.seqincrement::text, item.seqmin::text, item.seqmax::text,
      item.seqcache::text, item.seqcycle, current_last, current_called,
      item.owned_table, item.owned_column, table_min, table_max,
      candidate_text, valid_position
    );
  END LOOP;
END
$report$;

CREATE OR REPLACE FUNCTION pg_temp.report_digest_transition(
  state_value bytea,
  next_value bytea
)
RETURNS bytea
LANGUAGE SQL
IMMUTABLE
AS $digest$
  SELECT sha256(COALESCE(state_value, sha256(''::bytea)) || next_value)
$digest$;

CREATE AGGREGATE pg_temp.report_digest(bytea) (
  SFUNC = pg_temp.report_digest_transition,
  STYPE = bytea
);

CREATE TEMP TABLE report_ciphertext (
  aggregate_key text PRIMARY KEY,
  table_name text NOT NULL,
  column_name text NOT NULL,
  value_type text NOT NULL,
  primary_key_columns text[] NOT NULL,
  digest_binding text NOT NULL,
  rows_present bigint NOT NULL,
  total_bytes text NOT NULL,
  digest_sha256 text NOT NULL
);
DO $report$
DECLARE
  item record;
  value_expression text;
  primary_key_columns text[];
  primary_key_arguments text;
  primary_key_column text;
  row_hash_expression text;
  rows_count bigint;
  bytes_count text;
  aggregate_digest text;
BEGIN
  FOR item IN
    SELECT table_name, column_name, data_type
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND (
        position('ciphertext' in lower(column_name)) > 0
        OR lower(column_name) LIKE '%\_encrypted' ESCAPE '\'
      )
      AND data_type IN ('bytea', 'text', 'character varying', 'character')
    ORDER BY table_name, column_name
  LOOP
    IF item.data_type = 'bytea' THEN
      value_expression := format('%I', item.column_name);
    ELSE
      value_expression := format('convert_to(%I::text, ''UTF8'')', item.column_name);
    END IF;
    SELECT array_agg(attribute.attname ORDER BY key_column.ordinality)
    INTO primary_key_columns
    FROM pg_class table_class
    JOIN pg_namespace table_namespace ON table_namespace.oid = table_class.relnamespace
    JOIN pg_index primary_index
      ON primary_index.indrelid = table_class.oid AND primary_index.indisprimary
    CROSS JOIN LATERAL unnest(primary_index.indkey)
      WITH ORDINALITY AS key_column(attribute_number, ordinality)
    JOIN pg_attribute attribute
      ON attribute.attrelid = table_class.oid
      AND attribute.attnum = key_column.attribute_number
    WHERE table_namespace.nspname = 'public'
      AND table_class.relname = item.table_name;
    primary_key_columns := COALESCE(primary_key_columns, ARRAY[]::text[]);
    IF cardinality(primary_key_columns) > 0 THEN
      primary_key_arguments := '';
      FOREACH primary_key_column IN ARRAY primary_key_columns LOOP
        IF primary_key_arguments <> '' THEN
          primary_key_arguments := primary_key_arguments || ', ';
        END IF;
        primary_key_arguments := primary_key_arguments || format('%I', primary_key_column);
      END LOOP;
      row_hash_expression := format(
        'encode(sha256(convert_to(jsonb_build_array(%s)::text, ''UTF8'') || decode(''00'', ''hex'') || sha256(%s)), ''hex'')',
        primary_key_arguments, value_expression
      );
    ELSE
      row_hash_expression := format('encode(sha256(%s), ''hex'')', value_expression);
    END IF;
    EXECUTE format(
      'SELECT count(*), COALESCE(sum(octet_length(value_bytes)), 0)::text,
              encode(COALESCE(
                pg_temp.report_digest(decode(row_hash, ''hex'') ORDER BY row_hash),
                sha256(''''::bytea)
              ), ''hex'')
       FROM (
         SELECT %1$s AS value_bytes, %5$s AS row_hash
         FROM %2$I.%3$I
         WHERE %4$I IS NOT NULL
       ) AS ciphertext_rows',
      value_expression, 'public', item.table_name, item.column_name, row_hash_expression
    ) INTO rows_count, bytes_count, aggregate_digest;
    INSERT INTO report_ciphertext VALUES (
      item.table_name || '.' || item.column_name,
      item.table_name, item.column_name, item.data_type,
      primary_key_columns,
      CASE WHEN cardinality(primary_key_columns) > 0
        THEN 'primary-key-and-ciphertext'
        ELSE 'ciphertext-multiset-no-primary-key'
      END,
      rows_count, bytes_count, aggregate_digest
    );
  END LOOP;
END
$report$;

SELECT jsonb_build_object(
  'schema_migrations', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'filename', filename,
      'checksum', checksum,
      'applied_at', applied_at
    ) ORDER BY filename)
    FROM report_migrations
  ), '[]'::jsonb),
  'key_table_counts', COALESCE((
    SELECT jsonb_object_agg(table_name, jsonb_build_object(
      'exists', table_exists,
      'rows', row_count
    ) ORDER BY table_name)
    FROM report_key_counts
  ), '{}'::jsonb),
  'all_table_counts', COALESCE((
    SELECT jsonb_object_agg(table_name, row_count ORDER BY table_name)
    FROM report_all_table_counts
  ), '{}'::jsonb),
  'audit_seed_summaries', COALESCE((
    SELECT jsonb_object_agg(
      seed_name,
      jsonb_build_object(
        'table_name', table_name,
        'table_exists', table_exists,
        'rows', COALESCE((
          SELECT jsonb_agg(jsonb_build_object(
            'key', row_key,
            'fingerprint', fingerprint,
            'canonical', canonical
          ) ORDER BY row_key)
          FROM report_audit_seed_rows AS seed_row
          WHERE seed_row.seed_name = seed_table.seed_name
        ), '[]'::jsonb)
      )
    )
    FROM report_audit_seed_tables AS seed_table
  ), '{}'::jsonb),
  'settings_value_summaries', COALESCE((
    SELECT jsonb_object_agg(setting_key, value_sha256 ORDER BY setting_key)
    FROM report_settings
  ), '{}'::jsonb),
  'financial_aggregates', COALESCE((
    SELECT jsonb_object_agg(aggregate_key, jsonb_build_object(
      'table_exists', table_exists,
      'column_exists', column_exists,
      'non_null_rows', non_null_rows,
      'sum', total
    ) ORDER BY aggregate_key)
    FROM report_financial
  ), '{}'::jsonb),
  'sequences', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'schema', sequence_schema,
      'name', sequence_name,
      'start', start_value,
      'increment', increment_by,
      'min', min_value,
      'max', max_value,
      'cache', cache_size,
      'cycle', cycle,
      'last_value', last_value,
      'is_called', is_called,
      'owned_table', owned_table,
      'owned_column', owned_column,
      'owned_table_min', owned_table_min,
      'owned_table_max', owned_table_max,
      'next_value', next_value,
      'position_valid', position_valid
    ) ORDER BY sequence_schema, sequence_name)
    FROM report_sequences
  ), '[]'::jsonb),
  'constraints', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'table', tbl.relname,
      'name', con.conname,
      'type', con.contype,
      'validated', con.convalidated,
      'definition_sha256', encode(sha256(convert_to(pg_get_constraintdef(con.oid, TRUE), 'UTF8')), 'hex')
    ) ORDER BY tbl.relname, con.conname)
    FROM pg_constraint con
    JOIN pg_class tbl ON tbl.oid = con.conrelid
    JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
    WHERE ns.nspname = 'public'
  ), '[]'::jsonb),
  'indexes', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'table', tbl.relname,
      'name', idx.relname,
      'unique', ind.indisunique,
      'primary', ind.indisprimary,
      'valid', ind.indisvalid,
      'ready', ind.indisready,
      'definition_sha256', encode(sha256(convert_to(pg_get_indexdef(idx.oid), 'UTF8')), 'hex')
    ) ORDER BY tbl.relname, idx.relname)
    FROM pg_index ind
    JOIN pg_class idx ON idx.oid = ind.indexrelid
    JOIN pg_class tbl ON tbl.oid = ind.indrelid
    JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
    WHERE ns.nspname = 'public'
  ), '[]'::jsonb),
  'foreign_keys', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'table', source_table.relname,
      'name', con.conname,
      'referenced_table', target_table.relname,
      'validated', con.convalidated,
      'definition_sha256', encode(sha256(convert_to(pg_get_constraintdef(con.oid, TRUE), 'UTF8')), 'hex')
    ) ORDER BY source_table.relname, con.conname)
    FROM pg_constraint con
    JOIN pg_class source_table ON source_table.oid = con.conrelid
    JOIN pg_namespace source_ns ON source_ns.oid = source_table.relnamespace
    JOIN pg_class target_table ON target_table.oid = con.confrelid
    WHERE source_ns.nspname = 'public' AND con.contype = 'f'
  ), '[]'::jsonb),
  'ciphertext_summaries', COALESCE((
    SELECT jsonb_object_agg(aggregate_key, jsonb_build_object(
      'table', table_name,
      'column', column_name,
      'type', value_type,
      'primary_key_columns', primary_key_columns,
      'digest_binding', digest_binding,
      'rows_present', rows_present,
      'total_bytes', total_bytes,
      'digest_sha256', digest_sha256
    ) ORDER BY aggregate_key)
    FROM report_ciphertext
  ), '{}'::jsonb)
)::text;
SQL
  then
    return 1
  fi
  jq -e 'type == "object"' "$destination" >/dev/null 2>&1
}

collect_bridge_contract() {
  local phase="$1"
  local destination="$2"
  if ! "${psql_exec[@]}" > "$destination" 2>"$WORK_DIR/$phase-bridge-contract.log" <<'SQL'
-- MODELPORT_BRIDGE_CONTRACT_V1
WITH public_columns AS (
  SELECT
    cls.relname AS table_name,
    attr.attname AS column_name,
    format_type(attr.atttypid, attr.atttypmod) AS data_type,
    attr.attnotnull AS not_null,
    pg_get_expr(def.adbin, def.adrelid) AS default_expression
  FROM pg_attribute attr
  JOIN pg_class cls ON cls.oid = attr.attrelid
  JOIN pg_namespace ns ON ns.oid = cls.relnamespace
  LEFT JOIN pg_attrdef def
    ON def.adrelid = attr.attrelid AND def.adnum = attr.attnum
  WHERE ns.nspname = 'public'
    AND cls.relkind IN ('r', 'p')
    AND attr.attnum > 0
    AND NOT attr.attisdropped
),
platform_constraints AS (
  SELECT
    cls.relname AS table_name,
    con.conname AS constraint_name,
    con.convalidated AS validated,
    pg_get_constraintdef(con.oid, TRUE) AS definition
  FROM pg_constraint con
  JOIN pg_class cls ON cls.oid = con.conrelid
  JOIN pg_namespace ns ON ns.oid = cls.relnamespace
  WHERE ns.nspname = 'public'
    AND con.contype = 'c'
)
SELECT jsonb_build_object(
  '232_modelport_free_group_bridge',
    EXISTS (
      SELECT 1 FROM public_columns
      WHERE table_name = 'groups' AND column_name = 'is_free'
        AND data_type = 'boolean' AND not_null
        AND default_expression IN ('false', 'false::boolean')
    )
    AND EXISTS (
      SELECT 1 FROM public_columns
      WHERE table_name = 'batch_image_jobs' AND column_name = 'is_free_billing'
        AND data_type = 'boolean' AND not_null
        AND default_expression IN ('false', 'false::boolean')
    )
    AND EXISTS (
      SELECT 1
      FROM pg_proc proc
      JOIN pg_namespace ns ON ns.oid = proc.pronamespace
      WHERE ns.nspname = 'public'
        AND proc.oid = to_regprocedure('public.enqueue_group_auth_cache_invalidation()')
        AND position(
          'OLD.is_free IS NOT DISTINCT FROM NEW.is_free'
          IN pg_get_functiondef(proc.oid)
        ) > 0
    )
    AND EXISTS (
      SELECT 1
      FROM pg_trigger trigger
      JOIN pg_class cls ON cls.oid = trigger.tgrelid
      JOIN pg_namespace ns ON ns.oid = cls.relnamespace
      WHERE ns.nspname = 'public'
        AND cls.relname = 'groups'
        AND trigger.tgname = 'trg_groups_auth_cache_invalidation'
        AND NOT trigger.tgisinternal
        AND trigger.tgenabled <> 'D'
        AND trigger.tgfoid = to_regprocedure('public.enqueue_group_auth_cache_invalidation()')
    ),
  '233_modelport_lottery_bridge',
    NOT EXISTS (
      SELECT 1
      FROM (VALUES
        ('lottery_campaigns'), ('lottery_prizes'), ('lottery_entries'),
        ('lottery_draw_runs'), ('lottery_events')
      ) AS required(table_name)
      WHERE to_regclass(format('public.%I', required.table_name)) IS NULL
    )
    AND EXISTS (
      SELECT 1 FROM public_columns
      WHERE table_name = 'lottery_campaigns'
        AND column_name = 'full_draw_participant_limit'
        AND data_type = 'integer'
    )
    AND EXISTS (
      SELECT 1 FROM public_columns
      WHERE table_name = 'lottery_campaigns'
        AND column_name = 'full_draw_reached_at'
        AND data_type = 'timestamp with time zone'
    )
    AND NOT EXISTS (
      SELECT 1
      FROM (VALUES
        ('lottery_campaigns', 1, 2),
        ('lottery_prizes', 1, 2),
        ('lottery_entries', 1, 5),
        ('lottery_draw_runs', 1, 2),
        ('lottery_events', 1, 4)
      ) AS required(table_name, primary_keys, foreign_keys)
      WHERE (
        SELECT count(*) FROM pg_constraint con
        WHERE con.conrelid = to_regclass(format('public.%I', required.table_name))
          AND con.contype = 'p' AND con.convalidated
      ) < required.primary_keys
      OR (
        SELECT count(*) FROM pg_constraint con
        WHERE con.conrelid = to_regclass(format('public.%I', required.table_name))
          AND con.contype = 'f' AND con.convalidated
      ) < required.foreign_keys
    ),
  '234_modelport_instruction_audit_bridge',
    NOT EXISTS (
      SELECT 1
      FROM (VALUES
        ('instruction_audit_events', 'ai_review_id', 'bigint'),
        ('instruction_audit_v2_config', 'allow_empty_fields', 'boolean'),
        ('instruction_audit_v2_config', 'async_retry_schedule_seconds', 'integer[]'),
        ('instruction_audit_v2_ai_nodes', 'slot', 'character varying(16)'),
        ('instruction_audit_v2_ai_nodes', 'response_mode', 'character varying(16)'),
        ('instruction_audit_v2_ai_nodes', 'max_output_tokens', 'integer'),
        ('instruction_audit_v2_client_profiles', 'prompt_audit_enabled', 'boolean'),
        ('instruction_audit_v2_hashes', 'global_trust', 'boolean'),
        ('instruction_audit_v2_hashes', 'content_vault_id', 'bigint'),
        ('instruction_audit_v2_hashes', 'source_user_id', 'bigint'),
        ('instruction_audit_v2_hashes', 'source_user_email_snapshot', 'character varying(255)'),
        ('instruction_audit_v2_risk_hashes', 'source_user_id', 'bigint'),
        ('instruction_audit_v2_risk_hashes', 'source_user_email_snapshot', 'character varying(255)'),
        ('instruction_audit_v2_review_jobs', 'review_criteria', 'text'),
        ('instruction_audit_v2_review_jobs', 'observe_only', 'boolean'),
        ('instruction_audit_v2_review_jobs', 'source_user_id', 'bigint'),
        ('instruction_audit_v2_review_jobs', 'source_user_email_snapshot', 'character varying(255)'),
        ('instruction_audit_v2_events', 'selected_field', 'character varying(16)'),
        ('instruction_audit_v2_events', 'selected_sha256', 'character(64)'),
        ('instruction_audit_v2_events', 'review_job_id', 'bigint'),
        ('prompt_audit_jobs', 'audit_source', 'character varying(64)'),
        ('prompt_audit_jobs', 'instruction_config_version', 'bigint'),
        ('prompt_audit_jobs', 'client_profile_key', 'character varying(64)'),
        ('prompt_audit_jobs', 'client_profile_name', 'character varying(120)'),
        ('prompt_audit_jobs', 'trigger_reason', 'character varying(64)'),
        ('prompt_audit_jobs', 'model_contract_version', 'integer'),
        ('prompt_audit_jobs', 'effective_response_mode', 'character varying(16)'),
        ('prompt_audit_events', 'audit_source', 'character varying(64)'),
        ('prompt_audit_events', 'instruction_config_version', 'bigint'),
        ('prompt_audit_events', 'client_profile_key', 'character varying(64)'),
        ('prompt_audit_events', 'client_profile_name', 'character varying(120)'),
        ('prompt_audit_events', 'trigger_reason', 'character varying(64)'),
        ('prompt_audit_events', 'model_contract_version', 'integer'),
        ('prompt_audit_events', 'effective_response_mode', 'character varying(16)'),
        ('prompt_audit_events', 'scanner_version', 'character varying(255)')
      ) AS required(table_name, column_name, data_type)
      WHERE NOT EXISTS (
        SELECT 1 FROM public_columns column_state
        WHERE column_state.table_name = required.table_name
          AND column_state.column_name = required.column_name
          AND column_state.data_type = required.data_type
      )
    )
    AND EXISTS (SELECT 1 FROM instruction_audit_state WHERE id = 1)
    AND EXISTS (SELECT 1 FROM instruction_audit_runtime_config WHERE id = 1)
    AND EXISTS (SELECT 1 FROM instruction_audit_outcome_rollup_state WHERE id = 1)
    AND EXISTS (SELECT 1 FROM instruction_audit_operational_counters WHERE id = 1)
    AND EXISTS (SELECT 1 FROM instruction_audit_v2_config WHERE id = 1)
    AND (
      SELECT count(DISTINCT reason) = 13
      FROM instruction_audit_reason_policies
      WHERE reason IN (
        'hash_mismatch', 'fields_missing', 'field_invalid', 'invalid_json',
        'request_too_large', 'structure_too_complex', 'parse_timeout',
        'config_unavailable', 'group_not_allowed', 'client_not_allowed',
        'ai_rejected', 'ai_uncertain', 'ai_error'
      )
    )
    AND (
      SELECT count(DISTINCT profile_key) = 7 AND bool_and(built_in)
      FROM instruction_audit_v2_client_profiles
      WHERE profile_key IN (
        'codex_vscode', 'codex_cli', 'codex_desktop', 'opencode',
        'modelport_internal', 'other', 'unknown'
      )
    )
    AND EXISTS (SELECT 1 FROM settings WHERE key = 'instruction_audit_enabled')
    AND EXISTS (
      SELECT 1 FROM settings
      WHERE key = 'instruction_audit_evidence_retention_days'
    ),
  '235_batch_image_group_snapshot',
    EXISTS (
      SELECT 1 FROM public_columns
      WHERE table_name = 'batch_image_jobs' AND column_name = 'group_id'
        AND data_type = 'bigint' AND NOT not_null AND default_expression IS NULL
    ),
  '236_modelport_legacy_platform_constraints',
    EXISTS (
      SELECT 1 FROM platform_constraints contract
      WHERE contract.table_name = 'user_platform_quotas'
        AND contract.constraint_name = 'user_platform_quotas_platform_check'
        AND contract.validated
        AND NOT EXISTS (
          SELECT 1 FROM unnest(ARRAY[
            'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
            'kimi', 'zhipu', 'deepseek', 'qwen', 'glm', 'doubao',
            'siliconflow', 'openrouter', 'minimax', 'mimo'
          ]) AS required(value)
          WHERE position(required.value IN contract.definition) = 0
        )
    )
    AND EXISTS (
      SELECT 1 FROM platform_constraints contract
      WHERE contract.table_name = 'composite_model_routes'
        AND contract.constraint_name = 'composite_model_routes_target_platform_check'
        AND contract.validated
        AND NOT EXISTS (
          SELECT 1 FROM unnest(ARRAY[
            'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
            'kimi', 'zhipu', 'deepseek', 'qwen', 'glm', 'doubao',
            'siliconflow', 'openrouter', 'minimax', 'mimo'
          ]) AS required(value)
          WHERE position(required.value IN contract.definition) = 0
        )
    )
    AND EXISTS (
      SELECT 1 FROM platform_constraints contract
      WHERE contract.table_name = 'channel_monitors'
        AND contract.constraint_name = 'channel_monitors_provider_check'
        AND contract.validated
        AND NOT EXISTS (
          SELECT 1 FROM unnest(ARRAY[
            'openai', 'anthropic', 'gemini', 'grok', 'antigravity',
            'kimi', 'zhipu', 'deepseek', 'qwen', 'glm', 'doubao',
            'minimax', 'mimo'
          ]) AS required(value)
          WHERE position(required.value IN contract.definition) = 0
        )
    )
    AND EXISTS (
      SELECT 1 FROM platform_constraints contract
      WHERE contract.table_name = 'channel_monitor_request_templates'
        AND contract.constraint_name = 'channel_monitor_request_templates_provider_check'
        AND contract.validated
        AND NOT EXISTS (
          SELECT 1 FROM unnest(ARRAY[
            'openai', 'anthropic', 'gemini', 'grok', 'antigravity',
            'kimi', 'zhipu', 'deepseek', 'qwen', 'glm', 'doubao',
            'minimax', 'mimo'
          ]) AS required(value)
          WHERE position(required.value IN contract.definition) = 0
        )
    )
)::text;
SQL
  then
    return 1
  fi
  jq -e '
    type == "object" and length == 5 and
    all(to_entries[]; .value == true or .value == false)
  ' "$destination" >/dev/null 2>&1
}

if ! collect_snapshot before "$WORK_DIR/before.json"; then
  fail metadata_failed 'could not collect the pre-migration metadata snapshot'
fi

has_before_migration() {
  jq -e --arg filename "$1" '.schema_migrations | any(.filename == $filename)' \
    "$WORK_DIR/before.json" >/dev/null
}

core_snapshot_present=0
if jq -e '
  . as $snapshot |
  ($snapshot.schema_migrations | length > 0) and
  (["users", "groups", "api_keys", "accounts", "usage_logs"] |
    all(.[]; . as $table | $snapshot.key_table_counts[$table].exists == true))
' "$WORK_DIR/before.json" >/dev/null 2>&1; then
  core_snapshot_present=1
fi

legacy_evidence_present=0
if jq -e -n \
  --slurpfile manifest "$WORK_DIR/legacy-manifest.json" \
  --slurpfile snapshot "$WORK_DIR/before.json" '
    ($manifest[0] | map(.filename)) as $legacy_names |
    $snapshot[0].schema_migrations |
    any(.[]; . as $migration | ($legacy_names | index($migration.filename)) != null)
  ' >/dev/null; then
  legacy_evidence_present=1
fi
legacy_manifest_verified=true
if [[ "$SOURCE_KIND" == 'legacy-modelport' || "$legacy_evidence_present" -eq 1 ]]; then
  if ! validate_legacy_archive_and_ledger; then
    legacy_manifest_verified=false
    fail source_mismatch 'the legacy ModelPort migration ledger does not match the authoritative archive manifest'
  fi
fi

case "$SOURCE_KIND" in
  empty)
    DETECTED_LINEAGE='empty'
    ;;
  upstream)
    if [[ "$core_snapshot_present" -ne 1 ]] || \
      [[ "$legacy_evidence_present" -ne 0 ]] || \
      ! has_before_migration '230_plugin_artifacts.sql' || \
      has_before_migration '232_modelport_free_group_bridge.sql' || \
      has_before_migration '192_add_free_group_billing.sql' || \
      has_before_migration '224_prompt_audit_instruction_patch.sql'; then
      fail source_mismatch 'the restored snapshot does not match the upstream fingerprint'
    fi
    DETECTED_LINEAGE='upstream-v0.1.183'
    ;;
  legacy-modelport)
    if [[ "$core_snapshot_present" -ne 1 ]] || \
      [[ "$legacy_evidence_present" -ne 1 ]] || \
      has_before_migration '232_modelport_free_group_bridge.sql'; then
      fail source_mismatch 'the restored snapshot does not match the legacy ModelPort fingerprint'
    fi
    DETECTED_LINEAGE='modelport-custom-v0.1.176.2'
    ;;
  production)
    if [[ "$core_snapshot_present" -ne 1 ]]; then
      fail source_mismatch 'the restored production snapshot lacks the required migration ledger or core tables'
    fi
    if has_before_migration '232_modelport_free_group_bridge.sql'; then
      DETECTED_LINEAGE='modelport-current'
    elif [[ "$legacy_evidence_present" -eq 1 ]]; then
      DETECTED_LINEAGE='modelport-legacy'
    elif has_before_migration '230_plugin_artifacts.sql'; then
      DETECTED_LINEAGE='upstream'
    else
      fail source_mismatch 'the production snapshot has no supported upstream or ModelPort lineage fingerprint'
    fi
    ;;
esac

server_version="$("${psql_exec[@]}" -c 'SHOW server_version' 2>"$WORK_DIR/server-version.log")" || \
  fail metadata_failed 'could not record the PostgreSQL server version'
if [[ ! "$server_version" =~ ^([0-9]+)(\..*)?$ ]]; then
  fail metadata_failed 'PostgreSQL returned an invalid server version'
fi
postgres_server_major="${BASH_REMATCH[1]}"
if [[ -n "$EXPECTED_POSTGRES_MAJOR" && "$postgres_server_major" != "$EXPECTED_POSTGRES_MAJOR" ]]; then
  fail postgres_major_mismatch 'the isolated PostgreSQL server major does not match --expected-postgres-major'
fi
psql_version="$(docker exec --user '70:70' "$PG_CONTAINER" psql --version 2>"$WORK_DIR/psql-version.log")" || \
  fail metadata_failed 'could not record the psql version'
pg_restore_version="$(docker exec --user '70:70' "$PG_CONTAINER" pg_restore --version 2>"$WORK_DIR/pg-restore-version.log")" || \
  fail metadata_failed 'could not record the pg_restore version'
go_version="$(go version 2>"$WORK_DIR/go-version.log")" || fail metadata_failed 'could not record the Go version'
repository_commit="$repository_commit_start"

migration_binary_host="$WORK_DIR/modelport-restore-migrate"
migration_binary_container='/tmp/modelport-restore-migrate'
build_environment=(
  "CGO_ENABLED=0"
  "GOOS=$postgres_image_os"
  "GOARCH=$postgres_image_architecture"
)
if [[ "$SOURCE_KIND" == 'production' ]]; then
  build_environment+=(
    'GOTOOLCHAIN=local'
    'GOPROXY=off'
    'GOSUMDB=off'
    'GOWORK=off'
  )
fi
if ! (
  cd "$SOURCE_ROOT/backend"
  env "${build_environment[@]}" \
    go build -mod=readonly -trimpath -o "$migration_binary_host" ./cmd/modelport-restore-migrate
) >"$WORK_DIR/migration-build.log" 2>&1; then
  fail migration_failed 'could not build the isolated migration command for the pinned PostgreSQL image'
fi
chmod 0500 "$migration_binary_host" || fail migration_failed 'could not protect the isolated migration binary'
MIGRATION_BINARY_SHA256="$(sha256_file "$migration_binary_host")" || \
  fail migration_failed 'could not hash the isolated migration binary'
[[ "$MIGRATION_BINARY_SHA256" =~ ^[0-9a-f]{64}$ ]] || \
  fail migration_failed 'the isolated migration binary SHA-256 was invalid'
if ! docker exec --user '70:70' -i "$PG_CONTAINER" \
  sh -c 'umask 077; cat > /tmp/modelport-restore-migrate && chmod 0500 /tmp/modelport-restore-migrate' \
  < "$migration_binary_host" >"$WORK_DIR/migration-copy.log" 2>&1; then
  fail migration_failed 'could not copy the isolated migration binary into the PostgreSQL container'
fi
container_migration_sha256="$(docker exec --user '70:70' "$PG_CONTAINER" \
  sha256sum "$migration_binary_container" 2>"$WORK_DIR/migration-container-hash.log" | awk '{print $1}')" || \
  fail migration_failed 'could not hash the isolated migration binary inside the PostgreSQL container'
if [[ "$container_migration_sha256" != "$MIGRATION_BINARY_SHA256" ]]; then
  fail migration_failed 'the isolated migration binary changed while being copied into the PostgreSQL container'
fi

jq -n \
  --arg postgres_image_ref "$POSTGRES_IMAGE" \
  --arg postgres_image_id "$postgres_image_id" \
  --arg postgres_server_version "$server_version" \
  --arg postgres_server_major "$postgres_server_major" \
  --arg expected_postgres_major "$EXPECTED_POSTGRES_MAJOR" \
  --arg psql_version "$psql_version" \
  --arg pg_restore_version "$pg_restore_version" \
  --arg go_version "$go_version" \
    --arg migration_binary_sha256 "$MIGRATION_BINARY_SHA256" \
    --arg migration_source "$MIGRATION_SOURCE" \
  --arg repository_commit "$repository_commit" \
  --arg repository_tree "$repository_tree_start" \
  --arg expected_revision "$EXPECTED_REVISION" \
  --argjson repository_dirty_start "$repository_dirty_start" \
  --argjson docker_available_kib "$docker_available_kib" \
  --argjson production_storage_confirmed "$PRODUCTION_STORAGE_CONFIRMED" \
  --argjson legacy_manifest_verified "$legacy_manifest_verified" \
  '{
    postgres_image: {reference: $postgres_image_ref, id: $postgres_image_id},
    postgres_server_version: $postgres_server_version,
    postgres_server_major: ($postgres_server_major | tonumber),
    expected_postgres_major: (if $expected_postgres_major == "" then null else ($expected_postgres_major | tonumber) end),
    psql_version: $psql_version,
    pg_restore_version: $pg_restore_version,
    go_version: $go_version,
    migration_tool: {
      command: "modelport-restore-migrate",
      binary_sha256: $migration_binary_sha256,
      execution: "postgres-container-loopback",
      source: $migration_source
    },
    repository: {
      commit: $repository_commit,
      tree_sha: $repository_tree,
      expected_revision: (if $expected_revision == "" then null else $expected_revision end),
      dirty_at_start: $repository_dirty_start,
      dirty_at_end: null,
      unchanged_during_run: null
    },
    migration_runs: 0,
    legacy_manifest_verified: $legacy_manifest_verified,
    docker_volume_available_kib_before_restore: $docker_available_kib,
    isolation: {
      docker_internal_network: true,
      database_host: "127.0.0.1",
      database_port: 5432,
      host_port_published: false,
      migration_runs_inside_postgres_container: true,
      temporary_database_name_enforced: true,
      restore_role_superuser: false,
      source_dump_mounted: false,
      production_storage_confirmed: ($production_storage_confirmed == 1)
    }
  }' > "$WORK_DIR/runtime.json" || fail metadata_failed 'could not build runtime metadata'

restore_database_url="postgresql://$restore_user:$restore_password@127.0.0.1:5432/$database_name?sslmode=disable"
run_application_migrations() {
  local diagnostics="$1"
  printf '%s\n' "$restore_database_url" |
    docker exec --user '70:70' -i "$PG_CONTAINER" sh -c '
      IFS= read -r MODELPORT_RESTORE_DATABASE_URL
      export MODELPORT_RESTORE_DATABASE_URL
      exec /tmp/modelport-restore-migrate
    ' >"$diagnostics" 2>&1
}

if ! run_application_migrations "$WORK_DIR/migration-first.log"; then
  fail migration_failed 'the application migration entry rejected or failed on the isolated snapshot'
fi
MIGRATION_RUNS=1
if ! jq --argjson runs "$MIGRATION_RUNS" '.migration_runs = $runs' \
  "$WORK_DIR/runtime.json" > "$WORK_DIR/runtime-runs.json"; then
  fail metadata_failed 'could not record the first migration run'
fi
mv "$WORK_DIR/runtime-runs.json" "$WORK_DIR/runtime.json"

if ! collect_snapshot after-first "$WORK_DIR/after-first.json"; then
  fail metadata_failed 'could not collect the post-migration metadata snapshot'
fi
if ! collect_bridge_contract after-first "$WORK_DIR/bridge-after-first.json"; then
  fail invariant_failed 'could not prove the required bridge schema after the first migration run'
fi
if ! jq --slurpfile contract "$WORK_DIR/bridge-after-first.json" \
  '.bridge_contract = $contract[0]' \
  "$WORK_DIR/after-first.json" > "$WORK_DIR/after-first-contract.json"; then
  fail metadata_failed 'could not attach the first bridge contract result'
fi
mv "$WORK_DIR/after-first-contract.json" "$WORK_DIR/after-first.json"

if ! run_application_migrations "$WORK_DIR/migration-second.log"; then
  fail migration_idempotency_failed 'the second application migration run failed'
fi
MIGRATION_RUNS=2
if ! jq --argjson runs "$MIGRATION_RUNS" '.migration_runs = $runs' \
  "$WORK_DIR/runtime.json" > "$WORK_DIR/runtime-runs.json"; then
  fail metadata_failed 'could not record the second migration run'
fi
mv "$WORK_DIR/runtime-runs.json" "$WORK_DIR/runtime.json"

if ! collect_snapshot after "$WORK_DIR/after.json"; then
  fail metadata_failed 'could not collect the second post-migration metadata snapshot'
fi
if ! collect_bridge_contract after "$WORK_DIR/bridge-after.json"; then
  fail invariant_failed 'could not prove the required bridge schema after the second migration run'
fi
if ! jq --slurpfile contract "$WORK_DIR/bridge-after.json" \
  '.bridge_contract = $contract[0]' \
  "$WORK_DIR/after.json" > "$WORK_DIR/after-contract.json"; then
  fail metadata_failed 'could not attach the second bridge contract result'
fi
mv "$WORK_DIR/after-contract.json" "$WORK_DIR/after.json"

input_stable=true
verified_copy_stable=true
if [[ "$SOURCE_KIND" != 'empty' ]]; then
  open_dump_read || fail input_changed 'the input backup identity changed during the drill'
  final_sha256="$(sha256_file "$DUMP_FD_PATH")" || fail input_changed 'could not re-hash the input backup'
  open_dump_read || fail input_changed 'the input backup identity changed during the drill'
  final_size="$(wc -c < "$DUMP_FD_PATH" | tr -d '[:space:]')"
  if [[ "$final_sha256" != "$INPUT_SHA256" || "$final_size" != "$INPUT_SIZE" ]]; then
    input_stable=false
  fi
  final_verified_sha256="$(sha256_file "$VERIFIED_DUMP_PATH")" || \
    fail input_changed 'could not re-hash the stable restore input'
  final_verified_size="$(wc -c < "$VERIFIED_DUMP_PATH" | tr -d '[:space:]')"
  if [[ "$final_verified_sha256" != "$VERIFIED_DUMP_SHA256" || "$final_verified_size" != "$INPUT_SIZE" ]]; then
    verified_copy_stable=false
  fi
fi

repository_commit_end="$(git -C "$REPO_ROOT" rev-parse HEAD 2>"$WORK_DIR/git-end.log")" || \
  fail repository_changed 'could not re-check the repository HEAD'
repository_tree_end="$(git -C "$REPO_ROOT" rev-parse 'HEAD^{tree}' 2>>"$WORK_DIR/git-end.log")" || \
  fail repository_changed 'could not re-check the repository tree'
repository_dirty_end=false
if [[ -n "$(git -C "$REPO_ROOT" status --porcelain=v1 --untracked-files=normal 2>>"$WORK_DIR/git-end.log")" ]]; then
  repository_dirty_end=true
fi
repository_unchanged=true
if [[ "$repository_commit_end" != "$repository_commit_start" || \
      "$repository_tree_end" != "$repository_tree_start" ]]; then
  repository_unchanged=false
fi
if [[ "$SOURCE_KIND" == 'production' && "$repository_dirty_end" == true ]]; then
  repository_unchanged=false
fi
if ! jq \
  --argjson dirty_at_end "$repository_dirty_end" \
  --argjson unchanged "$repository_unchanged" \
  '.repository.dirty_at_end = $dirty_at_end |
   .repository.unchanged_during_run = $unchanged' \
  "$WORK_DIR/runtime.json" > "$WORK_DIR/runtime-final.json"; then
  fail metadata_failed 'could not finalize repository evidence'
fi
mv "$WORK_DIR/runtime-final.json" "$WORK_DIR/runtime.json"
if [[ "$SOURCE_KIND" == 'production' && "$repository_unchanged" != true ]]; then
  fail repository_changed 'the repository revision or cleanliness changed during the production drill'
fi

required_migrations=true
for required_migration in \
  232_modelport_free_group_bridge.sql \
  233_modelport_lottery_bridge.sql \
  234_modelport_instruction_audit_bridge.sql \
  235_batch_image_group_snapshot.sql \
  236_modelport_legacy_platform_constraints.sql; do
  if ! jq -e --arg filename "$required_migration" \
    '.schema_migrations | any(.filename == $filename)' "$WORK_DIR/after.json" >/dev/null; then
    required_migrations=false
  fi
done

bridge_contract_valid=true
if ! jq -e '.bridge_contract | all(.[]; . == true)' \
  "$WORK_DIR/after.json" >/dev/null; then
  bridge_contract_valid=false
fi

required_bridge_objects_present=true
if ! jq -e -n \
  --slurpfile tables "$WORK_DIR/required-bridge-tables.json" \
  --slurpfile constraints "$WORK_DIR/required-bridge-constraints.json" \
  --slurpfile indexes "$WORK_DIR/required-bridge-indexes.json" \
  --slurpfile after "$WORK_DIR/after.json" '
    ($after[0].all_table_counts | keys) as $present_tables |
    ($tables[0] | all(.[]; . as $name | ($present_tables | index($name)) != null)) and
    ($constraints[0] | all(.[]; . as $name |
      $after[0].constraints | any(.name == $name and .validated == true))) and
    ($indexes[0] | all(.[]; . as $name |
      $after[0].indexes | any(.name == $name and .valid == true and .ready == true)))
  ' >/dev/null; then
  required_bridge_objects_present=false
fi

ledger_monotonic=true
if [[ "$SOURCE_KIND" != 'empty' ]] && ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after.json" '
    ($after[0].schema_migrations | map({key: .filename, value: .checksum}) | from_entries) as $post |
    $before[0].schema_migrations |
    all(.[]; $post[.filename] == .checksum)
  ' >/dev/null; then
  ledger_monotonic=false
fi

audit_seed_rows_valid=true
if ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after-first.json" '
    def seed_summary($snapshot; $name):
      (((($snapshot.audit_seed_summaries // {})[$name] // {})
        | {table_exists: (.table_exists // false), rows: (.rows // [])}));
    def seed_rows($snapshot; $name): (seed_summary($snapshot; $name).rows);
    def seed_exists($snapshot; $name): (seed_summary($snapshot; $name).table_exists);
    def unique_row_keys($rows):
      ($rows | map(.key)) as $keys |
      ($keys | length) == ($keys | unique | length);
    def row_shape_valid($rows):
      all($rows[];
        (.key | type) == "string" and
        (.fingerprint | type) == "string" and
        (.fingerprint | length) > 0 and
        (.canonical | type) == "boolean"
      );
    def row_preserved($pre_rows; $post_rows):
      all($pre_rows[]; . as $pre_row |
        $post_rows | any(
          .key == $pre_row.key and
          .fingerprint == $pre_row.fingerprint and
          .canonical == $pre_row.canonical
        )
      );
    def fixed_seed_ok($pre_snapshot; $post_snapshot; $name; $expected):
      seed_summary($pre_snapshot; $name) as $pre |
      seed_summary($post_snapshot; $name) as $post |
      ($pre.rows | map(.key)) as $pre_keys |
      ($post.rows | map(.key)) as $post_keys |
      ($post.rows | map(
        . as $row | select((($pre_keys | index($row.key)) == null))
      )) as $new_rows |
      ((seed_exists($pre_snapshot; $name) | not) or
       seed_exists($post_snapshot; $name)) and
      row_shape_valid($pre.rows) and row_shape_valid($post.rows) and
      unique_row_keys($pre.rows) and unique_row_keys($post.rows) and
      row_preserved($pre.rows; $post.rows) and
      ($expected | all(.[]; . as $key |
        $post.rows | any(.key == $key and .canonical == true))) and
      ($post.rows | all(.[]; . as $row |
        (($pre_keys | index($row.key)) != null) or
        ($row.canonical == true and ($expected | index($row.key)) != null)
      )) and
      (($post.rows | map(.key) | sort) |
        all(.[]; . as $key |
          (($pre_keys | index($key)) != null) or
          (($expected | index($key)) != null))) and
      (($post.rows | length) - ($pre.rows | length)) == ($new_rows | length);
    def grant_seed_ok($pre_snapshot; $post_snapshot):
      seed_summary($pre_snapshot; "instruction_audit_sensitive_access_grants") as $pre |
      seed_summary($post_snapshot; "instruction_audit_sensitive_access_grants") as $post |
      ($pre.rows | map(.key)) as $pre_keys |
      ($post.rows | map(
        . as $row | select((($pre_keys | index($row.key)) == null))
      )) as $new_rows |
      ((seed_exists($pre_snapshot; "instruction_audit_sensitive_access_grants") | not) or
       seed_exists($post_snapshot; "instruction_audit_sensitive_access_grants")) and
      row_shape_valid($pre.rows) and row_shape_valid($post.rows) and
      unique_row_keys($pre.rows) and unique_row_keys($post.rows) and
      row_preserved($pre.rows; $post.rows) and
      (($post.rows | length) - ($pre.rows | length)) == ($new_rows | length) and
      (($pre.rows | length) > 0 or ($new_rows | length) <= 1) and
      ($new_rows | all(.[]; .canonical == true));
    {
      instruction_audit_state: ["1"],
      instruction_audit_reason_policies: [
        "hash_mismatch", "fields_missing", "field_invalid", "invalid_json",
        "request_too_large", "structure_too_complex", "parse_timeout",
        "config_unavailable", "group_not_allowed", "client_not_allowed",
        "ai_rejected", "ai_uncertain", "ai_error"
      ],
      instruction_audit_runtime_config: ["1"],
      instruction_audit_outcome_rollup_state: ["1"],
      instruction_audit_operational_counters: ["1"],
      instruction_audit_v2_config: ["1"],
      instruction_audit_v2_client_profiles: [
        "codex_vscode", "codex_cli", "codex_desktop", "opencode",
        "modelport_internal", "other", "unknown"
      ]
    } as $expected |
    ($before[0]) as $pre_snapshot |
    ($after[0]) as $post_snapshot |
    ($expected | to_entries | all(.[];
      fixed_seed_ok($pre_snapshot; $post_snapshot; .key; .value)
    )) and
    grant_seed_ok($pre_snapshot; $post_snapshot)
  ' >/dev/null; then
  audit_seed_rows_valid=false
fi

all_table_counts_preserved=true
if ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after-first.json" '
    def seed_summary($snapshot; $name):
      (((($snapshot.audit_seed_summaries // {})[$name] // {})
        | {table_exists: (.table_exists // false), rows: (.rows // [])}));
    def seed_rows($snapshot; $name): (seed_summary($snapshot; $name).rows);
    def seed_names: [
      "instruction_audit_state",
      "instruction_audit_reason_policies",
      "instruction_audit_runtime_config",
      "instruction_audit_outcome_rollup_state",
      "instruction_audit_operational_counters",
      "instruction_audit_sensitive_access_grants",
      "instruction_audit_v2_config",
      "instruction_audit_v2_client_profiles"
    ];
    def is_audit_table:
      startswith("instruction_audit_") or
      . == "security_notification_outbox" or
      . == "content_moderation_cyber_evidence";
    def new_seed_count($pre_snapshot; $post_snapshot; $name):
      (seed_rows($pre_snapshot; $name) | map(.key)) as $pre_keys |
      (seed_rows($post_snapshot; $name) |
        map(. as $row | select((($pre_keys | index($row.key)) == null))) | length);
    $before[0] as $pre_snapshot |
    $after[0] as $post_snapshot |
    ($pre_snapshot.all_table_counts // {}) as $pre |
    ($post_snapshot.all_table_counts // {}) as $post |
    (seed_names) as $seed_names |
    # Every seed summary must agree with the corresponding physical table
    # count. A missing table is represented by an absent count and zero rows.
    ($seed_names | all(.[]; . as $name |
      (seed_rows($pre_snapshot; $name) | length) ==
        (if ($pre | has($name)) then $pre[$name] else 0 end) and
      (seed_rows($post_snapshot; $name) | length) ==
        (if ($post | has($name)) then $post[$name] else 0 end)
    )) and
    (($pre | keys) | all(.[]; . as $name |
      if $name == "settings" then true
      elif ($post | has($name)) | not then false
      elif ($seed_names | index($name)) != null then
        ($post[$name] - $pre[$name]) ==
          new_seed_count($pre_snapshot; $post_snapshot; $name)
      else $post[$name] == $pre[$name]
      end
    )) and
    ((($pre | keys) + ($post | keys) | unique) | all(.[]; . as $name |
      if ($pre | has($name)) then true
      elif $name == "settings" then true
      elif ($seed_names | index($name)) != null then
        $post[$name] == new_seed_count($pre_snapshot; $post_snapshot; $name)
      elif ($name | is_audit_table) then $post[$name] == 0
      else true
      end
    ))
  ' >/dev/null; then
  all_table_counts_preserved=false
fi
if [[ "$audit_seed_rows_valid" != true ]]; then
  # A count delta is only meaningful when every permitted 234 seed row has
  # been identified and its pre-existing fingerprint was preserved.
  all_table_counts_preserved=false
fi

hash_false="$(sha256_text false)"
hash_zero="$(sha256_text 0)"
hash_off="$(sha256_text off)"
hash_disabled="$(sha256_text disabled)"
hash_true="$(sha256_text true)"
hash_v1="$(sha256_text v1)"
hash_30="$(sha256_text 30)"
if ! jq -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --arg hash_false "$hash_false" \
  --arg hash_zero "$hash_zero" \
  --arg hash_off "$hash_off" \
  --arg hash_disabled "$hash_disabled" \
  --arg hash_true "$hash_true" \
  --arg hash_v1 "$hash_v1" \
  --arg hash_30 "$hash_30" '
    $before[0] as $snapshot |
    ($snapshot.schema_migrations | map(.filename)) as $ledger |
    [$hash_false, $hash_zero, $hash_off, $hash_disabled] as $disabled_hashes |
    $snapshot.settings_value_summaries |
    if (($ledger | index("195_channel_monitor_mode.sql")) == null and
        (has("channel_monitor_mode") | not))
      then .["channel_monitor_mode"] = $hash_v1 else . end |
    if (($ledger | index("204_channel_monitor_hide_throughput.sql")) == null and
        (has("channel_monitor_hide_throughput") | not))
      then .["channel_monitor_hide_throughput"] = $hash_false else . end |
    . as $settings_after_204 |
    if (($ledger | index("206_channel_monitor_v2_privacy_defaults.sql")) == null and
        ($settings_after_204 | has("channel_monitor_hide_throughput")) and
        (($disabled_hashes | index($settings_after_204["channel_monitor_hide_throughput"])) != null))
      then .["channel_monitor_hide_throughput"] = $hash_true else . end |
    if (($ledger | index("226_channel_monitor_quota_mode.sql")) == null and
        (has("channel_monitor_show_quota") | not))
      then .["channel_monitor_show_quota"] = $hash_false else . end |
    if (($ledger | index("234_modelport_instruction_audit_bridge.sql")) == null and
        (has("instruction_audit_enabled") | not))
      then .["instruction_audit_enabled"] = $hash_false else . end |
    if (($ledger | index("234_modelport_instruction_audit_bridge.sql")) == null and
        (has("instruction_audit_evidence_retention_days") | not))
      then .["instruction_audit_evidence_retention_days"] = $hash_30 else . end
  ' > "$WORK_DIR/expected-settings.json"; then
  fail metadata_failed 'could not derive the migration-owned settings transition'
fi

settings_transition_valid=true
if ! jq -e -n \
  --arg source_kind "$SOURCE_KIND" \
  --slurpfile expected "$WORK_DIR/expected-settings.json" \
  --slurpfile after "$WORK_DIR/after-first.json" '
    ($after[0].settings_value_summaries as $actual |
      (if $source_kind == "empty" then
        ($expected[0] | to_entries | all(.[]; $actual[.key] == .value))
      else
        $actual == $expected[0]
      end) and
      ($after[0].all_table_counts.settings == ($actual | length)))
  ' >/dev/null; then
  settings_transition_valid=false
fi

financial_preserved=true
if [[ "$SOURCE_KIND" != 'empty' ]] && ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after.json" '
    $before[0].financial_aggregates as $pre |
    $after[0].financial_aggregates as $post |
    ($pre | to_entries | all(.[];
      if (.value.table_exists and .value.column_exists) then
        $post[.key] == .value
      else true end
    ))
  ' >/dev/null; then
  financial_preserved=false
fi

ciphertext_preserved=true
if [[ "$SOURCE_KIND" != 'empty' ]] && ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after.json" '
    $before[0].ciphertext_summaries as $pre |
    $after[0].ciphertext_summaries as $post |
    ($pre | to_entries | all(.[]; $post[.key] == .value))
  ' >/dev/null; then
  ciphertext_preserved=false
fi

second_migration_stable=true
if ! jq -e -n \
  --slurpfile first "$WORK_DIR/after-first.json" \
  --slurpfile second "$WORK_DIR/after.json" '
    ($first[0].schema_migrations == $second[0].schema_migrations) and
    ($first[0].all_table_counts == $second[0].all_table_counts) and
    ($first[0].key_table_counts == $second[0].key_table_counts) and
    ($first[0].financial_aggregates == $second[0].financial_aggregates) and
    ($first[0].settings_value_summaries == $second[0].settings_value_summaries) and
    ($first[0].audit_seed_summaries == $second[0].audit_seed_summaries) and
    ($first[0].ciphertext_summaries == $second[0].ciphertext_summaries) and
    ($first[0].bridge_contract == $second[0].bridge_contract) and
    ($first[0].sequences == $second[0].sequences) and
    ($first[0].constraints == $second[0].constraints) and
    ($first[0].indexes == $second[0].indexes) and
    ($first[0].foreign_keys == $second[0].foreign_keys)
  ' >/dev/null; then
  second_migration_stable=false
fi

sequences_valid=true
if ! jq -e '.sequences | all(.[]; .position_valid == true)' "$WORK_DIR/after.json" >/dev/null; then
  sequences_valid=false
fi

catalog_valid=true
if ! jq -e '
  def intentionally_not_valid:
    .table == "usage_logs" and .type == "c" and
    (.name == "usage_logs_image_billing_size_check" or
     .name == "usage_logs_image_size_source_check");
  (.constraints | all(.[]; .validated == true or intentionally_not_valid)) and
  (.indexes | all(.[]; .valid == true and .ready == true)) and
  (.foreign_keys | all(.[]; .validated == true))
' "$WORK_DIR/after.json" >/dev/null; then
  catalog_valid=false
fi

catalog_objects_preserved=true
if [[ "$SOURCE_KIND" != 'empty' ]] && ! jq -e -n \
  --slurpfile before "$WORK_DIR/before.json" \
  --slurpfile after "$WORK_DIR/after.json" '
    ($after[0].constraints |
      map({key: (.table + "\u0000" + .name), value: .}) | from_entries) as $post_constraints |
    ($after[0].indexes |
      map({key: (.table + "\u0000" + .name), value: .}) | from_entries) as $post_indexes |
    ($after[0].foreign_keys |
      map({key: (.table + "\u0000" + .name), value: .}) | from_entries) as $post_foreign_keys |
    ($after[0].sequences |
      map({key: (.schema + "\u0000" + .name), value: .}) | from_entries) as $post_sequences |
    ($before[0].schema_migrations |
      any(.filename == "236_modelport_legacy_platform_constraints.sql") | not) as $platform_bridge_pending |
    [
      "user_platform_quotas_platform_check",
      "composite_model_routes_target_platform_check",
      "channel_monitors_provider_check",
      "channel_monitor_request_templates_provider_check"
    ] as $platform_bridge_constraints |
    ($before[0].constraints | all(.[];
      . as $pre |
      $post_constraints[$pre.table + "\u0000" + $pre.name] as $post |
      $post != null and
      $post.type == $pre.type and
      $post.validated == $pre.validated and
      ($post.definition_sha256 == $pre.definition_sha256 or
       ($platform_bridge_pending and ($platform_bridge_constraints | index($pre.name)) != null))
    )) and
    ($before[0].indexes | all(.[];
      . as $pre |
      $post_indexes[$pre.table + "\u0000" + $pre.name] as $post |
      $post != null and
      [$post.unique, $post.primary, $post.valid, $post.ready, $post.definition_sha256] ==
      [$pre.unique, $pre.primary, $pre.valid, $pre.ready, $pre.definition_sha256]
    )) and
    ($before[0].foreign_keys | all(.[];
      . as $pre |
      $post_foreign_keys[$pre.table + "\u0000" + $pre.name] as $post |
      $post != null and
      [$post.referenced_table, $post.validated, $post.definition_sha256] ==
      [$pre.referenced_table, $pre.validated, $pre.definition_sha256]
    )) and
    ($before[0].sequences | all(.[];
      . as $pre |
      $post_sequences[$pre.schema + "\u0000" + $pre.name] as $post |
      $post != null and
      [$post.start, $post.increment, $post.min, $post.max, $post.cache, $post.cycle] ==
      [$pre.start, $pre.increment, $pre.min, $pre.max, $pre.cache, $pre.cycle]
    ))
  ' >/dev/null; then
  catalog_objects_preserved=false
fi

core_tables_present=true
if ! jq -e '
  . as $snapshot |
  ["users", "groups", "api_keys", "accounts", "usage_logs"] |
  all(.[]; . as $table | $snapshot.key_table_counts[$table].exists == true)
' "$WORK_DIR/after.json" >/dev/null 2>&1; then
  core_tables_present=false
fi

jq -n \
  --argjson input_stable "$input_stable" \
  --argjson verified_copy_stable "$verified_copy_stable" \
  --argjson required_migrations "$required_migrations" \
  --argjson bridge_contract_valid "$bridge_contract_valid" \
  --argjson required_bridge_objects_present "$required_bridge_objects_present" \
  --argjson ledger_monotonic "$ledger_monotonic" \
  --argjson audit_seed_rows_valid "$audit_seed_rows_valid" \
  --argjson all_table_counts_preserved "$all_table_counts_preserved" \
  --argjson settings_transition_valid "$settings_transition_valid" \
  --argjson financial_preserved "$financial_preserved" \
  --argjson ciphertext_preserved "$ciphertext_preserved" \
  --argjson second_migration_stable "$second_migration_stable" \
  --argjson sequences_valid "$sequences_valid" \
  --argjson catalog_valid "$catalog_valid" \
  --argjson catalog_objects_preserved "$catalog_objects_preserved" \
  --argjson core_tables_present "$core_tables_present" '
    [
      {name: "input_backup_unchanged", passed: $input_stable},
      {name: "verified_restore_copy_unchanged", passed: $verified_copy_stable},
      {name: "required_migrations_applied", passed: $required_migrations},
      {name: "required_bridge_contract_valid", passed: $bridge_contract_valid},
      {name: "required_bridge_objects_present", passed: $required_bridge_objects_present},
      {name: "migration_ledger_preserved", passed: $ledger_monotonic},
      {name: "modelport_instruction_audit_seed_rows_legal", passed: $audit_seed_rows_valid},
      {name: "all_preexisting_table_counts_preserved", passed: $all_table_counts_preserved},
      {name: "migration_owned_settings_transition_valid", passed: $settings_transition_valid},
      {name: "financial_aggregates_preserved", passed: $financial_preserved},
      {name: "ciphertext_byte_summaries_preserved", passed: $ciphertext_preserved},
      {name: "second_migration_run_data_stable", passed: $second_migration_stable},
      {name: "owned_sequence_positions_valid", passed: $sequences_valid},
      {name: "constraints_indexes_foreign_keys_valid", passed: $catalog_valid},
      {name: "preexisting_catalog_objects_preserved", passed: $catalog_objects_preserved},
      {name: "core_tables_present", passed: $core_tables_present}
    ]
  ' > "$WORK_DIR/invariants.json" || fail metadata_failed 'could not build invariant results'

if [[ "$input_stable" != true ]]; then
  fail input_changed 'the source backup changed while the drill was running'
fi
if [[ "$verified_copy_stable" != true ]]; then
  fail input_changed 'the stable restore input changed while the drill was running'
fi
if [[ "$required_migrations" != true || "$bridge_contract_valid" != true || \
      "$required_bridge_objects_present" != true || "$ledger_monotonic" != true || \
      "$all_table_counts_preserved" != true || "$settings_transition_valid" != true || \
      "$financial_preserved" != true || "$ciphertext_preserved" != true || \
      "$second_migration_stable" != true || "$sequences_valid" != true || \
      "$catalog_valid" != true || "$catalog_objects_preserved" != true || \
      "$core_tables_present" != true ]]; then
  fail invariant_failed 'one or more post-migration invariants failed'
fi

if ! cleanup_docker; then
  fail cleanup_failed 'one or more exact-name Docker resources could not be removed'
fi
if ! jq '.isolation.cleanup_completed = true' \
  "$WORK_DIR/runtime.json" > "$WORK_DIR/runtime-clean.json"; then
  fail metadata_failed 'could not record successful isolated resource cleanup'
fi
mv "$WORK_DIR/runtime-clean.json" "$WORK_DIR/runtime.json"

FAILURE_CODE='report_write_failed'
if ! stage_report passed; then
  fail report_write_failed 'could not stage the structured report'
fi
if ! cleanup_work_dir; then
  fail cleanup_failed 'could not remove the local sensitive work directory before publishing success'
fi
if ! publish_staged_report; then
  fail report_write_failed 'could not atomically publish the structured report'
fi

printf 'ModelPort isolated restore passed; report: %s\n' "$REPORT_PATH"
