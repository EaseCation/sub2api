<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { getOpenAIAccountGuardSettings, saveOpenAIAccountGuardSettings, type OpenAIAccountGuardSettings as GuardSettings } from '@/api/openaiAccountGuard'

const { t } = useI18n()
const appStore = useAppStore()
const settings = ref<GuardSettings | null>(null)
const exemptions = ref('')
const saving = ref(false)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    settings.value = await getOpenAIAccountGuardSettings()
    exemptions.value = (settings.value.exempt_paths ?? []).join('\n')
  } catch {
    error.value = t('admin.settings.openaiAccountGuard.loadFailed')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!settings.value || saving.value) return
  const cfg = settings.value
  if (!Number.isInteger(cfg.min_available_accounts) || cfg.min_available_accounts < 1 || cfg.min_available_accounts > 100000 ||
      !Number.isInteger(cfg.check_interval_seconds) || cfg.check_interval_seconds < 1 || cfg.check_interval_seconds > 3600 || !cfg.message.trim()) {
    error.value = t('admin.settings.openaiAccountGuard.invalid')
    return
  }
  saving.value = true
  error.value = ''
  try {
    settings.value = await saveOpenAIAccountGuardSettings({
      ...cfg,
      exempt_paths: exemptions.value.split('\n').map(line => line.trim()).filter(Boolean),
    })
    exemptions.value = settings.value.exempt_paths.join('\n')
    appStore.showSuccess(t('admin.settings.openaiAccountGuard.saved'))
    window.dispatchEvent(new Event('openai-account-guard-updated'))
  } catch {
    error.value = t('admin.settings.openaiAccountGuard.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="card" aria-labelledby="openai-account-guard-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="openai-account-guard-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiAccountGuard.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiAccountGuard.description') }}</p>
    </div>
    <div class="space-y-5 p-6">
      <p v-if="loading" role="status">{{ t('admin.settings.openaiAccountGuard.loading') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button v-if="!settings && !loading" type="button" class="btn btn-secondary" @click="load">{{ t('admin.settings.openaiAccountGuard.retry') }}</button>
      <fieldset v-if="settings" :disabled="saving" class="space-y-5">
        <div class="flex items-center justify-between gap-4">
          <label id="openai-account-guard-enabled" class="font-medium">{{ t('admin.settings.openaiAccountGuard.enabled') }}</label>
          <Toggle v-model="settings.enabled" aria-labelledby="openai-account-guard-enabled" />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="guard-threshold" class="mb-2 block text-sm font-medium">{{ t('admin.settings.openaiAccountGuard.threshold') }}</label>
            <input id="guard-threshold" v-model.number="settings.min_available_accounts" type="number" min="1" max="100000" step="1" class="input" />
          </div>
          <div>
            <label for="guard-interval" class="mb-2 block text-sm font-medium">{{ t('admin.settings.openaiAccountGuard.interval') }}</label>
            <input id="guard-interval" v-model.number="settings.check_interval_seconds" type="number" min="1" max="3600" step="1" class="input" />
          </div>
        </div>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiAccountGuard.cacheHint') }}</p>
        <div>
          <label for="guard-message" class="mb-2 block text-sm font-medium">{{ t('admin.settings.openaiAccountGuard.message') }}</label>
          <textarea id="guard-message" v-model="settings.message" rows="3" class="input" />
        </div>
        <details>
          <summary class="cursor-pointer text-sm font-medium">{{ t('admin.settings.openaiAccountGuard.exemptions') }}</summary>
          <label for="guard-exemptions" class="my-2 block text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiAccountGuard.exemptionsHint') }}</label>
          <textarea id="guard-exemptions" v-model="exemptions" rows="6" spellcheck="false" class="input font-mono text-sm" />
        </details>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">{{ t(saving ? 'admin.settings.openaiAccountGuard.saving' : 'admin.settings.openaiAccountGuard.save') }}</button>
        </div>
      </fieldset>
    </div>
  </section>
</template>
