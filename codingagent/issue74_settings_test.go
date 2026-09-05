package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestSettingsTypedOverridesPreserveEmptyCollectionsAndSparseJSON(t *testing.T) {
	var settings codingagent.Settings
	raw := `{"compaction":{"reserveTokens":123},"packages":[{"source":"npm:fixture","autoload":false,"extensions":[]}],"future":{"integer":9007199254740993},"defaultModel":null}`
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&want); err != nil {
		t.Fatal(err)
	}
	decoder = json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err = decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %s", encoded)
	}
	manager, err := codingagent.NewInMemorySettingsManager(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.ApplyOverrides(codingagent.Settings{Packages: []codingagent.PackageSource{}}); err != nil {
		t.Fatal(err)
	}
	packages, err := manager.GetPackages()
	if err != nil || len(packages) != 0 {
		t.Fatalf("explicit empty override = %v %v", packages, err)
	}
	compaction, err := manager.GetCompactionSettings()
	if err != nil || !compaction.Enabled || compaction.ReserveTokens != 123 || compaction.KeepRecentTokens != 20000 {
		t.Fatalf("sparse compaction = %+v %v", compaction, err)
	}
}

func TestSettingsGlobalFilePreservesExternalEditsAndOwnsSnapshots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"packages":[{"source":"npm:first","skills":["a"]}],"compaction":{"reserveTokens":100,"keepRecentTokens":200},"future":{"enabled":true}}`)
	manager, err := codingagent.NewSettingsManager(t.TempDir(), &dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.GetGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Packages[0].(*codingagent.FilteredPackageSource).Skills[0] = "mutated"
	snapshot.Extra["future"][0] = '!'
	packages, err := manager.GetPackages()
	if err != nil || packages[0].(*codingagent.FilteredPackageSource).Skills[0] != "a" {
		t.Fatalf("snapshot alias: %v %v", packages, err)
	}
	write(`{"packages":[],"compaction":{"reserveTokens":300,"keepRecentTokens":400},"future":{"enabled":false}}`)
	if err = manager.SetCompactionEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetDefaultModel("saved"); err != nil {
		t.Fatal(err)
	}
	if err = manager.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloaded, err := codingagent.NewSettingsManager(t.TempDir(), &dir)
	if err != nil {
		t.Fatal(err)
	}
	compaction, err := reloaded.GetCompactionSettings()
	if err != nil || compaction.Enabled || compaction.ReserveTokens != 300 || compaction.KeepRecentTokens != 400 {
		t.Fatalf("external nested edit: %+v %v", compaction, err)
	}
	packages, err = reloaded.GetPackages()
	if err != nil || len(packages) != 0 {
		t.Fatalf("external array edit: %v %v", packages, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"enabled": false`) {
		t.Fatalf("unknown data: %s %v", data, err)
	}
}

func TestSettingsProcessWriter(t *testing.T) {
	if os.Getenv("PIG_TEST_SETTINGS_WRITER") != "1" {
		return
	}
	dir := os.Getenv("PIG_TEST_SETTINGS_DIR")
	manager, err := codingagent.NewSettingsManager("unused", &dir)
	if err != nil {
		t.Fatal(err)
	}
	switch os.Getenv("PIG_TEST_SETTINGS_FIELD") {
	case "model":
		err = manager.SetDefaultModel("concurrent")
	case "theme":
		err = manager.SetTheme("dark")
	case "retry":
		err = manager.SetRetryEnabled(false)
	case "compaction":
		err = manager.SetCompactionEnabled(false)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	errs, err := manager.DrainErrors()
	if err != nil || len(errs) > 0 {
		t.Fatalf("write errors: %v %v", errs, err)
	}
}
func TestSettingsConcurrentProcessesPreserveUnrelatedFields(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[exists], func(t *testing.T) {
			dir := t.TempDir()
			if exists {
				if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"future":[1,2],"compaction":{"reserveTokens":42},"retry":{"provider":{"maxRetries":7}}}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var wg sync.WaitGroup
			for _, field := range []string{"model", "theme", "retry", "compaction"} {
				wg.Add(1)
				go func(field string) {
					defer wg.Done()
					command := exec.Command(os.Args[0], "-test.run=^TestSettingsProcessWriter$")
					command.Env = append(os.Environ(), "PIG_TEST_SETTINGS_WRITER=1", "PIG_TEST_SETTINGS_DIR="+dir, "PIG_TEST_SETTINGS_FIELD="+field)
					if output, err := command.CombinedOutput(); err != nil {
						t.Errorf("writer %s: %v %s", field, err, output)
					}
				}(field)
			}
			wg.Wait()
			manager, err := codingagent.NewSettingsManager("unused", &dir)
			if err != nil {
				t.Fatal(err)
			}
			global, err := manager.GetGlobalSettings()
			if err != nil {
				t.Fatal(err)
			}
			if global.DefaultModel == nil || *global.DefaultModel != "concurrent" || global.Theme == nil || *global.Theme != "dark" || global.Retry == nil || global.Retry.Enabled == nil || *global.Retry.Enabled || global.Compaction == nil || global.Compaction.Enabled {
				t.Fatalf("lost update: %+v", global)
			}
			if exists && (string(global.Extra["future"]) != "[\n    1,\n    2\n  ]" && !strings.Contains(string(global.Extra["future"]), "1")) {
				t.Fatalf("lost unknown field: %s", global.Extra["future"])
			}
			if exists && (global.Compaction.ReserveTokens != 42 || global.Retry.Provider == nil || *global.Retry.Provider.MaxRetries != 7) {
				t.Fatal("lost nested fields")
			}
		})
	}
}

func TestSettingsLoadWriteFailuresAndUntrustedBoundary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	manager, err := codingagent.NewSettingsManager("invalid\x00project", &dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read created directory: %v", err)
	}
	if err = manager.SetProjectTrusted(true); err == nil {
		t.Fatal("future project loading succeeded")
	}
	if err = manager.SetProjectPackages([]codingagent.PackageSource{codingagent.StringPackageSource("npm:never")}); err == nil {
		t.Fatal("project save succeeded")
	}
	if err = os.WriteFile(dir, []byte("blocked directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetDefaultModel("local"); err != nil {
		t.Fatal(err)
	}
	if err = manager.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	errs, err := manager.DrainErrors()
	if err != nil || len(errs) != 1 || !strings.HasPrefix(errs[0].Error(), "global settings:") {
		t.Fatalf("write failure = %v %v", errs, err)
	}
	model, err := manager.GetDefaultModel()
	if err != nil || model != "local" {
		t.Fatalf("local value = %q %v", model, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if manager.Reload(canceled) == nil || manager.Flush(canceled) == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestSettingsLockContentionIsObservableAndDoesNotCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	initial := []byte(`{"defaultModel":"initial"}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.NewSettingsManager("unused", &dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetDefaultModel("updated"); err != nil {
		t.Fatal(err)
	}
	errs, err := manager.DrainErrors()
	if err != nil || len(errs) != 1 || !strings.Contains(errs[0].Error(), "settings lock") {
		t.Fatalf("lock failure: %v %v", errs, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(initial) {
		t.Fatalf("locked file changed: %s %v", after, err)
	}
	if err = os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if err = manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	model, err := manager.GetDefaultModel()
	if err != nil || model != "updated" {
		t.Fatalf("failed modification was lost: %s %v", model, err)
	}
}
