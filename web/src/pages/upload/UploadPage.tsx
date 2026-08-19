import { useEffect, useRef, useState, type DragEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '../../api/queryKeys'
import { useAlbums, useConfig, useQuota, useSession, useUserPolicies } from '../../api/hooks'
import { useT } from '../../i18n'
import { cn } from '../../lib/cn'
import { formatBytes } from '../../lib/format'
import {
  EXPIRY_PRESETS,
  expiryPresetLabel,
  filterExpiryPresets,
  filterMaxViewsPresets,
  groupExpiresCapSec,
  maxViewsPresetLabel,
  resolveDefaultExpiresIn,
  resolveDefaultMaxViews,
} from '../../lib/imageAccessPresets'
import { loginHref } from '../../lib/safeNext'
import { useGlobal } from '../../store'
import { extLabel, useUploadQueue, type QueueOpts } from '../../upload/queue'
import { copyText } from '../../lib/copy'
import { QuotaBar, quotaLevel } from '../../ui/QuotaBar'
import { Segmented } from '../../ui/Segmented'
import { Skeleton } from '../../ui/Skeleton'
import { PageHeader } from '../../shell/PageHeader'
import { UploadCard } from './UploadCard'
import { FirstRunOnboarding } from './FirstRunOnboarding'
import { InstanceStatsBar } from './InstanceStatsBar'

const URL_RE = /^https?:\/\/\S+$/

/** 嵌套子节点之间移动时 relatedTarget 仍在当前元素内，不应当成离开。 */
function dragLeftElement(e: DragEvent) {
  const next = e.relatedTarget
  return !(next instanceof Node) || !e.currentTarget.contains(next)
}

/** Re-export for tests / callers that imported presets from this page. */
export { EXPIRY_PRESETS }

export function UploadPage() {
  const { t } = useT()
  const { data: me } = useSession()
  const isGuest = !me
  const quota = useQuota(!isGuest)
  const albums = useAlbums(!isGuest)
  const policies = useUserPolicies(!isGuest)
  const config = useConfig()
  const qc = useQueryClient()
  const pushToast = useGlobal((s) => s.pushToast)
  const items = useUploadQueue((s) => s.items)
  const addFiles = useUploadQueue((s) => s.addFiles)
  const addUrl = useUploadQueue((s) => s.addUrl)
  const clearDone = useUploadQueue((s) => s.clearDone)

  const prefs = me?.preferences
  const [searchParams] = useSearchParams()
  const albumFromUrl = Number(searchParams.get('album')) || 0
  const [drag, setDrag] = useState(false)
  const [pageDrag, setPageDrag] = useState(false)
  const [fetchUrl, setFetchUrl] = useState('')
  const [optsOpen, setOptsOpen] = useState(false)
  const [visibility, setVisibility] = useState<'public' | 'private'>(() =>
    prefs?.default_visibility === 'private' ? 'private' : 'public',
  )
  const [albumId, setAlbumId] = useState<number | null>(() =>
    albumFromUrl > 0 ? albumFromUrl : (prefs?.default_album_id ?? null),
  )
  // 相册页「上传到此」带 ?album=
  useEffect(() => {
    if (albumFromUrl > 0) setAlbumId(albumFromUrl)
  }, [albumFromUrl])
  const [policyId, setPolicyId] = useState<number | null>(() => prefs?.default_policy_id ?? null)
  const [expiresIn, setExpiresIn] = useState(0)
  const [maxViews, setMaxViews] = useState(0)
  const accessDefaultsSeeded = useRef(false)
  const fileInput = useRef<HTMLInputElement>(null)
  // 队列是全局 store:挂载时把已完成项播种为「已复制」,防路由往返后重复自动复制历史项(codex 终审)
  const copiedRef = useRef<Set<number>>(new Set())
  const copySeeded = useRef(false)
  if (!copySeeded.current) {
    copySeeded.current = true
    for (const i of items) {
      if (i.status === 'success' || i.status === 'instant') copiedRef.current.add(i.id)
    }
  }

  const full = !isGuest && quota.data ? quotaLevel(quota.data.used, quota.data.total) === 'full' : false
  const bwQuota = quota.data?.bandwidth_quota_month ?? 0
  const bwUsed = quota.data?.bandwidth_used_month ?? 0
  const bwFull = !isGuest && bwQuota > 0 && quotaLevel(bwUsed, bwQuota) === 'full'
  const guestUploadOn = !!config.data?.guest_upload_enabled
  const guestLimits = config.data?.guest
  /** 未登录且站点关闭游客上传：展示落地页，不可实际上传 */
  const needLogin = isGuest && config.data != null && !guestUploadOn
  const loginTo = loginHref('/')
  const limits = isGuest
    ? guestUploadOn && guestLimits
      ? { maxFileSize: guestLimits.max_file_size, allowedExts: guestLimits.allowed_exts ?? [] }
      : null
    : quota.data
      ? { maxFileSize: quota.data.max_file_size, allowedExts: quota.data.allowed_exts ?? [] }
      : null
  const showPolicy = (policies.data?.length ?? 0) > 1
  const accessPolicy = isGuest
    ? {
        default_expires_in: guestLimits?.default_expires_in ?? 0,
        max_expires_in: guestLimits?.max_expires_in ?? 0,
        default_max_views: guestLimits?.default_max_views ?? 0,
        max_max_views: guestLimits?.max_max_views ?? 0,
        force_max_age_days: guestLimits?.force_max_age_days ?? 0,
      }
    : {
        default_expires_in: quota.data?.default_expires_in ?? 0,
        max_expires_in: quota.data?.max_expires_in ?? 0,
        default_max_views: quota.data?.default_max_views ?? 0,
        max_max_views: quota.data?.max_max_views ?? 0,
        force_max_age_days: quota.data?.force_max_age_days ?? 0,
      }
  const expiresCap = groupExpiresCapSec(accessPolicy)
  const expiryPresets = filterExpiryPresets(expiresCap)
  const maxViewsPresets = filterMaxViewsPresets(accessPolicy.max_max_views)
  // 组策略就绪后预填默认（仅一次，避免覆盖用户已改选项）
  useEffect(() => {
    if (accessDefaultsSeeded.current) return
    const ready = isGuest ? config.data != null : quota.data != null
    if (!ready) return
    accessDefaultsSeeded.current = true
    setExpiresIn(
      resolveDefaultExpiresIn(
        accessPolicy.default_expires_in,
        expiresCap,
        expiryPresets,
      ),
    )
    setMaxViews(
      resolveDefaultMaxViews(
        accessPolicy.default_max_views,
        accessPolicy.max_max_views,
        maxViewsPresets,
      ),
    )
  }, [isGuest, config.data, quota.data, accessPolicy, expiresCap, expiryPresets, maxViewsPresets])

  const opts: QueueOpts = isGuest
    ? { visibility: 'public', albumId: null, policyId: null, expiresIn, maxViews }
    : { visibility, albumId, policyId: showPolicy ? policyId : null, expiresIn, maxViews }
  const expiryKey =
    expiryPresets.find((p) => p.sec === expiresIn)?.key
    ?? (expiresCap > 0 ? (expiryPresets[0]?.key ?? 'never') : 'never')
  const maxViewsKey =
    maxViewsPresets.find((p) => p.n === maxViews)?.key
    ?? (accessPolicy.max_max_views > 0 ? (maxViewsPresets[0]?.key ?? 'unlimited') : 'unlimited')
  const expiryLabel = expiryPresetLabel(
    expiryPresets.find((p) => p.key === expiryKey) ?? { key: expiryKey, sec: expiresIn },
    t,
  )
  const maxViewsLabel = maxViewsPresetLabel(
    maxViewsPresets.find((p) => p.key === maxViewsKey) ?? { key: maxViewsKey, n: maxViews },
    t,
  )

  const selectedAlbum = albumId != null ? albums.data?.items.find((a) => a.id === albumId) : undefined
  const albumForcesPrivate = selectedAlbum?.visibility === 'private'

  useEffect(() => {
    if (albumId !== null && albums.data && !albums.data.items.some((a) => a.id === albumId)) {
      setAlbumId(null)
    }
  }, [albums.data, albumId])
  useEffect(() => {
    if (albumForcesPrivate) setVisibility('private')
  }, [albumForcesPrivate])
  useEffect(() => {
    if (policyId !== null && policies.data && !policies.data.some((po) => po.id === policyId)) {
      setPolicyId(null)
    }
  }, [policies.data, policyId])

  // 每个完成项让配额失效刷新（导航容量条/警告条实时跟进）
  const doneCount = items.filter((i) => i.status === 'success' || i.status === 'instant').length
  useEffect(() => {
    if (doneCount > 0) {
      qc.invalidateQueries({ queryKey: queryKeys.quota })
      qc.invalidateQueries({ queryKey: queryKeys.imagesRoot })
      qc.invalidateQueries({ queryKey: queryKeys.admin.imagesRoot })
    }
  }, [doneCount, qc])

  const copyFmt = isGuest ? '' : (me?.preferences?.auto_copy_format ?? '')
  useEffect(() => {
    if (!copyFmt) return
    const active = items.some(
      (i) => i.status === 'queued' || i.status === 'uploading' || i.status === 'processing',
    )
    if (active) return
    const fresh = items.filter(
      (i) => (i.status === 'success' || i.status === 'instant') && i.result && !copiedRef.current.has(i.id),
    )
    if (!fresh.length) return
    for (const i of fresh) copiedRef.current.add(i.id)
    const text = fresh
      .map((i) => {
        const links = i.result!.links
        if (copyFmt === 'share') return links.share_url || links.url || ''
        if (copyFmt === 'url') return links.url
        if (copyFmt === 'markdown') return links.markdown
        if (copyFmt === 'html') return links.html
        if (copyFmt === 'bbcode') return links.bbcode
        return links.url
      })
      .filter(Boolean)
      .join('\n')
    navigator.clipboard
      .writeText(text)
      .then(() => pushToast(t('upload.toastAutoCopied', { count: fresh.length })))
      .catch(() => pushToast(t('upload.toastAutoCopyFailed')))
  }, [items, copyFmt, pushToast, t])

  // Ctrl+V 粘贴：监听只挂一次；闸门/选项经 ref 读最新值，避免 opts 每渲染新对象导致重绑。
  const pasteCtx = useRef({ needLogin, full, bwFull, limits, opts, addFiles, pushToast, t })
  pasteCtx.current = { needLogin, full, bwFull, limits, opts, addFiles, pushToast, t }
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const ctx = pasteCtx.current
      const imgs = [...(e.clipboardData?.items ?? [])]
        .filter((i) => i.type.startsWith('image/'))
        .map((i) => i.getAsFile())
        .filter((f): f is File => !!f)
      if (!imgs.length) return
      if (ctx.needLogin) return ctx.pushToast(ctx.t('upload.toastLoginRequired'))
      if (ctx.full) return ctx.pushToast(ctx.t('upload.toastQuotaFull'))
      if (ctx.bwFull) return ctx.pushToast(ctx.t('upload.toastBandwidthFull'))
      if (ctx.limits) ctx.addFiles(imgs, ctx.opts, ctx.limits)
    }
    window.addEventListener('paste', onPaste)
    return () => window.removeEventListener('paste', onPaste)
  }, [])

  function acceptFiles(list: FileList | File[]) {
    if (needLogin) return pushToast(t('upload.toastLoginRequired'))
    if (full) return pushToast(t('upload.toastQuotaFull'))
    if (bwFull) return pushToast(t('upload.toastBandwidthFull'))
    if (!limits) return
    const files = [...list].filter((f) => f.type.startsWith('image/') || f.name.includes('.'))
    if (files.length) addFiles(files, opts, limits)
  }

  function doFetch() {
    if (needLogin) return pushToast(t('upload.toastLoginRequired'))
    const u = fetchUrl.trim()
    if (!URL_RE.test(u)) return pushToast(t('upload.toastInvalidUrl'))
    if (full) return pushToast(t('upload.toastQuotaFull'))
    if (bwFull) return pushToast(t('upload.toastBandwidthFull'))
    addUrl(u, opts)
    setFetchUrl('')
    pushToast(t('upload.toastFetchQueued'))
  }

  function copyAllLinks() {
    const urls = items.filter((i) => i.result).map((i) => i.result!.links.url)
    if (!urls.length) return pushToast(t('upload.toastNoLinks'))
    copyText(urls.join('\n'), t('upload.nLinks', { count: urls.length }))
  }

  const albumName = albumId ? (albums.data?.items.find((a) => a.id === albumId)?.name ?? '') : t('upload.noAlbum')
  const summary = `${albumName} · ${visibility === 'public' ? t('upload.public') : t('upload.private')} · ${expiryLabel} · ${maxViewsLabel}`

  return (
    <div
      className="relative mx-auto max-w-[760px] pt-12 pb-8"
      onDragEnter={(e) => {
        e.preventDefault()
        if (e.dataTransfer.types.includes('Files')) setPageDrag(true)
      }}
      onDragOver={(e) => {
        e.preventDefault()
        if (e.dataTransfer.types.includes('Files')) setPageDrag(true)
      }}
      onDragLeave={(e) => {
        if (dragLeftElement(e)) {
          setPageDrag(false)
          setDrag(false)
        }
      }}
      onDrop={(e) => {
        e.preventDefault()
        setPageDrag(false)
        setDrag(false)
        acceptFiles(e.dataTransfer.files)
      }}
    >
      {!isGuest && <FirstRunOnboarding show />}

      {pageDrag && (
        <div className="pointer-events-none fixed inset-0 z-40 flex items-center justify-center border-2 border-dashed border-ink bg-[color-mix(in_srgb,var(--bg)_70%,transparent)] text-base font-bold">
          {t('upload.dropRelease')}
        </div>
      )}
      {/* 副标题跟在 sticky 标题条下方全宽左对齐，避免 right-extra 与统计条抢层级 */}
      <PageHeader kicker="UPLOAD" title={t('upload.title')} className="mb-3" />
      <p className="m-0 mb-5 max-w-[36rem] text-[13px] leading-relaxed text-muted">
        {t('upload.subtitle')}
      </p>

      {/* 落地页节奏：标题 → 信任指标 → 主 CTA → 次级示意区；登录态保持原上传工作区顺序 */}
      {needLogin ? (
        <div className="mb-5 flex flex-col gap-3.5">
          <InstanceStatsBar stats={config.data?.public_stats} />
          <div
            className="rounded border border-border bg-surface px-5 py-5 max-md:px-4"
            data-testid="login-gate"
          >
            <div className="mb-1.5 text-[15px] font-bold tracking-[-0.01em] text-ink">
              {t('upload.loginRequiredTitle')}
            </div>
            <p className="mb-4 mt-0 max-w-[36rem] text-[13px] leading-relaxed text-muted">
              {t('upload.loginRequiredDesc')}
            </p>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
              <Link
                to={loginTo}
                className="inline-flex h-[34px] items-center rounded-sm bg-btn px-4 text-[13px] font-bold text-btn-text no-underline hover:opacity-90"
              >
                {t('upload.loginRequiredCta')}
              </Link>
              {(!!config.data?.help_url?.trim() || !!config.data?.upgrade_url?.trim()) && (
                <span className="flex flex-wrap items-center gap-1.5 text-sm-plus">
                  {!!config.data?.help_url?.trim() && (
                    <a
                      href={config.data.help_url.trim()}
                      rel="noopener noreferrer"
                      className="text-ink underline underline-offset-2"
                    >
                      {t('upload.helpLink')}
                    </a>
                  )}
                  {!!config.data?.help_url?.trim() && !!config.data?.upgrade_url?.trim() && (
                    <span className="text-muted">·</span>
                  )}
                  {!!config.data?.upgrade_url?.trim() && (
                    <a
                      href={config.data.upgrade_url.trim()}
                      rel="noopener noreferrer"
                      className="text-ink underline underline-offset-2"
                    >
                      {t('upload.upgradeLink')}
                    </a>
                  )}
                </span>
              )}
            </div>
            <p className="mt-3 mb-0 text-xs leading-normal text-muted">{t('upload.loginRequiredHint')}</p>
          </div>
        </div>
      ) : (
        <>
          <InstanceStatsBar stats={config.data?.public_stats} className="mb-3.5" />
          {isGuest && guestUploadOn && guestLimits && (
            <div className="mb-[18px] flex items-center gap-2.5 border border-border bg-surface px-3.5 py-2.5 text-sm-plus text-muted">
              <span className="shrink-0 border border-border px-1.5 py-px font-mono text-2xs tracking-[0.08em] text-ink">
                {t('upload.guestMode')}
              </span>
              <span>
                {t('upload.guestLimits', {
                  size: formatBytes(guestLimits.max_file_size),
                  perDay: guestLimits.per_day,
                })}
              </span>
            </div>
          )}
          {/* 窄屏补位：顶栏 cluster 在 ≤900px 隐藏，上传页再露出用量，避免桌面双显 */}
          {!isGuest && quota.data && (
            <div
              className="mb-3.5 hidden flex-wrap gap-x-6 gap-y-4 rounded border border-border bg-surface px-3 py-2.5 max-[900px]:flex"
              data-testid="upload-quota-meters"
            >
              <QuotaBar used={quota.data.used} total={quota.data.total} kind="storage" to="/settings" />
              {bwQuota > 0 && (
                <QuotaBar used={bwUsed} total={bwQuota} kind="bandwidth" to="/settings" />
              )}
            </div>
          )}
        </>
      )}

      <div
        data-testid="dropzone"
        className={cn(
          'relative cursor-pointer rounded border border-dashed border-muted bg-surface px-6 text-center transition-[border-color] duration-150 hover:border-ink',
          needLogin ? 'cursor-default py-8 opacity-[0.78] hover:border-muted' : 'py-12',
          drag && 'border-ink',
        )}
        onClick={() => {
          if (needLogin) return pushToast(t('upload.toastLoginRequired'))
          if (full) return pushToast(t('upload.toastQuotaFull'))
          if (bwFull) return pushToast(t('upload.toastBandwidthFull'))
          fileInput.current?.click()
        }}
        onDragOver={(e) => {
          e.preventDefault()
          if (needLogin) return
          if (!drag) setDrag(true)
        }}
        onDragLeave={(e) => {
          if (dragLeftElement(e)) setDrag(false)
        }}
        onDrop={(e) => {
          e.preventDefault()
          e.stopPropagation()
          setPageDrag(false)
          setDrag(false)
          acceptFiles(e.dataTransfer.files)
        }}
      >
        {drag && (
          <div className="pointer-events-none absolute inset-0 z-[2] flex animate-[pulse_1.2s_infinite] items-center justify-center rounded border border-dashed border-ink bg-soft text-[15px] font-bold">
            {t('upload.dropRelease')}
          </div>
        )}
        {(full || bwFull) && (
          <div
            className="absolute inset-0 z-[3] flex cursor-not-allowed flex-col items-center justify-center gap-2 rounded bg-bg opacity-[0.92]"
            onClick={(e) => {
              e.stopPropagation()
              pushToast(full ? t('upload.toastQuotaFull') : t('upload.toastBandwidthFull'))
            }}
          >
            <span className="text-sm font-bold text-err">
              {full ? t('upload.fullTitle') : t('upload.bandwidthFullTitle')}
            </span>
            <span className="text-xs text-muted">
              {full ? t('upload.fullDesc') : t('upload.bandwidthFullDesc')}
            </span>
            {!!config.data?.upgrade_url?.trim() && (
              <a
                className="mt-2.5 inline-block text-sm-plus font-semibold text-ink underline underline-offset-2"
                href={config.data.upgrade_url.trim()}
                rel="noopener noreferrer"
                onClick={(e) => e.stopPropagation()}
              >
                {t('upload.upgradeLink')} →
              </a>
            )}
          </div>
        )}
        <div className="mx-auto mb-[18px] flex size-10 items-center justify-center rounded-sm border border-border bg-soft text-[17px] font-semibold text-ink">
          ↑
        </div>
        <div className="mb-[7px] text-[15px] font-bold tracking-[-0.005em]">{t('upload.dropTitle')}</div>
        <div className="mb-5 flex justify-center font-mono text-[11px] tracking-[0.05em] text-muted">
          {needLogin ? (
            t('upload.loginRequiredDropHint')
          ) : limits ? (
            `${extLabel(limits.allowedExts)} — MAX ${formatBytes(limits.maxFileSize)}`
          ) : (
            <Skeleton width={220} height={11} />
          )}
        </div>
        <div className="inline-flex items-center gap-2 text-sm-plus text-muted">
          <span className="rounded-[2px] border border-border bg-soft px-1.5 py-0.5 font-mono text-[11px] text-ink">
            Ctrl
          </span>
          <span className="rounded-[2px] border border-border bg-soft px-1.5 py-0.5 font-mono text-[11px] text-ink">
            V
          </span>
          {t('upload.pasteHint')}
        </div>
        <input
          ref={fileInput}
          type="file"
          accept="image/*"
          multiple
          className="hidden"
          disabled={needLogin}
          onChange={(e) => {
            if (e.target.files?.length) acceptFiles(e.target.files)
            e.target.value = ''
          }}
        />
      </div>

      <div
        className={cn(
          'mt-2.5 flex overflow-hidden rounded-sm border border-border bg-surface',
          needLogin && 'opacity-65 [&_button]:cursor-not-allowed [&_input:disabled]:cursor-not-allowed',
        )}
      >
        <span className="flex shrink-0 items-center border-r border-border bg-soft px-3 font-mono text-2xs tracking-[0.1em] text-muted">
          URL
        </span>
        <input
          className="min-w-0 flex-1 border-0 bg-transparent px-3 py-2.5 font-mono text-xs text-ink outline-none"
          value={fetchUrl}
          placeholder={t('upload.urlPlaceholder')}
          disabled={needLogin}
          onChange={(e) => setFetchUrl(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') doFetch()
          }}
        />
        <button
          type="button"
          className="shrink-0 cursor-pointer border-0 border-l border-border bg-surface px-[18px] text-xs font-bold text-ink hover:bg-soft disabled:cursor-not-allowed"
          disabled={needLogin}
          onClick={doFetch}
        >
          {t('upload.fetch')}
        </button>
      </div>

      {!isGuest && (
        <div className="mt-2.5 overflow-hidden rounded-sm border border-border bg-surface">
          <button
            type="button"
            className="flex w-full cursor-pointer items-center justify-between border-0 bg-transparent px-3.5 py-[11px] text-sm-plus font-bold text-ink hover:bg-soft"
            onClick={() => setOptsOpen((v) => !v)}
          >
            <span className="flex items-center gap-2">
              <span className="font-mono text-2xs tracking-[0.1em] text-muted">OPTIONS</span>
              {t('upload.options')}
            </span>
            <span className="flex items-center gap-2.5">
              <span className="font-mono text-xs-plus font-normal text-muted">{summary}</span>
              <span className="text-2xs text-muted">{optsOpen ? '▲' : '▼'}</span>
            </span>
          </button>
          {optsOpen && (
            <div className="grid animate-[fadeIn_0.15s] grid-cols-2 gap-3.5 border-t border-border px-3.5 pt-3.5 pb-4 max-[560px]:grid-cols-1">
              <div className="flex flex-col gap-1.5">
                <label className="text-[11.5px] font-semibold text-muted" htmlFor="opt-album">
                  {t('upload.uploadToAlbum')}
                </label>
                <select
                  id="opt-album"
                  className="cursor-pointer rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-sm-plus text-ink outline-none"
                  value={albumId ?? 'none'}
                  onChange={(e) => {
                    const next = e.target.value === 'none' ? null : Number(e.target.value)
                    setAlbumId(next)
                    const alb = albums.data?.items.find((a) => a.id === next)
                    if (alb?.visibility === 'private') setVisibility('private')
                  }}
                >
                  <option value="none">{t('upload.noAlbum')}</option>
                  {albums.data?.items.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="flex flex-col gap-1.5">
                <span className="text-[11.5px] font-semibold text-muted">{t('upload.visibility')}</span>
                <Segmented<'public' | 'private'>
                  options={[
                    { value: 'public', label: t('upload.public') },
                    { value: 'private', label: t('upload.private') },
                  ]}
                  value={visibility}
                  onChange={(v) => {
                    if (albumForcesPrivate) {
                      setVisibility('private')
                      return
                    }
                    setVisibility(v)
                  }}
                />
              </div>
              <div className="col-span-full flex flex-col gap-1.5">
                <span className="text-[11.5px] font-semibold text-muted">{t('upload.expiry')}</span>
                <Segmented
                  mono
                  options={expiryPresets.map((p) => ({
                    value: p.key,
                    label: expiryPresetLabel(p, t),
                  }))}
                  value={expiryKey}
                  onChange={(k) => {
                    const p = expiryPresets.find((x) => x.key === k)
                    setExpiresIn(p?.sec ?? (expiresCap > 0 ? expiresCap : 0))
                  }}
                />
              </div>
              <div className="col-span-full flex flex-col gap-1.5">
                <span className="text-[11.5px] font-semibold text-muted">{t('upload.maxViews')}</span>
                <Segmented
                  mono
                  options={maxViewsPresets.map((p) => ({
                    value: p.key,
                    label: maxViewsPresetLabel(p, t),
                  }))}
                  value={maxViewsKey}
                  onChange={(k) => {
                    const p = maxViewsPresets.find((x) => x.key === k)
                    setMaxViews(p?.n ?? (accessPolicy.max_max_views > 0 ? accessPolicy.max_max_views : 0))
                  }}
                />
              </div>
              {showPolicy && (
                <div className="flex flex-col gap-1.5">
                  <label className="text-[11.5px] font-semibold text-muted" htmlFor="opt-policy">
                    {t('upload.storagePolicy')}
                  </label>
                  <select
                    id="opt-policy"
                    className="cursor-pointer rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-sm-plus text-ink outline-none"
                    value={policyId ?? 'default'}
                    onChange={(e) => setPolicyId(e.target.value === 'default' ? null : Number(e.target.value))}
                  >
                    <option value="default">{t('upload.followDefault')}</option>
                    {policies.data?.map((po) => (
                      <option key={po.id} value={po.id}>
                        {po.name}
                      </option>
                    ))}
                  </select>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {!isGuest &&
        !!config.data?.plaza_enabled &&
        !!me &&
        !me.public_profile &&
        visibility === 'public' && (
          <div
            className="mt-2.5 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-sm border border-border bg-surface px-3.5 py-2.5 text-[12.5px] leading-snug text-muted"
            data-testid="plaza-opt-in-hint"
          >
            <span>{t('upload.plazaOptInHint')}</span>
            <Link
              to="/settings/profile"
              className="shrink-0 font-semibold text-ink underline underline-offset-2"
            >
              {t('upload.plazaOptInLink')}
            </Link>
          </div>
        )}

      {items.length > 0 && (
        <>
          <div className="mt-10 mb-3 flex items-baseline justify-between">
            <span className="font-mono text-[11px] tracking-[0.14em] text-muted">QUEUE — {items.length}</span>
            <span className="flex gap-3.5">
              <button
                type="button"
                className="cursor-pointer border-0 bg-transparent p-1 text-xs font-semibold text-muted hover:text-ink"
                onClick={copyAllLinks}
              >
                {t('upload.copyAllLinks')}
              </button>
              <button
                type="button"
                className="cursor-pointer border-0 bg-transparent p-1 text-xs font-semibold text-muted hover:text-ink"
                onClick={clearDone}
              >
                {t('upload.clearDone')}
              </button>
            </span>
          </div>
          <div className="flex flex-col gap-2">
            {items.map((i) => (
              <UploadCard key={i.id} item={i} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
