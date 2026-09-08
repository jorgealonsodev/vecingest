import { createApiClient } from "@vecingest/shared/client";

/**
 * The generated, typed API client (`packages/shared`), pointed at the
 * deployed API. `EXPO_PUBLIC_*` env vars are inlined at build time by Expo
 * (see PRD_go.md §7.7 / §9 CI notes on `EXPO_PUBLIC_API_URL`).
 */
export const apiClient = createApiClient({
  baseUrl: process.env.EXPO_PUBLIC_API_URL ?? "http://localhost:8080",
});
