import type { ReactNode } from 'react'
import { MobileWorkspaceHeader } from '@/components/layout/mobile-workspace-header'

interface StoryStageHeaderProps {
  isMobile: boolean
  controls: ReactNode
}

export function StoryStageHeader({ isMobile, controls }: StoryStageHeaderProps) {
  if (!isMobile) {
    return (
      <div className="nova-story-stage-header nova-topbar flex min-h-12 flex-wrap items-center justify-start gap-3 border-b px-4 py-2">
        <div className="nova-story-stage-controls flex min-w-0 flex-wrap items-center justify-start gap-2">{controls}</div>
      </div>
    )
  }
  return (
    <MobileWorkspaceHeader route="interactive">
      {controls}
    </MobileWorkspaceHeader>
  )
}
