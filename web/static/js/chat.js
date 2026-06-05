const chatPage = document.querySelector(".chat-page");
const messageList = document.getElementById("message-list");
const chatForm = document.getElementById("chat-form");
const messageInput = document.getElementById("message-input");
const onlineCount = document.getElementById("online-count");
const chatError = document.getElementById("chat-error");

const currentUser = chatPage?.dataset.currentUser || "";
let lastMessageId = 0;
let polling = false;

function nearBottom() {
  return messageList.scrollHeight - messageList.scrollTop - messageList.clientHeight < 80;
}

function scrollToBottom() {
  messageList.scrollTop = messageList.scrollHeight;
}

function showError(message) {
  chatError.textContent = message;
  chatError.hidden = false;
}

function clearError() {
  chatError.textContent = "";
  chatError.hidden = true;
}

function formatTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function appendMessage(message) {
  if (message.id <= lastMessageId) {
    return;
  }

  const shouldScroll = nearBottom();
  const item = document.createElement("article");
  item.className = "message";
  if (message.username === currentUser) {
    item.classList.add("own");
  }

  const meta = document.createElement("div");
  meta.className = "message-meta";

  const username = document.createElement("span");
  username.className = "message-user";
  username.textContent = message.username;

  const timestamp = document.createElement("time");
  timestamp.className = "message-time";
  timestamp.dateTime = message.timestamp;
  timestamp.textContent = formatTime(message.timestamp);

  const body = document.createElement("p");
  body.className = "message-body";
  body.textContent = message.message;

  meta.append(username, timestamp);
  item.append(meta, body);
  messageList.append(item);

  lastMessageId = message.id;
  if (shouldScroll) {
    scrollToBottom();
  }
}

async function loadMessages() {
  if (polling) {
    return;
  }

  polling = true;
  try {
    const response = await fetch(`/chat/messages?after=${lastMessageId}`, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error("Could not load messages.");
    }

    const data = await response.json();
    for (const message of data.messages || []) {
      appendMessage(message);
    }
    if (typeof data.onlineCount === "number") {
      onlineCount.textContent = data.onlineCount;
    }
    clearError();
  } catch (error) {
    showError(error.message);
  } finally {
    polling = false;
  }
}

chatForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError();

  const message = messageInput.value.trim();
  if (!message) {
    return;
  }
  if ([...message].length > 500) {
    showError("Message must be 500 characters or fewer.");
    return;
  }

  const button = chatForm.querySelector("button");
  button.disabled = true;

  try {
    const response = await fetch("/chat/send", {
      method: "POST",
      headers: {
        "Accept": "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ message }),
    });
    if (!response.ok) {
      throw new Error(await response.text());
    }

    const data = await response.json();
    if (data.message) {
      appendMessage(data.message);
      scrollToBottom();
    }
    if (typeof data.onlineCount === "number") {
      onlineCount.textContent = data.onlineCount;
    }
    messageInput.value = "";
  } catch (error) {
    showError(error.message.trim() || "Could not send message.");
  } finally {
    button.disabled = false;
    messageInput.focus();
  }
});

messageInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    chatForm.requestSubmit();
  }
});

loadMessages().then(scrollToBottom);
setInterval(loadMessages, 2000);
