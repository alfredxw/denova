import type { ReactNode } from 'react'
import { Images } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import { ImageAgentModelSettingsMenu } from './ImageAgentModelSettingsMenu'

interface ImageGenerationSettingsMenuProps {
  children: ReactNode
  disabled?: boolean
  projectId?: string
}

/** Groups every image-generation choice under one composer entry in Writing and Game modes. */
export function ImageGenerationSettingsMenu({
  children,
  disabled = false,
  projectId = '',
}: ImageGenerationSettingsMenuProps) {
  const { t } = useTranslation()

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        disabled={disabled}
        className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        aria-label={t('chat.imageGenerationOptions')}
      >
        <Images className="h-3.5 w-3.5" />
        <span className="min-w-0 flex-1 truncate">{t('chat.imageGenerationOptions')}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-72 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)] max-[700px]:[translate:calc(-100%+0.5rem)_0]">
        <ImageAgentModelSettingsMenu projectId={projectId} disabled={disabled} />
        {children}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  )
}
