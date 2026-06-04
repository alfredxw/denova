import { ChevronDown, ChevronRight } from 'lucide-react'

interface SectionHeaderProps {
  title: string
  count: number
  expanded: boolean
  onToggle: () => void
}

export function SectionHeader({ title, count, expanded, onToggle }: SectionHeaderProps) {
  return (
    <button type="button" className="nova-nav-item mt-3 flex w-full items-center gap-1 rounded-[var(--nova-radius)] py-1 text-left font-semibold text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={onToggle}>
      {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
      <span>{title}</span>
      <span className="ml-auto rounded-full bg-[var(--nova-active)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{count}</span>
    </button>
  )
}
