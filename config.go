package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// ============================================================
// Config — Configuration management from config.toml
// ============================================================

// Config is the root configuration structure
type Config struct {
	Server       ServerConfig       `toml:"server"`
	OpenAI       OpenAIConfig       `toml:"openai"`
	Embedding    EmbeddingConfig    `toml:"embedding"`
	MongoDB      MongoConfig        `toml:"mongodb"`
	SQLite       SQLiteConfig       `toml:"sqlite"`
	Qdrant       QdrantConfig       `toml:"qdrant"`
	Document     DocumentConfig     `toml:"document"`
	Search       SearchConfig       `toml:"search"`
	Logging      LoggingConfig      `toml:"logging"`
	Conversation *ConversationConfig `toml:"conversation,omitempty"`
}

// ServerConfig — HTTP server settings
type ServerConfig struct {
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	UploadDir string `toml:"upload_dir"`
}

// OpenAIConfig — OpenAI / LLM API configuration
type OpenAIConfig struct {
	APIKey          string `toml:"api_key"`
	BaseURL         string `toml:"base_url"`
	ChatModel       string `toml:"chat_model"`
	EmbeddingModel  string `toml:"embedding_model"`
}

// EmbeddingConfig — Standalone embedding API config
type EmbeddingConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
	Model   string `toml:"model"`
}

// QdrantConfig — Qdrant vector database config
type QdrantConfig struct {
	URL            string `toml:"url"`
	CollectionName string `toml:"collection_name"`
	VectorSize     int    `toml:"vector_size"`
	Distance       string `toml:"distance"`
}

// MongoConfig — MongoDB BM25 document store config
type MongoConfig struct {
	URI               string `toml:"uri"`
	Database          string `toml:"database"`
	ParentCollection  string `toml:"parent_collection"`
	ChunkCollection   string `toml:"chunk_collection"`
}

// SQLiteConfig — SQLite conversation store config
type SQLiteConfig struct {
	DBPath string `toml:"db_path"`
}

// ConversationConfig — Conversation compression config
type ConversationConfig struct {
	MaxHistoryMessages int      `toml:"max_history_messages"`
	MaxTokens          int      `toml:"max_tokens"`
	KeepFirstNMessages int      `toml:"keep_first_n_messages"`
	CompressThreshold  int      `toml:"compress_threshold"`
	KeepRecentMessages int      `toml:"keep_recent_messages"`
	ImportantKeywords  []string `toml:"important_keywords"`
	SummaryModel       string   `toml:"summary_model"`
}

// DocumentConfig — Document processing config
type DocumentConfig struct {
	ChunkSize      int      `toml:"chunk_size"`
	ChunkOverlap   int      `toml:"chunk_overlap"`
	SupportedTypes []string `toml:"supported_types"`
}

// SearchConfig — Search configuration
type SearchConfig struct {
	DefaultTopK     int     `toml:"default_top_k"`
	MinScore        float32 `toml:"min_score"`
	TestSampleCount int     `toml:"test_sample_count"`
}

// LoggingConfig — Logging configuration
type LoggingConfig struct {
	Level      string `toml:"level"`
	FileOutput bool   `toml:"file_output"`
	LogFile    string `toml:"log_file"`
}

// DefaultConversationConfig returns default conversation compression settings
func DefaultConversationConfig() *ConversationConfig {
	return &ConversationConfig{
		MaxHistoryMessages: 50,
		MaxTokens:          4000,
		KeepFirstNMessages: 2,
		CompressThreshold:  15,
		KeepRecentMessages: 5,
		ImportantKeywords:  []string{"我的名字", "我是", "记住", "设定", "角色"},
		SummaryModel:       "gpt-3.5-turbo",
	}
}

// Validate checks required configuration fields
func (c *Config) Validate() error {
	if c.OpenAI.APIKey == "" || strings.Contains(c.OpenAI.APIKey, "your-api-key") {
		return fmt.Errorf("请在 config.toml 中配置有效的 OpenAI API Key")
	}
	if c.Qdrant.URL == "" {
		return fmt.Errorf("Qdrant URL 不能为空")
	}
	validDims := []int{128, 256, 512, 768, 1024, 1536, 3072}
	valid := false
	for _, d := range validDims {
		if c.Qdrant.VectorSize == d {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("向量维度 %d 不是常用值，常用值: %v", c.Qdrant.VectorSize, validDims)
	}
	return nil
}

// ServerAddr returns the server listen address
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// ToLLMConfig converts to LLM config for use with lanchaingo
func (c *Config) ToLLMConfig() LLMConfig {
	return LLMConfig{
		APIKey:     c.OpenAI.APIKey,
		BaseURL:    c.OpenAI.BaseURL,
		Model:      c.OpenAI.ChatModel,
		Streaming:  false,
		Temperature: 0.7,
		MaxTokens:  500,
	}
}

// ToEmbeddingConfig converts to embedding config
func (c *Config) ToEmbeddingConfig() EmbeddingAPIConfig {
	key := c.Embedding.APIKey
	if key == "" {
		key = c.OpenAI.APIKey
	}
	baseURL := c.Embedding.BaseURL
	if baseURL == "" {
		baseURL = c.OpenAI.BaseURL
	}
	model := c.Embedding.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	return EmbeddingAPIConfig{
		APIKey:  key,
		BaseURL: baseURL,
		Model:   model,
	}
}

// LLMConfig mirrors the config for LLM chat models
type LLMConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	Streaming   bool
	Temperature float32
	MaxTokens   int
}

// EmbeddingAPIConfig mirrors the config for embedding models
type EmbeddingAPIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// LoadConfig loads configuration from file or environment
func LoadConfig() (*Config, error) {
	// Try config.toml in current directory
	if _, err := os.Stat("config.toml"); err == nil {
		return LoadConfigFromFile("config.toml")
	}

	// Try demo/config.toml
	if _, err := os.Stat("demo/config.toml"); err == nil {
		return LoadConfigFromFile("demo/config.toml")
	}

	// Fallback to environment variables
	return LoadConfigFromEnv()
}

// LoadConfigFromFile loads config from a TOML file
func LoadConfigFromFile(path string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("配置文件解析失败: %w", err)
	}

	// Set defaults
	if config.Conversation == nil {
		config.Conversation = DefaultConversationConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfigFromEnv loads config from environment variables
func LoadConfigFromEnv() (*Config, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("环境变量 OPENAI_API_KEY 未设置")
	}

	config := &Config{
		Server: ServerConfig{
			Host:      envOrDefault("SERVER_HOST", "0.0.0.0"),
			Port:      envOrDefaultInt("SERVER_PORT", 8080),
			UploadDir: envOrDefault("UPLOAD_DIR", "uploads"),
		},
		OpenAI: OpenAIConfig{
			APIKey:         apiKey,
			BaseURL:        envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			ChatModel:      envOrDefault("CHAT_MODEL", "gpt-3.5-turbo"),
			EmbeddingModel: envOrDefault("EMBEDDING_MODEL", "text-embedding-ada-002"),
		},
		Embedding: EmbeddingConfig{
			APIKey:  envOrDefault("EMBEDDING_API_KEY", apiKey),
			BaseURL: envOrDefault("EMBEDDING_BASE_URL", envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")),
			Model:   envOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"),
		},
		MongoDB: MongoConfig{
			URI:              envOrDefault("MONGODB_URI", "mongodb://localhost:27017"),
			Database:         "langchainrust_demo",
			ParentCollection: "bm25_parents",
			ChunkCollection:  "bm25_chunks",
		},
		SQLite: SQLiteConfig{
			DBPath: envOrDefault("SQLITE_DB_PATH", "conversations.db"),
		},
		Qdrant: QdrantConfig{
			URL:            envOrDefault("QDRANT_URL", "http://localhost:6334"),
			CollectionName: "demo_documents",
			VectorSize:     1536,
			Distance:       "Cosine",
		},
		Document: DocumentConfig{
			ChunkSize:    500,
			ChunkOverlap: 50,
			SupportedTypes: []string{"txt", "pdf", "md", "json", "csv"},
		},
		Search: SearchConfig{
			DefaultTopK:     5,
			MinScore:        0.5,
			TestSampleCount: 10,
		},
		Logging: LoggingConfig{
			Level:      "info",
			FileOutput: false,
			LogFile:    "logs/demo.log",
		},
		Conversation: DefaultConversationConfig(),
	}
	return config, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
