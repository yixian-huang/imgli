import { Link } from 'react-router'
import type { PublicAlbumImg, PublicAlbumMeta } from '../../api/types'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { RetryImg } from '../../ui/RetryImg'
import { albumPageURL } from './albumLinks'

type Props = {
  meta: PublicAlbumMeta
  cover: PublicAlbumImg | null
  coverIndex: number
  loading: boolean
  onEnterImmersive: (index0: number) => void
}

/** 杂志封面 hero；无封面时回落为简头。 */
export function PublicAlbumHero({ meta, cover, coverIndex, loading, onEnterImmersive }: Props) {
  const { t } = useT()
  const owner = meta.owner
  const ownerLabel = owner ? owner.nickname || owner.username : ''

  if (cover && !loading) {
    return (
      <section
        className="relative mb-8 overflow-hidden rounded-md bg-soft"
        data-testid="album-hero"
      >
        <div className="relative aspect-[16/9] max-h-[min(52vh,480px)] w-full min-h-[220px] sm:aspect-[21/9]">
          <RetryImg src={cover.url} alt="" className="absolute inset-0 size-full object-cover" />
          <div className="absolute inset-0 bg-gradient-to-t from-black/75 via-black/25 to-black/10" />
          <div className="absolute inset-x-0 bottom-0 flex flex-wrap items-end justify-between gap-4 p-5 sm:p-7">
            <div className="min-w-0 text-white">
              <div className="mb-2 font-mono text-[10px] tracking-[0.18em] text-white/65">ALBUM</div>
              <h1 className="m-0 text-[28px] font-bold tracking-[-0.03em] break-words drop-shadow sm:text-[34px]">
                {meta.name}
              </h1>
              <p className="mt-2 mb-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-[13px] text-white/75">
                <span>{t('albums.publicCount', { count: meta.image_count })}</span>
                {owner && (
                  <>
                    <span className="opacity-50">·</span>
                    {owner.public_profile ? (
                      <Link
                        to={`/u/${encodeURIComponent(owner.username)}`}
                        className="text-white no-underline hover:underline"
                      >
                        {t('albums.publicBy', { name: ownerLabel })}
                      </Link>
                    ) : (
                      <span>{t('albums.publicBy', { name: ownerLabel })}</span>
                    )}
                  </>
                )}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                className="h-9 cursor-pointer rounded-sm border-0 bg-white px-4 text-[12.5px] font-bold text-ink hover:bg-white/90"
                onClick={() => onEnterImmersive(Math.max(0, coverIndex))}
              >
                {t('albums.heroEnter')}
              </button>
              <button
                type="button"
                className="h-9 cursor-pointer rounded-sm border border-white/35 bg-black/30 px-4 text-[12.5px] font-semibold text-white backdrop-blur-sm hover:bg-black/45"
                onClick={() => void copyText(albumPageURL(meta.id, 'gallery', 0), t('albums.publicLink'))}
              >
                {t('albums.copyAlbumLink')}
              </button>
            </div>
          </div>
        </div>
      </section>
    )
  }

  return (
    <header className="mb-7 flex flex-wrap items-end justify-between gap-4 border-b border-border pb-5">
      <div className="min-w-0">
        <div className="mb-2 font-mono text-[10px] tracking-[0.16em] text-muted">ALBUM</div>
        <h1 className="m-0 text-[26px] font-bold tracking-[-0.02em] break-words sm:text-[30px]">
          {meta.name}
        </h1>
        <p className="mt-2 mb-0 text-[13px] text-muted">
          {t('albums.publicCount', { count: meta.image_count })}
        </p>
      </div>
    </header>
  )
}
