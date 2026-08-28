<template>
  <div v-if="hasHomeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex w-full max-w-5xl items-center justify-between gap-3">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <template v-if="isModelPortBrand">
            <img :src="markSource" alt="" aria-hidden="true" class="h-9 w-9 object-contain" />
            <img :src="wordmarkSource" :alt="`${siteName} logo`" class="h-7 max-w-36 object-contain" />
          </template>
          <template v-else>
            <BrandLogo
              :site-name="siteName"
              :site-logo="siteLogo"
              variant="mark"
              image-class="h-9 w-9 object-contain"
            />
            <span class="truncate text-base font-semibold">{{ siteName }}</span>
          </template>
        </div>

        <div class="flex shrink-0 items-center gap-1 sm:gap-2">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex h-10 w-10 items-center justify-center rounded-lg text-gray-500 transition hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex h-10 w-10 items-center justify-center rounded-lg text-gray-500 transition hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white sm:w-auto sm:gap-1.5 sm:px-2.5"
            :title="t('nav.modelPlaza')"
            :aria-label="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="sm" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <LocaleSwitcher />
          <button
            type="button"
            class="inline-flex h-10 w-10 items-center justify-center rounded-lg text-gray-500 transition hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
        </div>
      </nav>
    </header>

    <main class="flex flex-1 items-center justify-center px-5 py-16 text-center sm:px-6">
      <div class="w-full max-w-2xl">
        <img
          v-if="isModelPortBrand"
          :src="markSource"
          alt=""
          aria-hidden="true"
          class="mx-auto mb-7 h-20 w-20 object-contain"
        />
        <BrandLogo
          v-else
          :site-name="siteName"
          :site-logo="siteLogo"
          variant="mark"
          image-class="mx-auto mb-7 h-20 w-20 object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-semibold sm:text-4xl">{{ siteName }}</h1>
        <p class="mx-auto mt-4 max-w-xl whitespace-pre-wrap [overflow-wrap:anywhere] text-base leading-7 text-gray-600 dark:text-dark-300">
          {{ siteSubtitle }}
        </p>
        <router-link
          :to="startPath"
          class="mt-8 inline-flex min-h-10 items-center justify-center gap-2 rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          <Icon name="arrowRight" size="xs" :stroke-width="2" />
        </router-link>
      </div>
    </main>

    <footer class="border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 dark:border-dark-800 dark:text-dark-400">
      {{ copyrightText }}
    </footer>
  </div>

  <div
    v-else
    class="home-shell"
    :class="{ 'is-dark': isDark }"
    :data-docs-configured="Boolean(docUrl)"
  >
    <header class="home-header" :class="{ 'is-scrolled': isScrolled }">
      <nav class="home-nav" :aria-label="t('home.primaryNavigation')">
        <router-link to="/home" class="brand-link" :aria-label="siteName">
          <template v-if="isModelPortBrand">
            <img :src="markSource" alt="" aria-hidden="true" class="nav-mark" />
            <img :src="wordmarkSource" :alt="`${siteName} logo`" class="nav-wordmark" />
          </template>
          <template v-else>
            <BrandLogo
              :site-name="siteName"
              :site-logo="siteLogo"
              variant="mark"
              image-class="h-8 w-8 object-contain"
            />
            <span class="brand-name">{{ siteName }}</span>
          </template>
        </router-link>

        <div class="nav-actions">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="nav-control docs-link"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <router-link v-if="showModelPlazaEntry" :to="modelPlazaPath" class="nav-control pricing-link">
            {{ t('nav.modelPlaza') }}
          </router-link>
          <div class="locale-control"><LocaleSwitcher /></div>
          <button
            class="nav-control theme-action"
            type="button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
          <router-link :to="startPath" class="nav-control nav-start">
            {{ t('home.getStarted') }}
            <Icon name="arrowRight" size="xs" :stroke-width="2" />
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section class="hero-section" aria-labelledby="home-title">
        <HarborScene
          :dark="isDark"
          :label="t('home.harborSceneLabel')"
          :providers="providers"
        />

        <div class="home-width hero-layout">
          <div class="hero-copy">
            <p class="hero-kicker">
              <span class="kicker-rule"></span>
              <span v-if="isModelPortBrand" class="chinese-wordmark" aria-label="模型港">
                <span>模型</span><span class="chinese-wordmark-accent">港</span>
              </span>
              <span v-if="isModelPortBrand" class="kicker-divider" aria-hidden="true"></span>
              <span class="kicker-label">{{ t('home.heroKicker') }}</span>
            </p>
            <h1 id="home-title" class="hero-title">
              <template v-if="isModelPortBrand">
                <img :src="wordmarkSource" alt="" aria-hidden="true" />
                <span class="sr-only">{{ siteName }}</span>
              </template>
              <span v-else>{{ siteName }}</span>
            </h1>
            <p class="hero-tagline">{{ heroTagline }}</p>
            <p class="hero-description">{{ heroDescription }}</p>
            <div class="hero-actions">
              <router-link :to="startPath" class="primary-action">
                {{ t('home.getStarted') }}
                <Icon name="arrowRight" size="sm" :stroke-width="2" />
              </router-link>
              <router-link v-if="showModelPlazaEntry" :to="modelPlazaPath" class="secondary-action">
                {{ t('nav.modelPlaza') }}
              </router-link>
              <a
                v-else-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="secondary-action"
              >
                {{ t('home.viewDocs') }}
              </a>
            </div>
          </div>
        </div>
      </section>

    </main>

    <footer class="home-footer">
      <div class="home-width footer-layout">
        <p>{{ copyrightText }}</p>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BrandLogo from '@/components/common/BrandLogo.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import HarborScene, { type HarborProvider } from '@/components/home/HarborScene.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { GroupPlatform } from '@/types'
import {
  DEFAULT_SITE_NAME,
  MODELPORT_BRAND,
  modelPortAsset,
  usesModelPortBrand,
} from '@/utils/branding'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { platformAccentColor, platformLabel } from '@/utils/platformColors'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const isDark = ref(document.documentElement.classList.contains('dark'))
const isScrolled = ref(false)

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || DEFAULT_SITE_NAME
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || t('home.heroSubtitle')
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(
  () => appStore.cachedPublicSettings?.compact_home_enabled === true
)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isModelPortBrand = computed(() => usesModelPortBrand(siteName.value, siteLogo.value))
const heroTagline = computed(() => isModelPortBrand.value ? 'One port, All Models.' : siteSubtitle.value)
const heroDescription = computed(() =>
  isModelPortBrand.value ? t('home.heroDescription') : t('home.heroSubtitle')
)
const homeMetaDescription = computed(() =>
  isModelPortBrand.value ? t('home.metaDescription') : siteSubtitle.value
)
const wordmarkSource = computed(() => modelPortAsset('wordmark', isDark.value))
const markSource = computed(() => modelPortAsset('mark', isDark.value))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value)
)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const startPath = computed(() => isAuthenticated.value ? dashboardPath.value : '/login')
const modelPlazaPath = computed(() => isAuthenticated.value
  ? { path: '/model-plaza', query: { embedded: '1' } }
  : '/model-plaza'
)
const currentYear = computed(() => new Date().getFullYear())
const copyrightText = computed(
  () => isModelPortBrand.value
    ? `© ${currentYear.value} ${MODELPORT_BRAND.siteName} ${t('home.footer.copyrightNotice')}`
    : `© ${currentYear.value} ${siteName.value} ${t('home.footer.copyrightNotice')}`
)

const modelVendorPlatforms: GroupPlatform[] = [
  'openai',
  'anthropic',
  'gemini',
  'antigravity',
  'deepseek',
  'kimi',
  'zhipu',
  'grok',
  'composite',
]

const providers = computed<HarborProvider[]>(() => modelVendorPlatforms.map((platform) => ({
  name: platformLabel(platform),
  platform,
  color: platformAccentColor(platform),
  darkIcon: platform === 'grok' || platform === 'kimi',
})))

let descriptionElement: HTMLMetaElement | null = null
let createdDescriptionElement = false
let previousDescriptionContent: string | null = null
let homeMetadataActive = false

function updateHomeMetaDescription() {
  if (!descriptionElement) {
    descriptionElement = document.head.querySelector<HTMLMetaElement>('meta[name="description"]')
    if (descriptionElement) {
      previousDescriptionContent = descriptionElement.getAttribute('content')
    } else {
      descriptionElement = document.createElement('meta')
      descriptionElement.name = 'description'
      descriptionElement.dataset.homeManaged = 'true'
      document.head.appendChild(descriptionElement)
      createdDescriptionElement = true
    }
  }

  descriptionElement.content = homeMetaDescription.value
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark'
    || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

function updateHeader() {
  isScrolled.value = window.scrollY > 20
}

watch(homeMetaDescription, () => {
  if (homeMetadataActive) updateHomeMetaDescription()
})

onMounted(async () => {
  homeMetadataActive = true
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
  if (!homeMetadataActive) return
  updateHomeMetaDescription()
  updateHeader()
  window.addEventListener('scroll', updateHeader, { passive: true })
})

onBeforeUnmount(() => {
  homeMetadataActive = false
  window.removeEventListener('scroll', updateHeader)
  if (createdDescriptionElement) {
    descriptionElement?.remove()
  } else if (descriptionElement) {
    if (previousDescriptionContent === null) {
      descriptionElement.removeAttribute('content')
    } else {
      descriptionElement.content = previousDescriptionContent
    }
  }
})
</script>

<style scoped>
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.home-shell {
  --paper: #f4f2e8;
  --paper-soft: #edf2ee;
  --sky: #dfe9e4;
  --ink: #132a2d;
  --muted: #5f706e;
  --line: #bcc9c3;
  --line-soft: rgba(19, 42, 45, 0.15);
  --mint: #0b8b82;
  --coral: #e7664a;
  --yellow: #eab84e;
  --blue: #0d6efd;
  min-height: 100vh;
  overflow-x: clip;
  color: var(--ink);
  background: var(--paper);
  font-family: Inter, "Noto Sans CJK SC", "Noto Sans SC", "Microsoft YaHei", ui-sans-serif, system-ui, sans-serif;
}

.home-shell.is-dark {
  --paper: #0a171a;
  --paper-soft: #0d1d20;
  --sky: #07181d;
  --ink: #edf1e9;
  --muted: #9eb0ad;
  --line: #31484a;
  --line-soft: rgba(232, 240, 235, 0.14);
  --mint: #45c2b5;
  --coral: #f1785c;
  --yellow: #f2c96b;
  --blue: #2f82ff;
}

.home-width {
  width: min(100% - 40px, 1280px);
  margin-inline: auto;
}

.home-header {
  position: fixed;
  z-index: 30;
  top: 0;
  right: 0;
  left: 0;
  border-bottom: 1px solid transparent;
  transition: background-color 180ms ease, border-color 180ms ease, backdrop-filter 180ms ease;
}

.home-header.is-scrolled {
  border-color: var(--line-soft);
  background: color-mix(in srgb, var(--sky) 88%, transparent);
  backdrop-filter: blur(18px) saturate(120%);
}

.home-nav {
  display: flex;
  width: min(100% - 40px, 1280px);
  min-height: 72px;
  margin-inline: auto;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.brand-link,
.nav-actions,
.nav-control,
.hero-kicker,
.hero-actions,
.primary-action,
.secondary-action {
  display: flex;
  align-items: center;
}

.brand-link {
  min-width: 188px;
  gap: 10px;
}

.nav-wordmark {
  width: 146px;
  height: auto;
}

.nav-mark {
  display: block;
  width: 28px;
  height: 32px;
  object-fit: contain;
}

.brand-name {
  margin-left: 10px;
  color: var(--ink);
  font-size: 17px;
  font-weight: 760;
}

.nav-actions {
  min-height: 40px;
  justify-content: flex-end;
  gap: 2px;
}

.nav-control {
  min-height: 36px;
  padding: 0 11px;
  justify-content: center;
  gap: 7px;
  border: 0;
  border-radius: 5px;
  color: var(--muted);
  background: transparent;
  font-size: 13px;
  font-weight: 690;
  line-height: 1;
  transition: color 160ms ease, background-color 160ms ease;
}

.nav-control:hover,
.nav-control:focus-visible,
.locale-control :deep(button:hover) {
  color: var(--blue);
  background: color-mix(in srgb, var(--sky) 62%, transparent);
}

.locale-control {
  display: flex;
  min-height: 36px;
  align-items: center;
}

.locale-control :deep(button) {
  min-height: 36px;
  color: var(--muted);
}

.locale-control :deep(button > span:first-child) {
  display: none;
}

.locale-control :deep(button > span:nth-child(2)) {
  display: inline;
}

.theme-action {
  width: 36px;
  padding: 0;
}

.nav-start {
  color: var(--ink);
  font-weight: 760;
}

.hero-section {
  position: relative;
  height: max(700px, calc(100svh - 56px));
  min-height: 700px;
  overflow: hidden;
  background: var(--sky);
}

.hero-layout {
  position: relative;
  z-index: 4;
  display: flex;
  height: 100%;
  padding-top: 72px;
  align-items: flex-start;
  pointer-events: none;
}

.hero-copy {
  width: min(540px, 48%);
  padding-top: clamp(72px, 9vh, 96px);
  pointer-events: auto;
}

.hero-kicker {
  width: fit-content;
  margin: 0;
  gap: 11px;
  color: var(--muted);
  line-height: 1;
}

.kicker-rule {
  width: 36px;
  height: 2px;
  background: var(--coral);
}

.chinese-wordmark {
  display: inline-flex;
  color: var(--ink);
  font-family: "Noto Sans CJK SC", "Noto Sans SC", "Microsoft YaHei", sans-serif;
  font-size: 25px;
  font-weight: 900;
  line-height: 1;
  transform: skewX(-4deg);
  transform-origin: left center;
}

.chinese-wordmark-accent {
  color: var(--blue);
}

.kicker-divider {
  width: 1px;
  height: 22px;
  background: var(--line);
}

.kicker-label {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  font-weight: 760;
  line-height: 1;
  text-transform: uppercase;
}

.hero-title {
  width: min(500px, 100%);
  min-height: 68px;
  margin: 24px 0 0;
  color: var(--ink);
  font-size: 64px;
  font-weight: 820;
  line-height: 1;
  letter-spacing: 0;
  overflow-wrap: anywhere;
}

.hero-title img {
  display: block;
  width: 100%;
  height: auto;
}

.hero-tagline {
  margin: 24px 0 0;
  color: var(--ink);
  font-size: 27px;
  font-weight: 650;
  line-height: 1.2;
  letter-spacing: 0;
}

.hero-description {
  max-width: 510px;
  margin: 16px 0 0;
  color: var(--muted);
  font-size: 15px;
  line-height: 1.72;
}

.hero-actions {
  margin-top: 27px;
  gap: 10px;
}

.primary-action,
.secondary-action {
  min-width: 122px;
  min-height: 46px;
  padding: 0 19px;
  justify-content: center;
  gap: 9px;
  border-radius: 6px;
  font-size: 15px;
  font-weight: 760;
  transition: transform 160ms ease, color 160ms ease, border-color 160ms ease, background-color 160ms ease;
}

.primary-action {
  border: 1px solid var(--coral);
  color: #fff;
  background: var(--coral);
  box-shadow: 0 13px 32px color-mix(in srgb, var(--coral) 24%, transparent);
}

.primary-action:hover,
.primary-action:focus-visible {
  border-color: #d9563d;
  background: #d9563d;
  transform: translateY(-2px);
}

.secondary-action {
  border: 1px solid color-mix(in srgb, var(--ink) 34%, transparent);
  color: var(--ink);
  background: color-mix(in srgb, var(--paper) 55%, transparent);
  backdrop-filter: blur(10px);
}

.secondary-action:hover,
.secondary-action:focus-visible {
  border-color: var(--mint);
  color: var(--mint);
  transform: translateY(-2px);
}

.home-footer {
  border-top: 1px solid var(--line-soft);
  background: var(--paper);
}

.footer-layout {
  display: flex;
  min-height: 56px;
  align-items: center;
  justify-content: center;
  color: var(--muted);
  font-size: 10px;
}

.footer-layout p {
  margin: 0;
}

@media (min-width: 1800px) {
  .home-width,
  .home-nav {
    width: min(100% - 64px, 1536px);
  }

  .home-nav {
    min-height: 86px;
  }

  .brand-link {
    min-width: 224px;
    gap: 12px;
  }

  .nav-mark {
    width: 34px;
    height: 39px;
  }

  .nav-wordmark {
    width: 176px;
  }

  .nav-actions {
    min-height: 46px;
  }

  .nav-control,
  .locale-control,
  .locale-control :deep(button) {
    min-height: 42px;
  }

  .nav-control {
    padding-inline: 14px;
    gap: 8px;
    font-size: 15px;
  }

  .theme-action {
    width: 42px;
    padding: 0;
  }

  .hero-section {
    height: max(780px, calc(100svh - 62px));
    min-height: 780px;
  }

  .hero-layout {
    padding-top: 86px;
  }

  .hero-copy {
    width: min(640px, 43%);
    padding-top: 96px;
  }

  .hero-kicker {
    gap: 13px;
  }

  .kicker-rule {
    width: 44px;
  }

  .chinese-wordmark {
    font-size: 30px;
  }

  .kicker-divider {
    height: 27px;
  }

  .kicker-label {
    font-size: 15px;
  }

  .hero-title {
    width: min(600px, 100%);
    min-height: 82px;
    margin-top: 28px;
  }

  .hero-tagline {
    margin-top: 27px;
    font-size: 32px;
  }

  .hero-description {
    max-width: 600px;
    margin-top: 18px;
    font-size: 18px;
    line-height: 1.68;
  }

  .hero-actions {
    margin-top: 32px;
    gap: 12px;
  }

  .primary-action,
  .secondary-action {
    min-width: 146px;
    min-height: 54px;
    padding-inline: 23px;
    font-size: 17px;
  }

  .footer-layout {
    min-height: 62px;
    font-size: 11px;
  }
}

@media (max-width: 900px) {
  .hero-copy {
    width: min(520px, 58%);
  }

  .hero-title {
    width: min(430px, 100%);
  }
}

@media (max-width: 640px) {
  .home-width,
  .home-nav {
    width: calc(100% - 28px);
  }

  .home-nav {
    min-height: 62px;
    gap: 10px;
  }

  .brand-link {
    min-width: 29px;
    gap: 0;
  }

  .nav-wordmark {
    display: none;
  }

  .nav-mark {
    display: block;
  }

  .brand-name,
  .pricing-link {
    display: none;
  }

  .nav-actions {
    gap: 0;
  }

  .nav-control,
  .locale-control :deep(button) {
    min-height: 34px;
    font-size: 11px;
  }

  .theme-action {
    width: 34px;
  }

  .nav-start {
    padding: 0 7px;
  }

  .hero-section {
    height: max(700px, calc(100svh - 52px));
    min-height: 700px;
  }

  .hero-layout {
    padding-top: 62px;
  }

  .hero-copy {
    width: 100%;
    padding-top: 56px;
  }

  .hero-kicker {
    gap: 8px;
  }

  .kicker-rule {
    width: 24px;
  }

  .chinese-wordmark {
    font-size: 20px;
  }

  .kicker-divider {
    height: 19px;
  }

  .kicker-label {
    font-size: 12px;
  }

  .hero-title {
    width: min(320px, 100%);
    min-height: 44px;
    margin-top: 19px;
    font-size: 44px;
  }

  .hero-tagline {
    margin-top: 18px;
    font-size: 22px;
  }

  .hero-description {
    max-width: 360px;
    margin-top: 12px;
    font-size: 13px;
    line-height: 1.65;
  }

  .hero-actions {
    display: grid;
    margin-top: 20px;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .primary-action,
  .secondary-action {
    min-width: 0;
    min-height: 44px;
    padding: 0 12px;
  }

  .footer-layout {
    min-height: 52px;
  }
}

@media (max-width: 380px) {
  .locale-control {
    display: none;
  }

  .hero-title {
    width: min(292px, 100%);
  }

  .hero-tagline {
    font-size: 20px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-header,
  .nav-control,
  .primary-action,
  .secondary-action {
    transition: none;
  }

}
</style>
