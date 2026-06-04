(() => {
  const canvas = document.getElementById("platformerCanvas");
  const ctx = canvas.getContext("2d");
  const prompt = document.getElementById("notePrompt");
  const modal = document.getElementById("noteModal");
  const noteText = document.getElementById("noteText");
  const closeButton = document.getElementById("noteClose");
  const character = new Image();
  character.src = "/static/img/character.png";

  const world = { width: 2800, height: 900 };
  const gravity = 0.72;
  const keys = { left: false, right: false, up: false };
  let cameraX = 0;
  let cameraY = 0;
  let activeNote = null;
  let modalOpen = false;

  const player = {
    x: 90,
    y: 580,
    width: 52,
    height: 58,
    vx: 0,
    vy: 0,
    speed: 5.2,
    jump: 16,
    grounded: false,
    facing: 1
  };

  const platforms = [
    { x: 0, y: 820, w: 470, h: 80, kind: "ground" },
    { x: 450, y: 780, w: 360, h: 120, kind: "ground" },
    { x: 860, y: 820, w: 500, h: 80, kind: "ground" },
    { x: 1510, y: 760, w: 440, h: 140, kind: "ground" },
    { x: 2060, y: 810, w: 740, h: 90, kind: "ground" },
    { x: 260, y: 640, w: 180, h: 24, kind: "stone" },
    { x: 570, y: 560, w: 180, h: 24, kind: "stone" },
    { x: 900, y: 650, w: 210, h: 24, kind: "stone" },
    { x: 1220, y: 530, w: 190, h: 24, kind: "stone" },
    { x: 1540, y: 610, w: 180, h: 24, kind: "stone" },
    { x: 1810, y: 470, w: 160, h: 24, kind: "stone" },
    { x: 2120, y: 630, w: 220, h: 24, kind: "stone" },
    { x: 2450, y: 555, w: 180, h: 24, kind: "stone" }
  ];

  const hazards = [
    { x: 810, y: 845, w: 70, h: 55 },
    { x: 1360, y: 845, w: 150, h: 55 },
    { x: 1950, y: 840, w: 110, h: 60 }
  ];

  const notes = [
    { x: 330, y: 584, message: "Jaylub was here" },
    { x: 635, y: 504, message: "Jaylub is stinking too much today" },
    { x: 1300, y: 474, message: "Jaylub is not pdf file" },
    { x: 1880, y: 414, message: "Jaylub is our hero" },
    { x: 2525, y: 499, message: "This game is WIP (Work In Progress) so dw" }
  ];

  function resizeCanvas() {
    const wrap = canvas.parentElement;
    const rect = wrap.getBoundingClientRect();
    canvas.width = Math.max(640, Math.floor(rect.width * window.devicePixelRatio));
    canvas.height = Math.max(360, Math.floor(rect.height * window.devicePixelRatio));
    ctx.setTransform(window.devicePixelRatio, 0, 0, window.devicePixelRatio, 0, 0);
    ctx.imageSmoothingEnabled = false;
  }

  function viewWidth() {
    return canvas.width / window.devicePixelRatio;
  }

  function viewHeight() {
    return canvas.height / window.devicePixelRatio;
  }

  function intersects(a, b) {
    return a.x < b.x + b.w && a.x + a.width > b.x && a.y < b.y + b.h && a.y + a.height > b.y;
  }

  function resetPlayer() {
    player.x = 90;
    player.y = 580;
    player.vx = 0;
    player.vy = 0;
  }

  function updatePlayer() {
    player.vx *= 0.82;
    if (keys.left) {
      player.vx = -player.speed;
      player.facing = -1;
    }
    if (keys.right) {
      player.vx = player.speed;
      player.facing = 1;
    }
    if (keys.up && player.grounded) {
      player.vy = -player.jump;
      player.grounded = false;
    }

    player.x += player.vx;
    for (const platform of platforms) {
      if (!intersects(player, platform)) continue;
      if (player.vx > 0) player.x = platform.x - player.width;
      if (player.vx < 0) player.x = platform.x + platform.w;
      player.vx = 0;
    }

    player.vy += gravity;
    player.y += player.vy;
    player.grounded = false;

    for (const platform of platforms) {
      if (!intersects(player, platform)) continue;
      if (player.vy > 0) {
        player.y = platform.y - player.height;
        player.vy = 0;
        player.grounded = true;
      } else if (player.vy < 0) {
        player.y = platform.y + platform.h;
        player.vy = 0;
      }
    }

    player.x = Math.max(0, Math.min(world.width - player.width, player.x));
    if (player.y > world.height + 160 || hazards.some((hazard) => intersects(player, hazard))) {
      resetPlayer();
    }
  }

  function updateCamera() {
    cameraX = player.x + player.width / 2 - viewWidth() / 2;
    cameraY = player.y + player.height / 2 - viewHeight() / 2;
    cameraX = Math.max(0, Math.min(world.width - viewWidth(), cameraX));
    cameraY = Math.max(0, Math.min(world.height - viewHeight(), cameraY));
  }

  function updateNotes() {
    activeNote = null;
    for (const note of notes) {
      const dx = player.x + player.width / 2 - note.x;
      const dy = player.y + player.height / 2 - note.y;
      if (Math.hypot(dx, dy) < 95) {
        activeNote = note;
        break;
      }
    }
    prompt.classList.toggle("is-visible", Boolean(activeNote) && !modalOpen);
  }

  function drawBackground() {
    const w = viewWidth();
    const h = viewHeight();
    ctx.fillStyle = "#77c7ef";
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = "#9be3ff";
    ctx.fillRect(0, 0, w, Math.floor(h * 0.45));
    ctx.fillStyle = "#f7d884";
    ctx.fillRect(0, Math.floor(h * 0.72), w, Math.ceil(h * 0.28));

    ctx.fillStyle = "#fff8d7";
    drawCloud(180 - cameraX * 0.2, 95 - cameraY * 0.08);
    drawCloud(760 - cameraX * 0.2, 145 - cameraY * 0.08);
    drawCloud(1420 - cameraX * 0.2, 80 - cameraY * 0.08);

    drawHills("#438a80", 0.18, 640);
    drawHills("#27576a", 0.32, 730);
  }

  function drawCloud(x, y) {
    const px = Math.round(x / 8) * 8;
    const py = Math.round(y / 8) * 8;
    ctx.fillRect(px, py, 32, 16);
    ctx.fillRect(px + 24, py - 16, 48, 32);
    ctx.fillRect(px + 64, py, 40, 16);
    ctx.fillRect(px + 16, py + 16, 72, 16);
  }

  function drawHills(color, parallax, baseY) {
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.moveTo(-80, viewHeight());
    for (let x = -120; x < viewWidth() + 180; x += 80) {
      const worldX = x + cameraX * parallax;
      const y = Math.round((baseY - cameraY * 0.22 + Math.sin(worldX * 0.006) * 40) / 16) * 16;
      ctx.lineTo(x, y);
      ctx.lineTo(x + 40, y - 48);
      ctx.lineTo(x + 80, y);
    }
    ctx.lineTo(viewWidth() + 160, viewHeight());
    ctx.closePath();
    ctx.fill();
  }

  function drawPlatforms() {
    for (const platform of platforms) {
      const x = Math.round(platform.x - cameraX);
      const y = Math.round(platform.y - cameraY);
      const topColor = platform.kind === "ground" ? "#74b15a" : "#b99a68";
      const bodyColor = platform.kind === "ground" ? "#3f7040" : "#75614d";
      const shadowColor = platform.kind === "ground" ? "#25472d" : "#4c3f35";

      ctx.fillStyle = "#172033";
      ctx.fillRect(x - 4, y - 4, platform.w + 8, platform.h + 8);
      ctx.fillStyle = bodyColor;
      ctx.fillRect(x, y, platform.w, platform.h);
      ctx.fillStyle = topColor;
      ctx.fillRect(x, y, platform.w, 16);
      ctx.fillStyle = shadowColor;
      ctx.fillRect(x, y + platform.h - 12, platform.w, 12);

      for (let tileX = 0; tileX < platform.w; tileX += 32) {
        const tileW = Math.min(32, platform.w - tileX);
        if (tileW <= 0) continue;

        ctx.strokeStyle = shadowColor;
        ctx.lineWidth = 2;
        ctx.strokeRect(x + tileX, y, tileW, Math.min(platform.h, 32));
        ctx.fillStyle = "rgba(255, 255, 255, 0.12)";
        ctx.fillRect(x + tileX + 6, y + 6, Math.min(8, Math.max(0, tileW - 8)), 4);
      }
    }
  }

  function drawHazards() {
    for (const hazard of hazards) {
      const x = Math.round(hazard.x - cameraX);
      const y = Math.round(hazard.y - cameraY);
      for (let i = 0; i < hazard.w; i += 28) {
        ctx.fillStyle = "#172033";
        ctx.beginPath();
        ctx.moveTo(x + i, y + hazard.h);
        ctx.lineTo(x + i + 14, y + 5);
        ctx.lineTo(x + i + 28, y + hazard.h);
        ctx.closePath();
        ctx.fill();
        ctx.fillStyle = "#b4233a";
        ctx.beginPath();
        ctx.moveTo(x + i + 5, y + hazard.h);
        ctx.lineTo(x + i + 14, y + 16);
        ctx.lineTo(x + i + 23, y + hazard.h);
        ctx.closePath();
        ctx.fill();
      }
    }
  }

  function drawNotes() {
    for (const note of notes) {
      const x = Math.round(note.x - cameraX);
      const y = Math.round(note.y - cameraY);
      ctx.save();
      ctx.translate(x, y);
      ctx.translate(0, Math.round(Math.sin(performance.now() / 300 + note.x) * 4));
      ctx.fillStyle = "#172033";
      ctx.fillRect(-20, -26, 40, 52);
      ctx.fillStyle = "#fff7c2";
      ctx.fillRect(-16, -22, 32, 44);
      ctx.fillStyle = "#facc15";
      ctx.fillRect(-16, -22, 32, 8);
      ctx.fillStyle = "#c99638";
      ctx.fillRect(-10, -10, 20, 3);
      ctx.fillRect(-10, -2, 17, 3);
      ctx.fillRect(-10, 6, 22, 3);
      ctx.fillStyle = "#f59e0b";
      ctx.fillRect(-8, -36, 16, 12);
      ctx.fillStyle = "#fde68a";
      ctx.fillRect(-4, -32, 8, 4);
      ctx.restore();
    }
  }

  function drawPlayer() {
    const x = player.x - cameraX;
    const y = player.y - cameraY;
    ctx.save();
    ctx.imageSmoothingEnabled = false;
    ctx.translate(Math.round(x + player.width / 2), Math.round(y + player.height / 2));
    ctx.scale(player.facing, 1);
    ctx.fillStyle = "rgba(15, 23, 42, 0.35)";
    ctx.fillRect(-22, 28, 44, 8);
    if (character.complete && character.naturalWidth) {
      ctx.drawImage(character, -player.width / 2, -player.height / 2, player.width, player.height);
    } else {
      ctx.fillStyle = "#f97316";
      ctx.fillRect(-player.width / 2, -player.height / 2, player.width, player.height);
    }
    ctx.restore();
  }

  function render() {
    ctx.clearRect(0, 0, viewWidth(), viewHeight());
    drawBackground();
    drawNotes();
    drawHazards();
    drawPlatforms();
    drawPlayer();
  }

  function loop() {
    if (!modalOpen) {
      updatePlayer();
      updateCamera();
      updateNotes();
    }
    render();
    requestAnimationFrame(loop);
  }

  function openNote() {
    if (!activeNote) return;
    modalOpen = true;
    noteText.textContent = activeNote.message;
    modal.classList.add("is-open");
    prompt.classList.remove("is-visible");
    closeButton.focus();
  }

  function closeNote() {
    modalOpen = false;
    modal.classList.remove("is-open");
    canvas.focus();
  }

  window.addEventListener("keydown", (event) => {
    if (event.key === "ArrowLeft") keys.left = true;
    if (event.key === "ArrowRight") keys.right = true;
    if (event.key === "ArrowUp") keys.up = true;
    if (["ArrowLeft", "ArrowRight", "ArrowUp"].includes(event.key)) event.preventDefault();
    if (event.key === "Enter") {
      event.preventDefault();
      if (modalOpen) closeNote();
      else openNote();
    }
    if (event.key === "Escape" && modalOpen) closeNote();
  });

  window.addEventListener("keyup", (event) => {
    if (event.key === "ArrowLeft") keys.left = false;
    if (event.key === "ArrowRight") keys.right = false;
    if (event.key === "ArrowUp") keys.up = false;
  });

  closeButton.addEventListener("click", closeNote);
  modal.addEventListener("click", (event) => {
    if (event.target === modal) closeNote();
  });
  window.addEventListener("resize", resizeCanvas);

  resizeCanvas();
  loop();
})();
