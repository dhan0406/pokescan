import { act, fireEvent, render, screen, waitFor } from '@testing-library/react-native';

import { ApiError, type ApiClient, type ScanResponse } from '@/api/client';

import { ScanCamera } from './ScanCamera';

let mockFocused = true;
let mockCameraProps: { onCameraReady?: () => void; zoom?: number } = {};
const mockDeleted: string[] = [];
const mockTakePicture = jest.fn();
const mockPrepareUpload = jest.fn();

jest.mock('expo-camera', () => {
  // jest.mock factories cannot use imports from the top of the file.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const React = require('react');
  return {
    CameraView: React.forwardRef(function MockCameraView(props: { onCameraReady?: () => void; zoom?: number }, ref: React.Ref<unknown>) {
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

const retake: ScanResponse = { status: 'retake', reason: 'name_unreadable' };

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

  it('is disabled again when a freshly mounted camera has not reported ready (after "Scan again")', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockResolvedValue(retake)));
    await fireEvent.press(shutter());
    await waitFor(() => expect(screen.getByText('Scan again')).toBeTruthy());

    await fireEvent.press(screen.getByText('Scan again'));
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
    await waitFor(() => expect(screen.getByText('Scan again')).toBeTruthy());
    expect(mockDeleted).toEqual([]); // still showing it as the thumbnail

    await fireEvent.press(screen.getByText('Scan again'));
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
    await waitFor(() => expect(screen.getByText('Scan again')).toBeTruthy());

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

describe('ScanCamera zoom', () => {
  // A card is only 6 cm wide: filling the frame without zoom means holding the
  // phone closer than most lenses can focus, so the camera starts zoomed in.
  it('starts zoomed in', async () => {
    await renderReadyCamera(fakeApi(jest.fn()));
    expect(mockCameraProps.zoom).toBeGreaterThan(0);
  });

  it('zooms in and out in steps with the buttons', async () => {
    await renderReadyCamera(fakeApi(jest.fn()));
    const start = mockCameraProps.zoom as number;

    await fireEvent.press(screen.getByLabelText('Zoom in'));
    expect(mockCameraProps.zoom).toBeGreaterThan(start);

    await fireEvent.press(screen.getByLabelText('Zoom out'));
    await fireEvent.press(screen.getByLabelText('Zoom out'));
    expect(mockCameraProps.zoom).toBeLessThan(start);
  });

  it('never goes below 0 or above the maximum, and disables the button at each end', async () => {
    await renderReadyCamera(fakeApi(jest.fn()));
    for (let i = 0; i < 40; i++) await fireEvent.press(screen.getByLabelText('Zoom out'));
    expect(mockCameraProps.zoom).toBe(0);
    expect(screen.getByLabelText('Zoom out')).toBeDisabled();

    for (let i = 0; i < 40; i++) await fireEvent.press(screen.getByLabelText('Zoom in'));
    expect(mockCameraProps.zoom).toBeLessThanOrEqual(0.6);
    expect(screen.getByLabelText('Zoom in')).toBeDisabled();
  });

  it('keeps the chosen zoom for the next scan', async () => {
    await renderReadyCamera(fakeApi(jest.fn().mockResolvedValue(retake)));
    await fireEvent.press(screen.getByLabelText('Zoom in'));
    const chosen = mockCameraProps.zoom;

    await fireEvent.press(screen.getByLabelText('Take photo'));
    await waitFor(() => expect(screen.getByText('Scan again')).toBeTruthy());
    await fireEvent.press(screen.getByText('Scan again'));
    expect(mockCameraProps.zoom).toBe(chosen);
  });
});
