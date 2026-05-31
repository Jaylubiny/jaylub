const username = "preclik02";
const contributionGrid = document.getElementById("contribution-grid");
const contributionTotal = document.getElementById("contribution-total");
const monthLabels = document.getElementById("month-labels");
const activityStatus = document.getElementById("activity-status");
const refreshButton = document.getElementById("refresh-activity");

function dateKey(date) {
    return date.toISOString().slice(0, 10);
}

function startOfContributionYear() {
    const end = new Date();
    end.setHours(0, 0, 0, 0);

    const start = new Date(end);
    start.setDate(start.getDate() - 364);
    start.setDate(start.getDate() - start.getDay());
    return start;
}

function buildDays() {
    const start = startOfContributionYear();
    const days = [];

    for (let i = 0; i < 371; i += 1) {
        const day = new Date(start);
        day.setDate(start.getDate() + i);
        days.push(day);
    }

    return days;
}

function levelFromCount(count) {
    if (count <= 0) return 0;
    if (count <= 2) return 1;
    if (count <= 5) return 2;
    if (count <= 9) return 3;
    return 4;
}

function monthName(date) {
    return new Intl.DateTimeFormat(undefined, { month: "short" }).format(date);
}

function renderMonthLabels(days) {
    monthLabels.innerHTML = "";

    days.forEach((day, index) => {
        if (day.getDay() !== 0) return;

        const label = document.createElement("span");
        label.className = "month-label";
        label.textContent = day.getDate() <= 7 ? monthName(day) : "";
        monthLabels.appendChild(label);
    });
}

function renderContributions(contributions, total, sourceLabel) {
    const byDate = new Map(contributions.map((item) => [
        item.date,
        {
            count: Number(item.count) || 0,
            level: Number.isInteger(item.level) ? item.level : levelFromCount(Number(item.count) || 0),
        },
    ]));
    const days = buildDays();

    contributionGrid.innerHTML = "";
    renderMonthLabels(days);

    days.forEach((day) => {
        const data = byDate.get(dateKey(day)) || { count: 0, level: 0 };
        const cell = document.createElement("span");

        cell.className = "contribution-day";
        cell.dataset.level = String(Math.max(0, Math.min(data.level, 4)));
        cell.title = `${data.count} contributions on ${dateKey(day)}`;
        contributionGrid.appendChild(cell);
    });

    contributionTotal.textContent = `${total} contributions in the last year`;
    activityStatus.textContent = sourceLabel;
}

async function loadContributionCalendar() {
    const response = await fetch(`https://github-contributions-api.jogruber.de/v4/${username}?y=last`, {
        headers: {
            Accept: "application/json",
        },
    });

    if (!response.ok) {
        throw new Error(`Contribution API returned ${response.status}`);
    }

    const data = await response.json();
    const contributions = (data.contributions || []).map((item) => ({
        date: item.date,
        count: item.count,
        level: item.level,
    }));
    const total = data.total?.lastYear ?? contributions.reduce((sum, item) => sum + Number(item.count || 0), 0);

    return { contributions, total, sourceLabel: "Contribution calendar loaded from public GitHub data." };
}

async function loadPublicEventsFallback() {
    const response = await fetch(`https://api.github.com/users/${username}/events/public`, {
        headers: {
            Accept: "application/vnd.github+json",
        },
    });

    if (!response.ok) {
        throw new Error(`GitHub returned ${response.status}`);
    }

    const counts = new Map();
    const events = await response.json();

    events.forEach((event) => {
        const key = event.created_at.slice(0, 10);
        const amount = event.type === "PushEvent" ? event.payload?.commits?.length || 1 : 1;
        counts.set(key, (counts.get(key) || 0) + amount);
    });

    const contributions = Array.from(counts, ([date, count]) => ({
        date,
        count,
        level: levelFromCount(count),
    }));
    const total = contributions.reduce((sum, item) => sum + item.count, 0);

    return {
        contributions,
        total,
        sourceLabel: "Showing recent public events because the full contribution calendar could not be loaded.",
    };
}

async function loadActivity() {
    activityStatus.classList.remove("error");
    activityStatus.textContent = "Loading GitHub contribution graph...";
    contributionTotal.textContent = "Loading contributions...";
    refreshButton.disabled = true;

    try {
        let data;

        try {
            data = await loadContributionCalendar();
        } catch (error) {
            console.warn(error);
            data = await loadPublicEventsFallback();
        }

        renderContributions(data.contributions, data.total, data.sourceLabel);
    } catch (error) {
        contributionGrid.innerHTML = "";
        monthLabels.innerHTML = "";
        contributionTotal.textContent = "GitHub activity";
        activityStatus.classList.add("error");
        activityStatus.textContent = "Could not load GitHub activity right now. Try again later.";
        console.error(error);
    } finally {
        refreshButton.disabled = false;
    }
}

refreshButton.addEventListener("click", loadActivity);
loadActivity();
