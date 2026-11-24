package daemon_test

// Ordinary spoke edits are local-first: transport failure falls back to cached
// lease state. A stalled hub must leave time to check that state and commit the
// edit within the CLI's request budget, including when response headers arrive.
