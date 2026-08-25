package catalog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const catalogHelperEnv = "NETPROXY_CATALOG_HELPER"

func TestCatalogProcessHelper(t *testing.T) {
	operation := os.Getenv(catalogHelperEnv)
	if operation == "" {
		return
	}
	root := os.Getenv("NETPROXY_CATALOG_ROOT")
	groupID := os.Getenv("NETPROXY_CATALOG_GROUP")
	ctx := context.Background()

	switch operation {
	case "append":
		_, err := AppendNode(ctx, MutationOptions{
			GroupDir: filepath.Join(root, groupID), GroupID: groupID,
			Type: "local", Input: os.Getenv("NETPROXY_CATALOG_INPUT"),
		})
		if err != nil {
			t.Fatal(err)
		}
	case "edit":
		_, err := EditNode(ctx, MutationOptions{
			GroupDir: filepath.Join(root, groupID), GroupID: groupID,
			Tag: os.Getenv("NETPROXY_CATALOG_TAG"), Input: os.Getenv("NETPROXY_CATALOG_INPUT"),
		})
		if err != nil {
			t.Fatal(err)
		}
	case "remove":
		_, err := RemoveNode(ctx, MutationOptions{
			GroupDir: filepath.Join(root, groupID), GroupID: groupID,
			Tag: os.Getenv("NETPROXY_CATALOG_TAG"),
		})
		if err != nil {
			t.Fatal(err)
		}
	case "update":
		if err := helperMetadataUpdate(ctx, root, groupID); err != nil {
			t.Fatal(err)
		}
	case "hold-root":
		release, err := AcquireRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		writeCatalogSignal(t, os.Getenv("NETPROXY_CATALOG_READY"))
		waitCatalogSignal(t, os.Getenv("NETPROXY_CATALOG_CONTINUE"))
	case "blocked-commit":
		transactionRenameHook = func(point string) {
			if point != "move-provider.json" {
				return
			}
			writeCatalogSignal(t, os.Getenv("NETPROXY_CATALOG_READY"))
			waitCatalogSignal(t, os.Getenv("NETPROXY_CATALOG_CONTINUE"))
		}
		defer func() { transactionRenameHook = func(string) {} }()
		providerContent, metadataContent := helperTransactionContents(t, groupID)
		if err := CommitPair(root, filepath.Join(root, groupID), providerContent, metadataContent); err != nil {
			t.Fatal(err)
		}
	case "recover":
		writeCatalogSignal(t, os.Getenv("NETPROXY_CATALOG_ATTEMPT"))
		if err := Recover(root); err != nil {
			t.Fatal(err)
		}
	case "crash":
		point := os.Getenv("NETPROXY_CATALOG_POINT")
		transactionRenameHook = func(current string) {
			if current == point {
				panic("simulated interruption at " + current)
			}
		}
		if err := CommitPair(root, filepath.Join(root, groupID), []byte("new-provider"), []byte("new-meta")); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown Catalog helper operation %q", operation)
	}
}

func TestCatalogMultiProcessMixedMutations(t *testing.T) {
	root := t.TempDir()
	groupID := "mixed"
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: groupID, Name: "mixed", Input: "socks://base.example:1080#BASE\n" +
			"socks://edit.example:1081#EDIT_ME\nsocks://remove.example:1082#REMOVE_ME",
	}); err != nil {
		t.Fatal(err)
	}

	var commands []*exec.Cmd
	for index := range 4 {
		commands = append(commands, startCatalogHelper(t, root, groupID, "append", map[string]string{
			"NETPROXY_CATALOG_INPUT": fmt.Sprintf("socks://append-%d.example:1080#APPEND_%d", index, index),
		}))
	}
	commands = append(commands,
		startCatalogHelper(t, root, groupID, "edit", map[string]string{
			"NETPROXY_CATALOG_TAG":   "EDIT_ME",
			"NETPROXY_CATALOG_INPUT": "socks://edited.example:1083#EDITED",
		}),
		startCatalogHelper(t, root, groupID, "remove", map[string]string{
			"NETPROXY_CATALOG_TAG": "REMOVE_ME",
		}),
		startCatalogHelper(t, root, groupID, "update", nil),
	)
	waitCatalogCommands(t, commands)

	document, err := provider.Load(context.Background(), filepath.Join(root, groupID, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := provider.Inspect(document)
	if len(nodes) != 6 || !hasCatalogTag(nodes, "BASE") || !hasCatalogTag(nodes, "EDITED") || hasCatalogTag(nodes, "REMOVE_ME") {
		t.Fatalf("mixed mutations lost or duplicated nodes: %+v", nodes)
	}
	metadata, err := LoadMetadata(filepath.Join(root, groupID, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.NodeCount != len(nodes) || metadata.Revision != int64(len(commands)+1) {
		t.Fatalf("mixed mutations diverged: nodes=%d metadata=%+v", len(nodes), metadata)
	}
}

func TestCatalogMultiProcessSameGroupWrites(t *testing.T) {
	root := t.TempDir()
	groupID := "same-group"
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: groupID, Name: groupID, Input: "socks://base.example:1080#BASE",
	}); err != nil {
		t.Fatal(err)
	}
	commands := make([]*exec.Cmd, 0, 12)
	for index := range 12 {
		commands = append(commands, startCatalogHelper(t, root, groupID, "append", map[string]string{
			"NETPROXY_CATALOG_INPUT": fmt.Sprintf("socks://same-%d.example:1080#NODE_%d", index, index),
		}))
	}
	waitCatalogCommands(t, commands)

	document, err := provider.Load(context.Background(), filepath.Join(root, groupID, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := provider.Inspect(document)
	metadata, err := LoadMetadata(filepath.Join(root, groupID, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 13 || metadata.NodeCount != 13 || metadata.Revision != 13 {
		t.Fatalf("same-group writes diverged: nodes=%d metadata=%+v", len(nodes), metadata)
	}
}

func TestCatalogRecoverWaitsForActiveTransaction(t *testing.T) {
	root := t.TempDir()
	groupID := "recover-active"
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: groupID, Name: groupID, Input: "socks://old.example:1080#OLD",
	}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "blocked.ready")
	continuePath := filepath.Join(root, "blocked.continue")
	active := startCatalogHelper(t, root, groupID, "blocked-commit", map[string]string{
		"NETPROXY_CATALOG_READY":    ready,
		"NETPROXY_CATALOG_CONTINUE": continuePath,
	})
	waitForCatalogSignal(t, ready)

	attempt := filepath.Join(root, "recover.attempt")
	recoverCommand := startCatalogHelper(t, root, groupID, "recover", map[string]string{
		"NETPROXY_CATALOG_ATTEMPT": attempt,
	})
	waitForCatalogSignal(t, attempt)
	recoverDone := commandDone(recoverCommand)
	select {
	case err := <-recoverDone:
		t.Fatalf("Recover ran while another process held the Catalog lock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	writeCatalogSignal(t, continuePath)
	if err := waitCatalogCommand(active); err != nil {
		t.Fatalf("active transaction: %v", err)
	}
	if err := waitCatalogResult(recoverDone, 10*time.Second); err != nil {
		t.Fatalf("recover transaction: %v", err)
	}

	document, err := provider.Load(context.Background(), filepath.Join(root, groupID, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCatalogTag(provider.Inspect(document), "CHANGED") {
		t.Fatalf("active transaction was not preserved: %+v", provider.Inspect(document))
	}
	if entries, err := os.ReadDir(filepath.Join(root, stagingDirName)); err == nil && len(entries) != 0 {
		t.Fatalf("active transaction staging was not cleaned after commit: %v", entries)
	}
}

func TestCatalogRecoverAfterEachRenameInterruption(t *testing.T) {
	for _, point := range []string{"move-provider.json", "move-meta.json", "install-provider.json", "install-meta.json"} {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			groupID := "rename-" + strings.ReplaceAll(point, "-", "")
			groupDir := filepath.Join(root, groupID)
			if _, err := ImportGroup(context.Background(), ImportOptions{
				Root: root, GroupID: groupID, Name: groupID, Input: "socks://old.example:1080#OLD",
			}); err != nil {
				t.Fatal(err)
			}
			command := startCatalogHelper(t, root, groupID, "crash", map[string]string{
				"NETPROXY_CATALOG_POINT": point,
			})
			if err := waitCatalogCommand(command); err == nil {
				t.Fatal("crash helper unexpectedly succeeded")
			}
			if err := Recover(root); err != nil {
				t.Fatalf("recover after %s: %v", point, err)
			}
			document, err := provider.Load(context.Background(), filepath.Join(groupDir, "provider.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !hasCatalogTag(provider.Inspect(document), "OLD") {
				t.Fatalf("old provider was not restored after %s", point)
			}
			if entries, err := os.ReadDir(filepath.Join(root, stagingDirName)); err == nil && len(entries) != 0 {
				t.Fatalf("staging was not cleaned after %s: %v", point, entries)
			}
		})
	}
}

func TestCatalogStaleOwnerAndPIDReuseDoNotBypassFileLock(t *testing.T) {
	root := t.TempDir()
	path, err := catalogLockPath(root, "root")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("pid="+fmt.Sprint(os.Getpid())+"\nprocess=reused\ncreated_at=2000-01-01T00:00:00Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireRoot(root)
	if err != nil {
		t.Fatalf("stale owner metadata blocked a live lock: %v", err)
	}
	release()

	ready := filepath.Join(root, "hold.ready")
	continuePath := filepath.Join(root, "hold.continue")
	holder := startCatalogHelper(t, root, "default", "hold-root", map[string]string{
		"NETPROXY_CATALOG_READY":    ready,
		"NETPROXY_CATALOG_CONTINUE": continuePath,
	})
	waitForCatalogSignal(t, ready)
	acquired := make(chan error, 1)
	go func() {
		release, err := AcquireRoot(root)
		if err == nil {
			release()
		}
		acquired <- err
	}()
	select {
	case err := <-acquired:
		t.Fatalf("Catalog lock was not exclusive: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	writeCatalogSignal(t, continuePath)
	if err := waitCatalogCommand(holder); err != nil {
		t.Fatalf("lock holder: %v", err)
	}
	if err := waitCatalogResult(acquired, 10*time.Second); err != nil {
		t.Fatalf("lock competition: %v", err)
	}
}

func helperMetadataUpdate(ctx context.Context, root, groupID string) error {
	release, err := acquireCatalogMutation(root, groupID)
	if err != nil {
		return err
	}
	defer release()
	groupDir := filepath.Join(root, groupID)
	document, err := provider.LoadAllowEmpty(ctx, filepath.Join(groupDir, "provider.json"))
	if err != nil {
		return err
	}
	metadata, err := LoadMetadataLocked(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		return err
	}
	metadata.Revision++
	metadata.NodeCount = len(document.Outbounds) + len(document.Endpoints)
	metadata.UpdatedAt = FormatEpochUTC(time.Now().Unix())
	providerContent, err := provider.MarshalAllowEmpty(ctx, document)
	if err != nil {
		return err
	}
	metadataContent, err := json.Marshal(metadata, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	metadataContent = append(metadataContent, '\n')
	return commitPairLocked(root, groupDir, providerContent, metadataContent)
}

func helperTransactionContents(t *testing.T, groupID string) ([]byte, []byte) {
	t.Helper()
	parsed, err := convert.Input(context.Background(), "socks://changed.example:1080#CHANGED", false)
	if err != nil {
		t.Fatal(err)
	}
	providerContent, err := provider.MarshalAllowEmpty(context.Background(), parsed.Document)
	if err != nil {
		t.Fatal(err)
	}
	metadata := NewMetadata(groupID, groupID, "local", "", time.Now())
	metadata.NodeCount = 1
	metadata.Revision = 99
	metadataContent, err := json.Marshal(metadata, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	return providerContent, append(metadataContent, '\n')
}

func startCatalogHelper(t *testing.T, root, groupID, operation string, values map[string]string) *exec.Cmd {
	t.Helper()
	values = cloneCatalogHelperValues(values)
	values[catalogHelperEnv] = operation
	values["NETPROXY_CATALOG_ROOT"] = root
	values["NETPROXY_CATALOG_GROUP"] = groupID
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replace := values[key]; replace {
				continue
			}
		}
		env = append(env, entry)
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	command := exec.Command(os.Args[0], "-test.run=TestCatalogProcessHelper", "--")
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start Catalog helper %s: %v", operation, err)
	}
	return command
}

func cloneCatalogHelperValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values)+3)
	maps.Copy(cloned, values)
	return cloned
}

func waitCatalogCommands(t *testing.T, commands []*exec.Cmd) {
	t.Helper()
	for _, command := range commands {
		if err := waitCatalogCommand(command); err != nil {
			t.Fatalf("Catalog helper failed: %v", err)
		}
	}
}

func waitCatalogCommand(command *exec.Cmd) error {
	return waitCatalogResult(commandDone(command), 15*time.Second)
}

func commandDone(command *exec.Cmd) <-chan error {
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return done
}

func waitCatalogResult(done <-chan error, timeout time.Duration) error {
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return errors.New("Catalog helper timeout")
	}
}

func waitForCatalogSignal(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for Catalog helper signal %s", path)
}

func writeCatalogSignal(t *testing.T, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		t.Fatal("empty Catalog helper signal path")
	}
	if err := os.WriteFile(path, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitCatalogSignal(t *testing.T, path string) {
	waitForCatalogSignal(t, path)
}

func hasCatalogTag(nodes []provider.NodeSummary, tag string) bool {
	for _, node := range nodes {
		if node.Tag == tag {
			return true
		}
	}
	return false
}
