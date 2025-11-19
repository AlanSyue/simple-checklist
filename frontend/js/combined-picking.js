// Store the full picking list for filtering
let fullPickingList = [];
let currentFilteredList = [];

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
    fullPickingList = pickingList; // Store for filtering
    currentFilteredList = pickingList; // Initialize with full list

    loadingMessage.style.display = "none";
    pickingContent.style.display = "block";

    renderCombinedPickingList(pickingList);
    setupFilterListener();
    setupCalculateButton();
  } catch (error) {
    console.error("載入合併揀貨表失敗:", error);
    loadingMessage.innerHTML = `
      <div class="alert alert-danger" role="alert">
        <i class="bi bi-exclamation-triangle me-2"></i>載入合併揀貨表失敗：${error.message}
      </div>
    `;
  }
}

function setupFilterListener() {
  const filterInput = document.getElementById("product-filter");
  if (!filterInput) return;

  filterInput.addEventListener("input", (e) => {
    const filterText = e.target.value.trim().toLowerCase();

    if (filterText === "") {
      // Show all items
      currentFilteredList = fullPickingList;
      renderCombinedPickingList(fullPickingList);
    } else {
      // Filter items
      const filtered = fullPickingList.filter(item =>
        item.product_name.toLowerCase().includes(filterText)
      );
      currentFilteredList = filtered;
      renderCombinedPickingList(filtered);
    }
  });
}

function setupCalculateButton() {
  const calculateBtn = document.getElementById("calculate-total-btn");
  if (!calculateBtn) return;

  calculateBtn.addEventListener("click", () => {
    calculateTotal();
  });
}

function calculateTotal() {
  const totalSummary = document.getElementById("total-summary");
  const totalProductsSpan = document.getElementById("total-products");
  const totalQuantitySpan = document.getElementById("total-quantity");

  // Calculate based on current filtered list
  const productCount = currentFilteredList.length;
  const totalQuantity = currentFilteredList.reduce((sum, item) => sum + item.total_qty, 0);

  // Update display
  totalProductsSpan.textContent = productCount;
  totalQuantitySpan.textContent = totalQuantity;

  // Show the summary
  totalSummary.classList.remove("d-none");

  // Optional: scroll to the summary
  totalSummary.scrollIntoView({ behavior: "smooth", block: "nearest" });
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
