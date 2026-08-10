<template>
  <section class="min-w-0 space-y-5" data-test="instruction-v2-scopes">
    <div class="inline-flex max-w-full flex-wrap rounded-md bg-gray-100 p-1 dark:bg-dark-700">
      <button v-for="tab in tabs" :key="tab.value" type="button" class="rounded px-3 py-2 text-sm font-medium transition" :class="activeSection === tab.value ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-300 dark:hover:text-white'" @click="activeSection = tab.value">
        {{ tab.label }}
        <span class="ml-1 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] tabular-nums dark:bg-dark-700">{{ tab.count }}</span>
      </button>
    </div>

    <template v-if="activeSection === 'scopes'">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.auditScopes') }}</h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.auditScopesHint') }}</p>
        </div>
        <button type="button" class="btn btn-primary shrink-0" :disabled="!groups.length" @click="openScopeForm()"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.v2.addScope') }}</button>
      </div>
      <div v-if="scopes.length" class="resource-grid">
        <article v-for="scope in scopes" :key="scope.id" class="resource-card">
          <header class="flex min-w-0 items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ scope.group_name }}</h3>
                <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="scope.effective ? 'bg-primary-100 text-primary-700 dark:bg-primary-950/60 dark:text-primary-200' : 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'">{{ scope.effective ? t('admin.instructionAudit.v2.effective') : t('admin.instructionAudit.v2.ineffective') }}</span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ scope.group_id }} · {{ scope.group_platform || '-' }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <button type="button" class="icon-btn" :title="t('common.edit')" @click="openScopeForm(scope)"><Icon name="edit" size="sm" /></button>
              <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="scopeToDelete = scope"><Icon name="trash" size="sm" /></button>
            </div>
          </header>
          <div class="rounded-md bg-gray-50 px-3 py-3 dark:bg-dark-800/70">
            <p class="resource-label">{{ t('admin.instructionAudit.v2.clientScope') }}</p>
            <div class="mt-1 flex items-center gap-2">
              <Icon :name="scope.client_profile_id ? 'terminal' : 'globe'" size="sm" class="text-primary-500" />
              <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ scope.client_profile_name || t('admin.instructionAudit.v2.allClients') }}</span>
            </div>
            <p v-if="scope.client_profile_key" class="mt-1 font-mono text-[11px] text-gray-400">{{ scope.client_profile_key }}</p>
          </div>
          <footer class="mt-auto flex items-center justify-between gap-3 border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.scopeStatus') }}</span>
            <span :class="scope.enabled ? 'text-primary-700 dark:text-primary-300' : 'text-gray-400'">{{ scope.enabled ? t('common.enabled') : t('common.disabled') }}</span>
          </footer>
        </article>
      </div>
      <EmptyPanel v-else icon="shield" :title="t('admin.instructionAudit.v2.noScopes')" :description="t('admin.instructionAudit.v2.noScopesHint')" />
    </template>

    <template v-else-if="activeSection === 'clients'">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.clientProfiles') }}</h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.clientProfilesHint') }}</p>
        </div>
        <button type="button" class="btn btn-primary shrink-0" @click="openClientForm()"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.v2.addClientProfile') }}</button>
      </div>
      <div v-if="clients.length" class="resource-grid">
        <article v-for="client in clients" :key="client.id" class="resource-card">
          <header class="flex min-w-0 items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ client.name }}</h3>
                <span v-if="client.built_in" class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ t('admin.instructionAudit.v2.builtIn') }}</span>
              </div>
              <p class="mt-1 font-mono text-xs text-primary-600 dark:text-primary-400">{{ client.profile_key }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <button type="button" class="icon-btn" :title="t('common.edit')" @click="openClientForm(client)"><Icon name="edit" size="sm" /></button>
              <button v-if="!client.built_in" type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="clientToDelete = client"><Icon name="trash" size="sm" /></button>
            </div>
          </header>
          <p class="min-h-10 break-words text-xs text-gray-500 dark:text-gray-400">{{ client.description || '-' }}</p>
          <div class="space-y-2">
            <p class="resource-label">{{ t('admin.instructionAudit.v2.userAgentMatchers') }}</p>
            <div v-if="client.matchers.length" class="space-y-1.5">
              <div v-for="(matcher, index) in client.matchers" :key="`${client.id}-${index}`" class="flex min-w-0 items-center gap-2 rounded bg-gray-50 px-2 py-1.5 dark:bg-dark-800/70">
                <span class="shrink-0 rounded bg-primary-50 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">{{ matcher.type }}</span>
                <code class="min-w-0 flex-1 break-all text-[11px] text-gray-700 dark:text-gray-200">{{ matcher.value }}</code>
              </div>
            </div>
            <p v-else class="text-xs text-gray-400">{{ client.immutable_internal ? t('admin.instructionAudit.v2.internalIdentityOnly') : t('admin.instructionAudit.v2.fallbackClient') }}</p>
          </div>
          <footer class="mt-auto flex items-center justify-between gap-3 border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.priority') }} {{ client.priority }}</span>
            <span :class="client.enabled ? 'text-primary-700 dark:text-primary-300' : 'text-gray-400'">{{ client.enabled ? t('common.enabled') : t('common.disabled') }}</span>
          </footer>
        </article>
      </div>
      <EmptyPanel v-else icon="terminal" :title="t('admin.instructionAudit.v2.noClientProfiles')" :description="t('admin.instructionAudit.v2.noClientProfilesHint')" />
    </template>

    <template v-else>
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.userAllowlist') }}</h2>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.userAllowlistHint') }}</p>
      </div>

      <div class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/50 lg:grid-cols-[minmax(260px,1fr)_minmax(220px,1fr)_auto] lg:items-end">
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.v2.searchUser') }}</span>
          <div class="flex min-w-0">
            <input v-model.trim="userSearch" class="input min-w-0 rounded-r-none" :placeholder="t('admin.instructionAudit.v2.searchUserHint')" @keyup.enter="searchUsers" />
            <button type="button" class="btn btn-secondary rounded-l-none px-3" :disabled="userSearching" @click="searchUsers"><Icon name="search" size="sm" /></button>
          </div>
          <select v-if="userResults.length" v-model="selectedUserId" class="input mt-2">
            <option :value="0">{{ t('admin.instructionAudit.v2.selectUser') }}</option>
            <option v-for="user in userResults" :key="user.id" :value="user.id">{{ user.email || user.username }} (#{{ user.id }})</option>
          </select>
        </label>
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.v2.note') }}</span>
          <input v-model="allowlistNote" maxlength="500" class="input" />
        </label>
        <button type="button" class="btn btn-primary" :disabled="!selectedUserId || saving" @click="addAllowlist"><Icon name="userPlus" size="sm" />{{ t('admin.instructionAudit.v2.addAllowlist') }}</button>
      </div>

      <div v-if="allowlist.length" class="resource-grid">
        <article v-for="entry in allowlist" :key="entry.id" class="resource-card">
          <header class="flex min-w-0 items-start justify-between gap-3">
            <div class="min-w-0">
              <h3 class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ entry.email || entry.username || `#${entry.user_id}` }}</h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ entry.username || '-' }} · #{{ entry.user_id }}</p>
            </div>
            <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="allowlistToDelete = entry"><Icon name="trash" size="sm" /></button>
          </header>
          <p class="break-words text-sm text-gray-600 dark:text-gray-300">{{ entry.note || t('admin.instructionAudit.v2.noNote') }}</p>
          <footer class="mt-auto flex items-center justify-between gap-3 border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
            <span class="text-gray-400">{{ formatAuditDate(entry.created_at) }}</span>
            <span :class="entry.enabled ? 'text-primary-700 dark:text-primary-300' : 'text-gray-400'">{{ entry.enabled ? t('common.enabled') : t('common.disabled') }}</span>
          </footer>
        </article>
      </div>
      <EmptyPanel v-else icon="users" :title="t('admin.instructionAudit.v2.noAllowlist')" :description="t('admin.instructionAudit.v2.noAllowlistHint')" />
    </template>

    <BaseDialog :show="scopeForm.show" :title="scopeForm.id ? t('admin.instructionAudit.v2.editScope') : t('admin.instructionAudit.v2.addScope')" @close="closeScopeForm">
      <div class="space-y-4">
        <label class="block">
          <span class="input-label">{{ t('admin.instructionAudit.v2.downstreamGroup') }}</span>
          <select v-model="scopeForm.groupId" class="input">
            <option :value="0">{{ t('admin.instructionAudit.v2.selectGroup') }}</option>
            <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }}</option>
          </select>
        </label>
        <label class="block">
          <span class="input-label">{{ t('admin.instructionAudit.v2.clientScope') }}</span>
          <select v-model="scopeForm.clientProfileId" class="input">
            <option :value="0">{{ t('admin.instructionAudit.v2.allClients') }}</option>
            <option v-for="client in enabledClients" :key="client.id" :value="client.id">{{ client.name }}</option>
          </select>
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.clientScopeHint') }}</span>
        </label>
        <label class="flex items-center justify-between gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600">
          <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('common.enabled') }}</span>
          <input v-model="scopeForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </label>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeScopeForm">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !scopeForm.groupId" @click="saveScope"><Icon name="check" size="sm" />{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="clientForm.show" :title="clientForm.id ? t('admin.instructionAudit.v2.editClientProfile') : t('admin.instructionAudit.v2.addClientProfile')" width="wide" @close="closeClientForm">
      <div class="min-w-0 space-y-4">
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label class="min-w-0">
            <span class="input-label">{{ t('admin.instructionAudit.v2.profileKey') }}</span>
            <input v-model.trim="clientForm.profileKey" maxlength="64" pattern="[a-z][a-z0-9_]{1,63}" class="input font-mono" :disabled="clientForm.builtIn" />
          </label>
          <label class="min-w-0">
            <span class="input-label">{{ commonNameLabel }}</span>
            <input v-model="clientForm.name" maxlength="120" class="input" :disabled="clientForm.immutableInternal" />
          </label>
        </div>
        <label class="block min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.v2.profileDescription') }}</span>
          <input v-model="clientForm.description" maxlength="500" class="input" :disabled="clientForm.immutableInternal" />
        </label>
        <div class="grid gap-4 sm:grid-cols-2">
          <label>
            <span class="input-label">{{ t('admin.instructionAudit.v2.priority') }}</span>
            <input v-model.number="clientForm.priority" type="number" min="0" max="100000" class="input" :disabled="clientForm.immutableInternal" />
          </label>
          <label class="flex items-center justify-between gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600 sm:mt-6">
            <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('common.enabled') }}</span>
            <input v-model="clientForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </label>
        </div>
        <p v-if="clientForm.immutableInternal" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.immutableClient') }}</p>
        <fieldset class="min-w-0" :disabled="clientForm.immutableInternal">
          <div class="flex items-center justify-between gap-3">
            <legend class="input-label">{{ t('admin.instructionAudit.v2.userAgentMatchers') }}</legend>
            <button type="button" class="btn btn-secondary btn-sm" @click="addMatcher"><Icon name="plus" size="sm" />{{ t('common.add') }}</button>
          </div>
          <div class="mt-2 space-y-2">
            <div v-for="(matcher, index) in clientForm.matchers" :key="index" class="grid min-w-0 gap-2 rounded-md border border-gray-200 p-3 dark:border-dark-600 sm:grid-cols-[130px_minmax(0,1fr)_auto_auto] sm:items-center">
              <select v-model="matcher.type" class="input">
                <option value="prefix">prefix</option>
                <option value="regex">regex</option>
              </select>
              <input v-model="matcher.value" class="input min-w-0 font-mono text-xs" :placeholder="matcher.type === 'prefix' ? 'codex_cli_rs/' : '^client/[0-9]+'" />
              <label class="flex items-center gap-2 whitespace-nowrap text-xs text-gray-600 dark:text-gray-300"><input v-model="matcher.case_sensitive" type="checkbox" class="rounded border-gray-300 text-primary-600" />{{ t('admin.instructionAudit.v2.caseSensitive') }}</label>
              <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="clientForm.matchers.splice(index, 1)"><Icon name="trash" size="sm" /></button>
            </div>
          </div>
        </fieldset>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeClientForm">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !clientForm.profileKey || !clientForm.name" @click="saveClient"><Icon name="check" size="sm" />{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(scopeToDelete)" :title="t('admin.instructionAudit.v2.deleteScopeTitle')" :message="t('admin.instructionAudit.v2.deleteScopeConfirm')" danger @confirm="deleteScope" @cancel="scopeToDelete = null" />
    <ConfirmDialog :show="Boolean(clientToDelete)" :title="t('admin.instructionAudit.v2.deleteClientTitle')" :message="t('admin.instructionAudit.v2.deleteClientConfirm')" danger @confirm="deleteClient" @cancel="clientToDelete = null" />
    <ConfirmDialog :show="Boolean(allowlistToDelete)" :title="t('admin.instructionAudit.v2.deleteAllowlistTitle')" :message="t('admin.instructionAudit.v2.deleteAllowlistConfirm')" danger @confirm="deleteAllowlist" @cancel="allowlistToDelete = null" />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type {
  InstructionClientMatcher,
  InstructionClientProfile,
  InstructionGroupOption,
  InstructionScope,
  InstructionUserAllowlistEntry,
  InstructionUserOption,
} from '../v2Types'
import { formatAuditDate } from '../v2Presentation'

type Section = 'scopes' | 'clients' | 'allowlist'

const props = defineProps<{
  scopes: InstructionScope[]
  groups: InstructionGroupOption[]
  clients: InstructionClientProfile[]
  allowlist: InstructionUserAllowlistEntry[]
}>()
const emit = defineEmits<{ (event: 'changed'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const activeSection = ref<Section>('scopes')
const saving = ref(false)
const scopeToDelete = ref<InstructionScope | null>(null)
const clientToDelete = ref<InstructionClientProfile | null>(null)
const allowlistToDelete = ref<InstructionUserAllowlistEntry | null>(null)
const userSearch = ref('')
const userSearching = ref(false)
const userResults = ref<InstructionUserOption[]>([])
const selectedUserId = ref(0)
const allowlistNote = ref('')
const scopeForm = reactive({ show: false, id: 0, groupId: 0, clientProfileId: 0, enabled: true })
const clientForm = reactive({
  show: false,
  id: 0,
  builtIn: false,
  immutableInternal: false,
  profileKey: '',
  name: '',
  description: '',
  priority: 100,
  enabled: true,
  matchers: [] as InstructionClientMatcher[],
})

const tabs = computed(() => [
  { value: 'scopes' as const, label: t('admin.instructionAudit.v2.auditScopes'), count: props.scopes.length },
  { value: 'clients' as const, label: t('admin.instructionAudit.v2.clientProfiles'), count: props.clients.length },
  { value: 'allowlist' as const, label: t('admin.instructionAudit.v2.userAllowlist'), count: props.allowlist.length },
])
const enabledClients = computed(() => props.clients.filter((client) => client.enabled))
const commonNameLabel = computed(() => t('common.name'))

const EmptyPanel = defineComponent({
  props: { icon: { type: String, required: true }, title: { type: String, required: true }, description: { type: String, required: true } },
  setup(componentProps) {
    return () => h('div', { class: 'flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-gray-200 px-6 text-center dark:border-dark-600' }, [
      h(Icon, { name: componentProps.icon as 'shield', size: 'xl', class: 'text-gray-300 dark:text-dark-500' }),
      h('p', { class: 'mt-3 text-sm font-medium text-gray-700 dark:text-gray-200' }, componentProps.title),
      h('p', { class: 'mt-1 max-w-lg text-xs text-gray-500 dark:text-gray-400' }, componentProps.description),
    ])
  },
})

function openScopeForm(scope?: InstructionScope) {
  Object.assign(scopeForm, scope
    ? { show: true, id: scope.id, groupId: scope.group_id, clientProfileId: scope.client_profile_id ?? 0, enabled: scope.enabled }
    : { show: true, id: 0, groupId: 0, clientProfileId: 0, enabled: true })
}

function closeScopeForm() {
  if (!saving.value) scopeForm.show = false
}

async function saveScope() {
  saving.value = true
  try {
    await instructionAuditV2API.saveScope(scopeForm.id || null, { group_id: scopeForm.groupId, client_profile_id: scopeForm.clientProfileId || null, enabled: scopeForm.enabled })
    appStore.showSuccess(t('common.saved'))
    scopeForm.show = false
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function deleteScope() {
  if (!scopeToDelete.value) return
  try {
    await instructionAuditV2API.deleteScope(scopeToDelete.value!.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.scopeDeleted'))
    scopeToDelete.value = null
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function openClientForm(client?: InstructionClientProfile) {
  Object.assign(clientForm, client
    ? { show: true, id: client.id, builtIn: client.built_in, immutableInternal: client.immutable_internal, profileKey: client.profile_key, name: client.name, description: client.description, priority: client.priority, enabled: client.enabled, matchers: client.matchers.map((matcher) => ({ ...matcher })) }
    : { show: true, id: 0, builtIn: false, immutableInternal: false, profileKey: '', name: '', description: '', priority: 100, enabled: true, matchers: [] })
}

function closeClientForm() {
  if (!saving.value) clientForm.show = false
}

function addMatcher() {
  clientForm.matchers.push({ type: 'prefix', value: '', case_sensitive: false })
}

async function saveClient() {
  saving.value = true
  try {
    await instructionAuditV2API.saveClientProfile(clientForm.id || null, {
      profile_key: clientForm.profileKey,
      name: clientForm.name,
      description: clientForm.description,
      priority: clientForm.priority,
      enabled: clientForm.enabled,
      matchers: clientForm.matchers.filter((matcher) => matcher.value.trim()).map((matcher) => ({ ...matcher, value: matcher.value.trim() })),
    })
    appStore.showSuccess(t('common.saved'))
    clientForm.show = false
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function deleteClient() {
  if (!clientToDelete.value) return
  try {
    await instructionAuditV2API.deleteClientProfile(clientToDelete.value!.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.clientDeleted'))
    clientToDelete.value = null
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

async function searchUsers() {
  userSearching.value = true
  try {
    userResults.value = await instructionAuditV2API.searchUsers(userSearch.value)
    if (userResults.value.length === 1) selectedUserId.value = userResults.value[0].id
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    userSearching.value = false
  }
}

async function addAllowlist() {
  if (!selectedUserId.value) return
  saving.value = true
  try {
    await instructionAuditV2API.saveUserAllowlist(selectedUserId.value, allowlistNote.value)
    appStore.showSuccess(t('admin.instructionAudit.v2.allowlistAdded'))
    selectedUserId.value = 0
    allowlistNote.value = ''
    userResults.value = []
    userSearch.value = ''
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function deleteAllowlist() {
  if (!allowlistToDelete.value) return
  try {
    await instructionAuditV2API.deleteUserAllowlist(allowlistToDelete.value!.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.allowlistDeleted'))
    allowlistToDelete.value = null
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}
</script>

<style scoped>
.resource-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 330px), 1fr));
  gap: 0.875rem;
}

.resource-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800;
}

.resource-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}
</style>
