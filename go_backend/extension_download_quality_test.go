package gobackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func qualityTestManifest(name string, options ...QualityOption) *ExtensionManifest {
	return &ExtensionManifest{Name: name, QualityOptions: options}
}

func TestExtensionQualityKeepsAudioKindAcrossProviders(t *testing.T) {
	amazon := qualityTestManifest("source", QualityOption{ID: "best", Label: "FLAC Best Available"}, QualityOption{ID: "ac4", Label: "Dolby Atmos"})
	tidal := qualityTestManifest("target",
		QualityOption{ID: "DOLBY_ATMOS", Label: "Dolby Atmos", Description: "falls back to FLAC"},
		QualityOption{ID: "HI_RES_LOSSLESS", Label: "HiRes FLAC"},
		QualityOption{ID: "LOSSLESS", Label: "Lossless"},
		QualityOption{ID: "HIGH", Label: "High"},
	)
	for _, tc := range []struct{ requested, want string }{
		{"best", "HI_RES_LOSSLESS"}, {"DEFAULT", "HI_RES_LOSSLESS"}, {"", "HI_RES_LOSSLESS"},
		{"LOSSLESS", "LOSSLESS"}, {"lossless", "LOSSLESS"}, {"HI_RES_LOSSLESS", "HI_RES_LOSSLESS"},
		{"ac4", "DOLBY_ATMOS"}, {"DOLBY_ATMOS", "DOLBY_ATMOS"}, {"HIGH", "HIGH"},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			got, err := resolveExtensionDownloadQuality(tc.requested, amazon, tidal)
			if err != nil || got != tc.want {
				t.Fatalf("quality=%q err=%v; want %q", got, err, tc.want)
			}
		})
	}
	if got, err := resolveExtensionDownloadQuality("best", amazon, amazon); err != nil || got != "best" {
		t.Fatalf("same provider selection changed: %s %v", got, err)
	}
}

func TestExtensionQualityUsesDeclarationsAndRejectsLosslessDowngrade(t *testing.T) {
	source := qualityTestManifest("source", QualityOption{ID: "studio", Kind: "lossless"})
	target := qualityTestManifest("target", QualityOption{ID: "studio", Kind: "spatial"}, QualityOption{ID: "pcm", Kind: "lossless"})
	if got, err := resolveExtensionDownloadQuality("studio", source, target); err != nil || got != "pcm" {
		t.Fatalf("foreign ID collision: %s %v", got, err)
	}
	for _, target := range []*ExtensionManifest{
		qualityTestManifest("spatial-only", QualityOption{ID: "DOLBY_ATMOS"}),
		qualityTestManifest("lossy-only", QualityOption{ID: "mp3_128"}),
		qualityTestManifest("unknown", QualityOption{ID: "custom"}),
	} {
		if _, err := resolveExtensionDownloadQuality("studio", source, target); err == nil {
			t.Fatalf("accepted incompatible provider %s", target.Name)
		}
	}
	legacy := qualityTestManifest("legacy")
	if got, err := resolveExtensionDownloadQuality("custom", nil, legacy); err != nil || got != "custom" {
		t.Fatalf("legacy: %s %v", got, err)
	}
	lossless := qualityTestManifest("flac", QualityOption{ID: "flac"})
	if got, err := resolveExtensionDownloadQuality("DOLBY_ATMOS", nil, lossless); err != nil || got != "flac" {
		t.Fatalf("explicit Atmos FLAC fallback: %s %v", got, err)
	}
}

func TestExtensionQualityBestUsesProviderKind(t *testing.T) {
	source := qualityTestManifest("lossy-source", QualityOption{ID: "best", Label: "Best Audio"})
	source.Capabilities = map[string]any{"downloadFallbackTier": "low_res"}
	target := qualityTestManifest("target", QualityOption{ID: "DOLBY_ATMOS"}, QualityOption{ID: "LOSSLESS"}, QualityOption{ID: "HIGH"})
	if got, err := resolveExtensionDownloadQuality("best", source, target); err != nil || got != "HIGH" {
		t.Fatalf("lossy best: %s %v", got, err)
	}
	if kind := extensionQualityKind(QualityOption{ID: "DOLBY_ATMOS", Description: "best available FLAC fallback"}, target); kind != "spatial" {
		t.Fatal(kind)
	}
}

func TestDownloadFallbackAndVerificationResumePreserveLosslessQuality(t *testing.T) {
	source := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
	source.ID, source.Manifest.Name = "quality-source", "quality-source"
	source.Manifest.QualityOptions = []QualityOption{{ID: "best", Label: "FLAC Best Available"}}
	target := newTestLoadedExtension(t, ExtensionTypeDownloadProvider)
	target.ID, target.Manifest.Name = "quality-target", "quality-target"
	target.Manifest.QualityOptions = []QualityOption{{ID: "DOLBY_ATMOS"}, {ID: "HI_RES_LOSSLESS"}, {ID: "LOSSLESS"}}
	for ext, script := range map[*loadedExtension]string{
		source: `registerExtension({checkAvailability:function(){return {available:false};},download:function(){return {success:false,error_type:"not_found",error_message:"source unavailable"};}});`,
		target: `registerExtension({checkAvailability:function(){return {available:true,track_id:"target-track"};},download:function(id,quality){return {success:false,error_type:"not_found",error_message:"observed-quality:"+quality};}});`,
	} {
		if err := os.WriteFile(filepath.Join(ext.SourceDir, "index.js"), []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manager := getExtensionManager()
	manager.mu.Lock()
	previous := manager.extensions
	manager.extensions = map[string]*loadedExtension{source.ID: source, target.ID: target}
	manager.mu.Unlock()
	priority, fallback := GetProviderPriority(), GetExtensionFallbackProviderIDs()
	SetProviderPriority([]string{source.ID, target.ID})
	SetExtensionFallbackProviderIDs(nil)
	t.Cleanup(func() {
		teardownExtension(source)
		teardownExtension(target)
		manager.mu.Lock()
		manager.extensions = previous
		manager.mu.Unlock()
		SetProviderPriority(priority)
		SetExtensionFallbackProviderIDs(fallback)
		resetPreparedDownloadRequestCacheForTest()
	})
	for _, mode := range []string{"fallback", "direct-source", "verification-resume"} {
		t.Run(mode, func(t *testing.T) {
			req := DownloadRequest{Service: source.ID, ItemID: "quality-" + mode, TrackName: "Song", ArtistName: "Artist", AlbumName: "Album", ISRC: "USRC17607839", ReleaseDate: "2026-01-01", OutputDir: t.TempDir(), FilenameFormat: "{title}", Quality: "best", UseFallback: true}
			if mode == "direct-source" {
				req.Source = source.ID
			}
			if mode == "verification-resume" {
				cacheUnpreparedDownloadRequest(downloadPreparationKey(req), req)
			}
			response, err := DownloadWithExtensionFallback(req)
			if err != nil || response == nil || !strings.Contains(response.Error, "observed-quality:HI_RES_LOSSLESS") {
				t.Fatalf("response=%+v err=%v", response, err)
			}
		})
	}
	// A direct/verified invocation uses the same translation, without relying
	// on the normal provider-loop branch to repair the foreign token.
	req := DownloadRequest{Service: source.ID, Quality: "best", OutputDir: t.TempDir(), TrackName: "Direct"}
	var lastErr error
	var errorType string
	var retryAfter int
	attemptExtensionDownload(req, target, newExtensionProviderWrapper(target), "target-track", req.Quality, target.ID, nil, false, &lastErr, &errorType, &retryAfter)
	if lastErr == nil || !strings.Contains(lastErr.Error(), "observed-quality:HI_RES_LOSSLESS") {
		t.Fatal(lastErr)
	}
}
