export interface Preferences {
  default_album_id: number | null
  default_visibility: '' | 'public' | 'private'
  default_policy_id: number | null
  auto_copy_format: '' | 'url' | 'markdown' | 'html' | 'bbcode' | 'share'
  watermark: {
    enabled: boolean
    position: string
    opacity: number
    margin: number
  }
  /** 用户语言偏好;空串=跟随前端 detect */
  lang?: 'zh' | 'en' | ''
}

/** 九宫格水印位置(用户图水印 / 站级文字水印共用)。label 为 i18n key，渲染时 t(label)。 */
export const POSITIONS = [
  { value: 'tl', label: 'settings.wmPos.tl' },
  { value: 'tc', label: 'settings.wmPos.tc' },
  { value: 'tr', label: 'settings.wmPos.tr' },
  { value: 'ml', label: 'settings.wmPos.ml' },
  { value: 'mc', label: 'settings.wmPos.mc' },
  { value: 'mr', label: 'settings.wmPos.mr' },
  { value: 'bl', label: 'settings.wmPos.bl' },
  { value: 'bc', label: 'settings.wmPos.bc' },
  { value: 'br', label: 'settings.wmPos.br' },
] as const

export interface PolicyOption {
  id: number
  name: string
}

export interface User {
  id: number
  username: string
  email: string
  nickname: string
  is_admin: boolean
  email_verified: boolean
  created_at: string
  preferences: Preferences
  avatar_url: string
  watermark_set: boolean
  public_profile: boolean
}

export interface Quota {
  used: number
  total: number
  max_file_size: number
  allowed_exts: string[]
  /** 本月出站已用（字节）；缺省旧后端可无 */
  bandwidth_used_month?: number
  /** 组月流量硬顶；0=不限 */
  bandwidth_quota_month?: number
  /** 账期 YYYY-MM（Asia/Shanghai） */
  bandwidth_period?: string
  /** 组默认有效期秒；0=默认永久 */
  default_expires_in?: number
  /** 组有效期上限秒；0=允许永久 */
  max_expires_in?: number
  default_max_views?: number
  /** 0=允许不限 */
  max_max_views?: number
  retention_days?: number
  force_max_age_days?: number
}

export interface Links {
  url: string
  markdown: string
  html: string
  bbcode: string
  thumbnail_url: string
  share_url?: string
}

export interface UploadResult {
  key: string
  name: string
  size: number
  instant: boolean
  /** Same-user live image reuse (no new library row / quota). */
  reused?: boolean
  links: Links
  /** RFC3339 or null */
  expires_at?: string | null
}

export interface Album {
  id: number
  name: string
  visibility: string
  /** 公开访客页默认：gallery | immersive */
  default_view?: 'gallery' | 'immersive' | string
  /** 画廊点击单张是否进入沉浸；默认 true，false 时点图进分享页 */
  click_to_immersive?: boolean
  description?: string
  image_count: number
  cover_key: string
  list_in_plaza?: boolean
  has_access_password?: boolean
  created_at: string
}

/** 公开相册访客 API 元数据（含可选属主）。 */
export interface PublicAlbumMeta {
  id: number
  name: string
  visibility: string
  default_view?: string
  /** 画廊点击单张是否进入沉浸；缺省按 true */
  click_to_immersive?: boolean
  description?: string
  image_count: number
  cover_key: string
  password_required?: boolean
  has_access_password?: boolean
  created_at?: string
  owner?: {
    username: string
    nickname: string
    public_profile: boolean
  } | null
}

/** 广场/公开主页相册卡片。 */
export interface PublicAlbumCard {
  id: number
  name: string
  description?: string
  image_count: number
  cover_key: string
  cover_url?: string
  thumbnail_url?: string
  views?: number
  username: string
  nickname: string
  created_at: string
}

/** 公开相册访客网格/沉浸共用图行。 */
export interface PublicAlbumImg {
  key: string
  name: string
  ext: string
  width: number
  height: number
  size: number
  thumbnail_url: string
  url: string
  share_path?: string
}

export interface ImageItem {
  key: string
  /** vanity 别名，可选 */
  slug?: string | null
  name: string
  ext: string
  size: number
  width: number
  height: number
  visibility: string
  album_id: number | null
  created_at: string
  /** RFC3339;null=永久 */
  expires_at: string | null
  /** 0=不限 */
  max_views?: number
  views_served?: number
  /** 是否设置了访问口令（永不返回明文） */
  has_access_password?: boolean
  links: Links
  /** 分享页 URL（公开分享 API 额外字段） */
  share_url?: string
  /** 分享 API：需要口令且当前未解锁 */
  password_required?: boolean
}

/** GET /api/v1/s/{key} public share payload */
export type ShareImage = ImageItem

export interface ImageDetail extends ImageItem {
  mime: string
  upload_ip: string
}

export interface ImageStats {
  total: number
  daily: { date: string; views: number }[]
}

export interface BatchResult {
  skipped?: boolean
  key: string
  ok: boolean
  error?: string
}

export interface ImagesPage {
  items: ImageItem[]
  next_cursor: string
}

export interface TrashItem {
  key: string
  name: string
  ext: string
  size: number
  width: number
  height: number
  deleted_at: string
  days_left: number
}

export interface TrashPage {
  items: TrashItem[]
  next_cursor: string
}

export interface ApiToken {
  id: number
  name: string
  scope: 'upload' | 'full'
  created_at: string
  last_used_at: string | null
  token?: string
}

export interface DailyCount {
  date: string
  count: number
}

export interface TrafficDay {
  date: string
  views: number
}

export interface TopReferer {
  host: string
  count: number
  suspect?: boolean
}

export interface ChannelCount {
  channel: string
  count: number
}

export interface BandwidthTopUser {
  user_id: number
  username: string
  used: number
}

export interface AdminStats {
  users: number
  images: number
  storage: number
  today_uploads: number
  pending_images: number
  rejected_images: number
  tasks_pending: number
  tasks_running: number
  daily: DailyCount[] | null
  traffic_7d: TrafficDay[]
  traffic_30d?: TrafficDay[]
  top_referers: TopReferer[]
  top_referers_30d?: TopReferer[]
  signups_30d?: DailyCount[]
  signup_channels_30d?: ChannelCount[]
  bandwidth_used_month?: number
  bandwidth_top_users?: BandwidthTopUser[]
  origin_metering_only?: boolean
  stats_retention_days?: number
}

export interface RefererImageRow {
  key: string
  name: string
  count: number
}

export interface AuditLog {
  id: number
  actor_id: number | null
  actor_type: string
  action: string
  detail: string
  ip: string
  created_at: string
}

export interface AdminLogsPage {
  items: AuditLog[]
  total: number
  page: number
  limit: number
}

export interface AdminUser {
  id: number
  username: string
  email: string
  nickname: string
  group_id: number
  status: string
  is_admin: boolean
  used_storage: number
  /** 本月出站已用（字节） */
  bandwidth_used_month?: number
  /** 账期 YYYY-MM */
  bandwidth_period?: string
  created_at: string
  /** 最近 Web session 签发时间；无 session 为 null */
  last_seen_at?: string | null
  image_count: number
  email_verified?: boolean
  signup_channel?: string
}

export interface AdminUsersPage {
  items: AdminUser[]
  total: number
  page: number
  limit: number
}

/** 机审触发摘要（审核队列；来自 moderation_flag audit.results） */
export interface ModerationTrigger {
  plugin: string
  severity: string
  score?: number | null
  hits?: string[]
}

export interface AdminImageItem {
  key: string
  name: string
  ext: string
  size: number
  visibility: string
  status: string
  is_whitelisted: boolean
  nsfw_score: number | null
  username: string
  /** 游客图为 null */
  user_id: number | null
  created_at: string
  /** 存储策略 id（管理端定位物理对象） */
  policy_id: number
  policy_name: string
  /** local | s3 | webdav | … */
  policy_driver: string
  /** public | private */
  surface: string
  /** 存储对象键 / 路径 */
  path: string
  /** 是否已在回收站（软删） */
  in_trash: boolean
  deleted_at?: string
  links: Links
  /** 仅审核队列可选返回 */
  triggers?: ModerationTrigger[]
}

/** DELETE /admin/images/{key} 响应 */
export interface AdminImageDeleteResult {
  key: string
  deleted: boolean
  permanent: boolean
  physical_queued?: boolean
  object_retained?: boolean
  policy_id?: number
  path?: string
}

export interface AdminImagesPage {
  items: AdminImageItem[]
  total: number
  page: number
  limit: number
}

export interface ReviewBatchResult {
  key: string
  ok: boolean
  error?: string
}

export interface AdminGroup {
  id: number
  name: string
  is_default: boolean
  is_guest: boolean
  storage_quota: number
  max_file_size: number
  /** 月流量硬顶（字节）；0=不限 */
  bandwidth_quota_month: number
  rate_per_minute: number
  rate_per_hour: number
  rate_per_day: number
  allowed_exts: string[]
  allowed_policy_ids: number[] | null
  default_expires_in?: number
  max_expires_in?: number
  default_max_views?: number
  max_max_views?: number
  retention_days?: number
  force_max_age_days?: number
  created_at: string
  user_count: number
}

export interface StorageCaps {
  tier: 'first_class' | 'supported' | 'compat' | 'migrate_only' | string
  summary_key: string
  transport_tls_preferred: boolean
  allows_insecure: boolean
  range_get: boolean
  list_prefix: boolean
  multipart_upload: boolean
  public_cdn_offload_recommended: boolean
  private_presign_capable: boolean
  hot_path_ok: boolean
  feature_loss_keys: string[] | null
}

export interface StorageEffective {
  transport_is_tls: boolean
  public_cdn_redirect_configured: boolean
  private_presign_ready: boolean
}

export interface PolicyWarning {
  code: string
  message_key: string
  severity: 'warning' | 'info' | string
}

export interface AdminPolicy {
  id: number
  name: string
  driver: string
  config: string
  cdn_domain: string
  path_template: string
  enabled: boolean
  created_at: string
  file_count: number
  used_bytes: number
  live_image_count?: number
  trash_image_count?: number
  tier?: string
  caps?: StorageCaps
  effective?: StorageEffective
  warnings?: PolicyWarning[] | null
}

export interface AdminOCRKeywords {
  enabled: boolean
  endpoint: string
  api_key: string
  keywords: string[] | null
  on_hit: string
}

export interface AdminModeration {
  enabled: boolean
  provider: 'webhook' | 'aliyun' | 'tencent' | 'openai' | 'nsfwjs'
  endpoint: string
  api_key: string
  access_key_id: string
  access_key_secret: string
  region: string
  threshold: number
  action: string
  /** OCR+词表插件；缺省或 null 时前端按关闭处理 */
  ocr_keywords?: AdminOCRKeywords | null
  /** 登录用户机审入队概率 0–1；游客恒全审。缺省 1 */
  login_sample_rate?: number
  /** open | review；插件失败策略。缺省 open */
  on_plugin_error?: string
  /** 拒绝后邮件通知属主。缺省 false */
  notify_on_reject?: boolean
}

export interface AdminSMTP {
  host: string
  port: number
  username: string
  password: string
  from: string
  encryption: 'none' | 'starttls' | 'ssl'
}

export type MailKind = 'welcome' | 'verify' | 'reset' | 'change_email' | 'reject'

export interface MailKindCopy {
  subject?: LocaleMap
  body?: LocaleMap
}

export type MailTemplates = Partial<Record<MailKind, MailKindCopy>>

export interface HotlinkSettings {
  enabled: boolean
  allowed_domains: string[]
  allow_empty_referer: boolean
}

export interface TextWatermarkSettings {
  enabled: boolean
  text: string
  position: string
  opacity: number
  size_ratio: number
}

export interface ProcessingSettings {
  text_watermark: TextWatermarkSettings
  max_edge: number
  /** default true when omitted (privacy) */
  strip_exif?: boolean | null
  /** 0 = default 90; else 1–100. Applies when JPEG is re-encoded (keep path). */
  jpeg_quality?: number
  /** keep (default) | webp — jpeg/png only; gif/webp inputs unchanged */
  output_format?: 'keep' | 'webp' | string
  /** 0 = default 80; else 1–100 */
  webp_quality?: number
  /** default true: if webp ≥ baseline, keep jpeg/png */
  webp_skip_if_larger?: boolean | null
}

export interface ProcessingCapabilities {
  webp_encode: boolean
}

/** zh/en operator copy; API may still send a legacy plain string. */
export type LocaleMap = { zh?: string; en?: string }
export type LocaleField = string | LocaleMap

export interface SiteAnnouncement {
  enabled: boolean
  text: LocaleField
  link_url: string
  link_label: LocaleField
  dismissible: boolean
  starts_at: string
  ends_at: string
}

export interface FooterLink {
  label: LocaleField
  url: string
}

export interface FooterGroup {
  title: LocaleField
  links: FooterLink[]
}

export interface SiteFooter {
  groups: FooterGroup[]
}

export interface HTMLInject {
  head: string
  body_end: string
}

/** Share page branding: off | site name only | site + help/upgrade links */
export type ShareBranding = 'off' | 'site' | 'links'

export interface AdminSettings {
  site_name: string
  registration_mode: string
  guest_upload_enabled: boolean
  plaza_enabled: boolean
  moderation: AdminModeration
  smtp: AdminSMTP
  hotlink: HotlinkSettings
  processing: ProcessingSettings
  /** Build capabilities (e.g. vips webp encode). Not a stored setting. */
  processing_capabilities?: ProcessingCapabilities
  announcement?: SiteAnnouncement
  footer?: SiteFooter
  html_inject?: HTMLInject
  help_url?: string
  upgrade_url?: string
  register_notice?: LocaleField
  share_branding?: ShareBranding
  favicon_url?: string
  source_url?: string
  oss_credit?: 'on' | 'off'
  about_enabled?: boolean
  about_body?: string | { zh?: string; en?: string }
  welcome_email?: boolean
  mail_templates?: MailTemplates
  /** 内置默认文案（只读，供「填入默认」）。 */
  mail_template_defaults?: MailTemplates
  /** Brand accent (#RRGGBB); empty = product default btn colors */
  theme_accent?: string
  /** Optional solid page background (#RRGGBB); empty = light/dark default */
  theme_bg_color?: string
  /** Optional full-page background image URL */
  theme_bg_image_url?: string
  /** Scrim strength over background image, 0–1 (default 0.72) */
  theme_bg_dim?: number
  /** Panel frosted opacity with background image, 0–1 (default 0.78) */
  theme_glass?: number
  /** Operator public instance stats (default off). Admin form config. */
  public_stats?: PublicStatsConfig
}

/** Admin settings shape for public instance stats (flags only). */
export interface PublicStatsConfig {
  enabled: boolean
  /** YYYY-MM-DD anchor for uptime; empty = auto */
  since?: string
  show_uptime_days?: boolean
  show_live_images?: boolean
  show_users?: boolean
  show_used_bytes?: boolean
}

/** Public /config snapshot (computed fields; omitted when flag off). */
export interface PublicStatsSnapshot {
  enabled: boolean
  uptime_days?: number
  live_image_count?: number
  user_count?: number
  used_bytes?: number
  as_of?: string
}

export interface GuestLimits {
  max_file_size: number
  allowed_exts: string[]
  per_day: number
  default_expires_in?: number
  max_expires_in?: number
  default_max_views?: number
  max_max_views?: number
  force_max_age_days?: number
}

export interface PublicConfig {
  site_name: string
  registration_mode: string
  guest_upload_enabled: boolean
  guest: GuestLimits | null
  plaza_enabled: boolean
  /** Public site base (IMGLI_BASE_URL); used for client config snippets. */
  base_url?: string
  announcement?: SiteAnnouncement | null
  footer?: SiteFooter
  html_inject?: HTMLInject
  /** OIDC SSO available */
  oidc_enabled?: boolean
  /** Operator-configured help docs (optional; empty by default). */
  help_url?: string
  /** Operator-configured upgrade / self-host CTA (optional). */
  upgrade_url?: string
  /** Shown on register form when non-empty (string or {zh,en}). */
  register_notice?: LocaleField
  share_branding?: ShareBranding
  favicon_url?: string
  source_url?: string
  oss_credit?: 'on' | 'off'
  about_enabled?: boolean
  about_body?: string | { zh?: string; en?: string }
  theme_accent?: string
  theme_bg_color?: string
  theme_bg_image_url?: string
  theme_bg_dim?: number
  theme_glass?: number
  /** Present on public config; enabled false by default for self-host. */
  public_stats?: PublicStatsSnapshot
}

export interface DiscoverAuthor {
  user_id: number
  username: string
  nickname: string
  avatar_version: number
}

export interface DiscoverRow {
  key: string
  name: string
  ext: string
  created_at: string
  views: number
  author: DiscoverAuthor
}

export interface DiscoverPage {
  items: DiscoverRow[]
  next_cursor: string
}

export interface PublicProfileData {
  username: string
  nickname: string
  avatar_version: number
  joined_at: string
  public_image_count: number
}

export interface AdminInvite {
  id: number
  code: string
  status: 'unused' | 'used' | 'expired'
  created_by_name: string
  used_by_name: string
  created_at: string
  expires_at: string | null
  used_at: string | null
}

export interface AdminInvitesPage {
  items: AdminInvite[]
  total: number
  page: number
  limit: number
}
