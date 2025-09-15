'use client'

import { DeviceAutoSetup } from '@/components/device-auto-setup'
import { DeviceList } from '@/components/device-list'
import { DeviceStats } from '@/components/device-stats'
import { ProvisioningGuide } from '@/components/provisioning-guide'
import { QuickProvision } from '@/components/quick-provision'
import { TelemetryChart } from '@/components/telemetry-chart'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useToast } from '@/hooks/use-toast'
import { api } from '@/lib/api'
import type { Device, TelemetryData } from '@/lib/types'
import { MagnifyingGlassIcon, PlusIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useQuery } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'framer-motion'
import { useState } from 'react'

interface DashboardContentProps {
  initialData: {
    devices: Device[]
    telemetry: TelemetryData[]
  }
}

export function DashboardContent({ initialData }: DashboardContentProps) {
  const { toast } = useToast()
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null)
  const [showProvisioningGuide, setShowProvisioningGuide] = useState(false)

  const { data: devices, refetch: refetchDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: api.getDevices,
    initialData: initialData.devices,
    refetchInterval: 10000,
  })

  const { data: telemetry, refetch: refetchTelemetry } = useQuery({
    queryKey: ['telemetry', selectedDevice],
    queryFn: () => (selectedDevice ? api.getTelemetry(selectedDevice) : api.getMetrics()),
    initialData: initialData.telemetry,
    refetchInterval: 5000,
  })

  const handleDiscoverDevices = async () => {
    try {
      const discovered = await api.discoverDevices()
      toast({
        title: 'Discovery Complete',
        description: `Found ${discovered.length} device(s) on the network`,
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
    refetchTelemetry()
    toast({
      title: 'Refreshed',
      description: 'Dashboard data has been updated',
    })
  }

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
          <Button onClick={handleDiscoverDevices} variant="outline" size="sm">
            <MagnifyingGlassIcon className="mr-2 h-4 w-4" />
            Discover Devices
          </Button>
          <Button variant="default" size="sm" onClick={() => setShowProvisioningGuide(true)}>
            <PlusIcon className="mr-2 h-4 w-4" />
            Add Device
          </Button>
        </div>
      </motion.div>

      {/* Stats Overview */}
      <DeviceStats devices={devices || []} />

      {/* Device Auto-Setup */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
      >
        <DeviceAutoSetup />
      </motion.div>

      {/* Quick Provisioning */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.15 }}
      >
        <QuickProvision />
      </motion.div>

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
              <CardDescription>All devices in your fleet</CardDescription>
            </CardHeader>
            <CardContent>
              <DeviceList
                devices={devices || []}
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
              <TelemetryChart data={telemetry || []} />
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Provisioning Guide Dialog */}
      <Dialog open={showProvisioningGuide} onOpenChange={setShowProvisioningGuide}>
        <DialogContent className="max-w-6xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Provision New Device</DialogTitle>
          </DialogHeader>
          <ProvisioningGuide />
        </DialogContent>
      </Dialog>
    </div>
  )
}
