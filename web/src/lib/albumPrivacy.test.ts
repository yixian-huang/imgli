import { describe, expect, it } from 'vitest'
import { albumForcesPrivate } from './albumPrivacy'

const albums = [
  { id: 1, visibility: 'public' },
  { id: 2, visibility: 'private' },
]

describe('albumForcesPrivate', () => {
  it('未分类 / 缺列表 / 公开相册不拦截', () => {
    expect(albumForcesPrivate(albums, null)).toBe(false)
    expect(albumForcesPrivate(albums, 0)).toBe(false)
    expect(albumForcesPrivate(undefined, 2)).toBe(false)
    expect(albumForcesPrivate(albums, 1)).toBe(false)
  })

  it('私密相册拦截改回公开', () => {
    expect(albumForcesPrivate(albums, 2)).toBe(true)
  })
})
