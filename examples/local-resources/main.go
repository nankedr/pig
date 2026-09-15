package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "pig-local-resources-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.Setenv("HOME", filepath.Join(dir, "home")); err != nil {
		return err
	}
	cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
	builtin, err := codingagent.LoadBuiltinTheme("light")
	if err != nil {
		return err
	}
	theme, err := json.Marshal(map[string]any{"name": "review", "colors": builtin.ResolvedColors()})
	if err != nil {
		return err
	}
	for _, base := range []string{agentDir, filepath.Join(cwd, ".pig")} {
		for path, content := range map[string]string{
			"prompts/review.md":      "请审查 $1，来源：" + base,
			"skills/review/SKILL.md": "---\nname: review\ndescription: 审查变更与测试\n---\n检查用户指定文件。",
			"themes/review.json":     string(theme),
		} {
			path = filepath.Join(base, path)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				return err
			}
		}
	}
	settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
	if err != nil {
		return err
	}
	if err = settings.SetProjectTrusted(true); err != nil {
		return err
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
	if err != nil {
		return err
	}
	if err = loader.Reload(context.Background()); err != nil {
		return err
	}
	prompts, err := loader.GetPrompts()
	if err != nil {
		return err
	}
	skills, err := loader.GetSkills()
	if err != nil {
		return err
	}
	themes, err := loader.GetThemes()
	if err != nil {
		return err
	}
	for _, p := range prompts.Prompts {
		fmt.Printf("模板 /%s：%s\n", p.Name, p.Content)
	}
	fmt.Println(codingagent.FormatSkillsForPrompt(skills.Skills))
	selected, err := codingagent.SelectTheme("review", themes)
	if err != nil {
		return err
	}
	fmt.Println(selected.FG(codingagent.ThemeColorAccent, "主题效果"))
	for _, d := range append(append(prompts.Diagnostics, skills.Diagnostics...), themes.Diagnostics...) {
		if c := d.Collision; c != nil {
			fmt.Printf("%s %s：\n  获胜 %s (%s)\n  覆盖 %s (%s)\n", c.ResourceType, c.Name, c.WinnerPath, *c.WinnerSource, c.LoserPath, *c.LoserSource)
		}
	}
	return nil
}
