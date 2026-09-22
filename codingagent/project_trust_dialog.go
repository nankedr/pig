package codingagent

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/tui"
	"path/filepath"
)

type ProjectTrustOption struct {
	Label   string
	Trusted bool
	Updates []ProjectTrustUpdate
}

func GetProjectTrustOptions(cwd string, sessionOnly bool) ([]ProjectTrustOption, error) {
	path, err := canonicalTrustPath(cwd)
	if err != nil {
		return nil, err
	}
	options := []ProjectTrustOption{{Label: "Trust", Trusted: true, Updates: []ProjectTrustUpdate{{Path: path, Decision: ProjectTrustDecisionTrusted()}}}}
	if parent := filepath.Dir(path); parent != path {
		options = append(options, ProjectTrustOption{Label: "Trust parent folder (" + parent + ")", Trusted: true, Updates: []ProjectTrustUpdate{{Path: parent, Decision: ProjectTrustDecisionTrusted()}, {Path: path}}})
	}
	if sessionOnly {
		options = append(options, ProjectTrustOption{Label: "Trust (this session only)", Trusted: true})
	}
	options = append(options, ProjectTrustOption{Label: "Do not trust", Updates: []ProjectTrustUpdate{{Path: path, Decision: ProjectTrustDecisionUntrusted()}}})
	if sessionOnly {
		options = append(options, ProjectTrustOption{Label: "Do not trust (this session only)"})
	}
	return options, nil
}

type PrepareProjectSettingsOptions struct {
	CWD             string
	AgentDir        string
	SettingsManager *SettingsManager
	Override        ProjectTrustDecision
	// Terminal enables interactive selection for an unknown project under ask policy.
	Terminal tui.Terminal
}

// PrepareProjectSettings resolves and persists trust before reading project settings.
func PrepareProjectSettings(ctx context.Context, o PrepareProjectSettingsOptions) error {
	var selectDialog func(context.Context, string, []tui.SelectItem) (*tui.SelectItem, error)
	if o.Terminal != nil {
		selectDialog = func(ctx context.Context, title string, items []tui.SelectItem) (*tui.SelectItem, error) {
			return tui.ShowSelectDialog(ctx, o.Terminal, title, items)
		}
	}
	return prepareProjectSettings(ctx, o, selectDialog)
}
func prepareProjectSettings(ctx context.Context, o PrepareProjectSettingsOptions, selectDialog func(context.Context, string, []tui.SelectItem) (*tui.SelectItem, error)) error {
	if ctx == nil {
		return fmt.Errorf("trust context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.SettingsManager == nil {
		return fmt.Errorf("project settings manager is required")
	}
	if o.Override != nil {
		return o.SettingsManager.SetProjectTrusted(*o.Override)
	}
	resources, err := HasTrustRequiringProjectResources(ctx, o.CWD)
	if err != nil {
		return err
	}
	if !resources {
		return o.SettingsManager.SetProjectTrusted(true)
	}
	if o.AgentDir == "" {
		o.AgentDir, err = GetAgentDir()
		if err != nil {
			return err
		}
	}
	store := NewProjectTrustStore(o.AgentDir)
	decision, err := store.Get(ctx, o.CWD)
	if err != nil {
		return err
	}
	if decision != nil {
		return o.SettingsManager.SetProjectTrusted(*decision)
	}
	global, err := o.SettingsManager.GetGlobalSettings()
	if err != nil {
		return err
	}
	policy := DefaultProjectTrustAsk
	if global.DefaultProjectTrust != nil {
		policy = *global.DefaultProjectTrust
	}
	if policy == DefaultProjectTrustAlways || policy == DefaultProjectTrustNever || selectDialog == nil {
		return o.SettingsManager.SetProjectTrusted(policy == DefaultProjectTrustAlways)
	}
	options, err := GetProjectTrustOptions(o.CWD, true)
	if err != nil {
		return err
	}
	items := make([]tui.SelectItem, len(options))
	for i, option := range options {
		items[i] = tui.SelectItem{Value: option.Label, Label: option.Label}
	}
	title := "Trust project folder?\n" + tui.SafeTerminalText(o.CWD) + "\n\nTrust allows Pig to load .pig settings and project resources. Trusted content and Tools use your host user permissions. This is not Tool approval or a sandbox. Context Files load even without trust. Package installation and extension execution remain unavailable."
	selected, err := selectDialog(ctx, title, items)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	trusted := false
	if selected != nil {
		for _, option := range options {
			if option.Label == selected.Value {
				if len(option.Updates) > 0 {
					if err = store.SetMany(ctx, option.Updates); err != nil {
						return fmt.Errorf("save project trust: %w", err)
					}
				}
				trusted = option.Trusted
				break
			}
		}
	}
	return o.SettingsManager.SetProjectTrusted(trusted)
}
