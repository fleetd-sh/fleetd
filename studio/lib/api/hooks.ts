"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { authClient, fleetClient } from "./client";
import type { PasswordCredential } from "./gen/public/v1/auth_pb";
import type {
  DiscoverDevicesRequest,
  Event,
  GetDeviceStatsRequest,
  GetTelemetryRequest,
  ListDevicesRequest,
  StreamEventsRequest,
  UpdateDeviceRequest,
} from "./gen/public/v1/fleet_pb";

// Device hooks
export function useDevices(request?: Partial<ListDevicesRequest>) {
  return useQuery({
    queryKey: ["devices", request],
    queryFn: async () => {
      const response = await fleetClient.listDevices(request || {});
      return response;
    },
  });
}

export function useDevice(deviceId: string) {
  return useQuery({
    queryKey: ["device", deviceId],
    queryFn: async () => {
      const response = await fleetClient.getDevice({ deviceId });
      return response.device;
    },
    enabled: !!deviceId,
  });
}

export function useUpdateDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: UpdateDeviceRequest) => {
      const response = await fleetClient.updateDevice(request);
      return response.device;
    },
    onSuccess: (device) => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
      queryClient.invalidateQueries({ queryKey: ["device", device?.id] });
    },
  });
}

export function useDeleteDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (deviceId: string) => {
      await fleetClient.deleteDevice({ deviceId });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
    },
  });
}

// Device stats hook
export function useDeviceStats(request?: Partial<GetDeviceStatsRequest>) {
  return useQuery({
    queryKey: ["device-stats", request],
    queryFn: async () => {
      const response = await fleetClient.getDeviceStats(request || {});
      return response;
    },
  });
}

// Discovery hook
export function useDiscoverDevices() {
  return useMutation({
    mutationFn: async (request?: Partial<DiscoverDevicesRequest>) => {
      const response = await fleetClient.discoverDevices(request || {});
      return response.devices;
    },
  });
}

// Telemetry hook
export function useTelemetry(request?: Partial<GetTelemetryRequest>) {
  return useQuery({
    queryKey: ["telemetry", request],
    queryFn: async () => {
      const response = await fleetClient.getTelemetry(request || {});
      return response.points;
    },
    refetchInterval: 5000, // Refresh every 5 seconds
  });
}

// Event streaming hook
export function useEventStream(
  request?: Partial<StreamEventsRequest>,
  onEvent?: (event: Event) => void,
) {
  const abortControllerRef = useRef<AbortController | null>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    // Create new abort controller
    abortControllerRef.current = new AbortController();

    const streamEvents = async () => {
      try {
        const stream = fleetClient.streamEvents(request || {});

        for await (const event of stream) {
          // Call the event handler if provided
          onEvent?.(event);

          // Invalidate relevant queries based on event type
          switch (event.type) {
            case 1: // EVENT_TYPE_DEVICE_CONNECTED
            case 2: // EVENT_TYPE_DEVICE_DISCONNECTED
            case 3: // EVENT_TYPE_DEVICE_UPDATED
              queryClient.invalidateQueries({ queryKey: ["devices"] });
              if (event.deviceId) {
                queryClient.invalidateQueries({ queryKey: ["device", event.deviceId] });
              }
              break;
            case 4: // EVENT_TYPE_TELEMETRY
              queryClient.invalidateQueries({ queryKey: ["telemetry"] });
              break;
            case 8: // EVENT_TYPE_CONFIG_CHANGED
              queryClient.invalidateQueries({ queryKey: ["config"] });
              break;
          }
        }
      } catch (error) {
        if ((error as Error).name !== "AbortError") {
          console.error("Event stream error:", error);
        }
      }
    };

    streamEvents();

    return () => {
      abortControllerRef.current?.abort();
    };
  }, [request, onEvent, queryClient]);
}

// Auth hooks
export function useCurrentUser() {
  return useQuery({
    queryKey: ["current-user"],
    queryFn: async () => {
      const response = await authClient.getCurrentUser();
      return response.user;
    },
  });
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ email, password }: { email: string; password: string }) => {
      const response = await authClient.login({
        credential: {
          case: "password",
          value: { email, password } as PasswordCredential,
        },
      });
      return response;
    },
    onSuccess: (data) => {
      // Store tokens
      if (data.accessToken) {
        localStorage.setItem("auth_token", data.accessToken);
      }
      if (data.refreshToken) {
        localStorage.setItem("refresh_token", data.refreshToken);
      }
      // Invalidate user query to refetch
      queryClient.invalidateQueries({ queryKey: ["current-user"] });
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const refreshToken = localStorage.getItem("refresh_token");
      if (refreshToken) {
        await authClient.logout({ refreshToken });
      }
    },
    onSuccess: () => {
      // Clear tokens
      localStorage.removeItem("auth_token");
      localStorage.removeItem("refresh_token");
      localStorage.removeItem("org_id");
      // Clear all queries
      queryClient.clear();
    },
  });
}

// Organization hooks
// Organization hooks - commented out as organization service was removed
// export function useOrganization(organizationId: string) {
//   return useQuery({
//     queryKey: ["organization", organizationId],
//     queryFn: async () => {
//       const response = await orgClient.getOrganization({ organizationId });
//       return response.organization;
//     },
//     enabled: !!organizationId,
//   });
// }

// TODO: Implement when listTeams endpoint is available
// export function useTeams(organizationId: string) {
//   return useQuery({
//     queryKey: ['teams', organizationId],
//     queryFn: async () => {
//       const response = await orgClient.listTeams({ organizationId })
//       return response.teams
//     },
//     enabled: !!organizationId,
//   })
// }

// export function useMembers(organizationId: string, teamId?: string) {
//   return useQuery({
//     queryKey: ["members", organizationId, teamId],
//     queryFn: async () => {
//       const response = await orgClient.listMembers({ organizationId, teamId });
//       return response.members;
//     },
//     enabled: !!organizationId,
//   });
// }
