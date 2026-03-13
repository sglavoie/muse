package library

// Scan runs the three-stage scanner pipeline for root and returns a channel
// that receives Track values as batches complete:
//
//  1. A single walker goroutine emits audio file paths into a buffered channel.
//  2. runtime.NumCPU() hash worker goroutines consume paths concurrently and
//     emit HashedFile values into a second buffered channel.
//  3. A single ExifTool batch goroutine accumulates HashedFile values and
//     invokes extractor.Extract on each ~500-file batch, emitting Track values.
//
// The returned channel is closed once the walk has finished and all hash and
// ExifTool goroutines have drained, signalling scan-complete. Callers drain
// the channel with a range loop; channel close is the completion signal.
func Scan(root string, extractor MetadataExtractor) <-chan Track {
	return ExtractBatches(extractor, HashWorkers(Walk(root)))
}
