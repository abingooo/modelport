<template>
  <div
    ref="host"
    class="graphic-harbor"
    :class="{ 'is-dark': dark, 'is-reduced': reducedMotion, 'is-fallback': failed }"
    :data-scene-ready="ready ? 'true' : 'false'"
    :data-renderer="failed ? 'fallback' : 'canvas2d'"
    :data-visual-scale="sceneScale.toFixed(2)"
    :data-provider-count="resolvedProviders.length"
    :data-celestial-body="dark ? 'moon' : 'sun'"
    :data-moon-phase="dark ? moonPhase.name : undefined"
    :data-moon-illumination="dark ? moonPhase.illumination.toFixed(3) : undefined"
    :data-lighthouse-lit="dark ? 'true' : 'false'"
    :data-atmosphere="dark ? 'stars' : 'clouds'"
    data-crane-system="integrated-sts"
    data-ship-count="2"
    :style="{ '--terminal-offset': `${terminalOffset}px` }"
    @pointermove="handlePointerMove"
    @pointerleave="resetPointer"
  >
    <div class="fallback-harbor" aria-hidden="true">
      <span class="fallback-atmosphere"></span>
      <span
        class="fallback-celestial"
        :style="{ '--moon-shadow-offset': fallbackMoonShadowOffset }"
      ></span>
      <span class="fallback-water"></span>
      <span class="fallback-terminal"></span>
      <span class="fallback-gantry"></span>
      <span class="fallback-dock"></span>
      <span class="fallback-lighthouse"></span>
      <span class="fallback-ship fallback-ship-inbound"></span>
      <span class="fallback-ship fallback-ship-outbound"></span>
    </div>

    <canvas ref="canvas" role="img" :aria-label="label" />

    <div class="dock-model-badges" aria-hidden="true">
      <span
        v-for="(provider, index) in resolvedProviders"
        :key="provider.platform"
        class="dock-model-badge"
        :class="{ 'needs-dark-icon': provider.darkIcon }"
        :data-platform="provider.platform"
        :style="{ '--provider-color': provider.color, '--badge-index': index }"
      >
        <PlatformIcon :platform="provider.platform" size="xs" />
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { getMoonPhase, type MoonPhase } from '@/utils/moonPhase'

export interface HarborProvider {
  name: string
  platform: GroupPlatform
  color: string
  darkIcon?: boolean
}

interface Props {
  dark: boolean
  label: string
  providers?: HarborProvider[]
}

interface Palette {
  sky: string
  skyBand: string
  haze: string
  cloud: string
  star: string
  city: string
  citySoft: string
  water: string
  waterDeep: string
  waterLine: string
  ink: string
  structure: string
  structureSoft: string
  dock: string
  dockTop: string
  dockEdge: string
  cream: string
  coral: string
  mint: string
  yellow: string
}

interface HarborLayout {
  compact: boolean
  horizonY: number
  terminalOffsetY: number
  terminalLeft: number
  terminalRight: number
  deckY: number
  deckThickness: number
  dockFootX: number
  lighthouseX: number
  lighthouseBaseY: number
  lighthouseHeight: number
}

interface StarPoint {
  x: number
  y: number
  radius: number
  alpha: number
  phase: number
  speed: number
}

interface CloudPoint {
  x: number
  y: number
  scale: number
  speed: number
  alpha: number
}

const MID_SCENE_MIN_WIDTH = 700
const MID_SCENE_PEAK_WIDTH = 720
const MID_SCENE_END_WIDTH = 900
const MID_SCENE_START_OFFSET = 17
const MID_SCENE_PEAK_OFFSET = 44

const DEFAULT_PROVIDERS: HarborProvider[] = [
  { name: 'OpenAI', platform: 'openai', color: '#10a37f' },
  { name: 'Anthropic', platform: 'anthropic', color: '#d97757' },
  { name: 'Gemini', platform: 'gemini', color: '#4285f4' },
  { name: 'DeepSeek', platform: 'deepseek', color: '#4d6bfe' },
  { name: 'Qwen', platform: 'qwen', color: '#7147d8' },
  { name: '智谱AI', platform: 'glm', color: '#0899b8' },
  { name: 'Kimi', platform: 'kimi', color: '#0f8b8d', darkIcon: true },
  { name: 'ByteDance', platform: 'doubao', color: '#168cff' },
  { name: 'MiniMax', platform: 'minimax', color: '#e84a68' },
  { name: 'MiMo', platform: 'mimo', color: '#ff6900' },
  { name: 'Grok', platform: 'grok', color: '#5d625f', darkIcon: true },
]

const STARS: StarPoint[] = [
  { x: 0.52, y: 0.1, radius: 0.8, alpha: 0.55, phase: 0.2, speed: 0.0011 },
  { x: 0.58, y: 0.18, radius: 1.2, alpha: 0.72, phase: 1.8, speed: 0.0008 },
  { x: 0.63, y: 0.08, radius: 0.7, alpha: 0.48, phase: 3.1, speed: 0.0014 },
  { x: 0.67, y: 0.28, radius: 1.1, alpha: 0.64, phase: 4.5, speed: 0.0009 },
  { x: 0.75, y: 0.07, radius: 0.8, alpha: 0.6, phase: 2.4, speed: 0.0012 },
  { x: 0.79, y: 0.27, radius: 0.6, alpha: 0.5, phase: 5.2, speed: 0.0015 },
  { x: 0.83, y: 0.13, radius: 1.3, alpha: 0.75, phase: 0.9, speed: 0.0007 },
  { x: 0.88, y: 0.22, radius: 0.9, alpha: 0.62, phase: 3.8, speed: 0.001 },
  { x: 0.94, y: 0.09, radius: 0.7, alpha: 0.52, phase: 5.8, speed: 0.0013 },
  { x: 0.97, y: 0.31, radius: 1.1, alpha: 0.68, phase: 2.9, speed: 0.0009 },
  { x: 0.55, y: 0.36, radius: 0.6, alpha: 0.48, phase: 4.1, speed: 0.0012 },
  { x: 0.61, y: 0.43, radius: 0.9, alpha: 0.58, phase: 1.1, speed: 0.0008 },
  { x: 0.7, y: 0.38, radius: 0.7, alpha: 0.46, phase: 2.2, speed: 0.0014 },
  { x: 0.77, y: 0.46, radius: 1, alpha: 0.65, phase: 5, speed: 0.001 },
  { x: 0.86, y: 0.4, radius: 0.7, alpha: 0.5, phase: 3.3, speed: 0.0013 },
  { x: 0.92, y: 0.47, radius: 0.9, alpha: 0.61, phase: 0.4, speed: 0.0009 },
  { x: 0.49, y: 0.49, radius: 0.7, alpha: 0.42, phase: 5.5, speed: 0.0011 },
  { x: 0.57, y: 0.54, radius: 0.8, alpha: 0.54, phase: 2.7, speed: 0.0008 },
  { x: 0.66, y: 0.52, radius: 0.6, alpha: 0.46, phase: 4.8, speed: 0.0015 },
  { x: 0.74, y: 0.55, radius: 0.9, alpha: 0.59, phase: 1.6, speed: 0.001 },
  { x: 0.82, y: 0.51, radius: 0.7, alpha: 0.48, phase: 3.6, speed: 0.0012 },
  { x: 0.9, y: 0.56, radius: 1, alpha: 0.66, phase: 0.7, speed: 0.0009 },
]

const CLOUDS: CloudPoint[] = [
  { x: 0.06, y: 0.11, scale: 0.75, speed: 0.003, alpha: 0.3 },
  { x: 0.36, y: 0.31, scale: 1.05, speed: 0.0021, alpha: 0.24 },
  { x: 0.72, y: 0.22, scale: 0.88, speed: 0.0026, alpha: 0.28 },
  { x: 0.92, y: 0.43, scale: 0.62, speed: 0.0034, alpha: 0.2 },
]

const props = withDefaults(defineProps<Props>(), {
  providers: () => [],
})

const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const ready = ref(false)
const failed = ref(false)
const reducedMotion = ref(false)
const sceneScale = ref(1)
const terminalOffset = ref(0)
const moonPhase = ref<MoonPhase>(getMoonPhase())
const resolvedProviders = computed(() => props.providers.length ? props.providers : DEFAULT_PROVIDERS)
const fallbackMoonShadowOffset = computed(() => {
  const cycle = moonPhase.value.cycle
  const offset = cycle <= 0.5 ? -400 * cycle : 400 * (1 - cycle)
  return `${offset}%`
})

let context: CanvasRenderingContext2D | null = null
let width = 1
let height = 1
let pixelRatio = 1
let animationFrame = 0
let resizeObserver: ResizeObserver | null = null
let visibilityObserver: IntersectionObserver | null = null
let motionQuery: MediaQueryList | null = null
let isVisible = true
let startedAt = 0
let moonPhaseTimer = 0

const LARGE_DESKTOP_MIN_WIDTH = 1800
const LARGE_DESKTOP_SCENE_SCALE = 1.16

const pointer = {
  currentX: 0,
  currentY: 0,
  targetX: 0,
  targetY: 0,
}

function palette(): Palette {
  return props.dark
    ? {
        sky: '#06161b',
        skyBand: '#0a252b',
        haze: '#16383e',
        cloud: '#d9e6e2',
        star: '#dcebe6',
        city: '#17363c',
        citySoft: '#23484e',
        water: '#0b438c',
        waterDeep: '#061f4f',
        waterLine: '#62adff',
        ink: '#061216',
        structure: '#15292d',
        structureSoft: '#668084',
        dock: '#263e40',
        dockTop: '#738987',
        dockEdge: '#aec2bc',
        cream: '#e8e9dc',
        coral: '#f1785c',
        mint: '#45c2b5',
        yellow: '#f2c96b',
      }
    : {
        sky: '#e4eeea',
        skyBand: '#d4e4df',
        haze: '#bdd5ce',
        cloud: '#ffffff',
        star: '#ffffff',
        city: '#829d98',
        citySoft: '#9eb1ac',
        water: '#2c86df',
        waterDeep: '#0b55ae',
        waterLine: '#b9dcff',
        ink: '#14292c',
        structure: '#263b3d',
        structureSoft: '#647a77',
        dock: '#71837e',
        dockTop: '#aebdb6',
        dockEdge: '#d4dfd9',
        cream: '#f7f4e8',
        coral: '#e7664a',
        mint: '#0b8b82',
        yellow: '#eab84e',
      }
}

function polygon(points: Array<[number, number]>, fill: string | CanvasGradient) {
  if (!context || !points.length) return
  context.beginPath()
  context.moveTo(points[0][0], points[0][1])
  for (let index = 1; index < points.length; index += 1) {
    context.lineTo(points[index][0], points[index][1])
  }
  context.closePath()
  context.fillStyle = fill
  context.fill()
}

function drawCuboid(
  x: number,
  y: number,
  cuboidWidth: number,
  cuboidHeight: number,
  depth: number,
  fill: string,
  alpha = 1,
  ribs = true
) {
  if (!context) return
  const rise = Math.max(2, depth * 0.55)
  const topFace: Array<[number, number]> = [
    [x, y],
    [x + depth, y - rise],
    [x + cuboidWidth + depth, y - rise],
    [x + cuboidWidth, y],
  ]
  const sideFace: Array<[number, number]> = [
    [x + cuboidWidth, y],
    [x + cuboidWidth + depth, y - rise],
    [x + cuboidWidth + depth, y + cuboidHeight - rise],
    [x + cuboidWidth, y + cuboidHeight],
  ]

  context.save()
  context.globalAlpha = alpha
  context.fillStyle = fill
  context.fillRect(x, y, cuboidWidth, cuboidHeight)
  polygon(topFace, fill)
  polygon(sideFace, fill)

  context.globalAlpha = alpha * 0.27
  polygon(topFace, '#ffffff')
  context.globalAlpha = alpha * 0.2
  polygon(sideFace, '#061a36')

  if (ribs && cuboidWidth >= 14) {
    const ribCount = Math.max(1, Math.floor(cuboidWidth / 12))
    for (let rib = 1; rib <= ribCount; rib += 1) {
      const ribX = x + (cuboidWidth * rib) / (ribCount + 1)
      strokeLine([[ribX, y + 2], [ribX, y + cuboidHeight - 2]], '#ffffff', 0.7, alpha * 0.2)
    }
  }
  context.restore()
}

function strokeLine(
  points: Array<[number, number]>,
  color: string,
  lineWidth = 1,
  alpha = 1
) {
  if (!context || points.length < 2) return
  context.beginPath()
  context.moveTo(points[0][0], points[0][1])
  for (let index = 1; index < points.length; index += 1) {
    context.lineTo(points[index][0], points[index][1])
  }
  context.strokeStyle = color
  context.lineWidth = lineWidth
  context.globalAlpha = alpha
  context.stroke()
  context.globalAlpha = 1
}

function drawSky(colors: Palette, horizonY: number) {
  if (!context) return colors.sky
  const skyGradient = context.createLinearGradient(0, 0, 0, horizonY)
  skyGradient.addColorStop(0, colors.sky)
  skyGradient.addColorStop(0.58, colors.skyBand)
  skyGradient.addColorStop(1, colors.haze)
  context.fillStyle = skyGradient
  context.fillRect(0, 0, width, horizonY + 2)
  return skyGradient
}

function celestialPosition(parallaxX: number, parallaxY: number) {
  return {
    x: width * 0.72 + parallaxX * 0.16,
    y: height * 0.16 + parallaxY * 0.12,
    radius: Math.max(16, Math.min(29, Math.min(width, height) * 0.037)),
  }
}

function drawStars(colors: Palette, horizonY: number, time: number) {
  if (!context || !props.dark) return
  const drawing = context
  const moon = celestialPosition(0, 0)
  STARS.forEach((star) => {
    const x = width * star.x
    const y = horizonY * star.y
    if (Math.hypot(x - moon.x, y - moon.y) < moon.radius + 34) return
    const twinkle = reducedMotion.value
      ? 0.82
      : 0.72 + Math.sin(time * star.speed + star.phase) * 0.28
    drawing.beginPath()
    drawing.arc(x, y, star.radius, 0, Math.PI * 2)
    drawing.fillStyle = colors.star
    drawing.globalAlpha = star.alpha * twinkle
    drawing.fill()
  })
  drawing.globalAlpha = 1
}

function drawCloudShape(colors: Palette, x: number, y: number, scale: number, alpha: number) {
  if (!context) return
  context.save()
  context.translate(x, y)
  context.scale(scale, scale)
  context.fillStyle = colors.cloud
  context.globalAlpha = alpha
  context.beginPath()
  context.arc(-24, 4, 13, 0, Math.PI * 2)
  context.arc(-7, -3, 18, 0, Math.PI * 2)
  context.arc(15, 2, 14, 0, Math.PI * 2)
  context.arc(30, 7, 9, 0, Math.PI * 2)
  context.fill()
  context.fillRect(-35, 4, 72, 14)
  context.restore()
}

function drawClouds(colors: Palette, horizonY: number, time: number) {
  if (!context || props.dark) return
  const cloudZoneStart = width * (width < 700 ? 0.5 : 0.48)
  const cloudZoneWidth = width - cloudZoneStart
  CLOUDS.forEach((cloud) => {
    const cloudWidth = 84 * cloud.scale
    const travel = cloudZoneWidth + cloudWidth * 2
    const offset = reducedMotion.value ? cloud.x * travel : (cloud.x * travel + time * cloud.speed) % travel
    const x = cloudZoneStart - cloudWidth + offset
    const y = horizonY * cloud.y
    drawCloudShape(colors, x, y, cloud.scale, cloud.alpha)
  })
}

function drawSun(colors: Palette, parallaxX: number, parallaxY: number) {
  if (!context) return
  const { x, y, radius } = celestialPosition(parallaxX, parallaxY)

  context.fillStyle = colors.yellow
  context.globalAlpha = 0.055
  context.beginPath()
  context.arc(x, y, radius + 18, 0, Math.PI * 2)
  context.fill()
  context.globalAlpha = 0.1
  context.beginPath()
  context.arc(x, y, radius + 10, 0, Math.PI * 2)
  context.fill()

  context.strokeStyle = colors.yellow
  context.lineWidth = 1.6
  context.globalAlpha = 0.48
  for (let ray = 0; ray < 12; ray += 1) {
    const angle = ray * Math.PI / 6
    const inner = radius + 8
    const outer = inner + (ray % 2 === 0 ? 14 : 9)
    context.beginPath()
    context.moveTo(x + Math.cos(angle) * inner, y + Math.sin(angle) * inner)
    context.lineTo(x + Math.cos(angle) * outer, y + Math.sin(angle) * outer)
    context.stroke()
  }

  context.beginPath()
  context.arc(x, y, radius, 0, Math.PI * 2)
  context.fillStyle = colors.yellow
  context.globalAlpha = 0.96
  context.fill()
  context.globalAlpha = 1
}

function drawMoon(
  parallaxX: number,
  parallaxY: number,
  skyFill: string | CanvasGradient
) {
  if (!context) return
  const drawing = context
  const { x, y, radius } = celestialPosition(parallaxX, parallaxY)
  const cycle = moonPhase.value.cycle
  const shadowOffset = cycle <= 0.5
    ? -radius * 4 * cycle
    : radius * 4 * (1 - cycle)

  drawing.save()
  drawing.beginPath()
  drawing.arc(x, y, radius, 0, Math.PI * 2)
  drawing.clip()
  drawing.fillStyle = '#d9e3df'
  drawing.fillRect(x - radius, y - radius, radius * 2, radius * 2)

  const craters = [
    [-0.34, -0.22, 0.15],
    [0.23, -0.35, 0.1],
    [0.31, 0.24, 0.17],
    [-0.19, 0.34, 0.09],
  ] as const
  drawing.fillStyle = '#afc0bc'
  drawing.globalAlpha = 0.5
  craters.forEach(([offsetX, offsetY, craterRadius]) => {
    drawing.beginPath()
    drawing.arc(
      x + radius * offsetX,
      y + radius * offsetY,
      radius * craterRadius,
      0,
      Math.PI * 2
    )
    drawing.fill()
  })

  drawing.globalAlpha = 1
  drawing.beginPath()
  drawing.arc(x + shadowOffset, y, radius, 0, Math.PI * 2)
  drawing.fillStyle = skyFill
  drawing.fill()
  drawing.restore()
  drawing.globalAlpha = 1
}

function drawCelestialBody(
  colors: Palette,
  parallaxX: number,
  parallaxY: number,
  skyFill: string | CanvasGradient
) {
  if (props.dark) {
    drawMoon(parallaxX, parallaxY, skyFill)
    return
  }
  drawSun(colors, parallaxX, parallaxY)
}

function drawSkyline(colors: Palette, horizonY: number, parallaxX: number) {
  if (!context) return
  const buildingWidths = [24, 14, 30, 18, 44, 24, 56, 25, 48, 20, 52, 34]
  const safeStart = width < 700 ? -12 : width < 1000 ? width * 0.58 : width * 0.42
  const skylineEnd = width * 0.7
  let left = safeStart
  let index = 0

  while (left < skylineEnd) {
    const buildingWidth = buildingWidths[index % buildingWidths.length]
    const buildingHeight = 24 + ((index * 19) % 39)
    const x = left + parallaxX * (0.12 + index * 0.008)
    context.fillStyle = index % 3 === 0 ? colors.citySoft : colors.city
    context.globalAlpha = props.dark ? 0.8 : 0.86
    context.fillRect(x, horizonY - buildingHeight, buildingWidth, buildingHeight)
    if (index % 4 === 1) {
      context.fillRect(x + buildingWidth * 0.48, horizonY - buildingHeight - 15, 2, 15)
    }
    left += buildingWidth + 8
    index += 1
  }
  context.globalAlpha = 1
  context.fillStyle = colors.city
  context.fillRect(0, horizonY - 4, width, 5)
}

function drawWater(colors: Palette, horizonY: number) {
  if (!context) return
  const waterGradient = context.createLinearGradient(0, horizonY, 0, height)
  waterGradient.addColorStop(0, colors.water)
  waterGradient.addColorStop(1, colors.waterDeep)
  context.fillStyle = waterGradient
  context.fillRect(0, horizonY, width, height - horizonY)
  context.fillStyle = colors.waterLine
  context.globalAlpha = props.dark ? 0.18 : 0.28
  context.fillRect(0, horizonY, width, 2)
  context.globalAlpha = 1
}

function drawWaves(colors: Palette, horizonY: number, time: number) {
  if (!context) return
  const waveOffset = reducedMotion.value ? 0 : (time * 0.012) % 54
  context.strokeStyle = colors.waterLine
  context.globalAlpha = props.dark ? 0.13 : 0.2
  context.lineWidth = 1
  for (let row = 0; row < 8; row += 1) {
    const y = horizonY + 28 + row * Math.max(31, height * 0.052)
    context.beginPath()
    for (let x = -64 + waveOffset; x < width + 64; x += 54) {
      context.moveTo(x, y)
      context.quadraticCurveTo(x + 13.5, y - 3, x + 27, y)
      context.quadraticCurveTo(x + 40.5, y + 3, x + 54, y)
    }
    context.stroke()
  }
  context.globalAlpha = 1
}

function drawWaterReflections(colors: Palette, horizonY: number, time: number) {
  if (!context) return
  const drawing = context
  const drift = reducedMotion.value ? 0 : Math.sin(time * 0.00045) * 4
  const reflections = [
    [0.52, colors.citySoft, 46],
    [0.61, colors.yellow, 24],
    [0.73, colors.mint, 34],
    [0.83, colors.coral, 18],
  ] as const

  reflections.forEach(([xRatio, color, reflectionHeight], index) => {
    const x = width * xRatio + drift * (index % 2 ? 1 : -1)
    drawing.fillStyle = color
    drawing.globalAlpha = props.dark ? 0.08 : 0.09
    for (let stripe = 0; stripe < 4; stripe += 1) {
      const stripeWidth = Math.max(8, 30 - stripe * 5)
      drawing.fillRect(
        x - stripeWidth / 2 + (stripe % 2 ? 4 : -3),
        horizonY + 13 + stripe * (reflectionHeight / 4),
        stripeWidth,
        2
      )
    }
  })
  drawing.globalAlpha = 1
}

function drawRoutes(colors: Palette, horizonY: number, time: number) {
  if (!context) return
  const routes = [
    {
      start: [width * 0.03, height * 0.82],
      control: [width * 0.3, height * 0.66],
      end: [width * 0.55, horizonY + height * 0.09],
      color: colors.mint,
    },
    {
      start: [width * 0.2, height * 0.94],
      control: [width * 0.42, height * 0.72],
      end: [width * 0.62, horizonY + height * 0.08],
      color: colors.coral,
    },
    {
      start: [width * 0.01, height * 0.7],
      control: [width * 0.32, height * 0.62],
      end: [width * 0.72, horizonY + height * 0.06],
      color: colors.yellow,
    },
  ]

  routes.forEach((route, routeIndex) => {
    context?.beginPath()
    context?.moveTo(route.start[0], route.start[1])
    context?.quadraticCurveTo(route.control[0], route.control[1], route.end[0], route.end[1])
    if (!context) return
    context.strokeStyle = route.color
    context.globalAlpha = props.dark ? 0.24 : 0.31
    context.lineWidth = routeIndex === 0 ? 1.5 : 1
    context.setLineDash(routeIndex === 0 ? [7, 10] : [4, 11])
    context.stroke()
    context.setLineDash([])

    const pulseCount = reducedMotion.value ? 1 : 3
    for (let pulseIndex = 0; pulseIndex < pulseCount; pulseIndex += 1) {
      const phase = reducedMotion.value
        ? 0.56
        : (time * 0.00006 + pulseIndex / pulseCount + routeIndex * 0.17) % 1
      const inverse = 1 - phase
      const x = inverse * inverse * route.start[0]
        + 2 * inverse * phase * route.control[0]
        + phase * phase * route.end[0]
      const y = inverse * inverse * route.start[1]
        + 2 * inverse * phase * route.control[1]
        + phase * phase * route.end[1]
      context.beginPath()
      context.arc(x, y, routeIndex === 0 ? 2.8 : 2.1, 0, Math.PI * 2)
      context.fillStyle = route.color
      context.globalAlpha = 0.9
      context.fill()
    }
  })
  context.globalAlpha = 1
}

function getHarborLayout(horizonY: number): HarborLayout {
  const compact = width < MID_SCENE_MIN_WIDTH
  const terminalOffsetY = getTerminalOffset(width)
  return {
    compact,
    horizonY,
    terminalOffsetY,
    terminalLeft: width * (compact ? 0.47 : 0.54),
    terminalRight: width * (compact ? 0.76 : 0.82),
    deckY: horizonY
      + Math.min(compact ? 48 : 54, height * (compact ? 0.066 : 0.07))
      + terminalOffsetY,
    deckThickness: compact ? 16 : 20,
    dockFootX: width * (compact ? 0.8 : 0.84),
    lighthouseX: width * (compact ? 0.89 : 0.91),
    lighthouseBaseY: horizonY
      + Math.min(compact ? 156 : 176, height * (compact ? 0.205 : 0.22))
      + terminalOffsetY,
    lighthouseHeight: Math.max(
      compact ? 92 : 108,
      Math.min(compact ? 126 : 148, height * (compact ? 0.17 : 0.19))
    ),
  }
}

function getTerminalOffset(sceneWidth: number) {
  if (sceneWidth < MID_SCENE_MIN_WIDTH || sceneWidth >= MID_SCENE_END_WIDTH) return 0
  if (sceneWidth <= MID_SCENE_PEAK_WIDTH) {
    const progress = (sceneWidth - MID_SCENE_MIN_WIDTH)
      / (MID_SCENE_PEAK_WIDTH - MID_SCENE_MIN_WIDTH)
    return MID_SCENE_START_OFFSET
      + (MID_SCENE_PEAK_OFFSET - MID_SCENE_START_OFFSET) * progress
  }
  return MID_SCENE_PEAK_OFFSET
    * (MID_SCENE_END_WIDTH - sceneWidth)
    / (MID_SCENE_END_WIDTH - MID_SCENE_PEAK_WIDTH)
}

function lighthouseLanternY(layout: HarborLayout) {
  return layout.lighthouseBaseY - layout.lighthouseHeight - (layout.compact ? 14 : 16)
}

function drawLighthouseBeam(colors: Palette, layout: HarborLayout, time: number) {
  if (!context || !props.dark) return
  const originX = layout.lighthouseX
  const originY = lighthouseLanternY(layout)
  const sweep = reducedMotion.value ? -8 : Math.sin(time * 0.00042) * Math.min(34, width * 0.025)
  const beamLength = Math.max(250, width * 0.34)
  const endX = originX - beamLength

  context.globalAlpha = 0.13
  polygon([
    [originX - 4, originY - 4],
    [endX, originY - 45 + sweep],
    [endX, originY + 46 + sweep],
  ], colors.yellow)
  context.globalAlpha = 0.09
  polygon([
    [originX - 2, originY - 2],
    [endX, originY - 17 + sweep],
    [endX, originY + 18 + sweep],
  ], colors.cream)
  context.globalAlpha = 1
}

function drawTerminalAndDock(colors: Palette, layout: HarborLayout) {
  if (!context) return
  const {
    horizonY,
    terminalLeft,
    terminalRight,
    deckY,
    deckThickness,
    dockFootX,
  } = layout

  polygon([
    [terminalRight - 4, deckY - 18],
    [width, horizonY + 8],
    [width, height],
    [dockFootX, height],
    [terminalRight + 14, deckY + deckThickness + 4],
  ], colors.dock)

  polygon([
    [terminalRight - 4, deckY - 18],
    [width, horizonY + 8],
    [width, horizonY + height * 0.12],
    [terminalRight + 14, deckY + 2],
  ], colors.dockTop)

  const edgeStartX = terminalRight + 18
  polygon([
    [edgeStartX, deckY + 4],
    [width, horizonY + height * 0.115],
    [width, horizonY + height * 0.148],
    [edgeStartX, deckY + 18],
  ], colors.dockEdge)

  polygon([
    [terminalLeft, deckY - 8],
    [terminalRight - 2, deckY - 18],
    [terminalRight + 16, deckY - 3],
    [terminalLeft, deckY + 8],
  ], colors.dockTop)
  polygon([
    [terminalLeft, deckY + 8],
    [terminalRight + 16, deckY - 3],
    [terminalRight + 14, deckY + deckThickness],
    [terminalLeft, deckY + deckThickness + 7],
  ], colors.dock)

  context.fillStyle = colors.structure
  context.globalAlpha = 0.68
  const pileCount = layout.compact ? 3 : 5
  for (let pile = 0; pile < pileCount; pile += 1) {
    const x = terminalLeft + ((terminalRight - terminalLeft) * (pile + 0.7)) / pileCount
    context.fillRect(x, deckY + deckThickness - 2, 4, Math.min(54, height * 0.075))
  }
  context.globalAlpha = 1

  strokeLine([
    [terminalLeft, deckY - 9],
    [terminalRight - 2, deckY - 19],
  ], colors.cream, 1.5, 0.5)
}

function drawBreakwater(colors: Palette) {
  const compact = width < MID_SCENE_MIN_WIDTH
  const tipX = width * (compact ? 0.2 : 0.18)
  const startY = height * (compact ? 0.91 : 0.9)
  const tipY = height * (compact ? 0.855 : 0.845)
  const deckDepth = compact ? 13 : 17
  const wallDepth = compact ? 24 : 31
  const capWidth = compact ? 8 : 11

  polygon([
    [0, startY],
    [tipX, tipY],
    [tipX, tipY + deckDepth],
    [0, startY + deckDepth],
  ], colors.dockTop)
  polygon([
    [0, startY + deckDepth],
    [tipX, tipY + deckDepth],
    [tipX, tipY + deckDepth + wallDepth],
    [0, startY + deckDepth + wallDepth],
  ], colors.dock)
  polygon([
    [tipX - capWidth, tipY],
    [tipX, tipY],
    [tipX, tipY + deckDepth + wallDepth],
    [tipX - capWidth, tipY + deckDepth + wallDepth - 3],
  ], colors.dockEdge)

  context?.fillRect(tipX - capWidth / 2 - 1.5, tipY - 17, 3, 17)
  if (context) {
    context.fillStyle = colors.coral
    context.beginPath()
    context.arc(tipX - capWidth / 2, tipY - 19, 3.5, 0, Math.PI * 2)
    context.fill()
  }
}

function drawIntegratedGantry(colors: Palette, layout: HarborLayout, time: number) {
  if (!context) return
  const span = layout.terminalRight - layout.terminalLeft
  const beamStart = layout.terminalLeft - width * (layout.compact ? 0.08 : 0.1)
  const beamEnd = layout.terminalRight - span * 0.03
  const beamY = layout.horizonY
    - Math.min(
      layout.compact ? 68 : 102,
      height * (layout.compact ? 0.095 : 0.14)
    )
    + layout.terminalOffsetY
  const railY = layout.deckY - 8
  const leftFrame = layout.terminalLeft + span * 0.1
  const rightFrame = layout.terminalRight - span * 0.08

  drawCuboid(
    beamStart,
    beamY,
    beamEnd - beamStart,
    layout.compact ? 7 : 9,
    layout.compact ? 3 : 5,
    colors.structureSoft,
    0.94,
    false
  )
  strokeLine([[beamStart + 4, beamY + 10], [beamEnd, beamY + 10]], colors.structureSoft, 2.5, 0.86)

  const trussSegments = layout.compact ? 7 : 10
  for (let index = 0; index < trussSegments; index += 1) {
    const startX = beamStart + ((beamEnd - beamStart) * index) / trussSegments
    const endX = beamStart + ((beamEnd - beamStart) * (index + 1)) / trussSegments
    strokeLine([
      [startX, index % 2 === 0 ? beamY : beamY + 10],
      [endX, index % 2 === 0 ? beamY + 10 : beamY],
    ], colors.structureSoft, 1.25, 0.62)
  }

  [leftFrame, rightFrame].forEach((frameX, frameIndex) => {
    const flare = layout.compact ? 15 : 22
    strokeLine([[frameX, beamY + 9], [frameX - flare, railY]], colors.structureSoft, layout.compact ? 3.5 : 4)
    strokeLine([[frameX + 14, beamY + 9], [frameX + flare, railY]], colors.structureSoft, layout.compact ? 3.5 : 4)
    strokeLine([
      [frameX - flare * 0.72, railY - (railY - beamY) * 0.34],
      [frameX + flare * 0.72, railY - (railY - beamY) * 0.68],
    ], colors.structureSoft, 1.5, 0.75)
    context!.fillStyle = colors.structure
    context!.fillRect(frameX - flare - 4, railY - 3, 10, 5)
    context!.fillRect(frameX + flare - 5, railY - 3, 10, 5)
    if (frameIndex === 1) {
      polygon([
        [frameX - 21, beamY + 25],
        [frameX + 1, beamY + 25],
        [frameX - 3, beamY + 42],
        [frameX - 24, beamY + 42],
      ], colors.cream)
      context!.fillStyle = colors.ink
      context!.globalAlpha = 0.42
      context!.fillRect(frameX - 17, beamY + 29, 10, 7)
      context!.globalAlpha = 1
    }
  })

  strokeLine([[layout.terminalLeft, railY], [layout.terminalRight, railY - 9]], colors.structure, 2, 0.75)

  const trolleyPhase = reducedMotion.value ? 0.58 : (Math.sin(time * 0.00034) + 1) / 2
  const trolleyX = beamStart + (beamEnd - beamStart) * (0.22 + trolleyPhase * 0.62)
  const liftPhase = reducedMotion.value ? 0.5 : (Math.sin(time * 0.00052 + 1.4) + 1) / 2
  const hookY = Math.min(railY - 22, beamY + 34 + liftPhase * (railY - beamY - 70))
  context.fillStyle = colors.coral
  context.fillRect(trolleyX - 12, beamY - 5, 24, 9)
  strokeLine([[trolleyX - 5, beamY + 4], [trolleyX - 5, hookY]], colors.structureSoft, 1.5)
  strokeLine([[trolleyX + 5, beamY + 4], [trolleyX + 5, hookY]], colors.structureSoft, 1.5)

  const cargoColors = resolvedProviders.value.map((provider) => provider.color)
  const cargoColor = cargoColors.length
    ? cargoColors[Math.floor(time / 5500) % cargoColors.length]
    : colors.mint
  drawCuboid(trolleyX - 17, hookY, 34, 13, 5, cargoColor, 0.96)

  const nameplateX = leftFrame + (rightFrame - leftFrame) * 0.47
  context.fillStyle = colors.structure
  context.globalAlpha = 0.9
  context.fillRect(nameplateX - 29, beamY + 13, 58, layout.compact ? 14 : 17)
  context.fillStyle = colors.cream
  context.globalAlpha = 0.88
  context.font = `700 ${layout.compact ? 8 : 11}px "Noto Sans CJK SC", sans-serif`
  context.textAlign = 'center'
  context.textBaseline = 'middle'
  context.fillText('模型港', nameplateX, beamY + (layout.compact ? 20 : 21.5))
  context.globalAlpha = 1

  if (props.dark) {
    [beamStart + 10, beamEnd - 8].forEach((lightX, index) => {
      context!.beginPath()
      context!.arc(lightX, beamY + 4, 2.2, 0, Math.PI * 2)
      context!.fillStyle = index === 0 ? colors.coral : colors.mint
      context!.fill()
    })
  }
}

function drawContainers(colors: Palette, layout: HarborLayout) {
  if (!context) return
  const providerColors = resolvedProviders.value.map((provider) => provider.color)
  const containerColors = providerColors.length
    ? providerColors
    : [colors.coral, colors.mint, colors.yellow, colors.structureSoft]
  const span = layout.terminalRight - layout.terminalLeft
  const unitWidth = layout.compact ? 24 : Math.max(24, Math.min(31, width * 0.022))
  const unitHeight = layout.compact ? 13 : Math.max(11, Math.min(15, height * 0.018))
  const startX = layout.terminalLeft + span * (layout.compact ? 0.16 : 0.2)
  const baseY = layout.deckY - 10
  const rows = layout.compact ? 3 : 2
  const count = layout.compact ? 4 : 6

  for (let row = 0; row < rows; row += 1) {
    for (let column = 0; column < Math.max(2, count - Math.floor(row / 2)); column += 1) {
      const x = startX + column * (unitWidth + 3)
      const y = baseY - unitHeight - row * (unitHeight + 3)
      drawCuboid(
        x,
        y,
        unitWidth,
        unitHeight,
        layout.compact ? 3 : 4,
        containerColors[(column + row * 2) % containerColors.length],
        0.62
      )
    }
  }
  context.globalAlpha = 1
}

function drawLighthouse(colors: Palette, layout: HarborLayout) {
  if (!context) return
  const x = layout.lighthouseX
  const baseY = layout.lighthouseBaseY
  const towerHeight = layout.lighthouseHeight
  const topY = baseY - towerHeight
  const baseHalf = Math.max(22, Math.min(layout.compact ? 25 : 31, width * 0.021))
  const topHalf = Math.max(9, baseHalf * 0.38)

  polygon([
    [x - baseHalf - 12, baseY + 8],
    [x + baseHalf + 12, baseY + 8],
    [x + baseHalf + 18, baseY + 17],
    [x - baseHalf - 18, baseY + 17],
  ], colors.structure)

  polygon([
    [x - baseHalf, baseY],
    [x + baseHalf, baseY],
    [x + topHalf, topY],
    [x - topHalf, topY],
  ], colors.cream)
  polygon([
    [x, baseY],
    [x + baseHalf, baseY],
    [x + topHalf, topY],
    [x, topY],
  ], props.dark ? '#c8d1c9' : '#dedfd4')

  const halfAt = (ratio: number) => topHalf + (baseHalf - topHalf) * ratio
  ;[0.38, 0.66].forEach((ratio) => {
    const stripeHeight = layout.compact ? 11 : 13
    const y = topY + towerHeight * ratio
    const topWidth = halfAt(ratio)
    const bottomWidth = halfAt(Math.min(1, ratio + stripeHeight / towerHeight))
    polygon([
      [x - topWidth, y],
      [x + topWidth, y],
      [x + bottomWidth, y + stripeHeight],
      [x - bottomWidth, y + stripeHeight],
    ], colors.coral)
  })

  context.fillStyle = colors.structure
  context.fillRect(x - 6, baseY - 20, 12, 20)
  context.fillStyle = colors.waterLine
  context.globalAlpha = 0.56
  context.fillRect(x - 3, topY + towerHeight * 0.25, 6, 8)
  context.fillRect(x - 4, topY + towerHeight * 0.52, 8, 6)
  context.globalAlpha = 1

  const balconyY = topY - 4
  context.fillStyle = colors.structure
  context.fillRect(x - topHalf - 10, balconyY, (topHalf + 10) * 2, 5)
  strokeLine([[x - topHalf - 8, balconyY - 10], [x + topHalf + 8, balconyY - 10]], colors.structureSoft, 1.5)
  ;[-1, -0.33, 0.33, 1].forEach((offset) => {
    const railX = x + offset * (topHalf + 7)
    strokeLine([[railX, balconyY - 10], [railX, balconyY]], colors.structureSoft, 1)
  })

  const lanternTop = topY - (layout.compact ? 22 : 25)
  context.fillStyle = props.dark ? colors.yellow : colors.dockEdge
  context.globalAlpha = props.dark ? 0.95 : 0.72
  context.fillRect(x - 9, lanternTop, 18, 17)
  context.globalAlpha = 1
  context.strokeStyle = colors.structure
  context.lineWidth = 2
  ;[-9, 0, 9].forEach((offset) => {
    strokeLine([[x + offset, lanternTop], [x + offset, lanternTop + 17]], colors.structure, 1.4)
  })
  context.fillStyle = colors.structure
  context.fillRect(x - 12, lanternTop - 3, 24, 4)
  polygon([
    [x - 15, lanternTop - 3],
    [x, lanternTop - 16],
    [x + 15, lanternTop - 3],
  ], colors.structure)
  context.fillRect(x - 1, lanternTop - 22, 2, 7)
}

function drawShipWake(
  colors: Palette,
  x: number,
  y: number,
  scale: number,
  direction: 1 | -1,
  time: number
) {
  if (!context) return
  const wakeLength = 104 * scale
  const wave = reducedMotion.value ? 0 : Math.sin(time * 0.0016) * 2
  context.strokeStyle = colors.waterLine
  context.lineWidth = 1.4
  context.globalAlpha = props.dark ? 0.24 : 0.38
  for (let index = 0; index < 3; index += 1) {
    const startX = x - direction * (66 * scale + index * 10)
    const endX = x - direction * (wakeLength + index * 18)
    const wakeY = y + 27 * scale + index * 6 + wave
    context.beginPath()
    context.moveTo(startX, wakeY)
    context.quadraticCurveTo(
      (startX + endX) / 2,
      wakeY + (index % 2 ? 3 : -3),
      endX,
      wakeY + index * 2
    )
    context.stroke()
  }
  context.globalAlpha = 1
}

function drawShip(
  colors: Palette,
  x: number,
  y: number,
  scale: number,
  time: number,
  direction: 1 | -1,
  cargoOffset: number
) {
  if (!context) return
  drawShipWake(colors, x, y, scale, direction, time)
  context.save()
  context.translate(x, y + (reducedMotion.value ? 0 : Math.sin(time * 0.0012) * 2))
  context.scale(scale * direction, scale)
  const hullGradient = context.createLinearGradient(0, 0, 0, 24)
  hullGradient.addColorStop(0, colors.structureSoft)
  hullGradient.addColorStop(0.22, colors.structure)
  hullGradient.addColorStop(1, colors.ink)
  polygon([[-74, 0], [82, 0], [59, 23], [-57, 23]], hullGradient)
  polygon([[-74, 0], [82, 0], [72, 5], [-67, 5]], colors.structureSoft)
  strokeLine([[-67, 7], [70, 7]], colors.cream, 1, props.dark ? 0.3 : 0.22)

  drawCuboid(-54, -18, 34, 18, 5, colors.cream, 1, false)
  drawCuboid(-47, -29, 20, 11, 4, colors.structureSoft, 1, false)
  context.fillStyle = colors.ink
  context.fillRect(-43, -25, 5, 4)
  context.fillRect(-34, -25, 5, 4)
  strokeLine([[-37, -33], [-37, -43]], colors.structure, 1.5)
  strokeLine([[-37, -41], [-27, -37]], colors.structure, 1)

  const cargoColors = resolvedProviders.value.map((provider) => provider.color)
  const colorsToUse = cargoColors.length ? cargoColors : [colors.coral, colors.mint, colors.yellow]
  for (let row = 0; row < 3; row += 1) {
    for (let column = 0; column < 5 - row; column += 1) {
      drawCuboid(
        -9 + column * 18,
        -12 - row * 13,
        15,
        10,
        3,
        colorsToUse[(column + row + cargoOffset) % colorsToUse.length]
      )
    }
  }
  context.fillStyle = colors.waterLine
  context.globalAlpha = 0.5
  context.fillRect(-90, 28, 136, 2)
  context.restore()
  context.globalAlpha = 1
}

function interpolate(start: number, end: number, progress: number) {
  return start + (end - start) * progress
}

function drawShips(colors: Palette, layout: HarborLayout, time: number) {
  const elapsed = time - startedAt
  const inboundScale = Math.max(
    layout.compact ? 0.48 : 0.6,
    Math.min(layout.compact ? 0.66 : 1, width / 1320)
  )
  const outboundScale = Math.max(
    layout.compact ? 0.34 : 0.42,
    Math.min(layout.compact ? 0.5 : 0.66, width / 1900)
  )
  const inboundTravel = reducedMotion.value
    ? 0.46
    : (elapsed * 0.000014 + (layout.compact ? 0.2 : 0.28)) % 1
  const inboundX = interpolate(
    74 * inboundScale,
    layout.terminalLeft - 102 * inboundScale,
    inboundTravel
  ) + pointer.currentX * Math.min(5, width * 0.004)
  const inboundY = layout.horizonY
    + height * (layout.compact ? 0.205 : 0.19)
    - inboundTravel * height * 0.022
  drawShip(colors, inboundX, inboundY, inboundScale, time, 1, 0)

  const outboundTravel = reducedMotion.value
    ? 0.54
    : (elapsed * 0.0000105 + (layout.compact ? 0.58 : 0.64)) % 1
  const outboundX = interpolate(
    layout.terminalLeft - 102 * outboundScale,
    -74 * outboundScale,
    outboundTravel
  ) - pointer.currentX * Math.min(3, width * 0.0025)
  const outboundY = layout.horizonY
    + height * (layout.compact ? 0.095 : 0.105)
    + outboundTravel * height * 0.016
  drawShip(colors, outboundX, outboundY, outboundScale, time + 900, -1, 5)
}

function drawBuoys(colors: Palette, horizonY: number, time: number) {
  if (!context) return
  const buoys = [
    [0.28, 0.72, colors.coral],
    [0.43, 0.66, colors.yellow],
    [0.53, 0.61, colors.mint],
  ] as const
  buoys.forEach(([xRatio, yRatio, color], index) => {
    const x = width * xRatio
    const y = Math.max(horizonY + 24, height * yRatio)
    context!.fillStyle = colors.cream
    context!.fillRect(x - 1, y - 11, 2, 11)
    context!.beginPath()
    context!.arc(x, y, 4, 0, Math.PI * 2)
    context!.fillStyle = color
    context!.fill()
    context!.beginPath()
    context!.arc(x, y, 7 + Math.sin(time * 0.002 + index) * 1.5, 0, Math.PI * 2)
    context!.strokeStyle = color
    context!.globalAlpha = 0.22
    context!.stroke()
    context!.globalAlpha = 1
  })
}

function render(time = performance.now()) {
  if (!context || width <= 1 || height <= 1) return
  pointer.currentX += (pointer.targetX - pointer.currentX) * 0.055
  pointer.currentY += (pointer.targetY - pointer.currentY) * 0.055

  const colors = palette()
  const compact = width < MID_SCENE_MIN_WIDTH
  const horizonY = height * (compact ? 0.61 : 0.63)
  const layout = getHarborLayout(horizonY)
  const parallaxX = pointer.currentX * Math.min(16, width * 0.014)
  const parallaxY = pointer.currentY * Math.min(8, height * 0.01)

  const renderScale = pixelRatio * sceneScale.value
  context.setTransform(renderScale, 0, 0, renderScale, 0, 0)
  context.clearRect(0, 0, width, height)
  const skyFill = drawSky(colors, horizonY)
  drawStars(colors, horizonY, time)
  drawClouds(colors, horizonY, time)
  drawCelestialBody(colors, parallaxX, parallaxY, skyFill)
  drawSkyline(colors, horizonY, parallaxX)
  drawWater(colors, horizonY)
  drawWaterReflections(colors, horizonY, time)
  drawWaves(colors, horizonY, time)
  drawRoutes(colors, horizonY, time)
  drawLighthouseBeam(colors, layout, time)
  drawTerminalAndDock(colors, layout)
  drawBreakwater(colors)
  drawShips(colors, layout, time)
  drawIntegratedGantry(colors, layout, time)
  drawContainers(colors, layout)
  drawBuoys(colors, horizonY, time)
  drawLighthouse(colors, layout)

  ready.value = true
}

function loop(time: number) {
  render(time)
  if (!reducedMotion.value && isVisible && !document.hidden) {
    animationFrame = window.requestAnimationFrame(loop)
  }
}

function restartAnimation() {
  window.cancelAnimationFrame(animationFrame)
  render()
  if (!reducedMotion.value && isVisible && !document.hidden) {
    animationFrame = window.requestAnimationFrame(loop)
  }
}

function resize() {
  const element = canvas.value
  const container = host.value
  if (!element || !container || !context) return
  const bounds = container.getBoundingClientRect()
  const cssWidth = Math.max(1, Math.round(bounds.width))
  const cssHeight = Math.max(1, Math.round(bounds.height))
  sceneScale.value = cssWidth >= LARGE_DESKTOP_MIN_WIDTH
    ? LARGE_DESKTOP_SCENE_SCALE
    : 1
  width = cssWidth / sceneScale.value
  height = cssHeight / sceneScale.value
  terminalOffset.value = getTerminalOffset(width)
  pixelRatio = Math.min(2, window.devicePixelRatio || 1)
  element.width = Math.round(cssWidth * pixelRatio)
  element.height = Math.round(cssHeight * pixelRatio)
  element.style.width = `${cssWidth}px`
  element.style.height = `${cssHeight}px`
  render()
}

function handlePointerMove(event: PointerEvent) {
  const bounds = host.value?.getBoundingClientRect()
  if (!bounds) return
  pointer.targetX = ((event.clientX - bounds.left) / Math.max(1, bounds.width) - 0.5) * 2
  pointer.targetY = ((event.clientY - bounds.top) / Math.max(1, bounds.height) - 0.5) * 2
  if (reducedMotion.value) render()
}

function resetPointer() {
  pointer.targetX = 0
  pointer.targetY = 0
  if (reducedMotion.value) render()
}

function handleMotionChange(event: MediaQueryListEvent) {
  reducedMotion.value = event.matches
  restartAnimation()
}

function handleVisibilityChange() {
  moonPhase.value = getMoonPhase()
  restartAnimation()
}

function refreshMoonPhase() {
  moonPhase.value = getMoonPhase()
  if (props.dark) restartAnimation()
}

watch(
  () => [props.dark, props.providers] as const,
  () => nextTick(() => render()),
  { deep: true }
)

onMounted(() => {
  const element = canvas.value
  context = element?.getContext('2d') ?? null
  if (!context) {
    failed.value = true
    ready.value = true
    return
  }

  startedAt = performance.now()
  moonPhase.value = getMoonPhase()
  moonPhaseTimer = window.setInterval(refreshMoonPhase, 60 * 60 * 1000)
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion.value = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionChange)

  if (typeof ResizeObserver === 'function') {
    resizeObserver = new ResizeObserver(resize)
    if (host.value) resizeObserver.observe(host.value)
  }
  window.addEventListener('resize', resize)

  if ('IntersectionObserver' in window) {
    visibilityObserver = new IntersectionObserver(([entry]) => {
      isVisible = entry?.isIntersecting ?? true
      restartAnimation()
    })
    if (host.value) visibilityObserver.observe(host.value)
  }

  document.addEventListener('visibilitychange', handleVisibilityChange)
  resize()
  restartAnimation()
})

onBeforeUnmount(() => {
  window.cancelAnimationFrame(animationFrame)
  window.clearInterval(moonPhaseTimer)
  resizeObserver?.disconnect()
  visibilityObserver?.disconnect()
  motionQuery?.removeEventListener('change', handleMotionChange)
  window.removeEventListener('resize', resize)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.graphic-harbor {
  position: absolute;
  inset: 0;
  overflow: hidden;
  contain: strict;
  touch-action: pan-y;
}

canvas {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
}

.dock-model-badges {
  position: absolute;
  z-index: 2;
  top: calc(63% + 8px + var(--terminal-offset, 0px));
  right: 20%;
  display: grid;
  width: 204px;
  grid-template-columns: repeat(6, 30px);
  gap: 4px;
  pointer-events: none;
}

.dock-model-badge {
  --provider-color: #0d6efd;
  --badge-index: 0;
  display: inline-flex;
  width: 30px;
  height: 18px;
  border: 1px solid color-mix(in srgb, var(--provider-color) 72%, #dfe9e4);
  align-items: center;
  justify-content: center;
  color: #fff;
  background: var(--provider-color);
  box-shadow: inset 0 -3px 0 rgba(5, 24, 28, 0.16);
  animation: cargo-signal 8s ease-in-out infinite;
  animation-delay: calc(var(--badge-index) * -0.42s);
}

.is-dark .dock-model-badge {
  border-color: color-mix(in srgb, var(--provider-color) 72%, #07181d);
}

.is-dark .needs-dark-icon {
  color: #f4f7f2;
}

.is-dark .needs-dark-icon :deep(.model-icon path) {
  fill: #f4f7f2;
}

.is-dark .needs-dark-icon :deep(svg path) {
  fill: #f4f7f2;
}

.fallback-harbor {
  position: absolute;
  inset: 0;
  display: none;
  overflow: hidden;
  background: linear-gradient(to bottom, #e4eeea 0%, #d4e4df 58%, #bdd5ce 100%);
}

.is-dark .fallback-harbor {
  background: linear-gradient(to bottom, #06161b 0%, #0a252b 58%, #16383e 100%);
}

.is-fallback .fallback-harbor {
  display: block;
}

.is-fallback canvas {
  display: none;
}

.fallback-atmosphere {
  position: absolute;
  top: 18%;
  right: 12%;
  width: 76px;
  height: 18px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.34);
  box-shadow: -24px 4px 0 rgba(255, 255, 255, 0.26), 22px 5px 0 rgba(255, 255, 255, 0.24);
}

.is-dark .fallback-atmosphere {
  width: 2px;
  height: 2px;
  border-radius: 50%;
  background: #dcebe6;
  box-shadow: -80px 18px 0 #dcebe6, -45px -26px 0 #dcebe6, 35px 31px 0 #dcebe6, 84px -12px 0 #dcebe6;
}

.fallback-celestial {
  --moon-shadow-offset: 0%;
  position: absolute;
  top: 16%;
  left: 71%;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  overflow: hidden;
  background: #eab84e;
  box-shadow: 0 0 0 10px rgba(234, 184, 78, 0.08);
}

.is-dark .fallback-celestial {
  background: #d9e3df;
  box-shadow: none;
}

.is-dark .fallback-celestial::after {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  content: "";
  background: #071b20;
  transform: translateX(var(--moon-shadow-offset));
}

.fallback-water {
  position: absolute;
  inset: 61% 0 0;
  background: linear-gradient(to bottom, #2c86df, #0b55ae);
}

.is-dark .fallback-water {
  background: linear-gradient(to bottom, #0b438c, #061f4f);
}

.fallback-terminal {
  position: absolute;
  z-index: 1;
  right: 18%;
  bottom: 29%;
  width: 30%;
  height: 25px;
  background: #71837e;
  transform: skewY(-2deg);
}

.fallback-gantry {
  position: absolute;
  z-index: 2;
  right: 20%;
  bottom: 32%;
  width: 38%;
  height: 118px;
  border-top: 5px solid #647a77;
  border-right: 4px solid #647a77;
  border-left: 4px solid #647a77;
}

.fallback-dock {
  position: absolute;
  z-index: 1;
  right: -4%;
  bottom: -12%;
  width: 27%;
  height: 55%;
  background: #71837e;
  clip-path: polygon(0 12%, 100% 0, 100% 100%, 34% 100%);
}

.fallback-lighthouse {
  position: absolute;
  z-index: 3;
  right: 7%;
  bottom: 17%;
  width: 44px;
  height: 128px;
  background: #f7f4e8;
  clip-path: polygon(36% 0, 64% 0, 100% 100%, 0 100%);
}

.fallback-ship {
  position: absolute;
  z-index: 2;
  width: 126px;
  height: 22px;
  background: #263b3d;
  clip-path: polygon(0 0, 100% 0, 78% 100%, 14% 100%);
}

.fallback-ship-inbound {
  bottom: 20%;
  left: 31%;
}

.fallback-ship-outbound {
  bottom: 31%;
  left: 9%;
  transform: scale(0.72) scaleX(-1);
}

@keyframes cargo-signal {
  0%, 72%, 100% {
    filter: brightness(0.86) saturate(0.84);
    transform: translateY(0);
  }
  82% {
    filter: brightness(1.14) saturate(1.05);
    transform: translateY(-2px);
  }
}

@media (min-width: 1800px) {
  .dock-model-badges {
    top: calc(63% + 10px);
    width: 240px;
    grid-template-columns: repeat(6, 35px);
    gap: 5px;
  }

  .dock-model-badge {
    width: 35px;
    height: 22px;
  }

  .dock-model-badge :deep(svg) {
    width: 15px;
    height: 15px;
  }

  .fallback-atmosphere {
    width: 88px;
    height: 21px;
  }

  .fallback-celestial {
    width: 56px;
    height: 56px;
  }

  .fallback-gantry {
    height: 137px;
  }

  .fallback-lighthouse {
    width: 51px;
    height: 148px;
  }

  .fallback-ship {
    width: 146px;
    height: 26px;
  }
}

@media (max-width: 640px) {
  .dock-model-badges {
    top: calc(61% - 15px);
    right: 24%;
    width: 112px;
    grid-template-columns: repeat(4, 25px);
    gap: 4px;
  }

  .dock-model-badge {
    width: 25px;
    height: 16px;
  }

  .fallback-terminal {
    right: 24%;
    width: 29%;
  }

  .fallback-gantry {
    right: 25%;
    width: 36%;
    height: 100px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .dock-model-badge {
    animation: none;
  }
}
</style>
