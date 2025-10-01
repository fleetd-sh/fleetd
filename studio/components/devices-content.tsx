"use client";
import { DownloadIcon, PlusIcon, ReloadIcon } from "@radix-ui/react-icons";
import { useQuery } from "@tanstack/react-query";
import * as React from "react";
import { DeviceDataTable } from "@/components/device-data-table";
import { DeviceDetailEnhanced } from "@/components/device-detail-enhanced";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Snippet, SnippetContent, SnippetCopyButton, SnippetHeader } from "@/components/ui/snippet";
import { useSonnerToast } from "@/hooks/use-sonner-toast";
import { api } from "@/lib/api";
import type { Device } from "@/lib/types";
export function DevicesContent() {
  const { success, info, promise } = useSonnerToast();
  const [selectedDevice, setSelectedDevice] = React.useState<Device | null>(null);
  const [searchQuery, setSearchQuery] = React.useState("");
  const [showAddDevice, setShowAddDevice] = React.useState(false);
  const {
    data: devices = [],
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["devices"],
    queryFn: api.getDevices,
    refetchInterval: 30000, // Refresh every 30 seconds
  });
  const filteredDevices = React.useMemo(() => {
    if (!searchQuery) return devices;
    return devices.filter(
      (device) =>
        device.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        device.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
        device.type.toLowerCase().includes(searchQuery.toLowerCase()),
    );
  }, [devices, searchQuery]);
  const handleRefresh = async () => {
    const result = await refetch();
    if (result.isSuccess) {
      success("Devices refreshed");
    }
  };
  const handleExport = () => {
    const csv = [
      ["ID", "Name", "Type", "Status", "Version", "Last Seen"],
      ...devices.map((d) => [d.id, d.name, d.type, d.status, d.version, d.last_seen]),
    ]
      .map((row) => row.join(","))
      .join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `devices-${new Date().toISOString()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    success("Exported device list");
  };
  const stats = React.useMemo(() => {
    const online = devices.filter((d) => d.status === "online").length;
    const offline = devices.length - online;
    const types = new Set(devices.map((d) => d.type)).size;
    return { total: devices.length, online, offline, types };
  }, [devices]);
  if (selectedDevice) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => setSelectedDevice(null)}>
            ← Back to list
          </Button>
        </div>
        <DeviceDetailEnhanced
          device={selectedDevice}
          onDeploy={() => info("Deploy feature coming soon")}
          onRestart={async () => {
            promise(api.restartDevice(selectedDevice.id), {
              loading: "Restarting device...",
              success: "Device restart initiated",
              error: "Failed to restart device",
            });
          }}
          onUpdate={async () => {
            promise(api.updateDevice(selectedDevice.id, {}), {
              loading: "Updating device...",
              success: "Device update initiated",
              error: "Failed to update device",
            });
          }}
        />
      </div>
    );
  }
  return (
    <div className="space-y-6">
      {/* Stats Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm-atlas font-medium">Total Devices</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-xl-atlas font-bold">{stats.total}</div>
            <p className="text-xs-atlas text-muted-foreground">{stats.types} different types</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm-atlas font-medium">Online</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-xl-atlas font-bold text-green-600">{stats.online}</div>
            <p className="text-xs-atlas text-muted-foreground">
              {stats.total > 0 ? Math.round((stats.online / stats.total) * 100) : 0}% of fleet
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm-atlas font-medium">Offline</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-xl-atlas font-bold text-gray-600">{stats.offline}</div>
            <p className="text-xs-atlas text-muted-foreground">
              {stats.total > 0 ? Math.round((stats.offline / stats.total) * 100) : 0}% of fleet
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm-atlas font-medium">Health Score</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-xl-atlas font-bold">
              {stats.total > 0 ? Math.round((stats.online / stats.total) * 100) : 0}%
            </div>
            <p className="text-xs-atlas text-muted-foreground">Fleet availability</p>
          </CardContent>
        </Card>
      </div>
      {/* Action Bar */}
      <div className="flex flex-wrap gap-4 justify-between items-center">
        <div className="flex gap-2 flex-1 max-w-sm">
          <Input
            placeholder="Search devices..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="flex-1"
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={handleRefresh} variant="outline" size="sm">
            <ReloadIcon className="h-4 w-4 mr-2" />
            Refresh
          </Button>
          <Button onClick={handleExport} variant="outline" size="sm">
            <DownloadIcon className="h-4 w-4 mr-2" />
            Export
          </Button>
          <Button onClick={() => setShowAddDevice(true)} size="sm">
            <PlusIcon className="h-4 w-4 mr-2" />
            Add Device
          </Button>
        </div>
      </div>
      {/* Devices Table */}
      <Card>
        <CardHeader>
          <CardTitle>All Devices</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="text-center py-8 text-muted-foreground">Loading devices...</div>
          ) : (
            <DeviceDataTable devices={filteredDevices} onSelectDevice={setSelectedDevice} />
          )}
        </CardContent>
      </Card>
      {/* Add Device Dialog */}
      <Dialog open={showAddDevice} onOpenChange={setShowAddDevice}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add New Device</DialogTitle>
            <DialogDescription>Register a new device to your fleet</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm-atlas text-muted-foreground">
              Device provisioning feature coming soon. Use the CLI for now:
            </p>
            <Snippet defaultValue="provision">
              <SnippetHeader language="bash">
                <SnippetCopyButton value="fleetctl provision --device /dev/diskX" />
              </SnippetHeader>
              <SnippetContent language="bash">
                fleetctl provision --device /dev/diskX
              </SnippetContent>
            </Snippet>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
