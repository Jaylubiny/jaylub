(() => {
  "use strict";

  const maxBatchFiles = 25;
  const maxUploadBytes = 100 * 1024 * 1024;
  const audio = document.getElementById("audio-player");
  const sidebar = document.getElementById("sidebar");
  const menuToggle = document.getElementById("menu-toggle");
  const menuBackdrop = document.getElementById("menu-backdrop");
  const mobileNavigation = window.matchMedia("(max-width: 800px)");
  const state = {
    songs: [],
    favorites: [],
    playlists: [],
    currentPlaylist: null,
    pendingPlaylistSong: null,
    globalSearch: "",
    queue: [],
    queueIndex: -1,
    currentSong: null,
    playbackGeneration: 0,
    endedGeneration: -1,
    failedGeneration: -1,
    activeTab: "global",
    toastTimer: null,
    menuOpen: !mobileNavigation.matches
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

  function createSongRow(song, playlistId = null) {
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
      const list = state.activeTab === "favorites"
        ? state.favorites
        : state.activeTab === "playlists" && state.currentPlaylist
          ? state.currentPlaylist.songs
          : state.songs;
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

    if (playlistId !== null) {
      const remove = document.createElement("button");
      remove.className = "row-button";
      remove.type = "button";
      remove.textContent = "−";
      remove.setAttribute("aria-label", `Remove ${song.title} from this playlist`);
      remove.addEventListener("click", () => removeSongFromPlaylist(playlistId, song));
      actions.append(remove);
    } else {
      const addToPlaylist = document.createElement("button");
      addToPlaylist.className = "row-button";
      addToPlaylist.type = "button";
      addToPlaylist.textContent = "+";
      addToPlaylist.setAttribute("aria-label", `Add ${song.title} to a playlist`);
      addToPlaylist.addEventListener("click", () => openPlaylistDialog(song));
      actions.append(addToPlaylist);
    }
    row.append(art, titleCell, duration, actions);
    return row;
  }

  function renderList(container, songs, emptyMessage, playlistId = null) {
    container.replaceChildren();
    if (!songs.length) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = emptyMessage;
      container.append(empty);
      return;
    }
    songs.forEach((song) => container.append(createSongRow(song, playlistId)));
  }

  async function loadSongs() {
    const [all, favorites, playlists] = await Promise.all([
      api("/api/songs"),
      api("/api/favorites"),
      api("/api/playlists")
    ]);
    state.songs = all.songs;
    state.favorites = favorites.songs;
    state.playlists = playlists.playlists;
    renderGlobalLibrary();
    renderList(byId("favorites-list"), state.favorites, "Your favorites will appear here.");
    renderPlaylists();
    if (state.currentPlaylist && state.playlists.some((playlist) => playlist.id === state.currentPlaylist.id)) {
      await loadPlaylist(state.currentPlaylist.id);
    } else {
      state.currentPlaylist = null;
      byId("playlist-detail").hidden = true;
    }
  }

  function renderGlobalLibrary() {
    const query = state.globalSearch.trim().toLowerCase();
    const songs = query
      ? state.songs.filter((song) =>
          [song.title, song.artist, song.album, song.id]
            .some((value) => value.toLowerCase().includes(query)))
      : state.songs;
    const emptyMessage = state.songs.length
      ? "No tracks match your search."
      : "No tracks yet. Add the first MP3 from Upload music.";
    renderList(byId("global-list"), songs, emptyMessage);
  }

  function renderPlaylists() {
    const container = byId("playlist-list");
    container.replaceChildren();
    if (!state.playlists.length) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = "You haven't made any playlists yet.";
      container.append(empty);
      return;
    }
    state.playlists.forEach((playlist) => {
      const button = document.createElement("button");
      button.className = `playlist-card${state.currentPlaylist?.id === playlist.id ? " active" : ""}`;
      button.type = "button";
      button.setAttribute("aria-pressed", String(state.currentPlaylist?.id === playlist.id));
      const name = document.createElement("strong");
      name.textContent = playlist.name;
      const count = document.createElement("span");
      count.textContent = `${playlist.song_count} ${playlist.song_count === 1 ? "track" : "tracks"}`;
      button.append(name, count);
      button.addEventListener("click", () => {
        showPlaylist(playlist.id).catch((error) => showToast(error.message));
      });
      container.append(button);
    });
  }

  async function showPlaylist(playlistId) {
    await loadPlaylist(playlistId);
    renderPlaylists();
  }

  async function loadPlaylist(playlistId) {
    const data = await api(`/api/playlists/${encodeURIComponent(playlistId)}`);
    state.currentPlaylist = { ...data.playlist, songs: data.songs };
    byId("playlist-detail-title").textContent = data.playlist.name;
    byId("playlist-detail").hidden = false;
    renderList(
      byId("playlist-song-list"),
      data.songs,
      "This playlist is empty. Add tracks with the + button in the library.",
      playlistId
    );
  }

  function openPlaylistDialog(song) {
    if (!state.playlists.length) {
      showToast("Create a playlist before adding tracks.");
      switchTab("playlists");
      byId("playlist-name").focus();
      return;
    }
    const select = byId("playlist-select");
    select.replaceChildren();
    state.playlists.forEach((playlist) => {
      const option = document.createElement("option");
      option.value = String(playlist.id);
      option.textContent = playlist.name;
      select.append(option);
    });
    state.pendingPlaylistSong = song;
    byId("playlist-song-title").textContent = song.title;
    byId("playlist-dialog").showModal();
  }

  async function removeSongFromPlaylist(playlistId, song) {
    try {
      await api(`/api/playlists/${encodeURIComponent(playlistId)}/songs/${encodeURIComponent(song.id)}`, {
        method: "DELETE"
      });
      await loadSongs();
    } catch (error) {
      showToast(error.message);
    }
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
    state.currentSong = song;
    byId("now-title").textContent = song.title;
    byId("now-id").textContent = `ID: ${song.id}`;
    if ("mediaSession" in navigator && "MediaMetadata" in window) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: song.title,
        artist: song.artist || "Unknown Artist",
        album: song.album || "Jaylub Music",
        artwork: [
          { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png" },
          { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png" }
        ]
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
    state.playbackGeneration++;
    state.endedGeneration = -1;
    state.failedGeneration = -1;
    audio.src = `/api/stream?id=${encodeURIComponent(song.id)}`;
    audio.load();
    updateNowPlaying(song);
    const generation = state.playbackGeneration;
    audio.play().catch((error) => {
      if (generation !== state.playbackGeneration) return;
      byId("play-toggle").textContent = "▶";
      byId("play-toggle").setAttribute("aria-label", "Play");
      if ("mediaSession" in navigator) navigator.mediaSession.playbackState = "paused";
      showToast(`Could not play this track: ${error.message}`);
    });
  }

  function moveQueue(offset) {
    if (!state.queue.length) return;
    const nextIndex = state.queueIndex + offset;
    if (nextIndex < 0 || nextIndex >= state.queue.length) return;
    state.queueIndex = nextIndex;
    playCurrent();
  }

  function advanceEndedTrack() {
    if (!audio.ended || state.endedGeneration === state.playbackGeneration) return;
    state.endedGeneration = state.playbackGeneration;
    moveQueue(1);
  }

  function handleTrackError() {
    if (state.failedGeneration === state.playbackGeneration) return;
    state.failedGeneration = state.playbackGeneration;
    const failedSong = state.queue[state.queueIndex];
    if (failedSong && state.queueIndex + 1 < state.queue.length) {
      showToast(`Could not load "${failedSong.title}". Skipping to the next track.`);
      moveQueue(1);
      return;
    }
    showToast(failedSong
      ? `Could not load "${failedSong.title}".`
      : "This track could not be loaded.");
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
    if (!["global", "favorites", "playlists", "upload"].includes(tabName)) return;
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

  function upload(file, onProgress) {
    return new Promise((resolve, reject) => {
      const request = new XMLHttpRequest();
      const formData = new FormData();
      formData.append("file", file);
      request.open("POST", "/api/upload");
      request.withCredentials = true;
      request.upload.addEventListener("progress", (event) => {
        if (event.lengthComputable) onProgress(event.loaded, event.total);
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
    navigator.mediaSession.setActionHandler("play", () => {
      audio.play().catch((error) => showToast(`Could not resume playback: ${error.message}`));
    });
    navigator.mediaSession.setActionHandler("pause", () => audio.pause());
    navigator.mediaSession.setActionHandler("previoustrack", () => moveQueue(-1));
    navigator.mediaSession.setActionHandler("nexttrack", () => moveQueue(1));
    navigator.mediaSession.setActionHandler("seekto", (details) => {
      if (Number.isFinite(details.seekTime)) audio.currentTime = details.seekTime;
    });
  }

  function bindEvents() {
    if ("serviceWorker" in navigator) {
      navigator.serviceWorker.register("/sw.js?v=music-recovery-1", { scope: "/", updateViaCache: "none" })
        .then((registration) => registration.update())
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
    byId("global-search").addEventListener("input", (event) => {
      state.globalSearch = event.target.value;
      renderGlobalLibrary();
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
    byId("playlist-create-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const nameInput = byId("playlist-name");
      try {
        const result = await api("/api/playlists", {
          method: "POST",
          body: JSON.stringify({ name: nameInput.value })
        });
        nameInput.value = "";
        state.currentPlaylist = { ...result.playlist, songs: [] };
        await loadSongs();
        byId("playlist-detail").scrollIntoView({ behavior: "smooth", block: "start" });
      } catch (error) {
        showToast(error.message);
      }
    });
    byId("playlist-song-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      if (!state.pendingPlaylistSong) return;
      const playlistId = byId("playlist-select").value;
      try {
        await api(`/api/playlists/${encodeURIComponent(playlistId)}/songs`, {
          method: "POST",
          body: JSON.stringify({ song_id: state.pendingPlaylistSong.id })
        });
        byId("playlist-dialog").close();
        state.pendingPlaylistSong = null;
        await loadSongs();
        showToast("Track added to playlist.");
      } catch (error) {
        showToast(error.message);
      }
    });
    byId("playlist-dialog-cancel").addEventListener("click", () => {
      byId("playlist-dialog").close();
      state.pendingPlaylistSong = null;
    });
    byId("playlist-dialog").addEventListener("close", () => {
      state.pendingPlaylistSong = null;
    });
    byId("play-user-playlist").addEventListener("click", () => {
      if (!state.currentPlaylist?.songs.length) return showToast("Add tracks to this playlist first.");
      playQueue(state.currentPlaylist.songs);
    });
    byId("delete-user-playlist").addEventListener("click", async () => {
      if (!state.currentPlaylist || !window.confirm(`Delete "${state.currentPlaylist.name}"?`)) return;
      try {
        await api(`/api/playlists/${encodeURIComponent(state.currentPlaylist.id)}`, { method: "DELETE" });
        state.currentPlaylist = null;
        await loadSongs();
      } catch (error) {
        showToast(error.message);
      }
    });
    byId("upload-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const files = Array.from(byId("audio-file").files);
      const message = byId("upload-message");
      const progress = byId("upload-progress");
      if (!files.length) return;
      if (files.length > maxBatchFiles) {
        message.textContent = `Choose no more than ${maxBatchFiles} files at a time.`;
        return;
      }
      const invalidFile = files.find((file) => !file.name.toLowerCase().endsWith(".mp3"));
      if (invalidFile) {
        message.textContent = `${invalidFile.name} is not an MP3 file.`;
        return;
      }
      const oversizedFile = files.find((file) => file.size > maxUploadBytes);
      if (oversizedFile) {
        message.textContent = `${oversizedFile.name} is larger than the 100 MB limit.`;
        return;
      }
      const totalBytes = files.reduce((total, file) => total + file.size, 0);
      let completedBytes = 0;
      let uploadedCount = 0;
      const failures = [];
      progress.hidden = false;
      progress.value = 0;
      byId("upload-submit").disabled = true;
      try {
        for (const [index, file] of files.entries()) {
          message.textContent = `Uploading ${index + 1} of ${files.length}: ${file.name}`;
          try {
            await upload(file, (loaded) => {
              progress.value = totalBytes === 0
                ? 0
                : Math.round((completedBytes + Math.min(loaded, file.size)) / totalBytes * 100);
            });
            uploadedCount++;
          } catch (error) {
            failures.push(`${file.name}: ${error.message}`);
          }
          completedBytes += file.size;
          progress.value = totalBytes === 0 ? 100 : Math.round(completedBytes / totalBytes * 100);
        }
        if (uploadedCount) {
          await loadSongs();
          byId("upload-form").reset();
        }
        message.textContent = failures.length
          ? `Uploaded ${uploadedCount} of ${files.length} files. Failed: ${failures.join("; ")}`
          : `Uploaded ${uploadedCount} ${uploadedCount === 1 ? "track" : "tracks"} successfully.`;
      } catch (error) {
        message.textContent = `Uploaded ${uploadedCount} of ${files.length} files, but the library could not refresh: ${error.message}`;
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
      if ("mediaSession" in navigator &&
          typeof navigator.mediaSession.setPositionState === "function" &&
          Number.isFinite(audio.duration) && audio.duration > 0 &&
          Number.isFinite(audio.currentTime)) {
        navigator.mediaSession.setPositionState({
          duration: audio.duration,
          playbackRate: audio.playbackRate,
          position: Math.min(audio.currentTime, audio.duration)
        });
      }
    });
    audio.addEventListener("loadedmetadata", () => { byId("total-time").textContent = formatTime(audio.duration); });
    audio.addEventListener("ended", advanceEndedTrack);
    audio.addEventListener("error", handleTrackError);
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "visible") advanceEndedTrack();
    });
    window.addEventListener("pageshow", advanceEndedTrack);
    registerMediaSession();
  }

  bindEvents();
  loadSongs().catch((error) => showToast(error.message));
})();
