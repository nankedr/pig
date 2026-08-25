package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/ai"
)

// Result represents an expected operation outcome. Promise rejection and
// capability failure remain the separate Go error returned by the operation.
type Result[T any] struct {
	OK    bool
	Value T
	Error error
}

func OK[T any](value T) Result[T] {
	return Result[T]{OK: true, Value: value}
}

func Err[T any](err error) Result[T] {
	return Result[T]{Error: err}
}

func GetOrThrow[T any](result Result[T]) (T, error) {
	return result.Value, result.Error
}

func GetOrUndefined[T any](result Result[T]) (T, bool) {
	return result.Value, result.OK
}

func ToError(value any) error {
	if err, ok := value.(error); ok {
		return err
	}
	return &valueError{value: value}
}

type valueError struct {
	value any
}

func (e *valueError) Error() string {
	return fmt.Sprint(e.value)
}

type TaggedErrorValue struct {
	Tag        string
	Message    string
	Name       string
	Stack      string
	Cause      error
	Properties map[string]any
	factory    *struct{}
}

func (e *TaggedErrorValue) Error() string { return e.Message }
func (e *TaggedErrorValue) Unwrap() error { return e.Cause }

func (e *TaggedErrorValue) ToJSON() map[string]any {
	payload := make(map[string]any, len(e.Properties)+2)
	for key, value := range e.Properties {
		if key != "_tag" {
			payload[key] = value
		}
	}
	payload["_tag"] = e.Tag
	payload["message"] = e.Message
	return payload
}

type TaggedErrorFactory struct {
	tag   string
	token *struct{}
}

func TaggedError(tag string) TaggedErrorFactory {
	return TaggedErrorFactory{tag: tag, token: &struct{}{}}
}

func (f TaggedErrorFactory) New(message string, properties map[string]any) *TaggedErrorValue {
	values := make(map[string]any, len(properties))
	for key, value := range properties {
		values[key] = value
	}
	cause, _ := values["cause"].(error)
	return &TaggedErrorValue{
		Tag:        f.tag,
		Message:    message,
		Name:       f.tag,
		Cause:      cause,
		Properties: values,
		factory:    f.token,
	}
}

func (f TaggedErrorFactory) Is(value any) bool {
	err, ok := value.(*TaggedErrorValue)
	return ok && f.token != nil && err.factory == f.token
}

type ErrorMatchers[T any] map[string]func(*TaggedErrorValue) T

func MatchError[T any](err *TaggedErrorValue, matchers ErrorMatchers[T]) (T, bool) {
	matcher, ok := matchers[err.Tag]
	if !ok {
		var zero T
		return zero, false
	}
	return matcher(err), true
}

type RetryPolicy struct {
	Enabled     bool
	MaxRetries  int
	BaseDelayMS int64
}

type RetryCallbacks struct {
	OnRetryScheduled    func(attempt, maxAttempts int, delayMS int64, errorMessage string) error
	OnRetryAttemptStart func() error
	OnRetryFinished     func(success bool, attempt int, finalError *string) error
}

type AgentHarnessStreamOptions struct {
	Transport       *ai.Transport
	TimeoutMS       *int64
	MaxRetries      *int
	MaxRetryDelayMS *int64
	Headers         map[string]string
	Metadata        map[string]json.RawMessage
	CacheRetention  *ai.CacheRetention
}

type AgentHarnessStreamOptionsPatch struct {
	Transport       *ai.Transport
	TimeoutMS       *int64
	MaxRetries      *int
	MaxRetryDelayMS *int64
	Headers         map[string]*string
	Metadata        map[string]ai.Optional[json.RawMessage]
	CacheRetention  *ai.CacheRetention
}

type Skill struct {
	Name                   string
	Description            string
	Content                string
	FilePath               string
	DisableModelInvocation bool
}

type PromptTemplate struct {
	Name        string
	Description string
	Content     string
}

type AgentHarnessResources[TSkill, TPromptTemplate any] struct {
	PromptTemplates []TPromptTemplate
	Skills          []TSkill
}

type Resources = AgentHarnessResources[Skill, PromptTemplate]

type FileKind string

const (
	FileKindFile      FileKind = "file"
	FileKindDirectory FileKind = "directory"
	FileKindSymlink   FileKind = "symlink"
)

type FileErrorCode string

const (
	FileErrorAborted          FileErrorCode = "aborted"
	FileErrorNotFound         FileErrorCode = "not_found"
	FileErrorPermissionDenied FileErrorCode = "permission_denied"
	FileErrorNotDirectory     FileErrorCode = "not_directory"
	FileErrorIsDirectory      FileErrorCode = "is_directory"
	FileErrorInvalid          FileErrorCode = "invalid"
	FileErrorNotSupported     FileErrorCode = "not_supported"
	FileErrorUnknown          FileErrorCode = "unknown"
)

type FileError struct {
	Code    FileErrorCode
	Message string
	Path    string
	Cause   error
}

func (e *FileError) Error() string { return e.Message }
func (e *FileError) Unwrap() error { return e.Cause }

type ExecutionErrorCode string

const (
	ExecutionErrorAborted          ExecutionErrorCode = "aborted"
	ExecutionErrorTimeout          ExecutionErrorCode = "timeout"
	ExecutionErrorShellUnavailable ExecutionErrorCode = "shell_unavailable"
	ExecutionErrorSpawn            ExecutionErrorCode = "spawn_error"
	ExecutionErrorCallback         ExecutionErrorCode = "callback_error"
	ExecutionErrorUnknown          ExecutionErrorCode = "unknown"
)

type ExecutionError struct {
	Code    ExecutionErrorCode
	Message string
	Cause   error
}

func (e *ExecutionError) Error() string { return e.Message }
func (e *ExecutionError) Unwrap() error { return e.Cause }

type CompactionErrorCode string

const (
	CompactionErrorAborted             CompactionErrorCode = "aborted"
	CompactionErrorSummarizationFailed CompactionErrorCode = "summarization_failed"
)

type CompactionError struct {
	Code    CompactionErrorCode
	Message string
	Cause   error
}

func (e *CompactionError) Error() string { return e.Message }
func (e *CompactionError) Unwrap() error { return e.Cause }

type BranchSummaryErrorCode string

const (
	BranchSummaryErrorAborted             BranchSummaryErrorCode = "aborted"
	BranchSummaryErrorSummarizationFailed BranchSummaryErrorCode = "summarization_failed"
)

type BranchSummaryError struct {
	Code    BranchSummaryErrorCode
	Message string
	Cause   error
}

func (e *BranchSummaryError) Error() string { return e.Message }
func (e *BranchSummaryError) Unwrap() error { return e.Cause }

type FileInfo struct {
	Name    string
	Path    string
	Kind    FileKind
	Size    int64
	MtimeMS int64
}

type ReadTextLinesOptions struct {
	MaxLines int
}

type CreateDirOptions struct {
	Recursive bool
}

type RemoveOptions struct {
	Recursive bool
	Force     bool
}

type CreateTempFileOptions struct {
	Prefix string
	Suffix string
}

type FileSystem interface {
	CWD() string
	AbsolutePath(context.Context, string) Result[string]
	JoinPath(context.Context, []string) Result[string]
	ReadTextFile(context.Context, string) Result[string]
	ReadTextLines(context.Context, string, ReadTextLinesOptions) Result[[]string]
	ReadBinaryFile(context.Context, string) Result[[]byte]
	WriteFile(context.Context, string, []byte) Result[struct{}]
	AppendFile(context.Context, string, []byte) Result[struct{}]
	RenameFile(context.Context, string, string) Result[struct{}]
	FileInfo(context.Context, string) Result[FileInfo]
	ListDir(context.Context, string) Result[[]FileInfo]
	CanonicalPath(context.Context, string) Result[string]
	Exists(context.Context, string) Result[bool]
	CreateDir(context.Context, string, CreateDirOptions) Result[struct{}]
	Remove(context.Context, string, RemoveOptions) Result[struct{}]
	CreateTempDir(context.Context, string) Result[string]
	CreateTempFile(context.Context, CreateTempFileOptions) Result[string]
	Cleanup(context.Context) error
}

type ShellExecOptions struct {
	CWD        string
	Env        map[string]string
	InheritEnv *bool
	Timeout    int64
	OnStdout   func(string)
	OnStderr   func(string)
}

type ShellExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Shell interface {
	Exec(context.Context, string, ShellExecOptions) Result[ShellExecResult]
	Cleanup(context.Context) error
}

type ExecutionEnv interface {
	FileSystem
	Shell
}
