import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { env } from "@/env";
import { authStorage } from "@/lib/auth/storage";
import type { AuthClient, FleetClient } from "./client-types";
import { AuthService } from "./gen/public/v1/auth_pb";
import { FleetService } from "./gen/public/v1/fleet_pb";

const transport = createConnectTransport({
  baseUrl: env.NEXT_PUBLIC_API_URL,
  interceptors: [
    (next) => async (req) => {
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
        if (env.NODE_ENV === "development") {
          console.error("API Error:", error);
        }
        throw error;
      }
    },
  ],
});

// Note: Type casting is required due to Connect RPC generated types not directly matching
// biome-ignore lint/suspicious/noExplicitAny: Required for Connect RPC type compatibility
export const fleetClient = createClient(FleetService as any, transport) as unknown as FleetClient;
// biome-ignore lint/suspicious/noExplicitAny: Required for Connect RPC type compatibility
export const authClient = createClient(AuthService as any, transport) as unknown as AuthClient;

export * from "./gen/public/v1/auth_pb";
// Export types for convenience
export * from "./gen/public/v1/fleet_pb";
