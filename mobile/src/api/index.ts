import { fetch as expoFetch } from 'expo/fetch';

import { setupApi } from '../config';

// Expo only replaces process.env.EXPO_PUBLIC_* when it is written out literally,
// so the variable is referenced by name here rather than passing process.env.
//
// expo/fetch is used explicitly (it is also the global fetch on SDK 58 unless
// EXPO_PUBLIC_USE_RN_FETCH is set) because file uploads depend on it.
export const apiSetup = setupApi(
  { EXPO_PUBLIC_API_URL: process.env.EXPO_PUBLIC_API_URL },
  {
    fetch: expoFetch as unknown as typeof fetch,
    // Shows the real cause in the Expo terminal; the app only shows a plain message.
    onError: (path, error) => console.warn(`[api] ${path} failed:`, error),
  },
);

export * from './client';
