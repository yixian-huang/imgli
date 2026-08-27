import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { GroupsPage, daysFieldToSec, lifecycleBadges, secToDaysField } from './GroupsPage'
import { useGlobal } from '../../../store'

function jsonRes(body: unknown, status = 200): Response {
  return { ok: status < 400, status, json: () => Promise.resolve(body) } as unknown as Response
}
const env = (data: unknown) => ({ status: true, message: 'ok', data })
const GB = 1024 ** 3
const MB = 1024 ** 2

const groups = [
  { id: 1, name: '默认组', is_default: true, is_guest: false, storage_quota: 10 * GB, max_file_size: 20 * MB, bandwidth_quota_month: 5 * GB, rate_per_minute: 20, rate_per_hour: 200, rate_per_day: 1000, allowed_exts: ['png', 'jpg'], allowed_policy_ids: [1], max_expires_in: 7 * 86400, force_max_age_days: 7, created_at: '', user_count: 2 },
  { id: 2, name: 'VIP', is_default: false, is_guest: false, storage_quota: 100 * GB, max_file_size: 50 * MB, bandwidth_quota_month: 50 * GB, rate_per_minute: 60, rate_per_hour: 600, rate_per_day: 5000, allowed_exts: ['png'], allowed_policy_ids: [1], max_expires_in: 30 * 86400, force_max_age_days: 7, created_at: '', user_count: 0 },
]
const policies = [{ id: 1, name: '本地默认', driver: 'local', config: '{"root":"/data"}', cdn_domain: '', path_template: '', enabled: true, created_at: '', file_count: 0, used_bytes: 0 }]

let created: unknown = null
let patched: { id: string; body: unknown } | null = null
let deleted: string | null = null
function mockBackend() {
  created = null
  patched = null
  deleted = null
  vi.stubGlobal('fetch', vi.fn((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url)
    const m = init?.method
    if (u.includes('/admin/policies')) return Promise.resolve(jsonRes(env({ items: policies })))
    if (u.match(/\/admin\/groups\/\d+/) && m === 'PATCH') {
      patched = { id: u.split('/').pop()!, body: JSON.parse(String(init!.body)) }
      return Promise.resolve(jsonRes(env({ ...groups[1], ...(patched.body as object) })))
    }
    if (u.match(/\/admin\/groups\/\d+/) && m === 'DELETE') {
      deleted = u.split('/').pop()!
      return Promise.resolve(jsonRes(env({ id: 2, deleted: true })))
    }
    if (u.includes('/admin/groups') && m === 'POST') {
      created = JSON.parse(String(init!.body))
      return Promise.resolve(jsonRes(env({ id: 9 })))
    }
    if (u.includes('/admin/groups')) return Promise.resolve(jsonRes(env({ items: groups })))
    return Promise.resolve(jsonRes(env(null)))
  }))
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/admin/groups']}>
        <GroupsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

it('列表:组名与用户数', async () => {
  mockBackend()
  renderPage()
  expect(await screen.findByText('默认组')).toBeInTheDocument()
  expect(screen.getByText('VIP')).toBeInTheDocument()
})

it('内置组:名称锁定,无删除按钮', async () => {
  mockBackend()
  renderPage()
  await userEvent.click(await screen.findByText('默认组'))
  expect(screen.getByLabelText('组名')).toBeDisabled()
  expect(screen.queryByRole('button', { name: '删除' })).not.toBeInTheDocument()
})

it('编辑 VIP:差异提交只发改动字段', async () => {
  useGlobal.setState({ toasts: [] })
  mockBackend()
  renderPage()
  await userEvent.click(await screen.findByText('VIP'))
  const quota = screen.getByLabelText('容量配额（GB）')
  await userEvent.clear(quota)
  await userEvent.type(quota, '200')
  await userEvent.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => expect(patched).toEqual({ id: '2', body: { storage_quota: 200 * GB } }))
  await waitFor(() => expect(useGlobal.getState().toasts.some((x) => x.message === '用户组已保存')).toBe(true))
})

it('未改动点保存:空差异不发 PATCH，toast 提示无更改', async () => {
  useGlobal.setState({ toasts: [] })
  mockBackend()
  renderPage()
  await userEvent.click(await screen.findByText('VIP'))
  await userEvent.click(screen.getByRole('button', { name: '保存' }))
  await new Promise((r) => setTimeout(r, 50))
  const patchedCalls = (fetch as ReturnType<typeof vi.fn>).mock.calls.filter(
    (c) => /\/admin\/groups\/\d+/.test(String(c[0])) && (c[1] as RequestInit)?.method === 'PATCH',
  )
  expect(patchedCalls).toHaveLength(0)
  expect(useGlobal.getState().toasts.some((x) => x.message === '没有需要保存的更改')).toBe(true)
})

it('lifecycleBadges: max 与 force 分开展示，不用 min 盖住 max', () => {
  const t = (k: string, v?: Record<string, string | number>) => {
    if (k === 'adminA.lifecycleBadgeMax') return `≤${v?.days}d`
    if (k === 'adminA.lifecycleBadgeForce') return `force ${v?.days}d`
    if (k === 'adminA.lifecycleBadgeRetention') return `trash ${v?.days}d`
    return k
  }
  const badges = lifecycleBadges(
    { max_expires_in: 30 * 86400, force_max_age_days: 7, retention_days: 0 } as never,
    t,
  )
  expect(badges).toEqual(['≤30d', 'force 7d'])
  expect(secToDaysField(7 * 86400)).toBe('7')
  expect(daysFieldToSec('30')).toBe(30 * 86400)
})

it('列表: max 30d 显示 ≤30d 而非被 force 压成 ≤7d', async () => {
  mockBackend()
  renderPage()
  await screen.findByText('VIP')
  expect(screen.getByText('≤30d')).toBeInTheDocument()
  expect(screen.getAllByText('force 7d').length).toBeGreaterThan(0)
})

it('新建组:全量 body POST', async () => {
  mockBackend()
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: /新建组/ }))
  await userEvent.type(screen.getByLabelText('组名'), '试验组')
  await userEvent.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => expect((created as { name: string }).name).toBe('试验组'))
  expect(created).toMatchObject({
    allowed_exts: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'heic', 'heif'],
    allowed_policy_ids: expect.any(Array),
  })
})

it('删除 VIP:两击后 DELETE', async () => {
  mockBackend()
  renderPage()
  await userEvent.click(await screen.findByText('VIP'))
  await userEvent.click(screen.getByRole('button', { name: '删除' }))
  await userEvent.click(screen.getByRole('button', { name: '确认删除？' }))
  await waitFor(() => expect(deleted).toBe('2'))
})

it('策略选择器:策略加载失败与暂无策略提示不同', async () => {
  vi.stubGlobal('fetch', vi.fn((url: RequestInfo | URL) => {
    const u = String(url)
    if (u.includes('/admin/policies')) return Promise.resolve(jsonRes(env(null), 500))
    if (u.includes('/admin/groups')) return Promise.resolve(jsonRes(env({ items: groups })))
    return Promise.resolve(jsonRes(env(null)))
  }))
  renderPage()
  await userEvent.click(await screen.findByText('默认组'))
  expect(await screen.findByText('存储策略加载失败')).toBeInTheDocument()
  expect(screen.queryByText('暂无存储策略')).not.toBeInTheDocument()
})

it('策略选择器:无策略时提示暂无存储策略', async () => {
  vi.stubGlobal('fetch', vi.fn((url: RequestInfo | URL) => {
    const u = String(url)
    if (u.includes('/admin/policies')) return Promise.resolve(jsonRes(env({ items: [] })))
    if (u.includes('/admin/groups')) return Promise.resolve(jsonRes(env({ items: groups })))
    return Promise.resolve(jsonRes(env(null)))
  }))
  renderPage()
  await userEvent.click(await screen.findByText('默认组'))
  expect(await screen.findByText('暂无存储策略')).toBeInTheDocument()
  expect(screen.queryByText('存储策略加载失败')).not.toBeInTheDocument()
})
