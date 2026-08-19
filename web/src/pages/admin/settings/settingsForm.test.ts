import { describe, expect, it } from 'vitest'
import {
  smtpClientError,
  smtpIdentitySame,
  smtpLooksLikeEmail,
  smtpNeedsPasswordReenter,
  smtpPayload,
  smtpPortEncMismatch,
  smtpSuggestedPort,
  smtpTestRecipient,
  type FormState,
} from './settingsForm'

function baseForm(over: Partial<FormState> = {}): FormState {
  return {
    smtpHost: '',
    smtpPort: 587,
    smtpUser: '',
    smtpPassword: '',
    smtpFrom: '',
    smtpEnc: 'starttls',
    ...over,
  } as FormState
}

describe('smtpNeedsPasswordReenter', () => {
  const saved = { host: 'smtp.larksuite.com', username: '', password: '****wxyz' }

  it('改用户名且密码仍是掩码 → 必须重输', () => {
    const form = baseForm({
      smtpHost: 'smtp.larksuite.com',
      smtpUser: 'noreply@qqqu.de',
      smtpPassword: '****wxyz',
    })
    expect(smtpIdentitySame(form, saved)).toBe(false)
    expect(smtpNeedsPasswordReenter(form, saved)).toBe(true)
  })

  it('改用户名后密码被清空 → 必须重输', () => {
    const form = baseForm({
      smtpHost: 'smtp.larksuite.com',
      smtpUser: 'noreply@qqqu.de',
      smtpPassword: '',
    })
    expect(smtpNeedsPasswordReenter(form, saved)).toBe(true)
  })

  it('改用户名并填了新密码 → 放行', () => {
    const form = baseForm({
      smtpHost: 'smtp.larksuite.com',
      smtpUser: 'noreply@qqqu.de',
      smtpPassword: 'imap-smtp-pw',
    })
    expect(smtpNeedsPasswordReenter(form, saved)).toBe(false)
  })

  it('身份未变（只改端口）→ 掩码可保留', () => {
    const orig = { host: 'smtp.larksuite.com', username: 'noreply@qqqu.de', password: '****wxyz' }
    const form = baseForm({
      smtpHost: 'smtp.larksuite.com',
      smtpUser: 'noreply@qqqu.de',
      smtpPassword: '****wxyz',
      smtpPort: 587,
    })
    expect(smtpNeedsPasswordReenter(form, orig)).toBe(false)
  })
})

describe('smtp pairing / client checks', () => {
  it('默认 587 点 SSL → 465；465 点 STARTTLS → 587；自定义端口不动', () => {
    expect(smtpSuggestedPort('ssl', 587)).toBe(465)
    expect(smtpSuggestedPort('starttls', 465)).toBe(587)
    expect(smtpSuggestedPort('ssl', 2465)).toBe(2465)
    expect(smtpSuggestedPort('none', 587)).toBe(587)
  })

  it('465+STARTTLS / 587+SSL 视为不搭配', () => {
    expect(smtpPortEncMismatch(465, 'starttls')).toBe(true)
    expect(smtpPortEncMismatch(587, 'ssl')).toBe(true)
    expect(smtpPortEncMismatch(465, 'ssl')).toBe(false)
    expect(smtpPortEncMismatch(587, 'starttls')).toBe(false)
  })

  it('client error: 端口 / 发件人 / 无加密+用户名', () => {
    expect(smtpClientError(baseForm({ smtpPort: 0 }))).toBe('smtpPortInvalid')
    expect(smtpClientError(baseForm({ smtpFrom: 'not-mail' }))).toBe('smtpFromInvalid')
    expect(smtpClientError(baseForm({ smtpEnc: 'none', smtpUser: 'u' }))).toBe('smtpNoneWithAuth')
    expect(smtpClientError(baseForm({ smtpFrom: 'a@b.c', smtpPort: 587 }))).toBe(null)
  })

  it('测试收件人：手填优先，否则 from，再否则用户名', () => {
    const form = baseForm({ smtpFrom: 'from@qqqu.de', smtpUser: 'user@qqqu.de' })
    expect(smtpTestRecipient(form, ' me@img.li ')).toBe('me@img.li')
    expect(smtpTestRecipient(form, '')).toBe('from@qqqu.de')
    expect(smtpTestRecipient(baseForm({ smtpUser: 'user@qqqu.de' }), '')).toBe('user@qqqu.de')
    expect(smtpTestRecipient(baseForm({ smtpUser: 'plain' }), '')).toBe('')
  })

  it('smtpLooksLikeEmail', () => {
    expect(smtpLooksLikeEmail('noreply@qqqu.de')).toBe(true)
    expect(smtpLooksLikeEmail('plain')).toBe(false)
    expect(smtpLooksLikeEmail('a@b')).toBe(false)
  })
})

describe('smtpPayload', () => {
  it('trim host/username/from', () => {
    const form = baseForm({
      smtpHost: ' smtp.larksuite.com ',
      smtpUser: '  noreply@qqqu.de',
      smtpFrom: 'noreply@qqqu.de ',
      smtpPassword: 'pw',
      smtpPort: 465,
      smtpEnc: 'ssl',
    })
    expect(smtpPayload(form)).toEqual({
      host: 'smtp.larksuite.com',
      port: 465,
      username: 'noreply@qqqu.de',
      password: 'pw',
      from: 'noreply@qqqu.de',
      encryption: 'ssl',
    })
  })
})
