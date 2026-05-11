package services

import (
	"time"
)

type PageIndexService struct{
	cfg ServiceConfig
}

func NewPageIndexService(cfg ServiceConfig) *PageIndexService {
	return &PageIndexService{cfg: cfg}
}

type PageIndexInfo struct {
	DocID     string `json:"doc_id"`
	Title     string `json:"title"`
	NodeCount int    `json:"node_count"`
}

func (s *PageIndexService) BuildFromText(docID, title, text string) (*PageIndexInfo, error) {
	_ = time.Now()
	return &PageIndexInfo{
		DocID:     docID,
		Title:     title,
		NodeCount: 1,
	}, nil
}
