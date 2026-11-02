(() => {
  const settings = document.querySelector("[data-typing-settings]");
  if (!settings) return;

  const sizeInput = settings.querySelector("[data-text-size]");
  const sizeOutput = settings.querySelector("[data-text-size-value]");
  const status = settings.querySelector("[data-settings-status]");
  const resetButton = settings.querySelector("[data-reset-size]");
  const defaultSize = 18;

  function readSavedSize() {
    const savedSize = Number.parseInt(localStorage.getItem("typingTextSize") || "", 10);
    return Number.isInteger(savedSize) && savedSize >= 1 && savedSize <= 25
      ? savedSize
      : defaultSize;
  }

  function applySize(size) {
    sizeInput.value = String(size);
    sizeOutput.value = `${size} px`;
    sizeOutput.textContent = `${size} px`;
    document.documentElement.style.setProperty("--typing-text-size", `${size}px`);
    try {
      localStorage.setItem("typingTextSize", String(size));
      status.textContent = "Text size saved for this browser.";
    } catch {
      status.textContent = "Text size applied for this visit, but could not be saved in this browser.";
    }
  }

  sizeInput.addEventListener("input", () => {
    applySize(Number(sizeInput.value));
  });
  resetButton.addEventListener("click", () => applySize(defaultSize));
  applySize(readSavedSize());
})();
