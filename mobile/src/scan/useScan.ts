import type { CameraView } from 'expo-camera';
import { useCallback, useEffect, useReducer, useRef } from 'react';

import { ApiError, type ApiClient } from '@/api/client';
import { deleteQuietly } from '@/capture/files';
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
  const thumbnail = useRef<string | null>(null); // prepared photo currently shown as the result

  const discardThumbnail = useCallback(() => {
    if (thumbnail.current) deleteQuietly(thumbnail.current);
    thumbnail.current = null;
  }, []);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
      discardThumbnail();
    };
  }, [discardThumbnail]);

  const capture = useCallback(
    async (camera: CameraView, view: Size) => {
      if (busy.current) return;
      busy.current = true;
      dispatch({ type: 'start' });

      const abort = new AbortController();
      controller.current = abort;
      const live = () => mounted.current && !abort.signal.aborted;
      let preparedUri: string | undefined;
      try {
        const photo = await camera.takePictureAsync();
        const prepared = await prepareUpload(photo.uri, { width: photo.width, height: photo.height }, view);
        preparedUri = prepared.uri;
        const response = await api.scan(preparedUri, abort.signal);
        if (live()) {
          thumbnail.current = preparedUri; // kept for the result screen, deleted on reset
          dispatch({ type: 'succeeded', response, thumbnailUri: preparedUri });
        } else {
          deleteQuietly(preparedUri);
        }
      } catch (e) {
        if (preparedUri) deleteQuietly(preparedUri);
        if (live()) dispatch({ type: 'failed', error: e instanceof ApiError ? e : PHOTO_PROBLEM });
      } finally {
        busy.current = false;
      }
    },
    [api],
  );

  const reset = useCallback(() => {
    controller.current?.abort();
    discardThumbnail();
    dispatch({ type: 'reset' });
  }, [discardThumbnail]);

  return { state, capture, reset };
}
