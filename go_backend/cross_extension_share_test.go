package gobackend

import "testing"

func TestCrossExtensionShareUsesAlbumCollectionItems(t *testing.T) {
	ext := &loadedExtension{
		Manifest: &ExtensionManifest{
			Capabilities: map[string]any{
				"shareUrlTemplates": map[string]any{
					"album": "https://media.example/album/{id}",
				},
			},
		},
	}
	tracks := []ExtTrackMetadata{
		{
			ID:       "1440783617",
			Name:     "Nevermind",
			Artists:  "Nirvana",
			ItemType: "album",
		},
	}

	best := bestAlbumTrack(tracks, "Nevermind", "Nirvana")
	if best == nil {
		t.Fatal("expected album collection item to match")
	}
	if url := resolveCollectionShareURL(ext, "album", best); url != "https://media.example/album/1440783617" {
		t.Fatalf("album share URL = %q", url)
	}
}

func TestCrossExtensionShareUsesArtistCollectionItems(t *testing.T) {
	ext := &loadedExtension{
		Manifest: &ExtensionManifest{
			Capabilities: map[string]any{
				"shareUrlTemplates": map[string]any{
					"artist": "https://media.example/artist/{id}",
				},
			},
		},
	}
	tracks := []ExtTrackMetadata{
		{
			ID:       "UCrPe3hLA51968GwxHSZ1llw",
			Name:     "Nirvana",
			ItemType: "artist",
		},
	}

	best := bestArtistTrack(tracks, "Nirvana")
	if best == nil {
		t.Fatal("expected artist collection item to match")
	}
	if url := resolveCollectionShareURL(ext, "artist", best); url != "https://media.example/artist/UCrPe3hLA51968GwxHSZ1llw" {
		t.Fatalf("artist share URL = %q", url)
	}
}

func TestCrossExtensionShareCacheKeyIsProviderOrderStable(t *testing.T) {
	providerA := &extensionProviderWrapper{
		extension: &loadedExtension{
			ID:        "provider-a",
			SourceDir: "/extensions/provider-a",
			Manifest:  &ExtensionManifest{DisplayName: "Provider A"},
		},
	}
	providerB := &extensionProviderWrapper{
		extension: &loadedExtension{
			ID:        "provider-b",
			SourceDir: "/extensions/provider-b",
			Manifest:  &ExtensionManifest{DisplayName: "Provider B"},
		},
	}

	first := crossExtensionShareCacheKey("Nevermind", "Nirvana", "album", "metadata-source", []*extensionProviderWrapper{providerA, providerB})
	second := crossExtensionShareCacheKey("Nevermind", "Nirvana", "album", "metadata-source", []*extensionProviderWrapper{providerB, providerA})
	if first != second {
		t.Fatalf("cache key should not depend on provider order:\n%s\n%s", first, second)
	}
}

func TestCrossExtensionShareCacheableSkipsTransientErrors(t *testing.T) {
	cacheable := []CrossExtensionShareResult{
		{ExtensionID: "provider-a", Found: true, URL: "https://media.example/album/1"},
		{ExtensionID: "provider-b", Error: "album not found"},
		{ExtensionID: "provider-c", Error: "no results"},
	}
	if !crossExtensionShareResultsCacheable(cacheable) {
		t.Fatal("expected found and deterministic not-found results to be cacheable")
	}

	transient := []CrossExtensionShareResult{
		{ExtensionID: "provider-a", Found: true, URL: "https://media.example/album/1"},
		{ExtensionID: "provider-b", Error: "request failed: timeout"},
	}
	if crossExtensionShareResultsCacheable(transient) {
		t.Fatal("expected transient extension errors to skip cache")
	}
}
