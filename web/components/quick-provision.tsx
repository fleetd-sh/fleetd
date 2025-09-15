'use client'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { CheckCircledIcon, CopyIcon, DesktopIcon, RocketIcon } from '@radix-ui/react-icons'
import { useState } from 'react'

export function QuickProvision() {
  const { toast } = useToast()
  const [selectedTab, setSelectedTab] = useState('raspberrypi')

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast({
      title: 'Copied!',
      description: 'Command copied to clipboard',
    })
  }

  const fleetServerUrl = `http://${window.location.hostname}:8080`

  const templates = {
    raspberrypi: {
      title: 'Raspberry Pi',
      description: 'Quick setup for Raspberry Pi devices',
      scenarios: [
        {
          name: 'k3s Server (Control Plane)',
          icon: <DesktopIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/disk2 \\
  --name "k3s-server-01" \\
  --wifi-ssid "YOUR_WIFI" \\
  --wifi-pass "YOUR_PASSWORD" \\
  --plugin k3s \\
  --plugin-opt k3s.role=server \\
  --fleet-server ${fleetServerUrl}`,
        },
        {
          name: 'k3s Worker Node',
          icon: <DesktopIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/disk2 \\
  --name "k3s-worker-01" \\
  --wifi-ssid "YOUR_WIFI" \\
  --wifi-pass "YOUR_PASSWORD" \\
  --plugin k3s \\
  --plugin-opt k3s.role=agent \\
  --plugin-opt k3s.server=https://k3s-server-01.local:6443 \\
  --plugin-opt k3s.token=YOUR_K3S_TOKEN \\
  --fleet-server ${fleetServerUrl}`,
        },
        {
          name: 'Standalone Device',
          icon: <CheckCircledIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/disk2 \\
  --name "rpi-device-01" \\
  --wifi-ssid "YOUR_WIFI" \\
  --wifi-pass "YOUR_PASSWORD" \\
  --fleet-server ${fleetServerUrl}`,
        },
      ],
    },
    jetson: {
      title: 'NVIDIA Jetson',
      description: 'AI-enabled edge devices',
      scenarios: [
        {
          name: 'Jetson with Docker & NVIDIA Runtime',
          icon: <RocketIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/disk2 \\
  --name "jetson-ai-01" \\
  --plugin docker \\
  --plugin nvidia \\
  --fleet-server ${fleetServerUrl}`,
        },
        {
          name: 'Jetson k3s with GPU Support',
          icon: <RocketIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/disk2 \\
  --name "jetson-k3s-01" \\
  --plugin k3s \\
  --plugin nvidia \\
  --plugin-opt k3s.role=server \\
  --plugin-opt k3s.args="--docker" \\
  --fleet-server ${fleetServerUrl}`,
        },
      ],
    },
    x86: {
      title: 'x86/x64 Devices',
      description: 'Intel NUC, Mini PCs, and standard computers',
      scenarios: [
        {
          name: 'Ubuntu Server',
          icon: <DesktopIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/sdb \\
  --name "edge-server-01" \\
  --image-url https://releases.ubuntu.com/22.04/ubuntu-22.04.3-live-server-amd64.iso \\
  --fleet-server ${fleetServerUrl}`,
        },
        {
          name: 'Docker Host',
          icon: <DesktopIcon className="h-4 w-4" />,
          command: `fleet provision \\
  --device /dev/sdb \\
  --name "docker-host-01" \\
  --plugin docker \\
  --plugin docker-compose \\
  --fleet-server ${fleetServerUrl}`,
        },
      ],
    },
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Quick Provisioning Commands</CardTitle>
        <CardDescription>
          Copy these commands to quickly provision common device configurations
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs value={selectedTab} onValueChange={setSelectedTab}>
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="raspberrypi">Raspberry Pi</TabsTrigger>
            <TabsTrigger value="jetson">NVIDIA Jetson</TabsTrigger>
            <TabsTrigger value="x86">x86 Devices</TabsTrigger>
          </TabsList>

          {Object.entries(templates).map(([key, template]) => (
            <TabsContent key={key} value={key} className="space-y-4">
              <div className="text-sm text-muted-foreground">{template.description}</div>

              {template.scenarios.map((scenario, index) => (
                <Card key={index}>
                  <CardHeader className="pb-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2">
                        {scenario.icon}
                        <h4 className="font-medium">{scenario.name}</h4>
                      </div>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => copyToClipboard(scenario.command)}
                      >
                        <CopyIcon className="mr-2 h-3 w-3" />
                        Copy
                      </Button>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <pre className="bg-muted p-3 rounded-md overflow-x-auto text-xs">
                      <code>{scenario.command}</code>
                    </pre>
                  </CardContent>
                </Card>
              ))}

              <div className="bg-blue-50 dark:bg-blue-950 p-4 rounded-lg">
                <h4 className="font-medium text-sm mb-2">Before Running:</h4>
                <ul className="text-xs space-y-1 text-muted-foreground">
                  <li>
                    • Replace device path (check with <code>diskutil list</code> on macOS or{' '}
                    <code>lsblk</code> on Linux)
                  </li>
                  <li>• Update WiFi credentials if using wireless</li>
                  <li>
                    • For k3s workers, get token from server:{' '}
                    <code>sudo cat /var/lib/rancher/k3s/server/node-token</code>
                  </li>
                  <li>
                    • Ensure Fleet CLI is installed:{' '}
                    <code>go install fleetd.sh/cmd/fleet@latest</code>
                  </li>
                </ul>
              </div>
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  )
}
