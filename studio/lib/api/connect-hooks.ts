"use client";

import { useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { TelemetryService } from "./gen/fleetd/v1/telemetry_connect";
import { SettingsService } from "./gen/fleetd/v1/settings_connect";

// Get the API URL from environment or use default
const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8090";

// Create transport with authentication
function createAuthTransport() {
  return createConnectTransport({
    baseUrl: API_URL,
    interceptors: [
      (next) => async (req) => {
        const token = localStorage.getItem("auth_token");
        if (token) {
          req.header.set("authorization", `Bearer ${token}`);
        }
        return await next(req);
      },
    ],
  });
}

// Telemetry client hook
export function useTelemetryClient() {
  return useMemo(() => {
    const transport = createAuthTransport();
    return createClient(TelemetryService, transport);
  }, []);
}

// Settings client hook
export function useSettingsClient() {
  return useMemo(() => {
    const transport = createAuthTransport();
    return createClient(SettingsService, transport);
  }, []);
}
