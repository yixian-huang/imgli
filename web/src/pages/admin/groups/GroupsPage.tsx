import { useState, useId, type ReactNode } from 'react'
import {
  useAdminGroups,
  useAdminPolicies,
  useApplyGroupLifecycle,
  useCreateGroup,
  useDeleteGroup,
  usePreviewGroupLifecycle,
  useUpdateGroup,
  type GroupWriteBody,
} from '../../../api/adminHooks'
import type { AdminGroup } from '../../../api/types'
import { useT } from '../../../i18n'
import { cn } from '../../../lib/cn'
import { PageHeader } from '../../../shell/PageHeader'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { EmptyState } from '../../../ui/EmptyState'
import { InlineConfirm } from '../../../ui/InlineConfirm'
import { Input } from '../../../ui/Input'
import { AdminField } from '../ui/adminChrome'
import { AdminQueryGate } from '../ui/AdminQueryGate'

const DAY = 86400
const GB = 1024 ** 3
const MB = 1024 ** 2
const toGB = (b: number) => String(+(b / GB).toFixed(2))
const toMB = (b: number) => String(+(b / MB).toFixed(2))

const hintClass = 'text-xs leading-snug text-muted'
const grid2Class = 'grid grid-cols-2 gap-3'
const grid3Class = 'grid grid-cols-3 gap-3'

/** 秒 → 表单「天」：整除用整数，否则保留最多 2 位小数。 */
export function secToDaysField(sec: number): string {
  if (!sec || sec <= 0) return '0'
  const d = sec / DAY
  if (Number.isInteger(d)) return String(d)
  return String(+(d.toFixed(2)))
}

/** 表单「天」→ 秒（API）；非法/负 → 0。 */
export function daysFieldToSec(raw: string): number {
  const n = Number(raw)
  if (!Number.isFinite(n) || n <= 0) return 0
  return Math.round(n * DAY)
}

/**
 * 列表徽章：max_expires 与 force 分开展示，不用 min 合并
 * （否则 max=30d + force=7d 会误显示 ≤7d 掩盖上限）。
 */
export function lifecycleBadges(
  g: AdminGroup,
  t: (k: string, v?: Record<string, string | number>) => string,
): string[] {
  const badges: string[] = []
  const maxSec = g.max_expires_in ?? 0
  const force = g.force_max_age_days ?? 0
  const ret = g.retention_days ?? 0
  if (maxSec > 0) {
    if (maxSec < DAY) {
      badges.push(t('adminA.lifecycleBadgeMaxH', { hours: Math.max(1, Math.ceil(maxSec / 3600)) }))
    } else {
      const days = maxSec % DAY === 0 ? maxSec / DAY : Math.ceil(maxSec / DAY)
      badges.push(t('adminA.lifecycleBadgeMax', { days }))
    }
  }
  if (force > 0) badges.push(t('adminA.lifecycleBadgeForce', { days: force }))
  if (ret > 0) badges.push(t('adminA.lifecycleBadgeRetention', { days: ret }))
  return badges
}

interface FormState {
  name: string
  quotaGB: string
  maxMB: string
  /** 月流量硬顶 GB；0=不限 */
  bwGB: string
  perMin: string
  perHour: string
  perDay: string
  exts: string[]
  policyIds: number[]
  /** 默认/上限有效期：天（写入 API 时 ×86400） */
  defaultExpiresDays: string
  maxExpiresDays: string
  defaultMaxViews: string
  maxMaxViews: string
  retentionDays: string
  forceMaxAgeDays: string
}

const NEW_FORM: FormState = {
  name: '', quotaGB: '10', maxMB: '20', bwGB: '5', perMin: '20', perHour: '200', perDay: '1000',
  exts: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'heic', 'heif'], policyIds: [],
  defaultExpiresDays: '0', maxExpiresDays: '0', defaultMaxViews: '0', maxMaxViews: '0',
  retentionDays: '0', forceMaxAgeDays: '0',
}

function formOf(g: AdminGroup): FormState {
  return {
    name: g.name,
    quotaGB: toGB(g.storage_quota),
    maxMB: toMB(g.max_file_size),
    bwGB: toGB(g.bandwidth_quota_month ?? 0),
    perMin: String(g.rate_per_minute),
    perHour: String(g.rate_per_hour),
    perDay: String(g.rate_per_day),
    exts: g.allowed_exts ?? [],
    policyIds: g.allowed_policy_ids ?? [],
    defaultExpiresDays: secToDaysField(g.default_expires_in ?? 0),
    maxExpiresDays: secToDaysField(g.max_expires_in ?? 0),
    defaultMaxViews: String(g.default_max_views ?? 0),
    maxMaxViews: String(g.max_max_views ?? 0),
    retentionDays: String(g.retention_days ?? 0),
    forceMaxAgeDays: String(g.force_max_age_days ?? 0),
  }
}

function createBody(f: FormState): GroupWriteBody {
  return {
    name: f.name.trim(),
    storage_quota: Math.round(Number(f.quotaGB) * GB),
    max_file_size: Math.round(Number(f.maxMB) * MB),
    bandwidth_quota_month: Math.round(Number(f.bwGB) * GB),
    rate_per_minute: Number(f.perMin),
    rate_per_hour: Number(f.perHour),
    rate_per_day: Number(f.perDay),
    allowed_exts: f.exts,
    allowed_policy_ids: f.policyIds,
    default_expires_in: daysFieldToSec(f.defaultExpiresDays),
    max_expires_in: daysFieldToSec(f.maxExpiresDays),
    default_max_views: Number(f.defaultMaxViews) || 0,
    max_max_views: Number(f.maxMaxViews) || 0,
    retention_days: Number(f.retentionDays) || 0,
    force_max_age_days: Number(f.forceMaxAgeDays) || 0,
  }
}

// 差异提交:只发相对已加载组变化的字段(数值比显示串,避开 bytes 往返舍入抖动)。
function patchBody(o: AdminGroup, f: FormState): GroupWriteBody {
  const b: GroupWriteBody = {}
  if (f.name.trim() !== o.name) b.name = f.name.trim()
  if (f.quotaGB !== toGB(o.storage_quota)) b.storage_quota = Math.round(Number(f.quotaGB) * GB)
  if (f.maxMB !== toMB(o.max_file_size)) b.max_file_size = Math.round(Number(f.maxMB) * MB)
  if (f.bwGB !== toGB(o.bandwidth_quota_month ?? 0)) b.bandwidth_quota_month = Math.round(Number(f.bwGB) * GB)
  if (Number(f.perMin) !== o.rate_per_minute) b.rate_per_minute = Number(f.perMin)
  if (Number(f.perHour) !== o.rate_per_hour) b.rate_per_hour = Number(f.perHour)
  if (Number(f.perDay) !== o.rate_per_day) b.rate_per_day = Number(f.perDay)
  if (JSON.stringify(f.exts) !== JSON.stringify(o.allowed_exts ?? [])) b.allowed_exts = f.exts
  if (JSON.stringify(f.policyIds) !== JSON.stringify(o.allowed_policy_ids ?? [])) b.allowed_policy_ids = f.policyIds
  const defExp = daysFieldToSec(f.defaultExpiresDays)
  const maxExp = daysFieldToSec(f.maxExpiresDays)
  if (defExp !== (o.default_expires_in ?? 0)) b.default_expires_in = defExp
  if (maxExp !== (o.max_expires_in ?? 0)) b.max_expires_in = maxExp
  if (Number(f.defaultMaxViews) !== (o.default_max_views ?? 0)) b.default_max_views = Number(f.defaultMaxViews) || 0
  if (Number(f.maxMaxViews) !== (o.max_max_views ?? 0)) b.max_max_views = Number(f.maxMaxViews) || 0
  if (Number(f.retentionDays) !== (o.retention_days ?? 0)) b.retention_days = Number(f.retentionDays) || 0
  if (Number(f.forceMaxAgeDays) !== (o.force_max_age_days ?? 0)) b.force_max_age_days = Number(f.forceMaxAgeDays) || 0
  return b
}

function FormSection({
  title,
  open = true,
  children,
}: {
  title: string
  open?: boolean
  children: ReactNode
}) {
  return (
    <details
      className="group overflow-hidden rounded-sm border border-border bg-bg"
      open={open}
    >
      <summary
        className={cn(
          'cursor-pointer select-none list-none px-3 py-2.5 text-sm-plus font-bold tracking-[0.02em] text-ink',
          'hover:bg-soft marker:content-none [&::-webkit-details-marker]:hidden',
          "before:font-normal before:text-muted before:content-['▸_'] open:before:content-['▾_']",
        )}
      >
        {title}
      </summary>
      <div className="flex flex-col gap-3 border-t border-border px-3 pb-3 pt-3">{children}</div>
    </details>
  )
}

function ExtInput({ exts, onChange }: { exts: string[]; onChange(v: string[]): void }) {
  const { t } = useT()
  const inputId = useId()
  const [draft, setDraft] = useState('')
  const add = () => {
    const v = draft.trim().toLowerCase().replace(/^\./, '')
    if (v && !exts.includes(v)) onChange([...exts, v])
    setDraft('')
  }
  return (
    <AdminField label={<label htmlFor={inputId}>{t('adminA.allowedExts')}</label>}>
      <div className="flex flex-wrap gap-1.5 rounded-sm border border-border p-1.5">
        {exts.map((e) => (
          <span key={e} className="inline-flex items-center gap-1 rounded-sm bg-soft px-2 py-0.5 text-[13px]">
            {e}
            <button
              type="button"
              className="cursor-pointer border-0 bg-transparent text-muted"
              aria-label={t('adminA.removeExtAria', { ext: e })}
              onClick={() => onChange(exts.filter((x) => x !== e))}
            >
              ×
            </button>
          </span>
        ))}
        <input
          id={inputId}
          className="min-w-20 flex-1 border-0 bg-transparent font-inherit text-inherit outline-none"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ',') {
              e.preventDefault()
              add()
            }
          }}
          onBlur={add}
          placeholder={t('adminA.extPlaceholder')}
        />
      </div>
    </AdminField>
  )
}

export function GroupsPage() {
  const { t } = useT()
  const pushToast = useGlobal((s) => s.pushToast)
  const groupsQ = useAdminGroups()
  const policiesQ = useAdminPolicies()
  const create = useCreateGroup()
  const update = useUpdateGroup()
  const del = useDeleteGroup()
  const lifePreview = usePreviewGroupLifecycle()
  const lifeApply = useApplyGroupLifecycle()

  const groups = groupsQ.data?.items ?? []
  const policies = policiesQ.data?.items ?? []
  const [sel, setSel] = useState<number | 'new' | null>(null)
  const [form, setForm] = useState<FormState>(NEW_FORM)
  const [lifeMsg, setLifeMsg] = useState<string | null>(null)

  const current = typeof sel === 'number' ? groups.find((g) => g.id === sel) ?? null : null
  const builtin = !!current && (current.is_default || current.is_guest)
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm((f) => ({ ...f, [k]: v }))

  const selectGroup = (g: AdminGroup) => {
    setSel(g.id)
    setForm(formOf(g))
    setLifeMsg(null)
  }
  const selectNew = () => {
    setSel('new')
    setForm(NEW_FORM)
    setLifeMsg(null)
  }

  const submit = () => {
    if (sel === 'new') {
      create.mutate(createBody(form), {
        onSuccess: () => {
          pushToast(t('adminA.toastGroupCreated'))
          setSel(null)
        },
      })
      return
    }
    if (!current) return
    const body = patchBody(current, form)
    if (Object.keys(body).length === 0) {
      pushToast(t('adminA.toastGroupNoChanges'))
      return
    }
    const id = current.id
    update.mutate(
      { id, body },
      {
        onSuccess: (g) => {
          pushToast(t('adminA.toastGroupSaved'))
          // 用返回体合并当前行，列表 invalidate 前徽章/表单先一致
          if (g) {
            setForm(formOf({ ...current, ...g, id } as AdminGroup))
          }
        },
      },
    )
  }

  const onLifecyclePreview = () => {
    if (!current) return
    setLifeMsg(null)
    lifePreview.mutate(current.id, {
      onSuccess: (p) => {
        if (p.cap_sec <= 0) setLifeMsg(t('adminA.lifecycleNoCap'))
        else {
          setLifeMsg(t('adminA.lifecyclePreviewResult', {
            perm: p.permanent_count,
            over: p.over_cap_count,
            total: p.total,
            cap: p.cap_sec,
          }))
        }
      },
    })
  }

  const onLifecycleApply = () => {
    if (!current) return
    if (!window.confirm(t('adminA.lifecycleApplyConfirm'))) return
    lifeApply.mutate(
      { id: current.id },
      {
        onSuccess: (r) => {
          const msg = t('adminA.lifecycleApplyResult', { updated: r.updated, skipped: r.skipped })
          setLifeMsg(msg)
          pushToast(msg)
        },
      },
    )
  }

  return (
    <div>
      <PageHeader
        kicker="USER GROUPS"
        title={t('adminA.groupsTitle')}
        extra={<Button variant="primary" onClick={selectNew}>{t('adminA.newGroup')}</Button>}
      />
      <AdminQueryGate query={groupsQ}>
        {() => (
          <div className="mt-2 grid grid-cols-1 gap-5 md:grid-cols-[260px_1fr]">
            <div className="flex flex-col gap-1">
              {groups.map((g) => (
                <button
                  key={g.id}
                  type="button"
                  className={cn(
                    'flex cursor-pointer flex-col items-stretch gap-1.5 rounded-sm border border-border bg-surface px-3 py-2.5 text-left',
                    sel === g.id && 'border-ink',
                  )}
                  onClick={() => selectGroup(g)}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="overflow-hidden text-ellipsis whitespace-nowrap font-semibold">{g.name}</span>
                    {(g.is_default || g.is_guest) && (
                      <span className="flex-none rounded bg-soft px-1.5 py-px text-[11px] text-muted">
                        {g.is_guest ? t('adminA.guest') : t('adminA.defaultGroup')}
                      </span>
                    )}
                    <span className="ml-auto flex-none text-xs text-muted">
                      {t('adminA.memberCount', { count: g.user_count })}
                    </span>
                  </span>
                  {lifecycleBadges(g, t).length > 0 && (
                    <span className="flex flex-wrap gap-1">
                      {lifecycleBadges(g, t).map((b) => (
                        <span
                          key={b}
                          className="whitespace-nowrap rounded-sm border border-border px-1.5 py-px font-mono text-2xs text-muted"
                        >
                          {b}
                        </span>
                      ))}
                    </span>
                  )}
                </button>
              ))}
            </div>
            <div className="rounded-sm border border-border bg-surface p-5">
              {sel === null ? (
                <EmptyState title={t('adminA.selectOrCreate')} desc={t('adminA.selectOrCreateDesc')} />
              ) : (
                <div className="flex flex-col gap-3.5">
                  <Input
                    label={t('adminA.groupName')}
                    value={form.name}
                    disabled={builtin}
                    extra={builtin ? <span className={hintClass}>{t('adminA.builtinNameLocked')}</span> : undefined}
                    onChange={(e) => set('name', e.target.value)}
                  />

                  <FormSection title={t('adminA.sectionQuota')} open>
                    <div className={grid2Class}>
                      <Input
                        label={t('adminA.quotaGB')}
                        type="number"
                        min={0}
                        value={form.quotaGB}
                        onChange={(e) => set('quotaGB', e.target.value)}
                      />
                      <Input
                        label={t('adminA.maxFileMB')}
                        type="number"
                        min={0}
                        value={form.maxMB}
                        onChange={(e) => set('maxMB', e.target.value)}
                      />
                    </div>
                    <Input
                      label={t('adminA.bandwidthQuotaGB')}
                      type="number"
                      min={0}
                      value={form.bwGB}
                      extra={<span className={hintClass}>{t('adminA.bandwidthQuotaHint')}</span>}
                      onChange={(e) => set('bwGB', e.target.value)}
                    />
                  </FormSection>

                  <FormSection title={t('adminA.sectionRate')} open>
                    <div className={grid3Class}>
                      <Input
                        label={t('adminA.ratePerMin')}
                        type="number"
                        min={0}
                        value={form.perMin}
                        onChange={(e) => set('perMin', e.target.value)}
                      />
                      <Input
                        label={t('adminA.ratePerHour')}
                        type="number"
                        min={0}
                        value={form.perHour}
                        onChange={(e) => set('perHour', e.target.value)}
                      />
                      <Input
                        label={t('adminA.ratePerDay')}
                        type="number"
                        min={0}
                        value={form.perDay}
                        onChange={(e) => set('perDay', e.target.value)}
                      />
                    </div>
                  </FormSection>

                  <FormSection title={t('adminA.sectionExts')} open={false}>
                    <ExtInput exts={form.exts} onChange={(v) => set('exts', v)} />
                  </FormSection>

                  <FormSection title={t('adminA.sectionLifecycle')} open>
                    <p className={hintClass}>{t('adminA.lifecycleHint')}</p>
                    <p className={hintClass}>{t('adminA.lifecycleSecHint')}</p>
                    <div className={grid2Class}>
                      <Input
                        label={t('adminA.defaultExpiresDays')}
                        type="number"
                        min={0}
                        step="any"
                        value={form.defaultExpiresDays}
                        onChange={(e) => set('defaultExpiresDays', e.target.value)}
                      />
                      <Input
                        label={t('adminA.maxExpiresDays')}
                        type="number"
                        min={0}
                        step="any"
                        value={form.maxExpiresDays}
                        onChange={(e) => set('maxExpiresDays', e.target.value)}
                      />
                    </div>
                    <div className={grid2Class}>
                      <Input
                        label={t('adminA.defaultMaxViews')}
                        type="number"
                        min={0}
                        value={form.defaultMaxViews}
                        onChange={(e) => set('defaultMaxViews', e.target.value)}
                      />
                      <Input
                        label={t('adminA.maxMaxViews')}
                        type="number"
                        min={0}
                        value={form.maxMaxViews}
                        onChange={(e) => set('maxMaxViews', e.target.value)}
                      />
                    </div>
                    <div className={grid2Class}>
                      <Input
                        label={t('adminA.retentionDays')}
                        type="number"
                        min={0}
                        value={form.retentionDays}
                        onChange={(e) => set('retentionDays', e.target.value)}
                      />
                      <Input
                        label={t('adminA.forceMaxAgeDays')}
                        type="number"
                        min={0}
                        value={form.forceMaxAgeDays}
                        onChange={(e) => set('forceMaxAgeDays', e.target.value)}
                      />
                    </div>
                  </FormSection>

                  <FormSection title={t('adminA.sectionPolicies')} open={false}>
                    <AdminField label={t('adminA.allowedPolicies')}>
                      <div className="flex flex-wrap gap-3">
                        {policiesQ.isError ? (
                          <span className={hintClass}>{t('adminA.policiesLoadFailed')}</span>
                        ) : policies.length === 0 ? (
                          <span className={hintClass}>{t('adminA.noPolicies')}</span>
                        ) : null}
                        {policies.map((p) => (
                          <label key={p.id} className="inline-flex items-center gap-1.5 text-sm">
                            <input
                              type="checkbox"
                              checked={form.policyIds.includes(p.id)}
                              onChange={(e) =>
                                set(
                                  'policyIds',
                                  e.target.checked
                                    ? [...form.policyIds, p.id]
                                    : form.policyIds.filter((x) => x !== p.id),
                                )
                              }
                            />
                            {p.name}
                          </label>
                        ))}
                      </div>
                    </AdminField>
                  </FormSection>

                  {current && (
                    <FormSection title={t('adminA.sectionStock')} open={false}>
                      <p className={hintClass}>{t('adminA.lifecycleStockNote')}</p>
                      <div className="flex flex-wrap items-center gap-2">
                        <Button
                          variant="secondary"
                          disabled={lifePreview.isPending || lifeApply.isPending}
                          onClick={onLifecyclePreview}
                        >
                          {t('adminA.lifecyclePreview')}
                        </Button>
                        <Button
                          variant="secondary"
                          disabled={lifePreview.isPending || lifeApply.isPending}
                          onClick={onLifecycleApply}
                        >
                          {t('adminA.lifecycleApply')}
                        </Button>
                      </div>
                      {lifeMsg && <p className={hintClass}>{lifeMsg}</p>}
                    </FormSection>
                  )}

                  <div className="mt-1 flex gap-2.5">
                    <Button
                      variant="primary"
                      disabled={create.isPending || update.isPending || del.isPending}
                      onClick={submit}
                    >
                      {t('common.save')}
                    </Button>
                    {current && !builtin && (
                      <InlineConfirm
                        label={t('common.delete')}
                        onConfirm={() => del.mutate(current.id, { onSuccess: () => setSel(null) })}
                      />
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </AdminQueryGate>
    </div>
  )
}
