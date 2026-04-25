// discord.js
const blocks = document.querySelectorAll(".about-block");

const observer = new IntersectionObserver(entries => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            entry.target.classList.add("visible");
        }
    });
}, { threshold: 0.2 });

blocks.forEach(b => observer.observe(b));