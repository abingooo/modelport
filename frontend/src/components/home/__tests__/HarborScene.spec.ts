import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HarborScene from '../HarborScene.vue'

const context = {
  setTransform: vi.fn(),
  clearRect: vi.fn(),
  fillRect: vi.fn(),
  beginPath: vi.fn(),
  moveTo: vi.fn(),
  lineTo: vi.fn(),
  closePath: vi.fn(),
  fill: vi.fn(),
  stroke: vi.fn(),
  arc: vi.fn(),
  quadraticCurveTo: vi.fn(),
  save: vi.fn(),
  restore: vi.fn(),
  translate: vi.fn(),
  rotate: vi.fn(),
  scale: vi.fn(),
  setLineDash: vi.fn(),
  fillStyle: '',
  strokeStyle: '',
  shadowColor: '',
  shadowBlur: 0,
  lineWidth: 1,
  globalAlpha: 1,
}

describe('HarborScene', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      context as unknown as CanvasRenderingContext2D
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 960,
      height: 620,
      top: 0,
      left: 0,
      right: 960,
      bottom: 620,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
  })

  afterEach(() => vi.restoreAllMocks())

  it('draws routes, vessels, and the lighthouse in reduced-motion mode', async () => {
    const wrapper = mount(HarborScene, {
      props: { dark: false, label: 'Animated model harbor' },
    })
    await flushPromises()

    expect(wrapper.attributes('data-scene-ready')).toBe('true')
    expect(wrapper.get('canvas').attributes('aria-label')).toBe('Animated model harbor')
    expect(context.quadraticCurveTo.mock.calls.length).toBeGreaterThanOrEqual(3)
    expect(context.rotate).toHaveBeenCalled()
    expect(context.arc).toHaveBeenCalled()

    const drawsBeforePointer = context.fill.mock.calls.length
    await wrapper.trigger('pointermove', { clientX: 700, clientY: 300 })
    expect(context.fill.mock.calls.length).toBeGreaterThan(drawsBeforePointer)
  })
})
