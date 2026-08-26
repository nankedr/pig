package codingagent

import (
	"testing"
)

func TestPackageSourcePreservesUnionVariantSemantics(t *testing.T) {
	var stringSource PackageSource = StringPackageSource("github:owner/repo")
	if got := stringSource.ToString(); got != "github:owner/repo" {
		t.Fatalf("string source ToString() = %q, want %q", got, "github:owner/repo")
	}
	if got := stringSource.ValueOf(); got != "github:owner/repo" {
		t.Fatalf("string source ValueOf() = %#v, want the source string", got)
	}

	filteredSource := &FilteredPackageSource{Source: "github:owner/repo"}
	var objectSource PackageSource = filteredSource
	if got := objectSource.ToString(); got != "[object Object]" {
		t.Fatalf("object source ToString() = %q, want %q", got, "[object Object]")
	}
	if got := objectSource.ValueOf(); got != filteredSource {
		t.Fatalf("object source ValueOf() = %#v, want the same object %#v", got, filteredSource)
	}
}

func TestPackageSourceObjectLiteralRemainsObjectWithNoFilters(t *testing.T) {
	source := &FilteredPackageSource{Source: "github:owner/repo"}
	if got := source.ToString(); got != "[object Object]" {
		t.Fatalf("object literal ToString() = %q, want %q", got, "[object Object]")
	}
	if got := source.ValueOf(); got != source {
		t.Fatalf("object literal ValueOf() = %#v, want the object source %#v", got, source)
	}
}
