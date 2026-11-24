/** The loupe's arithmetic, kept out of React so it can be tested. */

export const MIN_ZOOM = 1;
export const MAX_ZOOM = 4;

/** One wheel notch (deltaY ~100) is about a fifth more or less; the zoom stays between 1x and 4x. */
export function wheelZoom(zoom: number, deltaY: number): number {
  const next = zoom * Math.exp(-deltaY * 0.002);
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.round(next * 100) / 100));
}

/**
 * The scroll position that keeps the point under the cursor where it is when
 * the zoom changes: `offset` is the cursor's distance from the view's edge,
 * `scroll` the view's scroll position, on one axis.
 */
export function scrollAbout(scroll: number, offset: number, from: number, to: number): number {
  return Math.max(0, ((scroll + offset) * to) / from - offset);
}
