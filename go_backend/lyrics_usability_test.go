package gobackend

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSharedLyricsUsabilityCases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "android", "app", "src", "test", "resources", "lyrics_usability_cases.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	decode := strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\t`, "\t")
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("invalid shared fixture: %q", line)
		}
		t.Run(fields[0], func(t *testing.T) {
			want, err := strconv.ParseBool(fields[1])
			if err != nil {
				t.Fatal(err)
			}
			if got := rawLyricsHasUsableContent(decode.Replace(fields[2])); got != want {
				t.Errorf("rawLyricsHasUsableContent(%q) = %v, want %v", fields[2], got, want)
			}
		})
	}
}
