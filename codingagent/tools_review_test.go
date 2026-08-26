package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestToolDefinitionExecutionSlotsRemainOpaqueUntilM7(t *testing.T) {
	definitionType := reflect.TypeOf(codingagent.ToolDefinition{})
	handlerType := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	wantFields := map[string]reflect.Type{
		"ConstrainedSampling": reflect.TypeOf((*ai.ConstrainedSampling)(nil)).Elem(),
		"Description":         reflect.TypeOf(""),
		"Execute":             handlerType,
		"ExecutionMode":       reflect.TypeOf(agent.ToolExecutionMode("")),
		"Label":               reflect.TypeOf(""),
		"Name":                reflect.TypeOf(""),
		"Parameters":          reflect.TypeOf(json.RawMessage(nil)),
		"PrepareArguments":    handlerType,
		"PromptGuidelines":    reflect.TypeOf([]string(nil)),
		"PromptSnippet":       reflect.TypeOf(""),
		"RenderCall":          handlerType,
		"RenderResult":        handlerType,
		"RenderShell":         reflect.TypeOf(""),
	}
	if definitionType.NumField() != len(wantFields) {
		t.Errorf("ToolDefinition has %d fields, want the %d pinned metadata/opaque fields", definitionType.NumField(), len(wantFields))
	}
	for name, want := range wantFields {
		field, ok := definitionType.FieldByName(name)
		if !ok {
			t.Errorf("ToolDefinition.%s is missing", name)
			continue
		}
		if field.Type != want {
			t.Errorf("ToolDefinition.%s type = %v, want %v", name, field.Type, want)
		}
		if field.Type.Kind() == reflect.Func {
			t.Errorf("ToolDefinition.%s remains executable before M7", name)
		}
	}
}

func TestToolDefinitionHasNoHiddenExecutableResource(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve tools review test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "tools.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "ToolDefinition" {
				continue
			}
			found = true
			carrier, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("ToolDefinition declaration = %T, want struct", typeSpec.Type)
			}
			for _, field := range carrier.Fields.List {
				for _, name := range field.Names {
					if !name.IsExported() {
						t.Errorf("ToolDefinition contains hidden resource field %q", name.Name)
					}
				}
				ast.Inspect(field.Type, func(node ast.Node) bool {
					if _, executable := node.(*ast.FuncType); executable {
						t.Errorf("ToolDefinition contains executable function syntax in %v", field.Type)
						return false
					}
					return true
				})
			}
		}
	}
	if !found {
		t.Fatal("ToolDefinition declaration is missing")
	}
}

func TestBuiltinToolDefinitionFactoriesAreCapabilityStubs(t *testing.T) {
	disabled := false
	resizeImages := true
	tests := []struct {
		name          string
		operation     string
		factory       any
		wantSignature reflect.Type
		call          func(*int) (codingagent.ToolDefinition, error)
	}{
		{
			name:          "bash",
			operation:     "CreateBashToolDefinition",
			factory:       codingagent.CreateBashToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.BashToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateBashToolDefinition("invalid\x00path", codingagent.BashToolOptions{
					Operations:               countingBashOperations{calls: calls},
					ExposeSessionEnvironment: &disabled,
					SpawnHook: func(spawnContext codingagent.BashSpawnContext) codingagent.BashSpawnContext {
						*calls++
						return spawnContext
					},
				})
			},
		},
		{
			name:          "edit",
			operation:     "CreateEditToolDefinition",
			factory:       codingagent.CreateEditToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.EditToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateEditToolDefinition("invalid\x00path", codingagent.EditToolOptions{Operations: countingEditOperations{calls: calls}})
			},
		},
		{
			name:          "find",
			operation:     "CreateFindToolDefinition",
			factory:       codingagent.CreateFindToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.FindToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateFindToolDefinition("invalid\x00path", codingagent.FindToolOptions{Operations: countingFindOperations{calls: calls}})
			},
		},
		{
			name:          "grep",
			operation:     "CreateGrepToolDefinition",
			factory:       codingagent.CreateGrepToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.GrepToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateGrepToolDefinition("invalid\x00path", codingagent.GrepToolOptions{Operations: countingGrepOperations{calls: calls}})
			},
		},
		{
			name:          "ls",
			operation:     "CreateLsToolDefinition",
			factory:       codingagent.CreateLsToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.LsToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateLsToolDefinition("invalid\x00path", codingagent.LsToolOptions{Operations: countingLsOperations{calls: calls}})
			},
		},
		{
			name:          "read",
			operation:     "CreateReadToolDefinition",
			factory:       codingagent.CreateReadToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.ReadToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateReadToolDefinition("invalid\x00path", codingagent.ReadToolOptions{
					AutoResizeImages: &resizeImages,
					Operations:       countingReadOperations{calls: calls},
				})
			},
		},
		{
			name:          "write",
			operation:     "CreateWriteToolDefinition",
			factory:       codingagent.CreateWriteToolDefinition,
			wantSignature: reflect.TypeOf((func(string, ...codingagent.WriteToolOptions) (codingagent.ToolDefinition, error))(nil)),
			call: func(calls *int) (codingagent.ToolDefinition, error) {
				return codingagent.CreateWriteToolDefinition("invalid\x00path", codingagent.WriteToolOptions{Operations: countingWriteOperations{calls: calls}})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := reflect.TypeOf(test.factory); got != test.wantSignature {
				t.Fatalf("%s signature = %v, want %v", test.operation, got, test.wantSignature)
			}

			calls := 0
			definition, err := test.call(&calls)
			if !reflect.DeepEqual(definition, codingagent.ToolDefinition{}) {
				t.Fatalf("%s result = %#v, want zero value", test.operation, definition)
			}
			if !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("%s error = %v, want ErrNotImplemented", test.operation, err)
			}
			var unavailable *codingagent.NotImplementedError
			if !errors.As(err, &unavailable) {
				t.Fatalf("%s error = %T, want *NotImplementedError", test.operation, err)
			}
			if unavailable.Module != "codingagent" || unavailable.Operation != test.operation {
				t.Fatalf("%s error = %#v, want module codingagent and exact operation", test.operation, unavailable)
			}
			if calls != 0 {
				t.Fatalf("%s invoked an operation or callback %d times, want zero", test.operation, calls)
			}
		})
	}
}

type countingBashOperations struct{ calls *int }

func (operations countingBashOperations) Exec(context.Context, string, string, codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
	*operations.calls++
	return codingagent.BashExecResult{}, nil
}

type countingEditOperations struct{ calls *int }

func (operations countingEditOperations) Access(context.Context, string) error {
	*operations.calls++
	return nil
}
func (operations countingEditOperations) ReadFile(context.Context, string) ([]byte, error) {
	*operations.calls++
	return nil, nil
}
func (operations countingEditOperations) WriteFile(context.Context, string, []byte) error {
	*operations.calls++
	return nil
}

type countingFindOperations struct{ calls *int }

func (operations countingFindOperations) Exists(context.Context, string) (bool, error) {
	*operations.calls++
	return false, nil
}
func (operations countingFindOperations) Glob(context.Context, string, string, codingagent.FindGlobOptions) ([]string, error) {
	*operations.calls++
	return nil, nil
}

type countingGrepOperations struct{ calls *int }

func (operations countingGrepOperations) IsDirectory(context.Context, string) (bool, error) {
	*operations.calls++
	return false, nil
}
func (operations countingGrepOperations) ReadFile(context.Context, string) (string, error) {
	*operations.calls++
	return "", nil
}

type countingLsOperations struct{ calls *int }

func (operations countingLsOperations) Exists(context.Context, string) (bool, error) {
	*operations.calls++
	return false, nil
}
func (operations countingLsOperations) Readdir(context.Context, string) ([]string, error) {
	*operations.calls++
	return nil, nil
}
func (operations countingLsOperations) Stat(context.Context, string) (codingagent.LsFileInfo, error) {
	*operations.calls++
	return nil, nil
}

type countingReadOperations struct{ calls *int }

func (operations countingReadOperations) Access(context.Context, string) error {
	*operations.calls++
	return nil
}
func (operations countingReadOperations) DetectImageMimeType(context.Context, string) (*string, error) {
	*operations.calls++
	return nil, nil
}
func (operations countingReadOperations) ReadFile(context.Context, string) ([]byte, error) {
	*operations.calls++
	return nil, nil
}

type countingWriteOperations struct{ calls *int }

func (operations countingWriteOperations) Mkdir(context.Context, string) error {
	*operations.calls++
	return nil
}
func (operations countingWriteOperations) WriteFile(context.Context, string, []byte) error {
	*operations.calls++
	return nil
}

func TestGenerateUnifiedPatch(t *testing.T) {
	tests := []struct {
		name    string
		oldText string
		newText string
		context int
		want    string
	}{
		{
			name:    "identical input keeps upstream file headers",
			oldText: "a\n",
			newText: "a\n",
			context: 4,
			want:    "--- f\n+++ f\n",
		},
		{
			name:    "replacement orders removal before addition",
			oldText: "a\nb\nc\n",
			newText: "a\nB\nc\n",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n",
		},
		{
			name:    "repeated lines follow upstream alignment",
			oldText: "a\nb\na\n",
			newText: "a\na\nb\n",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,3 +1,3 @@\n a\n-b\n a\n+b\n",
		},
		{
			name:    "create terminated file",
			newText: "one\ntwo\n",
			context: 4,
			want:    "--- f\n+++ f\n@@ -0,0 +1,2 @@\n+one\n+two\n",
		},
		{
			name:    "delete terminated file",
			oldText: "one\ntwo\n",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,2 +0,0 @@\n-one\n-two\n",
		},
		{
			name:    "create unterminated file",
			newText: "one",
			context: 4,
			want:    "--- f\n+++ f\n@@ -0,0 +1,1 @@\n+one\n\\ No newline at end of file\n",
		},
		{
			name:    "delete unterminated file",
			oldText: "one",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,1 +0,0 @@\n-one\n\\ No newline at end of file\n",
		},
		{
			name:    "add newline at EOF",
			oldText: "a",
			newText: "a\n",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,1 +1,1 @@\n-a\n\\ No newline at end of file\n+a\n",
		},
		{
			name:    "remove newline at EOF",
			oldText: "a\n",
			newText: "a",
			context: 4,
			want:    "--- f\n+++ f\n@@ -1,1 +1,1 @@\n-a\n+a\n\\ No newline at end of file\n",
		},
		{
			name:    "separated edits form separate hunks",
			oldText: "a\nb\nc\nd\ne\nf\ng\nh\n",
			newText: "a\nB\nc\nd\ne\nf\nG\nh\n",
			context: 1,
			want:    "--- f\n+++ f\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n@@ -6,3 +6,3 @@\n f\n-g\n+G\n h\n",
		},
		{
			name:    "zero context insertion uses empty old range",
			oldText: "a\nb\n",
			newText: "a\nx\nb\n",
			context: 0,
			want:    "--- f\n+++ f\n@@ -1,0 +2,1 @@\n+x\n",
		},
		{
			name:    "zero context deletion uses empty new range",
			oldText: "a\nx\nb\n",
			newText: "a\nb\n",
			context: 0,
			want:    "--- f\n+++ f\n@@ -2,1 +1,0 @@\n-x\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := codingagent.GenerateUnifiedPatch("f", test.oldText, test.newText, test.context); got != test.want {
				t.Fatalf("GenerateUnifiedPatch() mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}

	t.Run("large sparse change remains practical", func(t *testing.T) {
		const lineCount = 50_000
		lines := make([]string, lineCount)
		for i := range lines {
			lines[i] = fmt.Sprintf("line-%05d", i)
		}
		oldText := strings.Join(lines, "\n") + "\n"
		lines[lineCount/2] = "changed"
		newText := strings.Join(lines, "\n") + "\n"

		patch := codingagent.GenerateUnifiedPatch("large.txt", oldText, newText, 1)
		if !strings.Contains(patch, "@@ -25000,3 +25000,3 @@\n") {
			t.Fatalf("large sparse patch has wrong hunk header: %q", patch)
		}
		if !strings.Contains(patch, "-line-25000\n+changed\n") {
			t.Fatalf("large sparse patch does not contain the replacement: %q", patch)
		}
	})
}

func TestGenerateDiffString(t *testing.T) {
	tests := []struct {
		name          string
		oldText       string
		newText       string
		context       int
		wantDiff      string
		wantFirstLine int
	}{
		{
			name:          "replacement",
			oldText:       "a\nb\nc\n",
			newText:       "a\nB\nc\n",
			context:       4,
			wantDiff:      " 1 a\n-2 b\n+2 B\n 3 c",
			wantFirstLine: 2,
		},
		{
			name:          "newline added at EOF",
			oldText:       "a",
			newText:       "a\n",
			context:       4,
			wantDiff:      "-1 a\n+1 a",
			wantFirstLine: 1,
		},
		{
			name:          "newline removed at EOF",
			oldText:       "a\n",
			newText:       "a",
			context:       4,
			wantDiff:      "-1 a\n+1 a",
			wantFirstLine: 1,
		},
		{
			name:          "separated edits use one omission marker",
			oldText:       "a\nb\nc\nd\ne\nf\ng\nh\n",
			newText:       "a\nB\nc\nd\ne\nf\nG\nh\n",
			context:       1,
			wantDiff:      " 1 a\n-2 b\n+2 B\n 3 c\n   ...\n 6 f\n-7 g\n+7 G\n 8 h",
			wantFirstLine: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := codingagent.GenerateDiffString(test.oldText, test.newText, test.context)
			if got.Diff != test.wantDiff || got.FirstChangedLine != test.wantFirstLine {
				t.Fatalf("GenerateDiffString() = %#v, want diff %q and first line %d", got, test.wantDiff, test.wantFirstLine)
			}
		})
	}
}

func TestWithFileMutationQueueIsCapabilityStub(t *testing.T) {
	called := false
	got, err := codingagent.WithFileMutationQueue(context.Background(), "invalid\x00path", func(context.Context) (string, error) {
		called = true
		return "unexpected", nil
	})
	if got != "" || called || !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("WithFileMutationQueue() = (%q, %v), callback called=%v; want zero, ErrNotImplemented, false", got, err, called)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) || unavailable.Module != "codingagent" || unavailable.Operation != "WithFileMutationQueue" {
		t.Fatalf("structured error = %#v, want codingagent.WithFileMutationQueue", unavailable)
	}
}

func TestTruncationTypesAliasAgent(t *testing.T) {
	tests := []struct {
		name string
		got  reflect.Type
		want reflect.Type
	}{
		{"TruncationLimit", reflect.TypeOf(codingagent.TruncationLimit("")), reflect.TypeOf(agent.TruncationLimit(""))},
		{"TruncationOptions", reflect.TypeOf(codingagent.TruncationOptions{}), reflect.TypeOf(agent.TruncationOptions{})},
		{"TruncationResult", reflect.TypeOf(codingagent.TruncationResult{}), reflect.TypeOf(agent.TruncationResult{})},
		{"LineTruncation", reflect.TypeOf(codingagent.LineTruncation{}), reflect.TypeOf(agent.LineTruncation{})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Errorf("codingagent.%s type = %v, want exact alias of %v", test.name, test.got, test.want)
			}
		})
	}

	if codingagent.DefaultMaxLines != agent.DefaultMaxLines || codingagent.DefaultMaxBytes != agent.DefaultMaxBytes {
		t.Errorf("default truncation limits = (%d, %d), want agent limits (%d, %d)", codingagent.DefaultMaxLines, codingagent.DefaultMaxBytes, agent.DefaultMaxLines, agent.DefaultMaxBytes)
	}
	if string(codingagent.TruncationByLines) != string(agent.TruncationByLines) || string(codingagent.TruncationByBytes) != string(agent.TruncationByBytes) {
		t.Errorf("truncation limit constants = (%q, %q), want agent constants (%q, %q)", codingagent.TruncationByLines, codingagent.TruncationByBytes, agent.TruncationByLines, agent.TruncationByBytes)
	}
}

func TestTruncationWrapperSignatures(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"TruncateHead", codingagent.TruncateHead, (func(string, ...codingagent.TruncationOptions) codingagent.TruncationResult)(nil)},
		{"TruncateTail", codingagent.TruncateTail, (func(string, ...codingagent.TruncationOptions) codingagent.TruncationResult)(nil)},
		{"TruncateLine", codingagent.TruncateLine, (func(string, ...int) codingagent.LineTruncation)(nil)},
		{"FormatSize", codingagent.FormatSize, (func(int64) string)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, want := reflect.TypeOf(test.got), reflect.TypeOf(test.want); got != want {
				t.Errorf("codingagent.%s signature = %v, want %v", test.name, got, want)
			}
		})
	}
}

func TestTruncationWrappersMatchAgent(t *testing.T) {
	type truncationCase struct {
		name    string
		content string
		options []codingagent.TruncationOptions
	}
	cases := []truncationCase{
		{name: "defaults", content: "alpha\nbeta"},
		{name: "explicit limits", content: "alpha\nbeta\ngamma", options: []codingagent.TruncationOptions{{MaxLines: 2, MaxBytes: 100}}},
		{name: "negative limits", content: "alpha\nbeta", options: []codingagent.TruncationOptions{{MaxLines: -1, MaxBytes: -1}}},
		{name: "only first option is used", content: "alpha\nbeta\ngamma", options: []codingagent.TruncationOptions{{MaxLines: 1, MaxBytes: 100}, {MaxLines: 3, MaxBytes: 100}}},
	}
	for _, test := range cases {
		t.Run("head/"+test.name, func(t *testing.T) {
			limits := agent.TruncationOptions{}
			if len(test.options) != 0 {
				limits = agent.TruncationOptions{MaxLines: test.options[0].MaxLines, MaxBytes: test.options[0].MaxBytes}
			}
			if got, want := codingagent.TruncateHead(test.content, test.options...), agent.TruncateHead(test.content, limits); !reflect.DeepEqual(got, want) {
				t.Fatalf("TruncateHead() = %#v, want agent result %#v", got, want)
			}
		})
		t.Run("tail/"+test.name, func(t *testing.T) {
			limits := agent.TruncationOptions{}
			if len(test.options) != 0 {
				limits = agent.TruncationOptions{MaxLines: test.options[0].MaxLines, MaxBytes: test.options[0].MaxBytes}
			}
			if got, want := codingagent.TruncateTail(test.content, test.options...), agent.TruncateTail(test.content, limits); !reflect.DeepEqual(got, want) {
				t.Fatalf("TruncateTail() = %#v, want agent result %#v", got, want)
			}
		})
	}

	lineCases := []struct {
		name   string
		limits []int
	}{
		{name: "default"},
		{name: "explicit", limits: []int{2}},
		{name: "negative", limits: []int{-1}},
		{name: "only first limit is used", limits: []int{2, 5}},
	}
	for _, test := range lineCases {
		t.Run("line/"+test.name, func(t *testing.T) {
			limit := 0
			if len(test.limits) != 0 {
				limit = test.limits[0]
			}
			if got, want := codingagent.TruncateLine("alpha", test.limits...), agent.TruncateLine("alpha", limit); !reflect.DeepEqual(got, want) {
				t.Fatalf("TruncateLine() = %#v, want agent result %#v", got, want)
			}
		})
	}

	for _, size := range []int64{-1, 0, 1023, 1024, 1024 * 1024} {
		if got, want := codingagent.FormatSize(size), agent.FormatSize(size); got != want {
			t.Errorf("FormatSize(%d) = %q, want agent result %q", size, got, want)
		}
	}
}
