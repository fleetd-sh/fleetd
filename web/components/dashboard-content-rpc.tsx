'use client'

import { DeviceList } from '@/components/device-list'
import { DeviceStats } from '@/components/device-stats'
import { TelemetryChart } from '@/components/telemetry-chart'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useToast } from '@/hooks/use-toast'
import type { Device as ProtoDevice } from '@/lib/api/gen/public/v1/fleet_pb'
import {
  useDeviceStats,
  useDevices,
  useDiscoverDevices,
  useEventStream,
  useTelemetry,
} from '@/lib/api/hooks'
import type { Device } from '@/lib/types'
import { MagnifyingGlassIcon, PlusIcon, ReloadIcon } from '@radix-ui/react-icons'
import { motion } from 'framer-motion'
import { useState } from 'react'

export function DashboardContentRPC() {
  const { toast } = useToast()
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null)

  // Use Connect RPC hooks
  const { data: devicesResponse, refetch: refetchDevices } = useDevices()
  const { data: statsResponse } = useDeviceStats()
  const discoverMutation = useDiscoverDevices()

  // Convert proto devices to our local type
  const devices: Device[] =
    devicesResponse?.devices?.map((d: ProtoDevice) => ({
      id: d.id,
      name: d.name,
      type: d.type,
      version: d.version,
      last_seen: d.lastSeen
        ? new Date(
            Number(d.lastSeen.seconds) * 1000 + Number(d.lastSeen.nanos) / 1000000,
          ).toISOString()
        : new Date().toISOString(),
      status: d.status === 1 ? 'online' : 'offline',
      metadata: d.metadata ? JSON.stringify(d.metadata) : undefined,
    })) || []

  // Telemetry query with selected device
  const { data: telemetryPoints } = useTelemetry({
    deviceId: selectedDevice || '',
    limit: 100,
  })

  // Use event stream for real-time updates
  useEventStream(
    {
      deviceIds: selectedDevice ? [selectedDevice] : [],
    },
    (event) => {
      // Handle specific events
      switch (event.type) {
        case 1: // DEVICE_CONNECTED
          toast({
            title: 'Device Connected',
            description: event.message || `Device ${event.deviceId} is now online`,
          })
          break
        case 2: // DEVICE_DISCONNECTED
          toast({
            title: 'Device Disconnected',
            description: event.message || `Device ${event.deviceId} is now offline`,
            variant: 'destructive',
          })
          break
        case 9: // ALERT
          toast({
            title: 'Alert',
            description: event.message,
            variant: 'destructive',
          })
          break
      }
    },
  )

  const handleDiscoverDevices = async () => {
    try {
      const discovered = await discoverMutation.mutateAsync({ timeoutSeconds: 10 })
      toast({
        title: 'Discovery Complete',
        description: `Found ${discovered?.length || 0} device(s) on the network`,
      })
      refetchDevices()
    } catch (error) {
      toast({
        title: 'Discovery Failed',
        description: 'Failed to discover devices on the network',
        variant: 'destructive',
      })
    }
  }

  const handleRefresh = () => {
    refetchDevices()
    toast({
      title: 'Refreshed',
      description: 'Dashboard data has been updated',
    })
  }

  // Convert telemetry points for chart
  interface TelemetryPoint {
    deviceId: string
    metricName: string
    value: number
    timestamp?: { seconds: bigint; nanos: number }
  }

  const telemetryData =
    telemetryPoints?.map((point: TelemetryPoint) => ({
      device_id: point.deviceId,
      metric_name: point.metricName,
      value: point.value,
      timestamp: point.timestamp
        ? new Date(
            Number(point.timestamp.seconds) * 1000 + Number(point.timestamp.nanos) / 1000000,
          ).toISOString()
        : new Date().toISOString(),
    })) || []

  return (
    <div className="space-y-8">
      {/* Action Bar */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex flex-wrap gap-4 justify-between items-center"
      >
        <div className="flex gap-2">
          <Button onClick={handleRefresh} variant="outline" size="sm">
            <ReloadIcon className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          <Button
            onClick={handleDiscoverDevices}
            variant="outline"
            size="sm"
            disabled={discoverMutation.isPending}
          >
            <MagnifyingGlassIcon className="mr-2 h-4 w-4" />
            {discoverMutation.isPending ? 'Discovering...' : 'Discover Devices'}
          </Button>
          <Button variant="default" size="sm">
            <PlusIcon className="mr-2 h-4 w-4" />
            Add Device
          </Button>
        </div>
      </motion.div>

      {/* Stats Overview */}
      <DeviceStats devices={devices} />

      {/* Main Content Grid */}
      <div className="grid gap-8 lg:grid-cols-2">
        {/* Device List */}
        <motion.div
          initial={{ opacity: 0, x: -20 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ delay: 0.1 }}
        >
          <Card>
            <CardHeader>
              <CardTitle>Connected Devices</CardTitle>
              <CardDescription>
                {statsResponse
                  ? `${statsResponse.onlineDevices} online, ${statsResponse.offlineDevices} offline`
                  : 'All devices in your fleet'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DeviceList
                devices={devices}
                selectedDevice={selectedDevice}
                onSelectDevice={setSelectedDevice}
              />
            </CardContent>
          </Card>
        </motion.div>

        {/* Telemetry Chart */}
        <motion.div
          initial={{ opacity: 0, x: 20 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ delay: 0.2 }}
        >
          <Card>
            <CardHeader>
              <CardTitle>
                {selectedDevice ? `Telemetry for ${selectedDevice}` : 'Recent Telemetry'}
              </CardTitle>
              <CardDescription>Real-time metrics from your devices</CardDescription>
            </CardHeader>
            <CardContent>
              <TelemetryChart data={telemetryData} />
            </CardContent>
          </Card>
        </motion.div>
      </div>
    </div>
  )
}
