package gobackend

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSetExtensionFallbackProviderIDsJSONEmptyStringResetsDefault(t *testing.T) {
	original := GetExtensionFallbackProviderIDs()
	defer SetExtensionFallbackProviderIDs(original)

	SetExtensionFallbackProviderIDs([]string{"custom-ext"})

	if err := SetExtensionFallbackProviderIDsJSON(""); err != nil {
		t.Fatalf("SetExtensionFallbackProviderIDsJSON returned error: %v", err)
	}

	if got := GetExtensionFallbackProviderIDs(); got != nil {
		t.Fatalf("expected nil fallback provider list after reset, got %v", got)
	}
}

func TestBuildDownloadSuccessResponsePrefersRequestedAlbumMetadata(t *testing.T) {
	req := DownloadRequest{
		TrackName:   "Bonus Track",
		ArtistName:  "Artist",
		AlbumName:   "Album (Deluxe)",
		AlbumArtist: "Artist",
		ReleaseDate: "2024-01-01",
		TrackNumber: 14,
		DiscNumber:  1,
		ISRC:        "REQ123",
		CoverURL:    "https://example.com/cover.jpg",
		Genre:       "Pop",
		Label:       "Label",
		Copyright:   "Copyright",
	}

	result := DownloadResult{
		Title:       "Bonus Track",
		Artist:      "Artist",
		Album:       "Album",
		ReleaseDate: "2023-12-01",
		TrackNumber: 2,
		DiscNumber:  9,
		ISRC:        "RES456",
	}

	resp := buildDownloadSuccessResponse(
		req,
		result,
		"download-provider",
		"ok",
		"/tmp/test.flac",
		false,
	)

	if resp.Album != req.AlbumName {
		t.Fatalf("album = %q, want %q", resp.Album, req.AlbumName)
	}
	if resp.ReleaseDate != req.ReleaseDate {
		t.Fatalf("release date = %q, want %q", resp.ReleaseDate, req.ReleaseDate)
	}
	if resp.TrackNumber != req.TrackNumber {
		t.Fatalf("track number = %d, want %d", resp.TrackNumber, req.TrackNumber)
	}
	if resp.DiscNumber != req.DiscNumber {
		t.Fatalf("disc number = %d, want %d", resp.DiscNumber, req.DiscNumber)
	}
	if resp.Artist != result.Artist {
		t.Fatalf("artist = %q, want provider artist %q", resp.Artist, result.Artist)
	}
	if resp.ISRC != result.ISRC {
		t.Fatalf("isrc = %q, want provider isrc %q", resp.ISRC, result.ISRC)
	}
}

func TestPreferredReleaseMetadataPrefersRequestValues(t *testing.T) {
	album, releaseDate, trackNumber, discNumber := preferredReleaseMetadata(
		DownloadRequest{
			AlbumName:   "Album (Deluxe Edition)",
			ReleaseDate: "2024-01-01",
			TrackNumber: 13,
			DiscNumber:  2,
		},
		"Album",
		"2023-01-01",
		3,
		1,
	)

	if album != "Album (Deluxe Edition)" {
		t.Fatalf("album = %q", album)
	}
	if releaseDate != "2024-01-01" {
		t.Fatalf("release date = %q", releaseDate)
	}
	if trackNumber != 13 {
		t.Fatalf("track number = %d", trackNumber)
	}
	if discNumber != 2 {
		t.Fatalf("disc number = %d", discNumber)
	}
}

func TestBuildDownloadSuccessResponsePreservesRequestedTrackAndCover(t *testing.T) {
	req := DownloadRequest{
		TrackName:   "Album Track Title",
		ArtistName:  "Artist",
		AlbumName:   "Original Album",
		AlbumArtist: "Artist",
		CoverURL:    "https://cdn.source.test/original-album.jpg",
		ISRC:        "USAAA2600001",
	}

	result := DownloadResult{
		Title:    "Single Version Title",
		Artist:   "Artist",
		Album:    "Single Version Title",
		CoverURL: "https://cdn.provider.test/single.jpg",
		ISRC:     "USAAA2600001",
	}

	resp := buildDownloadSuccessResponse(
		req,
		result,
		"generic-provider",
		"ok",
		"/tmp/test.flac",
		false,
	)
	overlayExtensionDownloadMetadata(&resp, &ExtDownloadResult{
		Title:    result.Title,
		Artist:   result.Artist,
		Album:    result.Album,
		CoverURL: result.CoverURL,
		ISRC:     result.ISRC,
	})
	applyExtensionRequestFallbacks(&resp, req)

	if resp.Title != req.TrackName {
		t.Fatalf("title = %q, want requested title %q", resp.Title, req.TrackName)
	}
	if resp.Album != req.AlbumName {
		t.Fatalf("album = %q, want requested album %q", resp.Album, req.AlbumName)
	}
	if resp.CoverURL != req.CoverURL {
		t.Fatalf("cover url = %q, want requested album cover %q", resp.CoverURL, req.CoverURL)
	}
}

func TestBuildDownloadSuccessResponseFallsBackToProviderTrackAndCover(t *testing.T) {
	req := DownloadRequest{
		ArtistName:  "Artist",
		AlbumName:   "Album",
		AlbumArtist: "Artist",
	}

	result := DownloadResult{
		Title:    "Track",
		Artist:   "Artist",
		Album:    "Album",
		CoverURL: "https://cdn.provider.test/cover.jpg",
	}

	resp := buildDownloadSuccessResponse(
		req,
		result,
		"generic-provider",
		"ok",
		"/tmp/test.flac",
		false,
	)

	if resp.Title != result.Title {
		t.Fatalf("title = %q, want provider fallback %q", resp.Title, result.Title)
	}
	if resp.CoverURL != result.CoverURL {
		t.Fatalf("cover url = %q, want provider fallback %q", resp.CoverURL, result.CoverURL)
	}
}

func TestBuildDownloadSuccessResponseReturnsResolvedProviderFilename(t *testing.T) {
	req := DownloadRequest{
		TrackName:        "Track",
		ArtistName:       "Artist",
		DownloadProvider: "download-provider",
		ProviderTrackID:  "998877",
		FilenameFormat:   "{artist} - {title} [{isrc}] [{provider}-{provider_id}]",
		OutputExt:        ".flac",
	}
	result := DownloadResult{
		ISRC:            "USABC1234567",
		ActualExtension: ".m4a",
	}

	resp := buildDownloadSuccessResponse(
		req,
		result,
		"download-provider",
		"ok",
		"/proc/self/fd/10",
		false,
	)

	want := "Artist - Track [USABC1234567] [download-provider-998877].m4a"
	if resp.ResolvedFileName != want {
		t.Fatalf("resolved filename = %q, want %q", resp.ResolvedFileName, want)
	}
	if resp.ProviderTrackID != req.ProviderTrackID {
		t.Fatalf("provider track ID = %q, want %q", resp.ProviderTrackID, req.ProviderTrackID)
	}
}

func TestBuildDownloadSuccessResponseNormalizesDecryptionDescriptor(t *testing.T) {
	req := DownloadRequest{
		TrackName:  "Track",
		ArtistName: "Artist",
	}

	result := DownloadResult{
		Title:         "Track",
		Artist:        "Artist",
		DecryptionKey: "00112233",
	}

	resp := buildDownloadSuccessResponse(
		req,
		result,
		"download-provider",
		"ok",
		"/tmp/test.m4a",
		false,
	)

	if resp.Decryption == nil {
		t.Fatal("expected decryption descriptor to be present")
	}
	if resp.Decryption.Strategy != genericFFmpegMOVDecryptionStrategy {
		t.Fatalf("strategy = %q", resp.Decryption.Strategy)
	}
	if resp.Decryption.Key != result.DecryptionKey {
		t.Fatalf("key = %q, want %q", resp.Decryption.Key, result.DecryptionKey)
	}
}

func TestFormatMusicBrainzGenrePrefersHighestCountTag(t *testing.T) {
	got := formatMusicBrainzGenre([]musicBrainzTag{
		{Name: "art pop", Count: 3},
		{Name: "pop", Count: 8},
		{Name: "dance pop", Count: 5},
	})

	if got != "Pop" {
		t.Fatalf("genre = %q, want %q", got, "Pop")
	}
}

func TestSelectMusicBrainzAlbumArtistPrefersMatchingRelease(t *testing.T) {
	releases := []musicBrainzRelease{
		{
			Title: "Other Album",
			ArtistCredit: []musicBrainzArtistCredit{
				{Name: "Wrong Artist"},
			},
		},
		{
			Title: "Target Album",
			ArtistCredit: []musicBrainzArtistCredit{
				{Name: "Artist A", JoinPhrase: " & "},
				{Name: "Artist B"},
			},
		},
	}

	got := selectMusicBrainzAlbumArtist(releases, "Target Album")
	if got != "Artist A & Artist B" {
		t.Fatalf("album artist = %q, want matching release artist credit", got)
	}
}

func TestEnrichRequestExtendedMetadataUsesMusicBrainzAlbumArtist(t *testing.T) {
	origDeezerFetcher := fetchDeezerExtendedMetadataByISRC
	origMusicBrainzGenreFetcher := fetchMusicBrainzGenreByISRC
	origMusicBrainzAlbumArtistFetcher := fetchMusicBrainzAlbumArtistByISRC
	defer func() {
		fetchDeezerExtendedMetadataByISRC = origDeezerFetcher
		fetchMusicBrainzGenreByISRC = origMusicBrainzGenreFetcher
		fetchMusicBrainzAlbumArtistByISRC = origMusicBrainzAlbumArtistFetcher
	}()

	fetchDeezerExtendedMetadataByISRC = func(ctx context.Context, isrc string) (*AlbumExtendedMetadata, error) {
		return &AlbumExtendedMetadata{}, nil
	}
	fetchMusicBrainzGenreByISRC = func(isrc string) (string, error) {
		return "", fmt.Errorf("no genre")
	}
	fetchMusicBrainzAlbumArtistByISRC = func(isrc string, albumName string) (string, error) {
		if isrc != "TESTISRC" || albumName != "Target Album" {
			t.Fatalf("unexpected MusicBrainz args: %q / %q", isrc, albumName)
		}
		return "MusicBrainz Album Artist", nil
	}

	req := DownloadRequest{
		ISRC:       "TESTISRC",
		ArtistName: "Track Artist",
		AlbumName:  "Target Album",
	}

	enrichRequestExtendedMetadata(&req)

	if req.AlbumArtist != "MusicBrainz Album Artist" {
		t.Fatalf("album artist = %q, want MusicBrainz value", req.AlbumArtist)
	}
}

func TestEnrichRequestExtendedMetadataDoesNotFallbackAlbumArtistToTrackArtist(t *testing.T) {
	origDeezerFetcher := fetchDeezerExtendedMetadataByISRC
	origMusicBrainzGenreFetcher := fetchMusicBrainzGenreByISRC
	origMusicBrainzAlbumArtistFetcher := fetchMusicBrainzAlbumArtistByISRC
	defer func() {
		fetchDeezerExtendedMetadataByISRC = origDeezerFetcher
		fetchMusicBrainzGenreByISRC = origMusicBrainzGenreFetcher
		fetchMusicBrainzAlbumArtistByISRC = origMusicBrainzAlbumArtistFetcher
	}()

	fetchDeezerExtendedMetadataByISRC = func(ctx context.Context, isrc string) (*AlbumExtendedMetadata, error) {
		return &AlbumExtendedMetadata{}, nil
	}
	fetchMusicBrainzGenreByISRC = func(isrc string) (string, error) {
		return "", fmt.Errorf("no genre")
	}
	fetchMusicBrainzAlbumArtistByISRC = func(isrc string, albumName string) (string, error) {
		return "", fmt.Errorf("no album artist")
	}

	req := DownloadRequest{
		ISRC:       "TESTISRC",
		ArtistName: "Track Artist",
		AlbumName:  "Target Album",
	}

	enrichRequestExtendedMetadata(&req)

	if req.AlbumArtist != "" {
		t.Fatalf("album artist = %q, want empty when MusicBrainz has no value", req.AlbumArtist)
	}
}

func TestEnrichExtraMetadataByISRCFallsBackToMusicBrainzGenre(t *testing.T) {
	origDeezerFetcher := fetchDeezerExtendedMetadataByISRC
	origMusicBrainzFetcher := fetchMusicBrainzGenreByISRC
	defer func() {
		fetchDeezerExtendedMetadataByISRC = origDeezerFetcher
		fetchMusicBrainzGenreByISRC = origMusicBrainzFetcher
	}()

	fetchDeezerExtendedMetadataByISRC = func(ctx context.Context, isrc string) (*AlbumExtendedMetadata, error) {
		return nil, nil
	}
	fetchMusicBrainzGenreByISRC = func(isrc string) (string, error) {
		if isrc != "TEST123" {
			t.Fatalf("unexpected isrc: %q", isrc)
		}
		return "Alternative Rock", nil
	}

	genre := ""
	label := ""
	copyright := ""
	enrichExtraMetadataByISRC("DownloadWithFallback", "TEST123", &genre, &label, &copyright)

	if genre != "Alternative Rock" {
		t.Fatalf("genre = %q, want fallback genre", genre)
	}
	if label != "" {
		t.Fatalf("label = %q, want empty", label)
	}
	if copyright != "" {
		t.Fatalf("copyright = %q, want empty", copyright)
	}
}

func TestEnrichExtraMetadataByISRCPrefersDeezerGenre(t *testing.T) {
	origDeezerFetcher := fetchDeezerExtendedMetadataByISRC
	origMusicBrainzFetcher := fetchMusicBrainzGenreByISRC
	defer func() {
		fetchDeezerExtendedMetadataByISRC = origDeezerFetcher
		fetchMusicBrainzGenreByISRC = origMusicBrainzFetcher
	}()

	musicBrainzCalled := false
	fetchDeezerExtendedMetadataByISRC = func(ctx context.Context, isrc string) (*AlbumExtendedMetadata, error) {
		return &AlbumExtendedMetadata{
			Genre:     "Synthpop",
			Label:     "EMI",
			Copyright: "(C) Test",
		}, nil
	}
	fetchMusicBrainzGenreByISRC = func(isrc string) (string, error) {
		musicBrainzCalled = true
		return "Rock", nil
	}

	genre := ""
	label := ""
	copyright := ""
	enrichExtraMetadataByISRC("DownloadWithFallback", "TEST456", &genre, &label, &copyright)

	if genre != "Synthpop" {
		t.Fatalf("genre = %q, want Deezer genre", genre)
	}
	if label != "EMI" {
		t.Fatalf("label = %q, want Deezer label", label)
	}
	if copyright != "(C) Test" {
		t.Fatalf("copyright = %q, want Deezer copyright", copyright)
	}
	if musicBrainzCalled {
		t.Fatal("expected MusicBrainz not to be called when Deezer already provides genre")
	}
}

func TestApplyReEnrichTrackMetadataPreservesExistingReleaseDateWhenCandidateMissing(t *testing.T) {
	req := reEnrichRequest{
		SpotifyID:   "source-track-id",
		AlbumName:   "Original Album",
		ReleaseDate: "2024-01-01",
		ISRC:        "REQ123",
	}

	applyReEnrichTrackMetadata(&req, ExtTrackMetadata{
		AlbumName:   "Original Album (Deluxe)",
		ReleaseDate: "",
		ISRC:        "",
	})

	if req.ReleaseDate != "2024-01-01" {
		t.Fatalf("release date = %q, want existing value preserved", req.ReleaseDate)
	}
	if req.AlbumName != "Original Album (Deluxe)" {
		t.Fatalf("album = %q, want updated album", req.AlbumName)
	}
	if req.ISRC != "REQ123" {
		t.Fatalf("isrc = %q, want existing value preserved", req.ISRC)
	}
}

func TestApplyReEnrichTrackMetadataKeepsReleaseIdentityOnAlbumMismatch(t *testing.T) {
	req := reEnrichRequest{
		TrackName:   "Afsana",
		ArtistName:  "Artist Name",
		AlbumName:   "Original Soundtrack",
		CoverURL:    "https://covers/original.jpg",
		TrackNumber: 3,
		ReleaseDate: "2005-01-01",
	}

	applyReEnrichTrackMetadata(&req, ExtTrackMetadata{
		Name:        "Afsana",
		Artists:     "Artist Name",
		AlbumName:   "The Hit Machine",
		CoverURL:    "https://covers/compilation.jpg",
		TrackNumber: 17,
		ReleaseDate: "2010-01-01",
		ISRC:        "NEW123",
	})

	if req.AlbumName != "Original Soundtrack" {
		t.Fatalf("album = %q, want original release kept", req.AlbumName)
	}
	if req.CoverURL != "https://covers/original.jpg" {
		t.Fatalf("cover = %q, want original release cover kept", req.CoverURL)
	}
	if req.TrackNumber != 3 {
		t.Fatalf("track number = %d, want original position kept", req.TrackNumber)
	}
	if req.ReleaseDate != "2005-01-01" {
		t.Fatalf("release date = %q, want original date kept", req.ReleaseDate)
	}
	if req.ISRC != "NEW123" {
		t.Fatalf("isrc = %q, want recording-level fields still enriched", req.ISRC)
	}
}

func TestApplyReEnrichTrackMetadataReplacesStalePlaylistAlbumWhenRequested(t *testing.T) {
	req := reEnrichRequest{
		TrackName:              "Song",
		ArtistName:             "Artist",
		AlbumName:              "Road Trip Playlist",
		CoverURL:               "https://covers/playlist.jpg",
		TrackNumber:            42,
		ReleaseDate:            "",
		ReplaceReleaseMetadata: true,
	}

	applyReEnrichTrackMetadata(&req, ExtTrackMetadata{
		Name:        "Song",
		Artists:     "Artist",
		AlbumName:   "Actual Album",
		AlbumArtist: "Artist",
		CoverURL:    "https://covers/album.jpg",
		TrackNumber: 3,
		ReleaseDate: "2024-01-01",
	})

	if req.AlbumName != "Actual Album" {
		t.Fatalf("album = %q, want actual album", req.AlbumName)
	}
	if req.AlbumArtist != "Artist" {
		t.Fatalf("album artist = %q", req.AlbumArtist)
	}
	if req.CoverURL != "https://covers/album.jpg" {
		t.Fatalf("cover = %q", req.CoverURL)
	}
	if req.TrackNumber != 3 {
		t.Fatalf("track number = %d", req.TrackNumber)
	}
	if req.ReleaseDate != "2024-01-01" {
		t.Fatalf("release date = %q", req.ReleaseDate)
	}
}

func TestSelectBestReEnrichTrackPrefersCandidateWithReleaseDate(t *testing.T) {
	req := reEnrichRequest{
		TrackName:   "Song Title",
		ArtistName:  "Artist Name",
		AlbumName:   "Album Name",
		ReleaseDate: "",
		DurationMs:  180000,
	}

	tracks := []ExtTrackMetadata{
		{
			ID:          "first",
			Name:        "Song Title",
			Artists:     "Artist Name",
			AlbumName:   "Album Name",
			DurationMS:  180000,
			ReleaseDate: "",
			ProviderID:  "metadata-a",
		},
		{
			ID:          "second",
			Name:        "Song Title",
			Artists:     "Artist Name",
			AlbumName:   "Album Name",
			DurationMS:  180000,
			ReleaseDate: "2024-03-09",
			ProviderID:  "metadata-b",
		},
	}

	best := selectBestReEnrichTrack(req, tracks)
	if best == nil {
		t.Fatal("expected a selected track")
	}
	if best.ID != "second" {
		t.Fatalf("selected track = %q, want candidate with release date", best.ID)
	}
}

func TestSelectBestReEnrichTrackRejectsMismatchedSearchResults(t *testing.T) {
	req := reEnrichRequest{
		TrackName:  "Song Title",
		ArtistName: "Artist Name",
		AlbumName:  "Album Name",
		DurationMs: 180000,
	}

	tracks := []ExtTrackMetadata{
		{
			ID:          "wrong-rich-metadata",
			Name:        "Different Song",
			Artists:     "Different Artist",
			AlbumName:   "Album Name",
			DurationMS:  180000,
			ReleaseDate: "2024-03-09",
			TrackNumber: 4,
			DiscNumber:  1,
			ISRC:        "WRONG1234567",
			ProviderID:  "metadata-b",
		},
	}

	if best := selectBestReEnrichTrack(req, tracks); best != nil {
		t.Fatalf("selected track = %q, want no match", best.ID)
	}
}

func TestSelectBestReEnrichTrackAllowsExactISRCDespiteMetadataMismatch(t *testing.T) {
	req := reEnrichRequest{
		TrackName:  "Song Title",
		ArtistName: "Artist Name",
		ISRC:       "USRC17607839",
		DurationMs: 999999000,
	}

	tracks := []ExtTrackMetadata{
		{
			ID:         "same-isrc",
			Name:       "Different Song",
			Artists:    "Different Artist",
			DurationMS: 180000,
			ISRC:       "USRC17607839",
			ProviderID: "metadata-b",
		},
	}

	best := selectBestReEnrichTrack(req, tracks)
	if best == nil {
		t.Fatal("expected exact ISRC candidate to be selected")
	}
	if best.ID != "same-isrc" {
		t.Fatalf("selected track = %q, want exact ISRC candidate", best.ID)
	}
}

func TestSelectBestReEnrichTrackPlaceholderFallsBackToAlbum(t *testing.T) {
	req := reEnrichRequest{
		TrackName:  "Unknown Title",
		ArtistName: "Unknown Artist",
		AlbumName:  "Harry Styles",
		DurationMs: 180000,
	}

	tracks := []ExtTrackMetadata{
		{
			ID:         "album-match",
			Name:       "Sign of the Times",
			Artists:    "Harry Styles",
			AlbumName:  "Harry Styles",
			DurationMS: 180000,
			ProviderID: "metadata-b",
		},
	}

	best := selectBestReEnrichTrack(req, tracks)
	if best == nil {
		t.Fatal("expected album-matching candidate to be selected when title/artist are placeholders")
	}
	if best.ID != "album-match" {
		t.Fatalf("selected track = %q, want album-match", best.ID)
	}
}

func TestBuildReEnrichFFmpegMetadataOmitsEmptyFields(t *testing.T) {
	req := reEnrichRequest{
		TrackName:   "Song",
		ArtistName:  "Artist",
		AlbumName:   "Album",
		AlbumArtist: "",
		ReleaseDate: "",
		TrackNumber: 0,
		DiscNumber:  0,
		ISRC:        "",
		Genre:       "",
		Label:       "",
		Copyright:   "",
	}

	metadata := buildReEnrichFFmpegMetadata(&req, "")

	if metadata["TITLE"] != "Song" {
		t.Fatalf("title = %q", metadata["TITLE"])
	}
	if metadata["ARTIST"] != "Artist" {
		t.Fatalf("artist = %q", metadata["ARTIST"])
	}
	if metadata["ALBUM"] != "Album" {
		t.Fatalf("album = %q", metadata["ALBUM"])
	}

	for _, key := range []string{
		"ALBUMARTIST",
		"DATE",
		"TRACKNUMBER",
		"DISCNUMBER",
		"ISRC",
		"GENRE",
		"ORGANIZATION",
		"COPYRIGHT",
		"LYRICS",
		"UNSYNCEDLYRICS",
	} {
		if _, exists := metadata[key]; exists {
			t.Fatalf("did not expect key %s in metadata: %#v", key, metadata)
		}
	}
}

func TestBuildReEnrichSearchQuerySkipsPlaceholderArtist(t *testing.T) {
	req := reEnrichRequest{
		TrackName:  "Sign of the Times",
		ArtistName: "Unknown Artist",
		AlbumName:  "Harry Styles",
	}

	query := buildReEnrichSearchQuery(req)
	if query != "Sign of the Times" {
		t.Fatalf("query = %q", query)
	}

	req = reEnrichRequest{
		TrackName:  "Unknown Title",
		ArtistName: "Unknown Artist",
		AlbumName:  "Harry Styles",
	}
	query = buildReEnrichSearchQuery(req)
	if query != "Harry Styles" {
		t.Fatalf("fallback album query = %q", query)
	}
}

func TestApplyReEnrichTrackMetadataCopiesComposerAndTotals(t *testing.T) {
	req := reEnrichRequest{}

	applyReEnrichTrackMetadata(&req, ExtTrackMetadata{
		Name:        "Resolved Song",
		Artists:     "Resolved Artist",
		TrackNumber: 7,
		TotalTracks: 12,
		DiscNumber:  2,
		TotalDiscs:  3,
		Composer:    "Composer",
	})

	if req.TrackNumber != 7 || req.TotalTracks != 12 {
		t.Fatalf("track metadata = %d/%d", req.TrackNumber, req.TotalTracks)
	}
	if req.DiscNumber != 2 || req.TotalDiscs != 3 {
		t.Fatalf("disc metadata = %d/%d", req.DiscNumber, req.TotalDiscs)
	}
	if req.TrackName != "Resolved Song" || req.ArtistName != "Resolved Artist" {
		t.Fatalf("basic tags = %q / %q", req.TrackName, req.ArtistName)
	}
	if req.Composer != "Composer" {
		t.Fatalf("composer = %q", req.Composer)
	}
}

func TestBuildReEnrichFFmpegMetadataFormatsTotalsAndComposer(t *testing.T) {
	req := reEnrichRequest{
		TrackNumber: 7,
		TotalTracks: 12,
		DiscNumber:  2,
		TotalDiscs:  3,
		Composer:    "Composer",
	}

	metadata := buildReEnrichFFmpegMetadata(&req, "")

	if metadata["TRACKNUMBER"] != "7/12" {
		t.Fatalf("TRACKNUMBER = %q", metadata["TRACKNUMBER"])
	}
	if metadata["DISCNUMBER"] != "2/3" {
		t.Fatalf("DISCNUMBER = %q", metadata["DISCNUMBER"])
	}
	if metadata["COMPOSER"] != "Composer" {
		t.Fatalf("COMPOSER = %q", metadata["COMPOSER"])
	}
}

func TestReEnrichGranularISRCDoesNotChangeReleaseDate(t *testing.T) {
	req := reEnrichRequest{
		ReleaseDate:  "2020-01-02",
		ISRC:         "",
		UpdateFields: []string{"isrc"},
	}

	applyReEnrichTrackMetadata(&req, ExtTrackMetadata{
		ReleaseDate: "2025-04-05",
		ISRC:        "USRC17607839",
	})

	if req.ISRC != "USRC17607839" {
		t.Fatalf("isrc = %q", req.ISRC)
	}
	if req.ReleaseDate != "2020-01-02" {
		t.Fatalf("release date = %q, want existing value", req.ReleaseDate)
	}
	metadata := buildReEnrichFFmpegMetadata(&req, "")
	if metadata["ISRC"] != "USRC17607839" {
		t.Fatalf("ISRC metadata = %q", metadata["ISRC"])
	}
	if _, exists := metadata["DATE"]; exists {
		t.Fatalf("granular ISRC update unexpectedly included DATE: %#v", metadata)
	}
}

func TestBuildReEnrichResultMetadataOnlyIncludesSelectedGranularTags(t *testing.T) {
	req := reEnrichRequest{
		TrackName:    "Existing title",
		AlbumArtist:  "Resolved album artist",
		ISRC:         "USRC17607839",
		Genre:        "Rock",
		UpdateFields: []string{"album_artist", "isrc"},
	}

	metadata := buildReEnrichResultMetadata(&req)
	if metadata["album_artist"] != "Resolved album artist" {
		t.Fatalf("album_artist = %#v", metadata["album_artist"])
	}
	if metadata["isrc"] != "USRC17607839" {
		t.Fatalf("isrc = %#v", metadata["isrc"])
	}
	for _, key := range []string{"track_name", "release_date", "genre"} {
		if _, exists := metadata[key]; exists {
			t.Fatalf("unexpected key %q in preview metadata: %#v", key, metadata)
		}
	}
}

func TestReEnrichPreviewReturnsBeforeTouchingAudioFile(t *testing.T) {
	request, err := json.Marshal(reEnrichRequest{
		FilePath:     "content://library/nonexistent.flac",
		TrackName:    "Song",
		ArtistName:   "Artist",
		ISRC:         "USRC17607839",
		UpdateFields: []string{"isrc"},
		PreviewOnly:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := ReEnrichFile(string(request))
	if err != nil {
		t.Fatalf("preview unexpectedly touched the nonexistent file: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result["method"] != "preview" || result["success"] != true {
		t.Fatalf("result = %#v", result)
	}
	metadata, ok := result["enriched_metadata"].(map[string]any)
	if !ok || metadata["isrc"] != "USRC17607839" {
		t.Fatalf("preview metadata = %#v", result["enriched_metadata"])
	}
}
