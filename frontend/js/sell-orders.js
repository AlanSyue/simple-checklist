const aggregatedOrdersEndpoint = "/orders/uploaded/summary";
const lastUploadEndpoint = "/orders/uploaded/last";

const selector = {
  uploadForm: "order-upload-form",
  uploadInput: "order-upload-input",
  uploadButton: "order-upload-btn",
  uploadBadge: "last-upload-count",
  uploadMessage: "uploaded-result-message",
  tableBody: "uploaded-orders-body",
};

let aggregatedOrdersCache = [];
const lastUploadElementId = "sell-orders-last-upload";

document.addEventListener("DOMContentLoaded", () => {
  const form = document.getElementById(selector.uploadForm);
  if (form) {
    form.addEventListener("submit", handleUpload);
  }
  fetchAggregatedOrders();
  refreshLastUploadTime(lastUploadElementId);
});

async function handleUpload(event) {
  event.preventDefault();

  const fileInput = document.getElementById(selector.uploadInput);
  const uploadButton = document.getElementById(selector.uploadButton);
  if (!fileInput || !uploadButton) {
    return;
  }

  const file = fileInput.files?.[0];
  if (!file) {
    showAlert("請先選擇 .xlsx 檔案", "warning");
    return;
  }

  if (!file.name.toLowerCase().endsWith(".xlsx")) {
    showAlert("只接受 .xlsx 副檔名的檔案", "warning");
    return;
  }

  uploadButton.disabled = true;
  uploadButton.innerHTML = `<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> 上傳中...`;

  try {
    const formData = new FormData();
    formData.append("file", file);

    const response = await fetch("/orders/upload", {
      method: "POST",
      body: formData,
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => response.statusText);
      throw new Error(errorText || "上傳失敗，請檢查檔案格式");
    }

    const payload = await response.json().catch(() => ({}));
    const rowCount = payload.rows ?? payload.count ?? 0;
    updateUploadBadge(Number(rowCount));
    showAlert(`成功寫入 ${rowCount} 筆資料`, "success");
    fileInput.value = "";
    fetchAggregatedOrders();
  } catch (error) {
    console.error(error);
    showAlert(error.message || "上傳失敗，請稍後再試", "danger");
  } finally {
    uploadButton.disabled = false;
    uploadButton.innerHTML = `<i class="bi bi-upload me-1"></i>上傳報表`;
  }
}

async function fetchAggregatedOrders() {
  const body = document.getElementById(selector.tableBody);
  if (!body) {
    return;
  }

  try {
    const response = await fetch(aggregatedOrdersEndpoint);
    if (!response.ok) {
      const message = await response.text().catch(() => response.statusText);
      throw new Error(message || "無法取得已上傳資料");
    }

    const payload = await response.json();
    const orders = Array.isArray(payload)
      ? payload
      : payload.orders ?? [];

    aggregatedOrdersCache = orders;
    renderAggregatedOrders(orders);
    updateUploadBadge(orders.length);
    setResultMessage(orders.length > 0 ? `共 ${orders.length} 筆訂單` : "目前尚未上傳任何訂單", orders.length > 0 ? "info" : "warning");
    refreshLastUploadTime(lastUploadElementId);
  } catch (error) {
    console.error(error);
    aggregatedOrdersCache = [];
    renderAggregatedOrders([]);
    setResultMessage("無法取得上傳資料，請稍後再試", "warning");
  }
}

function renderAggregatedOrders(orders) {
  const body = document.getElementById(selector.tableBody);
  if (!body) {
    return;
  }

  body.innerHTML = "";

  if (!orders || orders.length === 0) {
    body.innerHTML = `
      <tr>
        <td colspan="8" class="text-center text-muted">目前沒有上傳資料</td>
      </tr>
    `;
    return;
  }

  orders.forEach((order, index) => {
    const row = document.createElement("tr");
    row.innerHTML = `
      <td>${order.order_no || "-"}</td>
      <td>${formatDate(order.ordered_at)}</td>
      <td>${order.receiver_name || "-"}</td>
      <td>${order.address || "-"}</td>
      <td class="text-end">${formatNumber(order.total_qty, 0)}</td>
      <td class="text-end">${formatNumber(order.total_amount, 0)}</td>
      <td class="text-center">
        <button class="btn btn-sm btn-outline-primary" type="button" onclick="showUploadedOrderDetail(${index})">
          <i class="bi bi-card-text me-1"></i>明細
        </button>
      </td>
    `;
    body.appendChild(row);
  });
}

function showUploadedOrderDetail(index) {
  const order = aggregatedOrdersCache[index];
  if (!order) {
    return;
  }

  const body = document.getElementById("orderSummaryBody");
  if (!body) {
    return;
  }

  const items = order.items || [];
  const itemRows = items.map(item => `
    <tr>
      <td>${item.product_name || "-"}</td>
      <td class="text-end">${formatNumber(item.unit_price, 0)}</td>
      <td class="text-end">${formatNumber(item.discount_price, 0)}</td>
      <td class="text-end">${formatNumber(item.qty, 0)}</td>
    </tr>
  `).join("");

  const primaryNote = items.find(item => item.note && item.note.trim() !== "")?.note || "無";

  body.innerHTML = `
    <p><strong>訂單編號：</strong>${order.order_no || "-"}</p>
    <p><strong>訂購日期：</strong>${formatDate(order.ordered_at)}</p>
    <p><strong>收件人：</strong>${order.receiver_name || "-"}</p>
    <p><strong>取件地址：</strong>${order.address || "-"}</p>
    <div class="table-responsive">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>商品名稱</th>
            <th class="text-end">單價</th>
            <th class="text-end">優惠價</th>
            <th class="text-end">數量</th>
          </tr>
        </thead>
        <tbody>
          ${itemRows || `<tr><td colspan="4" class="text-center text-muted">無明細</td></tr>`}
        </tbody>
      </table>
    </div>
    <p><strong>備註：</strong>${primaryNote}</p>
    <p class="text-end fw-bold mb-0">總金額：${formatNumber(order.total_amount, 0)}</p>
  `;

  const modalElement = document.getElementById("orderSummaryModal");
  if (modalElement) {
    const modal = new bootstrap.Modal(modalElement);
    modal.show();
  }
}

async function refreshLastUploadTime(elementId) {
  const el = document.getElementById(elementId);
  if (!el) {
    return;
  }

  try {
    const response = await fetch(lastUploadEndpoint);
    if (!response.ok) {
      throw new Error(await response.text().catch(() => response.statusText));
    }
    const payload = await response.json();
    const timestamp = payload.last_uploaded_at;
    if (!timestamp) {
      el.textContent = "最後上傳：尚未上傳";
      return;
    }
    const parsed = new Date(timestamp);
    if (Number.isNaN(parsed.getTime())) {
      el.textContent = "最後上傳：尚未上傳";
      return;
    }
    el.textContent = `最後上傳：${parsed.toLocaleString("zh-TW", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })}`;
  } catch (error) {
    console.error("refresh last upload time failed", error);
    el.textContent = "最後上傳：無法取得";
  }
}

function formatDate(value) {
  if (!value) {
    return "-";
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  return parsed.toLocaleString("zh-TW", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatNumber(value, fractionDigits = 2) {
  if (value === null || value === undefined || value === "") {
    return "-";
  }

  const number = Number(value);
  if (Number.isNaN(number)) {
    return value;
  }

  return number.toLocaleString("zh-TW", {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
}

function updateUploadBadge(count) {
  const badge = document.getElementById(selector.uploadBadge);
  if (!badge) {
    return;
  }

  if (count <= 0) {
    badge.textContent = "尚未上傳";
    badge.classList.remove("bg-success");
    badge.classList.add("bg-info", "text-dark");
    return;
  }

  badge.textContent = `${count} 筆`;
  badge.classList.remove("bg-info", "text-dark");
  badge.classList.add("bg-success");
}

function setResultMessage(text, type) {
  const container = document.getElementById(selector.uploadMessage);
  if (!container) {
    return;
  }

  if (!text) {
    container.innerHTML = "";
    return;
  }

  container.innerHTML = `
    <div class="alert alert-${type} py-2 mb-0" role="alert">
      ${text}
    </div>
  `;
}
