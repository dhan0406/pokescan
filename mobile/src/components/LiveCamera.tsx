import { CameraView } from 'expo-camera';
import { useRef, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { frameRect, type Size } from '@/capture/crop';
import { colors, spacing, type } from '@/theme/tokens';

import { CardFrame } from './CardFrame';

/**
 * The live camera with its card frame and shutter. Everything that describes
 * this particular camera (is it ready, how big is it) lives here, so it starts
 * fresh whenever the camera is mounted again and a shutter tap can never reach
 * a camera that has not started yet.
 */
export function LiveCamera({
  working,
  onShutter,
}: {
  working: boolean;
  onShutter: (camera: CameraView, view: Size) => void;
}) {
  const cameraRef = useRef<CameraView>(null);
  const [view, setView] = useState<Size | null>(null);
  const [ready, setReady] = useState(false);

  const canShoot = ready && view !== null && !working;

  return (
    <View
      testID="scan-camera"
      style={styles.fill}
      onLayout={(e) => setView({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
      <CameraView ref={cameraRef} style={StyleSheet.absoluteFill} facing="back" onCameraReady={() => setReady(true)} />
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
        onPress={() => cameraRef.current && view && onShutter(cameraRef.current, view)}
        style={[styles.shutter, !canShoot && styles.shutterDisabled]}>
        <View style={styles.shutterInner} />
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: colors.background },
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
