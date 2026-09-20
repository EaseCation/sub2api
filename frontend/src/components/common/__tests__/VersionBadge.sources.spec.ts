import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import VersionBadge from '../VersionBadge.vue'

const { app, update } = vi.hoisted(() => ({
  app: {
    versionLoading: false, currentVersion: '0.2.8', latestVersion: '0.2.8', hasUpdate: false,
    buildType: 'release', releaseInfo: null, versionWarning: '',
    officialVersion: { version: '9.0.0' }, fetchVersion: vi.fn(), clearVersionCache: vi.fn()
  },
  update: vi.fn().mockResolvedValue({ need_restart: false })
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({ useAuthStore: () => ({ isAdmin: true }), useAppStore: () => app }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('@/api/admin/system', () => ({
  performUpdate: update, restartService: vi.fn(), getRollbackVersions: vi.fn(), rollback: vi.fn()
}))
enableAutoUnmount(afterEach)
beforeEach(() => { app.hasUpdate = false; update.mockClear() })

describe('version sources', () => {
  it('shows the official version without an official update button', async () => {
    const wrapper = mount(VersionBadge, { global: { stubs: { Icon: true } } })
    await wrapper.get('button').trigger('click')
    const official = wrapper.get('[data-test="official-version"]')
    expect(official.text()).toContain('v9.0.0')
    expect(official.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('JunxuanB/sub2api')
    expect(wrapper.findAll('button').some(b => b.text().includes('version.updateNow'))).toBe(false)
  })
  it('offers the existing update action only for a fork update', async () => {
    app.hasUpdate = true
    const wrapper = mount(VersionBadge, { global: { stubs: { Icon: true } } })
    await wrapper.get('button').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('version.updateNow'))!.trigger('click')
    expect(update).toHaveBeenCalledOnce()
  })
})
