import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { t } from '../i18n'
import { errorText } from '../i18n/errorText'
import { useGlobal } from '../store'
import { api, ApiError, del, patch, post, put } from './client'
import { queryKeys } from './queryKeys'
import type { AdminGroup, AdminImageItem, AdminImagesPage, AdminInvitesPage, AdminLogsPage, AdminPolicy, AdminSettings, AdminStats, AdminUser, AdminUsersPage, RefererImageRow, ReviewBatchResult } from './types'

export function useAdminStats() {
  return useQuery({ queryKey: queryKeys.admin.stats, queryFn: () => api<AdminStats>('/admin/stats') })
}

export function useAdminRefererImages(host: string | null, days = 30) {
  const p = new URLSearchParams()
  if (host) p.set('host', host)
  p.set('days', String(days))
  return useQuery({
    queryKey: queryKeys.admin.refererImages(host, days),
    queryFn: () => api<{ host: string; items: RefererImageRow[] }>(`/admin/referers/images?${p}`),
    enabled: !!host,
  })
}

export interface LogsFilter {
  action?: string
  actor_type?: string
  page?: number
  limit?: number
}

export function useAdminLogs(f: LogsFilter = {}) {
  const p = new URLSearchParams()
  if (f.action) p.set('action', f.action)
  if (f.actor_type) p.set('actor_type', f.actor_type)
  if (f.page) p.set('page', String(f.page))
  if (f.limit) p.set('limit', String(f.limit))
  const qs = p.toString()
  return useQuery({
    queryKey: queryKeys.admin.logs(f),
    queryFn: () => api<AdminLogsPage>(`/admin/logs${qs ? `?${qs}` : ''}`),
  })
}

/** 侧栏审核 badge:limit=1 只为取 total;审核 mutation(④c)须 invalidate ['admin','review-count']。 */
export function useReviewCount() {
  return useQuery({
    queryKey: queryKeys.admin.reviewCount,
    queryFn: () => api<{ total: number }>('/admin/review?limit=1'),
    staleTime: 30_000,
    select: (d) => d.total,
  })
}

/** admin mutation 错误兜底:ApiError 经 errorText 本地化,其余展示通用文案;
 * 设置 hook 级 onError 即跳过 queryClient 的全局通用 toast。 */
function toastApiError(e: unknown) {
  useGlobal.getState().pushToast(
    e instanceof ApiError ? errorText(e.code, e.message) : t('errors.generic'),
  )
}

export interface AdminUsersFilter {
  q?: string
  group?: number
  status?: string
  channel?: string
  sort?: string
  page?: number
}

export function useAdminUsers(f: AdminUsersFilter = {}) {
  const p = new URLSearchParams()
  if (f.q) p.set('q', f.q)
  if (f.group) p.set('group', String(f.group))
  if (f.status) p.set('status', f.status)
  if (f.channel) p.set('channel', f.channel)
  if (f.sort) p.set('sort', f.sort)
  if (f.page && f.page > 1) p.set('page', String(f.page))
  const qs = p.toString()
  return useQuery({
    queryKey: queryKeys.admin.users(f),
    queryFn: () => api<AdminUsersPage>(`/admin/users${qs ? `?${qs}` : ''}`),
  })
}

export function useAdminGroups() {
  return useQuery({
    queryKey: queryKeys.admin.groups,
    queryFn: () => api<{ items: AdminGroup[] }>('/admin/groups'),
    staleTime: 60_000,
  })
}

export function useUpdateAdminUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: { group_id?: number; status?: string } }) =>
      patch<AdminUser>(`/admin/users/${id}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.usersRoot })
      qc.invalidateQueries({ queryKey: queryKeys.admin.groups })
    },
    onError: toastApiError,
  })
}

export function useResetAdminPassword() {
  return useMutation({
    mutationFn: (id: number) => post<{ password: string }>(`/admin/users/${id}/reset-password`),
    onError: toastApiError,
  })
}

export interface AdminImagesFilter {
  user?: number
  status?: string
  policy?: number
  page?: number
}

export function useAdminImages(f: AdminImagesFilter = {}) {
  const p = new URLSearchParams()
  if (f.user) p.set('user', String(f.user))
  if (f.status) p.set('status', f.status)
  if (f.policy) p.set('policy', String(f.policy))
  if (f.page && f.page > 1) p.set('page', String(f.page))
  const qs = p.toString()
  return useQuery({
    queryKey: queryKeys.admin.images(f),
    queryFn: () => api<AdminImagesPage>(`/admin/images${qs ? `?${qs}` : ''}`),
  })
}

export function useAdminPolicies() {
  return useQuery({
    queryKey: queryKeys.admin.policies,
    queryFn: () => api<{ items: AdminPolicy[] }>('/admin/policies'),
    staleTime: 60_000,
  })
}

export interface StorageMigrateProgress {
  scanned: number
  copied: number
  skipped: number
  failed: number
  sample_paths?: string[]
  errors?: string[]
}

export interface StorageMigrateJob {
  id: string
  from_policy_id: number
  to_policy_id: number
  dry_run: boolean
  delete_source: boolean
  limit: number
  status: string
  progress: StorageMigrateProgress
  cursor_after_id: number
  error?: string
  created_at: string
  updated_at: string
}

export function useStartStorageMigrate() {
  return useMutation({
    mutationFn: (body: {
      from_policy_id: number
      to_policy_id: number
      dry_run?: boolean
      delete_source?: boolean
      limit?: number
    }) => post<StorageMigrateJob>('/admin/storage/migrate', body),
    onError: toastApiError,
  })
}

export function useStorageMigrateJob(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.migrateJob(id ?? ''),
    queryFn: () => api<StorageMigrateJob>(`/admin/storage/migrate/${id}`),
    enabled: !!id,
    refetchInterval: (q) => {
      const s = q.state.data?.status
      if (s === 'done' || s === 'failed') return false
      return 1000
    },
  })
}

export function useSetImageWhitelist() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ key, on }: { key: string; on: boolean }) =>
      patch<AdminImageItem>(`/admin/images/${key}`, { is_whitelisted: on }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.imagesRoot })
      qc.invalidateQueries({ queryKey: queryKeys.admin.reviewCount })
    },
    onError: toastApiError,
  })
}

export function useDeleteAdminImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => del(`/admin/images/${key}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.imagesRoot })
      qc.invalidateQueries({ queryKey: queryKeys.admin.reviewCount })
    },
    onError: toastApiError,
  })
}

/* ---------- 审核队列 ---------- */

export function useAdminReview(page = 1) {
  const p = new URLSearchParams()
  if (page > 1) p.set('page', String(page))
  const qs = p.toString()
  return useQuery({
    queryKey: queryKeys.admin.review(page),
    queryFn: () => api<AdminImagesPage>(`/admin/review${qs ? `?${qs}` : ''}`),
  })
}

// 裁决后:队列出队、侧栏 badge、图片管理列表(状态从 pending 变 normal/rejected)均须刷新。
function invalidateReview(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: queryKeys.admin.reviewRoot })
  qc.invalidateQueries({ queryKey: queryKeys.admin.reviewCount })
  qc.invalidateQueries({ queryKey: queryKeys.admin.imagesRoot })
}

export function useReviewDecide() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ key, action }: { key: string; action: 'approve' | 'reject' }) =>
      post<AdminImageItem>(`/admin/review/${key}`, { action }),
    onSuccess: () => invalidateReview(qc),
    onError: toastApiError,
  })
}

export function useReviewBatch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ keys, action }: { keys: string[]; action: 'approve' | 'reject' }) =>
      post<{ results: ReviewBatchResult[] }>('/admin/review/batch', { keys, action }),
    onSuccess: () => invalidateReview(qc),
    onError: toastApiError,
  })
}

/* ---------- 用户组写操作 ---------- */

export interface GroupWriteBody {
  name?: string
  storage_quota?: number
  max_file_size?: number
  bandwidth_quota_month?: number
  rate_per_minute?: number
  rate_per_hour?: number
  rate_per_day?: number
  allowed_exts?: string[]
  allowed_policy_ids?: number[]
}

export function useCreateGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: GroupWriteBody) => post<AdminGroup>('/admin/groups', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.groups }),
    onError: toastApiError,
  })
}

export function useUpdateGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: GroupWriteBody }) =>
      patch<AdminGroup>(`/admin/groups/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.groups }),
    onError: toastApiError,
  })
}

export function useDeleteGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => del(`/admin/groups/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.groups }),
    onError: toastApiError,
  })
}

/* ---------- 存储策略写操作 ---------- */

export interface PolicyCreateBody {
  name: string
  driver: string
  config: string
  cdn_domain: string
  path_template: string
  enabled: boolean
}

export interface PolicyPatchBody {
  name?: string
  config?: string
  cdn_domain?: string
  path_template?: string
  enabled?: boolean
}

export function useCreatePolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: PolicyCreateBody) => post<AdminPolicy>('/admin/policies', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.policies }),
    onError: toastApiError,
  })
}

export function useUpdatePolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: PolicyPatchBody }) =>
      patch<AdminPolicy>(`/admin/policies/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.policies }),
    onError: toastApiError,
  })
}

export function useDeletePolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => del(`/admin/policies/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.policies }),
    onError: toastApiError,
  })
}

export function useTestPolicy() {
  return useMutation({
    mutationFn: (id: number) => post<{ ok: boolean; latency_ms: number }>(`/admin/policies/${id}/test`),
    onError: toastApiError,
  })
}

/* ---------- 系统设置 ---------- */

export function useAdminSettings() {
  return useQuery({ queryKey: queryKeys.admin.settings, queryFn: () => api<AdminSettings>('/admin/settings') })
}

export interface SettingsBody {
  site_name?: string
  registration_mode?: string
  guest_upload_enabled?: boolean
  plaza_enabled?: boolean
  moderation?: AdminSettings['moderation']
  smtp?: AdminSettings['smtp']
  hotlink?: AdminSettings['hotlink']
  processing?: AdminSettings['processing']
  announcement?: AdminSettings['announcement']
  footer?: AdminSettings['footer']
  html_inject?: AdminSettings['html_inject']
  help_url?: string
  upgrade_url?: string
  register_notice?: AdminSettings['register_notice']
  share_branding?: AdminSettings['share_branding']
  favicon_url?: string
  source_url?: string
  oss_credit?: AdminSettings['oss_credit']
  about_enabled?: boolean
  about_body?: AdminSettings['about_body']
  welcome_email?: boolean
}

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SettingsBody) => put<AdminSettings>('/admin/settings', body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.settings })
      qc.invalidateQueries({ queryKey: queryKeys.config })
    },
    onError: toastApiError,
  })
}

export function useTestSMTP() {
  return useMutation({
    mutationFn: (body: { to: string }) => post('/admin/settings/smtp/test', body),
    // 页面行内展示成败；hook 级存根跳过全局兜底 toast
    onError: () => {},
  })
}

export function useTestModeration() {
  return useMutation({
    mutationFn: () => post<{ score: number }>('/admin/settings/moderation/test'),
    // 页面 toast 展示成败；hook 级存根跳过全局兜底 toast
    onError: () => {},
  })
}

/* ---------- 邀请码 ---------- */

export function useAdminInvites(f: { status?: string; page?: number } = {}) {
  const p = new URLSearchParams()
  if (f.status) p.set('status', f.status)
  if (f.page) p.set('page', String(f.page))
  const qs = p.toString()
  return useQuery({
    queryKey: queryKeys.admin.invites(f),
    queryFn: () => api<AdminInvitesPage>(`/admin/invites${qs ? `?${qs}` : ''}`),
  })
}

export function useCreateInvites() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { count: number; expires_in_days?: number }) =>
      post<{ codes: string[] }>('/admin/invites', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.invitesRoot }),
    onError: toastApiError,
  })
}

export function useRevokeInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => del(`/admin/invites/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.invitesRoot }),
    onError: toastApiError,
  })
}
