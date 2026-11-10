import { CameraView } from 'expo-camera';
import { useRef, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { frameRect, type Size } from '@/capture/crop';
import { colors, spacing, type } from '@/theme/tokens';

import { CardFrame } from './CardFrame';

// Zoom is a 0-1 fraction of the device's maximum zoom (expo-camera). A card is
// only 6 cm wide, so filling the frame without zoom means holding the phone
// closer than most lenses can focus. Start zoomed in so it can be held back.
export const DEFAULT_ZOOM = 0.2;
export const MIN_ZOOM = 0;
export const MAX_ZOOM = 0.6;
const ZOOM_STEP = 0.05;

/** Moves zoom one step up (+1) or down (-1), staying within the limits. */
export function stepZoom(zoom: number, direction: 1 | -1): number {
  const next = Math.round((zoom + direction * ZOOM_STEP) * 100) / 100;
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, next));
}

/**
 * The live camera with its card frame and shutter. Everything that describes
 * this particular camera (is it ready, how big is it) lives here, so it starts
 * fresh whenever the camera is mounted again and a shutter tap can never reach
 * a camera that has not started yet.
 */
export function LiveCamera({
  working,
  zoom,
  onZoomChange,
  onShutter,
}: {
  working: boolean;
  zoom: number;
  onZoomChange: (zoom: number) => void;
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
      <CameraView
        ref={cameraRef}
        style={StyleSheet.absoluteFill}
        facing="back"
        zoom={zoom}
        onCameraReady={() => setReady(true)}
      />
      {view && <CardFrame frame={frameRect(view)} />}

      {working && (
        <View style={styles.reading}>
          <ActivityIndicator color={colors.accent} />
          <Text style={styles.readingText}>Reading card…</Text>
        </View>
      )}

      <View style={styles.zoomControls}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Zoom out"
          disabled={zoom <= MIN_ZOOM}
          onPress={() => onZoomChange(stepZoom(zoom, -1))}
          style={[styles.zoomButton, zoom <= MIN_ZOOM && styles.shutterDisabled]}>
          <Text style={styles.zoomText}>−</Text>
        </Pressable>
        <Text style={styles.zoomLabel}>Zoom</Text>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Zoom in"
          disabled={zoom >= MAX_ZOOM}
          onPress={() => onZoomChange(stepZoom(zoom, 1))}
          style={[styles.zoomButton, zoom >= MAX_ZOOM && styles.shutterDisabled]}>
          <Text style={styles.zoomText}>+</Text>
        </Pressable>
      </View>

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
  zoomControls: {
    position: 'absolute',
    right: spacing.md,
    bottom: spacing.lg + 12,
    flexDirection: 'column-reverse',
    alignItems: 'center',
    gap: spacing.sm,
    backgroundColor: 'rgba(0,0,0,0.55)',
    borderRadius: 24,
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.xs,
  },
  zoomButton: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(255,255,255,0.15)' },
  zoomText: { ...type.heading, color: colors.text, lineHeight: 28 },
  zoomLabel: { ...type.body, fontSize: 11, lineHeight: 14, color: colors.textMuted },
  shutterInner: { width: 52, height: 52, borderRadius: 26, backgroundColor: colors.text },
});
