import { Suspense } from "react";
import { DashboardContent } from "@/components/dashboard-content";
import { DashboardSkeleton } from "@/components/dashboard-skeleton";
import { ErrorBoundary } from "@/components/error-boundary";
import { api } from "@/lib/api";

export const dynamic = "force-dynamic";
export const revalidate = 10;

async function getInitialData() {
  try {
    const [devices, telemetry] = await Promise.all([api.getDevices(), api.getMetrics(10)]);

    return { devices, telemetry };
  } catch (error) {
    console.error("Failed to fetch initial data:", error);
    return { devices: [], telemetry: [] };
  }
}

export default async function DashboardPage() {
  const initialData = await getInitialData();

  return (
    <main className="container mx-auto px-4 py-8">
      <div className="mb-8">
        <h1 className="text-4xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground mt-2">
          Monitor and manage your fleet of devices in real-time
        </p>
      </div>

      <ErrorBoundary>
        <Suspense fallback={<DashboardSkeleton />}>
          <DashboardContent initialData={initialData} />
        </Suspense>
      </ErrorBoundary>
    </main>
  );
}
