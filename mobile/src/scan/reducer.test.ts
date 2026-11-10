import { ApiError, type ScanResponse } from '@/api/client';

import { scanReducer, type ScanState } from './reducer';

const response: ScanResponse = { status: 'retake', reason: 'name_unreadable' };
const error = new ApiError('network_error', "Couldn't reach the server.");

const idle: ScanState = { phase: 'idle' };
const working: ScanState = { phase: 'working' };
const done: ScanState = { phase: 'done', response, thumbnailUri: 'file:///thumb.jpg' };
const failed: ScanState = { phase: 'failed', error };

describe('scanReducer', () => {
  it('starts a scan from idle', () => {
    expect(scanReducer(idle, { type: 'start' })).toEqual(working);
  });

  it('carries the response and thumbnail when a scan succeeds', () => {
    expect(scanReducer(working, { type: 'succeeded', response, thumbnailUri: 'file:///thumb.jpg' })).toEqual(done);
  });

  it('carries the error when a scan fails', () => {
    expect(scanReducer(working, { type: 'failed', error })).toEqual(failed);
  });

  it.each([
    ['done', done],
    ['failed', failed],
    ['working (cancel)', working],
  ])('returns to idle on reset from %s', (_name, state) => {
    expect(scanReducer(state, { type: 'reset' })).toEqual(idle);
  });

  it('ignores a second start while working (double tap on the shutter)', () => {
    expect(scanReducer(working, { type: 'start' })).toBe(working);
  });

  it('ignores start while showing a result until reset', () => {
    expect(scanReducer(done, { type: 'start' })).toBe(done);
    expect(scanReducer(failed, { type: 'start' })).toBe(failed);
  });

  it('ignores a late result after the scan was reset', () => {
    expect(scanReducer(idle, { type: 'succeeded', response, thumbnailUri: 'x' })).toBe(idle);
    expect(scanReducer(idle, { type: 'failed', error })).toBe(idle);
  });

  it('ignores a result that arrives when a different result is already showing', () => {
    expect(scanReducer(done, { type: 'failed', error })).toBe(done);
    expect(scanReducer(failed, { type: 'succeeded', response, thumbnailUri: 'x' })).toBe(failed);
  });
});
