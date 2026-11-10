import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { ActivityIndicator, Image, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { apiSetup, type Pokemon } from '@/api';
import { colors, spacing, type } from '@/theme/tokens';

type Evolution = Pokemon['evolution'];

// Faint tint for the "screen" area, from the Pokémon's first type.
const TYPE_TINT: Record<string, string> = {
  normal: '#a8a77a', fire: '#ee8130', water: '#6390f0', electric: '#f7d02c', grass: '#7ac74c',
  ice: '#96d9d6', fighting: '#c22e28', poison: '#a33ea1', ground: '#e2bf65', flying: '#a98ff3',
  psychic: '#f95587', bug: '#a6b91a', rock: '#b6a136', ghost: '#735797', dragon: '#6f35fc',
  dark: '#705746', steel: '#b7b7ce', fairy: '#d685ad',
};

const STAT_LABELS: [string, string][] = [
  ['hp', 'HP'], ['attack', 'ATK'], ['defense', 'DEF'],
  ['special-attack', 'SP.A'], ['special-defense', 'SP.D'], ['speed', 'SPD'],
];

// Official artwork from the PokéAPI sprites repository, by National Pokédex number.
const artworkUrl = (id: number) =>
  `https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/other/official-artwork/${id}.png`;

function flatten(e: Evolution): string[] {
  return [e.name, ...e.evolvesTo.flatMap(flatten)];
}

export default function PokemonScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [state, setState] = useState<{ pokemon?: Pokemon; error?: string }>({});

  useEffect(() => {
    if (!apiSetup.ok) return;
    let cancelled = false;
    apiSetup.api.getPokemon(id).then(
      (pokemon) => !cancelled && setState({ pokemon }),
      (e: Error) => !cancelled && setState({ error: e.message }),
    );
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (state.error) {
    return (
      <View style={[styles.screen, styles.center]}>
        <Text style={styles.error}>{state.error}</Text>
      </View>
    );
  }
  const p = state.pokemon;
  if (!p) {
    return (
      <View style={[styles.screen, styles.center]}>
        <ActivityIndicator color={colors.accent} />
      </View>
    );
  }

  const tint = TYPE_TINT[p.types[0]] ?? colors.surface;
  const chain = flatten(p.evolution);

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={[styles.lcd, { borderColor: tint }]}>
        <View style={[styles.tint, { backgroundColor: tint }]} />
        <Text style={styles.dex}>No. {String(p.id).padStart(4, '0')}</Text>
        <Text style={styles.name}>{p.name.toUpperCase()}</Text>
        <View style={styles.row}>
          {p.types.map((t) => (
            <Text key={t} style={[styles.chip, { backgroundColor: TYPE_TINT[t] ?? colors.surface }]}>
              {t.toUpperCase()}
            </Text>
          ))}
        </View>
      </View>

      <Image source={{ uri: artworkUrl(p.id) }} style={styles.artwork} resizeMode="contain" />

      {p.flavorText ? <Text style={styles.flavor}>{p.flavorText}</Text> : null}

      <Text style={styles.heading}>Base stats</Text>
      {STAT_LABELS.map(([key, label]) => {
        const value = p.stats[key] ?? 0;
        return (
          <View key={key} style={styles.statRow}>
            <Text style={styles.statLabel}>{label}</Text>
            <Text style={styles.statValue}>{value}</Text>
            <View style={styles.barTrack}>
              <View style={[styles.barFill, { width: `${Math.min(100, (value / 255) * 100)}%`, backgroundColor: tint }]} />
            </View>
          </View>
        );
      })}

      {chain.length > 1 && (
        <>
          <Text style={styles.heading}>Evolutions</Text>
          <View style={styles.row}>
            {chain.map((name) => (
              <Pressable
                key={name}
                accessibilityRole="button"
                disabled={name === p.name}
                onPress={() => router.push({ pathname: '/pokemon/[id]', params: { id: name } })}
                style={[styles.evo, name === p.name && styles.evoCurrent]}>
                <Text style={styles.evoText}>{name}</Text>
              </Pressable>
            ))}
          </View>
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.background },
  center: { alignItems: 'center', justifyContent: 'center', padding: spacing.lg },
  content: { padding: spacing.md, gap: spacing.md },
  error: { ...type.body, color: colors.danger, textAlign: 'center' },
  lcd: { borderWidth: 2, borderRadius: 12, padding: spacing.md, gap: spacing.sm, overflow: 'hidden', backgroundColor: colors.surface },
  tint: { ...StyleSheet.absoluteFill, opacity: 0.12 },
  dex: { ...type.mono, color: colors.textMuted },
  name: { ...type.mono, fontSize: 26, lineHeight: 32, fontWeight: '700', color: colors.text },
  row: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm },
  chip: { ...type.mono, color: '#111', paddingHorizontal: spacing.sm, paddingVertical: spacing.xs, borderRadius: 6, overflow: 'hidden' },
  artwork: { width: '100%', height: 260 },
  flavor: { ...type.body, color: colors.text },
  heading: { ...type.heading, color: colors.text, marginTop: spacing.sm },
  statRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  statLabel: { ...type.mono, width: 48, color: colors.textMuted },
  statValue: { ...type.mono, width: 36, color: colors.text, textAlign: 'right' },
  barTrack: { flex: 1, height: 8, borderRadius: 4, backgroundColor: colors.surface },
  barFill: { height: 8, borderRadius: 4 },
  evo: { backgroundColor: colors.surface, borderRadius: 8, paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  evoCurrent: { borderWidth: 1, borderColor: colors.accent },
  evoText: { ...type.body, color: colors.text, textTransform: 'capitalize' },
});
