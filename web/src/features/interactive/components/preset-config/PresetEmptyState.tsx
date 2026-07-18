import { Layers3 } from 'lucide-react'
import type { ReactNode } from 'react'
import { EmptyState } from '@/components/common/EmptyState'

interface PresetEmptyStateProps {
  title: string
  description: string
  action?: ReactNode
}

/** Preset-domain empty state backed by the shared design-system variants. */
export function PresetEmptyState({ title, description, action }: PresetEmptyStateProps) {
  return (
    <EmptyState
      icon={Layers3}
      title={title}
      description={description}
      variant="dashed"
      content={action}
      className="m-5 min-h-48 w-auto flex-1 self-stretch bg-[var(--nova-surface)] sm:m-8"
    />
  )
}
