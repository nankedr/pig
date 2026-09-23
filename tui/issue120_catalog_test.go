package tui_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
)

func issue120Promote(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue31GoPackage+".")
	if name != "SettingsList" && !strings.HasPrefix(name, "SettingsList.") {
		return false
	}
	e.Status = catalog.StatusImplemented
	e.Partial = nil
	e.Notes = "Issue #120; settings search, navigation, submenu return and rendering; contract:codingagent/interactive-themes"
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue120_menu_test.go", Baseline: issue31BaselineCommit, CaseID: e.ID, InputHash: issue31SurfaceHash, ExecutionMethod: "go test ./codingagent -run 'TestThemeSettings.*120' -count=1", Expected: "locked Pi settings search and theme menu behavior", Actual: "PASS; full hashed fixture evidence in contract:codingagent/interactive-themes", Platform: "any", CatalogID: e.ID}}
	return true
}
