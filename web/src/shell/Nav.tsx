import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router'
import { useConfig, useLogout, useQuota } from '../api/hooks'
import type { User } from '../api/types'
import { useT } from '../i18n'
import { cn } from '../lib/cn'
import { useGlobal } from '../store'
import { BrandLockup } from '../ui/Brand'
import { RetryImg } from '../ui/RetryImg'
import { LangToggle } from '../ui/LangToggle'
import { NavQuotaCluster } from '../ui/QuotaBar'

const baseLinks: { to: string; key: string }[] = [
  { to: '/', key: 'upload' },
  { to: '/images', key: 'myImages' },
  { to: '/albums', key: 'albums' },
  { to: '/settings', key: 'settings' },
]

const themeBtn =
  'flex h-[30px] w-[30px] cursor-pointer items-center justify-center rounded-sm border border-border bg-surface text-sm text-ink hover:bg-soft'

export function Nav({ user }: { user: User }) {
  const { t, lang } = useT()
  const theme = useGlobal((s) => s.theme)
  const toggleTheme = useGlobal((s) => s.toggleTheme)
  const { data: config } = useConfig()
  const quota = useQuota()
  const logout = useLogout()
  const navigate = useNavigate()
  const [menuOpen, setMenuOpen] = useState(false)
  const [imgFailed, setImgFailed] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const links = useMemo(() => {
    const withLabel = baseLinks.map((l) => ({ ...l, label: t(`nav.${l.key}`) }))
    if (!config?.plaza_enabled) return withLabel
    const i = withLabel.findIndex((l) => l.to === '/settings')
    const next = [...withLabel]
    next.splice(i >= 0 ? i : next.length, 0, { to: '/explore', key: 'plaza', label: t('nav.plaza') })
    return next
  }, [config?.plaza_enabled, lang, t])

  useEffect(() => {
    setImgFailed(false)
  }, [user.avatar_url])

  useEffect(() => {
    if (!menuOpen) return
    const close = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [menuOpen])

  return (
    <header className="sticky top-0 z-10 flex h-14 items-center gap-9 border-b border-border bg-surface px-8 max-md:gap-4 max-md:px-4">
      <Link to="/" className="flex items-center text-ink hover:text-ink" aria-label={t('nav.homeAria')}>
        <BrandLockup beta word={config?.site_name} />
      </Link>
      <nav className="flex h-14 items-stretch gap-[26px] text-[13.5px] font-semibold max-md:hidden">
        {links.map((l) => (
          <NavLink
            key={l.to}
            to={l.to}
            end={l.to === '/'}
            className={({ isActive }) =>
              cn('flex items-center text-muted hover:text-ink', isActive && 'text-ink shadow-[inset_0_-2px_0_var(--text)]')
            }
          >
            {l.label}
          </NavLink>
        ))}
      </nav>
      <div className="ml-auto flex flex-none items-center gap-3">
        {quota.data && (
          <NavQuotaCluster
            storage={{ used: quota.data.used, total: quota.data.total }}
            bandwidth={
              (quota.data.bandwidth_quota_month ?? 0) > 0
                ? {
                    used: quota.data.bandwidth_used_month ?? 0,
                    total: quota.data.bandwidth_quota_month ?? 0,
                  }
                : null
            }
          />
        )}
        <button type="button" className={themeBtn} title={t('nav.toggleTheme')} onClick={toggleTheme}>
          {theme === 'light' ? '◐' : '◑'}
        </button>
        <LangToggle />
        <div className="relative" ref={menuRef}>
          <button
            type="button"
            className="avatar flex h-[30px] w-[30px] cursor-pointer items-center justify-center overflow-hidden rounded-full border border-border bg-soft p-0 text-xs font-bold text-ink"
            onClick={() => setMenuOpen((v) => !v)}
          >
            {user.avatar_url && !imgFailed ? (
              <RetryImg className="block h-full w-full rounded-full object-cover" src={user.avatar_url} alt="" onError={() => setImgFailed(true)} />
            ) : (
              (user.nickname || user.username).slice(0, 1)
            )}
          </button>
          {menuOpen && (
            <div className="absolute top-[38px] right-0 z-20 flex min-w-[180px] animate-[rise_0.2s] flex-col rounded-sm border border-border bg-surface p-1.5 shadow-[0_12px_32px_rgba(0,0,0,0.12)]">
              <div className="mb-1 border-b border-border px-2.5 py-2">
                <div className="text-sm-plus font-bold">{user.nickname || user.username}</div>
                <div className="mt-0.5 font-mono text-xs-plus text-muted">{user.email}</div>
              </div>
              {user.is_admin && (
                <Link to="/admin" className="block rounded-sm px-2.5 py-2 text-left text-sm-plus font-semibold text-ink hover:bg-soft" onClick={() => setMenuOpen(false)}>
                  {t('nav.admin')}
                </Link>
              )}
              <Link to="/settings" className="block rounded-sm px-2.5 py-2 text-left text-sm-plus font-semibold text-ink hover:bg-soft" onClick={() => setMenuOpen(false)}>
                {t('nav.settings')}
              </Link>
              <button
                type="button"
                className="block cursor-pointer rounded-sm border-0 bg-transparent px-2.5 py-2 text-left text-sm-plus font-semibold text-err hover:bg-soft"
                onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}
              >
                {t('nav.logout')}
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  )
}
