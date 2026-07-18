export type ProfileDiffLine = {
  type: 'same' | 'added' | 'removed';
  text: string;
};

// lineDiff uses a longest-common-subsequence alignment so a single insertion
// does not make every following line appear changed.
export function lineDiff(original: string, draft: string): ProfileDiffLine[] {
  const left = original.split('\n');
  const right = draft.split('\n');
  const lengths = Array.from({ length: left.length + 1 }, () =>
    new Uint32Array(right.length + 1)
  );

  for (let leftIndex = left.length - 1; leftIndex >= 0; leftIndex -= 1) {
    const row = lengths[leftIndex]!;
    const nextRow = lengths[leftIndex + 1]!;
    for (let rightIndex = right.length - 1; rightIndex >= 0; rightIndex -= 1) {
      row[rightIndex] =
        left[leftIndex] === right[rightIndex]
          ? (nextRow[rightIndex + 1] ?? 0) + 1
          : Math.max(nextRow[rightIndex] ?? 0, row[rightIndex + 1] ?? 0);
    }
  }

  const result: ProfileDiffLine[] = [];
  let leftIndex = 0;
  let rightIndex = 0;
  while (leftIndex < left.length && rightIndex < right.length) {
    if (left[leftIndex] === right[rightIndex]) {
      result.push({ type: 'same', text: left[leftIndex]! });
      leftIndex += 1;
      rightIndex += 1;
    } else if ((lengths[leftIndex + 1]![rightIndex] ?? 0) >= (lengths[leftIndex]![rightIndex + 1] ?? 0)) {
      result.push({ type: 'removed', text: left[leftIndex]! });
      leftIndex += 1;
    } else {
      result.push({ type: 'added', text: right[rightIndex]! });
      rightIndex += 1;
    }
  }
  while (leftIndex < left.length) {
    result.push({ type: 'removed', text: left[leftIndex]! });
    leftIndex += 1;
  }
  while (rightIndex < right.length) {
    result.push({ type: 'added', text: right[rightIndex]! });
    rightIndex += 1;
  }
  return result;
}
