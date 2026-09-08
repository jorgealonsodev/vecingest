import * as SecureStore from "expo-secure-store";

/**
 * Persistence for the refresh token, the one token that must survive an app
 * restart. Always goes through `expo-secure-store` — never `AsyncStorage`
 * and never plain component/module state (app-login-ui: Secure Token
 * Storage).
 */

const REFRESH_TOKEN_KEY = "vecingest.refresh_token";

export async function persistRefreshToken(refreshToken: string): Promise<void> {
  await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, refreshToken);
}

export async function readRefreshToken(): Promise<string | null> {
  return SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
}

export async function clearRefreshToken(): Promise<void> {
  await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
}
