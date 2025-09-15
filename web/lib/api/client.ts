import { env } from '@/env'
import { authStorage } from '@/lib/auth/storage'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import type { AuthClient, FleetClient, OrganizationClient } from './client-types'
import { AuthService } from './gen/public/v1/auth_connect'
import { FleetService } from './gen/public/v1/fleet_connect'
import { OrganizationService } from './gen/public/v1/organization_connect'

// Create transport with interceptors for auth and error handling
const transport = createConnectTransport({
  baseUrl: env.NEXT_PUBLIC_API_URL,
  interceptors: [
    // Auth interceptor
    (next) => async (req) => {
      // Add auth headers
      const headers = authStorage.getAuthHeaders()
      for (const [key, value] of Object.entries(headers)) {
        req.header.set(key, value)
      }

      return next(req)
    },
    // Error interceptor
    (next) => async (req) => {
      try {
        const res = await next(req)
        return res
      } catch (error) {
        // Log errors in development
        if (env.NODE_ENV === 'development') {
          console.error('API Error:', error)
        }
        throw error
      }
    },
  ],
})

// Create service clients
// Note: Type casting is required due to Connect RPC generated types not directly matching
export const fleetClient = createClient(FleetService as any, transport) as unknown as FleetClient
export const authClient = createClient(AuthService as any, transport) as unknown as AuthClient
export const orgClient = createClient(
  OrganizationService as any,
  transport,
) as unknown as OrganizationClient

// Export types for convenience
export * from './gen/public/v1/fleet_pb'
export * from './gen/public/v1/auth_pb'
export * from './gen/public/v1/organization_pb'
