"use client";

import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { useMemo } from "react";
import { DeviceService } from "./gen/fleetd/v1/device_pb";
import { SettingsService } from "./gen/fleetd/v1/settings_pb";
import { TelemetryService } from "./gen/fleetd/v1/telemetry_pb";

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

// Device client hook
export function useDeviceClient() {
  return useMemo(() => {
    const transport = createAuthTransport();
    return createClient(DeviceService, transport);
  }, []);
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
