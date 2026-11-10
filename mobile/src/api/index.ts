import { setupApi } from '../config';

// Expo only replaces process.env.EXPO_PUBLIC_* when it is written out literally,
// so the variable is referenced by name here rather than passing process.env.
export const apiSetup = setupApi({ EXPO_PUBLIC_API_URL: process.env.EXPO_PUBLIC_API_URL });

export * from './client';
