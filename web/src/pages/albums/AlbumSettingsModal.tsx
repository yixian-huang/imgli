import { useEffect, useState } from 'react'
import type { Album } from '../../api/types'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { Button } from '../../ui/Button'
import { Modal } from '../../ui/Modal'
import { Segmented } from '../../ui/Segmented'
import type { AlbumPublicMode } from './albumPublicView'

export type AlbumSettingsTab = 'share' | 'content' | 'stats'

type StatsData = {
  total: number
  daily: { date: string; views: number }[]
}

type Props = {
  open: boolean
  onClose: () => void
  tab: AlbumSettingsTab
  onTabChange: (t: AlbumSettingsTab) => void
  album: Album
  albumId: number
  hasImages: boolean
  pending: boolean
  setVisPending: boolean
  stats?: StatsData
  onUpdate: (body: {
    visibility?: string
    default_view?: 'gallery' | 'immersive'
    click_to_immersive?: boolean
    description?: string
    access_password?: string
    list_in_plaza?: boolean
  }) => void
  onBulkVis: (visibility: 'public' | 'private') => void
  onSaved: (msg: string) => void
}

/**
 * 相册低频配置：分享与访问 / 内容与广场 / 数据。
 * 主页面只保留图库操作，设置进弹窗。
 */
export function AlbumSettingsModal({
  open,
  onClose,
  tab,
  onTabChange,
  album,
  albumId,
  hasImages,
  pending,
  setVisPending,
  stats,
  onUpdate,
  onBulkVis,
  onSaved,
}: Props) {
  const { t } = useT()
  const [desc, setDesc] = useState(album.description ?? '')
  const [pw, setPw] = useState('')

  useEffect(() => {
    if (open) {
      setDesc(album.description ?? '')
      setPw('')
    }
  }, [open, album.id, album.description])

  const defaultView: AlbumPublicMode = album.default_view === 'immersive' ? 'immersive' : 'gallery'
  /** 缺省 true：与后端默认一致 */
  const clickToImmersive = album.click_to_immersive !== false
  const listInPlaza = album.list_in_plaza !== false

  const tabs: { value: AlbumSettingsTab; label: string }[] = [
    { value: 'share', label: t('albums.settingsTabShare') },
    { value: 'content', label: t('albums.settingsTabContent') },
    { value: 'stats', label: t('albums.settingsTabStats') },
  ]

  return (
    <Modal open={open} onClose={onClose} width={520} className="max-h-[min(90vh,720px)] overflow-y-auto">
      <div className="mb-1 font-mono text-2xs tracking-[0.14em] text-muted">ALBUM SETTINGS</div>
      <div className="mb-3 flex items-start justify-between gap-3">
        <h2 className="m-0 text-[16px] font-bold tracking-[-0.01em]">{t('albums.settingsTitle')}</h2>
        <button
          type="button"
          className="cursor-pointer border-0 bg-transparent p-0 text-lg leading-none text-muted hover:text-ink"
          aria-label={t('albums.cancel')}
          onClick={onClose}
        >
          ×
        </button>
      </div>

      <div className="mb-4">
        <Segmented<AlbumSettingsTab> compact options={tabs} value={tab} onChange={onTabChange} />
      </div>

      {tab === 'share' && (
        <div className="flex flex-col gap-4" data-testid="album-settings-share">
          <section>
            <div className="mb-1.5 text-[12.5px] font-semibold text-ink">{t('albums.visibility')}</div>
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={pending || album.visibility === 'public'}
                onClick={() => onUpdate({ visibility: 'public' })}
              >
                {t('albums.setPublic')}
              </Button>
              <Button
                disabled={pending || album.visibility === 'private'}
                onClick={() => onUpdate({ visibility: 'private' })}
              >
                {t('albums.setPrivate')}
              </Button>
            </div>
          </section>

          {album.visibility === 'public' && (
            <section>
              <div className="mb-1.5 flex flex-wrap gap-2">
                <Button
                  onClick={() => {
                    void copyText(`${window.location.origin}/a/${albumId}`, t('albums.publicLink'))
                  }}
                >
                  {t('albums.copyPublicLink')}
                </Button>
                <Button
                  onClick={() => {
                    window.open(`/a/${albumId}`, '_blank', 'noopener,noreferrer')
                  }}
                >
                  {t('albums.openPublicLink')}
                </Button>
              </div>
            </section>
          )}

          <section data-testid="album-default-view">
            <div className="mb-0.5 text-[12.5px] font-semibold text-ink">{t('albums.defaultViewLabel')}</div>
            <div className="mb-2 text-[11.5px] text-muted">{t('albums.defaultViewHint')}</div>
            <Segmented<AlbumPublicMode>
              compact
              options={[
                { value: 'gallery', label: t('albums.modeGallery') },
                { value: 'immersive', label: t('albums.modeImmersive') },
              ]}
              value={defaultView}
              onChange={(v) => {
                if (v === defaultView) return
                onUpdate({ default_view: v })
                onSaved(t('albums.defaultViewSaved'))
              }}
            />
          </section>

          <section data-testid="album-click-to-immersive">
            <div className="mb-0.5 text-[12.5px] font-semibold text-ink">{t('albums.clickToImmersiveLabel')}</div>
            <div className="mb-2 text-[11.5px] text-muted">{t('albums.clickToImmersiveHint')}</div>
            <Segmented<'on' | 'off'>
              compact
              options={[
                { value: 'on', label: t('albums.clickToImmersiveOn') },
                { value: 'off', label: t('albums.clickToImmersiveOff') },
              ]}
              value={clickToImmersive ? 'on' : 'off'}
              onChange={(v) => {
                const next = v === 'on'
                if (next === clickToImmersive) return
                onUpdate({ click_to_immersive: next })
                onSaved(t('albums.clickToImmersiveSaved'))
              }}
            />
          </section>

          <section>
            <div className="mb-0.5 text-[12.5px] font-semibold text-ink">{t('albums.accessPassword')}</div>
            <div className="mb-1.5 text-[11.5px] text-muted">{t('albums.accessPasswordHint')}</div>
            <div className="mb-2 text-[11.5px] text-muted">
              {album.has_access_password ? t('albums.accessPasswordSet') : t('albums.accessPasswordNone')}
            </div>
            <input
              type="password"
              className="mb-2 box-border w-full rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-[13px] text-ink outline-none"
              value={pw}
              placeholder={t('albums.accessPasswordPlaceholder')}
              onChange={(e) => setPw(e.target.value)}
              autoComplete="new-password"
            />
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={pending || !pw.trim()}
                onClick={() => {
                  onUpdate({ access_password: pw.trim() })
                  setPw('')
                  onSaved(t('albums.accessPasswordSaved'))
                }}
              >
                {t('albums.save')}
              </Button>
              {album.has_access_password && (
                <Button
                  disabled={pending}
                  onClick={() => {
                    onUpdate({ access_password: '' })
                    setPw('')
                    onSaved(t('albums.accessPasswordSaved'))
                  }}
                >
                  {t('albums.clearPassword')}
                </Button>
              )}
            </div>
          </section>
        </div>
      )}

      {tab === 'content' && (
        <div className="flex flex-col gap-4" data-testid="album-settings-content">
          <section>
            <div className="mb-1.5 text-[12.5px] font-semibold text-ink">{t('albums.description')}</div>
            <textarea
              className="mb-2 box-border min-h-[88px] w-full resize-y rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-[13px] text-ink outline-none"
              value={desc}
              placeholder={t('albums.descriptionPlaceholder')}
              onChange={(e) => setDesc(e.target.value)}
            />
            <Button
              disabled={pending || desc === (album.description ?? '')}
              onClick={() => {
                onUpdate({ description: desc })
                onSaved(t('albums.descriptionSaved'))
              }}
            >
              {t('albums.save')}
            </Button>
          </section>

          <section>
            <label className="flex cursor-pointer items-start gap-2.5">
              <input
                type="checkbox"
                className="mt-0.5"
                checked={listInPlaza}
                disabled={pending}
                onChange={(e) => {
                  onUpdate({ list_in_plaza: e.target.checked })
                  onSaved(t('albums.listInPlazaSaved'))
                }}
              />
              <span>
                <span className="block text-[12.5px] font-semibold text-ink">{t('albums.listInPlaza')}</span>
                <span className="mt-0.5 block text-[11.5px] text-muted">{t('albums.listInPlazaHint')}</span>
              </span>
            </label>
          </section>

          <section>
            <div className="mb-1.5 text-[11.5px] text-muted">{t('albums.bulkVisHint')}</div>
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={setVisPending || !hasImages || album.visibility === 'private'}
                onClick={() => onBulkVis('public')}
              >
                {t('albums.bulkPublic')}
              </Button>
              <Button disabled={setVisPending || !hasImages} onClick={() => onBulkVis('private')}>
                {t('albums.bulkPrivate')}
              </Button>
            </div>
          </section>
        </div>
      )}

      {tab === 'stats' && (
        <div data-testid="album-stats">
          <div className="mb-2 text-[12.5px] font-semibold text-ink">{t('albums.statsTitle')}</div>
          {stats && Array.isArray(stats.daily) ? (
            <>
              <div className="mb-3 font-mono text-[12px] text-muted" data-testid="album-stats-total">
                {t('albums.viewsTotal', { count: stats.total ?? 0 })}
              </div>
              <div className="flex h-14 items-end gap-px">
                {stats.daily.map((d) => {
                  const max = Math.max(1, ...stats.daily.map((x) => x.views))
                  const h = Math.max(2, Math.round((d.views / max) * 56))
                  return (
                    <div
                      key={d.date}
                      title={`${d.date}: ${d.views}`}
                      className="flex-1 rounded-[1px] bg-btn/70"
                      style={{ height: h }}
                    />
                  )
                })}
              </div>
            </>
          ) : (
            <div className="py-6 text-center text-[13px] text-muted">{t('discover.loading')}</div>
          )}
        </div>
      )}
    </Modal>
  )
}
