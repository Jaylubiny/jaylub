(() => {
  const canvas = document.getElementById("survivorCanvas");
  const ctx = canvas.getContext("2d");
  const stage = document.querySelector(".survivor-stage");
  const hud = document.getElementById("gameHud");
  const fullscreenButtons = document.querySelectorAll("[data-fullscreen-button]");
  const screens = {
    menu: document.getElementById("mainMenu"),
    shop: document.getElementById("shopScreen"),
    leaderboard: document.getElementById("leaderboardScreen"),
    gameOver: document.getElementById("gameOverScreen"),
    pause: document.getElementById("pauseScreen"),
  };

  const ui = {
    hpFill: document.getElementById("hpFill"),
    hpText: document.getElementById("hpText"),
    runGold: document.getElementById("runGold"),
    runKills: document.getElementById("runKills"),
    runTime: document.getElementById("runTime"),
    finalTime: document.getElementById("finalTime"),
    finalKills: document.getElementById("finalKills"),
    finalGold: document.getElementById("finalGold"),
    characterList: document.getElementById("characterList"),
    upgradeList: document.getElementById("upgradeList"),
    leaderboardRows: document.getElementById("leaderboardRows"),
    leaderboardEyebrow: document.getElementById("leaderboardEyebrow"),
    leaderboardValueHeader: document.getElementById("leaderboardValueHeader"),
  };

  const playerImage = new Image();
  playerImage.src = "/static/img/character.png";
  const goblinImage = new Image();
  goblinImage.src = "/static/img/character2.png";
  const enemyImage = new Image();
  enemyImage.src = "/static/img/enemy.png";
  const enemy2Image = new Image();
  enemy2Image.src = "/static/img/enemy2.png";
  const healImage = new Image();
  healImage.src = "/static/img/heal.png";

  const keys = new Set();
  const mouse = { x: 0, y: 0, worldX: 0, worldY: 0 };
  const world = { width: 2600, height: 2000 };
  const camera = { x: 0, y: 0 };
  const rngDetails = makeEnvironment();
  const state = {
    mode: "menu",
    profile: null,
    shop: {},
    leaderboard: [],
    leaderboardSort: "totalKills",
    running: false,
    paused: false,
    pausedAt: 0,
    lastTime: performance.now(),
    spawnTimer: 0,
    healSpawnTimer: 0,
    hitPause: 0,
    shake: 0,
    dying: false,
    deathTimer: 0,
    submittedGameOver: false,
  };

  let player;
  let enemies = [];
  let goldDrops = [];
  let healDrops = [];
  let particles = [];
  let numbers = [];
  let slashes = [];
  let projectiles = [];
  let nextEnemyId = 1;

  function resizeCanvas() {
    const rect = stage.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.max(640, Math.floor(rect.width * dpr));
    canvas.height = Math.max(360, Math.floor(rect.height * dpr));
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = false;
  }

  function viewWidth() {
    return canvas.width / (window.devicePixelRatio || 1);
  }

  function viewHeight() {
    return canvas.height / (window.devicePixelRatio || 1);
  }

  function showScreen(name) {
    for (const [screenName, element] of Object.entries(screens)) {
      element.hidden = screenName !== name;
    }
    hud.hidden = name !== "game" && name !== "pause";
    state.mode = name;
  }

  async function loadState() {
    const response = await fetch("/game/jaylive/state", { headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error("Could not load game state.");
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    state.leaderboard = data.leaderboard || [];
    renderMeta();
  }

  function renderMeta() {
    document.querySelectorAll("[data-gold]").forEach((el) => {
      el.textContent = state.profile?.gold ?? 0;
    });
    document.querySelectorAll("[data-lifetime-kills]").forEach((el) => {
      el.textContent = state.profile?.lifetimeKills ?? 0;
    });
    renderCharacters();
    renderShop();
    renderLeaderboard();
  }

  function renderCharacters() {
    const profile = state.profile || {};
    const characters = [
      {
        id: "jaylub",
        name: "Jaylub",
        image: "/static/img/character.png",
        detail: "Auto melee slash",
        unlocked: true,
        cost: 0,
      },
      {
        id: "goblin_jaylub",
        name: "Goblin Jaylub",
        image: "/static/img/character2.png",
        detail: "Shoots projectiles",
        unlocked: Boolean(profile.goblinJaylubUnlocked),
        cost: 100,
      },
    ];

    ui.characterList.textContent = "";
    for (const character of characters) {
      const selected = profile.selectedCharacter === character.id;
      const slot = document.createElement("div");
      slot.className = "character-slot";
      slot.classList.toggle("active", selected);

      const image = document.createElement("img");
      image.src = character.image;
      image.alt = character.name;

      const copy = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = character.name;
      const detail = document.createElement("span");
      detail.textContent = selected ? "Selected" : character.detail;
      copy.append(title, detail);

      const button = document.createElement("button");
      button.className = "pixel-button small";
      button.type = "button";
      if (selected) {
        button.textContent = "Selected";
        button.disabled = true;
      } else if (character.unlocked) {
        button.textContent = "Select";
        button.addEventListener("click", () => setCharacter("select", character.id));
      } else {
        button.textContent = `${character.cost} gold`;
        button.disabled = (profile.gold || 0) < character.cost;
        button.addEventListener("click", () => setCharacter("buy", character.id));
      }

      slot.append(image, copy, button);
      ui.characterList.append(slot);
    }
  }

  function renderShop() {
    const upgrades = [
      ["damage", "Damage", "+2 damage per level"],
      ["maxHp", "Max HP", "+12 maximum HP per level"],
      ["attackSpeed", "Attack Speed", "Faster automatic attacks"],
      ["moveSpeed", "Move Speed", "Faster movement"],
    ];
    if (state.profile?.selectedCharacter === "goblin_jaylub") {
      upgrades.push(["piercing", "Piercing", "Goblin projectiles pass through more enemies"]);
    }
    ui.upgradeList.textContent = "";
    for (const [id, name, detail] of upgrades) {
      const meta = state.shop[id] || { level: 0, cost: 35 };
      const row = document.createElement("div");
      row.className = "upgrade-row";

      const copy = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = name;
      const sub = document.createElement("span");
      const maxText = meta.max ? ` / ${meta.max}` : "";
      sub.textContent = `Level ${meta.level}${maxText} - ${detail}`;
      copy.append(title, sub);

      const button = document.createElement("button");
      button.className = "pixel-button small";
      button.type = "button";
      const maxed = Boolean(meta.max && meta.level >= meta.max);
      button.textContent = maxed ? "Maxed" : `${meta.cost} gold`;
      button.disabled = maxed || (state.profile?.gold ?? 0) < meta.cost;
      button.addEventListener("click", () => buyUpgrade(id));

      row.append(copy, button);
      ui.upgradeList.append(row);
    }
  }

  function renderLeaderboard() {
    const meta = leaderboardMeta(state.leaderboardSort);
    ui.leaderboardEyebrow.textContent = meta.eyebrow;
    ui.leaderboardValueHeader.textContent = meta.label;
    document.querySelectorAll("[data-leaderboard-sort]").forEach((button) => {
      const active = button.dataset.leaderboardSort === state.leaderboardSort;
      button.classList.toggle("is-active", active);
      button.classList.toggle("muted", !active);
    });

    ui.leaderboardRows.textContent = "";
    if (!state.leaderboard.length) {
      const row = document.createElement("tr");
      const cell = document.createElement("td");
      cell.colSpan = 3;
      cell.textContent = "No runs yet.";
      row.append(cell);
      ui.leaderboardRows.append(row);
      return;
    }
    for (const entry of state.leaderboard) {
      const row = document.createElement("tr");
      const value = meta.value(entry);
      for (const cellValue of [entry.rank, entry.username, value]) {
        const cell = document.createElement("td");
        cell.textContent = cellValue;
        row.append(cell);
      }
      ui.leaderboardRows.append(row);
    }
  }

  async function buyUpgrade(upgrade) {
    const response = await fetch("/game/jaylive/shop", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ upgrade }),
    });
    if (!response.ok) return;
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    renderMeta();
  }

  async function setCharacter(action, character) {
    const response = await fetch("/game/jaylive/character", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ action, character }),
    });
    if (!response.ok) return;
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    renderMeta();
  }

  async function loadLeaderboard(sort = state.leaderboardSort) {
    state.leaderboardSort = sort;
    const response = await fetch(`/game/jaylive/leaderboard?sort=${encodeURIComponent(sort)}`, { headers: { Accept: "application/json" } });
    if (!response.ok) return;
    const data = await response.json();
    state.leaderboard = data.leaderboard || [];
    renderLeaderboard();
  }

  function leaderboardMeta(sort) {
    if (sort === "bestRunKills") {
      return {
        eyebrow: "Most Kills In One Run",
        label: "Run Kills",
        value: (entry) => entry.bestRunKills,
      };
    }
    if (sort === "bestRunSeconds") {
      return {
        eyebrow: "Longest Survival Run",
        label: "Run Time",
        value: (entry) => formatTime(entry.bestRunSeconds || 0),
      };
    }
    return {
      eyebrow: "Total Enemies Killed",
      label: "Total Kills",
      value: (entry) => entry.totalKills,
    };
  }

  function statsFromProfile() {
    const p = state.profile || {};
    return {
      maxHp: 100 + (p.maxHpLevel || 0) * 12,
      damage: 10 + (p.damageLevel || 0) * 2,
      speed: 245 + (p.moveSpeedLevel || 0) * 13,
      attackCooldown: Math.max(0.22, 0.62 - (p.attackSpeedLevel || 0) * 0.035),
    };
  }

  function startRun() {
    const stats = statsFromProfile();
    player = {
      x: world.width / 2,
      y: world.height / 2,
      r: 24,
      angle: 0,
      hp: stats.maxHp,
      maxHp: stats.maxHp,
      damage: stats.damage,
      speed: stats.speed,
      character: state.profile?.selectedCharacter || "jaylub",
      attackCooldown: stats.attackCooldown,
      attackTimer: 0,
      invuln: 0,
      kills: 0,
      runGold: 0,
      startedAt: performance.now(),
      survivalSeconds: 0,
    };
    enemies = [];
    goldDrops = [];
    healDrops = [];
    particles = [];
    numbers = [];
    slashes = [];
    projectiles = [];
    nextEnemyId = 1;
    state.spawnTimer = 0;
    state.healSpawnTimer = 8 + Math.random() * 10;
    state.shake = 0;
    state.hitPause = 0;
    state.paused = false;
    state.pausedAt = 0;
    state.dying = false;
    state.deathTimer = 0;
    state.submittedGameOver = false;
    state.running = true;
    showScreen("game");
    updateHud();
  }

  function difficulty() {
    const minutes = Math.max(0, player.survivalSeconds / 60);
    return {
      hp: 26 + minutes * 14,
      damage: 8 + minutes * 3.2,
      speed: 86 + minutes * 7,
      spawnEvery: Math.max(0.22, 1.15 - minutes * 0.08),
      pack: Math.min(5, 1 + Math.floor(minutes / 1.5)),
      eliteChance: player.survivalSeconds < 45 ? 0 : Math.min(0.58, 0.08 + (player.survivalSeconds - 45) / 260),
    };
  }

  function spawnEnemy() {
    const d = difficulty();
    const elite = Math.random() < d.eliteChance;
    const side = Math.floor(Math.random() * 4);
    let x = 0;
    let y = 0;
    if (side === 0) { x = Math.random() * world.width; y = -40; }
    if (side === 1) { x = world.width + 40; y = Math.random() * world.height; }
    if (side === 2) { x = Math.random() * world.width; y = world.height + 40; }
    if (side === 3) { x = -40; y = Math.random() * world.height; }
    enemies.push({
      id: nextEnemyId++,
      x,
      y,
      type: elite ? "elite" : "normal",
      r: elite ? 28 : 22,
      hp: elite ? d.hp * 2.35 : d.hp,
      maxHp: elite ? d.hp * 2.35 : d.hp,
      damage: elite ? d.damage * 1.75 : d.damage,
      speed: (elite ? d.speed * 1.12 : d.speed) * (0.88 + Math.random() * 0.24),
      hitTimer: 0,
      contactTimer: 0,
      angle: 0,
    });
  }

  function update(dt) {
    if (!state.running || !player) return;
    if (state.paused) return;
    if (state.dying) {
      state.deathTimer -= dt;
      state.shake = state.deathTimer > 0 ? 14 : 0;
      updateEffects(dt);
      updateHud();
      if (state.deathTimer <= 0) {
        endRun();
      }
      return;
    }
    if (state.hitPause > 0) {
      state.hitPause -= dt;
      return;
    }

    player.survivalSeconds = Math.floor((performance.now() - player.startedAt) / 1000);
    player.attackTimer = Math.max(0, player.attackTimer - dt);
    player.invuln = Math.max(0, player.invuln - dt);
    state.shake = Math.max(0, state.shake - dt * 28);

    updateMouseWorld();
    updatePlayer(dt);
    updateCamera();
    autoAttack();
    updateSpawns(dt);
    updateHealSpawns(dt);
    updateEnemies(dt);
    updateDrops(dt);
    updateProjectiles(dt);
    updateEffects(dt);
    updateHud();

    if (player.hp <= 0) {
      startDeathShake();
    }
  }

  function updateMouseWorld() {
    const rect = canvas.getBoundingClientRect();
    mouse.worldX = mouse.x - rect.left + camera.x;
    mouse.worldY = mouse.y - rect.top + camera.y;
    player.angle = Math.atan2(mouse.worldY - player.y, mouse.worldX - player.x);
  }

  function updatePlayer(dt) {
    let ax = 0;
    let ay = 0;
    if (keys.has("KeyW")) ay -= 1;
    if (keys.has("KeyS")) ay += 1;
    if (keys.has("KeyA")) ax -= 1;
    if (keys.has("KeyD")) ax += 1;
    const len = Math.hypot(ax, ay) || 1;
    player.x += (ax / len) * player.speed * dt;
    player.y += (ay / len) * player.speed * dt;
    player.x = clamp(player.x, player.r, world.width - player.r);
    player.y = clamp(player.y, player.r, world.height - player.r);
  }

  function updateCamera() {
    camera.x = clamp(player.x - viewWidth() / 2, 0, world.width - viewWidth());
    camera.y = clamp(player.y - viewHeight() / 2, 0, world.height - viewHeight());
  }

  function updateSpawns(dt) {
    const d = difficulty();
    state.spawnTimer -= dt;
    if (state.spawnTimer <= 0) {
      for (let i = 0; i < d.pack; i++) spawnEnemy();
      state.spawnTimer = d.spawnEvery;
    }
  }

  function updateHealSpawns(dt) {
    state.healSpawnTimer -= dt;
    if (state.healSpawnTimer > 0) return;

    const missingHp = player.maxHp - player.hp;
    const healChance = missingHp > 0 ? 0.46 : 0.16;
    if (healDrops.length < 3 && Math.random() < healChance) {
      healDrops.push({
        x: 80 + Math.random() * (world.width - 160),
        y: 80 + Math.random() * (world.height - 160),
        amount: 15,
        life: 0,
      });
    }
    state.healSpawnTimer = 9 + Math.random() * 12;
  }

  function updateEnemies(dt) {
    for (const enemy of enemies) {
      const dx = player.x - enemy.x;
      const dy = player.y - enemy.y;
      const len = Math.hypot(dx, dy) || 1;
      enemy.angle = Math.atan2(dy, dx);
      enemy.x += (dx / len) * enemy.speed * dt;
      enemy.y += (dy / len) * enemy.speed * dt;
      enemy.hitTimer = Math.max(0, enemy.hitTimer - dt);
      enemy.contactTimer = Math.max(0, enemy.contactTimer - dt);

      if (circleHit(player, enemy) && enemy.contactTimer <= 0 && player.invuln <= 0) {
        damagePlayer(enemy.damage);
        enemy.contactTimer = 0.65;
      }
    }
  }

  function updateDrops(dt) {
    for (let i = goldDrops.length - 1; i >= 0; i--) {
      const drop = goldDrops[i];
      drop.life += dt;
      if (Math.hypot(player.x - drop.x, player.y - drop.y) < player.r + 18) {
        player.runGold += drop.value;
        burst(drop.x, drop.y, "#d5a43a", 10);
        numbers.push({ x: drop.x, y: drop.y - 20, text: `+${drop.value}`, color: "#d5a43a", life: 0.8 });
        goldDrops.splice(i, 1);
      }
    }

    for (let i = healDrops.length - 1; i >= 0; i--) {
      const drop = healDrops[i];
      drop.life += dt;
      if (Math.hypot(player.x - drop.x, player.y - drop.y) < player.r + 20) {
        const healed = Math.min(drop.amount, player.maxHp - player.hp);
        if (healed > 0) {
          player.hp += healed;
          burst(drop.x, drop.y, "#7fb069", 14);
          numbers.push({ x: drop.x, y: drop.y - 20, text: `+${healed} HP`, color: "#9bd27b", life: 0.9 });
        } else {
          burst(drop.x, drop.y, "#e7dfc9", 8);
        }
        healDrops.splice(i, 1);
      }
    }
  }

  function updateEffects(dt) {
    for (let i = particles.length - 1; i >= 0; i--) {
      const p = particles[i];
      p.x += p.vx * dt;
      p.y += p.vy * dt;
      p.life -= dt;
      if (p.life <= 0) particles.splice(i, 1);
    }
    for (let i = numbers.length - 1; i >= 0; i--) {
      const n = numbers[i];
      n.y -= 36 * dt;
      n.life -= dt;
      if (n.life <= 0) numbers.splice(i, 1);
    }
    for (let i = slashes.length - 1; i >= 0; i--) {
      slashes[i].life -= dt;
      if (slashes[i].life <= 0) slashes.splice(i, 1);
    }
  }

  function updateProjectiles(dt) {
    for (let i = projectiles.length - 1; i >= 0; i--) {
      const projectile = projectiles[i];
      projectile.x += projectile.vx * dt;
      projectile.y += projectile.vy * dt;
      projectile.life -= dt;

      let hit = false;
      for (let enemyIndex = enemies.length - 1; enemyIndex >= 0; enemyIndex--) {
        const enemy = enemies[enemyIndex];
        if (projectile.hitEnemyIds.has(enemy.id)) continue;
        if (Math.hypot(projectile.x - enemy.x, projectile.y - enemy.y) > projectile.r + enemy.r) continue;

        projectile.hitEnemyIds.add(enemy.id);
        enemy.hp -= projectile.damage;
        enemy.hitTimer = 0.12;
        numbers.push({ x: enemy.x, y: enemy.y - 28, text: String(projectile.damage), color: "#9bd27b", life: 0.62 });
        burst(projectile.x, projectile.y, "#7fb069", 8);
        if (enemy.hp <= 0) killEnemy(enemyIndex);
        projectile.pierceLeft--;
        if (projectile.pierceLeft < 0) {
          hit = true;
          break;
        }
      }

      if (hit || projectile.life <= 0 || projectile.x < 0 || projectile.y < 0 || projectile.x > world.width || projectile.y > world.height) {
        projectiles.splice(i, 1);
      }
    }
  }

  function attack() {
    if (!state.running || state.paused || state.dying || player.attackTimer > 0) return;
    player.attackTimer = player.attackCooldown;
    if (player.character === "goblin_jaylub") {
      shootProjectile();
      return;
    }

    const slash = { x: player.x, y: player.y, angle: player.angle, life: 0.16, maxLife: 0.16 };
    slashes.push(slash);
    state.shake = Math.max(state.shake, 3);

    for (const enemy of enemies) {
      const dx = enemy.x - player.x;
      const dy = enemy.y - player.y;
      const dist = Math.hypot(dx, dy);
      const angle = Math.atan2(dy, dx);
      const diff = Math.abs(wrapAngle(angle - player.angle));
      if (dist < 92 && diff < 0.95) {
        enemy.hp -= player.damage;
        enemy.hitTimer = 0.12;
        numbers.push({ x: enemy.x, y: enemy.y - 28, text: String(player.damage), color: "#f1ead7", life: 0.62 });
        burst(enemy.x, enemy.y, "#9f3f36", 8);
      }
    }

    for (let i = enemies.length - 1; i >= 0; i--) {
      if (enemies[i].hp <= 0) killEnemy(i);
    }
  }

  function autoAttack() {
    attack();
  }

  function shootProjectile() {
    const speed = 680;
    const vx = Math.cos(player.angle) * speed;
    const vy = Math.sin(player.angle) * speed;
    projectiles.push({
      x: player.x + Math.cos(player.angle) * 32,
      y: player.y + Math.sin(player.angle) * 32,
      vx,
      vy,
      r: 8,
      damage: player.damage,
      pierceLeft: state.profile?.piercingLevel || 0,
      hitEnemyIds: new Set(),
      life: 1.15,
      angle: player.angle,
    });
    state.shake = Math.max(state.shake, 2);
  }

  function killEnemy(index) {
    const enemy = enemies[index];
    player.kills++;
    const goldChance = enemy.type === "elite" ? 0.82 : 0.58;
    if (Math.random() < goldChance) {
      const value = enemy.type === "elite" ? 4 + Math.floor(Math.random() * 5) : 1 + Math.floor(Math.random() * 4);
      goldDrops.push({ x: enemy.x, y: enemy.y, value, life: 0 });
    }
    burst(enemy.x, enemy.y, enemy.type === "elite" ? "#9f5f3a" : "#6f7356", enemy.type === "elite" ? 28 : 18);
    enemies.splice(index, 1);
  }

  function damagePlayer(amount) {
    if (state.dying) return;
    player.hp -= Math.ceil(amount);
    player.invuln = 0.45;
    state.hitPause = 0.035;
    state.shake = 12;
    numbers.push({ x: player.x, y: player.y - 34, text: `-${Math.ceil(amount)}`, color: "#d95b4d", life: 0.75 });
    burst(player.x, player.y, "#d95b4d", 12);
  }

  function startDeathShake() {
    if (state.dying) return;
    player.hp = 0;
    state.dying = true;
    state.deathTimer = 1;
    state.hitPause = 0;
    state.shake = 14;
    burst(player.x, player.y, "#d95b4d", 24);
  }

  function pauseRun() {
    if (!state.running || state.paused || state.dying) return;
    state.paused = true;
    state.pausedAt = performance.now();
    keys.clear();
    showScreen("pause");
  }

  function resumeRun() {
    if (!state.running || !state.paused || !player) return;
    player.startedAt += performance.now() - state.pausedAt;
    state.paused = false;
    state.pausedAt = 0;
    state.lastTime = performance.now();
    showScreen("game");
  }

  function quitRun() {
    state.running = false;
    state.paused = false;
    state.pausedAt = 0;
    state.dying = false;
    state.deathTimer = 0;
    state.shake = 0;
    keys.clear();
    player = undefined;
    enemies = [];
    goldDrops = [];
    healDrops = [];
    particles = [];
    numbers = [];
    slashes = [];
    projectiles = [];
    showScreen("menu");
  }

  async function endRun() {
    state.running = false;
    ui.finalTime.textContent = formatTime(player.survivalSeconds);
    ui.finalKills.textContent = player.kills;
    ui.finalGold.textContent = player.runGold;
    showScreen("gameOver");

    if (state.submittedGameOver) return;
    state.submittedGameOver = true;
    const response = await fetch("/game/jaylive/run", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({
        kills: player.kills,
        gold: player.runGold,
        survivalSeconds: player.survivalSeconds,
      }),
    });
    if (response.ok) {
      const data = await response.json();
      state.profile = data.profile;
      state.shop = data.shop || state.shop;
      state.leaderboard = data.leaderboard || state.leaderboard;
      renderMeta();
    }
  }

  function draw() {
    const w = viewWidth();
    const h = viewHeight();
    ctx.clearRect(0, 0, w, h);
    ctx.save();
    const shakeX = state.shake ? (Math.random() - 0.5) * state.shake : 0;
    const shakeY = state.shake ? (Math.random() - 0.5) * state.shake : 0;
    ctx.translate(Math.round(-camera.x + shakeX), Math.round(-camera.y + shakeY));

    drawWorld();
    for (const drop of goldDrops) drawGold(drop);
    for (const drop of healDrops) drawHeal(drop);
    for (const enemy of enemies) drawEnemy(enemy);
    if (player) {
      drawPlayer();
      for (const slash of slashes) drawSlash(slash);
      for (const projectile of projectiles) drawProjectile(projectile);
    }
    for (const p of particles) drawParticle(p);
    for (const n of numbers) drawNumber(n);

    ctx.restore();
  }

  function drawWorld() {
    ctx.fillStyle = "#303028";
    ctx.fillRect(0, 0, world.width, world.height);

    const tile = 64;
    for (let y = 0; y < world.height; y += tile) {
      for (let x = 0; x < world.width; x += tile) {
        ctx.fillStyle = ((x / tile + y / tile) % 2 === 0) ? "#34332a" : "#2c302b";
        ctx.fillRect(x, y, tile, tile);
      }
    }

    ctx.strokeStyle = "#4b4538";
    ctx.lineWidth = 6;
    ctx.strokeRect(12, 12, world.width - 24, world.height - 24);

    for (const d of rngDetails) {
      ctx.fillStyle = d.color;
      ctx.fillRect(d.x, d.y, d.w, d.h);
      if (d.mark) {
        ctx.fillStyle = "rgba(10, 13, 14, 0.28)";
        ctx.fillRect(d.x + 4, d.y + 4, Math.max(4, d.w - 8), 4);
      }
    }
  }

  function drawPlayer() {
    const image = player.character === "goblin_jaylub" ? goblinImage : playerImage;
    ctx.save();
    ctx.translate(Math.round(player.x), Math.round(player.y));
    ctx.rotate(player.angle);
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(-22, 18, 44, 10);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -28, -28, 56, 56);
    } else {
      ctx.fillStyle = player.character === "goblin_jaylub" ? "#7fb069" : "#d5a43a";
      ctx.fillRect(-24, -24, 48, 48);
    }
    ctx.fillStyle = "#e7dfc9";
    ctx.fillRect(18, -4, 18, 8);
    ctx.restore();
  }

  function drawEnemy(enemy) {
    const size = enemy.type === "elite" ? 60 : 48;
    const half = size / 2;
    const shadowW = enemy.type === "elite" ? 34 : 40;
    const shadowH = enemy.type === "elite" ? 7 : 10;
    const image = enemy.type === "elite" ? enemy2Image : enemyImage;
    ctx.save();
    ctx.translate(Math.round(enemy.x), Math.round(enemy.y));
    ctx.rotate(enemy.angle);
    ctx.globalAlpha = enemy.hitTimer > 0 ? 0.65 : 1;
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(-shadowW / 2, half - 4, shadowW, shadowH);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -half, -half, size, size);
    } else {
      ctx.fillStyle = enemy.type === "elite" ? "#7a4c3a" : "#758d54";
      ctx.fillRect(-half + 2, -half + 2, size - 4, size - 4);
    }
    ctx.restore();

    const barW = enemy.type === "elite" ? 48 : 38;
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(enemy.x - barW / 2, enemy.y - half - 10, barW, 6);
    ctx.fillStyle = enemy.type === "elite" ? "#c46a42" : "#a93b36";
    ctx.fillRect(enemy.x - barW / 2 + 2, enemy.y - half - 8, (barW - 4) * clamp(enemy.hp / enemy.maxHp, 0, 1), 2);
  }

  function drawSlash(slash) {
    const t = slash.life / slash.maxLife;
    ctx.save();
    ctx.translate(slash.x, slash.y);
    ctx.rotate(slash.angle);
    ctx.globalAlpha = Math.max(0, t);
    ctx.fillStyle = "#e7dfc9";
    ctx.fillRect(24, -34, 12, 68);
    ctx.fillStyle = "#d5a43a";
    ctx.fillRect(36, -26, 12, 52);
    ctx.fillStyle = "#f1ead7";
    ctx.fillRect(48, -14, 28, 28);
    ctx.restore();
  }

  function drawProjectile(projectile) {
    ctx.save();
    ctx.translate(projectile.x, projectile.y);
    ctx.rotate(projectile.angle);
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(-8, -8, 20, 16);
    ctx.fillStyle = "#7fb069";
    ctx.fillRect(-5, -5, 16, 10);
    ctx.fillStyle = "#c8e6a0";
    ctx.fillRect(4, -3, 5, 4);
    ctx.restore();
  }

  function drawGold(drop) {
    const bob = Math.sin(drop.life * 8) * 3;
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(drop.x - 8, drop.y + 8, 16, 5);
    ctx.fillStyle = "#d5a43a";
    ctx.fillRect(drop.x - 7, drop.y - 7 + bob, 14, 14);
    ctx.fillStyle = "#f1d879";
    ctx.fillRect(drop.x - 3, drop.y - 5 + bob, 5, 4);
  }

  function drawHeal(drop) {
    const bob = Math.sin(drop.life * 6) * 4;
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(drop.x - 14, drop.y + 13, 28, 7);
    if (healImage.complete && healImage.naturalWidth) {
      ctx.drawImage(healImage, drop.x - 18, drop.y - 18 + bob, 36, 36);
      return;
    }
    ctx.fillStyle = "#e7dfc9";
    ctx.fillRect(drop.x - 14, drop.y - 14 + bob, 28, 28);
    ctx.fillStyle = "#7fb069";
    ctx.fillRect(drop.x - 4, drop.y - 11 + bob, 8, 22);
    ctx.fillRect(drop.x - 11, drop.y - 4 + bob, 22, 8);
  }

  function drawParticle(p) {
    ctx.globalAlpha = Math.max(0, p.life / p.maxLife);
    ctx.fillStyle = p.color;
    ctx.fillRect(Math.round(p.x), Math.round(p.y), p.size, p.size);
    ctx.globalAlpha = 1;
  }

  function drawNumber(n) {
    ctx.globalAlpha = Math.max(0, n.life);
    ctx.fillStyle = "#0a0d0e";
    ctx.font = "900 18px 'Courier New', monospace";
    ctx.fillText(n.text, Math.round(n.x + 2), Math.round(n.y + 2));
    ctx.fillStyle = n.color;
    ctx.fillText(n.text, Math.round(n.x), Math.round(n.y));
    ctx.globalAlpha = 1;
  }

  function updateHud() {
    ui.hpFill.style.width = `${clamp(player.hp / player.maxHp, 0, 1) * 100}%`;
    ui.hpText.textContent = `${Math.max(0, Math.ceil(player.hp))} / ${player.maxHp}`;
    ui.runGold.textContent = player.runGold;
    ui.runKills.textContent = player.kills;
    ui.runTime.textContent = formatTime(player.survivalSeconds);
  }

  function burst(x, y, color, count) {
    for (let i = 0; i < count; i++) {
      const angle = Math.random() * Math.PI * 2;
      const speed = 40 + Math.random() * 150;
      const life = 0.25 + Math.random() * 0.45;
      particles.push({
        x,
        y,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed,
        color,
        size: 3 + Math.floor(Math.random() * 4),
        life,
        maxLife: life,
      });
    }
  }

  function makeEnvironment() {
    const details = [];
    const colors = ["#484533", "#3f4132", "#4e463b", "#252a27", "#5c5240"];
    for (let i = 0; i < 180; i++) {
      details.push({
        x: Math.floor(Math.random() * world.width / 16) * 16,
        y: Math.floor(Math.random() * world.height / 16) * 16,
        w: 8 + Math.floor(Math.random() * 4) * 8,
        h: 8 + Math.floor(Math.random() * 3) * 8,
        color: colors[Math.floor(Math.random() * colors.length)],
        mark: Math.random() > 0.62,
      });
    }
    return details;
  }

  function circleHit(a, b) {
    return Math.hypot(a.x - b.x, a.y - b.y) < a.r + b.r;
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function wrapAngle(angle) {
    while (angle < -Math.PI) angle += Math.PI * 2;
    while (angle > Math.PI) angle -= Math.PI * 2;
    return angle;
  }

  function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${String(secs).padStart(2, "0")}`;
  }

  async function toggleFullscreen() {
    if (!document.fullscreenElement) {
      await stage.requestFullscreen();
      return;
    }
    await document.exitFullscreen();
  }

  function updateFullscreenButton() {
    const label = document.fullscreenElement === stage ? "Exit Fullscreen" : "Fullscreen";
    fullscreenButtons.forEach((button) => {
      button.textContent = label;
    });
  }

  function loop(now) {
    const dt = Math.min(0.033, (now - state.lastTime) / 1000);
    state.lastTime = now;
    update(dt);
    draw();
    requestAnimationFrame(loop);
  }

  document.getElementById("playButton").addEventListener("click", startRun);
  document.getElementById("playAgainButton").addEventListener("click", startRun);
  document.getElementById("mainMenuButton").addEventListener("click", () => showScreen("menu"));
  document.getElementById("continueButton").addEventListener("click", resumeRun);
  document.getElementById("quitButton").addEventListener("click", quitRun);
  fullscreenButtons.forEach((button) => {
    button.addEventListener("click", () => {
      toggleFullscreen().catch(() => {});
    });
  });
  document.getElementById("shopButton").addEventListener("click", () => {
    renderShop();
    showScreen("shop");
  });
  document.getElementById("leaderboardButton").addEventListener("click", () => {
    renderLeaderboard();
    showScreen("leaderboard");
    loadLeaderboard();
  });
  document.querySelectorAll("[data-leaderboard-sort]").forEach((button) => {
    button.addEventListener("click", () => loadLeaderboard(button.dataset.leaderboardSort));
  });
  document.querySelectorAll("[data-menu-back]").forEach((button) => {
    button.addEventListener("click", () => showScreen("menu"));
  });

  window.addEventListener("keydown", (event) => {
    if (["KeyW", "KeyA", "KeyS", "KeyD", "Escape"].includes(event.code)) {
      event.preventDefault();
    }
    if (event.code === "Escape") {
      if (state.paused) resumeRun();
      else pauseRun();
      return;
    }
    if (state.paused) return;
    keys.add(event.code);
  });
  window.addEventListener("keyup", (event) => keys.delete(event.code));
  window.addEventListener("mousemove", (event) => {
    mouse.x = event.clientX;
    mouse.y = event.clientY;
  });
  window.addEventListener("resize", resizeCanvas);
  document.addEventListener("fullscreenchange", () => {
    updateFullscreenButton();
    resizeCanvas();
  });

  resizeCanvas();
  updateFullscreenButton();
  showScreen("menu");
  loadState().catch(() => {});
  requestAnimationFrame(loop);
})();
