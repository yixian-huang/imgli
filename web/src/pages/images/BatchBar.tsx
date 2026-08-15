import { useEffect, useMemo, useState } from 'react'
import { useAlbums, useBatchImages } from '../../api/hooks'
import type { BatchResult, ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { albumForcesPrivate } from '../../lib/albumPrivacy'
import { previewBatchRename } from '../../lib/batchRename'
import { cn } from '../../lib/cn'
import { copyText } from '../../lib/copy'
import { useGlobal } from '../../store'
import { Button } from '../../ui/Button'
import { Modal } from '../../ui/Modal'

interface Props {
  selected: Set<string>
  items: ImageItem[]
  onClear(): void
  albumId?: number
}

function summarize(
  results: BatchResult[],
  verb: string,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  const skipped = results.filter((r) => r.skipped).length
  const ok = results.filter((r) => r.ok && !r.skipped).length
  const bad = results.filter((r) => !r.ok).length
  if (bad === 0 && skipped === 0) return t('images.batchDone', { verb, ok })
  if (bad === 0) return t('images.batchDoneWithSkip', { verb, ok, skipped })
  // 无 skip 时沿用旧文案，避免「跳过 0」
  if (skipped === 0) return t('images.batchDonePartial', { verb, ok, bad })
  return t('images.batchDonePartialSkip', { verb, ok, skipped, bad })
}

const actCls =
  'cursor-pointer whitespace-nowrap rounded-sm border-0 bg-[rgba(128,128,128,0.22)] px-3 py-[7px] text-xs font-semibold text-btn-text hover:bg-[rgba(128,128,128,0.38)]'

const fieldCls =
  'mb-2 box-border w-full rounded-sm border border-border bg-bg px-2.5 py-2 font-mono text-sm text-ink outline-none'

export function BatchBar({ selected, items, onClear, albumId }: Props) {
  const { t } = useT()
  const batch = useBatchImages()
  const albums = useAlbums()
  const pushToast = useGlobal((s) => s.pushToast)
  const [modal, setModal] = useState<'vis' | 'move' | 'rename' | null>(null)
  const [moveTo, setMoveTo] = useState<number | 'none'>('none')
  const [findText, setFindText] = useState('')
  const [replaceText, setReplaceText] = useState('')
  const [renamePattern, setRenamePattern] = useState('')
  const [startN, setStartN] = useState(1)
  const [ignoreCase, setIgnoreCase] = useState(true)
  const [cleanSep, setCleanSep] = useState(true)
  const [onlyChanged, setOnlyChanged] = useState(true)
  const [delArmed, setDelArmed] = useState(false)

  useEffect(() => {
    if (!delArmed) return
    const timer = setTimeout(() => setDelArmed(false), 2500)
    return () => clearTimeout(timer)
  }, [delArmed])

  const selectedItems = useMemo(
    () => items.filter((i) => selected.has(i.key)),
    [items, selected],
  )

  const albumName = useMemo(() => {
    if (albumId == null) return ''
    return albums.data?.items.find((a) => a.id === albumId)?.name ?? ''
  }, [albumId, albums.data?.items])
  const blockBatchPublic =
    selectedItems.length > 0 &&
    (albumForcesPrivate(albums.data?.items, albumId) ||
      selectedItems.every((i) => albumForcesPrivate(albums.data?.items, i.album_id)))

  const previewRows = useMemo(
    () =>
      previewBatchRename(
        selectedItems.map((i) => ({
          key: i.key,
          name: i.name,
          ext: i.ext || '',
          created_at: i.created_at,
        })),
        {
          find: findText,
          replace: replaceText,
          ignoreCase,
          cleanSeparators: cleanSep,
          pattern: renamePattern,
          startN,
          album: albumName,
        },
      ),
    [selectedItems, findText, replaceText, ignoreCase, cleanSep, renamePattern, startN, albumName],
  )

  const visiblePreview = useMemo(
    () => (onlyChanged ? previewRows.filter((r) => r.status !== 'unchanged') : previewRows),
    [previewRows, onlyChanged],
  )

  const stats = useMemo(() => {
    let change = 0
    let skip = 0
    let conflict = 0
    let empty = 0
    for (const r of previewRows) {
      if (r.status === 'ok') change++
      else if (r.status === 'unchanged') skip++
      else if (r.status === 'conflict') conflict++
      else empty++
    }
    return { change, skip, conflict, empty }
  }, [previewRows])

  if (selected.size === 0) return null
  const keys = selectedItems.map((i) => i.key)

  const CHUNK = 100

  type BatchBody = {
    action: 'delete' | 'visibility' | 'move' | 'rename'
    keys: string[]
    visibility?: string
    album_id?: number | null
    name_pattern?: string
    find?: string
    replace?: string
    replace_ignore_case?: boolean
    clean_separators?: boolean
    start_n?: number
    album_name?: string
  }

  const run = (body: BatchBody, verb: string) => {
    const chunks: string[][] = []
    for (let i = 0; i < body.keys.length; i += CHUNK) chunks.push(body.keys.slice(i, i + CHUNK))
    ;(async () => {
      const all: BatchResult[] = []
      try {
        for (const chunkKeys of chunks) {
          const d = await batch.mutateAsync({ ...body, keys: chunkKeys })
          all.push(...d.results)
        }
        pushToast(summarize(all, verb, t))
        setModal(null)
        onClear()
      } catch {
        pushToast(all.length ? t('images.batchRestFailed', { summary: summarize(all, verb, t) }) : t('images.opFailed'))
      }
    })()
  }

  const copyAll = () => {
    const urls = selectedItems.map((i) => i.links.url)
    copyText(urls.join('\n'), t('images.nLinks', { count: urls.length }))
  }

  const openModal = (m: 'vis' | 'move' | 'rename') => {
    setDelArmed(false)
    if (m === 'move') setMoveTo('none')
    if (m === 'rename') {
      setFindText('')
      setReplaceText('')
      setRenamePattern('')
      setStartN(1)
      setIgnoreCase(true)
      setCleanSep(true)
      setOnlyChanged(true)
    }
    setModal(m)
  }

  const renameReady =
    (!!findText.trim() || !!renamePattern.trim()) &&
    stats.change > 0 &&
    stats.conflict === 0 &&
    stats.empty === 0

  const confirmRename = () => {
    run(
      {
        action: 'rename',
        keys,
        find: findText.trim() || undefined,
        replace: replaceText,
        name_pattern: renamePattern.trim() || undefined,
        replace_ignore_case: ignoreCase,
        clean_separators: cleanSep,
        start_n: startN >= 1 ? startN : 1,
        album_name: albumName || undefined,
      },
      t('images.verbRename'),
    )
  }

  return (
    <>
      <div className="fixed bottom-[72px] left-1/2 z-30 flex max-w-[calc(100vw-24px)] -translate-x-1/2 animate-[fadeIn_0.2s] items-center gap-1 overflow-x-auto rounded bg-btn py-2 pr-2 pl-4 text-btn-text shadow-[0_8px_28px_rgba(0,0,0,0.25)] md:bottom-6 md:max-w-none">
        <span className="mr-2.5 whitespace-nowrap text-sm-plus font-bold">{t('images.selected', { count: selected.size })}</span>
        <button type="button" className={actCls} onClick={copyAll}>
          {t('images.copyLinks')}
        </button>
        <button type="button" className={actCls} onClick={() => openModal('rename')}>
          {t('images.batchRename')}
        </button>
        <button type="button" className={actCls} onClick={() => openModal('vis')}>
          {t('images.changeVisibility')}
        </button>
        <button type="button" className={actCls} onClick={() => openModal('move')}>
          {t('images.moveToAlbum')}
        </button>
        {albumId != null && (
          <button
            type="button"
            className={actCls}
            onClick={() => run({ action: 'move', keys, album_id: null }, t('images.verbRemoveAlbum'))}
          >
            {t('images.removeFromAlbumShort')}
          </button>
        )}
        <button
          type="button"
          className={cn(actCls, delArmed && 'bg-err text-white hover:bg-err')}
          onClick={() => {
            if (delArmed) {
              setDelArmed(false)
              run({ action: 'delete', keys }, t('images.verbDelete'))
            } else setDelArmed(true)
          }}
        >
          {delArmed ? t('images.confirmDeleteQ') : t('images.delete')}
        </button>
        <button
          type="button"
          className="cursor-pointer border-0 bg-transparent px-2.5 py-[7px] text-sm leading-none text-btn-text opacity-70 hover:opacity-100"
          title={t('images.cancelSelect')}
          onClick={onClear}
        >
          ×
        </button>
      </div>

      <Modal open={modal === 'vis'} onClose={() => setModal(null)}>
        <div className="mb-1.5 font-mono text-2xs tracking-[0.14em] text-muted">BATCH VISIBILITY</div>
        <div className="mb-3 text-[14.5px] font-bold">{t('images.setAs', { count: selected.size })}</div>
        <div className="flex flex-col gap-1.5">
          <Button
            disabled={blockBatchPublic}
            title={blockBatchPublic ? t('images.albumForcesPrivate') : undefined}
            onClick={() => run({ action: 'visibility', keys, visibility: 'public' }, t('images.verbPublic'))}
          >
            {t('images.setPublic')}
          </Button>
          <Button onClick={() => run({ action: 'visibility', keys, visibility: 'private' }, t('images.verbPrivate'))}>
            {t('images.setPrivate')}
          </Button>
        </div>
      </Modal>

      <Modal open={modal === 'move'} onClose={() => setModal(null)}>
        <div className="mb-1.5 font-mono text-2xs tracking-[0.14em] text-muted">BATCH MOVE</div>
        <div className="mb-3 text-[14.5px] font-bold">{t('images.moveN', { count: selected.size })}</div>
        <select
          className="mb-3 box-border w-full cursor-pointer rounded-sm border border-border bg-bg px-2.5 py-2 font-inherit text-sm-plus text-ink outline-none"
          aria-label={t('images.targetAlbum')}
          value={String(moveTo)}
          onChange={(e) => setMoveTo(e.target.value === 'none' ? 'none' : Number(e.target.value))}
        >
          <option value="none">{t('images.removeFromAlbum')}</option>
          {albums.data?.items.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
        <Button
          variant="primary"
          onClick={() => run({ action: 'move', keys, album_id: moveTo === 'none' ? null : moveTo }, t('images.verbMove'))}
        >
          {t('images.confirmMove')}
        </Button>
      </Modal>

      <Modal open={modal === 'rename'} onClose={() => setModal(null)} width={500}>
        <div className="mb-1.5 font-mono text-2xs tracking-[0.14em] text-muted">BATCH RENAME</div>
        <div className="mb-1 text-[14.5px] font-bold">{t('images.renameN', { count: selected.size })}</div>
        <p className="mt-0 mb-3 text-[12px] text-muted">{t('images.renamePipelineHint')}</p>

        <div className="mb-1 text-[11.5px] font-semibold text-muted">{t('images.renameStep1')}</div>
        <label className="mb-1 block text-[11px] text-muted">{t('images.renameFind')}</label>
        <textarea
          className={cn(fieldCls, 'min-h-[56px] resize-y')}
          value={findText}
          onChange={(e) => setFindText(e.target.value)}
          placeholder={t('images.renameFindPlaceholder')}
          aria-label={t('images.renameFind')}
        />
        <label className="mb-1 block text-[11px] text-muted">{t('images.renameReplace')}</label>
        <input
          className={fieldCls}
          value={replaceText}
          onChange={(e) => setReplaceText(e.target.value)}
          placeholder={t('images.renameReplacePlaceholder')}
          aria-label={t('images.renameReplace')}
        />

        <div className="mb-1 mt-1 text-[11.5px] font-semibold text-muted">{t('images.renameStep2')}</div>
        <p className="mt-0 mb-1.5 text-[11px] leading-snug text-muted">{t('images.renameTokensHint')}</p>
        <input
          className={fieldCls}
          value={renamePattern}
          onChange={(e) => setRenamePattern(e.target.value)}
          placeholder={t('images.renamePatternPlaceholder')}
          aria-label={t('images.renamePattern')}
        />
        <div className="mb-2 flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-1.5 text-[12px] text-ink">
            <span className="text-muted">{t('images.renameStartN')}</span>
            <input
              type="number"
              min={1}
              className="w-16 rounded-sm border border-border bg-bg px-2 py-1 font-mono text-sm outline-none"
              value={startN}
              onChange={(e) => setStartN(Math.max(1, Number.parseInt(e.target.value, 10) || 1))}
            />
          </label>
        </div>

        <div className="mb-2 flex flex-col gap-1.5 text-[12px] text-ink">
          <label className="flex cursor-pointer items-center gap-2">
            <input type="checkbox" checked={ignoreCase} onChange={(e) => setIgnoreCase(e.target.checked)} />
            {t('images.renameIgnoreCase')}
          </label>
          <label className="flex cursor-pointer items-center gap-2">
            <input type="checkbox" checked={cleanSep} onChange={(e) => setCleanSep(e.target.checked)} />
            {t('images.renameCleanSep')}
          </label>
          <label className="flex cursor-pointer items-center gap-2">
            <input type="checkbox" checked={onlyChanged} onChange={(e) => setOnlyChanged(e.target.checked)} />
            {t('images.renameOnlyChanged')}
          </label>
        </div>

        <div className="mb-2 font-mono text-[11px] text-muted" data-testid="batch-rename-stats">
          {t('images.renameStats', {
            change: stats.change,
            skip: stats.skip,
            conflict: stats.conflict,
          })}
        </div>

        {visiblePreview.length > 0 && (
          <ul
            className="mb-3 max-h-[200px] list-none overflow-y-auto rounded-sm border border-border bg-soft px-3 py-1.5 font-mono text-[11px]"
            data-testid="batch-rename-preview"
          >
            {visiblePreview.map((r) => (
              <li
                key={r.key}
                className={cn(
                  'border-b border-border/50 py-1.5 last:border-0',
                  r.status === 'conflict' && 'text-err',
                  r.status === 'empty' && 'text-err',
                  r.status === 'unchanged' && 'opacity-55',
                )}
              >
                <div className="truncate text-muted">{r.from}</div>
                <div className="truncate text-ink">
                  → {r.to}
                  {r.status === 'conflict' && ` (${t('images.renameConflict')})`}
                  {r.status === 'unchanged' && ` (${t('images.renameUnchanged')})`}
                  {r.status === 'empty' && ` (${t('images.renameEmpty')})`}
                </div>
              </li>
            ))}
          </ul>
        )}
        {visiblePreview.length === 0 && previewRows.length > 0 && (
          <div className="mb-3 text-[12px] text-muted">{t('images.renameNoChanges')}</div>
        )}

        <Button variant="primary" disabled={!renameReady} onClick={confirmRename}>
          {t('images.confirmRename')}
        </Button>
      </Modal>
    </>
  )
}
