package catalog

import (
	"context"
	"path/filepath"
	"time"
)

type testImportOptions struct {
	Root          string
	GroupID       string
	Name          string
	Input         string
	AllowInsecure bool
	Now           time.Time
}

func importTestGroup(options testImportOptions) (MutationResult, error) {
	err := InitializeGroup(context.Background(), GroupOptions{
		Root: options.Root, GroupID: options.GroupID, Name: options.Name, Type: "local", Now: options.Now,
	})
	if err != nil {
		return MutationResult{}, err
	}
	result, err := AppendNode(context.Background(), MutationOptions{
		GroupDir: filepath.Join(options.Root, options.GroupID),
		GroupID:  options.GroupID, Name: options.Name, Type: "local", Input: options.Input,
		AllowInsecure: options.AllowInsecure, Now: options.Now,
	})
	if err != nil {
		_ = DeleteGroup(options.Root, options.GroupID)
	}
	return result, err
}
