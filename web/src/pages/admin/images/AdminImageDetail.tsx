import {
  useDeleteAdminImage,
  usePurgeAdminImage,
  useRestoreAdminImage,
  useSetImageWhitelist,
} from '../../../api/adminHooks'
import type { AdminImageDeleteResult, AdminImageItem } from '../../../api/types'
import { useT } from '../../../i18n'
import { copyText } from '../../../lib/copy'
import { formatBytes, formatDate } from '../../../lib/format'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { RetryImg } from '../../../ui/RetryImg'
import { InlineConfirm } from '../../../ui/InlineConfirm'
import { Modal } from '../../../ui/Modal'

function policyLabel(item: AdminImageItem): string {
  if (item.policy_name) return `${item.policy_name} (#${item.policy_id})`
  if (item.policy_id) return `#${item.policy_id}`
  return '—'
}

function uploaderLabel(item: AdminImageItem, guestLabel: string): string {
  if (item.user_id == null) return guestLabel
  return `${item.username || '—'}(#${item.user_id})`
}

function purgeToast(t: (k: string) => string, res?: AdminImageDeleteResult): string {
  if (!res?.permanent) return t('adminA.toastPurged')
  if (res.object_retained) return t('adminA.toastPurgedRetained')
  if (res.physical_queued) return t('adminA.toastPurgedQueued')
  // permanent but neither queued nor retained: runner missing or enqueue failed
  return t('adminA.toastPurgedNoQueue')
}

export function AdminImageDetail({ item, onClose }: { item: AdminImageItem | null; onClose(): void }) {
  const { t } = useT()
  const pushToast = useGlobal((s) => s.pushToast)
  const wl = useSetImageWhitelist()
  const delM = useDeleteAdminImage()
  const purgeM = usePurgeAdminImage()
  const restoreM = useRestoreAdminImage()
  const statusLabel = (status: string) => {
    if (status === 'normal') return t('adminA.statusNormal')
    if (status === 'pending') return t('adminA.statusPending')
    if (status === 'rejected') return t('adminA.statusRejected')
    return status
  }
  const isGuest = item?.user_id == null
  const inTrash = !!item?.in_trash
  // 游客无回收站；已在回收站可恢复或彻底删除
  const showSoftTrash = !isGuest && !inTrash
  const busy = delM.isPending || purgeM.isPending || restoreM.isPending || wl.isPending

  return (
    <Modal open={item !== null} onClose={onClose} width={560}>
      {item && (
        <div className="flex flex-col gap-3.5">
          <RetryImg
            className="max-h-80 w-full rounded-sm bg-soft object-contain"
            src={item.links.thumbnail_url}
            alt={item.name}
          />
          <div className="flex flex-col gap-2.5">
            <div className="text-sm font-bold break-all">{item.name}</div>
            <dl className="m-0 grid grid-cols-[72px_1fr] gap-x-3 gap-y-1 text-sm-plus">
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.uploader')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{uploaderLabel(item, t('adminA.guestUploader'))}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.size')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{formatBytes(item.size)}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.status')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">
                {statusLabel(item.status)}
                {item.is_whitelisted && t('adminA.whitelistedSuffix')}
                {inTrash && ` · ${t('adminA.trashBadge')}`}
              </dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">NSFW</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{item.nsfw_score == null ? '—' : String(item.nsfw_score)}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.storagePolicy')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{policyLabel(item)}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.storageDriver')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{item.policy_driver || '—'}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.storageSurface')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{item.surface || '—'}</dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.storagePath')}</dt>
              <dd className="m-0 flex items-start gap-2 font-mono text-[11.5px] break-all">
                <code className="flex-1 font-mono text-[11px] leading-snug break-all">{item.path || '—'}</code>
                {item.path ? (
                  <Button variant="secondary" onClick={() => copyText(item.path, t('adminA.storagePath'))}>
                    {t('adminA.copy')}
                  </Button>
                ) : null}
              </dd>
              <dt className="self-center font-mono text-2xs tracking-[0.08em] text-muted">{t('adminA.uploadedAt')}</dt>
              <dd className="m-0 font-mono text-[11.5px] break-all">{formatDate(item.created_at)}</dd>
            </dl>
            <div className="flex items-center gap-2 rounded-sm border border-border bg-soft py-1.5 pr-1.5 pl-2.5">
              <span className="min-w-0 flex-1 overflow-hidden font-mono text-xs-plus text-ellipsis whitespace-nowrap text-muted">
                {item.links.url}
              </span>
              <Button variant="secondary" onClick={() => copyText(item.links.url, t('adminA.urlLabel'))}>
                {t('adminA.copy')}
              </Button>
            </div>
            <p className="m-0 text-[11.5px] leading-[1.45] text-muted">{t('adminA.deleteHint')}</p>
            <div className="flex flex-wrap justify-end gap-2">
              {!inTrash && (
                <Button
                  variant="secondary"
                  disabled={busy}
                  onClick={() => wl.mutate({ key: item.key, on: !item.is_whitelisted }, { onSuccess: onClose })}
                >
                  {item.is_whitelisted ? t('adminA.unwhitelist') : t('adminA.whitelist')}
                </Button>
              )}
              {showSoftTrash && (
                <InlineConfirm
                  label={t('adminA.moveToTrash')}
                  confirmLabel={t('adminA.confirmMoveToTrash')}
                  disabled={busy}
                  onConfirm={() =>
                    delM.mutate(item.key, {
                      onSuccess: (res) => {
                        pushToast(res.permanent ? purgeToast(t, res) : t('adminA.toastMovedToTrash'))
                        onClose()
                      },
                    })
                  }
                />
              )}
              {inTrash && (
                <Button
                  variant="secondary"
                  disabled={busy}
                  onClick={() =>
                    restoreM.mutate(item.key, {
                      onSuccess: () => {
                        pushToast(t('adminA.toastRestored'))
                        onClose()
                      },
                    })
                  }
                >
                  {t('adminA.restoreFromTrash')}
                </Button>
              )}
              <InlineConfirm
                label={t('adminA.purgePermanent')}
                confirmLabel={t('adminA.confirmPurge')}
                disabled={busy}
                onConfirm={() =>
                  purgeM.mutate(item.key, {
                    onSuccess: (res) => {
                      pushToast(purgeToast(t, res))
                      onClose()
                    },
                  })
                }
              />
            </div>
          </div>
        </div>
      )}
    </Modal>
  )
}
