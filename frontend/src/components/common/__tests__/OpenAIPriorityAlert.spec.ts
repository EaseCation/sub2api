import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIPriorityAlert from '../OpenAIPriorityAlert.vue'

const { getStatus } = vi.hoisted(() => ({ getStatus: vi.fn() }))
vi.mock('@/api/openaiPriority', () => ({ getOpenAIPriorityStatus: getStatus }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('OpenAI priority alert', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getStatus.mockReset()
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  })
  afterEach(() => vi.useRealTimers())
  it('shows for signed-out visitors and clears after recovery without a reload', async () => {
    getStatus.mockResolvedValueOnce({ active: true }).mockResolvedValue({ active: false })
    const wrapper = mount(OpenAIPriorityAlert)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('openaiPriority.alert')
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
    const calls = getStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(getStatus).toHaveBeenCalledTimes(calls)
  })
  it('retains a known active alert on temporary API failure', async () => {
    getStatus.mockResolvedValueOnce({ active: true }).mockRejectedValue(new Error('offline'))
    const wrapper = mount(OpenAIPriorityAlert)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
