#!/bin/sh
set -eu

repo_root=$(git rev-parse --show-toplevel)
manifest="$repo_root/backend/testdata/modelport_crypto_compat_v1.json"
harness="$repo_root/backend/testdata/modelport_crypto_compat_generator_test.go"

for required in git go jq tar cp cmp mktemp awk; do
	command -v "$required" >/dev/null 2>&1 || {
		echo "missing required command: $required" >&2
		exit 1
	}
done
test -f "$manifest"
test -f "$harness"

sha256_stdin() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 | awk '{print $1}'
	else
		echo "sha256sum or shasum is required" >&2
		return 1
	fi
}

source_tag=$(jq -er '.source.tag' "$manifest")
tag_object=$(jq -er '.source.tag_object' "$manifest")
source_commit=$(jq -er '.source.commit' "$manifest")
evidence_blob=$(jq -er '.source.instruction_evidence_cipher_blob' "$manifest")
evidence_sha256=$(jq -er '.source.instruction_evidence_cipher_sha256' "$manifest")
secret_blob=$(jq -er '.source.aes_encryptor_blob' "$manifest")
secret_sha256=$(jq -er '.source.aes_encryptor_sha256' "$manifest")
harness_blob=$(jq -er '.generator.harness_blob' "$manifest")
harness_sha256=$(jq -er '.generator.harness_sha256' "$manifest")

test "$(git -C "$repo_root" rev-parse "$source_tag")" = "$tag_object"
test "$(git -C "$repo_root" rev-parse "$source_tag^{commit}")" = "$source_commit"
test "$(git -C "$repo_root" rev-parse "$source_tag:backend/internal/securityaudit/instruction_evidence_cipher.go")" = "$evidence_blob"
test "$(git -C "$repo_root" show "$source_tag:backend/internal/securityaudit/instruction_evidence_cipher.go" | sha256_stdin)" = "$evidence_sha256"
test "$(git -C "$repo_root" rev-parse "$source_tag:backend/internal/repository/aes_encryptor.go")" = "$secret_blob"
test "$(git -C "$repo_root" show "$source_tag:backend/internal/repository/aes_encryptor.go" | sha256_stdin)" = "$secret_sha256"
test "$(git -C "$repo_root" hash-object "$harness")" = "$harness_blob"
test "$(sha256_stdin < "$harness")" = "$harness_sha256"

compat_tmp=$(mktemp -d "${TMPDIR:-/tmp}/modelport-crypto-compat.XXXXXX")
test -n "$compat_tmp"
cleanup() {
	rm -rf -- "$compat_tmp"
}
trap cleanup EXIT HUP INT TERM

git -C "$repo_root" archive "$source_commit" | tar -xf - -C "$compat_tmp"
cp "$harness" "$compat_tmp/backend/internal/securityaudit/modelport_crypto_compat_generator_test.go"

(
	cd "$compat_tmp/backend"
	MODELPORT_CRYPTO_VECTOR_OUTPUT="$compat_tmp/generated-vectors.json" \
		GOTOOLCHAIN=local \
		go test -run '^TestGenerateModelPortCryptoCompatibilityVectors$' -count=1 ./internal/securityaudit
)

jq -S '.vectors' "$manifest" > "$compat_tmp/expected-vectors.json"
jq -S '.' "$compat_tmp/generated-vectors.json" > "$compat_tmp/actual-vectors.json"
cmp "$compat_tmp/expected-vectors.json" "$compat_tmp/actual-vectors.json"

echo "ModelPort historical crypto compatibility vectors verified from $source_tag ($source_commit)"
