package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestLocalExtensionsSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/local-extensions.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Scenarios []struct {
			Name              string
			Files             map[string]string
			Links             map[string]string
			Trusted, Disabled bool
			Global            map[string][]string
			Paths             []string
		}
	}
	var want []struct {
		Entries []codingagent.ResolvedResource
		Errors  []struct{ Path, Error string }
	}
	if err = json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	for i, s := range input.Scenarios {
		t.Run(s.Name, func(t *testing.T) {
			root := t.TempDir()
			expand := func(p string) string { return strings.ReplaceAll(strings.ReplaceAll(p, "$ROOT", root), ".pi", ".pig") }
			for p, content := range s.Files {
				context100Write(t, expand(filepath.Join(root, p)), content)
			}
			for p, target := range s.Links {
				if err := os.Symlink(expand(target), expand(filepath.Join(root, p))); err != nil {
					t.Fatal(err)
				}
			}
			cwd, agentDir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
			data, _ := json.Marshal(s.Global)
			data = []byte(expand(string(data)))
			context100Write(t, filepath.Join(agentDir, "settings.json"), string(data))
			settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
			if err != nil {
				t.Fatal(err)
			}
			if err = settings.SetProjectTrusted(s.Trusted); err != nil {
				t.Fatal(err)
			}
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoExtensions: s.Disabled, AdditionalExtensionPaths: s.Paths})
			if err != nil {
				t.Fatal(err)
			}
			if err = loader.Reload(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, err := loader.GetExtensionDiscovery()
			if err != nil {
				t.Fatal(err)
			}
			expected := want[i].Entries
			for j := range expected {
				expected[j].Path = expand(expected[j].Path)
				expected[j].Metadata.BaseDir = expand(expected[j].Metadata.BaseDir)
				if expected[j].Metadata.Scope == codingagent.SourceScopeTemporary {
					expected[j].Metadata.Origin = codingagent.ResourceOriginTopLevel
				}
			}
			if !reflect.DeepEqual(got.Entries, expected) {
				t.Fatalf("entries = %#v, want %#v", got.Entries, expected)
			}
			if len(want[i].Errors) == 0 {
				t.Fatal("fixture must exercise failed loading")
			}
			result, err := loader.GetExtensions()
			var unavailable *codingagent.NotImplementedError
			if !errors.Is(err, codingagent.ErrNotImplemented) || !errors.As(err, &unavailable) || !reflect.DeepEqual(result, codingagent.LoadExtensionsResult{}) {
				t.Fatalf("execution result=%#v error=%v", result, err)
			}
			for _, d := range got.Diagnostics {
				if !strings.Contains(d.Message, "not executed") {
					t.Fatalf("misleading diagnostic: %#v", d)
				}
			}
			if len(got.Entries) > 0 {
				got.Entries[0].Path = "mutated"
				again, _ := loader.GetExtensionDiscovery()
				if again.Entries[0].Path == "mutated" {
					t.Fatal("borrowed discovery snapshot")
				}
			}
		})
	}
}

func TestLocalExtensionsReloadAndStubBoundaries(t *testing.T) {
	s, _, _, _, dir := reload106Session(t)
	loader := s.ResourceLoader().(*reload106Loader).ResourceLoader.(*codingagent.DefaultResourceLoader)
	path := filepath.Join(dir, "extensions", "effect.js")
	context100Write(t, path, `require('node:fs').writeFileSync('EXECUTED','bad'); throw Error('MODULE IMPORTED');`)
	before := s.SystemPrompt()
	if err := s.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	found, err := loader.GetExtensionDiscovery()
	if err != nil || len(found.Entries) != 1 {
		t.Fatalf("%+v %v", found, err)
	}
	if s.SystemPrompt() != before {
		t.Fatal("discovery changed prompt")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := loader.Reload(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertCodingAgentNotImplemented(t, loader.Reload(context.Background(), codingagent.ResourceLoaderReloadOptions{ResolveProjectTrust: newOpaqueHandler107()}), "DefaultResourceLoader.Reload")
	if _, err := loader.LoadProjectTrustExtensions(context.Background()); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatal(err)
	}
	assertCodingAgentNotImplemented(t, loader.ExtendResources(codingagent.ResourceExtensionPaths{}), "DefaultResourceLoader.ExtendResources")
	if _, err := codingagent.DiscoverAndLoadExtensions(); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatal(err)
	}
	for _, options := range []codingagent.DefaultResourceLoaderOptions{
		{ExtensionFactories: []codingagent.InlineExtension{{}}},
		{ExtensionsOverride: newOpaqueHandler107()},
		{SkillsOverride: newOpaqueHandler107()},
	} {
		if _, err := codingagent.NewDefaultResourceLoader(options); !errors.Is(err, codingagent.ErrNotImplemented) {
			t.Fatal(err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	found, err = loader.GetExtensionDiscovery()
	if err != nil || len(found.Entries) != 0 {
		t.Fatalf("deleted entry retained: %+v %v", found, err)
	}
}

func newOpaqueHandler107() codingagent.ExtensionHandler {
	return reflect.New(reflect.TypeOf(codingagent.ExtensionHandler(nil)).Elem()).Convert(reflect.TypeOf(codingagent.ExtensionHandler(nil))).Interface().(codingagent.ExtensionHandler)
}
