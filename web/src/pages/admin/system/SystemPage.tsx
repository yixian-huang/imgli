import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import {
  useCheckSystemUpdate,
  useCleanupPreview,
  useCleanupRun,
  useSystemHealth,
  useSystemUpgrade,
  useSystemVersion,
  type DoctorLevel,
} from '../../../api/adminHooks'
import { useT } from '../../../i18n'
import { cn } from '../../../lib/cn'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { PageHeader } from '../../../shell/PageHeader'
import { AdminQueryGate } from '../ui/AdminQueryGate'

const CHECKLIST_KEY = 'imgli_ops_checklist_dismissed'

const sectionClass = 'mb-3.5 rounded-sm border border-border bg-surface p-[18px]'
const sectionHeadClass = 'mb-3.5 flex flex-wrap items-baseline justify-between gap-3'
const h2Class = 'm-0 text-sm font-bold tracking-[0.02em]'
const hintClass = 'mb-3 mt-0 text-xs leading-snug text-muted'
const bannerCodeClass =
  'my-2 mb-3 block overflow-x-auto whitespace-pre-wrap rounded-sm border border-border bg-bg px-2.5 py-2 font-mono text-xs'
const monoClass = 'font-mono text-xs'
const rowClass = 'mb-2.5 flex flex-wrap items-center gap-2'
const levelBaseClass = 'font-mono text-[11px] font-bold uppercase tracking-[0.06em]'

function levelTone(level: DoctorLevel): string {
  if (level === 'fail') return 'text-err'
  if (level === 'warn') return 'text-warn'
  return 'text-ok'
}

function normalizeOrigin(raw: string): string {
  try {
    const u = new URL(raw.includes('://') ? raw : `http://${raw}`)
    return `${u.protocol}//${u.host}`
  } catch {
    return raw.replace(/\/$/, '')
  }
}

function isLocalishBase(base: string): boolean {
  try {
    const u = new URL(base)
    const h = u.hostname
    return h === 'localhost' || h === '127.0.0.1' || h === '::1'
  } catch {
    return /localhost|127\.0\.0\.1/.test(base)
  }
}

export function SystemPage() {
  const { t } = useT()
  const healthQ = useSystemHealth()
  const verQ = useSystemVersion()
  const checkUpdate = useCheckSystemUpdate()
  const doUpgrade = useSystemUpgrade()
  const previewCleanup = useCleanupPreview()
  const runCleanup = useCleanupRun()

  const [updateMsg, setUpdateMsg] = useState<string | null>(null)
  const [latestTag, setLatestTag] = useState<string | null>(null)
  const [cleanupMsg, setCleanupMsg] = useState<string | null>(null)
  const [checklistDismissed, setChecklistDismissed] = useState(
    () => typeof localStorage !== 'undefined' && localStorage.getItem(CHECKLIST_KEY) === '1',
  )

  const browserOrigin = typeof window !== 'undefined' ? window.location.origin : ''

  const runtime = healthQ.data?.runtime
  const baseNorm = runtime?.base_url ? normalizeOrigin(runtime.base_url) : ''
  const browserNorm = browserOrigin ? normalizeOrigin(browserOrigin) : ''
  const originMismatch = !!(baseNorm && browserNorm && baseNorm !== browserNorm)
  const localBase = runtime?.base_url ? isLocalishBase(runtime.base_url) : false

  const showChecklist = !checklistDismissed && (localBase || originMismatch)

  const suggestedEnv = useMemo(() => {
    if (!browserOrigin) return 'IMGLI_BASE_URL=https://your.domain'
    return `IMGLI_BASE_URL=${browserOrigin}\nIMGLI_TRUST_PROXY=true`
  }, [browserOrigin])

  const dismissChecklist = () => {
    localStorage.setItem(CHECKLIST_KEY, '1')
    setChecklistDismissed(true)
  }

  const onCheckUpdate = () => {
    setUpdateMsg(null)
    setLatestTag(null)
    checkUpdate.mutate(undefined, {
      onSuccess: (r) => {
        if (r.error) {
          setUpdateMsg(t('adminA.updateCheckFailed', { err: r.error }))
          return
        }
        if (r.update_available) {
          setLatestTag(r.latest ?? null)
          setUpdateMsg(t('adminA.updateAvailable', { latest: r.latest ?? '?' }))
        } else {
          setUpdateMsg(t('adminA.updateUpToDate', { current: r.current }))
        }
      },
    })
  }

  const onUpgrade = () => {
    if (!latestTag) return
    const hard = healthQ.data?.doctor.hard_fail
    const warnN = healthQ.data?.doctor.checks.filter((c) => c.level === 'warn').length ?? 0
    let conf = t('adminA.upgradeConfirm', { latest: latestTag })
    if (hard) conf = t('adminA.upgradePreflightFail') + '\n\n' + conf
    else if (warnN > 0) conf = t('adminA.upgradePreflightWarn', { n: warnN }) + '\n\n' + conf
    if (runtime?.install === 'docker') {
      conf = t('adminA.upgradeDockerBlocked') + '\n\n' + conf
    }
    if (!window.confirm(conf)) return
    doUpgrade.mutate(
      { confirm: true, tag: latestTag },
      {
        onSuccess: (r) => {
          if (r.mode === 'docker_blocked' || r.error) {
            setUpdateMsg(r.message || r.error || t('adminA.upgradeFailed'))
            return
          }
          setUpdateMsg(r.message || t('adminA.upgradeDone', { to: r.to ?? latestTag }))
          useGlobal.getState().pushToast(t('adminA.upgradeDoneToast'))
          // re-exec 后进程会换新二进制；稍后刷新版本信息
          window.setTimeout(() => {
            verQ.refetch()
            healthQ.refetch()
          }, 1500)
        },
      },
    )
  }

  const onPreviewCleanup = () => {
    setCleanupMsg(null)
    previewCleanup.mutate(
      { kinds: ['expired', 'trash', 'group_retention', 'group_force_age'] },
      {
        onSuccess: (r) => {
          const parts = (r.items ?? []).map((it) => `${it.kind}:${it.count}`)
          setCleanupMsg(t('adminA.cleanupPreviewResult', { detail: parts.join(' · ') || '—' }))
        },
      },
    )
  }

  const onRunCleanup = () => {
    if (!window.confirm(t('adminA.cleanupRunConfirm'))) return
    setCleanupMsg(null)
    runCleanup.mutate(
      { kinds: ['expired', 'trash', 'group_retention', 'group_force_age'], confirm: true, limit: 200 },
      {
        onSuccess: (r) => {
          const parts = (r.items ?? []).map((it) => `${it.kind}: deleted ${it.deleted ?? 0}`)
          setCleanupMsg(t('adminA.cleanupRunResult', { detail: parts.join(' · ') || '—' }))
          useGlobal.getState().pushToast(t('adminA.cleanupDoneToast'))
        },
      },
    )
  }

  return (
    <div>
      <PageHeader kicker="SYSTEM" title={t('adminA.systemTitle')} />

      {showChecklist && (
        <div
          className="mb-4 rounded-sm border border-warn/50 bg-warn/10 px-4 py-3.5"
          role="status"
        >
          <div className="mb-1.5 font-bold">{t('adminA.checklistTitle')}</div>
          <div className="mb-2.5 text-[13px] leading-normal text-ink">{t('adminA.checklistBody')}</div>
          <code className={bannerCodeClass}>{suggestedEnv}</code>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="primary" onClick={dismissChecklist}>
              {t('adminA.checklistDismiss')}
            </Button>
            <a
              href="https://github.com/yixian-huang/imgli/blob/main/docs/security-hardening.md#faq-reverse-proxy-loginregister-cross-site-rejected"
              target="_blank"
              rel="noreferrer"
            >
              {t('adminA.checklistDocs')}
            </a>
          </div>
        </div>
      )}

      <AdminQueryGate query={healthQ}>
        {(data) => (
          <>
            <section className={sectionClass}>
              <div className={sectionHeadClass}>
                <h2 className={h2Class}>{t('adminA.runtimeTitle')}</h2>
                <Button variant="secondary" disabled={healthQ.isFetching} onClick={() => healthQ.refetch()}>
                  {t('adminA.refreshHealth')}
                </Button>
              </div>
              <div className="mb-3 grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2.5">
                {[
                  { label: t('adminA.runningVersion'), value: data.runtime.version || verQ.data?.current || '…' },
                  { label: 'IMGLI_BASE_URL', value: data.runtime.base_url },
                  { label: 'trust_proxy', value: String(data.runtime.trust_proxy) },
                  { label: 'listen', value: data.runtime.listen },
                  { label: t('adminA.installShape'), value: data.runtime.install },
                  { label: 'data_dir', value: data.runtime.data_dir },
                  {
                    label: t('adminA.imagingBackend'),
                    value: data.runtime.imaging_backend || '—',
                  },
                  {
                    label: t('adminA.webpEncode'),
                    value:
                      data.runtime.webp_encode === true
                        ? t('adminA.capabilityYes')
                        : data.runtime.webp_encode === false
                          ? t('adminA.capabilityNo')
                          : '—',
                  },
                  {
                    label: t('adminA.heicDecode'),
                    value:
                      data.runtime.heic_decode === true
                        ? t('adminA.capabilityYes')
                        : data.runtime.heic_decode === false
                          ? t('adminA.capabilityNo')
                          : '—',
                  },
                  {
                    label: t('adminA.thumbExt'),
                    value: data.runtime.thumb_ext ? `.${data.runtime.thumb_ext}` : '—',
                  },
                  { label: t('adminA.requestHost'), value: data.runtime.request_host || '—' },
                  {
                    label: 'X-Forwarded-Proto',
                    value: data.runtime.forwarded_proto || '—',
                  },
                  {
                    label: 'X-Forwarded-For',
                    value: data.runtime.forwarded_for_set ? t('adminA.headerPresent') : '—',
                  },
                ].map((c) => (
                  <div key={c.label}>
                    <div className="mb-1 font-mono text-2xs tracking-[0.08em] text-muted">{c.label}</div>
                    <div className="break-all text-[13px] font-semibold">{c.value}</div>
                  </div>
                ))}
              </div>
              <p className={hintClass}>
                {data.runtime.webp_encode
                  ? t('adminA.imagingHintVips')
                  : t('adminA.imagingHintPureGo')}{' '}
                <Link to="/admin/settings" className="text-ink underline-offset-2 hover:underline">
                  {t('adminA.imagingSettingsLink')}
                </Link>
              </p>
              {originMismatch ? (
                <div className="mb-3 rounded-sm border border-err/50 bg-err/10 px-3.5 py-3 text-[13px] leading-normal">
                  {t('adminA.originMismatch', { browser: browserNorm, base: baseNorm })}
                </div>
              ) : (
                <div className="mb-3 text-[13px] text-muted">{t('adminA.originMatch', { origin: browserNorm || baseNorm })}</div>
              )}
              {data.runtime.install === 'docker' && (
                <p className={hintClass}>{t('adminA.dockerUpgradeHint')}</p>
              )}
            </section>

            <section className={sectionClass}>
              <div className={sectionHeadClass}>
                <h2 className={h2Class}>{t('adminA.doctorTitle')}</h2>
                {data.doctor.hard_fail ? (
                  <span className={cn(levelBaseClass, 'text-err')}>FAIL</span>
                ) : (
                  <span className={cn(levelBaseClass, 'text-ok')}>OK</span>
                )}
              </div>
              <p className={hintClass}>{t('adminA.doctorHint')}</p>
              <table className="w-full border-collapse text-[13px] [&_th]:border-b [&_th]:border-border [&_th]:px-1.5 [&_th]:py-2 [&_th]:text-left [&_th]:align-top [&_th]:font-mono [&_th]:text-2xs [&_th]:font-semibold [&_th]:tracking-[0.08em] [&_th]:text-muted [&_td]:border-b [&_td]:border-border [&_td]:px-1.5 [&_td]:py-2 [&_td]:text-left [&_td]:align-top">
                <thead>
                  <tr>
                    <th>{t('adminA.doctorCheck')}</th>
                    <th>{t('adminA.doctorLevel')}</th>
                    <th>{t('adminA.doctorMessage')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.doctor.checks.map((c) => (
                    <tr key={c.name + c.message}>
                      <td className={monoClass}>{c.name}</td>
                      <td>
                        <span className={cn(levelBaseClass, levelTone(c.level))}>{c.level}</span>
                      </td>
                      <td>{c.message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          </>
        )}
      </AdminQueryGate>

      <section className={sectionClass}>
        <h2 className={h2Class}>{t('adminA.upgradeSection')}</h2>
        <p className={hintClass}>{t('adminA.upgradeSectionHint')}</p>
        <div className={rowClass}>
          <code className={monoClass}>{verQ.data?.current ?? healthQ.data?.runtime.version ?? '…'}</code>
          <Button variant="secondary" disabled={checkUpdate.isPending} onClick={onCheckUpdate}>
            {t('adminA.checkUpdate')}
          </Button>
          {latestTag && runtime?.install !== 'docker' && (
            <Button variant="primary" disabled={doUpgrade.isPending} onClick={onUpgrade}>
              {t('adminA.upgradeTo', { latest: latestTag })}
            </Button>
          )}
        </div>
        {runtime?.install === 'docker' && (
          <code className={bannerCodeClass}>{t('adminA.dockerRedeploySnippet')}</code>
        )}
        {updateMsg && <div className="text-[13px] text-muted">{updateMsg}</div>}
      </section>

      <section className={sectionClass}>
        <h2 className={h2Class}>{t('adminA.cleanupSection')}</h2>
        <p className={hintClass}>{t('adminA.cleanupHint')}</p>
        <div className={rowClass}>
          <Button variant="secondary" disabled={previewCleanup.isPending} onClick={onPreviewCleanup}>
            {t('adminA.cleanupPreview')}
          </Button>
          <Button variant="primary" disabled={runCleanup.isPending} onClick={onRunCleanup}>
            {t('adminA.cleanupRun')}
          </Button>
        </div>
        {cleanupMsg && <div className="text-[13px] text-muted">{cleanupMsg}</div>}
      </section>

      <section className={sectionClass}>
        <h2 className={h2Class}>{t('adminA.opsLinksTitle')}</h2>
        <div className="mt-2 flex flex-wrap gap-3 text-[13px] [&_a]:text-ok">
          <Link to="/admin/policies">{t('adminA.linkPoliciesMigrate')}</Link>
          <Link to="/admin/logs">{t('nav.logs')}</Link>
          <a href="https://github.com/yixian-huang/imgli/blob/main/docs/backup.md" target="_blank" rel="noreferrer">
            {t('adminA.linkBackupDocs')}
          </a>
        </div>
        <p className={cn(hintClass, 'mt-3')}>{t('adminA.backupHint')}</p>
      </section>
    </div>
  )
}
