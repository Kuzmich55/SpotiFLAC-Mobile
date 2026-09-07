package gobackend

import (
	"runtime"
	"testing"
	"time"
	"weak"
)

func TestDiscardedPooledRuntimeCanBeCollected(t *testing.T) {
	ext := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
	discarded := func() weak.Pointer[extensionRuntime] {
		vm, rt, err := acquireIsolatedExtensionRuntime(ext)
		if err != nil {
			t.Fatal(err)
		}
		if err := vm.Set("temporaryDownloadBytes", vm.NewArrayBuffer(make([]byte, 16<<20))); err != nil {
			t.Fatal(err)
		}
		releaseIsolatedExtensionRuntime(ext, vm, rt, true, true, nil)
		vm, rt, err = acquireIsolatedExtensionRuntime(ext)
		if err != nil {
			t.Fatal(err)
		}
		pointer := weak.Make(rt)
		// An operation error retires the borrowed VM instead of pooling it.
		releaseIsolatedExtensionRuntime(ext, vm, rt, false, true, nil)
		return pointer
	}()
	runtime.GC()
	if discarded.Value() != nil {
		t.Fatal("retired runtime and its download buffer remain reachable from the idle pool")
	}
	runtime.KeepAlive(ext)
}

func TestReleaseMemoryUnderPressureClearsDisposableCaches(t *testing.T) {
	clearCoverMemoryCache()
	coverCachePut("https://example.com/cover.jpg", []byte("cover"))
	globalLyricsCache.ClearAll()
	globalLyricsCache.Set("artist", "track", 120, &LyricsResponse{PlainLyrics: "lyrics"})

	privateIPCacheMu.Lock()
	privateIPCache["example.com"] = privateIPCacheEntry{expiresAt: time.Now().Add(time.Hour)}
	privateIPCacheMu.Unlock()
	extensionHealthCacheMu.Lock()
	extensionHealthCache["extension"] = cachedExtensionHealthResult{expiresAt: time.Now().Add(time.Hour)}
	extensionHealthCacheMu.Unlock()

	ReleaseMemoryUnderPressure()

	coverMu.Lock()
	coverEntries, coverBytes := len(coverCache), coverCacheBytes
	coverMu.Unlock()
	if coverEntries != 0 || coverBytes != 0 {
		t.Fatalf("cover cache retained %d entries/%d bytes", coverEntries, coverBytes)
	}
	if globalLyricsCache.Size() != 0 {
		t.Fatalf("lyrics cache retained %d entries", globalLyricsCache.Size())
	}
	privateIPCacheMu.RLock()
	privateEntries := len(privateIPCache)
	privateIPCacheMu.RUnlock()
	if privateEntries != 0 {
		t.Fatalf("private IP cache retained %d entries", privateEntries)
	}
	extensionHealthCacheMu.Lock()
	healthEntries := len(extensionHealthCache)
	extensionHealthCacheMu.Unlock()
	if healthEntries != 0 {
		t.Fatalf("health cache retained %d entries", healthEntries)
	}
}
