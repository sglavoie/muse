package main

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

// upsertBatchSize is the number of tracks per database upsert batch during
// library scanning and rescanning.
const upsertBatchSize = 500
