import type { DiscoveredDevice } from "./api/gen/public/v1/fleet_pb";
import type { ConfigUpdate, Device, DeviceUpdate, TelemetryData, Update } from "./types";
import type {
  CreateProfileRequest,
  DeviceSetupResponse,
  ProvisioningProfile,
} from "./types/provisioning";

// For server-side rendering, use the internal Docker network URL
// For client-side, use the Next.js API route which proxies to the backend
const API_BASE =
  typeof window === "undefined" ? process.env.DEVICE_API_URL || "http://device-api:8080" : "";
const DEFAULT_TIMEOUT = 30000; // 30 seconds
const DEFAULT_RETRY_COUNT = 3;
const RETRY_DELAY_BASE = 1000; // 1 second

interface RetryOptions {
  maxRetries?: number;
  timeout?: number;
  retryableStatuses?: number[];
}

class ApiClient {
  private async sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  private async fetchWithTimeout(
    url: string,
    options: RequestInit,
    timeout: number,
  ): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeout);

    try {
      const response = await fetch(url, {
        ...options,
        signal: controller.signal,
      });
      clearTimeout(timeoutId);
      return response;
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(`Request timeout after ${timeout}ms`);
      }
      throw error;
    }
  }

  private async fetchWithRetry(
    url: string,
    options: RequestInit,
    retryOptions: RetryOptions = {},
  ): Promise<Response> {
    const {
      maxRetries = DEFAULT_RETRY_COUNT,
      timeout = DEFAULT_TIMEOUT,
      retryableStatuses = [408, 429, 500, 502, 503, 504],
    } = retryOptions;

    let lastError: Error | null = null;

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        const response = await this.fetchWithTimeout(url, options, timeout);

        // If response is ok or status is not retryable, return it
        if (response.ok || !retryableStatuses.includes(response.status)) {
          return response;
        }

        // If we have retries left and status is retryable, continue
        if (attempt < maxRetries) {
          const delay = RETRY_DELAY_BASE * 2 ** attempt; // Exponential backoff
          console.warn(`Request failed with status ${response.status}, retrying in ${delay}ms...`);
          await this.sleep(delay);
          continue;
        }

        // No retries left, return the failed response
        return response;
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));

        // Network errors are retryable
        if (attempt < maxRetries) {
          const delay = RETRY_DELAY_BASE * 2 ** attempt;
          console.warn(`Network error: ${lastError.message}, retrying in ${delay}ms...`);
          await this.sleep(delay);
        }
      }
    }

    throw lastError || new Error("Failed after all retry attempts");
  }

  private async fetchJson<T>(
    url: string,
    options?: RequestInit,
    retryOptions?: RetryOptions,
  ): Promise<T> {
    const response = await this.fetchWithRetry(
      `${API_BASE}${url}`,
      {
        ...options,
        headers: {
          "Content-Type": "application/json",
          ...options?.headers,
        },
      },
      retryOptions,
    );

    if (!response.ok) {
      throw new Error(`API error: ${response.status} ${response.statusText}`);
    }

    return response.json();
  }

  // Device endpoints
  async getDevices(): Promise<Device[]> {
    return this.fetchJson<Device[]>("/api/v1/devices");
  }

  async getDevice(id: string): Promise<Device> {
    return this.fetchJson<Device>(`/api/v1/devices/${id}`);
  }

  async updateDevice(id: string, device: Partial<Device>): Promise<Device> {
    return this.fetchJson<Device>(`/api/v1/devices/${id}`, {
      method: "PUT",
      body: JSON.stringify(device),
    });
  }

  async deleteDevice(id: string): Promise<void> {
    const response = await this.fetchWithRetry(`${API_BASE}/api/v1/devices/${id}`, {
      method: "DELETE",
    });
    if (!response.ok) {
      throw new Error(`Failed to delete device: ${response.status} ${response.statusText}`);
    }
  }

  async restartDevice(id: string): Promise<void> {
    const response = await this.fetchWithRetry(`${API_BASE}/api/v1/devices/${id}/restart`, {
      method: "POST",
    });
    if (!response.ok) {
      throw new Error(`Failed to restart device: ${response.status} ${response.statusText}`);
    }
  }

  // Telemetry endpoints
  async getTelemetry(deviceId?: string, limit = 100): Promise<TelemetryData[]> {
    const params = new URLSearchParams();
    if (deviceId) params.append("device_id", deviceId);
    params.append("limit", limit.toString());

    return this.fetchJson<TelemetryData[]>(`/api/v1/telemetry?${params}`);
  }

  async getMetrics(limit = 10): Promise<TelemetryData[]> {
    return this.fetchJson<TelemetryData[]>(`/api/v1/telemetry/metrics?limit=${limit}`);
  }

  // Configuration endpoints
  async getConfig(deviceId: string): Promise<ConfigUpdate> {
    return this.fetchJson<ConfigUpdate>(`/api/v1/config?device_id=${deviceId}`);
  }

  async updateConfig(deviceId: string, config: ConfigUpdate): Promise<void> {
    await this.fetchJson("/api/v1/config", {
      method: "POST",
      body: JSON.stringify({ device_id: deviceId, ...config }),
    });
  }

  // Discovery endpoint
  async discoverDevices(): Promise<DiscoveredDevice[]> {
    return this.fetchJson<DiscoveredDevice[]>("/api/v1/discover");
  }

  // Update endpoints
  async getUpdates(): Promise<Update[]> {
    return this.fetchJson<Update[]>("/api/v1/updates");
  }

  async createUpdate(update: Omit<Update, "id" | "created_at">): Promise<Update> {
    return this.fetchJson<Update>("/api/v1/updates", {
      method: "POST",
      body: JSON.stringify(update),
    });
  }

  async getDeviceUpdates(deviceId: string): Promise<DeviceUpdate[]> {
    return this.fetchJson<DeviceUpdate[]>(`/api/v1/devices/${deviceId}/updates`);
  }

  async applyUpdate(deviceId: string, updateId: number): Promise<void> {
    await this.fetchJson(`/api/v1/devices/${deviceId}/updates/${updateId}`, {
      method: "POST",
    });
  }

  // Auto-setup and provisioning endpoints
  async getDiscoveredDevices(): Promise<DiscoveredDevice[]> {
    return this.fetchJson<DiscoveredDevice[]>("/api/v1/discover/unregistered");
  }

  async getProvisioningProfiles(): Promise<ProvisioningProfile[]> {
    return this.fetchJson<ProvisioningProfile[]>("/api/v1/provisioning/profiles");
  }

  async createProvisioningProfile(profile: CreateProfileRequest): Promise<ProvisioningProfile> {
    return this.fetchJson<ProvisioningProfile>("/api/v1/provisioning/profiles", {
      method: "POST",
      body: JSON.stringify(profile),
    });
  }

  async setupDevices(deviceIds: string[], profileId: string): Promise<DeviceSetupResponse[]> {
    return this.fetchJson<DeviceSetupResponse[]>("/api/v1/provisioning/setup", {
      method: "POST",
      body: JSON.stringify({ device_ids: deviceIds, profile_id: profileId }),
    });
  }

  async deleteProvisioningProfile(profileId: string): Promise<void> {
    const response = await this.fetchWithRetry(
      `${API_BASE}/api/v1/provisioning/profiles/${profileId}`,
      { method: "DELETE" },
    );
    if (!response.ok) {
      throw new Error(`Failed to delete profile: ${response.status} ${response.statusText}`);
    }
  }
}

export const api = new ApiClient();
