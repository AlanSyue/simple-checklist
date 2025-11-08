let allOrders = []; // Store filtered orders
let allOrdersUnfiltered = []; // Store all unfiltered orders for tag dropdown
let detailModal;
let selectedFilterTags = [];
let showCompletedFilter = false;
let showRemarkFilter = false;
let showCustomerNoteFilter = false;

document.addEventListener("DOMContentLoaded", function() {
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
});

async function loadOrders() {
  try {
    let url = "/api/orders";
    const params = new URLSearchParams();
    const hasFilters = selectedFilterTags.length > 0 || showRemarkFilter || showCustomerNoteFilter;

    if (selectedFilterTags.length > 0) {
      selectedFilterTags.forEach(tag => params.append('tags', tag));
    }
    if (showRemarkFilter) {
      params.append('has_remark', 'true');
    }
    if (showCustomerNoteFilter) {
      params.append('has_customer_note', 'true');
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

  if (!filteredOrders || filteredOrders.length === 0) {
    list.innerHTML = `<tr><td colspan="10" class="text-center text-muted py-4">沒有符合條件的訂單</td></tr>`;
    return;
  }

  filteredOrders.forEach(order => {
    const row = document.createElement("tr");
    const shippingMethod = order.shipping_lines && order.shipping_lines.length > 0 ? order.shipping_lines[0].method_title : 'N/A';

    row.innerHTML = `
      <td><a href="#" onclick="showOrderDetails(${order.id}); return false;">${order.id}</a></td>
      <td>${order.shipping.first_name}</td>
      <td>${order.payment_method_title || 'N/A'}</td>
      <td>${order.total}</td>
      <td>${shippingMethod}</td>
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