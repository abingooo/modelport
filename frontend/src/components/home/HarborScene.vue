<template>
  <div
    ref="host"
    class="harbor-scene"
    :data-scene-ready="ready ? 'true' : 'false'"
    data-testid="harbor-scene"
  >
    <canvas ref="canvas" role="img" :aria-label="label" />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ dark: boolean; label: string }>()
const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const ready = ref(false)

let width = 0
let height = 0
let pixelRatio = 1
let animationFrame = 0
let scrollProgress = 0
let resizeObserver: ResizeObserver | null = null
let motionQuery: MediaQueryList | null = null
let reducedMotion = false
const pointer = { x: 0, y: 0, targetX: 0, targetY: 0 }

interface Route {
  start: [number, number]
  control: [number, number]
  end: [number, number]
  color: string
  offset: number
}

function routePoint(route: Route, progress: number): [number, number] {
  const inverse = 1 - progress
  return [
    inverse * inverse * route.start[0] + 2 * inverse * progress * route.control[0] + progress * progress * route.end[0],
    inverse * inverse * route.start[1] + 2 * inverse * progress * route.control[1] + progress * progress * route.end[1],
  ]
}

function routeAngle(route: Route, progress: number): number {
  const x = 2 * (1 - progress) * (route.control[0] - route.start[0]) + 2 * progress * (route.end[0] - route.control[0])
  const y = 2 * (1 - progress) * (route.control[1] - route.start[1]) + 2 * progress * (route.end[1] - route.control[1])
  return Math.atan2(y, x)
}

function drawVessel(context: CanvasRenderingContext2D, x: number, y: number, angle: number, color: string) {
  const scale = Math.max(0.72, Math.min(1.15, width / 1200))
  context.save()
  context.translate(x, y)
  context.rotate(angle)
  context.scale(scale, scale)
  context.beginPath()
  context.moveTo(13, 0)
  context.lineTo(5, -5)
  context.lineTo(-10, -5)
  context.lineTo(-14, 0)
  context.lineTo(-10, 5)
  context.lineTo(5, 5)
  context.closePath()
  context.fillStyle = color
  context.shadowColor = color
  context.shadowBlur = 12
  context.fill()
  context.shadowBlur = 0
  context.fillStyle = 'rgba(255, 255, 255, 0.92)'
  context.fillRect(-5, -2, 7, 4)
  context.restore()
}

function drawLighthouse(context: CanvasRenderingContext2D, x: number, y: number, time: number, color: string) {
  const angle = reducedMotion ? -0.7 : time * 0.0005
  const beamLength = Math.max(90, Math.min(180, width * 0.13))
  context.save()
  context.translate(x, y)
  context.rotate(angle)
  context.beginPath()
  context.moveTo(0, 0)
  context.lineTo(-beamLength, -18)
  context.lineTo(-beamLength, 18)
  context.closePath()
  context.fillStyle = props.dark ? 'rgba(255, 210, 112, 0.12)' : 'rgba(255, 223, 145, 0.16)'
  context.fill()
  context.restore()

  context.beginPath()
  context.arc(x, y, 18, 0, Math.PI * 2)
  context.strokeStyle = 'rgba(255, 255, 255, 0.28)'
  context.lineWidth = 1
  context.stroke()
  context.beginPath()
  context.arc(x, y, 5, 0, Math.PI * 2)
  context.fillStyle = color
  context.shadowColor = color
  context.shadowBlur = 18
  context.fill()
  context.shadowBlur = 0
}

function drawRoute(context: CanvasRenderingContext2D, route: Route, time: number) {
  context.beginPath()
  context.moveTo(route.start[0], route.start[1])
  context.quadraticCurveTo(route.control[0], route.control[1], route.end[0], route.end[1])
  context.strokeStyle = route.color
  context.globalAlpha = props.dark ? 0.42 : 0.48
  context.lineWidth = 1.25
  context.setLineDash([5, 9])
  context.stroke()
  context.setLineDash([])

  const progress = reducedMotion ? 0.56 : (time * 0.000045 + route.offset) % 1
  const [vesselX, vesselY] = routePoint(route, progress)
  drawVessel(context, vesselX, vesselY, routeAngle(route, progress), route.color)

  for (let index = 0; index < 3; index += 1) {
    const pulseProgress = reducedMotion
      ? (index + 1) / 4
      : (time * 0.00008 + route.offset + index / 3) % 1
    const [pulseX, pulseY] = routePoint(route, pulseProgress)
    context.beginPath()
    context.arc(pulseX, pulseY, 2.2, 0, Math.PI * 2)
    context.fillStyle = route.color
    context.globalAlpha = 0.92
    context.fill()
  }
  context.globalAlpha = 1
}

function render(time: number) {
  const element = canvas.value
  const context = element?.getContext('2d')
  if (!element || !context || width <= 0 || height <= 0) return

  pointer.x += (pointer.targetX - pointer.x) * 0.045
  pointer.y += (pointer.targetY - pointer.y) * 0.045
  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
  context.clearRect(0, 0, width, height)

  const shiftX = pointer.x * Math.min(16, width * 0.012)
  const shiftY = pointer.y * Math.min(8, height * 0.012) + scrollProgress * 10
  const colors = props.dark
    ? ['#63d5c6', '#ff8b70', '#ffd376']
    : ['#70e6d8', '#ff947d', '#ffd982']
  const lighthouse: [number, number] = [width * 0.78 + shiftX * 0.25, height * 0.47 + shiftY * 0.2]
  const routes: Route[] = [
    { start: [width * 0.02, height * 0.78], control: [width * 0.38 + shiftX, height * 0.6 + shiftY], end: lighthouse, color: colors[0], offset: 0.08 },
    { start: [width * 0.22, height * 0.94], control: [width * 0.48 + shiftX, height * 0.73 + shiftY], end: lighthouse, color: colors[1], offset: 0.42 },
    { start: [width * 0.48, height * 0.96], control: [width * 0.67 + shiftX, height * 0.76 + shiftY], end: lighthouse, color: colors[2], offset: 0.74 },
  ]

  routes.forEach((route) => drawRoute(context, route, time))
  drawLighthouse(context, lighthouse[0], lighthouse[1], time, colors[2])
  ready.value = true
}

function animate(time: number) {
  render(time)
  if (!reducedMotion && !document.hidden) animationFrame = requestAnimationFrame(animate)
}

function restart() {
  cancelAnimationFrame(animationFrame)
  render(performance.now())
  if (!reducedMotion && !document.hidden) animationFrame = requestAnimationFrame(animate)
}

function resize() {
  const element = canvas.value
  const bounds = host.value?.getBoundingClientRect()
  if (!element || !bounds) return
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
  pointer.targetX = ((clientX - bounds.left) / Math.max(1, bounds.width) - 0.5) * 2
  pointer.targetY = ((clientY - bounds.top) / Math.max(1, bounds.height) - 0.5) * 2
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
function onVisibilityChange() { restart() }
function onMotionChange(event: MediaQueryListEvent) { reducedMotion = event.matches; restart() }

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
  restart()
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
  z-index: 2;
  inset: 0;
  overflow: hidden;
  touch-action: pan-y;
}

canvas {
  display: block;
  width: 100%;
  height: 100%;
}
</style>
