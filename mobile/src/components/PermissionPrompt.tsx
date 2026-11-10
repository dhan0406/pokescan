import { Pressable, StyleSheet, Text, View } from 'react-native';

import { colors, spacing, type } from '@/theme/tokens';

/**
 * Shown when the camera is not allowed. If the system will still show its
 * permission dialog the button asks again; otherwise it opens Settings.
 */
export function PermissionPrompt({ canAskAgain, onPress }: { canAskAgain: boolean; onPress: () => void }) {
  return (
    <View style={styles.container}>
      <Text style={styles.heading}>Camera access is off</Text>
      <Text style={styles.body}>Pokescan needs your camera to read cards. Photos are only used to identify the card.</Text>
      <Pressable accessibilityRole="button" onPress={onPress} style={styles.button}>
        <Text style={styles.buttonText}>{canAskAgain ? 'Allow camera' : 'Open Settings'}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, padding: spacing.lg, justifyContent: 'center', gap: spacing.md },
  heading: { ...type.heading, color: colors.text },
  body: { ...type.body, color: colors.textMuted },
  button: { backgroundColor: colors.accent, borderRadius: 10, paddingVertical: spacing.md, alignItems: 'center' },
  buttonText: { ...type.body, color: colors.background, fontWeight: '700' },
});
