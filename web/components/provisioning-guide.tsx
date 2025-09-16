'use client'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { CheckCircledIcon, CopyIcon, DesktopIcon, RocketIcon } from '@radix-ui/react-icons'
import { motion } from 'framer-motion'
import { useState } from 'react'

interface ProvisionConfig {
  deviceType: 'raspberrypi' | 'x86' | 'jetson' | 'custom'
  setupType: 'k3s-server' | 'k3s-worker' | 'standalone' | 'docker-host'
  devicePath: string
  deviceName: string
  wifiSSID: string
  wifiPassword: string
  fleetServer: string
  sshKey: string
  k3sToken: string
  k3sServerUrl: string
  imageUrl: string
  plugins: string[]
}

const DEFAULT_IMAGES = {
  raspberrypi:
    'https://downloads.raspberrypi.org/raspios_lite_arm64/images/raspios_lite_arm64-2024-03-15/2024-03-15-raspios-bookworm-arm64-lite.img.xz',
  x86: 'https://releases.ubuntu.com/22.04/ubuntu-22.04.3-live-server-amd64.iso',
  jetson: 'https://developer.nvidia.com/jetson-nano-sd-card-image',
  custom: '',
}

export function ProvisioningGuide() {
  const { toast } = useToast()
  const [currentStep, setCurrentStep] = useState(1)
  const [config, setConfig] = useState<ProvisionConfig>({
    deviceType: 'raspberrypi',
    setupType: 'standalone',
    devicePath: '/dev/disk2',
    deviceName: '',
    wifiSSID: '',
    wifiPassword: '',
    fleetServer: `http://${window.location.hostname}:8080`,
    sshKey: '',
    k3sToken: '',
    k3sServerUrl: '',
    imageUrl: DEFAULT_IMAGES.raspberrypi,
    plugins: [],
  })

  const updateConfig = (key: keyof ProvisionConfig, value: any) => {
    setConfig((prev) => ({
      ...prev,
      [key]: value,
      // Auto-update image URL when device type changes
      ...(key === 'deviceType'
        ? { imageUrl: DEFAULT_IMAGES[value as keyof typeof DEFAULT_IMAGES] }
        : {}),
    }))
  }

  const generateCommand = (): string => {
    const cmd = ['fleet provision']

    // Required parameters
    cmd.push(`--device ${config.devicePath}`)

    if (config.deviceName) {
      cmd.push(`--name "${config.deviceName}"`)
    }

    // Network configuration
    if (config.wifiSSID) {
      cmd.push(`--wifi-ssid "${config.wifiSSID}"`)
      if (config.wifiPassword) {
        cmd.push(`--wifi-pass "${config.wifiPassword}"`)
      }
    }

    // Fleet server
    cmd.push(`--fleet-server ${config.fleetServer}`)

    // SSH key
    if (config.sshKey) {
      cmd.push(`--ssh-key-file ${config.sshKey}`)
    }

    // Custom image
    if (config.imageUrl && config.imageUrl !== DEFAULT_IMAGES[config.deviceType]) {
      cmd.push(`--image-url "${config.imageUrl}"`)
    }

    // k3s configuration
    if (config.setupType === 'k3s-server') {
      cmd.push('--plugin k3s')
      cmd.push('--plugin-opt k3s.role=server')
    } else if (config.setupType === 'k3s-worker') {
      cmd.push('--plugin k3s')
      cmd.push('--plugin-opt k3s.role=agent')
      if (config.k3sServerUrl) {
        cmd.push(`--plugin-opt k3s.server=${config.k3sServerUrl}`)
      }
      if (config.k3sToken) {
        cmd.push(`--plugin-opt k3s.token=${config.k3sToken}`)
      }
    } else if (config.setupType === 'docker-host') {
      cmd.push('--plugin docker')
    }

    // Additional plugins
    config.plugins.forEach((plugin) => {
      cmd.push(`--plugin ${plugin}`)
    })

    return cmd.join(' \\\n  ')
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast({
      title: 'Copied to clipboard',
      description: 'Command has been copied to your clipboard',
    })
  }

  const Step1DeviceType = () => (
    <motion.div
      initial={{ opacity: 0, x: 20 }}
      animate={{ opacity: 1, x: 0 }}
      className="space-y-4"
    >
      <div>
        <h3 className="text-lg font-semibold mb-2">Select Device Type</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Choose the type of device you want to provision
        </p>
      </div>

      <RadioGroup
        value={config.deviceType}
        onValueChange={(value) => updateConfig('deviceType', value)}
      >
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('deviceType', 'raspberrypi')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="raspberrypi" />
                <CardTitle className="text-base">Raspberry Pi</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Raspberry Pi 3, 4, 5, or Zero 2 W with Raspberry Pi OS
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('deviceType', 'x86')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="x86" />
                <CardTitle className="text-base">x86/x64 PC</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Intel NUC, Mini PC, or standard computer with Ubuntu
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('deviceType', 'jetson')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="jetson" />
                <CardTitle className="text-base">NVIDIA Jetson</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Jetson Nano, Xavier NX, or Orin for AI workloads
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('deviceType', 'custom')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="custom" />
                <CardTitle className="text-base">Custom Device</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Other ARM or x86 devices with custom OS image
              </p>
            </CardContent>
          </Card>
        </div>
      </RadioGroup>
    </motion.div>
  )

  const Step2SetupType = () => (
    <motion.div
      initial={{ opacity: 0, x: 20 }}
      animate={{ opacity: 1, x: 0 }}
      className="space-y-4"
    >
      <div>
        <h3 className="text-lg font-semibold mb-2">Select Setup Type</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Choose how you want to configure this device
        </p>
      </div>

      <RadioGroup
        value={config.setupType}
        onValueChange={(value) => updateConfig('setupType', value)}
      >
        <div className="grid grid-cols-1 gap-4">
          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('setupType', 'standalone')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="standalone" />
                <CardTitle className="text-base">Standalone Device</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Basic FleetD agent for monitoring and remote management
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('setupType', 'k3s-server')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="k3s-server" />
                <CardTitle className="text-base">k3s Server (Control Plane)</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Kubernetes control plane node for cluster management
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('setupType', 'k3s-worker')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="k3s-worker" />
                <CardTitle className="text-base">k3s Worker Node</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Kubernetes worker node to join existing cluster
              </p>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer hover:border-primary"
            onClick={() => updateConfig('setupType', 'docker-host')}
          >
            <CardHeader className="pb-3">
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="docker-host" />
                <CardTitle className="text-base">Docker Host</CardTitle>
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Container runtime for deploying Docker applications
              </p>
            </CardContent>
          </Card>
        </div>
      </RadioGroup>
    </motion.div>
  )

  const Step3Configuration = () => (
    <motion.div
      initial={{ opacity: 0, x: 20 }}
      animate={{ opacity: 1, x: 0 }}
      className="space-y-4"
    >
      <div>
        <h3 className="text-lg font-semibold mb-2">Device Configuration</h3>
        <p className="text-sm text-muted-foreground mb-4">Configure device-specific settings</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="device-path">Device Path *</Label>
          <Input
            id="device-path"
            value={config.devicePath}
            onChange={(e) => updateConfig('devicePath', e.target.value)}
            placeholder="/dev/disk2 or /dev/sdb"
          />
          <p className="text-xs text-muted-foreground">
            SD card or USB device path (use `diskutil list` on macOS or `lsblk` on Linux)
          </p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="device-name">Device Name *</Label>
          <Input
            id="device-name"
            value={config.deviceName}
            onChange={(e) => updateConfig('deviceName', e.target.value)}
            placeholder="edge-device-01"
          />
          <p className="text-xs text-muted-foreground">Unique name for this device</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="wifi-ssid">WiFi SSID</Label>
          <Input
            id="wifi-ssid"
            value={config.wifiSSID}
            onChange={(e) => updateConfig('wifiSSID', e.target.value)}
            placeholder="MyNetwork"
          />
          <p className="text-xs text-muted-foreground">Leave empty for ethernet-only</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="wifi-pass">WiFi Password</Label>
          <Input
            id="wifi-pass"
            type="password"
            value={config.wifiPassword}
            onChange={(e) => updateConfig('wifiPassword', e.target.value)}
            placeholder="••••••••"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="ssh-key">SSH Key File</Label>
          <Input
            id="ssh-key"
            value={config.sshKey}
            onChange={(e) => updateConfig('sshKey', e.target.value)}
            placeholder="~/.ssh/id_rsa.pub"
          />
          <p className="text-xs text-muted-foreground">Path to SSH public key for remote access</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="fleet-server">Fleet Server URL</Label>
          <Input
            id="fleet-server"
            value={config.fleetServer}
            onChange={(e) => updateConfig('fleetServer', e.target.value)}
            placeholder="http://192.168.1.100:8080"
          />
          <p className="text-xs text-muted-foreground">Fleet management server endpoint</p>
        </div>

        {config.setupType === 'k3s-worker' && (
          <>
            <div className="space-y-2">
              <Label htmlFor="k3s-server">k3s Server URL</Label>
              <Input
                id="k3s-server"
                value={config.k3sServerUrl}
                onChange={(e) => updateConfig('k3sServerUrl', e.target.value)}
                placeholder="https://k3s-server.local:6443"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="k3s-token">k3s Join Token</Label>
              <Input
                id="k3s-token"
                type="password"
                value={config.k3sToken}
                onChange={(e) => updateConfig('k3sToken', e.target.value)}
                placeholder="K10abc..."
              />
              <p className="text-xs text-muted-foreground">
                Get from server: `sudo cat /var/lib/rancher/k3s/server/node-token`
              </p>
            </div>
          </>
        )}

        {config.deviceType === 'custom' && (
          <div className="col-span-2 space-y-2">
            <Label htmlFor="image-url">Custom Image URL</Label>
            <Input
              id="image-url"
              value={config.imageUrl}
              onChange={(e) => updateConfig('imageUrl', e.target.value)}
              placeholder="https://example.com/custom-os.img.xz"
            />
          </div>
        )}
      </div>
    </motion.div>
  )

  const Step4Review = () => {
    const command = generateCommand()

    return (
      <motion.div
        initial={{ opacity: 0, x: 20 }}
        animate={{ opacity: 1, x: 0 }}
        className="space-y-4"
      >
        <div>
          <h3 className="text-lg font-semibold mb-2">Review & Execute</h3>
          <p className="text-sm text-muted-foreground mb-4">
            Copy and run this command in your terminal
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Generated Command</CardTitle>
            <CardDescription>
              Run this command on your local machine with the Fleet CLI installed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="relative">
              <pre className="bg-muted p-4 rounded-lg overflow-x-auto text-sm">
                <code>{command}</code>
              </pre>
              <Button
                size="sm"
                variant="outline"
                className="absolute top-2 right-2"
                onClick={() => copyToClipboard(command)}
              >
                <CopyIcon className="mr-2 h-4 w-4" />
                Copy
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Next Steps</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5" />
              <div>
                <p className="font-medium">1. Install Fleet CLI</p>
                <p className="text-sm text-muted-foreground">
                  {`go install fleetd.sh/cmd/fleet@latest`}
                </p>
              </div>
            </div>

            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5" />
              <div>
                <p className="font-medium">2. Insert SD Card / USB Drive</p>
                <p className="text-sm text-muted-foreground">
                  Make sure the device path matches: {config.devicePath}
                </p>
              </div>
            </div>

            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5" />
              <div>
                <p className="font-medium">3. Run the Command</p>
                <p className="text-sm text-muted-foreground">
                  This will download the OS image and write it to your device
                </p>
              </div>
            </div>

            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5" />
              <div>
                <p className="font-medium">4. Boot the Device</p>
                <p className="text-sm text-muted-foreground">
                  Insert the SD card/USB and power on. Device will auto-register in ~2-3 minutes
                </p>
              </div>
            </div>

            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5" />
              <div>
                <p className="font-medium">5. Check Dashboard</p>
                <p className="text-sm text-muted-foreground">
                  Return here to see your device appear with real-time telemetry
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        {config.setupType === 'k3s-server' && (
          <Card className="border-blue-200 bg-blue-50 dark:bg-blue-950">
            <CardHeader>
              <CardTitle className="text-base flex items-center">
                <RocketIcon className="mr-2 h-4 w-4" />
                k3s Server Token
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm">
                After the server boots, get the join token for worker nodes:
              </p>
              <pre className="bg-background p-2 rounded mt-2 text-xs">
                ssh {config.deviceName}.local sudo cat /var/lib/rancher/k3s/server/node-token
              </pre>
            </CardContent>
          </Card>
        )}
      </motion.div>
    )
  }

  const steps = [
    { number: 1, title: 'Device Type', component: Step1DeviceType },
    { number: 2, title: 'Setup Type', component: Step2SetupType },
    { number: 3, title: 'Configuration', component: Step3Configuration },
    { number: 4, title: 'Review', component: Step4Review },
  ]

  const CurrentStepComponent = steps[currentStep - 1].component

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Device Provisioning Guide</h2>
        <p className="text-muted-foreground mt-2">
          Follow this guide to provision new devices with FleetD
        </p>
      </div>

      {/* Step indicator */}
      <div className="flex items-center justify-between mb-8">
        {steps.map((step, index) => (
          <div key={step.number} className="flex items-center">
            <button
              onClick={() => setCurrentStep(step.number)}
              className={`
                flex items-center justify-center w-10 h-10 rounded-full font-medium
                ${
                  currentStep >= step.number
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-muted text-muted-foreground'
                }
                ${currentStep === step.number ? 'ring-2 ring-primary ring-offset-2' : ''}
                transition-all cursor-pointer hover:scale-105
              `}
            >
              {step.number}
            </button>
            {index < steps.length - 1 && (
              <div
                className={`
                w-full h-1 mx-2
                ${currentStep > step.number ? 'bg-primary' : 'bg-muted'}
                transition-colors
              `}
              />
            )}
          </div>
        ))}
      </div>

      {/* Step content */}
      <Card>
        <CardContent className="pt-6">
          <CurrentStepComponent />
        </CardContent>
      </Card>

      {/* Navigation buttons */}
      <div className="flex justify-between">
        <Button
          variant="outline"
          onClick={() => setCurrentStep(Math.max(1, currentStep - 1))}
          disabled={currentStep === 1}
        >
          Previous
        </Button>

        <Button
          onClick={() => setCurrentStep(Math.min(steps.length, currentStep + 1))}
          disabled={currentStep === steps.length}
        >
          Next
        </Button>
      </div>

      {/* Quick templates */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Quick Templates</CardTitle>
          <CardDescription>
            Common provisioning scenarios - click to load configuration
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Button
              variant="outline"
              className="justify-start"
              onClick={() => {
                setConfig({
                  ...config,
                  deviceType: 'raspberrypi',
                  setupType: 'k3s-server',
                  deviceName: 'k3s-master-01',
                  plugins: ['k3s'],
                })
                setCurrentStep(3)
              }}
            >
              <DesktopIcon className="mr-2 h-4 w-4" />
              RPi k3s Server
            </Button>

            <Button
              variant="outline"
              className="justify-start"
              onClick={() => {
                setConfig({
                  ...config,
                  deviceType: 'raspberrypi',
                  setupType: 'k3s-worker',
                  deviceName: 'k3s-worker-01',
                  plugins: ['k3s'],
                })
                setCurrentStep(3)
              }}
            >
              <DesktopIcon className="mr-2 h-4 w-4" />
              RPi k3s Worker
            </Button>

            <Button
              variant="outline"
              className="justify-start"
              onClick={() => {
                setConfig({
                  ...config,
                  deviceType: 'jetson',
                  setupType: 'docker-host',
                  deviceName: 'jetson-edge-01',
                  plugins: ['docker', 'nvidia'],
                })
                setCurrentStep(3)
              }}
            >
              <RocketIcon className="mr-2 h-4 w-4" />
              Jetson AI Node
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
