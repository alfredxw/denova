import type { ReactNode } from 'react'

export function SectionTitle({ title, description, badge }: { title: string; description: string; badge?: string }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <div className="preset-workspace-title text-sm font-semibold text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{description}</div>
      </div>
      {badge ? (
        <span className="rounded border border-[var(--nova-accent)]/35 bg-[var(--nova-accent)]/10 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">{badge}</span>
      ) : null}
    </div>
  )
}

export function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={`grid gap-1.5 ${className}`}>
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}
