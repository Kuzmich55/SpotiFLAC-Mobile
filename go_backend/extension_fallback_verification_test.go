package gobackend

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFallbackKeepsPendingVerificationOwnership(t *testing.T) {
	for _, mode := range []string{"fresh-response", "saved-challenge", "network-error", "download-error", "empty-download-error"} {
		t.Run(mode, func(t *testing.T) {
			primary := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
			primary.ID, primary.Manifest.Name = "primary-provider", "primary-provider"
			secondary := newTestLoadedExtension(t, ExtensionTypeMetadataProvider, ExtensionTypeDownloadProvider)
			secondary.ID, secondary.Manifest.Name = "secondary-provider", "secondary-provider"
			secondary.Manifest.Permissions.Storage = true
			secondary.Manifest.SignedSession = &SignedSessionConfig{
				Namespace: "fixture-session", BaseURL: "https://auth.example.test",
			}
			last := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
			last.ID, last.Manifest.Name = "last-provider", "last-provider"
			for ext, script := range map[*loadedExtension]string{
				primary: `registerExtension({
					checkAvailability: function() { return {available: true, track_id: "source-track"}; },
					download: function() { return {success: false, error_type: "api_error", error_message: "primary request failed", retry_after_seconds: 37}; }
				});`,
				secondary: `var savedChallenge;
				function queryCatalog() {
					var response = session.signedFetch("GET", "/catalog");
					if (response.needsVerification) {
						savedChallenge = new Error("VERIFY_REQUIRED");
						throw savedChallenge;
					}
					throw new Error("fixture expected a challenge");
				}
				registerExtension({
					searchTracks: queryCatalog,
					checkAvailability: function() {
						if (fixtureMode === "network-error") throw new Error("lookup network timeout");
						if (fixtureMode === "download-error" || fixtureMode === "empty-download-error") return {available: true, track_id: "matched-track"};
						if (fixtureMode === "saved-challenge") throw savedChallenge;
						return queryCatalog();
					},
					download: function() {
						if (fixtureMode === "download-error") throw new Error("download network timeout");
						if (fixtureMode === "empty-download-error") return {success: false};
						throw new Error("verification must finish before download");
					}
				});`,
				last: `registerExtension({checkAvailability: function() { recordLastProvider(); return {available: false}; }});`,
			} {
				if err := os.WriteFile(filepath.Join(ext.SourceDir, "index.js"), fmt.Appendf(nil, "var fixtureMode = %q;\n%s", mode, script), 0600); err != nil {
					t.Fatal(err)
				}
			}
			manager := getExtensionManager()
			manager.mu.Lock()
			previousExtensions := manager.extensions
			manager.extensions = map[string]*loadedExtension{primary.ID: primary, secondary.ID: secondary, last.ID: last}
			manager.mu.Unlock()
			previousPriority, previousFallback := GetProviderPriority(), GetExtensionFallbackProviderIDs()
			SetProviderPriority([]string{secondary.ID, last.ID, primary.ID})
			SetExtensionFallbackProviderIDs(nil)
			t.Cleanup(func() {
				for _, ext := range []*loadedExtension{primary, secondary, last} {
					ClearPendingAuthRequest(ext.ID)
					teardownExtension(ext)
				}
				manager.mu.Lock()
				manager.extensions = previousExtensions
				manager.mu.Unlock()
				SetProviderPriority(previousPriority)
				SetExtensionFallbackProviderIDs(previousFallback)
				resetPreparedDownloadRequestCacheForTest()
			})
			if err := secondary.ensureRuntimeReady(); err != nil {
				t.Fatal(err)
			}
			var bootstrapCalls atomic.Int32
			secondary.runtime.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				bootstrapCalls.Add(1)
				return &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header), Request: req,
					Body: io.NopCloser(strings.NewReader(`{"auth_url":"https://auth.example.test/verify"}`)),
				}, nil
			})}
			expectsVerification := mode == "fresh-response" || mode == "saved-challenge"
			if expectsVerification {
				_, err := newExtensionProviderWrapper(secondary).SearchTracks("Song Artist", 1)
				if err == nil || GetPendingAuthRequest(secondary.ID) == nil {
					t.Fatalf("metadata lookup did not create a pending challenge: %v", err)
				}
			}
			if err := last.ensureRuntimeReady(); err != nil {
				t.Fatal(err)
			}
			var laterCalls atomic.Int32
			if err := last.VM.Set("recordLastProvider", func() { laterCalls.Add(1) }); err != nil {
				t.Fatal(err)
			}
			response, err := DownloadWithExtensionFallback(DownloadRequest{
				Service: primary.ID, Source: primary.ID, ItemID: "fallback-" + mode,
				SpotifyID: "source-track", TrackName: "Song", ArtistName: "Artist", AlbumName: "Album",
				ISRC: "USABC2600001", ReleaseDate: "2026-01-01", Quality: "lossless",
				OutputDir: t.TempDir(), FilenameFormat: "{title}", UseFallback: true,
			})
			if err != nil || response == nil {
				t.Fatalf("fallback response=%+v error=%v", response, err)
			}
			wantType := "verification_required"
			switch mode {
			case "network-error":
				wantType = "network"
			case "download-error":
				wantType = "script_error"
			case "empty-download-error":
				wantType = "extension_error"
			}
			if response.ErrorType != wantType || response.Service != secondary.ID || response.RetryAfterSeconds != 0 {
				t.Fatalf("failure lost its type, owner, or retry delay: %+v", response)
			}
			if expectsVerification {
				if laterCalls.Load() != 0 || bootstrapCalls.Load() != 1 {
					t.Fatalf("pending challenge was skipped or recreated: later=%d bootstrap=%d", laterCalls.Load(), bootstrapCalls.Load())
				}
			} else if laterCalls.Load() != 1 {
				t.Fatal("ordinary lookup failures must allow the next provider")
			}
		})
	}
}
