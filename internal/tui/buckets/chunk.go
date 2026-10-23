package buckets

// DividerEvery is the canonical interval (in data rows) at which a divider
// line is emitted by Chunk.
const DividerEvery = 10

// ChunkOptions configures Chunk's interleaving behaviour.
type ChunkOptions struct {
	// StickyBucket is the bucket label to pin at the top of the visible
	// region. When non-empty, a bucket header for this label is emitted as
	// the first output line and any redundant inline header for the same
	// label on the first data row is suppressed.
	StickyBucket string
	// MaxRows caps the total number of output lines (dividers, bucket
	// headers, and data rows combined). Values <= 0 disable the cap.
	MaxRows int
	// DividerEvery is the period at which a divider line is emitted. A
	// divider is emitted after every Nth data row. Zero or negative
	// disables dividers.
	DividerEvery int
	// MakeDivider renders the divider line. Required when DividerEvery > 0.
	MakeDivider func() string
	// MakeBucketHeader renders a bucket header line for the given label.
	// Required when any non-empty bucket label appears.
	MakeBucketHeader func(label string) string
}

// Chunk interleaves dividers and bucket headers into dataRows. bucketLabels
// must be the same length as dataRows; bucketLabels[i] is the bucket label
// for dataRows[i] ("" means no bucketing for that row).
//
// Behaviour:
//   - When opts.StickyBucket is non-empty, an opening bucket header is
//     emitted with that label. If bucketLabels[0] equals that label, the
//     inline header for row 0 is skipped to avoid duplication.
//   - When bucketLabels[i] != "" and differs from the previous emitted
//     bucket label, an inline bucket header for that label is emitted
//     before the data row.
//   - A divider is emitted after every opts.DividerEvery data rows.
//   - Output is truncated to opts.MaxRows total lines (when > 0). If a
//     bucket header or divider cannot fit alongside its data row within
//     the remaining budget, it is skipped (the data row still renders).
func Chunk(dataRows, bucketLabels []string, opts ChunkOptions) []string {
	lines, _ := ChunkWithCount(dataRows, bucketLabels, opts)
	return lines
}

// ChunkWithCount also returns how many data rows survived headers and dividers.
func ChunkWithCount(dataRows, bucketLabels []string, opts ChunkOptions) ([]string, int) {
	if len(dataRows) != len(bucketLabels) {
		// Defensive: fall back to plain data when caller mismatches.
		if opts.MaxRows > 0 && len(dataRows) > opts.MaxRows {
			return dataRows[:opts.MaxRows], opts.MaxRows
		}
		return dataRows, len(dataRows)
	}
	drawn := 0
	c := newChunker(opts, len(dataRows))
	c.emitStickyHeader(len(dataRows))
	for i := 0; i < len(dataRows) && i < len(bucketLabels); i++ {
		if c.remaining <= 0 {
			break
		}
		if c.emitBucketHeader(bucketLabels[i]) {
			// A bucket header already provides visual separation; suppress the
			// divider on the same row so we don't render "[Bucket]" + "─" + row
			// (the divider between two unrelated sections is redundant). Reset
			// the divider counter so the next divider fires DividerEvery rows
			// after this header instead of immediately on the next row.
			c.dataSinceDivider = 0
		} else {
			c.emitDivider()
		}
		c.emitRow(dataRows[i])
		drawn++
	}
	return c.out, drawn
}

// chunker accumulates Chunk's output while tracking the remaining line budget
// and the bucket/divider bookkeeping carried between rows.
type chunker struct {
	opts             ChunkOptions
	out              []string
	remaining        int
	prevBucket       string
	dataSinceDivider int
}

func newChunker(opts ChunkOptions, dataRows int) chunker {
	remaining := opts.MaxRows
	if maxPossible := maxChunkOutputLines(dataRows); opts.MaxRows <= 0 || opts.MaxRows > maxPossible {
		remaining = maxPossible // effectively uncapped
	}
	return chunker{opts: opts, out: make([]string, 0, remaining), remaining: remaining}
}

// emitChrome appends a non-data line, but only when a data row can still follow
// it within the budget. It reports whether the line was written.
func (c *chunker) emitChrome(line string) bool {
	if c.remaining < 2 {
		return false
	}
	c.out = append(c.out, line)
	c.remaining--
	return true
}

// emitStickyHeader pins the sticky bucket at the top of the output. It needs
// room for a data row too, unless there are no data rows at all.
func (c *chunker) emitStickyHeader(dataRows int) {
	if c.opts.StickyBucket == "" || c.opts.MakeBucketHeader == nil {
		return
	}
	if c.remaining >= 2 || dataRows == 0 {
		c.out = append(c.out, c.opts.MakeBucketHeader(c.opts.StickyBucket))
		c.remaining--
	}
	c.prevBucket = c.opts.StickyBucket
}

// emitBucketHeader writes an inline header when label opens a new bucket,
// reporting whether a header line actually fit.
func (c *chunker) emitBucketHeader(label string) bool {
	if label == "" || label == c.prevBucket || c.opts.MakeBucketHeader == nil {
		return false
	}
	emitted := c.emitChrome(c.opts.MakeBucketHeader(label))
	c.prevBucket = label
	return emitted
}

// emitDivider writes the periodic divider once DividerEvery data rows have
// passed. When it cannot fit, dataSinceDivider is intentionally left alone so
// the next row retries (the `>=` guard keeps the counter from outrunning
// DividerEvery and silently disabling all future dividers).
func (c *chunker) emitDivider() {
	if c.opts.DividerEvery <= 0 || c.dataSinceDivider < c.opts.DividerEvery || c.opts.MakeDivider == nil {
		return
	}
	if c.emitChrome(c.opts.MakeDivider()) {
		c.dataSinceDivider = 0
	}
}

func (c *chunker) emitRow(row string) {
	c.out = append(c.out, row)
	c.remaining--
	c.dataSinceDivider++
}

func maxChunkOutputLines(dataRows int) int {
	maxInt := int(^uint(0) >> 1)
	if dataRows > (maxInt-1)/2 {
		return maxInt
	}
	return dataRows*2 + 1
}
