# 上傳訂單 xlsx 實作文件（Golang + PostgreSQL）

## 1. 資料表設計

```sql
CREATE TABLE uploaded_orders (
  id              BIGSERIAL PRIMARY KEY,
  order_no        TEXT,
  ordered_at      TIMESTAMP,
  receiver_name   TEXT,
  address         TEXT,
  product_name    TEXT,
  unit_price      NUMERIC,
  discount_price  NUMERIC,
  qty             INT,
  note            TEXT
);
```

---

## 2. API 規格

### Endpoint

POST `/orders/upload`

### Request

* Content-Type: `multipart/form-data`
* 欄位：`file` (xlsx)

### Response

```json
{
  "rows": 123
}
```

### 錯誤類型

* 400：欄位缺失、格式錯誤
* 500：解析或 DB 錯誤

---

## 3. Handler 實作流程

1. 從 multipart 取得上傳檔案
2. 使用 `excelize` 讀取第一個 sheet
3. 依據標題列動態對應欄位（避免欄位順序變動）
4. 每列轉換成 struct：

```go
type UploadedOrder struct {
  OrderNo       string
  OrderedAt     time.Time
  ReceiverName  string
  Address       string
  ProductName   string
  UnitPrice     float64
  DiscountPrice float64
  Qty           int
  Note          string
}
```

5. 確認所有資料均解析成功後才進行 DB 寫入（避免 partial write）

---

## 4. PostgreSQL 寫入流程

使用 transaction + truncate 避免殘留舊資料：

```go
func SaveUploadedOrders(ctx context.Context, orders []UploadedOrder) error {
  tx, err := db.BeginTx(ctx, nil)
  if err != nil { return err }
  defer tx.Rollback()

  // 清除舊資料
  if _, err := tx.ExecContext(ctx,
    `TRUNCATE TABLE uploaded_orders RESTART IDENTITY`); err != nil {
    return err
  }

  // 預備語句
  stmt, err := tx.PrepareContext(ctx, `
    INSERT INTO uploaded_orders
    (order_no, ordered_at, receiver_name, address,
     product_name, unit_price, discount_price, qty, note)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
  `)
  if err != nil { return err }
  defer stmt.Close()

  // 寫入
  for _, o := range orders {
    _, err := stmt.ExecContext(ctx,
      o.OrderNo, o.OrderedAt, o.ReceiverName, o.Address,
      o.ProductName, o.UnitPrice, o.DiscountPrice, o.Qty, o.Note,
    )
    if err != nil {
      return err
    }
  }

  return tx.Commit()
}
```

---

## 5. 錯誤處理與驗證

### 必要欄位（需完全匹配）

* 訂單編號
* 訂購日期
* 收件人姓名
* 取件地址
* 商品名稱(品名/規格)
* 單價
* 優惠價
* 數量
* 訂單備註

### 檢查重點

* 檔案副檔名必須為 `.xlsx`
* 日期格式需能 parse
* 金額與數量需可轉換成 float/int
* 若任一列發生錯誤 → 整個上傳失敗（回 400）

---

## 6. 延伸設計建議

* 新增 `upload_batches` 表做上傳紀錄（例如：時間 / 檔案名 / 上傳人）
* 對 `order_no` 建 index 加快查詢
* 多筆寫入可改為 Bulk Insert 提升速度
* 將解析 + DB 寫入改用 background worker（若未來檔案更大）
