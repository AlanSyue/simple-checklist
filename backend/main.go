package main

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type ChecklistItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Text      string    `json:"text"`
	Checked   bool      `json:"checked"`
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
}

type ChecklistPayload struct {
	Items []ChecklistItem `json:"items"`
}

func main() {
	r := gin.Default()

	// 更嚴謹的 CORS 設定
	config := cors.Config{
		AllowOrigins:     []string{"http://127.0.0.1:5500", "http://localhost:5500", "http://127.0.0.1:3000", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	}
	// 允許所有 header
	r.Use(cors.New(config))

	db, _ := gorm.Open(sqlite.Open("data/checklist.db"), &gorm.Config{})
	db.AutoMigrate(&ChecklistItem{})

	// 獲取所有檢查清單項目
	r.GET("/api/checklist", func(c *gin.Context) {
		var items []ChecklistItem
		db.Find(&items)
		c.JSON(200, items)
	})

	// 獲取未完成的項目（checked: false）
	r.GET("/api/checklist/pending", func(c *gin.Context) {
		var items []ChecklistItem
		db.Where("checked = ?", false).Find(&items)
		c.JSON(200, items)
	})

	// 新增項目
	r.POST("/api/checklist", func(c *gin.Context) {
		var body ChecklistPayload
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}
		for _, item := range body.Items {
			db.Create(&ChecklistItem{Text: item.Text, Checked: item.Checked})
		}
		c.JSON(200, gin.H{"status": "saved"})
	})

	// 單筆更新
	r.PATCH("/api/checklist/:id", func(c *gin.Context) {
		var update struct {
			Checked *bool   `json:"checked"`
			Text    *string `json:"text"`
		}
		if err := c.ShouldBindJSON(&update); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}
		id := c.Param("id")
		var item ChecklistItem
		if err := db.First(&item, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}
		if update.Checked != nil {
			item.Checked = *update.Checked
		}
		if update.Text != nil {
			item.Text = *update.Text
		}
		db.Save(&item)
		c.JSON(200, item)
	})

	// 單筆刪除
	r.DELETE("/api/checklist/:id", func(c *gin.Context) {
		id := c.Param("id")
		if err := db.Delete(&ChecklistItem{}, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Delete failed"})
			return
		}
		c.JSON(200, gin.H{"status": "deleted"})
	})

	r.Run(":8080")
}
