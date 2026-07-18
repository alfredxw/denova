import type { ReactNode } from 'react'
import { FormSectionHeader } from '@/components/forms/form-section-header'

interface PresetSectionHeaderProps {
  title: string
  description: string
  badge?: string
  action?: ReactNode
}

/** Shared title treatment for preset editor sections and consoles. */
export function PresetSectionHeader({ title, description, badge, action }: PresetSectionHeaderProps) {
  const trailing = action ?? (badge ? (
    <span className="rounded border border-[var(--nova-accent)]/35 bg-[var(--nova-accent)]/10 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
      {badge}
    </span>
  ) : undefined)

  return <FormSectionHeader title={title} description={description} action={trailing} />
}
