import { memo, type ComponentProps, type ReactNode } from 'react'
import { motion } from 'motion/react'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { sessionCanvas } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'

interface AgentChatPaneProps {
  emptyContent?: ReactNode
  messageListProps: ComponentProps<typeof MessageList>
  inputAreaProps: ComponentProps<typeof InputArea>
  /** Optional shared reading boundary for the timeline and composer. */
  contentClassName?: string
  /** Keeps the composer stable while the current Session canvas exits and the next one enters. */
  sessionTransitionPending?: boolean
  className?: string
}

const StableMessageList = memo(MessageList)

/** Shared assembly for the primary chat and the desktop split-chat pane. */
export function AgentChatPane({ emptyContent, messageListProps, inputAreaProps, contentClassName, sessionTransitionPending = false, className }: AgentChatPaneProps) {
  return (
    <div className={cn('relative flex h-full min-h-0 flex-col', className)}>
      <motion.div
        data-nova-session-canvas
        data-state={sessionTransitionPending ? 'transitioning' : 'active'}
        aria-busy={sessionTransitionPending}
        className={cn('relative flex min-h-0 flex-1 flex-col', sessionTransitionPending && 'pointer-events-none select-none')}
        variants={sessionCanvas}
        initial={false}
        animate={sessionTransitionPending ? 'transitioning' : 'active'}
      >
        {emptyContent}
        <StableMessageList
          {...messageListProps}
          contentClassName={contentClassName ?? messageListProps.contentClassName}
        />
      </motion.div>
      <InputArea
        {...inputAreaProps}
        contentClassName={contentClassName ?? inputAreaProps.contentClassName}
      />
    </div>
  )
}
