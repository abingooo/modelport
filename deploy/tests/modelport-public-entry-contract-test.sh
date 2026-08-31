#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

fail() {
  printf 'ModelPort public entry contract test failed: %s\n' "$1" >&2
  exit 1
}

assert_line() {
  file=$1
  line=$2
  grep -Fqx "$line" "$file" || fail "$file is missing: $line"
}

assert_contains() {
  file=$1
  text=$2
  grep -Fq "$text" "$file" || fail "$file is missing required content: $text"
}

version=$(tr -d '[:space:]' < custom/VERSION)
upstream_version=$(tr -d '[:space:]' < backend/cmd/server/UPSTREAM_VERSION)
release_tag="custom-v${version}"
release_image="ghcr.io/abingooo/modelport:${release_tag}"
compose_image_line="    image: \${MODELPORT_IMAGE:-${release_image}}"

assert_line .github/workflows/custom-image.yml '  RELEASE_IMAGE: ghcr.io/abingooo/modelport'
assert_contains .github/workflows/custom-image.yml 'tag="custom-v${version}"'
assert_contains .github/workflows/custom-image.yml 'platforms: linux/amd64'

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml; do
  assert_line "$compose_file" '  sub2api:'
  assert_line "$compose_file" "$compose_image_line"
  assert_line "$compose_file" '    platform: linux/amd64'
  assert_contains "$compose_file" 'DATABASE_DBNAME='
  assert_contains "$compose_file" 'sub2api}'
done

assert_line deploy/.env.example "MODELPORT_IMAGE=${release_image}"
assert_line deploy/.env.example "APPLE_CONTAINER_SUB2API_IMAGE=${release_image}"
assert_line deploy/docker-deploy.sh 'GITHUB_RAW_URL="https://raw.githubusercontent.com/abingooo/modelport/main/deploy"'
assert_line deploy/install.sh 'GITHUB_REPO="abingooo/modelport"'
assert_line deploy/install.sh 'MODELPORT_IMAGE="ghcr.io/abingooo/modelport:custom-v0.1.183.1"'
assert_contains deploy/install.sh 'binary_distribution_unsupported'
assert_contains deploy/install.sh 'It never falls back to an upstream Sub2API release.'
assert_line deploy/apple-container.sh 'FORMAL_MODELPORT_IMAGE="ghcr.io/abingooo/modelport:custom-v0.1.183.1"'
assert_contains deploy/apple-container.sh 'assert_supported_arm64_app_image'
assert_contains deploy/apple-container.sh 'Upstream Sub2API images are not valid ModelPort artifacts'
assert_line deploy/sub2api.service 'Documentation=https://github.com/abingooo/modelport'
assert_line deploy/config.example.yaml '  rp_display_name: "ModelPort"'

for readme in README.md README_CN.md README_JA.md; do
  assert_contains "$readme" '# ModelPort'
  assert_contains "$readme" "Sub2API \`v${upstream_version}\`"
  assert_contains "$readme" 'https://raw.githubusercontent.com/abingooo/modelport/main/deploy/docker-deploy.sh'
  assert_contains "$readme" 'git clone https://github.com/abingooo/modelport.git'
  assert_contains "$readme" '`linux/amd64`'
  assert_contains "$readme" 'ModelPort'
done

assert_contains deploy/README.md 'https://github.com/abingooo/modelport/releases'
assert_contains deploy/DOCKER.md "$release_image"
assert_contains deploy/DOCKER.md "Upstream Sub2API \`v${upstream_version}\`"
assert_contains deploy/APPLE_CONTAINER.md "$release_image"

for frontend_file in \
  frontend/src/stores/adminCompliance.ts \
  frontend/src/components/admin/AdminComplianceDialog.vue \
  frontend/src/views/admin/SettingsView.vue; do
  assert_contains "$frontend_file" 'https://github.com/abingooo/modelport/blob/main/docs/'
done

assert_contains backend/internal/service/admin_compliance.go 'AdminComplianceDocumentURLZH  = "https://github.com/abingooo/modelport/blob/main/docs/legal/admin-compliance.zh.md"'
assert_contains backend/internal/service/admin_compliance.go 'AdminComplianceDocumentURLEN  = "https://github.com/abingooo/modelport/blob/main/docs/legal/admin-compliance.en.md"'
assert_contains backend/internal/service/admin_compliance.go 'AdminComplianceVersion        = "v2026.06.10"'
assert_contains backend/internal/service/admin_compliance.go 'AdminComplianceAckPhraseZH    = "我已阅读、理解并同意 Sub2API 部署与运营合规承诺"'

for public_entry in \
  README.md README_CN.md README_JA.md \
  deploy/README.md deploy/DOCKER.md deploy/APPLE_CONTAINER.md \
  deploy/docker-deploy.sh deploy/install.sh deploy/sub2api.service \
  deploy/docker-compose.yml deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml deploy/.env.example \
  frontend/src/stores/adminCompliance.ts \
  frontend/src/components/admin/AdminComplianceDialog.vue \
  frontend/src/views/admin/SettingsView.vue \
  backend/internal/service/admin_compliance.go; do
  if grep -Eq 'raw\.githubusercontent\.com/Wei-Shaw/sub2api|git clone https://github\.com/Wei-Shaw/sub2api|weishaw/sub2api(:|@)|github\.com/Wei-Shaw/sub2api/(releases|blob/main/docs)' "$public_entry"; then
    fail "$public_entry still routes a public install or documentation entry to upstream Sub2API"
  fi
done

for runtime_config in \
  deploy/docker-compose.yml deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml deploy/.env.example; do
  if grep -Eq '^[[:space:]]*(image:|[A-Z0-9_]*IMAGE=).*:latest([[:space:]]|$)' "$runtime_config"; then
    fail "$runtime_config contains a mutable latest image default"
  fi
done

if grep -Fq '<title id="title">Sub2API</title>' frontend/public/logo.svg; then
  fail 'frontend fallback logo still exposes the upstream product title'
fi

apple_policy=$(mktemp)
trap 'rm -f "$apple_policy"' EXIT HUP INT TERM
sed -n '/^assert_supported_arm64_app_image() {$/,/^}$/p' deploy/apple-container.sh >"$apple_policy"
assert_contains "$apple_policy" 'assert_supported_arm64_app_image'

if APP_IMAGE="$release_image" FORMAL_MODELPORT_IMAGE="$release_image" \
  sh -c 'die() { exit 1; }; . "$1"; assert_supported_arm64_app_image' sh "$apple_policy"; then
  fail 'Apple helper accepted the formal linux/amd64 image for linux/arm64'
fi
if APP_IMAGE='weishaw/sub2api:latest' FORMAL_MODELPORT_IMAGE="$release_image" \
  sh -c 'die() { exit 1; }; . "$1"; assert_supported_arm64_app_image' sh "$apple_policy"; then
  fail 'Apple helper accepted an upstream Sub2API image'
fi
if ! APP_IMAGE='ghcr.io/abingooo/modelport:reviewed-arm64-test' FORMAL_MODELPORT_IMAGE="$release_image" \
  sh -c 'die() { exit 1; }; . "$1"; assert_supported_arm64_app_image' sh "$apple_policy"; then
  fail 'Apple helper rejected an explicit ModelPort ARM64 development image'
fi

printf 'ModelPort public entry contract test passed\n'
