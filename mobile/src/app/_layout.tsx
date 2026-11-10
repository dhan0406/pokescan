import { Stack } from 'expo-router';

import { colors } from '@/theme/tokens';

// Scan is the main screen; the Pokémon viewer is pushed over it.
export default function RootLayout() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen
        name="pokemon/[id]"
        options={{
          headerShown: true,
          title: '',
          headerStyle: { backgroundColor: colors.background },
          headerTintColor: colors.text,
          headerShadowVisible: false,
        }}
      />
    </Stack>
  );
}
