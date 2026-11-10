import { Tabs } from 'expo-router';

import { colors } from '@/theme/tokens';

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarStyle: { backgroundColor: colors.surface, borderTopColor: colors.background },
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.textMuted,
      }}>
      <Tabs.Screen name="index" options={{ title: 'Scan' }} />
      <Tabs.Screen name="binder" options={{ title: 'Binder' }} />
    </Tabs>
  );
}
