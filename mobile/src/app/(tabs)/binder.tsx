import { StyleSheet, Text, View } from 'react-native';

import { colors, spacing, type } from '@/theme/tokens';

export default function BinderScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.body}>Your binder will appear here.</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.background, padding: spacing.lg, justifyContent: 'center' },
  body: { ...type.body, color: colors.textMuted },
});
