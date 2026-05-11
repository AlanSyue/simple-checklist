package main

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// --- Existing Checklist Models ---
type ChecklistItem struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	Text         string     `json:"text"`
	Checked      bool       `json:"checked"`
	ReminderDate *time.Time `json:"reminderDate,omitempty"`
	CreatedAt    time.Time  `json:"createdAt" gorm:"autoCreateTime"`
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

type OrderDisplayWhitelist struct {
	OrderID   int       `gorm:"primaryKey" json:"order_id"`
	CreatedAt time.Time `json:"created_at"`
}

type WooOrder struct {
	ID                 int            `json:"id"`
	Status             string         `json:"status"`
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
	CVSStoreName       string         `json:"cvs_store_name" gorm:"-"`
}

type BillingInfo struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type ShippingInfo struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
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
	DisplayValue any    `json:"display_value"`
}

type ShippingLine struct {
	MethodTitle string `json:"method_title"`
	MethodID    string `json:"method_id"`
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
	IsShipping    bool      `json:"is_shipping" gorm:"default:false"`
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

type CombinedPickingItem struct {
	ProductName    string `json:"product_name"`
	TotalQty       int    `json:"total_qty"`
	WooCommerceQty int    `json:"woocommerce_qty"`
	SellQty        int    `json:"sell_qty"`
	Sources        string `json:"sources"` // "官網", "賣貨便", or "官網 + 賣貨便"
}

type UploadBatch struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type ProductNameMapping struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	OriginalName string    `json:"original_name" gorm:"uniqueIndex:idx_name_source;not null"`
	Source       string    `json:"source" gorm:"uniqueIndex:idx_name_source;not null"` // "woocommerce" or "sell"
	MappedName   string    `json:"mapped_name" gorm:"not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
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

func getCVSStoreName(order *WooOrder) string {
	for _, meta := range order.MetaData {
		if meta.Key == "_shipping_cvs_store_name" {
			if str, ok := meta.Value.(string); ok {
				return str
			}
		}
	}
	return ""
}

type ecpayShippingInfo struct {
	LogisticsID      string
	LogisticsSubType string
	PaymentNo        string
	ValidationNo     string
}

func getEcpayShippingInfo(order *WooOrder) *ecpayShippingInfo {
	var (
		shippingMap      map[string]interface{}
		activeTrackingNo string // from top-level 運送編號 meta
	)
	for _, meta := range order.MetaData {
		switch meta.Key {
		case "_ecpay_shipping_info":
			if m, ok := meta.Value.(map[string]interface{}); ok {
				shippingMap = m
			}
		case "運送編號":
			if s, ok := meta.Value.(string); ok {
				activeTrackingNo = s
			}
		}
	}
	if shippingMap == nil || len(shippingMap) == 0 {
		return nil
	}

	buildInfo := func(k string, v interface{}) *ecpayShippingInfo {
		info := &ecpayShippingInfo{LogisticsID: k}
		if innerMap, ok := v.(map[string]interface{}); ok {
			if st, ok := innerMap["LogisticsSubType"].(string); ok {
				info.LogisticsSubType = st
			}
			if pn, ok := innerMap["PaymentNo"].(string); ok {
				info.PaymentNo = pn
			}
			if vn, ok := innerMap["ValidationNo"].(string); ok {
				info.ValidationNo = vn
			}
		}
		return info
	}

	// Phase 1: match active tracking number
	if activeTrackingNo != "" {
		for k, v := range shippingMap {
			info := buildInfo(k, v)
			if info.PaymentNo+info.ValidationNo == activeTrackingNo {
				return info
			}
		}
	}

	// Phase 2: fallback to largest LogisticsID (monotonically increasing = newest)
	var bestKey string
	var bestID int64 = -1
	for k := range shippingMap {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil {
			if id > bestID {
				bestID = id
				bestKey = k
			}
		}
	}
	if bestKey != "" {
		return buildInfo(bestKey, shippingMap[bestKey])
	}

	// Ultra fallback: if LogisticsID isn't numeric (shouldn't happen), grab any
	for k, v := range shippingMap {
		return buildInfo(k, v)
	}
	return nil
}

func ecpayPrintURL(logisticsSubType string) string {
	switch logisticsSubType {
	case "UNIMARTC2C":
		return "https://logistics.ecpay.com.tw/Express/PrintUniMartC2COrderInfo"
	case "FAMIC2C":
		return "https://logistics.ecpay.com.tw/Express/PrintFAMIC2COrderInfo"
	case "HILIFEC2C":
		return "https://logistics.ecpay.com.tw/Express/PrintHILIFEC2COrderInfo"
	case "OKMARTC2C":
		return "https://logistics.ecpay.com.tw/Express/PrintOKMARTC2COrderInfo"
	default:
		return "https://logistics.ecpay.com.tw/helper/printTradeDocument"
	}
}

func generateCheckMacValue(params map[string]string, hashKey, hashIV string) string {
	// 1. Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. Build key=value& string
	var buf strings.Builder
	buf.WriteString("HashKey=" + hashKey + "&")
	for i, k := range keys {
		buf.WriteString(k + "=" + params[k])
		if i < len(keys)-1 {
			buf.WriteString("&")
		}
	}
	buf.WriteString("&HashIV=" + hashIV)

	// 3. URL encode
	encoded := url.QueryEscape(buf.String())

	// 4. To lower case
	encoded = strings.ToLower(encoded)

	// 5. .NET-style URL encoding replacements
	encoded = strings.ReplaceAll(encoded, "%2d", "-")
	encoded = strings.ReplaceAll(encoded, "%5f", "_")
	encoded = strings.ReplaceAll(encoded, "%2e", ".")
	encoded = strings.ReplaceAll(encoded, "%21", "!")
	encoded = strings.ReplaceAll(encoded, "%2a", "*")
	encoded = strings.ReplaceAll(encoded, "%28", "(")
	encoded = strings.ReplaceAll(encoded, "%29", ")")

	// 6. MD5 hash → uppercase
	hash := md5.Sum([]byte(encoded))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// reverseString returns s with its bytes in reverse order. Used for building
// ECPay MerchantTradeNo in the same format as RY-Tools plugin (strrev of unix seconds).
// Safe for ASCII-only input (unix timestamp digits are ASCII).
func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// ReissueResult represents the outcome of a single reissue attempt.
type ReissueResult struct {
	OrderID      int    `json:"order_id"`
	Success      bool   `json:"success"`
	NewPaymentNo string `json:"new_payment_no,omitempty"`
	LogisticsID  string `json:"logistics_id,omitempty"`
	SubType      string `json:"sub_type,omitempty"`
	Error        string `json:"error,omitempty"`
}

// subTypeFromShippingMethod maps RY-Tools shipping method IDs to ECPay LogisticsSubType.
func subTypeFromShippingMethod(methodID string) string {
	switch methodID {
	case "ry_ecpay_shipping_cvs_711":
		return "UNIMARTC2C"
	case "ry_ecpay_shipping_cvs_family":
		return "FAMIC2C"
	case "ry_ecpay_shipping_cvs_hilife":
		return "HILIFEC2C"
	case "ry_ecpay_shipping_cvs_ok":
		return "OKMARTC2C"
	}
	return ""
}

// findMetaString extracts a string-valued meta by key from the order's meta_data.
func findMetaString(order *WooOrder, key string) string {
	for _, m := range order.MetaData {
		if m.Key == key {
			if s, ok := m.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// sanitizeReceiverName matches the RY plugin behaviour: pure ASCII letters → 10 chars,
// otherwise strip ASCII letters then truncate to 5 Chinese characters.
func sanitizeReceiverName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	isPureAscii := true
	for _, r := range name {
		if r > 127 {
			isPureAscii = false
			break
		}
	}
	if isPureAscii {
		runes := []rune(name)
		if len(runes) > 10 {
			runes = runes[:10]
		}
		return string(runes)
	}
	// strip ASCII letters and digits, keep CJK and symbols
	var b strings.Builder
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > 5 {
		runes = runes[:5]
	}
	return string(runes)
}

// truncateRunes truncates a string to at most n runes (character-count, Chinese-safe).
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		runes = runes[:n]
	}
	return string(runes)
}

// --- Reissue in-flight dedup (server-side idempotency) ---

var (
	reissueInFlight   = make(map[int]time.Time)
	reissueInFlightMu sync.Mutex
)

// claimReissueSlot returns false if the same orderID was claimed within the
// last 30 seconds, otherwise it claims the slot and returns true. It also
// opportunistically evicts entries older than 5 minutes.
func claimReissueSlot(orderID int) bool {
	reissueInFlightMu.Lock()
	defer reissueInFlightMu.Unlock()
	now := time.Now()
	if t, ok := reissueInFlight[orderID]; ok && now.Sub(t) < 30*time.Second {
		return false
	}
	reissueInFlight[orderID] = now
	for id, ts := range reissueInFlight {
		if now.Sub(ts) > 5*time.Minute {
			delete(reissueInFlight, id)
		}
	}
	return true
}

// putWooOrderRaw sends a PUT to the WooCommerce order endpoint with the given
// JSON body. Shared by putWooOrderField and putWooOrderMeta.
func putWooOrderRaw(orderID int, body []byte) error {
	putURL := fmt.Sprintf("https://flowers.fenny-studio.com/wp-json/wc/v3/orders/%d", orderID)
	req, err := http.NewRequest("PUT", putURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(os.Getenv("WOO_API_KEY"), os.Getenv("WOO_API_SECRET"))
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT WooCommerce: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("WooCommerce PUT %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// putWooOrderField PUTs a single top-level order field (e.g. customer_note).
func putWooOrderField(orderID int, field string, value interface{}) error {
	body := map[string]interface{}{field: value}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal PUT body: %w", err)
	}
	return putWooOrderRaw(orderID, jsonBody)
}

// putWooOrderMeta PUTs a meta_data slice to a WC order.
func putWooOrderMeta(orderID int, metaData []map[string]interface{}) error {
	body := map[string]interface{}{"meta_data": metaData}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal PUT body: %w", err)
	}
	return putWooOrderRaw(orderID, jsonBody)
}

// regenerateTracking calls ECPay Create API to reissue a shipping number for an order
// and writes the result back to the WooCommerce order's meta_data. Never panics and
// never returns an error — failures are reported via ReissueResult.Error.
func regenerateTracking(orderID int) ReissueResult {
	result := ReissueResult{OrderID: orderID}

	// 1. Env sanity.
	merchantID := os.Getenv("ECPAY_MERCHANT_ID")
	hashKey := os.Getenv("ECPAY_HASH_KEY")
	hashIV := os.Getenv("ECPAY_HASH_IV")
	senderName := os.Getenv("ECPAY_SENDER_NAME")
	senderPhone := os.Getenv("ECPAY_SENDER_PHONE")
	senderCell := os.Getenv("ECPAY_SENDER_CELLPHONE")
	if merchantID == "" || hashKey == "" || hashIV == "" {
		result.Error = "ECPay credentials not configured"
		return result
	}
	if senderName == "" || senderPhone == "" || senderCell == "" {
		result.Error = "ECPay sender info not configured (ECPAY_SENDER_NAME/PHONE/CELLPHONE)"
		return result
	}

	// 2. Server-side idempotency dedup (catches accidental double-POSTs).
	if !claimReissueSlot(orderID) {
		result.Error = "同一訂單 30 秒內已請求過重新取號，請稍候再試"
		return result
	}

	// 3. Fetch the current WC order.
	order, err := fetchSingleOrder(orderID)
	if err != nil {
		result.Error = "fetch order: " + err.Error()
		return result
	}

	// 4. Order status gate — block terminal/invalid statuses.
	blockedStatuses := map[string]bool{
		"cancelled": true,
		"refunded":  true,
		"trash":     true,
		"failed":    true,
	}
	if blockedStatuses[order.Status] {
		result.Error = fmt.Sprintf("訂單狀態為 %s，不允許重新取號", order.Status)
		return result
	}

	// 5. Shipping method gate — must be ECPay CVS, using the CURRENT
	//    shipping_lines (authoritative), not stale _ecpay_shipping_info meta.
	if len(order.ShippingLines) == 0 {
		result.Error = "訂單沒有運送項目，無法重新取號"
		return result
	}
	methodID := order.ShippingLines[0].MethodID
	if !strings.HasPrefix(methodID, "ry_ecpay_shipping_cvs_") {
		result.Error = fmt.Sprintf("此訂單的運送方式不是綠界超商取貨（目前：%s），無法重新取號", methodID)
		return result
	}
	subType := subTypeFromShippingMethod(methodID)
	if subType == "" {
		result.Error = fmt.Sprintf("不支援的綠界 C2C 子類型：%s", methodID)
		return result
	}

	// 6. Store ID check.
	storeID := findMetaString(&order, "_shipping_cvs_store_ID")
	if storeID == "" {
		result.Error = "missing _shipping_cvs_store_ID meta"
		return result
	}

	// 7. GoodsAmount / GoodsName / Receiver sanitization.
	goodsAmount := 0
	if f, perr := strconv.ParseFloat(order.Total, 64); perr == nil {
		goodsAmount = int(math.Round(f))
	}
	if goodsAmount <= 0 {
		result.Error = "invalid GoodsAmount from order.Total: " + order.Total
		return result
	}

	if len(order.LineItems) == 0 {
		result.Error = "order has no line items"
		return result
	}
	goodsName := truncateRunes(order.LineItems[0].Name, 20)

	receiverName := sanitizeReceiverName(order.Shipping.LastName + order.Shipping.FirstName)
	if receiverName == "" {
		result.Error = "empty receiver name"
		return result
	}

	receiverCell := strings.NewReplacer("-", "", " ", "").Replace(order.Shipping.Phone)
	if receiverCell == "" {
		result.Error = "empty receiver phone"
		return result
	}

	// 8. WC write permission pre-check — no-op PUT with current customer_note.
	// If this fails (e.g. read-only key), we never call ECPay.
	if err := putWooOrderField(orderID, "customer_note", order.CustomerNote); err != nil {
		result.Error = "WC write permission check failed: " + err.Error()
		return result
	}

	// 9. Build ECPay params.
	tz, _ := time.LoadLocation("Asia/Taipei")
	if tz == nil {
		tz = time.FixedZone("CST", 8*3600)
	}
	now := time.Now()
	// Match RY-Tools plugin's pre_generate_trade_no format so its callback handler
	// can parse the order ID via strrpos('TS') and correctly route status updates.
	// Plugin format: <orderPrefix><orderID>TS<1digit><reversed-unix-seconds>
	// We use empty prefix (plugin's configured prefix is empty on this shop).
	randDigit := rand.IntN(10) // 0-9
	reversed := reverseString(strconv.FormatInt(now.Unix(), 10))
	merchantTradeNo := fmt.Sprintf("%dTS%d%s", orderID, randDigit, reversed)
	merchantTradeDate := now.In(tz).Format("2006/01/02 15:04:05")

	params := map[string]string{
		"MerchantID":        merchantID,
		"MerchantTradeNo":   merchantTradeNo,
		"MerchantTradeDate": merchantTradeDate,
		"LogisticsType":     "CVS",
		"LogisticsSubType":  subType,
		"GoodsAmount":       strconv.Itoa(goodsAmount),
		"GoodsName":         goodsName,
		"IsCollection":      "N",
		"CollectionAmount":  "0",
		"SenderName":        senderName,
		"SenderPhone":       senderPhone,
		"SenderCellPhone":   senderCell,
		"ReceiverName":      receiverName,
		"ReceiverCellPhone": receiverCell,
		"ReceiverStoreID":   storeID,
		"ServerReplyURL":    "https://flowers.fenny-studio.com/wc-api/ry_ecpay_shipping_callback/",
	}
	params["CheckMacValue"] = generateCheckMacValue(params, hashKey, hashIV)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	// 10. Log before POST.
	log.Printf("[reissue] order=%d MerchantTradeNo=%s subType=%s storeID=%s goodsAmount=%d",
		orderID, merchantTradeNo, subType, storeID, goodsAmount)

	// 11. POST to ECPay.
	httpClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", "https://logistics.ecpay.com.tw/Express/Create", strings.NewReader(form.Encode()))
	if err != nil {
		result.Error = "build request: " + err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		result.Error = "ECPay request: " + err.Error()
		return result
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = "read ECPay response: " + err.Error()
		return result
	}
	bodyStr := string(bodyBytes)

	// 12. Log after POST (always — success or failure).
	log.Printf("[reissue] order=%d ECPay HTTP=%d body=%q", orderID, resp.StatusCode, bodyStr)

	// 13. Parse response.
	parts := strings.SplitN(bodyStr, "|", 2)
	if len(parts) != 2 {
		result.Error = "malformed ECPay response: " + bodyStr
		return result
	}
	if parts[0] != "1" {
		result.Error = "ECPay: " + parts[1]
		return result
	}

	values, perr := url.ParseQuery(parts[1])
	if perr != nil {
		result.Error = "parse ECPay data: " + perr.Error()
		return result
	}
	logisticsID := values.Get("AllPayLogisticsID")
	paymentNo := values.Get("CVSPaymentNo")
	validationNo := values.Get("CVSValidationNo")
	bookingNote := values.Get("BookingNote")
	respSubType := values.Get("LogisticsSubType")
	if respSubType != "" {
		subType = respSubType
	}
	if logisticsID == "" {
		result.Error = "ECPay returned empty AllPayLogisticsID: " + bodyStr
		return result
	}

	// 14. Log after parse with new IDs — critical for recovery if WC PUT fails.
	log.Printf("[reissue] order=%d ECPay OK LogisticsID=%s PaymentNo=%s ValidationNo=%s",
		orderID, logisticsID, paymentNo, validationNo)

	// 15. Merge new entry into _ecpay_shipping_info.
	nowStr := now.In(tz).Format("2006-01-02T15:04:05-07:00")
	newEntry := map[string]interface{}{
		"ID":               logisticsID,
		"LogisticsType":    "CVS",
		"LogisticsSubType": subType,
		"PaymentNo":        paymentNo,
		"ValidationNo":     validationNo,
		"store_ID":         storeID,
		"BookingNote":      bookingNote,
		"status":           300,
		"status_msg":       "已成功",
		"create":           nowStr,
		"edit":             nowStr,
		"amount":           goodsAmount,
		"IsCollection":     "N",
		"temp":             "1",
	}

	mergedMap := map[string]interface{}{}
	for _, meta := range order.MetaData {
		if meta.Key == "_ecpay_shipping_info" {
			if m, ok := meta.Value.(map[string]interface{}); ok {
				for k, v := range m {
					mergedMap[k] = v
				}
			}
		}
	}
	mergedMap[logisticsID] = newEntry

	combinedNo := paymentNo + validationNo

	// 16. PUT meta back to WC.
	if err := putWooOrderMeta(orderID, []map[string]interface{}{
		{"key": "_ecpay_shipping_info", "value": mergedMap},
		{"key": "運送編號", "value": combinedNo},
	}); err != nil {
		result.Error = err.Error()
		return result
	}

	// 17. Return success.
	result.Success = true
	result.NewPaymentNo = combinedNo
	result.LogisticsID = logisticsID
	result.SubType = subType
	return result
}

// followEcpayFormRedirect parses auto-submit form from ECPay HTML and POSTs to target URL.
// Uses the provided http.Client to maintain cookies across redirects.
func followEcpayFormRedirect(client *http.Client, htmlBody string) (string, error) {
	actionRe := regexp.MustCompile(`action="([^"]+)"`)
	actionMatch := actionRe.FindStringSubmatch(htmlBody)
	if actionMatch == nil {
		return "", fmt.Errorf("無法解析 form action")
	}
	targetURL := actionMatch[1]

	inputRe := regexp.MustCompile(`<input[^>]+name="([^"]+)"[^>]+value="([^"]*)"`)
	inputs := inputRe.FindAllStringSubmatch(htmlBody, -1)

	formData := url.Values{}
	for _, input := range inputs {
		formData.Set(input[1], input[2])
	}

	resp, err := client.PostForm(targetURL, formData)
	if err != nil {
		return "", fmt.Errorf("POST to %s failed: %w", targetURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	return string(body), nil
}

// newCookieClient creates an http.Client with a cookie jar for maintaining session cookies
func newCookieClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// patchUnimartHtml modifies 7-11 HTML for thermal label printing (100mm x 150mm)
func patchUnimartHtml(html string) string {
	// Fix relative CSS paths to absolute
	html = strings.ReplaceAll(html, `href="css/`, `href="https://epayment.7-11.com.tw/C2C/C2CWeb/css/`)

	// Inject thermal print CSS before </head>
	thermalCSS := `<style>
@media print {
    @page { size: 100mm 150mm; margin: 0; }
    body { margin: 0 !important; padding: 0 !important; }
    #Panel1 > table { border-collapse: collapse; width: auto; }
    #Panel1 > table > tbody > tr { display: block; }
    #Panel1 > table > tbody > tr > td {
        display: block !important;
        border: none !important;
        padding: 0 !important;
        page-break-after: always;
    }
    #Panel1 > table > tbody > tr > td:empty {
        display: none !important;
        page-break-after: avoid !important;
    }
    #Panel1 > table > tbody > tr > td > div {
        width: 100mm !important;
        height: 150mm !important;
        margin: 0 !important;
        border: none !important;
    }
    p[style*="page-break"] { display: none !important; }
}
#printPageButton { position: fixed; top: 10px; right: 10px; z-index: 9999; padding: 8px 16px; cursor: pointer; }
@media print { #printPageButton { display: none !important; } }
</style>`

	html = strings.Replace(html, "</head>", thermalCSS+"\n</head>", 1)

	// Add print button after <body> tag
	bodyRe := regexp.MustCompile(`(<body[^>]*>)`)
	html = bodyRe.ReplaceAllString(html, `${1}<button id="printPageButton" onclick="window.print();">列印</button>`)

	return html
}

func buildProductPicking(db *gorm.DB, rows []UploadedOrder) []ProductPickingItem {
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

		// 套用名稱 mapping
		mappedName := getMappedProductName(db, name, "sell")

		acc, ok := byProduct[mappedName]
		if !ok {
			acc = &accumulator{orders: map[string]struct{}{}}
			byProduct[mappedName] = acc
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

func buildCombinedPickingList(db *gorm.DB, wooOrders []WooOrder, sellOrders []UploadedOrder) []CombinedPickingItem {
	// Create a map to track product quantities by source
	type productData struct {
		wooQty  int
		sellQty int
	}

	productMap := make(map[string]*productData)

	// Process WooCommerce orders
	for _, order := range wooOrders {
		for _, item := range order.LineItems {
			mappedName := getMappedProductName(db, item.Name, "woocommerce")

			if _, ok := productMap[mappedName]; !ok {
				productMap[mappedName] = &productData{}
			}
			productMap[mappedName].wooQty += item.Quantity
		}
	}

	// Process Sell orders
	for _, row := range sellOrders {
		if row.ProductName == "" {
			continue
		}

		name := strings.TrimSpace(row.ProductName)
		if name == "" {
			continue
		}

		mappedName := getMappedProductName(db, name, "sell")

		if _, ok := productMap[mappedName]; !ok {
			productMap[mappedName] = &productData{}
		}
		productMap[mappedName].sellQty += row.Qty
	}

	// Build the combined list
	var list []CombinedPickingItem
	for productName, data := range productMap {
		sources := ""
		if data.wooQty > 0 && data.sellQty > 0 {
			sources = "官網 + 賣貨便"
		} else if data.wooQty > 0 {
			sources = "官網"
		} else {
			sources = "賣貨便"
		}

		list = append(list, CombinedPickingItem{
			ProductName:    productName,
			TotalQty:       data.wooQty + data.sellQty,
			WooCommerceQty: data.wooQty,
			SellQty:        data.sellQty,
			Sources:        sources,
		})
	}

	// Sort by total quantity (descending), then by product name
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

func syncProductNamesFromWooCommerce(db *gorm.DB) error {
	orders, err := fetchProcessingOrders()
	if err != nil {
		return err
	}

	productNames := make(map[string]bool)
	for _, order := range orders {
		for _, item := range order.LineItems {
			if item.Name != "" {
				productNames[item.Name] = true
			}
		}
	}

	for name := range productNames {
		var mapping ProductNameMapping
		result := db.Where("original_name = ? AND source = ?", name, "woocommerce").First(&mapping)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 如果不存在，建立新的 mapping，預設 mapped_name 就是原始名稱
			newMapping := ProductNameMapping{
				OriginalName: name,
				Source:       "woocommerce",
				MappedName:   name,
			}
			if err := db.Create(&newMapping).Error; err != nil {
				log.Printf("Failed to create mapping for %s: %v", name, err)
			}
		}
	}

	return nil
}

func syncProductNamesFromSell(db *gorm.DB) error {
	var orders []UploadedOrder
	if err := db.Find(&orders).Error; err != nil {
		return err
	}

	productNames := make(map[string]bool)
	for _, order := range orders {
		if order.ProductName != "" {
			productNames[order.ProductName] = true
		}
	}

	for name := range productNames {
		var mapping ProductNameMapping
		result := db.Where("original_name = ? AND source = ?", name, "sell").First(&mapping)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 如果不存在，建立新的 mapping，預設 mapped_name 就是原始名稱
			newMapping := ProductNameMapping{
				OriginalName: name,
				Source:       "sell",
				MappedName:   name,
			}
			if err := db.Create(&newMapping).Error; err != nil {
				log.Printf("Failed to create mapping for %s: %v", name, err)
			}
		}
	}

	return nil
}

func getMappedProductName(db *gorm.DB, originalName string, source string) string {
	var mapping ProductNameMapping
	if err := db.Where("original_name = ? AND source = ?", originalName, source).First(&mapping).Error; err == nil {
		return mapping.MappedName
	}
	return originalName
}

// --- Structs and functions for Product Search ---

type ProductSearchRequest struct {
	ProductNames         []string `json:"product_names"`
	Mode                 string   `json:"mode"` // "contains", "exact", "excludes"
	ExcludedProductNames []string `json:"excluded_product_names"`
}

// matchesProductCriteria is the core logic for matching products.
func matchesProductCriteria(orderProductSet map[string]bool, requiredProducts []string, mode string) bool {
	requiredProductSet := make(map[string]bool)
	for _, p := range requiredProducts {
		requiredProductSet[p] = true
	}

	if len(requiredProducts) == 0 {
		return true // No criteria means all orders match
	}

	switch mode {
	case "contains":
		for reqProduct := range requiredProductSet {
			if orderProductSet[reqProduct] {
				return true // OR logic: return true if any product matches
			}
		}
		return false // If loop completes, no matches were found
	case "exact":
		if len(orderProductSet) != len(requiredProductSet) {
			return false // Must have the exact same number of unique products
		}
		for reqProduct := range requiredProductSet {
			if !orderProductSet[reqProduct] {
				return false // And all required products must be present
			}
		}
		return true
	case "excludes":
		for reqProduct := range requiredProductSet {
			if orderProductSet[reqProduct] {
				return false // Must not contain any of the required products
			}
		}
		return true
	default:
		return false
	}
}

func filterWooOrdersByProducts(db *gorm.DB, orders []WooOrder, req ProductSearchRequest) []WooOrder {
	matchedOrders := make([]WooOrder, 0)

	for _, order := range orders {
		orderProductNames := make(map[string]bool)
		for _, item := range order.LineItems {
			mappedName := getMappedProductName(db, item.Name, "woocommerce")
			orderProductNames[mappedName] = true
		}

		if matchesProductCriteria(orderProductNames, req.ProductNames, req.Mode) {
			matchedOrders = append(matchedOrders, order)
		}
	}
	return matchedOrders
}

func filterSellOrdersByProducts(db *gorm.DB, summaries []UploadedOrderSummary, req ProductSearchRequest) []UploadedOrderSummary {
	matchedSummaries := make([]UploadedOrderSummary, 0)

	for _, summary := range summaries {
		orderProductNames := make(map[string]bool)
		for _, item := range summary.Items {
			mappedName := getMappedProductName(db, item.ProductName, "sell")
			orderProductNames[mappedName] = true
		}

		if matchesProductCriteria(orderProductNames, req.ProductNames, req.Mode) {
			matchedSummaries = append(matchedSummaries, summary)
		}
	}
	return matchedSummaries
}

func filterWooOrdersByExcludedProducts(db *gorm.DB, orders []WooOrder, excludedProductNames []string) []WooOrder {
	if len(excludedProductNames) == 0 {
		return orders
	}

	excludedSet := make(map[string]bool)
	for _, name := range excludedProductNames {
		excludedSet[name] = true
	}

	filteredOrders := make([]WooOrder, 0)
	for _, order := range orders {
		shouldExclude := false
		for _, item := range order.LineItems {
			mappedName := getMappedProductName(db, item.Name, "woocommerce")
			if excludedSet[mappedName] {
				shouldExclude = true
				break
			}
		}
		if !shouldExclude {
			filteredOrders = append(filteredOrders, order)
		}
	}
	return filteredOrders
}

func filterSellOrdersByExcludedProducts(db *gorm.DB, summaries []UploadedOrderSummary, excludedProductNames []string) []UploadedOrderSummary {
	if len(excludedProductNames) == 0 {
		return summaries
	}

	excludedSet := make(map[string]bool)
	for _, name := range excludedProductNames {
		excludedSet[name] = true
	}

	filteredSummaries := make([]UploadedOrderSummary, 0)
	for _, summary := range summaries {
		shouldExclude := false
		for _, item := range summary.Items {
			mappedName := getMappedProductName(db, item.ProductName, "sell")
			if excludedSet[mappedName] {
				shouldExclude = true
				break
			}
		}
		if !shouldExclude {
			filteredSummaries = append(filteredSummaries, summary)
		}
	}
	return filteredSummaries
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
	db.AutoMigrate(&ChecklistItem{}, &OrderMetadata{}, &UploadedOrder{}, &UploadBatch{}, &ProductNameMapping{}, &OrderDisplayWhitelist{})

	// --- Gin Router Setup ---
	r := gin.Default()

	// Helper function to serve HTML with no-cache headers
	serveHTML := func(c *gin.Context, filepath string) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File(filepath)
	}

	// Helper function to filter WooOrders by date
	filterWooOrdersByDate := func(orders []WooOrder, startDateStr, endDateStr string) []WooOrder {
		if startDateStr == "" && endDateStr == "" {
			return orders
		}

		var filtered []WooOrder
		for _, order := range orders {
			dateMatch := true
			orderDate, err := time.Parse("2006-01-02T15:04:05", order.DateCreated)
			if err != nil {
				// Try parsing with other formats if needed
			} else {
				if startDateStr != "" {
					startDate, err := time.Parse("2006-01-02", startDateStr)
					if err == nil {
						if orderDate.Before(startDate) {
							dateMatch = false
						}
					}
				}
				if endDateStr != "" && dateMatch {
					endDate, err := time.Parse("2006-01-02", endDateStr)
					if err == nil {
						endDate = endDate.Add(24 * time.Hour)
						if orderDate.After(endDate) {
							dateMatch = false
						}
					}
				}
			}
			if dateMatch {
				filtered = append(filtered, order)
			}
		}
		return filtered
	}

	// Helper function to filter UploadedOrders by date
	filterUploadedOrdersByDate := func(orders []UploadedOrder, startDateStr, endDateStr string) []UploadedOrder {
		if startDateStr == "" && endDateStr == "" {
			return orders
		}

		var filtered []UploadedOrder
		for _, order := range orders {
			dateMatch := true
			if startDateStr != "" {
				startDate, err := time.Parse("2006-01-02", startDateStr)
				if err == nil {
					if order.OrderedAt.Before(startDate) {
						dateMatch = false
					}
				}
			}
			if endDateStr != "" && dateMatch {
				endDate, err := time.Parse("2006-01-02", endDateStr)
				if err == nil {
					endDate = endDate.Add(24 * time.Hour)
					if order.OrderedAt.After(endDate) {
						dateMatch = false
					}
				}
			}
			if dateMatch {
				filtered = append(filtered, order)
			}
		}
		return filtered
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
	r.GET("/order-list-print.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/order-list-print.html")
	})
	r.GET("/sell-picking-list-print.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-picking-list-print.html")
	})
	r.GET("/sell-order-list-print.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-order-list-print.html")
	})
	r.GET("/sell-orders.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-orders.html")
	})
	r.GET("/sell-picking.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/sell-picking.html")
	})
	r.GET("/product-mapping.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/product-mapping.html")
	})
	r.GET("/combined-picking.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/combined-picking.html")
	})
	r.GET("/product-order-search.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/product-order-search.html")
	})
	r.GET("/shipping-orders.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/shipping-orders.html")
	})
	r.GET("/shipping-sell-orders.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/shipping-sell-orders.html")
	})
	r.GET("/shipping-combined-picking.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/shipping-combined-picking.html")
	})
	r.GET("/shipping-product-order-search.html", func(c *gin.Context) {
		serveHTML(c, "./frontend/shipping-product-order-search.html")
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

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filtered := filterUploadedOrdersByDate(stored, startDate, endDate)

		c.JSON(http.StatusOK, buildUploadedOrderSummaries(filtered))
	})

	r.GET("/orders/picking", func(c *gin.Context) {
		var stored []UploadedOrder
		if err := db.Order("product_name, order_no").Find(&stored).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filtered := filterUploadedOrdersByDate(stored, startDate, endDate)

		c.JSON(http.StatusOK, buildProductPicking(db, filtered))
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

	// --- Shipping Sell Orders Routes ---
	r.POST("/orders/upload-shipping", func(c *gin.Context) {
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
			order.IsShipping = true // Mark as shipping order
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

	r.GET("/orders/uploaded-shipping/summary", func(c *gin.Context) {
		var stored []UploadedOrder
		if err := db.Where("is_shipping = ?", true).Order("ordered_at desc, id desc").Find(&stored).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filtered := filterUploadedOrdersByDate(stored, startDate, endDate)

		c.JSON(http.StatusOK, buildUploadedOrderSummaries(filtered))
	})

	r.DELETE("/orders/uploaded-shipping", func(c *gin.Context) {
		if err := db.Where("is_shipping = ?", true).Delete(&UploadedOrder{}).Error; err != nil {
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
		var body struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}
		for _, itemData := range body.Items {
			newItem := ChecklistItem{
				Text:    itemData["text"].(string),
				Checked: false,
			}
			if checked, ok := itemData["checked"].(bool); ok {
				newItem.Checked = checked
			}
			if val, exists := itemData["reminderDate"]; exists {
				if reminderDateStr, ok := val.(string); ok && reminderDateStr != "" {
					if parsedTime, err := time.Parse(time.RFC3339, reminderDateStr); err == nil {
						newItem.ReminderDate = &parsedTime
					}
				}
			}
			db.Create(&newItem)
		}
		c.JSON(200, gin.H{"status": "saved"})
	})

	api.PATCH("/checklist/:id", func(c *gin.Context) {
		var update map[string]interface{}
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
		if checked, ok := update["checked"].(bool); ok {
			item.Checked = checked
		}
		if text, ok := update["text"].(string); ok {
			item.Text = text
		}
		if reminderDateStr, ok := update["reminderDate"].(string); ok {
			if reminderDateStr != "" {
				if parsedTime, err := time.Parse(time.RFC3339, reminderDateStr); err == nil {
					item.ReminderDate = &parsedTime
				}
			} else {
				item.ReminderDate = nil
			}
		} else if _, exists := update["reminderDate"]; exists && update["reminderDate"] == nil {
			item.ReminderDate = nil
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
		var whitelist []OrderDisplayWhitelist
		if err := db.Order("order_id ASC").Find(&whitelist).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(whitelist) == 0 {
			c.JSON(http.StatusOK, []WooOrder{})
			return
		}
		whitelistIDs := make([]int, len(whitelist))
		for i, w := range whitelist {
			whitelistIDs[i] = w.OrderID
		}

		fetched, err := fetchMultipleOrders(whitelistIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		wooOrders := make([]WooOrder, 0, len(fetched))
		for _, o := range fetched {
			if o.Status == "processing" {
				wooOrders = append(wooOrders, o)
			}
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
			order.CVSStoreName = getCVSStoreName(&order)

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

			// Apply date range filter
			dateMatch := true
			startDateStr := c.Query("start_date")
			endDateStr := c.Query("end_date")

			if startDateStr != "" || endDateStr != "" {
				orderDate, err := time.Parse("2006-01-02T15:04:05", order.DateCreated)
				if err != nil {
					// Try parsing with other formats if needed, or log error
					// For now, assume standard WooCommerce format or skip if parse fails
					// log.Printf("Error parsing date for order %d: %v", order.ID, err)
				} else {
					if startDateStr != "" {
						startDate, err := time.Parse("2006-01-02", startDateStr)
						if err == nil {
							// Start of the day
							if orderDate.Before(startDate) {
								dateMatch = false
							}
						}
					}
					if endDateStr != "" && dateMatch {
						endDate, err := time.Parse("2006-01-02", endDateStr)
						if err == nil {
							// End of the day (add 24 hours)
							endDate = endDate.Add(24 * time.Hour)
							if orderDate.After(endDate) {
								dateMatch = false
							}
						}
					}
				}
			}

			if tagMatch && remarkMatch && customerNoteMatch && dateMatch {
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
		wooOrder.CVSStoreName = getCVSStoreName(&wooOrder)

		c.JSON(http.StatusOK, wooOrder)
	})

	api.POST("/orders/regenerate-tracking", func(c *gin.Context) {
		var req struct {
			OrderIDs []int `json:"order_ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		if len(req.OrderIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "order_ids is empty"})
			return
		}
		results := make([]ReissueResult, 0, len(req.OrderIDs))
		for _, id := range req.OrderIDs {
			results = append(results, regenerateTracking(id))
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
	})

	api.GET("/orders/:id/print-label", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
			return
		}

		merchantID := os.Getenv("ECPAY_MERCHANT_ID")
		hashKey := os.Getenv("ECPAY_HASH_KEY")
		hashIV := os.Getenv("ECPAY_HASH_IV")
		if merchantID == "" || hashKey == "" || hashIV == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ECPay credentials not configured"})
			return
		}

		wooOrder, err := fetchSingleOrder(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order: " + err.Error()})
			return
		}

		info := getEcpayShippingInfo(&wooOrder)
		if info == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "此訂單沒有綠界物流資訊"})
			return
		}

		params := map[string]string{
			"MerchantID":        merchantID,
			"AllPayLogisticsID": info.LogisticsID,
		}

		// C2C endpoints require CVSPaymentNo; UniMart C2C also requires CVSValidationNo
		isC2C := strings.HasSuffix(info.LogisticsSubType, "C2C")
		if isC2C {
			params["CVSPaymentNo"] = info.PaymentNo
			if info.LogisticsSubType == "UNIMARTC2C" {
				params["CVSValidationNo"] = info.ValidationNo
			}
		}

		checkMacValue := generateCheckMacValue(params, hashKey, hashIV)
		printURL := ecpayPrintURL(info.LogisticsSubType)

		// Server-side proxy: POST to ECPay and parse response
		formData := url.Values{}
		for k, v := range params {
			formData.Set(k, v)
		}
		formData.Set("CheckMacValue", checkMacValue)

		resp, err := http.PostForm(printURL, formData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "無法連線綠界列印服務: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "讀取綠界回應失敗: " + err.Error()})
			return
		}

		// UNIMARTC2C single order: redirect chain is too complex (4 steps + JS),
		// fall back to browser form-submit approach
		if info.LogisticsSubType == "UNIMARTC2C" {
			var fields strings.Builder
			for k, v := range params {
				fields.WriteString(fmt.Sprintf("  <input type=\"hidden\" name=\"%s\" value=\"%s\">\n", k, v))
			}
			fields.WriteString(fmt.Sprintf("  <input type=\"hidden\" name=\"CheckMacValue\" value=\"%s\">\n", checkMacValue))

			html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body>
<form id="ecpay-form" method="POST" action="%s">
%s</form>
<script>document.getElementById('ecpay-form').submit();</script>
</body>
</html>`, printURL, fields.String())

			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
			return
		}

		// Extract img src URLs from ECPay response HTML (for non-UNIMART)
		imgRe := regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
		matches := imgRe.FindAllStringSubmatch(string(body), -1)
		if len(matches) == 0 {
			// Debug: return raw ECPay HTML to inspect structure
			c.Data(http.StatusOK, "text/html; charset=utf-8", body)
			return
		}

		var labelPages strings.Builder
		for _, m := range matches {
			labelPages.WriteString(fmt.Sprintf("<div class=\"label-page\"><img src=\"%s\"></div>\n", m[1]))
		}

		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<style>
@media print {
    @page {
        size: 100mm 150mm;
        margin: 0;
    }
    body { margin: 0; padding: 0; }
    .label-page {
        page-break-after: always;
        width: 100mm;
        height: 150mm;
        overflow: hidden;
    }
    .label-page:last-child { page-break-after: avoid; }
    .label-page img {
        width: 175%%;
        max-width: none;
    }
    #printPageButton { display: none; }
}
body { margin: 20px; text-align: center; }
.label-page {
    margin: 10px auto;
    border: 1px dashed #ccc;
    padding: 0;
    max-width: 100mm;
    overflow: hidden;
}
.label-page img { width: 175%%; max-width: none; }
</style>
</head>
<body>
<button id="printPageButton" onclick="window.print();">列印</button>
%s
</body>
</html>`, labelPages.String())

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})

	api.GET("/orders/batch-print-label", func(c *gin.Context) {
		idsParam := c.Query("ids")
		if idsParam == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "請選擇至少一筆訂單"})
			return
		}
		idStrs := strings.Split(idsParam, ",")
		var orderIDs []int
		for _, s := range idStrs {
			id, err := strconv.Atoi(strings.TrimSpace(s))
			if err == nil {
				orderIDs = append(orderIDs, id)
			}
		}
		if len(orderIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "請選擇至少一筆訂單"})
			return
		}
		requestBody := struct{ OrderIDs []int }{OrderIDs: orderIDs}

		merchantID := os.Getenv("ECPAY_MERCHANT_ID")
		hashKey := os.Getenv("ECPAY_HASH_KEY")
		hashIV := os.Getenv("ECPAY_HASH_IV")
		if merchantID == "" || hashKey == "" || hashIV == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ECPay credentials not configured"})
			return
		}

		// Group orders by LogisticsSubType, collect comma-separated values
		type printGroup struct {
			logisticsIDs  []string
			paymentNos    []string
			validationNos []string
			subType       string
		}
		groups := make(map[string]*printGroup)

		for _, orderID := range requestBody.OrderIDs {
			wooOrder, err := fetchSingleOrder(orderID)
			if err != nil {
				continue
			}
			info := getEcpayShippingInfo(&wooOrder)
			if info == nil {
				continue
			}
			g, ok := groups[info.LogisticsSubType]
			if !ok {
				g = &printGroup{subType: info.LogisticsSubType}
				groups[info.LogisticsSubType] = g
			}
			g.logisticsIDs = append(g.logisticsIDs, info.LogisticsID)
			g.paymentNos = append(g.paymentNos, info.PaymentNo)
			g.validationNos = append(g.validationNos, info.ValidationNo)
		}

		if len(groups) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "所選訂單都沒有綠界物流資訊"})
			return
		}

		// Server-side proxy: POST to ECPay for each group and collect label content
		imgRe := regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
		var allImageURLs []string

		for _, g := range groups {
			if g.subType == "UNIMARTC2C" {
				// UNIMARTC2C: send all IDs together, follow redirect to 7-11
				params := map[string]string{
					"MerchantID":        merchantID,
					"AllPayLogisticsID": strings.Join(g.logisticsIDs, ","),
					"CVSPaymentNo":      strings.Join(g.paymentNos, ","),
					"CVSValidationNo":   strings.Join(g.validationNos, ","),
				}
				checkMacValue := generateCheckMacValue(params, hashKey, hashIV)
				printURL := ecpayPrintURL(g.subType)

				formData := url.Values{}
				for k, v := range params {
					formData.Set(k, v)
				}
				formData.Set("CheckMacValue", checkMacValue)

				client := newCookieClient()
				resp, err := client.PostForm(printURL, formData)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "無法連線綠界列印服務: " + err.Error()})
					return
				}
				ecpayBody, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "讀取綠界回應失敗: " + err.Error()})
					return
				}

				sevenHtml, err := followEcpayFormRedirect(client, string(ecpayBody))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "無法取得 7-11 列印頁面: " + err.Error()})
					return
				}

				c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(patchUnimartHtml(sevenHtml)))
				return
			}

			// Other logistics types: extract <img> one by one
			for i := 0; i < len(g.logisticsIDs); i++ {
				end := i + 1
				if end > len(g.logisticsIDs) {
					end = len(g.logisticsIDs)
				}

				params := map[string]string{
					"MerchantID":        merchantID,
					"AllPayLogisticsID": strings.Join(g.logisticsIDs[i:end], ","),
				}

				isC2C := strings.HasSuffix(g.subType, "C2C")
				if isC2C {
					params["CVSPaymentNo"] = strings.Join(g.paymentNos[i:end], ",")
				}

				checkMacValue := generateCheckMacValue(params, hashKey, hashIV)
				printURL := ecpayPrintURL(g.subType)

				formData := url.Values{}
				for k, v := range params {
					formData.Set(k, v)
				}
				formData.Set("CheckMacValue", checkMacValue)

				resp, err := http.PostForm(printURL, formData)
				if err != nil {
					log.Printf("批次列印: 無法連線綠界列印服務: %v", err)
					continue
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					log.Printf("批次列印: 讀取綠界回應失敗: %v", err)
					continue
				}

				matches := imgRe.FindAllStringSubmatch(string(body), -1)
				for _, m := range matches {
					allImageURLs = append(allImageURLs, m[1])
				}
			}
		}

		if len(allImageURLs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "無法從綠界回應中解析任何標籤圖片"})
			return
		}

		var labelPages strings.Builder
		for _, imgURL := range allImageURLs {
			labelPages.WriteString(fmt.Sprintf("<div class=\"label-page\"><img src=\"%s\"></div>\n", imgURL))
		}

		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<style>
@media print {
    @page {
        size: 100mm 150mm;
        margin: 0;
    }
    body { margin: 0; padding: 0; }
    .label-page {
        page-break-after: always;
        width: 100mm;
        height: 150mm;
        overflow: hidden;
    }
    .label-page:last-child { page-break-after: avoid; }
    .label-page img {
        width: 175%%;
        max-width: none;
    }
    .label-html iframe {
        width: 100%% !important;
        height: 100%% !important;
    }
    #printPageButton { display: none; }
}
body { margin: 20px; text-align: center; }
.label-page {
    margin: 10px auto;
    border: 1px dashed #ccc;
    padding: 0;
    max-width: 100mm;
    overflow: hidden;
}
.label-page img { width: 175%%; max-width: none; }
</style>
</head>
<body>
<button id="printPageButton" onclick="window.print();">列印</button>
%s
</body>
</html>`, labelPages.String())

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
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
			orders[i].CVSStoreName = getCVSStoreName(&orders[i])
		}

		c.JSON(http.StatusOK, orders)
	})

	api.POST("/orders/whitelist", func(c *gin.Context) {
		var body struct {
			IDs []int `json:"ids"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		seen := make(map[int]bool)
		dedup := make([]int, 0, len(body.IDs))
		for _, id := range body.IDs {
			if !seen[id] {
				seen[id] = true
				dedup = append(dedup, id)
			}
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("1 = 1").Delete(&OrderDisplayWhitelist{}).Error; err != nil {
				return err
			}
			if len(dedup) == 0 {
				return nil
			}
			records := make([]OrderDisplayWhitelist, len(dedup))
			now := time.Now()
			for i, id := range dedup {
				records[i] = OrderDisplayWhitelist{OrderID: id, CreatedAt: now}
			}
			return tx.Create(&records).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"count": len(dedup)})
	})

	api.GET("/orders/whitelist", func(c *gin.Context) {
		var whitelist []OrderDisplayWhitelist
		if err := db.Order("order_id ASC").Find(&whitelist).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ids := make([]int, len(whitelist))
		for i, w := range whitelist {
			ids[i] = w.OrderID
		}
		c.JSON(http.StatusOK, gin.H{"ids": ids})
	})

	api.DELETE("/orders/whitelist", func(c *gin.Context) {
		if err := db.Where("1 = 1").Delete(&OrderDisplayWhitelist{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Route for searching orders by products
	api.POST("/orders/search-by-products", func(c *gin.Context) {
		var req ProductSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Fetch all necessary data
		wooOrders, err := fetchProcessingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch WooCommerce orders: " + err.Error()})
			return
		}

		var sellOrderRows []UploadedOrder
		if err := db.Order("ordered_at desc, id desc").Find(&sellOrderRows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sell orders: " + err.Error()})
			return
		}
		sellOrderSummaries := buildUploadedOrderSummaries(sellOrderRows)

		// Filter orders by inclusion criteria
		matchedWooOrders := filterWooOrdersByProducts(db, wooOrders, req)
		matchedSellOrders := filterSellOrdersByProducts(db, sellOrderSummaries, req)

		// Filter orders by exclusion criteria
		finalWooOrders := filterWooOrdersByExcludedProducts(db, matchedWooOrders, req.ExcludedProductNames)
		finalSellOrders := filterSellOrdersByExcludedProducts(db, matchedSellOrders, req.ExcludedProductNames)

		// Sort WooCommerce orders by date_created (ascending)
		sort.Slice(finalWooOrders, func(i, j int) bool {
			dateI, errI := time.Parse("2006-01-02T15:04:05", finalWooOrders[i].DateCreated)
			dateJ, errJ := time.Parse("2006-01-02T15:04:05", finalWooOrders[j].DateCreated)
			if errI != nil || errJ != nil {
				return false
			}
			return dateI.Before(dateJ)
		})

		// Sort Sell orders by ordered_at (ascending)
		sort.Slice(finalSellOrders, func(i, j int) bool {
			return finalSellOrders[i].OrderedAt.Before(finalSellOrders[j].OrderedAt)
		})

		c.JSON(http.StatusOK, gin.H{
			"woo_orders":  finalWooOrders,
			"sell_orders": finalSellOrders,
		})
	})

	// --- Shipping Orders Routes (prepare-stock status) ---
	api.GET("/shipping-orders", func(c *gin.Context) {
		wooOrders, err := fetchShippingOrders()
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
		requestedTags := c.QueryArray("tags")
		hasRemark := c.Query("has_remark") == "true"
		hasCustomerNote := c.Query("has_customer_note") == "true"

		for _, order := range wooOrders {
			var metadata OrderMetadata
			if m, ok := metadataMap[order.ID]; ok {
				metadata = m
			} else {
				newMeta := OrderMetadata{OrderID: order.ID, Remark: "", Tags: []string{}}
				db.Create(&newMeta)
				metadata = newMeta
			}
			order.OrderMetadata = metadata
			order.CVSStoreName = getCVSStoreName(&order)

			// Apply tag filtering (OR logic)
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

			// Apply date range filter
			dateMatch := true
			startDateStr := c.Query("start_date")
			endDateStr := c.Query("end_date")

			if startDateStr != "" || endDateStr != "" {
				orderDate, err := time.Parse("2006-01-02T15:04:05", order.DateCreated)
				if err == nil {
					if startDateStr != "" {
						startDate, err := time.Parse("2006-01-02", startDateStr)
						if err == nil {
							if orderDate.Before(startDate) {
								dateMatch = false
							}
						}
					}
					if endDateStr != "" && dateMatch {
						endDate, err := time.Parse("2006-01-02", endDateStr)
						if err == nil {
							endDate = endDate.Add(24 * time.Hour)
							if orderDate.After(endDate) {
								dateMatch = false
							}
						}
					}
				}
			}

			if tagMatch && remarkMatch && customerNoteMatch && dateMatch {
				filteredWooOrders = append(filteredWooOrders, order)
			}
		}

		c.JSON(http.StatusOK, filteredWooOrders)
	})

	api.PUT("/shipping-orders/:id", func(c *gin.Context) {
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

		metadataUpdate.OrderID = id

		if err := db.Save(&metadataUpdate).Error; err != nil {
			log.Printf("Failed to save metadata for order %d: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save metadata: %v", err)})
			return
		}

		c.JSON(http.StatusOK, metadataUpdate)
	})

	api.POST("/shipping-orders/search-by-products", func(c *gin.Context) {
		var req ProductSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Fetch shipping orders
		wooOrders, err := fetchShippingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch WooCommerce orders: " + err.Error()})
			return
		}

		var sellOrderRows []UploadedOrder
		if err := db.Where("is_shipping = ?", true).Order("ordered_at desc, id desc").Find(&sellOrderRows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sell orders: " + err.Error()})
			return
		}
		sellOrderSummaries := buildUploadedOrderSummaries(sellOrderRows)

		// Filter orders by products
		matchedWooOrders := filterWooOrdersByProducts(db, wooOrders, req)
		matchedSellOrders := filterSellOrdersByProducts(db, sellOrderSummaries, req)

		// Filter by excluded products
		finalWooOrders := filterWooOrdersByExcludedProducts(db, matchedWooOrders, req.ExcludedProductNames)
		finalSellOrders := filterSellOrdersByExcludedProducts(db, matchedSellOrders, req.ExcludedProductNames)

		// Sort by date
		sort.Slice(finalWooOrders, func(i, j int) bool {
			dateI, errI := time.Parse("2006-01-02T15:04:05", finalWooOrders[i].DateCreated)
			dateJ, errJ := time.Parse("2006-01-02T15:04:05", finalWooOrders[j].DateCreated)
			if errI != nil || errJ != nil {
				return false
			}
			return dateI.Before(dateJ)
		})

		sort.Slice(finalSellOrders, func(i, j int) bool {
			return finalSellOrders[i].OrderedAt.Before(finalSellOrders[j].OrderedAt)
		})

		c.JSON(http.StatusOK, gin.H{
			"woo_orders":  finalWooOrders,
			"sell_orders": finalSellOrders,
		})
	})

	api.GET("/shipping-picking-list", func(c *gin.Context) {
		orders, err := fetchShippingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filtered := filterWooOrdersByDate(orders, startDate, endDate)

		pickingList := generatePickingList(db, filtered)
		c.JSON(http.StatusOK, pickingList)
	})

	api.GET("/shipping-combined-picking-list", func(c *gin.Context) {
		// Fetch WooCommerce shipping orders
		wooOrders, err := fetchShippingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch WooCommerce orders: %v", err)})
			return
		}

		// Fetch uploaded Sell orders (is_shipping = true)
		var sellOrderRows []UploadedOrder
		if err := db.Where("is_shipping = ?", true).Order("ordered_at desc, id desc").Find(&sellOrderRows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch sell orders: %v", err)})
			return
		}

		// Apply date filter
		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filteredWooOrders := filterWooOrdersByDate(wooOrders, startDate, endDate)
		filteredSellOrders := filterUploadedOrdersByDate(sellOrderRows, startDate, endDate)

		// Build combined picking list
		combinedList := buildCombinedPickingList(db, filteredWooOrders, filteredSellOrders)

		c.JSON(http.StatusOK, combinedList)
	})

	// --- Processing Orders Routes ---
	api.GET("/picking-list", func(c *gin.Context) {
		orders, err := fetchProcessingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")
		filtered := filterWooOrdersByDate(orders, startDate, endDate)

		pickingList := generatePickingList(db, filtered)
		c.JSON(http.StatusOK, pickingList)
	})

	api.GET("/combined-picking-list", func(c *gin.Context) {
		// Fetch WooCommerce processing orders
		wooOrders, err := fetchProcessingOrders()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch WooCommerce orders: %v", err)})
			return
		}

		// Fetch uploaded Sell orders
		var sellOrders []UploadedOrder
		if err := db.Order("product_name, order_no").Find(&sellOrders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch Sell orders: %v", err)})
			return
		}

		startDate := c.Query("start_date")
		endDate := c.Query("end_date")

		filteredWoo := filterWooOrdersByDate(wooOrders, startDate, endDate)
		filteredSell := filterUploadedOrdersByDate(sellOrders, startDate, endDate)

		// Build combined picking list
		combinedList := buildCombinedPickingList(db, filteredWoo, filteredSell)
		c.JSON(http.StatusOK, combinedList)
	})

	// Product Name Mapping Routes
	api.GET("/product-mappings", func(c *gin.Context) {
		var mappings []ProductNameMapping
		if err := db.Order("source, original_name").Find(&mappings).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, mappings)
	})

	api.PUT("/product-mappings/:id", func(c *gin.Context) {
		id := c.Param("id")
		var requestBody struct {
			MappedName string `json:"mapped_name"`
		}

		if err := c.ShouldBindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}

		if requestBody.MappedName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "mapped_name cannot be empty"})
			return
		}

		var mapping ProductNameMapping
		if err := db.First(&mapping, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mapping not found"})
			return
		}

		mapping.MappedName = requestBody.MappedName
		if err := db.Save(&mapping).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, mapping)
	})

	api.POST("/product-mappings/sync", func(c *gin.Context) {
		if err := syncProductNamesFromWooCommerce(db); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync WooCommerce products: " + err.Error()})
			return
		}

		if err := syncProductNamesFromSell(db); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync Sell products: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Product names synchronized successfully"})
	})

	r.Run(":8080")
}

type PickingListItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	OrderIDs []int  `json:"order_ids"`
}

func generatePickingList(db *gorm.DB, orders []WooOrder) []PickingListItem {
	pickingMap := make(map[string]*PickingListItem)

	for _, order := range orders {
		for _, item := range order.LineItems {
			// Apply product name mapping
			mappedName := getMappedProductName(db, item.Name, "woocommerce")

			if _, ok := pickingMap[mappedName]; !ok {
				pickingMap[mappedName] = &PickingListItem{
					Name:     mappedName,
					Quantity: 0,
					OrderIDs: []int{},
				}
			}
			pickingMap[mappedName].Quantity += item.Quantity
			pickingMap[mappedName].OrderIDs = append(pickingMap[mappedName].OrderIDs, order.ID)
		}
	}

	var pickingList []PickingListItem
	for _, item := range pickingMap {
		pickingList = append(pickingList, *item)
	}

	return pickingList
}

func fetchProcessingOrders() ([]WooOrder, error) {
	return fetchOrdersByStatus("processing")
}

func fetchShippingOrders() ([]WooOrder, error) {
	return fetchOrdersByStatus("prepare-stock")
}

func fetchOrdersByStatus(status string) ([]WooOrder, error) {
	var allOrders []WooOrder
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://flowers.fenny-studio.com/wp-json/wc/v3/orders?status=%s&per_page=%d&page=%d", status, perPage, page)

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

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("received non-200 status code: %d - %s", resp.StatusCode, string(bodyBytes))
		}

		var orders []WooOrder
		if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("error decoding response: %w", err)
		}
		resp.Body.Close()

		// 將這一頁的訂單加入總列表
		allOrders = append(allOrders, orders...)

		log.Printf("已取得第 %d 頁，共 %d 筆訂單（本頁 %d 筆）[狀態: %s]", page, len(allOrders), len(orders), status)

		// 如果這頁的訂單數少於 per_page，表示已經是最後一頁
		if len(orders) < perPage {
			break
		}

		page++
	}

	log.Printf("總共取得 %d 筆 processing 訂單", len(allOrders))
	return allOrders, nil
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
