const MODELPORT_REPOSITORY = 'abingooo/modelport'
const CUSTOM_VERSION_PATTERN = /^\d+\.\d+\.\d+\.\d+$/

export function resolveCurrentReleaseUrl(
  currentVersion: string,
  officialReleaseUrl?: string
): string {
  const version = currentVersion.trim().replace(/^v/, '')
  if (CUSTOM_VERSION_PATTERN.test(version)) {
    return `https://github.com/${MODELPORT_REPOSITORY}/releases/tag/custom-v${version}`
  }

  return officialReleaseUrl && officialReleaseUrl !== '#' ? officialReleaseUrl : ''
}
