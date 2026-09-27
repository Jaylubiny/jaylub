const chatPage = document.querySelector(".chat-page");
const messageList = document.getElementById("message-list");
const chatForm = document.getElementById("chat-form");
const messageInput = document.getElementById("message-input");
const onlineCount = document.getElementById("online-count");
const onlineUsersTooltip = document.getElementById("online-users-tooltip");
const chatError = document.getElementById("chat-error");
const chatConnection = document.getElementById("chat-connection");
const newMessages = document.getElementById("new-messages");
const messageCounter = document.getElementById("message-counter");
const fileInput = document.getElementById("file-input");
const fileButton = document.getElementById("file-button");
const fileName = document.getElementById("file-name");
const channelButtons = document.querySelectorAll("[data-chat-channel]");
const chatTitle = document.getElementById("chat-title");
const chatDescription = document.getElementById("chat-description");

const currentUser = chatPage?.dataset.currentUser || "";
const timeFormat = chatPage?.dataset.timeFormat || "24h";
const messageDensity = chatPage?.dataset.messageDensity || "comfortable";
const showTimestamps = chatPage?.dataset.showTimestamps !== "false";
const showImagePreviews = chatPage?.dataset.showImagePreviews !== "false";
let lastMessageId = 0;
let polling = false;
let activeChannel = "global";

if (chatPage) {
  chatPage.dataset.messageDensity = messageDensity;
}

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

function updateOnlineUsers(users) {
  if (!Array.isArray(users)) return;

  onlineUsersTooltip.replaceChildren();
  if (users.length === 0) {
    const empty = document.createElement("span");
    empty.className = "online-users-empty";
    empty.textContent = "No users online";
    onlineUsersTooltip.append(empty);
    return;
  }

  const list = document.createElement("ul");
  for (const username of users) {
    const item = document.createElement("li");
    item.textContent = username;
    list.append(item);
  }
  onlineUsersTooltip.append(list);
}

function setConnection(connected) {
  chatConnection.textContent = connected ? "Connected" : "Reconnecting…";
  chatConnection.classList.toggle("offline", !connected);
  chatConnection.hidden = connected;
}

function formatTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  if (timeFormat === "relative") {
    const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
    if (seconds < 60) return "just now";
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
  }
  return date.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: timeFormat === "12h",
  });
}

function appendMessage(message) {
  const emptyState = messageList.querySelector(".chat-empty");
  if (emptyState) {
    emptyState.remove();
  }

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
  timestamp.hidden = !showTimestamps;

  const body = document.createElement("p");
  body.className = "message-body";
  body.textContent = message.message;

  meta.append(username, timestamp);
  item.append(meta, body);
  for (const attachment of message.attachments || []) {
    const link = document.createElement("a");
    link.className = "message-attachment";
    link.href = attachment.url;
    link.target = "_blank";
    link.rel = "noopener";
    link.download = attachment.name;

    if (attachment.contentType?.split(";", 1)[0].toLowerCase().startsWith("image/") && showImagePreviews) {
      const image = document.createElement("img");
      image.className = "message-image";
      image.src = attachment.url;
      image.alt = attachment.name;
      link.append(image);
    } else {
      link.textContent = `📎 ${attachment.name}`;
    }
    item.append(link);
  }
  messageList.append(item);

  lastMessageId = message.id;
  if (shouldScroll) {
    scrollToBottom();
    newMessages.hidden = true;
  } else {
    newMessages.hidden = false;
  }
}

async function loadMessages() {
  if (polling) {
    return;
  }

  const requestedChannel = activeChannel;
  polling = true;
  try {
    const response = await fetch(`/chat/messages?channel=${requestedChannel}&after=${lastMessageId}`, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error("Could not load messages.");
    }

    const data = await response.json();
    if (requestedChannel !== activeChannel) return;
    for (const message of data.messages || []) {
      appendMessage(message);
    }
    if (lastMessageId === 0) {
      const emptyState = messageList.querySelector(".chat-empty");
      if (emptyState) {
        emptyState.textContent = "No messages yet. Start the conversation.";
      }
    }
    if (typeof data.onlineCount === "number") {
      onlineCount.textContent = data.onlineCount;
    }
    updateOnlineUsers(data.onlineUsers);
    clearError();
    setConnection(true);
  } catch (error) {
    setConnection(false);
    showError("Could not load messages. Retrying soon.");
  } finally {
    polling = false;
    if (requestedChannel !== activeChannel) {
      loadMessages();
    }
  }
}

channelButtons.forEach((button) => {
  button.addEventListener("click", () => {
    const channel = button.dataset.chatChannel;
    if ((channel !== "global" && channel !== "self") || channel === activeChannel) return;

    activeChannel = channel;
    lastMessageId = 0;
    messageList.replaceChildren();
    const loading = document.createElement("p");
    loading.className = "chat-empty";
    loading.textContent = "Loading messages…";
    messageList.append(loading);
    newMessages.hidden = true;
    chatTitle.textContent = channel === "self" ? "Notes to self" : "Chat";
    chatDescription.textContent = channel === "self"
      ? "A private channel only you can see."
      : "Global room for logged-in Jaylub users. Messages update automatically.";
    channelButtons.forEach((tab) => {
      const selected = tab === button;
      tab.classList.toggle("is-active", selected);
      tab.setAttribute("aria-pressed", String(selected));
    });
    loadMessages();
  });
});

chatForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError();

  const message = messageInput.value.trim();
  const file = fileInput.files[0];
  if (!message && !file) {
    showError("Write a message or choose a file.");
    return;
  }
  if ([...message].length > 500) {
    showError("Message must be 500 characters or fewer.");
    return;
  }

  const button = chatForm.querySelector('button[type="submit"]');
  button.disabled = true;
  const sendChannel = activeChannel;

  try {
    const formData = new FormData();
    formData.append("message", message);
    if (file) {
      formData.append("file", file);
    }

    const response = await fetch(`/chat/send?channel=${sendChannel}`, {
      method: "POST",
      headers: {
        "Accept": "application/json",
      },
      body: formData,
    });
    if (!response.ok) {
      if (response.status === 429) {
        throw new Error("Please wait a moment before sending another message.");
      }
      throw new Error("Could not send message.");
    }

    const data = await response.json();
    if (data.message && activeChannel === sendChannel) {
      appendMessage(data.message);
      scrollToBottom();
    }
    if (typeof data.onlineCount === "number") {
      onlineCount.textContent = data.onlineCount;
    }
    updateOnlineUsers(data.onlineUsers);
    if (activeChannel === sendChannel) {
      messageInput.value = "";
      fileInput.value = "";
      fileName.textContent = "";
      messageCounter.textContent = "0 / 500";
    }
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

messageInput.addEventListener("input", () => {
  messageCounter.textContent = `${[...messageInput.value].length} / 500`;
});

fileButton.addEventListener("click", () => fileInput.click());
fileInput.addEventListener("change", () => {
  fileName.textContent = fileInput.files[0]?.name || "";
});

messageList.addEventListener("scroll", () => {
  if (nearBottom()) {
    newMessages.hidden = true;
  }
});

newMessages.addEventListener("click", scrollToBottom);

loadMessages().then(scrollToBottom);
setInterval(loadMessages, 2000);
