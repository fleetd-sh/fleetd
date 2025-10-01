"use client";
import {
  CpuIcon,
  HardDriveIcon,
  MemoryStickIcon,
  NetworkIcon,
  ThermometerIcon,
  ZapIcon,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import type { Device } from "@/lib/api/gen/public/v1/fleet_pb";

interface TelemetryData {
  timestamp: number;
  cpuUsage: number;
  memoryUsed: number;
  memoryTotal: number;
  diskUsed: number;
  diskTotal: number;
  networkRx: number;
  networkTx: number;
  temperature?: number;
  processCount?: number;
  loadAvg1?: number;
  loadAvg5?: number;
  loadAvg15?: number;
}
interface DeviceTelemetryProps {
  device: Device;
  realtime?: boolean;
}
export function DeviceTelemetry({ device, realtime = false }: DeviceTelemetryProps) {
  const [telemetryHistory, setTelemetryHistory] = useState<TelemetryData[]>([]);
  const [currentMetrics, setCurrentMetrics] = useState<TelemetryData | null>(null);
  // Simulate real-time data (in production, this would be WebSocket/SSE)
  useEffect(() => {
    if (!realtime) return;
    const interval = setInterval(() => {
      const newData: TelemetryData = {
        timestamp: Date.now(),
        cpuUsage: Math.random() * 100,
        memoryUsed: device.systemInfo?.memoryTotal
          ? Number(device.systemInfo.memoryTotal) * (0.3 + Math.random() * 0.5)
          : 0,
        memoryTotal: Number(device.systemInfo?.memoryTotal || 0),
        diskUsed: device.systemInfo?.storageTotal
          ? Number(device.systemInfo.storageTotal) * (0.2 + Math.random() * 0.3)
          : 0,
        diskTotal: Number(device.systemInfo?.storageTotal || 0),
        networkRx: Math.random() * 1000000,
        networkTx: Math.random() * 500000,
        temperature: 25 + Math.random() * 35,
        processCount: device.systemInfo?.processCount || 0,
        loadAvg1: (device.systemInfo?.loadAverage?.load1 || 0) + (Math.random() - 0.5),
        loadAvg5: device.systemInfo?.loadAverage?.load5 || 0,
        loadAvg15: device.systemInfo?.loadAverage?.load15 || 0,
      };
      setCurrentMetrics(newData);
      setTelemetryHistory((prev) =>
        [...prev.slice(-59), newData].map((d, i) => ({
          ...d,
          time: `${60 - i}s`,
        })),
      );
    }, 1000);
    return () => clearInterval(interval);
  }, [device, realtime]);
  // Calculate percentages and format values
  const metrics = useMemo(() => {
    if (!currentMetrics) {
      return {
        cpuPercent: 0,
        memoryPercent: 0,
        diskPercent: 0,
        memoryUsedGB: "0",
        memoryTotalGB: "0",
        diskUsedGB: "0",
        diskTotalGB: "0",
      };
    }
    return {
      cpuPercent: Math.round(currentMetrics.cpuUsage),
      memoryPercent: currentMetrics.memoryTotal
        ? Math.round((currentMetrics.memoryUsed / currentMetrics.memoryTotal) * 100)
        : 0,
      diskPercent: currentMetrics.diskTotal
        ? Math.round((currentMetrics.diskUsed / currentMetrics.diskTotal) * 100)
        : 0,
      memoryUsedGB: formatBytes(currentMetrics.memoryUsed),
      memoryTotalGB: formatBytes(currentMetrics.memoryTotal),
      diskUsedGB: formatBytes(currentMetrics.diskUsed),
      diskTotalGB: formatBytes(currentMetrics.diskTotal),
    };
  }, [currentMetrics]);
  // Determine health status
  const getHealthStatus = () => {
    if (!currentMetrics) return "unknown";
    if (metrics.cpuPercent > 90 || metrics.memoryPercent > 90) return "critical";
    if (metrics.cpuPercent > 70 || metrics.memoryPercent > 70) return "warning";
    return "healthy";
  };
  const healthStatus = getHealthStatus();
  return (
    <div className="space-y-6">
      {/* Current Metrics Grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm flex items-center gap-2">
              <CpuIcon className="h-4 w-4" />
              CPU Usage
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{metrics.cpuPercent}%</div>
            <Progress value={metrics.cpuPercent} className="mt-2 h-1" />
            <div className="text-xs text-muted-foreground mt-1">
              {device.systemInfo?.cpuCores || 0} cores • {device.systemInfo?.cpuModel}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm flex items-center gap-2">
              <MemoryStickIcon className="h-4 w-4" />
              Memory
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{metrics.memoryPercent}%</div>
            <Progress value={metrics.memoryPercent} className="mt-2 h-1" />
            <div className="text-xs text-muted-foreground mt-1">
              {metrics.memoryUsedGB} / {metrics.memoryTotalGB}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm flex items-center gap-2">
              <HardDriveIcon className="h-4 w-4" />
              Storage
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{metrics.diskPercent}%</div>
            <Progress value={metrics.diskPercent} className="mt-2 h-1" />
            <div className="text-xs text-muted-foreground mt-1">
              {metrics.diskUsedGB} / {metrics.diskTotalGB}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm flex items-center gap-2">
              <ThermometerIcon className="h-4 w-4" />
              Temperature
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {currentMetrics?.temperature?.toFixed(1) || "--"}°C
            </div>
            <div className="text-xs text-muted-foreground mt-1">
              {currentMetrics?.temperature && currentMetrics.temperature > 60 ? (
                <Badge variant="destructive" className="text-xs">
                  High
                </Badge>
              ) : (
                <Badge variant="outline" className="text-xs">
                  Normal
                </Badge>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
      {/* System Load & Processes */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">System Load & Processes</CardTitle>
          <CardDescription>Load average and process information</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-3 gap-4">
            <div>
              <div className="text-sm text-muted-foreground">1 min avg</div>
              <div className="text-xl font-semibold">
                {currentMetrics?.loadAvg1?.toFixed(2) || "--"}
              </div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">5 min avg</div>
              <div className="text-xl font-semibold">
                {currentMetrics?.loadAvg5?.toFixed(2) || "--"}
              </div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">15 min avg</div>
              <div className="text-xl font-semibold">
                {currentMetrics?.loadAvg15?.toFixed(2) || "--"}
              </div>
            </div>
          </div>
          <div className="flex items-center justify-between pt-2 border-t">
            <span className="text-sm text-muted-foreground">Running Processes</span>
            <span className="font-semibold">{currentMetrics?.processCount || "--"}</span>
          </div>
        </CardContent>
      </Card>
      {/* Real-time Charts */}
      {realtime && telemetryHistory.length > 0 && (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">CPU & Memory Usage</CardTitle>
              <CardDescription>Last 60 seconds</CardDescription>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={200}>
                <LineChart data={telemetryHistory}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis dataKey="time" className="text-xs" />
                  <YAxis className="text-xs" domain={[0, 100]} />
                  <Tooltip />
                  <Line
                    type="monotone"
                    dataKey="cpuUsage"
                    stroke="hsl(var(--primary))"
                    strokeWidth={2}
                    dot={false}
                    name="CPU %"
                  />
                  <Line
                    type="monotone"
                    dataKey={(d) => (d.memoryTotal ? (d.memoryUsed / d.memoryTotal) * 100 : 0)}
                    stroke="hsl(var(--secondary))"
                    strokeWidth={2}
                    dot={false}
                    name="Memory %"
                  />
                </LineChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Network Traffic</CardTitle>
              <CardDescription>Bandwidth usage over time</CardDescription>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={200}>
                <AreaChart data={telemetryHistory}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis dataKey="time" className="text-xs" />
                  <YAxis className="text-xs" tickFormatter={(v) => formatBytes(v)} />
                  <Tooltip formatter={(v: number) => formatBytes(v)} />
                  <Area
                    type="monotone"
                    dataKey="networkRx"
                    stackId="1"
                    stroke="hsl(var(--primary))"
                    fill="hsl(var(--primary))"
                    fillOpacity={0.3}
                    name="RX"
                  />
                  <Area
                    type="monotone"
                    dataKey="networkTx"
                    stackId="1"
                    stroke="hsl(var(--secondary))"
                    fill="hsl(var(--secondary))"
                    fillOpacity={0.3}
                    name="TX"
                  />
                </AreaChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </>
      )}
      {/* Network Interfaces */}
      {device.systemInfo?.networkInterfaces && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm flex items-center gap-2">
              <NetworkIcon className="h-4 w-4" />
              Network Interfaces
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {device.systemInfo.networkInterfaces.map((iface, idx) => (
                <div
                  key={`network-${iface.name}-${idx}`}
                  className="flex items-center justify-between p-2 rounded-lg bg-muted/50"
                >
                  <div>
                    <div className="font-medium text-sm">{iface.name}</div>
                    <div className="text-xs text-muted-foreground">{iface.macAddress}</div>
                    <div className="text-xs mt-1">{iface.ipAddresses?.join(", ") || "No IP"}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={iface.isUp ? "default" : "secondary"} className="text-xs">
                      {iface.isUp ? "UP" : "DOWN"}
                    </Badge>
                    {iface.isLoopback && (
                      <Badge variant="outline" className="text-xs">
                        Loopback
                      </Badge>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
      {/* Health Status Summary */}
      <Card
        className={
          healthStatus === "critical"
            ? "border-red-500"
            : healthStatus === "warning"
              ? "border-yellow-500"
              : "border-green-500"
        }
      >
        <CardHeader>
          <CardTitle className="text-sm flex items-center gap-2">
            <ZapIcon className="h-4 w-4" />
            System Health
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <span className="text-sm">Overall Status</span>
            <Badge
              variant={
                healthStatus === "critical"
                  ? "destructive"
                  : healthStatus === "warning"
                    ? "secondary"
                    : "default"
              }
            >
              {healthStatus.toUpperCase()}
            </Badge>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
function formatBytes(bytes: number): string {
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  if (bytes === 0) return "0 B";
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / 1024 ** i).toFixed(1)} ${sizes[i]}`;
}
