import { useCameraPermissions } from 'expo-camera';
import { router, useIsFocused } from 'expo-router';
import { useEffect, useState } from 'react';
import { Linking, Pressable, StyleSheet, Text, View } from 'react-native';

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

  // A matched Pokémon card opens the viewer; the scan screen is ready again when the user comes back.
  useEffect(() => {
    if (state.phase === 'done' && state.response.status === 'match' && state.response.pokemon) {
      router.push({
        pathname: '/pokemon/[id]',
        params: { id: String(state.response.pokemon.id) },
      });
      reset();
    }
  }, [state, reset]);

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
    // A match opens the viewer (effect above); this is a retake.
    const tip =
      state.response.reason === 'not_found'
        ? "We read the name but it isn't a Pokémon we know. Try again with the whole top of the card in view."
        : "Couldn't read the Pokémon's name. Hold the phone steady, avoid glare, and keep the name at the top of the card in the frame.";
    return (
      <View style={[styles.fill, styles.result]}>
        <Text style={styles.heading}>Let&apos;s try again</Text>
        <Text style={styles.body}>{tip}</Text>
        <Pressable accessibilityRole="button" onPress={reset} style={styles.button}>
          <Text style={styles.buttonText}>Scan again</Text>
        </Pressable>
      </View>
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
  body: { ...type.body, color: colors.textMuted },
  heading: { ...type.heading, color: colors.text },
  error: { ...type.body, color: colors.danger },
  button: { backgroundColor: colors.accent, borderRadius: 10, paddingVertical: spacing.md, alignItems: 'center' },
  buttonText: { ...type.body, color: colors.background, fontWeight: '700' },
});
