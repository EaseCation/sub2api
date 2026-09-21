<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { getOpenAIAccountGuardStatus } from '@/api/openaiAccountGuard'

const message = ref('')
let timer: ReturnType<typeof setInterval> | undefined
let pending = false
let disposed = false

async function refresh() {
  if (pending || disposed || document.visibilityState === 'hidden') return
  pending = true
  try {
    const status = await getOpenAIAccountGuardStatus()
    if (!disposed) message.value = status.locked ? status.message : ''
  } catch {
    // Keep the last known notice during a temporary network failure.
  } finally {
    pending = false
  }
}

onMounted(() => {
  refresh()
  timer = setInterval(refresh, 30000)
  document.addEventListener('visibilitychange', refresh)
  window.addEventListener('openai-account-guard-updated', refresh)
})
onBeforeUnmount(() => {
  disposed = true
  if (timer) clearInterval(timer)
  document.removeEventListener('visibilitychange', refresh)
  window.removeEventListener('openai-account-guard-updated', refresh)
})
</script>

<template>
  <div v-if="message" role="alert" class="fixed bottom-4 left-4 right-4 z-50 mx-auto max-w-3xl whitespace-pre-wrap break-words rounded-xl border border-amber-300 bg-amber-50 px-5 py-4 text-sm font-medium text-amber-950 shadow-lg dark:border-amber-700 dark:bg-amber-950 dark:text-amber-100">
    {{ message }}
  </div>
</template>
