"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ActivityLogIcon,
  BarChartIcon,
  ClockIcon,
  DownloadIcon,
  MixerHorizontalIcon,
  ReloadIcon,
} from "@radix-ui/react-icons";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { api } from "@/lib/api";
import { format } from "date-fns";
import { useSonnerToast } from "@/hooks/use-sonner-toast";
import { useTelemetryClient } from "@/lib/api/connect-hooks";
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";

export function TelemetryContent() {
  const { toast } = useSonnerToast();
  const [selectedDevice, setSelectedDevice] = React.useState<string>("all");
  const [timeRange, setTimeRange] = React.useState("1h");
  const [logFilter, setLogFilter] = React.useState("");
  const [refreshInterval, setRefreshInterval] = React.useState(30);

  const telemetryClient = useTelemetryClient();

  const { data: devices = [] } = useQuery({
    queryKey: ["devices"],
    queryFn: api.getDevices,
  });

  // Mock telemetry data
  const mockMetrics = React.useMemo(() => {
    const now = Date.now();
    return Array.from({ length: 20 }, (_, i) => ({
      time: format(new Date(now - (19 - i) * 60000), "HH:mm"),
      cpu: Math.random() * 100,
      memory: Math.random() * 100,
      disk: 20 + Math.random() * 60,
      network: Math.random() * 100,
      temperature: 35 + Math.random() * 15,
    }));
  }, [timeRange]);

  const mockLogs = React.useMemo(() => {
    const levels = ["INFO", "DEBUG", "WARN", "ERROR"];
    const messages = [
      "Device connected successfully",
      "Telemetry data transmitted",
      "Health check passed",
      "Configuration updated",
      "High memory usage detected",
      "Network latency spike",
      "Deployment completed",
      "Cache cleared",
      "Service restarted",
    ];

    return Array.from({ length: 50 }, (_, i) => ({
      id: `log-${i}`,
      timestamp: new Date(Date.now() - i * 30000),
      level: levels[Math.floor(Math.random() * levels.length)],
      device: devices[Math.floor(Math.random() * devices.length)]?.name || "device-001",
      message: messages[Math.floor(Math.random() * messages.length)],
    }));
  }, [devices]);

  const { data: metrics = mockMetrics, refetch: refetchMetrics } = useQuery({
    queryKey: ["telemetry", selectedDevice, timeRange],
    queryFn: async () => {
      try {
        const response = await telemetryClient.getTelemetry({
          deviceId: selectedDevice === "all" ? "" : selectedDevice,
          limit: 20,
        });

        // Transform the response data to match our chart format
        return response.data.map(d => ({
          time: format(new Date(d.timestamp?.seconds ? d.timestamp.seconds * 1000 : Date.now()), "HH:mm"),
          cpu: d.cpuUsage || 0,
          memory: d.memoryUsage || 0,
          disk: d.diskUsage || 0,
          network: d.networkUsage || 0,
          temperature: d.temperature || 0,
        }));
      } catch (error) {
        console.error("Failed to fetch telemetry:", error);
        return mockMetrics; // Fallback to mock data
      }
    },
    refetchInterval: refreshInterval * 1000,
  });

  const { data: logs = mockLogs, refetch: refetchLogs } = useQuery({
    queryKey: ["logs", selectedDevice, logFilter],
    queryFn: async () => {
      try {
        const response = await telemetryClient.getLogs({
          deviceIds: selectedDevice === "all" ? [] : [selectedDevice],
          filter: logFilter,
          limit: 50,
        });

        // Transform the response data to match our log format
        return response.logs.map(log => ({
          id: log.id || `log-${Date.now()}`,
          timestamp: new Date(log.timestamp?.seconds ? log.timestamp.seconds * 1000 : Date.now()),
          level: log.level?.toString() || "INFO",
          device: log.deviceId || "unknown",
          message: log.message || "",
        }));
      } catch (error) {
        console.error("Failed to fetch logs:", error);
        return mockLogs; // Fallback to mock data
      }
    },
    refetchInterval: refreshInterval * 1000,
  });

  const filteredLogs = React.useMemo(() => {
    if (!logFilter) return logs;
    return logs.filter(log =>
      log.message.toLowerCase().includes(logFilter.toLowerCase()) ||
      log.device.toLowerCase().includes(logFilter.toLowerCase()) ||
      log.level.toLowerCase().includes(logFilter.toLowerCase())
    );
  }, [logs, logFilter]);

  const handleExportData = () => {
    const data = {
      metrics,
      logs: filteredLogs,
      timestamp: new Date().toISOString(),
      device: selectedDevice,
      timeRange,
    };

    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `telemetry-${new Date().toISOString()}.json`;
    a.click();
    URL.revokeObjectURL(url);

    toast.success("Telemetry data exported");
  };

  const getLevelColor = (level: string) => {
    switch (level) {
      case "ERROR":
        return "text-red-600";
      case "WARN":
        return "text-yellow-600";
      case "INFO":
        return "text-blue-600";
      case "DEBUG":
        return "text-gray-600";
      default:
        return "text-gray-600";
    }
  };

  return (
    <div className="space-y-6">
      {/* Controls */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Telemetry Controls</CardTitle>
            <div className="flex gap-2">
              <Button
                onClick={() => {
                  refetchMetrics();
                  refetchLogs();
                  toast.success("Data refreshed");
                }}
                variant="outline"
                size="sm"
              >
                <ReloadIcon className="mr-2 h-4 w-4" />
                Refresh
              </Button>
              <Button onClick={handleExportData} variant="outline" size="sm">
                <DownloadIcon className="mr-2 h-4 w-4" />
                Export
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-4">
            <div className="space-y-2">
              <Label>Device</Label>
              <Select value={selectedDevice} onValueChange={setSelectedDevice}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Devices</SelectItem>
                  {devices.map((device) => (
                    <SelectItem key={device.id} value={device.id}>
                      {device.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Time Range</Label>
              <Select value={timeRange} onValueChange={setTimeRange}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1h">Last Hour</SelectItem>
                  <SelectItem value="6h">Last 6 Hours</SelectItem>
                  <SelectItem value="24h">Last 24 Hours</SelectItem>
                  <SelectItem value="7d">Last 7 Days</SelectItem>
                  <SelectItem value="30d">Last 30 Days</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Refresh Interval</Label>
              <div className="flex items-center gap-2">
                <Slider
                  value={[refreshInterval]}
                  onValueChange={([value]) => setRefreshInterval(value)}
                  min={5}
                  max={60}
                  step={5}
                  className="flex-1"
                />
                <span className="text-sm w-12">{refreshInterval}s</span>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Log Filter</Label>
              <Input
                placeholder="Filter logs..."
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Metrics Dashboard */}
      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="alerts">Alerts</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            {/* CPU & Memory Chart */}
            <Card>
              <CardHeader>
                <CardTitle>CPU & Memory Usage</CardTitle>
                <CardDescription>System resource utilization</CardDescription>
              </CardHeader>
              <CardContent>
                <ResponsiveContainer width="100%" height={300}>
                  <LineChart data={metrics}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Line
                      type="monotone"
                      dataKey="cpu"
                      stroke="#3b82f6"
                      name="CPU %"
                      strokeWidth={2}
                    />
                    <Line
                      type="monotone"
                      dataKey="memory"
                      stroke="#10b981"
                      name="Memory %"
                      strokeWidth={2}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            {/* Network Chart */}
            <Card>
              <CardHeader>
                <CardTitle>Network Activity</CardTitle>
                <CardDescription>Bandwidth usage over time</CardDescription>
              </CardHeader>
              <CardContent>
                <ResponsiveContainer width="100%" height={300}>
                  <AreaChart data={metrics}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Area
                      type="monotone"
                      dataKey="network"
                      stroke="#8b5cf6"
                      fill="#8b5cf6"
                      fillOpacity={0.3}
                      name="Network Mb/s"
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            {/* Disk Usage Chart */}
            <Card>
              <CardHeader>
                <CardTitle>Disk Usage</CardTitle>
                <CardDescription>Storage utilization</CardDescription>
              </CardHeader>
              <CardContent>
                <ResponsiveContainer width="100%" height={300}>
                  <BarChart data={metrics}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Bar dataKey="disk" fill="#f59e0b" name="Disk %" />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>

            {/* Temperature Chart */}
            <Card>
              <CardHeader>
                <CardTitle>Temperature</CardTitle>
                <CardDescription>Device temperature monitoring</CardDescription>
              </CardHeader>
              <CardContent>
                <ResponsiveContainer width="100%" height={300}>
                  <LineChart data={metrics}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Line
                      type="monotone"
                      dataKey="temperature"
                      stroke="#ef4444"
                      name="Temperature °C"
                      strokeWidth={2}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Performance Metrics</CardTitle>
              <CardDescription>Detailed performance analysis</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px] flex items-center justify-center text-muted-foreground">
                Advanced performance metrics coming soon
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logs" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>System Logs</CardTitle>
              <CardDescription>
                Recent activity from {selectedDevice === "all" ? "all devices" : selectedDevice}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <ScrollArea className="h-[500px] w-full rounded-md border p-4">
                <div className="space-y-2">
                  {filteredLogs.map((log) => (
                    <div key={log.id} className="flex gap-3 font-mono text-sm">
                      <span className="text-muted-foreground">
                        {format(log.timestamp, "HH:mm:ss")}
                      </span>
                      <Badge variant="outline" className={getLevelColor(log.level)}>
                        {log.level}
                      </Badge>
                      <span className="text-muted-foreground">[{log.device}]</span>
                      <span>{log.message}</span>
                    </div>
                  ))}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="alerts" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Alert Configuration</CardTitle>
              <CardDescription>Set up monitoring alerts and thresholds</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-medium">High CPU Usage</h4>
                      <p className="text-sm text-muted-foreground">Alert when CPU > 80%</p>
                    </div>
                    <Badge>Active</Badge>
                  </div>
                </div>
                <div className="rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-medium">Low Disk Space</h4>
                      <p className="text-sm text-muted-foreground">Alert when disk > 90%</p>
                    </div>
                    <Badge>Active</Badge>
                  </div>
                </div>
                <div className="rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="font-medium">Device Offline</h4>
                      <p className="text-sm text-muted-foreground">Alert when device disconnects</p>
                    </div>
                    <Badge>Active</Badge>
                  </div>
                </div>
                <Button className="w-full">
                  <MixerHorizontalIcon className="mr-2 h-4 w-4" />
                  Configure Alerts
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}