let allOrders = []; // Store filtered orders
let allOrdersUnfiltered = []; // Store all unfiltered orders for tag dropdown
let detailModal;
let selectedFilterTags = [];
let showCompletedFilter = false;
let showRemarkFilter = false;
let showCustomerNoteFilter = false;
let startDateFilter = "";
let endDateFilter = "";
let selectedOrderIds = new Set(); // Store selected order IDs for export

document.addEventListener("DOMContentLoaded", function () {
  detailModal = new bootstrap.Modal(document.getElementById('orderDetailModal'));
  loadOrders();

  // Initialize tag filter dropdown
  const tagFilterDropdown = document.getElementById('tag-filter-dropdown');
  if (tagFilterDropdown) { // Check if element exists
    tagFilterDropdown.addEventListener('change', (event) => {
      if (event.target.classList.contains('dropdown-item-checkbox')) {
        const tag = event.target.value;
        if (event.target.checked) {
          selectedFilterTags.push(tag);
        } else {
          selectedFilterTags = selectedFilterTags.filter(t => t !== tag);
        }
        updateTagFilterButtonText(); // Update button text
        loadOrders(); // Reload orders to apply tag filter
      }
    });
  }

  // Initialize completed filter
  const completedFilter = document.getElementById('completed-filter');
  if (completedFilter) {
    completedFilter.addEventListener('change', (event) => {
      showCompletedFilter = event.target.checked;
      renderOrders(); // Re-render client-side
    });
  }

  // Initialize has remark filter
  const hasRemarkFilter = document.getElementById('has-remark-filter');
  if (hasRemarkFilter) {
    hasRemarkFilter.addEventListener('change', (event) => {
      showRemarkFilter = event.target.checked;
      loadOrders(); // Reload orders to apply remark filter
    });
  }

  // Initialize has customer note filter
  const hasCustomerNoteFilter = document.getElementById('has-customer-note-filter');
  if (hasCustomerNoteFilter) {
    hasCustomerNoteFilter.addEventListener('change', (event) => {
      showCustomerNoteFilter = event.target.checked;
      loadOrders(); // Reload orders to apply customer note filter
    });
  }

  const selectAllCheckbox = document.getElementById('select-all-checkbox');
  if (selectAllCheckbox) {
    selectAllCheckbox.addEventListener('change', toggleSelectAll);
  }

  // Initialize date filters
  const startDateInput = document.getElementById('start-date-filter');
  const endDateInput = document.getElementById('end-date-filter');

  if (startDateInput) {
    startDateInput.addEventListener('change', (event) => {
      startDateFilter = event.target.value;
      loadOrders();
    });
  }

  if (endDateInput) {
    endDateInput.addEventListener('change', (event) => {
      endDateFilter = event.target.value;
      loadOrders();
    });
  }
});

async function loadOrders() {
  try {
    let url = "/api/orders";
    const params = new URLSearchParams();
    const hasFilters = selectedFilterTags.length > 0 || showRemarkFilter || showCustomerNoteFilter || startDateFilter || endDateFilter;

    if (selectedFilterTags.length > 0) {
      selectedFilterTags.forEach(tag => params.append('tags', tag));
    }
    if (showRemarkFilter) {
      params.append('has_remark', 'true');
    }
    if (showCustomerNoteFilter) {
      params.append('has_customer_note', 'true');
    }
    if (startDateFilter) {
      params.append('start_date', startDateFilter);
    }
    if (endDateFilter) {
      params.append('end_date', endDateFilter);
    }

    if (params.toString()) {
      url += '?' + params.toString();
    }

    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`API request failed with status ${res.status}`);
    }
    allOrders = await res.json();
    if (!Array.isArray(allOrders)) {
      allOrders = [];
    }

    // Save unfiltered orders on first load or when no filters are active
    if (!hasFilters) {
      allOrdersUnfiltered = allOrders;
    }

    populateTagFilter();
    renderOrders();
  } catch (error) {
    console.error("載入Orders失敗:", error);
    showAlert("載入訂單列表失敗", "danger");
    allOrders = [];
    renderOrders();
  }
}

function updateTagFilterButtonText() {
  const button = document.getElementById('tagFilterDropdownButton');
  if (!button) return;

  if (selectedFilterTags.length === 0) {
    button.textContent = '篩選標籤';
  } else if (selectedFilterTags.length === 1) {
    button.textContent = `標籤: ${selectedFilterTags[0]}`;
  } else {
    button.textContent = `標籤 (${selectedFilterTags.length})`;
  }
}

function populateTagFilter() {
  const tagFilterDropdown = document.getElementById('tag-filter-dropdown');
  if (!tagFilterDropdown) return; // Exit if element doesn't exist

  tagFilterDropdown.innerHTML = ''; // Clear existing options

  const allUniqueTags = new Set();
  // Use unfiltered orders to get all available tags
  const ordersForTags = allOrdersUnfiltered.length > 0 ? allOrdersUnfiltered : allOrders;
  if (Array.isArray(ordersForTags)) {
    ordersForTags.forEach(order => {
      if (order.order_metadata.tags) {
        order.order_metadata.tags.forEach(tag => allUniqueTags.add(tag));
      }
    });
  }

  const sortedTags = Array.from(allUniqueTags).sort();

  // Add "Clear All" button if there are selected tags
  if (selectedFilterTags.length > 0) {
    const clearLi = document.createElement('li');
    clearLi.innerHTML = `
      <button class="dropdown-item text-danger" type="button" id="clear-tag-filters">
        <i class="bi bi-x-circle me-1"></i>清除所有篩選
      </button>
    `;
    clearLi.addEventListener('click', (e) => {
      e.stopPropagation();
      selectedFilterTags = [];
      updateTagFilterButtonText();
      loadOrders();
    });
    tagFilterDropdown.appendChild(clearLi);

    // Add divider
    const divider = document.createElement('li');
    divider.innerHTML = '<hr class="dropdown-divider">';
    tagFilterDropdown.appendChild(divider);
  }

  sortedTags.forEach(tag => {
    const li = document.createElement('li');
    li.innerHTML = `
      <div class="form-check dropdown-item">
        <input class="form-check-input dropdown-item-checkbox" type="checkbox" value="${tag}" id="tag-filter-${tag}" ${selectedFilterTags.includes(tag) ? 'checked' : ''}>
        <label class="form-check-label" for="tag-filter-${tag}">
          ${tag}
        </label>
      </div>
    `;
    // Prevent dropdown from closing when clicking on the item
    li.addEventListener('click', (e) => {
      e.stopPropagation();
    });
    tagFilterDropdown.appendChild(li);
  });

  // Update the dropdown button text
  updateTagFilterButtonText();
}

function renderOrders() {
  const list = document.getElementById("orders-list");
  list.innerHTML = "";

  if (!Array.isArray(allOrders)) {
    allOrders = [];
  }

  const filteredOrders = allOrders.filter(order => {
    // Filter by completed status (this is now client-side as backend filters by tags/remarks)
    const completedMatch = !showCompletedFilter || order.order_metadata.is_completed;
    return completedMatch;
  });

  // Sort by date_created ascending
  filteredOrders.sort((a, b) => {
    const dateA = new Date(a.date_created);
    const dateB = new Date(b.date_created);
    return dateA - dateB;
  });

  if (!filteredOrders || filteredOrders.length === 0) {
    list.innerHTML = `<tr><td colspan="13" class="text-center text-muted py-4">沒有符合條件的訂單</td></tr>`;
    return;
  }

  filteredOrders.forEach(order => {
    const row = document.createElement("tr");
    const shippingMethod = order.shipping_lines && order.shipping_lines.length > 0 ? order.shipping_lines[0].method_title : 'N/A';
    const cvsStoreName = order.cvs_store_name || '';
    const isSelected = selectedOrderIds.has(order.id);

    // Format date_created as YYYY-mm-dd HH:ii:ss
    const dateCreated = order.date_created ? new Date(order.date_created).toLocaleString('zh-TW', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false
    }).replace(/\//g, '-') : 'N/A';

    row.innerHTML = `
      <td><input type="checkbox" class="form-check-input order-checkbox" data-order-id="${order.id}" ${isSelected ? 'checked' : ''} onchange="toggleOrderSelection(${order.id})"></td>
      <td><a href="#" onclick="showOrderDetails(${order.id}); return false;">${order.id}</a></td>
      <td>${dateCreated}</td>
      <td>${order.shipping.first_name}</td>
      <td>${order.payment_method_title || 'N/A'}</td>
      <td>${order.total}</td>
      <td>${shippingMethod}</td>
      <td>${cvsStoreName}</td>
      <td>${order.customer_note || ''}</td>
      <td><input type="text" class="form-control form-control-sm remark-input" value="${order.order_metadata.remark || ''}" onchange="updateOrderMetadata(${order.id})"></td>
      <td class="tag-cell" id="tags-${order.id}"></td>
      <td><input type="checkbox" class="form-check-input is-completed-checkbox" ${order.order_metadata.is_completed ? 'checked' : ''} onchange="updateOrderMetadata(${order.id})"></td>
      <td><button class="btn btn-sm btn-info" onclick="showOrderDetails(${order.id})">詳細</button></td>
    `;
    list.appendChild(row);

    // Render tags using the new component
    renderTagInput(order.id, order.order_metadata.tags || []);
  });

  updateSelectedCount();
  updateSelectAllCheckbox();
}

function renderTagInput(orderId, initialTags) {
  const tagCell = document.getElementById(`tags-${orderId}`);
  if (!tagCell) return;

  tagCell.innerHTML = ''; // Clear existing content

  const tagContainer = document.createElement('div');
  tagContainer.className = 'tag-input-container';
  tagContainer.dataset.orderId = orderId;

  initialTags.forEach(tag => {
    const tagElement = createTagElement(orderId, tag);
    tagContainer.appendChild(tagElement);
  });

  const input = document.createElement('input');
  input.type = 'text';
  input.className = 'form-control form-control-sm tag-input-field';
  input.placeholder = '新增標籤...';
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && input.value.trim() !== '') {
      addTag(orderId, input.value.trim());
      input.value = '';
      e.preventDefault(); // Prevent form submission
    }
  });
  tagContainer.appendChild(input);
  tagCell.appendChild(tagContainer);
}

function createTagElement(orderId, tagText) {
  const span = document.createElement('span');
  span.className = 'badge bg-primary me-1 mb-1';
  span.textContent = tagText;

  const closeButton = document.createElement('button');
  closeButton.type = 'button';
  closeButton.className = 'btn-close btn-close-white ms-1';
  closeButton.setAttribute('aria-label', 'Remove tag');
  closeButton.addEventListener('click', () => removeTag(orderId, tagText));
  span.appendChild(closeButton);

  return span;
}

function addTag(orderId, tagText) {
  const order = allOrders.find(o => o.id === orderId);
  const unfilteredOrder = allOrdersUnfiltered.find(o => o.id === orderId);

  if (order) {
    // Ensure tags is an array before using includes
    if (!Array.isArray(order.order_metadata.tags)) {
      order.order_metadata.tags = [];
    }
    if (!order.order_metadata.tags.includes(tagText)) {
      order.order_metadata.tags.push(tagText);

      // Also update unfiltered list if the order exists there
      if (unfilteredOrder) {
        if (!Array.isArray(unfilteredOrder.order_metadata.tags)) {
          unfilteredOrder.order_metadata.tags = [];
        }
        if (!unfilteredOrder.order_metadata.tags.includes(tagText)) {
          unfilteredOrder.order_metadata.tags.push(tagText);
        }
      }

      updateOrderMetadata(orderId); // Save to backend
      renderTagInput(orderId, order.order_metadata.tags); // Re-render UI
    }
    populateTagFilter(); // Update filter options
  }
}

function removeTag(orderId, tagText) {
  const order = allOrders.find(o => o.id === orderId);
  const unfilteredOrder = allOrdersUnfiltered.find(o => o.id === orderId);

  if (order) {
    order.order_metadata.tags = order.order_metadata.tags.filter(tag => tag !== tagText);

    // Also update unfiltered list if the order exists there
    if (unfilteredOrder) {
      unfilteredOrder.order_metadata.tags = unfilteredOrder.order_metadata.tags.filter(tag => tag !== tagText);
    }

    updateOrderMetadata(orderId); // Save to backend
    renderTagInput(orderId, order.order_metadata.tags); // Re-render UI
    populateTagFilter(); // Update filter options
  }
}

async function updateOrderMetadata(orderId) {
  const row = document.getElementById(`tags-${orderId}`).closest('tr');
  const remark = row.querySelector('.remark-input').value;
  const order = allOrders.find(o => o.id === orderId);
  const unfilteredOrder = allOrdersUnfiltered.find(o => o.id === orderId);
  const tags = order.order_metadata.tags; // Get tags from local data
  const isCompleted = row.querySelector('.is-completed-checkbox').checked;

  const payload = {
    remark: remark,
    tags: tags,
    is_completed: isCompleted,
  };

  // Update local data immediately
  order.order_metadata.remark = remark;
  order.order_metadata.is_completed = isCompleted;

  // Also update unfiltered list if the order exists there
  if (unfilteredOrder) {
    unfilteredOrder.order_metadata.remark = remark;
    unfilteredOrder.order_metadata.is_completed = isCompleted;
    unfilteredOrder.order_metadata.tags = tags;
  }

  try {
    const res = await fetch(`/api/orders/${orderId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      throw new Error(`API request failed with status ${res.status}`);
    }
    showAlert("訂單資料已更新", "success");
  } catch (error) {
    console.error("更新失敗:", error);
    showAlert("更新訂單資料失敗", "danger");
  }
}

// 切換訂單選取狀態
function toggleOrderSelection(orderId) {
  if (selectedOrderIds.has(orderId)) {
    selectedOrderIds.delete(orderId);
  } else {
    selectedOrderIds.add(orderId);
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
    allOrders.forEach(order => {
      if (!showCompletedFilter || order.order_metadata.is_completed) {
        selectedOrderIds.add(order.id);
      }
    });
  }

  renderOrders();
}

// 分批獲取訂單詳細資料（使用批次 API）
async function fetchOrdersInBatches(orderIds, batchSize = 50) {
  const results = [];
  const totalBatches = Math.ceil(orderIds.length / batchSize);

  for (let i = 0; i < orderIds.length; i += batchSize) {
    const batch = orderIds.slice(i, i + batchSize);
    const currentBatch = Math.floor(i / batchSize) + 1;

    // 更新進度提示
    const exportBtn = document.getElementById('export-picking-list-btn');
    if (exportBtn) {
      exportBtn.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>載入中 (${currentBatch}/${totalBatches})...`;
      exportBtn.disabled = true;
    }

    // 使用批次 API 一次獲取多筆訂單
    try {
      const res = await fetch('/api/orders/batch', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ order_ids: batch })
      });

      if (!res.ok) {
        throw new Error(`Batch fetch failed with status ${res.status}`);
      }

      const batchOrders = await res.json();
      results.push(...batchOrders);
    } catch (error) {
      console.error(`Error fetching batch ${currentBatch}:`, error);
      // 如果批次 API 失敗，嘗試逐一獲取
      console.log('Falling back to individual fetch for this batch');
      for (const orderId of batch) {
        try {
          const res = await fetch(`/api/orders/${orderId}`);
          if (res.ok) {
            const order = await res.json();
            results.push(order);
          } else {
            // 從 allOrders 中找到基本資料作為後備
            const fallbackOrder = allOrders.find(order => order.id === orderId);
            if (fallbackOrder) {
              results.push(fallbackOrder);
            }
          }
        } catch (err) {
          console.error(`Error fetching order ${orderId}:`, err);
        }
      }
    }

    // 避免過快請求，稍微延遲（批次之間）
    if (i + batchSize < orderIds.length) {
      await new Promise(resolve => setTimeout(resolve, 200));
    }
  }

  return results;
}

// 匯出揀貨單
async function exportPickingList() {
  if (selectedOrderIds.size === 0) {
    showAlert("請至少選擇一筆訂單", "warning");
    return;
  }

  const exportBtn = document.getElementById('export-picking-list-btn');
  const originalBtnHTML = exportBtn ? exportBtn.innerHTML : '';

  // 先打開視窗（在用戶操作的同步上下文中）
  const printWindow = window.open('picking-list-print.html', '_blank');

  if (!printWindow) {
    showAlert("無法打開新視窗，請檢查瀏覽器的彈出視窗設定", "warning");
    return;
  }

  try {
    if (exportBtn) {
      exportBtn.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>載入訂單資料...`;
      exportBtn.disabled = true;
    }

    // Get selected order IDs
    const orderIds = Array.from(selectedOrderIds);

    // 建立一個監聽器，等待新視窗載入完成
    const loadPromise = new Promise((resolve) => {
      if (printWindow.document.readyState === 'complete') {
        resolve();
      } else {
        printWindow.addEventListener('load', resolve);
      }
    });

    // 同時開始獲取訂單資料
    const ordersPromise = fetchOrdersInBatches(orderIds, 50);

    // 等待兩者都完成
    const [, ordersWithDetails] = await Promise.all([loadPromise, ordersPromise]);

    if (ordersWithDetails.length === 0) {
      printWindow.close();
      throw new Error('無法載入訂單資料');
    }

    // 通過 postMessage 將資料傳遞給新視窗
    printWindow.postMessage({
      type: 'pickingListOrders',
      orders: ordersWithDetails
    }, window.location.origin);

    showAlert(`已開啟揀貨單預覽視窗（${ordersWithDetails.length} 筆訂單）`, "success");
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

// 產生揀貨單 PDF
function generatePickingListPDF(orders) {
  const { jsPDF } = window.jspdf;
  const doc = new jsPDF({
    orientation: 'portrait',
    unit: 'mm',
    format: 'a4'
  });

  // Set Chinese font
  doc.setFont('NotoSansTC-normal', 'normal');

  const exportDate = new Date().toLocaleString('zh-TW', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });

  let yPosition = 15;
  const pageHeight = doc.internal.pageSize.height;

  orders.forEach((order, index) => {
    // Check if we need a new page
    if (yPosition > pageHeight - 100 && index > 0) {
      doc.addPage();
      yPosition = 15;
    }

    // If this is not the first order, add separator
    if (index > 0) {
      doc.setLineWidth(0.5);
      doc.line(10, yPosition, 200, yPosition);
      yPosition += 10;
    }

    // Export date (only on first order)
    if (index === 0) {
      doc.setFontSize(10);
      doc.text(`匯出日期：${exportDate}`, 10, yPosition);
      yPosition += 10;
    }

    // Order details
    doc.setFontSize(12);
    doc.text(`訂單 ID: ${order.id}`, 10, yPosition);
    yPosition += 7;

    doc.setFontSize(10);

    const customerName = order.shipping?.first_name || '未知';
    const email = order.billing?.email || '無';
    const phone = order.billing?.phone || '無';
    const total = order.total || '0';

    // Get shipping info
    let shippingMethod = '自行取貨';
    let pickupNumber = '';

    if (order.shipping_lines && order.shipping_lines.length > 0) {
      shippingMethod = order.shipping_lines[0].method_title || '自行取貨';
    }

    const ecpayMeta = order.meta_data?.find(m => m.key === "_ecpay_shipping_info");
    if (ecpayMeta && typeof ecpayMeta.value === "object") {
      const firstKey = Object.keys(ecpayMeta.value)[0];
      const data = ecpayMeta.value[firstKey];
      const paymentNo = data.PaymentNo || "";
      const validationNo = data.ValidationNo || "";
      pickupNumber = paymentNo + validationNo;
    }

    doc.text(`訂購人姓名：${customerName}`, 10, yPosition);
    yPosition += 6;
    doc.text(`訂購人Email：${email}`, 10, yPosition);
    yPosition += 6;
    doc.text(`訂購人電話：${phone}`, 10, yPosition);
    yPosition += 6;
    doc.text(`訂單金額：${total}`, 10, yPosition);
    yPosition += 6;
    doc.text(`出貨方式：${shippingMethod}`, 10, yPosition);
    yPosition += 6;

    if (pickupNumber) {
      doc.text(`取貨單號：${pickupNumber}`, 10, yPosition);
      yPosition += 6;
    }

    yPosition += 3;

    // Product details table
    const tableData = [];
    if (order.line_items && order.line_items.length > 0) {
      order.line_items.forEach(item => {
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

        tableData.push([
          item.name || '無',
          metaText,
          item.quantity || 0,
          item.price || '0',
          '', // 揀貨
          ''  // 備註
        ]);
      });
    }

    doc.autoTable({
      startY: yPosition,
      head: [['商品名稱', '規格名稱', '數量', '價格', '揀貨', '備註']],
      body: tableData,
      theme: 'grid',
      styles: {
        fontSize: 9,
        cellPadding: 3,
        font: 'NotoSansTC-normal',
        lineColor: [0, 0, 0],
        lineWidth: 0.1
      },
      headStyles: {
        fillColor: [66, 139, 202],
        textColor: 255,
        font: 'NotoSansTC-normal',
        halign: 'center'
      },
      columnStyles: {
        0: { cellWidth: 50 },
        1: { cellWidth: 50 },
        2: { cellWidth: 15, halign: 'center' },
        3: { cellWidth: 20, halign: 'right' },
        4: { cellWidth: 15, halign: 'center' },
        5: { cellWidth: 40 }
      },
      margin: { left: 10, right: 10 }
    });

    yPosition = doc.lastAutoTable.finalY + 10;
  });

  // Save PDF
  const filename = `揀貨單_${new Date().getTime()}.pdf`;
  doc.save(filename);
}

// 匯出訂單列表
async function exportOrderList() {
  if (selectedOrderIds.size === 0) {
    showAlert("請至少選擇一筆訂單", "warning");
    return;
  }

  const exportBtn = document.getElementById('export-order-list-btn');
  const originalBtnHTML = exportBtn ? exportBtn.innerHTML : '';

  // 先打開視窗（在用戶操作的同步上下文中）
  const printWindow = window.open('order-list-print.html', '_blank');

  if (!printWindow) {
    showAlert("無法打開新視窗，請檢查瀏覽器的彈出視窗設定", "warning");
    return;
  }

  try {
    if (exportBtn) {
      exportBtn.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>載入訂單資料...`;
      exportBtn.disabled = true;
    }

    // Get selected order IDs
    const orderIds = Array.from(selectedOrderIds);

    // 建立一個監聽器，等待新視窗載入完成
    const loadPromise = new Promise((resolve) => {
      if (printWindow.document.readyState === 'complete') {
        resolve();
      } else {
        printWindow.addEventListener('load', resolve);
      }
    });

    // 同時開始獲取訂單資料
    const ordersPromise = fetchOrdersInBatches(orderIds, 50);

    // 等待兩者都完成
    const [, ordersWithDetails] = await Promise.all([loadPromise, ordersPromise]);

    if (ordersWithDetails.length === 0) {
      printWindow.close();
      throw new Error('無法載入訂單資料');
    }

    // Sort orders by date_created ascending
    ordersWithDetails.sort((a, b) => {
      const dateA = new Date(a.date_created);
      const dateB = new Date(b.date_created);
      return dateA - dateB;
    });

    // 通過 postMessage 將資料傳遞給新視窗
    printWindow.postMessage({
      type: 'orderListData',
      orders: ordersWithDetails
    }, window.location.origin);

    showAlert(`已開啟訂單列表（${ordersWithDetails.length} 筆訂單）`, "success");
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
