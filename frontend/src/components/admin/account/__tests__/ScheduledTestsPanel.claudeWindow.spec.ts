import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'

enableAutoUnmount(afterEach)

const { create, listByAccount } = vi.hoisted(() => ({ create: vi.fn(), listByAccount: vi.fn() }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/api/admin', () => ({ adminAPI: { scheduledTests: { create, listByAccount } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))

async function openPanel(allowClaudeWindow: boolean) {
  const wrapper = shallowMount(ScheduledTestsPanel, {
    props: { show: false, accountId: 1, modelOptions: [{ value: 'claude-sonnet-4-6', label: 'Sonnet' }], allowClaudeWindow },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  const add = wrapper.findAll('button').find(button => button.text().includes('admin.scheduledTests.addPlan'))!
  await add.trigger('click')
  return wrapper
}

describe('Claude window startup plans', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listByAccount.mockResolvedValue([])
    create.mockResolvedValue({ id: 1 })
  })

  it('is opt-in and creates a window plan without requiring a cron expression', async () => {
    const wrapper = await openPanel(true)
    expect(wrapper.findAllComponents(Toggle)[0].props('modelValue')).toBe(false)
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'claude-sonnet-4-6')
    wrapper.findAllComponents(Toggle)[0].vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.scheduledTests.cronExpression')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.autoRecoverHelp')
    const save = wrapper.findAll('button').find(button => button.text().includes('common.save'))!
    expect(save.attributes('disabled')).toBeUndefined()
    await save.trigger('click')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      account_id: 1, mode: 'claude_window', model_id: 'claude-sonnet-4-6', auto_recover: false,
    }))
    // Older servers require cron and must reject this mode rather than run a
    // regular connectivity test every minute when they ignore the mode field.
    expect(create.mock.calls[0][0]).not.toHaveProperty('cron_expression')
  })

  it('hides the mode for unsupported accounts', async () => {
    const wrapper = await openPanel(false)
    expect(wrapper.text()).not.toContain('admin.scheduledTests.claudeWindow')
    expect(wrapper.text()).toContain('admin.scheduledTests.cronExpression')
  })

  it('clears the opt-in when closing and changing accounts', async () => {
    const wrapper = await openPanel(true)
    wrapper.findAllComponents(Toggle)[0].vm.$emit('update:modelValue', true)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, accountId: 2, allowClaudeWindow: false })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('admin.scheduledTests.addPlan'))!.trigger('click')
    expect(wrapper.text()).toContain('admin.scheduledTests.cronExpression')
  })
})
