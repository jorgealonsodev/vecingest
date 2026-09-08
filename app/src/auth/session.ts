/**
 * In-memory-only session state for the short-lived access and CSRF tokens.
 *
 * The access token (15 min) and CSRF token are held in memory only, never
 * written to `expo-secure-store`, `AsyncStorage`, or any other on-disk
 * storage. Only the refresh token is persisted (see `secureTokens.ts`),
 * because it is the one token that must survive an app restart.
 */

interface Session {
  accessToken: string | null;
  csrfToken: string | null;
}

let session: Session = { accessToken: null, csrfToken: null };

export function setSession(next: Partial<Session>): void {
  session = { ...session, ...next };
}

export function getSession(): Session {
  return session;
}

export function clearSession(): void {
  session = { accessToken: null, csrfToken: null };
}
