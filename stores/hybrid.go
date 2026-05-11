package stores

// No external imports needed

type HybridSearchResult struct {
	Content     string
	RRFScore    float32
	BM25Score   *float64
	VectorScore *float64
	Source      string
	ID          string
}

type HybridStore struct {
	bm25Store   *BM25Store
	qdrantStore *QdrantStore
	topK        int
}

func NewHybridStore(bm25 *BM25Store, qdrant *QdrantStore, topK int) *HybridStore {
	return &HybridStore{
		bm25Store:   bm25,
		qdrantStore: qdrant,
		topK:        topK,
	}
}

func (s *HybridStore) Search(query string, k int) ([]HybridSearchResult, error) {
	bm25K := s.topK
	vectorK := s.topK

	bm25Results, err := s.bm25Store.Search(query, bm25K)
	if err != nil {
		return nil, err
	}
	vectorResults, err := s.qdrantStore.Search(query, vectorK)
	if err != nil {
		return nil, err
	}

	allDocs := make(map[string]*HybridSearchResult)
	kConst := 60.0

	for i, r := range bm25Results {
		rank := float64(i + 1)
		rrfScore := 1.0 / (kConst + rank)
		score := float32(rrfScore)
		bs := r.Score
		allDocs[r.ID] = &HybridSearchResult{
			Content:   r.Content,
			RRFScore:  score,
			BM25Score: &bs,
			Source:    "bm25",
			ID:        r.ID,
		}
	}

	for i, r := range vectorResults {
		rank := float64(i + 1)
		rrfScore := 1.0 / (kConst + rank)
		if existing, ok := allDocs[r.ID]; ok {
			existing.RRFScore += float32(rrfScore)
			existing.Source = "hybrid"
			vs := r.Score
			existing.VectorScore = &vs
		} else {
			score := float32(rrfScore)
			vs := r.Score
			allDocs[r.ID] = &HybridSearchResult{
				Content:     r.Document.Content,
				RRFScore:    score,
				VectorScore: &vs,
				Source:      "vector",
				ID:          r.ID,
			}
		}
	}

	var results []HybridSearchResult
	for _, r := range allDocs {
		results = append(results, *r)
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].RRFScore > results[i].RRFScore {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > k {
		results = results[:k]
	}

	return results, nil
}

func (s *HybridStore) AddDocuments(docs []Document) ([]string, error) {
	err := s.bm25Store.AddDocuments(docs)
	if err != nil {
		return nil, err
	}
	ids, err := s.qdrantStore.AddDocuments(docs)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *HybridStore) Clear() error {
	err := s.bm25Store.Clear()
	if err != nil {
		return err
	}
	return s.qdrantStore.Clear()
}

func (s *HybridStore) BM25Score(query, text string) float64 {
	qWords := splitWords(query)
	tWords := splitWords(text)
	if len(qWords) == 0 {
		return 0
	}
	matches := 0
	for _, q := range qWords {
		for _, t := range tWords {
			if t == q {
				matches++
				break
			}
		}
	}
	return float64(matches) / float64(len(qWords))
}
