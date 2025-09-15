import type {
  CreateUpdateRequest,
  CreateUpdateResponse,
  DeleteDeviceRequest,
  DeployUpdateRequest,
  DeployUpdateResponse,
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
  StreamEventsRequest,
  UpdateDeviceRequest,
  UpdateDeviceResponse,
} from './gen/public/v1/fleet_pb'

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
} from './gen/public/v1/auth_pb'

import type {
  GetOrganizationRequest,
  GetOrganizationResponse,
  InviteMemberRequest,
  InviteMemberResponse,
  ListMembersRequest,
  ListMembersResponse,
  RemoveMemberRequest,
  UpdateOrganizationRequest,
  UpdateOrganizationResponse,
} from './gen/public/v1/organization_pb'

import type { Empty } from '@bufbuild/protobuf/wkt'

export interface FleetClient {
  listDevices(request?: Partial<ListDevicesRequest>): Promise<ListDevicesResponse>
  getDevice(request: Partial<GetDeviceRequest>): Promise<GetDeviceResponse>
  updateDevice(request: Partial<UpdateDeviceRequest>): Promise<UpdateDeviceResponse>
  deleteDevice(request: Partial<DeleteDeviceRequest>): Promise<Empty>
  getDeviceStats(request?: Partial<GetDeviceStatsRequest>): Promise<GetDeviceStatsResponse>
  discoverDevices(request?: Partial<DiscoverDevicesRequest>): Promise<DiscoverDevicesResponse>
  getTelemetry(request?: Partial<GetTelemetryRequest>): Promise<GetTelemetryResponse>
  streamEvents(request?: Partial<StreamEventsRequest>): AsyncIterable<Event>
  createUpdate(request: Partial<CreateUpdateRequest>): Promise<CreateUpdateResponse>
  deployUpdate(request: Partial<DeployUpdateRequest>): Promise<DeployUpdateResponse>
}

export interface AuthClient {
  login(request: Partial<LoginRequest>): Promise<LoginResponse>
  logout(request: Partial<LogoutRequest>): Promise<Empty>
  refreshToken(request: Partial<RefreshTokenRequest>): Promise<RefreshTokenResponse>
  getCurrentUser(): Promise<GetCurrentUserResponse>
  createAPIKey(request: Partial<CreateAPIKeyRequest>): Promise<CreateAPIKeyResponse>
  listAPIKeys(request?: Partial<ListAPIKeysRequest>): Promise<ListAPIKeysResponse>
}

export interface OrganizationClient {
  getOrganization(request: Partial<GetOrganizationRequest>): Promise<GetOrganizationResponse>
  updateOrganization(
    request: Partial<UpdateOrganizationRequest>,
  ): Promise<UpdateOrganizationResponse>
  listMembers(request: Partial<ListMembersRequest>): Promise<ListMembersResponse>
  inviteMember(request: Partial<InviteMemberRequest>): Promise<InviteMemberResponse>
  removeMember(request: Partial<RemoveMemberRequest>): Promise<Empty>
}
