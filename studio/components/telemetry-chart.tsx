"use client";

import { format } from "date-fns";
import { motion } from "framer-motion";
import { useMemo } from "react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { TelemetryData } from "@/lib/types";

interface TelemetryChartProps {
  data: TelemetryData[];
}

export function TelemetryChart({ data }: TelemetryChartProps) {
  const chartData = useMemo(() => {
    // Group data by timestamp and metric name
    const grouped = data.reduce(
      (acc, item) => {
        const time = format(new Date(item.timestamp), "HH:mm:ss");
        if (!acc[time]) {
          acc[time] = { time };
        }
        acc[time][item.metric_name] = item.value;
        return acc;
      },
      {} as Record<string, Record<string, string | number>>,
    );

    return Object.values(grouped).slice(-20); // Show last 20 data points
  }, [data]);

  const metrics = useMemo(() => {
    const metricSet = new Set(data.map((d) => d.metric_name));
    return Array.from(metricSet);
  }, [data]);

  const colors = ["#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#ec4899"];

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-muted-foreground">
        No telemetry data available
      </div>
    );
  }

  return (
    <motion.div
      initial={{ opacity: 0, scale: 0.95 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.3 }}
      className="w-full h-64"
    >
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis dataKey="time" className="text-xs" />
          <YAxis className="text-xs" />
          <Tooltip
            contentStyle={{
              backgroundColor: "hsl(var(--background))",
              border: "1px solid hsl(var(--border))",
              borderRadius: "6px",
            }}
          />
          <Legend />
          {metrics.map((metric, index) => (
            <Line
              key={metric}
              type="monotone"
              dataKey={metric}
              stroke={colors[index % colors.length]}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </motion.div>
  );
}
