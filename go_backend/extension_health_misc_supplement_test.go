package gobackend

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestExtensionHealthClassificationAndValidation(t *testing.T) {
	if status, msg := classifyExtensionHealthBody([]byte(`{"status":"degraded"}`), ""); status != "degraded" || msg != "degraded" {
		t.Fatalf("status/message = %q/%q", status, msg)
	}
	if status, _ := classifyExtensionHealthBody([]byte(`not-json`), ""); status != "online" {
		t.Fatalf("invalid JSON status = %q", status)
	}
	if status, msg := classifyExtensionHealthBody([]byte(`{"services":{"provider":{"status":401,"label":"Provider","detail":"auth_required"}}}`), "provider"); status != "degraded" || !strings.Contains(msg, "Provider") {
		t.Fatalf("service status/message = %q/%q", status, msg)
	}
	if status, msg, ok := classifyExtensionHealthService(map[string]any{"services": map[string]any{}}, "missing"); !ok || status != "unknown" || !strings.Contains(msg, "missing") {
		t.Fatalf("missing service = %q/%q/%v", status, msg, ok)
	}
	if n, ok := healthNumber(json.Number("503")); !ok || n != 503 {
		t.Fatalf("health number = %d/%v", n, ok)
	}
	if !isExtensionHealthAuthRequired(" unauthorized ") {
		t.Fatal("expected auth required")
	}
	if !isTransientExtensionHealthError(context.DeadlineExceeded) || !isTransientExtensionHealthError(&net.DNSError{IsTimeout: true}) {
		t.Fatal("expected timeout health errors to be transient")
	}
	if !isTransientExtensionHealthError(&net.DNSError{IsNotFound: true}) {
		t.Fatal("expected health transport lookup errors to be indeterminate")
	}

	if result := CheckExtensionHealth(nil); result.Status != "offline" {
		t.Fatalf("nil health = %#v", result)
	}
	manifest := &ExtensionManifest{Permissions: ExtensionPermissions{Network: []string{"status.example.com"}}}
	invalidURL := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "bad", URL: "://bad"})
	if invalidURL.Status != "offline" {
		t.Fatalf("invalid URL = %#v", invalidURL)
	}
	insecure := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "http", URL: "http://status.example.com"})
	if insecure.Status != "offline" || !strings.Contains(insecure.Error, "https") {
		t.Fatalf("insecure = %#v", insecure)
	}
	disallowedHost := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "host", URL: "https://other.example.com"})
	if disallowedHost.Status != "offline" || !strings.Contains(disallowedHost.Error, "permissions") {
		t.Fatalf("host = %#v", disallowedHost)
	}
	badMethod := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "method", URL: "https://status.example.com", Method: "POST"})
	if badMethod.Status != "offline" || !strings.Contains(badMethod.Error, "method") {
		t.Fatalf("method = %#v", badMethod)
	}

	ext := &loadedExtension{
		ID: "health-ext",
		Manifest: &ExtensionManifest{
			ServiceHealth: []ExtensionHealthCheck{
				{ID: "required", URL: "http://status.example.com", Required: true},
				{ID: "optional", URL: "http://status.example.com", Required: false},
			},
		},
	}
	if result := CheckExtensionHealth(ext); result.Status != "offline" || len(result.Checks) != 2 {
		t.Fatalf("extension health = %#v", result)
	}
}

func TestCoverHelpersRejectEmptyURL(t *testing.T) {
	if data, err := downloadCoverToMemory(""); err == nil || data != nil {
		t.Fatalf("expected empty cover error")
	}
}

func TestPeekExtensionHealthCachedNeverRefreshesSynchronously(t *testing.T) {
	clearExtensionHealthCache()
	ext := &loadedExtension{
		ID: "cached-health-ext",
		Manifest: &ExtensionManifest{ServiceHealth: []ExtensionHealthCheck{{
			ID: "main", URL: "https://status.example.com",
		}}},
	}
	if _, ok := PeekExtensionHealthCached(ext); ok {
		t.Fatal("unexpected cache hit")
	}

	want := ExtensionHealthResult{ExtensionID: ext.ID, Status: "degraded"}
	extensionHealthCacheMu.Lock()
	extensionHealthCache[ext.ID] = cachedExtensionHealthResult{
		result: want, expiresAt: time.Now().Add(time.Minute),
	}
	extensionHealthCacheMu.Unlock()
	got, ok := PeekExtensionHealthCached(ext)
	if !ok || got.Status != want.Status {
		t.Fatalf("cached health = %#v/%v", got, ok)
	}

	extensionHealthCacheMu.Lock()
	entry := extensionHealthCache[ext.ID]
	entry.expiresAt = time.Now().Add(-time.Second)
	extensionHealthCache[ext.ID] = entry
	extensionHealthCacheMu.Unlock()
	if _, ok := PeekExtensionHealthCached(ext); ok {
		t.Fatal("expired cache entry was returned")
	}
	if stale, ok := peekExtensionHealthStale(ext); !ok || stale.Status != want.Status {
		t.Fatalf("stale health snapshot = %#v/%v", stale, ok)
	}
}
