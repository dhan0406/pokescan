import { useCallback, useEffect, useRef, useState } from 'react';
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
  const mounted = useRef(true);

  const check = useCallback(async () => {
    setStatus('checking');
    let next: Status = 'connected';
    try {
      await api.ping();
    } catch {
      next = 'unreachable';
    }
    if (mounted.current) setStatus(next);
  }, [api]);

  useEffect(() => {
    mounted.current = true;
    check();
    return () => {
      mounted.current = false;
    };
  }, [check]);

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
