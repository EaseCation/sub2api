<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { getOpenAIPrioritySettings, saveOpenAIPrioritySettings } from '@/api/openaiPriority'

const { t } = useI18n()
const app = useAppStore()
const form = reactive({ enabled: false, threshold: 2 })
const loaded = ref(false)
const saving = ref(false)
const loadError = ref(false)
async function load() {
  loadError.value = false
  try { Object.assign(form, await getOpenAIPrioritySettings()); loaded.value = true }
  catch { loadError.value = true }
}
onMounted(load)
async function save() {
  if (!loaded.value || saving.value) return
  if (!Number.isInteger(form.threshold) || form.threshold < 0 || form.threshold > 1000000) {
    app.showError(t('admin.settings.openaiPriority.invalidThreshold'))
    return
  }
  saving.value = true
  try {
    Object.assign(form, await saveOpenAIPrioritySettings({ ...form }))
    app.showSuccess(t('admin.settings.openaiPriority.saved'))
  } catch { app.showError(t('admin.settings.openaiPriority.saveFailed')) }
  finally { saving.value = false }
}
</script>

<template>
  <section class="card p-6 space-y-4">
    <div>
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.openaiPriority.title') }}</h2>
      <p class="input-hint">{{ t('admin.settings.openaiPriority.description') }}</p>
    </div>
    <div v-if="loadError" role="alert">
      {{ t('admin.settings.openaiPriority.loadFailed') }}
      <button type="button" class="btn btn-secondary ml-2" @click="load">{{ t('admin.settings.openaiPriority.retry') }}</button>
    </div>
    <fieldset :disabled="!loaded || saving" class="space-y-4">
      <label class="flex items-center gap-2">
        <input v-model="form.enabled" type="checkbox" />
        {{ t('admin.settings.openaiPriority.enabled') }}
      </label>
      <div v-if="form.enabled">
        <label for="openai-priority-threshold" class="input-label">{{ t('admin.settings.openaiPriority.threshold') }}</label>
        <input id="openai-priority-threshold" v-model.number="form.threshold" type="number" min="0" max="1000000" step="1" class="input max-w-xs" />
      </div>
      <button type="button" class="btn btn-primary" @click="save">{{ t('admin.settings.openaiPriority.save') }}</button>
    </fieldset>
  </section>
</template>
