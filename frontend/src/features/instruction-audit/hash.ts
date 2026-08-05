export async function sha256Utf8(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value)
  const digest = await globalThis.crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
}

export async function resolveInstructionHashDigest(
  mode: 'digest' | 'plaintext',
  digest: string,
  plaintext: string,
): Promise<string> {
  if (mode === 'plaintext') {
    if (plaintext.length === 0) throw new Error('Plaintext cannot be empty')
    return sha256Utf8(plaintext)
  }
  return digest.trim().toLowerCase()
}
