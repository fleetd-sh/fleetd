import { DashboardContent } from '@/components/dashboard-content'
import { DashboardSkeleton } from '@/components/dashboard-skeleton'
import { ErrorBoundary } from '@/components/error-boundary'
import { api } from '@/lib/api'
import { Suspense } from 'react'

export const dynamic = 'force-dynamic'
export const revalidate = 10

async function getInitialData() {
  try {
    const [devices, telemetry] = await Promise.all([api.getDevices(), api.getMetrics(10)])

    return { devices, telemetry }
  } catch (error) {
    console.error('Failed to fetch initial data:', error)
    return { devices: [], telemetry: [] }
  }
}

export default async function DashboardPage() {
  const initialData = await getInitialData()

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="container mx-auto px-4 py-6">
          <h1 className="text-3xl font-bold">FleetD Management Dashboard</h1>
          <p className="text-muted-foreground mt-2">
            Monitor and manage your fleet of devices in real-time
          </p>
        </div>
      </header>

      <main className="container mx-auto px-4 py-8">
        <ErrorBoundary>
          <Suspense fallback={<DashboardSkeleton />}>
            <DashboardContent initialData={initialData} />
          </Suspense>
        </ErrorBoundary>
      </main>
    </div>
  )
}
