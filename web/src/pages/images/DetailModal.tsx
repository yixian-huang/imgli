import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { renderSVG } from 'uqr'
import { useAlbums, useDeleteImage, useImageDetail, useImageStats, useQuota, useUpdateImage } from '../../api/hooks'
import type { ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { albumForcesPrivate } from '../../lib/albumPrivacy'
import { cn } from '../../lib/cn'
import { copyText } from '../../lib/copy'
import { formatBytes, formatDate } from '../../lib/format'
import {
  expiryPresetLabel,
  filterExpiryPresets,
  filterMaxViewsPresets,
  groupExpiresCapSec,
  maxViewsPresetLabel,
} from '../../lib/imageAccessPresets'
import { generateAccessPassword } from '../../lib/password'
import { useGlobal } from '../../store'
import { InlineConfirm } from '../../ui/InlineConfirm'
import { RetryImg } from '../../ui/RetryImg'
import { Segmented } from '../../ui/Segmented'

interface Props {
  items: ImageItem[]
  focusKey: string
  onClose(): void
  onNavigate(key: string): void
}

const renameInput =
  'box-border w-full min-w-0 max-w-full flex-1 rounded-sm border border-border bg-bg px-[9px] py-1.5 font-mono text-sm-plus text-ink outline-none'
const renameSave =
  'cursor-pointer rounded-sm border-0 bg-btn px-3 text-[11.5px] font-bold text-btn-text disabled:cursor-not-allowed disabled:opacity-50'
const removeExpiry =
  'cursor-pointer self-start rounded-[2px] border border-border bg-surface px-2.5 py-1 text-[11px] font-semibold text-ink hover:bg-soft disabled:cursor-not-allowed disabled:opacity-50'
const accessChip =
  'inline-flex max-w-full items-center overflow-hidden rounded-full border border-border bg-soft px-[7px] py-0.5 font-mono text-xs-plus font-medium leading-snug text-ellipsis whitespace-nowrap text-muted'
const sectionBase = 'flex w-full min-w-0 max-w-full min-h-0 box-border flex-col gap-2'
const sectionTitle = 'text-[11px] font-bold tracking-[0.04em] text-muted'
const sectionSub = 'min-w-0 text-right text-[11px] font-medium text-muted'
const kicker = 'mb-1 font-mono text-2xs tracking-[0.14em] text-muted'
const metaKey = 'text-muted'
const metaVal = 'min-w-0 overflow-hidden font-mono text-[11px] text-ellipsis max-md:break-all'
const policyWarn =
  'mb-2 flex flex-wrap items-center gap-2 rounded-sm border border-[color-mix(in_srgb,var(--warn)_45%,var(--border))] bg-[color-mix(in_srgb,var(--warn)_12%,var(--surface))] px-2.5 py-2 text-[11.5px] leading-snug text-ink'
const policyFix =
  'cursor-pointer whitespace-nowrap rounded-[2px] border border-border bg-surface px-2 py-[3px] text-[11px] font-semibold text-ink hover:border-muted disabled:cursor-not-allowed disabled:opacity-55'

/** 自定义说明浮层：原生 title 延迟长、易被裁切；portal 到 body 保证可见 */
function HelpTip({ text }: { text: string }) {
  const btnRef = useRef<HTMLButtonElement>(null)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ top: number; left: number; maxW: number }>({
    top: 0,
    left: 0,
    maxW: 280,
  })

  const place = () => {
    const el = btnRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const maxW = Math.min(300, Math.max(200, window.innerWidth - 24))
    // 优先在下方；贴底则翻到上方
    let top = r.bottom + 6
    const approxH = 110
    if (top + approxH > window.innerHeight - 8) {
      top = Math.max(8, r.top - approxH - 6)
    }
    let left = r.left
    if (left + maxW > window.innerWidth - 12) {
      left = Math.max(12, window.innerWidth - maxW - 12)
    }
    setPos({ top, left, maxW })
  }

  const clearHide = () => {
    if (hideTimer.current) {
      clearTimeout(hideTimer.current)
      hideTimer.current = null
    }
  }

  const show = () => {
    clearHide()
    place()
    setOpen(true)
  }

  const hideSoon = () => {
    clearHide()
    // 短延迟：鼠标从 ? 移到气泡时不闪断
    hideTimer.current = setTimeout(() => setOpen(false), 140)
  }

  const hideNow = () => {
    clearHide()
    setOpen(false)
  }

  useEffect(() => () => clearHide(), [])

  useEffect(() => {
    if (!open) return
    const onReposition = () => place()
    window.addEventListener('scroll', onReposition, true)
    window.addEventListener('resize', onReposition)
    return () => {
      window.removeEventListener('scroll', onReposition, true)
      window.removeEventListener('resize', onReposition)
    }
  }, [open])

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        className={cn(
          'inline-flex h-4 w-4 flex-none cursor-help select-none items-center justify-center rounded-full border border-border bg-soft p-0 font-mono text-2xs font-bold leading-none text-muted',
          'hover:border-ink hover:text-ink focus-visible:border-ink focus-visible:text-ink focus-visible:outline-none',
          open && 'border-ink text-ink',
        )}
        aria-label={text}
        aria-expanded={open}
        onMouseEnter={show}
        onMouseLeave={hideSoon}
        onFocus={show}
        onBlur={hideSoon}
        onClick={(e) => {
          // 移动端无 hover：点按切换
          e.preventDefault()
          e.stopPropagation()
          if (open) hideNow()
          else show()
        }}
      >
        ?
      </button>
      {open &&
        createPortal(
          <div
            role="tooltip"
            className="pointer-events-auto fixed z-100 box-border animate-[fadeIn_0.1s_ease-out] rounded border border-border bg-surface px-2.5 py-2 text-[11.5px] font-medium leading-[1.45] break-words whitespace-normal text-ink shadow-[0_6px_20px_rgba(0,0,0,0.12)]"
            style={{ top: pos.top, left: pos.left, maxWidth: pos.maxW }}
            onMouseEnter={show}
            onMouseLeave={hideSoon}
          >
            {text}
          </div>,
          document.body,
        )}
    </>
  )
}

/**
 * 桌面：右栏 2×2（上 元信息|链接，下 访问|统计）
 * 窄屏/移动：单列顺序 元信息 → 链接 → 访问 → 统计
 */
export function DetailModal({ items, focusKey, onClose, onNavigate }: Props) {
  const { t, lang } = useT()
  const idx = items.findIndex((i) => i.key === focusKey)
  const base = items[idx]
  const detail = useImageDetail(focusKey)
  const stats = useImageStats(focusKey)
  const albums = useAlbums()
  const quota = useQuota()
  const update = useUpdateImage()
  const remove = useDeleteImage()
  const pushToast = useGlobal((s) => s.pushToast)
  const [renaming, setRenaming] = useState(false)
  const [renameVal, setRenameVal] = useState('')
  const [moving, setMoving] = useState(false)
  const [expiryKey, setExpiryKey] = useState<string>('never')
  const [accessPw, setAccessPw] = useState('')
  const paneScrollRef = useRef<HTMLDivElement>(null)
  const accessSectionRef = useRef<HTMLElement>(null)
  const passwordInputRef = useRef<HTMLInputElement>(null)

  // 与上传页一致：按用户组 max/force 过滤有效期与访问次数预设
  const expiresCap = groupExpiresCapSec({
    max_expires_in: quota.data?.max_expires_in,
    force_max_age_days: quota.data?.force_max_age_days,
  })
  const maxMaxViews = quota.data?.max_max_views ?? 0
  const expiryPresets = filterExpiryPresets(expiresCap)
  const maxViewsPresets = filterMaxViewsPresets(maxMaxViews)
  const allowPermanent = expiresCap <= 0
  const allowUnlimitedViews = maxMaxViews <= 0

  const prevKey = idx > 0 ? items[idx - 1].key : null
  const nextKey = idx >= 0 && idx < items.length - 1 ? items[idx + 1].key : null

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = document.activeElement
      const editing = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement
      if (e.key === 'Escape') {
        onClose()
        return
      }
      if (editing) return
      if (e.key === 'ArrowLeft' && prevKey) onNavigate(prevKey)
      if (e.key === 'ArrowRight' && nextKey) onNavigate(nextKey)
    }
    document.addEventListener('keydown', onKey)

    const scrollY = window.scrollY
    const prev = {
      htmlOverflow: document.documentElement.style.overflow,
      bodyOverflow: document.body.style.overflow,
      bodyPosition: document.body.style.position,
      bodyTop: document.body.style.top,
      bodyLeft: document.body.style.left,
      bodyRight: document.body.style.right,
      bodyWidth: document.body.style.width,
    }
    document.documentElement.style.overflow = 'hidden'
    document.body.style.overflow = 'hidden'
    document.body.style.position = 'fixed'
    document.body.style.top = `-${scrollY}px`
    document.body.style.left = '0'
    document.body.style.right = '0'
    document.body.style.width = '100%'

    return () => {
      document.removeEventListener('keydown', onKey)
      document.documentElement.style.overflow = prev.htmlOverflow
      document.body.style.overflow = prev.bodyOverflow
      document.body.style.position = prev.bodyPosition
      document.body.style.top = prev.bodyTop
      document.body.style.left = prev.bodyLeft
      document.body.style.right = prev.bodyRight
      document.body.style.width = prev.bodyWidth
      document.documentElement.scrollTop = scrollY
      document.body.scrollTop = scrollY
    }
  }, [prevKey, nextKey, onClose, onNavigate])

  useEffect(() => {
    setRenaming(false)
    setMoving(false)
    setExpiryKey('never')
    setAccessPw('')
    if (paneScrollRef.current) paneScrollRef.current.scrollTop = 0
  }, [focusKey])

  const sharePageUrl = (() => {
    if (!base || base.visibility !== 'public') return null
    if (base.links.share_url) return base.links.share_url
    if (typeof window === 'undefined') return null
    return `${window.location.origin}/s/${base.key}`
  })()

  const qrUrl = sharePageUrl || base?.links.url || ''
  const qrSVG = useMemo(() => (qrUrl ? renderSVG(qrUrl) : ''), [qrUrl])

  if (!base) return null
  const d = detail.data
  const albumName = base.album_id
    ? (albums.data?.items.find((a) => a.id === base.album_id)?.name ?? `#${base.album_id}`)
    : t('images.uncategorized')
  const visLockedPublic = albumForcesPrivate(albums.data?.items, base.album_id)
  const expiresAt = d?.expires_at !== undefined ? d.expires_at : base.expires_at
  const expiryDisplay = (() => {
    if (!expiresAt) return t('images.permanent')
    const date = new Date(expiresAt).toLocaleDateString(lang === 'zh' ? 'zh-CN' : 'en-US')
    return t('images.expiresOn', { date })
  })()

  const commitRename = () => {
    const name = renameVal.trim()
    if (!name || name === base.name) return setRenaming(false)
    update.mutate({ key: base.key, body: { name } }, {
      onSuccess: () => {
        setRenaming(false)
        pushToast(t('images.renamed'))
      },
    })
  }

  const maxViews = d?.max_views ?? base.max_views ?? 0
  const viewsServed = d?.views_served ?? base.views_served ?? 0
  const maxViewsKey =
    maxViewsPresets.find((p) => p.n === maxViews)?.key
    ?? maxViewsPresets[0]?.key
    ?? 'unlimited'
  const expiryKeyForUi =
    expiryPresets.find((p) => p.key === expiryKey)?.key
    ?? expiryPresets[0]?.key
    ?? 'never'
  // 图上当前值是否超出组策略（旧图 / 改组后）
  const expiryOutOfPolicy =
    (!allowPermanent && !expiresAt)
    || (expiresCap > 0 && !!expiresAt && new Date(expiresAt).getTime() > Date.now() + expiresCap * 1000)
  const maxViewsOutOfPolicy =
    (!allowUnlimitedViews && maxViews <= 0)
    || (maxMaxViews > 0 && maxViews > maxMaxViews)
  const setMaxViews = (n: number) => {
    if (!allowUnlimitedViews && n <= 0) return
    if (maxMaxViews > 0 && n > maxMaxViews) return
    update.mutate({ key: base.key, body: { max_views: n } })
  }
  const setExpiry = (sec: number) => {
    if (!allowPermanent && sec <= 0) return
    if (expiresCap > 0 && sec > expiresCap) return
    update.mutate({ key: base.key, body: { expires_in: sec } })
  }
  const applyGroupMaxExpiry = () => {
    if (expiresCap <= 0) return
    const hit = expiryPresets.find((p) => p.sec === expiresCap)
    if (hit) setExpiryKey(hit.key)
    setExpiry(expiresCap)
  }
  const applyGroupMaxViews = () => {
    if (maxMaxViews <= 0) return
    setMaxViews(maxMaxViews)
  }

  const hasAccessPassword = !!(d?.has_access_password ?? base.has_access_password)
  const maxViewsChip =
    maxViews > 0
      ? t('images.accessChipViews', { used: viewsServed, max: maxViews })
      : t('images.accessChipUnlimited')
  const passwordChip = hasAccessPassword
    ? t('images.accessPasswordSet')
    : t('images.accessPasswordNone')

  const setAccessPassword = (password: string) => {
    update.mutate(
      { key: base.key, body: { access_password: password } },
      {
        onSuccess: () => {
          setAccessPw('')
          pushToast(
            password
              ? t('images.accessPasswordSet')
              : t('images.accessPasswordNone'),
          )
        },
      },
    )
  }
  const fillRandomPassword = () => {
    setAccessPw(generateAccessPassword(10))
  }

  const jumpToAccess = () => {
    const pane = paneScrollRef.current
    const section = accessSectionRef.current
    if (pane && section) {
      const top = section.offsetTop - 8
      if (typeof pane.scrollTo === 'function') {
        pane.scrollTo({ top, behavior: 'smooth' })
      } else {
        pane.scrollTop = top
      }
    }
    requestAnimationFrame(() => passwordInputRef.current?.focus({ preventScroll: true }))
  }

  const copyRows = [
    ...(sharePageUrl ? [{ kind: t('images.sharePage'), text: sharePageUrl }] : []),
    { kind: 'URL', text: base.links.url },
    { kind: 'MD', text: base.links.markdown },
    { kind: 'HTML', text: base.links.html },
    { kind: 'BBCODE', text: base.links.bbcode },
    { kind: t('images.thumbnail'), text: base.links.thumbnail_url },
  ]

  const visLabel = base.visibility === 'public' ? 'PUBLIC' : 'PRIVATE'
  const statsAvailable = !stats.isError && !!stats.data

  const sectionDivider =
    'max-[1100px]:border-r-0 max-[1100px]:pr-0 max-[1100px]:border-b max-[1100px]:border-border max-[1100px]:pb-3.5 min-[1101px]:border-r min-[1101px]:border-border min-[1101px]:pr-4'

  return createPortal(
    <div
      className="fixed inset-0 z-40 flex animate-[fadeIn_0.15s] items-end overflow-hidden overscroll-none bg-black/50 p-0 md:items-center md:justify-center md:p-4"
      onClick={onClose}
    >
      {prevKey && (
        <button
          type="button"
          title={t('images.prev')}
          className="absolute top-1/2 left-3 z-41 hidden h-[38px] w-[38px] -translate-y-1/2 cursor-pointer rounded-sm border border-white/25 bg-black/35 text-[15px] text-white hover:bg-black/60 md:block"
          onClick={(e) => {
            e.stopPropagation()
            onNavigate(prevKey)
          }}
        >
          ‹
        </button>
      )}
      {nextKey && (
        <button
          type="button"
          title={t('images.next')}
          className="absolute top-1/2 right-3 z-41 hidden h-[38px] w-[38px] -translate-y-1/2 cursor-pointer rounded-sm border border-white/25 bg-black/35 text-[15px] text-white hover:bg-black/60 md:block"
          onClick={(e) => {
            e.stopPropagation()
            onNavigate(nextKey)
          }}
        >
          ›
        </button>
      )}
      <div
        role="dialog"
        aria-modal="true"
        className="flex h-[min(94vh,100%)] max-h-[94vh] w-full min-h-0 max-w-none flex-col overflow-hidden overscroll-contain rounded-t-[12px] border border-border border-b-0 bg-surface md:h-[min(92vh,900px)] md:max-h-[92vh] md:max-w-[1280px] md:flex-row md:rounded md:border-b max-[1100px]:md:max-w-[1000px]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mx-auto mt-2 mb-0 h-1 w-9 flex-none rounded-[2px] bg-border md:hidden" />
        <div className="relative flex max-h-[22vh] min-h-[120px] flex-none flex-col items-center justify-center gap-2 overflow-hidden border-b border-border bg-soft md:max-h-none md:min-h-0 md:flex-[1.1] md:border-r md:border-b-0">
          <RetryImg
            className="max-h-[20vh] max-w-full object-contain md:max-h-[min(80vh,100%)]"
            src={base.links.url}
            alt={base.name}
          />
          <span className="absolute bottom-2.5 rounded-[2px] border border-border bg-surface px-[7px] py-px font-mono text-2xs text-muted">
            {idx + 1} / {items.length}
          </span>
        </div>
        <div className="flex min-h-0 w-full min-w-0 max-w-none flex-1 flex-col overflow-hidden md:w-[min(640px,52%)] md:min-w-[420px] md:max-w-[680px] md:flex-[0_0_min(640px,52%)] max-[1100px]:md:w-[min(420px,48%)] max-[1100px]:md:min-w-[340px] max-[1100px]:md:max-w-[460px] max-[1100px]:md:flex-[0_0_min(420px,48%)]">
          <div className="flex flex-none items-start justify-between gap-2.5 border-b border-border bg-surface px-3 py-2 md:px-4 md:pt-3 md:pb-2.5">
            <div className="min-w-0 flex-1">
              <div className={kicker}>DETAIL</div>
              {renaming ? (
                <div className="flex gap-1.5">
                  <input
                    className={renameInput}
                    autoFocus
                    value={renameVal}
                    onChange={(e) => setRenameVal(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') commitRename()
                    }}
                  />
                  <button type="button" className={renameSave} onClick={commitRename}>
                    {t('images.save')}
                  </button>
                </div>
              ) : (
                <div className="flex items-baseline gap-2">
                  <span className="text-sm font-bold leading-snug break-all">{base.name}</span>
                  <button
                    type="button"
                    className="flex-none cursor-pointer border-0 bg-transparent p-0.5 text-[11px] text-muted underline hover:text-ink"
                    onClick={() => {
                      setRenameVal(base.name)
                      setRenaming(true)
                    }}
                  >
                    {t('images.rename')}
                  </button>
                </div>
              )}
              {/* 状态一览：点芯片可跳到访问区 */}
              <div className="mt-2 flex flex-wrap gap-1" aria-label={t('images.accessControlHint')}>
                <button
                  type="button"
                  className="cursor-pointer border-0 bg-transparent p-0 font-inherit leading-none hover:[&_span]:border-ink hover:[&_span]:text-ink"
                  onClick={jumpToAccess}
                >
                  <span className={accessChip}>{expiryDisplay}</span>
                </button>
                <button
                  type="button"
                  className="cursor-pointer border-0 bg-transparent p-0 font-inherit leading-none hover:[&_span]:border-ink hover:[&_span]:text-ink"
                  onClick={jumpToAccess}
                >
                  <span className={accessChip}>{maxViewsChip}</span>
                </button>
                <button
                  type="button"
                  className="cursor-pointer border-0 bg-transparent p-0 font-inherit leading-none hover:[&_span]:border-ink hover:[&_span]:text-ink"
                  onClick={jumpToAccess}
                >
                  <span className={cn(accessChip, hasAccessPassword && 'border-ink bg-surface text-ink')}>
                    {passwordChip}
                  </span>
                </button>
              </div>
            </div>
            <button
              type="button"
              className="flex-none cursor-pointer border-0 bg-transparent px-1.5 py-0.5 text-lg leading-none text-muted hover:text-ink"
              onClick={onClose}
            >
              ×
            </button>
          </div>

          <div
            className="flex min-h-0 flex-1 flex-col gap-0 overflow-x-hidden overflow-y-auto overscroll-contain touch-pan-y scroll-pt-2 scroll-pb-4 px-3 pt-2.5 pb-4 md:px-4 md:pt-3.5 md:pb-[18px]"
            ref={paneScrollRef}
          >
            {/* 上排：元信息 | 链接 */}
            <div className="mb-3.5 grid w-full min-w-0 grid-cols-1 items-stretch gap-4 border-b border-border pb-3.5 max-w-full min-[1101px]:grid-cols-2 min-[1101px]:gap-x-5 min-[1101px]:gap-y-0">
              <section className={cn(sectionBase, sectionDivider)} aria-label={t('images.tabInfo')}>
                <div className="flex flex-none items-baseline justify-between gap-2">
                  <span className={sectionTitle}>{t('images.tabInfo')}</span>
                </div>
                <div className="grid w-full min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-xs">
                  <span className={metaKey}>{t('images.dims')}</span>
                  <span className={metaVal}>
                    {base.width} × {base.height}
                  </span>
                  <span className={metaKey}>{t('images.size')}</span>
                  <span className={metaVal}>{formatBytes(base.size)}</span>
                  <span className={metaKey}>{t('images.mime')}</span>
                  <span className={metaVal}>{d?.mime ?? '…'}</span>
                  <span className={metaKey}>{t('images.uploadedAt')}</span>
                  <span className={metaVal}>{formatDate(base.created_at)}</span>
                  <span className={metaKey}>{t('images.album')}</span>
                  <span className={metaVal}>{albumName}</span>
                  <span className={metaKey}>{t('images.visibility')}</span>
                  <button
                    type="button"
                    className="max-w-full justify-self-start cursor-pointer rounded-[2px] border border-border bg-surface px-2 py-0.5 font-mono text-2xs font-semibold text-ink hover:bg-soft disabled:opacity-50"
                    disabled={update.isPending || (visLockedPublic && base.visibility === 'private')}
                    title={visLockedPublic && base.visibility === 'private' ? t('images.albumForcesPrivate') : undefined}
                    onClick={() => {
                      const next = base.visibility === 'public' ? 'private' : 'public'
                      if (next === 'public' && visLockedPublic) {
                        pushToast(t('images.albumForcesPrivate'))
                        return
                      }
                      update.mutate({ key: base.key, body: { visibility: next } })
                    }}
                  >
                    {visLabel} — {t('images.clickToToggle')}
                  </button>
                  <span className={metaKey}>{t('images.slug')}</span>
                  <span className={metaVal}>
                    <input
                      className={renameInput}
                      defaultValue={base.slug ?? ''}
                      placeholder="my-photo"
                      onBlur={(e) => {
                        const v = e.target.value.trim().toLowerCase()
                        const cur = (base.slug ?? '').toLowerCase()
                        if (v === cur) return
                        update.mutate({ key: base.key, body: { slug: v } })
                      }}
                    />
                  </span>
                  <span className={metaKey}>{t('images.uploadIp')}</span>
                  <span className={cn(metaVal, 'text-muted')}>
                    {d ? t('images.ipSelfOnly', { ip: d.upload_ip || '—' }) : '…'}
                  </span>
                </div>
              </section>

              <section className={sectionBase} aria-label={t('images.tabLinks')}>
                <div className="flex flex-none items-baseline justify-between gap-2">
                  <span className={sectionTitle}>{t('images.tabLinks')}</span>
                </div>
                <div className="flex w-full max-w-full min-w-0 flex-col gap-2">
                  <div className="flex w-full max-w-full min-w-0 flex-col gap-[5px]">
                    {copyRows.map((r) => (
                      <div
                        key={r.kind}
                        className="box-border grid w-full max-w-full min-w-0 grid-cols-[2.75rem_minmax(0,1fr)_3.25rem] items-stretch overflow-hidden rounded-sm border border-border bg-bg md:grid-cols-[3rem_minmax(0,1fr)_auto]"
                      >
                        <span className="flex min-w-0 items-center justify-center overflow-hidden border-r border-border bg-soft px-0.5 font-mono text-[8px] tracking-[0.02em] text-ellipsis whitespace-nowrap text-muted md:px-1 md:text-[8.5px]">
                          {r.kind}
                        </span>
                        <span className="min-w-0 self-center overflow-hidden px-1.5 py-2 font-mono text-2xs text-ellipsis whitespace-nowrap text-ink md:px-2 md:py-[7px] md:text-xs-plus">
                          {r.text}
                        </span>
                        <button
                          type="button"
                          className="min-h-10 min-w-[3.25rem] shrink-0 cursor-pointer whitespace-nowrap border-0 border-l border-border bg-surface px-2 text-xs font-semibold text-ink hover:bg-soft md:min-h-0 md:min-w-[2.75rem] md:px-3 md:text-[11px]"
                          onClick={() => copyText(r.text, t('images.linkLabel', { kind: r.kind }))}
                        >
                          {t('images.copy')}
                        </button>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-none flex-row items-center gap-2 min-w-0">
                    <div className={cn(kicker, 'mb-0')}>QR</div>
                    <div
                      className="h-16 w-16 flex-none overflow-hidden rounded-sm border border-border bg-white md:h-[72px] md:w-[72px] [&_svg]:block [&_svg]:h-full [&_svg]:w-full"
                      dangerouslySetInnerHTML={{ __html: qrSVG }}
                    />
                  </div>
                </div>
              </section>
            </div>

            {/* 下排：访问 | 统计（无统计时访问占满） */}
            <div
              className={cn(
                'grid w-full max-w-full min-w-0 grid-cols-1 items-stretch gap-3.5 md:gap-4 min-[1101px]:gap-x-5 min-[1101px]:gap-y-0',
                statsAvailable ? 'min-[1101px]:grid-cols-2' : 'min-[1101px]:grid-cols-1',
              )}
            >
              <section
                ref={accessSectionRef}
                className={cn(
                  sectionBase,
                  'max-h-none overflow-visible min-w-0 max-[1100px]:max-h-none min-[1101px]:max-h-[min(42vh,380px)] min-[1101px]:overflow-x-hidden min-[1101px]:overflow-y-auto min-[1101px]:overscroll-contain',
                  statsAvailable && sectionDivider,
                )}
                aria-label={t('images.accessControl')}
              >
                <div className="flex flex-none items-baseline justify-between gap-2">
                  <span className={sectionTitle}>{t('images.accessControl')}</span>
                  <span className={sectionSub}>{t('images.accessControlHint')}</span>
                </div>

                <div className="flex min-w-0 flex-col gap-3">
                  <div className="flex max-w-full min-w-0 flex-col gap-1.5">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="inline-flex items-center gap-[5px] text-xs font-semibold text-ink">
                        {t('images.accessPassword')}
                        <HelpTip text={t('images.accessPasswordHint')} />
                      </span>
                      <span className="min-w-0 text-right font-mono text-[11px] text-muted">
                        {hasAccessPassword
                          ? t('images.accessPasswordSet')
                          : t('images.accessPasswordNone')}
                      </span>
                    </div>
                    <input
                      ref={passwordInputRef}
                      className={renameInput}
                      type="text"
                      value={accessPw}
                      placeholder={t('images.accessPasswordPlaceholder')}
                      onChange={(e) => setAccessPw(e.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                    />
                    <div className="flex flex-wrap items-center gap-1.5">
                      <button
                        type="button"
                        className={removeExpiry}
                        disabled={update.isPending}
                        onClick={fillRandomPassword}
                      >
                        {t('images.accessPasswordGenerate')}
                      </button>
                      {accessPw.trim() ? (
                        <button
                          type="button"
                          className={removeExpiry}
                          onClick={() => copyText(accessPw.trim(), t('images.accessPassword'))}
                        >
                          {t('images.accessPasswordCopy')}
                        </button>
                      ) : null}
                      <button
                        type="button"
                        className={cn(renameSave, 'min-h-7 px-3 py-1')}
                        disabled={!accessPw.trim() || update.isPending}
                        onClick={() => setAccessPassword(accessPw.trim())}
                      >
                        {t('images.accessPasswordSave')}
                      </button>
                      {hasAccessPassword && (
                        <button
                          type="button"
                          className={removeExpiry}
                          disabled={update.isPending}
                          onClick={() => setAccessPassword('')}
                        >
                          {t('images.accessPasswordClear')}
                        </button>
                      )}
                    </div>
                  </div>

                  <div className="flex max-w-full min-w-0 flex-col gap-1.5 border-t border-dashed border-border pt-2.5">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="inline-flex items-center gap-[5px] text-xs font-semibold text-ink">
                        {t('images.expiry')}
                        <HelpTip text={t('images.expiryWarn')} />
                      </span>
                      <span className="min-w-0 text-right font-mono text-[11px] text-muted">{expiryDisplay}</span>
                    </div>
                    {expiryOutOfPolicy && (
                      <p className={policyWarn}>
                        {t('images.expiryOutOfPolicy', {
                          max: expiresCap > 0
                            ? expiryPresetLabel({ key: `cap:${expiresCap}`, sec: expiresCap }, t)
                            : '—',
                        })}
                        {expiresCap > 0 && (
                          <button
                            type="button"
                            className={policyFix}
                            disabled={update.isPending}
                            onClick={applyGroupMaxExpiry}
                          >
                            {t('images.applyGroupMaxExpiry')}
                          </button>
                        )}
                      </p>
                    )}
                    <Segmented
                      mono
                      compact
                      options={expiryPresets.map((p) => ({
                        value: p.key,
                        label: expiryPresetLabel(p, t),
                      }))}
                      value={expiryKeyForUi}
                      onChange={(k) => {
                        setExpiryKey(k)
                        const p = expiryPresets.find((x) => x.key === k)
                        setExpiry(p?.sec ?? (allowPermanent ? 0 : expiresCap))
                      }}
                    />
                    {expiresAt && allowPermanent && (
                      <button
                        type="button"
                        className={removeExpiry}
                        disabled={update.isPending}
                        onClick={() => {
                          setExpiryKey('never')
                          setExpiry(0)
                        }}
                      >
                        {t('images.removeExpiry')}
                      </button>
                    )}
                  </div>

                  <div className="flex max-w-full min-w-0 flex-col gap-1.5 border-t border-dashed border-border pt-2.5">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="inline-flex items-center gap-[5px] text-xs font-semibold text-ink">
                        {t('images.maxViews')}
                        <HelpTip text={t('images.maxViewsHint')} />
                      </span>
                      <span className="min-w-0 text-right font-mono text-[11px] text-muted">
                        {maxViews > 0
                          ? t('images.maxViewsUsed', { used: viewsServed, max: maxViews })
                          : t('upload.maxViewsUnlimited')}
                      </span>
                    </div>
                    {maxViewsOutOfPolicy && (
                      <p className={policyWarn}>
                        {t('images.maxViewsOutOfPolicy', { max: maxMaxViews })}
                        {maxMaxViews > 0 && (
                          <button
                            type="button"
                            className={policyFix}
                            disabled={update.isPending}
                            onClick={applyGroupMaxViews}
                          >
                            {t('images.applyGroupMaxViews')}
                          </button>
                        )}
                      </p>
                    )}
                    <Segmented
                      mono
                      compact
                      options={maxViewsPresets.map((p) => ({
                        value: p.key,
                        label: maxViewsPresetLabel(p, t),
                      }))}
                      value={maxViewsKey}
                      onChange={(k) => {
                        const p = maxViewsPresets.find((x) => x.key === k)
                        setMaxViews(p?.n ?? (allowUnlimitedViews ? 0 : maxMaxViews))
                      }}
                    />
                  </div>

                  {/* 长说明默认收起：桌面悬停 ?，移动端点开「说明」 */}
                  <details className="group m-0 border-t border-dashed border-border pt-2">
                    <summary className="flex cursor-pointer list-none items-center gap-1 text-[11px] font-semibold text-muted select-none hover:text-ink [&::-webkit-details-marker]:hidden">
                      <span className="text-2xs transition-transform duration-100 group-open:rotate-90">▸</span>
                      {t('images.accessNotes')}
                    </summary>
                    <ul className="mt-1.5 mb-0 list-disc py-0 pl-[1.1em] text-[11px] leading-[1.45] text-muted">
                      <li className="mt-0">
                        <strong className="font-semibold text-ink">{t('images.accessPassword')}</strong>
                        {' — '}
                        {t('images.accessPasswordHint')}
                      </li>
                      <li className="mt-1">
                        <strong className="font-semibold text-ink">{t('images.expiry')}</strong>
                        {' — '}
                        {t('images.expiryWarn')}
                      </li>
                      <li className="mt-1">
                        <strong className="font-semibold text-ink">{t('images.maxViews')}</strong>
                        {' — '}
                        {t('images.maxViewsHint')}{' '}
                        {t('images.maxViewsNoStorage')}
                      </li>
                    </ul>
                  </details>
                </div>
              </section>

              {statsAvailable && (
                <section
                  className={cn(
                    sectionBase,
                    'max-h-none overflow-visible min-w-0 min-[1101px]:max-h-[min(42vh,380px)] min-[1101px]:overflow-x-hidden min-[1101px]:overflow-y-auto min-[1101px]:overscroll-contain',
                  )}
                  aria-label={t('images.accessStats')}
                >
                  <div className="flex flex-none items-baseline justify-between gap-2">
                    <span className={sectionTitle}>{t('images.accessStats')}</span>
                    <span className={sectionSub}>
                      {t('images.totalViews', { count: stats.data!.total })}
                    </span>
                  </div>
                  <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2">
                    <div className={kicker}>ACCESS — {t('images.accessLabel')}</div>
                    {(() => {
                      const daily = stats.data!.daily
                      const max = daily.reduce((m, day) => (day.views > m ? day.views : m), 0)
                      if (max === 0) {
                        return <div className="py-2 text-xs text-muted">{t('images.noAccess')}</div>
                      }
                      return (
                        <div
                          className="accessBars flex h-12 min-h-12 min-w-0 flex-1 items-end gap-0.5 py-1 md:h-[72px] md:min-h-14"
                          aria-label={t('images.last30Days')}
                        >
                          {daily.map((day) => (
                            <div
                              key={day.date}
                              className="accessBar min-w-0 flex-1 rounded-t-[1px] bg-btn opacity-75 hover:opacity-100"
                              title={`${day.date}: ${day.views}`}
                              style={{
                                height:
                                  day.views > 0
                                    ? `${Math.max(4, Math.round((day.views / max) * 100))}%`
                                    : '0%',
                              }}
                            />
                          ))}
                        </div>
                      )
                    })()}
                  </div>
                </section>
              )}
            </div>
          </div>

          <div className="z-[1] flex flex-none gap-2 border-t border-border bg-surface px-3 pt-2.5 pb-3.5 shadow-[0_-6px_12px_-10px_rgba(0,0,0,0.16)] md:px-3.5 md:pt-2.5 md:pb-3 [&>*]:flex-1">
            {moving ? (
              <select
                className="cursor-pointer rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-sm-plus text-ink outline-none"
                aria-label={t('images.moveToAlbum')}
                autoFocus
                disabled={update.isPending}
                defaultValue={String(base.album_id ?? 'none')}
                onChange={(e) => {
                  const v = e.target.value
                  update.mutate(
                    { key: base.key, body: { album_id: v === 'none' ? null : Number(v) } },
                    { onSuccess: () => { setMoving(false); pushToast(t('images.moved')) } },
                  )
                }}
              >
                <option value="none">{t('images.uncategorized')}</option>
                {albums.data?.items.map((a) => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
              </select>
            ) : (
              <button
                type="button"
                className="cursor-pointer rounded-sm border border-border bg-surface py-2 text-sm-plus font-semibold text-ink hover:bg-soft"
                onClick={() => setMoving(true)}
              >
                {t('images.moveToAlbum')}
              </button>
            )}
            <InlineConfirm
              label={t('images.addToTrash')}
              disabled={remove.isPending}
              onConfirm={() =>
                remove.mutate(base.key, {
                  onSuccess: () => {
                    pushToast(t('images.trashed'))
                    onClose()
                  },
                })
              }
            />
          </div>
        </div>
      </div>
    </div>,
    document.body,
  )
}
