package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadToolReadsFileWithNumberedLines(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "notes.txt", "alpha\nbeta\n")

	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{"file_path": p})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("expected success, got error result: %+v", got)
	}
	if want := "1: alpha\n2: beta"; got.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Content)
	}
}

func TestReadToolRespectsOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "notes.txt", "a\nb\nc\nd\n")

	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{
		"file_path": p,
		"offset":    2,
		"limit":     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("expected success, got error result: %+v", got)
	}
	want := "2: b\n3: c\n\n[1 more lines in file. Use offset=4 to continue.]"
	if got.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Content)
	}
}

func TestReadToolReadsRelativePathFromWorkingDir(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "notes.txt", "alpha\n")

	got, err := (&ReadTool{WorkingDir: dir}).Run(context.Background(), map[string]interface{}{"file_path": "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("expected success, got error result: %+v", got)
	}
	if want := "1: alpha"; got.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Content)
	}
}

func TestReadToolResolvesRelativePathAgainstConfiguredWorkingDir(t *testing.T) {
	base := t.TempDir()
	other := filepath.Join(base, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTempFile(t, other, "notes.txt", "from-working-dir\n")

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	got, err := (&ReadTool{WorkingDir: other}).Run(context.Background(), map[string]interface{}{"file_path": "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("expected success, got error result: %+v", got)
	}
	if want := "1: from-working-dir"; got.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Content)
	}
}

func TestReadToolRejectsDirectoryPaths(t *testing.T) {
	dir := t.TempDir()
	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{"file_path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsError || got.Content != "path is a directory, not a file" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestReadToolOffsetBeyondEOFReturnsUserFacingError(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "notes.txt", "only\n")

	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{"file_path": p, "offset": 2})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsError || got.Content != "offset 2 is beyond end of file (1 lines total)" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestReadToolRejectsInvalidOffsetAndLimitValues(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "notes.txt", "x\n")

	cases := []map[string]interface{}{
		{"file_path": p, "offset": 0},
		{"file_path": p, "limit": 0},
		{"file_path": p, "offset": 1.5},
		{"file_path": p, "limit": 1.5},
	}
	for _, args := range cases {
		got, err := (&ReadTool{}).Run(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsError {
			t.Fatalf("expected error result for args=%v, got %+v", args, got)
		}
	}
}

func TestReadToolEmptyFileReturnsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "empty.txt", "")

	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{"file_path": p})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError || got.Content != "" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestReadToolTruncatesLargeOutputAndIncludesContinuationHint(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 2005)
	for i := range lines {
		lines[i] = "x"
	}
	p := writeTempFile(t, dir, "huge.txt", strings.Join(lines, "\n")+"\n")

	got, err := (&ReadTool{}).Run(context.Background(), map[string]interface{}{"file_path": p})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("expected truncation success, got error result: %+v", got)
	}
	if !strings.Contains(got.Content, "[5 more lines in file. Use offset=2001 to continue.]") {
		t.Fatalf("expected continuation hint, got %q", got.Content)
	}
	if !strings.HasPrefix(got.Content, "1: x") {
		t.Fatalf("expected numbered content, got %q", got.Content)
	}
}
