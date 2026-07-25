<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe v-if="isHomeContentUrl" :src="homeContent.trim()" class="h-screen w-full border-0" allowfullscreen />
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="harbor-home" :class="{ 'harbor-home-dark': isDark }">
    <section class="harbor-hero">
      <HarborScene :dark="isDark" :label="t('home.harbor.sceneLabel')" />

      <header class="harbor-header">
        <nav class="harbor-shell flex h-full items-center justify-between gap-4" :aria-label="t('home.harbor.primaryNav')">
          <RouterLink to="/home" class="harbor-brand" aria-label="ModelPort home">
            <span class="harbor-brand-mark">
              <BrandLogo :site-name="siteName" :site-logo="siteLogo" image-class="h-full w-full object-contain" />
            </span>
            <span class="harbor-brand-name">{{ siteName }}</span>
          </RouterLink>

          <div class="hidden items-center gap-7 md:flex">
            <RouterLink to="/model-catalog" class="harbor-nav-link">{{ t('home.harbor.nav.catalog') }}</RouterLink>
            <RouterLink to="/lottery" class="harbor-nav-link">{{ t('home.harbor.nav.lottery') }}</RouterLink>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="harbor-nav-link">{{ t('home.docs') }}</a>
          </div>

          <div class="flex shrink-0 items-center gap-1 sm:gap-2">
            <LocaleSwitcher />
            <button type="button" class="harbor-icon-button" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme">
              <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
            </button>
            <RouterLink :to="isAuthenticated ? dashboardPath : '/login'" class="harbor-account-link">
              <span v-if="isAuthenticated" class="harbor-user-initial">{{ userInitial }}</span>
              <Icon v-else name="login" size="sm" />
              <span class="hidden sm:inline">{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
              <Icon name="arrowRight" size="xs" />
            </RouterLink>
          </div>
        </nav>
      </header>

      <div class="harbor-shell harbor-hero-content">
        <p class="harbor-kicker"><span />{{ t('home.harbor.eyebrow') }}</p>
        <h1 class="harbor-title">{{ siteName }}</h1>
        <p class="harbor-lead">{{ t('home.harbor.heroDescription') }}</p>
        <p v-if="siteSubtitle" class="harbor-subtitle">{{ siteSubtitle }}</p>
        <div class="mt-8 flex flex-col gap-3 sm:flex-row">
          <RouterLink to="/model-catalog" class="harbor-primary-action">
            <Icon name="grid" size="md" />{{ t('home.harbor.exploreModels') }}<Icon name="arrowRight" size="sm" />
          </RouterLink>
          <RouterLink :to="isAuthenticated ? '/keys' : '/login'" class="harbor-secondary-action">
            <Icon name="key" size="md" />{{ isAuthenticated ? t('home.harbor.manageKeys') : t('home.getStarted') }}
          </RouterLink>
        </div>
      </div>

      <div class="harbor-signal-rail">
        <div class="harbor-shell grid h-full grid-cols-3">
          <div v-for="signal in signals" :key="signal.label" class="harbor-signal">
            <span :class="['harbor-signal-light', signal.color]" />
            <span><strong>{{ signal.value }}</strong><small>{{ signal.label }}</small></span>
          </div>
        </div>
      </div>
    </section>

    <main>
      <section class="harbor-routes-section">
        <div class="harbor-shell py-20 md:py-24">
          <div class="max-w-2xl">
            <p class="section-index">01 / {{ t('home.harbor.routes.index') }}</p>
            <h2 class="section-title">{{ t('home.harbor.routes.title') }}</h2>
            <p class="section-description">{{ t('home.harbor.routes.description') }}</p>
          </div>

          <div class="route-grid mt-12">
            <RouterLink v-for="route in harborRoutes" :key="route.path" :to="route.path" class="route-berth">
              <div class="flex items-start justify-between gap-5">
                <span class="route-number">{{ route.number }}</span>
                <Icon name="arrowRight" size="md" class="route-arrow" />
              </div>
              <Icon :name="route.icon" size="xl" class="mt-9" :class="route.accent" />
              <h3>{{ route.title }}</h3>
              <p>{{ route.description }}</p>
            </RouterLink>
          </div>
        </div>
      </section>

      <section class="harbor-manifest-section">
        <div class="harbor-shell py-20 md:py-24">
          <div class="manifest-heading">
            <div>
              <p class="section-index section-index-dark">02 / {{ t('home.harbor.manifest.index') }}</p>
              <h2 class="section-title section-title-dark">{{ t('home.harbor.manifest.title') }}</h2>
            </div>
            <p class="section-description section-description-dark">{{ t('home.harbor.manifest.description') }}</p>
          </div>

          <div class="manifest-track" aria-label="Supported providers">
            <div v-for="provider in providers" :key="provider.platform" class="manifest-provider">
              <span :class="['manifest-icon', platformBadgeClass(provider.platform)]">
                <PlatformIcon :platform="provider.platform" size="lg" />
              </span>
              <span class="min-w-0"><strong>{{ platformLabel(provider.platform) }}</strong><small>{{ provider.lane }}</small></span>
              <span class="manifest-status">{{ t('home.harbor.manifest.online') }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="harbor-flow-section">
        <div class="harbor-shell py-20 md:py-28">
          <p class="section-index">03 / {{ t('home.harbor.flow.index') }}</p>
          <div class="flow-layout mt-6">
            <div>
              <h2 class="section-title">{{ t('home.harbor.flow.title') }}</h2>
              <p class="section-description max-w-xl">{{ t('home.harbor.flow.description') }}</p>
            </div>
            <div class="flow-map" aria-hidden="true">
              <div v-for="(step, index) in flowSteps" :key="step.title" class="flow-step">
                <span class="flow-node">{{ String(index + 1).padStart(2, '0') }}</span>
                <span><strong>{{ step.title }}</strong><small>{{ step.description }}</small></span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="harbor-cta-section">
        <div class="harbor-shell harbor-cta-inner">
          <div>
            <p class="section-index section-index-cta">04 / {{ t('home.harbor.cta.index') }}</p>
            <h2>{{ t('home.harbor.cta.title') }}</h2>
          </div>
          <RouterLink :to="isAuthenticated ? '/keys' : '/login'" class="harbor-cta-button">
            <Icon name="key" size="md" />{{ isAuthenticated ? t('home.harbor.manageKeys') : t('home.harbor.cta.action') }}<Icon name="arrowRight" size="sm" />
          </RouterLink>
        </div>
      </section>
    </main>

    <footer class="harbor-footer">
      <div class="harbor-shell flex flex-col gap-4 py-7 sm:flex-row sm:items-center sm:justify-between">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div class="flex items-center gap-5">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import BrandLogo from '@/components/common/BrandLogo.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import HarborScene from '@/components/home/HarborScene.vue'
import Icon from '@/components/icons/Icon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const isDark = ref(document.documentElement.classList.contains('dark'))

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'ModelPort')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})
const githubUrl = computed(() => siteName.value.trim().toLowerCase() === 'modelport'
  ? 'https://github.com/abingooo/modelport'
  : 'https://github.com/Wei-Shaw/sub2api')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => authStore.user?.email?.charAt(0).toUpperCase() || 'M')
const currentYear = computed(() => new Date().getFullYear())

const signals = computed(() => [
  { value: t('home.harbor.signals.oneKey'), label: t('home.harbor.signals.oneKeyLabel'), color: 'signal-teal' },
  { value: t('home.harbor.signals.routes'), label: t('home.harbor.signals.routesLabel'), color: 'signal-coral' },
  { value: t('home.harbor.signals.billing'), label: t('home.harbor.signals.billingLabel'), color: 'signal-yellow' },
])
const harborRoutes = computed(() => [
  { number: 'A1', path: '/model-catalog', icon: 'grid' as const, accent: 'text-teal-700 dark:text-teal-300', title: t('home.harbor.routes.catalog.title'), description: t('home.harbor.routes.catalog.description') },
  { number: 'B2', path: isAuthenticated.value ? '/keys' : '/login', icon: 'key' as const, accent: 'text-coral-600', title: t('home.harbor.routes.keys.title'), description: t('home.harbor.routes.keys.description') },
  { number: 'C3', path: '/lottery', icon: 'gift' as const, accent: 'text-amber-600 dark:text-amber-300', title: t('home.harbor.routes.lottery.title'), description: t('home.harbor.routes.lottery.description') },
])
const providers: Array<{ platform: GroupPlatform; lane: string }> = [
  { platform: 'openai', lane: 'ATL-01' }, { platform: 'anthropic', lane: 'PAC-02' },
  { platform: 'gemini', lane: 'ORB-03' }, { platform: 'deepseek', lane: 'DPS-04' },
  { platform: 'qwen', lane: 'QWN-05' }, { platform: 'glm', lane: 'GLM-06' },
  { platform: 'kimi', lane: 'KMI-07' }, { platform: 'doubao', lane: 'BYT-08' },
  { platform: 'siliconflow', lane: 'SFL-09' }, { platform: 'minimax', lane: 'MMX-10' },
  { platform: 'mimo', lane: 'MIM-11' }, { platform: 'grok', lane: 'XAI-12' },
]
const flowSteps = computed(() => [
  { title: t('home.harbor.flow.steps.request.title'), description: t('home.harbor.flow.steps.request.description') },
  { title: t('home.harbor.flow.steps.route.title'), description: t('home.harbor.flow.steps.route.description') },
  { title: t('home.harbor.flow.steps.model.title'), description: t('home.harbor.flow.steps.model.description') },
])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}
onMounted(() => {
  initTheme()
  void authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) void appStore.fetchPublicSettings()
})
</script>

<style scoped>
.harbor-home {
  --paper: #f4f2e8;
  --ink: #152b2d;
  --muted: #586b69;
  --line: #bdc8c1;
  --teal: #087f7a;
  --coral: #e7664a;
  --yellow: #eab84e;
  min-height: 100vh;
  background: var(--paper);
  color: var(--ink);
}

.harbor-home-dark {
  --paper: #0a171b;
  --ink: #e9eee6;
  --muted: #9aadaa;
  --line: #304548;
  --teal: #35b8ad;
  --coral: #f1785c;
  --yellow: #f2c96b;
}

.harbor-shell {
  width: min(100% - 32px, 1240px);
  margin-inline: auto;
}

.harbor-hero {
  position: relative;
  height: 92svh;
  min-height: 540px;
  overflow: hidden;
  background: #dfe9e4;
}

.harbor-home-dark .harbor-hero { background: #07171c; }

.harbor-header {
  position: relative;
  z-index: 10;
  height: 72px;
  border-bottom: 1px solid rgba(21, 43, 45, 0.18);
}

.harbor-home-dark .harbor-header { border-color: rgba(228, 231, 218, 0.16); }

.harbor-brand { display: inline-flex; min-width: 0; align-items: center; gap: 11px; }
.harbor-brand-mark { display: flex; width: 34px; height: 34px; flex: 0 0 34px; align-items: center; justify-content: center; overflow: hidden; border-radius: 6px; }
.harbor-brand-name { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 17px; font-weight: 750; color: var(--ink); }
.harbor-nav-link { font-size: 13px; font-weight: 650; color: var(--muted); transition: color 160ms ease; }
.harbor-nav-link:hover { color: var(--ink); }
.harbor-icon-button { display: inline-flex; width: 38px; height: 38px; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 6px; color: var(--muted); }
.harbor-icon-button:hover { border-color: var(--line); color: var(--ink); }
.harbor-account-link { display: inline-flex; min-height: 38px; align-items: center; gap: 7px; border-radius: 6px; background: var(--ink); padding: 0 12px; font-size: 12px; font-weight: 700; color: var(--paper); }
.harbor-user-initial { display: inline-flex; width: 20px; height: 20px; align-items: center; justify-content: center; border-radius: 50%; background: var(--coral); color: #fff; font-size: 10px; }

.harbor-hero-content {
  position: relative;
  z-index: 4;
  display: flex;
  height: calc(100% - 152px);
  flex-direction: column;
  justify-content: center;
  padding-bottom: 2vh;
  pointer-events: none;
}

.harbor-hero-content a { pointer-events: auto; }
.harbor-kicker { display: flex; align-items: center; gap: 10px; font-family: ui-monospace, monospace; font-size: 11px; font-weight: 700; text-transform: uppercase; color: var(--muted); }
.harbor-kicker span { width: 32px; height: 2px; background: var(--coral); }
.harbor-title { max-width: 760px; margin-top: 18px; overflow-wrap: anywhere; font-size: 76px; font-weight: 800; line-height: 0.94; letter-spacing: 0; color: var(--ink); text-wrap: balance; }
.harbor-lead { max-width: 590px; margin-top: 24px; font-size: 20px; font-weight: 560; line-height: 1.55; color: var(--ink); text-wrap: balance; }
.harbor-subtitle { max-width: 560px; margin-top: 8px; font-size: 13px; color: var(--muted); }
.harbor-primary-action, .harbor-secondary-action, .harbor-cta-button { display: inline-flex; min-height: 46px; align-items: center; justify-content: center; gap: 9px; border-radius: 6px; padding: 0 18px; font-size: 13px; font-weight: 750; transition: transform 160ms ease, background 160ms ease; }
.harbor-primary-action { background: var(--coral); color: #fff; }
.harbor-primary-action:hover, .harbor-cta-button:hover { transform: translateY(-2px); background: #d9583e; }
.harbor-secondary-action { border: 1px solid rgba(21, 43, 45, 0.32); background: rgba(244, 242, 232, 0.56); color: var(--ink); backdrop-filter: blur(8px); }
.harbor-home-dark .harbor-secondary-action { border-color: rgba(228, 231, 218, 0.28); background: rgba(10, 23, 27, 0.5); }
.harbor-secondary-action:hover { transform: translateY(-2px); border-color: var(--teal); }

.harbor-signal-rail { position: absolute; z-index: 5; right: 0; bottom: 0; left: 0; height: 80px; border-top: 1px solid rgba(21, 43, 45, 0.2); background: rgba(223, 233, 228, 0.74); backdrop-filter: blur(10px); }
.harbor-home-dark .harbor-signal-rail { border-color: rgba(228, 231, 218, 0.15); background: rgba(7, 23, 28, 0.75); }
.harbor-signal { display: flex; min-width: 0; align-items: center; gap: 12px; border-right: 1px solid rgba(21, 43, 45, 0.16); padding: 0 24px; }
.harbor-home-dark .harbor-signal { border-color: rgba(228, 231, 218, 0.12); }
.harbor-signal:last-child { border-right: 0; }
.harbor-signal-light { width: 7px; height: 7px; flex: 0 0 7px; border-radius: 50%; }
.signal-teal { background: var(--teal); box-shadow: 0 0 12px var(--teal); }
.signal-coral { background: var(--coral); box-shadow: 0 0 12px var(--coral); }
.signal-yellow { background: var(--yellow); box-shadow: 0 0 12px var(--yellow); }
.harbor-signal strong, .harbor-signal small { display: block; }
.harbor-signal > span:last-child { min-width: 0; }
.harbor-signal strong { font-family: ui-monospace, monospace; font-size: 13px; color: var(--ink); }
.harbor-signal small { margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; text-transform: uppercase; color: var(--muted); }

.harbor-routes-section, .harbor-flow-section { background: var(--paper); }
.section-index { font-family: ui-monospace, monospace; font-size: 10px; font-weight: 700; text-transform: uppercase; color: var(--teal); }
.section-title { margin-top: 14px; font-size: 38px; font-weight: 780; line-height: 1.13; letter-spacing: 0; color: var(--ink); text-wrap: balance; }
.section-description { margin-top: 14px; font-size: 15px; line-height: 1.75; color: var(--muted); }
.route-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); }
.route-berth { min-width: 0; padding: 28px; border-right: 1px solid var(--line); transition: background 180ms ease; }
.route-berth:last-child { border-right: 0; }
.route-berth:hover { background: rgba(8, 127, 122, 0.08); }
.route-number { font-family: ui-monospace, monospace; font-size: 11px; color: var(--muted); }
.route-arrow { color: var(--muted); transition: transform 180ms ease, color 180ms ease; }
.route-berth:hover .route-arrow { transform: translateX(4px); color: var(--coral); }
.route-berth h3 { margin-top: 20px; font-size: 18px; font-weight: 730; color: var(--ink); }
.route-berth p { margin-top: 8px; font-size: 13px; line-height: 1.65; color: var(--muted); }

.harbor-manifest-section { background: #10282d; color: #e8eee6; }
.manifest-heading { display: grid; grid-template-columns: 1fr minmax(280px, 0.6fr); align-items: end; gap: 48px; }
.section-index-dark { color: #6ed0c4; }
.section-title-dark { color: #edf1e8; }
.section-description-dark { color: #a3b4b0; }
.manifest-track { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-top: 48px; border-top: 1px solid #395055; border-left: 1px solid #395055; }
.manifest-provider { display: flex; min-width: 0; min-height: 88px; align-items: center; gap: 11px; border-right: 1px solid #395055; border-bottom: 1px solid #395055; padding: 14px; }
.manifest-icon { display: inline-flex; width: 34px; height: 34px; flex: 0 0 34px; align-items: center; justify-content: center; border: 1px solid; border-radius: 6px; }
.manifest-provider strong, .manifest-provider small { display: block; }
.manifest-provider strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; color: #edf1e8; }
.manifest-provider small { margin-top: 4px; font-family: ui-monospace, monospace; font-size: 9px; color: #78908d; }
.manifest-status { margin-left: auto; font-family: ui-monospace, monospace; font-size: 8px; text-transform: uppercase; color: #6ed0c4; }

.flow-layout { display: grid; grid-template-columns: minmax(0, 0.8fr) minmax(420px, 1.2fr); align-items: start; gap: 72px; }
.flow-map { border-top: 1px solid var(--line); }
.flow-step { position: relative; display: grid; grid-template-columns: 54px 1fr; min-height: 92px; align-items: center; border-bottom: 1px solid var(--line); }
.flow-step::before { position: absolute; top: -1px; left: 22px; width: 10px; height: 10px; border-radius: 50%; background: var(--coral); content: ''; transform: translateY(-50%); }
.flow-node { font-family: ui-monospace, monospace; font-size: 10px; color: var(--teal); }
.flow-step strong, .flow-step small { display: block; }
.flow-step strong { font-size: 14px; color: var(--ink); }
.flow-step small { margin-top: 5px; font-size: 12px; color: var(--muted); }

.harbor-cta-section { background: var(--coral); color: #fff; }
.harbor-cta-inner { display: flex; min-height: 250px; align-items: center; justify-content: space-between; gap: 40px; }
.section-index-cta { color: #ffe3d9; }
.harbor-cta-inner h2 { margin-top: 12px; max-width: 760px; font-size: 38px; font-weight: 780; line-height: 1.18; letter-spacing: 0; text-wrap: balance; }
.harbor-cta-button { flex: 0 0 auto; background: #10282d; color: #fff; }
.harbor-footer { border-top: 1px solid var(--line); background: var(--paper); font-size: 11px; color: var(--muted); }
.harbor-footer a:hover { color: var(--ink); }
.text-coral-600 { color: var(--coral); }

@media (max-width: 900px) {
  .harbor-title { font-size: 58px; }
  .harbor-lead { max-width: 520px; font-size: 17px; }
  .route-grid { grid-template-columns: 1fr; }
  .route-berth { border-right: 0; border-bottom: 1px solid var(--line); }
  .route-berth:last-child { border-bottom: 0; }
  .manifest-track { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .flow-layout { grid-template-columns: 1fr; gap: 44px; }
}

@media (max-width: 640px) {
  .harbor-shell { width: min(100% - 24px, 1240px); }
  .harbor-hero { height: 92svh; min-height: 540px; }
  .harbor-header { height: 64px; }
  .harbor-brand-name { max-width: 112px; font-size: 14px; }
  .harbor-brand-mark { width: 30px; height: 30px; flex-basis: 30px; }
  .harbor-hero-content { height: calc(100% - 136px); justify-content: flex-start; padding-top: 12vh; }
  .harbor-title { max-width: 94%; margin-top: 14px; font-size: 44px; line-height: 0.98; }
  .harbor-lead { max-width: 92%; margin-top: 18px; font-size: 15px; line-height: 1.5; }
  .harbor-subtitle { display: none; }
  .harbor-primary-action, .harbor-secondary-action { min-height: 43px; padding: 0 14px; }
  .harbor-signal-rail { height: 72px; }
  .harbor-signal { gap: 7px; padding: 0 8px; }
  .harbor-signal strong { font-size: 10px; }
  .harbor-signal small { font-size: 8px; }
  .harbor-signal-light { width: 5px; height: 5px; flex-basis: 5px; }
  .section-title, .harbor-cta-inner h2 { font-size: 30px; }
  .route-berth { padding: 24px 8px; }
  .manifest-heading { grid-template-columns: 1fr; gap: 12px; }
  .manifest-track { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .manifest-provider { min-height: 76px; padding: 10px; }
  .manifest-status { display: none; }
  .harbor-cta-inner { min-height: 300px; flex-direction: column; align-items: flex-start; justify-content: center; }
  .harbor-cta-button { width: 100%; }
}

@media (prefers-reduced-motion: reduce) {
  .harbor-home *, .harbor-home *::before, .harbor-home *::after { scroll-behavior: auto !important; transition-duration: 0.01ms !important; }
}
</style>
