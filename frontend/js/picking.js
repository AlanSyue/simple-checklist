async function loadPickingList() {
  try {
    const res = await fetch("/api/picking-list");
    const pickingList = await res.json();
    renderPickingList(pickingList);
  } catch (error) {
    console.error("載入Picking List失敗:", error);
    showAlert("載入揀貨表失敗", "danger");
  }
}

function renderPickingList(pickingList) {
  const list = document.getElementById("picking-list");
  list.innerHTML = "";
   if (!pickingList || pickingList.length === 0) {
    list.innerHTML = `<tr><td colspan="3" class="text-center text-muted py-4">沒有需要揀貨的商品</td></tr>`;
    return;
  }
  pickingList.forEach(item => {
    const row = document.createElement("tr");
    const orderIdsHtml = item.order_ids.map(id => `<a href="#" onclick="showOrderDetails(${id}); return false;">${id}</a>`).join(', ');
    row.innerHTML = `
      <td>${item.name}</td>
      <td>${item.quantity}</td>
      <td>${orderIdsHtml}</td>
    `;
    list.appendChild(row);
  });
}

loadPickingList();
