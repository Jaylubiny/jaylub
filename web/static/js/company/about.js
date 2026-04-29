// /web/static/js/company/about.js

document.addEventListener("DOMContentLoaded", () => {
    const emailBtn = document.getElementById("emailBtn");

    emailBtn.addEventListener("click", () => {
        window.location.href = "mailto:support@jaylub.com";
    });
});