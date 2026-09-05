package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestHeadlessSettingsSupportImplementedProviders(t *testing.T) {
	for _, test := range []struct{ provider, model string }{{"deepseek", "deepseek-v4-flash"}, {"deepseek", "deepseek-v4-pro"}} {
		t.Run(test.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request["model"] != test.model {
					t.Errorf("model = %v", request["model"])
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"delta\":{\"content\":\"configured\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			level := codingagent.Settings{DefaultProvider: &test.provider, DefaultModel: &test.model}
			settings, err := codingagent.NewInMemorySettingsManager(level)
			if err != nil {
				t.Fatal(err)
			}
			key := "fixture"
			runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), SettingsManager: settings, APIKey: &key, BaseURL: &server.URL, SessionManager: codingagent.NewInMemorySessionManager("unused")})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Session().Dispose()
			outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"hello"}})
			if err != nil || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason == "error" || len(outcome.Text) != 1 || outcome.Text[0] != "configured" {
				t.Fatalf("configured provider: %+v %v", outcome, err)
			}
		})
	}
}

func TestHeadlessFutureSettingsDoNotActivateCapabilities(t *testing.T) {
	cwd := t.TempDir()
	sentinel := filepath.Join(cwd, "executed")
	provider, model := "deepseek", "deepseek-v4-flash"
	analytics := true
	shell := "touch " + sentinel
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultProvider: &provider, DefaultModel: &model, EnableAnalytics: &analytics, EnableInstallTelemetry: &analytics, ShellCommandPrefix: &shell, DefaultProjectTrust: func() *codingagent.DefaultProjectTrust { v := codingagent.DefaultProjectTrustAlways; return &v }()})
	if err != nil {
		t.Fatal(err)
	}
	key := "fixture"
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: cwd, APIKey: &key, SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Session().Dispose(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("startup executed shell: %v", err)
	}
	if err = settings.SetPackages([]codingagent.PackageSource{codingagent.StringPackageSource("npm:never-install")}); err != nil {
		t.Fatal(err)
	}
	if _, err = codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: cwd, SettingsManager: settings}); err == nil {
		t.Fatal("unsupported resource assembly succeeded")
	}
}

func TestHeadlessSettingsGuardsUseEffectiveOverrides(t *testing.T) {
	cwd := t.TempDir()
	provider, model, key := "deepseek", "deepseek-v4-flash", "fixture"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultProvider: &provider, DefaultModel: &model, Packages: []codingagent.PackageSource{codingagent.StringPackageSource("npm:disabled")}})
	if err != nil {
		t.Fatal(err)
	}
	create := func() (*codingagent.AgentSessionRuntime, error) {
		return codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: cwd, SettingsManager: settings, APIKey: &key, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
	}
	if err = settings.ApplyOverrides(codingagent.Settings{Packages: []codingagent.PackageSource{}}); err != nil {
		t.Fatal(err)
	}
	runtime, err := create()
	if err != nil {
		t.Fatalf("disabled resource prevented startup: %v", err)
	}
	if err = runtime.Session().Dispose(); err != nil {
		t.Fatal(err)
	}
	if err = settings.SetPackages([]codingagent.PackageSource{}); err != nil {
		t.Fatal(err)
	}
	if err = settings.ApplyOverrides(codingagent.Settings{Packages: []codingagent.PackageSource{codingagent.StringPackageSource("npm:unavailable")}}); err != nil {
		t.Fatal(err)
	}
	if runtime, err = create(); err == nil {
		runtime.Session().Dispose()
		t.Fatal("resource override falsely succeeded")
	}
	if err = settings.ApplyOverrides(codingagent.Settings{Packages: []codingagent.PackageSource{}}); err != nil {
		t.Fatal(err)
	}
	proxy := "http://127.0.0.1:1"
	if err = settings.ApplyOverrides(codingagent.Settings{HTTPProxy: &proxy}); err != nil {
		t.Fatal(err)
	}
	if runtime, err = create(); err == nil {
		runtime.Session().Dispose()
		t.Fatal("proxy override falsely succeeded")
	}
}
