package ticketfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir sets the working directory to a temp dir for the duration of the test.
// NOT safe for t.Parallel() — os.Chdir is process-global.
func chdir(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return tmp
}

func TestWrite_CreatesTicketsDir(t *testing.T) {
	tmp := chdir(t)
	_, err := Write(1, 2, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "tickets")); os.IsNotExist(err) {
		t.Fatal("tickets/ directory not created")
	}
}

func TestWrite_ReturnsCorrectFilename(t *testing.T) {
	chdir(t)
	name, err := Write(10, 20, 30, 99)
	if err != nil {
		t.Fatal(err)
	}
	want := "10-20-30-99.txt"
	if name != want {
		t.Fatalf("want %q, got %q", want, name)
	}
}

func TestWrite_FileExists(t *testing.T) {
	tmp := chdir(t)
	name, err := Write(1, 2, 3, 42)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "tickets", name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("file %s not found", path)
	}
}

func TestWrite_FileContainsExpectedFields(t *testing.T) {
	tmp := chdir(t)
	name, err := Write(5, 6, 7, 42)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "tickets", name))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"FOLIO:", "Usuario: 5", "Ingeniero: 6", "Sucursal: 7", "Ticket: 42", "Fecha:"} {
		if !strings.Contains(content, want) {
			t.Errorf("file missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestWrite_MultipleCallsCreateMultipleFiles(t *testing.T) {
	tmp := chdir(t)
	for i := int64(1); i <= 3; i++ {
		if _, err := Write(1, 2, 3, i); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(tmp, "tickets"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 files, got %d", len(entries))
	}
}

func TestWrite_IdempotentDir(t *testing.T) {
	chdir(t)
	_, err1 := Write(1, 2, 3, 1)
	_, err2 := Write(1, 2, 3, 2)
	if err1 != nil || err2 != nil {
		t.Fatalf("write errors: %v, %v", err1, err2)
	}
}

func TestWriteAt_UsesProvidedDir(t *testing.T) {
	tmp := t.TempDir()
	name, err := writeAt(tmp, 9, 8, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, name)); os.IsNotExist(err) {
		t.Fatalf("file not found in custom dir %s", tmp)
	}
}
