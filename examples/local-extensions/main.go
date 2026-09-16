package main

import (
	"context"
	"errors"
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
	root, err := os.MkdirTemp("", "pig-local-extensions-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	dir := filepath.Join(root, "agent")
	if err = os.MkdirAll(filepath.Join(dir, "extensions"), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "extensions/example.js"), []byte(`throw Error("must never be imported")`), 0600); err != nil {
		return err
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: root, AgentDir: dir, NoContextFiles: true})
	if err != nil {
		return err
	}
	if err = loader.Reload(context.Background()); err != nil {
		return err
	}
	result, err := loader.GetExtensionDiscovery()
	if err != nil {
		return err
	}
	for _, d := range result.Diagnostics {
		fmt.Println(d.Message)
	}
	_, err = loader.GetExtensions()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		return fmt.Errorf("expected explicit execution stub, got %v", err)
	}
	fmt.Println(err)
	return nil
}
