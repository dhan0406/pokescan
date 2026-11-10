import type { CameraView } from 'expo-camera';
import { useCallback, useEffect, useReducer, useRef } from 'react';

import { ApiError, type ApiClient } from '@/api/client';
import { prepareUpload } from '@/capture/prepare';
import type { Size } from '@/capture/crop';

import { scanReducer, type ScanState } from './reducer';

const PHOTO_PROBLEM = new ApiError('internal_error', "Couldn't take the photo. Try again.");

/**
 * Runs one scan at a time: take the picture, crop and resize it, upload it,
 * and report the result. Safe against double taps and against the screen
 * going away mid-upload.
 */
export function useScan(api: ApiClient): {
  state: ScanState;
  capture: (camera: CameraView, view: Size) => Promise<void>;
  reset: () => void;
} {
  const [state, dispatch] = useReducer(scanReducer, { phase: 'idle' } as ScanState);
  const busy = useRef(false); // set synchronously, unlike state, so a double tap in one frame is caught
  const mounted = useRef(true);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);

  const capture = useCallback(
    async (camera: CameraView, view: Size) => {
      if (busy.current) return;
      busy.current = true;
      dispatch({ type: 'start' });

      const abort = new AbortController();
      controller.current = abort;
      try {
        const photo = await camera.takePictureAsync();
        const prepared = await prepareUpload(photo.uri, { width: photo.width, height: photo.height }, view);
        const response = await api.scan(prepared.uri, abort.signal);
        if (mounted.current && !abort.signal.aborted) {
          dispatch({ type: 'succeeded', response, thumbnailUri: prepared.uri });
        }
      } catch (e) {
        if (mounted.current && !abort.signal.aborted) {
          dispatch({ type: 'failed', error: e instanceof ApiError ? e : PHOTO_PROBLEM });
        }
      } finally {
        busy.current = false;
      }
    },
    [api],
  );

  const reset = useCallback(() => {
    controller.current?.abort();
    dispatch({ type: 'reset' });
  }, []);

  return { state, capture, reset };
}
