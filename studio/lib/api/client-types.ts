import type { Empty } from "@bufbuild/protobuf/wkt";

import type {
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  GetCurrentUserResponse,
  ListAPIKeysRequest,
  ListAPIKeysResponse,
  LoginRequest,
  LoginResponse,
  LogoutRequest,
  RefreshTokenRequest,
  RefreshTokenResponse,
} from "./gen/public/v1/auth_pb";
import type {
  CreateDeploymentRequest,
  CreateDeploymentResponse,
  DeleteDeviceRequest,
  DiscoverDevicesRequest,
  DiscoverDevicesResponse,
  Event,
  GetDeviceRequest,
  GetDeviceResponse,
  GetDeviceStatsRequest,
  GetDeviceStatsResponse,
  GetTelemetryRequest,
  GetTelemetryResponse,
  ListDevicesRequest,
  ListDevicesResponse,
  StartDeploymentRequest,
  StartDeploymentResponse,
  StreamEventsRequest,
  UpdateDeviceRequest,
  UpdateDeviceResponse,
} from "./gen/public/v1/fleet_pb";

export interface FleetClient {
  listDevices(request?: Partial<ListDevicesRequest>): Promise<ListDevicesResponse>;
  getDevice(request: Partial<GetDeviceRequest>): Promise<GetDeviceResponse>;
  updateDevice(request: Partial<UpdateDeviceRequest>): Promise<UpdateDeviceResponse>;
  deleteDevice(request: Partial<DeleteDeviceRequest>): Promise<Empty>;
  getDeviceStats(request?: Partial<GetDeviceStatsRequest>): Promise<GetDeviceStatsResponse>;
  discoverDevices(request?: Partial<DiscoverDevicesRequest>): Promise<DiscoverDevicesResponse>;
  getTelemetry(request?: Partial<GetTelemetryRequest>): Promise<GetTelemetryResponse>;
  streamEvents(request?: Partial<StreamEventsRequest>): AsyncIterable<Event>;
  createDeployment(request: Partial<CreateDeploymentRequest>): Promise<CreateDeploymentResponse>;
  startDeployment(request: Partial<StartDeploymentRequest>): Promise<StartDeploymentResponse>;
}

export interface AuthClient {
  login(request: Partial<LoginRequest>): Promise<LoginResponse>;
  logout(request: Partial<LogoutRequest>): Promise<Empty>;
  refreshToken(request: Partial<RefreshTokenRequest>): Promise<RefreshTokenResponse>;
  getCurrentUser(): Promise<GetCurrentUserResponse>;
  createAPIKey(request: Partial<CreateAPIKeyRequest>): Promise<CreateAPIKeyResponse>;
  listAPIKeys(request?: Partial<ListAPIKeysRequest>): Promise<ListAPIKeysResponse>;
}
