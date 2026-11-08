import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RepositoryUsagePanel from '../RepositoryUsagePanel.vue'

const getRepositoryUsage = vi.hoisted(() => vi.fn())
vi.mock('@/api/repositoryUsage', () => ({ getRepositoryUsage }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const wrappers: VueWrapper[] = []
function mountPanel(props = {}) {
  const wrapper = mount(RepositoryUsagePanel, { props: { startDate: '2026-11-01', endDate: '2026-11-07', filters: {}, ...props } })
  wrappers.push(wrapper)
  return wrapper
}
const response = (repository = 'github.com/owner/repo') => ({
  repositories: [{ repository, requests: 2, total_tokens: 500, cost: 1.5, actual_cost: 0.75 }],
  page: 1, page_size: 20, has_more: true, generated_at: '2026-11-07T12:00:00Z',
})

describe('RepositoryUsagePanel', () => {
  beforeEach(() => { getRepositoryUsage.mockReset(); getRepositoryUsage.mockResolvedValue(response()) })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('does no work while collapsed, then uses the period and supported filters', async () => {
    const wrapper = mountPanel({ admin: true, filters: { user_id: 7, api_key_id: 8, model: 'gpt-5', billing_mode: 'token', error_phase: 'forward' } })
    await flushPromises()
    expect(getRepositoryUsage).not.toHaveBeenCalled()
    await wrapper.setProps({ endDate: '2026-11-08' })
    expect(getRepositoryUsage).not.toHaveBeenCalled()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(getRepositoryUsage).toHaveBeenCalledTimes(1)
    expect(getRepositoryUsage).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-11-01', end_date: '2026-11-08', user_id: 7, api_key_id: 8, model: 'gpt-5', billing_mode: 'token', page: 1, page_size: 20,
    }), true, expect.any(AbortSignal))
    expect(getRepositoryUsage.mock.calls[0][0]).not.toHaveProperty('error_phase')
    expect(wrapper.text()).toContain('github.com/owner/repo')
    expect(wrapper.text()).toContain('$0.750000')
  })

  it('resets pagination once when the period changes and ignores unrelated filters', async () => {
    const wrapper = mountPanel()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').at(-1)!.trigger('click')
    await flushPromises()
    expect(getRepositoryUsage.mock.lastCall?.[0].page).toBe(2)
    await wrapper.setProps({ endDate: '2026-11-09', filters: { group_id: 4 } })
    await flushPromises()
    expect(getRepositoryUsage).toHaveBeenCalledTimes(3)
    expect(getRepositoryUsage.mock.lastCall?.[0]).toMatchObject({ page: 1, end_date: '2026-11-09', group_id: 4 })
    await wrapper.setProps({ filters: { group_id: 4, sort_by: 'created_at' } })
    await flushPromises()
    expect(getRepositoryUsage).toHaveBeenCalledTimes(3)
  })

  it('omits administrator scopes for user reports', async () => {
    const wrapper = mountPanel({ filters: { user_id: 999, account_id: 888, upstream_model_mismatch: true } })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    const [params, admin] = getRepositoryUsage.mock.calls[0]
    expect(admin).toBe(false)
    expect(params.user_id).toBeUndefined()
    expect(params.account_id).toBeUndefined()
    expect(params.upstream_model_mismatch).toBeUndefined()
  })

  it('aborts hidden work and ignores an old response after filters change', async () => {
    let finishOld!: (value: ReturnType<typeof response>) => void
    getRepositoryUsage.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    const wrapper = mountPanel()
    await wrapper.find('button').trigger('click')
    const oldSignal = getRepositoryUsage.mock.calls[0][2] as AbortSignal
    await wrapper.setProps({ startDate: '2026-11-02' })
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    finishOld(response('github.com/stale/repo'))
    await flushPromises()
    expect(wrapper.text()).not.toContain('github.com/stale/repo')
    expect(wrapper.text()).toContain('github.com/owner/repo')
    await wrapper.find('button').trigger('click')
    await wrapper.setProps({ startDate: '2026-11-03' })
    await flushPromises()
    expect(getRepositoryUsage).toHaveBeenCalledTimes(2)
  })

  it('shows query errors and disables next-page navigation', async () => {
    getRepositoryUsage.mockRejectedValue(new Error('unavailable'))
    const wrapper = mountPanel()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('usage.repositories.loadFailed')
    expect(wrapper.findAll('button').at(-1)!.attributes('disabled')).toBeDefined()
  })
})
