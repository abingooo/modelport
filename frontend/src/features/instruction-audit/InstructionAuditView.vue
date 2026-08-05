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
                  <th class="table-th">{{ t('admin.instructionAudit.group') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.platform') }}</th>
                  <th class="table-th">{{ t('admin.instructionAudit.ruleSet') }}</th>
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
                    <div class="flex items-center gap-3">
                      <Toggle :model-value="binding.enabled" :disabled="saving" @update:model-value="setBindingEnabled(binding, $event)" />
                      <span v-if="binding.enabled && !binding.effective" :class="statusPill('invalid')">{{ t('admin.instructionAudit.ineffectiveRule') }}</span>
                    </div>
                  </td>
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
        <div class="space-y-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div class="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.auditLogs') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.eventCount', { count: eventPage.total }) }}</p>
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

          <div v-if="advancedFiltersOpen" class="grid gap-3 lg:grid-cols-2 xl:grid-cols-4">
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
              <legend class="filter-legend">{{ t('admin.instructionAudit.reason') }}</legend>
              <div class="filter-options">
                <label v-for="reason in reasonOptions" :key="reason" class="filter-option">
                  <input v-model="eventFilters.reasons" type="checkbox" :value="reason" />
                  <span>{{ reasonLabel(reason) }}</span>
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
        <div v-if="eventPage.items.length" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/70">
              <tr>
                <th class="table-th">{{ t('admin.instructionAudit.time') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.user') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.group') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.model') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.fieldOne') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.fieldTwo') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.reason') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.notification') }}</th>
                <th class="table-th">{{ t('admin.instructionAudit.evidence') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-for="event in eventPage.items" :key="event.id" class="align-top">
                <td class="table-td whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">
                  {{ formatDate(event.created_at) }}
                  <button type="button" class="mt-1 block font-mono text-[11px] text-primary-600 hover:underline dark:text-primary-400" @click="setQueryFilter(event.request_id)">{{ event.request_id || `#${event.id}` }}</button>
                </td>
                <td class="table-td">
                  <button type="button" class="block text-left text-sm text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300" @click="setQueryFilter(event.user_email)">{{ event.user_email || '-' }}</button>
                  <p class="text-xs text-gray-400">
                    <button type="button" class="hover:text-primary-600" @click="setQueryFilter(String(event.user_id || ''))">#{{ event.user_id || '-' }}</button>
                    /
                    <button type="button" class="hover:text-primary-600" @click="setQueryFilter(String(event.api_key_id || ''))">key #{{ event.api_key_id || '-' }}</button>
                  </p>
                </td>
                <td class="table-td">
                  <button type="button" class="text-left text-sm text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300" @click="addArrayFilter('groupIds', event.group_id)">{{ event.group_name || '-' }}</button>
                  <p class="text-xs text-gray-400">#{{ event.group_id || '-' }}</p>
                </td>
                <td class="table-td"><button type="button" class="font-mono text-xs hover:text-primary-600" @click="setQueryFilter(event.model)">{{ event.model }}</button></td>
                <td class="table-td"><FieldDigest :field="event.instructions" @filter="addArrayFilter('instructionsResults', $event)" /></td>
                <td class="table-td"><FieldDigest :field="event.input1" @filter="addArrayFilter('input1Results', $event)" /></td>
                <td class="table-td text-xs"><button type="button" class="text-left hover:text-primary-600" @click="addArrayFilter('reasons', event.reason)">{{ reasonLabel(event.reason) }}</button><p class="mt-1 text-gray-400 dark:text-gray-500">v{{ event.config_version }} · {{ event.latency_ms }}ms</p></td>
                <td class="table-td">
                  <div class="space-y-1.5 whitespace-nowrap">
                    <button type="button" class="block" @click="addArrayFilter('userNotifications', event.user_notification_status)"><span :class="notificationPill(event.user_notification_status)">{{ t('admin.instructionAudit.userNotification') }} · {{ notificationLabel(event.user_notification_status) }}</span></button>
                    <button type="button" class="block" @click="addArrayFilter('opsNotifications', event.ops_notification_status)"><span :class="notificationPill(event.ops_notification_status)">{{ t('admin.instructionAudit.opsNotification') }} · {{ notificationLabel(event.ops_notification_status) }}</span></button>
                  </div>
                </td>
                <td class="table-td">
                  <button type="button" class="btn btn-secondary btn-sm whitespace-nowrap" @click="openEvidenceReview(event)">
                    <Icon name="eye" size="sm" />
                    {{ t('admin.instructionAudit.reviewEvidence') }}
                  </button>
                  <p class="mt-1 text-[11px] text-gray-400">{{ evidenceStatusLabel(event.evidence_status) }}</p>
                </td>
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
          <div class="flex items-center justify-between gap-3">
            <label class="input-label mb-0">{{ t('admin.instructionAudit.groups') }}</label>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.selectedGroups', { count: bindingDialog.groupIds.length }) }}</span>
          </div>
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
        <button type="button" class="btn btn-primary" :disabled="saving || !bindingDialog.groupIds.length || !bindingDialog.ruleSetId" @click="saveBinding">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <InstructionEvidenceReviewDialog
      :show="Boolean(evidenceReviewEvent)"
      :event="evidenceReviewEvent"
      @close="evidenceReviewEvent = null"
      @candidate="createCandidateFromReview"
    />

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
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import InstructionEvidenceReviewDialog from './InstructionEvidenceReviewDialog.vue'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from './api'
import { resolveInstructionHashDigest } from './hash'
import type {
  InstructionEventPage,
  InstructionEvent,
  InstructionFieldResult,
  InstructionGroupBinding,
  InstructionGroupOption,
  InstructionHashEntry,
  InstructionHashStatus,
  InstructionObservedSource,
  InstructionOverview,
  InstructionRuleSet,
  SaveInstructionHashRequest,
} from './types'

type Tab = 'rules' | 'hashes' | 'candidates' | 'events'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()
const activeTab = ref<Tab>('rules')
const loading = ref(false)
const saving = ref(false)
const overview = ref<InstructionOverview | null>(null)
const hashes = ref<InstructionHashEntry[]>([])
const ruleSets = ref<InstructionRuleSet[]>([])
const bindings = ref<InstructionGroupBinding[]>([])
const groupOptions = ref<InstructionGroupOption[]>([])
const hashStatusFilter = ref('')
const bindingToDelete = ref<InstructionGroupBinding | null>(null)
const eventPage = reactive<InstructionEventPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const evidenceReviewEvent = ref<InstructionEvent | null>(null)
const retentionDays = ref(30)
const advancedFiltersOpen = ref(false)
const eventFilters = reactive({
  q: '', range: '24h' as '1h' | '24h' | '7d' | 'custom', from: '', to: '',
  groupIds: [] as number[], reasons: [] as string[],
  instructionsResults: [] as string[], input1Results: [] as string[],
  userNotifications: [] as string[], opsNotifications: [] as string[],
})

const timeRanges = computed(() => [
  { value: '1h' as const, label: t('admin.instructionAudit.lastHour') },
  { value: '24h' as const, label: t('admin.instructionAudit.lastDay') },
  { value: '7d' as const, label: t('admin.instructionAudit.lastWeek') },
  { value: 'custom' as const, label: t('admin.instructionAudit.customRange') },
])
const reasonOptions = ['hash_mismatch', 'fields_missing', 'field_invalid', 'invalid_json', 'request_too_large', 'structure_too_complex', 'parse_timeout', 'config_unavailable']
const fieldResultOptions = ['missing', 'invalid', 'mismatch', 'match', 'not_checked']
const notificationOptions = ['pending', 'processing', 'retry', 'sent', 'failed', 'suppressed', 'no_recipient']

const tabs = computed(() => [
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

type EventArrayFilterKey = 'groupIds' | 'reasons' | 'instructionsResults' | 'input1Results' | 'userNotifications' | 'opsNotifications'

const activeFilterCount = computed(() =>
  eventFilters.groupIds.length
  + eventFilters.reasons.length
  + eventFilters.instructionsResults.length
  + eventFilters.input1Results.length
  + eventFilters.userNotifications.length
  + eventFilters.opsNotifications.length
  + (eventFilters.range === 'custom' ? 1 : 0),
)

const filterChips = computed(() => {
  const chips: Array<{ key: string; label: string; remove: () => void }> = []
  for (const id of eventFilters.groupIds) {
    const group = groupOptions.value.find((item) => item.id === id)
    chips.push({ key: `group-${id}`, label: `${t('admin.instructionAudit.group')}: ${group?.name || `#${id}`}`, remove: () => removeArrayFilter('groupIds', id) })
  }
  for (const reason of eventFilters.reasons) chips.push({ key: `reason-${reason}`, label: reasonLabel(reason), remove: () => removeArrayFilter('reasons', reason) })
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
const bindingDialog = reactive({ show: false, search: '', groupIds: [] as number[], ruleSetId: 0, enabled: true })

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

const FieldDigest = defineComponent({
  props: { field: { type: Object as () => InstructionFieldResult, required: true } },
  emits: ['filter'],
  setup(props, { emit }) {
    return () => h('div', { class: 'min-w-40 space-y-1.5' }, [
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
  try {
    const [nextOverview, nextHashes, nextRules, nextBindings, nextGroups] = await Promise.all([
      instructionAuditAPI.getOverview(),
      instructionAuditAPI.listHashes(),
      instructionAuditAPI.listRuleSets(),
      instructionAuditAPI.listGroupBindings(),
      instructionAuditAPI.listGroups(),
    ])
    overview.value = nextOverview
    retentionDays.value = nextOverview.evidence_retention_days || 30
    hashes.value = nextHashes
    ruleSets.value = nextRules
    bindings.value = nextBindings
    groupOptions.value = nextGroups
    await loadEvents(eventPage.page)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

async function loadEvents(page = 1) {
  const timeParams = eventTimeParams()
  const result = await instructionAuditAPI.listEvents({
    page,
    page_size: eventPage.page_size,
    q: eventFilters.q || undefined,
    from: timeParams.from,
    to: timeParams.to,
    group_ids: joinFilter(eventFilters.groupIds),
    reasons: joinFilter(eventFilters.reasons),
    instructions_results: joinFilter(eventFilters.instructionsResults),
    input1_results: joinFilter(eventFilters.input1Results),
    user_notifications: joinFilter(eventFilters.userNotifications),
    ops_notifications: joinFilter(eventFilters.opsNotifications),
  })
  Object.assign(eventPage, result)
  await syncEventFilterURL()
}

async function applyEventFilters() {
  await loadEvents(1)
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
    if (eventFilters.q) query.q = eventFilters.q
    if (eventFilters.range === 'custom') {
      if (eventFilters.from) query.from = eventFilters.from
      if (eventFilters.to) query.to = eventFilters.to
    }
    if (eventFilters.groupIds.length) query.group_ids = eventFilters.groupIds.join(',')
    if (eventFilters.reasons.length) query.reasons = eventFilters.reasons.join(',')
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
  if (['rules', 'hashes', 'candidates', 'events'].includes(tab)) activeTab.value = tab as Tab
  const range = String(route.query.range || '24h')
  if (['1h', '24h', '7d', 'custom'].includes(range)) eventFilters.range = range as typeof eventFilters.range
  eventFilters.q = String(route.query.q || '')
  eventFilters.from = String(route.query.from || '')
  eventFilters.to = String(route.query.to || '')
  eventFilters.groupIds = splitURLFilter(route.query.group_ids).map(Number).filter((id) => Number.isInteger(id) && id > 0)
  eventFilters.reasons = splitURLFilter(route.query.reasons)
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
    q: '', range: '24h', from: '', to: '', groupIds: [], reasons: [],
    instructionsResults: [], input1Results: [], userNotifications: [], opsNotifications: [],
  })
  advancedFiltersOpen.value = false
  void applyEventFilters()
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
  Object.assign(bindingDialog, { show: true, search: '', groupIds: [], ruleSetId: ruleSets.value.find((rule) => rule.enabled)?.id ?? ruleSets.value[0]?.id ?? 0, enabled: true })
}

function selectVisibleGroups() {
  const selected = new Set(bindingDialog.groupIds)
  for (const group of filteredGroupOptions.value) {
    selected.add(group.id)
  }
  bindingDialog.groupIds = [...selected]
}

async function saveBinding() {
  if (!bindingDialog.groupIds.length || !bindingDialog.ruleSetId) return
  saving.value = true
  try {
    await instructionAuditAPI.saveGroupBindings({
      group_ids: bindingDialog.groupIds,
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

async function setBindingEnabled(binding: InstructionGroupBinding, enabled: boolean) {
  saving.value = true
  try {
    await instructionAuditAPI.saveGroupBindings({
      group_ids: [binding.group_id],
      rule_set_id: binding.rule_set_id,
      enabled,
    })
    appStore.showSuccess(t('common.saved'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
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
    await instructionAuditAPI.deleteGroupBinding(bindingToDelete.value.id)
    bindingToDelete.value = null
    appStore.showSuccess(t('common.deleted'))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function saveEvidenceRetention() {
  if (!Number.isInteger(retentionDays.value) || retentionDays.value < 1 || retentionDays.value > 3650) return
  saving.value = true
  try {
    overview.value = await instructionAuditAPI.updateEvidenceRetention(retentionDays.value)
    appStore.showSuccess(t('common.saved'))
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
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
    await instructionAuditAPI.createCandidate(evidenceReviewEvent.value.id, source)
    appStore.showSuccess(t('admin.instructionAudit.candidateAdded'))
    evidenceReviewEvent.value = null
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
