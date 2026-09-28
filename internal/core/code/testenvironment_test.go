package code

import (
	"reflect"
	"testing"
)

func TestGoTestEnvironmentDeclaresCompletePinnedRunEnvironment(t *testing.T) {
	config := Config{GOOS: "darwin", GOARCH: "arm64", Toolchain: "go1.25.8", IncludeTests: true}
	environment, err := GoTestEnvironment(config)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"GOOS":         "darwin",
		"GOARCH":       "arm64",
		"CGO_ENABLED":  "0",
		"GOTOOLCHAIN":  "local",
		"GOWORK":       "off",
		"GOFLAGS":      "",
		"GOENV":        "off",
		"GOEXPERIMENT": "",
		"GOFIPS140":    "off",
		"GOPROXY":      "off",
		"GOSUMDB":      "off",
		"GOARM64":      "v8.0",
	}
	if !reflect.DeepEqual(environment, want) {
		t.Fatalf("prepared environment = %#v; want %#v", environment, want)
	}
}
