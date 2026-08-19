import { useEffect, useState } from 'react'
import {
  useAdminImages,
  useAdminImagesBatch,
  useAdminPolicies,
  useDeleteAdminImage,
  usePurgeAdminImage,
  useRestoreAdminImage,
  useSetImageWhitelist,
} from '../../../api/adminHooks'
import type { AdminImageItem } from '../../../api/types'
import { useT } from '../../../i18n'
import { cn } from '../../../lib/cn'
import { formatBytes } from '../../../lib/format'
import { useAdminSearchParam } from '../../../lib/useAdminSearchParam'
import { PageHeader } from '../../../shell/PageHeader'
import { useGlobal } from '../../../store'
import { ArmedButton } from '../../../ui/ArmedButton'
import { Button } from '../../../ui/Button'
import { EmptyState } from '../../../ui/EmptyState'
import { RetryImg } from '../../../ui/RetryImg'
import { AdminFilters, AdminSelect } from '../ui/adminChrome'
import { AdminQueryGate } from '../ui/AdminQueryGate'
import { Pager } from '../ui/Pager'
import { AdminImageDetail } from './AdminImageDetail'

function purgeToast(
  t: (k: string, v?: Record<string, string | number>) => string,
  res: { physical_queued?: boolean; object_retained?: boolean },
) {
  if (res.object_retained) return t('adminA.toastPurgedRetained')
  if (res.physical_queued) return t('adminA.toastPurgedQueued')
  return t('adminA.toastPurgedNoQueue')
}

const quickBtn =
  'flex h-6 w-6 cursor-pointer items-center justify-center rounded-sm border-0 bg-surface text-[11px] text-ink hover:bg-soft'
const quickArmed = 'w-auto min-w-6 bg-err px-1.5 text-2xs font-semibold text-white'
const batchAct =
  'cursor-pointer whitespace-nowrap rounded-sm border-0 bg-[rgba(128,128,128,0.22)] px-3 py-[7px] text-xs font-semibold text-btn-text hover:enabled:bg-[rgba(128,128,128,0.38)] disabled:cursor-not-allowed disabled:opacity-55'
const toolBtn =
  'cursor-pointer rounded-sm border border-border bg-surface px-2.5 py-1 font-mono text-[11px] text-ink hover:border-muted'

export function ImagesAdminPage() {
  const { t } = useT()
  const pushToast = useGlobal((s) => s.pushToast)
  const { params, setParam } = useAdminSearchParam()
  const user = Number(params.get('user')) || undefined
  const status = params.get('status') ?? ''
  const policy = Number(params.get('policy')) || undefined
  const deleted = params.get('deleted') || 'live'
  const page = Number(params.get('page')) || 1

  const images = useAdminImages({
    user,
    status: status || undefined,
    policy,
    deleted: deleted === 'live' ? undefined : deleted,
    page,
  })
  const policiesQ = useAdminPolicies()
  const wl = useSetImageWhitelist()
  const delM = useDeleteAdminImage()
  const purgeM = usePurgeAdminImage()
  const restoreM = useRestoreAdminImage()
  const batchM = useAdminImagesBatch()
  const [detail, setDetail] = useState<AdminImageItem | null>(null)
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const [trashArmed, setTrashArmed] = useState(false)
  const [purgeArmed, setPurgeArmed] = useState(false)
  const [restoreArmed, setRestoreArmed] = useState(false)

  // 筛选/翻页时清空选择，避免跨页误删
  useEffect(() => {
    setSelected(new Set())
    setTrashArmed(false)
    setPurgeArmed(false)
    setRestoreArmed(false)
  }, [user, status, policy, deleted, page])

  useEffect(() => {
    if (!trashArmed) return
    const timer = setTimeout(() => setTrashArmed(false), 2500)
    return () => clearTimeout(timer)
  }, [trashArmed])
  useEffect(() => {
    if (!purgeArmed) return
    const timer = setTimeout(() => setPurgeArmed(false), 2500)
    return () => clearTimeout(timer)
  }, [purgeArmed])
  useEffect(() => {
    if (!restoreArmed) return
    const timer = setTimeout(() => setRestoreArmed(false), 2500)
    return () => clearTimeout(timer)
  }, [restoreArmed])

  const onSoftDelete = (it: AdminImageItem) => {
    if (it.in_trash || it.user_id == null) {
      purgeM.mutate(it.key, {
        onSuccess: (res) => pushToast(purgeToast(t, res)),
      })
      return
    }
    delM.mutate(it.key, {
      onSuccess: (res) => {
        if (res.permanent) pushToast(purgeToast(t, res))
        else pushToast(t('adminA.toastMovedToTrash'))
      },
    })
  }

  const onPurge = (it: AdminImageItem) => {
    purgeM.mutate(it.key, {
      onSuccess: (res) => pushToast(purgeToast(t, res)),
    })
  }

  const onRestore = (it: AdminImageItem) => {
    restoreM.mutate(it.key, {
      onSuccess: () => pushToast(t('adminA.toastRestored')),
    })
  }

  const selectPage = (items: AdminImageItem[]) => {
    setSelected(new Set(items.map((i) => i.key)))
  }

  const runBatch = (action: 'trash' | 'purge' | 'restore') => {
    const keys = [...selected]
    if (keys.length === 0) return
    setTrashArmed(false)
    setPurgeArmed(false)
    setRestoreArmed(false)
    const CHUNK = 100
    const verb =
      action === 'purge' ? t('adminA.verbPurge') : action === 'restore' ? t('adminA.verbRestore') : t('adminA.verbTrash')
    ;(async () => {
      let ok = 0
      let failed = 0
      try {
        for (let i = 0; i < keys.length; i += CHUNK) {
          const chunk = keys.slice(i, i + CHUNK)
          const data = await batchM.mutateAsync({ keys: chunk, action })
          for (const r of data.results) {
            if (r.ok) ok++
            else failed++
          }
        }
        pushToast(
          failed === 0
            ? t('adminA.batchImagesDone', { ok, action: verb })
            : t('adminA.batchImagesPartial', { ok, failed }),
        )
        setSelected(new Set())
      } catch {
        pushToast(t('adminA.batchImagesFailed'))
      }
    })()
  }

  const busy =
    delM.isPending || purgeM.isPending || restoreM.isPending || batchM.isPending || wl.isPending
  const items = images.data?.items ?? []
  const canSoftBatch = selected.size > 0 && [...selected].some((k) => {
    const it = items.find((i) => i.key === k)
    return it && !it.in_trash && it.user_id != null
  })
  const canRestoreBatch = selected.size > 0 && [...selected].some((k) => {
    const it = items.find((i) => i.key === k)
    return it && it.in_trash
  })

  const badgeBase =
    'rounded-sm border border-warn bg-surface px-1.5 py-px font-mono text-[9px] tracking-[0.08em] text-warn'

  return (
    <div>
      <PageHeader
        kicker="ALL IMAGES"
        title={t('adminA.imagesTitle')}
        extra={
          <AdminFilters>
            {user && (
              <span className="inline-flex items-center gap-1.5 rounded-sm border border-border bg-soft px-2 py-1 font-mono text-xs-plus">
                {t('adminA.userFilterChip', { user })}
                <button
                  type="button"
                  className="cursor-pointer border-0 bg-transparent p-0 text-xs leading-none text-muted hover:text-err"
                  aria-label={t('adminA.clearUserFilterAria')}
                  onClick={() => setParam('user', '')}
                >
                  ×
                </button>
              </span>
            )}
            <AdminSelect
              value={deleted}
              onChange={(e) => setParam('deleted', e.target.value === 'live' ? '' : e.target.value)}
              aria-label={t('adminA.filterScopeAria')}
            >
              <option value="live">{t('adminA.scopeLive')}</option>
              <option value="trash">{t('adminA.scopeTrash')}</option>
              <option value="all">{t('adminA.scopeAll')}</option>
            </AdminSelect>
            <AdminSelect value={status} onChange={(e) => setParam('status', e.target.value)} aria-label={t('adminA.filterStatusAria')}>
              <option value="">{t('adminA.allStatuses')}</option>
              <option value="normal">{t('adminA.statusNormal')}</option>
              <option value="pending">{t('adminA.statusPending')}</option>
              <option value="rejected">{t('adminA.statusRejected')}</option>
            </AdminSelect>
            <AdminSelect value={policy ?? ''} onChange={(e) => setParam('policy', e.target.value)} aria-label={t('adminA.filterPolicyAria')}>
              <option value="">{t('adminA.allPolicies')}</option>
              {(policiesQ.data?.items ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </AdminSelect>
          </AdminFilters>
        }
      />
      <AdminQueryGate query={images}>
        {(data) =>
          data.items.length === 0 ? (
            data.total > 0 ? (
              <EmptyState badge="✓" title={t('adminA.pageCleared')} desc={t('adminA.pageClearedImagesDesc')}>
                <Button variant="secondary" onClick={() => setParam('page', '')}>
                  {t('adminA.backToPage1')}
                </Button>
              </EmptyState>
            ) : (
              <EmptyState title={t('adminA.noMatchingImages')} desc={t('adminA.noMatchingImagesDesc')} />
            )
          ) : (
            <>
              <div className="mb-3 flex flex-wrap items-center gap-2.5">
                <button type="button" className={toolBtn} onClick={() => selectPage(data.items)}>
                  {t('adminA.selectPage')}
                </button>
                {selected.size > 0 && (
                  <button type="button" className={toolBtn} onClick={() => setSelected(new Set())}>
                    {t('adminA.clearSelection')}
                  </button>
                )}
                <span className="font-mono text-xs-plus text-muted">{t('adminA.selectHint')}</span>
              </div>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(170px,1fr))] gap-3.5">
                {data.items.map((it) => {
                  const showSoft = !it.in_trash && it.user_id != null
                  const isSel = selected.has(it.key)
                  return (
                    <div
                      key={it.key}
                      className={cn(
                        // "card" kept for e2e [class*=card]
                        'card group relative animate-[rise_0.28s_both] cursor-pointer overflow-hidden rounded-sm border border-border bg-surface hover:border-muted',
                        isSel && 'border-btn shadow-[0_0_0_1px_var(--btn)]',
                      )}
                      onClick={() => setDetail(it)}
                    >
                      <div className="relative aspect-square bg-soft">
                        <RetryImg className="block h-full w-full object-cover" src={it.links.thumbnail_url} alt={it.name} loading="lazy" />
                        <label
                          className="absolute top-2 left-2 z-[2] flex h-[22px] w-[22px] cursor-pointer items-center justify-center rounded-sm bg-black/35"
                          onClick={(e) => e.stopPropagation()}
                          title={t('adminA.selectImage')}
                        >
                          <input
                            type="checkbox"
                            className="m-0 cursor-pointer"
                            checked={isSel}
                            onChange={() => {
                              setSelected((prev) => {
                                const next = new Set(prev)
                                if (next.has(it.key)) next.delete(it.key)
                                else next.add(it.key)
                                return next
                              })
                            }}
                          />
                        </label>
                        <div className="absolute top-2 right-2 flex gap-1">
                          {it.in_trash && <span className={cn(badgeBase, 'border-err text-err')}>{t('adminA.trashBadge')}</span>}
                          {it.status === 'pending' && <span className={badgeBase}>{t('adminA.statusPending')}</span>}
                          {it.status === 'rejected' && <span className={cn(badgeBase, 'border-err text-err')}>{t('adminA.statusRejected')}</span>}
                          {it.is_whitelisted && (
                            <span className="rounded-sm border border-btn bg-btn px-1.5 py-px font-mono text-[9px] tracking-[0.08em] text-btn-text">
                              WL
                            </span>
                          )}
                        </div>
                        <div className="absolute right-0 bottom-0 left-0 flex justify-end gap-1 bg-linear-to-t from-black/28 to-transparent p-1.5 opacity-0 transition-opacity duration-100 group-hover:opacity-100">
                          {!it.in_trash && (
                            <ArmedButton
                              title={it.is_whitelisted ? t('adminA.unwhitelist') : t('adminA.whitelist')}
                              armedTitle={it.is_whitelisted ? t('adminA.confirmUnwhitelist') : t('adminA.confirmWhitelist')}
                              className={quickBtn}
                              armedClassName={quickArmed}
                              armedChildren={t('adminA.confirmShort')}
                              onConfirm={() => wl.mutate({ key: it.key, on: !it.is_whitelisted })}
                            >
                              {t('adminA.whitelistGlyph')}
                            </ArmedButton>
                          )}
                          {showSoft && (
                            <ArmedButton
                              title={t('adminA.moveToTrash')}
                              armedTitle={t('adminA.confirmMoveToTrash')}
                              className={cn(quickBtn, 'text-xs text-err')}
                              armedClassName={quickArmed}
                              armedChildren={t('adminA.confirmShort')}
                              onConfirm={() => onSoftDelete(it)}
                            >
                              ×
                            </ArmedButton>
                          )}
                          {it.in_trash && (
                            <ArmedButton
                              title={t('adminA.restoreFromTrash')}
                              armedTitle={t('adminA.confirmRestore')}
                              className={cn(quickBtn, 'text-xs')}
                              armedClassName={quickArmed}
                              armedChildren={t('adminA.confirmShort')}
                              onConfirm={() => onRestore(it)}
                            >
                              ↺
                            </ArmedButton>
                          )}
                          <ArmedButton
                            title={t('adminA.purgePermanent')}
                            armedTitle={t('adminA.confirmPurge')}
                            className={cn(quickBtn, 'text-[13px] font-bold text-err')}
                            armedClassName={quickArmed}
                            armedChildren={t('adminA.confirmShort')}
                            onConfirm={() => onPurge(it)}
                          >
                            ⌫
                          </ArmedButton>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 border-t border-border px-2.5 py-2">
                        <span className="min-w-0 flex-1 overflow-hidden font-mono text-xs-plus text-ellipsis whitespace-nowrap text-ink">
                          {it.name}
                        </span>
                        <span className="flex-none font-mono text-2xs text-muted">
                          {it.user_id == null ? t('adminA.guestUploader') : it.username}
                        </span>
                        <span
                          className="max-w-[72px] flex-none overflow-hidden font-mono text-2xs text-ellipsis whitespace-nowrap text-muted"
                          title={it.path || undefined}
                        >
                          {it.policy_name || it.policy_driver || '—'}
                        </span>
                        <span className="flex-none font-mono text-2xs text-muted">{formatBytes(it.size)}</span>
                      </div>
                    </div>
                  )
                })}
              </div>
              <p className="mt-2.5 mr-0.5 ml-0.5 font-mono text-xs-plus text-muted">
                {t('adminA.imagesTotal', { total: data.total })}
              </p>
              <Pager
                page={page}
                limit={data.limit}
                total={data.total}
                onPage={(p) => setParam('page', p > 1 ? String(p) : '')}
              />
            </>
          )
        }
      </AdminQueryGate>

      {selected.size > 0 && (
        <div
          className="fixed bottom-6 left-1/2 z-40 flex -translate-x-1/2 items-center gap-1.5 rounded bg-btn py-2 pr-2 pl-4 text-btn-text shadow-[0_8px_28px_rgba(0,0,0,0.25)]"
          role="toolbar"
          aria-label={t('adminA.batchToolbar')}
        >
          <span className="mr-2 whitespace-nowrap text-sm-plus font-bold">
            {t('adminA.selectedCount', { count: selected.size })}
          </span>
          {canSoftBatch && (
            <button
              type="button"
              className={cn(batchAct, trashArmed && 'bg-err text-white hover:enabled:bg-err')}
              disabled={busy}
              onClick={() => {
                if (trashArmed) runBatch('trash')
                else {
                  setPurgeArmed(false)
                  setRestoreArmed(false)
                  setTrashArmed(true)
                }
              }}
            >
              {trashArmed ? t('adminA.confirmMoveToTrash') : t('adminA.batchMoveToTrash')}
            </button>
          )}
          {canRestoreBatch && (
            <button
              type="button"
              className={cn(batchAct, restoreArmed && 'bg-[rgba(255,255,255,0.22)]')}
              disabled={busy}
              onClick={() => {
                if (restoreArmed) runBatch('restore')
                else {
                  setTrashArmed(false)
                  setPurgeArmed(false)
                  setRestoreArmed(true)
                }
              }}
            >
              {restoreArmed ? t('adminA.confirmRestore') : t('adminA.batchRestore')}
            </button>
          )}
          <button
            type="button"
            className={cn(batchAct, purgeArmed && 'bg-err text-white hover:enabled:bg-err')}
            disabled={busy}
            onClick={() => {
              if (purgeArmed) runBatch('purge')
              else {
                setTrashArmed(false)
                setRestoreArmed(false)
                setPurgeArmed(true)
              }
            }}
          >
            {purgeArmed ? t('adminA.confirmPurge') : t('adminA.batchPurge')}
          </button>
          <button
            type="button"
            className="cursor-pointer border-0 bg-transparent px-2.5 py-[7px] text-sm leading-none text-btn-text opacity-70 hover:opacity-100"
            title={t('adminA.clearSelection')}
            onClick={() => setSelected(new Set())}
          >
            ×
          </button>
        </div>
      )}

      <AdminImageDetail item={detail} onClose={() => setDetail(null)} />
    </div>
  )
}
