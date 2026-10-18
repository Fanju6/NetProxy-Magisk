package module

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

// Local resource groups only own Catalog metadata and Provider content.
func CreateLocalGroup(ctx context.Context, options Options, id, name string) error {
	if err := validateResourceName(name); err != nil {
		return err
	}
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	return catalog.InitializeGroup(ctx, catalog.GroupOptions{Root: options.CatalogRoot, GroupID: id, Name: strings.TrimSpace(name), Type: "local"})
}

func RenameLocalGroup(ctx context.Context, options Options, id, name string) error {
	if err := validateResourceName(name); err != nil {
		return err
	}
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	if err := requireLocalGroup(ctx, options, id); err != nil {
		return err
	}
	return catalog.SetGroupName(ctx, options.CatalogRoot, id, name, time.Now())
}

func RemoveLocalGroup(ctx context.Context, options Options, id string) (err error) {
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	if err := requireLocalGroup(ctx, options, id); err != nil {
		return err
	}
	_, err = removeCatalogResource(ctx, options, id, "node")
	return err
}

func requireLocalGroup(ctx context.Context, options Options, id string) error {
	typ, err := catalog.GroupType(ctx, options.CatalogRoot, id)
	if err != nil {
		return err
	}
	if typ != "local" {
		return &service.Error{Code: "catalog.read_only", Message: "该操作仅适用于本地资源分组"}
	}
	return nil
}

func validateResourceName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 256 || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return &service.Error{Code: "catalog.invalid", Message: "资源名称必须为 1 至 256 字节的有效文本，不能包含控制字符"}
	}
	return nil
}
