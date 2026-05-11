package utils

import (
	"bufio"
	"math"
	"strings"
	"unicode/utf8"
)

// ChunkRecursive splits text recursively by paragraph -> line -> sentence -> char
func ChunkRecursive(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if overlap <= 0 {
		overlap = 50
	}
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end - overlap
		if start >= len(runes) {
			break
		}
	}
	return chunks
}

// ChunkLarge creates large chunks (1000 chars, 100 overlap)
func ChunkLarge(text string) []string {
	return ChunkRecursive(text, 1000, 100)
}

// ChunkSmall creates small chunks (200 chars, 30 overlap)
func ChunkSmall(text string) []string {
	return ChunkRecursive(text, 200, 30)
}

// ChunkParagraph splits by double newlines
func ChunkParagraph(text string) []string {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			chunks = append(chunks, p)
		}
	}
	return chunks
}

// ChunkToken splits by approximate token count
func ChunkToken(text string, maxTokens int) []string {
	if maxTokens <= 0 {
		maxTokens = 500
	}
	charsPerToken := 2.0
	maxChars := int(float64(maxTokens) * charsPerToken)
	return ChunkRecursive(text, maxChars, int(charsPerToken*10))
}

// ChunkSemantic splits by detecting topic boundaries using simple heuristics
func ChunkSemantic(text string, _ int, _ int) []string {
	lines := strings.Split(text, "\n")
	var chunks []string
	var current strings.Builder
	currentRunes := 0
	targetSize := 300

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
				currentRunes = 0
			}
			continue
		}

		lineRunes := utf8.RuneCountInString(line)
		if currentRunes+lineRunes > targetSize*2 && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentRunes = 0
		}

		if current.Len() > 0 {
			current.WriteString("\n")
			currentRunes++
		}
		current.WriteString(line)
		currentRunes += lineRunes
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// EstimateTokens estimates token count for a text (Chinese + English)
func EstimateTokens(text string) int {
	runes := []rune(text)
	chineseCount := 0
	for _, r := range runes {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}
	return chineseCount*2 + (len(runes)-chineseCount)/4 + 1
}

// Document represents a processed document
type Document struct {
	ID       string
	Content  string
	Metadata map[string]string
}

// DocumentProcessor handles document loading and chunking
type DocumentProcessor struct {
	chunkSize    int
	chunkOverlap int
}

func NewDocumentProcessor(chunkSize, chunkOverlap int) *DocumentProcessor {
	return &DocumentProcessor{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

func (p *DocumentProcessor) ProcessFile(content, filename string) ([]Document, []string, error) {
	original := Document{
		ID:      "doc_" + filename,
		Content: content,
		Metadata: map[string]string{
			"filename": filename,
		},
	}

	chunks := ChunkRecursive(content, p.chunkSize, p.chunkOverlap)
	return []Document{original}, chunks, nil
}

type SearchTester struct {
	topK int
}

func NewSearchTester(topK int) *SearchTester {
	return &SearchTester{topK: topK}
}

type TestCase struct {
	Document       string `json:"document"`
	Query          string `json:"query"`
	ExpectedInTopK int    `json:"expected_in_top_k"`
	Description    string `json:"description"`
}

type TestResult struct {
	TestCase TestCase `json:"test_case"`
	Found    bool     `json:"found"`
	Position *int     `json:"position,omitempty"`
	Score    *float64 `json:"score,omitempty"`
	Passed   bool     `json:"passed"`
}

type PrecisionReport struct {
	TotalTests     int          `json:"total_tests"`
	PassedTests    int          `json:"passed_tests"`
	PrecisionScore float64      `json:"precision_score"`
	AvgPosition    float64      `json:"average_position"`
	Results        []TestResult `json:"results"`
}

func (t *SearchTester) RunPrecisionTest(cases []TestCase, searchFn func(query string, topK int) ([]string, error)) (*PrecisionReport, error) {
	var results []TestResult
	passed := 0

	for _, tc := range cases {
		// Search
		docs, err := searchFn(tc.Query, t.topK)
		if err != nil {
			continue
		}

		found := false
		var pos int
		var score float64
		expectedDoc := tc.Document

		for i, doc := range docs {
			sim := cosineSimilarity(expectedDoc, doc)
			if sim > 0.5 {
				found = true
				pos = i + 1
				score = sim
				break
			}
		}

		testPassed := found && pos <= tc.ExpectedInTopK
		if testPassed {
			passed++
		}

		posCopy := pos
		results = append(results, TestResult{
			TestCase: tc,
			Found:    found,
			Position: &posCopy,
			Score:    &score,
			Passed:   testPassed,
		})
	}

	precision := float64(0)
	avgPos := float64(0)
	if len(cases) > 0 {
		precision = float64(passed) / float64(len(cases))
		totalPos := 0
		for _, r := range results {
			if r.Position != nil {
				totalPos += *r.Position
			}
		}
		avgPos = float64(totalPos) / float64(len(results))
	}

	return &PrecisionReport{
		TotalTests:     len(cases),
		PassedTests:    passed,
		PrecisionScore: precision,
		AvgPosition:    avgPos,
		Results:        results,
	}, nil
}

func GetDefaultTestCases() []TestCase {
	return []TestCase{
		{
			Document:       "RAG (Retrieval Augmented Generation) combines retrieval with LLM generation",
			Query:          "什么是RAG",
			ExpectedInTopK: 3,
			Description:    "RAG 概念检索",
		},
		{
			Document:       "LangChain is a framework for building LLM-powered applications",
			Query:          "LangChain 框架",
			ExpectedInTopK: 3,
			Description:    "LangChain 检索",
		},
	}
}

func cosineSimilarity(a, b string) float64 {
	wordsA := words(a)
	wordsB := words(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	allWords := make(map[string]bool)
	for _, w := range wordsA {
		allWords[w] = true
	}
	for _, w := range wordsB {
		allWords[w] = true
	}

	vecA := make([]float64, len(allWords))
	vecB := make([]float64, len(allWords))
	i := 0
	for w := range allWords {
		for _, wa := range wordsA {
			if wa == w {
				vecA[i]++
			}
		}
		for _, wb := range wordsB {
			if wb == w {
				vecB[i]++
			}
		}
		i++
	}

	dotProduct := 0.0
	magnitudeA := 0.0
	magnitudeB := 0.0
	for i := range vecA {
		dotProduct += vecA[i] * vecB[i]
		magnitudeA += vecA[i] * vecA[i]
		magnitudeB += vecB[i] * vecB[i]
	}

	if magnitudeA == 0 || magnitudeB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(magnitudeA) * math.Sqrt(magnitudeB))
}

func words(text string) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		result = append(result, strings.ToLower(scanner.Text()))
	}
	return result
}

// File type detection
func IsSupportedFile(filename string, supportedTypes []string) bool {
	for _, ext := range supportedTypes {
		if strings.HasSuffix(filename, "."+ext) {
			return true
		}
	}
	return false
}
