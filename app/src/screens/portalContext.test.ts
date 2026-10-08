import { resolvePortalContext } from "./portalContext";

describe("resolvePortalContext", () => {
  it("resolves a community membership to the community-scoped path parameter", () => {
    expect(
      resolvePortalContext({
        scope: "community",
        id: "community-1",
        name: "C/ Mayor 12, Irun",
        role: "owner",
      }),
    ).toEqual({
      scope: "community",
      role: "owner",
      path: "/v1/communities/{id}",
      params: { path: { id: "community-1" } },
    });
  });

  it("resolves an office membership to the caller-scoped office routes, which take no path parameter", () => {
    expect(
      resolvePortalContext({
        scope: "office",
        id: "office-1",
        name: "Fincas Bidasoa",
        role: "admin_staff",
      }),
    ).toEqual({
      scope: "office",
      role: "admin_staff",
      path: "/v1/offices/me",
      params: {},
    });
  });
});
