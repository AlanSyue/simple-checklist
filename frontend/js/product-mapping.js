let mappingsCache = [];

document.addEventListener("DOMContentLoaded", () => {
  loadMappings();
});

async function loadMappings() {
  const body = document.getElementById("mappings-body");

  try {
    const response = await fetch("/api/product-mappings");
    if (!response.ok) {
      throw new Error("Failed to load mappings");
    }

    const mappings = await response.json();
    mappingsCache = mappings;
    renderMappings(mappings);
  } catch (error) {
    console.error(error);
    body.innerHTML = `<tr><td colspan="4" class="text-center text-danger">載入失敗：${error.message}</td></tr>`;
  }
}

function renderMappings(mappings) {
  const body = document.getElementById("mappings-body");

  if (!mappings || mappings.length === 0) {
    body.innerHTML = `<tr><td colspan="4" class="text-center text-muted">尚無商品名稱對應資料，請點擊「同步商品名稱」按鈕</td></tr>`;
    return;
  }

  body.innerHTML = "";

  mappings.forEach(mapping => {
    const row = document.createElement("tr");
    const sourceBadge = mapping.source === "woocommerce"
      ? '<span class="badge bg-success">官網</span>'
      : '<span class="badge bg-info">賣貨便</span>';

    row.innerHTML = `
      <td>${mapping.original_name}</td>
      <td>${sourceBadge}</td>
      <td>
        <input type="text"
               class="form-control form-control-sm mapped-name-input"
               value="${mapping.mapped_name}"
               data-mapping-id="${mapping.id}"
               onchange="updateMappedName(${mapping.id}, this.value)">
      </td>
      <td class="text-center">
        <button class="btn btn-sm btn-outline-primary" onclick="resetMapping(${mapping.id}, '${mapping.original_name}')">
          <i class="bi bi-arrow-counterclockwise"></i> 重置
        </button>
      </td>
    `;
    body.appendChild(row);
  });
}

async function syncProductNames() {
  const syncBtn = document.getElementById("sync-btn");
  const originalHTML = syncBtn.innerHTML;

  syncBtn.disabled = true;
  syncBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>同步中...';

  try {
    const response = await fetch("/api/product-mappings/sync", {
      method: "POST"
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => response.statusText);
      throw new Error(errorText || "同步失敗");
    }

    const result = await response.json();
    showMessage(result.message || "同步成功", "success");
    loadMappings();
  } catch (error) {
    console.error(error);
    showMessage("同步失敗：" + error.message, "danger");
  } finally {
    syncBtn.disabled = false;
    syncBtn.innerHTML = originalHTML;
  }
}

async function updateMappedName(mappingId, newMappedName) {
  if (!newMappedName || newMappedName.trim() === "") {
    showMessage("對應名稱不能為空", "warning");
    loadMappings(); // 重新載入以恢復原值
    return;
  }

  try {
    const response = await fetch(`/api/product-mappings/${mappingId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({
        mapped_name: newMappedName.trim()
      })
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => response.statusText);
      throw new Error(errorText || "更新失敗");
    }

    showMessage("對應名稱已更新", "success");
    loadMappings(); // 重新載入
  } catch (error) {
    console.error(error);
    showMessage("更新失敗：" + error.message, "danger");
    loadMappings(); // 重新載入以恢復原值
  }
}

function resetMapping(mappingId, originalName) {
  updateMappedName(mappingId, originalName);
}

function showMessage(text, type) {
  const container = document.getElementById("result-message");
  if (!container) return;

  container.innerHTML = `
    <div class="alert alert-${type} alert-dismissible fade show" role="alert">
      ${text}
      <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    </div>
  `;

  // 3秒後自動消失
  setTimeout(() => {
    const alert = container.querySelector('.alert');
    if (alert) {
      alert.remove();
    }
  }, 3000);
}
