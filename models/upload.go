package models

// ChunkStrategy — Document chunking strategy
type ChunkStrategy int

const (
	ChunkRecursive  ChunkStrategy = iota
	ChunkLarge
	ChunkSmall
	ChunkParagraph
	ChunkToken
	ChunkSemantic
	ChunkPageIndex
)

func (s ChunkStrategy) String() string {
	switch s {
	case ChunkRecursive:
		return "recursive"
	case ChunkLarge:
		return "large"
	case ChunkSmall:
		return "small"
	case ChunkParagraph:
		return "paragraph"
	case ChunkToken:
		return "token"
	case ChunkSemantic:
		return "semantic"
	case ChunkPageIndex:
		return "pageindex"
	default:
		return "recursive"
	}
}

func ParseChunkStrategy(s string) ChunkStrategy {
	switch s {
	case "large":
		return ChunkLarge
	case "small":
		return ChunkSmall
	case "paragraph":
		return ChunkParagraph
	case "token":
		return ChunkToken
	case "semantic":
		return ChunkSemantic
	case "pageindex":
		return ChunkPageIndex
	default:
		return ChunkRecursive
	}
}

// UploadResponse — POST /api/upload response
type UploadResponse struct {
	Success       bool     `json:"success"`
	DocumentCount int      `json:"document_count"`
	ChunkCount    int      `json:"chunk_count"`
	Message       string   `json:"message"`
	DocumentIDs   []string `json:"document_ids"`
	ChunkStrategy string   `json:"chunk_strategy"`
}
