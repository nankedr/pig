package codingagent_test

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

var updateIssue104Surface = flag.Bool("update-issue104-surface", false, "regenerate issue #104 API snapshot")

func TestIssue104ThemeAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.Theme{}, codingagent.ThemeLoadResult{}, codingagent.HTMLExportOptions{}, codingagent.DefaultResourceLoaderOptions{}, codingagent.CreateHeadlessSessionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}
	typ := reflect.TypeOf((*codingagent.Theme)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	for _, f := range []any{codingagent.LoadThemeFromPath, codingagent.LoadBuiltinTheme, codingagent.SelectTheme, codingagent.ExportFromFileWithOptions} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue104_surface_golden.txt"
	if *updateIssue104Surface {
		if err := os.WriteFile(path, []byte(out.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(expected) != out.String() {
		t.Fatal("theme API snapshot drifted; use -update-issue104-surface")
	}
}

func TestThemesBuiltinAssetIntegrity(t *testing.T) {
	var manifest struct {
		Commit string
		Assets []struct{ Path, SHA256 string }
	}
	data, err := os.ReadFile("themes/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Commit != issue32BaselineCommit || len(manifest.Assets) != 2 {
		t.Fatal("theme asset provenance")
	}
	for _, asset := range manifest.Assets {
		data, err = os.ReadFile("themes/" + asset.Path)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != asset.SHA256 {
			t.Fatal("theme asset hash", asset.Path)
		}
	}
}
