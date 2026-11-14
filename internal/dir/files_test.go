package dir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindNearestFile_InCurrentDir(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "glock.yml")
	if _, err := os.Create(target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	found, err := FindNearestFile(tmp, 5, "glock.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != target {
		t.Errorf("wanted %s, got %s", target, found)
	}
}

func TestFindNearestFile_InParentDir(t *testing.T) {
	tmp := t.TempDir()
	child := filepath.Join(tmp, "child")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}

	target := filepath.Join(tmp, "glock.yml")
	if _, err := os.Create(target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	found, err := FindNearestFile(child, 5, "glock.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != target {
		t.Errorf("wanted %s, got %s", target, found)
	}
}

func TestFindNearestFile_NotFoundWithinHopLimit(t *testing.T) {
	tmp := t.TempDir()
	child := filepath.Join(tmp, "child")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}

	target := filepath.Join(tmp, "glock.yml")
	if _, err := os.Create(target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	_, err := FindNearestFile(child, 0, "glock.yml")
	if err == nil {
		t.Fatal("expected error when hop limit is 0")
	}
}

func TestFindNearestFile_NotFoundAtAll(t *testing.T) {
	tmp := t.TempDir()

	_, err := FindNearestFile(tmp, 3, "missing-file.txt")
	if err == nil {
		t.Fatal("expected error when file does not exist")
	}
}

func TestFindNearestFile_StopsAtRoot(t *testing.T) {
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)

	_, err := FindNearestFile(root, 10, "nonexistent.yml")
	if err == nil {
		t.Fatal("expected error when reaching filesystem root")
	}
}
