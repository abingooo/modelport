<template>
  <div ref="host" class="harbor-scene" :data-scene-ready="ready ? 'true' : 'false'">
    <canvas ref="canvas" class="h-full w-full" role="img" :aria-label="label" />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ dark: boolean; label: string }>()
const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const ready = ref(false)
const pointer = { currentX: 0, currentY: 0, targetX: 0, targetY: 0 }
let width = 0
let height = 0
let pixelRatio = 1
let scrollProgress = 0
let animationFrame = 0
let resizeObserver: ResizeObserver | null = null
let motionQuery: MediaQueryList | null = null
let reducedMotion = false

interface Palette {
  sky: string
  skyBand: string
  sea: string
  seaDeep: string
  horizon: string
  structure: string
  structureSoft: string
  dock: string
  dockEdge: string
  cream: string
  teal: string
  coral: string
  yellow: string
  route: string
}

function palette(): Palette {
  return props.dark ? {
    sky: '#07171c', skyBand: '#0c252b', sea: '#0b3640', seaDeep: '#082a32', horizon: '#17383b',
    structure: '#15272b', structureSoft: '#274044', dock: '#22363a', dockEdge: '#506669', cream: '#e4e7da',
    teal: '#35b8ad', coral: '#f1785c', yellow: '#f2c96b', route: '#8bd5ca',
  } : {
    sky: '#dfe9e4', skyBand: '#c9dcd6', sea: '#24717a', seaDeep: '#165965', horizon: '#8aa6a0',
    structure: '#263b3d', structureSoft: '#5e7472', dock: '#6b7d78', dockEdge: '#a6b5ae', cream: '#f7f4e8',
    teal: '#087f7a', coral: '#e7664a', yellow: '#eab84e', route: '#d8f2e9',
  }
}

function polygon(context: CanvasRenderingContext2D, points: Array<[number, number]>, fill: string) {
  if (!points.length) return
  context.beginPath()
  context.moveTo(points[0][0], points[0][1])
  for (let index = 1; index < points.length; index += 1) context.lineTo(points[index][0], points[index][1])
  context.closePath()
  context.fillStyle = fill
  context.fill()
}

function drawSkyline(context: CanvasRenderingContext2D, colors: Palette, horizonY: number, parallaxX: number) {
  context.fillStyle = colors.horizon
  const buildingWidths = [26, 42, 19, 55, 31, 47, 22, 64, 28, 38, 51, 24, 58, 34]
  let left = -30 + parallaxX * 0.25
  buildingWidths.forEach((buildingWidth, index) => {
    const buildingHeight = 20 + ((index * 23) % 56)
    context.fillRect(left, horizonY - buildingHeight, buildingWidth, buildingHeight)
    if (index % 3 === 0) context.fillRect(left + buildingWidth * 0.45, horizonY - buildingHeight - 15, 2, 15)
    left += buildingWidth + 8
  })
  context.fillRect(0, horizonY - 5, width, 8)
}

function drawCranes(context: CanvasRenderingContext2D, colors: Palette, horizonY: number, parallaxX: number) {
  context.strokeStyle = colors.structureSoft
  context.lineWidth = Math.max(1.5, width / 900)
  const cranePositions = [0.67, 0.77, 0.9]
  cranePositions.forEach((ratio, index) => {
    const craneX = width * ratio + parallaxX * (0.5 + index * 0.1)
    const craneTop = horizonY - 105 - index * 9
    context.beginPath()
    context.moveTo(craneX, horizonY + 24)
    context.lineTo(craneX, craneTop)
    context.lineTo(craneX + width * 0.1, craneTop + 12)
    context.moveTo(craneX - 18, craneTop + 34)
    context.lineTo(craneX, craneTop)
    context.moveTo(craneX + width * 0.065, craneTop + 8)
    context.lineTo(craneX + width * 0.065, craneTop + 58)
    context.stroke()
  })
}

function drawContainers(context: CanvasRenderingContext2D, colors: Palette, horizonY: number) {
  const containerColors = [colors.coral, colors.teal, colors.yellow, colors.structureSoft]
  const unitWidth = Math.max(20, width * 0.027)
  const unitHeight = Math.max(9, height * 0.016)
  const startX = width * 0.68
  for (let row = 0; row < 4; row += 1) {
    for (let column = 0; column < 8 - row; column += 1) {
      context.fillStyle = containerColors[(row + column * 2) % containerColors.length]
      context.fillRect(startX + column * (unitWidth + 3), horizonY + 15 - row * (unitHeight + 3), unitWidth, unitHeight)
      context.globalAlpha = 0.28
      context.fillStyle = colors.cream
      context.fillRect(startX + column * (unitWidth + 3) + 4, horizonY + 18 - row * (unitHeight + 3), 1, unitHeight - 6)
      context.globalAlpha = 1
    }
  }
}

function drawLighthouse(context: CanvasRenderingContext2D, colors: Palette, horizonY: number, time: number) {
  const lighthouseX = width * 0.855
  const baseY = horizonY + height * 0.18
  const towerHeight = Math.max(82, height * 0.17)
  const beamAngle = reducedMotion ? -0.22 : Math.sin(time * 0.00045) * 0.32 - 0.14
  const beamLength = Math.max(260, width * 0.32)

  context.save()
  context.translate(lighthouseX, baseY - towerHeight + 6)
  context.rotate(beamAngle)
  context.globalAlpha = props.dark ? 0.12 : 0.09
  polygon(context, [[0, -5], [-beamLength, -52], [-beamLength, 52]], colors.yellow)
  context.restore()
  context.globalAlpha = 1

  polygon(context, [
    [lighthouseX - 18, baseY], [lighthouseX + 18, baseY],
    [lighthouseX + 10, baseY - towerHeight], [lighthouseX - 10, baseY - towerHeight],
  ], colors.cream)
  context.fillStyle = colors.coral
  for (let stripe = 0; stripe < 3; stripe += 1) {
    context.fillRect(lighthouseX - 13 + stripe * 2, baseY - 28 - stripe * 25, 26 - stripe * 4, 10)
  }
  context.fillStyle = colors.structure
  context.fillRect(lighthouseX - 15, baseY - towerHeight - 7, 30, 8)
  context.fillStyle = colors.yellow
  context.fillRect(lighthouseX - 7, baseY - towerHeight - 3, 14, 11)
  polygon(context, [[lighthouseX - 18, baseY - towerHeight - 7], [lighthouseX, baseY - towerHeight - 21], [lighthouseX + 18, baseY - towerHeight - 7]], colors.structure)
}

function drawShip(context: CanvasRenderingContext2D, colors: Palette, shipX: number, shipY: number, scale: number, accent: string) {
  context.save()
  context.translate(shipX, shipY)
  context.scale(scale, scale)
  polygon(context, [[-78, 0], [88, 0], [62, 25], [-58, 25]], colors.structure)
  context.fillStyle = colors.cream
  context.fillRect(-54, -17, 34, 17)
  context.fillStyle = colors.structureSoft
  context.fillRect(-47, -28, 20, 11)
  const cargoColors = [accent, colors.teal, colors.yellow, colors.coral]
  for (let row = 0; row < 3; row += 1) {
    for (let column = 0; column < 5 - row; column += 1) {
      context.fillStyle = cargoColors[(column + row) % cargoColors.length]
      context.fillRect(-10 + column * 19, -12 - row * 13, 16, 10)
    }
  }
  context.fillStyle = colors.route
  context.globalAlpha = 0.55
  context.fillRect(-94, 29, 138, 2)
  context.restore()
  context.globalAlpha = 1
}

function drawRoutes(context: CanvasRenderingContext2D, colors: Palette, horizonY: number, time: number) {
  const routes = [
    { startX: width * 0.08, startY: height * 0.82, controlX: width * 0.36, controlY: height * 0.54, endX: width * 0.69, endY: horizonY + 26, color: colors.teal },
    { startX: width * 0.23, startY: height * 0.93, controlX: width * 0.5, controlY: height * 0.66, endX: width * 0.83, endY: horizonY + 42, color: colors.coral },
    { startX: width * 0.02, startY: height * 0.69, controlX: width * 0.38, controlY: height * 0.61, endX: width * 0.92, endY: horizonY + 70, color: colors.yellow },
  ]

  routes.forEach((route, routeIndex) => {
    context.beginPath()
    context.moveTo(route.startX, route.startY)
    context.quadraticCurveTo(route.controlX, route.controlY, route.endX, route.endY)
    context.strokeStyle = route.color
    context.globalAlpha = props.dark ? 0.25 : 0.32
    context.lineWidth = 1.2
    context.setLineDash([6, 9])
    context.stroke()
    context.setLineDash([])

    const pulseCount = reducedMotion ? 1 : 3
    for (let pulseIndex = 0; pulseIndex < pulseCount; pulseIndex += 1) {
      const phase = reducedMotion ? 0.58 : ((time * 0.000055 + pulseIndex / pulseCount + routeIndex * 0.19) % 1)
      const inverse = 1 - phase
      const pulseX = inverse * inverse * route.startX + 2 * inverse * phase * route.controlX + phase * phase * route.endX
      const pulseY = inverse * inverse * route.startY + 2 * inverse * phase * route.controlY + phase * phase * route.endY
      context.globalAlpha = 0.8
      context.fillStyle = route.color
      context.fillRect(pulseX - 2, pulseY - 2, 4, 4)
    }
  })
  context.globalAlpha = 1
}

function drawWaves(context: CanvasRenderingContext2D, colors: Palette, horizonY: number, time: number) {
  context.strokeStyle = colors.route
  context.globalAlpha = props.dark ? 0.15 : 0.22
  context.lineWidth = 1
  const waveOffset = reducedMotion ? 0 : (time * 0.015) % 48
  for (let row = 0; row < 8; row += 1) {
    const waveY = horizonY + 38 + row * height * 0.055
    context.beginPath()
    for (let waveX = -60 + waveOffset; waveX < width + 60; waveX += 48) {
      context.moveTo(waveX, waveY)
      context.quadraticCurveTo(waveX + 12, waveY - 3, waveX + 24, waveY)
      context.quadraticCurveTo(waveX + 36, waveY + 3, waveX + 48, waveY)
    }
    context.stroke()
  }
  context.globalAlpha = 1
}

function render(time: number) {
  const element = canvas.value
  if (!element || width <= 0 || height <= 0) return
  const context = element.getContext('2d')
  if (!context) return
  pointer.currentX += (pointer.targetX - pointer.currentX) * 0.045
  pointer.currentY += (pointer.targetY - pointer.currentY) * 0.045
  const colors = palette()
  const horizonRatio = width < 640 ? 0.59 : width < 900 ? 0.56 : 0.53
  const horizonY = height * (horizonRatio + scrollProgress * 0.025)
  const parallaxX = pointer.currentX * Math.min(16, width * 0.015)
  const parallaxY = pointer.currentY * Math.min(8, height * 0.012)

  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
  context.clearRect(0, 0, width, height)
  context.fillStyle = colors.sky
  context.fillRect(0, 0, width, height)
  context.fillStyle = colors.skyBand
  context.fillRect(0, horizonY - height * 0.16 + parallaxY * 0.3, width, height * 0.17)

  context.fillStyle = colors.yellow
  context.globalAlpha = props.dark ? 0.65 : 0.86
  context.beginPath()
  context.arc(width * 0.7 + parallaxX * 0.18, height * 0.19 + parallaxY * 0.2, Math.max(18, Math.min(width, height) * 0.035), 0, Math.PI * 2)
  context.fill()
  context.globalAlpha = 1

  drawSkyline(context, colors, horizonY, parallaxX)
  context.fillStyle = colors.sea
  context.fillRect(0, horizonY, width, height - horizonY)
  polygon(context, [[0, height * 0.83], [width, height * 0.72], [width, height], [0, height]], colors.seaDeep)
  drawWaves(context, colors, horizonY, time)
  drawRoutes(context, colors, horizonY, time)

  polygon(context, [[width * 0.62, horizonY + 5], [width, horizonY - 5], [width, height], [width * 0.78, height]], colors.dock)
  polygon(context, [[width * 0.74, horizonY + height * 0.17], [width, horizonY + height * 0.11], [width, horizonY + height * 0.14], [width * 0.76, horizonY + height * 0.2]], colors.dockEdge)
  drawCranes(context, colors, horizonY, parallaxX)
  drawContainers(context, colors, horizonY)
  drawLighthouse(context, colors, horizonY, time)

  const primaryProgress = reducedMotion ? 0.42 : ((time * 0.000018) % 1)
  const primaryShipX = -width * 0.12 + primaryProgress * width * 0.86 + parallaxX
  drawShip(context, colors, primaryShipX, height * 0.69 + Math.sin(time * 0.0011) * 2, Math.max(0.62, Math.min(1.15, width / 1250)), colors.coral)
  const secondaryProgress = reducedMotion ? 0.7 : ((time * 0.000025 + 0.46) % 1)
  drawShip(context, colors, width * 0.08 + secondaryProgress * width * 0.48, height * 0.83, Math.max(0.3, Math.min(0.52, width / 2100)), colors.teal)

  ready.value = true
}

function scheduleFrame(time: number) {
  render(time)
  if (!reducedMotion && !document.hidden) animationFrame = requestAnimationFrame(scheduleFrame)
}

function restartAnimation() {
  cancelAnimationFrame(animationFrame)
  render(performance.now())
  if (!reducedMotion && !document.hidden) animationFrame = requestAnimationFrame(scheduleFrame)
}

function resize() {
  const element = canvas.value
  const container = host.value
  if (!element || !container) return
  const bounds = container.getBoundingClientRect()
  width = Math.max(1, Math.round(bounds.width))
  height = Math.max(1, Math.round(bounds.height))
  pixelRatio = Math.min(2, window.devicePixelRatio || 1)
  element.width = Math.round(width * pixelRatio)
  element.height = Math.round(height * pixelRatio)
  render(performance.now())
}

function updatePointer(clientX: number, clientY: number) {
  const bounds = host.value?.getBoundingClientRect()
  if (!bounds) return
  pointer.targetX = ((clientX - bounds.left) / bounds.width - 0.5) * 2
  pointer.targetY = ((clientY - bounds.top) / bounds.height - 0.5) * 2
  if (reducedMotion) render(performance.now())
}

function onPointerMove(event: PointerEvent) { updatePointer(event.clientX, event.clientY) }
function onTouchMove(event: TouchEvent) {
  const touch = event.touches[0]
  if (touch) updatePointer(touch.clientX, touch.clientY)
}
function onScroll() {
  scrollProgress = Math.min(1, Math.max(0, window.scrollY / Math.max(1, height)))
  if (reducedMotion) render(performance.now())
}
function onVisibilityChange() { restartAnimation() }
function onMotionChange(event: MediaQueryListEvent) { reducedMotion = event.matches; restartAnimation() }

watch(() => props.dark, () => nextTick(() => render(performance.now())))
onMounted(() => {
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion = motionQuery.matches
  motionQuery.addEventListener('change', onMotionChange)
  resizeObserver = new ResizeObserver(resize)
  if (host.value) resizeObserver.observe(host.value)
  host.value?.addEventListener('pointermove', onPointerMove)
  host.value?.addEventListener('touchmove', onTouchMove, { passive: true })
  window.addEventListener('scroll', onScroll, { passive: true })
  document.addEventListener('visibilitychange', onVisibilityChange)
  resize()
  restartAnimation()
})
onBeforeUnmount(() => {
  cancelAnimationFrame(animationFrame)
  resizeObserver?.disconnect()
  motionQuery?.removeEventListener('change', onMotionChange)
  host.value?.removeEventListener('pointermove', onPointerMove)
  host.value?.removeEventListener('touchmove', onTouchMove)
  window.removeEventListener('scroll', onScroll)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.harbor-scene {
  position: absolute;
  inset: 0;
  overflow: hidden;
  contain: strict;
  touch-action: pan-y;
}
</style>
