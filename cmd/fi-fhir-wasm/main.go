//go:build js && wasm

// Command fi-fhir-wasm exposes the fi-fhir kernel to a browser page as a
// handful of JavaScript globals: the production Source Profile compiler, the
// production HL7v2 parser, and the FHIR projection the durable `fhir`
// destination uses (internal/integration/kernel). No filesystem, no network:
// nothing a page pastes into it leaves the page.
//
// Build with `make wasm` (dist/wasm/fi-fhir.wasm + wasm_exec.js), and load it
// with the wasm_exec.js shipped beside it. Once main has run, the page finds
// on globalThis:
//
//	fiFhirReady                  -> true
//	fiFhirVersion()              -> JSON {version, commit, builtAt, kernel: "slim"}
//	fiFhirProfiles()             -> JSON [{id, name, description, yaml}]
//	fiFhirSamples()              -> JSON [{id, name, source, format, text}]
//	fiFhirPreview(requestJSON)   -> JSON {ok, segments, events, diagnostics, bundle?, bundleProblem?, problems}
//	fiFhirValidateProfile(yaml)  -> JSON {ok, problems}
//
// Every function takes and returns strings and never throws: a bad request,
// profile, or message is {ok: false, problems: [{code, path, message}]}.
// cmd/fi-fhir-wasm/README.md documents the shapes; scripts/wasm-smoke.mjs
// pins them.
package main

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"syscall/js"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/kernel"
)

// Set by `make wasm` through -ldflags -X; the build information fills the
// gaps for a plain `go build`.
var (
	version = "dev"
	commit  = ""
	builtAt = ""
)

func main() {
	resolveBuildInfo()

	js.Global().Set("fiFhirVersion", js.FuncOf(func(js.Value, []js.Value) any {
		return guarded(func() string {
			return encode(map[string]string{
				"version": version,
				"commit":  commit,
				"builtAt": builtAt,
				"kernel":  "slim",
			})
		})
	}))
	js.Global().Set("fiFhirProfiles", js.FuncOf(func(js.Value, []js.Value) any {
		return guardedList(func() (any, error) { return kernel.Profiles() })
	}))
	js.Global().Set("fiFhirSamples", js.FuncOf(func(js.Value, []js.Value) any {
		return guardedList(func() (any, error) { return kernel.Samples() })
	}))
	js.Global().Set("fiFhirPreview", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guarded(func() string {
			input, ok := argString(args, 0)
			if !ok {
				return encode(kernel.PreviewResponse{
					Segments:    []kernel.Segment{},
					Events:      []kernel.Event{},
					Diagnostics: []kernel.Diagnostic{},
					Problems:    []kernel.Problem{{Code: kernel.CodeInputInvalid, Path: "$", Message: "fiFhirPreview takes one JSON string"}},
				})
			}
			return encode(kernel.Preview([]byte(input)))
		})
	}))
	js.Global().Set("fiFhirValidateProfile", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guarded(func() string {
			input, ok := argString(args, 0)
			if !ok {
				return encode(kernel.ValidateResponse{
					Problems: []kernel.Problem{{Code: kernel.CodeInputInvalid, Path: "$", Message: "fiFhirValidateProfile takes one YAML string"}},
				})
			}
			return encode(kernel.ValidateProfile(input))
		})
	}))
	js.Global().Set("fiFhirReady", js.ValueOf(true))

	// Keep the program alive; the page calls in through the globals above.
	select {}
}

func resolveBuildInfo() {
	if version == "" {
		version = "dev"
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch {
		case setting.Key == "vcs.revision" && commit == "":
			commit = setting.Value
		case setting.Key == "vcs.time" && builtAt == "":
			builtAt = setting.Value
		}
	}
}

// argString reads a string argument. A missing or non-string argument is
// reported, never coerced: String() on a JavaScript object yields
// "<object>", which would parse as garbage.
func argString(args []js.Value, index int) (string, bool) {
	if index >= len(args) || args[index].Type() != js.TypeString {
		return "", false
	}
	// Size is the kernel's check (INPUT_TOO_LARGE, MESSAGE_TOO_LARGE,
	// PROFILE_TOO_LARGE), so an oversized call gets the specific problem.
	return args[index].String(), true
}

// guarded turns a panic anywhere below into the contract's failure shape, so
// no call can throw into the page.
func guarded(call func() string) (result string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = encode(map[string]any{
				"ok":       false,
				"problems": []kernel.Problem{{Code: kernel.CodeInternalError, Path: "$", Message: fmt.Sprintf("the kernel failed: %v", recovered)}},
			})
		}
	}()
	return call()
}

// guardedList returns a JSON array, or an empty one if the embedded assets
// could not be read (a build defect that the Go tests rule out).
func guardedList(call func() (any, error)) string {
	return guarded(func() string {
		value, err := call()
		if err != nil {
			return "[]"
		}
		return encode(value)
	})
}

func encode(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"problems":[{"code":%q,"path":"$","message":%q}]}`, kernel.CodeInternalError, err.Error())
	}
	return string(data)
}
