import { fireEvent, render } from '@testing-library/react'
import { RetryImg } from './RetryImg'

it('onError 给 src 加 ?r=1 再拉一次', () => {
  const { container } = render(<RetryImg src="http://x/t/k.jpg" alt="k" />)
  const img = container.querySelector('img')!
  expect(img).toHaveAttribute('src', 'http://x/t/k.jpg')
  fireEvent.error(img)
  expect(img).toHaveAttribute('src', 'http://x/t/k.jpg?r=1')
})

it('相对路径同样追加 query', () => {
  const { container } = render(<RetryImg src="/t/k.jpg" alt="" />)
  const img = container.querySelector('img')!
  fireEvent.error(img)
  expect(img.getAttribute('src')).toBe('/t/k.jpg?r=1')
})

it('只重试一次，第二次 error 交给 onError', () => {
  const onError = vi.fn()
  const { container } = render(<RetryImg src="/t/k.jpg" alt="" onError={onError} />)
  const img = container.querySelector('img')!
  fireEvent.error(img)
  expect(onError).not.toHaveBeenCalled()
  fireEvent.error(img)
  expect(onError).toHaveBeenCalledTimes(1)
})
