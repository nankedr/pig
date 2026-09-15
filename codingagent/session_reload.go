package codingagent

import (
	"context"
	"fmt"
)

func (s *AgentSession) Reload(ctx context.Context) error {
	if s.agent == nil || s.resourceLoader == nil || s.extensionRunner != nil {
		return notImplemented("AgentSession.Reload")
	}
	if ctx == nil {
		return fmt.Errorf("reload context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.promptOptions.CWD == "" {
		s.mu.Unlock()
		return notImplemented("AgentSession.Reload")
	}
	if s.disposed || s.replacing || s.reloading || s.configurationNotifying || s.compactionCancel != nil || s.branchSummaryCancel != nil {
		s.mu.Unlock()
		return fmt.Errorf("AgentSession is busy or disposed")
	}
	s.reloading = true
	s.resourceVersion++
	options := CreateHeadlessSessionOptions{CWD: s.promptOptions.CWD, NoContextFiles: s.noContextFiles}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.reloading = false; s.mu.Unlock() }()
	if err := s.settingsManager.Reload(ctx); err != nil {
		return err
	}
	steering, err := s.settingsManager.GetSteeringMode()
	if err != nil {
		return err
	}
	followUp, err := s.settingsManager.GetFollowUpMode()
	if err != nil {
		return err
	}
	if err = s.agent.SetSteeringMode(steering); err != nil {
		return err
	}
	if err = s.agent.SetFollowUpMode(followUp); err != nil {
		return err
	}
	if loader, ok := s.resourceLoader.(*DefaultResourceLoader); ok && loader.settings != nil && loader.settings != s.settingsManager {
		if err = loader.settings.Reload(ctx); err != nil {
			return err
		}
	}
	if err = s.resourceLoader.Reload(ctx); err != nil {
		return err
	}
	return configureSessionPrompt(ctx, s, options)
}
