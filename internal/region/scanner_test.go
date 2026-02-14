package region

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractRegionPath(t *testing.T) {
	tests := []struct {
		line     string
		tag      string
		wantPath string
		wantOk   bool
	}{
		{"// @region:app.search.sources", "region", "app.search.sources", true},
		{"// @endregion:app.search.sources", "endregion", "app.search.sources", true},
		{"# @region:app.billing", "region", "app.billing", true},
		{"# @endregion:app.billing", "endregion", "app.billing", true},
		{"-- @region:app.db.migrations", "region", "app.db.migrations", true},
		{"<!-- @region:app.web.templates -->", "region", "app.web.templates", true},
		{"<!-- @endregion:app.web.templates -->", "endregion", "app.web.templates", true},
		{"/* @region:app.styles.main */", "region", "app.styles.main", true},
		{"normal code line", "region", "", false},
		{"", "region", "", false},
		{"// some comment", "region", "", false},
	}

	for _, tt := range tests {
		gotPath, gotOk := extractRegionPath(tt.line, tt.tag)
		if gotPath != tt.wantPath || gotOk != tt.wantOk {
			t.Errorf("extractRegionPath(%q, %q) = (%q, %v), want (%q, %v)",
				tt.line, tt.tag, gotPath, gotOk, tt.wantPath, tt.wantOk)
		}
	}
}

func TestGetCommentPrefix(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"main.go", "//"},
		{"script.py", "#"},
		{"query.sql", "--"},
		{"style.css", "/*"},
		{"page.html", "<!--"},
		{"app.vue", "<!--"},
		{"unknown.xyz", "//"},
	}

	for _, tt := range tests {
		got := GetCommentPrefix(tt.filename)
		if got != tt.want {
			t.Errorf("GetCommentPrefix(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestGetRegionTag(t *testing.T) {
	tests := []struct {
		path     string
		filename string
		want     string
	}{
		{"app.search", "main.go", "// @region:app.search"},
		{"app.search", "script.py", "# @region:app.search"},
		{"app.search", "query.sql", "-- @region:app.search"},
		{"app.search", "style.css", "/* @region:app.search */"},
		{"app.search", "page.html", "<!-- @region:app.search -->"},
	}

	for _, tt := range tests {
		got := GetRegionTag(tt.path, tt.filename)
		if got != tt.want {
			t.Errorf("GetRegionTag(%q, %q) = %q, want %q", tt.path, tt.filename, got, tt.want)
		}
	}
}

func TestScanFile(t *testing.T) {
	dir := t.TempDir()

	// Create a test file with region markers
	content := `// @region:app.search
package search

// @region:app.search.adapter
func Query() {}
// @endregion:app.search.adapter

// @region:app.search.parser
func Parse() {}
// @endregion:app.search.parser

// @endregion:app.search
`
	file := filepath.Join(dir, "search.go")
	os.WriteFile(file, []byte(content), 0644)

	markers, warnings, err := ScanFile(file)
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}

	if len(markers) != 3 {
		t.Fatalf("expected 3 markers, got %d", len(markers))
	}

	// Check outer region
	if markers[0].Path != "app.search" {
		t.Errorf("expected app.search, got %s", markers[0].Path)
	}
	if markers[0].StartLine != 1 || markers[0].EndLine != 12 {
		t.Errorf("expected lines 1-12, got %d-%d", markers[0].StartLine, markers[0].EndLine)
	}

	// Check nested regions
	if markers[1].Path != "app.search.adapter" {
		t.Errorf("expected app.search.adapter, got %s", markers[1].Path)
	}
	if markers[2].Path != "app.search.parser" {
		t.Errorf("expected app.search.parser, got %s", markers[2].Path)
	}
}

func TestScanFileWarnings(t *testing.T) {
	dir := t.TempDir()

	// Unclosed region
	content := `// @region:app.unclosed
package test
`
	file := filepath.Join(dir, "bad.go")
	os.WriteFile(file, []byte(content), 0644)

	_, warnings, err := ScanFile(file)
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "never closed") {
		t.Errorf("expected 'never closed' warning, got: %s", warnings[0])
	}
}

func TestBuildTree(t *testing.T) {
	markers := []*RegionMarker{
		{Path: "app.search", File: "search.go", StartLine: 1, EndLine: 50},
		{Path: "app.search.adapter", File: "search.go", StartLine: 5, EndLine: 20},
		{Path: "app.search.parser", File: "search.go", StartLine: 22, EndLine: 40},
		{Path: "app.billing", File: "billing.go", StartLine: 1, EndLine: 30},
	}

	tree := BuildTree(markers)

	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 root child (app), got %d", len(tree.Children))
	}

	app := tree.Children[0]
	if app.Name != "app" {
		t.Errorf("expected 'app', got %s", app.Name)
	}
	if len(app.Children) != 2 {
		t.Fatalf("expected 2 children (billing, search), got %d", len(app.Children))
	}
}

func TestFormatTree(t *testing.T) {
	markers := []*RegionMarker{
		{Path: "app.search", File: "search.go", StartLine: 1, EndLine: 50},
		{Path: "app.billing", File: "billing.go", StartLine: 1, EndLine: 30},
	}

	tree := BuildTree(markers)
	output := FormatTree(tree, "", true)

	if !strings.Contains(output, "search") {
		t.Error("tree output should contain 'search'")
	}
	if !strings.Contains(output, "billing") {
		t.Error("tree output should contain 'billing'")
	}
}

func TestScaffoldRegion(t *testing.T) {
	dir := t.TempDir()

	// Test creating a new Go file with region markers
	file := filepath.Join(dir, "src", "search.go")
	err := ScaffoldRegion(file, "app.search")
	if err != nil {
		t.Fatalf("ScaffoldRegion error: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "// @region:app.search") {
		t.Error("file should contain region start marker")
	}
	if !strings.Contains(content, "// @endregion:app.search") {
		t.Error("file should contain region end marker")
	}
	if !strings.Contains(content, "package src") {
		t.Error("Go file should contain package declaration")
	}

	// Test idempotency — scaffolding again shouldn't duplicate
	err = ScaffoldRegion(file, "app.search")
	if err != nil {
		t.Fatalf("ScaffoldRegion (idempotent) error: %v", err)
	}

	data2, _ := os.ReadFile(file)
	if string(data2) != string(data) {
		t.Error("scaffolding an existing region should be idempotent")
	}
}

func TestScaffoldRegionPython(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "handler.py")

	err := ScaffoldRegion(file, "app.handler")
	if err != nil {
		t.Fatalf("ScaffoldRegion error: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# @region:app.handler") {
		t.Error("Python file should use # comment prefix for region markers")
	}
}

func TestFileHasRegionMarkers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	os.WriteFile(file, []byte("// @region:app.test\ncode\n// @endregion:app.test\n"), 0644)

	if !FileHasRegionMarkers(file, "app.test") {
		t.Error("should find region markers")
	}
	if FileHasRegionMarkers(file, "app.other") {
		t.Error("should not find non-existent region")
	}
}

func TestParseArchMdLegacy(t *testing.T) {
	dir := t.TempDir()
	content := `# Architecture
# @region:app
# @endregion:app
# @region:app.search
# @endregion:app.search
# @region:app.billing
# @endregion:app.billing
`
	os.WriteFile(filepath.Join(dir, "arch.md"), []byte(content), 0644)

	paths, err := ParseArchMd(dir)
	if err != nil {
		t.Fatalf("ParseArchMd error: %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d: %v", len(paths), paths)
	}
}

func TestParseArchMdDotwalk(t *testing.T) {
	dir := t.TempDir()
	content := `# Architecture
# Namespace dotwalk format

app                          # Root application namespace
app.search                   # Search subsystem
app.search.sources           # Source management
app.search.sources.btv2      # BT v2 adapter
app.billing                  # Billing
`
	os.WriteFile(filepath.Join(dir, "arch.md"), []byte(content), 0644)

	paths, err := ParseArchMd(dir)
	if err != nil {
		t.Fatalf("ParseArchMd error: %v", err)
	}

	if len(paths) != 5 {
		t.Errorf("expected 5 paths, got %d: %v", len(paths), paths)
	}

	entries, _ := ParseArchMdEntries(dir)
	found := false
	for _, e := range entries {
		if e.Path == "app.search" && e.Comment == "Search subsystem" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find app.search with comment 'Search subsystem'")
	}
}

func TestIsValidNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"app", true},
		{"app.search", true},
		{"app.search.sources.btv2", true},
		{"app_core", true},
		{"", false},
		{"123", false},
		{"app..search", false},
		{".app", false},
		{"app.", false},
		{"app search", false},
	}

	for _, tt := range tests {
		got := isValidNamespace(tt.input)
		if got != tt.want {
			t.Errorf("isValidNamespace(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestValidateArchNamespaces(t *testing.T) {
	dir := t.TempDir()

	// Valid hierarchy
	content := `# Architecture
app
app.search
app.search.sources
app.billing
`
	os.WriteFile(filepath.Join(dir, "arch.md"), []byte(content), 0644)

	issues := ValidateArchNamespaces(dir)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %v", issues)
	}

	// Missing parent
	content2 := `# Architecture
app
app.search.sources
`
	os.WriteFile(filepath.Join(dir, "arch.md"), []byte(content2), 0644)

	issues2 := ValidateArchNamespaces(dir)
	if len(issues2) != 1 {
		t.Errorf("expected 1 issue (missing parent), got %d: %v", len(issues2), issues2)
	}
	if len(issues2) > 0 && !strings.Contains(issues2[0], "no parent") {
		t.Errorf("expected 'no parent' message, got: %s", issues2[0])
	}
}

func TestParseGamignore(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment
vendor/
*.pb.go
testdata/
`
	os.WriteFile(filepath.Join(dir, ".gamignore"), []byte(content), 0644)

	patterns := ParseGamignore(dir)
	if len(patterns) != 3 {
		t.Errorf("expected 3 patterns, got %d: %v", len(patterns), patterns)
	}
}

func TestIsIgnored(t *testing.T) {
	patterns := []string{"vendor/", "*.pb.go", "testdata/"}

	tests := []struct {
		path string
		want bool
	}{
		{"vendor/lib.go", true},
		{"types.pb.go", true},
		{"testdata/fixture.json", true},
		{"src/main.go", false},
		{"internal/service.go", false},
	}

	for _, tt := range tests {
		got := isIgnored(tt.path, patterns)
		if got != tt.want {
			t.Errorf("isIgnored(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
