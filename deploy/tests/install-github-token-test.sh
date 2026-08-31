#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Extract only the curl helper so this security test also runs on macOS's Bash
# 3.2; the installer itself intentionally requires Bash 4 for message maps.
sed -n '/^github_api_curl() {$/,/^}$/p' "$ROOT_DIR/deploy/install.sh" \
    > "$TEMP_DIR/github-api-curl.sh"
grep -Fq 'github_api_curl()' "$TEMP_DIR/github-api-curl.sh"

{
    printf '%s\n' 'MODELPORT_RELEASE="0.1.183.1"'
    printf '%s\n' 'MODELPORT_IMAGE="ghcr.io/abingooo/modelport:custom-v0.1.183.1"'
    sed -n '/^print_binary_distribution_help() {$/,/^}$/p' "$ROOT_DIR/deploy/install.sh"
    sed -n '/^binary_distribution_unsupported() {$/,/^}$/p' "$ROOT_DIR/deploy/install.sh"
    sed -n '/^main() {$/,/^}$/p' "$ROOT_DIR/deploy/install.sh"
} > "$TEMP_DIR/binary-policy.sh"

if bash -c 'source "$1"; main install' bash "$TEMP_DIR/binary-policy.sh" \
    >"$TEMP_DIR/binary-install.stdout" 2>"$TEMP_DIR/binary-install.stderr"; then
    echo "installer accepted an unsupported binary installation" >&2
    exit 1
fi
grep -Fq 'does not publish binary archives or checksums' "$TEMP_DIR/binary-install.stderr"
grep -Fq 'It never falls back to an upstream Sub2API release.' "$TEMP_DIR/binary-install.stderr"
test ! -s "$TEMP_DIR/binary-install.stdout"

bash -c 'source "$1"; main --help' bash "$TEMP_DIR/binary-policy.sh" \
    >"$TEMP_DIR/binary-help.stdout" 2>"$TEMP_DIR/binary-help.stderr"
grep -Fq 'ghcr.io/abingooo/modelport:custom-v0.1.183.1 (linux/amd64)' "$TEMP_DIR/binary-help.stdout"
test ! -s "$TEMP_DIR/binary-help.stderr"

cat > "$TEMP_DIR/curl" <<'EOF'
#!/bin/bash
printf '%s\n' "$@" > "$CURL_ARGS_LOG"
env > "${CURL_ARGS_LOG}.env"
cat > "${CURL_ARGS_LOG}.stdin"
EOF
chmod +x "$TEMP_DIR/curl"

mkdir "$TEMP_DIR/home"
cat > "$TEMP_DIR/home/.curlrc" <<'EOF'
url = "https://example.com/collect"
header = "X-Leaked-From-Curlrc: yes"
EOF

run_api_curl() {
    CURL_ARGS_LOG="$1" HOME="$TEMP_DIR/home" PATH="$TEMP_DIR:$PATH" UPDATE_GITHUB_TOKEN="${2:-}" \
        GITHUB_TOKEN="github-fallback" GH_TOKEN="gh-fallback" \
        bash -c 'source "$1"; github_api_curl -s "$2"' bash \
        "$TEMP_DIR/github-api-curl.sh" "https://api.github.com/repos/abingooo/modelport/releases/latest"
}

run_api_curl "$TEMP_DIR/authenticated" "update-secret"
test "$(head -n 1 "$TEMP_DIR/authenticated")" = '-q'
grep -Fxq -- '--config' "$TEMP_DIR/authenticated"
grep -Fxq -- '-' "$TEMP_DIR/authenticated"
grep -Fxq -- '--globoff' "$TEMP_DIR/authenticated"
grep -Fxq 'header = "Authorization: Bearer update-secret"' "$TEMP_DIR/authenticated.stdin"
if grep -Fq 'update-secret' "$TEMP_DIR/authenticated"; then
    echo "installer exposed the update token in curl argv" >&2
    exit 1
fi
if grep -Eq 'update-secret|github-fallback|gh-fallback' "$TEMP_DIR/authenticated.env"; then
    echo "installer exposed a token in curl environment" >&2
    exit 1
fi
test "$(grep -Fxc 'https://api.github.com/repos/abingooo/modelport/releases/latest' "$TEMP_DIR/authenticated")" -eq 1
if grep -Fq 'example.com/collect' "$TEMP_DIR/authenticated" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/authenticated" ||
    grep -Fq 'example.com/collect' "$TEMP_DIR/authenticated.stdin" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/authenticated.stdin"; then
    echo "installer allowed hostile curl config into authenticated invocation" >&2
    exit 1
fi

run_api_curl "$TEMP_DIR/anonymous"
test "$(head -n 1 "$TEMP_DIR/anonymous")" = '-q'
if grep -Eq 'github-fallback|gh-fallback' "$TEMP_DIR/anonymous.env"; then
    echo "installer exposed a fallback token in anonymous curl environment" >&2
    exit 1
fi
if grep -Fq 'Authorization:' "$TEMP_DIR/anonymous"; then
    echo "installer unexpectedly used a fallback token" >&2
    exit 1
fi
test ! -s "$TEMP_DIR/anonymous.stdin"
test "$(grep -Fxc 'https://api.github.com/repos/abingooo/modelport/releases/latest' "$TEMP_DIR/anonymous")" -eq 1
if grep -Fq 'example.com/collect' "$TEMP_DIR/anonymous" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/anonymous"; then
    echo "installer allowed hostile curl config into anonymous invocation" >&2
    exit 1
fi

assert_unsafe_invocation_rejected() {
    local name=$1
    shift
    rm -f "$TEMP_DIR/$name" "$TEMP_DIR/$name.stdin"
    if CURL_ARGS_LOG="$TEMP_DIR/$name" PATH="$TEMP_DIR:$PATH" UPDATE_GITHUB_TOKEN="update-secret" \
        bash -c 'source "$1"; shift; github_api_curl "$@"' bash \
        "$TEMP_DIR/github-api-curl.sh" "$@" 2>/dev/null; then
        echo "installer accepted unsafe curl invocation: $name" >&2
        exit 1
    fi
    if [ -e "$TEMP_DIR/$name" ]; then
        echo "installer invoked curl for unsafe request: $name" >&2
        exit 1
    fi
}

assert_unsafe_invocation_rejected non-api -s \
    "https://github.com/abingooo/modelport/releases/download/custom-v1.0.0.1/asset"
assert_unsafe_invocation_rejected mixed-host -s \
    "https://api.github.com/repos/abingooo/modelport/releases/latest" \
    "https://example.com/collect"
assert_unsafe_invocation_rejected multiple-api -s \
    "https://api.github.com/repos/abingooo/modelport/releases/latest" \
    "https://api.github.com/repos/abingooo/modelport/releases"
assert_unsafe_invocation_rejected url-option -s --url \
    "https://example.com/collect" \
    "https://api.github.com/repos/abingooo/modelport/releases/latest"

# Every installer release API request must use the scoped helper.
test "$(grep -c 'github_api_curl .*https://api.github.com/' "$ROOT_DIR/deploy/install.sh")" -eq 3

# Asset and checksum downloads must continue to call curl directly.
grep -Fq 'curl -sL "$download_url"' "$ROOT_DIR/deploy/install.sh"
grep -Fq 'curl -sL "$checksum_url"' "$ROOT_DIR/deploy/install.sh"

# The current release has no binary assets. All install/update entry points
# fail before invoking any of the retained legacy download helpers.
grep -Fq 'binary_distribution_unsupported' "$ROOT_DIR/deploy/install.sh"
grep -Fq 'It never falls back to an upstream Sub2API release.' "$ROOT_DIR/deploy/install.sh"
if grep -Eq 'Wei-Shaw/sub2api|weishaw/sub2api' "$ROOT_DIR/deploy/install.sh"; then
    echo "installer still references an upstream Sub2API distribution" >&2
    exit 1
fi

echo "install GitHub token checks passed"
