import { SafeAreaView } from 'react-native-safe-area-context';
import { StyleSheet, Text, View } from 'react-native';

import { apiSetup } from '@/api';
import { ScanCamera } from '@/components/ScanCamera';
import { ServerStatus } from '@/components/ServerStatus';
import { colors, spacing, type } from '@/theme/tokens';

export default function ScanScreen() {
  if (!apiSetup.ok) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.body}>
          <Text style={styles.heading}>Can&apos;t find the server address</Text>
          <Text style={styles.error}>{apiSetup.message}</Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <ServerStatus api={apiSetup.api} />
      <ScanCamera api={apiSetup.api} />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.background },
  body: { flex: 1, padding: spacing.lg, justifyContent: 'center', gap: spacing.md },
  heading: { ...type.heading, color: colors.text },
  error: { ...type.body, color: colors.danger },
});
