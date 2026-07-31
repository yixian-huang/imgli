import { useMemo, useState } from 'react'
import {
  useAdminPolicies,
  useCreatePolicy,
  useDeletePolicy,
  useStartStorageMigrate,
  useStorageMigrateJob,
  useTestPolicy,
  useUpdatePolicy,
} from '../../../api/adminHooks'
import type { AdminPolicy, StorageCaps } from '../../../api/types'
import { useT } from '../../../i18n'
import { useGlobal } from '../../../store'
import { PageHeader } from '../../../shell/PageHeader'
import { Button } from '../../../ui/Button'
import { EmptyState } from '../../../ui/EmptyState'
import { InlineConfirm } from '../../../ui/InlineConfirm'
import { Input } from '../../../ui/Input'
import { Segmented } from '../../../ui/Segmented'
import { Toggle } from '../../../ui/Toggle'
import { AdminQueryGate } from '../ui/AdminQueryGate'
import forms from '../ui/adminForms.module.css'
import styles from './PoliciesPage.module.css'

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
    cdn: p.cdn_domain,
    tpl: p.path_template,
    enabled: p.enabled,
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
    <div className={styles.capsPanel} data-tier={caps.tier}>
      <div className={styles.capsTitle}>
        {t('adminB.capsTitle')}
        <span className={styles.tierBadge}>{tierLabel(t, caps.tier)}</span>
      </div>
      <p className={styles.capsSummary}>{summary}</p>
      <ul className={styles.capsList}>
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
        <div className={styles.limitations}>
          <div className={forms.label}>{t('adminB.capsLimitations')}</div>
          <ul>
            {losses.map((k) => (
              <li key={k}>{t(k)}</li>
            ))}
          </ul>
        </div>
      )}
      {warnings && warnings.length > 0 && (
        <ul className={styles.warnList}>
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

  const policies = policiesQ.data?.items ?? []
  const [sel, setSel] = useState<number | 'new' | null>(null)
  const [form, setForm] = useState<FormState>(NEW_FORM)
  const [testMsg, setTestMsg] = useState<string | null>(null)
  const [compatAck, setCompatAck] = useState(false)
  const [migFrom, setMigFrom] = useState<number | ''>('')
  const [migTo, setMigTo] = useState<number | ''>('')
  const [migDry, setMigDry] = useState(true)
  const [migDel, setMigDel] = useState(false)
  const [migLimit, setMigLimit] = useState('0')
  const [migJobId, setMigJobId] = useState<string | null>(null)
  const migJobQ = useStorageMigrateJob(migJobId)
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
    setCompatAck(false)
  }
  const selectNew = () => {
    setSel('new')
    setForm(NEW_FORM)
    setTestMsg(null)
    setCompatAck(false)
  }

  const caps = current?.caps && current.driver === form.driver ? current.caps : staticCaps(form.driver)
  const warnings =
    current?.warnings && current.driver === form.driver
      ? current.warnings
      : form.cdn.trim() && !caps.public_cdn_offload_recommended
        ? [{ code: 'cdn_not_recommended', message_key: 'adminB.warnCdnWithoutCap', severity: 'warning' }]
        : form.driver === 'ftp' && form.ftpAllowInsecure
          ? [{ code: 'insecure_transport', message_key: 'adminB.warnInsecureTransport', severity: 'warning' }]
          : undefined

  const config = buildConfig(form)

  const needsCompatAck =
    form.driver === 'ftp' && form.enabled && (sel === 'new' || Boolean(current && !current.enabled))

  const submit = () => {
    setTestMsg(null)
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
        { onSuccess: () => setSel(null) },
      )
    } else if (current) {
      update.mutate({
        id: current.id,
        body: { name: form.name.trim(), config, cdn_domain: form.cdn, path_template: form.tpl, enabled: form.enabled },
      })
    }
  }

  const runTest = () => {
    if (!current) return
    setTestMsg(null)
    test.mutate(current.id, { onSuccess: (r) => setTestMsg(t('adminB.connectedMs', { ms: r.latency_ms })) })
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
          <div className={styles.migratePanel}>
            <div className={styles.migrateTitle}>{t('adminB.migrateTitle')}</div>
            <p className={forms.hint}>{t('adminB.migrateDesc')}</p>
            <div className={styles.migrateRow}>
              <label className={forms.field}>
                <span className={forms.label}>{t('adminB.migrateFrom')}</span>
                <select
                  className={styles.migrateSelect}
                  value={migFrom === '' ? '' : String(migFrom)}
                  onChange={(e) => setMigFrom(e.target.value ? Number(e.target.value) : '')}
                >
                  <option value="">—</option>
                  {policyOptions.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className={forms.field}>
                <span className={forms.label}>{t('adminB.migrateTo')}</span>
                <select
                  className={styles.migrateSelect}
                  value={migTo === '' ? '' : String(migTo)}
                  onChange={(e) => setMigTo(e.target.value ? Number(e.target.value) : '')}
                >
                  <option value="">—</option>
                  {policyOptions.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </label>
              <Input
                label={t('adminB.migrateLimit')}
                value={migLimit}
                onChange={(e) => setMigLimit(e.target.value)}
              />
            </div>
            <div className={styles.migrateToggles}>
              <label className={styles.ackRow}>
                <input type="checkbox" checked={migDry} onChange={(e) => setMigDry(e.target.checked)} />
                <span>{t('adminB.migrateDryRun')}</span>
              </label>
              <label className={styles.ackRow}>
                <input
                  type="checkbox"
                  checked={migDel}
                  disabled={migDry}
                  onChange={(e) => setMigDel(e.target.checked)}
                />
                <span>{t('adminB.migrateDeleteSource')}</span>
              </label>
              <Button variant="secondary" disabled={startMigrate.isPending} onClick={runMigrate}>
                {t('adminB.migrateStart')}
              </Button>
            </div>
            {migJobQ.data && (
              <div className={styles.migrateStatus}>
                <div>{t('adminB.migrateStatus', { status: migJobQ.data.status })}</div>
                <div>
                  {t('adminB.migrateProgress', {
                    scanned: migJobQ.data.progress.scanned,
                    copied: migJobQ.data.progress.copied,
                    skipped: migJobQ.data.progress.skipped,
                    failed: migJobQ.data.progress.failed,
                  })}
                </div>
                {migJobQ.data.error && <p className={styles.warnInline}>{migJobQ.data.error}</p>}
              </div>
            )}
          </div>
          <div className={styles.split}>
            <div className={styles.list}>
              {policies.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className={[styles.row, sel === p.id && styles.rowActive].filter(Boolean).join(' ')}
                  onClick={() => selectPolicy(p)}
                >
                  <span className={styles.rowName}>{p.name}</span>
                  {p.tier === 'compat' && <span className={styles.tierBadge}>{t('adminB.tierCompat')}</span>}
                  {!p.enabled && <span className={styles.off}>{t('adminB.disabled')}</span>}
                  <span className={styles.rowCount}>{t('adminB.fileCount', { count: p.file_count })}</span>
                </button>
              ))}
            </div>
            <div className={styles.detail}>
              {sel === null ? (
                <EmptyState title={t('adminB.selectOrCreatePolicy')} desc={t('adminB.selectOrCreatePolicyDesc')} />
              ) : (
                <div className={styles.form}>
                  <Input label={t('adminB.name')} value={form.name} onChange={(e) => set('name', e.target.value)} />
                  <div className={forms.field}>
                    <span className={forms.label}>{t('adminB.driver')}</span>
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
                      <div className={styles.driver}>{driverLabel}</div>
                    )}
                  </div>
                  <PolicyCapsPanel caps={caps} warnings={warnings} />
                  {form.driver === 'ftp' && <p className={forms.hint}>{t('storage.help.ftpPreferProxy')}</p>}
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
                        extra={<span className={forms.hint}>{t('adminB.secretMaskHint')}</span>}
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
                        extra={<span className={forms.hint}>{t('adminB.secretMaskHint')}</span>}
                        onChange={(e) => set('ftpPassword', e.target.value)}
                        onFocus={(e) => e.target.select()}
                      />
                      <Input
                        label={t('adminB.ftpPrefix')}
                        value={form.ftpPrefix}
                        placeholder="imgli"
                        onChange={(e) => set('ftpPrefix', e.target.value)}
                      />
                      <div className={styles.toggleRow}>
                        <span className={forms.label}>{t('adminB.ftpAllowInsecure')}</span>
                        <Toggle checked={form.ftpAllowInsecure} onChange={(v) => set('ftpAllowInsecure', v)} />
                      </div>
                      <span className={forms.hint}>{t('adminB.ftpAllowInsecureHint')}</span>
                    </>
                  ) : (
                    <>
                      <Input
                        label="Endpoint"
                        value={form.s3Endpoint}
                        placeholder="s3.us-east-1.amazonaws.com"
                        onChange={(e) => set('s3Endpoint', e.target.value)}
                      />
                      <Input
                        label="Region"
                        value={form.s3Region}
                        placeholder="us-east-1"
                        onChange={(e) => set('s3Region', e.target.value)}
                      />
                      <Input label="Bucket" value={form.s3Bucket} onChange={(e) => set('s3Bucket', e.target.value)} />
                      <Input label="AccessKey ID" value={form.s3AKID} onChange={(e) => set('s3AKID', e.target.value)} />
                      <Input
                        label="AccessKey Secret"
                        value={form.s3Secret}
                        extra={<span className={forms.hint}>{t('adminB.secretMaskHint')}</span>}
                        onChange={(e) => set('s3Secret', e.target.value)}
                        onFocus={(e) => e.target.select()}
                      />
                      <div className={forms.field}>
                        <span className={forms.label}>{t('adminB.pathStyle')}</span>
                        <Segmented<'true' | 'false'>
                          options={[
                            { value: 'false', label: t('adminB.pathStyleVirtual') },
                            { value: 'true', label: t('adminB.pathStylePath') },
                          ]}
                          value={form.s3PathStyle}
                          onChange={(v) => set('s3PathStyle', v)}
                        />
                      </div>
                      <Input
                        label={t('adminB.prefix')}
                        value={form.s3Prefix}
                        placeholder="imgli/"
                        onChange={(e) => set('s3Prefix', e.target.value)}
                      />
                      <Input
                        label={t('adminB.presignDomain')}
                        value={form.s3PresignDomain}
                        placeholder="https://s3.img.li"
                        extra={<span className={forms.hint}>{t('adminB.presignDomainHint')}</span>}
                        onChange={(e) => set('s3PresignDomain', e.target.value)}
                      />
                    </>
                  )}
                  <Input
                    label={t('adminB.cdnDomain')}
                    value={form.cdn}
                    placeholder={t('adminB.cdnDomainPlaceholder')}
                    onChange={(e) => set('cdn', e.target.value)}
                  />
                  {!caps.public_cdn_offload_recommended && form.cdn.trim() && (
                    <p className={styles.warnInline}>{t('adminB.warnCdnWithoutCap')}</p>
                  )}
                  <Input label={t('adminB.pathTemplate')} value={form.tpl} onChange={(e) => set('tpl', e.target.value)} />
                  <div className={styles.toggleRow}>
                    <span className={forms.label}>{t('adminB.enabled')}</span>
                    <Toggle checked={form.enabled} onChange={(v) => set('enabled', v)} />
                  </div>
                  {needsCompatAck && (
                    <label className={styles.ackRow}>
                      <input type="checkbox" checked={compatAck} onChange={(e) => setCompatAck(e.target.checked)} />
                      <span>
                        <strong>{t('adminB.confirmCompatEnableTitle')}</strong> — {t('adminB.confirmCompatEnableBody')} (
                        {t('adminB.confirmCompatEnableAck')})
                      </span>
                    </label>
                  )}
                  <div className={styles.actions}>
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
                    {testMsg && <span className={styles.testOk}>{testMsg}</span>}
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
