<template>
  <section class="mt-4 space-y-3 border-t border-gray-200 pt-4 dark:border-dark-400">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.groups.sessionSkill.title') }}</h4>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.sessionSkill.description') }}</p>
      </div>
      <Toggle
        :model-value="enabled"
        :aria-label="t('admin.groups.sessionSkill.title')"
        @update:model-value="$emit('update:enabled', $event)"
      />
    </div>
    <label v-if="enabled" class="block">
      <span class="input-label">{{ t('admin.groups.sessionSkill.label') }}</span>
      <textarea
        :value="skill"
        class="input mt-1 min-h-36 w-full"
        rows="6"
        maxlength="16000"
        required
        :placeholder="t('admin.groups.sessionSkill.placeholder')"
        @input="$emit('update:skill', ($event.target as HTMLTextAreaElement).value)"
      />
      <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.sessionSkill.hint') }}</span>
    </label>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

defineProps<{ enabled: boolean; skill: string }>()
defineEmits<{ 'update:enabled': [value: boolean]; 'update:skill': [value: string] }>()
const { t } = useI18n()
</script>
