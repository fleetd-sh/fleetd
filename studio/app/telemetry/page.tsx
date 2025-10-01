import { Suspense } from "react";
import { DashboardLoadingSkeleton } from "@/components/loading-states";
import { TelemetryContent } from "@/components/telemetry-content";

export const metadata = {
  title: "Telemetry | fleetd",
  description: "Monitor device metrics, logs, and performance data",
};

export default function TelemetryPage() {
  return (
    <main className="container mx-auto px-4 py-8">
      <div className="mb-8">
        <h1 className="text-4xl font-bold tracking-tight">Telemetry</h1>
        <p className="text-muted-foreground mt-2">
          Monitor device metrics, logs, and performance data
        </p>
      </div>

      <Suspense fallback={<DashboardLoadingSkeleton />}>
        <TelemetryContent />
      </Suspense>
    </main>
  );
}
