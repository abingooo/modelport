<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- homeContent is an administrator-controlled setting. -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="home-shell">
    <section class="hero-section">
      <img
        src="/landing/modelport-harbor.jpg"
        :alt="t('home.heroImageAlt')"
        class="hero-image"
      />
      <div class="hero-overlay"></div>
      <div class="route-signal route-signal-one" aria-hidden="true"><span></span></div>
      <div class="route-signal route-signal-two" aria-hidden="true"><span></span></div>

      <header class="hero-header">
        <nav class="hero-nav" :aria-label="t('home.primaryNavigation')">
          <router-link to="/home" class="brand-link" :aria-label="siteName">
            <img
              v-if="isModelPortBrand"
              src="/branding/modelport-wordmark-dark.png"
              :alt="`${siteName} logo`"
              class="hero-wordmark"
            />
            <BrandLogo
              v-else
              :site-name="siteName"
              :site-logo="siteLogo"
              variant="mark"
              image-class="h-9 w-9 object-contain"
            />
            <span v-if="!isModelPortBrand" class="brand-name">{{ siteName }}</span>
          </router-link>

          <div class="desktop-navigation">
            <router-link to="/model-pricing" class="nav-link">
              {{ t('home.modelPricing') }}
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="nav-link"
            >
              {{ t('home.docs') }}
            </a>
            <a
              :href="githubUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="nav-link"
            >
              GitHub
            </a>
          </div>

          <div class="nav-actions">
            <div class="hero-locale"><LocaleSwitcher /></div>
            <button
              class="icon-action"
              type="button"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              @click="toggleTheme"
            >
              <Icon v-if="isDark" name="sun" size="sm" />
              <Icon v-else name="moon" size="sm" />
            </button>
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="header-action"
            >
              <span v-if="isAuthenticated" class="user-initial">{{ userInitial }}</span>
              {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
              <Icon name="arrowRight" size="xs" :stroke-width="2" />
            </router-link>
          </div>
        </nav>
      </header>

      <div class="hero-content">
        <p class="hero-kicker">{{ t('home.heroKicker') }}</p>
        <h1>{{ siteName }}</h1>
        <p class="hero-tagline">{{ heroTagline }}</p>
        <p class="hero-description">{{ t('home.heroDescription') }}</p>
        <div class="hero-actions">
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="primary-action"
          >
            {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
            <Icon name="arrowRight" size="sm" :stroke-width="2" />
          </router-link>
          <router-link to="/model-pricing" class="secondary-action">
            {{ t('home.viewModelPricing') }}
          </router-link>
        </div>
      </div>

      <div class="hero-manifest" aria-hidden="true">
        <span>OPENAI</span>
        <span>ANTHROPIC</span>
        <span>GOOGLE</span>
        <span class="manifest-status"><i></i>{{ t('home.gatewayOnline') }}</span>
      </div>
    </section>

    <main>
      <section class="fleet-section">
        <div class="section-inner">
          <div class="section-heading">
            <div>
              <p class="section-kicker">{{ t('home.fleet.kicker') }}</p>
              <h2>{{ t('home.fleet.title') }}</h2>
            </div>
            <p>{{ t('home.fleet.description') }}</p>
          </div>

          <div class="provider-route" role="list" :aria-label="t('home.fleet.routeLabel')">
            <div
              v-for="provider in providers"
              :key="provider.platform"
              class="provider-stop"
              role="listitem"
            >
              <span class="provider-icon" :class="`provider-${provider.platform}`">
                <PlatformIcon :platform="provider.platform" size="lg" />
              </span>
              <span>{{ provider.name }}</span>
            </div>
          </div>

          <div class="gateway-destination">
            <span class="destination-line"></span>
            <span class="destination-mark">
              <BrandLogo
                :site-name="siteName"
                :site-logo="siteLogo"
                variant="mark"
                image-class="h-8 w-8 object-contain"
              />
            </span>
            <span class="destination-copy">
              <strong>{{ siteName }}</strong>
              <small>{{ t('home.fleet.destination') }}</small>
            </span>
          </div>
        </div>
      </section>

      <section class="protocol-section">
        <div class="protocol-inner">
          <div class="protocol-copy">
            <p class="section-kicker">{{ t('home.protocols.kicker') }}</p>
            <h2>{{ t('home.protocols.title') }}</h2>
            <p>{{ t('home.protocols.description') }}</p>
            <div class="protocol-points">
              <span><Icon name="check" size="sm" />{{ t('home.protocols.oneKey') }}</span>
              <span><Icon name="check" size="sm" />{{ t('home.protocols.nativeClients') }}</span>
              <span><Icon name="check" size="sm" />{{ t('home.protocols.clearBilling') }}</span>
            </div>
          </div>

          <div class="request-console">
            <div class="console-toolbar">
              <div class="protocol-tabs" role="tablist" :aria-label="t('home.protocols.tabLabel')">
                <button
                  v-for="protocol in protocols"
                  :key="protocol.id"
                  type="button"
                  role="tab"
                  :aria-selected="activeProtocol === protocol.id"
                  :class="{ active: activeProtocol === protocol.id }"
                  @click="activeProtocol = protocol.id"
                >
                  {{ protocol.label }}
                </button>
              </div>
              <button
                type="button"
                class="copy-action"
                :title="copied ? t('home.protocols.copied') : t('home.protocols.copy')"
                :aria-label="copied ? t('home.protocols.copied') : t('home.protocols.copy')"
                @click="copyRequest"
              >
                <Icon :name="copied ? 'check' : 'copy'" size="sm" />
              </button>
            </div>
            <pre><code>{{ activeRequest }}</code></pre>
            <div class="console-status">
              <span><i></i>{{ t('home.protocols.requestReady') }}</span>
              <span>{{ activeProtocolConfig.endpoint }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="closing-section">
        <div class="closing-inner">
          <div>
            <p class="section-kicker">{{ t('home.closing.kicker') }}</p>
            <h2>{{ t('home.closing.title') }}</h2>
          </div>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="primary-action"
          >
            {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
            <Icon name="arrowRight" size="sm" :stroke-width="2" />
          </router-link>
        </div>
      </section>
    </main>

    <footer class="home-footer">
      <div class="footer-inner">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div>
          <router-link to="/model-pricing">{{ t('home.modelPricing') }}</router-link>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
            {{ t('home.docs') }}
          </a>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BrandLogo from '@/components/common/BrandLogo.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { GroupPlatform } from '@/types'
import { sanitizeUrl } from '@/utils/url'

type ProtocolId = 'openai' | 'anthropic' | 'google'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform'
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isModelPortBrand = computed(() => siteName.value.trim().toLowerCase() === 'modelport')
const heroTagline = computed(() =>
  isModelPortBrand.value ? 'one port，all model' : siteSubtitle.value
)
const githubUrl = computed(() =>
  isModelPortBrand.value
    ? 'https://github.com/abingooo/modelport'
    : 'https://github.com/Wei-Shaw/sub2api'
)

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => authStore.user?.email?.charAt(0).toUpperCase() || '')
const currentYear = computed(() => new Date().getFullYear())

const providers: Array<{ name: string; platform: GroupPlatform }> = [
  { name: 'OpenAI', platform: 'openai' },
  { name: 'Claude', platform: 'anthropic' },
  { name: 'Gemini', platform: 'gemini' },
  { name: 'DeepSeek', platform: 'deepseek' },
  { name: 'Qwen', platform: 'qwen' },
  { name: 'Kimi', platform: 'kimi' },
  { name: 'GLM', platform: 'glm' },
  { name: 'ByteDance', platform: 'doubao' },
]

const protocols: Array<{ id: ProtocolId; label: string }> = [
  { id: 'openai', label: 'OpenAI' },
  { id: 'anthropic', label: 'Anthropic' },
  { id: 'google', label: 'Google' },
]
const protocolConfigs: Record<ProtocolId, { endpoint: string; request: (origin: string) => string }> = {
  openai: {
    endpoint: '/v1/chat/completions',
    request: (origin) => `curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer $MODELPORT_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"Hello"}]}'`,
  },
  anthropic: {
    endpoint: '/v1/messages',
    request: (origin) => `curl ${origin}/v1/messages \\
  -H "x-api-key: $MODELPORT_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"claude-sonnet-4-5","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'`,
  },
  google: {
    endpoint: '/v1beta/models/{model}:generateContent',
    request: (origin) => `curl ${origin}/v1beta/models/gemini-2.5-pro:generateContent \\
  -H "x-goog-api-key: $MODELPORT_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"contents":[{"parts":[{"text":"Hello"}]}]}'`,
  },
}

const activeProtocol = ref<ProtocolId>('openai')
const copied = ref(false)
const activeProtocolConfig = computed(() => protocolConfigs[activeProtocol.value])
const activeRequest = computed(() => activeProtocolConfig.value.request(window.location.origin))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

async function copyRequest() {
  try {
    await navigator.clipboard.writeText(activeRequest.value)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1600)
  } catch {
    copied.value = false
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.home-shell {
  min-height: 100vh;
  overflow-x: hidden;
  background: #f7f9fc;
  color: #10223d;
}

.hero-section {
  position: relative;
  min-height: calc(100svh - 160px);
  overflow: hidden;
  background: #07162b;
  color: #fff;
}

.hero-image,
.hero-overlay {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.hero-image {
  object-fit: cover;
  object-position: center 54%;
  transform: scale(1.02);
}

.hero-overlay {
  background:
    linear-gradient(90deg, rgba(3, 13, 28, 0.95) 0%, rgba(3, 14, 30, 0.82) 42%, rgba(3, 15, 32, 0.3) 76%, rgba(3, 15, 32, 0.48) 100%),
    linear-gradient(0deg, rgba(2, 10, 23, 0.84) 0%, transparent 38%, rgba(2, 10, 23, 0.22) 100%);
}

.hero-header {
  position: relative;
  z-index: 4;
  border-bottom: 1px solid rgba(255, 255, 255, 0.14);
}

.hero-nav {
  display: flex;
  min-height: 72px;
  max-width: 1240px;
  margin: 0 auto;
  padding: 0 28px;
  align-items: center;
  justify-content: space-between;
  gap: 28px;
}

.brand-link,
.nav-actions,
.desktop-navigation,
.header-action,
.hero-actions,
.protocol-points span,
.console-status,
.footer-inner,
.footer-inner div {
  display: flex;
  align-items: center;
}

.brand-link {
  min-width: 168px;
  gap: 10px;
}

.brand-name {
  color: #fff;
  font-size: 17px;
  font-weight: 700;
}

.hero-wordmark {
  width: auto;
  height: 32px;
}

.desktop-navigation {
  margin-left: auto;
  gap: 30px;
}

.nav-link {
  color: rgba(255, 255, 255, 0.72);
  font-size: 14px;
  transition: color 160ms ease;
}

.nav-link:hover {
  color: #fff;
}

.nav-actions {
  gap: 8px;
  flex-shrink: 0;
}

.icon-action,
.copy-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.icon-action {
  width: 34px;
  height: 34px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 8px;
  color: rgba(255, 255, 255, 0.82);
  background: rgba(6, 20, 42, 0.3);
}

.icon-action:hover {
  border-color: rgba(255, 255, 255, 0.42);
  color: #fff;
}

.header-action {
  min-height: 36px;
  padding: 0 13px;
  gap: 7px;
  border-radius: 8px;
  color: #07162b;
  background: #fff;
  font-size: 13px;
  font-weight: 700;
}

.user-initial {
  display: inline-flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #0d6ef2;
  font-size: 10px;
}

.hero-locale :deep(button) {
  color: rgba(255, 255, 255, 0.78);
}

.hero-locale :deep(button:hover) {
  color: #fff;
  background: rgba(255, 255, 255, 0.1);
}

.hero-content {
  position: relative;
  z-index: 3;
  max-width: 1240px;
  margin: 0 auto;
  padding: 16vh 28px 160px;
}

.hero-kicker,
.section-kicker {
  margin: 0 0 18px;
  color: #63a2ff;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.hero-content h1 {
  max-width: 760px;
  margin: 0;
  color: #fff;
  font-size: 72px;
  font-weight: 750;
  line-height: 1.04;
  letter-spacing: 0;
}

.hero-tagline {
  margin: 14px 0 0;
  color: #fff;
  font-size: 28px;
  font-weight: 500;
  line-height: 1.25;
}

.hero-description {
  max-width: 530px;
  margin: 24px 0 0;
  color: rgba(230, 239, 250, 0.78);
  font-size: 16px;
  line-height: 1.75;
}

.hero-actions {
  margin-top: 34px;
  flex-wrap: wrap;
  gap: 12px;
}

.primary-action,
.secondary-action {
  display: inline-flex;
  min-height: 46px;
  padding: 0 20px;
  align-items: center;
  justify-content: center;
  gap: 9px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 750;
  transition: transform 160ms ease, background 160ms ease, border-color 160ms ease;
}

.primary-action {
  color: #fff;
  background: #0d6ef2;
  box-shadow: 0 10px 30px rgba(13, 110, 242, 0.26);
}

.primary-action:hover,
.secondary-action:hover {
  transform: translateY(-1px);
}

.primary-action:hover {
  background: #075bd8;
}

.secondary-action {
  border: 1px solid rgba(255, 255, 255, 0.28);
  color: #fff;
  background: rgba(4, 16, 34, 0.36);
  backdrop-filter: blur(10px);
}

.secondary-action:hover {
  border-color: rgba(255, 255, 255, 0.62);
  background: rgba(4, 16, 34, 0.56);
}

.hero-manifest {
  position: absolute;
  z-index: 3;
  right: max(28px, calc((100vw - 1184px) / 2));
  bottom: 28px;
  display: flex;
  align-items: center;
  gap: 20px;
  color: rgba(255, 255, 255, 0.58);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10px;
}

.manifest-status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: rgba(255, 255, 255, 0.8);
}

.manifest-status i,
.console-status i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #31c48d;
  box-shadow: 0 0 0 4px rgba(49, 196, 141, 0.14);
}

.route-signal {
  position: absolute;
  z-index: 2;
  height: 1px;
  overflow: visible;
  background: rgba(105, 172, 255, 0.38);
  transform-origin: left center;
}

.route-signal span {
  position: absolute;
  top: -3px;
  left: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #79b5ff;
  box-shadow: 0 0 14px rgba(121, 181, 255, 0.9);
  animation: route-pulse 5.5s linear infinite;
}

.route-signal-one {
  right: 2%;
  bottom: 34%;
  width: 38%;
  transform: rotate(-14deg);
}

.route-signal-two {
  right: 7%;
  bottom: 23%;
  width: 29%;
  transform: rotate(9deg);
}

.route-signal-two span {
  animation-delay: -2.8s;
}

@keyframes route-pulse {
  from { left: 0; opacity: 0; }
  8% { opacity: 1; }
  92% { opacity: 1; }
  to { left: 100%; opacity: 0; }
}

.fleet-section,
.protocol-section,
.closing-section {
  position: relative;
}

.fleet-section {
  padding: 88px 28px 96px;
  background: #f7f9fc;
}

.section-inner,
.protocol-inner,
.closing-inner,
.footer-inner {
  max-width: 1184px;
  margin: 0 auto;
}

.section-heading {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 430px);
  gap: 56px;
  align-items: end;
}

.section-heading h2,
.protocol-copy h2,
.closing-inner h2 {
  margin: 0;
  color: #10223d;
  font-size: 36px;
  font-weight: 750;
  line-height: 1.18;
  letter-spacing: 0;
}

.section-heading > p,
.protocol-copy > p {
  margin: 0;
  color: #65748a;
  font-size: 15px;
  line-height: 1.75;
}

.provider-route {
  position: relative;
  display: grid;
  grid-template-columns: repeat(8, minmax(90px, 1fr));
  margin-top: 64px;
}

.provider-route::before {
  position: absolute;
  top: 23px;
  right: 5%;
  left: 5%;
  height: 1px;
  background: #cbd6e5;
  content: '';
}

.provider-stop {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: #526177;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.provider-icon {
  display: flex;
  width: 46px;
  height: 46px;
  align-items: center;
  justify-content: center;
  border: 1px solid #d7e0eb;
  border-radius: 50%;
  color: #15243a;
  background: #fff;
  box-shadow: 0 6px 18px rgba(26, 50, 82, 0.08);
}

.provider-openai { color: #111827; }
.provider-anthropic { color: #c5673e; }
.provider-gemini { color: #4285f4; }
.provider-deepseek { color: #4d6bfe; }
.provider-qwen { color: #6b55dd; }
.provider-kimi { color: #111827; }
.provider-glm { color: #246bfd; }
.provider-doubao { color: #325dff; }

.gateway-destination {
  display: flex;
  max-width: 420px;
  margin: 54px auto 0;
  align-items: center;
  justify-content: center;
  gap: 13px;
}

.destination-line {
  width: 72px;
  height: 1px;
  background: #0d6ef2;
}

.destination-mark {
  display: flex;
  width: 48px;
  height: 48px;
  align-items: center;
  justify-content: center;
  border: 1px solid #b9d2f8;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 8px 24px rgba(13, 110, 242, 0.14);
}

.destination-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.destination-copy strong {
  color: #10223d;
  font-size: 14px;
}

.destination-copy small {
  color: #7d899b;
  font-size: 11px;
}

.protocol-section {
  padding: 104px 28px;
  color: #fff;
  background: #07162b;
}

.protocol-inner {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(540px, 1.2fr);
  gap: 88px;
  align-items: center;
}

.protocol-copy h2 {
  color: #fff;
}

.protocol-copy > p {
  max-width: 460px;
  margin-top: 22px;
  color: #9caec4;
}

.protocol-points {
  display: flex;
  margin-top: 30px;
  flex-direction: column;
  gap: 12px;
}

.protocol-points span {
  gap: 9px;
  color: #d8e3f0;
  font-size: 13px;
}

.protocol-points svg {
  color: #59a0ff;
}

.request-console {
  overflow: hidden;
  border: 1px solid #263c59;
  border-radius: 8px;
  background: #041020;
  box-shadow: 0 22px 60px rgba(0, 0, 0, 0.28);
}

.console-toolbar {
  display: flex;
  min-height: 52px;
  padding: 8px 10px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid #1d3049;
}

.protocol-tabs {
  display: inline-flex;
  padding: 3px;
  border-radius: 7px;
  background: #0c1d33;
}

.protocol-tabs button {
  min-height: 30px;
  padding: 0 12px;
  border-radius: 5px;
  color: #7f91a8;
  font-size: 11px;
  font-weight: 700;
}

.protocol-tabs button.active {
  color: #fff;
  background: #19314f;
}

.copy-action {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  color: #8fa2b9;
}

.copy-action:hover {
  color: #fff;
  background: #122640;
}

.request-console pre {
  min-height: 250px;
  margin: 0;
  padding: 25px 24px;
  overflow-x: auto;
  color: #c9d8ea;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.9;
  white-space: pre;
}

.console-status {
  min-height: 42px;
  padding: 0 18px;
  justify-content: space-between;
  gap: 20px;
  border-top: 1px solid #1d3049;
  color: #6f8299;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10px;
}

.console-status span:first-child {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #9db0c7;
}

.closing-section {
  padding: 72px 28px;
  background: #eaf1fa;
}

.closing-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 40px;
}

.closing-inner .section-kicker {
  margin-bottom: 10px;
}

.home-footer {
  padding: 26px 28px;
  border-top: 1px solid #dce5f0;
  color: #748196;
  background: #f7f9fc;
  font-size: 12px;
}

.footer-inner {
  justify-content: space-between;
  gap: 28px;
}

.footer-inner div {
  gap: 20px;
}

.footer-inner a:hover {
  color: #0d6ef2;
}

:global(.dark) .home-shell,
:global(.dark) .fleet-section,
:global(.dark) .home-footer {
  color: #d8e3f0;
  background: #07111f;
}

:global(.dark) .section-heading h2,
:global(.dark) .destination-copy strong {
  color: #f2f6fb;
}

:global(.dark) .section-heading > p,
:global(.dark) .provider-stop,
:global(.dark) .destination-copy small,
:global(.dark) .home-footer {
  color: #8fa0b5;
}

:global(.dark) .provider-route::before {
  background: #26384f;
}

:global(.dark) .provider-icon,
:global(.dark) .destination-mark {
  border-color: #2b405a;
  background: #0e1d30;
}

:global(.dark) .provider-openai,
:global(.dark) .provider-kimi {
  color: #f2f6fb;
}

:global(.dark) .closing-section {
  background: #0d1b2d;
}

:global(.dark) .closing-inner h2 {
  color: #f2f6fb;
}

:global(.dark) .home-footer {
  border-color: #1a2c42;
}

@media (max-width: 900px) {
  .desktop-navigation {
    display: none;
  }

  .hero-content {
    padding-top: 13vh;
  }

  .hero-content h1 {
    font-size: 60px;
  }

  .section-heading,
  .protocol-inner {
    grid-template-columns: 1fr;
  }

  .section-heading {
    gap: 18px;
  }

  .protocol-inner {
    gap: 52px;
  }

  .provider-route {
    grid-template-columns: repeat(8, 112px);
    margin-right: -28px;
    margin-left: -28px;
    padding: 0 28px 12px;
    overflow-x: auto;
  }

  .provider-route::before {
    right: 56px;
    left: 56px;
  }
}

@media (max-width: 640px) {
  .hero-section {
    min-height: calc(100svh - 100px);
  }

  .hero-image {
    object-position: 58% center;
  }

  .hero-overlay {
    background: linear-gradient(90deg, rgba(3, 13, 28, 0.94), rgba(3, 14, 30, 0.58));
  }

  .hero-nav {
    min-height: 64px;
    padding: 0 18px;
    gap: 12px;
  }

  .brand-link {
    min-width: 0;
  }

  .brand-link :deep(img) {
    max-width: 132px;
    height: 25px;
  }

  .hero-wordmark {
    max-width: 132px;
    height: 25px;
  }

  .nav-actions {
    position: absolute;
    right: 18px;
    display: flex;
  }

  .hero-locale,
  .header-action svg,
  .user-initial {
    display: none;
  }

  .header-action {
    min-height: 34px;
    padding: 0 11px;
  }

  .hero-content {
    padding: 15vh 18px 118px;
  }

  .hero-content h1 {
    font-size: 46px;
    line-height: 1.08;
  }

  .hero-tagline {
    margin-top: 10px;
    font-size: 22px;
  }

  .hero-description {
    max-width: 330px;
    margin-top: 20px;
    font-size: 14px;
    overflow-wrap: anywhere;
  }

  .hero-actions {
    margin-top: 28px;
  }

  .primary-action,
  .secondary-action {
    min-height: 44px;
    padding: 0 16px;
  }

  .hero-manifest {
    right: 18px;
    bottom: 20px;
    left: 18px;
    justify-content: space-between;
    gap: 10px;
  }

  .hero-manifest > span:not(.manifest-status) {
    display: none;
  }

  .route-signal-one {
    width: 54%;
  }

  .route-signal-two {
    display: none;
  }

  .fleet-section,
  .protocol-section {
    padding: 72px 18px;
  }

  .section-heading h2,
  .protocol-copy h2,
  .closing-inner h2 {
    font-size: 29px;
  }

  .provider-route {
    margin-top: 46px;
    margin-right: -18px;
    margin-left: -18px;
    padding-right: 18px;
    padding-left: 18px;
  }

  .gateway-destination {
    justify-content: flex-start;
  }

  .destination-line {
    width: 36px;
  }

  .protocol-inner {
    gap: 38px;
  }

  .console-toolbar {
    align-items: flex-start;
  }

  .protocol-tabs {
    overflow-x: auto;
  }

  .protocol-tabs button {
    padding: 0 9px;
  }

  .request-console pre {
    min-height: 286px;
    padding: 20px 18px;
    font-size: 11px;
  }

  .console-status span:last-child {
    display: none;
  }

  .closing-section {
    padding: 60px 18px;
  }

  .closing-inner,
  .footer-inner {
    align-items: flex-start;
    flex-direction: column;
  }

  .home-footer {
    padding: 24px 18px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .route-signal span {
    animation: none;
  }

  .primary-action,
  .secondary-action {
    transition: none;
  }
}
</style>
