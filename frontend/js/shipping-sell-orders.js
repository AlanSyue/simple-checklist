const aggregatedOrdersEndpoint = "/orders/uploaded-shipping/summary";
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
let startDateFilter = "";
let endDateFilter = "";
let selectedOrderIds = new Set(); // Store selected order IDs for export

document.addEventListener("DOMContentLoaded", () => {
  const form = document.getElementById(selector.uploadForm);
  if (form) {
    form.addEventListener("submit", handleUpload);
  }

  const clearBtn = document.getElementById("clear-orders-btn");
  if (clearBtn) {
    clearBtn.addEventListener("click", handleClearOrders);
  }

  // Initialize date filters
  const startDateInput = document.getElementById('start-date-filter');
  const endDateInput = document.getElementById('end-date-filter');

  if (startDateInput) {
    startDateInput.addEventListener('change', (event) => {
      startDateFilter = event.target.value;
      fetchAggregatedOrders();
    });
  }

  if (endDateInput) {
    endDateInput.addEventListener('change', (event) => {
      endDateFilter = event.target.value;
      fetchAggregatedOrders();
    });
  }

  const selectAllCheckbox = document.getElementById('select-all-checkbox');
  if (selectAllCheckbox) {
    selectAllCheckbox.addEventListener('change', toggleSelectAll);
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

  const files = fileInput.files;
  if (!files || files.length === 0) {
    showAlert("請先選擇 .xlsx 檔案", "warning");
    return;
  }

  for (const file of files) {
    if (!file.name.toLowerCase().endsWith(".xlsx")) {
      showAlert(`檔案 "${file.name}" 不是 .xlsx 格式`, "warning");
      return;
    }
  }

  uploadButton.disabled = true;
  uploadButton.innerHTML = `<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> 上傳中...`;

  try {
    let totalRows = 0;

    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      const formData = new FormData();
      formData.append("file", file);

      uploadButton.innerHTML = `<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> 上傳中 (${i + 1}/${files.length})...`;

      const response = await fetch("/orders/upload-shipping", {
        method: "POST",
        body: formData,
      });

      if (!response.ok) {
        const errorText = await response.text().catch(() => response.statusText);
        throw new Error(`檔案 "${file.name}" 上傳失敗：${errorText || "請檢查檔案格式"}`);
      }

      const payload = await response.json().catch(() => ({}));
      const rowCount = payload.rows ?? payload.count ?? 0;
      totalRows += rowCount;
    }

    updateUploadBadge(totalRows);
    showAlert(`成功上傳 ${files.length} 個檔案，共 ${totalRows} 筆資料`, "success");
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
    let url = aggregatedOrdersEndpoint;
    const params = new URLSearchParams();
    if (startDateFilter) {
      params.append('start_date', startDateFilter);
    }
    if (endDateFilter) {
      params.append('end_date', endDateFilter);
    }

    if (params.toString()) {
      url += '?' + params.toString();
    }

    const response = await fetch(url);
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
        <td colspan="9" class="text-center text-muted">目前沒有上傳資料</td>
      </tr>
    `;
    return;
  }

  // Sort orders by ordered_at ascending
  orders.sort((a, b) => {
    const dateA = new Date(a.ordered_at);
    const dateB = new Date(b.ordered_at);
    return dateA - dateB;
  });

  orders.forEach((order, index) => {
    // Extract note from first item that has a note
    const note = (order.items || []).find(item => item.note && item.note.trim() !== "")?.note || "";
    const isSelected = selectedOrderIds.has(order.order_no);

    const row = document.createElement("tr");
    row.innerHTML = `
      <td><input type="checkbox" class="form-check-input order-checkbox" data-order-no="${order.order_no}" ${isSelected ? 'checked' : ''} onchange="toggleOrderSelection('${order.order_no}')"></td>
      <td>${order.order_no || "-"}</td>
      <td>${formatDate(order.ordered_at)}</td>
      <td>${order.receiver_name || "-"}</td>
      <td>${order.address || "-"}</td>
      <td class="text-end">${formatNumber(order.total_qty, 0)}</td>
      <td class="text-end">${formatNumber(order.total_amount, 0)}</td>
      <td>${note}</td>
      <td class="text-center">
        <button class="btn btn-sm btn-outline-primary" type="button" onclick="showUploadedOrderDetail(${index})">
          <i class="bi bi-card-text me-1"></i>明細
        </button>
      </td>
    `;
    body.appendChild(row);
  });

  updateSelectedCount();
  updateSelectAllCheckbox();
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

async function handleClearOrders() {
  if (!confirm("確定要清空所有訂單資料嗎？此操作無法復原。")) {
    return;
  }

  const clearBtn = document.getElementById("clear-orders-btn");
  if (!clearBtn) {
    return;
  }

  const originalHTML = clearBtn.innerHTML;
  clearBtn.disabled = true;
  clearBtn.innerHTML = `<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> 清空中...`;

  try {
    const response = await fetch("/orders/uploaded-shipping", {
      method: "DELETE",
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => response.statusText);
      throw new Error(errorText || "清空失敗");
    }

    showAlert("已清空所有訂單資料", "success");
    fetchAggregatedOrders();
  } catch (error) {
    console.error(error);
    showAlert(error.message || "清空失敗，請稍後再試", "danger");
  } finally {
    clearBtn.disabled = false;
    clearBtn.innerHTML = originalHTML;
  }
}

// 切換訂單選取狀態
function toggleOrderSelection(orderNo) {
  if (selectedOrderIds.has(orderNo)) {
    selectedOrderIds.delete(orderNo);
  } else {
    selectedOrderIds.add(orderNo);
  }
  updateSelectedCount();
  updateSelectAllCheckbox();
}

// 更新選取數量顯示
function updateSelectedCount() {
  const countElement = document.getElementById('selected-count');
  if (countElement) {
    countElement.textContent = `已選擇 ${selectedOrderIds.size} 筆訂單`;
  }
}

// 更新全選checkbox狀態
function updateSelectAllCheckbox() {
  const selectAllCheckbox = document.getElementById('select-all-checkbox');
  if (!selectAllCheckbox) return;

  const orderCheckboxes = document.querySelectorAll('.order-checkbox');
  const allChecked = orderCheckboxes.length > 0 && Array.from(orderCheckboxes).every(cb => cb.checked);
  const someChecked = Array.from(orderCheckboxes).some(cb => cb.checked);

  selectAllCheckbox.checked = allChecked;
  selectAllCheckbox.indeterminate = someChecked && !allChecked;
}

// 全選/取消全選
function toggleSelectAll() {
  const selectAllCheckbox = document.getElementById('select-all-checkbox');
  const isChecked = selectAllCheckbox.checked;

  selectedOrderIds.clear();

  if (isChecked) {
    aggregatedOrdersCache.forEach(order => {
      selectedOrderIds.add(order.order_no);
    });
  }

  renderAggregatedOrders(aggregatedOrdersCache);
}

// 匯出揀貨單
async function exportPickingList() {
  if (selectedOrderIds.size === 0) {
    showAlert("請至少選擇一筆訂單", "warning");
    return;
  }

  const exportBtn = document.getElementById('export-picking-list-btn');
  const originalBtnHTML = exportBtn ? exportBtn.innerHTML : '';

  const printWindow = window.open('sell-picking-list-print.html', '_blank');

  if (!printWindow) {
    showAlert("無法打開新視窗，請檢查瀏覽器的彈出視窗設定", "warning");
    return;
  }

  try {
    if (exportBtn) {
      exportBtn.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>載入訂單資料...`;
      exportBtn.disabled = true;
    }

    const selectedOrders = aggregatedOrdersCache.filter(order => selectedOrderIds.has(order.order_no));

    if (selectedOrders.length === 0) {
      printWindow.close();
      throw new Error('無法載入訂單資料');
    }

    const loadPromise = new Promise((resolve) => {
      if (printWindow.document.readyState === 'complete') {
        resolve();
      } else {
        printWindow.addEventListener('load', resolve);
      }
    });

    await loadPromise;

    // 額外延遲確保事件監聽器已設置
    await new Promise(resolve => setTimeout(resolve, 100));

    selectedOrders.sort((a, b) => {
      const dateA = new Date(a.ordered_at);
      const dateB = new Date(b.ordered_at);
      return dateA - dateB;
    });

    printWindow.postMessage({
      type: 'sellPickingListOrders',
      orders: selectedOrders
    }, window.location.origin);

    showAlert(`已開啟揀貨單（${selectedOrders.length} 筆訂單）`, "success");
  } catch (error) {
    console.error("匯出揀貨單失敗:", error);
    if (printWindow && !printWindow.closed) {
      printWindow.close();
    }
    showAlert(`匯出揀貨單失敗：${error.message}`, "danger");
  } finally {
    if (exportBtn) {
      exportBtn.disabled = false;
      exportBtn.innerHTML = originalBtnHTML;
    }
  }
}

// 匯出訂單列表
async function exportOrderList() {
  if (selectedOrderIds.size === 0) {
    showAlert("請至少選擇一筆訂單", "warning");
    return;
  }

  const exportBtn = document.getElementById('export-order-list-btn');
  const originalBtnHTML = exportBtn ? exportBtn.innerHTML : '';

  const printWindow = window.open('sell-order-list-print.html', '_blank');

  if (!printWindow) {
    showAlert("無法打開新視窗，請檢查瀏覽器的彈出視窗設定", "warning");
    return;
  }

  try {
    if (exportBtn) {
      exportBtn.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>載入訂單資料...`;
      exportBtn.disabled = true;
    }

    const selectedOrders = aggregatedOrdersCache.filter(order => selectedOrderIds.has(order.order_no));

    if (selectedOrders.length === 0) {
      printWindow.close();
      throw new Error('無法載入訂單資料');
    }

    const loadPromise = new Promise((resolve) => {
      if (printWindow.document.readyState === 'complete') {
        resolve();
      } else {
        printWindow.addEventListener('load', resolve);
      }
    });

    await loadPromise;

    // 額外延遲確保事件監聽器已設置
    await new Promise(resolve => setTimeout(resolve, 100));

    selectedOrders.sort((a, b) => {
      const dateA = new Date(a.ordered_at);
      const dateB = new Date(b.ordered_at);
      return dateA - dateB;
    });

    printWindow.postMessage({
      type: 'sellOrderListData',
      orders: selectedOrders
    }, window.location.origin);

    showAlert(`已開啟訂單列表（${selectedOrders.length} 筆訂單）`, "success");
  } catch (error) {
    console.error("匯出訂單列表失敗:", error);
    if (printWindow && !printWindow.closed) {
      printWindow.close();
    }
    showAlert(`匯出訂單列表失敗：${error.message}`, "danger");
  } finally {
    if (exportBtn) {
      exportBtn.disabled = false;
      exportBtn.innerHTML = originalBtnHTML;
    }
  }
}
