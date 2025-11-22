document.addEventListener('DOMContentLoaded', () => {
  // DOM Elements
  const allProductsContainer = document.getElementById('all-products');
  const selectedProductsContainer = document.getElementById('selected-products');
  const productFilterInput = document.getElementById('product-filter');
  const searchBtn = document.getElementById('search-btn');
  const searchModeSelect = document.getElementById('search-mode');
  const searchResultsContainer = document.getElementById('search-results');
  const orderDetailBody = document.getElementById('orderDetailBody');

  // State
  let allProducts = [];
  let selectedProducts = new Set();
  let draggedProduct = null;
  let allSearchResults = { woo_orders: [], sell_orders: [] }; // Store fetched results
  let detailModal; // Bootstrap modal instance

  // --- Initialization ---
  async function initialize() {
    detailModal = new bootstrap.Modal(document.getElementById('orderDetailModal'));
    setupEventListeners();
    await loadAllProducts();
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

    // Drag and Drop
    allProductsContainer.addEventListener('dragstart', handleDragStart);
    selectedProductsContainer.addEventListener('dragover', handleDragOver);
    selectedProductsContainer.addEventListener('dragleave', handleDragLeave);
    selectedProductsContainer.addEventListener('drop', handleDrop);

    // Remove selected product
    selectedProductsContainer.addEventListener('click', handleRemoveProduct);
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
      selectedProductsContainer.innerHTML = '<div class="text-muted p-3 text-center">將商品拖曳到此處</div>';
      return;
    }

    selectedProductsContainer.innerHTML = Array.from(selectedProducts).map(product => `
      <span class="badge bg-primary fs-6 m-1 p-2 product-tag" data-product-name="${product}">
        ${product}
        <i class="bi bi-x-circle ms-1"></i>
      </span>
    `).join('');
  }

  function renderResults(results) {
    allSearchResults = results; // Store results globally
    searchResultsContainer.innerHTML = '';
    const { woo_orders, sell_orders } = results;

    if (!woo_orders?.length && !sell_orders?.length) {
      searchResultsContainer.innerHTML = '<div class="alert alert-warning">找不到符合條件的訂單。</div>';
      return;
    }

    let html = `
      <div class="table-responsive">
        <table class="table table-striped table-hover">
          <thead>
            <tr>
              <th>訂單編號</th>
              <th>姓名</th>
              <th>出貨方式</th>
              <th>金額</th>
              <th>訂購時間</th>
              <th>客戶備註</th>
              <th>訂單來源</th>
              <th>詳細</th>
            </tr>
          </thead>
          <tbody>
    `;

    // Render WooCommerce Orders
    (woo_orders || []).forEach(order => {
      const shippingMethod = order.shipping_lines && order.shipping_lines.length > 0 ? order.shipping_lines[0].method_title : 'N/A';
      const orderDate = new Date(order.date_created);
      html += `
        <tr>
          <td>${order.id}</td>
          <td>${order.shipping.first_name}</td>
          <td>${shippingMethod}</td>
          <td>${order.total}</td>
          <td>${orderDate.toLocaleDateString()} ${orderDate.toLocaleTimeString()}</td>
          <td>${order.customer_note || ''}</td>
          <td>官網</td>
          <td><button class="btn btn-sm btn-info" onclick="showOrderDetail('woo', ${order.id})">詳細</button></td>
        </tr>
      `;
    });

    // Render Sell Orders
    (sell_orders || []).forEach(order => {
      const orderDate = new Date(order.ordered_at);
      html += `
        <tr>
          <td>${order.order_no}</td>
          <td>${order.receiver_name}</td>
          <td>7-ELEVEN</td>
          <td>${order.total_amount}</td>
          <td>${orderDate.toLocaleDateString()} ${orderDate.toLocaleTimeString()}</td>
          <td>${order.note || ''}</td>
          <td>賣貨便</td>
          <td><button class="btn btn-sm btn-info" onclick="showOrderDetail('sell', '${order.order_no}')">詳細</button></td>
        </tr>
      `;
    });

    html += `
          </tbody>
        </table>
      </div>
    `;
    searchResultsContainer.innerHTML = html;
  }

  // --- Event Handlers ---
  function handleDragStart(e) {
    if (e.target.classList.contains('product-item')) {
      draggedProduct = e.target.dataset.productName;
      e.dataTransfer.setData('text/plain', draggedProduct);
      e.target.style.opacity = '0.5';
    }
  }

  function handleDragOver(e) {
    e.preventDefault();
    selectedProductsContainer.classList.add('bg-secondary-subtle');
  }
  
  function handleDragLeave(e) {
    selectedProductsContainer.classList.remove('bg-secondary-subtle');
  }

  function handleDrop(e) {
    e.preventDefault();
    selectedProductsContainer.classList.remove('bg-secondary-subtle');
    const productName = e.dataTransfer.getData('text/plain');
    
    if (productName) {
      selectedProducts.add(productName);
      renderSelectedProducts();
    }
    
    // Reset opacity on original item
    const originalElement = allProductsContainer.querySelector(`[data-product-name="${productName}"]`);
    if (originalElement) {
      originalElement.style.opacity = '1';
    }
    draggedProduct = null;
  }

  function handleRemoveProduct(e) {
    const target = e.target.closest('.product-tag');
    if (target) {
      const productName = target.dataset.productName;
      selectedProducts.delete(productName);
      renderSelectedProducts();
    }
  }

  async function handleSearch() {
    const productNames = Array.from(selectedProducts);
    if (productNames.length === 0) {
      alert('請至少選擇一個商品');
      return;
    }

    const payload = {
      product_names: productNames,
      mode: searchModeSelect.value,
    };

    searchBtn.disabled = true;
    searchBtn.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> 搜尋中...';

    try {
      const response = await fetch('/api/orders/search-by-products', {
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
  window.showOrderDetail = function(source, id) {
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
