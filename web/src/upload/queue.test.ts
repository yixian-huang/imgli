import { ApiError } from '../api/client'
import type { UploadResult } from '../api/types'
import { extLabel, retryableCode, useUploadQueue } from './queue'
import * as uploader from './uploader'

vi.mock('./uploader')

const RESULT: UploadResult = {
  key: 'k1',
  name: 'a.png',
  size: 100,
  instant: false,
  links: { url: 'u', markdown: 'm', html: 'h', bbcode: 'b', thumbnail_url: 't' },
}

type Deferred = { resolve(r: UploadResult): void; reject(e: unknown): void; onProgress(p: number): void; aborted: boolean }
let pending: Deferred[] = []

function mockUploads() {
  pending = []
  vi.mocked(uploader.uploadFile).mockImplementation((_f, _opts, onProgress) => {
    const d = { onProgress, aborted: false } as Deferred
    const promise = new Promise<UploadResult>((resolve, reject) => {
      d.resolve = resolve
      d.reject = reject
    })
    pending.push(d)
    return {
      promise,
      abort: () => {
        d.aborted = true
        d.reject(new ApiError(0, 'aborted', '已取消'))
      },
    }
  })
}

const LIMITS = { maxFileSize: 5 * 1024 * 1024, allowedExts: ['png', 'jpg', 'jpeg', 'gif', 'webp'] }
const OPTS = { visibility: 'public' as const, albumId: null, policyId: null, expiresIn: 0, maxViews: 0 }

function makeFile(name: string, size = 100) {
  return new File([new Uint8Array(size)], name, { type: 'image/png' })
}
const flush = () => new Promise((r) => setTimeout(r, 0))

beforeEach(() => {
  mockUploads()
  useUploadQueue.setState({ items: [] })
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:x'), revokeObjectURL: vi.fn() })
})
afterEach(() => vi.unstubAllGlobals())

it('并发上限 3：第 4 个保持 queued，前面完成后补位', async () => {
  useUploadQueue.getState().addFiles([makeFile('1.png'), makeFile('2.png'), makeFile('3.png'), makeFile('4.png')], OPTS, LIMITS)
  await flush()
  const st = () => useUploadQueue.getState().items.map((i) => i.status)
  expect(st().filter((s) => s === 'uploading')).toHaveLength(3)
  expect(st().filter((s) => s === 'queued')).toHaveLength(1)

  pending[0].resolve(RESULT)
  await flush()
  expect(st().filter((s) => s === 'success')).toHaveLength(1)
  expect(st().filter((s) => s === 'uploading')).toHaveLength(3) // 第 4 个补位
})

it('进度 100 转 processing，响应到达转 success/instant', async () => {
  useUploadQueue.getState().addFiles([makeFile('a.png')], OPTS, LIMITS)
  await flush()
  pending[0].onProgress(100)
  expect(useUploadQueue.getState().items[0].status).toBe('processing')
  pending[0].resolve({ ...RESULT, instant: true })
  await flush()
  const it0 = useUploadQueue.getState().items[0]
  expect(it0.status).toBe('instant')
  expect(it0.result?.links.url).toBe('u')
})

it('预检：超大与后缀不符直接 failed 且不发请求', async () => {
  useUploadQueue.getState().addFiles([makeFile('big.png', 6 * 1024 * 1024), makeFile('x.psd')], OPTS, LIMITS)
  await flush()
  const [big, psd] = useUploadQueue.getState().items
  expect(big.status).toBe('failed')
  expect(big.retryable).toBe(false)
  expect(big.reason).toContain('文件超过大小上限')
  expect(psd.status).toBe('failed')
  expect(psd.reason).toContain('格式不允许')
  expect(uploader.uploadFile).not.toHaveBeenCalled()
})

it('失败码映射 retryable，重试重置回队列', async () => {
  expect(retryableCode('network_error')).toBe(true)
  expect(retryableCode('rate_limited')).toBe(true)
  expect(retryableCode('quota_exceeded')).toBe(false)
  expect(retryableCode('ext_not_allowed')).toBe(false)
  expect(retryableCode('heic_unsupported')).toBe(false)

  useUploadQueue.getState().addFiles([makeFile('a.png')], OPTS, LIMITS)
  await flush()
  pending[0].reject(new ApiError(0, 'network_error', '网络错误，请检查连接'))
  await flush()
  let item = useUploadQueue.getState().items[0]
  expect(item.status).toBe('failed')
  expect(item.retryable).toBe(true)
  expect(item.reason).toContain('网络错误')

  useUploadQueue.getState().retry(item.id)
  await flush()
  item = useUploadQueue.getState().items[0]
  expect(item.status).toBe('uploading')
  expect(uploader.uploadFile).toHaveBeenCalledTimes(2)
})

it('remove 中止在途上传并 revoke 预览', async () => {
  useUploadQueue.getState().addFiles([makeFile('a.png')], OPTS, LIMITS)
  await flush()
  const id = useUploadQueue.getState().items[0].id
  useUploadQueue.getState().remove(id)
  await flush()
  expect(pending[0].aborted).toBe(true)
  expect(useUploadQueue.getState().items).toHaveLength(0)
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:x')
})

it('clearDone 只清 success/instant', async () => {
  useUploadQueue.getState().addFiles([makeFile('a.png'), makeFile('b.png')], OPTS, LIMITS)
  await flush()
  pending[0].resolve(RESULT)
  pending[1].reject(new ApiError(0, 'network_error', 'x'))
  await flush()
  useUploadQueue.getState().clearDone()
  const items = useUploadQueue.getState().items
  expect(items).toHaveLength(1)
  expect(items[0].status).toBe('failed')
})

it('选相册/策略的项成功后直传完整 QueueOpts 且 finish 不再 PATCH album', async () => {
  const opts = { visibility: 'private' as const, albumId: 7, policyId: 3, expiresIn: 604800, maxViews: 0 }
  useUploadQueue.getState().addFiles([makeFile('a.png')], opts, LIMITS)
  await flush()
  expect(uploader.uploadFile).toHaveBeenCalledWith(expect.any(File), opts, expect.any(Function))
  expect(uploader.uploadFile).toHaveBeenCalledTimes(1)
  pending[0].resolve(RESULT)
  await flush()
  expect(useUploadQueue.getState().items[0].status).toBe('success')
  // 成功后无额外 upload 调用；album 已在上传事务内落库
  expect(uploader.uploadFile).toHaveBeenCalledTimes(1)
  expect(uploader.uploadFromURL).not.toHaveBeenCalled()
})

it('extLabel 对 null/undefined 容错', () => {
  expect(extLabel(null)).toBe('')
  expect(extLabel(undefined)).toBe('')
})
