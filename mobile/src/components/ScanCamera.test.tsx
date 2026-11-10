import { act, fireEvent, render, screen, waitFor } from '@testing-library/react-native';

import { ApiError, type ApiClient, type ScanResponse } from '@/api/client';

import { ScanCamera } from './ScanCamera';

let mockFocused = true;
let mockCameraProps: { onCameraReady?: () => void } = {};
const mockDeleted: string[] = [];
const mockTakePicture = jest.fn();
const mockPrepareUpload = jest.fn();

jest.mock('expo-camera', () => {
  // jest.mock factories cannot use imports from the top of the file.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const React = require('react');
  return {
    CameraView: React.forwardRef(function MockCameraView(props: { onCameraReady?: () => void }, ref: React.Ref<unknown>) {
      mockCameraProps = props;
      React.useImperativeHandle(ref, () => ({ takePictureAsync: mockTakePicture }));
      return null;
    }),
    useCameraPermissions: () => [{ granted: true, canAskAgain: true }, jest.fn()],
  };
});
jest.mock('expo-router', () => ({ useIsFocused: () => mockFocused }));
jest.mock('@/capture/prepare', () => ({ prepareUpload: (...args: unknown[]) => mockPrepareUpload(...args) }));
jest.mock('@/capture/files', () => ({ deleteQuietly: (uri: string) => mockDeleted.push(uri) }));

const retake: ScanResponse = { status: 'retake', reason: 'card_unreadable' };

function fakeApi(scan: ApiClient['scan']): ApiClient {
  return { ping: jest.fn(), getPokemon: jest.fn(), scan };
}

const shutter = () => screen.getByLabelText('Take photo');

/** React Native reports a view's layout when it mounts; the test does the same. */
async function layoutCamera() {
  await fireEvent(screen.getByTestId('scan-camera'), 'layout', {
    nativeEvent: { layout: { x: 0, y: 0, width: 390, height: 844 } },
  });
}

/** Renders the camera, sizes it, and reports the camera ready. */
async function renderReadyCamera(api: ApiClient) {
  const utils = await render(<ScanCamera api={api} />);
  await layoutCamera();
  await act(async () => mockCameraProps.onCameraReady?.());
  return utils;
}

beforeEach(() => {
  mockFocused = true;
  mockCameraProps = {};
  mockDeleted.length = 0;
  mockTakePicture.mockReset().mockResolvedValue({ uri: 'file:///cache/Camera/p.jpg', width: 3024, height: 4032 });
  mockPrepareUpload.mockReset().mockResolvedValue({ uri: 'file:///cache/ImageManipulator/crop.jpg', width: 857, height: 1200 });
});

describe('ScanCamera shutter', () => {
  it('is disabled until the camera reports ready, then enabled', async () => {
    await render(<ScanCamera api={fakeApi(jest.fn())} />);
    await layoutCamera();
    expect(shutter()).toBeDisabled();
    await act(async () => mockCameraProps.onCameraReady?.());
    expect(shutter()).toBeEnabled();
  });

  it('is disabled again when a freshly mounted camera has not reported ready (after "Scan another card")', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockResolvedValue(retake)));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Scan another card')).toBeTruthy());

    await fireEvent.press(screen.getByText('Scan another card'));
    await layoutCamera();

    // The old camera was unmounted. The new one has not started yet, so a tap
    // now would call takePictureAsync on a camera that is not ready.
    expect(shutter()).toBeDisabled();
    await act(async () => mockCameraProps.onCameraReady?.());
    expect(shutter()).toBeEnabled();
  });

  it('is disabled again when a failed scan is retried', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockRejectedValue(new ApiError('network_error', 'No connection.'))));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Try again')).toBeTruthy());

    await fireEvent.press(screen.getByText('Try again'));
    await layoutCamera();
    expect(shutter()).toBeDisabled();
  });
});

describe('ScanCamera cleanup of cached photos', () => {
  it('deletes the prepared photo when the user scans another card', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockResolvedValue(retake)));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Scan another card')).toBeTruthy());
    expect(mockDeleted).toEqual([]); // still showing it as the thumbnail

    await fireEvent.press(screen.getByText('Scan another card'));
    expect(mockDeleted).toEqual(['file:///cache/ImageManipulator/crop.jpg']);
  });

  it('deletes the prepared photo when the upload fails', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockRejectedValue(new ApiError('timeout', 'Too slow.'))));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Try again')).toBeTruthy());
    expect(mockDeleted).toEqual(['file:///cache/ImageManipulator/crop.jpg']);
  });

  it('deletes the thumbnail when the screen goes away while showing a result', async () => {
    const { unmount } = await renderReadyCamera(fakeApi(jest.fn().mockResolvedValue(retake)));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Scan another card')).toBeTruthy());

    await unmount();
    expect(mockDeleted).toEqual(['file:///cache/ImageManipulator/crop.jpg']);
  });

  it('does not delete anything when preparing the photo itself fails (prepareUpload cleans up after itself)', async () => {
    mockPrepareUpload.mockRejectedValue(new Error('decode failed'));
    await renderReadyCamera(fakeApi(jest.fn()));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Try again')).toBeTruthy());
    expect(mockDeleted).toEqual([]);
  });
});
