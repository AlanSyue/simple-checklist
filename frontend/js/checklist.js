let items = [];
let currentEditingItemId = null;
let reminderModal = null;

async function loadChecklist() {
  try {
    const res = await fetch("/api/checklist");
    items = await res.json();
    renderChecklist();
  } catch (error) {
    console.error("載入Checklist失敗:", error);
    showAlert("載入Checklist失敗", "danger");
  }
}

function renderChecklist() {
  const list = document.getElementById("list");
  list.innerHTML = "";
  if (!items || items.length === 0) {
    list.innerHTML = `<div class="text-center text-muted py-4"><i class="bi bi-check-circle display-4"></i><p class="mt-2">太棒了！所有項目都完成了！</p></div>`;
  } else {
    items.forEach((item, i) => {
      const itemElement = document.createElement("div");
      itemElement.className = "list-group-item d-flex align-items-center";

      // 檢查是否過期
      const isExpired = item.reminderDate && new Date(item.reminderDate) < new Date();

      // 格式化提醒日期
      let reminderDisplay = "";
      if (item.reminderDate) {
        const reminderDate = new Date(item.reminderDate);
        const formattedDate = reminderDate.toLocaleString('zh-TW', {
          year: 'numeric',
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit'
        });
        const dateClass = isExpired ? 'text-danger' : 'text-muted';
        reminderDisplay = `<div><small class="${dateClass}"><i class="bi bi-clock me-1"></i>${formattedDate}</small></div>`;
      }

      // 設定文字樣式
      let textClass = item.checked ? 'completed-item' : '';
      if (isExpired && !item.checked) {
        textClass = 'text-danger text-decoration-line-through';
      }

      itemElement.innerHTML = `
        <div class="form-check me-3"><input class="form-check-input" type="checkbox" ${item.checked ? "checked" : ""} onchange="toggle(${i})"></div>
        <div class="flex-grow-1" id="item-text-${item.id}">
          <div class="${textClass}">${item.text}</div>
          ${reminderDisplay}
        </div>
        <button class="btn btn-sm btn-outline-primary ms-2" onclick="openReminderModal(${item.id})" title="設定提醒">
          <i class="bi bi-bell"></i>
        </button>
        <button class="btn btn-sm btn-outline-secondary ms-2" onclick="editItem(${i})"><i class="bi bi-pencil"></i></button>
        <button class="btn btn-sm btn-outline-danger ms-2" onclick="deleteItem(${i})"><i class="bi bi-trash"></i></button>`;
      list.appendChild(itemElement);
    });
  }
}

function addItem() {
  const input = document.getElementById("item");
  if (input.value.trim()) {
    const newItem = {
      text: input.value,
      checked: false
    };

    fetch("/api/checklist", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items: [newItem] })
    }).then(() => {
      input.value = "";
      loadChecklist();
      showAlert("項目新增成功！", "success");
    });
  }
}

function toggle(i) {
  const item = items[i];
  fetch(`/api/checklist/${item.id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ checked: !item.checked })
  }).then(() => {
    loadChecklist();
    showAlert("項目狀態已更新！", "success");
  });
}

function openReminderModal(itemId) {
  currentEditingItemId = itemId;
  const item = items.find(i => i.id === itemId);

  // 初始化模態框
  if (!reminderModal) {
    reminderModal = new bootstrap.Modal(document.getElementById('reminderModal'));
  }

  // 設定當前提醒日期（轉換為本地時間）
  const modalInput = document.getElementById('modalReminderDate');
  if (item.reminderDate) {
    const date = new Date(item.reminderDate);
    // 轉換為本地時間格式（不是 UTC）
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    modalInput.value = `${year}-${month}-${day}T${hours}:${minutes}`;
  } else {
    modalInput.value = "";
  }

  // 顯示或隱藏清除按鈕
  const clearBtn = document.getElementById('clearReminderBtn');
  if (item.reminderDate) {
    clearBtn.style.display = 'inline-block';
  } else {
    clearBtn.style.display = 'none';
  }

  reminderModal.show();
}

function saveReminder() {
  const modalInput = document.getElementById('modalReminderDate');
  const selectedDate = modalInput.value;

  if (!selectedDate) {
    showAlert("請選擇提醒日期時間", "warning");
    return;
  }

  // 將使用者輸入的本地時間（datetime-local）解析為正確的 Date 物件
  // datetime-local 的值格式為 "YYYY-MM-DDTHH:mm"
  const reminderDate = new Date(selectedDate);
  const now = new Date();

  // 檢查是否為過期日期
  if (reminderDate < now) {
    showAlert("無法設定過期的提醒時間！", "danger");
    return;
  }

  // 將本地時間轉換為 ISO 字串儲存（會自動轉為 UTC）
  // 但 JavaScript 的 Date 會正確處理時區
  fetch(`/api/checklist/${currentEditingItemId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reminderDate: reminderDate.toISOString() })
  }).then(() => {
    loadChecklist();
    showAlert("提醒時間已設定！", "success");
    reminderModal.hide();
  });
}

function clearReminder() {
  if (!confirm("確定要清除提醒時間嗎？")) {
    return;
  }

  fetch(`/api/checklist/${currentEditingItemId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reminderDate: null })
  }).then(() => {
    loadChecklist();
    showAlert("提醒時間已清除！", "success");
    reminderModal.hide();
  });
}

window.editItem = function(i) {
  const item = items[i];
  const textDiv = document.getElementById(`item-text-${item.id}`);

  textDiv.innerHTML = `
    <div class="d-flex align-items-center gap-2">
      <input type='text' class='form-control form-control-sm' id='edit-input-${item.id}' value='${item.text}' style='max-width: 300px;'>
      <button class='btn btn-sm btn-primary' onclick='saveEdit(${i})'>儲存</button>
      <button class='btn btn-sm btn-secondary' onclick='cancelEdit()'>取消</button>
    </div>`;
  document.getElementById(`edit-input-${item.id}`).focus();
}

window.saveEdit = function(i) {
  const item = items[i];
  const newText = document.getElementById(`edit-input-${item.id}`).value.trim();

  if (newText && newText !== item.text) {
    fetch(`/api/checklist/${item.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: newText })
    }).then(() => {
      loadChecklist();
      showAlert("項目已更新！", "success");
    });
  } else {
    loadChecklist();
  }
}

window.cancelEdit = function() {
  loadChecklist();
}

window.deleteItem = function(i) {
  const item = items[i];
  if (confirm("確定要刪除這個項目嗎？")) {
    fetch(`/api/checklist/${item.id}`, {
      method: "DELETE"
    }).then(() => {
      loadChecklist();
      showAlert("項目已刪除！", "success");
    });
  }
}

document.getElementById("item").addEventListener("keypress", function(e) {
  if (e.key === "Enter") {
    addItem();
  }
});

// Initial load
loadChecklist();
