import { useToast } from '@/components/ui/use-toast'
import { useCallback, useState } from 'react'

interface ErrorWithRetry extends Error {
  retry?: () => Promise<void>
}

interface UseErrorHandlerOptions {
  showToast?: boolean
  fallbackMessage?: string
  maxRetries?: number
  retryDelay?: number
}

export function useErrorHandler(options: UseErrorHandlerOptions = {}) {
  const { toast } = useToast()
  const [retryCount, setRetryCount] = useState(0)
  const {
    showToast = true,
    fallbackMessage = 'An unexpected error occurred',
    maxRetries = 3,
    retryDelay = 1000,
  } = options

  const handleError = useCallback(
    async (error: unknown, customMessage?: string) => {
      console.error('Error caught:', error)

      let message = fallbackMessage
      let retry: (() => Promise<void>) | undefined

      if (error instanceof Error) {
        message = error.message || fallbackMessage
        if ('retry' in error) {
          retry = (error as ErrorWithRetry).retry
        }
      } else if (typeof error === 'string') {
        message = error
      }

      if (customMessage) {
        message = customMessage
      }

      // Show toast notification if enabled
      if (showToast) {
        toast({
          variant: 'destructive',
          title: 'Error',
          description: message,
        })
      }

      return { message, retry }
    },
    [toast, showToast, fallbackMessage],
  )

  const resetRetryCount = useCallback(() => {
    setRetryCount(0)
  }, [])

  return { handleError, retryCount, resetRetryCount }
}

// Async operation wrapper with error handling
export function useAsyncOperation<T = void>() {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)
  const { handleError } = useErrorHandler()

  const execute = useCallback(
    async (operation: () => Promise<T>): Promise<T | undefined> => {
      setLoading(true)
      setError(null)

      try {
        const result = await operation()
        return result
      } catch (err) {
        handleError(err)
        setError(err instanceof Error ? err : new Error(String(err)))
        return undefined
      } finally {
        setLoading(false)
      }
    },
    [handleError],
  )

  return { execute, loading, error }
}

// Helper to create retryable errors
export function createRetryableError(
  message: string,
  retryFn: () => Promise<void>,
): ErrorWithRetry {
  const error = new Error(message) as ErrorWithRetry
  error.retry = retryFn
  return error
}
