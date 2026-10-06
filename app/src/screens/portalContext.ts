import type { paths } from "@vecingest/shared/client";
import type { schemas } from "@vecingest/shared/schemas";
import type { z } from "zod";

/** One entry of `GET /v1/me`'s `memberships`, from the generated schema. */
export type Membership = z.infer<typeof schemas.MembershipEntry>;

/**
 * User-facing Spanish label for each generated `MembershipEntry.role`. A
 * `Record` over the generated union, so a role added to `openapi.yaml`
 * fails the typecheck here instead of rendering a raw enum value.
 */
export const ROLE_LABELS: Record<Membership["role"], string> = {
  owner: "Propietario",
  tenant: "Inquilino",
  admin: "Administrador",
  admin_staff: "Personal del despacho",
};

/**
 * The working context a picked membership resolves to, entirely client-side
 * (app-portal-memberships: Context Selector For Multiple Memberships — "no
 * new 'set active context' endpoint is required"). `path` and `params` are
 * shaped for the generated `apiClient`, so a later portal screen can call
 * `apiClient.GET(context.path, context.params)` with no extra lookup.
 *
 * - A community membership scopes by URL: `/v1/communities/{id}` and its
 *   sub-routes take the community id as the path parameter.
 * - An office membership takes NO path parameter: the office routes are
 *   caller-scoped (`/v1/offices/me`, `/v1/offices/me/members`) and the API
 *   resolves the office from the session, never from a client-supplied id.
 */
export type PortalContext =
  | {
      scope: "community";
      role: Membership["role"];
      path: "/v1/communities/{id}" & keyof paths;
      params: { path: { id: string } };
    }
  | {
      scope: "office";
      role: Membership["role"];
      path: "/v1/offices/me" & keyof paths;
      params: Record<string, never>;
    };

export function resolvePortalContext(membership: Membership): PortalContext {
  if (membership.scope === "office") {
    return {
      scope: "office",
      role: membership.role,
      path: "/v1/offices/me",
      params: {},
    };
  }
  return {
    scope: "community",
    role: membership.role,
    path: "/v1/communities/{id}",
    params: { path: { id: membership.id } },
  };
}
