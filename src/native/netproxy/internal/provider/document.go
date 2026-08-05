package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/sagernet/sing-box/option"
	providerparser "github.com/sagernet/sing-box/provider/parser"
	SJSON "github.com/sagernet/sing/common/json"
)

type Document struct {
	Outbounds []option.Outbound `json:"outbounds"`
	Endpoints []option.Endpoint `json:"endpoints,omitempty"`
}

type Diagnostic struct {
	Index   int    `json:"index,omitempty"`
	Source  string `json:"source,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ParseResult struct {
	Document    Document     `json:"-"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type NodeSummary struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Server   string `json:"server,omitempty"`
	Port     uint16 `json:"port,omitempty"`
}

func ParseDocument(ctx context.Context, content []byte) (Document, error) {
	ctx = Context(ctx)
	outbounds, endpoints, err := providerparser.ParseBoxSubscription(ctx, string(content))
	if err != nil {
		return Document{}, err
	}
	document := Document{Outbounds: outbounds, Endpoints: endpoints}
	if err := Validate(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func Load(ctx context.Context, path string) (Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	return ParseDocument(ctx, content)
}

func LoadAllowEmpty(ctx context.Context, path string) (Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(content, &shape); err != nil {
		return Document{}, err
	}
	if len(shape) == 0 || len(shape) > 2 {
		return ParseDocument(ctx, content)
	}
	for key := range shape {
		if key != "outbounds" && key != "endpoints" {
			return ParseDocument(ctx, content)
		}
	}
	var outbounds, endpoints []json.RawMessage
	if raw := shape["outbounds"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &outbounds); err != nil {
			return Document{}, err
		}
	}
	if raw := shape["endpoints"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &endpoints); err != nil {
			return Document{}, err
		}
	}
	if len(outbounds) == 0 && len(endpoints) == 0 {
		return Document{}, nil
	}
	return ParseDocument(ctx, content)
}

func Marshal(ctx context.Context, document Document) ([]byte, error) {
	if err := Validate(document); err != nil {
		return nil, err
	}
	return marshalDocument(ctx, document)
}

func MarshalAllowEmpty(ctx context.Context, document Document) ([]byte, error) {
	if len(document.Outbounds)+len(document.Endpoints) > 0 {
		if err := Validate(document); err != nil {
			return nil, err
		}
	}
	return marshalDocument(ctx, document)
}

func marshalDocument(ctx context.Context, document Document) ([]byte, error) {
	content, err := SJSON.MarshalContext(Context(ctx), document)
	if err != nil {
		return nil, err
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, content, "", "  "); err != nil {
		return nil, err
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}

func SaveAtomic(ctx context.Context, path string, document Document) error {
	content, err := Marshal(ctx, document)
	if err != nil {
		return err
	}
	return WriteAtomic(path, content, 0o600)
}

func SaveAtomicAllowEmpty(ctx context.Context, path string, document Document) error {
	content, err := MarshalAllowEmpty(ctx, document)
	if err != nil {
		return err
	}
	return WriteAtomic(path, content, 0o600)
}

func WriteAtomic(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".netproxy-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if runtime.GOOS != "windows" || !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(temporaryPath, path); err != nil {
			return err
		}
	}
	committed = true
	return nil
}

func Validate(document Document) error {
	if len(document.Outbounds) == 0 && len(document.Endpoints) == 0 {
		return errors.New("provider does not contain any nodes")
	}
	seen := make(map[string]struct{}, len(document.Outbounds)+len(document.Endpoints))
	for index, outbound := range document.Outbounds {
		if err := validateTag(outbound.Tag, seen); err != nil {
			return fmt.Errorf("outbound[%d]: %w", index, err)
		}
		if outbound.Type == "" || outbound.Options == nil {
			return fmt.Errorf("outbound[%d]: missing protocol options", index)
		}
		if serverOptions, ok := outbound.Options.(option.ServerOptionsWrapper); ok {
			server := serverOptions.TakeServerOptions()
			if strings.TrimSpace(server.Server) == "" {
				return fmt.Errorf("outbound[%d] %q: missing server", index, outbound.Tag)
			}
			if server.ServerPort == 0 {
				return fmt.Errorf("outbound[%d] %q: invalid server port", index, outbound.Tag)
			}
		}
	}
	for index, endpoint := range document.Endpoints {
		if err := validateTag(endpoint.Tag, seen); err != nil {
			return fmt.Errorf("endpoint[%d]: %w", index, err)
		}
		if endpoint.Type == "" || endpoint.Options == nil {
			return fmt.Errorf("endpoint[%d]: missing protocol options", index)
		}
	}
	return nil
}

func validateTag(tag string, seen map[string]struct{}) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("missing tag")
	}
	if _, exists := seen[tag]; exists {
		return fmt.Errorf("duplicate tag %q", tag)
	}
	seen[tag] = struct{}{}
	return nil
}

func NormalizeTags(document *Document) {
	used := make(map[string]int, len(document.Outbounds)+len(document.Endpoints))
	unique := func(tag, fallback string) string {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			tag = fallback
		}
		used[tag]++
		if used[tag] == 1 {
			return tag
		}
		base := tag
		for suffix := used[tag]; ; suffix++ {
			candidate := fmt.Sprintf("%s_%d", base, suffix)
			if used[candidate] == 0 {
				used[candidate] = 1
				return candidate
			}
		}
	}
	for index := range document.Outbounds {
		document.Outbounds[index].Tag = unique(document.Outbounds[index].Tag, document.Outbounds[index].Type)
	}
	for index := range document.Endpoints {
		document.Endpoints[index].Tag = unique(document.Endpoints[index].Tag, document.Endpoints[index].Type)
	}
}

func Filter(document Document, includePattern, excludePattern string) (Document, error) {
	var include, exclude *regexp.Regexp
	var err error
	if includePattern != "" {
		include, err = regexp.Compile(includePattern)
		if err != nil {
			return Document{}, fmt.Errorf("invalid include expression: %w", err)
		}
	}
	if excludePattern != "" {
		exclude, err = regexp.Compile(excludePattern)
		if err != nil {
			return Document{}, fmt.Errorf("invalid exclude expression: %w", err)
		}
	}
	keep := func(tag string) bool {
		return (include == nil || include.MatchString(tag)) && (exclude == nil || !exclude.MatchString(tag))
	}
	filtered := Document{}
	for _, outbound := range document.Outbounds {
		if keep(outbound.Tag) {
			filtered.Outbounds = append(filtered.Outbounds, outbound)
		}
	}
	for _, endpoint := range document.Endpoints {
		if keep(endpoint.Tag) {
			filtered.Endpoints = append(filtered.Endpoints, endpoint)
		}
	}
	if len(filtered.Outbounds) == 0 && len(filtered.Endpoints) == 0 {
		return Document{}, errors.New("all nodes were removed by filters")
	}
	return filtered, nil
}

func Append(target *Document, source Document) {
	target.Outbounds = append(target.Outbounds, source.Outbounds...)
	target.Endpoints = append(target.Endpoints, source.Endpoints...)
	NormalizeTags(target)
}

func Remove(target *Document, tag string) bool {
	removed := false
	outbounds := target.Outbounds[:0]
	for _, outbound := range target.Outbounds {
		if outbound.Tag == tag {
			removed = true
			continue
		}
		outbounds = append(outbounds, outbound)
	}
	target.Outbounds = outbounds
	endpoints := target.Endpoints[:0]
	for _, endpoint := range target.Endpoints {
		if endpoint.Tag == tag {
			removed = true
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	target.Endpoints = endpoints
	return removed
}

func Select(document Document, tag string) (Document, bool) {
	selected := Document{}
	for _, outbound := range document.Outbounds {
		if outbound.Tag == tag {
			selected.Outbounds = append(selected.Outbounds, outbound)
		}
	}
	for _, endpoint := range document.Endpoints {
		if endpoint.Tag == tag {
			selected.Endpoints = append(selected.Endpoints, endpoint)
		}
	}
	return selected, len(selected.Outbounds)+len(selected.Endpoints) > 0
}

func Inspect(document Document) []NodeSummary {
	summaries := make([]NodeSummary, 0, len(document.Outbounds)+len(document.Endpoints))
	for _, outbound := range document.Outbounds {
		summary := NodeSummary{Tag: outbound.Tag, Protocol: outbound.Type}
		if serverOptions, ok := outbound.Options.(option.ServerOptionsWrapper); ok {
			server := serverOptions.TakeServerOptions()
			summary.Server = displayServer(server.Server)
			summary.Port = server.ServerPort
		}
		summaries = append(summaries, summary)
	}
	for _, endpoint := range document.Endpoints {
		summaries = append(summaries, NodeSummary{Tag: endpoint.Tag, Protocol: endpoint.Type})
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].Tag < summaries[j].Tag })
	return summaries
}

func displayServer(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	if address := net.ParseIP(server); address != nil && address.To4() != nil {
		parts := strings.Split(server, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".*.*"
		}
	}
	if address := net.ParseIP(server); address != nil && address.To4() == nil {
		parts := strings.Split(server, ":")
		if len(parts) > 2 {
			return strings.Join(parts[:2], ":") + ":*"
		}
	}
	labels := strings.Split(server, ".")
	if len(labels) > 2 {
		return "*." + strings.Join(labels[len(labels)-2:], ".")
	}
	return server
}
