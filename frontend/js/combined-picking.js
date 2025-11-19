async function loadCombinedPickingList() {
  const loadingMessage = document.getElementById("loading-message");
  const pickingContent = document.getElementById("picking-content");

  try {
    const res = await fetch("/api/combined-picking-list");

    if (!res.ok) {
      const errorText = await res.text().catch(() => res.statusText);
      throw new Error(errorText || "無法載入合併揀貨表");
    }

    const pickingList = await res.json();

    loadingMessage.style.display = "none";
    pickingContent.style.display = "block";

    renderCombinedPickingList(pickingList);
  } catch (error) {
    console.error("載入合併揀貨表失敗:", error);
    loadingMessage.innerHTML = `
      <div class="alert alert-danger" role="alert">
        <i class="bi bi-exclamation-triangle me-2"></i>載入合併揀貨表失敗：${error.message}
      </div>
    `;
  }
}

function renderCombinedPickingList(pickingList) {
  const tbody = document.getElementById("picking-list");
  tbody.innerHTML = "";

  if (!pickingList || pickingList.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="3" class="text-center text-muted py-4">
          目前沒有需要揀貨的商品
        </td>
      </tr>
    `;
    return;
  }

  pickingList.forEach(item => {
    const row = document.createElement("tr");

    // Create source badge
    let sourceBadge = "";
    if (item.sources === "官網 + 賣貨便") {
      sourceBadge = `
        <span class="badge bg-primary me-1">官網 ${item.woocommerce_qty}</span>
        <span class="badge bg-success">賣貨便 ${item.sell_qty}</span>
      `;
    } else if (item.sources === "官網") {
      sourceBadge = `<span class="badge bg-primary">官網 ${item.woocommerce_qty}</span>`;
    } else {
      sourceBadge = `<span class="badge bg-success">賣貨便 ${item.sell_qty}</span>`;
    }

    row.innerHTML = `
      <td>${item.product_name}</td>
      <td class="fw-bold">${item.total_qty}</td>
      <td>${sourceBadge}</td>
    `;
    tbody.appendChild(row);
  });
}

// Load the combined picking list when page loads
document.addEventListener("DOMContentLoaded", () => {
  loadCombinedPickingList();
});
