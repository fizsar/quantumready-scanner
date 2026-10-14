package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// labMarker is a string that exists only in the -lab code of cmd/qrscan
// (cmd/qrscan/lab.go). Keep both in sync.
const labMarker = "qrscan-lab-addr-policy"

func TestScannerOptions(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want scanner.Intensity
		err  bool
	}{
		{nil, scanner.IntensityFull, false},
		{map[string]string{"SCAN_INTENSITY": "basic"}, scanner.IntensityBasic, false},
		{map[string]string{"SCAN_INTENSITY": "full"}, scanner.IntensityFull, false},
		{map[string]string{"SCAN_INTENSITY": "lab"}, "", true},
		// Nothing in the environment can relax the address policy.
		{map[string]string{"SCAN_LAB": "1", "QRSCAN_LAB": "1", "LAB": "1", "SCAN_ALLOW_PRIVATE": "1"}, scanner.IntensityFull, false},
	} {
		opts, err := scannerOptions(func(k string) string { return tc.env[k] })
		if (err != nil) != tc.err || opts.Intensity != tc.want {
			t.Errorf("env %v: intensity %q, err %v", tc.env, opts.Intensity, err)
		}
		if opts.AddrPolicy != nil || opts.Resolver != nil || opts.RootCAs != nil {
			t.Errorf("env %v: scanner options changed beyond the intensity: %+v", tc.env, opts)
		}
	}
}

// TestLabCodeNotInProductionBinaries builds the production binaries and
// checks that the -lab code of cmd/qrscan is not in them (and that the check
// works, by finding it in qrscan). It also checks that the image never builds
// qrscan.
func TestLabCodeNotInProductionBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, cmd := range []string{"api", "worker", "migrate", "qrscan"} {
		out := filepath.Join(dir, cmd)
		build := exec.Command("go", "build", "-trimpath", "-o", out, "./cmd/"+cmd)
		build.Dir = root
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", cmd, err, b)
		}
		bin, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		has := bytes.Contains(bin, []byte(labMarker))
		if cmd == "qrscan" && !has {
			t.Errorf("marker not found in qrscan: the check would not detect the lab code")
		}
		if cmd != "qrscan" && has {
			t.Errorf("the -lab code of qrscan is in the %s binary", cmd)
		}
	}
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dockerfile), "qrscan") || strings.Contains(string(dockerfile), "./cmd/...") {
		t.Error("the Dockerfile must build only api, migrate and worker, never qrscan")
	}
}
