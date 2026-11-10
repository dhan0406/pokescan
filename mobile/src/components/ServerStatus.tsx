import { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text } from 'react-native';

import type { ApiClient } from '@/api/client';
import { colors, spacing, type } from '@/theme/tokens';

type Status = 'checking' | 'connected' | 'unreachable';

const LABELS: Record<Status, string> = {
  checking: 'Server: checking…',
  connected: 'Server: connected',
  unreachable: "Server: can't connect. Check that the backend is running and the phone is on the same Wi-Fi. Tap to retry.",
};

/** A small banner showing whether the backend answers. Tap to check again. */
export function ServerStatus({ api }: { api: ApiClient }) {
  const [status, setStatus] = useState<Status>('checking');
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false; // ignore a late answer after unmount or a newer check
    api.ping().then(
      () => (cancelled ? undefined : setStatus('connected')),
      () => (cancelled ? undefined : setStatus('unreachable')),
    );
    return () => {
      cancelled = true;
    };
  }, [api, attempt]);

  const check = () => {
    setStatus('checking');
    setAttempt((n) => n + 1);
  };

  return (
    <Pressable onPress={check} accessibilityRole="button" style={styles.banner}>
      <Text style={[styles.text, status === 'unreachable' && styles.error, status === 'connected' && styles.ok]}>
        {LABELS[status]}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  banner: { paddingVertical: spacing.sm, paddingHorizontal: spacing.md, backgroundColor: colors.surface },
  text: { ...type.body, fontSize: 13, lineHeight: 18, color: colors.textMuted },
  ok: { color: colors.accent },
  error: { color: colors.danger },
});
