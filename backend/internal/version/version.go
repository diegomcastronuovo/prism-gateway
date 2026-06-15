// Package version holds the gateway's static version metadata.
// BackendVersion should be incremented manually when a SPEC is completed or a
// production deployment is prepared. GitCommit and BuildTime can be injected
// at compile time via -ldflags; safe defaults are provided for local builds.
package version

var BackendVersion = "1.0.10"
var GitCommit = "test"
var BuildTime = "04/06/2026 09:00"
var ReleaseNotes = "Initial Full Release"
