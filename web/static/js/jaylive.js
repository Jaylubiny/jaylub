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
    bossPanel: document.getElementById("bossPanel"),
    bossFill: document.getElementById("bossFill"),
    bossText: document.getElementById("bossText"),
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
  const bossImages = [new Image(), new Image(), new Image()];
  bossImages[0].src = "/static/img/boss.png";
  bossImages[1].src = "/static/img/boss2.png";
  bossImages[2].src = "/static/img/boss3.png";

  const bossConfigs = [
    { name: "Boss 1", hp: 500, reward: 250, imageIndex: 0, r: 66, size: 138, speed: 54, cooldown: 1.8, shotSpeed: 230, damage: 14, burstShots: 12 },
    { name: "Boss 2", hp: 1500, reward: 500, imageIndex: 1, r: 80, size: 168, speed: 62, cooldown: 1.35, shotSpeed: 260, damage: 18, burstShots: 14 },
    { name: "Boss 3", hp: 3000, reward: 1000, imageIndex: 2, r: 94, size: 198, speed: 70, cooldown: 1.05, shotSpeed: 290, damage: 24, burstShots: 16 },
  ];

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
    spawnPauseTimer: 0,
    nextBossAt: 180,
    bossEncounter: 0,
    runToken: "",
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
  let boss;
  let bossProjectiles = [];
  let treasureChests = [];
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
      attackCooldown: attackCooldownFromLevel(p.attackSpeedLevel || 0),
    };
  }

  function attackCooldownFromLevel(level) {
    return Math.max(0.018, 0.62 / (1 + level * 0.045 + Math.sqrt(level) * 0.45));
  }

  async function startRun() {
    const runResponse = await fetch("/game/jaylive/start", {
      method: "POST",
      headers: { "Accept": "application/json" },
    });
    if (!runResponse.ok) return;
    const runData = await runResponse.json();
    state.runToken = runData.runToken || "";
    if (!state.runToken) return;

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
    bossProjectiles = [];
    treasureChests = [];
    boss = undefined;
    nextEnemyId = 1;
    state.spawnTimer = 0;
    state.healSpawnTimer = 8 + Math.random() * 10;
    state.spawnPauseTimer = 0;
    state.nextBossAt = 180;
    state.bossEncounter = 0;
    state.runToken = runData.runToken;
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
    const sizeScale = Math.min(1.45, 1 + minutes * 0.035);
    return {
      hp: (26 + minutes * 14) * (1 + minutes * 0.025),
      damage: 8 + minutes * 3.2,
      speed: 86 + minutes * 7,
      spawnEvery: Math.max(0.22, 1.15 - minutes * 0.08),
      pack: Math.min(5, 1 + Math.floor(minutes / 1.5)),
      eliteChance: player.survivalSeconds < 45 ? 0 : Math.min(0.58, 0.08 + (player.survivalSeconds - 45) / 260),
      sizeScale,
    };
  }

  function spawnEnemy(options = {}) {
    const d = difficulty();
    const elite = options.elite ?? Math.random() < d.eliteChance;
    const side = Math.floor(Math.random() * 4);
    let x = 0;
    let y = 0;
    if (side === 0) { x = Math.random() * world.width; y = -40; }
    if (side === 1) { x = world.width + 40; y = Math.random() * world.height; }
    if (side === 2) { x = Math.random() * world.width; y = world.height + 40; }
    if (side === 3) { x = -40; y = Math.random() * world.height; }
    const sizeScale = options.sizeScale || d.sizeScale;
    const radius = (elite ? 28 : 22) * sizeScale;
    enemies.push({
      id: nextEnemyId++,
      x: options.x ?? x,
      y: options.y ?? y,
      type: elite ? "elite" : "normal",
      bossMinion: Boolean(options.bossMinion),
      r: radius,
      size: (elite ? 60 : 48) * sizeScale,
      hp: options.hp ?? (elite ? d.hp * 2.35 : d.hp),
      maxHp: options.hp ?? (elite ? d.hp * 2.35 : d.hp),
      damage: options.damage ?? (elite ? d.damage * 1.75 : d.damage),
      speed: options.speed ?? ((elite ? d.speed * 1.12 : d.speed) * (0.88 + Math.random() * 0.24)),
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
    player.attackTimer -= dt;
    player.invuln = Math.max(0, player.invuln - dt);
    state.shake = Math.max(0, state.shake - dt * 28);

    updateMouseWorld();
    updatePlayer(dt);
    updateCamera();
    autoAttack();
    updateBossTiming(dt);
    updateBoss(dt);
    updateSpawns(dt);
    updateHealSpawns(dt);
    updateEnemies(dt);
    updateDrops(dt);
    updateProjectiles(dt);
    updateBossProjectiles(dt);
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

  function updateBossTiming() {
    if (boss || state.spawnPauseTimer > 0) return;
    if (player.survivalSeconds >= state.nextBossAt) startBossEncounter();
  }

  function startBossEncounter() {
    state.bossEncounter++;
    const config = bossConfigs[Math.min(state.bossEncounter - 1, bossConfigs.length - 1)];
    enemies = [];
    bossProjectiles = [];
    state.spawnTimer = 0;
    state.healSpawnTimer = Math.max(state.healSpawnTimer, 4);

    const angle = Math.random() * Math.PI * 2;
    boss = {
      ...config,
      x: clamp(player.x + Math.cos(angle) * 520, config.r + 30, world.width - config.r - 30),
      y: clamp(player.y + Math.sin(angle) * 520, config.r + 30, world.height - config.r - 30),
      hp: config.hp,
      maxHp: config.hp,
      attackTimer: 1.2,
      patternIndex: 0,
      minionTimer: 0,
      hitTimer: 0,
      angle: 0,
    };
    numbers.push({ x: boss.x, y: boss.y - boss.r - 28, text: `${boss.name} appears`, color: "#d5a43a", life: 1.4 });
    burst(boss.x, boss.y, "#d5a43a", 42);
    maintainBossMinions();
  }

  function updateBoss(dt) {
    if (!boss) return;

    const dx = player.x - boss.x;
    const dy = player.y - boss.y;
    const dist = Math.hypot(dx, dy) || 1;
    boss.angle = Math.atan2(dy, dx);
    const desired = 340;
    const moveSign = dist > desired + 60 ? 1 : dist < desired - 90 ? -1 : 0;
    boss.x = clamp(boss.x + (dx / dist) * boss.speed * moveSign * dt, boss.r, world.width - boss.r);
    boss.y = clamp(boss.y + (dy / dist) * boss.speed * moveSign * dt, boss.r, world.height - boss.r);
    boss.hitTimer = Math.max(0, boss.hitTimer - dt);

    boss.attackTimer -= dt;
    if (boss.attackTimer <= 0) {
      performBossAttack();
      boss.patternIndex = (boss.patternIndex + 1) % 3;
      boss.attackTimer = boss.cooldown;
    }

    boss.minionTimer -= dt;
    if (boss.minionTimer <= 0) {
      maintainBossMinions();
      boss.minionTimer = 15;
    }
  }

  function performBossAttack() {
    if (!boss) return;
    if (boss.patternIndex === 0) {
      fireBossProjectile(boss.angle);
      return;
    }
    if (boss.patternIndex === 1) {
      const spread = Math.PI / 9;
      for (let i = -2; i <= 2; i++) fireBossProjectile(boss.angle + i * spread);
      return;
    }
    for (let i = 0; i < boss.burstShots; i++) {
      fireBossProjectile((Math.PI * 2 * i) / boss.burstShots);
    }
  }

  function fireBossProjectile(angle) {
    const speed = boss.shotSpeed;
    bossProjectiles.push({
      x: boss.x + Math.cos(angle) * (boss.r + 12),
      y: boss.y + Math.sin(angle) * (boss.r + 12),
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      r: 17,
      damage: boss.damage,
      angle,
      life: 6,
    });
  }

  function updateBossProjectiles(dt) {
    for (let i = bossProjectiles.length - 1; i >= 0; i--) {
      const projectile = bossProjectiles[i];
      projectile.x += projectile.vx * dt;
      projectile.y += projectile.vy * dt;
      projectile.life -= dt;

      if (circleHit(player, projectile) && player.invuln <= 0) {
        damagePlayer(projectile.damage);
        burst(projectile.x, projectile.y, "#a93b36", 10);
        bossProjectiles.splice(i, 1);
        continue;
      }

      if (
        projectile.life <= 0 ||
        projectile.x < -80 ||
        projectile.y < -80 ||
        projectile.x > world.width + 80 ||
        projectile.y > world.height + 80
      ) {
        bossProjectiles.splice(i, 1);
      }
    }
  }

  function maintainBossMinions() {
    const active = enemies.filter((enemy) => enemy.bossMinion).length;
    for (let i = active; i < 5; i++) spawnBossMinion();
  }

  function spawnBossMinion() {
    const angle = Math.random() * Math.PI * 2;
    const distance = 260 + Math.random() * 180;
    const d = difficulty();
    spawnEnemy({
      x: clamp((boss?.x ?? player.x) + Math.cos(angle) * distance, 80, world.width - 80),
      y: clamp((boss?.y ?? player.y) + Math.sin(angle) * distance, 80, world.height - 80),
      elite: true,
      bossMinion: true,
      sizeScale: Math.max(1.05, d.sizeScale),
      hp: d.hp * 2.1,
      damage: d.damage * 1.55,
      speed: d.speed * 1.08,
    });
  }

  function damageBoss(amount, x, y) {
    if (!boss) return;
    boss.hp -= amount;
    boss.hitTimer = 0.14;
    numbers.push({ x: x || boss.x, y: (y || boss.y) - boss.r - 18, text: String(amount), color: "#f1ead7", life: 0.66 });
    burst(x || boss.x, y || boss.y, "#d5a43a", 10);
    if (boss.hp <= 0) defeatBoss();
  }

  function defeatBoss() {
    if (!boss) return;
    const defeated = boss;
    treasureChests.push({
      x: defeated.x,
      y: defeated.y,
      value: defeated.reward,
      life: 0,
      collected: false,
    });
    numbers.push({ x: defeated.x, y: defeated.y - defeated.r - 36, text: `${defeated.name} defeated`, color: "#d5a43a", life: 1.35 });
    burst(defeated.x, defeated.y, "#d5a43a", 60);
    boss = undefined;
    bossProjectiles = [];
    enemies = [];
    state.spawnPauseTimer = 5;
    state.nextBossAt = player.survivalSeconds + 180;
  }

  function updateSpawns(dt) {
    if (boss) return;
    state.spawnPauseTimer = Math.max(0, state.spawnPauseTimer - dt);
    if (state.spawnPauseTimer > 0) return;

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
    for (let i = treasureChests.length - 1; i >= 0; i--) {
      const chest = treasureChests[i];
      chest.life += dt;
      if (!chest.collected && Math.hypot(player.x - chest.x, player.y - chest.y) < player.r + 34) {
        chest.collected = true;
        const healed = Math.ceil(player.maxHp * 0.5);
        player.runGold += chest.value;
        player.hp += healed;
        numbers.push({ x: chest.x, y: chest.y - 34, text: `+${chest.value} gold`, color: "#d5a43a", life: 1.1 });
        numbers.push({ x: chest.x, y: chest.y - 58, text: `+${healed} HP`, color: "#9bd27b", life: 1.1 });
        burst(chest.x, chest.y, "#d5a43a", 36);
        burst(chest.x, chest.y, "#7fb069", 18);
        treasureChests.splice(i, 1);
      }
    }

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
        const healed = drop.amount;
        player.hp += healed;
        burst(drop.x, drop.y, "#7fb069", 14);
        numbers.push({ x: drop.x, y: drop.y - 20, text: `+${healed} HP`, color: "#9bd27b", life: 0.9 });
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

      if (!hit && boss && !projectile.hitBoss && Math.hypot(projectile.x - boss.x, projectile.y - boss.y) <= projectile.r + boss.r) {
        projectile.hitBoss = true;
        damageBoss(projectile.damage, projectile.x, projectile.y);
        hit = true;
      }

      if (hit || projectile.life <= 0 || projectile.x < 0 || projectile.y < 0 || projectile.x > world.width || projectile.y > world.height) {
        projectiles.splice(i, 1);
      }
    }
  }

  function attack() {
    if (!state.running || state.paused || state.dying || player.attackTimer > 0) return false;
    if (player.character === "goblin_jaylub") {
      shootProjectile();
      return true;
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

    if (boss) {
      const dx = boss.x - player.x;
      const dy = boss.y - player.y;
      const dist = Math.hypot(dx, dy);
      const angle = Math.atan2(dy, dx);
      const diff = Math.abs(wrapAngle(angle - player.angle));
      if (dist < boss.r + 92 && diff < 1.05) {
        damageBoss(player.damage, boss.x, boss.y);
      }
    }
    return true;
  }

  function autoAttack() {
    let attacks = 0;
    while (attacks < 4 && attack()) {
      player.attackTimer += player.attackCooldown;
      attacks++;
    }
    if (attacks >= 4 && player.attackTimer <= 0) {
      player.attackTimer = player.attackCooldown;
    }
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
    bossProjectiles = [];
    treasureChests = [];
    boss = undefined;
    state.spawnPauseTimer = 0;
    state.runToken = "";
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
        runToken: state.runToken,
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
    for (const chest of treasureChests) drawTreasure(chest);
    for (const enemy of enemies) drawEnemy(enemy);
    if (boss) drawBoss();
    for (const projectile of bossProjectiles) drawBossProjectile(projectile);
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
    const size = enemy.size || (enemy.type === "elite" ? 60 : 48);
    const half = size / 2;
    const shadowW = enemy.type === "elite" ? size * 0.56 : size * 0.82;
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

  function drawBoss() {
    const image = bossImages[boss.imageIndex];
    const half = boss.size / 2;
    ctx.save();
    ctx.translate(Math.round(boss.x), Math.round(boss.y));
    ctx.rotate(boss.angle);
    ctx.globalAlpha = boss.hitTimer > 0 ? 0.72 : 1;
    ctx.fillStyle = "rgba(0, 0, 0, 0.34)";
    ctx.fillRect(-half * 0.55, half - 8, half * 1.1, 16);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -half, -half, boss.size, boss.size);
    } else {
      ctx.fillStyle = "#8a4f3b";
      ctx.fillRect(-half, -half, boss.size, boss.size);
    }
    ctx.restore();
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

  function drawBossProjectile(projectile) {
    const size = 34;
    ctx.save();
    ctx.translate(projectile.x, projectile.y);
    ctx.rotate(projectile.angle);
    ctx.fillStyle = "rgba(0, 0, 0, 0.3)";
    ctx.fillRect(-13, 12, 26, 6);
    if (enemyImage.complete && enemyImage.naturalWidth) {
      ctx.drawImage(enemyImage, -size / 2, -size / 2, size, size);
    } else {
      ctx.fillStyle = "#758d54";
      ctx.fillRect(-14, -14, 28, 28);
    }
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

  function drawTreasure(chest) {
    const bob = Math.sin(chest.life * 5) * 3;
    ctx.fillStyle = "rgba(0, 0, 0, 0.32)";
    ctx.fillRect(chest.x - 24, chest.y + 18, 48, 10);
    ctx.fillStyle = "#5c2f1f";
    ctx.fillRect(chest.x - 24, chest.y - 12 + bob, 48, 32);
    ctx.fillStyle = "#d5a43a";
    ctx.fillRect(chest.x - 24, chest.y - 12 + bob, 48, 9);
    ctx.fillRect(chest.x - 4, chest.y - 12 + bob, 8, 32);
    ctx.fillStyle = "#f1d879";
    ctx.fillRect(chest.x - 2, chest.y + 2 + bob, 4, 6);
    ctx.strokeStyle = "#0a0d0e";
    ctx.lineWidth = 3;
    ctx.strokeRect(chest.x - 24, chest.y - 12 + bob, 48, 32);
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
    if (ui.bossPanel) {
      ui.bossPanel.hidden = !boss;
      if (boss) {
        ui.bossFill.style.width = `${clamp(boss.hp / boss.maxHp, 0, 1) * 100}%`;
        ui.bossText.textContent = `${boss.name} ${Math.max(0, Math.ceil(boss.hp))} / ${boss.maxHp}`;
      }
    }
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
