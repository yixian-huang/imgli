import { Link } from 'react-router'
import type { PublicAlbumCard } from '../../api/types'
import { useT } from '../../i18n'
import { RetryImg } from '../../ui/RetryImg'

type Props = { card: PublicAlbumCard }

/** 广场/公开主页相册卡片。 */
export function AlbumCard({ card }: Props) {
  const { t } = useT()
  const title = card.name
  const by = card.nickname || card.username
  const src = card.thumbnail_url || card.cover_url || ''

  return (
    <Link
      to={`/a/${card.id}`}
      className="group block overflow-hidden rounded-sm border border-border bg-surface text-inherit no-underline transition-shadow hover:shadow-[0_4px_16px_rgba(0,0,0,0.08)]"
      data-testid="plaza-album-card"
    >
      <div className="relative aspect-[4/3] bg-soft">
        {src ? (
          <RetryImg src={src} alt="" className="absolute inset-0 size-full object-cover transition-transform duration-300 group-hover:scale-[1.03]" />
        ) : null}
      </div>
      <div className="px-3 py-2.5">
        <div className="truncate text-[13.5px] font-semibold">{title}</div>
        <div className="mt-1 flex flex-wrap gap-x-2 text-[11.5px] text-muted">
          <span>@{card.username || by}</span>
          <span>{t('discover.albumImages', { count: card.image_count })}</span>
          {typeof card.views === 'number' && card.views > 0 && (
            <span>{t('discover.albumViews', { count: card.views })}</span>
          )}
        </div>
      </div>
    </Link>
  )
}
