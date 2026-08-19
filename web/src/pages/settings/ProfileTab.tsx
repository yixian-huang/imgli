import { useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { ApiError } from '../../api/client'
import {
  useChangePassword,
  useConfig,
  useDeleteAccount,
  useDeleteAvatar,
  useResendVerification,
  useSession,
  useChangeEmail,
  useUpdateProfile,
  useUploadAvatar,
} from '../../api/hooks'
import { useT } from '../../i18n'
import { errorText } from '../../i18n/errorText'
import { useGlobal } from '../../store'
import { Button } from '../../ui/Button'
import { RetryImg } from '../../ui/RetryImg'
import { InlineConfirm } from '../../ui/InlineConfirm'
import { Tag } from '../../ui/Tag'
import { Toggle } from '../../ui/Toggle'

const STRONG_RE = /^(?=.*[A-Za-z])(?=.*\d).{8,}$/

const field = 'flex flex-col gap-1.5'
const label = 'text-xs font-semibold text-muted'
const input =
  'rounded-sm border border-border bg-bg px-3 py-[9px] font-inherit text-[13px] text-ink outline-none focus:border-muted'
const kicker = 'mb-3 font-mono text-2xs tracking-[0.14em] text-muted'
const card = 'flex flex-col gap-3.5 rounded-sm border border-border bg-surface p-[18px]'
const section = 'mb-7'
const grid2 = 'grid grid-cols-2 gap-3.5 max-[560px]:grid-cols-1'
const errLine = 'animate-[fadeIn_0.15s] text-xs text-err'

export function ProfileTab() {
  const { t } = useT()
  const { data: user } = useSession()
  const { data: config } = useConfig()
  const updateProfile = useUpdateProfile()
  const changePwd = useChangePassword()
  const changeEmail = useChangeEmail()
  const resend = useResendVerification()
  const uploadAvatar = useUploadAvatar()
  const deleteAvatar = useDeleteAvatar()
  const deleteAccount = useDeleteAccount()
  const pushToast = useGlobal((s) => s.pushToast)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const fileRef = useRef<HTMLInputElement>(null)
  const [nick, setNick] = useState<string | null>(null)
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [pwdErr, setPwdErr] = useState<string | null>(null)
  const [delPwd, setDelPwd] = useState('')
  const [delErr, setDelErr] = useState<string | null>(null)
  const [newEmail, setNewEmail] = useState('')
  const [emailPwd, setEmailPwd] = useState('')
  const [emailErr, setEmailErr] = useState<string | null>(null)

  if (!user) return null
  const nickVal = nick ?? user.nickname ?? ''

  function savePwd() {
    setPwdErr(null)
    if (!oldPwd) return setPwdErr(t('settings.errCurrentPasswordRequired'))
    if (!STRONG_RE.test(newPwd)) return setPwdErr(t('settings.errPasswordWeak'))
    changePwd.mutate(
      { old_password: oldPwd, new_password: newPwd },
      {
        onSuccess: () => {
          setOldPwd('')
          setNewPwd('')
          pushToast(t('settings.toastPasswordUpdated'))
        },
        onError: (e) =>
          setPwdErr(
            e instanceof ApiError && e.code === 'invalid_credentials'
              ? t('settings.errCurrentPasswordWrong')
              : errorText((e as ApiError).code, (e as Error).message),
          ),
      },
    )
  }

  return (
    <div>
      <div className={section}>
        <div className={kicker}>{t('settings.profileKicker')}</div>
        <div className={card}>
          <div className="mb-4 flex items-center gap-4">
            {user.avatar_url ? (
              <RetryImg
                className="size-16 rounded-full border border-border object-cover"
                src={user.avatar_url}
                alt={t('settings.avatarAlt')}
              />
            ) : (
              <div className="flex size-16 items-center justify-center rounded-full border border-border bg-soft text-2xl font-bold text-ink">
                {(user.nickname || user.username).slice(0, 1)}
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <input
                ref={fileRef}
                type="file"
                accept="image/*"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) uploadAvatar.mutate(f, { onSuccess: () => pushToast(t('settings.toastAvatarUpdated')) })
                  e.target.value = ''
                }}
              />
              <Button variant="secondary" disabled={uploadAvatar.isPending} onClick={() => fileRef.current?.click()}>
                {t('settings.uploadAvatar')}
              </Button>
              {user.avatar_url && (
                <InlineConfirm
                  label={t('settings.removeAvatar')}
                  confirmLabel={t('settings.confirmRemoveAvatar')}
                  onConfirm={() => deleteAvatar.mutate()}
                />
              )}
            </div>
          </div>
          <div className={grid2}>
            <div className={field}>
              <label className={label} htmlFor="nick">
                {t('settings.nickname')}
              </label>
              <input id="nick" className={input} value={nickVal} onChange={(e) => setNick(e.target.value)} />
            </div>
            <div className={field}>
              <label className={label} htmlFor="email">
                {t('settings.email')}
              </label>
              <input
                id="email"
                className={`${input} bg-soft font-mono text-xs text-muted`}
                value={user.email}
                readOnly
              />
              <div className="flex flex-wrap items-center gap-2">
                {user.email_verified ? (
                  <Tag variant="ok">{t('settings.verified')}</Tag>
                ) : (
                  <>
                    <Tag variant="warn">{t('settings.unverified')}</Tag>
                    <Button
                      variant="secondary"
                      disabled={resend.isPending}
                      onClick={() =>
                        resend.mutate(undefined, {
                          onSuccess: () => pushToast(t('settings.toastVerificationSent')),
                        })
                      }
                    >
                      {t('settings.resendVerification')}
                    </Button>
                  </>
                )}
              </div>
              <div className={`${field} mt-3`}>
                <label className={label} htmlFor="new-email">
                  {t('settings.changeEmail')}
                </label>
                <input
                  id="new-email"
                  className={input}
                  value={newEmail}
                  onChange={(e) => setNewEmail(e.target.value)}
                  placeholder={t('settings.newEmailPlaceholder')}
                />
                <input
                  type="password"
                  className={`${input} mt-2`}
                  value={emailPwd}
                  onChange={(e) => setEmailPwd(e.target.value)}
                  placeholder={t('settings.confirmPassword')}
                />
                {emailErr && <div className={errLine}>{emailErr}</div>}
                <Button
                  variant="secondary"
                  className="mt-2"
                  disabled={changeEmail.isPending}
                  onClick={() => {
                    setEmailErr(null)
                    changeEmail.mutate(
                      { password: emailPwd, new_email: newEmail.trim() },
                      {
                        onSuccess: () => {
                          setNewEmail('')
                          setEmailPwd('')
                          pushToast(t('settings.toastChangeEmailSent'))
                        },
                        onError: (e) =>
                          setEmailErr(
                            e instanceof ApiError ? errorText(e.code, e.message) : t('settings.errGeneric'),
                          ),
                      },
                    )
                  }}
                >
                  {t('settings.sendChangeEmail')}
                </Button>
              </div>
            </div>
          </div>
          <div className={field}>
            <div className="flex items-center justify-between">
              <span className={label}>{t('settings.publicProfile')}</span>
              <Toggle
                checked={!!user.public_profile}
                onChange={(v) =>
                  updateProfile.mutate({ public_profile: v }, { onSuccess: () => pushToast(t('settings.toastSaved')) })
                }
                aria-label={t('settings.publicProfileAria')}
              />
            </div>
            <div className={`${label} font-normal`}>
              {t(config?.plaza_enabled ? 'settings.publicProfileHintPlazaOn' : 'settings.publicProfileHint', {
                username: user.username,
              })}
            </div>
            {!!config?.plaza_enabled && !user.public_profile && (
              <div className={`${label} font-normal text-ink`} data-testid="public-profile-plaza-off">
                {t('settings.publicProfileOffWhilePlaza')}
              </div>
            )}
          </div>
          <Button
            variant="primary"
            className="self-start"
            disabled={updateProfile.isPending}
            onClick={() =>
              updateProfile.mutate({ nickname: nickVal.trim() }, { onSuccess: () => pushToast(t('settings.toastSaved')) })
            }
          >
            {t('settings.saveChanges')}
          </Button>
        </div>
      </div>

      <div className={section}>
        <div className={kicker}>{t('settings.passwordKicker')}</div>
        <div className={card}>
          <div className={grid2}>
            <div className={field}>
              <label className={label} htmlFor="old-pwd">
                {t('settings.currentPassword')}
              </label>
              <input
                id="old-pwd"
                type="password"
                className={input}
                value={oldPwd}
                onChange={(e) => setOldPwd(e.target.value)}
              />
            </div>
            <div className={field}>
              <label className={label} htmlFor="new-pwd">
                {t('settings.newPassword')}
              </label>
              <input
                id="new-pwd"
                type="password"
                placeholder={t('settings.newPasswordPlaceholder')}
                className={input}
                value={newPwd}
                onChange={(e) => setNewPwd(e.target.value)}
              />
            </div>
          </div>
          {pwdErr && <div className={errLine}>{pwdErr}</div>}
          <Button className="self-start" disabled={changePwd.isPending} onClick={savePwd}>
            {t('settings.updatePassword')}
          </Button>
        </div>
      </div>

      <div className={section}>
        <div className={kicker}>{t('settings.dangerKicker')}</div>
        <div className={`${card} border-err`}>
          <p className="mb-3 mt-0 text-[13px] leading-[1.55] text-muted">
            {t('settings.dangerTextBefore')}
            <strong className="font-bold text-err">{t('settings.dangerTextStrong')}</strong>
            {t('settings.dangerTextAfter')}
          </p>
          <div className={field}>
            <label className={label} htmlFor="del-pwd">
              {t('settings.deleteConfirmPassword')}
            </label>
            <input
              id="del-pwd"
              type="password"
              className={input}
              value={delPwd}
              onChange={(e) => setDelPwd(e.target.value)}
            />
          </div>
          {delErr && <div className={errLine}>{delErr}</div>}
          <InlineConfirm
            label={t('settings.deleteAccount')}
            confirmLabel={t('settings.confirmDeleteAccount')}
            disabled={!delPwd || deleteAccount.isPending}
            onConfirm={() => {
              setDelErr(null)
              deleteAccount.mutate(
                { password: delPwd },
                {
                  onSuccess: () => {
                    pushToast(t('settings.toastAccountDeleted'))
                    qc.clear()
                    navigate('/login')
                  },
                  onError: (e) =>
                    setDelErr(
                      e instanceof ApiError && e.code === 'invalid_credentials'
                        ? t('settings.errPasswordWrong')
                        : e instanceof ApiError && e.code === 'admin_cannot_self_delete'
                          ? t('settings.errAdminCannotSelfDelete')
                          : errorText((e as ApiError).code, (e as Error).message),
                    ),
                },
              )
            }}
          />
        </div>
      </div>
    </div>
  )
}
