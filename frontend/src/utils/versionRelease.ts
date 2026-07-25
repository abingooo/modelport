const MODELPORT_REPOSITORY = 'abingooo/modelport'
const CUSTOM_STABLE_VERSION_PATTERN = /^\d+\.\d+\.\d+\.\d+$/
const CUSTOM_PRERELEASE_VERSION_PATTERN =
  /^\d+\.\d+\.\d+\.\d+-(?:dev|beta|rc)\.\d+$/

export function resolveCurrentReleaseUrl(
  currentVersion: string,
  officialReleaseUrl?: string
): string {
  const version = currentVersion.trim().replace(/^v/, '')
  if (CUSTOM_STABLE_VERSION_PATTERN.test(version)) {
    return `https://github.com/${MODELPORT_REPOSITORY}/releases/tag/custom-v${version}`
  }

  if (CUSTOM_PRERELEASE_VERSION_PATTERN.test(version)) {
    return `https://github.com/${MODELPORT_REPOSITORY}/releases/tag/dev-v${version}`
  }

  return officialReleaseUrl && officialReleaseUrl !== '#' ? officialReleaseUrl : ''
}
