import { memo, type ComponentProps, type ReactNode } from 'react'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { cn } from '@/lib/utils'

interface AgentChatPaneProps {
  emptyContent?: ReactNode
  messageListProps: ComponentProps<typeof MessageList>
  inputAreaProps: ComponentProps<typeof InputArea>
  /** Optional shared reading boundary for the timeline and composer. */
  contentClassName?: string
  className?: string
}

const StableMessageList = memo(MessageList)

/** Shared assembly for the primary chat and the desktop split-chat pane. */
export function AgentChatPane({ emptyContent, messageListProps, inputAreaProps, contentClassName, className }: AgentChatPaneProps) {
  return (
    <div className={cn('relative flex h-full min-h-0 flex-col', className)}>
      {emptyContent}
      <StableMessageList
        {...messageListProps}
        contentClassName={contentClassName ?? messageListProps.contentClassName}
      />
      <InputArea
        {...inputAreaProps}
        contentClassName={contentClassName ?? inputAreaProps.contentClassName}
      />
    </div>
  )
}
