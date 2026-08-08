import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')

describe('AppHeader responsive sizing', () => {
  it('lets the page title shrink before the action toolbar', () => {
    expect(headerSource).toContain('class="flex min-w-0 flex-1 items-center gap-2 sm:gap-4"')
    expect(headerSource).toContain('class="flex shrink-0 items-center gap-1 sm:gap-3"')
    expect(headerSource).toContain('class="truncate text-lg font-semibold')
    expect(headerSource).toContain('class="truncate text-xs')
  })

  it('bounds long display names in the user menu', () => {
    expect(headerSource).toContain('class="hidden min-w-0 max-w-32 text-left md:block"')
    expect(headerSource).toContain('class="truncate text-sm font-medium')
  })
})
