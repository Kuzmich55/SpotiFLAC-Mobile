package gobackend

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQualitySizeParametersSurviveManifestRoundTrip(t *testing.T) {
	const source = `{"name":"sample-audio","version":"1.0.0","description":"Sample","type":["download_provider"],"qualityOptions":[{"id":"studio","label":"Studio","kind":"lossless","sizeEstimate":{"bitDepth":24,"sampleRate":96000,"channels":2,"isMaximum":true}},{"id":"compact","label":"Compact","sizeEstimate":{"bitrateKbps":256}},{"id":"legacy","label":"Legacy"}]}`
	manifest, err := ParseManifest([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest.QualityOptions)
	if err != nil {
		t.Fatal(err)
	}
	var options []QualityOption
	if err := json.Unmarshal(encoded, &options); err != nil {
		t.Fatal(err)
	}
	want := QualitySizeEstimate{BitDepth: 24, SampleRate: 96000, Channels: 2, IsMaximum: true}
	if options[0].SizeEstimate == nil || *options[0].SizeEstimate != want {
		t.Fatalf("lossless parameters lost: %+v", options[0].SizeEstimate)
	}
	if options[1].SizeEstimate == nil || options[1].SizeEstimate.BitrateKbps != 256 {
		t.Fatalf("bitrate lost: %+v", options[1].SizeEstimate)
	}
	if options[2].SizeEstimate != nil || strings.Contains(string(encoded), `"sizeEstimate":null`) {
		t.Fatalf("legacy quality acquired size parameters: %s", encoded)
	}
}
