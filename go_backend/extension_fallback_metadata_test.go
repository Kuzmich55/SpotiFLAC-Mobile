package gobackend

import "testing"

func TestOverlayExtensionReleaseMetadataFillsMissingRequestFields(t *testing.T) {
	req := DownloadRequest{AlbumType: "single"}
	track := ExtTrackMetadata{
		AlbumType: "album",
		Explicit:  true,
		UPC:       "4006381333931",
		Comment:   "https://source.example/album/1532211596",
	}

	overlayExtensionReleaseMetadata(&req, track)

	if req.AlbumType != "single" {
		t.Fatalf("existing album type was overwritten: %q", req.AlbumType)
	}
	if !req.Explicit {
		t.Fatal("explicit flag from source enrichment was not retained")
	}
	if req.UPC != track.UPC {
		t.Fatalf("UPC = %q, want %q", req.UPC, track.UPC)
	}
	if req.Comment != track.Comment {
		t.Fatalf("comment = %q, want %q", req.Comment, track.Comment)
	}

	response := buildDownloadSuccessResponse(
		req,
		DownloadResult{},
		"download-provider",
		"downloaded",
		"song.m4a",
		false,
	)
	if response.UPC != track.UPC || response.AlbumType != "single" ||
		!response.Explicit || response.Comment != track.Comment {
		t.Fatalf("enriched metadata was lost in download response: %#v", response)
	}
}

func TestOverlayExtensionReleaseMetadataDoesNotEraseExistingValues(t *testing.T) {
	req := DownloadRequest{
		AlbumType: "ep",
		Explicit:  true,
		UPC:       "existing-upc",
		Comment:   "existing-comment",
	}

	overlayExtensionReleaseMetadata(&req, ExtTrackMetadata{})

	if req.AlbumType != "ep" || !req.Explicit ||
		req.UPC != "existing-upc" || req.Comment != "existing-comment" {
		t.Fatalf("existing release metadata changed: %#v", req)
	}
}

func TestBuildSourceExtensionTrackMetadataCarriesKnownIdentifiers(t *testing.T) {
	req := DownloadRequest{
		SpotifyID: "source-id",
		TidalID:   "alternate-id-a",
		QobuzID:   "alternate-id-b",
		DeezerID:  "alternate-id-c",
	}

	track := buildSourceExtensionTrackMetadata(req)
	if track.ID != req.SpotifyID || track.SpotifyID != req.SpotifyID {
		t.Fatalf("primary identifier was not propagated: %#v", track)
	}
	if track.TidalID != req.TidalID || track.QobuzID != req.QobuzID || track.DeezerID != req.DeezerID {
		t.Fatalf("alternate identifiers were not propagated: %#v", track)
	}
}

func TestOverlaySourceExtensionTrackIdentityPreservesRequestedValues(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Album Display Title",
		ArtistName: "Album Artist Credit",
	}

	overlaySourceExtensionTrackIdentity(&req, ExtTrackMetadata{
		Name:    "Single Display Title",
		Artists: "Provider Artist Credit",
	})

	if req.TrackName != "Album Display Title" {
		t.Fatalf("track name = %q, want requested title", req.TrackName)
	}
	if req.ArtistName != "Album Artist Credit" {
		t.Fatalf("artist name = %q, want requested credit", req.ArtistName)
	}
}

func TestOverlaySourceExtensionTrackIdentityFillsMissingValues(t *testing.T) {
	req := DownloadRequest{}
	overlaySourceExtensionTrackIdentity(&req, ExtTrackMetadata{
		Name:    "Resolved Title",
		Artists: "Resolved Artist",
	})

	if req.TrackName != "Resolved Title" || req.ArtistName != "Resolved Artist" {
		t.Fatalf("missing identity fields were not enriched: %#v", req)
	}
}

func TestExtensionPreparedDownloadContextMergesHostMetadata(t *testing.T) {
	providerContext := map[string]any{
		"token":      "opaque-provider-value",
		"host_track": "provider-key-must-not-win",
	}
	req := DownloadRequest{
		ProviderTrackID: "provider-track-1",
		TrackName:       "Song",
		ArtistName:      "Artist",
		AlbumName:       "Album",
		ISRC:            "ISRC123",
	}

	merged := extensionPreparedDownloadContext(req, providerContext)
	if merged["token"] != "opaque-provider-value" {
		t.Fatalf("provider context was not preserved: %#v", merged)
	}
	hostTrack, ok := merged["host_track"].(map[string]any)
	if !ok {
		t.Fatalf("host_track = %#v", merged["host_track"])
	}
	if hostTrack["id"] != req.ProviderTrackID || hostTrack["name"] != req.TrackName || hostTrack["isrc"] != req.ISRC {
		t.Fatalf("host metadata was not propagated: %#v", hostTrack)
	}
	if providerContext["host_track"] != "provider-key-must-not-win" {
		t.Fatalf("input provider context was mutated: %#v", providerContext)
	}
}
