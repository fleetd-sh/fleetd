"use client";

import { Suspense } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MetricsDashboard } from "@/components/metrics-dashboard";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ExternalLinkIcon, DashboardIcon, ActivityLogIcon } from "@radix-ui/react-icons";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

// Get Grafana URL from environment
const GRAFANA_URL = process.env.NEXT_PUBLIC_GRAFANA_URL || "http://localhost:3001";
const ENABLE_EMBEDDED_GRAFANA = process.env.NEXT_PUBLIC_ENABLE_GRAFANA === "true";

function GrafanaEmbed() {
  if (!ENABLE_EMBEDDED_GRAFANA) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>External Grafana</CardTitle>
          <CardDescription>
            Access the full Grafana dashboard for advanced analytics
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Alert>
            <AlertTitle>Grafana Dashboard</AlertTitle>
            <AlertDescription className="space-y-4">
              <p>
                For the full Grafana experience with advanced dashboards and alerting,
                access the external Grafana instance.
              </p>
              <div className="flex gap-2">
                <Button asChild>
                  <a href={GRAFANA_URL} target="_blank" rel="noopener noreferrer">
                    <DashboardIcon className="mr-2 h-4 w-4" />
                    Open Grafana
                    <ExternalLinkIcon className="ml-2 h-4 w-4" />
                  </a>
                </Button>
                <Button variant="outline" asChild>
                  <a href={`${GRAFANA_URL}/explore`} target="_blank" rel="noopener noreferrer">
                    <ActivityLogIcon className="mr-2 h-4 w-4" />
                    Explore Logs
                    <ExternalLinkIcon className="ml-2 h-4 w-4" />
                  </a>
                </Button>
              </div>
            </AlertDescription>
          </Alert>

          <div className="mt-6 space-y-4">
            <h3 className="text-lg font-semibold">Available Dashboards</h3>
            <div className="grid grid-cols-2 gap-4">
              <Card className="cursor-pointer hover:bg-muted/50" onClick={() => window.open(`${GRAFANA_URL}/d/fleet-overview`, "_blank")}>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Fleet Overview</CardTitle>
                  <CardDescription className="text-xs">
                    High-level fleet metrics and KPIs
                  </CardDescription>
                </CardHeader>
              </Card>

              <Card className="cursor-pointer hover:bg-muted/50" onClick={() => window.open(`${GRAFANA_URL}/d/device-metrics`, "_blank")}>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Device Metrics</CardTitle>
                  <CardDescription className="text-xs">
                    Detailed device performance data
                  </CardDescription>
                </CardHeader>
              </Card>

              <Card className="cursor-pointer hover:bg-muted/50" onClick={() => window.open(`${GRAFANA_URL}/d/api-performance`, "_blank")}>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">API Performance</CardTitle>
                  <CardDescription className="text-xs">
                    API latency and throughput
                  </CardDescription>
                </CardHeader>
              </Card>

              <Card className="cursor-pointer hover:bg-muted/50" onClick={() => window.open(`${GRAFANA_URL}/d/alerts`, "_blank")}>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Alerts & Issues</CardTitle>
                  <CardDescription className="text-xs">
                    Active alerts and incidents
                  </CardDescription>
                </CardHeader>
              </Card>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <Alert>
        <AlertTitle>Embedded Grafana</AlertTitle>
        <AlertDescription>
          The Grafana dashboard is embedded below. You can also{" "}
          <a
            href={GRAFANA_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="underline"
          >
            open it in a new tab
          </a>
          {" "}for full screen access.
        </AlertDescription>
      </Alert>

      <div className="border rounded-lg overflow-hidden">
        <iframe
          src={GRAFANA_URL}
          className="w-full h-[800px]"
          title="Grafana Dashboard"
          frameBorder="0"
        />
      </div>
    </div>
  );
}

export default function ObservabilityPage() {
  return (
    <div className="container mx-auto py-6 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Observability</h1>
        <p className="text-muted-foreground mt-2">
          Comprehensive metrics, logs, and traces for your fleet
        </p>
      </div>

      <Tabs defaultValue="metrics" className="space-y-4">
        <TabsList>
          <TabsTrigger value="metrics">Metrics Dashboard</TabsTrigger>
          <TabsTrigger value="grafana">Grafana</TabsTrigger>
          <TabsTrigger value="integrations">Integrations</TabsTrigger>
        </TabsList>

        <TabsContent value="metrics" className="space-y-4">
          <Suspense fallback={
            <div className="flex items-center justify-center h-96">
              <div className="animate-spin h-8 w-8 border-4 border-primary border-t-transparent rounded-full" />
            </div>
          }>
            <MetricsDashboard />
          </Suspense>
        </TabsContent>

        <TabsContent value="grafana" className="space-y-4">
          <GrafanaEmbed />
        </TabsContent>

        <TabsContent value="integrations" className="space-y-4">
          <div className="grid grid-cols-3 gap-4">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <div className="h-10 w-10 bg-blue-100 dark:bg-blue-900 rounded flex items-center justify-center">
                    <span className="text-xl font-bold text-blue-600 dark:text-blue-400">VM</span>
                  </div>
                  <div>
                    <CardTitle className="text-lg">VictoriaMetrics</CardTitle>
                    <CardDescription className="text-xs">
                      Time-series database
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Status</span>
                    <span className="text-green-600">Connected</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Endpoint</span>
                    <span className="font-mono text-xs">localhost:8428</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Retention</span>
                    <span>30 days</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <div className="h-10 w-10 bg-green-100 dark:bg-green-900 rounded flex items-center justify-center">
                    <span className="text-xl font-bold text-green-600 dark:text-green-400">L</span>
                  </div>
                  <div>
                    <CardTitle className="text-lg">Loki</CardTitle>
                    <CardDescription className="text-xs">
                      Log aggregation
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Status</span>
                    <span className="text-green-600">Connected</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Endpoint</span>
                    <span className="font-mono text-xs">localhost:3100</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Ingestion Rate</span>
                    <span>~1.2 MB/s</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <div className="h-10 w-10 bg-orange-100 dark:bg-orange-900 rounded flex items-center justify-center">
                    <span className="text-xl font-bold text-orange-600 dark:text-orange-400">G</span>
                  </div>
                  <div>
                    <CardTitle className="text-lg">Grafana</CardTitle>
                    <CardDescription className="text-xs">
                      Visualization platform
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Status</span>
                    <span className="text-green-600">Available</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Version</span>
                    <span>v10.2.0</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Dashboards</span>
                    <span>12 active</span>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Data Sources</CardTitle>
              <CardDescription>
                Configure how observability data flows through the system
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div>
                    <h4 className="font-medium">Prometheus Remote Write</h4>
                    <p className="text-sm text-muted-foreground">
                      Devices push metrics directly to VictoriaMetrics
                    </p>
                  </div>
                  <Button variant="outline" size="sm">Configure</Button>
                </div>

                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div>
                    <h4 className="font-medium">Loki Push API</h4>
                    <p className="text-sm text-muted-foreground">
                      Logs are streamed in real-time to Loki
                    </p>
                  </div>
                  <Button variant="outline" size="sm">Configure</Button>
                </div>

                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div>
                    <h4 className="font-medium">OpenTelemetry</h4>
                    <p className="text-sm text-muted-foreground">
                      Distributed tracing support (coming soon)
                    </p>
                  </div>
                  <Button variant="outline" size="sm" disabled>Coming Soon</Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}