import { Ellipsis, ListTree } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import '@/components/file-tree/FileTreeMenu.css'

export function ManuscriptStructureMenu({ onChangeStructure }: { onChangeStructure: () => void }) {
  const { t } = useTranslation()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <TooltipIconButton label={t('bookStructure.actions')} tooltipSide="bottom"><Ellipsis /></TooltipIconButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="nova-file-tree-menu-surface nova-file-tree-menu-radix">
        <DropdownMenuGroup>
          <DropdownMenuItem className="nova-file-tree-menu-item" onSelect={onChangeStructure}>
            <ListTree />{t('bookStructure.change')}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
