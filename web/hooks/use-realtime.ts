'use client'

import { useToast } from '@/hooks/use-toast'
import { useSSE } from '@/lib/sse'
import { useQueryClient } from '@tanstack/react-query'

export function useRealtime() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  useSSE('/api/v1/events', {
    onMessage: (data) => {
      switch (data.type) {
        case 'device_update':
          // Invalidate device queries to refetch latest data
          queryClient.invalidateQueries({ queryKey: ['devices'] })
          if (data.device_id) {
            queryClient.invalidateQueries({ queryKey: ['device', data.device_id] })
          }
          break

        case 'telemetry_update':
          // Invalidate telemetry queries
          queryClient.invalidateQueries({ queryKey: ['telemetry'] })
          break

        case 'device_connected':
          toast({
            title: 'Device Connected',
            description: `${data.device_name || data.device_id} is now online`,
          })
          queryClient.invalidateQueries({ queryKey: ['devices'] })
          break

        case 'device_disconnected':
          toast({
            title: 'Device Disconnected',
            description: `${data.device_name || data.device_id} is now offline`,
            variant: 'destructive',
          })
          queryClient.invalidateQueries({ queryKey: ['devices'] })
          break

        case 'update_available':
          toast({
            title: 'Update Available',
            description: `New update ${data.version} is available for deployment`,
          })
          queryClient.invalidateQueries({ queryKey: ['updates'] })
          break

        default:
          console.log('Unknown SSE event type:', data.type)
      }
    },
    onError: () => {
      console.error('Lost connection to server, attempting to reconnect...')
    },
    onOpen: () => {
      console.log('Connected to real-time events')
    },
  })
}
