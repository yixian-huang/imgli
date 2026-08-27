import { expect, test } from 'vitest'
import { detectLang, t } from './index'
import { useGlobal } from '../store'

test('t 取值中英', () => {
  useGlobal.setState({ lang: 'zh' })
  expect(t('common.save')).toBe('保存')
  useGlobal.setState({ lang: 'en' })
  expect(t('common.save')).toBe('Save')
})

test('t 插值', () => {
  useGlobal.setState({ lang: 'en' })
  expect(t('common.copied', { label: 'URL' })).toBe('Copied URL')
  useGlobal.setState({ lang: 'zh' })
  expect(t('common.copied', { label: 'URL' })).toBe('已复制 URL')
})

test('缺键回落 key', () => {
  expect(t('no.such.key')).toBe('no.such.key')
})

test('heic_unsupported / heicDecode 中英', () => {
  useGlobal.setState({ lang: 'zh' })
  expect(t('errors.heic_unsupported')).toBe(
    '当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）',
  )
  expect(t('adminA.heicDecode')).toBe('HEIC 解码')
  useGlobal.setState({ lang: 'en' })
  expect(t('errors.heic_unsupported')).toBe(
    'This build cannot decode HEIC. Use the official Docker image or make build-vips (libheif required).',
  )
  expect(t('adminA.heicDecode')).toBe('HEIC decode')
})

test('detectLang localStorage 优先', () => {
  localStorage.setItem('imgli-lang', 'en')
  expect(detectLang()).toBe('en')
  localStorage.setItem('imgli-lang', 'zh')
  expect(detectLang()).toBe('zh')
})
