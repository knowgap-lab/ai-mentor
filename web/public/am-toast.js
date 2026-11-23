(function () {
  function showToast(message) {
    var el = document.createElement("div");
    el.setAttribute("role", "alert");
    el.className = "am-t";
    el.textContent = message;
    document.body.appendChild(el);
  }
  window.amShowToast = showToast;
})();