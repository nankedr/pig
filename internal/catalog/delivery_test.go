package catalog_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func readDeliveryJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "parity", path))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
}

func deliveryCatalog(t *testing.T) map[string]catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(repoRoot(t), "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		result[entry.ID] = entry
	}
	return result
}

func TestDeliveryScope(t *testing.T) {
	var scope struct {
		SchemaVersion           int      `json:"schema_version"`
		Decision                string   `json:"decision"`
		Frontier                string   `json:"frontier"`
		BaselineCommit          string   `json:"baseline_commit"`
		CatalogBaselineCommit   string   `json:"catalog_baseline_commit"`
		DefaultRemainingVersion string   `json:"default_remaining_version"`
		DefaultRemainingIssue   int      `json:"default_remaining_issue"`
		Semantics               string   `json:"semantics"`
		Excluded                []string `json:"excluded"`
		ExcludedCatalogIDs      []string `json:"excluded_catalog_ids"`
		Scopes                  []struct {
			ID         string   `json:"id"`
			Version    string   `json:"version"`
			Issues     []int    `json:"issues"`
			Branch     string   `json:"branch"`
			CatalogIDs []string `json:"catalog_ids"`
		} `json:"scopes"`
	}
	readDeliveryJSON(t, "delivery-scope.json", &scope)
	if scope.SchemaVersion != 1 || scope.BaselineCommit != baselineCommit || scope.CatalogBaselineCommit != catalogBaselineCommit {
		t.Fatal("delivery scope must retain both fixed baselines")
	}
	if scope.DefaultRemainingVersion != "V2" || scope.DefaultRemainingIssue != 127 || scope.Frontier != "V1 Responses" {
		t.Fatal("remaining gaps and frontier do not match ADR-0043")
	}
	if _, err := os.Stat(filepath.Join(repoRoot(t), scope.Decision)); err != nil {
		t.Fatal(err)
	}
	entries := deliveryCatalog(t)
	seen := map[string]bool{}
	versions := map[string]string{}
	for _, group := range scope.Scopes {
		if group.ID == "" || seen[group.ID] || group.Branch == "" || len(group.Issues) == 0 || len(group.CatalogIDs) == 0 {
			t.Fatalf("incomplete or duplicate scope: %s", group.ID)
		}
		seen[group.ID] = true
		versions[group.ID] = group.Version
		if group.Version != "V1" && group.Version != "V2" {
			t.Fatalf("invalid version: %s", group.ID)
		}
		refs := map[string]bool{}
		for _, id := range group.CatalogIDs {
			if entries[id].ID == "" || refs[id] {
				t.Fatalf("missing or duplicate Catalog reference in %s: %s", group.ID, id)
			}
			refs[id] = true
			for _, excluded := range scope.ExcludedCatalogIDs {
				if id == excluded {
					t.Fatalf("excluded capability in delivery scope: %s", id)
				}
			}
		}
	}
	for _, id := range []string{"retained", "responses", "image-input", "v1-release"} {
		if versions[id] != "V1" {
			t.Fatalf("missing V1 scope: %s", id)
		}
	}
	for _, id := range []string{"openai-catalog", "providers-auth", "image-remaining", "harness", "remote", "packages", "platforms", "extensions", "full-parity"} {
		if versions[id] != "V2" {
			t.Fatalf("missing V2 scope: %s", id)
		}
	}
	for _, id := range scope.ExcludedCatalogIDs {
		if entries[id].ID == "" {
			t.Fatalf("unknown exclusion: %s", id)
		}
	}
	var manifest catalog.Manifest
	readDeliveryJSON(t, "catalog.manifest.json", &manifest)
	if manifest.DeliveryScope != "delivery-scope.json" || manifest.ResponsesMatrix != "responses-matrix.json" {
		t.Fatal("Catalog manifest must link its delivery and service matrices")
	}
}

func TestResponsesMatrixReport(t *testing.T) {
	var matrix struct {
		SchemaVersion int               `json:"schema_version"`
		CheckedAt     string            `json:"checked_at"`
		Sources       map[string]string `json:"sources"`
		ModelConfig   struct {
			Source        string   `json:"source"`
			BaseURL       string   `json:"base_url"`
			API           string   `json:"api"`
			CredentialEnv string   `json:"credential_env"`
			TextModels    []string `json:"text_models"`
			VisionModels  []string `json:"vision_models"`
			SmokeModel    string   `json:"smoke_model"`
			SmokeVerified bool     `json:"smoke_verified"`
			Note          string   `json:"note"`
		} `json:"model_config"`
		PigStatusSource string `json:"pig_status_source"`
		PiBehaviorRule  string `json:"pi_behavior_rule"`
		Rows            []struct {
			ID               string `json:"id"`
			Standard         string `json:"standard"`
			DeepSeekSupport  string `json:"deepseek_support"`
			DeepSeekBehavior string `json:"deepseek_behavior"`
			CatalogID        string `json:"catalog_id"`
			Version          string `json:"version"`
			Issue            int    `json:"issue"`
			Verification     string `json:"verification"`
		} `json:"rows"`
	}
	readDeliveryJSON(t, "responses-matrix.json", &matrix)
	if matrix.SchemaVersion != 1 || matrix.CheckedAt == "" || matrix.PiBehaviorRule == "" || matrix.PigStatusSource == "" {
		t.Fatal("matrix requires dated sources and separate Pi/Pig semantics")
	}
	for _, name := range []string{"deepseek_guide", "deepseek_reference", "vision", "openai_state"} {
		if !strings.HasPrefix(matrix.Sources[name], "https://") {
			t.Fatalf("missing source: %s", name)
		}
	}
	config := matrix.ModelConfig
	if config.Source != "current-official-docs" || config.BaseURL != "https://api.deepseek.com" || config.API != "openai-responses" || config.CredentialEnv != "DEEPSEEK_API_KEY" || config.Note == "" {
		t.Fatal("current service configuration must have independent provenance")
	}
	contains := func(values []string, target string) bool {
		for _, v := range values {
			if v == target {
				return true
			}
		}
		return false
	}
	if !contains(config.TextModels, config.SmokeModel) || !contains(config.VisionModels, config.SmokeModel) {
		t.Fatal("smoke model must support both text and vision in the recorded sources")
	}
	entries := deliveryCatalog(t)
	var b strings.Builder
	b.WriteString("<!-- GENERATED FILE - DO NOT EDIT BY HAND -->\n# Responses 能力矩阵（生成视图）\n\n")
	fmt.Fprintf(&b, "来源：`parity/responses-matrix.json` + `parity/catalog.jsonl`；官方资料查阅日期：%s。\n\n", matrix.CheckedAt)
	b.WriteString("Pig 状态来自 Catalog，不从 DeepSeek 支持状态推导。`inventoried` 为未实现行为，`scaffolded` 仅有契约/Stub；共享条目在实现前必须细分，不能将一个入口的证据推广到所有行。\n\n")
	b.WriteString("| 能力 | 标准线协议 | DeepSeek 支持状态 / 行为 | Pig 状态 / Catalog ID | 归属 | 验证要求 |\n| --- | --- | --- | --- | --- | --- |\n")
	seen := map[string]bool{}
	for _, row := range matrix.Rows {
		if row.ID == "" || seen[row.ID] || row.Standard == "" || row.DeepSeekBehavior == "" || row.Verification == "" || row.Issue <= 0 {
			t.Fatalf("incomplete or duplicate Responses row: %s", row.ID)
		}
		seen[row.ID] = true
		switch row.DeepSeekSupport {
		case "supported", "partial", "ignored", "unsupported":
		default:
			t.Fatalf("invalid service support: %s", row.ID)
		}
		if row.Version != "V1" && row.Version != "V2" {
			t.Fatalf("invalid delivery version: %s", row.ID)
		}
		entry, ok := entries[row.CatalogID]
		if !ok {
			t.Fatalf("missing Catalog reference: %s", row.CatalogID)
		}
		fmt.Fprintf(&b, "| %s | %s | %s：%s | %s / `%s` | %s / #%d | %s |\n", row.ID, row.Standard, row.DeepSeekSupport, row.DeepSeekBehavior, entry.Status, entry.ID, row.Version, row.Issue, row.Verification)
	}
	for _, id := range []string{"text", "stream", "function-tools", "reasoning", "history", "images", "parallel-limits", "storage-background", "format-sampling", "custom-tools", "hosted-tools", "other-options"} {
		if !seen[id] {
			t.Fatalf("missing Responses boundary: %s", id)
		}
	}
	path := filepath.Join(repoRoot(t), "parity/reports/responses.md")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != b.String() {
		t.Fatal("Responses report is stale; run go test ./internal/catalog -run TestResponsesMatrixReport -update")
	}
}
