import type { AdminSettings, MailKind, MailTemplates, ShareBranding, SiteAnnouncement } from '../../../api/types'
import { toLocaleMap } from '../../../lib/locale'

export const MAIL_KINDS: MailKind[] = ['welcome', 'verify', 'reset', 'change_email', 'reject']

export type MailKindCopyForm = { subject: FormLocale; body: FormLocale }
export type MailTemplatesForm = Record<MailKind, MailKindCopyForm>

export function emptyMailCopy(): MailKindCopyForm {
  return { subject: emptyLocale(), body: emptyLocale() }
}

export function emptyMailTemplates(): MailTemplatesForm {
  return {
    welcome: emptyMailCopy(),
    verify: emptyMailCopy(),
    reset: emptyMailCopy(),
    change_email: emptyMailCopy(),
    reject: emptyMailCopy(),
  }
}

export function mailTemplatesOf(raw?: MailTemplates | null): MailTemplatesForm {
  const out = emptyMailTemplates()
  if (!raw) return out
  for (const k of MAIL_KINDS) {
    const c = raw[k]
    if (!c) continue
    out[k] = { subject: toLocaleMap(c.subject), body: toLocaleMap(c.body) }
  }
  return out
}

export type FormLocale = { zh: string; en: string }

export type FormFooterLink = { label: FormLocale; url: string }
export type FormFooterGroup = { title: FormLocale; links: FormFooterLink[] }

export type ModProvider = 'webhook' | 'aliyun' | 'tencent' | 'openai' | 'nsfwjs'

export type SettingsTab =
  | 'basic'
  | 'appearance'
  | 'slots'
  | 'moderation'
  | 'ocr'
  | 'smtp'
  | 'hotlink'
  | 'processing'

export const SETTINGS_TABS: {
  key: SettingsTab
  labelKey:
    | 'basic'
    | 'appearance'
    | 'slotsTab'
    | 'moderation'
    | 'ocrSection'
    | 'smtpSection'
    | 'hotlink'
    | 'processing'
}[] = [
  { key: 'basic', labelKey: 'basic' },
  { key: 'appearance', labelKey: 'appearance' },
  { key: 'slots', labelKey: 'slotsTab' },
  { key: 'moderation', labelKey: 'moderation' },
  { key: 'ocr', labelKey: 'ocrSection' },
  { key: 'smtp', labelKey: 'smtpSection' },
  { key: 'hotlink', labelKey: 'hotlink' },
  { key: 'processing', labelKey: 'processing' },
]

export const emptyLocale = (): FormLocale => ({ zh: '', en: '' })

export const emptyAnn = (): SiteAnnouncement => ({
  enabled: false,
  text: emptyLocale(),
  link_url: '',
  link_label: emptyLocale(),
  dismissible: true,
  starts_at: '',
  ends_at: '',
})

export interface FormState {
  siteName: string
  regMode: 'open' | 'invite' | 'closed'
  guestUpload: boolean
  plazaEnabled: boolean
  modEnabled: boolean
  modProvider: ModProvider
  modEndpoint: string
  modApiKey: string
  modAKID: string
  modAKSecret: string
  modRegion: string
  modThreshold: number
  modAction: 'pending' | 'rejected'
  loginSampleRate: number
  onPluginError: 'open' | 'review'
  notifyOnReject: boolean
  ocrEnabled: boolean
  ocrEndpoint: string
  ocrApiKey: string
  ocrKeywords: string
  ocrOnHit: 'review' | 'block'
  smtpHost: string
  smtpPort: number
  smtpUser: string
  smtpPassword: string
  smtpFrom: string
  smtpEnc: 'none' | 'starttls' | 'ssl'
  hotlinkEnabled: boolean
  hotlinkDomains: string
  hotlinkAllowEmpty: boolean
  twEnabled: boolean
  twText: string
  twPos: string
  twOpacity: number
  twSizeRatio: number
  maxEdge: number
  stripExif: boolean
  jpegQuality: number
  outputFormat: 'keep' | 'webp'
  webpQuality: number
  webpSkipIfLarger: boolean
  webpEncodeAvailable: boolean
  ann: SiteAnnouncement
  footerGroups: FormFooterGroup[]
  htmlHead: string
  htmlBodyEnd: string
  helpUrl: string
  upgradeUrl: string
  registerNotice: FormLocale
  shareBranding: ShareBranding
  faviconUrl: string
  sourceUrl: string
  ossCredit: 'on' | 'off'
  aboutEnabled: boolean
  aboutBody: FormLocale
  welcomeEmail: boolean
  mailTemplates: MailTemplatesForm
  mailDefaults: MailTemplatesForm
  themeAccent: string
  themeBgColor: string
  themeBgImageUrl: string
  themeBgDim: number
  themeGlass: number
  publicStatsEnabled: boolean
  publicStatsSince: string
  publicStatsShowUptime: boolean
  publicStatsShowImages: boolean
  publicStatsShowUsers: boolean
  publicStatsShowBytes: boolean
}

export type FormSet = <K extends keyof FormState>(k: K, v: FormState[K]) => void

const MOD_PROVIDERS: ModProvider[] = ['webhook', 'aliyun', 'tencent', 'openai', 'nsfwjs']

export function formOf(s: AdminSettings): FormState {
  const tw = s.processing?.text_watermark
  const p = s.moderation.provider
  return {
    siteName: s.site_name,
    regMode: s.registration_mode === 'closed' ? 'closed' : s.registration_mode === 'invite' ? 'invite' : 'open',
    guestUpload: s.guest_upload_enabled,
    plazaEnabled: s.plaza_enabled,
    modEnabled: s.moderation.enabled,
    modProvider: MOD_PROVIDERS.includes(p as ModProvider) ? (p as ModProvider) : 'webhook',
    modEndpoint: s.moderation.endpoint,
    modApiKey: s.moderation.api_key,
    modAKID: s.moderation.access_key_id ?? '',
    modAKSecret: s.moderation.access_key_secret ?? '',
    modRegion: s.moderation.region ?? '',
    modThreshold: s.moderation.threshold,
    modAction: s.moderation.action === 'rejected' ? 'rejected' : 'pending',
    loginSampleRate:
      s.moderation.login_sample_rate != null && s.moderation.login_sample_rate >= 0
        ? s.moderation.login_sample_rate
        : 1,
    onPluginError: s.moderation.on_plugin_error === 'review' ? 'review' : 'open',
    notifyOnReject: !!s.moderation.notify_on_reject,
    ocrEnabled: s.moderation.ocr_keywords?.enabled ?? false,
    ocrEndpoint: s.moderation.ocr_keywords?.endpoint ?? '',
    ocrApiKey: s.moderation.ocr_keywords?.api_key ?? '',
    ocrKeywords: (s.moderation.ocr_keywords?.keywords ?? []).join('\n'),
    ocrOnHit: s.moderation.ocr_keywords?.on_hit === 'block' ? 'block' : 'review',
    smtpHost: s.smtp.host,
    smtpPort: s.smtp.port,
    smtpUser: s.smtp.username,
    smtpPassword: s.smtp.password,
    smtpFrom: s.smtp.from,
    smtpEnc: s.smtp.encryption === 'none' || s.smtp.encryption === 'ssl' ? s.smtp.encryption : 'starttls',
    hotlinkEnabled: s.hotlink.enabled,
    hotlinkDomains: s.hotlink.allowed_domains.join('\n'),
    hotlinkAllowEmpty: s.hotlink.allow_empty_referer,
    twEnabled: tw?.enabled ?? false,
    twText: tw?.text ?? '',
    twPos: tw?.position || 'br',
    twOpacity: tw?.opacity != null && tw.opacity >= 0.05 ? tw.opacity : 0.35,
    twSizeRatio: tw?.size_ratio != null && tw.size_ratio >= 0.01 ? tw.size_ratio : 0.05,
    maxEdge: s.processing?.max_edge ?? 0,
    stripExif: s.processing?.strip_exif !== false,
    jpegQuality: s.processing?.jpeg_quality ?? 0,
    webpEncodeAvailable: s.processing_capabilities?.webp_encode === true,
    outputFormat:
      s.processing?.output_format === 'webp' && s.processing_capabilities?.webp_encode === true
        ? 'webp'
        : 'keep',
    webpQuality: s.processing?.webp_quality ?? 0,
    webpSkipIfLarger: s.processing?.webp_skip_if_larger !== false,
    ann: s.announcement
      ? {
          enabled: !!s.announcement.enabled,
          text: toLocaleMap(s.announcement.text),
          link_url: s.announcement.link_url ?? '',
          link_label: toLocaleMap(s.announcement.link_label),
          dismissible: s.announcement.dismissible !== false,
          starts_at: s.announcement.starts_at ?? '',
          ends_at: s.announcement.ends_at ?? '',
        }
      : emptyAnn(),
    footerGroups: s.footer?.groups?.length
      ? s.footer.groups.map((g) => ({
          title: toLocaleMap(g.title),
          links: (g.links ?? []).map((l) => ({
            label: toLocaleMap(l.label),
            url: l.url ?? '',
          })),
        }))
      : [],
    htmlHead: s.html_inject?.head ?? '',
    htmlBodyEnd: s.html_inject?.body_end ?? '',
    helpUrl: s.help_url ?? '',
    upgradeUrl: s.upgrade_url ?? '',
    registerNotice: toLocaleMap(s.register_notice),
    shareBranding:
      s.share_branding === 'off' || s.share_branding === 'links'
        ? s.share_branding
        : 'site',
    faviconUrl: s.favicon_url ?? '',
    sourceUrl: s.source_url ?? '',
    ossCredit: s.oss_credit === 'off' ? 'off' : 'on',
    aboutEnabled: !!s.about_enabled,
    aboutBody: toLocaleMap(s.about_body),
    welcomeEmail: s.welcome_email !== false,
    mailTemplates: mailTemplatesOf(s.mail_templates),
    mailDefaults: mailTemplatesOf(s.mail_template_defaults),
    themeAccent: s.theme_accent ?? '',
    themeBgColor: s.theme_bg_color ?? '',
    themeBgImageUrl: s.theme_bg_image_url ?? '',
    themeBgDim:
      typeof s.theme_bg_dim === 'number' && s.theme_bg_dim >= 0 && s.theme_bg_dim <= 1
        ? s.theme_bg_dim
        : 0.72,
    themeGlass:
      typeof s.theme_glass === 'number' && s.theme_glass >= 0 && s.theme_glass <= 1
        ? s.theme_glass
        : 0.78,
    publicStatsEnabled: !!s.public_stats?.enabled,
    publicStatsSince: s.public_stats?.since ?? '',
    publicStatsShowUptime: s.public_stats?.show_uptime_days !== false,
    publicStatsShowImages: s.public_stats?.show_live_images !== false,
    publicStatsShowUsers: !!s.public_stats?.show_users,
    publicStatsShowBytes: !!s.public_stats?.show_used_bytes,
  }
}

export function smtpIdentitySame(
  form: FormState,
  orig?: { host: string; username: string },
): boolean {
  return !!orig && form.smtpHost.trim() === orig.host && form.smtpUser.trim() === orig.username
}

export function smtpPayload(form: FormState): {
  host: string
  port: number
  username: string
  password: string
  from: string
  encryption: FormState['smtpEnc']
} {
  return {
    host: form.smtpHost.trim(),
    port: form.smtpPort,
    username: form.smtpUser.trim(),
    password: form.smtpPassword,
    from: form.smtpFrom.trim(),
    encryption: form.smtpEnc,
  }
}

/** 改了 host/username 后仍拿着掩码或被清空的旧密码 → 必须重输，不能把旧凭据打到新身份。 */
export function smtpNeedsPasswordReenter(
  form: FormState,
  orig?: { host: string; username: string; password: string },
): boolean {
  if (smtpIdentitySame(form, orig)) return false
  if (form.smtpPassword.startsWith('****')) return true
  return form.smtpPassword === '' && !!orig?.password
}

export function smtpLooksLikeEmail(s: string): boolean {
  return /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(s.trim())
}

export function smtpPortEncMismatch(port: number, enc: FormState['smtpEnc']): boolean {
  return (port === 465 && enc === 'starttls') || (port === 587 && enc === 'ssl')
}

/** 只在端口仍是 587/465 这一对默认值时跟着加密方式改，自定义端口不动。 */
export function smtpSuggestedPort(enc: FormState['smtpEnc'], current: number): number {
  if (enc === 'ssl' && current === 587) return 465
  if (enc === 'starttls' && current === 465) return 587
  return current
}

export type SmtpClientErrorKey = 'smtpPortInvalid' | 'smtpFromInvalid' | 'smtpNoneWithAuth'

export function smtpClientError(form: FormState): SmtpClientErrorKey | null {
  if (form.smtpPort < 1 || form.smtpPort > 65535) return 'smtpPortInvalid'
  if (form.smtpFrom.trim() && !smtpLooksLikeEmail(form.smtpFrom)) return 'smtpFromInvalid'
  if (form.smtpEnc === 'none' && form.smtpUser.trim()) return 'smtpNoneWithAuth'
  return null
}

export function smtpTestRecipient(form: FormState, testTo: string): string {
  const typed = testTo.trim()
  if (typed) return typed
  if (smtpLooksLikeEmail(form.smtpFrom)) return form.smtpFrom.trim()
  if (smtpLooksLikeEmail(form.smtpUser)) return form.smtpUser.trim()
  return ''
}
