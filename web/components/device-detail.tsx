'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useDevice } from '@/lib/api/hooks'
import { format } from 'date-fns'
import { RefreshCwIcon, SettingsIcon, TrashIcon } from 'lucide-react'
import { DeviceSystemInfo } from './device-system-info'

interface DeviceDetailProps {
  deviceId: string
}

export function DeviceDetail({ deviceId }: DeviceDetailProps) {
  const { data: device, isLoading, error, refetch } = useDevice(deviceId)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (error || !device) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Device Not Found</CardTitle>
          <CardDescription>Unable to load device details</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      {/* Device Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-2xl">{device.name}</CardTitle>
              <CardDescription>
                {device.id} • Last seen{' '}
                {device.lastSeen
                  ? format(new Date(Number(device.lastSeen.seconds) * 1000), 'PPpp')
                  : 'Never'}
              </CardDescription>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => refetch()}>
                <RefreshCwIcon className="h-4 w-4 mr-2" />
                Refresh
              </Button>
              <Button variant="outline" size="sm">
                <SettingsIcon className="h-4 w-4 mr-2" />
                Configure
              </Button>
              <Button variant="destructive" size="sm">
                <TrashIcon className="h-4 w-4 mr-2" />
                Remove
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Badge variant={device.status === 1 ? 'default' : 'secondary'}>
              {device.status === 1 ? 'Online' : 'Offline'}
            </Badge>
            <Badge variant="outline">Type: {device.type}</Badge>
            <Badge variant="outline">Version: {device.version}</Badge>
            {device.systemInfo && (
              <>
                <Badge variant="outline">OS: {device.systemInfo.os}</Badge>
                <Badge variant="outline">Arch: {device.systemInfo.arch}</Badge>
              </>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Tabbed Content */}
      <Tabs defaultValue="system" className="w-full">
        <TabsList className="grid w-full grid-cols-4">
          <TabsTrigger value="system">System</TabsTrigger>
          <TabsTrigger value="telemetry">Telemetry</TabsTrigger>
          <TabsTrigger value="updates">Updates</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="system" className="space-y-4">
          <DeviceSystemInfo systemInfo={device.systemInfo} />

          {/* Capabilities */}
          {device.metadata && Object.keys(device.metadata).length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Capabilities</CardTitle>
                <CardDescription>Device features and configuration</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  {Object.entries(device.metadata).map(([key, value]) => (
                    <div key={key}>
                      <span className="text-muted-foreground">{key}:</span>
                      <p className="font-mono">{String(value)}</p>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="telemetry">
          <Card>
            <CardHeader>
              <CardTitle>Telemetry</CardTitle>
              <CardDescription>Real-time metrics and monitoring</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground">Telemetry data will be displayed here</p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="updates">
          <Card>
            <CardHeader>
              <CardTitle>Updates</CardTitle>
              <CardDescription>Update history and available updates</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground">Update information will be displayed here</p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logs">
          <Card>
            <CardHeader>
              <CardTitle>Logs</CardTitle>
              <CardDescription>Device logs and events</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground">Device logs will be displayed here</p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
