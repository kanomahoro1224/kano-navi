/**
 * For each day of the week, how many empty days its row stands for in the
 * list layout: a run of two or more empty days (today never among them) is
 * one row, its first day counting the run and the rest 0; any other day is 1.
 */
export function emptyRuns(empty: boolean[]): number[] {
  const runs = empty.map(() => 1)
  for (let start = 0; start < empty.length;) {
    let end = start
    while (empty[end]) end += 1
    if (end - start > 1) {
      runs[start] = end - start
      for (let day = start + 1; day < end; day += 1) runs[day] = 0
    }
    start = Math.max(end, start + 1)
  }
  return runs
}
