package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

func (c *cli) catalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		return c.catalogSnapshot(ctx, "", false)
	case "show":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return usageError("用法: netproxyctl catalog show <分组>")
		}
		return c.catalogSnapshot(ctx, args[1], true)
	default:
		return usageError("用法: netproxyctl catalog list|show")
	}
}

func (c *cli) catalogSnapshot(ctx context.Context, group string, withNodes bool) error {
	options := c.options
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{
		Root: options.CatalogRoot, ProgressDir: options.ProgressDir,
		Type: "all", WithNodes: withNodes, GroupID: group,
	})
	if err != nil {
		return err
	}
	if withNodes {
		if len(groups) == 0 {
			return fmt.Errorf("Catalog 分组不存在: %s", group)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.show", Message: "Catalog 分组快照", Data: groups[0]})
	} else {
		summaries := make([]catalog.GroupSummary, 0, len(groups))
		for _, entry := range groups {
			summaries = append(summaries, entry.Group)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.groups", Message: "Catalog 分组快照", Data: summaries})
	}
	return nil
}
