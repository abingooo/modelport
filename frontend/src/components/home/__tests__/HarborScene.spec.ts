import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HarborScene from '../HarborScene.vue'

const context = {
  setTransform: vi.fn(), clearRect: vi.fn(), fillRect: vi.fn(), beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(),
  closePath: vi.fn(), fill: vi.fn(), stroke: vi.fn(), arc: vi.fn(), quadraticCurveTo: vi.fn(), save: vi.fn(), restore: vi.fn(),
  translate: vi.fn(), rotate: vi.fn(), scale: vi.fn(), setLineDash: vi.fn(),
  fillStyle: '', strokeStyle: '', lineWidth: 1, globalAlpha: 1,
}

describe('HarborScene', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 900, height: 600, top: 0, left: 0, right: 900, bottom: 600, x: 0, y: 0, toJSON: () => ({}),
    })
  })

  afterEach(() => vi.restoreAllMocks())

  it('draws a nonblank reduced-motion harbor and responds to pointer input', async () => {
    const wrapper = mount(HarborScene, { props: { dark: false, label: 'Animated harbor' } })
    await flushPromises()
    expect(wrapper.attributes('data-scene-ready')).toBe('true')
    expect(wrapper.get('canvas').attributes('aria-label')).toBe('Animated harbor')
    expect(context.fillRect).toHaveBeenCalled()
    expect(context.arc).toHaveBeenCalled()

    const callsBeforePointer = context.fillRect.mock.calls.length
    await wrapper.trigger('pointermove', { clientX: 700, clientY: 300 })
    expect(context.fillRect.mock.calls.length).toBeGreaterThan(callsBeforePointer)
  })
})
