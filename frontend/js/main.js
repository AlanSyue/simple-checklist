function showAlert(message, type) {
  const alertDiv = document.createElement("div");
  alertDiv.className = `alert alert-${type} alert-dismissible fade show position-fixed`;
  alertDiv.style.cssText = "top: 20px; right: 20px; z-index: 1050; min-width: 300px;";
  alertDiv.innerHTML = `${message}<button type="button" class="btn-close" data-bs-dismiss="alert"></button>`;
  document.body.appendChild(alertDiv);
  setTimeout(() => { if (alertDiv.parentNode) { alertDiv.remove(); } }, 3000);
}

function showOrderDetails(orderId) {
  // Fetch the order details directly from the API
  fetch(`/api/orders/${orderId}`)
    .then(response => {
      if (!response.ok) {
        throw new Error(`API request failed with status ${response.status}`);
      }
      return response.json();
    })
    .then(order => {
      const detailBody = document.getElementById('orderDetailBody');
      
      // 訂購人姓名
      const customerName = order.shipping.first_name || "未知姓名";

      // 商品明細
      let productsHtml = '';
      if (order.line_items && order.line_items.length > 0) {
        productsHtml = `
          <table class="table table-bordered table-sm">
            <thead>
              <tr>
                <th>商品名稱</th>
                <th>規格名稱</th>
                <th>數量</th>
                <th>價格</th>
              </tr>
            </thead>
            <tbody>
        `;
        order.line_items.forEach((lineItem) => {
          const metas = lineItem.meta_data || [];
          const metaText = metas
            .map((m) => {
              const key = m.display_key || m.key;
              const value = m.display_value || m.value;
              return `${key}: ${value}`;
            })
            .join("，");
          productsHtml += `
            <tr>
              <td>${lineItem.name}</td>
              <td>${metaText || '無'}</td>
              <td>${lineItem.quantity}</td>
              <td>${lineItem.price || 'N/A'}</td>
            </tr>
          `;
        });
        productsHtml += `
            </tbody>
          </table>
        `;
      } else {
        productsHtml = '<p>無商品明細</p>';
      }

      // 出貨資訊 (取貨單號)
      let shipmentInfo = "自行取貨";
      let paymentNo = "";
      let validationNo = "";
      const ecpayMeta = order.meta_data.find(m => m.key === "_ecpay_shipping_info");
      if (ecpayMeta && typeof ecpayMeta.value === "object") {
        const firstKey = Object.keys(ecpayMeta.value)[0];
        const data = ecpayMeta.value[firstKey];
        const method = order.shipping_lines?.[0]?.method_title || "自行取貨";
        paymentNo = data.PaymentNo || "";
        validationNo = data.ValidationNo || "";
        shipmentInfo = `📦 出貨資訊 ${method}`;
      }

      detailBody.innerHTML = `
        <p><strong>ID:</strong> ${order.id}</p>
        <p><strong>訂購人姓名:</strong> ${customerName}</p>
        <p><strong>Email:</strong> ${order.billing ? order.billing.email : 'N/A'}</p>
        <p><strong>聯絡電話:</strong> ${order.billing ? order.billing.phone : 'N/A'}</p>
        <p><strong>付款方式:</strong> ${order.payment_method_title || 'N/A'}</p>
        <p><strong>金額:</strong> ${order.total}</p>
        <p><strong>出貨方式:</strong> ${shipmentInfo}</p>
        ${paymentNo ? `<p><strong>取貨單號:</strong> ${paymentNo}${validationNo}</p>` : ''}
        <p><strong>客戶備註:</strong> ${order.customer_note || '無'}</p>
        <p><strong>備註:</strong> ${order.order_metadata.remark || '無'}</p>
        <p><strong>標籤:</strong> ${Array.isArray(order.order_metadata.tags) && order.order_metadata.tags.length > 0 ? order.order_metadata.tags.join(', ') : '無'}</p>
        <p><strong>是否完成:</strong> ${order.order_metadata.is_completed ? '是' : '否'}</p>
        <hr>
        <h5>商品明細</h5>
        ${productsHtml}
      `;

      const modalElement = document.getElementById('orderDetailModal');
      if (modalElement) {
        const modal = new bootstrap.Modal(modalElement);
        modal.show();
      } else {
        console.error("Order detail modal element not found.");
      }
    })
    .catch(error => {
      console.error("Error fetching order details:", error);
      showAlert("載入訂單詳細資料失敗", "danger");
    });
}
