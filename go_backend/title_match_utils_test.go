package gobackend

import "testing"

func TestNormalizeLooseTitle_Separators(t *testing.T) {
	got := normalizeLooseTitle("Doctor / Cops")
	if got != "doctor cops" {
		t.Fatalf("expected doctor cops, got %q", got)
	}

	got = normalizeLooseTitle("Doctor _ Cops")
	if got != "doctor cops" {
		t.Fatalf("expected doctor cops, got %q", got)
	}
}

func TestNormalizeLooseTitle_EmojiAndSymbols(t *testing.T) {
	got := normalizeLooseTitle("Music Of The Spheres 🌎✨")
	if got != "music of the spheres" {
		t.Fatalf("expected music of the spheres, got %q", got)
	}
}

func TestTrackMatchesRequest_SongLinkBypassesArtistAndTitle(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Ringišpil",
		ArtistName: "Djordje Balasevic",
	}
	resolved := resolvedTrackInfo{
		Title:                "Completely Different Title",
		ArtistName:           "Totally Different Artist",
		SkipNameVerification: true,
	}

	if !trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected SongLink-resolved track to bypass artist/title verification")
	}
}

func TestTrackMatchesRequest_SongLinkStillChecksDuration(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Ringišpil",
		ArtistName: "Djordje Balasevic",
		DurationMS: 180000,
	}
	resolved := resolvedTrackInfo{
		Title:                "Completely Different Title",
		ArtistName:           "Totally Different Artist",
		Duration:             240,
		SkipNameVerification: true,
	}

	if trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected SongLink-resolved track with large duration mismatch to be rejected")
	}
}

func TestTrackMatchesRequestRejectsDifferentAlbumWithoutExactISRC(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Bewafa",
		ArtistName: "Imran Khan",
		AlbumName:  "Unforgettable",
		ISRC:       "GBUM70901234",
	}
	resolved := resolvedTrackInfo{
		Title:      "Bewafa",
		ArtistName: "Imran Khan, Tarandeep Singh",
		AlbumName:  "Bewafa",
		ISRC:       "QZXYZ2600001",
	}

	if trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected same-title cover from a different album to be rejected")
	}
}

func TestTrackMatchesRequestAcceptsDifferentEditionWithExactISRC(t *testing.T) {
	req := DownloadRequest{
		TrackName: "Song",
		AlbumName: "Original Album",
		ISRC:      "USRC17607839",
	}
	resolved := resolvedTrackInfo{
		Title:     "Song",
		AlbumName: "Deluxe Collection",
		ISRC:      "usrc17607839",
	}

	if !trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected an exact ISRC match to accept another release edition")
	}
}

func TestTrackMatchesRequestAcceptsSameTrackFromDifferentRelease(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Crossing Field",
		ArtistName: "LiSA",
		AlbumName:  "Crossing Field - EP",
		DurationMS: 233000,
	}
	resolved := resolvedTrackInfo{
		Title:      "Crossing Field",
		ArtistName: "LiSA",
		AlbumName:  "LANDSPACE",
		Duration:   233,
	}

	if !trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected the same track to be accepted across release albums")
	}
}

func TestTrackMatchesRequestRejectsAlbumMismatchForDifferentVersion(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Song (Live)",
		ArtistName: "Artist",
		AlbumName:  "Live at the Theatre",
	}
	resolved := resolvedTrackInfo{
		Title:      "Song",
		ArtistName: "Artist",
		AlbumName:  "Studio Album",
	}

	if trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected an album mismatch to reject a different track version")
	}
}

func TestTrackMatchesRequestRejectsConflictingISRCDespiteStrongNames(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Song",
		ArtistName: "Artist",
		AlbumName:  "Original Album",
		ISRC:       "USAAA2600001",
	}
	resolved := resolvedTrackInfo{
		Title:      "Song",
		ArtistName: "Artist",
		AlbumName:  "Other Album",
		ISRC:       "USAAA2600002",
	}

	if trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected conflicting ISRCs to keep album verification strict")
	}
}

func TestTrackMatchesRequestRejectsDurationMismatchAcrossReleases(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Crossing Field",
		ArtistName: "LiSA",
		AlbumName:  "Crossing Field - EP",
		DurationMS: 233000,
	}
	resolved := resolvedTrackInfo{
		Title:      "Crossing Field",
		ArtistName: "LiSA",
		AlbumName:  "LANDSPACE",
		Duration:   280,
	}

	if trackMatchesRequest(req, resolved, "test") {
		t.Fatal("expected a large duration mismatch to reject another recording")
	}
}

func TestTitlesMatch_SeparatorVariants(t *testing.T) {
	if !titlesMatch("Doctor / Cops", "Doctor _ Cops") {
		t.Fatal("expected titlesMatch to accept / vs _ variant")
	}
}

func TestTrackMatchingPreservesArtistCreditBoundaries(t *testing.T) {
	req := DownloadRequest{
		TrackName: "Signal - Remix", ArtistName: "Composer, Lead Singer & Lyric Writer",
		AlbumName: "Original Soundtrack", DurationMS: 234000,
	}
	for _, artist := range []string{
		"Composer, Lead Singer, Guest Writer",
		"Lead Singer & Composer",
		"GUEST WRITER; LEAD SINGER",
	} {
		t.Run(artist, func(t *testing.T) {
			resolved := resolvedTrackInfo{
				Title: "Signal (Remix)", ArtistName: artist,
				AlbumName: "Original Soundtrack", Duration: 234,
			}
			if !trackMatchesRequest(req, resolved, "test") {
				t.Fatal("matching recording rejected because contributor credits differ")
			}
			tracks := []ExtTrackMetadata{{
				Name: resolved.Title, Artists: artist, AlbumName: "Collection",
				DurationMS: 234000, ProviderID: "provider",
			}}
			if selectBestMetadataEnrichmentTrack(req, tracks) == nil {
				t.Fatal("matching recording rejected during metadata enrichment")
			}
		})
	}
}

func TestTrackMetadataTolerancePreservesRecordingIdentity(t *testing.T) {
	req := DownloadRequest{
		TrackName: "Signal - Remix", ArtistName: "Composer, Lead Singer & Lyric Writer",
		AlbumName: "Original Soundtrack", DurationMS: 234000,
	}
	for _, tc := range []struct {
		name     string
		title    string
		artist   string
		duration int
		want     bool
	}{
		{"punctuation", "Signal (Remix)", req.ArtistName, 234, true},
		{"original", "Signal", req.ArtistName, 234, false},
		{"named mix", "Signal (Club Mix)", req.ArtistName, 234, false},
		{"live remix", "Signal (Remix Live)", req.ArtistName, 234, false},
		{"other artist", "Signal (Remix)", "Unrelated Singer", 234, false},
		{"other duration", "Signal (Remix)", "Composer, Lead Singer, Guest Writer", 305, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := trackMatchesRequest(req, resolvedTrackInfo{
				Title: tc.title, ArtistName: tc.artist, AlbumName: req.AlbumName, Duration: tc.duration,
			}, "test")
			if got != tc.want {
				t.Fatalf("trackMatchesRequest = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTitlesMatch_EmojiStrict(t *testing.T) {
	if titlesMatch("🪐", "Higher Power") {
		t.Fatal("expected emoji title not to match unrelated textual title")
	}
	if !titlesMatch("🪐", "🪐") {
		t.Fatal("expected identical emoji titles to match")
	}
}

func TestSelectBestMetadataEnrichmentTrackSkipsWrongFirstResult(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Song",
		ArtistName: "Original Artist",
		DurationMS: 180000,
	}
	tracks := []ExtTrackMetadata{
		{
			Name:       "Song",
			Artists:    "Cover Band",
			AlbumName:  "Covers",
			DurationMS: 180000,
			ProviderID: "first",
		},
		{
			Name:        "Song",
			Artists:     "Original Artist",
			AlbumName:   "Original Album",
			ReleaseDate: "2026-01-01",
			ISRC:        "USAAA2600001",
			DurationMS:  180000,
			ProviderID:  "second",
		},
	}

	best := selectBestMetadataEnrichmentTrack(req, tracks)
	if best == nil || best.ProviderID != "second" {
		t.Fatalf("best metadata match = %#v, want second result", best)
	}
}

func TestSelectBestMetadataEnrichmentTrackRejectsConflictingISRC(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Song",
		ArtistName: "Artist",
		ISRC:       "USAAA2600001",
	}
	tracks := []ExtTrackMetadata{{
		Name:       "Song",
		Artists:    "Artist",
		AlbumName:  "Album",
		ISRC:       "USAAA2600002",
		ProviderID: "provider",
	}}

	if best := selectBestMetadataEnrichmentTrack(req, tracks); best != nil {
		t.Fatalf("expected conflicting ISRC to be rejected, got %#v", best)
	}
}

func TestSelectBestMetadataEnrichmentTrackAcceptsExactISRC(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Localized Song Name",
		ArtistName: "Localized Artist Name",
		ISRC:       "USAAA2600001",
	}
	tracks := []ExtTrackMetadata{{
		Name:       "Original Song Name",
		Artists:    "Original Artist Name",
		AlbumName:  "Album",
		ISRC:       "usaaa2600001",
		ProviderID: "provider",
	}}

	if best := selectBestMetadataEnrichmentTrack(req, tracks); best == nil {
		t.Fatal("expected exact ISRC to provide a confident metadata match")
	}
}

func TestSelectBestMetadataEnrichmentTrackRejectsWeakArtistMatch(t *testing.T) {
	req := DownloadRequest{TrackName: "Song", ArtistName: "Artist"}
	tracks := []ExtTrackMetadata{{
		Name:       "Song",
		Artists:    "Artist feat. Someone Else",
		AlbumName:  "Album",
		ProviderID: "provider",
	}}

	if best := selectBestMetadataEnrichmentTrack(req, tracks); best != nil {
		t.Fatalf("expected fuzzy artist match without duration to be rejected, got %#v", best)
	}
}
