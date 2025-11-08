let items = [];

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
      itemElement.innerHTML = `
        <div class="form-check me-3"><input class="form-check-input" type="checkbox" ${item.checked ? "checked" : ""} onchange="toggle(${i})"></div>
        <div class="flex-grow-1" id="item-text-${item.id}"><span class="${item.checked ? 'completed-item' : ''}">${item.text}</span></div>
        <button class="btn btn-sm btn-outline-secondary ms-2" onclick="editItem(${i})"><i class="bi bi-pencil"></i></button>
        <button class="btn btn-sm btn-outline-danger ms-2" onclick="deleteItem(${i})"><i class="bi bi-trash"></i></button>`;
      list.appendChild(itemElement);
    });
  }
}

function addItem() {
  const input = document.getElementById("item");
  if (input.value.trim()) {
    fetch("/api/checklist", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items: [{ text: input.value, checked: false }] })
    }).then(() => { input.value = ""; loadChecklist(); showAlert("項目新增成功！", "success"); });
  }
}

function toggle(i) {
  const item = items[i];
  fetch(`/api/checklist/${item.id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ checked: !item.checked })
  }).then(() => { loadChecklist(); showAlert("項目已完成！", "success"); });
}

window.editItem = function(i) {
  const item = items[i];
  const textDiv = document.getElementById(`item-text-${item.id}`);
  textDiv.innerHTML = `<input type='text' class='form-control form-control-sm' id='edit-input-${item.id}' value='${item.text}' style='max-width: 200px; display: inline-block;'><button class='btn btn-sm btn-primary ms-1' onclick='saveEdit(${i})'>儲存</button><button class='btn btn-sm btn-secondary ms-1' onclick='cancelEdit()'>取消</button>`;
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
    }).then(() => { loadChecklist(); showAlert("項目已更新！", "success"); });
  } else {
    loadChecklist();
  }
}
window.cancelEdit = function() { loadChecklist(); }
window.deleteItem = function(i) {
  const item = items[i];
  if (confirm("確定要刪除這個項目嗎？")) {
    fetch(`/api/checklist/${item.id}`, { method: "DELETE" }).then(() => { loadChecklist(); showAlert("項目已刪除！", "success"); });
  }
}
document.getElementById("item").addEventListener("keypress", function(e) { if (e.key === "Enter") { addItem(); } });

// Initial load
loadChecklist();
