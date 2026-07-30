import type { ReactNode } from 'react'
import styles from './Segmented.module.css'

interface Props<T extends string> {
  options: { value: T; label: ReactNode }[]
  value: T
  onChange(v: T): void
  mono?: boolean
  /** Tighter padding; wraps when the bar is narrow (detail pane, mobile). */
  compact?: boolean
}

export function Segmented<T extends string>({ options, value, onChange, mono, compact }: Props<T>) {
  return (
    <div className={[styles.group, compact && styles.compact].filter(Boolean).join(' ')}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          className={[styles.item, o.value === value && styles.active, mono && styles.mono]
            .filter(Boolean)
            .join(' ')}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
