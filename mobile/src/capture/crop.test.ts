import { CARD_ASPECT, UPLOAD_LONG_EDGE, cropRectForPhoto, frameRect, uploadSize } from './crop';

const near = (actual: number, expected: number, tol = 1) => Math.abs(actual - expected) <= tol;

describe('frameRect', () => {
  it('is a centered 2.5:3.5 portrait frame at 90% of the view width', () => {
    const f = frameRect({ width: 390, height: 844 });
    expect(f.width).toBeCloseTo(351);
    expect(f.height).toBeCloseTo(351 / CARD_ASPECT);
    expect(f.originX).toBeCloseTo((390 - f.width) / 2);
    expect(f.originY).toBeCloseTo((844 - f.height) / 2);
  });

  it('shrinks to fit when the view is too short for the card at that width', () => {
    const view = { width: 400, height: 400 };
    const f = frameRect(view);
    expect(f.height).toBeLessThanOrEqual(view.height);
    expect(f.width / f.height).toBeCloseTo(CARD_ASPECT);
    expect(f.originX).toBeGreaterThanOrEqual(0);
    expect(f.originY).toBeGreaterThanOrEqual(0);
  });
});

describe('cropRectForPhoto', () => {
  const view = { width: 390, height: 844 };

  function expectValidCrop(crop: ReturnType<typeof cropRectForPhoto>, photo: { width: number; height: number }) {
    for (const v of [crop.originX, crop.originY, crop.width, crop.height]) {
      expect(Number.isInteger(v)).toBe(true);
    }
    expect(crop.originX).toBeGreaterThanOrEqual(0);
    expect(crop.originY).toBeGreaterThanOrEqual(0);
    expect(crop.width).toBeGreaterThan(0);
    expect(crop.height).toBeGreaterThan(0);
    expect(crop.originX + crop.width).toBeLessThanOrEqual(photo.width);
    expect(crop.originY + crop.height).toBeLessThanOrEqual(photo.height);
  }

  it('doubles the frame when the photo is exactly twice the view at the same aspect', () => {
    const photo = { width: 780, height: 1688 };
    const frame = frameRect(view);
    const crop = cropRectForPhoto(photo, view, frame);
    expect(near(crop.originX, frame.originX * 2)).toBe(true);
    expect(near(crop.originY, frame.originY * 2)).toBe(true);
    expect(near(crop.width, frame.width * 2)).toBe(true);
    expect(near(crop.height, frame.height * 2)).toBe(true);
    expectValidCrop(crop, photo);
  });

  it('keeps the card aspect and stays centered for a 3:4 photo in a tall view (preview crops the sides)', () => {
    const photo = { width: 3024, height: 4032 };
    const crop = cropRectForPhoto(photo, view, frameRect(view));
    expectValidCrop(crop, photo);
    expect(Math.abs(crop.width / crop.height / CARD_ASPECT - 1)).toBeLessThan(0.005);
    expect(near(crop.originX + crop.width / 2, photo.width / 2)).toBe(true);
  });

  it('stays valid for a landscape photo in a portrait view', () => {
    const photo = { width: 4032, height: 3024 };
    const crop = cropRectForPhoto(photo, view, frameRect(view));
    expectValidCrop(crop, photo);
    expect(Math.abs(crop.width / crop.height / CARD_ASPECT - 1)).toBeLessThan(0.01);
  });

  it('stays valid for a photo much taller than the view (preview crops top and bottom)', () => {
    const photo = { width: 1000, height: 4000 };
    const crop = cropRectForPhoto(photo, view, frameRect(view));
    expectValidCrop(crop, photo);
    expect(near(crop.originY + crop.height / 2, photo.height / 2, 2)).toBe(true);
  });

  it('clamps a frame that touches the view edge instead of overshooting the photo', () => {
    for (const photo of [
      { width: 780, height: 1688 },
      { width: 3024, height: 4032 },
      { width: 1001, height: 2003 },
    ]) {
      const crop = cropRectForPhoto(photo, view, frameRect(view, 1));
      expectValidCrop(crop, photo);
    }
  });
});

describe('uploadSize', () => {
  it('scales the long edge down to 2000 and keeps the aspect ratio', () => {
    const size = uploadSize({ originX: 0, originY: 0, width: 2300, height: 3220 });
    expect(UPLOAD_LONG_EDGE).toBe(2000);
    expect(size.height).toBe(2000);
    expect(near(size.width, (2300 / 3220) * 2000)).toBe(true);
  });

  it('uses the width as the long edge for a landscape crop', () => {
    const size = uploadSize({ originX: 0, originY: 0, width: 3000, height: 2000 });
    expect(size).toEqual({ width: 2000, height: 1333 });
  });

  it('never upscales', () => {
    expect(uploadSize({ originX: 5, originY: 5, width: 1400, height: 1960 })).toEqual({ width: 1400, height: 1960 });
  });
});
