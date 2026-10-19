import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import LogsView from './LogsView.vue'
import { getLogs } from '@/api'
import type { LogPage } from '@/types'

vi.mock('@/api', () => ({ getLogs: vi.fn() }))
const entry = { id: 10, time: '2026-10-19T01:00:00Z', level: 'info' as const, category: 'checkin', source: 'scheduled', message: 'checkin completed <script>alert(1)</script>' }
const page: LogPage = { entries: [entry], total: 2, retained: 2, has_more: true, next_before: 10, accounts: [], models: [], max_entries: 20000, max_bytes: 20971520, storage_ok: true }
let wrapper: VueWrapper | undefined
const mock = vi.mocked(getLogs)
const button = (text: string) => wrapper!.findAll('button').find((node) => node.text().includes(text))!

describe('LogsView', () => {
  beforeEach(() => { vi.useFakeTimers(); mock.mockReset(); mock.mockResolvedValue(page) })
  afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

  it('renders escaped logs and combines filters with debounce', async () => {
    wrapper = mount(LogsView); await flushPromises()
    expect(wrapper.findAll('.log-entry')).toHaveLength(1)
    expect(wrapper.find('script').exists()).toBe(false)
    await wrapper.find('input[type="search"]').setValue('retry')
    await wrapper.findAll('select')[2]!.setValue('checkin')
    await vi.advanceTimersByTimeAsync(300); await flushPromises()
    expect(mock.mock.lastCall?.[0]).toMatchObject({ q: 'retry', category: 'checkin', limit: 100 })
    await button('重置筛选').trigger('click'); await vi.advanceTimersByTimeAsync(300); await flushPromises()
    expect(mock.mock.lastCall?.[0]).toMatchObject({ q: '', category: '' })
  })

  it('uses stable pagination and pauses polling while browsing history', async () => {
    wrapper = mount(LogsView); await flushPromises()
    mock.mockResolvedValue({ ...page, entries: [{ ...entry, id: 9 }], has_more: false })
    await button('加载更早').trigger('click'); await flushPromises()
    expect(mock.mock.lastCall?.[0]?.before).toBe(10)
    expect(wrapper.findAll('.log-entry')).toHaveLength(2)
    expect(wrapper.text()).toContain('已暂停自动刷新')
    const count = mock.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(mock).toHaveBeenCalledTimes(count)
  })

  it('polls, aborts on unmount, and ignores outdated responses after filters change', async () => {
    let resolve!: (value: LogPage) => void
    mock.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    wrapper = mount(LogsView)
    await wrapper.find('input[type="search"]').setValue('new')
    await vi.advanceTimersByTimeAsync(300); await flushPromises()
    resolve({ ...page, entries: [{ ...entry, message: 'stale response' }] }); await flushPromises()
    expect(wrapper.text()).not.toContain('stale response')
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(mock.mock.calls.length).toBeGreaterThan(2)
    const signal = mock.mock.lastCall?.[1]
    wrapper.unmount(); wrapper = undefined
    expect(signal?.aborted).toBe(true)
    const count = mock.mock.calls.length; await vi.advanceTimersByTimeAsync(10000)
    expect(mock).toHaveBeenCalledTimes(count)
  })

  it('shows errors, retry and storage warnings', async () => {
    mock.mockRejectedValueOnce(new Error('请求失败'))
    wrapper = mount(LogsView); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('请求失败')
    mock.mockResolvedValue({ ...page, entries: [], total: 0, storage_ok: false })
    await button('重试').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('没有符合筛选条件')
    expect(wrapper.text()).toContain('日志写入磁盘异常')
  })
})
