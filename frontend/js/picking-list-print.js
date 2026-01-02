// 從 localStorage 或 postMessage 獲取訂單數據
let ordersReceived = false;

document.addEventListener("DOMContentLoaded", function () {
  // 顯示載入中的訊息
  document.getElementById('picking-list-content').innerHTML =
    '<div class="alert alert-info"><span class="spinner-border spinner-border-sm me-2"></span>正在載入訂單資料...</div>';

  // 先嘗試從 localStorage 獲取（向後兼容）
  const ordersData = localStorage.getItem('pickingListOrders');
  if (ordersData) {
    try {
      const orders = JSON.parse(ordersData);
      renderPickingList(orders);
      ordersReceived = true;
      // 清除 localStorage 中的資料（可選）
      // localStorage.removeItem('pickingListOrders');
      return;
    } catch (error) {
      console.error('解析 localStorage 訂單資料失敗:', error);
    }
  }

  // 設置超時，如果 10 秒內沒有收到資料，顯示錯誤
  const timeout = setTimeout(() => {
    if (!ordersReceived) {
      document.getElementById('picking-list-content').innerHTML =
        '<div class="alert alert-warning">等待訂單資料超時，請重試</div>';
    }
  }, 10000);

  // 清除超時計時器的函數
  window.clearLoadTimeout = () => clearTimeout(timeout);
});

// 監聽來自父視窗的訂單資料
window.addEventListener('message', function (event) {
  // 驗證訊息來源
  if (event.origin !== window.location.origin) {
    console.warn('Received message from unexpected origin:', event.origin);
    return;
  }

  if (event.data && event.data.type === 'pickingListOrders') {
    ordersReceived = true;
    if (window.clearLoadTimeout) {
      window.clearLoadTimeout();
    }

    try {
      renderPickingList(event.data.orders);
    } catch (error) {
      console.error('渲染訂單資料失敗:', error);
      document.getElementById('picking-list-content').innerHTML =
        '<div class="alert alert-danger">渲染訂單資料失敗</div>';
    }
  }
});

function renderPickingList(orders) {
  // 顯示匯出日期
  const exportDate = new Date().toLocaleString('zh-TW', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
  document.getElementById('export-date').textContent = `匯出日期：${exportDate}`;

  const container = document.getElementById('picking-list-content');
  container.innerHTML = '';

  orders.forEach((order, index) => {
    const orderSection = createOrderSection(order, index);
    container.appendChild(orderSection);
  });
}

function createOrderSection(order, index) {
  const section = document.createElement('div');
  section.className = 'order-section';

  // 訂單標題
  const header = document.createElement('div');
  header.className = 'order-header';
  header.innerHTML = `<h4>訂單 ID: ${order.id}</h4>`;
  section.appendChild(header);

  // 訂單資訊
  const orderInfo = document.createElement('div');
  orderInfo.className = 'order-info';

  const customerName = order.shipping?.first_name || '未知';
  const email = order.billing?.email || '無';
  const phone = order.billing?.phone || '無';
  const total = order.total || '0';
  const paymentMethod = order.payment_method_title || 'N/A';
  const dateCreated = order.date_created ? new Date(order.date_created).toLocaleString('zh-TW') : 'N/A';

  // 取得出貨方式
  let shippingMethod = '自行取貨';
  let pickupNumber = '';

  if (order.shipping_lines && order.shipping_lines.length > 0) {
    shippingMethod = order.shipping_lines[0].method_title || '自行取貨';
  }

  // 取得取貨單號（綠界超商取貨）
  const ecpayMeta = order.meta_data?.find(m => m.key === "_ecpay_shipping_info");
  if (ecpayMeta && typeof ecpayMeta.value === "object") {
    const firstKey = Object.keys(ecpayMeta.value)[0];
    const data = ecpayMeta.value[firstKey];
    const paymentNo = data.PaymentNo || "";
    const validationNo = data.ValidationNo || "";
    pickupNumber = paymentNo + validationNo;
  }

  orderInfo.innerHTML = `
    <div class="order-info-row">
      <div class="order-info-label">訂購人姓名：</div>
      <div>${customerName}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">訂單成立時間：</div>
      <div>${dateCreated}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">訂購人 Email：</div>
      <div>${email}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">訂購人電話：</div>
      <div>${phone}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">付款方式：</div>
      <div>${paymentMethod}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">訂單金額：</div>
      <div>${total}</div>
    </div>
    <div class="order-info-row">
      <div class="order-info-label">出貨方式：</div>
      <div>${shippingMethod}</div>
    </div>
    ${order.cvs_store_name ? `
    <div class="order-info-row">
      <div class="order-info-label">取貨門市：</div>
      <div>${order.cvs_store_name}</div>
    </div>
    ` : ''}
    ${pickupNumber ? `
    <div class="order-info-row">
      <div class="order-info-label">取貨單號：</div>
      <div>${pickupNumber}</div>
    </div>
    ` : ''}
    ${order.customer_note ? `
    <div class="order-info-row">
      <div class="order-info-label">客戶備註：</div>
      <div>${order.customer_note}</div>
    </div>
    ` : ''}
    ${order.order_metadata?.remark ? `
    <div class="order-info-row">
      <div class="order-info-label">系統備註：</div>
      <div>${order.order_metadata.remark}</div>
    </div>
    ` : ''}
  `;
  section.appendChild(orderInfo);

  // 產品表格
  const productsTable = createProductsTable(order.line_items || []);
  section.appendChild(productsTable);

  return section;
}

function createProductsTable(lineItems) {
  const table = document.createElement('table');
  table.className = 'products-table';

  // 表頭
  const thead = document.createElement('thead');
  thead.innerHTML = `
    <tr>
      <th style="width: 25%">商品名稱</th>
      <th style="width: 20%">規格名稱</th>
      <th style="width: 10%">數量</th>
      <th style="width: 10%">價格</th>
      <th style="width: 10%">總額</th>
      <th style="width: 10%">揀貨</th>
      <th style="width: 15%">備註</th>
    </tr>
  `;
  table.appendChild(thead);

  // 表身
  const tbody = document.createElement('tbody');

  if (lineItems.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="center">無商品</td></tr>';
  } else {
    lineItems.forEach(item => {
      const excludedMetaKeys = new Set([
        '_reduced_stock',
        '_advanced_woo_discount_item_total_discount',
        '_wdr_discounts'
      ]);
      const metas = item.meta_data || [];
      const metaText = metas
        .filter(m => !excludedMetaKeys.has(m.key))
        .map(m => {
          const key = m.display_key || m.key;
          const value = m.display_value || m.value;
          return `${key}: ${value}`;
        })
        .join(", ") || '無';

      const quantity = item.quantity || 0;
      const price = parseFloat(item.price) || 0;
      const subtotal = quantity * price;

      const row = document.createElement('tr');
      row.innerHTML = `
        <td>${item.name || '無'}</td>
        <td>${metaText}</td>
        <td class="center">${quantity}</td>
        <td class="right">${Math.round(price)}</td>
        <td class="right">${Math.round(subtotal)}</td>
        <td class="center"></td>
        <td></td>
      `;
      tbody.appendChild(row);
    });
  }

  table.appendChild(tbody);
  return table;
}
