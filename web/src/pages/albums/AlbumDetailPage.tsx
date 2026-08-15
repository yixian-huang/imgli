import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import {
  defaultFilter,
  useAlbumReorder,
  useAlbumSetImagesVisibility,
  useAlbumStats,
  useAlbums,
  useDeleteImage,
  useImages,
  useUpdateAlbum,
  useUpdateImage,
} from '../../api/hooks'
import type { ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { albumForcesPrivate } from '../../lib/albumPrivacy'
import { copyText } from '../../lib/copy'
import { formatDate } from '../../lib/format'
import { cn } from '../../lib/cn'
import { useGlobal } from '../../store'
import { Button } from '../../ui/Button'
import { EmptyState } from '../../ui/EmptyState'
import { BatchBar } from '../images/BatchBar'
import { DetailModal } from '../images/DetailModal'
import { ImageGrid } from '../images/ImageGrid'
import { AlbumSettingsModal, type AlbumSettingsTab } from './AlbumSettingsModal'

const chipCls =
  'inline-flex items-center rounded-[2px] border border-border px-[7px] py-0.5 font-mono text-[9.5px] tracking-[0.1em] text-muted'

export function AlbumDetailPage() {
  const { t } = useT()
  const { id: idParam } = useParams()
  const id = Number(idParam)
  const albums = useAlbums()
  const album = albums.data?.items.find((a) => a.id === id)
  const images = useImages(useMemo(() => ({ ...defaultFilter, album: id, sort: 'position' }), [id]))
  const update = useUpdateImage()
  const removeImg = useDeleteImage()
  const updateAlbum = useUpdateAlbum()
  const setVis = useAlbumSetImagesVisibility()
  const reorder = useAlbumReorder()
  const stats = useAlbumStats(Number.isFinite(id) ? id : undefined)
  const pushToast = useGlobal((s) => s.pushToast)
  const navigate = useNavigate()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const [renaming, setRenaming] = useState(false)
  const [renameVal, setRenameVal] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsTab, setSettingsTab] = useState<AlbumSettingsTab>('share')
  const sentinelRef = useRef<HTMLDivElement>(null)

  const items = useMemo(() => images.data?.pages.flatMap((p) => p.items) ?? [], [images.data])

  useEffect(() => {
    setSelected((s) => {
      const live = new Set(items.map((i) => i.key))
      const next = new Set([...s].filter((k) => live.has(k)))
      return next.size === s.size ? s : next
    })
  }, [items])

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

  if (!albums.isLoading && !album) {
    return (
      <div className="mx-auto max-w-[1120px] pt-11">
        <EmptyState badge="404" title={t('albums.notFound')} desc={t('albums.notFoundDesc')}>
          <Button variant="primary" onClick={() => navigate('/albums')}>
            {t('albums.backToAlbums')}
          </Button>
        </EmptyState>
      </div>
    )
  }

  const commitRename = () => {
    const name = renameVal.trim()
    if (!name || !album || name === album.name) return setRenaming(false)
    updateAlbum.mutate(
      { id, body: { name } },
      {
        onSuccess: () => {
          setRenaming(false)
          pushToast(t('albums.renamed'))
        },
      },
    )
  }

  const quickVis = (item: ImageItem) => {
    const next = item.visibility === 'public' ? 'private' : 'public'
    if (next === 'public' && albumForcesPrivate(albums.data?.items, item.album_id)) {
      pushToast(t('images.albumForcesPrivate'))
      return
    }
    update.mutate({ key: item.key, body: { visibility: next } })
  }

  const allSelected = items.length > 0 && selected.size === items.length
  const toggleAll = () => setSelected(allSelected ? new Set() : new Set(items.map((i) => i.key)))

  const openSettings = (tab: AlbumSettingsTab = 'share') => {
    setSettingsTab(tab)
    setSettingsOpen(true)
  }

  const listInPlaza = album?.list_in_plaza !== false
  const selectedCover = selected.size === 1 ? items.find((i) => selected.has(i.key)) : undefined
  const coverBlocked =
    album?.visibility === 'public' &&
    !!selectedCover &&
    (selectedCover.visibility === 'private' || !!selectedCover.has_access_password)

  return (
    <div className="mx-auto max-w-[1120px] pt-11">
      {/* L0 顶栏：身份 + 主操作 */}
      <div className="mb-4 flex animate-[fadeIn_0.2s] flex-wrap items-end justify-between gap-4 border-b border-border pb-[18px]">
        <div className="min-w-0">
          <Link
            to="/albums"
            className="mb-2 inline-block font-mono text-[11px] tracking-[0.14em] text-muted hover:text-ink"
          >
            ← ALBUMS
          </Link>
          {renaming ? (
            <div className="flex gap-1.5">
              <input
                className="w-[220px] rounded-sm border border-muted bg-bg px-2.5 py-1 font-inherit text-xl font-bold text-ink outline-none"
                autoFocus
                value={renameVal}
                onChange={(e) => setRenameVal(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') commitRename()
                }}
              />
              <button
                type="button"
                className="cursor-pointer rounded-sm border-0 bg-btn px-3.5 py-[7px] text-xs font-bold text-btn-text"
                onClick={commitRename}
              >
                {t('albums.save')}
              </button>
            </div>
          ) : (
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <h1 className="m-0 text-[26px] font-bold tracking-[-0.015em]">{album?.name ?? '…'}</h1>
              <button
                type="button"
                className="cursor-pointer border-0 bg-transparent p-0 text-[11.5px] text-muted underline hover:text-ink"
                onClick={() => {
                  setRenameVal(album?.name ?? '')
                  setRenaming(true)
                }}
              >
                {t('albums.rename')}
              </button>
              {album && (
                <span className={chipCls}>{album.visibility === 'public' ? 'PUBLIC' : 'PRIVATE'}</span>
              )}
              {album?.has_access_password && <span className={chipCls}>LOCKED</span>}
              {album && !listInPlaza && <span className={chipCls}>NO PLAZA</span>}
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          {album?.visibility === 'public' && (
            <>
              <Button
                onClick={() => {
                  void copyText(`${window.location.origin}/a/${album.id}`, t('albums.publicLink'))
                }}
              >
                {t('albums.copyPublicLink')}
              </Button>
              <Button
                onClick={() => {
                  window.open(`/a/${album.id}`, '_blank', 'noopener,noreferrer')
                }}
              >
                {t('albums.openPublicLink')}
              </Button>
            </>
          )}
          <Button onClick={() => openSettings('share')} data-testid="album-settings-btn">
            {t('albums.settings')}
          </Button>
          <Link to={album ? `/?album=${album.id}` : '/'}>
            <Button variant="primary">{t('albums.uploadToAlbum')}</Button>
          </Link>
        </div>
      </div>

      {/* 摘要行：可点访问量进数据 Tab */}
      <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs-plus tracking-[0.06em] text-muted">
        <span>
          {album ? t('albums.detailMeta', { count: album.image_count, date: formatDate(album.created_at) }) : ''}
        </span>
        {stats.data && (
          <button
            type="button"
            className={cn(
              'cursor-pointer border-0 bg-transparent p-0 font-mono text-xs-plus tracking-[0.06em] text-muted underline-offset-2 hover:text-ink hover:underline',
            )}
            data-testid="album-stats-total"
            onClick={() => openSettings('stats')}
          >
            {t('albums.viewsTotal', { count: stats.data.total })}
          </button>
        )}
      </div>

      {/* L1 网格操作条 */}
      {items.length > 0 && (
        <div className="mb-3.5 flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={toggleAll}>{allSelected ? t('albums.deselectAll') : t('albums.selectAll')}</Button>
            <Button
              disabled={reorder.isPending || items.length === 0}
              onClick={() =>
                reorder.mutate(
                  { id, keys: items.map((i) => i.key) },
                  { onSuccess: () => pushToast(t('albums.orderSaved')) },
                )
              }
            >
              {t('albums.saveOrder')}
            </Button>
            {selected.size === 1 && (
              <Button
                disabled={updateAlbum.isPending || coverBlocked}
                title={coverBlocked ? t('albums.coverMustBePublic') : undefined}
                onClick={() => {
                  const key = [...selected][0]
                  if (!key) return
                  if (coverBlocked) {
                    pushToast(t('albums.coverMustBePublic'))
                    return
                  }
                  updateAlbum.mutate(
                    { id, body: { cover_key: key } },
                    { onSuccess: () => pushToast(t('albums.coverSaved')) },
                  )
                }}
              >
                {t('albums.setCover')}
              </Button>
            )}
          </div>
          <div className="text-[11.5px] text-muted">{t('albums.orderHint')}</div>
        </div>
      )}

      {items.length === 0 && !images.isLoading ? (
        <EmptyState title={t('albums.detailEmptyTitle')} desc={t('albums.detailEmptyDesc')}>
          <Link to={album ? `/?album=${album.id}` : '/'}>
            <Button variant="primary">{t('albums.goUpload')}</Button>
          </Link>
        </EmptyState>
      ) : (
        <ImageGrid
          items={items}
          view="grid"
          selected={selected}
          sentinelRef={sentinelRef}
          loadingMore={isFetchingNextPage}
          onToggleSelect={(k) =>
            setSelected((s) => {
              const n = new Set(s)
              if (n.has(k)) n.delete(k)
              else n.add(k)
              return n
            })
          }
          onOpen={setFocusKey}
          onQuickVis={quickVis}
          onQuickDel={(k) => removeImg.mutate(k)}
        />
      )}

      <BatchBar selected={selected} items={items} albumId={id} onClear={() => setSelected(new Set())} />
      {focusKey && (
        <DetailModal
          items={items}
          focusKey={focusKey}
          onClose={() => setFocusKey(null)}
          onNavigate={setFocusKey}
        />
      )}

      {album && (
        <AlbumSettingsModal
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          tab={settingsTab}
          onTabChange={setSettingsTab}
          album={album}
          albumId={id}
          hasImages={items.length > 0 || (album.image_count ?? 0) > 0}
          pending={updateAlbum.isPending}
          setVisPending={setVis.isPending}
          stats={
            stats.data && Array.isArray(stats.data.daily)
              ? { total: stats.data.total, daily: stats.data.daily }
              : undefined
          }
          onUpdate={(body) => updateAlbum.mutate({ id, body })}
          onBulkVis={(visibility) =>
            setVis.mutate(
              { id, visibility },
              {
                onSuccess: (d) => pushToast(t('albums.bulkVisDone', { count: d.updated })),
              },
            )
          }
          onSaved={(msg) => pushToast(msg)}
        />
      )}
    </div>
  )
}
