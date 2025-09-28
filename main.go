package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"gorm.io/gorm"
)

// 预设代码
type PresetCode struct {
	ID    uint   `gorm:"primarykey" json:"id"`
	Query string `gorm:"unique" json:"query"`
	Code  string `json:"code"`
}

type PresetCodeInput struct {
	Code  string `json:"code" binding:"required"`
	Query string `json:"query" binding:"required"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := gorm.Open(sqlite.Open("database.db"), &gorm.Config{})

	if err != nil {
		panic("fail to get data")
	}

	db.AutoMigrate(&PresetCode{})

	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowMethods = []string{"GET", "POST", "DELETE", "PUT"}
	config.AllowOrigins = []string{
		"https://code.xuyue.cc",
		"http://code.xuyue.cc",
		"http://10.13.114.114",
		"http://localhost:3000",
	}

	r.Use(cors.New(config))

	r.GET("/", func(c *gin.Context) {
		var codes []PresetCode
		db.Order("id desc").Find(&codes)
		c.JSON(http.StatusOK, gin.H{"data": codes})
	})

	r.GET("/query/:query", func(c *gin.Context) {
		var code PresetCode
		db.Where("query = ?", c.Param("query")).First(&code)
		if code.Code != "" {
			c.JSON(http.StatusOK, gin.H{"data": code})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Record not found!"})
		}
	})

	r.POST("/", func(c *gin.Context) {
		var input PresetCodeInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		code := PresetCode{Code: input.Code, Query: input.Query}
		if err := db.Create(&code).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"data": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": code})
	})

	r.DELETE("/:id", func(c *gin.Context) {
		var code PresetCode
		if err := db.Where("id = ?", c.Param("id")).First(&code).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
			return
		}

		db.Delete(&code)
		c.JSON(http.StatusOK, gin.H{"data": true})
	})

	r.POST("/ai", func(c *gin.Context) {
		var payload struct {
			Code      string `json:"code"`
			ErrorInfo string `json:"error_info"`
			Language  string `json:"language"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		systemPrompt := "你是编程老师，擅长分析代码和错误信息，一般出错在语法和格式，请指出错误在第几行，并给出中文的、简要的解决方法。用 markdown 格式返回。"
		userPrompt := fmt.Sprintf("编程语言：%s\n代码：\n```%s\n```\n错误信息：\n```%s\n```", payload.Language, payload.Code, payload.ErrorInfo)

		apiKey := os.Getenv("API_KEY")

		if apiKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "API_KEY is not set"})
			return
		}

		client := openai.NewClient(
			option.WithBaseURL("https://api.deepseek.com"),
			option.WithAPIKey(apiKey),
		)

		ctx := c.Request.Context()
		stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(userPrompt),
			},
			Seed:  openai.Int(0),
			Model: "deepseek-chat",
		})

		if err := stream.Err(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		defer stream.Close()

		header := c.Writer.Header()
		header.Set("Content-Type", "text/event-stream")

		if _, ok := c.Writer.(http.Flusher); !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
			return
		}

		c.Status(http.StatusOK)

		replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n")

		writeSSE := func(w io.Writer, event string, payload gin.H) {
			sanitized := make(gin.H, len(payload))
			for k, v := range payload {
				sanitized[k] = v
			}
			if data, ok := payload["data"].(string); ok {
				sanitized["data"] = replacer.Replace(data)
			}
			bytes, err := json.Marshal(sanitized)
			if err != nil {
				log.Printf("failed to marshal sse payload: %v", err)
				return
			}
			if event != "" {
				fmt.Fprintf(w, "event: %s\n", event)
			}
			fmt.Fprintf(w, "data: %s\n\n", bytes)
		}

		sentDone := false

		c.Stream(func(w io.Writer) bool {
			if sentDone {
				return false
			}

			for stream.Next() {
				chunk := stream.Current()
				var builder strings.Builder
				for _, choice := range chunk.Choices {
					if choice.Delta.Content != "" {
						builder.WriteString(choice.Delta.Content)
					}
				}
				if builder.Len() == 0 {
					continue
				}
				writeSSE(w, "chunk", gin.H{"data": builder.String()})
				return true
			}

			sentDone = true
			if err := stream.Err(); err != nil {
				writeSSE(w, "error", gin.H{"message": err.Error()})
			}
			writeSSE(w, "done", gin.H{"data": ""})
			return true
		})
	})

	gin.SetMode(gin.ReleaseMode)

	r.Run()
}
