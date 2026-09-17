import type { ReactNode } from 'react'

export function Panel({ title, action, children }: { title?: ReactNode; action?: ReactNode; children: ReactNode }) {
  return (
    <div className="rounded-2xl overflow-hidden mb-6" style={{ background: 'var(--surface)', border: '1px solid var(--border)' }}>
      {title && (
        <div className="px-5 py-4 flex items-center justify-between" style={{ borderBottom: '1px solid var(--border)' }}>
          <h2 className="font-semibold" style={{ color: 'var(--text)' }}>
            {title}
          </h2>
          {action}
        </div>
      )}
      <div className="p-5">{children}</div>
    </div>
  )
}

export function StatTile({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div className="rounded-xl p-4" style={{ background: 'var(--surface-2)' }}>
      <div className="text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
        {label}
      </div>
      <div className="text-2xl font-semibold" style={{ color: 'var(--text)' }}>
        {value}
      </div>
      {hint && (
        <div className="text-xs mt-1" style={{ color: 'var(--text-muted)' }}>
          {hint}
        </div>
      )}
    </div>
  )
}

export function EmptyNote({ children }: { children: ReactNode }) {
  return (
    <p className="text-sm rounded-lg px-4 py-3" style={{ color: 'var(--text-muted)', background: 'var(--surface-2)' }}>
      {children}
    </p>
  )
}
