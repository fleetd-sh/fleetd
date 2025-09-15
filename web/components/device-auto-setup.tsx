'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { api } from '@/lib/api'
import type { DiscoveredDevice } from '@/lib/api/gen/public/v1/fleet_pb'
import type { Device } from '@/lib/types'
import {
  CheckCircledIcon,
  CrossCircledIcon,
  DownloadIcon,
  GearIcon,
  InfoCircledIcon,
  LightningBoltIcon,
  MagicWandIcon,
  RocketIcon,
} from '@radix-ui/react-icons'
import { useMutation, useQuery } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'framer-motion'
import { useState } from 'react'

interface ProvisioningProfile {
  id: string
  name: string
  description: string
  wifiSSID?: string
  enableSSH: boolean
  autoUpdate: boolean
  plugins: string[]
  createdAt: string
  lastUsed?: string
}

// DiscoveredDevice type is imported from protobuf-generated types

export function DeviceAutoSetup() {
  const { toast } = useToast()
  const [selectedDevices, setSelectedDevices] = useState<Set<string>>(new Set())
  const [selectedProfile, setSelectedProfile] = useState<string>('')
  const [isSetupDialogOpen, setIsSetupDialogOpen] = useState(false)
  const [autoSetupEnabled, setAutoSetupEnabled] = useState(false)

  // Fetch discovered devices
  const { data: discoveredDevices, refetch: refetchDiscovered } = useQuery({
    queryKey: ['discovered-devices'],
    queryFn: api.getDiscoveredDevices,
    refetchInterval: 30000,
  })

  // Fetch provisioning profiles
  const { data: profiles, refetch: refetchProfiles } = useQuery({
    queryKey: ['provisioning-profiles'],
    queryFn: api.getProvisioningProfiles,
  })

  // Auto-setup mutation
  const setupDevicesMutation = useMutation({
    mutationFn: async ({ deviceIds, profileId }: { deviceIds: string[]; profileId: string }) => {
      return api.setupDevices(deviceIds, profileId)
    },
    onSuccess: (data) => {
      toast({
        title: 'Setup Complete',
        description: `Successfully configured ${data.length} device(s)`,
      })
      setSelectedDevices(new Set())
      refetchDiscovered()
    },
    onError: () => {
      toast({
        title: 'Setup Failed',
        description: 'Failed to configure selected devices',
        variant: 'destructive',
      })
    },
  })

  // Create profile mutation
  const createProfileMutation = useMutation({
    mutationFn: api.createProvisioningProfile,
    onSuccess: () => {
      toast({
        title: 'Profile Created',
        description: 'Provisioning profile saved successfully',
      })
      refetchProfiles()
    },
  })

  const handleSelectAll = () => {
    if (!discoveredDevices) return

    const unregistered = discoveredDevices.filter((d) => !d.isRegistered)
    if (selectedDevices.size === unregistered.length) {
      setSelectedDevices(new Set())
    } else {
      setSelectedDevices(new Set(unregistered.map((d) => d.deviceId)))
    }
  }

  const handleAutoSetup = () => {
    if (selectedDevices.size === 0 || !selectedProfile) {
      toast({
        title: 'Selection Required',
        description: 'Please select devices and a provisioning profile',
        variant: 'destructive',
      })
      return
    }

    setupDevicesMutation.mutate({
      deviceIds: Array.from(selectedDevices),
      profileId: selectedProfile,
    })
  }

  const unregisteredDevices = discoveredDevices?.filter((d) => !d.isRegistered) || []

  return (
    <>
      {/* Auto-Setup Card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <MagicWandIcon className="h-5 w-5" />
                Device Auto-Setup
              </CardTitle>
              <CardDescription>
                Automatically configure discovered devices with Fleet credentials
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Label htmlFor="auto-setup">Auto-setup new devices</Label>
              <Switch
                id="auto-setup"
                checked={autoSetupEnabled}
                onCheckedChange={setAutoSetupEnabled}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Discovered Devices Section */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-medium">Unregistered Devices</h3>
              {unregisteredDevices.length > 0 && (
                <Button variant="ghost" size="sm" onClick={handleSelectAll}>
                  {selectedDevices.size === unregisteredDevices.length
                    ? 'Deselect All'
                    : 'Select All'}
                </Button>
              )}
            </div>

            <ScrollArea className="h-[200px] border rounded-lg">
              <AnimatePresence mode="popLayout">
                {unregisteredDevices.length === 0 ? (
                  <motion.div
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    className="flex items-center justify-center h-full text-muted-foreground p-8"
                  >
                    <div className="text-center">
                      <InfoCircledIcon className="h-8 w-8 mx-auto mb-2" />
                      <p>No unregistered devices found</p>
                      <p className="text-xs mt-1">Devices will appear here when discovered</p>
                    </div>
                  </motion.div>
                ) : (
                  <div className="p-3 space-y-2">
                    {unregisteredDevices.map((device, index) => (
                      <motion.div
                        key={device.deviceId}
                        initial={{ opacity: 0, x: -20 }}
                        animate={{ opacity: 1, x: 0 }}
                        transition={{ delay: index * 0.05 }}
                        className={`p-3 rounded-lg border cursor-pointer transition-colors ${
                          selectedDevices.has(device.deviceId)
                            ? 'bg-primary/10 border-primary'
                            : 'hover:bg-muted'
                        }`}
                        onClick={() => {
                          const newSelected = new Set(selectedDevices)
                          if (newSelected.has(device.deviceId)) {
                            newSelected.delete(device.deviceId)
                          } else {
                            newSelected.add(device.deviceId)
                          }
                          setSelectedDevices(newSelected)
                        }}
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <div className="flex items-center gap-2">
                              <span className="font-medium text-sm">
                                {device.deviceName || 'Unknown Device'}
                              </span>
                              {device.version && (
                                <Badge variant="outline" className="text-xs">
                                  v{device.version}
                                </Badge>
                              )}
                            </div>
                            <div className="text-xs text-muted-foreground mt-1">
                              Address: {device.address}:{device.port}
                            </div>
                          </div>
                          {selectedDevices.has(device.deviceId) && (
                            <CheckCircledIcon className="h-5 w-5 text-primary" />
                          )}
                        </div>
                      </motion.div>
                    ))}
                  </div>
                )}
              </AnimatePresence>
            </ScrollArea>
          </div>

          {/* Provisioning Profile Selection */}
          <div>
            <Label htmlFor="profile">Provisioning Profile</Label>
            <div className="flex gap-2 mt-2">
              <Select value={selectedProfile} onValueChange={setSelectedProfile}>
                <SelectTrigger className="flex-1">
                  <SelectValue placeholder="Select a profile" />
                </SelectTrigger>
                <SelectContent>
                  {profiles?.map((profile) => (
                    <SelectItem key={profile.id} value={profile.id}>
                      <div>
                        <div className="font-medium">{profile.name}</div>
                        <div className="text-xs text-muted-foreground">{profile.description}</div>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Dialog>
                <DialogTrigger asChild>
                  <Button variant="outline" size="icon">
                    <GearIcon className="h-4 w-4" />
                  </Button>
                </DialogTrigger>
                <DialogContent className="max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>Manage Provisioning Profiles</DialogTitle>
                    <DialogDescription>
                      Create and manage reusable device configuration profiles
                    </DialogDescription>
                  </DialogHeader>
                  <ProfileManager
                    profiles={profiles || []}
                    onCreateProfile={(profile) => {
                      if (profile.name && profile.description) {
                        createProfileMutation.mutate({
                          name: profile.name,
                          description: profile.description,
                          wifiSSID: profile.wifiSSID,
                          enableSSH: profile.enableSSH || false,
                          autoUpdate: profile.autoUpdate || false,
                          plugins: profile.plugins || [],
                        })
                      }
                    }}
                  />
                </DialogContent>
              </Dialog>
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex gap-2">
            <Button
              className="flex-1"
              onClick={handleAutoSetup}
              disabled={
                selectedDevices.size === 0 || !selectedProfile || setupDevicesMutation.isPending
              }
            >
              {setupDevicesMutation.isPending ? (
                <>Loading...</>
              ) : (
                <>
                  <RocketIcon className="mr-2 h-4 w-4" />
                  Setup {selectedDevices.size} Device(s)
                </>
              )}
            </Button>

            <Dialog open={isSetupDialogOpen} onOpenChange={setIsSetupDialogOpen}>
              <DialogTrigger asChild>
                <Button variant="outline">
                  <DownloadIcon className="mr-2 h-4 w-4" />
                  Manual Setup
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Manual Device Setup</DialogTitle>
                  <DialogDescription>
                    Download configuration or run setup commands on devices
                  </DialogDescription>
                </DialogHeader>
                <ManualSetupInstructions profileId={selectedProfile} />
              </DialogContent>
            </Dialog>
          </div>

          {/* Auto-Setup Status */}
          {autoSetupEnabled && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              className="p-3 bg-primary/5 rounded-lg border border-primary/20"
            >
              <div className="flex items-center gap-2">
                <LightningBoltIcon className="h-4 w-4 text-primary" />
                <span className="text-sm font-medium">Auto-Setup Active</span>
              </div>
              <p className="text-xs text-muted-foreground mt-1">
                New devices will be automatically configured using the selected profile
              </p>
            </motion.div>
          )}
        </CardContent>
      </Card>
    </>
  )
}

// Profile Manager Component
function ProfileManager({
  profiles,
  onCreateProfile,
}: {
  profiles: ProvisioningProfile[]
  onCreateProfile: (profile: Partial<ProvisioningProfile>) => void
}) {
  const [newProfile, setNewProfile] = useState<Partial<ProvisioningProfile>>({
    name: '',
    description: '',
    wifiSSID: '',
    enableSSH: true,
    autoUpdate: true,
    plugins: [],
  })

  return (
    <Tabs defaultValue="existing" className="w-full">
      <TabsList className="grid w-full grid-cols-2">
        <TabsTrigger value="existing">Existing Profiles</TabsTrigger>
        <TabsTrigger value="create">Create New</TabsTrigger>
      </TabsList>

      <TabsContent value="existing" className="space-y-2">
        <ScrollArea className="h-[300px]">
          {profiles.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">No profiles created yet</div>
          ) : (
            <div className="space-y-2">
              {profiles.map((profile) => (
                <Card key={profile.id}>
                  <CardHeader className="p-4">
                    <div className="flex justify-between items-start">
                      <div>
                        <CardTitle className="text-base">{profile.name}</CardTitle>
                        <CardDescription className="text-xs mt-1">
                          {profile.description}
                        </CardDescription>
                      </div>
                      <Badge variant="outline" className="text-xs">
                        {profile.plugins.length} plugins
                      </Badge>
                    </div>
                    <div className="flex gap-4 mt-3 text-xs text-muted-foreground">
                      {profile.wifiSSID && <span>WiFi: {profile.wifiSSID}</span>}
                      {profile.enableSSH && <span>SSH Enabled</span>}
                      {profile.autoUpdate && <span>Auto-Update</span>}
                    </div>
                  </CardHeader>
                </Card>
              ))}
            </div>
          )}
        </ScrollArea>
      </TabsContent>

      <TabsContent value="create" className="space-y-4">
        <div className="space-y-3">
          <div>
            <Label htmlFor="profile-name">Profile Name</Label>
            <Input
              id="profile-name"
              value={newProfile.name}
              onChange={(e) => setNewProfile({ ...newProfile, name: e.target.value })}
              placeholder="e.g., Production Devices"
            />
          </div>

          <div>
            <Label htmlFor="profile-desc">Description</Label>
            <Input
              id="profile-desc"
              value={newProfile.description}
              onChange={(e) => setNewProfile({ ...newProfile, description: e.target.value })}
              placeholder="Configuration for production environment"
            />
          </div>

          <div>
            <Label htmlFor="wifi-ssid">WiFi SSID (Optional)</Label>
            <Input
              id="wifi-ssid"
              value={newProfile.wifiSSID}
              onChange={(e) => setNewProfile({ ...newProfile, wifiSSID: e.target.value })}
              placeholder="Network name"
            />
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="enable-ssh">Enable SSH Access</Label>
            <Switch
              id="enable-ssh"
              checked={newProfile.enableSSH}
              onCheckedChange={(checked) => setNewProfile({ ...newProfile, enableSSH: checked })}
            />
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="auto-update">Enable Auto-Updates</Label>
            <Switch
              id="auto-update"
              checked={newProfile.autoUpdate}
              onCheckedChange={(checked) => setNewProfile({ ...newProfile, autoUpdate: checked })}
            />
          </div>

          <Button
            className="w-full"
            onClick={() => onCreateProfile(newProfile)}
            disabled={!newProfile.name}
          >
            Create Profile
          </Button>
        </div>
      </TabsContent>
    </Tabs>
  )
}

// Manual Setup Instructions Component
function ManualSetupInstructions({ profileId }: { profileId: string }) {
  const serverUrl = typeof window !== 'undefined' ? window.location.origin : ''

  const setupScript = `#!/bin/bash
# FleetD Device Setup Script
# Generated for profile: ${profileId}

FLEET_SERVER="${serverUrl}"
PROFILE_ID="${profileId}"

# Download and install FleetD agent
curl -sSL $FLEET_SERVER/install.sh | bash

# Configure with profile
fleetd configure --server $FLEET_SERVER --profile $PROFILE_ID

# Start agent
systemctl enable fleetd
systemctl start fleetd

echo "Device configured successfully!"
`

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-medium mb-2">Option 1: Run Setup Script</h3>
        <pre className="bg-muted p-3 rounded-lg text-xs overflow-x-auto">
          <code>{setupScript}</code>
        </pre>
        <Button
          variant="outline"
          size="sm"
          className="mt-2"
          onClick={() => {
            navigator.clipboard.writeText(setupScript)
          }}
        >
          Copy Script
        </Button>
      </div>

      <div>
        <h3 className="text-sm font-medium mb-2">Option 2: Manual Configuration</h3>
        <ol className="text-sm space-y-2">
          <li>1. SSH into your device</li>
          <li>
            2. Download FleetD agent:
            <code className="bg-muted px-2 py-1 rounded text-xs block mt-1">
              curl -sSL {serverUrl}/install.sh | bash
            </code>
          </li>
          <li>
            3. Configure the agent:
            <code className="bg-muted px-2 py-1 rounded text-xs block mt-1">
              fleetd configure --server {serverUrl}
            </code>
          </li>
          <li>
            4. Start the service:
            <code className="bg-muted px-2 py-1 rounded text-xs block mt-1">
              systemctl start fleetd
            </code>
          </li>
        </ol>
      </div>
    </div>
  )
}
