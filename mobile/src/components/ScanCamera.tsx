import { useCameraPermissions } from 'expo-camera';
import { useIsFocused } from 'expo-router';
import { useState } from 'react';
import { Image, Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import type { ApiClient } from '@/api/client';
import { useScan } from '@/scan/useScan';
import { colors, spacing, type } from '@/theme/tokens';

import { DEFAULT_ZOOM, LiveCamera } from './LiveCamera';
import { PermissionPrompt } from './PermissionPrompt';

/** Camera with a card frame and shutter, and the result of the last scan. */
export function ScanCamera({ api }: { api: ApiClient }) {
  const [permission, requestPermission] = useCameraPermissions();
  const focused = useIsFocused();
  const { state, capture, reset } = useScan(api);
  const [zoom, setZoom] = useState(DEFAULT_ZOOM); // kept here so it survives the camera remounting between scans

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

  // Only one camera preview may be active at a time, so the camera is not
  // mounted while the tab is out of focus.
  if (!focused) return <View style={styles.fill} />;

  return <LiveCamera working={state.phase === 'working'} zoom={zoom} onZoomChange={setZoom} onShutter={capture} />;
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
});
