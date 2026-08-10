<template>
  <AppLayout>
    <main class="w-full min-w-0 max-w-none space-y-5" data-test="instruction-audit-v2-view">
      <header class="flex min-w-0 flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div class="min-w-0">
          <div class="flex min-w-0 flex-wrap items-center gap-2.5">
            <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.title') }}</h1>
            <span v-if="config" class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold" :class="modePill(config.effective_mode)">
              <span class="h-1.5 w-1.5 rounded-full" :class="config.effective_mode === 'enforce' ? 'bg-primary-500' : config.effective_mode === 'observe' ? 'bg-amber-500' : 'bg-gray-400'" />
              {{ modeLabel(t, config.effective_mode) }}
            </span>
          </div>
          <p class="mt-1 max-w-4xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.description') }}</p>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <router-link :to="{ path: '/admin/settings', query: { tab: 'features' } }" class="btn btn-secondary">
            <Icon name="cog" size="sm" />{{ t('admin.instructionAudit.systemSettings') }}
          </router-link>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="refreshAll">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}
          </button>
        </div>
      </header>

      <div v-if="loadError" role="alert" class="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0" />
        <div class="min-w-0">
          <p class="font-semibold">{{ t('admin.instructionAudit.v2.loadFailed') }}</p>
          <p class="mt-0.5 break-words text-xs">{{ loadError }}</p>
        </div>
      </div>

      <template v-if="config">
        <div v-if="config.mode !== config.effective_mode || !config.evidence_encryption_ready" class="grid min-w-0 gap-2 lg:grid-cols-2">
          <div v-if="config.mode !== config.effective_mode" class="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
            <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0" />
            <span>{{ config.risk_control_enabled ? t('admin.instructionAudit.v2.modeNotEffective') : t('admin.instructionAudit.v2.riskControlDisabled') }}</span>
          </div>
          <div v-if="!config.evidence_encryption_ready" class="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
            <Icon name="lock" size="md" class="mt-0.5 shrink-0" />
            <span>{{ t('admin.instructionAudit.v2.encryptionUnavailable') }}</span>
          </div>
        </div>

        <section class="grid min-w-0 grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6" :aria-label="t('admin.instructionAudit.v2.statistics')">
          <article v-for="stat in statisticsCards" :key="stat.label" class="min-w-0 rounded-md border border-gray-200 bg-white px-3 py-3 shadow-sm dark:border-dark-600 dark:bg-dark-800">
            <div class="flex min-w-0 items-center justify-between gap-2">
              <p class="truncate text-[11px] font-semibold uppercase text-gray-400" :title="stat.label">{{ stat.label }}</p>
              <Icon :name="stat.icon" size="sm" :class="stat.iconClass" />
            </div>
            <p class="mt-2 truncate text-xl font-semibold tabular-nums text-gray-950 dark:text-white" :title="stat.value">{{ stat.value }}</p>
            <p class="mt-0.5 truncate text-[11px] text-gray-400" :title="stat.hint">{{ stat.hint }}</p>
          </article>
        </section>

        <nav class="flex min-w-0 flex-wrap gap-1 border-b border-gray-200 dark:border-dark-700" :aria-label="t('admin.instructionAudit.v2.workspaceTabs')">
          <button v-for="tab in tabs" :key="tab.value" type="button" class="relative flex min-w-0 items-center gap-2 px-3 py-3 text-sm font-medium transition sm:px-4" :class="activeTab === tab.value ? 'text-primary-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'" @click="setTab(tab.value)">
            <Icon :name="tab.icon" size="sm" />
            <span>{{ tab.label }}</span>
            <span v-if="tab.count != null" class="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] tabular-nums dark:bg-dark-700">{{ tab.count }}</span>
            <span v-if="activeTab === tab.value" class="absolute inset-x-2 bottom-0 h-0.5 rounded bg-primary-600" />
          </button>
        </nav>

        <InstructionV2EventsPanel
          v-if="activeTab === 'events'"
          :groups="groups"
          :clients="clients"
          :refresh-key="eventRefreshKey"
          @filters-change="loadStatistics"
          @trusted="handleTrustedChanged"
        />
        <InstructionV2TrustedPanel
          v-else-if="activeTab === 'hashes'"
          :scopes="scopes"
          :refresh-key="hashRefreshKey"
          @changed="handleTrustedChanged"
        />
        <InstructionV2RiskPanel
          v-else-if="activeTab === 'risk'"
          :refresh-key="riskRefreshKey"
          @changed="handlePolicyChanged"
        />
        <InstructionV2ReviewJobsPanel
          v-else-if="activeTab === 'reviews'"
          :refresh-key="reviewRefreshKey"
          @changed="handlePolicyChanged"
        />
        <InstructionV2ScopePanel
          v-else-if="activeTab === 'scopes'"
          :scopes="scopes"
          :groups="groups"
          :clients="clients"
          :allowlist="allowlist"
          @changed="reloadReferences"
        />
        <InstructionV2AISettingsPanel
          v-else
          :config="config"
          :nodes="aiNodes"
          @config-updated="handleConfigUpdated"
          @changed="reloadReferences"
        />
      </template>

      <div v-else-if="loading" class="space-y-4">
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6"><div v-for="index in 6" :key="index" class="h-24 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" /></div>
        <div class="h-14 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
        <div class="h-80 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import InstructionV2AISettingsPanel from './components/InstructionV2AISettingsPanel.vue'
import InstructionV2EventsPanel from './components/InstructionV2EventsPanel.vue'
import InstructionV2ReviewJobsPanel from './components/InstructionV2ReviewJobsPanel.vue'
import InstructionV2RiskPanel from './components/InstructionV2RiskPanel.vue'
import InstructionV2ScopePanel from './components/InstructionV2ScopePanel.vue'
import InstructionV2TrustedPanel from './components/InstructionV2TrustedPanel.vue'
import instructionAuditV2API from './v2Api'
import type {
  InstructionAINode,
  InstructionClientProfile,
  InstructionEventFilters,
  InstructionGroupOption,
  InstructionScope,
  InstructionStatistics,
  InstructionUserAllowlistEntry,
  InstructionV2Config,
} from './v2Types'
import { modeLabel, modePill } from './v2Presentation'

type Tab = 'events' | 'hashes' | 'risk' | 'reviews' | 'scopes' | 'ai'
type IconName = InstanceType<typeof Icon>['$props']['name']

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const activeTab = ref<Tab>(normalizeTab(String(route.query.tab || 'events')))
const loading = ref(true)
const loadError = ref('')
const config = ref<InstructionV2Config | null>(null)
const statistics = ref<InstructionStatistics | null>(null)
const groups = ref<InstructionGroupOption[]>([])
const scopes = ref<InstructionScope[]>([])
const clients = ref<InstructionClientProfile[]>([])
const allowlist = ref<InstructionUserAllowlistEntry[]>([])
const aiNodes = ref<InstructionAINode[]>([])
const eventRefreshKey = ref(0)
const hashRefreshKey = ref(0)
const riskRefreshKey = ref(0)
const reviewRefreshKey = ref(0)
let statisticsRequest = 0

const tabs = computed<Array<{ value: Tab; label: string; icon: IconName; count?: number }>>(() => [
  { value: 'events', label: t('admin.instructionAudit.v2.tabs.events'), icon: 'clipboard', count: statistics.value?.total ?? 0 },
  { value: 'hashes', label: t('admin.instructionAudit.v2.tabs.hashes'), icon: 'key', count: config.value?.active_hash_count ?? 0 },
  { value: 'risk', label: t('admin.instructionAudit.v2.tabs.risk'), icon: 'ban', count: config.value?.active_risk_hash_count ?? 0 },
  { value: 'reviews', label: t('admin.instructionAudit.v2.tabs.reviews'), icon: 'clock', count: config.value?.pending_review_job_count ?? 0 },
  { value: 'scopes', label: t('admin.instructionAudit.v2.tabs.scopes'), icon: 'shield', count: config.value?.active_scope_count ?? 0 },
  { value: 'ai', label: t('admin.instructionAudit.v2.tabs.ai'), icon: 'brain', count: config.value?.enabled_ai_node_count ?? 0 },
])
const statisticsCards = computed<Array<{ label: string; value: string; hint: string; icon: IconName; iconClass: string }>>(() => {
  const item = statistics.value
  return [
    { label: t('admin.instructionAudit.v2.stats.total'), value: String(item?.total ?? 0), hint: t('admin.instructionAudit.v2.stats.totalHint'), icon: 'chartBar', iconClass: 'text-gray-400' },
    { label: t('admin.instructionAudit.v2.stats.blocked'), value: String(item?.blocked ?? 0), hint: t('admin.instructionAudit.v2.stats.blockedHint'), icon: 'ban', iconClass: 'text-red-500' },
    { label: t('admin.instructionAudit.v2.stats.hashPass'), value: String(item?.hash_pass ?? 0), hint: t('admin.instructionAudit.v2.stats.hashPassHint'), icon: 'key', iconClass: 'text-primary-500' },
    { label: t('admin.instructionAudit.v2.stats.aiPass'), value: String(item?.ai_pass ?? 0), hint: t('admin.instructionAudit.v2.stats.aiPassHint'), icon: 'brain', iconClass: 'text-cyan-500' },
    { label: t('admin.instructionAudit.v2.stats.exceptionPass'), value: String(item?.empty_or_allowlist_pass ?? 0), hint: t('admin.instructionAudit.v2.stats.exceptionPassHint'), icon: 'users', iconClass: 'text-emerald-500' },
    { label: t('admin.instructionAudit.v2.stats.blockRate'), value: `${((item?.block_rate ?? 0) * 100).toFixed(1)}%`, hint: t('admin.instructionAudit.v2.stats.aiFailures', { count: item?.ai_failures ?? 0 }), icon: 'chart', iconClass: 'text-amber-500' },
  ]
})

onMounted(refreshAll)

async function refreshAll() {
  loading.value = true
  loadError.value = ''
  try {
    const [nextConfig, nextStatistics, nextGroups, nextScopes, nextClients, nextAllowlist, nextNodes] = await Promise.all([
      instructionAuditV2API.getConfig(),
      instructionAuditV2API.getStatistics(),
      instructionAuditV2API.listGroups(),
      instructionAuditV2API.listScopes(),
      instructionAuditV2API.listClientProfiles(),
      instructionAuditV2API.listUserAllowlist(),
      instructionAuditV2API.listAINodes(),
    ])
    config.value = nextConfig
    statistics.value = nextStatistics
    groups.value = nextGroups
    scopes.value = nextScopes
    clients.value = nextClients
    allowlist.value = nextAllowlist
    aiNodes.value = nextNodes
    eventRefreshKey.value += 1
    hashRefreshKey.value += 1
  } catch (caught) {
    loadError.value = extractApiErrorMessage(caught, t('common.error'))
  } finally {
    loading.value = false
  }
}

async function reloadReferences() {
  try {
    const [nextConfig, nextGroups, nextScopes, nextClients, nextAllowlist, nextNodes] = await Promise.all([
      instructionAuditV2API.getConfig(),
      instructionAuditV2API.listGroups(),
      instructionAuditV2API.listScopes(),
      instructionAuditV2API.listClientProfiles(),
      instructionAuditV2API.listUserAllowlist(),
      instructionAuditV2API.listAINodes(),
    ])
    config.value = nextConfig
    groups.value = nextGroups
    scopes.value = nextScopes
    clients.value = nextClients
    allowlist.value = nextAllowlist
    aiNodes.value = nextNodes
    eventRefreshKey.value += 1
    hashRefreshKey.value += 1
    riskRefreshKey.value += 1
    reviewRefreshKey.value += 1
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

async function loadStatistics(filters: InstructionEventFilters = {}) {
  const request = ++statisticsRequest
  try {
    const next = await instructionAuditV2API.getStatistics(filters)
    if (request === statisticsRequest) statistics.value = next
  } catch (caught) {
    if (request === statisticsRequest) appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function setTab(tab: Tab) {
  activeTab.value = tab
  router.replace({ query: { ...route.query, tab } })
}

function normalizeTab(value: string): Tab {
  if (value === 'hashes' || value === 'candidates') return 'hashes'
  if (value === 'risk') return 'risk'
  if (value === 'reviews' || value === 'jobs') return 'reviews'
  if (value === 'scopes' || value === 'rules' || value === 'config') return 'scopes'
  if (value === 'ai' || value === 'policies') return 'ai'
  return 'events'
}

function handleTrustedChanged() {
  hashRefreshKey.value += 1
  reloadReferences()
}

function handlePolicyChanged() {
  riskRefreshKey.value += 1
  reviewRefreshKey.value += 1
  hashRefreshKey.value += 1
  reloadReferences()
}

function handleConfigUpdated(next: InstructionV2Config) {
  config.value = next
  eventRefreshKey.value += 1
}
</script>
