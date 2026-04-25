// CTA button action
document.getElementById("cta-btn").addEventListener("click", () => {
    alert("Welcome to Jaylub 🚀");
});

// Scroll animation for about blocks
const blocks = document.querySelectorAll(".about-block");

const observer = new IntersectionObserver(entries => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            entry.target.classList.add("visible");
        }
    });
}, {
    threshold: 0.2
});

blocks.forEach(block => {
    observer.observe(block);
});