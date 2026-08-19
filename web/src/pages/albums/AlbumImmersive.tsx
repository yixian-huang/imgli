import { useCallback, useEffect, useMemo, useRef, useState, type WheelEvent as ReactWheelEvent } from 'react'
import { Link } from 'react-router'
import type { PublicAlbumImg } from '../../api/types'
import { useT } from '../../i18n'
import { cn } from '../../lib/cn'
import { copyText } from '../../lib/copy'
import { RetryImg } from '../../ui/RetryImg'
import { sharePageURL } from './albumLinks'
import { filmstripWindow } from './filmstripWindow'
import {
  IcoClose,
  IcoExternal,
  IcoLink,
  IcoNext,
  IcoPrev,
  IcoShare,
  IcoZoomIn,
  IcoZoomOut,
  IconBtn,
} from './immersiveIcons'
import {
  extractImagePalette,
  paletteBackdropStyle,
  type ImagePalette,
} from './imagePalette'
import { useImmersiveFullscreen } from './useImmersiveFullscreen'

export type ImmersiveImg = Pick<
  PublicAlbumImg,
  'key' | 'name' | 'thumbnail_url' | 'url' | 'share_path' | 'width' | 'height' | 'size'
>

type Props = {
  items: ImmersiveImg[]
  index: number
  totalCount: number
  canPrev: boolean
  canNext: boolean
  onClose: () => void
  onPrev: () => void
  onNext: () => void
  onSelectIndex: (i: number) => void
}

const MIN_SCALE = 1
const MAX_SCALE = 4
const ZOOM_STEP = 0.35
const FILM_TILE = 56
const FILM_HALF = 24

/**
 * 沉浸层（Pixtale 取向）：全出血无边距、图标操作、主色 letterbox、胶片条。
 */
export function AlbumImmersive({
  items,
  index,
  totalCount,
  canPrev,
  canNext,
  onClose,
  onPrev,
  onNext,
  onSelectIndex,
}: Props) {
  const { t } = useT()
  const active = items[index]
  const filmRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<{ x: number; y: number; ox: number; oy: number } | null>(null)
  const [scale, setScale] = useState(1)
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const [chromeVisible, setChromeVisible] = useState(true)
  const [palette, setPalette] = useState<ImagePalette | null>(null)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)

  useImmersiveFullscreen(rootRef)

  useEffect(() => {
    setScale(1)
    setOffset({ x: 0, y: 0 })
  }, [index, active?.key])

  useEffect(() => {
    if (!active) {
      setPalette(null)
      return
    }
    let cancelled = false
    const src = active.thumbnail_url || active.url
    void extractImagePalette(src).then((p) => {
      if (!cancelled) setPalette(p)
    })
    return () => {
      cancelled = true
    }
  }, [active?.key, active?.thumbnail_url, active?.url])

  useEffect(() => {
    const root = filmRef.current
    if (!root) return
    const el = root.querySelector<HTMLElement>(`[data-film-i="${index}"]`)
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ behavior: 'smooth', inline: 'center', block: 'nearest' })
    }
  }, [index])

  useEffect(() => {
    for (let d = -2; d <= 2; d++) {
      if (d === 0) continue
      const img = items[index + d]
      if (!img) continue
      const full = new Image()
      full.src = img.url
    }
    for (let d = -4; d <= 4; d++) {
      if (d === 0) continue
      const img = items[index + d]
      if (!img) continue
      const th = new Image()
      th.src = img.thumbnail_url
    }
  }, [index, items])

  const filmWin = useMemo(
    () => filmstripWindow(items.length, index, FILM_HALF, FILM_TILE),
    [items.length, index],
  )

  const bumpChrome = useCallback(() => {
    setChromeVisible(true)
    if (hideTimer.current) clearTimeout(hideTimer.current)
    hideTimer.current = setTimeout(() => setChromeVisible(false), 1800)
  }, [])

  useEffect(() => {
    bumpChrome()
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current)
    }
  }, [index, bumpChrome])

  const clampScale = (s: number) => Math.min(MAX_SCALE, Math.max(MIN_SCALE, s))

  const zoomBy = (delta: number) => {
    setScale((s) => {
      const next = clampScale(s + delta)
      if (next <= MIN_SCALE) setOffset({ x: 0, y: 0 })
      return next
    })
    bumpChrome()
  }

  const onWheel = (e: ReactWheelEvent) => {
    e.preventDefault()
    zoomBy(e.deltaY > 0 ? -ZOOM_STEP : ZOOM_STEP)
  }

  const onDoubleClick = () => {
    setScale((s) => {
      if (s > 1.2) {
        setOffset({ x: 0, y: 0 })
        return 1
      }
      return 2.2
    })
    bumpChrome()
  }

  const swipeRef = useRef<{ x: number; y: number; t: number } | null>(null)
  const onPointerDown = (e: React.PointerEvent) => {
    e.currentTarget.setPointerCapture(e.pointerId)
    if (scale > 1) {
      dragRef.current = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y }
      swipeRef.current = null
    } else {
      swipeRef.current = { x: e.clientX, y: e.clientY, t: Date.now() }
      dragRef.current = null
    }
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const d = dragRef.current
    if (d) setOffset({ x: d.ox + (e.clientX - d.x), y: d.oy + (e.clientY - d.y) })
  }
  const onPointerUp = (e: React.PointerEvent) => {
    const sw = swipeRef.current
    if (sw && scale <= 1) {
      const dx = e.clientX - sw.x
      const dy = e.clientY - sw.y
      const dt = Date.now() - sw.t
      if (Math.abs(dx) > 56 && Math.abs(dx) > Math.abs(dy) * 1.2 && dt < 650) {
        if (dx < 0) onNext()
        else onPrev()
      }
    }
    dragRef.current = null
    swipeRef.current = null
    try {
      e.currentTarget.releasePointerCapture(e.pointerId)
    } catch {
      /* ignore */
    }
  }

  if (!active) return null

  const dim =
    active.width && active.height && active.width > 0 && active.height > 0
      ? `${active.width}×${active.height}`
      : ''

  const backdrop = palette ? paletteBackdropStyle(palette) : undefined

  return (
    <div
      ref={rootRef}
      className="fixed inset-0 z-[90] animate-[fadeIn_0.2s]"
      style={{
        backgroundColor: backdrop?.backgroundColor ?? '#0a0a0b',
        backgroundImage: backdrop?.backgroundImage,
      }}
      role="dialog"
      aria-modal="true"
      aria-label={active.name}
      data-testid="album-immersive"
      onClick={onClose}
      onMouseMove={bumpChrome}
    >
      <div className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden>
        <RetryImg
          key={active.key + '-bg'}
          src={active.thumbnail_url}
          alt=""
          className="size-full scale-110 object-cover opacity-[0.22] blur-3xl saturate-125"
        />
      </div>

      <div
        className="absolute inset-0 z-0 flex items-center justify-center overflow-hidden"
        onClick={(e) => e.stopPropagation()}
        onWheel={onWheel}
      >
        <div
          className={cn(
            'relative touch-none',
            scale > 1 ? 'cursor-grab active:cursor-grabbing' : 'cursor-grab',
          )}
          style={{
            transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})`,
            transition: dragRef.current ? undefined : 'transform 0.15s ease-out',
            maxWidth: '100vw',
            maxHeight: '100vh',
          }}
          onDoubleClick={onDoubleClick}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <RetryImg
            key={active.key}
            src={active.url}
            alt={active.name}
            className="max-h-screen max-w-screen object-contain select-none animate-[fadeIn_0.3s_ease-out]"
            draggable={false}
          />
        </div>
      </div>

      <div
        className={cn(
          'pointer-events-none absolute inset-y-0 left-0 z-[2] flex items-center pl-1 transition-opacity duration-300 sm:pl-2',
          chromeVisible ? 'opacity-100' : 'opacity-0',
        )}
      >
        <div className="pointer-events-auto">
          <IconBtn label={t('albums.immersivePrev')} disabled={!canPrev} onClick={onPrev}>
            <IcoPrev />
          </IconBtn>
        </div>
      </div>
      <div
        className={cn(
          'pointer-events-none absolute inset-y-0 right-0 z-[2] flex items-center pr-1 transition-opacity duration-300 sm:pr-2',
          chromeVisible ? 'opacity-100' : 'opacity-0',
        )}
      >
        <div className="pointer-events-auto">
          <IconBtn label={t('albums.immersiveNext')} disabled={!canNext} onClick={onNext}>
            <IcoNext />
          </IconBtn>
        </div>
      </div>

      <div
        className={cn(
          'absolute inset-x-0 top-0 z-[3] flex items-start justify-between px-2 pt-2 transition-opacity duration-300 sm:px-3 sm:pt-3',
          chromeVisible ? 'opacity-100' : 'opacity-0 pointer-events-none',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <IconBtn label={t('albums.closeLightbox')} onClick={onClose}>
          <IcoClose />
        </IconBtn>
        <div className="flex items-center gap-0.5">
          <IconBtn label={t('albums.zoomOut')} onClick={() => zoomBy(-ZOOM_STEP)}>
            <IcoZoomOut />
          </IconBtn>
          <IconBtn label={t('albums.zoomIn')} onClick={() => zoomBy(ZOOM_STEP)}>
            <IcoZoomIn />
          </IconBtn>
          <IconBtn
            label={t('albums.copyImageUrl')}
            onClick={() => void copyText(`${window.location.origin}${active.url}`, t('albums.copyImageUrl'))}
          >
            <IcoLink />
          </IconBtn>
          <IconBtn
            label={t('albums.copyImageShare')}
            onClick={() => void copyText(sharePageURL(active), t('albums.copyImageShare'))}
          >
            <IcoShare />
          </IconBtn>
          <Link
            to={active.share_path || `/s/${active.key}`}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={t('albums.openImageShare')}
            title={t('albums.openImageShare')}
            className="flex size-11 items-center justify-center text-white/90 no-underline transition-[color,transform] hover:scale-105 hover:text-white"
          >
            <IcoExternal />
          </Link>
        </div>
      </div>

      <div
        className={cn(
          'absolute inset-x-0 bottom-0 z-[3] transition-opacity duration-300',
          chromeVisible ? 'opacity-100' : 'opacity-0 pointer-events-none',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 pb-2 sm:px-5">
          <div
            className="max-w-[min(90vw,640px)] truncate text-[15px] font-semibold tracking-[-0.01em] text-white"
            style={{ textShadow: '0 1px 8px rgba(0,0,0,0.75), 0 0 2px rgba(0,0,0,0.5)' }}
          >
            {active.name}
          </div>
          <div
            className="mt-1 flex flex-wrap items-center gap-x-2 font-mono text-[11.5px] tracking-[0.04em] text-white/80"
            style={{ textShadow: '0 1px 6px rgba(0,0,0,0.7)' }}
          >
            <span>{t('albums.immersiveIndex', { current: index + 1, total: totalCount })}</span>
            {dim && (
              <>
                <span className="opacity-40">·</span>
                <span>{dim}</span>
              </>
            )}
          </div>
        </div>

        <div className="pb-[max(0.5rem,env(safe-area-inset-bottom))]" data-testid="album-filmstrip">
          <div
            ref={filmRef}
            className="flex gap-1.5 overflow-x-auto px-3 py-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
            data-testid="album-filmstrip-track"
          >
            {filmWin.padLeft > 0 && (
              <div className="shrink-0" style={{ width: filmWin.padLeft }} aria-hidden />
            )}
            {items.slice(filmWin.start, filmWin.end).map((img, off) => {
              const i = filmWin.start + off
              return (
                <button
                  key={img.key}
                  type="button"
                  data-film-i={i}
                  aria-label={img.name}
                  aria-current={i === index ? 'true' : undefined}
                  className={cn(
                    'relative h-12 w-12 shrink-0 cursor-pointer overflow-hidden rounded-[2px] border-0 bg-transparent p-0 opacity-55 transition-opacity',
                    i === index && 'opacity-100 ring-1 ring-white/90',
                  )}
                  onClick={() => onSelectIndex(i)}
                >
                  <RetryImg
                    src={img.thumbnail_url}
                    alt=""
                    className="size-full object-cover"
                    loading={Math.abs(i - index) <= 2 ? 'eager' : 'lazy'}
                  />
                </button>
              )
            })}
            {filmWin.padRight > 0 && (
              <div className="shrink-0" style={{ width: filmWin.padRight }} aria-hidden />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
