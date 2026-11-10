import type { ApiError, ScanResponse } from '@/api/client';

export type ScanState =
  | { phase: 'idle' }
  | { phase: 'working' }
  | { phase: 'done'; response: ScanResponse; thumbnailUri: string }
  | { phase: 'failed'; error: ApiError };

export type ScanAction =
  | { type: 'start' }
  | { type: 'succeeded'; response: ScanResponse; thumbnailUri: string }
  | { type: 'failed'; error: ApiError }
  | { type: 'reset' };

/**
 * One scan at a time. Anything that does not make sense for the current phase
 * (a second tap while working, a result arriving after the user backed out) is
 * ignored by returning the same state object.
 */
export function scanReducer(state: ScanState, action: ScanAction): ScanState {
  switch (action.type) {
    case 'start':
      return state.phase === 'idle' ? { phase: 'working' } : state;
    case 'succeeded':
      return state.phase === 'working'
        ? { phase: 'done', response: action.response, thumbnailUri: action.thumbnailUri }
        : state;
    case 'failed':
      return state.phase === 'working' ? { phase: 'failed', error: action.error } : state;
    case 'reset':
      return state.phase === 'idle' ? state : { phase: 'idle' };
  }
}
