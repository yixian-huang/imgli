import { useEffect, useState } from 'react'
import { RetryImg } from '../../ui/RetryImg'
import { Link } from 'react-router'
import type { DiscoverRow } from '../../api/types'

interface Props {
  row: DiscoverRow
  onOpen: (row: DiscoverRow) => void
}

/** 广场/用户图库卡片：缩略图 + 作者行，点击整卡打开灯箱。 */
export function ImageCard({ row, onOpen }: Props) {
  const [imgFailed, setImgFailed] = useState(false)
  const [avatarFailed, setAvatarFailed] = useState(false)
  const displayName = row.author.nickname || row.author.username
  const initial = (displayName[0] || '?').toUpperCase()

  useEffect(() => {
    setImgFailed(false)
  }, [row.key])

  useEffect(() => {
    setAvatarFailed(false)
  }, [row.author.user_id, row.author.avatar_version])

  return (
    <div
      className="card cursor-pointer overflow-hidden rounded-md border border-border bg-surface transition-[transform,border-color,box-shadow] duration-150 ease-out hover:-translate-y-0.5 hover:border-muted hover:shadow-[0_4px_12px_rgba(0,0,0,0.06)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ink"
      role="button"
      tabIndex={0}
      onClick={() => onOpen(row)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onOpen(row)
        }
      }}
    >
      <div className="relative aspect-square bg-soft">
        {imgFailed ? (
          <div
            className="flex h-full w-full items-center justify-center font-mono text-[11px] tracking-[0.08em] text-muted"
            aria-hidden
          >
            {row.ext?.toUpperCase() || 'IMG'}
          </div>
        ) : (
          <RetryImg
            className="block h-full w-full object-cover"
            src={`/t/${row.key}`}
            alt={row.name}
            loading="lazy"
            onError={() => setImgFailed(true)}
          />
        )}
      </div>
      <div className="border-t border-border px-2.5 py-2">
        <Link
          to={`/u/${row.author.username}`}
          className="group/author flex min-w-0 items-center gap-2 text-ink no-underline"
          onClick={(e) => e.stopPropagation()}
        >
          {avatarFailed ? (
            <span
              className="flex h-[22px] w-[22px] flex-none items-center justify-center rounded-full bg-soft text-[11px] font-semibold text-muted"
              aria-hidden
            >
              {initial}
            </span>
          ) : (
            <RetryImg
              className="h-[22px] w-[22px] flex-none rounded-full bg-soft object-cover"
              src={`/avatar/${row.author.user_id}?v=${row.author.avatar_version}`}
              alt=""
              loading="lazy"
              onError={() => setAvatarFailed(true)}
            />
          )}
          <span className="overflow-hidden text-xs text-ellipsis whitespace-nowrap group-hover/author:underline">
            {displayName}
          </span>
        </Link>
      </div>
    </div>
  )
}
