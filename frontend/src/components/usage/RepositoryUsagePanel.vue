<template>
  <section class="card overflow-hidden" data-testid="repository-usage-panel">
    <button
      type="button"
      class="flex w-full items-center justify-between px-6 py-4 text-left font-medium text-gray-900 dark:text-gray-100"
      :aria-expanded="expanded"
      aria-controls="repository-usage-content"
      @click="expanded = !expanded"
    >
      <span>{{ t('usage.repositories.title') }}</span>
      <span aria-hidden="true">{{ expanded ? '−' : '+' }}</span>
    </button>
    <div v-if="expanded" id="repository-usage-content" class="border-t border-gray-100 dark:border-dark-700">
      <div class="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('usage.repositories.hint') }}</p>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          {{ t('common.refresh') }}
        </button>
      </div>
      <div v-if="error" role="alert" class="px-6 py-6 text-sm text-red-600 dark:text-red-400">{{ error }}</div>
      <div v-else class="overflow-x-auto" :aria-busy="loading">
        <table class="w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            <tr>
              <th class="px-6 py-3">Repository</th>
              <th class="px-6 py-3 text-right">{{ t('usage.totalRequests') }}</th>
              <th class="px-6 py-3 text-right">{{ t('usage.totalTokens') }}</th>
              <th class="px-6 py-3 text-right">{{ t('usage.repositories.cost') }}</th>
              <th class="px-6 py-3 text-right">{{ t('usage.repositories.actualCost') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <tr v-if="loading"><td colspan="5" class="px-6 py-8 text-center">{{ t('common.loading') }}</td></tr>
            <tr v-else-if="!result?.repositories.length"><td colspan="5" class="px-6 py-8 text-center text-gray-500">{{ t('usage.repositories.empty') }}</td></tr>
            <tr v-for="item in loading ? [] : result?.repositories" :key="item.repository">
              <td class="max-w-sm break-all px-6 py-3 font-medium text-gray-800 dark:text-gray-200">{{ item.repository }}</td>
              <td class="px-6 py-3 text-right tabular-nums">{{ item.requests.toLocaleString() }}</td>
              <td class="px-6 py-3 text-right tabular-nums">{{ item.total_tokens.toLocaleString() }}</td>
              <td class="px-6 py-3 text-right tabular-nums">${{ item.cost.toFixed(6) }}</td>
              <td class="px-6 py-3 text-right font-medium tabular-nums text-green-600 dark:text-green-400">${{ item.actual_cost.toFixed(6) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-end gap-3 px-6 py-3 text-sm">
        <span class="mr-auto text-xs text-gray-400">{{ t('usage.repositories.freshness') }}</span>
        <button type="button" class="btn btn-secondary" :disabled="loading || page <= 1" @click="page--">{{ t('usage.repositories.previous') }}</button>
        <span>{{ page }}</span>
        <button type="button" class="btn btn-secondary" :disabled="loading || !!error || !result?.has_more || page >= 1000" @click="page++">{{ t('common.next') }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getRepositoryUsage, type RepositoryStatsPage, type RepositoryUsageFilters } from '@/api/repositoryUsage'

const props = withDefaults(defineProps<{
  startDate: string
  endDate: string
  filters: RepositoryUsageFilters
  admin?: boolean
}>(), { admin: false })
const { t } = useI18n()
const expanded = ref(false)
const loading = ref(false)
const error = ref('')
const page = ref(1)
const result = ref<RepositoryStatsPage | null>(null)
let controller: AbortController | undefined
let requestSeq = 0

// Pass only supported report filters. Pagination and unrelated error-tab filters
// from the parent must not cause extra requests or fragment the report cache.
const params = computed<RepositoryUsageFilters>(() => ({
  start_date: props.startDate,
  end_date: props.endDate,
  user_id: props.admin ? props.filters.user_id : undefined,
  account_id: props.admin ? props.filters.account_id : undefined,
  api_key_id: props.filters.api_key_id || undefined,
  group_id: props.filters.group_id || undefined,
  model: props.filters.model || undefined,
  request_type: props.filters.request_type || undefined,
  stream: props.filters.stream,
  native_compaction_v2: props.filters.native_compaction_v2,
  billing_type: props.filters.billing_type,
  billing_mode: props.filters.billing_mode || undefined,
  upstream_model_mismatch: props.admin ? props.filters.upstream_model_mismatch : undefined,
}))
const filterKey = computed(() => JSON.stringify(params.value))

async function load() {
  if (!expanded.value) return
  controller?.abort()
  controller = new AbortController()
  const seq = ++requestSeq
  loading.value = true
  error.value = ''
  try {
    const response = await getRepositoryUsage({ ...params.value, page: page.value, page_size: 20 }, props.admin, controller.signal)
    if (seq === requestSeq) result.value = response
  } catch {
    if (seq === requestSeq && !controller.signal.aborted) {
      error.value = t('usage.repositories.loadFailed')
      result.value = null
    }
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

// A single watcher coalesces date/filter/page changes into one report request.
watch([expanded, filterKey, page], ([open, key], [, oldKey]) => {
  if (key !== oldKey && page.value !== 1) {
    page.value = 1
    return
  }
  if (open) void load()
  else {
    controller?.abort()
    requestSeq++
    loading.value = false
  }
})
onUnmounted(() => { requestSeq++; controller?.abort() })
</script>
