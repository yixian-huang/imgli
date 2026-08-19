import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useAlbums, useCreateAlbum, useDeleteAlbum } from '../../api/hooks'
import type { Album } from '../../api/types'
import { useT } from '../../i18n'
import { cn } from '../../lib/cn'
import { formatDate } from '../../lib/format'
import { useGlobal } from '../../store'
import { Button } from '../../ui/Button'
import { EmptyState } from '../../ui/EmptyState'
import { RetryImg } from '../../ui/RetryImg'
import { Modal } from '../../ui/Modal'
import { PageHeader } from '../../shell/PageHeader'

export function AlbumsPage() {
  const { t } = useT()
  const albums = useAlbums()
  const create = useCreateAlbum()
  const remove = useDeleteAlbum()
  const pushToast = useGlobal((s) => s.pushToast)
  const navigate = useNavigate()
  const [showNew, setShowNew] = useState(false)
  const [newName, setNewName] = useState('')
  const [newVis, setNewVis] = useState<'public' | 'private'>('public')
  const [delTarget, setDelTarget] = useState<Album | null>(null)

  const items = albums.data?.items ?? []

  function createAlbum() {
    const name = newName.trim()
    if (!name) return pushToast(t('albums.nameRequired'))
    create.mutate(
      { name, visibility: newVis },
      {
        onSuccess: () => {
          setShowNew(false)
          setNewName('')
          pushToast(t('albums.created'))
        },
      },
    )
  }

  function doDelete(withImages: boolean) {
    if (!delTarget) return
    remove.mutate(
      { id: delTarget.id, withImages },
      {
        onSuccess: () => {
          pushToast(withImages ? t('albums.deletedWithImages') : t('albums.deletedKeepImages'))
          setDelTarget(null)
        },
      },
    )
  }

  return (
    <div className="mx-auto max-w-[1120px] pt-11">
      <PageHeader
        kicker="ALBUMS"
        title={t('albums.title')}
        extra={
          <Button variant="primary" onClick={() => setShowNew(true)}>
            {t('albums.newAlbum')}
          </Button>
        }
      />

      {albums.isLoading ? null : items.length === 0 ? (
        <EmptyState title={t('albums.emptyTitle')} desc={t('albums.emptyDesc')}>
          <Button variant="primary" onClick={() => setShowNew(true)}>
            {t('albums.newFirstAlbum')}
          </Button>
        </EmptyState>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
          {items.map((a) => (
            <div
              key={a.id}
              className="relative animate-[rise_0.28s_both] cursor-pointer overflow-hidden rounded-sm border border-border bg-surface hover:border-muted"
              onClick={() => navigate(`/albums/${a.id}`)}
            >
              <div className="grid aspect-video grid-cols-[2fr_1fr] grid-rows-2 gap-px bg-border">
                <div className="row-span-2 flex items-center justify-center overflow-hidden bg-soft">
                  {a.cover_key ? (
                    <RetryImg className="block size-full object-cover" src={`/t/${a.cover_key}.jpg`} alt="" loading="lazy" />
                  ) : (
                    <span className="font-mono text-2xs tracking-[0.08em] text-muted">COVER</span>
                  )}
                </div>
                <div className="stripe flex items-center justify-center bg-surface" />
                <div className="stripe flex items-center justify-center bg-surface">
                  {a.image_count > 1 && (
                    <span className="font-mono text-2xs text-muted">+{a.image_count - 1}</span>
                  )}
                </div>
              </div>
              <span
                className={cn(
                  'absolute top-2.5 right-2.5 rounded-[2px] border border-border bg-surface px-[7px] py-0.5 font-mono text-[9.5px] tracking-[0.1em] text-muted',
                  a.visibility === 'public' && 'border-btn bg-btn text-btn-text',
                )}
              >
                {a.visibility === 'public' ? 'PUBLIC' : 'PRIVATE'}
              </span>
              <div className="flex items-center justify-between gap-2.5 border-t border-border px-[13px] py-[11px]">
                <div className="min-w-0">
                  <div className="truncate text-[13.5px] font-bold">{a.name}</div>
                  <div className="mt-[3px] font-mono text-2xs tracking-[0.05em] text-muted">
                    {t('albums.meta', { count: a.image_count, date: formatDate(a.created_at) })}
                  </div>
                </div>
                <button
                  type="button"
                  title={t('albums.deleteAlbumTitle')}
                  className="shrink-0 cursor-pointer border-0 bg-transparent px-1.5 py-1 text-[15px] leading-none text-muted hover:text-err"
                  onClick={(e) => {
                    e.stopPropagation()
                    setDelTarget(a)
                  }}
                >
                  ×
                </button>
              </div>
            </div>
          ))}
          <div
            className="flex min-h-[220px] cursor-pointer flex-col items-center justify-center gap-2.5 rounded-sm border border-dashed border-muted text-muted transition-[border-color,color] duration-150 hover:border-ink hover:text-ink"
            onClick={() => setShowNew(true)}
          >
            <div className="flex size-[34px] items-center justify-center rounded-sm border border-border bg-soft text-base">
              ＋
            </div>
            <span className="text-sm-plus font-semibold">{t('albums.newAlbumShort')}</span>
          </div>
        </div>
      )}

      <Modal open={showNew} onClose={() => setShowNew(false)} width={400}>
        <div className="mb-1.5 font-mono text-2xs tracking-[0.14em] text-muted">NEW ALBUM</div>
        <div className="mb-3.5 text-base font-bold">{t('albums.newAlbumTitle')}</div>
        <div className="mb-4 flex flex-col gap-2">
          <label className="text-xs font-semibold text-muted" htmlFor="album-name">
            {t('albums.nameLabel')}
          </label>
          <input
            id="album-name"
            className="rounded-sm border border-border bg-bg px-3 py-[9px] font-inherit text-[13.5px] text-ink outline-none focus:border-muted"
            placeholder={t('albums.namePlaceholder')}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
        </div>
        <div className="mb-4 flex flex-col gap-2">
          <span className="text-xs font-semibold text-muted">{t('albums.visibility')}</span>
          <div className="flex overflow-hidden rounded-sm border border-border">
            <button
              type="button"
              className={cn(
                'flex-1 cursor-pointer border-0 bg-surface py-[9px] text-xs font-semibold text-muted transition-[background,color] duration-150',
                newVis === 'public' && 'bg-btn text-btn-text',
              )}
              onClick={() => setNewVis('public')}
            >
              {t('albums.visPublic')}
            </button>
            <button
              type="button"
              className={cn(
                'flex-1 cursor-pointer border-0 border-l border-border bg-surface py-[9px] text-xs font-semibold text-muted transition-[background,color] duration-150',
                newVis === 'private' && 'bg-btn text-btn-text',
              )}
              onClick={() => setNewVis('private')}
            >
              {t('albums.visPrivate')}
            </button>
          </div>
        </div>
        <Button variant="primary" className="w-full py-[11px] text-[13px]" onClick={createAlbum} disabled={create.isPending}>
          {t('albums.create')}
        </Button>
      </Modal>

      <Modal open={!!delTarget} onClose={() => setDelTarget(null)}>
        <div className="mb-1.5 font-mono text-2xs tracking-[0.14em] text-err">DELETE ALBUM</div>
        <div className="mb-3.5 text-base font-bold">{t('albums.deleteConfirm', { name: delTarget?.name ?? '' })}</div>
        <p className="-mt-1.5 mb-3.5 text-sm-plus leading-relaxed text-muted">
          {t('albums.deleteDesc', { count: delTarget?.image_count ?? 0 })}
        </p>
        <div className="flex flex-col gap-2">
          <button
            type="button"
            className="cursor-pointer rounded-sm border border-border bg-surface px-3.5 py-[11px] text-left text-sm-plus font-semibold text-ink hover:bg-soft disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => doDelete(false)}
            disabled={remove.isPending}
          >
            {t('albums.deleteAlbumOnly')}
            <span className="mt-0.5 block text-[11px] font-normal text-muted">{t('albums.deleteAlbumOnlySub')}</span>
          </button>
          <button
            type="button"
            className="cursor-pointer rounded-sm border border-err bg-surface px-3.5 py-[11px] text-left text-sm-plus font-semibold text-err hover:bg-soft disabled:cursor-not-allowed disabled:opacity-50 [&_span]:text-inherit [&_span]:opacity-75"
            onClick={() => doDelete(true)}
            disabled={remove.isPending}
          >
            {t('albums.deleteWithImages')}
            <span className="mt-0.5 block text-[11px] font-normal">
              {t('albums.deleteWithImagesSub', { count: delTarget?.image_count ?? 0 })}
            </span>
          </button>
          <button
            type="button"
            className="cursor-pointer border-0 bg-transparent p-2 text-sm-plus font-semibold text-muted hover:text-ink"
            onClick={() => setDelTarget(null)}
          >
            {t('albums.cancel')}
          </button>
        </div>
      </Modal>
    </div>
  )
}
