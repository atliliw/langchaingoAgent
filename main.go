package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/atliliw/lanchaingo-agent/handlers"
	"github.com/atliliw/lanchaingo-agent/services"
)

func main() {
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	svcCfg := services.ServiceConfig{
		ServerAddr:   config.ServerAddr(),
		QdrantURL:    config.Qdrant.URL,
		QdrantColl:   config.Qdrant.CollectionName,
		QdrantSize:   config.Qdrant.VectorSize,
		QdrantDist:   config.Qdrant.Distance,
		MongoURI:     config.MongoDB.URI,
		MongoDB:      config.MongoDB.Database,
		MongoParent:  config.MongoDB.ParentCollection,
		MongoChunk:   config.MongoDB.ChunkCollection,
		SQLitePath:   config.SQLite.DBPath,
		ChunkSize:    config.Document.ChunkSize,
		ChunkOverlap: config.Document.ChunkOverlap,
		MinScore:     float64(config.Search.MinScore),
		DefaultTopK:  config.Search.DefaultTopK,
		UploadDir:    config.Server.UploadDir,
		OpenAIAPIKey: config.OpenAI.APIKey,
		OpenAIBaseURL: config.OpenAI.BaseURL,
		ChatModel:    config.OpenAI.ChatModel,
		EmbedModel:   config.Embedding.Model,
	}

	api, err := services.NewApiService(svcCfg)
	if err != nil {
		log.Fatalf("API 服务初始化失败: %v", err)
	}

	if err := os.MkdirAll(config.Server.UploadDir, 0755); err != nil {
		log.Printf("WARNING: 上传目录创建失败: %v", err)
	}

	state := &handlers.AppState{
		API: api,
		Cfg: svcCfg,
	}

	router := handlers.NewRouter(state)

	addr := config.ServerAddr()
	log.Printf("启动服务: %s", addr)
	log.Printf("Qdrant URL: %s", config.Qdrant.URL)
	log.Printf("Collection: %s", config.Qdrant.CollectionName)
	log.Printf("服务运行在 http://%s", addr)
	log.Printf("打开浏览器访问 http://%s 即可使用", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func init() {
	fmt.Println("=== Lanchaingo Agent (Go Port of LangChainRust Agent) ===")
	fmt.Println("Starting server...")
}
