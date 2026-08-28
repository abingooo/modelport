import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HarborScene, { type HarborProvider } from '../HarborScene.vue'

const gradient = {
  addColorStop: vi.fn(),
}

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
  clip: vi.fn(),
  createLinearGradient: vi.fn(() => gradient),
  quadraticCurveTo: vi.fn(),
  save: vi.fn(),
  restore: vi.fn(),
  translate: vi.fn(),
  rotate: vi.fn(),
  scale: vi.fn(),
  setLineDash: vi.fn(),
  fillText: vi.fn(),
  fillStyle: '',
  strokeStyle: '',
  lineWidth: 1,
  globalAlpha: 1,
  font: '',
  textAlign: 'start',
  textBaseline: 'alphabetic',
}

const providers: HarborProvider[] = [
  { name: 'OpenAI', platform: 'openai', color: '#10a37f' },
  { name: 'Anthropic', platform: 'anthropic', color: '#d97757' },
  { name: 'Gemini', platform: 'gemini', color: '#4285f4' },
  { name: 'Antigravity', platform: 'antigravity', color: '#a855f7' },
  { name: 'DeepSeek', platform: 'deepseek', color: '#4d6bfe' },
  { name: '智谱AI', platform: 'zhipu', color: '#6366f1' },
  { name: 'Kimi', platform: 'kimi', color: '#0f8b8d', darkIcon: true },
  { name: 'Composite', platform: 'composite', color: '#06b6d4' },
]

function motionPreference(matches: boolean, removeEventListener = vi.fn()) {
  return {
    matches,
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener,
    dispatchEvent: vi.fn(),
  }
}

function mountScene() {
  return mount(HarborScene, {
    props: {
      dark: false,
      label: 'Animated graphic model harbor',
      providers,
    },
  })
}

describe('HarborScene', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      context as unknown as CanvasRenderingContext2D
    )
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 960,
      height: 720,
      top: 0,
      left: 0,
      right: 960,
      bottom: 720,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    vi.spyOn(window, 'matchMedia').mockReturnValue(motionPreference(true))
  })

  afterEach(() => vi.restoreAllMocks())

  it('draws an accessible nonblank 2D harbor with provider-colored cargo', async () => {
    const wrapper = mountScene()
    await flushPromises()

    expect(wrapper.attributes('data-scene-ready')).toBe('true')
    expect(wrapper.attributes('data-renderer')).toBe('canvas2d')
    expect(wrapper.attributes('data-provider-count')).toBe(String(providers.length))
    expect(wrapper.attributes('data-celestial-body')).toBe('sun')
    expect(wrapper.attributes('data-atmosphere')).toBe('clouds')
    expect(wrapper.attributes('data-crane-system')).toBe('integrated-sts')
    expect(wrapper.attributes('data-lighthouse-lit')).toBe('false')
    expect(wrapper.attributes('data-ship-count')).toBe('2')
    expect(wrapper.get('canvas').attributes('aria-label')).toBe('Animated graphic model harbor')
    expect(context.fillRect).toHaveBeenCalled()
    expect(context.createLinearGradient).toHaveBeenCalled()
    expect(gradient.addColorStop).toHaveBeenCalledWith(0, '#2c86df')
    expect(gradient.addColorStop).toHaveBeenCalledWith(1, '#0b55ae')
    expect(context.save.mock.calls.length).toBeGreaterThan(10)
    expect(context.restore.mock.calls.length).toBeGreaterThan(10)
    expect(context.fillText).toHaveBeenCalledWith('模型港', expect.any(Number), expect.any(Number))
    expect(context.quadraticCurveTo).toHaveBeenCalled()
    expect(context.arc).toHaveBeenCalled()
    expect(context.scale.mock.calls.some(([scaleX]) => Number(scaleX) > 0)).toBe(true)
    expect(context.scale.mock.calls.some(([scaleX]) => Number(scaleX) < 0)).toBe(true)
    expect(wrapper.findAll('.dock-model-badge')).toHaveLength(providers.length)
    expect(wrapper.find('.scene-copyright').exists()).toBe(false)
    expect(wrapper.find('.scene-telemetry').exists()).toBe(false)

    await wrapper.setProps({ dark: true })
    await flushPromises()
    expect(wrapper.attributes('data-celestial-body')).toBe('moon')
    expect(wrapper.attributes('data-atmosphere')).toBe('stars')
    expect(wrapper.attributes('data-moon-phase')).toBeTruthy()
    expect(Number(wrapper.attributes('data-moon-illumination'))).toBeGreaterThanOrEqual(0)
    expect(wrapper.attributes('data-lighthouse-lit')).toBe('true')
    expect(context.clip).toHaveBeenCalled()

    const callsBeforePointer = context.fillRect.mock.calls.length
    await wrapper.trigger('pointermove', { clientX: 740, clientY: 350 })
    expect(context.fillRect.mock.calls.length).toBeGreaterThan(callsBeforePointer)
    wrapper.unmount()
  })

  it('keeps a visible CSS harbor when canvas is unavailable', async () => {
    vi.mocked(HTMLCanvasElement.prototype.getContext).mockReturnValue(null)
    const wrapper = mountScene()
    await flushPromises()

    expect(wrapper.attributes('data-scene-ready')).toBe('true')
    expect(wrapper.attributes('data-renderer')).toBe('fallback')
    expect(wrapper.classes()).toContain('is-fallback')
    expect(wrapper.attributes('role')).toBe('img')
    expect(wrapper.attributes('aria-label')).toBe('Animated graphic model harbor')
    expect(wrapper.get('canvas').attributes('aria-hidden')).toBe('true')
    expect(wrapper.find('.fallback-harbor').exists()).toBe(true)
    expect(wrapper.find('.fallback-terminal').exists()).toBe(true)
    expect(wrapper.find('.fallback-gantry').exists()).toBe(true)
    expect(wrapper.findAll('.fallback-ship')).toHaveLength(2)
    wrapper.unmount()
  })

  it('enlarges fixed scene details without changing the large-screen canvas bounds', async () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 1920,
      height: 900,
      top: 0,
      left: 0,
      right: 1920,
      bottom: 900,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })

    const wrapper = mountScene()
    await flushPromises()

    expect(wrapper.attributes('data-visual-scale')).toBe('1.16')
    expect(wrapper.get('canvas').attributes('style')).toContain('width: 1920px')
    expect(wrapper.get('canvas').attributes('style')).toContain('height: 900px')
    expect(
      context.setTransform.mock.calls.some(([scaleX]) => (
        Number(scaleX) > Math.min(2, window.devicePixelRatio || 1)
      ))
    ).toBe(true)
    wrapper.unmount()
  })

  it('tracks reduced motion and removes its listener on unmount', async () => {
    const removeEventListener = vi.fn()
    vi.mocked(window.matchMedia).mockReturnValue(motionPreference(true, removeEventListener))
    const wrapper = mountScene()
    await flushPromises()

    expect(wrapper.classes()).toContain('is-reduced')
    wrapper.unmount()
    expect(removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })
})
