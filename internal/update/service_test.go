package update

import "testing"

func TestSelectAssetForPlatform(t *testing.T) {
	assets := []githubAsset{
		{Name: "checksums.txt"},
		{Name: "nova-v0.1.11-darwin-arm64.tar.gz", DownloadURL: "asset-api-url"},
		{Name: "nova-v0.1.11-linux-x64.tar.gz"},
	}
	asset := selectAsset(assets, "darwin-arm64")
	if asset == nil || asset.Name != "nova-v0.1.11-darwin-arm64.tar.gz" {
		t.Fatalf("unexpected asset: %#v", asset)
	}
	if got := selectAsset(assets, "windows-x64"); got != nil {
		t.Fatalf("windows asset should not match: %#v", got)
	}
}

func TestPlatformKeyNormalizesAMD64(t *testing.T) {
	if got := platformKey("darwin", "amd64"); got != "darwin-x64" {
		t.Fatalf("platformKey darwin/amd64 = %s", got)
	}
	if got := platformKey("linux", "arm64"); got != "linux-arm64" {
		t.Fatalf("platformKey linux/arm64 = %s", got)
	}
}
