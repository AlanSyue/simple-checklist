package main

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
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
	ID                 int            `json:"id"`
	DateCreated        string         `json:"date_created"`
	Shipping           ShippingInfo   `json:"shipping"`
	Billing            BillingInfo    `json:"billing"`
	Total              string         `json:"total"`
	LineItems          []LineItem     `json:"line_items"`
	MetaData           []MetaData     `json:"meta_data"`
	CustomerNote       string         `json:"customer_note"`
	ShippingLines      []ShippingLine `json:"shipping_lines"`
	PaymentMethodTitle string         `json:"payment_method_title"`
	OrderMetadata      OrderMetadata  `json:"order_metadata" gorm:"-"`
}

type BillingInfo struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type ShippingInfo struct {
	FirstName string `json:"first_name"`
}

type LineItem struct {
	Name     string     `json:"name"`
	Quantity int        `json:"quantity"`
	Price    float64    `json:"price"`
	Total    string     `json:"total"`
	MetaData []MetaData `json:"meta_data"`
}

type MetaData struct {
	Key          string `json:"key"`
	Value        any    `json:"value"`
	DisplayKey   string `json:"display_key"`
	DisplayValue string `json:"display_value"`
}

type ShippingLine struct {
	MethodTitle string `json:"method_title"`
}

type UploadedOrder struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	OrderNo       string    `json:"order_no"`
	OrderedAt     time.Time `json:"ordered_at"`
	ReceiverName  string    `json:"receiver_name"`
	Address       string    `json:"address"`
	ProductName   string    `json:"product_name"`
	UnitPrice     float64   `json:"unit_price"`
	DiscountPrice float64   `json:"discount_price"`
	Qty           int       `json:"qty"`
	Note          string    `json:"note"`
}

type UploadedOrderItem struct {
	ProductName   string  `json:"product_name"`
	UnitPrice     float64 `json:"unit_price"`
	DiscountPrice float64 `json:"discount_price"`
	Qty           int     `json:"qty"`
	Note          string  `json:"note"`
}

type UploadedOrderSummary struct {
	OrderNo      string              `json:"order_no"`
	OrderedAt    time.Time           `json:"ordered_at"`
	ReceiverName string              `json:"receiver_name"`
	Address      string              `json:"address"`
	TotalQty     int                 `json:"total_qty"`
	TotalAmount  float64             `json:"total_amount"`
	Items        []UploadedOrderItem `json:"items"`
}

type ProductPickingItem struct {
	ProductName string   `json:"product_name"`
	TotalQty    int      `json:"total_qty"`
	OrderNos    []string `json:"order_nos"`
}

type UploadBatch struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UploadedAt time.Time `json:"uploaded_at"`
}

var normalizedHeaderMapping = map[string]string{
	"orderno":     "order_no",
	"ordernumber": "order_no",
	"order_no":    "order_no",
	"訂單編號":        "order_no",

	"orderedat":       "ordered_at",
	"ordereddatetime": "ordered_at",
	"ordereddate":     "ordered_at",
	"orderedtime":     "ordered_at",
	"ordered_at":      "ordered_at",
	"訂購日期":            "ordered_at",
	"訂單日期":            "ordered_at",

	"receivername": "receiver_name",
	"收件人姓名":        "receiver_name",
	"收件人":          "receiver_name",

	"address": "address",
	"取件地址":    "address",
	"地址":      "address",

	"productname": "product_name",
	"商品名稱":        "product_name",

	"unitprice":  "unit_price",
	"unit_price": "unit_price",
	"單價":         "unit_price",

	"discountprice":   "discount_price",
	"discount_price":  "discount_price",
	"優惠價":             "discount_price",
	"折扣後價格":           "discount_price",
	"discountedprice": "discount_price",

	"qty":      "qty",
	"quantity": "qty",
	"數量":       "qty",

	"note": "note",
	"備註":   "note",
	"訂單備註": "note",
}

var headerContainsMapping = []struct {
	substr string
	key    string
}{
	{substr: "商品名稱", key: "product_name"},
	{substr: "品名", key: "product_name"},
	{substr: "規格", key: "product_name"},
	{substr: "付款方式", key: "payment_method"},
}

var requiredUploadedColumns = []string{
	"order_no",
	"ordered_at",
	"receiver_name",
	"address",
	"product_name",
	"unit_price",
	"discount_price",
	"qty",
	"note",
}

func normalizeHeader(raw string) string {
	cleaned := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer(
		" ", "", "_", "", "-", "", "：", "", ":", "",
		"(", "", ")", "", "（", "", "）", "", ".", "", "。", "",
		"、", "", "/", "", "／", "",
		"\n", "", "\r", "", "\t", "",
	)
	return replacer.Replace(cleaned)
}

func buildHeaderIndex(header []string) (map[string]int, []string, error) {
	index := make(map[string]int)
	for idx, raw := range header {
		norm := normalizeHeader(raw)
		if key, ok := normalizedHeaderMapping[norm]; ok {
			index[key] = idx
			continue
		}

		for _, candidate := range headerContainsMapping {
			target := normalizeHeader(candidate.substr)
			if target != "" && strings.Contains(norm, target) {
				if _, exists := index[candidate.key]; !exists {
					index[candidate.key] = idx
				}
				break
			}
		}
	}

	var missing []string
	for _, required := range requiredUploadedColumns {
		if _, ok := index[required]; !ok {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return nil, missing, fmt.Errorf("缺少必要欄位：%s", strings.Join(missing, ","))
	}

	return index, nil, nil
}

func rowHasData(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

func logRow(label string, rowNumber int, row []string) {
	if len(row) == 0 {
		log.Printf("%s 第 %d 列：<空列>", label, rowNumber)
		return
	}
	log.Printf("%s 第 %d 列：%s", label, rowNumber, strings.Join(row, " | "))
}

func parseUploadedRow(row []string, headerIndex map[string]int, rowNumber int) (UploadedOrder, error) {
	get := func(column string) string {
		if idx, ok := headerIndex[column]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	order := UploadedOrder{}
	if value := get("order_no"); value != "" {
		order.OrderNo = value
	} else {
		return order, fmt.Errorf("第 %d 列缺少訂單編號", rowNumber)
	}

	if value := get("receiver_name"); value != "" {
		order.ReceiverName = value
	} else {
		return order, fmt.Errorf("第 %d 列缺少收件人", rowNumber)
	}

	if value := get("address"); value != "" {
		order.Address = value
	} else {
		return order, fmt.Errorf("第 %d 列缺少取件地址", rowNumber)
	}

	if value := get("product_name"); value != "" {
		order.ProductName = value
	} else {
		return order, fmt.Errorf("第 %d 列缺少商品名稱", rowNumber)
	}

	order.Note = get("note")

	if value := get("ordered_at"); value != "" {
		parsed, err := parseDateTime(value)
		if err != nil {
			return order, fmt.Errorf("第 %d 列的訂購日期格式錯誤：%w", rowNumber, err)
		}
		order.OrderedAt = parsed
	} else {
		return order, fmt.Errorf("第 %d 列缺少訂購日期", rowNumber)
	}

	if value := get("unit_price"); value != "" {
		parsed, err := parseNumber(value)
		if err != nil {
			return order, fmt.Errorf("第 %d 列的單價格式錯誤：%w", rowNumber, err)
		}
		order.UnitPrice = parsed
	} else {
		return order, fmt.Errorf("第 %d 列缺少單價", rowNumber)
	}

	if value := get("discount_price"); value != "" {
		parsed, err := parseNumber(value)
		if err != nil {
			return order, fmt.Errorf("第 %d 列的優惠價格式錯誤：%w", rowNumber, err)
		}
		order.DiscountPrice = parsed
	} else {
		return order, fmt.Errorf("第 %d 列缺少優惠價", rowNumber)
	}

	if value := get("qty"); value != "" {
		parsed, err := parseInteger(value)
		if err != nil {
			return order, fmt.Errorf("第 %d 列的數量格式錯誤：%w", rowNumber, err)
		}
		order.Qty = parsed
	} else {
		return order, fmt.Errorf("第 %d 列缺少數量", rowNumber)
	}

	return order, nil
}

func detectHeaderRow(rows [][]string) (map[string]int, int, error) {
	for idx, row := range rows {
		if !rowHasData(row) {
			continue
		}
		logRow("試驗標題列", idx+1, row)
		if headerIndex, missing, err := buildHeaderIndex(row); err == nil {
			return headerIndex, idx + 1, nil
		} else if len(missing) > 0 {
			log.Printf("試驗標題列 第 %d 列缺少欄位：%s", idx+1, strings.Join(missing, ", "))
		}
	}
	return nil, 0, fmt.Errorf("找不到包含完整標題列的資料")
}

func parseNumber(raw string) (float64, error) {
	clean := strings.ReplaceAll(strings.ReplaceAll(raw, ",", ""), "，", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, fmt.Errorf("空的數字欄位")
	}
	return strconv.ParseFloat(clean, 64)
}

func parseInteger(raw string) (int, error) {
	clean := strings.ReplaceAll(strings.ReplaceAll(raw, ",", ""), "，", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, fmt.Errorf("空的數字欄位")
	}

	if strings.Contains(clean, ".") {
		parsed, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			return 0, err
		}
		if math.Mod(parsed, 1) > 0 {
			return 0, fmt.Errorf("數量必須是整數")
		}
		return int(parsed), nil
	}

	return strconv.Atoi(clean)
}

func parseDateTime(raw string) (time.Time, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return time.Time{}, fmt.Errorf("空的日期欄位")
	}

	if numeric, err := strconv.ParseFloat(clean, 64); err == nil {
		if t, err := excelize.ExcelDateToTime(numeric, false); err == nil {
			return t, nil
		}
	}

	layouts := []string{
		"2006/01/02 15:04:05",
		"2006-01-02 15:04:05",
		"2006/01/02",
		"2006-01-02",
		"2006.01.02 15:04:05",
		"2006.1.2 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, clean); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("無法解析日期：%s", raw)
}

func saveUploadedOrders(db *gorm.DB, orders []UploadedOrder) error {
	if len(orders) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&orders).Error; err != nil {
			return err
		}
		return nil
	})
}

func clearUploadedOrders(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		switch tx.Dialector.Name() {
		case "postgres":
			if err := tx.Exec("TRUNCATE TABLE uploaded_orders RESTART IDENTITY").Error; err != nil {
				return err
			}
		default:
			if err := tx.Exec("DELETE FROM uploaded_orders").Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func buildUploadedOrderSummaries(rows []UploadedOrder) []UploadedOrderSummary {
	grouped := make(map[string]*UploadedOrderSummary)
	for _, row := range rows {
		if row.OrderNo == "" {
			continue
		}

		summary, exists := grouped[row.OrderNo]
		if !exists {
			summary = &UploadedOrderSummary{
				OrderNo:      row.OrderNo,
				OrderedAt:    row.OrderedAt,
				ReceiverName: row.ReceiverName,
				Address:      row.Address,
				Items:        []UploadedOrderItem{},
			}
			grouped[row.OrderNo] = summary
		}

		if !row.OrderedAt.IsZero() {
			if summary.OrderedAt.IsZero() || row.OrderedAt.Before(summary.OrderedAt) {
				summary.OrderedAt = row.OrderedAt
			}
		}

		if summary.ReceiverName == "" {
			summary.ReceiverName = row.ReceiverName
		}

		if summary.Address == "" {
			summary.Address = row.Address
		}

		summary.TotalQty += row.Qty
		summary.TotalAmount += row.DiscountPrice * float64(row.Qty)
		summary.Items = append(summary.Items, UploadedOrderItem{
			ProductName:   row.ProductName,
			UnitPrice:     row.UnitPrice,
			DiscountPrice: row.DiscountPrice,
			Qty:           row.Qty,
			Note:          row.Note,
		})
	}

	var summaries []UploadedOrderSummary
	for _, summary := range grouped {
		summaries = append(summaries, *summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].OrderedAt.After(summaries[j].OrderedAt)
	})

	return summaries
}

func buildProductPicking(rows []UploadedOrder) []ProductPickingItem {
	type accumulator struct {
		totalQty int
		orders   map[string]struct{}
	}

	byProduct := make(map[string]*accumulator)
	for _, row := range rows {
		if row.ProductName == "" {
			continue
		}

		name := strings.TrimSpace(row.ProductName)
		if name == "" {
			continue
		}

		acc, ok := byProduct[name]
		if !ok {
			acc = &accumulator{orders: map[string]struct{}{}}
			byProduct[name] = acc
		}

		acc.totalQty += row.Qty
		if row.OrderNo != "" {
			acc.orders[row.OrderNo] = struct{}{}
		}
	}

	var list []ProductPickingItem
	for productName, acc := range byProduct {
		orderNos := make([]string, 0, len(acc.orders))
		for no := range acc.orders {
			orderNos = append(orderNos, no)
		}
		sort.Strings(orderNos)
		list = append(list, ProductPickingItem{
			ProductName: productName,
			TotalQty:    acc.totalQty,
			OrderNos:    orderNos,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].TotalQty == list[j].TotalQty {
			return list[i].ProductName < list[j].ProductName
		}
		return list[i].TotalQty > list[j].TotalQty
	})

	return list
}

func recordUploadBatch(db *gorm.DB) error {
	return db.Create(&UploadBatch{UploadedAt: time.Now()}).Error
}

func getLastUploadTime(db *gorm.DB) (time.Time, error) {
	var batch UploadBatch
	err := db.Order("uploaded_at desc").First(&batch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return batch.UploadedAt, nil
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
	db.AutoMigrate(&ChecklistItem{}, &OrderMetadata{}, &UploadedOrder{}, &UploadBatch{})

	// --- Gin Router Setup ---
	r := gin.Default()

	// Helper function to serve HTML with no-cache headers
	serveHTML := func(c *gin.Context, filepath string) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File(filepath)
	}

	r.GET("/", func(c *gin.Context) {
		serveHTML(c, "./frontend/index.html")
	})
	r.GET("/index.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/index.html")
	})
	r.GET("/orders.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/orders.html")
	})
	r.GET("/picking.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/picking.html")
	})
	r.GET("/picking-list-print.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/picking-list-print.html")
	})
	r.GET("/sell-orders.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-orders.html")
	})
	r.GET("/sell-picking.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-picking.html")
	})
	r.POST("/orders/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file 欄位缺失"})
			return
		}

		if !strings.HasSuffix(strings.ToLower(file.Filename), ".xlsx") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "僅接受 .xlsx 檔案"})
			return
		}

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "無法讀取上傳檔案"})
			return
		}
		defer src.Close()

		xl, err := excelize.OpenReader(src)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("解析檔案失敗：%v", err)})
			return
		}

		sheetName := xl.GetSheetName(xl.GetActiveSheetIndex())
		if sheetName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "找不到有效的工作表"})
			return
		}

		rows, err := xl.GetRows(sheetName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "讀取工作表資料失敗"})
			return
		}

		if len(rows) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "沒有資料"})
			return
		}

		headerIndex, dataStartRow, err := detectHeaderRow(rows)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var orders []UploadedOrder
		for i := dataStartRow; i < len(rows); i++ {
			logRow("解析列", i+1, rows[i])
			if !rowHasData(rows[i]) {
				continue
			}
			order, err := parseUploadedRow(rows[i], headerIndex, i+1)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			orders = append(orders, order)
		}

		if err := saveUploadedOrders(db, orders); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if err := recordUploadBatch(db); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"rows": len(orders)})
	})

	r.GET("/orders/uploaded", func(c *gin.Context) {
		var stored []UploadedOrder
		if err := db.Order("id desc").Find(&stored).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, stored)
	})

	r.GET("/orders/uploaded/summary", func(c *gin.Context) {
		var stored []UploadedOrder
		if err := db.Order("ordered_at desc, id desc").Find(&stored).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, buildUploadedOrderSummaries(stored))
	})

	r.GET("/orders/picking", func(c *gin.Context) {
		var stored []UploadedOrder
		if err := db.Order("product_name, order_no").Find(&stored).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildProductPicking(stored))
	})

	r.GET("/orders/uploaded/last", func(c *gin.Context) {
		last, err := getLastUploadTime(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if last.IsZero() {
			c.JSON(http.StatusOK, gin.H{"last_uploaded_at": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"last_uploaded_at": last.Format(time.RFC3339)})
	})

	r.DELETE("/orders/uploaded", func(c *gin.Context) {
		if err := clearUploadedOrders(db); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "cleared"})
	})

	// Static files with cache control
	r.GET("/js/*filepath", func(c *gin.Context) {
		// Disable cache for JS files to avoid stale code
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File("./frontend/js/" + c.Param("filepath"))
	})

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

	api.POST("/orders/batch", func(c *gin.Context) {
		var requestBody struct {
			OrderIDs []int `json:"order_ids"`
		}

		if err := c.ShouldBindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		if len(requestBody.OrderIDs) == 0 {
			c.JSON(http.StatusOK, []WooOrder{})
			return
		}

		if len(requestBody.OrderIDs) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum 100 orders per request"})
			return
		}

		orders, err := fetchMultipleOrders(requestBody.OrderIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Get all metadata in one query
		var metadatas []OrderMetadata
		if len(requestBody.OrderIDs) > 0 {
			db.Where("order_id IN ?", requestBody.OrderIDs).Find(&metadatas)
		}

		// Create a map for easy lookup
		metadataMap := make(map[int]OrderMetadata)
		for _, m := range metadatas {
			metadataMap[m.OrderID] = m
		}

		// Merge metadata with orders
		for i := range orders {
			if m, ok := metadataMap[orders[i].ID]; ok {
				orders[i].OrderMetadata = m
			} else {
				// If metadata doesn't exist, create it
				newMeta := OrderMetadata{OrderID: orders[i].ID, Remark: "", Tags: []string{}}
				db.Create(&newMeta)
				orders[i].OrderMetadata = newMeta
			}
		}

		c.JSON(http.StatusOK, orders)
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
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	OrderIDs []int  `json:"order_ids"`
}

func generatePickingList(orders []WooOrder) []PickingListItem {
	pickingMap := make(map[string]*PickingListItem)

	for _, order := range orders {
		for _, item := range order.LineItems {
			if _, ok := pickingMap[item.Name]; !ok {
				pickingMap[item.Name] = &PickingListItem{
					Name:     item.Name,
					Quantity: 0,
					OrderIDs: []int{},
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

func fetchMultipleOrders(orderIDs []int) ([]WooOrder, error) {
	if len(orderIDs) == 0 {
		return []WooOrder{}, nil
	}

	// WooCommerce API supports fetching multiple orders using include parameter
	// Build comma-separated list of order IDs
	includeParam := ""
	for i, id := range orderIDs {
		if i > 0 {
			includeParam += ","
		}
		includeParam += strconv.Itoa(id)
	}

	url := fmt.Sprintf("https://flowers.fenny-studio.com/wp-json/wc/v3/orders?include=%s&per_page=%d", includeParam, len(orderIDs))

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
