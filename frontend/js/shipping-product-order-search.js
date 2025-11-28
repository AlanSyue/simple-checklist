document.addEventListener('DOMContentLoaded', () => {
  // DOM Elements
  const allProductsContainer = document.getElementById('all-products');
  const selectedProductsContainer = document.getElementById('selected-products');
  const excludedProductsContainer = document.getElementById('excluded-products');
  const productFilterInput = document.getElementById('product-filter');
  const searchBtn = document.getElementById('search-btn');
  const searchModeSelect = document.getElementById('search-mode');
  const searchResultsContainer = document.getElementById('search-results');
  const orderDetailBody = document.getElementById('orderDetailBody');

  // State
  let allProducts = [];
  let selectedProducts = new Set();
  let excludedProducts = new Set();
  let draggedProduct = null;
  let allSearchResults = { woo_orders: [], sell_orders: [] };
  let detailModal;

  // --- Initialization ---
  async function initialize() {
    detailModal = new bootstrap.Modal(document.getElementById('orderDetailModal'));
    setupEventListeners();
    await loadAllProducts();
    renderSelectedProducts();
    renderExcludedProducts();
  }

  async function loadAllProducts() {
    try {
      const response = await fetch('/api/product-mappings');
      if (!response.ok) throw new Error('Failed to load products');
      const mappings = await response.json();

      const mappedNames = new Set(mappings.map(m => m.mapped_name));
      allProducts = Array.from(mappedNames).sort();

      renderAllProducts();
    } catch (error) {
      console.error('Error loading products:', error);
      allProductsContainer.innerHTML = '<div class="text-danger">無法載入商品列表</div>';
    }
  }

  function setupEventListeners() {
    productFilterInput.addEventListener('input', () => renderAllProducts(productFilterInput.value));
    searchBtn.addEventListener('click', handleSearch);

    // Add product by clicking
    allProductsContainer.addEventListener('click', handleProductClick);

    // Drag and Drop for Selected Products
    allProductsContainer.addEventListener('dragstart', handleDragStart);
    selectedProductsContainer.addEventListener('dragover', handleDragOver);
    selectedProductsContainer.addEventListener('dragleave', handleDragLeave);
    selectedProductsContainer.addEventListener('drop', handleDropSelected);

    // Drag and Drop for Excluded Products
    excludedProductsContainer.addEventListener('dragover', handleDragOver);
    excludedProductsContainer.addEventListener('dragleave', handleDragLeave);
    excludedProductsContainer.addEventListener('drop', handleDropExcluded);

    // Remove products
    selectedProductsContainer.addEventListener('click', handleRemoveProduct);
    excludedProductsContainer.addEventListener('click', handleRemoveProduct);
  }

  // --- Rendering ---
  function renderAllProducts(filter = '') {
    const lowerCaseFilter = filter.toLowerCase();
    const filtered = allProducts.filter(p => p.toLowerCase().includes(lowerCaseFilter));

    if (filtered.length === 0) {
      allProductsContainer.innerHTML = '<div class="text-muted p-2">沒有符合的商品</div>';
      return;
    }

    allProductsContainer.innerHTML = filtered.map(product => `
      <div class="p-2 border-bottom product-item" draggable="true" data-product-name="${product}">
        ${product}
      </div>
    `).join('');
  }

  function renderSelectedProducts() {
    if (selectedProducts.size === 0) {
      selectedProductsContainer.innerHTML = '<div class="text-muted p-3 text-center">點擊或拖曳商品至此處</div>';
      return;
    }
    selectedProductsContainer.innerHTML = Array.from(selectedProducts).map(product => `
      <span class="badge bg-primary fs-6 m-1 p-2 product-tag" data-product-name="${product}" data-list="selected">
        ${product}
        <i class="bi bi-x-circle ms-1"></i>
      </span>
    `).join('');
  }

  function renderExcludedProducts() {
    if (excludedProducts.size === 0) {
      excludedProductsContainer.innerHTML = '<div class="text-muted p-3 text-center">點擊或拖曳商品至此處以排除</div>';
      return;
    }
    excludedProductsContainer.innerHTML = Array.from(excludedProducts).map(product => `
      <span class="badge bg-danger fs-6 m-1 p-2 product-tag" data-product-name="${product}" data-list="excluded">
        ${product}
        <i class="bi bi-x-circle ms-1"></i>
      </span>
    `).join('');
  }

  // Helper function to format date as YYYY-MM-DD HH:mm:ss
  function formatDateTime(dateString) {
    const date = new Date(dateString);
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    const seconds = String(date.getSeconds()).padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
  }

  function renderResults(results) {
    allSearchResults = results;
    searchResultsContainer.innerHTML = '';
    const productSummaryContainer = document.getElementById('product-summary');
    const { woo_orders, sell_orders } = results;

    if (!woo_orders?.length && !sell_orders?.length) {
      searchResultsContainer.innerHTML = '<div class="alert alert-warning">找不到符合條件的訂單。</div>';
      productSummaryContainer.innerHTML = '';
      return;
    }

    // Build product summary
    const productMap = new Map();

    // Count products from WooCommerce orders
    (woo_orders || []).forEach(order => {
      (order.line_items || []).forEach(item => {
        const productName = item.name;
        const quantity = item.quantity || 0;
        if (productMap.has(productName)) {
          productMap.set(productName, productMap.get(productName) + quantity);
        } else {
          productMap.set(productName, quantity);
        }
      });
    });

    // Count products from Sell orders
    (sell_orders || []).forEach(order => {
      (order.items || []).forEach(item => {
        const productName = item.product_name;
        const quantity = item.qty || 0;
        if (productMap.has(productName)) {
          productMap.set(productName, productMap.get(productName) + quantity);
        } else {
          productMap.set(productName, quantity);
        }
      });
    });

    // Render product summary table
    if (productMap.size > 0) {
      const sortedProducts = Array.from(productMap.entries()).sort((a, b) => a[0].localeCompare(b[0]));
      let summaryHtml = `
        <div class="card">
          <div class="card-header bg-success text-white">
            <h5 class="mb-0"><i class="bi bi-box-seam me-2"></i>商品統計</h5>
          </div>
          <div class="card-body">
            <div class="table-responsive">
              <table class="table table-striped table-hover">
                <thead>
                  <tr>
                    <th>商品名稱</th>
                    <th class="text-end">數量</th>
                  </tr>
                </thead>
                <tbody>
      `;

      sortedProducts.forEach(([productName, quantity]) => {
        summaryHtml += `
          <tr>
            <td>${productName}</td>
            <td class="text-end"><strong>${quantity}</strong></td>
          </tr>
        `;
      });

      summaryHtml += `
                </tbody>
              </table>
            </div>
          </div>
        </div>
      `;
      productSummaryContainer.innerHTML = summaryHtml;
    } else {
      productSummaryContainer.innerHTML = '';
    }

    // Combine and normalize orders
    const combinedOrders = [];

    // Add WooCommerce orders
    (woo_orders || []).forEach(order => {
      const shippingMethod = order.shipping_lines && order.shipping_lines.length > 0 ? order.shipping_lines[0].method_title : 'N/A';
      combinedOrders.push({
        source: 'woo',
        sourceId: order.id,
        orderNo: order.id,
        name: order.shipping.first_name,
        shippingMethod: shippingMethod,
        cvsStoreName: order.cvs_store_name || '',
        amount: order.total,
        orderTime: new Date(order.date_created),
        orderTimeStr: order.date_created,
        note: order.customer_note || '',
        sourceLabel: '官網'
      });
    });

    // Add Sell orders
    (sell_orders || []).forEach(order => {
      combinedOrders.push({
        source: 'sell',
        sourceId: order.order_no,
        orderNo: order.order_no,
        name: order.receiver_name,
        shippingMethod: '7-ELEVEN',
        cvsStoreName: '',
        amount: order.total_amount,
        orderTime: new Date(order.ordered_at),
        orderTimeStr: order.ordered_at,
        note: order.note || '',
        sourceLabel: '賣貨便'
      });
    });

    // Sort by order time (ascending)
    combinedOrders.sort((a, b) => a.orderTime - b.orderTime);

    let html = `
      <div class="table-responsive">
        <table class="table table-striped table-hover">
          <thead>
            <tr>
              <th>訂單編號</th>
              <th>姓名</th>
              <th>出貨方式</th>
              <th>取貨門市</th>
              <th>金額</th>
              <th>訂購時間</th>
              <th>客戶備註</th>
              <th>訂單來源</th>
              <th>詳細</th>
            </tr>
          </thead>
          <tbody>
    `;

    combinedOrders.forEach(order => {
      const detailOnClick = order.source === 'woo'
        ? `showOrderDetail('woo', ${order.sourceId})`
        : `showOrderDetail('sell', '${order.sourceId}')`;

      html += `
        <tr>
          <td>${order.orderNo}</td>
          <td>${order.name}</td>
          <td>${order.shippingMethod}</td>
          <td>${order.cvsStoreName}</td>
          <td>${order.amount}</td>
          <td>${formatDateTime(order.orderTimeStr)}</td>
          <td>${order.note}</td>
          <td>${order.sourceLabel}</td>
          <td><button class="btn btn-sm btn-info" onclick="${detailOnClick}">詳細</button></td>
        </tr>
      `;
    });

    html += `</tbody></table></div>`;
    searchResultsContainer.innerHTML = html;
  }

  // --- Event Handlers ---
  function handleProductClick(e) {
    const productItem = e.target.closest('.product-item');
    if (productItem) {
      const productName = productItem.dataset.productName;
      const clickMode = document.querySelector('input[name="click-mode"]:checked').value;

      if (productName) {
        if (clickMode === 'add') {
          excludedProducts.delete(productName);
          selectedProducts.add(productName);
        } else {
          selectedProducts.delete(productName);
          excludedProducts.add(productName);
        }
        renderSelectedProducts();
        renderExcludedProducts();
      }
    }
  }

  function handleDragStart(e) {
    if (e.target.classList.contains('product-item')) {
      draggedProduct = e.target.dataset.productName;
      e.dataTransfer.setData('text/plain', draggedProduct);
      e.target.style.opacity = '0.5';
    }
  }

  function handleDragOver(e) {
    e.preventDefault();
    e.currentTarget.classList.add('bg-secondary-subtle');
  }

  function handleDragLeave(e) {
    e.currentTarget.classList.remove('bg-secondary-subtle');
  }

  function handleDropSelected(e) {
    e.preventDefault();
    e.currentTarget.classList.remove('bg-secondary-subtle');
    const productName = e.dataTransfer.getData('text/plain');
    if (productName) {
      excludedProducts.delete(productName);
      selectedProducts.add(productName);
      renderSelectedProducts();
      renderExcludedProducts();
    }
    resetDragState();
  }

  function handleDropExcluded(e) {
    e.preventDefault();
    e.currentTarget.classList.remove('bg-secondary-subtle');
    const productName = e.dataTransfer.getData('text/plain');
    if (productName) {
      selectedProducts.delete(productName);
      excludedProducts.add(productName);
      renderSelectedProducts();
      renderExcludedProducts();
    }
    resetDragState();
  }

  function resetDragState() {
    if (draggedProduct) {
      const originalElement = allProductsContainer.querySelector(`[data-product-name="${draggedProduct}"]`);
      if (originalElement) {
        originalElement.style.opacity = '1';
      }
    }
    draggedProduct = null;
  }

  function handleRemoveProduct(e) {
    const target = e.target.closest('.product-tag');
    if (target) {
      const productName = target.dataset.productName;
      const list = target.dataset.list;
      if (list === 'selected') {
        selectedProducts.delete(productName);
        renderSelectedProducts();
      } else if (list === 'excluded') {
        excludedProducts.delete(productName);
        renderExcludedProducts();
      }
    }
  }

  async function handleSearch() {
    const productNames = Array.from(selectedProducts);
    const excludedProductNames = Array.from(excludedProducts);

    if (productNames.length === 0 && searchModeSelect.value !== 'excludes') {
      alert('請至少選擇一個搜尋商品');
      return;
    }
    if (productNames.length === 0 && searchModeSelect.value === 'excludes' && excludedProductNames.length === 0) {
      alert('請至少選擇一個搜尋商品或排除商品');
      return;
    }

    const payload = {
      product_names: productNames,
      mode: searchModeSelect.value,
      excluded_product_names: excludedProductNames,
    };

    searchBtn.disabled = true;
    searchBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span> 搜尋中...';

    try {
      const response = await fetch('/api/shipping-orders/search-by-products', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || '搜尋失敗');
      }

      const results = await response.json();
      renderResults(results);
    } catch (error) {
      console.error('Search error:', error);
      searchResultsContainer.innerHTML = `<div class="alert alert-danger">${error.message}</div>`;
    } finally {
      searchBtn.disabled = false;
      searchBtn.innerHTML = '<i class="bi bi-search"></i> 搜尋';
    }
  }

  // --- Detail Modal Functionality ---
  window.showOrderDetail = function (source, id) {
    let order;
    if (source === 'woo') {
      order = allSearchResults.woo_orders.find(o => o.id === id);
    } else if (source === 'sell') {
      order = allSearchResults.sell_orders.find(o => o.order_no === id);
    }

    if (!order) {
      orderDetailBody.innerHTML = '<p class="text-danger">找不到訂單詳細資料。</p>';
      detailModal.show();
      return;
    }

    let detailHtml = '';
    if (source === 'woo') {
      const shippingMethod = order.shipping_lines && order.shipping_lines.length > 0 ? order.shipping_lines[0].method_title : 'N/A';
      const cvsStoreName = order.cvs_store_name || '';
      const productsHtml = (order.line_items || []).map(item => `<li>${item.name} (x${item.quantity}) - $${item.total}</li>`).join('');
      detailHtml = `
        <p><strong>訂單 ID:</strong> ${order.id}</p>
        <p><strong>訂購日期:</strong> ${new Date(order.date_created).toLocaleString()}</p>
        <p><strong>訂購人:</strong> ${order.shipping.first_name}</p>
        <p><strong>Email:</strong> ${order.billing.email}</p>
        <p><strong>電話:</strong> ${order.billing.phone}</p>
        <p><strong>總金額:</strong> ${order.total}</p>
        <p><strong>付款方式:</strong> ${order.payment_method_title}</p>
        <p><strong>出貨方式:</strong> ${shippingMethod}</p>
        ${cvsStoreName ? `<p><strong>取貨門市:</strong> ${cvsStoreName}</p>` : ''}
        <p><strong>客戶備註:</strong> ${order.customer_note || '無'}</p>
        <h6>商品列表:</h6>
        <ul>${productsHtml}</ul>
      `;
    } else if (source === 'sell') {
      const productsHtml = (order.items || []).map(item => `<li>${item.product_name} (x${item.qty}) - $${item.discount_price * item.qty}</li>`).join('');
      detailHtml = `
        <p><strong>訂單編號:</strong> ${order.order_no}</p>
        <p><strong>訂購日期:</strong> ${new Date(order.ordered_at).toLocaleString()}</p>
        <p><strong>收件人:</strong> ${order.receiver_name}</p>
        <p><strong>地址:</strong> ${order.address}</p>
        <p><strong>總數量:</strong> ${order.total_qty}</p>
        <p><strong>總金額:</strong> ${order.total_amount}</p>
        <p><strong>備註:</strong> ${order.note || '無'}</p>
        <h6>商品列表:</h6>
        <ul>${productsHtml}</ul>
      `;
    }
    orderDetailBody.innerHTML = detailHtml;
    detailModal.show();
  };

  // --- Start the app ---
  initialize();
});