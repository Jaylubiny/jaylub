(() => {
  "use strict";

  const audio = document.getElementById("audio-player");
  const sidebar = document.getElementById("sidebar");
  const menuToggle = document.getElementById("menu-toggle");
  const menuBackdrop = document.getElementById("menu-backdrop");
  const mobileNavigation = window.matchMedia("(max-width: 800px)");
  const state = {
    songs: [],
    favorites: [],
    queue: [],
    queueIndex: -1,
    activeTab: "global",
    toastTimer: null,
    menuOpen: !mobileNavigation.matches,
    installPrompt: null
  };

  const byId = (id) => document.getElementById(id);
  const isEditableTarget = (target) => {
    const tagName = target.tagName;
    return ["INPUT", "TEXTAREA", "SELECT", "BUTTON", "A"].includes(tagName)
      || target.isContentEditable;
  };
  const formatTime = (seconds) => {
    if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
    const whole = Math.floor(seconds);
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, "0")}`;
  };

  function showToast(message) {
    const toast = byId("toast");
    toast.textContent = message;
    toast.classList.add("show");
    window.clearTimeout(state.toastTimer);
    state.toastTimer = window.setTimeout(() => toast.classList.remove("show"), 3200);
  }

  function togglePlayback() {
    if (audio.paused) {
      if (!audio.src) return;
      audio.play().catch((error) => showToast(error.message));
    } else {
      audio.pause();
    }
  }

  async function api(path, options = {}) {
    const response = await fetch(path, {
      credentials: "same-origin",
      ...options,
      headers: {
        ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
        ...options.headers
      }
    });
    if (!response.ok) {
      let message = `Request failed (${response.status})`;
      try {
        const payload = await response.json();
        if (payload.error) message = payload.error;
      } catch (_) { /* Keep the status-based message for non-JSON errors. */ }
      throw new Error(message);
    }
    return response.status === 204 ? null : response.json();
  }

  function createSongRow(song) {
    const row = document.createElement("article");
    row.className = "song-row";

    const art = document.createElement("span");
    art.className = "song-art";
    art.setAttribute("aria-hidden", "true");
    art.textContent = "♫";

    const titleCell = document.createElement("div");
    const title = document.createElement("div");
    title.className = "song-title";
    title.textContent = song.title;
    const id = document.createElement("div");
    id.className = "song-subtitle song-id";
    id.textContent = `ID: ${song.id}`;
    titleCell.append(title, id);

    const duration = document.createElement("div");
    duration.className = "song-cell duration";
    duration.textContent = formatTime(song.duration_seconds);

    const actions = document.createElement("div");
    actions.className = "song-actions";
    const play = document.createElement("button");
    play.className = "row-button";
    play.type = "button";
    play.textContent = "▶";
    play.setAttribute("aria-label", `Play ${song.title}`);
    play.addEventListener("click", () => {
      const list = state.activeTab === "favorites" ? state.favorites : state.songs;
      playQueue(list, song.id);
    });

    const favorite = document.createElement("button");
    favorite.className = `row-button${song.is_favorite ? " favorite" : ""}`;
    favorite.type = "button";
    favorite.textContent = song.is_favorite ? "★" : "☆";
    favorite.setAttribute("aria-label", song.is_favorite ? `Remove ${song.title} from favorites` : `Add ${song.title} to favorites`);
    favorite.setAttribute("aria-pressed", String(song.is_favorite));
    favorite.addEventListener("click", () => toggleFavorite(song));
    actions.append(play, favorite);
    row.append(art, titleCell, duration, actions);
    return row;
  }

  function renderList(container, songs, emptyMessage) {
    container.replaceChildren();
    if (!songs.length) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = emptyMessage;
      container.append(empty);
      return;
    }
    songs.forEach((song) => container.append(createSongRow(song)));
  }

  async function loadSongs() {
    const [all, favorites] = await Promise.all([
      api("/api/songs"),
      api("/api/favorites")
    ]);
    state.songs = all.songs;
    state.favorites = favorites.songs;
    renderList(byId("global-list"), state.songs, "No tracks yet. Add the first MP3 from Upload music.");
    renderList(byId("favorites-list"), state.favorites, "Your favorites will appear here.");
  }

  async function toggleFavorite(song) {
    try {
      if (song.is_favorite) {
        await api(`/api/favorites/${encodeURIComponent(song.id)}`, { method: "DELETE" });
      } else {
        await api("/api/favorites", { method: "POST", body: JSON.stringify({ song_id: song.id }) });
      }
      await loadSongs();
    } catch (error) {
      showToast(error.message);
    }
  }

  function updateNowPlaying(song) {
    byId("now-title").textContent = song.title;
    byId("now-id").textContent = `ID: ${song.id}`;
    if ("mediaSession" in navigator && "MediaMetadata" in window) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: song.title
      });
    }
  }

  function playQueue(songs, selectedId = null) {
    state.queue = songs.slice();
    state.queueIndex = selectedId
      ? state.queue.findIndex((song) => song.id === selectedId)
      : 0;
    if (state.queueIndex < 0) return;
    playCurrent();
  }

  function playCurrent() {
    const song = state.queue[state.queueIndex];
    if (!song) return;
    audio.src = `/api/stream?id=${encodeURIComponent(song.id)}`;
    updateNowPlaying(song);
    audio.play().catch((error) => showToast(`Could not play this track: ${error.message}`));
    byId("play-toggle").textContent = "❚❚";
    byId("play-toggle").setAttribute("aria-label", "Pause");
  }

  function moveQueue(offset) {
    if (!state.queue.length) return;
    const nextIndex = state.queueIndex + offset;
    if (nextIndex < 0 || nextIndex >= state.queue.length) return;
    state.queueIndex = nextIndex;
    playCurrent();
  }

  function setMenuOpen(open) {
    state.menuOpen = open;
    sidebar.classList.toggle("open", open);
    sidebar.classList.toggle("collapsed", !open);
    menuToggle.setAttribute("aria-expanded", String(open));
    menuToggle.setAttribute("aria-label", open ? "Close navigation" : "Open navigation");
    menuBackdrop.hidden = !open || !mobileNavigation.matches;
  }

  function switchTab(tabName) {
    if (!["global", "favorites", "upload"].includes(tabName)) return;
    state.activeTab = tabName;
    document.querySelectorAll(".tab-button").forEach((button) => {
      const active = button.dataset.tab === tabName;
      button.classList.toggle("active", active);
      if (active) button.setAttribute("aria-current", "page");
      else button.removeAttribute("aria-current");
    });
    document.querySelectorAll(".view").forEach((view) => {
      view.hidden = view.id !== `view-${tabName}`;
    });
    if (mobileNavigation.matches) setMenuOpen(false);
  }

  function upload(file) {
    return new Promise((resolve, reject) => {
      const request = new XMLHttpRequest();
      const formData = new FormData();
      formData.append("file", file);
      request.open("POST", "/api/upload");
      request.withCredentials = true;
      request.upload.addEventListener("progress", (event) => {
        if (!event.lengthComputable) return;
        byId("upload-progress").value = Math.round(event.loaded / event.total * 100);
      });
      request.addEventListener("load", () => {
        let response;
        try { response = JSON.parse(request.responseText); }
        catch (_) { response = {}; }
        if (request.status >= 200 && request.status < 300) resolve(response);
        else reject(new Error(response.error || `Upload failed (${request.status})`));
      });
      request.addEventListener("error", () => reject(new Error("Network error while uploading.")));
      request.send(formData);
    });
  }

  function registerMediaSession() {
    if (!("mediaSession" in navigator)) return;
    navigator.mediaSession.setActionHandler("play", () => audio.play());
    navigator.mediaSession.setActionHandler("pause", () => audio.pause());
    navigator.mediaSession.setActionHandler("previoustrack", () => moveQueue(-1));
    navigator.mediaSession.setActionHandler("nexttrack", () => moveQueue(1));
    navigator.mediaSession.setActionHandler("seekto", (details) => {
      if (Number.isFinite(details.seekTime)) audio.currentTime = details.seekTime;
    });
  }

  function bindEvents() {
    const installButton = byId("install-app");
    window.addEventListener("beforeinstallprompt", (event) => {
      event.preventDefault();
      state.installPrompt = event;
      installButton.hidden = false;
    });
    window.addEventListener("appinstalled", () => {
      state.installPrompt = null;
      installButton.hidden = true;
    });
    installButton.addEventListener("click", async () => {
      if (!state.installPrompt) return;
      state.installPrompt.prompt();
      const choice = await state.installPrompt.userChoice;
      state.installPrompt = null;
      installButton.hidden = true;
      if (choice.outcome === "accepted") showToast("Jaylub Music installed.");
    });
    if ("serviceWorker" in navigator) {
      navigator.serviceWorker.register("/sw.js", { scope: "/" })
        .catch((error) => console.error("Could not register music offline app:", error));
    }
    const userMenu = document.querySelector("[data-user-menu]");
    const userMenuButton = document.querySelector("[data-user-menu-button]");
    const userDropdown = document.querySelector("[data-user-dropdown]");
    if (userMenu && userMenuButton && userDropdown) {
      userMenuButton.addEventListener("click", () => {
        const open = userDropdown.hidden;
        userDropdown.hidden = !open;
        userMenuButton.setAttribute("aria-expanded", String(open));
      });
      document.addEventListener("click", (event) => {
        if (!userMenu.contains(event.target)) {
          userDropdown.hidden = true;
          userMenuButton.setAttribute("aria-expanded", "false");
        }
      });
      document.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && !userDropdown.hidden) {
          userDropdown.hidden = true;
          userMenuButton.setAttribute("aria-expanded", "false");
          userMenuButton.focus();
        }
      });
    }
    document.querySelectorAll(".tab-button").forEach((button) => {
      button.addEventListener("click", () => switchTab(button.dataset.tab));
    });
    document.querySelector("[data-refresh]").addEventListener("click", () => {
      loadSongs().catch((error) => showToast(error.message));
    });
    setMenuOpen(state.menuOpen);
    menuToggle.addEventListener("click", () => setMenuOpen(!state.menuOpen));
    menuBackdrop.addEventListener("click", () => setMenuOpen(false));
    mobileNavigation.addEventListener("change", (event) => setMenuOpen(!event.matches));
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && state.menuOpen && mobileNavigation.matches) {
        setMenuOpen(false);
        menuToggle.focus();
        return;
      }
      if ((event.code !== "Space" && event.key !== " ") ||
          event.defaultPrevented ||
          event.altKey ||
          event.ctrlKey ||
          event.shiftKey ||
          event.metaKey ||
          isEditableTarget(event.target)) {
        return;
      }
      event.preventDefault();
      if (event.repeat) return;
      togglePlayback();
    });
    byId("play-playlist").addEventListener("click", () => {
      if (!state.favorites.length) return showToast("Add some favorites to build a playlist.");
      playQueue(state.favorites);
    });
    byId("upload-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const file = byId("audio-file").files[0];
      const message = byId("upload-message");
      const progress = byId("upload-progress");
      if (!file) return;
      if (!file.name.toLowerCase().endsWith(".mp3")) {
        message.textContent = "Choose an MP3 file.";
        return;
      }
      if (file.size > 100 * 1024 * 1024) {
        message.textContent = "This file is larger than the 100 MB limit.";
        return;
      }
      progress.hidden = false;
      progress.value = 0;
      byId("upload-submit").disabled = true;
      message.textContent = "Uploading and reading track information…";
      try {
        await upload(file);
        message.textContent = "Track uploaded successfully.";
        byId("upload-form").reset();
        await loadSongs();
      } catch (error) {
        message.textContent = error.message;
      } finally {
        byId("upload-submit").disabled = false;
      }
    });
    byId("play-toggle").addEventListener("click", togglePlayback);
    byId("previous-track").addEventListener("click", () => moveQueue(-1));
    byId("next-track").addEventListener("click", () => moveQueue(1));
    byId("seek-bar").addEventListener("input", (event) => {
      if (Number.isFinite(audio.duration)) audio.currentTime = Number(event.target.value) / 1000 * audio.duration;
    });
    byId("volume").addEventListener("input", (event) => { audio.volume = Number(event.target.value); });
    audio.volume = Number(byId("volume").value);
    audio.addEventListener("play", () => {
      byId("play-toggle").textContent = "❚❚";
      byId("play-toggle").setAttribute("aria-label", "Pause");
      if ("mediaSession" in navigator) navigator.mediaSession.playbackState = "playing";
    });
    audio.addEventListener("pause", () => {
      byId("play-toggle").textContent = "▶";
      byId("play-toggle").setAttribute("aria-label", "Play");
      if ("mediaSession" in navigator) navigator.mediaSession.playbackState = "paused";
    });
    audio.addEventListener("timeupdate", () => {
      byId("current-time").textContent = formatTime(audio.currentTime);
      byId("seek-bar").value = Number.isFinite(audio.duration) && audio.duration > 0
        ? String(Math.round(audio.currentTime / audio.duration * 1000))
        : "0";
    });
    audio.addEventListener("loadedmetadata", () => { byId("total-time").textContent = formatTime(audio.duration); });
    audio.addEventListener("ended", () => moveQueue(1));
    audio.addEventListener("error", () => showToast("This track could not be loaded."));
    registerMediaSession();
  }

  bindEvents();
  loadSongs().catch((error) => showToast(error.message));
})();
