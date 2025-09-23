import { Suspense } from "react";
import { SettingsContent } from "@/components/settings-content";
import { LoadingSkeleton } from "@/components/loading-states";

export const metadata = {
  title: "Settings | fleetd",
  description: "Configure system settings and preferences",
};

export default function SettingsPage() {
  return (
    <main className="container mx-auto px-4 py-8">
      <div className="mb-8">
        <h1 className="text-4xl font-bold tracking-tight">Settings</h1>
        <p className="text-muted-foreground mt-2">
          Configure system settings and preferences
        </p>
      </div>

      <Suspense fallback={<LoadingSkeleton />}>
        <SettingsContent />
      </Suspense>
    </main>
  );
}