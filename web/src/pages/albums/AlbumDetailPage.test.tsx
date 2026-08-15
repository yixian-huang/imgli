import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { useGlobal } from '../../store'
import { AlbumDetailPage } from './AlbumDetailPage'

function jsonRes(body: unknown): Response {
  return { ok: true, status: 200, json: () => Promise.resolve(body) } as unknown as Response
}
const env = (data: unknown) => ({ status: true, message: 'ok', data })
const LINKS = { url: 'http://x/i/a.png', markdown: 'm', html: 'h', bbcode: 'b', thumbnail_url: 'http://x/t/a.jpg' }

class FakeIO {
  cb: IntersectionObserverCallback
  constructor(cb: IntersectionObserverCallback) {
    this.cb = cb
  }
  observe() {}
  disconnect() {}
  unobserve() {}
}

function mockBackend(visibility: 'public' | 'private' = 'private', imageVis: 'public' | 'private' = 'public') {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url)
      if (u.includes('/albums/') && u.includes('/stats'))
        return Promise.resolve(jsonRes(env({ total: 0, daily: Array.from({ length: 30 }, (_, i) => ({ date: `2026-07-${String(i + 1).padStart(2, '0')}`, views: 0 })) })))
      if (u.match(/\/albums\/?\d*$/) || (u.includes('/albums') && !u.includes('/albums/') && (!init || !init.method)))
        return Promise.resolve(
          jsonRes(
            env({
              items: [
                {
                  id: 7,
                  name: '工作',
                  visibility,
                  image_count: 1,
                  cover_key: 'a',
                  created_at: '2026-07-16T00:00:00Z',
                  list_in_plaza: true,
                  has_access_password: false,
                  description: '',
                },
              ],
            }),
          ),
        )
      if (init?.method === 'PATCH') return Promise.resolve(jsonRes(env({ id: 7, name: '改名', visibility: 'public' })))
      if (u.includes('/images?'))
        return Promise.resolve(
          jsonRes(env({ items: [{ key: 'a', name: 'a.png', ext: 'png', size: 1, width: 1, height: 1, visibility: imageVis, album_id: 7, created_at: '2026-07-16T00:00:00Z', expires_at: null, links: LINKS }], next_cursor: '' })),
        )
      return Promise.resolve(jsonRes(env(null)))
    }),
  )
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/albums/7']}>
        <Routes>
          <Route path="/albums/:id" element={<AlbumDetailPage />} />
          <Route path="/albums" element={<div>LIST</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.stubGlobal('IntersectionObserver', FakeIO as unknown as typeof IntersectionObserver)
  mockBackend()
})
afterEach(() => vi.unstubAllGlobals())

it('详情页：相册筛选取数、页头徽章、返回链接', async () => {
  renderPage()
  expect(await screen.findByText('工作')).toBeInTheDocument()
  expect(screen.getByText('PRIVATE')).toBeInTheDocument()
  expect(await screen.findByText('a.png')).toBeInTheDocument()
  const f = vi.mocked(fetch)
  expect(f.mock.calls.some((c) => String(c[0]).includes('album=7'))).toBe(true)
  expect(screen.getByRole('link', { name: /ALBUMS/ })).toHaveAttribute('href', '/albums')
})

it('内联重命名 PATCH 相册名', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('工作')
  await user.click(screen.getByRole('button', { name: '重命名' }))
  const input = screen.getByDisplayValue('工作')
  await user.clear(input)
  await user.type(input, '改名')
  await user.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => {
    const f = vi.mocked(fetch)
    const call = f.mock.calls.find((c) => {
      if ((c[1] as RequestInit)?.method !== 'PATCH') return false
      try {
        return JSON.parse((c[1] as RequestInit).body as string).name === '改名'
      } catch {
        return false
      }
    })
    expect(call).toBeTruthy()
    expect(String(call![0])).toContain('/albums/7')
  })
})

it('设置弹窗：打开后可见分享与访问，默认视图在弹窗内', async () => {
  const user = userEvent.setup()
  mockBackend('public')
  renderPage()
  await screen.findByText('工作')
  expect(screen.queryByTestId('album-default-view')).not.toBeInTheDocument()
  await user.click(screen.getByTestId('album-settings-btn'))
  expect(await screen.findByTestId('album-settings-share')).toBeInTheDocument()
  expect(screen.getByTestId('album-default-view')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '内容与广场' }))
  expect(await screen.findByTestId('album-settings-content')).toBeInTheDocument()
})

it('公开相册：复制访客链接有 toast，并提供打开访客页入口', async () => {
  const user = userEvent.setup()
  mockBackend('public')
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  const open = vi.spyOn(window, 'open').mockImplementation(() => null)
  useGlobal.setState({ toasts: [] })

  renderPage()
  await screen.findByText('工作')
  expect(screen.getByText('PUBLIC')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: '打开访客页' }))
  expect(open).toHaveBeenCalledWith('/a/7', '_blank', 'noopener,noreferrer')

  await user.click(screen.getByRole('button', { name: '复制访客链接' }))
  await waitFor(() => {
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/a/7`)
  })
  expect(useGlobal.getState().toasts.some((t) => t.message.includes('访客链接'))).toBe(true)
  open.mockRestore()
})

it('私密相册内快捷切换不把图改回公开', async () => {
  const user = userEvent.setup()
  mockBackend('private', 'private')
  useGlobal.setState({ toasts: [] })
  renderPage()
  await screen.findByText('a.png')
  await user.click(screen.getByTitle('切换可见性'))
  await waitFor(() => {
    expect(useGlobal.getState().toasts.some((t) => t.message.includes('不能设为公开'))).toBe(true)
  })
  const visPatch = vi.mocked(fetch).mock.calls.find((c) => {
    if ((c[1] as RequestInit)?.method !== 'PATCH') return false
    try {
      return JSON.parse((c[1] as RequestInit).body as string).visibility === 'public'
    } catch {
      return false
    }
  })
  expect(visPatch).toBeFalsy()
})
