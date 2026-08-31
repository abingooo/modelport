#!/usr/bin/env bash
set -Eeuo pipefail

umask 077
export LC_ALL=C

repo_root="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/deploy/modelport-isolated-redis-restore.sh"
test_root="$(mktemp -d /tmp/modelport-isolated-redis-restore-test.XXXXXX)"
bin_dir="$test_root/bin"
state_dir="$test_root/state"
external_dir="$test_root/encrypted-external"
docker_log="$test_root/docker.log"
stdout_log="$test_root/stdout.log"
stderr_log="$test_root/stderr.log"
report_swap_link=''
report_swap_redirect=''
input_mutate_path=''
real_generator_container=''
real_generator_volume=''
real_generator_created=0
real_generator_volume_created=0

readonly image_ref='registry.example.invalid/redis@sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576'
readonly image_id='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
readonly rpo_at='2026-08-28T00:00:00Z'
readonly fixture_secret='MODELPORT_REDIS_SECRET_KEY_VALUE_DO_NOT_LEAK_92f18d'
docker_socket="$test_root/docker.sock"

cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if [[ "$real_generator_created" -eq 1 ]]; then
    if ! docker rm --force "$real_generator_container" >/dev/null 2>&1; then
      status=1
    fi
    real_generator_created=0
  fi
  if [[ "$real_generator_volume_created" -eq 1 ]]; then
    if ! docker volume rm "$real_generator_volume" >/dev/null 2>&1; then
      status=1
    fi
    real_generator_volume_created=0
  fi
  if [[ "${MODELPORT_TEST_KEEP:-0}" == '1' ]]; then
    printf 'Preserved test directory: %s\n' "$test_root" >&2
    exit "$status"
  fi
  rm -rf -- "$test_root"
  exit "$status"
}
trap cleanup EXIT

fail() {
  printf 'ModelPort isolated Redis restore contract test failed: %s\n' "$1" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

file_mode() {
  local value=''
  if value="$(stat -f '%Lp' "$1" 2>/dev/null)"; then
    printf '%s' "$value"
  else
    stat -c '%a' "$1"
  fi
}

assert_contains() {
  local needle="$1"
  local file="$2"
  grep -Fq -- "$needle" "$file" || fail "expected $file to contain: $needle"
}

assert_not_contains() {
  local needle="$1"
  local file="$2"
  if grep -Fq -- "$needle" "$file"; then
    fail "expected $file not to contain: $needle"
  fi
}

assert_no_owned_resources() {
  if find "$state_dir" -maxdepth 1 \
    \( -name 'container-*' -o -name 'network-*' -o -name 'volume-*' -o -name 'volume-data-*' \) | grep -q .; then
    find "$state_dir" -maxdepth 1 >&2
    fail 'an owned mock Docker resource remained after the run'
  fi
}

reset_run() {
  rm -f -- "$docker_log" "$stdout_log" "$stderr_log" \
    "$state_dir/docker-copy-source" "$state_dir/restore-input-sha"
  find "$state_dir" -mindepth 1 -maxdepth 1 \
    \( -name 'container-*' -o -name 'network-*' -o -name 'volume-*' -o -name 'volume-data-*' \) \
    -exec rm -rf -- {} +
}

mkdir -p "$bin_dir" "$state_dir" "$external_dir"
python3 - "$docker_socket" <<'PY'
import socket
import sys

sock = socket.socket(socket.AF_UNIX)
sock.bind(sys.argv[1])
sock.close()
PY

cat > "$bin_dir/docker" <<'MOCK_DOCKER'
#!/usr/bin/env bash
set -Eeuo pipefail

printf '%q ' "$@" >> "$MODELPORT_TEST_DOCKER_LOG"
printf '\n' >> "$MODELPORT_TEST_DOCKER_LOG"

last_argument() {
  local value=''
  for value in "$@"; do :; done
  printf '%s' "$value"
}

argument_after() {
  local wanted="$1"
  shift
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "$wanted" ]]; then
      [[ $# -ge 2 ]] || exit 91
      printf '%s' "$2"
      return 0
    fi
    shift
  done
  return 1
}

argument_matching() {
  local pattern="$1"
  local value=''
  shift
  for value in "$@"; do
    if [[ "$value" == $pattern ]]; then
      printf '%s' "$value"
      return 0
    fi
  done
  return 1
}

mount_source() {
  local spec="$1"
  local field=''
  local source=''
  local target=''
  local mount_type=''
  local nocopy=0
  local fields=()

  IFS=',' read -r -a fields <<< "$spec"
  for field in "${fields[@]}"; do
    case "$field" in
      type=*) mount_type="${field#type=}" ;;
      source=*) source="${field#source=}" ;;
      target=*) target="${field#target=}" ;;
      volume-nocopy) nocopy=1 ;;
    esac
  done
  [[ "$mount_type" == 'volume' && -n "$source" && "$target" == '/data' && "$nocopy" -eq 1 ]] || exit 101
  printf '%s' "$source"
}

mount_has_field() {
  local spec="$1"
  local wanted="$2"
  local field=''
  local fields=()

  IFS=',' read -r -a fields <<< "$spec"
  for field in "${fields[@]}"; do
    [[ "$field" == "$wanted" ]] && return 0
  done
  return 1
}

remove_container() {
  local name="$1"
  rm -f -- "$MODELPORT_TEST_STATE/container-$name"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

file_owner() {
  local value=''
  if value="$(stat -c '%u:%g' "$1" 2>/dev/null)" && [[ "$value" =~ ^[0-9]+:[0-9]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  stat -f '%u:%g' "$1"
}

file_mode() {
  local value=''
  if value="$(stat -c '%a' "$1" 2>/dev/null)" && [[ "$value" =~ ^[0-7]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  stat -f '%Lp' "$1"
}

container_volume() {
  cat "$MODELPORT_TEST_STATE/container-$1"
}

volume_data() {
  printf '%s/volume-data-%s' "$MODELPORT_TEST_STATE" "$1"
}

volume_file() {
  local volume="$1"
  local container_path="$2"
  printf '%s%s' "$(volume_data "$volume")" "${container_path#/data}"
}

case "${1:-}" in
  context)
    case "${2:-}" in
      show)
        [[ "$MODELPORT_TEST_FAILURE" != 'docker_context_show' ]] || exit 1
        printf '%s\n' modelport-test
        ;;
      inspect)
        if [[ "$MODELPORT_TEST_FAILURE" == 'docker_context_remote' ]]; then
          printf '%s\n' 'ssh://production.example.invalid'
        elif [[ "$MODELPORT_TEST_FAILURE" == 'docker_context_missing' ]]; then
          printf 'unix://%s/missing-docker.sock\n' "$MODELPORT_TEST_STATE"
        else
          printf 'unix://%s\n' "$MODELPORT_TEST_DOCKER_SOCKET"
        fi
        ;;
      *) exit 92 ;;
    esac
    ;;
  image)
    [[ "${2:-}" == 'inspect' ]] || exit 92
    [[ "$MODELPORT_TEST_FAILURE" != 'image_unavailable' ]] || exit 1
    if [[ "$MODELPORT_TEST_FAILURE" == 'report_parent_swap' ]]; then
      rm -f -- "$MODELPORT_TEST_REPORT_SWAP_LINK"
      ln -s "$MODELPORT_TEST_REPORT_SWAP_REDIRECT" "$MODELPORT_TEST_REPORT_SWAP_LINK"
    fi
    printf '%s\n' "$MODELPORT_TEST_IMAGE_ID"
    ;;
  network)
    case "${2:-}" in
      create)
        name="$(last_argument "$@")"
        if [[ "$MODELPORT_TEST_FAILURE" == 'network_collision' ]]; then
          touch "$MODELPORT_TEST_STATE/unowned-network-$name"
          exit 1
        fi
        touch "$MODELPORT_TEST_STATE/network-$name"
        printf '%s\n' "$name"
        ;;
      inspect)
        name="$(last_argument "$@")"
        if [[ -e "$MODELPORT_TEST_STATE/unowned-network-$name" ]]; then
          printf '%s\n' foreign-run
          exit 0
        fi
        [[ -e "$MODELPORT_TEST_STATE/network-$name" ]] || exit 1
        printf '%s\n' "${name##*-}"
        ;;
      rm)
        name="$(last_argument "$@")"
        rm -f -- "$MODELPORT_TEST_STATE/network-$name"
        printf '%s\n' "$name"
        ;;
      *) exit 93 ;;
    esac
    ;;
  volume)
    case "${2:-}" in
      create)
        name="$(last_argument "$@")"
        touch "$MODELPORT_TEST_STATE/volume-$name"
        mkdir "$MODELPORT_TEST_STATE/volume-data-$name"
        printf '%s\n' "$name"
        ;;
      inspect)
        name="$(last_argument "$@")"
        [[ -e "$MODELPORT_TEST_STATE/volume-$name" ]] || exit 1
        printf '%s\n' "${name##*-}"
        ;;
      rm)
        name="$(last_argument "$@")"
        if [[ "$MODELPORT_TEST_FAILURE" == 'cleanup' ]]; then
          exit 1
        fi
        rm -f -- "$MODELPORT_TEST_STATE/volume-$name"
        rm -rf -- "$MODELPORT_TEST_STATE/volume-data-$name"
        printf '%s\n' "$name"
        ;;
      *) exit 94 ;;
    esac
    ;;
  container)
    [[ "${2:-}" == inspect ]] || exit 94
    name="$(last_argument "$@")"
    [[ -e "$MODELPORT_TEST_STATE/container-$name" ]] || exit 1
    printf '%s\n' "${name##*-}"
    ;;
  create)
    name="$(argument_after --name "$@")"
    mount="$(argument_after --mount "$@")"
    volume="$(mount_source "$mount")"
    mount_has_field "$mount" readonly && exit 103
    printf '%s' "$volume" > "$MODELPORT_TEST_STATE/container-$name"
    if [[ "$MODELPORT_TEST_FAILURE" == 'init_create_partial' && "$name" == modelport-redis-restore-init-* ]]; then
      exit 1
    fi
    printf 'mock-%s\n' "$name"
    ;;
  cp)
    [[ "$MODELPORT_TEST_FAILURE" != 'copy' ]] || exit 1
    source_path="${2:-}"
    destination="${3:-}"
    container="${destination%%:*}"
    container_path="${destination#*:}"
    volume="$(container_volume "$container")"
    target_path="$(volume_file "$volume" "$container_path")"
    mkdir -p "$(dirname -- "$target_path")"
    if [[ "$source_path" == /* ]]; then
      printf '%s' "$source_path" > "$MODELPORT_TEST_STATE/docker-copy-source"
    else
      printf '%s/%s' "$PWD" "${source_path#./}" > "$MODELPORT_TEST_STATE/docker-copy-source"
    fi
    if [[ "$MODELPORT_TEST_FAILURE" == 'input_mutate_restore' ]]; then
      cp -- "$MODELPORT_TEST_SWAP_INPUT_PATH" "$MODELPORT_TEST_SWAP_INPUT_PATH.modelport-test-original"
      printf '%s\n' 'REDIS0011 transient same-inode replacement bytes' > "$MODELPORT_TEST_SWAP_INPUT_PATH"
    fi
    cp -- "$source_path" "$target_path"
    if [[ "$MODELPORT_TEST_FAILURE" == 'input_mutate_restore' ]]; then
      cp -- "$MODELPORT_TEST_SWAP_INPUT_PATH.modelport-test-original" "$MODELPORT_TEST_SWAP_INPUT_PATH"
      rm -f -- "$MODELPORT_TEST_SWAP_INPUT_PATH.modelport-test-original"
    fi
    sha256_file "$target_path" > "$MODELPORT_TEST_STATE/restore-input-sha"
    ;;
  start)
    name="$(last_argument "$@")"
    if [[ "$MODELPORT_TEST_FAILURE" == 'server_start' && "$name" == modelport-redis-restore-server-* ]]; then
      exit 1
    fi
    if [[ "$name" == modelport-redis-restore-init-* ]]; then
      volume="$(container_volume "$name")"
      data="$(volume_data "$volume")"
      chmod 0777 "$data"
    fi
    printf '%s\n' "$name"
    ;;
  wait)
    printf '%s\n' '0'
    ;;
  rm)
    name="$(last_argument "$@")"
    remove_container "$name"
    printf '%s\n' "$name"
    ;;
  run)
    name="$(argument_after --name "$@")"
    entrypoint="$(argument_after --entrypoint "$@")"
    mount="$(argument_after --mount "$@")"
    volume="$(mount_source "$mount")"
    target="$(last_argument "$@")"
    target_path="$(volume_file "$volume" "$target")"
    touch "$MODELPORT_TEST_STATE/container-$name"
    case "$entrypoint" in
      sha256sum)
        mount_has_field "$mount" readonly || exit 104
        [[ -f "$target_path" ]] || exit 98
        printf '%s  %s\n' "$(sha256_file "$target_path")" "$target"
        remove_container "$name"
        ;;
      redis-check-rdb|redis-check-aof)
        if [[ "$MODELPORT_TEST_FAILURE" == 'checker' ]]; then
          printf '%s\n' 'mock checker rejected input' >&2
          exit 1
        fi
        [[ -f "$target_path" ]] || exit 99
        if [[ "$entrypoint" == 'redis-check-aof' ]]; then
          mount_has_field "$mount" readonly && exit 105
          [[ "$target" == '/data/.modelport-legacy-aof-check-copy' && "$(file_mode "$target_path")" == '600' ]] || exit 100
          magic="$(od -An -tx1 -N5 "$target_path" | tr -d '[:space:]')"
          if [[ "$magic" == '5245444953' ]]; then
            base_name='appendonly.aof.1.base.rdb'
          else
            base_name='appendonly.aof.1.base.aof'
          fi
          base_path="$(volume_file "$volume" "/data/appendonlydir/$base_name")"
          [[ -f "$base_path" && "$(file_mode "$base_path")" == '444' ]] || exit 106
          printf '%s\n' 'Start checking Old-Style AOF' 'AOF is valid'
        else
          mount_has_field "$mount" readonly || exit 107
          printf '%s\n' \
            '[offset 9] Selecting DB ID 0' \
            '[offset 32] Selecting DB ID 2' \
            'RDB looks OK!'
        fi
        remove_container "$name"
        ;;
      *) exit 95 ;;
    esac
    ;;
  inspect)
    if [[ "$MODELPORT_TEST_FAILURE" == 'server_exit' ]]; then
      printf '%s\n' 'exited'
    else
      printf '%s\n' 'running'
    fi
    ;;
  exec)
    joined="$*"
    init_name="$(argument_matching 'modelport-redis-restore-init-*' "$@" || true)"
    if [[ -n "$init_name" && "$joined" == *'test -f /tmp/.modelport-init-ready'* ]]; then
      :
    elif [[ -n "$init_name" && "$joined" == *"stat -c %u:%g /data/"* ]]; then
      volume="$(container_volume "$init_name")"
      target="$(last_argument "$@")"
      target_path="$(volume_file "$volume" "$target")"
      [[ -f "$target_path" ]] || exit 102
      file_owner "$target_path"
      printf '\n'
    elif [[ -n "$init_name" && "$joined" == *' sh -c '* ]]; then
      volume="$(container_volume "$init_name")"
      data="$(volume_data "$volume")"
      if [[ "$MODELPORT_TEST_SOURCE_KIND" == 'legacy-aof' ]]; then
        mkdir -p "$data/appendonlydir"
        magic="$(od -An -tx1 -N5 "$data/input.aof" | tr -d '[:space:]')"
        if [[ "$magic" == '5245444953' ]]; then
          base_name='appendonly.aof.1.base.rdb'
        else
          base_name='appendonly.aof.1.base.aof'
        fi
        mv "$data/input.aof" "$data/appendonlydir/$base_name"
        cp "$data/appendonlydir/$base_name" "$data/.modelport-legacy-aof-check-copy"
        chmod 0777 "$data/appendonlydir"
        chmod 0444 "$data/appendonlydir/$base_name"
        chmod 0600 "$data/.modelport-legacy-aof-check-copy"
        printf 'file %s seq 1 type b\n' "$base_name" > "$data/appendonlydir/appendonly.aof.manifest"
        chmod 0666 "$data/appendonlydir/appendonly.aof.manifest"
      else
        chmod 0444 "$data/dump.rdb"
      fi
    elif [[ "$joined" == *' --raw PING' ]]; then
      printf '%s\n' 'PONG'
    elif [[ "$joined" == *' --raw INFO server' ]]; then
      printf '%s\n' '# Server' 'redis_version:8.2.1'
    elif [[ "$joined" == *' --raw INFO persistence' ]]; then
      [[ "$MODELPORT_TEST_FAILURE" != 'probe' ]] || exit 1
      printf '%s\n' \
        '# Persistence' \
        'loading:0' \
        'rdb_last_save_time:1700000000' \
        'rdb_last_bgsave_status:ok'
      if [[ "$MODELPORT_TEST_SOURCE_KIND" == 'legacy-aof' ]]; then
        printf '%s\n' \
          'aof_enabled:1' \
          'aof_last_write_status:ok' \
          'aof_current_size:128' \
          'aof_base_size:96'
      else
        printf '%s\n' \
          'aof_enabled:0' \
          'aof_last_write_status:ok'
      fi
    elif [[ "$joined" == *' --raw INFO keyspace' ]]; then
      printf '%s\n' '# Keyspace' 'db0:keys=3,expires=1,avg_ttl=24000' 'db2:keys=2,expires=0,avg_ttl=0,subexpiry=0'
    elif [[ "$joined" == *' --raw DBSIZE' ]]; then
      printf '%s\n' '3'
    elif [[ "$joined" == *'redis-cli --version' ]]; then
      printf '%s\n' 'redis-cli 8.2.1'
    else
      exit 96
    fi
    ;;
  *) exit 97 ;;
esac
MOCK_DOCKER
chmod 0755 "$bin_dir/docker"

rdb_input="$external_dir/production-dump.rdb"
aof_input="$external_dir/production-appendonly.aof"
printf 'REDIS0011%s\n' "$fixture_secret" > "$rdb_input"
printf '%s\r\n' \
  '*3' \
  "\$3" \
  'SET' \
  "\$10" \
  'sample-key' \
  "\$${#fixture_secret}" \
  "$fixture_secret" \
  > "$aof_input"
rdb_sha="$(sha256_file "$rdb_input")"
aof_sha="$(sha256_file "$aof_input")"

run_script() {
  local source_kind="$1"
  local failure_mode="$2"
  local input_sha="$3"
  shift 3
  if [[ "${MODELPORT_TEST_OMIT_REDIS_MAJOR:-0}" != '1' ]]; then
    set -- --expected-redis-major "${MODELPORT_TEST_EXPECTED_REDIS_MAJOR:-8}" "$@"
  fi
  env \
    PATH="$bin_dir:$PATH" \
    MODELPORT_TEST_DOCKER_LOG="$docker_log" \
    MODELPORT_TEST_DOCKER_SOCKET="$docker_socket" \
    MODELPORT_TEST_STATE="$state_dir" \
    MODELPORT_TEST_IMAGE_ID="$image_id" \
    MODELPORT_TEST_INPUT_SHA="$input_sha" \
    MODELPORT_TEST_SOURCE_KIND="$source_kind" \
    MODELPORT_TEST_FAILURE="$failure_mode" \
    MODELPORT_TEST_REPORT_SWAP_LINK="$report_swap_link" \
    MODELPORT_TEST_REPORT_SWAP_REDIRECT="$report_swap_redirect" \
    MODELPORT_TEST_SWAP_INPUT_PATH="$input_mutate_path" \
    DOCKER_HOST="${MODELPORT_TEST_DOCKER_HOST:-}" \
    /bin/bash "$script" "$@" >"$stdout_log" 2>"$stderr_log"
}

expect_pre_docker_failure() {
  reset_run
  if run_script rdb none "$rdb_sha" "$@"; then
    fail "invalid invocation unexpectedly passed: $*"
  fi
  [[ ! -s "$docker_log" ]] || fail 'Docker was called before local validation rejected the invocation'
}

expect_runtime_failure() {
  local source_kind="$1"
  local failure_mode="$2"
  local input_path="$3"
  local input_sha="$4"
  local report="$5"
  local legacy_confirm=''
  local expected_major=8
  if [[ "$source_kind" == 'legacy-aof' ]]; then
    legacy_confirm='--confirm-complete-legacy-aof'
  fi
  if [[ "$failure_mode" == redis_major_mismatch ]]; then
    expected_major=7
  fi
  reset_run
  rm -f -- "$report"
  if MODELPORT_TEST_EXPECTED_REDIS_MAJOR="$expected_major" \
    run_script "$source_kind" "$failure_mode" "$input_sha" \
    --source-kind "$source_kind" \
    --input "$input_path" \
    --expected-sha256 "$input_sha" \
    --rpo-at "$rpo_at" \
    --redis-image "$image_ref" \
    --report "$report" \
    ${legacy_confirm:+"$legacy_confirm"} \
    --confirm-sensitive-storage; then
    fail "failure mode $failure_mode unexpectedly passed"
  fi
  [[ ! -e "$report" ]] || fail "failure mode $failure_mode published a success report"
}

run_success() {
  local source_kind="$1"
  local input_path="$2"
  local input_sha="$3"
  local report="$4"
  local expected_aof="$5"
  local legacy_confirm=''
  local report_parent_physical=''
  if [[ "$source_kind" == 'legacy-aof' ]]; then
    legacy_confirm='--confirm-complete-legacy-aof'
  fi
  reset_run
  rm -f -- "$report"
  run_script "$source_kind" none "$input_sha" \
    --source-kind "$source_kind" \
    --input "$input_path" \
    --expected-sha256 "$input_sha" \
    --rpo-at "$rpo_at" \
    --redis-image "$image_ref" \
    --report "$report" \
    ${legacy_confirm:+"$legacy_confirm"} \
    --confirm-sensitive-storage

  jq -e \
    --arg kind "$source_kind" \
    --arg sha "$input_sha" \
    --arg image "$image_ref" \
    --arg image_id "$image_id" \
    --argjson expected_aof "$expected_aof" '
      .schema_version == 1 and .result == "passed" and
      .input.kind == $kind and
      .input.sha256 == $sha and
      .input.verified_copy_sha256 == $sha and
      .input.expected_sha256 == $sha and
      .input.disposable_copy_sha256 == $sha and
      .input.path_recorded == false and
      .input.complete_legacy_aof_confirmed == $expected_aof and
      (.input.size_bytes | type == "number") and .input.size_bytes > 0 and
      .rpo.boundary_utc == "2026-08-28T00:00:00Z" and
      .rpo.basis == "operator_supplied_persistence_boundary" and
      (.rpo.age_seconds_at_verification | type == "number") and
      .runtime.redis_image.reference == $image and
      .runtime.redis_image.id == $image_id and
      .runtime.redis_server_version == "8.2.1" and
      .runtime.redis_server_major == 8 and
      .runtime.expected_redis_major == 8 and
      .runtime.redis_cli_version == "redis-cli 8.2.1" and
      .verification.ping == "PONG" and
      .verification.stable_verified_input_copy_used == true and
      .verification.checker_writable_temporary_copy == $expected_aof and
      .verification.restore_input_read_only_during_check == true and
      .verification.configured_databases == 16 and
      .verification.dbsize_default_database == 3 and
      .verification.keyspace == [
        {database:"db0",keys:3,expires:1,avg_ttl_ms:24000,subexpiry:0},
        {database:"db2",keys:2,expires:0,avg_ttl_ms:0,subexpiry:0}
      ] and
      .verification.persistence.loading == 0 and
      .verification.persistence.aof_enabled == $expected_aof and
      .verification.persistence.aof_current_size == (if $expected_aof then 128 else null end) and
      .verification.persistence.aof_base_size == (if $expected_aof then 96 else null end) and
      .isolation.preloaded_digest_pinned_image == true and
      .isolation.docker_internal_network == true and
      .isolation.published_ports == false and
      .isolation.read_only_rootfs == true and
      .isolation.capabilities_dropped == true and
      .isolation.no_new_privileges == true and
      .isolation.disposable_random_volume == true and
      .isolation.resources_removed_before_success_report == true and
      .content_disclosure.key_names_included == false and
      .content_disclosure.values_included == false
    ' "$report" >/dev/null || fail "success report failed its contract: $report"

  [[ "$(file_mode "$report")" == '600' ]] || fail 'success report mode was not 0600'
  assert_not_contains "$input_path" "$report"
  assert_not_contains "$fixture_secret" "$report"
  assert_not_contains 'sample-key' "$report"
  if find "$(dirname -- "$report")" -maxdepth 1 -name '.modelport-isolated-redis-report.*' | grep -q .; then
    fail 'a report staging file remained after success'
  fi
  if find "$(dirname -- "$report")" -maxdepth 1 -name '.modelport-isolated-redis-restore-work.*' | grep -q .; then
    fail 'the encrypted report parent retained a Redis restore work directory'
  fi
  stable_input_path="$(<"$state_dir/docker-copy-source")"
  report_parent_physical="$(cd -P -- "$(dirname -- "$report")" && pwd)"
  [[ "$stable_input_path" == "$report_parent_physical"/.modelport-isolated-redis-restore-work.*/verified-input.backup ]] || \
    fail 'Docker did not consume the private stable input copy'
  [[ ! -e "$stable_input_path" ]] || fail 'the private stable input copy remained after success'
  assert_no_owned_resources
}

run_real_drill() {
  local drill_image="$1"
  local drill_dir="$external_dir/real-drill"
  local drill_suffix=''
  local drill_image_id=''
  local ready=0
  local attempt=0
  local rdb_input_real="$drill_dir/complete.rdb"
  local rdb_report_real="$drill_dir/rdb-report.json"
  local legacy_input_real="$drill_dir/complete-legacy.aof"
  local legacy_report_real="$drill_dir/legacy-aof-report.json"
  local rdb_stdout="$drill_dir/rdb-stdout.log"
  local rdb_stderr="$drill_dir/rdb-stderr.log"
  local legacy_stdout="$drill_dir/legacy-aof-stdout.log"
  local legacy_stderr="$drill_dir/legacy-aof-stderr.log"
  local synthetic_rdb_key='synthetic-rdb-drill-key'
  local synthetic_rdb_value='synthetic-rdb-drill-value'
  local synthetic_legacy_key='synthetic-legacy-key'
  local synthetic_legacy_value='synthetic-legacy-value'
  local synthetic_legacy_db2_key='test-db2'
  local synthetic_legacy_db2_value='synthetic-legacy-db2-value'
  local legacy_key_length=''
  local legacy_value_length=''
  local legacy_db2_key_length=''
  local legacy_db2_value_length=''
  local rdb_sha_real=''
  local legacy_sha_real=''
  local save_output=''
  local drill_redis_major=''

  [[ "$drill_image" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[0-9a-f]{64}$ ]] ||
    fail 'MODELPORT_REDIS_DRILL_IMAGE must be a digest-pinned local image reference'
  drill_image_id="$(docker image inspect --format '{{.Id}}' "$drill_image")" ||
    fail 'the real-drill Redis image is not preloaded locally'
  [[ "$drill_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] ||
    fail 'the real-drill Redis image resolved to an invalid image ID'

  drill_suffix="$(printf '%s' "$test_root" | awk -F. '{print $NF}')"
  [[ "$drill_suffix" =~ ^[A-Za-z0-9]+$ ]] || fail 'could not derive a safe real-drill suffix'
  mkdir "$drill_dir"
  real_generator_container="modelport-redis-real-drill-generator-$drill_suffix"
  real_generator_volume="modelport-redis-real-drill-data-$drill_suffix"
  if docker container inspect "$real_generator_container" >/dev/null 2>&1 ||
     docker volume inspect "$real_generator_volume" >/dev/null 2>&1; then
    fail 'a real-drill Docker resource name already exists'
  fi
  docker volume create \
    --label 'com.abingooo.modelport.purpose=isolated-redis-restore-real-drill' \
    "$real_generator_volume" >/dev/null
  real_generator_volume_created=1
  docker run -d \
    --name "$real_generator_container" \
    --network none \
    --pull=never \
    --read-only \
    --user '0:0' \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    --pids-limit 128 \
    --mount "type=volume,source=$real_generator_volume,target=/data,volume-nocopy" \
    --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=32m,mode=0700' \
    --entrypoint redis-server \
    "$drill_image_id" \
    --bind 127.0.0.1 \
    --protected-mode yes \
    --save '' \
    --appendonly no \
    --databases 32 \
    --dir /data \
    --dbfilename dump.rdb \
    >"$drill_dir/generator-start.log" 2>&1
  real_generator_created=1

  while (( attempt < 30 )); do
    if [[ "$(docker exec "$real_generator_container" redis-cli --raw PING 2>/dev/null || true)" == 'PONG' ]]; then
      ready=1
      break
    fi
    sleep 1
    attempt=$((attempt + 1))
  done
  [[ "$ready" -eq 1 ]] || fail 'the real-drill Redis generator did not become ready'
  drill_redis_major="$(docker exec "$real_generator_container" redis-cli --raw INFO server |
    awk -F: '$1 == "redis_version" {split($2, version, "."); print version[1]; exit}')"
  [[ "$drill_redis_major" =~ ^[0-9]+$ && "$drill_redis_major" -gt 0 ]] || \
    fail 'the real-drill Redis image returned an invalid major version'

  docker exec "$real_generator_container" \
    redis-cli -n 0 SET "$synthetic_rdb_key" "$synthetic_rdb_value" >/dev/null
  docker exec "$real_generator_container" \
    redis-cli -n 20 SET "$synthetic_rdb_key" "$synthetic_rdb_value" >/dev/null
  save_output="$(docker exec "$real_generator_container" redis-cli --raw SAVE)" || \
    fail 'the real-drill Redis generator SAVE command failed'
  if [[ "$save_output" != 'OK' ]]; then
    docker logs "$real_generator_container" >"$drill_dir/generator-runtime.log" 2>&1 || true
    printf 'real-drill SAVE response: %s\n' "$save_output" >&2
    fail 'the real-drill Redis generator did not persist its RDB'
  fi
  if ! docker exec "$real_generator_container" test -f /data/dump.rdb; then
    docker exec "$real_generator_container" redis-cli --raw CONFIG GET dir dbfilename \
      >"$drill_dir/generator-config.log" 2>&1 || true
    docker logs "$real_generator_container" >"$drill_dir/generator-runtime.log" 2>&1 || true
    fail 'the real-drill Redis generator did not create /data/dump.rdb'
  fi
  docker cp "$real_generator_container:/data/dump.rdb" "$rdb_input_real" >/dev/null
  docker rm --force "$real_generator_container" >/dev/null
  real_generator_created=0
  docker volume rm "$real_generator_volume" >/dev/null
  real_generator_volume_created=0

  rdb_sha_real="$(sha256_file "$rdb_input_real")"
  if ! /bin/bash "$script" \
    --source-kind rdb \
    --input "$rdb_input_real" \
    --expected-sha256 "$rdb_sha_real" \
    --expected-redis-major "$drill_redis_major" \
    --rpo-at "$rpo_at" \
    --redis-image "$drill_image" \
    --report "$rdb_report_real" \
    --confirm-sensitive-storage \
    >"$rdb_stdout" 2>"$rdb_stderr"; then
    sed -n '1,40p' "$rdb_stderr" >&2
    fail 'the real RDB drill failed'
  fi
  jq -e '
    .result == "passed" and
    .input.kind == "rdb" and
    .input.complete_legacy_aof_confirmed == false and
    .verification.configured_databases == 21 and
    any(.verification.keyspace[]; .database == "db0" and .keys == 1) and
    any(.verification.keyspace[]; .database == "db20" and .keys == 1) and
    .verification.persistence.aof_enabled == false
  ' "$rdb_report_real" >/dev/null || fail 'the real RDB drill report failed its contract'

  legacy_key_length="$(printf '%s' "$synthetic_legacy_key" | wc -c | tr -d '[:space:]')"
  legacy_value_length="$(printf '%s' "$synthetic_legacy_value" | wc -c | tr -d '[:space:]')"
  legacy_db2_key_length="$(printf '%s' "$synthetic_legacy_db2_key" | wc -c | tr -d '[:space:]')"
  legacy_db2_value_length="$(printf '%s' "$synthetic_legacy_db2_value" | wc -c | tr -d '[:space:]')"
  printf '%s\r\n' \
    '*2' "\$6" 'SELECT' "\$1" '0' \
    '*3' "\$3" 'SET' "\$$legacy_key_length" "$synthetic_legacy_key" \
    "\$$legacy_value_length" "$synthetic_legacy_value" \
    '*2' "\$6" 'SELECT' "\$1" '2' \
    '*3' "\$3" 'SET' "\$$legacy_db2_key_length" "$synthetic_legacy_db2_key" \
    "\$$legacy_db2_value_length" "$synthetic_legacy_db2_value" \
    > "$legacy_input_real"
  legacy_sha_real="$(sha256_file "$legacy_input_real")"
  if ! /bin/bash "$script" \
    --source-kind legacy-aof \
    --input "$legacy_input_real" \
    --expected-sha256 "$legacy_sha_real" \
    --expected-redis-major "$drill_redis_major" \
    --rpo-at "$rpo_at" \
    --redis-image "$drill_image" \
    --report "$legacy_report_real" \
    --confirm-complete-legacy-aof \
    --confirm-sensitive-storage \
    >"$legacy_stdout" 2>"$legacy_stderr"; then
    sed -n '1,40p' "$legacy_stderr" >&2
    fail 'the real legacy AOF drill failed'
  fi
  jq -e '
    .result == "passed" and
    .input.kind == "legacy-aof" and
    .input.complete_legacy_aof_confirmed == true and
    .verification.integrity_checker == "redis-check-aof" and
    .verification.checker_writable_temporary_copy == true and
    .verification.restore_input_read_only_during_check == true and
    .verification.configured_databases == 16 and
    any(.verification.keyspace[]; .database == "db0" and .keys == 1) and
    any(.verification.keyspace[]; .database == "db2" and .keys == 1) and
    .verification.persistence.aof_enabled == true
  ' "$legacy_report_real" >/dev/null || fail 'the real legacy AOF drill report failed its contract'

  for disclosure in \
    "$synthetic_rdb_key" "$synthetic_rdb_value" \
    "$synthetic_legacy_key" "$synthetic_legacy_value" \
    "$synthetic_legacy_db2_key" "$synthetic_legacy_db2_value"; do
    assert_not_contains "$disclosure" "$rdb_report_real"
    assert_not_contains "$disclosure" "$rdb_stdout"
    assert_not_contains "$disclosure" "$rdb_stderr"
    assert_not_contains "$disclosure" "$legacy_report_real"
    assert_not_contains "$disclosure" "$legacy_stdout"
    assert_not_contains "$disclosure" "$legacy_stderr"
  done
  [[ "$(file_mode "$rdb_report_real")" == '600' ]] || fail 'the real RDB drill report mode was not 0600'
  [[ "$(file_mode "$legacy_report_real")" == '600' ]] || fail 'the real legacy AOF drill report mode was not 0600'
}

# Local validation must fail before Docker is contacted.
MODELPORT_TEST_OMIT_REDIS_MAJOR=1 expect_pre_docker_failure \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/missing-redis-major.json" \
  --confirm-sensitive-storage

expect_pre_docker_failure \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/checksum-mismatch.json" \
  --confirm-sensitive-storage

expect_pre_docker_failure \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image redis:8-alpine \
  --report "$external_dir/tag-only-image.json" \
  --confirm-sensitive-storage

expect_pre_docker_failure \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image 'redis@sha256:not-a-valid-digest' \
  --report "$external_dir/invalid-image.json" \
  --confirm-sensitive-storage

expect_pre_docker_failure \
  --source-kind aof \
  --input "$aof_input" \
  --expected-sha256 "$aof_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/generic-aof-kind.json" \
  --confirm-sensitive-storage

expect_pre_docker_failure \
  --source-kind legacy-aof \
  --input "$aof_input" \
  --expected-sha256 "$aof_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/unconfirmed-legacy-aof.json" \
  --confirm-sensitive-storage

for multipart_name in \
  'appendonly.aof.manifest' \
  'appendonly.aof.1.base.aof' \
  'appendonly.aof.1.base.rdb' \
  'appendonly.aof.2.incr.aof'; do
  multipart_member_input="$external_dir/$multipart_name"
  cp -- "$aof_input" "$multipart_member_input"
  expect_pre_docker_failure \
    --source-kind legacy-aof \
    --input "$multipart_member_input" \
    --expected-sha256 "$aof_sha" \
    --rpo-at "$rpo_at" \
    --redis-image "$image_ref" \
    --report "$external_dir/$multipart_name-report.json" \
    --confirm-complete-legacy-aof \
    --confirm-sensitive-storage
done

expect_pre_docker_failure \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/rdb-with-legacy-confirmation.json" \
  --confirm-complete-legacy-aof \
  --confirm-sensitive-storage

input_link="$external_dir/input-link.rdb"
ln -s "$rdb_input" "$input_link"
expect_pre_docker_failure \
  --source-kind rdb \
  --input "$input_link" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/symlink-input.json" \
  --confirm-sensitive-storage

input_dir="$external_dir/input-directory"
mkdir "$input_dir"
expect_pre_docker_failure \
  --source-kind rdb \
  --input "$input_dir" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$external_dir/directory-input.json" \
  --confirm-sensitive-storage

unavailable_image_report="$external_dir/unavailable-image.json"
expect_runtime_failure rdb image_unavailable "$rdb_input" "$rdb_sha" "$unavailable_image_report"
assert_contains 'image inspect --format' "$docker_log"
assert_not_contains 'network create' "$docker_log"
assert_no_owned_resources

MODELPORT_TEST_DOCKER_HOST='tcp://production.example.invalid:2376' \
  expect_runtime_failure rdb none "$rdb_input" "$rdb_sha" "$external_dir/docker-host-rejected.json"
[[ ! -s "$docker_log" ]] || fail 'Docker was called when DOCKER_HOST was set'

expect_runtime_failure rdb docker_context_remote "$rdb_input" "$rdb_sha" "$external_dir/remote-context-rejected.json"
assert_not_contains 'image inspect' "$docker_log"

expect_runtime_failure rdb docker_context_missing "$rdb_input" "$rdb_sha" "$external_dir/missing-socket-rejected.json"
assert_not_contains 'image inspect' "$docker_log"

partial_create_report="$external_dir/partial-create-failed.json"
expect_runtime_failure rdb init_create_partial "$rdb_input" "$rdb_sha" "$partial_create_report"
assert_contains 'rm --force modelport-redis-restore-init-' "$docker_log"
assert_no_owned_resources

collision_report="$external_dir/network-collision-failed.json"
expect_runtime_failure rdb network_collision "$rdb_input" "$rdb_sha" "$collision_report"
assert_not_contains 'network rm modelport-redis-restore-net-' "$docker_log"
find "$state_dir" -maxdepth 1 -type f -name 'unowned-network-modelport-redis-restore-net-*' | grep -q . || \
  fail 'the simulated unowned network collision was not preserved'
find "$state_dir" -maxdepth 1 -type f -name 'unowned-network-modelport-redis-restore-net-*' -delete
assert_no_owned_resources

# Successful RDB restore produces aggregate-only evidence and hardened Docker calls.
rdb_report="$external_dir/rdb-report.json"
run_success rdb "$rdb_input" "$rdb_sha" "$rdb_report" false
jq -e '.verification.integrity_checker == "redis-check-rdb"' "$rdb_report" >/dev/null || \
  fail 'RDB report recorded the wrong checker'
assert_contains 'network create --internal --label com.abingooo.modelport.purpose=isolated-redis-restore' "$docker_log"
assert_contains '--label com.abingooo.modelport.restore-run=' "$docker_log"
assert_contains 'modelport-redis-restore-net-' "$docker_log"
assert_contains '--network none' "$docker_log"
assert_contains '--pull=never' "$docker_log"
assert_contains '--read-only' "$docker_log"
assert_contains '--cap-drop ALL' "$docker_log"
assert_contains '--security-opt no-new-privileges' "$docker_log"
assert_contains 'volume create --label com.abingooo.modelport.purpose=isolated-redis-restore' "$docker_log"
assert_contains '--mount type=volume\,source=modelport-redis-restore-data-' "$docker_log"
assert_contains 'target=/data\,volume-nocopy\,readonly' "$docker_log"
assert_contains 'network rm modelport-redis-restore-net-' "$docker_log"
assert_contains 'volume rm modelport-redis-restore-data-' "$docker_log"
assert_contains 'rm --force modelport-redis-restore-server-' "$docker_log"
assert_not_contains '--network host' "$docker_log"
assert_not_contains '--publish' "$docker_log"

expect_runtime_failure rdb redis_major_mismatch "$rdb_input" "$rdb_sha" "$external_dir/redis-major-mismatch.json"
assert_contains 'redis_major_mismatch' "$stderr_log"

pull_count="$(grep -o -- '--pull=never' "$docker_log" | wc -l | tr -d '[:space:]')"
[[ "$pull_count" == '4' ]] || fail "expected four --pull=never container operations, found $pull_count"

# The restore consumes the verified private copy even if the source is modified
# in place and restored while Docker copies the input.
stable_copy_report="$external_dir/stable-copy-report.json"
reset_run
input_mutate_path="$rdb_input"
run_script rdb input_mutate_restore "$rdb_sha" \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$stable_copy_report" \
  --confirm-sensitive-storage
input_mutate_path=''
jq -e --arg sha "$rdb_sha" '
  .result == "passed" and
  .input.sha256 == $sha and
  .input.verified_copy_sha256 == $sha and
  .input.disposable_copy_sha256 == $sha and
  .verification.stable_verified_input_copy_used == true
' "$stable_copy_report" >/dev/null || fail 'the stable-copy mutation drill produced invalid evidence'
[[ "$(<"$state_dir/restore-input-sha")" == "$rdb_sha" ]] || \
  fail 'Docker consumed transiently modified source bytes instead of the verified copy'
[[ "$(sha256_file "$rdb_input")" == "$rdb_sha" ]] || \
  fail 'the stable-copy mutation drill did not restore the source fixture'
stable_input_path="$(<"$state_dir/docker-copy-source")"
external_dir_physical="$(cd -P -- "$external_dir" && pwd)"
[[ "$stable_input_path" == "$external_dir_physical"/.modelport-isolated-redis-restore-work.*/verified-input.backup ]] || \
  fail 'the stable-copy mutation drill did not use the private verified input'
[[ ! -e "$stable_input_path" ]] || fail 'the stable-copy mutation drill left the private input behind'
assert_no_owned_resources

# The report remains bound to the validated physical directory if a symlink
# ancestor is retargeted after validation.
bound_report_dir="$test_root/bound-report-parent"
redirect_report_dir="$test_root/redirect-report-parent"
report_parent_link="$test_root/report-parent-link"
mkdir "$bound_report_dir" "$redirect_report_dir"
ln -s "$bound_report_dir" "$report_parent_link"
report_swap_link="$report_parent_link"
report_swap_redirect="$redirect_report_dir"
bound_report="$bound_report_dir/bound-report.json"
reset_run
run_script rdb report_parent_swap "$rdb_sha" \
  --source-kind rdb \
  --input "$rdb_input" \
  --expected-sha256 "$rdb_sha" \
  --rpo-at "$rpo_at" \
  --redis-image "$image_ref" \
  --report "$report_parent_link/bound-report.json" \
  --confirm-sensitive-storage
report_swap_link=''
report_swap_redirect=''
[[ -f "$bound_report" ]] || fail 'the bound physical report parent did not receive the report'
[[ ! -e "$redirect_report_dir/bound-report.json" ]] || fail 'the report followed a retargeted symlink ancestor'
jq -e '.result == "passed" and .input.kind == "rdb"' "$bound_report" >/dev/null || \
  fail 'the report-parent binding run produced an invalid report'
assert_contains "$bound_report" "$stdout_log"
assert_no_owned_resources

# A complete legacy single-file AOF uses a writable checker copy and never passes --fix.
aof_report="$external_dir/aof-report.json"
run_success legacy-aof "$aof_input" "$aof_sha" "$aof_report" true
jq -e '.verification.integrity_checker == "redis-check-aof"' "$aof_report" >/dev/null || \
  fail 'legacy AOF report recorded the wrong checker'
assert_contains '--entrypoint redis-check-aof' "$docker_log"
assert_contains '/data/.modelport-legacy-aof-check-copy' "$docker_log"
assert_not_contains '--fix' "$docker_log"

# Checker, startup, and probe failures publish no success report and clean resources.
checker_report="$external_dir/checker-rejected.json"
expect_runtime_failure rdb checker "$rdb_input" "$rdb_sha" "$checker_report"
assert_contains '--entrypoint redis-check-rdb' "$docker_log"
assert_contains 'rm --force modelport-redis-restore-check-' "$docker_log"
assert_no_owned_resources

start_report="$external_dir/start-failed.json"
expect_runtime_failure rdb server_start "$rdb_input" "$rdb_sha" "$start_report"
assert_contains 'rm --force modelport-redis-restore-server-' "$docker_log"
assert_contains 'network rm modelport-redis-restore-net-' "$docker_log"
assert_contains 'volume rm modelport-redis-restore-data-' "$docker_log"
assert_no_owned_resources

probe_report="$external_dir/probe-failed.json"
expect_runtime_failure rdb probe "$rdb_input" "$rdb_sha" "$probe_report"
assert_contains 'rm --force modelport-redis-restore-server-' "$docker_log"
assert_no_owned_resources

# A cleanup error must fail closed and must not publish a success report.
cleanup_report="$external_dir/cleanup-failed.json"
expect_runtime_failure rdb cleanup "$rdb_input" "$rdb_sha" "$cleanup_report"
assert_contains 'volume rm modelport-redis-restore-data-' "$docker_log"
assert_contains 'cleanup_failed' "$stderr_log"

if [[ -n "${MODELPORT_REDIS_DRILL_IMAGE:-}" ]]; then
  run_real_drill "$MODELPORT_REDIS_DRILL_IMAGE"
fi

printf 'ModelPort isolated Redis restore contract test passed\n'
