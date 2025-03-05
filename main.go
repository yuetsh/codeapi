package main

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
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
		"http://localhost:3000",
	}

	r.Use(cors.New(config))

	r.GET("", func(c *gin.Context) {
		var codes []PresetCode
		db.Order("id desc").Find(&codes)
		c.JSON(http.StatusOK, gin.H{"data": codes})
	})

	r.GET("/:query", func(c *gin.Context) {
		var code PresetCode
		db.Where("query = ?", c.Param("query")).First(&code)
		if code.Code != "" {
			c.JSON(http.StatusOK, gin.H{"data": code})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Record not found!"})
		}
	})

	r.POST("", func(c *gin.Context) {
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

	r.DELETE(":id", func(c *gin.Context) {
		var code PresetCode
		if err := db.Where("id = ?", c.Param("id")).First(&code).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
			return
		}

		db.Delete(&code)
		c.JSON(http.StatusOK, gin.H{"data": true})
	})

	r.Run()
}
