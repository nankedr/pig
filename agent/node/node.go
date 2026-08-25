package node

import (
	"context"

	"github.com/nankedr/pig/agent"
)

type NodeExecutionEnv struct {
	workingDirectory string
}

func NewNodeExecutionEnv(cwd string) *NodeExecutionEnv {
	return &NodeExecutionEnv{workingDirectory: cwd}
}

func (e *NodeExecutionEnv) CWD() string { return e.workingDirectory }

func deferred[T any](operation string) agent.Result[T] {
	return agent.Err[T](&agent.NotImplementedError{Module: "agent/node", Operation: "NodeExecutionEnv." + operation})
}

func (*NodeExecutionEnv) AbsolutePath(context.Context, string) agent.Result[string] {
	return deferred[string]("AbsolutePath")
}

func (*NodeExecutionEnv) JoinPath(context.Context, []string) agent.Result[string] {
	return deferred[string]("JoinPath")
}

func (*NodeExecutionEnv) ReadTextFile(context.Context, string) agent.Result[string] {
	return deferred[string]("ReadTextFile")
}

func (*NodeExecutionEnv) ReadTextLines(context.Context, string, agent.ReadTextLinesOptions) agent.Result[[]string] {
	return deferred[[]string]("ReadTextLines")
}

func (*NodeExecutionEnv) ReadBinaryFile(context.Context, string) agent.Result[[]byte] {
	return deferred[[]byte]("ReadBinaryFile")
}

func (*NodeExecutionEnv) WriteFile(context.Context, string, []byte) agent.Result[struct{}] {
	return deferred[struct{}]("WriteFile")
}

func (*NodeExecutionEnv) AppendFile(context.Context, string, []byte) agent.Result[struct{}] {
	return deferred[struct{}]("AppendFile")
}

func (*NodeExecutionEnv) RenameFile(context.Context, string, string) agent.Result[struct{}] {
	return deferred[struct{}]("RenameFile")
}

func (*NodeExecutionEnv) FileInfo(context.Context, string) agent.Result[agent.FileInfo] {
	return deferred[agent.FileInfo]("FileInfo")
}

func (*NodeExecutionEnv) ListDir(context.Context, string) agent.Result[[]agent.FileInfo] {
	return deferred[[]agent.FileInfo]("ListDir")
}

func (*NodeExecutionEnv) CanonicalPath(context.Context, string) agent.Result[string] {
	return deferred[string]("CanonicalPath")
}

func (*NodeExecutionEnv) Exists(context.Context, string) agent.Result[bool] {
	return deferred[bool]("Exists")
}

func (*NodeExecutionEnv) CreateDir(context.Context, string, agent.CreateDirOptions) agent.Result[struct{}] {
	return deferred[struct{}]("CreateDir")
}

func (*NodeExecutionEnv) Remove(context.Context, string, agent.RemoveOptions) agent.Result[struct{}] {
	return deferred[struct{}]("Remove")
}

func (*NodeExecutionEnv) CreateTempDir(context.Context, string) agent.Result[string] {
	return deferred[string]("CreateTempDir")
}

func (*NodeExecutionEnv) CreateTempFile(context.Context, agent.CreateTempFileOptions) agent.Result[string] {
	return deferred[string]("CreateTempFile")
}

func (*NodeExecutionEnv) Exec(context.Context, string, agent.ShellExecOptions) agent.Result[agent.ShellExecResult] {
	return deferred[agent.ShellExecResult]("Exec")
}

func (*NodeExecutionEnv) Cleanup(context.Context) error {
	return &agent.NotImplementedError{Module: "agent/node", Operation: "NodeExecutionEnv.Cleanup"}
}

var _ agent.ExecutionEnv = (*NodeExecutionEnv)(nil)
