import type { ReactNode } from 'react'
import { Button, type buttonVariants } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { VariantProps } from 'class-variance-authority'

type TooltipIconButtonProps = React.ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    label: string
    children: ReactNode
    showTooltip?: boolean
    tooltipDelayMs?: number
    tooltipSide?: 'top' | 'right' | 'bottom' | 'left'
  }

/** 带 Tooltip 的图标按钮，统一图标操作的可访问名称和提示行为。 */
export function TooltipIconButton({
  label,
  children,
  variant = 'ghost',
  size = 'icon-xs',
  className,
  showTooltip = true,
  tooltipDelayMs = 0,
  tooltipSide = 'right',
  'aria-label': ariaLabel,
  ...props
}: TooltipIconButtonProps) {
  const button = (
    <Button
      type="button"
      variant={variant}
      size={size}
      className={className}
      aria-label={ariaLabel || label}
      {...props}
    >
      {children}
    </Button>
  )

  if (!showTooltip) return button

  return (
    <TooltipProvider delayDuration={tooltipDelayMs}>
      <Tooltip delayDuration={tooltipDelayMs}>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent side={tooltipSide} sideOffset={6}>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
