import { CameraView, useCameraPermissions } from 'expo-camera';
import { useIsFocused } from 'expo-router';
import { useRef, useState } from 'react';
import { ActivityIndicator, Image, Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import type { ApiClient } from '@/api/client';
import { frameRect, type Size } from '@/capture/crop';
import { useScan } from '@/scan/useScan';
import { colors, spacing, type } from '@/theme/tokens';

import { CardFrame } from './CardFrame';
import { PermissionPrompt } from './PermissionPrompt';

/** Camera with a card frame and shutter, and the result of the last scan. */
export function ScanCamera({ api }: { api: ApiClient }) {
  const [permission, requestPermission] = useCameraPermissions();
  const focused = useIsFocused();
  const cameraRef = useRef<CameraView>(null);
  const [view, setView] = useState<Size | null>(null);
  const [ready, setReady] = useState(false);
  const { state, capture, reset } = useScan(api);

  if (!permission) return <View style={styles.fill} />; // still loading

  if (!permission.granted) {
    return (
      <PermissionPrompt
        canAskAgain={permission.canAskAgain}
        onPress={() => (permission.canAskAgain ? requestPermission() : Linking.openSettings())}
      />
    );
  }

  if (state.phase === 'done') {
    return (
      <ScrollView style={styles.fill} contentContainerStyle={styles.result}>
        <Image source={{ uri: state.thumbnailUri }} style={styles.thumbnail} resizeMode="contain" />
        <Text style={styles.mono}>{JSON.stringify(state.response, null, 2)}</Text>
        <Pressable accessibilityRole="button" onPress={reset} style={styles.button}>
          <Text style={styles.buttonText}>Scan another card</Text>
        </Pressable>
      </ScrollView>
    );
  }

  if (state.phase === 'failed') {
    return (
      <View style={[styles.fill, styles.result]}>
        <Text style={styles.heading}>That didn&apos;t work</Text>
        <Text style={styles.error}>{state.error.message}</Text>
        <Pressable accessibilityRole="button" onPress={reset} style={styles.button}>
          <Text style={styles.buttonText}>Try again</Text>
        </Pressable>
      </View>
    );
  }

  const working = state.phase === 'working';
  const canShoot = ready && view !== null && !working;

  return (
    <View
      style={styles.fill}
      onLayout={(e) => setView({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
      {/* Only one camera preview may be active, so unmount it when the tab is not focused. */}
      {focused && (
        <CameraView ref={cameraRef} style={StyleSheet.absoluteFill} facing="back" onCameraReady={() => setReady(true)} />
      )}
      {view && <CardFrame frame={frameRect(view)} />}

      {working && (
        <View style={styles.reading}>
          <ActivityIndicator color={colors.accent} />
          <Text style={styles.readingText}>Reading card…</Text>
        </View>
      )}

      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Take photo"
        disabled={!canShoot}
        onPress={() => cameraRef.current && view && capture(cameraRef.current, view)}
        style={[styles.shutter, !canShoot && styles.shutterDisabled]}>
        <View style={styles.shutterInner} />
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: colors.background },
  result: { padding: spacing.lg, gap: spacing.md, justifyContent: 'center' },
  thumbnail: { width: '100%', height: 280 },
  mono: { ...type.mono, color: colors.text },
  heading: { ...type.heading, color: colors.text },
  error: { ...type.body, color: colors.danger },
  button: { backgroundColor: colors.accent, borderRadius: 10, paddingVertical: spacing.md, alignItems: 'center' },
  buttonText: { ...type.body, color: colors.background, fontWeight: '700' },
  reading: {
    position: 'absolute',
    top: spacing.lg,
    alignSelf: 'center',
    flexDirection: 'row',
    gap: spacing.sm,
    alignItems: 'center',
    backgroundColor: 'rgba(0,0,0,0.7)',
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.md,
    borderRadius: 20,
  },
  readingText: { ...type.body, color: colors.text },
  shutter: {
    position: 'absolute',
    bottom: spacing.lg,
    alignSelf: 'center',
    width: 72,
    height: 72,
    borderRadius: 36,
    borderWidth: 4,
    borderColor: colors.text,
    alignItems: 'center',
    justifyContent: 'center',
  },
  shutterDisabled: { opacity: 0.4 },
  shutterInner: { width: 52, height: 52, borderRadius: 26, backgroundColor: colors.text },
});
