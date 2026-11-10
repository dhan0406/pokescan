import { deleteQuietly } from './files';

const mockDelete = jest.fn();
const mockUris: string[] = [];

jest.mock('expo-file-system', () => ({
  File: class {
    constructor(uri: string) {
      mockUris.push(uri);
    }
    delete() {
      mockDelete();
    }
  },
}));

beforeEach(() => {
  mockDelete.mockReset();
  mockUris.length = 0;
});

describe('deleteQuietly', () => {
  it('deletes the file at the uri', () => {
    deleteQuietly('file:///cache/Camera/photo.jpg');
    expect(mockUris).toEqual(['file:///cache/Camera/photo.jpg']);
    expect(mockDelete).toHaveBeenCalledTimes(1);
  });

  it('does not throw when the file is already gone or cannot be deleted', () => {
    mockDelete.mockImplementation(() => {
      throw new Error('File does not exist');
    });
    expect(() => deleteQuietly('file:///missing.jpg')).not.toThrow();
  });
});
