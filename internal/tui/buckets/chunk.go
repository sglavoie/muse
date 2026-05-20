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
	if len(dataRows) != len(bucketLabels) {
		// Defensive: fall back to plain data when caller mismatches.
		return dataRows
	}
	remaining := opts.MaxRows
	if opts.MaxRows <= 0 {
		remaining = len(dataRows) * 3 // effectively uncapped
	}
	out := make([]string, 0, remaining)
	prevBucket := ""
	if opts.StickyBucket != "" && opts.MakeBucketHeader != nil {
		if remaining > 0 {
			out = append(out, opts.MakeBucketHeader(opts.StickyBucket))
			remaining--
		}
		prevBucket = opts.StickyBucket
	}
	dataSinceDivider := 0
	for i := 0; i < len(dataRows) && i < len(bucketLabels); i++ {
		if remaining <= 0 {
			break
		}
		row := dataRows[i]
		cur := bucketLabels[i]
		emittedBucketHeader := false
		if cur != "" && cur != prevBucket && opts.MakeBucketHeader != nil {
			if remaining >= 2 {
				out = append(out, opts.MakeBucketHeader(cur))
				remaining--
				emittedBucketHeader = true
			}
			prevBucket = cur
		}
		// A bucket header already provides visual separation; suppress the
		// divider on the same row so we don't render "[Bucket]" + "─" + row
		// (the divider between two unrelated sections is redundant). Reset
		// the divider counter so the next divider fires DividerEvery rows
		// after this header instead of immediately on the next row.
		if emittedBucketHeader {
			dataSinceDivider = 0
		} else if opts.DividerEvery > 0 && dataSinceDivider == opts.DividerEvery &&
			opts.MakeDivider != nil {
			if remaining >= 2 {
				out = append(out, opts.MakeDivider())
				remaining--
				dataSinceDivider = 0
			}
		}
		out = append(out, row)
		remaining--
		dataSinceDivider++
	}
	return out
}
