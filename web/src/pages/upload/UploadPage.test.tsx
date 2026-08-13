import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createEvent, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { sessionKey } from '../../api/hooks'
import type { User } from '../../api/types'
import { useGlobal } from '../../store'
import { useUploadQueue, type QueueItem } from '../../upload/queue'
import { uploadFile } from '../../upload/uploader'
import { UploadPage } from './UploadPage'

vi.mock('../../upload/uploader', () => ({
  uploadFile: vi.fn(() => ({ promise: new Promise(() => {}), abort: vi.fn() })),
  uploadFromURL: vi.fn(() => new Promise(() => {})),
}))

const GB = 1024 ** 3
let quotaUsed = 1 * GB

const EMPTY_PREFS = {
  default_album_id: null as number | null,
  default_visibility: '' as const,
  default_policy_id: null as number | null,
  auto_copy_format: '' as const,
  watermark: { enabled: false, position: '', opacity: 0, margin: 0 },
}

const BASE_USER: User = {
  id: 1,
  username: 'ling',
  email: 'l@img.li',
  nickname: '凌',
  is_admin: false,
  email_verified: true,
  created_at: '',
  preferences: EMPTY_PREFS,
  avatar_url: '',
  watermark_set: false,
  public_profile: false,
}

function jsonRes(body: unknown, status = 200): Response {
  return { ok: status < 400, status, json: () => Promise.resolve(body) } as unknown as Response
}
const env = (data: unknown) => ({ status: true, message: 'ok', data })

function mockBackend(sessionUser: User = BASE_USER, opts: { plazaEnabled?: boolean } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL) => {
      const u = String(url)
      if (u.endsWith('/auth/session')) return Promise.resolve(jsonRes(env(sessionUser)))
      if (u.endsWith('/user/quota'))
        return Promise.resolve(
          jsonRes(env({ used: quotaUsed, total: 10 * GB, max_file_size: 20 * 1024 ** 2, allowed_exts: ['png', 'jpg', 'jpeg', 'gif', 'webp'] })),
        )
      if (u.endsWith('/albums'))
        return Promise.resolve(jsonRes(env({ items: [{ id: 7, name: '工作', visibility: 'private', image_count: 3, cover_key: '', created_at: '' }] })))
      if (u.endsWith('/user/policies')) return Promise.resolve(jsonRes(env([{ id: 1, name: '本地' }])))
      if (u.endsWith('/config'))
        return Promise.resolve(
          jsonRes(env({
            site_name: 'img.li', registration_mode: 'open', guest_upload_enabled: false,
            guest: null,
            plaza_enabled: !!opts.plazaEnabled,
          })),
        )
      return Promise.resolve(jsonRes({ status: false, message: '', data: { code: 'not_found' } }, 404))
    }),
  )
}

function mockGuestBackend() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: RequestInfo | URL) => {
      const u = String(url)
      if (u.endsWith('/auth/session'))
        return Promise.resolve(jsonRes({ status: false, message: '未登录', data: { code: 'unauthorized' } }, 401))
      if (u.endsWith('/config'))
        return Promise.resolve(
          jsonRes(env({
            site_name: 'img.li', registration_mode: 'open', guest_upload_enabled: true,
            guest: { max_file_size: 5 * 1024 ** 2, allowed_exts: ['png', 'jpg'], per_day: 3 },
          })),
        )
      return Promise.resolve(jsonRes({ status: false, message: '', data: { code: 'not_found' } }, 404))
    }),
  )
}

function renderPage(seedSession?: User) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  if (seedSession) qc.setQueryData(sessionKey, seedSession)
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <UploadPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return qc
}

beforeEach(() => {
  quotaUsed = 1 * GB
  useUploadQueue.setState({ items: [] })
  useGlobal.setState({ toasts: [] })
  mockBackend()
})
afterEach(() => vi.unstubAllGlobals())

it('限制标签由 quota 数据渲染', async () => {
  renderPage()
  expect(await screen.findByText('PNG · JPG · GIF · WEBP — MAX 20 MB')).toBeInTheDocument()
})

it('选项面板默认收起，展开含相册、可见性与有效期（无存储策略）', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByText(/MAX 20 MB/)
  expect(screen.queryByText('上传到相册')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /上传选项/ }))
  expect(screen.getByText('上传到相册')).toBeInTheDocument()
  expect(await screen.findByRole('option', { name: '工作' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '公开' })).toBeInTheDocument()
  expect(screen.queryByText('存储策略')).not.toBeInTheDocument()
  expect(screen.getByText('有效期')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '7 天' })).toBeInTheDocument()
})

it('广场开启且未开公开主页：公开可见性显示 opt-in 引导', async () => {
  const user = userEvent.setup()
  mockBackend(BASE_USER, { plazaEnabled: true })
  renderPage()
  await screen.findByText(/MAX 20 MB/)
  expect(await screen.findByTestId('plaza-opt-in-hint')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '去开启' })).toHaveAttribute('href', '/settings/profile')
  await user.click(screen.getByRole('button', { name: /上传选项/ }))
  await user.click(screen.getByRole('button', { name: '私密' }))
  expect(screen.queryByTestId('plaza-opt-in-hint')).not.toBeInTheDocument()
})

it('广场关闭：不显示广场 opt-in 引导', async () => {
  mockBackend(BASE_USER, { plazaEnabled: false })
  renderPage()
  await screen.findByText(/MAX 20 MB/)
  expect(screen.queryByTestId('plaza-opt-in-hint')).not.toBeInTheDocument()
})

it('选 7 天有效期 → 入队 opts.expiresIn=604800', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByText(/MAX 20 MB/)
  await user.click(screen.getByRole('button', { name: /上传选项/ }))
  await user.click(screen.getByRole('button', { name: '7 天' }))
  fireEvent.drop(screen.getByTestId('dropzone'), {
    dataTransfer: { files: [new File([new Uint8Array(10)], 'e.png', { type: 'image/png' })], types: ['Files'] },
  })
  expect(useUploadQueue.getState().items[0]?.opts.expiresIn).toBe(604800)
})

it('拖放文件只入队并上传一次，且清除拖拽提示', async () => {
  renderPage()
  await screen.findByText(/MAX 20 MB/) // 等 quota 加载完成（limits 就绪才可入队）
  const dz = screen.getByTestId('dropzone')
  const file = new File([new Uint8Array(10)], 'drop.png', { type: 'image/png' })
  fireEvent.dragOver(dz, { dataTransfer: { files: [file], types: ['Files'] } })
  expect(screen.getAllByText('松开即上传')).toHaveLength(2)
  fireEvent.drop(dz, { dataTransfer: { files: [file], types: ['Files'] } })
  expect(useUploadQueue.getState().items).toHaveLength(1)
  expect(useUploadQueue.getState().items[0]?.name).toBe('drop.png')
  expect(uploadFile).toHaveBeenCalledTimes(1)
  expect(screen.queryByText('松开即上传')).not.toBeInTheDocument()
})

it('拖入后移出页面会清除拖拽提示且不上传', async () => {
  renderPage()
  await screen.findByText(/MAX 20 MB/)
  const dz = screen.getByTestId('dropzone')
  const pageContent = screen.getByText('上传图片')
  const file = new File([new Uint8Array(10)], 'cancel.png', { type: 'image/png' })

  fireEvent.dragOver(dz, { dataTransfer: { files: [file], types: ['Files'] } })
  expect(screen.getAllByText('松开即上传')).toHaveLength(2)

  const moveInside = createEvent.dragLeave(dz)
  Object.defineProperty(moveInside, 'relatedTarget', { value: pageContent })
  fireEvent(dz, moveInside)
  expect(screen.getAllByText('松开即上传')).toHaveLength(1)

  fireEvent.dragLeave(pageContent)
  expect(screen.queryByText('松开即上传')).not.toBeInTheDocument()
  expect(useUploadQueue.getState().items).toHaveLength(0)
  expect(uploadFile).not.toHaveBeenCalled()
})

it('URL 抓取行：非法 URL toast，合法入队', async () => {
  const user = userEvent.setup()
  renderPage()
  const input = await screen.findByPlaceholderText(/粘贴图片链接/)
  await user.type(input, 'not-a-url')
  await user.click(screen.getByRole('button', { name: '抓取' }))
  expect(useGlobal.getState().toasts.at(-1)?.message).toBe('请输入有效的图片 URL')
  await user.clear(input)
  await user.type(input, 'https://x.com/a.png')
  await user.click(screen.getByRole('button', { name: '抓取' }))
  expect(useUploadQueue.getState().items[0]?.kind).toBe('url')
})

it('配额满：禁用层出现，点击/拖放只 toast 不入队', async () => {
  quotaUsed = 10 * GB
  renderPage()
  expect(await screen.findByText('容量已满，上传已禁用')).toBeInTheDocument()
  const dz = screen.getByTestId('dropzone')
  fireEvent.drop(dz, { dataTransfer: { files: [new File([new Uint8Array(1)], 'x.png', { type: 'image/png' })], types: ['Files'] } })
  expect(useUploadQueue.getState().items).toHaveLength(0)
  expect(useGlobal.getState().toasts.at(-1)?.message).toBe('容量已满，无法上传')
  fireEvent.click(screen.getByText('容量已满，上传已禁用'))
  expect(useGlobal.getState().toasts.at(-1)?.message).toBe('容量已满，无法上传')
})

it('Ctrl+V 粘贴图片入队', async () => {
  renderPage()
  await screen.findByText(/MAX 20 MB/) // 等 quota 加载完成
  const file = new File([new Uint8Array(8)], 'shot.png', { type: 'image/png' })
  fireEvent.paste(window, { clipboardData: { items: [{ type: 'image/png', getAsFile: () => file }] } })
  expect(useUploadQueue.getState().items[0]?.name).toBe('shot.png')
})

it('游客态：限额横幅、无上传选项、按游客限额校验、不请求登录态接口', async () => {
  mockGuestBackend()
  renderPage()
  expect(await screen.findByText(/游客模式/)).toBeInTheDocument()
  expect(screen.getByText(/每日 3 张/)).toBeInTheDocument()
  expect(await screen.findByText('PNG · JPG — MAX 5 MB')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /上传选项/ })).not.toBeInTheDocument()
  const big = new File([new Uint8Array(1)], 'big.png', { type: 'image/png' })
  Object.defineProperty(big, 'size', { value: 6 * 1024 ** 2 })
  fireEvent.drop(screen.getByTestId('dropzone'), { dataTransfer: { files: [big], types: ['Files'] } })
  expect(useUploadQueue.getState().items[0]?.status).toBe('failed')
  const urls = (fetch as ReturnType<typeof vi.fn>).mock.calls.map((c) => String(c[0]))
  expect(urls.some((u) => u.includes('/user/quota'))).toBe(false)
  expect(urls.some((u) => u.includes('/albums'))).toBe(false)
})

it('游客入队项 opts 恒 public 且不归相册', async () => {
  mockGuestBackend()
  renderPage()
  await screen.findByText(/MAX 5 MB/)
  fireEvent.drop(screen.getByTestId('dropzone'), {
    dataTransfer: { files: [new File([new Uint8Array(10)], 'a.png', { type: 'image/png' })], types: ['Files'] },
  })
  expect(useUploadQueue.getState().items[0]?.opts).toEqual({
    visibility: 'public',
    albumId: null,
    policyId: null,
    expiresIn: 0, maxViews: 0,
  })
})

it('偏好初始值：session preferences 驱动 summary 含私密', async () => {
  const session: User = {
    ...BASE_USER,
    preferences: {
      default_visibility: 'private',
      default_album_id: 7,
      default_policy_id: null,
      auto_copy_format: 'markdown',
      watermark: { enabled: false, position: '', opacity: 0, margin: 0 },
    },
  }
  mockBackend(session)
  renderPage(session)
  expect(await screen.findByText(/私密/)).toBeInTheDocument()
})

it('队列全部完成后按 auto_copy_format 聚合自动复制', async () => {
  const session: User = {
    ...BASE_USER,
    preferences: {
      default_visibility: '',
      default_album_id: null,
      default_policy_id: null,
      auto_copy_format: 'markdown',
      watermark: { enabled: false, position: '', opacity: 0, margin: 0 },
    },
  }
  mockBackend(session)
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

  renderPage(session)
  await screen.findByText(/MAX 20 MB/)

  const links1 = {
    url: 'https://img.li/i/a.png',
    markdown: '![a](https://img.li/i/a.png)',
    html: '<img a>',
    bbcode: '[img]a[/img]',
    thumbnail_url: 't1',
  }
  const links2 = {
    url: 'https://img.li/i/b.png',
    markdown: '![b](https://img.li/i/b.png)',
    html: '<img b>',
    bbcode: '[img]b[/img]',
    thumbnail_url: 't2',
  }
  const base: Omit<QueueItem, 'id' | 'result' | 'status' | 'name'> = {
    kind: 'file',
    size: 1,
    ext: 'png',
    pct: 100,
    thumb: null,
    reason: null,
    retryable: false,
    opts: { visibility: 'public', albumId: null, policyId: null, expiresIn: 0, maxViews: 0 },
  }
  useUploadQueue.setState({
    items: [
      { ...base, id: 101, name: 'a.png', status: 'success', result: { key: 'a', name: 'a.png', size: 1, instant: false, links: links1 } },
      { ...base, id: 102, name: 'b.png', status: 'success', result: { key: 'b', name: 'b.png', size: 1, instant: false, links: links2 } },
    ],
  })

  await waitFor(() => {
    expect(writeText).toHaveBeenCalledTimes(1)
  })
  expect(writeText).toHaveBeenCalledWith(`${links1.markdown}\n${links2.markdown}`)
  expect(useGlobal.getState().toasts.some((t) => t.message.includes('已自动复制 2 条链接'))).toBe(true)
})
