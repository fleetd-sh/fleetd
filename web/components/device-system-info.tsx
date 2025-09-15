'use client'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { Device } from '@/lib/api/gen/public/v1/fleet_pb'
import { CpuIcon, HardDriveIcon, MemoryStickIcon, ServerIcon } from 'lucide-react'

type SystemInfo = NonNullable<Device['systemInfo']>

interface DeviceSystemInfoProps {
  systemInfo?: SystemInfo
}

export function DeviceSystemInfo({ systemInfo }: DeviceSystemInfoProps) {
  if (!systemInfo) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>System Information</CardTitle>
          <CardDescription>No system information available</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const formatBytes = (bytes: bigint | number): string => {
    const b = typeof bytes === 'bigint' ? Number(bytes) : bytes
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    if (b === 0) return '0 B'
    const i = Math.floor(Math.log(b) / Math.log(1024))
    return `${(b / 1024 ** i).toFixed(2)} ${sizes[i]}`
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>System Information</CardTitle>
        <CardDescription>Hardware and software details</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Host Information */}
        <div>
          <h3 className="font-semibold mb-3 flex items-center gap-2">
            <ServerIcon className="h-4 w-4" />
            Host
          </h3>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-muted-foreground">Hostname:</span>
              <p className="font-mono">{systemInfo.hostname || 'Unknown'}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Platform:</span>
              <p className="font-mono">{systemInfo.platform || 'Unknown'}</p>
            </div>
            <div>
              <span className="text-muted-foreground">OS:</span>
              <p className="font-mono">
                {systemInfo.os} {systemInfo.osVersion}
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">Kernel:</span>
              <p className="font-mono text-xs">{systemInfo.kernelVersion || 'Unknown'}</p>
            </div>
          </div>
        </div>

        {/* CPU Information */}
        <div>
          <h3 className="font-semibold mb-3 flex items-center gap-2">
            <CpuIcon className="h-4 w-4" />
            CPU
          </h3>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-muted-foreground">Model:</span>
              <p className="font-mono text-xs">{systemInfo.cpuModel || 'Unknown'}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Architecture:</span>
              <p className="font-mono">{systemInfo.arch || 'Unknown'}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Cores:</span>
              <p className="font-mono">{systemInfo.cpuCores || 0}</p>
            </div>
            {systemInfo.extra?.cpu_logical_cores && (
              <div>
                <span className="text-muted-foreground">Logical Cores:</span>
                <p className="font-mono">{systemInfo.extra.cpu_logical_cores}</p>
              </div>
            )}
          </div>
        </div>

        {/* Memory Information */}
        <div>
          <h3 className="font-semibold mb-3 flex items-center gap-2">
            <MemoryStickIcon className="h-4 w-4" />
            Memory
          </h3>
          <div className="text-sm">
            <span className="text-muted-foreground">Total:</span>
            <p className="font-mono">{formatBytes(systemInfo.memoryTotal || 0)}</p>
          </div>
        </div>

        {/* Storage Information */}
        <div>
          <h3 className="font-semibold mb-3 flex items-center gap-2">
            <HardDriveIcon className="h-4 w-4" />
            Storage
          </h3>
          <div className="text-sm">
            <span className="text-muted-foreground">Total:</span>
            <p className="font-mono">{formatBytes(systemInfo.storageTotal || 0)}</p>
          </div>
        </div>

        {/* Additional Information */}
        {systemInfo.extra && Object.keys(systemInfo.extra).length > 0 && (
          <div>
            <h3 className="font-semibold mb-3">Additional Info</h3>
            <div className="flex flex-wrap gap-2">
              {systemInfo.extra.virtualization_system && (
                <Badge variant="secondary">
                  Virt: {systemInfo.extra.virtualization_system} (
                  {systemInfo.extra.virtualization_role})
                </Badge>
              )}
              {systemInfo.extra.go_version && (
                <Badge variant="secondary">Go: {systemInfo.extra.go_version}</Badge>
              )}
              {systemInfo.extra.boot_time && (
                <Badge variant="secondary">
                  Boot:{' '}
                  {new Date(Number.parseInt(systemInfo.extra.boot_time) * 1000).toLocaleString()}
                </Badge>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
