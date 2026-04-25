document.addEventListener("DOMContentLoaded", () => {
    const btn = document.getElementById("cta-btn");

    btn.addEventListener("click", () => {
        btn.innerText = "Loading...";
        
        setTimeout(() => {
            btn.innerText = "Welcome!";
            btn.style.background = "#22c55e";
        }, 1000);
    });
});