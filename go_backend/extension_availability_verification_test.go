package gobackend

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestAvailabilityPreservesCanonicalVerificationOnFailure(t *testing.T) {
	for _, script := range []string{
		`throw new Error("VERIFY_REQUIRED");`,
		`return null;`,
		`return undefined;`,
		`return {available:false};`,
	} {
		t.Run(script, func(t *testing.T) {
			ext := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
			t.Cleanup(func() { teardownExtension(ext) })
			if err := ext.ensureRuntimeReady(); err != nil {
				t.Fatal(err)
			}
			if err := ext.VM.Set("requireChallenge", func(goja.FunctionCall) goja.Value {
				ext.runtime.noteVerificationRequired("https://example.test/challenge")
				return goja.Undefined()
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := ext.VM.RunString(`extension.checkAvailability = function(){ requireChallenge(); ` + script + ` };`); err != nil {
				t.Fatal(err)
			}
			_, err := newExtensionProviderWrapper(ext).CheckAvailabilityForItemID("", "Song", "Artist", "", "", "", "", 180000, "")
			if err == nil || classifyDownloadErrorType(err.Error()) != "verification_required" || !strings.Contains(err.Error(), ext.ID) {
				t.Fatalf("canonical verification not preserved: %v", err)
			}
			if ext.runtime.consumeVerificationRequired() != "" {
				t.Fatal("verification evidence leaked into the next call")
			}
		})
	}
}

func TestAvailabilityDoesNotPromoteUntrustedOrStaleVerification(t *testing.T) {
	ext := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
	t.Cleanup(func() { teardownExtension(ext) })
	if err := ext.ensureRuntimeReady(); err != nil {
		t.Fatal(err)
	}
	if _, err := ext.VM.RunString(`extension.checkAvailability = function(){ throw new Error("VERIFY_REQUIRED"); };`); err != nil {
		t.Fatal(err)
	}
	ext.runtime.noteVerificationRequired("https://example.test/stale-challenge")
	_, err := newExtensionProviderWrapper(ext).CheckAvailabilityForItemID("", "Song", "Artist", "", "", "", "", 180000, "")
	if err == nil || classifyDownloadErrorType(err.Error()) == "verification_required" {
		t.Fatalf("untrusted exception inherited a previous challenge: %v", err)
	}
}

func TestExtensionVerificationErrorsRequireOwnedPendingChallenge(t *testing.T) {
	for _, tc := range []struct {
		name          string
		message       string
		pendingOwner  string
		challengeAge  time.Duration
		authURL       string
		wantChallenge bool
	}{
		{"fresh", "VERIFY_REQUIRED", "coverage-ext", 0, "https://example.test/verify", true},
		{"missing", "VERIFY_REQUIRED", "", 0, "", false},
		{"other-provider", "VERIFY_REQUIRED", "other-provider", 0, "https://example.test/verify", false},
		{"expired", "VERIFY_REQUIRED", "coverage-ext", pendingAuthRequestTTL + time.Second, "https://example.test/verify", false},
		{"future", "VERIFY_REQUIRED", "coverage-ext", -time.Minute, "https://example.test/verify", false},
		{"missing-url", "VERIFY_REQUIRED", "coverage-ext", 0, "", false},
		{"network-error", "network timeout", "coverage-ext", 0, "https://example.test/verify", false},
		{"provider-auth", "PROVIDER_AUTH_FAILED: VERIFY_REQUIRED", "coverage-ext", 0, "https://example.test/verify", false},
		{"http-status", "HTTP 401: VERIFY_REQUIRED", "coverage-ext", 0, "https://example.test/verify", false},
		{"cancelled", "cancelled: VERIFY_REQUIRED", "coverage-ext", 0, "https://example.test/verify", false},
		{"throwing-getter", "ordinary failure", "coverage-ext", 0, "https://example.test/verify", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext := newTestLoadedExtension(t, ExtensionTypeMetadataProvider, ExtensionTypeDownloadProvider)
			t.Cleanup(func() {
				ClearPendingAuthRequest(ext.ID)
				ClearPendingAuthRequest(tc.pendingOwner)
				teardownExtension(ext)
			})
			if err := ext.ensureRuntimeReady(); err != nil {
				t.Fatal(err)
			}
			if err := ext.VM.Set("fixtureMessage", tc.message); err != nil {
				t.Fatal(err)
			}
			if _, err := ext.VM.RunString(`extension.searchTracks = extension.checkAvailability = function() { throw new Error(fixtureMessage); };`); err != nil {
				t.Fatal(err)
			}
			if tc.name == "throwing-getter" {
				if _, err := ext.VM.RunString(`extension.searchTracks = extension.checkAvailability = function() {
					throw {toString: function() { return "ordinary failure"; }, get message() { throw new Error("broken getter"); }};
				};`); err != nil {
					t.Fatal(err)
				}
			}
			if tc.pendingOwner != "" {
				if err := registerPendingAuthRequest(&PendingAuthRequest{
					ExtensionID: tc.pendingOwner,
					AuthURL:     tc.authURL,
					CreatedAt:   time.Now().Add(-tc.challengeAge),
				}); err != nil {
					t.Fatal(err)
				}
			}
			provider := newExtensionProviderWrapper(ext)
			_, searchErr := provider.SearchTracks("Song Artist", 1)
			_, availabilityErr := provider.CheckAvailabilityForItemID("", "Song", "Artist", "", "", "", "", 180000, "")
			for _, err := range []error{searchErr, availabilityErr} {
				if err == nil {
					t.Fatal("extension error was lost")
				}
				if got := classifyDownloadErrorType(err.Error()) == "verification_required"; got != tc.wantChallenge {
					t.Fatalf("verification=%v, want %v: %v", got, tc.wantChallenge, err)
				}
			}
			if tc.pendingOwner == "" && GetPendingAuthRequest(ext.ID) != nil {
				t.Fatal("error classification created a challenge")
			}
		})
	}
}
