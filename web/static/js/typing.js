(() => {
  const words = [
    "able", "about", "above", "accept", "account", "action", "active", "adapt",
    "add", "adjust", "advice", "after", "again", "agree", "ahead", "allow",
    "almost", "alone", "along", "already", "also", "always", "amount", "answer",
    "any", "appear", "apply", "area", "around", "arrange", "ask", "attend",
    "avoid", "aware", "back", "balance", "basic", "become", "begin", "behind",
    "believe", "best", "better", "between", "build", "business", "call", "calm",
    "carry", "cause", "change", "check", "choose", "clear", "close", "common",
    "compare", "complete", "consider", "continue", "control", "correct", "create",
    "current", "daily", "decide", "deep", "define", "develop", "different",
    "direct", "discover", "discuss", "during", "early", "easy", "effect",
    "effort", "either", "enough", "ensure", "enter", "equal", "event", "every",
    "example", "expect", "explain", "extra", "fact", "fair", "fast", "feature",
    "feel", "field", "final", "find", "focus", "follow", "force", "form",
    "forward", "free", "full", "future", "general", "get", "give", "goal",
    "good", "great", "group", "grow", "guide", "habit", "handle", "happen",
    "happy", "hard", "help", "high", "hold", "hope", "idea", "impact",
    "improve", "include", "indeed", "inform", "inside", "instead", "interest",
    "item", "join", "just", "keep", "kind", "know", "large", "last", "later",
    "learn", "leave", "level", "life", "light", "likely", "limit", "list",
    "little", "local", "long", "look", "main", "make", "manage", "many",
    "match", "matter", "maybe", "meaning", "measure", "meet", "member",
    "method", "might", "mind", "model", "moment", "more", "most", "move",
    "much", "name", "near", "need", "never", "next", "notice", "number",
    "offer", "often", "once", "open", "option", "order", "other", "outside",
    "own", "page", "part", "people", "perhaps", "place", "plan", "play",
    "point", "possible", "power", "prepare", "present", "press", "pretty",
    "prevent", "problem", "process", "produce", "program", "proper", "provide",
    "public", "purpose", "quality", "quick", "quiet", "quite", "raise", "reach",
    "read", "ready", "real", "reason", "receive", "record", "reduce", "refer",
    "regular", "relate", "remain", "remember", "remove", "repeat", "reply",
    "report", "require", "result", "return", "review", "right", "role", "room",
    "rule", "run", "safe", "same", "save", "search", "second", "select",
    "sense", "serve", "set", "share", "short", "show", "simple", "since",
    "single", "small", "solve", "sort", "sound", "space", "speak", "special",
    "start", "state", "stay", "step", "still", "stop", "story", "strong",
    "study", "subject", "success", "suggest", "support", "sure", "system",
    "table", "take", "talk", "task", "team", "tell", "test", "thank",
    "there", "thing", "think", "this", "though", "time", "today", "together",
    "total", "toward", "train", "treat", "true", "trust", "turn", "under",
    "understand", "until", "update", "use", "usual", "value", "various",
    "very", "view", "visit", "wait", "walk", "want", "watch", "water", "well",
    "what", "when", "where", "whether", "whole", "wide", "will", "wish",
    "within", "without", "work", "world", "write", "year", "young"
  ];

  const app = document.querySelector("[data-typing-app]");
  if (!app) return;

  const modeSelect = app.querySelector("[data-mode]");
  const target = app.querySelector("[data-target]");
  const result = app.querySelector("[data-result]");
  const timeDisplay = app.querySelector("[data-time]");
  const wpmDisplay = app.querySelector("[data-wpm]");
  const accuracyDisplay = app.querySelector("[data-accuracy]");
  const personalBestDisplay = app.querySelector("[data-personal-best]");
  const statsPB = app.querySelector("[data-stats-pb]");
  const statsAccuracy = app.querySelector("[data-stats-accuracy]");
  const testCount = app.querySelector("[data-test-count]");
  const leaderboard = app.querySelector("[data-leaderboard]");
  const restartButton = app.querySelector("[data-restart]");

  const savedTextSize = Number.parseInt(localStorage.getItem("typingTextSize") || "", 10);
  if (Number.isInteger(savedTextSize) && savedTextSize >= 1 && savedTextSize <= 25) {
    document.documentElement.style.setProperty("--typing-text-size", `${savedTextSize}px`);
  }

  let text = "";
  let typed = "";
  let cursor = 0;
  let durationSeconds = 30;
  let startedAt = 0;
  let timer = null;
  let totalTypedCharacters = 0;
  let completed = false;
  let renderedLength = 0;
  let lastWord = "";

  const getDuration = () => Number.parseInt(modeSelect.value, 10);

  function appendWord() {
    let nextWord;
    do {
      nextWord = words[Math.floor(Math.random() * words.length)];
    } while (nextWord === lastWord);
    if (text.length > 0) text += " ";
    text += nextWord;
    lastWord = nextWord;
  }

  function ensureTextAhead() {
    while (text.length - cursor < 120) appendWord();
  }

  function renderText() {
    ensureTextAhead();
    const characters = Array.from(text);
    for (let index = renderedLength; index < characters.length; index += 1) {
      const span = document.createElement("span");
      span.textContent = characters[index];
      target.append(span);
    }

    let correctCharacters = 0;
    for (let index = 0; index < target.children.length; index += 1) {
      const character = target.children[index];
      character.removeAttribute("data-state");
      if (index < typed.length) {
        const correct = typed[index] === characters[index];
        character.dataset.state = correct ? "correct" : "incorrect";
        if (correct) correctCharacters += 1;
      }
    }

    if (cursor < target.children.length) {
      target.children[cursor].dataset.state = "current";
    }
    renderedLength = characters.length;
    setCaret(cursor);
    updateMetrics(correctCharacters);
  }

  function setCaret(position) {
    const selection = window.getSelection();
    if (!selection || target.children.length === 0) return;

    const boundedPosition = Math.max(0, Math.min(position, target.children.length));
    const range = document.createRange();
    if (boundedPosition === target.children.length) {
      const lastCharacter = target.children[boundedPosition - 1];
      range.setStart(lastCharacter.firstChild, lastCharacter.firstChild.length);
    } else {
      range.setStart(target.children[boundedPosition].firstChild, 0);
    }
    range.collapse(true);
    selection.removeAllRanges();
    selection.addRange(range);
  }

  function updateMetrics(correctCharacters) {
    const seconds = startedAt ? Math.max((Date.now() - startedAt) / 1000, 1) : 0;
    const wpm = seconds > 0 ? Math.floor((correctCharacters / 5) / (seconds / 60)) : 0;
    const accuracy = totalTypedCharacters > 0
      ? (correctCharacters / totalTypedCharacters) * 100
      : 100;
    wpmDisplay.textContent = String(wpm);
    accuracyDisplay.textContent = `${Math.round(accuracy)}%`;
  }

  function reset() {
    if (timer !== null) window.clearInterval(timer);
    timer = null;
    durationSeconds = getDuration();
    startedAt = 0;
    totalTypedCharacters = 0;
    completed = false;
    typed = "";
    cursor = 0;
    target.contentEditable = "true";
    lastWord = "";
    renderedLength = 0;
    text = "";
    target.replaceChildren();
    timeDisplay.textContent = String(durationSeconds);
    wpmDisplay.textContent = "0";
    accuracyDisplay.textContent = "100%";
    result.textContent = "";
    renderText();
    target.focus();
    refresh();
  }

  function startTimer() {
    startedAt = Date.now();
    timer = window.setInterval(() => {
      const elapsedSeconds = Math.floor((Date.now() - startedAt) / 1000);
      timeDisplay.textContent = String(Math.max(durationSeconds - elapsedSeconds, 0));
      renderText();
      if (elapsedSeconds >= durationSeconds) finishTest();
    }, 200);
  }

  function editInput(event) {
    event.preventDefault();
    if (completed) return;

    if (event.inputType === "insertText" && event.data) {
      if (!startedAt) startTimer();
      typed = typed.slice(0, cursor) + event.data + typed.slice(cursor);
      cursor += event.data.length;
      totalTypedCharacters += event.data.length;
    } else if (event.inputType === "insertFromPaste" && event.data) {
      if (!startedAt) startTimer();
      typed = typed.slice(0, cursor) + event.data + typed.slice(cursor);
      cursor += event.data.length;
      totalTypedCharacters += event.data.length;
    } else if (event.inputType === "deleteContentBackward" && cursor > 0) {
      typed = typed.slice(0, cursor - 1) + typed.slice(cursor);
      cursor -= 1;
    } else if (event.inputType === "deleteContentForward" && cursor < typed.length) {
      typed = typed.slice(0, cursor) + typed.slice(cursor + 1);
    } else {
      return;
    }
    renderText();
  }

  async function finishTest() {
    if (completed) return;
    completed = true;
    if (timer !== null) window.clearInterval(timer);
    timer = null;
    target.contentEditable = "false";
    renderText();

    const seconds = Math.max((Date.now() - startedAt) / 1000, 1);
    let correctCharacters = 0;
    for (let index = 0; index < typed.length; index += 1) {
      if (typed[index] === text[index]) correctCharacters += 1;
    }
    const wpm = Math.floor((correctCharacters / 5) / (seconds / 60));
    const accuracy = totalTypedCharacters > 0
      ? (correctCharacters / totalTypedCharacters) * 100
      : 100;
    updateMetrics(correctCharacters);

    if (wpm < 5 || wpm > 260) {
      result.textContent = `Test complete: ${wpm} WPM. Scores must be between 5 and 260 WPM to be submitted.`;
      return;
    }

    result.textContent = `Test complete: ${wpm} WPM at ${Math.round(accuracy)}% accuracy. Saving score…`;
    try {
      const response = await fetch("/api/typing/score", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          wpm,
          accuracy: Number(accuracy.toFixed(2)),
          mode: modeSelect.value
        })
      });
      if (response.status === 401) {
        result.textContent = "Test complete, but you need to sign in to save your score.";
        return;
      }
      if (!response.ok) throw new Error("Score could not be saved.");
      const saved = await response.json();
      result.textContent = `Test complete: ${wpm} WPM at ${Math.round(accuracy)}% accuracy${saved.is_new_pb ? " — new personal best!" : "!"}`;
      await refresh();
    } catch (error) {
      result.textContent = `${error.message} Your result is still shown above.`;
    }
  }

  function formatDate(value) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "—" : date.toLocaleDateString();
  }

  async function loadLeaderboard() {
    const response = await fetch(`/api/typing/leaderboard?mode=${encodeURIComponent(modeSelect.value)}`);
    if (!response.ok) throw new Error("Leaderboard is unavailable.");
    const entries = await response.json();
    if (entries.length === 0) {
      leaderboard.innerHTML = '<tr><td colspan="5">No scores yet for this mode.</td></tr>';
      return;
    }
    leaderboard.replaceChildren(...entries.map((entry) => {
      const row = document.createElement("tr");
      [entry.rank, entry.username, entry.wpm, `${Math.round(entry.accuracy)}%`, formatDate(entry.created_at)]
        .forEach((value) => {
          const cell = document.createElement("td");
          cell.textContent = String(value);
          row.append(cell);
        });
      return row;
    }));
  }

  async function loadUserStats() {
    const response = await fetch(`/api/typing/user-stats?mode=${encodeURIComponent(modeSelect.value)}`);
    if (response.status === 401) {
      personalBestDisplay.textContent = "Sign in to save scores";
      statsPB.textContent = "—";
      statsAccuracy.textContent = "—";
      testCount.textContent = "0";
      return;
    }
    if (!response.ok) throw new Error("Personal stats are unavailable.");
    const stats = await response.json();
    const best = `${stats.pb_wpm} WPM`;
    personalBestDisplay.textContent = best;
    statsPB.textContent = best;
    statsAccuracy.textContent = `${Number(stats.average_accuracy).toFixed(1)}%`;
    testCount.textContent = String(stats.test_count);
  }

  async function refresh() {
    const results = await Promise.allSettled([loadLeaderboard(), loadUserStats()]);
    if (results.some((item) => item.status === "rejected") &&
        !result.textContent.trim()) {
      result.textContent = "Some statistics could not be loaded. You can still start a test.";
    }
  }

  target.addEventListener("beforeinput", editInput);
  target.addEventListener("paste", (event) => {
    event.preventDefault();
    const pastedText = event.clipboardData?.getData("text/plain");
    if (pastedText) {
      editInput({
        preventDefault: () => {},
        inputType: "insertText",
        data: pastedText
      });
    }
  });
  target.addEventListener("keydown", (event) => {
    if (event.key === "Tab") {
      event.preventDefault();
      reset();
    } else if (
      event.key === "ArrowLeft" ||
      event.key === "ArrowRight" ||
      event.key === "ArrowUp" ||
      event.key === "ArrowDown" ||
      event.key === "Home" ||
      event.key === "End" ||
      event.key === "PageUp" ||
      event.key === "PageDown"
    ) {
      event.preventDefault();
      setCaret(cursor);
    } else if (event.key === "Enter") {
      event.preventDefault();
    }
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      reset();
    }
  });
  target.addEventListener("pointerdown", (event) => {
    event.preventDefault();
    target.focus();
    setCaret(cursor);
  });
  target.addEventListener("selectstart", (event) => event.preventDefault());
  target.addEventListener("dragstart", (event) => event.preventDefault());

  restartButton.addEventListener("click", reset);
  modeSelect.addEventListener("change", reset);
  reset();
})();
