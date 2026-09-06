package gobackend

import (
	"strings"
	"testing"

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
