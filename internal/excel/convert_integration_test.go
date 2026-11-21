//go:build integration

package excel_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"tally/internal/excel"
)

func TestLibreOfficeRoundTrip(t *testing.T) {
	soffice, err := excel.FindSoffice()
	if err != nil {
		t.Fatal(err)
	}
	dir := testDirectory(t)
	source := createWorkbook(t, dir)
	profile := filepath.Join(dir, "profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(soffice,
		"--headless",
		"-env:UserInstallation=file://"+profile,
		"--convert-to", "xls",
		"--outdir", dir,
		source,
	)
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("convert to xls: %v %s", err, output)
	}
	legacy := filepath.Join(dir, "inspect.xls")
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal(err)
	}
	converted, err := (excel.Converter{
		SofficePath: soffice,
		Timeout:     2 * time.Minute,
	}).Convert(legacy, filepath.Join(dir, "converted"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := excel.InspectWorkbook(source)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := excel.InspectWorkbook(converted)
	if err != nil {
		t.Fatal(err)
	}
	if err := excel.CompareWorkbookInfo(expected, actual); err != nil {
		t.Fatal(err)
	}
}
