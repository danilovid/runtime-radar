package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func writeDocs(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"quickstart/quickstart.md": "# Quickstart\n\nRuntime Radar monitors containers with Tetragon.\n" +
			"Install the Helm chart and open the UI.\n",
		"guides/detectors/guide.md": "# Detector guide\n\n" +
			strings.Repeat("A detector is a program written in Go that looks for threats in runtime events.\n", 60) +
			"\n## Cryptominer\n\nThe CS_RT_CRYPTOMINER detector finds known miners.\n" +
			strings.Repeat("More prose about detectors and TinyGo.\n", 60),
		"guides/help/help.md": "# Справка\n\nРаздел «События рантайма» показывает события, собранные Tetragon.\n",
		"notes.txt":           "this file is not markdown and must be ignored",
	}

	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("can't create %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("can't write %s: %v", path, err)
		}
	}

	return root
}

func TestLoadIndexesMarkdownOnly(t *testing.T) {
	t.Parallel()

	index, err := Load(writeDocs(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if index.Len() != 3 {
		t.Fatalf("indexed %d documents, want 3 markdown files", index.Len())
	}
}

func TestLoadMissingRootIsNotAnError(t *testing.T) {
	t.Parallel()

	index, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if index.Len() != 0 || len(index.Search("anything", 5)) != 0 {
		t.Error("an empty index must simply find nothing")
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	index, err := Load(writeDocs(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name     string
		query    string
		wantPath string
	}{
		{name: "identifier", query: "CS_RT_CRYPTOMINER", wantPath: "guides/detectors/guide.md"},
		{name: "english phrase", query: "install the helm chart", wantPath: "quickstart/quickstart.md"},
		{name: "russian words", query: "события рантайма", wantPath: "guides/help/help.md"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			matches := index.Search(tc.query, 5)
			if len(matches) == 0 {
				t.Fatalf("query %q found nothing", tc.query)
			}

			if matches[0].Path != tc.wantPath {
				t.Errorf("best match = %q, want %q", matches[0].Path, tc.wantPath)
			}
			if matches[0].Fragment == "" {
				t.Error("best match has an empty fragment")
			}
		})
	}
}

func TestSearchBoundsFragments(t *testing.T) {
	t.Parallel()

	index, err := Load(writeDocs(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, match := range index.Search("detector", 5) {
		if len(match.Fragment) > MaxFragmentBytes {
			t.Errorf("%s: fragment is %d bytes, want at most %d", match.Path, len(match.Fragment), MaxFragmentBytes)
		}
		if !utf8.ValidString(match.Fragment) {
			t.Errorf("%s: fragment is not valid UTF-8", match.Path)
		}
	}
}

func TestSearchLimitAndRelevance(t *testing.T) {
	t.Parallel()

	index, err := Load(writeDocs(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := index.Search("Tetragon", 1); len(got) != 1 {
		t.Fatalf("matches = %d, want the limit to be honoured", len(got))
	}

	matches := index.Search("Tetragon", 5)
	if len(matches) < 2 {
		t.Fatalf("matches = %d, want the word to be found in several documents", len(matches))
	}
	for i := 1; i < len(matches); i++ {
		if matches[i-1].Score < matches[i].Score {
			t.Errorf("matches are not sorted by score: %v", matches)
		}
	}

	if got := index.Search("", 5); len(got) != 0 {
		t.Error("an empty query must find nothing")
	}
	if got := index.Search("wireguard", 5); len(got) != 0 {
		t.Error("a word that occurs nowhere must find nothing")
	}
}

func TestSearchFragmentCarriesTheMatch(t *testing.T) {
	t.Parallel()

	index, err := Load(writeDocs(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches := index.Search("CS_RT_CRYPTOMINER", 1)
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}

	// The match sits in the middle of a long file: the fragment must be cut
	// around it rather than taken from the top of the document.
	if !strings.Contains(matches[0].Fragment, "CS_RT_CRYPTOMINER") {
		t.Errorf("fragment does not contain the match: %q", matches[0].Fragment)
	}
	if matches[0].Title != "Detector guide" {
		t.Errorf("title = %q, want the first heading of the file", matches[0].Title)
	}
}
