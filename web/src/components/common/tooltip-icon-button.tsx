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
    tooltipSkipDelayMs?: number
    tooltipSide?: 'top' | 'right' | 'bottom' | 'left'
    tooltipSideOffset?: number
    useTooltipProvider?: boolean
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
  tooltipSkipDelayMs,
  tooltipSide = 'right',
  tooltipSideOffset = 6,
  useTooltipProvider = true,
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

  const tooltip = (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side={tooltipSide} sideOffset={tooltipSideOffset}>{label}</TooltipContent>
    </Tooltip>
  )

  if (!useTooltipProvider) return tooltip

  return (
    <TooltipProvider delayDuration={tooltipDelayMs} skipDelayDuration={tooltipSkipDelayMs}>
      {tooltip}
    </TooltipProvider>
  )
}
