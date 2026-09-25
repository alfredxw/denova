import { toast as sonner } from 'sonner'
import { ErrorDiagnostic } from '@/components/common/inline-error-notice'
import { diagnosticText } from '@/lib/error-diagnostics'

// Keep the existing toast API. Errors stay visible until dismissed and can be
// copied; successful notifications retain Sonner's normal behavior.
export const toast = Object.assign((...args: Parameters<typeof sonner>) => sonner(...args), {
  ...sonner,
  error: (message: Parameters<typeof sonner.error>[0], options?: Parameters<typeof sonner.error>[1]) => {
    const title = typeof message === 'string' ? diagnosticText(message) : message
    const description = typeof options?.description === 'string' ? diagnosticText(options.description) : ''
    return sonner.error(title, {
      ...options, duration: Infinity, closeButton: true,
      description: <>
        {typeof options?.description !== 'string' ? options?.description : null}
        <ErrorDiagnostic title={typeof title === 'string' ? title : ''} message={description} />
      </>,
    })
  },
})
