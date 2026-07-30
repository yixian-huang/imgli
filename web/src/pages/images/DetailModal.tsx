import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { renderSVG } from 'uqr'
import { useAlbums, useDeleteImage, useImageDetail, useImageStats, useUpdateImage } from '../../api/hooks'
import type { ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { formatBytes, formatDate } from '../../lib/format'
import {
  EXPIRY_LABEL_KEY,
  EXPIRY_PRESETS,
  MAX_VIEWS_LABEL_KEY,
  MAX_VIEWS_PRESETS,
  type ExpiryKey,
  type MaxViewsKey,
} from '../../lib/imageAccessPresets'
import { generateAccessPassword } from '../../lib/password'
import { useGlobal } from '../../store'
import { InlineConfirm } from '../../ui/InlineConfirm'
import { Segmented } from '../../ui/Segmented'
import styles from './DetailModal.module.css'

interface Props {
  items: ImageItem[]
  focusKey: string
  onClose(): void
  onNavigate(key: string): void
}

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
        className={styles.helpTip}
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
            className={styles.helpTipBubble}
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
  const update = useUpdateImage()
  const remove = useDeleteImage()
  const pushToast = useGlobal((s) => s.pushToast)
  const [renaming, setRenaming] = useState(false)
  const [renameVal, setRenameVal] = useState('')
  const [moving, setMoving] = useState(false)
  const [expiryKey, setExpiryKey] = useState<ExpiryKey>('never')
  const [accessPw, setAccessPw] = useState('')
  const paneScrollRef = useRef<HTMLDivElement>(null)
  const accessSectionRef = useRef<HTMLElement>(null)
  const passwordInputRef = useRef<HTMLInputElement>(null)

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

  const setExpiry = (sec: number) => {
    update.mutate({ key: base.key, body: { expires_in: sec } })
  }

  const maxViews = d?.max_views ?? base.max_views ?? 0
  const viewsServed = d?.views_served ?? base.views_served ?? 0
  const maxViewsKey: MaxViewsKey =
    MAX_VIEWS_PRESETS.find((p) => p.n === maxViews)?.key ?? 'unlimited'
  const setMaxViews = (n: number) => {
    update.mutate({ key: base.key, body: { max_views: n } })
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

  return createPortal(
    <div className={styles.mask} onClick={onClose}>
      {prevKey && (
        <button type="button" title={t('images.prev')} className={`${styles.navBtn} ${styles.navPrev}`} onClick={(e) => { e.stopPropagation(); onNavigate(prevKey) }}>
          ‹
        </button>
      )}
      {nextKey && (
        <button type="button" title={t('images.next')} className={`${styles.navBtn} ${styles.navNext}`} onClick={(e) => { e.stopPropagation(); onNavigate(nextKey) }}>
          ›
        </button>
      )}
      <div role="dialog" aria-modal="true" className={styles.box} onClick={(e) => e.stopPropagation()}>
        <div className={styles.handle} />
        <div className={styles.preview}>
          <img className={styles.previewImg} src={base.links.url} alt={base.name} />
          <span className={styles.pos}>{idx + 1} / {items.length}</span>
        </div>
        <div className={styles.pane}>
          <div className={styles.headRow}>
            <div className={styles.headLeft}>
              <div className={styles.kicker}>DETAIL</div>
              {renaming ? (
                <div className={styles.renameRow}>
                  <input
                    className={styles.renameInput}
                    autoFocus
                    value={renameVal}
                    onChange={(e) => setRenameVal(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') commitRename()
                    }}
                  />
                  <button type="button" className={styles.renameSave} onClick={commitRename}>{t('images.save')}</button>
                </div>
              ) : (
                <div className={styles.nameRow}>
                  <span className={styles.name}>{base.name}</span>
                  <button
                    type="button"
                    className={styles.renameLink}
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
              <div className={styles.headChips} aria-label={t('images.accessControlHint')}>
                <button type="button" className={styles.accessChipBtn} onClick={jumpToAccess}>
                  <span className={styles.accessChip}>{expiryDisplay}</span>
                </button>
                <button type="button" className={styles.accessChipBtn} onClick={jumpToAccess}>
                  <span className={styles.accessChip}>{maxViewsChip}</span>
                </button>
                <button type="button" className={styles.accessChipBtn} onClick={jumpToAccess}>
                  <span className={hasAccessPassword ? `${styles.accessChip} ${styles.accessChipOn}` : styles.accessChip}>
                    {passwordChip}
                  </span>
                </button>
              </div>
            </div>
            <button type="button" className={styles.closeBtn} onClick={onClose}>×</button>
          </div>

          <div className={styles.paneScroll} ref={paneScrollRef}>
            {/* 上排：元信息 | 链接 */}
            <div className={styles.rowTop}>
              <section className={`${styles.section} ${styles.sectionCard}`} aria-label={t('images.tabInfo')}>
                <div className={styles.sectionHead}>
                  <span className={styles.sectionTitle}>{t('images.tabInfo')}</span>
                </div>
                <div className={styles.metaTable}>
                  <span className={styles.metaKey}>{t('images.dims')}</span>
                  <span className={styles.metaVal}>{base.width} × {base.height}</span>
                  <span className={styles.metaKey}>{t('images.size')}</span>
                  <span className={styles.metaVal}>{formatBytes(base.size)}</span>
                  <span className={styles.metaKey}>{t('images.mime')}</span>
                  <span className={styles.metaVal}>{d?.mime ?? '…'}</span>
                  <span className={styles.metaKey}>{t('images.uploadedAt')}</span>
                  <span className={styles.metaVal}>{formatDate(base.created_at)}</span>
                  <span className={styles.metaKey}>{t('images.album')}</span>
                  <span className={styles.metaVal}>{albumName}</span>
                  <span className={styles.metaKey}>{t('images.visibility')}</span>
                  <button
                    type="button"
                    className={styles.visToggle}
                    disabled={update.isPending}
                    onClick={() =>
                      update.mutate({
                        key: base.key,
                        body: { visibility: base.visibility === 'public' ? 'private' : 'public' },
                      })
                    }
                  >
                    {visLabel} — {t('images.clickToToggle')}
                  </button>
                  <span className={styles.metaKey}>{t('images.slug')}</span>
                  <span className={styles.metaVal}>
                    <input
                      className={styles.renameInput}
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
                  <span className={styles.metaKey}>{t('images.uploadIp')}</span>
                  <span className={`${styles.metaVal} ${styles.metaMuted}`}>
                    {d ? t('images.ipSelfOnly', { ip: d.upload_ip || '—' }) : '…'}
                  </span>
                </div>
              </section>

              <section className={`${styles.section} ${styles.sectionCard}`} aria-label={t('images.tabLinks')}>
                <div className={styles.sectionHead}>
                  <span className={styles.sectionTitle}>{t('images.tabLinks')}</span>
                </div>
                <div className={styles.copySection}>
                  <div className={styles.copyCol}>
                    {copyRows.map((r) => (
                      <div key={r.kind} className={styles.copyRow}>
                        <span className={styles.copyKind}>{r.kind}</span>
                        <span className={styles.copyText}>{r.text}</span>
                        <button
                          type="button"
                          className={styles.copyBtn}
                          onClick={() => copyText(r.text, t('images.linkLabel', { kind: r.kind }))}
                        >
                          {t('images.copy')}
                        </button>
                      </div>
                    ))}
                  </div>
                  <div className={styles.qrCol}>
                    <div className={styles.kicker}>QR</div>
                    <div className={styles.qrBox} dangerouslySetInnerHTML={{ __html: qrSVG }} />
                  </div>
                </div>
              </section>
            </div>

            {/* 下排：访问 | 统计（无统计时访问占满） */}
            <div className={statsAvailable ? styles.rowBottom : `${styles.rowBottom} ${styles.rowBottomSolo}`}>
              <section
                ref={accessSectionRef}
                className={`${styles.section} ${styles.sectionCard} ${styles.accessCell}`}
                aria-label={t('images.accessControl')}
              >
                <div className={styles.sectionHead}>
                  <span className={styles.sectionTitle}>{t('images.accessControl')}</span>
                  <span className={styles.sectionSub}>{t('images.accessControlHint')}</span>
                </div>

                <div className={styles.accessGrid}>
                  <div className={styles.accessField}>
                    <div className={styles.accessFieldHead}>
                      <span className={styles.accessFieldLabel}>
                        {t('images.accessPassword')}
                        <HelpTip text={t('images.accessPasswordHint')} />
                      </span>
                      <span className={styles.accessFieldValue}>
                        {hasAccessPassword
                          ? t('images.accessPasswordSet')
                          : t('images.accessPasswordNone')}
                      </span>
                    </div>
                    <input
                      ref={passwordInputRef}
                      className={styles.renameInput}
                      type="text"
                      value={accessPw}
                      placeholder={t('images.accessPasswordPlaceholder')}
                      onChange={(e) => setAccessPw(e.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                    />
                    <div className={styles.accessPwActions}>
                      <button
                        type="button"
                        className={styles.removeExpiry}
                        disabled={update.isPending}
                        onClick={fillRandomPassword}
                      >
                        {t('images.accessPasswordGenerate')}
                      </button>
                      {accessPw.trim() ? (
                        <button
                          type="button"
                          className={styles.removeExpiry}
                          onClick={() => copyText(accessPw.trim(), t('images.accessPassword'))}
                        >
                          {t('images.accessPasswordCopy')}
                        </button>
                      ) : null}
                      <button
                        type="button"
                        className={styles.renameSave}
                        disabled={!accessPw.trim() || update.isPending}
                        onClick={() => setAccessPassword(accessPw.trim())}
                      >
                        {t('images.accessPasswordSave')}
                      </button>
                      {hasAccessPassword && (
                        <button
                          type="button"
                          className={styles.removeExpiry}
                          disabled={update.isPending}
                          onClick={() => setAccessPassword('')}
                        >
                          {t('images.accessPasswordClear')}
                        </button>
                      )}
                    </div>
                  </div>

                  <div className={styles.accessField}>
                    <div className={styles.accessFieldHead}>
                      <span className={styles.accessFieldLabel}>
                        {t('images.expiry')}
                        <HelpTip text={t('images.expiryWarn')} />
                      </span>
                      <span className={styles.accessFieldValue}>{expiryDisplay}</span>
                    </div>
                    <Segmented<ExpiryKey>
                      mono
                      compact
                      options={EXPIRY_PRESETS.map((p) => ({
                        value: p.key,
                        label: t(EXPIRY_LABEL_KEY[p.key]),
                      }))}
                      value={expiryKey}
                      onChange={(k) => {
                        setExpiryKey(k)
                        const p = EXPIRY_PRESETS.find((x) => x.key === k)
                        setExpiry(p?.sec ?? 0)
                      }}
                    />
                    {expiresAt && (
                      <button
                        type="button"
                        className={styles.removeExpiry}
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

                  <div className={styles.accessField}>
                    <div className={styles.accessFieldHead}>
                      <span className={styles.accessFieldLabel}>
                        {t('images.maxViews')}
                        <HelpTip text={t('images.maxViewsHint')} />
                      </span>
                      <span className={styles.accessFieldValue}>
                        {maxViews > 0
                          ? t('images.maxViewsUsed', { used: viewsServed, max: maxViews })
                          : t('upload.maxViewsUnlimited')}
                      </span>
                    </div>
                    <Segmented<MaxViewsKey>
                      mono
                      compact
                      options={MAX_VIEWS_PRESETS.map((p) => ({
                        value: p.key,
                        label: t(MAX_VIEWS_LABEL_KEY[p.key]),
                      }))}
                      value={maxViewsKey}
                      onChange={(k) => {
                        const p = MAX_VIEWS_PRESETS.find((x) => x.key === k)
                        setMaxViews(p?.n ?? 0)
                      }}
                    />
                  </div>

                  {/* 长说明默认收起：桌面悬停 ?，移动端点开「说明」 */}
                  <details className={styles.accessNotes}>
                    <summary>{t('images.accessNotes')}</summary>
                    <ul className={styles.accessNotesList}>
                      <li>
                        <strong>{t('images.accessPassword')}</strong>
                        {' — '}
                        {t('images.accessPasswordHint')}
                      </li>
                      <li>
                        <strong>{t('images.expiry')}</strong>
                        {' — '}
                        {t('images.expiryWarn')}
                      </li>
                      <li>
                        <strong>{t('images.maxViews')}</strong>
                        {' — '}
                        {t('images.maxViewsHint')}
                      </li>
                    </ul>
                  </details>
                </div>
              </section>

              {statsAvailable && (
                <section
                  className={`${styles.section} ${styles.sectionCard} ${styles.statsCell}`}
                  aria-label={t('images.accessStats')}
                >
                  <div className={styles.sectionHead}>
                    <span className={styles.sectionTitle}>{t('images.accessStats')}</span>
                    <span className={styles.sectionSub}>
                      {t('images.totalViews', { count: stats.data!.total })}
                    </span>
                  </div>
                  <div className={styles.accessSection}>
                    <div className={styles.kicker}>ACCESS — {t('images.accessLabel')}</div>
                    {(() => {
                      const daily = stats.data!.daily
                      const max = daily.reduce((m, day) => (day.views > m ? day.views : m), 0)
                      if (max === 0) {
                        return <div className={styles.accessEmpty}>{t('images.noAccess')}</div>
                      }
                      return (
                        <div className={styles.accessBars} aria-label={t('images.last30Days')}>
                          {daily.map((day) => (
                            <div
                              key={day.date}
                              className={styles.accessBar}
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

          <div className={styles.footRow}>
            {moving ? (
              <select
                className={styles.moveSelect}
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
              <button type="button" className={styles.footBtn} onClick={() => setMoving(true)}>{t('images.moveToAlbum')}</button>
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
