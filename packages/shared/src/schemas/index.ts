// GENERATED FILE — DO NOT EDIT.
// Source: api/openapi/openapi.yaml
// Regenerate with `make gen` (api-contract-generation: Generation Chain).

import { z } from "zod";

const CreateOfficeRequest = z.object({
  $schema: z.string().url().optional(),
  address: z.string().max(255).optional(),
  admin_email: z.string().email(),
  admin_name: z.string().min(1).max(255),
  cif: z.string().min(1).max(32),
  collegiate_number: z.string().max(64).optional(),
  email: z.string().email().optional(),
  name: z.string().min(1).max(255),
  phone: z.string().max(32).optional(),
});
const OfficeResponse = z.object({
  $schema: z.string().url().optional(),
  address: z.string().optional(),
  cif: z.string(),
  collegiate_number: z.string().optional(),
  created_at: z.string().datetime({ offset: true }),
  email: z.string().optional(),
  id: z.string(),
  name: z.string(),
  phone: z.string().optional(),
});
const AcceptInvitationRequest = z.object({
  $schema: z.string().url().optional(),
  consent: z.boolean(),
  device_name: z.string().max(255).optional(),
  name: z.string().min(1).max(255),
  password: z.string().min(12),
  phone: z.string().max(32).optional(),
  platform: z.enum(["ios", "android", "web"]),
  short_code: z.string().optional(),
  token: z.string().optional(),
});
const LoginResponse = z.object({
  $schema: z.string().url().optional(),
  access_token: z.string(),
  csrf_token: z.string().optional(),
  expires_in: z.number().int(),
  refresh_token: z.string().optional(),
});
const ForgotPasswordRequest = z.object({
  $schema: z.string().url().optional(),
  email: z.string().email(),
});
const ForgotPasswordResponse = z.object({
  $schema: z.string().url().optional(),
  accepted: z.boolean(),
});
const LoginRequest = z.object({
  $schema: z.string().url().optional(),
  device_name: z.string().max(255).optional(),
  email: z.string().email(),
  password: z.string().min(12),
  platform: z.enum(["ios", "android", "web"]),
});
const RefreshRequest = z
  .object({ $schema: z.string().url(), refresh_token: z.string() })
  .partial();
const CSRFTokenResponse = z.object({
  $schema: z.string().url().optional(),
  csrf_token: z.string(),
});
const ResetPasswordRequest = z.object({
  $schema: z.string().url().optional(),
  new_password: z.string().min(12),
  token: z.string(),
});
const ResetPasswordResponse = z.object({
  $schema: z.string().url().optional(),
  accepted: z.boolean(),
});
const SuperadminLoginRequest = z.object({
  $schema: z.string().url().optional(),
  device_name: z.string().max(255).optional(),
  email: z.string().email(),
  password: z.string().min(12),
  platform: z.enum(["ios", "android", "web"]),
  totp_code: z.string().optional(),
});
const CommunityResponse = z.object({
  $schema: z.string().url().optional(),
  address: z.string().optional(),
  annual_budget: z.string().optional(),
  cif: z.string().optional(),
  city: z.string().optional(),
  created_at: z.string().datetime({ offset: true }),
  dpa_signed_at: z.string().datetime({ offset: true }).optional(),
  id: z.string(),
  last_ordinary_meeting_at: z.string().datetime({ offset: true }).optional(),
  name: z.string(),
  office_id: z.string(),
  parent_community_id: z.string().optional(),
  postal_code: z.string().optional(),
  province: z.string().optional(),
  reserve_fund: z.string().optional(),
  secretary_is_office: z.boolean(),
  updated_at: z.string().datetime({ offset: true }),
});
const ListCommunitiesResponse = z.object({
  $schema: z.string().url().optional(),
  communities: z.union([z.array(CommunityResponse), z.null()]),
});
const CreateCommunityRequest = z.object({
  $schema: z.string().url().optional(),
  address: z.string().max(255).optional(),
  annual_budget: z.string().optional(),
  cif: z.string().max(32).optional(),
  city: z.string().max(128).optional(),
  name: z.string().min(1).max(255),
  office_id: z.string(),
  parent_community_id: z.string().uuid().optional(),
  postal_code: z.string().max(16).optional(),
  province: z.string().max(128).optional(),
  reserve_fund: z.string().optional(),
  secretary_is_office: z.boolean().optional(),
});
const CommunityDetailResponse = z.object({
  $schema: z.string().url().optional(),
  address: z.string().optional(),
  annual_budget: z.string().optional(),
  cif: z.string().optional(),
  city: z.string().optional(),
  created_at: z.string().datetime({ offset: true }),
  dpa_signed_at: z.string().datetime({ offset: true }).optional(),
  id: z.string(),
  last_ordinary_meeting_at: z.string().datetime({ offset: true }).optional(),
  name: z.string(),
  office_id: z.string(),
  office_member_count: z.number().int(),
  parent_community_id: z.string().optional(),
  postal_code: z.string().optional(),
  province: z.string().optional(),
  reserve_fund: z.string().optional(),
  secretary_is_office: z.boolean(),
  unit_count: z.number().int(),
  updated_at: z.string().datetime({ offset: true }),
});
const UpdateCommunityRequest = z
  .object({
    $schema: z.string().url(),
    address: z.string().max(255),
    annual_budget: z.string(),
    cif: z.string().max(32),
    city: z.string().max(128),
    name: z.string().min(1).max(255),
    parent_community_id: z.string().uuid(),
    postal_code: z.string().max(16),
    province: z.string().max(128),
    reserve_fund: z.string(),
    secretary_is_office: z.boolean(),
  })
  .partial();
const InvitationResponse = z.object({
  accepted_at: z.string().datetime({ offset: true }).optional(),
  community_id: z.string(),
  created_at: z.string().datetime({ offset: true }),
  email: z.string().optional(),
  expires_at: z.string().datetime({ offset: true }),
  id: z.string(),
  role: z.string(),
  sent_count: z.number().int(),
  status: z.string(),
  unit_id: z.string().optional(),
  updated_at: z.string().datetime({ offset: true }),
});
const ListInvitationsResponse = z.object({
  $schema: z.string().url().optional(),
  invitations: z.union([z.array(InvitationResponse), z.null()]),
});
const CreateInvitationRequest = z.object({
  $schema: z.string().url().optional(),
  email: z.string().email(),
  role: z.enum(["owner", "tenant"]),
  unit_id: z.string().uuid(),
});
const CreateInvitationResponse = z.object({
  $schema: z.string().url().optional(),
  accepted_at: z.string().datetime({ offset: true }).optional(),
  community_id: z.string(),
  created_at: z.string().datetime({ offset: true }),
  email: z.string().optional(),
  expires_at: z.string().datetime({ offset: true }),
  id: z.string(),
  role: z.string(),
  sent_count: z.number().int(),
  short_code: z.string(),
  status: z.string(),
  token: z.string(),
  unit_id: z.string().optional(),
  updated_at: z.string().datetime({ offset: true }),
});
const UnitMemberCreateRequest = z.object({
  consent_text_version: z.string().optional(),
  electronic_notifications_consent: z.boolean().optional(),
  email: z.string().email(),
  notification_address: z.string().optional(),
  role: z.enum(["owner", "tenant"]),
  tenure: z.enum(["full_owner", "bare_owner", "usufructuary"]).optional(),
});
const CreateUnitRequest = z.object({
  $schema: z.string().url().optional(),
  block: z.string().max(64).optional(),
  cadastral_ref: z.string().max(64).optional(),
  door: z.string().max(32).optional(),
  floor: z.string().max(32).optional(),
  members: z.union([z.array(UnitMemberCreateRequest), z.null()]).optional(),
  participation_coefficient: z.string().optional(),
  type: z.enum(["flat", "premises", "garage", "storage"]),
});
const UnitMemberResponse = z.object({
  $schema: z.string().url().optional(),
  community_id: z.string(),
  consent_text_version: z.string().optional(),
  created_at: z.string().datetime({ offset: true }),
  electronic_notifications_consent_at: z
    .string()
    .datetime({ offset: true })
    .optional(),
  id: z.string(),
  notification_address: z.string().optional(),
  role: z.string(),
  tenure: z.string(),
  unit_id: z.string(),
  updated_at: z.string().datetime({ offset: true }),
  user_id: z.string(),
});
const UnitCreateResponse = z.object({
  $schema: z.string().url().optional(),
  block: z.string().optional(),
  cadastral_ref: z.string().optional(),
  community_id: z.string(),
  created_at: z.string().datetime({ offset: true }),
  door: z.string().optional(),
  floor: z.string().optional(),
  id: z.string(),
  members: z.union([z.array(UnitMemberResponse), z.null()]).optional(),
  participation_coefficient: z.string().optional(),
  type: z.string(),
  updated_at: z.string().datetime({ offset: true }),
  warnings: z.union([z.array(z.string()), z.null()]).optional(),
});
const UnitImportRowResult = z.object({
  block: z.string().optional(),
  door: z.string().optional(),
  errors: z.union([z.array(z.string()), z.null()]).optional(),
  floor: z.string().optional(),
  owner_dni_cif: z.string().optional(),
  owner_name: z.string().optional(),
  row: z.number().int(),
  type: z.string().optional(),
  unit_id: z.string().optional(),
});
const UnitImportResponse = z.object({
  $schema: z.string().url().optional(),
  dry_run: z.boolean(),
  imported: z.number().int(),
  rows: z.union([z.array(UnitImportRowResult), z.null()]),
  warnings: z.union([z.array(z.string()), z.null()]).optional(),
});
const LiveResponse = z.object({
  $schema: z.string().url().optional(),
  ok: z.boolean(),
});
const ReadyResponse = z.object({
  $schema: z.string().url().optional(),
  ok: z.boolean(),
});
const PreviewInvitationRequest = z
  .object({
    $schema: z.string().url(),
    short_code: z.string(),
    token: z.string(),
  })
  .partial();
const PreviewInvitationResponse = z.object({
  $schema: z.string().url().optional(),
  community_id: z.string(),
  community_name: z.string(),
  expires_at: z.string().datetime({ offset: true }),
  role: z.string(),
  unit_id: z.string().optional(),
});
const RevokeInvitationResponse = z.object({
  $schema: z.string().url().optional(),
  status: z.string(),
});
const ResendInvitationResponse = z.object({
  $schema: z.string().url().optional(),
  sent_count: z.number().int(),
});
const MeResponse = z.object({
  $schema: z.string().url().optional(),
  email: z.string(),
  id: z.string(),
  is_superadmin: z.boolean(),
});
const SessionSummary = z.object({
  created_at: z.string().datetime({ offset: true }),
  device_name: z.string().optional(),
  family_id: z.string(),
  last_used_at: z.string().datetime({ offset: true }).optional(),
  platform: z.string(),
});
const ListSessionsResponse = z.object({
  $schema: z.string().url().optional(),
  sessions: z.union([z.array(SessionSummary), z.null()]),
});
const ListOfficesResponse = z.object({
  $schema: z.string().url().optional(),
  offices: z.union([z.array(OfficeResponse), z.null()]),
});
const OfficeMemberResponse = z.object({
  $schema: z.string().url().optional(),
  created_at: z.string().datetime({ offset: true }),
  id: z.string(),
  office_id: z.string(),
  role: z.string(),
  user_id: z.string(),
});
const ListOfficeMembersResponse = z.object({
  $schema: z.string().url().optional(),
  members: z.union([z.array(OfficeMemberResponse), z.null()]),
});
const AddOfficeMemberRequest = z.object({
  $schema: z.string().url().optional(),
  email: z.string().email(),
});
const ListUnitMembersResponse = z.object({
  $schema: z.string().url().optional(),
  members: z.union([z.array(UnitMemberResponse), z.null()]),
});
const UpdateUnitMemberRequest = z
  .object({
    $schema: z.string().url(),
    consent_text_version: z.string(),
    electronic_notifications_consent: z.boolean(),
    notification_address: z.string(),
    role: z.enum(["owner", "tenant"]),
    tenure: z.enum(["full_owner", "bare_owner", "usufructuary"]),
  })
  .partial();
const Cookie = z.object({
  Domain: z.string(),
  Expires: z.string().datetime({ offset: true }),
  HttpOnly: z.boolean(),
  MaxAge: z.number().int(),
  Name: z.string(),
  Partitioned: z.boolean(),
  Path: z.string(),
  Quoted: z.boolean(),
  Raw: z.string(),
  RawExpires: z.string(),
  SameSite: z.number().int(),
  Secure: z.boolean(),
  Unparsed: z.union([z.array(z.string()), z.null()]),
  Value: z.string(),
});
const ErrorDetail = z
  .object({ location: z.string(), message: z.string(), value: z.unknown() })
  .partial();
const ErrorModel = z
  .object({
    $schema: z.string().url(),
    detail: z.string(),
    errors: z.union([z.array(ErrorDetail), z.null()]),
    instance: z.string().url(),
    status: z.number().int(),
    title: z.string(),
    type: z.string().url().default("about:blank"),
  })
  .partial();

export const schemas = {
  CreateOfficeRequest,
  OfficeResponse,
  AcceptInvitationRequest,
  LoginResponse,
  ForgotPasswordRequest,
  ForgotPasswordResponse,
  LoginRequest,
  RefreshRequest,
  CSRFTokenResponse,
  ResetPasswordRequest,
  ResetPasswordResponse,
  SuperadminLoginRequest,
  CommunityResponse,
  ListCommunitiesResponse,
  CreateCommunityRequest,
  CommunityDetailResponse,
  UpdateCommunityRequest,
  InvitationResponse,
  ListInvitationsResponse,
  CreateInvitationRequest,
  CreateInvitationResponse,
  UnitMemberCreateRequest,
  CreateUnitRequest,
  UnitMemberResponse,
  UnitCreateResponse,
  UnitImportRowResult,
  UnitImportResponse,
  LiveResponse,
  ReadyResponse,
  PreviewInvitationRequest,
  PreviewInvitationResponse,
  RevokeInvitationResponse,
  ResendInvitationResponse,
  MeResponse,
  SessionSummary,
  ListSessionsResponse,
  ListOfficesResponse,
  OfficeMemberResponse,
  ListOfficeMembersResponse,
  AddOfficeMemberRequest,
  ListUnitMembersResponse,
  UpdateUnitMemberRequest,
  Cookie,
  ErrorDetail,
  ErrorModel,
};
