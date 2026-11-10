import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';

import { cropRectForPhoto, frameRect, uploadSize, type Size } from './crop';
import { deleteQuietly } from './files';

export type PreparedUpload = { uri: string; width: number; height: number };

/**
 * Crops a captured photo to the card frame shown on screen, resizes it to the
 * upload size and saves it as a JPEG. `photo` is the photo's pixel size and
 * `view` the size of the camera view the frame was drawn in.
 *
 * The full-size source photo is deleted once it has been processed, and the
 * native image objects are released so the decoded bitmap is freed now rather
 * than whenever the JS garbage collector runs. The returned file is the
 * caller's to delete.
 */
export async function prepareUpload(photoUri: string, photo: Size, view: Size): Promise<PreparedUpload> {
  const crop = cropRectForPhoto(photo, view, frameRect(view));
  const target = uploadSize(crop);

  const context = ImageManipulator.manipulate(photoUri).crop(crop).resize(target);
  let image: Awaited<ReturnType<typeof context.renderAsync>> | undefined;
  try {
    image = await context.renderAsync();
    const saved = await image.saveAsync({ compress: 0.8, format: SaveFormat.JPEG });
    return { uri: saved.uri, width: saved.width, height: saved.height };
  } finally {
    image?.release();
    context.release();
    deleteQuietly(photoUri);
  }
}
