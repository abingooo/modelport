<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-8">
      <header class="border-b border-gray-200 pb-6 dark:border-dark-700">
        <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ t('modelPortStore.title') }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('modelPortStore.description') }}</p>
      </header>

      <section class="grid gap-4 md:grid-cols-2">
        <article class="flex min-h-44 flex-col border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
          <div class="flex h-10 w-10 items-center justify-center rounded-md bg-teal-50 text-teal-700 dark:bg-teal-950/40 dark:text-teal-300">
            <Icon name="creditCard" size="md" />
          </div>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('modelPortStore.balanceTitle') }}</h2>
          <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('modelPortStore.balanceDescription') }}</p>
          <button
            type="button"
            class="btn btn-secondary mt-auto self-start"
            :disabled="!paymentEnabled"
            @click="openRecharge"
          >
            <Icon name="creditCard" size="sm" />
            {{ paymentEnabled ? t('modelPortStore.recharge') : t('modelPortStore.rechargeUnavailable') }}
          </button>
        </article>

        <article class="flex min-h-44 flex-col border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
          <div class="flex h-10 w-10 items-center justify-center rounded-md bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
            <Icon name="gift" size="md" />
          </div>
          <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('modelPortStore.redeemTitle') }}</h2>
          <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('modelPortStore.redeemDescription') }}</p>
          <button type="button" class="btn btn-secondary mt-auto self-start" @click="router.push('/redeem')">
            <Icon name="gift" size="sm" />
            {{ t('modelPortStore.redeem') }}
          </button>
        </article>
      </section>

      <section class="border-y border-dashed border-gray-300 py-14 text-center dark:border-dark-600">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-md bg-gray-100 text-gray-400 dark:bg-dark-800 dark:text-gray-500">
          <Icon name="inbox" size="lg" />
        </div>
        <h2 class="mt-4 text-base font-semibold text-gray-800 dark:text-gray-200">{{ t('modelPortStore.emptyTitle') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('modelPortStore.emptyDescription') }}</p>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const paymentEnabled = computed(() => appStore.cachedPublicSettings?.payment_enabled === true)

function openRecharge() {
  if (!paymentEnabled.value) return
  router.push({ path: '/purchase', query: { tab: 'recharge' } })
}
</script>
