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

export type ProfileDiffRow = ProfileDiffLine | { type: 'skip'; count: number };

// collapseUnchanged keeps every changed line plus `context` unchanged lines
// around it and folds longer unchanged runs into one `skip` row, so a
// reviewer sees the edits without scrolling the whole profile. A run of a
// single unchanged line is kept as-is (a fold row would be no shorter).
export function collapseUnchanged(lines: ProfileDiffLine[], context = 3): ProfileDiffRow[] {
  const keep = lines.map(() => false);
  lines.forEach((line, index) => {
    if (line.type === 'same') return;
    const from = Math.max(0, index - context);
    const to = Math.min(lines.length - 1, index + context);
    for (let i = from; i <= to; i += 1) keep[i] = true;
  });

  const rows: ProfileDiffRow[] = [];
  let run: ProfileDiffLine[] = [];
  const flush = () => {
    if (run.length === 1) rows.push(run[0]!);
    else if (run.length > 1) rows.push({ type: 'skip', count: run.length });
    run = [];
  };
  lines.forEach((line, index) => {
    if (keep[index]) {
      flush();
      rows.push(line);
    } else {
      run.push(line);
    }
  });
  flush();
  return rows;
}
