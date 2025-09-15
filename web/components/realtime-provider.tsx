'use client'

import { useRealtime } from '@/hooks/use-realtime'

export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  useRealtime()
  return <>{children}</>
}
