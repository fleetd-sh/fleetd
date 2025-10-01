"use client";
import { PlusIcon, ReloadIcon } from "@radix-ui/react-icons";
import { useQuery } from "@tanstack/react-query";
import { motion } from "framer-motion";
import { useState } from "react";
import { DeviceDataTable } from "@/components/device-data-table";
import { DeviceStats } from "@/components/device-stats";
import { ProvisioningGuide } from "@/components/provisioning-guide";
import { TelemetryChart } from "@/components/telemetry-chart";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { Device, TelemetryData } from "@/lib/types";

interface DashboardContentProps {
  initialData: {
    devices: Device[];
    telemetry: TelemetryData[];
  };
}
export function DashboardContent({ initialData }: DashboardContentProps) {
  const { toast } = useToast();
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null);
  const [showProvisioningGuide, setShowProvisioningGuide] = useState(false);
  const { data: devices, refetch: refetchDevices } = useQuery({
    queryKey: ["devices"],
    queryFn: api.getDevices,
    initialData: initialData.devices,
    refetchInterval: 10000,
  });
  const { data: telemetry, refetch: refetchTelemetry } = useQuery({
    queryKey: ["telemetry", selectedDevice],
    queryFn: () => (selectedDevice ? api.getTelemetry(selectedDevice) : api.getMetrics()),
    initialData: initialData.telemetry,
    refetchInterval: 5000,
  });
  const handleRefresh = () => {
    refetchDevices();
    refetchTelemetry();
    toast({
      title: "Refreshed",
      description: "Dashboard data has been updated",
    });
  };
  return (
    <div className="space-y-8">
      {/* Action Bar */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex flex-wrap gap-4 justify-between items-center"
      >
        <div className="flex flex-wrap gap-2">
          <Button onClick={handleRefresh} variant="outline" size="sm">
            <ReloadIcon className="h-4 w-4 mr-2" />
            Refresh
          </Button>
          <Button variant="default" size="sm" onClick={() => setShowProvisioningGuide(true)}>
            <PlusIcon className="h-4 w-4 mr-2" />
            Add Device
          </Button>
        </div>
      </motion.div>
      {/* Stats Overview */}
      <DeviceStats devices={devices || []} />
      {/* Main Content with Tabs */}
      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList className="grid w-full max-w-[400px] grid-cols-3">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="devices">Devices</TabsTrigger>
          <TabsTrigger value="telemetry">Telemetry</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="space-y-4">
          <div className="grid gap-8 lg:grid-cols-2">
            {/* Device Overview Card */}
            <motion.div
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ delay: 0.1 }}
            >
              <Card className="h-full">
                <CardHeader>
                  <CardTitle>Device Overview</CardTitle>
                  <CardDescription>Quick status of your fleet</CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="flex flex-col justify-center h-64 space-y-8">
                    <div className="flex items-center">
                      <div className="ml-4 space-y-1">
                        <p className="text-sm font-medium leading-none">Total Devices</p>
                        <p className="text-2xl font-bold">{devices?.length || 0}</p>
                      </div>
                    </div>
                    <div className="flex items-center">
                      <div className="ml-4 space-y-1">
                        <p className="text-sm font-medium leading-none">Online</p>
                        <p className="text-2xl font-bold text-green-600">
                          {devices?.filter((d) => d.status === "online").length || 0}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center">
                      <div className="ml-4 space-y-1">
                        <p className="text-sm font-medium leading-none">Offline</p>
                        <p className="text-2xl font-bold text-gray-600">
                          {devices?.filter((d) => d.status !== "online").length || 0}
                        </p>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
            {/* Telemetry Chart */}
            <motion.div
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ delay: 0.2 }}
            >
              <Card className="h-full">
                <CardHeader>
                  <CardTitle>Recent Telemetry</CardTitle>
                  <CardDescription>Real-time metrics from your devices</CardDescription>
                </CardHeader>
                <CardContent>
                  <TelemetryChart data={telemetry || []} />
                </CardContent>
              </Card>
            </motion.div>
          </div>
        </TabsContent>
        <TabsContent value="devices" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Device Management</CardTitle>
              <CardDescription>View and manage all devices in your fleet</CardDescription>
            </CardHeader>
            <CardContent>
              <DeviceDataTable
                devices={devices || []}
                onSelectDevice={(device) => setSelectedDevice(device.id)}
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="telemetry" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>
                {selectedDevice ? `Telemetry for ${selectedDevice}` : "System Telemetry"}
              </CardTitle>
              <CardDescription>Detailed metrics and performance data</CardDescription>
            </CardHeader>
            <CardContent className="h-[500px]">
              <TelemetryChart data={telemetry || []} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
      {/* Provisioning Guide Dialog */}
      <Dialog open={showProvisioningGuide} onOpenChange={setShowProvisioningGuide}>
        <DialogContent className="max-w-6xl max-h-[90vh] flex flex-col">
          <DialogHeader>
            <DialogTitle>Provision New Device</DialogTitle>
          </DialogHeader>
          <div className="flex-1 overflow-y-auto">
            <ProvisioningGuide onClose={() => setShowProvisioningGuide(false)} />
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
