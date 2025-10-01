import { Suspense } from "react";
import { DevicesContent } from "@/components/devices-content";
import { DashboardLoadingSkeleton } from "@/components/loading-states";

export const metadata = {
  title: "Devices | fleetd",
  description: "Manage and monitor all devices in your fleet",
};

export default function DevicesPage() {
  return (
    <main className="container mx-auto px-4 py-8">
      <div className="mb-8">
        <h1 className="text-4xl font-bold tracking-tight">Devices</h1>
        <p className="text-muted-foreground mt-2">Manage and monitor all devices in your fleet</p>
      </div>

      <Suspense fallback={<DashboardLoadingSkeleton />}>
        <DevicesContent />
      </Suspense>
    </main>
  );
}
