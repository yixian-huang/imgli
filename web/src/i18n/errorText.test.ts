import { expect, test } from 'vitest'
import { errorText } from './errorText'
import { useGlobal } from '../store'

test('en:已知 code → 英文译文', () => {
  useGlobal.setState({ lang: 'en' })
  expect(errorText('unauthorized', '请先登录')).toBe('Please sign in')
})

test('en:未知 code → 回落后端 message', () => {
  useGlobal.setState({ lang: 'en' })
  expect(errorText('weird_code', 'backend msg')).toBe('backend msg')
})

test('zh:恒用后端 message(保细分不丢信息,不回归)', () => {
  useGlobal.setState({ lang: 'zh' })
  expect(errorText('invalid_request', '不能封禁自己')).toBe('不能封禁自己')
  expect(errorText('unauthorized', '请先登录')).toBe('请先登录')
})

test('无 code → fallback', () => {
  useGlobal.setState({ lang: 'en' })
  expect(errorText(undefined, 'fb')).toBe('fb')
})

test('en: detailed invalid_request keeps server message', () => {
  useGlobal.setState({ lang: 'en' })
  const long =
    'config 无效: CDN 域名须以 http:// 或 https:// 开头，例如 https://cdn.example.com'
  expect(errorText('invalid_request', long)).toBe(long)
})

test('heic_unsupported: zh/en 都走 i18n，不回落中文后端原文', () => {
  const zh = '当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）'
  const en = 'This build cannot decode HEIC. Use the official Docker image or make build-vips (libheif required).'
  useGlobal.setState({ lang: 'zh' })
  expect(errorText('heic_unsupported', 'ignored fallback')).toBe(zh)
  useGlobal.setState({ lang: 'en' })
  expect(errorText('heic_unsupported', zh)).toBe(en)
})
