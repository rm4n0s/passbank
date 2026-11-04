// Package testenv isolates tests from the user's real configuration and loads
// fixtures (SPEC §17, CLAUDE.md "Don't"): no test may read or write the user's
// home folder, configuration, cache, state or runtime folder, and no test may
// open a database that is not a fixture under testdata/.
package testenv
