document.addEventListener("DOMContentLoaded", () => {

    document.addEventListener("mousemove", (e) => {
        const x = e.clientX / window.innerWidth;
        const y = e.clientY / window.innerHeight;

        document.body.style.background = `
            radial-gradient(circle at ${x * 100}% ${y * 100}%,
            #1e293b,
            #020617)
        `;
    });
});







// smooth scroll for internal links (optional)
document.querySelectorAll('a[href^="/"]').forEach(link => {
    link.addEventListener("click", () => {
        window.scrollTo({ top: 0, behavior: "smooth" });
    });
});