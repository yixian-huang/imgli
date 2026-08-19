import { memo, useRef, type RefObject } from 'react'
import type { ImageItem } from '../../api/types'
import { useT } from '../../i18n'
import { cn } from '../../lib/cn'
import { copyText } from '../../lib/copy'
import { formatBytes, formatDate } from '../../lib/format'
import type { View } from '../../store'
import { ArmedButton } from '../../ui/ArmedButton'
import { RetryImg } from '../../ui/RetryImg'
import { useWindowVirtual } from './useWindowVirtual'

const LIST_ROW_H = 53 // ≈ padding + 36px thumb + border

interface CardActions {
  onToggleSelect(key: string): void
  onOpen(key: string): void
  onQuickVis(item: ImageItem): void
  onQuickDel(key: string): void
}

interface Props extends CardActions {
  items: ImageItem[]
  view: View
  selected: Set<string>
  sentinelRef: RefObject<HTMLDivElement | null>
  loadingMore: boolean
}

const quickBtn =
  'flex h-6 w-6 cursor-pointer items-center justify-center rounded-[2px] border-0 bg-surface text-[11px] text-ink hover:bg-soft'
const quickDel = 'text-xs text-err'
const quickDelArmed =
  'w-auto min-w-6 bg-err px-1.5 text-2xs font-semibold tracking-[0.02em] text-white shadow-[0_0_0_1px_var(--err)]'

/** 卡内删除钮：第一击转红并显示「确认」，2.5s 未确认还原；第二击才真正删除。 */
function QuickDel({ onConfirm }: { onConfirm(): void }) {
  const { t } = useT()
  return (
    <ArmedButton
      title={t('images.delete')}
      armedTitle={t('images.confirmDelete')}
      className={cn(quickBtn, quickDel)}
      armedClassName={quickDelArmed}
      armedChildren={t('images.confirmShort')}
      onConfirm={onConfirm}
    >
      ×
    </ArmedButton>
  )
}

function Check({ item, selected, onToggleSelect }: { item: ImageItem; selected: boolean } & Pick<CardActions, 'onToggleSelect'>) {
  const { t } = useT()
  return (
    <button
      type="button"
      title={t('images.select')}
      className={cn(
        'absolute top-2 left-2 z-[2] flex h-[18px] w-[18px] cursor-pointer items-center justify-center rounded-[2px] border border-border bg-surface p-0 text-[11px] leading-none text-btn-text',
        selected && 'border-btn bg-btn',
      )}
      onClick={(e) => {
        e.stopPropagation()
        onToggleSelect(item.key)
      }}
    >
      {selected ? '✓' : ''}
    </button>
  )
}

function HoverActions({ item, onQuickVis, onQuickDel }: { item: ImageItem } & Pick<CardActions, 'onQuickVis' | 'onQuickDel'>) {
  const { t } = useT()
  return (
    <div className="absolute right-0 bottom-0 left-0 flex justify-end gap-1 bg-linear-to-t from-black/28 to-transparent p-1.5 opacity-0 transition-opacity duration-100 group-hover:opacity-100 max-md:opacity-100">
      <button
        type="button"
        title={t('images.copyUrlTitle')}
        className={quickBtn}
        onClick={(e) => {
          e.stopPropagation()
          copyText(item.links.url, t('images.linkNameUrl'))
        }}
      >
        ⧉
      </button>
      <button
        type="button"
        title={t('images.toggleVisibility')}
        className={quickBtn}
        onClick={(e) => {
          e.stopPropagation()
          onQuickVis(item)
        }}
      >
        {item.visibility === 'private' ? '◌' : '◉'}
      </button>
      <QuickDel onConfirm={() => onQuickDel(item.key)} />
    </div>
  )
}

const Card = memo(function Card({
  item,
  view,
  selected,
  ...a
}: { item: ImageItem; view: View; selected: boolean } & CardActions) {
  const { t } = useT()
  return (
    <div
      className={cn(
        // "card" kept for e2e [class*=card]
        'card group relative animate-[rise_0.28s_both] cursor-pointer overflow-hidden rounded-sm border border-border bg-surface [content-visibility:auto] [contain-intrinsic-size:220px] hover:border-muted',
        view === 'masonry' && 'mb-3.5 break-inside-avoid',
        selected && 'border-ink',
      )}
      onClick={() => a.onOpen(item.key)}
    >
      <div
        className={cn('relative aspect-square bg-soft', view === 'masonry' && 'aspect-auto')}
        style={view === 'masonry' ? { aspectRatio: `${item.width || 1} / ${item.height || 1}` } : undefined}
      >
        <RetryImg className="block h-full w-full object-cover" src={item.links.thumbnail_url} alt={item.name} loading="lazy" />
        <Check item={item} selected={selected} onToggleSelect={a.onToggleSelect} />
        {item.visibility === 'private' && (
          <span className="absolute top-2 right-2 rounded-[2px] bg-btn px-1.5 py-px font-mono text-[9px] tracking-[0.08em] text-btn-text">
            {t('images.privateBadge')}
          </span>
        )}
        <HoverActions item={item} onQuickVis={a.onQuickVis} onQuickDel={a.onQuickDel} />
      </div>
      <div className="flex items-center gap-2 border-t border-border px-2.5 py-2">
        <span className="min-w-0 flex-1 overflow-hidden font-mono text-xs-plus text-ellipsis whitespace-nowrap text-ink">
          {item.name}
        </span>
        <span className="flex-none font-mono text-2xs text-muted">{formatBytes(item.size)}</span>
      </div>
    </div>
  )
})

const listCols = 'grid-cols-[34px_44px_1.8fr_1fr_0.7fr_0.6fr_0.7fr_1fr_auto]'

const ListRow = memo(function ListRow({
  item,
  selected,
  ...a
}: { item: ImageItem; selected: boolean } & CardActions) {
  const { t } = useT()
  return (
    <div
      className={cn(
        'group grid min-h-[53px] cursor-pointer items-center gap-3 border-b border-border px-3.5 py-2 box-border',
        listCols,
        'hover:bg-soft',
      )}
      onClick={() => a.onOpen(item.key)}
    >
      <button
        type="button"
        title={t('images.select')}
        className={cn(
          'static flex h-[18px] w-[18px] cursor-pointer items-center justify-center rounded-[2px] border border-border bg-surface p-0 text-[11px] leading-none text-btn-text',
          selected && 'border-btn bg-btn',
        )}
        onClick={(e) => {
          e.stopPropagation()
          a.onToggleSelect(item.key)
        }}
      >
        {selected ? '✓' : ''}
      </button>
      <RetryImg
        className="h-9 w-9 rounded-[2px] border border-border object-cover bg-soft"
        src={item.links.thumbnail_url}
        alt=""
        loading="lazy"
      />
      <span className="overflow-hidden font-mono text-[11px] text-ellipsis whitespace-nowrap">{item.name}</span>
      <span className="font-mono text-xs-plus text-muted">
        {item.width} × {item.height}
      </span>
      <span className="font-mono text-xs-plus text-muted">{formatBytes(item.size)}</span>
      <span className="font-mono text-xs-plus text-muted">{item.ext.toUpperCase()}</span>
      <span
        className={cn(
          'justify-self-start rounded-[2px] border border-border px-[7px] py-px font-mono text-[9.5px] tracking-[0.06em] text-muted',
          item.visibility === 'public' && 'text-ink',
        )}
      >
        {item.visibility === 'public' ? 'PUBLIC' : 'PRIVATE'}
      </span>
      <span className="font-mono text-xs-plus text-muted">{formatDate(item.created_at)}</span>
      <div className="flex gap-1 opacity-75 group-hover:opacity-100">
        <button
          type="button"
          title={t('images.copyUrlTitle')}
          className={cn(quickBtn, 'border border-border')}
          onClick={(e) => {
            e.stopPropagation()
            copyText(item.links.url, t('images.linkNameUrl'))
          }}
        >
          ⧉
        </button>
        <QuickDel onConfirm={() => a.onQuickDel(item.key)} />
      </div>
    </div>
  )
})

export function ImageGrid({ items, view, selected, sentinelRef, loadingMore, ...a }: Props) {
  const { t } = useT()
  const listBodyRef = useRef<HTMLDivElement>(null)
  // 列表视图：窗口滚动虚拟化，只挂载可视区行（无限滚动仍用 sentinel）
  const virt = useWindowVirtual(view === 'list' ? items.length : 0, LIST_ROW_H, listBodyRef)
  const listSlice = view === 'list' ? items.slice(virt.start, virt.end) : items

  return (
    <>
      {view === 'list' ? (
        <div className="overflow-hidden rounded-sm border border-border bg-surface">
          <div
            className={cn(
              'grid items-center gap-3 border-b border-border bg-soft px-3.5 py-2 font-mono text-2xs tracking-[0.1em] text-muted box-border min-h-[53px]',
              listCols,
            )}
          >
            <span></span>
            <span></span>
            <span>{t('images.colName')}</span>
            <span>{t('images.colDims')}</span>
            <span>{t('images.colSize')}</span>
            <span>{t('images.colFormat')}</span>
            <span>{t('images.colVisibility')}</span>
            <span>{t('images.colUploaded')}</span>
            <span></span>
          </div>
          <div ref={listBodyRef} className="relative w-full" style={{ height: virt.totalHeight || undefined }}>
            <div style={{ transform: `translateY(${virt.offsetY}px)` }}>
              {listSlice.map((i) => (
                <ListRow key={i.key} item={i} selected={selected.has(i.key)} {...a} />
              ))}
            </div>
          </div>
        </div>
      ) : (
        <div
          className={
            view === 'masonry'
              ? 'columns-2 gap-3.5 md:columns-4'
              : 'grid grid-cols-[repeat(auto-fill,minmax(140px,1fr))] gap-3.5 md:grid-cols-[repeat(auto-fill,minmax(170px,1fr))]'
          }
        >
          {items.map((i) => (
            <Card key={i.key} item={i} view={view} selected={selected.has(i.key)} {...a} />
          ))}
        </div>
      )}
      <div ref={sentinelRef} className="h-px">
        {loadingMore && (
          <span className="block p-4 text-center font-mono text-[11px] text-muted">{t('images.loadingMore')}</span>
        )}
      </div>
    </>
  )
}
