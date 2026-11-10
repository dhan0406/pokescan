import { File } from 'expo-file-system';

import { ApiError, createApiClient } from './client';

type Call = { url: string; init: RequestInit };

/** A fetch that records calls and answers with `handler`. */
function fakeFetch(handler: (call: Call) => Response | Promise<Response>) {
  const calls: Call[] = [];
  const fn = jest.fn(async (url: string | URL | Request, init?: RequestInit) => {
    const call = { url: String(url), init: init ?? {} };
    calls.push(call);
    return handler(call);
  });
  return { fetch: fn as unknown as typeof fetch, calls };
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

/** A fetch that never answers but rejects like fetch does when aborted. */
function hangingFetch() {
  return jest.fn(
    (_url: string | URL | Request, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          const e = new Error('Aborted');
          e.name = 'AbortError';
          reject(e);
        });
      }),
  ) as unknown as typeof fetch;
}

// Node's FormData turns React Native's { uri, name, type } file object into a
// string, so tests swap in one that keeps what was appended.
class RecordingFormData {
  parts: [string, unknown][] = [];
  append(name: string, value: unknown) {
    this.parts.push([name, value]);
  }
}
const RealFormData = globalThis.FormData;
beforeEach(() => {
  (globalThis as unknown as { FormData: unknown }).FormData = RecordingFormData;
});
afterEach(() => {
  globalThis.FormData = RealFormData;
});

jest.mock('expo-file-system', () => ({
  File: class {
    uri: string;
    constructor(uri: string) {
      this.uri = uri;
    }
  },
}));

const BASE = 'http://192.168.1.5:8080';

describe('ping', () => {
  it('resolves on a 200 and requests <baseUrl>/ping', async () => {
    const { fetch, calls } = fakeFetch(() => json({ status: 'ok' }));
    await expect(createApiClient({ baseUrl: BASE, fetch }).ping()).resolves.toBeUndefined();
    expect(calls[0].url).toBe(`${BASE}/ping`);
  });

  it('does not produce a double slash for a base URL with a trailing slash', async () => {
    const { fetch, calls } = fakeFetch(() => json({ status: 'ok' }));
    await createApiClient({ baseUrl: `${BASE}/`, fetch }).ping();
    expect(calls[0].url).toBe(`${BASE}/ping`);
  });
});

describe('scan', () => {
  const matchBody = {
    status: 'match',
    card: { id: 'base1-58', name: 'Pikachu', number: '58', setName: 'Base', setTotal: 102, imageUrl: 'https://x/58.png', dexNumber: 25 },
  };

  it('posts multipart with the file part and no Content-Type header', async () => {
    const { fetch, calls } = fakeFetch(() => json(matchBody));
    await createApiClient({ baseUrl: BASE, fetch }).scan('file:///tmp/card.jpg');

    expect(calls[0].url).toBe(`${BASE}/v1/scan`);
    expect(calls[0].init.method).toBe('POST');
    const headers = new Headers(calls[0].init.headers);
    expect(headers.has('Content-Type')).toBe(false);
    // SDK 58's global fetch is expo/fetch, which uploads an expo-file-system File,
    // not React Native's { uri, name, type } object.
    const body = calls[0].init.body as unknown as RecordingFormData;
    expect(body.parts).toHaveLength(1);
    const [field, part] = body.parts[0];
    expect(field).toBe('image');
    expect(part).toBeInstanceOf(File);
    expect((part as { uri: string }).uri).toBe('file:///tmp/card.jpg');
  });

  it.each([
    ['match', matchBody],
    ['choose', { status: 'choose', candidates: [matchBody.card, matchBody.card] }],
    ['retake', { status: 'retake', reason: 'card_unreadable' }],
  ])('returns the parsed %s response', async (_name, body) => {
    const { fetch } = fakeFetch(() => json(body));
    await expect(createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg')).resolves.toEqual(body);
  });

  it('maps a server error body to an ApiError with the server code, status and message', async () => {
    const { fetch } = fakeFetch(() => json({ code: 'bad_image', message: 'That photo could not be read.' }, 400));
    const err = await createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ code: 'bad_image', status: 400, message: 'That photo could not be read.' });
  });

  it('treats an HTML error page as bad_response without leaking the HTML', async () => {
    const html = '<html><body>502 Bad Gateway nginx</body></html>';
    const { fetch } = fakeFetch(() => new Response(html, { status: 502 }));
    const err = await createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe('bad_response');
    expect(err.message).not.toContain('nginx');
    expect(err.message).not.toContain('<html');
  });

  it('treats an HTML 200 as bad_response', async () => {
    const { fetch } = fakeFetch(() => new Response('<html>captive portal</html>', { status: 200 }));
    const err = await createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg').catch((e) => e);
    expect(err).toMatchObject({ code: 'bad_response' });
    expect(err.message).not.toContain('captive');
  });

  it('treats JSON without a status field as bad_response', async () => {
    const { fetch } = fakeFetch(() => json({ hello: 'world' }));
    await expect(createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg')).rejects.toMatchObject({
      code: 'bad_response',
    });
  });

  it('maps a network failure to network_error', async () => {
    const fetch = jest.fn(async () => {
      throw new TypeError('Network request failed');
    }) as unknown as typeof globalThis.fetch;
    const err = await createApiClient({ baseUrl: BASE, fetch }).scan('file:///x.jpg').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe('network_error');
    expect(err.message).not.toContain('Network request failed');
  });

  it('times out with code timeout', async () => {
    const start = Date.now();
    const err = await createApiClient({ baseUrl: BASE, fetch: hangingFetch(), scanTimeoutMs: 50 })
      .scan('file:///x.jpg')
      .catch((e) => e);
    expect(err).toMatchObject({ code: 'timeout' });
    expect(Date.now() - start).toBeLessThan(500);
  });

  it('rejects with network_error "Cancelled." when the caller aborts, and does not hang', async () => {
    const controller = new AbortController();
    const pending = createApiClient({ baseUrl: BASE, fetch: hangingFetch() }).scan('file:///x.jpg', controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ code: 'network_error', message: 'Cancelled.' });
  });
});

describe('onError', () => {
  it('reports the underlying failure while the user still sees the plain message', async () => {
    const boom = new TypeError('Network request failed');
    const fetch = jest.fn(async () => {
      throw boom;
    }) as unknown as typeof globalThis.fetch;
    const onError = jest.fn();
    const err = await createApiClient({ baseUrl: BASE, fetch, onError }).scan('file:///x.jpg').catch((e) => e);
    expect(onError).toHaveBeenCalledWith('/v1/scan', boom);
    expect(err.message).not.toContain('Network request failed');
  });

  it('is not called for errors the server reported', async () => {
    const { fetch } = fakeFetch(() => json({ code: 'bad_image', message: 'Nope.' }, 400));
    const onError = jest.fn();
    await createApiClient({ baseUrl: BASE, fetch, onError }).scan('file:///x.jpg').catch(() => undefined);
    expect(onError).not.toHaveBeenCalled();
  });

  it('is not called when the caller cancels', async () => {
    const controller = new AbortController();
    const onError = jest.fn();
    const pending = createApiClient({ baseUrl: BASE, fetch: hangingFetch(), onError }).scan('file:///x.jpg', controller.signal);
    controller.abort();
    await pending.catch(() => undefined);
    expect(onError).not.toHaveBeenCalled();
  });
});

describe('getPokemon', () => {
  const pikachu = { id: 25, name: 'pikachu', types: ['electric'], stats: {}, flavorText: '', evolution: { name: 'pikachu', evolvesTo: [] } };

  it('requests /v1/pokemon/<encoded id> and returns the body', async () => {
    const { fetch, calls } = fakeFetch(() => json(pikachu));
    await expect(createApiClient({ baseUrl: BASE, fetch }).getPokemon('mr mime')).resolves.toEqual(pikachu);
    expect(calls[0].url).toBe(`${BASE}/v1/pokemon/mr%20mime`);
  });

  it('maps a 404 to not_found', async () => {
    const { fetch } = fakeFetch(() => json({ code: 'not_found', message: "We couldn't find that." }, 404));
    await expect(createApiClient({ baseUrl: BASE, fetch }).getPokemon('x')).rejects.toMatchObject({
      code: 'not_found',
      status: 404,
    });
  });
});
