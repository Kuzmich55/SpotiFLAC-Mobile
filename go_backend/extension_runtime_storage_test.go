package gobackend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func setStorageValue(t *testing.T, runtime *extensionRuntime, key string, value any) {
	t.Helper()
	result := runtime.storageSet(goja.FunctionCall{
		Arguments: []goja.Value{
			runtime.vm.ToValue(key),
			runtime.vm.ToValue(value),
		},
	})
	if !result.ToBoolean() {
		t.Fatalf("storage.set(%q) returned false", key)
	}
}

func TestExtensionJSONCacheUsesIdentityAndIsolatesSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")
	if err := os.WriteFile(path, []byte(`{"value":"first"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loads := 0
	load := func() (map[string]any, error) {
		loads++
		return readJSONMapFile(path)
	}

	mu := extensionFileMu(path)
	mu.Lock()
	first, err := readCachedJSONMapLocked(path, load)
	mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	first["value"] = "mutated locally"

	mu.Lock()
	second, err := readCachedJSONMapLocked(path, load)
	mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || second["value"] != "first" {
		t.Fatalf("cache did not isolate/reuse snapshot: loads=%d data=%#v", loads, second)
	}

	if err := os.WriteFile(path, []byte(`{"value":"external update"}`), 0600); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	third, err := readCachedJSONMapLocked(path, load)
	mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if loads != 2 || third["value"] != "external update" {
		t.Fatalf("external update was not reloaded: loads=%d data=%#v", loads, third)
	}
}

func TestExtensionStorageKeyReadIsolatedAndFresh(t *testing.T) {
	dataDir := t.TempDir()
	ext := &loadedExtension{ID: "key-read", Manifest: &ExtensionManifest{Name: "key-read"}, DataDir: dataDir}
	a := newExtensionRuntime(ext)
	b := newExtensionRuntime(ext)
	a.RegisterAPIs(goja.New())
	b.RegisterAPIs(goja.New())
	setStorageValue(t, a, "nested", map[string]any{"items": []any{map[string]any{"value": "original"}}})
	read := func(r *extensionRuntime, key string) goja.Value {
		return r.storageGet(goja.FunctionCall{Arguments: []goja.Value{r.vm.ToValue(key)}})
	}
	value := read(a, "nested").ToObject(a.vm)
	items := value.Get("items").ToObject(a.vm)
	if err := items.Get("0").ToObject(a.vm).Set("value", "local mutation"); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []*extensionRuntime{a, b} {
		got := read(runtime, "nested").ToObject(runtime.vm).Get("items").ToObject(runtime.vm).Get("0").ToObject(runtime.vm).Get("value").String()
		if got != "original" {
			t.Fatalf("VM mutation leaked: %q", got)
		}
	}
	setStorageValue(t, b, "nested", "updated")
	if got := read(a, "nested").String(); got != "updated" {
		t.Fatalf("cross-runtime update not visible: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "storage.json"), []byte(`{"nested":"external replacement","nullValue":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := read(a, "nested").String(); got != "external replacement" {
		t.Fatalf("external replacement not visible: %q", got)
	}
	if !goja.IsNull(read(a, "nullValue")) || !goja.IsUndefined(read(a, "absent")) {
		t.Fatal("null and absent values must remain distinct")
	}
	if err := os.Remove(filepath.Join(dataDir, "storage.json")); err != nil {
		t.Fatal(err)
	}
	if !goja.IsUndefined(read(a, "nested")) {
		t.Fatal("removed storage file reused stale value")
	}
}

func BenchmarkExtensionCachedSingleKeyRead(b *testing.B) {
	for _, size := range []int{10, 10000} {
		b.Run(fmt.Sprintf("entries_%d", size), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "storage.json")
			snapshot := map[string]any{"token": "value"}
			for i := 0; i < size; i++ {
				snapshot[fmt.Sprint(i)] = map[string]any{"items": []any{"large cached value", i}}
			}
			data, _ := json.Marshal(snapshot)
			if err := os.WriteFile(path, data, 0600); err != nil {
				b.Fatal(err)
			}
			load := func() (map[string]any, error) { return readJSONMapFile(path) }
			mu := extensionFileMu(path)
			mu.Lock()
			defer mu.Unlock()
			if _, _, err := readCachedJSONValueLocked(path, "token", load); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := readCachedJSONValueLocked(path, "token", load); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestExtensionRuntimeStorageConcurrentRuntimesMergeWrites(t *testing.T) {
	dataDir := t.TempDir()
	ext := &loadedExtension{ID: "merge-test", Manifest: &ExtensionManifest{Name: "merge-test"}, DataDir: dataDir}
	runtimeA := newExtensionRuntime(ext)
	runtimeB := newExtensionRuntime(ext)
	runtimeA.RegisterAPIs(goja.New())
	runtimeB.RegisterAPIs(goja.New())

	start := make(chan struct{})
	done := make(chan bool, 2)
	go func() {
		<-start
		result := runtimeA.storageSet(goja.FunctionCall{Arguments: []goja.Value{
			runtimeA.vm.ToValue("from_a"), runtimeA.vm.ToValue("a"),
		}})
		done <- result.ToBoolean()
	}()
	go func() {
		<-start
		result := runtimeB.storageSet(goja.FunctionCall{Arguments: []goja.Value{
			runtimeB.vm.ToValue("from_b"), runtimeB.vm.ToValue("b"),
		}})
		done <- result.ToBoolean()
	}()
	close(start)
	if !<-done || !<-done {
		t.Fatal("concurrent storage write failed")
	}

	storage := readStorageMap(t, filepath.Join(dataDir, "storage.json"))
	if storage["from_a"] != "a" || storage["from_b"] != "b" {
		t.Fatalf("concurrent storage writes were not merged: %#v", storage)
	}

	credStart := make(chan struct{})
	credDone := make(chan struct{}, 2)
	for _, item := range []struct {
		runtime *extensionRuntime
		key     string
	}{
		{runtimeA, "token_a"},
		{runtimeB, "token_b"},
	} {
		item := item
		go func() {
			<-credStart
			result := item.runtime.credentialsStore(goja.FunctionCall{Arguments: []goja.Value{
				item.runtime.vm.ToValue(item.key),
				item.runtime.vm.ToValue(item.key + "_value"),
			}})
			if success, _ := result.Export().(map[string]any)["success"].(bool); !success {
				t.Errorf("credentialsStore(%s) failed", item.key)
			}
			credDone <- struct{}{}
		}()
	}
	close(credStart)
	<-credDone
	<-credDone

	reader := newExtensionRuntime(ext)
	reader.RegisterAPIs(goja.New())
	for _, key := range []string{"token_a", "token_b"} {
		got := reader.credentialsGet(goja.FunctionCall{Arguments: []goja.Value{reader.vm.ToValue(key)}}).String()
		if got != key+"_value" {
			t.Fatalf("credential %s = %q", key, got)
		}
	}
}

func readStorageMap(t *testing.T, storagePath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("failed to read storage file: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal storage file: %v", err)
	}
	return parsed
}

func TestExtensionRuntimeStorage_AtomicWriteCompactJSON(t *testing.T) {
	ext := &loadedExtension{
		ID: "storage-test",
		Manifest: &ExtensionManifest{
			Name: "storage-test",
		},
		DataDir: t.TempDir(),
	}

	runtime := newExtensionRuntime(ext)
	runtime.RegisterAPIs(goja.New())

	setStorageValue(t, runtime, "k1", "v1")
	setStorageValue(t, runtime, "k2", 2)

	storagePath := filepath.Join(ext.DataDir, "storage.json")
	deadline := time.Now().Add(1500 * time.Millisecond)

	var raw []byte
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(storagePath)
		if err == nil {
			raw = data
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(raw) == 0 {
		t.Fatalf("storage.json was not written within timeout")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal storage file: %v", err)
	}
	if parsed["k1"] != "v1" {
		t.Fatalf("expected k1=v1, got %v", parsed["k1"])
	}
	if parsed["k2"] != float64(2) {
		t.Fatalf("expected k2=2, got %v", parsed["k2"])
	}
	if bytes.Contains(raw, []byte("\n")) {
		t.Fatalf("expected compact JSON without indentation, got: %q", string(raw))
	}
}

func TestUnloadExtension_FlushesPendingStorage(t *testing.T) {
	ext := &loadedExtension{
		ID: "unload-storage-test",
		Manifest: &ExtensionManifest{
			Name: "unload-storage-test",
		},
		DataDir: t.TempDir(),
		VM:      goja.New(),
	}

	runtime := newExtensionRuntime(ext)
	runtime.RegisterAPIs(ext.VM)
	ext.runtime = runtime

	manager := &extensionManager{
		extensions: map[string]*loadedExtension{
			ext.ID: ext,
		},
	}

	setStorageValue(t, runtime, "persist_on_unload", true)

	if err := manager.UnloadExtension(ext.ID); err != nil {
		t.Fatalf("UnloadExtension failed: %v", err)
	}

	storagePath := filepath.Join(ext.DataDir, "storage.json")
	parsed := readStorageMap(t, storagePath)
	if parsed["persist_on_unload"] != true {
		t.Fatalf("expected pending storage value to be flushed on unload, got %v", parsed["persist_on_unload"])
	}
}
