(() => {
  const app = document.querySelector(".email-app");
  if (!app) return;

  const mailbox = app.dataset.mailbox.toLowerCase();
  const list = document.querySelector("#email-list");
  const folderTitle = document.querySelector("#folder-title");
  const search = document.querySelector("#email-search");
  const notice = document.querySelector("#email-notice");
  const reader = document.querySelector("#email-reader");
  const readerEmpty = document.querySelector("#reader-empty");
  const workspace = document.querySelector(".email-workspace");
  const readerPanel = document.querySelector(".email-reader-panel");
  const composeModal = document.querySelector("#compose-modal");
  const composeForm = document.querySelector("#compose-form");
  const composeError = document.querySelector("#compose-error");
  const state = { folder: "inbox", emails: [], selectedId: null, loading: false };
  const titles = { inbox: "Inbox", sent: "Sent", trash: "Trash" };

  function showNotice(message) {
    notice.textContent = message;
  }

  function selectedEmail() {
    return state.emails.find((email) => email.id === state.selectedId) || null;
  }

  function dateText(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value || "";
    return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
  }

  function emailOwner(email) {
    return state.folder === "sent" || (state.folder === "trash" && email.previous_folder === "sent")
      ? email.recipient
      : email.sender;
  }

  function renderReader() {
    const email = selectedEmail();
    if (!email) {
      reader.hidden = true;
      readerEmpty.hidden = false;
      workspace.classList.remove("has-selected-message");
      return;
    }

    workspace.classList.add("has-selected-message");
    readerEmpty.hidden = true;
    reader.hidden = false;
    document.querySelector("#message-subject").textContent = email.subject || "(no subject)";
    document.querySelector("#message-from").textContent = email.sender;
    document.querySelector("#message-to").textContent = email.recipient;
    document.querySelector("#message-date").textContent = dateText(email.timestamp);
    document.querySelector("#message-body").textContent = email.body;

    const actions = document.querySelector("#message-actions");
    actions.replaceChildren();
    actions.append(actionButton("Reply", () => replyTo(email)));
  }

  function replyTo(email) {
    const isOwnMessage = email.sender.toLowerCase() === mailbox;
    const recipient = isOwnMessage ? email.recipient : email.sender;
    const subject = email.subject.trim();
    composeForm.elements.to.value = recipient;
    composeForm.elements.subject.value = /^re:/i.test(subject) ? subject : `Re: ${subject}`;
    composeForm.elements.body.value = `\n\nOn ${dateText(email.timestamp)}, ${email.sender} wrote:\n> ${email.body.replace(/\r\n?/g, "\n").split("\n").join("\n> ")}`;
    composeError.textContent = "";
    composeModal.showModal();
    composeForm.elements.body.focus();
  }

  function actionButton(label, callback, danger = false) {
    const button = document.createElement("button");
    button.className = `email-action${danger ? " is-danger" : ""}`;
    button.type = "button";
    button.textContent = label;
    button.addEventListener("click", callback);
    return button;
  }

  function renderList() {
    const query = search.value.trim().toLowerCase();
    const emails = state.emails.filter((email) => {
      const correctOwner = state.folder === "sent"
        ? email.sender.toLowerCase() === mailbox
        : state.folder === "trash"
          ? email.previous_folder === "sent"
            ? email.sender.toLowerCase() === mailbox
            : email.recipient.toLowerCase() === mailbox
          : email.recipient.toLowerCase() === mailbox;
      const matchesSearch = !query || [email.sender, email.recipient, email.subject, email.body]
        .some((value) => value.toLowerCase().includes(query));
      return correctOwner && matchesSearch;
    });
    list.replaceChildren();

    if (!emails.length) {
      const empty = document.createElement("p");
      empty.className = "email-state";
      empty.textContent = query ? "No matching messages." : `No messages in ${titles[state.folder].toLowerCase()}.`;
      list.append(empty);
      if (!emails.some((email) => email.id === state.selectedId)) {
        state.selectedId = null;
        renderReader();
      }
      return;
    }

    for (const email of emails) {
      const row = document.createElement("button");
      row.className = `email-row${email.read ? "" : " unread"}${email.id === state.selectedId ? " is-selected" : ""}`;
      row.type = "button";
      row.setAttribute("role", "listitem");

      const sender = document.createElement("span");
      sender.className = "email-row-sender";
      sender.textContent = emailOwner(email);
      const date = document.createElement("time");
      date.className = "email-row-date";
      date.dateTime = email.timestamp;
      date.textContent = dateText(email.timestamp);
      const subject = document.createElement("span");
      subject.className = "email-row-subject";
      subject.textContent = email.subject || "(no subject)";
      const snippet = document.createElement("span");
      snippet.className = "email-row-snippet";
      snippet.textContent = email.body.replace(/\s+/g, " ").trim();
      row.append(sender, date, subject, snippet);
      row.addEventListener("click", async () => {
        state.selectedId = email.id;
        renderList();
        renderReader();
        if (window.matchMedia("(max-width: 760px)").matches) {
          readerPanel.scrollIntoView({ block: "start" });
        }
        if (!email.read) {
          try {
            await request("/api/emails/read", { method: "POST", body: JSON.stringify({ id: email.id, read: true }) });
            email.read = true;
            renderList();
          } catch (error) {
            showNotice(error.message);
          }
        }
      });
      list.append(row);
    }
  }

  async function request(url, options = {}) {
    const response = await fetch(url, {
      credentials: "same-origin",
      ...options,
      headers: { ...(options.body ? { "Content-Type": "application/json" } : {}), ...options.headers },
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(detail || `Request failed (${response.status})`);
    }
    return response.status === 204 ? null : response.json();
  }

  async function loadMailbox() {
    if (state.loading) return;
    state.loading = true;
    try {
      const emails = await request(`/api/emails?folder=${encodeURIComponent(state.folder)}`);
      state.emails = emails;
      folderTitle.textContent = titles[state.folder];
      renderList();
      renderReader();
      showNotice("");
    } catch (error) {
      list.replaceChildren();
      const failure = document.createElement("p");
      failure.className = "email-state";
      failure.textContent = `Could not load mail: ${error.message}`;
      list.append(failure);
      showNotice("");
    } finally {
      state.loading = false;
    }
  }

  async function mutate(endpoint, payload) {
    try {
      await request(endpoint, { method: "POST", body: JSON.stringify(payload) });
      state.selectedId = null;
      await loadMailbox();
      showNotice("Mailbox updated.");
    } catch (error) {
      showNotice(error.message);
    }
  }

  document.querySelectorAll("[data-folder]").forEach((button) => {
    button.addEventListener("click", () => {
      state.folder = button.dataset.folder;
      state.selectedId = null;
      workspace.classList.remove("has-selected-message");
      document.querySelectorAll("[data-folder]").forEach((item) => {
        const active = item === button;
        item.classList.toggle("is-active", active);
        if (active) item.setAttribute("aria-current", "page");
        else item.removeAttribute("aria-current");
      });
      loadMailbox();
    });
  });

  document.querySelector("#refresh-email").addEventListener("click", loadMailbox);
  document.querySelector("#back-to-messages").addEventListener("click", () => {
    workspace.classList.remove("has-selected-message");
    state.selectedId = null;
    renderList();
    renderReader();
  });
  search.addEventListener("input", renderList);
  document.querySelector("#open-compose").addEventListener("click", () => {
    composeError.textContent = "";
    composeModal.showModal();
    composeForm.elements.to.focus();
  });
  document.querySelector("#close-compose").addEventListener("click", () => composeModal.close());
  composeModal.addEventListener("click", (event) => {
    if (event.target === composeModal) composeModal.close();
  });

  composeForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    composeError.textContent = "";
    const payload = Object.fromEntries(new FormData(composeForm).entries());
    try {
      await request("/api/send", { method: "POST", body: JSON.stringify(payload) });
      composeForm.reset();
      composeModal.close();
      showNotice("Email sent.");
      state.folder = "sent";
      state.selectedId = null;
      document.querySelector('[data-folder="sent"]').click();
    } catch (error) {
      composeError.textContent = error.message;
    }
  });

  loadMailbox();
  window.setInterval(loadMailbox, 10000);
})();
