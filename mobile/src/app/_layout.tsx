import { Stack } from 'expo-router';

// Tabs live in (tabs). The card viewer will be pushed over them as a stack
// screen, so the root is a Stack.
export default function RootLayout() {
  return <Stack screenOptions={{ headerShown: false }} />;
}
