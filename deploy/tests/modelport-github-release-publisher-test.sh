#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
publisher="$repo_root/custom/release/publish-github-release-create-only"
test_root="$(mktemp -d /tmp/modelport-github-release-publisher-test.XXXXXX)"
mock_bin="$test_root/bin"

cleanup() {
  rm -rf -- "$test_root"
}
trap cleanup EXIT

fail() {
  printf 'ModelPort GitHub release publisher test failed: %s\n' "$1" >&2
  exit 1
}

mkdir "$mock_bin"

cat > "$mock_bin/curl" <<'MOCK_CURL'
#!/usr/bin/env bash
set -Eeuo pipefail

cat >/dev/null

method='GET'
output=''
data_path=''
url=''
while [[ $# -gt 0 ]]; do
  case "$1" in
    --request)
      method="$2"
      shift 2
      ;;
    --output)
      output="$2"
      shift 2
      ;;
    --data-binary)
      data_path="${2#@}"
      shift 2
      ;;
    --config|--header|--connect-timeout|--max-time|--proto|--proto-redir|--write-out)
      shift 2
      ;;
    --silent|--show-error|--location)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

state="$MODELPORT_GITHUB_TEST_STATE"
mode="$MODELPORT_GITHUB_TEST_MODE"
log="$state/requests.log"
printf '%s %s\n' "$method" "$url" >> "$log"

respond_json() {
  local status="$1"
  local source="$2"
  cp -- "$source" "$output"
  printf '%s' "$status"
}

respond_empty() {
  local status="$1"
  : > "$output"
  printf '%s' "$status"
}

error_json="$state/error.json"
printf '{"message":"mock failure"}\n' > "$error_json"

case "$method $url" in
  "POST https://api.github.com/repos/abingooo/modelport/git/tags")
    jq --arg sha 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' \
      '{sha:$sha,tag:.tag,message:.message,object:{type:.type,sha:.object}}' \
      "$data_path" > "$state/tag-object.json"
    respond_json 201 "$state/tag-object.json"
    ;;

  "POST https://api.github.com/repos/abingooo/modelport/git/refs")
    if [[ "$mode" == tag_collision ]]; then
      respond_json 422 "$error_json"
      exit 0
    fi
    if [[ "$mode" == tag_ambiguous ]]; then
      jq '{ref:.ref,object:{type:"tag",sha:.sha}}' "$data_path" > "$state/tag-ref.json"
      respond_json 500 "$error_json"
      exit 0
    fi
    jq '{ref:.ref,object:{type:"tag",sha:.sha}}' "$data_path" > "$state/tag-ref.json"
    respond_json 201 "$state/tag-ref.json"
    ;;

  "POST https://api.github.com/repos/abingooo/modelport/releases")
    if [[ "$mode" == release_collision ]]; then
      jq -n '
        {id:909,node_id:"external-release",tag_name:"custom-v0.1.183.1",
         target_commitish:"external",name:"external",body:"external",draft:true,
         prerelease:false,immutable:false,assets:[]}
      ' > "$state/release.json"
      respond_json 422 "$error_json"
      exit 0
    fi
    if [[ "$mode" == tag_replaced ]]; then
      jq '.object.sha = "cccccccccccccccccccccccccccccccccccccccc"' \
        "$state/tag-ref.json" > "$state/tag-ref-next.json"
      mv "$state/tag-ref-next.json" "$state/tag-ref.json"
      respond_json 500 "$error_json"
      exit 0
    fi
    jq '
      . + {id:101,node_id:"owned-release-node",html_url:
        "https://github.com/abingooo/modelport/releases/tag/custom-v0.1.183.1",
        upload_url:"https://uploads.github.com/repos/abingooo/modelport/releases/101/assets{?name,label}",
        immutable:false,assets:[]}
    ' "$data_path" > "$state/release.json"
    printf '[]\n' > "$state/assets.json"
    respond_json 201 "$state/release.json"
    ;;

  "POST https://uploads.github.com/repos/abingooo/modelport/releases/101/assets?name="*)
    asset_name="${url##*?name=}"
    if [[ "$mode" == asset_collision ]]; then
      jq -n --arg name "$asset_name" \
        '[{id:999,node_id:"external-asset-node",name:$name,size:7,state:"uploaded"}]' \
        > "$state/assets.json"
      printf 'foreign' > "$state/asset-999.bin"
      respond_json 422 "$error_json"
      exit 0
    fi
    if [[ "$mode" == release_mutation ]]; then
      jq '.name = "externally changed"' "$state/release.json" > "$state/release-next.json"
      mv "$state/release-next.json" "$state/release.json"
      respond_json 500 "$error_json"
      exit 0
    fi
    asset_count="$(jq 'length' "$state/assets.json")"
    asset_id="$((201 + asset_count))"
    asset_size="$(wc -c < "$data_path" | tr -d '[:space:]')"
    asset_response="$state/asset-response.json"
    jq -n \
      --argjson id "$asset_id" \
      --arg node_id "owned-asset-$asset_id" \
      --arg name "$asset_name" \
      --argjson size "$asset_size" \
      '{id:$id,node_id:$node_id,name:$name,size:$size,state:"uploaded"}' \
      > "$asset_response"
    jq --slurpfile asset "$asset_response" '. + [$asset[0]]' \
      "$state/assets.json" > "$state/assets-next.json"
    mv "$state/assets-next.json" "$state/assets.json"
    cp -- "$data_path" "$state/asset-$asset_id.bin"
    respond_json 201 "$asset_response"
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/releases/101")
    if [[ -f "$state/release.json" ]] && [[ "$(jq -r '.id' "$state/release.json")" == 101 ]]; then
      if [[ "$mode" == published_get_mutable ]] && \
         [[ "$(jq -r '.draft' "$state/release.json")" == false ]]; then
        jq '.immutable = false' "$state/release.json" > "$state/release-next.json"
        mv "$state/release-next.json" "$state/release.json"
        respond_json 200 "$state/release.json"
      else
        respond_json 200 "$state/release.json"
      fi
    else
      respond_json 404 "$error_json"
    fi
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/releases/101/assets?per_page=100")
    if [[ "$mode" == asset_page_full ]]; then
      # Model a full first page with an external asset on page two.  The
      # publisher must refuse to delete the draft because it cannot prove the
      # response is complete from this endpoint alone.
      if [[ ! -f "$state/assets-page-ready" ]]; then
        jq '. + [{id:999,node_id:"external-hidden-node",name:"hidden-external.bin",size:13,state:"uploaded"}]' \
          "$state/assets.json" > "$state/assets-next.json"
        mv "$state/assets-next.json" "$state/assets.json"
        : > "$state/assets-page-ready"
      fi
      jq '.[0:100]' "$state/assets.json" > "$state/assets-page.json"
      respond_json 200 "$state/assets-page.json"
      exit 0
    fi
    if [[ -f "$state/assets.json" ]]; then
      respond_json 200 "$state/assets.json"
    else
      printf '[]\n' > "$state/empty-assets.json"
      respond_json 200 "$state/empty-assets.json"
    fi
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/releases/assets/"*)
    asset_id="${url##*/}"
    if [[ ! -f "$state/asset-$asset_id.bin" ]]; then
      respond_json 404 "$error_json"
      exit 0
    fi
    if [[ "$mode" == download_mismatch && "$asset_id" == 202 ]]; then
      printf 'changed bytes' > "$output"
      printf '200'
    else
      cp -- "$state/asset-$asset_id.bin" "$output"
      printf '200'
    fi
    ;;

  "PATCH https://api.github.com/repos/abingooo/modelport/releases/101")
    if [[ "$mode" == publish_failure || "$mode" == asset_page_full ]]; then
      respond_json 500 "$error_json"
      exit 0
    fi
    jq --arg mode "$mode" '
      .draft = false |
      .prerelease = false |
      .immutable = ($mode != "publish_response_mutable") |
      .published_at = "2026-08-29T00:00:00Z"
    ' "$state/release.json" > "$state/release-next.json"
    mv "$state/release-next.json" "$state/release.json"
    if [[ "$mode" == published_get_mutable ]]; then
      jq '.immutable = true' "$state/release.json" > "$state/publish-response.json"
      respond_json 200 "$state/publish-response.json"
    else
      respond_json 200 "$state/release.json"
    fi
    ;;

  "DELETE https://api.github.com/repos/abingooo/modelport/releases/101")
    rm -f -- "$state/release.json" "$state/assets.json" "$state"/asset-*.bin
    respond_empty 204
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/releases/tags/custom-v0.1.183.1")
    if [[ -f "$state/release.json" ]]; then
      respond_json 200 "$state/release.json"
    else
      respond_json 404 "$error_json"
    fi
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/git/ref/tags/custom-v0.1.183.1")
    if [[ -f "$state/tag-ref.json" ]]; then
      respond_json 200 "$state/tag-ref.json"
    else
      respond_json 404 "$error_json"
    fi
    ;;

  "GET https://api.github.com/repos/abingooo/modelport/git/tags/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
    respond_json 200 "$state/tag-object.json"
    ;;

  "DELETE https://api.github.com/repos/abingooo/modelport/git/refs/tags/custom-v0.1.183.1")
    rm -f -- "$state/tag-ref.json"
    respond_empty 204
    ;;

  *)
    printf 'unexpected mock request: %s %s\n' "$method" "$url" >&2
    respond_json 500 "$error_json"
    ;;
esac
MOCK_CURL
chmod 0755 "$mock_bin/curl"

body="$test_root/release-notes.md"
asset_one="$test_root/manifest-digest.txt"
asset_two="$test_root/release-metadata.json"
printf 'ModelPort release notes\n' > "$body"
printf 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n' > "$asset_one"
printf '{"version":"0.1.183.1"}\n' > "$asset_two"

page_assets_dir="$test_root/page-assets"
mkdir "$page_assets_dir"
declare -a page_asset_args=()
for ((page_asset_index=1; page_asset_index<=100; page_asset_index++)); do
  printf -v page_asset_number '%03d' "$page_asset_index"
  page_asset_path="$page_assets_dir/page-${page_asset_number}.txt"
  printf 'page asset %s\n' "$page_asset_number" > "$page_asset_path"
  page_asset_args+=(--asset "$page_asset_path")
done

run_case() {
  local mode="$1"
  shift
  local state="$test_root/state-$mode"
  local stdout="$state/stdout.log"
  local stderr="$state/stderr.log"
  local -a asset_args=(--asset "$asset_one" --asset "$asset_two")
  if [[ "$#" -gt 0 ]]; then
    asset_args=("$@")
  fi
  mkdir "$state"
  if env \
    PATH="$mock_bin:$PATH" \
    GH_TOKEN=modelport_test_token \
    GITHUB_API_URL=https://api.github.com \
    MODELPORT_GITHUB_TEST_STATE="$state" \
    MODELPORT_GITHUB_TEST_MODE="$mode" \
    /bin/bash "$publisher" \
      --repository abingooo/modelport \
      --tag custom-v0.1.183.1 \
      --candidate-sha 1111111111111111111111111111111111111111 \
      --name 'ModelPort 0.1.183.1' \
      --body "$body" \
      "${asset_args[@]}" \
      >"$stdout" 2>"$stderr"; then
    return 0
  fi
  return 1
}

expect_failure() {
  local mode="$1"
  shift
  if run_case "$mode" "$@"; then
    fail "negative vector unexpectedly published: $mode"
  fi
}

run_case success || fail 'valid create-only publication was rejected'
success_state="$test_root/state-success"
jq -e '
  .url == "https://github.com/abingooo/modelport/releases/tag/custom-v0.1.183.1" and
  .tag == "custom-v0.1.183.1" and
  .candidate_revision == "1111111111111111111111111111111111111111" and
  .annotated_tag_object == "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" and
  .release_id == 101 and (.assets | length) == 2
' "$success_state/stdout.log" >/dev/null || fail 'success output identity was invalid'
jq -e '.draft == false and .immutable == true' "$success_state/release.json" >/dev/null || \
  fail 'the verified draft was not formally published'
jq -e '
  .ref == "refs/tags/custom-v0.1.183.1" and
  .object.type == "tag" and
  .object.sha == "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
' "$success_state/tag-ref.json" >/dev/null || fail 'success tag ref identity was invalid'
jq -e '
  .tag == "custom-v0.1.183.1" and
  .object.type == "commit" and
  .object.sha == "1111111111111111111111111111111111111111"
' "$success_state/tag-object.json" >/dev/null || fail 'annotated tag did not bind the candidate'
if grep -q '^DELETE ' "$success_state/requests.log"; then
  fail 'successful publication attempted rollback deletion'
fi
last_download="$(grep -n 'GET.*/releases/assets/' "$success_state/requests.log" | tail -n 1 | cut -d: -f1)"
publish_line="$(grep -n '^PATCH.*/releases/101$' "$success_state/requests.log" | cut -d: -f1)"
final_get_line="$(grep -n '^GET.*/releases/101$' "$success_state/requests.log" | tail -n 1 | cut -d: -f1)"
[[ -n "$last_download" && -n "$publish_line" && "$last_download" -lt "$publish_line" ]] || \
  fail 'formal publication occurred before every asset was downloaded and verified'
[[ -n "$final_get_line" && "$publish_line" -lt "$final_get_line" ]] || \
  fail 'formal publication was not followed by an immutable Release GET verification'

expect_failure tag_collision
tag_collision_state="$test_root/state-tag_collision"
[[ ! -e "$tag_collision_state/release.json" ]] || fail 'tag collision created a Release'
if grep -q '^DELETE ' "$tag_collision_state/requests.log"; then
  fail 'tag collision attempted to delete an unowned resource'
fi

expect_failure release_collision
release_collision_state="$test_root/state-release_collision"
jq -e '.node_id == "external-release" and .draft == true' \
  "$release_collision_state/release.json" >/dev/null || fail 'external Release collision was not preserved'
[[ -f "$release_collision_state/tag-ref.json" ]] || \
  fail 'owned tag was deleted while an external Release used it'
if grep -q '^DELETE ' "$release_collision_state/requests.log"; then
  fail 'Release collision attempted to delete an external resource'
fi

expect_failure asset_collision
asset_collision_state="$test_root/state-asset_collision"
jq -e 'length == 1 and .[0].node_id == "external-asset-node"' \
  "$asset_collision_state/assets.json" >/dev/null || fail 'external asset collision was not preserved'
[[ -f "$asset_collision_state/release.json" && -f "$asset_collision_state/tag-ref.json" ]] || \
  fail 'unknown external asset did not block Release and tag rollback'
if grep -q '^DELETE ' "$asset_collision_state/requests.log"; then
  fail 'asset collision attempted to delete a resource containing external data'
fi

expect_failure release_mutation
release_mutation_state="$test_root/state-release_mutation"
jq -e '.name == "externally changed" and .draft == true' \
  "$release_mutation_state/release.json" >/dev/null || fail 'mutated draft was not preserved'
if grep -q '^DELETE ' "$release_mutation_state/requests.log"; then
  fail 'mutated Release identity did not block rollback deletion'
fi

expect_failure tag_replaced
tag_replaced_state="$test_root/state-tag_replaced"
jq -e '.object.sha == "cccccccccccccccccccccccccccccccccccccccc"' \
  "$tag_replaced_state/tag-ref.json" >/dev/null || fail 'replaced tag ref was not preserved'
if grep -q '^DELETE ' "$tag_replaced_state/requests.log"; then
  fail 'replaced tag identity did not block rollback deletion'
fi

expect_failure tag_ambiguous
tag_ambiguous_state="$test_root/state-tag_ambiguous"
[[ ! -e "$tag_ambiguous_state/tag-ref.json" ]] || \
  fail 'an exact tag ref was not rolled back after an ambiguous create response'
grep -q '^DELETE.*/git/refs/tags/custom-v0.1.183.1$' \
  "$tag_ambiguous_state/requests.log" || \
  fail 'ambiguous tag create did not trigger identity-aware ref rollback'

expect_failure asset_page_full "${page_asset_args[@]}"
asset_page_full_state="$test_root/state-asset_page_full"
[[ -f "$asset_page_full_state/release.json" &&
   -f "$asset_page_full_state/tag-ref.json" &&
   -f "$asset_page_full_state/assets.json" ]] || \
  fail 'full asset page did not preserve the draft and tag for review'
jq -e 'length == 101 and .[-1].node_id == "external-hidden-node"' \
  "$asset_page_full_state/assets.json" >/dev/null || \
  fail 'full asset page fixture did not retain the hidden external asset'
if grep -q '^DELETE ' "$asset_page_full_state/requests.log"; then
  fail 'full asset page triggered unsafe draft or tag deletion'
fi
if grep -q '^PATCH ' "$asset_page_full_state/requests.log"; then
  fail 'full asset page reached formal publication before complete asset verification'
fi

for clean_rollback_mode in publish_failure download_mismatch; do
  expect_failure "$clean_rollback_mode"
  clean_state="$test_root/state-$clean_rollback_mode"
  [[ ! -e "$clean_state/release.json" && ! -e "$clean_state/tag-ref.json" ]] || \
    fail "exact owned resources were not rolled back: $clean_rollback_mode"
  grep -q '^DELETE.*/releases/101$' "$clean_state/requests.log" || \
    fail "owned draft Release was not deleted: $clean_rollback_mode"
  grep -q '^DELETE.*/git/refs/tags/custom-v0.1.183.1$' "$clean_state/requests.log" || \
    fail "owned tag ref was not deleted: $clean_rollback_mode"
done

for mutable_publication_mode in publish_response_mutable published_get_mutable; do
  expect_failure "$mutable_publication_mode"
  mutable_state="$test_root/state-$mutable_publication_mode"
  jq -e '.draft == false and .immutable == false' "$mutable_state/release.json" >/dev/null || \
    fail "non-immutable public Release state was not preserved: $mutable_publication_mode"
  [[ -f "$mutable_state/tag-ref.json" ]] || \
    fail "published tag was removed after immutable verification failed: $mutable_publication_mode"
  if grep -q '^DELETE ' "$mutable_state/requests.log"; then
    fail "non-draft Release triggered unsafe rollback deletion: $mutable_publication_mode"
  fi
  grep -Fq 'immutable Release' "$mutable_state/stderr.log" || \
    fail "immutable verification failure was not reported: $mutable_publication_mode"
done

printf 'ModelPort create-only GitHub release publisher test passed\n'
