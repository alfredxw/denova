import type { ReactNode } from 'react'
import { Images } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DropdownMenuGroup,
  DropdownMenuSub,
  DropdownMenuSubContent,
} from '@/components/ui/dropdown-menu'
import { ImageAgentModelSettingsMenu } from './ImageAgentModelSettingsMenu'
import { ComposerMenuSubTrigger } from './ComposerMenuRow'

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
    <DropdownMenuGroup>
      <DropdownMenuSub>
        <ComposerMenuSubTrigger
          icon={Images}
          label={t('chat.imageGenerationOptions')}
          disabled={disabled}
          aria-label={t('chat.imageGenerationOptions')}
        />
        <DropdownMenuSubContent className="w-72 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)] max-[700px]:[translate:calc(-100%+0.5rem)_0]">
          <ImageAgentModelSettingsMenu projectId={projectId} disabled={disabled} />
          {children}
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    </DropdownMenuGroup>
  )
}
