import { useState } from 'react'
import { Link } from 'react-router'
import {
  useAdminLogs,
  useAdminRefererImages,
  useAdminStats,
  useCheckSystemUpdate,
  useSystemUpgrade,
  useSystemVersion,
} from '../../../api/adminHooks'
import { useT } from '../../../i18n'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { formatBytes, formatDate } from '../../../lib/format'
import { PageHeader } from '../../../shell/PageHeader'
import { Skeleton } from '../../../ui/Skeleton'
import { AdminQueryGate } from '../ui/AdminQueryGate'
import { ACTION_LABELS, dotColor } from '../ui/auditActions'
import styles from './DashboardPage.module.css'
import { TrendChart } from './TrendChart'

type RefWindow = 7 | 30

export function DashboardPage() {
  const { t } = useT()
  const stats = useAdminStats()
  const logs = useAdminLogs({ limit: 8 })
  const verQ = useSystemVersion()
  const checkUpdate = useCheckSystemUpdate()
  const doUpgrade = useSystemUpgrade()
  const [refWindow, setRefWindow] = useState<RefWindow>(30)
  const [selectedHost, setSelectedHost] = useState<string | null>(null)
  const [updateMsg, setUpdateMsg] = useState<string | null>(null)
  const [latestTag, setLatestTag] = useState<string | null>(null)
  const hostImages = useAdminRefererImages(selectedHost, refWindow)

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
          if (r.release_url) {
            useGlobal.getState().pushToast(t('adminA.updateAvailableToast', { latest: r.latest ?? '' }))
          }
        } else {
          setUpdateMsg(t('adminA.updateUpToDate', { current: r.current }))
        }
      },
    })
  }

  const onUpgrade = () => {
    if (!latestTag) return
    if (!window.confirm(t('adminA.upgradeConfirm', { latest: latestTag }))) return
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
        },
      },
    )
  }

  return (
    <div>
      <PageHeader kicker="DASHBOARD" title={t('adminA.dashTitle')} />
      <div className={styles.versionBar}>
        <span className={styles.versionLabel}>{t('adminA.runningVersion')}</span>
        <code className={styles.versionCode}>{verQ.data?.current ?? '…'}</code>
        <Button variant="secondary" disabled={checkUpdate.isPending} onClick={onCheckUpdate}>
          {t('adminA.checkUpdate')}
        </Button>
        {latestTag && (
          <Button variant="primary" disabled={doUpgrade.isPending} onClick={onUpgrade}>
            {t('adminA.upgradeTo', { latest: latestTag })}
          </Button>
        )}
        {updateMsg && (
          <span className={styles.versionMsg}>
            {updateMsg}
            {checkUpdate.data?.release_url && checkUpdate.data.update_available && (
              <>
                {' '}
                <a href={checkUpdate.data.release_url} target="_blank" rel="noreferrer">
                  {t('adminA.releaseNotes')}
                </a>
              </>
            )}
          </span>
        )}
      </div>
      <AdminQueryGate query={stats}>
        {(data) => {
          const referers =
            refWindow === 7
              ? (data.top_referers ?? [])
              : (data.top_referers_30d ?? data.top_referers ?? [])
          return (
        <>
          <div className={styles.sectionLabel}>{t('adminA.opsSection')}</div>
          <div className={styles.cards}>
            {[
              { label: t('adminA.usersCount'), value: String(data.users) },
              {
                label: t('adminA.signups30dTotal'),
                value: String((data.signups_30d ?? []).reduce((a, d) => a + d.count, 0)),
              },
              {
                label: t('adminA.bandwidthMonth'),
                value: formatBytes(data.bandwidth_used_month ?? 0),
              },
              { label: t('adminA.totalStorage'), value: formatBytes(data.storage) },
            ].map((c) => (
              <div key={c.label} className={styles.card}>
                <div className={styles.cardLabel}>{c.label}</div>
                <div className={styles.cardValue}>{c.value}</div>
              </div>
            ))}
          </div>
          {(data.bandwidth_top_users?.length ?? 0) > 0 && (
            <div className={styles.bwStrip}>
              <span className={styles.bwLabel}>{t('adminA.bandwidthTopUsers')}</span>
              {data.bandwidth_top_users!.map((u) => (
                <Link key={u.user_id} className={styles.bwChip} to={`/admin/users?q=${encodeURIComponent(u.username)}`}>
                  {u.username}
                  <span className={styles.bwAmt}>{formatBytes(u.used)}</span>
                </Link>
              ))}
            </div>
          )}

          <div className={styles.sectionLabel}>{t('adminA.inventorySection')}</div>
          <div className={styles.cards}>
            {[
              { label: t('adminA.imagesCount'), value: String(data.images) },
              { label: t('adminA.todayUploads'), value: String(data.today_uploads) },
              { label: t('adminA.pendingImages'), value: String(data.pending_images ?? 0) },
              { label: t('adminA.rejectedImages'), value: String(data.rejected_images ?? 0) },
              { label: t('adminA.tasksPending'), value: String(data.tasks_pending ?? 0) },
              { label: t('adminA.tasksRunning'), value: String(data.tasks_running ?? 0) },
            ].map((c) => (
              <div key={c.label} className={styles.card}>
                <div className={styles.cardLabel}>{c.label}</div>
                <div className={styles.cardValue}>{c.value}</div>
              </div>
            ))}
          </div>

          <div className={styles.panels}>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{t('adminA.signupsTrend30d')}</span>
                <span>{t('adminA.unitUsersPerDay')}</span>
              </div>
              <TrendChart daily={data.signups_30d ?? []} days={30} />
            </div>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{t('adminA.signupChannels')}</span>
              </div>
              {(data.signup_channels_30d ?? []).length === 0 ? (
                <div className={styles.eventEmpty}>{t('adminA.noSignups')}</div>
              ) : (
                <table className={styles.refTable}>
                  <thead>
                    <tr>
                      <th>{t('adminA.channel')}</th>
                      <th>{t('adminA.hits')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(data.signup_channels_30d ?? []).map((r) => (
                      <tr key={r.channel}>
                        <td>{channelLabel(t, r.channel)}</td>
                        <td className={styles.refCount}>{r.count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>

          <div className={styles.panels}>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{t('adminA.uploadTrend30d')}</span>
                <span>{t('adminA.unitImagesPerDay')}</span>
              </div>
              <TrendChart daily={data.daily ?? []} />
            </div>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{t('adminA.recentEvents')}</span>
              </div>
              <div className={styles.events}>
                {logs.data && logs.data.items.length === 0 && <div className={styles.eventEmpty}>{t('adminA.noEvents')}</div>}
                {logs.data?.items.map((l) => (
                  <div key={l.id} className={styles.event}>
                    <span className={styles.dot} style={{ background: dotColor(l.action) }} />
                    <div className={styles.eventBody}>
                      <div className={styles.eventText}>
                        {ACTION_LABELS[l.action] ?? l.action}
                        {l.actor_id != null && <span className={styles.eventActor}> · #{l.actor_id}</span>}
                      </div>
                      <div className={styles.eventTime}>{formatDate(l.created_at)}</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className={styles.panels}>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{refWindow === 7 ? t('adminA.traffic7d') : t('adminA.traffic30d')}</span>
                <span className={styles.windowToggle}>
                  <button type="button" className={refWindow === 7 ? styles.winActive : styles.winBtn} onClick={() => setRefWindow(7)}>
                    7d
                  </button>
                  <button type="button" className={refWindow === 30 ? styles.winActive : styles.winBtn} onClick={() => setRefWindow(30)}>
                    30d
                  </button>
                </span>
              </div>
              <TrendChart
                daily={(refWindow === 7
                  ? (data.traffic_7d ?? [])
                  : (data.traffic_30d ?? data.traffic_7d ?? [])
                ).map((d) => ({ date: d.date, count: d.views }))}
                days={refWindow}
              />
              <p className={styles.caveat}>{t('adminA.originMeteringNote')}</p>
            </div>
            <div className={styles.panel}>
              <div className={styles.panelHead}>
                <span>{t('adminA.topReferers')}</span>
                <span className={styles.windowToggle}>
                  <button type="button" className={refWindow === 7 ? styles.winActive : styles.winBtn} onClick={() => setRefWindow(7)}>
                    7d
                  </button>
                  <button type="button" className={refWindow === 30 ? styles.winActive : styles.winBtn} onClick={() => setRefWindow(30)}>
                    30d
                  </button>
                </span>
              </div>
              {referers.length === 0 ? (
                <div className={styles.eventEmpty}>{t('adminA.noReferers')}</div>
              ) : (
                <table className={styles.refTable}>
                  <thead>
                    <tr>
                      <th>{t('adminA.domain')}</th>
                      <th>{t('adminA.hits')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {referers.map((r) => (
                      <tr
                        key={r.host}
                        className={styles.refRow}
                        onClick={() => setSelectedHost(r.host === selectedHost ? null : r.host)}
                      >
                        <td>
                          {r.host}
                          {r.suspect ? <span className={styles.suspect}>{t('adminA.suspect')}</span> : null}
                        </td>
                        <td className={styles.refCount}>{r.count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              <p className={styles.caveat}>{t('adminA.originMeteringNote')}</p>
              {selectedHost && (
                <div className={styles.hostDetail}>
                  <div className={styles.panelHead}>
                    <span>
                      {t('adminA.refererImages')}: {selectedHost}
                    </span>
                    <button type="button" className={styles.winBtn} onClick={() => setSelectedHost(null)}>
                      ×
                    </button>
                  </div>
                  {hostImages.isLoading ? (
                    <Skeleton height={80} />
                  ) : (hostImages.data?.items ?? []).length === 0 ? (
                    <div className={styles.eventEmpty}>{t('adminA.noRefererImages')}</div>
                  ) : (
                    <table className={styles.refTable}>
                      <thead>
                        <tr>
                          <th>{t('adminA.imageKey')}</th>
                          <th>{t('adminA.hits')}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {hostImages.data!.items.map((it) => (
                          <tr key={it.key}>
                            <td>
                              <Link to={`/admin/images?q=${encodeURIComponent(it.key)}`}>{it.key}</Link>
                              {it.name ? <span className={styles.eventActor}> · {it.name}</span> : null}
                            </td>
                            <td className={styles.refCount}>{it.count}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              )}
            </div>
          </div>
        </>
          )
        }}
      </AdminQueryGate>
    </div>
  )
}

function channelLabel(t: (k: string) => string, ch: string): string {
  switch (ch) {
    case 'direct':
      return t('adminA.channelDirect')
    case 'invite':
      return t('adminA.channelInvite')
    case 'utm':
      return t('adminA.channelUtm')
    case 'referer':
      return t('adminA.channelReferer')
    default:
      return t('adminA.channelUnknown')
  }
}
