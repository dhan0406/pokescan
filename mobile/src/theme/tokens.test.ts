import { colors, spacing, type } from './tokens';

describe('design tokens', () => {
  it('defines the colors the app uses', () => {
    for (const key of ['background', 'surface', 'text', 'textMuted', 'accent', 'danger'] as const) {
      expect(typeof colors[key]).toBe('string');
      expect(colors[key].length).toBeGreaterThan(0);
    }
  });

  it('has increasing spacing steps', () => {
    const steps = [spacing.xs, spacing.sm, spacing.md, spacing.lg, spacing.xl];
    steps.forEach((s) => expect(typeof s).toBe('number'));
    for (let i = 1; i < steps.length; i++) {
      expect(steps[i]).toBeGreaterThan(steps[i - 1]);
    }
  });

  it('defines body, heading and mono type styles with numeric sizes', () => {
    for (const key of ['body', 'heading', 'mono'] as const) {
      expect(typeof type[key].fontSize).toBe('number');
    }
  });
});
