// GENERATED FILE — DO NOT EDIT.
// Source: api/openapi/openapi.yaml
// Regenerate with `make gen` (api-contract-generation: Generation Chain).

import { z } from "zod";

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
const LoginResponse = z.object({
  $schema: z.string().url().optional(),
  access_token: z.string(),
  csrf_token: z.string().optional(),
  expires_in: z.number().int(),
  refresh_token: z.string().optional(),
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
const LiveResponse = z.object({
  $schema: z.string().url().optional(),
  ok: z.boolean(),
});
const ReadyResponse = z.object({
  $schema: z.string().url().optional(),
  ok: z.boolean(),
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
  ForgotPasswordRequest,
  ForgotPasswordResponse,
  LoginRequest,
  LoginResponse,
  RefreshRequest,
  CSRFTokenResponse,
  ResetPasswordRequest,
  ResetPasswordResponse,
  SuperadminLoginRequest,
  LiveResponse,
  ReadyResponse,
  MeResponse,
  SessionSummary,
  ListSessionsResponse,
  Cookie,
  ErrorDetail,
  ErrorModel,
};
