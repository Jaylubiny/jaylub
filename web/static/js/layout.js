document.addEventListener("DOMContentLoaded", () => {

    let pendingCursorFrame = 0;
    document.addEventListener("mousemove", (e) => {
        const x = `${e.clientX}px`;
        const y = `${e.clientY}px`;

        if (pendingCursorFrame) {
            cancelAnimationFrame(pendingCursorFrame);
        }
        pendingCursorFrame = requestAnimationFrame(() => {
            document.documentElement.style.setProperty("--cursor-x", x);
            document.documentElement.style.setProperty("--cursor-y", y);
            pendingCursorFrame = 0;
        });
    });

    const menu = document.querySelector("[data-user-menu]");
    const button = document.querySelector("[data-user-menu-button]");
    const dropdown = document.querySelector("[data-user-dropdown]");

    if (menu && button && dropdown) {
        button.addEventListener("click", () => {
            const isOpen = !dropdown.hidden;
            dropdown.hidden = isOpen;
            button.setAttribute("aria-expanded", String(!isOpen));
        });

        document.addEventListener("click", (event) => {
            if (!menu.contains(event.target)) {
                dropdown.hidden = true;
                button.setAttribute("aria-expanded", "false");
            }
        });

        document.addEventListener("keydown", (event) => {
            if (event.key === "Escape") {
                dropdown.hidden = true;
                button.setAttribute("aria-expanded", "false");
            }
        });
    }
});







// smooth scroll for internal links (optional)
document.querySelectorAll('a[href^="/"]').forEach(link => {
    link.addEventListener("click", () => {
        window.scrollTo({ top: 0, behavior: "smooth" });
    });
});
