package gobackend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestInitializeVMLockedBoundsTopLevelScript(t *testing.T) {
	previousTimeout := extensionLifecycleTimeout
	extensionLifecycleTimeout = 10 * time.Millisecond
	t.Cleanup(func() { extensionLifecycleTimeout = previousTimeout })

	sourceDir := t.TempDir()
	indexPath := filepath.Join(sourceDir, "index.js")
	if err := os.WriteFile(indexPath, []byte("registerExtension({}); while (true) {}"), 0600); err != nil {
		t.Fatal(err)
	}
	ext := &loadedExtension{
		ID:        "lifecycle-top-level-timeout",
		Manifest:  &ExtensionManifest{Name: "lifecycle-top-level-timeout"},
		SourceDir: sourceDir,
		DataDir:   t.TempDir(),
	}

	err := initializeVMLocked(ext)
	if err == nil || !IsTimeoutError(err) {
		t.Fatalf("initialize error = %v, want timeout", err)
	}
	if ext.VM != nil || ext.runtime != nil || ext.initialized {
		t.Fatalf("timed-out VM was not discarded: VM=%v runtime=%v initialized=%v", ext.VM, ext.runtime, ext.initialized)
	}
}

func TestInitializeAndCleanupLifecycleCallbacksAreBounded(t *testing.T) {
	previousTimeout := extensionLifecycleTimeout
	extensionLifecycleTimeout = 10 * time.Millisecond
	t.Cleanup(func() { extensionLifecycleTimeout = previousTimeout })

	vm := goja.New()
	if _, err := vm.RunString(`extension = {
		initialize: function() { while (true) {} },
		cleanup: function() { while (true) {} }
	}`); err != nil {
		t.Fatal(err)
	}

	if err := initializeExtensionRuntimeWithSettings(vm, "lifecycle-callback-timeout", map[string]any{"quality": "lossless"}); err == nil || !IsTimeoutError(err) {
		t.Fatalf("initialize callback error = %v, want timeout", err)
	}
	if err := runCleanupOnVM(vm); err == nil || !IsTimeoutError(err) {
		t.Fatalf("cleanup callback error = %v, want timeout", err)
	}
}

func TestLifecycleTimeoutQuarantinesUnresponsiveCleanupVM(t *testing.T) {
	previousTimeout := extensionLifecycleTimeout
	previousGrace := jsInterruptGracePeriod
	extensionLifecycleTimeout = 10 * time.Millisecond
	jsInterruptGracePeriod = 10 * time.Millisecond
	t.Cleanup(func() {
		extensionLifecycleTimeout = previousTimeout
		jsInterruptGracePeriod = previousGrace
	})

	vm := goja.New()
	release := make(chan struct{})
	if err := vm.Set("block", func() { <-release }); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(`extension = { cleanup: function() { block(); } }`); err != nil {
		t.Fatal(err)
	}
	ext := &loadedExtension{
		ID:       "lifecycle-cleanup-quarantine",
		Manifest: &ExtensionManifest{Name: "lifecycle-cleanup-quarantine"},
		VM:       vm,
		runtime:  &extensionRuntime{},
	}

	done := make(chan error, 1)
	go func() {
		done <- func() error { ext.VMMu.Lock(); defer ext.VMMu.Unlock(); teardownVMLocked(ext); return nil }()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("teardown did not return after cleanup timeout")
	}
	ext.VMMu.Lock()
	vmRemaining, runtimeRemaining := ext.VM, ext.runtime
	ext.VMMu.Unlock()
	if vmRemaining != nil || runtimeRemaining != nil || !hasQuarantinedRuntime(ext) {
		t.Fatalf("unsafe cleanup was not quarantined: VM=%v runtime=%v quarantined=%v", vmRemaining, runtimeRemaining, hasQuarantinedRuntime(ext))
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for hasQuarantinedRuntime(ext) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hasQuarantinedRuntime(ext) {
		t.Fatal("quarantined cleanup runtime did not finish")
	}
}

func TestSignedSessionGrantRetryHonorsCancellationAndReleasesCoordinator(t *testing.T) {
	previousWait := signedSessionRetryWaitContext
	signedSessionRetryWaitContext = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { signedSessionRetryWaitContext = previousWait })

	var calls atomic.Int32
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": []string{"300"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	})
	runtime := newSignedSessionTestRuntime(t, "signed-cancel", transport)
	runtime.manifest.SignedSession = &SignedSessionConfig{
		Namespace: "signed-cancel",
		BaseURL:   "https://auth.example.com",
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- runtime.exchangeSignedSessionGrantContext(ctx, "grant-cancel") }()

	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("exchange request was not started")
	}

	coordinator, err := runtime.signedSessionCoordinator(signedSessionConfigWithDefaults(runtime.manifest.SignedSession))
	if err != nil {
		t.Fatal(err)
	}
	lockAcquired := make(chan struct{})
	go func() {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		close(lockAcquired)
	}()
	select {
	case <-lockAcquired:
	case <-time.After(time.Second):
		t.Fatal("coordinator mutex remained held during Retry-After wait")
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("exchange error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled exchange did not return")
	}

	coordinator.mu.Lock()
	inFlight := coordinator.exchangeInFlight
	coordinator.mu.Unlock()
	if inFlight {
		t.Fatal("coordinator exchange lease remained in flight after cancellation")
	}
}

func TestSignedSessionClearInvalidatesInFlightExchangeCommit(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(requestStarted)
		select {
		case <-releaseResponse:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"session_id":"late","session_secret":"late-secret","expires_at":"2099-01-01T00:00:00Z"}`,
			)),
			Request: req,
		}, nil
	})
	runtime := newSignedSessionTestRuntime(t, "signed-clear-in-flight", transport)
	runtime.manifest.SignedSession = &SignedSessionConfig{
		Namespace: "signed-clear-in-flight",
		BaseURL:   "https://auth.example.com",
	}

	errCh := make(chan error, 1)
	go func() { errCh <- runtime.exchangeSignedSessionGrant("grant-before-clear") }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("exchange request was not started")
	}

	clearResult := runtime.signedSessionClear(goja.FunctionCall{}).Export().(map[string]any)
	if clearResult["success"] != true {
		t.Fatalf("clear result = %#v, want success", clearResult)
	}
	close(releaseResponse)
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "superseded by session clear") {
			t.Fatalf("exchange error = %v, want clear-generation rejection", err)
		}
	case <-time.After(time.Second):
		t.Fatal("exchange did not finish")
	}

	config := signedSessionConfigWithDefaults(runtime.manifest.SignedSession)
	record, err := runtime.loadSignedSession(config)
	if err != nil {
		t.Fatal(err)
	}
	if record.SessionID != "" || record.SessionSecret != "" || record.ExpiresAt != "" {
		t.Fatalf("cleared session was resurrected: %#v", record)
	}
}

func TestCancelAllActiveDownloadsDoesNotPoisonIdleItems(t *testing.T) {
	activeContext, activeCancel := context.WithCancel(context.Background())
	idleContext, idleCancel := context.WithCancel(context.Background())
	t.Cleanup(idleCancel)
	registry := &cancelRegistry{entries: map[string]*cancelEntry{
		"active": {ctx: activeContext, cancel: activeCancel, refs: 1},
		"idle":   {ctx: idleContext, cancel: idleCancel, refs: 0},
	}}

	ids := registry.requestCancelActive()
	if len(ids) != 1 || ids[0] != "active" {
		t.Fatalf("cancelled IDs = %v, want [active]", ids)
	}
	if !errors.Is(activeContext.Err(), context.Canceled) {
		t.Fatalf("active context error = %v, want cancellation", activeContext.Err())
	}
	if idleContext.Err() != nil || registry.entries["idle"].canceled {
		t.Fatal("idle entry was poisoned by active cancellation")
	}
}
