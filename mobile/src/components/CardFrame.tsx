import { StyleSheet, View } from 'react-native';

import type { Rect } from '@/capture/crop';
import { colors } from '@/theme/tokens';

/** Dims everything outside the card frame and outlines the frame. Ignores touches. */
export function CardFrame({ frame }: { frame: Rect }) {
  const right = frame.originX + frame.width;
  const bottom = frame.originY + frame.height;
  return (
    <View pointerEvents="none" style={StyleSheet.absoluteFill}>
      <View style={[styles.dim, { top: 0, left: 0, right: 0, height: frame.originY }]} />
      <View style={[styles.dim, { top: bottom, left: 0, right: 0, bottom: 0 }]} />
      <View style={[styles.dim, { top: frame.originY, left: 0, width: frame.originX, height: frame.height }]} />
      <View style={[styles.dim, { top: frame.originY, left: right, right: 0, height: frame.height }]} />
      <View
        style={[
          styles.outline,
          { top: frame.originY, left: frame.originX, width: frame.width, height: frame.height },
        ]}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  dim: { position: 'absolute', backgroundColor: 'rgba(0,0,0,0.55)' },
  outline: { position: 'absolute', borderWidth: 2, borderColor: colors.accent, borderRadius: 10 },
});
