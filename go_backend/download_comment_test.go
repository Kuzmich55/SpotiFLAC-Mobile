package gobackend

import "testing"

func TestBuildDownloadedFileCommentKeepsSourceComment(t *testing.T) {
	const source = "https://source.example/album/example/123"
	got := buildDownloadedFileComment(source, "https://provider.example/albums/example")
	if got != source {
		t.Fatalf("comment = %q, want %q", got, source)
	}
}

func TestBuildDownloadedFileCommentUsesProviderCommentWhenSourceIsEmpty(t *testing.T) {
	const provider = "https://provider.example/albums/example"
	got := buildDownloadedFileComment("", provider)
	if got != provider {
		t.Fatalf("comment = %q, want %q", got, provider)
	}
}

func TestBuildDownloadedFileCommentStaysEmptyWithoutProviderComment(t *testing.T) {
	if got := buildDownloadedFileComment("", ""); got != "" {
		t.Fatalf("comment = %q, want empty", got)
	}
}

func TestBuildDownloadedFileCommentTrimsWhitespace(t *testing.T) {
	const original = "https://example.test/album/1"
	if got := buildDownloadedFileComment("  "+original+"\r\n", ""); got != original {
		t.Fatalf("comment = %q, want %q", got, original)
	}
}
