export type Size = { width: number; height: number };
export type Rect = { originX: number; originY: number; width: number; height: number };

/** A Pokémon card is 2.5 x 3.5 inches. */
export const CARD_ASPECT = 2.5 / 3.5;
/** Photos are resized so their long edge is this many pixels before upload. */
export const UPLOAD_LONG_EDGE = 1200;

/**
 * The card-shaped frame drawn over the camera, in view points: portrait,
 * centered, `widthFraction` of the view's width. If that would be taller than
 * the view, it shrinks to fit.
 */
export function frameRect(view: Size, widthFraction = 0.8): Rect {
  let width = view.width * widthFraction;
  let height = width / CARD_ASPECT;
  if (height > view.height) {
    height = view.height;
    width = height * CARD_ASPECT;
  }
  return {
    originX: (view.width - width) / 2,
    originY: (view.height - height) / 2,
    width,
    height,
  };
}

/**
 * Maps the frame on screen to the matching region of the captured photo.
 *
 * The camera preview scales the photo to *cover* the view and centers it, so
 * part of the photo is off screen when the aspect ratios differ. A point on
 * screen at (x, y) is the photo pixel ((x + offsetX) / scale, (y + offsetY) / scale).
 * The result is integers, clamped inside the photo.
 */
export function cropRectForPhoto(photo: Size, view: Size, frame: Rect): Rect {
  const scale = Math.max(view.width / photo.width, view.height / photo.height);
  const offsetX = (photo.width * scale - view.width) / 2;
  const offsetY = (photo.height * scale - view.height) / 2;

  const left = clamp(Math.floor((frame.originX + offsetX) / scale), 0, photo.width - 1);
  const top = clamp(Math.floor((frame.originY + offsetY) / scale), 0, photo.height - 1);
  const right = clamp(Math.round((frame.originX + frame.width + offsetX) / scale), left + 1, photo.width);
  const bottom = clamp(Math.round((frame.originY + frame.height + offsetY) / scale), top + 1, photo.height);

  return { originX: left, originY: top, width: right - left, height: bottom - top };
}

/** The size to resize a crop to: long edge `UPLOAD_LONG_EDGE`, never upscaled. */
export function uploadSize(crop: Rect): Size {
  const longEdge = Math.max(crop.width, crop.height);
  if (longEdge <= UPLOAD_LONG_EDGE) return { width: crop.width, height: crop.height };
  const scale = UPLOAD_LONG_EDGE / longEdge;
  return { width: Math.round(crop.width * scale), height: Math.round(crop.height * scale) };
}

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max);
}
