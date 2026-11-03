document.addEventListener("DOMContentLoaded", () => {
  const countdown = document.querySelector("[data-lockout-countdown]");
  const submitButton = document.querySelector(".login-submit");
  if (countdown && submitButton) {
    let remaining = Number.parseInt(countdown.dataset.seconds || "0", 10);
    if (Number.isFinite(remaining) && remaining > 0) {
      submitButton.disabled = true;
      const updateCountdown = () => {
        if (remaining <= 0) {
          countdown.textContent = "now";
          submitButton.disabled = false;
          return false;
        }
        const minutes = Math.floor(remaining / 60);
        const seconds = remaining % 60;
        countdown.textContent = minutes > 0
          ? `${minutes}m ${seconds}s`
          : `${seconds} seconds`;
        remaining -= 1;
        return true;
      };
      if (updateCountdown()) {
        const timer = window.setInterval(() => {
          if (!updateCountdown()) window.clearInterval(timer);
        }, 1000);
      }
    } else {
      countdown.textContent = "now";
    }
  }

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
