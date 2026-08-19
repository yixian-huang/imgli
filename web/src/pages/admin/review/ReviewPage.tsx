import { useAdminImagesBatch, useAdminReview, useReviewBatch, useReviewDecide } from '../../../api/adminHooks'
import { useT } from '../../../i18n'
import { formatBytes } from '../../../lib/format'
import { useAdminSearchParam } from '../../../lib/useAdminSearchParam'
import { PageHeader } from '../../../shell/PageHeader'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { EmptyState } from '../../../ui/EmptyState'
import { RetryImg } from '../../../ui/RetryImg'
import { Tag } from '../../../ui/Tag'
import { AdminQueryGate } from '../ui/AdminQueryGate'
import { Pager } from '../ui/Pager'

function nsfwVariant(score: number | null): 'ok' | 'warn' | 'err' | 'muted' {
  if (score == null) return 'muted'
  if (score >= 0.8) return 'err'
  if (score >= 0.5) return 'warn'
  return 'ok'
}

export function ReviewPage() {
  const { t } = useT()
  const { params, setParam } = useAdminSearchParam()
  const page = Number(params.get('page')) || 1
  const q = useAdminReview(page)
  const decide = useReviewDecide()
  const batch = useReviewBatch()
  const purgeBatch = useAdminImagesBatch()

  const setPage = (p: number) => setParam('page', p > 1 ? String(p) : '')

  const busy = decide.isPending || batch.isPending || purgeBatch.isPending
  const pageKeys = (q.data?.items ?? []).map((i) => i.key)

  return (
    <div>
      <PageHeader
        kicker="REVIEW QUEUE"
        title={t('adminA.reviewTitle')}
        extra={
          pageKeys.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="primary"
                disabled={busy}
                onClick={() =>
                  batch.mutate(
                    { keys: pageKeys, action: 'approve' },
                    {
                      onSuccess: (data) => {
                        const ok = data.results.filter((r) => r.ok).length
                        const failed = data.results.length - ok
                        if (failed > 0) useGlobal.getState().pushToast(t('adminA.batchPartial', { ok, failed }))
                      },
                    },
                  )
                }
              >
                {t('adminA.approveAll', { count: pageKeys.length })}
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                onClick={() => {
                  if (!window.confirm(t('adminA.reviewPurgeConfirm', { count: pageKeys.length }))) return
                  purgeBatch.mutate(
                    { keys: pageKeys, action: 'purge' },
                    {
                      onSuccess: (data) => {
                        const ok = data.results.filter((r) => r.ok).length
                        const failed = data.results.length - ok
                        useGlobal.getState().pushToast(
                          failed === 0
                            ? t('adminA.batchImagesDone', { ok, action: t('adminA.verbPurge') })
                            : t('adminA.batchImagesPartial', { ok, failed }),
                        )
                      },
                    },
                  )
                }}
              >
                {t('adminA.reviewPurgePage', { count: pageKeys.length })}
              </Button>
            </div>
          ) : undefined
        }
      />
      <AdminQueryGate query={q}>
        {(data) => {
          const items = data.items
          return items.length === 0 ? (
            data.total > 0 ? (
              <EmptyState badge="✓" title={t('adminA.pageCleared')} desc={t('adminA.pageClearedReviewDesc')}>
                <Button variant="secondary" onClick={() => setPage(1)}>
                  {t('adminA.backToPage1')}
                </Button>
              </EmptyState>
            ) : (
              <EmptyState badge="✓" title="ALL CLEAR" desc={t('adminA.allClearDesc')} />
            )
          ) : (
            <>
              <div className="mt-2 grid grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-4">
                {items.map((it) => (
                  <div key={it.key} className="flex gap-3.5 rounded-sm border border-border bg-surface p-3">
                    <RetryImg
                      className="h-[140px] w-[140px] flex-none rounded-sm bg-soft object-cover"
                      src={it.links.thumbnail_url}
                      alt={it.name}
                      loading="lazy"
                    />
                    <div className="flex min-w-0 flex-col gap-1.5">
                      <div className="overflow-hidden text-ellipsis whitespace-nowrap font-semibold">{it.name}</div>
                      <div className="text-[13px] text-muted">
                        {it.username}（#{it.user_id}） · {formatBytes(it.size)}
                      </div>
                      <div className="my-0.5">
                        <Tag variant={nsfwVariant(it.nsfw_score)}>
                          NSFW {it.nsfw_score == null ? '—' : it.nsfw_score.toFixed(2)}
                        </Tag>
                      </div>
                      {it.triggers && it.triggers.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5 text-xs text-muted" title={t('adminA.triggerReasons')}>
                          {it.triggers.map((tr, i) => {
                            const bits = [tr.plugin, tr.severity]
                            if (tr.score != null) bits.push(tr.score.toFixed(2))
                            if (tr.hits?.length) bits.push(tr.hits.join(','))
                            return (
                              <span
                                key={`${tr.plugin}-${i}`}
                                className="max-w-full overflow-hidden text-ellipsis whitespace-nowrap rounded-sm border border-border bg-soft px-1.5 py-0.5 font-mono"
                              >
                                {bits.join(' · ')}
                              </span>
                            )
                          })}
                        </div>
                      ) : null}
                      <div className="mt-auto flex gap-2">
                        <Button variant="primary" disabled={busy} onClick={() => decide.mutate({ key: it.key, action: 'approve' })}>
                          {t('adminA.approve')}
                        </Button>
                        <Button variant="danger" disabled={busy} onClick={() => decide.mutate({ key: it.key, action: 'reject' })}>
                          {t('adminA.reject')}
                        </Button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <p className="mt-4 mb-2 text-[13px] text-muted">{t('adminA.pendingTotal', { total: data.total })}</p>
              <Pager page={page} limit={data.limit} total={data.total} onPage={setPage} />
            </>
          )
        }}
      </AdminQueryGate>
    </div>
  )
}
