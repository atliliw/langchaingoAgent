package stores

import (
	"context"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BM25Store struct {
	client          *mongo.Client
	dbName          string
	parentCollName  string
	chunkCollName   string
	documents       []BM25Doc
	mu              sync.RWMutex
}

type BM25Doc struct {
	ID       string
	Content  string
	ParentID string
	Score    float64
	IsMerged bool
	Metadata map[string]string
}

type DocumentFileInfo struct {
	ParentID string
	Filename string
	Metadata map[string]string
}

func NewBM25Store(uri, dbName, parentColl, chunkColl string) (*BM25Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	return &BM25Store{
		client:         client,
		dbName:         dbName,
		parentCollName: parentColl,
		chunkCollName:  chunkColl,
		documents:      make([]BM25Doc, 0),
	}, nil
}

func (s *BM25Store) AddDocuments(docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.client.Database(s.dbName)
	parentColl := db.Collection(s.parentCollName)
	chunkColl := db.Collection(s.chunkCollName)

	for _, doc := range docs {
		parentID := doc.ID
		if parentID == "" {
			parentID = "parent_" + doc.Content
		}
		parentDoc := bson.M{
			"_id":      parentID,
			"content":  doc.Content,
			"metadata": doc.Metadata,
		}
		parentColl.UpdateByID(context.Background(), parentID, bson.M{"$set": parentDoc}, options.Update().SetUpsert(true))

		chunkColl.InsertOne(context.Background(), bson.M{
			"parent_id": parentID,
			"content":   doc.Content,
			"metadata":  doc.Metadata,
		})

		s.documents = append(s.documents, BM25Doc{
			ID:       parentID,
			Content:  doc.Content,
			ParentID: parentID,
			Score:    0,
			IsMerged: false,
			Metadata: doc.Metadata,
		})
	}
	return nil
}

func (s *BM25Store) Search(query string, k int) ([]BM25Doc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var scored []BM25Doc
	for _, doc := range s.documents {
		score := simpleBM25Score(query, doc.Content)
		doc.Score = score
		scored = append(scored, doc)
	}
	sortByScore(scored)
	if len(scored) > k {
		scored = scored[:k]
	}
	return scored, nil
}

func simpleBM25Score(query, text string) float64 {
	qWords := splitWords(query)
	tWords := splitWords(text)
	if len(qWords) == 0 {
		return 0
	}
	n := len(tWords)
	avgDL := float64(n)
	k1 := 1.5
	b := 0.75
	score := 0.0
	for _, q := range qWords {
		df := 0
		for _, t := range tWords {
			if t == q {
				df++
			}
		}
		idf := 1.0
		numer := float64(df) * (k1 + 1)
		denom := float64(df) + k1*(1-b+b*float64(n)/avgDL)
		if denom == 0 {
			denom = 1
		}
		score += idf * numer / denom
	}
	return score
}

func splitWords(s string) []string {
	var words []string
	var current []rune
	for _, r := range s {
		if isWordChar(r) {
			current = append(current, r)
		} else if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r >= 0x4e00
}

func sortByScore(docs []BM25Doc) {
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			if docs[j].Score > docs[i].Score {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}
}

func (s *BM25Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.documents)
}

func (s *BM25Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents = make([]BM25Doc, 0)
	db := s.client.Database(s.dbName)
	db.Collection(s.parentCollName).Drop(context.Background())
	db.Collection(s.chunkCollName).Drop(context.Background())
	return nil
}

func (s *BM25Store) IsMongo() bool {
	return true
}

func (s *BM25Store) ListDocuments() ([]DocumentInfo, error) {
	db := s.client.Database(s.dbName)
	coll := db.Collection(s.parentCollName)
	cursor, err := coll.Find(context.Background(), bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var infos []DocumentInfo
	for cursor.Next(context.Background()) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		id := getBSONString(doc, "_id")
		content := getBSONString(doc, "content")
		meta := make(map[string]string)
		if md, ok := doc["metadata"].(bson.M); ok {
			for k, v := range md {
				meta[k] = toString(v)
			}
		}
		infos = append(infos, DocumentInfo{
			ID:             id,
			Title:          meta["original_filename"],
			ContentPreview: truncate(content, 100),
			ChunkCount:     0,
			Metadata:       meta,
		})
	}
	return infos, nil
}

func (s *BM25Store) DeleteDocument(parentID string) error {
	db := s.client.Database(s.dbName)
	db.Collection(s.parentCollName).DeleteOne(context.Background(), bson.M{"_id": parentID})
	db.Collection(s.chunkCollName).DeleteMany(context.Background(), bson.M{"parent_id": parentID})
	return nil
}

func (s *BM25Store) GetDocumentInfo(parentID string) (*DocumentFileInfo, error) {
	db := s.client.Database(s.dbName)
	var doc bson.M
	err := db.Collection(s.parentCollName).FindOne(context.Background(), bson.M{"_id": parentID}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	meta := make(map[string]string)
	if md, ok := doc["metadata"].(bson.M); ok {
		for k, v := range md {
			meta[k] = toString(v)
		}
	}
	return &DocumentFileInfo{
		ParentID: parentID,
		Filename: meta["original_filename"],
		Metadata: meta,
	}, nil
}

func (s *BM25Store) AddDocumentTags(parentID string, tags []string) error {
	db := s.client.Database(s.dbName)
	_, err := db.Collection(s.parentCollName).UpdateByID(
		context.Background(),
		parentID,
		bson.M{"$set": bson.M{"metadata.tags": tags}},
	)
	return err
}

func (s *BM25Store) GetDocumentsByTag(tag string) ([]DocumentInfo, error) {
	db := s.client.Database(s.dbName)
	cursor, err := db.Collection(s.parentCollName).Find(
		context.Background(),
		bson.M{"metadata.tags": tag},
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var infos []DocumentInfo
	for cursor.Next(context.Background()) {
		var doc bson.M
		cursor.Decode(&doc)
		id := getBSONString(doc, "_id")
		content := getBSONString(doc, "content")
		meta := make(map[string]string)
		if md, ok := doc["metadata"].(bson.M); ok {
			for k, v := range md {
				meta[k] = toString(v)
			}
		}
		infos = append(infos, DocumentInfo{
			ID:             id,
			Title:          meta["original_filename"],
			ContentPreview: truncate(content, 100),
			ChunkCount:     0,
			Metadata:       meta,
		})
	}
	return infos, nil
}

type DocumentInfo struct {
	ID             string
	Title          string
	ContentPreview string
	ChunkCount     int
	Metadata       map[string]string
}

func getBSONString(doc bson.M, key string) string {
	if v, ok := doc[key]; ok {
		return toString(v)
	}
	return ""
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}
