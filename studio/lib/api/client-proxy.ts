import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { authStorage } from "@/lib/auth/storage";
import type { AuthClient, FleetClient } from "./client-types";
import { AuthService } from "./gen/public/v1/auth_pb";
import { FleetService } from "./gen/public/v1/fleet_pb";

/**
 * Create API transport with proper URL based on environment
 * - Server-side: Direct Docker DNS URLs
 * - Client-side: Proxy through Next.js API routes
 */
function getApiUrl() {
  if (typeof window === "undefined") {
    // Server-side: Use Docker DNS
    return process.env.PLATFORM_API_URL || "http://platform-api:8080";
  } else {
    // Client-side: Use Next.js proxy
    return "/api/proxy";
  }
}

// Create transport with interceptors for auth and error handling
const transport = createConnectTransport({
  baseUrl: getApiUrl(),
  interceptors: [
    // Auth interceptor
    (next) => async (req) => {
      // Add auth headers
      const headers = authStorage.getAuthHeaders();
      for (const [key, value] of Object.entries(headers)) {
        req.header.set(key, value);
      }

      return next(req);
    },
    // Error interceptor
    (next) => async (req) => {
      try {
        const res = await next(req);
        return res;
      } catch (error) {
        // Log errors in development
        if (process.env.NODE_ENV === "development") {
          console.error("[API Error]", error);
        }
        // Re-throw for handling in the application
        throw error;
      }
    },
  ],
});

// Create service clients with proper typing
// biome-ignore lint/suspicious/noExplicitAny: Required for Connect RPC type compatibility
export const fleetClient = createClient(FleetService as any, transport) as unknown as FleetClient;
// biome-ignore lint/suspicious/noExplicitAny: Required for Connect RPC type compatibility
export const authClient = createClient(AuthService as any, transport) as unknown as AuthClient;

// Re-export types
export * from "./gen/public/v1/auth_pb";
export * from "./gen/public/v1/fleet_pb";