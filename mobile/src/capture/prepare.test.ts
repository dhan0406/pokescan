import { cropRectForPhoto, frameRect, uploadSize } from './crop';
import { prepareUpload } from './prepare';

const mockDeleted: string[] = [];
const mockImage = {
  saveAsync: jest.fn(),
  release: jest.fn(),
};
const mockContext = {
  crop: jest.fn(),
  resize: jest.fn(),
  renderAsync: jest.fn(),
  release: jest.fn(),
};

jest.mock('expo-image-manipulator', () => ({
  SaveFormat: { JPEG: 'jpeg' },
  ImageManipulator: { manipulate: jest.fn(() => mockContext) },
}));

jest.mock('expo-file-system', () => ({
  File: class {
    uri: string;
    constructor(uri: string) {
      this.uri = uri;
    }
    delete() {
      mockDeleted.push(this.uri);
    }
  },
}));

const photo = { width: 3024, height: 4032 };
const view = { width: 390, height: 844 };

beforeEach(() => {
  mockDeleted.length = 0;
  mockContext.crop.mockReset().mockReturnValue(mockContext);
  mockContext.resize.mockReset().mockReturnValue(mockContext);
  mockContext.renderAsync.mockReset().mockResolvedValue(mockImage);
  mockContext.release.mockReset();
  mockImage.saveAsync.mockReset().mockResolvedValue({ uri: 'file:///cache/ImageManipulator/out.jpg', width: 857, height: 1200 });
  mockImage.release.mockReset();
});

describe('prepareUpload', () => {
  it('crops to the frame, resizes to the upload size and saves a JPEG at quality 0.8', async () => {
    const result = await prepareUpload('file:///cache/Camera/photo.jpg', photo, view);

    const crop = cropRectForPhoto(photo, view, frameRect(view));
    expect(mockContext.crop).toHaveBeenCalledWith(crop);
    expect(mockContext.resize).toHaveBeenCalledWith(uploadSize(crop));
    expect(mockImage.saveAsync).toHaveBeenCalledWith({ compress: 0.8, format: 'jpeg' });
    expect(result).toEqual({ uri: 'file:///cache/ImageManipulator/out.jpg', width: 857, height: 1200 });
  });

  it('deletes the full-size source photo but keeps the prepared file', async () => {
    await prepareUpload('file:///cache/Camera/photo.jpg', photo, view);
    expect(mockDeleted).toEqual(['file:///cache/Camera/photo.jpg']);
  });

  it('releases the native image objects', async () => {
    await prepareUpload('file:///cache/Camera/photo.jpg', photo, view);
    expect(mockContext.release).toHaveBeenCalledTimes(1);
    expect(mockImage.release).toHaveBeenCalledTimes(1);
  });

  it('still deletes the source photo and releases the context when saving fails, and rethrows', async () => {
    mockImage.saveAsync.mockRejectedValue(new Error('disk full'));
    await expect(prepareUpload('file:///cache/Camera/photo.jpg', photo, view)).rejects.toThrow('disk full');
    expect(mockDeleted).toEqual(['file:///cache/Camera/photo.jpg']);
    expect(mockContext.release).toHaveBeenCalledTimes(1);
    expect(mockImage.release).toHaveBeenCalledTimes(1);
  });

  it('still deletes the source photo when rendering fails', async () => {
    mockContext.renderAsync.mockRejectedValue(new Error('decode failed'));
    await expect(prepareUpload('file:///cache/Camera/photo.jpg', photo, view)).rejects.toThrow('decode failed');
    expect(mockDeleted).toEqual(['file:///cache/Camera/photo.jpg']);
    expect(mockContext.release).toHaveBeenCalledTimes(1);
  });
});
