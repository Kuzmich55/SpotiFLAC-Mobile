package gobackend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func runResolutionScript(t *testing.T, r *extensionRuntime, parent context.Context, allowance time.Duration, script string) (goja.Value, error) {
	t.Helper()
	ctx, finish := r.beginResolutionBudget(parent, allowance)
	defer finish()
	return RunWithTimeoutContextAndRecover(ctx, r.vm, script, 3*time.Second)
}

func TestResolutionBudgetInterruptsBlockedOperationsAsTimeout(t *testing.T) {
	for _, operation := range []string{"http", "sleep", "signed-session", "busy-script"} {
		t.Run(operation, func(t *testing.T) {
			r := newFileDownloadTestRuntime(t, func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			})
			r.vm.Set("blockedHTTP", func() goja.Value { return r.doExtensionHTTP("GET", "https://cdn.example.com/api", nil, false, nil) })
			r.vm.Set("sleep", r.sleep)
			r.vm.Set("signedSessionWait", func() goja.Value {
				ctx, cancel := r.signedSessionExchangeContext()
				defer cancel()
				<-ctx.Done()
				return r.vm.ToValue(false)
			})
			script := map[string]string{
				"http":           "blockedHTTP(); true",
				"sleep":          "sleep(300000); true",
				"signed-session": "signedSessionWait(); true",
				"busy-script":    "while (true) {}",
			}[operation]
			started := time.Now()
			_, err := runResolutionScript(t, r, context.Background(), 50*time.Millisecond, script)
			if !IsTimeoutError(err) || errors.Is(err, ErrExtensionRequestCancelled) || IsRuntimeUnsafeError(err) {
				t.Fatalf("expected safe timeout, got %v", err)
			}
			if time.Since(started) > time.Second {
				t.Fatal("blocked operation did not stop promptly")
			}
			// The interrupt was cleared and the operation context was detached.
			value, err := runResolutionScript(t, r, context.Background(), time.Second, "42")
			if err != nil || value.ToInteger() != 42 {
				t.Fatalf("reuse: %v, %v", value, err)
			}
		})
	}
}

type resolutionSlowBody struct {
	ctx   context.Context
	reads int
}

func (b *resolutionSlowBody) Read(p []byte) (int, error) {
	if b.reads == 3 {
		return 0, io.EOF
	}
	if b.reads > 0 {
		select {
		case <-time.After(90 * time.Millisecond):
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		}
	}
	p[0] = 'a'
	b.reads++
	return 1, nil
}
func (*resolutionSlowBody) Close() error { return nil }

func TestResolutionBudgetAllowsActiveNativeTransfers(t *testing.T) {
	for _, kind := range []string{"plain", "chunked", "segments"} {
		t.Run(kind, func(t *testing.T) {
			r := newFileDownloadTestRuntime(t, func(req *http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("Content-Length", "3")
				status := http.StatusOK
				if req.Header.Get("Range") != "" {
					status = http.StatusPartialContent
					header.Set("Content-Range", "bytes 0-2/3")
				}
				var body io.ReadCloser = &resolutionSlowBody{ctx: req.Context()}
				if req.Method == "HEAD" {
					body = io.NopCloser(strings.NewReader(""))
				}
				return &http.Response{StatusCode: status, Header: header, Body: body, ContentLength: 3, Request: req}, nil
			})
			r.vm.Set("download", r.fileDownload)
			r.vm.Set("segments", r.fileDownloadSegments)
			script := `download("https://cdn.example.com/audio", "audio.flac")`
			if kind == "chunked" {
				script = `download("https://cdn.example.com/audio", "audio.flac", {chunked:true})`
			}
			if kind == "segments" {
				script = `segments(["https://cdn.example.com/audio"], "audio.flac")`
			}
			started := time.Now()
			value, err := runResolutionScript(t, r, context.Background(), 70*time.Millisecond, script)
			if err != nil {
				t.Fatal(err)
			}
			if result := value.Export().(map[string]any); result["success"] != true {
				t.Fatalf("transfer failed: %#v", result)
			}
			if time.Since(started) < 180*time.Millisecond {
				t.Fatal("transfer did not exceed resolution allowance")
			}
			data, err := os.ReadFile(filepath.Join(r.dataDir, "audio.flac"))
			if err != nil || string(data) != "aaa" {
				t.Fatalf("output: %q, %v", data, err)
			}
		})
	}
}

func TestResolutionBudgetDoesNotResetAcrossTransfersAndRefresh(t *testing.T) {
	r := newFileDownloadTestRuntime(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &resolutionSlowBody{ctx: req.Context()}, ContentLength: 3, Request: req}, nil
	})
	r.vm.Set("download", r.fileDownload)
	r.vm.Set("sleep", r.sleep)
	_, err := runResolutionScript(t, r, context.Background(), 120*time.Millisecond, `
 sleep(70);
 var result = download("https://cdn.example.com/audio", "audio.flac");
 if (!result.success) throw new Error("transfer failed");
 sleep(70); // refresh must spend the remaining allowance, not a new 120ms
 true;
 `)
	if !IsTimeoutError(err) {
		t.Fatalf("expected cumulative timeout, got %v", err)
	}
}

func TestResolutionBudgetIncludesTransferFirstByteAndProgressCallbacks(t *testing.T) {
	for _, mode := range []string{"first-byte", "callback"} {
		t.Run(mode, func(t *testing.T) {
			r := newFileDownloadTestRuntime(t, func(req *http.Request) (*http.Response, error) {
				body := &resolutionSlowBody{ctx: req.Context()}
				if mode == "first-byte" {
					body.reads = 1
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, ContentLength: 3, Request: req}, nil
			})
			r.vm.Set("download", r.fileDownload)
			r.vm.Set("sleep", r.sleep)
			_, err := runResolutionScript(t, r, context.Background(), 60*time.Millisecond, `download("https://cdn.example.com/audio", "audio.flac", {onProgress:function() { sleep(1000); }})`)
			if !IsTimeoutError(err) {
				t.Fatalf("expected timeout, got %v", err)
			}
		})
	}
}

func TestResolutionBudgetPreservesUserCancellationDuringTransfer(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newFileDownloadTestRuntime(t, func(req *http.Request) (*http.Response, error) {
		time.AfterFunc(40*time.Millisecond, cancel)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &resolutionSlowBody{ctx: req.Context()}, ContentLength: 3, Request: req}, nil
	})
	r.vm.Set("download", r.fileDownload)
	_, err := runResolutionScript(t, r, parent, time.Second, `download("https://cdn.example.com/audio","audio.flac")`)
	if !errors.Is(err, ErrExtensionRequestCancelled) || IsTimeoutError(err) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestResolutionBudgetConcurrentPausesAndStop(t *testing.T) {
	b := newResolutionBudget(context.Background(), time.Second)
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				resume := b.pause()
				b.remainingTime()
				resume()
				resume()
			}
		}()
	}
	workers.Wait()
	b.stop()
	if !errors.Is(context.Cause(b.ctx), context.Canceled) {
		t.Fatalf("unexpected cause: %v", context.Cause(b.ctx))
	}
}

func TestResolutionBudgetPoolClearsOperationContext(t *testing.T) {
	ext := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
	provider := newExtensionProviderWrapper(ext)
	for i := 0; i < 2; i++ {
		result, err := provider.Download("track-1", "LOSSLESS", filepath.Join(t.TempDir(), "audio.flac"), "", nil)
		if err != nil || !result.Success {
			t.Fatalf("download: %#v, %v", result, err)
		}
		ext.isolatedPoolMu.Lock()
		if len(ext.isolatedPool) != 1 {
			ext.isolatedPoolMu.Unlock()
			t.Fatal("healthy runtime was not pooled")
		}
		r := ext.isolatedPool[0].runtime
		ext.isolatedPoolMu.Unlock()
		if r.currentResolutionBudget() != nil || r.activeOperationContext(context.Background()).Err() != nil {
			t.Fatal("pooled runtime retained expired resolution context")
		}
	}
}

func TestResolutionBudgetFFmpegWaitExcludesConversionAndHonorsCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "cancel"}[cancelled], func(t *testing.T) {
			r := newFileDownloadTestRuntime(t, nil)
			r.extensionID = "resolution-ffmpeg-test"
			r.vm.Set("convert", r.ffmpegConvert)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			responderDone := make(chan struct{})
			go func() {
				defer close(responderDone)
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) {
					ffmpegCommandsMu.RLock()
					id := ""
					for key, command := range ffmpegCommands {
						if command.ExtensionID == r.extensionID {
							id = key
							break
						}
					}
					ffmpegCommandsMu.RUnlock()
					if id != "" {
						time.Sleep(120 * time.Millisecond)
						if cancelled {
							cancel()
						} else {
							SetFFmpegCommandResult(id, true, "converted", "")
						}
						return
					}
					time.Sleep(time.Millisecond)
				}
			}()
			value, err := runResolutionScript(t, r, parent, 60*time.Millisecond, `convert("input.flac","output.flac",{codec:"flac"})`)
			<-responderDone
			if cancelled {
				if !errors.Is(err, ErrExtensionRequestCancelled) {
					t.Fatalf("expected cancel: %v", err)
				}
			} else if err != nil || value.Export().(map[string]any)["success"] != true {
				t.Fatalf("conversion: %v, %v", value, err)
			}
			ffmpegCommandsMu.RLock()
			defer ffmpegCommandsMu.RUnlock()
			for _, command := range ffmpegCommands {
				if command.ExtensionID == r.extensionID {
					t.Fatal("FFmpeg command leaked")
				}
			}
		})
	}
}
