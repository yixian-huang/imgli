import { useState } from 'react'
import { useParams, Link } from 'react-router'
import { ApiError } from '../../api/client'
import { useConfig, useShareImage, useUnlockShare } from '../../api/hooks'
import { useT } from '../../i18n'
import { copyText } from '../../lib/copy'
import { formatBytes } from '../../lib/format'
import { BrandLockup } from '../../ui/Brand'
import { Button } from '../../ui/Button'
import { RetryImg } from '../../ui/RetryImg'
import { Input } from '../../ui/Input'
import { LangToggle } from '../../ui/LangToggle'
import { ShareBrandFooter } from '../../ui/ShareBrandFooter'
import { useGlobal } from '../../store'

/** Public share landing: preview + copy links for public/normal images. */
export function SharePage() {
  const { key = '' } = useParams()
  const { t, lang } = useT()
  const theme = useGlobal((s) => s.theme)
  const toggleTheme = useGlobal((s) => s.toggleTheme)
  const q = useShareImage(key)
  const unlock = useUnlockShare()
  const cfg = useConfig()
  const data = q.data
  const [pw, setPw] = useState('')
  const siteName = (cfg.data?.site_name || 'imgli').trim() || 'imgli'

  const notFound = q.error instanceof ApiError && q.error.httpStatus === 404
  const needPw = !!(data?.password_required || (data?.has_access_password && !data?.links?.url))

  let expiryLabel = ''
  if (data?.expires_at) {
    const d = new Date(data.expires_at)
    expiryLabel = d.toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US')
  }

  return (
    <div className="min-h-dvh text-ink">
      <header className="flex items-center justify-between border-b border-border bg-surface px-5 py-3">
        <Link to="/" className="flex items-center text-inherit no-underline" aria-label={siteName}>
          <BrandLockup word={cfg.data?.site_name} />
        </Link>
        <div className="flex items-center gap-2.5">
          <button
            type="button"
            className="h-[34px] w-[34px] cursor-pointer rounded-sm border border-border bg-surface text-sm text-ink"
            title={t('nav.toggleTheme')}
            onClick={toggleTheme}
          >
            {theme === 'light' ? '◐' : '◑'}
          </button>
          <LangToggle />
          <Link
            to="/"
            className="inline-flex h-[34px] items-center rounded-sm border border-ink bg-ink px-3 text-sm-plus font-bold text-bg no-underline"
          >
            {t('share.uploadCta')}
          </Link>
        </div>
      </header>

      <main className="mx-auto max-w-[960px] px-4 pt-7 pb-12">
        {q.isLoading && <div className="px-4 py-12 text-center text-muted">{t('share.loading')}</div>}
        {notFound && (
          <div className="mx-auto my-10 max-w-[420px] px-4 py-12 text-center text-muted">
            <div className="mb-2 font-mono text-2xs tracking-[0.12em] text-muted">NOT FOUND</div>
            <h1 className="mb-2 mt-0 text-[22px] font-bold text-ink">{t('share.notFoundTitle')}</h1>
            <p className="mb-[18px] mt-0 text-[13.5px] leading-normal">{t('share.notFoundDesc')}</p>
            <Link to="/">
              <Button variant="primary">{t('share.uploadCta')}</Button>
            </Link>
          </div>
        )}
        {q.isError && !notFound && (
          <div className="px-4 py-12 text-center text-muted">{t('share.loadFailed')}</div>
        )}
        {data && needPw && (
          <div className="mx-auto my-10 max-w-[420px] px-4 py-12 text-center text-muted">
            <div className="mb-2 font-mono text-2xs tracking-[0.12em] text-muted">PASSWORD</div>
            <h1 className="mb-2 mt-0 text-[22px] font-bold text-ink">{t('share.passwordTitle')}</h1>
            <p className="mb-[18px] mt-0 text-[13.5px] leading-normal">{t('share.passwordHint')}</p>
            <form
              className="mx-auto flex max-w-[320px] flex-col gap-3 text-left"
              onSubmit={(e) => {
                e.preventDefault()
                unlock.mutate(
                  { key, password: pw },
                  {
                    onError: () => {
                      /* toast via mutation error display below */
                    },
                  },
                )
              }}
            >
              <Input
                type="password"
                label={t('share.passwordPlaceholder')}
                value={pw}
                onChange={(e) => setPw(e.target.value)}
                autoComplete="current-password"
              />
              {unlock.isError && (
                <p className="mb-[18px] mt-0 text-[13.5px] leading-normal">
                  {unlock.error instanceof ApiError && unlock.error.httpStatus === 401
                    ? t('share.passwordWrong')
                    : t('share.loadFailed')}
                </p>
              )}
              <Button variant="primary" type="submit" disabled={!pw.trim() || unlock.isPending}>
                {t('share.passwordSubmit')}
              </Button>
            </form>
          </div>
        )}
        {data && !needPw && (
          <div className="grid grid-cols-[1.2fr_1fr] gap-6 overflow-hidden rounded border border-border bg-surface max-md:grid-cols-1">
            <div className="flex min-h-[280px] items-center justify-center bg-soft p-4 max-md:min-h-[200px]">
              <RetryImg
                className="max-h-[min(70vh,560px)] max-w-full rounded-[2px] object-contain"
                src={data.links.url}
                alt={data.name}
              />
            </div>
            <div className="flex flex-col gap-3 px-[22px] pt-[22px] pb-6">
              <h1 className="m-0 text-lg font-bold leading-snug break-all">{data.name}</h1>
              <div className="flex flex-wrap items-center gap-1 font-mono text-[11.5px] text-muted">
                {data.width > 0 && data.height > 0 && (
                  <span>
                    {data.width}×{data.height}
                  </span>
                )}
                {data.size > 0 && (
                  <>
                    <span className="opacity-50">·</span>
                    <span>{formatBytes(data.size)}</span>
                  </>
                )}
                {expiryLabel && (
                  <>
                    <span className="opacity-50">·</span>
                    <span>{t('share.expires', { date: expiryLabel })}</span>
                  </>
                )}
                {!!data.max_views && data.max_views > 0 && (
                  <>
                    <span className="opacity-50">·</span>
                    <span>
                      {t('images.maxViewsUsed', {
                        used: data.views_served ?? 0,
                        max: data.max_views,
                      })}
                    </span>
                  </>
                )}
              </div>
              <div className="mt-1 flex flex-wrap gap-2">
                <Button
                  variant="primary"
                  onClick={() => copyText(data.links.url, t('share.copyUrl'))}
                >
                  {t('share.copyUrl')}
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => copyText(data.links.markdown, t('share.copyMarkdown'))}
                >
                  {t('share.copyMarkdown')}
                </Button>
                {(data.share_url || data.links.share_url) && (
                  <Button
                    variant="secondary"
                    onClick={() =>
                      copyText(data.share_url || data.links.share_url || '', t('share.copyShare'))
                    }
                  >
                    {t('share.copyShare')}
                  </Button>
                )}
              </div>
              <pre className="mt-1 mb-0 max-h-[88px] overflow-auto rounded-sm border border-border bg-bg px-3 py-2.5 font-mono text-[11px] leading-normal break-all whitespace-pre-wrap text-muted">
                {data.links.url}
              </pre>
            </div>
          </div>
        )}
      </main>
      <ShareBrandFooter
        siteName={siteName}
        branding={cfg.data?.share_branding || 'off'}
        helpURL={cfg.data?.help_url}
        upgradeURL={cfg.data?.upgrade_url}
        className="mx-auto max-w-[960px] px-4 pt-2 pb-7"
      />
    </div>
  )
}
