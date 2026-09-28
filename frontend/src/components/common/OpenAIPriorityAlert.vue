<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getOpenAIPriorityStatus } from '@/api/openaiPriority'

const { t } = useI18n()
const active = ref(false)
let timer: ReturnType<typeof setInterval> | undefined
let request: AbortController | undefined
let stopped = false

async function refresh() {
  if (stopped || request || document.visibilityState === 'hidden') return
  request = new AbortController()
  try {
    const status = await getOpenAIPriorityStatus(request.signal)
    if (!stopped) active.value = status.active
  } catch {
    // Keep the last known state during a transient network failure.
  } finally {
    request = undefined
  }
}

onMounted(() => {
  void refresh()
  timer = setInterval(refresh, 5000)
  document.addEventListener('visibilitychange', refresh)
  window.addEventListener('openai-priority-updated', refresh)
})
onBeforeUnmount(() => {
  stopped = true
  clearInterval(timer)
  request?.abort()
  document.removeEventListener('visibilitychange', refresh)
  window.removeEventListener('openai-priority-updated', refresh)
})
</script>

<template>
  <div v-if="active" aria-hidden="true" class="h-40" />
  <aside v-if="active" role="alert" class="fixed inset-x-4 bottom-4 z-[80] mx-auto max-w-4xl rounded-xl border border-amber-400 bg-amber-50 px-5 py-4 text-sm text-amber-950 shadow-lg dark:border-amber-600 dark:bg-amber-950 dark:text-amber-100">
    <strong class="mb-1 block">{{ t('admin.settings.openaiPriority.title') }}</strong>
    {{ t('admin.settings.openaiPriority.alert') }}
  </aside>
</template>
