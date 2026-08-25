package agent

import "context"

type JSONLSessionRepoFileSystem interface {
	AbsolutePath(context.Context, string) Result[string]
	JoinPath(context.Context, []string) Result[string]
	ReadTextFile(context.Context, string) Result[string]
	ReadTextLines(context.Context, string, ReadTextLinesOptions) Result[[]string]
	WriteFile(context.Context, string, []byte) Result[struct{}]
	AppendFile(context.Context, string, []byte) Result[struct{}]
	RenameFile(context.Context, string, string) Result[struct{}]
	FileInfo(context.Context, string) Result[FileInfo]
	ListDir(context.Context, string) Result[[]FileInfo]
	Exists(context.Context, string) Result[bool]
	CreateDir(context.Context, string, CreateDirOptions) Result[struct{}]
	Remove(context.Context, string, RemoveOptions) Result[struct{}]
}

type JSONLSessionRepoOptions struct {
	FS           JSONLSessionRepoFileSystem
	SessionsRoot string
}

type JSONLSessionMetadata struct {
	SessionMetadata
	CWD                     string
	Path                    string
	ModifiedAt              int64
	SourceFormat            int
	LegacyParentSessionPath string
	LegacyParentPathSet     bool
	Metadata                map[string]JSONValue
}

type JSONLSessionCreateOptions struct {
	SessionCreateOptions
	CWD      string
	Metadata map[string]JSONValue
}

type JSONLSessionListOptions struct {
	CWD string
}

type JSONLV4Header struct {
	Kind                    string
	Version                 int
	ID                      string
	CreatedAt               int64
	CWD                     string
	ParentSessionID         string
	ParentSessionIDSet      bool
	LegacyParentSessionPath string
	LegacyParentPathSet     bool
	Metadata                map[string]JSONValue
}

type JSONLSessionRepo struct {
	options JSONLSessionRepoOptions
}

func NewJSONLSessionRepo(options JSONLSessionRepoOptions) *JSONLSessionRepo {
	return &JSONLSessionRepo{options: options}
}

func (r *JSONLSessionRepo) Create(context.Context, JSONLSessionCreateOptions) (*Session, error) {
	return nil, newNotImplemented("JSONLSessionRepo.Create")
}

func (r *JSONLSessionRepo) Open(context.Context, JSONLSessionMetadata) (*Session, error) {
	return nil, newNotImplemented("JSONLSessionRepo.Open")
}

func (r *JSONLSessionRepo) List(context.Context, JSONLSessionListOptions) ([]JSONLSessionMetadata, error) {
	return nil, newNotImplemented("JSONLSessionRepo.List")
}

func (r *JSONLSessionRepo) Delete(context.Context, JSONLSessionMetadata) error {
	return newNotImplemented("JSONLSessionRepo.Delete")
}

func (r *JSONLSessionRepo) Fork(context.Context, JSONLSessionMetadata, ForkOptions, JSONLSessionCreateOptions) (*Session, error) {
	return nil, newNotImplemented("JSONLSessionRepo.Fork")
}
