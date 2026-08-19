import { useState } from 'react'
import { ApiError } from '../../../../api/client'
import { usePreviewMail, useTestMail } from '../../../../api/adminHooks'
import type { MailKind } from '../../../../api/types'
import { useT } from '../../../../i18n'
import { errorText } from '../../../../i18n/errorText'
import { Button } from '../../../../ui/Button'
import { Input } from '../../../../ui/Input'
import { Segmented } from '../../../../ui/Segmented'
import { Toggle } from '../../../../ui/Toggle'
import {
  emptyMailCopy,
  MAIL_KINDS,
  smtpLooksLikeEmail,
  smtpPayload,
  smtpPortEncMismatch,
  smtpSuggestedPort,
  smtpTestRecipient,
  type FormSet,
  type FormState,
} from '../settingsForm'
import { s } from '../settingsUi'

interface Props {
  form: FormState
  set: FormSet
  testTo: string
  setTestTo: (v: string) => void
  testMsg: string | null
  testOk: boolean
  testPending: boolean
  onTest: () => void
}

export function SmtpTab({ form, set, testTo, setTestTo, testMsg, testOk, testPending, onTest }: Props) {
  const { t } = useT()
  const user = form.smtpUser.trim()
  const userLooksMail = smtpLooksLikeEmail(form.smtpUser)
  const needPassword = !!user && !form.smtpPassword
  return (
    <>
    <section className={s.section}>
      <h2 className={s.h2}>{t('adminB.smtpSection')}</h2>
      <p className={s.hint}>{t('adminB.smtpIntro')}</p>
      <Input
        label={t('adminB.smtpHost')}
        placeholder={t('adminB.smtpHostPlaceholder')}
        value={form.smtpHost}
        onChange={(e) => {
          set('smtpHost', e.target.value)
          if (form.smtpPassword.startsWith('****')) set('smtpPassword', '')
        }}
      />
      <Input
        label={t('adminB.port')}
        type="number"
        value={String(form.smtpPort)}
        extra={<span className={s.hint}>{t('adminB.smtpPortHint')}</span>}
        onChange={(e) => set('smtpPort', Number(e.target.value) || 0)}
      />
      {smtpPortEncMismatch(form.smtpPort, form.smtpEnc) && (
        <span className={s.hintWarn}>{t('adminB.smtpPortEncMismatch')}</span>
      )}
      <Input
        label={t('adminB.username')}
        placeholder={t('adminB.smtpUserPlaceholder')}
        value={form.smtpUser}
        onChange={(e) => {
          set('smtpUser', e.target.value)
          if (form.smtpPassword.startsWith('****')) set('smtpPassword', '')
        }}
      />
      {user && !userLooksMail ? (
        <span className={s.hintWarn}>{t('adminB.smtpUserNotEmail')}</span>
      ) : !user ? (
        <span className={s.hint}>{t('adminB.smtpUserHint')}</span>
      ) : null}
      <Input
        label={t('adminB.smtpPassword')}
        type="password"
        autoComplete="new-password"
        placeholder={t('adminB.noPasswordPlaceholder')}
        value={form.smtpPassword}
        extra={<span className={s.hint}>{t('adminB.passwordMaskHint')}</span>}
        onChange={(e) => set('smtpPassword', e.target.value)}
        onFocus={(e) => e.target.select()}
      />
      {needPassword ? (
        <span className={s.hintWarn}>{t('adminB.smtpPasswordNeeded')}</span>
      ) : (
        <span className={s.hint}>{t('adminB.smtpPasswordHint')}</span>
      )}
      <Input
        label={t('adminB.from')}
        placeholder="no-reply@img.li"
        value={form.smtpFrom}
        extra={<span className={s.hint}>{t('adminB.smtpFromHint')}</span>}
        onChange={(e) => set('smtpFrom', e.target.value)}
      />
      <div className={s.field}>
        <span className={s.label}>{t('adminB.encryption')}</span>
        <Segmented
          options={[
            { value: 'none', label: t('adminB.noEncryption') },
            { value: 'starttls', label: 'STARTTLS' },
            { value: 'ssl', label: 'SSL' },
          ]}
          value={form.smtpEnc}
          onChange={(v) => {
            const enc = v as FormState['smtpEnc']
            set('smtpEnc', enc)
            const next = smtpSuggestedPort(enc, form.smtpPort)
            if (next !== form.smtpPort) set('smtpPort', next)
          }}
        />
      </div>
      <div className={s.field}>
        <div className={s.sliderHead}>
          <span className={s.label}>{t('adminB.welcomeEmail')}</span>
          <Toggle
            aria-label={t('adminB.welcomeEmail')}
            checked={form.welcomeEmail}
            onChange={(v) => set('welcomeEmail', v)}
          />
        </div>
        <span className={s.hint}>{t('adminB.welcomeEmailHint')}</span>
      </div>
      <div className={s.field}>
        <span className={s.label}>{t('adminB.testSend')}</span>
        <div className={s.testRow}>
          <Input
            label={t('adminB.testRecipient')}
            placeholder={t('adminB.testRecipientPlaceholder')}
            value={testTo}
            onChange={(e) => setTestTo(e.target.value)}
          />
          <Button variant="secondary" disabled={testPending} onClick={onTest}>
            {t('adminB.sendTestEmail')}
          </Button>
        </div>
        {testMsg && <span className={testOk ? s.hint : s.hintWarn}>{testMsg}</span>}
      </div>
    </section>
    <MailCopyEditor form={form} set={set} testTo={testTo} />
    </>
  )
}

function MailCopyEditor({
  form,
  set,
  testTo,
}: {
  form: FormState
  set: FormSet
  testTo: string
}) {
  const { t, lang } = useT()
  const [kind, setKind] = useState<MailKind>('welcome')
  const [previewLang, setPreviewLang] = useState<'zh' | 'en'>(lang === 'en' ? 'en' : 'zh')
  const [preview, setPreview] = useState<{ subject: string; html: string } | null>(null)
  const [msg, setMsg] = useState<string | null>(null)
  const [msgOk, setMsgOk] = useState(false)
  const previewMut = usePreviewMail()
  const testMut = useTestMail()
  const copy = form.mailTemplates[kind]
  const patch = (next: Partial<typeof copy>) =>
    set('mailTemplates', { ...form.mailTemplates, [kind]: { ...copy, ...next } })

  const fail = (m: string) => {
    setMsgOk(false)
    setMsg(m)
  }

  return (
    <section className={s.section}>
      <h2 className={s.h2}>{t('adminB.mailCopy')}</h2>
      <p className={s.hint}>{t('adminB.mailCopyHint')}</p>
      <nav className={s.subTabs} aria-label={t('adminB.mailCopy')}>
        {MAIL_KINDS.map((k) => (
          <button
            key={k}
            type="button"
            className={[s.subTab, kind === k && s.subTabActive].filter(Boolean).join(' ')}
            aria-pressed={kind === k}
            onClick={() => {
              setKind(k)
              setPreview(null)
              setMsg(null)
            }}
          >
            {t(`adminB.mailKind_${k}`)}
          </button>
        ))}
      </nav>
      <div className={s.localePair}>
        <Input
          label={`${t('adminB.mailSubject')} · ${t('adminB.localeZh')}`}
          value={copy.subject.zh}
          placeholder={form.mailDefaults[kind].subject.zh}
          onChange={(e) => patch({ subject: { ...copy.subject, zh: e.target.value } })}
        />
        <Input
          label={`${t('adminB.mailSubject')} · ${t('adminB.localeEn')}`}
          value={copy.subject.en}
          placeholder={form.mailDefaults[kind].subject.en}
          onChange={(e) => patch({ subject: { ...copy.subject, en: e.target.value } })}
        />
      </div>
      <div className={s.localePair}>
        <div className={s.field}>
          <span className={s.label}>{`${t('adminB.mailBody')} · ${t('adminB.localeZh')}`}</span>
          <textarea
            className={s.textarea}
            rows={5}
            maxLength={2000}
            value={copy.body.zh}
            placeholder={form.mailDefaults[kind].body.zh}
            onChange={(e) => patch({ body: { ...copy.body, zh: e.target.value } })}
          />
        </div>
        <div className={s.field}>
          <span className={s.label}>{`${t('adminB.mailBody')} · ${t('adminB.localeEn')}`}</span>
          <textarea
            className={s.textarea}
            rows={5}
            maxLength={2000}
            value={copy.body.en}
            placeholder={form.mailDefaults[kind].body.en}
            onChange={(e) => patch({ body: { ...copy.body, en: e.target.value } })}
          />
        </div>
      </div>
      <div className={s.lexiconToolbar}>
        <Button
          variant="secondary"
          onClick={() => {
            set('mailTemplates', { ...form.mailTemplates, [kind]: form.mailDefaults[kind] })
            setPreview(null)
          }}
        >
          {t('adminB.mailFillDefault')}
        </Button>
        <Button
          variant="secondary"
          onClick={() => {
            set('mailTemplates', { ...form.mailTemplates, [kind]: emptyMailCopy() })
            setPreview(null)
          }}
        >
          {t('adminB.mailClearCopy')}
        </Button>
        <Button
          variant="secondary"
          disabled={previewMut.isPending}
          onClick={() => {
            setMsg(null)
            previewMut.mutate(
              { kind, lang: previewLang, templates: form.mailTemplates },
              {
                onSuccess: (data) => {
                  setPreview(data)
                  setMsgOk(true)
                  setMsg(null)
                },
                onError: (err) =>
                  fail(err instanceof ApiError ? errorText(err.code, err.message) : t('adminB.testFailed')),
              },
            )
          }}
        >
          {t('adminB.mailPreview')}
        </Button>
        <Button
          variant="secondary"
          disabled={testMut.isPending}
          onClick={() => {
            const to = smtpTestRecipient(form, testTo)
            if (!form.smtpHost.trim()) return fail(t('adminB.smtpHostRequired'))
            if (!to.includes('@')) return fail(t('adminB.invalidEmail'))
            setMsg(null)
            testMut.mutate(
              { to, kind, lang: previewLang, templates: form.mailTemplates, smtp: smtpPayload(form) },
              {
                onSuccess: () => {
                  setMsgOk(true)
                  setMsg(t('adminB.sentCheckInbox'))
                },
                onError: (err) =>
                  fail(err instanceof ApiError ? errorText(err.code, err.message) : t('adminB.sendFailed')),
              },
            )
          }}
        >
          {t('adminB.mailTestKind')}
        </Button>
        <Segmented
          options={[
            { value: 'zh', label: t('adminB.localeZh') },
            { value: 'en', label: t('adminB.localeEn') },
          ]}
          value={previewLang}
          onChange={(v) => setPreviewLang(v === 'en' ? 'en' : 'zh')}
        />
      </div>
      {msg && <span className={msgOk ? s.hint : s.hintWarn}>{msg}</span>}
      {preview && (
        <div className={s.field}>
          <span className={s.label}>{preview.subject}</span>
          <iframe
            title={t('adminB.mailPreview')}
            sandbox=""
            srcDoc={preview.html}
            className="h-[280px] w-full rounded-sm border border-border bg-white"
          />
        </div>
      )}
    </section>
  )
}
