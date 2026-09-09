package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
)

func rpcData[T any](ctx context.Context, c *RPCClient, method string, command map[string]any) (data T, err error) {
	if c == nil || !c.initialized {
		return data, notImplemented("RPCClient." + method)
	}
	response, err := c.sendRPC(ctx, command)
	if err == nil {
		err = json.Unmarshal(response.Data, &data)
	}
	return data, err
}

func (c *RPCClient) NewSession(ctx context.Context, parent ...string) (bool, error) {
	command := map[string]any{"type": "new_session"}
	if len(parent) > 0 {
		command["parentSession"] = parent[0]
	}
	data, err := rpcData[struct{ Cancelled bool }](ctx, c, "NewSession", command)
	return data.Cancelled, err
}
func (c *RPCClient) SwitchSession(ctx context.Context, path string) (bool, error) {
	data, err := rpcData[struct{ Cancelled bool }](ctx, c, "SwitchSession", map[string]any{"type": "switch_session", "sessionPath": path})
	return data.Cancelled, err
}
func (c *RPCClient) Clone(ctx context.Context) (bool, error) {
	data, err := rpcData[struct{ Cancelled bool }](ctx, c, "Clone", map[string]any{"type": "clone"})
	return data.Cancelled, err
}
func (c *RPCClient) Fork(ctx context.Context, id string) (string, bool, error) {
	data, err := rpcData[struct {
		Text      string
		Cancelled bool
	}](ctx, c, "Fork", map[string]any{"type": "fork", "entryId": id})
	return data.Text, data.Cancelled, err
}
func (c *RPCClient) Compact(ctx context.Context, instructions ...string) (CompactionResult, error) {
	command := map[string]any{"type": "compact"}
	if len(instructions) > 0 {
		command["customInstructions"] = instructions[0]
	}
	return rpcData[CompactionResult](ctx, c, "Compact", command)
}
func (c *RPCClient) SetAutoCompaction(ctx context.Context, enabled bool) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetAutoCompaction")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_auto_compaction", "enabled": enabled})
	return err
}
func (c *RPCClient) SetSessionName(ctx context.Context, name string) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetSessionName")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_session_name", "name": name})
	return err
}
func (c *RPCClient) GetForkMessages(ctx context.Context) ([]ForkMessage, error) {
	data, err := rpcData[struct{ Messages []ForkMessage }](ctx, c, "GetForkMessages", map[string]any{"type": "get_fork_messages"})
	return data.Messages, err
}
func (c *RPCClient) GetEntries(ctx context.Context, since ...string) ([]SessionEntry, *string, error) {
	command := map[string]any{"type": "get_entries"}
	if len(since) > 0 {
		command["since"] = since[0]
	}
	data, err := rpcData[struct {
		Entries []json.RawMessage
		LeafID  *string
	}](ctx, c, "GetEntries", command)
	if err != nil {
		return nil, nil, err
	}
	entries := make([]SessionEntry, len(data.Entries))
	for i, raw := range data.Entries {
		entries[i], err = decodeSessionEntry(raw)
		if err != nil {
			return nil, nil, err
		}
	}
	return entries, data.LeafID, nil
}
func (c *RPCClient) GetTree(ctx context.Context) ([]SessionTreeNode, *string, error) {
	data, err := rpcData[struct {
		Tree   []rpcTreeNode
		LeafID *string
	}](ctx, c, "GetTree", map[string]any{"type": "get_tree"})
	if err != nil {
		return nil, nil, err
	}
	tree, err := decodeRPCTree(data.Tree)
	return tree, data.LeafID, err
}

type rpcTreeNode struct {
	Entry          json.RawMessage `json:"entry"`
	Children       []rpcTreeNode   `json:"children"`
	Label          *string         `json:"label,omitempty"`
	LabelTimestamp *string         `json:"labelTimestamp,omitempty"`
}

func encodeRPCTree(tree []SessionTreeNode) ([]rpcTreeNode, error) {
	result := make([]rpcTreeNode, len(tree))
	for i, node := range tree {
		entry, err := marshalSessionEntry(node.Entry)
		if err != nil {
			return nil, err
		}
		children, err := encodeRPCTree(node.Children)
		if err != nil {
			return nil, err
		}
		result[i] = rpcTreeNode{entry, children, node.Label, node.LabelTimestamp}
	}
	return result, nil
}
func decodeRPCTree(tree []rpcTreeNode) ([]SessionTreeNode, error) {
	result := make([]SessionTreeNode, len(tree))
	for i, node := range tree {
		entry, err := decodeSessionEntry(node.Entry)
		if err != nil {
			return nil, err
		}
		children, err := decodeRPCTree(node.Children)
		if err != nil {
			return nil, err
		}
		result[i] = SessionTreeNode{entry, children, node.Label, node.LabelTimestamp}
	}
	return result, nil
}

func (c *RPCClient) ExportHTML(ctx context.Context, output ...string) (string, error) {
	command := map[string]any{"type": "export_html"}
	if len(output) > 0 {
		command["outputPath"] = output[0]
	}
	data, err := rpcData[struct{ Path string }](ctx, c, "ExportHTML", command)
	if err == nil && data.Path == "" {
		err = fmt.Errorf("export_html response is missing path")
	}
	return data.Path, err
}
