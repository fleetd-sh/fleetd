'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Device } from '@/lib/api/gen/public/v1/fleet_pb'
import {
  ActivityIcon,
  BatteryIcon,
  CpuIcon,
  FilterIcon,
  HardDriveIcon,
  NetworkIcon,
  RefreshCwIcon,
  SearchIcon,
  ServerIcon,
  ThermometerIcon,
  WifiIcon,
} from 'lucide-react'
import { useMemo, useState } from 'react'

interface FleetOverviewProps {
  devices: Device[]
  onDeviceSelect: (deviceId: string) => void
}

export function FleetOverview({ devices, onDeviceSelect }: FleetOverviewProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedType, setSelectedType] = useState<string>('all')
  const [selectedStatus, setSelectedStatus] = useState<string>('all')

  // Group devices by type and status
  const deviceStats = useMemo(() => {
    const stats = {
      total: devices.length,
      online: 0,
      offline: 0,
      updating: 0,
      byType: new Map<string, number>(),
      byOS: new Map<string, number>(),
      criticalAlerts: 0,
      warnings: 0,
    }

    for (const device of devices) {
      // Status counts
      if (device.status === 1) stats.online++
      else if (device.status === 2) stats.offline++
      else if (device.status === 3) stats.updating++

      // Type counts
      const type = device.systemInfo?.os || device.type || 'unknown'
      stats.byType.set(type, (stats.byType.get(type) || 0) + 1)

      // OS distribution
      const os = device.systemInfo?.os || 'unknown'
      stats.byOS.set(os, (stats.byOS.get(os) || 0) + 1)

      // Check for alerts (high load, low memory, etc.)
      if (device.systemInfo?.loadAverage?.load1 && device.systemInfo.loadAverage.load1 > 5) {
        stats.criticalAlerts++
      }
      if (
        device.systemInfo?.memoryTotal &&
        device.systemInfo?.processCount &&
        device.systemInfo.processCount > 500
      ) {
        stats.warnings++
      }
    }

    return stats
  }, [devices])

  // Filter devices
  const filteredDevices = useMemo(() => {
    return devices.filter((device) => {
      // Search filter
      if (
        searchQuery &&
        !device.name.toLowerCase().includes(searchQuery.toLowerCase()) &&
        !device.id.toLowerCase().includes(searchQuery.toLowerCase()) &&
        !device.systemInfo?.hostname?.toLowerCase().includes(searchQuery.toLowerCase())
      ) {
        return false
      }

      // Type filter
      if (selectedType !== 'all' && device.type !== selectedType) {
        return false
      }

      // Status filter
      if (selectedStatus !== 'all') {
        if (selectedStatus === 'online' && device.status !== 1) return false
        if (selectedStatus === 'offline' && device.status !== 2) return false
        if (selectedStatus === 'updating' && device.status !== 3) return false
      }

      return true
    })
  }, [devices, searchQuery, selectedType, selectedStatus])

  // Get device health color
  const getHealthColor = (device: Device) => {
    if (device.status !== 1) return 'text-gray-400'
    if (device.systemInfo?.loadAverage?.load1 && device.systemInfo.loadAverage.load1 > 5)
      return 'text-red-500'
    if (device.systemInfo?.loadAverage?.load1 && device.systemInfo.loadAverage.load1 > 2)
      return 'text-yellow-500'
    return 'text-green-500'
  }

  // Get device icon based on type
  const getDeviceIcon = (device: Device) => {
    const type = device.type?.toLowerCase() || ''
    if (type.includes('server')) return <ServerIcon className="h-4 w-4" />
    if (type.includes('raspberry') || type.includes('rpi')) return <CpuIcon className="h-4 w-4" />
    if (type.includes('esp')) return <WifiIcon className="h-4 w-4" />
    if (type.includes('sensor')) return <ThermometerIcon className="h-4 w-4" />
    return <ServerIcon className="h-4 w-4" />
  }

  return (
    <div className="space-y-6">
      {/* Stats Overview */}
      <div className="grid grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Total Devices</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{deviceStats.total}</div>
            <div className="text-xs text-muted-foreground mt-1">
              {deviceStats.byType.size} different types
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Online</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">{deviceStats.online}</div>
            <Progress value={(deviceStats.online / deviceStats.total) * 100} className="mt-2 h-1" />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Alerts</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-baseline gap-3">
              <span className="text-2xl font-bold text-red-600">{deviceStats.criticalAlerts}</span>
              <span className="text-lg text-yellow-600">{deviceStats.warnings}</span>
            </div>
            <div className="text-xs text-muted-foreground mt-1">Critical / Warnings</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Updates</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">{deviceStats.updating}</div>
            <div className="text-xs text-muted-foreground mt-1">Currently updating</div>
          </CardContent>
        </Card>
      </div>

      {/* Filters and Search */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Fleet Devices</CardTitle>
            <div className="flex gap-2">
              <div className="relative">
                <SearchIcon className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search devices..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-8 w-64"
                />
              </div>
              <Button variant="outline" size="sm">
                <FilterIcon className="h-4 w-4 mr-2" />
                Filters
              </Button>
              <Button variant="outline" size="sm">
                <RefreshCwIcon className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="grid" className="space-y-4">
            <TabsList>
              <TabsTrigger value="grid">Grid View</TabsTrigger>
              <TabsTrigger value="list">List View</TabsTrigger>
              <TabsTrigger value="map">Map View</TabsTrigger>
              <TabsTrigger value="metrics">Metrics</TabsTrigger>
            </TabsList>

            <TabsContent value="grid" className="space-y-4">
              <div className="grid grid-cols-3 gap-4">
                {filteredDevices.map((device) => (
                  <Card
                    key={device.id}
                    className="cursor-pointer hover:shadow-lg transition-shadow"
                    onClick={() => onDeviceSelect(device.id)}
                  >
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-2">
                          {getDeviceIcon(device)}
                          <div>
                            <CardTitle className="text-sm">{device.name}</CardTitle>
                            <CardDescription className="text-xs mt-1">
                              {device.systemInfo?.hostname || device.id.slice(0, 8)}
                            </CardDescription>
                          </div>
                        </div>
                        <ActivityIcon className={`h-4 w-4 ${getHealthColor(device)}`} />
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      {/* System Info */}
                      <div className="text-xs space-y-1">
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">OS:</span>
                          <span>
                            {device.systemInfo?.os} {device.systemInfo?.osVersion}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">CPU:</span>
                          <span>{device.systemInfo?.cpuCores || 0} cores</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Memory:</span>
                          <span>{formatBytes(device.systemInfo?.memoryTotal || 0)}</span>
                        </div>
                      </div>

                      {/* Real-time Metrics */}
                      {device.systemInfo?.loadAverage && (
                        <div className="space-y-1">
                          <div className="flex justify-between text-xs">
                            <span className="text-muted-foreground">Load:</span>
                            <span>{device.systemInfo.loadAverage.load1?.toFixed(2)}</span>
                          </div>
                          <Progress
                            value={Math.min((device.systemInfo.loadAverage.load1 || 0) * 20, 100)}
                            className="h-1"
                          />
                        </div>
                      )}

                      {/* Network Status */}
                      {device.systemInfo?.networkInterfaces && (
                        <div className="flex items-center gap-1">
                          <NetworkIcon className="h-3 w-3 text-muted-foreground" />
                          <span className="text-xs">
                            {
                              device.systemInfo.networkInterfaces.filter(
                                (i) => i.isUp && !i.isLoopback,
                              ).length
                            }{' '}
                            active
                          </span>
                        </div>
                      )}

                      {/* Tags/Status */}
                      <div className="flex flex-wrap gap-1">
                        <Badge
                          variant={device.status === 1 ? 'default' : 'secondary'}
                          className="text-xs"
                        >
                          {device.status === 1
                            ? 'Online'
                            : device.status === 2
                              ? 'Offline'
                              : 'Updating'}
                        </Badge>
                        {device.systemInfo?.timezone && (
                          <Badge variant="outline" className="text-xs">
                            {device.systemInfo.timezone.split('/').pop()}
                          </Badge>
                        )}
                        {device.systemInfo?.manufacturer && (
                          <Badge variant="outline" className="text-xs">
                            {device.systemInfo.manufacturer}
                          </Badge>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </TabsContent>

            <TabsContent value="list">
              <div className="space-y-2">
                {filteredDevices.map((device) => (
                  <div
                    key={device.id}
                    className="flex items-center justify-between p-4 border rounded-lg hover:bg-accent cursor-pointer"
                    onClick={() => onDeviceSelect(device.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        onDeviceSelect(device.id)
                      }
                    }}
                  >
                    <div className="flex items-center gap-4">
                      {getDeviceIcon(device)}
                      <div>
                        <div className="font-medium">{device.name}</div>
                        <div className="text-sm text-muted-foreground">
                          {device.systemInfo?.os} • {device.systemInfo?.arch} •{' '}
                          {device.systemInfo?.cpuModel}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-4">
                      <div className="text-right text-sm">
                        <div>{formatBytes(device.systemInfo?.memoryTotal || 0)} RAM</div>
                        <div className="text-muted-foreground">
                          Load: {device.systemInfo?.loadAverage?.load1?.toFixed(2) || 'N/A'}
                        </div>
                      </div>
                      <Badge variant={device.status === 1 ? 'default' : 'secondary'}>
                        {device.status === 1 ? 'Online' : 'Offline'}
                      </Badge>
                    </div>
                  </div>
                ))}
              </div>
            </TabsContent>

            <TabsContent value="map">
              <div className="h-96 flex items-center justify-center border rounded-lg bg-muted/10">
                <div className="text-center">
                  <NetworkIcon className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
                  <p className="text-muted-foreground">Geographic device distribution</p>
                  <p className="text-sm text-muted-foreground mt-2">
                    Map view would show device locations based on IP geolocation
                  </p>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="metrics">
              <div className="grid grid-cols-2 gap-4">
                {/* OS Distribution */}
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">OS Distribution</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-2">
                      {Array.from(deviceStats.byOS.entries()).map(([os, count]) => (
                        <div key={os} className="flex items-center justify-between">
                          <span className="text-sm">{os}</span>
                          <div className="flex items-center gap-2">
                            <Progress
                              value={(count / deviceStats.total) * 100}
                              className="w-24 h-2"
                            />
                            <span className="text-sm text-muted-foreground w-8">{count}</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </CardContent>
                </Card>

                {/* Average System Load */}
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">System Health</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-3">
                      <div>
                        <div className="flex justify-between text-sm mb-1">
                          <span>Average CPU Load</span>
                          <span>
                            {(
                              devices.reduce(
                                (acc, d) => acc + (d.systemInfo?.loadAverage?.load1 || 0),
                                0,
                              ) / devices.length
                            ).toFixed(2)}
                          </span>
                        </div>
                        <Progress value={20} className="h-2" />
                      </div>
                      <div>
                        <div className="flex justify-between text-sm mb-1">
                          <span>Memory Utilization</span>
                          <span>65%</span>
                        </div>
                        <Progress value={65} className="h-2" />
                      </div>
                      <div>
                        <div className="flex justify-between text-sm mb-1">
                          <span>Storage Usage</span>
                          <span>42%</span>
                        </div>
                        <Progress value={42} className="h-2" />
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}

function formatBytes(bytes: bigint | number): string {
  const b = typeof bytes === 'bigint' ? Number(bytes) : bytes
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  if (b === 0) return '0 B'
  const i = Math.floor(Math.log(b) / Math.log(1024))
  return `${(b / 1024 ** i).toFixed(1)} ${sizes[i]}`
}
