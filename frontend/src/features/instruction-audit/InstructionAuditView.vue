<template>
  <AppLayout>
    <div class="mx-auto max-w-[1500px] space-y-6 p-4 sm:p-6 lg:p-8">
      <header class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">
              {{ t('admin.instructionAudit.title') }}
            </h1>
            <span
              class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
              :class="overview?.enabled
                ? 'bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300'
                : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
            >
              <span class="h-1.5 w-1.5 rounded-full" :class="overview?.enabled ? 'bg-primary-500' : 'bg-gray-400'" />
              {{ overview?.enabled ? t('common.enabled') : t('common.disabled') }}
            </span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.instructionAudit.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <router-link to="/admin/settings" class="btn btn-secondary">
            <Icon name="cog" size="sm" />
            {{ t('admin.instructionAudit.systemSettings') }}
          </router-link>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="refreshAll">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </header>

      <div
        v-if="overview?.load_error"
        class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
      >
        {{ t('admin.instructionAudit.loadError') }}
      </div>

      <div
        v-if="overview && overview.persist_failure_count > 0"
        class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
      >
        {{ t('admin.instructionAudit.eventPersistenceWarning', { failed: overview.persist_failure_count }) }}
      </div>

      <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <div v-for="stat in overviewStats" :key="stat.label" class="card px-4 py-4">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ stat.label }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ stat.value }}</p>
        </div>
      </div>

      <div class="flex max-w-full gap-1 overflow-x-auto rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          type="button"
          class="min-w-max rounded-md px-4 py-2 text-sm font-medium transition-colors"
          :class="activeTab === tab.value
            ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
            : 'text-gray-600 hover:text-gray-950 dark:text-gray-400 dark:hover:text-white'"
          @click="activeTab = tab.value"
        >
          {{ tab.label }}
          <span v-if="tab.count !== undefined" class="ml-1 text-xs opacity-70">{{ tab.count }}</span>
        </button>
      </div>

      <template v-if="activeTab === 'rules'">
        <section class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.ruleSets') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.ruleSetCount', { count: ruleSets.length }) }}</p>
            </div>
            <button type="button" class="btn btn-primary" @click="openRuleSetDialog()">
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.addRuleSet') }}
            </button>
          </div>
          <div v-if="ruleSets.length" class="divide-y divide-gray-100 dark:divide-dark-700">
            <div v-for="rule in ruleSets" :key="rule.id" class="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="font-medium text-gray-950 dark:text-white">{{ rule.name }}</p>
                  <span :class="statusPill(rule.enabled ? 'active' : 'disabled')">
                    {{ rule.enabled ? t('common.enabled') : t('common.disabled') }}
                  </span>
                  <span class="text-xs text-gray-400">v{{ rule.version }}</span>
                </div>
                <p v-if="rule.description" class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ rule.description }}</p>
                <div class="mt-2 flex flex-wrap gap-1.5">
                  <span v-for="hash in rule.hashes" :key="hash.id" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ hash.name }}
                  </span>
                  <span v-if="!rule.hashes.length" class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.instructionAudit.noHashes') }}</span>
                </div>
              </div>
              <button type="button" class="btn btn-ghost btn-sm self-start lg:self-auto" @click="openRuleSetDialog(rule)">
                <Icon name="edit" size="sm" />
                {{ t('common.edit') }}
              </button>
            </div>
          </div>
          <EmptyState v-else :description="t('admin.instructionAudit.emptyRuleSets')" />
        </section>

        <section class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.bindings') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.bindingCount', { count: bindings.length }) }}</p>
            </div>
            <button type="button" class="btn btn-primary" :disabled="!ruleSets.length" @click="openBindingDialog">
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.addBinding') }}
            </button>
          </div>
          <div v-if="bindings.length" class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/70">
                <tr>
                  <th class="table-th">{{ t('admin.instructionAudit.user') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.model') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.ruleSet') }}</th>
                  <th class="table-th">{{ t('common.status') }}</th>
                  <th class="table-th text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-for="binding in bindings" :key="binding.id">
                  <td class="table-td">
                    <p class="font-medium text-gray-900 dark:text-white">{{ binding.user_email || binding.username || `#${binding.user_id}` }}</p>
                    <p class="text-xs text-gray-400">#{{ binding.user_id }}</p>
                  </td>
                  <td class="table-td font-mono text-xs text-gray-700 dark:text-gray-200">{{ binding.model }}</td>
                  <td class="table-td">{{ binding.rule_set_name }}</td>
                  <td class="table-td"><span :class="statusPill(binding.enabled ? 'active' : 'disabled')">{{ binding.enabled ? t('common.enabled') : t('common.disabled') }}</span></td>
                  <td class="table-td text-right">
                    <button type="button" class="btn btn-ghost btn-sm text-red-600 dark:text-red-400" :title="t('common.delete')" :aria-label="t('common.delete')" @click="requestDeleteBinding(binding)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <EmptyState v-else :description="t('admin.instructionAudit.emptyBindings')" />
        </section>
      </template>

      <section v-else-if="activeTab === 'hashes' || activeTab === 'candidates'" class="card overflow-hidden">
        <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-950 dark:text-white">
              {{ activeTab === 'candidates' ? t('admin.instructionAudit.candidateHashes') : t('admin.instructionAudit.hashLibrary') }}
            </h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.hashCount', { count: visibleHashes.length }) }}</p>
          </div>
          <div class="flex items-center gap-2">
            <select v-if="activeTab === 'hashes'" v-model="hashStatusFilter" class="input h-9 min-w-36 py-1.5 text-sm">
              <option value="">{{ t('common.all') }}</option>
              <option value="active">{{ t('common.enabled') }}</option>
              <option value="disabled">{{ t('common.disabled') }}</option>
              <option value="expired">{{ t('admin.instructionAudit.expired') }}</option>
            </select>
            <button type="button" class="btn btn-primary" @click="openHashDialog()">
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.addHash') }}
            </button>
          </div>
        </div>
        <div v-if="visibleHashes.length" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/70">
              <tr>
                <th class="table-th">{{ t('common.name') }}</th>
                <th class="table-th">SHA-256</th>
                <th class="table-th">{{ t('admin.instructionAudit.source') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.client') }}</th>
                <th class="table-th">{{ t('common.status') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.createdAt') }}</th>
                <th class="table-th text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-for="hash in visibleHashes" :key="hash.id">
                <td class="table-td">
                  <p class="font-medium text-gray-900 dark:text-white">{{ hash.name }}</p>
                  <p v-if="hash.note" class="max-w-xs truncate text-xs text-gray-400">{{ hash.note }}</p>
                </td>
                <td class="table-td font-mono text-xs" :title="hash.digest">{{ compactDigest(hash.digest) }}</td>
                <td class="table-td">{{ sourceLabel(hash.observed_source) }}</td>
                <td class="table-td text-sm">{{ [hash.client_name, hash.client_version].filter(Boolean).join(' ') || '-' }}</td>
                <td class="table-td"><span :class="statusPill(hash.status)">{{ hashStatusLabel(hash.status) }}</span></td>
                <td class="table-td text-xs text-gray-500 dark:text-gray-400">{{ formatDate(hash.created_at) }}</td>
                <td class="table-td text-right">
                  <button type="button" class="btn btn-ghost btn-sm" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openHashDialog(hash)">
                    <Icon name="edit" size="sm" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <EmptyState v-else :description="t('admin.instructionAudit.emptyHashes')" />
      </section>

      <section v-else class="card overflow-hidden">
        <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.auditLogs') }}</h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.eventCount', { count: eventPage.total }) }}</p>
          </div>
          <div class="grid gap-2 sm:grid-cols-[10rem_15rem_auto]">
            <input v-model.trim="eventFilters.userId" type="number" min="1" class="input h-9 py-1.5 text-sm" :placeholder="t('admin.instructionAudit.userId')" @keyup.enter="loadEvents(1)" />
            <input v-model.trim="eventFilters.model" type="search" class="input h-9 py-1.5 text-sm" :placeholder="t('admin.instructionAudit.model')" @keyup.enter="loadEvents(1)" />
            <button type="button" class="btn btn-secondary" @click="loadEvents(1)">
              <Icon name="search" size="sm" />
              {{ t('common.search') }}
            </button>
          </div>
        </div>
        <div v-if="eventPage.items.length" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/70">
              <tr>
                <th class="table-th">{{ t('admin.instructionAudit.time') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.user') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.model') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.fieldOne') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.fieldTwo') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.reason') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.notification') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-for="event in eventPage.items" :key="event.id" class="align-top">
                <td class="table-td whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">
                  {{ formatDate(event.created_at) }}
                  <p class="mt-1 font-mono text-[11px] text-gray-400">#{{ event.id }}</p>
                </td>
                <td class="table-td">
                  <p class="text-sm text-gray-900 dark:text-white">{{ event.user_email || '-' }}</p>
                  <p class="text-xs text-gray-400">#{{ event.user_id || '-' }} / key #{{ event.api_key_id || '-' }}</p>
                </td>
                <td class="table-td font-mono text-xs">{{ event.model }}</td>
                <td class="table-td"><FieldDigest :field="event.instructions" @candidate="createCandidate(event.id, 'instructions')" /></td>
                <td class="table-td"><FieldDigest :field="event.input1" @candidate="createCandidate(event.id, 'input1')" /></td>
                <td class="table-td text-xs">{{ reasonLabel(event.reason) }}<p class="mt-1 text-gray-400 dark:text-gray-500">v{{ event.config_version }} · {{ event.latency_ms }}ms</p></td>
                <td class="table-td"><span :class="notificationPill(event.notification_status)">{{ notificationLabel(event.notification_status) }}</span></td>
              </tr>
            </tbody>
          </table>
          <Pagination
            :total="eventPage.total"
            :page="eventPage.page"
            :page-size="eventPage.page_size"
            @update:page="loadEvents"
            @update:page-size="changeEventPageSize"
          />
        </div>
        <EmptyState v-else :description="t('admin.instructionAudit.emptyEvents')" />
      </section>
    </div>

    <BaseDialog :show="hashDialog.show" :title="hashDialog.id ? t('admin.instructionAudit.editHash') : t('admin.instructionAudit.addHash')" width="wide" @close="closeHashDialog">
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="saveHash">
        <div class="sm:col-span-2">
          <label class="input-label">{{ t('common.name') }}</label>
          <input v-model.trim="hashDialog.form.name" class="input" required maxlength="160" />
        </div>
        <template v-if="!hashDialog.id">
          <div class="sm:col-span-2">
            <div class="mb-2 flex gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
              <button v-for="mode in hashInputModes" :key="mode.value" type="button" class="flex-1 rounded-md px-3 py-2 text-sm" :class="hashDialog.mode === mode.value ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500'" @click="hashDialog.mode = mode.value">
                {{ mode.label }}
              </button>
            </div>
            <label class="input-label">{{ hashDialog.mode === 'digest' ? 'SHA-256' : t('admin.instructionAudit.plaintext') }}</label>
            <input v-if="hashDialog.mode === 'digest'" v-model.trim="hashDialog.form.digest" class="input font-mono" required maxlength="64" autocomplete="off" />
            <textarea v-else v-model="hashDialog.plaintext" class="input min-h-36 font-mono text-sm" required autocomplete="off" />
          </div>
        </template>
        <div v-else class="sm:col-span-2">
          <label class="input-label">SHA-256</label>
          <input :value="hashDialog.form.digest" class="input font-mono" disabled />
        </div>
        <div>
          <label class="input-label">{{ t('common.status') }}</label>
          <select v-model="hashDialog.form.status" class="input">
            <option value="candidate">{{ t('admin.instructionAudit.candidate') }}</option>
            <option value="active">{{ t('common.enabled') }}</option>
            <option value="disabled">{{ t('common.disabled') }}</option>
            <option value="expired">{{ t('admin.instructionAudit.expired') }}</option>
          </select>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.source') }}</label>
          <select v-model="hashDialog.form.observed_source" class="input">
            <option value="">-</option>
            <option value="instructions">{{ t('admin.instructionAudit.fieldOne') }}</option>
            <option value="input1">{{ t('admin.instructionAudit.fieldTwo') }}</option>
          </select>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.clientName') }}</label>
          <input v-model.trim="hashDialog.form.client_name" class="input" maxlength="120" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.clientVersion') }}</label>
          <input v-model.trim="hashDialog.form.client_version" class="input" maxlength="120" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.validFrom') }}</label>
          <input v-model="hashDialog.validFrom" type="datetime-local" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.validUntil') }}</label>
          <input v-model="hashDialog.validUntil" type="datetime-local" class="input" />
        </div>
        <div class="sm:col-span-2">
          <label class="input-label">{{ t('admin.instructionAudit.note') }}</label>
          <textarea v-model.trim="hashDialog.form.note" class="input min-h-20" />
        </div>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeHashDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="!canSaveHash" @click="saveHash">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="ruleDialog.show" :title="ruleDialog.id ? t('admin.instructionAudit.editRuleSet') : t('admin.instructionAudit.addRuleSet')" width="wide" @close="ruleDialog.show = false">
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('common.name') }}</label>
          <input v-model.trim="ruleDialog.name" class="input" maxlength="160" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.ruleDescription') }}</label>
          <textarea v-model.trim="ruleDialog.description" class="input min-h-20" />
        </div>
        <div class="flex items-center justify-between rounded-lg border border-gray-200 px-4 py-3 dark:border-dark-600">
          <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('common.enabled') }}</span>
          <Toggle v-model="ruleDialog.enabled" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.hashLibrary') }}</label>
          <div class="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <label v-for="hash in hashes" :key="hash.id" class="flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-gray-50 dark:hover:bg-dark-700">
              <input v-model="ruleDialog.hashIds" type="checkbox" :value="hash.id" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0 flex-1 text-sm text-gray-800 dark:text-gray-200">{{ hash.name }}</span>
              <span :class="statusPill(hash.status)">{{ hashStatusLabel(hash.status) }}</span>
            </label>
            <p v-if="!hashes.length" class="py-6 text-center text-sm text-gray-400">{{ t('admin.instructionAudit.emptyHashes') }}</p>
          </div>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="ruleDialog.show = false">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="saveRuleSet">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="bindingDialog.show" :title="t('admin.instructionAudit.addBinding')" width="normal" @close="bindingDialog.show = false">
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.user') }}</label>
          <div class="flex gap-2">
            <input v-model.trim="bindingDialog.search" class="input" :placeholder="t('admin.instructionAudit.searchUsers')" @keyup.enter="searchUsers" />
            <button type="button" class="btn btn-secondary" :title="t('common.search')" :aria-label="t('common.search')" @click="searchUsers"><Icon name="search" size="sm" /></button>
          </div>
          <select v-model.number="bindingDialog.userId" class="input mt-2">
            <option :value="0">{{ t('admin.instructionAudit.selectUser') }}</option>
            <option v-for="user in userOptions" :key="user.id" :value="user.id">{{ user.email || user.username || `#${user.id}` }} (#{{ user.id }})</option>
          </select>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.model') }}</label>
          <input v-model.trim="bindingDialog.model" class="input font-mono" maxlength="255" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.ruleSet') }}</label>
          <select v-model.number="bindingDialog.ruleSetId" class="input">
            <option :value="0">{{ t('admin.instructionAudit.selectRuleSet') }}</option>
            <option v-for="rule in ruleSets" :key="rule.id" :value="rule.id">{{ rule.name }}</option>
          </select>
        </div>
        <div class="flex items-center justify-between rounded-lg border border-gray-200 px-4 py-3 dark:border-dark-600">
          <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('common.enabled') }}</span>
          <Toggle v-model="bindingDialog.enabled" />
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="bindingDialog.show = false">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="saveBinding">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(bindingToDelete)"
      :title="t('admin.instructionAudit.deleteBinding')"
      :message="t('admin.instructionAudit.deleteBindingConfirm')"
      danger
      @confirm="deleteBinding"
      @cancel="bindingToDelete = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from './api'
import { resolveInstructionHashDigest } from './hash'
import type {
  InstructionBinding,
  InstructionEventPage,
  InstructionFieldResult,
  InstructionHashEntry,
  InstructionHashStatus,
  InstructionObservedSource,
  InstructionOverview,
  InstructionRuleSet,
  InstructionUserOption,
  SaveInstructionHashRequest,
} from './types'

type Tab = 'rules' | 'hashes' | 'candidates' | 'events'

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<Tab>('rules')
const loading = ref(false)
const saving = ref(false)
const overview = ref<InstructionOverview | null>(null)
const hashes = ref<InstructionHashEntry[]>([])
const ruleSets = ref<InstructionRuleSet[]>([])
const bindings = ref<InstructionBinding[]>([])
const userOptions = ref<InstructionUserOption[]>([])
const hashStatusFilter = ref('')
const bindingToDelete = ref<InstructionBinding | null>(null)
const eventPage = reactive<InstructionEventPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const eventFilters = reactive({ userId: '', model: '' })

const tabs = computed(() => [
  { value: 'rules' as const, label: t('admin.instructionAudit.rulesAndBindings'), count: bindings.value.length },
  { value: 'hashes' as const, label: t('admin.instructionAudit.hashLibrary'), count: hashes.value.filter((item) => item.status !== 'candidate').length },
  { value: 'candidates' as const, label: t('admin.instructionAudit.candidateHashes'), count: hashes.value.filter((item) => item.status === 'candidate').length },
  { value: 'events' as const, label: t('admin.instructionAudit.auditLogs'), count: eventPage.total },
])

const overviewStats = computed(() => [
  { label: t('admin.instructionAudit.activeHashes'), value: overview.value?.active_hash_count ?? 0 },
  { label: t('admin.instructionAudit.ruleSets'), value: overview.value?.rule_set_count ?? 0 },
  { label: t('admin.instructionAudit.activeBindings'), value: overview.value?.active_binding_count ?? 0 },
  { label: t('admin.instructionAudit.persistFailures'), value: overview.value?.persist_failure_count ?? 0 },
  { label: t('admin.instructionAudit.pendingEmails'), value: overview.value?.pending_email_count ?? 0 },
  { label: t('admin.instructionAudit.configVersion'), value: overview.value?.config_version ?? '-' },
])

const visibleHashes = computed(() => {
  if (activeTab.value === 'candidates') return hashes.value.filter((item) => item.status === 'candidate')
  return hashes.value.filter((item) => item.status !== 'candidate' && (!hashStatusFilter.value || item.status === hashStatusFilter.value))
})

const hashDialog = reactive({
  show: false,
  id: null as number | null,
  mode: 'digest' as 'digest' | 'plaintext',
  plaintext: '',
  validFrom: '',
  validUntil: '',
  form: emptyHashForm(),
})

const ruleDialog = reactive({ show: false, id: null as number | null, name: '', description: '', enabled: true, hashIds: [] as number[] })
const bindingDialog = reactive({ show: false, search: '', userId: 0, model: '', ruleSetId: 0, enabled: true })

const hashInputModes = computed(() => [
  { value: 'digest' as const, label: t('admin.instructionAudit.enterDigest') },
  { value: 'plaintext' as const, label: t('admin.instructionAudit.calculateLocally') },
])

const canSaveHash = computed(() => {
  if (saving.value || !hashDialog.form.name.trim()) return false
  if (hashDialog.id) return true
  if (hashDialog.mode === 'plaintext') return hashDialog.plaintext.length > 0
  return /^[0-9a-f]{64}$/i.test(hashDialog.form.digest.trim())
})

const FieldDigest = defineComponent({
  props: { field: { type: Object as () => InstructionFieldResult, required: true } },
  emits: ['candidate'],
  setup(props, { emit }) {
    return () => h('div', { class: 'min-w-40 space-y-1.5' }, [
      h('span', { class: statusPill(props.field.result) }, fieldResultLabel(props.field.result)),
      props.field.sha256 ? h('p', { class: 'font-mono text-[11px] text-gray-500 dark:text-gray-400', title: props.field.sha256 }, compactDigest(props.field.sha256)) : null,
      props.field.sha256 ? h('button', { type: 'button', class: 'text-xs font-medium text-primary-600 hover:underline dark:text-primary-400', onClick: () => emit('candidate') }, t('admin.instructionAudit.addCandidate')) : null,
    ])
  },
})

function emptyHashForm(): SaveInstructionHashRequest {
  return { digest: '', name: '', note: '', observed_source: '', client_name: '', client_version: '', status: 'candidate' }
}

async function refreshAll() {
  loading.value = true
  try {
    const [nextOverview, nextHashes, nextRules, nextBindings] = await Promise.all([
      instructionAuditAPI.getOverview(),
      instructionAuditAPI.listHashes(),
      instructionAuditAPI.listRuleSets(),
      instructionAuditAPI.listBindings(),
    ])
    overview.value = nextOverview
    hashes.value = nextHashes
    ruleSets.value = nextRules
    bindings.value = nextBindings
    await loadEvents(eventPage.page)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

async function loadEvents(page = 1) {
  const result = await instructionAuditAPI.listEvents({
    page,
    page_size: eventPage.page_size,
    user_id: eventFilters.userId ? Number(eventFilters.userId) : undefined,
    model: eventFilters.model || undefined,
  })
  Object.assign(eventPage, result)
}

async function changeEventPageSize(pageSize: number) {
  eventPage.page_size = pageSize
  await loadEvents(1)
}

function openHashDialog(item?: InstructionHashEntry) {
  hashDialog.show = true
  hashDialog.id = item?.id ?? null
  hashDialog.mode = 'digest'
  hashDialog.plaintext = ''
  hashDialog.validFrom = toDateTimeLocal(item?.valid_from)
  hashDialog.validUntil = toDateTimeLocal(item?.valid_until)
  hashDialog.form = item ? {
    digest: item.digest,
    name: item.name,
    note: item.note,
    observed_source: item.observed_source,
    client_name: item.client_name,
    client_version: item.client_version,
    status: item.status,
  } : emptyHashForm()
}

function closeHashDialog() {
  hashDialog.show = false
  hashDialog.plaintext = ''
}

async function saveHash() {
  if (!canSaveHash.value) return
  saving.value = true
  try {
    const digest = hashDialog.id
      ? hashDialog.form.digest
      : await resolveInstructionHashDigest(hashDialog.mode, hashDialog.form.digest, hashDialog.plaintext)
    const payload: SaveInstructionHashRequest = {
      ...hashDialog.form,
      digest,
      valid_from: fromDateTimeLocal(hashDialog.validFrom),
      valid_until: fromDateTimeLocal(hashDialog.validUntil),
    }
    if (hashDialog.id) {
      await instructionAuditAPI.updateHash(hashDialog.id, {
        name: payload.name,
        note: payload.note,
        observed_source: payload.observed_source,
        client_name: payload.client_name,
        client_version: payload.client_version,
        status: payload.status,
        valid_from: payload.valid_from,
        valid_until: payload.valid_until,
        clear_valid_from: !hashDialog.validFrom,
        clear_valid_until: !hashDialog.validUntil,
      })
    }
    else await instructionAuditAPI.createHash(payload)
    closeHashDialog()
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    hashDialog.plaintext = ''
    saving.value = false
  }
}

function openRuleSetDialog(rule?: InstructionRuleSet) {
  ruleDialog.show = true
  ruleDialog.id = rule?.id ?? null
  ruleDialog.name = rule?.name ?? ''
  ruleDialog.description = rule?.description ?? ''
  ruleDialog.enabled = rule?.enabled ?? true
  ruleDialog.hashIds = rule?.hashes.map((item) => item.id) ?? []
}

async function saveRuleSet() {
  if (!ruleDialog.name.trim()) return
  saving.value = true
  try {
    await instructionAuditAPI.saveRuleSet(ruleDialog.id, {
      name: ruleDialog.name,
      description: ruleDialog.description,
      enabled: ruleDialog.enabled,
      hash_ids: ruleDialog.hashIds,
    })
    ruleDialog.show = false
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

async function openBindingDialog() {
  Object.assign(bindingDialog, { show: true, search: '', userId: 0, model: '', ruleSetId: ruleSets.value[0]?.id ?? 0, enabled: true })
  await searchUsers()
}

async function searchUsers() {
  try {
    userOptions.value = await instructionAuditAPI.searchUsers(bindingDialog.search)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function saveBinding() {
  if (!bindingDialog.userId || !bindingDialog.model || !bindingDialog.ruleSetId) return
  saving.value = true
  try {
    await instructionAuditAPI.saveBinding({
      user_id: bindingDialog.userId,
      model: bindingDialog.model,
      rule_set_id: bindingDialog.ruleSetId,
      enabled: bindingDialog.enabled,
    })
    bindingDialog.show = false
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

function requestDeleteBinding(binding: InstructionBinding) {
  bindingToDelete.value = binding
}

async function deleteBinding() {
  if (!bindingToDelete.value) return
  try {
    await instructionAuditAPI.deleteBinding(bindingToDelete.value.id)
    bindingToDelete.value = null
    appStore.showSuccess(t('common.deleted'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function createCandidate(eventId: number, source: 'instructions' | 'input1') {
  try {
    await instructionAuditAPI.createCandidate(eventId, source)
    appStore.showSuccess(t('admin.instructionAudit.candidateAdded'))
    await refreshAll()
    activeTab.value = 'candidates'
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

function compactDigest(value: string): string {
  return value ? `${value.slice(0, 10)}…${value.slice(-8)}` : '-'
}

function formatDate(value?: string | null): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function toDateTimeLocal(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function fromDateTimeLocal(value: string): string | null {
  return value ? new Date(value).toISOString() : null
}

function sourceLabel(source: InstructionObservedSource): string {
  if (source === 'instructions') return t('admin.instructionAudit.fieldOne')
  if (source === 'input1') return t('admin.instructionAudit.fieldTwo')
  return '-'
}

function hashStatusLabel(status: InstructionHashStatus): string {
  if (status === 'active') return t('common.enabled')
  if (status === 'disabled') return t('common.disabled')
  if (status === 'expired') return t('admin.instructionAudit.expired')
  return t('admin.instructionAudit.candidate')
}

function statusPill(status: string): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
  if (status === 'active' || status === 'match' || status === 'sent') return `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300`
  if (status === 'candidate' || status === 'pending' || status === 'retry' || status === 'processing') return `${base} bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300`
  if (status === 'disabled' || status === 'missing' || status === 'not_checked') return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
}

function notificationPill(status: string): string {
  return statusPill(status)
}

function notificationLabel(status: string): string {
  const key = `admin.instructionAudit.notifications.${status}`
  return t(key)
}

function fieldResultLabel(result: string): string {
  return t(`admin.instructionAudit.fieldResults.${result}`)
}

function reasonLabel(reason: string): string {
  return t(`admin.instructionAudit.reasons.${reason}`)
}

watch(activeTab, (tab) => {
  if (tab === 'events' && !eventPage.items.length) void loadEvents(1)
})

onMounted(refreshAll)
</script>

<style scoped>
.table-th {
  padding: 0.75rem 1.25rem;
  text-align: left;
  font-size: 0.6875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: rgb(107 114 128);
}

.table-td {
  padding: 0.875rem 1.25rem;
  color: rgb(75 85 99);
}

:global(.dark) .table-th,
:global(.dark) .table-td {
  color: rgb(209 213 219);
}
</style>
