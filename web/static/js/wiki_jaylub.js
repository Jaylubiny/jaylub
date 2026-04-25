document.addEventListener("DOMContentLoaded", () => {
    document.querySelectorAll(".topbar a").forEach(link => {
        link.addEventListener("click", (e) => {
            e.preventDefault();

            const id = link.getAttribute("href").substring(1);
            const target = document.getElementById(id);

            if (target) {
                target.scrollIntoView({
                    behavior: "smooth",
                    block: "start"
                });
            }
        });
    });
});