"use client";

import {
  ArrowDownIcon,
  ArrowUpIcon,
  BarChartIcon,
  CheckCircledIcon,
  CrossCircledIcon,
  DownloadIcon,
  MagnifyingGlassIcon,
  MixerHorizontalIcon,
  ReloadIcon,
} from "@radix-ui/react-icons";
import { useQuery } from "@tanstack/react-query";
import { format, subDays, subHours, subMinutes } from "date-fns";
import { useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  Brush,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Scatter,
  ScatterChart,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

// VictoriaMetrics API client
class VictoriaMetricsClient {
  constructor(private baseUrl = "/api/vm") {}

  async query(query: string, time?: Date): Promise<any> {
    const params = new URLSearchParams({ query });
    if (time) params.append("time", Math.floor(time.getTime() / 1000).toString());

    const response = await fetch(`${this.baseUrl}/query?${params}`);
    if (!response.ok) throw new Error("Failed to fetch metrics");
    return response.json();
  }

  async queryRange(query: string, start: Date, end: Date, step: string = "60s"): Promise<any> {
    const params = new URLSearchParams({
      query,
      start: Math.floor(start.getTime() / 1000).toString(),
      end: Math.floor(end.getTime() / 1000).toString(),
      step,
    });

    const response = await fetch(`${this.baseUrl}/query_range?${params}`);
    if (!response.ok) throw new Error("Failed to fetch metrics");
    return response.json();
  }

  async getLabels(): Promise<string[]> {
    const response = await fetch(`${this.baseUrl}/labels`);
    if (!response.ok) throw new Error("Failed to fetch labels");
    const data = await response.json();
    return data.data || [];
  }

  async getLabelValues(label: string): Promise<string[]> {
    const response = await fetch(`${this.baseUrl}/label/${label}/values`);
    if (!response.ok) throw new Error("Failed to fetch label values");
    const data = await response.json();
    return data.data || [];
  }

  async getMetricNames(): Promise<string[]> {
    return this.getLabelValues("__name__");
  }
}

// Loki API client
class LokiClient {
  constructor(private baseUrl = "/api/loki") {}

  async queryRange(query: string, start: Date, end: Date, limit = 1000): Promise<any> {
    const params = new URLSearchParams({
      query,
      start: (start.getTime() * 1000000).toString(), // nanoseconds
      end: (end.getTime() * 1000000).toString(),
      limit: limit.toString(),
    });

    const response = await fetch(`${this.baseUrl}/query_range?${params}`);
    if (!response.ok) throw new Error("Failed to fetch logs");
    return response.json();
  }

  async getLabels(start?: Date, end?: Date): Promise<string[]> {
    const params = new URLSearchParams();
    if (start) params.append("start", (start.getTime() * 1000000).toString());
    if (end) params.append("end", (end.getTime() * 1000000).toString());

    const response = await fetch(`${this.baseUrl}/labels?${params}`);
    if (!response.ok) throw new Error("Failed to fetch labels");
    const data = await response.json();
    return data.data || [];
  }

  async getLabelValues(label: string, start?: Date, end?: Date): Promise<string[]> {
    const params = new URLSearchParams();
    if (start) params.append("start", (start.getTime() * 1000000).toString());
    if (end) params.append("end", (end.getTime() * 1000000).toString());

    const response = await fetch(`${this.baseUrl}/label/${label}/values?${params}`);
    if (!response.ok) throw new Error("Failed to fetch label values");
    const data = await response.json();
    return data.data || [];
  }
}

interface MetricsPanelProps {
  title: string;
  query: string;
  type?: "line" | "area" | "bar" | "scatter" | "pie";
  timeRange: { start: Date; end: Date };
  refreshInterval?: number;
  height?: number;
  showLegend?: boolean;
  stacked?: boolean;
  unit?: string;
  thresholds?: { value: number; color: string; label?: string }[];
}

function MetricsPanel({
  title,
  query,
  type = "line",
  timeRange,
  refreshInterval = 30000,
  height = 300,
  showLegend = true,
  stacked = false,
  unit = "",
  thresholds = [],
}: MetricsPanelProps) {
  const vmClient = useMemo(() => new VictoriaMetricsClient(), []);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["metrics", query, timeRange],
    queryFn: () => vmClient.queryRange(query, timeRange.start, timeRange.end),
    refetchInterval: refreshInterval,
  });

  const chartData = useMemo(() => {
    if (!data?.data?.result) return [];

    // Transform Prometheus format to Recharts format
    const allTimestamps = new Set<number>();
    const series = new Map<string, Map<number, number>>();

    data.data.result.forEach((result: any) => {
      const name =
        Object.entries(result.metric)
          .filter(([k]) => k !== "__name__")
          .map(([k, v]) => `${k}="${v}"`)
          .join(",") || "value";

      const seriesData = new Map<number, number>();
      result.values.forEach(([timestamp, value]: [number, string]) => {
        allTimestamps.add(timestamp);
        seriesData.set(timestamp, parseFloat(value));
      });
      series.set(name, seriesData);
    });

    return Array.from(allTimestamps)
      .sort((a, b) => a - b)
      .map((timestamp) => {
        const point: any = { time: format(new Date(timestamp * 1000), "HH:mm") };
        series.forEach((data, name) => {
          point[name] = data.get(timestamp) || 0;
        });
        return point;
      });
  }, [data]);

  const seriesNames = useMemo(() => {
    return Object.keys(chartData[0] || {}).filter((k) => k !== "time");
  }, [chartData]);

  const colors = [
    "#3b82f6",
    "#10b981",
    "#f59e0b",
    "#ef4444",
    "#8b5cf6",
    "#ec4899",
    "#14b8a6",
    "#f97316",
    "#6366f1",
    "#84cc16",
  ];

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center" style={{ height }}>
            <ReloadIcon className="animate-spin" />
          </div>
        </CardContent>
      </Card>
    );
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center text-red-500" style={{ height }}>
            <CrossCircledIcon className="mr-2" />
            Error loading metrics
          </div>
        </CardContent>
      </Card>
    );
  }

  const renderChart = () => {
    const commonProps = {
      data: chartData,
      margin: { top: 5, right: 5, bottom: 5, left: 5 },
    };

    switch (type) {
      case "area":
        return (
          <AreaChart {...commonProps}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip formatter={(value: number) => `${value}${unit}`} />
            {showLegend && <Legend />}
            {seriesNames.map((name, idx) => (
              <Area
                key={name}
                type="monotone"
                dataKey={name}
                stackId={stacked ? "stack" : undefined}
                stroke={colors[idx % colors.length]}
                fill={colors[idx % colors.length]}
                fillOpacity={0.3}
              />
            ))}
            {thresholds.map((threshold, idx) => (
              <ReferenceLine
                key={idx}
                y={threshold.value}
                stroke={threshold.color}
                strokeDasharray="3 3"
                label={threshold.label}
              />
            ))}
          </AreaChart>
        );

      case "bar":
        return (
          <BarChart {...commonProps}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip formatter={(value: number) => `${value}${unit}`} />
            {showLegend && <Legend />}
            {seriesNames.map((name, idx) => (
              <Bar
                key={name}
                dataKey={name}
                stackId={stacked ? "stack" : undefined}
                fill={colors[idx % colors.length]}
              />
            ))}
          </BarChart>
        );

      case "scatter":
        return (
          <ScatterChart {...commonProps}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip formatter={(value: number) => `${value}${unit}`} />
            {showLegend && <Legend />}
            {seriesNames.map((name, idx) => (
              <Scatter
                key={name}
                data={chartData}
                dataKey={name}
                fill={colors[idx % colors.length]}
              />
            ))}
          </ScatterChart>
        );
      default:
        return (
          <LineChart {...commonProps}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="time" />
            <YAxis />
            <Tooltip formatter={(value: number) => `${value}${unit}`} />
            {showLegend && <Legend />}
            <Brush dataKey="time" height={30} />
            {seriesNames.map((name, idx) => (
              <Line
                key={name}
                type="monotone"
                dataKey={name}
                stroke={colors[idx % colors.length]}
                strokeWidth={2}
                dot={false}
              />
            ))}
            {thresholds.map((threshold, idx) => (
              <ReferenceLine
                key={idx}
                y={threshold.value}
                stroke={threshold.color}
                strokeDasharray="3 3"
                label={threshold.label}
              />
            ))}
          </LineChart>
        );
    }
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{title}</CardTitle>
          <Button variant="ghost" size="icon" onClick={() => refetch()}>
            <ReloadIcon className="h-4 w-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <ResponsiveContainer width="100%" height={height}>
          {renderChart()}
        </ResponsiveContainer>
      </CardContent>
    </Card>
  );
}

interface LogBrowserProps {
  timeRange: { start: Date; end: Date };
  height?: number;
}

function LogBrowser({ timeRange, height = 500 }: LogBrowserProps) {
  const [query, setQuery] = useState('{source="device-api"}');
  const [filter, setFilter] = useState("");
  const lokiClient = useMemo(() => new LokiClient(), []);

  const { data, isLoading, refetch } = useQuery({
    queryKey: ["logs", query, timeRange],
    queryFn: () => lokiClient.queryRange(query, timeRange.start, timeRange.end),
    refetchInterval: 30000,
  });

  const logs = useMemo(() => {
    if (!data?.data?.result) return [];

    const allLogs: any[] = [];
    data.data.result.forEach((stream: any) => {
      stream.values.forEach(([timestamp, line]: [string, string]) => {
        allLogs.push({
          timestamp: new Date(parseInt(timestamp, 10) / 1000000), // nanoseconds to ms
          ...stream.stream,
          message: line,
        });
      });
    });

    return allLogs
      .sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime())
      .filter((log) => !filter || log.message.toLowerCase().includes(filter.toLowerCase()));
  }, [data, filter]);

  const getLevelColor = (level: string) => {
    switch (level?.toLowerCase()) {
      case "error":
        return "text-red-600 dark:text-red-400";
      case "warn":
      case "warning":
        return "text-yellow-600 dark:text-yellow-400";
      case "info":
        return "text-blue-600 dark:text-blue-400";
      case "debug":
        return "text-gray-600 dark:text-gray-400";
      default:
        return "text-gray-600 dark:text-gray-400";
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Log Browser</CardTitle>
        <div className="flex gap-2 mt-2">
          <Input
            placeholder="LogQL query..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1"
          />
          <Input
            placeholder="Filter logs..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="max-w-xs"
          />
          <Button onClick={() => refetch()}>
            <MagnifyingGlassIcon className="mr-2 h-4 w-4" />
            Search
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <ScrollArea className="border rounded-md" style={{ height }}>
          <div className="p-4 font-mono text-xs">
            {isLoading ? (
              <div className="flex items-center justify-center h-full">
                <ReloadIcon className="animate-spin" />
              </div>
            ) : logs.length === 0 ? (
              <div className="text-center text-muted-foreground">No logs found</div>
            ) : (
              <div className="space-y-1">
                {logs.map((log, idx) => (
                  <div key={idx} className="flex gap-2 hover:bg-muted/50 p-1 rounded">
                    <span className="text-muted-foreground min-w-[140px]">
                      {format(log.timestamp, "yyyy-MM-dd HH:mm:ss")}
                    </span>
                    <span className={cn("min-w-[60px]", getLevelColor(log.level))}>
                      [{log.level || "INFO"}]
                    </span>
                    {log.device_id && (
                      <span className="text-muted-foreground min-w-[100px]">{log.device_id}</span>
                    )}
                    <span className="flex-1 break-all">{log.message}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}

export function MetricsDashboard() {
  const [timeRange, setTimeRange] = useState("1h");
  const [customQuery, setCustomQuery] = useState("");
  const [showQueryBuilder, setShowQueryBuilder] = useState(false);

  const timeRanges = useMemo(() => {
    const now = new Date();
    return {
      "5m": { start: subMinutes(now, 5), end: now },
      "15m": { start: subMinutes(now, 15), end: now },
      "30m": { start: subMinutes(now, 30), end: now },
      "1h": { start: subHours(now, 1), end: now },
      "3h": { start: subHours(now, 3), end: now },
      "6h": { start: subHours(now, 6), end: now },
      "12h": { start: subHours(now, 12), end: now },
      "24h": { start: subDays(now, 1), end: now },
      "7d": { start: subDays(now, 7), end: now },
      "30d": { start: subDays(now, 30), end: now },
    };
  }, []);

  const currentTimeRange = timeRanges[timeRange as keyof typeof timeRanges];

  return (
    <div className="space-y-6">
      {/* Header Controls */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Metrics & Observability</CardTitle>
              <CardDescription>
                Real-time metrics from VictoriaMetrics and logs from Loki
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Select value={timeRange} onValueChange={setTimeRange}>
                <SelectTrigger className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.keys(timeRanges).map((range) => (
                    <SelectItem key={range} value={range}>
                      Last {range}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Dialog open={showQueryBuilder} onOpenChange={setShowQueryBuilder}>
                <DialogTrigger asChild>
                  <Button variant="outline">
                    <MixerHorizontalIcon className="mr-2 h-4 w-4" />
                    Query Builder
                  </Button>
                </DialogTrigger>
                <DialogContent className="max-w-3xl">
                  <DialogHeader>
                    <DialogTitle>Custom Query Builder</DialogTitle>
                    <DialogDescription>
                      Build custom PromQL queries for VictoriaMetrics
                    </DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4">
                    <Textarea
                      placeholder="Enter PromQL query..."
                      value={customQuery}
                      onChange={(e) => setCustomQuery(e.target.value)}
                      className="font-mono min-h-[100px]"
                    />
                    <div className="text-sm text-muted-foreground">
                      <p>Example queries:</p>
                      <ul className="list-disc list-inside mt-2 space-y-1">
                        <li>rate(http_requests_total[5m])</li>
                        <li>avg(cpu_usage) by (device_id)</li>
                        <li>histogram_quantile(0.95, http_request_duration_seconds)</li>
                      </ul>
                    </div>
                  </div>
                </DialogContent>
              </Dialog>

              <Button variant="outline" size="icon">
                <DownloadIcon className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Main Dashboard */}
      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList className="grid grid-cols-5 w-full max-w-2xl">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="devices">Devices</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="custom">Custom</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          {/* Key Metrics */}
          <div className="grid grid-cols-4 gap-4">
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Total Devices</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">1,234</div>
                <div className="flex items-center text-xs text-green-600">
                  <ArrowUpIcon className="mr-1" />
                  <span>12% from last hour</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Active Devices</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">987</div>
                <div className="flex items-center text-xs text-muted-foreground">
                  <CheckCircledIcon className="mr-1" />
                  <span>80% online</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Error Rate</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">0.23%</div>
                <div className="flex items-center text-xs text-green-600">
                  <ArrowDownIcon className="mr-1" />
                  <span>5% improvement</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Avg Response Time</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">124ms</div>
                <div className="flex items-center text-xs text-yellow-600">
                  <ArrowUpIcon className="mr-1" />
                  <span>2ms slower</span>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Main Charts */}
          <div className="grid grid-cols-2 gap-4">
            <MetricsPanel
              title="Request Rate"
              query="rate(fleetd_api_requests_total[5m])"
              type="area"
              timeRange={currentTimeRange}
              unit=" req/s"
            />

            <MetricsPanel
              title="Error Rate"
              query='rate(fleetd_api_requests_total{status=~"5.."}[5m])'
              type="line"
              timeRange={currentTimeRange}
              unit=" errors/s"
              thresholds={[
                { value: 0.1, color: "orange", label: "Warning" },
                { value: 0.5, color: "red", label: "Critical" },
              ]}
            />

            <MetricsPanel
              title="CPU Usage by Device"
              query="avg(device_cpu_usage) by (device_id)"
              type="area"
              timeRange={currentTimeRange}
              unit="%"
              stacked
            />

            <MetricsPanel
              title="Memory Usage"
              query="avg(device_memory_usage) by (device_id)"
              type="line"
              timeRange={currentTimeRange}
              unit=" MB"
            />
          </div>

          {/* Response Time Distribution */}
          <MetricsPanel
            title="Response Time Distribution"
            query="histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))"
            type="bar"
            timeRange={currentTimeRange}
            height={250}
            unit=" ms"
          />
        </TabsContent>

        <TabsContent value="devices" className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <MetricsPanel
              title="Devices by Status"
              query="count(up) by (status)"
              type="bar"
              timeRange={currentTimeRange}
              stacked
            />

            <MetricsPanel
              title="Device Registrations"
              query="rate(fleetd_device_registrations_total[1h])"
              type="area"
              timeRange={currentTimeRange}
            />

            <MetricsPanel
              title="Telemetry Data Rate"
              query="rate(fleetd_telemetry_received_total[5m])"
              type="line"
              timeRange={currentTimeRange}
              unit=" msgs/s"
            />

            <MetricsPanel
              title="Device Heartbeats"
              query="rate(fleetd_device_heartbeats_total[5m])"
              type="area"
              timeRange={currentTimeRange}
            />
          </div>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <MetricsPanel
              title="P95 Latency"
              query="histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))"
              type="line"
              timeRange={currentTimeRange}
              unit=" ms"
            />

            <MetricsPanel
              title="P99 Latency"
              query="histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))"
              type="line"
              timeRange={currentTimeRange}
              unit=" ms"
            />

            <MetricsPanel
              title="Database Query Time"
              query="rate(database_query_duration_seconds_sum[5m])"
              type="area"
              timeRange={currentTimeRange}
              unit=" s"
            />

            <MetricsPanel
              title="Cache Hit Rate"
              query="rate(cache_hits_total[5m]) / (rate(cache_hits_total[5m]) + rate(cache_misses_total[5m]))"
              type="line"
              timeRange={currentTimeRange}
              unit="%"
            />
          </div>
        </TabsContent>

        <TabsContent value="logs" className="space-y-4">
          <LogBrowser timeRange={currentTimeRange} height={600} />
        </TabsContent>

        <TabsContent value="custom" className="space-y-4">
          {customQuery ? (
            <MetricsPanel
              title="Custom Query"
              query={customQuery}
              type="line"
              timeRange={currentTimeRange}
              height={400}
            />
          ) : (
            <Card>
              <CardContent className="py-20">
                <div className="text-center text-muted-foreground">
                  <BarChartIcon className="mx-auto h-12 w-12 mb-4" />
                  <p>No custom query defined</p>
                  <p className="text-sm mt-2">
                    Use the Query Builder to create a custom visualization
                  </p>
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
