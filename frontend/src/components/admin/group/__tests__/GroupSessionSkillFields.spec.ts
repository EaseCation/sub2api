import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import GroupSessionSkillFields from '../GroupSessionSkillFields.vue'

describe('GroupSessionSkillFields', () => {
  it('shows instructions only when enabled and preserves draft text when toggled', async () => {
    const wrapper = mount(GroupSessionSkillFields, {
      props: { enabled: false, skill: 'Check Git and request confirmation' },
      global: { plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false })] }
    })
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:enabled')).toEqual([[true]])
    await wrapper.setProps({ enabled: true })
    expect(wrapper.get('textarea').element.value).toBe('Check Git and request confirmation')
    expect(wrapper.get('textarea').attributes('required')).toBeDefined()
    await wrapper.get('textarea').setValue('Updated instructions')
    expect(wrapper.emitted('update:skill')).toEqual([['Updated instructions']])
    await wrapper.setProps({ skill: 'Updated instructions', enabled: false })
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.setProps({ enabled: true })
    expect(wrapper.get('textarea').element.value).toBe('Updated instructions')
    wrapper.unmount()
  })
})
