document.addEventListener("DOMContentLoaded", function() {
  const navContainer = document.getElementById("nav-container");
  if (navContainer) {
    fetch("nav.html")
      .then(response => response.text())
      .then(data => {
        navContainer.innerHTML = data;
        // Set active tab
        const page = window.location.pathname.split("/").pop();
        if (page === "index.html") {
          document.getElementById("nav-checklist").classList.add("active");
        } else if (page === "orders.html") {
          document.getElementById("nav-orders").classList.add("active");
        } else if (page === "picking.html") {
          document.getElementById("nav-picking").classList.add("active");
        } else if (page === "sell-picking.html") {
          document.getElementById("nav-sell-picking").classList.add("active");
        } else if (page === "sell-orders.html") {
          document.getElementById("nav-sell-orders").classList.add("active");
        }
      });
  }
});
