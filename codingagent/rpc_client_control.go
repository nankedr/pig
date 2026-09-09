package codingagent

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func (c *RPCClient) SetSteeringMode(ctx context.Context, mode agent.QueueMode) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetSteeringMode")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_steering_mode", "mode": mode})
	return err
}
func (c *RPCClient) SetFollowUpMode(ctx context.Context, mode agent.QueueMode) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetFollowUpMode")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_follow_up_mode", "mode": mode})
	return err
}
func (c *RPCClient) SetThinkingLevel(ctx context.Context, level agent.ThinkingLevel) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetThinkingLevel")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_thinking_level", "level": level})
	return err
}
func (c *RPCClient) SetAutoRetry(ctx context.Context, enabled bool) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.SetAutoRetry")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "set_auto_retry", "enabled": enabled})
	return err
}
func (c *RPCClient) AbortRetry(ctx context.Context) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.AbortRetry")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "abort_retry"})
	return err
}
func (c *RPCClient) AbortBash(ctx context.Context) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.AbortBash")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "abort_bash"})
	return err
}
func (c *RPCClient) Steer(ctx context.Context, text string, images ...[]ai.ImageContent) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.Steer")
	}
	command := map[string]any{"type": "steer", "message": text}
	if len(images) > 0 {
		command["images"] = images[0]
	}
	_, err := c.sendRPC(ctx, command)
	return err
}
func (c *RPCClient) FollowUp(ctx context.Context, text string, images ...[]ai.ImageContent) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.FollowUp")
	}
	command := map[string]any{"type": "follow_up", "message": text}
	if len(images) > 0 {
		command["images"] = images[0]
	}
	_, err := c.sendRPC(ctx, command)
	return err
}
func (c *RPCClient) SetModel(ctx context.Context, provider, modelID string) (ModelInfo, error) {
	var data ModelInfo
	if c == nil || !c.initialized {
		return data, notImplemented("RPCClient.SetModel")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "set_model", "provider": provider, "modelId": modelID})
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data, err
}
func (c *RPCClient) CycleModel(ctx context.Context) (ModelCycleResult, error) {
	var data ModelCycleResult
	if c == nil || !c.initialized {
		return data, notImplemented("RPCClient.CycleModel")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "cycle_model"})
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data, err
}
func (c *RPCClient) GetAvailableModels(ctx context.Context) ([]ModelInfo, error) {
	var data struct{ Models []ModelInfo }
	if c == nil || !c.initialized {
		return data.Models, notImplemented("RPCClient.GetAvailableModels")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_available_models"})
	if err != nil {
		return data.Models, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data.Models, err
}
func (c *RPCClient) GetAvailableThinkingLevels(ctx context.Context) ([]agent.ThinkingLevel, error) {
	var data struct{ Levels []agent.ThinkingLevel }
	if c == nil || !c.initialized {
		return data.Levels, notImplemented("RPCClient.GetAvailableThinkingLevels")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_available_thinking_levels"})
	if err != nil {
		return data.Levels, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data.Levels, err
}
func (c *RPCClient) CycleThinkingLevel(ctx context.Context) (agent.ThinkingLevel, error) {
	var data struct{ Level agent.ThinkingLevel }
	if c == nil || !c.initialized {
		return data.Level, notImplemented("RPCClient.CycleThinkingLevel")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "cycle_thinking_level"})
	if err != nil {
		return data.Level, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data.Level, err
}
func (c *RPCClient) Bash(ctx context.Context, command string) (BashResult, error) {
	var data BashResult
	if c == nil || !c.initialized {
		return data, notImplemented("RPCClient.Bash")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "bash", "command": command})
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(response.Data, &data)
	return data, err
}
func (c *RPCClient) GetSessionStats(ctx context.Context) (SessionStats, error) {
	var data SessionStats
	if c == nil || !c.initialized {
		return data, notImplemented("RPCClient.GetSessionStats")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_session_stats"})
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(response.Data, &data)
	var total struct{ Tokens struct{ Total int64 } }
	if err == nil {
		err = json.Unmarshal(response.Data, &total)
		data.Tokens.TotalTokens = total.Tokens.Total
	}
	return data, err
}
