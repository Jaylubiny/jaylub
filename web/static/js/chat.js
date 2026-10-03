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
const openGiftDialogButton = document.getElementById("open-gift-dialog");
const giftDialog = document.getElementById("coin-gift-dialog");
const giftForm = document.getElementById("coin-gift-form");
const giftType = document.getElementById("gift-type");
const giftRecipientField = document.getElementById("gift-recipient-field");
const giftRecipient = document.getElementById("gift-recipient");
const giftAmount = document.getElementById("gift-amount");

const currentUser = chatPage?.dataset.currentUser || "";
const timeFormat = chatPage?.dataset.timeFormat || "24h";
const messageDensity = chatPage?.dataset.messageDensity || "comfortable";
const showTimestamps = chatPage?.dataset.showTimestamps !== "false";
const showImagePreviews = chatPage?.dataset.showImagePreviews !== "false";
let lastMessageId = 0;
let globalMessageCursor = 0;
let globalMessageCursorReady = false;
let unreadTitleCount = 0;
let unreadPollInProgress = false;
let polling = false;
let activeChannel = "global";
const originalDocumentTitle = document.title;

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

function updateUnreadDocumentTitle() {
  document.title = unreadTitleCount > 0
    ? `+${unreadTitleCount} — ${originalDocumentTitle}`
    : originalDocumentTitle;
}

function observeGlobalMessages(messages) {
  if (!Array.isArray(messages)) return;
  if (!globalMessageCursorReady) {
    for (const message of messages) {
      globalMessageCursor = Math.max(globalMessageCursor, message.id);
    }
    globalMessageCursorReady = true;
    return;
  }

  for (const message of messages) {
    if (message.id <= globalMessageCursor) continue;
    globalMessageCursor = message.id;
    if (message.username !== currentUser && document.visibilityState === "hidden") {
      unreadTitleCount += 1;
    }
  }
  updateUnreadDocumentTitle();
}

async function pollGlobalMessagesForTitle() {
  if (activeChannel !== "self" || !globalMessageCursorReady || unreadPollInProgress) return;
  unreadPollInProgress = true;
  try {
    const response = await fetch(`/chat/messages?channel=global&after=${globalMessageCursor}`, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error("Could not check for new global messages.");
    }
    const data = await response.json();
    observeGlobalMessages(data.messages);
    clearError();
  } catch (error) {
    showError(error.message);
  } finally {
    unreadPollInProgress = false;
  }
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
  if (message.gift) {
    item.classList.add("gift-message");
    body.hidden = true;
    const gift = document.createElement("div");
    gift.className = "gift-event";
    const description = document.createElement("p");
    description.className = "gift-event-description";
    description.textContent = formatGiftDescription(message.gift);
    gift.append(description);
    if (message.gift.type === "card") {
      gift.dataset.giftCard = String(message.gift.id);
    } else if (message.gift.type === "claim") {
      const originalCard = messageList.querySelector(`[data-gift-card="${message.gift.id}"]`);
      if (originalCard) {
        const originalDescription = originalCard.querySelector(".gift-event-description");
        if (originalDescription) originalDescription.textContent = formatGiftDescription(message.gift);
        originalCard.querySelector("[data-claim-gift]")?.remove();
      }
    }
    if (message.gift.type === "card" && !message.gift.claimed && !message.gift.expired && message.gift.senderUsername !== currentUser) {
      const claimButton = document.createElement("button");
      claimButton.className = "gift-claim-button";
      claimButton.type = "button";
      claimButton.dataset.claimGift = String(message.gift.id);
      claimButton.textContent = `Claim ${message.gift.amount} coins`;
      gift.append(claimButton);
    }
    item.append(gift);
  }
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

function formatGiftDescription(gift) {
  if (gift.type === "direct") {
    return `${gift.senderUsername} gifted ${gift.amount} Jaylive coins to ${gift.recipientUsername}.`;
  }
  if (gift.type === "claim") {
    return `${gift.claimedByUsername || gift.recipientUsername} claimed a ${gift.amount}-coin gift card from ${gift.senderUsername}.`;
  }
  if (gift.claimed) {
    return `${gift.senderUsername} posted a ${gift.amount}-coin gift card, claimed by ${gift.claimedByUsername}.`;
  }
  if (gift.expired) {
    return `${gift.senderUsername} posted a ${gift.amount}-coin gift card that expired unclaimed.`;
  }
  return `${gift.senderUsername} posted a ${gift.amount}-coin Jaylive gift card.`;
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
    if (requestedChannel === "global") {
      observeGlobalMessages(data.messages);
    }
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
    openGiftDialogButton.hidden = channel !== "global";
    channelButtons.forEach((tab) => {
      const selected = tab === button;
      tab.classList.toggle("is-active", selected);
      tab.setAttribute("aria-pressed", String(selected));
    });
    loadMessages();
  });
});

function updateGiftRecipientVisibility() {
  const directGift = giftType.value === "direct";
  giftRecipientField.hidden = !directGift;
  giftRecipient.disabled = !directGift || giftRecipient.options.length === 0;
  giftRecipient.required = directGift;
}

async function loadGiftRecipients() {
  const response = await fetch("/chat/gift-recipients", {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error("Could not load gift recipients.");
  }
  const data = await response.json();
  giftRecipient.replaceChildren();
  for (const recipient of data.recipients || []) {
    const option = document.createElement("option");
    option.value = String(recipient.id);
    option.textContent = recipient.username;
    giftRecipient.append(option);
  }
  updateGiftRecipientVisibility();
}

openGiftDialogButton.addEventListener("click", async () => {
  clearError();
  try {
    await loadGiftRecipients();
    updateGiftRecipientVisibility();
    giftDialog.showModal();
  } catch (error) {
    showError(error.message);
  }
});

giftType.addEventListener("change", updateGiftRecipientVisibility);
document.getElementById("cancel-gift").addEventListener("click", () => giftDialog.close());

giftForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const submitButton = giftForm.querySelector('button[type="submit"]');
  submitButton.disabled = true;
  try {
    const response = await fetch("/chat/gift", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({
        type: giftType.value,
        amount: Number(giftAmount.value),
        recipientId: Number(giftRecipient.value) || 0,
      }),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(error.trim() || "Could not send gift.");
    }
    const data = await response.json();
    if (data.message && activeChannel === "global") appendMessage(data.message);
    giftForm.reset();
    updateGiftRecipientVisibility();
    giftDialog.close();
  } catch (error) {
    showError(error.message.trim() || "Could not send gift.");
  } finally {
    submitButton.disabled = false;
  }
});

messageList.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-claim-gift]");
  if (!button) return;
  button.disabled = true;
  let alreadyClaimed = false;
  try {
    const response = await fetch("/chat/gift/claim", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ giftId: Number(button.dataset.claimGift) }),
    });
    if (!response.ok) {
      const error = await response.text();
      alreadyClaimed = response.status === 409;
      button.textContent = response.status === 410 ? "Expired" : alreadyClaimed ? "Already claimed" : "Try again";
      if (response.status === 410) {
        button.closest(".message")?.querySelector(".gift-event-description")?.replaceChildren("This gift card has expired.");
      }
      throw new Error(error.trim() || "Could not claim gift card.");
    }
    const data = await response.json();
    button.textContent = "Claimed by you";
    const giftCard = button.closest(".message");
    if (giftCard) {
      const description = giftCard.querySelector(".gift-event-description");
      if (description) description.textContent = "You claimed this gift card.";
    }
    if (data.message) appendMessage(data.message);
  } catch (error) {
    showError(error.message.trim() || "Could not claim gift card.");
  } finally {
    if (button.isConnected) {
      button.disabled = alreadyClaimed || button.textContent === "Claimed by you" || button.textContent === "Expired";
    }
  }
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

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible" && unreadTitleCount > 0) {
    unreadTitleCount = 0;
    updateUnreadDocumentTitle();
  }
});

loadMessages().then(scrollToBottom);
setInterval(loadMessages, 2000);
setInterval(pollGlobalMessagesForTitle, 2000);
