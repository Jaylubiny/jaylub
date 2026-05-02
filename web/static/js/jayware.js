document.addEventListener("DOMContentLoaded", () => {

    // Highlight danger section on load
    const dangerBox = document.querySelector(".danger");

    if (dangerBox) {
        setTimeout(() => {
            dangerBox.style.boxShadow = "0 0 20px rgba(255, 59, 59, 0.3)";
        }, 500);
    }

    // Add subtle fade-in animation for sections
    const boxes = document.querySelectorAll(".warning-box");

    boxes.forEach((box, index) => {
        box.style.opacity = "0";
        box.style.transform = "translateY(10px)";

        setTimeout(() => {
            box.style.transition = "0.5s ease";
            box.style.opacity = "1";
            box.style.transform = "translateY(0)";
        }, 150 * index);
    });

});