import { createApiClient, type ApiClient } from './api/client';

const EXAMPLE = 'http://192.168.0.10:8080';

/**
 * Reads and validates the backend address. Throws an error that names the
 * variable and shows an example when it is missing or not an http(s) URL.
 * Trailing slashes are removed.
 */
export function readApiUrl(env: { EXPO_PUBLIC_API_URL?: string }): string {
  const value = env.EXPO_PUBLIC_API_URL?.trim() ?? '';
  if (!/^https?:\/\/\S+$/.test(value)) {
    throw new Error(
      `EXPO_PUBLIC_API_URL is not set to a server address. Copy mobile/.env.example to mobile/.env and set it, for example ${EXAMPLE}, then restart Expo.`,
    );
  }
  return value.replace(/\/+$/, '');
}

export type ApiSetup = { ok: true; api: ApiClient } | { ok: false; message: string };

/** Builds the API client, or returns the configuration problem so a screen can show it instead of crashing. */
export function setupApi(env: { EXPO_PUBLIC_API_URL?: string }): ApiSetup {
  try {
    return { ok: true, api: createApiClient({ baseUrl: readApiUrl(env) }) };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : String(e) };
  }
}
