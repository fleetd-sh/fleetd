export interface ProvisioningProfile {
  id: string
  name: string
  description: string
  wifiSSID?: string
  wifiPassword?: string
  enableSSH: boolean
  sshPublicKey?: string
  autoUpdate: boolean
  plugins: string[]
  pluginConfigs?: Record<string, unknown>
  createdAt: string
  updatedAt: string
  lastUsed?: string
  usageCount: number
}

// DiscoveredDevice type is now imported from protobuf-generated types
// See: lib/api/gen/public/v1/fleet_pb.ts

export interface DeviceSetupRequest {
  deviceIds: string[]
  profileId: string
}

export interface DeviceSetupResponse {
  deviceId: string
  success: boolean
  message?: string
  error?: string
}

export interface CreateProfileRequest {
  name: string
  description: string
  wifiSSID?: string
  wifiPassword?: string
  enableSSH: boolean
  sshPublicKey?: string
  autoUpdate: boolean
  plugins: string[]
  pluginConfigs?: Record<string, unknown>
}

export interface ProvisioningError {
  code: string
  message: string
  details?: unknown
}
