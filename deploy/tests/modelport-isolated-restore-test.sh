#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$repo_root/deploy/modelport-isolated-restore.sh"
test_root="$(mktemp -d /tmp/modelport-isolated-restore-test.XXXXXX)"
bin_dir="$test_root/bin"
state_dir="$test_root/state"
external_dir="$test_root/encrypted-external"
docker_log="$test_root/docker.log"
stdout_log="$test_root/stdout.log"
stderr_log="$test_root/stderr.log"
readonly image_ref='registry.example.invalid/postgres@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2'
readonly image_id='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
readonly revision='0123456789abcdef0123456789abcdef01234567'
readonly tree_sha='89abcdef0123456789abcdef0123456789abcdef'
readonly fixture_secret='MODELPORT_FIXTURE_CIPHERTEXT_DO_NOT_LEAK_9f85c2'
docker_socket="$test_root/docker.sock"
report_swap_link=''
report_swap_redirect=''

legacy_ledger_json="$(jq -Rn '
  [inputs |
    select(length > 0 and (startswith("#") | not)) |
    split("\t") |
    {filename: .[0], checksum: .[2], applied_at: "2025-01-01 00:00:00+00"}]
' < "$repo_root/backend/migrations/modelport_legacy/v0.1.176.2/manifest.tsv")"
required_bridge_tables_json="$(
  sed -nE \
    's/^[[:space:]]*CREATE TABLE IF NOT EXISTS ([A-Za-z0-9_]+).*/\1/p' \
    "$repo_root/backend/migrations/233_modelport_lottery_bridge.sql" \
    "$repo_root/backend/migrations/234_modelport_instruction_audit_bridge.sql" |
    LC_ALL=C sort -u | jq -Rsc 'split("\n") | map(select(length > 0))'
)"
required_bridge_constraints_json="$(
  sed -nE \
    's/.*CONSTRAINT[[:space:]]+(IF[[:space:]]+EXISTS[[:space:]]+)?([A-Za-z0-9_]+).*/\2/p' \
    "$repo_root/backend/migrations/233_modelport_lottery_bridge.sql" \
    "$repo_root/backend/migrations/234_modelport_instruction_audit_bridge.sql" \
    "$repo_root/backend/migrations/236_modelport_legacy_platform_constraints.sql" |
    LC_ALL=C sort -u | jq -Rsc 'split("\n") | map(select(length > 0))'
)"
required_bridge_indexes_json="$(
  sed -nE \
    's/^[[:space:]]*CREATE (UNIQUE )?INDEX IF NOT EXISTS ([A-Za-z0-9_]+).*/\2/p' \
    "$repo_root/backend/migrations/233_modelport_lottery_bridge.sql" \
    "$repo_root/backend/migrations/234_modelport_instruction_audit_bridge.sql" |
    LC_ALL=C sort -u | jq -Rsc 'split("\n") | map(select(length > 0))'
)"

cleanup() {
  if [[ "${MODELPORT_TEST_KEEP:-0}" == 1 ]]; then
    printf 'Preserved test directory: %s\n' "$test_root" >&2
    return
  fi
  rm -rf -- "$test_root"
}
trap cleanup EXIT

fail() {
  printf 'ModelPort isolated restore contract test failed: %s\n' "$1" >&2
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
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
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

mkdir -p "$bin_dir" "$state_dir" "$external_dir"
touch "$state_dir/unowned-sentinel"
python3 - "$docker_socket" <<'PY'
import socket
import sys

sock = socket.socket(socket.AF_UNIX)
sock.bind(sys.argv[1])
sock.close()
PY

cat > "$bin_dir/docker" <<'MOCK_DOCKER'
#!/usr/bin/env bash
set -euo pipefail

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

emit_snapshot() {
  local phase="$1"
  local kind="$MODELPORT_TEST_FIXTURE_KIND"
  local migrations settings key_counts all_counts finance ciphertext sequence_position_valid missing_table
  local audit_seed_summaries audit_seed_present audit_seed_legacy
  local hash_false='fcbcf165908dd18a9e49f7ff27810176db8e9f63b4352213741664245224f8aa'
  local hash_true='b5bea41b6c623f7c09f1bf24dcae58ebab3c0cdd90ad966bc43a45b44867e12b'
  local hash_v1='3bfc269594ef649228e9a74bab00f042efc91d5acc6fbee31a382e80d42388fe'
  local hash_30='624b60c58c9d8bfb6ff1886c2fd605d2adeb6ea4da576068201b6c6958ce93f4'
  local hash_site='cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'
  local before_upstream after_upstream before_legacy after_legacy current

  before_upstream='[
    {"filename":"195_channel_monitor_mode.sql","checksum":"195","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"204_channel_monitor_hide_throughput.sql","checksum":"204","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"206_channel_monitor_v2_privacy_defaults.sql","checksum":"206","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"226_channel_monitor_quota_mode.sql","checksum":"226","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"230_plugin_artifacts.sql","checksum":"230","applied_at":"2026-01-01 00:00:00+00"}
  ]'
  before_legacy="$MODELPORT_TEST_LEGACY_LEDGER_JSON"
  after_upstream='[
    {"filename":"195_channel_monitor_mode.sql","checksum":"195","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"204_channel_monitor_hide_throughput.sql","checksum":"204","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"206_channel_monitor_v2_privacy_defaults.sql","checksum":"206","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"226_channel_monitor_quota_mode.sql","checksum":"226","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"230_plugin_artifacts.sql","checksum":"230","applied_at":"2026-01-01 00:00:00+00"},
    {"filename":"232_modelport_free_group_bridge.sql","checksum":"232","applied_at":"2026-01-02 00:00:00+00"},
    {"filename":"233_modelport_lottery_bridge.sql","checksum":"233","applied_at":"2026-01-02 00:00:00+00"},
    {"filename":"234_modelport_instruction_audit_bridge.sql","checksum":"234","applied_at":"2026-01-02 00:00:00+00"},
    {"filename":"235_batch_image_group_snapshot.sql","checksum":"235","applied_at":"2026-01-02 00:00:00+00"},
    {"filename":"236_modelport_legacy_platform_constraints.sql","checksum":"236","applied_at":"2026-01-02 00:00:00+00"}
  ]'
  after_legacy="$(jq -cn \
    --argjson legacy "$before_legacy" \
    --argjson active "$after_upstream" \
    '$legacy + $active')"
  current="$after_upstream"
  sequence_position_valid=true

  if [[ "$phase" -eq 1 ]]; then
    case "$kind" in
      empty) migrations='[]' ;;
      upstream) migrations="$before_upstream" ;;
      legacy-modelport) migrations="$before_legacy" ;;
      production) migrations="$current" ;;
      mismatch) migrations="$(jq -c '.[0].checksum = "invalid"' <<<"$before_legacy")" ;;
      *) exit 92 ;;
    esac
  else
    case "$kind" in
      legacy-modelport|mismatch) migrations="$after_legacy" ;;
      *) migrations="$after_upstream" ;;
    esac
  fi

  audit_seed_present=false
  audit_seed_legacy=false
  if [[ "$phase" -gt 1 || "$kind" == production || "$kind" == legacy-modelport || "$kind" == mismatch ]]; then
    audit_seed_present=true
  fi
  if [[ "$kind" == legacy-modelport || "$kind" == mismatch ]]; then
    audit_seed_legacy=true
  fi
  audit_seed_summaries="$(jq -cn \
    --argjson present "$audit_seed_present" \
    --argjson legacy "$audit_seed_legacy" '
      def fixed($name; $keys): {
        table_name: $name,
        table_exists: $present,
        rows: (if $present then ($keys | map({key: ., fingerprint: ("a" * 64), canonical: true})) else [] end)
      };
      {
        instruction_audit_state: fixed("instruction_audit_state"; ["1"]),
        instruction_audit_reason_policies: fixed("instruction_audit_reason_policies"; [
          "hash_mismatch", "fields_missing", "field_invalid", "invalid_json",
          "request_too_large", "structure_too_complex", "parse_timeout",
          "config_unavailable", "group_not_allowed", "client_not_allowed",
          "ai_rejected", "ai_uncertain", "ai_error"
        ]),
        instruction_audit_runtime_config: fixed("instruction_audit_runtime_config"; ["1"]),
        instruction_audit_outcome_rollup_state: fixed("instruction_audit_outcome_rollup_state"; ["1"]),
        instruction_audit_operational_counters: fixed("instruction_audit_operational_counters"; ["1"]),
        instruction_audit_v2_config: fixed("instruction_audit_v2_config"; ["1"]),
        instruction_audit_v2_client_profiles: fixed("instruction_audit_v2_client_profiles"; [
          "codex_vscode", "codex_cli", "codex_desktop", "opencode",
          "modelport_internal", "other", "unknown"
        ]),
        instruction_audit_sensitive_access_grants: {
          table_name: "instruction_audit_sensitive_access_grants",
          table_exists: $present,
          rows: (if $present then [{
            key: "9001",
            fingerprint: (if $legacy then ("b" * 64) else ("c" * 64) end),
            canonical: (if $legacy then false else true end)
          }] else [] end)
        }
      }
    ')"

  if [[ "$phase" -eq 1 ]]; then
    case "$kind" in
      empty) settings='{}' ;;
      upstream)
        settings="{\"site_name\":\"$hash_site\",\"channel_monitor_mode\":\"$hash_v1\",\"channel_monitor_hide_throughput\":\"$hash_true\",\"channel_monitor_show_quota\":\"$hash_false\"}"
        ;;
      legacy-modelport|mismatch) settings="{\"site_name\":\"$hash_site\"}" ;;
      production)
        settings="{\"site_name\":\"$hash_site\",\"channel_monitor_mode\":\"$hash_v1\",\"channel_monitor_hide_throughput\":\"$hash_true\",\"channel_monitor_show_quota\":\"$hash_false\",\"instruction_audit_enabled\":\"$hash_false\",\"instruction_audit_evidence_retention_days\":\"$hash_30\"}"
        ;;
    esac
  else
    if [[ "$kind" == empty ]]; then
      settings="{\"full_chain_default\":\"$hash_site\",\"channel_monitor_mode\":\"$hash_v1\",\"channel_monitor_hide_throughput\":\"$hash_true\",\"channel_monitor_show_quota\":\"$hash_false\",\"instruction_audit_enabled\":\"$hash_false\",\"instruction_audit_evidence_retention_days\":\"$hash_30\"}"
    else
      settings="{\"site_name\":\"$hash_site\",\"channel_monitor_mode\":\"$hash_v1\",\"channel_monitor_hide_throughput\":\"$hash_true\",\"channel_monitor_show_quota\":\"$hash_false\",\"instruction_audit_enabled\":\"$hash_false\",\"instruction_audit_evidence_retention_days\":\"$hash_30\"}"
    fi
  fi

  if [[ "$phase" -eq 1 && "$kind" == empty ]]; then
    key_counts='{
      "users":{"exists":false,"rows":null},"groups":{"exists":false,"rows":null},
      "api_keys":{"exists":false,"rows":null},"accounts":{"exists":false,"rows":null},
      "usage_logs":{"exists":false,"rows":null}
    }'
    all_counts='{}'
  else
    key_counts='{
      "users":{"exists":true,"rows":3},"groups":{"exists":true,"rows":2},
      "api_keys":{"exists":true,"rows":4},"accounts":{"exists":true,"rows":2},
      "channels":{"exists":true,"rows":1},"usage_logs":{"exists":true,"rows":5},
      "payment_orders":{"exists":true,"rows":2},"user_subscriptions":{"exists":true,"rows":1},
      "lottery_campaigns":{"exists":true,"rows":0},"lottery_prizes":{"exists":true,"rows":0},
      "lottery_entries":{"exists":true,"rows":0},"lottery_draw_runs":{"exists":true,"rows":0},
      "lottery_events":{"exists":true,"rows":0},"batch_image_jobs":{"exists":true,"rows":0}
    }'
    all_counts="$(jq -cn --argjson settings "$settings" '{users:3,groups:2,api_keys:4,accounts:2,usage_logs:5,payment_orders:2,user_subscriptions:1,settings:($settings|length)}')"
    if [[ "$phase" -gt 1 || "$kind" == production || "$kind" == legacy-modelport || "$kind" == mismatch ]]; then
      all_counts="$(jq -cn \
        --argjson base "$all_counts" \
        --argjson required "$MODELPORT_TEST_REQUIRED_BRIDGE_TABLES_JSON" '
          reduce $required[] as $table ($base; .[$table] = (.[$table] // 0))
        ')"
    fi
    if [[ "$audit_seed_present" == true ]]; then
      all_counts="$(jq -cn \
        --argjson base "$all_counts" \
        --argjson seeds "$audit_seed_summaries" '
          reduce ($seeds | to_entries[]) as $seed ($base;
            .[$seed.key] = ($seed.value.rows | length)
          )
        ')"
    fi
  fi

  finance='{
    "users.balance":{"table_exists":true,"column_exists":true,"non_null_rows":3,"sum":"12.50000000"},
    "users.frozen_balance":{"table_exists":true,"column_exists":true,"non_null_rows":3,"sum":"1.00000000"},
    "users.total_recharged":{"table_exists":true,"column_exists":true,"non_null_rows":3,"sum":"30.00000000"},
    "payment_orders.amount":{"table_exists":true,"column_exists":true,"non_null_rows":2,"sum":"20.00"},
    "payment_orders.pay_amount":{"table_exists":true,"column_exists":true,"non_null_rows":2,"sum":"20.00"},
    "payment_orders.refund_amount":{"table_exists":true,"column_exists":true,"non_null_rows":2,"sum":"0.00"},
    "usage_logs.total_cost":{"table_exists":true,"column_exists":true,"non_null_rows":5,"sum":"2.5000000000"},
    "usage_logs.actual_cost":{"table_exists":true,"column_exists":true,"non_null_rows":5,"sum":"2.0000000000"},
    "api_keys.quota":{"table_exists":true,"column_exists":true,"non_null_rows":4,"sum":"100.00000000"},
    "api_keys.quota_used":{"table_exists":true,"column_exists":true,"non_null_rows":4,"sum":"2.00000000"}
  }'
  ciphertext='{
    "users.totp_secret_encrypted":{
      "table":"users","column":"totp_secret_encrypted","type":"text",
      "primary_key_columns":["id"],"digest_binding":"primary-key-and-ciphertext",
      "rows_present":1,"total_bytes":"48",
      "digest_sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
    }
  }'
  if [[ "$MODELPORT_TEST_FAILURE" == invariant && "$phase" -eq 3 ]]; then
    all_counts="$(jq -c '.users = 4' <<<"$all_counts")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == audit_seed_extra && "$phase" -eq 3 ]]; then
    all_counts="$(jq -c '.instruction_audit_events = 1' <<<"$all_counts")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == audit_seed_unknown && "$phase" -eq 3 ]]; then
    audit_seed_summaries="$(jq -c '
      .instruction_audit_reason_policies.rows += [{key:"future_reason",fingerprint:("d" * 64),canonical:false}]
    ' <<<"$audit_seed_summaries")"
    all_counts="$(jq -c '.instruction_audit_reason_policies += 1' <<<"$all_counts")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == audit_seed_changed && "$phase" -eq 3 ]]; then
    audit_seed_summaries="$(jq -c '
      .instruction_audit_state.rows[0].fingerprint = ("e" * 64)
    ' <<<"$audit_seed_summaries")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == audit_seed_missing && "$phase" -eq 3 ]]; then
    audit_seed_summaries="$(jq -c '
      .instruction_audit_v2_client_profiles.rows |= map(select(.key != "unknown"))
    ' <<<"$audit_seed_summaries")"
    all_counts="$(jq -c '.instruction_audit_v2_client_profiles -= 1' <<<"$all_counts")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == bridge_object && "$phase" -eq 3 ]]; then
    missing_table="$(jq -r '.[0]' <<<"$MODELPORT_TEST_REQUIRED_BRIDGE_TABLES_JSON")"
    all_counts="$(jq -c --arg table "$missing_table" 'del(.[$table])' <<<"$all_counts")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == required_migration && "$phase" -eq 3 ]]; then
    migrations="$(jq -c 'map(select(.filename != "236_modelport_legacy_platform_constraints.sql"))' <<<"$migrations")"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == sequence_invalid && "$phase" -eq 3 ]]; then
    sequence_position_valid=false
  fi

  local constraints indexes
  constraints='[
    {"table":"users","name":"users_pkey","type":"p","validated":true,"definition_sha256":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
    {"table":"usage_logs","name":"usage_logs_image_billing_size_check","type":"c","validated":false,"definition_sha256":"1111111111111111111111111111111111111111111111111111111111111111"},
    {"table":"usage_logs","name":"usage_logs_image_size_source_check","type":"c","validated":false,"definition_sha256":"2222222222222222222222222222222222222222222222222222222222222222"}
  ]'
  indexes='[{"table":"users","name":"users_pkey","unique":true,"primary":true,"valid":true,"ready":true,"definition_sha256":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}]'
  if [[ "$phase" -gt 1 || "$kind" == production ]]; then
    constraints="$(jq -cn \
      --argjson baseline "$constraints" \
      --argjson required "$MODELPORT_TEST_REQUIRED_BRIDGE_CONSTRAINTS_JSON" '
        $baseline + ($required | map({
          table:"modelport_contract", name:., type:"c", validated:true,
          definition_sha256:"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
        }))
      ')"
    indexes="$(jq -cn \
      --argjson baseline "$indexes" \
      --argjson required "$MODELPORT_TEST_REQUIRED_BRIDGE_INDEXES_JSON" '
        $baseline + ($required | map({
          table:"modelport_contract", name:., unique:false, primary:false,
          valid:true, ready:true,
          definition_sha256:"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
        }))
      ')"
  fi
  if [[ "$MODELPORT_TEST_FAILURE" == catalog_invalid && "$phase" -eq 3 ]]; then
    constraints="$(jq -c '. + [{
      table:"users", name:"unexpected_not_valid_check", type:"c", validated:false,
      definition_sha256:"3333333333333333333333333333333333333333333333333333333333333333"
    }]' <<<"$constraints")"
  fi

  jq -cn \
    --argjson migrations "$migrations" \
    --argjson key_counts "$key_counts" \
    --argjson all_counts "$all_counts" \
    --argjson settings "$settings" \
    --argjson audit_seed_summaries "$audit_seed_summaries" \
    --argjson finance "$finance" \
    --argjson constraints "$constraints" \
    --argjson indexes "$indexes" \
    --argjson sequence_position_valid "$sequence_position_valid" \
    --argjson ciphertext "$ciphertext" '
      {
        schema_migrations:$migrations,
        key_table_counts:$key_counts,
        all_table_counts:$all_counts,
        settings_value_summaries:$settings,
        audit_seed_summaries:$audit_seed_summaries,
        financial_aggregates:$finance,
        sequences:[{
          schema:"public",name:"users_id_seq",start:"1",increment:"1",min:"1",
          max:"9223372036854775807",cache:"1",cycle:false,last_value:"3",is_called:true,
          owned_table:"users",owned_column:"id",owned_table_min:"1",owned_table_max:"3",
          next_value:"4",position_valid:$sequence_position_valid
        }],
        constraints:$constraints,
        indexes:$indexes,
        foreign_keys:[{table:"api_keys",name:"api_keys_user_id_fkey",referenced_table:"users",validated:true,definition_sha256:"abababababababababababababababababababababababababababababababab"}],
        ciphertext_summaries:$ciphertext
      }
    '
}

case "${1:-}" in
  context)
    case "${2:-}" in
      show)
        [[ "$MODELPORT_TEST_FAILURE" != docker_context_show ]] || exit 1
        printf '%s\n' modelport-test
        ;;
      inspect)
        if [[ "$MODELPORT_TEST_FAILURE" == docker_context_remote ]]; then
          printf '%s\n' 'tcp://production.example.invalid:2376'
        elif [[ "$MODELPORT_TEST_FAILURE" == docker_context_missing ]]; then
          printf 'unix://%s/missing-docker.sock\n' "$MODELPORT_TEST_STATE"
        else
          printf 'unix://%s\n' "$MODELPORT_TEST_DOCKER_SOCKET"
        fi
        ;;
      *) exit 92 ;;
    esac
    ;;
  image)
    [[ "${2:-}" == inspect ]] || exit 93
    if [[ "$MODELPORT_TEST_FAILURE" == input_swap && ! -f "$MODELPORT_TEST_STATE/input-swapped" ]]; then
      mv -- "$MODELPORT_TEST_SWAP_DUMP_PATH" "$MODELPORT_TEST_SWAP_DUMP_PATH.modelport-test-original"
      printf '%s\n' 'replacement backup bytes' > "$MODELPORT_TEST_SWAP_DUMP_PATH"
      touch "$MODELPORT_TEST_STATE/input-swapped"
    fi
    if [[ "$MODELPORT_TEST_FAILURE" == input_mutate_restore ]]; then
      cp -- "$MODELPORT_TEST_SWAP_DUMP_PATH" "$MODELPORT_TEST_SWAP_DUMP_PATH.modelport-test-mutated-original"
      printf '%s\n' 'transient same-inode replacement backup bytes' > "$MODELPORT_TEST_SWAP_DUMP_PATH"
    fi
    if [[ "$MODELPORT_TEST_FAILURE" == report_parent_swap ]]; then
      rm -f -- "$MODELPORT_TEST_REPORT_SWAP_LINK"
      ln -s "$MODELPORT_TEST_REPORT_SWAP_REDIRECT" "$MODELPORT_TEST_REPORT_SWAP_LINK"
    fi
    printf '%s\n' "$MODELPORT_TEST_IMAGE_ID linux arm64"
    ;;
  network)
    case "${2:-}" in
      create)
        name="$(last_argument "$@")"
        if [[ "$MODELPORT_TEST_FAILURE" == network_collision ]]; then
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
      *) exit 94 ;;
    esac
    ;;
  volume)
    case "${2:-}" in
      create)
        name="$(last_argument "$@")"
        touch "$MODELPORT_TEST_STATE/volume-$name"
        printf '%s\n' "$name"
        ;;
      inspect)
        name="$(last_argument "$@")"
        [[ -e "$MODELPORT_TEST_STATE/volume-$name" ]] || exit 1
        printf '%s\n' "${name##*-}"
        ;;
      rm)
        name="$(last_argument "$@")"
        if [[ "$MODELPORT_TEST_FAILURE" == cleanup_once && ! -f "$MODELPORT_TEST_STATE/cleanup-volume-failed" ]]; then
          touch "$MODELPORT_TEST_STATE/cleanup-volume-failed"
          exit 1
        fi
        rm -f -- "$MODELPORT_TEST_STATE/volume-$name"
        printf '%s\n' "$name"
        ;;
      *) exit 95 ;;
    esac
    ;;
  container)
    [[ "${2:-}" == inspect ]] || exit 95
    name="$(last_argument "$@")"
    [[ -e "$MODELPORT_TEST_STATE/container-$name" ]] || exit 1
    printf '%s\n' "${name##*-}"
    ;;
  create)
    name="$(argument_after --name "$@")"
    touch "$MODELPORT_TEST_STATE/container-$name"
    if [[ "$MODELPORT_TEST_FAILURE" == container_create && "$name" == modelport-restore-pg-* ]]; then
      exit 1
    fi
    printf 'mock-%s\n' "$name"
    ;;
  start)
    name="$(last_argument "$@")"
    if [[ "$name" == modelport-restore-space-* ]]; then
      printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
      printf 'mockfs 104857600 1 104857599 1%% /var/lib/postgresql/data\n'
    elif [[ "$MODELPORT_TEST_FAILURE" == container_start ]]; then
      exit 1
    else
      printf '%s\n' "$name"
    fi
    ;;
  inspect)
    if [[ "$MODELPORT_TEST_FAILURE" == health ]]; then
      printf '%s\n' unhealthy
    else
      printf '%s\n' healthy
    fi
    ;;
  port)
    printf '%s\n' '127.0.0.1:55432'
    ;;
  rm)
    name="$(last_argument "$@")"
    rm -f -- "$MODELPORT_TEST_STATE/container-$name"
    printf '%s\n' "$name"
    ;;
  exec)
    joined="$*"
    if [[ "$joined" == *'pg_restore --version'* ]]; then
      printf '%s\n' 'pg_restore (PostgreSQL) 18.1'
    elif [[ "$joined" == *'psql --version'* ]]; then
      printf '%s\n' 'psql (PostgreSQL) 18.1'
    elif [[ "$joined" == *'SHOW server_version'* ]]; then
      printf '%s\n' '18.1'
    elif [[ "$joined" == *'pg_restore --list'* ]]; then
      cat >/dev/null
      [[ "$MODELPORT_TEST_FAILURE" != archive ]] || exit 1
      printf '%s\n' '; Archive created by pg_dump 18.1'
    elif [[ "$joined" == *'pg_restore --exit-on-error'* || "$joined" == *'psql -X -v ON_ERROR_STOP=1 --single-transaction'* ]]; then
      cat > "$MODELPORT_TEST_STATE/restore-input"
      if [[ "$MODELPORT_TEST_FAILURE" == input_mutate_restore ]]; then
        cp -- "$MODELPORT_TEST_SWAP_DUMP_PATH.modelport-test-mutated-original" "$MODELPORT_TEST_SWAP_DUMP_PATH"
      fi
      [[ "$MODELPORT_TEST_FAILURE" != restore ]] || exit 1
    elif [[ "$joined" == *'cat > /tmp/modelport-restore-migrate'* ]]; then
      cat > "$MODELPORT_TEST_STATE/copied-migration-binary"
      [[ "$MODELPORT_TEST_FAILURE" != migration_copy ]] || exit 1
    elif [[ "$joined" == *'sha256sum /tmp/modelport-restore-migrate'* ]]; then
      [[ "$MODELPORT_TEST_FAILURE" != migration_hash ]] || exit 1
      if [[ "$MODELPORT_TEST_FAILURE" == migration_hash_mismatch ]]; then
        printf '%064d  %s\n' 0 /tmp/modelport-restore-migrate
        exit 0
      fi
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$MODELPORT_TEST_STATE/copied-migration-binary"
      else
        shasum -a 256 "$MODELPORT_TEST_STATE/copied-migration-binary"
      fi
    elif [[ "$joined" == *'exec /tmp/modelport-restore-migrate'* ]]; then
      url="$(cat)"
      [[ "$url" =~ ^postgresql://modelport_restore_user_[0-9a-f]{32}:[0-9a-f]{48}@127\.0\.0\.1:5432/modelport_restore_[0-9a-f]{32}\?sslmode=disable$ ]] || exit 97
      count_file="$MODELPORT_TEST_STATE/migration-count"
      count=0
      [[ ! -f "$count_file" ]] || count="$(<"$count_file")"
      count=$((count + 1))
      printf '%s\n' "$count" > "$count_file"
      if [[ "$MODELPORT_TEST_FAILURE" == migration_first && "$count" -eq 1 ]]; then
        exit 1
      fi
      if [[ "$MODELPORT_TEST_FAILURE" == migration_second && "$count" -eq 2 ]]; then
        exit 1
      fi
      printf 'migration-%s-ok\n' "$count" >> "$MODELPORT_TEST_STATE/migration-events"
    elif [[ "$joined" == *"sh -c umask"* ]]; then
      cat >/dev/null
    elif [[ "$joined" == *' psql '* ]]; then
      payload="$(cat)"
      if [[ "$payload" == *MODELPORT_REPORT_SNAPSHOT_V1* ]]; then
        count_file="$MODELPORT_TEST_STATE/snapshot-count"
        count=0
        [[ ! -f "$count_file" ]] || count="$(<"$count_file")"
        count=$((count + 1))
        printf '%s\n' "$count" > "$count_file"
        if [[ "$MODELPORT_TEST_FAILURE" == metadata && "$count" -eq 1 ]]; then
          exit 1
        fi
        emit_snapshot "$count"
      elif [[ "$payload" == *MODELPORT_BRIDGE_CONTRACT_V1* ]]; then
        if [[ "$MODELPORT_TEST_FAILURE" == bridge_contract ]]; then
          printf '%s\n' '{
            "232_modelport_free_group_bridge":true,
            "233_modelport_lottery_bridge":true,
            "234_modelport_instruction_audit_bridge":false,
            "235_batch_image_group_snapshot":true,
            "236_modelport_legacy_platform_constraints":true
          }'
        else
          printf '%s\n' '{
            "232_modelport_free_group_bridge":true,
            "233_modelport_lottery_bridge":true,
            "234_modelport_instruction_audit_bridge":true,
            "235_batch_image_group_snapshot":true,
            "236_modelport_legacy_platform_constraints":true
          }'
        fi
      fi
    else
      cat >/dev/null || true
    fi
    ;;
  *) exit 96 ;;
esac
MOCK_DOCKER

cat > "$bin_dir/go" <<'MOCK_GO'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  version)
    printf '%s\n' 'go version go1.27.0 darwin/arm64'
    ;;
  run)
    url="${MODELPORT_RESTORE_DATABASE_URL:-}"
    [[ "$url" =~ ^postgresql://modelport_restore_user_[0-9a-f]{32}:[0-9a-f]{48}@127\.0\.0\.1:55432/modelport_restore_[0-9a-f]{32}\?sslmode=disable$ ]] || exit 97
    count_file="$MODELPORT_TEST_STATE/migration-count"
    count=0
    [[ ! -f "$count_file" ]] || count="$(<"$count_file")"
    count=$((count + 1))
    printf '%s\n' "$count" > "$count_file"
    if [[ "$MODELPORT_TEST_FAILURE" == migration_first && "$count" -eq 1 ]]; then
      exit 1
    fi
    if [[ "$MODELPORT_TEST_FAILURE" == migration_second && "$count" -eq 2 ]]; then
      exit 1
    fi
    printf 'migration-%s-ok\n' "$count" >> "$MODELPORT_TEST_STATE/migration-events"
    ;;
  build)
    [[ "$MODELPORT_TEST_FAILURE" != migration_build ]] || exit 1
    build_directory="$(pwd -P)"
    if [[ "$MODELPORT_TEST_FIXTURE_KIND" == production ]]; then
      [[ "$build_directory" == */.modelport-isolated-restore-work.*/source-snapshot/backend ]] || exit 98
      [[ "${GOTOOLCHAIN:-}" == local ]] || exit 98
      [[ "${GOPROXY:-}" == off ]] || exit 98
      [[ "${GOSUMDB:-}" == off ]] || exit 98
      [[ "${GOWORK:-}" == off ]] || exit 98
      touch "$MODELPORT_TEST_STATE/source-snapshot-build"
    else
      [[ "$build_directory" == "$MODELPORT_TEST_EXPECTED_BACKEND" ]] || exit 98
    fi
    [[ "${CGO_ENABLED:-}" == 0 ]] || exit 98
    [[ "${GOOS:-}" == linux ]] || exit 98
    [[ "${GOARCH:-}" == arm64 ]] || exit 98
    [[ " $* " == *' -mod=readonly '* ]] || exit 98
    [[ " $* " == *' -trimpath '* ]] || exit 98
    target=''
    for argument in "$@"; do
      target="$argument"
    done
    [[ "$target" == ./cmd/modelport-restore-migrate ]] || exit 98
    output=''
    shift
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == -o ]]; then
        output="$2"
        break
      fi
      shift
    done
    [[ -n "$output" ]] || exit 98
    printf '%s\n' 'mock migration binary' > "$output"
    ;;
  test)
    ;;
  *) exit 98 ;;
esac
MOCK_GO

cat > "$bin_dir/chmod" <<'MOCK_CHMOD'
#!/usr/bin/env bash
set -euo pipefail
target=''
for argument in "$@"; do
  target="$argument"
done
if [[ "$MODELPORT_TEST_FAILURE" == migration_chmod && "${target##*/}" == modelport-restore-migrate ]]; then
  exit 1
fi
exec /bin/chmod "$@"
MOCK_CHMOD

cat > "$bin_dir/rm" <<'MOCK_RM'
#!/usr/bin/env bash
set -euo pipefail
target=''
for argument in "$@"; do
  [[ "$argument" == -- ]] || target="$argument"
done
if [[ "$MODELPORT_TEST_FAILURE" == local_cleanup && " $* " == *' -rf '* && \
      "$target" == */.modelport-isolated-restore-work.* && \
      ! -f "$MODELPORT_TEST_STATE/local-cleanup-failed" ]]; then
  printf '%s' "$target" > "$MODELPORT_TEST_STATE/local-cleanup-target"
  if [[ -n "${MODELPORT_TEST_EXPECT_REPORT:-}" && -e "$MODELPORT_TEST_EXPECT_REPORT" ]]; then
    touch "$MODELPORT_TEST_STATE/passed-report-visible-before-local-cleanup"
  fi
  touch "$MODELPORT_TEST_STATE/local-cleanup-failed"
  exit 1
fi
exec /bin/rm "$@"
MOCK_RM

cat > "$bin_dir/sha256sum" <<'MOCK_SHA256SUM'
#!/usr/bin/env bash
set -euo pipefail
target=''
for argument in "$@"; do
  [[ "$argument" == -- ]] || target="$argument"
done
if [[ "${target##*/}" == modelport-restore-migrate ]]; then
  case "$MODELPORT_TEST_FAILURE" in
    migration_host_hash) exit 1 ;;
    migration_host_hash_invalid)
      printf '%s  %s\n' invalid-sha256 "$target"
      exit 0
      ;;
  esac
fi
if [[ -x /usr/bin/sha256sum ]]; then
  exec /usr/bin/sha256sum "$@"
fi
exec /usr/bin/shasum -a 256 "$@"
MOCK_SHA256SUM

cat > "$bin_dir/git" <<'MOCK_GIT'
#!/usr/bin/env bash
set -euo pipefail
joined="$*"
case "$joined" in
  *'rev-parse HEAD^{tree}'*) printf '%s\n' "$MODELPORT_TEST_TREE_SHA" ;;
  *'rev-parse HEAD'*) printf '%s\n' "$MODELPORT_TEST_REVISION" ;;
  *'archive --format=tar'*)
    [[ "$joined" == *" $MODELPORT_TEST_REVISION" ]] || exit 99
    tar -cf - -C "$MODELPORT_TEST_ARCHIVE_SOURCE" \
      backend/cmd/modelport-restore-migrate backend/migrations
    ;;
  *'status --porcelain=v1 --untracked-files=normal'*)
    [[ "$MODELPORT_TEST_GIT_DIRTY" != true ]] || printf '%s\n' ' M tracked-file'
    ;;
  *) exit 99 ;;
esac
MOCK_GIT

chmod 0755 "$bin_dir/docker" "$bin_dir/go" "$bin_dir/chmod" "$bin_dir/rm" \
  "$bin_dir/sha256sum" "$bin_dir/git"

plain_dump="$external_dir/upstream.sql"
gzip_dump="$external_dir/legacy.sql.gz"
custom_dump="$external_dir/production.dump"
custom_gzip_dump="$external_dir/upstream.dump.gz"
printf '%s\n' '-- PostgreSQL plain dump fixture' "-- $fixture_secret" > "$plain_dump"
gzip -c -- "$plain_dump" > "$gzip_dump"
printf 'PGDMP%s\n' "$fixture_secret" > "$custom_dump"
gzip -c -- "$custom_dump" > "$custom_gzip_dump"

reset_run() {
  rm -f -- "$docker_log" "$stdout_log" "$stderr_log" \
    "$state_dir/snapshot-count" "$state_dir/migration-count" "$state_dir/migration-events"
  find "$state_dir" -maxdepth 1 -type f ! -name unowned-sentinel -delete
}

run_script() {
  local fixture_kind="$1"
  local failure_mode="$2"
  shift 2
  env \
    PATH="$bin_dir:$PATH" \
    MODELPORT_TEST_DOCKER_LOG="$docker_log" \
    MODELPORT_TEST_DOCKER_SOCKET="$docker_socket" \
    MODELPORT_TEST_STATE="$state_dir" \
    MODELPORT_TEST_IMAGE_ID="$image_id" \
    MODELPORT_TEST_FIXTURE_KIND="$fixture_kind" \
    MODELPORT_TEST_FAILURE="$failure_mode" \
    MODELPORT_TEST_REVISION="$revision" \
    MODELPORT_TEST_TREE_SHA="$tree_sha" \
    MODELPORT_TEST_GIT_DIRTY=false \
    MODELPORT_TEST_LEGACY_LEDGER_JSON="$legacy_ledger_json" \
    MODELPORT_TEST_REQUIRED_BRIDGE_TABLES_JSON="$required_bridge_tables_json" \
    MODELPORT_TEST_REQUIRED_BRIDGE_CONSTRAINTS_JSON="$required_bridge_constraints_json" \
    MODELPORT_TEST_REQUIRED_BRIDGE_INDEXES_JSON="$required_bridge_indexes_json" \
    MODELPORT_TEST_EXPECTED_BACKEND="$repo_root/backend" \
    MODELPORT_TEST_ARCHIVE_SOURCE="$repo_root" \
    MODELPORT_TEST_SWAP_DUMP_PATH="${MODELPORT_TEST_SWAP_DUMP_PATH:-}" \
    MODELPORT_TEST_REPORT_SWAP_LINK="$report_swap_link" \
    MODELPORT_TEST_REPORT_SWAP_REDIRECT="$report_swap_redirect" \
    MODELPORT_TEST_EXPECT_REPORT="${MODELPORT_TEST_EXPECT_REPORT:-}" \
    DOCKER_HOST="${MODELPORT_TEST_DOCKER_HOST:-}" \
    /bin/bash "$script" "$@" >"$stdout_log" 2>"$stderr_log"
}

assert_cleanup() {
  [[ -f "$state_dir/unowned-sentinel" ]] || fail 'cleanup removed the unowned sentinel'
  if find "$state_dir" -maxdepth 1 -type f \( -name 'container-*' -o -name 'network-*' -o -name 'volume-*' \) | grep -q .; then
    find "$state_dir" -maxdepth 1 -type f >&2
    fail 'an owned Docker resource remained after the run'
  fi
}

expect_usage_failure() {
  reset_run
  if run_script upstream none "$@"; then
    fail "invalid arguments were accepted: $*"
  fi
  [[ ! -s "$docker_log" ]] || fail 'Docker was called before invalid arguments were rejected'
}

expect_reported_failure() {
  local expected_code="$1"
  local fixture_kind="$2"
  local failure_mode="$3"
  local report="$4"
  shift 4
  reset_run
  rm -f -- "$report"
  if run_script "$fixture_kind" "$failure_mode" "$@" --report "$report"; then
    fail "failure mode $failure_mode unexpectedly passed"
  fi
  jq -e --arg code "$expected_code" '.result == "failed" and .failure_code == $code' "$report" >/dev/null || \
    fail "failure mode $failure_mode did not emit failure_code=$expected_code"
  assert_cleanup
}

missing="$external_dir/missing.json"
expect_usage_failure
expect_usage_failure --source-kind empty --dump "$plain_dump" --report "$missing"
expect_usage_failure --source-kind upstream --report "$missing"
expect_usage_failure --source-kind upstream --dump "$plain_dump" --report relative.json
expect_usage_failure --source-kind upstream --dump "$plain_dump" --report "$missing" --dsn 'postgresql://production.invalid/db'
expect_usage_failure --source-kind upstream --dump "$plain_dump" --report "$missing" --postgres-image postgres:18-alpine
expect_usage_failure --source-kind production --dump "$custom_dump" --report "$missing"
expect_usage_failure --source-kind production --dump "$custom_dump" --expected-postgres-major invalid --report "$missing"

existing_report="$external_dir/existing.json"
printf '%s\n' preserved > "$existing_report"
expect_usage_failure --source-kind upstream --dump "$plain_dump" --report "$existing_report"
[[ "$(<"$existing_report")" == preserved ]] || fail 'existing report was modified'
symlink_report="$external_dir/report-link.json"
ln -s "$existing_report" "$symlink_report"
expect_usage_failure --source-kind upstream --dump "$plain_dump" --report "$symlink_report"
[[ "$(<"$existing_report")" == preserved ]] || fail 'report symlink target was modified'

repo_parent_link="$test_root/repo-parent-link"
ln -s "$repo_root/deploy" "$repo_parent_link"
repo_dump_sha="$(sha256_file "$repo_parent_link/modelport-isolated-restore.sh")"
expect_usage_failure \
  --source-kind production \
  --dump "$repo_parent_link/modelport-isolated-restore.sh" \
  --expected-sha256 "$repo_dump_sha" \
  --expected-revision "$revision" \
  --expected-postgres-major 18 \
  --confirm-production-storage \
  --postgres-image "$image_ref" \
  --report "$external_dir/repo-dump-rejected.json"

report_parent_link="$test_root/report-parent-link"
ln -s "$repo_root/deploy" "$report_parent_link"
production_sha="$(sha256_file "$custom_dump")"
expect_usage_failure \
  --source-kind production \
  --dump "$custom_dump" \
  --expected-sha256 "$production_sha" \
  --expected-revision "$revision" \
  --expected-postgres-major 18 \
  --confirm-production-storage \
  --postgres-image "$image_ref" \
  --report "$report_parent_link/.restore-report-$PPID.json"

run_success() {
  local fixture_kind="$1"
  local dump_path="$2"
  local expected_format="$3"
  local expected_method="$4"
  local report="$5"
  shift 5
  local input_sha=''
  reset_run
  rm -f -- "$report"
  if [[ -n "$dump_path" ]]; then
    input_sha="$(sha256_file "$dump_path")"
    run_script "$fixture_kind" none \
      --source-kind "$fixture_kind" --dump "$dump_path" --expected-sha256 "$input_sha" \
      --postgres-image "$image_ref" --report "$report" "$@"
  else
    run_script empty none --source-kind empty --postgres-image "$image_ref" --report "$report" "$@"
  fi
  jq -e \
    --arg format "$expected_format" \
    --arg method "$expected_method" \
    --arg input_sha "$input_sha" '
      .result == "passed" and .failure_code == null and
      .input.format == $format and .restore.method == $method and
      (if $input_sha == "" then
         .input.sha256 == null and .input.verified_copy_sha256 == null and
         .restore.verified_copy_used == false
       else
         .input.sha256 == $input_sha and .input.verified_copy_sha256 == $input_sha and
         .restore.verified_copy_used == true
       end) and
      (.runtime.postgres_image.reference | length > 0)
    ' "$report" >/dev/null || fail "success report failed its basic contract: $report"
  jq -e '
    .runtime.postgres_image.id == "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" and
    .runtime.postgres_server_version == "18.1" and
    .runtime.postgres_server_major == 18 and
    .runtime.migration_runs == 2 and
    (.runtime.migration_tool.binary_sha256 | test("^[0-9a-f]{64}$")) and
    .runtime.migration_tool.execution == "postgres-container-loopback" and
    (.runtime.migration_tool.source == "working-tree" or
     .runtime.migration_tool.source == "git-archive") and
    .runtime.isolation.host_port_published == false and
    .runtime.repository.commit == "0123456789abcdef0123456789abcdef01234567" and
    .runtime.repository.tree_sha == "89abcdef0123456789abcdef0123456789abcdef" and
    .runtime.repository.unchanged_during_run == true and
    (.before.schema_migrations | type == "array") and
    (.after_first_migration.all_table_counts | type == "object") and
    (.after.financial_aggregates | type == "object") and
    (.after.sequences | type == "array") and
    (.after.constraints | type == "array") and
    (.after.indexes | type == "array") and
    (.after.foreign_keys | type == "array") and
    (.after.bridge_contract | to_entries | all(.[]; .value == true)) and
    (.before | has("settings_value_summaries") | not) and
    (.after_first_migration | has("settings_value_summaries") | not) and
    (.after | has("settings_value_summaries") | not) and
    (.after.ciphertext_summaries["users.totp_secret_encrypted"].digest_binding == "primary-key-and-ciphertext") and
    ([.invariants[].passed] | all) and
    ([.after.schema_migrations[].filename] | index("232_modelport_free_group_bridge.sql") != null) and
    ([.after.schema_migrations[].filename] | index("233_modelport_lottery_bridge.sql") != null) and
    ([.after.schema_migrations[].filename] | index("234_modelport_instruction_audit_bridge.sql") != null) and
    ([.after.schema_migrations[].filename] | index("235_batch_image_group_snapshot.sql") != null) and
    ([.after.schema_migrations[].filename] | index("236_modelport_legacy_platform_constraints.sql") != null)
  ' "$report" >/dev/null || fail "success report omitted required evidence: $report"
  [[ "$(file_mode "$report")" == 600 ]] || fail 'report mode was not 0600'
  if find "$(dirname -- "$report")" -maxdepth 1 -name '.modelport-isolated-restore-report.*' | grep -q .; then
    fail 'an atomic report temporary file was left behind'
  fi
  if find "$(dirname -- "$report")" -maxdepth 1 -name '.modelport-isolated-restore-work.*' | grep -q .; then
    fail 'the encrypted report parent retained a restore work directory'
  fi
  [[ "$(<"$state_dir/migration-count")" == 2 ]] || fail 'application migrations did not run exactly twice'
  assert_cleanup
  for checked_file in "$report" "$stdout_log" "$stderr_log" "$docker_log"; do
    assert_not_contains "$fixture_secret" "$checked_file"
    assert_not_contains DockerRootDir "$checked_file"
  done
}

upstream_report="$external_dir/upstream-report.json"
run_success upstream "$plain_dump" plain-sql psql-single-transaction "$upstream_report"
assert_contains 'network create --internal' "$docker_log"
assert_contains '--label com.abingooo.modelport.restore-run=' "$docker_log"
assert_not_contains '--publish' "$docker_log"
assert_contains '--pull=never' "$docker_log"
assert_contains '--read-only' "$docker_log"
assert_contains '--cap-drop ALL' "$docker_log"
assert_contains '--security-opt no-new-privileges' "$docker_log"
assert_contains 'dst=/var/lib/postgresql' "$docker_log"
assert_contains '--tmpfs /tmp:rw\,exec\,nosuid\,nodev\,uid=70\,gid=70\,mode=0700' "$docker_log"
assert_not_contains '--tmpfs /tmp:rw\,nosuid\,nodev\,noexec' "$docker_log"
assert_contains '--health-cmd pg_isready\ -h\ 127.0.0.1\ -p\ 5432\ -U\ modelport_admin_' "$docker_log"
assert_contains 'rm --force --volumes' "$docker_log"
assert_contains 'cat\ \>\ /tmp/modelport-restore-migrate' "$docker_log"
assert_contains 'sha256sum /tmp/modelport-restore-migrate' "$docker_log"
assert_contains 'exec /tmp/modelport-restore-migrate' "$docker_log"
assert_contains '-h 127.0.0.1 -p 5432' "$docker_log"
assert_contains '-d modelport_restore_' "$docker_log"
assert_contains 'psql -X -v ON_ERROR_STOP=1 --single-transaction' "$docker_log"
assert_contains 'volume create --label com.abingooo.modelport.purpose=isolated-restore' "$docker_log"
assert_not_contains '--network host' "$docker_log"
assert_not_contains "$plain_dump" "$docker_log"

# The restore consumes the already-verified private copy even if the source is
# modified in place and restored before the final source check.
stable_copy_report="$external_dir/stable-copy-report.json"
stable_copy_sha="$(sha256_file "$plain_dump")"
reset_run
MODELPORT_TEST_SWAP_DUMP_PATH="$plain_dump" \
  run_script upstream input_mutate_restore \
    --source-kind upstream --dump "$plain_dump" --expected-sha256 "$stable_copy_sha" \
    --postgres-image "$image_ref" --report "$stable_copy_report"
jq -e --arg sha "$stable_copy_sha" '
  .result == "passed" and
  .input.sha256 == $sha and .input.verified_copy_sha256 == $sha and
  .restore.verified_copy_used == true and
  (.invariants | any(.name == "verified_restore_copy_unchanged" and .passed == true))
' "$stable_copy_report" >/dev/null || fail 'the stable-copy mutation drill produced invalid evidence'
[[ "$(sha256_file "$state_dir/restore-input")" == "$stable_copy_sha" ]] || \
  fail 'the restore consumed transiently modified source bytes instead of the verified copy'
[[ "$(sha256_file "$plain_dump")" == "$stable_copy_sha" ]] || \
  fail 'the mutation drill did not restore the original source fixture in place'
rm -f -- "$plain_dump.modelport-test-mutated-original"
assert_cleanup

# Bind publication to the physical directory, not to a symlink ancestor that
# can be retargeted after argument validation.
bound_report_dir="$test_root/bound-report-parent"
redirect_report_dir="$test_root/redirect-report-parent"
report_parent_link="$test_root/report-parent-link-external"
mkdir "$bound_report_dir" "$redirect_report_dir"
ln -s "$bound_report_dir" "$report_parent_link"
report_swap_link="$report_parent_link"
report_swap_redirect="$redirect_report_dir"
bound_report="$bound_report_dir/bound-report.json"
reset_run
run_script upstream report_parent_swap \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$stable_copy_sha" \
  --postgres-image "$image_ref" --report "$report_parent_link/bound-report.json"
report_swap_link=''
report_swap_redirect=''
[[ -f "$bound_report" ]] || fail 'the bound physical report parent did not receive the report'
[[ ! -e "$redirect_report_dir/bound-report.json" ]] || \
  fail 'the report followed a retargeted symlink ancestor'
jq -e '.result == "passed"' "$bound_report" >/dev/null || \
  fail 'the report-parent binding run produced an invalid report'
assert_contains "$bound_report" "$stdout_log"
assert_cleanup

legacy_report="$external_dir/legacy-report.json"
run_success legacy-modelport "$gzip_dump" plain-sql-gzip psql-single-transaction "$legacy_report"
jq -e '.detected_lineage == "modelport-custom-v0.1.176.2"' "$legacy_report" >/dev/null || \
  fail 'legacy lineage was not recorded'

custom_gzip_report="$external_dir/custom-gzip-report.json"
run_success upstream "$custom_gzip_dump" postgres-custom-gzip pg_restore-single-transaction "$custom_gzip_report"
assert_contains 'pg_restore --list' "$docker_log"
assert_contains 'pg_restore --exit-on-error --single-transaction --clean --if-exists --no-owner --no-acl' "$docker_log"

empty_report="$external_dir/empty-report.json"
run_success empty '' none none "$empty_report"

production_report="$external_dir/production-report.json"
reset_run
run_script production none \
  --source-kind production \
  --dump "$custom_dump" \
  --expected-sha256 "$production_sha" \
  --expected-revision "$revision" \
  --expected-postgres-major 18 \
  --confirm-production-storage \
  --postgres-image "$image_ref" \
  --report "$production_report"
jq -e '
  .result == "passed" and .source_kind == "production" and
  .runtime.migration_tool.source == "git-archive" and
  .runtime.expected_postgres_major == 18 and
  .runtime.postgres_server_major == 18 and
  .runtime.repository.expected_revision == "0123456789abcdef0123456789abcdef01234567" and
  .runtime.repository.dirty_at_start == false and
  .runtime.repository.dirty_at_end == false and
  .runtime.isolation.production_storage_confirmed == true
' "$production_report" >/dev/null || fail 'production report lacked repository/storage evidence'
[[ -f "$state_dir/source-snapshot-build" ]] || \
  fail 'the production migration binary was not built from the expected-revision archive'
assert_cleanup

expect_reported_failure postgres_major_mismatch production none "$external_dir/postgres-major-mismatch.json" \
  --source-kind production --dump "$custom_dump" --expected-sha256 "$production_sha" \
  --expected-revision "$revision" --expected-postgres-major 17 --confirm-production-storage \
  --postgres-image "$image_ref"

MODELPORT_TEST_DOCKER_HOST='tcp://production.example.invalid:2376' \
  expect_reported_failure docker_endpoint_invalid upstream none "$external_dir/docker-host-rejected.json" \
    --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" \
    --postgres-image "$image_ref"
[[ ! -s "$docker_log" ]] || fail 'Docker was called when DOCKER_HOST was set'

expect_reported_failure docker_endpoint_invalid upstream docker_context_remote "$external_dir/remote-context-rejected.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" \
  --postgres-image "$image_ref"
assert_not_contains 'image inspect' "$docker_log"

expect_reported_failure docker_endpoint_invalid upstream docker_context_missing "$external_dir/missing-socket-rejected.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" \
  --postgres-image "$image_ref"
assert_not_contains 'image inspect' "$docker_log"

bad_hash_report="$external_dir/bad-hash-report.json"
expect_reported_failure input_checksum_mismatch upstream none "$bad_hash_report" \
  --source-kind upstream --dump "$plain_dump" \
  --expected-sha256 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff' \
  --postgres-image "$image_ref"
[[ ! -s "$docker_log" ]] || fail 'Docker was called for an input checksum mismatch'

expect_reported_failure format_invalid upstream archive "$external_dir/archive-failure.json" \
  --source-kind upstream --dump "$custom_dump" --expected-sha256 "$(sha256_file "$custom_dump")" --postgres-image "$image_ref"
expect_reported_failure restore_failed upstream restore "$external_dir/restore-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_build "$external_dir/migration-build-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_chmod "$external_dir/migration-chmod-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_host_hash "$external_dir/migration-host-hash-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_host_hash_invalid "$external_dir/migration-host-hash-invalid-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_copy "$external_dir/migration-copy-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_hash "$external_dir/migration-hash-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_hash_mismatch "$external_dir/migration-hash-mismatch-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure migration_failed upstream migration_first "$external_dir/migration-first-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
jq -e '.runtime.migration_runs == 0' "$external_dir/migration-first-failure.json" >/dev/null || \
  fail 'first migration failure overstated the completed migration count'
expect_reported_failure migration_idempotency_failed upstream migration_second "$external_dir/migration-second-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
jq -e '.runtime.migration_runs == 1' "$external_dir/migration-second-failure.json" >/dev/null || \
  fail 'second migration failure reported the wrong completed migration count'
expect_reported_failure metadata_failed upstream metadata "$external_dir/metadata-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure source_mismatch mismatch none "$external_dir/source-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream invariant "$external_dir/invariant-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream audit_seed_extra "$external_dir/audit-seed-extra-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream audit_seed_unknown "$external_dir/audit-seed-unknown-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream audit_seed_changed "$external_dir/audit-seed-changed-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream audit_seed_missing "$external_dir/audit-seed-missing-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream bridge_contract "$external_dir/bridge-contract-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream bridge_object "$external_dir/bridge-object-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream required_migration "$external_dir/required-migration-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream sequence_invalid "$external_dir/sequence-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure invariant_failed upstream catalog_invalid "$external_dir/catalog-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure postgres_start_failed upstream container_start "$external_dir/start-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
expect_reported_failure postgres_start_failed upstream container_create "$external_dir/create-failure.json" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
assert_contains 'rm --force --volumes modelport-restore-pg-' "$docker_log"

collision_report="$external_dir/network-collision-failure.json"
expect_reported_failure cleanup_failed upstream network_collision "$collision_report" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
assert_not_contains 'network rm modelport-restore-' "$docker_log"
find "$state_dir" -maxdepth 1 -type f -name 'unowned-network-modelport-restore-*' | grep -q . || \
  fail 'the simulated unowned network collision was not preserved'
find "$state_dir" -maxdepth 1 -type f -name 'unowned-network-modelport-restore-*' -delete

swap_report="$external_dir/input-swap-failure.json"
MODELPORT_TEST_SWAP_DUMP_PATH="$plain_dump" \
  expect_reported_failure input_changed upstream input_swap "$swap_report" \
    --source-kind upstream --dump "$plain_dump" \
    --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
mv -- "$plain_dump.modelport-test-original" "$plain_dump"

cleanup_failure_report="$external_dir/cleanup-failure.json"
expect_reported_failure cleanup_failed upstream cleanup_once "$cleanup_failure_report" \
  --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
jq -e '.result == "failed" and .failure_code == "cleanup_failed"' \
  "$cleanup_failure_report" >/dev/null || fail 'cleanup failure was reported as passed'
[[ "$(grep -Fc 'volume rm ' "$docker_log")" -eq 2 ]] || \
  fail 'cleanup failure did not retry the exact volume once'

local_cleanup_report="$external_dir/local-cleanup-failure.json"
MODELPORT_TEST_EXPECT_REPORT="$local_cleanup_report" \
  expect_reported_failure cleanup_failed upstream local_cleanup "$local_cleanup_report" \
    --source-kind upstream --dump "$plain_dump" --expected-sha256 "$(sha256_file "$plain_dump")" --postgres-image "$image_ref"
[[ -f "$state_dir/local-cleanup-failed" ]] || fail 'the local cleanup failure was not exercised'
[[ ! -f "$state_dir/passed-report-visible-before-local-cleanup" ]] || \
  fail 'a passed report was published before the sensitive work directory cleanup completed'
local_cleanup_target="$(<"$state_dir/local-cleanup-target")"
[[ ! -e "$local_cleanup_target" ]] || fail 'the local sensitive work directory remained after cleanup retry'

reset_run
dirty_report="$external_dir/dirty-production-report.json"
if env \
  PATH="$bin_dir:$PATH" \
  MODELPORT_TEST_DOCKER_LOG="$docker_log" \
  MODELPORT_TEST_STATE="$state_dir" \
  MODELPORT_TEST_IMAGE_ID="$image_id" \
  MODELPORT_TEST_FIXTURE_KIND=production \
  MODELPORT_TEST_FAILURE=none \
  MODELPORT_TEST_REVISION="$revision" \
  MODELPORT_TEST_TREE_SHA="$tree_sha" \
  MODELPORT_TEST_GIT_DIRTY=true \
  MODELPORT_TEST_LEGACY_LEDGER_JSON="$legacy_ledger_json" \
  MODELPORT_TEST_REQUIRED_BRIDGE_TABLES_JSON="$required_bridge_tables_json" \
  MODELPORT_TEST_REQUIRED_BRIDGE_CONSTRAINTS_JSON="$required_bridge_constraints_json" \
  MODELPORT_TEST_REQUIRED_BRIDGE_INDEXES_JSON="$required_bridge_indexes_json" \
  /bin/bash "$script" \
    --source-kind production --dump "$custom_dump" --expected-sha256 "$production_sha" \
    --expected-revision "$revision" --expected-postgres-major 18 --confirm-production-storage \
    --postgres-image "$image_ref" --report "$dirty_report" \
    >"$stdout_log" 2>"$stderr_log"; then
  fail 'production accepted a dirty repository checkout'
fi
jq -e '.failure_code == "repository_mismatch"' "$dirty_report" >/dev/null || \
  fail 'dirty production checkout emitted the wrong failure code'
[[ ! -s "$docker_log" ]] || fail 'Docker was called for a dirty production checkout'

if [[ -n "${MODELPORT_POSTGRES_DRILL_IMAGE:-}" ]]; then
  real_report="$external_dir/real-empty-restore-report.json"
  real_stdout="$external_dir/real-empty-restore.stdout"
  real_stderr="$external_dir/real-empty-restore.stderr"
  [[ "$MODELPORT_POSTGRES_DRILL_IMAGE" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[0-9a-f]{64}$ ]] || \
    fail 'MODELPORT_POSTGRES_DRILL_IMAGE must be a digest-pinned local image reference'
  real_image_id="$(docker image inspect --format '{{.Id}}' "$MODELPORT_POSTGRES_DRILL_IMAGE")" || \
    fail 'the real-drill PostgreSQL image is not preloaded locally'
  [[ "$real_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || \
    fail 'the real-drill PostgreSQL image resolved to an invalid image ID'

  if ! /bin/bash "$script" \
      --source-kind empty \
      --expected-postgres-major 18 \
      --postgres-image "$MODELPORT_POSTGRES_DRILL_IMAGE" \
      --report "$real_report" \
      >"$real_stdout" 2>"$real_stderr"; then
    fail 'the real PostgreSQL empty restore drill failed'
  fi
  jq -e '
    .result == "passed" and
    .source_kind == "empty" and
    .runtime.postgres_server_major == 18 and
    .runtime.expected_postgres_major == 18 and
    .runtime.migration_runs == 2 and
    .runtime.isolation.docker_internal_network == true and
    .runtime.isolation.host_port_published == false and
    .runtime.isolation.cleanup_completed == true and
    all(.invariants[]; .passed == true)
  ' "$real_report" >/dev/null || fail 'the real PostgreSQL empty drill report failed its contract'
  [[ "$(file_mode "$real_report")" == 600 ]] || fail 'the real PostgreSQL drill report mode was not 0600'
  for checked_file in "$real_report" "$real_stdout" "$real_stderr"; do
    assert_not_contains "$fixture_secret" "$checked_file"
  done
fi

printf 'ModelPort isolated restore contract test passed\n'
