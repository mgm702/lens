// export_test.go exposes internal helpers to the test package.
// This file is only compiled during `go test`.
package simulator

var ExportedRenderPrompt = renderPrompt
var ExportedFormatHistory = formatHistory
