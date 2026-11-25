/** The board lists at least this many posts, and two full rows when wider. */
export const POST_WALL_MIN = 6

/** A single-column wall, as on phones, lists this many posts. */
export const POST_WALL_ONE_COLUMN = 3

/**
 * How many of the available posts the wall shows at a column count: one
 * column shows up to POST_WALL_ONE_COLUMN; wider walls show up to
 * POST_WALL_MIN or two full rows, whichever is more, and only whole rows, so
 * the grid never ends on a ragged one. Fewer posts than one row show as they
 * are.
 */
export function postWallCount(available: number, columns: number): number {
  const width = Math.max(1, Math.floor(columns))
  const limit =
    width === 1 ? POST_WALL_ONE_COLUMN : Math.max(POST_WALL_MIN, width * 2)
  const shown = Math.min(Math.max(0, available), limit)
  return shown < width ? shown : shown - (shown % width)
}
