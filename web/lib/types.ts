export interface Device {
  id: string
  name: string
  type: string
  version: string
  last_seen: string
  status: 'online' | 'offline'
  metadata?: string
  api_key?: string
  created_at?: string
  updated_at?: string
}

export interface TelemetryData {
  id?: number
  device_id: string
  timestamp: string
  metric_name: string
  value: number
  metadata?: string
}

export interface ConfigUpdate {
  server_url: string
  api_key?: string
  config: string
}

export interface Update {
  id: number
  version: string
  description?: string
  binary_url: string
  checksum: string
  created_at: string
}

export interface DeviceUpdate {
  device_id: string
  update_id: number
  status: 'pending' | 'downloading' | 'applying' | 'completed' | 'failed'
  applied_at?: string
}

// DiscoveredDevice type is now imported from protobuf-generated types
// See: lib/api/gen/public/v1/fleet_pb.ts
