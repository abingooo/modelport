#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

fail() {
  printf 'ModelPort release contract test failed: %s\n' "$1" >&2
  exit 1
}

workflow=.github/workflows/custom-image.yml
ci_workflow=.github/workflows/backend-ci.yml
upstream_workflow=.github/workflows/release.yml
github_slot_check=custom/release/assert-github-slots-absent
image_slot_check=custom/release/assert-image-tags-absent
release_environment_check=custom/release/assert-production-release-environment
restore_attestation_check=custom/release/assert-production-restore-attestation
portable_sha256=custom/release/sha256-stdin
github_release_publisher=custom/release/publish-github-release-create-only
github_release_publisher_contract=deploy/tests/modelport-github-release-publisher-test.sh
postgres_restore_tool=deploy/modelport-isolated-restore.sh
postgres_restore_contract=deploy/tests/modelport-isolated-restore-test.sh
postgres_restore_doc=deploy/MODELPORT_ISOLATED_RESTORE.md
redis_restore_tool=deploy/modelport-isolated-redis-restore.sh
redis_restore_contract=deploy/modelport-isolated-redis-restore-contract-test.sh
redis_restore_doc=deploy/MODELPORT_ISOLATED_REDIS_RESTORE.md
production_runbook=docs/MODELPORT_PRODUCTION_UPGRADE_RUNBOOK_CN.md
goal_prompt=docs/MODELPORT_GOAL_PROMPT_CN.md
crypto_compat_verifier=backend/scripts/verify-modelport-crypto-compat-vectors.sh
crypto_compat_manifest=backend/testdata/modelport_crypto_compat_v1.json
crypto_compat_harness=backend/testdata/modelport_crypto_compat_generator_test.go
version=$(tr -d '[:space:]' < custom/VERSION)
expected_upstream_sha=e8cb019fabf8b55199436229044cbf9aa7a82564
expected_gitleaks_version=8.30.1
expected_gitleaks_linux_x64_sha256=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
expected_govulncheck_version=v1.7.0
expected_actionlint_version=1.7.12
expected_actionlint_linux_x64_sha256=8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8
expected_shellcheck_version=0.11.0
expected_shellcheck_linux_x64_sha256=8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198

for required_release_dependency in \
  "$github_slot_check" "$image_slot_check" "$release_environment_check" \
  "$restore_attestation_check" "$portable_sha256" "$github_release_publisher" \
  "$github_release_publisher_contract"; do
  [ -x "$required_release_dependency" ] || \
    fail "release dependency is missing or not executable: $required_release_dependency"
  git ls-files --error-unmatch "$required_release_dependency" >/dev/null 2>&1 || \
    fail "release dependency is not tracked by Git: $required_release_dependency"
  if git check-ignore --no-index -q "$required_release_dependency"; then
    fail "release dependency is ignored: $required_release_dependency"
  fi
done

for required_restore_artifact in \
  "$postgres_restore_tool" "$postgres_restore_contract" "$postgres_restore_doc" \
  "$redis_restore_tool" "$redis_restore_contract" "$redis_restore_doc" \
  "$production_runbook"; do
  [ -s "$required_restore_artifact" ] || \
    fail "restore gate artifact is missing or empty: $required_restore_artifact"
  git ls-files --error-unmatch "$required_restore_artifact" >/dev/null 2>&1 || \
    fail "restore gate artifact is not tracked by Git: $required_restore_artifact"
  if git check-ignore --no-index -q "$required_restore_artifact"; then
    fail "restore gate artifact is ignored: $required_restore_artifact"
  fi
done

[ -s "$goal_prompt" ] || fail 'revised ModelPort Goal prompt is missing or empty'
git ls-files --error-unmatch "$goal_prompt" >/dev/null 2>&1 || \
  fail 'revised ModelPort Goal prompt is not tracked by Git'
if git check-ignore --no-index -q "$goal_prompt"; then
  fail 'revised ModelPort Goal prompt is ignored'
fi

for crypto_compat_artifact in \
  "$crypto_compat_verifier" "$crypto_compat_manifest" "$crypto_compat_harness"; do
  [ -s "$crypto_compat_artifact" ] || \
    fail "crypto compatibility artifact is missing or empty: $crypto_compat_artifact"
  git ls-files --error-unmatch "$crypto_compat_artifact" >/dev/null 2>&1 || \
    fail "crypto compatibility artifact is not tracked by Git: $crypto_compat_artifact"
  if git check-ignore --no-index -q "$crypto_compat_artifact"; then
    fail "crypto compatibility artifact is ignored: $crypto_compat_artifact"
  fi
done
[ -x "$crypto_compat_verifier" ] || \
  fail 'crypto compatibility verifier is not executable'

for required_restore_executable in \
  "$postgres_restore_tool" "$postgres_restore_contract" \
  "$redis_restore_tool" "$redis_restore_contract"; do
  [ -x "$required_restore_executable" ] || \
    fail "restore gate executable is missing execute permission: $required_restore_executable"
done

[ "$version" = '0.1.183.1' ] || fail 'custom/VERSION is not 0.1.183.1'
[ "$(tr -d '[:space:]' < backend/cmd/server/VERSION)" = "$version" ] || \
  fail 'backend and custom versions differ'
[ "$(tr -d '[:space:]' < backend/cmd/server/UPSTREAM_VERSION)" = '0.1.183' ] || \
  fail 'upstream version is not 0.1.183'
[ "$(git rev-parse 'v0.1.183^{commit}')" = "$expected_upstream_sha" ] || \
  fail 'Sub2API v0.1.183 does not resolve to the locked upstream commit'
[ -s "docs/releases/${version}.md" ] || fail 'release notes are missing'

for dockerfile in Dockerfile Dockerfile.goreleaser deploy/Dockerfile; do
  grep -Fqx 'LABEL maintainer="ModelPort Maintainers"' "$dockerfile" || \
    fail "ModelPort maintainer label is missing from $dockerfile"
  grep -Fqx 'LABEL description="ModelPort - Unified Multi-Model API Gateway"' "$dockerfile" || \
    fail "ModelPort legacy description label is missing from $dockerfile"
  grep -Fqx 'LABEL org.opencontainers.image.title="ModelPort"' "$dockerfile" || \
    fail "ModelPort OCI title label is missing from $dockerfile"
  grep -Fqx 'LABEL org.opencontainers.image.description="ModelPort - Unified Multi-Model API Gateway"' "$dockerfile" || \
    fail "ModelPort OCI description label is missing from $dockerfile"
  grep -Fqx 'LABEL org.opencontainers.image.authors="ModelPort Maintainers"' "$dockerfile" || \
    fail "ModelPort OCI authors label is missing from $dockerfile"
  grep -Fqx 'LABEL org.opencontainers.image.source="https://github.com/abingooo/modelport"' "$dockerfile" || \
    fail "ModelPort OCI source label is missing from $dockerfile"
done

grep -Fqx '  RELEASE_IMAGE: ghcr.io/abingooo/modelport' "$workflow" || \
  fail 'release image repository changed'
grep -Fqx '  EXPECTED_REPOSITORY: abingooo/modelport' "$workflow" || \
  fail 'release repository identity gate is missing'
grep -Fqx '  group: modelport-release' "$workflow" || \
  fail 'release workflow must serialize all publication attempts'
grep -Fqx '  LEGACY_MODELPORT_TAG: custom-v0.1.176.2' "$workflow" || \
  fail 'legacy compatibility tag changed'
for workflow_label in \
  '            maintainer=ModelPort Maintainers' \
  '            description=ModelPort - Unified Multi-Model API Gateway' \
  '            org.opencontainers.image.title=ModelPort' \
  '            org.opencontainers.image.description=ModelPort - Unified Multi-Model API Gateway' \
  '            org.opencontainers.image.authors=ModelPort Maintainers'; do
  grep -Fqx "$workflow_label" "$workflow" || \
    fail "explicit release image brand label is missing: $workflow_label"
done
for inspected_label in \
  maintainer description org.opencontainers.image.title \
  org.opencontainers.image.description org.opencontainers.image.authors \
  org.opencontainers.image.source; do
  inspection_count=$(grep -Fc "docker image inspect --format '{{index .Config.Labels \"${inspected_label}\"}}'" "$workflow" || true)
  [ "$inspection_count" -eq 2 ] || \
    fail "release workflow must verify $inspected_label on the candidate and pulled image"
done
grep -Fq 'make_latest:"false"' "$github_release_publisher" || \
  fail 'GitHub Release must not become the mutable latest release'
# shellcheck disable=SC2016 # The workflow expression is matched literally.
immutable_preflight_count=$(grep -Fc \
  'gh api --method GET "repos/${GITHUB_REPOSITORY}/immutable-releases"' "$workflow" || true)
[ "$immutable_preflight_count" -eq 2 ] || \
  fail 'workflow must preflight GitHub Immutable Releases early and immediately before publication'
grep -Fq "'.immutable' <<< \"\${release_json}\"" "$workflow" || \
  fail 'public verification must confirm release immutability'

base_image_count=$(grep -Ec '^  (NODE|GOLANG|ALPINE|POSTGRES|REDIS)_IMAGE: [^[:space:]]+@sha256:[0-9a-f]{64}$' "$workflow" || true)
[ "$base_image_count" -eq 5 ] || fail 'all five release base images must be pinned by digest'
grep -Fqx "  GITLEAKS_VERSION: $expected_gitleaks_version" "$workflow" || \
  fail 'Gitleaks release version is not pinned'
grep -Fqx "  GITLEAKS_LINUX_X64_SHA256: $expected_gitleaks_linux_x64_sha256" "$workflow" || \
  fail 'Gitleaks release archive checksum is not pinned'
grep -Fqx "  GOVULNCHECK_VERSION: $expected_govulncheck_version" "$workflow" || \
  fail 'govulncheck version is not pinned'
grep -Fq 'golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}' "$workflow" || \
  fail 'release workflow must install govulncheck at the pinned version'
grep -Fq 'grep -Fqx "Scanner: govulncheck@${GOVULNCHECK_VERSION}"' "$workflow" || \
  fail 'release workflow must verify the pinned govulncheck version'
grep -Fq '"${scanner_dir}/govulncheck" ./...' "$workflow" || \
  fail 'release workflow must run reachable govulncheck against the backend'
grep -Fq 'pnpm audit --prod --audit-level=high --json' "$workflow" || \
  fail 'release workflow must audit frontend production dependencies'
grep -Fq 'check_pnpm_audit_exceptions.py' "$workflow" || \
  fail 'release workflow must validate approved frontend audit exceptions'
grep -Fq -- '--exceptions ../.github/audit-exceptions.yml' "$workflow" || \
  fail 'release workflow must use the repository audit exception policy'
grep -Fqx "  ACTIONLINT_VERSION: $expected_actionlint_version" "$workflow" || \
  fail 'actionlint version is not pinned'
grep -Fqx "  ACTIONLINT_LINUX_X64_SHA256: $expected_actionlint_linux_x64_sha256" "$workflow" || \
  fail 'actionlint archive checksum is not pinned'
grep -Fqx "  SHELLCHECK_VERSION: $expected_shellcheck_version" "$workflow" || \
  fail 'ShellCheck version is not pinned'
grep -Fqx "  SHELLCHECK_LINUX_X64_SHA256: $expected_shellcheck_linux_x64_sha256" "$workflow" || \
  fail 'ShellCheck archive checksum is not pinned'
grep -Fq '"${actionlint_bin}" .github/workflows/*.yml' "$workflow" || \
  fail 'release workflow must execute actionlint over all workflows'
grep -Fq '"${shellcheck_bin}" -x -S warning' "$workflow" || \
  fail 'release workflow must execute ShellCheck over tracked shell scripts'
grep -Fq "git ls-files '*.sh' 'custom/release/*' 'custom/update/modelport-docker-updater'" "$workflow" || \
  fail 'release workflow must include the extensionless ModelPort updater in ShellCheck coverage'
grep -Fq 'run: go vet ./...' "$workflow" || \
  fail 'release workflow must run an explicit Go vet gate'
if grep -Fq 'cache-to: type=gha' "$workflow"; then
  fail 'release workflow must not write an external build cache before release gates'
fi

grep -Fqx '          scan-type: config' "$workflow" || \
  fail 'release quality gates must scan Dockerfile configuration'
grep -Fq "docker image inspect --format '{{.Config.User}}'" "$workflow" || \
  fail 'release image inspection must require the non-root image user'
grep -Fqx '          docker volume create modelport-smoke-data' "$workflow" || \
  fail 'release smoke test must exercise the application named volume'
grep -Fq 'docker exec modelport-smoke id -u' "$workflow" || \
  fail 'release smoke test must verify the running application UID'

for pipeline in "$ci_workflow" "$workflow"; do
  grep -Fq '/bin/bash deploy/tests/modelport-isolated-restore-test.sh' "$pipeline" || \
    fail "PostgreSQL restore contract is missing from $pipeline"
  grep -Fq '/bin/bash deploy/modelport-isolated-redis-restore-contract-test.sh' "$pipeline" || \
    fail "Redis restore contract is missing from $pipeline"
  grep -Fq "go test -race ./internal/service ./internal/repository ./internal/securityaudit -run 'Lottery|Instruction|Free|APIKey' -count=1" "$pipeline" || \
    fail "critical ModelPort race gate is missing from $pipeline"
  grep -Fq '/bin/sh backend/scripts/verify-modelport-crypto-compat-vectors.sh' "$pipeline" || \
    fail "historical crypto compatibility gate is missing from $pipeline"
  for frontend_command in \
    'pnpm run lint:check' 'pnpm run typecheck' 'pnpm run test:run' 'pnpm run build'; do
    grep -Fq "$frontend_command" "$pipeline" || \
      fail "complete frontend gate ($frontend_command) is missing from $pipeline"
  done
done
grep -Fq 'docker pull "${POSTGRES_IMAGE}"' "$workflow" || \
  fail 'release quality gates must preload the pinned PostgreSQL drill image'
grep -Fq 'docker pull "${REDIS_IMAGE}"' "$workflow" || \
  fail 'release quality gates must preload the pinned Redis drill image'
grep -Fq 'MODELPORT_POSTGRES_DRILL_IMAGE="${POSTGRES_IMAGE}"' "$workflow" || \
  fail 'release quality gates must run the real PostgreSQL empty restore drill'
grep -Fq 'MODELPORT_REDIS_DRILL_IMAGE="${REDIS_IMAGE}"' "$workflow" || \
  fail 'release quality gates must run the real Redis RDB/AOF restore drill'

for visual_gate_file in \
  frontend/playwright.config.ts frontend/e2e/home.visual.spec.ts \
  frontend/scripts/serve-visual.mjs; do
  [ -s "$visual_gate_file" ] || fail "Playwright visual gate file is missing: $visual_gate_file"
  git ls-files --error-unmatch "$visual_gate_file" >/dev/null 2>&1 || \
    fail "Playwright visual gate file is not tracked by Git: $visual_gate_file"
done

ruby - "$workflow" <<'RUBY' || \
  fail 'public release verification does not validate provenance and SBOM attestations'
require 'yaml'

workflow_path = ARGV.fetch(0)
workflow = YAML.safe_load(File.read(workflow_path), aliases: true)
verify_job = workflow.fetch('jobs').fetch('verify-release')
verify_steps = verify_job.fetch('steps')
login_steps = verify_steps.select do |step|
  step.fetch('uses', '') == 'docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9'
end
raise 'verify-release must contain exactly one pinned GHCR login step' unless
  login_steps.length == 1
login_step_index = verify_steps.index(login_steps.first)
attestation_steps = verify_steps.select do |step|
  step.fetch('name', '') == 'Verify public provenance and SBOM attestations'
end
raise 'verify-release must contain exactly one attestation verification step' unless
  attestation_steps.length == 1
run = attestation_steps.first.fetch('run', '').to_s
raise 'attestation verification must use gh attestation verify' unless
  run.scan('gh attestation verify').length == 1
raise 'attestation verification must check both provenance and SBOM' unless
  run.scan('verify_attestation').length == 3
raise 'provenance verification must require the SLSA v1 predicate' unless
  run.scan("verify_attestation --predicate-type 'https://slsa.dev/provenance/v1'").length == 1
raise 'SBOM verification must require the SPDX predicate' unless
  run.scan("verify_attestation --predicate-type 'https://spdx.dev/Document'").length == 1
raise 'attestation verification must specify the signer workflow exactly once' unless
  run.scan('--signer-workflow').length == 1
raise 'provenance verification must bind the release repository' unless
  run.include?('--repo "${GITHUB_REPOSITORY}"')
raise 'attestation verification must bind the custom-image workflow' unless
  run.include?('${GITHUB_REPOSITORY}/.github/workflows/custom-image.yml')
raise 'attestation verification must bind the production ref' unless
  run.include?('--source-ref "${GITHUB_REF}"')
raise 'attestation verification must bind the candidate commit' unless
  run.include?('--source-digest "${GITHUB_SHA}"')
raise 'attestation verification must use the read-only workflow token' unless
  attestation_steps.first.fetch('env', {})['GH_TOKEN'] == '${{ github.token }}'
raise 'verify-release must grant read-only attestation permission' unless
  verify_job.fetch('permissions', {})['attestations'] == 'read'
attestation_step_index = verify_steps.index(attestation_steps.first)
raise 'GHCR login must precede attestation verification' unless
  login_step_index < attestation_step_index
raise 'GHCR login must target ghcr.io' unless
  login_steps.first.fetch('with', {})['registry'] == 'ghcr.io'
raise 'GHCR login must use the workflow actor' unless
  login_steps.first.fetch('with', {})['username'] == '${{ github.actor }}'
raise 'GHCR login must use the workflow token' unless
  login_steps.first.fetch('with', {})['password'] == '${{ github.token }}'
raise 'attestation verification must use bounded retry' unless
  run.include?('for attempt in 1 2 3 4 5')
raise 'attestation verification retry backoff must be bounded' unless
  run.include?('sleep $((attempt * 5))') && run.include?('after five attempts')
RUBY

grep -Fq '"@playwright/test": "1.62.1"' frontend/package.json || \
  fail 'Playwright test dependency must remain exactly pinned'
grep -Fq 'pnpm exec playwright install --with-deps chromium' "$workflow" || \
  fail 'release workflow must install the pinned Playwright Chromium build'
grep -Fq 'pnpm run test:visual' "$workflow" || \
  fail 'release workflow must execute the Playwright visual gate'
grep -Fq 'frontend/playwright-report' "$workflow" || \
  fail 'release workflow must retain the Playwright HTML report'
grep -Fq 'frontend/test-results/visual' "$workflow" || \
  fail 'release workflow must retain Playwright screenshots and traces'
grep -Fqx '          if-no-files-found: error' "$workflow" || \
  fail 'release workflow must fail when Playwright evidence is missing'

ruby - "$workflow" "$expected_upstream_sha" <<'RUBY' || \
  fail 'release workflow job ownership or publication ordering is invalid'
require 'yaml'

workflow_path, expected_upstream_sha = ARGV
workflow = YAML.safe_load(File.read(workflow_path), aliases: true)
raise 'workflow root must be a mapping' unless workflow.is_a?(Hash)
raise 'UPSTREAM_SHA is not the locked Sub2API commit' unless
  workflow.dig('env', 'UPSTREAM_SHA') == expected_upstream_sha

jobs = workflow.fetch('jobs')
target_jobs = %w[quality image release]
environment_name = 'modelport-production-release'
environment_validator = '/bin/sh custom/release/assert-production-release-environment'
attestation_validator = '/bin/sh custom/release/assert-production-restore-attestation'
immutable_preflight = 'gh api --method GET "repos/${GITHUB_REPOSITORY}/immutable-releases"'

quality = jobs.fetch('quality')
quality_steps = quality.fetch('steps')
gitleaks_steps = quality_steps.each_index.select do |index|
  quality_steps.fetch(index).fetch('name', '') == 'Run pinned Gitleaks release gate'
end
raise 'quality must contain exactly one Gitleaks release gate' unless gitleaks_steps.length == 1
gitleaks_step_index = gitleaks_steps.first
gitleaks_step = quality_steps.fetch(gitleaks_step_index)
raise 'Gitleaks release gate must not tolerate failures' if
  gitleaks_step['continue-on-error'] == true
gitleaks_run = gitleaks_step.fetch('run', '').to_s
required_gitleaks_fragments = [
  '--connect-timeout 15 --max-time 120 --retry-max-time 180',
  'sha256sum --check --strict -',
  'test "$("${gitleaks_bin}" version)" = "${GITLEAKS_VERSION}"',
  'test -z "$(git status --porcelain=v1 --untracked-files=all)"',
  'git archive "${UPSTREAM_SHA}"',
  '--exit-code 42 --timeout 600 --report-format json',
  '.Secret == "REDACTED"',
  '--exit-code 1 --timeout 600 --baseline-path "${upstream_report}" .',
  '"${gitleaks_bin}" git --no-banner --no-color --redact=100',
  '--log-opts="${UPSTREAM_SHA}..${GITHUB_SHA}" .'
]
required_gitleaks_fragments.each do |fragment|
  raise "Gitleaks release gate is missing: #{fragment}" unless gitleaks_run.include?(fragment)
end
raise 'Gitleaks baseline must accept only clean or dedicated finding exit codes' unless
  gitleaks_run.include?('0|42) ;;')

checkout_step = quality_steps.fetch(0)
raise 'Gitleaks history scan requires a full checkout' unless
  checkout_step.fetch('with', {})['fetch-depth'] == 0

deployment_step_index = quality_steps.index do |step|
  step.fetch('name', '') == 'Deployment and updater tests'
end
playwright_upload_step_index = quality_steps.index do |step|
  step.fetch('name', '') == 'Upload Playwright visual evidence after quality gates'
end
raise 'quality must run deployment/updater contracts before uploading Playwright evidence' unless
  deployment_step_index && playwright_upload_step_index &&
    deployment_step_index < playwright_upload_step_index
raise 'quality must not contain a pre-gate Playwright artifact upload' if
  quality_steps.each_index.any? do |index|
    index < playwright_upload_step_index &&
      quality_steps.fetch(index).fetch('uses', '').to_s ==
        'actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a'
  end
raise 'release workflow must not configure cache-to: type=gha' unless
  (quality_steps + jobs.fetch('image').fetch('steps')).all? do |step|
    !step.fetch('run', '').to_s.include?('cache-to: type=gha') &&
      !step.fetch('with', {}).to_s.include?('cache-to: type=gha')
  end

raise 'quality must contain an explicit Go vet gate' unless
  quality_steps.any? do |step|
    step.fetch('name', '') == 'Backend vet' && step.fetch('run', '').to_s.include?('go vet ./...')
  end
raise 'quality must contain the pinned workflow and shell static gates' unless
  quality_steps.any? do |step|
    step.fetch('name', '') == 'Run pinned workflow and shell static gates' &&
      step.fetch('run', '').to_s.include?('actionlint_bin') &&
      step.fetch('run', '').to_s.include?('shellcheck_bin')
  end

%w[image release].each do |publishing_job_name|
  dependencies = Array(jobs.fetch(publishing_job_name)['needs'])
  raise "#{publishing_job_name} must wait for the Gitleaks quality gate" unless
    dependencies.include?('quality')
end

environment_owners = jobs.each_with_object([]) do |(name, job), owners|
  owners << name if job['environment'] == environment_name
end
raise 'protected Environment must belong only to quality, image, and release' unless
  environment_owners.sort == target_jobs.sort

target_jobs.each do |job_name|
  job = jobs.fetch(job_name)
  raise "#{job_name} must use the protected Environment" unless
    job['environment'] == environment_name
  steps = job.fetch('steps')
  runs = steps.map { |step| step.fetch('run', '').to_s }
  environment_calls = runs.sum { |run| run.scan(environment_validator).length }
  attestation_calls = runs.sum { |run| run.scan(attestation_validator).length }
  raise "#{job_name} must query and validate the Environment exactly once" unless
    environment_calls == 1
  raise "#{job_name} must validate the restore attestation exactly once" unless
    attestation_calls == 1
  environment_step = runs.index { |run| run.include?(environment_validator) }
  attestation_step = runs.index { |run| run.include?(attestation_validator) }
  environment_step_definition = steps.fetch(environment_step)
  expected_token = '${{ secrets.MODELPORT_RELEASE_ADMIN_TOKEN || github.token }}'
  raise "#{job_name} Environment query token is missing" unless
    environment_step_definition.fetch('env', {})['GH_TOKEN'] == expected_token

  case job_name
  when 'quality'
    validation_run = runs.fetch(environment_step)
    output_write = validation_run.index('${GITHUB_OUTPUT}')
    raise 'quality output write marker is missing' unless output_write
    raise 'quality Environment validation must precede its output write' unless
      validation_run.index(environment_validator) < output_write
    raise 'quality attestation validation must precede its output write' unless
      validation_run.index(attestation_validator) < output_write
  when 'image'
    registry_write_steps = steps.each_index.select do |index|
      step = steps.fetch(index)
      run = runs.fetch(index)
      step_options = step.fetch('with', {})
      run.lines.any? { |line| line.strip.start_with?('docker push ') } ||
        run.include?('cosign sign --yes') ||
        (step_options.is_a?(Hash) &&
          (step_options['push-to-registry'] == true || step_options['push'] == true))
    end
    raise 'image registry writes are missing' if registry_write_steps.empty?
    raise 'image Environment validation must precede every registry write' unless
      registry_write_steps.all? { |index| environment_step < index }
    raise 'image attestation validation must precede every registry write' unless
      registry_write_steps.all? { |index| attestation_step < index }
  when 'release'
    release_write_steps = steps.each_index.select do |index|
      step = steps.fetch(index)
      run = runs.fetch(index)
      step.fetch('uses', '').start_with?('softprops/action-gh-release@') ||
        run.include?('/bin/bash custom/release/publish-github-release-create-only') ||
        run.lines.any? do |line|
          command = line.strip
          command.start_with?('gh release create ') || command.start_with?('git push ')
        end
    end
    raise 'GitHub Release write must occur exactly once' unless release_write_steps.length == 1
    release_write_step = release_write_steps.first
    raise 'release Environment validation must precede the GitHub Release write' unless
      environment_step < release_write_step
    raise 'release attestation validation must precede the GitHub Release write' unless
      attestation_step < release_write_step
    release_write_run = runs.fetch(release_write_step)
    raise 'late Immutable Releases preflight must run in the GitHub Release write step' unless
      release_write_run.scan(immutable_preflight).length == 1
    immutable_preflight_offset = release_write_run.index(immutable_preflight)
    publisher_offset = release_write_run.index(
      '/bin/bash custom/release/publish-github-release-create-only'
    )
    raise 'late Immutable Releases preflight must precede the create-only publisher' unless
      immutable_preflight_offset && publisher_offset && immutable_preflight_offset < publisher_offset
    significant_lines = release_write_run.lines.map(&:strip).reject do |line|
      line.empty? || line.start_with?('#')
    end
    immutable_preflight_line = significant_lines.index do |line|
      line.include?(immutable_preflight)
    end
    publisher_line = significant_lines.index do |line|
      line.include?('/bin/bash custom/release/publish-github-release-create-only')
    end
    raise 'late Immutable Releases preflight must be immediately before the publisher' unless
      immutable_preflight_line && publisher_line == immutable_preflight_line + 1
    expected_immutable_token = '${{ secrets.MODELPORT_RELEASE_ADMIN_TOKEN || github.token }}'
    raise 'late Immutable Releases preflight token is missing' unless
      steps.fetch(release_write_step).fetch('env', {})['IMMUTABLE_RELEASES_TOKEN'] ==
        expected_immutable_token
    raise 'late Immutable Releases preflight must use the protected admin token' unless
      release_write_run.include?('GH_TOKEN="${IMMUTABLE_RELEASES_TOKEN}"')
  end
end

all_runs = jobs.values.flat_map do |job|
  job.fetch('steps', []).map { |step| step.fetch('run', '').to_s }
end
raise 'Environment validator leaked into an unexpected job' unless
  all_runs.sum { |run| run.scan(environment_validator).length } == target_jobs.length
raise 'attestation validator leaked into an unexpected job' unless
  all_runs.sum { |run| run.scan(attestation_validator).length } == target_jobs.length
RUBY
grep -Fq 'production_restore_attestation_sha256:' "$workflow" || \
  fail 'production restore attestation hash input is missing'
grep -Fq 'production_restore_attestation_utc:' "$workflow" || \
  fail 'production restore attestation timestamp input is missing'
grep -Fq 'confirm_wait_for_manual_update:' "$workflow" || \
  fail 'manual production update confirmation is missing'
grep -Fqx '        default: false' "$workflow" || \
  fail 'manual production update confirmation must default to false'
# shellcheck disable=SC2016 # Match the literal validator variable reference.
grep -Fq '[ "${manual_update_confirmation}" = true ]' "$restore_attestation_check" || \
  fail 'release validation must enforce the manual update boundary confirmation'
grep -Fq 'MODELPORT_PRODUCTION_RESTORE_ATTESTATION_SHA256' "$workflow" || \
  fail 'approved restore attestation environment secret gate is missing'
grep -Fq 'MODELPORT_PRODUCTION_RESTORE_ATTESTATION_BINDING_SHA256' "$workflow" || \
  fail 'approved restore attestation binding secret gate is missing'
grep -Fqx '  RESTORE_ATTESTATION_MAX_AGE_SECONDS: 86400' "$workflow" || \
  fail 'production restore attestation must expire after 24 hours'
# shellcheck disable=SC2016 # Match the literal workflow jq expression.
grep -Fq 'candidate_revision:$candidate_revision,upstream_revision:$upstream_revision' "$workflow" || \
  fail 'public restore attestation must bind candidate and upstream revisions'
grep -Fq '.can_admins_bypass == false' "$release_environment_check" || \
  fail 'production release environment must disable administrator bypass'
grep -Fq '.prevent_self_review == true' "$release_environment_check" || \
  fail 'production release environment must prevent self review'
grep -Fq 'modelport-production-release must require independent review without administrator bypass' \
  "$release_environment_check" || \
  fail 'independent required reviewer preflight is missing'
grep -Fq 'production-restore-attestation.json' "$workflow" || \
  fail 'non-sensitive restore attestation release asset is missing'
grep -Fq '/bin/bash custom/release/publish-github-release-create-only' "$workflow" || \
  fail 'create-only GitHub Release publisher is missing'
if grep -Fq 'softprops/action-gh-release@' "$workflow"; then
  fail 'an update-capable GitHub Release action must not publish ModelPort'
fi
grep -Fq 'draft:true' "$github_release_publisher" || \
  fail 'GitHub Release assets must be staged in a draft'
# shellcheck disable=SC2016 # Match the literal publisher status guard.
grep -Fq '[[ "$API_STATUS" == 201 ]]' "$github_release_publisher" || \
  fail 'create-only GitHub writes must require HTTP 201'
grep -Fq 'release_assets_are_owned' "$github_release_publisher" || \
  fail 'GitHub Release rollback must reject unknown assets'
published_immutable_check_count=$(grep -Fc \
  'release_identity_is_owned_published_immutable "' "$github_release_publisher" || true)
[ "$published_immutable_check_count" -eq 2 ] || \
  fail 'publisher must verify immutable identity in both PATCH response and final GET'
grep -Fq '.immutable == true' "$github_release_publisher" || \
  fail 'publisher immutable identity check must fail closed unless immutable is true'

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

attestation_binding_sha256() {
  binding_sha256=$1
  binding_version=$2
  binding_candidate=$3
  binding_upstream=$4
  binding_recorded_at=$5
  binding_json="$(jq -cn \
    --arg sha256 "$binding_sha256" \
    --arg version "$binding_version" \
    --arg candidate_revision "$binding_candidate" \
    --arg upstream_revision "$binding_upstream" \
    --arg recorded_at "$binding_recorded_at" \
    '{schema_version:1,sha256:$sha256,version:$version,
      candidate_revision:$candidate_revision,upstream_revision:$upstream_revision,
      recorded_at:$recorded_at}')" || return 1
  printf '%s' "$binding_json" | /bin/sh "$portable_sha256"
}

rfc3339_from_epoch() {
  ruby -rtime -e 'puts Time.at(Integer(ARGV.fetch(0))).utc.iso8601' "$1"
}

assert_attestation_rejected() {
  vector_name=$1
  expected_error=$2
  shift 2
  vector_error="$temporary_dir/attestation-${vector_name}.err"
  if /bin/sh "$restore_attestation_check" "$@" > /dev/null 2> "$vector_error"; then
    fail "restore attestation negative vector was accepted: $vector_name"
  fi
  grep -Fq "$expected_error" "$vector_error" || \
    fail "restore attestation negative vector reached the wrong rejection: $vector_name"
}

fixed_binding="$(jq -cn \
  --arg sha256 '0000000000000000000000000000000000000000000000000000000000000000' \
  --arg version '0.1.183.1' \
  --arg candidate_revision '1111111111111111111111111111111111111111' \
  --arg upstream_revision "$expected_upstream_sha" \
  --arg recorded_at '2026-08-29T00:00:00Z' \
  '{schema_version:1,sha256:$sha256,version:$version,
    candidate_revision:$candidate_revision,upstream_revision:$upstream_revision,
    recorded_at:$recorded_at}')"
fixed_binding_sha256="$(printf '%s' "$fixed_binding" | /bin/sh "$portable_sha256")"
[ "$fixed_binding_sha256" = '51bbac88e5e644eeb2e22b81d32bbd6735aacf9f6eec31077987fdbc8f5fe38a' ] || \
  fail 'restore attestation canonical binding vector changed'

current_revision=$(git rev-parse HEAD)
now_epoch=$(date -u '+%s')
current_recorded_at=$(rfc3339_from_epoch "$now_epoch")
current_attestation_sha256='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
current_binding_sha256="$(attestation_binding_sha256 \
  "$current_attestation_sha256" "$version" "$current_revision" \
  "$expected_upstream_sha" "$current_recorded_at")"
/bin/sh "$restore_attestation_check" \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  "$current_binding_sha256" 86400 true || \
  fail 'valid restore attestation binding was rejected'

assert_attestation_rejected invalid-binding \
  'restore attestation binding does not match the approved value' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' \
  86400 true

assert_attestation_rejected manual-false \
  'manual production update boundary was not confirmed' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  "$current_binding_sha256" 86400 false

future_recorded_at=$(rfc3339_from_epoch "$((now_epoch + 3600))")
future_binding_sha256="$(attestation_binding_sha256 \
  "$current_attestation_sha256" "$version" "$current_revision" \
  "$expected_upstream_sha" "$future_recorded_at")"
assert_attestation_rejected future \
  'restore attestation timestamp is in the future' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$future_recorded_at" "$current_attestation_sha256" \
  "$future_binding_sha256" 86400 true

current_commit_epoch=$(git show -s --format=%ct "$current_revision")
predates_recorded_at=$(rfc3339_from_epoch "$((current_commit_epoch - 1))")
predates_binding_sha256="$(attestation_binding_sha256 \
  "$current_attestation_sha256" "$version" "$current_revision" \
  "$expected_upstream_sha" "$predates_recorded_at")"
assert_attestation_rejected predates-candidate \
  'restore attestation predates the candidate commit' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$predates_recorded_at" "$current_attestation_sha256" \
  "$predates_binding_sha256" 86400 true

stale_recorded_at=$(rfc3339_from_epoch "$((now_epoch - 86401))")
upstream_commit_epoch=$(git show -s --format=%ct "$expected_upstream_sha")
[ "$upstream_commit_epoch" -le "$((now_epoch - 86401))" ] || \
  fail 'locked upstream commit is too recent for the >24h attestation vector'
stale_binding_sha256="$(attestation_binding_sha256 \
  "$current_attestation_sha256" "$version" "$expected_upstream_sha" \
  "$expected_upstream_sha" "$stale_recorded_at")"
assert_attestation_rejected older-than-24h \
  'restore attestation is older than the allowed release window' \
  "$version" "$expected_upstream_sha" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$stale_recorded_at" "$current_attestation_sha256" \
  "$stale_binding_sha256" 86400 true

assert_attestation_rejected max-age-over-24h \
  'restore attestation maximum age exceeds 24 hours' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  "$current_binding_sha256" 86401 true

assert_attestation_rejected max-age-overflow \
  'restore attestation maximum age exceeds 24 hours' \
  "$version" "$current_revision" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  "$current_binding_sha256" \
  999999999999999999999999999999999999999999999 true

unknown_candidate=0000000000000000000000000000000000000000
unknown_binding_sha256="$(attestation_binding_sha256 \
  "$current_attestation_sha256" "$version" "$unknown_candidate" \
  "$expected_upstream_sha" "$current_recorded_at")"
assert_attestation_rejected unknown-candidate \
  'candidate revision is not available in the checkout' \
  "$version" "$unknown_candidate" "$expected_upstream_sha" \
  "$current_attestation_sha256" "$current_recorded_at" "$current_attestation_sha256" \
  "$unknown_binding_sha256" 86400 true

portable_hash_test_dir="$temporary_dir/portable-hash"
mkdir -p "$portable_hash_test_dir/both" "$portable_hash_test_dir/shasum-only" \
  "$portable_hash_test_dir/missing"
cat > "$portable_hash_test_dir/both/sha256sum" <<'EOF'
#!/bin/sh
echo '1111111111111111111111111111111111111111111111111111111111111111  -'
EOF
cat > "$portable_hash_test_dir/both/shasum" <<'EOF'
#!/bin/sh
echo '2222222222222222222222222222222222222222222222222222222222222222  -'
EOF
cp "$portable_hash_test_dir/both/shasum" "$portable_hash_test_dir/shasum-only/shasum"
chmod 0755 "$portable_hash_test_dir/both/sha256sum" \
  "$portable_hash_test_dir/both/shasum" "$portable_hash_test_dir/shasum-only/shasum"
[ "$(printf test | PATH="$portable_hash_test_dir/both" /bin/sh "$portable_sha256")" = \
  '1111111111111111111111111111111111111111111111111111111111111111' ] || \
  fail 'portable SHA-256 helper did not prefer sha256sum'
[ "$(printf test | PATH="$portable_hash_test_dir/shasum-only" /bin/sh "$portable_sha256")" = \
  '2222222222222222222222222222222222222222222222222222222222222222' ] || \
  fail 'portable SHA-256 helper did not fall back to shasum'
if printf test | PATH="$portable_hash_test_dir/missing" \
  /bin/sh "$portable_sha256" > /dev/null 2> "$portable_hash_test_dir/missing.err"; then
  fail 'portable SHA-256 helper accepted an environment without a hash tool'
fi
grep -Fq 'SHA-256 tool unavailable' "$portable_hash_test_dir/missing.err" || \
  fail 'portable SHA-256 helper did not report the missing tool clearly'

if grep -Eq '(^|[[:space:]])(ssh|scp|rsync)([[:space:]]|$)|MODELPORT_PRODUCTION_(HOST|SSH)|update-request' "$workflow"; then
  fail 'release workflow must not contain production connection or update-request operations'
fi

postgres_contract_line=$(grep -nF '/bin/bash deploy/tests/modelport-isolated-restore-test.sh' "$workflow" | head -n 1 | cut -d: -f1)
redis_contract_line=$(grep -nF '/bin/bash deploy/modelport-isolated-redis-restore-contract-test.sh' "$workflow" | head -n 1 | cut -d: -f1)
# shellcheck disable=SC2016 # Match the literal workflow variable reference.
image_publish_line=$(grep -nF 'docker push "${commit_ref}"' "$workflow" | head -n 1 | cut -d: -f1)
[ "$postgres_contract_line" -lt "$image_publish_line" ] || \
  fail 'PostgreSQL restore contract must run before image publication'
[ "$redis_contract_line" -lt "$image_publish_line" ] || \
  fail 'Redis restore contract must run before image publication'

if grep -Eq '(^|[[:space:]])[^#]*:latest([[:space:]]|$)' "$workflow"; then
  fail 'ModelPort workflow must not publish or consume latest image tags'
fi

if awk '
  /^[[:space:]]*uses:/ {
    ref = $0
    sub(/^.*uses:[[:space:]]*/, "", ref)
    sub(/[[:space:]]*#.*/, "", ref)
    if (ref !~ /@[0-9a-f]{40}$/) exit 1
  }
' "$workflow"; then
  :
else
  fail 'every action dependency must be pinned to a 40-character commit'
fi

for workflow_file in .github/workflows/*.yml .github/workflows/*.yaml; do
  [ -f "$workflow_file" ] || continue
  if awk '
    /^[[:space:]]*uses:/ {
      ref = $0
      sub(/^.*uses:[[:space:]]*/, "", ref)
      sub(/[[:space:]]*#.*/, "", ref)
      if (ref !~ /@[0-9a-f]{40}$/) exit 1
    }
  ' "$workflow_file"; then
    :
  else
    fail "every action dependency must be pinned in $workflow_file"
  fi
done

gate_count=$(grep -Fc "github.repository == 'Wei-Shaw/sub2api'" "$upstream_workflow" || true)
[ "$gate_count" -eq 4 ] || fail 'all four upstream release jobs must be disabled in ModelPort'

github_slot_check_count=$(grep -Fc '/bin/sh custom/release/assert-github-slots-absent' "$workflow" || true)
[ "$github_slot_check_count" -eq 2 ] || fail 'GitHub slots must be checked before build and before release'
image_slot_check_count=$(grep -Fc '/bin/sh custom/release/assert-image-tags-absent' "$workflow" || true)
[ "$image_slot_check_count" -eq 2 ] || \
  fail 'each GHCR release tag must have its own last-moment fail-closed check'
# shellcheck disable=SC2016 # Match literal workflow commands and verify their order.
commit_check_line=$(grep -nF '/bin/sh custom/release/assert-image-tags-absent "${commit_ref}"' "$workflow" | cut -d: -f1)
# shellcheck disable=SC2016 # Match literal workflow commands and verify their order.
commit_push_line=$(grep -nF 'docker push "${commit_ref}"' "$workflow" | cut -d: -f1)
# shellcheck disable=SC2016 # Match literal workflow commands and verify their order.
version_check_line=$(grep -nF '/bin/sh custom/release/assert-image-tags-absent "${version_ref}"' "$workflow" | cut -d: -f1)
version_create_line=$(grep -nF 'docker buildx imagetools create' "$workflow" | cut -d: -f1)
[ "$commit_push_line" -eq "$((commit_check_line + 1))" ] || \
  fail 'SHA tag absence must be checked immediately before its staging push'
[ "$commit_push_line" -lt "$version_check_line" ] || \
  fail 'the verified SHA staging anchor must exist before checking the version tag'
[ "$version_create_line" -eq "$((version_check_line + 1))" ] || \
  fail 'version tag absence must be checked immediately before its digest-only write'
# shellcheck disable=SC2016 # Match the literal workflow variable reference.
if grep -Fq 'docker push "${version_ref}"' "$workflow"; then
  fail 'the version tag must be created only from the verified staging digest'
fi
# shellcheck disable=SC2016 # Match the literal workflow digest reference.
grep -Fq '"${RELEASE_IMAGE}@${staging_digest}"' "$workflow" || \
  fail 'the version tag source is not bound to the verified staging digest'
grep -Fq 'GHCR exposes no documented atomic create-only manifest-tag write' "$workflow" || \
  fail 'the unavoidable GHCR tag race boundary must remain explicit'
grep -Fq 'GitHub tag/Release 发布使用 create-only REST 请求' "$production_runbook" || \
  fail 'the production runbook does not document create-only GitHub publication'
grep -Fq '不能原子消除外部写入者' "$production_runbook" || \
  fail 'the production runbook must disclose the residual GHCR tag race'
# shellcheck disable=SC2016 # The workflow command is matched literally.
grep -Fq 'created="$(git show -s --format=%cI "${GITHUB_SHA}")"' "$workflow" || \
  fail 'image creation time must come from the exact release commit'

/bin/sh -n "$github_slot_check"
/bin/sh -n "$image_slot_check"
/bin/sh -n "$release_environment_check"
/bin/sh -n "$restore_attestation_check"
/bin/sh -n "$portable_sha256"
/bin/bash -n "$github_release_publisher"
/bin/bash -n "$github_release_publisher_contract"
/bin/bash -n "$postgres_restore_contract"
/bin/bash -n "$redis_restore_contract"

mock_bin="$temporary_dir/bin"
mkdir -p "$mock_bin"

cat > "$mock_bin/git" <<'EOF'
#!/bin/sh
exit "${MODELPORT_TEST_GIT_STATUS:-2}"
EOF
cat > "$mock_bin/curl" <<'EOF'
#!/bin/sh
cat >/dev/null
case "${MODELPORT_TEST_CURL_MODE:-404}" in
  network) exit 7 ;;
  *) printf '%s' "${MODELPORT_TEST_CURL_MODE:-404}" ;;
esac
EOF
cat > "$mock_bin/docker" <<'EOF'
#!/bin/sh
case "${MODELPORT_TEST_DOCKER_MODE:-absent}" in
  exists) exit 0 ;;
  absent)
    image_path="${4#ghcr.io/}"
    image_repository="${image_path%%:*}"
    image_tag="${image_path#*:}"
    echo "ERROR: failed to do request: Head \"https://ghcr.io/v2/${image_repository}/manifests/${image_tag}\": 404 Not Found" >&2
    echo 'time="2026-08-31T18:25:06+08:00" level=fatal msg="exit status 1"' >&2
    exit 1
    ;;
  manifest_unknown) echo 'manifest unknown' >&2; exit 1 ;;
  buildx_not_found)
    echo "ERROR: $4: not found" >&2
    echo 'time="2026-08-31T18:25:06+08:00" level=fatal msg="exit status 1"' >&2
    exit 1
    ;;
  head_404)
    image_path="${4#ghcr.io/}"
    image_repository="${image_path%%:*}"
    image_tag="${image_path#*:}"
    echo "ERROR: failed to do request: Head \"https://ghcr.io/v2/${image_repository}/manifests/${image_tag}\": 404 Not Found" >&2
    echo 'time="2026-08-31T18:25:06+08:00" level=fatal msg="exit status 1"' >&2
    exit 1
    ;;
  head_404_wrong_ref)
    echo 'ERROR: failed to do request: Head "https://ghcr.io/v2/other/model/manifests/custom-v0.1.183.1": 404 Not Found' >&2
    echo 'time="2026-08-31T18:25:06+08:00" level=fatal msg="exit status 1"' >&2
    exit 1
    ;;
  unauthorized) echo 'unexpected status from HEAD request: 401 Unauthorized' >&2; exit 1 ;;
  network) echo 'dial tcp: network is unreachable' >&2; exit 1 ;;
  token_404) echo 'unexpected status from token request: 404 Not Found' >&2; exit 1 ;;
  mixed_404_unauthorized)
    echo 'unexpected status from HEAD request: 404 Not Found' >&2
    echo 'token request failed: 401 Unauthorized' >&2
    exit 1
    ;;
  mixed_manifest_network)
    echo 'manifest unknown' >&2
    echo 'dial tcp: network is unreachable' >&2
    exit 1
    ;;
  version_404_unauthorized) echo 'ghcr.io/abingooo/modelport:custom-v0.1.404.1: 401 Unauthorized' >&2; exit 1 ;;
  sha_404_unauthorized) echo 'ghcr.io/abingooo/modelport:sha-a404b123456789abcdef0123456789abcdef0123: 401 Unauthorized' >&2; exit 1 ;;
esac
EOF
cat > "$mock_bin/gh" <<'EOF'
#!/bin/sh
[ "$#" -eq 4 ] && [ "$1" = api ] && [ "$2" = --method ] && [ "$3" = GET ] && \
  [ "$4" = 'repos/abingooo/modelport/environments/modelport-production-release' ] || exit 64
case "${MODELPORT_TEST_GH_ENV_MODE:-valid}" in
  valid)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":123,"login":"independent-reviewer"}}]}]}'
    ;;
  admin-bypass)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":true,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":123}}]}]}'
    ;;
  self-review)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":false,"reviewers":[{"type":"User","reviewer":{"id":123}}]}]}'
    ;;
  no-reviewer)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[]}]}'
    ;;
  no-required-reviewer-rule)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"wait_timer","wait_timer":0}]}'
    ;;
  duplicate-required-reviewer-rule)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":123}}]},{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"Team","reviewer":{"id":456}}]}]}'
    ;;
  wrong-name)
    printf '%s\n' '{"name":"another-environment","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":123}}]}]}'
    ;;
  malformed-reviewer)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{}}]}]}'
    ;;
  reviewer-id-zero)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":0}}]}]}'
    ;;
  reviewer-id-negative)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":-1}}]}]}'
    ;;
  reviewer-id-fractional)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":1.5}}]}]}'
    ;;
  reviewer-id-string)
    printf '%s\n' '{"name":"modelport-production-release","can_admins_bypass":false,"protection_rules":[{"type":"required_reviewers","prevent_self_review":true,"reviewers":[{"type":"User","reviewer":{"id":"123"}}]}]}'
    ;;
  malformed) printf '%s\n' 'not-json' ;;
  api-error) exit 1 ;;
  *) exit 65 ;;
esac
EOF
chmod 0755 "$mock_bin/git" "$mock_bin/curl" "$mock_bin/docker" "$mock_bin/gh"

run_github_slot_check() {
  PATH="$mock_bin:$PATH" \
  GH_TOKEN=modelport-test-token \
  MODELPORT_TEST_GIT_STATUS="${MODELPORT_TEST_GIT_STATUS:-2}" \
  MODELPORT_TEST_CURL_MODE="${MODELPORT_TEST_CURL_MODE:-404}" \
    /bin/sh "$github_slot_check" origin custom-v0.1.183.1 abingooo/modelport
}

run_image_slot_check() {
  PATH="$mock_bin:$PATH" \
  MODELPORT_TEST_DOCKER_MODE="${MODELPORT_TEST_DOCKER_MODE:-absent}" \
    /bin/sh "$image_slot_check" \
      ghcr.io/abingooo/modelport:custom-v0.1.183.1 \
      ghcr.io/abingooo/modelport:sha-0123456789abcdef0123456789abcdef01234567
}

run_environment_check() {
  PATH="$mock_bin:$PATH" \
  GH_TOKEN=modelport-test-token \
  MODELPORT_TEST_GH_ENV_MODE="${MODELPORT_TEST_GH_ENV_MODE:-valid}" \
    /bin/sh "$release_environment_check" abingooo/modelport
}

MODELPORT_TEST_GH_ENV_MODE=valid run_environment_check >/dev/null || \
  fail 'valid production release Environment was rejected'
if PATH="$mock_bin:$PATH" GH_TOKEN='' MODELPORT_TEST_GH_ENV_MODE=valid \
  /bin/sh "$release_environment_check" abingooo/modelport >/dev/null 2>&1; then
  fail 'production release Environment query accepted a missing token'
fi
for environment_mode in \
  admin-bypass self-review no-reviewer no-required-reviewer-rule \
  duplicate-required-reviewer-rule wrong-name malformed-reviewer malformed api-error; do
  if MODELPORT_TEST_GH_ENV_MODE="$environment_mode" \
    run_environment_check >/dev/null 2>&1; then
    fail "unsafe production release Environment was accepted: $environment_mode"
  fi
done
for environment_mode in reviewer-id-zero reviewer-id-negative \
  reviewer-id-fractional reviewer-id-string; do
  if MODELPORT_TEST_GH_ENV_MODE="$environment_mode" \
    run_environment_check >/dev/null 2>&1; then
    fail "non-positive or non-integral reviewer ID was accepted: $environment_mode"
  fi
done

MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE=404 run_github_slot_check >/dev/null || \
  fail 'confirmed absent GitHub slots were rejected'
if MODELPORT_TEST_GIT_STATUS=0 run_github_slot_check >/dev/null 2>&1; then
  fail 'an existing Git tag was treated as absent'
fi
if MODELPORT_TEST_GIT_STATUS=128 run_github_slot_check >/dev/null 2>&1; then
  fail 'a Git transport failure was treated as absence'
fi
if MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE=200 run_github_slot_check >/dev/null 2>&1; then
  fail 'an existing GitHub Release was treated as absent'
fi
for curl_mode in 403 500 network; do
  if MODELPORT_TEST_GIT_STATUS=2 MODELPORT_TEST_CURL_MODE="$curl_mode" run_github_slot_check >/dev/null 2>&1; then
    fail "GitHub API failure $curl_mode was treated as absence"
  fi
done

MODELPORT_TEST_DOCKER_MODE=absent run_image_slot_check >/dev/null || \
  fail 'a confirmed 404 GHCR response was rejected'
MODELPORT_TEST_DOCKER_MODE=manifest_unknown run_image_slot_check >/dev/null || \
  fail 'a confirmed manifest-unknown GHCR response was rejected'
MODELPORT_TEST_DOCKER_MODE=buildx_not_found run_image_slot_check >/dev/null || \
  fail 'the current Buildx not-found response was rejected'
MODELPORT_TEST_DOCKER_MODE=head_404 run_image_slot_check >/dev/null || \
  fail 'a direct HEAD 404 GHCR response was rejected'
for docker_mode in \
  exists unauthorized network token_404 mixed_404_unauthorized mixed_manifest_network \
  version_404_unauthorized sha_404_unauthorized head_404_wrong_ref; do
  if MODELPORT_TEST_DOCKER_MODE="$docker_mode" run_image_slot_check >/dev/null 2>&1; then
    fail "GHCR state $docker_mode was treated as absence"
  fi
done

/bin/bash "$github_release_publisher_contract"

printf 'ModelPort release contract test passed\n'
