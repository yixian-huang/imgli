import { useEffect } from 'react'
import { Link } from 'react-router'
import type { DiscoverRow } from '../../api/types'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { RetryImg } from '../../ui/RetryImg'

interface Props {
  row: DiscoverRow | null
  onClose: () => void
}

/** 广场灯箱：大图预览 + 复制外链 + 作者入口。row 为 null 时不渲染。 */
export function Lightbox({ row, onClose }: Props) {
  const { t } = useT()

  useEffect(() => {
    if (!row) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [row, onClose])

  if (!row) return null

  const displayName = row.author.nickname || row.author.username
  const externalUrl = `${location.origin}/i/${row.key}`

  return (
    <div
      className="fixed inset-0 z-50 flex animate-[fadeIn_0.15s] items-center justify-center bg-black/72 p-3 sm:p-6"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="relative flex max-h-[92vh] max-w-[min(960px,100%)] flex-col items-center"
        role="dialog"
        aria-modal="true"
        aria-label={row.name}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          type="button"
          className="absolute top-1 right-1 z-[2] flex h-8 w-8 cursor-pointer items-center justify-center rounded-full border border-white/25 bg-black/55 text-lg leading-none text-white hover:bg-black/80 sm:-top-2 sm:-right-2"
          title={t('discover.close')}
          onClick={onClose}
        >
          ×
        </button>
        <RetryImg
          className="block max-h-[78vh] max-w-full rounded bg-black/20 object-contain sm:max-h-[90vh]"
          src={`/i/${row.key}`}
          alt={row.name}
          onClick={(e) => e.stopPropagation()}
        />
        <div className="mt-3 flex w-full items-center justify-between gap-4 px-1">
          <Link
            to={`/u/${row.author.username}`}
            className="min-w-0 overflow-hidden text-[13px] text-ellipsis whitespace-nowrap text-white/90 no-underline hover:text-white hover:underline"
            onClick={onClose}
          >
            {displayName}
          </Link>
          <button
            type="button"
            className="h-[30px] flex-none cursor-pointer rounded-sm border border-white/28 bg-white/12 px-3.5 text-xs font-semibold text-white hover:bg-white/20"
            onClick={() => copyText(externalUrl, t('discover.externalLink'))}
          >
            {t('discover.copyExternal')}
          </button>
        </div>
      </div>
    </div>
  )
}
