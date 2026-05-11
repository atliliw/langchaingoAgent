package models

// SearchRequest — POST /api/search/*
type SearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (r *SearchRequest) GetTopK() int {
	if r.TopK <= 0 {
		return 5
	}
	return r.TopK
}

// SearchResponse — Search API response
type SearchResponse struct {
	Query       string            `json:"query"`
	Mode        string            `json:"mode"`
	Results     []SearchResultItem `json:"results"`
	TotalCount  int               `json:"total_count"`
}

// SearchResultItem — One search result
type SearchResultItem struct {
	ID       *string     `json:"id,omitempty"`
	Content  string      `json:"content"`
	Score    float32     `json:"score"`
	Source   *string     `json:"source,omitempty"`
	Metadata interface{} `json:"metadata"`
}

// CompareRequest — POST /api/search/compare
type CompareRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (r *CompareRequest) GetTopK() int {
	if r.TopK <= 0 {
		return 5
	}
	return r.TopK
}

// CompareResponse — Three-way search comparison
type CompareResponse struct {
	Query          string            `json:"query"`
	VectorResults  []SearchResultItem `json:"vector_results"`
	BM25Results    []SearchResultItem `json:"bm25_results"`
	HybridResults  []SearchResultItem `json:"hybrid_results"`
	Comparison     SearchComparison  `json:"comparison"`
}

// SearchComparison — Comparison metrics
type SearchComparison struct {
	VectorTop1Score float32 `json:"vector_top1_score"`
	BM25Top1Score   float32 `json:"bm25_top1_score"`
	HybridTop1Score float32 `json:"hybrid_top1_score"`
	OverlapCount    int     `json:"overlap_count"`
	UniqueVector    int     `json:"unique_vector"`
	UniqueBM25      int     `json:"unique_bm25"`
	UniqueHybrid    int     `json:"unique_hybrid"`
}
