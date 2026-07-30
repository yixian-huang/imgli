import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ImageItem } from '../../api/types'
import { useGlobal } from '../../store'
import { DetailModal } from './DetailModal'

const LINKS = (k: string) => ({ url: `http://x/i/${k}.png`, markdown: `![${k}](u)`, html: '<img>', bbcode: '[img]u[/img]', thumbnail_url: `http://x/t/${k}.jpg` })
const item = (k: string, over: Partial<ImageItem> = {}): ImageItem => ({
  key: k, name: `${k}.png`, ext: 'png', size: 2048, width: 1920, height: 1080,
  visibility: 'public', album_id: null, created_at: '2026-07-16T00:00:00Z', expires_at: null, links: LINKS(k),
  ...over,
})
const items = [item('a'), item('b'), item('c')]

function jsonRes(body: unknown): Response {
  return { ok: true, status: 200, json: () => Promise.resolve(body) } as unknown as Response
}
const env = (data: unknown) => ({ status: true, message: 'ok', data })

function makeDaily(nonzero: Record<number, number> = {}): { date: string; views: number }[] {
  return Array.from({ length: 30 }, (_, i) => ({
    date: `2026-06-${String(i + 1).padStart(2, '0')}`,
    views: nonzero[i] ?? 0,
  }))
}

function mockBackend(opts: { stats?: { total: number; daily: { date: string; views: number }[] } | 'fail' } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url)
      if (u.includes('/albums')) return Promise.resolve(jsonRes(env({ items: [{ id: 7, name: '工作', visibility: 'private', image_count: 0, cover_key: '', created_at: '' }] })))
      if (init?.method === 'PATCH' || init?.method === 'DELETE') return Promise.resolve(jsonRes(env({})))
      if (u.includes('/stats')) {
        if (opts.stats === 'fail') {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ status: false, message: '服务器内部错误', data: { code: 'internal_error' } }),
          } as unknown as Response)
        }
        const body = opts.stats ?? { total: 0, daily: makeDaily() }
        return Promise.resolve(jsonRes(env(body)))
      }
      if (u.match(/\/images\/\w+$/))
        return Promise.resolve(jsonRes(env({ ...item(u.split('/').pop()!), mime: 'image/png', upload_ip: '203.0.113.42' })))
      return Promise.resolve(jsonRes(env(null)))
    }),
  )
}

function renderModal(focusKey = 'b', list: ImageItem[] = items) {
  const onClose = vi.fn()
  const onNavigate = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <DetailModal items={list} focusKey={focusKey} onClose={onClose} onNavigate={onNavigate} />
    </QueryClientProvider>,
  )
  return { onClose, onNavigate }
}

beforeEach(() => {
  useGlobal.setState({ toasts: [] })
  mockBackend()
})
afterEach(() => vi.unstubAllGlobals())

it('单栏连续：元信息、链接二维码、访问控制同屏', async () => {
  renderModal()
  expect(await screen.findByText('1920 × 1080')).toBeInTheDocument()
  expect(screen.getByText('2 / 3')).toBeInTheDocument()
  expect(await screen.findByText('image/png')).toBeInTheDocument()
  expect(screen.getByText(/203\.0\.113\.42/)).toBeInTheDocument()
  expect(screen.getByText(/仅自己可见/)).toBeInTheDocument()
  expect(screen.getAllByRole('button', { name: '复制' })).toHaveLength(6)
  expect(document.querySelector('svg')).toBeTruthy()
  expect(screen.getByPlaceholderText('输入或生成口令')).toBeInTheDocument()
  expect(screen.getByText('访问控制')).toBeInTheDocument()
})

it('ACCESS:总访问与近 30 天柱图', async () => {
  mockBackend({ stats: { total: 5, daily: makeDaily({ 10: 2, 20: 3 }) } })
  renderModal()
  expect(await screen.findByText(/ACCESS/)).toBeInTheDocument()
  expect(screen.getAllByText(/总访问\s*5/).length).toBeGreaterThan(0)
  expect(document.querySelectorAll('[class*="accessBars"] > [class*="accessBar"]').length).toBe(30)
})

it('ACCESS:全 0 显示暂无访问', async () => {
  mockBackend({ stats: { total: 0, daily: makeDaily() } })
  renderModal()
  expect(await screen.findByText('暂无访问')).toBeInTheDocument()
  expect(document.querySelectorAll('[class*="accessBars"] > [class*="accessBar"]')).toHaveLength(0)
})

it('ACCESS:请求失败静默隐藏统计区块', async () => {
  mockBackend({ stats: 'fail' })
  renderModal()
  await screen.findByText('1920 × 1080')
  await waitFor(() => {
    expect(screen.queryByText(/ACCESS/)).not.toBeInTheDocument()
    expect(screen.queryByText(/总访问/)).not.toBeInTheDocument()
  })
})

it('键盘 ←/→ 导航、Escape 关闭', async () => {
  const { onClose, onNavigate } = renderModal()
  await screen.findByText('2 / 3')
  fireEvent.keyDown(document, { key: 'ArrowRight' })
  expect(onNavigate).toHaveBeenCalledWith('c')
  fireEvent.keyDown(document, { key: 'ArrowLeft' })
  expect(onNavigate).toHaveBeenCalledWith('a')
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(onClose).toHaveBeenCalled()
})

it('首张无上一张', async () => {
  const { onNavigate } = renderModal('a')
  await screen.findByText('1 / 3')
  fireEvent.keyDown(document, { key: 'ArrowLeft' })
  expect(onNavigate).not.toHaveBeenCalled()
})

it('内联重命名 PATCH name', async () => {
  const user = userEvent.setup()
  renderModal()
  await screen.findByText('b.png')
  await user.click(screen.getByRole('button', { name: '重命名' }))
  const input = screen.getByDisplayValue('b.png')
  await user.clear(input)
  await user.type(input, 'renamed.png')
  await user.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => {
    const f = vi.mocked(fetch)
    const call = f.mock.calls.find((c) => (c[1] as RequestInit)?.method === 'PATCH')
    expect(call).toBeTruthy()
    expect(JSON.parse((call![1] as RequestInit).body as string)).toEqual({ name: 'renamed.png' })
  })
})

it('复制行含分享页共 6 条（无短链），加入回收站两击后 DELETE 并关闭', async () => {
  const user = userEvent.setup()
  const { onClose } = renderModal()
  await screen.findByText('b.png')
  expect(screen.getAllByRole('button', { name: '复制' })).toHaveLength(6)
  await user.click(screen.getByRole('button', { name: '加入回收站' }))
  await user.click(screen.getByRole('button', { name: '确认删除？' }))
  await waitFor(() => {
    expect(vi.mocked(fetch).mock.calls.find((c) => (c[1] as RequestInit)?.method === 'DELETE')).toBeTruthy()
  })
  await waitFor(() => expect(onClose).toHaveBeenCalled())
})

it('重命名编辑态方向键不切图', async () => {
  const user = userEvent.setup()
  const { onNavigate } = renderModal()
  await screen.findByText('b.png')
  await user.click(screen.getByRole('button', { name: '重命名' }))
  const input = screen.getByDisplayValue('b.png')
  input.focus()
  fireEvent.keyDown(document, { key: 'ArrowLeft' })
  expect(onNavigate).not.toHaveBeenCalled()
  fireEvent.keyDown(document, { key: 'Escape' })
})

it('expires_at 为 null 显示永久，无移除过期；口令可编辑', async () => {
  const user = userEvent.setup()
  renderModal('b')
  await screen.findByText('b.png')
  expect(screen.getAllByText('永久').length).toBeGreaterThan(0)
  expect(screen.queryByRole('button', { name: '移除过期' })).not.toBeInTheDocument()
  expect(screen.getByPlaceholderText('输入或生成口令')).toBeInTheDocument()
  // 长说明默认折叠，展开后可读
  await user.click(screen.getByText('说明'))
  expect(screen.getByText(/到期后永久删除/)).toBeInTheDocument()
})

it('expires_at 有值显示过期且可移除', async () => {
  const withExp = [item('a'), item('b', { expires_at: '2026-08-01T00:00:00Z' }), item('c')]
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url)
      if (u.includes('/albums')) return Promise.resolve(jsonRes(env({ items: [] })))
      if (init?.method === 'PATCH' || init?.method === 'DELETE') return Promise.resolve(jsonRes(env({})))
      if (u.includes('/stats')) return Promise.resolve(jsonRes(env({ total: 0, daily: makeDaily() })))
      if (u.match(/\/images\/\w+$/))
        return Promise.resolve(
          jsonRes(env({
            ...item('b', { expires_at: '2026-08-01T00:00:00Z' }),
            mime: 'image/png',
            upload_ip: '1.1.1.1',
          })),
        )
      return Promise.resolve(jsonRes(env(null)))
    }),
  )
  renderModal('b', withExp)
  expect(await screen.findByRole('button', { name: '移除过期' })).toBeInTheDocument()
  const labels = screen.getAllByText(/过期/).filter((el) => el.tagName !== 'BUTTON')
  expect(labels.length).toBeGreaterThan(0)
})

it('编辑有效期预设 PATCH expires_in；移除过期传 0', async () => {
  const user = userEvent.setup()
  const withExp = [item('a'), item('b', { expires_at: '2026-08-01T00:00:00Z' }), item('c')]
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url)
      if (u.includes('/albums')) return Promise.resolve(jsonRes(env({ items: [] })))
      if (init?.method === 'PATCH' || init?.method === 'DELETE') return Promise.resolve(jsonRes(env({})))
      if (u.includes('/stats')) return Promise.resolve(jsonRes(env({ total: 0, daily: makeDaily() })))
      if (u.match(/\/images\/\w+$/))
        return Promise.resolve(
          jsonRes(env({ ...item('b', { expires_at: '2026-08-01T00:00:00Z' }), mime: 'image/png', upload_ip: '1.1.1.1' })),
        )
      return Promise.resolve(jsonRes(env(null)))
    }),
  )
  renderModal('b', withExp)
  expect(await screen.findByRole('button', { name: '移除过期' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '7 天' }))
  await waitFor(() => {
    const call = vi.mocked(fetch).mock.calls.find(
      (c) =>
        (c[1] as RequestInit)?.method === 'PATCH' &&
        JSON.parse((c[1] as RequestInit).body as string).expires_in === 604800,
    )
    expect(call).toBeTruthy()
  })
  await user.click(screen.getByRole('button', { name: '移除过期' }))
  await waitFor(() => {
    const call = vi.mocked(fetch).mock.calls.find(
      (c) =>
        (c[1] as RequestInit)?.method === 'PATCH' &&
        JSON.parse((c[1] as RequestInit).body as string).expires_in === 0,
    )
    expect(call).toBeTruthy()
  })
})
