document.addEventListener("DOMContentLoaded", () => {
  const modal = document.querySelector("[data-signup-modal]");
  const openButton = document.querySelector("[data-open-signup]");
  const closeButtons = document.querySelectorAll("[data-close-signup]");

  if (!modal || !openButton) {
    return;
  }

  openButton.addEventListener("click", () => {
    modal.hidden = false;
  });

  closeButtons.forEach((button) => {
    button.addEventListener("click", () => {
      modal.hidden = true;
    });
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      modal.hidden = true;
    }
  });
});
