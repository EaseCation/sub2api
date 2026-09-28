import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIPrioritySettings from '../OpenAIPrioritySettings.vue'
const mocks = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/openaiPriority', () => ({ getOpenAIPrioritySettings: mocks.get, saveOpenAIPrioritySettings: mocks.save }))
vi.mock('@/stores', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('OpenAI priority settings', () => {
 beforeEach(() => {
   vi.clearAllMocks()
   mocks.get.mockResolvedValue({ enabled: false, threshold: 2 })
   mocks.save.mockImplementation(async (v) => v)
 })
 it('loads default-off state and saves an enabled threshold', async () => {
   const wrapper = mount(OpenAIPrioritySettings)
   await flushPromises()
   expect((wrapper.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
   await wrapper.get('input[type="checkbox"]').setValue(true)
   expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe('2')
   await wrapper.get('input[type="number"]').setValue(3)
   await wrapper.get('button').trigger('click')
   await flushPromises()
   expect(mocks.save).toHaveBeenCalledWith({ enabled: true, threshold: 3 })
   wrapper.unmount()
 })
 it('rejects fractional or negative thresholds', async () => {
   const wrapper = mount(OpenAIPrioritySettings)
   await flushPromises()
   await wrapper.get('input[type="checkbox"]').setValue(true)
   for (const value of [-1, 1.5]) {
     await wrapper.get('input[type="number"]').setValue(value)
     await wrapper.get('button').trigger('click')
   }
   expect(mocks.save).not.toHaveBeenCalled()
   expect(mocks.showError).toHaveBeenCalledTimes(2)
   wrapper.unmount()
 })
 it('prevents saving defaults when loading fails', async () => {
   mocks.get.mockRejectedValue(new Error('offline'))
   const wrapper = mount(OpenAIPrioritySettings)
   await flushPromises()
   expect(wrapper.find('[role="alert"]').exists()).toBe(true)
   expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
   wrapper.unmount()
 })
})
