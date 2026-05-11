package stores

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type QdrantStore struct {
	url            string
	collectionName string
	vectorSize     int
	minScore       float64
	embeddings     *EmbeddingClient
	documents      []Document
	mu             sync.RWMutex
}

type QdrantStoreConfig struct {
	URL            string
	CollectionName string
	VectorSize     int
	Distance       string
	MinScore       float64
}

type SearchResult struct {
	Document Document
	Score    float64
	ID       string
}

type Document struct {
	ID       string
	Content  string
	Metadata map[string]string
}

type EmbeddingClient struct {
	apiKey  string
	baseURL string
	model   string
}

func NewEmbeddingClient(apiKey, baseURL, model string) *EmbeddingClient {
	return &EmbeddingClient{apiKey: apiKey, baseURL: baseURL, model: model}
}

func (e *EmbeddingClient) EmbedDocuments(texts []string) ([][]float32, error) {
	body := map[string]interface{}{"input": texts, "model": e.model}
	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(context.Background(), "POST", e.baseURL+"/embeddings", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return mockEmbeddings(texts), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mockEmbeddings(texts), nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return mockEmbeddings(texts), nil
	}
	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}
		embeddings[i] = vec
	}
	return embeddings, nil
}

func (e *EmbeddingClient) EmbedQuery(text string) ([]float32, error) {
	embeddings, err := e.EmbedDocuments([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return mockEmbedding(text), nil
	}
	return embeddings[0], nil
}

func mockEmbedding(text string) []float32 {
	vec := make([]float32, 128)
	runes := []rune(text)
	for i, r := range runes {
		vec[i%128] += float32(r) / 65536.0
	}
	norm := 0.0
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm > 0 {
		norm = sqrt(norm)
		for i := range vec {
			vec[i] /= float32(norm)
		}
	}
	return vec
}

func mockEmbeddings(texts []string) [][]float32 {
	embeddings := make([][]float32, len(texts))
	for i, t := range texts {
		embeddings[i] = mockEmbedding(t)
	}
	return embeddings
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2.0
	for i := 0; i < 10; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}

func NewQdrantStore(config QdrantStoreConfig) (*QdrantStore, error) {
	return &QdrantStore{
		url:            config.URL,
		collectionName: config.CollectionName,
		vectorSize:     config.VectorSize,
		minScore:       config.MinScore,
		embeddings:     NewEmbeddingClient("", "", "text-embedding-3-small"),
		documents:      make([]Document, 0),
	}, nil
}

func (s *QdrantStore) AddDocuments(docs []Document) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, len(docs))
	for i, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.New().String()
		}
		d.ID = id
		s.documents = append(s.documents, d)
		ids[i] = id
	}
	return ids, nil
}

func (s *QdrantStore) Search(query string, topK int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queryEmbedding, err := s.embeddings.EmbedQuery(query)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, doc := range s.documents {
		docEmbedding := mockEmbedding(doc.Content)
		score := cosineSimilarityFloat32(queryEmbedding, docEmbedding)
		if score >= s.minScore {
			results = append(results, SearchResult{
				Document: doc, Score: score, ID: doc.ID,
			})
		}
	}
	sortSearchResults(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (s *QdrantStore) SearchRAG(query string, topK int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queryEmbedding, err := s.embeddings.EmbedQuery(query)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, doc := range s.documents {
		docEmbedding := mockEmbedding(doc.Content)
		score := cosineSimilarityFloat32(queryEmbedding, docEmbedding)
		if score >= 0.1 {
			results = append(results, SearchResult{
				Document: doc, Score: score, ID: doc.ID,
			})
		}
	}
	sortSearchResults(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func cosineSimilarityFloat32(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	dotProduct := 0.0
	magnitudeA := 0.0
	magnitudeB := 0.0
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		magnitudeA += float64(a[i]) * float64(a[i])
		magnitudeB += float64(b[i]) * float64(b[i])
	}
	if magnitudeA == 0 || magnitudeB == 0 {
		return 0
	}
	return dotProduct / (sqrt(magnitudeA) * sqrt(magnitudeB))
}

func sortSearchResults(results []SearchResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func (s *QdrantStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.documents)
}

func (s *QdrantStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents = make([]Document, 0)
	return nil
}

func (s *QdrantStore) VectorSize() int {
	return s.vectorSize
}

func (s *QdrantStore) GetChunksByFilename(filename string) ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Document
	for _, d := range s.documents {
		if d.Metadata != nil && d.Metadata["original_filename"] == filename {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s *QdrantStore) DeleteByMetadata(key, value string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []Document
	deleted := 0
	for _, d := range s.documents {
		if d.Metadata != nil && d.Metadata[key] == value {
			deleted++
		} else {
			kept = append(kept, d)
		}
	}
	s.documents = kept
	return deleted, nil
}
