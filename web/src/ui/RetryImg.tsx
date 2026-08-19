import { useEffect, useState, type ImgHTMLAttributes, type SyntheticEvent } from 'react'

function retrySrc(src: string): string {
  try {
    const abs = src.startsWith('http://') || src.startsWith('https://')
    const u = new URL(src, 'http://local.invalid')
    u.searchParams.set('r', '1')
    if (abs) return u.toString()
    return u.pathname + u.search + u.hash
  } catch {
    return src.includes('?') ? `${src}&r=1` : `${src}?r=1`
  }
}

/** 第一次解码失败时带 ?r=1 再拉一次（破浏览器/反代对 5xx 的短缓存）。 */
export function RetryImg({ src, onError, ...rest }: ImgHTMLAttributes<HTMLImageElement> & { src: string }) {
  const [cur, setCur] = useState(src)
  const [tried, setTried] = useState(false)

  useEffect(() => {
    setCur(src)
    setTried(false)
  }, [src])

  const handleError = (e: SyntheticEvent<HTMLImageElement>) => {
    if (!tried && src) {
      setTried(true)
      setCur(retrySrc(src))
      return
    }
    onError?.(e)
  }

  return <img {...rest} src={cur} onError={handleError} />
}
