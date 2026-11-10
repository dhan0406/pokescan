// Shared design tokens. The viewer gets the bold treatment later; the rest of
// the app stays quiet and uses these.
import { Platform } from 'react-native';

export const colors = {
  background: '#0f1115',
  surface: '#1a1d24',
  text: '#f2f4f8',
  textMuted: '#9aa3b2',
  accent: '#ffcb05',
  danger: '#ff6b6b',
} as const;

export const spacing = {
  xs: 4,
  sm: 8,
  md: 16,
  lg: 24,
  xl: 40,
} as const;

// Monospace is for headings and stat readouts only, per the design spec.
const mono = Platform.select({ ios: 'Menlo', default: 'monospace' });

export const type = {
  body: { fontSize: 16, lineHeight: 22 },
  heading: { fontSize: 22, lineHeight: 28, fontWeight: '700' as const },
  mono: { fontSize: 14, lineHeight: 20, fontFamily: mono },
} as const;
