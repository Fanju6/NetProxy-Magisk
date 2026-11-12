package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"os"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func writeEffectiveConfig(ctx context.Context, singBoxDir, destination string, inbounds []jsontext.Value, providers []jsontext.Value) error {
	content, err := os.ReadFile(paths.SingBoxConfig(singBoxDir))
	if err != nil {
		return err
	}
	document, err := configObject(content)
	if err != nil {
		return err
	}
	if err := validateManagedProviderTags(document); err != nil {
		return err
	}
	if err := validateConfiguredCatalogProviders(document, providers); err != nil {
		return err
	}
	for key, entries := range map[string][]jsontext.Value{
		"inbounds": inbounds, "providers": providers,
	} {
		if err := appendConfigEntries(document, key, entries); err != nil {
			return err
		}
	}
	content, err = json.Marshal(document, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeConfigAtomic(destination, append(content, '\n'), 0o600)
}

// Append only managed arrays; keep unrelated native JSON and array ordering intact.
func appendConfigEntries(document map[string]jsontext.Value, key string, managed []jsontext.Value) error {
	if len(managed) == 0 {
		return nil
	}
	var entries []jsontext.Value
	if raw, ok := document[key]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
	}
	content, err := json.Marshal(append(entries, managed...))
	if err != nil {
		return err
	}
	document[key] = content
	return nil
}
