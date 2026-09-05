import { useEffect, useMemo, useState } from 'react'
import {
  useAdminPolicies,
  useCancelStorageMigrate,
  useCreatePolicy,
  useDeletePolicy,
  useResumeStorageMigrate,
  useStartStorageMigrate,
  useStorageMigrateJob,
  useStorageMigrateJobs,
  useTestPolicy,
  useUpdatePolicy,
} from '../../../api/adminHooks'
import { Link } from 'react-router'
import type { AdminPolicy, StorageCaps } from '../../../api/types'
import { useT } from '../../../i18n'
import { cn } from '../../../lib/cn'
import { formatBytes } from '../../../lib/format'
import {
  PATH_TEMPLATE_PRESETS,
  previewObjectKey,
} from '../../../lib/pathTemplatePreview'
import { useGlobal } from '../../../store'
import { PageHeader } from '../../../shell/PageHeader'
import { Button } from '../../../ui/Button'
import { EmptyState } from '../../../ui/EmptyState'
import { InlineConfirm } from '../../../ui/InlineConfirm'
import { Input } from '../../../ui/Input'
import { Segmented } from '../../../ui/Segmented'
import { Toggle } from '../../../ui/Toggle'
import { AdminField, AdminSelect } from '../ui/adminChrome'
import { AdminQueryGate } from '../ui/AdminQueryGate'

type DriverKind = 'local' | 's3' | 'webdav' | 'ftp'

interface FormState {
  name: string
  driver: DriverKind
  root: string
  s3Endpoint: string
  s3Region: string
  s3Bucket: string
  s3AKID: string
  s3Secret: string
  s3PathStyle: 'true' | 'false'
  s3Prefix: string
  s3PresignDomain: string
  davEndpoint: string
  davUsername: string
  davPassword: string
  ftpHost: string
  ftpPort: string
  ftpUsername: string
  ftpPassword: string
  ftpPrefix: string
  ftpAllowInsecure: boolean
  cdn: string
  tpl: string
  enabled: boolean
}

const NEW_FORM: FormState = {
  name: '',
  driver: 'local',
  root: '',
  s3Endpoint: '',
  s3Region: '',
  s3Bucket: '',
  s3AKID: '',
  s3Secret: '',
  s3PathStyle: 'false',
  s3Prefix: '',
  s3PresignDomain: '',
  davEndpoint: '',
  davUsername: '',
  davPassword: '',
  ftpHost: '',
  ftpPort: '21',
  ftpUsername: '',
  ftpPassword: '',
  ftpPrefix: '',
  ftpAllowInsecure: false,
  cdn: '',
  tpl: '{Y}/{m}/{d}/{uniqid}.{ext}',
  enabled: true,
}

const hintClass = 'text-xs leading-snug text-muted'
const labelClass = 'text-[13px] text-muted'
const tierBadgeClass =
  'rounded bg-[color-mix(in_srgb,var(--warn)_25%,var(--soft))] px-1.5 py-px text-[11px] font-medium text-ink'
const ackRowClass = 'flex items-start gap-2.5 text-[13px] leading-snug'
const warnInlineClass = 'm-0 text-xs leading-snug text-err'
const toggleRowClass = 'flex items-center gap-3'
const presetChipClass =
  'cursor-pointer rounded border border-border bg-card px-2 py-0.5 text-[11px] text-muted hover:border-ink/30 hover:text-ink'

function pathStyleLikelyUnsupported(endpoint: string): boolean {
  const ep = endpoint.trim().toLowerCase().replace(/^https?:\/\//, '')
  return (
    ep.includes('aliyuncs.com') ||
    ep.includes('myqcloud.com') ||
    ep.includes('amazonaws.com') ||
    ep.includes('r2.cloudflarestorage.com') ||
    ep.includes('qiniucs.com')
  )
}

function presetLabel(t: (k: string) => string, id: string): string {
  if (id === 'flat') return t('adminB.pathTplPresetFlat')
  if (id === 'toSecond') return t('adminB.pathTplPresetToSecond')
  if (id === 'digits') return t('adminB.pathTplPresetDigits')
  return t('adminB.pathTplPresetDefault')
}

/** Example fills only; operators still enter real bucket/keys. */
const S3_VENDOR_EXAMPLES: {
  id: string
  endpoint: string
  region: string
  pathStyle: 'true' | 'false'
}[] = [
  { id: 'oss', endpoint: 'oss-cn-hangzhou.aliyuncs.com', region: 'cn-hangzhou', pathStyle: 'false' },
  { id: 'cos', endpoint: 'cos.ap-guangzhou.myqcloud.com', region: 'ap-guangzhou', pathStyle: 'false' },
  { id: 'r2', endpoint: 'https://<ACCOUNT_ID>.r2.cloudflarestorage.com', region: 'auto', pathStyle: 'false' },
  { id: 'minio', endpoint: 'http://127.0.0.1:9000', region: 'us-east-1', pathStyle: 'true' },
]

function vendorExampleLabel(t: (k: string) => string, id: string): string {
  if (id === 'cos') return t('adminB.s3VendorCos')
  if (id === 'r2') return t('adminB.s3VendorR2')
  if (id === 'minio') return t('adminB.s3VendorMinio')
  return t('adminB.s3VendorOss')
}

/** Apply server-normalized config (e.g. prefix trailing /) back into the form. */
function applyPolicyToForm(p: AdminPolicy, setForm: (f: FormState) => void) {
  setForm(formOf(p))
}

function formOf(p: AdminPolicy): FormState {
  let root = ''
  let s3Endpoint = ''
  let s3Region = ''
  let s3Bucket = ''
  let s3AKID = ''
  let s3Secret = ''
  let s3PathStyle: 'true' | 'false' = 'false'
  let s3Prefix = ''
  let s3PresignDomain = ''
  let davEndpoint = ''
  let davUsername = ''
  let davPassword = ''
  let ftpHost = ''
  let ftpPort = '21'
  let ftpUsername = ''
  let ftpPassword = ''
  let ftpPrefix = ''
  let ftpAllowInsecure = false
  try {
    const cfg = JSON.parse(p.config) as Record<string, string>
    if (p.driver === 's3') {
      s3Endpoint = cfg.endpoint ?? ''
      s3Region = cfg.region ?? ''
      s3Bucket = cfg.bucket ?? ''
      s3AKID = cfg.access_key_id ?? ''
      s3Secret = cfg.secret_access_key ?? ''
      s3PathStyle = cfg.path_style === 'true' ? 'true' : 'false'
      s3Prefix = cfg.prefix ?? ''
      s3PresignDomain = cfg.presign_domain ?? ''
    } else if (p.driver === 'webdav') {
      davEndpoint = cfg.endpoint ?? ''
      davUsername = cfg.username ?? ''
      davPassword = cfg.password ?? ''
    } else if (p.driver === 'ftp') {
      ftpHost = cfg.host ?? cfg.endpoint ?? ''
      ftpPort = cfg.port ?? '21'
      ftpUsername = cfg.username ?? ''
      ftpPassword = cfg.password ?? ''
      ftpPrefix = cfg.prefix ?? cfg.root ?? ''
      ftpAllowInsecure = cfg.allow_insecure === 'true'
    } else {
      root = cfg.root ?? ''
    }
  } catch {
    /* ignore bad config */
  }
  const driver: DriverKind =
    p.driver === 's3' ? 's3' : p.driver === 'webdav' ? 'webdav' : p.driver === 'ftp' ? 'ftp' : 'local'
  return {
    name: p.name,
    driver,
    root,
    s3Endpoint,
    s3Region,
    s3Bucket,
    s3AKID,
    s3Secret,
    s3PathStyle,
    s3Prefix,
    s3PresignDomain,
    davEndpoint,
    davUsername,
    davPassword,
    ftpHost,
    ftpPort,
    ftpUsername,
    ftpPassword,
    ftpPrefix,
    ftpAllowInsecure,
    cdn: p.cdn_domain ?? '',
    tpl: p.path_template || NEW_FORM.tpl,
    enabled: p.enabled !== false,
  }
}

function buildConfig(form: FormState): string {
  if (form.driver === 's3') {
    return JSON.stringify({
      endpoint: form.s3Endpoint.trim(),
      region: form.s3Region.trim(),
      bucket: form.s3Bucket.trim(),
      access_key_id: form.s3AKID.trim(),
      secret_access_key: form.s3Secret,
      path_style: form.s3PathStyle,
      prefix: form.s3Prefix.trim(),
      presign_domain: form.s3PresignDomain.trim(),
    })
  }
  if (form.driver === 'webdav') {
    return JSON.stringify({
      endpoint: form.davEndpoint.trim(),
      username: form.davUsername.trim(),
      password: form.davPassword,
    })
  }
  if (form.driver === 'ftp') {
    return JSON.stringify({
      host: form.ftpHost.trim(),
      port: form.ftpPort.trim() || '21',
      username: form.ftpUsername.trim(),
      password: form.ftpPassword,
      prefix: form.ftpPrefix.trim(),
      allow_insecure: form.ftpAllowInsecure ? 'true' : 'false',
    })
  }
  return JSON.stringify({ root: form.root.trim() })
}

/** Client-side caps mirror for unsaved driver switch (backend remains authoritative on load). */
function staticCaps(driver: DriverKind): StorageCaps {
  const base = {
    transport_tls_preferred: true,
    allows_insecure: false,
    range_get: true,
    list_prefix: false,
    multipart_upload: false,
    feature_loss_keys: [] as string[],
  }
  if (driver === 's3') {
    return {
      ...base,
      tier: 'first_class',
      summary_key: 'storage.caps.summary.s3',
      public_cdn_offload_recommended: true,
      private_presign_capable: true,
      hot_path_ok: true,
      feature_loss_keys: null,
    }
  }
  if (driver === 'webdav') {
    return {
      ...base,
      tier: 'supported',
      summary_key: 'storage.caps.summary.webdav',
      public_cdn_offload_recommended: false,
      private_presign_capable: false,
      hot_path_ok: true,
      feature_loss_keys: [
        'storage.loss.no_presign',
        'storage.loss.cdn_not_typical',
        'storage.loss.vendor_semantics',
      ],
    }
  }
  if (driver === 'ftp') {
    return {
      ...base,
      tier: 'compat',
      summary_key: 'storage.caps.summary.ftp',
      allows_insecure: true,
      range_get: false,
      public_cdn_offload_recommended: false,
      private_presign_capable: false,
      hot_path_ok: false,
      feature_loss_keys: [
        'storage.loss.no_presign',
        'storage.loss.no_cdn_offload',
        'storage.loss.cdn_not_typical',
        'storage.loss.hot_path',
        'storage.loss.ftp_security',
        'storage.loss.ftp_reliability',
      ],
    }
  }
  return {
    ...base,
    tier: 'first_class',
    summary_key: 'storage.caps.summary.local',
    public_cdn_offload_recommended: false,
    private_presign_capable: false,
    hot_path_ok: true,
    feature_loss_keys: ['storage.loss.no_presign', 'storage.loss.cdn_not_typical'],
  }
}

function tierLabel(t: (k: string) => string, tier?: string): string {
  if (tier === 'compat') return t('adminB.tierCompat')
  if (tier === 'supported') return t('adminB.tierSupported')
  if (tier === 'migrate_only') return t('adminB.tierMigrateOnly')
  return t('adminB.tierFirstClass')
}

function PolicyCapsPanel({
  caps,
  warnings,
}: {
  caps: StorageCaps
  warnings?: AdminPolicy['warnings']
}) {
  const { t } = useT()
  const yn = (v: boolean) => (v ? t('adminB.capsYes') : t('adminB.capsNo'))
  const summary = t(caps.summary_key)
  const losses = caps.feature_loss_keys ?? []
  return (
    <div
      className={cn(
        'flex flex-col gap-2 rounded-sm border border-border bg-soft px-3.5 py-3',
        caps.tier === 'compat' && 'border-err/45',
      )}
      data-tier={caps.tier}
    >
      <div className="flex items-center gap-2 text-[13px] font-semibold">
        {t('adminB.capsTitle')}
        <span className={tierBadgeClass}>{tierLabel(t, caps.tier)}</span>
      </div>
      <p className="m-0 text-[13px] leading-snug text-muted">{summary}</p>
      <ul className="m-0 pl-[1.1em] text-xs text-ink">
        <li>
          {t('adminB.capsPublicCdn')}: {yn(caps.public_cdn_offload_recommended)}
        </li>
        <li>
          {t('adminB.capsPrivatePresign')}: {yn(caps.private_presign_capable)}
        </li>
        <li>
          {t('adminB.capsHotPath')}: {yn(caps.hot_path_ok)}
        </li>
        <li>
          {t('adminB.capsTlsPreferred')}: {yn(caps.transport_tls_preferred)}
        </li>
      </ul>
      {losses.length > 0 && (
        <div className="[&_ul]:m-1 [&_ul]:mt-1 [&_ul]:pl-[1.1em] [&_ul]:text-xs [&_ul]:text-muted">
          <div className={labelClass}>{t('adminB.capsLimitations')}</div>
          <ul>
            {losses.map((k) => (
              <li key={k}>{t(k)}</li>
            ))}
          </ul>
        </div>
      )}
      {warnings && warnings.length > 0 && (
        <ul className="m-0 pl-[1.1em] text-xs text-err">
          {warnings.map((w) => (
            <li key={w.code} data-severity={w.severity}>
              {t(w.message_key)}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function PoliciesPage() {
  const { t } = useT()
  const policiesQ = useAdminPolicies()
  const create = useCreatePolicy()
  const update = useUpdatePolicy()
  const del = useDeletePolicy()
  const test = useTestPolicy()
  const startMigrate = useStartStorageMigrate()
  const cancelMigrate = useCancelStorageMigrate()
  const resumeMigrate = useResumeStorageMigrate()
  const migListQ = useStorageMigrateJobs()

  const policies = policiesQ.data?.items ?? []
  const [sel, setSel] = useState<number | 'new' | null>(null)
  const [form, setForm] = useState<FormState>(NEW_FORM)
  const [testMsg, setTestMsg] = useState<string | null>(null)
  const [testOk, setTestOk] = useState(false)
  const [compatAck, setCompatAck] = useState(false)
  const [migFrom, setMigFrom] = useState<number | ''>('')
  const [migTo, setMigTo] = useState<number | ''>('')
  const [migDry, setMigDry] = useState(true)
  const [migDel, setMigDel] = useState(false)
  const [migLimit, setMigLimit] = useState('0')
  const [migJobId, setMigJobId] = useState<string | null>(null)
  const migJobQ = useStorageMigrateJob(migJobId)

  useEffect(() => {
    if (migJobId) return
    const items = migListQ.data?.items
    if (!items?.length) return
    const live = items.find((j) => j.status === 'pending' || j.status === 'running' || j.status === 'failed')
    setMigJobId((live ?? items[0]).id)
  }, [migListQ.data, migJobId])
  const policyOptions = useMemo(
    () => policies.map((p) => ({ id: p.id, label: `${p.name} (#${p.id})${p.enabled ? '' : ' · off'}` })),
    [policies],
  )

  const current = typeof sel === 'number' ? policies.find((p) => p.id === sel) ?? null : null
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm((f) => ({ ...f, [k]: v }))

  const selectPolicy = (p: AdminPolicy) => {
    setSel(p.id)
    setForm(formOf(p))
    setTestMsg(null)
    setTestOk(false)
    setCompatAck(false)
  }
  const selectNew = () => {
    setSel('new')
    setForm(NEW_FORM)
    setTestMsg(null)
    setTestOk(false)
    setCompatAck(false)
  }

  const caps = current?.caps && current.driver === form.driver ? current.caps : staticCaps(form.driver)
  const clientWarnings = useMemo(() => {
    const w: NonNullable<AdminPolicy['warnings']> = []
    if ((form.cdn ?? '').trim() && !caps.public_cdn_offload_recommended) {
      w.push({ code: 'cdn_not_recommended', message_key: 'adminB.warnCdnWithoutCap', severity: 'warning' })
    }
    if (form.driver === 'ftp' && form.ftpAllowInsecure) {
      w.push({ code: 'insecure_transport', message_key: 'adminB.warnInsecureTransport', severity: 'warning' })
    }
    if (
      form.driver === 's3' &&
      form.s3PathStyle === 'true' &&
      pathStyleLikelyUnsupported(form.s3Endpoint)
    ) {
      w.push({ code: 'path_style_vendor', message_key: 'adminB.warnPathStyleVendor', severity: 'warning' })
    }
    return w
  }, [
    form.cdn,
    form.driver,
    form.ftpAllowInsecure,
    form.s3Endpoint,
    form.s3PathStyle,
    caps.public_cdn_offload_recommended,
  ])
  const warnings =
    current?.warnings && current.driver === form.driver && sel !== 'new'
      ? (() => {
          // Merge client-side path_style warning for unsaved form edits.
          const codes = new Set(current.warnings.map((x) => x.code))
          const extra = clientWarnings.filter((x) => !codes.has(x.code))
          return [...current.warnings, ...extra]
        })()
      : clientWarnings.length
        ? clientWarnings
        : undefined

  const objectKeyPreview = useMemo(
    () =>
      previewObjectKey({
        prefix: form.driver === 's3' ? form.s3Prefix : form.driver === 'ftp' ? form.ftpPrefix : '',
        template: form.tpl,
        surface: 'public',
        ext: 'png',
      }),
    [form.driver, form.s3Prefix, form.ftpPrefix, form.tpl],
  )

  const config = buildConfig(form)

  const needsCompatAck =
    form.driver === 'ftp' && form.enabled && (sel === 'new' || Boolean(current && !current.enabled))

  const submit = () => {
    setTestMsg(null)
    setTestOk(false)
    if (needsCompatAck && !compatAck) return
    if (sel === 'new') {
      create.mutate(
        {
          name: form.name.trim(),
          driver: form.driver,
          config,
          cdn_domain: form.cdn,
          path_template: form.tpl,
          enabled: form.enabled,
        },
        {
          onSuccess: (p) => {
            // Prefer server DTO so normalized prefix/path_template appear; incomplete mocks clear selection.
            if (p?.id && typeof p.config === 'string') {
              setSel(p.id)
              applyPolicyToForm(p, setForm)
            } else {
              setSel(null)
            }
            setCompatAck(false)
          },
        },
      )
    } else if (current) {
      update.mutate(
        {
          id: current.id,
          body: {
            name: form.name.trim(),
            config,
            cdn_domain: form.cdn,
            path_template: form.tpl,
            enabled: form.enabled,
          },
        },
        {
          onSuccess: (p) => {
            if (p && typeof p.config === 'string') {
              applyPolicyToForm(p, setForm)
            }
          },
        },
      )
    }
  }

  const runTest = () => {
    if (!current) return
    setTestMsg(null)
    setTestOk(false)
    test.mutate(current.id, {
      onSuccess: (r) => {
        setTestOk(true)
        setTestMsg(t('adminB.connectedMs', { ms: r.latency_ms }))
      },
      onError: (e) => {
        // toastApiError already fired from hook; also pin failure text under the button.
        const msg =
          e && typeof e === 'object' && 'message' in e && typeof (e as { message: unknown }).message === 'string'
            ? (e as { message: string }).message
            : t('adminB.testFailed')
        setTestOk(false)
        setTestMsg(msg)
      },
    })
  }

  const applyVendorExample = (id: string) => {
    const ex = S3_VENDOR_EXAMPLES.find((x) => x.id === id)
    if (!ex) return
    setForm((f) => ({
      ...f,
      s3Endpoint: ex.endpoint,
      s3Region: ex.region,
      s3PathStyle: ex.pathStyle,
    }))
  }

  const driverLabel =
    form.driver === 's3'
      ? 'S3'
      : form.driver === 'webdav'
        ? 'WebDAV'
        : form.driver === 'ftp'
          ? t('adminB.driverFtp')
          : t('adminB.driverLocal')

  const runMigrate = () => {
    const toast = (msg: string) => useGlobal.getState().pushToast(msg)
    if (policyOptions.length < 2) {
      toast(t('adminB.migrateNeedTwo'))
      return
    }
    if (migFrom === '' || migTo === '' || migFrom === migTo) {
      toast(t('adminB.migratePickDifferent'))
      return
    }
    const limit = Number.parseInt(migLimit, 10)
    startMigrate.mutate(
      {
        from_policy_id: migFrom,
        to_policy_id: migTo,
        dry_run: migDry,
        delete_source: migDel && !migDry,
        limit: Number.isFinite(limit) && limit > 0 ? limit : 0,
      },
      { onSuccess: (j) => setMigJobId(j.id) },
    )
  }

  return (
    <div>
      <PageHeader
        kicker="STORAGE POLICY"
        title={t('adminB.policiesTitle')}
        extra={
          <Button variant="primary" onClick={selectNew}>
            {t('adminB.newPolicy')}
          </Button>
        }
      />
      <AdminQueryGate query={policiesQ}>
        {() => (
          <>
          <div className="mb-5 rounded-sm border border-border bg-soft px-[1.1rem] py-4">
            <div className="mb-1.5 font-semibold">{t('adminB.migrateTitle')}</div>
            <p className={hintClass}>{t('adminB.migrateDesc')}</p>
            <div className="my-3 grid grid-cols-[repeat(auto-fit,minmax(12rem,1fr))] gap-3">
              <AdminField label={t('adminB.migrateFrom')}>
                <AdminSelect
                  className="h-auto w-full py-[0.45rem]"
                  value={migFrom === '' ? '' : String(migFrom)}
                  onChange={(e) => setMigFrom(e.target.value ? Number(e.target.value) : '')}
                >
                  <option value="">—</option>
                  {policyOptions.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label}
                    </option>
                  ))}
                </AdminSelect>
              </AdminField>
              <AdminField label={t('adminB.migrateTo')}>
                <AdminSelect
                  className="h-auto w-full py-[0.45rem]"
                  value={migTo === '' ? '' : String(migTo)}
                  onChange={(e) => setMigTo(e.target.value ? Number(e.target.value) : '')}
                >
                  <option value="">—</option>
                  {policyOptions.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label}
                    </option>
                  ))}
                </AdminSelect>
              </AdminField>
              <Input
                label={t('adminB.migrateLimit')}
                value={migLimit}
                onChange={(e) => setMigLimit(e.target.value)}
              />
            </div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-3.5">
              <label className={ackRowClass}>
                <input
                  type="checkbox"
                  className="mt-[3px]"
                  checked={migDry}
                  onChange={(e) => setMigDry(e.target.checked)}
                />
                <span>{t('adminB.migrateDryRun')}</span>
              </label>
              <label className={ackRowClass}>
                <input
                  type="checkbox"
                  className="mt-[3px]"
                  checked={migDel}
                  disabled={migDry}
                  onChange={(e) => setMigDel(e.target.checked)}
                />
                <span>{t('adminB.migrateDeleteSource')}</span>
              </label>
              <Button
                variant="secondary"
                disabled={
                  startMigrate.isPending ||
                  migJobQ.data?.status === 'pending' ||
                  migJobQ.data?.status === 'running'
                }
                onClick={runMigrate}
              >
                {t('adminB.migrateStart')}
              </Button>
              {(migJobQ.data?.status === 'pending' || migJobQ.data?.status === 'running') && (
                <Button
                  variant="secondary"
                  disabled={cancelMigrate.isPending}
                  onClick={() => migJobId && cancelMigrate.mutate(migJobId)}
                >
                  {t('adminB.migrateCancel')}
                </Button>
              )}
              {migJobQ.data?.status === 'failed' && (
                <Button
                  variant="secondary"
                  disabled={resumeMigrate.isPending}
                  onClick={() => migJobId && resumeMigrate.mutate(migJobId)}
                >
                  {t('adminB.migrateResume')}
                </Button>
              )}
            </div>
            {migJobQ.data && (
              <div className="mt-3 text-[0.9rem] opacity-90">
                <div>{t('adminB.migrateStatus', { status: migJobQ.data.status })}</div>
                <div>
                  {t('adminB.migrateProgress', {
                    scanned: migJobQ.data.progress.scanned,
                    copied: migJobQ.data.progress.copied,
                    skipped: migJobQ.data.progress.skipped,
                    failed: migJobQ.data.progress.failed,
                  })}
                </div>
                {migJobQ.data.error && <p className={warnInlineClass}>{migJobQ.data.error}</p>}
              </div>
            )}
          </div>
          <div className="mt-2 grid grid-cols-1 gap-5 md:grid-cols-[260px_1fr]">
            <div className="flex flex-col gap-1">
              {policies.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className={cn(
                    'flex cursor-pointer items-center gap-2 rounded-sm border border-border bg-surface px-3 py-2.5 text-left',
                    sel === p.id && 'border-ink',
                  )}
                  onClick={() => selectPolicy(p)}
                >
                  <span className="font-semibold">{p.name}</span>
                  {p.tier === 'compat' && <span className={tierBadgeClass}>{t('adminB.tierCompat')}</span>}
                  {!p.enabled && (
                    <span className="rounded bg-soft px-1.5 py-px text-[11px] text-err">{t('adminB.disabled')}</span>
                  )}
                  <span className="ml-auto text-xs text-muted">
                    {t('adminB.fileCount', { count: p.file_count })}
                    {p.used_bytes > 0 ? ` · ${formatBytes(p.used_bytes)}` : ''}
                  </span>
                </button>
              ))}
            </div>
            <div className="rounded-sm border border-border bg-surface p-5">
              {sel === null ? (
                <EmptyState title={t('adminB.selectOrCreatePolicy')} desc={t('adminB.selectOrCreatePolicyDesc')} />
              ) : (
                <div className="flex flex-col gap-3.5">
                  {typeof sel === 'number' && (
                    <AdminField>
                      <span className={hintClass}>{t('adminB.policyStatsHint')}</span>
                      <span className={hintClass}>
                        {t('adminB.fileCount', { count: policies.find((x) => x.id === sel)?.file_count ?? 0 })}
                        {' · '}
                        {formatBytes(policies.find((x) => x.id === sel)?.used_bytes ?? 0)}
                        {' · '}
                        {t('adminB.policyImageSplit', {
                          live: policies.find((x) => x.id === sel)?.live_image_count ?? 0,
                          trash: policies.find((x) => x.id === sel)?.trash_image_count ?? 0,
                        })}
                      </span>
                      <Link className={hintClass} to={`/admin/images?policy=${sel}`}>
                        {t('adminB.viewPolicyImages')}
                      </Link>
                    </AdminField>
                  )}
                  <Input label={t('adminB.name')} value={form.name} onChange={(e) => set('name', e.target.value)} />
                  <AdminField label={t('adminB.driver')}>
                    {sel === 'new' ? (
                      <Segmented<DriverKind>
                        options={[
                          { value: 'local', label: t('adminB.driverLocal') },
                          { value: 's3', label: 'S3' },
                          { value: 'webdav', label: 'WebDAV' },
                          { value: 'ftp', label: t('adminB.driverFtp') },
                        ]}
                        value={form.driver}
                        onChange={(v) => {
                          set('driver', v)
                          setCompatAck(false)
                        }}
                      />
                    ) : (
                      <div className="driver rounded-sm border border-border bg-soft px-3 py-2.5 text-muted">{driverLabel}</div>
                    )}
                  </AdminField>
                  <PolicyCapsPanel caps={caps} warnings={warnings} />
                  {form.driver === 'ftp' && <p className={hintClass}>{t('storage.help.ftpPreferProxy')}</p>}
                  {form.driver === 'local' ? (
                    <Input
                      label={t('adminB.storagePath')}
                      value={form.root}
                      placeholder="/data/uploads"
                      onChange={(e) => set('root', e.target.value)}
                    />
                  ) : form.driver === 'webdav' ? (
                    <>
                      <Input
                        label="Endpoint"
                        value={form.davEndpoint}
                        placeholder="https://dav.example.com/imgli"
                        onChange={(e) => set('davEndpoint', e.target.value)}
                      />
                      <Input
                        label={t('adminB.username')}
                        value={form.davUsername}
                        placeholder={t('adminB.davUserPlaceholder')}
                        onChange={(e) => set('davUsername', e.target.value)}
                      />
                      <Input
                        label={t('adminB.password')}
                        value={form.davPassword}
                        extra={<span className={hintClass}>{t('adminB.secretMaskHint')}</span>}
                        onChange={(e) => set('davPassword', e.target.value)}
                        onFocus={(e) => e.target.select()}
                      />
                    </>
                  ) : form.driver === 'ftp' ? (
                    <>
                      <Input
                        label={t('adminB.ftpHost')}
                        value={form.ftpHost}
                        placeholder="ftp.example.com"
                        onChange={(e) => set('ftpHost', e.target.value)}
                      />
                      <Input label={t('adminB.ftpPort')} value={form.ftpPort} onChange={(e) => set('ftpPort', e.target.value)} />
                      <Input
                        label={t('adminB.username')}
                        value={form.ftpUsername}
                        onChange={(e) => set('ftpUsername', e.target.value)}
                      />
                      <Input
                        label={t('adminB.password')}
                        value={form.ftpPassword}
                        extra={<span className={hintClass}>{t('adminB.secretMaskHint')}</span>}
                        onChange={(e) => set('ftpPassword', e.target.value)}
                        onFocus={(e) => e.target.select()}
                      />
                      <Input
                        label={t('adminB.ftpPrefix')}
                        value={form.ftpPrefix}
                        placeholder="imgli"
                        onChange={(e) => set('ftpPrefix', e.target.value)}
                      />
                      <div className={toggleRowClass}>
                        <span className={labelClass}>{t('adminB.ftpAllowInsecure')}</span>
                        <Toggle checked={form.ftpAllowInsecure} onChange={(v) => set('ftpAllowInsecure', v)} />
                      </div>
                      <span className={hintClass}>{t('adminB.ftpAllowInsecureHint')}</span>
                    </>
                  ) : (
                    <>
                      <Input
                        label="Endpoint"
                        value={form.s3Endpoint}
                        placeholder="oss-cn-hangzhou.aliyuncs.com"
                        extra={<span className={hintClass}>{t('adminB.s3EndpointHint')}</span>}
                        onChange={(e) => set('s3Endpoint', e.target.value)}
                      />
                      <div className="flex flex-col gap-1">
                        <span className={labelClass}>{t('adminB.s3VendorExamples')}</span>
                        <div className="flex flex-wrap gap-1.5">
                          {S3_VENDOR_EXAMPLES.map((v) => (
                            <button
                              key={v.id}
                              type="button"
                              className={presetChipClass}
                              onClick={() => applyVendorExample(v.id)}
                            >
                              {vendorExampleLabel(t, v.id)}
                            </button>
                          ))}
                        </div>
                        <span className={hintClass}>{t('adminB.s3VendorExamplesHint')}</span>
                      </div>
                      <Input
                        label="Region"
                        value={form.s3Region}
                        placeholder="cn-hangzhou"
                        extra={<span className={hintClass}>{t('adminB.s3RegionHint')}</span>}
                        onChange={(e) => set('s3Region', e.target.value)}
                      />
                      <Input label="Bucket" value={form.s3Bucket} onChange={(e) => set('s3Bucket', e.target.value)} />
                      <Input label="AccessKey ID" value={form.s3AKID} onChange={(e) => set('s3AKID', e.target.value)} />
                      <Input
                        label="AccessKey Secret"
                        value={form.s3Secret}
                        extra={<span className={hintClass}>{t('adminB.secretMaskHint')}</span>}
                        onChange={(e) => set('s3Secret', e.target.value)}
                        onFocus={(e) => e.target.select()}
                      />
                      <AdminField label={t('adminB.pathStyle')}>
                        <Segmented<'true' | 'false'>
                          options={[
                            { value: 'false', label: t('adminB.pathStyleVirtual') },
                            { value: 'true', label: t('adminB.pathStylePath') },
                          ]}
                          value={form.s3PathStyle}
                          onChange={(v) => set('s3PathStyle', v)}
                        />
                        <span className={hintClass}>{t('adminB.pathStyleHint')}</span>
                      </AdminField>
                      <Input
                        label={t('adminB.prefix')}
                        value={form.s3Prefix}
                        placeholder="upload/"
                        extra={<span className={hintClass}>{t('adminB.prefixHint')}</span>}
                        onChange={(e) => set('s3Prefix', e.target.value)}
                      />
                      <Input
                        label={t('adminB.presignDomain')}
                        value={form.s3PresignDomain}
                        placeholder="https://s3.img.li"
                        extra={<span className={hintClass}>{t('adminB.presignDomainHint')}</span>}
                        onChange={(e) => set('s3PresignDomain', e.target.value)}
                      />
                    </>
                  )}
                  <Input
                    label={t('adminB.cdnDomain')}
                    value={form.cdn}
                    placeholder={t('adminB.cdnDomainPlaceholder')}
                    extra={<span className={hintClass}>{t('adminB.cdnDomainHint')}</span>}
                    onChange={(e) => set('cdn', e.target.value)}
                  />
                  {!caps.public_cdn_offload_recommended && form.cdn.trim() && (
                    <p className={warnInlineClass}>{t('adminB.warnCdnWithoutCap')}</p>
                  )}
                  <Input
                    label={t('adminB.pathTemplate')}
                    value={form.tpl}
                    extra={<span className={hintClass}>{t('adminB.pathTemplateHint')}</span>}
                    onChange={(e) => set('tpl', e.target.value)}
                  />
                  <div className="flex flex-col gap-1.5">
                    <span className={labelClass}>{t('adminB.pathTemplatePresets')}</span>
                    <div className="flex flex-wrap gap-1.5">
                      {PATH_TEMPLATE_PRESETS.map((p) => (
                        <button
                          key={p.id}
                          type="button"
                          className={presetChipClass}
                          onClick={() => set('tpl', p.template)}
                        >
                          {presetLabel(t, p.id)}
                        </button>
                      ))}
                    </div>
                    <div className="rounded-sm border border-border bg-soft px-2.5 py-1.5 font-mono text-[11px] leading-snug text-muted break-all">
                      <span className="mr-1.5 font-sans text-muted/80">{t('adminB.pathTemplatePreview')}:</span>
                      {objectKeyPreview}
                    </div>
                  </div>
                  <div className={toggleRowClass}>
                    <span className={labelClass}>{t('adminB.enabled')}</span>
                    <Toggle checked={form.enabled} onChange={(v) => set('enabled', v)} />
                  </div>
                  {needsCompatAck && (
                    <label className={ackRowClass}>
                      <input
                        type="checkbox"
                        className="mt-[3px]"
                        checked={compatAck}
                        onChange={(e) => setCompatAck(e.target.checked)}
                      />
                      <span>
                        <strong>{t('adminB.confirmCompatEnableTitle')}</strong> — {t('adminB.confirmCompatEnableBody')} (
                        {t('adminB.confirmCompatEnableAck')})
                      </span>
                    </label>
                  )}
                  <div className="mt-1 flex items-center gap-2.5">
                    <Button
                      variant="primary"
                      disabled={
                        create.isPending ||
                        update.isPending ||
                        del.isPending ||
                        (needsCompatAck && !compatAck)
                      }
                      onClick={submit}
                    >
                      {t('common.save')}
                    </Button>
                    {current && (
                      <>
                        <Button variant="secondary" disabled={test.isPending} onClick={runTest}>
                          {t('adminB.testConnection')}
                        </Button>
                        <InlineConfirm
                          label={t('common.delete')}
                          onConfirm={() => del.mutate(current.id, { onSuccess: () => setSel(null) })}
                        />
                      </>
                    )}
                    {testMsg && (
                      <span
                        className={
                          testOk
                            ? 'text-[13px] text-ok'
                            : 'max-w-xl text-[13px] leading-snug text-err break-all'
                        }
                      >
                        {testMsg}
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
          </>
        )}
      </AdminQueryGate>
    </div>
  )
}
