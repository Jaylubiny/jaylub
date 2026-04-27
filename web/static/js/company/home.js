document.addEventListener("DOMContentLoaded", () => {
    console.log("Jaylubiny subdomain loaded.");

    // Simple hover animation enhancement (optional polish)
    const cards = document.querySelectorAll(".card");

    cards.forEach(card => {
        card.addEventListener("mouseenter", () => {
            card.style.transition = "transform 0.15s ease";
        });

        card.addEventListener("mouseleave", () => {
            card.style.transition = "transform 0.2s ease";
        });
    });

    // Example future hook (analytics / company logic)
    function track(event) {
        console.log("[Jaylubiny analytics]", event);
    }

    cards.forEach(card => {
        card.addEventListener("click", () => {
            track("card_click:" + card.querySelector("h2")?.innerText);
        });
    });
});
