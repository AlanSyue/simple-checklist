// 從 localStorage 獲取訂單數據
document.addEventListener("DOMContentLoaded", function() {
  const ordersData = localStorage.getItem('pickingListOrders');

  if (!ordersData) {
    document.getElementById('picking-list-content').innerHTML =
      '<div class="alert alert-warning">無法載入訂單資料</div>';
    return;
  }

  try {
    const orders = JSON.parse(ordersData);
    renderPickingList(orders);

    // 清除 localStorage 中的資料（可選）
    // localStorage.removeItem('pickingListOrders');
  } catch (error) {
    console.error('解析訂單資料失敗:', error);
    document.getElementById('picking-list-content').innerHTML =
      '<div class="alert alert-danger">訂單資料格式錯誤</div>';
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
      <th style="width: 30%">商品名稱</th>
      <th style="width: 25%">規格名稱</th>
      <th style="width: 10%">數量</th>
      <th style="width: 10%">價格</th>
      <th style="width: 10%">揀貨</th>
      <th style="width: 15%">備註</th>
    </tr>
  `;
  table.appendChild(thead);

  // 表身
  const tbody = document.createElement('tbody');

  if (lineItems.length === 0) {
    tbody.innerHTML = '<tr><td colspan="6" class="center">無商品</td></tr>';
  } else {
    lineItems.forEach(item => {
      const metas = item.meta_data || [];
      const metaText = metas
        .filter(m => m.key !== '_reduced_stock')
        .map(m => {
          const key = m.display_key || m.key;
          const value = m.display_value || m.value;
          return `${key}: ${value}`;
        })
        .join(", ") || '無';

      const row = document.createElement('tr');
      row.innerHTML = `
        <td>${item.name || '無'}</td>
        <td>${metaText}</td>
        <td class="center">${item.quantity || 0}</td>
        <td class="right">${item.price || '0'}</td>
        <td class="center"></td>
        <td></td>
      `;
      tbody.appendChild(row);
    });
  }

  table.appendChild(tbody);
  return table;
}
