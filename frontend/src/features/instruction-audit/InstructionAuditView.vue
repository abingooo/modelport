<template>
  <AppLayout>
    <div class="w-full min-w-0 max-w-none space-y-6">
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
          <router-link :to="{ path: '/admin/settings', query: { tab: 'features' } }" class="btn btn-secondary">
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

      <div
        v-if="overview"
        class="flex flex-col gap-3 rounded-md border px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
        :class="overview.evidence_encryption_available
          ? 'border-primary-200 bg-primary-50/60 dark:border-primary-900/60 dark:bg-primary-950/20'
          : 'border-red-200 bg-red-50 dark:border-red-900/60 dark:bg-red-950/30'"
      >
        <div>
          <p class="text-sm font-medium" :class="overview.evidence_encryption_available ? 'text-primary-800 dark:text-primary-200' : 'text-red-700 dark:text-red-300'">
            {{ overview.evidence_encryption_available
              ? t('admin.instructionAudit.evidenceEncryptionReady')
              : t('admin.instructionAudit.evidenceEncryptionUnavailable') }}
          </p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.evidenceRetentionHint') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <input v-model.number="retentionDays" type="number" min="1" max="3650" class="input h-9 w-24 py-1.5 text-sm" :aria-label="t('admin.instructionAudit.evidenceRetentionDays')" />
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.days') }}</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="saveEvidenceRetention">
            <Icon name="check" size="sm" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <div v-for="stat in overviewStats" :key="stat.label" class="card px-4 py-4">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ stat.label }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ stat.value }}</p>
        </div>
      </div>

      <InstructionAuditStatistics
        :statistics="statistics"
        :loading="statisticsLoading"
        :error="statisticsError"
      />

      <section v-if="overview" aria-labelledby="instruction-audit-runtime-health-title">
        <div class="mb-3">
          <h2 id="instruction-audit-runtime-health-title" class="text-base font-semibold text-gray-950 dark:text-white">
            {{ t('admin.instructionAudit.runtimeHealth.title') }}
          </h2>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtimeHealth.hint') }}</p>
        </div>
        <dl class="grid gap-3" style="grid-template-columns: repeat(auto-fit, minmax(min(100%, 10.5rem), 1fr));">
          <div v-for="metric in runtimeHealthStats" :key="metric.key" class="min-h-24 min-w-0 rounded-md border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800">
            <dt class="break-words text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</dt>
            <dd class="mt-2 break-words text-xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ metric.value }}</dd>
            <p v-if="metric.hint" class="mt-1 break-words text-[11px] text-gray-400 dark:text-gray-500">{{ metric.hint }}</p>
          </div>
        </dl>
      </section>

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

      <InstructionAuditRuntimeConfig
        v-if="activeTab === 'config'"
        :config="runtimeConfig"
        :loading="loading"
        :saving="runtimeSaving"
        :error="runtimeError"
        @save="saveRuntimeConfig"
      />

      <InstructionAuditReasonPolicies
        v-else-if="activeTab === 'policies'"
        :policies="reasonPolicies"
        :loading="loading"
        :error="policiesError"
        :saving-reason="savingReason"
        :config-version="overview?.config_version || runtimeConfig?.config_version || 0"
        @save="saveReasonPolicy"
      />

      <template v-else-if="activeTab === 'rules'">
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
                  <span v-if="rule.system_managed" class="rounded bg-cyan-50 px-2 py-1 text-xs font-medium text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300">{{ t('admin.instructionAudit.systemManaged') }}</span>
                </div>
                <p v-if="rule.description" class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ rule.description }}</p>
                <div class="mt-2 flex flex-wrap gap-1.5">
                  <span v-for="hash in rule.hashes" :key="hash.id" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ hash.name }}
                  </span>
                  <span v-if="rule.allow_empty_fields" class="rounded bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">
                    {{ t('admin.instructionAudit.allowEmptyFields') }}
                  </span>
                  <span v-if="rule.allowed_users.length" class="rounded bg-cyan-50 px-2 py-1 text-xs font-medium text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300">
                    {{ t('admin.instructionAudit.userAllowlistCount', { count: rule.allowed_users.length }) }}
                  </span>
                  <span v-if="!rule.hashes.length && !rule.allow_empty_fields && !rule.allowed_users.length" class="text-xs text-amber-600 dark:text-amber-400">
                    {{ t('admin.instructionAudit.noAllowConditions') }}
                  </span>
                </div>
              </div>
              <div class="flex shrink-0 items-center gap-1 self-start lg:self-auto">
                <button v-if="!rule.system_managed" type="button" class="btn btn-ghost btn-sm" @click="openRuleSetDialog(rule)">
                  <Icon name="edit" size="sm" />
                  {{ t('common.edit') }}
                </button>
                <button v-if="!rule.system_managed" type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('admin.instructionAudit.deleteRuleSet')" :aria-label="t('admin.instructionAudit.deleteRuleSet')" @click="ruleSetToDelete = rule">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
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
            <button type="button" class="btn btn-primary" :disabled="!ruleSets.length" @click="openBindingDialog()">
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.addBinding') }}
            </button>
          </div>
          <div v-if="bindings.length" class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/70">
                <tr>
                  <th class="table-th">{{ t('admin.instructionAudit.group') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.platform') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.ruleSet') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.clientScope') }}</th>
                  <th class="table-th">{{ t('common.status') }}</th>
                  <th class="table-th text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-for="binding in bindings" :key="binding.id">
                  <td class="table-td">
                    <p class="font-medium text-gray-900 dark:text-white">{{ binding.group_name }}</p>
                    <p class="text-xs text-gray-400">#{{ binding.group_id }}</p>
                  </td>
                  <td class="table-td">
                    <span class="font-mono text-xs text-gray-700 dark:text-gray-200">{{ binding.platform }}</span>
                    <span v-if="binding.group_status !== 'active'" class="ml-2" :class="statusPill('disabled')">{{ t('common.disabled') }}</span>
                  </td>
                  <td class="table-td">{{ binding.rule_set_name }}</td>
                  <td class="table-td">
                    <div class="flex max-w-sm flex-wrap gap-1.5">
                      <span v-for="clientType in binding.client_types" :key="clientType" :class="clientTypePill(clientType)">
                        {{ clientTypeLabel(clientType) }}
                      </span>
                    </div>
                  </td>
                  <td class="table-td">
                    <div class="flex items-center gap-3">
                      <Toggle :model-value="binding.enabled" :disabled="saving" @update:model-value="setBindingEnabled(binding, $event)" />
                      <span v-if="binding.enabled && !binding.effective" :class="statusPill('invalid')">{{ t('admin.instructionAudit.ineffectiveRule') }}</span>
                    </div>
                  </td>
                  <td class="table-td text-right">
                    <button type="button" class="btn btn-ghost btn-sm" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openBindingDialog(binding)">
                      <Icon name="edit" size="sm" />
                    </button>
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
                  <button type="button" class="btn btn-ghost btn-sm" :title="t('admin.instructionAudit.hashDetail.title')" :aria-label="t('admin.instructionAudit.hashDetail.title')" @click="hashDetailID = hash.id">
                    <Icon name="eye" size="sm" />
                  </button>
                  <button type="button" class="btn btn-ghost btn-sm" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openHashDialog(hash)">
                    <Icon name="edit" size="sm" />
                  </button>
                  <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('admin.instructionAudit.deleteHash')" :aria-label="t('admin.instructionAudit.deleteHash')" @click="hashToDelete = hash">
                    <Icon name="trash" size="sm" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <EmptyState v-else :description="t('admin.instructionAudit.emptyHashes')" />
      </section>

      <section v-else class="card overflow-hidden">
        <div class="space-y-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div class="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
            <div class="flex flex-wrap items-end gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.auditLogs') }}</h2>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.eventCount', { count: eventPage.total }) }}</p>
              </div>
              <div class="flex flex-wrap gap-2">
                <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedEventIds.length || deletingEvents" @click="requestBatchDeleteEvents">
                  <Icon name="trash" size="sm" />
                  {{ t('admin.instructionAudit.deleteSelected', { count: selectedEventIds.length }) }}
                </button>
                <button type="button" class="btn btn-danger btn-sm" :disabled="deletingEvents" @click="openLogCleanupDialog">
                  <Icon name="trash" size="sm" />
                  {{ t('admin.instructionAudit.clearLogs') }}
                </button>
              </div>
            </div>
            <div class="flex min-w-0 flex-1 flex-col gap-2 xl:max-w-4xl xl:flex-row xl:justify-end">
              <div class="flex min-w-0 flex-1">
                <input
                  v-model.trim="eventFilters.q"
                  type="search"
                  class="input h-9 min-w-0 rounded-r-none py-1.5 text-sm"
                  :placeholder="t('admin.instructionAudit.searchPlaceholder')"
                  @keyup.enter="applyEventFilters"
                />
                <button type="button" class="btn btn-primary h-9 rounded-l-none" @click="applyEventFilters">
                  <Icon name="search" size="sm" />
                  {{ t('common.search') }}
                </button>
              </div>
              <div class="flex max-w-full gap-1 overflow-x-auto rounded-md bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  v-for="range in timeRanges"
                  :key="range.value"
                  type="button"
                  class="min-w-max rounded px-3 py-1.5 text-xs font-medium"
                  :class="eventFilters.range === range.value ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'"
                  @click="setTimeRange(range.value)"
                >
                  {{ range.label }}
                </button>
              </div>
              <button type="button" class="btn btn-secondary h-9" @click="advancedFiltersOpen = !advancedFiltersOpen">
                <Icon name="filter" size="sm" />
                {{ t('admin.instructionAudit.advancedFilters') }}
                <span v-if="activeFilterCount" class="rounded-full bg-primary-100 px-1.5 py-0.5 text-[10px] text-primary-700 dark:bg-primary-900/50 dark:text-primary-200">{{ activeFilterCount }}</span>
              </button>
            </div>
          </div>

          <div v-if="eventFilters.range === 'custom'" class="grid gap-3 rounded-md border border-gray-200 p-3 dark:border-dark-600 sm:grid-cols-2">
            <label class="text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.instructionAudit.fromTime') }}
              <input v-model="eventFilters.from" type="datetime-local" class="input mt-1" />
            </label>
            <label class="text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.instructionAudit.toTime') }}
              <input v-model="eventFilters.to" type="datetime-local" class="input mt-1" />
            </label>
          </div>

          <div v-if="advancedFiltersOpen" class="grid gap-3 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.requestSubject') }}</legend>
              <div class="space-y-2 p-2">
                <label class="block">
                  <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.userId') }}</span>
                  <input v-model.number="eventFilters.userId" type="number" min="1" class="input mt-1 h-8 py-1 text-xs" />
                </label>
                <label class="block">
                  <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.model') }}</span>
                  <input v-model.trim="eventFilters.model" class="input mt-1 h-8 py-1 text-xs" />
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.group') }}</legend>
              <div class="filter-options">
                <label v-for="group in groupOptions" :key="group.id" class="filter-option">
                  <input v-model="eventFilters.groupIds" type="checkbox" :value="group.id" />
                  <span class="truncate">{{ group.name }}</span>
                  <span class="ml-auto text-[10px] text-gray-400">#{{ group.id }}</span>
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.client') }}</legend>
              <div class="filter-options">
                <label v-for="client in clientOptions" :key="client.value" class="filter-option">
                  <input v-model="eventFilters.clientTypes" type="checkbox" :value="client.value" />
                  <span>{{ client.label }}</span>
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.reason') }}</legend>
              <div class="filter-options">
                <label v-for="reason in reasonOptions" :key="reason" class="filter-option">
                  <input v-model="eventFilters.reasons" type="checkbox" :value="reason" />
                  <span>{{ reasonLabel(reason) }}</span>
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.finalOutcome') }}</legend>
              <div class="filter-options">
                <label v-for="outcome in outcomeOptions" :key="outcome" class="filter-option">
                  <input v-model="eventFilters.finalOutcomes" type="checkbox" :value="outcome" />
                  <span>{{ outcomeLabel(outcome) }}</span>
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.validationResults') }}</legend>
              <div class="filter-options">
                <label v-for="result in fieldResultOptions" :key="`i-${result}`" class="filter-option">
                  <input v-model="eventFilters.instructionsResults" type="checkbox" :value="result" />
                  <span>instructions: {{ fieldResultLabel(result) }}</span>
                </label>
                <label v-for="result in fieldResultOptions" :key="`f-${result}`" class="filter-option">
                  <input v-model="eventFilters.input1Results" type="checkbox" :value="result" />
                  <span>input[1]: {{ fieldResultLabel(result) }}</span>
                </label>
              </div>
            </fieldset>
            <fieldset class="filter-fieldset">
              <legend class="filter-legend">{{ t('admin.instructionAudit.notification') }}</legend>
              <div class="filter-options">
                <label v-for="status in notificationOptions" :key="`u-${status}`" class="filter-option">
                  <input v-model="eventFilters.userNotifications" type="checkbox" :value="status" />
                  <span>{{ t('admin.instructionAudit.userNotification') }}: {{ notificationLabel(status) }}</span>
                </label>
                <label v-for="status in notificationOptions" :key="`o-${status}`" class="filter-option">
                  <input v-model="eventFilters.opsNotifications" type="checkbox" :value="status" />
                  <span>{{ t('admin.instructionAudit.opsNotification') }}: {{ notificationLabel(status) }}</span>
                </label>
              </div>
            </fieldset>
          </div>

          <div v-if="filterChips.length" class="flex flex-wrap items-center gap-2">
            <button v-for="chip in filterChips" :key="chip.key" type="button" class="filter-chip" @click="chip.remove">
              <span>{{ chip.label }}</span>
              <Icon name="x" size="xs" />
            </button>
            <button type="button" class="text-xs font-medium text-primary-600 hover:underline dark:text-primary-400" @click="resetEventFilters">
              {{ t('admin.instructionAudit.resetFilters') }}
            </button>
          </div>
        </div>
        <div v-if="eventPage.items.length">
          <div class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800 xl:hidden">
            <article v-for="event in eventPage.items" :key="`compact-${event.id}`" class="space-y-4 p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="flex min-w-0 items-start gap-3">
                  <input type="checkbox" class="mt-1" :checked="selectedEventIds.includes(event.id)" :aria-label="t('admin.instructionAudit.selectEvent', { id: event.id })" @change="toggleEventSelection(event.id)" />
                  <div class="min-w-0">
                    <div class="flex items-center gap-1.5">
                      <button type="button" class="font-semibold text-primary-700 hover:underline dark:text-primary-300" @click="setEventIDFilter(event.id)">{{ t('admin.instructionAudit.eventNumber', { id: event.id }) }}</button>
                      <button type="button" class="icon-btn h-6 w-6" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyText(String(event.id))"><Icon name="copy" size="xs" /></button>
                    </div>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ formatDate(event.created_at) }}</p>
                    <button type="button" class="mt-1 block max-w-full truncate font-mono text-[11px] text-primary-600 hover:underline dark:text-primary-400" :title="event.request_id" @click="setQueryFilter(event.request_id)">{{ event.request_id || '-' }}</button>
                  </div>
                </div>
                <div class="flex shrink-0 flex-wrap justify-end gap-1">
                  <button type="button" class="btn btn-primary btn-sm" :disabled="!eventHasDigest(event) || !ruleSets.length" @click="openAddToRuleSetDialog(event)"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.quickAdd') }}</button>
                  <button type="button" class="icon-btn" :title="t('admin.instructionAudit.reviewEvidence')" :aria-label="t('admin.instructionAudit.reviewEvidence')" @click="openEvidenceReview(event)"><Icon name="eye" size="sm" /></button>
                  <router-link :to="opsLogLink(event)" class="icon-btn" :title="t('admin.instructionAudit.viewSystemLog')" :aria-label="t('admin.instructionAudit.viewSystemLog')"><Icon name="externalLink" size="sm" /></router-link>
                  <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" :aria-label="t('common.delete')" @click="eventToDelete = event"><Icon name="trash" size="sm" /></button>
                </div>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div class="min-w-0">
                  <p class="text-[11px] font-semibold uppercase text-gray-400">{{ t('admin.instructionAudit.requestSubject') }}</p>
                  <button type="button" class="mt-1 block max-w-full truncate text-left text-sm text-gray-900 hover:text-primary-600 dark:text-white" :title="event.user_email" @click="setQueryFilter(event.user_email)">{{ event.user_email || '-' }}</button>
                  <p class="text-xs text-gray-400">user #{{ event.user_id || '-' }} · key #{{ event.api_key_id || '-' }}</p>
                  <button type="button" class="mt-1 block max-w-full truncate text-left text-xs text-gray-600 hover:text-primary-600 dark:text-gray-300" @click="addArrayFilter('groupIds', event.group_id)">{{ event.group_name || '-' }} #{{ event.group_id || '-' }}</button>
                </div>
                <div class="min-w-0">
                  <p class="text-[11px] font-semibold uppercase text-gray-400">{{ t('admin.instructionAudit.clientAndModel') }}</p>
                  <button type="button" class="mt-1" :class="clientTypePill(event.client_type)" @click="addArrayFilter('clientTypes', event.client_type)">{{ clientTypeLabel(event.client_type) }}</button>
                  <button type="button" class="mt-1 block max-w-full truncate font-mono text-xs text-gray-700 hover:text-primary-600 dark:text-gray-300" :title="event.model" @click="setQueryFilter(event.model)">{{ event.model || '-' }}</button>
                </div>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div class="space-y-2">
                  <p class="text-[11px] font-semibold uppercase text-gray-400">{{ t('admin.instructionAudit.validationResults') }}</p>
                  <div class="grid grid-cols-[4.5rem_minmax(0,1fr)] items-start gap-1.5"><span class="font-mono text-[11px] text-gray-400">instructions</span><FieldDigest :field="event.instructions" @filter="addArrayFilter('instructionsResults', $event)" /></div>
                  <div class="grid grid-cols-[4.5rem_minmax(0,1fr)] items-start gap-1.5"><span class="font-mono text-[11px] text-gray-400">input[1]</span><FieldDigest :field="event.input1" @filter="addArrayFilter('input1Results', $event)" /></div>
                </div>
                <div>
                  <p class="text-[11px] font-semibold uppercase text-gray-400">{{ t('admin.instructionAudit.decisionAndNotification') }}</p>
                  <button type="button" class="mt-1" :class="outcomePill(event.final_outcome)" @click="addArrayFilter('finalOutcomes', event.final_outcome)">{{ outcomeLabel(event.final_outcome) }}</button>
                  <button type="button" class="mt-1 block text-left text-xs font-medium text-gray-700 hover:underline dark:text-gray-300" @click="addArrayFilter('reasons', event.final_reason || event.reason)">{{ reasonLabel(event.final_reason || event.reason) }}</button>
                  <p class="mt-1 text-[11px] text-gray-400">{{ formatBytes(event.body_bytes) }} · v{{ event.config_version }} · {{ eventAuditLatency(event) }}ms · {{ evidenceStatusLabel(event.evidence_status) }}</p>
                  <div class="mt-2 flex flex-wrap gap-1">
                    <button type="button" @click="addArrayFilter('userNotifications', event.user_notification_status)"><span :class="notificationPill(event.user_notification_status)">{{ t('admin.instructionAudit.userNotification') }} · {{ notificationLabel(event.user_notification_status) }}</span></button>
                    <button type="button" @click="addArrayFilter('opsNotifications', event.ops_notification_status)"><span :class="notificationPill(event.ops_notification_status)">{{ t('admin.instructionAudit.opsNotification') }} · {{ notificationLabel(event.ops_notification_status) }}</span></button>
                  </div>
                </div>
              </div>
            </article>
          </div>
          <div class="hidden min-w-0 overflow-hidden xl:block">
          <table class="w-full table-fixed divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/70">
              <tr>
                <th class="table-th w-10">
                  <input type="checkbox" :checked="allVisibleEventsSelected" :aria-label="t('admin.instructionAudit.selectAllEvents')" @change="toggleAllVisibleEvents" />
                </th>
                <th class="table-th w-[15%]">{{ t('admin.instructionAudit.eventInfo') }}</th>
                <th class="table-th w-[15%]">{{ t('admin.instructionAudit.requestSubject') }}</th>
                <th class="table-th w-[13%]">{{ t('admin.instructionAudit.clientAndModel') }}</th>
                <th class="table-th w-[19%]">{{ t('admin.instructionAudit.validationResults') }}</th>
                <th class="table-th w-[16%]">{{ t('admin.instructionAudit.decisionAndNotification') }}</th>
                <th class="table-th w-[17%] text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-for="event in eventPage.items" :key="event.id" class="align-top">
                <td class="table-td">
                  <input type="checkbox" :checked="selectedEventIds.includes(event.id)" :aria-label="t('admin.instructionAudit.selectEvent', { id: event.id })" @change="toggleEventSelection(event.id)" />
                </td>
                <td class="table-td text-xs text-gray-500 dark:text-gray-400">
                  <div class="flex items-center gap-1.5">
                    <button type="button" class="font-semibold text-primary-700 hover:underline dark:text-primary-300" @click="setEventIDFilter(event.id)">{{ t('admin.instructionAudit.eventNumber', { id: event.id }) }}</button>
                    <button type="button" class="icon-btn h-6 w-6" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyText(String(event.id))"><Icon name="copy" size="xs" /></button>
                  </div>
                  <p class="mt-1">{{ formatDate(event.created_at) }}</p>
                  <div class="mt-1 flex min-w-0 items-center gap-1">
                    <button type="button" class="min-w-0 truncate font-mono text-[11px] text-primary-600 hover:underline dark:text-primary-400" :title="event.request_id" @click="setQueryFilter(event.request_id)">{{ event.request_id || '-' }}</button>
                    <button v-if="event.request_id" type="button" class="icon-btn h-6 w-6 shrink-0" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyText(event.request_id)"><Icon name="copy" size="xs" /></button>
                  </div>
                </td>
                <td class="table-td">
                  <button type="button" class="block max-w-full truncate text-left text-sm text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300" :title="event.user_email" @click="setQueryFilter(event.user_email)">{{ event.user_email || '-' }}</button>
                  <p class="mt-0.5 text-xs text-gray-400">user #{{ event.user_id || '-' }} · key #{{ event.api_key_id || '-' }}</p>
                  <button type="button" class="mt-2 block max-w-full truncate text-left text-xs font-medium text-gray-700 hover:text-primary-600 dark:text-gray-300" :title="event.group_name" @click="addArrayFilter('groupIds', event.group_id)">{{ event.group_name || '-' }} <span class="text-gray-400">#{{ event.group_id || '-' }}</span></button>
                </td>
                <td class="table-td">
                  <button type="button" :class="clientTypePill(event.client_type)" @click="addArrayFilter('clientTypes', event.client_type)">
                    {{ clientTypeLabel(event.client_type) }}
                  </button>
                  <button type="button" class="mt-2 block max-w-full truncate text-left font-mono text-xs text-gray-700 hover:text-primary-600 dark:text-gray-300" :title="event.model" @click="setQueryFilter(event.model)">{{ event.model || '-' }}</button>
                  <p v-if="event.client_user_agent" class="mt-1 max-w-full truncate font-mono text-[10px] text-gray-400" :title="event.client_user_agent">
                    {{ event.client_user_agent }}
                  </p>
                </td>
                <td class="table-td">
                  <div class="space-y-2">
                    <div class="grid grid-cols-[4.5rem_minmax(0,1fr)] items-start gap-1.5"><span class="font-mono text-[11px] text-gray-400">instructions</span><FieldDigest :field="event.instructions" @filter="addArrayFilter('instructionsResults', $event)" /></div>
                    <div class="grid grid-cols-[4.5rem_minmax(0,1fr)] items-start gap-1.5"><span class="font-mono text-[11px] text-gray-400">input[1]</span><FieldDigest :field="event.input1" @filter="addArrayFilter('input1Results', $event)" /></div>
                  </div>
                </td>
                <td class="table-td">
                  <button type="button" :class="outcomePill(event.final_outcome)" @click="addArrayFilter('finalOutcomes', event.final_outcome)">{{ outcomeLabel(event.final_outcome) }}</button>
                  <button type="button" class="mt-1 block text-left text-xs font-medium text-gray-700 hover:underline dark:text-gray-300" @click="addArrayFilter('reasons', event.final_reason || event.reason)">{{ reasonLabel(event.final_reason || event.reason) }}</button>
                  <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">{{ formatBytes(event.body_bytes) }} · v{{ event.config_version }} · {{ eventAuditLatency(event) }}ms<span v-if="event.ai_latency_ms != null"> · AI {{ event.ai_latency_ms }}ms</span></p>
                  <div class="mt-2 flex flex-wrap gap-1">
                    <button type="button" @click="addArrayFilter('userNotifications', event.user_notification_status)"><span :class="notificationPill(event.user_notification_status)">{{ t('admin.instructionAudit.userNotification') }} · {{ notificationLabel(event.user_notification_status) }}</span></button>
                    <button type="button" @click="addArrayFilter('opsNotifications', event.ops_notification_status)"><span :class="notificationPill(event.ops_notification_status)">{{ t('admin.instructionAudit.opsNotification') }} · {{ notificationLabel(event.ops_notification_status) }}</span></button>
                  </div>
                </td>
                <td class="table-td text-right">
                  <div class="flex flex-wrap justify-end gap-1">
                    <button type="button" class="btn btn-primary btn-sm" :disabled="!eventHasDigest(event) || !ruleSets.length" @click="openAddToRuleSetDialog(event)">
                      <Icon name="plus" size="sm" />
                      {{ t('admin.instructionAudit.quickAdd') }}
                    </button>
                    <button type="button" class="icon-btn" :title="t('admin.instructionAudit.reviewEvidence')" :aria-label="t('admin.instructionAudit.reviewEvidence')" @click="openEvidenceReview(event)"><Icon name="eye" size="sm" /></button>
                    <router-link :to="opsLogLink(event)" class="icon-btn" :title="t('admin.instructionAudit.viewSystemLog')" :aria-label="t('admin.instructionAudit.viewSystemLog')"><Icon name="externalLink" size="sm" /></router-link>
                    <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" :aria-label="t('common.delete')" @click="eventToDelete = event"><Icon name="trash" size="sm" /></button>
                  </div>
                  <p class="mt-1 text-[11px] text-gray-400">{{ evidenceStatusLabel(event.evidence_status) }}</p>
                </td>
              </tr>
            </tbody>
          </table>
          </div>
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
            <option value="revoked" disabled>{{ t('admin.instructionAudit.hashStatuses.revoked') }}</option>
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
        <div class="rounded-lg border border-gray-200 px-4 py-3 dark:border-dark-600">
          <div class="flex items-center justify-between gap-4">
            <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('admin.instructionAudit.allowEmptyFields') }}</span>
            <Toggle v-model="ruleDialog.allowEmptyFields" />
          </div>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.allowEmptyFieldsHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.userAllowlist') }}</label>
          <OpenAIFastPolicyUserSelector
            v-model="ruleDialog.allowedUserIds"
            :initial-users="ruleDialog.initialUsers"
          />
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.userAllowlistHint') }}</p>
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

    <BaseDialog :show="bindingDialog.show" :title="bindingDialog.editingId ? t('admin.instructionAudit.editBinding') : t('admin.instructionAudit.addBinding')" width="wide" @close="bindingDialog.show = false">
      <div class="space-y-4">
        <div>
          <div class="flex items-center justify-between gap-3">
            <label class="input-label mb-0">{{ t('admin.instructionAudit.groups') }}</label>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.selectedGroups', { count: bindingDialog.groupIds.length }) }}</span>
          </div>
          <div v-if="editingBinding" class="mt-2 flex items-center gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium text-gray-900 dark:text-white">{{ editingBinding.group_name }}</span>
              <span class="block text-xs text-gray-400">#{{ editingBinding.group_id }}</span>
            </span>
            <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400">{{ editingBinding.platform }}</span>
          </div>
          <template v-else>
            <input v-model.trim="bindingDialog.search" type="search" class="input mt-2" :placeholder="t('admin.instructionAudit.searchGroups')" />
            <div class="mt-2 flex items-center justify-end gap-3 text-xs">
              <button type="button" class="font-medium text-primary-600 hover:underline dark:text-primary-400" @click="selectVisibleGroups">{{ t('admin.instructionAudit.selectVisible') }}</button>
              <button type="button" class="font-medium text-gray-500 hover:underline dark:text-gray-400" @click="bindingDialog.groupIds = []">{{ t('admin.instructionAudit.clearSelection') }}</button>
            </div>
            <div class="mt-2 max-h-72 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
              <label v-for="group in filteredGroupOptions" :key="group.id" class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-3 py-2.5 last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700">
                <input v-model="bindingDialog.groupIds" type="checkbox" :value="group.id" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
                  <span class="block text-xs text-gray-400">#{{ group.id }}</span>
                </span>
                <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400">{{ group.platform }}</span>
                <span v-if="group.status !== 'active'" :class="statusPill('disabled')">{{ t('common.disabled') }}</span>
              </label>
              <p v-if="!filteredGroupOptions.length" class="px-3 py-8 text-center text-sm text-gray-400">{{ t('admin.instructionAudit.noMatchingGroups') }}</p>
            </div>
          </template>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.clientScope') }}</label>
          <div class="grid grid-cols-2 gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-700">
            <button
              v-for="mode in clientScopeModes"
              :key="mode.value"
              type="button"
              class="rounded px-3 py-2 text-sm font-medium transition-colors"
              :class="bindingDialog.clientScope === mode.value ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'"
              @click="bindingDialog.clientScope = mode.value"
            >
              {{ mode.label }}
            </button>
          </div>
          <div v-if="bindingDialog.clientScope === 'selected'" class="mt-3 grid gap-2 sm:grid-cols-2">
            <label v-for="client in clientOptions" :key="client.value" class="flex cursor-pointer items-start gap-3 rounded-md border border-gray-200 px-3 py-2.5 hover:border-primary-300 dark:border-dark-600 dark:hover:border-primary-700">
              <input v-model="bindingDialog.clientTypes" type="checkbox" :value="client.value" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ client.label }}</span>
                <span class="block break-all font-mono text-[11px] text-gray-400">{{ client.pattern }}</span>
              </span>
            </label>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.clientScopeHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.ruleSet') }}</label>
          <select v-model.number="bindingDialog.ruleSetId" class="input" :disabled="Boolean(bindingDialog.editingId)">
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
        <button type="button" class="btn btn-primary" :disabled="!canSaveBinding" @click="saveBinding">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <InstructionEvidenceReviewDialog
      :show="Boolean(evidenceReviewEvent)"
      :event="evidenceReviewEvent"
      :translation-enabled="runtimeConfig?.translation_enabled || false"
      :external-translation-enabled="runtimeConfig?.external_translation_enabled || false"
      @close="evidenceReviewEvent = null"
      @candidate="createCandidateFromReview"
    />

    <InstructionHashDetailDialog
      :show="Boolean(hashDetailID)"
      :hash-id="hashDetailID"
      :translation-enabled="runtimeConfig?.translation_enabled || false"
      :external-translation-enabled="runtimeConfig?.external_translation_enabled || false"
      @close="hashDetailID = null"
      @changed="refreshAll"
    />

    <BaseDialog :show="cleanupDialog.show" :title="t('admin.instructionAudit.clearLogs')" width="wide" @close="closeLogCleanupDialog">
      <div class="space-y-4">
        <div class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
          {{ t('admin.instructionAudit.clearLogsWarning') }}
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.clearRange') }}</label>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <button v-for="preset in cleanupPresets" :key="preset.value" type="button" class="rounded-md border px-3 py-2 text-sm font-medium" :class="cleanupDialog.preset === preset.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-300'" @click="setCleanupPreset(preset.value)">{{ preset.label }}</button>
          </div>
        </div>
        <div v-if="cleanupDialog.preset === 'custom'" class="grid gap-3 sm:grid-cols-2">
          <label class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.instructionAudit.fromTime') }}<input v-model="cleanupDialog.from" type="datetime-local" class="input mt-1" @change="invalidateCleanupPreview" /></label>
          <label class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.instructionAudit.toTime') }}<input v-model="cleanupDialog.to" type="datetime-local" class="input mt-1" @change="invalidateCleanupPreview" /></label>
        </div>
        <div class="rounded-md border border-gray-200 px-4 py-3 text-sm dark:border-dark-600">
          <p class="font-medium text-gray-900 dark:text-white">{{ t('admin.instructionAudit.currentFilterWillApply') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ cleanupFilterSummary }}</p>
        </div>
        <div v-if="cleanupDialog.preview" class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/60 dark:bg-amber-950/30">
          <p class="text-sm font-semibold text-amber-800 dark:text-amber-200">{{ t('admin.instructionAudit.clearPreviewCount', { count: cleanupDialog.preview.matched_count }) }}</p>
          <p class="mt-1 text-xs text-amber-700/80 dark:text-amber-300/80">{{ t('admin.instructionAudit.clearPreviewSnapshot', { id: cleanupDialog.preview.snapshot_max_id }) }}</p>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeLogCleanupDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-secondary" :disabled="cleanupDialog.previewing || !canPreviewCleanup" @click="previewLogCleanup">{{ cleanupDialog.previewing ? t('common.loading') : t('admin.instructionAudit.previewClear') }}</button>
        <button type="button" class="btn btn-danger" :disabled="cleanupDialog.deleting || !cleanupDialog.preview || cleanupDialog.preview.matched_count === 0" @click="confirmLogCleanup">{{ cleanupDialog.deleting ? t('admin.instructionAudit.deleting') : t('admin.instructionAudit.confirmClear') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="Boolean(addToRuleSetDialog.event)" :title="t('admin.instructionAudit.quickAddToRuleSet')" width="wide" @close="closeAddToRuleSetDialog">
      <div v-if="addToRuleSetDialog.event" class="space-y-4">
        <div class="rounded-md border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.instructionAudit.eventNumber', { id: addToRuleSetDialog.event.id }) }}</p>
          <p class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ addToRuleSetDialog.event.request_id || '-' }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.selectDigests') }}</label>
          <div class="space-y-2">
            <label v-for="source in availableEventSources(addToRuleSetDialog.event)" :key="source.value" class="flex cursor-pointer items-start gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600">
              <input v-model="addToRuleSetDialog.sources" type="checkbox" :value="source.value" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0"><span class="block text-sm font-medium text-gray-900 dark:text-white">{{ source.label }}</span><span class="mt-1 block break-all font-mono text-[11px] text-gray-500 dark:text-gray-400">{{ source.digest }}</span></span>
            </label>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.instructionAudit.selectRuleSet') }}</label>
          <select v-model.number="addToRuleSetDialog.ruleSetId" class="input">
            <option :value="0">{{ t('admin.instructionAudit.selectRuleSet') }}</option>
            <option v-for="rule in ruleSets" :key="rule.id" :value="rule.id">{{ rule.name }} · {{ rule.enabled ? t('common.enabled') : t('common.disabled') }}</option>
          </select>
        </div>
        <label class="flex cursor-pointer items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/60 dark:bg-amber-950/30">
          <input v-model="addToRuleSetDialog.confirmed" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-amber-800 dark:text-amber-200">{{ t('admin.instructionAudit.quickAddConfirmation') }}</span>
        </label>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeAddToRuleSetDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="!canQuickAdd || addToRuleSetDialog.saving" @click="confirmAddToRuleSet"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.quickAdd') }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(hashToDelete)" :title="t('admin.instructionAudit.deleteHash')" :message="t('admin.instructionAudit.deleteHashConfirm')" danger @confirm="deleteHash" @cancel="hashToDelete = null" />
    <ConfirmDialog :show="Boolean(ruleSetToDelete)" :title="t('admin.instructionAudit.deleteRuleSet')" :message="t('admin.instructionAudit.deleteRuleSetConfirm')" danger @confirm="deleteRuleSet" @cancel="ruleSetToDelete = null" />
    <ConfirmDialog :show="Boolean(eventToDelete)" :title="t('admin.instructionAudit.deleteEvent')" :message="t('admin.instructionAudit.deleteEventConfirm', { id: eventToDelete?.id })" danger @confirm="deleteSingleEvent" @cancel="eventToDelete = null" />
    <ConfirmDialog :show="batchDeleteRequested" :title="t('admin.instructionAudit.deleteSelectedTitle')" :message="t('admin.instructionAudit.deleteSelectedConfirm', { count: selectedEventIds.length })" danger @confirm="deleteSelectedEvents" @cancel="batchDeleteRequested = false" />

    <ConfirmDialog
      :show="Boolean(bindingToDelete)"
      :title="t('admin.instructionAudit.deleteBinding')"
      :message="t('admin.instructionAudit.deleteBindingConfirm')"
      danger
      @confirm="deleteBinding"
      @cancel="bindingToDelete = null"
    />
    <TotpStepUpDialog :controller="instructionStepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import OpenAIFastPolicyUserSelector from '@/views/admin/settings/OpenAIFastPolicyUserSelector.vue'
import InstructionEvidenceReviewDialog from './InstructionEvidenceReviewDialog.vue'
import InstructionAuditStatistics from './components/InstructionAuditStatistics.vue'
import InstructionAuditRuntimeConfig from './components/InstructionAuditRuntimeConfig.vue'
import InstructionAuditReasonPolicies from './components/InstructionAuditReasonPolicies.vue'
import InstructionHashDetailDialog from './components/InstructionHashDetailDialog.vue'
import { useClipboard } from '@/composables/useClipboard'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from './api'
import { instructionStatisticsFilters } from './filters'
import { resolveInstructionHashDigest } from './hash'
import type {
  InstructionClientType,
  InstructionDetectedClientType,
  InstructionDeletePreview,
  InstructionEventDeleteFilter,
  InstructionEventFilters,
  InstructionEventPage,
  InstructionEvent,
  InstructionFieldResult,
  InstructionGroupBinding,
  InstructionGroupOption,
  InstructionHashEntry,
  InstructionHashStatus,
  InstructionObservedSource,
  InstructionOverview,
  InstructionReasonPolicy,
  InstructionRuntimeConfig,
  InstructionStatistics,
  InstructionFinalOutcome,
  InstructionRuleSet,
  InstructionRuleSetUser,
  SaveInstructionHashRequest,
  UpdateInstructionReasonPolicyRequest,
  UpdateInstructionRuntimeConfigRequest,
} from './types'

type Tab = 'config' | 'policies' | 'rules' | 'hashes' | 'candidates' | 'events'
type CleanupPreset = '1h' | '24h' | '7d' | 'custom'
type EventDigestSource = 'instructions' | 'input1'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const instructionStepUp = useStepUp()
const route = useRoute()
const router = useRouter()
const activeTab = ref<Tab>('events')
const loading = ref(false)
const saving = ref(false)
const runtimeSaving = ref(false)
const statisticsLoading = ref(false)
const savingReason = ref('')
const overview = ref<InstructionOverview | null>(null)
const runtimeConfig = ref<InstructionRuntimeConfig | null>(null)
const reasonPolicies = ref<InstructionReasonPolicy[]>([])
const statistics = ref<InstructionStatistics | null>(null)
const runtimeError = ref('')
const policiesError = ref('')
const statisticsError = ref('')
const hashes = ref<InstructionHashEntry[]>([])
const ruleSets = ref<InstructionRuleSet[]>([])
const bindings = ref<InstructionGroupBinding[]>([])
const groupOptions = ref<InstructionGroupOption[]>([])
const hashStatusFilter = ref('')
const bindingToDelete = ref<InstructionGroupBinding | null>(null)
const hashToDelete = ref<InstructionHashEntry | null>(null)
const ruleSetToDelete = ref<InstructionRuleSet | null>(null)
const eventToDelete = ref<InstructionEvent | null>(null)
const selectedEventIds = ref<number[]>([])
const deletingEvents = ref(false)
const batchDeleteRequested = ref(false)
const eventPage = reactive<InstructionEventPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const evidenceReviewEvent = ref<InstructionEvent | null>(null)
const hashDetailID = ref<number | null>(null)
const retentionDays = ref(30)
const advancedFiltersOpen = ref(false)
const eventFilters = reactive({
  eventId: null as number | null,
  q: '', userId: null as number | null, model: '', range: '24h' as '1h' | '24h' | '7d' | 'custom', from: '', to: '',
  groupIds: [] as number[], reasons: [] as string[],
  finalOutcomes: [] as InstructionFinalOutcome[],
  clientTypes: [] as InstructionDetectedClientType[],
  instructionsResults: [] as string[], input1Results: [] as string[],
  userNotifications: [] as string[], opsNotifications: [] as string[],
})
const cleanupDialog = reactive<{
  show: boolean
  preset: CleanupPreset
  from: string
  to: string
  preview: InstructionDeletePreview | null
  filter: InstructionEventDeleteFilter | null
  previewing: boolean
  deleting: boolean
}>({ show: false, preset: '24h', from: '', to: '', preview: null, filter: null, previewing: false, deleting: false })
const addToRuleSetDialog = reactive<{
  event: InstructionEvent | null
  sources: EventDigestSource[]
  ruleSetId: number
  confirmed: boolean
  saving: boolean
}>({ event: null, sources: [], ruleSetId: 0, confirmed: false, saving: false })

const timeRanges = computed(() => [
  { value: '1h' as const, label: t('admin.instructionAudit.lastHour') },
  { value: '24h' as const, label: t('admin.instructionAudit.lastDay') },
  { value: '7d' as const, label: t('admin.instructionAudit.lastWeek') },
  { value: 'custom' as const, label: t('admin.instructionAudit.customRange') },
])
const cleanupPresets = computed<Array<{ value: CleanupPreset, label: string }>>(() => [
  { value: '1h', label: t('admin.instructionAudit.lastHour') },
  { value: '24h', label: t('admin.instructionAudit.lastDay') },
  { value: '7d', label: t('admin.instructionAudit.lastWeek') },
  { value: 'custom', label: t('admin.instructionAudit.customRange') },
])
const reasonOptions = ['hash_mismatch', 'fields_missing', 'field_invalid', 'invalid_json', 'request_too_large', 'structure_too_complex', 'parse_timeout', 'config_unavailable', 'group_not_allowed', 'client_not_allowed', 'ai_rejected', 'ai_uncertain', 'ai_error']
const outcomeOptions: InstructionFinalOutcome[] = ['blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass']
const fieldResultOptions = ['missing', 'invalid', 'mismatch', 'match', 'not_checked']
const notificationOptions = ['pending', 'processing', 'retry', 'sent', 'failed', 'suppressed', 'no_recipient']
const detectedClientTypes: readonly InstructionDetectedClientType[] = ['codex_vscode', 'codex_cli', 'codex_desktop', 'opencode', 'modelport_internal', 'other', 'unknown']
const clientOptions = computed<Array<{ value: InstructionDetectedClientType, label: string, pattern: string }>>(() => [
  { value: 'codex_vscode', label: t('admin.instructionAudit.clients.codex_vscode'), pattern: 'codex_vscode/ · codex_vscode_copilot/' },
  { value: 'codex_cli', label: t('admin.instructionAudit.clients.codex_cli'), pattern: 'codex_cli_rs/ · codex-tui/' },
  { value: 'codex_desktop', label: t('admin.instructionAudit.clients.codex_desktop'), pattern: 'Codex Desktop/ · codex_chatgpt_desktop/' },
  { value: 'opencode', label: t('admin.instructionAudit.clients.opencode'), pattern: 'opencode/' },
  { value: 'modelport_internal', label: t('admin.instructionAudit.clients.modelport_internal'), pattern: t('admin.instructionAudit.trustedInternalIdentity') },
  { value: 'other', label: t('admin.instructionAudit.clients.other'), pattern: t('admin.instructionAudit.otherClientPattern') },
  { value: 'unknown', label: t('admin.instructionAudit.clients.unknown'), pattern: t('admin.instructionAudit.unknownClientPattern') },
])
const clientScopeModes = computed(() => [
  { value: 'all' as const, label: t('admin.instructionAudit.allClients') },
  { value: 'selected' as const, label: t('admin.instructionAudit.selectedClients') },
])

const tabs = computed(() => [
  { value: 'config' as const, label: t('admin.instructionAudit.runtime.tab') },
  { value: 'policies' as const, label: t('admin.instructionAudit.policies.tab'), count: reasonPolicies.value.length },
  { value: 'rules' as const, label: t('admin.instructionAudit.rulesAndBindings'), count: bindings.value.length },
  { value: 'hashes' as const, label: t('admin.instructionAudit.hashLibrary'), count: hashes.value.filter((item) => item.status !== 'candidate').length },
  { value: 'candidates' as const, label: t('admin.instructionAudit.candidateHashes'), count: hashes.value.filter((item) => item.status === 'candidate').length },
  { value: 'events' as const, label: t('admin.instructionAudit.auditLogs'), count: eventPage.total },
])

const overviewStats = computed(() => [
  { label: t('admin.instructionAudit.activeHashes'), value: overview.value?.active_hash_count ?? 0 },
  { label: t('admin.instructionAudit.ruleSets'), value: overview.value?.rule_set_count ?? 0 },
  { label: t('admin.instructionAudit.auditedGroups'), value: overview.value?.audited_group_count ?? 0 },
  { label: t('admin.instructionAudit.effectiveGroups'), value: overview.value?.effective_group_count ?? 0 },
  { label: t('admin.instructionAudit.persistFailures'), value: overview.value?.persist_failure_count ?? 0 },
  { label: t('admin.instructionAudit.pendingEmails'), value: overview.value?.pending_email_count ?? 0 },
])

const runtimeHealthStats = computed(() => {
  const value = overview.value
  if (!value) return []
  return [
    { key: 'persisted', label: t('admin.instructionAudit.runtimeHealth.persisted'), value: value.persisted_outcome_count },
    { key: 'aggregated', label: t('admin.instructionAudit.runtimeHealth.aggregated'), value: value.aggregated_outcome_count },
    { key: 'expired', label: t('admin.instructionAudit.runtimeHealth.expiredAggregates'), value: value.expired_aggregate_event_count },
    { key: 'loss', label: t('admin.instructionAudit.runtimeHealth.statisticsLoss'), value: value.statistics_loss_count },
    {
      key: 'audit-latency', label: t('admin.instructionAudit.runtimeHealth.auditLatency'),
      value: `${value.audit_latency_p95_ms} / ${value.audit_latency_p99_ms} ms`,
      hint: t('admin.instructionAudit.runtimeHealth.samples', { count: value.audit_latency_sample_count }),
    },
    {
      key: 'ai-latency', label: t('admin.instructionAudit.runtimeHealth.aiLatency'),
      value: `${value.ai_latency_p95_ms} / ${value.ai_latency_p99_ms} ms`,
      hint: t('admin.instructionAudit.runtimeHealth.samples', { count: value.ai_latency_sample_count }),
    },
    {
      key: 'translation-backlog', label: t('admin.instructionAudit.runtimeHealth.translationBacklog'),
      value: value.translation_pending_count + value.translation_processing_count,
      hint: t('admin.instructionAudit.runtimeHealth.activeWorkers', { count: value.translation_active_workers }),
    },
    {
      key: 'translation-failures', label: t('admin.instructionAudit.runtimeHealth.translationFailures'),
      value: value.translation_failed_count,
      hint: t('admin.instructionAudit.runtimeHealth.workerFailures', { count: value.translation_worker_fail_total }),
    },
  ]
})

type EventArrayFilterKey = 'groupIds' | 'clientTypes' | 'reasons' | 'finalOutcomes' | 'instructionsResults' | 'input1Results' | 'userNotifications' | 'opsNotifications'

const activeFilterCount = computed(() =>
  (eventFilters.eventId ? 1 : 0)
  + (eventFilters.userId ? 1 : 0)
  + (eventFilters.model ? 1 : 0)
  + eventFilters.groupIds.length
  + eventFilters.clientTypes.length
  + eventFilters.reasons.length
  + eventFilters.finalOutcomes.length
  + eventFilters.instructionsResults.length
  + eventFilters.input1Results.length
  + eventFilters.userNotifications.length
  + eventFilters.opsNotifications.length
  + (eventFilters.range === 'custom' ? 1 : 0),
)

const filterChips = computed(() => {
  const chips: Array<{ key: string; label: string; remove: () => void }> = []
  if (eventFilters.eventId) chips.push({ key: 'event-id', label: t('admin.instructionAudit.eventNumber', { id: eventFilters.eventId }), remove: clearEventIDFilter })
  if (eventFilters.userId) chips.push({ key: 'user-id', label: `${t('admin.instructionAudit.userId')}: ${eventFilters.userId}`, remove: () => { eventFilters.userId = null; void applyEventFilters() } })
  if (eventFilters.model) chips.push({ key: 'model', label: `${t('admin.instructionAudit.model')}: ${eventFilters.model}`, remove: () => { eventFilters.model = ''; void applyEventFilters() } })
  for (const id of eventFilters.groupIds) {
    const group = groupOptions.value.find((item) => item.id === id)
    chips.push({ key: `group-${id}`, label: `${t('admin.instructionAudit.group')}: ${group?.name || `#${id}`}`, remove: () => removeArrayFilter('groupIds', id) })
  }
  for (const clientType of eventFilters.clientTypes) chips.push({ key: `client-${clientType}`, label: `${t('admin.instructionAudit.client')}: ${clientTypeLabel(clientType)}`, remove: () => removeArrayFilter('clientTypes', clientType) })
  for (const reason of eventFilters.reasons) chips.push({ key: `reason-${reason}`, label: reasonLabel(reason), remove: () => removeArrayFilter('reasons', reason) })
  for (const outcome of eventFilters.finalOutcomes) chips.push({ key: `outcome-${outcome}`, label: outcomeLabel(outcome), remove: () => removeArrayFilter('finalOutcomes', outcome) })
  for (const result of eventFilters.instructionsResults) chips.push({ key: `instructions-${result}`, label: `instructions: ${fieldResultLabel(result)}`, remove: () => removeArrayFilter('instructionsResults', result) })
  for (const result of eventFilters.input1Results) chips.push({ key: `input1-${result}`, label: `input[1]: ${fieldResultLabel(result)}`, remove: () => removeArrayFilter('input1Results', result) })
  for (const status of eventFilters.userNotifications) chips.push({ key: `user-${status}`, label: `${t('admin.instructionAudit.userNotification')}: ${notificationLabel(status)}`, remove: () => removeArrayFilter('userNotifications', status) })
  for (const status of eventFilters.opsNotifications) chips.push({ key: `ops-${status}`, label: `${t('admin.instructionAudit.opsNotification')}: ${notificationLabel(status)}`, remove: () => removeArrayFilter('opsNotifications', status) })
  return chips
})

const visibleHashes = computed(() => {
  if (activeTab.value === 'candidates') return hashes.value.filter((item) => item.status === 'candidate')
  return hashes.value.filter((item) => item.status !== 'candidate' && (!hashStatusFilter.value || item.status === hashStatusFilter.value))
})

const allVisibleEventsSelected = computed(() =>
  eventPage.items.length > 0 && eventPage.items.every((event) => selectedEventIds.value.includes(event.id)),
)

const canPreviewCleanup = computed(() => {
  if (cleanupDialog.previewing || cleanupDialog.deleting || !cleanupDialog.from || !cleanupDialog.to) return false
  const from = new Date(cleanupDialog.from)
  const to = new Date(cleanupDialog.to)
  return !Number.isNaN(from.getTime()) && !Number.isNaN(to.getTime()) && from < to
})

const cleanupFilterSummary = computed(() => {
  if (!cleanupDialog.from || !cleanupDialog.to) return t('admin.instructionAudit.clearRangeIncomplete')
  const additionalFilters = activeFilterCount.value + (eventFilters.q ? 1 : 0)
  return t('admin.instructionAudit.clearFilterSummary', {
    from: formatDate(cleanupDialog.from),
    to: formatDate(cleanupDialog.to),
    count: additionalFilters,
  })
})

const canQuickAdd = computed(() =>
  Boolean(addToRuleSetDialog.event)
  && addToRuleSetDialog.sources.length > 0
  && addToRuleSetDialog.ruleSetId > 0
  && addToRuleSetDialog.confirmed
  && !addToRuleSetDialog.saving,
)

const hashDialog = reactive({
  show: false,
  id: null as number | null,
  mode: 'digest' as 'digest' | 'plaintext',
  plaintext: '',
  validFrom: '',
  validUntil: '',
  form: emptyHashForm(),
})

const ruleDialog = reactive({
  show: false,
  id: null as number | null,
  name: '',
  description: '',
  enabled: true,
  allowEmptyFields: false,
  hashIds: [] as number[],
  allowedUserIds: [] as number[],
  initialUsers: [] as InstructionRuleSetUser[],
})
const bindingDialog = reactive({
  show: false,
  editingId: null as number | null,
  search: '',
  groupIds: [] as number[],
  ruleSetId: 0,
  clientScope: 'all' as 'all' | 'selected',
  clientTypes: [] as InstructionDetectedClientType[],
  enabled: true,
})
const editingBinding = computed(() => bindings.value.find((binding) => binding.id === bindingDialog.editingId) ?? null)

const filteredGroupOptions = computed(() => {
  const query = bindingDialog.search.trim().toLocaleLowerCase()
  if (!query) return groupOptions.value
  return groupOptions.value.filter((group) =>
    group.name.toLocaleLowerCase().includes(query)
    || group.platform.toLocaleLowerCase().includes(query)
    || String(group.id) === query,
  )
})

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

const canSaveBinding = computed(() =>
  !saving.value
  && bindingDialog.groupIds.length > 0
  && bindingDialog.ruleSetId > 0
  && (bindingDialog.clientScope === 'all' || bindingDialog.clientTypes.length > 0),
)

const FieldDigest = defineComponent({
  props: { field: { type: Object as () => InstructionFieldResult, required: true } },
  emits: ['filter'],
  setup(props, { emit }) {
    return () => h('div', { class: 'min-w-0 space-y-1.5' }, [
      props.field.result ? h('button', { type: 'button', class: statusPill(props.field.result), onClick: () => emit('filter', props.field.result) }, fieldResultLabel(props.field.result)) : null,
      props.field.sha256 ? h('button', { type: 'button', class: 'block max-w-44 truncate font-mono text-[11px] text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-300', title: props.field.sha256, onClick: () => emit('filter', props.field.result) }, compactDigest(props.field.sha256)) : null,
    ])
  },
})

function emptyHashForm(): SaveInstructionHashRequest {
  return { digest: '', name: '', note: '', observed_source: '', client_name: '', client_version: '', status: 'candidate' }
}

async function refreshAll() {
  loading.value = true
  runtimeError.value = ''
  policiesError.value = ''
  try {
    const results = await Promise.allSettled([
      instructionAuditAPI.getOverview(),
      instructionAuditAPI.getRuntimeConfig(),
      instructionAuditAPI.listReasonPolicies(),
      instructionAuditAPI.listHashes(),
      instructionAuditAPI.listRuleSets(),
      instructionAuditAPI.listGroupBindings(),
      instructionAuditAPI.listGroups(),
    ])
    const [overviewResult, configResult, policiesResult, hashesResult, rulesResult, bindingsResult, groupsResult] = results
    if (overviewResult.status === 'fulfilled') {
      overview.value = overviewResult.value
      retentionDays.value = overviewResult.value.evidence_retention_days || 30
    }
    if (configResult.status === 'fulfilled') runtimeConfig.value = configResult.value
    else runtimeError.value = errorMessage(configResult.reason)
    if (policiesResult.status === 'fulfilled') reasonPolicies.value = policiesResult.value
    else policiesError.value = errorMessage(policiesResult.reason)
    if (hashesResult.status === 'fulfilled') hashes.value = hashesResult.value
    if (rulesResult.status === 'fulfilled') ruleSets.value = rulesResult.value
    if (bindingsResult.status === 'fulfilled') bindings.value = bindingsResult.value
    if (groupsResult.status === 'fulfilled') groupOptions.value = groupsResult.value
    const coreFailure = [overviewResult, hashesResult, rulesResult, bindingsResult, groupsResult].find((result) => result.status === 'rejected')
    if (coreFailure?.status === 'rejected') appStore.showError(errorMessage(coreFailure.reason))
    await Promise.all([loadEvents(eventPage.page), loadStatistics()])
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    loading.value = false
  }
}

async function saveRuntimeConfig(payload: UpdateInstructionRuntimeConfigRequest) {
  if (runtimeSaving.value) return
  runtimeSaving.value = true
  try {
    runtimeConfig.value = await instructionStepUp.run(() => instructionAuditAPI.updateRuntimeConfig(payload))
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    runtimeSaving.value = false
  }
}

async function saveReasonPolicy(reason: string, payload: UpdateInstructionReasonPolicyRequest) {
  if (savingReason.value) return
  savingReason.value = reason
  try {
    await instructionStepUp.run(() => instructionAuditAPI.updateReasonPolicy(reason, payload))
    appStore.showSuccess(t('common.saved'))
    const [nextPolicies, nextOverview, nextRuntime] = await Promise.all([
      instructionAuditAPI.listReasonPolicies(),
      instructionAuditAPI.getOverview(),
      instructionAuditAPI.getRuntimeConfig(),
    ])
    reasonPolicies.value = nextPolicies
    overview.value = nextOverview
    runtimeConfig.value = nextRuntime
  } catch (error) {
    reportError(error)
  } finally {
    savingReason.value = ''
  }
}

async function loadEvents(page = 1) {
  const result = await instructionAuditAPI.listEvents({
    ...currentEventFilters(),
    page,
    page_size: eventPage.page_size,
  })
  Object.assign(eventPage, result)
  await syncEventFilterURL()
}

async function applyEventFilters() {
  try {
    await Promise.all([loadEvents(1), loadStatistics()])
  } catch (error) {
    appStore.showError(errorMessage(error))
  }
}

async function loadStatistics() {
  statisticsLoading.value = true
  statisticsError.value = ''
  try {
    statistics.value = await instructionAuditAPI.getStatistics(instructionStatisticsFilters(currentEventFilters()))
  } catch (error) {
    statisticsError.value = errorMessage(error)
  } finally {
    statisticsLoading.value = false
  }
}

function currentEventFilters(): InstructionEventFilters {
  const timeParams = eventTimeParams()
  return {
    event_id: eventFilters.eventId || undefined,
    q: eventFilters.q || undefined,
    user_id: eventFilters.userId || undefined,
    model: eventFilters.model || undefined,
    from: timeParams.from,
    to: timeParams.to,
    group_ids: joinFilter(eventFilters.groupIds),
    client_types: joinFilter(eventFilters.clientTypes),
    reasons: joinFilter(eventFilters.reasons),
    final_outcomes: joinFilter(eventFilters.finalOutcomes),
    instructions_results: joinFilter(eventFilters.instructionsResults),
    input1_results: joinFilter(eventFilters.input1Results),
    user_notifications: joinFilter(eventFilters.userNotifications),
    ops_notifications: joinFilter(eventFilters.opsNotifications),
  }
}

function eventTimeParams(): { from?: string; to?: string } {
  if (eventFilters.range === 'custom') {
    return {
      from: eventFilters.from ? new Date(eventFilters.from).toISOString() : undefined,
      to: eventFilters.to ? new Date(eventFilters.to).toISOString() : undefined,
    }
  }
  const milliseconds = eventFilters.range === '1h' ? 60 * 60_000 : eventFilters.range === '7d' ? 7 * 24 * 60 * 60_000 : 24 * 60 * 60_000
  return { from: new Date(Date.now() - milliseconds).toISOString() }
}

function joinFilter(values: Array<string | number>): string | undefined {
  return values.length ? values.join(',') : undefined
}

async function syncEventFilterURL() {
  const query: Record<string, string> = { tab: activeTab.value }
  if (activeTab.value === 'events') {
    query.range = eventFilters.range
    if (eventFilters.eventId) query.event_id = String(eventFilters.eventId)
    if (eventFilters.q) query.q = eventFilters.q
    if (eventFilters.userId) query.user_id = String(eventFilters.userId)
    if (eventFilters.model) query.model = eventFilters.model
    if (eventFilters.range === 'custom') {
      if (eventFilters.from) query.from = eventFilters.from
      if (eventFilters.to) query.to = eventFilters.to
    }
    if (eventFilters.groupIds.length) query.group_ids = eventFilters.groupIds.join(',')
    if (eventFilters.clientTypes.length) query.client_types = eventFilters.clientTypes.join(',')
    if (eventFilters.reasons.length) query.reasons = eventFilters.reasons.join(',')
    if (eventFilters.finalOutcomes.length) query.final_outcomes = eventFilters.finalOutcomes.join(',')
    if (eventFilters.instructionsResults.length) query.instructions_results = eventFilters.instructionsResults.join(',')
    if (eventFilters.input1Results.length) query.input1_results = eventFilters.input1Results.join(',')
    if (eventFilters.userNotifications.length) query.user_notifications = eventFilters.userNotifications.join(',')
    if (eventFilters.opsNotifications.length) query.ops_notifications = eventFilters.opsNotifications.join(',')
    if (eventPage.page > 1) query.page = String(eventPage.page)
    if (eventPage.page_size !== 20) query.page_size = String(eventPage.page_size)
  }
  await router.replace({ path: route.path, query })
}

function hydrateEventFiltersFromURL() {
  const tab = String(route.query.tab || '')
  if (['config', 'policies', 'rules', 'hashes', 'candidates', 'events'].includes(tab)) activeTab.value = tab as Tab
  const range = String(route.query.range || '24h')
  if (['1h', '24h', '7d', 'custom'].includes(range)) eventFilters.range = range as typeof eventFilters.range
  const eventID = Number(route.query.event_id)
  eventFilters.eventId = Number.isInteger(eventID) && eventID > 0 ? eventID : null
  eventFilters.q = String(route.query.q || '')
  const userID = Number(route.query.user_id)
  eventFilters.userId = Number.isInteger(userID) && userID > 0 ? userID : null
  eventFilters.model = String(route.query.model || '')
  eventFilters.from = String(route.query.from || '')
  eventFilters.to = String(route.query.to || '')
  eventFilters.groupIds = splitURLFilter(route.query.group_ids).map(Number).filter((id) => Number.isInteger(id) && id > 0)
  eventFilters.clientTypes = splitURLFilter(route.query.client_types).filter(isInstructionDetectedClientType)
  eventFilters.reasons = splitURLFilter(route.query.reasons)
  eventFilters.finalOutcomes = splitURLFilter(route.query.final_outcomes).filter(isInstructionFinalOutcome)
  eventFilters.instructionsResults = splitURLFilter(route.query.instructions_results)
  eventFilters.input1Results = splitURLFilter(route.query.input1_results)
  eventFilters.userNotifications = splitURLFilter(route.query.user_notifications)
  eventFilters.opsNotifications = splitURLFilter(route.query.ops_notifications)
  eventPage.page = Math.max(1, Number(route.query.page) || 1)
  eventPage.page_size = Math.min(100, Math.max(1, Number(route.query.page_size) || 20))
  advancedFiltersOpen.value = activeFilterCount.value > 0
}

function splitURLFilter(value: unknown): string[] {
  return String(value || '').split(',').map((item) => item.trim()).filter(Boolean)
}

function setTimeRange(range: typeof eventFilters.range) {
  eventFilters.range = range
  if (range !== 'custom') {
    eventFilters.from = ''
    eventFilters.to = ''
    void applyEventFilters()
  }
}

function setQueryFilter(value: string) {
  const next = String(value || '').trim()
  if (!next) return
  eventFilters.q = next
  void applyEventFilters()
}

function setEventIDFilter(id: number) {
  if (!Number.isInteger(id) || id <= 0) return
  eventFilters.eventId = id
  eventFilters.range = 'custom'
  eventFilters.from = ''
  eventFilters.to = ''
  void applyEventFilters()
}

function clearEventIDFilter() {
  eventFilters.eventId = null
  void applyEventFilters()
}

function addArrayFilter(key: EventArrayFilterKey, value: string | number | null | undefined) {
  if (value === null || value === undefined || value === '') return
  const list = eventFilters[key] as Array<string | number>
  if (!list.includes(value)) list.push(value)
  advancedFiltersOpen.value = true
  void applyEventFilters()
}

function removeArrayFilter(key: EventArrayFilterKey, value: string | number) {
  const list = eventFilters[key] as Array<string | number>
  const index = list.indexOf(value)
  if (index >= 0) list.splice(index, 1)
  void applyEventFilters()
}

function resetEventFilters() {
  Object.assign(eventFilters, {
    eventId: null, q: '', userId: null, model: '', range: '24h', from: '', to: '', groupIds: [], clientTypes: [], reasons: [],
    finalOutcomes: [],
    instructionsResults: [], input1Results: [], userNotifications: [], opsNotifications: [],
  })
  advancedFiltersOpen.value = false
  void applyEventFilters()
}

async function changeEventPageSize(pageSize: number) {
  eventPage.page_size = pageSize
  await loadEvents(1)
}

function toggleEventSelection(id: number) {
  const index = selectedEventIds.value.indexOf(id)
  if (index >= 0) selectedEventIds.value.splice(index, 1)
  else selectedEventIds.value.push(id)
}

function toggleAllVisibleEvents() {
  const visibleIDs = eventPage.items.map((event) => event.id)
  if (allVisibleEventsSelected.value) {
    selectedEventIds.value = selectedEventIds.value.filter((id) => !visibleIDs.includes(id))
    return
  }
  selectedEventIds.value = [...new Set([...selectedEventIds.value, ...visibleIDs])]
}

function requestBatchDeleteEvents() {
  if (!selectedEventIds.value.length) return
  batchDeleteRequested.value = true
}

async function reloadEventsAfterDelete() {
  await Promise.all([loadEvents(eventPage.page), loadStatistics()])
  if (!eventPage.items.length && eventPage.page > 1) await loadEvents(Math.max(1, eventPage.pages))
}

async function deleteSingleEvent() {
  if (!eventToDelete.value || deletingEvents.value) return
  deletingEvents.value = true
  try {
    const result = await instructionStepUp.run(() => instructionAuditAPI.deleteEvent(eventToDelete.value!.id))
    selectedEventIds.value = selectedEventIds.value.filter((id) => id !== eventToDelete.value?.id)
    eventToDelete.value = null
    appStore.showSuccess(t('admin.instructionAudit.eventsDeleted', { count: result.deleted_events }))
    await reloadEventsAfterDelete()
  } catch (error) {
    reportError(error)
  } finally {
    deletingEvents.value = false
  }
}

async function deleteSelectedEvents() {
  if (!selectedEventIds.value.length || deletingEvents.value) return
  batchDeleteRequested.value = false
  deletingEvents.value = true
  try {
    const result = await instructionStepUp.run(() => instructionAuditAPI.batchDeleteEvents([...selectedEventIds.value]))
    selectedEventIds.value = []
    appStore.showSuccess(t('admin.instructionAudit.eventsDeleted', { count: result.deleted_events }))
    await reloadEventsAfterDelete()
  } catch (error) {
    reportError(error)
  } finally {
    deletingEvents.value = false
  }
}

function openLogCleanupDialog() {
  cleanupDialog.show = true
  cleanupDialog.preview = null
  cleanupDialog.filter = null
  if (eventFilters.range === 'custom') {
    cleanupDialog.preset = 'custom'
    cleanupDialog.from = eventFilters.from
    cleanupDialog.to = eventFilters.to
    return
  }
  setCleanupPreset(eventFilters.range)
}

function closeLogCleanupDialog() {
  if (cleanupDialog.previewing || cleanupDialog.deleting) return
  cleanupDialog.show = false
  cleanupDialog.preview = null
  cleanupDialog.filter = null
}

function setCleanupPreset(preset: CleanupPreset) {
  cleanupDialog.preset = preset
  invalidateCleanupPreview()
  if (preset === 'custom') {
    cleanupDialog.from = eventFilters.range === 'custom' ? eventFilters.from : ''
    cleanupDialog.to = eventFilters.range === 'custom' ? eventFilters.to : ''
    return
  }
  const duration = preset === '1h' ? 60 * 60_000 : preset === '7d' ? 7 * 24 * 60 * 60_000 : 24 * 60 * 60_000
  const to = new Date()
  cleanupDialog.from = toDateTimeLocal(new Date(to.getTime() - duration).toISOString())
  cleanupDialog.to = toDateTimeLocal(to.toISOString())
}

function invalidateCleanupPreview() {
  cleanupDialog.preview = null
  cleanupDialog.filter = null
}

function buildCleanupFilter(): InstructionEventDeleteFilter | null {
  if (!canPreviewCleanup.value) return null
  return {
    event_id: eventFilters.eventId || undefined,
    q: eventFilters.q || undefined,
    user_id: eventFilters.userId || undefined,
    model: eventFilters.model || undefined,
    from: new Date(cleanupDialog.from).toISOString(),
    to: new Date(cleanupDialog.to).toISOString(),
    group_ids: [...eventFilters.groupIds],
    client_types: [...eventFilters.clientTypes],
    reasons: [...eventFilters.reasons],
    outcomes: [...eventFilters.finalOutcomes],
    instructions_results: [...eventFilters.instructionsResults],
    input1_results: [...eventFilters.input1Results],
    user_notifications: [...eventFilters.userNotifications],
    ops_notifications: [...eventFilters.opsNotifications],
  }
}

async function previewLogCleanup() {
  const filter = buildCleanupFilter()
  if (!filter) return
  cleanupDialog.previewing = true
  try {
    cleanupDialog.preview = await instructionStepUp.run(() => instructionAuditAPI.previewDeleteEvents(filter))
    cleanupDialog.filter = filter
  } catch (error) {
    reportError(error)
  } finally {
    cleanupDialog.previewing = false
  }
}

async function confirmLogCleanup() {
  if (!cleanupDialog.preview || !cleanupDialog.filter || cleanupDialog.deleting) return
  cleanupDialog.deleting = true
  try {
    const result = await instructionStepUp.run(() => instructionAuditAPI.deleteEventsByFilter(cleanupDialog.filter!, cleanupDialog.preview!))
    selectedEventIds.value = []
    appStore.showSuccess(t('admin.instructionAudit.eventsDeleted', { count: result.deleted_events }))
    cleanupDialog.show = false
    cleanupDialog.preview = null
    cleanupDialog.filter = null
    await Promise.all([loadEvents(1), loadStatistics()])
  } catch (error) {
    reportError(error)
  } finally {
    cleanupDialog.deleting = false
  }
}

function eventHasDigest(event: InstructionEvent): boolean {
  return availableEventSources(event).length > 0
}

function availableEventSources(event: InstructionEvent): Array<{ value: EventDigestSource, label: string, digest: string }> {
  const sources: Array<{ value: EventDigestSource, label: string, digest: string }> = []
  if (/^[0-9a-f]{64}$/i.test(event.instructions.sha256 || '')) sources.push({ value: 'instructions', label: t('admin.instructionAudit.fieldOne'), digest: event.instructions.sha256 })
  if (/^[0-9a-f]{64}$/i.test(event.input1.sha256 || '')) sources.push({ value: 'input1', label: t('admin.instructionAudit.fieldTwo'), digest: event.input1.sha256 })
  return sources
}

function openAddToRuleSetDialog(event: InstructionEvent) {
  const sources = availableEventSources(event)
  addToRuleSetDialog.event = event
  addToRuleSetDialog.sources = sources.map((source) => source.value)
  addToRuleSetDialog.ruleSetId = ruleSets.value.find((rule) => rule.enabled)?.id ?? ruleSets.value[0]?.id ?? 0
  addToRuleSetDialog.confirmed = false
}

function closeAddToRuleSetDialog() {
  if (addToRuleSetDialog.saving) return
  addToRuleSetDialog.event = null
  addToRuleSetDialog.sources = []
  addToRuleSetDialog.ruleSetId = 0
  addToRuleSetDialog.confirmed = false
}

async function confirmAddToRuleSet() {
  if (!canQuickAdd.value || !addToRuleSetDialog.event) return
  const eventID = addToRuleSetDialog.event.id
  addToRuleSetDialog.saving = true
  try {
    const result = await instructionStepUp.run(() => instructionAuditAPI.addEventToRuleSet(
      eventID,
      addToRuleSetDialog.ruleSetId,
      [...addToRuleSetDialog.sources],
    ))
    appStore.showSuccess(t('admin.instructionAudit.quickAddSuccess', {
      attached: result.attached_hashes,
      created: result.created_hashes,
      activated: result.activated_hashes,
    }))
    addToRuleSetDialog.saving = false
    closeAddToRuleSetDialog()
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    addToRuleSetDialog.saving = false
  }
}

function opsLogLink(event: InstructionEvent) {
  return {
    path: '/admin/ops',
    query: {
      system_log_q: `\"event_id\": ${event.id}`,
      system_log_range: '30d',
    },
    hash: '#ops-system-logs',
  }
}

async function copyText(value: string) {
  await copyToClipboard(value)
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
      raw_content: hashDialog.id || hashDialog.mode !== 'plaintext' ? undefined : hashDialog.plaintext,
      valid_from: fromDateTimeLocal(hashDialog.validFrom),
      valid_until: fromDateTimeLocal(hashDialog.validUntil),
    }
    if (hashDialog.id) {
      await instructionStepUp.run(() => instructionAuditAPI.updateHash(hashDialog.id!, {
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
      }))
    }
    else await instructionStepUp.run(() => instructionAuditAPI.createHash(payload))
    closeHashDialog()
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    hashDialog.plaintext = ''
    saving.value = false
  }
}

async function deleteHash() {
  if (!hashToDelete.value || saving.value) return
  saving.value = true
  try {
    await instructionStepUp.run(() => instructionAuditAPI.deleteHash(hashToDelete.value!.id))
    hashToDelete.value = null
    appStore.showSuccess(t('common.deleted'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

function openRuleSetDialog(rule?: InstructionRuleSet) {
  ruleDialog.show = true
  ruleDialog.id = rule?.id ?? null
  ruleDialog.name = rule?.name ?? ''
  ruleDialog.description = rule?.description ?? ''
  ruleDialog.enabled = rule?.enabled ?? true
  ruleDialog.allowEmptyFields = rule?.allow_empty_fields ?? false
  ruleDialog.hashIds = rule?.hashes.map((item) => item.id) ?? []
  ruleDialog.allowedUserIds = rule?.allowed_users.map((user) => user.id) ?? []
  ruleDialog.initialUsers = rule?.allowed_users.map((user) => ({ ...user })) ?? []
}

async function saveRuleSet() {
  if (!ruleDialog.name.trim()) return
  saving.value = true
  try {
    await instructionStepUp.run(() => instructionAuditAPI.saveRuleSet(ruleDialog.id, {
      name: ruleDialog.name,
      description: ruleDialog.description,
      enabled: ruleDialog.enabled,
      allow_empty_fields: ruleDialog.allowEmptyFields,
      hash_ids: ruleDialog.hashIds,
      allowed_user_ids: ruleDialog.allowedUserIds,
    }))
    ruleDialog.show = false
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

async function deleteRuleSet() {
  if (!ruleSetToDelete.value || saving.value) return
  saving.value = true
  try {
    await instructionStepUp.run(() => instructionAuditAPI.deleteRuleSet(ruleSetToDelete.value!.id))
    ruleSetToDelete.value = null
    appStore.showSuccess(t('common.deleted'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

function openBindingDialog(binding?: InstructionGroupBinding) {
  const clientTypes = binding?.client_types?.length ? binding.client_types : ['all'] as InstructionClientType[]
  const allClients = clientTypes.includes('all')
  Object.assign(bindingDialog, {
    show: true,
    editingId: binding?.id ?? null,
    search: '',
    groupIds: binding ? [binding.group_id] : [],
    ruleSetId: binding?.rule_set_id ?? ruleSets.value.find((rule) => rule.enabled)?.id ?? ruleSets.value[0]?.id ?? 0,
    clientScope: allClients ? 'all' : 'selected',
    clientTypes: allClients ? [] : clientTypes.filter(isInstructionDetectedClientType),
    enabled: binding?.enabled ?? true,
  })
}

function selectVisibleGroups() {
  const selected = new Set(bindingDialog.groupIds)
  for (const group of filteredGroupOptions.value) {
    selected.add(group.id)
  }
  bindingDialog.groupIds = [...selected]
}

async function saveBinding() {
  if (!canSaveBinding.value) return
  saving.value = true
  try {
    const clientTypes: InstructionClientType[] = bindingDialog.clientScope === 'all'
      ? ['all']
      : [...bindingDialog.clientTypes]
    await instructionStepUp.run(() => instructionAuditAPI.saveGroupBindings({
      group_ids: bindingDialog.groupIds,
      rule_set_id: bindingDialog.ruleSetId,
      client_types: clientTypes,
      enabled: bindingDialog.enabled,
    }))
    bindingDialog.show = false
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

async function setBindingEnabled(binding: InstructionGroupBinding, enabled: boolean) {
  saving.value = true
  try {
    await instructionStepUp.run(() => instructionAuditAPI.saveGroupBindings({
      group_ids: [binding.group_id],
      rule_set_id: binding.rule_set_id,
      client_types: binding.client_types?.length ? binding.client_types : ['all'],
      enabled,
    }))
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

function requestDeleteBinding(binding: InstructionGroupBinding) {
  bindingToDelete.value = binding
}

async function deleteBinding() {
  if (!bindingToDelete.value) return
  try {
    await instructionStepUp.run(() => instructionAuditAPI.deleteGroupBinding(bindingToDelete.value!.id))
    bindingToDelete.value = null
    appStore.showSuccess(t('common.deleted'))
    await refreshAll()
  } catch (error) {
    reportError(error)
  }
}

async function saveEvidenceRetention() {
  if (!Number.isInteger(retentionDays.value) || retentionDays.value < 1 || retentionDays.value > 3650) return
  saving.value = true
  try {
    overview.value = await instructionStepUp.run(() => instructionAuditAPI.updateEvidenceRetention(retentionDays.value))
    appStore.showSuccess(t('common.saved'))
  } catch (error) {
    reportError(error)
  } finally {
    saving.value = false
  }
}

function openEvidenceReview(event: InstructionEvent) {
  evidenceReviewEvent.value = event
}

async function createCandidateFromReview(source: 'instructions' | 'input1') {
  if (!evidenceReviewEvent.value) return
  try {
    await instructionStepUp.run(() => instructionAuditAPI.createCandidate(evidenceReviewEvent.value!.id, source))
    appStore.showSuccess(t('admin.instructionAudit.candidateAdded'))
    evidenceReviewEvent.value = null
    await refreshAll()
    activeTab.value = 'candidates'
  } catch (error) {
    reportError(error)
  }
}

function compactDigest(value: string): string {
  return value ? `${value.slice(0, 10)}…${value.slice(-8)}` : '-'
}

function formatDate(value?: string | null): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function eventAuditLatency(event: InstructionEvent): number {
  return event.audit_latency_ms ?? event.latency_ms ?? 0
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

function isInstructionDetectedClientType(value: string): value is InstructionDetectedClientType {
  return (detectedClientTypes as readonly string[]).includes(value)
}

function isInstructionFinalOutcome(value: string): value is InstructionFinalOutcome {
  return (outcomeOptions as readonly string[]).includes(value)
}

function clientTypeLabel(clientType: string): string {
  if (clientType === 'all') return t('admin.instructionAudit.allClients')
  return clientOptions.value.find((client) => client.value === clientType)?.label || clientType
}

function clientTypePill(clientType: string): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
  if (clientType === 'all') return `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300`
  if (clientType === 'codex_vscode') return `${base} bg-indigo-50 text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300`
  if (clientType === 'codex_cli') return `${base} bg-cyan-50 text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300`
  if (clientType === 'codex_desktop') return `${base} bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300`
  if (clientType === 'opencode') return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300`
  if (clientType === 'modelport_internal') return `${base} bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300`
  if (clientType === 'unknown') return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
  return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
}

function hashStatusLabel(status: InstructionHashStatus): string {
  if (status === 'active') return t('common.enabled')
  if (status === 'disabled') return t('common.disabled')
  if (status === 'expired') return t('admin.instructionAudit.expired')
  if (status === 'revoked') return t('admin.instructionAudit.hashStatuses.revoked')
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

function outcomeLabel(outcome: string): string {
  return t(`admin.instructionAudit.outcomes.${outcome}`, outcome)
}

function outcomePill(outcome: string): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
  if (outcome === 'blocked') return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
  if (outcome === 'policy_allow') return `${base} bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300`
  if (outcome === 'ai_pass') return `${base} bg-cyan-50 text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300`
  if (outcome === 'hash_pass') return `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300`
  return `${base} bg-indigo-50 text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300`
}

function formatBytes(value?: number | null): string {
  if (value == null) return '-'
  if (value < 1024) return `${value}B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)}KiB`
  return `${(value / 1024 / 1024).toFixed(1)}MiB`
}

function errorMessage(error: unknown): string {
  return extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error'))
}

function reportError(error: unknown) {
  if (!isStepUpCancelled(error)) appStore.showError(errorMessage(error))
}

function evidenceStatusLabel(status: string): string {
  return t(`admin.instructionAudit.evidenceStatuses.${status}`)
}

watch(activeTab, (tab) => {
  if (tab === 'events' && !eventPage.items.length) void loadEvents(1)
  else void syncEventFilterURL()
})

hydrateEventFiltersFromURL()
onMounted(refreshAll)
</script>

<style scoped>
.table-th {
  padding: 0.625rem;
  text-align: left;
  font-size: 0.6875rem;
  font-weight: 600;
  text-transform: uppercase;
  color: rgb(107 114 128);
}

.table-td {
  padding: 0.75rem 0.625rem;
  color: rgb(75 85 99);
}

:global(.dark) .table-th,
:global(.dark) .table-td {
  color: rgb(209 213 219);
}

.filter-fieldset {
  @apply min-w-0 rounded-md border border-gray-200 p-3 dark:border-dark-600;
}

.filter-legend {
  @apply px-1 text-xs font-semibold text-gray-700 dark:text-gray-200;
}

.filter-options {
  @apply mt-1 max-h-48 space-y-1 overflow-y-auto;
}

.filter-option {
  @apply flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700;
}

.filter-option input {
  @apply h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500;
}

.filter-chip {
  @apply inline-flex items-center gap-1 rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 hover:bg-primary-100 dark:bg-primary-950/50 dark:text-primary-300 dark:hover:bg-primary-900/60;
}
</style>
