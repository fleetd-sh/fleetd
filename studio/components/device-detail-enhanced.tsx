"use client";
import {
  CubeIcon,
  DesktopIcon,
  GlobeIcon,
  LightningBoltIcon,
  ReaderIcon,
  RocketIcon,
  UpdateIcon,
} from "@radix-ui/react-icons";
import { format } from "date-fns";
import * as React from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { Device } from "@/lib/types";

interface DeviceDetailEnhancedProps {
  device: Device;
  onDeploy?: () => void;
  onRestart?: () => void;
  onUpdate?: () => void;
}
export function DeviceDetailEnhanced({
  device,
  onDeploy,
  onRestart,
  onUpdate,
}: DeviceDetailEnhancedProps) {
  const [systemMetrics] = React.useState({
    cpu: Math.random() * 100,
    memory: Math.random() * 100,
    disk: Math.random() * 100,
    network: Math.random() * 100,
  });
  return (
    <div className="space-y-6">
      {/* Device Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          <h2 className="text-xl-atlas font-bold tracking-tight">{device.name}</h2>
          <div className="flex items-center gap-2 text-sm-atlas text-muted-foreground">
            <Badge variant={device.status === "online" ? "default" : "secondary"}>
              {device.status}
            </Badge>
            <span>•</span>
            <span>ID: {device.id}</span>
            <span>•</span>
            <span>Last seen: {format(new Date(device.last_seen), "PPpp")}</span>
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={onUpdate}>
            <UpdateIcon className="mr-2 h-4 w-4" />
            Update
          </Button>
          <Button variant="outline" size="sm" onClick={onRestart}>
            <ReaderIcon className="mr-2 h-4 w-4" />
            Restart
          </Button>
          <Button size="sm" onClick={onDeploy}>
            <RocketIcon className="mr-2 h-4 w-4" />
            Deploy
          </Button>
        </div>
      </div>
      {/* Device Tabs */}
      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList className="grid w-full grid-cols-5 lg:w-[500px]">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="metrics">Metrics</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="deployments">Deploy</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {/* CPU Card */}
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm-atlas font-medium">CPU Usage</CardTitle>
                <LightningBoltIcon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-xl-atlas font-bold">{systemMetrics.cpu.toFixed(1)}%</div>
                <Progress value={systemMetrics.cpu} className="mt-2" />
              </CardContent>
            </Card>
            {/* Memory Card */}
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm-atlas font-medium">Memory</CardTitle>
                <CubeIcon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-xl-atlas font-bold">{systemMetrics.memory.toFixed(1)}%</div>
                <Progress value={systemMetrics.memory} className="mt-2" />
              </CardContent>
            </Card>
            {/* Disk Card */}
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm-atlas font-medium">Disk Usage</CardTitle>
                <DesktopIcon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-xl-atlas font-bold">{systemMetrics.disk.toFixed(1)}%</div>
                <Progress value={systemMetrics.disk} className="mt-2" />
              </CardContent>
            </Card>
            {/* Network Card */}
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm-atlas font-medium">Network</CardTitle>
                <GlobeIcon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-xl-atlas font-bold">
                  {systemMetrics.network.toFixed(1)} Mb/s
                </div>
                <Progress value={systemMetrics.network} className="mt-2" />
              </CardContent>
            </Card>
          </div>
          {/* Device Information */}
          <Card>
            <CardHeader>
              <CardTitle>Device Information</CardTitle>
              <CardDescription>Detailed information about this device</CardDescription>
            </CardHeader>
            <CardContent>
              <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">Device Type</dt>
                  <dd className="text-sm-atlas">{device.type}</dd>
                </div>
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">Version</dt>
                  <dd className="text-sm-atlas">v{device.version}</dd>
                </div>
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">IP Address</dt>
                  <dd className="text-sm-atlas font-mono">
                    192.168.1.{Math.floor(Math.random() * 255)}
                  </dd>
                </div>
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">MAC Address</dt>
                  <dd className="text-sm-atlas font-mono">AA:BB:CC:DD:EE:FF</dd>
                </div>
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">Uptime</dt>
                  <dd className="text-sm-atlas">7 days, 14 hours</dd>
                </div>
                <div>
                  <dt className="text-sm-atlas font-medium text-muted-foreground">Location</dt>
                  <dd className="text-sm-atlas">Zone A, Rack 3</dd>
                </div>
              </dl>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="metrics" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Performance Metrics</CardTitle>
              <CardDescription>Real-time performance monitoring</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px] flex items-center justify-center text-muted-foreground">
                Performance charts would be displayed here
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="logs" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>System Logs</CardTitle>
              <CardDescription>Recent activity and events</CardDescription>
            </CardHeader>
            <CardContent>
              <ScrollArea className="h-[400px] w-full rounded-md border p-4">
                <div className="space-y-2 font-mono text-sm">
                  <div className="flex gap-2">
                    <span className="text-muted-foreground">
                      [{format(new Date(), "HH:mm:ss")}]
                    </span>
                    <span className="text-green-600">INFO</span>
                    <span>Device connected successfully</span>
                  </div>
                  <div className="flex gap-2">
                    <span className="text-muted-foreground">
                      [{format(new Date(), "HH:mm:ss")}]
                    </span>
                    <span className="text-blue-600">DEBUG</span>
                    <span>Telemetry data transmitted</span>
                  </div>
                  <div className="flex gap-2">
                    <span className="text-muted-foreground">
                      [{format(new Date(), "HH:mm:ss")}]
                    </span>
                    <span className="text-green-600">INFO</span>
                    <span>Health check passed</span>
                  </div>
                  <div className="flex gap-2">
                    <span className="text-muted-foreground">
                      [{format(new Date(), "HH:mm:ss")}]
                    </span>
                    <span className="text-yellow-600">WARN</span>
                    <span>High memory usage detected (85%)</span>
                  </div>
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="deployments" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Deployment Management</CardTitle>
              <CardDescription>Deploy and manage applications on this device</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-semibold">Application v2.1.0</h4>
                      <p className="text-sm text-muted-foreground">Ready to deploy</p>
                    </div>
                    <Button size="sm">Deploy Now</Button>
                  </div>
                </div>
                <div className="rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-semibold">Firmware Update 3.0.0</h4>
                      <p className="text-sm text-muted-foreground">Available for installation</p>
                    </div>
                    <Button size="sm" variant="outline">
                      Schedule Update
                    </Button>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="settings" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Device Settings</CardTitle>
              <CardDescription>Configure device parameters and preferences</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium">Auto-update</p>
                    <p className="text-sm text-muted-foreground">Automatically install updates</p>
                  </div>
                  <Button variant="outline" size="sm">
                    Configure
                  </Button>
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium">Monitoring Interval</p>
                    <p className="text-sm text-muted-foreground">Currently set to 30 seconds</p>
                  </div>
                  <Button variant="outline" size="sm">
                    Change
                  </Button>
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium">Alert Thresholds</p>
                    <p className="text-sm text-muted-foreground">Customize alert conditions</p>
                  </div>
                  <Button variant="outline" size="sm">
                    Edit
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
