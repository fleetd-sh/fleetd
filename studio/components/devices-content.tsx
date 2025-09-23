"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  PlusIcon,
  ReloadIcon,
  MagnifyingGlassIcon,
  DownloadIcon,
  TrashIcon
} from "@radix-ui/react-icons";
import { DeviceDataTable } from "@/components/device-data-table";
import { DeviceDetailEnhanced } from "@/components/device-detail-enhanced";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { api } from "@/lib/api";
import type { Device } from "@/lib/types";
import { useSonnerToast } from "@/hooks/use-sonner-toast";

export function DevicesContent() {
  const { toast } = useSonnerToast();
  const [selectedDevice, setSelectedDevice] = React.useState<Device | null>(null);
  const [searchQuery, setSearchQuery] = React.useState("");
  const [showAddDevice, setShowAddDevice] = React.useState(false);

  const { data: devices = [], isLoading, refetch } = useQuery({
    queryKey: ["devices"],
    queryFn: api.getDevices,
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  const filteredDevices = React.useMemo(() => {
    if (!searchQuery) return devices;

    return devices.filter(device =>
      device.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      device.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      device.type.toLowerCase().includes(searchQuery.toLowerCase())
    );
  }, [devices, searchQuery]);

  const handleRefresh = async () => {
    const result = await refetch();
    if (result.isSuccess) {
      toast.success("Devices refreshed");
    }
  };

  const handleDiscoverDevices = async () => {
    toast.loading("Discovering devices...");
    try {
      const discovered = await api.discoverDevices();
      toast.success(`Found ${discovered.length} new device(s)`);
      refetch();
    } catch (error) {
      toast.error("Discovery failed", "Could not discover devices on the network");
    }
  };

  const handleExport = () => {
    const csv = [
      ["ID", "Name", "Type", "Status", "Version", "Last Seen"],
      ...devices.map(d => [
        d.id,
        d.name,
        d.type,
        d.status,
        d.version,
        d.last_seen
      ])
    ].map(row => row.join(",")).join("\n");

    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `devices-${new Date().toISOString()}.csv`;
    a.click();
    URL.revokeObjectURL(url);

    toast.success("Exported device list");
  };

  const stats = React.useMemo(() => {
    const online = devices.filter(d => d.status === "online").length;
    const offline = devices.length - online;
    const types = new Set(devices.map(d => d.type)).size;

    return { total: devices.length, online, offline, types };
  }, [devices]);

  if (selectedDevice) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelectedDevice(null)}
          >
            ← Back to list
          </Button>
        </div>
        <DeviceDetailEnhanced
          device={selectedDevice}
          onDeploy={() => toast.info("Deploy feature coming soon")}
          onRestart={async () => {
            toast.promise(
              api.restartDevice(selectedDevice.id),
              {
                loading: "Restarting device...",
                success: "Device restart initiated",
                error: "Failed to restart device"
              }
            );
          }}
          onUpdate={async () => {
            toast.promise(
              api.updateDevice(selectedDevice.id),
              {
                loading: "Updating device...",
                success: "Device update initiated",
                error: "Failed to update device"
              }
            );
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
            <CardTitle className="text-sm font-medium">Total Devices</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats.total}</div>
            <p className="text-xs text-muted-foreground">
              {stats.types} different types
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Online</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">{stats.online}</div>
            <p className="text-xs text-muted-foreground">
              {stats.total > 0 ? Math.round((stats.online / stats.total) * 100) : 0}% of fleet
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Offline</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gray-600">{stats.offline}</div>
            <p className="text-xs text-muted-foreground">
              {stats.total > 0 ? Math.round((stats.offline / stats.total) * 100) : 0}% of fleet
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Health Score</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {stats.total > 0 ? Math.round((stats.online / stats.total) * 100) : 0}%
            </div>
            <p className="text-xs text-muted-foreground">Fleet availability</p>
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

        <div className="flex gap-2">
          <Button onClick={handleRefresh} variant="outline" size="sm">
            <ReloadIcon className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          <Button onClick={handleDiscoverDevices} variant="outline" size="sm">
            <MagnifyingGlassIcon className="mr-2 h-4 w-4" />
            Discover
          </Button>
          <Button onClick={handleExport} variant="outline" size="sm">
            <DownloadIcon className="mr-2 h-4 w-4" />
            Export
          </Button>
          <Button onClick={() => setShowAddDevice(true)} size="sm">
            <PlusIcon className="mr-2 h-4 w-4" />
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
            <div className="text-center py-8 text-muted-foreground">
              Loading devices...
            </div>
          ) : (
            <DeviceDataTable
              devices={filteredDevices}
              onSelectDevice={setSelectedDevice}
            />
          )}
        </CardContent>
      </Card>

      {/* Add Device Dialog */}
      <Dialog open={showAddDevice} onOpenChange={setShowAddDevice}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add New Device</DialogTitle>
            <DialogDescription>
              Register a new device to your fleet
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-muted-foreground">
              Device provisioning feature coming soon. Use the CLI for now:
            </p>
            <pre className="bg-muted p-3 rounded text-xs">
              fleetctl provision --device /dev/diskX
            </pre>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}