package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestZipListAndExtract(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "Vitality vs MOUZ.zip")
	f, _ := os.Create(zp)
	z := zip.NewWriter(f)
	for _, n := range []string{"readme.txt", "vitality-vs-mouz-m1-mirage.dem", "vitality-vs-mouz-m2-inferno.dem"} {
		w, _ := z.Create(n)
		w.Write([]byte(n))
	}
	z.Close()
	f.Close()

	files, err := listFiles(zp)
	if err != nil || len(files) != 2 {
		t.Fatalf("listFiles = %v, %v", files, err)
	}
	dst := filepath.Join(dir, "out", "x.dem")
	if err := extract(zp, files[1], dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != files[1] {
		t.Fatalf("extracted %q", b)
	}
	if err := extract(zp, "missing.dem", dst+"2"); err == nil {
		t.Fatal("want error for missing file")
	}
	if got := shortName("Vitality vs MOUZ - m1 - Mirage.dem"); got != "vitality_vs_mouz_m1_mirage" {
		t.Fatalf("shortName = %q", got)
	}
}
