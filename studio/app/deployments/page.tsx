import { Suspense } from "react";
import { DeploymentsContent } from "@/components/deployments-content";
import { LoadingSkeleton } from "@/components/loading-states";

export const metadata = {
  title: "Deployments | fleetd",
  description: "Deploy and manage applications across your fleet",
};

export default function DeploymentsPage() {
  return (
    <main className="container mx-auto px-4 py-8">
      <div className="mb-8">
        <h1 className="text-4xl font-bold tracking-tight">Deployments</h1>
        <p className="text-muted-foreground mt-2">
          Deploy and manage applications across your fleet
        </p>
      </div>

      <Suspense fallback={<LoadingSkeleton />}>
        <DeploymentsContent />
      </Suspense>
    </main>
  );
}