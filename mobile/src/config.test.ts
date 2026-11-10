import { readApiUrl, setupApi } from './config';

describe('readApiUrl', () => {
  it('returns a valid URL unchanged', () => {
    expect(readApiUrl({ EXPO_PUBLIC_API_URL: 'http://192.168.1.5:8080' })).toBe('http://192.168.1.5:8080');
    expect(readApiUrl({ EXPO_PUBLIC_API_URL: 'https://abc.trycloudflare.com' })).toBe('https://abc.trycloudflare.com');
  });

  it('strips trailing slashes', () => {
    expect(readApiUrl({ EXPO_PUBLIC_API_URL: 'http://192.168.1.5:8080/' })).toBe('http://192.168.1.5:8080');
    expect(readApiUrl({ EXPO_PUBLIC_API_URL: 'http://192.168.1.5:8080//' })).toBe('http://192.168.1.5:8080');
  });

  it('trims surrounding whitespace', () => {
    expect(readApiUrl({ EXPO_PUBLIC_API_URL: ' http://192.168.1.5:8080 \n' })).toBe('http://192.168.1.5:8080');
  });

  it.each([
    ['undefined', undefined],
    ['empty', ''],
    ['blank', '   '],
    ['no scheme', '192.168.1.5:8080'],
    ['wrong scheme', 'ftp://192.168.1.5'],
  ])('throws for %s, and the message names EXPO_PUBLIC_API_URL with an example', (_name, value) => {
    expect(() => readApiUrl({ EXPO_PUBLIC_API_URL: value })).toThrow(/EXPO_PUBLIC_API_URL/);
    expect(() => readApiUrl({ EXPO_PUBLIC_API_URL: value })).toThrow(/http:\/\/192\.168/);
  });
});

describe('setupApi', () => {
  it('returns a client when the URL is valid', () => {
    const setup = setupApi({ EXPO_PUBLIC_API_URL: 'http://192.168.1.5:8080' });
    expect(setup.ok).toBe(true);
    if (setup.ok) {
      expect(typeof setup.api.ping).toBe('function');
      expect(typeof setup.api.scan).toBe('function');
    }
  });

  it('returns the message instead of throwing when the URL is missing', () => {
    const setup = setupApi({ EXPO_PUBLIC_API_URL: undefined });
    expect(setup.ok).toBe(false);
    if (!setup.ok) expect(setup.message).toMatch(/EXPO_PUBLIC_API_URL/);
  });
});
