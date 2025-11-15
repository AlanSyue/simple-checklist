package main

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// --- Existing Checklist Models ---
type ChecklistItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Text      string    `json:"text"`
	Checked   bool      `json:"checked"`
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
}

type ChecklistPayload struct {
	Items []ChecklistItem `json:"items"`
}

// --- New WooCommerce Order Models ---

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal StringArray value: %T %v", value, value)
	}

	if len(bytes) == 0 {
		*a = []string{}
		return nil
	}

	return json.Unmarshal(bytes, a)
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil || len(a) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

type OrderMetadata struct {
	OrderID     int         `gorm:"primaryKey"`
	Remark      string      `json:"remark"`
	Tags        StringArray `json:"tags" gorm:"type:jsonb"`
	IsCompleted bool        `json:"is_completed"`
}

type WooOrder struct {
	ID                 int          `json:"id"`
	Shipping           ShippingInfo `json:"shipping"`
	Billing            BillingInfo  `json:"billing"`
	Total              string       `json:"total"`
	LineItems          []LineItem   `json:"line_items"`
	MetaData           []MetaData   `json:"meta_data"`
	CustomerNote       string       `json:"customer_note"`
	ShippingLines      []ShippingLine `json:"shipping_lines"`
	PaymentMethodTitle string       `json:"payment_method_title"`
	OrderMetadata      OrderMetadata `json:"order_metadata" gorm:"-"`
}

type BillingInfo struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type ShippingInfo struct {
	FirstName string `json:"first_name"`
}

type LineItem struct {
	Name      string     `json:"name"`
	Quantity  int        `json:"quantity"`
	Price     float64    `json:"price"`
	Total     string     `json:"total"`
	MetaData  []MetaData `json:"meta_data"`
}

type MetaData struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
	DisplayKey string `json:"display_key"`
	DisplayValue string `json:"display_value"`
}

type ShippingLine struct {
	MethodTitle string `json:"method_title"`
}


func main() {
	// --- Database Connection ---
	dsn := fmt.Sprintf("host=postgres user=%s password=%s dbname=%s port=5432 sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
	)
	var db *gorm.DB
	var err error
	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database (attempt %d): %v", i+1, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database after multiple attempts: %v", err)
	}
	db.AutoMigrate(&ChecklistItem{}, &OrderMetadata{})

	// --- Gin Router Setup ---
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.File("./frontend/index.html")
	})
	r.GET("/index.html", func(c *gin.Context) {
		c.File("./frontend/index.html")
	})
	r.GET("/orders.html", func(c *gin.Context) {
		c.File("./frontend/orders.html")
	})
	r.GET("/picking.html", func(c *gin.Context) {
		c.File("./frontend/picking.html")
	})
	r.GET("/picking-list-print.html", func(c *gin.Context) {
		c.File("./frontend/picking-list-print.html")
	})
	r.Static("/js", "./frontend/js")
	r.Static("/css", "./frontend/css")
	r.StaticFile("/nav.html", "./frontend/nav.html")

	api := r.Group("/api")

	// --- Existing Checklist Routes ---
	api.GET("/checklist", func(c *gin.Context) {
		var items []ChecklistItem
		db.Find(&items)
		c.JSON(200, items)
	})

	api.GET("/checklist/pending", func(c *gin.Context) {
		var items []ChecklistItem
		db.Where("checked = ?", false).Find(&items)
		c.JSON(200, items)
	})

	api.POST("/checklist", func(c *gin.Context) {
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

	api.PATCH("/checklist/:id", func(c *gin.Context) {
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

	api.DELETE("/checklist/:id", func(c *gin.Context) {
		id := c.Param("id")
		if err := db.Delete(&ChecklistItem{}, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Delete failed"})
			return
		}
		c.JSON(200, gin.H{"status": "deleted"})
	})

// ... (file content up to the new routes)
// --- New WooCommerce Routes ---
	api.GET("/orders", func(c *gin.Context) {
		wooOrders, err := fetchProcessingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Get all order IDs
		orderIDs := make([]int, len(wooOrders))
		for i, order := range wooOrders {
			orderIDs[i] = order.ID
		}

		// Fetch all metadata in one query
		var metadatas []OrderMetadata
		if len(orderIDs) > 0 {
			db.Where("order_id IN ?", orderIDs).Find(&metadatas)
		}

		// Create a map for easy lookup
		metadataMap := make(map[int]OrderMetadata)
		for _, m := range metadatas {
			metadataMap[m.OrderID] = m
		}

		// Merge data and apply tag filter
		var filteredWooOrders []WooOrder
		requestedTags := c.QueryArray("tags") // Get tags as a slice of strings from query parameter
		hasRemark := c.Query("has_remark") == "true"
		hasCustomerNote := c.Query("has_customer_note") == "true"

		for _, order := range wooOrders {
			var metadata OrderMetadata
			if m, ok := metadataMap[order.ID]; ok {
				metadata = m
			} else {
				// If metadata doesn't exist, create it with empty tags
				newMeta := OrderMetadata{OrderID: order.ID, Remark: "", Tags: []string{}}
				db.Create(&newMeta)
				metadata = newMeta
			}
			order.OrderMetadata = metadata

			// Apply tag filtering (OR logic - match any of the requested tags)
			tagMatch := true
			if len(requestedTags) > 0 {
				hasAnyTag := false
				for _, reqTag := range requestedTags {
					for _, orderTag := range metadata.Tags {
						if reqTag == orderTag {
							hasAnyTag = true
							break
						}
					}
					if hasAnyTag {
						break
					}
				}
				tagMatch = hasAnyTag
			}

			// Apply remark filter
			remarkMatch := true
			if hasRemark {
				remarkMatch = metadata.Remark != ""
			}

			// Apply customer note filter
			customerNoteMatch := true
			if hasCustomerNote {
				customerNoteMatch = order.CustomerNote != ""
			}

			if tagMatch && remarkMatch && customerNoteMatch {
				filteredWooOrders = append(filteredWooOrders, order)
			}
		}

		c.JSON(http.StatusOK, filteredWooOrders)
	})

	api.PUT("/orders/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
			return
		}

		var metadataUpdate OrderMetadata
		if err := c.ShouldBindJSON(&metadataUpdate); err != nil {
			log.Printf("Failed to bind JSON: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}

		fmt.Printf("Received metadata update for order %d: %+v\n", id, metadataUpdate) // Debug log

		metadataUpdate.OrderID = id

		if err := db.Save(&metadataUpdate).Error; err != nil {
			log.Printf("Failed to save metadata for order %d: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metadata: %v", err)})
			return
		}

		c.JSON(http.StatusOK, metadataUpdate)
	})

	api.GET("/orders/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
			return
		}

		wooOrder, err := fetchSingleOrder(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadata OrderMetadata
		if err := db.First(&metadata, wooOrder.ID).Error; err != nil {
			// If not found, create it
			metadata = OrderMetadata{OrderID: wooOrder.ID, Remark: "", Tags: []string{}}
			db.Create(&metadata)
		}
		wooOrder.OrderMetadata = metadata

		c.JSON(http.StatusOK, wooOrder)
	})

	api.GET("/picking-list", func(c *gin.Context) {
		orders, err := fetchProcessingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		pickingList := generatePickingList(orders)
		c.JSON(http.StatusOK, pickingList)
	})


	r.Run(":8080")
}

type PickingListItem struct {
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	OrderIDs  []int  `json:"order_ids"`
}

func generatePickingList(orders []WooOrder) []PickingListItem {
	pickingMap := make(map[string]*PickingListItem)

	for _, order := range orders {
		for _, item := range order.LineItems {
			if _, ok := pickingMap[item.Name]; !ok {
				pickingMap[item.Name] = &PickingListItem{
					Name:      item.Name,
					Quantity:  0,
					OrderIDs:  []int{},
				}
			}
			pickingMap[item.Name].Quantity += item.Quantity
			pickingMap[item.Name].OrderIDs = append(pickingMap[item.Name].OrderIDs, order.ID)
		}
	}

	var pickingList []PickingListItem
	for _, item := range pickingMap {
		pickingList = append(pickingList, *item)
	}

	return pickingList
}

func fetchProcessingOrders() ([]WooOrder, error) {
// ... (rest of the file is the same)

	url := "https://flowers.fenny-studio.com/wp-json/wc/v3/orders?status=processing&per_page=100"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	apiKey := os.Getenv("WOO_API_KEY")
	apiSecret := os.Getenv("WOO_API_SECRET")
	req.SetBasicAuth(apiKey, apiSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("received non-200 status code: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var orders []WooOrder
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return orders, nil
}

func fetchSingleOrder(orderID int) (WooOrder, error) {
	url := fmt.Sprintf("https://flowers.fenny-studio.com/wp-json/wc/v3/orders/%d", orderID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return WooOrder{}, fmt.Errorf("error creating request: %w", err)
	}

	apiKey := os.Getenv("WOO_API_KEY")
	apiSecret := os.Getenv("WOO_API_SECRET")
	req.SetBasicAuth(apiKey, apiSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return WooOrder{}, fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return WooOrder{}, fmt.Errorf("received non-200 status code: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var order WooOrder
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return WooOrder{}, fmt.Errorf("error decoding response: %w", err)
	}

	return order, nil
}

