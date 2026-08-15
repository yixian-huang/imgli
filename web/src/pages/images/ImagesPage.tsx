import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { defaultFilter, useAlbums, useDeleteImage, useImages, useUpdateImage, type ImagesFilter } from '../../api/hooks'
import type { ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { albumForcesPrivate } from '../../lib/albumPrivacy'
import { useDebounced } from '../../lib/useDebounced'
import { useGlobal } from '../../store'
import { Button } from '../../ui/Button'
import { EmptyState } from '../../ui/EmptyState'
import { PageHeader } from '../../shell/PageHeader'
import { BatchBar } from './BatchBar'
import { DetailModal } from './DetailModal'
import { ImageGrid } from './ImageGrid'
import { Toolbar } from './Toolbar'

function isFiltered(f: ImagesFilter): boolean {
  return f.q !== '' || f.format !== 'ALL' || f.album !== 'all' || f.visibility !== 'all'
}

function filterFromParams(p: URLSearchParams): ImagesFilter {
  const albumRaw = p.get('album')
  let album: ImagesFilter['album'] = 'all'
  if (albumRaw === 'none') album = 'none'
  else if (albumRaw && /^\d+$/.test(albumRaw)) album = Number(albumRaw)
  const format = (p.get('format') || 'ALL').toUpperCase()
  const visibility = p.get('visibility') || 'all'
  const sort = p.get('sort') || 'date'
  return {
    q: p.get('q') ?? '',
    format: (['ALL', 'PNG', 'JPG', 'GIF', 'WEBP'].includes(format) ? format : 'ALL') as ImagesFilter['format'],
    album,
    visibility: (['all', 'public', 'private'].includes(visibility)
      ? visibility
      : 'all') as ImagesFilter['visibility'],
    sort: (['date', 'size', 'name'].includes(sort) ? sort : 'date') as ImagesFilter['sort'],
  }
}

function writeFilterParams(p: URLSearchParams, f: ImagesFilter): URLSearchParams {
  const n = new URLSearchParams(p)
  const setOrDel = (k: string, v: string, def: string) => {
    if (!v || v === def) n.delete(k)
    else n.set(k, v)
  }
  setOrDel('q', f.q, '')
  setOrDel('format', f.format, 'ALL')
  setOrDel('album', f.album === 'all' ? '' : String(f.album), '')
  setOrDel('visibility', f.visibility, 'all')
  setOrDel('sort', f.sort, 'date')
  return n
}

export function ImagesPage() {
  const { t } = useT()
  const [params, setParams] = useSearchParams()
  const [filter, setFilter] = useState(() => filterFromParams(params))
  // URL → state（后退/外链）
  useEffect(() => {
    setFilter(filterFromParams(params))
  }, [params])
  const debouncedQ = useDebounced(filter.q, 300)
  // q 防抖后写回 URL
  useEffect(() => {
    setParams((p) => {
      if ((p.get('q') ?? '') === debouncedQ) return p
      const n = new URLSearchParams(p)
      if (debouncedQ) n.set('q', debouncedQ)
      else n.delete('q')
      return n
    })
  }, [debouncedQ])
  const setFilterAndUrl = (next: ImagesFilter | ((f: ImagesFilter) => ImagesFilter)) => {
    setFilter((prev) => {
      const f = typeof next === 'function' ? next(prev) : next
      setParams((p) => writeFilterParams(p, { ...f, q: debouncedQ === prev.q ? debouncedQ : f.q }))
      return f
    })
  }
  const effective = useMemo(() => ({ ...filter, q: debouncedQ }), [filter, debouncedQ])
  const view = useGlobal((s) => s.view)
  const pushToast = useGlobal((s) => s.pushToast)
  const albums = useAlbums()
  const images = useImages(effective)
  const update = useUpdateImage()
  const remove = useDeleteImage()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)

  const items = useMemo(() => images.data?.pages.flatMap((p) => p.items) ?? [], [images.data])

  // 幽灵选中收敛：项因删除/筛选/变更离开列表时，从选择集剔除
  useEffect(() => {
    setSelected((s) => {
      const live = new Set(items.map((i) => i.key))
      const next = new Set([...s].filter((k) => live.has(k)))
      return next.size === s.size ? s : next
    })
  }, [items])

  // 无限滚动：sentinel 相交即翻页
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = images
  useEffect(() => {
    const el = sentinelRef.current
    if (!el) return
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && hasNextPage && !isFetchingNextPage) fetchNextPage()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, items.length])

  const toggleSelect = useCallback((key: string) => {
    setSelected((s) => {
      const n = new Set(s)
      if (n.has(key)) n.delete(key)
      else n.add(key)
      return n
    })
  }, [])
  const allSelected = items.length > 0 && selected.size === items.length
  const toggleAll = useCallback(
    () => setSelected(allSelected ? new Set() : new Set(items.map((i) => i.key))),
    [allSelected, items],
  )

  const quickVis = useCallback(
    (item: ImageItem) => {
      const next = item.visibility === 'public' ? 'private' : 'public'
      if (next === 'public' && albumForcesPrivate(albums.data?.items, item.album_id)) {
        pushToast(t('images.albumForcesPrivate'))
        return
      }
      update.mutate({ key: item.key, body: { visibility: next } })
    },
    [albums.data?.items, pushToast, t, update],
  )
  const quickDel = useCallback(
    (key: string) =>
      remove.mutate(key, {
        onSuccess: () =>
          setSelected((s) => {
            const n = new Set(s)
            n.delete(key)
            return n
          }),
      }),
    [remove],
  )
  const onOpen = useCallback((k: string) => setFocusKey(k), [])

  return (
    <div className="mx-auto max-w-[1120px] pt-11">
      <PageHeader
        kicker="LIBRARY"
        title={t('images.title')}
        extra={
          <div className="flex items-baseline gap-4">
            <span className="font-mono text-[11px] tracking-[0.05em] text-muted">
              {t('images.loaded', { count: items.length })}
            </span>
            <Link to="/trash" className="text-xs font-semibold text-muted underline hover:text-ink">
              {t('images.trash')}
            </Link>
          </div>
        }
      />
      <Toolbar
        filter={filter}
        onFilter={(f) => {
          setFilterAndUrl(f)
          setSelected(new Set())
        }}
        allSelected={allSelected}
        onToggleAll={toggleAll}
        selectable={items.length > 0}
      />

      {images.isLoading ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(170px,1fr))] gap-3.5">
          {Array.from({ length: 8 }, (_, i) => (
            <div key={i} className="overflow-hidden rounded-sm border border-border bg-surface">
              <div
                className="aspect-square animate-[pulse_1.4s_infinite] bg-soft"
                style={{ animationDelay: `${i * 90}ms` }}
              />
              <div
                className="mx-2.5 my-2.5 h-[9px] w-[70%] animate-[pulse_1.4s_infinite] rounded-[1px] bg-soft"
                style={{ animationDelay: `${i * 90}ms` }}
              />
            </div>
          ))}
        </div>
      ) : images.isError ? (
        <EmptyState badge="ERROR" title={t('images.loadFailed')} desc={t('images.loadFailedDesc')}>
          <Button variant="primary" onClick={() => images.refetch()}>
            {t('images.retry')}
          </Button>
        </EmptyState>
      ) : items.length === 0 && isFiltered(effective) ? (
        <div className="animate-[fadeIn_0.2s] py-20 text-center text-muted">
          <div className="mb-2.5 font-mono text-[11px] tracking-[0.14em]">NO RESULTS</div>
          <div className="mb-4 text-[13px]">{t('images.noResults')}</div>
          <Button onClick={() => setFilterAndUrl(defaultFilter)}>{t('images.clearFilters')}</Button>
        </div>
      ) : items.length === 0 ? (
        <EmptyState title={t('images.emptyTitle')} desc={t('images.emptyDesc')}>
          <Link to="/">
            <Button variant="primary">{t('images.goUpload')}</Button>
          </Link>
        </EmptyState>
      ) : (
        <ImageGrid
          items={items}
          view={view}
          selected={selected}
          sentinelRef={sentinelRef}
          loadingMore={isFetchingNextPage}
          onToggleSelect={toggleSelect}
          onOpen={onOpen}
          onQuickVis={quickVis}
          onQuickDel={quickDel}
        />
      )}
      <BatchBar selected={selected} items={items} onClear={() => setSelected(new Set())} />
      {focusKey && (
        <DetailModal items={items} focusKey={focusKey} onClose={() => setFocusKey(null)} onNavigate={setFocusKey} />
      )}
    </div>
  )
}
