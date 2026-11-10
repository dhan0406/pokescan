import type { components } from './schema';

export type ScanResponse = components['schemas']['ScanResponse'];
export type Pokemon = components['schemas']['Pokemon'];
export type Card = components['schemas']['Card'];
type ErrorBody = components['schemas']['ErrorBody'];

/** Codes the server sends, plus three the client produces itself. */
export type ApiErrorCode = ErrorBody['code'] | 'network_error' | 'timeout' | 'bad_response';

const SERVER_CODES: ReadonlySet<string> = new Set<ErrorBody['code']>([
  'bad_image',
  'image_too_large',
  'not_found',
  'method_not_allowed',
  'ocr_failed',
  'internal_error',
  'upstream_error',
  'upstream_timeout',
]);

// What the user sees for errors the client produces. Server errors carry their
// own plain-language message. Nothing here ever includes raw exception text or
// the body of an unexpected reply.
const CLIENT_MESSAGES = {
  network_error: "Couldn't reach the server. Check your connection and try again.",
  timeout: 'The server took too long to answer. Try again.',
  bad_response: "The server sent a reply we couldn't read. Try again in a moment.",
} as const;

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status?: number;

  constructor(code: ApiErrorCode, message: string, status?: number) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

export type ApiClientOptions = {
  /** Server address, for example http://192.168.1.5:8080. A trailing slash is ignored. */
  baseUrl: string;
  fetch?: typeof fetch;
  /** Timeout for POST /v1/scan, which includes OCR. Default 25 s. */
  scanTimeoutMs?: number;
  /** Timeout for every other request. Default 10 s. */
  timeoutMs?: number;
};

export type ApiClient = {
  ping(): Promise<void>;
  scan(photoUri: string, signal?: AbortSignal): Promise<ScanResponse>;
  getPokemon(idOrName: string): Promise<Pokemon>;
};

type RequestOptions = { method: 'GET' | 'POST'; body?: FormData; timeoutMs: number; signal?: AbortSignal };

export function createApiClient(opts: ApiClientOptions): ApiClient {
  const baseUrl = opts.baseUrl.replace(/\/+$/, '');
  const doFetch: typeof fetch = opts.fetch ?? ((input, init) => fetch(input, init));
  const scanTimeoutMs = opts.scanTimeoutMs ?? 25_000;
  const timeoutMs = opts.timeoutMs ?? 10_000;

  async function request(path: string, o: RequestOptions): Promise<unknown> {
    const controller = new AbortController();
    let timedOut = false;
    let cancelled = false;
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, o.timeoutMs);
    const onCallerAbort = () => {
      cancelled = true;
      controller.abort();
    };
    if (o.signal?.aborted) onCallerAbort();
    o.signal?.addEventListener('abort', onCallerAbort);

    try {
      let res: Response;
      let text: string;
      try {
        res = await doFetch(baseUrl + path, { method: o.method, body: o.body, signal: controller.signal });
        text = await res.text();
      } catch {
        if (timedOut) throw new ApiError('timeout', CLIENT_MESSAGES.timeout);
        if (cancelled) throw new ApiError('network_error', 'Cancelled.');
        throw new ApiError('network_error', CLIENT_MESSAGES.network_error);
      }

      let parsed: unknown;
      try {
        parsed = JSON.parse(text);
      } catch {
        throw new ApiError('bad_response', CLIENT_MESSAGES.bad_response, res.status);
      }

      if (!res.ok) throw serverError(parsed, res.status);
      return parsed;
    } finally {
      clearTimeout(timer);
      o.signal?.removeEventListener('abort', onCallerAbort);
    }
  }

  return {
    async ping() {
      await request('/ping', { method: 'GET', timeoutMs });
    },

    async scan(photoUri, signal) {
      const form = new FormData();
      // React Native uploads a file by appending an object with a local uri.
      // TypeScript's FormData type does not know about it, hence the cast.
      form.append('image', { uri: photoUri, name: 'card.jpg', type: 'image/jpeg' } as unknown as Blob);
      const body = await request('/v1/scan', { method: 'POST', body: form, timeoutMs: scanTimeoutMs, signal });
      if (!isObject(body) || typeof body.status !== 'string') {
        throw new ApiError('bad_response', CLIENT_MESSAGES.bad_response);
      }
      return body as ScanResponse;
    },

    async getPokemon(idOrName) {
      const body = await request(`/v1/pokemon/${encodeURIComponent(idOrName)}`, { method: 'GET', timeoutMs });
      if (!isObject(body) || typeof body.id !== 'number') {
        throw new ApiError('bad_response', CLIENT_MESSAGES.bad_response);
      }
      return body as unknown as Pokemon;
    },
  };
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Builds an ApiError from an error reply, using the server's message when it sent a valid error body. */
function serverError(parsed: unknown, status: number): ApiError {
  if (isObject(parsed) && typeof parsed.code === 'string' && typeof parsed.message === 'string') {
    const code = SERVER_CODES.has(parsed.code) ? (parsed.code as ErrorBody['code']) : 'internal_error';
    return new ApiError(code, parsed.message, status);
  }
  return new ApiError('bad_response', CLIENT_MESSAGES.bad_response, status);
}
