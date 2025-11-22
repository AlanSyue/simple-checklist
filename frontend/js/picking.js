let startDateFilter = "";
let endDateFilter = "";

document.addEventListener("DOMContentLoaded", function () {
  // Initialize date filters
  const startDateInput = document.getElementById('start-date-filter');
  const endDateInput = document.getElementById('end-date-filter');

  if (startDateInput) {
    startDateInput.addEventListener('change', (event) => {
      startDateFilter = event.target.value;
      loadPickingList();
    });
  }

  if (endDateInput) {
    endDateInput.addEventListener('change', (event) => {
      endDateFilter = event.target.value;
      loadPickingList();
    });
  }

  loadPickingList();
});

async function loadPickingList() {
  try {
    let url = "/api/picking-list";
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

    const res = await fetch(url);
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


