//go:build integration && perfregress

package perf

import "time"

// injectedRegression is the negative control for budget 1.
//
// Built with -tags perfregress, every durable-accept iteration sleeps this long
// inside its measured window. The budget is p95 <= 250 ms, so a harness that
// still reports the budget met with this build is measuring something other
// than the accept path, and none of its numbers may be cited. The delay lives
// in benchmark setup, never in product code: the product binary cannot be
// built with it.
const injectedRegression = 300 * time.Millisecond
