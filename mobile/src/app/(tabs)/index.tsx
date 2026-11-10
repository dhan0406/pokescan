import { StyleSheet, Text, View } from 'react-native';

import { colors, spacing, type } from '@/theme/tokens';

export default function ScanScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.heading}>Scan</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.background, padding: spacing.lg, justifyContent: 'center' },
  heading: { ...type.heading, color: colors.text },
});
