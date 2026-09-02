// Package docs holds an in-memory full-text index over the product
// documentation shipped inside the container image. The corpus is a few
// hundred kilobytes of Markdown, so a word index plus a substring scan is both
// fast enough and free of any external dependency.
package docs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxFragmentBytes is the size limit of a single fragment returned to the
	// caller. It keeps a search over a long guide from flooding the context of
	// the model that asked for it.
	MaxFragmentBytes = 2 * 1024

	// How much of the fragment is spent on what precedes the match.
	fragmentLeadBytes = 512

	// Weight of a whole-query substring hit relative to a single word hit.
	phraseWeight = 10

	// Documentation is small; refuse to index anything that clearly isn't it.
	maxDocumentBytes = 4 * 1024 * 1024
)

// Document is one indexed Markdown file.
type Document struct {
	// Path is relative to the root the index was loaded from.
	Path string
	// Title is the first Markdown heading of the file, if any.
	Title string

	content string
	lowered string
	// terms maps a lowercased word to how many times it occurs in the file.
	terms map[string]int
}

// Match is a document fragment relevant to a query.
type Match struct {
	Path     string  `json:"path" jsonschema:"path of the documentation file, relative to the documentation root"`
	Title    string  `json:"title,omitempty" jsonschema:"title of the documentation file"`
	Score    float64 `json:"score" jsonschema:"relevance score, higher is more relevant"`
	Fragment string  `json:"fragment" jsonschema:"fragment of the file around the match, up to 2 KB"`
}

// Index is a searchable set of documents. It is read-only once loaded and safe
// for concurrent use.
type Index struct {
	documents []*Document
}

// Load walks root and indexes every *.md file under it. A missing root is not
// an error: the service still starts, search_docs just finds nothing.
func Load(root string) (*Index, error) {
	index := &Index{}

	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}

		return nil, fmt.Errorf("can't stat docs directory: %w", err)
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("can't stat %s: %w", path, err)
		}
		if info.Size() > maxDocumentBytes {
			return nil
		}

		content, err := os.ReadFile(path) // #nosec G304 -- the path comes from walking the configured docs directory
		if err != nil {
			return fmt.Errorf("can't read %s: %w", path, err)
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}

		index.documents = append(index.documents, newDocument(filepath.ToSlash(relative), string(content)))

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(index.documents, func(i, j int) bool {
		return index.documents[i].Path < index.documents[j].Path
	})

	return index, nil
}

// Len returns the number of indexed documents.
func (ix *Index) Len() int {
	if ix == nil {
		return 0
	}

	return len(ix.documents)
}

// Search returns up to limit fragments relevant to query, most relevant first.
func (ix *Index) Search(query string, limit int) []Match {
	if ix == nil || len(ix.documents) == 0 || limit <= 0 {
		return nil
	}

	loweredQuery := strings.ToLower(strings.TrimSpace(query))
	words := tokenize(loweredQuery)

	if loweredQuery == "" {
		return nil
	}

	matches := make([]Match, 0, len(ix.documents))

	for _, doc := range ix.documents {
		score, offset := doc.score(loweredQuery, words)
		if score == 0 {
			continue
		}

		matches = append(matches, Match{
			Path:     doc.Path,
			Title:    doc.Title,
			Score:    score,
			Fragment: doc.fragment(offset),
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}

		return matches[i].Path < matches[j].Path
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	return matches
}

func newDocument(path, content string) *Document {
	lowered := strings.ToLower(content)

	doc := &Document{
		Path:    path,
		Title:   firstHeading(content),
		content: content,
		lowered: lowered,
		terms:   make(map[string]int),
	}

	for _, word := range tokenize(lowered) {
		doc.terms[word]++
	}

	return doc
}

// score rates the document against the query and reports the byte offset the
// fragment should be centred on. A whole-query substring hit outweighs
// scattered word hits, so that "kill pod" prefers the page that spells it out.
func (d *Document) score(query string, words []string) (score float64, offset int) {
	offset = -1

	if at := strings.Index(d.lowered, query); at >= 0 {
		score += phraseWeight
		offset = at
	}

	var best int

	for _, word := range words {
		count := d.terms[word]
		if count == 0 {
			// The word may still occur as a part of a longer one: Markdown is
			// full of paths and identifiers that no tokenizer splits the way a
			// human would.
			if at := strings.Index(d.lowered, word); at >= 0 {
				score += 0.5
				if offset < 0 {
					offset = at
				}
			}

			continue
		}

		score += 1 + float64(count)/10

		// The rarest matched word is the most telling place to quote from.
		if offset < 0 || (best != 0 && count < best) {
			if at := strings.Index(d.lowered, word); at >= 0 {
				offset = at
				best = count
			}
		}
	}

	if score > 0 && offset < 0 {
		offset = 0
	}

	// The title is what a reader scans first, so a hit there is worth as much
	// as a handful of hits in the body.
	if d.Title != "" && strings.Contains(strings.ToLower(d.Title), query) {
		score += phraseWeight / 2
	}

	return score, offset
}

// fragment cuts up to MaxFragmentBytes of the document around offset, snapped
// to line boundaries so that Markdown structure survives the cut.
func (d *Document) fragment(offset int) string {
	if offset < 0 || offset > len(d.content) {
		offset = 0
	}

	start := offset - fragmentLeadBytes
	if start < 0 {
		start = 0
	} else if at := strings.IndexByte(d.content[start:offset], '\n'); at >= 0 {
		start += at + 1
	}

	end := start + MaxFragmentBytes
	if end >= len(d.content) {
		return d.content[start:]
	}

	if at := strings.LastIndexByte(d.content[start:end], '\n'); at > 0 {
		end = start + at
	} else {
		// A single line longer than the limit: back off to a rune boundary so
		// that the fragment stays valid UTF-8.
		for end > start && !utf8.RuneStart(d.content[end]) {
			end--
		}
	}

	return d.content[start:end]
}

func firstHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}

	return ""
}

// tokenize splits text into lowercased words. Letters and digits of any script
// are kept together, everything else separates: the documentation is written in
// both Russian and English and is full of identifiers like CS_RT_CRYPTOMINER.
func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
}
