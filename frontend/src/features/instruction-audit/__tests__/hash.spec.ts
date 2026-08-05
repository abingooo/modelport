import { createHash, webcrypto } from 'node:crypto'
import { beforeAll, describe, expect, it } from 'vitest'
import { resolveInstructionHashDigest, sha256Utf8 } from '../hash'

beforeAll(() => {
  if (!globalThis.crypto?.subtle) {
    Object.defineProperty(globalThis, 'crypto', { configurable: true, value: webcrypto })
  }
})

describe('instruction audit browser hashing', () => {
  it('hashes exact UTF-8 text without normalization', async () => {
    const value = ' Line\n模型港 '
    const expected = createHash('sha256').update(value, 'utf8').digest('hex')
    expect(await sha256Utf8(value)).toBe(expected)
    expect(await sha256Utf8(value)).not.toBe(await sha256Utf8(value.trim()))
  })

  it('rejects empty plaintext and normalizes a supplied digest', async () => {
    await expect(resolveInstructionHashDigest('plaintext', '', '')).rejects.toThrow('Plaintext cannot be empty')
    await expect(resolveInstructionHashDigest('digest', ' ABCD ', '')).resolves.toBe('abcd')
  })
})
