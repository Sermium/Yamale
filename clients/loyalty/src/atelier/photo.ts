/**
 * A jeweler's photo, made fit for the record.
 *
 * Re-encoding as JPEG does two jobs. It bounds the size, so a 12-megapixel
 * phone photo does not cost a buyer on a metered connection 6 MB. And it
 * drops the EXIF block, which can hold the place a photo was taken; a
 * workshop's coordinates have no business in a public record.
 */

export function fitWithin(width: number, height: number, max: number): { width: number; height: number } {
  if (width <= max && height <= max) return { width, height };
  const scale = max / Math.max(width, height);
  return { width: Math.round(width * scale), height: Math.round(height * scale) };
}

export async function preparePhoto(file: Blob, max = 2400, quality = 0.85): Promise<Uint8Array> {
  const bitmap = await createImageBitmap(file); // honours EXIF orientation before the data is dropped
  const { width, height } = fitWithin(bitmap.width, bitmap.height, max);
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  canvas.getContext('2d')!.drawImage(bitmap, 0, 0, width, height);
  bitmap.close();
  const blob = await new Promise<Blob>((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('the photo could not be encoded'))), 'image/jpeg', quality));
  return new Uint8Array(await blob.arrayBuffer());
}
