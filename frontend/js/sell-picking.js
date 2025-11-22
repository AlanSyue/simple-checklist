const sellPickingEndpoint = "/orders/picking";
const aggregatedOrdersEndpoint = "/orders/uploaded/summary";
const lastUploadEndpoint = "/orders/uploaded/last";
const lastUploadElementId = "sell-picking-last-upload";

let aggregatedOrdersCache = [];
let relatedPickingItems = [];
let startDateFilter = "";
let endDateFilter = "";

document.addEventListener("DOMContentLoaded", () => {
  // Initialize date filters
  const startDateInput = document.getElementById('start-date-filter');
  const endDateInput = document.getElementById('end-date-filter');

  if (startDateInput) {
    startDateInput.addEventListener('change', (event) => {
      startDateFilter = event.target.value;
      fetchSellPicking();
    });
  }

  if (endDateInput) {
    endDateInput.addEventListener('change', (event) => {
      endDateFilter = event.target.value;
      fetchSellPicking();
    });
  }

  fetchSellPicking();
  fetchAggregatedOrderSummary();
});

async function fetchSellPicking() {
  const body = document.getElementById("sell-picking-body");
  if (!body) return;

  try {
    let url = sellPickingEndpoint;
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
      throw new Error(await response.text().catch(() => response.statusText));
    }
    const data = await response.json();
    renderSellPickingList(Array.isArray(data) ? data : []);
    refreshLastUploadTime(lastUploadElementId);
  } catch (err) {
    console.error(err);
    body.innerHTML = `
      <tr>
        <td colspan="3" class="text-center text-danger">無法取得揀貨資料</td>
      </tr>
    `;
    const el = document.getElementById(lastUploadElementId);
    if (el) {
      el.textContent = "最後上傳：無法取得";
    }
  }
}

function renderSellPickingList(items) {
  const body = document.getElementById("sell-picking-body");
  if (!body) return;

  if (!items || items.length === 0) {
    body.innerHTML = `
      <tr>
        <td colspan="3" class="text-center text-muted">尚未上傳任何訂單</td>
      </tr>
    `;
    return;
  }

  body.innerHTML = "";
  relatedPickingItems = items;
  items.forEach((item, index) => {
    const orderNos = item.order_nos || [];
    const row = document.createElement("tr");
    row.innerHTML = `
      <td>${item.product_name || "-"}</td>
      <td class="text-end">${formatNumber(item.total_qty, 0)}</td>
      <td>
        ${orderNos.length === 0 ? "<span class=\"text-muted\">無訂單</span>" : `
          <button
            class="btn btn-sm btn-outline-secondary"
            type="button"
            onclick="showRelatedOrders(${index})"
          >查看</button>
        `}
      </td>
    `;
    body.appendChild(row);
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

async function fetchAggregatedOrderSummary() {
  try {
    const response = await fetch(aggregatedOrdersEndpoint);
    if (!response.ok) {
      throw new Error(await response.text().catch(() => response.statusText));
    }
    const payload = await response.json();
    aggregatedOrdersCache = Array.isArray(payload) ? payload : [];
  } catch (error) {
    console.error("fetch aggregated summary failed", error);
    aggregatedOrdersCache = [];
  }
}

function showUploadedOrderDetailByNo(orderNo) {
  const order = aggregatedOrdersCache.find(o => o.order_no === orderNo);
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

function showRelatedOrders(index) {
  const item = relatedPickingItems[index];
  if (!item || !Array.isArray(item.order_nos) || item.order_nos.length === 0) {
    return;
  }

  const body = document.getElementById("relatedOrdersBody");
  if (!body) {
    return;
  }

  const buttons = item.order_nos
    .map(no => `<button type="button" class="btn btn-sm btn-outline-primary me-1 mb-1" onclick="openOrderDetailFromRelated('${no}')">${no}</button>`)
    .join("");

  body.innerHTML = `
    <p><strong>${item.product_name || "商品"}</strong> 的相關訂單</p>
    <div class="d-flex flex-wrap gap-1">${buttons}</div>
  `;

  const modalElement = document.getElementById("relatedOrdersModal");
  if (modalElement) {
    const modal = new bootstrap.Modal(modalElement);
    modal.show();
  }
}

function openOrderDetailFromRelated(orderNo) {
  const relatedModalElement = document.getElementById("relatedOrdersModal");
  const existingModal = relatedModalElement ? bootstrap.Modal.getInstance(relatedModalElement) : null;
  if (existingModal) {
    existingModal.hide();
  }
  showUploadedOrderDetailByNo(orderNo);
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
