package codingagent_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/codingagent"
)

// legacyAgentSessionBoundary makes the production composition explicit without
// allowing codingagent to duplicate the lower-level Agent implementation.
type legacyAgentSessionBoundary interface {
	Agent() *agent.Agent
	SessionManager() *codingagent.SessionManager
}

var _ legacyAgentSessionBoundary = (*codingagent.AgentSession)(nil)

type panicCredentialStore struct{ calls *int }

func (s panicCredentialStore) called() {
	(*s.calls)++
	panic("credential store must not be called by a capability stub")
}

func (s panicCredentialStore) Read(context.Context, ai.ProviderID, ai.AuthOperationOptions) (ai.Credential, error) {
	s.called()
	return nil, nil
}

func (s panicCredentialStore) List(context.Context, ai.AuthOperationOptions) ([]ai.CredentialInfo, error) {
	s.called()
	return nil, nil
}

func (s panicCredentialStore) Modify(context.Context, ai.ProviderID, ai.CredentialModifyFunc, ai.AuthOperationOptions) (ai.Credential, error) {
	s.called()
	return nil, nil
}

func (s panicCredentialStore) Delete(context.Context, ai.ProviderID, ai.AuthOperationOptions) error {
	s.called()
	return nil
}

type panicModelsStore struct{ calls *int }

func (s panicModelsStore) Read(context.Context, ai.ProviderID) (ai.ModelsStoreEntry, bool, error) {
	(*s.calls)++
	panic("models store must not be called by a capability stub")
}

func (s panicModelsStore) Write(context.Context, ai.ProviderID, ai.ModelsStoreEntry) error {
	(*s.calls)++
	panic("models store must not be called by a capability stub")
}

func (s panicModelsStore) Delete(context.Context, ai.ProviderID) error {
	(*s.calls)++
	panic("models store must not be called by a capability stub")
}

func TestCreateAgentSessionAssemblyStubsAreSideEffectFree(t *testing.T) {
	env := newSDKSentinelEnvironment(t)
	ctx := context.Background()

	result, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{
		CWD:      env.cwd,
		AgentDir: env.agentDir,
	})
	assertCodingAgentNotImplemented(t, err, "CreateAgentSession")
	if !reflect.DeepEqual(result, codingagent.CreateAgentSessionResult{}) {
		t.Fatalf("CreateAgentSession result = %#v, want zero value", result)
	}

	services, err := codingagent.CreateAgentSessionServices(ctx, codingagent.CreateAgentSessionServicesOptions{
		CWD:      env.cwd,
		AgentDir: env.agentDir,
	})
	assertCodingAgentNotImplemented(t, err, "CreateAgentSessionServices")
	if !reflect.DeepEqual(services, codingagent.AgentSessionServices{}) {
		t.Fatalf("CreateAgentSessionServices result = %#v, want zero value", services)
	}

	fromServices, err := codingagent.CreateAgentSessionFromServices(ctx, codingagent.CreateAgentSessionFromServicesOptions{})
	assertCodingAgentNotImplemented(t, err, "CreateAgentSessionFromServices")
	if !reflect.DeepEqual(fromServices, codingagent.CreateAgentSessionResult{}) {
		t.Fatalf("CreateAgentSessionFromServices result = %#v, want zero value", fromServices)
	}

	env.assertUnchanged(t)
}

func TestModelRuntimeStubsDoNotReadCredentialsModelsOrNetwork(t *testing.T) {
	env := newSDKSentinelEnvironment(t)
	credentialCalls, modelStoreCalls := 0, 0
	runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{
		Credentials:       panicCredentialStore{calls: &credentialCalls},
		ModelsStore:       panicModelsStore{calls: &modelStoreCalls},
		AuthPath:          filepath.Join(env.agentDir, "auth.json"),
		ModelsPath:        ai.Some(filepath.Join(env.agentDir, "models.json")),
		AllowModelNetwork: true,
		CatalogBaseURL:    "http://127.0.0.1:1/must-not-connect",
	})
	assertCodingAgentNotImplemented(t, err, "NewModelRuntime")
	if runtime != nil {
		t.Fatalf("NewModelRuntime result = %#v, want nil", runtime)
	}

	var zero codingagent.ModelRuntime
	err = zero.Refresh(context.Background(), ai.ModelsRefreshOptions{AllowNetwork: ai.Some(true)})
	assertCodingAgentNotImplemented(t, err, "ModelRuntime.Refresh")
	if credentialCalls != 0 || modelStoreCalls != 0 {
		t.Fatalf("dependency calls = credentials %d, models %d; want zero", credentialCalls, modelStoreCalls)
	}
	env.assertUnchanged(t)
}

func TestSettingsResourcePackageAndTrustStubsDoNotTouchHostState(t *testing.T) {
	env := newSDKSentinelEnvironment(t)
	ctx := context.Background()

	trusted := true
	settings, err := codingagent.NewSettingsManager(env.cwd, &env.agentDir, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted})
	assertCodingAgentNotImplemented(t, err, "NewSettingsManager")
	if settings != nil {
		t.Fatalf("NewSettingsManager result = %#v, want nil", settings)
	}
	var zeroSettings codingagent.SettingsManager
	assertCodingAgentNotImplemented(t, zeroSettings.Reload(ctx), "SettingsManager.Reload")

	var loader codingagent.DefaultResourceLoader
	assertCodingAgentNotImplemented(t, loader.Reload(ctx), "DefaultResourceLoader.Reload")
	assertCodingAgentNotImplemented(t, loader.ExtendResources(codingagent.ResourceExtensionPaths{}), "DefaultResourceLoader.ExtendResources")

	var packages codingagent.DefaultPackageManager
	resolved, err := packages.Resolve(ctx, nil)
	assertCodingAgentNotImplemented(t, err, "DefaultPackageManager.Resolve")
	if !reflect.DeepEqual(resolved, codingagent.ResolvedPaths{}) {
		t.Fatalf("Resolve result = %#v, want zero value", resolved)
	}

	store := codingagent.NewProjectTrustStore(env.agentDir)
	decision, err := store.Get(ctx, env.cwd)
	assertCodingAgentNotImplemented(t, err, "ProjectTrustStore.Get")
	if decision != nil {
		t.Fatalf("Get decision = %#v, want nil", decision)
	}
	assertCodingAgentNotImplemented(t, store.Set(ctx, env.cwd, codingagent.ProjectTrustDecisionTrusted()), "ProjectTrustStore.Set")

	env.assertUnchanged(t)
}

func TestInteractiveComponentInvalidationIsAnExplicitCapabilityStub(t *testing.T) {
	var component codingagent.ArminComponent
	assertCodingAgentNotImplemented(t, component.Invalidate(), "Component.Invalidate")
}

func TestDefaultToolFactoryIsAnInertCapabilityStub(t *testing.T) {
	env := newSDKSentinelEnvironment(t)
	tool, err := codingagent.CreateReadTool(env.cwd, codingagent.ReadToolOptions{})
	assertCodingAgentNotImplemented(t, err, "CreateReadTool")
	if !reflect.DeepEqual(tool, agent.ErasedAgentTool{}) {
		t.Fatalf("CreateReadTool result = %#v, want zero value", tool)
	}
	env.assertUnchanged(t)
}

func TestRemoteSessionStubsDoNotCreateTransportOrLeaseState(t *testing.T) {
	transportCalls := 0
	c, err := client.NewClient(client.ClientOptions{
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			transportCalls++
			panic("transport factory must not be called by a capability stub")
		},
	})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	opened, err := codingagent.OpenRemoteSession(context.Background(), c, "session-1", codingagent.RemoteSessionOptions{})
	assertCodingAgentNotImplemented(t, err, "OpenRemoteSession")
	if opened != nil {
		t.Fatalf("OpenRemoteSession result = %#v, want nil", opened)
	}

	created, err := codingagent.CreateRemoteSession(context.Background(), c, codingagent.CreateRemoteSessionOptions{CWD: "/sentinel/cwd"}, codingagent.RemoteSessionOptions{})
	assertCodingAgentNotImplemented(t, err, "CreateRemoteSession")
	if created != nil {
		t.Fatalf("CreateRemoteSession result = %#v, want nil", created)
	}
	if transportCalls != 0 {
		t.Fatalf("transport factory calls = %d, want zero", transportCalls)
	}
	if c.ConnectionState() != client.ConnectionStateDisconnected || c.Connected() || c.Disposed() || c.Snapshot() != nil {
		t.Fatal("remote-session stubs changed client state")
	}
}

func TestProductionV3SessionTypesRemainSeparateFromHarnessV4(t *testing.T) {
	if codingagent.CurrentSessionVersion != 3 {
		t.Fatalf("CurrentSessionVersion = %d, want 3", codingagent.CurrentSessionVersion)
	}
	production := reflect.TypeOf((*codingagent.SessionManager)(nil))
	harnessSession := reflect.TypeOf((*agent.Session)(nil))
	harnessRuntime := reflect.TypeOf((*agent.AgentHarness)(nil))
	if production == harnessSession || production == harnessRuntime {
		t.Fatalf("production v3 SessionManager aliases a Harness v4 type: %v", production)
	}
}

func TestAmbientStateCapabilityStubsCannotImportHostIO(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	packageDir := filepath.Dir(thisFile)
	forbidden := map[string]bool{
		"io/fs":    true,
		"net":      true,
		"net/http": true,
		"os":       true,
		"os/exec":  true,
	}
	for _, name := range []string{
		"models.go",
		"packages.go",
		"resources.go",
		"sdk.go",
		"session_services.go",
		"settings.go",
		"trust.go",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(packageDir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote %s import %s: %v", name, spec.Path.Value, err)
			}
			if forbidden[path] {
				t.Errorf("ambient-state scaffold %s imports host I/O package %q", name, path)
			}
		}
	}
}

func assertCodingAgentNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var target *codingagent.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "codingagent" || target.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want module codingagent operation %s", target, operation)
	}
}

type sdkSentinelEnvironment struct {
	root, cwd, agentDir string
	before              map[string]sdkTreeEntry
}

type sdkTreeEntry struct {
	Mode    os.FileMode
	Content string
	Link    string
}

func newSDKSentinelEnvironment(t *testing.T) sdkSentinelEnvironment {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(home, ".pig", "agent")
	for _, dir := range []string{agentDir, filepath.Join(cwd, ".pig"), filepath.Join(cwd, ".agents", "skills")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	files := map[string]string{
		filepath.Join(agentDir, "auth.json"):                "not-json-auth",
		filepath.Join(agentDir, "models.json"):              "not-json-models",
		filepath.Join(agentDir, "settings.json"):            "not-json-global-settings",
		filepath.Join(agentDir, "trust.json"):               "not-json-trust",
		filepath.Join(cwd, ".pig", "settings.json"):         "not-json-project-settings",
		filepath.Join(cwd, ".pig", "SYSTEM.md"):             "must-not-be-read",
		filepath.Join(cwd, ".agents", "skills", "SKILL.md"): "must-not-be-read",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PIG_OFFLINE", "false")
	return sdkSentinelEnvironment{root: root, cwd: cwd, agentDir: agentDir, before: snapshotSDKTree(t, root)}
}

func (e sdkSentinelEnvironment) assertUnchanged(t *testing.T) {
	t.Helper()
	got := snapshotSDKTree(t, e.root)
	if !reflect.DeepEqual(got, e.before) {
		t.Fatalf("stub changed sentinel tree:\n got: %#v\nwant: %#v", got, e.before)
	}
}

func snapshotSDKTree(t *testing.T, root string) map[string]sdkTreeEntry {
	t.Helper()
	snapshot := make(map[string]sdkTreeEntry)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		record := sdkTreeEntry{Mode: info.Mode()}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			record.Link, err = os.Readlink(path)
		case info.Mode().IsRegular():
			var data []byte
			data, err = os.ReadFile(path)
			record.Content = string(data)
		}
		if err != nil {
			return err
		}
		snapshot[relative] = record
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot sentinel tree: %v", err)
	}
	return snapshot
}
