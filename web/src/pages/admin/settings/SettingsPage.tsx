import { useEffect, useRef, useState } from 'react'
import { ApiError } from '../../../api/client'
import { useAdminSettings, useTestModeration, useTestSMTP, useUpdateSettings } from '../../../api/adminHooks'
import type { SiteAnnouncement } from '../../../api/types'
import { useT } from '../../../i18n'
import { errorText } from '../../../i18n/errorText'
import { applySiteTheme } from '../../../lib/siteTheme'
import { PageHeader } from '../../../shell/PageHeader'
import { useGlobal } from '../../../store'
import { Button } from '../../../ui/Button'
import { AdminQueryGate } from '../ui/AdminQueryGate'
import { formatLexiconExport, mergeLexiconText, parseLexiconText } from './lexicon'
import {
  formOf,
  SETTINGS_TABS,
  smtpClientError,
  smtpNeedsPasswordReenter,
  smtpPayload,
  smtpTestRecipient,
  type FormFooterGroup,
  type FormLocale,
  type FormState,
  type SettingsTab,
} from './settingsForm'
import { s } from './settingsUi'
import { AppearanceTab } from './tabs/AppearanceTab'
import { BasicTab } from './tabs/BasicTab'
import { HotlinkTab } from './tabs/HotlinkTab'
import { ModerationTab } from './tabs/ModerationTab'
import { OcrTab } from './tabs/OcrTab'
import { ProcessingTab } from './tabs/ProcessingTab'
import { SlotsTab } from './tabs/SlotsTab'
import { SmtpTab } from './tabs/SmtpTab'

export function SettingsPage() {
  const { t } = useT()
  const q = useAdminSettings()
  const update = useUpdateSettings()
  const [form, setForm] = useState<FormState | null>(null)
  const [tab, setTab] = useState<SettingsTab>('basic')
  const [testTo, setTestTo] = useState('')
  const [testMsg, setTestMsg] = useState<string | null>(null)
  const [testOk, setTestOk] = useState(false)
  const testSMTP = useTestSMTP()
  const testModeration = useTestModeration()
  const lexiconFileRef = useRef<HTMLInputElement>(null)
  const lexiconImportMode = useRef<'replace' | 'merge'>('merge')

  // 仅首次加载时初始化;后台 refetch 不打断编辑。保存成功另行用返回值重置(见 submit)。
  useEffect(() => {
    setForm((f) => (f === null && q.data ? formOf(q.data) : f))
  }, [q.data])

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm((f) => (f ? { ...f, [k]: v } : f))

  const keywordCount = form ? parseLexiconText(form.ocrKeywords).length : 0

  const importLexiconFile = (file: File, mode: 'replace' | 'merge') => {
    const reader = new FileReader()
    reader.onload = () => {
      const text = String(reader.result ?? '')
      setForm((f) => {
        if (!f) return f
        const next = mode === 'merge' ? mergeLexiconText(f.ocrKeywords, text) : parseLexiconText(text).join('\n')
        return { ...f, ocrKeywords: next }
      })
      useGlobal.getState().pushToast(
        mode === 'merge' ? t('adminB.ocrImportMerged') : t('adminB.ocrImportReplaced'),
      )
    }
    reader.onerror = () => useGlobal.getState().pushToast(t('adminB.ocrImportFailed'))
    reader.readAsText(file)
  }

  const exportLexiconFile = () => {
    if (!form) return
    const words = parseLexiconText(form.ocrKeywords)
    const blob = new Blob([formatLexiconExport(words, form.siteName.trim() || undefined)], {
      type: 'text/plain;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `imgli-lexicon-${new Date().toISOString().slice(0, 10)}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const doTest = () => {
    if (!form) return
    const fail = (msg: string) => {
      setTestOk(false)
      setTestMsg(msg)
    }
    if (!form.smtpHost.trim()) return fail(t('adminB.smtpHostRequired'))
    const clientErr = smtpClientError(form)
    if (clientErr) return fail(t(`adminB.${clientErr}`))
    if (smtpNeedsPasswordReenter(form, q.data?.smtp)) {
      return fail(t('adminB.smtpPasswordReenter'))
    }
    const to = smtpTestRecipient(form, testTo)
    if (!to.includes('@')) return fail(t('adminB.invalidEmail'))
    setTestMsg(null)
    testSMTP.mutate(
      { to, smtp: smtpPayload(form) },
      {
        onSuccess: () => {
          setTestOk(true)
          setTestMsg(t('adminB.sentCheckInbox'))
        },
        onError: (err) =>
          fail(err instanceof ApiError ? errorText(err.code, err.message) : t('adminB.sendFailed')),
      },
    )
  }

  const doTestModeration = () => {
    testModeration.mutate(undefined, {
      onSuccess: (data) => {
        useGlobal.getState().pushToast(t('adminB.testDoneScore', { score: data.score.toFixed(2) }))
      },
      onError: (err) => {
        useGlobal
          .getState()
          .pushToast(err instanceof ApiError ? errorText(err.code, err.message) : t('adminB.testFailed'))
      },
    })
  }

  const submit = () => {
    if (!form) return
    const smtpErr = smtpClientError(form)
    if (smtpErr) {
      useGlobal.getState().pushToast(t(`adminB.${smtpErr}`))
      return
    }
    if (smtpNeedsPasswordReenter(form, q.data?.smtp)) {
      useGlobal.getState().pushToast(t('adminB.smtpPasswordReenter'))
      return
    }
    // 掩码凭据(****开头)仅在路由身份未变时后端才会保留;切 provider/改指向后
    // 发掩码值会被后端「改指向即失效」拒为 400,故此时清空——让后端按「缺凭据」
    // 给可操作校验错,而非阻断正常切换(codex 终审)。
    const orig = q.data?.moderation
    const apiKeyCtxSame = !!orig && form.modProvider === orig.provider && form.modEndpoint.trim() === orig.endpoint
    const akSecretCtxSame =
      !!orig && form.modProvider === orig.provider && form.modRegion === orig.region && form.modAKID === orig.access_key_id
    const ocrKeyCtxSame = !!orig && form.ocrEndpoint.trim() === (orig.ocr_keywords?.endpoint ?? '')
    const outApiKey = form.modApiKey.startsWith('****') && !apiKeyCtxSame ? '' : form.modApiKey
    const outAKSecret = form.modAKSecret.startsWith('****') && !akSecretCtxSame ? '' : form.modAKSecret
    const outOcrKey = form.ocrApiKey.startsWith('****') && !ocrKeyCtxSame ? '' : form.ocrApiKey
    const ocrKeywords = parseLexiconText(form.ocrKeywords)
    update.mutate(
      {
        site_name: form.siteName.trim(),
        registration_mode: form.regMode,
        guest_upload_enabled: form.guestUpload,
        plaza_enabled: form.plazaEnabled,
        moderation: {
          enabled: form.modEnabled,
          provider: form.modProvider,
          endpoint: form.modEndpoint.trim(),
          api_key: outApiKey,
          access_key_id: form.modAKID,
          access_key_secret: outAKSecret,
          region: form.modRegion,
          threshold: form.modThreshold,
          action: form.modAction,
          login_sample_rate: form.loginSampleRate,
          on_plugin_error: form.onPluginError,
          notify_on_reject: form.notifyOnReject,
          ocr_keywords: {
            enabled: form.ocrEnabled,
            endpoint: form.ocrEndpoint.trim(),
            api_key: outOcrKey,
            keywords: ocrKeywords,
            on_hit: form.ocrOnHit,
          },
        },
        smtp: smtpPayload(form),
        hotlink: {
          enabled: form.hotlinkEnabled,
          allowed_domains: form.hotlinkDomains.split('\n').map((d) => d.trim()).filter(Boolean),
          allow_empty_referer: form.hotlinkAllowEmpty,
        },
        processing: {
          text_watermark: {
            enabled: form.twEnabled,
            text: form.twText.trim(),
            position: form.twPos,
            opacity: form.twOpacity,
            size_ratio: form.twSizeRatio,
          },
          max_edge: form.maxEdge,
          strip_exif: form.stripExif,
          jpeg_quality: form.jpegQuality,
          output_format: form.outputFormat,
          webp_quality: form.webpQuality,
          webp_skip_if_larger: form.webpSkipIfLarger,
        },
        announcement: {
          enabled: form.ann.enabled,
          text: {
            zh: typeof form.ann.text === 'string' ? form.ann.text.trim() : (form.ann.text?.zh ?? '').trim(),
            en: typeof form.ann.text === 'string' ? '' : (form.ann.text?.en ?? '').trim(),
          },
          link_url: form.ann.link_url.trim(),
          link_label: {
            zh:
              typeof form.ann.link_label === 'string'
                ? form.ann.link_label.trim()
                : (form.ann.link_label?.zh ?? '').trim(),
            en:
              typeof form.ann.link_label === 'string' ? '' : (form.ann.link_label?.en ?? '').trim(),
          },
          dismissible: form.ann.dismissible,
          starts_at: form.ann.starts_at.trim(),
          ends_at: form.ann.ends_at.trim(),
        },
        footer: {
          groups: form.footerGroups.map((g) => ({
            title: { zh: g.title.zh.trim(), en: g.title.en.trim() },
            links: g.links
              .map((l) => ({
                label: { zh: l.label.zh.trim(), en: l.label.en.trim() },
                url: l.url.trim(),
              }))
              .filter((l) => (l.label.zh || l.label.en) && l.url),
          })),
        },
        html_inject: {
          head: form.htmlHead,
          body_end: form.htmlBodyEnd,
        },
        help_url: form.helpUrl.trim(),
        upgrade_url: form.upgradeUrl.trim(),
        register_notice: {
          zh: form.registerNotice.zh.trim(),
          en: form.registerNotice.en.trim(),
        },
        share_branding: form.shareBranding,
        favicon_url: form.faviconUrl.trim(),
        source_url: form.sourceUrl.trim(),
        oss_credit: form.ossCredit,
        about_enabled: form.aboutEnabled,
        about_body: {
          zh: form.aboutBody.zh.trim(),
          en: form.aboutBody.en.trim(),
        },
        welcome_email: form.welcomeEmail,
        mail_templates: form.mailTemplates,
        theme_accent: form.themeAccent.trim(),
        theme_bg_color: form.themeBgColor.trim(),
        theme_bg_image_url: form.themeBgImageUrl.trim(),
        theme_bg_dim: form.themeBgDim,
        theme_glass: form.themeGlass,
        public_stats: {
          enabled: form.publicStatsEnabled,
          since: form.publicStatsSince.trim(),
          show_uptime_days: form.publicStatsShowUptime,
          show_live_images: form.publicStatsShowImages,
          show_users: form.publicStatsShowUsers,
          show_used_bytes: form.publicStatsShowBytes,
        },
      },
      {
        onSuccess: (data) => {
          setForm(formOf(data))
          applySiteTheme(data)
          useGlobal.getState().pushToast(t('common.saved'))
        },
      },
    )
  }

  const setAnn = <K extends keyof SiteAnnouncement>(k: K, v: SiteAnnouncement[K]) =>
    setForm((f) => (f ? { ...f, ann: { ...f.ann, [k]: v } } : f))

  const patchFooterGroup = (gi: number, patch: Partial<FormFooterGroup>) =>
    setForm((f) => {
      if (!f) return f
      const groups = f.footerGroups.map((g, i) => (i === gi ? { ...g, ...patch } : g))
      return { ...f, footerGroups: groups }
    })

  const patchFooterLink = (
    gi: number,
    li: number,
    patch: { label?: FormLocale; url?: string },
  ) =>
    setForm((f) => {
      if (!f) return f
      const groups = f.footerGroups.map((g, i) => {
        if (i !== gi) return g
        const links = g.links.map((l, j) => (j === li ? { ...l, ...patch } : l))
        return { ...g, links }
      })
      return { ...f, footerGroups: groups }
    })

  return (
    <div>
      <PageHeader kicker="SYSTEM SETTINGS" title={t('adminB.settingsTitle')} />
      <AdminQueryGate query={{ isError: q.isError, data: form ?? undefined, refetch: q.refetch }}>
        {(f) => (
          <div className={s.form}>
            <nav className={s.tabs} aria-label={t('adminB.settingsTitle')}>
              {SETTINGS_TABS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  className={[s.tab, tab === item.key && s.tabActive].filter(Boolean).join(' ')}
                  aria-pressed={tab === item.key}
                  onClick={() => setTab(item.key)}
                >
                  {t(`adminB.${item.labelKey}`)}
                </button>
              ))}
            </nav>

            {tab === 'basic' && <BasicTab form={f} set={set} />}
            {tab === 'appearance' && <AppearanceTab form={f} set={set} />}
            {tab === 'slots' && (
              <SlotsTab
                form={f}
                set={set}
                setForm={setForm}
                setAnn={setAnn}
                patchFooterGroup={patchFooterGroup}
                patchFooterLink={patchFooterLink}
              />
            )}
            {tab === 'moderation' && (
              <ModerationTab
                form={f}
                set={set}
                testPending={testModeration.isPending}
                onTest={doTestModeration}
              />
            )}
            {tab === 'ocr' && (
              <OcrTab
                form={f}
                set={set}
                keywordCount={keywordCount}
                lexiconFileRef={lexiconFileRef}
                lexiconImportMode={lexiconImportMode}
                onImportFile={importLexiconFile}
                onExport={exportLexiconFile}
              />
            )}
            {tab === 'smtp' && (
              <SmtpTab
                form={f}
                set={set}
                testTo={testTo}
                setTestTo={setTestTo}
                testMsg={testMsg}
                testOk={testOk}
                testPending={testSMTP.isPending}
                onTest={doTest}
              />
            )}
            {tab === 'hotlink' && <HotlinkTab form={f} set={set} />}
            {tab === 'processing' && <ProcessingTab form={f} set={set} />}

            <div className={s.actions}>
              <Button variant="primary" disabled={update.isPending} onClick={submit}>
                {t('adminB.saveSettings')}
              </Button>
            </div>
          </div>
        )}
      </AdminQueryGate>
    </div>
  )
}
