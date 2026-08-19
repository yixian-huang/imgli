import { useEffect, useRef } from 'react'
import type { PublicAlbumImg } from '../../api/types'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { Button } from '../../ui/Button'
import { RetryImg } from '../../ui/RetryImg'
import { EmptyState } from '../../ui/EmptyState'
import { aspectStyle, sharePageURL } from './albumLinks'

type Props = {
  rows: PublicAlbumImg[]
  loading: boolean
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onFetchMore: () => void
  onOpenImmersive: (index0: number) => void
  /** 点击缩略图是否进入沉浸；false 时改为打开图片分享页。缺省 true。 */
  clickToImmersive?: boolean
}

/** 瀑布流网格 + 触底无限滚动哨兵。 */
export function PublicAlbumMasonry({
  rows,
  loading,
  hasNextPage,
  isFetchingNextPage,
  onFetchMore,
  onOpenImmersive,
  clickToImmersive = true,
}: Props) {
  const { t } = useT()
  const sentinelRef = useRef<HTMLDivElement>(null)
  const hasNextRef = useRef(hasNextPage)
  const fetchingRef = useRef(isFetchingNextPage)
  const fetchRef = useRef(onFetchMore)
  hasNextRef.current = hasNextPage
  fetchingRef.current = isFetchingNextPage
  fetchRef.current = onFetchMore

  useEffect(() => {
    const el = sentinelRef.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && hasNextRef.current && !fetchingRef.current) {
          fetchRef.current()
        }
      },
      { root: null, rootMargin: '480px 0px', threshold: 0 },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [rows.length, hasNextPage])

  if (loading) {
    return <div className="px-4 py-12 text-center text-muted">{t('discover.loading')}</div>
  }
  if (rows.length === 0) {
    return <EmptyState title={t('albums.publicEmpty')} desc={t('albums.publicEmptyDesc')} />
  }

  return (
    <>
      <div
        className="columns-2 gap-3.5 sm:columns-3 lg:columns-4 [column-gap:0.9rem]"
        data-testid="album-masonry"
      >
        {rows.map((r, i) => {
          const media = (
            <>
              <RetryImg
                src={r.thumbnail_url}
                alt=""
                loading="lazy"
                decoding="async"
                className="absolute inset-0 block size-full object-cover transition-[transform,filter] duration-500 ease-out group-hover:scale-[1.04] group-hover:brightness-105"
              />
              <span className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/35 via-transparent to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100" />
              <span className="pointer-events-none absolute right-2 bottom-2 left-2 truncate text-[11px] font-medium text-white opacity-0 drop-shadow transition-opacity duration-300 group-hover:opacity-100">
                {r.name}
              </span>
            </>
          )
          const shellCls =
            'relative block w-full cursor-pointer border-0 bg-transparent p-0 text-left no-underline'
          return (
            <div
              key={r.key}
              className="group relative mb-3.5 break-inside-avoid overflow-hidden rounded-[3px] bg-soft shadow-[0_1px_0_rgba(0,0,0,0.04)] [content-visibility:auto] [contain-intrinsic-size:220px] animate-[rise_0.4s_both]"
              style={{ animationDelay: `${Math.min(i, 12) * 28}ms` }}
            >
              {clickToImmersive ? (
                <button
                  type="button"
                  className={shellCls}
                  style={aspectStyle(r)}
                  onClick={() => onOpenImmersive(i)}
                  aria-label={r.name}
                >
                  {media}
                </button>
              ) : (
                <a
                  href={r.share_path || `/s/${r.key}`}
                  className={shellCls}
                  style={aspectStyle(r)}
                  aria-label={r.name}
                >
                  {media}
                </a>
              )}
              <button
                type="button"
                className="absolute top-2 right-2 z-[1] hidden cursor-pointer rounded-full border border-white/30 bg-black/45 px-2.5 py-1 text-[10.5px] font-semibold text-white backdrop-blur-sm group-hover:inline-flex group-focus-within:inline-flex"
                onClick={(e) => {
                  e.stopPropagation()
                  void copyText(sharePageURL(r), t('albums.copyImageShare'))
                }}
              >
                {t('albums.copyImageShare')}
              </button>
            </div>
          )
        })}
      </div>
      <div
        ref={sentinelRef}
        className="h-8 w-full"
        data-testid="album-scroll-sentinel"
        aria-hidden
      />
      {hasNextPage && (
        <div className="mt-2 flex justify-center">
          <Button variant="secondary" disabled={isFetchingNextPage} onClick={onFetchMore}>
            {isFetchingNextPage ? t('discover.loading') : t('discover.loadMore')}
          </Button>
        </div>
      )}
    </>
  )
}
